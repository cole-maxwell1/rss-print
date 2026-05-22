package db

import (
	"fmt"
	"log"

	"rss-print/internal/models"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver
	"xorm.io/xorm"
)

// InitDB initializes the SQLite database connection and performs migrations.
func InitDB(dsn string) (*xorm.Engine, error) {
	// Enable WAL mode for better concurrency and append _busy_timeout to prevent 'database is locked' errors.
	// We append query parameters if not present, but for simplicity we can construct it if DSN is just a filename.
	connStr := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dsn)

	engine, err := xorm.NewEngine("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create db engine: %w", err)
	}

	// Limit concurrent connections to avoid SQLite write locking issues even with WAL
	engine.SetMaxOpenConns(1)

	// Ping the database to ensure connection is valid
	if err := engine.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	// Perform schema synchronization (migrations)
	err = engine.Sync(
		new(models.User),
		new(models.Feed),
		new(models.Article),
		new(models.PrintJob),
		new(models.Printer),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sync database schema: %w", err)
	}

	log.Println("Database initialized and schema synced successfully.")

	return engine, nil
}
