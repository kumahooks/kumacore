// Package migrate validates and applies centralized SQL migrations.
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"kumacore/core/db/dialect"
	"kumacore/core/db/driver"
)

const trackingTableName = "schema_migrations"

type connection interface {
	driver.DBTX
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// Source describes the centralized migration directory for one backend.
type Source struct {
	Backend    string
	FileSystem fs.FS
	Directory  string
}

type migrationFile struct {
	sequence int
	filename string
	checksum string
	sql      string
}

type appliedMigration struct {
	sequence       int
	filename       string
	checksumSHA256 string
	appliedAtUnix  int64
}

// Plan describes validated pending migrations ready to apply.
type Plan struct {
	pendingMigrations []migrationFile
}

// PendingCount returns the number of migrations that will be applied.
func (plan Plan) PendingCount() int {
	return len(plan.pendingMigrations)
}

// Validate checks the centralized migration source and returns a plan without executing SQL.
func Validate(
	ctx context.Context,
	databaseConnection driver.DBTX,
	databaseDialect dialect.Dialect,
	migrationSource Source,
) (Plan, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if databaseDialect == nil {
		return Plan{}, fmt.Errorf("[database:migrate:Validate] nil dialect")
	}

	if strings.TrimSpace(migrationSource.Backend) == "" {
		return Plan{}, fmt.Errorf("[database:migrate:Validate] empty migration backend")
	}

	if migrationSource.Backend != databaseDialect.Name() {
		return Plan{}, fmt.Errorf(
			"[database:migrate:Validate] backend mismatch: got %q, want %q",
			migrationSource.Backend,
			databaseDialect.Name(),
		)
	}

	migrationFiles, err := readMigrationFiles(migrationSource)
	if err != nil {
		return Plan{}, err
	}

	appliedMigrations, err := loadAppliedMigrations(ctx, databaseConnection)
	if err != nil {
		return Plan{}, err
	}

	pendingMigrations, err := validateMigrations(migrationFiles, appliedMigrations)
	if err != nil {
		return Plan{}, err
	}

	return Plan{pendingMigrations: pendingMigrations}, nil
}

// Apply validates the migration source and applies pending migrations in order.
func Apply(
	ctx context.Context,
	databaseConnection connection,
	databaseDialect dialect.Dialect,
	migrationSource Source,
) error {
	plan, err := Validate(ctx, databaseConnection, databaseDialect, migrationSource)
	if err != nil {
		return err
	}

	return ApplyPlan(ctx, databaseConnection, plan)
}

// ApplyPlan applies a validated migration plan in order.
func ApplyPlan(ctx context.Context, databaseConnection connection, plan Plan) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if len(plan.pendingMigrations) == 0 {
		log.Printf("[database:migrate:ApplyPlan] migrations up to date")
		return nil
	}

	transaction, err := databaseConnection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("[database:migrate:ApplyPlan] migrate: begin plan: %w", err)
	}

	if err := ensureTrackingTable(ctx, transaction); err != nil {
		_ = transaction.Rollback()
		return err
	}

	for _, migrationFile := range plan.pendingMigrations {
		if err := applyMigration(ctx, transaction, migrationFile); err != nil {
			_ = transaction.Rollback()
			return err
		}
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("[database:migrate:ApplyPlan] commit plan: %w", err)
	}

	for _, migrationFile := range plan.pendingMigrations {
		log.Printf(
			"[database:migrate:ApplyPlan] applied migration %04d (%s)",
			migrationFile.sequence,
			migrationFile.filename,
		)
	}

	log.Printf("[database:migrate:ApplyPlan] %d migration(s) applied", len(plan.pendingMigrations))

	return nil
}

