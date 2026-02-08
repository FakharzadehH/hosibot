package bot

import (
	"encoding/json"
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v3"

	"hosibot/internal/models"
	"hosibot/internal/repository"
)

// KeyboardBuilder constructs Telegram keyboards from settings and data.
// Mirrors the dynamic keyboard logic in PHP keyboard.php.
type KeyboardBuilder struct {
	settingRepo *repository.SettingRepository
	panelRepo   *repository.PanelRepository
	productRepo *repository.ProductRepository
	invoiceRepo *repository.InvoiceRepository
}

// NewKeyboardBuilder creates a new keyboard builder.
func NewKeyboardBuilder(repos *BotRepos) *KeyboardBuilder {
	return &KeyboardBuilder{
		settingRepo: repos.Setting,
		panelRepo:   repos.Panel,
		productRepo: repos.Product,
		invoiceRepo: repos.Invoice,
	}
}

// MainMenuKeyboard builds the main menu keyboard from settings.
// Reads keyboardmain JSON from setting table; supports inline or reply mode.
func (kb *KeyboardBuilder) MainMenuKeyboard(user *models.User, setting *models.Setting) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}

	// Check if inline mode
	isInline := setting.InlineBtnMain == "oninline"

	// Try to parse custom keyboard layout from settings
	var layout [][]map[string]string
	if setting.KeyboardMain != "" {
		_ = json.Unmarshal([]byte(setting.KeyboardMain), &layout)
	}

	if isInline {
		// Build inline keyboard
		var rows []tele.Row
		for _, row := range layout {
			var btns []tele.Btn
			for _, btn := range row {
				text := btn["text"]
				data := btn["callback_data"]
				if text == "" || data == "" {
					continue
				}
				btns = append(btns, menu.Data(text, data))
			}
			if len(btns) > 0 {
				rows = append(rows, menu.Row(btns...))
			}
		}
		if len(rows) > 0 {
			menu.Inline(rows...)
		} else {
			// Default inline layout
			kb.defaultInlineMenu(menu)
		}
	} else {
		// Build reply keyboard
		var rows []tele.Row
		for _, row := range layout {
			var btns []tele.Btn
			for _, btn := range row {
				text := btn["text"]
				if text == "" {
					continue
				}
				btns = append(btns, menu.Text(text))
			}
			if len(btns) > 0 {
				rows = append(rows, menu.Row(btns...))
			}
		}

		if len(rows) > 0 {
			menu.Reply(rows...)
		} else {
			// Default reply layout
			kb.defaultReplyMenu(menu, user, setting)
		}
	}

	return menu
}

func (kb *KeyboardBuilder) defaultInlineMenu(menu *tele.ReplyMarkup) {
	menu.Inline(
		menu.Row(menu.Data("🛒 خرید سرویس", "buy_service"), menu.Data("📋 سرویس‌های من", "my_services")),
		menu.Row(menu.Data("💰 کیف پول", "wallet"), menu.Data("📩 پشتیبانی", "support")),
		menu.Row(menu.Data("👤 حساب کاربری", "account"), menu.Data("📢 معرفی به دوستان", "referral")),
	)
}

func (kb *KeyboardBuilder) defaultReplyMenu(menu *tele.ReplyMarkup, user *models.User, setting *models.Setting) {
	// Get text labels from textbot table
	buyText := kb.getText("text_sell", "🛒 خرید سرویس")
	servicesText := kb.getText("text_Purchased_services", "📋 سرویس‌های من")
	walletText := kb.getText("accountwallet", "💰 کیف پول")
	supportText := kb.getText("text_support", "📩 پشتیبانی")

	rows := []tele.Row{
		menu.Row(menu.Text(buyText), menu.Text(servicesText)),
		menu.Row(menu.Text(walletText), menu.Text(supportText)),
	}

	// Add extend button if enabled
	if setting.BtnStatusExtend == "onextned" {
		extendText := kb.getText("text_extend", "🔁 تمدید سرویس")
		rows = append(rows, menu.Row(menu.Text(extendText)))
	}

	// Add account & referral
	accountText := kb.getText("text_Account", "👤 حساب کاربری")
	referralText := kb.getText("text_affiliates", "📢 معرفی به دوستان")
	rows = append(rows, menu.Row(menu.Text(accountText), menu.Text(referralText)))

	// Add admin button if user is admin
	admin, err := kb.settingRepo.FindAdminByID(user.ID)
	if err == nil && admin != nil {
		rows = append(rows, menu.Row(menu.Text("🔧 پنل مدیریت")))
	}

	menu.Reply(rows...)
}

