package models

import "database/sql"

// Panel maps to the `marzban_panel` table.
type Panel struct {
	ID                uint           `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CodePanel         string         `gorm:"column:code_panel;size:200" json:"code_panel"`
	NamePanel         string         `gorm:"column:name_panel;size:2000" json:"name_panel"`
	Status            string         `gorm:"column:status;size:500" json:"status"`
	URLPanel          string         `gorm:"column:url_panel;size:2000" json:"url_panel"`
	UsernamePanel     string         `gorm:"column:username_panel;size:200" json:"username_panel"`
	PasswordPanel     string         `gorm:"column:password_panel;size:200" json:"password_panel"`
	Agent             string         `gorm:"column:agent;size:200" json:"agent"`
	SubLink           string         `gorm:"column:sublink;size:500" json:"sublink"`
	Config            string         `gorm:"column:config;size:500" json:"config"`
	MethodUsername    string         `gorm:"column:MethodUsername;size:700" json:"method_username"`
	TestAccount       string         `gorm:"column:TestAccount;size:100" json:"test_account"`
	LimitPanel        string         `gorm:"column:limit_panel;size:100" json:"limit_panel"`
	NameCustom        string         `gorm:"column:namecustom;size:100" json:"namecustom"`
	MethodExtend      string         `gorm:"column:Methodextend;size:100" json:"method_extend"`
	Connection        string         `gorm:"column:conecton;size:100" json:"connection"`
	LinkSubX          string         `gorm:"column:linksubx;size:1000" json:"link_sub_x"`
	InboundID         string         `gorm:"column:inboundid;size:100" json:"inbound_id"`
	Type              string         `gorm:"column:type;size:100" json:"type"`
	InboundStatus     string         `gorm:"column:inboundstatus;size:100" json:"inbound_status"`
	InboundDeactive   string         `gorm:"column:inbound_deactive;size:100" json:"inbound_deactive"`
	TimeUserTest      string         `gorm:"column:time_usertest;size:100" json:"time_usertest"`
	ValUserTest       string         `gorm:"column:val_usertest;size:100" json:"val_usertest"`
	SecretCode        string         `gorm:"column:secret_code;size:200" json:"secret_code"`
	PriceChangeLoc    string         `gorm:"column:priceChangeloc;size:200" json:"price_change_loc"`
	PriceExtraVolume  string         `gorm:"column:priceextravolume;size:500" json:"price_extra_volume"`
	PriceCustomVolume string         `gorm:"column:pricecustomvolume;size:500" json:"price_custom_volume"`
	PriceCustomTime   string         `gorm:"column:pricecustomtime;size:500" json:"price_custom_time"`
	PriceExtraTime    string         `gorm:"column:priceextratime;size:500" json:"price_extra_time"`
	MainVolume        string         `gorm:"column:mainvolume;size:500" json:"main_volume"`
	MaxVolume         string         `gorm:"column:maxvolume;size:500" json:"max_volume"`
	MainTime          string         `gorm:"column:maintime;size:500" json:"main_time"`
	MaxTime           string         `gorm:"column:maxtime;size:500" json:"max_time"`
	StatusExtend      string         `gorm:"column:status_extend;size:100" json:"status_extend"`
	DateLogin         string         `gorm:"column:datelogin;type:text" json:"date_login"`
	Proxies           string         `gorm:"column:proxies;type:text" json:"proxies"`
	Inbounds          string         `gorm:"column:inbounds;type:text" json:"inbounds"`
	SubVIP            sql.NullString `gorm:"column:subvip;size:60" json:"sub_vip"`
	ChangeLoc         sql.NullString `gorm:"column:changeloc;size:60" json:"change_loc"`
	OnHoldTest        sql.NullString `gorm:"column:on_hold_test;size:60" json:"on_hold_test"`
	VersionPanel      sql.NullString `gorm:"column:version_panel;size:60" json:"version_panel"`
	CustomVolume      string         `gorm:"column:customvolume;type:text" json:"custom_volume"`
	HideUser          string         `gorm:"column:hide_user;type:text" json:"hide_user"`
}

func (Panel) TableName() string {
	return "marzban_panel"
}
