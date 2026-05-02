package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"kumacore/core/httpx"
	"kumacore/core/module"
	"kumacore/core/security"
)

func (application *App) newRouter(moduleMiddlewares []module.MiddlewareRegistrar) *chi.Mux {
	middlewares := []func(http.Handler) http.Handler{
		chiMiddleware.Logger,
		chiMiddleware.Recoverer,
		security.Middleware,
	}

	if application.options.AuthMiddleware != nil {
		middlewares = append(middlewares, application.options.AuthMiddleware)
	}

	middlewares = append(middlewares, moduleMiddlewares...)

	return httpx.NewRouter(middlewares...)
}
