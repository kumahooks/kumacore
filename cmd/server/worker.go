package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	// kumacore:begin jobs-imports
	"kumacore/app/jobs/backup"
	"kumacore/app/jobs/logging"
	// kumacore:end jobs-imports

	"kumacore/app/migrations"
	"kumacore/core/config"
	"kumacore/core/worker"
)

type workerBootstrap struct {
	runtime *worker.Runtime
	jobs    []worker.Job
}

func buildWorkerBootstrap(configuration *config.Config, appDatabase *sql.DB) (workerBootstrap, error) {
	if !configuration.Core.Worker.Enabled {
		return workerBootstrap{jobs: []worker.Job{}}, nil
	}

	workerDatabase, workerDialect, err := openWorkerDatabase(configuration)
	if err != nil {
		return workerBootstrap{}, err
	}

	dataDir := filepath.Dir(configuration.Core.DB.Path)
	backupDir := configuration.Core.Backup.Dir

	jobs := []worker.Job{
		// kumacore:begin jobs-register
		{
			Name:     "logging:cleanup",
			Interval: 24 * time.Hour,
			Run: func(ctx context.Context, _ any) error {
				return logging.Cleanup(ctx, configuration.Core.Logging.Dir)
			},
		},
		{
			Name:     "backup:daily",
			Interval: 24 * time.Hour,
			Run: func(ctx context.Context, _ any) error {
				return backup.Run(ctx, appDatabase, workerDatabase, dataDir, backupDir, backup.SuffixDaily)
			},
		},
		{
			Name:     "backup:weekly",
			Interval: 7 * 24 * time.Hour,
			Run: func(ctx context.Context, _ any) error {
				return backup.Run(ctx, appDatabase, workerDatabase, dataDir, backupDir, backup.SuffixWeekly)
			},
		},
		{
			Name:     "backup:cleanup",
			Interval: 24 * time.Hour,
			Run: func(ctx context.Context, _ any) error {
				return backup.Cleanup(ctx, backupDir)
			},
		},
		// kumacore:end jobs-register
	}

	runtime, err := worker.NewRuntime(
		workerDatabase,
		workerDialect,
		migrations.WorkerSource(),
		configuration.Core.Worker.PollInterval,
		configuration.Core.Worker.MaxAttempts,
	)
	if err != nil {
		return workerBootstrap{}, err
	}

	return workerBootstrap{
		runtime: runtime,
		jobs:    jobs,
	}, nil
}
