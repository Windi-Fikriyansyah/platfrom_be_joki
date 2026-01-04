package handlers

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/realtime"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/services/wallet"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type JobOfferHandler struct {
	DB            *gorm.DB
	Hub           *realtime.Hub
	RDB           *redis.Client
	WalletService *wallet.WalletService
}

func NewJobOfferHandler(db *gorm.DB, hub *realtime.Hub, rdb *redis.Client, walletService *wallet.WalletService) *JobOfferHandler {
	return &JobOfferHandler{DB: db, Hub: hub, RDB: rdb, WalletService: walletService}
}

// CreateOfferRequest is the request body for creating a job offer
type CreateOfferRequest struct {
	ProductID      *uint  `json:"product_id"`
	Price          int64  `json:"price"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	RevisionCount  int    `json:"revision_count"`
	StartDate      string `json:"start_date"`      // ISO format: 2026-01-03
	DeliveryDate   string `json:"delivery_date"`   // ISO format: 2026-01-05
	DeliveryFormat string `json:"delivery_format"` // e.g., ".pdf, .png"
	Notes          string `json:"notes"`
}

// JobOfferResponse is the response DTO for job offer
type JobOfferResponse struct {
	ID             string `json:"id"`
	OrderCode      string `json:"order_code"`
	ConversationID string `json:"conversation_id"`
	FreelancerID   string `json:"freelancer_id"`
	ClientID       string `json:"client_id"`
	ProductID      *uint  `json:"product_id,omitempty"`

	Price       int64 `json:"price"`
	PlatformFee int64 `json:"platform_fee"`
	NetAmount   int64 `json:"net_amount"`

	Title         string `json:"title"`
	Description   string `json:"description"`
	RevisionCount int    `json:"revision_count"`

	StartDate      string `json:"start_date"`
	DeliveryDate   string `json:"delivery_date"`
	DeliveryFormat string `json:"delivery_format"`
	Notes          string `json:"notes"`

	WorkDeliveryLink  string `json:"work_delivery_link"`
	WorkDeliveryFiles string `json:"work_delivery_files"`
	UsedRevisionCount int    `json:"used_revision_count"`

	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`

	// Optional embedded data
	Product    *ProductMini `json:"product,omitempty"`
	Freelancer *UserMini    `json:"freelancer,omitempty"`
	Client     *UserMini    `json:"client,omitempty"`
}

type ProductMini struct {
	ID       uint   `json:"id"`
	Title    string `json:"title"`
	CoverURL string `json:"cover_url,omitempty"`
}

func toJobOfferResponse(offer *models.JobOffer) JobOfferResponse {
	resp := JobOfferResponse{
		ID:                offer.ID.String(),
		OrderCode:         offer.OrderCode,
		ConversationID:    offer.ConversationID.String(),
		FreelancerID:      offer.FreelancerID.String(),
		ClientID:          offer.ClientID.String(),
		ProductID:         offer.ProductID,
		Price:             offer.Price,
		PlatformFee:       offer.PlatformFee,
		NetAmount:         offer.NetAmount,
		Title:             offer.Title,
		Description:       offer.Description,
		RevisionCount:     offer.RevisionCount,
		StartDate:         offer.StartDate.Format("2006-01-02"),
		DeliveryDate:      offer.DeliveryDate.Format("2006-01-02"),
		DeliveryFormat:    offer.DeliveryFormat,
		Notes:             offer.Notes,
		WorkDeliveryLink:  offer.WorkDeliveryLink,
		WorkDeliveryFiles: offer.WorkDeliveryFiles,
		UsedRevisionCount: offer.UsedRevisionCount,
		Status:            string(offer.Status),
		CreatedAt:         offer.CreatedAt,
	}

	if offer.Product != nil {
		resp.Product = &ProductMini{
			ID:       offer.Product.ID,
			Title:    offer.Product.Title,
			CoverURL: offer.Product.CoverURL,
		}
	}

	if offer.Freelancer != nil {
		resp.Freelancer = &UserMini{
			ID:   offer.Freelancer.ID.String(),
			Name: offer.Freelancer.Name,
		}
		if offer.Freelancer.FreelancerProfile != nil {
			resp.Freelancer.FreelancerProfile = &struct {
				SystemName string `json:"system_name,omitempty"`
				PhotoURL   string `json:"photo_url,omitempty"`
			}{
				SystemName: offer.Freelancer.FreelancerProfile.SystemName,
				PhotoURL:   offer.Freelancer.FreelancerProfile.PhotoURL,
			}
		}
	}

	if offer.Client != nil {
		resp.Client = &UserMini{
			ID:   offer.Client.ID.String(),
			Name: offer.Client.Name,
		}
	}

	return resp
}

