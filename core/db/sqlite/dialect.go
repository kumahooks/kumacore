package sqlite

import "kumacore/core/db/dialect"

var _ dialect.Dialect = Dialect{}

// Dialect is the SQLite dialect implementation.
type Dialect struct{}

// Name returns the driver name.
func (Dialect) Name() string {
	return DriverName
}

// Placeholder returns SQLite's positional placeholder.
func (Dialect) Placeholder(_ int) string {
	return "?"
}

// CurrentUnixTimestampExpression returns SQLite's Unix timestamp expression.
func (Dialect) CurrentUnixTimestampExpression() string {
	return "unixepoch()"
}
