package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogWriterRotatesToUTCDateFile(t *testing.T) {
	directory := t.TempDir()

	staleFile, err := os.OpenFile(filepath.Join(directory, "stale.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open stale log file: %v", err)
	}

	writer := &logWriter{dir: directory, date: "1970-01-01", file: staleFile}
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Write([]byte("first")); err != nil {
		t.Fatalf("Write first: %v", err)
	}

	if _, err := writer.Write([]byte("second")); err != nil {
		t.Fatalf("Write second: %v", err)
	}

	todayPath := filepath.Join(directory, time.Now().UTC().Format("2006-01-02")+"_app.log")
	contents, err := os.ReadFile(todayPath)
	if err != nil {
		t.Fatalf("read %s: %v", todayPath, err)
	}

	if string(contents) != "firstsecond" {
		t.Fatalf("log contents: got %q, want %q", string(contents), "firstsecond")
	}
}

func TestLogWriterKeepsSameFileWithinDate(t *testing.T) {
	directory := t.TempDir()

	filePath := filepath.Join(directory, time.Now().UTC().Format("2006-01-02")+"_app.log")
	logFile, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}

	writer := &logWriter{dir: directory, date: time.Now().UTC().Format("2006-01-02"), file: logFile}

	if _, err := writer.Write([]byte("first")); err != nil {
		t.Fatalf("Write first: %v", err)
	}

	if _, err := writer.Write([]byte("second")); err != nil {
		t.Fatalf("Write second: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	contents, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}

	if string(contents) != "firstsecond" {
		t.Fatalf("log contents: got %q, want %q", string(contents), "firstsecond")
	}

	directoryEntries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(directoryEntries) != 1 {
		t.Fatalf("log directory entries: got %d, want 1", len(directoryEntries))
	}
}
