package api

import (
	"database/sql"
	"fmt"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/models"
	"hosibot/internal/pkg/telegram"
	"hosibot/internal/pkg/utils"
)

// UserHandler handles all user API actions.
// Matches PHP api/users.php behavior exactly.
type UserHandler struct {
	repos  *Repos
	botAPI *telegram.BotAPI
	logger *zap.Logger
}

func NewUserHandler(repos *Repos, botAPI *telegram.BotAPI, logger *zap.Logger) *UserHandler {
	return &UserHandler{repos: repos, botAPI: botAPI, logger: logger}
}

// Handle routes user API requests based on the "actions" field.
// POST /api/users
func (h *UserHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "users":
		return h.listUsers(c, body)
	case "user":
		return h.getUser(c, body)
	case "user_add":
		return h.addUser(c, body)
	case "block_user":
		return h.blockUser(c, body)
	case "verify_user":
		return h.verifyUser(c, body)
	case "change_status_user":
		return h.changeStatusUser(c, body)
	case "add_balance":
		return h.addBalance(c, body)
	case "withdrawal":
		return h.withdrawal(c, body)
	case "accept_number":
		return h.acceptNumber(c, body)
	case "send_message":
		return h.sendMessage(c, body)
	case "set_limit_test":
		return h.setLimitTest(c, body)
	case "transfer_account":
		return h.transferAccount(c, body)
	case "join_channel_exception":
		return h.joinChannelException(c, body)
	case "cron_notif":
		return h.cronNotif(c, body)
	case "manage_show_cart":
		return h.manageShowCart(c, body)
	case "zero_balance":
		return h.zeroBalance(c, body)
	case "affiliates_users":
		return h.affiliatesUsers(c, body)
	case "remove_affiliates":
		return h.removeAffiliates(c, body)
	case "remove_affiliate_user":
		return h.removeAffiliateUser(c, body)
	case "set_agent":
		return h.setAgent(c, body)
	case "set_expire_agent":
		return h.setExpireAgent(c, body)
	case "set_becoming_negative":
		return h.setBecomingNegative(c, body)
	case "set_percentage_discount":
		return h.setPercentageDiscount(c, body)
	case "active_bot_agent":
		return h.activeBotAgent(c, body)
	case "remove_agent_bot":
		return h.removeAgentBot(c, body)
	case "set_price_volume_agent_bot":
		return h.setPriceVolumeAgentBot(c, body)
	case "set_price_time_agent_bot":
		return h.setPriceTimeAgentBot(c, body)
	case "SetPanelAgentShow":
		return h.setPanelAgentShow(c, body)
	case "SetLimitChangeLocation":
		return h.setLimitChangeLocation(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

// listUsers - action: "users"
func (h *UserHandler) listUsers(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")
	agent := getStringField(body, "agent")

	users, total, err := h.repos.User.FindAll(limit, page, q, agent)
	if err != nil {
		h.logger.Error("Failed to list users", zap.Error(err))
		return errorResponse(c, "Failed to retrieve users")
	}

	return successResponse(c, "Successful", paginatedResponse(users, total, page, limit))
}

// getUser - action: "user"
func (h *UserHandler) getUser(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	user, err := h.repos.User.FindByID(chatID)
	if err != nil {
		return errorResponse(c, "User not found")
	}

	// Get additional stats (matching PHP behavior)
	invoiceCount, _ := h.repos.Invoice.CountByUserID(chatID)
	activeInvoices, _ := h.repos.Invoice.CountActiveByUserID(chatID)
	paymentCount, _ := h.repos.Payment.CountByUserID(chatID)
	paymentSum, _ := h.repos.Payment.SumByUserID(chatID)

	// Get available panels and products
	panels, _ := h.repos.Panel.FindActive()
	products, _, _ := h.repos.Product.FindAll(0, 1, "")

	result := map[string]interface{}{
		"user":            user,
		"invoice_count":   invoiceCount,
		"active_invoices": activeInvoices,
		"payment_count":   paymentCount,
		"payment_sum":     paymentSum,
		"panels":          panels,
		"products":        products,
	}

	return successResponse(c, "Successful", result)
}

// addUser - action: "user_add"
func (h *UserHandler) addUser(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	// Check if user exists
	exists, _ := h.repos.User.Exists(chatID)
	if exists {
		return errorResponse(c, "User already exists")
	}

	// Get user info from Telegram
	_, _ = h.botAPI.GetChat(chatID)

	user := &models.User{
		ID:          chatID,
		Step:        "none",
		UserStatus:  "active",
		Balance:     0,
		Register:    utils.NowUnix(),
		Verify:      "0",
		Agent:       "0",
		CardPayment: "0",
	}

	if err := h.repos.User.Create(user); err != nil {
		h.logger.Error("Failed to create user", zap.Error(err))
		return errorResponse(c, "Failed to create user")
	}

	return successResponse(c, "User created successfully", user)
}

// blockUser - action: "block_user"
func (h *UserHandler) blockUser(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	description := getStringField(body, "description")
	typeBlock := getStringField(body, "type_block")

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Block(chatID, description, typeBlock); err != nil {
		return errorResponse(c, "Failed to update user status")
	}

	msg := "User blocked successfully"
	if typeBlock == "unblock" {
		msg = "User unblocked successfully"
	}
	return successResponse(c, msg, nil)
}

// verifyUser - action: "verify_user"
func (h *UserHandler) verifyUser(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	typeVerify := getStringField(body, "type_verify")

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.SetVerify(chatID, typeVerify); err != nil {
		return errorResponse(c, "Failed to update verify status")
	}
	return successResponse(c, "Successful", nil)
}

// changeStatusUser - action: "change_status_user"
func (h *UserHandler) changeStatusUser(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	statusType := getStringField(body, "type")

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"User_Status": statusType}); err != nil {
		return errorResponse(c, "Failed to update user status")
	}
	return successResponse(c, "Successful", nil)
}

