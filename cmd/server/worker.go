package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	// kumacore:begin jobs-imports
	jobslogging "kumacore/app/jobs/logging"
	// kumacore:end jobs-imports

	"kumacore/core/config"
	"kumacore/core/worker"
)

type workerBootstrap struct {
	runtime *worker.Runtime
	jobs    []worker.Job
}

func buildWorkerBootstrap(configuration *config.Config, fileSystem fs.FS) (workerBootstrap, error) {
	jobs := []worker.Job{
		// kumacore:begin jobs-register
		{
			Name:     "logging:cleanup",
			Interval: 24 * time.Hour,
			Run: func(ctx context.Context, _ any) error {
				return jobslogging.Cleanup(ctx, configuration.Core.Logging.Dir)
			},
		},
		// kumacore:end jobs-register
	}

	if !configuration.Core.Worker.Enabled {
		return workerBootstrap{jobs: jobs}, nil
	}

	if err := os.MkdirAll(
		filepath.Join(configuration.App.RootDir, workerMigrationDirectory),
		0o755,
	); err != nil {
		return workerBootstrap{}, fmt.Errorf("[server:buildWorkerBootstrap] create worker migration directory: %w", err)
	}

	workerDatabase, workerDialect, err := openWorkerDatabase(configuration)
	if err != nil {
		return workerBootstrap{}, err
	}

	runtime, err := worker.NewRuntime(
		workerDatabase,
		workerDialect,
		newWorkerMigrationSource(fileSystem, workerDialect),
		configuration.Core.Worker.PollInterval,
		configuration.Core.Worker.MaxAttempts,
	)
	if err != nil {
		return workerBootstrap{}, fmt.Errorf("[server:buildWorkerBootstrap] create worker runtime: %w", err)
	}

	return workerBootstrap{
		runtime: runtime,
		jobs:    jobs,
	}, nil
}
