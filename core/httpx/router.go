// Package httpx provides the core HTTP router contracts and wiring helpers.
package httpx

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

// Router is the route surface exposed to app-local modules.
type Router interface {
	Get(pattern string, handlerFunc http.HandlerFunc)
	Post(pattern string, handlerFunc http.HandlerFunc)
	Patch(pattern string, handlerFunc http.HandlerFunc)
	Delete(pattern string, handlerFunc http.HandlerFunc)
	Head(pattern string, handlerFunc http.HandlerFunc)
	Handle(pattern string, handler http.Handler)
	Use(middlewares ...func(http.Handler) http.Handler)
}

// RouteRegistrar follows the meryl.moe module Routes pattern.
type RouteRegistrar = func(chi.Router)

// NewRouter creates a Chi router and applies global middleware.
func NewRouter(middlewares ...func(http.Handler) http.Handler) *chi.Mux {
	router := chi.NewRouter()
	router.Use(middlewares...)

	return router
}

// RegisterRoutes delegates route ownership to app-local module registrars.
func RegisterRoutes(router chi.Router, registrars ...RouteRegistrar) {
	for _, register := range registrars {
		register(router)
	}
}

// RegisterStatic mounts static files and conventional metadata files.
func RegisterStatic(router chi.Router, staticDir string) {
	fileServer := http.FileServer(fileOnlyFS{fileSystem: http.Dir(staticDir)})
	router.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	router.Get("/robots.txt", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, filepath.Join(staticDir, "robots.txt"))
	})

	router.Get("/.well-known/security.txt", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, filepath.Join(staticDir, ".well-known", "security.txt"))
	})
}

// fileOnlyFS wraps http.FileSystem and rejects directory requests.
type fileOnlyFS struct {
	fileSystem http.FileSystem
}

func (fileOnly fileOnlyFS) Open(name string) (http.File, error) {
	file, err := fileOnly.fileSystem.Open(name)
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	if stat.IsDir() {
		file.Close()
		return nil, os.ErrNotExist
	}

	return file, nil
}
