package handlers

import (
	"log"
	"math"
	"strconv"
	"time"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/realtime"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/services/tripay"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type PaymentHandler struct {
	DB            *gorm.DB
	TripayService *tripay.TripayService
	Hub           *realtime.Hub
}

func NewPaymentHandler(db *gorm.DB, tripayService *tripay.TripayService, hub *realtime.Hub) *PaymentHandler {
	return &PaymentHandler{DB: db, TripayService: tripayService, Hub: hub}
}

type CreatePaymentRequest struct {
	OfferID       string `json:"offer_id"`
	PaymentMethod string `json:"payment_method"`
}

func (h *PaymentHandler) GetChannels(c *fiber.Ctx) error {
	channels, err := h.TripayService.GetPaymentChannels()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to fetch channels: " + err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": channels})
}

func (h *PaymentHandler) CreatePayment(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}

	var req CreatePaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid request"})
	}

	if req.PaymentMethod == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Payment method is required"})
	}

	// Fetch Offer
	var offer models.JobOffer
	if err := h.DB.Preload("Client").Preload("Freelancer").First(&offer, "id = ?", req.OfferID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "Offer not found"})
	}

	// Validate User is Client
	if offer.ClientID.String() != userID.(string) {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "Only client can pay for this offer"})
	}

	// Validate Offer Status
	if offer.Status != models.OfferStatusPending {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Offer is not in pending status"})
	}

	// Init Tripay Transaction
	// Using "OFFER-{OrderCode}" ensures uniqueness and reference
	merchantRef := "INV-" + offer.OrderCode

	// Ensure client data exists
	clientName := offer.Client.Name
	clientEmail := offer.Client.Email
	clientPhone := "08123456789" // Placeholder if phone not in User model, ideally should be fetched

	// Fetch Channels to calculate correct fee
	channels, err := h.TripayService.GetPaymentChannels()
	if err != nil {
		log.Printf("Failed to fetch channels for fee calculation: %v", err)
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to calculate fees"})
	}

	var selectedChannel tripay.PaymentChannel
	var channelFound bool
	for _, ch := range channels {
		if ch.Code == req.PaymentMethod {
			selectedChannel = ch
			channelFound = true
			break
		}
	}

	if !channelFound {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid payment method"})
	}

	// Calculate Fee
	var flatFee float64
	var percentFee float64

	// Helper to safely parse interface{} to float64
	toFloat := func(v interface{}) float64 {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case string:
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				return f
			}
		}
		return 0
	}

	flatFee = toFloat(selectedChannel.Fee.Flat)
	percentFee = toFloat(selectedChannel.Fee.Percent)

	totalFee := flatFee + (float64(offer.Price) * percentFee / 100)
	totalAmount := offer.Price + int64(math.Ceil(totalFee))

	resp, err := h.TripayService.CreateTransaction(
		merchantRef,
		totalAmount,
		clientName,
		clientEmail,
		clientPhone,
		offer.Title,
		req.PaymentMethod,
	)

	if err != nil {
		log.Printf("Tripay error: %v", err)
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Payment gateway error: " + err.Error()})
	}

	// Create Transaction Record
	trx := models.Transaction{
		JobOfferID:        offer.ID,
		Reference:         resp.Data.Reference,
		MerchantRef:       resp.Data.MerchantRef,
		TotalAmount:       resp.Data.Amount,
		PaymentMethodCode: req.PaymentMethod,
		PaymentMethod:     req.PaymentMethod, // Initial assumption, detailed name comes from callback or channel list
		CheckoutURL:       resp.Data.CheckoutURL,
		Status:            models.TransactionStatusUnpaid,
		// Store estimated fees initially
		FeeCustomer: int64(math.Ceil(totalFee)),
		TotalFee:    int64(math.Ceil(totalFee)),
	}

	if err := h.DB.Create(&trx).Error; err != nil {
		log.Printf("Failed to save transaction: %v", err)
		// Don't fail the request, just log. The user can still pay via Tripay link.
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"checkout_url": resp.Data.CheckoutURL,
			"reference":    resp.Data.Reference,
		},
	})
}

