package models

import (
	"time"
)

// User represents an authenticated user on the system
type User struct {
	ID           int64     `xorm:"pk autoincr 'id'"`
	Username     string    `xorm:"unique notnull 'username'"`
	PasswordHash string    `xorm:"notnull 'password_hash'"`
	CreatedAt    time.Time `xorm:"created 'created_at'"`
	UpdatedAt    time.Time `xorm:"updated 'updated_at'"`
}

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

// Article represents a parsed RSS item, used to prevent duplicate printing
type Article struct {
	ID          int64     `xorm:"pk autoincr 'id'"`
	FeedID      int64     `xorm:"notnull index 'feed_id'"`
	GUID        string    `xorm:"notnull unique(feed_guid) 'guid'"` // Prevent duplicates per feed
	Title       string    `xorm:"notnull 'title'"`
	URL         string    `xorm:"'url'"`
	PublishedAt time.Time `xorm:"'published_at'"`
	CreatedAt   time.Time `xorm:"created 'created_at'"`
}

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

// Printer represents a discovered or manually added IPP printer
type Printer struct {
	ID        int64     `xorm:"pk autoincr 'id'"`
	Name      string    `xorm:"notnull 'name'"` // Friendly name (e.g., "Living Room Brother")
	Host      string    `xorm:"notnull 'host'"` // IP or hostname
	Port      int       `xorm:"notnull default 631 'port'"`
	URI       string    `xorm:"notnull 'uri'"` // Full IPP URI (e.g., ipp://192.168.1.100/ipp/print)
	IsDefault bool      `xorm:"notnull default 0 'is_default'"`
	CreatedAt time.Time `xorm:"created 'created_at'"`
	UpdatedAt time.Time `xorm:"updated 'updated_at'"`
}
