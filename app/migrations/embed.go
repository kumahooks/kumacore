// Package migrations embeds SQL migration files for the application.
package migrations

import "embed"

//go:embed all:sqlite/app all:sqlite/worker
var Files embed.FS