// Callback payload from Tripay (re-type definition if needed or keep existing)
type TripayCallbackPayload struct {
	Reference         string `json:"reference"`
	MerchantRef       string `json:"merchant_ref"`
	PaymentMethod     string `json:"payment_method"`
	PaymentMethodCode string `json:"payment_method_code"`
	TotalAmount       int64  `json:"total_amount"`
	FeeMerchant       int64  `json:"fee_merchant"`
	FeeCustomer       int64  `json:"fee_customer"`
	TotalFee          int64  `json:"total_fee"`
	AmountReceived    int64  `json:"amount_received"`
	IsClosedPayment   int    `json:"is_closed_payment"`
	Status            string `json:"status"` // PAID, EXPIRED, FAILED, REFUND
	PaidAt            int64  `json:"paid_at"`
	Note              string `json:"note"`
}

func (h *PaymentHandler) HandleCallback(c *fiber.Ctx) error {
	// 1. Get Signature from Header
	signature := c.Get("X-Callback-Signature")
	if signature == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Missing signature"})
	}

	// 2. Validate Signature
	body := c.Body()
	if !h.TripayService.ValidateSignature(signature, string(body)) {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid signature"})
	}

	// 3. Parse Payload
	var payload TripayCallbackPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid payload"})
	}

	// 4. Update Transaction Record
	var trx models.Transaction
	if err := h.DB.Where("reference = ?", payload.Reference).First(&trx).Error; err != nil {
		log.Printf("Transaction not found for ref: %s", payload.Reference)
		// Not returning error to Tripay, but log it. We might need to handle 'not found' case properly
		// or maybe recreate it? Unlikely if CreatePayment succeeded.
	} else {
		// Update fields
		trx.Status = models.TransactionStatus(payload.Status) // Assuming status strings match (PAID, FAILED, etc)
		trx.PaymentMethod = payload.PaymentMethod
		trx.PaymentMethodCode = payload.PaymentMethodCode
		trx.TotalAmount = payload.TotalAmount
		trx.FeeMerchant = payload.FeeMerchant
		trx.FeeCustomer = payload.FeeCustomer
		trx.TotalFee = payload.TotalFee
		trx.AmountReceived = payload.AmountReceived
		trx.Note = payload.Note

		if payload.PaidAt > 0 {
			t := time.Unix(payload.PaidAt, 0)
			trx.PaidAt = &t
		}

		h.DB.Save(&trx)
	}

	// 5. Update Offer Status
	if payload.Status == "PAID" {
		// Extract Order Code from MerchantRef "INV-{OrderCode}"
		if len(payload.MerchantRef) < 5 {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid merchant ref"})
		}
		orderCode := payload.MerchantRef[4:]

		var offer models.JobOffer
		if err := h.DB.Where("order_code = ?", orderCode).First(&offer).Error; err != nil {
			log.Printf("Offer not found for callback ref: %s", payload.MerchantRef)
			return c.JSON(fiber.Map{"success": false, "message": "Offer not found, but ignored"})
		}

		// Update Status
		if offer.Status == models.OfferStatusPending {
			offer.Status = models.OfferStatusPaid
			h.DB.Save(&offer)

			// Broadcast update
			h.DB.Preload("Freelancer").Preload("Freelancer.FreelancerProfile").
				Preload("Client").Preload("Product").
				First(&offer, "id = ?", offer.ID)

			h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
				"type":  "offer_status_update",
				"offer": toJobOfferResponse(&offer),
			})

			// Create System Message
			sysMsg := models.Message{
				ConversationID: offer.ConversationID,
				SenderID:       offer.ClientID, // Sender is Client (payer), but type is system
				Type:           "system",
				Text:           "Pemberi Kerja telah melakukan pembayaran ke Platform. Freelancer sekarang dapat mulai bekerja.",
				CreatedAt:      time.Now(),
			}

			if err := h.DB.Create(&sysMsg).Error; err == nil {
				// Broadcast new message
				h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
					"type":    "new_message",
					"message": sysMsg,
				})
			}
		}
	}

	return c.JSON(fiber.Map{"success": true})
}
