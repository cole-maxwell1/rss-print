package models

import "time"

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

