package repository

import (
	"gorm.io/gorm"

	"hosibot/internal/models"
)

// PanelRepository handles panel database operations.
type PanelRepository struct {
	db *gorm.DB
}

func NewPanelRepository(db *gorm.DB) *PanelRepository {
	return &PanelRepository{db: db}
}

// FindAll returns panels with pagination and search.
func (r *PanelRepository) FindAll(limit, page int, query string) ([]models.Panel, int64, error) {
	var panels []models.Panel
	var total int64

	db := r.db.Model(&models.Panel{})

	if query != "" {
		search := "%" + query + "%"
		db = db.Where("name_panel LIKE ? OR code_panel LIKE ? OR url_panel LIKE ?",
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

	if err := db.Limit(limit).Offset(offset).Find(&panels).Error; err != nil {
		return nil, 0, err
	}
	return panels, total, nil
}

// FindByID returns a panel by ID.
func (r *PanelRepository) FindByID(id int) (*models.Panel, error) {
	var panel models.Panel
	if err := r.db.Where("id = ?", id).First(&panel).Error; err != nil {
		return nil, err
	}
	return &panel, nil
}

// FindByCode returns a panel by code.
func (r *PanelRepository) FindByCode(code string) (*models.Panel, error) {
	var panel models.Panel
	if err := r.db.Where("code_panel = ?", code).First(&panel).Error; err != nil {
		return nil, err
	}
	return &panel, nil
}

// FindActive returns all active panels.
func (r *PanelRepository) FindActive() ([]models.Panel, error) {
	var panels []models.Panel
	err := r.db.Where("status = ?", "active").Find(&panels).Error
	return panels, err
}

// FindByType returns panels filtered by type (marzban, x-ui_single, etc.).
func (r *PanelRepository) FindByType(panelType string) ([]models.Panel, error) {
	var panels []models.Panel
	err := r.db.Where("type = ?", panelType).Find(&panels).Error
	return panels, err
}

// Create creates a new panel.
func (r *PanelRepository) Create(panel *models.Panel) error {
	return r.db.Create(panel).Error
}

// Update updates panel fields.
func (r *PanelRepository) Update(id int, updates map[string]interface{}) error {
	return r.db.Model(&models.Panel{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes a panel.
func (r *PanelRepository) Delete(id int) error {
	return r.db.Where("id = ?", id).Delete(&models.Panel{}).Error
}

// FindByName returns a panel by name_panel.
func (r *PanelRepository) FindByName(name string) (*models.Panel, error) {
	var panel models.Panel
	if err := r.db.Where("name_panel = ?", name).First(&panel).Error; err != nil {
		return nil, err
	}
	return &panel, nil
}

// IncrementCounter increments the counter_panel (stored in conecton column) for a panel.
func (r *PanelRepository) IncrementCounter(code string) error {
	return r.db.Model(&models.Panel{}).Where("code_panel = ?", code).
		UpdateColumn("conecton", gorm.Expr("CAST(conecton AS UNSIGNED) + 1")).Error
}
