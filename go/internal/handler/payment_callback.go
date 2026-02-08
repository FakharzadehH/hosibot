package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/models"
	"hosibot/internal/panel"
	"hosibot/internal/payment"
	"hosibot/internal/pkg/httpclient"
	"hosibot/internal/pkg/telegram"
	"hosibot/internal/repository"
)

// PaymentCallbackHandler handles gateway callbacks.
type PaymentCallbackHandler struct {
	repos   *CallbackRepos
	botAPI  *telegram.BotAPI
	gateway payment.Gateway
	logger  *zap.Logger
}

// CallbackRepos bundles repositories for payment callbacks.
type CallbackRepos struct {
	User    *repository.UserRepository
	Product *repository.ProductRepository
	Invoice *repository.InvoiceRepository
	Payment *repository.PaymentRepository
	Panel   *repository.PanelRepository
	Setting *repository.SettingRepository
}

// NewPaymentCallbackHandler creates a new payment callback handler.
func NewPaymentCallbackHandler(
	repos *CallbackRepos,
	botAPI *telegram.BotAPI,
	logger *zap.Logger,
) *PaymentCallbackHandler {
	return &PaymentCallbackHandler{
		repos:  repos,
		botAPI: botAPI,
		logger: logger,
	}
}

// ── ZarinPal callback ────────────────────────────────────────────────

func (h *PaymentCallbackHandler) ZarinPalCallback(c echo.Context) error {
	authority := c.QueryParam("Authority")
	statusParam := c.QueryParam("Status")

	if authority == "" {
		return h.renderPaymentResult(c, "خطا", "پارامترهای نامعتبر", "", 0)
	}

	// Find payment by authority (stored in dec_not_confirmed)
	var paymentReport models.PaymentReport
	if err := h.repos.Setting.DB().
		Where("dec_not_confirmed = ?", authority).
		First(&paymentReport).Error; err != nil {
		return h.renderPaymentResult(c, "خطا", "تراکنش یافت نشد", authority, 0)
	}

	price := parseIntSafe(paymentReport.Price)
	orderID := paymentReport.IDOrder

	if statusParam != "OK" {
		return h.renderPaymentResult(c, "پرداخت انجام نشد", "کاربر از پرداخت منصرف شد", orderID, price)
	}

	// Already paid?
	if paymentReport.PaymentStatus == "paid" {
		return h.renderPaymentResult(c, "پرداخت موفق", "این تراکنش قبلاً پرداخت شده است", orderID, price)
	}

	// Get merchant ID
	merchantID, _ := h.repos.Setting.GetPaySetting("merchant_zarinpal")
	if merchantID == "" {
		return h.renderPaymentResult(c, "خطا", "مرچنت کد تنظیم نشده", orderID, price)
	}

	// Verify with ZarinPal
	gw := payment.NewZarinPalGateway(merchantID, false)
	result, err := gw.VerifyPayment(c.Request().Context(), authority, price)
	if err != nil {
		h.logger.Error("ZarinPal verify error", zap.Error(err))
		return h.renderPaymentResult(c, "خطا", "خطا در تأیید پرداخت", orderID, price)
	}

	if !result.Verified {
		msg := result.Message
		if msg == "" {
			msg = "تأیید پرداخت ناموفق"
		}
		return h.renderPaymentResult(c, "پرداخت ناموفق", msg, orderID, price)
	}

	// Payment verified — process it
	h.processConfirmedPayment(&paymentReport, "zarinpal", result.RefID)

	// Cashback
	h.applyCashback(&paymentReport, "chashbackzarinpal")

	// Report to channel
	user, _ := h.repos.User.FindByID(paymentReport.IDUser)
	username := ""
	if user != nil {
		username = user.Username
	}
	reportText := fmt.Sprintf(
		"💵 پرداخت جدید\n\nآیدی عددی کاربر: %s\nنام کاربری: @%s\nمبلغ: %s تومان\nشماره تراکنش: %s\nروش پرداخت: درگاه زرین پال",
		paymentReport.IDUser, username, formatNumber(price), result.RefID,
	)
	h.reportToChannel(reportText, "paymentreport")

	return h.renderPaymentResult(c, "پرداخت موفق", "از انجام تراکنش متشکریم!", orderID, price)
}

