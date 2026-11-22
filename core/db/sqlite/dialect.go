package sqlite

import "kumacore/core/db/dialect"

var _ dialect.Dialect = Dialect{}

type Dialect struct{}

func (Dialect) Name() string {
	return DriverName
}

func (Dialect) Placeholder(_ int) string {
	return "?"
}

func (Dialect) CurrentUnixTimestampExpression() string {
	return "unixepoch()"
}
