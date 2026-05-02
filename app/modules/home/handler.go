// Package home is a simple home page example.
package home

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"kumacore/core/render"
)

// Handler handles requests for the home page.
type Handler struct {
	renderer        render.Renderer
	applicationName string
}

// NewHandler returns a Handler backed by the given renderer.
func NewHandler(renderer render.Renderer, applicationName string) *Handler {
	return &Handler{renderer: renderer, applicationName: applicationName}
}

// Routes registers the home page route on the given router.
func Routes(handler *Handler) func(chi.Router) {
	return func(router chi.Router) {
		router.Get("/", handler.Index)
	}
}

// Index renders the home page.
func (handler *Handler) Index(writer http.ResponseWriter, request *http.Request) {
	pageFile := "app/modules/home/home.html"
	data := map[string]any{
		"Title":           handler.applicationName,
		"ApplicationName": handler.applicationName,
	}

	if err := handler.renderer.Render(writer, request, pageFile, "page-content", data); err != nil {
		log.Printf("[home:Index] render: %v", err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}
