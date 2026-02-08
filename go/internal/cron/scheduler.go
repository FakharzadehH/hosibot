package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"hosibot/internal/config"
	"hosibot/internal/models"
	"hosibot/internal/panel"
	"hosibot/internal/pkg/httpclient"
	"hosibot/internal/pkg/telegram"
	"hosibot/internal/repository"
)

// Scheduler manages all cron jobs.
type Scheduler struct {
	cron             *cron.Cron
	cfg              *config.Config
	logger           *zap.Logger
	repos            *CronRepos
	botAPI           *telegram.BotAPI
	paymentProcessor PaymentProcessor
	panels           map[string]panel.PanelClient
}

// CronRepos bundles repositories needed by cron jobs.
type CronRepos struct {
	User    *repository.UserRepository
	Product *repository.ProductRepository
	Invoice *repository.InvoiceRepository
	Payment *repository.PaymentRepository
	Panel   *repository.PanelRepository
	Setting *repository.SettingRepository
	CronJob *repository.CronJobRepository
}

// PaymentProcessor is implemented by components that can fulfill a paid order.
type PaymentProcessor interface {
	ProcessPaidOrder(orderID string) error
}

// New creates a new cron scheduler.
func New(cfg *config.Config, repos *CronRepos, botAPI *telegram.BotAPI, paymentProcessor PaymentProcessor, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		cron:             cron.New(cron.WithSeconds()),
		cfg:              cfg,
		logger:           logger,
		repos:            repos,
		botAPI:           botAPI,
		paymentProcessor: paymentProcessor,
		panels:           make(map[string]panel.PanelClient),
	}
}

// Start registers and starts all cron jobs.
func (s *Scheduler) Start() {
	s.logger.Info("Starting cron scheduler...")

	// Check expired agents - every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		s.logger.Debug("Running: check expired agents")
		s.checkExpiredConfigs()
	})

	// Disable expired configs - every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		s.logger.Debug("Running: disable expired configs")
		s.disableExpiredConfigs()
	})

	// Activate configs - every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		s.logger.Debug("Running: activate configs")
		s.activateConfigs()
	})

	// Daily status report - at 23:45 (matching PHP)
	s.cron.AddFunc("0 45 23 * * *", func() {
		s.logger.Debug("Running: daily status report")
		s.dailyStatusReport()
	})

	// Notification service - every 10 minutes
	s.cron.AddFunc("0 */10 * * * *", func() {
		s.logger.Debug("Running: notifications service")
		s.notificationsService()
	})

	// Backup - daily at 3 AM
	s.cron.AddFunc("0 0 3 * * *", func() {
		s.logger.Debug("Running: backup")
		s.backup()
	})

	// Config test cleanup - every 10 minutes
	s.cron.AddFunc("0 */10 * * * *", func() {
		s.logger.Debug("Running: config test")
		s.configTest()
	})

	// Panel uptime check - every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		s.logger.Debug("Running: panel uptime check")
		s.panelUptimeCheck()
	})

	// Payment expire - every hour
	s.cron.AddFunc("0 0 * * * *", func() {
		s.logger.Debug("Running: payment expire")
		s.paymentExpire()
	})

	// Plisio payment polling - every 3 minutes
	s.cron.AddFunc("0 */3 * * * *", func() {
		s.logger.Debug("Running: plisio payment check")
		s.plisioPaymentCheck()
	})

	// Auto-confirm manual card payments - every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		s.logger.Debug("Running: auto confirm card payments")
		s.autoConfirmCardPayments()
	})

	// Lottery score reward - daily at 00:00
	s.cron.AddFunc("0 0 0 * * *", func() {
		s.logger.Debug("Running: lottery report")
		s.lotteryReport()
	})

	// Marzban node uptime check - every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		s.logger.Debug("Running: node uptime check")
		s.nodeUptimeCheck()
	})

	// On-hold service check - every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		s.logger.Debug("Running: on-hold check")
		s.onHoldCheck()
	})

	// Queue-backed broadcast/gift jobs - every minute
	s.cron.AddFunc("0 * * * * *", func() {
		s.logger.Debug("Running: queued sendmessage/gift jobs")
		s.processQueuedJobs()
	})

	s.cron.Start()
	s.logger.Info("Cron scheduler started")
}

// Stop gracefully stops the cron scheduler.
func (s *Scheduler) Stop() context.Context {
	return s.cron.Stop()
}

// ── Check expired agents (expireagent.php) ────────────────────────────

func (s *Scheduler) checkExpiredConfigs() {
	defer s.recoverFromPanic("checkExpiredConfigs")

	// Find users with expire set
	var users []models.User
	s.repos.Setting.DB().Where("expire IS NOT NULL AND expire != ''").Find(&users)

	now := time.Now().Unix()

	for _, user := range users {
		if !user.Expire.Valid || user.Expire.String == "" {
			continue
		}

		expireTime := parseInt64Safe(user.Expire.String)
		if expireTime <= 0 {
			continue
		}

		remaining := expireTime - now
		if remaining < 0 {
			// Agent expired — demote
			s.logger.Info("Agent expired", zap.String("user_id", user.ID))

			// Notify user
			expireText, _ := s.repos.Setting.GetText("expireagent")
			if expireText == "" {
				expireText = "⚠️ اشتراک نمایندگی شما به پایان رسیده است."
			}
			s.botAPI.SendMessage(user.ID, expireText, nil)

			// Demote agent
			_ = s.repos.User.Update(user.ID, map[string]interface{}{
				"agent":  "f",
				"expire": nil,
			})

			// Report
			s.reportToChannel(fmt.Sprintf("⏰ انقضای نمایندگی\n👤 کاربر: %s", user.ID))
		}
	}
}

