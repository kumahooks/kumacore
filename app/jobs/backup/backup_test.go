package backup_test

import (
	"archive/zip"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backup "kumacore/app/jobs/backup"
	"kumacore/app/testing/db"
	"kumacore/core/db/sqlite"
)

var databaseTemplate *db.Template

func TestMain(testingMain *testing.M) {
	os.Exit(db.NewTemplate(testingMain, &databaseTemplate))
}

// newDatabaseToBackup returns a fresh on-disk SQLite database with a marker
// table seeded with marker, so the snapshot has something to verify against.
func newDatabaseToBackup(t *testing.T, marker string) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "lain.db")
	database, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open lain db: %v", err)
	}

	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec("CREATE TABLE marker(value TEXT)"); err != nil {
		t.Fatalf("create marker table: %v", err)
	}

	if _, err := database.Exec("INSERT INTO marker VALUES (?)", marker); err != nil {
		t.Fatalf("insert marker: %v", err)
	}

	return database
}

// writeFile writes a small file at path inside dir.
func writeFile(t *testing.T, dir string, name string, contents string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// hasZipEntry reports whether the zip at path contains an entry with name.
func hasZipEntry(t *testing.T, zipPath string, name string) bool {
	t.Helper()

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == name {
			return true
		}
	}

	return false
}

// snapshotHasMarker opens the entry named "app.db" or "worker.db" inside the
// zip, copies it to a temp file, opens it as SQLite, and checks the marker
// table contains the expected value.
func snapshotHasMarker(t *testing.T, zipPath string, entryName string, expectedMarker string) {
	t.Helper()

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	var found bool
	for _, file := range reader.File {
		if file.Name != entryName {
			continue
		}

		found = true

		readCloser, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", entryName, err)
		}
		defer readCloser.Close()

		snapshotPath := filepath.Join(t.TempDir(), "restored.db")
		snapshotOutput, err := os.Create(snapshotPath)
		if err != nil {
			t.Fatalf("create snapshot file: %v", err)
		}

		if _, err = io.Copy(snapshotOutput, readCloser); err != nil {
			snapshotOutput.Close()
			t.Fatalf("copy snapshot: %v", err)
		}
		snapshotOutput.Close()

		database, err := sqlite.Open(snapshotPath)
		if err != nil {
			t.Fatalf("open snapshot db: %v", err)
		}
		defer database.Close()

		var value string
		if err := database.QueryRow("SELECT value FROM marker").Scan(&value); err != nil {
			t.Fatalf("query marker: %v", err)
		}

		if value != expectedMarker {
			t.Errorf("marker in %s: got %q, want %q", entryName, value, expectedMarker)
		}
	}

	if !found {
		t.Errorf("zip entry %s not found", entryName)
	}
}

// setupDataDir creates a temp directory mimicking /data/ with the db files,
// sqlite internal files, a log file, and a nested noise attachment.
func setupDataDir(t *testing.T) string {
	t.Helper()

	dataDir := t.TempDir()

	// SQLite db + internal files must be skipped by the walker.
	writeFile(t, dataDir, "app.db", "should-be-skipped")
	writeFile(t, dataDir, "app.db-wal", "should-be-skipped")
	writeFile(t, dataDir, "app.db-shm", "should-be-skipped")
	writeFile(t, dataDir, "worker.db", "should-be-skipped")
	writeFile(t, dataDir, "worker.db-wal", "should-be-skipped")

	// Real user content that must be backed up.
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	writeFile(t, logsDir, "2024-01-01_app.log", "log content")

	noiseDir := filepath.Join(dataDir, "noise")
	if err := os.MkdirAll(noiseDir, 0o755); err != nil {
		t.Fatalf("mkdir noise: %v", err)
	}
	writeFile(t, noiseDir, "attachment.webm", "webm bytes")

	// An existing zip must be skipped to avoid recursive backups.
	writeFile(t, dataDir, "stale.zip", "stale")

	return dataDir
}

