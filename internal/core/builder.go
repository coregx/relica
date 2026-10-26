package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/coregx/relica/internal/analyzer"
	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/util"
)

// selectAliasRegex matches explicit "AS alias" in SELECT columns.
// Only matches explicit AS keyword to avoid false positives with expressions like "level + 1".
var selectAliasRegex = regexp.MustCompile(`(?i)\s+AS\s+([\w\-.]+)$`)

// replacePlaceholders replaces positional ? placeholders with dialect-specific
// placeholders (e.g. $1, $2 for PostgreSQL), starting from startIndex.
//
// Handles PostgreSQL JSONB operators and SQL syntax:
//   - ?? (client-side escape) → emits single ? to the server
//   - ?| and ?& (JSONB array operators) → preserved as-is
//   - ? followed by 'literal' (JSONB key-existence) → preserved as-is
//   - ? inside single-quoted strings ('why?') → not replaced
//   - ? inside SQL comments (-- ... or /* ... */) → not replaced
//   - Escaped quotes ('it”s') → handled correctly
//
// For a JSONB key from a parameter, write: data ?? ? (first ?? escapes to ?,
// second ? becomes $N).
//
// Known limitations: E'...\'...' (C-style escapes), $$...$$ (dollar-quoting),
// and “odd?col” (quoted identifiers with ?) are not handled.
//
// For MySQL/SQLite dialects where Placeholder(1) == “?”, clause is returned unchanged.
//
//nolint:funlen // SQL lexer requires sequential state tracking; splitting reduces clarity.
func replacePlaceholders(clause string, startIndex int, dialect dialects.Dialect) (string, int) {
	var b strings.Builder
	b.Grow(len(clause) + 16)
	inString := false
	paramIdx := startIndex

	for i := 0; i < len(clause); i++ {
		ch := clause[i]

		switch {
		case ch == '\'':
			if inString {
				// Check for escaped quote '' inside a string literal.
				if i+1 < len(clause) && clause[i+1] == '\'' {
					b.WriteByte(ch)
					i++
					b.WriteByte(clause[i])
					continue
				}
				// Closing quote.
				inString = false
			} else {
				inString = true
			}
			b.WriteByte(ch)

		case ch == '-' && !inString && i+1 < len(clause) && clause[i+1] == '-':
			// Single-line SQL comment: skip until end of line
			b.WriteByte(ch)
			i++
			b.WriteByte(clause[i])
			for i+1 < len(clause) && clause[i+1] != '\n' {
				i++
				b.WriteByte(clause[i])
			}

		case ch == '/' && !inString && i+1 < len(clause) && clause[i+1] == '*':
			// Block comment: skip until */
			b.WriteByte(ch)
			i++
			b.WriteByte(clause[i])
			for i+1 < len(clause) {
				i++
				b.WriteByte(clause[i])
				if clause[i] == '/' && clause[i-1] == '*' {
					break
				}
			}

		case ch == '?' && !inString:
			// PostgreSQL JSONB operators: ?? → emit single ? (client-side escape),
			// ?| and ?& → emit as-is (array operators, not placeholders).
			if i+1 < len(clause) {
				next := clause[i+1]
				if next == '?' {
					// ?? is client-side escape for literal ? in PostgreSQL
					b.WriteByte('?')
					i++
					continue
				}
				if next == '|' || next == '&' {
					// ?| and ?& are JSONB array operators
					b.WriteByte(ch)
					b.WriteByte(next)
					i++
					continue
				}
			}
			// JSONB single-key operator: data ? 'key'
			// Heuristic: if the next non-space char is a single quote,
			// this is a JSONB key-existence test, not a placeholder.
			isJSONBOp := false
			for j := i + 1; j < len(clause); j++ {
				if clause[j] == ' ' || clause[j] == '\t' {
					continue
				}
				if clause[j] == '\'' {
					isJSONBOp = true
				}
				break
			}
			if isJSONBOp {
				b.WriteByte(ch)
				continue
			}
			b.WriteString(dialect.Placeholder(paramIdx))
			paramIdx++

		default:
			b.WriteByte(ch)
		}
	}

	return b.String(), paramIdx - startIndex
}

// resolveNamedParams checks if the SQL condition contains named placeholders {:name}
// and resolves them to positional ? placeholders using the provided Params map.
// If the condition has no named placeholders, returns it unchanged with original params.
// Returns an error if any named placeholder has no corresponding key in the Params map.
//
// Usage in Where:
//
//	Where("id = {:id} AND status = {:status}", relica.Params{"id": 1, "status": "active"})
//	Where("status = ?", 1)  // also works (positional)
func resolveNamedParams(condition string, params []any) (string, []any, error) {
	if !namedPlaceholderRegex.MatchString(condition) {
		return condition, params, nil
	}
	if len(params) != 1 {
		return condition, params, nil
	}
	p, ok := asParams(params[0])
	if !ok {
		return condition, params, nil
	}
	var orderedArgs []any
	var missing []string
	resolved := namedPlaceholderRegex.ReplaceAllStringFunc(condition, func(match string) string {
		name := match[2 : len(match)-1]
		if val, exists := p[name]; exists {
			orderedArgs = append(orderedArgs, val)
		} else {
			missing = append(missing, ":"+name)
			orderedArgs = append(orderedArgs, nil) // placeholder to keep arg count aligned
		}
		return "?"
	})
	if len(missing) > 0 {
		return resolved, orderedArgs, fmt.Errorf("relica: missing named parameter(s): %s", strings.Join(missing, ", "))
	}
	return resolved, orderedArgs, nil
}

// QueryPlan represents a unified query execution plan from database EXPLAIN.
// Type alias from internal/analyzer package.
type QueryPlan = analyzer.QueryPlan

// newAnalyzerForDB creates a database-specific analyzer.
func newAnalyzerForDB(db *DB) (analyzer.Analyzer, error) {
	switch db.driverName {
	case "postgres", "postgresql":
		return analyzer.NewPostgresAnalyzer(db.sqlDB), nil
	case "mysql":
		return analyzer.NewMySQLAnalyzer(db.sqlDB), nil
	case "sqlite", "sqlite3":
		return analyzer.NewSQLiteAnalyzer(db.sqlDB), nil
	default:
		return nil, fmt.Errorf("EXPLAIN not supported for driver: %s", db.driverName)
	}
}

// QueryBuilder constructs type-safe queries.
// When tx is not nil, all queries execute within that transaction.
type QueryBuilder struct {
	db  *DB
	tx  *sql.Tx         // nil for non-transactional queries
	ctx context.Context // context for all queries built by this builder
}

// WithContext sets the context for all queries built by this builder.
// The context will be used for all subsequent query operations unless overridden.
func (qb *QueryBuilder) WithContext(ctx context.Context) *QueryBuilder {
	qb.ctx = ctx
	return qb
}

// JoinInfo represents a JOIN clause in SELECT query.
type JoinInfo struct {
	JoinType string // "INNER JOIN", "LEFT JOIN", "RIGHT JOIN", "FULL OUTER JOIN", "CROSS JOIN"
	Table    string // Table name with optional alias: "users u", "messages m"
	On       any    // string | Expression | nil
}

// unionInfo represents a set operation (UNION, INTERSECT, EXCEPT) between queries.
// Database support for set operations:
//
// PostgreSQL 9.1+:  UNION ✓, UNION ALL ✓, INTERSECT ✓, EXCEPT ✓
// MySQL 8.0+:       UNION ✓, UNION ALL ✓
// MySQL 8.0.31+:    UNION ✓, UNION ALL ✓, INTERSECT ✓, EXCEPT ✓
// SQLite 3.25+:     UNION ✓, UNION ALL ✓, INTERSECT ✓, EXCEPT ✓
type unionInfo struct {
	query *SelectQuery // The query to combine with
	all   bool         // true = UNION ALL, false = UNION (removes duplicates)
	op    string       // "UNION", "INTERSECT", "EXCEPT" (default: "UNION")
}

// fromSource represents the source of a FROM clause (table name or subquery).
type fromSource struct {
	isSubquery bool
	table      string       // table name (when isSubquery = false)
	subquery   *SelectQuery // subquery (when isSubquery = true)
	alias      string       // alias for subquery (required for subqueries)
}

// cteInfo represents a Common Table Expression (CTE).
type cteInfo struct {
	name      string       // CTE name (e.g., "sales_summary")
	query     *SelectQuery // The CTE query
	recursive bool         // true for WITH RECURSIVE
}

// subExprEntry holds a type-safe SELECT expression (e.g. a correlated subquery)
// together with the alias it will be given in the SELECT clause.
type subExprEntry struct {
	exp   Expression
	alias string
}

// SelectQuery represents a SELECT query being built.
type SelectQuery struct {
	builder       *QueryBuilder
	scanMode      *ScanMode // per-query override of DB.scanMode (ADR-018)
	columns       []string
	table         string         // DEPRECATED: use fromSrc instead (kept for backward compatibility)
	fromSrc       *fromSource    // FROM source (table or subquery)
	selectExprs   []RawExp       // Raw SELECT expressions (for scalar subqueries, etc.)
	subExprs      []subExprEntry // Type-safe SELECT expressions (subqueries, computed columns)
	joins         []JoinInfo
	where         []string
	params        []any
	groupBy       []clauseItem // GROUP BY items in call order (GroupBy, GroupByExpr, GroupBySub)
	havingClauses []struct {
		condition string
		args      []any
	} // HAVING clauses (WHERE for aggregates)
	orderBy     []clauseItem    // ORDER BY items in call order (OrderBy, OrderByExpr, OrderBySub)
	limitValue  *int64          // LIMIT value (nil = not set)
	offsetValue *int64          // OFFSET value (nil = not set)
	unions      []unionInfo     // Set operations: UNION, INTERSECT, EXCEPT
	ctes        []cteInfo       // Common Table Expressions (CTEs)
	distinct    bool            // SELECT DISTINCT flag
	lockClause  string          // FOR UPDATE, FOR SHARE, etc. (empty = no lock)
	ctx         context.Context // context for this specific query
	buildErr    error           // stored programming error (replaces panic in fluent chain)
}

// clauseItem is one ORDER BY or GROUP BY entry. The column, raw and
// type-safe forms share one list so that the clause renders, and binds its
// parameters, in the order the builder methods were called.
type clauseItem struct {
	column string     // GroupBy/OrderBy: column name (ORDER BY: optional ASC/DESC), quoted when rendered
	raw    *RawExp    // GroupByExpr/OrderByExpr: SQL used as is
	exp    Expression // GroupBySub/OrderBySub
}

// ForUpdate adds FOR UPDATE clause for pessimistic row-level locking.
//
// Row locks need a transaction: outside one the lock ends with the statement,
// so Build returns an error on every dialect (ADR-020). A lock combined with
// UNION/INTERSECT/EXCEPT, GROUP BY, DISTINCT, Count() or Exists() is an error
// too. Inside a transaction SQLite drops the clause (database-level locking).
func (sq *SelectQuery) ForUpdate() *SelectQuery {
	sq.lockClause = "FOR UPDATE"
	return sq
}

// ForShare adds FOR SHARE clause for shared row-level locking. Same rules as
// ForUpdate. MySQL 8.0+ only: MariaDB has no FOR SHARE (it uses LOCK IN SHARE MODE).
func (sq *SelectQuery) ForShare() *SelectQuery {
	sq.lockClause = "FOR SHARE"
	return sq
}

// ForUpdateSkipLocked adds FOR UPDATE SKIP LOCKED for non-blocking lock
// acquisition (job queues). Same rules as ForUpdate: outside a transaction two
// workers would claim the same row, so it is a build error.
func (sq *SelectQuery) ForUpdateSkipLocked() *SelectQuery {
	sq.lockClause = "FOR UPDATE SKIP LOCKED"
	return sq
}

// WithContext sets the context for this SELECT query.
// This overrides any context set on the QueryBuilder.
func (sq *SelectQuery) WithContext(ctx context.Context) *SelectQuery {
	sq.ctx = ctx
	return sq
}

// From specifies the table to select from.
func (sq *SelectQuery) From(table string) *SelectQuery {
	sq.table = table
	sq.fromSrc = &fromSource{
		isSubquery: false,
		table:      table,
	}
	return sq
}

// FromSelect specifies a subquery as the FROM source.
// The alias parameter is required and will be used to reference the subquery in the outer query.
//
// Example:
//
//	sub := db.Builder().Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")
//	db.Builder().FromSelect(sub, "order_counts").
//	    Select("user_id", "cnt").
//	    Where("cnt > ?", 10).
//	    All(&results)
//
// Generates:
//
//	SELECT user_id, cnt
//	FROM (SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id) AS order_counts
//	WHERE cnt > 10
func (sq *SelectQuery) FromSelect(subquery *SelectQuery, alias string) *SelectQuery {
	if alias == "" {
		sq.buildErr = fmt.Errorf("relica: FromSelect requires a non-empty alias for the subquery")
		return sq
	}
	if subquery == nil {
		sq.buildErr = fmt.Errorf("relica: FromSelect requires a non-nil subquery")
		return sq
	}
	sq.fromSrc = &fromSource{
		isSubquery: true,
		subquery:   subquery,
		alias:      alias,
	}
	// Clear table for safety (subquery takes precedence)
	sq.table = ""
	return sq
}