// ── NOWPayments callback ─────────────────────────────────────────────

func (h *PaymentCallbackHandler) NOWPaymentsCallback(c echo.Context) error {
	var data map[string]interface{}
	if err := c.Bind(&data); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	paymentStatus, _ := data["payment_status"].(string)
	if paymentStatus != "finished" {
		return c.JSON(http.StatusOK, map[string]string{"status": "ignored"})
	}

	paymentID, _ := data["payment_id"].(float64)
	invoiceID, _ := data["invoice_id"].(string)

	// Find payment report by invoice_id (stored in dec_not_confirmed)
	var paymentReport models.PaymentReport
	if err := h.repos.Setting.DB().
		Where("dec_not_confirmed = ?", invoiceID).
		First(&paymentReport).Error; err != nil {
		h.logger.Warn("NOWPayments: payment not found", zap.String("invoice_id", invoiceID))
		return c.JSON(http.StatusOK, map[string]string{"status": "not_found"})
	}

	if paymentReport.PaymentStatus == "paid" {
		return c.JSON(http.StatusOK, map[string]string{"status": "already_paid"})
	}

	// Process the payment
	refID := fmt.Sprintf("%.0f", paymentID)
	h.processConfirmedPayment(&paymentReport, "nowpayments", refID)

	// Cashback
	h.applyCashback(&paymentReport, "cashbacknowpayment")

	// Report
	user, _ := h.repos.User.FindByID(paymentReport.IDUser)
	username := ""
	if user != nil {
		username = user.Username
	}

	actuallyPaid, _ := data["actually_paid"].(float64)
	payinHash, _ := data["payin_hash"].(string)

	reportText := fmt.Sprintf(
		"💵 پرداخت جدید\n👤 نام کاربری: @%s\n🆔 آیدی: %s\n💸 مبلغ: %s تومان\n🔗 <a href=\"https://tronscan.org/#/transaction/%s\">لینک پرداخت</a>\n📥 مبلغ واریز: %.4f\n💳 روش پرداخت: nowpayment",
		username, paymentReport.IDUser, formatNumber(parseIntSafe(paymentReport.Price)), payinHash, actuallyPaid,
	)
	h.reportToChannel(reportText, "paymentreport")

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ── Tronado callback ─────────────────────────────────────────────────

func (h *PaymentCallbackHandler) TronadoCallback(c echo.Context) error {
	var data struct {
		PaymentID        string  `json:"PaymentID"`
		Hash             string  `json:"Hash"`
		IsPaid           bool    `json:"IsPaid"`
		TronAmount       float64 `json:"TronAmount"`
		ActualTronAmount float64 `json:"ActualTronAmount"`
	}
	if err := c.Bind(&data); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if data.PaymentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing PaymentID"})
	}

	// Find payment report
	var paymentReport models.PaymentReport
	if err := h.repos.Setting.DB().
		Where("id_order = ?", data.PaymentID).
		First(&paymentReport).Error; err != nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "not_found"})
	}

	if paymentReport.PaymentStatus == "paid" || paymentReport.PaymentStatus == "expire" {
		return c.JSON(http.StatusOK, map[string]string{"status": "already_processed"})
	}

	// Get Tronado API key
	apiKey, _ := h.repos.Setting.GetPaySetting("apiternado")
	if apiKey == "" {
		h.logger.Error("Tronado API key not configured")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "gateway not configured"})
	}

	// Extract order ID from hash: "TrndOrderID_{id}"
	orderIDPart := ""
	if parts := strings.SplitN(data.Hash, "TrndOrderID_", 2); len(parts) == 2 {
		orderIDPart = parts[1]
	}

	// Verify with Tronado API
	verifyBody, _ := json.Marshal(map[string]string{"id": orderIDPart})
	client := httpclient.New().
		WithTimeout(30*time.Second).
		WithHeader("Content-Type", "application/json").
		WithHeader("x-api-key", apiKey)
	resp, err := client.Post("https://bot.tronado.cloud/Order/GetStatus", verifyBody)
	if err != nil {
		h.logger.Error("Tronado verify request failed", zap.Error(err))
		return c.JSON(http.StatusOK, map[string]string{"status": "verify_failed"})
	}

	var verifyResult struct {
		IsPaid     bool    `json:"IsPaid"`
		TronAmount float64 `json:"TronAmount"`
	}
	if err := json.Unmarshal(resp, &verifyResult); err != nil {
		h.logger.Error("Tronado verify parse error", zap.Error(err))
		return c.JSON(http.StatusOK, map[string]string{"status": "parse_error"})
	}

	if !verifyResult.IsPaid || !data.IsPaid || data.TronAmount != verifyResult.TronAmount {
		return c.JSON(http.StatusOK, map[string]string{"status": "not_verified"})
	}

	// Store raw callback data
	callbackJSON, _ := json.Marshal(data)
	_ = h.repos.Payment.UpdateByOrderID(paymentReport.IDOrder, map[string]interface{}{
		"dec_not_confirmed": string(callbackJSON),
	})

	// Process confirmed payment
	h.processConfirmedPayment(&paymentReport, "tronado", data.Hash)

	// Cashback
	h.applyCashback(&paymentReport, "chashbackiranpay2")

	// Report to channel
	user, _ := h.repos.User.FindByID(paymentReport.IDUser)
	username := ""
	if user != nil {
		username = user.Username
	}
	price := parseIntSafe(paymentReport.Price)
	balanceLow := ""
	if data.TronAmount < data.ActualTronAmount {
		balanceLow = "❌ کاربر کمتر از مبلغ تعیین شده واریز کرده است.\n"
	}
	reportText := fmt.Sprintf(
		"💵 پرداخت جدید\n%s👤 نام کاربری کاربر: @%s\n🆔 آیدی عددی کاربر: %s\n💸 مبلغ تراکنش: %s\n🔗 <a href=\"https://tronscan.org/#/transaction/%s\">لینک پرداخت</a>\n📥 مبلغ واریز شده ترون: %.4f\n💳 روش پرداخت: ترونادو",
		balanceLow, username, paymentReport.IDUser, formatNumber(price), data.Hash, data.TronAmount,
	)
	h.reportToChannel(reportText, "paymentreport")

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ── IranPay callback ─────────────────────────────────────────────────