// ── Disable expired configs (disableconfig.php) ───────────────────────

func (s *Scheduler) disableExpiredConfigs() {
	defer s.recoverFromPanic("disableExpiredConfigs")

	// Select 10 random users with checkstatus = '2'
	var userIDs []string
	s.repos.Setting.DB().Model(&models.User{}).
		Where("checkstatus = '2'").
		Order("RAND()").Limit(10).
		Pluck("id", &userIDs)

	for _, userID := range userIDs {
		// Get their active invoices
		var invoices []models.Invoice
		s.repos.Setting.DB().Where(
			"id_user = ? AND Status IN ?",
			userID,
			[]string{"active", "end_of_time", "end_of_volume", "sendedwarn", "send_on_hold"},
		).Order("RAND()").Limit(10).Find(&invoices)

		if len(invoices) == 0 {
			// No active invoices — reset checkstatus
			_ = s.repos.User.Update(userID, map[string]interface{}{
				"checkstatus": "0",
			})
			continue
		}

		for _, inv := range invoices {
			panelModel, err := s.repos.Panel.FindByName(inv.ServiceLocation)
			if err != nil {
				continue
			}

			panelClient, err := s.getPanelClient(panelModel)
			if err != nil {
				continue
			}

			ctx := context.Background()
			panelUser, err := panelClient.GetUser(ctx, inv.Username)
			if err != nil {
				continue
			}

			if panelUser.Status == "active" {
				_ = panelClient.DisableUser(ctx, inv.Username)
			}

			_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
				"Status": "disablebyadmin",
			})
		}
	}
}

// ── Activate configs (activeconfig.php) ───────────────────────────────

func (s *Scheduler) activateConfigs() {
	defer s.recoverFromPanic("activateConfigs")

	// Select 10 random users with checkstatus = '1'
	var userIDs []string
	s.repos.Setting.DB().Model(&models.User{}).
		Where("checkstatus = '1'").
		Order("RAND()").Limit(10).
		Pluck("id", &userIDs)

	for _, userID := range userIDs {
		var invoices []models.Invoice
		s.repos.Setting.DB().Where(
			"id_user = ? AND Status = ?",
			userID,
			"disablebyadmin",
		).Order("RAND()").Limit(10).Find(&invoices)

		if len(invoices) == 0 {
			_ = s.repos.User.Update(userID, map[string]interface{}{
				"checkstatus": "0",
			})
			continue
		}

		for _, inv := range invoices {
			panelModel, err := s.repos.Panel.FindByName(inv.ServiceLocation)
			if err != nil {
				continue
			}

			panelClient, err := s.getPanelClient(panelModel)
			if err != nil {
				continue
			}

			ctx := context.Background()
			panelUser, err := panelClient.GetUser(ctx, inv.Username)
			if err != nil {
				continue
			}

			if panelUser.Status == "disabled" {
				_ = panelClient.EnableUser(ctx, inv.Username)
			}

			_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
				"Status": "active",
			})
		}
	}
}

// ── Daily status report (statusday.php) ───────────────────────────────

