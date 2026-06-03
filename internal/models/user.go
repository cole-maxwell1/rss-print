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
