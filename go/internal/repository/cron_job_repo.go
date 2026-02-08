package repository

import (
	"encoding/json"

	"gorm.io/gorm"

	"hosibot/internal/models"
)

// CronJobRepository handles queue-backed cron jobs.
type CronJobRepository struct {
	db *gorm.DB
}

func NewCronJobRepository(db *gorm.DB) *CronJobRepository {
	return &CronJobRepository{db: db}
}

// CreateJobWithItems creates a job and its pending targets in one transaction.
// If externalRef is non-empty and an active job already exists for that ref, it returns that job.
func (r *CronJobRepository) CreateJobWithItems(kind, externalRef string, payload interface{}, targets []string) (*models.CronJob, error) {
	if externalRef != "" {
		var existing models.CronJob
		err := r.db.Where("external_ref = ? AND kind = ? AND status IN ?", externalRef, kind, []string{"pending", "running"}).
			Order("id DESC").
			First(&existing).Error
		if err == nil {
			return &existing, nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}

	payloadRaw, _ := json.Marshal(payload)
	seen := make(map[string]bool)
	uniqueTargets := make([]string, 0, len(targets))
	for _, t := range targets {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		uniqueTargets = append(uniqueTargets, t)
	}

	job := &models.CronJob{
		Kind:        kind,
		Status:      "pending",
		ExternalRef: externalRef,
		Payload:     string(payloadRaw),
		TotalItems:  len(uniqueTargets),
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(job).Error; err != nil {
			return err
		}

		if len(uniqueTargets) == 0 {
			return nil
		}

		items := make([]models.CronJobItem, 0, len(uniqueTargets))
		for _, target := range uniqueTargets {
			items = append(items, models.CronJobItem{
				JobID:  job.ID,
				Target: target,
				Status: "pending",
			})
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return nil, err
	}

	return job, nil
}

// FindNextActiveByKind picks the next running/pending job for a kind.
func (r *CronJobRepository) FindNextActiveByKind(kind string) (*models.CronJob, error) {
	var running models.CronJob
	err := r.db.Where("kind = ? AND status = ?", kind, "running").
		Order("id ASC").
		First(&running).Error
	if err == nil {
		return &running, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	var pending models.CronJob
	err = r.db.Where("kind = ? AND status = ?", kind, "pending").
		Order("id ASC").
		First(&pending).Error
	if err != nil {
		return nil, err
	}
	return &pending, nil
}

func (r *CronJobRepository) HasActiveKind(kind string) (bool, error) {
	var count int64
	err := r.db.Model(&models.CronJob{}).
		Where("kind = ? AND status IN ?", kind, []string{"pending", "running"}).
		Count(&count).Error
	return count > 0, err
}

func (r *CronJobRepository) MarkRunning(jobID uint) error {
	return r.db.Model(&models.CronJob{}).
		Where("id = ? AND status IN ?", jobID, []string{"pending", "running"}).
		Update("status", "running").Error
}

func (r *CronJobRepository) Finalize(jobID uint, status, lastError string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if lastError != "" {
		updates["last_error"] = lastError
	}
	return r.db.Model(&models.CronJob{}).Where("id = ?", jobID).Updates(updates).Error
}

func (r *CronJobRepository) CountPendingItems(jobID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.CronJobItem{}).
		Where("job_id = ? AND status = ?", jobID, "pending").
		Count(&count).Error
	return count, err
}

func (r *CronJobRepository) ListPendingItems(jobID uint, limit int) ([]models.CronJobItem, error) {
	var items []models.CronJobItem
	q := r.db.Where("job_id = ? AND status = ?", jobID, "pending").Order("id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&items).Error
	return items, err
}

// MarkItemDone marks a job item as done and increments processed counter.
func (r *CronJobRepository) MarkItemDone(jobID, itemID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.CronJobItem{}).
			Where("id = ? AND job_id = ? AND status = ?", itemID, jobID, "pending").
			Updates(map[string]interface{}{
				"status":     "done",
				"attempts":   gorm.Expr("attempts + 1"),
				"last_error": "",
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		return tx.Model(&models.CronJob{}).Where("id = ?", jobID).
			Update("processed_items", gorm.Expr("processed_items + 1")).Error
	})
}

// MarkItemFailed marks a job item as failed and increments counters.
func (r *CronJobRepository) MarkItemFailed(jobID, itemID uint, errMsg string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.CronJobItem{}).
			Where("id = ? AND job_id = ? AND status = ?", itemID, jobID, "pending").
			Updates(map[string]interface{}{
				"status":     "failed",
				"attempts":   gorm.Expr("attempts + 1"),
				"last_error": errMsg,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		return tx.Model(&models.CronJob{}).Where("id = ?", jobID).
			Updates(map[string]interface{}{
				"processed_items": gorm.Expr("processed_items + 1"),
				"failed_items":    gorm.Expr("failed_items + 1"),
				"last_error":      errMsg,
			}).Error
	})
}
