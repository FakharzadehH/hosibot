package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"hosibot/internal/config"
	"hosibot/internal/models"
	"hosibot/internal/panel"
	"hosibot/internal/payment"
	"hosibot/internal/pkg/httpclient"
	"hosibot/internal/pkg/telegram"
	"hosibot/internal/pkg/utils"
	"hosibot/internal/repository"
)

// Bot wraps the telebot instance and handlers.
type Bot struct {
	tb         *tele.Bot
	webhook    *tele.Webhook
	useWebhook bool
	cfg        *config.Config
	repos      *BotRepos
	logger     *zap.Logger
	panels     map[string]panel.PanelClient
	keyboard   *KeyboardBuilder
	botAPI     *telegram.BotAPI
}

// BotRepos bundles all repositories needed by bot handlers.
type BotRepos struct {
	User    *repository.UserRepository
	Product *repository.ProductRepository
	Invoice *repository.InvoiceRepository
	Payment *repository.PaymentRepository
	Panel   *repository.PanelRepository
	Setting *repository.SettingRepository
}

// New creates and configures a new Bot instance.
func New(cfg *config.Config, repos *BotRepos, botAPI *telegram.BotAPI, logger *zap.Logger) (*Bot, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Bot.UpdateMode))
	if mode == "" {
		mode = "auto"
	}

	useWebhook := true
	switch mode {
	case "polling":
		useWebhook = false
	case "webhook":
		useWebhook = true
	default: // auto
		useWebhook = strings.TrimSpace(cfg.Bot.WebhookURL) != ""
	}

	var poller tele.Poller
	var webhook *tele.Webhook
	if useWebhook {
		if strings.TrimSpace(cfg.Bot.WebhookURL) == "" {
			return nil, fmt.Errorf("BOT_WEBHOOK_URL is required when BOT_UPDATE_MODE=webhook")
		}
		webhook = &tele.Webhook{
			Listen:   "", // Empty: we mount on Echo instead of telebot's own server
			Endpoint: &tele.WebhookEndpoint{PublicURL: cfg.Bot.WebhookURL},
		}
		poller = webhook
	} else {
		poller = &tele.LongPoller{Timeout: 10 * time.Second}
	}

	pref := tele.Settings{
		Token:  cfg.Bot.Token,
		Poller: poller,
		OnError: func(err error, c tele.Context) {
			logger.Error("telebot error", zap.Error(err))
		},
	}

	tb, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("failed to create telebot: %w", err)
	}

	b := &Bot{
		tb:         tb,
		webhook:    webhook,
		useWebhook: useWebhook,
		cfg:        cfg,
		repos:      repos,
		logger:     logger,
		panels:     make(map[string]panel.PanelClient),
		keyboard:   NewKeyboardBuilder(repos),
		botAPI:     botAPI,
	}

	b.registerHandlers()

	return b, nil
}

// GetTelebot returns the underlying telebot instance for webhook integration.
func (b *Bot) GetTelebot() *tele.Bot {
	return b.tb
}

// WebhookHandler returns the webhook handler for mounting on Echo.
// Returns nil when running in long-polling mode.
func (b *Bot) WebhookHandler() http.Handler {
	return b.webhook
}

// Start begins polling/webhook processing.
func (b *Bot) Start() {
	if b.useWebhook {
		b.logger.Info("Starting Telegram bot", zap.String("mode", "webhook"), zap.String("webhook_url", b.cfg.Bot.WebhookURL))
	} else {
		// Long polling requires webhook to be removed first.
		if err := b.tb.RemoveWebhook(true); err != nil {
			b.logger.Warn("Failed to remove webhook before long polling", zap.Error(err))
		}
		b.logger.Info("Starting Telegram bot", zap.String("mode", "polling"))
	}
	b.tb.Start()
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() {
	b.tb.Stop()
}

// registerHandlers sets up all bot message and callback handlers.
func (b *Bot) registerHandlers() {
	b.tb.Handle("/start", b.handleStart)
	b.tb.Handle(tele.OnText, b.handleText)
	b.tb.Handle(tele.OnContact, b.handleContact)
	b.tb.Handle(tele.OnCallback, b.handleCallback)
	b.tb.Handle(tele.OnPhoto, b.handlePhoto)
}

// ── /start ────────────────────────────────────────────────────────────

func (b *Bot) handleStart(c tele.Context) error {
	chatID := fmt.Sprintf("%d", c.Chat().ID)

	user, err := b.repos.User.FindByID(chatID)
	if err != nil {
		newUser := &models.User{
			ID:         chatID,
			Step:       "none",
			UserStatus: "active",
			Register:   utils.NowUnix(),
			Agent:      "0",
		}

		payload := c.Message().Payload
		if payload != "" && strings.HasPrefix(payload, "ref_") {
			referrer := strings.TrimPrefix(payload, "ref_")
			newUser.Affiliates = referrer
			_ = b.repos.User.UpdateField(referrer, "affiliatescount", "affiliatescount + 1")
		}

		if err := b.repos.User.Create(newUser); err != nil {
			b.logger.Error("Failed to create user", zap.String("chat_id", chatID), zap.Error(err))
		}
		user = newUser
	}

	_ = b.repos.User.UpdateStep(chatID, "none")

	if strings.EqualFold(user.UserStatus, "blocked") || strings.EqualFold(user.UserStatus, "block") {
		return c.Send("⛔ حساب شما مسدود شده است.")
	}

	// Check phone verification requirement
	setting, _ := b.repos.Setting.GetSettings()
	if setting != nil && setting.GetNumber == "on_number" && user.Number == "" {
		_ = b.repos.User.UpdateStep(chatID, "get_number")
		menu := &tele.ReplyMarkup{ResizeKeyboard: true}
		menu.Reply(menu.Row(menu.Contact("📱 ارسال شماره تماس")))
		return c.Send("لطفاً شماره تماس خود را ارسال کنید:", menu)
	}

	return b.sendMainMenu(c, chatID)
}

// ── Text routing ──────────────────────────────────────────────────────

func (b *Bot) handleText(c tele.Context) error {
	chatID := fmt.Sprintf("%d", c.Chat().ID)

	user, err := b.repos.User.FindByID(chatID)
	if err != nil {
		return c.Send("لطفاً از /start استفاده کنید.")
	}

	if strings.EqualFold(user.UserStatus, "blocked") || strings.EqualFold(user.UserStatus, "block") {
		return c.Send("⛔ حساب شما مسدود شده است.")
	}

	if !b.checkRateLimit(user) {
		return nil
	}

	text := c.Message().Text

	switch user.Step {
	case "none":
		return b.handleMainMenu(c, user, text)
	case "buy_service":
		return b.handleBuyService(c, user, text)
	case "select_location":
		return b.handleSelectLocation(c, user, text)
	case "select_product":
		return b.handleSelectProduct(c, user, text)
	case "charge_wallet", "getprice":
		return b.handleChargeWallet(c, user, text)
	case "my_services":
		return b.handleMyServices(c, user, text)
	case "support":
		return b.handleSupport(c, user, text)
	case "gettextticket":
		return b.handleSupportMessage(c, user, text)
	case "statusnamecustom":
		return b.handleCustomName(c, user, text)
	case "get_code_user":
		return b.handleDiscountCode(c, user, text)
	case "get_number":
		return c.Send("لطفاً شماره تماس خود را با دکمه زیر ارسال کنید.")
	default:
		if strings.HasPrefix(user.Step, "extend_") {
			return b.handleExtendService(c, user, text)
		}
		if strings.HasPrefix(user.Step, "pay_") || user.Step == "get_step_payment" {
			return b.handlePayment(c, user, text)
		}
		if user.Step == "cart_to_cart_user" {
			return c.Send("لطفاً تصویر رسید خود را ارسال کنید.")
		}
		return b.sendMainMenu(c, chatID)
	}
}

// ── Contact ───────────────────────────────────────────────────────────

func (b *Bot) handleContact(c tele.Context) error {
	chatID := fmt.Sprintf("%d", c.Chat().ID)
	contact := c.Message().Contact
	if contact == nil {
		return nil
	}

	if fmt.Sprintf("%d", contact.UserID) != chatID {
		return c.Send("⚠️ لطفاً شماره خودتان را ارسال کنید.")
	}

	_ = b.repos.User.Update(chatID, map[string]interface{}{
		"number": contact.PhoneNumber,
		"verify": "1",
	})
	_ = b.repos.User.UpdateStep(chatID, "none")

	return b.sendMainMenu(c, chatID)
}

// ── Callback queries ──────────────────────────────────────────────────

func (b *Bot) handleCallback(c tele.Context) error {
	chatID := fmt.Sprintf("%d", c.Chat().ID)
	data := c.Callback().Data

	user, err := b.repos.User.FindByID(chatID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "لطفاً /start بزنید"})
	}

	_ = c.Respond()

	switch {
	case data == "main_menu":
		_ = b.repos.User.UpdateStep(chatID, "none")
		return b.sendMainMenu(c, chatID)
	case data == "admin_panel":
		return c.Send("لطفاً از پنل وب مدیریت استفاده کنید.")

	case data == "buy_service" || data == "buy":
		_ = b.repos.User.UpdateStep(chatID, "buy_service")
		return b.sendBuyMenu(c, user)

	case data == "my_services" || data == "myservice":
		return b.sendMyServices(c, user)

	case data == "wallet" || data == "account":
		return b.sendWalletInfo(c, user)

	case data == "charge_wallet" || data == "Add_Balance":
		return b.startChargeWallet(c, user)

	case data == "discount_code" || data == "Discount":
		_ = b.repos.User.UpdateStep(chatID, "get_code_user")
		return c.Send("🎁 کد تخفیف یا هدیه خود را وارد کنید:")

	case data == "support":
		return b.showSupportDepartments(c, user)

	case data == "referral":
		return b.showReferralInfo(c, user)

	// Location selection
	case strings.HasPrefix(data, "loc_"):
		return b.handleLocationSelect(c, user, data)

	// Category selection
	case strings.HasPrefix(data, "cat_"):
		return b.handleCategorySelect(c, user, data)

	// Product selection
	case strings.HasPrefix(data, "prod_"):
		return b.handleProductSelect(c, user, data)

	// Purchase confirmation
	case strings.HasPrefix(data, "confirm_"):
		return b.handlePurchaseConfirm(c, user, data)

	// Service detail
	case strings.HasPrefix(data, "srv_"):
		return b.handleServiceSelect(c, user, data)

	// Service pagination
	case strings.HasPrefix(data, "srvpage_"):
		return b.handleServicePage(c, user, data)

	// Service actions
	case strings.HasPrefix(data, "updateinfo_"):
		return b.handleServiceSelect(c, user, strings.Replace(data, "updateinfo_", "srv_", 1))
	case strings.HasPrefix(data, "togglesvc_"):
		return b.handleToggleServiceStatus(c, user, data)
	case strings.HasPrefix(data, "resettraffic_"):
		return b.handleResetServiceTraffic(c, user, data)
	case strings.HasPrefix(data, "suburl_"):
		return b.handleSubscriptionURL(c, user, data)
	case strings.HasPrefix(data, "configs_"):
		return b.handleShowConfigs(c, user, data)

	// Extend
	case strings.HasPrefix(data, "extend_"):
		return b.handleExtendCallback(c, user, data)
	case strings.HasPrefix(data, "extprod_"):
		return b.handleExtendProductSelect(c, user, data)

	// Payment methods
	case strings.HasPrefix(data, "pay_"):
		return b.handlePaymentMethod(c, user, data)

	// Card payment receipt
	case strings.HasPrefix(data, "sendreceipt_"):
		orderID := strings.TrimPrefix(data, "sendreceipt_")
		_ = b.repos.User.UpdateStep(chatID, "cart_to_cart_user")
		_ = b.repos.User.Update(chatID, map[string]interface{}{
			"Processing_value_one": orderID,
		})
		return c.Send("📸 لطفاً تصویر رسید پرداخت خود را ارسال کنید:")

	// Admin payment confirm/reject
	case strings.HasPrefix(data, "confirmpay_"):
		return b.handleAdminPaymentConfirm(c, user, data)
	case strings.HasPrefix(data, "rejectpay_"):
		return b.handleAdminPaymentReject(c, user, data)

	// Support department selection
	case strings.HasPrefix(data, "dept_"):
		return b.handleDepartmentSelect(c, user, data)

	// Remove service
	case strings.HasPrefix(data, "removeservice_"):
		return b.handleRemoveService(c, user, data)

	// Change link
	case strings.HasPrefix(data, "changelink_"):
		return b.handleChangeLink(c, user, data)

	default:
		b.logger.Debug("Unknown callback", zap.String("data", data), zap.String("user", chatID))
		return nil
	}
}

