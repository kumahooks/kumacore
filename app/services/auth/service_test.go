package authservice_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	authrepository "kumacore/app/repositories/auth"
	authservice "kumacore/app/services/auth"
	"kumacore/app/testing/db"
	"kumacore/app/testing/testfactory"
	"kumacore/core/cache"
)

var databaseTemplate *db.Template

func TestMain(testingMain *testing.M) {
	os.Exit(db.NewTemplate(testingMain, &databaseTemplate))
}

func TestAuthenticate_PermissionsLoadError_NoSessionCreated(t *testing.T) {
	database := databaseTemplate.Open(t)

	hash, err := bcrypt.GenerateFromPassword([]byte("lets all love lain"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate bcrypt hash: %v", err)
	}

	now := time.Now().Unix()
	if _, err = testfactory.CreateUser(context.Background(), database, testfactory.UserSeed{
		ID:           "permissions-fail-user",
		Username:     "permissions-fail",
		PasswordHash: string(hash),
		UpdatedAt:    now,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create test user: %v", err)
	}

	// Wrap the real repository so ListUserRoles fails after the credential lookup succeeds. This exercises the reorder:
	// the session must not be created when the permission read fails.
	failingRepo := &failingPermissionsRepository{
		repository:  authrepository.NewRepository(database),
		permissions: errors.New("simulated permission query failure"),
	}

	service, err := authservice.NewService(
		failingRepo,
		time.Hour,
		cache.NewCache[string, authservice.CachedUser](5*time.Minute),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	_, rawToken, _, authenticateError := service.Authenticate(
		context.Background(),
		"permissions-fail",
		"lets all love lain",
	)

	if authenticateError == nil {
		t.Fatal("expected Authenticate error when permission load fails, got nil")
	}

	if rawToken != "" {
		t.Errorf("raw token: got %q, want empty (no token should be returned on failure)", rawToken)
	}

	// No session row should exist for the user. The session is created only after loadPermissions succeeds. A transient
	// permission-read failure must not leave an orphaned session the caller has no token for.
	var sessionCount int
	err = database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE user_id = ?",
		"permissions-fail-user",
	).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if sessionCount != 0 {
		t.Errorf("sessions created: got %d, want 0 (no orphaned session on permission load failure)", sessionCount)
	}
}

// newTestService builds a Service backed by the given database and a real repository. Tests that need a failing mock
// can pass a wrapper instead.
func newTestService(t *testing.T, database *sql.DB) *authservice.Service {
	t.Helper()

	service, err := authservice.NewService(
		authrepository.NewRepository(database),
		time.Hour,
		cache.NewCache[string, authservice.CachedUser](5*time.Minute),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	return service
}

// seedUser inserts a user with a bcrypt-hashed password and returns its ID.
func seedUser(t *testing.T, database *sql.DB, username string, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate bcrypt hash: %v", err)
	}

	now := time.Now().Unix()
	userID := username + "-id"

	if _, err := testfactory.CreateUser(context.Background(), database, testfactory.UserSeed{
		ID:           userID,
		Username:     username,
		PasswordHash: string(hash),
		UpdatedAt:    now,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create test user: %v", err)
	}

	return userID
}

// seedSession inserts a session row for the given user and returns its token hash.
func seedSession(t *testing.T, database *sql.DB, userID string) string {
	t.Helper()

	tokenHash, err := testfactory.CreateSession(context.Background(), database, testfactory.SessionSeed{
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	return tokenHash
}

// seedSessionWithRawToken inserts a session row with a known raw token and returns both the raw token and the token
// hash. Used by tests that need to populate the auth cache via UserForToken and then verify eviction.
func seedSessionWithRawToken(t *testing.T, database *sql.DB, userID string) (rawToken string, tokenHash string) {
	t.Helper()

	rawToken = "raw-" + userID
	tokenHash, err := testfactory.CreateSession(context.Background(), database, testfactory.SessionSeed{
		RawToken:  rawToken,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	return rawToken, tokenHash
}

func TestChangeUsername_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	err := service.ChangeUsername(context.Background(), userID, "wrong-password", "newlain")
	if !errors.Is(err, authservice.ErrInvalidCredentials) {
		t.Errorf("error: got %v, want ErrInvalidCredentials", err)
	}

	var username string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT username FROM users WHERE id = ?",
		userID,
	).Scan(&username); err != nil {
		t.Fatalf("query username: %v", err)
	}

	if username != "lain" {
		t.Errorf("username after failed change: got %q, want %q", username, "lain")
	}
}

func TestChangeUsername_EmptyUsername_ReturnsErrInvalidUsername(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	for _, input := range []string{"", "   ", "\t\n"} {
		err := service.ChangeUsername(context.Background(), userID, "correct-horse", input)
		if !errors.Is(err, authservice.ErrInvalidUsername) {
			t.Errorf("error for %q: got %v, want ErrInvalidUsername", input, err)
		}
	}
}

func TestChangeUsername_TooLongUsername_ReturnsErrInvalidUsername(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	longUsername := strings.Repeat("a", authservice.MaxUsernameLength+1)

	err := service.ChangeUsername(context.Background(), userID, "correct-horse", longUsername)
	if !errors.Is(err, authservice.ErrInvalidUsername) {
		t.Errorf("error: got %v, want ErrInvalidUsername", err)
	}
}

func TestChangeUsername_MaxLengthUsername_Succeeds(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	maxUsername := strings.Repeat("a", authservice.MaxUsernameLength)

	err := service.ChangeUsername(context.Background(), userID, "correct-horse", maxUsername)
	if err != nil {
		t.Fatalf("change username at max length: %v", err)
	}

	var username string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT username FROM users WHERE id = ?",
		userID,
	).Scan(&username); err != nil {
		t.Fatalf("query username: %v", err)
	}

	if username != maxUsername {
		t.Errorf("username after change: got %q, want %q", username, maxUsername)
	}
}

func TestChangeUsername_Collision_ReturnsErrUsernameTaken(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	seedUser(t, database, "existing", "password-one")
	userID := seedUser(t, database, "lain", "correct-horse")

	err := service.ChangeUsername(context.Background(), userID, "correct-horse", "existing")
	if !errors.Is(err, authservice.ErrUsernameTaken) {
		t.Errorf("error: got %v, want ErrUsernameTaken", err)
	}

	var username string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT username FROM users WHERE id = ?",
		userID,
	).Scan(&username); err != nil {
		t.Fatalf("query username: %v", err)
	}

	if username != "lain" {
		t.Errorf("username after collision: got %q, want %q", username, "lain")
	}
}

func TestChangeUsername_IdenticalToCurrent_ReturnsErrInvalidUsername(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	seedSession(t, database, userID)

	err := service.ChangeUsername(context.Background(), userID, "correct-horse", "lain")
	if !errors.Is(err, authservice.ErrInvalidUsername) {
		t.Errorf("error: got %v, want ErrInvalidUsername", err)
	}

	// Sessions must be untouched: the early return happens before any write, so the session we seeded must still be present.
	var sessionCount int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE user_id = ?",
		userID,
	).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if sessionCount != 1 {
		t.Errorf("sessions after identical change: got %d, want 1 (unchanged)", sessionCount)
	}
}

func TestChangeUsername_Success_UpdatesUsernameAndDeletesSessions(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	tokenHash := seedSession(t, database, userID)

	err := service.ChangeUsername(context.Background(), userID, "correct-horse", "newlain")
	if err != nil {
		t.Fatalf("change username: %v", err)
	}

	var username string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT username FROM users WHERE id = ?",
		userID,
	).Scan(&username); err != nil {
		t.Fatalf("query username: %v", err)
	}

	if username != "newlain" {
		t.Errorf("username after change: got %q, want %q", username, "newlain")
	}

	var sessionCount int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE user_id = ?",
		userID,
	).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if sessionCount != 0 {
		t.Errorf("sessions after change: got %d, want 0", sessionCount)
	}

	var tokenExists int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE token_hash = ?",
		tokenHash,
	).Scan(&tokenExists); err != nil {
		t.Fatalf("check token: %v", err)
	}

	if tokenExists != 0 {
		t.Errorf("original session still exists: got %d, want 0", tokenExists)
	}
}

