package log

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestFormatHandler_WritesSingleLine(t *testing.T) {
	var buffer bytes.Buffer
	handler := newFormatHandler(&buffer, slog.LevelDebug)

	recordTime := time.Date(2026, time.August, 7, 10, 52, 56, 120*int(time.Millisecond), time.FixedZone("UTC-3", -3*60*60))
	record := slog.NewRecord(recordTime, slog.LevelInfo, "job started", 0)
	record.AddAttrs(
		slog.String("caller", "worker:executeQueued"),
		slog.String("job", "backup:daily"),
		slog.Int("attempt", 2),
	)

	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	expected := "(2026-08-07T10:52:56.120-03:00) [INFO - worker:executeQueued] job started job=backup:daily attempt=2\n"
	if buffer.String() != expected {
		t.Fatalf("Handle output:\ngot  %q\nwant %q", buffer.String(), expected)
	}
}

func TestFormatHandler_OmitsEmptyMessage(t *testing.T) {
	var buffer bytes.Buffer
	handler := newFormatHandler(&buffer, slog.LevelDebug)

	recordTime := time.Date(2026, time.August, 7, 10, 52, 56, 0, time.UTC)
	record := slog.NewRecord(recordTime, slog.LevelInfo, "", 0)
	record.AddAttrs(slog.String("caller", "http:RequestLogger"), slog.String("method", "GET"))

	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	expected := "(2026-08-07T10:52:56.000+00:00) [INFO - http:RequestLogger] method=GET\n"
	if buffer.String() != expected {
		t.Fatalf("Handle output:\ngot  %q\nwant %q", buffer.String(), expected)
	}
}

func TestFormatHandler_HoistsCallerFromPreAttrs(t *testing.T) {
	var buffer bytes.Buffer
	handler := newFormatHandler(&buffer, slog.LevelDebug).WithAttrs([]slog.Attr{
		slog.String("caller", "app:Initialize"),
		slog.Bool("dev", true),
	})

	record := slog.NewRecord(time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC), slog.LevelWarn, "message", 0)
	record.AddAttrs(slog.String("caller", "app:Initialize"))

	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	expected := "(2026-08-07T12:00:00.000+00:00) [WARN - app:Initialize] message dev=true\n"
	if buffer.String() != expected {
		t.Fatalf("Handle output:\ngot  %q\nwant %q", buffer.String(), expected)
	}
}

func TestFormatHandler_QuotesAmbiguousStrings(t *testing.T) {
	var buffer bytes.Buffer
	handler := newFormatHandler(&buffer, slog.LevelDebug)

	record := slog.NewRecord(time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC), slog.LevelError, "failed", 0)
	record.AddAttrs(
		slog.String("caller", "backup:Run"),
		slog.String("spaced", "two words"),
		slog.String("equals", "key=value"),
		slog.String("empty", ""),
		slog.String("plain", "plain"),
	)

	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	expected := `(2026-08-07T12:00:00.000+00:00) [ERROR - backup:Run] failed spaced="two words" equals="key=value" empty="" plain=plain` + "\n"
	if buffer.String() != expected {
		t.Fatalf("Handle output:\ngot  %q\nwant %q", buffer.String(), expected)
	}
}

func TestFormatHandler_FormatsScalarKinds(t *testing.T) {
	var buffer bytes.Buffer
	handler := newFormatHandler(&buffer, slog.LevelDebug)

	recordTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	record := slog.NewRecord(recordTime, slog.LevelInfo, "kinds", 0)
	record.AddAttrs(
		slog.String("caller", "test:Formats"),
		slog.Int64("int", -7),
		slog.Uint64("uint", 7),
		slog.Float64("float", 1.5),
		slog.Bool("bool", false),
		slog.Duration("duration", 1500*time.Millisecond),
		slog.Time("time", recordTime),
		slog.Any("err", errors.New("boom")),
	)

	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	output := buffer.String()
	for _, fragment := range []string{
		"int=-7",
		"uint=7",
		"float=1.5",
		"bool=false",
		"duration=1.5s",
		"time=2026-08-07T12:00:00.000+00:00",
		"err=boom",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("Handle output missing %q:\n%s", fragment, output)
		}
	}
}

func TestFormatHandler_RespectsLevel(t *testing.T) {
	handler := newFormatHandler(&bytes.Buffer{}, slog.LevelInfo)

	if handler.Enabled(t.Context(), slog.LevelDebug) {
		t.Fatal("Enabled: debug must be disabled at info level")
	}

	if !handler.Enabled(t.Context(), slog.LevelInfo) {
		t.Fatal("Enabled: info must be enabled at info level")
	}
}

func TestFormatHandler_WithAttrsKeepsWriterShared(t *testing.T) {
	var buffer bytes.Buffer
	handler := newFormatHandler(&buffer, slog.LevelDebug)
	derived := handler.WithAttrs([]slog.Attr{slog.String("caller", "test:Derived")})

	record := slog.NewRecord(time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC), slog.LevelInfo, "", 0)
	if err := derived.Handle(t.Context(), record); err != nil {
		t.Fatalf("derived Handle: %v", err)
	}

	record = slog.NewRecord(time.Date(2026, time.August, 7, 12, 0, 1, 0, time.UTC), slog.LevelInfo, "", 0)
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("base Handle: %v", err)
	}

	if lines := strings.Count(buffer.String(), "\n"); lines != 2 {
		t.Fatalf("Handle lines: got %d, want 2\n%s", lines, buffer.String())
	}
}