// ── Photo handler ─────────────────────────────────────────────────────

func (b *Bot) handlePhoto(c tele.Context) error {
	chatID := fmt.Sprintf("%d", c.Chat().ID)
	user, err := b.repos.User.FindByID(chatID)
	if err != nil {
		return nil
	}

	if user.Step == "cart_to_cart_user" {
		return b.handlePaymentReceipt(c, user)
	}

	return nil
}

// ── Rate limit ────────────────────────────────────────────────────────

func (b *Bot) checkRateLimit(user *models.User) bool {
	now := time.Now().Unix()
	lastMsg := utils.ParseInt64(user.LastMessageTime, 0)

	if now-lastMsg > 60 {
		_ = b.repos.User.Update(user.ID, map[string]interface{}{
			"message_count":     "1",
			"last_message_time": fmt.Sprintf("%d", now),
		})
		return true
	}

	count := utils.ParseInt(user.MessageCount, 0)
	if count > 30 {
		return false
	}

	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"message_count":     fmt.Sprintf("%d", count+1),
		"last_message_time": fmt.Sprintf("%d", now),
	})
	return true
}

// ── Main Menu ─────────────────────────────────────────────────────────

func (b *Bot) sendMainMenu(c tele.Context, chatID string) error {
	user, _ := b.repos.User.FindByID(chatID)
	setting, _ := b.repos.Setting.GetSettings()

	var menu *tele.ReplyMarkup
	if user != nil && setting != nil {
		menu = b.keyboard.MainMenuKeyboard(user, setting)
	} else {
		menu = &tele.ReplyMarkup{ResizeKeyboard: true}
		menu.Reply(
			menu.Row(menu.Text("🛒 خرید سرویس"), menu.Text("📋 سرویس‌های من")),
			menu.Row(menu.Text("💰 کیف پول"), menu.Text("📩 پشتیبانی")),
		)
	}

	startText, _ := b.repos.Setting.GetText("textstart")
	if startText == "" {
		startText = "🏠 منوی اصلی"
	}

	return c.Send(startText, menu)
}

func (b *Bot) handleMainMenu(c tele.Context, user *models.User, text string) error {
	// Load text labels to match against
	buyText, _ := b.repos.Setting.GetText("text_sell")
	servicesText, _ := b.repos.Setting.GetText("text_Purchased_services")
	walletText, _ := b.repos.Setting.GetText("accountwallet")
	supportText, _ := b.repos.Setting.GetText("text_support")
	extendText, _ := b.repos.Setting.GetText("text_extend")
	affiliateText, _ := b.repos.Setting.GetText("text_affiliates")
	accountText, _ := b.repos.Setting.GetText("text_Account")

	switch {
	case matchText(text, buyText, "خرید", "buy", "🛒"):
		_ = b.repos.User.UpdateStep(user.ID, "buy_service")
		return b.sendBuyMenu(c, user)
	case matchText(text, servicesText, "سرویس", "services", "📋"):
		return b.sendMyServices(c, user)
	case matchText(text, walletText, "کیف پول", "wallet", "💰"):
		return b.sendWalletInfo(c, user)
	case matchText(text, supportText, "پشتیبانی", "support", "📩"):
		return b.showSupportDepartments(c, user)
	case matchText(text, extendText, "تمدید", "extend", "🔁"):
		return b.sendMyServices(c, user)
	case matchText(text, affiliateText, "معرفی", "referral", "📢"):
		return b.showReferralInfo(c, user)
	case matchText(text, accountText, "حساب", "account", "👤"):
		return b.sendWalletInfo(c, user)
	case text == "🔧 پنل مدیریت":
		return c.Send("لطفاً از پنل وب مدیریت استفاده کنید.")
	default:
		return b.sendMainMenu(c, user.ID)
	}
}

// matchText checks if text matches the label or any of the fallback substrings.
func matchText(text, label string, fallbacks ...string) bool {
	lower := strings.ToLower(text)
	if label != "" && strings.Contains(lower, strings.ToLower(label)) {
		return true
	}
	for _, fb := range fallbacks {
		if strings.Contains(lower, strings.ToLower(fb)) {
			return true
		}
	}
	return false
}

// ── Buy Flow ──────────────────────────────────────────────────────────

func (b *Bot) sendBuyMenu(c tele.Context, user *models.User) error {
	setting, _ := b.repos.Setting.GetSettings()

	// Check category mode
	useCategory := false
	if setting != nil && (setting.StatusCategory == "oncategory" || setting.StatusCategory == "oncategorys") {
		useCategory = true
	}

	panels, err := b.repos.Panel.FindActive()
	if err != nil || len(panels) == 0 {
		return c.Send("⚠️ در حال حاضر سرویسی موجود نیست.")
	}

	// Filter by agent access
	var accessiblePanels []models.Panel
	for _, p := range panels {
		if b.keyboard.panelAccessibleByUser(p, user) {
			accessiblePanels = append(accessiblePanels, p)
		}
	}

	if len(accessiblePanels) == 0 {
		return c.Send("⚠️ سرویسی برای شما موجود نیست.")
	}

	// If only one panel, skip location selection
	if len(accessiblePanels) == 1 {
		p := accessiblePanels[0]
		_ = b.repos.User.Update(user.ID, map[string]interface{}{
			"Processing_value_four": p.CodePanel,
		})

		if useCategory {
			kb := b.keyboard.CategoryKeyboard(p.CodePanel)
			return c.Send("📂 دسته‌بندی مورد نظر را انتخاب کنید:", kb)
		}

		kb := b.keyboard.ProductKeyboard(p.CodePanel, user, 0)
		return c.Send("📦 محصول مورد نظر را انتخاب کنید:", kb)
	}

	// Show location selection
	kb := b.keyboard.LocationKeyboard(user)
	buyMenuText, _ := b.repos.Setting.GetText("textlistpanel")
	if buyMenuText == "" {
		buyMenuText = "📍 لوکیشن مورد نظر را انتخاب کنید:"
	}
	return c.Send(buyMenuText, kb)
}

func (b *Bot) handleBuyService(c tele.Context, user *models.User, text string) error {
	// Text-based product selection is handled via callbacks in this implementation
	return b.sendBuyMenu(c, user)
}

func (b *Bot) handleSelectLocation(c tele.Context, user *models.User, text string) error {
	return b.sendBuyMenu(c, user)
}

func (b *Bot) handleSelectProduct(c tele.Context, user *models.User, text string) error {
	return b.sendBuyMenu(c, user)
}

func (b *Bot) handleLocationSelect(c tele.Context, user *models.User, data string) error {
	panelCode := strings.TrimPrefix(data, "loc_")

	panel, err := b.repos.Panel.FindByCode(panelCode)
	if err != nil {
		return c.Send("⚠️ لوکیشن یافت نشد.")
	}

	// Check capacity
	activeCount, _ := b.repos.Invoice.CountByLocation(panel.NamePanel)
	limitPanel := parseIntSafe(panel.LimitPanel)
	if limitPanel > 0 && int(activeCount) >= limitPanel {
		return c.Send("⚠️ ظرفیت این لوکیشن تکمیل شده است.")
	}

	// Store selected panel
	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"Processing_value_four": panelCode,
	})

	setting, _ := b.repos.Setting.GetSettings()
	useCategory := setting != nil && (setting.StatusCategory == "oncategory" || setting.StatusCategory == "oncategorys")

	if useCategory {
		kb := b.keyboard.CategoryKeyboard(panelCode)
		return c.Send("📂 دسته‌بندی مورد نظر را انتخاب کنید:", kb)
	}

	kb := b.keyboard.ProductKeyboard(panelCode, user, 0)
	return c.Send("📦 محصول مورد نظر را انتخاب کنید:", kb)
}

func (b *Bot) handleCategorySelect(c tele.Context, user *models.User, data string) error {
	// Format: cat_<panelCode>_<categoryID>
	parts := strings.SplitN(strings.TrimPrefix(data, "cat_"), "_", 2)
	if len(parts) != 2 {
		return c.Send("⚠️ خطا در انتخاب دسته‌بندی.")
	}

	panelCode := parts[0]
	categoryID := parseIntSafe(parts[1])

	kb := b.keyboard.ProductKeyboard(panelCode, user, categoryID)
	return c.Send("📦 محصول مورد نظر را انتخاب کنید:", kb)
}

func (b *Bot) handleProductSelect(c tele.Context, user *models.User, data string) error {
	productIDStr := strings.TrimPrefix(data, "prod_")
	productID := parseIntSafe(productIDStr)

	product, err := b.repos.Product.FindByID(productID)
	if err != nil {
		return c.Send("⚠️ محصول یافت نشد.")
	}

	// Reload user to get latest Processing_value_four (panel code)
	user, _ = b.repos.User.FindByID(user.ID)
	panelCode := user.ProcessingValueFour

	panel, err := b.repos.Panel.FindByCode(panelCode)
	if err != nil {
		return c.Send("⚠️ لوکیشن یافت نشد.")
	}

	// Calculate price with discount
	price := parseIntSafe(product.PriceProduct)
	discountPct := 0
	if user.PriceDiscount.Valid && user.PriceDiscount.String != "" && user.PriceDiscount.String != "0" {
		discountPct = parseIntSafe(user.PriceDiscount.String)
		price = price - (price * discountPct / 100)
	}

	// Generate username
	username := b.generateUsername(panel, user)

	// Store purchase context
	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"Processing_value":     fmt.Sprintf("%d", price),
		"Processing_value_one": username,
		"Processing_value_tow": product.CodeProduct,
		"step":                 "confirm_purchase",
	})

	// Build invoice preview
	invoiceText := fmt.Sprintf(
		"🧾 <b>پیش فاکتور</b>\n\n"+
			"📦 محصول: %s\n"+
			"📍 لوکیشن: %s\n"+
			"💾 حجم: %s گیگابایت\n"+
			"⏱ مدت: %s روز\n"+
			"💰 قیمت: %s تومان\n"+
			"👛 موجودی کیف پول: %s تومان\n"+
			"👤 نام کاربری: %s",
		product.NameProduct,
		panel.NamePanel,
		product.VolumeConstraint,
		product.ServiceTime,
		formatNumber(price),
		formatNumber(user.Balance),
		username,
	)

	// Temporary invoice ID for confirmation
	tempID := fmt.Sprintf("%s_%s_%d", panelCode, product.CodeProduct, time.Now().Unix())

	kb := b.keyboard.ConfirmPurchaseKeyboard(tempID)
	return c.Send(invoiceText, kb, tele.ModeHTML)
}