func (h *PaymentCallbackHandler) IranPayCallback(c echo.Context) error {
	var data struct {
		HashID    string `json:"hashid"`
		Authority string `json:"authority"`
		Status    int    `json:"status"`
	}
	if err := c.Bind(&data); err != nil {
		return h.renderPaymentResult(c, "خطا", "درخواست نامعتبر", "", 0)
	}

	if data.HashID == "" {
		return h.renderPaymentResult(c, "خطا", "پارامترهای نامعتبر", "", 0)
	}

	// Find payment report
	var paymentReport models.PaymentReport
	if err := h.repos.Setting.DB().
		Where("id_order = ?", data.HashID).
		First(&paymentReport).Error; err != nil {
		return h.renderPaymentResult(c, "خطا", "تراکنش یافت نشد", data.HashID, 0)
	}

	price := parseIntSafe(paymentReport.Price)
	invoiceID := paymentReport.IDOrder

	if data.Status != 100 {
		return h.renderPaymentResult(c, "ناموفق", "پرداخت انجام نشد", invoiceID, price)
	}

	if paymentReport.PaymentStatus == "paid" {
		return h.renderPaymentResult(c, "پرداخت موفق", "این تراکنش قبلاً پرداخت شده است", invoiceID, price)
	}

	// Get IranPay API key
	apiKey, _ := h.repos.Setting.GetPaySetting("marchent_floypay")
	if apiKey == "" {
		return h.renderPaymentResult(c, "خطا", "کلید API تنظیم نشده", invoiceID, price)
	}

	// Verify with IranPay (tetra98)
	verifyBody, _ := json.Marshal(map[string]interface{}{
		"ApiKey":    apiKey,
		"authority": data.Authority,
		"hashid":    invoiceID,
	})
	client := httpclient.New().
		WithTimeout(30*time.Second).
		WithHeader("Content-Type", "application/json").
		WithHeader("Accept", "application/json")
	resp, err := client.Post("https://tetra98.ir/api/verify", verifyBody)
	if err != nil {
		h.logger.Error("IranPay verify request failed", zap.Error(err))
		return h.renderPaymentResult(c, "خطا", "خطا در تأیید پرداخت", invoiceID, price)
	}

	var verifyResult struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(resp, &verifyResult); err != nil {
		h.logger.Error("IranPay verify parse error", zap.Error(err))
		return h.renderPaymentResult(c, "خطا", "خطا در پردازش پاسخ درگاه", invoiceID, price)
	}

	if verifyResult.Status != 100 {
		return h.renderPaymentResult(c, "ناموفق", "تأیید پرداخت ناموفق", invoiceID, price)
	}

	// Payment verified — process it
	h.processConfirmedPayment(&paymentReport, "iranpay", data.Authority)

	// Cashback
	h.applyCashback(&paymentReport, "chashbackiranpay1")

	// Report to channel
	user, _ := h.repos.User.FindByID(paymentReport.IDUser)
	username := ""
	if user != nil {
		username = user.Username
	}
	reportText := fmt.Sprintf(
		"💵 پرداخت جدید\n\nآیدی عددی کاربر: %s\nنام کاربری کاربر: @%s\nمبلغ تراکنش: %s\nروش پرداخت: ارزی ریالی اول",
		paymentReport.IDUser, username, formatNumber(price),
	)
	h.reportToChannel(reportText, "paymentreport")

	return h.renderPaymentResult(c, "پرداخت موفق", "از انجام تراکنش متشکریم!", invoiceID, price)
}

