package core

import (
	"strings"
	"testing"
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

			if q == nil {
				t.Fatal("expected non-nil")
			}
			if !strings.Contains(q.sql, "ORDER BY CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END") {
				t.Errorf("%q does not contain %q", q.sql, "ORDER BY CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END")
			}
			if strings.Contains(q.sql, `"CASE"`) {
				t.Errorf("%q should not contain %q", q.sql, `"CASE"`)
			}
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

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "ORDER BY CASE WHEN priority = ") {
		t.Errorf("%q does not contain %q", q.sql, "ORDER BY CASE WHEN priority = ")
	}
	if !strings.Contains(q.sql, "THEN 0 ELSE 1 END") {
		t.Errorf("%q does not contain %q", q.sql, "THEN 0 ELSE 1 END")
	}
	// Params: "active" (WHERE) + "high" (OrderByExpr)
	if len(q.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(q.params))
	}
	if q.params[0] != "active" {
		t.Errorf("got %v, want %v", q.params[0], "active")
	}
	if q.params[1] != "high" {
		t.Errorf("got %v, want %v", q.params[1], "high")
	}
}

func TestOrderByExpr_PostgreSQL_PlaceholderNumbering(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		Where("status = ?", "active").
		Where("user_id = ?", 42).
		OrderByExpr("CASE WHEN priority = ? THEN 0 ELSE 1 END", "high").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	// WHERE uses $1 and $2, OrderByExpr should use $3
	if !strings.Contains(q.sql, "$1") {
		t.Errorf("%q does not contain %q", q.sql, "$1")
	}
	if !strings.Contains(q.sql, "$2") {
		t.Errorf("%q does not contain %q", q.sql, "$2")
	}
	// Params order: active, 42, high
	if len(q.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(q.params))
	}
	if q.params[2] != "high" {
		t.Errorf("got %v, want %v", q.params[2], "high")
	}
}

func TestOrderByExpr_CombinedWithOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "title", "due_date").From("tasks").
		OrderByExpr("CASE WHEN due_date < CURRENT_DATE THEN 0 ELSE 1 END").
		OrderBy("due_date ASC").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	// Both should be in ORDER BY
	if !strings.Contains(q.sql, "ORDER BY") {
		t.Errorf("%q does not contain %q", q.sql, "ORDER BY")
	}
	if !strings.Contains(q.sql, "CASE WHEN") {
		t.Errorf("%q does not contain %q", q.sql, "CASE WHEN")
	}
	if !strings.Contains(q.sql, `"due_date" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `"due_date" ASC`)
	}
}

func TestOrderByExpr_MultipleExprs(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		OrderByExpr("CASE WHEN urgent = ? THEN 0 ELSE 1 END", true).
		OrderByExpr("COALESCE(due_date, '9999-12-31')").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "CASE WHEN urgent =") {
		t.Errorf("%q does not contain %q", q.sql, "CASE WHEN urgent =")
	}
	if !strings.Contains(q.sql, "COALESCE(due_date") {
		t.Errorf("%q does not contain %q", q.sql, "COALESCE(due_date")
	}
}

func TestOrderByExpr_OnlyExpr_NoRegularOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		OrderByExpr("RANDOM()").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "ORDER BY RANDOM()") {
		t.Errorf("%q does not contain %q", q.sql, "ORDER BY RANDOM()")
	}
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

			if q == nil {
				t.Fatal("expected non-nil")
			}
			if !strings.Contains(q.sql, "GROUP BY "+tt.expr) {
				t.Errorf("%q does not contain %q", q.sql, "GROUP BY "+tt.expr)
			}
		})
	}
}

func TestGroupByExpr_ExtractYear(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("EXTRACT(YEAR FROM order_date) AS year", "COUNT(*)").From("orders").
		GroupByExpr("EXTRACT(YEAR FROM order_date)").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "GROUP BY EXTRACT(YEAR FROM order_date)") {
		t.Errorf("%q does not contain %q", q.sql, "GROUP BY EXTRACT(YEAR FROM order_date)")
	}
}

func TestGroupByExpr_CombinedWithGroupBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("status", "DATE(created_at)", "COUNT(*)").From("orders").
		GroupBy("status").
		GroupByExpr("DATE(created_at)").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, `GROUP BY "status", DATE(created_at)`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "status", DATE(created_at)`)
	}
}

