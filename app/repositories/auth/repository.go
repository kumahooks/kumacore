// Package authrepository provides persistence operations for auth users, roles, and sessions.
package authrepository

import (
	"context"
	"embed"

	"kumacore/app/repositories"
	"kumacore/core/db/driver"
)

//go:embed queries/*.sql
var queryFiles embed.FS

var (
	findCredentialsByUsernameQuery = repositories.MustReadQuery(queryFiles, "queries/find_credentials_by_username.sql")
	createSessionQuery             = repositories.MustReadQuery(queryFiles, "queries/create_session.sql")
	updateUserLastLoginQuery       = repositories.MustReadQuery(queryFiles, "queries/update_user_last_login.sql")
	deleteSessionQuery             = repositories.MustReadQuery(queryFiles, "queries/delete_session.sql")
	findSessionUserQuery           = repositories.MustReadQuery(queryFiles, "queries/find_session_user.sql")
	listUserRolePermissionsQuery   = repositories.MustReadQuery(queryFiles, "queries/list_user_role_permissions.sql")
)

type Credentials struct {
	UserID       string
	PasswordHash string
}

type SessionUser struct {
	ID       string
	Username string
}

// Repository owns auth persistence.
type Repository interface {
	FindCredentialsByUsername(ctx context.Context, username string) (Credentials, error)
	CreateSession(ctx context.Context, tokenHash string, userID string, createdAtUnix int64, expiresAtUnix int64) error
	UpdateUserLastLogin(ctx context.Context, userID string, lastLoginAtUnix int64) error
	DeleteSession(ctx context.Context, tokenHash string) error
	FindSessionUser(ctx context.Context, tokenHash string, nowUnix int64) (SessionUser, error)
	ListUserRolePermissions(ctx context.Context, userID string) ([]int64, error)
}

type SQLRepository struct {
	databaseConnection driver.DBTX
}

// NewRepository returns a SQL auth repository.
func NewRepository(databaseConnection driver.DBTX) *SQLRepository {
	return &SQLRepository{databaseConnection: databaseConnection}
}

// FindCredentialsByUsername returns password credentials for a username.
func (repository *SQLRepository) FindCredentialsByUsername(ctx context.Context, username string) (Credentials, error) {
	var credentials Credentials
	err := repository.databaseConnection.QueryRowContext(ctx, findCredentialsByUsernameQuery, username).
		Scan(&credentials.UserID, &credentials.PasswordHash)

	return credentials, err
}

// CreateSession inserts a session row.
func (repository *SQLRepository) CreateSession(
	ctx context.Context,
	tokenHash string,
	userID string,
	createdAtUnix int64,
	expiresAtUnix int64,
) error {
	_, err := repository.databaseConnection.ExecContext(
		ctx,
		createSessionQuery,
		tokenHash,
		userID,
		createdAtUnix,
		expiresAtUnix,
	)

	return err
}

// UpdateUserLastLogin records the user's latest successful login.
func (repository *SQLRepository) UpdateUserLastLogin(ctx context.Context, userID string, lastLoginAtUnix int64) error {
	_, err := repository.databaseConnection.ExecContext(ctx, updateUserLastLoginQuery, lastLoginAtUnix, userID)

	return err
}

// DeleteSession deletes a session row.
func (repository *SQLRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := repository.databaseConnection.ExecContext(ctx, deleteSessionQuery, tokenHash)

	return err
}

// FindSessionUser returns the user for a valid unexpired session.
func (repository *SQLRepository) FindSessionUser(
	ctx context.Context,
	tokenHash string,
	nowUnix int64,
) (SessionUser, error) {
	var user SessionUser
	err := repository.databaseConnection.QueryRowContext(ctx, findSessionUserQuery, tokenHash, nowUnix).
		Scan(&user.ID, &user.Username)

	return user, err
}

// ListUserRolePermissions returns all permission bitmasks assigned to a user.
func (repository *SQLRepository) ListUserRolePermissions(ctx context.Context, userID string) ([]int64, error) {
	rows, err := repository.databaseConnection.QueryContext(ctx, listUserRolePermissionsQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []int64
	for rows.Next() {
		var rolePermissions int64
		if err := rows.Scan(&rolePermissions); err != nil {
			return nil, err
		}

		permissions = append(permissions, rolePermissions)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return permissions, nil
}
