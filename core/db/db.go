// Package db selects and opens the configured database adapter.
package db

import (
	"database/sql"
	"fmt"

	"kumacore/core/db/dialect"
	"kumacore/core/db/sqlite"
)

// Open opens a database for the configured driver and returns its dialect metadata.
func Open(driverName string, path string) (*sql.DB, dialect.Dialect, error) {
	if driverName != sqlite.DriverName {
		return nil, nil, fmt.Errorf("[database] unsupported driver %q", driverName)
	}

	database, err := sqlite.Open(path)
	if err != nil {
		return nil, nil, err
	}

	return database, sqlite.Dialect{}, nil
}
