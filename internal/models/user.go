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

type User struct {
	ID       uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Name      string    `gorm:"not null"`
	Email     string    `gorm:"uniqueIndex;not null"`
	Phone     string    `gorm:"type:varchar(30);uniqueIndex"`
	Password  string    `gorm:"not null"`
	Role      Role      `gorm:"type:varchar(20);not null;index"`
	IsActive  bool      `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
