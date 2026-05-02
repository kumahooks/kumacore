package db_test

import (
	"strings"
	"testing"

	"kumacore/core/db"
	"kumacore/core/db/sqlite"
)

func TestOpen_SQLite_ReturnsDatabaseAndDialect(t *testing.T) {
	database, databaseDialect, err := db.Open(sqlite.DriverName, ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if databaseDialect.Name() != sqlite.DriverName {
		t.Errorf("dialect name: got %q, want %q", databaseDialect.Name(), sqlite.DriverName)
	}
}

func TestOpen_UnsupportedDriver_ReturnsError(t *testing.T) {
	database, databaseDialect, err := db.Open("postgres", ":memory:")
	if err == nil {
		t.Fatal("Open error: got nil, want unsupported driver error")
	}

	if database != nil {
		t.Fatal("database: got non-nil, want nil")
	}

	if databaseDialect != nil {
		t.Fatal("dialect: got non-nil, want nil")
	}

	if !strings.Contains(err.Error(), `[database:Open] unsupported driver "postgres"`) {
		t.Fatalf("error: got %q, want unsupported driver message", err.Error())
	}
}
