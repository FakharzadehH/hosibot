package bootstrap

import (
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	"hosibot/internal/models"
)

// MigrateAndSeed ensures required tables exist and inserts baseline rows for singleton tables.
func MigrateAndSeed(db *gorm.DB) error {
	if err := db.AutoMigrate(allModels()...); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}
	if err := seedDefaults(db); err != nil {
		return fmt.Errorf("seed defaults failed: %w", err)
	}
	return nil
}

func allModels() []interface{} {
	return []interface{}{
		// Core entities
		&models.User{},
		&models.Product{},
		&models.Invoice{},
		&models.PaymentReport{},
		&models.Panel{},
		&models.ServicePanel{},
		// Settings / config / legacy support tables
		&models.Admin{},
		&models.Setting{},
		&models.TextBot{},
		&models.Channel{},
		&models.Discount{},
		&models.DiscountSell{},
		&models.CardNumber{},
		&models.TopicID{},
		&models.Category{},
		&models.ServiceOther{},
		&models.PaySetting{},
		&models.ShopSetting{},
		&models.LogsAPI{},
		&models.SupportMessage{},
		&models.GiftCodeConsumed{},
		&models.ManualSell{},
		&models.Help{},
		&models.BotSaz{},
		&models.RequestAgent{},
		&models.CancelService{},
		&models.Departman{},
		&models.WheelList{},
		&models.Affiliates{},
		&models.ReagentReport{},
		&models.App{},
		// Queue-backed cron
		&models.CronJob{},
		&models.CronJobItem{},
	}
}

func seedDefaults(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := ensureDefaultSetting(tx); err != nil {
			return err
		}
		if err := ensureDefaultPaySettings(tx); err != nil {
			return err
		}
		if err := ensureDefaultAffiliates(tx); err != nil {
			return err
		}
		return nil
	})
}

func ensureDefaultSetting(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&models.Setting{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaultKeyboard := `{"keyboard":[[{"text":"text_sell"},{"text":"text_extend"}],[{"text":"text_usertest"},{"text":"text_wheel_luck"}],[{"text":"text_Purchased_services"},{"text":"accountwallet"}],[{"text":"text_affiliates"},{"text":"text_Tariff_list"}],[{"text":"text_support"},{"text":"text_help"}]]}`
	defaultCronStatus := `{"notifications":"off","test":"off","uptime_panel":"off","payment":"off","lottery":"off","node_uptime":"off","on_hold":"off"}`

	row := models.Setting{
		BotStatus:             "on",
		RollStatus:            "off",
		GetNumber:             "off_number",
		IranNumber:            "",
		NotUser:               "0",
		ChannelReport:         "",
		LimitUserTestAll:      "0",
		AffiliatesStatus:      "offaffiliates",
		AffiliatesPercentage:  "0",
		RemoveDayC:            "0",
		ShowCard:              "0",
		NumberCount:           "0",
		StatusNewUser:         "on",
		StatusAgentRequest:    "on",
		StatusCategory:        "offcategory",
		StatusTerffh:          "offterffh",
		VolumeWarn:            "0",
		InlineBtnMain:         "off",
		VerifyStart:           "off",
		IDSupport:             "",
		StatusNameCustom:      "offnamecustom",
		StatusCategoryGeneral: "offcategorys",
		StatusSupportPV:       "0",
		AgentReqPrice:         "0",
		BulkBuy:               "0",
		OnHoldDay:             "0",
		CronVolumeRe:          "0",
		VerifyBuCodeUser:      "0",
		ScoreStatus:           "0",
		LotteryPrize:          "[]",
		WheelLuck:             "0",
		WheelLuckPrice:        "0",
		BtnStatusExtend:       "0",
		DayWarn:               "0",
		CategoryHelp:          "0",
		LinkAppStatus:         "0",
		IPLogin:               "0",
		WheelAgent:            "0",
		LotteryAgent:          "0",
		LanguageEN:            "0",
		LanguageRU:            "0",
		StatusFirstWheel:      "0",
		StatusLimitChangeLoc:  "0",
		DebtSettlement:        "0",
		Dice:                  "0",
		KeyboardMain:          defaultKeyboard,
		StatusNoteForF:        "0",
		StatusCopyCart:        "0",
		TimeAutoNotVerify:     "0",
		StatusKeyboardConfig:  "0",
		CronStatus:            defaultCronStatus,
		LimitNumber:           sql.NullString{String: "0", Valid: true},
	}
	return tx.Create(&row).Error
}

func ensureDefaultAffiliates(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&models.Affiliates{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	row := models.Affiliates{
		Description:      "",
		StatusCommission: "off",
		Discount:         "0",
		PriceDiscount:    "0",
		PorsantOneBuy:    "0",
		IDMedia:          "",
	}
	return tx.Create(&row).Error
}

func ensureDefaultPaySettings(tx *gorm.DB) error {
	defaults := map[string]string{
		"minamount":                 "10000",
		"maxamount":                 "500000000",
		"cardnum":                   "",
		"cardname":                  "",
		"merchant_zarinpal":         "",
		"merchant_id_aqayepardakht": "",
		"marchent_floypay":          "",
		"apiternado":                "",
		"apinowpayment":             "",
		"apikey_nowpayment":         "",
		"marchent_tronseller":       "",
		"walletaddress":             "",
		"urlpaymenttron":            "",
		"statuscardautoconfirm":     "offautoconfirm",
		"autoconfirmcart":           "offauto",
		"Exception_auto_cart":       "[]",
		"chashbackcart":             "0",
		"chashbackplisio":           "0",
	}

	for key, value := range defaults {
		var count int64
		if err := tx.Model(&models.PaySetting{}).Where("NamePay = ?", key).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		row := models.PaySetting{NamePay: key, ValuePay: value}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
