package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kumacore/core/httpx"
	"kumacore/core/module"
	"kumacore/core/render"
)

// Initialize builds the template manager and wires module middleware and routes.
func (application *App) Initialize(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	logFile, err := application.initLogging()
	if err != nil {
		return err
	}
	application.runtime.logFile = logFile

	log.Printf("[app:Initialize] logging initialized")

	registry, err := module.NewRegistry(application.options.Modules...)
	if err != nil {
		application.close()
		return err
	}

	resolvedModules, err := registry.Resolve()
	if err != nil {
		application.close()
		return err
	}

	if err := application.openDatabase(); err != nil {
		application.close()
		return err
	}

	contributions, err := module.CollectContributions(resolvedModules...)
	if err != nil {
		application.close()
		return err
	}

	if application.options.WorkerRuntime != nil {
		if err := application.options.WorkerRuntime.Register(contributions.JobRegistrars()...); err != nil {
			application.close()
			return fmt.Errorf("[app:Initialize] register worker jobs: %w", err)
		}
	}

	if err := application.runMigrations(ctx); err != nil {
		application.close()
		return err
	}

	templateManager, err := render.NewManager(
		application.options.Configuration.Core.App.Dev,
		application.options.FileSystem,
	)
	if err != nil {
		application.close()
		return fmt.Errorf("[app:Initialize] initialize renderer: %w", err)
	}
	application.runtime.renderer = templateManager

	log.Printf("[app:Initialize] templates initialized (dev=%v)", application.options.Configuration.Core.App.Dev)

	application.runtime.router = application.newRouter(contributions.MiddlewareRegistrars())

	httpx.RegisterStatic(application.runtime.router, application.options.StaticDir)
	httpx.RegisterRoutes(application.runtime.router, contributions.RouteRegistrars()...)

	log.Printf("[app:Initialize] routes registered")

	return nil
}

// Start binds the server to addr and begins serving requests.
// Blocks until SIGINT or SIGTERM is received, then drains in-flight requests
// with a 30-second deadline before returning.
func (application *App) Start(addr string) error {
	if application.runtime.router == nil {
		return fmt.Errorf("[app:Start] app is not initialized")
	}
	defer application.close()

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           application.runtime.router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if application.options.WorkerRuntime != nil && application.options.Configuration.Core.Worker.Enabled {
		application.options.WorkerRuntime.Start(shutdownContext)
		log.Printf("[app:Start] worker started")
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("[app:Start] server starting on %s", addr)

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-shutdownContext.Done():
		stop()

		log.Printf("[app:Start] server shutting down")

		drainContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		return httpServer.Shutdown(drainContext)
	}
}

func (application *App) close() {
	if application.runtime.logFile != nil {
		application.runtime.logFile.Close()
	}

	if application.runtime.database != nil {
		application.runtime.database.Close()
	}
}
