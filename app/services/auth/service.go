// Package authservice manages auth users, credential checks, and session lifecycle.
package authservice

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kumacore/app/repositories/auth"
)

// ErrInvalidCredentials is returned by Authenticate when credentials are invalid.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Service manages credential validation, session creation, and session deletion.
type Service struct {
	repository        authrepository.Repository
	sessionTTL        time.Duration
	dummyPasswordHash []byte
}

// NewService creates a Service backed by the given repository.
func NewService(repository authrepository.Repository, sessionTTL time.Duration) (*Service, error) {
	dummyPasswordHash, err := bcrypt.GenerateFromPassword([]byte("owo7"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("[auth:NewService] generate dummy password hash: %w", err)
	}

	return &Service{
		repository:        repository,
		sessionTTL:        sessionTTL,
		dummyPasswordHash: dummyPasswordHash,
	}, nil
}

// Authenticate validates username and password, creates a session, and returns the raw session token.
func (service *Service) Authenticate(
	ctx context.Context,
	username string,
	password string,
) (rawToken string, expiresAt time.Time, err error) {
	credentials, err := service.repository.FindCredentialsByUsername(ctx, username)

	userFound := true
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, fmt.Errorf("[auth:Authenticate] find credentials: %w", err)
		}

		userFound = false
		credentials.PasswordHash = string(service.dummyPasswordHash)
	}

	compareErr := bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(password))
	if !userFound || compareErr != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}

	rawToken, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("[auth:Authenticate] generate token: %w", err)
	}

	now := time.Now()
	expiresAt = now.Add(service.sessionTTL)

	if err := service.repository.CreateSession(
		ctx,
		tokenHash,
		credentials.UserID,
		now.Unix(),
		expiresAt.Unix(),
	); err != nil {
		return "", time.Time{}, fmt.Errorf("[auth:Authenticate] create session: %w", err)
	}

	if err := service.repository.UpdateUserLastLogin(ctx, credentials.UserID, now.Unix()); err != nil {
		log.Printf("[auth:Authenticate] update last login: %v", err)
	}

	return rawToken, expiresAt, nil
}

// Logout deletes the session identified by rawToken.
func (service *Service) Logout(ctx context.Context, rawToken string) error {
	tokenHash := HashToken(rawToken)

	if err := service.repository.DeleteSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("[auth:Logout] delete session: %w", err)
	}

	return nil
}

// UserForToken returns the authenticated user for a valid raw session token.
func (service *Service) UserForToken(ctx context.Context, rawToken string, now time.Time) (User, error) {
	tokenHash := HashToken(rawToken)

	sessionUser, err := service.repository.FindSessionUser(ctx, tokenHash, now.Unix())
	if err != nil {
		return User{}, fmt.Errorf("[auth:UserForToken] find session user: %w", err)
	}

	user := User{ID: sessionUser.ID, Username: sessionUser.Username}

	permissions, err := service.repository.ListUserRolePermissions(ctx, user.ID)
	if err != nil {
		return User{}, fmt.Errorf("[auth:UserForToken] list role permissions: %w", err)
	}

	for _, rolePermissions := range permissions {
		user.Permissions |= rolePermissions
	}

	return user, nil
}

// HashToken returns the hex-encoded SHA-256 hash of the given raw token.
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
