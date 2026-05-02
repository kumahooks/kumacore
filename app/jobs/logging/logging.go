// Package logging provides the background cleanup logic for old log files.
package logging

import (
	"context"
	"log"
)

// Cleanup deletes log files older than 30 days from dir.
func Cleanup(_ context.Context, dir string) error {
	deleted, err := deleteOldLogs(dir)
	if err != nil {
		return err
	}

	if deleted > 0 {
		log.Printf("[logging:job] cleanup: deleted %d old log file(s)", deleted)
	}

	return nil
}
