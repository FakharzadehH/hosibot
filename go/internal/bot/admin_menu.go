package bot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"hosibot/internal/models"
	"hosibot/internal/panel"
	"hosibot/internal/pkg/utils"
)

func (b *Bot) isAdminWithRole(chatID string) (bool, string) {
	id := strings.TrimSpace(chatID)
	if id == "" {
		return false, ""
	}

	admin, err := b.repos.Setting.FindAdminByID(id)
	if err == nil && admin != nil {
		role := strings.TrimSpace(admin.Rule)
		if role == "" {
			role = "administrator"
		}
		return true, role
	}

	for _, part := range strings.Split(strings.TrimSpace(os.Getenv("BOT_ADMIN_ID")), ",") {
		if strings.TrimSpace(part) == id {
			return true, "administrator"
		}
	}

	return false, ""
}

func (b *Bot) adminMenuKeyboard(role string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	role = strings.ToLower(strings.TrimSpace(role))

	rows := []tele.Row{
		menu.Row(menu.Text("📊 آمار ربات"), menu.Text("💵 رسید های تایید نشده")),
	}

	switch role {
	case "support":
		rows = append(rows, menu.Row(menu.Text("👁‍🗨 جستجو کاربر"), menu.Text("👤 مدیریت کاربر")))
	case "seller":
		rows = append(rows, menu.Row(menu.Text("👤 مدیریت کاربر")))
	default: // administrator
		rows = append(rows, menu.Row(menu.Text("👁‍🗨 جستجو کاربر"), menu.Text("👤 مدیریت کاربر")))
		rows = append(rows, menu.Row(menu.Text("🖥 مدیریت پنل‌ها")))
		rows = append(rows, menu.Row(menu.Text("📡 مدیریت کانال"), menu.Text("📝 مدیریت متن‌ها")))
		rows = append(rows, menu.Row(menu.Text("📚 مدیریت آموزش"), menu.Text("⚙️ وضعیت قابلیت‌ها")))
		rows = append(rows, menu.Row(menu.Text("🏬 تنظیمات فروشگاه")))
		rows = append(rows, menu.Row(menu.Text("💎 مالی"), menu.Text("🤙 بخش پشتیبانی")))
		rows = append(rows, menu.Row(menu.Text("📱 مدیریت برنامه‌ها"), menu.Text("📬 گزارش ربات")))
		rows = append(rows, menu.Row(menu.Text("👥 مدیریت ادمین‌ها"), menu.Text("📣 پیام همگانی")))
		rows = append(rows, menu.Row(menu.Text("➕ افزودن ادمین")))
	}

	rows = append(rows, menu.Row(menu.Text("🔙 بازگشت به منوی کاربر")))
	menu.Reply(rows...)
	return menu
}

func (b *Bot) adminRoleKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("administrator"), menu.Text("Seller"), menu.Text("support")),
		menu.Row(menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) adminUserActionsKeyboard(target *models.User, blocked bool) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	targetUserID := target.ID
	agentType := strings.ToLower(strings.TrimSpace(target.Agent))
	if agentType == "" {
		agentType = "f"
	}
	cronEnabled := strings.TrimSpace(target.StatusCron.String) != "0"
	botSaz, _ := b.repos.Setting.FindBotSazByUserID(targetUserID)
	hasAgentBot := botSaz != nil

	blockBtn := menu.Data("🚫 مسدود", "admin_user_block_"+targetUserID)
	if blocked {
		blockBtn = menu.Data("✅ رفع مسدودی", "admin_user_unblock_"+targetUserID)
	}

	verifyBtn := menu.Data("📑 احراز کاربر", "admin_user_verify_"+targetUserID)
	if strings.TrimSpace(target.Verify) == "1" {
		verifyBtn = menu.Data("📑 لغو احراز", "admin_user_unverify_"+targetUserID)
	}

	cardBtn := menu.Data("💳 فعالسازی کارت", "admin_user_showcard_"+targetUserID)
	if strings.TrimSpace(target.CardPayment) == "1" {
		cardBtn = menu.Data("💳 غیرفعالسازی کارت", "admin_user_hidecard_"+targetUserID)
	}
	cronBtnText := "🕚 خاموش کردن کرون پیام"
	if !cronEnabled {
		cronBtnText = "🕚 روشن کردن کرون پیام"
	}

	rows := []tele.Row{
		menu.Row(blockBtn, menu.Data("🔄 بروزرسانی", "admin_user_refresh_"+targetUserID)),
		menu.Row(menu.Data("➕ افزایش موجودی", "admin_user_addbal_"+targetUserID), menu.Data("➖ کسر موجودی", "admin_user_subbal_"+targetUserID)),
		menu.Row(menu.Data("🎁 درصد تخفیف", "admin_user_discount_"+targetUserID), menu.Data("✍️ پیام به کاربر", "admin_user_message_"+targetUserID)),
		menu.Row(verifyBtn, cardBtn),
		menu.Row(menu.Data("0️⃣ صفر کردن موجودی", "admin_user_zero_"+targetUserID), menu.Data(cronBtnText, "admin_user_togglecron_"+targetUserID)),
	}

	switch agentType {
	case "n":
		rows = append(rows, menu.Row(
			menu.Data("🏷 نماینده: n", "admin_user_refresh_"+targetUserID),
			menu.Data("⭐ ارتقا به n2", "admin_user_agent_n2_"+targetUserID),
		))
	case "n2":
		rows = append(rows, menu.Row(
			menu.Data("🏷 نماینده: n2", "admin_user_refresh_"+targetUserID),
			menu.Data("⬇️ تنزل به n", "admin_user_agent_n_"+targetUserID),
		))
	default:
		rows = append(rows, menu.Row(
			menu.Data("➕ نماینده n", "admin_user_agent_n_"+targetUserID),
			menu.Data("➕ نماینده n2", "admin_user_agent_n2_"+targetUserID),
		))
	}

	if agentType == "n" || agentType == "n2" {
		rows = append(rows,
			menu.Row(menu.Data("❌ حذف نمایندگی", "admin_user_agent_f_"+targetUserID), menu.Data("💸 سقف خرید منفی", "admin_user_maxbuy_"+targetUserID)),
			menu.Row(menu.Data("⏱️ زمان انقضا نمایندگی", "admin_user_expire_"+targetUserID), menu.Data("➕ محدودیت تست", "admin_user_limittest_"+targetUserID)),
			menu.Row(menu.Data("🌍 محدودیت تغییر لوکیشن", "admin_user_changeloc_"+targetUserID)),
		)
	}

	rows = append(rows,
		menu.Row(menu.Data("👥 لیست زیرمجموعه‌ها", "admin_user_afflist_"+targetUserID), menu.Data("🧹 حذف زیرمجموعه‌ها", "admin_user_affclear_"+targetUserID)),
		menu.Row(menu.Data("🔁 انتقال حساب", "admin_user_transfer_"+targetUserID), menu.Data("🛒 افزودن سفارش", "admin_user_manualorder_"+targetUserID)),
	)

	if agentType == "n" || agentType == "n2" {
		if hasAgentBot {
			rows = append(rows,
				menu.Row(menu.Data("❌ حذف ربات فروش", "admin_user_agentbot_remove_"+targetUserID)),
				menu.Row(menu.Data("🔋 قیمت پایه حجم", "admin_user_agentbot_setvol_"+targetUserID), menu.Data("⏳ قیمت پایه زمان", "admin_user_agentbot_settime_"+targetUserID)),
				menu.Row(menu.Data("❌ مخفی کردن یک پنل", "admin_user_agentbot_hidepanel_"+targetUserID), menu.Data("🗑 پنل‌های مخفی‌شده", "admin_user_agentbot_showhidden_"+targetUserID)),
			)
		} else {
			rows = append(rows, menu.Row(menu.Data("🤖 فعالسازی ربات فروش", "admin_user_agentbot_create_"+targetUserID)))
		}
	}

	rows = append(rows, menu.Row(menu.Data("🔙 پنل مدیریت", "admin_panel")))

	menu.Inline(rows...)
	return menu
}

func (b *Bot) sendAdminMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok {
		return c.Send("⛔ دسترسی به پنل مدیریت ندارید.")
	}

	_ = b.repos.User.UpdateStep(chatID, "none")
	msg := b.adminPanelWelcomeText()
	if err := c.Send(msg, b.adminMenuKeyboard(role), tele.ModeHTML); err != nil {
		return err
	}

	user, err := b.repos.User.FindByID(chatID)
	if err != nil || user == nil {
		return nil
	}
	if user.HideMiniAppInstruction.Valid && strings.TrimSpace(user.HideMiniAppInstruction.String) == "1" {
		return nil
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("دیگر نمایش نده ⛓️‍💥", "hide_mini_app_instruction")))
	return c.Send(b.adminMiniAppInstructionText(), menu, tele.ModeHTML)
}

func (b *Bot) handleAdminMainMenuAction(c tele.Context, user *models.User, text string) (bool, error) {
	if user == nil {
		return false, nil
	}
	chatID := user.ID
	ok, role := b.isAdminWithRole(chatID)

	switch strings.TrimSpace(text) {
	case "🔧 پنل مدیریت", "/panel", "panel":
		if !ok {
			return true, c.Send("⛔ دسترسی به پنل مدیریت ندارید.")
		}
		return true, b.sendAdminMenu(c, chatID)

	case "📊 آمار ربات":
		if !ok {
			return true, c.Send("⛔ دسترسی ندارید.")
		}
		return true, b.sendAdminStats(c)

	case "💵 رسید های تایید نشده":
		if !ok || strings.EqualFold(role, "support") {
			return true, c.Send("⛔ دسترسی ندارید.")
		}
		return true, b.sendPendingPaymentsForAdmin(c)

	case "👁‍🗨 جستجو کاربر", "👤 مدیریت کاربر":
		if !ok {
			return true, c.Send("⛔ دسترسی ندارید.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_search_user")
		return true, c.Send("شناسه عددی یا یوزرنیم کاربر را ارسال کنید. مثال: 123456789 یا @username")

	case "➕ افزودن ادمین":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین اضافه کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_add_admin_id")
		return true, c.Send("آیدی عددی کاربر جدید را ارسال کنید.")

	case "🖥 مدیریت پنل‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
		}
		return true, b.sendAdminPanelManageMenu(c, chatID)

	case "📡 مدیریت کانال":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند کانال‌ها را مدیریت کند.")
		}
		return true, b.sendAdminChannelManageMenu(c, chatID)
	case "👥 مدیریت ادمین‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین‌ها را مدیریت کند.")
		}
		return true, b.sendAdminAdminManageMenu(c, chatID)
	case "➕ افزودن کانال":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند کانال اضافه کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_channel_add_name")
		return true, c.Send("نام دکمه کانال را ارسال کنید.")
	case "📋 لیست کانال‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند کانال‌ها را مدیریت کند.")
		}
		return true, b.sendAdminChannelList(c)
	case "🗑 حذف کانال":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند کانال‌ها را مدیریت کند.")
		}
		return true, b.sendAdminChannelRemoveList(c, chatID)
	case "🗑 حذف همه کانال‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند کانال‌ها را مدیریت کند.")
		}
		if err := b.repos.Setting.DeleteAllChannels(); err != nil {
			return true, c.Send("❌ حذف همه کانال‌ها ناموفق بود.")
		}
		return true, c.Send("✅ همه کانال‌ها حذف شدند.", b.adminChannelManageKeyboard())

	case "📝 مدیریت متن‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند متن‌ها را مدیریت کند.")
		}
		return true, b.sendAdminTextManageMenu(c, chatID)
	case "📱 مدیریت برنامه‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه‌ها را مدیریت کند.")
		}
		return true, b.sendAdminAppManageMenu(c, chatID)
	case "📬 گزارش ربات":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات گزارش را مدیریت کند.")
		}
		setting, _ := b.repos.Setting.GetSettings()
		current := ""
		if setting != nil {
			current = setting.ChannelReport
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_set_report_channel")
		return true, c.Send(fmt.Sprintf("آیدی/یوزرنیم کانال گزارش را ارسال کنید.\nمقدار فعلی: <code>%s</code>", emptyDash(current)), tele.ModeHTML)
	case "💎 مالی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.sendAdminFinanceMenu(c, chatID)
	case "🤙 بخش پشتیبانی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
		}
		return true, b.sendAdminSupportManageMenu(c, chatID)
	case "🏬 تنظیمات فروشگاه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		return true, b.sendAdminShopManageMenu(c, chatID)
	case "📋 لیست متن‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند متن‌ها را مدیریت کند.")
		}
		return true, b.sendAdminTextList(c, 1)
	case "🆔 ویرایش با کلید":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند متن‌ها را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "none")
		return true, c.Send("از بخش «📋 لیست متن‌ها» کلید را انتخاب کنید.", b.adminTextManageKeyboard())

	case "📚 مدیریت آموزش":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش‌ها را مدیریت کند.")
		}
		return true, b.sendAdminHelpManageMenu(c, chatID)
	case "➕ افزودن آموزش":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش اضافه کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_help_add_name")
		return true, c.Send("عنوان آموزش را ارسال کنید.")
	case "📋 لیست آموزش":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش‌ها را مدیریت کند.")
		}
		return true, b.sendAdminHelpList(c)
	case "🗑 حذف آموزش":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش‌ها را مدیریت کند.")
		}
		return true, b.sendAdminHelpDeleteList(c)
	case "📋 لیست ادمین‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین‌ها را مدیریت کند.")
		}
		return true, b.sendAdminAdminsList(c, false)
	case "🗑 حذف ادمین":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین‌ها را مدیریت کند.")
		}
		return true, b.sendAdminAdminsList(c, true)
	case "➕ افزودن برنامه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه اضافه کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_app_add_name")
		return true, c.Send("نام برنامه را ارسال کنید.")
	case "🗑 حذف برنامه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه حذف کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_app_remove_name")
		return true, c.Send("نام برنامه‌ای که باید حذف شود را ارسال کنید.")
	case "✏️ ویرایش لینک برنامه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند لینک برنامه را ویرایش کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_app_edit_name")
		return true, c.Send("نام برنامه را ارسال کنید.")
	case "📋 لیست برنامه‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه‌ها را مدیریت کند.")
		}
		return true, b.sendAdminAppsList(c)
	case "💳 تنظیم شماره کارت":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_finance_card_add")
		return true, c.Send("شماره کارت و نام دارنده کارت را ارسال کنید.\nفرمت: <code>6037991234567890|Ali Ahmadi</code>", tele.ModeHTML)
	case "📋 لیست کارت‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.sendAdminFinanceCardList(c)
	case "❌ حذف شماره کارت":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_finance_card_remove")
		return true, c.Send("شماره کارتی که باید حذف شود را ارسال کنید.")
	case "♻️ تایید خودکار رسید":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "autoconfirmcart", "تایید خودکار رسید", "onauto/offauto")
	case "🎁 کش بک کارت":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "chashbackcart", "درصد کش‌بک کارت", "0-100")
	case "🔒 نمایش کارت بعد پرداخت اول":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		setting, _ := b.repos.Setting.GetSettings()
		next := "1"
		if setting != nil && strings.TrimSpace(setting.ShowCard) == "1" {
			next = "0"
		}
		if err := b.repos.Setting.UpdateSetting("showcard", next); err != nil {
			return true, c.Send("❌ بروزرسانی تنظیم ناموفق بود.")
		}
		stateText := "روشن"
		if next == "0" {
			stateText = "خاموش"
		}
		return true, c.Send("✅ وضعیت نمایش کارت به کارت: "+stateText, b.adminFinanceKeyboard())
	case "🧩 API NowPayments":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "apinowpayment", "API NowPayments", "api_key")
	case "🧩 API Ternado":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "apiternado", "API Ternado", "api_key")
	case "🧩 مرچنت زرین پال":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "merchant_zarinpal", "مرچنت زرین پال", "merchant_id")
	case "🧩 مرچنت آقای پرداخت":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "merchant_id_aqayepardakht", "مرچنت آقای پرداخت", "pin")
	case "🧩 مرچنت FloyPay":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "marchent_floypay", "مرچنت FloyPay", "api_key")
	case "🧩 مرچنت TronSeller":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.beginAdminPaySettingInput(c, user, "marchent_tronseller", "مرچنت TronSeller", "merchant/api key")
	case "📋 لیست درگاه‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
		}
		return true, b.sendAdminFinanceGatewayList(c)
	case "👤 تنظیم آیدی پشتیبانی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_support_set_id")
		return true, c.Send("آیدی عددی پشتیبانی را ارسال کنید. مثال: <code>123456789</code>", tele.ModeHTML)
	case "📝 متن دکمه ☎️ پشتیبانی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_support_set_text")
		return true, c.Send("متن جدید دکمه پشتیبانی را ارسال کنید.")
	case "➕ افزودن دپارتمان پشتیبانی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_support_dept_add_name")
		return true, c.Send("نام دپارتمان پشتیبانی را ارسال کنید.")
	case "📋 لیست دپارتمان‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
		}
		return true, b.sendAdminSupportDepartmentsList(c, false)
	case "🗑 حذف دپارتمان پشتیبانی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
		}
		return true, b.sendAdminSupportDepartmentsList(c, true)
	case "🛍 اضافه کردن محصول":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		panels, _, _ := b.repos.Panel.FindAll(1, 1, "")
		if len(panels) == 0 {
			return true, c.Send("❌ ابتدا یک پنل اضافه کنید.", b.adminPanelManageKeyboard())
		}
		b.clearAdminState(chatID)
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_product_add_name")
		return true, c.Send("نام محصول را ارسال کنید.")
	case "📋 لیست محصولات":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		return true, b.sendAdminShopProductList(c)
	case "❌ حذف محصول":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_product_delete_id")
		return true, c.Send("شناسه محصول (ID) برای حذف را ارسال کنید.")
	case "✏️ ویرایش محصول":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_product_edit_id")
		return true, c.Send("شناسه محصول (ID) برای ویرایش را ارسال کنید.")
	case "➕ دسته بندی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_category_add_name")
		return true, c.Send("نام دسته بندی جدید را ارسال کنید.")
	case "📋 لیست دسته‌بندی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		return true, b.sendAdminShopCategoryList(c)
	case "🗑 حذف دسته بندی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_category_delete_id")
		return true, c.Send("شناسه دسته بندی برای حذف را ارسال کنید.")
	case "🎁 ساخت کد هدیه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_gift_add_code")
		return true, c.Send("کد هدیه را ارسال کنید (فقط حروف/اعداد).")
	case "📋 لیست کدهای هدیه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		return true, b.sendAdminShopGiftCodeList(c)
	case "❌ حذف کد هدیه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_gift_delete_code")
		return true, c.Send("کد هدیه برای حذف را ارسال کنید.")
	case "🎁 ساخت کد تخفیف":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_discount_add_code")
		return true, c.Send("کد تخفیف را ارسال کنید (فقط حروف/اعداد).")
	case "📋 لیست کدهای تخفیف":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		return true, b.sendAdminShopDiscountCodeList(c)
	case "❌ حذف کد تخفیف":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "admin_shop_discount_delete_code")
		return true, c.Send("کد تخفیف برای حذف را ارسال کنید.")

	case "⚙️ وضعیت قابلیت‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند قابلیت‌ها را مدیریت کند.")
		}
		return true, b.sendAdminFeatureToggleMenu(c, chatID)
	case "📣 پیام همگانی":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
		}
		if b.repos.CronJob == nil {
			return true, c.Send("❌ ماژول صف پیام در دسترس نیست.")
		}
		return true, b.sendAdminBroadcastMenu(c, chatID)
	case "✉️ پیام به همه کاربران فعال":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
		}
		state := decodeAdminState(user.ProcessingValue)
		state["broadcast_target"] = "all_active"
		delete(state, "broadcast_days")
		b.saveAdminState(chatID, state)
		_ = b.repos.User.UpdateStep(chatID, "admin_broadcast_text")
		return true, c.Send("متن پیام را ارسال کنید.")
	case "🛍 پیام به کاربران دارای سرویس":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
		}
		state := decodeAdminState(user.ProcessingValue)
		state["broadcast_target"] = "with_service"
		delete(state, "broadcast_days")
		b.saveAdminState(chatID, state)
		_ = b.repos.User.UpdateStep(chatID, "admin_broadcast_text")
		return true, c.Send("متن پیام را ارسال کنید.")
	case "🆕 پیام به کاربران بدون سرویس":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
		}
		state := decodeAdminState(user.ProcessingValue)
		state["broadcast_target"] = "without_service"
		delete(state, "broadcast_days")
		b.saveAdminState(chatID, state)
		_ = b.repos.User.UpdateStep(chatID, "admin_broadcast_text")
		return true, c.Send("متن پیام را ارسال کنید.")
	case "📴 پیام به کاربران غیرفعال":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
		}
		state := decodeAdminState(user.ProcessingValue)
		state["broadcast_target"] = "inactive_days"
		delete(state, "broadcast_days")
		b.saveAdminState(chatID, state)
		_ = b.repos.User.UpdateStep(chatID, "admin_broadcast_inactive_days")
		return true, c.Send("تعداد روز عدم فعالیت را ارسال کنید. مثال: 7")
	case "📌 لغو پین برای همه":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند این عملیات را انجام دهد.")
		}
		jobID, count, err := b.enqueueBroadcastJob(chatID, "unpinmessage", "", "all_active", 0)
		if err != nil {
			return true, c.Send("❌ " + err.Error())
		}
		return true, c.Send(fmt.Sprintf("✅ عملیات لغو پین در صف قرار گرفت.\nشناسه عملیات: %d\nگیرندگان: %d", jobID, count), b.adminBroadcastKeyboard())

	case "➕ افزودن پنل":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پنل اضافه کند.")
		}
		_ = b.repos.User.UpdateStep(chatID, "none")
		b.clearAdminState(chatID)
		return true, c.Send("نوع پنل را انتخاب کنید:", b.adminPanelTypeKeyboard())

	case "📋 لیست پنل‌ها":
		if !ok || !strings.EqualFold(role, "administrator") {
			return true, c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
		}
		return true, b.sendAdminPanelList(c)

	case "🔙 بازگشت به پنل مدیریت":
		if !ok {
			return true, c.Send("⛔ دسترسی ندارید.")
		}
		return true, b.sendAdminMenu(c, chatID)

	case "🔙 بازگشت به منوی کاربر":
		_ = b.repos.User.UpdateStep(chatID, "none")
		return true, b.sendMainMenu(c, chatID)
	}

	return false, nil
}

