// internal/models/freelancer_profile.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type FreelancerType string
const (
	FreelancerFullTime FreelancerType = "full_time"
	FreelancerPartTime FreelancerType = "part_time"
)

type OnboardingStatus string
const (
	StatusDraft        OnboardingStatus = "draft"
	StatusPendingReview OnboardingStatus = "pending_review"
	StatusApproved     OnboardingStatus = "approved"
	StatusRejected     OnboardingStatus = "rejected"
)

type FreelancerProfile struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`

	// Step tracking
	OnboardingStep   int             `gorm:"not null;default:1" json:"onboarding_step"` // 1..5
	OnboardingStatus OnboardingStatus `gorm:"type:varchar(30);not null;default:'draft'" json:"onboarding_status"`

	// Step 1 - photo
	PhotoURL string `gorm:"type:text" json:"photo_url"`

	// Step 2 - basic profile
	SystemName     string         `gorm:"type:varchar(120)" json:"system_name"`
	FreelancerType FreelancerType `gorm:"type:varchar(30)" json:"freelancer_type"`

	// Step 3 - about
	About string `gorm:"type:text" json:"about"`

	// Step 4 - identity (KTP)
	FirstName  string `gorm:"type:varchar(80)" json:"first_name"`
	MiddleName string `gorm:"type:varchar(80)" json:"middle_name"`
	LastName   string `gorm:"type:varchar(80)" json:"last_name"`
	NIK string `gorm:"type:varchar(16);uniqueIndex;index" json:"nik"`


	KTPAddress string `gorm:"type:text" json:"ktp_address"`
	PostalCode string `gorm:"type:varchar(10)" json:"postal_code"`
	Kelurahan  string `gorm:"type:varchar(80)" json:"kelurahan"`
	Kecamatan  string `gorm:"type:varchar(80)" json:"kecamatan"`
	City       string `gorm:"type:varchar(120)" json:"city"`

	// Step 5 - contact confirm
	ContactEmail   string `gorm:"type:varchar(150)" json:"contact_email"` // otomatis dari email user
	ContactPhone   string `gorm:"type:varchar(30)" json:"contact_phone"`
	CurrentAddress string `gorm:"type:text" json:"current_address"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
