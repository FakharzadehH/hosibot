package models

import "database/sql"

// User maps to the `user` table.
// Primary key is the Telegram chat ID stored as string.
type User struct {
	ID                     string         `gorm:"column:id;primaryKey;size:500" json:"id"`
	LimitUserTest          int            `gorm:"column:limit_usertest;default:0" json:"limit_usertest"`
	RollStatus             bool           `gorm:"column:roll_Status;default:false" json:"roll_status"`
	Username               string         `gorm:"column:username;size:500" json:"username"`
	ProcessingValue        string         `gorm:"column:Processing_value;type:text" json:"processing_value"`
	ProcessingValueOne     string         `gorm:"column:Processing_value_one;type:text" json:"processing_value_one"`
	ProcessingValueTwo     string         `gorm:"column:Processing_value_tow;type:text" json:"processing_value_tow"`
	ProcessingValueFour    string         `gorm:"column:Processing_value_four;type:text" json:"processing_value_four"`
	Step                   string         `gorm:"column:step;size:500" json:"step"`
	DescriptionBlocking    sql.NullString `gorm:"column:description_blocking;type:text" json:"description_blocking"`
	Number                 string         `gorm:"column:number;size:300" json:"number"`
	Balance                int            `gorm:"column:Balance;default:0" json:"balance"`
	UserStatus             string         `gorm:"column:User_Status;size:500" json:"user_status"`
	PageNumber             int            `gorm:"column:pagenumber;default:0" json:"pagenumber"`
	MessageCount           string         `gorm:"column:message_count;size:100" json:"message_count"`
	LastMessageTime        string         `gorm:"column:last_message_time;size:100" json:"last_message_time"`
	Agent                  string         `gorm:"column:agent;size:100" json:"agent"`
	AffiliatesCount        string         `gorm:"column:affiliatescount;size:100" json:"affiliatescount"`
	Affiliates             string         `gorm:"column:affiliates;size:100" json:"affiliates"`
	NameCustom             string         `gorm:"column:namecustom;size:300" json:"namecustom"`
	NumberUsername         string         `gorm:"column:number_username;size:300" json:"number_username"`
	Register               string         `gorm:"column:register;size:100" json:"register"`
	Verify                 string         `gorm:"column:verify;size:100" json:"verify"`
	CardPayment            string         `gorm:"column:cardpayment;size:100" json:"cardpayment"`
	CodeInvitation         sql.NullString `gorm:"column:codeInvitation;size:100" json:"code_invitation"`
	PriceDiscount          sql.NullString `gorm:"column:pricediscount;size:100;default:'0'" json:"pricediscount"`
	HideMiniAppInstruction sql.NullString `gorm:"column:hide_mini_app_instruction;size:20;default:'0'" json:"hide_mini_app_instruction"`
	MaxBuyAgent            sql.NullString `gorm:"column:maxbuyagent;size:100;default:'0'" json:"maxbuyagent"`
	JoinChannel            sql.NullString `gorm:"column:joinchannel;size:100;default:'0'" json:"joinchannel"`
	CheckStatus            sql.NullString `gorm:"column:checkstatus;size:50;default:'0'" json:"checkstatus"`
	BotType                sql.NullString `gorm:"column:bottype;type:text" json:"bottype"`
	Score                  int            `gorm:"column:score;default:0" json:"score"`
	LimitChangeLoc         sql.NullString `gorm:"column:limitchangeloc;size:50;default:'0'" json:"limitchangeloc"`
	StatusCron             sql.NullString `gorm:"column:status_cron;size:20;default:'1'" json:"status_cron"`
	Expire                 sql.NullString `gorm:"column:expire;size:100" json:"expire"`
	Token                  sql.NullString `gorm:"column:token;size:100" json:"token"`
}

func (User) TableName() string {
	return "user"
}
