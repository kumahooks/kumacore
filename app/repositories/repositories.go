// Package repositories provides shared utilities for SQL repositories.
package repositories

import (
	"fmt"
	"io/fs"
)

// MustReadQuery reads an embedded SQL file at init time and panics on failure.
func MustReadQuery(fileSystem fs.FS, name string) string {
	queryBytes, err := fs.ReadFile(fileSystem, name)
	if err != nil {
		panic(fmt.Sprintf("[repositories:MustReadQuery] read %s: %v", name, err))
	}

	return string(queryBytes)
}