// AndSelect appends additional columns to the SELECT clause.
// Useful for conditional column building where columns are added based on runtime conditions.
func (sq *SelectQuery) AndSelect(cols ...string) *SelectQuery {
	sq.columns = append(sq.columns, cols...)
	return sq
}

// SelectExpr adds a raw SQL expression to the SELECT clause.
// This is useful for scalar subqueries, window functions, or complex expressions.
//
// Example:
//
//	db.Builder().
//	    Select("id", "name").
//	    SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) AS order_count").
//	    From("users").
//	    All(&results)
//
// Generates:
//
//	SELECT "id", "name", (SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) AS order_count
//	FROM users
//
// Note: The SQL expression is used as-is. You're responsible for proper quoting and SQL injection prevention.
func (sq *SelectQuery) SelectExpr(expr string, args ...any) *SelectQuery {
	sq.selectExprs = append(sq.selectExprs, RawExp{
		SQL:  expr,
		Args: args,
	})
	return sq
}

// SelectSub adds a type-safe SQL expression to the SELECT clause with a quoted alias.
// The expression is built using the current dialect, so identifiers are quoted correctly.
// This is the recommended way to add correlated subqueries or computed columns.
//
// Example:
//
//	sub := db.Builder().Select("COUNT(*)").From("orders").Where(relica.EqCol("orders.user_id", "users.id"))
//	db.Builder().Select("id", "name").SelectSub(sub.AsExpression(), "order_count").From("users").All(&results)
//
// Generates (PostgreSQL):
//
//	SELECT "id", "name", (SELECT COUNT(*) FROM "orders" WHERE "orders"."user_id" = "users"."id") AS "order_count" FROM "users"
func (sq *SelectQuery) SelectSub(exp Expression, alias string) *SelectQuery {
	if alias == "" {
		sq.buildErr = fmt.Errorf("relica: SelectSub requires a non-empty alias")
		return sq
	}
	if err := ExpressionError(exp); err != nil {
		sq.buildErr = err
		return sq
	}
	sq.subExprs = append(sq.subExprs, subExprEntry{exp: exp, alias: alias})
	return sq
}

// Where adds a WHERE condition.
// Accepts either a string with placeholders or an Expression.
//
// String example:
//
//	Where("status = ? AND age > ?", 1, 18)
//
// Expression example:
//
//	Where(relica.And(
//	    relica.Eq("status", 1),
//	    relica.GreaterThan("age", 18),
//	))
func (sq *SelectQuery) Where(condition any, params ...any) *SelectQuery {
	switch cond := condition.(type) {
	case string:
		resolved, resolvedArgs, err := resolveNamedParams(cond, params)
		if err != nil {
			sq.buildErr = err
			return sq
		}
		sq.where = append(sq.where, resolved)
		sq.params = append(sq.params, resolvedArgs...)

	case Expression:
		if err := ExpressionError(cond); err != nil {
			sq.buildErr = err
			return sq
		}
		sqlStr, args := cond.Build(sq.builder.db.dialect)
		if sqlStr != "" {
			sq.where = append(sq.where, sqlStr)
			sq.params = append(sq.params, args...)
		}

	default:
		sq.buildErr = fmt.Errorf("relica: Where() expects string or Expression, got %T", condition)
	}

	return sq
}

// AndWhere adds a WHERE condition with AND logic.
// If no existing WHERE clause exists, behaves like Where().
// Multiple conditions are combined with AND.
//
// String example:
//
//	AndWhere("age > ?", 18)
//
// Expression example:
//
//	AndWhere(relica.GreaterThan("age", 18))
func (sq *SelectQuery) AndWhere(condition any, params ...any) *SelectQuery {
	// Simply delegate to Where() which already uses AND logic for multiple calls.
	return sq.Where(condition, params...)
}

// OrWhere adds a WHERE condition with OR logic.
// If no existing WHERE clause exists, behaves like Where().
// The new condition is combined with existing conditions using OR.
//
// String example:
//
//	OrWhere("status = ?", "admin")
//
// Expression example:
//
//	OrWhere(relica.Eq("status", "admin"))
func (sq *SelectQuery) OrWhere(condition any, params ...any) *SelectQuery {
	if len(sq.where) == 0 {
		// No existing WHERE clause - just add it.
		return sq.Where(condition, params...)
	}

	// Build the new condition.
	var newSQL string
	var newArgs []any

	switch cond := condition.(type) {
	case string:
		var err error
		newSQL, newArgs, err = resolveNamedParams(cond, params)
		if err != nil {
			sq.buildErr = err
			return sq
		}

	case Expression:
		if err := ExpressionError(cond); err != nil {
			sq.buildErr = err
			return sq
		}
		newSQL, newArgs = cond.Build(sq.builder.db.dialect)
		if newSQL == "" {
			return sq
		}

	default:
		sq.buildErr = fmt.Errorf("relica: OrWhere() expects string or Expression, got %T", condition)
		return sq
	}

	// Combine existing WHERE with new condition using OR.
	// Wrap both sides in parentheses for correct precedence.
	// Existing: "status = $1 AND age > $2" → "(status = $1 AND age > $2)"
	// Combined: "(status = $1 AND age > $2) OR (role = $3)"
	existingWhere := strings.Join(sq.where, " AND ")
	combined := "(" + existingWhere + ") OR (" + newSQL + ")"

	// Replace WHERE clauses with combined condition.
	sq.where = []string{combined}
	sq.params = append(sq.params, newArgs...)

	return sq
}

// Join adds a generic JOIN clause to the SELECT query.
// joinType specifies the type of join ("INNER JOIN", "LEFT JOIN", etc.).
// table is the table name with optional alias (e.g., "users u", "messages m").
// on can be a string, Expression, or nil (for CROSS JOIN).
//
// Example:
//
//	Join("INNER JOIN", "users u", "m.user_id = u.id")
//	Join("LEFT JOIN", "attachments a", relica.Eq("m.id", relica.Raw("a.message_id")))
func (sq *SelectQuery) Join(joinType, table string, on any) *SelectQuery {
	if err := ExpressionError(on); err != nil {
		sq.buildErr = err
		return sq
	}
	sq.joins = append(sq.joins, JoinInfo{
		JoinType: joinType,
		Table:    table,
		On:       on,
	})
	return sq
}

// InnerJoin adds an INNER JOIN clause to the SELECT query.
// table is the table name with optional alias (e.g., "users u").
// on can be a string or Expression specifying the join condition.
//
// Example:
//
//	InnerJoin("users u", "m.user_id = u.id")
//	InnerJoin("users u", relica.Eq("m.user_id", relica.Raw("u.id")))
func (sq *SelectQuery) InnerJoin(table string, on any) *SelectQuery {
	return sq.Join("INNER JOIN", table, on)
}

// LeftJoin adds a LEFT JOIN (LEFT OUTER JOIN) clause to the SELECT query.
// table is the table name with optional alias (e.g., "attachments a").
// on can be a string or Expression specifying the join condition.
//
// Example:
//
//	LeftJoin("attachments a", "m.id = a.message_id")
func (sq *SelectQuery) LeftJoin(table string, on any) *SelectQuery {
	return sq.Join("LEFT JOIN", table, on)
}

// RightJoin adds a RIGHT JOIN (RIGHT OUTER JOIN) clause to the SELECT query.
// table is the table name with optional alias.
// on can be a string or Expression specifying the join condition.
//
// Example:
//
//	RightJoin("users u", "m.user_id = u.id")
func (sq *SelectQuery) RightJoin(table string, on any) *SelectQuery {
	return sq.Join("RIGHT JOIN", table, on)
}

// FullJoin adds a FULL OUTER JOIN clause to the SELECT query.
// table is the table name with optional alias.
// on can be a string or Expression specifying the join condition.
// Note: Not supported by MySQL. Use PostgreSQL or SQLite.
//
// Example:
//
//	FullJoin("users u", "m.user_id = u.id")
func (sq *SelectQuery) FullJoin(table string, on any) *SelectQuery {
	return sq.Join("FULL OUTER JOIN", table, on)
}

// CrossJoin adds a CROSS JOIN clause to the SELECT query.
// table is the table name with optional alias.
// CROSS JOIN does not require an ON condition (Cartesian product).
//
// Example:
//
//	CrossJoin("attachments")
func (sq *SelectQuery) CrossJoin(table string) *SelectQuery {
	return sq.Join("CROSS JOIN", table, nil)
}

// OrderBy adds ORDER BY clause with optional direction (ASC/DESC).
// Supports multiple columns with optional direction specification.
// Chainable: OrderBy, OrderByExpr and OrderBySub append to the same clause
// in call order.
//
// Examples:
//
//	OrderBy("age DESC")                    // Single column descending
//	OrderBy("status ASC", "created_at")    // Multiple columns (created_at defaults to ASC)
//	OrderBy("name").OrderBy("age DESC")    // Chained calls
//
// Each argument is "column" or "column ASC|DESC"; an argument containing "("
// (a function call) is used as is. Anything else — arithmetic, NULLS
// FIRST/LAST, COLLATE, several columns in one string — is a build error: use
// OrderByExpr, or one argument per column.
func (sq *SelectQuery) OrderBy(columns ...string) *SelectQuery {
	for _, col := range columns {
		item, err := orderByItem(col)
		if err != nil {
			if sq.buildErr == nil {
				sq.buildErr = err
			}
			continue
		}
		sq.orderBy = append(sq.orderBy, item)
	}
	return sq
}

// orderByItem parses one OrderBy argument. The parser it replaces kept the
// first word and the direction and dropped the rest without a word:
// "salary + 1 DESC" became ORDER BY "salary", and NULLS LAST, COLLATE or a
// second column after a comma vanished. ozzo-dbx quotes everything before the
// direction as one name, which at least fails in the database; Relica rejects
// it before anything is sent.
func orderByItem(col string) (clauseItem, error) {
	if strings.Contains(col, "(") {
		// A function call or expression, used as is like everywhere a column
		// name may contain "(" (quoteColumn).
		return clauseItem{raw: &RawExp{SQL: strings.TrimSpace(col)}}, nil
	}
	fields := strings.Fields(col)
	switch {
	case len(fields) == 1:
		return clauseItem{column: fields[0]}, nil
	case len(fields) == 2 && (strings.EqualFold(fields[1], "ASC") || strings.EqualFold(fields[1], "DESC")):
		return clauseItem{column: fields[0] + " " + strings.ToUpper(fields[1])}, nil
	}
	return clauseItem{}, fmt.Errorf("relica: OrderBy(%q) takes \"column\" or \"column ASC|DESC\"; "+
		"use OrderByExpr for expressions, NULLS FIRST/LAST, COLLATE, and one argument per column", col)
}

// OrderByExpr adds a raw SQL expression to the ORDER BY clause.
// The expression is used as-is without quoting — useful for CASE WHEN,
// complex functions, or any expression that shouldn't be treated as a column name.
//
// Example:
//
//	OrderByExpr("CASE WHEN status = ? THEN 0 ELSE 1 END", "active")
//	OrderByExpr("FIELD(id, ?, ?, ?)", 3, 1, 2)
func (sq *SelectQuery) OrderByExpr(expr string, args ...any) *SelectQuery {
	sq.orderBy = append(sq.orderBy, clauseItem{raw: &RawExp{SQL: expr, Args: args}})
	return sq
}

// OrderBySub adds a type-safe expression to the ORDER BY clause.
// Use with CaseWhen() or any Expression builder for complex ordering without raw SQL.
//
// Example:
//
//	OrderBySub(relica.CaseWhen().
//	    When("t.due_date < CURRENT_DATE", 0).
//	    When("t.due_date IS NULL", 3).
//	    Else(1))
func (sq *SelectQuery) OrderBySub(exp Expression) *SelectQuery {
	if err := ExpressionError(exp); err != nil {
		sq.buildErr = err
		return sq
	}
	sq.orderBy = append(sq.orderBy, clauseItem{exp: exp})
	return sq
}

// Limit sets the LIMIT clause for the query.
// Limits the number of rows returned by the query.
//
// Example:
//
//	Limit(100)  // Return at most 100 rows
func (sq *SelectQuery) Limit(limit int64) *SelectQuery {
	sq.limitValue = &limit
	return sq
}

// Offset sets the OFFSET clause for the query.
// Skips the specified number of rows before returning results.
//
// Example:
//
//	Offset(200)  // Skip first 200 rows
func (sq *SelectQuery) Offset(offset int64) *SelectQuery {
	sq.offsetValue = &offset
	return sq
}

// Union combines this query with another using UNION (removes duplicates).
// UNION returns distinct rows from both queries.
//
// Example:
//
//	q1 := db.Builder().Select("name").From("users").Where("status = ?", 1)
//	q2 := db.Builder().Select("name").From("archived_users").Where("status = ?", 1)
//	q1.Union(q2).All(&names)
//
// Generates:
//
//	(SELECT name FROM users WHERE status = $1) UNION (SELECT name FROM archived_users WHERE status = $2)
//
// Note: Column count and types must match between queries.
func (sq *SelectQuery) Union(other *SelectQuery) *SelectQuery {
	return sq.addSetOperation("Union", other, unionInfo{query: other, all: false, op: "UNION"})
}

