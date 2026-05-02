// Package app provides the core HTTP application lifecycle using Chi router.
// It handles server initialization, route registration, static file serving,
// migration execution, and graceful shutdown.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"kumacore/core/config"
	"kumacore/core/db/dialect"
	"kumacore/core/db/migrate"
	"kumacore/core/httpx"
	"kumacore/core/render"
	"kumacore/core/worker"
)

const (
	staticDirectory           = "app/web/static"
	defaultMigrationDirectory = "app/migrations/sqlite/app"
)

type WorkerRuntime interface {
	Initialize(ctx context.Context) error
	Register(jobs ...worker.Job) error
	Start(ctx context.Context)
	Close() error
}

type Options struct {
	Configuration   *config.Config
	Database        *sql.DB
	Dialect         dialect.Dialect
	// Caller must not mutate these slices after passing to New.
	Middleware      []func(http.Handler) http.Handler
	Routes          []httpx.RouteRegistrar
	Jobs            []worker.Job
	FileSystem      fs.FS
	StaticDir       string
	MigrationSource migrate.Source
	Renderer        render.Renderer
	WorkerRuntime   WorkerRuntime
}

type App struct {
	options Options
	runtime runtime
}

type runtime struct {
	router   *chi.Mux
	database *sql.DB
	dialect  dialect.Dialect
	renderer render.Renderer
	logFile  io.Closer
}

// New creates an App with configured runtime dependencies.
func New(options Options) (*App, error) {
	if options.Configuration == nil {
		return nil, fmt.Errorf("[app:New] nil configuration")
	}

	if options.Database == nil {
		return nil, fmt.Errorf("[app:New] nil database")
	}

	if options.Dialect == nil {
		return nil, fmt.Errorf("[app:New] nil dialect")
	}

	fileSystem := options.FileSystem
	if fileSystem == nil {
		fileSystem = os.DirFS(options.Configuration.App.RootDir)
	}

	staticDir := options.StaticDir
	if staticDir == "" {
		staticDir = filepath.Join(options.Configuration.App.RootDir, staticDirectory)
	}

	options.FileSystem = fileSystem
	options.StaticDir = staticDir

	return &App{
		options: options,
		runtime: runtime{
			database: options.Database,
			dialect:  options.Dialect,
		},
	}, nil
}

func (application *App) Address() string {
	return fmt.Sprintf(
		"%s:%d",
		application.options.Configuration.Core.Server.Host,
		application.options.Configuration.Core.Server.Port,
	)
}

func (application *App) Router() http.Handler {
	return application.runtime.router
}

func (application *App) Database() *sql.DB {
	return application.runtime.database
}

func (application *App) Dialect() dialect.Dialect {
	return application.runtime.dialect
}

func (application *App) Renderer() render.Renderer {
	return application.runtime.renderer
}