func (b *Bot) handleAdminSearchUserInput(c tele.Context, adminUser *models.User, text string) error {
	chatID := adminUser.ID
	ok, _ := b.isAdminWithRole(chatID)
	if !ok {
		_ = b.repos.User.UpdateStep(chatID, "none")
		return c.Send("⛔ دسترسی ندارید.")
	}

	raw := strings.TrimSpace(text)
	if raw == "" {
		return c.Send("ورودی نامعتبر است. شناسه یا یوزرنیم کاربر را ارسال کنید.")
	}

	var target *models.User
	var err error
	if strings.HasPrefix(raw, "@") || strings.ContainsAny(raw, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_") {
		target, err = b.repos.User.FindByUsername(raw)
	} else {
		target, err = b.repos.User.FindByID(raw)
	}
	if err != nil || target == nil {
		return c.Send("کاربر پیدا نشد.")
	}

	_ = b.repos.User.UpdateStep(chatID, "none")
	return b.sendAdminUserCard(c, target.ID)
}

func (b *Bot) sendAdminUserCard(c tele.Context, targetUserID string) error {
	target, err := b.repos.User.FindByID(targetUserID)
	if err != nil || target == nil {
		return c.Send("کاربر پیدا نشد.")
	}

	blocked := strings.EqualFold(target.UserStatus, "block") || strings.EqualFold(target.UserStatus, "blocked")
	statusText := "✅ فعال"
	if blocked {
		statusText = "🚫 مسدود"
	}

	agentType := strings.TrimSpace(target.Agent)
	if agentType == "" {
		agentType = "f"
	}
	expireText := "نامشخص"
	if target.Expire.Valid {
		expireText = formatAgentExpire(target.Expire.String)
	}
	maxBuy := "0"
	if target.MaxBuyAgent.Valid && strings.TrimSpace(target.MaxBuyAgent.String) != "" {
		maxBuy = strings.TrimSpace(target.MaxBuyAgent.String)
	}
	changeLocLimit := "0"
	if target.LimitChangeLoc.Valid && strings.TrimSpace(target.LimitChangeLoc.String) != "" {
		changeLocLimit = strings.TrimSpace(target.LimitChangeLoc.String)
	}
	cronStatus := "✅ روشن"
	if strings.TrimSpace(target.StatusCron.String) == "0" {
		cronStatus = "❌ خاموش"
	}

	text := fmt.Sprintf(
		"👤 <b>مشخصات کاربر</b>\n\n"+
			"🆔 آیدی: <code>%s</code>\n"+
			"👤 یوزرنیم: @%s\n"+
			"📱 شماره: %s\n"+
			"💰 موجودی: %s تومان\n"+
			"🎁 درصد تخفیف: %s\n"+
			"🪪 احراز هویت: %s\n"+
			"💳 نمایش کارت: %s\n"+
			"👥 گروه کاربری: %s\n"+
			"💸 سقف خرید منفی: %s\n"+
			"⏱️ انقضا نمایندگی: %s\n"+
			"➕ محدودیت تست: %d\n"+
			"🌍 محدودیت تغییر لوکیشن: %s\n"+
			"🕚 کرون پیام: %s\n"+
			"📌 وضعیت: %s\n"+
			"🎯 مرحله: %s",
		target.ID,
		emptyDash(target.Username),
		emptyDash(target.Number),
		formatNumber(target.Balance),
		emptyDash(target.PriceDiscount.String),
		map[bool]string{true: "✅", false: "❌"}[strings.TrimSpace(target.Verify) == "1"],
		map[bool]string{true: "✅", false: "❌"}[strings.TrimSpace(target.CardPayment) == "1"],
		agentType,
		maxBuy,
		expireText,
		target.LimitUserTest,
		changeLocLimit,
		cronStatus,
		statusText,
		emptyDash(target.Step),
	)

	return c.Send(text, b.adminUserActionsKeyboard(target, blocked), tele.ModeHTML)
}