func readMigrationFiles(migrationSource Source) ([]migrationFile, error) {
	if migrationSource.FileSystem == nil {
		return nil, fmt.Errorf("[database:migrate:readMigrationFiles] nil migration filesystem")
	}

	directory := strings.TrimSpace(migrationSource.Directory)
	if directory == "" {
		return nil, fmt.Errorf("[database:migrate:readMigrationFiles] empty migration directory")
	}
	if !fs.ValidPath(directory) {
		return nil, fmt.Errorf("[database:migrate:readMigrationFiles] invalid migration directory %q", directory)
	}

	dirEntries, err := fs.ReadDir(migrationSource.FileSystem, directory)
	if err != nil {
		return nil, fmt.Errorf("[database:migrate:readMigrationFiles] read %s: %w", directory, err)
	}

	migrationFiles := make([]migrationFile, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			return nil, fmt.Errorf(
				"[database:migrate:readMigrationFiles] %s contains subdirectory %q",
				directory,
				dirEntry.Name(),
			)
		}

		sequence, err := parseMigrationFilename(dirEntry.Name())
		if err != nil {
			return nil, fmt.Errorf("[database:migrate:readMigrationFiles] %s: %w", dirEntry.Name(), err)
		}

		fileContents, err := fs.ReadFile(migrationSource.FileSystem, path.Join(directory, dirEntry.Name()))
		if err != nil {
			return nil, fmt.Errorf(
				"[database:migrate:readMigrationFiles] read %s/%s: %w",
				directory,
				dirEntry.Name(),
				err,
			)
		}

		fileChecksum := sha256.Sum256(fileContents)

		migrationFiles = append(migrationFiles, migrationFile{
			sequence: sequence,
			filename: dirEntry.Name(),
			checksum: hex.EncodeToString(fileChecksum[:]),
			sql:      string(fileContents),
		})
	}

	sort.Slice(migrationFiles, func(leftIndex, rightIndex int) bool {
		if migrationFiles[leftIndex].sequence != migrationFiles[rightIndex].sequence {
			return migrationFiles[leftIndex].sequence < migrationFiles[rightIndex].sequence
		}

		return migrationFiles[leftIndex].filename < migrationFiles[rightIndex].filename
	})

	for index, migrationFile := range migrationFiles {
		if index > 0 && migrationFiles[index-1].sequence == migrationFile.sequence {
			return nil, fmt.Errorf(
				"[database:migrate:readMigrationFiles] duplicate migration sequence %04d",
				migrationFile.sequence,
			)
		}

		expectedSequence := index + 1
		if migrationFile.sequence != expectedSequence {
			return nil, fmt.Errorf("[database:migrate:readMigrationFiles] sequence hole at %04d", expectedSequence)
		}
	}

	return migrationFiles, nil
}

func parseMigrationFilename(filename string) (int, error) {
	if path.Base(filename) != filename {
		return 0, fmt.Errorf("[database:migrate:parseMigrationFilename] invalid filename %q", filename)
	}

	if !strings.HasSuffix(filename, ".sql") {
		return 0, fmt.Errorf("[database:migrate:parseMigrationFilename] invalid filename %q", filename)
	}

	nameWithoutSuffix := strings.TrimSuffix(filename, ".sql")
	prefix, slug, found := strings.Cut(nameWithoutSuffix, "_")
	if !found || slug == "" {
		return 0, fmt.Errorf("[database:migrate:parseMigrationFilename] invalid filename %q", filename)
	}
	if len(prefix) != 4 {
		return 0, fmt.Errorf("[database:migrate:parseMigrationFilename] invalid filename %q", filename)
	}

	sequence, err := strconv.Atoi(prefix)
	if err != nil || sequence < 1 {
		return 0, fmt.Errorf("[database:migrate:parseMigrationFilename] invalid filename %q", filename)
	}

	return sequence, nil
}

func loadAppliedMigrations(ctx context.Context, databaseConnection driver.DBTX) (map[int]appliedMigration, error) {
	tableExists, err := trackingTableExists(ctx, databaseConnection)
	if err != nil {
		return nil, err
	}
	if !tableExists {
		return map[int]appliedMigration{}, nil
	}

	rows, err := databaseConnection.QueryContext(
		ctx,
		fmt.Sprintf(
			"SELECT sequence, filename, checksum_sha256, applied_at_unix FROM %s ORDER BY sequence",
			trackingTableName,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("[database:migrate:loadAppliedMigrations] query %s: %w", trackingTableName, err)
	}
	defer rows.Close()

	appliedMigrations := make(map[int]appliedMigration)
	for rows.Next() {
		var appliedMigrationRecord appliedMigration
		if err := rows.Scan(
			&appliedMigrationRecord.sequence,
			&appliedMigrationRecord.filename,
			&appliedMigrationRecord.checksumSHA256,
			&appliedMigrationRecord.appliedAtUnix,
		); err != nil {
			return nil, fmt.Errorf("[database:migrate:loadAppliedMigrations] scan %s: %w", trackingTableName, err)
		}

		appliedMigrations[appliedMigrationRecord.sequence] = appliedMigrationRecord
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("[database:migrate:loadAppliedMigrations] miterate %s: %w", trackingTableName, err)
	}

	return appliedMigrations, nil
}

func trackingTableExists(ctx context.Context, databaseConnection driver.DBTX) (bool, error) {
	var tableName string
	err := databaseConnection.QueryRowContext(
		ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
		trackingTableName,
	).Scan(&tableName)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("[database:migrate:trackingTableExists] check %s: %w", trackingTableName, err)
	}

	return true, nil
}

