package models

import "time"

// PrintJob tracks the state of an automated print request
type PrintJob struct {
	ID         int64     `xorm:"pk autoincr 'id'"`
	ArticleID  int64     `xorm:"notnull index 'article_id'"`
	PrinterID  int64     `xorm:"notnull index 'printer_id'"`
	Status     string    `xorm:"notnull default 'Pending' 'status'"` // Pending, Sent, Failed
	RetryCount int       `xorm:"notnull default 0 'retry_count'"`
	LastError  string    `xorm:"'last_error'"`
	CreatedAt  time.Time `xorm:"created 'created_at'"`
	UpdatedAt  time.Time `xorm:"updated 'updated_at'"`
}
