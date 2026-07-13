package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"kumacore/app/migrations"
	"kumacore/core/config"
	"kumacore/core/db"
	"kumacore/core/db/dialect"
	"kumacore/core/db/migrate"
)

func openAppDatabase(configuration *config.Config) (*sql.DB, dialect.Dialect, error) {
	if err := os.MkdirAll(filepath.Dir(configuration.Core.DB.Path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("[server:openAppDatabase] create database directory: %w", err)
	}

	databaseConnection, databaseDialect, err := db.Open(
		configuration.Core.DB.Driver,
		configuration.Core.DB.Path,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("[server:openAppDatabase] open database: %w", err)
	}

	return databaseConnection, databaseDialect, nil
}

func openWorkerDatabase(configuration *config.Config) (*sql.DB, dialect.Dialect, error) {
	if err := os.MkdirAll(filepath.Dir(configuration.Core.Worker.DBPath), 0o755); err != nil {
		return nil, nil, fmt.Errorf("[server:openWorkerDatabase] create worker database directory: %w", err)
	}

	databaseConnection, databaseDialect, err := db.Open(
		configuration.Core.DB.Driver,
		configuration.Core.Worker.DBPath,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("[server:openWorkerDatabase] open worker database: %w", err)
	}

	return databaseConnection, databaseDialect, nil
}

func newAppMigrationSource() migrate.Source {
	return migrations.AppSource()
}

func newWorkerMigrationSource() migrate.Source {
	return migrations.WorkerSource()
}
