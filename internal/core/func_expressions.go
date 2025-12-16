// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"fmt"
	"strings"

	"github.com/coregx/relica/internal/dialects"
)

// =============================================================================
// CASE Expression
// =============================================================================

// CaseExp represents a SQL CASE expression.
// Supports both simple CASE (with column) and searched CASE (conditions only).
type CaseExp struct {
	column    string       // For simple CASE: CASE column WHEN ...
	whens     []whenClause // WHEN conditions
	elseValue interface{}  // ELSE value (optional)
	alias     string       // AS alias
}

// whenClause represents a single WHEN clause in a CASE expression.
type whenClause struct {
	condition interface{} // For simple CASE: value to match; for searched: condition string
	result    interface{} // THEN result
}

// Case creates a simple CASE expression.
//
// Example:
//
//	relica.Case("status").
//	    When("active", 1).
//	    When("inactive", 0).
//	    Else(-1).
//	    As("status_code")
//
// Generates: CASE "status" WHEN 'active' THEN 1 WHEN 'inactive' THEN 0 ELSE -1 END AS "status_code"
func Case(column string) *CaseExp {
	return &CaseExp{column: column}
}

// CaseWhen creates a searched CASE expression (without column).
//
// Example:
//
//	relica.CaseWhen().
//	    When("age < 18", "minor").
//	    When("age >= 18 AND age < 65", "adult").
//	    Else("senior").
//	    As("age_group")
//
// Generates: CASE WHEN age < 18 THEN 'minor' WHEN age >= 18 AND age < 65 THEN 'adult' ELSE 'senior' END
func CaseWhen() *CaseExp {
	return &CaseExp{}
}

// When adds a WHEN clause to the CASE expression.
func (c *CaseExp) When(condition, result interface{}) *CaseExp {
	c.whens = append(c.whens, whenClause{condition: condition, result: result})
	return c
}

// Else sets the ELSE value for the CASE expression.
func (c *CaseExp) Else(value interface{}) *CaseExp {
	c.elseValue = value
	return c
}

// As sets an alias for the CASE expression.
func (c *CaseExp) As(alias string) *CaseExp {
	c.alias = alias
	return c
}

// Build implements the Expression interface.
func (c *CaseExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(c.whens) == 0 {
		return "", nil
	}

	var sql strings.Builder
	// Pre-allocate for WHEN results + potential conditions + ELSE
	args := make([]interface{}, 0, len(c.whens)*2+1)

	// Start CASE
	if c.column != "" {
		// Simple CASE: CASE column
		sql.WriteString("CASE ")
		sql.WriteString(dialect.QuoteIdentifier(c.column))
	} else {
		// Searched CASE: CASE
		sql.WriteString("CASE")
	}

	// WHEN clauses
	for _, when := range c.whens {
		sql.WriteString(" WHEN ")

		if c.column != "" {
			// Simple CASE: WHEN value
			sql.WriteString("?")
			args = append(args, when.condition)
		} else {
			// Searched CASE: WHEN condition (raw SQL)
			sql.WriteString(fmt.Sprint(when.condition))
		}

		sql.WriteString(" THEN ?")
		args = append(args, when.result)
	}

	// ELSE clause
	if c.elseValue != nil {
		sql.WriteString(" ELSE ?")
		args = append(args, c.elseValue)
	}

	sql.WriteString(" END")

	// Alias
	if c.alias != "" {
		sql.WriteString(" AS ")
		sql.WriteString(dialect.QuoteIdentifier(c.alias))
	}

	return sql.String(), args
}

// =============================================================================
// COALESCE Expression
// =============================================================================

// CoalesceExp represents a SQL COALESCE expression.
// Returns the first non-NULL value from the list.
type CoalesceExp struct {
	values []interface{}
	alias  string
}

// Coalesce creates a COALESCE expression.
//
// Example:
//
//	relica.Coalesce("nickname", "first_name", "'Anonymous'").As("display_name")
//
// Generates: COALESCE("nickname", "first_name", 'Anonymous') AS "display_name"
func Coalesce(values ...interface{}) *CoalesceExp {
	return &CoalesceExp{values: values}
}

// As sets an alias for the COALESCE expression.
func (c *CoalesceExp) As(alias string) *CoalesceExp {
	c.alias = alias
	return c
}

// Build implements the Expression interface.
func (c *CoalesceExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(c.values) == 0 {
		return "", nil
	}

	var parts []string
	var args []interface{}

	for _, val := range c.values {
		switch v := val.(type) {
		case string:
			// Check if it's a column name (no quotes) or literal (with quotes)
			if strings.HasPrefix(v, "'") || strings.HasPrefix(v, "\"") {
				// Literal value - use as-is
				parts = append(parts, v)
			} else {
				// Column name - quote it
				parts = append(parts, dialect.QuoteIdentifier(v))
			}
		case Expression:
			// Nested expression
			sql, subArgs := v.Build(dialect)
			parts = append(parts, sql)
			args = append(args, subArgs...)
		default:
			// Value - use placeholder
			parts = append(parts, "?")
			args = append(args, v)
		}
	}

	sql := "COALESCE(" + strings.Join(parts, ", ") + ")"

	if c.alias != "" {
		sql += " AS " + dialect.QuoteIdentifier(c.alias)
	}

	return sql, args
}

// =============================================================================
// NULLIF Expression
// =============================================================================

// NullIfExp represents a SQL NULLIF expression.
// Returns NULL if the two expressions are equal, otherwise returns the first expression.
type NullIfExp struct {
	expr1 interface{}
	expr2 interface{}
	alias string
}

