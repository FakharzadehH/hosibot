package models

// Invoice maps to the `invoice` table.
type Invoice struct {
	IDInvoice       string `gorm:"column:id_invoice;primaryKey;size:200" json:"id_invoice"`
	IDUser          string `gorm:"column:id_user;size:200" json:"id_user"`
	Username        string `gorm:"column:username;size:300" json:"username"`
	ServiceLocation string `gorm:"column:Service_location;size:300" json:"service_location"`
	TimeSell        string `gorm:"column:time_sell;size:200" json:"time_sell"`
	NameProduct     string `gorm:"column:name_product;size:200" json:"name_product"`
	PriceProduct    string `gorm:"column:price_product;size:200" json:"price_product"`
	Volume          string `gorm:"column:Volume;size:200" json:"volume"`
	ServiceTime     string `gorm:"column:Service_time;size:200" json:"service_time"`
	UUID            string `gorm:"column:uuid;type:text" json:"uuid"`
	Note            string `gorm:"column:note;size:500" json:"note"`
	UserInfo        string `gorm:"column:user_info;type:text" json:"user_info"`
	BotType         string `gorm:"column:bottype;size:200" json:"bottype"`
	Referral        string `gorm:"column:refral;size:100" json:"referral"`
	TimeCron        string `gorm:"column:time_cron;size:100" json:"time_cron"`
	Notifications   string `gorm:"column:notifctions;type:text" json:"notifications"`
	Status          string `gorm:"column:Status;size:200" json:"status"`
}

func (Invoice) TableName() string {
	return "invoice"
}
