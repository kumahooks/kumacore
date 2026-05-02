package app

import (
	"context"
	"errors"
	"io/fs"
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
	"kumacore/core/db/migrate"
	"kumacore/core/module"
)

type testModule struct {
	id       string
	register func(module.Registrar) error
}

func (testModule testModule) ID() string {
	return testModule.id
}

func (testModule testModule) Register(registrar module.Registrar) error {
	if testModule.register == nil {
		return nil
	}

	return testModule.register(registrar)
}

type testWorkerRuntime struct {
	registeredJobs []module.JobRegistrar
	started        bool
}

func (runtime *testWorkerRuntime) Register(jobs ...module.JobRegistrar) error {
	runtime.registeredJobs = append(runtime.registeredJobs, jobs...)
	return nil
}

func (runtime *testWorkerRuntime) Start(ctx context.Context) {
	runtime.started = true
}

func TestInitialize_RegistersRoutesMiddlewareAndWorkerJobs(t *testing.T) {
	configuration := testConfiguration(t)
	workerRuntime := &testWorkerRuntime{}

	application, err := New(Options{
		Configuration: configuration,
		Modules: []module.Module{
			testModule{
				id: "home",
				register: func(registrar module.Registrar) error {
					registrar.Middleware(func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
							writer.Header().Set("X-Test-Middleware", "registered")
							next.ServeHTTP(writer, request)
						})
					})

					registrar.Routes(func(router chi.Router) {
						router.Get("/", func(writer http.ResponseWriter, request *http.Request) {
							_, _ = writer.Write([]byte("ok"))
						})
					})

					registrar.Jobs(module.JobRegistrar{Name: "test:job"})
					return nil
				},
			},
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

	if workerRuntime.started {
		t.Fatal("worker started during Initialize")
	}
}

func TestInitialize_InvalidModuleListAbortsBeforeDatabaseOpen(t *testing.T) {
	configuration := testConfiguration(t)
	configuration.Core.DB.Path = filepath.Join(t.TempDir(), "data", "app.db")

	application, err := New(Options{
		Configuration: configuration,
		Modules: []module.Module{
			testModule{id: "home"},
			testModule{id: "home"},
		},
		FileSystem:      testFileSystem(),
		MigrationSource: testMigrationSource(testFileSystem()),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = application.Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), `duplicate module ID "home"`) {
		t.Fatalf("Initialize: got %v, want duplicate module ID error", err)
	}

	if _, err := os.Stat(filepath.Dir(configuration.Core.DB.Path)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("database dir stat: got %v, want not exist", err)
	}
}

func TestInitialize_MigrationFailureClosesDatabase(t *testing.T) {
	configuration := testConfiguration(t)

	application, err := New(Options{
		Configuration: configuration,
		Modules: []module.Module{
			testModule{id: "home"},
		},
		FileSystem: testFileSystem(),
		MigrationSource: migrate.Source{
			Backend: "sqlite",
			FileSystem: fstest.MapFS{
				"app/migrations/sqlite/0002_create_widgets.sql": &fstest.MapFile{
					Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);"),
				},
			},
			Directory: "app/migrations/sqlite",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = application.Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), "sequence hole") {
		t.Fatalf("Initialize: got %v, want sequence hole", err)
	}

	if application.runtime.database == nil {
		t.Fatal("database: got nil, want opened database")
	}

	if err := application.runtime.database.PingContext(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("database ping: got %v, want closed database", err)
	}
}

func TestStart_UninitializedAppReturnsError(t *testing.T) {
	application, err := New(Options{Configuration: testConfiguration(t)})
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

func testFileSystem() fstest.MapFS {
	return fstest.MapFS{
		"app/web/templates/layouts/base.html": &fstest.MapFile{
			Data: []byte(`{{define "base"}}{{template "page-content" .}}{{end}}`),
		},
		"app/web/templates/components/navbar.html": &fstest.MapFile{
			Data: []byte(`{{define "navbar"}}{{end}}`),
		},
		"app/migrations/sqlite/0001_create_widgets.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE widgets (id INTEGER PRIMARY KEY);`),
		},
	}
}

func testMigrationSource(fileSystem fstest.MapFS) migrate.Source {
	return migrate.Source{
		Backend:    "sqlite",
		FileSystem: fileSystem,
		Directory:  "app/migrations/sqlite",
	}
}
