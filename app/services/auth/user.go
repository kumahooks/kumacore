package authservice

import "context"

type authContextKey struct{}

// User holds the authenticated user's identity injected into the request context.
type User struct {
	ID          string
	Username    string
	Permissions int64
}

// WithUser returns a new context with the given User stored under the auth key.
func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, authContextKey{}, user)
}

// AuthUser returns the authenticated User from the request context.
func AuthUser(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(authContextKey{}).(User)

	return user, ok
}
