package main

import (
	"database/sql"
	"net/http"

	// kumacore:begin modules-imports
	"golang.org/x/crypto/bcrypt"
	authmiddleware "kumacore/app/middleware/auth"
	"kumacore/app/migrations"
	authmodule "kumacore/app/modules/auth"
	"kumacore/app/modules/errorscreen"
	healthmodule "kumacore/app/modules/health"
	"kumacore/app/modules/home"
	authrepository "kumacore/app/repositories/auth"
	authservice "kumacore/app/services/auth"
	"kumacore/core/cache"
	// kumacore:end modules-imports

	"kumacore/core/config"
	"kumacore/core/db/dialect"
	"kumacore/core/httpx"
	"kumacore/core/render"
)

type moduleBootstrap struct {
	middleware []func(http.Handler) http.Handler
	routes     []httpx.RouteRegistrar
}

func buildModules(
	configuration *config.Config,
	renderer render.Renderer,
	appDatabase *sql.DB,
	appDialect dialect.Dialect,
) (moduleBootstrap, error) {
	// kumacore:begin modules-setup
	authRepository := authrepository.NewRepository(appDatabase)
	userCache := cache.NewCache[string, authservice.CachedUser](configuration.Core.Session.CacheTTL)
	authService, err := authservice.NewService(
		authRepository,
		configuration.Core.Session.TTL,
		userCache,
		bcrypt.DefaultCost,
	)
	if err != nil {
		return moduleBootstrap{}, err
	}

	secureCookie := !configuration.Core.App.Dev
	homeHandler := home.NewHandler(renderer, configuration.App.Name)
	authHandler := authmodule.NewHandler(renderer, authService, secureCookie)
	notFoundHandler := errorscreen.NewHandler(renderer)

	healthHandler := healthmodule.NewHandler(
		appDatabase,
		appDialect,
		migrations.AppSource(),
	)
	// kumacore:end modules-setup

	return moduleBootstrap{
		middleware: []func(http.Handler) http.Handler{
			// kumacore:begin modules-middleware
			authmiddleware.LoadAuth(authService),
			// kumacore:end modules-middleware
		},
		routes: []httpx.RouteRegistrar{
			// kumacore:begin modules-routes
			home.Routes(homeHandler),
			authmodule.Routes(authHandler),
			healthmodule.Routes(healthHandler),
			errorscreen.Routes(notFoundHandler),
			// kumacore:end modules-routes
		},
	}, nil
}
