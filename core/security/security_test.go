package security_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kumacore/core/security"
)

func TestSecurity_SetsXFrameOptions(t *testing.T) {
	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options: got %q, want %q", got, "DENY")
	}
}

func TestSecurity_SetsContentSecurityPolicy(t *testing.T) {
	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	want := "default-src 'self'; style-src 'self' 'sha256-faU7yAF8NxuMTNEwVmBz+VcYeIoBQ2EMHW3WaVxCvnk=' 'sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU='; style-src-attr 'unsafe-inline'; script-src 'self'; img-src 'self'"
	if got := recorder.Header().Get("Content-Security-Policy"); got != want {
		t.Errorf("Content-Security-Policy: got %q, want %q", got, want)
	}
}

func TestSecurity_CallsNextHandler(t *testing.T) {
	called := false
	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Error("next handler was not called")
	}
}