func TestChangeUsername_TrimsWhitespace(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	err := service.ChangeUsername(context.Background(), userID, "correct-horse", "  newlain  ")
	if err != nil {
		t.Fatalf("change username: %v", err)
	}

	var username string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT username FROM users WHERE id = ?",
		userID,
	).Scan(&username); err != nil {
		t.Fatalf("query username: %v", err)
	}

	if username != "newlain" {
		t.Errorf("username after trim: got %q, want %q", username, "newlain")
	}
}

func TestChangeUsername_NonExistentUser_ReturnsErrInvalidCredentials(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	err := service.ChangeUsername(context.Background(), "ghost-id", "any-password", "newname")
	if !errors.Is(err, authservice.ErrInvalidCredentials) {
		t.Errorf("error: got %v, want ErrInvalidCredentials", err)
	}
}

func TestChangePassword_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	err := service.ChangePassword(context.Background(), userID, "wrong-password", "new-password")
	if !errors.Is(err, authservice.ErrInvalidCredentials) {
		t.Errorf("error: got %v, want ErrInvalidCredentials", err)
	}
}

func TestChangePassword_EmptyPassword_ReturnsErrInvalidPassword(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	err := service.ChangePassword(context.Background(), userID, "correct-horse", "")
	if !errors.Is(err, authservice.ErrInvalidPassword) {
		t.Errorf("error: got %v, want ErrInvalidPassword", err)
	}
}

