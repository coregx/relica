package core

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/coregx/relica/internal/analyzer"
	"github.com/coregx/relica/internal/dialects"
)

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
	JoinType string      // "INNER JOIN", "LEFT JOIN", "RIGHT JOIN", "FULL OUTER JOIN", "CROSS JOIN"
	Table    string      // Table name with optional alias: "users u", "messages m"
	On       interface{} // string | Expression | nil
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

// SelectQuery represents a SELECT query being built.
type SelectQuery struct {
	builder       *QueryBuilder
	columns       []string
	table         string      // DEPRECATED: use fromSrc instead (kept for backward compatibility)
	fromSrc       *fromSource // FROM source (table or subquery)
	selectExprs   []RawExp    // Raw SELECT expressions (for scalar subqueries, etc.)
	joins         []JoinInfo
	where         []string
	params        []interface{}
	groupBy       []string // GROUP BY columns: ["user_id", "status"]
	havingClauses []struct {
		condition string
		args      []interface{}
	} // HAVING clauses (WHERE for aggregates)
	orderBy     []string        // ORDER BY clauses: ["age DESC", "name ASC", "created_at"]
	limitValue  *int64          // LIMIT value (nil = not set)
	offsetValue *int64          // OFFSET value (nil = not set)
	unions      []unionInfo     // Set operations: UNION, INTERSECT, EXCEPT
	ctes        []cteInfo       // Common Table Expressions (CTEs)
	distinct    bool            // SELECT DISTINCT flag
	ctx         context.Context // context for this specific query
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
		panic("FromSelect: alias is required for subquery")
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

// SelectExpr adds a raw SQL expression to the SELECT clause.
// This is useful for scalar subqueries, window functions, or complex expressions.
//
// Example:
//
//	db.Builder().
//	    Select("id", "name").
//	    SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id)", "order_count").
//	    From("users").
//	    All(&results)
//
// Generates:
//
//	SELECT id, name, (SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) AS order_count
//	FROM users
//
// Note: The SQL expression is used as-is. You're responsible for proper quoting and SQL injection prevention.
func (sq *SelectQuery) SelectExpr(expr string, args ...interface{}) *SelectQuery {
	sq.selectExprs = append(sq.selectExprs, RawExp{
		SQL:  expr,
		Args: args,
	})
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
func (sq *SelectQuery) Where(condition interface{}, params ...interface{}) *SelectQuery {
	switch cond := condition.(type) {
	case string:
		// Legacy string-based WHERE (backward compatible)
		sq.where = append(sq.where, cond)
		sq.params = append(sq.params, params...)

	case Expression:
		// New Expression-based WHERE
		sqlStr, args := cond.Build(sq.builder.db.dialect)
		if sqlStr != "" {
			sq.where = append(sq.where, sqlStr)
			sq.params = append(sq.params, args...)
		}

	default:
		panic("Where() expects string or Expression")
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
func (sq *SelectQuery) AndWhere(condition interface{}, params ...interface{}) *SelectQuery {
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
func (sq *SelectQuery) OrWhere(condition interface{}, params ...interface{}) *SelectQuery {
	if len(sq.where) == 0 {
		// No existing WHERE clause - just add it.
		return sq.Where(condition, params...)
	}

	// Build the new condition.
	var newSQL string
	var newArgs []interface{}

	switch cond := condition.(type) {
	case string:
		newSQL = cond
		newArgs = params

	case Expression:
		newSQL, newArgs = cond.Build(sq.builder.db.dialect)
		if newSQL == "" {
			// Empty expression - nothing to add.
			return sq
		}

	default:
		panic("OrWhere() expects string or Expression")
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
func (sq *SelectQuery) Join(joinType, table string, on interface{}) *SelectQuery {
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
func (sq *SelectQuery) InnerJoin(table string, on interface{}) *SelectQuery {
	return sq.Join("INNER JOIN", table, on)
}

// LeftJoin adds a LEFT JOIN (LEFT OUTER JOIN) clause to the SELECT query.
// table is the table name with optional alias (e.g., "attachments a").
// on can be a string or Expression specifying the join condition.
//
// Example:
//
//	LeftJoin("attachments a", "m.id = a.message_id")
func (sq *SelectQuery) LeftJoin(table string, on interface{}) *SelectQuery {
	return sq.Join("LEFT JOIN", table, on)
}

// RightJoin adds a RIGHT JOIN (RIGHT OUTER JOIN) clause to the SELECT query.
// table is the table name with optional alias.
// on can be a string or Expression specifying the join condition.
//
// Example:
//
//	RightJoin("users u", "m.user_id = u.id")
func (sq *SelectQuery) RightJoin(table string, on interface{}) *SelectQuery {
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
func (sq *SelectQuery) FullJoin(table string, on interface{}) *SelectQuery {
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
// Chainable: multiple OrderBy() calls append to the same clause.
//
// Examples:
//
//	OrderBy("age DESC")                    // Single column descending
//	OrderBy("status ASC", "created_at")    // Multiple columns (created_at defaults to ASC)
//	OrderBy("name").OrderBy("age DESC")    // Chained calls
func (sq *SelectQuery) OrderBy(columns ...string) *SelectQuery {
	sq.orderBy = append(sq.orderBy, columns...)
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
	if other != nil {
		sq.unions = append(sq.unions, unionInfo{query: other, all: false, op: "UNION"})
	}
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
	if other != nil {
		sq.unions = append(sq.unions, unionInfo{query: other, all: true, op: "UNION"})
	}
	return sq
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
	if other != nil {
		sq.unions = append(sq.unions, unionInfo{query: other, all: false, op: "INTERSECT"})
	}
	return sq
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
	if other != nil {
		sq.unions = append(sq.unions, unionInfo{query: other, all: false, op: "EXCEPT"})
	}
	return sq
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
		panic("CTE name cannot be empty")
	}
	if query == nil {
		panic("CTE query cannot be nil")
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
//	anchor := db.Builder().Select("id", "name", "manager_id", "1 as level").
//	    From("employees").
//	    Where("manager_id IS NULL")
//
//	recursive := db.Builder().Select("e.id", "e.name", "e.manager_id", "h.level + 1").
//	    From("employees e").
//	    Join("INNER JOIN hierarchy h ON e.manager_id = h.id")
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
//	    SELECT id, name, manager_id, 1 as level FROM "employees" WHERE manager_id IS NULL
//	    UNION ALL
//	    SELECT e.id, e.name, e.manager_id, h.level + 1 FROM "employees" AS "e" INNER JOIN hierarchy h ON e.manager_id = h.id
//	)
//	SELECT * FROM "hierarchy" ORDER BY "level", "name"
//
// Database support:
//   - PostgreSQL: ✓ (all versions)
//   - MySQL 8.0+: ✓ (added in MySQL 8.0.1)
//   - SQLite 3.25+: ✓ (added in SQLite 3.25.0)
func (sq *SelectQuery) WithRecursive(name string, query *SelectQuery) *SelectQuery {
	if name == "" {
		panic("CTE name cannot be empty")
	}
	if query == nil {
		panic("CTE query cannot be nil")
	}
	// Validate that query contains UNION (required for recursive CTE)
	if len(query.unions) == 0 {
		panic("recursive CTE requires UNION or UNION ALL")
	}
	sq.ctes = append(sq.ctes, cteInfo{
		name:      name,
		query:     query,
		recursive: true,
	})
	return sq
}

// Distinct sets whether to select distinct rows.
// When enabled, adds DISTINCT keyword to the SELECT clause to eliminate duplicate rows.
// Multiple calls to Distinct() override previous settings.
//
// Example:
//
//	db.Builder().Select("category").From("products").Distinct(true).All(&categories)
//	// SELECT DISTINCT "category" FROM "products"
//
//	db.Builder().Select("*").From("users").Distinct(false).All(&users)
//	// SELECT * FROM "users"
func (sq *SelectQuery) Distinct(v bool) *SelectQuery {
	sq.distinct = v
	return sq
}

// buildTableWithAlias builds a table reference with optional alias.
// Input: "users u" → Output: "users" AS "u" (quoted)
// Input: "users" → Output: "users" (quoted)
func (sq *SelectQuery) buildTableWithAlias(table string, dialect dialects.Dialect) string {
	tableParts := strings.Fields(table)
	if len(tableParts) == 2 {
		// Table with alias
		quotedTable := dialect.QuoteIdentifier(tableParts[0])
		quotedAlias := dialect.QuoteIdentifier(tableParts[1])
		return quotedTable + " AS " + quotedAlias
	}
	// Simple table name
	return dialect.QuoteIdentifier(table)
}

// buildFrom constructs the FROM clause, handling both tables and subqueries.
// Returns the FROM SQL fragment and appends any subquery parameters to params.
func (sq *SelectQuery) buildFrom(dialect dialects.Dialect, params *[]interface{}) string {
	// Prefer fromSrc if set (supports subqueries)
	if sq.fromSrc != nil {
		if sq.fromSrc.isSubquery {
			// FROM (SELECT ...) AS alias
			subSQL, subArgs := sq.fromSrc.subquery.buildSQL(dialect)
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
func (sq *SelectQuery) buildJoins(dialect dialects.Dialect, params *[]interface{}) string {
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
				panic(fmt.Sprintf("JOIN ON must be string, Expression, or nil, got %T", join.On))
			}
		}

		parts = append(parts, part)
	}

	return strings.Join(parts, "")
}

// buildOrderBy constructs the ORDER BY clause from the orderBy slice.
// Returns empty string if no ORDER BY is specified.
// Parses column direction (ASC/DESC) and quotes column names.
func (sq *SelectQuery) buildOrderBy(dialect dialects.Dialect) string {
	if len(sq.orderBy) == 0 {
		return ""
	}

	parts := make([]string, 0, len(sq.orderBy))
	for _, col := range sq.orderBy {
		// Parse "column [ASC|DESC]"
		fields := strings.Fields(col)
		if len(fields) == 0 {
			continue
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

		parts = append(parts, quoted)
	}

	if len(parts) == 0 {
		return ""
	}

	return " ORDER BY " + strings.Join(parts, ", ")
}

// quoteColumnName quotes a column name, handling table prefixes.
// Examples: "age" → "age", "users.age" → "users"."age"
func (sq *SelectQuery) quoteColumnName(col string, dialect dialects.Dialect) string {
	// Check for table.column format
	if strings.Contains(col, ".") {
		parts := strings.SplitN(col, ".", 2)
		return dialect.QuoteIdentifier(parts[0]) + "." + dialect.QuoteIdentifier(parts[1])
	}
	return dialect.QuoteIdentifier(col)
}

// buildLimitOffset constructs the LIMIT and OFFSET clauses.
// Returns empty string if neither is set.
func (sq *SelectQuery) buildLimitOffset() string {
	var result string

	if sq.limitValue != nil {
		result += fmt.Sprintf(" LIMIT %d", *sq.limitValue)
	}

	if sq.offsetValue != nil {
		result += fmt.Sprintf(" OFFSET %d", *sq.offsetValue)
	}

	return result
}

// buildSelect constructs the SELECT clause, handling aggregate functions and raw expressions.
// Detects aggregate functions (contains "(") and column aliases (contains "AS").
// Returns "*" if no columns and no selectExprs specified.
// Includes DISTINCT keyword if sq.distinct is true.
func (sq *SelectQuery) buildSelect(dialect dialects.Dialect) string {
	parts := make([]string, 0, len(sq.columns)+len(sq.selectExprs))

	// Add regular columns
	for _, col := range sq.columns {
		// Determine column type and format accordingly
		switch {
		case col == "*":
			// Wildcard - use as-is, don't quote
			parts = append(parts, "*")
		case strings.Contains(col, "("):
			// Aggregate function or expression - use as-is
			parts = append(parts, col)
		case strings.Contains(col, " as ") || strings.Contains(col, " AS "):
			// Column with alias - use as-is
			parts = append(parts, col)
		default:
			// Simple column name - quote it
			parts = append(parts, sq.quoteColumnName(col, dialect))
		}
	}

	// Add raw SELECT expressions (scalar subqueries, etc.)
	for _, expr := range sq.selectExprs {
		parts = append(parts, expr.SQL)
	}

	columns := "*"
	if len(parts) > 0 {
		columns = strings.Join(parts, ", ")
	}

	// Add DISTINCT keyword if enabled
	if sq.distinct {
		return "DISTINCT " + columns
	}

	return columns
}

// GroupBy adds GROUP BY clause.
// Multiple columns supported: GroupBy("user_id", "status")
// Chainable: GroupBy("a").GroupBy("b")
func (sq *SelectQuery) GroupBy(columns ...string) *SelectQuery {
	sq.groupBy = append(sq.groupBy, columns...)
	return sq
}

// buildGroupBy constructs the GROUP BY clause from the groupBy slice.
// Returns empty string if no GROUP BY is specified.
// Quotes column names using dialect.
func (sq *SelectQuery) buildGroupBy(dialect dialects.Dialect) string {
	if len(sq.groupBy) == 0 {
		return ""
	}

	parts := make([]string, 0, len(sq.groupBy))
	for _, col := range sq.groupBy {
		parts = append(parts, sq.quoteColumnName(col, dialect))
	}

	return " GROUP BY " + strings.Join(parts, ", ")
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
func (sq *SelectQuery) Having(condition interface{}, args ...interface{}) *SelectQuery {
	switch cond := condition.(type) {
	case string:
		// String-based HAVING
		sq.havingClauses = append(sq.havingClauses, struct {
			condition string
			args      []interface{}
		}{
			condition: cond,
			args:      args,
		})

	case Expression:
		// Expression-based HAVING
		sqlStr, exprArgs := cond.Build(sq.builder.db.dialect)
		if sqlStr != "" {
			sq.havingClauses = append(sq.havingClauses, struct {
				condition string
				args      []interface{}
			}{
				condition: sqlStr,
				args:      exprArgs,
			})
		}

	default:
		panic(fmt.Sprintf("Having() expects string or Expression, got %T", condition))
	}

	return sq
}

// buildHaving constructs the HAVING clause from the havingClauses slice.
// Returns empty string if no HAVING is specified.
// Multiple clauses are combined with AND.
// Appends parameters to params slice.
func (sq *SelectQuery) buildHaving(params *[]interface{}) string {
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

// renumberHavingPlaceholders renumbers placeholders in HAVING clause for PostgreSQL.
// For databases using positional placeholders ($1, $2), replaces ? with numbered placeholders.
func (sq *SelectQuery) renumberHavingPlaceholders(havingClause string, totalParams int, dialect dialects.Dialect) string {
	if dialect.Placeholder(1) == "?" || len(sq.havingClauses) == 0 {
		return havingClause
	}

	// Count current params (JOIN + WHERE)
	currentParamCount := totalParams - len(sq.havingClauses)
	for i := 0; i < len(sq.havingClauses); i++ {
		for range sq.havingClauses[i].args {
			currentParamCount++
			placeholder := dialect.Placeholder(currentParamCount)
			havingClause = strings.Replace(havingClause, "?", placeholder, 1)
		}
	}

	return havingClause
}

// buildWhere constructs the WHERE clause from the where slice.
// Returns empty string if no WHERE is specified.
// Multiple clauses are combined with AND.
// Appends parameters to params slice and handles placeholder renumbering for PostgreSQL.
func (sq *SelectQuery) buildWhere(dialect dialects.Dialect, params *[]interface{}) string {
	if len(sq.where) == 0 {
		return ""
	}

	whereParams := sq.params
	whereClause := " WHERE " + strings.Join(sq.where, " AND ")

	// Renumber WHERE placeholders for PostgreSQL ($1, $2, etc.)
	if dialect.Placeholder(1) != "?" {
		// Start numbering after CTE + SelectExpr + FROM + JOIN params
		startIndex := len(*params) + 1
		for i := range whereParams {
			placeholder := dialect.Placeholder(startIndex + i)
			whereClause = strings.Replace(whereClause, "?", placeholder, 1)
		}
	}

	// Append WHERE params after CTE + SelectExpr + FROM + JOIN params
	*params = append(*params, whereParams...)

	return whereClause
}

// buildWithClause generates the WITH clause for CTEs.
func (sq *SelectQuery) buildWithClause(dialect dialects.Dialect) (string, []interface{}) {
	if len(sq.ctes) == 0 {
		return "", nil
	}

	var parts []string
	var allArgs []interface{}

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

	// Build each CTE
	cteStrings := make([]string, 0, len(sq.ctes))
	for _, cte := range sq.ctes {
		cteSQL, cteArgs := cte.query.buildSQL(dialect)

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

// buildSQL constructs the SQL string and parameters for SelectQuery.
// This is the core implementation shared by both Build() and the Expression interface.
// Parameter ordering: CTEs → SelectExprs → FROM subquery → JOINs → WHERE → HAVING
func (sq *SelectQuery) buildSQL(dialect dialects.Dialect) (string, []interface{}) {
	// Collect all parameters in correct order
	var allParams []interface{}
	var parts []string

	// 1. Build WITH clause if CTEs exist
	if len(sq.ctes) > 0 {
		withClause, withArgs := sq.buildWithClause(dialect)
		parts = append(parts, withClause)
		allParams = append(allParams, withArgs...)
	}

	// 2. Add params from SelectExpr (scalar subqueries in SELECT clause)
	for _, expr := range sq.selectExprs {
		allParams = append(allParams, expr.Args...)
	}

	// 3. Build SELECT clause (handles aggregates and raw expressions)
	cols := sq.buildSelect(dialect)

	// 4. Build FROM clause (may be table or subquery)
	fromClause := sq.buildFrom(dialect, &allParams)

	// 5. Build JOIN clause (adds params via pointer)
	joinClause := sq.buildJoins(dialect, &allParams)

	// 6. Build WHERE clause (adds params via pointer)
	whereClause := sq.buildWhere(dialect, &allParams)

	// 7. Renumber SelectExpr, FROM, and JOIN placeholders if needed (PostgreSQL)
	if dialect.Placeholder(1) != "?" {
		// Renumber SELECT expressions
		selectExprParamCount := 0
		for _, expr := range sq.selectExprs {
			selectExprParamCount += len(expr.Args)
		}

		// Renumber FROM subquery placeholders (already handled in buildFrom)
		// Renumber JOIN placeholders (already handled in buildJoins)
	}

	// 8. Build GROUP BY clause
	groupByClause := sq.buildGroupBy(dialect)

	// 9. Build HAVING clause (adds params via pointer)
	havingClause := sq.buildHaving(&allParams)

	// Renumber HAVING placeholders if needed (PostgreSQL)
	havingClause = sq.renumberHavingPlaceholders(havingClause, len(allParams), dialect)

	// 10. Build ORDER BY clause
	orderByClause := sq.buildOrderBy(dialect)

	// 11. Build LIMIT/OFFSET clause
	limitOffsetClause := sq.buildLimitOffset()

	// Construct SQL: SELECT ... FROM ... JOIN ... WHERE ... GROUP BY ... HAVING ... ORDER BY ... LIMIT ... OFFSET
	query := "SELECT " + cols + fromClause + joinClause + whereClause + groupByClause + havingClause + orderByClause + limitOffsetClause

	// 12. Handle set operations (UNION, INTERSECT, EXCEPT)
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

// buildSetOperations handles UNION, INTERSECT, EXCEPT operations.
// This method is extracted from buildSQL to reduce cognitive complexity.
func (sq *SelectQuery) buildSetOperations(mainQuery string, allParams []interface{}, dialect dialects.Dialect) (string, []interface{}) {
	// Wrap main query in parentheses
	mainSQL := "(" + mainQuery + ")"

	for _, u := range sq.unions {
		// Build union query SQL
		unionSQL, unionArgs := u.query.buildSQL(dialect)

		// Renumber placeholders if needed (PostgreSQL)
		if dialect.Placeholder(1) != "?" {
			// Renumber placeholders to continue from current parameter count
			startIndex := len(allParams) + 1
			for i := 0; i < len(unionArgs); i++ {
				oldPlaceholder := dialect.Placeholder(i + 1)
				newPlaceholder := dialect.Placeholder(startIndex + i)
				unionSQL = strings.Replace(unionSQL, oldPlaceholder, newPlaceholder, 1)
			}
		}

		// Determine operation keyword
		op := u.op
		if op == "" {
			op = "UNION"
		}
		if u.all && op == "UNION" {
			op = "UNION ALL"
		}

		// Append set operation: (query1) UNION (query2)
		mainSQL += " " + op + " (" + unionSQL + ")"

		// Merge parameters in order
		allParams = append(allParams, unionArgs...)
	}

	return mainSQL, allParams
}

// Build constructs the Query object from SelectQuery.
func (sq *SelectQuery) Build() *Query {
	query, allParams := sq.buildSQL(sq.builder.db.dialect)

	// Context priority: query ctx > builder ctx > nil
	ctx := sq.ctx
	if ctx == nil {
		ctx = sq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: allParams,
		db:     sq.builder.db,
		tx:     sq.builder.tx,
		ctx:    ctx,
	}
}

// One scans a single row into dest.
func (sq *SelectQuery) One(dest interface{}) error {
	return sq.Build().One(dest)
}

// All scans all rows into dest slice.
func (sq *SelectQuery) All(dest interface{}) error {
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
func (sq *SelectQuery) Row(dest ...interface{}) error {
	return sq.Build().Row(dest...)
}

// Column scans the first column of all rows into a slice.
// The slice parameter must be a pointer to a slice of the appropriate type.
//
// Example:
//
//	var ids []int
//	err := db.Select("id").From("users").Where("status = ?", "active").Column(&ids)
func (sq *SelectQuery) Column(slice interface{}) error {
	return sq.Build().Column(slice)
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
	// Build the SELECT query
	sqlQuery, params := sq.buildSQL(sq.builder.db.dialect)

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
// This is necessary because SelectQuery.Build() returns *Query, not (string, []interface{}).
type selectQueryExpression struct {
	query *SelectQuery
}

// Build implements the Expression interface for SelectQuery.
// This allows SelectQuery to be used in subquery contexts (IN, EXISTS, FROM).
func (sqe *selectQueryExpression) Build(dialect dialects.Dialect) (string, []interface{}) {
	return sqe.query.buildSQL(dialect)
}

// AsExpression converts a SelectQuery to an Expression, allowing it to be used as a subquery.
// This is useful when embedding a SelectQuery in WHERE clauses like IN or EXISTS.
//
// Example:
//
//	sub := db.Builder().Select("user_id").From("orders").Where("total > ?", 100)
//	db.Builder().Select("*").From("users").Where(In("id", sub.AsExpression())).All(&users)
//
// Note: In most cases, you can pass SelectQuery directly to expressions without calling AsExpression,
// as the expression builders will detect and handle SelectQuery automatically.
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
func (qb *QueryBuilder) Insert(table string, values map[string]interface{}) *Query {
	// Get sorted keys for deterministic SQL generation (prevents cache misses)
	keys := getKeys(values)

	placeholders := make([]string, 0, len(keys))
	params := make([]interface{}, 0, len(keys))

	for i, col := range keys {
		placeholders = append(placeholders, qb.db.dialect.Placeholder(i+1))
		params = append(params, values[col])
	}

	query := `INSERT INTO ` + qb.db.dialect.QuoteIdentifier(table) +
		` (` + strings.Join(keys, ", ") + `) ` +
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
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// UpsertQuery represents an UPSERT (INSERT ... ON CONFLICT/DUPLICATE) query.
type UpsertQuery struct {
	builder         *QueryBuilder
	table           string
	values          map[string]interface{}
	conflictColumns []string
	updateColumns   []string
	doNothing       bool
	ctx             context.Context // context for this specific query
}

// WithContext sets the context for this UPSERT query.
// This overrides any context set on the QueryBuilder.
func (uq *UpsertQuery) WithContext(ctx context.Context) *UpsertQuery {
	uq.ctx = ctx
	return uq
}

// Upsert creates an UPSERT query for the given table and values.
// UPSERT is INSERT with conflict resolution (UPDATE or IGNORE).
func (qb *QueryBuilder) Upsert(table string, values map[string]interface{}) *UpsertQuery {
	return &UpsertQuery{
		builder: qb,
		table:   table,
		values:  values,
	}
}

// OnConflict specifies the columns that determine a conflict.
// For PostgreSQL/SQLite: columns in UNIQUE constraint or PRIMARY KEY.
// For MySQL: this is optional (uses PRIMARY KEY or UNIQUE keys automatically).
func (uq *UpsertQuery) OnConflict(columns ...string) *UpsertQuery {
	uq.conflictColumns = columns
	return uq
}

// DoUpdate specifies which columns to update on conflict.
// If not called, defaults to updating all columns except conflict columns.
func (uq *UpsertQuery) DoUpdate(columns ...string) *UpsertQuery {
	uq.updateColumns = columns
	uq.doNothing = false
	return uq
}

// DoNothing specifies to ignore conflicts (do not update).
// This is equivalent to INSERT IGNORE in MySQL or ON CONFLICT DO NOTHING in PostgreSQL.
func (uq *UpsertQuery) DoNothing() *UpsertQuery {
	uq.doNothing = true
	uq.updateColumns = nil
	return uq
}

// Build constructs the Query object from UpsertQuery.
func (uq *UpsertQuery) Build() *Query {
	keys := getKeys(uq.values)
	placeholders := make([]string, 0, len(keys))
	params := make([]interface{}, 0, len(keys))

	for i, col := range keys {
		placeholders = append(placeholders, uq.builder.db.dialect.Placeholder(i+1))
		params = append(params, uq.values[col])
	}

	// Build base INSERT statement
	query := `INSERT INTO ` + uq.builder.db.dialect.QuoteIdentifier(uq.table) +
		` (` + strings.Join(keys, ", ") + `) ` +
		`VALUES (` + strings.Join(placeholders, ", ") + `)`

	// Add conflict resolution if specified
	if uq.doNothing {
		// PostgreSQL/SQLite: ON CONFLICT DO NOTHING
		// MySQL: needs special handling in dialect
		query += uq.builder.db.dialect.UpsertSQL(uq.table, uq.conflictColumns, nil)
	} else if len(uq.conflictColumns) > 0 || len(uq.updateColumns) > 0 {
		// If no update columns specified, update all except conflict columns
		updateCols := uq.updateColumns
		if len(updateCols) == 0 {
			updateCols = filterKeys(keys, uq.conflictColumns)
		}
		query += uq.builder.db.dialect.UpsertSQL(uq.table, uq.conflictColumns, updateCols)
	}

	// Context priority: query ctx > builder ctx > nil
	ctx := uq.ctx
	if ctx == nil {
		ctx = uq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: params,
		db:     uq.builder.db,
		tx:     uq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the UPSERT query and returns the result.
func (uq *UpsertQuery) Execute() (interface{}, error) {
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
	builder *QueryBuilder
	table   string
	values  map[string]interface{}
	where   []string
	params  []interface{}
	ctx     context.Context // context for this specific query
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
func (uq *UpdateQuery) Set(values map[string]interface{}) *UpdateQuery {
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
func (uq *UpdateQuery) Where(condition interface{}, params ...interface{}) *UpdateQuery {
	switch cond := condition.(type) {
	case string:
		// Legacy string-based WHERE (backward compatible)
		uq.where = append(uq.where, cond)
		uq.params = append(uq.params, params...)

	case Expression:
		// New Expression-based WHERE
		sqlStr, args := cond.Build(uq.builder.db.dialect)
		if sqlStr != "" {
			uq.where = append(uq.where, sqlStr)
			uq.params = append(uq.params, args...)
		}

	default:
		panic("Where() expects string or Expression")
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
func (uq *UpdateQuery) AndWhere(condition interface{}, params ...interface{}) *UpdateQuery {
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
func (uq *UpdateQuery) OrWhere(condition interface{}, params ...interface{}) *UpdateQuery {
	if len(uq.where) == 0 {
		// No existing WHERE clause - just add it.
		return uq.Where(condition, params...)
	}

	// Build the new condition.
	var newSQL string
	var newArgs []interface{}

	switch cond := condition.(type) {
	case string:
		newSQL = cond
		newArgs = params

	case Expression:
		newSQL, newArgs = cond.Build(uq.builder.db.dialect)
		if newSQL == "" {
			// Empty expression - nothing to add.
			return uq
		}

	default:
		panic("OrWhere() expects string or Expression")
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
func (uq *UpdateQuery) Build() *Query {
	// Get sorted keys for deterministic SQL generation
	keys := getKeys(uq.values)

	// Build SET clause with placeholders
	setClauses := make([]string, 0, len(keys))
	setParams := make([]interface{}, 0, len(keys))

	for i, col := range keys {
		setClauses = append(setClauses, col+" = "+uq.builder.db.dialect.Placeholder(i+1))
		setParams = append(setParams, uq.values[col])
	}

	// Build WHERE clause
	whereClause := ""
	whereParams := uq.params
	if len(uq.where) > 0 {
		whereClause = " WHERE " + strings.Join(uq.where, " AND ")

		// Renumber WHERE placeholders for PostgreSQL ($1, $2, etc.)
		if uq.builder.db.dialect.Placeholder(1) != "?" {
			startIndex := len(setParams) + 1
			for i := range whereParams {
				placeholder := uq.builder.db.dialect.Placeholder(startIndex + i)
				whereClause = strings.Replace(whereClause, "?", placeholder, 1)
			}
		}
	}

	// Construct SQL
	query := "UPDATE " + uq.builder.db.dialect.QuoteIdentifier(uq.table) +
		" SET " + strings.Join(setClauses, ", ") + whereClause

	// Combine SET and WHERE parameters
	setParams = append(setParams, whereParams...)

	// Context priority: query ctx > builder ctx > nil
	ctx := uq.ctx
	if ctx == nil {
		ctx = uq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: setParams,
		db:     uq.builder.db,
		tx:     uq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the UPDATE query and returns the result.
func (uq *UpdateQuery) Execute() (interface{}, error) {
	return uq.Build().Execute()
}

// DeleteQuery represents a DELETE query being built.
type DeleteQuery struct {
	builder *QueryBuilder
	table   string
	where   []string
	params  []interface{}
	ctx     context.Context // context for this specific query
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
func (dq *DeleteQuery) Where(condition interface{}, params ...interface{}) *DeleteQuery {
	switch cond := condition.(type) {
	case string:
		// Legacy string-based WHERE (backward compatible)
		dq.where = append(dq.where, cond)
		dq.params = append(dq.params, params...)

	case Expression:
		// New Expression-based WHERE
		sqlStr, args := cond.Build(dq.builder.db.dialect)
		if sqlStr != "" {
			dq.where = append(dq.where, sqlStr)
			dq.params = append(dq.params, args...)
		}

	default:
		panic("Where() expects string or Expression")
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
func (dq *DeleteQuery) AndWhere(condition interface{}, params ...interface{}) *DeleteQuery {
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
func (dq *DeleteQuery) OrWhere(condition interface{}, params ...interface{}) *DeleteQuery {
	if len(dq.where) == 0 {
		// No existing WHERE clause - just add it.
		return dq.Where(condition, params...)
	}

	// Build the new condition.
	var newSQL string
	var newArgs []interface{}

	switch cond := condition.(type) {
	case string:
		newSQL = cond
		newArgs = params

	case Expression:
		newSQL, newArgs = cond.Build(dq.builder.db.dialect)
		if newSQL == "" {
			// Empty expression - nothing to add.
			return dq
		}

	default:
		panic("OrWhere() expects string or Expression")
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
func (dq *DeleteQuery) Build() *Query {
	// Build WHERE clause
	whereClause := ""
	whereParams := dq.params
	if len(dq.where) > 0 {
		whereClause = " WHERE " + strings.Join(dq.where, " AND ")

		// Renumber WHERE placeholders for PostgreSQL ($1, $2, etc.)
		if dq.builder.db.dialect.Placeholder(1) != "?" {
			for i := range whereParams {
				placeholder := dq.builder.db.dialect.Placeholder(i + 1)
				whereClause = strings.Replace(whereClause, "?", placeholder, 1)
			}
		}
	}

	// Construct SQL
	query := "DELETE FROM " + dq.builder.db.dialect.QuoteIdentifier(dq.table) + whereClause

	// Context priority: query ctx > builder ctx > nil
	ctx := dq.ctx
	if ctx == nil {
		ctx = dq.builder.ctx
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
func (dq *DeleteQuery) Execute() (interface{}, error) {
	return dq.Build().Execute()
}

// BatchInsertQuery represents a batch INSERT query being built.
// It allows inserting multiple rows with a single SQL statement for performance.
type BatchInsertQuery struct {
	builder *QueryBuilder
	table   string
	columns []string
	rows    [][]interface{}
	ctx     context.Context // context for this specific query
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
		rows:    make([][]interface{}, 0),
	}
}

// Values adds a row of values to the batch insert.
// The number of values must match the number of columns specified in BatchInsert.
// Panics if the value count doesn't match the column count (fail fast).
func (biq *BatchInsertQuery) Values(values ...interface{}) *BatchInsertQuery {
	if len(values) != len(biq.columns) {
		panic(fmt.Sprintf("BatchInsert: expected %d values, got %d", len(biq.columns), len(values)))
	}
	biq.rows = append(biq.rows, values)
	return biq
}

// ValuesMap adds a row from a map of column names to values.
// Values are extracted in the order of columns specified in BatchInsert.
// Missing columns will have nil values.
func (biq *BatchInsertQuery) ValuesMap(values map[string]interface{}) *BatchInsertQuery {
	row := make([]interface{}, len(biq.columns))
	for i, col := range biq.columns {
		row[i] = values[col]
	}
	return biq.Values(row...)
}

// Build constructs the Query object from BatchInsertQuery.
// Generates SQL in the form: INSERT INTO table (cols) VALUES (?, ?), (?, ?), ...
// Panics if no rows have been added (fail fast).
func (biq *BatchInsertQuery) Build() *Query {
	if len(biq.rows) == 0 {
		panic("BatchInsert: no rows to insert")
	}

	// Build column list with proper quoting
	quotedColumns := make([]string, len(biq.columns))
	for i, col := range biq.columns {
		quotedColumns[i] = biq.builder.db.dialect.QuoteIdentifier(col)
	}

	// Build VALUES clause with placeholders for all rows
	valueClauses := make([]string, len(biq.rows))
	params := make([]interface{}, 0, len(biq.rows)*len(biq.columns))

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

	query := "INSERT INTO " + biq.builder.db.dialect.QuoteIdentifier(biq.table) +
		" (" + strings.Join(quotedColumns, ", ") + ") VALUES " +
		strings.Join(valueClauses, ", ")

	// Context priority: query ctx > builder ctx > nil
	ctx := biq.ctx
	if ctx == nil {
		ctx = biq.builder.ctx
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
func (biq *BatchInsertQuery) Execute() (interface{}, error) {
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
	keyValue interface{}
	values   map[string]interface{}
}

// BatchUpdate creates a batch UPDATE query for the specified table.
// The keyColumn is used to identify which rows to update (typically the primary key).
// Example:
//
//	db.Builder().BatchUpdate("users", "id").
//	    Set(1, map[string]interface{}{"name": "Alice", "status": "active"}).
//	    Set(2, map[string]interface{}{"name": "Bob", "status": "inactive"}).
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
func (buq *BatchUpdateQuery) Set(keyValue interface{}, values map[string]interface{}) *BatchUpdateQuery {
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
// Panics if no updates have been added (fail fast).
func (buq *BatchUpdateQuery) Build() *Query {
	if len(buq.updates) == 0 {
		panic("BatchUpdate: no updates to apply")
	}

	// Collect all key values for WHERE IN clause
	keyValues := make([]interface{}, len(buq.updates))
	for i, update := range buq.updates {
		keyValues[i] = update.keyValue
	}

	// Build CASE-WHEN for each column
	setClauses := make([]string, 0, len(buq.updateColumns))
	params := make([]interface{}, 0)
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

	query := "UPDATE " + buq.builder.db.dialect.QuoteIdentifier(buq.table) +
		" SET " + strings.Join(setClauses, ", ") +
		" WHERE " + buq.builder.db.dialect.QuoteIdentifier(buq.keyColumn) +
		" IN (" + strings.Join(whereInPlaceholders, ", ") + ")"

	// Context priority: query ctx > builder ctx > nil
	ctx := buq.ctx
	if ctx == nil {
		ctx = buq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: params,
		db:     buq.builder.db,
		tx:     buq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the batch UPDATE query and returns the result.
func (buq *BatchUpdateQuery) Execute() (interface{}, error) {
	return buq.Build().Execute()
}
