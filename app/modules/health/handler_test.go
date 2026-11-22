package health_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"kumacore/app/modules/health"
	coredb "kumacore/core/db"
	"kumacore/core/db/dialect"
	"kumacore/core/db/migrate"
	"kumacore/core/db/sqlite"
)

func newTestDatabase(t *testing.T) (*sql.DB, dialect.Dialect) {
	t.Helper()

	database, databaseDialect, err := coredb.Open(sqlite.DriverName, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	return database, databaseDialect
}

func testMigrationSource() migrate.Source {
	return migrate.Source{
		Backend: sqlite.DriverName,
		FileSystem: fstest.MapFS{
			"migrations/0001_create_widgets.sql": &fstest.MapFile{
				Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);"),
			},
		},
		Directory: "migrations",
	}
}

func TestLive_ReturnsOK(t *testing.T) {
	database, databaseDialect := newTestDatabase(t)
	handler := health.NewHandler(database, databaseDialect, testMigrationSource())

	recorder := httptest.NewRecorder()
	handler.Live(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if body := recorder.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("body: got %q, want status ok", body)
	}
}

func TestReady_AppliedMigrationsReturnsOK(t *testing.T) {
	database, databaseDialect := newTestDatabase(t)
	source := testMigrationSource()

	if err := migrate.Apply(context.Background(), database, databaseDialect, source); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	handler := health.NewHandler(database, databaseDialect, source)

	recorder := httptest.NewRecorder()
	handler.Ready(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if body := recorder.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("body: got %q, want status ok", body)
	}
}

func TestReady_PendingMigrationReturnsServiceUnavailable(t *testing.T) {
	database, databaseDialect := newTestDatabase(t)
	handler := health.NewHandler(database, databaseDialect, testMigrationSource())

	recorder := httptest.NewRecorder()
	handler.Ready(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	if body := recorder.Body.String(); !strings.Contains(body, `"migrations":"pending"`) {
		t.Errorf("body: got %q, want pending migrations", body)
	}
}

func TestReady_InvalidMigrationReturnsServiceUnavailable(t *testing.T) {
	database, databaseDialect := newTestDatabase(t)
	source := migrate.Source{
		Backend: sqlite.DriverName,
		FileSystem: fstest.MapFS{
			"migrations/0002_create_widgets.sql": &fstest.MapFile{
				Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);"),
			},
		},
		Directory: "migrations",
	}
	handler := health.NewHandler(database, databaseDialect, source)

	recorder := httptest.NewRecorder()
	handler.Ready(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	if body := recorder.Body.String(); !strings.Contains(body, `"migrations":"invalid"`) {
		t.Errorf("body: got %q, want invalid migrations", body)
	}
}
