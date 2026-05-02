package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"kumacore/core/db/sqlite"
)

func TestParseMigrationFilename(t *testing.T) {
	sequence, err := parseMigrationFilename("0001_create_users.sql")
	if err != nil {
		t.Fatalf("parseMigrationFilename: %v", err)
	}

	if sequence != 1 {
		t.Fatalf("sequence: got %d, want 1", sequence)
	}

	for _, filename := range []string{
		"001_create_users.sql",
		"0000_create_users.sql",
		"0001.sql",
		"0001_create_users.txt",
		"nested/0001_create_users.sql",
	} {
		if _, err := parseMigrationFilename(filename); err == nil {
			t.Fatalf("parseMigrationFilename(%q): got nil error, want failure", filename)
		}
	}
}

func TestApply_AppliesPendingMigrations(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
		"migrations/sqlite/0002_create_sessions.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE sessions (id INTEGER PRIMARY KEY);`),
		},
	}

	err := Apply(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	records := readAppliedMigrations(t, database)
	if len(records) != 2 {
		t.Fatalf("applied migration count: got %d, want 2", len(records))
	}

	if record := records[1]; record.filename != "0001_create_users.sql" {
		t.Fatalf("record 1 filename: got %q, want %q", record.filename, "0001_create_users.sql")
	}

	if record := records[2]; record.filename != "0002_create_sessions.sql" {
		t.Fatalf("record 2 filename: got %q, want %q", record.filename, "0002_create_sessions.sql")
	}

	firstChecksum := checksumForFile(t, filesystem, "migrations/sqlite/0001_create_users.sql")
	if record := records[1]; record.checksumSHA256 != firstChecksum {
		t.Fatalf("record 1 checksum: got %q, want %q", record.checksumSHA256, firstChecksum)
	}
}

func TestValidate_ReturnsPlanWithoutExecution(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
	}

	plan, err := Validate(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if plan.PendingCount() != 1 {
		t.Fatalf("pending count: got %d, want 1", plan.PendingCount())
	}

	if tableExists(t, database, trackingTableName) {
		t.Fatal("schema_migrations table: got present, want absent")
	}
}

func TestValidate_RejectsBackendMismatch(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
	}

	source := migrationSource(filesystem)
	source.Backend = "postgres"

	_, err := Validate(context.Background(), database, sqlite.Dialect{}, source)
	if err == nil || !strings.Contains(err.Error(), "backend mismatch") {
		t.Fatalf("Validate backend mismatch: got %v, want backend mismatch", err)
	}

	if tableExists(t, database, trackingTableName) {
		t.Fatal("schema_migrations table: got present, want absent")
	}
}

func TestApplyPlan_AppliesValidatedPlan(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
	}

	plan, err := Validate(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if err := ApplyPlan(context.Background(), database, plan); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}

	if count := appliedMigrationCount(t, database); count != 1 {
		t.Fatalf("applied migration count: got %d, want 1", count)
	}
}

func TestApply_RejectsTamperedAppliedMigrationBeforeExecution(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
	}

	err := Apply(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err != nil {
		t.Fatalf("initial Apply: %v", err)
	}

	filesystem["migrations/sqlite/0001_create_users.sql"].Data = []byte(
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
	)

	err = Apply(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Apply tampered migration: got %v, want checksum mismatch", err)
	}

	if count := appliedMigrationCount(t, database); count != 1 {
		t.Fatalf("applied migration count: got %d, want 1", count)
	}
}

func TestApply_RejectsSequenceHoleBeforeExecution(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
		"migrations/sqlite/0003_create_sessions.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE sessions (id INTEGER PRIMARY KEY);`),
		},
	}

	err := Apply(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err == nil || !strings.Contains(err.Error(), "sequence hole") {
		t.Fatalf("Apply hole: got %v, want sequence hole", err)
	}

	if tableExists(t, database, trackingTableName) {
		t.Fatal("schema_migrations table: got present, want absent")
	}
}

func TestApply_RejectsDuplicateSequenceBeforeExecution(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
		"migrations/sqlite/0001_create_sessions.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE sessions (id INTEGER PRIMARY KEY);`),
		},
	}

	err := Apply(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err == nil || !strings.Contains(err.Error(), "duplicate migration sequence") {
		t.Fatalf("Apply duplicate: got %v, want duplicate sequence", err)
	}

	if tableExists(t, database, trackingTableName) {
		t.Fatal("schema_migrations table: got present, want absent")
	}
}

