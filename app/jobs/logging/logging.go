// Package logging provides the background cleanup logic for old log files.
package logging

import (
	"context"
	"log"
)

// Cleanup deletes log files older than the retention period.
func Cleanup(_ context.Context, dir string) error {
	deletedLogCount, err := deleteOldLogs(dir)
	if err != nil {
		return err
	}

	if deletedLogCount > 0 {
		log.Printf("[logging:Cleanup] deleted %d old log file(s)", deletedLogCount)
	}

	return nil
}