func (b *Bot) sendAdminStats(c tele.Context) error {
	db := b.repos.Setting.DB()

	var usersTotal int64
	var invoicesTotal int64
	var servicesActive int64
	var pendingPayments int64
	var paidTotal int64

	_ = db.Model(&models.User{}).Count(&usersTotal).Error
	_ = db.Model(&models.Invoice{}).Where("Status != ?", "Unpaid").Count(&invoicesTotal).Error
	_ = db.Model(&models.Invoice{}).Where("Status IN ?", []string{"active", "end_of_time", "end_of_volume", "sendedwarn", "send_on_hold"}).Count(&servicesActive).Error
	_ = db.Model(&models.PaymentReport{}).Where("payment_Status = ?", "waiting").Count(&pendingPayments).Error
	_ = db.Model(&models.PaymentReport{}).
		Where("payment_Status = ? AND Payment_Method NOT IN ?", "paid", []string{"add balance by admin", "low balance by admin"}).
		Select("COALESCE(SUM(CAST(price AS SIGNED)), 0)").Scan(&paidTotal).Error

	text := fmt.Sprintf(
		"📊 <b>آمار ربات</b>\n\n"+
			"👥 کاربران: <code>%d</code>\n"+
			"🧾 کل فروش‌ها: <code>%d</code>\n"+
			"✅ سرویس‌های فعال: <code>%d</code>\n"+
			"⏳ رسیدهای تاییدنشده: <code>%d</code>\n"+
			"💵 مجموع پرداخت موفق: <code>%s</code> تومان\n"+
			"🕒 زمان گزارش: <code>%s</code>",
		usersTotal,
		invoicesTotal,
		servicesActive,
		pendingPayments,
		formatNumber(int(paidTotal)),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return c.Send(text, tele.ModeHTML)
}

func (b *Bot) sendPendingPaymentsForAdmin(c tele.Context) error {
	db := b.repos.Setting.DB()

	var pending []models.PaymentReport
	if err := db.Where("payment_Status = ?", "waiting").Order("time DESC").Limit(20).Find(&pending).Error; err != nil {
		return c.Send("خطا در خواندن رسیدهای تاییدنشده.")
	}
	if len(pending) == 0 {
		return c.Send("✅ رسید تاییدنشده‌ای وجود ندارد.")
	}

	if err := c.Send(fmt.Sprintf("🧾 تعداد رسیدهای تاییدنشده: %d", len(pending))); err != nil {
		return err
	}

	for _, p := range pending {
		text := fmt.Sprintf(
			"💵 <b>رسید تاییدنشده</b>\n\n"+
				"🔖 سفارش: <code>%s</code>\n"+
				"👤 کاربر: <code>%s</code>\n"+
				"💳 روش: %s\n"+
				"💰 مبلغ: %s تومان",
			p.IDOrder,
			p.IDUser,
			b.paymentMethodLabel(p.PaymentMethod),
			p.Price,
		)
		_ = c.Send(text, b.keyboard.AdminPaymentConfirmKeyboard(p.IDOrder), tele.ModeHTML)
	}

	return nil
}

func (b *Bot) handleAdminUserCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, _ := b.isAdminWithRole(adminID)
	if !ok {
		return c.Send("⛔ دسترسی ندارید.")
	}

	switch {
	case strings.HasPrefix(data, "admin_user_block_"):
		targetID := strings.TrimPrefix(data, "admin_user_block_")
		if err := b.repos.User.Block(targetID, "blocked by admin", "block"); err != nil {
			return c.Send("❌ خطا در مسدودسازی کاربر.")
		}
		return b.sendAdminUserCard(c, targetID)

	case strings.HasPrefix(data, "admin_user_unblock_"):
		targetID := strings.TrimPrefix(data, "admin_user_unblock_")
		if err := b.repos.User.Block(targetID, "", "active"); err != nil {
			return c.Send("❌ خطا در رفع مسدودی کاربر.")
		}
		return b.sendAdminUserCard(c, targetID)

	case strings.HasPrefix(data, "admin_user_addbal_"):
		targetID := strings.TrimPrefix(data, "admin_user_addbal_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_balance_add_amount")
		return c.Send(fmt.Sprintf("مبلغ افزایش موجودی برای کاربر %s را ارسال کنید (عدد تومان).", targetID))

	case strings.HasPrefix(data, "admin_user_subbal_"):
		targetID := strings.TrimPrefix(data, "admin_user_subbal_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_balance_sub_amount")
		return c.Send(fmt.Sprintf("مبلغ کسر موجودی برای کاربر %s را ارسال کنید (عدد تومان).", targetID))

	case strings.HasPrefix(data, "admin_user_refresh_"):
		targetID := strings.TrimPrefix(data, "admin_user_refresh_")
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_verify_"):
		targetID := strings.TrimPrefix(data, "admin_user_verify_")
		if err := b.repos.User.SetVerify(targetID, "1"); err != nil {
			return c.Send("❌ خطا در بروزرسانی وضعیت احراز.")
		}
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_unverify_"):
		targetID := strings.TrimPrefix(data, "admin_user_unverify_")
		if err := b.repos.User.SetVerify(targetID, "0"); err != nil {
			return c.Send("❌ خطا در بروزرسانی وضعیت احراز.")
		}
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_showcard_"):
		targetID := strings.TrimPrefix(data, "admin_user_showcard_")
		if err := b.repos.User.UpdateField(targetID, "cardpayment", "1"); err != nil {
			return c.Send("❌ خطا در بروزرسانی وضعیت کارت.")
		}
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_hidecard_"):
		targetID := strings.TrimPrefix(data, "admin_user_hidecard_")
		if err := b.repos.User.UpdateField(targetID, "cardpayment", "0"); err != nil {
			return c.Send("❌ خطا در بروزرسانی وضعیت کارت.")
		}
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_zero_"):
		targetID := strings.TrimPrefix(data, "admin_user_zero_")
		if err := b.repos.User.SetBalance(targetID, 0); err != nil {
			return c.Send("❌ خطا در صفر کردن موجودی.")
		}
		_, _ = b.botAPI.SendMessage(targetID, "ℹ️ موجودی کیف پول شما توسط مدیریت صفر شد.", nil)
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_discount_"):
		targetID := strings.TrimPrefix(data, "admin_user_discount_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_set_discount")
		return c.Send("درصد تخفیف کاربر را ارسال کنید (0 تا 100).")
	case strings.HasPrefix(data, "admin_user_message_"):
		targetID := strings.TrimPrefix(data, "admin_user_message_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_send_message")
		return c.Send("متن پیام برای کاربر را ارسال کنید.")
	case strings.HasPrefix(data, "admin_user_agent_n2_"):
		targetID := strings.TrimPrefix(data, "admin_user_agent_n2_")
		if err := b.repos.User.Update(targetID, map[string]interface{}{
			"agent":  "n2",
			"expire": sql.NullString{Valid: false},
		}); err != nil {
			return c.Send("❌ خطا در تغییر نوع نمایندگی.")
		}
		_, _ = b.botAPI.SendMessage(targetID, "✅ وضعیت شما به نماینده پیشرفته (n2) تغییر کرد.", nil)
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_agent_n_"):
		targetID := strings.TrimPrefix(data, "admin_user_agent_n_")
		if err := b.repos.User.Update(targetID, map[string]interface{}{
			"agent":  "n",
			"expire": sql.NullString{Valid: false},
		}); err != nil {
			return c.Send("❌ خطا در تغییر نوع نمایندگی.")
		}
		_, _ = b.botAPI.SendMessage(targetID, "✅ وضعیت شما به نماینده عادی (n) تغییر کرد.", nil)
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_agent_f_"):
		targetID := strings.TrimPrefix(data, "admin_user_agent_f_")
		if err := b.repos.User.Update(targetID, map[string]interface{}{
			"agent":         "f",
			"pricediscount": sql.NullString{String: "0", Valid: true},
			"expire":        sql.NullString{Valid: false},
			"maxbuyagent":   sql.NullString{String: "0", Valid: true},
		}); err != nil {
			return c.Send("❌ خطا در حذف نمایندگی.")
		}
		_, _ = b.botAPI.SendMessage(targetID, "ℹ️ وضعیت نمایندگی شما غیرفعال شد.", nil)
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_maxbuy_"):
		targetID := strings.TrimPrefix(data, "admin_user_maxbuy_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_set_maxbuy")
		return c.Send("حداکثر خرید منفی نماینده را ارسال کنید (تومان). اگر نامحدود است 0 ارسال کنید.")
	case strings.HasPrefix(data, "admin_user_expire_"):
		targetID := strings.TrimPrefix(data, "admin_user_expire_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_set_expire_days")
		return c.Send("تعداد روز تا انقضای نمایندگی را ارسال کنید. برای حذف انقضا عدد 0 را ارسال کنید.")
	case strings.HasPrefix(data, "admin_user_limittest_"):
		targetID := strings.TrimPrefix(data, "admin_user_limittest_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_set_limittest")
		return c.Send("محدودیت اکانت تست کاربر را ارسال کنید (عدد >= 0).")
	case strings.HasPrefix(data, "admin_user_changeloc_"):
		targetID := strings.TrimPrefix(data, "admin_user_changeloc_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_set_changeloc_limit")
		return c.Send("محدودیت تغییر لوکیشن برای کاربر را ارسال کنید (عدد >= 0).")
	case strings.HasPrefix(data, "admin_user_transfer_"):
		targetID := strings.TrimPrefix(data, "admin_user_transfer_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_transfer_id")
		return c.Send("آیدی عددی جدید کاربر را ارسال کنید.")
	case strings.HasPrefix(data, "admin_user_togglecron_"):
		targetID := strings.TrimPrefix(data, "admin_user_togglecron_")
		target, err := b.repos.User.FindByID(targetID)
		if err != nil || target == nil {
			return c.Send("کاربر پیدا نشد.")
		}
		next := "0"
		if strings.TrimSpace(target.StatusCron.String) == "0" {
			next = "1"
		}
		if err := b.repos.User.UpdateField(targetID, "status_cron", next); err != nil {
			return c.Send("❌ خطا در بروزرسانی وضعیت کرون کاربر.")
		}
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_afflist_"):
		targetID := strings.TrimPrefix(data, "admin_user_afflist_")
		return b.sendAdminUserAffiliates(c, targetID)
	case strings.HasPrefix(data, "admin_user_affclear_"):
		targetID := strings.TrimPrefix(data, "admin_user_affclear_")
		db := b.repos.Setting.DB()
		if err := db.Model(&models.User{}).
			Where("affiliates = ?", targetID).
			Updates(map[string]interface{}{"affiliates": "0"}).Error; err != nil {
			return c.Send("❌ خطا در حذف زیرمجموعه‌های کاربر.")
		}
		_ = b.repos.User.UpdateField(targetID, "affiliatescount", "0")
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_agentbot_create_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		targetID := strings.TrimPrefix(data, "admin_user_agentbot_create_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_agentbot_token")
		return c.Send("توکن ربات فروش را ارسال کنید.")
	case strings.HasPrefix(data, "admin_user_agentbot_remove_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		targetID := strings.TrimPrefix(data, "admin_user_agentbot_remove_")
		if err := b.removeAgentBotForUser(targetID); err != nil {
			return c.Send("❌ " + err.Error())
		}
		_ = c.Send("✅ ربات فروش حذف شد.")
		return b.sendAdminUserCard(c, targetID)
	case strings.HasPrefix(data, "admin_user_agentbot_setvol_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		targetID := strings.TrimPrefix(data, "admin_user_agentbot_setvol_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_agentbot_price_volume")
		return c.Send("قیمت پایه حجم نماینده را ارسال کنید.")
	case strings.HasPrefix(data, "admin_user_agentbot_settime_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		targetID := strings.TrimPrefix(data, "admin_user_agentbot_settime_")
		_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
		_ = b.repos.User.UpdateStep(adminID, "admin_user_agentbot_price_time")
		return c.Send("قیمت پایه زمان نماینده را ارسال کنید.")
	case strings.HasPrefix(data, "admin_user_agentbot_hidepanel_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		targetID := strings.TrimPrefix(data, "admin_user_agentbot_hidepanel_")
		return b.sendAdminAgentBotHidePanelList(c, adminUser, targetID)
	case strings.HasPrefix(data, "admin_user_agentbot_showhidden_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		targetID := strings.TrimPrefix(data, "admin_user_agentbot_showhidden_")
		return b.sendAdminAgentBotHiddenPanelsList(c, adminUser, targetID)
	case strings.HasPrefix(data, "admin_user_agentbot_hidepick_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		token := strings.TrimPrefix(data, "admin_user_agentbot_hidepick_")
		return b.handleAdminAgentBotHidePick(c, adminUser, token)
	case strings.HasPrefix(data, "admin_user_agentbot_unhide_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
		}
		token := strings.TrimPrefix(data, "admin_user_agentbot_unhide_")
		return b.handleAdminAgentBotUnhidePick(c, adminUser, token)
	case strings.HasPrefix(data, "admin_user_manualorder_panel_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند سفارش دستی ثبت کند.")
		}
		token := strings.TrimPrefix(data, "admin_user_manualorder_panel_")
		return b.handleAdminManualOrderPanelPick(c, adminUser, token)
	case strings.HasPrefix(data, "admin_user_manualorder_product_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند سفارش دستی ثبت کند.")
		}
		token := strings.TrimPrefix(data, "admin_user_manualorder_product_")
		return b.handleAdminManualOrderProductPick(c, adminUser, token)
	case strings.HasPrefix(data, "admin_user_manualorder_"):
		if _, role := b.isAdminWithRole(adminID); !strings.EqualFold(role, "administrator") {
			return c.Send("⛔ فقط مدیر اصلی می‌تواند سفارش دستی ثبت کند.")
		}
		targetID := strings.TrimPrefix(data, "admin_user_manualorder_")
		return b.sendAdminManualOrderPanelPicker(c, adminUser, targetID)
	}

	return c.Send("عملیات نامعتبر است.")
}

func (b *Bot) handleAdminBalanceAmountInput(c tele.Context, adminUser *models.User, text string, subtract bool) error {
	adminID := adminUser.ID
	ok, _ := b.isAdminWithRole(adminID)
	if !ok {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ دسترسی ندارید.")
	}

	amount := parseIntSafe(strings.TrimSpace(text))
	if amount <= 0 {
		return c.Send("مقدار نامعتبر است. فقط عدد بزرگتر از صفر ارسال کنید.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از جستجو کاربر اقدام کنید.")
	}

	if subtract {
		amount = -amount
	}
	if err := b.repos.User.UpdateBalance(targetID, amount); err != nil {
		return c.Send("❌ خطا در بروزرسانی موجودی.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) handleAdminUserDiscountInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, _ := b.isAdminWithRole(adminID)
	if !ok {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ دسترسی ندارید.")
	}
	pct := parseIntSafe(strings.TrimSpace(text))
	if pct < 0 || pct > 100 {
		return c.Send("درصد نامعتبر است. عددی بین 0 تا 100 ارسال کنید.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از جستجو کاربر اقدام کنید.")
	}
	if err := b.repos.User.UpdateField(targetID, "pricediscount", strconv.Itoa(pct)); err != nil {
		return c.Send("❌ خطا در بروزرسانی درصد تخفیف.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	_, _ = b.botAPI.SendMessage(targetID, fmt.Sprintf("🎁 درصد تخفیف شما توسط مدیریت روی %d%% تنظیم شد.", pct), nil)
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) handleAdminUserMessageInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, _ := b.isAdminWithRole(adminID)
	if !ok {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ دسترسی ندارید.")
	}
	msg := strings.TrimSpace(text)
	if msg == "" {
		return c.Send("متن پیام نمی‌تواند خالی باشد.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از جستجو کاربر اقدام کنید.")
	}

	if _, err := b.botAPI.SendMessage(targetID, "📩 یک پیام از سمت مدیریت برای شما ارسال شد:\n\n"+msg, nil); err != nil {
		return c.Send("❌ ارسال پیام به کاربر ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	return c.Send("✅ پیام به کاربر ارسال شد.")
}

func (b *Bot) handleAdminUserMaxBuyInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند این بخش را مدیریت کند.")
	}

	maxBuy := parseIntSafe(strings.TrimSpace(text))
	if maxBuy < 0 {
		return c.Send("مقدار نامعتبر است. عددی بزرگتر یا مساوی صفر ارسال کنید.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از کارت کاربر شروع کنید.")
	}

	if err := b.repos.User.Update(targetID, map[string]interface{}{
		"maxbuyagent": sql.NullString{String: strconv.Itoa(maxBuy), Valid: true},
	}); err != nil {
		return c.Send("❌ خطا در ذخیره سقف خرید منفی.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) handleAdminUserExpireDaysInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند این بخش را مدیریت کند.")
	}

	days := parseIntSafe(strings.TrimSpace(text))
	if days < 0 {
		return c.Send("مقدار نامعتبر است. عددی بزرگتر یا مساوی صفر ارسال کنید.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از کارت کاربر شروع کنید.")
	}

	updates := map[string]interface{}{}
	if days == 0 {
		updates["expire"] = sql.NullString{Valid: false}
	} else {
		expireTS := time.Now().Unix() + int64(days)*86400
		updates["expire"] = sql.NullString{String: strconv.FormatInt(expireTS, 10), Valid: true}
	}

	if err := b.repos.User.Update(targetID, updates); err != nil {
		return c.Send("❌ خطا در ذخیره زمان انقضا.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) handleAdminUserLimitTestInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند این بخش را مدیریت کند.")
	}

	limit := parseIntSafe(strings.TrimSpace(text))
	if limit < 0 {
		return c.Send("مقدار نامعتبر است. عددی بزرگتر یا مساوی صفر ارسال کنید.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از کارت کاربر شروع کنید.")
	}

	if err := b.repos.User.Update(targetID, map[string]interface{}{"limit_usertest": limit}); err != nil {
		return c.Send("❌ خطا در ذخیره محدودیت تست.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) handleAdminUserChangeLocLimitInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند این بخش را مدیریت کند.")
	}

	limit := parseIntSafe(strings.TrimSpace(text))
	if limit < 0 {
		return c.Send("مقدار نامعتبر است. عددی بزرگتر یا مساوی صفر ارسال کنید.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از کارت کاربر شروع کنید.")
	}

	if err := b.repos.User.Update(targetID, map[string]interface{}{
		"limitchangeloc": sql.NullString{String: strconv.Itoa(limit), Valid: true},
	}); err != nil {
		return c.Send("❌ خطا در ذخیره محدودیت تغییر لوکیشن.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) handleAdminUserTransferInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند این بخش را مدیریت کند.")
	}

	newID := strings.TrimSpace(text)
	if newID == "" {
		return c.Send("آیدی جدید نامعتبر است.")
	}

	oldID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if oldID == "" || oldID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست. دوباره از کارت کاربر شروع کنید.")
	}
	if oldID == newID {
		return c.Send("آیدی جدید با آیدی فعلی یکسان است.")
	}

	exists, err := b.repos.User.Exists(newID)
	if err == nil && exists {
		return c.Send("این آیدی قبلاً در سیستم ثبت شده است.")
	}

	if err := b.repos.User.TransferAccount(oldID, newID); err != nil {
		return c.Send("❌ انتقال حساب کاربری ناموفق بود.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	_, _ = b.botAPI.SendMessage(newID, "✅ حساب شما به شناسه جدید منتقل شد.", nil)
	return c.Send(fmt.Sprintf("✅ انتقال حساب انجام شد: <code>%s</code> ➜ <code>%s</code>", oldID, newID), tele.ModeHTML)
}

func (b *Bot) sendAdminUserAffiliates(c tele.Context, targetID string) error {
	users, err := b.repos.User.FindAffiliates(targetID)
	if err != nil {
		return c.Send("❌ خطا در دریافت زیرمجموعه‌ها.")
	}
	if len(users) == 0 {
		return c.Send("این کاربر زیرمجموعه‌ای ندارد.")
	}

	maxRows := 50
	lines := make([]string, 0, len(users)+2)
	lines = append(lines, fmt.Sprintf("👥 <b>زیرمجموعه‌های کاربر %s</b>", targetID))
	if len(users) > maxRows {
		lines = append(lines, fmt.Sprintf("نمایش %d مورد از %d مورد:", maxRows, len(users)))
	}
	for i, u := range users {
		if i >= maxRows {
			break
		}
		lines = append(lines, fmt.Sprintf("%d) <code>%s</code>  @%s", i+1, u.ID, emptyDash(u.Username)))
	}

	return c.Send(strings.Join(lines, "\n"), tele.ModeHTML)
}

func (b *Bot) handleAdminUserAgentBotTokenInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست.")
	}

	token := strings.TrimSpace(text)
	if token == "" {
		return c.Send("توکن نمی‌تواند خالی باشد.")
	}

	if err := b.activateAgentBotForUser(targetID, token); err != nil {
		return c.Send("❌ " + err.Error())
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	_ = c.Send("✅ ربات فروش فعال شد.")
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) handleAdminUserAgentBotPriceVolumeInput(c tele.Context, adminUser *models.User, text string) error {
	return b.handleAdminUserAgentBotPriceInput(c, adminUser, text, "minpricevolume")
}

func (b *Bot) handleAdminUserAgentBotPriceTimeInput(c tele.Context, adminUser *models.User, text string) error {
	return b.handleAdminUserAgentBotPriceInput(c, adminUser, text, "minpricetime")
}

func (b *Bot) handleAdminUserAgentBotPriceInput(c tele.Context, adminUser *models.User, text, key string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند ربات فروش را مدیریت کند.")
	}

	amount := parseIntSafe(strings.TrimSpace(text))
	if amount <= 0 {
		return c.Send("مقدار نامعتبر است. عدد بزرگتر از صفر ارسال کنید.")
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کاربر هدف مشخص نیست.")
	}

	botSaz, err := b.repos.Setting.FindBotSazByUserID(targetID)
	if err != nil || botSaz == nil {
		return c.Send("این کاربر ربات فروش فعال ندارد.")
	}

	setting := decodeAnyJSONToMap(botSaz.Setting)
	setting[key] = amount
	if key == "minpricevolume" {
		setting["pricevolume"] = amount
	} else if key == "minpricetime" {
		setting["pricetime"] = amount
	}
	buf, _ := json.Marshal(setting)

	if err := b.repos.Setting.UpdateBotSazByUserID(targetID, map[string]interface{}{"setting": string(buf)}); err != nil {
		return c.Send("❌ ذخیره تنظیمات ربات فروش ناموفق بود.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})
	_ = c.Send("✅ تنظیمات قیمت ربات فروش بروزرسانی شد.")
	return b.sendAdminUserCard(c, targetID)
}

func (b *Bot) sendAdminAgentBotHidePanelList(c tele.Context, adminUser *models.User, targetID string) error {
	botSaz, err := b.repos.Setting.FindBotSazByUserID(targetID)
	if err != nil || botSaz == nil {
		return c.Send("این کاربر ربات فروش فعال ندارد.")
	}

	hidden := parseStringArrayJSON(botSaz.HidePanel)
	hiddenSet := make(map[string]bool, len(hidden))
	for _, name := range hidden {
		hiddenSet[strings.TrimSpace(name)] = true
	}

	panels, _, err := b.repos.Panel.FindAll(500, 1, "")
	if err != nil {
		return c.Send("❌ خطا در دریافت لیست پنل‌ها.")
	}
	if len(panels) == 0 {
		return c.Send("پنلی یافت نشد.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	clearStateByPrefix(state, "ab_hide_")
	clearStateByPrefix(state, "ab_unhide_")

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(panels)+1)
	for _, p := range panels {
		panelName := strings.TrimSpace(p.NamePanel)
		if panelName == "" || hiddenSet[panelName] {
			continue
		}
		token := strings.ToLower(utils.RandomCode(6))
		state["ab_hide_"+token] = targetID + "|" + panelName
		rows = append(rows, menu.Row(menu.Data("❌ "+panelName, "admin_user_agentbot_hidepick_"+token)))
	}

	if len(rows) == 0 {
		b.saveAdminState(adminUser.ID, state)
		return c.Send("پنل دیگری برای مخفی‌سازی وجود ندارد.")
	}

	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "admin_user_refresh_"+targetID)))
	menu.Inline(rows...)
	b.saveAdminState(adminUser.ID, state)
	return c.Send("یک پنل را برای مخفی‌سازی انتخاب کنید:", menu)
}

func (b *Bot) sendAdminAgentBotHiddenPanelsList(c tele.Context, adminUser *models.User, targetID string) error {
	botSaz, err := b.repos.Setting.FindBotSazByUserID(targetID)
	if err != nil || botSaz == nil {
		return c.Send("این کاربر ربات فروش فعال ندارد.")
	}

	hidden := parseStringArrayJSON(botSaz.HidePanel)
	if len(hidden) == 0 {
		return c.Send("پنل مخفی‌شده‌ای برای این ربات وجود ندارد.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	clearStateByPrefix(state, "ab_hide_")
	clearStateByPrefix(state, "ab_unhide_")

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(hidden)+1)
	for _, panelName := range hidden {
		panelName = strings.TrimSpace(panelName)
		if panelName == "" {
			continue
		}
		token := strings.ToLower(utils.RandomCode(6))
		state["ab_unhide_"+token] = targetID + "|" + panelName
		rows = append(rows, menu.Row(menu.Data("✅ "+panelName, "admin_user_agentbot_unhide_"+token)))
	}

	if len(rows) == 0 {
		b.saveAdminState(adminUser.ID, state)
		return c.Send("پنل مخفی‌شده‌ای برای این ربات وجود ندارد.")
	}

	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "admin_user_refresh_"+targetID)))
	menu.Inline(rows...)
	b.saveAdminState(adminUser.ID, state)
	return c.Send("برای نمایش مجدد، پنل را انتخاب کنید:", menu)
}

func (b *Bot) handleAdminAgentBotHidePick(c tele.Context, adminUser *models.User, token string) error {
	key := "ab_hide_" + strings.TrimSpace(token)
	state := decodeAdminState(adminUser.ProcessingValue)
	mapped := strings.TrimSpace(state[key])
	targetID, panelName, ok := parseTargetPanelMapping(mapped)
	if !ok {
		return c.Send("گزینه انتخابی نامعتبر است.")
	}

	botSaz, err := b.repos.Setting.FindBotSazByUserID(targetID)
	if err != nil || botSaz == nil {
		return c.Send("این کاربر ربات فروش فعال ندارد.")
	}

	hidden := parseStringArrayJSON(botSaz.HidePanel)
	if !containsString(hidden, panelName) {
		hidden = append(hidden, panelName)
	}
	buf, _ := json.Marshal(hidden)
	if err := b.repos.Setting.UpdateBotSazByUserID(targetID, map[string]interface{}{"hide_panel": string(buf)}); err != nil {
		return c.Send("❌ بروزرسانی پنل‌های مخفی‌شده ناموفق بود.")
	}

	delete(state, key)
	b.saveAdminState(adminUser.ID, state)
	return b.sendAdminAgentBotHidePanelList(c, adminUser, targetID)
}

func (b *Bot) handleAdminAgentBotUnhidePick(c tele.Context, adminUser *models.User, token string) error {
	key := "ab_unhide_" + strings.TrimSpace(token)
	state := decodeAdminState(adminUser.ProcessingValue)
	mapped := strings.TrimSpace(state[key])
	targetID, panelName, ok := parseTargetPanelMapping(mapped)
	if !ok {
		return c.Send("گزینه انتخابی نامعتبر است.")
	}

	botSaz, err := b.repos.Setting.FindBotSazByUserID(targetID)
	if err != nil || botSaz == nil {
		return c.Send("این کاربر ربات فروش فعال ندارد.")
	}

	hidden := parseStringArrayJSON(botSaz.HidePanel)
	hidden = removeString(hidden, panelName)
	buf, _ := json.Marshal(hidden)
	if err := b.repos.Setting.UpdateBotSazByUserID(targetID, map[string]interface{}{"hide_panel": string(buf)}); err != nil {
		return c.Send("❌ بروزرسانی پنل‌های مخفی‌شده ناموفق بود.")
	}

	delete(state, key)
	b.saveAdminState(adminUser.ID, state)
	return b.sendAdminAgentBotHiddenPanelsList(c, adminUser, targetID)
}

func (b *Bot) sendAdminManualOrderPanelPicker(c tele.Context, adminUser *models.User, targetID string) error {
	if _, err := b.repos.User.FindByID(targetID); err != nil {
		return c.Send("کاربر هدف یافت نشد.")
	}

	panels, _, err := b.repos.Panel.FindAll(500, 1, "")
	if err != nil {
		return c.Send("❌ خطا در دریافت پنل‌ها.")
	}
	if len(panels) == 0 {
		return c.Send("پنلی ثبت نشده است.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	clearStateByPrefix(state, "mo_panel_")
	clearStateByPrefix(state, "mo_prod_")

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(panels)+1)
	for _, p := range panels {
		code := strings.TrimSpace(p.CodePanel)
		name := strings.TrimSpace(p.NamePanel)
		if code == "" || name == "" {
			continue
		}
		token := strings.ToLower(utils.RandomCode(6))
		state["mo_panel_"+token] = targetID + "|" + code
		rows = append(rows, menu.Row(menu.Data(name, "admin_user_manualorder_panel_"+token)))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "admin_user_refresh_"+targetID)))
	menu.Inline(rows...)
	b.saveAdminState(adminUser.ID, state)
	return c.Send("لوکیشن/پنل سفارش دستی را انتخاب کنید:", menu)
}

func (b *Bot) handleAdminManualOrderPanelPick(c tele.Context, adminUser *models.User, token string) error {
	key := "mo_panel_" + strings.TrimSpace(token)
	state := decodeAdminState(adminUser.ProcessingValue)
	mapped := strings.TrimSpace(state[key])
	targetID, panelCode, ok := parseTargetPanelMapping(mapped)
	if !ok {
		return c.Send("گزینه انتخابی نامعتبر است.")
	}

	panelModel, err := b.repos.Panel.FindByCode(panelCode)
	if err != nil || panelModel == nil {
		return c.Send("پنل انتخابی یافت نشد.")
	}

	var products []models.Product
	db := b.repos.Setting.DB()
	if err := db.Where("(Location = ? OR Location = '/all')", panelModel.NamePanel).Order("id DESC").Find(&products).Error; err != nil {
		return c.Send("❌ خطا در دریافت محصولات پنل.")
	}
	if len(products) == 0 {
		return c.Send("محصولی برای این پنل یافت نشد.")
	}

	clearStateByPrefix(state, "mo_prod_")
	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(products)+1)
	for _, p := range products {
		pid := int(p.ID)
		if pid <= 0 {
			continue
		}
		tokenP := strings.ToLower(utils.RandomCode(6))
		state["mo_prod_"+tokenP] = targetID + "|" + panelCode + "|" + strconv.Itoa(pid)
		label := fmt.Sprintf("%s | %sت", emptyDash(p.NameProduct), formatNumber(parseIntSafe(p.PriceProduct)))
		rows = append(rows, menu.Row(menu.Data(label, "admin_user_manualorder_product_"+tokenP)))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 بازگشت", "admin_user_manualorder_"+targetID)))
	menu.Inline(rows...)
	b.saveAdminState(adminUser.ID, state)
	return c.Send("محصول سفارش دستی را انتخاب کنید:", menu)
}

func (b *Bot) handleAdminManualOrderProductPick(c tele.Context, adminUser *models.User, token string) error {
	key := "mo_prod_" + strings.TrimSpace(token)
	state := decodeAdminState(adminUser.ProcessingValue)
	mapped := strings.TrimSpace(state[key])
	parts := strings.SplitN(mapped, "|", 3)
	if len(parts) != 3 {
		return c.Send("گزینه انتخابی نامعتبر است.")
	}
	targetID := strings.TrimSpace(parts[0])
	panelCode := strings.TrimSpace(parts[1])
	productID := strings.TrimSpace(parts[2])
	if targetID == "" || panelCode == "" || productID == "" {
		return c.Send("گزینه انتخابی نامعتبر است.")
	}
	clearStateByPrefix(state, "mo_panel_")
	clearStateByPrefix(state, "mo_prod_")
	b.saveAdminState(adminUser.ID, state)

	_ = b.repos.User.Update(adminUser.ID, map[string]interface{}{
		"Processing_value_one":  targetID,
		"Processing_value_tow":  panelCode,
		"Processing_value_four": productID,
	})
	_ = b.repos.User.UpdateStep(adminUser.ID, "admin_user_manualorder_username")
	return c.Send("نام کاربری سرویس را ارسال کنید. مثال: <code>user_123</code>", tele.ModeHTML)
}

func (b *Bot) handleAdminUserManualOrderUsernameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند سفارش دستی ثبت کند.")
	}

	username := strings.TrimSpace(text)
	if !isValidPanelUsername(username) {
		return c.Send("نام کاربری نامعتبر است. نمونه معتبر: <code>user_123</code>", tele.ModeHTML)
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	panelCode := strings.TrimSpace(adminUser.ProcessingValueTwo)
	productID := parseIntSafe(strings.TrimSpace(adminUser.ProcessingValueFour))
	if targetID == "" || panelCode == "" || productID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("اطلاعات سفارش ناقص است. دوباره تلاش کنید.")
	}

	targetUser, err := b.repos.User.FindByID(targetID)
	if err != nil || targetUser == nil {
		return c.Send("کاربر هدف یافت نشد.")
	}
	panelModel, err := b.repos.Panel.FindByCode(panelCode)
	if err != nil || panelModel == nil {
		return c.Send("پنل انتخابی یافت نشد.")
	}
	product, err := b.repos.Product.FindByID(productID)
	if err != nil || product == nil {
		return c.Send("محصول انتخابی یافت نشد.")
	}

	subURL, err := b.createManualServiceForUser(adminID, targetUser, product, panelModel, username)
	if err != nil {
		return c.Send("❌ " + err.Error())
	}

	_ = b.repos.User.Update(adminID, map[string]interface{}{
		"Processing_value_one":  "",
		"Processing_value_tow":  "",
		"Processing_value_four": "",
	})
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send(
		fmt.Sprintf("✅ سفارش دستی برای کاربر <code>%s</code> ثبت شد.\n👤 نام کاربری: <code>%s</code>\n🔗 لینک ساب: <code>%s</code>", targetID, username, emptyDash(subURL)),
		tele.ModeHTML,
	)
}

func (b *Bot) createManualServiceForUser(adminID string, targetUser *models.User, product *models.Product, panelModel *models.Panel, username string) (string, error) {
	ctx := context.Background()
	panelClient, err := b.getPanelClient(panelModel)
	if err != nil {
		return "", fmt.Errorf("خطا در اتصال به پنل")
	}

	volumeGB := parseIntSafe(product.VolumeConstraint)
	dataLimit := int64(volumeGB) * 1024 * 1024 * 1024
	serviceDays := parseIntSafe(product.ServiceTime)

	createReq := panel.CreateUserRequest{
		Username:       username,
		DataLimit:      dataLimit,
		ExpireDays:     serviceDays,
		DataLimitReset: product.DataLimitReset,
		Note:           targetUser.ID,
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
		return "", fmt.Errorf("ساخت سرویس روی پنل ناموفق بود")
	}

	invoiceID := fmt.Sprintf("%d", time.Now().UnixNano()/1e6)
	userInfoJSON, _ := json.Marshal(panelUser.Links)
	uuidJSON, _ := json.Marshal(panelUser.Proxies)
	invoice := &models.Invoice{
		IDInvoice:       invoiceID,
		IDUser:          targetUser.ID,
		Username:        username,
		ServiceLocation: panelModel.NamePanel,
		TimeSell:        fmt.Sprintf("%d", time.Now().Unix()),
		NameProduct:     product.NameProduct,
		PriceProduct:    product.PriceProduct,
		Volume:          product.VolumeConstraint,
		ServiceTime:     product.ServiceTime,
		UUID:            string(uuidJSON),
		Note:            "manual by admin " + adminID,
		UserInfo:        string(userInfoJSON),
		BotType:         "",
		Referral:        targetUser.Affiliates,
		Notifications:   `{"volume":false,"time":false}`,
		Status:          "active",
	}
	if err := b.repos.Invoice.Create(invoice); err != nil {
		return "", fmt.Errorf("ثبت فاکتور سفارش دستی ناموفق بود")
	}

	_ = b.repos.Panel.IncrementCounter(panelModel.CodePanel)

	subURL := strings.TrimSpace(panelUser.SubLink)
	if subURL == "" && len(panelUser.Links) > 0 {
		subURL = strings.TrimSpace(panelUser.Links[0])
	}
	userMsg := fmt.Sprintf(
		"✅ یک سرویس توسط مدیریت برای شما اضافه شد.\n\n📦 محصول: %s\n📍 لوکیشن: %s\n👤 نام کاربری: %s\n🔗 لینک ساب:\n<code>%s</code>",
		product.NameProduct,
		panelModel.NamePanel,
		username,
		emptyDash(subURL),
	)
	_, _ = b.botAPI.SendMessage(targetUser.ID, userMsg, nil)

	return subURL, nil
}

func (b *Bot) activateAgentBotForUser(targetID, token string) error {
	targetID = strings.TrimSpace(targetID)
	token = strings.TrimSpace(token)
	if targetID == "" || token == "" {
		return fmt.Errorf("آیدی کاربر یا توکن نامعتبر است")
	}

	totalBots, _ := b.repos.Setting.CountBotSaz()
	if totalBots >= 15 {
		return fmt.Errorf("حداکثر 15 ربات فروش می‌توانید فعال کنید")
	}

	userBotCount, _ := b.repos.Setting.CountBotSazByUserID(targetID)
	if userBotCount > 0 {
		return fmt.Errorf("این کاربر قبلاً ربات فروش فعال کرده است")
	}

	tokenCount, _ := b.repos.Setting.CountBotSazByToken(token)
	if tokenCount > 0 {
		return fmt.Errorf("این توکن قبلاً ثبت شده است")
	}

	username, err := getTelegramBotUsernameByToken(token)
	if err != nil {
		return fmt.Errorf("توکن ربات معتبر نیست")
	}

	if err := createAgentBotFilesLocal(targetID, username, token); err != nil {
		b.logger.Warn("Failed to create agent bot files", zap.Error(err), zap.String("chat_id", targetID))
	}
	if err := b.setAgentBotWebhookLocal(targetID, username, token); err != nil {
		b.logger.Warn("Failed to set agent bot webhook", zap.Error(err), zap.String("chat_id", targetID))
	}

	settingJSON, _ := json.Marshal(map[string]interface{}{
		"minpricetime":     4000,
		"pricetime":        4000,
		"minpricevolume":   4000,
		"pricevolume":      4000,
		"support_username": "@support",
		"Channel_Report":   0,
		"cart_info":        "جهت پرداخت مبلغ را به شماره کارت زیر واریز نمایید",
		"show_product":     true,
	})
	adminIDsJSON, _ := json.Marshal([]string{targetID})

	bot := &models.BotSaz{
		IDUser:    targetID,
		BotToken:  token,
		AdminIDs:  string(adminIDsJSON),
		Username:  username,
		Time:      time.Now().Format("2006/01/02 15:04:05"),
		Setting:   string(settingJSON),
		HidePanel: "[]",
	}
	if err := b.repos.Setting.CreateBotSaz(bot); err != nil {
		return fmt.Errorf("ثبت ربات فروش ناموفق بود")
	}

	if err := b.repos.User.Update(targetID, map[string]interface{}{
		"token": sql.NullString{String: token, Valid: true},
	}); err != nil {
		return fmt.Errorf("ذخیره توکن کاربر ناموفق بود")
	}

	_, _ = b.botAPI.SendMessage(targetID, "✅ ربات فروش برای حساب شما فعال شد.", nil)
	return nil
}

func (b *Bot) removeAgentBotForUser(targetID string) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return fmt.Errorf("آیدی کاربر نامعتبر است")
	}

	botSaz, err := b.repos.Setting.FindBotSazByUserID(targetID)
	if err != nil || botSaz == nil {
		return fmt.Errorf("این کاربر ربات فروش فعالی ندارد")
	}

	if err := deleteAgentBotFilesLocal(targetID, botSaz.Username); err != nil {
		b.logger.Warn("Failed to remove agent bot files", zap.Error(err), zap.String("chat_id", targetID))
	}
	if strings.TrimSpace(botSaz.BotToken) != "" {
		if err := deleteTelegramWebhookByToken(botSaz.BotToken); err != nil {
			b.logger.Warn("Failed to remove agent bot webhook", zap.Error(err), zap.String("chat_id", targetID))
		}
	}

	if err := b.repos.Setting.DeleteBotSazByUserID(targetID); err != nil {
		return fmt.Errorf("حذف ربات فروش ناموفق بود")
	}
	if err := b.repos.User.Update(targetID, map[string]interface{}{
		"token": sql.NullString{Valid: false},
	}); err != nil {
		return fmt.Errorf("پاکسازی توکن کاربر ناموفق بود")
	}

	_, _ = b.botAPI.SendMessage(targetID, "ℹ️ ربات فروش شما توسط مدیریت حذف شد.", nil)
	return nil
}

func (b *Bot) handleAdminAddAdminIDInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین اضافه کند.")
	}

	targetID := strings.TrimSpace(text)
	if targetID == "" {
		return c.Send("آیدی نامعتبر است. آیدی عددی ارسال کنید.")
	}

	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": targetID})
	_ = b.repos.User.UpdateStep(adminID, "admin_add_admin_role")
	return c.Send("نقش ادمین جدید را انتخاب کنید:", b.adminRoleKeyboard())
}

func (b *Bot) handleAdminAddAdminRoleInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین اضافه کند.")
	}

	selectedRole := strings.TrimSpace(text)
	switch selectedRole {
	case "administrator", "Seller", "support":
	default:
		if selectedRole == "🔙 بازگشت به پنل مدیریت" {
			_ = b.repos.User.UpdateStep(adminID, "none")
			return b.sendAdminMenu(c, adminID)
		}
		return c.Send("نقش نامعتبر است. یکی از administrator / Seller / support را انتخاب کنید.", b.adminRoleKeyboard())
	}

	targetID := strings.TrimSpace(adminUser.ProcessingValueOne)
	if targetID == "" || targetID == "none" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("آیدی هدف نامشخص است. دوباره عملیات را شروع کنید.")
	}

	if err := b.repos.Setting.UpsertAdmin(targetID, selectedRole); err != nil {
		return c.Send("❌ خطا در ثبت ادمین جدید.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	_ = b.repos.User.Update(adminID, map[string]interface{}{"Processing_value_one": ""})

	_, _ = b.botAPI.SendMessage(targetID, "✅ شما به عنوان ادمین در ربات ثبت شدید.", nil)

	return c.Send(fmt.Sprintf("✅ ادمین %s با نقش %s ثبت شد.", targetID, selectedRole), b.adminMenuKeyboard("administrator"))
}

func (b *Bot) adminPanelManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("➕ افزودن پنل"), menu.Text("📋 لیست پنل‌ها")),
		menu.Row(menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) adminPanelTypeKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Marzban", "admin_add_panel_type_marzban"), menu.Data("PasarGuard", "admin_add_panel_type_pasarguard")),
		menu.Row(menu.Data("Hiddify", "admin_add_panel_type_hiddify"), menu.Data("Marzneshin", "admin_add_panel_type_marzneshin")),
		menu.Row(menu.Data("X-UI", "admin_add_panel_type_x-ui_single"), menu.Data("Alireza", "admin_add_panel_type_alireza_single")),
		menu.Row(menu.Data("S-UI", "admin_add_panel_type_s_ui"), menu.Data("WGDashboard", "admin_add_panel_type_wgdashboard")),
		menu.Row(menu.Data("IBSng", "admin_add_panel_type_ibsng"), menu.Data("MikroTik", "admin_add_panel_type_mikrotik")),
		menu.Row(menu.Data("🔙 بازگشت", "admin_panel_manage")),
	)
	return menu
}

func (b *Bot) sendAdminPanelManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	b.clearAdminState(chatID)
	return c.Send("🖥 مدیریت پنل‌ها", b.adminPanelManageKeyboard())
}

func (b *Bot) sendAdminPanelList(c tele.Context) error {
	panels, _, err := b.repos.Panel.FindAll(200, 1, "")
	if err != nil {
		return c.Send("❌ خطا در خواندن لیست پنل‌ها.")
	}
	if len(panels) == 0 {
		return c.Send("پنلی ثبت نشده است.", b.adminPanelManageKeyboard())
	}

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(panels)+1)
	for _, p := range panels {
		title := fmt.Sprintf("%s | %s | %s", panelStatusEmoji(p.Status), p.NamePanel, panelTypeLabel(p.Type))
		rows = append(rows, menu.Row(menu.Data(title, fmt.Sprintf("admin_panel_open_%d", p.ID))))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 مدیریت پنل‌ها", "admin_panel_manage")))
	menu.Inline(rows...)
	return c.Send("📋 لیست پنل‌ها:", menu)
}

func (b *Bot) sendAdminPanelDetail(c tele.Context, panelID int) error {
	p, err := b.repos.Panel.FindByID(panelID)
	if err != nil || p == nil {
		return c.Send("پنل یافت نشد.")
	}

	status := "🔴 غیرفعال"
	if strings.EqualFold(strings.TrimSpace(p.Status), "active") {
		status = "🟢 فعال"
	}

	text := fmt.Sprintf(
		"🖥 <b>جزئیات پنل</b>\n\n"+
			"🆔 ID: <code>%d</code>\n"+
			"🔑 Code: <code>%s</code>\n"+
			"🏷 نام: <b>%s</b>\n"+
			"🧩 نوع: <code>%s</code>\n"+
			"📡 URL: <code>%s</code>\n"+
			"👤 Username: <code>%s</code>\n"+
			"📦 Limit: <code>%s</code>\n"+
			"📌 Status: %s",
		p.ID,
		emptyDash(p.CodePanel),
		emptyDash(p.NamePanel),
		panelTypeLabel(p.Type),
		emptyDash(p.URLPanel),
		emptyDash(p.UsernamePanel),
		emptyDash(p.LimitPanel),
		status,
	)

	menu := &tele.ReplyMarkup{}
	toggleText := "🔴 غیرفعال کردن"
	if !strings.EqualFold(strings.TrimSpace(p.Status), "active") {
		toggleText = "🟢 فعال کردن"
	}
	menu.Inline(
		menu.Row(menu.Data(toggleText, fmt.Sprintf("admin_panel_toggle_%d", p.ID)), menu.Data("🗑 حذف", fmt.Sprintf("admin_panel_delete_%d", p.ID))),
		menu.Row(menu.Data("✍️ ویرایش نام", fmt.Sprintf("admin_panel_edit_name_%d", p.ID)), menu.Data("🔗 ویرایش آدرس", fmt.Sprintf("admin_panel_edit_url_%d", p.ID))),
		menu.Row(menu.Data("💎 تنظیم شناسه اینباند", fmt.Sprintf("admin_panel_set_inbound_%d", p.ID)), menu.Data("🔗 دامنه لینک ساب", fmt.Sprintf("admin_panel_set_subdomain_%d", p.ID))),
		menu.Row(menu.Data("💡 روش ساخت نام", fmt.Sprintf("admin_panel_set_method_%d", p.ID))),
		menu.Row(menu.Data("⏳ زمان سرویس تست", fmt.Sprintf("admin_panel_set_test_time_%d", p.ID)), menu.Data("💾 حجم اکانت تست", fmt.Sprintf("admin_panel_set_test_volume_%d", p.ID))),
		menu.Row(menu.Data("🔄 بروزرسانی", fmt.Sprintf("admin_panel_open_%d", p.ID)), menu.Data("📋 لیست پنل‌ها", "admin_panel_list")),
		menu.Row(menu.Data("🔙 مدیریت پنل‌ها", "admin_panel_manage")),
	)
	return c.Send(text, menu, tele.ModeHTML)
}

