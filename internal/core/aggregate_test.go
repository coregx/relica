package core

import (
	"strings"
	"testing"
)

// TestSelectQuery_Aggregate_Count tests COUNT(*) aggregate function
func TestSelectQuery_Aggregate_Count(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("COUNT(*) as total").
		From("users")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if q.sql != `SELECT COUNT(*) as total FROM "users"` {
		t.Errorf("got %v, want %v", q.sql, `SELECT COUNT(*) as total FROM "users"`)
	}
	if len(q.params) != 0 {
		t.Errorf("COUNT(*) should have no params: got %d", len(q.params))
	}
}

// TestSelectQuery_Aggregate_Sum tests SUM(column) aggregate function
func TestSelectQuery_Aggregate_Sum(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("SUM(price) as total_price").
		From("orders")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if q.sql != `SELECT SUM(price) as total_price FROM "orders"` {
		t.Errorf("got %v, want %v", q.sql, `SELECT SUM(price) as total_price FROM "orders"`)
	}
	if len(q.params) != 0 {
		t.Errorf("expected empty params, got %d", len(q.params))
	}
}

// TestSelectQuery_Aggregate_Multiple tests multiple aggregate functions
func TestSelectQuery_Aggregate_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("COUNT(*) as cnt", "SUM(price) as total", "AVG(price) as avg_price").
		From("orders")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify all aggregates are present
	if !strings.Contains(q.sql, `COUNT(*) as cnt`) {
		t.Errorf("%q does not contain %q", q.sql, `COUNT(*) as cnt`)
	}
	if !strings.Contains(q.sql, `SUM(price) as total`) {
		t.Errorf("%q does not contain %q", q.sql, `SUM(price) as total`)
	}
	if !strings.Contains(q.sql, `AVG(price) as avg_price`) {
		t.Errorf("%q does not contain %q", q.sql, `AVG(price) as avg_price`)
	}
	if len(q.params) != 0 {
		t.Errorf("expected empty params, got %d", len(q.params))
	}
}

// TestSelectQuery_Aggregate_MixedColumns tests mixing regular columns with aggregates
func TestSelectQuery_Aggregate_MixedColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*) as message_count").
		From("messages")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify column is quoted and aggregate is not
	if !strings.Contains(q.sql, `"user_id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"user_id"`)
	}
	if !strings.Contains(q.sql, `COUNT(*) as message_count`) {
		t.Errorf("%q does not contain %q", q.sql, `COUNT(*) as message_count`)
	}
	if len(q.params) != 0 {
		t.Errorf("expected empty params, got %d", len(q.params))
	}
}

// TestSelectQuery_GroupBy_Single tests GROUP BY with single column
func TestSelectQuery_GroupBy_Single(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "COUNT(*) as cnt").
		From("messages").
		GroupBy("user_id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, `GROUP BY "user_id"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "user_id"`)
	}
	if len(q.params) != 0 {
		t.Errorf("expected empty params, got %d", len(q.params))
	}

	// Verify clause order: SELECT ... FROM ... GROUP BY
	selectIdx := indexOf(q.sql, "SELECT")
	fromIdx := indexOf(q.sql, "FROM")
	groupIdx := indexOf(q.sql, "GROUP BY")
	if selectIdx >= fromIdx {
		t.Errorf("expected SELECT before FROM")
	}
	if fromIdx >= groupIdx {
		t.Errorf("expected FROM before GROUP BY")
	}
}

// TestSelectQuery_GroupBy_Multiple tests GROUP BY with multiple columns
func TestSelectQuery_GroupBy_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id", "status", "COUNT(*) as cnt").
		From("messages").
		GroupBy("user_id", "status")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify both columns are in GROUP BY
	if !strings.Contains(q.sql, `GROUP BY "user_id", "status"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "user_id", "status"`)
	}
	if len(q.params) != 0 {
		t.Errorf("expected empty params, got %d", len(q.params))
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify both columns are in GROUP BY
	if !strings.Contains(q.sql, `GROUP BY "user_id", "status"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "user_id", "status"`)
	}
}

