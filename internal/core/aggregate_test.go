package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelectQuery_Aggregate_Count tests COUNT(*) aggregate function
func TestSelectQuery_Aggregate_Count(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("COUNT(*) as total").
		From("users")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Equal(t, `SELECT COUNT(*) as total FROM "users"`, q.sql)
	assert.Empty(t, q.params, "COUNT(*) should have no params")
}

// TestSelectQuery_Aggregate_Sum tests SUM(column) aggregate function
func TestSelectQuery_Aggregate_Sum(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("SUM(price) as total_price").
		From("orders")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Equal(t, `SELECT SUM(price) as total_price FROM "orders"`, q.sql)
	assert.Empty(t, q.params)
}

// TestSelectQuery_Aggregate_Multiple tests multiple aggregate functions
func TestSelectQuery_Aggregate_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("COUNT(*) as cnt", "SUM(price) as total", "AVG(price) as avg_price").
		From("orders")

	q := query.Build()
	require.NotNil(t, q)

	// Verify all aggregates are present
	assert.Contains(t, q.sql, `COUNT(*) as cnt`)
	assert.Contains(t, q.sql, `SUM(price) as total`)
	assert.Contains(t, q.sql, `AVG(price) as avg_price`)
	assert.Empty(t, q.params)
}

// TestSelectQuery_Aggregate_MixedColumns tests mixing regular columns with aggregates
func TestSelectQuery_Aggregate_MixedColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*) as message_count").
		From("messages")

	q := query.Build()
	require.NotNil(t, q)

	// Verify column is quoted and aggregate is not
	assert.Contains(t, q.sql, `"user_id"`)
	assert.Contains(t, q.sql, `COUNT(*) as message_count`)
	assert.Empty(t, q.params)
}

// TestSelectQuery_GroupBy_Single tests GROUP BY with single column
func TestSelectQuery_GroupBy_Single(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*) as cnt").
		From("messages").
		GroupBy("user_id")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, `GROUP BY "user_id"`)
	assert.Empty(t, q.params)

	// Verify clause order: SELECT ... FROM ... GROUP BY
	selectIdx := indexOf(q.sql, "SELECT")
	fromIdx := indexOf(q.sql, "FROM")
	groupIdx := indexOf(q.sql, "GROUP BY")
	assert.Less(t, selectIdx, fromIdx, "SELECT should come before FROM")
	assert.Less(t, fromIdx, groupIdx, "FROM should come before GROUP BY")
}

// TestSelectQuery_GroupBy_Multiple tests GROUP BY with multiple columns
func TestSelectQuery_GroupBy_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "status", "COUNT(*) as cnt").
		From("messages").
		GroupBy("user_id", "status")

	q := query.Build()
	require.NotNil(t, q)

	// Verify both columns are in GROUP BY
	assert.Contains(t, q.sql, `GROUP BY "user_id", "status"`)
	assert.Empty(t, q.params)
}

// TestSelectQuery_GroupBy_Chainable tests GroupBy is chainable
func TestSelectQuery_GroupBy_Chainable(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "status", "COUNT(*)").
		From("messages").
		GroupBy("user_id").
		GroupBy("status")

	q := query.Build()
	require.NotNil(t, q)

	// Verify both columns are in GROUP BY
	assert.Contains(t, q.sql, `GROUP BY "user_id", "status"`)
}

// TestSelectQuery_GroupBy_WithTablePrefix tests GROUP BY with table.column format
func TestSelectQuery_GroupBy_WithTablePrefix(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("m.user_id", "COUNT(*)").
		From("messages m").
		GroupBy("m.user_id")

	q := query.Build()
	require.NotNil(t, q)

	// Verify table prefix is quoted correctly: "m"."user_id"
	assert.Contains(t, q.sql, `GROUP BY "m"."user_id"`)
}

// TestSelectQuery_Having_String tests HAVING clause with string condition
func TestSelectQuery_Having_String(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*) as cnt").
		From("messages").
		GroupBy("user_id").
		Having("COUNT(*) > ?", 100)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, `HAVING COUNT(*) > $1`)
	assert.Equal(t, []interface{}{100}, q.params)

	// Verify clause order: GROUP BY ... HAVING
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	assert.Less(t, groupIdx, havingIdx, "GROUP BY should come before HAVING")
}

// TestSelectQuery_Having_Multiple tests multiple HAVING clauses (combined with AND)
func TestSelectQuery_Having_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*) as cnt", "SUM(size) as total_size").
		From("messages").
		GroupBy("user_id").
		Having("COUNT(*) > ?", 100).
		Having("SUM(size) < ?", 10000)

	q := query.Build()
	require.NotNil(t, q)

	// Verify both conditions are combined with AND
	assert.Contains(t, q.sql, `HAVING COUNT(*) > $1 AND SUM(size) < $2`)
	assert.Equal(t, []interface{}{100, 10000}, q.params)
}