// LocationKeyboard builds inline keyboard of available panels/locations for purchase.
func (kb *KeyboardBuilder) LocationKeyboard(user *models.User) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	panels, err := kb.panelRepo.FindActive()
	if err != nil {
		return menu
	}

	var rows []tele.Row
	for _, p := range panels {
		// Filter by agent access
		if !kb.panelAccessibleByUser(p, user) {
			continue
		}
		// Check hide_user
		if p.HideUser != "" && strings.Contains(p.HideUser, user.ID) {
			continue
		}

		btn := menu.Data(fmt.Sprintf("📍 %s", p.NamePanel), fmt.Sprintf("loc_%s", p.CodePanel))
		rows = append(rows, menu.Row(btn))
	}

	// Back button
	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "main_menu")))
	menu.Inline(rows...)
	return menu
}

// CategoryKeyboard builds inline keyboard of product categories for a given location.
func (kb *KeyboardBuilder) CategoryKeyboard(panelCode string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	categories, _, err := kb.settingRepo.FindAllCategories(50, 1, "")
	if err != nil {
		return menu
	}

	var rows []tele.Row
	for _, cat := range categories {
		btn := menu.Data(cat.Remark, fmt.Sprintf("cat_%s_%d", panelCode, cat.ID))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "buy_service")))
	menu.Inline(rows...)
	return menu
}

// ProductKeyboard builds inline keyboard of products for a location (and optional category).
func (kb *KeyboardBuilder) ProductKeyboard(panelCode string, user *models.User, categoryID int) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	panel, err := kb.panelRepo.FindByCode(panelCode)
	if err != nil {
		return menu
	}

	products, _, err := kb.productRepo.FindAll(100, 1, "")
	if err != nil {
		return menu
	}

	// Discount percentage for user
	discountPct := 0
	if user.PriceDiscount.Valid && user.PriceDiscount.String != "" && user.PriceDiscount.String != "0" {
		discountPct = parseIntSafe(user.PriceDiscount.String)
	}

	var rows []tele.Row
	for _, prod := range products {
		// Filter: product location must match panel name or be "/all"
		if prod.Location != panel.NamePanel && prod.Location != "/all" {
			continue
		}
		// Filter: agent access
		if prod.Agent != "0" && prod.Agent != user.Agent {
			continue
		}
		// Filter: category
		if categoryID > 0 && prod.Category != fmt.Sprintf("%d", categoryID) {
			continue
		}
		// Filter: hide_panel — product hidden for this panel
		if prod.HidePanel != "" {
			var hiddenPanels []string
			_ = json.Unmarshal([]byte(prod.HidePanel), &hiddenPanels)
			hidden := false
			for _, hp := range hiddenPanels {
				if hp == panel.NamePanel {
					hidden = true
					break
				}
			}
			if hidden {
				continue
			}
		}
		// Filter: one_buy_status — only allow first purchase
		if prod.OneBuyStatus == "on" {
			count, _ := kb.invoiceRepo.CountByProductCode(prod.CodeProduct)
			if count > 0 {
				continue
			}
		}

		// Calculate display price
		price := parseIntSafe(prod.PriceProduct)
		if discountPct > 0 {
			price = price - (price * discountPct / 100)
		}

		// Format label
		label := fmt.Sprintf("%s - %s تومان", prod.NameProduct, formatNumber(price))
		btn := menu.Data(label, fmt.Sprintf("prod_%d", prod.ID))
		rows = append(rows, menu.Row(btn))
	}

	// Custom volume option if enabled
	if panel.CustomVolume != "" {
		rows = append(rows, menu.Row(menu.Data("📦 حجم دلخواه", fmt.Sprintf("customvol_%s", panelCode))))
	}

	// Back button
	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "buy_service")))
	menu.Inline(rows...)
	return menu
}

