// Package log configures the slog default logger and provides request-ID propagation.
package log

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type contextKey struct{}

// maxInboundRequestIDLen bounds the accepted length of an inbound X-Request-Id header. Larger values are truncated.
const maxInboundRequestIDLen = 64

// Setup builds the slog default logger writing to writer, with level determined by isDevelopment (Debug in dev, Info in
// prod), and returns the configured logger. The handler chain injects request_id from the record's context when one is
// present.
func Setup(writer io.Writer, isDevelopment bool) *slog.Logger {
	level := slog.LevelInfo
	if isDevelopment {
		level = slog.LevelDebug
	}

	baseHandler := newFormatHandler(writer, level)

	logger := slog.New(requestIDHandler{inner: baseHandler})
	slog.SetDefault(logger)

	return logger
}

// RequestIDMiddleware reads X-Request-Id from the inbound request (generating one if absent, truncating oversize values),
// stores it in the request context, and echoes it on the response.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		id := request.Header.Get("X-Request-Id")
		if len(id) > maxInboundRequestIDLen {
			id = id[:maxInboundRequestIDLen]
		}

		if id == "" {
			id = generateRequestID()
		}

		writer.Header().Set("X-Request-Id", id)

		ctx := WithRequestID(request.Context(), id)

		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

// RequestLogger emits one slog.InfoContext line per request with method, path, status, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()

		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)

		slog.InfoContext(
			request.Context(),
			"",
			slog.String("caller", "http:RequestLogger"),
			slog.String("method", request.Method),
			slog.String("path", request.URL.RequestURI()),
			slog.Int("status", recorder.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// WithRequestID returns a context carrying the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}

	return context.WithValue(ctx, contextKey{}, id)
}

// RequestIDFromContext returns the request ID stored in ctx, or ("", false) when none is present
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKey{}).(string)
	return id, ok
}

func generateRequestID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)

	return hex.EncodeToString(buffer)
}

type requestIDHandler struct {
	inner slog.Handler
}

func (handler requestIDHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.inner.Enabled(ctx, level)
}

func (handler requestIDHandler) Handle(ctx context.Context, record slog.Record) error {
	if id, ok := RequestIDFromContext(ctx); ok {
		record.AddAttrs(slog.String("request_id", id))
	}

	return handler.inner.Handle(ctx, record)
}

func (handler requestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDHandler{inner: handler.inner.WithAttrs(attrs)}
}

// WithGroup is a no-op.
func (handler requestIDHandler) WithGroup(_ string) slog.Handler {
	return handler
}

// statusRecorder captures the response status for RequestLogger. The embedded http.ResponseWriter keeps WriteHeader
// passthrough. Only the status field is recorded.
//
// Unwrap exposes the underlying writer to http.ResponseController so optional extensions (SetWriteDeadline, Hijack,
// Flush) reach the real connection. Without it, the controller walk stops at statusRecorder and methods like SetWriteDeadline
// used by user-file serving paths return ErrNotSupported.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(code int) {
	recorder.status = code
	recorder.ResponseWriter.WriteHeader(code)
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}