// CreateOffer creates a new job offer for a conversation
func (h *JobOfferHandler) CreateOffer(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized",
		})
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID format",
		})
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID",
		})
	}

	convID := c.Params("id")
	convUUID, err := uuid.Parse(convID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid conversation ID",
		})
	}

	// Parse request body
	var req CreateOfferRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Price <= 0 {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Price is required and must be positive",
		})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Title is required",
		})
	}

	// Verify conversation exists and user is the freelancer
	var conv models.Conversation
	if err := h.DB.First(&conv, "id = ?", convUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Conversation not found",
		})
	}

	// User must be the freelancer in this conversation
	if conv.FreelancerID != userUUID {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"message": "Only freelancer can create job offers",
		})
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		startDate = time.Now()
	}

	deliveryDate, err := time.Parse("2006-01-02", req.DeliveryDate)
	if err != nil {
		deliveryDate = time.Now().AddDate(0, 0, 7) // Default 7 days
	}

	// Calculate platform fee (10%)
	platformFee := req.Price / 10
	netAmount := req.Price - platformFee

	// Generate unique order code
	var orderCode string
	for {
		orderCode = models.GenerateOrderCode()
		var existing models.JobOffer
		if h.DB.Where("order_code = ?", orderCode).First(&existing).Error == gorm.ErrRecordNotFound {
			break
		}
	}

	// Create job offer
	offer := models.JobOffer{
		OrderCode:      orderCode,
		ConversationID: convUUID,
		FreelancerID:   userUUID,
		ClientID:       conv.ClientID,
		ProductID:      req.ProductID,
		Price:          req.Price,
		PlatformFee:    platformFee,
		NetAmount:      netAmount,
		Title:          req.Title,
		Description:    req.Description,
		RevisionCount:  req.RevisionCount,
		StartDate:      startDate,
		DeliveryDate:   deliveryDate,
		DeliveryFormat: req.DeliveryFormat,
		Notes:          req.Notes,
		Status:         models.OfferStatusPending,
	}

	if err := h.DB.Create(&offer).Error; err != nil {
		log.Println("Error creating job offer:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to create job offer",
		})
	}

	// Load relations for response
	h.DB.Preload("Freelancer").Preload("Freelancer.FreelancerProfile").
		Preload("Client").Preload("Product").
		First(&offer, "id = ?", offer.ID)

	// Create a system message in the conversation
	msg := models.Message{
		ConversationID: convUUID,
		SenderID:       userUUID,
		Text:           "[OFFER]" + offer.ID.String(), // Special marker for offer messages
		IsRead:         false,
	}

	if err := h.DB.Create(&msg).Error; err != nil {
		log.Println("Error creating offer message:", err)
	}

	// Update conversation last message time
	h.DB.Model(&models.Conversation{}).
		Where("id = ?", conv.ID).
		Update("last_message_at", msg.CreatedAt)

	// Broadcast via WebSocket
	msgResp := MessageResponse{
		ID:             msg.ID.String(),
		ConversationID: msg.ConversationID.String(),
		SenderID:       msg.SenderID.String(),
		Text:           msg.Text,
		IsRead:         msg.IsRead,
		CreatedAt:      msg.CreatedAt,
	}

	h.Hub.SendToConversation(conv.ClientID, conv.FreelancerID, fiber.Map{
		"type":    "new_message",
		"message": msgResp,
		"offer":   toJobOfferResponse(&offer),
	})

	return c.Status(201).JSON(fiber.Map{
		"success": true,
		"data":    toJobOfferResponse(&offer),
	})
}

