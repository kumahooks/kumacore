package app

import (
	"context"

	"kumacore/core/db/migrate"
)

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
