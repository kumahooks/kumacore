package sqlite

import "testing"

func TestDialect(t *testing.T) {
	databaseDialect := Dialect{}

	if databaseDialect.Name() != DriverName {
		t.Errorf("Name: got %q, want %q", databaseDialect.Name(), DriverName)
	}

	if databaseDialect.Placeholder(1) != "?" {
		t.Errorf("Placeholder: got %q, want %q", databaseDialect.Placeholder(1), "?")
	}

	if databaseDialect.CurrentUnixTimestampExpression() != "unixepoch()" {
		t.Errorf(
			"CurrentUnixTimestampExpression: got %q, want %q",
			databaseDialect.CurrentUnixTimestampExpression(),
			"unixepoch()",
		)
	}
}
