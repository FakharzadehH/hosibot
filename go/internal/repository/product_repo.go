package repository

import (
	"gorm.io/gorm"

	"hosibot/internal/models"
)

// ProductRepository handles product database operations.
type ProductRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// FindAll returns products with pagination and search.
func (r *ProductRepository) FindAll(limit, page int, query string) ([]models.Product, int64, error) {
	var products []models.Product
	var total int64

	db := r.db.Model(&models.Product{})

	if query != "" {
		search := "%" + query + "%"
		db = db.Where("name_product LIKE ? OR code_product LIKE ?", search, search)
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

	if err := db.Limit(limit).Offset(offset).Find(&products).Error; err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

// FindByID returns a product by ID.
func (r *ProductRepository) FindByID(id int) (*models.Product, error) {
	var product models.Product
	if err := r.db.Where("id = ?", id).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

// FindByCode returns a product by code.
func (r *ProductRepository) FindByCode(code string) (*models.Product, error) {
	var product models.Product
	if err := r.db.Where("code_product = ?", code).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

// Create creates a new product.
func (r *ProductRepository) Create(product *models.Product) error {
	return r.db.Create(product).Error
}

// Update updates product fields.
func (r *ProductRepository) Update(id int, updates map[string]interface{}) error {
	return r.db.Model(&models.Product{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes a product by ID.
func (r *ProductRepository) Delete(id int) error {
	return r.db.Where("id = ?", id).Delete(&models.Product{}).Error
}

// CountByLocation counts products for a specific panel location.
func (r *ProductRepository) CountByLocation(locationCode string) (int64, error) {
	var count int64
	err := r.db.Model(&models.Product{}).Where("Location = ?", locationCode).Count(&count).Error
	return count, err
}
