package main

import (
	"database/sql"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	// kumacore:begin modules-imports
	authmiddleware "kumacore/app/middleware/auth"
	authmodule "kumacore/app/modules/auth"
	healthmodule "kumacore/app/modules/health"
	"kumacore/app/modules/home"
	authrepository "kumacore/app/repositories/auth"
	authservice "kumacore/app/services/auth"
	// kumacore:end modules-imports

	"kumacore/core/config"
	"kumacore/core/db/dialect"
	"kumacore/core/render"
)

type moduleBootstrap struct {
	middleware []func(http.Handler) http.Handler
	routes     []func(chi.Router)
}

func buildModules(
	configuration *config.Config,
	fileSystem fs.FS,
	renderer render.Renderer,
	appDatabase *sql.DB,
	appDialect dialect.Dialect,
) (moduleBootstrap, error) {
	_ = fileSystem
	_ = appDatabase
	_ = appDialect

	// kumacore:begin modules-setup
	authRepository := authrepository.NewRepository(appDatabase)
	authService, err := authservice.NewService(authRepository, configuration.Core.Session.TTL)
	if err != nil {
		return moduleBootstrap{}, err
	}

	homeHandler := home.NewHandler(renderer, configuration.App.Name)
	authHandler := authmodule.NewHandler(renderer, authService, configuration.Core.App.Dev)

	healthHandler := healthmodule.NewHandler(
		appDatabase,
		appDialect,
		newAppMigrationSource(fileSystem, appDialect),
	)
	// kumacore:end modules-setup

	return moduleBootstrap{
		middleware: []func(http.Handler) http.Handler{
			// kumacore:begin modules-middleware
			authmiddleware.LoadAuth(authService),
			// kumacore:end modules-middleware
		},
		routes: []func(chi.Router){
			// kumacore:begin modules-routes
			home.Routes(homeHandler),
			authmodule.Routes(authHandler),
			healthmodule.Routes(healthHandler),
			// kumacore:end modules-routes
		},
	}, nil
}