func (b *Bot) handlePurchaseConfirm(c tele.Context, user *models.User, data string) error {
	chatID := user.ID

	// Reload user state
	user, _ = b.repos.User.FindByID(chatID)
	price := parseIntSafe(user.ProcessingValue)
	username := user.ProcessingValueOne
	productCode := user.ProcessingValueTwo
	panelCode := user.ProcessingValueFour

	if username == "" || productCode == "" || panelCode == "" {
		return c.Send("⚠️ اطلاعات خرید یافت نشد. لطفاً دوباره تلاش کنید.")
	}

	product, err := b.repos.Product.FindByCode(productCode)
	if err != nil {
		return c.Send("⚠️ محصول یافت نشد.")
	}

	panelModel, err := b.repos.Panel.FindByCode(panelCode)
	if err != nil {
		return c.Send("⚠️ لوکیشن یافت نشد.")
	}

	// Check balance
	if user.Balance < price {
		deficit := price - user.Balance
		// Redirect to payment
		_ = b.repos.User.Update(chatID, map[string]interface{}{
			"Processing_value":     fmt.Sprintf("%d", deficit),
			"Processing_value_tow": "getconfigafterpay",
			"Processing_value_one": username,
			"step":                 "get_step_payment",
		})

		payKB := b.keyboard.PaymentMethodKeyboard(deficit, user)
		return c.Send(
			fmt.Sprintf("💳 موجودی کیف پول شما کافی نیست.\nمبلغ مورد نیاز: %s تومان\n\nروش پرداخت را انتخاب کنید:", formatNumber(deficit)),
			payKB,
		)
	}

	// Sufficient balance — create service on panel
	return b.createServiceOnPanel(c, user, product, panelModel, username, price)
}

// createServiceOnPanel creates the VPN service and updates all records.
func (b *Bot) createServiceOnPanel(c tele.Context, user *models.User, product *models.Product, panelModel *models.Panel, username string, price int) error {
	chatID := user.ID
	ctx := context.Background()

	// Get/create panel client
	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		b.logger.Error("Failed to get panel client", zap.Error(err))
		return c.Send("⚠️ خطا در اتصال به پنل. لطفاً دوباره تلاش کنید.")
	}

	// Calculate expire and volume
	volumeGB := parseIntSafe(product.VolumeConstraint)
	dataLimit := int64(volumeGB) * 1024 * 1024 * 1024 // GB to bytes
	serviceDays := parseIntSafe(product.ServiceTime)
	if serviceDays > 0 {
		_ = time.Now().Unix() + int64(serviceDays)*86400 // expire calculated later in panel create
	}

	// Create user on panel
	createReq := panel.CreateUserRequest{
		Username:       username,
		DataLimit:      dataLimit,
		ExpireDays:     serviceDays,
		DataLimitReset: product.DataLimitReset,
		Note:           chatID,
	}

	// Parse inbounds
	if product.Inbounds != "" {
		var inbounds map[string][]string
		if json.Unmarshal([]byte(product.Inbounds), &inbounds) == nil {
			createReq.Inbounds = inbounds
		}
	}
	// Parse proxies
	if product.Proxies != "" {
		var proxies map[string]string
		if json.Unmarshal([]byte(product.Proxies), &proxies) == nil {
			createReq.Proxies = proxies
		}
	}

	panelUser, err := panelClient.CreateUser(ctx, createReq)
	if err != nil {
		b.logger.Error("Failed to create user on panel", zap.String("username", username), zap.Error(err))
		return c.Send("⚠️ خطا در ایجاد سرویس. لطفاً با پشتیبانی تماس بگیرید.")
	}

	// Create invoice
	invoiceID := fmt.Sprintf("%d", time.Now().UnixNano()/1e6)
	userInfoJSON, _ := json.Marshal(panelUser.Links)
	uuidJSON, _ := json.Marshal(panelUser.Proxies)

	invoice := &models.Invoice{
		IDInvoice:       invoiceID,
		IDUser:          chatID,
		Username:        username,
		ServiceLocation: panelModel.NamePanel,
		TimeSell:        fmt.Sprintf("%d", time.Now().Unix()),
		NameProduct:     product.NameProduct,
		PriceProduct:    fmt.Sprintf("%d", price),
		Volume:          product.VolumeConstraint,
		ServiceTime:     product.ServiceTime,
		UUID:            string(uuidJSON),
		Note:            "",
		UserInfo:        string(userInfoJSON),
		BotType:         "",
		Referral:        user.Affiliates,
		Notifications:   `{"volume":false,"time":false}`,
		Status:          "active",
	}

	if err := b.repos.Invoice.Create(invoice); err != nil {
		b.logger.Error("Failed to create invoice", zap.Error(err))
	}

	// Deduct balance
	_ = b.repos.User.UpdateBalance(chatID, -price)

	// Reset step
	_ = b.repos.User.UpdateStep(chatID, "none")
	_ = b.repos.User.Update(chatID, map[string]interface{}{
		"Processing_value":      "none",
		"Processing_value_one":  "none",
		"Processing_value_tow":  "none",
		"Processing_value_four": "none",
	})

	// Increment panel counter
	b.repos.Panel.IncrementCounter(panelModel.CodePanel)

	// Build subscription URL
	subURL := panelUser.SubLink
	if panelModel.SubVIP.Valid && panelModel.SubVIP.String == "onsubvip" {
		subURL = fmt.Sprintf("%s/sub/%s", b.cfg.Bot.WebhookURL, invoiceID)
	}

	// Send config to user
	configText := fmt.Sprintf(
		"✅ <b>سرویس شما با موفقیت ایجاد شد!</b>\n\n"+
			"📦 محصول: %s\n"+
			"📍 لوکیشن: %s\n"+
			"👤 نام کاربری: %s\n"+
			"💾 حجم: %s گیگابایت\n"+
			"⏱ مدت: %s روز\n"+
			"💰 قیمت: %s تومان\n\n"+
			"🔗 لینک اشتراک:\n<code>%s</code>\n\n"+
			"📋 برای مشاهده جزئیات از منوی «سرویس‌های من» استفاده کنید.",
		product.NameProduct,
		panelModel.NamePanel,
		username,
		product.VolumeConstraint,
		product.ServiceTime,
		formatNumber(price),
		subURL,
	)

	_ = c.Send(configText, tele.ModeHTML)

	// Handle affiliate commission
	b.handleAffiliateCommission(user, price)

	// Report to channel
	b.reportToChannel(fmt.Sprintf(
		"🛒 خرید جدید\n👤 کاربر: %s\n📦 محصول: %s\n📍 لوکیشن: %s\n💰 مبلغ: %s تومان",
		chatID, product.NameProduct, panelModel.NamePanel, formatNumber(price),
	))

	// Send main menu after
	_ = b.repos.User.UpdateStep(chatID, "none")
	return b.sendMainMenu(c, chatID)
}

// ── Wallet ────────────────────────────────────────────────────────────

func (b *Bot) sendWalletInfo(c tele.Context, user *models.User) error {
	// Count services
	activeCount, _ := b.repos.Invoice.CountActiveByUserID(user.ID)
	totalCount, _ := b.repos.Invoice.CountByUserID(user.ID)

	walletText := fmt.Sprintf(
		"👛 <b>کیف پول</b>\n\n"+
			"💰 موجودی: %s تومان\n"+
			"📊 سرویس‌های فعال: %d\n"+
			"📋 کل سرویس‌ها: %d\n"+
			"🆔 شناسه: <code>%s</code>",
		formatNumber(user.Balance),
		activeCount,
		totalCount,
		user.ID,
	)

	kb := b.keyboard.WalletKeyboard(user)
	return c.Send(walletText, kb, tele.ModeHTML)
}

func (b *Bot) startChargeWallet(c tele.Context, user *models.User) error {
	_ = b.repos.User.UpdateStep(user.ID, "getprice")
	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"Processing_value":     "none",
		"Processing_value_one": "none",
		"Processing_value_tow": "none",
	})

	chargeText, _ := b.repos.Setting.GetText("textchargewallet")
	if chargeText == "" {
		chargeText = "💰 مبلغ مورد نظر برای شارژ کیف پول را به تومان وارد کنید:"
	}
	return c.Send(chargeText)
}

func (b *Bot) handleChargeWallet(c tele.Context, user *models.User, text string) error {
	// Convert Persian numbers
	text = utils.ConvertPersianToEnglish(text)
	amount := parseIntSafe(text)

	if amount <= 0 {
		return c.Send("⚠️ لطفاً یک عدد معتبر وارد کنید.")
	}

	// Check min/max from PaySettings
	minAmount, _ := b.repos.Setting.GetPaySetting("minamount")
	maxAmount, _ := b.repos.Setting.GetPaySetting("maxamount")
	minAmt := parseIntSafe(minAmount)
	maxAmt := parseIntSafe(maxAmount)

	if minAmt > 0 && amount < minAmt {
		return c.Send(fmt.Sprintf("⚠️ حداقل مبلغ شارژ %s تومان است.", formatNumber(minAmt)))
	}
	if maxAmt > 0 && amount > maxAmt {
		return c.Send(fmt.Sprintf("⚠️ حداکثر مبلغ شارژ %s تومان است.", formatNumber(maxAmt)))
	}

	// Store amount and show payment methods
	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"Processing_value": fmt.Sprintf("%d", amount),
		"step":             "get_step_payment",
	})

	// If Processing_value_tow is already set (e.g., getconfigafterpay), keep it
	user, _ = b.repos.User.FindByID(user.ID)
	if user.ProcessingValueTwo == "" || user.ProcessingValueTwo == "none" {
		_ = b.repos.User.Update(user.ID, map[string]interface{}{
			"Processing_value_tow": "none",
		})
	}

	payKB := b.keyboard.PaymentMethodKeyboard(amount, user)
	return c.Send(
		fmt.Sprintf("💳 مبلغ: %s تومان\n\nروش پرداخت را انتخاب کنید:", formatNumber(amount)),
		payKB,
	)
}

// ── My Services ───────────────────────────────────────────────────────

func (b *Bot) sendMyServices(c tele.Context, user *models.User) error {
	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"pagenumber": 0,
	})
	return b.showServicesPage(c, user, 1)
}

func (b *Bot) showServicesPage(c tele.Context, user *models.User, page int) error {
	// Get active services (matching PHP statuses)
	var invoices []models.Invoice
	db := b.repos.Setting.DB()
	db.Where("id_user = ? AND Status IN ?", user.ID,
		[]string{"active", "end_of_time", "end_of_volume", "sendedwarn", "send_on_hold"},
	).Order("time_sell DESC").Offset((page - 1) * 20).Limit(20).Find(&invoices)

	if len(invoices) == 0 {
		noServiceText, _ := b.repos.Setting.GetText("textnoservice")
		if noServiceText == "" {
			noServiceText = "📋 شما سرویس فعالی ندارید."
		}
		return c.Send(noServiceText)
	}

	kb := b.keyboard.ServiceListKeyboard(invoices, page)
	return c.Send("📋 سرویس‌های شما:", kb)
}

func (b *Bot) handleMyServices(c tele.Context, user *models.User, text string) error {
	return b.sendMyServices(c, user)
}

func (b *Bot) handleServicePage(c tele.Context, user *models.User, data string) error {
	pageStr := strings.TrimPrefix(data, "srvpage_")
	page := parseIntSafe(pageStr)
	if page < 1 {
		page = 1
	}
	return b.showServicesPage(c, user, page)
}

