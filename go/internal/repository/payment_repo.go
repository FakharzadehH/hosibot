package repository

import (
	"gorm.io/gorm"

	"hosibot/internal/models"
)

// PaymentRepository handles payment report database operations.
type PaymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

// FindAll returns payments with pagination and search.
func (r *PaymentRepository) FindAll(limit, page int, query string) ([]models.PaymentReport, int64, error) {
	var payments []models.PaymentReport
	var total int64

	db := r.db.Model(&models.PaymentReport{})

	if query != "" {
		search := "%" + query + "%"
		db = db.Where("id_order LIKE ? OR id_user LIKE ? OR Payment_Method LIKE ?",
			search, search, search)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	if err := db.Limit(limit).Offset(offset).Order("time DESC").Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}

// FindByOrderID returns a payment by order ID.
func (r *PaymentRepository) FindByOrderID(orderID string) (*models.PaymentReport, error) {
	var payment models.PaymentReport
	if err := r.db.Where("id_order = ?", orderID).First(&payment).Error; err != nil {
		return nil, err
	}
	return &payment, nil
}

// FindByUserID returns payments for a specific user.
func (r *PaymentRepository) FindByUserID(userID string) ([]models.PaymentReport, error) {
	var payments []models.PaymentReport
	err := r.db.Where("id_user = ?", userID).Order("time DESC").Find(&payments).Error
	return payments, err
}

// Create creates a new payment report.
func (r *PaymentRepository) Create(payment *models.PaymentReport) error {
	return r.db.Create(payment).Error
}

// Update updates a payment report.
func (r *PaymentRepository) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&models.PaymentReport{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateByOrderID updates a payment report by order ID.
func (r *PaymentRepository) UpdateByOrderID(orderID string, updates map[string]interface{}) error {
	return r.db.Model(&models.PaymentReport{}).Where("id_order = ?", orderID).Updates(updates).Error
}

// CountByUserID counts payments for a user.
func (r *PaymentRepository) CountByUserID(userID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.PaymentReport{}).Where("id_user = ?", userID).Count(&count).Error
	return count, err
}

// SumByUserID returns total payment amount for a user.
func (r *PaymentRepository) SumByUserID(userID string) (int64, error) {
	var sum int64
	err := r.db.Model(&models.PaymentReport{}).
		Where("id_user = ? AND payment_Status = ?", userID, "paid").
		Select("COALESCE(SUM(CAST(price AS SIGNED)), 0)").Scan(&sum).Error
	return sum, err
}
