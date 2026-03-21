package database

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/nilabh/arthaledger/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPostgresDB creates and returns a new PostgreSQL database connection.
// It returns an error instead of panicking so the caller can handle it gracefully.
func NewPostgresDB(cfg *config.Config) (*gorm.DB, error) {
	// Build PostgreSQL DSN with explicit timezone for consistent timestamp handling
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	// Use verbose SQL logging in development, silent in production
	gormLogger := logger.Default.LogMode(logger.Silent)
	if cfg.App.Env == "development" {
		gormLogger = logger.Default.LogMode(logger.Info)
	}

	// Open database connection
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection (host=%s db=%s): %w",
			cfg.Database.Host, cfg.Database.Name, err)
	}

	// Get underlying sql.DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Connection pool tuning
	sqlDB.SetMaxIdleConns(10)                  // idle connections kept alive
	sqlDB.SetMaxOpenConns(100)                 // hard cap on concurrent connections
	sqlDB.SetConnMaxLifetime(time.Hour)        // recycle connections after 1 h to avoid stale state
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // release idle conns faster

	slog.Info("Connected to PostgreSQL",
		"host", cfg.Database.Host,
		"port", cfg.Database.Port,
		"database", cfg.Database.Name,
	)

	return db, nil
}
