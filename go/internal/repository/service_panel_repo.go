package repository

import (
	"gorm.io/gorm"

	"hosibot/internal/models"
)

// ServicePanelRepository handles CRUD operations for the legacy `panel` table.
type ServicePanelRepository struct {
	db *gorm.DB
}

func NewServicePanelRepository(db *gorm.DB) *ServicePanelRepository {
	return &ServicePanelRepository{db: db}
}

func (r *ServicePanelRepository) FindAll(limit, page int, query string) ([]models.ServicePanel, int64, error) {
	var items []models.ServicePanel
	var total int64

	db := r.db.Model(&models.ServicePanel{})
	if query != "" {
		search := "%" + query + "%"
		db = db.Where("name_panel LIKE ? OR code_panel LIKE ? OR Location LIKE ?", search, search, search)
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

	if err := db.Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *ServicePanelRepository) FindByID(id int) (*models.ServicePanel, error) {
	var panel models.ServicePanel
	if err := r.db.Where("id = ?", id).First(&panel).Error; err != nil {
		return nil, err
	}
	return &panel, nil
}

func (r *ServicePanelRepository) CountByName(name string) (int64, error) {
	var count int64
	err := r.db.Model(&models.ServicePanel{}).Where("name_panel = ?", name).Count(&count).Error
	return count, err
}

func (r *ServicePanelRepository) Create(panel *models.ServicePanel) error {
	return r.db.Create(panel).Error
}

func (r *ServicePanelRepository) Update(id int, updates map[string]interface{}) error {
	return r.db.Model(&models.ServicePanel{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ServicePanelRepository) Delete(id int) error {
	return r.db.Where("id = ?", id).Delete(&models.ServicePanel{}).Error
}
