// Package sqlite manages SQLite database connections.
package sqlite

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

// Open opens or creates a SQLite database at path and applies connection pragmas.
func Open(path string) (*sql.DB, error) {
	dsnOptions, err := dsnOptionsWithPragmas(path)
	if err != nil {
		return nil, err
	}

	database, err := sql.Open(DriverName, dsnOptions)
	if err != nil {
		return nil, fmt.Errorf("[database:sqlite:Open] open: %w", err)
	}

	// SQLite :memory: creates one isolated DB per connection. Keep a single pooled connection so schema/data remain consistent
	// in tests.
	if path == ":memory:" || strings.HasPrefix(path, "file::memory:") {
		database.SetMaxOpenConns(1)
	}

	slog.Info(
		"opened database",
		slog.String("caller", "database:sqlite:Open"),
		slog.String("path", path),
	)
	slog.Debug(
		"configured via DSN pragmas (WAL, foreign keys)",
		slog.String("caller", "database:sqlite:Open"),
	)

	return database, nil
}

// dsnOptionsWithPragmas injects per-connection PRAGMA settings via modernc DSN params.
//
// journal_mode=WAL: switches from the default rollback journal to write-ahead-log. In default mode a write locks the
// entire file, blocking all readers. In WAL mode readers do not block writers and a writer does not block readers. ref:
// https://sqlite.org/wal.html
//
// synchronous=NORMAL: under WAL, this skips the per-commit fsync of the WAL and only fsyncs at checkpoint. The default
// (FULL) fsyncs on every commit, which serializes writers and produces long tails when a commit lands during a checkpoint.
// NORMAL trades a small theoretical durability window (a power loss can lose the last few committed transactions, but
// not corrupt the DB) for a large write-throughput gain.
// ref: https://sqlite.org/pragma.html#pragma_synchronous
//
// foreign_keys=ON: SQLite does not enforce foreign key constraints by default.
// ref: https://sqlite.org/foreignkeys.html
//
// busy_timeout=5000: WAL still serializes writers. When two connections try to write at the same time, SQLite returns
// SQLITE_BUSY unless a busy_timeout is set, in which case it blocks and retries internally until the lock is released
// or the timeout elapses.
// ref: https://sqlite.org/c3ref/busy_handler.html
func dsnOptionsWithPragmas(path string) (string, error) {
	basePath := path
	rawQuery := ""
	if before, after, hasQuery := strings.Cut(path, "?"); hasQuery {
		basePath = before
		rawQuery = after
	}

	queryValues, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("[database:sqlite:dsnOptionsWithPragmas] parse dsn query: %w", err)
	}

	queryValues.Add("_pragma", "journal_mode(WAL)")
	queryValues.Add("_pragma", "synchronous(NORMAL)")
	queryValues.Add("_pragma", "foreign_keys(ON)")
	queryValues.Add("_pragma", "busy_timeout(5000)")

	return basePath + "?" + queryValues.Encode(), nil
}