func TestRun_CreatesBackupZip_WithSnapshotsAndFiles(t *testing.T) {
	appDatabase := newDatabaseToBackup(t, "app-marker")
	workerDatabase := newDatabaseToBackup(t, "worker-marker")

	dataDir := setupDataDir(t)
	backupDir := t.TempDir()

	if err := backup.Run(
		context.Background(),
		appDatabase,
		workerDatabase,
		dataDir,
		backupDir,
		backup.SuffixDaily,
	); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Exactly one zip must exist.
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}

	zipPath := filepath.Join(backupDir, entries[0].Name())

	// The zip must contain the app snapshot, the worker snapshot, and the
	// real data files, but not the raw db/wal/shm/zip files.
	if !hasZipEntry(t, zipPath, "app.db") {
		t.Error("zip must contain app.db snapshot")
	}
	if !hasZipEntry(t, zipPath, "worker.db") {
		t.Error("zip must contain worker.db snapshot")
	}
	if !hasZipEntry(t, zipPath, "logs/2024-01-01_app.log") {
		t.Error("zip must contain logs/2024-01-01_app.log")
	}
	if !hasZipEntry(t, zipPath, "noise/attachment.webm") {
		t.Error("zip must contain noise/attachment.webm")
	}
	if hasZipEntry(t, zipPath, "stale.zip") {
		t.Error("zip must not contain existing .zip files")
	}
	if hasZipEntry(t, zipPath, "app.db-wal") {
		t.Error("zip must not contain raw db sidecar files")
	}
	if hasZipEntry(t, zipPath, "worker.db-wal") {
		t.Error("zip must not contain raw db sidecar files")
	}

	// The snapshots must be real, restorable SQLite databases carrying the
	// marker rows from the live databases.
	snapshotHasMarker(t, zipPath, "app.db", "app-marker")
	snapshotHasMarker(t, zipPath, "worker.db", "worker-marker")
}

func TestRun_SkipsWorkerSnapshot_WhenNil(t *testing.T) {
	appDatabase := newDatabaseToBackup(t, "app-marker")
	dataDir := setupDataDir(t)
	backupDir := t.TempDir()

	if err := backup.Run(
		context.Background(),
		appDatabase,
		nil,
		dataDir,
		backupDir,
		backup.SuffixDaily,
	); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}

	zipPath := filepath.Join(backupDir, entries[0].Name())

	if !hasZipEntry(t, zipPath, "app.db") {
		t.Error("zip must contain app.db snapshot")
	}
	if hasZipEntry(t, zipPath, "worker.db") {
		t.Error("zip must not contain worker.db when worker db is nil")
	}
}

func TestRun_PrunesOldDailyBackups(t *testing.T) {
	appDatabase := newDatabaseToBackup(t, "app-marker")
	dataDir := t.TempDir()
	backupDir := t.TempDir()

	// Seed four old daily backups: 10, 9, 8, 7 days ago. All are 4+ days old,
	// so all four should be pruned after a new one is created.
	now := time.Now().UTC()
	for _, daysAgo := range []int{10, 9, 8, 7} {
		name := "backup_" + now.AddDate(0, 0, -daysAgo).Format("20060102") + "_daily.zip"
		writeFile(t, backupDir, name, "old")
	}

	if err := backup.Run(
		context.Background(),
		appDatabase,
		nil,
		dataDir,
		backupDir,
		backup.SuffixDaily,
	); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	// Only the freshly-created backup must remain.
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup after prune, got %d", len(entries))
	}

	if entries[0].Name() != "backup_"+now.Format("20060102")+"_daily.zip" {
		t.Errorf("expected today's backup to remain, got %s", entries[0].Name())
	}
}

