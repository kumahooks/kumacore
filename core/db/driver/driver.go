// Package driver defines the DBTX interface for database operations.
package driver

import (
	"context"
	"database/sql"
)

// DBTX is the minimal shared database and transaction contract.
type DBTX interface {
	ExecContext(ctx context.Context, query string, arguments ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, arguments ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, arguments ...any) *sql.Row
}