// addSetOperation appends a set-operation branch; a nil branch is a build
// error, not a silently dropped branch.
func (sq *SelectQuery) addSetOperation(method string, other *SelectQuery, u unionInfo) *SelectQuery {
	if other == nil {
		sq.buildErr = fmt.Errorf("relica: %s() requires a non-nil query", method)
		return sq
	}
	sq.unions = append(sq.unions, u)
	return sq
}

// UnionAll combines this query with another using UNION ALL (keeps duplicates).
// UNION ALL is faster than UNION as it doesn't remove duplicates.
//
// Example:
//
//	q1 := db.Builder().Select("id").From("orders_2023")
//	q2 := db.Builder().Select("id").From("orders_2024")
//	q1.UnionAll(q2).All(&orderIDs)
//
// Generates:
//
//	(SELECT id FROM orders_2023) UNION ALL (SELECT id FROM orders_2024)
func (sq *SelectQuery) UnionAll(other *SelectQuery) *SelectQuery {
	return sq.addSetOperation("UnionAll", other, unionInfo{query: other, all: true, op: "UNION"})
}

// Intersect combines this query with another using INTERSECT (rows in both queries).
// INTERSECT returns only rows that appear in both result sets.
//
// Example:
//
//	q1 := db.Builder().Select("id").From("users")
//	q2 := db.Builder().Select("user_id").From("orders")
//	q1.Intersect(q2).All(&ids)  // Users who have placed orders
//
// Generates:
//
//	(SELECT id FROM users) INTERSECT (SELECT user_id FROM orders)
//
// Database support:
//   - PostgreSQL 9.1+: ✓
//   - MySQL 8.0.31+: ✓ (earlier versions will return error)
//   - SQLite 3.25+: ✓
func (sq *SelectQuery) Intersect(other *SelectQuery) *SelectQuery {
	return sq.addSetOperation("Intersect", other, unionInfo{query: other, all: false, op: "INTERSECT"})
}

// Except combines this query with another using EXCEPT (rows in first but not second).
// EXCEPT returns rows from the first query that don't appear in the second.
// Also known as MINUS in Oracle.
//
// Example:
//
//	q1 := db.Builder().Select("id").From("all_users")
//	q2 := db.Builder().Select("user_id").From("banned_users")
//	q1.Except(q2).All(&activeUsers)  // All users except banned ones
//
// Generates:
//
//	(SELECT id FROM all_users) EXCEPT (SELECT user_id FROM banned_users)
//
// Database support:
//   - PostgreSQL 9.1+: ✓
//   - MySQL 8.0.31+: ✓ (earlier versions will return error)
//   - SQLite 3.25+: ✓
func (sq *SelectQuery) Except(other *SelectQuery) *SelectQuery {
	return sq.addSetOperation("Except", other, unionInfo{query: other, all: false, op: "EXCEPT"})
}

// With adds a Common Table Expression (CTE) to the query.
//
// Example:
//
//	cte := db.Builder().Select("user_id", "SUM(total) as total").
//	    From("orders").
//	    GroupBy("user_id")
//
//	result := db.Builder().Select("*").
//	    With("order_totals", cte).
//	    From("order_totals").
//	    Where("total > ?", 1000).
//	    All(&users)
//
// Generates:
//
//	WITH "order_totals" AS (SELECT user_id, SUM(total) as total FROM "orders" GROUP BY user_id)
//	SELECT * FROM "order_totals" WHERE total > $1
func (sq *SelectQuery) With(name string, query *SelectQuery) *SelectQuery {
	if name == "" {
		sq.buildErr = fmt.Errorf("relica: With() requires a non-empty CTE name")
		return sq
	}
	if query == nil {
		sq.buildErr = fmt.Errorf("relica: With() requires a non-nil CTE query")
		return sq
	}
	sq.ctes = append(sq.ctes, cteInfo{
		name:      name,
		query:     query,
		recursive: false,
	})
	return sq
}

// WithRecursive adds a recursive Common Table Expression (CTE) to the query.
// The query MUST use UNION or UNION ALL to separate the anchor and recursive parts.
//
// Example (organizational hierarchy):
//
//	anchor := db.Builder().Select("id", "name", "manager_id").SelectExpr("1 AS level").
//	    From("employees").
//	    Where("manager_id IS NULL")
//
//	recursive := db.Builder().Select("e.id", "e.name", "e.manager_id").SelectExpr("h.level + 1").
//	    From("employees e").
//	    InnerJoin("hierarchy h", "e.manager_id = h.id")
//
//	cte := anchor.UnionAll(recursive)
//
//	result := db.Builder().Select("*").
//	    WithRecursive("hierarchy", cte).
//	    From("hierarchy").
//	    OrderBy("level", "name").
//	    All(&employees)
//
// Generates:
//
//	WITH RECURSIVE "hierarchy" AS (
//	    SELECT "id", "name", "manager_id", 1 AS level FROM "employees" WHERE manager_id IS NULL
//	    UNION ALL
//	    SELECT "e"."id", "e"."name", "e"."manager_id", h.level + 1 FROM "employees" AS "e" INNER JOIN hierarchy h ON e.manager_id = h.id
//	)
//	SELECT * FROM "hierarchy" ORDER BY "level", "name"
//
// Database support:
//   - PostgreSQL: ✓ (all versions)
//   - MySQL 8.0+: ✓ (added in MySQL 8.0.1)
//   - SQLite 3.25+: ✓ (added in SQLite 3.25.0)
func (sq *SelectQuery) WithRecursive(name string, query *SelectQuery) *SelectQuery {
	if name == "" {
		sq.buildErr = fmt.Errorf("relica: WithRecursive() requires a non-empty CTE name")
		return sq
	}
	if query == nil {
		sq.buildErr = fmt.Errorf("relica: WithRecursive() requires a non-nil CTE query")
		return sq
	}
	// Validate that query contains UNION (required for recursive CTE)
	if len(query.unions) == 0 {
		sq.buildErr = fmt.Errorf("relica: WithRecursive() requires a query with UNION or UNION ALL (recursive CTE must have an anchor and recursive part)")
		return sq
	}
	sq.ctes = append(sq.ctes, cteInfo{
		name:      name,
		query:     query,
		recursive: true,
	})
	return sq
}

// Distinct adds the DISTINCT keyword to the SELECT clause, eliminating duplicate rows.
//
// Example:
//
//	db.Builder().Select("category").From("products").Distinct().All(&categories)
//	// SELECT DISTINCT "category" FROM "products"
func (sq *SelectQuery) Distinct() *SelectQuery {
	sq.distinct = true
	return sq
}

// buildTableWithAlias builds a table reference with optional alias.
// Input: "users u" → Output: "users" AS "u" (quoted)
// Input: "public.users u" → Output: "public"."users" AS "u" (schema-qualified, quoted)
// Input: "users" → Output: "users" (quoted)
// Input: "public.users" → Output: "public"."users" (schema-qualified, quoted)
//
// Uses quoteColumn for the table part so schema.table identifiers are quoted per-part
// rather than as a single string (which would produce "public.users" instead of "public"."users").
func (sq *SelectQuery) buildTableWithAlias(table string, dialect dialects.Dialect) string {
	tableParts := strings.Fields(table)
	if len(tableParts) == 2 {
		// Table (possibly schema-qualified) with alias
		quotedTable := quoteColumn(tableParts[0], dialect)
		quotedAlias := dialect.QuoteIdentifier(tableParts[1])
		return quotedTable + " AS " + quotedAlias
	}
	// Simple table name or schema-qualified (no alias)
	return quoteColumn(table, dialect)
}

// buildFrom constructs the FROM clause, handling both tables and subqueries.
// Returns the FROM SQL fragment and appends any subquery parameters to params.
// When called from renderSQL, subqueries are rendered with renderSQL so all
// placeholders remain as ? for the single final replacePlaceholders pass.
func (sq *SelectQuery) buildFrom(dialect dialects.Dialect, params *[]any) string {
	// Prefer fromSrc if set (supports subqueries)
	if sq.fromSrc != nil {
		if sq.fromSrc.isSubquery {
			// FROM (SELECT ...) AS alias — use renderSQL so ? placeholders are
			// preserved for the single outer replacePlaceholders pass in buildSQL.
			subSQL, subArgs := sq.fromSrc.subquery.renderSQL(dialect)
			*params = append(*params, subArgs...)
			quotedAlias := dialect.QuoteIdentifier(sq.fromSrc.alias)
			return " FROM (" + subSQL + ") AS " + quotedAlias
		}
		// Regular table
		return " FROM " + sq.buildTableWithAlias(sq.fromSrc.table, dialect)
	}

	// Fallback to legacy table field (backward compatibility)
	if sq.table != "" {
		return " FROM " + sq.buildTableWithAlias(sq.table, dialect)
	}

	// No FROM clause (e.g., SELECT 1)
	return ""
}

// buildJoins constructs the JOIN clause from the joins slice.
// Returns empty string if no joins are specified.
// On unsupported ON type, stores the error in sq.buildErr and returns empty string.
func (sq *SelectQuery) buildJoins(dialect dialects.Dialect, params *[]any) string {
	if len(sq.joins) == 0 {
		return ""
	}

	parts := make([]string, 0, len(sq.joins))

	for _, join := range sq.joins {
		part := " " + join.JoinType + " "

		// Build table with optional alias
		part += sq.buildTableWithAlias(join.Table, dialect)

		// Build ON condition
		if join.On != nil {
			part += " ON "

			switch on := join.On.(type) {
			case string:
				// String-based ON: use as-is
				part += on

			case Expression:
				// Expression-based ON
				sqlStr, args := on.Build(dialect)
				part += sqlStr
				*params = append(*params, args...)

			default:
				sq.buildErr = fmt.Errorf("relica: JOIN ON must be string, Expression, or nil, got %T", join.On)
				return ""
			}
		}

		parts = append(parts, part)
	}

	return strings.Join(parts, "")
}

// buildOrderBy constructs the ORDER BY clause and its parameters.
// Returns empty string if no ORDER BY is specified.
// Parses column direction (ASC/DESC) and quotes column names.
func (sq *SelectQuery) buildOrderBy(dialect dialects.Dialect) (string, []any) {
	parts, args := buildClauseItems(sq.orderBy, dialect, func(col string) (string, bool) {
		// Parse "column [ASC|DESC]"
		fields := strings.Fields(col)
		if len(fields) == 0 {
			return "", false
		}

		// Quote column name (may include table prefix: "users.age" → "users"."age")
		quoted := sq.quoteColumnName(fields[0], dialect)

		// Add direction if specified
		if len(fields) > 1 {
			direction := strings.ToUpper(fields[1])
			if direction == "ASC" || direction == "DESC" {
				quoted += " " + direction
			}
		}
		return quoted, true
	})
	if len(parts) == 0 {
		return "", nil
	}

	return " ORDER BY " + strings.Join(parts, ", "), args
}

// buildClauseItems renders ORDER BY or GROUP BY items in call order and
// collects their parameters in the same order. column renders a column item;
// it returns false to drop the item.
func buildClauseItems(items []clauseItem, dialect dialects.Dialect, column func(string) (string, bool)) ([]string, []any) {
	parts := make([]string, 0, len(items))
	var args []any
	for _, item := range items {
		switch {
		case item.raw != nil:
			parts = append(parts, item.raw.SQL)
			args = append(args, item.raw.Args...)
		case item.exp != nil:
			expSQL, expArgs := item.exp.Build(dialect)
			parts = append(parts, expSQL)
			args = append(args, expArgs...)
		default:
			if col, ok := column(item.column); ok {
				parts = append(parts, col)
			}
		}
	}
	return parts, args
}

// quoteColumnName quotes a column name, handling table prefixes.
// Examples: "age" → "age", "users.age" → "users"."age"
func (sq *SelectQuery) quoteColumnName(col string, dialect dialects.Dialect) string {
	return quoteColumn(col, dialect)
}

// buildLimitOffset constructs the LIMIT and OFFSET clauses.
// Returns empty string if neither is set.
func (sq *SelectQuery) buildLimitOffset() string {
	var result string

	if sq.limitValue != nil {
		result += fmt.Sprintf(" LIMIT %d", *sq.limitValue)
	} else if sq.offsetValue != nil {
		// MySQL requires LIMIT before OFFSET; emit max value for compatibility
		result += " LIMIT 9223372036854775807"
	}

	if sq.offsetValue != nil {
		result += fmt.Sprintf(" OFFSET %d", *sq.offsetValue)
	}

	return result
}

// formatSelectColumn formats a single column token for the SELECT clause.
// Aggregates, aliases, and wildcards are passed through as-is; plain identifiers are quoted.
func (sq *SelectQuery) formatSelectColumn(col string, dialect dialects.Dialect) string {
	switch {
	case col == "*":
		return "*"
	case strings.HasSuffix(col, ".*"):
		// "n.*" → "n".*, "schema.table.*" → "schema"."table".*
		tablePart := col[:len(col)-2]
		return quoteColumn(tablePart, dialect) + ".*"
	case strings.Contains(col, "("):
		return col
	default:
		matches := selectAliasRegex.FindStringSubmatch(col)
		if len(matches) > 0 {
			colPart := col[:len(col)-len(matches[0])]
			alias := matches[1]
			return quoteColumn(colPart, dialect) + " AS " + dialect.QuoteIdentifier(alias)
		}
		return sq.quoteColumnName(col, dialect)
	}
}

