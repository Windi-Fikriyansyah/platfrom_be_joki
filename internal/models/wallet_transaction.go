package models

import (
	"time"

	"github.com/google/uuid"
)

type WalletTrxType string

const (
	WalletTrxCredit WalletTrxType = "credit" // Pendapatan masuk
	WalletTrxDebit  WalletTrxType = "debit"  // Penarikan/Pengurangan
	WalletTrxRefund WalletTrxType = "refund" // Pengembalian dana
)

type WalletTransaction struct {
	ID          uuid.UUID     `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID     `gorm:"type:uuid;index;not null" json:"user_id"`
	Amount      int64         `gorm:"not null" json:"amount"`
	Type        WalletTrxType `gorm:"type:varchar(20);not null" json:"type"`
	Description string        `gorm:"type:text" json:"description"`
	ReferenceID *uuid.UUID    `gorm:"type:uuid;index" json:"reference_id,omitempty"` // ID JobOffer atau ID Penarikan
	CreatedAt   time.Time     `json:"created_at"`

	// Relation
	User *User `gorm:"foreignKey:UserID" json:"-"`
}
