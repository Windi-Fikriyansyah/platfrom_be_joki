package models

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleClient     Role = "client"
	RoleFreelancer Role = "freelancer"
	RoleAdmin      Role = "admin"
)

// internal/models/user.go
type User struct {
	ID    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name  string    `gorm:"not null" json:"name"`
	Email string    `gorm:"uniqueIndex;not null" json:"email"`
	Phone string    `gorm:"type:varchar(30);uniqueIndex" json:"phone"`

	Password string `gorm:"not null" json:"-"` 
	Role     Role `gorm:"type:varchar(20);not null;index" json:"role"`
	IsActive bool `gorm:"default:true" json:"is_active"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// ✅ HAS ONE freelancer_profile (freelancer_profiles.user_id -> users.id)
	FreelancerProfile *FreelancerProfile `gorm:"foreignKey:UserID;references:ID" json:"freelancer_profile,omitempty"`
}

