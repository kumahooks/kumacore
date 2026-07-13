// Package backup provides the background backup and retention jobs for the
// /data/ directory.
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

const SuffixDaily = "daily"

const SuffixWeekly = "weekly"

// maxRetainedPerSuffix caps how many backups of a given suffix are kept after
// the count-based cleanup pass.
const maxRetainedPerSuffix = 3

// Run creates a backup zip of dataDir in backupDir named with the current UTC
// date and the given suffix, then prunes older backups of the same suffix past
// the retention age. appDatabase is always snapshotted; workerDatabase is
// snapshotted when non-nil and skipped when nil (e.g. WORKER_ENABLED=false).
func Run(
	ctx context.Context,
	appDatabase *sql.DB,
	workerDatabase *sql.DB,
	dataDir string,
	backupDir string,
	suffix string,
) error {
	maxAge, err := suffixMaxAge(suffix)
	if err != nil {
		return err
	}

	zipPath, err := createBackup(ctx, appDatabase, workerDatabase, dataDir, backupDir, suffix)
	if err != nil {
		return err
	}

	log.Printf("[backup:Run] created %s", zipPath)

	deletedCount, err := pruneByDate(backupDir, suffix, maxAge)
	if err != nil {
		return err
	}

	if deletedCount > 0 {
		log.Printf("[backup:Run] pruned %d old %s backup(s)", deletedCount, suffix)
	}

	return nil
}

// Cleanup removes stray files in backupDir that do not match the backup
// filename pattern, then enforces the count-based retention: if there are more
// than maxRetainedPerSuffix backups of a given suffix, the oldest are deleted
// down to the cap.
func Cleanup(_ context.Context, backupDir string) error {
	strayDeletedCount, err := deleteStrayFiles(backupDir)
	if err != nil {
		return err
	}

	if strayDeletedCount > 0 {
		log.Printf("[backup:Cleanup] deleted %d stray file(s)", strayDeletedCount)
	}

	totalPruned, err := pruneByCount(backupDir, SuffixDaily, maxRetainedPerSuffix)
	if err != nil {
		return err
	}

	weeklyPruned, err := pruneByCount(backupDir, SuffixWeekly, maxRetainedPerSuffix)
	if err != nil {
		return err
	}

	totalPruned += weeklyPruned

	if totalPruned > 0 {
		log.Printf("[backup:Cleanup] pruned %d excess backup(s)", totalPruned)
	}

	return nil
}

// suffixMaxAge maps a backup suffix to the retention age beyond which older
// backups of that suffix are deleted after a new one is created.
func suffixMaxAge(suffix string) (time.Duration, error) {
	switch suffix {
	case SuffixDaily:
		return (maxRetainedPerSuffix + 1) * 24 * time.Hour, nil
	case SuffixWeekly:
		return (maxRetainedPerSuffix + 1) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("[backup:suffixMaxAge] unknown suffix %q", suffix)
	}
}
