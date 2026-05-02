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
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, fmt.Errorf("[logging:job] read log dir: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -logRetentionDays)
	deleted := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, "_app.log") {
			continue
		}

		dateStr := strings.TrimSuffix(name, "_app.log")

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		if date.Before(cutoff) {
			path := filepath.Join(dir, name)
			if err := os.Remove(path); err != nil {
				log.Printf("[logging:job] cleanup: remove %s: %v", name, err)
				continue
			}

			deleted++
		}
	}

	return deleted, nil
}
