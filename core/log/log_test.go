package log

import (
	"bytes"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupInstallsDefaultLogger(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	var buffer bytes.Buffer
	logger := Setup(&buffer, true)

	if logger != slog.Default() {
		t.Fatal("Setup: returned logger is not the default logger")
	}

	logger.Debug("debug line", slog.String("caller", "test:Setup"))

	if !strings.Contains(buffer.String(), "[DEBUG - test:Setup] debug line") {
		t.Fatalf("Setup: debug line missing:\n%s", buffer.String())
	}
}

func TestSetupSuppressesDebugOutsideDevelopment(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	var buffer bytes.Buffer
	logger := Setup(&buffer, false)

	logger.Debug("debug line", slog.String("caller", "test:Setup"))
	logger.Info("info line", slog.String("caller", "test:Setup"))

	output := buffer.String()
	if strings.Contains(output, "debug line") {
		t.Fatalf("Setup: debug line must be suppressed:\n%s", output)
	}

	if !strings.Contains(output, "info line") {
		t.Fatalf("Setup: info line missing:\n%s", output)
	}
}

func TestSetupInjectsRequestIDFromContext(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	var buffer bytes.Buffer
	logger := Setup(&buffer, true)

	ctx := WithRequestID(t.Context(), "request-123")
	logger.InfoContext(ctx, "with id", slog.String("caller", "test:Setup"))

	if !strings.Contains(buffer.String(), "request_id=request-123") {
		t.Fatalf("Setup: request_id missing:\n%s", buffer.String())
	}
}

func TestSetupOmitsRequestIDWithoutContext(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	var buffer bytes.Buffer
	logger := Setup(&buffer, true)

	logger.Info("without id", slog.String("caller", "test:Setup"))

	if strings.Contains(buffer.String(), "request_id") {
		t.Fatalf("Setup: unexpected request_id:\n%s", buffer.String())
	}
}

func TestWithRequestIDIgnoresEmptyID(t *testing.T) {
	ctx := t.Context()

	if withID := WithRequestID(ctx, ""); withID != ctx {
		t.Fatal("WithRequestID: empty id must return the original context")
	}

	if _, ok := RequestIDFromContext(WithRequestID(ctx, "abc")); !ok {
		t.Fatal("RequestIDFromContext: id must be present")
	}

	if _, ok := RequestIDFromContext(ctx); ok {
		t.Fatal("RequestIDFromContext: id must be absent")
	}
}

func TestRequestIDMiddlewareGeneratesID(t *testing.T) {
	var seenID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenID, _ = RequestIDFromContext(request.Context())
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	responseID := recorder.Header().Get("X-Request-Id")
	if responseID == "" {
		t.Fatal("RequestIDMiddleware: response header is empty")
	}

	if seenID != responseID {
		t.Fatalf("RequestIDMiddleware: context id %q differs from header %q", seenID, responseID)
	}

	decoded, err := hex.DecodeString(responseID)
	if err != nil {
		t.Fatalf("RequestIDMiddleware: id is not hex: %v", err)
	}

	if len(decoded) != 16 {
		t.Fatalf("RequestIDMiddleware: id length: got %d bytes, want 16", len(decoded))
	}
}

func TestRequestIDMiddlewarePreservesInboundID(t *testing.T) {
	var seenID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenID, _ = RequestIDFromContext(request.Context())
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-Id", "inbound-id")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if seenID != "inbound-id" {
		t.Fatalf("RequestIDMiddleware: context id: got %q, want %q", seenID, "inbound-id")
	}

	if responseID := recorder.Header().Get("X-Request-Id"); responseID != "inbound-id" {
		t.Fatalf("RequestIDMiddleware: response id: got %q, want %q", responseID, "inbound-id")
	}
}

func TestRequestIDMiddlewareTruncatesInboundID(t *testing.T) {
	oversizeID := strings.Repeat("a", maxInboundRequestIDLen+10)

	var seenID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenID, _ = RequestIDFromContext(request.Context())
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-Id", oversizeID)

	handler.ServeHTTP(httptest.NewRecorder(), request)

	if len(seenID) != maxInboundRequestIDLen {
		t.Fatalf("RequestIDMiddleware: truncated id length: got %d, want %d", len(seenID), maxInboundRequestIDLen)
	}

	if seenID != oversizeID[:maxInboundRequestIDLen] {
		t.Fatalf("RequestIDMiddleware: truncated id: got %q", seenID)
	}
}

func TestRequestLoggerEmitsOneLine(t *testing.T) {
	buffer := testLoggerBuffer(t, slog.LevelInfo)

	handler := RequestLogger(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items?page=2", nil))

	output := buffer.String()
	if lines := strings.Count(output, "\n"); lines != 1 {
		t.Fatalf("RequestLogger: line count: got %d, want 1\n%s", lines, output)
	}

	for _, fragment := range []string{
		"[INFO - http:RequestLogger]",
		"method=GET",
		"path=\"/items?page=2\"",
		"status=201",
		"duration=",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("RequestLogger: output missing %q:\n%s", fragment, output)
		}
	}
}

func TestRequestLoggerCapturesImplicitStatus(t *testing.T) {
	buffer := testLoggerBuffer(t, slog.LevelInfo)

	handler := RequestLogger(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buffer.String(), "status=200") {
		t.Fatalf("RequestLogger: implicit status missing:\n%s", buffer.String())
	}
}

func TestRequestLoggerIncludesRequestID(t *testing.T) {
	buffer := testLoggerBuffer(t, slog.LevelInfo)

	handler := RequestIDMiddleware(RequestLogger(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buffer.String(), "request_id=") {
		t.Fatalf("RequestLogger: request_id missing:\n%s", buffer.String())
	}
}

func TestStatusRecorderUnwrap(t *testing.T) {
	recorder := httptest.NewRecorder()
	wrapped := &statusRecorder{ResponseWriter: recorder, status: http.StatusOK}

	if wrapped.Unwrap() != recorder {
		t.Fatal("statusRecorder.Unwrap: did not return the underlying writer")
	}
}

// testLoggerBuffer installs a format handler behind the request ID handler as the slog default logger and returns the
// buffer receiving its output.
func testLoggerBuffer(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()

	previousLogger := slog.Default()
	buffer := &bytes.Buffer{}
	slog.SetDefault(slog.New(requestIDHandler{inner: newFormatHandler(buffer, level)}))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	return buffer
}
