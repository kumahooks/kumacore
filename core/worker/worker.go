// Package worker provides a simple background job runner.
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"kumacore/core/db/dialect"
	"kumacore/core/db/migrate"
)

// Job is a named background task.
// Interval = 0 is for on-demand jobs enqueued to the worker database.
type Job struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context, payload any) error
}

// Runtime holds worker jobs, the queue database, and runner settings.
type Runtime struct {
	database        *sql.DB
	dialect         dialect.Dialect
	migrationSource migrate.Source
	pollInterval    time.Duration
	maxAttempts     int
	jobs            []Job
	jobMap          map[string]Job
	queue           *jobQueue
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewRuntime creates a worker runtime backed by a dedicated worker database.
func NewRuntime(
	database *sql.DB,
	databaseDialect dialect.Dialect,
	migrationSource migrate.Source,
	pollInterval time.Duration,
	maxAttempts int,
) (*Runtime, error) {
	if database == nil {
		return nil, fmt.Errorf("[worker:NewRuntime] nil database")
	}

	if databaseDialect == nil {
		return nil, fmt.Errorf("[worker:NewRuntime] nil dialect")
	}

	if pollInterval <= 0 {
		return nil, fmt.Errorf("[worker:NewRuntime] poll interval must be positive")
	}

	if maxAttempts < 1 {
		return nil, fmt.Errorf("[worker:NewRuntime] max attempts must be at least 1")
	}

	return &Runtime{
		database:        database,
		dialect:         databaseDialect,
		migrationSource: migrationSource,
		pollInterval:    pollInterval,
		maxAttempts:     maxAttempts,
		jobMap:          make(map[string]Job),
		queue:           &jobQueue{repository: newSQLQueueRepository(database)},
	}, nil
}

// Initialize validates and applies worker database migrations.
func (runtime *Runtime) Initialize(ctx context.Context) error {
	plan, err := migrate.Validate(ctx, runtime.database, runtime.dialect, runtime.migrationSource)
	if err != nil {
		return err
	}

	return migrate.ApplyPlan(ctx, runtime.database, plan)
}

// Register appends one or more jobs to the runtime.
func (runtime *Runtime) Register(jobs ...Job) error {
	for _, job := range jobs {
		jobName := strings.TrimSpace(job.Name)
		if jobName == "" {
			return fmt.Errorf("[worker:Register] empty job name")
		}

		if job.Run == nil {
			return fmt.Errorf("[worker:Register] nil run function for %s", jobName)
		}

		if _, exists := runtime.jobMap[jobName]; exists {
			return fmt.Errorf("[worker:Register] duplicate job %q", jobName)
		}

		job.Name = jobName
		runtime.jobs = append(runtime.jobs, job)
		runtime.jobMap[jobName] = job

		slog.Info(
			"registered job",
			slog.String("caller", "worker:Register"),
			slog.String("job", job.Name),
			slog.Duration("interval", job.Interval),
		)
	}

	return nil
}

// Start spawns scheduled jobs and starts the queue poll loop.
func (runtime *Runtime) Start(ctx context.Context) {
	ctx, runtime.cancel = context.WithCancel(ctx)

	slog.Info(
		"worker initialized",
		slog.String("caller", "worker:Start"),
		slog.Int("jobs", len(runtime.jobs)),
	)

	for _, job := range runtime.jobs {
		if job.Interval > 0 {
			runtime.wg.Add(1)
			go func() {
				defer runtime.wg.Done()
				runJob(ctx, job)
			}()
		}
	}

	runtime.wg.Add(1)
	go func() {
		defer runtime.wg.Done()
		runtime.pollLoop(ctx)
	}()
}

// Close cancels all running goroutines, waits for them to drain, then
// closes the dedicated worker database.
func (runtime *Runtime) Close() error {
	if runtime.database == nil {
		return nil
	}

	if runtime.cancel != nil {
		runtime.cancel()
	}

	runtime.wg.Wait()

	err := runtime.database.Close()
	runtime.database = nil

	return err
}

// Enqueue writes a job to the DB queue for asynchronous execution.
func (runtime *Runtime) Enqueue(name string, payload any) error {
	if _, ok := runtime.jobMap[name]; !ok {
		return fmt.Errorf("[worker:Enqueue] unknown job: %s", name)
	}

	return runtime.queue.enqueue(name, payload)
}

// TriggerJob spawns a one-off run of the named job directly.
func (runtime *Runtime) TriggerJob(name string, payload any) error {
	job, ok := runtime.jobMap[name]
	if !ok {
		return fmt.Errorf("[worker:TriggerJob] unknown job: %s", name)
	}

	runtime.wg.Add(1)
	go func() {
		defer runtime.wg.Done()
		runOnce(context.Background(), job, payload)
	}()

	return nil
}

func (runtime *Runtime) pollLoop(ctx context.Context) {
	runtime.queue.resetOrphaned(ctx)

	ticker := time.NewTicker(runtime.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runtime.processQueue(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (runtime *Runtime) processQueue(ctx context.Context) {
	records, err := runtime.queue.claimPending(ctx)
	if err != nil {
		slog.ErrorContext(
			ctx, "poll queue",
			slog.String("caller", "worker:processQueue"),
			slog.Any("err", err),
		)

		return
	}

	for _, record := range records {
		runtime.wg.Add(1)
		go func() {
			defer runtime.wg.Done()
			runtime.executeQueued(ctx, record)
		}()
	}
}

func (runtime *Runtime) executeQueued(ctx context.Context, record jobRecord) {
	job, ok := runtime.jobMap[record.name]
	if !ok {
		slog.ErrorContext(
			ctx, "unknown job",
			slog.String("caller", "worker:executeQueued"),
			slog.String("job", record.name),
			slog.String("job_id", record.id),
		)

		if err := runtime.queue.bury(
			ctx,
			record,
			record.attempts,
			fmt.Sprintf("unknown job: %s", record.name),
		); err != nil {
			slog.ErrorContext(
				ctx, "bury unknown job",
				slog.String("caller", "worker:executeQueued"),
				slog.String("job", record.name),
				slog.Any("err", err),
			)
		}

		return
	}

	var payload any
	if record.payload != nil {
		if err := json.Unmarshal([]byte(*record.payload), &payload); err != nil {
			slog.ErrorContext(
				ctx, "unmarshal payload",
				slog.String("caller", "worker:executeQueued"),
				slog.String("job", record.name),
				slog.String("job_id", record.id),
				slog.Any("err", err),
			)

			if err = runtime.queue.bury(ctx, record, record.attempts, err.Error()); err != nil {
				slog.ErrorContext(
					ctx, "bury unmarshal failure",
					slog.String("caller", "worker:executeQueued"),
					slog.String("job", record.name),
					slog.Any("err", err),
				)
			}

			return
		}
	}

	newAttempts := record.attempts + 1
	slog.InfoContext(
		ctx, "job started",
		slog.String("caller", "worker:executeQueued"),
		slog.String("job", record.name),
		slog.Int("attempt", newAttempts),
		slog.Int("max_attempts", runtime.maxAttempts),
	)

	start := time.Now()
	runErr := job.Run(ctx, payload)
	if runErr == nil {
		slog.InfoContext(
			ctx, "job done",
			slog.String("caller", "worker:executeQueued"),
			slog.String("job", record.name),
			slog.Duration("duration", time.Since(start)),
		)

		if err := runtime.queue.complete(ctx, record, newAttempts); err != nil {
			slog.ErrorContext(
				ctx, "complete job",
				slog.String("caller", "worker:executeQueued"),
				slog.String("job", record.name),
				slog.Any("err", err),
			)
		}

		return
	}

	slog.ErrorContext(
		ctx, "job attempt failed",
		slog.String("caller", "worker:executeQueued"),
		slog.String("job", record.name),
		slog.Int("attempt", newAttempts),
		slog.Any("err", runErr),
	)

	if newAttempts >= runtime.maxAttempts {
		slog.ErrorContext(
			ctx, "max attempts reached, burying",
			slog.String("caller", "worker:executeQueued"),
			slog.String("job", record.name),
			slog.String("job_id", record.id),
		)

		if err := runtime.queue.bury(ctx, record, newAttempts, runErr.Error()); err != nil {
			slog.ErrorContext(
				ctx, "bury failed job",
				slog.String("caller", "worker:executeQueued"),
				slog.String("job", record.name),
				slog.Any("err", err),
			)
		}

		return
	}

	if err := runtime.queue.retry(ctx, record.id, newAttempts); err != nil {
		slog.ErrorContext(
			ctx, "retry job",
			slog.String("caller", "worker:executeQueued"),
			slog.String("job", record.name),
			slog.Any("err", err),
		)
	}
}

func runJob(ctx context.Context, job Job) {
	runOnce(ctx, job, nil)

	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runOnce(ctx, job, nil)
		case <-ctx.Done():
			return
		}
	}
}

func runOnce(ctx context.Context, job Job, payload any) {
	slog.InfoContext(
		ctx, "job started",
		slog.String("caller", "worker:runOnce"),
		slog.String("job", job.Name),
	)

	start := time.Now()

	if err := job.Run(ctx, payload); err != nil {
		slog.ErrorContext(
			ctx, "job failed",
			slog.String("caller", "worker:runOnce"),
			slog.String("job", job.Name),
			slog.Any("err", err),
		)

		return
	}

	slog.InfoContext(
		ctx, "job done",
		slog.String("caller", "worker:runOnce"),
		slog.String("job", job.Name),
		slog.Duration("duration", time.Since(start)),
	)
}
