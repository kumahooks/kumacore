// Package logging provides the background cleanup logic for old log files.
package logging

import (
	"context"
	"log/slog"
)

// Cleanup deletes log files older than the retention period.
func Cleanup(ctx context.Context, dir string) error {
	deletedLogCount, err := deleteOldLogs(dir)
	if err != nil {
		return err
	}

	if deletedLogCount > 0 {
		slog.InfoContext(
			ctx, "deleted old log files",
			slog.String("caller", "logging:Cleanup"),
			slog.Int("count", deletedLogCount),
		)
	}

	return nil
}
