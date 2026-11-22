package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"kumacore/core/httpx"
	logx "kumacore/core/log"
	"kumacore/core/security"
)

func (application *App) newRouter(appMiddlewares []func(http.Handler) http.Handler) *chi.Mux {
	middlewares := []func(http.Handler) http.Handler{
		logx.RequestIDMiddleware,
		logx.RequestLogger,
		chiMiddleware.Recoverer,
		httpx.LimitRequestBody(application.options.Configuration.Core.Server.MaxBodyBytes),
		security.Middleware,
		security.SameOriginMiddleware,
	}

	middlewares = append(middlewares, appMiddlewares...)

	return httpx.NewRouter(middlewares...)
}