func TestApply_RollsBackAllPendingMigrationsWhenExecutionFails(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
		"migrations/sqlite/0002_create_sessions.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE sessions (`),
		},
	}

	err := Apply(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err == nil || !strings.Contains(err.Error(), "run 0002") {
		t.Fatalf("Apply invalid migration: got %v, want run 0002 failure", err)
	}

	if tableExists(t, database, "users") {
		t.Fatal("users table: got present, want absent")
	}

	if tableExists(t, database, trackingTableName) {
		t.Fatal("schema_migrations table: got present, want absent")
	}
}

func TestApply_RejectsAppliedSequenceHole(t *testing.T) {
	database := mustOpenDatabase(t)
	defer database.Close()

	mustCreateTrackingTable(t, database)
	mustInsertAppliedMigration(t, database, 1, "0001_create_users.sql", "checksum-one")
	mustInsertAppliedMigration(t, database, 3, "0003_create_sessions.sql", "checksum-three")

	filesystem := fstest.MapFS{
		"migrations/sqlite/0001_create_users.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`),
		},
		"migrations/sqlite/0002_create_sessions.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE sessions (id INTEGER PRIMARY KEY);`),
		},
		"migrations/sqlite/0003_create_roles.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE roles (id INTEGER PRIMARY KEY);`),
		},
	}

	err := Apply(context.Background(), database, sqlite.Dialect{}, migrationSource(filesystem))
	if err == nil || !strings.Contains(err.Error(), "applied sequence hole") {
		t.Fatalf("Apply applied hole: got %v, want applied sequence hole", err)
	}
}

func migrationSource(filesystem fstest.MapFS) Source {
	return Source{
		Backend:    sqlite.DriverName,
		FileSystem: filesystem,
		Directory:  "migrations/sqlite",
	}
}

func mustOpenDatabase(t *testing.T) *sql.DB {
	t.Helper()

	database, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}

	return database
}

func readAppliedMigrations(t *testing.T, database *sql.DB) map[int]appliedMigration {
	t.Helper()

	rows, err := database.Query(
		fmt.Sprintf(
			"SELECT sequence, filename, checksum_sha256, applied_at_unix FROM %s ORDER BY sequence",
			trackingTableName,
		),
	)
	if err != nil {
		t.Fatalf("query applied migrations: %v", err)
	}
	defer rows.Close()

	records := make(map[int]appliedMigration)
	for rows.Next() {
		var record appliedMigration
		if err := rows.Scan(
			&record.sequence,
			&record.filename,
			&record.checksumSHA256,
			&record.appliedAtUnix,
		); err != nil {
			t.Fatalf("scan applied migration: %v", err)
		}

		records[record.sequence] = record
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("iterate applied migrations: %v", err)
	}

	return records
}

func checksumForFile(t *testing.T, filesystem fstest.MapFS, filename string) string {
	t.Helper()

	fileContents, err := fs.ReadFile(filesystem, filename)
	if err != nil {
		t.Fatalf("read file %q: %v", filename, err)
	}

	fileChecksum := sha256.Sum256(fileContents)
	return hex.EncodeToString(fileChecksum[:])
}

func appliedMigrationCount(t *testing.T, database *sql.DB) int {
	t.Helper()

	var count int
	if err := database.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", trackingTableName)).Scan(&count); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}

	return count
}

func tableExists(t *testing.T, database *sql.DB, tableName string) bool {
	t.Helper()

	var existingTableName string
	err := database.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
		tableName,
	).Scan(&existingTableName)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("table exists query: %v", err)
	}

	return true
}

func mustCreateTrackingTable(t *testing.T, database *sql.DB) {
	t.Helper()

	_, err := database.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			sequence INTEGER NOT NULL PRIMARY KEY,
			filename TEXT NOT NULL,
			checksum_sha256 TEXT NOT NULL,
			applied_at_unix INTEGER NOT NULL
		)
	`, trackingTableName))
	if err != nil {
		t.Fatalf("create tracking table: %v", err)
	}
}

func mustInsertAppliedMigration(
	t *testing.T,
	database *sql.DB,
	sequence int,
	filename string,
	checksumSHA256 string,
) {
	t.Helper()

	_, err := database.Exec(
		fmt.Sprintf(
			"INSERT INTO %s (sequence, filename, checksum_sha256, applied_at_unix) VALUES (?, ?, ?, ?)",
			trackingTableName,
		),
		sequence,
		filename,
		checksumSHA256,
		1,
	)
	if err != nil {
		t.Fatalf("insert applied migration: %v", err)
	}
}
