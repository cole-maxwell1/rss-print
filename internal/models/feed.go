package models

import "time"

// Feed represents an RSS/Atom feed URL to poll
type Feed struct {
	ID              int64     `xorm:"pk autoincr 'id'"`
	Name            string    `xorm:"notnull 'name'"`
	URL             string    `xorm:"notnull unique 'url'"`
	AuthHeaderName  string    `xorm:"'auth_header_name'"`
	AuthHeaderValue string    `xorm:"'auth_header_value'"`
	AutoPrint       bool      `xorm:"notnull default 0 'auto_print'"`     // Automatically print new articles
	PrinterID       int64     `xorm:"index 'printer_id'"`                 // Default printer for this feed (0 = global default)
	PollInterval    int       `xorm:"notnull default 30 'poll_interval'"` // Poll interval in minutes
	LastPolledAt    time.Time `xorm:"'last_polled_at'"`
	LastError       string    `xorm:"'last_error'"`
	CreatedAt       time.Time `xorm:"created 'created_at'"`
	UpdatedAt       time.Time `xorm:"updated 'updated_at'"`
}