// ── AqayePardakht callback ───────────────────────────────────────────

func (h *PaymentCallbackHandler) AqayePardakhtCallback(c echo.Context) error {
	invoiceID := c.FormValue("invoice_id")
	transID := c.FormValue("transid")

	if invoiceID == "" {
		return h.renderPaymentResult(c, "خطا", "پارامترهای نامعتبر", "", 0)
	}

	// Find payment report
	var paymentReport models.PaymentReport
	if err := h.repos.Setting.DB().
		Where("id_order = ?", invoiceID).
		First(&paymentReport).Error; err != nil {
		return h.renderPaymentResult(c, "خطا", "تراکنش یافت نشد", invoiceID, 0)
	}

	price := parseIntSafe(paymentReport.Price)

	if paymentReport.PaymentStatus == "paid" {
		return h.renderPaymentResult(c, "پرداخت موفق", "این تراکنش قبلاً پرداخت شده است", invoiceID, price)
	}

	// Get AqayePardakht PIN
	pin, _ := h.repos.Setting.GetPaySetting("merchant_id_aqayepardakht")
	if pin == "" {
		return h.renderPaymentResult(c, "خطا", "کلید API تنظیم نشده", invoiceID, price)
	}

	// Verify with AqayePardakht
	verifyBody, _ := json.Marshal(map[string]interface{}{
		"pin":     pin,
		"amount":  price,
		"transid": transID,
	})
	client := httpclient.New().
		WithTimeout(30*time.Second).
		WithHeader("Content-Type", "application/json")
	resp, err := client.Post("https://panel.aqayepardakht.ir/api/v2/verify", verifyBody)
	if err != nil {
		h.logger.Error("AqayePardakht verify request failed", zap.Error(err))
		return h.renderPaymentResult(c, "خطا", "خطا در تأیید پرداخت", invoiceID, price)
	}

	var verifyResult struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(resp, &verifyResult); err != nil {
		h.logger.Error("AqayePardakht verify parse error", zap.Error(err))
		return h.renderPaymentResult(c, "خطا", "خطا در پردازش پاسخ درگاه", invoiceID, price)
	}

	switch verifyResult.Code {
	case "1":
		// Success — process payment
	case "2":
		return h.renderPaymentResult(c, "پرداخت قبلی", "تراکنش قبلا وریفای و پرداخت شده است", invoiceID, price)
	default:
		return h.renderPaymentResult(c, "پرداخت انجام نشد", "تأیید پرداخت ناموفق", invoiceID, price)
	}

	// Payment verified — process it
	h.processConfirmedPayment(&paymentReport, "aqayepardakht", transID)

	// Cashback
	h.applyCashback(&paymentReport, "chashbackaqaypardokht")

	// Report to channel
	user, _ := h.repos.User.FindByID(paymentReport.IDUser)
	username := ""
	if user != nil {
		username = user.Username
	}
	reportText := fmt.Sprintf(
		"💵 پرداخت جدید\n\nآیدی عددی کاربر: %s\nنام کاربری کاربر: @%s\nمبلغ تراکنش: %s\nروش پرداخت: درگاه آقای پرداخت",
		paymentReport.IDUser, username, formatNumber(price),
	)
	h.reportToChannel(reportText, "paymentreport")

	return h.renderPaymentResult(c, "پرداخت موفق", "از انجام تراکنش متشکریم!", invoiceID, price)
}

