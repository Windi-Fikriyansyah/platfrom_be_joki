package handlers

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/realtime"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type ChatHandler struct {
	DB  *gorm.DB
	Hub *realtime.Hub
	RDB *redis.Client
}

func NewChatHandler(db *gorm.DB, hub *realtime.Hub, rdb *redis.Client) *ChatHandler {
	return &ChatHandler{DB: db, Hub: hub, RDB: rdb}
}

// CreateOrGetConversation creates a new conversation or returns existing one
func (h *ChatHandler) CreateOrGetConversation(c *fiber.Ctx) error {
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

	var req struct {
		SellerID  *string `json:"seller_id"`
		ProductID *uint   `json:"product_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request",
		})
	}

	// Get current user
	var user models.User
	if err := h.DB.First(&user, "id = ?", userUUID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "User not found",
		})
	}

	var freelancerID uuid.UUID
	var clientID uuid.UUID

	// Determine roles
	if req.SellerID != nil {
		sellerUUID, err := uuid.Parse(*req.SellerID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"success": false,
				"message": "Invalid seller ID",
			})
		}
		freelancerID = sellerUUID
		clientID = userUUID
	} else if req.ProductID != nil {
		// Get product owner
		var product models.Product
		if err := h.DB.First(&product, "id = ?", req.ProductID).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{
				"success": false,
				"message": "Product not found",
			})
		}
		freelancerID = product.UserID
		clientID = userUUID
	} else {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "seller_id or product_id required",
		})
	}

	// Check if conversation exists
	var conv models.Conversation
	err = h.DB.
		Where("client_id = ? AND freelancer_id = ?", clientID, freelancerID).
		Order("updated_at DESC").
		First(&conv).Error

	created := false
	if err == gorm.ErrRecordNotFound {
		// Create new conversation
		conv = models.Conversation{
			ClientID:      clientID,
			FreelancerID:  freelancerID,
			ProductID:     req.ProductID,
			LastMessageAt: time.Now(),
		}
		if err := h.DB.Create(&conv).Error; err != nil {
			log.Println("Error creating conversation:", err)
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Failed to create conversation",
			})
		}
		created = true
	} else if err != nil && err != gorm.ErrRecordNotFound {
		log.Println("Error fetching conversation:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to fetch conversation",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"created": created,
		"data":    conv,
	})
}

type UserMini struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	FreelancerProfile *struct {
		SystemName string `json:"system_name,omitempty"`
		PhotoURL   string `json:"photo_url,omitempty"`
	} `json:"freelancer_profile,omitempty"`
}

type MessageMini struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	Type           string    `json:"type"`
	Text           string    `json:"text"`
	IsRead         bool      `json:"is_read"`
	CreatedAt      time.Time `json:"created_at"`
}

type ConversationOut struct {
	ID          string    `json:"id"`
	BuyerID     string    `json:"buyer_id"`
	SellerID    string    `json:"seller_id"`
	ProductID   *uint     `json:"product_id,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	UnreadCount int64     `json:"unread_count"`

	Buyer             *UserMini    `json:"buyer,omitempty"`
	Seller            *UserMini    `json:"seller,omitempty"`
	LastMessage       *MessageMini `json:"last_message,omitempty"`
	LatestOfferStatus *string      `json:"latest_offer_status,omitempty"`
}

// GetConversations returns user's conversations
func (h *ChatHandler) GetConversations(c *fiber.Ctx) error {
	userUUID, err := getUserUUID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}

	var convs []models.Conversation
	if err := h.DB.
		Preload("Client").
		Preload("Client.FreelancerProfile").
		Preload("Freelancer").
		Preload("Freelancer.FreelancerProfile").
		Where("client_id = ? OR freelancer_id = ?", userUUID, userUUID).
		Order("last_message_at DESC").
		Find(&convs).Error; err != nil {

		log.Println("Error fetching conversations:", err)
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to fetch conversations"})
	}

	out := make([]ConversationOut, 0, len(convs))

	for _, conv := range convs {
		// unread_count
		var unreadCount int64
		h.DB.Model(&models.Message{}).
			Where("conversation_id = ? AND sender_id != ? AND is_read = false", conv.ID, userUUID).
			Count(&unreadCount)

		// last_message
		var last models.Message
		var lastPtr *MessageMini = nil
		if err := h.DB.
			Where("conversation_id = ?", conv.ID).
			Order("created_at DESC").
			Limit(1).
			First(&last).Error; err == nil {

			lastPtr = &MessageMini{
				ID:             last.ID.String(),
				ConversationID: last.ConversationID.String(),
				SenderID:       last.SenderID.String(),
				Type:           last.Type,
				Text:           last.Text,
				IsRead:         last.IsRead,
				CreatedAt:      last.CreatedAt,
			}
		}

		// map buyer/seller
		var buyerMini *UserMini
		if conv.Client != nil {
			buyerMini = &UserMini{
				ID:   conv.Client.ID.String(),
				Name: conv.Client.Name,
			}
			if conv.Client.FreelancerProfile != nil {
				buyerMini.FreelancerProfile = &struct {
					SystemName string `json:"system_name,omitempty"`
					PhotoURL   string `json:"photo_url,omitempty"`
				}{
					SystemName: conv.Client.FreelancerProfile.SystemName,
					PhotoURL:   conv.Client.FreelancerProfile.PhotoURL,
				}
			}
		}

		var sellerMini *UserMini
		if conv.Freelancer != nil {
			sellerMini = &UserMini{
				ID:   conv.Freelancer.ID.String(),
				Name: conv.Freelancer.Name,
			}
			if conv.Freelancer.FreelancerProfile != nil {
				sellerMini.FreelancerProfile = &struct {
					SystemName string `json:"system_name,omitempty"`
					PhotoURL   string `json:"photo_url,omitempty"`
				}{
					SystemName: conv.Freelancer.FreelancerProfile.SystemName,
					PhotoURL:   conv.Freelancer.FreelancerProfile.PhotoURL,
				}
			}
		}

		// latest_offer_status
		var latestOffer models.JobOffer
		var latestOfferStatus *string = nil
		if err := h.DB.
			Where("conversation_id = ?", conv.ID).
			Order("created_at DESC").
			Limit(1).
			First(&latestOffer).Error; err == nil {
			statusStr := string(latestOffer.Status)
			latestOfferStatus = &statusStr
		}

		out = append(out, ConversationOut{
			ID:                conv.ID.String(),
			BuyerID:           conv.ClientID.String(),
			SellerID:          conv.FreelancerID.String(),
			ProductID:         conv.ProductID,
			UpdatedAt:         conv.LastMessageAt, // Next kamu pakai updated_at untuk sorting
			UnreadCount:       unreadCount,
			Buyer:             buyerMini,
			Seller:            sellerMini,
			LastMessage:       lastPtr,
			LatestOfferStatus: latestOfferStatus,
		})
	}

	return c.JSON(fiber.Map{"success": true, "data": out})
}

