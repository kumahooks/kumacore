// Package main starts kumacore.
package main

import (
	"context"
	"log"
	"os"

	// Core components
	"kumacore/core/app"
	"kumacore/core/config"
	"kumacore/core/module"
	"kumacore/core/render"

	// Modules
	"kumacore/modules/home"
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

	application, err := app.New(app.Options{
		Configuration: configuration,
		Modules: []module.Module{
			home.New(renderer, configuration.App.Name),
		},
		FileSystem: fileSystem,
		Renderer:   renderer,
	})
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