func (s *Scheduler) dailyStatusReport() {
	defer s.recoverFromPanic("dailyStatusReport")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil || setting.ChannelReport == "" {
		return
	}

	// Calculate today's start (Unix timestamp)
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayStart := startOfDay.Unix()

	db := s.repos.Setting.DB()

	// Invoice stats
	var invoiceCount int64
	var totalRevenue, totalVolume int64
	db.Model(&models.Invoice{}).
		Where("time_sell >= ? AND Status = 'active' AND name_product != 'سرویس تست'", fmt.Sprintf("%d", todayStart)).
		Count(&invoiceCount)

	db.Model(&models.Invoice{}).
		Where("time_sell >= ? AND Status = 'active' AND name_product != 'سرویس تست'", fmt.Sprintf("%d", todayStart)).
		Select("COALESCE(SUM(CAST(price_product AS UNSIGNED)), 0)").
		Scan(&totalRevenue)

	db.Model(&models.Invoice{}).
		Where("time_sell >= ? AND Status = 'active' AND name_product != 'سرویس تست'", fmt.Sprintf("%d", todayStart)).
		Select("COALESCE(SUM(CAST(Volume AS UNSIGNED)), 0)").
		Scan(&totalVolume)

	// Test service count
	var testCount int64
	db.Model(&models.Invoice{}).
		Where("time_sell >= ? AND name_product = 'سرویس تست'", fmt.Sprintf("%d", todayStart)).
		Count(&testCount)

	// New users
	var newUserCount int64
	db.Model(&models.User{}).
		Where("register >= ?", fmt.Sprintf("%d", todayStart)).
		Count(&newUserCount)

	// Extensions
	var extendCount int64
	var extendRevenue int64
	db.Model(&models.ServiceOther{}).
		Where("time >= ? AND type = 'extend_user' AND status != 'unpaid'", fmt.Sprintf("%d", todayStart)).
		Count(&extendCount)
	db.Model(&models.ServiceOther{}).
		Where("time >= ? AND type = 'extend_user' AND status != 'unpaid'", fmt.Sprintf("%d", todayStart)).
		Select("COALESCE(SUM(CAST(price AS UNSIGNED)), 0)").
		Scan(&extendRevenue)

	// Build report
	report := fmt.Sprintf(
		"📊 <b>گزارش روزانه - %s</b>\n\n"+
			"🛒 تعداد فروش: %d\n"+
			"💰 درآمد: %s تومان\n"+
			"💾 حجم فروخته‌شده: %d GB\n\n"+
			"🧪 سرویس تست: %d\n"+
			"👤 کاربران جدید: %d\n\n"+
			"🔁 تمدید: %d (درآمد: %s تومان)\n",
		now.Format("2006/01/02"),
		invoiceCount,
		formatNumberCron(int(totalRevenue)),
		totalVolume,
		testCount,
		newUserCount,
		extendCount,
		formatNumberCron(int(extendRevenue)),
	)

	// Per-panel stats
	panels, _ := s.repos.Panel.FindActive()
	if len(panels) > 0 {
		report += "\n📍 <b>آمار هر لوکیشن:</b>\n"
		for _, p := range panels {
			var pCount int64
			var pRevenue int64
			db.Model(&models.Invoice{}).
				Where("time_sell >= ? AND Service_location = ? AND Status = 'active' AND name_product != 'سرویس تست'",
					fmt.Sprintf("%d", todayStart), p.NamePanel).
				Count(&pCount)
			db.Model(&models.Invoice{}).
				Where("time_sell >= ? AND Service_location = ? AND Status = 'active' AND name_product != 'سرویس تست'",
					fmt.Sprintf("%d", todayStart), p.NamePanel).
				Select("COALESCE(SUM(CAST(price_product AS UNSIGNED)), 0)").
				Scan(&pRevenue)

			if pCount > 0 {
				report += fmt.Sprintf("  • %s: %d فروش / %s تومان\n", p.NamePanel, pCount, formatNumberCron(int(pRevenue)))
			}
		}
	}

	s.reportToChannel(report)
}

// ── Notification service (NoticationsService.php) ─────────────────────

func (s *Scheduler) notificationsService() {
	defer s.recoverFromPanic("notificationsService")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil {
		return
	}

	// Check if cron is enabled
	if setting.CronStatus != "" {
		var cronStatus map[string]interface{}
		if json.Unmarshal([]byte(setting.CronStatus), &cronStatus) == nil {
			if v, ok := cronStatus["notifications"]; ok && v == "off" {
				return
			}
		}
	}

	volumeWarnGB := parseIntSafeCron(setting.VolumeWarn)
	dayWarn := parseIntSafeCron(setting.DayWarn)
	removeDays := parseIntSafeCron(setting.RemoveDayC)

	db := s.repos.Setting.DB()

	// Fetch active invoices not recently checked (1h cooldown)
	oneHourAgo := fmt.Sprintf("%d", time.Now().Unix()-3600)
	var invoices []models.Invoice
	db.Where(
		"Status IN ? AND name_product != 'سرویس تست' AND (time_cron <= ? OR time_cron IS NULL OR time_cron = '')",
		[]string{"active", "end_of_time", "end_of_volume", "sendedwarn", "send_on_hold"},
		oneHourAgo,
	).Order("time_cron").Limit(30).Find(&invoices)

	for _, inv := range invoices {
		// Update time_cron to prevent reprocessing
		_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
			"time_cron": fmt.Sprintf("%d", time.Now().Unix()),
		})

		panelModel, err := s.repos.Panel.FindByName(inv.ServiceLocation)
		if err != nil {
			continue
		}

		panelClient, err := s.getPanelClient(panelModel)
		if err != nil {
			continue
		}

		ctx := context.Background()
		panelUser, err := panelClient.GetUser(ctx, inv.Username)
		if err != nil {
			continue
		}

		// Parse notification flags
		var notifs map[string]bool
		if inv.Notifications != "" {
			_ = json.Unmarshal([]byte(inv.Notifications), &notifs)
		}
		if notifs == nil {
			notifs = map[string]bool{"volume": false, "time": false}
		}

		// Check user's cron notification preference
		user, err := s.repos.User.FindByID(inv.IDUser)
		if err != nil {
			continue
		}

		// Volume warning
		if volumeWarnGB > 0 && panelUser.DataLimit > 0 && !notifs["volume"] {
			remainingBytes := panelUser.DataLimit - panelUser.UsedTraffic
			remainingGB := float64(remainingBytes) / (1024 * 1024 * 1024)
			if remainingGB <= float64(volumeWarnGB) && remainingGB > 0 && panelUser.Status == "active" {
				// Send volume warning
				volumeWarnText, _ := s.repos.Setting.GetText("textvolumewarn")
				if volumeWarnText == "" {
					volumeWarnText = fmt.Sprintf("⚠️ حجم باقی‌مانده سرویس %s: %.2f GB", inv.Username, remainingGB)
				} else {
					volumeWarnText = strings.ReplaceAll(volumeWarnText, "{username}", inv.Username)
					volumeWarnText = strings.ReplaceAll(volumeWarnText, "{remaining}", fmt.Sprintf("%.2f", remainingGB))
				}

				if !user.StatusCron.Valid || user.StatusCron.String != "0" {
					s.botAPI.SendMessage(inv.IDUser, volumeWarnText, nil)
				}

				notifs["volume"] = true
				notifsJSON, _ := json.Marshal(notifs)
				_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
					"notifctions": string(notifsJSON),
				})
			}
		}

		// Time warning
		if dayWarn > 0 && panelUser.ExpireTime > 0 && !notifs["time"] {
			daysRemaining := (panelUser.ExpireTime - time.Now().Unix()) / 86400
			if daysRemaining <= int64(dayWarn) && daysRemaining > 0 {
				timeWarnText, _ := s.repos.Setting.GetText("textdaywarn")
				if timeWarnText == "" {
					timeWarnText = fmt.Sprintf("⚠️ %d روز تا انقضای سرویس %s", daysRemaining, inv.Username)
				} else {
					timeWarnText = strings.ReplaceAll(timeWarnText, "{username}", inv.Username)
					timeWarnText = strings.ReplaceAll(timeWarnText, "{days}", fmt.Sprintf("%d", daysRemaining))
				}

				if !user.StatusCron.Valid || user.StatusCron.String != "0" {
					s.botAPI.SendMessage(inv.IDUser, timeWarnText, nil)
				}

				notifs["time"] = true
				notifsJSON, _ := json.Marshal(notifs)
				_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
					"notifctions": string(notifsJSON),
				})
			}
		}

		// Check if service should be removed (expired past threshold)
		if removeDays > 0 && panelUser.ExpireTime > 0 {
			daysExpired := (time.Now().Unix() - panelUser.ExpireTime) / 86400
			if daysExpired >= int64(removeDays) && (panelUser.Status == "expired" || panelUser.Status == "limited") {
				_ = panelClient.DeleteUser(ctx, inv.Username)
				_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
					"Status": "removeTime",
				})

				removeText, _ := s.repos.Setting.GetText("textremoveservice")
				if removeText == "" {
					removeText = fmt.Sprintf("🗑 سرویس %s حذف شد (منقضی).", inv.Username)
				}
				if !user.StatusCron.Valid || user.StatusCron.String != "0" {
					s.botAPI.SendMessage(inv.IDUser, removeText, nil)
				}
			}
		}
	}
}