// NullIf creates a NULLIF expression.
//
// Example:
//
//	relica.NullIf("email", "''").As("valid_email")
//
// Generates: NULLIF("email", ”) AS "valid_email"
func NullIf(expr1, expr2 interface{}) *NullIfExp {
	return &NullIfExp{expr1: expr1, expr2: expr2}
}

// As sets an alias for the NULLIF expression.
func (n *NullIfExp) As(alias string) *NullIfExp {
	n.alias = alias
	return n
}

// buildExprValue builds a single value for use in SQL functions.
func buildExprValue(val interface{}, dialect dialects.Dialect) (string, []interface{}) {
	switch v := val.(type) {
	case string:
		// Check if it's a column name (no quotes) or literal (with quotes)
		if strings.HasPrefix(v, "'") || strings.HasPrefix(v, "\"") {
			// Literal value - use as-is
			return v, nil
		}
		// Column name - quote it
		return dialect.QuoteIdentifier(v), nil
	case Expression:
		return v.Build(dialect)
	default:
		return "?", []interface{}{v}
	}
}

// Build implements the Expression interface.
func (n *NullIfExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	var args []interface{}

	sql1, args1 := buildExprValue(n.expr1, dialect)
	args = append(args, args1...)

	sql2, args2 := buildExprValue(n.expr2, dialect)
	args = append(args, args2...)

	sql := "NULLIF(" + sql1 + ", " + sql2 + ")"

	if n.alias != "" {
		sql += " AS " + dialect.QuoteIdentifier(n.alias)
	}

	return sql, args
}

// =============================================================================
// GREATEST / LEAST Expressions
// =============================================================================

// GreatestLeastExp represents a SQL GREATEST or LEAST expression.
type GreatestLeastExp struct {
	values  []interface{}
	funcSQL string // "GREATEST" or "LEAST"
	alias   string
}

// Greatest creates a GREATEST expression.
// Returns the largest value from the list.
//
// Example:
//
//	relica.Greatest("price", "discount_price", "sale_price").As("max_price")
//
// Generates: GREATEST("price", "discount_price", "sale_price") AS "max_price"
//
// Note: SQLite does not have GREATEST/LEAST - use MAX/MIN in subquery instead.
func Greatest(values ...interface{}) *GreatestLeastExp {
	return &GreatestLeastExp{values: values, funcSQL: "GREATEST"}
}

// Least creates a LEAST expression.
// Returns the smallest value from the list.
//
// Example:
//
//	relica.Least("price", "discount_price", "sale_price").As("min_price")
//
// Generates: LEAST("price", "discount_price", "sale_price") AS "min_price"
func Least(values ...interface{}) *GreatestLeastExp {
	return &GreatestLeastExp{values: values, funcSQL: "LEAST"}
}

// As sets an alias for the expression.
func (g *GreatestLeastExp) As(alias string) *GreatestLeastExp {
	g.alias = alias
	return g
}

// Build implements the Expression interface.
func (g *GreatestLeastExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(g.values) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(g.values))
	args := make([]interface{}, 0, len(g.values))

	for _, val := range g.values {
		sql, subArgs := buildExprValue(val, dialect)
		parts = append(parts, sql)
		args = append(args, subArgs...)
	}

	// SQLite doesn't have GREATEST/LEAST - use MAX/MIN workaround
	var sql string
	switch dialect.(type) {
	case *dialects.SQLiteDialect:
		if g.funcSQL == "GREATEST" {
			sql = "MAX(" + strings.Join(parts, ", ") + ")"
		} else {
			sql = "MIN(" + strings.Join(parts, ", ") + ")"
		}
	default:
		sql = g.funcSQL + "(" + strings.Join(parts, ", ") + ")"
	}

	if g.alias != "" {
		sql += " AS " + dialect.QuoteIdentifier(g.alias)
	}

	return sql, args
}

// =============================================================================
// CONCAT Expression
// =============================================================================

// ConcatExp represents a SQL string concatenation.
// Uses database-specific syntax:
//   - PostgreSQL/SQLite: value1 || value2 || value3
//   - MySQL: CONCAT(value1, value2, value3)
type ConcatExp struct {
	values []interface{}
	alias  string
}

// Concat creates a string concatenation expression.
//
// Example:
//
//	relica.Concat("first_name", "' '", "last_name").As("full_name")
//
// PostgreSQL/SQLite: "first_name" || ' ' || "last_name" AS "full_name"
// MySQL: CONCAT("first_name", ' ', "last_name") AS "full_name"
func Concat(values ...interface{}) *ConcatExp {
	return &ConcatExp{values: values}
}

// As sets an alias for the CONCAT expression.
func (c *ConcatExp) As(alias string) *ConcatExp {
	c.alias = alias
	return c
}

// Build implements the Expression interface.
func (c *ConcatExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(c.values) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(c.values))
	args := make([]interface{}, 0, len(c.values))

	for _, val := range c.values {
		sql, subArgs := buildExprValue(val, dialect)
		parts = append(parts, sql)
		args = append(args, subArgs...)
	}

	// MySQL uses CONCAT(), PostgreSQL/SQLite use || operator
	var sql string
	switch dialect.(type) {
	case *dialects.MySQLDialect:
		sql = "CONCAT(" + strings.Join(parts, ", ") + ")"
	default:
		sql = strings.Join(parts, " || ")
	}

	if c.alias != "" {
		sql += " AS " + dialect.QuoteIdentifier(c.alias)
	}

	return sql, args
}
