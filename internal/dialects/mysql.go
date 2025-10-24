package dialects

import (
	"fmt"
	"strings"
)

// MySQLDialect implements MySQL-specific SQL dialect.
type MySQLDialect struct{}

// QuoteIdentifier quotes a MySQL identifier using backticks.
func (d *MySQLDialect) QuoteIdentifier(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// Placeholder returns MySQL placeholder format (always "?").
func (d *MySQLDialect) Placeholder(_ int) string {
	return "?"
}

// UpsertSQL generates MySQL UPSERT syntax using ON DUPLICATE KEY UPDATE.
func (d *MySQLDialect) UpsertSQL(_ string, _, updateCols []string) string {
	if updateCols == nil {
		// MySQL doesn't have DO NOTHING, but we can simulate it by updating to same value
		// However, INSERT IGNORE is better - but requires different SQL structure
		// For now, we'll return empty to make it a plain INSERT (will fail on duplicate)
		// Users should use DoUpdate() for MySQL
		return ""
	}

	updates := make([]string, len(updateCols))
	for i, col := range updateCols {
		updates[i] = fmt.Sprintf("%s = VALUES(%s)", col, col)
	}

	return fmt.Sprintf(" ON DUPLICATE KEY UPDATE %s",
		strings.Join(updates, ", "))
}

func init() {
	RegisterDialect("mysql", &MySQLDialect{})
}
