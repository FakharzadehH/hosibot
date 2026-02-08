package models

import "time"

// CronJob stores a queued cron task to be processed incrementally by scheduler workers.
type CronJob struct {
	ID             uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Kind           string    `gorm:"column:kind;size:50;index:idx_cron_jobs_kind_status,priority:1" json:"kind"`
	Status         string    `gorm:"column:status;size:30;index:idx_cron_jobs_kind_status,priority:2" json:"status"`
	ExternalRef    string    `gorm:"column:external_ref;size:255;index:idx_cron_jobs_external_ref" json:"external_ref"`
	Payload        string    `gorm:"column:payload;type:longtext" json:"payload"`
	TotalItems     int       `gorm:"column:total_items;default:0" json:"total_items"`
	ProcessedItems int       `gorm:"column:processed_items;default:0" json:"processed_items"`
	FailedItems    int       `gorm:"column:failed_items;default:0" json:"failed_items"`
	LastError      string    `gorm:"column:last_error;type:text" json:"last_error"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (CronJob) TableName() string {
	return "cron_jobs"
}

// CronJobItem stores a single target entry for a cron job.
type CronJobItem struct {
	ID        uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	JobID     uint      `gorm:"column:job_id;index:idx_cron_job_items_job_status,priority:1;index:idx_cron_job_items_job_target,priority:1" json:"job_id"`
	Target    string    `gorm:"column:target;size:255;index:idx_cron_job_items_job_target,priority:2" json:"target"`
	Status    string    `gorm:"column:status;size:30;index:idx_cron_job_items_job_status,priority:2" json:"status"`
	Attempts  int       `gorm:"column:attempts;default:0" json:"attempts"`
	LastError string    `gorm:"column:last_error;type:text" json:"last_error"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (CronJobItem) TableName() string {
	return "cron_job_items"
}
