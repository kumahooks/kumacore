// Package auth implements the login, logout, and session management handlers.
package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	authmiddleware "kumacore/app/middleware/auth"
	authservice "kumacore/app/services/auth"
	"kumacore/core/httpx"
	"kumacore/core/render"
)

// Handler handles login, logout, and session creation.
type Handler struct {
	renderer     render.Renderer
	authService  *authservice.Service
	secureCookie bool
}

// NewHandler returns a Handler backed by the given auth service and renderer.
func NewHandler(
	renderer render.Renderer,
	authService *authservice.Service,
	secureCookie bool,
) *Handler {
	return &Handler{
		renderer:     renderer,
		authService:  authService,
		secureCookie: secureCookie,
	}
}

// Routes registers the login, logout, and session creation routes on the given router.
func Routes(handler *Handler) func(chi.Router) {
	return func(router chi.Router) {
		// Public
		router.Get("/auth", handler.Login)
		router.Post("/auth", handler.Authenticate)

		// Authenticated
		router.With(authmiddleware.RequireAuth(handler.authService)).Post("/auth/logout", handler.Logout)
		router.With(authmiddleware.RequireAuth(handler.authService)).Get("/auth/me", handler.Me)
	}
}

// Login renders the login form.
func (handler *Handler) Login(writer http.ResponseWriter, request *http.Request) {
	handler.renderLogin(writer, request, "")
}

// Authenticate validates credentials, creates a session, and redirects to /auth/me.
func (handler *Handler) Authenticate(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 1024)
	if err := request.ParseForm(); err != nil {
		writer.WriteHeader(http.StatusForbidden)
		handler.renderLogin(writer, request, "invalid credentials")

		return
	}

	username := request.FormValue("username")
	password := request.FormValue("password")
	if username == "" || password == "" {
		writer.WriteHeader(http.StatusForbidden)
		handler.renderLogin(writer, request, "invalid credentials")

		return
	}

	_, rawToken, expiresAt, err := handler.authService.Authenticate(request.Context(), username, password)
	if err != nil {
		if errors.Is(err, authservice.ErrInvalidCredentials) {
			writer.WriteHeader(http.StatusForbidden)
			handler.renderLogin(writer, request, "invalid credentials")
			return
		}

		slog.ErrorContext(
			request.Context(), "authenticate",
			slog.String("caller", "auth:Authenticate"),
			slog.Any("err", err),
		)

		writer.WriteHeader(http.StatusInternalServerError)
		handler.renderLogin(writer, request, "internal server error")

		return
	}

	httpx.SetCookie(writer, "session", rawToken, expiresAt, handler.secureCookie)
	http.Redirect(writer, request, "/auth/me", http.StatusSeeOther)
}

// Logout deletes the session cookie and redirects to the login page.
func (handler *Handler) Logout(writer http.ResponseWriter, request *http.Request) {
	sessionCookie, err := request.Cookie("session")
	if err == nil {
		if err := handler.authService.Logout(request.Context(), sessionCookie.Value); err != nil {
			slog.ErrorContext(
				request.Context(), "delete session",
				slog.String("caller", "auth:Logout"),
				slog.Any("err", err),
			)
		}
	}

	httpx.ClearCookie(writer, "session", handler.secureCookie)

	if request.Header.Get("HX-Request-Type") == "partial" {
		writer.Header().Set("HX-Redirect", "/auth")
		writer.WriteHeader(http.StatusOK)

		return
	}

	http.Redirect(writer, request, "/auth", http.StatusSeeOther)
}

// Me renders the authenticated user status page.
func (handler *Handler) Me(writer http.ResponseWriter, request *http.Request) {
	user, _ := authservice.AuthUser(request.Context())

	pageFile := "modules/auth/me.html"
	data := map[string]any{"Title": "auth/me", "User": user}

	if err := handler.renderer.Render(writer, request, pageFile, "page-content", data); err != nil {
		slog.ErrorContext(
			request.Context(), "render",
			slog.String("caller", "auth:Me"),
			slog.Any("err", err),
		)

		http.Error(writer, "internal server error", http.StatusInternalServerError)
	}
}

// renderLogin renders the login form with an optional error message.
func (handler *Handler) renderLogin(writer http.ResponseWriter, request *http.Request, errorMessage string) {
	pageFile := "modules/auth/auth.html"
	data := map[string]any{"Title": "auth"}

	if errorMessage != "" {
		data["Error"] = errorMessage
	}

	if err := handler.renderer.Render(writer, request, pageFile, "page-content", data); err != nil {
		slog.ErrorContext(
			request.Context(), "render",
			slog.String("caller", "auth:renderLogin"),
			slog.Any("err", err),
		)

		http.Error(writer, "internal server error", http.StatusInternalServerError)
	}
}
