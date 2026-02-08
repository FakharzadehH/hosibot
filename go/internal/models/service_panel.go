package models

// ServicePanel maps to the `panel` table used by api/panels.php CRUD actions.
type ServicePanel struct {
	ID               uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CodePanel        string `gorm:"column:code_panel;size:200" json:"code_panel"`
	NamePanel        string `gorm:"column:name_panel;size:2000" json:"name_panel"`
	PricePanel       string `gorm:"column:price_panel;size:200" json:"price_panel"`
	VolumeConstraint string `gorm:"column:Volume_constraint;size:200" json:"volume_constraint"`
	ServiceTime      string `gorm:"column:Service_time;size:200" json:"service_time"`
	Location         string `gorm:"column:Location;size:500" json:"location"`
	Agent            string `gorm:"column:agent;size:200" json:"agent"`
	Note             string `gorm:"column:note;type:text" json:"note"`
	DataLimitReset   string `gorm:"column:data_limit_reset;size:200" json:"data_limit_reset"`
	Inbounds         string `gorm:"column:inbounds;type:text" json:"inbounds"`
	Proxies          string `gorm:"column:proxies;type:text" json:"proxies"`
	Category         string `gorm:"column:category;size:500" json:"category"`
	OneBuyStatus     string `gorm:"column:one_buy_status;size:50" json:"one_buy_status"`
	HidePanel        string `gorm:"column:hide_panel;type:text" json:"hide_panel"`
	SubLink          string `gorm:"column:sublink;size:500" json:"sublink"`
	Config           string `gorm:"column:config;size:500" json:"config"`
	Status           string `gorm:"column:status;size:200" json:"status"`
}

func (ServicePanel) TableName() string {
	return "panel"
}