// GetUnreadTotal returns the total count of unread messages across all conversations
func (h *ChatHandler) GetUnreadTotal(c *fiber.Ctx) error {
	userUUID, err := getUserUUID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}

	var count int64
	// Count messages where recipient is current user, and is_read is false
	// We check against all conversations where user is a member
	err = h.DB.Model(&models.Message{}).
		Joins("JOIN conversations ON messages.conversation_id = conversations.id").
		Where("(conversations.client_id = ? OR conversations.freelancer_id = ?) AND messages.sender_id != ? AND messages.is_read = false", userUUID, userUUID, userUUID).
		Count(&count).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to count unread messages"})
	}

	return c.JSON(fiber.Map{"success": true, "data": count})
}

// MessageResponse DTO untuk response message
type MessageResponse struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	Type           string    `json:"type"`
	Text           string    `json:"text"`
	IsRead         bool      `json:"is_read"`
	CreatedAt      time.Time `json:"created_at"`
}

// GetMessages returns messages for a conversation
func (h *ChatHandler) GetMessages(c *fiber.Ctx) error {
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

	// Get messages
	var messages []models.Message
	err = h.DB.
		Where("conversation_id = ?", convUUID).
		Order("created_at ASC").
		Find(&messages).Error

	if err != nil {
		log.Println("Error fetching messages:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to fetch messages",
		})
	}

	// Mark messages as read
	if err := h.DB.Model(&models.Message{}).
		Where("conversation_id = ? AND sender_id != ? AND is_read = false",
			convUUID, userUUID).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": time.Now(),
		}).Error; err != nil {
		log.Println("Error marking messages as read:", err)
		// Don't fail the request, just log it
	}

	// Transform to response
	var responses []MessageResponse
	for _, msg := range messages {
		responses = append(responses, MessageResponse{
			ID:             msg.ID.String(),
			ConversationID: msg.ConversationID.String(),
			SenderID:       msg.SenderID.String(),
			Type:           msg.Type,
			Text:           msg.Text,
			IsRead:         msg.IsRead,
			CreatedAt:      msg.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    responses,
	})
}

