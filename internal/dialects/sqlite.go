package dialects

import "strings"

// SQLiteDialect implements SQLite-specific SQL dialect.
type SQLiteDialect struct{}

func init() {
	RegisterDialect("sqlite", &SQLiteDialect{})
	RegisterDialect("sqlite3", &SQLiteDialect{})
}

// QuoteIdentifier quotes a SQLite identifier using backticks, as ozzo-dbx
// does. Not double quotes: SQLite reads a double-quoted name that matches no
// column as a string literal (its "double-quoted string literal" fallback),
// so a misspelled column in a SELECT list, WHERE or ORDER BY silently became
// a constant ("x" = 'x' is true for every row). A backticked name is
// always an identifier, and a typo fails with "no such column".
func (d *SQLiteDialect) QuoteIdentifier(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// Placeholder returns SQLite placeholder format (always "?").
func (d *SQLiteDialect) Placeholder(_ int) string {
	return "?"
}

// UpsertSQL generates SQLite UPSERT syntax using ON CONFLICT.
func (d *SQLiteDialect) UpsertSQL(u Upsert) string {
	return onConflictSQL(u, "excluded")
}
