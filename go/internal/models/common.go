package models

import "database/sql"

// Admin maps to the `admin` table.
type Admin struct {
	IDAdmin  string `gorm:"column:id_admin;primaryKey;size:500" json:"id_admin"`
	Username string `gorm:"column:username;size:1000" json:"username"`
	Password string `gorm:"column:password;size:1000" json:"password"`
	Rule     string `gorm:"column:rule;size:500" json:"rule"`
}

func (Admin) TableName() string {
	return "admin"
}

// Setting maps to the `setting` table (single-row config table).
type Setting struct {
	BotStatus             string         `gorm:"column:Bot_Status;size:200" json:"bot_status"`
	RollStatus            string         `gorm:"column:roll_Status;size:200" json:"roll_status"`
	GetNumber             string         `gorm:"column:get_number;size:200" json:"get_number"`
	IranNumber            string         `gorm:"column:iran_number;size:200" json:"iran_number"`
	NotUser               string         `gorm:"column:NotUser;size:200" json:"not_user"`
	ChannelReport         string         `gorm:"column:Channel_Report;size:600" json:"channel_report"`
	LimitUserTestAll      string         `gorm:"column:limit_usertest_all;size:600" json:"limit_usertest_all"`
	AffiliatesStatus      string         `gorm:"column:affiliatesstatus;size:600" json:"affiliates_status"`
	AffiliatesPercentage  string         `gorm:"column:affiliatespercentage;size:600" json:"affiliates_percentage"`
	RemoveDayC            string         `gorm:"column:removedayc;size:600" json:"remove_day_c"`
	ShowCard              string         `gorm:"column:showcard;size:200" json:"show_card"`
	NumberCount           string         `gorm:"column:numbercount;size:600" json:"number_count"`
	StatusNewUser         string         `gorm:"column:statusnewuser;size:600" json:"status_new_user"`
	StatusAgentRequest    string         `gorm:"column:statusagentrequest;size:600" json:"status_agent_request"`
	StatusCategory        string         `gorm:"column:statuscategory;size:200" json:"status_category"`
	StatusTerffh          string         `gorm:"column:statusterffh;size:200" json:"status_terffh"`
	VolumeWarn            string         `gorm:"column:volumewarn;size:200" json:"volume_warn"`
	InlineBtnMain         string         `gorm:"column:inlinebtnmain;size:200" json:"inline_btn_main"`
	VerifyStart           string         `gorm:"column:verifystart;size:200" json:"verify_start"`
	IDSupport             string         `gorm:"column:id_support;size:200" json:"id_support"`
	StatusNameCustom      string         `gorm:"column:statusnamecustom;size:100" json:"status_name_custom"`
	StatusCategoryGeneral string         `gorm:"column:statuscategorygenral;size:100" json:"status_category_general"`
	StatusSupportPV       string         `gorm:"column:statussupportpv;size:100" json:"status_support_pv"`
	AgentReqPrice         string         `gorm:"column:agentreqprice;size:100" json:"agent_req_price"`
	BulkBuy               string         `gorm:"column:bulkbuy;size:100" json:"bulk_buy"`
	OnHoldDay             string         `gorm:"column:on_hold_day;size:100" json:"on_hold_day"`
	CronVolumeRe          string         `gorm:"column:cronvolumere;size:100" json:"cron_volume_re"`
	VerifyBuCodeUser      string         `gorm:"column:verifybucodeuser;size:100" json:"verify_bu_code_user"`
	ScoreStatus           string         `gorm:"column:scorestatus;size:100" json:"score_status"`
	LotteryPrize          string         `gorm:"column:Lottery_prize;type:text" json:"lottery_prize"`
	WheelLuck             string         `gorm:"column:wheelـluck;size:45" json:"wheel_luck"`
	WheelLuckPrice        string         `gorm:"column:wheelـluck_price;size:45" json:"wheel_luck_price"`
	BtnStatusExtend       string         `gorm:"column:btn_status_extned;size:45" json:"btn_status_extend"`
	DayWarn               string         `gorm:"column:daywarn;size:45" json:"day_warn"`
	CategoryHelp          string         `gorm:"column:categoryhelp;size:45" json:"category_help"`
	LinkAppStatus         string         `gorm:"column:linkappstatus;size:45" json:"link_app_status"`
	IPLogin               string         `gorm:"column:iplogin;size:45" json:"ip_login"`
	WheelAgent            string         `gorm:"column:wheelagent;size:45" json:"wheel_agent"`
	LotteryAgent          string         `gorm:"column:Lotteryagent;size:45" json:"lottery_agent"`
	LanguageEN            string         `gorm:"column:languageen;size:45" json:"language_en"`
	LanguageRU            string         `gorm:"column:languageru;size:45" json:"language_ru"`
	StatusFirstWheel      string         `gorm:"column:statusfirstwheel;size:45" json:"status_first_wheel"`
	StatusLimitChangeLoc  string         `gorm:"column:statuslimitchangeloc;size:45" json:"status_limit_change_loc"`
	DebtSettlement        string         `gorm:"column:Debtsettlement;size:45" json:"debt_settlement"`
	Dice                  string         `gorm:"column:Dice;size:45" json:"dice"`
	KeyboardMain          string         `gorm:"column:keyboardmain;type:text" json:"keyboard_main"`
	StatusNoteForF        string         `gorm:"column:statusnoteforf;size:45" json:"status_note_for_f"`
	StatusCopyCart        string         `gorm:"column:statuscopycart;size:45" json:"status_copy_cart"`
	TimeAutoNotVerify     string         `gorm:"column:timeauto_not_verify;size:20" json:"time_auto_not_verify"`
	StatusKeyboardConfig  string         `gorm:"column:status_keyboard_config;size:20" json:"status_keyboard_config"`
	CronStatus            string         `gorm:"column:cron_status;type:text" json:"cron_status"`
	LimitNumber           sql.NullString `gorm:"column:limitnumber;size:200" json:"limit_number"`
}

