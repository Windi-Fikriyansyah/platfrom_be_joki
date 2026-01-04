package wallet

import (
	"errors"
	"fmt"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WalletService struct {
	DB *gorm.DB
}

func NewWalletService(db *gorm.DB) *WalletService {
	return &WalletService{DB: db}
}

// CreditFreelancer adds funds to freelancer's balance and creates a ledger entry.
// This should be called within a DB transaction.
func (s *WalletService) CreditFreelancer(tx *gorm.DB, userID uuid.UUID, amount int64, referenceID uuid.UUID, description string) error {
	if amount <= 0 {
		return errors.New("amount to credit must be greater than zero")
	}

	// 1. Update FreelancerProfile balance atomically
	result := tx.Model(&models.FreelancerProfile{}).
		Where("user_id = ?", userID).
		Update("balance", gorm.Expr("balance + ?", amount))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("freelancer profile not found for user %s", userID)
	}

	// 2. Create WalletTransaction (Ledger)
	ledger := models.WalletTransaction{
		ID:          uuid.New(),
		UserID:      userID,
		Amount:      amount,
		Type:        models.WalletTrxCredit,
		Description: description,
		ReferenceID: &referenceID,
	}

	if err := tx.Create(&ledger).Error; err != nil {
		return err
	}

	return nil
}

// CreditClient adds funds to client's balance (e.g., for refunds) and creates a ledger entry.
// This should be called within a DB transaction.
func (s *WalletService) CreditClient(tx *gorm.DB, userID uuid.UUID, amount int64, referenceID uuid.UUID, description string) error {
	if amount <= 0 {
		return errors.New("amount to credit must be greater than zero")
	}

	// 1. Update User balance atomically
	result := tx.Model(&models.User{}).
		Where("id = ?", userID).
		Update("balance", gorm.Expr("balance + ?", amount))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found for id %s", userID)
	}

	// 2. Create WalletTransaction (Ledger)
	ledger := models.WalletTransaction{
		ID:          uuid.New(),
		UserID:      userID,
		Amount:      amount,
		Type:        models.WalletTrxRefund, // Use refund type for client credits
		Description: description,
		ReferenceID: &referenceID,
	}

	if err := tx.Create(&ledger).Error; err != nil {
		return err
	}

	return nil
}

// DebitClient deducts funds from client's balance and creates a ledger entry.
// This should be called within a DB transaction.
func (s *WalletService) DebitClient(tx *gorm.DB, userID uuid.UUID, amount int64, referenceID uuid.UUID, description string) error {
	if amount <= 0 {
		return errors.New("amount to debit must be greater than zero")
	}

	// 1. Check current balance first to ensure it doesn't go negative (if needed)
	// For additional security, we can use a check constraint in DB, but here we do it in code too.
	var user models.User
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, "id = ?", userID).Error; err != nil {
		return err
	}

	if user.Balance < amount {
		return errors.New("insufficient balance")
	}

	// 2. Update User balance atomically
	result := tx.Model(&models.User{}).
		Where("id = ? AND balance >= ?", userID, amount).
		Update("balance", gorm.Expr("balance - ?", amount))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("failed to deduct balance: user not found or insufficient balance")
	}

	// 3. Create WalletTransaction (Ledger)
	ledger := models.WalletTransaction{
		ID:          uuid.New(),
		UserID:      userID,
		Amount:      amount,
		Type:        models.WalletTrxDebit,
		Description: description,
		ReferenceID: &referenceID,
	}

	if err := tx.Create(&ledger).Error; err != nil {
		return err
	}

	return nil
}