// buildSelect constructs the SELECT clause, handling aggregate functions and raw expressions.
// renderedSubExprs contains pre-built SQL strings for subExprs entries (dialect-specific,
// with placeholders already renumbered); passed in from buildSQL to keep renumbering logic centralized.
// Returns "*" if no columns, no selectExprs, and no subExprs specified.
// Includes DISTINCT keyword if sq.distinct is true.
func (sq *SelectQuery) buildSelect(dialect dialects.Dialect, renderedSubExprs []string) string {
	parts := make([]string, 0, len(sq.columns)+len(sq.selectExprs)+len(renderedSubExprs))

	for _, col := range sq.columns {
		parts = append(parts, sq.formatSelectColumn(col, dialect))
	}

	for _, expr := range sq.selectExprs {
		parts = append(parts, expr.SQL)
	}

	// Add type-safe subquery expressions (pre-built with dialect quoting and renumbered placeholders)
	for i, sub := range sq.subExprs {
		var sqlFrag string
		if i < len(renderedSubExprs) {
			sqlFrag = renderedSubExprs[i]
		}
		parts = append(parts, "("+sqlFrag+") AS "+dialect.QuoteIdentifier(sub.alias))
	}

	columns := "*"
	if len(parts) > 0 {
		columns = strings.Join(parts, ", ")
	}

	if sq.distinct {
		return "DISTINCT " + columns
	}
	return columns
}

// GroupBy adds GROUP BY clause.
// Multiple columns supported: GroupBy("user_id", "status")
// Chainable: GroupBy, GroupByExpr and GroupBySub append to the same clause
// in call order.
func (sq *SelectQuery) GroupBy(columns ...string) *SelectQuery {
	for _, col := range columns {
		sq.groupBy = append(sq.groupBy, clauseItem{column: col})
	}
	return sq
}

// GroupByExpr adds a raw SQL expression to the GROUP BY clause.
// The expression is used as-is without quoting.
//
// Example:
//
//	GroupByExpr("DATE(created_at)")
//	GroupByExpr("EXTRACT(YEAR FROM order_date)")
func (sq *SelectQuery) GroupByExpr(expr string, args ...any) *SelectQuery {
	sq.groupBy = append(sq.groupBy, clauseItem{raw: &RawExp{SQL: expr, Args: args}})
	return sq
}

// GroupBySub adds a type-safe expression to the GROUP BY clause.
func (sq *SelectQuery) GroupBySub(exp Expression) *SelectQuery {
	if err := ExpressionError(exp); err != nil {
		sq.buildErr = err
		return sq
	}
	sq.groupBy = append(sq.groupBy, clauseItem{exp: exp})
	return sq
}

// buildGroupBy constructs the GROUP BY clause and its parameters.
// Returns empty string if no GROUP BY is specified.
// Quotes column names using dialect.
func (sq *SelectQuery) buildGroupBy(dialect dialects.Dialect) (string, []any) {
	if len(sq.groupBy) == 0 {
		return "", nil
	}

	parts, args := buildClauseItems(sq.groupBy, dialect, func(col string) (string, bool) {
		return sq.quoteColumnName(col, dialect), true
	})
	return " GROUP BY " + strings.Join(parts, ", "), args
}

// Having adds HAVING clause (WHERE for aggregates).
// Accepts string or Expression (same as Where).
// Multiple calls are combined with AND.
//
// String example:
//
//	Having("COUNT(*) > ?", 100)
//
// Expression example:
//
//	Having(relica.GreaterThan("COUNT(*)", 100))
func (sq *SelectQuery) Having(condition any, args ...any) *SelectQuery {
	switch cond := condition.(type) {
	case string:
		// String-based HAVING; {:name} placeholders resolve exactly as in Where.
		resolved, resolvedArgs, err := resolveNamedParams(cond, args)
		if err != nil {
			sq.buildErr = err
			return sq
		}
		sq.havingClauses = append(sq.havingClauses, struct {
			condition string
			args      []any
		}{
			condition: resolved,
			args:      resolvedArgs,
		})

	case Expression:
		if err := ExpressionError(cond); err != nil {
			sq.buildErr = err
			return sq
		}
		sqlStr, exprArgs := cond.Build(sq.builder.db.dialect)
		if sqlStr != "" {
			sq.havingClauses = append(sq.havingClauses, struct {
				condition string
				args      []any
			}{
				condition: sqlStr,
				args:      exprArgs,
			})
		}

	default:
		sq.buildErr = fmt.Errorf("relica: Having() expects string or Expression, got %T", condition)
	}

	return sq
}

// buildHaving constructs the HAVING clause from the havingClauses slice.
// Returns empty string if no HAVING is specified.
// Multiple clauses are combined with AND.
// Appends parameters to params slice.
func (sq *SelectQuery) buildHaving(params *[]any) string {
	if len(sq.havingClauses) == 0 {
		return ""
	}

	parts := make([]string, 0, len(sq.havingClauses))
	for _, clause := range sq.havingClauses {
		parts = append(parts, clause.condition)
		*params = append(*params, clause.args...)
	}

	return " HAVING " + strings.Join(parts, " AND ")
}

// buildWhere constructs the WHERE clause from the where slice.
// Returns empty string if no WHERE is specified.
// Multiple clauses are combined with AND.
// Appends parameters to params slice. Placeholders remain as ? — the single
// replacePlaceholders pass in buildSQL handles final renumbering for PostgreSQL.
func (sq *SelectQuery) buildWhere(_ dialects.Dialect, params *[]any) string {
	if len(sq.where) == 0 {
		return ""
	}

	whereParams := sq.params
	whereClause := " WHERE " + strings.Join(sq.where, " AND ")

	// Append WHERE params (no renumbering here — buildSQL does one final pass)
	*params = append(*params, whereParams...)

	return whereClause
}

// buildWithClause generates the WITH clause for CTEs.
func (sq *SelectQuery) buildWithClause(dialect dialects.Dialect) (string, []any) {
	if len(sq.ctes) == 0 {
		return "", nil
	}

	var parts []string
	var allArgs []any

	// Check if any CTE is recursive
	hasRecursive := false
	for _, cte := range sq.ctes {
		if cte.recursive {
			hasRecursive = true
			break
		}
	}

	// Start WITH clause
	if hasRecursive {
		parts = append(parts, "WITH RECURSIVE")
	} else {
		parts = append(parts, "WITH")
	}

	// Build each CTE using renderSQL so ? placeholders are preserved for the
	// single outer replacePlaceholders pass applied by buildSQL.
	cteStrings := make([]string, 0, len(sq.ctes))
	for _, cte := range sq.ctes {
		cteSQL, cteArgs := cte.query.renderSQL(dialect)

		// Quote CTE name
		quotedName := dialect.QuoteIdentifier(cte.name)

		// Format: cte_name AS (cte_query)
		cteString := quotedName + " AS (" + cteSQL + ")"
		cteStrings = append(cteStrings, cteString)
		allArgs = append(allArgs, cteArgs...)
	}

	// Join CTEs with commas
	parts = append(parts, strings.Join(cteStrings, ", "))

	return strings.Join(parts, " "), allArgs
}

// renderSQL assembles the full SQL string with all placeholders as literal ?
// and collects args in textual order. It intentionally does NOT call
// replacePlaceholders — that single pass is deferred to buildSQL so that
// subqueries embedded in WHERE (IN, EXISTS), GROUP BY expressions, ORDER BY
// expressions, CTEs, and UNION branches all share the same flat ? sequence
// and are renumbered exactly once at the outermost level.
//
// Parameter ordering follows the statement text: CTEs → SelectExprs → SubExprs →
// FROM subquery → JOINs → WHERE → GROUP BY → HAVING → ORDER BY → UNION branches.
func (sq *SelectQuery) renderSQL(dialect dialects.Dialect) (string, []any) {
	// Collect all parameters in correct order
	var allParams []any
	var parts []string

	// 1. Build WITH clause if CTEs exist (uses renderSQL recursively)
	if len(sq.ctes) > 0 {
		withClause, withArgs := sq.buildWithClause(dialect)
		parts = append(parts, withClause)
		allParams = append(allParams, withArgs...)
	}

	// 2. Add params from SelectExpr (raw scalar subqueries in SELECT clause)
	for _, expr := range sq.selectExprs {
		allParams = append(allParams, expr.Args...)
	}

	// 3. Build type-safe subquery SELECT expressions (SelectSub).
	// Expressions are rendered with renderSQL so their ? placeholders remain
	// raw and are renumbered by the single final pass in buildSQL.
	renderedSubExprs := make([]string, len(sq.subExprs))
	for i, sub := range sq.subExprs {
		subSQL, subArgs := sub.exp.Build(dialect)
		renderedSubExprs[i] = subSQL
		allParams = append(allParams, subArgs...)
	}

	// 4. Build SELECT clause (handles aggregates, raw expressions, and subExprs)
	cols := sq.buildSelect(dialect, renderedSubExprs)

	// 5. Build FROM clause (may be table or subquery; uses renderSQL for subqueries)
	fromClause := sq.buildFrom(dialect, &allParams)

	// 6. Build JOIN clause (adds params via pointer)
	joinClause := sq.buildJoins(dialect, &allParams)

	// 7. Build WHERE clause (appends params; no renumbering — deferred to buildSQL)
	whereClause := sq.buildWhere(dialect, &allParams)

	// 8. Build GROUP BY clause. Its params go before HAVING's: GROUP BY comes
	// first in the statement, whatever order GroupBy* and Having were called in.
	groupByClause, groupByArgs := sq.buildGroupBy(dialect)
	allParams = append(allParams, groupByArgs...)

	// 9. Build HAVING clause (adds params via pointer)
	havingClause := sq.buildHaving(&allParams)

	// 10. Build ORDER BY clause
	orderByClause, orderByArgs := sq.buildOrderBy(dialect)
	allParams = append(allParams, orderByArgs...)

	// 11. Build LIMIT/OFFSET clause
	limitOffsetClause := sq.buildLimitOffset()

	// 12. Build lock clause (FOR UPDATE/FOR SHARE — skip for SQLite)
	lockClause := ""
	if sq.lockClause != "" && sq.builder != nil {
		if _, isSQLite := sq.builder.db.dialect.(*dialects.SQLiteDialect); !isSQLite {
			lockClause = " " + sq.lockClause
		}
	}

	// Construct SQL: SELECT ... FROM ... JOIN ... WHERE ... GROUP BY ... HAVING ... ORDER BY ... LIMIT ... OFFSET ... FOR UPDATE
	query := "SELECT " + cols + fromClause + joinClause + whereClause + groupByClause + havingClause + orderByClause + limitOffsetClause + lockClause

	// 13. Handle set operations (UNION, INTERSECT, EXCEPT) — uses renderSQL recursively
	if len(sq.unions) > 0 {
		mainSQL, finalParams := sq.buildSetOperations(query, allParams, dialect)
		// Prepend WITH clause if exists
		if len(parts) > 0 {
			mainSQL = strings.Join(parts, " ") + " " + mainSQL
		}
		return mainSQL, finalParams
	}

	// Prepend WITH clause if exists
	if len(parts) > 0 {
		query = strings.Join(parts, " ") + " " + query
	}

	return query, allParams
}

// buildSQL constructs the final SQL string and parameters for SelectQuery.
// It calls renderSQL to assemble the full query with all placeholders as ?
// then applies a single replacePlaceholders pass to renumber them for the
// target dialect (e.g. $1, $2, $3 for PostgreSQL). This single-pass approach
// ensures correct sequential numbering across WHERE, GROUP BY, ORDER BY,
// subqueries, CTEs, and UNION branches without any intermediate renumbering.
func (sq *SelectQuery) buildSQL(dialect dialects.Dialect) (string, []any) {
	rawSQL, args := sq.renderSQL(dialect)
	finalSQL, count := replacePlaceholders(rawSQL, 1, dialect)
	if count != len(args) {
		sq.buildErr = fmt.Errorf("relica: %d placeholders but %d args", count, len(args))
	}
	return finalSQL, args
}

// buildSetOperations handles UNION, INTERSECT, EXCEPT operations.
// This method is extracted from renderSQL to reduce cognitive complexity.
// Each branch is rendered via renderSQL so all ? placeholders are preserved
// for the single final replacePlaceholders pass applied by buildSQL.
func (sq *SelectQuery) buildSetOperations(mainQuery string, allParams []any, dialect dialects.Dialect) (string, []any) {
	_, sqlite := dialect.(*dialects.SQLiteDialect)
	// The first operand's CTEs are rendered in front of the whole statement
	// by renderSQL, and its set operations are this compound itself.
	mainSQL := setOperand(mainQuery, sqlite, sq.hasRowLimits())

	for _, u := range sq.unions {
		// Build union query SQL with raw ? placeholders (no renumbering here)
		unionSQL, unionArgs := u.query.renderSQL(dialect)

		// Determine operation keyword
		op := u.op
		if op == "" {
			op = "UNION"
		}
		if u.all && op == "UNION" {
			op = "UNION ALL"
		}

		branch := u.query
		mainSQL += " " + op + " " + setOperand(unionSQL, sqlite,
			branch.hasRowLimits() || len(branch.ctes) > 0 || len(branch.unions) > 0)

		// Merge parameters in order
		allParams = append(allParams, unionArgs...)
	}

	return mainSQL, allParams
}

