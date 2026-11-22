package auth_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"kumacore/app/modules/auth"
	authrepository "kumacore/app/repositories/auth"
	authservice "kumacore/app/services/auth"
	"kumacore/app/testing/db"
	"kumacore/app/testing/testfactory"
	"kumacore/core/cache"
)

// mockRenderer captures the last render call for inspection.
type mockRenderer struct {
	lastData any
	err      error
}

func (mock *mockRenderer) Render(
	writer http.ResponseWriter,
	request *http.Request,
	pageFile string,
	fragment string,
	data any,
) error {
	mock.lastData = data

	return mock.err
}

func (mock *mockRenderer) RenderError(writer http.ResponseWriter, request *http.Request, status int, message string) {
	writer.WriteHeader(status)
}

var databaseTemplate *db.Template

func TestMain(testingMain *testing.M) {
	os.Exit(db.NewTemplate(testingMain, &databaseTemplate))
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	return databaseTemplate.Open(t)
}

func insertTestUser(t *testing.T, database *sql.DB, username string, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	testUserID := "344bab10-0913-4fa0-824c-5ea9d4548d85"
	now := time.Now().Unix()

	if _, err := testfactory.CreateUser(context.Background(), database, testfactory.UserSeed{
		ID:           testUserID,
		Username:     username,
		PasswordHash: string(hash),
		UpdatedAt:    now,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create test user: %v", err)
	}

	return testUserID
}

func newTestHandler(t *testing.T, database *sql.DB, renderer *mockRenderer) *auth.Handler {
	t.Helper()

	userCache := cache.NewCache[string, authservice.CachedUser](5 * time.Minute)
	service, err := authservice.NewService(
		authrepository.NewRepository(database),
		168*time.Hour,
		userCache,
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	return auth.NewHandler(renderer, service, true)
}

func TestLogin_GET_RendersForm(t *testing.T) {
	renderer := &mockRenderer{}
	handler := newTestHandler(t, openTestDB(t), renderer)

	recorder := httptest.NewRecorder()
	handler.Login(recorder, httptest.NewRequest(http.MethodGet, "/auth", nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAuthenticate_ValidCredentials_RedirectsToMeAndSetsCookie(t *testing.T) {
	database := openTestDB(t)

	insertTestUser(t, database, "auth-login", "lets all love lain")
	form := url.Values{"username": {"auth-login"}, "password": {"lets all love lain"}}

	renderer := &mockRenderer{}
	handler := newTestHandler(t, database, renderer)

	request := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()

	handler.Authenticate(recorder, request)

	if recorder.Code != http.StatusSeeOther {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusSeeOther)
	}

	if location := recorder.Header().Get("Location"); location != "/auth/me" {
		t.Errorf("redirect location: got %q, want %q", location, "/auth/me")
	}

	var sessionCookie *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "session" {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}

	if sessionCookie.Value == "" {
		t.Error("session cookie value is empty")
	}

	if !sessionCookie.HttpOnly {
		t.Error("session cookie missing HttpOnly")
	}

	if !sessionCookie.Secure {
		t.Error("session cookie missing Secure")
	}
}

func TestAuthenticate_WrongPassword_Returns403WithError(t *testing.T) {
	database := openTestDB(t)

	insertTestUser(t, database, "auth-wrong", "lets all love lain")
	form := url.Values{"username": {"auth-wrong"}, "password": {"uwu"}}

	renderer := &mockRenderer{}
	handler := newTestHandler(t, database, renderer)

	request := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()

	handler.Authenticate(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusForbidden)
	}

	dataMap, ok := renderer.lastData.(map[string]any)
	if !ok {
		t.Fatal("render data is not map[string]any")
	}

	authError, hasError := dataMap["Error"]
	if !hasError {
		t.Error("expected \"Error\" key in render data for wrong password, got none")
	}

	if authError != "invalid credentials" {
		t.Errorf("auth error: got %s, want %s", authError, "invalid credentials")
	}
}

func TestAuthenticate_UnknownUser_Returns403WithError(t *testing.T) {
	form := url.Values{"username": {"baudrillard"}, "password": {"rosetta stoned"}}

	renderer := &mockRenderer{}
	handler := newTestHandler(t, openTestDB(t), renderer)

	request := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()

	handler.Authenticate(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusForbidden)
	}

	dataMap, ok := renderer.lastData.(map[string]any)
	if !ok {
		t.Fatal("render data is not map[string]any")
	}

	authError, hasError := dataMap["Error"]
	if !hasError {
		t.Error("expected \"Error\" key in render data for unknown user, got none")
	}

	if authError != "invalid credentials" {
		t.Errorf("auth error: got %s, want %s", authError, "invalid credentials")
	}
}

func TestAuthenticate_EmptyFields_Returns403WithError(t *testing.T) {
	form := url.Values{"username": {""}, "password": {""}}

	renderer := &mockRenderer{}
	handler := newTestHandler(t, openTestDB(t), renderer)

	request := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()

	handler.Authenticate(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusForbidden)
	}

	dataMap, ok := renderer.lastData.(map[string]any)
	if !ok {
		t.Fatal("render data is not map[string]any")
	}

	authError, hasError := dataMap["Error"]
	if !hasError {
		t.Error("expected \"Error\" key in render data for empty fields, got none")
	}

	if authError != "invalid credentials" {
		t.Errorf("auth error: got %s, want %s", authError, "invalid credentials")
	}
}

func TestAuthenticate_InternalFailure_Returns500WithInternalErrorMessage(t *testing.T) {
	database := openTestDB(t)
	insertTestUser(t, database, "auth-internal", "lets all love lain")
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	form := url.Values{"username": {"auth-internal"}, "password": {"lets all love lain"}}

	renderer := &mockRenderer{}
	handler := newTestHandler(t, database, renderer)

	request := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	handler.Authenticate(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	dataMap, ok := renderer.lastData.(map[string]any)
	if !ok {
		t.Fatal("render data is not map[string]any")
	}

	if authError := dataMap["Error"]; authError != "internal server error" {
		t.Errorf("auth error: got %v, want %q", authError, "internal server error")
	}
}

func TestAuthenticate_RendererErrorOnFailurePath_Returns500WithGenericBody(t *testing.T) {
	form := url.Values{"username": {"unknown"}, "password": {"whatever"}}

	renderer := &mockRenderer{err: errors.New("template failure")}
	handler := newTestHandler(t, openTestDB(t), renderer)

	request := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	handler.Authenticate(recorder, request)

	// The 403 status is written before renderLogin is called, so the renderer failure cannot override it.
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusForbidden)
	}

	if body := recorder.Body.String(); body != "internal server error\n" {
		t.Errorf("body: got %q, want %q (no error leak to client)", body, "internal server error\n")
	}
}

