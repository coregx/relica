package core

import (
	"strings"

	"github.com/coregx/relica/internal/dialects"
)

// QuoteTableName quotes a table name using the dialect's identifier quoting style.
// A schema-qualified name is quoted per part: "app.users" → "app"."users"
// (PostgreSQL) / `app`.`users` (MySQL).
func (db *DB) QuoteTableName(s string) string {
	return quoteTableName(s, db.dialect)
}

// quoteTableName quotes every dot-separated part of a table name. Unlike
// quoteColumn it passes nothing through unquoted (no function calls, no "*"):
// a table name is always an identifier.
func quoteTableName(name string, dialect dialects.Dialect) string {
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = dialect.QuoteIdentifier(p)
	}
	return strings.Join(parts, ".")
}

// QuoteColumnName quotes a column name using the dialect's identifier quoting style.
// Handles dotted identifiers (e.g., "table.column") by quoting each part separately.
// For PostgreSQL: "table"."column". For MySQL: `table`.`column`.
func (db *DB) QuoteColumnName(s string) string {
	return quoteColumn(s, db.dialect)
}

// GenerateParamName generates a dialect-specific parameter placeholder for the given index.
// For PostgreSQL: "$1", "$2", etc. For MySQL/SQLite: "?".
func (db *DB) GenerateParamName(index int) string {
	return db.dialect.Placeholder(index)
}