// ── Subscription endpoint ────────────────────────────────────────────

func (h *PaymentCallbackHandler) SubscriptionHandler(c echo.Context) error {
	uuid := c.Param("uuid")
	if uuid == "" {
		return c.String(http.StatusNotFound, "Not found")
	}

	// Find invoice by UUID
	var invoice models.Invoice
	if err := h.repos.Setting.DB().
		Where("uuid = ?", uuid).
		First(&invoice).Error; err != nil {
		return c.String(http.StatusNotFound, "Service not found")
	}

	// Get panel and forward to panel's sub link
	panelModel, err := h.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil {
		return c.String(http.StatusNotFound, "Panel not found")
	}

	panelClient, err := panel.PanelFactory(panelModel)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Panel error")
	}

	ctx := c.Request().Context()
	if err := panelClient.Authenticate(ctx); err != nil {
		return c.String(http.StatusInternalServerError, "Panel auth error")
	}

	subLink, err := panelClient.GetSubscriptionLink(ctx, invoice.Username)
	if err != nil {
		return c.String(http.StatusNotFound, "Subscription not found")
	}

	return c.Redirect(http.StatusFound, subLink)
}

// ── Helpers ──────────────────────────────────────────────────────────

// processConfirmedPayment runs the DirectPayment logic for confirmed payments.
func (h *PaymentCallbackHandler) processConfirmedPayment(paymentReport *models.PaymentReport, method, refID string) {
	amount := parseIntSafe(paymentReport.Price)
	userID := paymentReport.IDUser

	// Mark as paid
	_ = h.repos.Payment.UpdateByOrderID(paymentReport.IDOrder, map[string]interface{}{
		"payment_Status": "paid",
		"at_updated":     fmt.Sprintf("%d", time.Now().Unix()),
	})

	// Parse id_invoice: "purpose|data"
	parts := strings.SplitN(paymentReport.IDInvoice, "|", 2)
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
		h.createServiceAfterPayment(userID, extraData, amount)
	case "getextenduser":
		h.extendServiceAfterPayment(userID, extraData, amount)
	default:
		// Charge wallet
		user, _ := h.repos.User.FindByID(userID)
		if user != nil {
			newBalance := user.Balance + amount
			_ = h.repos.User.Update(userID, map[string]interface{}{
				"Balance": newBalance,
			})

			walletText := fmt.Sprintf("✅ مبلغ %s تومان با موفقیت به کیف پول شما اضافه شد.\n💰 موجودی: %s تومان",
				formatNumber(amount), formatNumber(newBalance))
			h.botAPI.SendMessage(userID, walletText, nil)
		}
	}

	// Delete instruction message if exists
	if paymentReport.MessageID > 0 {
		h.botAPI.DeleteMessage(userID, paymentReport.MessageID)
	}
}

