package authmiddleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authmiddleware "kumacore/app/middleware/auth"
	authservice "kumacore/app/services/auth"
)

func TestRequireAuth_PartialRequestSetsHXRedirect(t *testing.T) {
	handler := authmiddleware.RequireAuth(&authservice.Service{})(
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

func TestRequireAuth_DirectRequestRedirects(t *testing.T) {
	handler := authmiddleware.RequireAuth(&authservice.Service{})(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			t.Error("next handler must not be called when session is absent")
		}),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusSeeOther {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusSeeOther)
	}

	if location := recorder.Header().Get("Location"); location != "/" {
		t.Errorf("redirect location: got %q, want %q", location, "/")
	}
}