func TestGroupByExpr_WithParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("bucket", "COUNT(*)").From("orders").
		GroupByExpr("CASE WHEN total > ? THEN 'high' ELSE 'low' END", 1000).
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "GROUP BY CASE WHEN total >") {
		t.Errorf("%q does not contain %q", q.sql, "GROUP BY CASE WHEN total >")
	}
	if len(q.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(q.params))
	}
	if q.params[0] != 1000 {
		t.Errorf("got %v, want %v", q.params[0], 1000)
	}
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

	if q == nil {
		t.Fatal("expected non-nil")
	}
	// Params: "active" (WHERE), 5 (HAVING), 10 (OrderByExpr)
	if len(q.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(q.params))
	}
	if q.params[0] != "active" {
		t.Errorf("got %v, want %v", q.params[0], "active")
	}
	if q.params[1] != 5 {
		t.Errorf("got %v, want %v", q.params[1], 5)
	}
	if q.params[2] != 10 {
		t.Errorf("got %v, want %v", q.params[2], 10)
	}
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

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if strings.Contains(q.sql, `"CASE"`) {
		t.Errorf("%q should not contain %q", q.sql, `"CASE"`)
	}
	if !strings.Contains(q.sql, "CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END") {
		t.Errorf("%q does not contain %q", q.sql, "CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END")
	}
	if !strings.Contains(q.sql, `"t"."due_date" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `"t"."due_date" ASC`)
	}
}

// =============================================================================
// OrderBySub — type-safe expressions (CaseWhen builder)
// =============================================================================

func TestOrderBySub_CaseWhen(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "title").From("tasks t").
		OrderBySub(CaseWhen().
			When("t.due_date < CURRENT_DATE", 0).
			When("t.due_date = CURRENT_DATE", 1).
			When("t.due_date IS NULL", 3).
			Else(2)).
		OrderBy("t.due_date ASC").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	// CaseWhen: conditions are raw SQL, THEN results are parameterized
	if !strings.Contains(q.sql, "CASE WHEN t.due_date < CURRENT_DATE THEN ?") {
		t.Errorf("%q does not contain %q", q.sql, "CASE WHEN t.due_date < CURRENT_DATE THEN ?")
	}
	if !strings.Contains(q.sql, "WHEN t.due_date IS NULL THEN ?") {
		t.Errorf("%q does not contain %q", q.sql, "WHEN t.due_date IS NULL THEN ?")
	}
	if !strings.Contains(q.sql, "ELSE ?") {
		t.Errorf("%q does not contain %q", q.sql, "ELSE ?")
	}
	if !strings.Contains(q.sql, `"t"."due_date" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `"t"."due_date" ASC`)
	}
	for _, v := range []interface{}{0, 1, 2, 3} {
		found := false
		for _, p := range q.params {
			if p == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q does not contain %q", q.params, v)
		}
	}
}

func TestOrderBySub_CaseWhenWithParams_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").From("tasks").
		Where("user_id = ?", 42).
		OrderBySub(CaseWhen().
			When("status IN ('DONE','CANCELLED')", 1).
			Else(0)).
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	// WHERE param + CaseWhen params
	if q.params[0] != 42 {
		t.Errorf("got %v, want %v", q.params[0], 42)
	}
	if !strings.Contains(q.sql, "ORDER BY CASE") {
		t.Errorf("%q does not contain %q", q.sql, "ORDER BY CASE")
	}
}

func TestOrderBySub_CombinedWithOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "title").From("tasks t").
		OrderBySub(CaseWhen().
			When("t.status = 'urgent'", 0).
			Else(1)).
		OrderBy("t.created_at DESC").
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	// Regular OrderBy comes first, then Sub expressions
	if !strings.Contains(q.sql, `"t"."created_at" DESC`) {
		t.Errorf("%q does not contain %q", q.sql, `"t"."created_at" DESC`)
	}
	if !strings.Contains(q.sql, "CASE") {
		t.Errorf("%q does not contain %q", q.sql, "CASE")
	}
}

func TestOrderBySub_SimpleCase(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Simple CASE (with column)
	q := qb.Select("id").From("tasks").
		OrderBySub(Case("priority").
			When("high", 0).
			When("medium", 1).
			Else(2)).
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, `ORDER BY CASE "priority"`) {
		t.Errorf("%q does not contain %q", q.sql, `ORDER BY CASE "priority"`)
	}
}

// =============================================================================
// GroupBySub
// =============================================================================

func TestGroupBySub_CaseWhen(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("COUNT(*)").From("tasks").
		GroupBySub(CaseWhen().
			When("priority = 'high'", "critical").
			Else("normal")).
		Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "GROUP BY CASE") {
		t.Errorf("%q does not contain %q", q.sql, "GROUP BY CASE")
	}
}
