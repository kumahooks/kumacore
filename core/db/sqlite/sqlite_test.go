package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"kumacore/core/db/driver"
)

func TestOpen_Succeeds(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	database.Close()
}

func TestOpen_FileDatabase_UsesWALMode(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
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
	database, err := Open(":memory:")
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

func TestOpen_BusyTimeoutConfigured(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	var busyTimeout int
	if err := database.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}

	if busyTimeout != 5000 {
		t.Errorf("busy_timeout: got %d, want %d", busyTimeout, 5000)
	}
}

func TestOpen_MemoryDatabase_UsesSingleConnection(t *testing.T) {
	database, err := Open(":memory:")
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
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	assertDBTX(t, database)
}

func TestTransaction_IsDBTXCompatible(t *testing.T) {
	database, err := Open(":memory:")
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

func TestBackupTo_CreatesConsistentSnapshot(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	defer source.Close()

	if _, err = source.Exec("CREATE TABLE marker(value TEXT)"); err != nil {
		t.Fatalf("create marker table: %v", err)
	}

	if _, err = source.Exec("INSERT INTO marker VALUES ('lain')"); err != nil {
		t.Fatalf("insert marker: %v", err)
	}

	destinationPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err = BackupTo(context.Background(), source, destinationPath); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}

	destination, err := Open(destinationPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer destination.Close()

	var value string
	if err := destination.QueryRow("SELECT value FROM marker").Scan(&value); err != nil {
		t.Fatalf("query marker: %v", err)
	}

	if value != "lain" {
		t.Errorf("marker: got %q, want %q", value, "lain")
	}
}
