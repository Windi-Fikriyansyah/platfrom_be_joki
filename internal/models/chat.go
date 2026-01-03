// internal/models/chat.go
package models

import (
	"time"

	"github.com/google/uuid"
)

// Conversation represents a chat conversation between users
type Conversation struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`

	ClientID     uuid.UUID `gorm:"type:uuid;index" json:"client_id"`
	FreelancerID uuid.UUID `gorm:"type:uuid;index" json:"freelancer_id"`

	// optional, tapi jangan dijadikan kunci unik kalau mau 1-1 berdasarkan orang saja
	ProductID *uint `gorm:"index" json:"product_id,omitempty"`

	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	Client     *User     `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	Freelancer *User     `gorm:"foreignKey:FreelancerID" json:"freelancer,omitempty"`
	Product    *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Messages   []Message `gorm:"foreignKey:ConversationID" json:"messages,omitempty"`
}

type ConversationMemberRead struct {
	ID                uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ConversationID    uuid.UUID `gorm:"type:uuid;index" json:"conversation_id"`
	UserID            uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	LastReadMessageID uuid.UUID `gorm:"type:uuid" json:"last_read_message_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message represents a message in a conversation
type Message struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ConversationID uuid.UUID  `gorm:"type:uuid;index" json:"conversation_id"`
	SenderID       uuid.UUID  `gorm:"type:uuid;index" json:"sender_id"`
	Type           string     `gorm:"default:'text'" json:"type"` // text, offer, system
	Text           string     `json:"text"`
	IsRead         bool       `gorm:"default:false" json:"is_read"`
	ReadAt         *time.Time `json:"read_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Preloaded relation
	Sender *User `gorm:"foreignKey:SenderID" json:"sender,omitempty"`
}