// GetOffers returns all job offers for a conversation
func (h *JobOfferHandler) GetOffers(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized",
		})
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID format",
		})
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID",
		})
	}

	convID := c.Params("id")
	convUUID, err := uuid.Parse(convID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid conversation ID",
		})
	}

	// Verify user is part of conversation
	var conv models.Conversation
	if err := h.DB.First(&conv, "id = ?", convUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Conversation not found",
		})
	}

	if conv.ClientID != userUUID && conv.FreelancerID != userUUID {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"message": "Access denied",
		})
	}

	// Get offers
	var offers []models.JobOffer
	if err := h.DB.
		Preload("Freelancer").
		Preload("Freelancer.FreelancerProfile").
		Preload("Client").
		Preload("Product").
		Where("conversation_id = ?", convUUID).
		Order("created_at DESC").
		Find(&offers).Error; err != nil {
		log.Println("Error fetching job offers:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to fetch job offers",
		})
	}

	// Convert to response
	out := make([]JobOfferResponse, 0, len(offers))
	for _, offer := range offers {
		out = append(out, toJobOfferResponse(&offer))
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    out,
	})
}

// GetOffer returns a single job offer by ID
func (h *JobOfferHandler) GetOffer(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized",
		})
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID format",
		})
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID",
		})
	}

	offerID := c.Params("id")
	offerUUID, err := uuid.Parse(offerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid offer ID",
		})
	}

	var offer models.JobOffer
	if err := h.DB.
		Preload("Freelancer").
		Preload("Freelancer.FreelancerProfile").
		Preload("Client").
		Preload("Product").
		First(&offer, "id = ?", offerUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Job offer not found",
		})
	}

	// Verify user is part of this offer
	if offer.ClientID != userUUID && offer.FreelancerID != userUUID {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"message": "Access denied",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    toJobOfferResponse(&offer),
	})
}

// UpdateOfferStatusRequest is the request body for updating offer status
type UpdateOfferStatusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus updates the status of a job offer
func (h *JobOfferHandler) UpdateStatus(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized",
		})
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID format",
		})
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID",
		})
	}

	offerID := c.Params("id")
	offerUUID, err := uuid.Parse(offerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid offer ID",
		})
	}

	var req UpdateOfferStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
		})
	}

	// Validate status
	validStatuses := map[string]bool{
		"pending":   true,
		"paid":      true,
		"working":   true,
		"delivered": true,
		"completed": true,
		"cancelled": true,
	}

	if !validStatuses[req.Status] {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid status",
		})
	}

	var offer models.JobOffer
	if err := h.DB.First(&offer, "id = ?", offerUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Job offer not found",
		})
	}

	// Verify user is part of this offer
	if offer.ClientID != userUUID && offer.FreelancerID != userUUID {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"message": "Access denied",
		})
	}

	// Update status
	offer.Status = models.JobOfferStatus(req.Status)
	if err := h.DB.Save(&offer).Error; err != nil {
		log.Println("Error updating job offer status:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to update status",
		})
	}

	// Reload with relations
	h.DB.Preload("Freelancer").Preload("Freelancer.FreelancerProfile").
		Preload("Client").Preload("Product").
		First(&offer, "id = ?", offer.ID)

	// Broadcast status change via WebSocket
	h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
		"type":  "offer_status_update",
		"offer": toJobOfferResponse(&offer),
	})

	return c.JSON(fiber.Map{
		"success": true,
		"data":    toJobOfferResponse(&offer),
	})
}

