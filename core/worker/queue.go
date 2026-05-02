package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type jobRecord struct {
	id       string
	name     string
	payload  *string
	attempts int
}

type jobQueue struct {
	repository *SQLQueueRepository
}

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

	now := time.Now().Unix()
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

func (queue *jobQueue) claimPending(ctx context.Context) ([]jobRecord, error) {
	records, err := queue.repository.ClaimPending(ctx, time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("[worker:claimPending] claim pending: %w", err)
	}

	return records, nil
}

func (queue *jobQueue) complete(ctx context.Context, record jobRecord, attempts int) error {
	if err := queue.repository.Complete(ctx, record, attempts, time.Now().Unix()); err != nil {
		return fmt.Errorf("[worker:complete] complete job: %w", err)
	}

	return nil
}

func (queue *jobQueue) retry(ctx context.Context, id string, attempts int) error {
	if err := queue.repository.Retry(ctx, id, attempts, time.Now().Unix()); err != nil {
		return fmt.Errorf("[worker:retry] retry job: %w", err)
	}

	return nil
}

func (queue *jobQueue) bury(ctx context.Context, record jobRecord, attempts int, lastError string) error {
	if err := queue.repository.Bury(ctx, record, attempts, lastError, time.Now().Unix()); err != nil {
		return fmt.Errorf("[worker:bury] bury job: %w", err)
	}

	return nil
}

func (queue *jobQueue) resetOrphaned(ctx context.Context) {
	affected, err := queue.repository.ResetOrphaned(ctx, time.Now().Unix())
	if err != nil {
		log.Printf("[worker:resetOrphaned] reset orphaned: %v", err)
		return
	}

	if affected > 0 {
		log.Printf("[worker:resetOrphaned] reset %d orphaned job(s)", affected)
	}
}