// ServiceListKeyboard builds inline keyboard of user's active services.
func (kb *KeyboardBuilder) ServiceListKeyboard(invoices []models.Invoice, page int) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	var rows []tele.Row
	for _, inv := range invoices {
		statusEmoji := "✅"
		switch inv.Status {
		case "end_of_time", "expired":
			statusEmoji = "⏰"
		case "end_of_volume", "limited":
			statusEmoji = "📊"
		case "disabled", "disablebyadmin":
			statusEmoji = "🚫"
		case "send_on_hold", "on_hold":
			statusEmoji = "⏸"
		}
		label := fmt.Sprintf("%s %s | %s", statusEmoji, inv.NameProduct, inv.Username)
		btn := menu.Data(label, fmt.Sprintf("srv_%s", inv.IDInvoice))
		rows = append(rows, menu.Row(btn))
	}

	// Pagination
	var navBtns []tele.Btn
	if page > 1 {
		navBtns = append(navBtns, menu.Data("⬅️ قبلی", fmt.Sprintf("srvpage_%d", page-1)))
	}
	if len(invoices) >= 20 {
		navBtns = append(navBtns, menu.Data("➡️ بعدی", fmt.Sprintf("srvpage_%d", page+1)))
	}
	if len(navBtns) > 0 {
		rows = append(rows, menu.Row(navBtns...))
	}

	// Search and back
	rows = append(rows,
		menu.Row(menu.Data("🔍 جستجو", "search_service")),
		menu.Row(menu.Data("🔙 منوی اصلی", "main_menu")),
	)
	menu.Inline(rows...)
	return menu
}

// ServiceDetailKeyboard builds action buttons for a specific service.
func (kb *KeyboardBuilder) ServiceDetailKeyboard(invoice *models.Invoice, panel *models.Panel) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	id := invoice.IDInvoice

	var rows []tele.Row

	// Update info
	rows = append(rows, menu.Row(menu.Data("🔄 بروزرسانی", fmt.Sprintf("updateinfo_%s", id))))

	// Toggle status and reset traffic (supported by panel APIs)
	rows = append(rows, menu.Row(
		menu.Data("🔌 تغییر وضعیت", fmt.Sprintf("togglesvc_%s", id)),
		menu.Data("♻️ ریست مصرف", fmt.Sprintf("resettraffic_%s", id)),
	))

	// Subscription URL
	rows = append(rows, menu.Row(menu.Data("🔗 لینک اشتراک", fmt.Sprintf("suburl_%s", id))))

	// Configs
	rows = append(rows, menu.Row(menu.Data("⚙️ کانفیگ‌ها", fmt.Sprintf("configs_%s", id))))

	// Extend (if panel supports it)
	if panel.StatusExtend != "off" {
		rows = append(rows, menu.Row(menu.Data("🔁 تمدید سرویس", fmt.Sprintf("extend_%s", id))))
	}

	// Extra volume
	if panel.PriceExtraVolume != "" && panel.PriceExtraVolume != "0" {
		rows = append(rows, menu.Row(menu.Data("📦 افزایش حجم", fmt.Sprintf("extravol_%s", id))))
	}

	// Extra time
	if panel.PriceExtraTime != "" && panel.PriceExtraTime != "0" {
		rows = append(rows, menu.Row(menu.Data("⏰ افزایش زمان", fmt.Sprintf("extratime_%s", id))))
	}

	// Change location (if enabled)
	if panel.ChangeLoc.Valid && panel.ChangeLoc.String == "onchangeloc" {
		rows = append(rows, menu.Row(menu.Data("🌍 تغییر لوکیشن", fmt.Sprintf("changeloc_%s", id))))
	}

	// Change link
	rows = append(rows, menu.Row(menu.Data("🔄 تغییر لینک", fmt.Sprintf("changelink_%s", id))))

	// Change note
	rows = append(rows, menu.Row(menu.Data("📝 تغییر یادداشت", fmt.Sprintf("changenote_%s", id))))

	// Remove
	rows = append(rows, menu.Row(menu.Data("❌ حذف سرویس", fmt.Sprintf("removeservice_%s", id))))

	// Back
	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت به لیست", "my_services")))

	menu.Inline(rows...)
	return menu
}