func TestChangePassword_TooShortPassword_ReturnsErrInvalidPassword(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	shortPassword := strings.Repeat("a", authservice.MinPasswordLength-1)

	err := service.ChangePassword(context.Background(), userID, "correct-horse", shortPassword)
	if !errors.Is(err, authservice.ErrInvalidPassword) {
		t.Errorf("error: got %v, want ErrInvalidPassword", err)
	}
}

func TestChangePassword_MinLengthPassword_Succeeds(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	minPassword := strings.Repeat("a", authservice.MinPasswordLength)

	err := service.ChangePassword(context.Background(), userID, "correct-horse", minPassword)
	if err != nil {
		t.Fatalf("change password at min length: %v", err)
	}

	var passwordHash string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT password_hash FROM users WHERE id = ?",
		userID,
	).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(minPassword)); err != nil {
		t.Errorf("min-length password does not match the hash: %v", err)
	}
}

func TestChangePassword_TooLongPassword_ReturnsErrInvalidPassword(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	longPassword := strings.Repeat("a", authservice.MaxPasswordLength+1)

	err := service.ChangePassword(context.Background(), userID, "correct-horse", longPassword)
	if !errors.Is(err, authservice.ErrInvalidPassword) {
		t.Errorf("error: got %v, want ErrInvalidPassword", err)
	}
}

func TestChangePassword_MaxLengthPassword_Succeeds(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")

	maxPassword := strings.Repeat("a", authservice.MaxPasswordLength)

	err := service.ChangePassword(context.Background(), userID, "correct-horse", maxPassword)
	if err != nil {
		t.Fatalf("change password at max length: %v", err)
	}

	var passwordHash string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT password_hash FROM users WHERE id = ?",
		userID,
	).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(maxPassword)); err != nil {
		t.Errorf("max-length password does not match the hash: %v", err)
	}
}

func TestChangePassword_Success_UpdatesHashAndDeletesSessions(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	tokenHash := seedSession(t, database, userID)

	err := service.ChangePassword(context.Background(), userID, "correct-horse", "new-password")
	if err != nil {
		t.Fatalf("change password: %v", err)
	}

	var passwordHash string
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT password_hash FROM users WHERE id = ?",
		userID,
	).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("correct-horse")) == nil {
		t.Error("old password still matches the new hash")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("new-password")); err != nil {
		t.Errorf("new password does not match the hash: %v", err)
	}

	var sessionCount int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE user_id = ?",
		userID,
	).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if sessionCount != 0 {
		t.Errorf("sessions after change: got %d, want 0", sessionCount)
	}

	var tokenExists int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE token_hash = ?",
		tokenHash,
	).Scan(&tokenExists); err != nil {
		t.Fatalf("check token: %v", err)
	}

	if tokenExists != 0 {
		t.Errorf("original session still exists: got %d, want 0", tokenExists)
	}
}

func TestChangePassword_NonExistentUser_ReturnsErrInvalidCredentials(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	err := service.ChangePassword(context.Background(), "ghost-id", "any-password", "new-password")
	if !errors.Is(err, authservice.ErrInvalidCredentials) {
		t.Errorf("error: got %v, want ErrInvalidCredentials", err)
	}
}

