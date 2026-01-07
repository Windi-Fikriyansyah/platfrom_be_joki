package handlers

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FreelancerDashboardHandler struct {
	DB *gorm.DB
}

func NewFreelancerDashboardHandler(db *gorm.DB) *FreelancerDashboardHandler {
	return &FreelancerDashboardHandler{DB: db}
}

func (h *FreelancerDashboardHandler) Routes(r fiber.Router, authMiddleware fiber.Handler) {
	g := r.Group("/freelancer", authMiddleware)
	g.Get("/dashboard/stats", h.GetDashboardStats)
	g.Get("/orders", h.GetOrders)
	g.Get("/earnings", h.GetEarnings)
}

// GetDashboardStats returns summary for the dashboard
func (h *FreelancerDashboardHandler) GetDashboardStats(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	// 1. Active Orders (Pending, Paid, Working, Delivered)
	var activeOrders int64
	if err := h.DB.Model(&models.JobOffer{}).
		Where("freelancer_id = ?", userID).
		Where("status IN ?", []models.JobOfferStatus{
			models.OfferStatusPending,
			models.OfferStatusPaid,
			models.OfferStatusWorking,
			models.OfferStatusDelivered,
		}).
		Count(&activeOrders).Error; err != nil {
		log.Printf("[DashboardStats] Error counting active orders for user %v: %v", userID, err)
	}

	log.Printf("[DashboardStats] UserID: %s | ActiveOrders: %d", userID, activeOrders)

	// 2. Unread Chats
	// Count messages where I am the freelancer in the conversation,
	// sender is NOT me, and is_read is false
	var unreadChats int64
	// Using a join is safer
	h.DB.Table("messages").
		Joins("JOIN conversations ON messages.conversation_id = conversations.id").
		Where("conversations.freelancer_id = ?", userID).
		Where("messages.sender_id != ?", userID).
		Where("messages.is_read = ?", false).
		Count(&unreadChats)

	// 3. Earnings (Sum of Credit transactions)
	var totalEarnings int64
	h.DB.Model(&models.WalletTransaction{}).
		Where("user_id = ?", userID).
		Where("type = ?", models.WalletTrxCredit).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalEarnings)

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"active_orders":  activeOrders,
			"unread_chats":   unreadChats,
			"total_earnings": totalEarnings,
		},
	})
}

// GetOrders returns list of orders for this freelancer
func (h *FreelancerDashboardHandler) GetOrders(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var offers []models.JobOffer
	var total int64

	q := h.DB.Model(&models.JobOffer{}).
		Preload("Client").
		Preload("Product").
		Where("freelancer_id = ?", userID)

	// Filter by status if needed
	status := c.Query("status")
	if status != "" {
		q = q.Where("status = ?", status)
	}

	q.Count(&total)

	log.Printf("[GetOrders] UserID: %s | Total: %d | StatusFilter: %s", userID, total, status)

	q.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&offers)

	// Transform response
	data := make([]fiber.Map, 0, len(offers))
	for _, o := range offers {
		clientName := "Client"
		clientPhoto := ""
		if o.Client != nil {
			clientName = o.Client.Name
			// User struct doesn't have PhotoURL yet, so we leave it empty
			clientPhoto = ""
		}

		productTitle := "Custom Order"
		if o.Product != nil {
			productTitle = o.Product.Title
		}

		data = append(data, fiber.Map{
			"id":            o.ID,
			"order_code":    o.OrderCode,
			"title":         o.Title,
			"price":         o.Price,
			"net_amount":    o.NetAmount,
			"status":        o.Status,
			"created_at":    o.CreatedAt,
			"delivery_date": o.DeliveryDate,
			"client": fiber.Map{
				"name":      clientName,
				"photo_url": clientPhoto,
			},
			"product_title": productTitle,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
		"meta": fiber.Map{
			"page":        page,
			"limit":       limit,
			"total_items": total,
			"total_pages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

// GetEarnings returns earnings history
func (h *FreelancerDashboardHandler) GetEarnings(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	var creditTotal int64
	h.DB.Model(&models.WalletTransaction{}).
		Where("user_id = ? AND type = ?", userID, "credit").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&creditTotal)

	var debitTotal int64
	h.DB.Model(&models.WalletTransaction{}).
		Where("user_id = ? AND type = ?", userID, "debit").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&debitTotal)

	totalEarnings := creditTotal - debitTotal

	var history []models.WalletTransaction
	if err := h.DB.Where("user_id = ?", userID).Order("created_at desc").Limit(50).Find(&history).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to fetch earnings history",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"total_earnings": totalEarnings,
			"history":        history,
		},
	})
}

// GetProfile returns the full freelancer profile
func (h *FreelancerDashboardHandler) GetProfile(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	var p models.FreelancerProfile
	if err := h.DB.Where("user_id = ?", userID).First(&p).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Profile not found",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    p,
	})
}

type UpdateSettingsRequest struct {
	SystemName     string `json:"system_name"`
	FreelancerType string `json:"freelancer_type"`
	About          string `json:"about"`
	ContactPhone   string `json:"contact_phone"`
	CurrentAddress string `json:"current_address"`
}

// UpdateSettings updates the editable fields of the profile
func (h *FreelancerDashboardHandler) UpdateSettings(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	var req UpdateSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
		})
	}

	var p models.FreelancerProfile
	if err := h.DB.Where("user_id = ?", userID).First(&p).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Profile not found",
		})
	}

	if req.SystemName != "" {
		p.SystemName = req.SystemName
	}
	if req.FreelancerType != "" {
		p.FreelancerType = models.FreelancerType(req.FreelancerType)
	}
	if req.About != "" {
		p.About = req.About
	}
	if req.ContactPhone != "" {
		p.ContactPhone = req.ContactPhone
	}
	if req.CurrentAddress != "" {
		p.CurrentAddress = req.CurrentAddress
	}
	p.UpdatedAt = time.Now()

	if err := h.DB.Save(&p).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to update profile",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Profile updated successfully",
		"data":    p,
	})
}

// UpdatePhoto handles profile photo update
func (h *FreelancerDashboardHandler) UpdatePhoto(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	file, err := c.FormFile("photo")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Photo is required",
		})
	}

	// Basic validation
	if file.Size > 2*1024*1024 { // 2MB
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "File too large (max 2MB)",
		})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid file format (jpg/png only)",
		})
	}

	// Create upload dir if not exists
	uploadDir := "uploads/freelancers/" + userID.String()
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to create upload directory",
		})
	}

	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	path := filepath.Join(uploadDir, filename)

	if err := c.SaveFile(file, path); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to save file",
		})
	}

	// Public URL (assuming static file serving matches this structure)
	// Typically: /uploads/freelancers/{uuid}/{filename}
	// Note: h.PublicBaseURL should be injected but we can infer or use relative
	publicURL := fmt.Sprintf("/uploads/freelancers/%s/%s", userID.String(), filename)

	var p models.FreelancerProfile
	if err := h.DB.Where("user_id = ?", userID).First(&p).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Profile not found",
		})
	}

	p.PhotoURL = publicURL
	p.UpdatedAt = time.Now()

	if err := h.DB.Save(&p).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to update profile photo",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Photo updated successfully",
		"data":    p,
	})
}