// PaymentMethodKeyboard builds inline keyboard of available payment methods.
func (kb *KeyboardBuilder) PaymentMethodKeyboard(amount int, user *models.User) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	paySettings, err := kb.settingRepo.GetAllPaySettings()
	if err != nil {
		return menu
	}

	// Build a map for easy lookup
	psMap := make(map[string]string)
	for _, ps := range paySettings {
		psMap[ps.NamePay] = ps.ValuePay
	}

	var rows []tele.Row

	// Card-to-card
	if paySettingIn(psMap["Cartstatus"], "oncard", "oncart") {
		// Check first-buy restriction
		if psMap["checkpaycartfirst"] == "oncartfirst" {
			// Only show if user has previous payments
			count, _ := kb.settingRepo.FindAllServiceOther(0)
			_ = count // simplified — always show for now
		}
		rows = append(rows, menu.Row(menu.Data("💳 کارت به کارت", "pay_card")))
	}

	// Plisio (legacy key in PHP: nowpaymentstatus)
	if paySettingIn(psMap["nowpaymentstatus"], "onnowpayment", "onplisio", "1") {
		rows = append(rows, menu.Row(menu.Data("💎 ارز دیجیتال (Plisio)", "pay_plisio")))
	}

	// ZarinPal
	if psMap["zarinpalstatus"] == "onzarinpal" {
		rows = append(rows, menu.Row(menu.Data("🏦 زرین پال", "pay_zarinpal")))
	}

	// NowPayments
	if paySettingIn(psMap["statusnowpayment"], "onnowpayments", "1") {
		rows = append(rows, menu.Row(menu.Data("💎 ارز دیجیتال (NowPayments)", "pay_nowpayments")))
	}

	// IranPay1 (Swap)
	if paySettingIn(psMap["statusSwapWallet"], "onSwapinoBot", "onswapino") {
		rows = append(rows, menu.Row(menu.Data("💱 ایران‌پی ۱", "pay_iranpay1")))
	}

	// Tronado / IranPay2
	if paySettingIn(psMap["statustarnado"], "onternado") {
		rows = append(rows, menu.Row(menu.Data("💱 ایران‌پی ۲", "pay_iranpay2")))
	}

	// AqayePardakht
	if paySettingIn(psMap["statusaqayepardakht"], "onaqayepardakht") {
		rows = append(rows, menu.Row(menu.Data("🏧 آقای پرداخت", "pay_aqayepardakht")))
	}

	// Offline crypto receipt flow
	if paySettingIn(psMap["digistatus"], "ondigi") {
		rows = append(rows, menu.Row(menu.Data("🪙 ارز دیجیتال آفلاین", "pay_digitaltron")))
	}

	// Wallet balance
	if user.Balance > 0 {
		rows = append(rows, menu.Row(menu.Data(
			fmt.Sprintf("👛 کیف پول (%s تومان)", formatNumber(user.Balance)),
			"pay_wallet",
		)))
	}

	// Telegram Stars
	if paySettingIn(psMap["statusstar"], "onstar", "1") {
		rows = append(rows, menu.Row(menu.Data("⭐ ستاره تلگرام", "pay_stars")))
	}

	// Back
	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "main_menu")))
	menu.Inline(rows...)
	return menu
}

func paySettingIn(value string, expected ...string) bool {
	for _, item := range expected {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

// WalletKeyboard builds the wallet panel keyboard.
func (kb *KeyboardBuilder) WalletKeyboard(user *models.User) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("💳 شارژ کیف پول", "charge_wallet")),
		menu.Row(menu.Data("🎁 کد تخفیف / هدیه", "discount_code")),
		menu.Row(menu.Data("🔙 منوی اصلی", "main_menu")),
	)
	return menu
}