// addBalance - action: "add_balance"
func (h *UserHandler) addBalance(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	amount := getIntField(body, "amount", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}
	if amount <= 0 {
		return errorResponse(c, "Amount must be greater than 0")
	}

	if err := h.repos.User.UpdateBalance(chatID, amount); err != nil {
		return errorResponse(c, "Failed to add balance")
	}
	return successResponse(c, "Balance added successfully", nil)
}

// withdrawal - action: "withdrawal"
func (h *UserHandler) withdrawal(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	amount := getIntField(body, "amount", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}
	if amount <= 0 {
		return errorResponse(c, "Amount must be greater than 0")
	}

	if err := h.repos.User.UpdateBalance(chatID, -amount); err != nil {
		return errorResponse(c, "Failed to withdraw balance")
	}
	return successResponse(c, "Balance withdrawn successfully", nil)
}

// acceptNumber - action: "accept_number"
func (h *UserHandler) acceptNumber(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"verify": "1"}); err != nil {
		return errorResponse(c, "Failed to accept number")
	}
	return successResponse(c, "Successful", nil)
}

// sendMessage - action: "send_message"
func (h *UserHandler) sendMessage(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	text := getStringField(body, "text")
	file := getStringField(body, "file")
	contentType := getStringField(body, "content_type")

	if chatID == "" || text == "" {
		return errorResponse(c, "chat_id and text are required")
	}

	if file != "" && contentType != "" {
		// Handle file sending based on content type
		switch contentType {
		case "image":
			_, err := h.botAPI.SendPhotoBase64(chatID, file, text)
			if err != nil {
				return errorResponse(c, "Failed to send image: "+err.Error())
			}
		default:
			_, err := h.botAPI.SendMessage(chatID, text, nil)
			if err != nil {
				return errorResponse(c, "Failed to send message: "+err.Error())
			}
		}
	} else {
		_, err := h.botAPI.SendMessage(chatID, text, nil)
		if err != nil {
			return errorResponse(c, "Failed to send message: "+err.Error())
		}
	}

	return successResponse(c, "Message sent successfully", nil)
}

// setLimitTest - action: "set_limit_test"
func (h *UserHandler) setLimitTest(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	limitTest := getIntField(body, "limit_test", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"limit_usertest": limitTest}); err != nil {
		return errorResponse(c, "Failed to set limit test")
	}
	return successResponse(c, "Successful", nil)
}

// transferAccount - action: "transfer_account"
func (h *UserHandler) transferAccount(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	newUserID := getStringField(body, "new_userid")

	if chatID == "" || newUserID == "" {
		return errorResponse(c, "chat_id and new_userid are required")
	}

	if err := h.repos.User.TransferAccount(chatID, newUserID); err != nil {
		return errorResponse(c, "Failed to transfer account: "+err.Error())
	}
	return successResponse(c, "Account transferred successfully", nil)
}

// joinChannelException - action: "join_channel_exception"
func (h *UserHandler) joinChannelException(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"joinchannel": "1"}); err != nil {
		return errorResponse(c, "Failed to set join channel exception")
	}
	return successResponse(c, "Successful", nil)
}

// cronNotif - action: "cron_notif"
func (h *UserHandler) cronNotif(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	t := getStringField(body, "type")

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"status_cron": t}); err != nil {
		return errorResponse(c, "Failed to update cron notification")
	}
	return successResponse(c, "Successful", nil)
}

// manageShowCart - action: "manage_show_cart"
func (h *UserHandler) manageShowCart(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	t := getStringField(body, "type")

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"cardpayment": t}); err != nil {
		return errorResponse(c, "Failed to update show cart")
	}
	return successResponse(c, "Successful", nil)
}

// zeroBalance - action: "zero_balance"
func (h *UserHandler) zeroBalance(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.SetBalance(chatID, 0); err != nil {
		return errorResponse(c, "Failed to zero balance")
	}
	return successResponse(c, "Successful", nil)
}

