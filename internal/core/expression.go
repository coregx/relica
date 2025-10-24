// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coregx/relica/internal/dialects"
)

// Expression represents a database expression that can be embedded in a SQL statement.
// Expressions are used to build complex WHERE clauses in a type-safe, fluent manner.
//
// Example:
//
//	db.Builder().Select().From("users").
//	    Where(relica.And(
//	        relica.HashExp{"status": 1},
//	        relica.GreaterThan("age", 18),
//	    )).
//	    All(&users)
type Expression interface {
	// Build converts the expression into a SQL fragment and returns parameter values.
	// The dialect parameter is used for proper identifier quoting.
	// Returns SQL string with "?" placeholders and a slice of parameter values.
	// Placeholder renumbering to dialect-specific format happens at query build time.
	Build(dialect dialects.Dialect) (sql string, args []interface{})
}

// RawExp represents a raw SQL expression with optional parameter bindings.
// Use this when you need to embed custom SQL that isn't covered by other expression types.
//
// Example:
//
//	relica.NewExp("age > ? AND status = ?", 18, "active")
type RawExp struct {
	SQL  string
	Args []interface{}
}

// NewExp creates a new raw SQL expression with optional parameter bindings.
// The SQL string can contain ? placeholders which will be replaced with dialect-specific
// placeholders during query building.
func NewExp(sql string, args ...interface{}) Expression {
	return &RawExp{
		SQL:  sql,
		Args: args,
	}
}

// Build converts the raw expression into a SQL fragment.
// The SQL string is returned as-is, with args passed through unchanged.
// Placeholder conversion (? → $1, $2, etc.) happens at the query builder level.
func (e *RawExp) Build(_ dialects.Dialect) (string, []interface{}) {
	return e.SQL, e.Args
}

// HashExp represents a hash-based expression using a map of column-value pairs.
// It provides convenient syntax for common WHERE conditions with automatic handling
// of special cases.
//
// Special value handling:
//   - nil value → "column IS NULL"
//   - []interface{} → "column IN (...)"
//   - Expression → recursively builds nested expression
//
// Example:
//
//	relica.HashExp{
//	    "status": 1,                    // status = 1
//	    "age": []int{18, 19, 20},       // age IN (18, 19, 20)
//	    "deleted_at": nil,              // deleted_at IS NULL
//	}
//
// Generates: status = 1 AND age IN (18, 19, 20) AND deleted_at IS NULL
type HashExp map[string]interface{}

// buildHashExpValue processes a single key-value pair from HashExp.
func buildHashExpValue(key string, value interface{}, dialect dialects.Dialect) (sql string, args []interface{}) {
	col := dialect.QuoteIdentifier(key)

	switch v := value.(type) {
	case nil:
		return col + " IS NULL", nil

	case Expression:
		sql, args = v.Build(dialect)
		if sql != "" {
			return "(" + sql + ")", args
		}
		return "", nil

	case []interface{}:
		if len(v) == 0 {
			return "0=1", nil
		}
		in := In(key, v...)
		return in.Build(dialect)

	default:
		return col + "=?", []interface{}{value}
	}
}

// Build converts a HashExp into a SQL fragment.
// Map keys are sorted to ensure deterministic SQL generation.
// Multiple conditions are combined with AND.
func (e HashExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(e) == 0 {
		return "", nil
	}

	// Sort keys for deterministic SQL generation
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	var args []interface{}

	for _, key := range keys {
		sql, subArgs := buildHashExpValue(key, e[key], dialect)
		if sql != "" {
			parts = append(parts, sql)
			args = append(args, subArgs...)
		}
	}

	if len(parts) == 0 {
		return "", nil
	}

	if len(parts) == 1 {
		return parts[0], args
	}

	return strings.Join(parts, " AND "), args
}

// CompareExp represents a comparison expression (=, <>, >, <, >=, <=).
type CompareExp struct {
	Col      string
	Operator string
	Value    interface{}
}

// Eq generates an equality expression (column = value).
// If value is nil, generates "column IS NULL" instead.
func Eq(col string, value interface{}) Expression {
	return &CompareExp{Col: col, Operator: "=", Value: value}
}

// NotEq generates an inequality expression (column <> value).
// If value is nil, generates "column IS NOT NULL" instead.
func NotEq(col string, value interface{}) Expression {
	return &CompareExp{Col: col, Operator: "<>", Value: value}
}

// GreaterThan generates a greater-than expression (column > value).
func GreaterThan(col string, value interface{}) Expression {
	return &CompareExp{Col: col, Operator: ">", Value: value}
}

// LessThan generates a less-than expression (column < value).
func LessThan(col string, value interface{}) Expression {
	return &CompareExp{Col: col, Operator: "<", Value: value}
}

