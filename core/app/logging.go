package app

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type logWriter struct {
	mutex sync.Mutex
	dir   string
	date  string
	file  *os.File
}

// Write writes to a date-prefixed log file, rotating to a new file
// automatically when the calendar date changes.
func (writer *logWriter) Write(payload []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	if today := time.Now().Format("2006-01-02"); today != writer.date {
		path := filepath.Join(writer.dir, today+"_app.log")

		newFile, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			writer.file.Close()

			writer.file = newFile
			writer.date = today
		}
	}

	return writer.file.Write(payload)
}

func (writer *logWriter) Close() error {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	return writer.file.Close()
}

// initLogging opens a date-prefixed log file in the configured directory and
// routes all log output to both stdout and the file, rotating at midnight.
// The caller is responsible for closing the returned io.Closer when done.
func (application *App) initLogging() (io.Closer, error) {
	logDir := application.options.Configuration.Core.Logging.Dir
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("[app:initLogging] create log directory %q: %w", logDir, err)
	}

	date := time.Now().Format("2006-01-02")
	logFilePath := filepath.Join(logDir, date+"_app.log")

	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("[app:initLogging] open log file %q: %w", logFilePath, err)
	}

	rotatingLog := &logWriter{dir: logDir, date: date, file: logFile}
	log.SetOutput(io.MultiWriter(os.Stdout, rotatingLog))

	return rotatingLog, nil
}