func TestLogin_RendererError_Returns500WithGenericBody(t *testing.T) {
	renderer := &mockRenderer{err: errors.New("template failure")}
	handler := newTestHandler(t, openTestDB(t), renderer)

	recorder := httptest.NewRecorder()
	handler.Login(recorder, httptest.NewRequest(http.MethodGet, "/auth", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	if body := recorder.Body.String(); body != "internal server error\n" {
		t.Errorf("body: got %q, want %q (no error leak to client)", body, "internal server error\n")
	}
}

func TestLogout_DeletesSessionAndClearsCookie(t *testing.T) {
	database := openTestDB(t)
	userID := insertTestUser(t, database, "auth-logout", "owo")

	// Insert a session with a known raw token so we can verify deletion.
	rawToken := strings.Repeat("letsalllovelain", 6)
	storedHash := authservice.HashToken(rawToken)

	if _, err := testfactory.CreateSession(context.Background(), database, testfactory.SessionSeed{
		TokenHash: storedHash,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	renderer := &mockRenderer{}
	handler := newTestHandler(t, database, renderer)

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})

	recorder := httptest.NewRecorder()

	handler.Logout(recorder, request)

	if recorder.Code != http.StatusSeeOther {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusSeeOther)
	}

	if location := recorder.Header().Get("Location"); location != "/auth" {
		t.Errorf("redirect location: got %q, want %q", location, "/auth")
	}

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE token_hash = ?", storedHash,
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if count != 0 {
		t.Errorf("session not deleted from database after logout")
	}

	var clearedCookie *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "session" {
			clearedCookie = cookie
			break
		}
	}

	if clearedCookie == nil {
		t.Fatal("no session cookie in logout response")
	}

	if clearedCookie.MaxAge != -1 {
		t.Errorf("cookie MaxAge: got %d, want -1", clearedCookie.MaxAge)
	}
}

