package dialects

import (
	"fmt"
	"strings"
)

// SQLiteDialect implements SQLite-specific SQL dialect.
type SQLiteDialect struct{}

func init() {
	RegisterDialect("sqlite", &SQLiteDialect{})
	RegisterDialect("sqlite3", &SQLiteDialect{})
}

// QuoteIdentifier quotes a SQLite identifier using double quotes.
func (d *SQLiteDialect) QuoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// Placeholder returns SQLite placeholder format (always "?").
func (d *SQLiteDialect) Placeholder(_ int) string {
	return "?"
}

// UpsertSQL generates SQLite UPSERT syntax using ON CONFLICT.
func (d *SQLiteDialect) UpsertSQL(_ string, conflictColumns, updateCols []string) string {
	if updateCols == nil {
		// DO NOTHING case
		if len(conflictColumns) > 0 {
			return fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", strings.Join(conflictColumns, ", "))
		}
		return " ON CONFLICT DO NOTHING"
	}

	// DO UPDATE case
	updates := make([]string, len(updateCols))
	for i, col := range updateCols {
		updates[i] = fmt.Sprintf("%s = excluded.%s", col, col)
	}

	return fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s",
		strings.Join(conflictColumns, ", "),
		strings.Join(updates, ", "))
}