func (b *Bot) handleAdminPanelCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}

	switch {
	case data == "admin_panel_manage":
		return b.sendAdminPanelManageMenu(c, adminID)
	case data == "admin_panel_list":
		return b.sendAdminPanelList(c)
	case strings.HasPrefix(data, "admin_add_panel_type_"):
		panelType := normalizePanelType(strings.TrimPrefix(data, "admin_add_panel_type_"))
		if panelType == "" {
			return c.Send("نوع پنل نامعتبر است.", b.adminPanelTypeKeyboard())
		}
		b.clearAdminState(adminID)
		b.setAdminStateValue(adminID, "type", panelType)
		_ = b.repos.User.UpdateStep(adminID, "admin_add_panel_name")
		return c.Send(fmt.Sprintf("نوع پنل انتخاب شد: %s\nحالا نام پنل را ارسال کنید.", panelTypeLabel(panelType)))
	case strings.HasPrefix(data, "admin_panel_open_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_open_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		return b.sendAdminPanelDetail(c, panelID)
	case strings.HasPrefix(data, "admin_panel_toggle_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_toggle_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		p, err := b.repos.Panel.FindByID(panelID)
		if err != nil || p == nil {
			return c.Send("پنل یافت نشد.")
		}
		next := "active"
		if strings.EqualFold(strings.TrimSpace(p.Status), "active") {
			next = "deactive"
		}
		if err := b.repos.Panel.Update(panelID, map[string]interface{}{"status": next}); err != nil {
			return c.Send("❌ خطا در بروزرسانی وضعیت پنل.")
		}
		return b.sendAdminPanelDetail(c, panelID)
	case strings.HasPrefix(data, "admin_panel_delete_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_delete_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		if err := b.repos.Panel.Delete(panelID); err != nil {
			return c.Send("❌ خطا در حذف پنل.")
		}
		return c.Send("✅ پنل حذف شد.", b.adminPanelManageKeyboard())
	case strings.HasPrefix(data, "admin_panel_edit_name_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_edit_name_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		p, err := b.repos.Panel.FindByID(panelID)
		if err != nil || p == nil {
			return c.Send("پنل یافت نشد.")
		}
		state := decodeAdminState(adminUser.ProcessingValue)
		state["panel_edit_id"] = strconv.Itoa(panelID)
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_edit_name")
		return c.Send(fmt.Sprintf("نام جدید پنل «%s» را ارسال کنید.", emptyDash(p.NamePanel)))
	case strings.HasPrefix(data, "admin_panel_edit_url_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_edit_url_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		p, err := b.repos.Panel.FindByID(panelID)
		if err != nil || p == nil {
			return c.Send("پنل یافت نشد.")
		}
		state := decodeAdminState(adminUser.ProcessingValue)
		state["panel_edit_id"] = strconv.Itoa(panelID)
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_edit_url")
		return c.Send(fmt.Sprintf("آدرس جدید پنل «%s» را ارسال کنید.\nآدرس فعلی: <code>%s</code>", emptyDash(p.NamePanel), emptyDash(p.URLPanel)), tele.ModeHTML)
	case strings.HasPrefix(data, "admin_panel_set_inbound_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_set_inbound_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		state := decodeAdminState(adminUser.ProcessingValue)
		state["panel_edit_id"] = strconv.Itoa(panelID)
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_set_inbound")
		return c.Send("شناسه اینباند جدید را ارسال کنید. مثال: 1")
	case strings.HasPrefix(data, "admin_panel_set_subdomain_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_set_subdomain_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		state := decodeAdminState(adminUser.ProcessingValue)
		state["panel_edit_id"] = strconv.Itoa(panelID)
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_set_subdomain")
		return c.Send("دامنه/آدرس لینک ساب را ارسال کنید. مثال: https://sub.example.com")
	case strings.HasPrefix(data, "admin_panel_set_method_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_set_method_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		state := decodeAdminState(adminUser.ProcessingValue)
		state["panel_edit_id"] = strconv.Itoa(panelID)
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_set_method")
		return c.Send(
			"روش ساخت نام کاربری را ارسال کنید:\n" +
				"1) آیدی عددی + حروف و عدد رندوم\n" +
				"2) نام کاربری + عدد رندوم\n" +
				"3) نام کاربری + عدد به ترتیب\n" +
				"4) متن دلخواه + عدد رندوم\n" +
				"5) متن دلخواه + عدد ترتیبی\n" +
				"6) متن دلخواه نماینده + عدد ترتیبی",
		)
	case strings.HasPrefix(data, "admin_panel_set_test_time_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_set_test_time_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		state := decodeAdminState(adminUser.ProcessingValue)
		state["panel_edit_id"] = strconv.Itoa(panelID)
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_set_test_time")
		return c.Send("زمان سرویس تست (روز) را ارسال کنید.")
	case strings.HasPrefix(data, "admin_panel_set_test_volume_"):
		panelID, err := strconv.Atoi(strings.TrimPrefix(data, "admin_panel_set_test_volume_"))
		if err != nil || panelID <= 0 {
			return c.Send("شناسه پنل نامعتبر است.")
		}
		state := decodeAdminState(adminUser.ProcessingValue)
		state["panel_edit_id"] = strconv.Itoa(panelID)
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_set_test_volume")
		return c.Send("حجم سرویس تست (GB) را ارسال کنید.")
	}

	return c.Send("عملیات پنل نامعتبر است.")
}

func (b *Bot) handleAdminPanelEditNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}

	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام پنل نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	panelID := parseIntSafe(state["panel_edit_id"])
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	if err := b.repos.Panel.Update(panelID, map[string]interface{}{"name_panel": name}); err != nil {
		return c.Send("❌ بروزرسانی نام پنل ناموفق بود.")
	}
	delete(state, "panel_edit_id")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func (b *Bot) handleAdminPanelEditURLInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}

	raw := strings.TrimSpace(text)
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return c.Send("لینک معتبر نیست. مثال: https://panel.example.com")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	panelID := parseIntSafe(state["panel_edit_id"])
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	if err := b.repos.Panel.Update(panelID, map[string]interface{}{"url_panel": strings.TrimRight(raw, "/")}); err != nil {
		return c.Send("❌ بروزرسانی آدرس پنل ناموفق بود.")
	}
	delete(state, "panel_edit_id")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func (b *Bot) getPanelIDFromAdminState(adminUser *models.User) int {
	if adminUser == nil {
		return 0
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	return parseIntSafe(strings.TrimSpace(state["panel_edit_id"]))
}

func (b *Bot) handleAdminPanelSetInboundInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}
	panelID := b.getPanelIDFromAdminState(adminUser)
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	inboundID := parseIntSafe(strings.TrimSpace(text))
	if inboundID <= 0 {
		return c.Send("شناسه اینباند نامعتبر است.")
	}
	if err := b.repos.Panel.Update(panelID, map[string]interface{}{"inboundid": strconv.Itoa(inboundID)}); err != nil {
		return c.Send("❌ بروزرسانی شناسه اینباند ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func (b *Bot) handleAdminPanelSetSubdomainInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}
	panelID := b.getPanelIDFromAdminState(adminUser)
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	subdomain := strings.TrimSpace(text)
	if subdomain == "" {
		return c.Send("دامنه لینک ساب نمی‌تواند خالی باشد.")
	}
	if !strings.HasPrefix(strings.ToLower(subdomain), "http://") && !strings.HasPrefix(strings.ToLower(subdomain), "https://") {
		subdomain = "https://" + subdomain
	}
	if err := b.repos.Panel.Update(panelID, map[string]interface{}{"linksubx": strings.TrimRight(subdomain, "/")}); err != nil {
		return c.Send("❌ بروزرسانی دامنه لینک ساب ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func normalizePanelMethodInput(raw string) (method string, needsCustom bool, ok bool) {
	value := strings.TrimSpace(raw)
	switch value {
	case "1", "آیدی عددی + حروف و عدد رندوم", "id+random":
		return "آیدی عددی + حروف و عدد رندوم", false, true
	case "2", "نام کاربری + عدد رندوم", "username+random":
		return "نام کاربری + عدد رندوم", false, true
	case "3", "نام کاربری + عدد به ترتیب", "username+serial":
		return "نام کاربری + عدد به ترتیب", true, true
	case "4", "متن دلخواه + عدد رندوم", "custom+random":
		return "متن دلخواه + عدد رندوم", true, true
	case "5", "متن دلخواه + عدد ترتیبی", "custom+serial":
		return "متن دلخواه + عدد ترتیبی", true, true
	case "6", "متن دلخواه نماینده + عدد ترتیبی", "agentcustom+serial":
		return "متن دلخواه نماینده + عدد ترتیبی", true, true
	default:
		return "", false, false
	}
}

func (b *Bot) handleAdminPanelSetMethodInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}
	panelID := b.getPanelIDFromAdminState(adminUser)
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	method, needsCustom, valid := normalizePanelMethodInput(text)
	if !valid {
		return c.Send("روش نامعتبر است. یکی از گزینه‌های 1 تا 6 را ارسال کنید.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	state["panel_method_username"] = method
	b.saveAdminState(adminID, state)

	if needsCustom {
		_ = b.repos.User.UpdateStep(adminID, "admin_panel_set_namecustom")
		return c.Send("نام پیشفرض/متن دلخواه را ارسال کنید.")
	}

	if err := b.repos.Panel.Update(panelID, map[string]interface{}{"MethodUsername": method}); err != nil {
		return c.Send("❌ بروزرسانی روش ساخت نام کاربری ناموفق بود.")
	}
	delete(state, "panel_method_username")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func (b *Bot) handleAdminPanelSetNameCustomInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}
	panelID := b.getPanelIDFromAdminState(adminUser)
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	nameCustom := strings.TrimSpace(text)
	if nameCustom == "" {
		return c.Send("متن دلخواه نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	method := strings.TrimSpace(state["panel_method_username"])
	if method == "" {
		method = "آیدی عددی + حروف و عدد رندوم"
	}
	if err := b.repos.Panel.Update(panelID, map[string]interface{}{
		"MethodUsername": method,
		"namecustom":     nameCustom,
	}); err != nil {
		return c.Send("❌ بروزرسانی تنظیمات نام کاربری ناموفق بود.")
	}
	delete(state, "panel_method_username")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func (b *Bot) handleAdminPanelSetTestTimeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}
	panelID := b.getPanelIDFromAdminState(adminUser)
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	days := parseIntSafe(strings.TrimSpace(text))
	if days <= 0 {
		return c.Send("زمان سرویس تست نامعتبر است.")
	}
	if err := b.repos.Panel.Update(panelID, map[string]interface{}{"time_usertest": strconv.Itoa(days)}); err != nil {
		return c.Send("❌ بروزرسانی زمان سرویس تست ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func (b *Bot) handleAdminPanelSetTestVolumeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل‌ها را مدیریت کند.")
	}
	panelID := b.getPanelIDFromAdminState(adminUser)
	if panelID <= 0 {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه پنل مشخص نیست. دوباره تلاش کنید.")
	}
	volume := parseIntSafe(strings.TrimSpace(text))
	if volume <= 0 {
		return c.Send("حجم سرویس تست نامعتبر است.")
	}
	if err := b.repos.Panel.Update(panelID, map[string]interface{}{"val_usertest": strconv.Itoa(volume)}); err != nil {
		return c.Send("❌ بروزرسانی حجم سرویس تست ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return b.sendAdminPanelDetail(c, panelID)
}

func (b *Bot) handleAdminAddPanelNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل اضافه کند.")
	}

	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام پنل نمی‌تواند خالی باشد.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	panelType := normalizePanelType(state["type"])
	if panelType == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("ابتدا نوع پنل را انتخاب کنید.", b.adminPanelTypeKeyboard())
	}

	count, err := b.repos.Panel.CountByName(name)
	if err == nil && count > 0 {
		return c.Send("این نام پنل قبلاً ثبت شده است. نام دیگری انتخاب کنید.")
	}

	state["name"] = name
	b.saveAdminState(adminID, state)

	_ = b.repos.User.UpdateStep(adminID, "admin_add_panel_url")
	return c.Send("لینک پنل را ارسال کنید. مثال: https://panel.example.com")
}

func (b *Bot) handleAdminAddPanelURLInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل اضافه کند.")
	}

	raw := strings.TrimSpace(text)
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return c.Send("لینک معتبر نیست. مثال: https://panel.example.com")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	panelType := normalizePanelType(state["type"])
	if panelType == "" {
		return c.Send("نوع پنل مشخص نیست. دوباره شروع کنید.", b.adminPanelTypeKeyboard())
	}
	state["url"] = strings.TrimRight(raw, "/")
	b.saveAdminState(adminID, state)

	switch panelType {
	case "hiddify":
		state["username"] = "null"
		state["password"] = "null"
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_add_panel_limit")
		return c.Send("ظرفیت پنل را ارسال کنید (عدد).")
	case "s_ui", "wgdashboard":
		state["username"] = "null"
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_add_panel_password")
		return c.Send("توکن پنل را ارسال کنید.")
	default:
		_ = b.repos.User.UpdateStep(adminID, "admin_add_panel_username")
		return c.Send("نام کاربری پنل را ارسال کنید.")
	}
}

func (b *Bot) handleAdminAddPanelUsernameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل اضافه کند.")
	}

	username := strings.TrimSpace(text)
	if username == "" {
		return c.Send("نام کاربری پنل نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["username"] = username
	b.saveAdminState(adminID, state)

	_ = b.repos.User.UpdateStep(adminID, "admin_add_panel_password")
	return c.Send("رمز پنل را ارسال کنید.")
}

func (b *Bot) handleAdminAddPanelPasswordInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل اضافه کند.")
	}

	password := strings.TrimSpace(text)
	if password == "" {
		return c.Send("رمز/توکن پنل نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["password"] = password
	b.saveAdminState(adminID, state)

	_ = b.repos.User.UpdateStep(adminID, "admin_add_panel_limit")
	return c.Send("ظرفیت پنل را ارسال کنید (عدد).")
}

func (b *Bot) handleAdminAddPanelLimitInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پنل اضافه کند.")
	}

	limit := parseIntSafe(strings.TrimSpace(text))
	if limit <= 0 {
		return c.Send("ظرفیت نامعتبر است. فقط عدد بزرگتر از صفر ارسال کنید.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	panelType := normalizePanelType(state["type"])
	name := strings.TrimSpace(state["name"])
	panelURL := strings.TrimSpace(state["url"])
	username := strings.TrimSpace(state["username"])
	password := strings.TrimSpace(state["password"])

	if panelType == "" || name == "" || panelURL == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		b.clearAdminState(adminID)
		return c.Send("اطلاعات پنل کامل نیست. لطفاً دوباره از اول شروع کنید.", b.adminPanelManageKeyboard())
	}

	count, err := b.repos.Panel.CountByName(name)
	if err == nil && count > 0 {
		return c.Send("این نام پنل قبلاً ثبت شده است. نام دیگری انتخاب کنید.")
	}

	panel := buildPanelWithDefaults(panelType, name, panelURL, username, password, limit)
	if err := b.repos.Panel.Create(panel); err != nil {
		return c.Send("❌ خطا در ثبت پنل.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	b.clearAdminState(adminID)

	msg := fmt.Sprintf("✅ پنل %s با موفقیت ثبت شد.\nنوع: %s", name, panelTypeLabel(panelType))
	switch panelType {
	case "x-ui_single", "alireza_single":
		msg += "\nنکته: شناسه اینباند و دامنه لینک ساب را در تنظیمات پنل کامل کنید."
	case "marzban", "pasarguard", "marzneshin", "hiddify", "s_ui":
		msg += "\nنکته: تنظیمات اینباند/پروتکل پنل را بعد از افزودن بررسی کنید."
	case "wgdashboard":
		msg += "\nنکته: شناسه کانفیگ/اینباند را در تنظیمات پنل تعیین کنید."
	case "ibsng":
		msg += "\nنکته: نام گروه پیشفرض IBSng را در تنظیمات پنل وارد کنید."
	}

	return c.Send(msg, b.adminPanelManageKeyboard())
}

func (b *Bot) adminChannelManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("➕ افزودن کانال"), menu.Text("📋 لیست کانال‌ها")),
		menu.Row(menu.Text("🗑 حذف کانال"), menu.Text("🗑 حذف همه کانال‌ها")),
		menu.Row(menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminChannelManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند کانال‌ها را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("📡 مدیریت کانال‌ها", b.adminChannelManageKeyboard())
}

func (b *Bot) sendAdminChannelList(c tele.Context) error {
	channels, err := b.repos.Setting.GetChannels()
	if err != nil {
		return c.Send("❌ خطا در دریافت لیست کانال‌ها.")
	}
	if len(channels) == 0 {
		return c.Send("کانالی ثبت نشده است.", b.adminChannelManageKeyboard())
	}

	lines := make([]string, 0, len(channels)+1)
	lines = append(lines, "📋 <b>لیست کانال‌ها</b>")
	for i, ch := range channels {
		lines = append(lines, fmt.Sprintf("%d) %s\n🔗 عضویت: %s\n📡 لینک/ID: %s", i+1, emptyDash(ch.Remark), emptyDash(ch.LinkJoin), emptyDash(ch.Link)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func (b *Bot) sendAdminChannelRemoveList(c tele.Context, chatID string) error {
	channels, err := b.repos.Setting.GetChannels()
	if err != nil {
		return c.Send("❌ خطا در دریافت کانال‌ها.")
	}
	if len(channels) == 0 {
		return c.Send("کانالی برای حذف وجود ندارد.", b.adminChannelManageKeyboard())
	}

	state := decodeAdminState("")
	if u, err := b.repos.User.FindByID(chatID); err == nil && u != nil {
		state = decodeAdminState(u.ProcessingValue)
	}
	for k := range state {
		if strings.HasPrefix(k, "chan_rm_") {
			delete(state, k)
		}
	}

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(channels)+1)
	for _, ch := range channels {
		token := strings.ToLower(utils.RandomCode(6))
		state["chan_rm_"+token] = ch.Remark
		rows = append(rows, menu.Row(menu.Data("❌ "+emptyDash(ch.Remark), "admin_channel_rm_"+token)))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 مدیریت کانال", "admin_channel_manage")))
	b.saveAdminState(chatID, state)
	menu.Inline(rows...)
	return c.Send("کدام کانال حذف شود؟", menu)
}

func (b *Bot) handleAdminChannelCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند کانال‌ها را مدیریت کند.")
	}

	switch {
	case data == "admin_channel_manage":
		return b.sendAdminChannelManageMenu(c, adminID)
	case strings.HasPrefix(data, "admin_channel_rm_"):
		token := strings.TrimPrefix(data, "admin_channel_rm_")
		state := decodeAdminState(adminUser.ProcessingValue)
		remark := strings.TrimSpace(state["chan_rm_"+token])
		if remark == "" {
			return c.Send("کانال انتخابی پیدا نشد.")
		}
		if err := b.repos.Setting.DeleteChannelByRemark(remark); err != nil {
			return c.Send("❌ حذف کانال ناموفق بود.")
		}
		delete(state, "chan_rm_"+token)
		b.saveAdminState(adminID, state)
		return c.Send("✅ کانال حذف شد.", b.adminChannelManageKeyboard())
	}
	return c.Send("عملیات کانال نامعتبر است.")
}

func (b *Bot) handleAdminChannelAddNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند کانال اضافه کند.")
	}

	remark := strings.TrimSpace(text)
	if remark == "" {
		return c.Send("نام دکمه کانال نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["channel_remark"] = remark
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_channel_add_join")
	return c.Send("لینک عضویت را ارسال کنید. مثال: https://t.me/xxxx")
}

func (b *Bot) handleAdminChannelAddJoinInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند کانال اضافه کند.")
	}

	linkJoin := strings.TrimSpace(text)
	if _, err := url.ParseRequestURI(linkJoin); err != nil {
		return c.Send("لینک عضویت معتبر نیست. مثال: https://t.me/xxxx")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["channel_join"] = linkJoin
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_channel_add_link")
	return c.Send("لینک یا آیدی کانال را ارسال کنید (مثل @channel یا https://t.me/channel).")
}

func (b *Bot) handleAdminChannelAddLinkInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند کانال اضافه کند.")
	}

	link := strings.TrimSpace(text)
	if link == "" {
		return c.Send("لینک/آیدی کانال نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	remark := strings.TrimSpace(state["channel_remark"])
	linkJoin := strings.TrimSpace(state["channel_join"])
	if remark == "" || linkJoin == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		b.clearAdminState(adminID)
		return c.Send("اطلاعات کانال ناقص است. دوباره تلاش کنید.", b.adminChannelManageKeyboard())
	}

	ch := &models.Channel{
		Remark:   remark,
		LinkJoin: linkJoin,
		Link:     link,
	}
	if err := b.repos.Setting.CreateChannel(ch); err != nil {
		return c.Send("❌ ثبت کانال ناموفق بود.")
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	b.clearAdminState(adminID)
	return c.Send("✅ کانال با موفقیت ثبت شد.", b.adminChannelManageKeyboard())
}

var adminEditableTextKeys = []string{
	"textstart", "text_sell", "text_extend", "text_usertest", "text_wheel_luck",
	"text_Purchased_services", "accountwallet", "text_affiliates", "text_Tariff_list",
	"text_support", "text_help", "text_fq", "text_dec_fq", "text_channel",
	"textselectlocation", "text_pishinvoice", "textafterpay", "textafterpayibsng",
	"text_cart", "text_cart_auto", "textaftertext", "textmanual", "crontest",
	"text_wgdashboard", "text_Account", "textrequestagent", "textpanelagent",
	"text_Add_Balance", "textlistpanel", "text_roll",
}

func (b *Bot) adminTextManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("📋 لیست متن‌ها"), menu.Text("🆔 ویرایش با کلید")),
		menu.Row(menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminTextManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند متن‌ها را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("📝 مدیریت متن‌ها", b.adminTextManageKeyboard())
}

func (b *Bot) sendAdminTextList(c tele.Context, page int) error {
	if page < 1 {
		page = 1
	}
	perPage := 10
	total := len(adminEditableTextKeys)
	totalPages := (total + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * perPage
	end := start + perPage
	if end > total {
		end = total
	}

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, perPage+2)
	for _, key := range adminEditableTextKeys[start:end] {
		rows = append(rows, menu.Row(menu.Data(key, "admin_text_pick_"+key)))
	}
	nav := []tele.Btn{}
	if page > 1 {
		nav = append(nav, menu.Data("⬅️", fmt.Sprintf("admin_text_list_%d", page-1)))
	}
	nav = append(nav, menu.Data(fmt.Sprintf("%d/%d", page, totalPages), "admin_text_noop"))
	if page < totalPages {
		nav = append(nav, menu.Data("➡️", fmt.Sprintf("admin_text_list_%d", page+1)))
	}
	rows = append(rows, menu.Row(nav...))
	rows = append(rows, menu.Row(menu.Data("🔙 مدیریت متن", "admin_text_manage")))
	menu.Inline(rows...)
	return c.Send("کلید متن مورد نظر را انتخاب کنید:", menu)
}

func (b *Bot) handleAdminTextCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند متن‌ها را مدیریت کند.")
	}

	switch {
	case data == "admin_text_manage":
		return b.sendAdminTextManageMenu(c, adminID)
	case strings.HasPrefix(data, "admin_text_list_"):
		page := parseIntSafe(strings.TrimPrefix(data, "admin_text_list_"))
		return b.sendAdminTextList(c, page)
	case strings.HasPrefix(data, "admin_text_pick_"):
		key := strings.TrimPrefix(data, "admin_text_pick_")
		found := false
		for _, k := range adminEditableTextKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			return c.Send("کلید متن نامعتبر است.")
		}
		current, _ := b.repos.Setting.GetText(key)
		state := decodeAdminState(adminUser.ProcessingValue)
		state["text_key"] = key
		b.saveAdminState(adminID, state)
		_ = b.repos.User.UpdateStep(adminID, "admin_text_set_value")
		return c.Send(fmt.Sprintf("کلید: <code>%s</code>\nمتن فعلی:\n<code>%s</code>\n\nمتن جدید را ارسال کنید.", key, emptyDash(current)), tele.ModeHTML)
	case data == "admin_text_noop":
		return nil
	}
	return c.Send("عملیات متن نامعتبر است.")
}

func (b *Bot) handleAdminTextSetValueInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند متن‌ها را مدیریت کند.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	key := strings.TrimSpace(state["text_key"])
	if key == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کلید متن مشخص نیست. دوباره از لیست انتخاب کنید.")
	}
	if err := b.repos.Setting.SetText(key, text); err != nil {
		return c.Send("❌ ذخیره متن ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	delete(state, "text_key")
	b.saveAdminState(adminID, state)
	return c.Send("✅ متن با موفقیت ذخیره شد.", b.adminTextManageKeyboard())
}

func (b *Bot) adminHelpManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("➕ افزودن آموزش"), menu.Text("📋 لیست آموزش")),
		menu.Row(menu.Text("🗑 حذف آموزش"), menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminHelpManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش‌ها را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("📚 مدیریت آموزش", b.adminHelpManageKeyboard())
}

func (b *Bot) sendAdminHelpList(c tele.Context) error {
	items, err := b.repos.Setting.GetAllHelp()
	if err != nil {
		return c.Send("❌ خطا در دریافت آموزش‌ها.")
	}
	if len(items) == 0 {
		return c.Send("آموزشی ثبت نشده است.", b.adminHelpManageKeyboard())
	}
	lines := []string{"📚 <b>لیست آموزش‌ها</b>"}
	for _, h := range items {
		lines = append(lines, fmt.Sprintf("ID: <code>%d</code>\n🏷 %s\n📝 %s", h.ID, emptyDash(h.NameOS), emptyDash(h.DescriptionOS)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func (b *Bot) sendAdminHelpDeleteList(c tele.Context) error {
	items, err := b.repos.Setting.GetAllHelp()
	if err != nil {
		return c.Send("❌ خطا در دریافت آموزش‌ها.")
	}
	if len(items) == 0 {
		return c.Send("آموزشی برای حذف وجود ندارد.", b.adminHelpManageKeyboard())
	}
	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(items)+1)
	for _, h := range items {
		title := fmt.Sprintf("❌ %d | %s", h.ID, h.NameOS)
		rows = append(rows, menu.Row(menu.Data(title, fmt.Sprintf("admin_help_del_%d", h.ID))))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 مدیریت آموزش", "admin_help_manage")))
	menu.Inline(rows...)
	return c.Send("کدام آموزش حذف شود؟", menu)
}

func (b *Bot) handleAdminHelpCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش‌ها را مدیریت کند.")
	}

	switch {
	case data == "admin_help_manage":
		return b.sendAdminHelpManageMenu(c, adminID)
	case strings.HasPrefix(data, "admin_help_del_"):
		id := parseIntSafe(strings.TrimPrefix(data, "admin_help_del_"))
		if id <= 0 {
			return c.Send("شناسه آموزش نامعتبر است.")
		}
		if err := b.repos.Setting.DeleteHelp(id); err != nil {
			return c.Send("❌ حذف آموزش ناموفق بود.")
		}
		return c.Send("✅ آموزش حذف شد.", b.adminHelpManageKeyboard())
	}
	return c.Send("عملیات آموزش نامعتبر است.")
}

func (b *Bot) handleAdminHelpAddNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش اضافه کند.")
	}

	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام آموزش نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["help_name"] = name
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_help_add_desc")
	return c.Send("توضیحات آموزش را ارسال کنید.")
}

func (b *Bot) handleAdminHelpAddDescInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند آموزش اضافه کند.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	name := strings.TrimSpace(state["help_name"])
	if name == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("نام آموزش مشخص نیست. دوباره تلاش کنید.")
	}

	item := &models.Help{
		NameOS:        name,
		DescriptionOS: text,
		Category:      "general",
		MediaOS:       "",
		TypeMediaOS:   "none",
	}
	if err := b.repos.Setting.CreateHelp(item); err != nil {
		return c.Send("❌ ذخیره آموزش ناموفق بود.")
	}

	delete(state, "help_name")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ آموزش با موفقیت ثبت شد.", b.adminHelpManageKeyboard())
}

type featureToggleItem struct {
	Column string
	Label  string
	On     string
	Off    string
}

