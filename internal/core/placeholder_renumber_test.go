package core

import (
	"fmt"
	"strings"
	"testing"
)

// verifyPlaceholderSequence checks that for PostgreSQL, $1..$N are all present
// and no raw ? remain.
func verifyPlaceholderSequence(t *testing.T, sql string, argCount int) {
	t.Helper()
	if strings.Contains(sql, "?") {
		t.Errorf("raw ? remaining in PostgreSQL SQL: %s", sql)
	}
	for i := 1; i <= argCount; i++ {
		ph := fmt.Sprintf("$%d", i)
		if !strings.Contains(sql, ph) {
			t.Errorf("missing %s in SQL: %s", ph, sql)
		}
	}
}

// TestSubquery_In_PostgreSQL verifies that IN(subquery) placeholders are
// renumbered sequentially with the outer query — no $1 collision.
func TestSubquery_In_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").
		Where("status = ?", "done").
		Where("total > ?", 100)

	sq := qb.Select().From("users").
		Where("active = ?", true).
		Where(In("id", sub.AsExpression()))

	q := sq.Build()

	// Expected args order: true (outer WHERE), done (sub WHERE 1), 100 (sub WHERE 2)
	verifyPlaceholderSequence(t, q.sql, 3)

	if len(q.params) != 3 {
		t.Fatalf("expected 3 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != true {
		t.Errorf("params[0]: got %v, want true", q.params[0])
	}
	if q.params[1] != "done" {
		t.Errorf("params[1]: got %v, want \"done\"", q.params[1])
	}
	if q.params[2] != 100 {
		t.Errorf("params[2]: got %v, want 100", q.params[2])
	}

	// Outer WHERE must reference $1, subquery must reference $2 and $3
	if !strings.Contains(q.sql, "active = $1") {
		t.Errorf("outer WHERE should use $1: %s", q.sql)
	}
	if !strings.Contains(q.sql, "status = $2") {
		t.Errorf("sub WHERE 1 should use $2: %s", q.sql)
	}
	if !strings.Contains(q.sql, "total > $3") {
		t.Errorf("sub WHERE 2 should use $3: %s", q.sql)
	}
}

// TestSubquery_Exists_PostgreSQL verifies EXISTS(subquery) placeholder renumbering.
func TestSubquery_Exists_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("1").From("orders").
		Where("user_id = ? AND status = ?", 42, "active")

	sq := qb.Select().From("users").
		Where("age > ?", 18).
		Where(Exists(sub.AsExpression()))

	q := sq.Build()

	// Expected args order: 18 (outer WHERE), 42, "active" (sub WHERE)
	verifyPlaceholderSequence(t, q.sql, 3)

	if len(q.params) != 3 {
		t.Fatalf("expected 3 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != 18 {
		t.Errorf("params[0]: got %v, want 18", q.params[0])
	}
	if q.params[2] != "active" {
		t.Errorf("params[2]: got %v, want \"active\"", q.params[2])
	}
}

// TestGroupByExpr_PostgreSQL verifies that ? in GroupByExpr is renumbered.
func TestGroupByExpr_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("status", "COUNT(*)").From("users").
		Where("active = ?", true).
		GroupByExpr("CASE WHEN age > ? THEN 'senior' ELSE 'junior' END", 60)

	q := sq.Build()

	// Expected args order: true (WHERE), 60 (GroupByExpr)
	verifyPlaceholderSequence(t, q.sql, 2)

	if len(q.params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != true {
		t.Errorf("params[0]: got %v, want true", q.params[0])
	}
	if q.params[1] != 60 {
		t.Errorf("params[1]: got %v, want 60", q.params[1])
	}

	if !strings.Contains(q.sql, "GROUP BY CASE WHEN age > $2") {
		t.Errorf("GroupByExpr should reference $2: %s", q.sql)
	}
}

// TestOrderByExpr_PostgreSQL verifies that ? in OrderByExpr is renumbered.
func TestOrderByExpr_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id", "name").From("users").
		Where("active = ?", true).
		OrderByExpr("CASE WHEN priority = ? THEN 0 ELSE 1 END", "high")

	q := sq.Build()

	// Expected args order: true (WHERE), "high" (OrderByExpr)
	verifyPlaceholderSequence(t, q.sql, 2)

	if len(q.params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != true {
		t.Errorf("params[0]: got %v, want true", q.params[0])
	}
	if q.params[1] != "high" {
		t.Errorf("params[1]: got %v, want \"high\"", q.params[1])
	}

	if !strings.Contains(q.sql, "CASE WHEN priority = $2") {
		t.Errorf("OrderByExpr should reference $2: %s", q.sql)
	}
}