// UpdateOffer updates an existing job offer
func (h *JobOfferHandler) UpdateOffer(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized",
		})
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID format",
		})
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid user ID",
		})
	}

	offerID := c.Params("id")
	offerUUID, err := uuid.Parse(offerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid offer ID",
		})
	}

	// Parse request body
	var req CreateOfferRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Price <= 0 {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Price is required and must be positive",
		})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Title is required",
		})
	}

	var offer models.JobOffer
	if err := h.DB.First(&offer, "id = ?", offerUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Job offer not found",
		})
	}

	// Verify user is owner (freelancer)
	if offer.FreelancerID != userUUID {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"message": "Access denied",
		})
	}

	// Verify status is pending
	if offer.Status != models.OfferStatusPending {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Cannot update offer that is not pending",
		})
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		startDate = time.Now()
	}

	deliveryDate, err := time.Parse("2006-01-02", req.DeliveryDate)
	if err != nil {
		deliveryDate = time.Now().AddDate(0, 0, 7)
	}

	// Recalculate fees
	platformFee := req.Price / 10
	netAmount := req.Price - platformFee

	// Update fields
	offer.Price = req.Price
	offer.PlatformFee = platformFee
	offer.NetAmount = netAmount
	offer.Title = req.Title
	offer.Description = req.Description
	offer.RevisionCount = req.RevisionCount
	offer.StartDate = startDate
	offer.DeliveryDate = deliveryDate
	offer.DeliveryFormat = req.DeliveryFormat
	offer.Notes = req.Notes
	offer.ProductID = req.ProductID

	if err := h.DB.Save(&offer).Error; err != nil {
		log.Println("Error updating job offer:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to update job offer",
		})
	}

	// Load relations
	h.DB.Preload("Freelancer").Preload("Freelancer.FreelancerProfile").
		Preload("Client").Preload("Product").
		First(&offer, "id = ?", offer.ID)

	// Broadcast update
	h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
		"type":  "offer_status_update", // Client treats this as a generic update
		"offer": toJobOfferResponse(&offer),
	})

	return c.JSON(fiber.Map{
		"success": true,
		"data":    toJobOfferResponse(&offer),
	})
}

// DeliverWork handles work submission by freelancer
func (h *JobOfferHandler) DeliverWork(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}

	userUUID, _ := uuid.Parse(userID.(string))
	offerID := c.Params("id")
	offerUUID, err := uuid.Parse(offerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid offer ID"})
	}

	var offer models.JobOffer
	if err := h.DB.First(&offer, "id = ?", offerUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "Offer not found"})
	}

	if offer.FreelancerID != userUUID {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "Only the assigned freelancer can deliver work"})
	}

	// Allow delivery if paid, working, or already delivered (for updates)
	isUpdate := offer.Status == models.OfferStatusDelivered
	if !isUpdate && offer.Status != models.OfferStatusPaid && offer.Status != models.OfferStatusWorking {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Order must be in 'Paid', 'Working', or 'Delivered' status to deliver"})
	}

	workURL := c.FormValue("work_url")

	// Handle Multiple File Uploads
	form, err := c.MultipartForm()
	var filePaths []string
	if err != nil {
		log.Println("Error parsing multipart form:", err)
	} else {
		files := form.File["files"]
		for _, file := range files {
			// Limit 25MB check (Fiber usually has a global limit, but we can check individually)
			if file.Size > 25*1024*1024 {
				return c.Status(400).JSON(fiber.Map{"success": false, "message": "File " + file.Filename + " exceeds 25MB limit"})
			}

			// Save file
			ext := filepath.Ext(file.Filename)
			filename := uuid.New().String() + ext
			uploadDir := "./uploads/deliveries"
			os.MkdirAll(uploadDir, 0755)

			savePath := filepath.Join(uploadDir, filename)
			if err := c.SaveFile(file, savePath); err != nil {
				log.Println("Error saving delivery file:", err)
				continue
			}

			publicPath := "/uploads/deliveries/" + filename
			base := os.Getenv("APP_BASE_URL")
			if base != "" {
				publicPath = strings.TrimRight(base, "/") + publicPath
			}
			filePaths = append(filePaths, publicPath)
		}
	}

	filesJSON, _ := json.Marshal(filePaths)

	// Update Offer
	offer.Status = models.OfferStatusDelivered
	offer.WorkDeliveryLink = workURL
	offer.WorkDeliveryFiles = string(filesJSON)
	offer.UpdatedAt = time.Now()

	if err := h.DB.Save(&offer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update offer status"})
	}

	// Create Delivery System Message
	msgText := "Freelancer mengirimkan pekerjaan untuk ditinjau dan disetujui.\n\nPembeli dapat meminta revisi dalam kurun waktu 7 hari. Jika tidak ada respon dalam jangka waktu yang ditentukan, sistem akan secara otomatis menyetujui pekerjaan untuk freelancer."
	if isUpdate {
		msgText = "Freelancer telah memperbarui hasil pekerjaan.\n\nSilakan tinjau kembali hasil pekerjaan terbaru yang telah dikirimkan."
	}

	msg := models.Message{
		ID:             uuid.New(),
		ConversationID: offer.ConversationID,
		SenderID:       userUUID,
		Text:           msgText,
		Type:           "delivery", // Special type for custom rendering
		IsRead:         false,
	}
	h.DB.Create(&msg)

	// Broadcast Updates
	h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
		"type": "new_message",
		"message": fiber.Map{
			"id":              msg.ID.String(),
			"conversation_id": msg.ConversationID.String(),
			"sender_id":       msg.SenderID.String(),
			"text":            msg.Text,
			"type":            msg.Type,
			"created_at":      msg.CreatedAt,
		},
		"offer": toJobOfferResponse(&offer),
	})

	h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
		"type":  "offer_status_update",
		"offer": toJobOfferResponse(&offer),
	})

	return c.JSON(fiber.Map{
		"success": true,
		"data":    toJobOfferResponse(&offer),
	})
}

