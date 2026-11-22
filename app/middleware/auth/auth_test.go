package authmiddleware_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kumacore/app/middleware/auth"
	authrepository "kumacore/app/repositories/auth"
	authservice "kumacore/app/services/auth"
	"kumacore/app/testing/db"
	"kumacore/app/testing/testfactory"
	"kumacore/core/cache"
	"kumacore/core/render"
)

const testUserID = "a2ccf831-0d18-4d77-b153-18cdc2334586"

var databaseTemplate *db.Template

func TestMain(testingMain *testing.M) {
	os.Exit(db.NewTemplate(testingMain, &databaseTemplate))
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	return databaseTemplate.Open(t)
}

func newTestAuthService(t *testing.T, database *sql.DB) *authservice.Service {
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

func insertTestSession(t *testing.T, database *sql.DB, rawToken string, expiresAt int64) {
	t.Helper()

	now := time.Now().Unix()

	if _, err := testfactory.CreateUser(context.Background(), database, testfactory.UserSeed{
		ID:           testUserID,
		Username:     "auth-test",
		PasswordHash: "PASSWORD_HASH",
		UpdatedAt:    now,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create test user: %v", err)
	}

	if _, err := testfactory.CreateSession(context.Background(), database, testfactory.SessionSeed{
		RawToken:  rawToken,
		UserID:    testUserID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatalf("create test session: %v", err)
	}
}

func captureUser(t *testing.T, authService *authservice.Service, rawToken string) authservice.User {
	t.Helper()

	var capturedUser authservice.User
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := authservice.AuthUser(request.Context())
		if !ok {
			t.Error("expected user in context, got none")
			return
		}
		capturedUser = user
	})

	handler := authmiddleware.LoadAuth(authService)(next)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})
	handler.ServeHTTP(httptest.NewRecorder(), request)

	return capturedUser
}

// usersEqual compares two User values field-by-field.
func usersEqual(user1, user2 authservice.User) bool {
	return user1.ID == user2.ID &&
		user1.Username == user2.Username &&
		user1.Permissions == user2.Permissions &&
		slices.Equal(user1.RoleNames, user2.RoleNames)
}

func TestLoadAuth_ValidSession_InjectsUser(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("lain", 12)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())

	handler := authmiddleware.LoadAuth(
		authService,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			user, ok := authservice.AuthUser(request.Context())
			if !ok {
				t.Error("expected user in context, got none")
				return
			}
			if user.ID == "" {
				t.Error("user ID in context is empty")
			}
			if user.Username == "" {
				t.Error("username in context is empty")
			}
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestLoadAuth_NoCookie_NoUserInContext(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	nextCalled := false
	handler := authmiddleware.LoadAuth(
		authService,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			nextCalled = true
			if _, ok := authservice.AuthUser(request.Context()); ok {
				t.Error("expected no user in context, got one")
			}
		}),
	)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !nextCalled {
		t.Error("no cookie: next handler was not called")
	}
}

func TestLoadAuth_ExpiredSession_NoUserInContext(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("lain42", 8)
	insertTestSession(t, database, rawToken, time.Now().Add(-time.Hour).Unix())

	handler := authmiddleware.LoadAuth(
		authService,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := authservice.AuthUser(request.Context()); ok {
				t.Error("expected no user for expired session, got one")
			}
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestLoadAuth_InvalidToken_NoUserInContext(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	handler := authmiddleware.LoadAuth(
		authService,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := authservice.AuthUser(request.Context()); ok {
				t.Error("expected no user for invalid token, got one")
			}
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: "notavalidtoken"})
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestRequireAuth_ValidSession_CallsNext(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("lainuwu", 8)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())

	nextCalled := false
	handler := authmiddleware.RequireAuth(
		authService,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			nextCalled = true
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/relay", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if !nextCalled {
		t.Error("next handler was not called for valid session")
	}
}

func TestRequireAuth_NoSession_RedirectsToRoot(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	handler := authmiddleware.RequireAuth(
		authService,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			t.Error("next handler must not be called when session is absent")
		}),
	)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusSeeOther {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusSeeOther)
	}

	if location := recorder.Header().Get("Location"); location != "/" {
		t.Errorf("redirect location: got %q, want %q", location, "/")
	}
}

