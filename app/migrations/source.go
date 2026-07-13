package migrations

import (
	"kumacore/core/db/migrate"
	"kumacore/core/db/sqlite"
)

func AppSource() migrate.Source {
	return migrate.Source{
		Backend:    sqlite.DriverName,
		FileSystem: Files,
		Directory:  "sqlite/app",
	}
}

func WorkerSource() migrate.Source {
	return migrate.Source{
		Backend:    sqlite.DriverName,
		FileSystem: Files,
		Directory:  "sqlite/worker",
	}
}
