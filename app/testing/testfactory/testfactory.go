// Package testfactory provides record creation helpers for tests.
//
// Each Create* function inserts one row with sensible defaults for zero-valued
// fields. Functions accept driver.DBTX so they work with both *sql.DB and
// *sql.Tx.
package testfactory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"fmt"
	"time"

	"github.com/google/uuid"

	"kumacore/app/repositories"
	"kumacore/core/db/driver"
)

//go:embed queries/*.sql
var queryFiles embed.FS

var (
	createUserQuery     = repositories.MustReadQuery(queryFiles, "queries/create_user.sql")
	createSessionQuery  = repositories.MustReadQuery(queryFiles, "queries/create_session.sql")
	createRoleQuery     = repositories.MustReadQuery(queryFiles, "queries/create_role.sql")
	assignUserRoleQuery = repositories.MustReadQuery(queryFiles, "queries/assign_user_role.sql")
)

// UserSeed describes one user row to create.
type UserSeed struct {
	ID           string
	Username     string
	PasswordHash string
	UpdatedAt    int64
	CreatedAt    int64
}

// SessionSeed describes one session row to create.
type SessionSeed struct {
	RawToken  string
	TokenHash string
	UserID    string
	CreatedAt int64
	ExpiresAt int64
}

// RoleSeed describes one role row to create.
type RoleSeed struct {
	ID          int
	Name        string
	Permissions int64
}

// UserRoleSeed describes one users_roles row to create.
type UserRoleSeed struct {
	UserID string
	RoleID int
}

// CreateUser inserts one user row and returns its ID.
func CreateUser(ctx context.Context, databaseConnection driver.DBTX, userSeed UserSeed) (string, error) {
	now := time.Now().UTC().Unix()

	if userSeed.ID == "" {
		userSeed.ID = uuid.New().String()
	}

	if userSeed.Username == "" {
		userSeed.Username = userSeed.ID
	}

	if userSeed.PasswordHash == "" {
		userSeed.PasswordHash = "irrelevant"
	}

	if userSeed.UpdatedAt == 0 {
		userSeed.UpdatedAt = now
	}

	if userSeed.CreatedAt == 0 {
		userSeed.CreatedAt = now
	}

	if _, err := databaseConnection.ExecContext(
		ctx,
		createUserQuery,
		userSeed.ID,
		userSeed.Username,
		userSeed.PasswordHash,
		userSeed.UpdatedAt,
		userSeed.CreatedAt,
	); err != nil {
		return "", fmt.Errorf("[testfactory:CreateUser] insert user %q: %w", userSeed.Username, err)
	}

	return userSeed.ID, nil
}

// CreateSession inserts one session row and returns its token hash.
func CreateSession(ctx context.Context, databaseConnection driver.DBTX, sessionSeed SessionSeed) (string, error) {
	now := time.Now().UTC().Unix()

	if sessionSeed.TokenHash == "" {
		if sessionSeed.RawToken == "" {
			randomBytes := make([]byte, 32)
			if _, err := rand.Read(randomBytes); err != nil {
				return "", fmt.Errorf("[testfactory:CreateSession] generate token: %w", err)
			}

			sessionSeed.RawToken = fmt.Sprintf("%x", randomBytes)
		}

		hash := sha256.Sum256([]byte(sessionSeed.RawToken))
		sessionSeed.TokenHash = fmt.Sprintf("%x", hash[:])
	}

	if sessionSeed.CreatedAt == 0 {
		sessionSeed.CreatedAt = now
	}

	if sessionSeed.ExpiresAt == 0 {
		sessionSeed.ExpiresAt = time.Now().UTC().Add(time.Hour).Unix()
	}

	if _, err := databaseConnection.ExecContext(
		ctx,
		createSessionQuery,
		sessionSeed.TokenHash,
		sessionSeed.UserID,
		sessionSeed.CreatedAt,
		sessionSeed.ExpiresAt,
	); err != nil {
		return "", fmt.Errorf("[testfactory:CreateSession] insert session: %w", err)
	}

	return sessionSeed.TokenHash, nil
}

// CreateRole inserts one role row and returns its ID.
func CreateRole(ctx context.Context, databaseConnection driver.DBTX, roleSeed RoleSeed) (int, error) {
	if _, err := databaseConnection.ExecContext(
		ctx,
		createRoleQuery,
		roleSeed.ID,
		roleSeed.Name,
		roleSeed.Permissions,
	); err != nil {
		return 0, fmt.Errorf("[testfactory:CreateRole] insert role %q: %w", roleSeed.Name, err)
	}

	return roleSeed.ID, nil
}

// AssignUserRole inserts one users_roles row.
func AssignUserRole(ctx context.Context, databaseConnection driver.DBTX, userRoleSeed UserRoleSeed) error {
	if _, err := databaseConnection.ExecContext(
		ctx,
		assignUserRoleQuery,
		userRoleSeed.UserID,
		userRoleSeed.RoleID,
	); err != nil {
		return fmt.Errorf("[testfactory:AssignUserRole] insert user role: %w", err)
	}

	return nil
}