func TestClearSessions_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	seedSession(t, database, userID)

	err := service.ClearSessions(context.Background(), userID, "wrong-password")
	if !errors.Is(err, authservice.ErrInvalidCredentials) {
		t.Errorf("error: got %v, want ErrInvalidCredentials", err)
	}

	var sessionCount int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE user_id = ?",
		userID,
	).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if sessionCount != 1 {
		t.Errorf("sessions after failed clear: got %d, want 1 (unchanged)", sessionCount)
	}
}

func TestClearSessions_Success_DeletesAllSessions(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	seedSession(t, database, userID)
	seedSession(t, database, userID)
	seedSession(t, database, userID)

	err := service.ClearSessions(context.Background(), userID, "correct-horse")
	if err != nil {
		t.Fatalf("clear sessions: %v", err)
	}

	var sessionCount int
	if err := database.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sessions WHERE user_id = ?",
		userID,
	).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if sessionCount != 0 {
		t.Errorf("sessions after clear: got %d, want 0", sessionCount)
	}
}

func TestClearSessions_NonExistentUser_ReturnsErrInvalidCredentials(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	err := service.ClearSessions(context.Background(), "ghost-id", "any-password")
	if !errors.Is(err, authservice.ErrInvalidCredentials) {
		t.Errorf("error: got %v, want ErrInvalidCredentials", err)
	}
}

// TestChangeUsername_Success_EvictsCachedSession verifies that after a successful username change, a previously cached
// session token no longer authenticates. DeleteSessionsByUser returns the deleted token hashes so the service can remove
// them from the user cache. Without that, a captured token would keep working until the cache TTL elapsed.
func TestChangeUsername_Success_EvictsCachedSession(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	rawToken, _ := seedSessionWithRawToken(t, database, userID)

	if _, err := service.UserForToken(context.Background(), rawToken, time.Now()); err != nil {
		t.Fatalf("populate cache: %v", err)
	}

	if err := service.ChangeUsername(context.Background(), userID, "correct-horse", "newlain"); err != nil {
		t.Fatalf("change username: %v", err)
	}

	if _, err := service.UserForToken(context.Background(), rawToken, time.Now()); err == nil {
		t.Error("UserForToken returned cached user after username change")
	}
}

