// Package dialect defines SQL dialect metadata used by core and modules.
package dialect

// Dialect describes the small SQL differences core needs to know about.
type Dialect interface {
	Name() string
	Placeholder(position int) string
	CurrentUnixTimestampExpression() string
}
