package dialects

import "strings"

// SQLiteDialect implements SQLite-specific SQL dialect.
type SQLiteDialect struct{}

func init() {
	RegisterDialect("sqlite", &SQLiteDialect{})
	RegisterDialect("sqlite3", &SQLiteDialect{})
}

// QuoteIdentifier quotes a SQLite identifier using double quotes.
func (d *SQLiteDialect) QuoteIdentifier(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// Placeholder returns SQLite placeholder format (always "?").
func (d *SQLiteDialect) Placeholder(_ int) string {
	return "?"
}

// UpsertSQL generates SQLite UPSERT syntax using ON CONFLICT.
func (d *SQLiteDialect) UpsertSQL(u Upsert) string {
	return onConflictSQL(u, "excluded")
}