// RequestRevision handles revision request by client
func (h *JobOfferHandler) RequestRevision(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}

	userUUID, _ := uuid.Parse(userID.(string))
	offerID := c.Params("id")
	offerUUID, err := uuid.Parse(offerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid offer ID"})
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil || req.Reason == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Revision reason is required"})
	}

	var offer models.JobOffer
	if err := h.DB.First(&offer, "id = ?", offerUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "Offer not found"})
	}

	if offer.ClientID != userUUID {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "Only the client can request revisions"})
	}

	if offer.Status != models.OfferStatusDelivered {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Revision can only be requested for delivered work"})
	}

	if offer.UsedRevisionCount >= offer.RevisionCount {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Revision limit reached. No more revisions available for this offer."})
	}

	// Update Offer
	offer.Status = models.OfferStatusWorking
	offer.UsedRevisionCount++
	offer.UpdatedAt = time.Now()

	if err := h.DB.Save(&offer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update offer status"})
	}

	// Create Revision Message
	msg := models.Message{
		ID:             uuid.New(),
		ConversationID: offer.ConversationID,
		SenderID:       userUUID,
		Text:           req.Reason,
		Type:           "revision", // Special type for revision info
		IsRead:         false,
	}
	h.DB.Create(&msg)

	// Broadcast Updates
	h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
		"type": "new_message",
		"message": fiber.Map{
			"id":              msg.ID.String(),
			"conversation_id": msg.ConversationID.String(),
			"sender_id":       msg.SenderID.String(),
			"text":            msg.Text,
			"type":            msg.Type,
			"created_at":      msg.CreatedAt,
		},
		"offer": toJobOfferResponse(&offer),
	})

	h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
		"type":  "offer_status_update",
		"offer": toJobOfferResponse(&offer),
	})

	return c.JSON(fiber.Map{"success": true, "data": toJobOfferResponse(&offer)})
}