func (h *PaymentCallbackHandler) createServiceAfterPayment(userID, username string, price int) {
	user, _ := h.repos.User.FindByID(userID)
	if user == nil {
		return
	}

	productCode := user.ProcessingValueTwo
	panelCode := user.ProcessingValueFour

	product, _ := h.repos.Product.FindByCode(productCode)
	if product == nil {
		return
	}

	panelModel, _ := h.repos.Panel.FindByCode(panelCode)
	if panelModel == nil {
		return
	}

	panelClient, err := panel.PanelFactory(panelModel)
	if err != nil {
		h.logger.Error("Panel factory error", zap.Error(err))
		return
	}

	ctx := fmt.Sprintf("payment-callback-%s", userID)
	_ = ctx

	cctx := make(map[string]string)
	_ = cctx

	bgCtx := createBgContext()
	if err := panelClient.Authenticate(bgCtx); err != nil {
		h.logger.Error("Panel auth error", zap.Error(err))
		return
	}

	volumeGB := parseIntSafe(product.VolumeConstraint)
	timeDays := parseIntSafe(product.ServiceTime)

	var inbounds map[string][]string
	if panelModel.Inbounds != "" {
		_ = json.Unmarshal([]byte(panelModel.Inbounds), &inbounds)
	}

	var proxies map[string]string
	if panelModel.Proxies != "" {
		_ = json.Unmarshal([]byte(panelModel.Proxies), &proxies)
	}

	req := panel.CreateUserRequest{
		Username:   username,
		DataLimit:  int64(volumeGB) * 1024 * 1024 * 1024,
		ExpireDays: timeDays,
		Inbounds:   inbounds,
		Proxies:    proxies,
	}

	panelUser, err := panelClient.CreateUser(bgCtx, req)
	if err != nil {
		h.logger.Error("Failed to create user on panel", zap.Error(err))
		h.botAPI.SendMessage(userID, "❌ خطا در ایجاد سرویس. لطفاً با پشتیبانی تماس بگیرید.", nil)
		return
	}

	// Create invoice
	invoiceID := fmt.Sprintf("INV-%d-%s", time.Now().Unix(), username)
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
		UUID:            panelUser.SubLink,
		Status:          "active",
	}
	_ = h.repos.Invoice.Create(invoice)

	// Increment panel counter
	_ = h.repos.Panel.IncrementCounter(panelModel.CodePanel)

	// Send success message
	afterPayText, _ := h.repos.Setting.GetText("textafterpay")
	if afterPayText == "" {
		afterPayText = "✅ سرویس شما با موفقیت ایجاد شد!"
	}
	afterPayText = strings.ReplaceAll(afterPayText, "{username}", username)
	afterPayText = strings.ReplaceAll(afterPayText, "{sublink}", panelUser.SubLink)
	afterPayText = strings.ReplaceAll(afterPayText, "{product}", product.NameProduct)
	afterPayText = strings.ReplaceAll(afterPayText, "{volume}", product.VolumeConstraint)
	afterPayText = strings.ReplaceAll(afterPayText, "{time}", product.ServiceTime)
	afterPayText = strings.ReplaceAll(afterPayText, "{price}", formatNumber(price))

	h.botAPI.SendMessage(userID, afterPayText, nil)

	// Report
	reportText := fmt.Sprintf(
		"🛒 خرید جدید\n👤 کاربر: %s (@%s)\n📦 محصول: %s\n💰 مبلغ: %s تومان\n📍 لوکیشن: %s\n👤 یوزرنیم: %s",
		userID, user.Username, product.NameProduct, formatNumber(price), panelModel.NamePanel, username,
	)
	h.reportToChannel(reportText, "report")

	// Reset user step
	_ = h.repos.User.Update(userID, map[string]interface{}{
		"step": "none",
	})
}

