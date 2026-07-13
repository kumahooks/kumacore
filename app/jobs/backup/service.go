package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"kumacore/core/db/sqlite"
)

// backupFilenamePattern matches backup_YYYYMMDD_<suffix>.zip.
var backupFilenamePattern = regexp.MustCompile(`^backup_(\d{8})_(daily|weekly)\.zip$`)

// skipExtensions are file extensions in dataDir that are never copied raw:
// the .db files are replaced by online backup snapshots, an WAL/SHM are
// SQLite-internal stuff.
var skipExtensions = map[string]bool{
	".db":     true,
	".db-shm": true,
	".db-wal": true,
	".zip":    true,
}

// createBackup snapshots both databases to temp files, zips the contents of
// dataDir plus the snapshots into backupDir, and returns the created zip path.
func createBackup(
	ctx context.Context,
	appDatabase *sql.DB,
	workerDatabase *sql.DB,
	dataDir string,
	backupDir string,
	suffix string,
) (string, error) {
	timestamp := time.Now().UTC()
	zipName := fmt.Sprintf("backup_%s_%s.zip", timestamp.Format("20060102"), suffix)
	zipPath := filepath.Join(backupDir, zipName)

	appSnapshot, err := snapshotDatabase(ctx, appDatabase)
	if err != nil {
		return "", fmt.Errorf("[backup:createBackup] snapshot app db: %w", err)
	}
	defer os.Remove(appSnapshot)

	var workerSnapshot string
	if workerDatabase != nil {
		workerSnapshot, err = snapshotDatabase(ctx, workerDatabase)
		if err != nil {
			return "", fmt.Errorf("[backup:createBackup] snapshot worker db: %w", err)
		}
		defer os.Remove(workerSnapshot)
	}

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("[backup:createBackup] create %s: %w", zipPath, err)
	}

	zipWriter := zip.NewWriter(zipFile)

	backupError := func() error {
		if err := addFileToZip(zipWriter, appSnapshot, "app.db"); err != nil {
			return fmt.Errorf("[backup:createBackup] add app.db: %w", err)
		}

		if workerSnapshot != "" {
			if err := addFileToZip(zipWriter, workerSnapshot, "worker.db"); err != nil {
				return fmt.Errorf("[backup:createBackup] add worker.db: %w", err)
			}
		}

		if err := zipDataDir(zipWriter, dataDir); err != nil {
			return fmt.Errorf("[backup:createBackup] zip data dir: %w", err)
		}

		if err := zipWriter.Close(); err != nil {
			return fmt.Errorf("[backup:createBackup] close zip writer: %w", err)
		}

		return nil
	}()

	if backupError != nil {
		_ = zipFile.Close()
		_ = os.Remove(zipPath)

		return "", backupError
	}

	if err := zipFile.Close(); err != nil {
		_ = os.Remove(zipPath)

		return "", fmt.Errorf("[backup:createBackup] close zip file: %w", err)
	}

	return zipPath, nil
}

// snapshotDatabase writes a consistent snapshot of database to a temp file in
// the OS temp dir and returns its path. The caller must remove the file.
func snapshotDatabase(ctx context.Context, database *sql.DB) (string, error) {
	tempFile, err := os.CreateTemp("", "database-backup-*.db")
	if err != nil {
		return "", fmt.Errorf("[backup:snapshotDatabase] create temp: %w", err)
	}

	tempPath := tempFile.Name()
	tempFile.Close()

	if err := sqlite.BackupTo(ctx, database, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	return tempPath, nil
}

// zipDataDir walks dataDir and adds every non-skipped file to zipWriter using
// its path relative to dataDir as the archive entry name.
func zipDataDir(zipWriter *zip.Writer, dataDir string) error {
	return filepath.WalkDir(dataDir, func(path string, dirEntry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("[backup:zipDataDir] data dir does not exist: %s", dataDir)
				return nil
			}

			return err
		}

		if dirEntry.IsDir() {
			return nil
		}

		if skipExtensions[filepath.Ext(path)] {
			return nil
		}

		relativePath, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}

		relativePath = filepath.ToSlash(relativePath)

		if err := addFileToZip(zipWriter, path, relativePath); err != nil {
			return fmt.Errorf("[backup:zipDataDir] add %s: %w", relativePath, err)
		}

		return nil
	})
}

// addFileToZip copies the file at filePath into the zip archive at entry name.
func addFileToZip(zipWriter *zip.Writer, filePath string, name string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = name

	entryWriter, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(entryWriter, file)
	return err
}

// pruneByDate deletes backups in backupDir whose filename date is older than
// maxAge. Only backups matching the given suffix are considered.
func pruneByDate(backupDir string, suffix string, maxAge time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-maxAge)
	deletedCount := 0

	dirEntries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, fmt.Errorf("[backup:pruneByDate] read backup dir: %w", err)
	}

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		name := dirEntry.Name()
		matches := backupFilenamePattern.FindStringSubmatch(name)
		if matches == nil || matches[2] != suffix {
			continue
		}

		backupDate, err := time.Parse("20060102", matches[1])
		if err != nil {
			continue
		}

		if backupDate.Before(cutoff) {
			if err := os.Remove(filepath.Join(backupDir, name)); err != nil {
				log.Printf("[backup:pruneByDate] remove %s: %v", name, err)
				continue
			}

			deletedCount++
		}
	}

	return deletedCount, nil
}

// pruneByCount deletes the oldest backups of a given suffix until at most keep
// remain. Returns the number deleted.
func pruneByCount(backupDir string, suffix string, keep int) (int, error) {
	backups, err := listBackupsBySuffix(backupDir, suffix)
	if err != nil {
		return 0, err
	}

	if len(backups) <= keep {
		return 0, nil
	}

	// backups is sorted oldest first; delete everything past the keep newest.
	deletedCount := 0
	for i := 0; i < len(backups)-keep; i++ {
		if err := os.Remove(filepath.Join(backupDir, backups[i].name)); err != nil {
			log.Printf("[backup:pruneByCount] remove %s: %v", backups[i].name, err)
			continue
		}

		deletedCount++
	}

	return deletedCount, nil
}

// deleteStrayFiles removes files in backupDir that do not match the backup
// filename pattern. Directories are left untouched.
func deleteStrayFiles(backupDir string) (int, error) {
	dirEntries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, fmt.Errorf("[backup:deleteStrayFiles] read backup dir: %w", err)
	}

	deletedCount := 0
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		name := dirEntry.Name()
		if backupFilenamePattern.MatchString(name) {
			continue
		}

		if err := os.Remove(filepath.Join(backupDir, name)); err != nil {
			log.Printf("[backup:deleteStrayFiles] remove %s: %v", name, err)
			continue
		}

		deletedCount++
	}

	return deletedCount, nil
}

// backupEntry pairs a backup filename with its parsed date for sorting.
type backupEntry struct {
	name string
	date time.Time
}

// listBackupsBySuffix returns backups of the given suffix sorted oldest first.
func listBackupsBySuffix(backupDir string, suffix string) ([]backupEntry, error) {
	dirEntries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("[backup:listBackupsBySuffix] read backup dir: %w", err)
	}

	var backups []backupEntry
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		name := dirEntry.Name()
		matches := backupFilenamePattern.FindStringSubmatch(name)
		if matches == nil || matches[2] != suffix {
			continue
		}

		backupDate, err := time.Parse("20060102", matches[1])
		if err != nil {
			continue
		}

		backups = append(backups, backupEntry{name: name, date: backupDate})
	}

	sort.Slice(backups, func(i int, j int) bool {
		return backups[i].date.Before(backups[j].date)
	})

	return backups, nil
}