// CompleteOrder handles order completion by client
func (h *JobOfferHandler) CompleteOrder(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}

	userUUID, _ := uuid.Parse(userID.(string))
	offerID := c.Params("id")
	offerUUID, err := uuid.Parse(offerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid offer ID"})
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		var offer models.JobOffer
		// Lock the offer row for update (Idempotency / Race condition prevention)
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&offer, "id = ?", offerUUID).Error; err != nil {
			return err
		}

		if offer.ClientID != userUUID {
			return fiber.NewError(403, "Only the client can complete the order")
		}

		// IDEMPOTENCY: If already completed, just return
		if offer.Status == models.OfferStatusCompleted {
			log.Printf("Offer %s already completed (Idempotency)", offer.ID)
			return nil
		}

		if offer.Status != models.OfferStatusDelivered {
			return fiber.NewError(400, "Only delivered orders can be completed")
		}

		// 1. Update Offer Status
		offer.Status = models.OfferStatusCompleted
		offer.UpdatedAt = time.Now()
		if err := tx.Save(&offer).Error; err != nil {
			return err
		}

		// 2. ESCROW RELEASE LOGIC via WalletService
		desc := "Pembayaran pesanan #" + offer.OrderCode
		if err := h.WalletService.CreditFreelancer(tx, offer.FreelancerID, offer.NetAmount, offer.ID, desc); err != nil {
			log.Printf("Failed to release escrow for offer %s: %v", offer.ID, err)
			return err
		}

		// 3. Create System Message
		msg := models.Message{
			ID:             uuid.New(),
			ConversationID: offer.ConversationID,
			SenderID:       userUUID,
			Text:           "Pesanan telah diselesaikan oleh pembeli. Dana telah diteruskan ke saldo Freelancer. Terima kasih!",
			Type:           "system",
			IsRead:         false,
			CreatedAt:      time.Now(),
		}
		if err := tx.Create(&msg).Error; err != nil {
			return err
		}

		// Broadcast Updates (We can do this after commit if preferred, but doing inside is common for simple hubs)
		h.Hub.SendToConversation(offer.ClientID, offer.FreelancerID, fiber.Map{
			"type": "new_message",
			"message": fiber.Map{
				"id":              msg.ID.String(),
				"conversation_id": msg.ConversationID.String(),
				"sender_id":       msg.SenderID.String(),
				"text":            msg.Text,
				"type":            msg.Type,
				"created_at":      msg.CreatedAt,
			},
		})

		return nil
	})

	if err != nil {
		if e, ok := err.(*fiber.Error); ok {
			return c.Status(e.Code).JSON(fiber.Map{"success": false, "message": e.Message})
		}
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to complete order: " + err.Error()})
	}

	// Reload to get updated status and potentially other info for response
	var finalOffer models.JobOffer
	h.DB.Preload("Freelancer").Preload("Freelancer.FreelancerProfile").
		Preload("Client").Preload("Product").
		First(&finalOffer, "id = ?", offerUUID)

	h.Hub.SendToConversation(finalOffer.ClientID, finalOffer.FreelancerID, fiber.Map{
		"type":  "offer_status_update",
		"offer": toJobOfferResponse(&finalOffer),
	})

	return c.JSON(fiber.Map{"success": true, "data": toJobOfferResponse(&finalOffer)})
}

// StartAutoCompletionWorker runs a background job to complete orders after 3 days of delivery
func (h *JobOfferHandler) StartAutoCompletionWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			log.Println("[AutoCompletionWorker] Scanning for delivered orders to auto-complete...")
			h.scanAndCompleteOrders()
		}
	}()
}

func (h *JobOfferHandler) scanAndCompleteOrders() {
	var deliveredOffers []models.JobOffer
	threeDaysAgo := time.Now().Add(-72 * time.Hour)

	// Cari offer status 'delivered' yang updatedAt (saat dikirim) <= 3 hari yang lalu
	err := h.DB.Where("status = ? AND updated_at <= ?", models.OfferStatusDelivered, threeDaysAgo).Find(&deliveredOffers).Error
	if err != nil {
		log.Printf("[AutoCompletionWorker] Error fetching delivered offers: %v", err)
		return
	}

	for _, offer := range deliveredOffers {
		log.Printf("[AutoCompletionWorker] Auto-completing Offer %s (Order: %s)", offer.ID, offer.OrderCode)

		h.DB.Transaction(func(tx *gorm.DB) error {
			// Lock row
			var currentOffer models.JobOffer
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&currentOffer, "id = ?", offer.ID).Error; err != nil {
				return err
			}

			// Idempotency
			if currentOffer.Status != models.OfferStatusDelivered {
				return nil
			}

			// 1. Update status
			currentOffer.Status = models.OfferStatusCompleted
			currentOffer.UpdatedAt = time.Now()
			if err := tx.Save(&currentOffer).Error; err != nil {
				return err
			}

			// 2. Release Escrow via WalletService
			desc := "Penyelesaian otomatis pesanan #" + currentOffer.OrderCode + " (tanpa respon dari pembeli)"
			if err := h.WalletService.CreditFreelancer(tx, currentOffer.FreelancerID, currentOffer.NetAmount, currentOffer.ID, desc); err != nil {
				return err
			}

			// 3. Create System Message
			msg := models.Message{
				ID:             uuid.New(),
				ConversationID: currentOffer.ConversationID,
				SenderID:       currentOffer.FreelancerID,
				Type:           "system",
				Text:           "Pesanan telah diselesaikan secara otomatis oleh sistem (3 hari setelah pengiriman tanpa respon). Dana diteruskan ke Freelancer.",
				CreatedAt:      time.Now(),
			}
			if err := tx.Create(&msg).Error; err != nil {
				return err
			}

			// 4. Broadcast
			h.Hub.SendToConversation(currentOffer.ClientID, currentOffer.FreelancerID, fiber.Map{
				"type": "new_message",
				"message": fiber.Map{
					"id":              msg.ID.String(),
					"conversation_id": msg.ConversationID.String(),
					"sender_id":       msg.SenderID.String(),
					"text":            msg.Text,
					"type":            msg.Type,
					"created_at":      msg.CreatedAt,
				},
			})

			h.Hub.SendToConversation(currentOffer.ClientID, currentOffer.FreelancerID, fiber.Map{
				"type":  "offer_status_update",
				"offer": toJobOfferResponse(&currentOffer),
			})

			return nil
		})
	}
}