func (b *Bot) handleServiceSelect(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "srv_")

	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	// Get panel info
	panelModel, err := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil {
		return c.Send("⚠️ پنل یافت نشد.")
	}

	// Get live data from panel
	ctx := context.Background()
	panelClient, err := b.getPanelClient(panelModel)

	var statusText, volumeText, timeText, onlineText string

	if err == nil {
		panelUser, err := panelClient.GetUser(ctx, invoice.Username)
		if err == nil {
			// Status
			switch panelUser.Status {
			case "active":
				statusText = "✅ فعال"
			case "disabled":
				statusText = "🚫 غیرفعال"
			case "expired":
				statusText = "⏰ منقضی"
			case "limited":
				statusText = "📊 محدود"
			case "on_hold":
				statusText = "⏸ در انتظار"
			default:
				statusText = panelUser.Status
			}

			// Volume
			usedGB := float64(panelUser.UsedTraffic) / (1024 * 1024 * 1024)
			totalGB := float64(panelUser.DataLimit) / (1024 * 1024 * 1024)
			remainGB := totalGB - usedGB
			if panelUser.DataLimit == 0 {
				volumeText = fmt.Sprintf("مصرف: %.2f GB | نامحدود", usedGB)
			} else {
				volumeText = fmt.Sprintf("مصرف: %.2f / %.2f GB | باقی: %.2f GB", usedGB, totalGB, remainGB)
			}

			// Time
			if panelUser.ExpireTime > 0 {
				remaining := panelUser.ExpireTime - time.Now().Unix()
				days := remaining / 86400
				if days < 0 {
					timeText = "منقضی شده"
				} else {
					timeText = fmt.Sprintf("%d روز باقی‌مانده", days)
				}
			} else {
				timeText = "نامحدود"
			}

			// Online status
			if panelUser.OnlineAt > 0 {
				lastSeen := time.Since(time.Unix(panelUser.OnlineAt, 0))
				if lastSeen.Minutes() < 5 {
					onlineText = "🟢 آنلاین"
				} else {
					onlineText = fmt.Sprintf("🔴 آخرین اتصال: %s", utils.TimeAgo(panelUser.OnlineAt))
				}
			}
		}
	}

	if statusText == "" {
		statusText = "نامشخص"
	}

	detailText := fmt.Sprintf(
		"📋 <b>جزئیات سرویس</b>\n\n"+
			"📦 محصول: %s\n"+
			"📍 لوکیشن: %s\n"+
			"👤 نام کاربری: <code>%s</code>\n"+
			"📊 وضعیت: %s\n"+
			"💾 حجم: %s\n"+
			"⏱ زمان: %s",
		invoice.NameProduct,
		invoice.ServiceLocation,
		invoice.Username,
		statusText,
		volumeText,
		timeText,
	)

	if onlineText != "" {
		detailText += fmt.Sprintf("\n🌐 %s", onlineText)
	}

	kb := b.keyboard.ServiceDetailKeyboard(invoice, panelModel)
	return c.Send(detailText, kb, tele.ModeHTML)
}

func (b *Bot) handleSubscriptionURL(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "suburl_")
	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	panelModel, _ := b.repos.Panel.FindByName(invoice.ServiceLocation)

	// Get subscription URL
	ctx := context.Background()
	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		return c.Send("⚠️ خطا در اتصال به پنل.")
	}

	subLink, err := panelClient.GetSubscriptionLink(ctx, invoice.Username)
	if err != nil {
		return c.Send("⚠️ خطا در دریافت لینک اشتراک.")
	}

	return c.Send(fmt.Sprintf("🔗 لینک اشتراک:\n<code>%s</code>", subLink), tele.ModeHTML)
}

func (b *Bot) handleShowConfigs(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "configs_")
	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	// Parse configs from user_info
	var links []string
	if invoice.UserInfo != "" {
		_ = json.Unmarshal([]byte(invoice.UserInfo), &links)
	}

	if len(links) == 0 {
		return c.Send("⚠️ کانفیگی یافت نشد.")
	}

	configText := "⚙️ <b>کانفیگ‌ها:</b>\n\n"
	for i, link := range links {
		configText += fmt.Sprintf("%d. <code>%s</code>\n\n", i+1, link)
	}

	return c.Send(configText, tele.ModeHTML)
}

func (b *Bot) handleToggleServiceStatus(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "togglesvc_")
	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	panelModel, err := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil {
		return c.Send("⚠️ پنل یافت نشد.")
	}

	ctx := context.Background()
	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		return c.Send("⚠️ خطا در اتصال به پنل.")
	}

	panelUser, err := panelClient.GetUser(ctx, invoice.Username)
	if err != nil {
		return c.Send("⚠️ خطا در دریافت وضعیت سرویس.")
	}

	newStatus := "active"
	if panelUser.Status == "active" {
		newStatus = "disabled"
	}

	if newStatus == "disabled" {
		if err := panelClient.DisableUser(ctx, invoice.Username); err != nil {
			return c.Send("⚠️ خطا در غیرفعال‌سازی سرویس.")
		}
	} else {
		if err := panelClient.EnableUser(ctx, invoice.Username); err != nil {
			return c.Send("⚠️ خطا در فعال‌سازی سرویس.")
		}
	}

	_ = b.repos.Invoice.Update(invoice.IDInvoice, map[string]interface{}{
		"Status": newStatus,
	})

	if newStatus == "disabled" {
		return c.Send("✅ سرویس غیرفعال شد.")
	}
	return c.Send("✅ سرویس فعال شد.")
}

func (b *Bot) handleResetServiceTraffic(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "resettraffic_")
	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	panelModel, err := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil {
		return c.Send("⚠️ پنل یافت نشد.")
	}

	ctx := context.Background()
	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		return c.Send("⚠️ خطا در اتصال به پنل.")
	}

	if err := panelClient.ResetTraffic(ctx, invoice.Username); err != nil {
		return c.Send("⚠️ خطا در ریست مصرف سرویس.")
	}

	return c.Send("✅ مصرف سرویس با موفقیت ریست شد.")
}

// ── Support ───────────────────────────────────────────────────────────

func (b *Bot) showSupportDepartments(c tele.Context, user *models.User) error {
	_ = b.repos.User.UpdateStep(user.ID, "support")
	kb := b.keyboard.SupportDepartmentKeyboard()

	supportText, _ := b.repos.Setting.GetText("textsupport")
	if supportText == "" {
		supportText = "📩 بخش مورد نظر را انتخاب کنید:"
	}
	return c.Send(supportText, kb)
}

func (b *Bot) handleSupport(c tele.Context, user *models.User, text string) error {
	// If text is typed in support step, treat as message
	return b.showSupportDepartments(c, user)
}

func (b *Bot) handleDepartmentSelect(c tele.Context, user *models.User, data string) error {
	deptIDStr := strings.TrimPrefix(data, "dept_")
	deptID := parseIntSafe(deptIDStr)

	// Find department
	var dept models.Departman
	if err := b.repos.Setting.DB().Where("id = ?", deptID).First(&dept).Error; err != nil {
		return c.Send("⚠️ بخش یافت نشد.")
	}

	// Store department selection
	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"Processing_value_four": fmt.Sprintf("%d", dept.ID),
		"step":                  "gettextticket",
	})

	return c.Send(fmt.Sprintf("📩 پیام خود را برای بخش «%s» بنویسید:", dept.NameDepartman))
}

func (b *Bot) handleSupportMessage(c tele.Context, user *models.User, text string) error {
	chatID := user.ID

	// Reload user for department ID
	user, _ = b.repos.User.FindByID(chatID)
	deptID := parseIntSafe(user.ProcessingValueFour)

	var dept models.Departman
	b.repos.Setting.DB().Where("id = ?", deptID).First(&dept)

	// Generate tracking code
	tracking := utils.RandomHex(5)

	// Create support message
	supportMsg := models.SupportMessage{
		Tracking:      tracking,
		IDSupport:     dept.IDSupport,
		IDUser:        chatID,
		NameDepartman: dept.NameDepartman,
		Text:          text,
		Time:          fmt.Sprintf("%d", time.Now().Unix()),
		Status:        "Unseen",
	}
	b.repos.Setting.DB().Create(&supportMsg)

	// Forward to support user
	supportText := fmt.Sprintf(
		"📩 <b>تیکت جدید</b>\n\n"+
			"🔖 کد پیگیری: %s\n"+
			"👤 کاربر: %s\n"+
			"📂 بخش: %s\n\n"+
			"💬 پیام:\n%s",
		tracking, chatID, dept.NameDepartman, text,
	)

	replyMarkup := map[string]interface{}{
		"inline_keyboard": [][]map[string]interface{}{
			{
				{"text": "📝 پاسخ", "callback_data": fmt.Sprintf("reply_%s_%s", chatID, tracking)},
			},
		},
	}
	b.botAPI.SendMessage(dept.IDSupport, supportText, replyMarkup)

	_ = b.repos.User.UpdateStep(chatID, "none")

	return c.Send(fmt.Sprintf("✅ پیام شما ارسال شد.\n🔖 کد پیگیری: %s", tracking))
}

// ── Extend Service ────────────────────────────────────────────────────

func (b *Bot) handleExtendService(c tele.Context, user *models.User, text string) error {
	return b.sendMyServices(c, user)
}

func (b *Bot) handleExtendCallback(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "extend_")

	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	panelModel, err := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil {
		return c.Send("⚠️ پنل یافت نشد.")
	}

	if panelModel.StatusExtend == "off" {
		return c.Send("⚠️ امکان تمدید برای این لوکیشن وجود ندارد.")
	}

	kb := b.keyboard.ExtendProductKeyboard(panelModel.CodePanel, user, invoiceID)
	return c.Send("🔁 محصول مورد نظر برای تمدید را انتخاب کنید:", kb)
}

func (b *Bot) handleExtendProductSelect(c tele.Context, user *models.User, data string) error {
	// Format: extprod_<invoiceID>_<productID>
	trimmed := strings.TrimPrefix(data, "extprod_")
	parts := strings.SplitN(trimmed, "_", 2)
	if len(parts) != 2 {
		return c.Send("⚠️ خطا در انتخاب محصول.")
	}

	invoiceID := parts[0]
	productID := parseIntSafe(parts[1])

	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	product, err := b.repos.Product.FindByID(productID)
	if err != nil {
		return c.Send("⚠️ محصول یافت نشد.")
	}

	price := parseIntSafe(product.PriceProduct)
	discountPct := 0
	if user.PriceDiscount.Valid && user.PriceDiscount.String != "" && user.PriceDiscount.String != "0" {
		discountPct = parseIntSafe(user.PriceDiscount.String)
		price = price - (price * discountPct / 100)
	}

	if user.Balance < price {
		deficit := price - user.Balance
		_ = b.repos.User.Update(user.ID, map[string]interface{}{
			"Processing_value":     fmt.Sprintf("%d", deficit),
			"Processing_value_one": fmt.Sprintf("%s%%%s", invoice.Username, invoiceID),
			"Processing_value_tow": "getextenduser",
			"step":                 "get_step_payment",
		})

		payKB := b.keyboard.PaymentMethodKeyboard(deficit, user)
		return c.Send(
			fmt.Sprintf("💳 موجودی کافی نیست.\nمبلغ مورد نیاز: %s تومان\n\nروش پرداخت:", formatNumber(deficit)),
			payKB,
		)
	}

	// Sufficient balance — extend directly
	return b.performExtend(c, user, invoice, product, price)
}