// GreaterOrEqual generates a greater-than-or-equal expression (column >= value).
func GreaterOrEqual(col string, value interface{}) Expression {
	return &CompareExp{Col: col, Operator: ">=", Value: value}
}

// LessOrEqual generates a less-than-or-equal expression (column <= value).
func LessOrEqual(col string, value interface{}) Expression {
	return &CompareExp{Col: col, Operator: "<=", Value: value}
}

// Build converts a comparison expression into a SQL fragment.
func (e *CompareExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	col := dialect.QuoteIdentifier(e.Col)

	// Handle NULL comparison
	if e.Value == nil {
		if e.Operator == "=" {
			return col + " IS NULL", nil
		}
		if e.Operator == "<>" {
			return col + " IS NOT NULL", nil
		}
	}

	// Handle Expression values
	if expr, ok := e.Value.(Expression); ok {
		sql, args := expr.Build(dialect)
		return col + e.Operator + "(" + sql + ")", args
	}

	// Simple comparison
	return col + e.Operator + "?", []interface{}{e.Value}
}

// InExp represents an IN or NOT IN expression.
type InExp struct {
	Col    string
	Values []interface{}
	Not    bool
}

// In generates an IN expression (column IN (value1, value2, ...)).
// If values is empty, generates "0=1" (always false).
// If values contains a single element, generates "column = value" for optimization.
func In(col string, values ...interface{}) Expression {
	return &InExp{Col: col, Values: values, Not: false}
}

// NotIn generates a NOT IN expression (column NOT IN (value1, value2, ...)).
// If values is empty, generates empty string (always true).
// If values contains a single element, generates "column <> value" for optimization.
func NotIn(col string, values ...interface{}) Expression {
	return &InExp{Col: col, Values: values, Not: true}
}

// buildInExpSingleValue handles IN expression with a single value.
func buildInExpSingleValue(col string, val interface{}, not bool) (string, []interface{}) {
	if val == nil {
		if not {
			return col + " IS NOT NULL", nil
		}
		return col + " IS NULL", nil
	}
	// Non-NULL single value
	if not {
		return col + "<>?", []interface{}{val}
	}
	return col + "=?", []interface{}{val}
}