// errQueryCycle reports a query that contains itself, for example
// q.Union(q): rendering it would never end.
var errQueryCycle = errors.New("relica: a query cannot contain itself (FromSelect, set operation or CTE)")

// err returns the first construction error of the query or of a query it
// embeds: the FromSelect subquery, the set-operation branches and the CTEs.
// Each of them is rendered into this statement, so its error is this
// statement's error. Subqueries passed as expressions (In, Exists, SelectSub)
// are checked when they are added, by ExpressionError.
func (sq *SelectQuery) err() error {
	return sq.errVisiting(map[*SelectQuery]bool{})
}

func (sq *SelectQuery) errVisiting(path map[*SelectQuery]bool) error {
	if sq.buildErr != nil {
		return sq.buildErr
	}
	if path[sq] {
		return errQueryCycle
	}
	path[sq] = true
	defer delete(path, sq)

	if sq.fromSrc != nil && sq.fromSrc.subquery != nil {
		if err := sq.fromSrc.subquery.errVisiting(path); err != nil {
			return err
		}
	}
	for _, u := range sq.unions {
		if err := u.query.errVisiting(path); err != nil {
			return err
		}
	}
	for _, c := range sq.ctes {
		if err := c.query.errVisiting(path); err != nil {
			return err
		}
	}
	return nil
}

// setOperand renders one operand of UNION/INTERSECT/EXCEPT. PostgreSQL and
// MySQL take every operand in parentheses, which keeps its ORDER BY/LIMIT and
// its own set operations to itself. SQLite's compound-select grammar takes no
// parentheses: a plain operand is written bare, and one that needs its own
// ORDER BY, LIMIT, OFFSET, WITH or set operations (contained) becomes the
// subquery operand SELECT * FROM (...), since bare it would bind to the whole
// compound or regroup it left to right.
func setOperand(sqlStr string, sqlite, contained bool) string {
	switch {
	case !sqlite:
		return "(" + sqlStr + ")"
	case contained:
		return "SELECT * FROM (" + sqlStr + ")"
	default:
		return sqlStr
	}
}

// hasRowLimits reports whether the query has ORDER BY, LIMIT or OFFSET.
func (sq *SelectQuery) hasRowLimits() bool {
	return len(sq.orderBy) > 0 || sq.limitValue != nil || sq.offsetValue != nil
}

// Build constructs the Query object from SelectQuery.
// If a programming error was stored during query construction (e.g., wrong type
// passed to Where), the error is propagated through the Query and returned by
// Execute/All/One/Row at call time instead of panicking.
func (sq *SelectQuery) Build() *Query {
	// Context priority: query ctx > builder ctx > nil
	ctx := sq.ctx
	if ctx == nil {
		ctx = sq.builder.ctx
	}

	if sq.buildErr == nil {
		sq.buildErr = sq.err()
	}
	if sq.buildErr == nil {
		sq.buildErr = sq.lockError()
	}

	// Return a Query that carries the build error; all execution methods check prepErr.
	if sq.buildErr != nil {
		return &Query{
			prepErr: sq.buildErr,
			db:      sq.builder.db,
			tx:      sq.builder.tx,
			ctx:     ctx,
		}
	}

	query, allParams := sq.buildSQL(sq.builder.db.dialect)

	// buildJoins / buildSQL may set buildErr when encountering bad JOIN ON type.
	if sq.buildErr != nil {
		return &Query{
			prepErr: sq.buildErr,
			db:      sq.builder.db,
			tx:      sq.builder.tx,
			ctx:     ctx,
		}
	}

	return &Query{
		sql:           query,
		params:        allParams,
		db:            sq.builder.db,
		tx:            sq.builder.tx,
		ctx:           ctx,
		scanMode:      sq.scanMode,
		listedColumns: sq.listsColumns(),
	}
}

// ScanMode overrides the DB's scan mode for this query (ADR-018).
func (sq *SelectQuery) ScanMode(m ScanMode) *SelectQuery {
	sq.scanMode = &m
	return sq
}

// listsColumns reports whether the SELECT names its columns, so a struct
// scan requires only those. "*" and "t.*" are not a list.
func (sq *SelectQuery) listsColumns() bool {
	if len(sq.columns)+len(sq.selectExprs)+len(sq.subExprs) == 0 {
		return false
	}
	for _, c := range sq.columns {
		if c = strings.TrimSpace(c); c == "*" || strings.HasSuffix(c, ".*") {
			return false
		}
	}
	for _, e := range sq.selectExprs {
		if s := strings.TrimSpace(e.SQL); s == "*" || strings.HasSuffix(s, ".*") {
			return false
		}
	}
	return true
}

// validateModelDest validates dest is a non-nil struct pointer and returns the dereferenced Value.
func validateModelDest(dest any) (reflect.Value, error) {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}, errors.New("model: dest must be a non-nil pointer to struct")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, errors.New("model: dest must be a pointer to struct")
	}
	return v, nil
}

// extractNonZeroPK extracts PK info from struct and validates at least one PK field is non-zero.
func (sq *SelectQuery) extractNonZeroPK(v reflect.Value) (*util.PrimaryKeyInfo, error) {
	pkInfo, err := sq.builder.db.primaryKey(v)
	if err != nil {
		return nil, err
	}
	for _, val := range pkInfo.Values {
		if !util.IsPrimaryKeyZero(val) {
			return pkInfo, nil
		}
	}
	return nil, errors.New("model: primary key not set (all PK fields are zero)")
}

// Model reads PK values from dest struct fields, auto-detects table (if From not set),
// appends PK WHERE conditions, and scans a single row into dest.
// Terminal method — returns error, not *SelectQuery.
// Returns error if all PK fields are zero or struct has no PK.
func (sq *SelectQuery) Model(dest any) error {
	v, err := validateModelDest(dest)
	if err != nil {
		return err
	}

	if sq.table == "" && sq.fromSrc == nil {
		tableName, err := sq.builder.db.tableNameOf(dest)
		if err != nil {
			return err
		}
		sq.From(tableName)
	}

	pkInfo, err := sq.extractNonZeroPK(v)
	if err != nil {
		return err
	}

	for i, col := range pkInfo.Columns {
		sq.Where(Eq(col, pkInfo.Values[i].Interface()))
	}

	return sq.Limit(1).One(dest)
}

// One scans a single row into dest.
func (sq *SelectQuery) One(dest any) error {
	return sq.Build().One(dest)
}

// All scans all rows into dest slice.
func (sq *SelectQuery) All(dest any) error {
	return sq.Build().All(dest)
}

// Row scans a single row into individual variables.
// Returns sql.ErrNoRows if no rows are found.
//
// Example:
//
//	var name string
//	var age int
//	err := db.Select("name", "age").From("users").Where("id = ?", 1).Row(&name, &age)
func (sq *SelectQuery) Row(dest ...any) error {
	return sq.Build().Row(dest...)
}

// Column scans the first column of all rows into a slice.
// The slice parameter must be a pointer to a slice of the appropriate type.
//
// Example:
//
//	var ids []int
//	err := db.Select("id").From("users").Where("status = ?", "active").Column(&ids)
func (sq *SelectQuery) Column(slice any) error {
	return sq.Build().Column(slice)
}