func TestChangePassword_Success_EvictsCachedSession(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	rawToken, _ := seedSessionWithRawToken(t, database, userID)

	if _, err := service.UserForToken(context.Background(), rawToken, time.Now()); err != nil {
		t.Fatalf("populate cache: %v", err)
	}

	if err := service.ChangePassword(context.Background(), userID, "correct-horse", "new-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}

	if _, err := service.UserForToken(context.Background(), rawToken, time.Now()); err == nil {
		t.Error("UserForToken returned cached user after password change")
	}
}

func TestClearSessions_Success_EvictsCachedSession(t *testing.T) {
	database := databaseTemplate.Open(t)
	service := newTestService(t, database)

	userID := seedUser(t, database, "lain", "correct-horse")
	rawToken, _ := seedSessionWithRawToken(t, database, userID)

	if _, err := service.UserForToken(context.Background(), rawToken, time.Now()); err != nil {
		t.Fatalf("populate cache: %v", err)
	}

	if err := service.ClearSessions(context.Background(), userID, "correct-horse"); err != nil {
		t.Fatalf("clear sessions: %v", err)
	}

	if _, err := service.UserForToken(context.Background(), rawToken, time.Now()); err == nil {
		t.Error("UserForToken returned cached user after session clear")
	}
}

// failingCredentialRepository wraps a real repository and forces a configurable subset of credential-change methods to
// return an error. Methods that are not overridden delegate to the inner repository. Used to exercise the service's
// internal-error wrapping paths.
type failingCredentialRepository struct {
	authrepository.Repository
	findCredentialsByUserID error
	updateUsername          error
	updatePassword          error
	deleteSessionsByUser    error
	inTransaction           error
}

func (repo *failingCredentialRepository) FindCredentialsByUserID(
	ctx context.Context,
	userID string,
) (authrepository.Credentials, error) {
	if repo.findCredentialsByUserID != nil {
		return authrepository.Credentials{}, repo.findCredentialsByUserID
	}

	return repo.Repository.FindCredentialsByUserID(ctx, userID)
}

func (repo *failingCredentialRepository) UpdateUsername(
	ctx context.Context,
	userID string,
	username string,
	updatedAtUnix int64,
) error {
	if repo.updateUsername != nil {
		return repo.updateUsername
	}

	return repo.Repository.UpdateUsername(ctx, userID, username, updatedAtUnix)
}

func (repo *failingCredentialRepository) UpdatePassword(
	ctx context.Context,
	userID string,
	passwordHash string,
	updatedAtUnix int64,
) error {
	if repo.updatePassword != nil {
		return repo.updatePassword
	}

	return repo.Repository.UpdatePassword(ctx, userID, passwordHash, updatedAtUnix)
}

func (repo *failingCredentialRepository) DeleteSessionsByUser(ctx context.Context, userID string) ([]string, error) {
	if repo.deleteSessionsByUser != nil {
		return nil, repo.deleteSessionsByUser
	}

	return repo.Repository.DeleteSessionsByUser(ctx, userID)
}

func (repo *failingCredentialRepository) InTransaction(
	ctx context.Context,
	work func(authrepository.Repository) error,
) error {
	if repo.inTransaction != nil {
		return repo.inTransaction
	}

	// Forward the same failure flags into the tx-scoped wrapper so the service observes the configured errors from inside
	// the closure.
	return repo.Repository.InTransaction(ctx, func(transactionRepository authrepository.Repository) error {
		return work(&failingCredentialRepository{
			Repository:              transactionRepository,
			findCredentialsByUserID: repo.findCredentialsByUserID,
			updateUsername:          repo.updateUsername,
			updatePassword:          repo.updatePassword,
			deleteSessionsByUser:    repo.deleteSessionsByUser,
		})
	})
}

var errRepositoryFailure = errors.New("simulated repository failure")

func TestChangeUsername_FindCredentialsError_ReturnsWrappedError(t *testing.T) {
	database := databaseTemplate.Open(t)
	repository := &failingCredentialRepository{
		Repository:              authrepository.NewRepository(database),
		findCredentialsByUserID: errRepositoryFailure,
	}

	service, err := authservice.NewService(
		repository,
		time.Hour,
		cache.NewCache[string, authservice.CachedUser](5*time.Minute),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	userID := seedUser(t, database, "lain", "correct-horse")

	changeErr := service.ChangeUsername(context.Background(), userID, "correct-horse", "newlain")
	if !errors.Is(changeErr, errRepositoryFailure) {
		t.Errorf("error: got %v, want it to wrap errRepositoryFailure", changeErr)
	}
}

func TestChangeUsername_InTransactionError_ReturnsWrappedError(t *testing.T) {
	database := databaseTemplate.Open(t)
	repository := &failingCredentialRepository{
		Repository:    authrepository.NewRepository(database),
		inTransaction: errRepositoryFailure,
	}

	service, err := authservice.NewService(
		repository,
		time.Hour,
		cache.NewCache[string, authservice.CachedUser](5*time.Minute),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	userID := seedUser(t, database, "lain", "correct-horse")

	changeErr := service.ChangeUsername(context.Background(), userID, "correct-horse", "newlain")
	if !errors.Is(changeErr, errRepositoryFailure) {
		t.Errorf("error: got %v, want it to wrap errRepositoryFailure", changeErr)
	}
}

func TestChangePassword_DeleteSessionsError_ReturnsWrappedError(t *testing.T) {
	database := databaseTemplate.Open(t)
	repository := &failingCredentialRepository{
		Repository:           authrepository.NewRepository(database),
		deleteSessionsByUser: errRepositoryFailure,
	}

	service, err := authservice.NewService(
		repository,
		time.Hour,
		cache.NewCache[string, authservice.CachedUser](5*time.Minute),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	userID := seedUser(t, database, "lain", "correct-horse")

	changeErr := service.ChangePassword(context.Background(), userID, "correct-horse", "new-password")
	if !errors.Is(changeErr, errRepositoryFailure) {
		t.Errorf("error: got %v, want it to wrap errRepositoryFailure", changeErr)
	}
}

func TestClearSessions_FindCredentialsError_ReturnsWrappedError(t *testing.T) {
	database := databaseTemplate.Open(t)
	repository := &failingCredentialRepository{
		Repository:              authrepository.NewRepository(database),
		findCredentialsByUserID: errRepositoryFailure,
	}

	service, err := authservice.NewService(
		repository,
		time.Hour,
		cache.NewCache[string, authservice.CachedUser](5*time.Minute),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	userID := seedUser(t, database, "lain", "correct-horse")

	changeErr := service.ClearSessions(context.Background(), userID, "correct-horse")
	if !errors.Is(changeErr, errRepositoryFailure) {
		t.Errorf("error: got %v, want it to wrap errRepositoryFailure", changeErr)
	}
}

func TestClearSessions_DeleteSessionsError_ReturnsWrappedError(t *testing.T) {
	database := databaseTemplate.Open(t)
	repository := &failingCredentialRepository{
		Repository:           authrepository.NewRepository(database),
		deleteSessionsByUser: errRepositoryFailure,
	}

	service, err := authservice.NewService(
		repository,
		time.Hour,
		cache.NewCache[string, authservice.CachedUser](5*time.Minute),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	userID := seedUser(t, database, "lain", "correct-horse")

	changeErr := service.ClearSessions(context.Background(), userID, "correct-horse")
	if !errors.Is(changeErr, errRepositoryFailure) {
		t.Errorf("error: got %v, want it to wrap errRepositoryFailure", changeErr)
	}
}

// failingPermissionsRepository wraps a real repository and forces ListUserRoles to return an error. All other methods
// delegate.
type failingPermissionsRepository struct {
	repository  authrepository.Repository
	permissions error
}

func (repo *failingPermissionsRepository) FindCredentialsByUsername(
	ctx context.Context,
	username string,
) (authrepository.Credentials, error) {
	return repo.repository.FindCredentialsByUsername(ctx, username)
}

func (repo *failingPermissionsRepository) FindCredentialsByUserID(
	ctx context.Context,
	userID string,
) (authrepository.Credentials, error) {
	return repo.repository.FindCredentialsByUserID(ctx, userID)
}

func (repo *failingPermissionsRepository) CreateSession(
	ctx context.Context,
	tokenHash string,
	userID string,
	createdAtUnix int64,
	expiresAtUnix int64,
) error {
	return repo.repository.CreateSession(ctx, tokenHash, userID, createdAtUnix, expiresAtUnix)
}

func (repo *failingPermissionsRepository) UpdateUserLastLogin(
	ctx context.Context,
	userID string,
	lastLoginAtUnix int64,
) error {
	return repo.repository.UpdateUserLastLogin(ctx, userID, lastLoginAtUnix)
}

func (repo *failingPermissionsRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	return repo.repository.DeleteSession(ctx, tokenHash)
}

func (repo *failingPermissionsRepository) DeleteSessionsByUser(ctx context.Context, userID string) ([]string, error) {
	return repo.repository.DeleteSessionsByUser(ctx, userID)
}

func (repo *failingPermissionsRepository) UpdateUsername(
	ctx context.Context,
	userID string,
	username string,
	updatedAtUnix int64,
) error {
	return repo.repository.UpdateUsername(ctx, userID, username, updatedAtUnix)
}

func (repo *failingPermissionsRepository) UpdatePassword(
	ctx context.Context,
	userID string,
	passwordHash string,
	updatedAtUnix int64,
) error {
	return repo.repository.UpdatePassword(ctx, userID, passwordHash, updatedAtUnix)
}

func (repo *failingPermissionsRepository) FindSessionUser(
	ctx context.Context,
	tokenHash string,
	nowUnix int64,
) (authrepository.SessionUser, error) {
	return repo.repository.FindSessionUser(ctx, tokenHash, nowUnix)
}

func (repo *failingPermissionsRepository) ListUserRoles(
	ctx context.Context,
	userID string,
) ([]authrepository.Role, error) {
	return nil, repo.permissions
}

func (repo *failingPermissionsRepository) InTransaction(
	ctx context.Context,
	work func(authrepository.Repository) error,
) error {
	return repo.repository.InTransaction(ctx, work)
}

// Compile-time assertion that the mock satisfies the Repository interface.
var _ authrepository.Repository = (*failingPermissionsRepository)(nil)