// Build converts an IN expression into a SQL fragment.
func (e *InExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(e.Values) == 0 {
		// Empty IN clause
		if e.Not {
			return "", nil // NOT IN () → always true
		}
		return "0=1", nil // IN () → always false
	}

	col := dialect.QuoteIdentifier(e.Col)

	// Single value optimization
	if len(e.Values) == 1 {
		return buildInExpSingleValue(col, e.Values[0], e.Not)
	}

	// Multiple values
	var placeholders []string
	var args []interface{}

	for _, val := range e.Values {
		if val == nil {
			placeholders = append(placeholders, "NULL")
		} else {
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	op := "IN"
	if e.Not {
		op = "NOT IN"
	}

	sql := fmt.Sprintf("%s %s (%s)", col, op, strings.Join(placeholders, ", "))
	return sql, args
}

// BetweenExp represents a BETWEEN or NOT BETWEEN expression.
type BetweenExp struct {
	Col      string
	From, To interface{}
	Not      bool
}

// Between generates a BETWEEN expression (column BETWEEN from AND to).
func Between(col string, from, to interface{}) Expression {
	return &BetweenExp{Col: col, From: from, To: to, Not: false}
}

// NotBetween generates a NOT BETWEEN expression (column NOT BETWEEN from AND to).
func NotBetween(col string, from, to interface{}) Expression {
	return &BetweenExp{Col: col, From: from, To: to, Not: true}
}

// Build converts a BETWEEN expression into a SQL fragment.
func (e *BetweenExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	col := dialect.QuoteIdentifier(e.Col)

	op := "BETWEEN"
	if e.Not {
		op = "NOT BETWEEN"
	}

	sql := fmt.Sprintf("%s %s ? AND ?", col, op)
	return sql, []interface{}{e.From, e.To}
}

// LikeExp represents a LIKE, NOT LIKE, or ILIKE expression with automatic escaping.
type LikeExp struct {
	Col         string
	Values      []string
	Like        string   // "LIKE", "NOT LIKE", or "ILIKE"
	Or          bool     // true = OR, false = AND
	Left, Right bool     // Wildcard matching on left/right
	Escape      []string // Escape character pairs
}

// DefaultLikeEscape specifies the default special character escaping for LIKE expressions.
// The strings at 2i positions are the special characters to be escaped while those at 2i+1
// positions are the corresponding escaped versions.
var DefaultLikeEscape = []string{"\\", "\\\\", "%", "\\%", "_", "\\_"}

// Like generates a LIKE expression with automatic wildcard and escaping.
// By default, values are wrapped with % on both sides for partial matching.
//
// Example:
//
//	relica.Like("name", "john")           // name LIKE '%john%'
//	relica.Like("name", "key", "word")    // name LIKE '%key%' AND name LIKE '%word%'
func Like(col string, values ...string) *LikeExp {
	return &LikeExp{
		Col:    col,
		Values: values,
		Like:   "LIKE",
		Left:   true,
		Right:  true,
		Escape: DefaultLikeEscape,
	}
}

// NotLike generates a NOT LIKE expression.
// For example: NotLike("name", "john") → name NOT LIKE '%john%'
func NotLike(col string, values ...string) *LikeExp {
	exp := Like(col, values...)
	exp.Like = "NOT LIKE"
	return exp
}

// OrLike generates a LIKE expression where the column should match ANY of the values (OR logic).
// For example: OrLike("name", "key", "word") → name LIKE '%key%' OR name LIKE '%word%'
func OrLike(col string, values ...string) *LikeExp {
	exp := Like(col, values...)
	exp.Or = true
	return exp
}

// OrNotLike generates a NOT LIKE expression with OR logic.
func OrNotLike(col string, values ...string) *LikeExp {
	exp := NotLike(col, values...)
	exp.Or = true
	return exp
}

// Match sets wildcard matching on the left and/or right of the values.
// By default, both are true (e.g., "%value%").
// Call Match(false, true) to generate "value%" (suffix matching only).
func (e *LikeExp) Match(left, right bool) *LikeExp {
	e.Left, e.Right = left, right
	return e
}

// EscapeChars sets custom escape characters for LIKE expressions.
// Must be an even number of strings: [special1, escaped1, special2, escaped2, ...].
func (e *LikeExp) EscapeChars(chars ...string) *LikeExp {
	if len(chars)%2 != 0 {
		panic("LikeExp.EscapeChars requires even number of strings")
	}
	e.Escape = chars
	return e
}

// Build converts a LIKE expression into a SQL fragment.
func (e *LikeExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(e.Values) == 0 {
		return "", nil
	}

	col := dialect.QuoteIdentifier(e.Col)
	parts := make([]string, 0, len(e.Values))
	args := make([]interface{}, 0, len(e.Values))

	for _, val := range e.Values {
		// Escape special characters
		for j := 0; j < len(e.Escape); j += 2 {
			val = strings.ReplaceAll(val, e.Escape[j], e.Escape[j+1])
		}

		// Add wildcards
		if e.Left {
			val = "%" + val
		}
		if e.Right {
			val += "%"
		}

		parts = append(parts, fmt.Sprintf("%s %s ?", col, e.Like))
		args = append(args, val)
	}

	join := " AND "
	if e.Or {
		join = " OR "
	}

	return strings.Join(parts, join), args
}

// AndOrExp represents an AND or OR combination of multiple expressions.
type AndOrExp struct {
	Exps []Expression
	Op   string // "AND" or "OR"
}

// And generates an AND expression which concatenates multiple expressions with AND.
// Nil expressions are automatically filtered out.
//
// Example:
//
//	relica.And(
//	    relica.Eq("status", 1),
//	    relica.GreaterThan("age", 18),
//	)
//
// Generates: (status = 1) AND (age > 18)
func And(exps ...Expression) Expression {
	return &AndOrExp{Exps: exps, Op: "AND"}
}

// Or generates an OR expression which concatenates multiple expressions with OR.
// Nil expressions are automatically filtered out.
func Or(exps ...Expression) Expression {
	return &AndOrExp{Exps: exps, Op: "OR"}
}

// Build converts an AND/OR expression into a SQL fragment.
func (e *AndOrExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if len(e.Exps) == 0 {
		return "", nil
	}

	var parts []string
	var args []interface{}

	for _, exp := range e.Exps {
		if exp == nil {
			continue
		}

		sql, subArgs := exp.Build(dialect)
		if sql != "" {
			parts = append(parts, sql)
			args = append(args, subArgs...)
		}
	}

	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 {
		return parts[0], args
	}

	// Wrap each part in parentheses for correct precedence
	return "(" + strings.Join(parts, ") "+e.Op+" (") + ")", args
}

// NotExp represents a NOT expression which prefixes NOT to an expression.
type NotExp struct {
	Exp Expression
}

// Not generates a NOT expression which prefixes "NOT" to the specified expression.
//
// Example:
//
//	relica.Not(relica.In("status", 1, 2, 3))
//
// Generates: NOT (status IN (1, 2, 3))
func Not(exp Expression) Expression {
	return &NotExp{Exp: exp}
}

// Build converts a NOT expression into a SQL fragment.
func (e *NotExp) Build(dialect dialects.Dialect) (string, []interface{}) {
	if e.Exp == nil {
		return "", nil
	}

	sql, args := e.Exp.Build(dialect)
	if sql == "" {
		return "", nil
	}

	return "NOT (" + sql + ")", args
}