func TestRoutes_ProtectedEndpointsRejectUnauthenticatedRequests(t *testing.T) {
	database := openTestDB(t)
	handler := newTestHandler(t, database, &mockRenderer{})

	router := chi.NewRouter()
	auth.Routes(handler)(router)

	tests := []struct {
		name           string
		method         string
		path           string
		hxRequest      bool
		wantStatus     int
		wantRedirect   string
		wantHXRedirect string
	}{
		{
			name:         "logout normal",
			method:       http.MethodPost,
			path:         "/auth/logout",
			wantStatus:   http.StatusSeeOther,
			wantRedirect: "/",
		},
		{
			name:           "logout htmx",
			method:         http.MethodPost,
			path:           "/auth/logout",
			hxRequest:      true,
			wantStatus:     http.StatusUnauthorized,
			wantHXRedirect: "/",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if test.hxRequest {
				request.Header.Set("HX-Request-Type", "partial")
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Errorf("status: got %d, want %d", recorder.Code, test.wantStatus)
			}

			if location := recorder.Header().Get("Location"); location != test.wantRedirect {
				t.Errorf("Location: got %q, want %q", location, test.wantRedirect)
			}

			if hxRedirect := recorder.Header().Get("HX-Redirect"); hxRedirect != test.wantHXRedirect {
				t.Errorf("HX-Redirect: got %q, want %q", hxRedirect, test.wantHXRedirect)
			}
		})
	}
}

func TestRoutes_LogoutAuthenticated_DeletesSessionAndClearsCookie(t *testing.T) {
	database := openTestDB(t)
	userID := insertTestUser(t, database, "auth-logout-auth", "owo")

	rawToken := strings.Repeat("letsalllovelain", 6)
	storedHash := authservice.HashToken(rawToken)

	if _, err := testfactory.CreateSession(context.Background(), database, testfactory.SessionSeed{
		TokenHash: storedHash,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := newTestHandler(t, database, &mockRenderer{})

	router := chi.NewRouter()
	auth.Routes(handler)(router)

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusSeeOther {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusSeeOther)
	}

	if location := recorder.Header().Get("Location"); location != "/auth" {
		t.Errorf("redirect location: got %q, want %q", location, "/auth")
	}

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE token_hash = ?", storedHash,
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if count != 0 {
		t.Errorf("session not deleted from database after logout")
	}

	var clearedCookie *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "session" {
			clearedCookie = cookie
			break
		}
	}

	if clearedCookie == nil {
		t.Fatal("no session cookie in logout response")
	}

	if clearedCookie.MaxAge != -1 {
		t.Errorf("cookie MaxAge: got %d, want -1", clearedCookie.MaxAge)
	}
}

func TestLogout_HTMXRequest_RespondsWithHXRedirectAndOK(t *testing.T) {
	database := openTestDB(t)
	userID := insertTestUser(t, database, "auth-logout-htmx", "owo")

	rawToken := strings.Repeat("letsalllovelain", 6)
	storedHash := authservice.HashToken(rawToken)

	if _, err := testfactory.CreateSession(context.Background(), database, testfactory.SessionSeed{
		TokenHash: storedHash,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := newTestHandler(t, database, &mockRenderer{})

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: rawToken})
	request.Header.Set("HX-Request-Type", "partial")

	recorder := httptest.NewRecorder()
	handler.Logout(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if hxRedirect := recorder.Header().Get("HX-Redirect"); hxRedirect != "/auth" {
		t.Errorf("HX-Redirect: got %q, want %q", hxRedirect, "/auth")
	}

	if location := recorder.Header().Get("Location"); location != "" {
		t.Errorf("Location header should be empty for HTMX logout, got %q", location)
	}

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE token_hash = ?", storedHash,
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if count != 0 {
		t.Errorf("session not deleted from database after logout")
	}
}
