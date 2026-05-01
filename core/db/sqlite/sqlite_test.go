package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"kumacore/core/db/driver"
	"kumacore/core/db/sqlite"
)

func TestOpen_Succeeds(t *testing.T) {
	database, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	database.Close()
}

func TestOpen_FileDatabase_UsesWALMode(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	var journalMode string
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("journal_mode: got %q, want %q", journalMode, "wal")
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	database, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	_, err = database.Exec(`CREATE TABLE parents (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("create parents: %v", err)
	}

	_, err = database.Exec(`CREATE TABLE children (parent_id INTEGER NOT NULL REFERENCES parents(id))`)
	if err != nil {
		t.Fatalf("create children: %v", err)
	}

	_, err = database.Exec(`INSERT INTO children (parent_id) VALUES (?)`, 999)
	if err == nil {
		t.Fatal("insert child: got nil, want foreign key violation")
	}
}

func TestOpen_MemoryDatabase_UsesSingleConnection(t *testing.T) {
	database, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	_, err = database.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create items: %v", err)
	}

	_, err = database.Exec(`INSERT INTO items (name) VALUES (?)`, "lain")
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}

	var name string
	if err := database.QueryRow(`SELECT name FROM items WHERE id = ?`, 1).Scan(&name); err != nil {
		t.Fatalf("query item: %v", err)
	}

	if name != "lain" {
		t.Errorf("name: got %q, want %q", name, "lain")
	}
}

func TestOpen_ReturnsDBTXCompatibleDatabase(t *testing.T) {
	database, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	assertDBTX(t, database)
}

func TestTransaction_IsDBTXCompatible(t *testing.T) {
	database, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	transaction, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer transaction.Rollback()

	assertDBTX(t, transaction)
}

func assertDBTX(t *testing.T, database driver.DBTX) {
	t.Helper()

	if database == nil {
		t.Fatal("DBTX: got nil")
	}
}

var (
	_ driver.DBTX = (*sql.DB)(nil)
	_ driver.DBTX = (*sql.Tx)(nil)
)
