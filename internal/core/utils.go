package core

import (
	"reflect"
	"strconv"
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
	if t.Kind() == reflect.Ptr {
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

// QuoteTableName quotes a table name for safe SQL usage.
func (db *DB) QuoteTableName(s string) string {
	// Simplified implementation.
	return `"` + s + `"`
}

// QuoteColumnName quotes a column name for safe SQL usage.
func (db *DB) QuoteColumnName(s string) string {
	// Simplified implementation.
	return `"` + s + `"`
}

// GenerateParamName generates a unique parameter placeholder name.
func (db *DB) GenerateParamName() string {
	return "p" + strconv.Itoa(len(db.params)+1)
}
