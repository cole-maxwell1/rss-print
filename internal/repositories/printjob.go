package repositories

import (
	"rss-print/internal/models"

	"xorm.io/xorm"
)

// PrintJobRepo provides data access for PrintJob records.
type PrintJobRepo struct {
	engine *xorm.Engine
}

func NewPrintJobRepo(engine *xorm.Engine) *PrintJobRepo {
	return &PrintJobRepo{engine: engine}
}

// ListRecent returns the most recent print jobs, newest first, up to limit.
func (r *PrintJobRepo) ListRecent(limit int) ([]models.PrintJob, error) {
	var jobs []models.PrintJob
	if err := r.engine.OrderBy("created_at DESC").Limit(limit).Find(&jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// ListPending returns jobs that are pending or failed but still within retry budget.
func (r *PrintJobRepo) ListPending() ([]models.PrintJob, error) {
	var jobs []models.PrintJob
	if err := r.engine.Where("status = 'Pending' OR (status = 'Failed' AND retry_count < 3)").Find(&jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// GetByID returns the print job with the given id, or found=false when none exists.
func (r *PrintJobRepo) GetByID(id int64) (*models.PrintJob, bool, error) {
	var job models.PrintJob
	has, err := r.engine.ID(id).Get(&job)
	if err != nil || !has {
		return nil, has, err
	}
	return &job, true, nil
}

// Create inserts a new print job.
func (r *PrintJobRepo) Create(job *models.PrintJob) error {
	_, err := r.engine.Insert(job)
	return err
}

// UpdateStatus persists a job's status and last error.
func (r *PrintJobRepo) UpdateStatus(job *models.PrintJob) error {
	_, err := r.engine.ID(job.ID).Cols("status", "last_error", "updated_at").Update(job)
	return err
}

// MarkFailed persists a job's failed status, including the incremented retry count.
func (r *PrintJobRepo) MarkFailed(job *models.PrintJob) error {
	_, err := r.engine.ID(job.ID).Cols("status", "last_error", "retry_count", "updated_at").Update(job)
	return err
}