// TestGroupByExpr_OrderByExpr_Combined_PostgreSQL verifies that GroupByExpr
// and OrderByExpr params are correctly numbered after WHERE and HAVING.
func TestGroupByExpr_OrderByExpr_Combined_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("status", "COUNT(*)").From("users").
		Where("active = ?", true).                                            // $1
		Having("COUNT(*) > ?", 5).                                            // $2
		GroupByExpr("CASE WHEN age > ? THEN 'senior' ELSE 'junior' END", 60). // $3
		OrderByExpr("CASE WHEN priority = ? THEN 0 ELSE 1 END", "high")       // $4

	q := sq.Build()

	verifyPlaceholderSequence(t, q.sql, 4)

	if len(q.params) != 4 {
		t.Fatalf("expected 4 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != true {
		t.Errorf("params[0]: got %v, want true", q.params[0])
	}
	if q.params[1] != 5 {
		t.Errorf("params[1]: got %v, want 5", q.params[1])
	}
	if q.params[2] != 60 {
		t.Errorf("params[2]: got %v, want 60", q.params[2])
	}
	if q.params[3] != "high" {
		t.Errorf("params[3]: got %v, want \"high\"", q.params[3])
	}
}

// TestMySQL_SubqueryIn_Unchanged verifies MySQL queries use ? natively — no $N.
func TestMySQL_SubqueryIn_Unchanged(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("status = ?", "done")
	sq := qb.Select().From("users").
		Where("active = ?", true).
		Where(In("id", sub.AsExpression()))

	q := sq.Build()

	if strings.Contains(q.sql, "$") {
		t.Errorf("MySQL SQL should not contain $N: %s", q.sql)
	}
	if !strings.Contains(q.sql, "?") {
		t.Errorf("MySQL SQL should use ? placeholders: %s", q.sql)
	}
}

// TestMySQL_GroupByExpr_Unchanged verifies MySQL GroupByExpr stays as ?.
func TestMySQL_GroupByExpr_Unchanged(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("status", "COUNT(*)").From("users").
		Where("active = ?", true).
		GroupByExpr("CASE WHEN age > ? THEN 'senior' ELSE 'junior' END", 60)

	q := sq.Build()

	if strings.Contains(q.sql, "$") {
		t.Errorf("MySQL SQL should not contain $N: %s", q.sql)
	}
}

// TestSQLite_OrderByExpr_Unchanged verifies SQLite OrderByExpr stays as ?.
func TestSQLite_OrderByExpr_Unchanged(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id", "name").From("users").
		Where("active = ?", true).
		OrderByExpr("CASE WHEN priority = ? THEN 0 ELSE 1 END", "high")

	q := sq.Build()

	if strings.Contains(q.sql, "$") {
		t.Errorf("SQLite SQL should not contain $N: %s", q.sql)
	}
}

// TestUnion_PostgreSQL verifies UNION branch placeholders are renumbered sequentially.
func TestUnion_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("users").Where("status = ?", "active")
	q2 := qb.Select("name").From("archived_users").Where("status = ?", "archived")
	q := q1.Union(q2).Build()

	verifyPlaceholderSequence(t, q.sql, 2)

	if len(q.params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != "active" {
		t.Errorf("params[0]: got %v, want \"active\"", q.params[0])
	}
	if q.params[1] != "archived" {
		t.Errorf("params[1]: got %v, want \"archived\"", q.params[1])
	}
}

// TestCTE_PostgreSQL verifies CTE placeholders are renumbered sequentially.
func TestCTE_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	cte := qb.Select("user_id", "SUM(total) as total").
		From("orders").
		Where("status = ?", "paid").
		GroupBy("user_id")

	q := qb.Select().
		With("order_totals", cte).
		From("order_totals").
		Where("total > ?", 1000).
		Build()

	// CTE param ($1 = "paid") + outer WHERE param ($2 = 1000)
	verifyPlaceholderSequence(t, q.sql, 2)

	if len(q.params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != "paid" {
		t.Errorf("params[0]: got %v, want \"paid\"", q.params[0])
	}
	if q.params[1] != 1000 {
		t.Errorf("params[1]: got %v, want 1000", q.params[1])
	}
}

// TestFromSelect_PostgreSQL verifies FROM subquery placeholders are renumbered.
func TestFromSelect_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id", "COUNT(*) as cnt").
		From("orders").
		Where("status = ?", "paid").
		GroupBy("user_id")

	q := qb.Select("user_id", "cnt").
		FromSelect(sub, "order_counts").
		Where("cnt > ?", 10).
		Build()

	// FROM subquery param ($1 = "paid") + outer WHERE param ($2 = 10)
	verifyPlaceholderSequence(t, q.sql, 2)

	if len(q.params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(q.params), q.params)
	}
	if q.params[0] != "paid" {
		t.Errorf("params[0]: got %v, want \"paid\"", q.params[0])
	}
	if q.params[1] != 10 {
		t.Errorf("params[1]: got %v, want 10", q.params[1])
	}
}