func (h *PaymentCallbackHandler) extendServiceAfterPayment(userID, invoiceID string, price int) {
	user, _ := h.repos.User.FindByID(userID)
	if user == nil {
		return
	}

	invoice, _ := h.repos.Invoice.FindByID(invoiceID)
	if invoice == nil {
		return
	}

	productCode := user.ProcessingValueTwo
	product, _ := h.repos.Product.FindByCode(productCode)
	if product == nil {
		return
	}

	panelModel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
	if panelModel == nil {
		return
	}

	panelClient, err := panel.PanelFactory(panelModel)
	if err != nil {
		return
	}

	bgCtx := createBgContext()
	if err := panelClient.Authenticate(bgCtx); err != nil {
		return
	}

	panelUser, err := panelClient.GetUser(bgCtx, invoice.Username)
	if err != nil {
		return
	}

	volumeGB := parseIntSafe(product.VolumeConstraint)
	timeDays := parseIntSafe(product.ServiceTime)
	extendMethod := panelModel.MethodExtend

	var newExpire int64
	var newDataLimit int64

	switch extendMethod {
	case "1": // Reset + add new
		newExpire = time.Now().Unix() + int64(timeDays)*86400
		newDataLimit = int64(volumeGB) * 1024 * 1024 * 1024
	case "2": // Add to current
		currentExpire := panelUser.ExpireTime
		if currentExpire < time.Now().Unix() {
			currentExpire = time.Now().Unix()
		}
		newExpire = currentExpire + int64(timeDays)*86400
		newDataLimit = panelUser.DataLimit + int64(volumeGB)*1024*1024*1024
	case "3": // Reset time, add volume
		newExpire = time.Now().Unix() + int64(timeDays)*86400
		newDataLimit = panelUser.DataLimit + int64(volumeGB)*1024*1024*1024
	default: // "4" or default: add time, reset volume
		currentExpire := panelUser.ExpireTime
		if currentExpire < time.Now().Unix() {
			currentExpire = time.Now().Unix()
		}
		newExpire = currentExpire + int64(timeDays)*86400
		newDataLimit = int64(volumeGB) * 1024 * 1024 * 1024
	}

	modReq := panel.ModifyUserRequest{
		Status:     "active",
		DataLimit:  newDataLimit,
		ExpireTime: newExpire,
	}

	_, err = panelClient.ModifyUser(bgCtx, invoice.Username, modReq)
	if err != nil {
		h.logger.Error("Failed to extend user", zap.Error(err))
		return
	}

	// Reset traffic if method 1
	if extendMethod == "1" {
		_ = panelClient.ResetTraffic(bgCtx, invoice.Username)
	}

	// Update invoice
	_ = h.repos.Invoice.Update(invoice.IDInvoice, map[string]interface{}{
		"Status":        "active",
		"name_product":  product.NameProduct,
		"price_product": fmt.Sprintf("%d", price),
		"Volume":        product.VolumeConstraint,
		"Service_time":  product.ServiceTime,
		"notifctions":   "",
	})

	// Record extension
	svcOther := &models.ServiceOther{
		IDUser:   userID,
		Username: invoice.Username,
		Value:    fmt.Sprintf("%s|%s", product.VolumeConstraint, product.ServiceTime),
		Time:     fmt.Sprintf("%d", time.Now().Unix()),
		Price:    fmt.Sprintf("%d", price),
		Type:     "extend_user",
		Status:   "paid",
	}
	h.repos.Setting.DB().Create(svcOther)

	// Notify user
	h.botAPI.SendMessage(userID, fmt.Sprintf("✅ سرویس %s با موفقیت تمدید شد!", invoice.Username), nil)

	// Report
	reportText := fmt.Sprintf(
		"🔁 تمدید سرویس\n👤 کاربر: %s\n📦 محصول: %s\n💰 مبلغ: %s تومان\n👤 یوزرنیم: %s",
		userID, product.NameProduct, formatNumber(price), invoice.Username,
	)
	h.reportToChannel(reportText, "report")

	_ = h.repos.User.Update(userID, map[string]interface{}{
		"step": "none",
	})
}