// MarkAsRead marks messages as read
func (h *ChatHandler) MarkAsRead(c *fiber.Ctx) error {
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

	var req struct {
		LastReadMessageID string `json:"last_read_message_id"`
	}
	c.BodyParser(&req)

	// Verify conversation exists and user has access
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

	// Mark messages as read
	if err := h.DB.Model(&models.Message{}).
		Where("conversation_id = ? AND sender_id != ? AND is_read = false",
			convUUID, userUUID).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": time.Now(),
		}).Error; err != nil {
		log.Println("Error marking messages as read:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to mark messages as read",
		})
	}

	return c.JSON(fiber.Map{"success": true})
}

// SendMessage sends a message in a conversation
func (h *ChatHandler) SendMessage(c *fiber.Ctx) error {
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

	var req struct {
		Text string `json:"text"`
	}

	if err := c.BodyParser(&req); err != nil || req.Text == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Text is required",
		})
	}

	// Verify conversation
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

	// Create message
	msg := models.Message{
		ConversationID: convUUID,
		SenderID:       userUUID,
		Text:           req.Text,
		IsRead:         false,
	}

	if err := h.DB.Create(&msg).Error; err != nil {
		log.Println("Error creating message:", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to send message",
		})
	}

	// Update conversation
	// Update conversation
	_ = h.DB.Model(&models.Conversation{}).
		Where("id = ?", conv.ID).
		Update("last_message_at", msg.CreatedAt).Error

	// Transform message response
	msgResp := MessageResponse{
		ID:             msg.ID.String(),
		ConversationID: msg.ConversationID.String(),
		SenderID:       msg.SenderID.String(),
		Type:           msg.Type,
		Text:           msg.Text,
		IsRead:         msg.IsRead,
		CreatedAt:      msg.CreatedAt,
	}

	// Broadcast via WebSocket to both users
	h.Hub.SendToConversation(conv.ClientID, conv.FreelancerID, fiber.Map{
		"type":    "new_message",
		"message": msgResp,
	})

	// Send push notification via Redis (optional)
	recipientID := conv.ClientID
	if userUUID == conv.ClientID {
		recipientID = conv.FreelancerID
	}

	notif := map[string]interface{}{
		"type":            "chat_message",
		"conversation_id": convUUID.String(),
		"sender_id":       userUUID.String(),
		"text":            req.Text,
	}
	payload, _ := json.Marshal(notif)
	h.RDB.Publish(context.Background(), "notifications:"+recipientID.String(), payload)

	return c.JSON(fiber.Map{
		"success": true,
		"data":    msgResp,
	})
}

// WebSocketHandler handles WebSocket connections
func (h *ChatHandler) WebSocketHandler(c *websocket.Conn) {
	// Get userID from query
	userID := c.Query("user_id")
	if userID == "" {
		log.Println("WebSocket: user_id parameter missing")
		c.Close()
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		log.Println("WebSocket: invalid user_id:", userID, "error:", err)
		c.Close()
		return
	}

	log.Printf("WebSocket: user %s connected\n", userID)

	client := &realtime.Client{
		ID:     uuid.New().String(),
		UserID: userUUID,
		Conn:   &realtime.WebSocketConn{Conn: c},
		Send:   make(chan []byte, 256),
	}

	h.Hub.RegisterClient(client)
	defer func() {
		h.Hub.UnregisterClient(client)
		log.Printf("WebSocket: user %s disconnected\n", userID)
	}()

	// Send messages from hub to client
	go func() {
		for msg := range client.Send {
			if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Println("WebSocket write error:", err)
				return
			}
		}
	}()

	// Read messages from client (keep connection alive)
	for {
		var payload map[string]interface{}
		if err := c.ReadJSON(&payload); err != nil {
			log.Printf("WebSocket read error for user %s: %v\n", userID, err)
			break
		}
		log.Printf("WebSocket: received from user %s: %v\n", userID, payload)

		// Handle ping/pong
		if msgType, ok := payload["type"].(string); ok && msgType == "pong" {
			// Client responded to ping, connection is alive
			continue
		}
	}
}
