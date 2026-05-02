package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	coredb "kumacore/core/db"
	"kumacore/core/db/migrate"
)

const workerMigrationSQL = `
CREATE TABLE job_queue (
	id         TEXT    PRIMARY KEY,
	name       TEXT    NOT NULL,
	payload    TEXT,
	attempts   INTEGER NOT NULL DEFAULT 0,
	status     TEXT    NOT NULL DEFAULT 'pending',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE job_history (
	id           TEXT    PRIMARY KEY,
	name         TEXT    NOT NULL,
	payload      TEXT,
	attempts     INTEGER NOT NULL,
	completed_at INTEGER NOT NULL
);

CREATE TABLE job_graveyard (
	id         TEXT    PRIMARY KEY,
	name       TEXT    NOT NULL,
	payload    TEXT,
	attempts   INTEGER NOT NULL,
	last_error TEXT    NOT NULL,
	buried_at  INTEGER NOT NULL
);
`

func TestRuntime_Register_JobsAvailableAfterStart(t *testing.T) {
	runtime := newTestRuntime(t)

	if err := runtime.Register(
		Job{Name: "test:alpha", Run: func(ctx context.Context, payload any) error { return nil }},
		Job{Name: "test:beta", Run: func(ctx context.Context, payload any) error { return nil }},
	); err != nil {
		t.Fatalf("Register: %v", err)
	}

	runtime.Start(context.Background())

	if err := runtime.TriggerJob("test:alpha", nil); err != nil {
		t.Errorf("test:alpha not found after Start: %v", err)
	}

	if err := runtime.TriggerJob("test:beta", nil); err != nil {
		t.Errorf("test:beta not found after Start: %v", err)
	}
}

func TestRuntime_Start_ScheduledJobRunsImmediately(t *testing.T) {
	runtime := newTestRuntime(t)
	executed := make(chan struct{}, 1)

	if err := runtime.Register(Job{
		Name:     "test:scheduled",
		Interval: time.Hour,
		Run: func(ctx context.Context, payload any) error {
			select {
			case executed <- struct{}{}:
			default:
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	runtime.Start(t.Context())

	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Error("scheduled job did not run immediately on Start")
	}
}

func TestRuntime_Start_OnDemandJobDoesNotRunAutomatically(t *testing.T) {
	runtime := newTestRuntime(t)
	executed := make(chan struct{}, 1)

	if err := runtime.Register(Job{
		Name: "test:ondemand",
		Run: func(ctx context.Context, payload any) error {
			executed <- struct{}{}
			return nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	runtime.Start(t.Context())

	select {
	case <-executed:
		t.Error("on-demand job ran automatically after Start")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestQueue_RetryThenComplete_MovesToHistory(t *testing.T) {
	runtime := newTestRuntime(t)

	var attempts int32
	if err := runtime.Register(Job{
		Name: "test:lain",
		Run: func(ctx context.Context, payload any) error {
			currentAttempt := atomic.AddInt32(&attempts, 1)
			if currentAttempt < 3 {
				return errors.New("temporary failure")
			}

			return nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := runtime.Enqueue("test:lain", "payload"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	processQueueOnceSynchronously(t, runtime, context.Background())
	processQueueOnceSynchronously(t, runtime, context.Background())
	processQueueOnceSynchronously(t, runtime, context.Background())

	var queueCount int
	if err := runtime.database.QueryRow("SELECT COUNT(*) FROM job_queue").Scan(&queueCount); err != nil {
		t.Fatalf("count queue rows: %v", err)
	}

	if queueCount != 0 {
		t.Errorf("job_queue count: got %d, want 0", queueCount)
	}

	var historyAttempts int
	if err := runtime.database.QueryRow("SELECT attempts FROM job_history WHERE name = 'test:lain'").
		Scan(&historyAttempts); err != nil {
		t.Fatalf("query history attempts: %v", err)
	}

	if historyAttempts != 3 {
		t.Errorf("history attempts: got %d, want 3", historyAttempts)
	}
}

func TestQueue_MaxAttempts_BuriesJob(t *testing.T) {
	runtime := newTestRuntime(t)

	if err := runtime.Register(Job{
		Name: "test:always-fail",
		Run: func(ctx context.Context, payload any) error {
			return errors.New("permanent failure")
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := runtime.Enqueue("test:always-fail", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	processQueueOnceSynchronously(t, runtime, context.Background())
	processQueueOnceSynchronously(t, runtime, context.Background())
	processQueueOnceSynchronously(t, runtime, context.Background())

	var queueCount int
	if err := runtime.database.QueryRow("SELECT COUNT(*) FROM job_queue").Scan(&queueCount); err != nil {
		t.Fatalf("count queue rows: %v", err)
	}

	if queueCount != 0 {
		t.Errorf("job_queue count: got %d, want 0", queueCount)
	}

	var graveyardAttempts int
	var lastError string
	if err := runtime.database.QueryRow(
		"SELECT attempts, last_error FROM job_graveyard WHERE name = 'test:always-fail'",
	).Scan(&graveyardAttempts, &lastError); err != nil {
		t.Fatalf("query graveyard row: %v", err)
	}

	if graveyardAttempts != 3 {
		t.Errorf("graveyard attempts: got %d, want 3", graveyardAttempts)
	}

	if lastError != "permanent failure" {
		t.Errorf("graveyard error: got %q, want %q", lastError, "permanent failure")
	}
}

func TestQueue_ResetOrphaned(t *testing.T) {
	runtime := newTestRuntime(t)

	_, err := runtime.database.Exec(
		"INSERT INTO job_queue (id, name, attempts, status, created_at, updated_at) VALUES ('job-1', 'test:job', 1, 'running', 1, 1)",
	)
	if err != nil {
		t.Fatalf("insert running job: %v", err)
	}

	runtime.queue.resetOrphaned(context.Background())

	var status string
	if err := runtime.database.QueryRow("SELECT status FROM job_queue WHERE id = 'job-1'").Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}

	if status != "pending" {
		t.Errorf("status: got %q, want pending", status)
	}
}

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()

	database, databaseDialect, err := coredb.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	t.Cleanup(func() { _ = database.Close() })

	runtime, err := NewRuntime(
		database,
		databaseDialect,
		migrate.Source{
			Backend: databaseDialect.Name(),
			FileSystem: fstest.MapFS{
				"app/migrations/sqlite/worker/0001_create_worker_tables.sql": &fstest.MapFile{
					Data: []byte(workerMigrationSQL),
				},
			},
			Directory: "app/migrations/sqlite/worker",
		},
		50*time.Millisecond,
		3,
	)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := runtime.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	return runtime
}

func processQueueOnceSynchronously(t *testing.T, runtime *Runtime, ctx context.Context) {
	t.Helper()

	records, err := runtime.queue.claimPending(ctx)
	if err != nil {
		t.Fatalf("claim pending: %v", err)
	}

	for _, record := range records {
		runtime.executeQueued(ctx, record)
	}
}