func (h *PaymentCallbackHandler) applyCashback(paymentReport *models.PaymentReport, settingKey string) {
	cashbackStr, _ := h.repos.Setting.GetPaySetting(settingKey)
	cashbackPct := parseIntSafe(cashbackStr)
	if cashbackPct <= 0 {
		return
	}

	price := parseIntSafe(paymentReport.Price)
	cashbackAmount := (price * cashbackPct) / 100

	user, _ := h.repos.User.FindByID(paymentReport.IDUser)
	if user == nil {
		return
	}

	newBalance := user.Balance + cashbackAmount
	_ = h.repos.User.Update(paymentReport.IDUser, map[string]interface{}{
		"Balance": newBalance,
	})

	text := fmt.Sprintf("🎁 کاربر عزیز مبلغ %s تومان به عنوان هدیه به حساب شما واریز گردید.", formatNumber(cashbackAmount))
	h.botAPI.SendMessage(paymentReport.IDUser, text, nil)
}

func (h *PaymentCallbackHandler) reportToChannel(text, topicType string) {
	setting, _ := h.repos.Setting.GetSettings()
	if setting == nil || setting.ChannelReport == "" {
		return
	}

	topicID, _ := h.repos.Setting.GetTopicID(topicType)

	params := map[string]interface{}{
		"chat_id":    setting.ChannelReport,
		"text":       text,
		"parse_mode": "HTML",
	}
	if topicID != "" {
		params["message_thread_id"] = topicID
	}

	h.botAPI.Call("sendMessage", params)
}

func (h *PaymentCallbackHandler) renderPaymentResult(c echo.Context, title, message, orderID string, amount int) error {
	html := `<!DOCTYPE html>
<html dir="rtl">
<head>
    <meta charset="UTF-8">
    <title>فاکتور پرداخت</title>
    <style>
        body { font-family: Tahoma, sans-serif; background: #f2f2f2; margin: 0; padding: 20px; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
        .box { background: #fff; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); padding: 40px; text-align: center; max-width: 400px; width: 100%; }
        h1 { color: #333; margin-bottom: 20px; }
        p { color: #666; margin-bottom: 10px; }
    </style>
</head>
<body>
    <div class="box">
        <h1>{{.Title}}</h1>
        {{if .OrderID}}<p>شماره تراکنش: <span>{{.OrderID}}</span></p>{{end}}
        {{if .Amount}}<p>مبلغ: <span>{{.AmountStr}}</span> تومان</p>{{end}}
        <p>{{.Message}}</p>
    </div>
</body>
</html>`

	tmpl, err := template.New("payment").Parse(html)
	if err != nil {
		return c.String(http.StatusInternalServerError, "template error")
	}

	data := map[string]interface{}{
		"Title":     title,
		"Message":   message,
		"OrderID":   orderID,
		"Amount":    amount > 0,
		"AmountStr": formatNumber(amount),
	}

	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)
	return tmpl.Execute(c.Response().Writer, data)
}

// ── Utility functions ────────────────────────────────────────────────

func createBgContext() interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(interface{}) interface{}
} {
	return bgContext{}
}

type bgContext struct{}

func (bgContext) Deadline() (time.Time, bool)   { return time.Time{}, false }
func (bgContext) Done() <-chan struct{}         { return nil }
func (bgContext) Err() error                    { return nil }
func (bgContext) Value(interface{}) interface{} { return nil }

func parseIntSafe(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

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