func TestRequireAuthOrNotFound_ValidSession_CallsNext(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)
	renderer := &mockRenderer{}

	rawToken := strings.Repeat("lainuwu", 8)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())

	nextCalled := false
	handler := authmiddleware.RequireAuthOrNotFound(
		authService,
		renderer,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			nextCalled = true
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if !nextCalled {
		t.Error("next handler was not called for valid session")
	}

	if renderer.renderErrorWasCall {
		t.Error("RenderError must not be called for valid session")
	}
}

func TestRequireAuthOrNotFound_NoSession_Returns404(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)
	renderer := &mockRenderer{}

	handler := authmiddleware.RequireAuthOrNotFound(
		authService,
		renderer,
	)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			t.Error("next handler must not be called when session is absent")
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if !renderer.renderErrorWasCall {
		t.Error("RenderError was not called")
	}

	if renderer.renderErrorStatus != http.StatusNotFound {
		t.Errorf("RenderError status: got %d, want %d", renderer.renderErrorStatus, http.StatusNotFound)
	}

	if renderer.renderErrorMessage != "ls" {
		t.Errorf("RenderError message: got %q, want %q", renderer.renderErrorMessage, "ls")
	}

	if location := recorder.Header().Get("Location"); location != "" {
		t.Errorf("redirect location: got %q, want empty (no redirect for 404)", location)
	}
}

func insertRole(t *testing.T, database *sql.DB, id int, name string, permissions int64) {
	t.Helper()

	if _, err := testfactory.CreateRole(context.Background(), database, testfactory.RoleSeed{
		ID:          id,
		Name:        name,
		Permissions: permissions,
	}); err != nil {
		t.Fatalf("create role %s: %v", name, err)
	}
}

func assignRole(t *testing.T, database *sql.DB, userID string, roleID int) {
	t.Helper()

	if err := testfactory.AssignUserRole(
		context.Background(),
		database,
		testfactory.UserRoleSeed{UserID: userID, RoleID: roleID},
	); err != nil {
		t.Fatalf("assign role %d to user %s: %v", roleID, userID, err)
	}
}

func TestLoadAuth_ValidSession_NoRoles_ZeroPermissions(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("noroles", 8)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())

	user := captureUser(t, authService, rawToken)
	if user.Permissions != 0 {
		t.Errorf("permissions: got %d, want 0", user.Permissions)
	}
}

func TestLoadAuth_ValidSession_SingleRole_PermissionsLoaded(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("singlerole", 6)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())
	insertRole(t, database, 101, "single-role", authservice.PermissionUnlimitedStorage)
	assignRole(t, database, testUserID, 101)

	user := captureUser(t, authService, rawToken)
	if user.Permissions != authservice.PermissionUnlimitedStorage {
		t.Errorf("permissions: got %d, want %d", user.Permissions, authservice.PermissionUnlimitedStorage)
	}
}

func TestLoadAuth_ValidSession_MultipleRoles_PermissionsORed(t *testing.T) {
	const permissionA int64 = 1 << 0
	const permissionB int64 = 1 << 1

	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("multirole", 6)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())
	insertRole(t, database, 102, "role-a", permissionA)
	insertRole(t, database, 103, "role-b", permissionB)
	assignRole(t, database, testUserID, 102)
	assignRole(t, database, testUserID, 103)

	user := captureUser(t, authService, rawToken)
	want := permissionA | permissionB
	if user.Permissions != want {
		t.Errorf("permissions: got %d, want %d", user.Permissions, want)
	}
}

