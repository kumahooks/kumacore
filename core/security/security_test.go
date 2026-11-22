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

	want := "default-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; style-src 'self' 'sha256-faU7yAF8NxuMTNEwVmBz+VcYeIoBQ2EMHW3WaVxCvnk=' 'sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU='; style-src-attr 'unsafe-inline'; script-src 'self'; img-src 'self'"
	if got := recorder.Header().Get("Content-Security-Policy"); got != want {
		t.Errorf("Content-Security-Policy: got %q, want %q", got, want)
	}
}

func TestSecurity_SetsPermissionsPolicy(t *testing.T) {
	handler := security.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	want := "camera=(), microphone=(), geolocation=(), payment=(), usb=()"
	if got := recorder.Header().Get("Permissions-Policy"); got != want {
		t.Errorf("Permissions-Policy: got %q, want %q", got, want)
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

func TestSameOriginMiddleware_SafeMethodsPass(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
		t.Run(method, func(t *testing.T) {
			called := false
			handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
			}))

			request := httptest.NewRequest(method, "https://kumacore.test/", nil)
			request.Header.Set("Origin", "https://evil.example")

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
			}

			if !called {
				t.Error("next handler was not called")
			}
		})
	}
}

func TestSameOriginMiddleware_UnsafeRequestWithoutHeadersPasses(t *testing.T) {
	called := false
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if !called {
		t.Error("next handler was not called")
	}
}

func TestSameOriginMiddleware_MatchingOriginPasses(t *testing.T) {
	called := false
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Origin", "https://kumacore.test")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if !called {
		t.Error("next handler was not called")
	}
}

func TestSameOriginMiddleware_MatchingRefererPasses(t *testing.T) {
	called := false
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Referer", "https://kumacore.test/")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if !called {
		t.Error("next handler was not called")
	}
}

func TestSameOriginMiddleware_MismatchedOriginRejected(t *testing.T) {
	called := false
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Origin", "https://evil.example")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	if called {
		t.Error("next handler was called for a cross-origin request")
	}
}

func TestSameOriginMiddleware_MismatchedRefererRejected(t *testing.T) {
	called := false
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Referer", "https://evil.example/page")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	if called {
		t.Error("next handler was called for a cross-origin request")
	}
}

func TestSameOriginMiddleware_NullOriginRejected(t *testing.T) {
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Origin", "null")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSameOriginMiddleware_NonHTTPSchemeRejected(t *testing.T) {
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Origin", "ftp://kumacore.test")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSameOriginMiddleware_OriginHostIsCaseInsensitive(t *testing.T) {
	called := false
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	request := httptest.NewRequest(http.MethodPost, "https://KUMACORE.TEST/auth", nil)
	request.Header.Set("Origin", "https://kumacore.test")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if !called {
		t.Error("next handler was not called")
	}
}

func TestSameOriginMiddleware_ForwardedHostIgnored(t *testing.T) {
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:3000/auth", nil)
	request.Header.Set("Origin", "https://kumacore.test")
	request.Header.Set("X-Forwarded-Host", "kumacore.test")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSameOriginMiddleware_CrossSiteFetchMetadataRejected(t *testing.T) {
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Sec-Fetch-Site", "cross-site")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSameOriginMiddleware_OriginWinsOverReferer(t *testing.T) {
	handler := security.SameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	request := httptest.NewRequest(http.MethodPost, "https://kumacore.test/auth", nil)
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Referer", "https://kumacore.test/")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