func validateMigrations(
	migrationFiles []migrationFile,
	appliedMigrations map[int]appliedMigration,
) ([]migrationFile, error) {
	if err := validateAppliedMigrations(appliedMigrations); err != nil {
		return nil, err
	}

	pendingMigrations := make([]migrationFile, 0)
	plannedMigrationBySequence := make(map[int]migrationFile)

	for _, plannedMigration := range migrationFiles {
		if existingAppliedMigration, exists := appliedMigrations[plannedMigration.sequence]; exists {
			if existingAppliedMigration.filename != plannedMigration.filename {
				return nil, fmt.Errorf(
					"[database:migrate:trackingTableExists] migration %04d filename mismatch: got %q, want %q",
					plannedMigration.sequence,
					existingAppliedMigration.filename,
					plannedMigration.filename,
				)
			}

			if existingAppliedMigration.checksumSHA256 != plannedMigration.checksum {
				return nil, fmt.Errorf(
					"[database:migrate:trackingTableExists] migration %04d checksum mismatch",
					plannedMigration.sequence,
				)
			}

			plannedMigrationBySequence[plannedMigration.sequence] = plannedMigration
			continue
		}

		pendingMigrations = append(pendingMigrations, plannedMigration)
		plannedMigrationBySequence[plannedMigration.sequence] = plannedMigration
	}

	for sequence, appliedMigrationRecord := range appliedMigrations {
		if _, exists := plannedMigrationBySequence[sequence]; exists {
			continue
		}

		return nil, fmt.Errorf(
			"[database:migrate:trackingTableExists] applied migration missing from filesystem: %04d (%s)",
			appliedMigrationRecord.sequence,
			appliedMigrationRecord.filename,
		)
	}

	return pendingMigrations, nil
}

func validateAppliedMigrations(appliedMigrations map[int]appliedMigration) error {
	appliedMigrationRecords := make([]appliedMigration, 0, len(appliedMigrations))
	for _, appliedMigrationRecord := range appliedMigrations {
		appliedMigrationRecords = append(appliedMigrationRecords, appliedMigrationRecord)
	}

	sort.Slice(appliedMigrationRecords, func(leftIndex, rightIndex int) bool {
		if appliedMigrationRecords[leftIndex].sequence != appliedMigrationRecords[rightIndex].sequence {
			return appliedMigrationRecords[leftIndex].sequence < appliedMigrationRecords[rightIndex].sequence
		}

		return appliedMigrationRecords[leftIndex].filename < appliedMigrationRecords[rightIndex].filename
	})

	for index, appliedMigrationRecord := range appliedMigrationRecords {
		expectedSequence := index + 1
		if appliedMigrationRecord.sequence != expectedSequence {
			return fmt.Errorf(
				"[database:migrate:validateAppliedMigrations] applied sequence hole at %04d",
				expectedSequence,
			)
		}
	}

	return nil
}

func ensureTrackingTable(ctx context.Context, databaseConnection driver.DBTX) error {
	_, err := databaseConnection.ExecContext(
		ctx,
		fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				sequence INTEGER NOT NULL PRIMARY KEY,
				filename TEXT NOT NULL,
				checksum_sha256 TEXT NOT NULL,
				applied_at_unix INTEGER NOT NULL
			)
		`, trackingTableName),
	)
	if err != nil {
		return fmt.Errorf("[database:migrate:ensureTrackingTable] migrate: create %s: %w", trackingTableName, err)
	}

	return nil
}

func applyMigration(ctx context.Context, databaseConnection driver.DBTX, migrationFile migrationFile) error {
	if _, err := databaseConnection.ExecContext(ctx, migrationFile.sql); err != nil {
		return fmt.Errorf("[database:migrate:applyMigration] run %04d: %w", migrationFile.sequence, err)
	}

	if _, err := databaseConnection.ExecContext(
		ctx,
		fmt.Sprintf(
			"INSERT INTO %s (sequence, filename, checksum_sha256, applied_at_unix) VALUES (?, ?, ?, ?)",
			trackingTableName,
		),
		migrationFile.sequence,
		migrationFile.filename,
		migrationFile.checksum,
		time.Now().Unix(),
	); err != nil {
		return fmt.Errorf("[database:migrate:applyMigration] record %04d: %w", migrationFile.sequence, err)
	}

	return nil
}
