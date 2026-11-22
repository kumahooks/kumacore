// Package db manages the SQLite database connection.
package db

import (
	"database/sql"
	"fmt"

	"kumacore/core/db/dialect"
	"kumacore/core/db/sqlite"
)

func Open(driverName string, path string) (*sql.DB, dialect.Dialect, error) {
	if driverName != sqlite.DriverName {
		return nil, nil, fmt.Errorf("[database:Open] unsupported driver %q", driverName)
	}

	database, err := sqlite.Open(path)
	if err != nil {
		return nil, nil, err
	}

	return database, sqlite.Dialect{}, nil
}