// ── Backup (backupbot.php) ────────────────────────────────────────────

func (s *Scheduler) backup() {
	defer s.recoverFromPanic("backup")

	// Database backup via mysqldump would require exec.Command
	// For now, report that backup ran
	s.logger.Info("Backup job triggered — database backup requires mysqldump binary")
	s.reportToChannel("📦 وظیفه بکاپ اجرا شد. (بکاپ دیتابیس نیاز به پیکربندی mysqldump دارد)")
}

// ── Config test cleanup (configtest.php) ──────────────────────────────

func (s *Scheduler) configTest() {
	defer s.recoverFromPanic("configTest")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil {
		return
	}

	// Check if test cron is enabled
	if setting.CronStatus != "" {
		var cronStatus map[string]interface{}
		if json.Unmarshal([]byte(setting.CronStatus), &cronStatus) == nil {
			if v, ok := cronStatus["test"]; ok && v == "off" {
				return
			}
		}
	}

	db := s.repos.Setting.DB()

	// Select 15 random test service invoices
	var invoices []models.Invoice
	db.Where("Status != 'disabled' AND name_product = 'سرویس تست'").
		Order("RAND()").Limit(15).Find(&invoices)

	for _, inv := range invoices {
		panelModel, err := s.repos.Panel.FindByName(inv.ServiceLocation)
		if err != nil {
			continue
		}

		panelClient, err := s.getPanelClient(panelModel)
		if err != nil {
			continue
		}

		ctx := context.Background()
		panelUser, err := panelClient.GetUser(ctx, inv.Username)
		if err != nil {
			continue
		}

		// If not active or on_hold, remove
		if panelUser.Status != "active" && panelUser.Status != "on_hold" {
			_ = panelClient.DeleteUser(ctx, inv.Username)
			_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
				"Status": "disabled",
			})

			// Notify user
			user, err := s.repos.User.FindByID(inv.IDUser)
			if err != nil {
				continue
			}

			testEndText, _ := s.repos.Setting.GetText("crontest")
			if testEndText == "" {
				testEndText = "⚠️ سرویس تست شما به پایان رسید."
			}

			if !user.StatusCron.Valid || user.StatusCron.String != "0" {
				buyKB := map[string]interface{}{
					"inline_keyboard": [][]map[string]interface{}{
						{{"text": "🛒 خرید سرویس", "callback_data": "buy_service"}},
					},
				}
				s.botAPI.SendMessage(inv.IDUser, testEndText, buyKB)
			}
		}
	}
}

// ── Panel uptime check (uptime_panel.php) ─────────────────────────────

