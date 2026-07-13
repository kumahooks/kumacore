// Package db provides migrated database templates for tests.
//
// The template is migration-only: it applies app migrations to produce a bare
// schema with no seed data. Tests create the records they need through
// testfactory functions.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"kumacore/app/migrations"
	coredb "kumacore/core/db"
	"kumacore/core/db/migrate"
	"kumacore/core/db/sqlite"
)

// Template stores a migrated database copy path.
type Template struct {
	path string
}

// NewTemplate creates one migrated template database for a test package,
// runs testingMain, and cleans up the template directory.
func NewTemplate(testingMain *testing.M, template **Template) int {
	directory, err := os.MkdirTemp("", "kumacore-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db:NewTemplate] create test directory: %v\n", err)
		return 1
	}

	exitCode := run(testingMain, template, directory)

	if err := os.RemoveAll(directory); err != nil {
		fmt.Fprintf(os.Stderr, "[db:NewTemplate] remove test directory: %v\n", err)
		return 1
	}

	return exitCode
}

// Open returns an isolated copy of the template database.
func (template *Template) Open(testingHandle testing.TB) *sql.DB {
	testingHandle.Helper()

	databasePath := filepath.Join(testingHandle.TempDir(), "test.db")
	copyFile(testingHandle, template.path, databasePath)

	database, _, err := coredb.Open(sqlite.DriverName, databasePath)
	if err != nil {
		testingHandle.Fatalf("[db:Open] open test db: %v", err)
	}

	testingHandle.Cleanup(func() { database.Close() })

	return database
}

func run(testingMain *testing.M, template **Template, directory string) int {
	templatePath := filepath.Join(directory, "migrated.db")
	database, _, err := coredb.Open(sqlite.DriverName, templatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db:run] open template db: %v\n", err)
		return 1
	}

	if err := migrate.Apply(
		context.Background(),
		database,
		sqlite.Dialect{},
		migrations.AppSource(),
	); err != nil {
		_ = database.Close()
		fmt.Fprintf(os.Stderr, "[db:run] apply migrations: %v\n", err)
		return 1
	}

	if _, err := database.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = database.Close()
		fmt.Fprintf(os.Stderr, "[db:run] checkpoint template db: %v\n", err)
		return 1
	}

	if err := database.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "[db:run] close template db: %v\n", err)
		return 1
	}

	*template = &Template{path: templatePath}

	return testingMain.Run()
}

func copyFile(testingHandle testing.TB, sourcePath string, destinationPath string) {
	testingHandle.Helper()

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		testingHandle.Fatalf("[db:copyFile] open source file: %v", err)
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		testingHandle.Fatalf("[db:copyFile] create destination file: %v", err)
	}

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		_ = destinationFile.Close()
		testingHandle.Fatalf("[db:copyFile] copy file: %v", err)
	}

	if err := destinationFile.Close(); err != nil {
		testingHandle.Fatalf("[db:copyFile] close destination file: %v", err)
	}
}
