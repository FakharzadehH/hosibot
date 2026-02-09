package repository

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"hosibot/internal/models"
)

// SettingRepository handles settings, textbot, channels, and other config tables.
type SettingRepository struct {
	db *gorm.DB
}

func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

// DB returns the underlying gorm.DB instance.
func (r *SettingRepository) DB() *gorm.DB {
	return r.db
}

// --- Setting ---

// GetSettings returns the single settings row.
func (r *SettingRepository) GetSettings() (*models.Setting, error) {
	var setting models.Setting
	if err := r.db.First(&setting).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

// UpdateSetting updates a specific setting field.
func (r *SettingRepository) UpdateSetting(column string, value interface{}) error {
	return r.db.Model(&models.Setting{}).Where("1=1").Update(column, value).Error
}

// --- TextBot ---

// GetText returns a text entry by ID.
func (r *SettingRepository) GetText(id string) (string, error) {
	var tb models.TextBot
	if err := r.db.Where("id_text = ?", id).First(&tb).Error; err != nil {
		return "", err
	}
	return tb.Text, nil
}

// GetAllTexts returns all text entries.
func (r *SettingRepository) GetAllTexts() ([]models.TextBot, error) {
	var texts []models.TextBot
	err := r.db.Find(&texts).Error
	return texts, err
}

// SetText inserts or updates a text entry.
func (r *SettingRepository) SetText(id, text string) error {
	return r.db.Save(&models.TextBot{IDText: id, Text: text}).Error
}

// --- Channels ---

// GetChannels returns all channels.
func (r *SettingRepository) GetChannels() ([]models.Channel, error) {
	var channels []models.Channel
	err := r.db.Find(&channels).Error
	return channels, err
}

// --- Discount ---

// FindAllDiscounts returns discounts with pagination.
func (r *SettingRepository) FindAllDiscounts(limit, page int, query string) ([]models.Discount, int64, error) {
	var items []models.Discount
	var total int64

	db := r.db.Model(&models.Discount{})
	if query != "" {
		db = db.Where("code LIKE ?", "%"+query+"%")
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
	if err := db.Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *SettingRepository) FindDiscountByID(id int) (*models.Discount, error) {
	var d models.Discount
	if err := r.db.Where("id = ?", id).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *SettingRepository) CreateDiscount(d *models.Discount) error {
	return r.db.Create(d).Error
}

func (r *SettingRepository) DeleteDiscount(id int) error {
	return r.db.Where("id = ?", id).Delete(&models.Discount{}).Error
}

// --- DiscountSell ---

func (r *SettingRepository) FindAllDiscountSells(limit, page int, query string) ([]models.DiscountSell, int64, error) {
	var items []models.DiscountSell
	var total int64

	db := r.db.Model(&models.DiscountSell{})
	if query != "" {
		db = db.Where("codeDiscount LIKE ?", "%"+query+"%")
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
	if err := db.Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *SettingRepository) FindDiscountSellByID(id int) (*models.DiscountSell, error) {
	var d models.DiscountSell
	if err := r.db.Where("id = ?", id).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *SettingRepository) CreateDiscountSell(d *models.DiscountSell) error {
	return r.db.Create(d).Error
}

func (r *SettingRepository) DeleteDiscountSell(id int) error {
	return r.db.Where("id = ?", id).Delete(&models.DiscountSell{}).Error
}

// --- Category ---

func (r *SettingRepository) FindAllCategories(limit, page int, query string) ([]models.Category, int64, error) {
	var items []models.Category
	var total int64

	db := r.db.Model(&models.Category{})
	if query != "" {
		db = db.Where("remark LIKE ?", "%"+query+"%")
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
	if err := db.Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *SettingRepository) FindCategoryByID(id int) (*models.Category, error) {
	var c models.Category
	if err := r.db.Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *SettingRepository) CreateCategory(c *models.Category) error {
	return r.db.Create(c).Error
}

func (r *SettingRepository) UpdateCategory(id int, updates map[string]interface{}) error {
	return r.db.Model(&models.Category{}).Where("id = ?", id).Updates(updates).Error
}

func (r *SettingRepository) DeleteCategory(id int) error {
	return r.db.Where("id = ?", id).Delete(&models.Category{}).Error
}

// --- PaySetting / ShopSetting (key-value) ---

func (r *SettingRepository) GetPaySetting(name string) (string, error) {
	var ps models.PaySetting
	if err := r.db.Where("NamePay = ?", name).First(&ps).Error; err != nil {
		return "", err
	}
	return ps.ValuePay, nil
}

func (r *SettingRepository) SetPaySetting(name, value string) error {
	return r.db.Save(&models.PaySetting{NamePay: name, ValuePay: value}).Error
}

func (r *SettingRepository) GetAllPaySettings() ([]models.PaySetting, error) {
	var settings []models.PaySetting
	err := r.db.Find(&settings).Error
	return settings, err
}

func (r *SettingRepository) GetShopSetting(name string) (string, error) {
	var ss models.ShopSetting
	if err := r.db.Where("Namevalue = ?", name).First(&ss).Error; err != nil {
		return "", err
	}
	return ss.Value, nil
}

func (r *SettingRepository) SetShopSetting(name, value string) error {
	return r.db.Save(&models.ShopSetting{NameValue: name, Value: value}).Error
}

func (r *SettingRepository) GetAllShopSettings() ([]models.ShopSetting, error) {
	var settings []models.ShopSetting
	err := r.db.Find(&settings).Error
	return settings, err
}

// --- Admin ---

func (r *SettingRepository) FindAdminByID(id string) (*models.Admin, error) {
	var admin models.Admin
	if err := r.db.Where("id_admin = ?", id).First(&admin).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

func (r *SettingRepository) FindAdminByUsername(username string) (*models.Admin, error) {
	var admin models.Admin
	if err := r.db.Where("username = ?", username).First(&admin).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

func (r *SettingRepository) GetAllAdmins() ([]models.Admin, error) {
	var admins []models.Admin
	err := r.db.Find(&admins).Error
	return admins, err
}

// --- Logs API ---

func (r *SettingRepository) CreateAPILog(header, data interface{}, ip, actions string) error {
	headerJSON, _ := json.Marshal(header)
	dataJSON, _ := json.Marshal(data)

	log := models.LogsAPI{
		Header:  string(headerJSON),
		Data:    string(dataJSON),
		IP:      ip,
		Time:    time.Now().Format("2006/01/02 15:04:05"),
		Actions: actions,
	}
	return r.db.Create(&log).Error
}

// --- TopicID ---

func (r *SettingRepository) GetTopicID(report string) (string, error) {
	var t models.TopicID
	if err := r.db.Where("report = ?", report).First(&t).Error; err != nil {
		return "", err
	}
	return t.IDReport, nil
}

// --- CardNumber ---

func (r *SettingRepository) GetAllCardNumbers() ([]models.CardNumber, error) {
	var cards []models.CardNumber
	err := r.db.Find(&cards).Error
	return cards, err
}

// --- ServiceOther ---

func (r *SettingRepository) FindAllServiceOther(limit int) ([]models.ServiceOther, error) {
	var services []models.ServiceOther
	db := r.db.Model(&models.ServiceOther{})
	if limit > 0 {
		db = db.Limit(limit)
	}
	err := db.Find(&services).Error
	return services, err
}

// --- BotSaz ---

func (r *SettingRepository) CountBotSaz() (int64, error) {
	var count int64
	err := r.db.Model(&models.BotSaz{}).Count(&count).Error
	return count, err
}

func (r *SettingRepository) CountBotSazByUserID(userID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.BotSaz{}).Where("id_user = ?", userID).Count(&count).Error
	return count, err
}

func (r *SettingRepository) CountBotSazByToken(token string) (int64, error) {
	var count int64
	err := r.db.Model(&models.BotSaz{}).Where("bot_token = ?", token).Count(&count).Error
	return count, err
}

func (r *SettingRepository) FindBotSazByUserID(userID string) (*models.BotSaz, error) {
	var bot models.BotSaz
	if err := r.db.Where("id_user = ?", userID).First(&bot).Error; err != nil {
		return nil, err
	}
	return &bot, nil
}

func (r *SettingRepository) CreateBotSaz(bot *models.BotSaz) error {
	return r.db.Create(bot).Error
}

func (r *SettingRepository) UpdateBotSazByUserID(userID string, updates map[string]interface{}) error {
	return r.db.Model(&models.BotSaz{}).Where("id_user = ?", userID).Updates(updates).Error
}

func (r *SettingRepository) DeleteBotSazByUserID(userID string) error {
	return r.db.Where("id_user = ?", userID).Delete(&models.BotSaz{}).Error
}
