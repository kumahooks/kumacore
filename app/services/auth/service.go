// Package authservice defines the authenticated user type, context accessors, and the Service that manages session
// lifecycle.
package authservice

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	authrepository "kumacore/app/repositories/auth"
	"kumacore/core/cache"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrUsernameTaken is returned by ChangeUsername when the requested username collides with an existing row's UNIQUE
// constraint.
var ErrUsernameTaken = errors.New("username already taken")

// ErrInvalidUsername is returned by ChangeUsername when the requested username fails validation (empty or too long).
var ErrInvalidUsername = errors.New("invalid username")

// ErrInvalidPassword is returned by ChangePassword when the new password is shorter than MinPasswordLength or longer
// than MaxPasswordLength (bcrypt truncates above 72 bytes).
var ErrInvalidPassword = errors.New("invalid password")

// MaxUsernameLength mirrors the users.username column's practical limit. The schema does not enforce a length, but
// bounding it here prevents unbounded input from reaching the DB.
const MaxUsernameLength = 64

// MinPasswordLength bounds the plaintext away from weak values.
const MinPasswordLength = 12

// MaxPasswordLength bounds the plaintext passed to bcrypt. bcrypt truncates input longer than 72 bytes, which would
// silently accept mismatching passwords that share a 72-byte prefix. Reject upfront instead so callers know the password
// was not used as given.
const MaxPasswordLength = 72

// CachedUser wraps a User with the session expiry timestamp so the cache can reject entries whose underlying session
// has expired even if the cache TTL has not elapsed.
type CachedUser struct {
	User      User
	ExpiresAt int64
}

type Service struct {
	repository        authrepository.Repository
	sessionTTL        time.Duration
	dummyPasswordHash []byte
	userCache         *cache.Cache[string, CachedUser]

	// bcryptCost is the work factor passed to bcrypt.GenerateFromPassword. Production uses bcrypt.DefaultCost.
	bcryptCost int
}

func NewService(
	repository authrepository.Repository,
	sessionTTL time.Duration,
	userCache *cache.Cache[string, CachedUser],
	bcryptCost int,
) (*Service, error) {
	if userCache == nil {
		return nil, fmt.Errorf("[auth:NewService] userCache is required")
	}

	dummyPasswordHash, err := bcrypt.GenerateFromPassword([]byte("owo7"), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("[auth:NewService] generate dummy password hash: %w", err)
	}

	return &Service{
		repository:        repository,
		sessionTTL:        sessionTTL,
		dummyPasswordHash: dummyPasswordHash,
		userCache:         userCache,
		bcryptCost:        bcryptCost,
	}, nil
}

func (service *Service) Authenticate(
	ctx context.Context,
	username string,
	password string,
) (user User, rawToken string, expiresAt time.Time, err error) {
	credentials, err := service.repository.FindCredentialsByUsername(ctx, username)

	userFound := true
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return User{}, "", time.Time{}, fmt.Errorf("[auth:Authenticate] find credentials: %w", err)
		}

		userFound = false
		credentials.PasswordHash = string(service.dummyPasswordHash)
	}

	compareErr := bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(password))
	if !userFound || compareErr != nil {
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}

	rawToken, tokenHash, err := generateSessionToken()
	if err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("[auth:Authenticate] generate token: %w", err)
	}

	user = User{ID: credentials.UserID, Username: credentials.Username}

	// Load permissions before creating the session so a transient failure of the role-permission read does not leave an
	// orphaned session row that the caller has no token for.
	if err = service.loadPermissions(ctx, &user); err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("[auth:Authenticate] list role permissions: %w", err)
	}

	now := time.Now().UTC()
	expiresAt = now.Add(service.sessionTTL)

	if err = service.repository.CreateSession(
		ctx,
		tokenHash,
		credentials.UserID,
		now.Unix(),
		expiresAt.UTC().Unix(),
	); err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("[auth:Authenticate] create session: %w", err)
	}

	if err = service.repository.UpdateUserLastLogin(ctx, credentials.UserID, now.Unix()); err != nil {
		slog.ErrorContext(
			ctx, "update last login",
			slog.String("caller", "auth:Authenticate"),
			slog.String("user_id", credentials.UserID),
			slog.Any("err", err),
		)
	}

	// Populate the cache so the first request after login does not re-query the DB for the session and permissions.
	service.userCache.Set(tokenHash, CachedUser{User: user, ExpiresAt: expiresAt.UTC().Unix()})

	return user, rawToken, expiresAt, nil
}

func (service *Service) Logout(ctx context.Context, rawToken string) error {
	tokenHash := HashToken(rawToken)

	if err := service.repository.DeleteSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("[auth:Logout] delete session: %w", err)
	}

	service.userCache.Remove(tokenHash)

	return nil
}

func (service *Service) UserForToken(ctx context.Context, rawToken string, now time.Time) (User, error) {
	tokenHash := HashToken(rawToken)

	if cached, ok := service.userCache.Get(tokenHash); ok {
		if cached.ExpiresAt <= now.Unix() {
			service.userCache.Remove(tokenHash)
		} else {
			return cached.User, nil
		}
	}

	sessionUser, err := service.repository.FindSessionUser(ctx, tokenHash, now.Unix())
	if err != nil {
		return User{}, fmt.Errorf("[auth:UserForToken] find session user: %w", err)
	}

	user := User{ID: sessionUser.ID, Username: sessionUser.Username}

	if err = service.loadPermissions(ctx, &user); err != nil {
		return User{}, fmt.Errorf("[auth:UserForToken] list role permissions: %w", err)
	}

	// Cache the user with the session expiry so cache hits can reject expired sessions regardless of the cache TTL.
	service.userCache.Set(tokenHash, CachedUser{User: user, ExpiresAt: sessionUser.ExpiresAt})

	return user, nil
}

