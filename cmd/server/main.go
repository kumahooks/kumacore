// Package main starts kumacore.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	jobslogging "kumacore/app/jobs/logging"
	authmiddleware "kumacore/app/middleware/auth"
	authmodule "kumacore/app/modules/auth"
	healthmodule "kumacore/app/modules/health"
	"kumacore/app/modules/home"
	authrepository "kumacore/app/repositories/auth"
	authservice "kumacore/app/services/auth"
	"kumacore/core/app"
	"kumacore/core/config"
	"kumacore/core/db"
	"kumacore/core/db/migrate"
	"kumacore/core/render"
	"kumacore/core/worker"
)

// TODO: this initialization is getting too convoluted
// i need to think of a better generalized way to initialize each one
func main() {
	configuration, err := config.Load()
	if err != nil {
		log.Fatalf("[server:main] load config: %v", err)
	}

	// Renderer Initialization
	fileSystem := os.DirFS(configuration.App.RootDir)
	renderer, err := render.NewManager(configuration.Core.App.Dev, fileSystem)
	if err != nil {
		log.Fatalf("[server:main] initialize renderer: %v", err)
	}

	// Database Initialization
	if err = os.MkdirAll(filepath.Dir(configuration.Core.DB.Path), 0o755); err != nil {
		log.Fatalf("[server:main] create database directory: %v", err)
	}

	appDatabase, appDialect, err := db.Open(configuration.Core.DB.Driver, configuration.Core.DB.Path)
	if err != nil {
		log.Fatalf("[server:main] open database: %v", err)
	}

	// Services Initialization
	authRepository := authrepository.NewRepository(appDatabase)
	authService, err := authservice.NewService(authRepository, configuration.Core.Session.TTL)
	if err != nil {
		log.Fatalf("[server:main] create auth service: %v", err)
	}

	// Handlers Initialization
	homeHandler := home.NewHandler(renderer, configuration.App.Name)
	authHandler := authmodule.NewHandler(renderer, authService, configuration.Core.App.Dev)

	// Health endpoint uses the app migration source for readiness checks.
	healthHandler := healthmodule.NewHandler(
		appDatabase,
		appDialect,
		migrate.Source{
			Backend:    appDialect.Name(),
			FileSystem: fileSystem,
			Directory:  "app/migrations/sqlite/app",
		},
	)

	// Worker Initialization
	var workerRuntime *worker.Runtime
	if configuration.Core.Worker.Enabled {
		if err = os.MkdirAll(filepath.Dir(configuration.Core.Worker.DBPath), 0o755); err != nil {
			log.Fatalf("[server:main] create worker database directory: %v", err)
		}

		workerDatabase, workerDialect, err := db.Open(configuration.Core.DB.Driver, configuration.Core.Worker.DBPath)
		if err != nil {
			log.Fatalf("[server:main] open worker database: %v", err)
		}

		workerRuntime, err = worker.NewRuntime(
			workerDatabase,
			workerDialect,
			migrate.Source{
				Backend:    workerDialect.Name(),
				FileSystem: fileSystem,
				Directory:  "app/migrations/sqlite/worker",
			},
			configuration.Core.Worker.PollInterval,
			configuration.Core.Worker.MaxAttempts,
		)
		if err != nil {
			log.Fatalf("[server:main] create worker runtime: %v", err)
		}
	}

	opts := app.Options{
		Configuration: configuration,
		Database:      appDatabase,
		Dialect:       appDialect,
		Middleware: []func(next http.Handler) http.Handler{
			authmiddleware.LoadAuth(authService),
		},
		Routes: []func(router chi.Router){
			home.Routes(homeHandler),
			authmodule.Routes(authHandler),
			healthmodule.Routes(healthHandler),
		},
		Jobs: []worker.Job{
			{
				Name:     "logging:cleanup",
				Interval: 24 * time.Hour,
				Run: func(ctx context.Context, _ any) error {
					return jobslogging.Cleanup(ctx, configuration.Core.Logging.Dir)
				},
			},
		},
		FileSystem: fileSystem,
		Renderer:   renderer,
	}

	if workerRuntime != nil {
		opts.WorkerRuntime = workerRuntime
	}

	application, err := app.New(opts)
	if err != nil {
		log.Fatalf("[server:main] create app: %v", err)
	}

	if err := application.Initialize(context.Background()); err != nil {
		log.Fatalf("[server:main] initialize app: %v", err)
	}

	if err := application.Start(application.Address()); err != nil {
		log.Fatalf("[server:main] start app: %v", err)
	}
}
