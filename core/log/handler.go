package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

// timestampFormat is the strict timestamp prefix used by every record: (2006-01-02T15:04:05.000-07:00)
const timestampFormat = "2006-01-02T15:04:05.000-07:00"

// formatHandler writes slog records in a strict single-line format:
//
//	(2026-08-07T10:52:56.120-03:00) [INFO - caller] message key=value ...
type formatHandler struct {
	writer   io.Writer
	level    slog.Level
	preAttrs []slog.Attr
	mu       *sync.Mutex
}

func newFormatHandler(writer io.Writer, level slog.Level) *formatHandler {
	return &formatHandler{
		writer: writer,
		level:  level,
		mu:     &sync.Mutex{},
	}
}

func (handler *formatHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= handler.level
}

func (handler *formatHandler) Handle(_ context.Context, record slog.Record) error {
	var caller string
	remaining := make([]slog.Attr, 0, len(handler.preAttrs)+record.NumAttrs())

	for _, attr := range handler.preAttrs {
		if attr.Key == "caller" {
			caller = attr.Value.String()
			continue
		}

		remaining = append(remaining, attr)
	}

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "caller" {
			caller = attr.Value.String()
			return true
		}

		remaining = append(remaining, attr)

		return true
	})

	var builder strings.Builder
	builder.WriteByte('(')
	builder.WriteString(record.Time.Format(timestampFormat))
	builder.WriteString(") [")
	builder.WriteString(record.Level.String())
	builder.WriteString(" - ")
	builder.WriteString(caller)
	builder.WriteByte(']')
	if record.Message != "" {
		builder.WriteByte(' ')
		builder.WriteString(record.Message)
	}

	for _, attr := range remaining {
		builder.WriteByte(' ')
		builder.WriteString(attr.Key)
		builder.WriteByte('=')
		builder.WriteString(formatValue(attr.Value))
	}
	builder.WriteByte('\n')

	handler.mu.Lock()
	_, err := handler.writer.Write([]byte(builder.String()))
	handler.mu.Unlock()

	return err
}

func (handler *formatHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, 0, len(handler.preAttrs)+len(attrs))
	combined = append(combined, handler.preAttrs...)
	combined = append(combined, attrs...)

	return &formatHandler{
		writer:   handler.writer,
		level:    handler.level,
		preAttrs: combined,
		mu:       handler.mu,
	}
}

// WithGroup is a no-op.
func (handler *formatHandler) WithGroup(_ string) slog.Handler {
	return handler
}

// formatValue renders a slog.Value as a single token. String-like values are quoted only when they would otherwise be
// ambiguous (spaces, tabs, newlines, "=", or empty). Numeric, duration, and bool kinds use their native string forms
// which never need quoting.
func formatValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return formatString(value.String())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(timestampFormat)
	default:
		return formatString(fmt.Sprint(value.Any()))
	}
}

func formatString(value string) string {
	if value == "" || strings.ContainsAny(value, " \t\n\r=") {
		return strconv.Quote(value)
	}

	return value
}