func (Setting) TableName() string {
	return "setting"
}

// TextBot maps to the `textbot` table.
type TextBot struct {
	IDText string `gorm:"column:id_text;primaryKey;size:600" json:"id_text"`
	Text   string `gorm:"column:text;type:text" json:"text"`
}

func (TextBot) TableName() string {
	return "textbot"
}

// Channel maps to the `channels` table.
type Channel struct {
	Remark   string `gorm:"column:remark;size:200" json:"remark"`
	LinkJoin string `gorm:"column:linkjoin;size:200" json:"linkjoin"`
	Link     string `gorm:"column:link;size:200" json:"link"`
}

func (Channel) TableName() string {
	return "channels"
}

// Discount maps to the `Discount` table.
type Discount struct {
	ID        uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Code      string `gorm:"column:code;size:2000" json:"code"`
	Price     string `gorm:"column:price;size:200" json:"price"`
	LimitUse  string `gorm:"column:limituse;size:200" json:"limit_use"`
	LimitUsed string `gorm:"column:limitused;size:200" json:"limit_used"`
}

func (Discount) TableName() string {
	return "Discount"
}

// DiscountSell maps to the `DiscountSell` table.
type DiscountSell struct {
	ID            uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CodeDiscount  string `gorm:"column:codeDiscount;size:1000" json:"code_discount"`
	Price         string `gorm:"column:price;size:200" json:"price"`
	LimitDiscount string `gorm:"column:limitDiscount;size:500" json:"limit_discount"`
	Agent         string `gorm:"column:agent;size:500" json:"agent"`
	UseFirst      string `gorm:"column:usefirst;size:100" json:"use_first"`
	UseUser       string `gorm:"column:useuser;size:100" json:"use_user"`
	CodeProduct   string `gorm:"column:code_product;size:100" json:"code_product"`
	CodePanel     string `gorm:"column:code_panel;size:100" json:"code_panel"`
	Time          string `gorm:"column:time;size:100" json:"time"`
	Type          string `gorm:"column:type;size:100" json:"type"`
	UsedDiscount  string `gorm:"column:usedDiscount;size:500" json:"used_discount"`
}

func (DiscountSell) TableName() string {
	return "DiscountSell"
}

// CardNumber maps to the `card_number` table.
type CardNumber struct {
	CardNumber string `gorm:"column:cardnumber;primaryKey;size:500" json:"cardnumber"`
	NameCard   string `gorm:"column:namecard;size:1000" json:"namecard"`
}

func (CardNumber) TableName() string {
	return "card_number"
}

