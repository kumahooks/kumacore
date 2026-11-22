package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"kumacore/core/httpx"
)

func findCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()

	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}

	t.Fatalf("cookie %q not set", name)

	return nil
}

func TestSetCookie_WritesValueAndExpiry(t *testing.T) {
	recorder := httptest.NewRecorder()
	expiresAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	httpx.SetCookie(recorder, "session", "token-abc", expiresAt, true)

	cookie := findCookie(t, recorder, "session")

	if cookie.Value != "token-abc" {
		t.Errorf("value: got %q, want %q", cookie.Value, "token-abc")
	}

	if !cookie.Expires.Equal(expiresAt) {
		t.Errorf("expires: got %v, want %v", cookie.Expires, expiresAt)
	}
}

func TestSetCookie_SetsHardenedAttributes(t *testing.T) {
	recorder := httptest.NewRecorder()

	httpx.SetCookie(recorder, "session", "v", time.Now().Add(time.Hour), true)

	cookie := findCookie(t, recorder, "session")

	if cookie.Path != "/" {
		t.Errorf("path: got %q, want %q", cookie.Path, "/")
	}

	if !cookie.HttpOnly {
		t.Error("HttpOnly: got false, want true")
	}

	if !cookie.Secure {
		t.Error("Secure: got false, want true")
	}

	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite: got %v, want %v", cookie.SameSite, http.SameSiteStrictMode)
	}
}

func TestSetCookie_SecureFalseForDev(t *testing.T) {
	recorder := httptest.NewRecorder()

	httpx.SetCookie(recorder, "session", "v", time.Now().Add(time.Hour), false)

	cookie := findCookie(t, recorder, "session")

	if cookie.Secure {
		t.Error("Secure: got true, want false for dev")
	}
}

func TestClearCookie_ExpiresAndMaxAgesOut(t *testing.T) {
	recorder := httptest.NewRecorder()

	httpx.ClearCookie(recorder, "session", true)

	cookie := findCookie(t, recorder, "session")

	if cookie.Value != "" {
		t.Errorf("value: got %q, want empty", cookie.Value)
	}

	if cookie.MaxAge != -1 {
		t.Errorf("MaxAge: got %d, want -1", cookie.MaxAge)
	}

	if !cookie.Expires.Before(time.Now()) {
		t.Errorf("expires: got %v, want in the past", cookie.Expires)
	}
}

func TestClearCookie_MatchesSetCookieAttributes(t *testing.T) {
	for _, secure := range []bool{true, false} {
		t.Run("secure="+strconv.FormatBool(secure), func(t *testing.T) {
			setRecorder := httptest.NewRecorder()
			clearRecorder := httptest.NewRecorder()

			httpx.SetCookie(setRecorder, "session", "v", time.Now().Add(time.Hour), secure)
			httpx.ClearCookie(clearRecorder, "session", secure)

			setCookie := findCookie(t, setRecorder, "session")
			clearCookie := findCookie(t, clearRecorder, "session")

			if clearCookie.Path != setCookie.Path {
				t.Errorf("path: clear %q, set %q", clearCookie.Path, setCookie.Path)
			}

			if clearCookie.HttpOnly != setCookie.HttpOnly {
				t.Errorf("HttpOnly: clear %v, set %v", clearCookie.HttpOnly, setCookie.HttpOnly)
			}

			if clearCookie.Secure != setCookie.Secure {
				t.Errorf("Secure: clear %v, set %v", clearCookie.Secure, setCookie.Secure)
			}

			if clearCookie.SameSite != setCookie.SameSite {
				t.Errorf("SameSite: clear %v, set %v", clearCookie.SameSite, setCookie.SameSite)
			}
		})
	}
}

func TestSetCookie_DistinctNamesCoexist(t *testing.T) {
	recorder := httptest.NewRecorder()

	httpx.SetCookie(recorder, "session", "s", time.Now().Add(time.Hour), true)
	httpx.SetCookie(recorder, "csrf", "c", time.Now().Add(time.Hour), true)

	names := map[string]bool{}
	for _, cookie := range recorder.Result().Cookies() {
		names[cookie.Name] = true
	}

	if !names["session"] {
		t.Error("session cookie missing")
	}

	if !names["csrf"] {
		t.Error("csrf cookie missing")
	}
}
