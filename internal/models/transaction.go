package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionStatus string

const (
	TransactionStatusUnpaid  TransactionStatus = "UNPAID"
	TransactionStatusPaid    TransactionStatus = "PAID"
	TransactionStatusFailed  TransactionStatus = "FAILED"
	TransactionStatusExpired TransactionStatus = "EXPIRED"
	TransactionStatusRefund  TransactionStatus = "REFUND"
)

type Transaction struct {
	ID                uuid.UUID         `gorm:"type:char(36);primaryKey" json:"id"`
	JobOfferID        uuid.UUID         `gorm:"type:char(36);index" json:"job_offer_id"`
	JobOffer          JobOffer          `gorm:"foreignKey:JobOfferID" json:"job_offer"`
	Reference         string            `gorm:"type:varchar(50);uniqueIndex" json:"reference"`    // Tripay Reference
	MerchantRef       string            `gorm:"type:varchar(50);uniqueIndex" json:"merchant_ref"` // INV-{OrderCode}
	PaymentMethod     string            `gorm:"type:varchar(50)" json:"payment_method"`
	PaymentMethodCode string            `gorm:"type:varchar(50)" json:"payment_method_code"`
	TotalAmount       int64             `json:"total_amount"`
	FeeMerchant       int64             `json:"fee_merchant"`
	FeeCustomer       int64             `json:"fee_customer"`
	TotalFee          int64             `json:"total_fee"`
	AmountReceived    int64             `json:"amount_received"`
	CheckoutURL       string            `gorm:"type:text" json:"checkout_url"`
	Status            TransactionStatus `gorm:"type:varchar(20);default:'UNPAID'" json:"status"`
	PaidAt            *time.Time        `json:"paid_at"`
	Note              string            `gorm:"type:text" json:"note"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

func (t *Transaction) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return
}
