package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// OrderByExpr
// =============================================================================

func TestOrderByExpr_CaseWhen(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
	}{
		{"postgres", "postgres"},
		{"mysql", "mysql"},
		{"sqlite", "sqlite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}

			q := qb.Select("id", "title").From("tasks t").
				OrderByExpr("CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END").
				Build()

			require.NotNil(t, q)
			assert.Contains(t, q.sql, "ORDER BY CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END")
			assert.NotContains(t, q.sql, `"CASE"`)
		})
	}
}

func TestOrderByExpr_WithParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		Where("status = ?", "active").
		OrderByExpr("CASE WHEN priority = ? THEN 0 ELSE 1 END", "high").
		Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, "ORDER BY CASE WHEN priority = ")
	assert.Contains(t, q.sql, "THEN 0 ELSE 1 END")
	// Params: "active" (WHERE) + "high" (OrderByExpr)
	assert.Len(t, q.params, 2)
	assert.Equal(t, "active", q.params[0])
	assert.Equal(t, "high", q.params[1])
}

func TestOrderByExpr_PostgreSQL_PlaceholderNumbering(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		Where("status = ?", "active").
		Where("user_id = ?", 42).
		OrderByExpr("CASE WHEN priority = ? THEN 0 ELSE 1 END", "high").
		Build()

	require.NotNil(t, q)
	// WHERE uses $1 and $2, OrderByExpr should use $3
	assert.Contains(t, q.sql, "$1")
	assert.Contains(t, q.sql, "$2")
	// Params order: active, 42, high
	assert.Len(t, q.params, 3)
	assert.Equal(t, "high", q.params[2])
}

func TestOrderByExpr_CombinedWithOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "title", "due_date").From("tasks").
		OrderByExpr("CASE WHEN due_date < CURRENT_DATE THEN 0 ELSE 1 END").
		OrderBy("due_date ASC").
		Build()

	require.NotNil(t, q)
	// Both should be in ORDER BY
	assert.Contains(t, q.sql, "ORDER BY")
	assert.Contains(t, q.sql, "CASE WHEN")
	assert.Contains(t, q.sql, `"due_date" ASC`)
}

func TestOrderByExpr_MultipleExprs(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		OrderByExpr("CASE WHEN urgent = ? THEN 0 ELSE 1 END", true).
		OrderByExpr("COALESCE(due_date, '9999-12-31')").
		Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, "CASE WHEN urgent =")
	assert.Contains(t, q.sql, "COALESCE(due_date")
}

func TestOrderByExpr_OnlyExpr_NoRegularOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		OrderByExpr("RANDOM()").
		Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, "ORDER BY RANDOM()")
}

// =============================================================================
// GroupByExpr
// =============================================================================

func TestGroupByExpr_DateFunction(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		expr    string
	}{
		{"postgres", "postgres", "DATE(created_at)"},
		{"mysql", "mysql", "DATE(created_at)"},
		{"sqlite", "sqlite", "DATE(created_at)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}

			q := qb.Select("DATE(created_at) AS day", "COUNT(*)").From("orders").
				GroupByExpr(tt.expr).
				Build()

			require.NotNil(t, q)
			assert.Contains(t, q.sql, "GROUP BY "+tt.expr)
		})
	}
}

func TestGroupByExpr_ExtractYear(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("EXTRACT(YEAR FROM order_date) AS year", "COUNT(*)").From("orders").
		GroupByExpr("EXTRACT(YEAR FROM order_date)").
		Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, "GROUP BY EXTRACT(YEAR FROM order_date)")
}

func TestGroupByExpr_CombinedWithGroupBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("status", "DATE(created_at)", "COUNT(*)").From("orders").
		GroupBy("status").
		GroupByExpr("DATE(created_at)").
		Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, `GROUP BY "status", DATE(created_at)`)
}

func TestGroupByExpr_WithParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("bucket", "COUNT(*)").From("orders").
		GroupByExpr("CASE WHEN total > ? THEN 'high' ELSE 'low' END", 1000).
		Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, "GROUP BY CASE WHEN total >")
	assert.Len(t, q.params, 1)
	assert.Equal(t, 1000, q.params[0])
}

// =============================================================================
// Combined: OrderByExpr + GroupByExpr + WHERE params
// =============================================================================

func TestCombined_OrderByExpr_GroupByExpr_WHERE(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("DATE(created_at) AS day", "COUNT(*) AS cnt").
		From("tasks").
		Where("status = ?", "active").
		GroupByExpr("DATE(created_at)").
		Having("COUNT(*) > ?", 5).
		OrderByExpr("CASE WHEN COUNT(*) > ? THEN 0 ELSE 1 END", 10).
		OrderBy("day DESC").
		Build()

	require.NotNil(t, q)
	// Params: "active" (WHERE), 5 (HAVING), 10 (OrderByExpr)
	assert.Len(t, q.params, 3)
	assert.Equal(t, "active", q.params[0])
	assert.Equal(t, 5, q.params[1])
	assert.Equal(t, 10, q.params[2])
}

// =============================================================================
// Issue #34 — exact reproduction
// =============================================================================

func TestIssue34_CaseWhenOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "title", "due_date").From("tasks t").
		OrderByExpr("CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END").
		OrderBy("t.due_date ASC").
		Build()

	require.NotNil(t, q)
	// Must NOT contain quoted "CASE"
	assert.NotContains(t, q.sql, `"CASE"`)
	// Must contain raw CASE expression
	assert.Contains(t, q.sql, "CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END")
	// Regular OrderBy still quoted
	assert.Contains(t, q.sql, `"t"."due_date" ASC`)
}