func (b *Bot) performExtend(c tele.Context, user *models.User, invoice *models.Invoice, product *models.Product, price int) error {
	ctx := context.Background()
	panelModel, err := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil {
		return c.Send("⚠️ پنل یافت نشد.")
	}

	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		return c.Send("⚠️ خطا در اتصال به پنل.")
	}

	// Get current user data
	panelUser, err := panelClient.GetUser(ctx, invoice.Username)
	if err != nil {
		return c.Send("⚠️ خطا در دریافت اطلاعات سرویس.")
	}

	volumeGB := parseIntSafe(product.VolumeConstraint)
	dataLimit := int64(volumeGB) * 1024 * 1024 * 1024
	serviceDays := parseIntSafe(product.ServiceTime)

	// Calculate new expire and volume based on extend method
	var newExpire int64
	var newDataLimit int64

	switch panelModel.MethodExtend {
	case "1": // Reset volume and time
		newExpire = time.Now().Unix() + int64(serviceDays)*86400
		newDataLimit = dataLimit
		_ = panelClient.ResetTraffic(ctx, invoice.Username)
	case "2": // Add time and volume
		if panelUser.ExpireTime > time.Now().Unix() {
			newExpire = panelUser.ExpireTime + int64(serviceDays)*86400
		} else {
			newExpire = time.Now().Unix() + int64(serviceDays)*86400
		}
		newDataLimit = panelUser.DataLimit + dataLimit
	case "3": // Reset time, add old volume
		newExpire = time.Now().Unix() + int64(serviceDays)*86400
		remaining := panelUser.DataLimit - panelUser.UsedTraffic
		if remaining < 0 {
			remaining = 0
		}
		newDataLimit = remaining + dataLimit
		_ = panelClient.ResetTraffic(ctx, invoice.Username)
	case "4": // Reset volume, add time
		if panelUser.ExpireTime > time.Now().Unix() {
			newExpire = panelUser.ExpireTime + int64(serviceDays)*86400
		} else {
			newExpire = time.Now().Unix() + int64(serviceDays)*86400
		}
		newDataLimit = dataLimit
		_ = panelClient.ResetTraffic(ctx, invoice.Username)
	default: // Default: reset both
		newExpire = time.Now().Unix() + int64(serviceDays)*86400
		newDataLimit = dataLimit
		_ = panelClient.ResetTraffic(ctx, invoice.Username)
	}

	// Modify user on panel
	modReq := panel.ModifyUserRequest{
		Status:     "active",
		DataLimit:  newDataLimit,
		ExpireTime: newExpire,
	}
	_, err = panelClient.ModifyUser(ctx, invoice.Username, modReq)
	if err != nil {
		b.logger.Error("Failed to extend service", zap.Error(err))
		return c.Send("⚠️ خطا در تمدید سرویس.")
	}

	// Deduct balance
	_ = b.repos.User.UpdateBalance(user.ID, -price)

	// Update invoice
	_ = b.repos.Invoice.Update(invoice.IDInvoice, map[string]interface{}{
		"Status": "active",
	})

	// Record in service_other
	serviceOther := models.ServiceOther{
		IDUser:   user.ID,
		Username: invoice.Username,
		Value:    product.CodeProduct,
		Time:     fmt.Sprintf("%d", time.Now().Unix()),
		Price:    fmt.Sprintf("%d", price),
		Type:     "extend_user",
		Status:   "paid",
	}
	b.repos.Setting.DB().Create(&serviceOther)

	_ = b.repos.User.UpdateStep(user.ID, "none")

	// Report
	b.reportToChannel(fmt.Sprintf(
		"🔁 تمدید سرویس\n👤 کاربر: %s\n📦 محصول: %s\n📍 لوکیشن: %s\n💰 مبلغ: %s تومان",
		user.ID, product.NameProduct, invoice.ServiceLocation, formatNumber(price),
	))

	_ = c.Send(fmt.Sprintf("✅ سرویس %s با موفقیت تمدید شد.", invoice.Username))
	return b.sendMainMenu(c, user.ID)
}

// ── Payment ───────────────────────────────────────────────────────────

func (b *Bot) handlePayment(c tele.Context, user *models.User, text string) error {
	// If in get_step_payment, parse as amount
	return b.handleChargeWallet(c, user, text)
}

func (b *Bot) handlePaymentMethod(c tele.Context, user *models.User, data string) error {
	method := strings.TrimPrefix(data, "pay_")
	chatID := user.ID

	user, _ = b.repos.User.FindByID(chatID)
	amount := parseIntSafe(user.ProcessingValue)

	if amount <= 0 {
		return c.Send("⚠️ مبلغ پرداخت نامعتبر است.")
	}

	if method == "stars" {
		return c.Send("⚠️ پرداخت با ستاره تلگرام در نسخه Go هنوز فعال نشده است.")
	}

	if method != "wallet" {
		if err := b.validateGatewayAmount(method, amount); err != nil {
			return c.Send(err.Error())
		}
	}

	// Generate order ID
	orderID := utils.RandomHex(5)

	// Create payment report
	payment := &models.PaymentReport{
		IDUser:        chatID,
		IDOrder:       orderID,
		Time:          fmt.Sprintf("%d", time.Now().Unix()),
		Price:         fmt.Sprintf("%d", amount),
		PaymentStatus: "Unpaid",
		PaymentMethod: b.paymentMethodStorageName(method),
		IDInvoice:     fmt.Sprintf("%s|%s", user.ProcessingValueTwo, user.ProcessingValueOne),
	}
	_ = b.repos.Payment.Create(payment)

	switch method {
	case "card":
		return b.handleCardPayment(c, user, orderID, amount)
	case "wallet":
		return b.handleWalletPayment(c, user, orderID, amount)
	case "zarinpal":
		return b.handleZarinpalPayment(c, user, orderID, amount)
	case "nowpayments":
		return b.handleNowPaymentsPayment(c, user, orderID, amount)
	case "plisio":
		return b.handlePlisioPayment(c, user, orderID, amount)
	case "aqayepardakht":
		return b.handleAqayePardakhtPayment(c, user, orderID, amount)
	case "iranpay1":
		return b.handleIranPay1Payment(c, user, orderID, amount)
	case "iranpay2":
		return b.handleTronadoPayment(c, user, orderID, amount)
	case "digitaltron":
		return b.handleOfflineCryptoPayment(c, user, orderID, amount)
	default:
		return c.Send("⚠️ روش پرداخت نامعتبر است.")
	}
}

func (b *Bot) handleCardPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	// Get card number from settings
	cardNum, _ := b.repos.Setting.GetPaySetting("cardnum")
	cardName, _ := b.repos.Setting.GetPaySetting("cardname")

	if cardNum == "" {
		return c.Send("⚠️ شماره کارت تنظیم نشده است.")
	}

	// Add random amount for auto-verification
	randomExtra := rand.Intn(99) + 1
	totalAmount := amount + randomExtra

	payText := fmt.Sprintf(
		"💳 <b>پرداخت کارت به کارت</b>\n\n"+
			"💰 مبلغ: <b>%s</b> تومان\n"+
			"💳 شماره کارت:\n<code>%s</code>\n"+
			"👤 به نام: %s\n\n"+
			"⚠️ لطفاً <b>دقیقاً</b> مبلغ %s تومان واریز کنید.\n"+
			"📸 سپس رسید پرداخت را ارسال نمایید.",
		formatNumber(totalAmount),
		cardNum,
		cardName,
		formatNumber(totalAmount),
	)

	// Update order amount
	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"price": fmt.Sprintf("%d", totalAmount),
	})

	kb := b.keyboard.CardPaymentKeyboard(orderID)
	return c.Send(payText, kb, tele.ModeHTML)
}

func (b *Bot) handleWalletPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	if user.Balance < amount {
		return c.Send("⚠️ موجودی کیف پول شما کافی نیست.")
	}

	// Mark payment as paid
	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"payment_Status": "paid",
	})

	// Process payment
	return b.directPayment(c, orderID)
}

func (b *Bot) handleZarinpalPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	merchantID, _ := b.repos.Setting.GetPaySetting("merchant_zarinpal")
	if merchantID == "" {
		return c.Send("⚠️ درگاه زرین‌پال تنظیم نشده است.")
	}

	setting, _ := b.repos.Setting.GetSettings()
	if setting == nil {
		return c.Send("⚠️ خطا در خواندن تنظیمات.")
	}

	// Build callback URL
	callbackURL := fmt.Sprintf("%s/payment/zarinpal/callback", b.cfg.Bot.Domain)

	gw := payment.NewZarinPalGateway(merchantID, false)
	result, err := gw.CreatePayment(context.Background(), amount, orderID, user.ID, callbackURL)
	if err != nil {
		b.logger.Error("ZarinPal create payment error", zap.Error(err))
		return c.Send("❌ خطا در ایجاد لینک پرداخت. لطفاً مجدد تلاش کنید.")
	}

	// Save authority for verification later
	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"dec_not_confirmed": result.Authority,
	})

	kb := &tele.ReplyMarkup{}
	kb.Inline(tele.Row{kb.URL("💳 پرداخت", result.PaymentURL)})

	return c.Send(fmt.Sprintf("🔗 لطفاً برای پرداخت مبلغ %s تومان روی دکمه زیر کلیک کنید:", formatNumber(amount)), kb, tele.ModeHTML)
}

func (b *Bot) handleNowPaymentsPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	apiKey := b.firstPaySetting("apikey_nowpayment", "marchent_tronseller")
	if apiKey == "" {
		return c.Send("⚠️ درگاه NowPayments تنظیم نشده است.")
	}

	usdRate, _, err := b.getCryptoRates(context.Background())
	if err != nil {
		b.logger.Error("Failed to fetch crypto rates", zap.Error(err))
		return c.Send("❌ خطا در دریافت نرخ ارز. لطفاً مجدد تلاش کنید.")
	}

	usdAmount := int(math.Round(float64(amount) / float64(usdRate)))
	if usdAmount < 1 {
		usdAmount = 1
	}

	callbackURL := fmt.Sprintf("%s/payment/nowpayments/callback", b.cfg.Bot.Domain)

	gw := payment.NewNOWPaymentsGateway(apiKey)
	result, err := gw.CreatePayment(context.Background(), usdAmount, orderID, user.ID, callbackURL)
	if err != nil {
		b.logger.Error("NowPayments create payment error", zap.Error(err))
		return c.Send("❌ خطا در ایجاد لینک پرداخت. لطفاً مجدد تلاش کنید.")
	}

	// Save invoice ID for verification
	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"dec_not_confirmed": result.InvoiceID,
	})

	kb := &tele.ReplyMarkup{}
	kb.Inline(tele.Row{kb.URL("💳 پرداخت کریپتو", result.PaymentURL)})

	return c.Send(
		fmt.Sprintf("🔗 مبلغ شما %s تومان (حدود %d USD) است. برای پرداخت روی دکمه زیر کلیک کنید:", formatNumber(amount), usdAmount),
		kb,
		tele.ModeHTML,
	)
}

func (b *Bot) handlePlisioPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	apiKey := b.firstPaySetting("apinowpayment")
	if apiKey == "" {
		return c.Send("⚠️ درگاه Plisio تنظیم نشده است.")
	}

	_, trxRate, err := b.getCryptoRates(context.Background())
	if err != nil {
		b.logger.Error("Failed to fetch crypto rates", zap.Error(err))
		return c.Send("❌ خطا در دریافت نرخ ارز. لطفاً مجدد تلاش کنید.")
	}

	trxAmount := float64(amount) / trxRate
	if trxAmount <= 0 {
		return c.Send("⚠️ مبلغ پرداخت نامعتبر است.")
	}

	q := url.Values{}
	q.Set("currency", "TRX")
	q.Set("amount", fmt.Sprintf("%.4f", trxAmount))
	q.Set("order_number", orderID)
	q.Set("email", "customer@plisio.net")
	q.Set("order_name", "plisio")
	q.Set("language", "fa")
	q.Set("api_key", apiKey)

	reqURL := "https://api.plisio.net/api/v1/invoices/new?" + q.Encode()
	rawResp, err := httpclient.New().WithTimeout(30 * time.Second).Get(reqURL)
	if err != nil {
		b.logger.Error("Plisio create payment error", zap.Error(err))
		return c.Send("❌ خطا در ایجاد لینک پرداخت. لطفاً مجدد تلاش کنید.")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		b.logger.Error("Plisio response parse error", zap.Error(err))
		return c.Send("❌ پاسخ نامعتبر از درگاه Plisio دریافت شد.")
	}

	data, _ := payload["data"].(map[string]interface{})
	invoiceURL := parseStringFromAny(data["invoice_url"])
	txnID := parseStringFromAny(data["txn_id"])
	if invoiceURL == "" {
		msg := parseStringFromAny(payload["message"])
		if msg == "" {
			msg = "ایجاد فاکتور در Plisio ناموفق بود."
		}
		return c.Send("❌ " + msg)
	}

	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"dec_not_confirmed": txnID,
	})

	kb := &tele.ReplyMarkup{}
	kb.Inline(tele.Row{kb.URL("💳 پرداخت با Plisio", invoiceURL)})

	return c.Send(
		fmt.Sprintf("🔗 مبلغ شما %s تومان (حدود %.4f TRX) است. برای پرداخت روی دکمه زیر کلیک کنید:", formatNumber(amount), trxAmount),
		kb,
		tele.ModeHTML,
	)
}

