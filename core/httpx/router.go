// Package httpx provides the core HTTP router contracts and wiring helpers.
package httpx

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

// RouteRegistrar is a function that registers routes on a chi.Router.
// Each module exposes a Routes function returning this type so that
// route paths are owned by the module rather than the central wiring layer.
//
// Type alias, not a new type, so callers can return func(chi.Router)
// without importing this package.
type RouteRegistrar = func(chi.Router)

// NewRouter creates a Chi router and applies global middleware.
func NewRouter(middlewares ...func(http.Handler) http.Handler) *chi.Mux {
	router := chi.NewRouter()
	router.Use(middlewares...)

	return router
}

// RegisterRoutes delegates all page routes to the provided registrars.
// Each module registers its own paths via Routes.
func RegisterRoutes(router chi.Router, registrars ...RouteRegistrar) {
	for _, register := range registrars {
		register(router)
	}
}

// RegisterStatic mounts static files and conventional metadata files.
func RegisterStatic(router chi.Router, staticDir string) {
	staticFileHandler := newStaticFileHandler(staticDir)

	router.Get("/static/*", staticFileHandler)
	router.Head("/static/*", staticFileHandler)

	router.Get("/robots.txt", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, filepath.Join(staticDir, "robots.txt"))
	})

	router.Get("/.well-known/security.txt", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, filepath.Join(staticDir, ".well-known", "security.txt"))
	})
}

// newStaticFileHandler serves files from staticDir.
//
// This handler sets cache validators and content metadata for static files.
func newStaticFileHandler(staticDir string) http.HandlerFunc {
	staticFileSystem := fileOnlyFS{fileSystem: http.Dir(staticDir)}

	return func(writer http.ResponseWriter, request *http.Request) {
		filePath := chi.URLParam(request, "*")
		file, err := staticFileSystem.Open(filePath)
		if err != nil {
			http.NotFound(writer, request)
			return
		}
		defer file.Close()

		fileInfo, err := file.Stat()
		if err != nil {
			http.NotFound(writer, request)
			return
		}

		modificationTime := fileInfo.ModTime()
		entityTag := calculateStaticEntityTag(fileInfo)

		responseHeader := writer.Header()
		responseHeader.Set("Cache-Control", "no-cache")
		responseHeader.Add("Vary", "Accept-Encoding")
		if entityTag != "" {
			responseHeader.Set("Etag", entityTag)
		}

		if responseHeader.Get("Content-Type") == "" {
			mimeType := mime.TypeByExtension(filepath.Ext(filePath))
			if mimeType == "" {
				// Go will sniff the content-type: https://www.youtube.com/watch?v=8t8JYpt0egE
				responseHeader["Content-Type"] = nil
			} else {
				responseHeader.Set("Content-Type", mimeType)
			}
		}

		http.ServeContent(writer, request, fileInfo.Name(), modificationTime, file)
	}
}

func calculateStaticEntityTag(fileInfo os.FileInfo) string {
	modificationTime := fileInfo.ModTime()
	if modificationTimeUnix := modificationTime.Unix(); modificationTimeUnix == 0 || modificationTimeUnix == 1 {
		return ""
	}

	var entityTagBuilder strings.Builder
	entityTagBuilder.WriteRune('"')
	entityTagBuilder.WriteString(strconv.FormatInt(modificationTime.UnixNano(), 36))
	entityTagBuilder.WriteString(strconv.FormatInt(fileInfo.Size(), 36))
	entityTagBuilder.WriteRune('"')

	return entityTagBuilder.String()
}

// fileOnlyFS wraps http.FileSystem and rejects directory requests,
// preventing directory listing on the static file server.
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
