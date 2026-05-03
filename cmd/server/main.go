// Package main starts kumacore.
package main

import (
	"context"
	"log"
	"os"

	"kumacore/core/app"
	"kumacore/core/config"
	"kumacore/core/render"
)

func main() {
	configuration, err := config.Load()
	if err != nil {
		log.Fatalf("[server:main] load config: %v", err)
	}

	fileSystem := os.DirFS(configuration.App.RootDir)

	renderer, err := render.NewManager(configuration.Core.App.Dev, fileSystem)
	if err != nil {
		log.Fatalf("[server:main] initialize renderer: %v", err)
	}

	appDatabase, appDialect, err := openAppDatabase(configuration)
	if err != nil {
		log.Fatalf("[server:main] open app database: %v", err)
	}

	modules, err := buildModules(configuration, fileSystem, renderer, appDatabase, appDialect)
	if err != nil {
		log.Fatalf("[server:main] build modules: %v", err)
	}

	workerRuntime, err := buildWorkerBootstrap(configuration, fileSystem)
	if err != nil {
		log.Fatalf("[server:main] build worker bootstrap: %v", err)
	}

	options := app.Options{
		Configuration: configuration,
		Database:      appDatabase,
		Dialect:       appDialect,
		Middleware:    modules.middleware,
		Routes:        modules.routes,
		Jobs:          workerRuntime.jobs,
		FileSystem:    fileSystem,
		Renderer:      renderer,
	}

	if workerRuntime.runtime != nil {
		options.WorkerRuntime = workerRuntime.runtime
	}

	application, err := app.New(options)
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