var adminFeatureToggleItems = []featureToggleItem{
	{Column: "Bot_Status", Label: "روشن/خاموش ربات", On: "botstatuson", Off: "botstatusoff"},
	{Column: "get_number", Label: "دریافت شماره کاربر", On: "on_number", Off: "off_number"},
	{Column: "statuscategory", Label: "حالت دسته‌بندی خرید", On: "oncategory", Off: "offcategory"},
	{Column: "statusagentrequest", Label: "درخواست نمایندگی", On: "onrequestagent", Off: "offrequestagent"},
	{Column: "statusnewuser", Label: "اعلان کاربر جدید", On: "onnewuser", Off: "offnewuser"},
	{Column: "roll_Status", Label: "نمایش قوانین", On: "rolleon", Off: "rolleoff"},
	{Column: "iran_number", Label: "محدودیت شماره ایران", On: "onAuthenticationiran", Off: "offAuthenticationiran"},
	{Column: "verifystart", Label: "بررسی شروع", On: "onverify", Off: "offverify"},
	{Column: "statussupportpv", Label: "پشتیبانی در پیوی", On: "onpvsupport", Off: "offpvsupport"},
	{Column: "statusnamecustom", Label: "نام سفارشی", On: "onnamecustom", Off: "offnamecustom"},
	{Column: "bulkbuy", Label: "خرید عمده", On: "onbulk", Off: "offbulk"},
	{Column: "affiliatesstatus", Label: "زیرمجموعه‌گیری", On: "onaffiliates", Off: "offaffiliates"},
	{Column: "inlinebtnmain", Label: "منوی اینلاین", On: "oninline", Off: "offinline"},
	{Column: "linkappstatus", Label: "لینک برنامه", On: "onlinkapp", Off: "offlinkapp"},
	{Column: "btn_status_extned", Label: "دکمه تمدید", On: "onextned", Off: "offextned"},
	{Column: "scorestatus", Label: "سیستم امتیاز", On: "1", Off: "0"},
	{Column: "verifybucodeuser", Label: "بررسی کد تخفیف", On: "on", Off: "off"},
}

func (b *Bot) sendAdminFeatureToggleMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند قابلیت‌ها را مدیریت کند.")
	}

	setting, err := b.repos.Setting.GetSettings()
	if err != nil || setting == nil {
		return c.Send("❌ تنظیمات یافت نشد.")
	}

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(adminFeatureToggleItems)+1)
	for _, item := range adminFeatureToggleItems {
		current := getSettingColumnValue(setting, item.Column)
		stateText := "❌ خاموش"
		if strings.EqualFold(strings.TrimSpace(current), item.On) {
			stateText = "✅ روشن"
		}
		btn := menu.Data(fmt.Sprintf("%s | %s", item.Label, stateText), "admin_feature_toggle_"+item.Column)
		rows = append(rows, menu.Row(btn))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 پنل مدیریت", "admin_panel")))
	menu.Inline(rows...)
	return c.Send("⚙️ وضعیت قابلیت‌ها (برای تغییر روی هر مورد بزنید):", menu)
}

func (b *Bot) handleAdminFeatureCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند قابلیت‌ها را مدیریت کند.")
	}

	column := strings.TrimPrefix(data, "admin_feature_toggle_")
	var target *featureToggleItem
	for i := range adminFeatureToggleItems {
		if adminFeatureToggleItems[i].Column == column {
			target = &adminFeatureToggleItems[i]
			break
		}
	}
	if target == nil {
		return c.Send("گزینه نامعتبر است.")
	}

	setting, err := b.repos.Setting.GetSettings()
	if err != nil || setting == nil {
		return c.Send("❌ تنظیمات یافت نشد.")
	}
	current := getSettingColumnValue(setting, target.Column)
	next := target.On
	if strings.EqualFold(strings.TrimSpace(current), target.On) {
		next = target.Off
	}
	if err := b.repos.Setting.UpdateSetting(target.Column, next); err != nil {
		return c.Send("❌ بروزرسانی تنظیمات ناموفق بود.")
	}
	return b.sendAdminFeatureToggleMenu(c, adminID)
}

func (b *Bot) adminBroadcastKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("✉️ پیام به همه کاربران فعال"), menu.Text("🛍 پیام به کاربران دارای سرویس")),
		menu.Row(menu.Text("🆕 پیام به کاربران بدون سرویس"), menu.Text("📴 پیام به کاربران غیرفعال")),
		menu.Row(menu.Text("📌 لغو پین برای همه"), menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminBroadcastMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("📣 مدیریت پیام همگانی", b.adminBroadcastKeyboard())
}

func (b *Bot) handleAdminBroadcastInactiveDaysInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
	}
	days := parseIntSafe(strings.TrimSpace(text))
	if days <= 0 {
		return c.Send("عدد روز نامعتبر است. مثال: 7")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	state["broadcast_target"] = "inactive_days"
	state["broadcast_days"] = strconv.Itoa(days)
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_broadcast_text")
	return c.Send("متن پیام را ارسال کنید.")
}

func (b *Bot) enqueueBroadcastJob(adminID, messageType, message, targetMode string, days int) (uint, int, error) {
	if b.repos.CronJob == nil {
		return 0, 0, fmt.Errorf("ماژول صف پیام در دسترس نیست")
	}

	active, err := b.repos.CronJob.HasActiveKind("sendmessage")
	if err == nil && active {
		return 0, 0, fmt.Errorf("یک عملیات پیام همگانی در حال اجراست")
	}

	targets, err := b.loadBroadcastTargets(targetMode, days)
	if err != nil {
		return 0, 0, err
	}
	if len(targets) == 0 {
		return 0, 0, fmt.Errorf("کاربری برای این عملیات یافت نشد")
	}

	payload := map[string]interface{}{
		"id_admin":    adminID,
		"id_message":  0,
		"type":        messageType,
		"message":     message,
		"pingmessage": "no",
		"btnmessage":  "",
	}
	ref := fmt.Sprintf("admin-sendmessage:%s:%s:%d", adminID, strings.ToLower(messageType), time.Now().Unix())
	job, err := b.repos.CronJob.CreateJobWithItems("sendmessage", ref, payload, targets)
	if err != nil {
		return 0, 0, fmt.Errorf("ایجاد صف ارسال ناموفق بود")
	}
	return job.ID, len(targets), nil
}

func (b *Bot) loadBroadcastTargets(mode string, days int) ([]string, error) {
	db := b.repos.Setting.DB()
	targets := make([]string, 0)

	switch strings.TrimSpace(mode) {
	case "", "all_active":
		if err := db.Model(&models.User{}).
			Where("User_Status NOT IN ?", []string{"block", "blocked"}).
			Pluck("id", &targets).Error; err != nil {
			return nil, fmt.Errorf("خطا در دریافت کاربران")
		}
	case "with_service":
		err := db.Raw(`
			SELECT u.id
			FROM user u
			WHERE u.User_Status NOT IN ('block','blocked')
			AND EXISTS (SELECT 1 FROM invoice i WHERE i.id_user = u.id AND i.Status != 'Unpaid')
		`).Scan(&targets).Error
		if err != nil {
			return nil, fmt.Errorf("خطا در دریافت کاربران دارای سرویس")
		}
	case "without_service":
		err := db.Raw(`
			SELECT u.id
			FROM user u
			WHERE u.User_Status NOT IN ('block','blocked')
			AND NOT EXISTS (SELECT 1 FROM invoice i WHERE i.id_user = u.id)
		`).Scan(&targets).Error
		if err != nil {
			return nil, fmt.Errorf("خطا در دریافت کاربران بدون سرویس")
		}
	case "inactive_days":
		if days <= 0 {
			return nil, fmt.Errorf("تعداد روز نامعتبر است")
		}
		threshold := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
		err := db.Raw(`
			SELECT u.id
			FROM user u
			WHERE u.User_Status NOT IN ('block','blocked')
			AND (CAST(COALESCE(NULLIF(u.last_message_time,''),'0') AS UNSIGNED) < ?)
		`, threshold).Scan(&targets).Error
		if err != nil {
			return nil, fmt.Errorf("خطا در دریافت کاربران غیرفعال")
		}
	default:
		return nil, fmt.Errorf("حالت ارسال نامعتبر است")
	}
	return targets, nil
}

func (b *Bot) handleAdminBroadcastTextInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند پیام همگانی ارسال کند.")
	}
	if b.repos.CronJob == nil {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("❌ ماژول صف پیام در دسترس نیست.")
	}

	msg := strings.TrimSpace(text)
	if msg == "" {
		return c.Send("متن پیام نمی‌تواند خالی باشد.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	targetMode := strings.TrimSpace(state["broadcast_target"])
	if targetMode == "" {
		targetMode = "all_active"
	}
	days := parseIntSafe(strings.TrimSpace(state["broadcast_days"]))

	jobID, count, err := b.enqueueBroadcastJob(adminID, "sendmessage", msg, targetMode, days)
	if err != nil {
		return c.Send("❌ " + err.Error())
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	delete(state, "broadcast_target")
	delete(state, "broadcast_days")
	b.saveAdminState(adminID, state)
	return c.Send(fmt.Sprintf("✅ ارسال همگانی در صف قرار گرفت.\nشناسه عملیات: %d\nگیرندگان: %d", jobID, count), b.adminMenuKeyboard("administrator"))
}

func (b *Bot) adminAdminManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("📋 لیست ادمین‌ها"), menu.Text("➕ افزودن ادمین")),
		menu.Row(menu.Text("🗑 حذف ادمین"), menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminAdminManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین‌ها را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("👥 مدیریت ادمین‌ها", b.adminAdminManageKeyboard())
}

func (b *Bot) sendAdminAdminsList(c tele.Context, removeMode bool) error {
	admins, err := b.repos.Setting.GetAllAdmins()
	if err != nil {
		return c.Send("❌ خطا در دریافت لیست ادمین‌ها.")
	}
	if len(admins) == 0 {
		return c.Send("ادمینی ثبت نشده است.", b.adminAdminManageKeyboard())
	}

	if !removeMode {
		lines := []string{"👥 <b>لیست ادمین‌ها</b>"}
		for _, a := range admins {
			lines = append(lines, fmt.Sprintf("🆔 <code>%s</code>\n🎭 نقش: <b>%s</b>", emptyDash(a.IDAdmin), emptyDash(a.Rule)))
		}
		return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
	}

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(admins)+1)
	for _, a := range admins {
		label := fmt.Sprintf("❌ %s (%s)", a.IDAdmin, a.Rule)
		rows = append(rows, menu.Row(menu.Data(label, "admin_admin_del_"+a.IDAdmin)))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 مدیریت ادمین", "admin_admin_manage")))
	menu.Inline(rows...)
	return c.Send("کدام ادمین حذف شود؟", menu)
}

func (b *Bot) handleAdminAdminCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند ادمین‌ها را مدیریت کند.")
	}

	switch {
	case data == "admin_admin_manage":
		return b.sendAdminAdminManageMenu(c, adminID)
	case strings.HasPrefix(data, "admin_admin_del_"):
		target := strings.TrimSpace(strings.TrimPrefix(data, "admin_admin_del_"))
		if target == "" {
			return c.Send("شناسه ادمین نامعتبر است.")
		}
		if target == adminID {
			return c.Send("⛔ حذف خودتان مجاز نیست.")
		}
		if err := b.repos.Setting.DeleteAdminByID(target); err != nil {
			return c.Send("❌ حذف ادمین ناموفق بود.")
		}
		return c.Send("✅ ادمین حذف شد.", b.adminAdminManageKeyboard())
	}
	return c.Send("عملیات ادمین نامعتبر است.")
}

func (b *Bot) adminAppManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("➕ افزودن برنامه"), menu.Text("✏️ ویرایش لینک برنامه")),
		menu.Row(menu.Text("🗑 حذف برنامه"), menu.Text("📋 لیست برنامه‌ها")),
		menu.Row(menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminAppManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه‌ها را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("📱 مدیریت برنامه‌ها", b.adminAppManageKeyboard())
}

func (b *Bot) sendAdminAppsList(c tele.Context) error {
	apps, err := b.repos.Setting.GetAllApps()
	if err != nil {
		return c.Send("❌ خطا در دریافت لیست برنامه‌ها.")
	}
	if len(apps) == 0 {
		return c.Send("برنامه‌ای ثبت نشده است.", b.adminAppManageKeyboard())
	}
	lines := []string{"📱 <b>لیست برنامه‌ها</b>"}
	for _, app := range apps {
		lines = append(lines, fmt.Sprintf("🏷 <b>%s</b>\n🔗 <code>%s</code>", emptyDash(app.Name), emptyDash(app.Link)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func (b *Bot) handleAdminAppAddNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه اضافه کند.")
	}
	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام برنامه نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["app_name"] = name
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_app_add_link")
	return c.Send("لینک برنامه را ارسال کنید.")
}

func (b *Bot) handleAdminAppAddLinkInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه اضافه کند.")
	}
	link := strings.TrimSpace(text)
	if _, err := url.ParseRequestURI(link); err != nil {
		return c.Send("لینک معتبر نیست. مثال: https://example.com/app")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	name := strings.TrimSpace(state["app_name"])
	if name == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("نام برنامه مشخص نیست. دوباره تلاش کنید.")
	}
	if err := b.repos.Setting.CreateApp(&models.App{Name: name, Link: link}); err != nil {
		return c.Send("❌ ثبت برنامه ناموفق بود.")
	}
	delete(state, "app_name")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ لینک برنامه با موفقیت اضافه شد.", b.adminAppManageKeyboard())
}

func (b *Bot) handleAdminAppEditNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه را ویرایش کند.")
	}
	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام برنامه نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["app_name"] = name
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_app_edit_link")
	return c.Send("لینک جدید برنامه را ارسال کنید.")
}

func (b *Bot) handleAdminAppEditLinkInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه را ویرایش کند.")
	}
	link := strings.TrimSpace(text)
	if _, err := url.ParseRequestURI(link); err != nil {
		return c.Send("لینک معتبر نیست. مثال: https://example.com/app")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	name := strings.TrimSpace(state["app_name"])
	if name == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("نام برنامه مشخص نیست. دوباره تلاش کنید.")
	}
	if err := b.repos.Setting.UpdateAppLink(name, link); err != nil {
		return c.Send("❌ بروزرسانی لینک برنامه ناموفق بود.")
	}
	delete(state, "app_name")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ لینک برنامه بروزرسانی شد.", b.adminAppManageKeyboard())
}

func (b *Bot) handleAdminAppRemoveNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند برنامه حذف کند.")
	}
	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام برنامه نمی‌تواند خالی باشد.")
	}
	if err := b.repos.Setting.DeleteAppByName(name); err != nil {
		return c.Send("❌ حذف برنامه ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ برنامه حذف شد.", b.adminAppManageKeyboard())
}

func (b *Bot) adminFinanceKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("💳 تنظیم شماره کارت"), menu.Text("📋 لیست کارت‌ها")),
		menu.Row(menu.Text("❌ حذف شماره کارت"), menu.Text("♻️ تایید خودکار رسید")),
		menu.Row(menu.Text("🎁 کش بک کارت"), menu.Text("🔒 نمایش کارت بعد پرداخت اول")),
		menu.Row(menu.Text("🧩 API NowPayments"), menu.Text("🧩 API Ternado")),
		menu.Row(menu.Text("🧩 مرچنت زرین پال"), menu.Text("🧩 مرچنت آقای پرداخت")),
		menu.Row(menu.Text("🧩 مرچنت FloyPay"), menu.Text("🧩 مرچنت TronSeller")),
		menu.Row(menu.Text("📋 لیست درگاه‌ها"), menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminFinanceMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("💎 مدیریت مالی", b.adminFinanceKeyboard())
}

func (b *Bot) sendAdminFinanceCardList(c tele.Context) error {
	cards, err := b.repos.Setting.GetAllCardNumbers()
	if err != nil {
		return c.Send("❌ خطا در دریافت لیست کارت‌ها.")
	}
	if len(cards) == 0 {
		return c.Send("کارتی ثبت نشده است.", b.adminFinanceKeyboard())
	}
	lines := []string{"💳 <b>لیست شماره کارت‌ها</b>"}
	for _, card := range cards {
		lines = append(lines, fmt.Sprintf("• <code>%s</code>\n👤 %s", emptyDash(card.CardNumber), emptyDash(card.NameCard)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func parseAdminCardInput(raw string) (string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ""
	}
	delimiters := []string{"|", ",", "\n"}
	for _, delim := range delimiters {
		if !strings.Contains(value, delim) {
			continue
		}
		parts := strings.SplitN(value, delim, 2)
		card := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		return card, name
	}
	return "", ""
}

func isLikelyCardNumber(card string) bool {
	if card == "" {
		return false
	}
	if len(card) < 12 || len(card) > 24 {
		return false
	}
	for _, ch := range card {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (b *Bot) handleAdminFinanceCardAddInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
	}

	cardNum, cardName := parseAdminCardInput(text)
	if !isLikelyCardNumber(cardNum) || strings.TrimSpace(cardName) == "" {
		return c.Send("ورودی نامعتبر است.\nفرمت درست: <code>6037991234567890|Ali Ahmadi</code>", tele.ModeHTML)
	}

	if err := b.repos.Setting.SaveCardNumber(cardNum, cardName); err != nil {
		return c.Send("❌ ذخیره شماره کارت ناموفق بود.")
	}
	_ = b.repos.Setting.SetPaySetting("cardnum", cardNum)
	_ = b.repos.Setting.SetPaySetting("cardname", cardName)

	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ شماره کارت ثبت شد.", b.adminFinanceKeyboard())
}

func (b *Bot) handleAdminFinanceCardRemoveInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
	}

	cardNum := strings.TrimSpace(text)
	if !isLikelyCardNumber(cardNum) {
		return c.Send("شماره کارت نامعتبر است.")
	}
	if err := b.repos.Setting.DeleteCardNumber(cardNum); err != nil {
		return c.Send("❌ حذف شماره کارت ناموفق بود.")
	}

	currentCard, _ := b.repos.Setting.GetPaySetting("cardnum")
	if strings.TrimSpace(currentCard) == cardNum {
		cards, _ := b.repos.Setting.GetAllCardNumbers()
		if len(cards) > 0 {
			_ = b.repos.Setting.SetPaySetting("cardnum", cards[0].CardNumber)
			_ = b.repos.Setting.SetPaySetting("cardname", cards[0].NameCard)
		} else {
			_ = b.repos.Setting.SetPaySetting("cardnum", "")
			_ = b.repos.Setting.SetPaySetting("cardname", "")
		}
	}

	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ شماره کارت حذف شد.", b.adminFinanceKeyboard())
}

func (b *Bot) beginAdminPaySettingInput(c tele.Context, adminUser *models.User, key, label, hint string) error {
	adminID := adminUser.ID
	current, _ := b.repos.Setting.GetPaySetting(key)

	state := decodeAdminState(adminUser.ProcessingValue)
	state["pay_key"] = key
	state["pay_label"] = label
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_finance_pay_value")

	return c.Send(
		fmt.Sprintf(
			"تنظیم: <b>%s</b>\nمقدار فعلی: <code>%s</code>\nمقدار جدید را ارسال کنید. راهنما: <code>%s</code>",
			label,
			emptyDash(current),
			emptyDash(hint),
		),
		tele.ModeHTML,
	)
}

func (b *Bot) handleAdminFinancePaySettingInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات مالی را مدیریت کند.")
	}

	value := strings.TrimSpace(text)
	if value == "" {
		return c.Send("مقدار نمی‌تواند خالی باشد.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	key := strings.TrimSpace(state["pay_key"])
	label := strings.TrimSpace(state["pay_label"])
	if key == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("کلید تنظیم مشخص نیست. دوباره از منوی مالی شروع کنید.")
	}

	switch key {
	case "chashbackcart":
		pct := parseIntSafe(value)
		if pct < 0 || pct > 100 {
			return c.Send("درصد کش‌بک باید بین 0 تا 100 باشد.")
		}
		value = strconv.Itoa(pct)
	case "autoconfirmcart":
		normalized := strings.ToLower(value)
		switch normalized {
		case "on", "onauto", "true", "1":
			value = "onauto"
		case "off", "offauto", "false", "0":
			value = "offauto"
		}
	}

	if err := b.repos.Setting.SetPaySetting(key, value); err != nil {
		return c.Send("❌ ذخیره تنظیم مالی ناموفق بود.")
	}
	// Compatibility aliases used in legacy PHP/Go paths.
	if key == "apinowpayment" {
		_ = b.repos.Setting.SetPaySetting("apikey_nowpayment", value)
	}

	delete(state, "pay_key")
	delete(state, "pay_label")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send(fmt.Sprintf("✅ مقدار «%s» ذخیره شد.", emptyDash(label)), b.adminFinanceKeyboard())
}