func TestRun_PreservesRecentDailyBackups(t *testing.T) {
	appDatabase := newDatabaseToBackup(t, "app-marker")
	dataDir := t.TempDir()
	backupDir := t.TempDir()

	now := time.Now().UTC()
	// 1 and 2 days old are within the 4-day retention window.
	for _, daysAgo := range []int{2, 1} {
		name := "backup_" + now.AddDate(0, 0, -daysAgo).Format("20060102") + "_daily.zip"
		writeFile(t, backupDir, name, "recent")
	}

	if err := backup.Run(
		context.Background(),
		appDatabase,
		nil,
		dataDir,
		backupDir,
		backup.SuffixDaily,
	); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(entries))
	}
}

func TestRun_DailyAndWeekly_AreIndependent(t *testing.T) {
	appDatabase := newDatabaseToBackup(t, "app-snapshot-marker")
	dataDir := t.TempDir()
	backupDir := t.TempDir()

	now := time.Now().UTC()
	// A 10-day-old weekly backup is within the 4-week retention window and
	// must NOT be pruned by the daily run.
	weeklyName := "backup_" + now.AddDate(0, 0, -10).Format("20060102") + "_weekly.zip"
	writeFile(t, backupDir, weeklyName, "weekly")

	if err := backup.Run(
		context.Background(),
		appDatabase,
		nil,
		dataDir,
		backupDir,
		backup.SuffixDaily,
	); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(backupDir, weeklyName)); err != nil {
		t.Error("weekly backup must not be pruned by daily run")
	}
}

func TestCleanup_DeletesStrayFiles(t *testing.T) {
	backupDir := t.TempDir()

	writeFile(t, backupDir, "README.txt", "stray")
	writeFile(t, backupDir, "random.db", "stray")
	writeFile(t, backupDir, "backup_20240101_daily.zip", "keep")
	writeFile(t, backupDir, "backup_20240108_weekly.zip", "keep")

	if err := backup.Cleanup(context.Background(), backupDir); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if _, err := os.Stat(filepath.Join(backupDir, "README.txt")); !os.IsNotExist(err) {
		t.Error("stray README.txt must be deleted")
	}
	if _, err := os.Stat(filepath.Join(backupDir, "random.db")); !os.IsNotExist(err) {
		t.Error("stray random.db must be deleted")
	}
	if _, err := os.Stat(filepath.Join(backupDir, "backup_20240101_daily.zip")); err != nil {
		t.Error("valid daily backup must be kept")
	}
	if _, err := os.Stat(filepath.Join(backupDir, "backup_20240108_weekly.zip")); err != nil {
		t.Error("valid weekly backup must be kept")
	}
}

func TestCleanup_EnforcesCountRetention(t *testing.T) {
	backupDir := t.TempDir()

	now := time.Now().UTC()
	// Seed 4 daily backups (all within the date window); cleanup must keep
	// the 3 newest and delete the oldest.
	for _, daysAgo := range []int{3, 2, 1, 0} {
		name := "backup_" + now.AddDate(0, 0, -daysAgo).Format("20060102") + "_daily.zip"
		writeFile(t, backupDir, name, "daily")
	}

	// Seed 2 weekly backups; both must survive.
	for _, weeksAgo := range []int{1, 0} {
		name := "backup_" + now.AddDate(0, 0, -weeksAgo*7).Format("20060102") + "_weekly.zip"
		writeFile(t, backupDir, name, "weekly")
	}

	if err := backup.Cleanup(context.Background(), backupDir); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	var dailyCount, weeklyCount int
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "_daily") {
			dailyCount++
		}

		if strings.Contains(entry.Name(), "_weekly") {
			weeklyCount++
		}
	}

	if dailyCount != 3 {
		t.Errorf("expected 3 daily backups after cleanup, got %d", dailyCount)
	}

	if weeklyCount != 2 {
		t.Errorf("expected 2 weekly backups after cleanup, got %d", weeklyCount)
	}
}

func TestCleanup_NonexistentDir_Succeeds(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")
	if err := backup.Cleanup(context.Background(), nonexistent); err != nil {
		t.Errorf("Cleanup on nonexistent dir: %v", err)
	}
}
