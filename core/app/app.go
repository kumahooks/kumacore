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
	"kumacore/core/module"
	"kumacore/core/render"
)

const (
	staticDirectory           = "web/static"
	defaultMigrationDirectory = "migrations/sqlite"
)

type WorkerRuntime interface {
	Register(jobs ...module.JobRegistrar) error
	Start(ctx context.Context)
}

type Options struct {
	Configuration   *config.Config
	Modules         []module.Module
	FileSystem      fs.FS
	StaticDir       string
	MigrationSource migrate.Source
	AuthMiddleware  func(http.Handler) http.Handler
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
	renderer *render.Manager
	logFile  io.Closer
}

// New creates an App with configured runtime dependencies.
func New(options Options) (*App, error) {
	if options.Configuration == nil {
		return nil, fmt.Errorf("[app:New] nil configuration")
	}

	fileSystem := options.FileSystem
	if fileSystem == nil {
		fileSystem = os.DirFS(options.Configuration.App.RootDir)
	}

	staticDir := options.StaticDir
	if staticDir == "" {
		staticDir = filepath.Join(options.Configuration.App.RootDir, staticDirectory)
	}

	options.Modules = append([]module.Module(nil), options.Modules...)
	options.FileSystem = fileSystem
	options.StaticDir = staticDir

	return &App{options: options}, nil
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
