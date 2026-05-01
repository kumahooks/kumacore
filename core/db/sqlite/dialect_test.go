package sqlite_test

import (
	"testing"

	"kumacore/core/db/sqlite"
)

func TestDialect(t *testing.T) {
	databaseDialect := sqlite.Dialect{}

	if databaseDialect.Name() != sqlite.DriverName {
		t.Errorf("Name: got %q, want %q", databaseDialect.Name(), sqlite.DriverName)
	}

	if databaseDialect.Placeholder(1) != "?" {
		t.Errorf("Placeholder: got %q, want %q", databaseDialect.Placeholder(1), "?")
	}

	if databaseDialect.CurrentUnixTimestampExpression() != "unixepoch()" {
		t.Errorf("CurrentUnixTimestampExpression: got %q, want %q", databaseDialect.CurrentUnixTimestampExpression(), "unixepoch()")
	}
}
