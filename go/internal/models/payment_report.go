package models

// PaymentReport maps to the `Payment_report` table.
type PaymentReport struct {
	ID              uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	IDUser          string `gorm:"column:id_user;size:200" json:"id_user"`
	IDOrder         string `gorm:"column:id_order;size:2000" json:"id_order"`
	Time            string `gorm:"column:time;size:200" json:"time"`
	AtUpdated       string `gorm:"column:at_updated;size:200" json:"at_updated"`
	Price           string `gorm:"column:price;size:200" json:"price"`
	DecNotConfirmed string `gorm:"column:dec_not_confirmed;type:text" json:"dec_not_confirmed"`
	PaymentMethod   string `gorm:"column:Payment_Method;size:400" json:"payment_method"`
	PaymentStatus   string `gorm:"column:payment_Status;size:100" json:"payment_status"`
	BotType         string `gorm:"column:bottype;size:300" json:"bottype"`
	MessageID       int    `gorm:"column:message_id" json:"message_id"`
	IDInvoice       string `gorm:"column:id_invoice;size:1000" json:"id_invoice"`
}

func (PaymentReport) TableName() string {
	return "Payment_report"
}