func (s *Scheduler) panelUptimeCheck() {
	defer s.recoverFromPanic("panelUptimeCheck")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil {
		return
	}

	// Check if uptime cron is enabled
	if setting.CronStatus != "" {
		var cronStatus map[string]interface{}
		if json.Unmarshal([]byte(setting.CronStatus), &cronStatus) == nil {
			if v, ok := cronStatus["uptime_panel"]; ok && v == "off" {
				return
			}
		}
	}

	panels, _, err := s.repos.Panel.FindAll(100, 1, "")
	if err != nil {
		return
	}

	admins, _ := s.repos.Setting.GetAllAdmins()

	for _, p := range panels {
		if p.Status != "active" {
			continue
		}

		// Parse URL to get host:port
		parsedURL, err := url.Parse(p.URLPanel)
		if err != nil {
			continue
		}

		host := parsedURL.Hostname()
		port := parsedURL.Port()
		if port == "" {
			if parsedURL.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}

		// Try TCP connection
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
		if err != nil {
			// Panel is down — alert admins
			alertText := fmt.Sprintf("🔴 <b>پنل %s آفلاین است!</b>\n\n🌐 آدرس: %s\n❌ خطا: %s", p.NamePanel, p.URLPanel, err.Error())
			for _, admin := range admins {
				s.botAPI.SendMessage(admin.IDAdmin, alertText, nil)
			}
		} else {
			conn.Close()
		}
	}
}

// ── Payment expire (payment_expire.php) ───────────────────────────────

func (s *Scheduler) paymentExpire() {
	defer s.recoverFromPanic("paymentExpire")

	db := s.repos.Setting.DB()

	// Find unpaid payments older than 24 hours
	cutoff := fmt.Sprintf("%d", time.Now().Unix()-86400)

	var payments []models.PaymentReport
	db.Where("payment_Status = 'Unpaid' AND time < ?", cutoff).Find(&payments)

	for _, p := range payments {
		// Delete the payment instruction message from user chat
		if p.MessageID > 0 {
			s.botAPI.DeleteMessage(p.IDUser, p.MessageID)
		}

		// Mark as expired
		_ = s.repos.Payment.UpdateByOrderID(p.IDOrder, map[string]interface{}{
			"payment_Status": "expire",
		})
	}

	s.logger.Debug("Payment expire completed", zap.Int("processed", len(payments)))
}

// ── Plisio payment polling (plisio.php) ──────────────────────────────

