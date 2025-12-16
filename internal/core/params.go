// Package core provides the core database functionality including connection management,
// query building, statement caching, and result scanning for Relica.
package core

import (
	"fmt"
	"regexp"
	"strings"
)

// Params represents named parameter values for query binding.
// Named parameters are specified in SQL using {:name} syntax.
//
// Example:
//
//	db.NewQuery("SELECT * FROM users WHERE id={:id} AND status={:status}").
//	    Bind(relica.Params{"id": 1, "status": "active"}).
//	    All(&users)
type Params map[string]interface{}

var (
	// namedPlaceholderRegex matches named parameter placeholders {:name}.
	namedPlaceholderRegex = regexp.MustCompile(`\{:(\w+)\}`)

	// quoteRegex matches table and column quoting syntax.
	// {{table_name}} - quotes table name (double curly braces)
	// [[column_name]] - quotes column name (double square brackets)
	// Pattern matches word characters, hyphens, dots, and spaces to support schema.table format.
	quoteRegex = regexp.MustCompile(`(\{\{[\w\-. ]+\}\}|\[\[[\w\-. ]+\]\])`)
)

// processSQL replaces named parameter placeholders {:name} with dialect-specific
// positional placeholders ($1, $2 for PostgreSQL; ?, ? for MySQL/SQLite).
// It also quotes table names {{table}} and column names [[column]] using the
// dialect-specific quoting.
//
// The function returns:
//  1. The SQL string with placeholders and quoted identifiers replaced
//  2. The list of parameter names in order of appearance
//
// Example:
//
//	sql := "SELECT [[name]] FROM {{users}} WHERE [[id]]={:id} AND [[status]]={:status}"
//	newSQL, params := processSQL(sql, dialect)
//	// PostgreSQL: "SELECT "name" FROM "users" WHERE "id"=$1 AND "status"=$2", ["id", "status"]
//	// MySQL:      "SELECT `name` FROM `users` WHERE `id`=? AND `status`=?", ["id", "status"]
//
// If the same parameter name appears multiple times, it will be in the list multiple times.
// For schema-prefixed identifiers like {{schema.table}}, each part is quoted separately.
func (db *DB) processSQL(sql string) (string, []string) {
	var paramNames []string
	count := 0

	// Step 1: Replace named placeholders {:name} with positional placeholders
	result := namedPlaceholderRegex.ReplaceAllStringFunc(sql, func(match string) string {
		count++
		// Extract parameter name from {:name} by removing {: and }
		paramName := match[2 : len(match)-1]
		paramNames = append(paramNames, paramName)
		return db.dialect.Placeholder(count)
	})

	// Step 2: Quote table names {{name}} and column names [[name]]
	result = quoteRegex.ReplaceAllStringFunc(result, func(match string) string {
		// Extract identifier name by removing {{ }} or [[ ]]
		identifier := match[2 : len(match)-2]
		return db.quoteIdentifier(identifier)
	})

	return result, paramNames
}

// quoteIdentifier quotes an identifier using the dialect-specific quoting.
// For schema-prefixed identifiers like "schema.table", each part is quoted separately.
//
// Example:
//
//	PostgreSQL: "users" → "users", "public.users" → "public"."users"
//	MySQL: `users` → `users`, `mydb.users` → `mydb`.`users`
func (db *DB) quoteIdentifier(identifier string) string {
	// Handle schema-prefixed identifiers (e.g., "schema.table")
	if strings.Contains(identifier, ".") {
		parts := strings.Split(identifier, ".")
		quoted := make([]string, len(parts))
		for i, part := range parts {
			// Trim spaces from each part before quoting
			quoted[i] = db.dialect.QuoteIdentifier(strings.TrimSpace(part))
		}
		return strings.Join(quoted, ".")
	}

	// Simple identifier - quote as-is
	return db.dialect.QuoteIdentifier(strings.TrimSpace(identifier))
}

// bindParams converts named parameters to positional values based on the parameter order.
// Returns an error if any required parameter is missing from the params map.
//
// Example:
//
//	paramNames := []string{"id", "status", "id"}
//	params := Params{"id": 1, "status": "active"}
//	values, err := bindParams(params, paramNames)
//	// Returns: []interface{}{1, "active", 1}, nil
func bindParams(params Params, paramNames []string) ([]interface{}, error) {
	values := make([]interface{}, len(paramNames))

	for i, name := range paramNames {
		value, ok := params[name]
		if !ok {
			return nil, fmt.Errorf("missing parameter: %s", name)
		}
		values[i] = value
	}

	return values, nil
}