// TestSelectQuery_Having_Expression tests HAVING with Expression
// Note: Expressions are designed for column comparisons, not aggregate functions.
// For aggregate functions in HAVING, use string-based conditions.
func TestSelectQuery_Having_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Using GreaterThan expression with regular column (not aggregate)
	// For aggregates, use string-based HAVING: Having("COUNT(*) > ?", 100)
	expr := GreaterThan("user_id", 100)

	query := qb.Select("user_id", "COUNT(*) as cnt").
		From("messages").
		GroupBy("user_id").
		Having(expr)

	q := query.Build()
	require.NotNil(t, q)

	// Verify HAVING clause with column expression
	assert.Contains(t, q.sql, `HAVING "user_id">$1`)
	assert.Equal(t, []interface{}{100}, q.params)
}

// TestSelectQuery_GroupBy_Having_Combined tests complete GROUP BY + HAVING query
func TestSelectQuery_GroupBy_Having_Combined(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	tests := []struct {
		name     string
		setup    func() *SelectQuery
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name: "Basic GROUP BY + HAVING",
			setup: func() *SelectQuery {
				return qb.Select("user_id", "COUNT(*) as cnt").
					From("messages").
					GroupBy("user_id").
					Having("COUNT(*) > ?", 100)
			},
			wantSQL:  `SELECT "user_id", COUNT(*) as cnt FROM "messages" GROUP BY "user_id" HAVING COUNT(*) > $1`,
			wantArgs: []interface{}{100},
		},
		{
			name: "Multiple columns GROUP BY",
			setup: func() *SelectQuery {
				return qb.Select("user_id", "status", "COUNT(*)").
					From("messages").
					GroupBy("user_id", "status").
					Having("COUNT(*) > ?", 50)
			},
			wantSQL:  `SELECT "user_id", "status", COUNT(*) FROM "messages" GROUP BY "user_id", "status" HAVING COUNT(*) > $1`,
			wantArgs: []interface{}{50},
		},
		{
			name: "Multiple HAVING conditions",
			setup: func() *SelectQuery {
				return qb.Select("user_id", "COUNT(*)", "AVG(size)").
					From("messages").
					GroupBy("user_id").
					Having("COUNT(*) > ?", 100).
					Having("AVG(size) < ?", 2048)
			},
			wantSQL:  `SELECT "user_id", COUNT(*), AVG(size) FROM "messages" GROUP BY "user_id" HAVING COUNT(*) > $1 AND AVG(size) < $2`,
			wantArgs: []interface{}{100, 2048},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := tt.setup()
			q := query.Build()
			require.NotNil(t, q)

			assert.Equal(t, tt.wantSQL, q.sql)
			assert.Equal(t, tt.wantArgs, q.params)
		})
	}
}

// TestSelectQuery_Aggregate_WithJoin tests aggregates with JOIN (Phase 1 feature)
func TestSelectQuery_Aggregate_WithJoin(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("u.name", "COUNT(m.id) as message_count").
		From("users u").
		InnerJoin("messages m", "m.user_id = u.id").
		GroupBy("u.name").
		Having("COUNT(m.id) > ?", 10)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure with JOIN
	assert.Contains(t, q.sql, `INNER JOIN "messages" AS "m"`)
	assert.Contains(t, q.sql, `GROUP BY "u"."name"`)
	assert.Contains(t, q.sql, `HAVING COUNT(m.id) > $1`)
	assert.Equal(t, []interface{}{10}, q.params)

	// Verify clause order: FROM ... JOIN ... GROUP BY ... HAVING
	fromIdx := indexOf(q.sql, "FROM")
	joinIdx := indexOf(q.sql, "INNER JOIN")
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	assert.Less(t, fromIdx, joinIdx)
	assert.Less(t, joinIdx, groupIdx)
	assert.Less(t, groupIdx, havingIdx)
}

// TestSelectQuery_Aggregate_WithOrderBy tests aggregates with ORDER BY (Phase 2 feature)
func TestSelectQuery_Aggregate_WithOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*) as cnt").
		From("messages").
		GroupBy("user_id").
		Having("COUNT(*) > ?", 100).
		OrderBy("cnt DESC").
		Limit(10)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure with ORDER BY and LIMIT
	assert.Contains(t, q.sql, `GROUP BY "user_id"`)
	assert.Contains(t, q.sql, `HAVING COUNT(*) > $1`)
	assert.Contains(t, q.sql, `ORDER BY "cnt" DESC`)
	assert.Contains(t, q.sql, `LIMIT 10`)
	assert.Equal(t, []interface{}{100}, q.params)

	// Verify clause order: GROUP BY ... HAVING ... ORDER BY ... LIMIT
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	orderIdx := indexOf(q.sql, "ORDER BY")
	limitIdx := indexOf(q.sql, "LIMIT")
	assert.Less(t, groupIdx, havingIdx)
	assert.Less(t, havingIdx, orderIdx)
	assert.Less(t, orderIdx, limitIdx)
}

