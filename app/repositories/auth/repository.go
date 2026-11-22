// Package authrepository provides SQL-backed persistence for auth operations: credentials, sessions, and the self-service
// credential-change writes that the auth service orchestrates inside its transactions.
package authrepository

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"modernc.org/sqlite"

	"kumacore/app/repositories"
	"kumacore/core/db/driver"
)

//go:embed queries/*.sql
var queryFiles embed.FS

var (
	findCredentialsByUsernameQuery = repositories.MustReadQuery(queryFiles, "queries/find_credentials_by_username.sql")
	findCredentialsByUserIDQuery   = repositories.MustReadQuery(queryFiles, "queries/find_credentials_by_user_id.sql")
	createSessionQuery             = repositories.MustReadQuery(queryFiles, "queries/create_session.sql")
	updateUserLastLoginQuery       = repositories.MustReadQuery(queryFiles, "queries/update_user_last_login.sql")
	updateUsernameQuery            = repositories.MustReadQuery(queryFiles, "queries/update_username.sql")
	updatePasswordQuery            = repositories.MustReadQuery(queryFiles, "queries/update_password.sql")
	deleteSessionQuery             = repositories.MustReadQuery(queryFiles, "queries/delete_session.sql")
	deleteSessionsByUserQuery      = repositories.MustReadQuery(queryFiles, "queries/delete_sessions_by_user.sql")
	findSessionUserQuery           = repositories.MustReadQuery(queryFiles, "queries/find_session_user.sql")
	listUserRolesQuery             = repositories.MustReadQuery(queryFiles, "queries/list_user_roles.sql")
)

// ErrUniqueConstraint is returned by repository methods when a write violates a UNIQUE constraint. The service maps it
// to a domain error.
var ErrUniqueConstraint = errors.New("unique constraint violation")

// ErrInTransactionNested is returned by InTransaction when called on a repository that is already backed by a transaction.
// The service only calls InTransaction on the top-level repository, never on the tx-scoped one passed into work.
var ErrInTransactionNested = errors.New("InTransaction called inside a transaction")

// sqliteConstraintUnique is the extended result code for SQLITE_CONSTRAINT_UNIQUE as defined by modernc.org/sqlite/lib
// (see error.go in the driver package).
const sqliteConstraintUnique = 2067

// Credentials holds the ID, canonical username, and bcrypt hash for a user.
type Credentials struct {
	UserID       string
	Username     string
	PasswordHash string
}

// SessionUser holds user information from a valid session.
type SessionUser struct {
	ID        string
	Username  string
	ExpiresAt int64
}

// Role holds the permission bitmask and name of a single role assigned to a user.
type Role struct {
	Permissions int64
	Name        string
}

// Repository defines the data access interface for auth self-service flows: login, session restore, and credential
// changes.
type Repository interface {
	FindCredentialsByUsername(ctx context.Context, username string) (Credentials, error)
	FindCredentialsByUserID(ctx context.Context, userID string) (Credentials, error)
	CreateSession(ctx context.Context, tokenHash string, userID string, createdAtUnix int64, expiresAtUnix int64) error
	UpdateUserLastLogin(ctx context.Context, userID string, lastLoginAtUnix int64) error
	UpdateUsername(ctx context.Context, userID string, username string, updatedAtUnix int64) error
	UpdatePassword(ctx context.Context, userID string, passwordHash string, updatedAtUnix int64) error
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteSessionsByUser(ctx context.Context, userID string) ([]string, error)
	FindSessionUser(ctx context.Context, tokenHash string, nowUnix int64) (SessionUser, error)
	ListUserRoles(ctx context.Context, userID string) ([]Role, error)

	// InTransaction runs work against a Repository backed by a single database transaction. The transaction is committed
	// when work returns nil and rolled back on any error.
	InTransaction(ctx context.Context, work func(Repository) error) error
}

// SQLRepository is the SQL-backed implementation of Repository.
type SQLRepository struct {
	databaseConnection driver.DBTX
}

// Compile-time assertion that SQLRepository satisfies Repository.
var _ Repository = (*SQLRepository)(nil)

// NewRepository creates a new SQLRepository backed by the given connection.
func NewRepository(databaseConnection driver.DBTX) *SQLRepository {
	return &SQLRepository{databaseConnection: databaseConnection}
}

// FindCredentialsByUsername returns the user ID, canonical username, and password hash for the given username.
func (repository *SQLRepository) FindCredentialsByUsername(ctx context.Context, username string) (Credentials, error) {
	var credentials Credentials
	err := repository.databaseConnection.QueryRowContext(ctx, findCredentialsByUsernameQuery, username).
		Scan(&credentials.UserID, &credentials.Username, &credentials.PasswordHash)

	return credentials, err
}

