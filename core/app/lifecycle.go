package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kumacore/core/httpx"
	logx "kumacore/core/log"
	"kumacore/core/render"
)

// Initialize builds the template manager, runs migrations, and wires middleware and routes.
func (application *App) Initialize(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	writer, logFile, err := application.initLogging()
	if err != nil {
		return err
	}

	application.runtime.logFile = logFile

	logx.Setup(writer, application.options.Configuration.Core.App.Dev)
	log.SetOutput(writer)

	slog.Info("logging initialized", slog.String("caller", "app:Initialize"))

	if err := application.runMigrations(ctx); err != nil {
		application.close()
		return err
	}

	if application.options.WorkerRuntime != nil {
		if err := application.options.WorkerRuntime.Initialize(ctx); err != nil {
			application.close()
			return fmt.Errorf("[app:Initialize] initialize worker: %w", err)
		}

		if err := application.options.WorkerRuntime.Register(application.options.Jobs...); err != nil {
			application.close()
			return fmt.Errorf("[app:Initialize] register worker jobs: %w", err)
		}
	}

	if application.options.Renderer != nil {
		application.runtime.renderer = application.options.Renderer
	} else {
		templateManager, err := render.NewManager(
			application.options.Configuration.Core.App.Dev,
			application.options.FileSystem,
			application.options.ErrorTemplatePath,
		)
		if err != nil {
			application.close()
			return fmt.Errorf("[app:Initialize] initialize renderer: %w", err)
		}

		application.runtime.renderer = templateManager
	}

	slog.Info(
		"templates initialized",
		slog.String("caller", "app:Initialize"),
		slog.Bool("dev", application.options.Configuration.Core.App.Dev),
	)

	application.runtime.router = application.newRouter(application.options.Middleware)

	httpx.RegisterStatic(application.runtime.router, application.options.StaticDir)
	httpx.RegisterRoutes(application.runtime.router, application.options.Routes...)

	slog.Info("routes registered", slog.String("caller", "app:Initialize"))

	return nil
}

// Start binds the server to addr and begins serving requests. Blocks until SIGINT or SIGTERM is received, then drains
// in-flight requests with a 30-second deadline before returning.
func (application *App) Start(addr string) error {
	if application.runtime.router == nil {
		return fmt.Errorf("[app:Start] app is not initialized")
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           application.runtime.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if application.options.WorkerRuntime != nil && application.options.Configuration.Core.Worker.Enabled {
		application.options.WorkerRuntime.Start(shutdownContext)
		slog.Info("worker started", slog.String("caller", "app:Start"))
	}
	defer application.close()

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info(
			"server starting",
			slog.String("caller", "app:Start"),
			slog.String("addr", addr),
		)

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-shutdownContext.Done():
		stop()

		slog.Info("server shutting down", slog.String("caller", "app:Start"))

		drainContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		return httpServer.Shutdown(drainContext)
	}
}

func (application *App) close() {
	if application.options.WorkerRuntime != nil {
		application.options.WorkerRuntime.Close()
	}

	if application.runtime.logFile != nil {
		application.runtime.logFile.Close()
		application.runtime.logFile = nil
	}

	if application.runtime.database != nil {
		application.runtime.database.Close()
		application.runtime.database = nil
	}
}