// TestSelectQuery_Aggregate_CompleteQuery tests all features combined (JOIN + WHERE + GROUP BY + HAVING + ORDER BY + LIMIT)
func TestSelectQuery_Aggregate_CompleteQuery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("u.name", "COUNT(m.id) as message_count", "SUM(m.size) as total_size").
		From("users u").
		InnerJoin("messages m", "m.user_id = u.id").
		Where("m.status = ?", 1).
		GroupBy("u.name").
		Having("COUNT(m.id) > ?", 100).
		OrderBy("message_count DESC").
		Limit(50)

	q := query.Build()
	require.NotNil(t, q)

	// Verify complete SQL structure
	expectedSQL := `SELECT "u"."name", COUNT(m.id) as message_count, SUM(m.size) as total_size ` +
		`FROM "users" AS "u" INNER JOIN "messages" AS "m" ON m.user_id = u.id ` +
		`WHERE m.status = $1 ` +
		`GROUP BY "u"."name" ` +
		`HAVING COUNT(m.id) > $2 ` +
		`ORDER BY "message_count" DESC ` +
		`LIMIT 50`
	assert.Equal(t, expectedSQL, q.sql)
	assert.Equal(t, []interface{}{1, 100}, q.params)

	// Verify correct clause order
	fromIdx := indexOf(q.sql, "FROM")
	joinIdx := indexOf(q.sql, "INNER JOIN")
	whereIdx := indexOf(q.sql, "WHERE")
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	orderIdx := indexOf(q.sql, "ORDER BY")
	limitIdx := indexOf(q.sql, "LIMIT")

	assert.Less(t, fromIdx, joinIdx, "FROM before JOIN")
	assert.Less(t, joinIdx, whereIdx, "JOIN before WHERE")
	assert.Less(t, whereIdx, groupIdx, "WHERE before GROUP BY")
	assert.Less(t, groupIdx, havingIdx, "GROUP BY before HAVING")
	assert.Less(t, havingIdx, orderIdx, "HAVING before ORDER BY")
	assert.Less(t, orderIdx, limitIdx, "ORDER BY before LIMIT")
}

// TestSelectQuery_Aggregate_PostgreSQL_Quoting tests PostgreSQL-specific quoting for aggregates
func TestSelectQuery_Aggregate_PostgreSQL_Quoting(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*)").
		From("messages").
		GroupBy("user_id").
		Having("COUNT(*) > ?", 100)

	q := query.Build()
	require.NotNil(t, q)

	// PostgreSQL uses double quotes for identifiers and $1 for placeholders
	assert.Contains(t, q.sql, `"user_id"`)
	assert.Contains(t, q.sql, `"messages"`)
	assert.Contains(t, q.sql, `$1`)
}

// TestSelectQuery_Aggregate_MySQL_Quoting tests MySQL-specific quoting for aggregates
func TestSelectQuery_Aggregate_MySQL_Quoting(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*)").
		From("messages").
		GroupBy("user_id").
		Having("COUNT(*) > ?", 100)

	q := query.Build()
	require.NotNil(t, q)

	// MySQL uses backticks for identifiers and ? for placeholders
	assert.Contains(t, q.sql, "`user_id`")
	assert.Contains(t, q.sql, "`messages`")
	assert.Contains(t, q.sql, "?")
}

// TestSelectQuery_Aggregate_SQLite_Quoting tests SQLite-specific quoting for aggregates
func TestSelectQuery_Aggregate_SQLite_Quoting(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*)").
		From("messages").
		GroupBy("user_id").
		Having("COUNT(*) > ?", 100)

	q := query.Build()
	require.NotNil(t, q)

	// SQLite uses double quotes for identifiers and ? for placeholders
	assert.Contains(t, q.sql, `"user_id"`)
	assert.Contains(t, q.sql, `"messages"`)
	assert.Contains(t, q.sql, "?")
}

// TestSelectQuery_GroupBy_NoAggregate tests GROUP BY without aggregate (valid but unusual)
func TestSelectQuery_GroupBy_NoAggregate(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id").
		From("messages").
		GroupBy("user_id")

	q := query.Build()
	require.NotNil(t, q)

	// Valid SQL: SELECT DISTINCT-like behavior
	assert.Equal(t, `SELECT "user_id" FROM "messages" GROUP BY "user_id"`, q.sql)
}

// TestSelectQuery_Having_WithWhere tests HAVING combined with WHERE (different filters)
func TestSelectQuery_Having_WithWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*)").
		From("messages").
		Where("status = ?", 1). // Filters rows BEFORE grouping
		GroupBy("user_id").
		Having("COUNT(*) > ?", 100) // Filters groups AFTER aggregation

	q := query.Build()
	require.NotNil(t, q)

	// Verify WHERE comes before GROUP BY, HAVING comes after
	assert.Contains(t, q.sql, `WHERE status = $1`)
	assert.Contains(t, q.sql, `GROUP BY "user_id"`)
	assert.Contains(t, q.sql, `HAVING COUNT(*) > $2`)
	assert.Equal(t, []interface{}{1, 100}, q.params)

	// Verify clause order
	whereIdx := indexOf(q.sql, "WHERE")
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	assert.Less(t, whereIdx, groupIdx, "WHERE before GROUP BY")
	assert.Less(t, groupIdx, havingIdx, "GROUP BY before HAVING")
}