func (s *Scheduler) plisioPaymentCheck() {
	defer s.recoverFromPanic("plisioPaymentCheck")

	apiKey := mustGetPaySetting(s.repos.Setting, "apinowpayment")
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(apiKey) == "0" {
		return
	}

	db := s.repos.Setting.DB()
	var payments []models.PaymentReport
	db.Where("payment_Status = 'Unpaid' AND Payment_Method = 'plisio'").Find(&payments)

	for _, payment := range payments {
		if payment.PaymentStatus == "paid" || strings.TrimSpace(payment.DecNotConfirmed) == "" {
			continue
		}

		reqURL := fmt.Sprintf(
			"https://api.plisio.net/api/v1/operations?api_key=%s&search=%s",
			url.QueryEscape(apiKey),
			url.QueryEscape(payment.IDOrder),
		)

		rawResp, err := httpclient.New().WithTimeout(25 * time.Second).Get(reqURL)
		if err != nil {
			s.logger.Debug("Plisio status request failed",
				zap.String("order_id", payment.IDOrder),
				zap.Error(err),
			)
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(rawResp, &payload); err != nil {
			s.logger.Debug("Plisio status parse failed",
				zap.String("order_id", payment.IDOrder),
				zap.Error(err),
			)
			continue
		}

		status := s.plisioExtractStatus(payload)
		switch status {
		case "", "cancelled":
			expireText := fmt.Sprintf(
				"❌ تراکنش زیر بدلیل عدم پرداخت منقضی شد، لطفا وجهی بابت این تراکنش پرداخت نکنید\n\n🛒 کد سفارش: %s\n💰 مبلغ: %s تومان",
				payment.IDOrder,
				payment.Price,
			)
			s.botAPI.SendMessage(payment.IDUser, expireText, nil)
			_ = s.repos.Payment.UpdateByOrderID(payment.IDOrder, map[string]interface{}{
				"payment_Status": "expire",
				"at_updated":     fmt.Sprintf("%d", time.Now().Unix()),
			})

		case "completed":
			_ = s.repos.Payment.UpdateByOrderID(payment.IDOrder, map[string]interface{}{
				"payment_Status": "paid",
				"at_updated":     fmt.Sprintf("%d", time.Now().Unix()),
			})

			if s.paymentProcessor != nil {
				if err := s.paymentProcessor.ProcessPaidOrder(payment.IDOrder); err != nil {
					s.logger.Error("Plisio fulfillment failed",
						zap.String("order_id", payment.IDOrder),
						zap.Error(err),
					)
					s.reportToTopic(
						fmt.Sprintf("❌ خطا در تایید پرداخت Plisio\n👤 %s\n🔖 %s\n🧾 %v", payment.IDUser, payment.IDOrder, err),
						"errorreport",
					)
					continue
				}
			}

			cashbackPercent := parseIntSafeCron(mustGetPaySetting(s.repos.Setting, "chashbackplisio"))
			if cashbackPercent > 0 {
				price := parseIntSafeCron(payment.Price)
				cashback := (price * cashbackPercent) / 100
				if cashback > 0 {
					_ = s.repos.User.UpdateBalance(payment.IDUser, cashback)
					s.botAPI.SendMessage(
						payment.IDUser,
						fmt.Sprintf("🎁 کاربر عزیز مبلغ %s تومان به عنوان هدیه به حساب شما واریز گردید.", formatNumberCron(cashback)),
						nil,
					)
				}
			}

			txURL, invoiceURL, invoiceTotal := s.plisioExtractReportFields(payload)
			reportText := fmt.Sprintf(
				"💵 پرداخت جدید\n- 🆔 آیدی عددی کاربر : %s\n- 💸 مبلغ تراکنش %s\n- 🔗 <a href=\"%s\">لینک پرداخت</a>\n- 🔗 <a href=\"%s\">لینک پرداخت Plisio</a>\n- 📥 مبلغ واریز شده : %s\n- 💳 روش پرداخت : plisio",
				payment.IDUser,
				payment.Price,
				defaultOrDash(txURL),
				defaultOrDash(invoiceURL),
				defaultOrDash(invoiceTotal),
			)
			s.reportToTopic(reportText, "paymentreport")
		}
	}
}

// ── On-hold check (on_hold.php) ───────────────────────────────────────

func (s *Scheduler) onHoldCheck() {
	defer s.recoverFromPanic("onHoldCheck")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil {
		return
	}

	onHoldDays := parseIntSafeCron(setting.OnHoldDay)
	if onHoldDays <= 0 {
		return
	}

	db := s.repos.Setting.DB()

	// Fetch random Marzban panels
	var panels []models.Panel
	db.Where("type = 'marzban' AND status = 'active'").Order("RAND()").Limit(25).Find(&panels)

	for _, p := range panels {
		panelClient, err := s.getPanelClient(&p)
		if err != nil {
			continue
		}

		ctx := context.Background()

		// Try to get on_hold users from panel
		// Note: This requires panel API support for listing users by status
		stats, err := panelClient.GetSystemStats(ctx)
		if err != nil {
			continue
		}

		// Check if there are on_hold users in the stats
		_ = stats

		// Find on_hold invoices in our DB for this panel
		var invoices []models.Invoice
		db.Where(
			"Service_location = ? AND Status IN ?",
			p.NamePanel,
			[]string{"active", "send_on_hold"},
		).Find(&invoices)

		for _, inv := range invoices {
			panelUser, err := panelClient.GetUser(ctx, inv.Username)
			if err != nil {
				continue
			}

			if panelUser.Status != "on_hold" {
				continue
			}

			if inv.Status == "send_on_hold" {
				continue // Already notified
			}

			// Check days since purchase
			timeSell := parseInt64Safe(inv.TimeSell)
			daysSincePurchase := (time.Now().Unix() - timeSell) / 86400

			if daysSincePurchase >= int64(onHoldDays) {
				// Check no location change exists
				var changeCount int64
				db.Model(&models.ServiceOther{}).
					Where("username = ? AND type = 'change_location'", inv.Username).
					Count(&changeCount)

				if changeCount > 0 {
					continue
				}

				// Send reminder
				onHoldText, _ := s.repos.Setting.GetText("textonhold")
				if onHoldText == "" {
					onHoldText = fmt.Sprintf("⏸ سرویس %s شما %d روز است که فعال نشده. لطفاً اتصال برقرار کنید.", inv.Username, daysSincePurchase)
				}

				s.botAPI.SendMessage(inv.IDUser, onHoldText, nil)

				_ = s.repos.Invoice.Update(inv.IDInvoice, map[string]interface{}{
					"Status": "send_on_hold",
				})
			}
		}
	}
}

// ── Auto confirm manual payments (croncard.php) ──────────────────────

func (s *Scheduler) autoConfirmCardPayments() {
	defer s.recoverFromPanic("autoConfirmCardPayments")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil {
		return
	}

	statusCardAutoConfirm, _ := s.repos.Setting.GetPaySetting("statuscardautoconfirm")
	if statusCardAutoConfirm == "onautoconfirm" {
		return
	}
	confirmMode, _ := s.repos.Setting.GetPaySetting("autoconfirmcart")
	if confirmMode == "offauto" {
		return
	}

	waitMinutes := parseIntSafeCron(setting.TimeAutoNotVerify)
	if waitMinutes < 0 {
		waitMinutes = 0
	}

	exceptionRaw, _ := s.repos.Setting.GetPaySetting("Exception_auto_cart")
	exceptionUsers := parseExceptionUserIDs(exceptionRaw)

	cashbackPercent := parseIntSafeCron(mustGetPaySetting(s.repos.Setting, "chashbackcart"))

	db := s.repos.Setting.DB()
	var payments []models.PaymentReport
	db.Where(
		"payment_Status = 'waiting' AND Payment_Method IN ? AND (bottype IS NULL OR bottype = '')",
		[]string{"cart to cart", "arze digital offline", "card"},
	).Find(&payments)

	now := time.Now()
	for _, payment := range payments {
		if payment.AtUpdated == "" {
			continue
		}

		atUpdated, ok := parseFlexibleTime(payment.AtUpdated)
		if !ok {
			continue
		}

		since := now.Sub(atUpdated)
		if since >= time.Hour {
			continue
		}
		if since <= time.Duration(waitMinutes)*time.Minute {
			continue
		}
		if exceptionUsers[payment.IDUser] {
			continue
		}

		// Match PHP flow: set payment paid before fulfillment.
		_ = s.repos.Payment.UpdateByOrderID(payment.IDOrder, map[string]interface{}{
			"payment_Status":    "paid",
			"dec_not_confirmed": "تایید توسط ربات بدون بررسی",
			"at_updated":        fmt.Sprintf("%d", now.Unix()),
		})

		if s.paymentProcessor != nil {
			if err := s.paymentProcessor.ProcessPaidOrder(payment.IDOrder); err != nil {
				s.logger.Error("Auto-confirm payment fulfill failed",
					zap.String("order_id", payment.IDOrder),
					zap.String("user_id", payment.IDUser),
					zap.Error(err),
				)
				s.reportToTopic(
					fmt.Sprintf("❌ خطا در تایید خودکار پرداخت\n👤 %s\n🔖 %s\n🧾 %v", payment.IDUser, payment.IDOrder, err),
					"errorreport",
				)
				continue
			}
		}

		if cashbackPercent > 0 {
			price := parseIntSafeCron(payment.Price)
			cashback := (price * cashbackPercent) / 100
			if cashback > 0 {
				_ = s.repos.User.UpdateBalance(payment.IDUser, cashback)
				s.botAPI.SendMessage(
					payment.IDUser,
					fmt.Sprintf("🎁 کاربر عزیز مبلغ %s تومان به عنوان هدیه به حساب شما واریز گردید.", formatNumberCron(cashback)),
					nil,
				)
			}
		}

		paymentReportText := fmt.Sprintf(
			"💵 پرداخت جدید\n\nآیدی عددی کاربر : %s\nمبلغ تراکنش %s\nروش پرداخت : تایید خودکار بدون بررسی\n%s",
			payment.IDUser,
			payment.Price,
			payment.PaymentMethod,
		)
		s.reportToTopic(paymentReportText, "paymentreport")
	}
}

// ── Lottery score reward (lottery.php) ───────────────────────────────

func (s *Scheduler) lotteryReport() {
	defer s.recoverFromPanic("lotteryReport")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil {
		return
	}
	if setting.ScoreStatus != "1" {
		return
	}

	var rawPrizes []interface{}
	if err := json.Unmarshal([]byte(setting.LotteryPrize), &rawPrizes); err != nil {
		return
	}

	var prizes []int
	for _, item := range rawPrizes {
		switch v := item.(type) {
		case float64:
			prizes = append(prizes, int(v))
		case string:
			prizes = append(prizes, parseIntSafeCron(v))
		}
	}
	if len(prizes) == 0 {
		return
	}

	limit := len(prizes)
	if limit > 3 {
		limit = 3
	}

	db := s.repos.Setting.DB()
	q := db.Model(&models.User{}).
		Where("LOWER(User_Status) = ? AND score != 0", "active")
	if setting.LotteryAgent != "1" {
		q = q.Where("agent = ?", "f")
	}

	var winners []models.User
	q.Order("score DESC").Limit(limit).Find(&winners)

	textLotteryGroup := "📌 ادمین عزیز کاربران زیر برنده قرعه کشی و حسابشان شارژ گردید.\n"
	for i, user := range winners {
		if i >= len(prizes) {
			break
		}
		prizeAmount := prizes[i]
		if prizeAmount <= 0 {
			continue
		}

		_ = s.repos.User.UpdateBalance(user.ID, prizeAmount)

		rank := i + 1
		s.botAPI.SendMessage(
			user.ID,
			fmt.Sprintf(
				"🎁 نتیجه قرعه کشی\n\n😎 کاربر عزیز تبریک! شما نفر %d برنده %s تومان موجودی شدید و حساب شما شارژ گردید.",
				rank,
				formatNumberCron(prizeAmount),
			),
			nil,
		)

		username := user.Username
		if username == "" {
			username = "-"
		}
		textLotteryGroup += fmt.Sprintf(
			"\nنام کاربری : @%s\nآیدی عددی : %s\nمبلغ : %s\nنفر : %d\n--------------",
			username,
			user.ID,
			formatNumberCron(prizeAmount),
			rank,
		)
	}

	s.reportToTopic(textLotteryGroup, "otherreport")
	db.Model(&models.User{}).Where("score != 0").Update("score", 0)
}

// ── Marzban node uptime check (uptime_node.php) ──────────────────────

func (s *Scheduler) nodeUptimeCheck() {
	defer s.recoverFromPanic("nodeUptimeCheck")

	setting, err := s.repos.Setting.GetSettings()
	if err != nil {
		return
	}
	if !cronFeatureEnabled(setting.CronStatus, "uptime_node") {
		return
	}

	panels, err := s.repos.Panel.FindByType("marzban")
	if err != nil {
		return
	}

	for _, p := range panels {
		if strings.ToLower(p.Status) != "active" {
			continue
		}

		panelClient, err := s.getPanelClient(&p)
		if err != nil {
			continue
		}

		marzClient, ok := panelClient.(*panel.MarzbanClient)
		if !ok {
			continue
		}

		nodes, err := marzClient.GetNodes(context.Background())
		if err != nil {
			continue
		}

		for _, node := range nodes {
			status := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", node["status"])))
			if status == "connected" || status == "disabled" {
				continue
			}

			name := strings.TrimSpace(fmt.Sprintf("%v", node["name"]))
			if name == "" || name == "<nil>" {
				name = "-"
			}

			message := strings.TrimSpace(fmt.Sprintf("%v", node["message"]))
			if message == "" || message == "<nil>" {
				message = "-"
			}

			textNode := fmt.Sprintf(
				"🚨 ادمین عزیز نود با اسم %s متصل نیست.\nوضعیت نود : %s\n✍️ دلیل خطا : <code>%s</code>",
				name, status, message,
			)
			s.reportToTopic(textNode, "errorreport")
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

func (s *Scheduler) getPanelClient(panelModel *models.Panel) (panel.PanelClient, error) {
	key := panelModel.CodePanel
	if client, ok := s.panels[key]; ok {
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

	s.panels[key] = client
	return client, nil
}

func (s *Scheduler) reportToChannel(text string) {
	s.reportToTopic(text, "report")
}

func (s *Scheduler) reportToTopic(text, topicType string) {
	setting, err := s.repos.Setting.GetSettings()
	if err != nil || setting.ChannelReport == "" {
		return
	}

	topicID, _ := s.repos.Setting.GetTopicID(topicType)

	params := map[string]interface{}{
		"chat_id":    setting.ChannelReport,
		"text":       text,
		"parse_mode": "HTML",
	}
	if topicID != "" {
		params["message_thread_id"] = topicID
	}

	s.botAPI.Call("sendMessage", params)
}

func (s *Scheduler) plisioExtractStatus(payload map[string]interface{}) string {
	dataMap, _ := payload["data"].(map[string]interface{})
	ops, _ := dataMap["operations"].([]interface{})
	if len(ops) == 0 {
		return ""
	}

	firstOp, _ := ops[0].(map[string]interface{})
	status := strings.ToLower(strings.TrimSpace(parseStringSafe(firstOp["status"])))
	return status
}

func (s *Scheduler) plisioExtractReportFields(payload map[string]interface{}) (txURL, invoiceURL, invoiceTotal string) {
	dataMap, _ := payload["data"].(map[string]interface{})
	ops, _ := dataMap["operations"].([]interface{})
	if len(ops) > 0 {
		if firstOp, ok := ops[0].(map[string]interface{}); ok {
			txURL = parseStringSafe(firstOp["tx_url"])
			if txURL == "" {
				if txURLs, ok := firstOp["tx_urls"].([]interface{}); ok && len(txURLs) > 0 {
					txURL = parseStringSafe(txURLs[0])
				}
			}
			invoiceTotal = parseStringSafe(firstOp["invoice_total_sum"])
		}
	}

	if invoiceURL == "" {
		invoiceURL = parseStringSafe(payload["invoice_url"])
	}
	if invoiceURL == "" {
		invoiceURL = parseStringSafe(dataMap["invoice_url"])
	}
	if invoiceTotal == "" {
		invoiceTotal = parseStringSafe(payload["invoice_total_sum"])
	}
	if invoiceTotal == "" {
		invoiceTotal = parseStringSafe(dataMap["invoice_total_sum"])
	}

	return txURL, invoiceURL, invoiceTotal
}

func defaultOrDash(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}

func (s *Scheduler) recoverFromPanic(jobName string) {
	if r := recover(); r != nil {
		s.logger.Error("Cron job panicked", zap.String("job", jobName), zap.Any("error", r))
	}
}

func parseIntSafeCron(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

func parseInt64Safe(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func parseStringSafe(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		out := strings.TrimSpace(fmt.Sprintf("%v", v))
		if out == "<nil>" {
			return ""
		}
		return out
	}
}

func parseFlexibleTime(v string) (time.Time, bool) {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return time.Time{}, false
	}

	if unixRaw, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		// Milliseconds (13 digits) fallback.
		if unixRaw > 1_000_000_000_000 {
			return time.Unix(0, unixRaw*int64(time.Millisecond)), true
		}
		return time.Unix(unixRaw, 0), true
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02T15:04:05",
		"2006/01/02",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return ts, true
		}
	}

	return time.Time{}, false
}

func parseExceptionUserIDs(raw string) map[string]bool {
	users := make(map[string]bool)
	if strings.TrimSpace(raw) == "" {
		return users
	}

	var listAny []interface{}
	if err := json.Unmarshal([]byte(raw), &listAny); err == nil {
		for _, item := range listAny {
			id := strings.TrimSpace(fmt.Sprintf("%v", item))
			if id != "" && id != "<nil>" {
				users[id] = true
			}
		}
		return users
	}

	var listStrings []string
	if err := json.Unmarshal([]byte(raw), &listStrings); err == nil {
		for _, id := range listStrings {
			id = strings.TrimSpace(id)
			if id != "" {
				users[id] = true
			}
		}
		return users
	}

	// Allow comma-separated fallback.
	for _, item := range strings.Split(raw, ",") {
		id := strings.TrimSpace(item)
		if id != "" {
			users[id] = true
		}
	}
	return users
}

func mustGetPaySetting(settingRepo *repository.SettingRepository, name string) string {
	v, _ := settingRepo.GetPaySetting(name)
	return v
}

func cronFeatureEnabled(rawJSON, key string) bool {
	if strings.TrimSpace(rawJSON) == "" {
		return true
	}

	var status map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &status); err != nil {
		return true
	}

	value, ok := status[key]
	if !ok {
		return true
	}

	switch v := value.(type) {
	case bool:
		return v
	case string:
		v = strings.ToLower(strings.TrimSpace(v))
		return v != "" && v != "off" && v != "false" && v != "0"
	case float64:
		return v != 0
	default:
		return true
	}
}

func formatNumberCron(n int) string {
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

// Ensure rand is seeded (for RAND() equivalent)
func init() {
	rand.Seed(time.Now().UnixNano())
}
