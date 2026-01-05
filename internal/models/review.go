package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Review struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	JobOfferID   uuid.UUID `gorm:"type:uuid;index;unique" json:"job_offer_id"`
	ClientID     uuid.UUID `gorm:"type:uuid;index" json:"client_id"`
	FreelancerID uuid.UUID `gorm:"type:uuid;index" json:"freelancer_id"`
	ProductID    *uint     `gorm:"index" json:"product_id,omitempty"`

	Rating  int    `gorm:"not null" json:"rating"` // 1-5
	Comment string `gorm:"type:text" json:"comment"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	JobOffer   *JobOffer `gorm:"foreignKey:JobOfferID" json:"job_offer,omitempty"`
	Client     *User     `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	Freelancer *User     `gorm:"foreignKey:FreelancerID" json:"freelancer,omitempty"`
	Product    *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

func (r *Review) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return
}