// Count returns the number of rows the query would return: with GROUP BY the
// number of groups, with DISTINCT the number of distinct rows, with a set
// operation the rows of the compound result. ORDER BY, LIMIT and OFFSET are
// ignored, so a paginated query reports its total; in a set operation they
// belong to the first branch and are kept. A query whose select list is plain
// columns is sent as SELECT COUNT(*) with its FROM, JOIN and WHERE; one with
// GROUP BY, HAVING, DISTINCT, a set operation or an expression in the select
// list (which may aggregate) is counted as a derived table.
//
// Example:
//
//	count, err := db.Select().From("users").Where(relica.Eq("status", 1)).Count()
func (sq *SelectQuery) Count() (int64, error) {
	if err := sq.err(); err != nil {
		return 0, err
	}
	if sq.locksRows() {
		return 0, errLockWithAggregate
	}
	sqlStr, params := sq.countStatement()
	if sq.buildErr != nil {
		return 0, sq.buildErr
	}
	var count int64
	if err := sq.aggregateQuery(sqlStr, params).Row(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Exists reports whether the query returns at least one row, by the same rule
// as Count: GROUP BY, HAVING, DISTINCT, set operations and CTEs apply; ORDER BY,
// LIMIT and OFFSET are ignored. The query is sent as SELECT EXISTS(...).
//
// Example:
//
//	exists, err := db.Select().From("users").Where(relica.Eq("email", email)).Exists()
func (sq *SelectQuery) Exists() (bool, error) {
	if err := sq.err(); err != nil {
		return false, err
	}
	if sq.locksRows() {
		return false, errLockWithAggregate
	}
	sqlStr, params := sq.existsStatement()
	if sq.buildErr != nil {
		return false, sq.buildErr
	}
	var exists bool
	if err := sq.aggregateQuery(sqlStr, params).Row(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// countAlias names the derived table Count wraps a grouped, distinct or
// compound query in; PostgreSQL and MySQL require an alias there.
const countAlias = "relica_count"

// countStatement renders the SQL Count sends (see Count).
func (sq *SelectQuery) countStatement() (string, []any) {
	src := sq.aggregateSource()
	if len(src.groupBy) > 0 || len(src.havingClauses) > 0 || src.distinct || len(src.unions) > 0 ||
		src.selectListHasExpr() {
		inner, args := src.renderSQL(sq.builder.db.dialect)
		sq.keepRenderErr(src)
		return sq.withAggregate("SELECT COUNT(*) FROM ("+inner+") AS "+countAlias, args)
	}
	// A list of plain columns does not change the number of rows: replace it,
	// with no parameters to drop.
	src.columns = []string{"COUNT(*)"}
	body, args := src.renderSQL(sq.builder.db.dialect)
	sq.keepRenderErr(src)
	return sq.withAggregate(body, args)
}

// selectListHasExpr reports whether the select list holds anything besides
// plain column references: a SelectExpr, a SelectSub, or a Select argument
// with "(" (a function call). Such a list may aggregate — MAX(x) without
// GROUP BY returns one row — so Count cannot replace it with COUNT(*); telling
// aggregates from other expressions by their text would be a guess.
func (sq *SelectQuery) selectListHasExpr() bool {
	if len(sq.selectExprs) > 0 || len(sq.subExprs) > 0 {
		return true
	}
	for _, c := range sq.columns {
		if strings.Contains(c, "(") {
			return true
		}
	}
	return false
}

// existsStatement renders the SQL Exists sends (see Exists).
func (sq *SelectQuery) existsStatement() (string, []any) {
	src := sq.aggregateSource()
	inner, args := src.renderSQL(sq.builder.db.dialect)
	sq.keepRenderErr(src)
	return sq.withAggregate("SELECT EXISTS("+inner+")", args)
}

// aggregateSource is the query whose rows Count and Exists evaluate: a copy
// without CTEs (withAggregate puts them in front of the whole statement) and,
// unless it is a set operation, without ORDER BY, LIMIT and OFFSET. In a set
// operation those belong to the first branch, so they stay.
func (sq *SelectQuery) aggregateSource() *SelectQuery {
	src := *sq
	src.ctes = nil
	if len(src.unions) == 0 {
		src.orderBy = nil
		src.limitValue = nil
		src.offsetValue = nil
	}
	return &src
}

// keepRenderErr keeps an error renderSQL stored on the copy (an unsupported
// JOIN ON type, for example).
func (sq *SelectQuery) keepRenderErr(src *SelectQuery) {
	if sq.buildErr == nil {
		sq.buildErr = src.buildErr
	}
}

// withAggregate prepends the query's CTEs to body and renumbers the
// placeholders once for the dialect. A placeholder/argument mismatch is stored
// in sq.buildErr, as buildSQL does.
func (sq *SelectQuery) withAggregate(body string, bodyArgs []any) (string, []any) {
	dialect := sq.builder.db.dialect
	args := bodyArgs
	if len(sq.ctes) > 0 {
		withClause, withArgs := sq.buildWithClause(dialect)
		body = withClause + " " + body
		args = append(append([]any{}, withArgs...), bodyArgs...)
	}
	finalSQL, count := replacePlaceholders(body, 1, dialect)
	if count != len(args) {
		sq.buildErr = fmt.Errorf("relica: %d placeholders but %d args", count, len(args))
	}
	return finalSQL, args
}

// aggregateQuery wraps a rendered Count/Exists statement in a Query bound to
// the query's DB or transaction and context.
func (sq *SelectQuery) aggregateQuery(sqlStr string, params []any) *Query {
	ctx := sq.ctx
	if ctx == nil {
		ctx = sq.builder.ctx
	}
	return &Query{
		sql:    sqlStr,
		params: params,
		db:     sq.builder.db,
		tx:     sq.builder.tx,
		ctx:    ctx,
	}
}

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
// A query with a construction error (a bad Where, a nil branch, an OrderBy
// that is not a column, placeholders that do not match the args) returns ""
// and nil, as UpdateQuery and DeleteQuery do; running it returns the error.
// Rules about where a statement may run (row locks need a transaction) are
// checked when it runs, not here.
//
// Example:
//
//	sql, params := db.Select().From("users").Where(relica.Eq("id", 1)).ToSQL()
func (sq *SelectQuery) ToSQL() (string, []any) {
	if sq.err() != nil {
		return "", nil
	}
	sqlStr, params := sq.buildSQL(sq.builder.db.dialect)
	if sq.buildErr != nil {
		return "", nil
	}
	return sqlStr, params
}

// Explain analyzes the query execution plan without executing the query.
// Returns QueryPlan with estimated metrics (cost, rows, index usage).
// Currently only supported for PostgreSQL databases.
//
// Example:
//
//	plan, err := db.Builder().
//	    Select("*").
//	    From("users").
//	    Where("email = ?", "alice@example.com").
//	    Explain()
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Cost: %.2f, Rows: %d, Uses Index: %v\n",
//	    plan.Cost, plan.EstimatedRows, plan.UsesIndex)
func (sq *SelectQuery) Explain() (*QueryPlan, error) {
	return sq.explainQuery(false)
}

// ExplainAnalyze analyzes the query execution plan AND executes the query.
// Returns QueryPlan with both estimated and actual metrics.
// Currently only supported for PostgreSQL databases.
//
// WARNING: This method ACTUALLY EXECUTES the query, including any side effects
// (INSERT, UPDATE, DELETE in CTEs, triggers, etc.). Use with caution.
//
// Example:
//
//	plan, err := db.Builder().
//	    Select("*").
//	    From("users").
//	    Where("status = ?", 1).
//	    ExplainAnalyze()
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Actual Time: %v, Actual Rows: %d, Buffers Hit: %d\n",
//	    plan.ActualTime, plan.ActualRows, plan.BuffersHit)
func (sq *SelectQuery) ExplainAnalyze() (*QueryPlan, error) {
	return sq.explainQuery(true)
}

// explainQuery implements query analysis using database EXPLAIN functionality.
func (sq *SelectQuery) explainQuery(withAnalyze bool) (*QueryPlan, error) {
	if err := sq.err(); err != nil {
		return nil, err
	}
	// Build the SELECT query
	sqlQuery, params := sq.buildSQL(sq.builder.db.dialect)
	if sq.buildErr != nil {
		return nil, sq.buildErr
	}

	// Context priority: query ctx > builder ctx > background
	ctx := sq.ctx
	if ctx == nil {
		ctx = sq.builder.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Create analyzer based on database driver
	queryAnalyzer, err := newAnalyzerForDB(sq.builder.db)
	if err != nil {
		return nil, err
	}

	// Execute EXPLAIN
	if withAnalyze {
		return queryAnalyzer.ExplainAnalyze(ctx, sqlQuery, params)
	}
	return queryAnalyzer.Explain(ctx, sqlQuery, params)
}

// selectQueryExpression is an adapter that allows SelectQuery to implement the Expression interface.
// This is necessary because SelectQuery.Build() returns *Query, not (string, []any).
type selectQueryExpression struct {
	query *SelectQuery
}

// Build implements the Expression interface for SelectQuery.
// This allows SelectQuery to be used in subquery contexts (IN, EXISTS, FROM).
// Uses renderSQL so ? placeholders are preserved for the single final
// replacePlaceholders pass applied by the outermost buildSQL call.
func (sqe *selectQueryExpression) Build(dialect dialects.Dialect) (string, []any) {
	return sqe.query.renderSQL(dialect)
}

// AsExpression converts a SelectQuery to an Expression, allowing it to be used as a subquery.
// This is useful when embedding a SelectQuery in WHERE clauses like IN or EXISTS.
//
// Example:
//
//	sub := db.Builder().Select("user_id").From("orders").Where("total > ?", 100)
//	db.Builder().Select("*").From("users").Where(In("id", sub.AsExpression())).All(&users)
//
// Note: The public relica.SelectQuery wrapper does NOT implement Expression directly.
// Always call AsExpression() when passing a SelectQuery to In(), Exists(), or other expressions.
func (sq *SelectQuery) AsExpression() Expression {
	return &selectQueryExpression{query: sq}
}

// Select starts a SELECT query.
func (qb *QueryBuilder) Select(cols ...string) *SelectQuery {
	return &SelectQuery{
		builder: qb,
		columns: cols,
	}
}

// Insert builds an INSERT query.
// Returns a Query with a prepErr if values is nil or empty — INSERT with no columns
// produces invalid SQL, so the error is surfaced at execution time without panicking.
func (qb *QueryBuilder) Insert(table string, values map[string]any) *Query {
	if len(values) == 0 {
		return &Query{
			prepErr: fmt.Errorf("relica: Insert requires a non-empty values map"),
			db:      qb.db,
			tx:      qb.tx,
			ctx:     qb.ctx,
		}
	}

	// Get sorted keys for deterministic SQL generation (prevents cache misses)
	keys := getKeys(values)

	placeholders := make([]string, 0, len(keys))
	params := make([]any, 0, len(keys))

	for i, col := range keys {
		placeholders = append(placeholders, qb.db.dialect.Placeholder(i+1))
		params = append(params, values[col])
	}

	quotedKeys := make([]string, len(keys))
	for i, k := range keys {
		quotedKeys[i] = qb.db.dialect.QuoteIdentifier(k)
	}

	query := `INSERT INTO ` + quoteTableName(table, qb.db.dialect) +
		` (` + strings.Join(quotedKeys, ", ") + `) ` +
		`VALUES (` + strings.Join(placeholders, ", ") + `)`

	return &Query{
		sql:    query,
		params: params,
		db:     qb.db,
		tx:     qb.tx,
		ctx:    qb.ctx,
	}
}

// getKeys returns sorted map keys for deterministic SQL generation.
func getKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// UpsertQuery represents an UPSERT (INSERT ... ON CONFLICT/DUPLICATE) query.
type UpsertQuery struct {
	builder  *QueryBuilder
	table    string
	values   map[string]any
	conflict conflictClause
	ctx      context.Context // context for this specific query
}

// WithContext sets the context for this UPSERT query.
// This overrides any context set on the QueryBuilder.
func (uq *UpsertQuery) WithContext(ctx context.Context) *UpsertQuery {
	uq.ctx = ctx
	return uq
}

// Upsert creates an UPSERT query for the given table and values.
// It needs exactly one conflict action: DoUpdate, DoUpdateAll or DoNothing (ADR-019).
func (qb *QueryBuilder) Upsert(table string, values map[string]any) *UpsertQuery {
	return &UpsertQuery{
		builder: qb,
		table:   table,
		values:  values,
	}
}

// OnConflict sets the conflict target. Required for DoUpdate/DoUpdateAll on
// PostgreSQL and SQLite; advisory on MySQL, which fires on any unique key (the
// target columns are still left out of DoUpdateAll there).
func (uq *UpsertQuery) OnConflict(columns ...string) *UpsertQuery {
	uq.conflict.onConflict(columns)
	return uq
}

// DoUpdate sets the listed columns from the proposed row on conflict. Each
// column must be among the inserted ones.
func (uq *UpsertQuery) DoUpdate(columns ...string) *UpsertQuery {
	uq.conflict.setAction(upsertDoUpdate, columns)
	return uq
}

// DoUpdateAll sets every inserted column except the conflict target from the
// proposed row on conflict.
func (uq *UpsertQuery) DoUpdateAll() *UpsertQuery {
	uq.conflict.setAction(upsertDoUpdateAll, nil)
	return uq
}

// DoNothing keeps the existing row on conflict: ON CONFLICT DO NOTHING on
// PostgreSQL/SQLite; on MySQL a self-assignment of the first conflict column (or
// the first inserted column) in ON DUPLICATE KEY UPDATE.
func (uq *UpsertQuery) DoNothing() *UpsertQuery {
	uq.conflict.setAction(upsertDoNothing, nil)
	return uq
}

// Build constructs the Query object from UpsertQuery. An invalid upsert
// returns a Query that carries the error; nothing is sent.
func (uq *UpsertQuery) Build() *Query {
	// Context priority: query ctx > builder ctx > nil
	ctx := uq.ctx
	if ctx == nil {
		ctx = uq.builder.ctx
	}
	if len(uq.values) == 0 {
		return &Query{prepErr: errors.New("relica: Upsert requires a non-empty values map"),
			db: uq.builder.db, tx: uq.builder.tx, ctx: ctx}
	}

	keys := getKeys(uq.values)
	conflictSQL, err := uq.conflict.render(uq.builder.db, keys)
	if err != nil {
		return &Query{prepErr: err, db: uq.builder.db, tx: uq.builder.tx, ctx: ctx}
	}

	placeholders := make([]string, 0, len(keys))
	params := make([]any, 0, len(keys))

	for i, col := range keys {
		placeholders = append(placeholders, uq.builder.db.dialect.Placeholder(i+1))
		params = append(params, uq.values[col])
	}

	quotedKeys := make([]string, len(keys))
	for i, k := range keys {
		quotedKeys[i] = uq.builder.db.dialect.QuoteIdentifier(k)
	}

	// Build base INSERT statement
	query := `INSERT INTO ` + quoteTableName(uq.table, uq.builder.db.dialect) +
		` (` + strings.Join(quotedKeys, ", ") + `) ` +
		`VALUES (` + strings.Join(placeholders, ", ") + `)` +
		conflictSQL

	return &Query{
		sql:    query,
		params: params,
		db:     uq.builder.db,
		tx:     uq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the UPSERT query and returns the result.
func (uq *UpsertQuery) Execute() (any, error) {
	return uq.Build().Execute()
}

// filterKeys returns keys that are not in the exclude list.
func filterKeys(keys, exclude []string) []string {
	excludeMap := make(map[string]bool)
	for _, e := range exclude {
		excludeMap[e] = true
	}

	filtered := make([]string, 0, len(keys))
	for _, k := range keys {
		if !excludeMap[k] {
			filtered = append(filtered, k)
		}
	}
	return filtered
}

// UpdateQuery represents an UPDATE query being built.
type UpdateQuery struct {
	builder  *QueryBuilder
	table    string
	values   map[string]any
	where    []string
	params   []any
	ctx      context.Context // context for this specific query
	buildErr error           // stored programming error (replaces panic in fluent chain)

	whereCalled bool // Where/AndWhere/OrWhere was called (ADR-020)
	allRows     bool // AllRows(): the statement may touch every row
}

// WithContext sets the context for this UPDATE query.
// This overrides any context set on the QueryBuilder.
func (uq *UpdateQuery) WithContext(ctx context.Context) *UpdateQuery {
	uq.ctx = ctx
	return uq
}

// Update creates an UPDATE query for the specified table.
func (qb *QueryBuilder) Update(table string) *UpdateQuery {
	return &UpdateQuery{
		builder: qb,
		table:   table,
	}
}

// Set specifies the columns and values to update.
// Values should be a map of column names to new values.
func (uq *UpdateQuery) Set(values map[string]any) *UpdateQuery {
	for _, v := range values {
		if err := ExpressionError(v); err != nil {
			uq.buildErr = err
			return uq
		}
	}
	uq.values = values
	return uq
}

// Where adds a WHERE condition to the UPDATE query.
// Accepts either a string with placeholders or an Expression.
// Multiple Where calls are combined with AND.
//
// String example:
//
//	Where("status = ?", 1)
//
// Expression example:
//
//	Where(relica.Eq("status", 1))
func (uq *UpdateQuery) Where(condition any, params ...any) *UpdateQuery {
	uq.whereCalled = true
	switch cond := condition.(type) {
	case string:
		if strings.TrimSpace(cond) == "" {
			uq.setBuildErr(errEmptyCondition)
			return uq
		}
		resolved, resolvedArgs, err := resolveNamedParams(cond, params)
		if err != nil {
			uq.buildErr = err
			return uq
		}
		uq.where = append(uq.where, resolved)
		uq.params = append(uq.params, resolvedArgs...)

	case Expression:
		if err := ExpressionError(cond); err != nil {
			uq.buildErr = err
			return uq
		}
		sqlStr, args := cond.Build(uq.builder.db.dialect)
		if sqlStr == "" {
			uq.setBuildErr(errEmptyCondition)
			return uq
		}
		uq.where = append(uq.where, sqlStr)
		uq.params = append(uq.params, args...)

	default:
		uq.buildErr = fmt.Errorf("relica: Where() expects string or Expression, got %T", condition)
	}

	return uq
}

// AndWhere adds a WHERE condition with AND logic.
// If no existing WHERE clause exists, behaves like Where().
// Multiple conditions are combined with AND.
//
// String example:
//
//	AndWhere("age > ?", 18)
//
// Expression example:
//
//	AndWhere(relica.GreaterThan("age", 18))
func (uq *UpdateQuery) AndWhere(condition any, params ...any) *UpdateQuery {
	// Simply delegate to Where() which already uses AND logic for multiple calls.
	return uq.Where(condition, params...)
}

// OrWhere adds a WHERE condition with OR logic.
// If no existing WHERE clause exists, behaves like Where().
// The new condition is combined with existing conditions using OR.
//
// String example:
//
//	OrWhere("status = ?", "admin")
//
// Expression example:
//
//	OrWhere(relica.Eq("status", "admin"))
//
//nolint:dupl // OrWhere on SelectQuery/UpdateQuery/DeleteQuery share structure but operate on different receiver types; a generic helper would require any gymnastics.
func (uq *UpdateQuery) OrWhere(condition any, params ...any) *UpdateQuery {
	if len(uq.where) == 0 {
		// No existing WHERE clause - just add it.
		return uq.Where(condition, params...)
	}

	// Build the new condition.
	var newSQL string
	var newArgs []any

	switch cond := condition.(type) {
	case string:
		var err error
		if strings.TrimSpace(cond) == "" {
			uq.setBuildErr(errEmptyCondition)
			return uq
		}
		newSQL, newArgs, err = resolveNamedParams(cond, params)
		if err != nil {
			uq.buildErr = err
			return uq
		}

	case Expression:
		if err := ExpressionError(cond); err != nil {
			uq.buildErr = err
			return uq
		}
		newSQL, newArgs = cond.Build(uq.builder.db.dialect)
		if newSQL == "" {
			uq.setBuildErr(errEmptyCondition)
			return uq
		}

	default:
		uq.buildErr = fmt.Errorf("relica: OrWhere() expects string or Expression, got %T", condition)
		return uq
	}

	// Combine existing WHERE with new condition using OR.
	// Wrap both sides in parentheses for correct precedence.
	existingWhere := strings.Join(uq.where, " AND ")
	combined := "(" + existingWhere + ") OR (" + newSQL + ")"

	// Replace WHERE clauses with combined condition.
	uq.where = []string{combined}
	uq.params = append(uq.params, newArgs...)

	return uq
}

// Build constructs the Query object from UpdateQuery.
// If a programming error was stored during query construction, it is propagated
// through the Query and returned by Execute at call time instead of panicking.
func (uq *UpdateQuery) Build() *Query {
	// Context priority: query ctx > builder ctx > nil
	ctx := uq.ctx
	if ctx == nil {
		ctx = uq.builder.ctx
	}

	if uq.buildErr == nil {
		uq.buildErr = wholeTableError(uq.whereCalled, uq.allRows)
	}
	if uq.buildErr != nil {
		return &Query{
			prepErr: uq.buildErr,
			db:      uq.builder.db,
			tx:      uq.builder.tx,
			ctx:     ctx,
		}
	}

	// UPDATE with no values produces invalid SQL ("UPDATE t SET WHERE ...").
	// Return a clean error rather than a malformed query.
	if len(uq.values) == 0 {
		return &Query{
			prepErr: fmt.Errorf("relica: Update requires values, call Set() before Build()"),
			db:      uq.builder.db,
			tx:      uq.builder.tx,
			ctx:     ctx,
		}
	}

	// Get sorted keys for deterministic SQL generation
	keys := getKeys(uq.values)

	// Build SET clause with placeholders
	setClauses := make([]string, 0, len(keys))
	setParams := make([]any, 0, len(keys))

	for _, col := range keys {
		val := uq.values[col]
		quotedCol := uq.builder.db.dialect.QuoteIdentifier(col)
		if expr, ok := val.(Expression); ok {
			exprSQL, exprArgs := expr.Build(uq.builder.db.dialect)
			setClauses = append(setClauses, quotedCol+" = "+exprSQL)
			setParams = append(setParams, exprArgs...)
		} else {
			setClauses = append(setClauses, quotedCol+" = ?")
			setParams = append(setParams, val)
		}
	}

	// Build WHERE clause (keep ? — single-pass renumbering below)
	whereClause := ""
	whereParams := uq.params
	if len(uq.where) > 0 {
		whereClause = " WHERE " + strings.Join(uq.where, " AND ")
	}

	// Construct raw SQL with ? placeholders
	rawSQL := "UPDATE " + quoteTableName(uq.table, uq.builder.db.dialect) +
		" SET " + strings.Join(setClauses, ", ") + whereClause

	// Single-pass renumbering for PostgreSQL
	allParams := make([]any, 0, len(setParams)+len(whereParams))
	allParams = append(allParams, setParams...)
	allParams = append(allParams, whereParams...)
	query, count := replacePlaceholders(rawSQL, 1, uq.builder.db.dialect)
	if count != len(allParams) {
		return &Query{
			prepErr: fmt.Errorf("relica: %d placeholders but %d args", count, len(allParams)),
			db:      uq.builder.db,
			tx:      uq.builder.tx,
			ctx:     ctx,
		}
	}

	return &Query{
		sql:    query,
		params: allParams,
		db:     uq.builder.db,
		tx:     uq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the UPDATE query and returns the result.
func (uq *UpdateQuery) Execute() (any, error) {
	return uq.Build().Execute()
}

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
//
// Example:
//
//	sql, params := db.Update("users").Set(map[string]any{"status": 1}).Where(relica.Eq("id", 1)).ToSQL()
func (uq *UpdateQuery) ToSQL() (string, []any) {
	q := uq.Build()
	return q.sql, q.params
}

// DeleteQuery represents a DELETE query being built.
type DeleteQuery struct {
	builder  *QueryBuilder
	table    string
	where    []string
	params   []any
	ctx      context.Context // context for this specific query
	buildErr error           // stored programming error (replaces panic in fluent chain)

	whereCalled bool // Where/AndWhere/OrWhere was called (ADR-020)
	allRows     bool // AllRows(): the statement may touch every row
}

// WithContext sets the context for this DELETE query.
// This overrides any context set on the QueryBuilder.
func (dq *DeleteQuery) WithContext(ctx context.Context) *DeleteQuery {
	dq.ctx = ctx
	return dq
}

// Delete creates a DELETE query for the specified table.
func (qb *QueryBuilder) Delete(table string) *DeleteQuery {
	return &DeleteQuery{
		builder: qb,
		table:   table,
	}
}

// Where adds a WHERE condition to the DELETE query.
// Accepts either a string with placeholders or an Expression.
// Multiple Where calls are combined with AND.
//
// String example:
//
//	Where("id = ?", 123)
//
// Expression example:
//
//	Where(relica.Eq("id", 123))
func (dq *DeleteQuery) Where(condition any, params ...any) *DeleteQuery {
	dq.whereCalled = true
	switch cond := condition.(type) {
	case string:
		if strings.TrimSpace(cond) == "" {
			dq.setBuildErr(errEmptyCondition)
			return dq
		}
		resolved, resolvedArgs, err := resolveNamedParams(cond, params)
		if err != nil {
			dq.buildErr = err
			return dq
		}
		dq.where = append(dq.where, resolved)
		dq.params = append(dq.params, resolvedArgs...)

	case Expression:
		if err := ExpressionError(cond); err != nil {
			dq.buildErr = err
			return dq
		}
		sqlStr, args := cond.Build(dq.builder.db.dialect)
		if sqlStr == "" {
			dq.setBuildErr(errEmptyCondition)
			return dq
		}
		dq.where = append(dq.where, sqlStr)
		dq.params = append(dq.params, args...)

	default:
		dq.buildErr = fmt.Errorf("relica: Where() expects string or Expression, got %T", condition)
	}

	return dq
}

// AndWhere adds a WHERE condition with AND logic.
// If no existing WHERE clause exists, behaves like Where().
// Multiple conditions are combined with AND.
//
// String example:
//
//	AndWhere("age > ?", 18)
//
// Expression example:
//
//	AndWhere(relica.GreaterThan("age", 18))
func (dq *DeleteQuery) AndWhere(condition any, params ...any) *DeleteQuery {
	// Simply delegate to Where() which already uses AND logic for multiple calls.
	return dq.Where(condition, params...)
}

// OrWhere adds a WHERE condition with OR logic.
// If no existing WHERE clause exists, behaves like Where().
// The new condition is combined with existing conditions using OR.
//
// String example:
//
//	OrWhere("status = ?", "admin")
//
// Expression example:
//
//	OrWhere(relica.Eq("status", "admin"))
//
//nolint:dupl // OrWhere on SelectQuery/UpdateQuery/DeleteQuery share structure but operate on different receiver types; a generic helper would require any gymnastics.
func (dq *DeleteQuery) OrWhere(condition any, params ...any) *DeleteQuery {
	if len(dq.where) == 0 {
		// No existing WHERE clause - just add it.
		return dq.Where(condition, params...)
	}

	// Build the new condition.
	var newSQL string
	var newArgs []any

	switch cond := condition.(type) {
	case string:
		var err error
		if strings.TrimSpace(cond) == "" {
			dq.setBuildErr(errEmptyCondition)
			return dq
		}
		newSQL, newArgs, err = resolveNamedParams(cond, params)
		if err != nil {
			dq.buildErr = err
			return dq
		}

	case Expression:
		if err := ExpressionError(cond); err != nil {
			dq.buildErr = err
			return dq
		}
		newSQL, newArgs = cond.Build(dq.builder.db.dialect)
		if newSQL == "" {
			dq.setBuildErr(errEmptyCondition)
			return dq
		}

	default:
		dq.buildErr = fmt.Errorf("relica: OrWhere() expects string or Expression, got %T", condition)
		return dq
	}

	// Combine existing WHERE with new condition using OR.
	// Wrap both sides in parentheses for correct precedence.
	existingWhere := strings.Join(dq.where, " AND ")
	combined := "(" + existingWhere + ") OR (" + newSQL + ")"

	// Replace WHERE clauses with combined condition.
	dq.where = []string{combined}
	dq.params = append(dq.params, newArgs...)

	return dq
}

// Build constructs the Query object from DeleteQuery.
// If a programming error was stored during query construction, it is propagated
// through the Query and returned by Execute at call time instead of panicking.
func (dq *DeleteQuery) Build() *Query {
	// Context priority: query ctx > builder ctx > nil
	ctx := dq.ctx
	if ctx == nil {
		ctx = dq.builder.ctx
	}

	if dq.buildErr == nil {
		dq.buildErr = wholeTableError(dq.whereCalled, dq.allRows)
	}
	if dq.buildErr != nil {
		return &Query{
			prepErr: dq.buildErr,
			db:      dq.builder.db,
			tx:      dq.builder.tx,
			ctx:     ctx,
		}
	}

	// Build WHERE clause (keep ? — single-pass renumbering below)
	whereClause := ""
	whereParams := dq.params
	if len(dq.where) > 0 {
		whereClause = " WHERE " + strings.Join(dq.where, " AND ")
	}

	// Construct raw SQL and single-pass renumber
	rawSQL := "DELETE FROM " + quoteTableName(dq.table, dq.builder.db.dialect) + whereClause
	query, count := replacePlaceholders(rawSQL, 1, dq.builder.db.dialect)
	if count != len(whereParams) {
		return &Query{
			prepErr: fmt.Errorf("relica: %d placeholders but %d args", count, len(whereParams)),
			db:      dq.builder.db,
			tx:      dq.builder.tx,
			ctx:     ctx,
		}
	}

	return &Query{
		sql:    query,
		params: whereParams,
		db:     dq.builder.db,
		tx:     dq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the DELETE query and returns the result.
func (dq *DeleteQuery) Execute() (any, error) {
	return dq.Build().Execute()
}

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
//
// Example:
//
//	sql, params := db.Delete("users").Where(relica.Eq("id", 1)).ToSQL()
func (dq *DeleteQuery) ToSQL() (string, []any) {
	q := dq.Build()
	return q.sql, q.params
}

// BatchInsertQuery represents a batch INSERT query being built.
// It allows inserting multiple rows with a single SQL statement for performance.
type BatchInsertQuery struct {
	builder  *QueryBuilder
	table    string
	columns  []string
	rows     [][]any
	conflict conflictClause
	ctx      context.Context // context for this specific query
	buildErr error           // stored programming error (replaces panic in fluent chain)
}

// WithContext sets the context for this batch INSERT query.
// This overrides any context set on the QueryBuilder.
func (biq *BatchInsertQuery) WithContext(ctx context.Context) *BatchInsertQuery {
	biq.ctx = ctx
	return biq
}

// BatchInsert creates a batch INSERT query for the specified table and columns.
// This is optimized for inserting multiple rows in a single statement.
// Example:
//
//	db.Builder().BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").
//	    Values("Bob", "bob@example.com").
//	    Execute()
func (qb *QueryBuilder) BatchInsert(table string, columns []string) *BatchInsertQuery {
	return &BatchInsertQuery{
		builder: qb,
		table:   table,
		columns: columns,
		rows:    make([][]any, 0),
	}
}

// Values adds a row of values to the batch insert.
// The number of values must match the number of columns specified in BatchInsert.
// Stores an error (instead of panicking) if the value count doesn't match the column count.
func (biq *BatchInsertQuery) Values(values ...any) *BatchInsertQuery {
	if len(values) != len(biq.columns) {
		biq.buildErr = fmt.Errorf("relica: BatchInsert.Values expected %d values for %d columns, got %d", len(biq.columns), len(biq.columns), len(values))
		return biq
	}
	biq.rows = append(biq.rows, values)
	return biq
}

// ValuesMap adds a row from a map of column names to values.
// Values are extracted in the order of columns specified in BatchInsert.
// Missing columns will have nil values; a key that is not one of the batch
// columns is an error (it used to be dropped silently).
func (biq *BatchInsertQuery) ValuesMap(values map[string]any) *BatchInsertQuery {
	known := make(map[string]bool, len(biq.columns))
	for _, col := range biq.columns {
		known[col] = true
	}
	for _, key := range getKeys(values) {
		if !known[key] {
			if biq.buildErr == nil {
				biq.buildErr = fmt.Errorf("relica: BatchInsert.ValuesMap key %q is not one of the batch columns %v", key, biq.columns)
			}
			return biq
		}
	}
	row := make([]any, len(biq.columns))
	for i, col := range biq.columns {
		row[i] = values[col]
	}
	return biq.Values(row...)
}

// OnConflict sets the conflict target and turns the batch into an upsert, which
// then needs exactly one of DoUpdate, DoUpdateAll, DoNothing (ADR-019).
// Required for DoUpdate/DoUpdateAll on PostgreSQL and SQLite; advisory on MySQL.
func (biq *BatchInsertQuery) OnConflict(columns ...string) *BatchInsertQuery {
	biq.conflict.onConflict(columns)
	return biq
}

// DoUpdate sets the listed columns from each proposed row on conflict. Each
// column must be one of the batch columns.
func (biq *BatchInsertQuery) DoUpdate(columns ...string) *BatchInsertQuery {
	biq.conflict.setAction(upsertDoUpdate, columns)
	return biq
}

// DoUpdateAll sets every batch column except the conflict target from each
// proposed row on conflict.
func (biq *BatchInsertQuery) DoUpdateAll() *BatchInsertQuery {
	biq.conflict.setAction(upsertDoUpdateAll, nil)
	return biq
}

// DoNothing keeps existing rows on conflict: ON CONFLICT DO NOTHING on
// PostgreSQL/SQLite; on MySQL a self-assignment of the first conflict column (or
// the first batch column) in ON DUPLICATE KEY UPDATE.
func (biq *BatchInsertQuery) DoNothing() *BatchInsertQuery {
	biq.conflict.setAction(upsertDoNothing, nil)
	return biq
}

// Build constructs the Query object from BatchInsertQuery.
// Generates SQL in the form: INSERT INTO table (cols) VALUES (?, ?), (?, ?), ...
// If OnConflict is set, appends the appropriate conflict resolution clause.
// If a programming error was stored (e.g., wrong Values count or no rows added),
// it is propagated through the Query and returned by Execute at call time.
func (biq *BatchInsertQuery) Build() *Query {
	// Context priority: query ctx > builder ctx > nil
	ctx := biq.ctx
	if ctx == nil {
		ctx = biq.builder.ctx
	}

	if biq.buildErr != nil {
		return &Query{
			prepErr: biq.buildErr,
			db:      biq.builder.db,
			tx:      biq.builder.tx,
			ctx:     ctx,
		}
	}

	if len(biq.rows) == 0 {
		return &Query{
			prepErr: fmt.Errorf("relica: BatchInsert.Build called with no rows to insert"),
			db:      biq.builder.db,
			tx:      biq.builder.tx,
			ctx:     ctx,
		}
	}

	// Build column list with proper quoting
	quotedColumns := make([]string, len(biq.columns))
	for i, col := range biq.columns {
		quotedColumns[i] = biq.builder.db.dialect.QuoteIdentifier(col)
	}

	// Build VALUES clause with placeholders for all rows
	valueClauses := make([]string, len(biq.rows))
	params := make([]any, 0, len(biq.rows)*len(biq.columns))

	paramIndex := 1
	for i, row := range biq.rows {
		placeholders := make([]string, len(biq.columns))
		for j := 0; j < len(biq.columns); j++ {
			placeholders[j] = biq.builder.db.dialect.Placeholder(paramIndex)
			params = append(params, row[j])
			paramIndex++
		}
		valueClauses[i] = "(" + strings.Join(placeholders, ", ") + ")"
	}

	query := "INSERT INTO " + quoteTableName(biq.table, biq.builder.db.dialect) +
		" (" + strings.Join(quotedColumns, ", ") + ") VALUES " +
		strings.Join(valueClauses, ", ")

	// A batch without OnConflict or an action is a plain INSERT.
	if biq.conflict.used() {
		conflictSQL, err := biq.conflict.render(biq.builder.db, biq.columns)
		if err != nil {
			return &Query{prepErr: err, db: biq.builder.db, tx: biq.builder.tx, ctx: ctx}
		}
		query += conflictSQL
	}

	return &Query{
		sql:    query,
		params: params,
		db:     biq.builder.db,
		tx:     biq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the batch INSERT query and returns the result.
func (biq *BatchInsertQuery) Execute() (any, error) {
	return biq.Build().Execute()
}

// BatchUpdateQuery represents a batch UPDATE query using CASE-WHEN logic.
// It updates multiple rows with different values in a single SQL statement.
type BatchUpdateQuery struct {
	builder       *QueryBuilder
	table         string
	keyColumn     string
	updates       []batchUpdateRow
	updateColumns []string        // Cached list of columns to update
	ctx           context.Context // context for this specific query
}

// WithContext sets the context for this batch UPDATE query.
// This overrides any context set on the QueryBuilder.
func (buq *BatchUpdateQuery) WithContext(ctx context.Context) *BatchUpdateQuery {
	buq.ctx = ctx
	return buq
}

// batchUpdateRow represents a single row update in a batch.
type batchUpdateRow struct {
	keyValue any
	values   map[string]any
}

// BatchUpdate creates a batch UPDATE query for the specified table.
// The keyColumn is used to identify which rows to update (typically the primary key).
// Example:
//
//	db.Builder().BatchUpdate("users", "id").
//	    Set(1, map[string]any{"name": "Alice", "status": "active"}).
//	    Set(2, map[string]any{"name": "Bob", "status": "inactive"}).
//	    Execute()
func (qb *QueryBuilder) BatchUpdate(table, keyColumn string) *BatchUpdateQuery {
	return &BatchUpdateQuery{
		builder:   qb,
		table:     table,
		keyColumn: keyColumn,
		updates:   make([]batchUpdateRow, 0),
	}
}

// Set adds a row update to the batch.
// keyValue is the value of the key column for this row.
// values contains the columns and their new values for this row.
func (buq *BatchUpdateQuery) Set(keyValue any, values map[string]any) *BatchUpdateQuery {
	buq.updates = append(buq.updates, batchUpdateRow{
		keyValue: keyValue,
		values:   values,
	})

	// Update the list of columns to update (union of all columns across all rows)
	if buq.updateColumns == nil {
		buq.updateColumns = getKeys(values)
	} else {
		// Add any new columns from this row
		for col := range values {
			found := false
			for _, existing := range buq.updateColumns {
				if existing == col {
					found = true
					break
				}
			}
			if !found {
				buq.updateColumns = append(buq.updateColumns, col)
			}
		}
		sort.Strings(buq.updateColumns) // Keep sorted for consistency
	}

	return buq
}

// Build constructs the Query object from BatchUpdateQuery.
// Generates SQL using CASE-WHEN for each column:
//
//	UPDATE table SET
//	  col1 = CASE key WHEN ? THEN ? WHEN ? THEN ? END,
//	  col2 = CASE key WHEN ? THEN ? WHEN ? THEN ? END
//	WHERE key IN (?, ?)
//
// If no updates have been added, returns a Query carrying an error instead of panicking.
func (buq *BatchUpdateQuery) Build() *Query {
	// Context priority: query ctx > builder ctx > nil
	ctx := buq.ctx
	if ctx == nil {
		ctx = buq.builder.ctx
	}

	if len(buq.updates) == 0 {
		return &Query{
			prepErr: fmt.Errorf("relica: BatchUpdate.Build called with no updates to apply"),
			db:      buq.builder.db,
			tx:      buq.builder.tx,
			ctx:     ctx,
		}
	}

	// Collect all key values for WHERE IN clause
	keyValues := make([]any, len(buq.updates))
	for i, update := range buq.updates {
		keyValues[i] = update.keyValue
	}

	// Build CASE-WHEN for each column
	setClauses := make([]string, 0, len(buq.updateColumns))
	params := make([]any, 0)
	paramIndex := 1

	for _, col := range buq.updateColumns {
		// Build: col = CASE key_column WHEN ? THEN ? WHEN ? THEN ? ELSE col END
		// The ELSE clause preserves the existing value for rows not being updated for this column
		quotedCol := buq.builder.db.dialect.QuoteIdentifier(col)
		caseWhen := quotedCol + " = CASE " + buq.builder.db.dialect.QuoteIdentifier(buq.keyColumn)

		for _, update := range buq.updates {
			// Only add WHEN clause if this row has this column
			if val, exists := update.values[col]; exists {
				caseWhen += " WHEN " + buq.builder.db.dialect.Placeholder(paramIndex) +
					" THEN " + buq.builder.db.dialect.Placeholder(paramIndex+1)
				params = append(params, update.keyValue, val)
				paramIndex += 2
			}
		}

		// ELSE clause preserves existing value for rows not updating this column
		caseWhen += " ELSE " + quotedCol + " END"
		setClauses = append(setClauses, caseWhen)
	}

	// Build WHERE IN clause
	whereInPlaceholders := make([]string, len(keyValues))
	for i := range keyValues {
		whereInPlaceholders[i] = buq.builder.db.dialect.Placeholder(paramIndex)
		params = append(params, keyValues[i])
		paramIndex++
	}

	query := "UPDATE " + quoteTableName(buq.table, buq.builder.db.dialect) +
		" SET " + strings.Join(setClauses, ", ") +
		" WHERE " + buq.builder.db.dialect.QuoteIdentifier(buq.keyColumn) +
		" IN (" + strings.Join(whereInPlaceholders, ", ") + ")"

	return &Query{
		sql:    query,
		params: params,
		db:     buq.builder.db,
		tx:     buq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the batch UPDATE query and returns the result.
func (buq *BatchUpdateQuery) Execute() (any, error) {
	return buq.Build().Execute()
}

// lockError enforces ADR-020 rules 3 and 4 for row locks.
func (sq *SelectQuery) lockError() error {
	if !sq.locksRows() {
		return nil
	}
	if len(sq.unions) > 0 || len(sq.groupBy) > 0 || sq.distinct {
		return errors.New("relica: a row lock (ForUpdate/ForShare/ForUpdateSkipLocked) cannot be combined with " +
			"UNION/INTERSECT/EXCEPT, GROUP BY or DISTINCT: the result rows are not table rows")
	}
	if sq.builder.tx == nil {
		return errors.New("relica: ForUpdate/ForShare/ForUpdateSkipLocked require a transaction: " +
			"outside one the lock ends with the statement")
	}
	return nil
}

// locksRows reports whether the query or one of its set-operation branches
// has a row lock (ForUpdate/ForShare/ForUpdateSkipLocked).
func (sq *SelectQuery) locksRows() bool {
	if sq.lockClause != "" {
		return true
	}
	for _, u := range sq.unions {
		if u.query != nil && u.query.lockClause != "" {
			return true
		}
	}
	return false
}

// errLockWithAggregate is returned by Count and Exists on a locking query.
var errLockWithAggregate = errors.New("relica: a row lock (ForUpdate/ForShare/ForUpdateSkipLocked) cannot be " +
	"combined with Count() or Exists(): lock the rows with All/One, or drop the lock")

// AllRows lets the statement touch every row of the table. Without it an
// UPDATE with no Where is a build error (ADR-020).
func (uq *UpdateQuery) AllRows() *UpdateQuery {
	uq.allRows = true
	return uq
}

func (uq *UpdateQuery) setBuildErr(err error) {
	if uq.buildErr == nil {
		uq.buildErr = err
	}
}

// AllRows lets the statement touch every row of the table. Without it an
// DELETE with no Where is a build error (ADR-020).
func (dq *DeleteQuery) AllRows() *DeleteQuery {
	dq.allRows = true
	return dq
}

func (dq *DeleteQuery) setBuildErr(err error) {
	if dq.buildErr == nil {
		dq.buildErr = err
	}
}

// errEmptyCondition: a Where condition that renders no SQL (And(), HashExp{},
// Like with no values, "") would drop the WHERE and hit the whole table.
var errEmptyCondition = errors.New("relica: the UPDATE/DELETE condition renders empty (And(), Or(), HashExp{}, " +
	"Like with no values, \"\"), which would affect every row; use AllRows() without Where to do that on purpose")

// wholeTableError enforces ADR-020 rule 1.
func wholeTableError(whereCalled, allRows bool) error {
	switch {
	case allRows && whereCalled:
		return errors.New("relica: AllRows() together with Where: choose one")
	case !allRows && !whereCalled:
		return errors.New("relica: UPDATE/DELETE without Where would affect every row; call AllRows() to do that on purpose")
	}
	return nil
}
