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
	RegisterDialect("pgx", &PostgresDialect{})
}

// QuoteIdentifier quotes a PostgreSQL identifier using double quotes.
func (d *PostgresDialect) QuoteIdentifier(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// Placeholder returns PostgreSQL placeholder format ($1, $2, etc.).
func (d *PostgresDialect) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

// UpsertSQL generates PostgreSQL UPSERT syntax using ON CONFLICT.
func (d *PostgresDialect) UpsertSQL(u Upsert) string {
	return onConflictSQL(u, "EXCLUDED")
}

// onConflictSQL renders the PostgreSQL/SQLite ON CONFLICT clause; excluded is
// the spelling of the proposed-row table.
func onConflictSQL(u Upsert, excluded string) string {
	target := ""
	if len(u.Target) > 0 {
		target = " (" + strings.Join(u.Target, ", ") + ")"
	}
	if u.DoNothing {
		return " ON CONFLICT" + target + " DO NOTHING"
	}
	parts := make([]string, len(u.Update))
	for i, col := range u.Update {
		parts[i] = fmt.Sprintf("%s = %s.%s", col, excluded, col)
	}
	return " ON CONFLICT" + target + " DO UPDATE SET " + strings.Join(parts, ", ")
}
