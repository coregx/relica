package core

import (
	"reflect"
	"strings"
)

// DefaultFieldMapFunc converts Go struct field names to snake_case database column names.
func DefaultFieldMapFunc(field string) string {
	result := make([]rune, 0, len(field)+5)
	for i, r := range field {
		if i > 0 && 'A' <= r && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// GetTableName extracts the database table name from a model struct or interface.
func GetTableName(model interface{}) string {
	if tm, ok := model.(TableModel); ok {
		return tm.TableName()
	}

	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() == reflect.Slice {
		return GetTableName(reflect.Zero(t.Elem()).Interface())
	}

	return DefaultFieldMapFunc(t.Name())
}

// TableModel defines an interface for models that provide custom table names.
type TableModel interface {
	TableName() string
}

// QuoteIdentifier quotes a database identifier using double quotes.
func QuoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// QuoteTableName quotes a table name using the dialect's identifier quoting style.
// For PostgreSQL: double-quotes. For MySQL: backticks.
func (db *DB) QuoteTableName(s string) string {
	return db.dialect.QuoteIdentifier(s)
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