func TestUserForToken_SecondCallReturnsCachedUser(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("cached", 8)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())

	user1, err := authService.UserForToken(context.Background(), rawToken, time.Now().UTC())
	if err != nil {
		t.Fatalf("first UserForToken: %v", err)
	}

	user2, err := authService.UserForToken(context.Background(), rawToken, time.Now().UTC())
	if err != nil {
		t.Fatalf("second UserForToken: %v", err)
	}

	if !usersEqual(user1, user2) {
		t.Errorf("cached user mismatch: got %+v, want %+v", user2, user1)
	}
}

func TestLogout_InvalidatesCache(t *testing.T) {
	database := openTestDB(t)

	authService := newTestAuthService(t, database)

	rawToken := strings.Repeat("logout", 8)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())

	if _, err := authService.UserForToken(context.Background(), rawToken, time.Now().UTC()); err != nil {
		t.Fatalf("UserForToken before logout: %v", err)
	}

	if err := authService.Logout(context.Background(), rawToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	_, err := authService.UserForToken(context.Background(), rawToken, time.Now().UTC())
	if err == nil {
		t.Error("UserForToken after logout: expected error, got nil")
	}
}

func TestUserForToken_ExpiredCacheEntry_TriggersDBRequery(t *testing.T) {
	database := openTestDB(t)

	cacheInstance := cache.NewCache[string, authservice.CachedUser](50 * time.Millisecond)
	authService, err := authservice.NewService(
		authrepository.NewRepository(database),
		time.Hour,
		cacheInstance,
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	rawToken := strings.Repeat("expiry", 8)
	insertTestSession(t, database, rawToken, time.Now().Add(time.Hour).Unix())

	user1, err := authService.UserForToken(context.Background(), rawToken, time.Now().UTC())
	if err != nil {
		t.Fatalf("first UserForToken: %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	user2, err := authService.UserForToken(context.Background(), rawToken, time.Now().UTC())
	if err != nil {
		t.Fatalf("second UserForToken after cache expiry: %v", err)
	}

	if !usersEqual(user2, user1) {
		t.Errorf("user after re-query: got %+v, want %+v", user2, user1)
	}
}

func TestUserForToken_ExpiredSessionInCache_RejectsAndRequeries(t *testing.T) {
	database := openTestDB(t)

	// Use a long cache TTL so the cache entry outlives the session expiry. This tests that UserForToken checks session
	// expiry on cache hit.
	cacheInstance := cache.NewCache[string, authservice.CachedUser](time.Hour)
	authService, err := authservice.NewService(
		authrepository.NewRepository(database),
		time.Hour,
		cacheInstance,
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	rawToken := strings.Repeat("stalecache", 8)
	// Session expires in 2 seconds (unix timestamps are second-precision, so sub-second expiry values get truncated to 0).
	insertTestSession(t, database, rawToken, time.Now().Add(2*time.Second).Unix())

	// Populate the cache while the session is still valid.
	user1, err := authService.UserForToken(context.Background(), rawToken, time.Now().UTC())
	if err != nil {
		t.Fatalf("UserForToken with valid session: %v", err)
	}

	// Wait for the session to expire by time. The cache entry is still present (TTL is 1h) but the session has expired.
	time.Sleep(2200 * time.Millisecond)

	// UserForToken must check session expiry on the cached entry and reject it.
	_, err = authService.UserForToken(context.Background(), rawToken, time.Now().UTC())
	if err == nil {
		t.Fatal("UserForToken with expired session: expected error, got nil (returned stale cached user)")
	}

	// Verify the stale entry was evicted from the cache.
	if _, ok := cacheInstance.Get(authservice.HashToken(rawToken)); ok {
		t.Error("cache still holds stale entry after rejected hit")
	}

	_ = user1
}

type mockRenderer struct {
	renderErrorStatus  int
	renderErrorMessage string
	renderErrorWasCall bool
}

func (mock *mockRenderer) Render(
	http.ResponseWriter,
	*http.Request,
	string,
	string,
	any,
) error {
	return nil
}

func (mock *mockRenderer) RenderError(writer http.ResponseWriter, _ *http.Request, status int, message string) {
	mock.renderErrorStatus = status
	mock.renderErrorMessage = message
	mock.renderErrorWasCall = true
	writer.WriteHeader(status)
}

func TestRequireAllPermissions_NoPermissions_PanicsAtConstruction(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when no permissions are specified")
		}
	}()

	authmiddleware.RequireAllPermissions(&mockRenderer{})
}

func TestRequireAllPermissions_AuthenticatedWithoutPermission_Returns404(t *testing.T) {
	renderer := &mockRenderer{}
	middleware := authmiddleware.RequireAllPermissions(renderer, authservice.PermissionUnlimitedStorage)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("next handler must not be called when permission is missing")
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request = request.WithContext(authservice.WithUser(request.Context(), authservice.User{
		ID:          "user-1",
		Username:    "lain",
		Permissions: 0,
	}))

	recorder := httptest.NewRecorder()
	middleware.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if !renderer.renderErrorWasCall {
		t.Error("RenderError was not called")
	}

	if renderer.renderErrorStatus != http.StatusNotFound {
		t.Errorf("RenderError status: got %d, want %d", renderer.renderErrorStatus, http.StatusNotFound)
	}

	if renderer.renderErrorMessage != "ls" {
		t.Errorf("RenderError message: got %q, want %q", renderer.renderErrorMessage, "ls")
	}
}

func TestRequireAllPermissions_AuthenticatedWithPermission_CallsNext(t *testing.T) {
	renderer := &mockRenderer{}
	nextCalled := false
	middleware := authmiddleware.RequireAllPermissions(renderer, authservice.PermissionUnlimitedStorage)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request = request.WithContext(authservice.WithUser(request.Context(), authservice.User{
		ID:          "user-1",
		Username:    "lain",
		Permissions: authservice.PermissionUnlimitedStorage,
	}))

	middleware.ServeHTTP(httptest.NewRecorder(), request)

	if !nextCalled {
		t.Error("next handler was not called for user with permission")
	}

	if renderer.renderErrorWasCall {
		t.Error("RenderError must not be called for user with permission")
	}
}

func TestRequireAllPermissions_AuthenticatedWithPartialPermission_Returns404(t *testing.T) {
	const permissionA int64 = 1 << 0
	const permissionB int64 = 1 << 1

	renderer := &mockRenderer{}
	middleware := authmiddleware.RequireAllPermissions(renderer, permissionA, permissionB)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("next handler must not be called when a permission is missing")
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request = request.WithContext(authservice.WithUser(request.Context(), authservice.User{
		ID:          "user-1",
		Username:    "lain",
		Permissions: permissionA,
	}))

	recorder := httptest.NewRecorder()
	middleware.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestRequireAllPermissions_Unauthenticated_Returns404(t *testing.T) {
	renderer := &mockRenderer{}
	middleware := authmiddleware.RequireAllPermissions(renderer, authservice.PermissionUnlimitedStorage)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("next handler must not be called without an authenticated user")
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)

	recorder := httptest.NewRecorder()
	middleware.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

// Compile-time assertion that mockRenderer satisfies render.Renderer.
var _ render.Renderer = (*mockRenderer)(nil)

func TestRequireAuth_PartialRequestSetsHXRedirect(t *testing.T) {
	authService := &authservice.Service{}

	handler := authmiddleware.RequireAuth(authService)(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			t.Error("next handler must not be called when session is absent")
		}),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request.Header.Set("HX-Request-Type", "partial")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	if hxRedirect := recorder.Header().Get("HX-Redirect"); hxRedirect != "/" {
		t.Errorf("HX-Redirect: got %q, want %q", hxRedirect, "/")
	}
}
