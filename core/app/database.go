package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"kumacore/core/db"
	"kumacore/core/db/migrate"
)

func (application *App) openDatabase() error {
	if err := os.MkdirAll(filepath.Dir(application.options.Configuration.Core.DB.Path), 0o755); err != nil {
		return fmt.Errorf("[app:openDatabase] create database directory: %w", err)
	}

	database, databaseDialect, err := db.Open(
		application.options.Configuration.Core.DB.Driver,
		application.options.Configuration.Core.DB.Path,
	)
	if err != nil {
		return err
	}

	application.runtime.database = database
	application.runtime.dialect = databaseDialect

	return nil
}

func (application *App) runMigrations(ctx context.Context) error {
	migrationSource := application.options.MigrationSource
	if migrationSource.FileSystem == nil {
		migrationSource.FileSystem = application.options.FileSystem
	}
	if migrationSource.Backend == "" {
		migrationSource.Backend = application.runtime.dialect.Name()
	}
	if migrationSource.Directory == "" {
		migrationSource.Directory = defaultMigrationDirectory
	}

	plan, err := migrate.Validate(ctx, application.runtime.database, application.runtime.dialect, migrationSource)
	if err != nil {
		return err
	}

	return migrate.ApplyPlan(ctx, application.runtime.database, plan)
}
