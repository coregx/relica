package dialects

import (
	"fmt"
	"strings"
)

// MySQLDialect implements MySQL-specific SQL dialect.
type MySQLDialect struct{}

// QuoteIdentifier quotes a MySQL identifier using backticks.
func (d *MySQLDialect) QuoteIdentifier(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// Placeholder returns MySQL placeholder format (always "?").
func (d *MySQLDialect) Placeholder(_ int) string {
	return "?"
}

// UpsertSQL generates MySQL UPSERT syntax using ON DUPLICATE KEY UPDATE.
// The conflict target is not part of MySQL syntax: the statement fires on any
// unique key.
func (d *MySQLDialect) UpsertSQL(u Upsert) string {
	if u.DoNothing {
		// MySQL has no DO NOTHING. A self-assignment keeps the row; INSERT IGNORE
		// would also turn unrelated errors (bad values, FK failures) into warnings.
		return fmt.Sprintf(" ON DUPLICATE KEY UPDATE %s = %s", u.NoOp, u.NoOp)
	}

	updates := make([]string, len(u.Update))
	for i, col := range u.Update {
		if u.RowAlias {
			updates[i] = fmt.Sprintf("%s = excluded.%s", col, col)
		} else {
			updates[i] = fmt.Sprintf("%s = VALUES(%s)", col, col)
		}
	}

	alias := ""
	if u.RowAlias {
		alias = " AS excluded"
	}
	return alias + " ON DUPLICATE KEY UPDATE " + strings.Join(updates, ", ")
}

func init() {
	RegisterDialect("mysql", &MySQLDialect{})
}