// loadPermissions aggregates the permission bitmasks of all roles assigned to the user into user.Permissions and collects
// the role names into user.RoleNames. It returns the raw repository error so callers can wrap it with their own
// [package:Function] prefix.
func (service *Service) loadPermissions(ctx context.Context, user *User) error {
	user.Permissions = 0
	user.RoleNames = nil

	roles, err := service.repository.ListUserRoles(ctx, user.ID)
	if err != nil {
		return err
	}

	for _, role := range roles {
		user.Permissions |= role.Permissions
		user.RoleNames = append(user.RoleNames, role.Name)
	}

	return nil
}

func HashToken(raw string) string {
	hash := sha256.Sum256([]byte(raw))

	return hex.EncodeToString(hash[:])
}

func generateSessionToken() (raw string, tokenHash string, err error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("[auth:generateSessionToken] read random bytes: %w", err)
	}

	raw = hex.EncodeToString(tokenBytes)
	tokenHash = HashToken(raw)

	return raw, tokenHash, nil
}

// ChangeUsername verifies the current password, sets the new username, and revokes every session for the user (including
// the caller's own), forcing a re-login under the new identity. The credential lookup, username update, and session
// deletion run in one transaction so a failure between writes cannot leave the row mutated with sessions still valid.
//
// Returns:
// - ErrInvalidCredentials when the current password is wrong.
// - ErrInvalidUsername when the new username is empty, too long, or identical
// - ErrUsernameTaken when the new username collides with another row.
func (service *Service) ChangeUsername(
	ctx context.Context,
	userID string,
	currentPassword string,
	newUsername string,
) error {
	newUsername = strings.TrimSpace(newUsername)
	if newUsername == "" || len(newUsername) > MaxUsernameLength {
		return ErrInvalidUsername
	}

	var deletedTokenHashes []string
	err := service.repository.InTransaction(ctx, func(transactionRepository authrepository.Repository) error {
		credentials, err := transactionRepository.FindCredentialsByUserID(ctx, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidCredentials
			}

			return fmt.Errorf("[auth:ChangeUsername] find credentials: %w", err)
		}

		if err = bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(currentPassword)); err != nil {
			return ErrInvalidCredentials
		}

		// SQLite treats UPDATE ... SET username = <same value> as a no-op, not a UNIQUE violation, so submitting the current
		// username would otherwise succeed, silently wipe every session, and force a re-login for nothing.
		if newUsername == credentials.Username {
			return ErrInvalidUsername
		}

		now := time.Now().UTC().Unix()
		if err = transactionRepository.UpdateUsername(ctx, userID, newUsername, now); err != nil {
			if errors.Is(err, authrepository.ErrUniqueConstraint) {
				return ErrUsernameTaken
			}

			return fmt.Errorf("[auth:ChangeUsername] update username: %w", err)
		}

		deletedTokenHashes, err = transactionRepository.DeleteSessionsByUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("[auth:ChangeUsername] delete sessions: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, tokenHash := range deletedTokenHashes {
		service.userCache.Remove(tokenHash)
	}

	return nil
}

// ChangePassword verifies the current password, sets the new password hash, and revokes every session for the user,
// forcing a re-login. The credential lookup, password update, and session deletion run in one transaction so a failure
// between writes cannot leave the row mutated with sessions still valid.
//
// Returns:
// - ErrInvalidCredentials when the current password is wrong.
// - ErrInvalidPassword when the new password is empty or longer than MaxPasswordLength
func (service *Service) ChangePassword(
	ctx context.Context,
	userID string,
	currentPassword string,
	newPassword string,
) error {
	if len(newPassword) < MinPasswordLength || len(newPassword) > MaxPasswordLength {
		return ErrInvalidPassword
	}

	var deletedTokenHashes []string
	err := service.repository.InTransaction(ctx, func(transactionRepository authrepository.Repository) error {
		credentials, err := transactionRepository.FindCredentialsByUserID(ctx, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidCredentials
			}

			return fmt.Errorf("[auth:ChangePassword] find credentials: %w", err)
		}

		if err = bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(currentPassword)); err != nil {
			return ErrInvalidCredentials
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), service.bcryptCost)
		if err != nil {
			return fmt.Errorf("[auth:ChangePassword] hash password: %w", err)
		}

		now := time.Now().UTC().Unix()
		if err = transactionRepository.UpdatePassword(ctx, userID, string(newHash), now); err != nil {
			return fmt.Errorf("[auth:ChangePassword] update password: %w", err)
		}

		deletedTokenHashes, err = transactionRepository.DeleteSessionsByUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("[auth:ChangePassword] delete sessions: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, tokenHash := range deletedTokenHashes {
		service.userCache.Remove(tokenHash)
	}

	return nil
}

// ClearSessions verifies the current password, then deletes every session for the user. Returns ErrInvalidCredentials
// when the current password is wrong.
func (service *Service) ClearSessions(ctx context.Context, userID string, currentPassword string) error {
	credentials, err := service.repository.FindCredentialsByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredentials
		}

		return fmt.Errorf("[auth:ClearSessions] find credentials: %w", err)
	}

	if err = bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	deletedTokenHashes, err := service.repository.DeleteSessionsByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("[auth:ClearSessions] delete sessions: %w", err)
	}

	for _, tokenHash := range deletedTokenHashes {
		service.userCache.Remove(tokenHash)
	}

	return nil
}
