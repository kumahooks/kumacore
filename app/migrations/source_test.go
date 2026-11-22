package migrations_test

import (
	"context"
	"testing"

	"kumacore/app/migrations"
	coredb "kumacore/core/db"
	"kumacore/core/db/migrate"
	"kumacore/core/db/sqlite"
)

func TestWorkerSource_Applies(t *testing.T) {
	database, databaseDialect, err := coredb.Open(sqlite.DriverName, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := migrate.Apply(
		context.Background(),
		database,
		databaseDialect,
		migrations.WorkerSource(),
	); err != nil {
		t.Fatalf("apply worker migrations: %v", err)
	}

	var tableName string
	if err := database.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'job_queue'",
	).Scan(&tableName); err != nil {
		t.Fatalf("query job_queue table: %v", err)
	}

	if tableName != "job_queue" {
		t.Errorf("table name: got %q, want %q", tableName, "job_queue")
	}
}
