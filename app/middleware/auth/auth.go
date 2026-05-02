// Package authmiddleware contains HTTP middleware for session authentication.
package authmiddleware

import (
	"net/http"
	"time"

	"kumacore/app/services/auth"
)

// LoadAuth validates the session cookie against the database and injects the
// user into the request context if the session is valid and not expired.
// Always calls next regardless of auth state (use RequireAuth to enforce auth).
func LoadAuth(authenticationService *authservice.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			cookie, err := request.Cookie("session")
			if err != nil {
				next.ServeHTTP(writer, request)
				return
			}

			// TODO: I should at some point think about cache here,
			// otherwise every auth request will do this and it's kinda... eh
			user, err := authenticationService.UserForToken(request.Context(), cookie.Value, time.Now())
			if err != nil {
				next.ServeHTTP(writer, request)
				return
			}

			ctx := authservice.WithUser(request.Context(), user)
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

// RequireAuth redirects to / if no authenticated user is present.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := authservice.AuthUser(request.Context()); !ok {
			http.Redirect(writer, request, "/", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(writer, request)
	})
}
