package repository

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"hosibot/internal/models"
)

// UserRepository handles all user database operations.
type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindAll returns users with pagination and optional search/agent filter.
func (r *UserRepository) FindAll(limit, page int, query, agent string) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	db := r.db.Model(&models.User{})

	if query != "" {
		search := "%" + query + "%"
		db = db.Where("id LIKE ? OR username LIKE ? OR namecustom LIKE ? OR number LIKE ?",
			search, search, search, search)
	}
	if agent != "" {
		db = db.Where("agent = ?", agent)
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

	if err := db.Limit(limit).Offset(offset).Order("register DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// FindByID finds a user by Telegram chat ID.
func (r *UserRepository) FindByID(chatID string) (*models.User, error) {
	var user models.User
	if err := r.db.Where("id = ?", chatID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Create inserts a new user.
func (r *UserRepository) Create(user *models.User) error {
	return r.db.Create(user).Error
}

// Update updates user fields.
func (r *UserRepository) Update(chatID string, updates map[string]interface{}) error {
	return r.db.Model(&models.User{}).Where("id = ?", chatID).Updates(updates).Error
}

// UpdateStep updates the user's step field (frequently used in bot flow).
func (r *UserRepository) UpdateStep(chatID, step string) error {
	return r.db.Model(&models.User{}).Where("id = ?", chatID).Update("step", step).Error
}

// UpdateBalance adds or subtracts from user balance.
func (r *UserRepository) UpdateBalance(chatID string, amount int) error {
	return r.db.Model(&models.User{}).Where("id = ?", chatID).
		Update("Balance", gorm.Expr("Balance + ?", amount)).Error
}

// SetBalance sets user balance to exact value.
func (r *UserRepository) SetBalance(chatID string, amount int) error {
	return r.db.Model(&models.User{}).Where("id = ?", chatID).
		Update("Balance", amount).Error
}

// Block blocks or unblocks a user.
func (r *UserRepository) Block(chatID, description, typeBlock string) error {
	updates := map[string]interface{}{
		"description_blocking": description,
	}
	if typeBlock == "block" {
		updates["User_Status"] = "blocked"
	} else {
		updates["User_Status"] = "active"
		updates["description_blocking"] = ""
	}
	return r.db.Model(&models.User{}).Where("id = ?", chatID).Updates(updates).Error
}

// SetVerify sets user verification status.
func (r *UserRepository) SetVerify(chatID, verify string) error {
	return r.db.Model(&models.User{}).Where("id = ?", chatID).Update("verify", verify).Error
}

// CountByAgent counts users belonging to a specific agent.
func (r *UserRepository) CountByAgent(agentID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("agent = ?", agentID).Count(&count).Error
	return count, err
}

// FindAffiliates returns all users referred by a specific user.
func (r *UserRepository) FindAffiliates(chatID string) ([]models.User, error) {
	var users []models.User
	err := r.db.Where("affiliates = ?", chatID).Find(&users).Error
	return users, err
}

// TransferAccount moves a user and updates related records.
func (r *UserRepository) TransferAccount(oldID, newID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Update user ID
		if err := tx.Model(&models.User{}).Where("id = ?", oldID).Update("id", newID).Error; err != nil {
			return fmt.Errorf("failed to update user id: %w", err)
		}
		// Update invoice references
		if err := tx.Model(&models.Invoice{}).Where("id_user = ?", oldID).Update("id_user", newID).Error; err != nil {
			return fmt.Errorf("failed to update invoices: %w", err)
		}
		// Update payment references
		if err := tx.Model(&models.PaymentReport{}).Where("id_user = ?", oldID).Update("id_user", newID).Error; err != nil {
			return fmt.Errorf("failed to update payments: %w", err)
		}
		// Update affiliate references
		if err := tx.Model(&models.User{}).Where("affiliates = ?", oldID).Update("affiliates", newID).Error; err != nil {
			return fmt.Errorf("failed to update affiliates: %w", err)
		}
		return nil
	})
}

// Exists checks whether user with given ID exists.
func (r *UserRepository) Exists(chatID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("id = ?", chatID).Count(&count).Error
	return count > 0, err
}

// Delete removes a user by chat ID.
func (r *UserRepository) Delete(chatID string) error {
	return r.db.Where("id = ?", chatID).Delete(&models.User{}).Error
}

// FindAgents returns all agent users.
func (r *UserRepository) FindAgents() ([]models.User, error) {
	var users []models.User
	err := r.db.Where("agent != '' AND agent != '0'").Find(&users).Error
	return users, err
}

// UpdateField dynamically updates a single column, equivalent to PHP's update() function.
func (r *UserRepository) UpdateField(chatID, column string, value interface{}) error {
	// Sanitize column name to prevent SQL injection
	allowedColumns := map[string]bool{
		"step": true, "Balance": true, "User_Status": true, "verify": true,
		"agent": true, "affiliates": true, "affiliatescount": true,
		"Processing_value": true, "Processing_value_one": true,
		"Processing_value_tow": true, "Processing_value_four": true,
		"number": true, "namecustom": true, "pagenumber": true,
		"message_count": true, "last_message_time": true, "cardpayment": true,
		"limit_usertest": true, "maxbuyagent": true, "joinchannel": true,
		"checkstatus": true, "bottype": true, "score": true,
		"limitchangeloc": true, "status_cron": true, "expire": true, "token": true,
		"codeInvitation": true, "pricediscount": true, "description_blocking": true,
		"roll_Status": true, "register": true, "number_username": true,
		"hide_mini_app_instruction": true,
	}
	col := strings.TrimSpace(column)
	if !allowedColumns[col] {
		return fmt.Errorf("column %q is not allowed", col)
	}
	return r.db.Model(&models.User{}).Where("id = ?", chatID).Update(col, value).Error
}