func (b *Bot) handleAqayePardakhtPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	pin := b.firstPaySetting("merchant_id_aqayepardakht")
	if pin == "" {
		return c.Send("⚠️ درگاه آقای پرداخت تنظیم نشده است.")
	}

	callbackURL := fmt.Sprintf("%s/payment/aqayepardakht/callback", b.cfg.Bot.Domain)
	reqBody, _ := json.Marshal(map[string]interface{}{
		"pin":        pin,
		"amount":     amount,
		"callback":   callbackURL,
		"invoice_id": orderID,
	})

	rawResp, err := httpclient.New().
		WithTimeout(30*time.Second).
		WithHeader("Content-Type", "application/json").
		WithHeader("Accept", "application/json").
		Post("https://panel.aqayepardakht.ir/api/v2/create", reqBody)
	if err != nil {
		b.logger.Error("AqayePardakht create payment error", zap.Error(err))
		return c.Send("❌ خطا در ایجاد لینک پرداخت. لطفاً مجدد تلاش کنید.")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		b.logger.Error("AqayePardakht response parse error", zap.Error(err))
		return c.Send("❌ پاسخ نامعتبر از درگاه آقای پرداخت دریافت شد.")
	}

	status := strings.ToLower(strings.TrimSpace(parseStringFromAny(payload["status"])))
	if status != "success" {
		msg := parseStringFromAny(payload["message"])
		if msg == "" {
			msg = "ایجاد فاکتور در آقای پرداخت ناموفق بود."
		}
		return c.Send("❌ " + msg)
	}

	transID := parseStringFromAny(payload["transid"])
	if transID == "" {
		return c.Send("❌ شناسه تراکنش از درگاه دریافت نشد.")
	}

	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"dec_not_confirmed": transID,
	})

	paymentURL := "https://panel.aqayepardakht.ir/startpay/" + transID
	kb := &tele.ReplyMarkup{}
	kb.Inline(tele.Row{kb.URL("💳 پرداخت", paymentURL)})

	return c.Send(fmt.Sprintf("🔗 لطفاً برای پرداخت مبلغ %s تومان روی دکمه زیر کلیک کنید:", formatNumber(amount)), kb, tele.ModeHTML)
}

func (b *Bot) handleIranPay1Payment(c tele.Context, user *models.User, orderID string, amount int) error {
	apiKey := b.firstPaySetting("marchent_floypay")
	if apiKey == "" {
		return c.Send("⚠️ درگاه ایران‌پی ۱ تنظیم نشده است.")
	}

	callbackURL := fmt.Sprintf("%s/payment/iranpay/callback", b.cfg.Bot.Domain)
	reqBody, _ := json.Marshal(map[string]interface{}{
		"ApiKey":      apiKey,
		"Hash_id":     orderID,
		"Amount":      fmt.Sprintf("%d", amount*10),
		"CallbackURL": callbackURL,
	})

	rawResp, err := httpclient.New().
		WithTimeout(30*time.Second).
		WithHeader("Content-Type", "application/json").
		WithHeader("Accept", "application/json").
		Post("https://tetra98.ir/api/create_order", reqBody)
	if err != nil {
		b.logger.Error("IranPay1 create payment error", zap.Error(err))
		return c.Send("❌ خطا در ایجاد لینک پرداخت. لطفاً مجدد تلاش کنید.")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		b.logger.Error("IranPay1 response parse error", zap.Error(err))
		return c.Send("❌ پاسخ نامعتبر از درگاه ایران‌پی ۱ دریافت شد.")
	}

	statusOK := parseStringFromAny(payload["status"]) == "100"
	if !statusOK {
		if codeFloat, ok := payload["status"].(float64); ok {
			statusOK = int(codeFloat) == 100
		}
	}
	if !statusOK {
		msg := parseStringFromAny(payload["message"])
		if msg == "" {
			msg = "ایجاد فاکتور در ایران‌پی ۱ ناموفق بود."
		}
		return c.Send("❌ " + msg)
	}

	paymentURL := parseStringFromAny(payload["payment_url_bot"])
	if paymentURL == "" {
		paymentURL = parseStringFromAny(payload["payment_url"])
	}
	if paymentURL == "" {
		paymentURL = parseStringFromAny(payload["url"])
	}
	if paymentURL == "" {
		return c.Send("❌ لینک پرداخت از درگاه دریافت نشد.")
	}

	authority := parseStringFromAny(payload["Authority"])
	if authority != "" {
		_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
			"dec_not_confirmed": authority,
		})
	}

	kb := &tele.ReplyMarkup{}
	kb.Inline(tele.Row{kb.URL("💳 پرداخت", paymentURL)})

	return c.Send(fmt.Sprintf("🔗 لطفاً برای پرداخت مبلغ %s تومان روی دکمه زیر کلیک کنید:", formatNumber(amount)), kb, tele.ModeHTML)
}

func (b *Bot) handleTronadoPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	apiKey := b.firstPaySetting("apiternado")
	walletAddress := b.firstPaySetting("walletaddress")
	paymentURL := b.firstPaySetting("urlpaymenttron")
	if paymentURL == "" {
		paymentURL = "https://bot.tronado.cloud/Order/GetPaymentLink"
	}
	if apiKey == "" || walletAddress == "" {
		return c.Send("⚠️ درگاه ترونادو تنظیم نشده است.")
	}

	_, trxRate, err := b.getCryptoRates(context.Background())
	if err != nil {
		b.logger.Error("Failed to fetch crypto rates", zap.Error(err))
		return c.Send("❌ خطا در دریافت نرخ ارز. لطفاً مجدد تلاش کنید.")
	}
	trxAmount := math.Round((float64(amount)/trxRate)*10000) / 10000

	reqBody, _ := json.Marshal(map[string]interface{}{
		"PaymentID":     orderID,
		"WalletAddress": walletAddress,
		"TronAmount":    trxAmount,
		"CallbackUrl":   fmt.Sprintf("%s/payment/tronado/callback", b.cfg.Bot.Domain),
	})

	rawResp, err := httpclient.New().
		WithTimeout(30*time.Second).
		WithHeader("x-api-key", apiKey).
		WithHeader("Content-Type", "application/json").
		Post(paymentURL, reqBody)
	if err != nil {
		b.logger.Error("Tronado create payment error", zap.Error(err))
		return c.Send("❌ خطا در ایجاد لینک پرداخت. لطفاً مجدد تلاش کنید.")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		b.logger.Error("Tronado response parse error", zap.Error(err))
		return c.Send("❌ پاسخ نامعتبر از درگاه ترونادو دریافت شد.")
	}

	success := false
	switch v := payload["IsSuccessful"].(type) {
	case bool:
		success = v
	case string:
		success = strings.EqualFold(v, "true")
	}
	if !success {
		msg := parseStringFromAny(payload["Message"])
		if msg == "" {
			msg = parseStringFromAny(payload["message"])
		}
		if msg == "" {
			msg = "ایجاد فاکتور در ترونادو ناموفق بود."
		}
		return c.Send("❌ " + msg)
	}

	token := ""
	if dataMap, ok := payload["Data"].(map[string]interface{}); ok {
		token = parseStringFromAny(dataMap["Token"])
	}
	if token == "" {
		return c.Send("❌ لینک پرداخت از درگاه دریافت نشد.")
	}

	kb := &tele.ReplyMarkup{}
	kb.Inline(tele.Row{kb.URL("💳 پرداخت ترونادو", "https://t.me/tronado_robot/customerpayment?startapp="+token)})

	return c.Send(
		fmt.Sprintf("🔗 مبلغ شما %s تومان (حدود %.4f TRX) است. برای پرداخت روی دکمه زیر کلیک کنید:", formatNumber(amount), trxAmount),
		kb,
		tele.ModeHTML,
	)
}

func (b *Bot) handleOfflineCryptoPayment(c tele.Context, user *models.User, orderID string, amount int) error {
	walletAddress := b.firstPaySetting("walletaddress")
	if walletAddress == "" {
		return c.Send("⚠️ آدرس کیف پول برای پرداخت آفلاین تنظیم نشده است.")
	}

	_, trxRate, err := b.getCryptoRates(context.Background())
	if err != nil {
		b.logger.Error("Failed to fetch crypto rates", zap.Error(err))
		return c.Send("❌ خطا در دریافت نرخ ارز. لطفاً مجدد تلاش کنید.")
	}
	trxAmount := math.Round((float64(amount)/trxRate)*10000) / 10000

	text := fmt.Sprintf(
		"✅ تراکنش شما ایجاد شد\n\n"+
			"🛒 کد پیگیری: <code>%s</code>\n"+
			"🌐 شبکه: TRX\n"+
			"💳 آدرس ولت: <code>%s</code>\n"+
			"💲 مبلغ تراکنش: %.4f TRX\n"+
			"💰 معادل تومانی: %s تومان\n\n"+
			"📸 پس از پرداخت روی دکمه زیر بزنید و رسید را ارسال کنید.",
		orderID,
		walletAddress,
		trxAmount,
		formatNumber(amount),
	)

	kb := b.keyboard.CardPaymentKeyboard(orderID)
	return c.Send(text, kb, tele.ModeHTML)
}

// ── Payment Receipt ───────────────────────────────────────────────────

func (b *Bot) handlePaymentReceipt(c tele.Context, user *models.User) error {
	chatID := user.ID
	user, _ = b.repos.User.FindByID(chatID)
	orderID := user.ProcessingValueOne

	if orderID == "" || orderID == "none" {
		return c.Send("⚠️ اطلاعات پرداخت یافت نشد.")
	}

	// Get payment record
	payment, err := b.repos.Payment.FindByOrderID(orderID)
	if err != nil {
		return c.Send("⚠️ سفارش یافت نشد.")
	}

	// Forward receipt photo to all admins
	admins, _ := b.repos.Setting.GetAllAdmins()
	for _, admin := range admins {
		if admin.Rule == "support" {
			continue // Skip support-only admins
		}

		receiptText := fmt.Sprintf(
			"📸 <b>رسید پرداخت</b>\n\n"+
				"👤 کاربر: %s\n"+
				"💰 مبلغ: %s تومان\n"+
				"🔖 شماره سفارش: %s\n"+
				"💳 روش: %s",
			chatID,
			payment.Price,
			orderID,
			b.paymentMethodLabel(payment.PaymentMethod),
		)

		// Forward the photo message
		b.botAPI.ForwardMessage(admin.IDAdmin, chatID, c.Message().ID)
		adminKB := b.keyboard.AdminPaymentConfirmKeyboard(orderID)
		b.botAPI.SendMessage(admin.IDAdmin, receiptText, adminKB)
	}

	// Match PHP behavior: manual receipt submissions move payment to waiting.
	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"payment_Status": "waiting",
		"at_updated":     fmt.Sprintf("%d", time.Now().Unix()),
	})

	_ = b.repos.User.UpdateStep(chatID, "none")

	return c.Send("✅ رسید پرداخت شما ارسال شد. لطفاً منتظر تایید بمانید.")
}

