package app

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"

	"kumacore/core/config"
	coredb "kumacore/core/db"
	"kumacore/core/db/dialect"
	"kumacore/core/db/migrate"
	"kumacore/core/worker"
)

type testWorkerRuntime struct {
	registeredJobs []worker.Job
	initialized    bool
	started        bool
	closed         bool
}

func (runtime *testWorkerRuntime) Initialize(ctx context.Context) error {
	runtime.initialized = true
	return nil
}

func (runtime *testWorkerRuntime) Register(jobs ...worker.Job) error {
	runtime.registeredJobs = append(runtime.registeredJobs, jobs...)
	return nil
}

func (runtime *testWorkerRuntime) Start(ctx context.Context) {
	runtime.started = true
}

func (runtime *testWorkerRuntime) Close() error {
	runtime.closed = true
	return nil
}

func TestInitialize_RegistersRoutesMiddlewareAndWorkerJobs(t *testing.T) {
	configuration := testConfiguration(t)
	workerRuntime := &testWorkerRuntime{}
	database, databaseDialect := testDatabase(t)

	application, err := New(Options{
		Configuration: configuration,
		Database:      database,
		Dialect:       databaseDialect,
		Middleware: []func(http.Handler) http.Handler{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					writer.Header().Set("X-Test-Middleware", "registered")
					next.ServeHTTP(writer, request)
				})
			},
		},
		Routes: []func(chi.Router){
			func(router chi.Router) {
				router.Get("/", func(writer http.ResponseWriter, request *http.Request) {
					_, _ = writer.Write([]byte("ok"))
				})
			},
		},
		Jobs: []worker.Job{
			{Name: "test:job", Run: func(ctx context.Context, payload any) error { return nil }},
		},
		FileSystem:      testFileSystem(),
		MigrationSource: testMigrationSource(testFileSystem()),
		WorkerRuntime:   workerRuntime,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(application.close)

	if err := application.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	application.Router().ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", responseRecorder.Code, http.StatusOK)
	}

	if responseRecorder.Body.String() != "ok" {
		t.Fatalf("body: got %q, want %q", responseRecorder.Body.String(), "ok")
	}

	if responseRecorder.Header().Get("X-Test-Middleware") != "registered" {
		t.Fatalf("middleware header: got %q, want registered", responseRecorder.Header().Get("X-Test-Middleware"))
	}

	if len(workerRuntime.registeredJobs) != 1 {
		t.Fatalf("registered jobs: got %d, want 1", len(workerRuntime.registeredJobs))
	}

	if !workerRuntime.initialized {
		t.Fatal("worker not initialized during Initialize")
	}

	if workerRuntime.started {
		t.Fatal("worker started during Initialize")
	}
}

func TestNew_NilDatabaseReturnsError(t *testing.T) {
	configuration := testConfiguration(t)

	_, err := New(Options{
		Configuration: configuration,
	})
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("New: got %v, want nil database error", err)
	}
}

func TestInitialize_MigrationFailureClosesDatabase(t *testing.T) {
	configuration := testConfiguration(t)
	database, databaseDialect := testDatabase(t)

	application, err := New(Options{
		Configuration: configuration,
		Database:      database,
		Dialect:       databaseDialect,
		FileSystem:    testFileSystem(),
		MigrationSource: migrate.Source{
			Backend: "sqlite",
			FileSystem: fstest.MapFS{
				"app/migrations/sqlite/app/0002_create_widgets.sql": &fstest.MapFile{
					Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);"),
				},
			},
			Directory: "app/migrations/sqlite/app",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = application.Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), "sequence hole") {
		t.Fatalf("Initialize: got %v, want sequence hole", err)
	}

	if application.runtime.database != nil {
		t.Fatal("database: got open handle, want nil after close")
	}
}

func TestStart_UninitializedAppReturnsError(t *testing.T) {
	database, databaseDialect := testDatabase(t)

	application, err := New(Options{
		Configuration: testConfiguration(t),
		Database:      database,
		Dialect:       databaseDialect,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = application.Start("127.0.0.1:0")
	if err == nil || !strings.Contains(err.Error(), "app is not initialized") {
		t.Fatalf("Start: got %v, want uninitialized error", err)
	}
}

func testConfiguration(t *testing.T) *config.Config {
	t.Helper()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
	})

	var configuration config.Config
	configuration.Core.DB.Driver = "sqlite"
	configuration.Core.DB.Path = ":memory:"
	configuration.Core.Logging.Dir = filepath.Join(t.TempDir(), "logs")
	configuration.App.Name = "test"
	configuration.App.RootDir = "."

	return &configuration
}

func testDatabase(t *testing.T) (*sql.DB, dialect.Dialect) {
	t.Helper()

	database, databaseDialect, err := coredb.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	t.Cleanup(func() { _ = database.Close() })

	return database, databaseDialect
}

func testFileSystem() fstest.MapFS {
	return fstest.MapFS{
		"app/web/templates/layouts/base.html": &fstest.MapFile{
			Data: []byte(`{{define "base"}}{{template "page-content" .}}{{end}}`),
		},
		"app/web/templates/components/navbar.html": &fstest.MapFile{
			Data: []byte(`{{define "navbar"}}{{end}}`),
		},
		"app/migrations/sqlite/app/0001_create_widgets.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE widgets (id INTEGER PRIMARY KEY);`),
		},
	}
}

func testMigrationSource(fileSystem fstest.MapFS) migrate.Source {
	return migrate.Source{
		Backend:    "sqlite",
		FileSystem: fileSystem,
		Directory:  "app/migrations/sqlite/app",
	}
}