// FindCredentialsByUserID returns the user ID, canonical username, and password hash for the given user ID.
func (repository *SQLRepository) FindCredentialsByUserID(ctx context.Context, userID string) (Credentials, error) {
	var credentials Credentials
	err := repository.databaseConnection.QueryRowContext(ctx, findCredentialsByUserIDQuery, userID).
		Scan(&credentials.UserID, &credentials.Username, &credentials.PasswordHash)

	return credentials, err
}

// CreateSession inserts a new session record with the given token hash and expiry.
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

// UpdateUserLastLogin sets the last login timestamp for the given user.
func (repository *SQLRepository) UpdateUserLastLogin(ctx context.Context, userID string, lastLoginAtUnix int64) error {
	_, err := repository.databaseConnection.ExecContext(ctx, updateUserLastLoginQuery, lastLoginAtUnix, userID)

	return err
}

// UpdateUsername sets the username column for the given user. Returns ErrUniqueConstraint when the new username collides
// with an existing row's UNIQUE value. The service maps that to a domain error.
func (repository *SQLRepository) UpdateUsername(
	ctx context.Context,
	userID string,
	username string,
	updatedAtUnix int64,
) error {
	_, err := repository.databaseConnection.ExecContext(ctx, updateUsernameQuery, username, updatedAtUnix, userID)

	return mapUniqueConstraintError(err)
}

// mapUniqueConstraintError inspects a driver error and returns ErrUniqueConstraint when it is a SQLite UNIQUE constraint
// violation (extended code SQLITE_CONSTRAINT_UNIQUE = 2067). Any other error is returned unchanged.
func mapUniqueConstraintError(err error) error {
	var sqliteError *sqlite.Error
	if errors.As(err, &sqliteError) && sqliteError.Code() == sqliteConstraintUnique {
		return ErrUniqueConstraint
	}

	return err
}

// UpdatePassword sets the password_hash column for the given user. The caller is expected to pass a pre-hashed bcrypt
// value.
func (repository *SQLRepository) UpdatePassword(
	ctx context.Context,
	userID string,
	passwordHash string,
	updatedAtUnix int64,
) error {
	_, err := repository.databaseConnection.ExecContext(ctx, updatePasswordQuery, passwordHash, updatedAtUnix, userID)

	return err
}

// DeleteSession removes the session with the given token hash.
func (repository *SQLRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := repository.databaseConnection.ExecContext(ctx, deleteSessionQuery, tokenHash)

	return err
}

// DeleteSessionsByUser removes every session row for the given user and returns the token hashes of the deleted rows so
// the caller can evict the corresponding cache entries.
func (repository *SQLRepository) DeleteSessionsByUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := repository.databaseConnection.QueryContext(ctx, deleteSessionsByUserQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokenHashes []string
	for rows.Next() {
		var tokenHash string
		if err := rows.Scan(&tokenHash); err != nil {
			return nil, err
		}

		tokenHashes = append(tokenHashes, tokenHash)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tokenHashes, nil
}

// FindSessionUser returns the user for a valid, non-expired session token.
func (repository *SQLRepository) FindSessionUser(
	ctx context.Context,
	tokenHash string,
	nowUnix int64,
) (SessionUser, error) {
	var user SessionUser
	err := repository.databaseConnection.QueryRowContext(ctx, findSessionUserQuery, tokenHash, nowUnix).
		Scan(&user.ID, &user.Username, &user.ExpiresAt)

	return user, err
}

// ListUserRoles returns the permission bitmask and name of every role assigned to the user.
func (repository *SQLRepository) ListUserRoles(ctx context.Context, userID string) ([]Role, error) {
	rows, err := repository.databaseConnection.QueryContext(ctx, listUserRolesQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.Permissions, &role.Name); err != nil {
			return nil, err
		}

		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return roles, nil
}

// InTransaction runs work against a Repository backed by a single database transaction. The transaction is committed
// when work returns nil and rolled back on any error. Returning ErrInTransactionNested when the repository is already
// backed by a transaction prevents accidental nested calls.
func (repository *SQLRepository) InTransaction(ctx context.Context, work func(Repository) error) error {
	database, ok := repository.databaseConnection.(*sql.DB)
	if !ok {
		return ErrInTransactionNested
	}

	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("[authrepository:InTransaction] begin: %w", err)
	}

	transactionRepository := NewRepository(transaction)

	if err := work(transactionRepository); err != nil {
		_ = transaction.Rollback()
		return err
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("[authrepository:InTransaction] commit: %w", err)
	}

	return nil
}