// ── Admin Payment Confirm/Reject ──────────────────────────────────────

func (b *Bot) handleAdminPaymentConfirm(c tele.Context, user *models.User, data string) error {
	orderID := strings.TrimPrefix(data, "confirmpay_")

	// Update payment status
	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"payment_Status": "paid",
		"at_updated":     fmt.Sprintf("%d", time.Now().Unix()),
	})

	// Process payment
	_ = b.directPayment(c, orderID)

	return c.Send(fmt.Sprintf("✅ پرداخت %s تایید شد.", orderID))
}

func (b *Bot) handleAdminPaymentReject(c tele.Context, user *models.User, data string) error {
	orderID := strings.TrimPrefix(data, "rejectpay_")

	_ = b.repos.Payment.UpdateByOrderID(orderID, map[string]interface{}{
		"payment_Status": "rejected",
		"at_updated":     fmt.Sprintf("%d", time.Now().Unix()),
	})

	// Notify user
	payment, _ := b.repos.Payment.FindByOrderID(orderID)
	if payment != nil {
		b.botAPI.SendMessage(payment.IDUser, "❌ پرداخت شما تایید نشد. لطفاً با پشتیبانی تماس بگیرید.", nil)
	}

	return c.Send(fmt.Sprintf("❌ پرداخت %s رد شد.", orderID))
}

// ── DirectPayment — processes confirmed payments ──────────────────────

// ProcessPaidOrder allows external components (e.g., cron) to run the same
// paid-order fulfillment flow used by bot callback handlers.
func (b *Bot) ProcessPaidOrder(orderID string) error {
	return b.processPaidOrder(orderID)
}

func (b *Bot) directPayment(c tele.Context, orderID string) error {
	return b.processPaidOrder(orderID)
}

func (b *Bot) processPaidOrder(orderID string) error {
	payment, err := b.repos.Payment.FindByOrderID(orderID)
	if err != nil {
		return fmt.Errorf("payment not found: %s", orderID)
	}

	amount := parseIntSafe(payment.Price)
	userID := payment.IDUser

	// Parse id_invoice field: "purpose|data"
	parts := strings.SplitN(payment.IDInvoice, "|", 2)
	purpose := ""
	extraData := ""
	if len(parts) >= 1 {
		purpose = parts[0]
	}
	if len(parts) >= 2 {
		extraData = parts[1]
	}

	switch purpose {
	case "getconfigafterpay":
		// Create service after payment
		user, _ := b.repos.User.FindByID(userID)
		if user == nil {
			break
		}

		username := extraData
		productCode := user.ProcessingValueTwo
		panelCode := user.ProcessingValueFour

		// Add amount to balance first
		_ = b.repos.User.UpdateBalance(userID, amount)

		product, err := b.repos.Product.FindByCode(productCode)
		if err != nil {
			b.botAPI.SendMessage(userID, "⚠️ خطا: محصول یافت نشد.", nil)
			break
		}

		panelModel, err := b.repos.Panel.FindByCode(panelCode)
		if err != nil {
			b.botAPI.SendMessage(userID, "⚠️ خطا: لوکیشن یافت نشد.", nil)
			break
		}

		price := parseIntSafe(product.PriceProduct)
		if user.PriceDiscount.Valid && user.PriceDiscount.String != "0" {
			discPct := parseIntSafe(user.PriceDiscount.String)
			price = price - (price * discPct / 100)
		}

		// Use the bot context or send via BotAPI directly
		b.createServiceDirect(userID, user, product, panelModel, username, price)

	case "getextenduser":
		// Extend service after payment
		user, _ := b.repos.User.FindByID(userID)
		if user == nil {
			break
		}

		// extraData format: username%invoiceID
		extParts := strings.SplitN(extraData, "%", 2)
		if len(extParts) != 2 {
			break
		}

		invoiceID := extParts[1]
		invoice, err := b.repos.Invoice.FindByID(invoiceID)
		if err != nil {
			break
		}

		// Add to balance then extend
		_ = b.repos.User.UpdateBalance(userID, amount)

		productCode := user.ProcessingValueTwo
		product, _ := b.repos.Product.FindByCode(productCode)
		if product != nil {
			price := parseIntSafe(product.PriceProduct)
			b.performExtendDirect(userID, user, invoice, product, price)
		}

	default:
		// Plain wallet charge
		_ = b.repos.User.UpdateBalance(userID, amount)
		b.botAPI.SendMessage(userID,
			fmt.Sprintf("✅ کیف پول شما با مبلغ %s تومان شارژ شد.", formatNumber(amount)),
			nil,
		)
	}

	return nil
}

// createServiceDirect creates service without tele.Context (called from directPayment).
func (b *Bot) createServiceDirect(userID string, user *models.User, product *models.Product, panelModel *models.Panel, username string, price int) {
	ctx := context.Background()

	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		b.botAPI.SendMessage(userID, "⚠️ خطا در اتصال به پنل.", nil)
		return
	}

	volumeGB := parseIntSafe(product.VolumeConstraint)
	dataLimit := int64(volumeGB) * 1024 * 1024 * 1024
	serviceDays := parseIntSafe(product.ServiceTime)

	createReq := panel.CreateUserRequest{
		Username:       username,
		DataLimit:      dataLimit,
		ExpireDays:     serviceDays,
		DataLimitReset: product.DataLimitReset,
		Note:           userID,
	}

	if product.Inbounds != "" {
		var inbounds map[string][]string
		if json.Unmarshal([]byte(product.Inbounds), &inbounds) == nil {
			createReq.Inbounds = inbounds
		}
	}

	if product.Proxies != "" {
		var proxies map[string]string
		if json.Unmarshal([]byte(product.Proxies), &proxies) == nil {
			createReq.Proxies = proxies
		}
	}

	panelUser, err := panelClient.CreateUser(ctx, createReq)
	if err != nil {
		b.botAPI.SendMessage(userID, "⚠️ خطا در ایجاد سرویس.", nil)
		return
	}

	invoiceID := fmt.Sprintf("%d", time.Now().UnixNano()/1e6)
	userInfoJSON, _ := json.Marshal(panelUser.Links)
	uuidJSON, _ := json.Marshal(panelUser.Proxies)

	invoice := &models.Invoice{
		IDInvoice:       invoiceID,
		IDUser:          userID,
		Username:        username,
		ServiceLocation: panelModel.NamePanel,
		TimeSell:        fmt.Sprintf("%d", time.Now().Unix()),
		NameProduct:     product.NameProduct,
		PriceProduct:    fmt.Sprintf("%d", price),
		Volume:          product.VolumeConstraint,
		ServiceTime:     product.ServiceTime,
		UUID:            string(uuidJSON),
		UserInfo:        string(userInfoJSON),
		Referral:        user.Affiliates,
		Notifications:   `{"volume":false,"time":false}`,
		Status:          "active",
	}
	_ = b.repos.Invoice.Create(invoice)
	_ = b.repos.User.UpdateBalance(userID, -price)
	_ = b.repos.User.UpdateStep(userID, "none")

	b.repos.Panel.IncrementCounter(panelModel.CodePanel)

	subURL := panelUser.SubLink
	configText := fmt.Sprintf(
		"✅ <b>سرویس شما ایجاد شد!</b>\n\n📦 %s\n👤 %s\n🔗 لینک:\n<code>%s</code>",
		product.NameProduct, username, subURL,
	)
	b.botAPI.SendMessage(userID, configText, nil)

	b.handleAffiliateCommission(user, price)
	b.reportToChannel(fmt.Sprintf("🛒 خرید\n👤 %s\n📦 %s\n💰 %s", userID, product.NameProduct, formatNumber(price)))
}

// performExtendDirect extends service without tele.Context.
func (b *Bot) performExtendDirect(userID string, user *models.User, invoice *models.Invoice, product *models.Product, price int) {
	ctx := context.Background()
	panelModel, err := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil {
		return
	}

	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		return
	}

	panelUser, err := panelClient.GetUser(ctx, invoice.Username)
	if err != nil {
		return
	}

	volumeGB := parseIntSafe(product.VolumeConstraint)
	dataLimit := int64(volumeGB) * 1024 * 1024 * 1024
	serviceDays := parseIntSafe(product.ServiceTime)

	newExpire := time.Now().Unix() + int64(serviceDays)*86400
	if panelUser.ExpireTime > time.Now().Unix() {
		newExpire = panelUser.ExpireTime + int64(serviceDays)*86400
	}

	modReq := panel.ModifyUserRequest{
		Status:     "active",
		DataLimit:  dataLimit,
		ExpireTime: newExpire,
	}
	_, _ = panelClient.ModifyUser(ctx, invoice.Username, modReq)
	_ = panelClient.ResetTraffic(ctx, invoice.Username)

	_ = b.repos.User.UpdateBalance(userID, -price)
	_ = b.repos.Invoice.Update(invoice.IDInvoice, map[string]interface{}{"Status": "active"})
	_ = b.repos.User.UpdateStep(userID, "none")

	b.botAPI.SendMessage(userID, fmt.Sprintf("✅ سرویس %s تمدید شد.", invoice.Username), nil)
}

// ── Service Actions ───────────────────────────────────────────────────

func (b *Bot) handleRemoveService(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "removeservice_")

	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	ctx := context.Background()
	panelModel, _ := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if panelModel != nil {
		panelClient, err := b.getPanelClient(panelModel)
		if err == nil {
			_ = panelClient.DeleteUser(ctx, invoice.Username)
		}
	}

	_ = b.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": "deleted"})

	b.reportToChannel(fmt.Sprintf("❌ حذف سرویس\n👤 %s\n📦 %s", user.ID, invoice.Username))

	return c.Send("✅ سرویس حذف شد.")
}

func (b *Bot) handleChangeLink(c tele.Context, user *models.User, data string) error {
	invoiceID := strings.TrimPrefix(data, "changelink_")

	invoice, err := b.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return c.Send("⚠️ سرویس یافت نشد.")
	}

	ctx := context.Background()
	panelModel, _ := b.repos.Panel.FindByName(invoice.ServiceLocation)
	if panelModel == nil {
		return c.Send("⚠️ پنل یافت نشد.")
	}

	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		return c.Send("⚠️ خطا در اتصال به پنل.")
	}

	newLink, err := panelClient.RevokeSubscription(ctx, invoice.Username)
	if err != nil {
		return c.Send("⚠️ خطا در تغییر لینک.")
	}

	return c.Send(fmt.Sprintf("✅ لینک جدید:\n<code>%s</code>", newLink), tele.ModeHTML)
}

// ── Discount Code ─────────────────────────────────────────────────────

func (b *Bot) handleDiscountCode(c tele.Context, user *models.User, text string) error {
	chatID := user.ID
	code := strings.TrimSpace(text)

	// Find discount
	var discount models.Discount
	if err := b.repos.Setting.DB().Where("code = ?", code).First(&discount).Error; err != nil {
		return c.Send("⚠️ کد تخفیف نامعتبر است.")
	}

	// Check usage limit
	limitUse := parseIntSafe(discount.LimitUse)
	limitUsed := parseIntSafe(discount.LimitUsed)
	if limitUse > 0 && limitUsed >= limitUse {
		return c.Send("⚠️ این کد تخفیف قبلاً استفاده شده است.")
	}

	// Check if user already used this code
	var consumed models.GiftCodeConsumed
	if err := b.repos.Setting.DB().Where("code = ? AND id_user = ?", code, chatID).First(&consumed).Error; err == nil {
		return c.Send("⚠️ شما قبلاً از این کد استفاده کرده‌اید.")
	}

	// Apply discount
	amount := parseIntSafe(discount.Price)
	_ = b.repos.User.UpdateBalance(chatID, amount)

	// Record usage
	b.repos.Setting.DB().Create(&models.GiftCodeConsumed{
		Code:   code,
		IDUser: chatID,
	})

	// Increment used counter
	b.repos.Setting.DB().Model(&models.Discount{}).Where("code = ?", code).
		Update("limitused", fmt.Sprintf("%d", limitUsed+1))

	_ = b.repos.User.UpdateStep(chatID, "none")

	return c.Send(fmt.Sprintf("✅ کد تخفیف اعمال شد.\n💰 مبلغ %s تومان به کیف پول شما اضافه شد.", formatNumber(amount)))
}