func (b *Bot) sendAdminFinanceGatewayList(c tele.Context) error {
	type kv struct {
		Label string
		Key   string
	}
	items := []kv{
		{Label: "API NowPayments", Key: "apinowpayment"},
		{Label: "API Ternado", Key: "apiternado"},
		{Label: "مرچنت زرین پال", Key: "merchant_zarinpal"},
		{Label: "مرچنت آقای پرداخت", Key: "merchant_id_aqayepardakht"},
		{Label: "مرچنت FloyPay", Key: "marchent_floypay"},
		{Label: "مرچنت TronSeller", Key: "marchent_tronseller"},
		{Label: "کش‌بک کارت", Key: "chashbackcart"},
		{Label: "تایید خودکار رسید", Key: "autoconfirmcart"},
	}
	lines := []string{"💎 <b>تنظیمات مالی/درگاه‌ها</b>"}
	for _, item := range items {
		value, _ := b.repos.Setting.GetPaySetting(item.Key)
		lines = append(lines, fmt.Sprintf("• %s\n<code>%s</code>", item.Label, emptyDash(value)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func (b *Bot) adminSupportManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("👤 تنظیم آیدی پشتیبانی"), menu.Text("📝 متن دکمه ☎️ پشتیبانی")),
		menu.Row(menu.Text("➕ افزودن دپارتمان پشتیبانی"), menu.Text("📋 لیست دپارتمان‌ها")),
		menu.Row(menu.Text("🗑 حذف دپارتمان پشتیبانی"), menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminSupportManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("🤙 مدیریت پشتیبانی", b.adminSupportManageKeyboard())
}

func (b *Bot) sendAdminSupportDepartmentsList(c tele.Context, removeMode bool) error {
	items, err := b.repos.Setting.GetAllDepartments()
	if err != nil {
		return c.Send("❌ خطا در دریافت دپارتمان‌ها.")
	}
	if len(items) == 0 {
		return c.Send("دپارتمانی ثبت نشده است.", b.adminSupportManageKeyboard())
	}

	if !removeMode {
		lines := []string{"📋 <b>لیست دپارتمان‌های پشتیبانی</b>"}
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("ID: <code>%d</code>\n🏷 %s\n👤 پشتیبان: <code>%s</code>", item.ID, emptyDash(item.NameDepartman), emptyDash(item.IDSupport)))
		}
		return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
	}

	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(items)+1)
	for _, item := range items {
		title := fmt.Sprintf("❌ %d | %s", item.ID, item.NameDepartman)
		rows = append(rows, menu.Row(menu.Data(title, fmt.Sprintf("admin_support_dept_del_%d", item.ID))))
	}
	rows = append(rows, menu.Row(menu.Data("🔙 مدیریت پشتیبانی", "admin_support_manage")))
	menu.Inline(rows...)
	return c.Send("کدام دپارتمان حذف شود؟", menu)
}

func (b *Bot) handleAdminSupportCallback(c tele.Context, adminUser *models.User, data string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
	}

	switch {
	case data == "admin_support_manage":
		return b.sendAdminSupportManageMenu(c, adminID)
	case strings.HasPrefix(data, "admin_support_dept_del_"):
		id := parseIntSafe(strings.TrimPrefix(data, "admin_support_dept_del_"))
		if id <= 0 {
			return c.Send("شناسه دپارتمان نامعتبر است.")
		}
		if err := b.repos.Setting.DeleteDepartment(id); err != nil {
			return c.Send("❌ حذف دپارتمان ناموفق بود.")
		}
		return c.Send("✅ دپارتمان حذف شد.", b.adminSupportManageKeyboard())
	}
	return c.Send("عملیات پشتیبانی نامعتبر است.")
}

func (b *Bot) handleAdminSupportIDInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
	}
	supportID := strings.TrimSpace(text)
	if supportID == "" {
		return c.Send("آیدی پشتیبانی نمی‌تواند خالی باشد.")
	}
	if err := b.repos.Setting.UpdateSetting("id_support", supportID); err != nil {
		return c.Send("❌ ذخیره آیدی پشتیبانی ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ آیدی پشتیبانی ذخیره شد.", b.adminSupportManageKeyboard())
}

func (b *Bot) handleAdminSupportTextInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
	}
	label := strings.TrimSpace(text)
	if label == "" {
		return c.Send("متن دکمه پشتیبانی نمی‌تواند خالی باشد.")
	}
	if err := b.repos.Setting.SetText("text_support", label); err != nil {
		return c.Send("❌ ذخیره متن دکمه پشتیبانی ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ متن دکمه پشتیبانی ذخیره شد.", b.adminSupportManageKeyboard())
}

func (b *Bot) handleAdminSupportDeptAddNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
	}
	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام دپارتمان نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["support_dept_name"] = name
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_support_dept_add_id")
	return c.Send("آیدی عددی پشتیبان این دپارتمان را ارسال کنید.")
}

func (b *Bot) handleAdminSupportDeptAddIDInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند بخش پشتیبانی را مدیریت کند.")
	}

	supportID := strings.TrimSpace(text)
	if supportID == "" {
		return c.Send("آیدی پشتیبان نمی‌تواند خالی باشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	name := strings.TrimSpace(state["support_dept_name"])
	if name == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("نام دپارتمان مشخص نیست. دوباره تلاش کنید.")
	}

	item := &models.Departman{
		IDSupport:     supportID,
		NameDepartman: name,
	}
	if err := b.repos.Setting.CreateDepartment(item); err != nil {
		return c.Send("❌ ثبت دپارتمان ناموفق بود.")
	}

	delete(state, "support_dept_name")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ دپارتمان پشتیبانی ثبت شد.", b.adminSupportManageKeyboard())
}

func (b *Bot) adminShopManageKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("🛍 اضافه کردن محصول"), menu.Text("📋 لیست محصولات")),
		menu.Row(menu.Text("✏️ ویرایش محصول"), menu.Text("❌ حذف محصول")),
		menu.Row(menu.Text("➕ دسته بندی"), menu.Text("📋 لیست دسته‌بندی")),
		menu.Row(menu.Text("🗑 حذف دسته بندی")),
		menu.Row(menu.Text("🎁 ساخت کد هدیه"), menu.Text("❌ حذف کد هدیه")),
		menu.Row(menu.Text("🎁 ساخت کد تخفیف"), menu.Text("❌ حذف کد تخفیف")),
		menu.Row(menu.Text("📋 لیست کدهای هدیه"), menu.Text("📋 لیست کدهای تخفیف")),
		menu.Row(menu.Text("🔙 بازگشت به پنل مدیریت")),
	)
	return menu
}

func (b *Bot) sendAdminShopManageMenu(c tele.Context, chatID string) error {
	ok, role := b.isAdminWithRole(chatID)
	if !ok || !strings.EqualFold(role, "administrator") {
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	_ = b.repos.User.UpdateStep(chatID, "none")
	return c.Send("🏬 مدیریت فروشگاه", b.adminShopManageKeyboard())
}

func (b *Bot) adminShopProductEditKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text("نام محصول"), menu.Text("قیمت")),
		menu.Row(menu.Text("حجم"), menu.Text("زمان")),
		menu.Row(menu.Text("یادداشت"), menu.Text("دسته بندی")),
		menu.Row(menu.Text("نوع کاربری"), menu.Text("موقعیت محصول")),
		menu.Row(menu.Text("نوع ریست حجم")),
		menu.Row(menu.Text("🔙 بازگشت به فروشگاه")),
	)
	return menu
}

func (b *Bot) sendAdminShopProductList(c tele.Context) error {
	products, _, err := b.repos.Product.FindAll(300, 1, "")
	if err != nil {
		return c.Send("❌ خطا در دریافت لیست محصولات.")
	}
	if len(products) == 0 {
		return c.Send("محصولی ثبت نشده است.", b.adminShopManageKeyboard())
	}
	lines := []string{"📦 <b>لیست محصولات</b>"}
	for _, p := range products {
		lines = append(lines, fmt.Sprintf("ID: <code>%d</code>\n🏷 %s\n💰 %s\n📍 %s | 👥 %s | 📂 %s", p.ID, emptyDash(p.NameProduct), emptyDash(p.PriceProduct), emptyDash(p.Location), emptyDash(p.Agent), emptyDash(p.Category)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func (b *Bot) sendAdminShopCategoryList(c tele.Context) error {
	items, _, err := b.repos.Setting.FindAllCategories(300, 1, "")
	if err != nil {
		return c.Send("❌ خطا در دریافت لیست دسته‌بندی.")
	}
	if len(items) == 0 {
		return c.Send("دسته‌بندی ثبت نشده است.", b.adminShopManageKeyboard())
	}
	lines := []string{"📂 <b>لیست دسته‌بندی‌ها</b>"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("ID: <code>%d</code> | %s", item.ID, emptyDash(item.Remark)))
	}
	return c.Send(strings.Join(lines, "\n"), tele.ModeHTML)
}

func (b *Bot) sendAdminShopGiftCodeList(c tele.Context) error {
	items, _, err := b.repos.Setting.FindAllDiscounts(300, 1, "")
	if err != nil {
		return c.Send("❌ خطا در دریافت کدهای هدیه.")
	}
	if len(items) == 0 {
		return c.Send("کد هدیه‌ای ثبت نشده است.", b.adminShopManageKeyboard())
	}
	lines := []string{"🎁 <b>لیست کدهای هدیه</b>"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("🎫 <code>%s</code>\n💰 مبلغ: %s\n📌 مصرف: %s / %s", emptyDash(item.Code), emptyDash(item.Price), emptyDash(item.LimitUsed), emptyDash(item.LimitUse)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func (b *Bot) sendAdminShopDiscountCodeList(c tele.Context) error {
	items, _, err := b.repos.Setting.FindAllDiscountSells(300, 1, "")
	if err != nil {
		return c.Send("❌ خطا در دریافت کدهای تخفیف.")
	}
	if len(items) == 0 {
		return c.Send("کد تخفیفی ثبت نشده است.", b.adminShopManageKeyboard())
	}
	lines := []string{"🏷 <b>لیست کدهای تخفیف</b>"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("🎫 <code>%s</code>\n📉 مقدار: %s\n📌 سقف مصرف: %s", emptyDash(item.CodeDiscount), emptyDash(item.Price), emptyDash(item.LimitDiscount)))
	}
	return c.Send(strings.Join(lines, "\n\n"), tele.ModeHTML)
}

func listPanelNamesForAdmin(panels []models.Panel) []string {
	out := make([]string, 0, len(panels))
	for _, panel := range panels {
		name := strings.TrimSpace(panel.NamePanel)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func containsStringValue(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func normalizeProductAgentValue(raw string) (string, bool) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "all", "0":
		return "0", true
	case "f", "n", "n2":
		return value, true
	default:
		return "", false
	}
}

func (b *Bot) handleAdminShopProductAddNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام محصول نمی‌تواند خالی باشد.")
	}
	if len(name) > 150 {
		return c.Send("نام محصول باید کمتر از 150 کاراکتر باشد.")
	}
	var count int64
	_ = b.repos.Setting.DB().Model(&models.Product{}).Where("name_product = ?", name).Count(&count).Error
	if count > 0 {
		return c.Send("محصولی با این نام از قبل وجود دارد.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_name"] = name
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_agent")
	return c.Send("نوع کاربری محصول را ارسال کنید: <code>f</code> یا <code>n</code> یا <code>n2</code> یا <code>all</code>", tele.ModeHTML)
}

func (b *Bot) handleAdminShopProductAddAgentInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	agent, valid := normalizeProductAgentValue(text)
	if !valid {
		return c.Send("نوع کاربری نامعتبر است. مقادیر مجاز: f / n / n2 / all")
	}

	panels, _, err := b.repos.Panel.FindAll(500, 1, "")
	if err != nil || len(panels) == 0 {
		return c.Send("❌ لیست پنل‌ها در دسترس نیست.")
	}
	panelNames := listPanelNamesForAdmin(panels)

	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_agent"] = agent
	state["product_panel_names"] = strings.Join(panelNames, "\n")
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_location")
	return c.Send("موقعیت محصول را ارسال کنید (نام پنل یا <code>/all</code>):\n"+strings.Join(panelNames, "\n"), tele.ModeHTML)
}

func (b *Bot) handleAdminShopProductAddLocationInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	location := strings.TrimSpace(text)
	if location == "" {
		return c.Send("موقعیت محصول نمی‌تواند خالی باشد.")
	}

	if location != "/all" {
		panels, _, _ := b.repos.Panel.FindAll(500, 1, "")
		panelNames := listPanelNamesForAdmin(panels)
		if !containsStringValue(panelNames, location) {
			return c.Send("موقعیت پنل نامعتبر است. نام پنل معتبر یا /all ارسال کنید.")
		}
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_location"] = location
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_category")
	return c.Send("نام دسته‌بندی محصول را ارسال کنید (یا <code>none</code>):", tele.ModeHTML)
}

func (b *Bot) handleAdminShopProductAddCategoryInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}

	category := strings.TrimSpace(text)
	if strings.EqualFold(category, "none") {
		category = ""
	}
	if category != "" {
		var count int64
		_ = b.repos.Setting.DB().Model(&models.Category{}).Where("remark = ?", category).Count(&count).Error
		if count == 0 {
			return c.Send("دسته‌بندی وجود ندارد. ابتدا آن را اضافه کنید یا مقدار none بفرستید.")
		}
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_category"] = category
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_volume")
	return c.Send("حجم محصول (گیگ) را ارسال کنید.")
}

func (b *Bot) handleAdminShopProductAddVolumeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	volume := parseIntSafe(strings.TrimSpace(text))
	if volume < 0 {
		return c.Send("حجم نامعتبر است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_volume"] = strconv.Itoa(volume)
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_time")
	return c.Send("زمان محصول (روز) را ارسال کنید.")
}

func (b *Bot) handleAdminShopProductAddTimeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	days := parseIntSafe(strings.TrimSpace(text))
	if days < 0 {
		return c.Send("زمان نامعتبر است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_time"] = strconv.Itoa(days)
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_price")
	return c.Send("قیمت محصول (تومان) را ارسال کنید.")
}

func (b *Bot) handleAdminShopProductAddPriceInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	price := parseIntSafe(strings.TrimSpace(text))
	if price < 0 {
		return c.Send("قیمت نامعتبر است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_price"] = strconv.Itoa(price)
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_reset")
	return c.Send("نوع ریست حجم را ارسال کنید (مثال: no_reset / month).")
}

func (b *Bot) handleAdminShopProductAddResetInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	resetType := strings.TrimSpace(text)
	if resetType == "" {
		resetType = "no_reset"
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["product_reset"] = resetType
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_add_note")
	return c.Send("یادداشت محصول را ارسال کنید (یا <code>none</code>).", tele.ModeHTML)
}

func (b *Bot) handleAdminShopProductAddNoteInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	name := strings.TrimSpace(state["product_name"])
	agent := strings.TrimSpace(state["product_agent"])
	location := strings.TrimSpace(state["product_location"])
	category := strings.TrimSpace(state["product_category"])
	volume := strings.TrimSpace(state["product_volume"])
	serviceTime := strings.TrimSpace(state["product_time"])
	price := strings.TrimSpace(state["product_price"])
	resetType := strings.TrimSpace(state["product_reset"])
	note := strings.TrimSpace(text)
	if strings.EqualFold(note, "none") {
		note = ""
	}

	if name == "" || agent == "" || location == "" || volume == "" || serviceTime == "" || price == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		b.clearAdminState(adminID)
		return c.Send("اطلاعات محصول ناقص است. دوباره از ابتدا شروع کنید.", b.adminShopManageKeyboard())
	}

	product := &models.Product{
		CodeProduct:      strings.ToLower(utils.RandomHex(2)),
		NameProduct:      name,
		PriceProduct:     price,
		VolumeConstraint: volume,
		Location:         location,
		ServiceTime:      serviceTime,
		Agent:            agent,
		Note:             note,
		DataLimitReset:   defaultIfEmpty(resetType, "no_reset"),
		OneBuyStatus:     "0",
		Inbounds:         "",
		Proxies:          "",
		Category:         category,
		HidePanel:        "{}",
	}
	if err := b.repos.Product.Create(product); err != nil {
		return c.Send("❌ ثبت محصول ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	b.clearAdminState(adminID)
	return c.Send(fmt.Sprintf("✅ محصول ثبت شد.\nID: %d\nکد: %s", product.ID, product.CodeProduct), b.adminShopManageKeyboard())
}

func (b *Bot) handleAdminShopProductDeleteIDInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	id := parseIntSafe(strings.TrimSpace(text))
	if id <= 0 {
		return c.Send("شناسه محصول نامعتبر است.")
	}
	if err := b.repos.Product.Delete(id); err != nil {
		return c.Send("❌ حذف محصول ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ محصول حذف شد.", b.adminShopManageKeyboard())
}

func (b *Bot) handleAdminShopProductEditIDInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	id := parseIntSafe(strings.TrimSpace(text))
	if id <= 0 {
		return c.Send("شناسه محصول نامعتبر است.")
	}
	product, err := b.repos.Product.FindByID(id)
	if err != nil || product == nil {
		return c.Send("محصول یافت نشد.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["shop_product_edit_id"] = strconv.Itoa(id)
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_edit_field")
	return c.Send(
		fmt.Sprintf(
			"📌 محصول انتخاب شد:\nID: <code>%d</code>\nنام: %s\nقیمت: %s\nحجم: %s\nزمان: %s",
			product.ID,
			emptyDash(product.NameProduct),
			emptyDash(product.PriceProduct),
			emptyDash(product.VolumeConstraint),
			emptyDash(product.ServiceTime),
		),
		b.adminShopProductEditKeyboard(),
		tele.ModeHTML,
	)
}

func (b *Bot) handleAdminShopProductEditFieldInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}

	fieldText := strings.TrimSpace(text)
	if fieldText == "🔙 بازگشت به فروشگاه" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return b.sendAdminShopManageMenu(c, adminID)
	}

	columnMap := map[string]string{
		"نام محصول":    "name_product",
		"قیمت":         "price_product",
		"حجم":          "Volume_constraint",
		"زمان":         "Service_time",
		"یادداشت":      "note",
		"دسته بندی":    "category",
		"نوع کاربری":   "agent",
		"موقعیت محصول": "Location",
		"نوع ریست حجم": "data_limit_reset",
	}
	column, okCol := columnMap[fieldText]
	if !okCol {
		return c.Send("گزینه نامعتبر است.", b.adminShopProductEditKeyboard())
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	if strings.TrimSpace(state["shop_product_edit_id"]) == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("شناسه محصول مشخص نیست. دوباره عملیات را شروع کنید.")
	}
	state["shop_product_edit_column"] = column
	state["shop_product_edit_label"] = fieldText
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_edit_value")
	return c.Send("مقدار جدید را ارسال کنید.")
}

func (b *Bot) handleAdminShopProductEditValueInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}

	state := decodeAdminState(adminUser.ProcessingValue)
	productID := parseIntSafe(state["shop_product_edit_id"])
	column := strings.TrimSpace(state["shop_product_edit_column"])
	if productID <= 0 || column == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("اطلاعات ویرایش ناقص است. دوباره تلاش کنید.")
	}

	value := strings.TrimSpace(text)
	if value == "" {
		return c.Send("مقدار نمی‌تواند خالی باشد.")
	}

	switch column {
	case "price_product", "Volume_constraint", "Service_time":
		n := parseIntSafe(value)
		if n < 0 {
			return c.Send("مقدار عددی نامعتبر است.")
		}
		value = strconv.Itoa(n)
	case "agent":
		nAgent, valid := normalizeProductAgentValue(value)
		if !valid {
			return c.Send("نوع کاربری نامعتبر است. مقادیر مجاز: f / n / n2 / all")
		}
		value = nAgent
	case "Location":
		if value != "/all" {
			panels, _, _ := b.repos.Panel.FindAll(500, 1, "")
			panelNames := listPanelNamesForAdmin(panels)
			if !containsStringValue(panelNames, value) {
				return c.Send("موقعیت پنل نامعتبر است. نام پنل معتبر یا /all ارسال کنید.")
			}
		}
	}

	if err := b.repos.Product.Update(productID, map[string]interface{}{column: value}); err != nil {
		return c.Send("❌ بروزرسانی محصول ناموفق بود.")
	}

	_ = b.repos.User.UpdateStep(adminID, "admin_shop_product_edit_field")
	delete(state, "shop_product_edit_column")
	delete(state, "shop_product_edit_label")
	b.saveAdminState(adminID, state)
	return c.Send("✅ محصول بروزرسانی شد. برای ادامه، فیلد دیگری انتخاب کنید.", b.adminShopProductEditKeyboard())
}

func (b *Bot) handleAdminShopCategoryAddNameInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	name := strings.TrimSpace(text)
	if name == "" {
		return c.Send("نام دسته‌بندی نمی‌تواند خالی باشد.")
	}
	if err := b.repos.Setting.CreateCategory(&models.Category{Remark: name}); err != nil {
		return c.Send("❌ ثبت دسته‌بندی ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ دسته‌بندی ثبت شد.", b.adminShopManageKeyboard())
}

