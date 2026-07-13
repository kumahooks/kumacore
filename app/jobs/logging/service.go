package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const logRetentionDays = 30

func deleteOldLogs(dir string) (int, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, fmt.Errorf("[logging:deleteOldLogs] read log dir: %w", err)
	}

	cutoffDate := time.Now().UTC().AddDate(0, 0, -logRetentionDays)
	deletedLogCount := 0

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		name := dirEntry.Name()
		if !strings.HasSuffix(name, "_app.log") {
			continue
		}

		dateString := strings.TrimSuffix(name, "_app.log")
		logDate, err := time.Parse("2006-01-02", dateString)
		if err != nil {
			continue
		}

		if logDate.Before(cutoffDate) {
			logPath := filepath.Join(dir, name)
			if err := os.Remove(logPath); err != nil {
				log.Printf("[logging:deleteOldLogs] remove %s: %v", name, err)
				continue
			}

			deletedLogCount++
		}
	}

	return deletedLogCount, nil
}