// ── Custom Name ───────────────────────────────────────────────────────

func (b *Bot) handleCustomName(c tele.Context, user *models.User, text string) error {
	// Sanitize name
	name := utils.SanitizeUsername(text)
	if name == "" || len(name) > 30 {
		return c.Send("⚠️ نام نامعتبر است. حداکثر 30 کاراکتر.")
	}

	_ = b.repos.User.Update(user.ID, map[string]interface{}{
		"Processing_value": name,
		"step":             "buy_service",
	})

	return b.sendBuyMenu(c, user)
}

// ── Referral ──────────────────────────────────────────────────────────

func (b *Bot) showReferralInfo(c tele.Context, user *models.User) error {
	setting, _ := b.repos.Setting.GetSettings()
	affiliateText, _ := b.repos.Setting.GetText("textaffiliates")

	refLink := fmt.Sprintf("https://t.me/%s?start=ref_%s", b.tb.Me.Username, user.ID)

	if affiliateText == "" {
		affiliateText = "📢 <b>معرفی به دوستان</b>\n\n" +
			"با معرفی ربات به دوستانتان، از خرید آن‌ها کمیسیون دریافت کنید!\n\n"
	}

	affiliateText += fmt.Sprintf(
		"\n🔗 لینک دعوت:\n<code>%s</code>\n\n"+
			"👥 تعداد زیرمجموعه: %s",
		refLink,
		user.AffiliatesCount,
	)

	if setting != nil && setting.AffiliatesPercentage != "" {
		affiliateText += fmt.Sprintf("\n💰 درصد کمیسیون: %s%%", setting.AffiliatesPercentage)
	}

	return c.Send(affiliateText, tele.ModeHTML)
}

// ── Helpers ───────────────────────────────────────────────────────────

func (b *Bot) getPanelClient(panelModel *models.Panel) (panel.PanelClient, error) {
	key := panelModel.CodePanel
	if client, ok := b.panels[key]; ok {
		return client, nil
	}

	client, err := panel.PanelFactory(panelModel)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := client.Authenticate(ctx); err != nil {
		return nil, fmt.Errorf("auth failed for panel %s: %w", panelModel.NamePanel, err)
	}

	b.panels[key] = client
	return client, nil
}

func (b *Bot) generateUsername(panelModel *models.Panel, user *models.User) string {
	method := panelModel.MethodUsername

	switch method {
	case "آیدی عددی + حروف و عدد رندوم", "1":
		return fmt.Sprintf("%s_%s", user.ID, utils.RandomCode(5))
	case "نام کاربری + عدد به ترتیب", "2":
		counter := parseIntSafe(panelModel.Connection) + 1
		b.repos.Panel.IncrementCounter(panelModel.CodePanel)
		name := user.Username
		if name == "" {
			name = user.ID
		}
		return fmt.Sprintf("%s%d", utils.SanitizeUsername(name), counter)
	case "متن دلخواه + عدد رندوم", "5":
		prefix := panelModel.NameCustom
		if prefix == "" {
			prefix = "user"
		}
		return fmt.Sprintf("%s_%s", prefix, utils.RandomCode(5))
	case "متن دلخواه + عدد ترتیبی", "6":
		prefix := panelModel.NameCustom
		if prefix == "" {
			prefix = "user"
		}
		counter := parseIntSafe(panelModel.Connection) + 1
		b.repos.Panel.IncrementCounter(panelModel.CodePanel)
		return fmt.Sprintf("%s%d", prefix, counter)
	case "آیدی عددی+عدد ترتیبی", "7":
		counter := parseIntSafe(panelModel.Connection) + 1
		b.repos.Panel.IncrementCounter(panelModel.CodePanel)
		return fmt.Sprintf("%s%d", user.ID, counter)
	default:
		return fmt.Sprintf("%s_%s", user.ID, utils.RandomCode(5))
	}
}

func (b *Bot) handleAffiliateCommission(user *models.User, price int) {
	if user.Affiliates == "" || user.Affiliates == "0" {
		return
	}

	setting, _ := b.repos.Setting.GetSettings()
	if setting == nil || setting.AffiliatesStatus != "onaffiliates" {
		return
	}

	pct := parseIntSafe(setting.AffiliatesPercentage)
	if pct <= 0 {
		return
	}

	commission := price * pct / 100
	if commission <= 0 {
		return
	}

	_ = b.repos.User.UpdateBalance(user.Affiliates, commission)

	b.botAPI.SendMessage(user.Affiliates,
		fmt.Sprintf("💰 کمیسیون %s تومان از خرید زیرمجموعه به کیف پول شما اضافه شد.", formatNumber(commission)),
		nil,
	)
}

func (b *Bot) reportToChannel(text string) {
	setting, err := b.repos.Setting.GetSettings()
	if err != nil || setting.ChannelReport == "" {
		return
	}

	topicID, _ := b.repos.Setting.GetTopicID("report")

	params := map[string]interface{}{
		"chat_id":    setting.ChannelReport,
		"text":       text,
		"parse_mode": "HTML",
	}
	if topicID != "" {
		params["message_thread_id"] = topicID
	}

	b.botAPI.Call("sendMessage", params)
}

func (b *Bot) validateGatewayAmount(method string, amount int) error {
	rangeKeys := map[string][2]string{
		"card":          {"minbalancecart", "maxbalancecart"},
		"zarinpal":      {"minbalancezarinpal", "maxbalancezarinpal"},
		"nowpayments":   {"minbalancenowpayment", "maxbalancenowpayment"},
		"plisio":        {"minbalanceplisio", "maxbalanceplisio"},
		"aqayepardakht": {"minbalanceaqayepardakht", "maxbalanceaqayepardakht"},
		"iranpay1":      {"minbalanceiranpay1", "maxbalanceiranpay1"},
		"iranpay2":      {"minbalanceiranpay2", "maxbalanceiranpay2"},
		"digitaltron":   {"minbalancedigitaltron", "maxbalancedigitaltron"},
		"stars":         {"minbalancestar", "maxbalancestar"},
	}

	keys, ok := rangeKeys[method]
	if !ok {
		return nil
	}

	minRaw, _ := b.repos.Setting.GetPaySetting(keys[0])
	maxRaw, _ := b.repos.Setting.GetPaySetting(keys[1])
	minValue := parseIntSafe(minRaw)
	maxValue := parseIntSafe(maxRaw)

	if minValue > 0 && amount < minValue {
		return fmt.Errorf("⚠️ حداقل مبلغ این روش پرداخت %s تومان است.", formatNumber(minValue))
	}
	if maxValue > 0 && amount > maxValue {
		return fmt.Errorf("⚠️ حداکثر مبلغ این روش پرداخت %s تومان است.", formatNumber(maxValue))
	}

	return nil
}

func (b *Bot) paymentMethodStorageName(method string) string {
	switch method {
	case "card":
		return "cart to cart"
	case "wallet":
		return "wallet"
	case "zarinpal":
		return "zarinpal"
	case "nowpayments":
		return "nowpayment"
	case "plisio":
		return "plisio"
	case "aqayepardakht":
		return "aqayepardakht"
	case "iranpay1":
		return "Currency Rial 1"
	case "iranpay2":
		return "Currency Rial 2"
	case "digitaltron":
		return "arze digital offline"
	case "stars":
		return "Star Telegram"
	default:
		return method
	}
}

func (b *Bot) paymentMethodLabel(storageName string) string {
	switch strings.ToLower(strings.TrimSpace(storageName)) {
	case "cart to cart":
		return "کارت به کارت"
	case "wallet":
		return "کیف پول"
	case "zarinpal":
		return "زرین پال"
	case "nowpayment", "nowpayments":
		return "NowPayments"
	case "plisio":
		return "Plisio"
	case "aqayepardakht":
		return "آقای پرداخت"
	case "currency rial 1":
		return "ایران‌پی ۱"
	case "currency rial 2":
		return "ترونادو"
	case "arze digital offline":
		return "ارزی آفلاین"
	case "star telegram":
		return "ستاره تلگرام"
	default:
		if storageName == "" {
			return "-"
		}
		return storageName
	}
}

func (b *Bot) firstPaySetting(keys ...string) string {
	for _, key := range keys {
		value, _ := b.repos.Setting.GetPaySetting(key)
		value = strings.TrimSpace(value)
		if value != "" && value != "0" {
			return value
		}
	}
	return ""
}

func (b *Bot) getCryptoRates(ctx context.Context) (usdToman int, trxToman float64, err error) {
	_ = ctx
	client := httpclient.New().WithTimeout(20 * time.Second)

	wallexRaw, err := client.Get("https://api.wallex.ir/v1/markets")
	if err != nil {
		return 0, 0, err
	}

	var wallex map[string]interface{}
	if err := json.Unmarshal(wallexRaw, &wallex); err != nil {
		return 0, 0, err
	}

	resultMap, _ := wallex["result"].(map[string]interface{})
	symbolsMap, _ := resultMap["symbols"].(map[string]interface{})
	usdttmnMap, _ := symbolsMap["USDTTMN"].(map[string]interface{})
	statsMap, _ := usdttmnMap["stats"].(map[string]interface{})
	usdPrice := parseFloatFromAny(statsMap["lastPrice"])
	if usdPrice <= 0 {
		return 0, 0, fmt.Errorf("invalid USDTTMN rate")
	}

	tronRaw, err := client.Get("https://api.diadata.org/v1/assetQuotation/Tron/0x0000000000000000000000000000000000000000")
	if err != nil {
		return 0, 0, err
	}

	var tron map[string]interface{}
	if err := json.Unmarshal(tronRaw, &tron); err != nil {
		return 0, 0, err
	}
	tronPriceUSD := parseFloatFromAny(tron["Price"])
	if tronPriceUSD <= 0 {
		return 0, 0, fmt.Errorf("invalid TRX/USD rate")
	}

	usdToman = int(math.Round(usdPrice))
	trxToman = tronPriceUSD * float64(usdToman)
	if usdToman <= 0 || trxToman <= 0 {
		return 0, 0, fmt.Errorf("invalid computed rates")
	}
	return usdToman, trxToman, nil
}

func parseStringFromAny(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		if math.Mod(v, 1) == 0 {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		out := strings.TrimSpace(fmt.Sprintf("%v", value))
		if out == "<nil>" {
			return ""
		}
		return out
	}
}

func parseFloatFromAny(value interface{}) float64 {
	switch v := value.(type) {
	case nil:
		return 0
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		cleaned := strings.ReplaceAll(strings.TrimSpace(v), ",", "")
		f, _ := strconv.ParseFloat(cleaned, 64)
		return f
	default:
		s := strings.ReplaceAll(strings.TrimSpace(fmt.Sprintf("%v", value)), ",", "")
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
}

// containsAny checks if text contains any of the given substrings (case-insensitive).
func containsAny(text string, subs ...string) bool {
	lower := strings.ToLower(text)
	for _, sub := range subs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