func (b *Bot) handleAdminShopCategoryDeleteIDInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	id := parseIntSafe(strings.TrimSpace(text))
	if id <= 0 {
		return c.Send("شناسه دسته‌بندی نامعتبر است.")
	}
	if err := b.repos.Setting.DeleteCategory(id); err != nil {
		return c.Send("❌ حذف دسته‌بندی ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ دسته‌بندی حذف شد.", b.adminShopManageKeyboard())
}

func isAlphaNumericOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			continue
		}
		return false
	}
	return true
}

func (b *Bot) handleAdminShopGiftAddCodeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	code := strings.TrimSpace(text)
	if !isAlphaNumericOnly(code) {
		return c.Send("کد نامعتبر است. فقط حروف و اعداد مجاز است.")
	}
	if _, err := b.repos.Setting.FindDiscountByCode(code); err == nil {
		return c.Send("این کد هدیه قبلاً ثبت شده است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["gift_code"] = code
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_gift_add_amount")
	return c.Send("مبلغ هدیه (تومان) را ارسال کنید.")
}

func (b *Bot) handleAdminShopGiftAddAmountInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	amount := parseIntSafe(strings.TrimSpace(text))
	if amount <= 0 {
		return c.Send("مبلغ نامعتبر است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["gift_amount"] = strconv.Itoa(amount)
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_gift_add_limit")
	return c.Send("حداکثر تعداد مصرف را ارسال کنید (0 یعنی نامحدود).")
}

func (b *Bot) handleAdminShopGiftAddLimitInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	limit := parseIntSafe(strings.TrimSpace(text))
	if limit < 0 {
		return c.Send("حد مصرف نامعتبر است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	code := strings.TrimSpace(state["gift_code"])
	amount := strings.TrimSpace(state["gift_amount"])
	if code == "" || amount == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		b.clearAdminState(adminID)
		return c.Send("اطلاعات کد هدیه ناقص است. دوباره تلاش کنید.")
	}
	item := &models.Discount{
		Code:      code,
		Price:     amount,
		LimitUse:  strconv.Itoa(limit),
		LimitUsed: "0",
	}
	if err := b.repos.Setting.CreateDiscount(item); err != nil {
		return c.Send("❌ ثبت کد هدیه ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	b.clearAdminState(adminID)
	return c.Send("✅ کد هدیه ثبت شد.", b.adminShopManageKeyboard())
}

func (b *Bot) handleAdminShopGiftDeleteCodeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	code := strings.TrimSpace(text)
	if code == "" {
		return c.Send("کد نمی‌تواند خالی باشد.")
	}
	if err := b.repos.Setting.DeleteDiscountByCode(code); err != nil {
		return c.Send("❌ حذف کد هدیه ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ کد هدیه حذف شد.", b.adminShopManageKeyboard())
}

func (b *Bot) handleAdminShopDiscountAddCodeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	code := strings.TrimSpace(text)
	if !isAlphaNumericOnly(code) {
		return c.Send("کد نامعتبر است. فقط حروف و اعداد مجاز است.")
	}
	if _, err := b.repos.Setting.FindDiscountSellByCode(code); err == nil {
		return c.Send("این کد تخفیف قبلاً ثبت شده است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["discount_code"] = code
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_discount_add_percent")
	return c.Send("درصد/مقدار تخفیف را ارسال کنید.")
}

func (b *Bot) handleAdminShopDiscountAddPercentInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	discountValue := parseIntSafe(strings.TrimSpace(text))
	if discountValue <= 0 {
		return c.Send("مقدار تخفیف نامعتبر است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	state["discount_value"] = strconv.Itoa(discountValue)
	b.saveAdminState(adminID, state)
	_ = b.repos.User.UpdateStep(adminID, "admin_shop_discount_add_limit")
	return c.Send("حداکثر مصرف کد تخفیف را ارسال کنید (0 یعنی نامحدود).")
}

func (b *Bot) handleAdminShopDiscountAddLimitInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	limit := parseIntSafe(strings.TrimSpace(text))
	if limit < 0 {
		return c.Send("حد مصرف نامعتبر است.")
	}
	state := decodeAdminState(adminUser.ProcessingValue)
	code := strings.TrimSpace(state["discount_code"])
	discountValue := strings.TrimSpace(state["discount_value"])
	if code == "" || discountValue == "" {
		_ = b.repos.User.UpdateStep(adminID, "none")
		b.clearAdminState(adminID)
		return c.Send("اطلاعات کد تخفیف ناقص است. دوباره تلاش کنید.")
	}

	item := &models.DiscountSell{
		CodeDiscount:  code,
		Price:         discountValue,
		LimitDiscount: strconv.Itoa(limit),
		Agent:         "all",
		UseFirst:      "0",
		UseUser:       "0",
		CodeProduct:   "all",
		CodePanel:     "all",
		Time:          fmt.Sprintf("%d", time.Now().Unix()),
		Type:          "percent",
		UsedDiscount:  "0",
	}
	if err := b.repos.Setting.CreateDiscountSell(item); err != nil {
		return c.Send("❌ ثبت کد تخفیف ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	b.clearAdminState(adminID)
	return c.Send("✅ کد تخفیف ثبت شد.", b.adminShopManageKeyboard())
}

func (b *Bot) handleAdminShopDiscountDeleteCodeInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند فروشگاه را مدیریت کند.")
	}
	code := strings.TrimSpace(text)
	if code == "" {
		return c.Send("کد نمی‌تواند خالی باشد.")
	}
	if err := b.repos.Setting.DeleteDiscountSellByCode(code); err != nil {
		return c.Send("❌ حذف کد تخفیف ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	return c.Send("✅ کد تخفیف حذف شد.", b.adminShopManageKeyboard())
}

func (b *Bot) handleAdminSetReportChannelInput(c tele.Context, adminUser *models.User, text string) error {
	adminID := adminUser.ID
	ok, role := b.isAdminWithRole(adminID)
	if !ok || !strings.EqualFold(role, "administrator") {
		_ = b.repos.User.UpdateStep(adminID, "none")
		return c.Send("⛔ فقط مدیر اصلی می‌تواند تنظیمات گزارش را مدیریت کند.")
	}
	channel := strings.TrimSpace(text)
	if channel == "" {
		return c.Send("شناسه/یوزرنیم کانال نمی‌تواند خالی باشد.")
	}

	_, err := b.botAPI.SendMessage(channel, "✅ تست کانال گزارش هوسی‌بات", nil)
	if err != nil {
		return c.Send("❌ ارسال پیام تست به کانال ناموفق بود. دسترسی ربات را بررسی کنید.")
	}

	createdTopics, topicWarnings := b.ensureAdminReportTopics(channel)

	if err := b.repos.Setting.UpdateSetting("Channel_Report", channel); err != nil {
		return c.Send("❌ ذخیره تنظیم کانال گزارش ناموفق بود.")
	}
	_ = b.repos.User.UpdateStep(adminID, "none")
	msg := "✅ کانال گزارش با موفقیت ثبت شد."
	if createdTopics > 0 {
		msg += fmt.Sprintf("\n📌 تعداد تاپیک‌های گزارش ایجاد/بروزرسانی‌شده: %d", createdTopics)
	}
	if len(topicWarnings) > 0 {
		msg += "\n⚠️ هشدار تاپیک‌ها:\n- " + strings.Join(topicWarnings, "\n- ")
	}
	return c.Send(msg, b.adminMenuKeyboard("administrator"))
}

func (b *Bot) ensureAdminReportTopics(channel string) (int, []string) {
	if b.repos == nil || b.repos.Setting == nil {
		return 0, []string{"repository unavailable"}
	}

	topicDefs := []struct {
		Name   string
		Report string
	}{
		{Name: "🤖 بکاپ ربات نماینده", Report: "backupfile"},
		{Name: "📝 گزارش اطلاع رسانی ها", Report: "reportcron"},
		{Name: "🌙 گزارش شبانه", Report: "reportnight"},
		{Name: "🎁 گزارش پورسانت ها", Report: "porsantreport"},
		{Name: "🛍 گزارش های خرید", Report: "reportbuy"},
		{Name: "📌 گزارش خرید خدمات", Report: "invoicepay"},
		{Name: "🔑 گزارش اکانت تست", Report: "reporttest"},
		{Name: "⚙️ سایر گزارشات", Report: "otherreport"},
		{Name: "❌ گزارش خطا ها", Report: "errorreport"},
		{Name: "💰 گزارش مالی", Report: "paymentreport"},
	}

	created := 0
	warnings := make([]string, 0)
	db := b.repos.Setting.DB()

	for _, def := range topicDefs {
		raw, err := b.botAPI.Call("createForumTopic", map[string]interface{}{
			"chat_id": channel,
			"name":    def.Name,
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", def.Name, err))
			continue
		}

		var payload struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
			Result      struct {
				MessageThreadID int `json:"message_thread_id"`
			} `json:"result"`
		}
		if jsonErr := json.Unmarshal([]byte(raw), &payload); jsonErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: invalid response", def.Name))
			continue
		}
		if !payload.OK || payload.Result.MessageThreadID <= 0 {
			desc := strings.TrimSpace(payload.Description)
			if desc == "" {
				desc = "createForumTopic failed"
			}
			warnings = append(warnings, fmt.Sprintf("%s: %s", def.Name, desc))
			continue
		}

		if err := db.Save(&models.TopicID{
			Report:   def.Report,
			IDReport: strconv.Itoa(payload.Result.MessageThreadID),
		}).Error; err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: db save failed", def.Name))
			continue
		}
		created++
	}

	return created, warnings
}

func (b *Bot) handleHideMiniAppInstruction(c tele.Context, user *models.User) error {
	if user == nil {
		return nil
	}
	if err := b.repos.User.Update(user.ID, map[string]interface{}{"hide_mini_app_instruction": "1"}); err != nil {
		return c.Send("❌ خطا در ذخیره تنظیم.")
	}
	return c.Send("✅ پیام آموزش مینی‌اپ دیگر نمایش داده نمی‌شود.")
}

func (b *Bot) adminPanelWelcomeText() string {
	botVer := readOptionalVersionFile("version")
	if botVer == "" {
		botVer = "unknown"
	}
	appVer := readOptionalVersionFile("app/version")
	if appVer == "" {
		appVer = "unknown"
	}
	return fmt.Sprintf("💎 نسخه Bot: <code>%s</code>\n📌 نسخه Mini App: <code>%s</code>", botVer, appVer)
}

func (b *Bot) adminMiniAppInstructionText() string {
	base := strings.TrimRight(strings.TrimSpace(b.cfg.Bot.Domain), "/")
	if base == "" {
		base = "https://your-domain.example"
	}
	return "📌 <b>آموزش فعالسازی مینی‌اپ در BotFather</b>\n" +
		"1) /mybots -> انتخاب ربات\n" +
		"2) Bot Settings -> Configure Mini App\n" +
		"3) Mini App URL را روی آدرس زیر قرار دهید:\n" +
		fmt.Sprintf("<code>%s/app/</code>", base)
}

func readOptionalVersionFile(path string) string {
	buf, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(buf))
}

func getSettingColumnValue(s *models.Setting, column string) string {
	if s == nil {
		return ""
	}
	switch column {
	case "Bot_Status":
		return s.BotStatus
	case "get_number":
		return s.GetNumber
	case "statuscategory":
		return s.StatusCategory
	case "statusagentrequest":
		return s.StatusAgentRequest
	case "statusnewuser":
		return s.StatusNewUser
	case "roll_Status":
		return s.RollStatus
	case "iran_number":
		return s.IranNumber
	case "verifystart":
		return s.VerifyStart
	case "statussupportpv":
		return s.StatusSupportPV
	case "statusnamecustom":
		return s.StatusNameCustom
	case "bulkbuy":
		return s.BulkBuy
	case "affiliatesstatus":
		return s.AffiliatesStatus
	case "inlinebtnmain":
		return s.InlineBtnMain
	case "linkappstatus":
		return s.LinkAppStatus
	case "btn_status_extned":
		return s.BtnStatusExtend
	case "scorestatus":
		return s.ScoreStatus
	case "verifybucodeuser":
		return s.VerifyBuCodeUser
	default:
		return ""
	}
}

func decodeAdminState(raw string) map[string]string {
	state := map[string]string{}
	payload := strings.TrimSpace(raw)
	if payload == "" {
		return state
	}
	_ = json.Unmarshal([]byte(payload), &state)
	return state
}

func (b *Bot) saveAdminState(chatID string, state map[string]string) {
	if len(state) == 0 {
		_ = b.repos.User.Update(chatID, map[string]interface{}{"Processing_value": ""})
		return
	}
	buf, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = b.repos.User.Update(chatID, map[string]interface{}{"Processing_value": string(buf)})
}

func (b *Bot) clearAdminState(chatID string) {
	_ = b.repos.User.Update(chatID, map[string]interface{}{"Processing_value": ""})
}

func (b *Bot) setAdminStateValue(chatID, key, value string) {
	user, err := b.repos.User.FindByID(chatID)
	if err != nil || user == nil {
		return
	}
	state := decodeAdminState(user.ProcessingValue)
	state[key] = value
	b.saveAdminState(chatID, state)
}

func normalizePanelType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "marzban":
		return "marzban"
	case "pasarguard":
		return "pasarguard"
	case "hiddify":
		return "hiddify"
	case "marzneshin":
		return "marzneshin"
	case "xui", "x-ui_single", "x-ui":
		return "x-ui_single"
	case "alireza", "alireza_single":
		return "alireza_single"
	case "s_ui", "s-ui", "sui":
		return "s_ui"
	case "wgdashboard", "wgdashboard_panel":
		return "wgdashboard"
	case "ibsng":
		return "ibsng"
	case "mikrotik":
		return "mikrotik"
	default:
		return ""
	}
}

func panelTypeLabel(panelType string) string {
	switch normalizePanelType(panelType) {
	case "marzban":
		return "Marzban"
	case "pasarguard":
		return "PasarGuard"
	case "hiddify":
		return "Hiddify"
	case "marzneshin":
		return "Marzneshin"
	case "x-ui_single":
		return "X-UI"
	case "alireza_single":
		return "Alireza"
	case "s_ui":
		return "S-UI"
	case "wgdashboard":
		return "WGDashboard"
	case "ibsng":
		return "IBSng"
	case "mikrotik":
		return "MikroTik"
	default:
		return panelType
	}
}

func panelStatusEmoji(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), "active") {
		return "🟢"
	}
	return "🔴"
}

func buildPanelWithDefaults(panelType, name, panelURL, username, password string, limit int) *models.Panel {
	priceDefault := `{"f":"4000","n":"4000","n2":"4000"}`
	rangeDefault := `{"f":"1","n":"1","n2":"1"}`
	maxDefault := `{"f":"1000","n":"1000","n2":"1000"}`
	customVolumeDefault := `{"f":"0","n":"0","n2":"0"}`

	return &models.Panel{
		CodePanel:         strings.ToLower(utils.RandomHex(2)),
		NamePanel:         strings.TrimSpace(name),
		SubLink:           "onsublink",
		Config:            "offconfig",
		MethodUsername:    "آیدی عددی + حروف و عدد رندوم",
		TestAccount:       "ONTestAccount",
		Status:            "active",
		LimitPanel:        strconv.Itoa(limit),
		NameCustom:        "none",
		MethodExtend:      "1",
		Type:              normalizePanelType(panelType),
		Connection:        "offconecton",
		InboundID:         "1",
		Agent:             "all",
		InboundDeactive:   "1",
		InboundStatus:     "offinbounddisable",
		URLPanel:          strings.TrimRight(strings.TrimSpace(panelURL), "/"),
		UsernamePanel:     defaultIfEmpty(strings.TrimSpace(username), "null"),
		PasswordPanel:     defaultIfEmpty(strings.TrimSpace(password), "null"),
		TimeUserTest:      "1",
		ValUserTest:       "100",
		LinkSubX:          strings.TrimRight(strings.TrimSpace(panelURL), "/"),
		PriceExtraVolume:  priceDefault,
		PriceExtraTime:    priceDefault,
		PriceCustomVolume: priceDefault,
		PriceCustomTime:   priceDefault,
		MainVolume:        rangeDefault,
		MaxVolume:         maxDefault,
		MainTime:          rangeDefault,
		MaxTime:           maxDefault,
		StatusExtend:      "on_extend",
		SubVIP:            sql.NullString{String: "offsubvip", Valid: true},
		ChangeLoc:         sql.NullString{String: "offchangeloc", Valid: true},
		CustomVolume:      customVolumeDefault,
		OnHoldTest:        sql.NullString{String: "1", Valid: true},
		VersionPanel:      sql.NullString{String: "0", Valid: true},
	}
}

func emptyDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func formatAgentExpire(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "نامشخص"
	}
	ts, err := strconv.ParseInt(v, 10, 64)
	if err != nil || ts <= 0 {
		return "بدون انقضا"
	}
	t := time.Unix(ts, 0)
	if t.Before(time.Now()) {
		return "منقضی شده"
	}
	return t.Format("2006-01-02 15:04")
}

func decodeAnyJSONToMap(raw string) map[string]interface{} {
	out := map[string]interface{}{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func clearStateByPrefix(state map[string]string, prefix string) {
	for k := range state {
		if strings.HasPrefix(k, prefix) {
			delete(state, k)
		}
	}
}

func parseTargetPanelMapping(raw string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(raw), "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	targetID := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if targetID == "" || value == "" {
		return "", "", false
	}
	return targetID, value, true
}

func parseStringArrayJSON(raw string) []string {
	result := make([]string, 0)
	if strings.TrimSpace(raw) == "" {
		return result
	}

	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		for _, item := range list {
			item = strings.TrimSpace(item)
			if item != "" && !containsString(result, item) {
				result = append(result, item)
			}
		}
		return result
	}

	var listAny []interface{}
	if err := json.Unmarshal([]byte(raw), &listAny); err == nil {
		for _, item := range listAny {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" && !containsString(result, s) {
					result = append(result, s)
				}
			}
		}
	}
	return result
}

func containsString(list []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), needle) {
			return true
		}
	}
	return false
}

func removeString(list []string, needle string) []string {
	out := make([]string, 0, len(list))
	needle = strings.TrimSpace(needle)
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), needle) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func isValidPanelUsername(username string) bool {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 33 {
		return false
	}
	if strings.HasSuffix(username, "_") {
		return false
	}
	ok, _ := regexp.MatchString(`^[A-Za-z][A-Za-z0-9_]{2,32}$`, username)
	return ok
}

func getTelegramBotUsernameByToken(token string) (string, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", strings.TrimSpace(token))
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK || strings.TrimSpace(result.Result.Username) == "" {
		return "", fmt.Errorf("invalid token")
	}
	return strings.TrimSpace(result.Result.Username), nil
}

func deleteTelegramWebhookByToken(token string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook", strings.TrimSpace(token))
	resp, err := http.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (b *Bot) setAgentBotWebhookLocal(chatID, username, token string) error {
	baseURL := strings.TrimSpace(os.Getenv("BOT_DOMAIN"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(b.cfg.Bot.Domain)
	}
	if baseURL == "" {
		return nil
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(strings.ToLower(baseURL), "http://") && !strings.HasPrefix(strings.ToLower(baseURL), "https://") {
		baseURL = "https://" + baseURL
	}

	webhookURL := fmt.Sprintf("%s/vpnbot/%s%s/index.php", baseURL, strings.TrimSpace(chatID), strings.TrimSpace(username))
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", strings.TrimSpace(token))
	resp, err := http.PostForm(apiURL, url.Values{"url": {webhookURL}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func createAgentBotFilesLocal(chatID, username, token string) error {
	baseDir, err := findVPNBotBaseDirLocal()
	if err != nil {
		return err
	}

	sourceDir := filepath.Join(baseDir, "Default")
	targetDir := filepath.Join(baseDir, chatID+username)

	if _, err := os.Stat(sourceDir); err != nil {
		return err
	}

	_ = os.RemoveAll(targetDir)
	if err := copyDirLocal(sourceDir, targetDir); err != nil {
		return err
	}

	configPath := filepath.Join(targetDir, "config.php")
	content, err := os.ReadFile(configPath)
	if err == nil {
		updated := strings.ReplaceAll(string(content), "BotTokenNew", token)
		if writeErr := os.WriteFile(configPath, []byte(updated), 0o644); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func deleteAgentBotFilesLocal(chatID, username string) error {
	baseDir, err := findVPNBotBaseDirLocal()
	if err != nil {
		return err
	}
	targetDir := filepath.Join(baseDir, chatID+username)
	return os.RemoveAll(targetDir)
}

func findVPNBotBaseDirLocal() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(cwd, "vpnbot"),
		filepath.Join(cwd, "..", "vpnbot"),
		filepath.Join(cwd, "..", "..", "vpnbot"),
	}
	for _, dir := range candidates {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir, nil
		}
	}
	return "", fmt.Errorf("vpnbot directory not found")
}

func copyDirLocal(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode())
	})
}