// CancelOrder handles order cancellation by freelancer
func (h *JobOfferHandler) CancelOrder(c *fiber.Ctx) error {
	userID := c.Locals("userId")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}

	userUUID, _ := uuid.Parse(userID.(string))
	offerID := c.Params("id")
	offerUUID, err := uuid.Parse(offerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid offer ID"})
	}

	var offer models.JobOffer
	if err := h.DB.First(&offer, "id = ?", offerUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "Offer not found"})
	}

	// Only freelancer can cancel (as requested)
	if offer.FreelancerID != userUUID {
		return c.Status(403).JSON(fiber.Map{"success": false, "message": "Only the freelancer can cancel this order"})
	}

	// Allow cancelling pending OR paid orders
	if offer.Status != models.OfferStatusPending && offer.Status != models.OfferStatusPaid {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Hanya pesanan pending atau berstatus paid yang dapat dibatalkan"})
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// Lock row
		var currentOffer models.JobOffer
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&currentOffer, "id = ?", offer.ID).Error; err != nil {
			return err
		}

		// Idempotency
		if currentOffer.Status == models.OfferStatusCancelled {
			return nil
		}

		// 1. Refund logic if already PAID
		if currentOffer.Status == models.OfferStatusPaid {
			desc := "Pengembalian dana untuk pembatalan pesanan #" + currentOffer.OrderCode
			// Refund to Client's platform balance
			if err := h.WalletService.CreditClient(tx, currentOffer.ClientID, currentOffer.Price, currentOffer.ID, desc); err != nil {
				return err
			}
		}

		// 2. Update status
		currentOffer.Status = models.OfferStatusCancelled
		currentOffer.UpdatedAt = time.Now()
		if err := tx.Save(&currentOffer).Error; err != nil {
			return err
		}

		// 3. Create System Message
		cancelMsg := "Pesanan #" + currentOffer.OrderCode + " telah dibatalkan oleh freelancer."
		if currentOffer.Status == models.OfferStatusCancelled && offer.Status == models.OfferStatusPaid {
			cancelMsg += " Dana telah dikembalikan ke saldo Jokiin Anda."
		}

		msg := models.Message{
			ID:             uuid.New(),
			ConversationID: currentOffer.ConversationID,
			SenderID:       userUUID,
			Type:           "system",
			Text:           cancelMsg,
			CreatedAt:      time.Now(),
		}
		if err := tx.Create(&msg).Error; err != nil {
			return err
		}

		// 4. Broadcast
		h.Hub.SendToConversation(currentOffer.ClientID, currentOffer.FreelancerID, fiber.Map{
			"type": "new_message",
			"message": fiber.Map{
				"id":              msg.ID.String(),
				"conversation_id": msg.ConversationID.String(),
				"sender_id":       msg.SenderID.String(),
				"text":            msg.Text,
				"type":            msg.Type,
				"created_at":      msg.CreatedAt,
			},
		})

		h.Hub.SendToConversation(currentOffer.ClientID, currentOffer.FreelancerID, fiber.Map{
			"type":  "offer_status_update",
			"offer": toJobOfferResponse(&currentOffer),
		})

		return nil
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to cancel order"})
	}

	return c.JSON(fiber.Map{"success": true, "data": toJobOfferResponse(&offer)})
}
