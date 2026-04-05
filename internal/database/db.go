package database

import (
	"github.com/Full-finger/NDisk/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func New(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	if cfg.Database.Driver == "sqlite" {
		dialector = sqlite.Open(cfg.Database.DSN)
	}
	// TODO: 后续加 PostgreSQL 支持

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
