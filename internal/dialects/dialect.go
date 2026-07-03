// Package dialects provides database-specific SQL dialect implementations for
// PostgreSQL, MySQL, and SQLite, handling identifier quoting, placeholders, and
// UPSERT operations.
package dialects

import "fmt"

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

// GetDialect retrieves a registered dialect by driver name.
// Panics with an actionable message if the dialect is not registered.
// Supported built-in names: "postgres", "postgresql", "pgx", "mysql", "sqlite", "sqlite3".
// Custom dialects can be added via RegisterDialect.
func GetDialect(name string) Dialect {
	if d, ok := dialects[name]; ok {
		return d
	}
	panic(fmt.Sprintf(
		"relica: unsupported database dialect %q. Supported built-in dialects: "+
			"postgres, postgresql, pgx, mysql, sqlite, sqlite3. "+
			"Use dialects.RegisterDialect() to register a custom dialect.",
		name,
	))
}
