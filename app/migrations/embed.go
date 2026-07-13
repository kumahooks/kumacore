// Package migrations embeds SQL migration files for the application.
package migrations

import "embed"

//go:embed sqlite/app/*.sql sqlite/worker/*.sql
var Files embed.FS
