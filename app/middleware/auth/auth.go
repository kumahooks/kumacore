// Package authmiddleware contains HTTP middleware for session authentication.
package authmiddleware

import (
	"net/http"
	"time"

	authservice "kumacore/app/services/auth"
)

// LoadAuth validates the session cookie against the database and injects the
// user into the request context if the session is valid and not expired.
// Always calls next regardless of auth state (use RequireAuth to enforce auth).
func LoadAuth(authService *authservice.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			cookie, err := request.Cookie("session")
			if err != nil {
				next.ServeHTTP(writer, request)
				return
			}

			user, err := authService.UserForToken(request.Context(), cookie.Value, time.Now().UTC())
			if err != nil {
				next.ServeHTTP(writer, request)
				return
			}

			ctx := authservice.WithUser(request.Context(), user)
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

// RequireAuth wraps LoadAuth and redirects to / if no valid session is present.
func RequireAuth(authService *authservice.Service) func(http.Handler) http.Handler {
	load := LoadAuth(authService)

	return func(next http.Handler) http.Handler {
		return load(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := authservice.AuthUser(request.Context()); !ok {
				if request.Header.Get("HX-Request") == "true" {
					writer.Header().Set("HX-Redirect", "/")
					writer.WriteHeader(http.StatusUnauthorized)
					return
				}

				http.Redirect(writer, request, "/", http.StatusSeeOther)
				return
			}

			next.ServeHTTP(writer, request)
		}))
	}
}
