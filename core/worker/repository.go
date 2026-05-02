package worker

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
)

//go:embed queries/*.sql
var workerQueryFiles embed.FS

var (
	enqueueJobQuery      = mustReadQuery(workerQueryFiles, "queries/enqueue_job.sql")
	claimPendingQuery    = mustReadQuery(workerQueryFiles, "queries/claim_pending.sql")
	insertHistoryQuery   = mustReadQuery(workerQueryFiles, "queries/insert_history.sql")
	deleteQueueJobQuery  = mustReadQuery(workerQueryFiles, "queries/delete_queue_job.sql")
	retryJobQuery        = mustReadQuery(workerQueryFiles, "queries/retry_job.sql")
	insertGraveyardQuery = mustReadQuery(workerQueryFiles, "queries/insert_graveyard.sql")
	resetOrphanedQuery   = mustReadQuery(workerQueryFiles, "queries/reset_orphaned.sql")
)

type SQLQueueRepository struct {
	database *sql.DB
}

func newSQLQueueRepository(database *sql.DB) *SQLQueueRepository {
	return &SQLQueueRepository{database: database}
}

func (repository *SQLQueueRepository) Enqueue(
	ctx context.Context,
	id string,
	name string,
	payload *string,
	createdAtUnix int64,
) error {
	_, err := repository.database.ExecContext(ctx, enqueueJobQuery, id, name, payload, createdAtUnix, createdAtUnix)

	return err
}

func (repository *SQLQueueRepository) ClaimPending(ctx context.Context, updatedAtUnix int64) ([]jobRecord, error) {
	rows, err := repository.database.QueryContext(ctx, claimPendingQuery, updatedAtUnix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []jobRecord
	for rows.Next() {
		var record jobRecord
		if err := rows.Scan(&record.id, &record.name, &record.payload, &record.attempts); err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func (repository *SQLQueueRepository) Complete(
	ctx context.Context,
	record jobRecord,
	attempts int,
	completedAtUnix int64,
) error {
	transaction, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := transaction.ExecContext(
		ctx,
		insertHistoryQuery,
		record.id,
		record.name,
		record.payload,
		attempts,
		completedAtUnix,
	); err != nil {
		transaction.Rollback()
		return err
	}

	if _, err := transaction.ExecContext(ctx, deleteQueueJobQuery, record.id); err != nil {
		transaction.Rollback()
		return err
	}

	return transaction.Commit()
}

func (repository *SQLQueueRepository) Retry(ctx context.Context, id string, attempts int, updatedAtUnix int64) error {
	_, err := repository.database.ExecContext(ctx, retryJobQuery, attempts, updatedAtUnix, id)

	return err
}

func (repository *SQLQueueRepository) Bury(
	ctx context.Context,
	record jobRecord,
	attempts int,
	lastError string,
	buriedAtUnix int64,
) error {
	transaction, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := transaction.ExecContext(
		ctx,
		insertGraveyardQuery,
		record.id,
		record.name,
		record.payload,
		attempts,
		lastError,
		buriedAtUnix,
	); err != nil {
		transaction.Rollback()
		return err
	}

	if _, err := transaction.ExecContext(ctx, deleteQueueJobQuery, record.id); err != nil {
		transaction.Rollback()
		return err
	}

	return transaction.Commit()
}

func (repository *SQLQueueRepository) ResetOrphaned(ctx context.Context, updatedAtUnix int64) (int64, error) {
	result, err := repository.database.ExecContext(ctx, resetOrphanedQuery, updatedAtUnix)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func mustReadQuery(fileSystem fs.FS, name string) string {
	queryBytes, err := fs.ReadFile(fileSystem, name)
	if err != nil {
		panic(fmt.Sprintf("[worker:mustReadQuery] read %s: %v", name, err))
	}

	return string(queryBytes)
}
