// Package worker provides a simple background job runner. Scheduled jobs (Interval > 0) first run one Interval after
// Start, then repeat at their interval. On-demand jobs (Interval == 0) are enqueued to the DB queue via Enqueue and
// executed by the poll loop, with automatic retry and graveyard on repeated failure.
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"kumacore/core/db/dialect"
	"kumacore/core/db/migrate"
)

// Job describes a unit of work the Runtime can execute. If Interval > 0 the job runs on a timer. If Interval == 0 it
// must be enqueued via Runtime.Enqueue and is executed by the poll loop.
type Job struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context, payload any) error
}

// Stats is a snapshot of the background queue stats.
type Stats struct {
	Pending         int
	Running         int
	Buried          int
	History         int
	OldestPendingAt int64
}

// Runtime manages a pool of background jobs and a queue-based execution loop.
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
	wg              *errgroup.Group
}

// NewRuntime creates a Runtime that polls the queue on the given interval and retries failed jobs up to maxAttempts
// times before burying them.
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

// Initialize validates and applies pending database migrations.
func (runtime *Runtime) Initialize(ctx context.Context) error {
	plan, err := migrate.Validate(ctx, runtime.database, runtime.dialect, runtime.migrationSource)
	if err != nil {
		return err
	}

	return migrate.ApplyPlan(ctx, runtime.database, plan)
}

// Register adds jobs to the runtime. Each job name must be unique.
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

// Start launches timer-based jobs and begins polling the queue.
func (runtime *Runtime) Start(ctx context.Context) {
	ctx, runtime.cancel = context.WithCancel(ctx)
	runtime.wg, ctx = errgroup.WithContext(ctx)

	slog.Info(
		"worker initialized",
		slog.String("caller", "worker:Start"),
		slog.Int("jobs", len(runtime.jobs)),
	)

	for _, job := range runtime.jobs {
		if job.Interval > 0 {
			runtime.wg.Go(func() error {
				runJob(ctx, job)
				return nil
			})
		}
	}

	runtime.wg.Go(func() error {
		runtime.pollLoop(ctx)
		return nil
	})
}

// Close cancels the runtime, waits for running jobs to finish, and closes the database.
func (runtime *Runtime) Close() error {
	if runtime.database == nil {
		return nil
	}

	if runtime.cancel != nil {
		runtime.cancel()
	}

	if runtime.wg != nil {
		_ = runtime.wg.Wait()
	}

	err := runtime.database.Close()
	runtime.database = nil

	return err
}

// Enqueue adds an on-demand job to the queue for execution by the poll loop.
func (runtime *Runtime) Enqueue(name string, payload any) error {
	if _, ok := runtime.jobMap[name]; !ok {
		return fmt.Errorf("[worker:Enqueue] unknown job: %s", name)
	}

	return runtime.queue.enqueue(name, payload)
}

// Stats returns a snapshot of the background queue.
func (runtime *Runtime) Stats(ctx context.Context) (Stats, error) {
	return runtime.queue.stats(ctx)
}

// pollLoop periodically claims pending jobs and processes them.
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

// processQueue claims pending jobs from the queue and executes each in its own goroutine.
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
		runtime.wg.Go(func() error {
			runtime.executeQueued(ctx, record)
			return nil
		})
	}
}

// executeQueued runs a single queued job, handling success, retry, and bury logic.
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

// runJob runs the job on its Interval ticker. The first run fires one Interval after Start, not immediately, so a server
// restart does not trigger a burst of scheduled jobs all at once.
func runJob(ctx context.Context, job Job) {
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

// runOnce executes the job's Run function and logs the outcome.
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
