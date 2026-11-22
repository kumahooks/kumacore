package main

import (
	"context"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	kumacoreapp "kumacore/app"
	"kumacore/app/migrations"
	"kumacore/core/app"
	"kumacore/core/config"
	"kumacore/core/render"
)

func main() {
	configuration, err := config.Load()
	if err != nil {
		log.Fatalf("[server:main] load config: %v", err)
	}

	if err = os.MkdirAll(configuration.Core.Backup.Dir, 0o755); err != nil {
		log.Fatalf("[server:main] create backup directory: %v", err)
	}

	var fileSystem fs.FS
	if configuration.Core.App.Dev {
		fileSystem = os.DirFS(filepath.Join(configuration.App.RootDir, "app"))
	} else {
		fileSystem = kumacoreapp.Templates
	}

	renderer, err := render.NewManager(configuration.Core.App.Dev, fileSystem, "modules/errorscreen/errorscreen.html")
	if err != nil {
		log.Fatalf("[server:main] initialize renderer: %v", err)
	}

	appDatabase, appDialect, err := openAppDatabase(configuration)
	if err != nil {
		log.Fatalf("[server:main] open app database: %v", err)
	}

	workerRuntime, err := buildWorkerBootstrap(configuration, appDatabase)
	if err != nil {
		log.Fatalf("[server:main] build worker runtime: %v", err)
	}

	modules, err := buildModules(configuration, renderer, appDatabase, appDialect)
	if err != nil {
		log.Fatalf("[server:main] build modules: %v", err)
	}

	options := app.Options{
		Configuration:   configuration,
		Database:        appDatabase,
		Dialect:         appDialect,
		Middleware:      modules.middleware,
		Routes:          modules.routes,
		Jobs:            workerRuntime.jobs,
		FileSystem:      fileSystem,
		StaticDir:       configuration.App.Static.Dir,
		MigrationSource: migrations.AppSource(),
		Renderer:        renderer,
	}

	if workerRuntime.runtime != nil {
		options.WorkerRuntime = workerRuntime.runtime
	}

	application, err := app.New(options)
	if err != nil {
		log.Fatalf("[server:main] create app: %v", err)
	}

	if err = application.Initialize(context.Background()); err != nil {
		log.Fatalf("[server:main] initialize app: %v", err)
	}

	if err = application.Start(application.Address()); err != nil {
		slog.Error(
			"start app",
			slog.String("caller", "server:main"),
			slog.Any("err", err),
		)

		os.Exit(1)
	}
}