// TopicID maps to the `topicid` table (key-value).
type TopicID struct {
	Report   string `gorm:"column:report;primaryKey;size:500" json:"report"`
	IDReport string `gorm:"column:idreport;type:text" json:"id_report"`
}

func (TopicID) TableName() string {
	return "topicid"
}

// Category maps to the `category` table.
type Category struct {
	ID     uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Remark string `gorm:"column:remark;size:500" json:"remark"`
}

func (Category) TableName() string {
	return "category"
}

// ServiceOther maps to the `service_other` table.
type ServiceOther struct {
	ID       uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	IDUser   string `gorm:"column:id_user;size:500" json:"id_user"`
	Username string `gorm:"column:username;size:1000" json:"username"`
	Value    string `gorm:"column:value;size:1000" json:"value"`
	Time     string `gorm:"column:time;size:200" json:"time"`
	Price    string `gorm:"column:price;size:200" json:"price"`
	Type     string `gorm:"column:type;size:1000" json:"type"`
	Status   string `gorm:"column:status;size:200" json:"status"`
	Output   string `gorm:"column:output;type:text" json:"output"`
}

func (ServiceOther) TableName() string {
	return "service_other"
}

// PaySetting maps to the `PaySetting` table (key-value).
type PaySetting struct {
	NamePay  string `gorm:"column:NamePay;primaryKey;size:500" json:"name_pay"`
	ValuePay string `gorm:"column:ValuePay;type:text" json:"value_pay"`
}

func (PaySetting) TableName() string {
	return "PaySetting"
}

// ShopSetting maps to the `shopSetting` table (key-value).
type ShopSetting struct {
	NameValue string `gorm:"column:Namevalue;primaryKey;size:500" json:"name_value"`
	Value     string `gorm:"column:value;type:text" json:"value"`
}

func (ShopSetting) TableName() string {
	return "shopSetting"
}

// LogsAPI maps to the `logs_api` table.
type LogsAPI struct {
	ID      uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Header  string `gorm:"column:header;type:json" json:"header"`
	Data    string `gorm:"column:data;type:json" json:"data"`
	IP      string `gorm:"column:ip;size:200" json:"ip"`
	Time    string `gorm:"column:time;size:200" json:"time"`
	Actions string `gorm:"column:actions;size:200" json:"actions"`
}

func (LogsAPI) TableName() string {
	return "logs_api"
}

// SupportMessage maps to the `support_message` table.
type SupportMessage struct {
	ID            uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Tracking      string `gorm:"column:Tracking;size:100" json:"tracking"`
	IDSupport     string `gorm:"column:idsupport;size:100" json:"id_support"`
	IDUser        string `gorm:"column:iduser;size:100" json:"id_user"`
	NameDepartman string `gorm:"column:name_departman;size:600" json:"name_departman"`
	Text          string `gorm:"column:text;type:text" json:"text"`
	Result        string `gorm:"column:result;type:text" json:"result"`
	Time          string `gorm:"column:time;size:200" json:"time"`
	Status        string `gorm:"column:status;type:enum('Answered','Pending','Unseen','Customerresponse','close')" json:"status"`
}

func (SupportMessage) TableName() string {
	return "support_message"
}

// GiftCodeConsumed maps to the `Giftcodeconsumed` table.
type GiftCodeConsumed struct {
	ID     uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Code   string `gorm:"column:code;size:2000" json:"code"`
	IDUser string `gorm:"column:id_user;size:200" json:"id_user"`
}

func (GiftCodeConsumed) TableName() string {
	return "Giftcodeconsumed"
}

// ManualSell maps to the `manualsell` table.
type ManualSell struct {
	ID            uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CodePanel     string `gorm:"column:codepanel;size:100" json:"code_panel"`
	CodeProduct   string `gorm:"column:codeproduct;size:100" json:"code_product"`
	NameRecord    string `gorm:"column:namerecord;size:200" json:"name_record"`
	Username      string `gorm:"column:username;size:500" json:"username"`
	ContentRecord string `gorm:"column:contentrecord;type:text" json:"content_record"`
	Status        string `gorm:"column:status;size:200" json:"status"`
}

func (ManualSell) TableName() string {
	return "manualsell"
}

