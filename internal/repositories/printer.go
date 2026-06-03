package repositories

import (
	"rss-print/internal/models"

	"xorm.io/xorm"
)

// PrinterRepo provides data access for Printer records.
type PrinterRepo struct {
	engine *xorm.Engine
}

func NewPrinterRepo(engine *xorm.Engine) *PrinterRepo {
	return &PrinterRepo{engine: engine}
}

// List returns all printers ordered with the default first, then by name.
func (r *PrinterRepo) List() ([]models.Printer, error) {
	var printers []models.Printer
	if err := r.engine.OrderBy("is_default DESC, name ASC").Find(&printers); err != nil {
		return nil, err
	}
	return printers, nil
}

// ListByCreated returns all printers ordered with the default first, then newest.
func (r *PrinterRepo) ListByCreated() ([]models.Printer, error) {
	var printers []models.Printer
	if err := r.engine.OrderBy("is_default DESC, created_at DESC").Find(&printers); err != nil {
		return nil, err
	}
	return printers, nil
}

// GetByID returns the printer with the given id, or found=false when none exists.
func (r *PrinterRepo) GetByID(id int64) (*models.Printer, bool, error) {
	var printer models.Printer
	has, err := r.engine.ID(id).Get(&printer)
	if err != nil || !has {
		return nil, has, err
	}
	return &printer, true, nil
}

// GetByURI returns the printer matching uri, or found=false when none exists.
func (r *PrinterRepo) GetByURI(uri string) (*models.Printer, bool, error) {
	var printer models.Printer
	has, err := r.engine.Where("uri = ?", uri).Get(&printer)
	if err != nil || !has {
		return nil, has, err
	}
	return &printer, true, nil
}

// GetDefault returns the printer flagged as default, or found=false when none is set.
func (r *PrinterRepo) GetDefault() (*models.Printer, bool, error) {
	var printer models.Printer
	has, err := r.engine.Where("is_default = ?", true).Get(&printer)
	if err != nil || !has {
		return nil, has, err
	}
	return &printer, true, nil
}

// Count returns the total number of printers.
func (r *PrinterRepo) Count() (int64, error) {
	return r.engine.Count(new(models.Printer))
}

// Create inserts a new printer.
func (r *PrinterRepo) Create(printer *models.Printer) error {
	_, err := r.engine.Insert(printer)
	return err
}

// UpdateDetails persists the editable fields of an existing printer.
func (r *PrinterRepo) UpdateDetails(printer *models.Printer) error {
	_, err := r.engine.ID(printer.ID).Cols("name", "host", "port", "uri", "updated_at").Update(printer)
	return err
}

// MakeDefault clears the default flag on every printer and sets it on the given
// one, in a single transaction so exactly one printer is ever the default.
func (r *PrinterRepo) MakeDefault(printer *models.Printer) error {
	_, err := r.engine.Transaction(func(session *xorm.Session) (any, error) {
		if _, err := session.Exec("UPDATE printer SET is_default = 0"); err != nil {
			return nil, err
		}
		printer.IsDefault = true
		if _, err := session.ID(printer.ID).Cols("is_default", "updated_at").Update(printer); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}
