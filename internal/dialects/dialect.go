// Package dialects provides database-specific SQL dialect implementations for
// PostgreSQL, MySQL, and SQLite, handling identifier quoting, placeholders, and
// UPSERT operations.
package dialects

// Dialect defines database-specific behaviors.
type Dialect interface {
	QuoteIdentifier(string) string
	Placeholder(int) string
	UpsertSQL(string, []string, []string) string
}

var dialects = make(map[string]Dialect)

// RegisterDialect registers a database dialect by driver name.
func RegisterDialect(name string, d Dialect) {
	dialects[name] = d
}

// GetDialect retrieves a registered dialect by driver name, panics if not found.
func GetDialect(name string) Dialect {
	if d, ok := dialects[name]; ok {
		return d
	}
	panic("unsupported dialect: " + name)
}
