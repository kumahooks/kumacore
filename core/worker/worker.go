// Package worker provides a simple background job runner.
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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

		log.Printf("[worker:Register] registered %s to run every %s", job.Name, job.Interval)
	}

	return nil
}

// Start spawns scheduled jobs and starts the queue poll loop.
func (runtime *Runtime) Start(ctx context.Context) {
	ctx, runtime.cancel = context.WithCancel(ctx)

	log.Printf("[worker:Start] initialized (%d job(s))", len(runtime.jobs))

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
		log.Printf("[worker:processQueue] poll: %v", err)
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
		log.Printf("[worker:executeQueued] %s: unknown job (id=%s)", record.name, record.id)

		if err := runtime.queue.bury(
			ctx,
			record,
			record.attempts,
			fmt.Sprintf("unknown job: %s", record.name),
		); err != nil {
			log.Printf("[worker:executeQueued] %s: bury: %v", record.name, err)
		}

		return
	}

	var payload any
	if record.payload != nil {
		if err := json.Unmarshal([]byte(*record.payload), &payload); err != nil {
			log.Printf("[worker:executeQueued] %s: unmarshal payload: %v (id=%s)", record.name, err, record.id)

			if err = runtime.queue.bury(ctx, record, record.attempts, err.Error()); err != nil {
				log.Printf("[worker:executeQueued] %s: bury: %v", record.name, err)
			}

			return
		}
	}

	newAttempts := record.attempts + 1
	log.Printf("[worker:executeQueued] %s: started (attempt %d/%d)", record.name, newAttempts, runtime.maxAttempts)

	start := time.Now()

	runErr := job.Run(ctx, payload)
	if runErr == nil {
		log.Printf("[worker:executeQueued] %s: done (%s)", record.name, time.Since(start))

		if err := runtime.queue.complete(ctx, record, newAttempts); err != nil {
			log.Printf("[worker:executeQueued] %s: complete: %v", record.name, err)
		}

		return
	}

	log.Printf("[worker:executeQueued] %s: attempt %d failed: %v", record.name, newAttempts, runErr)

	if newAttempts >= runtime.maxAttempts {
		log.Printf("[worker:executeQueued] %s: max attempts reached, burying (id=%s)", record.name, record.id)

		if err := runtime.queue.bury(ctx, record, newAttempts, runErr.Error()); err != nil {
			log.Printf("[worker:executeQueued] %s: bury: %v", record.name, err)
		}

		return
	}

	if err := runtime.queue.retry(ctx, record.id, newAttempts); err != nil {
		log.Printf("[worker:executeQueued] %s: retry: %v", record.name, err)
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
	log.Printf("[worker:runOnce] %s: started", job.Name)
	start := time.Now()

	if err := job.Run(ctx, payload); err != nil {
		log.Printf("[worker:runOnce] %s: %v", job.Name, err)
		return
	}

	log.Printf("[worker:runOnce] %s: done (%s)", job.Name, time.Since(start))
}
