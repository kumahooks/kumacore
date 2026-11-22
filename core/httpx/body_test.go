package httpx_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kumacore/core/httpx"
)

func TestLimitRequestBody_UnderLimitReadsBody(t *testing.T) {
	const body = "hello"

	handler := httpx.LimitRequestBody(16)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		if string(content) != body {
			t.Errorf("body: got %q, want %q", string(content), body)
		}
	}))

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestLimitRequestBody_OverLimitReturnsMaxBytesError(t *testing.T) {
	handler := httpx.LimitRequestBody(4)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, err := io.ReadAll(request.Body)

		var maxBytesError *http.MaxBytesError
		if !errors.As(err, &maxBytesError) {
			t.Fatalf("read body: got %v, want *http.MaxBytesError", err)
		}
	}))

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestLimitRequestBody_NonPositiveLimitDisablesCap(t *testing.T) {
	const body = "hello"

	handler := httpx.LimitRequestBody(0)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		if string(content) != body {
			t.Errorf("body: got %q, want %q", string(content), body)
		}
	}))

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	handler.ServeHTTP(httptest.NewRecorder(), request)
}