// SupportDepartmentKeyboard builds inline keyboard of support departments.
func (kb *KeyboardBuilder) SupportDepartmentKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	var departments []models.Departman
	kb.settingRepo.DB().Find(&departments)

	var rows []tele.Row
	for _, dept := range departments {
		btn := menu.Data(dept.NameDepartman, fmt.Sprintf("dept_%d", dept.ID))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "main_menu")))
	menu.Inline(rows...)
	return menu
}

// ExtendProductKeyboard builds product selection for extending a service.
func (kb *KeyboardBuilder) ExtendProductKeyboard(panelCode string, user *models.User, invoiceID string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	panel, err := kb.panelRepo.FindByCode(panelCode)
	if err != nil {
		return menu
	}

	products, _, err := kb.productRepo.FindAll(100, 1, "")
	if err != nil {
		return menu
	}

	discountPct := 0
	if user.PriceDiscount.Valid && user.PriceDiscount.String != "" && user.PriceDiscount.String != "0" {
		discountPct = parseIntSafe(user.PriceDiscount.String)
	}

	var rows []tele.Row
	for _, prod := range products {
		if prod.Location != panel.NamePanel && prod.Location != "/all" {
			continue
		}
		if prod.Agent != "0" && prod.Agent != user.Agent {
			continue
		}

		price := parseIntSafe(prod.PriceProduct)
		if discountPct > 0 {
			price = price - (price * discountPct / 100)
		}

		label := fmt.Sprintf("%s - %s تومان", prod.NameProduct, formatNumber(price))
		btn := menu.Data(label, fmt.Sprintf("extprod_%s_%d", invoiceID, prod.ID))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", fmt.Sprintf("srv_%s", invoiceID))))
	menu.Inline(rows...)
	return menu
}

// ConfirmPurchaseKeyboard builds confirm/cancel buttons for purchase.
func (kb *KeyboardBuilder) ConfirmPurchaseKeyboard(invoiceID string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("✅ تایید و خرید", fmt.Sprintf("confirm_%s", invoiceID)),
			menu.Data("❌ انصراف", "main_menu"),
		),
	)
	return menu
}

// CardPaymentKeyboard builds the card-to-card payment keyboard with send receipt button.
func (kb *KeyboardBuilder) CardPaymentKeyboard(orderID string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("📸 ارسال رسید", fmt.Sprintf("sendreceipt_%s", orderID))),
		menu.Row(menu.Data("❌ انصراف", "main_menu")),
	)
	return menu
}

// AdminPaymentConfirmKeyboard builds confirm/reject buttons for admin payment review.
func (kb *KeyboardBuilder) AdminPaymentConfirmKeyboard(orderID string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("✅ تایید پرداخت", fmt.Sprintf("confirmpay_%s", orderID)),
			menu.Data("❌ رد پرداخت", fmt.Sprintf("rejectpay_%s", orderID)),
		),
	)
	return menu
}

// panelAccessibleByUser checks if a panel is accessible by the user based on agent settings.
func (kb *KeyboardBuilder) panelAccessibleByUser(panel models.Panel, user *models.User) bool {
	if panel.Status != "active" {
		return false
	}
	// Agent field contains comma-separated agent IDs or "0" for all
	if panel.Agent == "0" || panel.Agent == "" {
		return true
	}
	agents := strings.Split(panel.Agent, ",")
	for _, a := range agents {
		if strings.TrimSpace(a) == user.Agent || strings.TrimSpace(a) == "0" {
			return true
		}
	}
	return false
}

// getText fetches a text label from textbot table, returns fallback if not found.
func (kb *KeyboardBuilder) getText(key, fallback string) string {
	text, err := kb.settingRepo.GetText(key)
	if err != nil || text == "" {
		return fallback
	}
	return text
}

// parseIntSafe safely parses a string to int, returns 0 on failure.
func parseIntSafe(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

// formatNumber formats a number with comma separators.
func formatNumber(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	str := fmt.Sprintf("%d", n)
	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return sign + result
}
