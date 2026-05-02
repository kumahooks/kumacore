// Package auth implements a login page and auth service
package auth

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"kumacore/app/middleware/auth"
	"kumacore/app/services/auth"
	"kumacore/core/render"
)

// Handler handles login, logout, and authenticated user status requests.
type Handler struct {
	renderer     render.Renderer
	authService  *authservice.Service
	secureCookie bool
}

// NewHandler returns a Handler backed by the given auth service and renderer.
func NewHandler(renderer render.Renderer, authenticationService *authservice.Service, isDevelopment bool) *Handler {
	return &Handler{
		renderer:     renderer,
		authService:  authenticationService,
		secureCookie: !isDevelopment,
	}
}

// Routes registers auth routes on the given router.
func Routes(handler *Handler) func(chi.Router) {
	return func(router chi.Router) {
		router.Get("/auth", handler.Login)
		router.Post("/auth", handler.Authenticate)
		router.Post("/auth/logout", handler.Logout)

		router.With(authmiddleware.RequireAuth).Get("/auth/me", handler.Me)
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

	rawToken, expiresAt, err := handler.authService.Authenticate(request.Context(), username, password)
	if err != nil {
		if errors.Is(err, authservice.ErrInvalidCredentials) {
			writer.WriteHeader(http.StatusForbidden)
			handler.renderLogin(writer, request, "invalid credentials")

			return
		}

		log.Printf("[auth:Authenticate] authenticate: %v", err)

		writer.WriteHeader(http.StatusInternalServerError)
		handler.renderLogin(writer, request, "internal server error")

		return
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     "session",
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   handler.secureCookie,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(writer, request, "/auth/me", http.StatusSeeOther)
}

// Logout deletes the session, clears the cookie, and redirects to /.
func (handler *Handler) Logout(writer http.ResponseWriter, request *http.Request) {
	sessionCookie, err := request.Cookie("session")
	if err == nil {
		if err := handler.authService.Logout(request.Context(), sessionCookie.Value); err != nil {
			log.Printf("[auth:Logout] logout: %v", err)
		}
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   handler.secureCookie,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

// Me renders the authenticated user status page.
func (handler *Handler) Me(writer http.ResponseWriter, request *http.Request) {
	user, _ := authservice.AuthUser(request.Context())

	pageFile := "app/modules/auth/me.html"
	data := map[string]any{"Title": "auth/me", "User": user}

	if err := handler.renderer.Render(writer, request, pageFile, "page-content", data); err != nil {
		log.Printf("[auth:Me] render: %v", err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func (handler *Handler) renderLogin(writer http.ResponseWriter, request *http.Request, errorMessage string) {
	pageFile := "app/modules/auth/auth.html"
	data := map[string]any{"Title": "auth"}

	if errorMessage != "" {
		data["Error"] = errorMessage
	}

	if err := handler.renderer.Render(writer, request, pageFile, "page-content", data); err != nil {
		log.Printf("[auth:renderLogin] render: %v", err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}