// TestSelectQuery_GroupBy_WithTablePrefix tests GROUP BY with table.column format
func TestSelectQuery_GroupBy_WithTablePrefix(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("m.user_id", "COUNT(*)").
		From("messages m").
		GroupBy("m.user_id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify table prefix is quoted correctly: "m"."user_id"
	if !strings.Contains(q.sql, `GROUP BY "m"."user_id"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "m"."user_id"`)
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, `HAVING COUNT(*) > $1`) {
		t.Errorf("%q does not contain %q", q.sql, `HAVING COUNT(*) > $1`)
	}
	want := []interface{}{100}
	if len(q.params) != len(want) || q.params[0] != want[0] {
		t.Errorf("got %v, want %v", q.params, want)
	}

	// Verify clause order: GROUP BY ... HAVING
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	if groupIdx >= havingIdx {
		t.Errorf("expected GROUP BY before HAVING")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify both conditions are combined with AND
	if !strings.Contains(q.sql, `HAVING COUNT(*) > $1 AND SUM(size) < $2`) {
		t.Errorf("%q does not contain %q", q.sql, `HAVING COUNT(*) > $1 AND SUM(size) < $2`)
	}
	want := []interface{}{100, 10000}
	if len(q.params) != 2 || q.params[0] != want[0] || q.params[1] != want[1] {
		t.Errorf("got %v, want %v", q.params, want)
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify HAVING clause with column expression
	if !strings.Contains(q.sql, `HAVING "user_id" > $1`) {
		t.Errorf("%q does not contain %q", q.sql, `HAVING "user_id" > $1`)
	}
	want := []interface{}{100}
	if len(q.params) != 1 || q.params[0] != want[0] {
		t.Errorf("got %v, want %v", q.params, want)
	}
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
			if q == nil {
				t.Fatal("expected non-nil")
			}

			if q.sql != tt.wantSQL {
				t.Errorf("got %v, want %v", q.sql, tt.wantSQL)
			}
			if len(q.params) != len(tt.wantArgs) {
				t.Errorf("got %v, want %v", q.params, tt.wantArgs)
			} else {
				for i := range tt.wantArgs {
					if q.params[i] != tt.wantArgs[i] {
						t.Errorf("param[%d]: got %v, want %v", i, q.params[i], tt.wantArgs[i])
					}
				}
			}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure with JOIN
	if !strings.Contains(q.sql, `INNER JOIN "messages" AS "m"`) {
		t.Errorf("%q does not contain %q", q.sql, `INNER JOIN "messages" AS "m"`)
	}
	if !strings.Contains(q.sql, `GROUP BY "u"."name"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "u"."name"`)
	}
	if !strings.Contains(q.sql, `HAVING COUNT(m.id) > $1`) {
		t.Errorf("%q does not contain %q", q.sql, `HAVING COUNT(m.id) > $1`)
	}
	want := []interface{}{10}
	if len(q.params) != 1 || q.params[0] != want[0] {
		t.Errorf("got %v, want %v", q.params, want)
	}

	// Verify clause order: FROM ... JOIN ... GROUP BY ... HAVING
	fromIdx := indexOf(q.sql, "FROM")
	joinIdx := indexOf(q.sql, "INNER JOIN")
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	if fromIdx >= joinIdx {
		t.Errorf("expected FROM before INNER JOIN")
	}
	if joinIdx >= groupIdx {
		t.Errorf("expected INNER JOIN before GROUP BY")
	}
	if groupIdx >= havingIdx {
		t.Errorf("expected GROUP BY before HAVING")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure with ORDER BY and LIMIT
	if !strings.Contains(q.sql, `GROUP BY "user_id"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "user_id"`)
	}
	if !strings.Contains(q.sql, `HAVING COUNT(*) > $1`) {
		t.Errorf("%q does not contain %q", q.sql, `HAVING COUNT(*) > $1`)
	}
	if !strings.Contains(q.sql, `ORDER BY "cnt" DESC`) {
		t.Errorf("%q does not contain %q", q.sql, `ORDER BY "cnt" DESC`)
	}
	if !strings.Contains(q.sql, `LIMIT 10`) {
		t.Errorf("%q does not contain %q", q.sql, `LIMIT 10`)
	}
	want := []interface{}{100}
	if len(q.params) != 1 || q.params[0] != want[0] {
		t.Errorf("got %v, want %v", q.params, want)
	}

	// Verify clause order: GROUP BY ... HAVING ... ORDER BY ... LIMIT
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	orderIdx := indexOf(q.sql, "ORDER BY")
	limitIdx := indexOf(q.sql, "LIMIT")
	if groupIdx >= havingIdx {
		t.Errorf("expected GROUP BY before HAVING")
	}
	if havingIdx >= orderIdx {
		t.Errorf("expected HAVING before ORDER BY")
	}
	if orderIdx >= limitIdx {
		t.Errorf("expected ORDER BY before LIMIT")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify complete SQL structure
	expectedSQL := `SELECT "u"."name", COUNT(m.id) as message_count, SUM(m.size) as total_size ` +
		`FROM "users" AS "u" INNER JOIN "messages" AS "m" ON m.user_id = u.id ` +
		`WHERE m.status = $1 ` +
		`GROUP BY "u"."name" ` +
		`HAVING COUNT(m.id) > $2 ` +
		`ORDER BY "message_count" DESC ` +
		`LIMIT 50`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}
	want := []interface{}{1, 100}
	if len(q.params) != 2 || q.params[0] != want[0] || q.params[1] != want[1] {
		t.Errorf("got %v, want %v", q.params, want)
	}

	// Verify correct clause order
	fromIdx := indexOf(q.sql, "FROM")
	joinIdx := indexOf(q.sql, "INNER JOIN")
	whereIdx := indexOf(q.sql, "WHERE")
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	orderIdx := indexOf(q.sql, "ORDER BY")
	limitIdx := indexOf(q.sql, "LIMIT")

	if fromIdx >= joinIdx {
		t.Errorf("FROM before JOIN")
	}
	if joinIdx >= whereIdx {
		t.Errorf("JOIN before WHERE")
	}
	if whereIdx >= groupIdx {
		t.Errorf("WHERE before GROUP BY")
	}
	if groupIdx >= havingIdx {
		t.Errorf("GROUP BY before HAVING")
	}
	if havingIdx >= orderIdx {
		t.Errorf("HAVING before ORDER BY")
	}
	if orderIdx >= limitIdx {
		t.Errorf("ORDER BY before LIMIT")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// PostgreSQL uses double quotes for identifiers and $1 for placeholders
	if !strings.Contains(q.sql, `"user_id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"user_id"`)
	}
	if !strings.Contains(q.sql, `"messages"`) {
		t.Errorf("%q does not contain %q", q.sql, `"messages"`)
	}
	if !strings.Contains(q.sql, `$1`) {
		t.Errorf("%q does not contain %q", q.sql, `$1`)
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// MySQL uses backticks for identifiers and ? for placeholders
	if !strings.Contains(q.sql, "`user_id`") {
		t.Errorf("%q does not contain %q", q.sql, "`user_id`")
	}
	if !strings.Contains(q.sql, "`messages`") {
		t.Errorf("%q does not contain %q", q.sql, "`messages`")
	}
	if !strings.Contains(q.sql, "?") {
		t.Errorf("%q does not contain %q", q.sql, "?")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// SQLite uses double quotes for identifiers and ? for placeholders
	if !strings.Contains(q.sql, `"user_id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"user_id"`)
	}
	if !strings.Contains(q.sql, `"messages"`) {
		t.Errorf("%q does not contain %q", q.sql, `"messages"`)
	}
	if !strings.Contains(q.sql, "?") {
		t.Errorf("%q does not contain %q", q.sql, "?")
	}
}

// TestSelectQuery_GroupBy_NoAggregate tests GROUP BY without aggregate (valid but unusual)
func TestSelectQuery_GroupBy_NoAggregate(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("user_id").
		From("messages").
		GroupBy("user_id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Valid SQL: SELECT DISTINCT-like behavior
	want := `SELECT "user_id" FROM "messages" GROUP BY "user_id"`
	if q.sql != want {
		t.Errorf("got %v, want %v", q.sql, want)
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify WHERE comes before GROUP BY, HAVING comes after
	if !strings.Contains(q.sql, `WHERE status = $1`) {
		t.Errorf("%q does not contain %q", q.sql, `WHERE status = $1`)
	}
	if !strings.Contains(q.sql, `GROUP BY "user_id"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "user_id"`)
	}
	if !strings.Contains(q.sql, `HAVING COUNT(*) > $2`) {
		t.Errorf("%q does not contain %q", q.sql, `HAVING COUNT(*) > $2`)
	}
	want := []interface{}{1, 100}
	if len(q.params) != 2 || q.params[0] != want[0] || q.params[1] != want[1] {
		t.Errorf("got %v, want %v", q.params, want)
	}

	// Verify clause order
	whereIdx := indexOf(q.sql, "WHERE")
	groupIdx := indexOf(q.sql, "GROUP BY")
	havingIdx := indexOf(q.sql, "HAVING")
	if whereIdx >= groupIdx {
		t.Errorf("expected WHERE before GROUP BY")
	}
	if groupIdx >= havingIdx {
		t.Errorf("expected GROUP BY before HAVING")
	}
}
