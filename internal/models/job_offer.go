// internal/models/job_offer.go
package models

import (
	"math/rand"
	"time"

	"github.com/google/uuid"
)

type JobOfferStatus string

const (
	OfferStatusPending   JobOfferStatus = "pending"   // Menunggu Pembayaran
	OfferStatusPaid      JobOfferStatus = "paid"      // Pembayaran Diterima
	OfferStatusWorking   JobOfferStatus = "working"   // Sedang Bekerja
	OfferStatusDelivered JobOfferStatus = "delivered" // Terkirim
	OfferStatusCompleted JobOfferStatus = "completed" // Selesai
	OfferStatusCancelled JobOfferStatus = "cancelled" // Dibatalkan
)

type JobOffer struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderCode      string    `gorm:"unique;size:10" json:"order_code"` // e.g., L9POKTVJ
	ConversationID uuid.UUID `gorm:"type:uuid;index" json:"conversation_id"`
	FreelancerID   uuid.UUID `gorm:"type:uuid;index" json:"freelancer_id"`
	ClientID       uuid.UUID `gorm:"type:uuid;index" json:"client_id"`
	ProductID      *uint     `gorm:"index" json:"product_id,omitempty"`

	// Step 1: Harga
	Price       int64 `json:"price"`        // Harga Pekerjaan
	PlatformFee int64 `json:"platform_fee"` // Komisi Platform (10%)
	NetAmount   int64 `json:"net_amount"`   // Anda akan menerima

	// Step 2: Deskripsi
	Title         string `json:"title"`
	Description   string `json:"description"`    // Deskripsi Pekerjaan dan Langkah Kerja
	RevisionCount int    `json:"revision_count"` // Bisa revisi X kali

	// Step 3: Hasil Pekerjaan
	StartDate      time.Time `json:"start_date"`
	DeliveryDate   time.Time `json:"delivery_date"`
	DeliveryFormat string    `json:"delivery_format"` // e.g., .pdf, .png
	Notes          string    `json:"notes"`           // Catatan tambahan

	// Submission Data
	WorkDeliveryLink  string `json:"work_delivery_link"`
	WorkDeliveryFiles string `json:"work_delivery_files"` // JSON string or comma-separated URLs
	UsedRevisionCount int    `gorm:"default:0" json:"used_revision_count"`

	Status JobOfferStatus `gorm:"default:pending" json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	Conversation *Conversation `gorm:"foreignKey:ConversationID" json:"conversation,omitempty"`
	Freelancer   *User         `gorm:"foreignKey:FreelancerID" json:"freelancer,omitempty"`
	Client       *User         `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	Product      *Product      `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

// GenerateOrderCode generates a random alphanumeric code
func GenerateOrderCode() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, 8)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
