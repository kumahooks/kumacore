package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"modernc.org/sqlite"
)

// backuper is the online-backup interface satisfied by the modernc.org/sqlite
// driver connection.
type backuper interface {
	NewBackup(destinationURI string) (*sqlite.Backup, error)
}

// BackupTo writes a snapshot of database to destinationPath using the SQLite
// online backup API.
//
// ref: https://www.sqlite.org/backup.html
func BackupTo(ctx context.Context, database *sql.DB, destinationPath string) error {
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("[sqlite:BackupTo] acquire conn: %w", err)
	}
	defer conn.Close()

	err = conn.Raw(func(driverConnection any) error {
		backupSource, ok := driverConnection.(backuper)
		if !ok {
			return fmt.Errorf("[sqlite:BackupTo] driver connection does not implement backuper")
		}

		backup, err := backupSource.NewBackup(destinationPath)
		if err != nil {
			return fmt.Errorf("[sqlite:BackupTo] init backup: %w", err)
		}

		for {
			more, err := backup.Step(-1)
			if err != nil {
				_ = backup.Finish()
				return fmt.Errorf("[sqlite:BackupTo] step: %w", err)
			}

			if !more {
				break
			}
		}

		if err := backup.Finish(); err != nil {
			return fmt.Errorf("[sqlite:BackupTo] finish: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
