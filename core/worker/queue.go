package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// jobRecord represents a single row from the job_queue table.
type jobRecord struct {
	id       string
	name     string
	payload  *string
	attempts int
}

// jobQueue provides high-level operations over the queue repository.
type jobQueue struct {
	repository *SQLQueueRepository
}

// enqueue marshals the payload and inserts a new pending job.
func (queue *jobQueue) enqueue(name string, payload any) error {
	var encodedPayload *string
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("[worker:enqueue] marshal payload: %w", err)
		}

		serialized := string(encoded)
		encodedPayload = &serialized
	}

	now := time.Now().UTC().Unix()
	if err := queue.repository.Enqueue(
		context.Background(),
		uuid.New().String(),
		name,
		encodedPayload,
		now,
	); err != nil {
		return fmt.Errorf("[worker:enqueue] insert job: %w", err)
	}

	return nil
}

// claimPending marks all pending jobs as running and returns them.
func (queue *jobQueue) claimPending(ctx context.Context) ([]jobRecord, error) {
	records, err := queue.repository.ClaimPending(ctx, time.Now().UTC().Unix())
	if err != nil {
		return nil, fmt.Errorf("[worker:claimPending] claim pending: %w", err)
	}

	return records, nil
}

// complete removes the job from the queue and records it in the history table.
func (queue *jobQueue) complete(ctx context.Context, record jobRecord, attempts int) error {
	if err := queue.repository.Complete(ctx, record, attempts, time.Now().UTC().Unix()); err != nil {
		return fmt.Errorf("[worker:complete] complete job: %w", err)
	}

	return nil
}

// retry resets a failed job back to pending with incremented attempts.
func (queue *jobQueue) retry(ctx context.Context, id string, attempts int) error {
	if err := queue.repository.Retry(ctx, id, attempts, time.Now().UTC().Unix()); err != nil {
		return fmt.Errorf("[worker:retry] retry job: %w", err)
	}

	return nil
}

// bury moves a permanently failed job into the graveyard and deletes it from the queue.
func (queue *jobQueue) bury(ctx context.Context, record jobRecord, attempts int, lastError string) error {
	if err := queue.repository.Bury(ctx, record, attempts, lastError, time.Now().UTC().Unix()); err != nil {
		return fmt.Errorf("[worker:bury] bury job: %w", err)
	}

	return nil
}

// stats returns queue counts and the oldest pending job's Unix creation time.
func (queue *jobQueue) stats(ctx context.Context) (Stats, error) {
	stats, err := queue.repository.Stats(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("[worker:stats] queue stats: %w", err)
	}

	return stats, nil
}

// resetOrphaned marks any stuck running jobs back to pending.
func (queue *jobQueue) resetOrphaned(ctx context.Context) {
	affected, err := queue.repository.ResetOrphaned(ctx, time.Now().UTC().Unix())
	if err != nil {
		slog.ErrorContext(
			ctx, "reset orphaned jobs",
			slog.String("caller", "worker:resetOrphaned"),
			slog.Any("err", err),
		)

		return
	}

	if affected > 0 {
		slog.InfoContext(
			ctx, "reset orphaned jobs",
			slog.String("caller", "worker:resetOrphaned"),
			slog.Int64("count", affected),
		)
	}
}