// affiliatesUsers - action: "affiliates_users"
func (h *UserHandler) affiliatesUsers(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	users, err := h.repos.User.FindAffiliates(chatID)
	if err != nil {
		return errorResponse(c, "Failed to get affiliates")
	}
	return successResponse(c, "Successful", users)
}

// removeAffiliates - action: "remove_affiliates"
func (h *UserHandler) removeAffiliates(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"affiliates": "0", "affiliatescount": "0"}); err != nil {
		return errorResponse(c, "Failed to remove affiliates")
	}
	return successResponse(c, "Successful", nil)
}

// removeAffiliateUser - action: "remove_affiliate_user"
func (h *UserHandler) removeAffiliateUser(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"affiliates": "0"}); err != nil {
		return errorResponse(c, "Failed to remove affiliate user")
	}
	return successResponse(c, "Successful", nil)
}

// setAgent - action: "set_agent"
func (h *UserHandler) setAgent(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	agentType := getStringField(body, "agent_type")

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{"agent": agentType}); err != nil {
		return errorResponse(c, "Failed to set agent")
	}
	return successResponse(c, "Successful", nil)
}

// setExpireAgent - action: "set_expire_agent"
func (h *UserHandler) setExpireAgent(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	expireTime := getIntField(body, "expire_time", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	expire := fmt.Sprintf("%d", expireTime)
	if err := h.repos.User.Update(chatID, map[string]interface{}{"expire": sql.NullString{String: expire, Valid: true}}); err != nil {
		return errorResponse(c, "Failed to set expire agent")
	}
	return successResponse(c, "Successful", nil)
}

// setBecomingNegative - action: "set_becoming_negative"
func (h *UserHandler) setBecomingNegative(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	amount := getIntField(body, "amount", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"maxbuyagent": sql.NullString{String: fmt.Sprintf("%d", amount), Valid: true},
	}); err != nil {
		return errorResponse(c, "Failed to set becoming negative")
	}
	return successResponse(c, "Successful", nil)
}

// setPercentageDiscount - action: "set_percentage_discount"
func (h *UserHandler) setPercentageDiscount(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	percentage := getIntField(body, "percentage", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"pricediscount": sql.NullString{String: fmt.Sprintf("%d", percentage), Valid: true},
	}); err != nil {
		return errorResponse(c, "Failed to set percentage discount")
	}
	return successResponse(c, "Successful", nil)
}

// activeBotAgent - action: "active_bot_agent"
func (h *UserHandler) activeBotAgent(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	token := getStringField(body, "token")

	if chatID == "" || token == "" {
		return errorResponse(c, "chat_id and token are required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"token": sql.NullString{String: token, Valid: true},
	}); err != nil {
		return errorResponse(c, "Failed to activate bot agent")
	}
	return successResponse(c, "Successful", nil)
}

// removeAgentBot - action: "remove_agent_bot"
func (h *UserHandler) removeAgentBot(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"token": sql.NullString{Valid: false},
	}); err != nil {
		return errorResponse(c, "Failed to remove agent bot")
	}
	return successResponse(c, "Successful", nil)
}

// setPriceVolumeAgentBot - action: "set_price_volume_agent_bot"
func (h *UserHandler) setPriceVolumeAgentBot(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	amount := getIntField(body, "amount", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"Processing_value": fmt.Sprintf("%d", amount),
	}); err != nil {
		return errorResponse(c, "Failed to set price volume agent bot")
	}
	return successResponse(c, "Successful", nil)
}

// setPriceTimeAgentBot - action: "set_price_time_agent_bot"
func (h *UserHandler) setPriceTimeAgentBot(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	amount := getIntField(body, "amount", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"Processing_value_one": fmt.Sprintf("%d", amount),
	}); err != nil {
		return errorResponse(c, "Failed to set price time agent bot")
	}
	return successResponse(c, "Successful", nil)
}

// setPanelAgentShow - action: "SetPanelAgentShow"
func (h *UserHandler) setPanelAgentShow(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	panels := body["panels"]

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"Processing_value_four": fmt.Sprintf("%v", panels),
	}); err != nil {
		return errorResponse(c, "Failed to set panel agent show")
	}
	return successResponse(c, "Successful", nil)
}

// setLimitChangeLocation - action: "SetLimitChangeLocation"
func (h *UserHandler) setLimitChangeLocation(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	limit := getIntField(body, "Limit", 0)

	if chatID == "" {
		return errorResponse(c, "chat_id is required")
	}

	if err := h.repos.User.Update(chatID, map[string]interface{}{
		"limitchangeloc": sql.NullString{String: fmt.Sprintf("%d", limit), Valid: true},
	}); err != nil {
		return errorResponse(c, "Failed to set limit change location")
	}
	return successResponse(c, "Successful", nil)
}
