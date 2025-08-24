package gorm

import (
	"fmt"

	"service-faas/internal/core/functions"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

// New creates a new GORM database instance and runs migrations.
func New(dsn string, lg zerolog.Logger) (*gorm.DB, error) {
	// Configure GORM's logger to use Zerolog
	gormLogger := gormlog.New(
		&lg,
		gormlog.Config{
			SlowThreshold: 0, // log all queries
			LogLevel:      gormlog.Info,
			Colorful:      false,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	// AutoMigrate will create the 'functions' table based on the struct definition.
	if err := db.AutoMigrate(&functions.Function{}); err != nil {
		return nil, fmt.Errorf("gorm migrate: %w", err)
	}
	lg.Info().Msg("database migration successful")

	return db, nil
}
