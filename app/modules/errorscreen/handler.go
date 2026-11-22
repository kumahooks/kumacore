// Package errorscreen renders the 404 not-found screen. The module's only route is registered on the router's NotFound
// path and delegates to render.Manager.RenderError, which uses the same errorscreen.html template. NotFound is a thin
// wrapper over RenderError that fixes the 404 message ("ls").
package errorscreen

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"kumacore/core/render"
)

// Handler renders the 404 not-found page.
type Handler struct {
	renderer render.Renderer
}

// NewHandler returns a Handler backed by the given renderer.
func NewHandler(renderer render.Renderer) *Handler {
	return &Handler{renderer: renderer}
}

// Routes registers the 404 handler on the router's NotFound path.
func Routes(handler *Handler) func(chi.Router) {
	return func(router chi.Router) {
		router.NotFound(handler.NotFound)
	}
}

// NotFound renders the 404 page. It delegates to Renderer.RenderError so all error rendering shares one implementation
// (status header, fallback, landmark preservation).
func (handler *Handler) NotFound(writer http.ResponseWriter, request *http.Request) {
	handler.renderer.RenderError(writer, request, http.StatusNotFound, "ls")
}
