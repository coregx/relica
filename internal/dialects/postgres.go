package dialects

import (
	"fmt"
	"strings"
)

// PostgresDialect implements PostgreSQL-specific SQL dialect.
type PostgresDialect struct{}

func init() {
	RegisterDialect("postgres", &PostgresDialect{})
	RegisterDialect("postgresql", &PostgresDialect{})
}

// QuoteIdentifier quotes a PostgreSQL identifier using double quotes.
func (d *PostgresDialect) QuoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// Placeholder returns PostgreSQL placeholder format ($1, $2, etc.).
func (d *PostgresDialect) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

// UpsertSQL generates PostgreSQL UPSERT syntax using ON CONFLICT.
func (d *PostgresDialect) UpsertSQL(_ string, conflictColumns, updateCols []string) string {
	if updateCols == nil {
		// DO NOTHING case
		if len(conflictColumns) > 0 {
			return fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", strings.Join(conflictColumns, ", "))
		}
		return " ON CONFLICT DO NOTHING"
	}

	// DO UPDATE case
	return fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s",
		strings.Join(conflictColumns, ", "),
		buildUpdateSet(updateCols),
	)
}

// buildUpdateSet builds the SET clause for UPDATE.
func buildUpdateSet(cols []string) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = fmt.Sprintf("%s = EXCLUDED.%s", col, col)
	}
	return strings.Join(parts, ", ")
}
