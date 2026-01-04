package tripay

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type TripayService struct {
	Client       *http.Client
	APIKey       string
	PrivateKey   string
	MerchantCode string
	BaseURL      string
}

func NewTripayService() *TripayService {
	baseURL := "https://tripay.co.id/api-sandbox" // Default to sandbox
	if os.Getenv("TRIPAY_ENV") == "production" {
		baseURL = "https://tripay.co.id/api"
	}

	return &TripayService{
		Client:       &http.Client{Timeout: 15 * time.Second},
		APIKey:       os.Getenv("TRIPAY_API_KEY"),
		PrivateKey:   os.Getenv("TRIPAY_PRIVATE_KEY"),
		MerchantCode: os.Getenv("TRIPAY_MERCHANT_CODE"),
		BaseURL:      baseURL,
	}
}

type OrderItem struct {
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Quantity int    `json:"quantity"`
}

type TransactionRequest struct {
	Method        string      `json:"method"`
	MerchantRef   string      `json:"merchant_ref"`
	Amount        int64       `json:"amount"`
	CustomerName  string      `json:"customer_name"`
	CustomerEmail string      `json:"customer_email"`
	CustomerPhone string      `json:"customer_phone"`
	OrderItems    []OrderItem `json:"order_items"`
	Callback      string      `json:"callback_url"`
	ReturnUrl     string      `json:"return_url"`
	ExpiredTime   int64       `json:"expired_time"` // Unix timestamp
	Signature     string      `json:"signature"`
}

type TransactionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Reference   string `json:"reference"`
		MerchantRef string `json:"merchant_ref"`
		CheckoutURL string `json:"checkout_url"`
		Amount      int64  `json:"amount"`
	} `json:"data"`
}

func (s *TripayService) CreateTransaction(
	merchantRef string,
	amount int64,
	customerName, customerEmail, customerPhone string,
	itemName string,
	method string,
	returnUrl string,
) (*TransactionResponse, error) {

	// 1. Calculate Signature
	// HMAC-SHA256( merchant_code + merchant_ref + amount, private_key )
	sigData := fmt.Sprintf("%s%s%d", s.MerchantCode, merchantRef, amount)
	signature := s.generateSignature(sigData)

	// 2. Prepare Request
	// Defaulting to "BCAVA" or any valid method, usually closed payment requires method.
	// However, for "Closed Payment" in Tripay, we typically assume specific method or allow user to choose.
	// If we use "Open Payment" creating a generic checkout URL might be different.
	// For simplicity in this flow, we might try to use their "Closed Payment" but getting a Checkout URL usually implies
	// creating a transaction with a method OR using their payment page if supported.
	// Let's assume we want to redirect to their hosted checkout if possible, or we pick a default method for now
	// OR we might need to ask the user to pick a method first?
	// The user requirement says "redirect to tripay", so usually that means getting a checkout_url.
	// For Closed Payment, Tripay returns a 'checkout_url' that displays payment instructions.

	// Hardcoding 'BCAVA' for now as placeholder if needed, or maybe Tripay allows empty method for hosted?
	// Actually Tripay API 'request transaction' needs 'method'.
	// Let's use a popular one or make it dynamic if we had a frontend selector.
	// For "Pay button" that redirects, maybe we use a general method or just "QRIS" as default.
	// method := "QRIS"
	// Use method passed in argument function signature (but wait, signature previously didn't have method)

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("APP_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "https://2a117ce1bea3.ngrok-free.app" // Default local backend
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://127.0.0.1:3000" // Default local frontend
	}

	reqBody := TransactionRequest{
		Method:        method,
		MerchantRef:   merchantRef,
		Amount:        amount,
		CustomerName:  customerName,
		CustomerEmail: customerEmail,
		CustomerPhone: customerPhone,
		OrderItems: []OrderItem{
			{
				Name:     itemName,
				Price:    amount,
				Quantity: 1,
			},
		},
		Callback:    baseURL + "/tripay/callback",
		ReturnUrl:   returnUrl,
		ExpiredTime: time.Now().Add(24 * time.Hour).Unix(),
		Signature:   signature,
	}

	jsonBody, _ := json.Marshal(reqBody)

	// 3. Send Request
	req, err := http.NewRequest("POST", s.BaseURL+"/transaction/create", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	var apiResp TransactionResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("tripay error: %s", apiResp.Message)
	}

	return &apiResp, nil
}

type PaymentChannel struct {
	Group string `json:"group"`
	Code  string `json:"code"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Fee   struct {
		Flat    interface{} `json:"flat"`
		Percent interface{} `json:"percent"`
	} `json:"total_fee"`
	IconURL string `json:"icon_url"`
}

type ChannelResponse struct {
	Success bool             `json:"success"`
	Message string           `json:"message"`
	Data    []PaymentChannel `json:"data"`
}

func (s *TripayService) GetPaymentChannels() ([]PaymentChannel, error) {
	req, err := http.NewRequest("GET", s.BaseURL+"/merchant/payment-channel", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.APIKey)

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	var apiResp ChannelResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("tripay error: %s", apiResp.Message)
	}

	return apiResp.Data, nil
}

func (s *TripayService) generateSignature(data string) string {
	h := hmac.New(sha256.New, []byte(s.PrivateKey))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *TripayService) ValidateSignature(incomingSig, jsonBody string) bool {
	// Validating callback signature
	// Tripay Callback Signature: HMAC-SHA256( JSON_BODY, private_key )
	h := hmac.New(sha256.New, []byte(s.PrivateKey))
	h.Write([]byte(jsonBody))
	calculated := hex.EncodeToString(h.Sum(nil))
	return calculated == incomingSig
}