// Help maps to the `help` table.
type Help struct {
	ID            uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NameOS        string `gorm:"column:name_os;size:500" json:"name_os"`
	MediaOS       string `gorm:"column:Media_os;size:5000" json:"media_os"`
	TypeMediaOS   string `gorm:"column:type_Media_os;size:500" json:"type_media_os"`
	Category      string `gorm:"column:category;type:text" json:"category"`
	DescriptionOS string `gorm:"column:Description_os;type:text" json:"description_os"`
}

func (Help) TableName() string {
	return "help"
}

// BotSaz maps to the `botsaz` table.
type BotSaz struct {
	ID        uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	IDUser    string `gorm:"column:id_user;size:200" json:"id_user"`
	BotToken  string `gorm:"column:bot_token;size:200" json:"bot_token"`
	AdminIDs  string `gorm:"column:admin_ids;type:text" json:"admin_ids"`
	Username  string `gorm:"column:username;size:200" json:"username"`
	Setting   string `gorm:"column:setting;type:text" json:"setting"`
	HidePanel string `gorm:"column:hide_panel;type:json" json:"hide_panel"`
	Time      string `gorm:"column:time;size:200" json:"time"`
}

func (BotSaz) TableName() string {
	return "botsaz"
}

// RequestAgent maps to the `Requestagent` table.
type RequestAgent struct {
	ID          string `gorm:"column:id;primaryKey;size:500" json:"id"`
	Username    string `gorm:"column:username;size:500" json:"username"`
	Time        string `gorm:"column:time;size:500" json:"time"`
	Description string `gorm:"column:Description;size:500" json:"description"`
	Status      string `gorm:"column:status;size:500" json:"status"`
	Type        string `gorm:"column:type;size:500" json:"type"`
}

func (RequestAgent) TableName() string {
	return "Requestagent"
}

// CancelService maps to the `cancel_service` table.
type CancelService struct {
	ID          uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	IDUser      string `gorm:"column:id_user;size:500" json:"id_user"`
	Username    string `gorm:"column:username;size:1000" json:"username"`
	Description string `gorm:"column:description;type:text" json:"description"`
	Status      string `gorm:"column:status;size:1000" json:"status"`
}

func (CancelService) TableName() string {
	return "cancel_service"
}

// Departman maps to the `departman` table.
type Departman struct {
	ID            uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	IDSupport     string `gorm:"column:idsupport;size:200" json:"id_support"`
	NameDepartman string `gorm:"column:name_departman;size:600" json:"name_departman"`
}

func (Departman) TableName() string {
	return "departman"
}

// WheelList maps to the `wheel_list` table.
type WheelList struct {
	ID        uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	IDUser    string `gorm:"column:id_user;size:200" json:"id_user"`
	Time      string `gorm:"column:time;size:200" json:"time"`
	FirstName string `gorm:"column:first_name;size:200" json:"first_name"`
	WheelCode string `gorm:"column:wheel_code;size:200" json:"wheel_code"`
	Price     string `gorm:"column:price;size:200" json:"price"`
}

func (WheelList) TableName() string {
	return "wheel_list"
}

// Affiliates maps to the `affiliates` table (single-row config).
type Affiliates struct {
	Description      string `gorm:"column:description;type:text" json:"description"`
	StatusCommission string `gorm:"column:status_commission;size:200" json:"status_commission"`
	Discount         string `gorm:"column:Discount;size:200" json:"discount"`
	PriceDiscount    string `gorm:"column:price_Discount;size:200" json:"price_discount"`
	PorsantOneBuy    string `gorm:"column:porsant_one_buy;size:100" json:"porsant_one_buy"`
	IDMedia          string `gorm:"column:id_media;size:300" json:"id_media"`
}

func (Affiliates) TableName() string {
	return "affiliates"
}

// ReagentReport maps to the `reagent_report` table.
type ReagentReport struct {
	ID      uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID  int64  `gorm:"column:user_id;uniqueIndex" json:"user_id"`
	GetGift bool   `gorm:"column:get_gift" json:"get_gift"`
	Time    string `gorm:"column:time;size:50" json:"time"`
	Reagent string `gorm:"column:reagent;size:30" json:"reagent"`
}

func (ReagentReport) TableName() string {
	return "reagent_report"
}

// App maps to the `app` table.
type App struct {
	ID   uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name string `gorm:"column:name;size:200" json:"name"`
	Link string `gorm:"column:link;size:200" json:"link"`
}

func (App) TableName() string {
	return "app"
}
