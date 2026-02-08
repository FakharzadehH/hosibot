package models

// Product maps to the `product` table.
type Product struct {
	ID               uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CodeProduct      string `gorm:"column:code_product;size:200" json:"code_product"`
	NameProduct      string `gorm:"column:name_product;size:2000" json:"name_product"`
	PriceProduct     string `gorm:"column:price_product;size:2000" json:"price_product"`
	VolumeConstraint string `gorm:"column:Volume_constraint;size:2000" json:"volume_constraint"`
	Location         string `gorm:"column:Location;size:200" json:"location"`
	ServiceTime      string `gorm:"column:Service_time;size:200" json:"service_time"`
	Agent            string `gorm:"column:agent;size:100" json:"agent"`
	Note             string `gorm:"column:note;type:text" json:"note"`
	DataLimitReset   string `gorm:"column:data_limit_reset;size:200" json:"data_limit_reset"`
	OneBuyStatus     string `gorm:"column:one_buy_status;size:20" json:"one_buy_status"`
	Inbounds         string `gorm:"column:inbounds;type:text" json:"inbounds"`
	Proxies          string `gorm:"column:proxies;type:text" json:"proxies"`
	Category         string `gorm:"column:category;size:400" json:"category"`
	HidePanel        string `gorm:"column:hide_panel;type:text" json:"hide_panel"`
}

func (Product) TableName() string {
	return "product"
}
