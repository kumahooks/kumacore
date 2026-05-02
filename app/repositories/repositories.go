// Package repositories contains shared repository helpers.
package repositories

import (
	"fmt"
	"io/fs"
)

// MustReadQuery reads a SQL query file from fileSystem and panics if it cannot be read.
func MustReadQuery(fileSystem fs.FS, name string) string {
	queryBytes, err := fs.ReadFile(fileSystem, name)
	if err != nil {
		panic(fmt.Sprintf("[repositories:MustReadQuery] read %s: %v", name, err))
	}

	return string(queryBytes)
}
