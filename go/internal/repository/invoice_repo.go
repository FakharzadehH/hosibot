package repository

import (
	"strconv"

	"gorm.io/gorm"

	"hosibot/internal/models"
)

// InvoiceRepository handles invoice database operations.
type InvoiceRepository struct {
	db *gorm.DB
}

func NewInvoiceRepository(db *gorm.DB) *InvoiceRepository {
	return &InvoiceRepository{db: db}
}

// FindAll returns invoices with pagination and search.
func (r *InvoiceRepository) FindAll(limit, page int, query string) ([]models.Invoice, int64, error) {
	var invoices []models.Invoice
	var total int64

	db := r.db.Model(&models.Invoice{})

	if query != "" {
		search := "%" + query + "%"
		db = db.Where("id_invoice LIKE ? OR username LIKE ? OR id_user LIKE ? OR name_product LIKE ?",
			search, search, search, search)
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

	if err := db.Limit(limit).Offset(offset).Order("time_sell DESC").Find(&invoices).Error; err != nil {
		return nil, 0, err
	}
	return invoices, total, nil
}

// FindByID returns an invoice by its invoice ID.
func (r *InvoiceRepository) FindByID(invoiceID string) (*models.Invoice, error) {
	var invoice models.Invoice
	if err := r.db.Where("id_invoice = ?", invoiceID).First(&invoice).Error; err != nil {
		return nil, err
	}
	return &invoice, nil
}

// FindByUsername returns the first invoice row for a service username.
func (r *InvoiceRepository) FindByUsername(username string) (*models.Invoice, error) {
	var invoice models.Invoice
	if err := r.db.Where("username = ?", username).Order("time_sell DESC").First(&invoice).Error; err != nil {
		return nil, err
	}
	return &invoice, nil
}

// FindByUserID returns all invoices for a user.
func (r *InvoiceRepository) FindByUserID(userID string) ([]models.Invoice, error) {
	var invoices []models.Invoice
	err := r.db.Where("id_user = ?", userID).Order("time_sell DESC").Find(&invoices).Error
	return invoices, err
}

// FindActiveByUserID returns active invoices for a user.
func (r *InvoiceRepository) FindActiveByUserID(userID string) ([]models.Invoice, error) {
	var invoices []models.Invoice
	err := r.db.Where("id_user = ? AND Status = ?", userID, "active").Find(&invoices).Error
	return invoices, err
}

// Create creates a new invoice.
func (r *InvoiceRepository) Create(invoice *models.Invoice) error {
	return r.db.Create(invoice).Error
}

// Update updates invoice fields.
func (r *InvoiceRepository) Update(invoiceID string, updates map[string]interface{}) error {
	return r.db.Model(&models.Invoice{}).Where("id_invoice = ?", invoiceID).Updates(updates).Error
}

// Delete deletes an invoice.
func (r *InvoiceRepository) Delete(invoiceID string) error {
	return r.db.Where("id_invoice = ?", invoiceID).Delete(&models.Invoice{}).Error
}

// CountByUserID counts total invoices for a user.
func (r *InvoiceRepository) CountByUserID(userID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.Invoice{}).Where("id_user = ?", userID).Count(&count).Error
	return count, err
}

// CountActiveByUserID counts active invoices for a user.
func (r *InvoiceRepository) CountActiveByUserID(userID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.Invoice{}).Where("id_user = ? AND Status = ?", userID, "active").Count(&count).Error
	return count, err
}

// CountByLocation counts active invoices for a given panel/location.
func (r *InvoiceRepository) CountByLocation(location string) (int64, error) {
	var count int64
	err := r.db.Model(&models.Invoice{}).Where(
		"Service_location = ? AND Status IN ?",
		location,
		[]string{"active", "end_of_time", "sendedwarn", "end_of_volume", "send_on_hold"},
	).Count(&count).Error
	return count, err
}

// CountByProductCode counts invoices by product code (name_product matching).
func (r *InvoiceRepository) CountByProductCode(code string) (int64, error) {
	var count int64
	err := r.db.Model(&models.Invoice{}).Where("name_product = ?", code).Count(&count).Error
	return count, err
}

// ProductStatsByName returns invoice count and sum_price for a product name.
// PHP parity: SELECT COUNT(username), SUM(price_product) FROM invoice WHERE name_product = ?
func (r *InvoiceRepository) ProductStatsByName(name string) (int64, int64, error) {
	var prices []string
	if err := r.db.Model(&models.Invoice{}).
		Where("name_product = ?", name).
		Pluck("price_product", &prices).Error; err != nil {
		return 0, 0, err
	}

	var sum int64
	for _, raw := range prices {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		sum += v
	}
	return int64(len(prices)), sum, nil
}

// FindServices returns all active services (invoices with active status).
func (r *InvoiceRepository) FindServices(limit, page int, query string) ([]models.Invoice, int64, error) {
	var invoices []models.Invoice
	var total int64

	db := r.db.Model(&models.Invoice{}).Where("Status = ?", "active")

	if query != "" {
		search := "%" + query + "%"
		db = db.Where("username LIKE ? OR id_user LIKE ? OR id_invoice LIKE ?",
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

	if err := db.Limit(limit).Offset(offset).Order("time_sell DESC").Find(&invoices).Error; err != nil {
		return nil, 0, err
	}
	return invoices, total, nil
}
