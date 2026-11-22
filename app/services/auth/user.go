package authservice

import "context"

type authContextKey struct{}

// User represents an authenticated user with aggregated role permissions.
type User struct {
	ID          string
	Username    string
	Permissions int64
	RoleNames   []string
}

// WithUser injects a User into the context.
func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, authContextKey{}, user)
}

// AuthUser extracts the authenticated User from the context.
func AuthUser(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(authContextKey{}).(User)

	return user, ok
}

// HasAllPermissions reports whether the user holds every one of the given permission bits. With no arguments it returns
// false.
func (user User) HasAllPermissions(permissions ...int64) bool {
	if len(permissions) == 0 {
		return false
	}

	for _, permission := range permissions {
		if user.Permissions&permission == 0 {
			return false
		}
	}

	return true
}

// PermissionDescriptions returns the human-readable names of every permission bit the user holds, in registry order.
// Returns nil when the user holds no permissions. The returned slice is freshly allocated and safe for callers to mutate.
func (user User) PermissionDescriptions() []string {
	var names []string
	for _, entry := range permissionRegistry {
		if user.Permissions&entry.Bit != 0 {
			names = append(names, entry.Description)
		}
	}

	return names
}
