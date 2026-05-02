// Package sqlite manages SQLite database connections.
package sqlite

import (
	"database/sql"
	"fmt"
	"log"
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

	// SQLite :memory: creates one isolated DB per connection.
	// Keep a single pooled connection so schema/data remain consistent in tests.
	if path == ":memory:" || strings.HasPrefix(path, "file::memory:") {
		database.SetMaxOpenConns(1)
	}

	log.Printf("[database:sqlite:Open] opened %s", path)
	log.Printf("[database:sqlite:Open] configured via DSN pragmas (WAL, foreign keys)")

	return database, nil
}

// dsnOptionsWithPragmas injects per-connection PRAGMA settings via modernc DSN params.
//
// journal_mode=WAL: switches from the default rollback journal to write-ahead-log.
// In default mode a write locks the entire file, blocking all readers.
// In WAL mode readers do not block writers and a writer does not block readers.
// ref: https://sqlite.org/wal.html
//
// foreign_keys=ON: SQLite does not enforce foreign key constraints by default.
// ref: https://sqlite.org/foreignkeys.html
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
	queryValues.Add("_pragma", "foreign_keys(ON)")

	return basePath + "?" + queryValues.Encode(), nil
}
