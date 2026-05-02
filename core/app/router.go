package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"kumacore/core/httpx"
	"kumacore/core/security"
)

func (application *App) newRouter(appMiddlewares []func(http.Handler) http.Handler) *chi.Mux {
	middlewares := []func(http.Handler) http.Handler{
		chiMiddleware.Logger,
		chiMiddleware.Recoverer,
		security.Middleware,
	}

	middlewares = append(middlewares, appMiddlewares...)

	return httpx.NewRouter(middlewares...)
}
