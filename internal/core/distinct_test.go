package core

import (
	"strings"
	"testing"
)

// TestSelectQuery_Distinct tests that Distinct() adds the DISTINCT keyword.
func TestSelectQuery_Distinct_True(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("category").
		From("products").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL contains DISTINCT keyword
	if !strings.Contains(q.sql, `SELECT DISTINCT "category"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "category"`)
	}
	if !strings.Contains(q.sql, `FROM "products"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "products"`)
	}
	if len(q.params) != 0 {
		t.Errorf("DISTINCT should have no params, got %d", len(q.params))
	}
}

// TestSelectQuery_Distinct_False tests that without Distinct() there is no DISTINCT keyword.
func TestSelectQuery_Distinct_False(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("name").
		From("users")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL does NOT contain DISTINCT
	if strings.Contains(q.sql, "DISTINCT") {
		t.Errorf("%q should not contain %q", q.sql, "DISTINCT")
	}
	if !strings.Contains(q.sql, `SELECT "name"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT "name"`)
	}
	if !strings.Contains(q.sql, `FROM "users"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "users"`)
	}
}

// TestSelectQuery_Distinct_Default tests default behavior (no DISTINCT).
func TestSelectQuery_Distinct_Default(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("email").
		From("contacts")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// By default, DISTINCT should not be present
	if strings.Contains(q.sql, "DISTINCT") {
		t.Errorf("%q should not contain %q", q.sql, "DISTINCT")
	}
	if !strings.Contains(q.sql, `SELECT "email"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT "email"`)
	}
}

// TestSelectQuery_Distinct_MultipleColumns tests DISTINCT with multiple columns.
func TestSelectQuery_Distinct_MultipleColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("country", "city").
		From("locations").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "country", "city"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "country", "city"`)
	}
	if !strings.Contains(q.sql, `FROM "locations"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "locations"`)
	}
}

// TestSelectQuery_Distinct_Wildcard tests DISTINCT with wildcard selector.
func TestSelectQuery_Distinct_Wildcard(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("*").
		From("logs").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT *`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT *`)
	}
	if !strings.Contains(q.sql, `FROM "logs"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "logs"`)
	}
}

// TestSelectQuery_Distinct_WithWhere tests DISTINCT with WHERE clause.
func TestSelectQuery_Distinct_WithWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("status").
		From("orders").
		Where("total > ?", 100).
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "status"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "status"`)
	}
	if !strings.Contains(q.sql, `FROM "orders"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "orders"`)
	}
	if !strings.Contains(q.sql, `WHERE`) {
		t.Errorf("%q does not contain %q", q.sql, `WHERE`)
	}
	if len(q.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(q.params))
	}
	if q.params[0] != 100 {
		t.Errorf("got %v, want %v", q.params[0], 100)
	}
}

// TestSelectQuery_Distinct_WithOrderBy tests DISTINCT with ORDER BY.
func TestSelectQuery_Distinct_WithOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("department").
		From("employees").
		Distinct().
		OrderBy("department ASC")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "department"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "department"`)
	}
	if !strings.Contains(q.sql, `ORDER BY "department" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `ORDER BY "department" ASC`)
	}
}

// TestSelectQuery_Distinct_WithLimit tests DISTINCT with LIMIT.
func TestSelectQuery_Distinct_WithLimit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("tag").
		From("posts").
		Distinct().
		Limit(10)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "tag"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "tag"`)
	}
	if !strings.Contains(q.sql, `LIMIT 10`) {
		t.Errorf("%q does not contain %q", q.sql, `LIMIT 10`)
	}
}

// TestSelectQuery_Distinct_WithJoin tests DISTINCT with JOIN.
func TestSelectQuery_Distinct_WithJoin(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("u.country").
		From("users u").
		InnerJoin("orders o", "o.user_id = u.id").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "u"."country"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "u"."country"`)
	}
	if !strings.Contains(q.sql, `INNER JOIN`) {
		t.Errorf("%q does not contain %q", q.sql, `INNER JOIN`)
	}
}

// TestSelectQuery_Distinct_Toggle tests that Distinct() enables DISTINCT and a fresh query without it has none.
func TestSelectQuery_Distinct_Toggle(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Enable DISTINCT
	query := qb.Select("role").
		From("users").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "DISTINCT") {
		t.Errorf("%q does not contain %q", q.sql, "DISTINCT")
	}

	// Without Distinct() — no DISTINCT keyword
	query2 := qb.Select("role").
		From("users")

	q2 := query2.Build()
	if q2 == nil {
		t.Fatal("expected non-nil")
	}
	if strings.Contains(q2.sql, "DISTINCT") {
		t.Errorf("%q should not contain %q", q2.sql, "DISTINCT")
	}
}

// TestSelectQuery_Distinct_Chainable tests that Distinct() returns SelectQuery.
func TestSelectQuery_Distinct_Chainable(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Verify method chaining works
	query := qb.Select("type").
		From("items").
		Distinct().
		Where("active = ?", true).
		OrderBy("type").
		Limit(5)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "type"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "type"`)
	}
	if !strings.Contains(q.sql, `WHERE`) {
		t.Errorf("%q does not contain %q", q.sql, `WHERE`)
	}
	if !strings.Contains(q.sql, `ORDER BY`) {
		t.Errorf("%q does not contain %q", q.sql, `ORDER BY`)
	}
	if !strings.Contains(q.sql, `LIMIT 5`) {
		t.Errorf("%q does not contain %q", q.sql, `LIMIT 5`)
	}
}

// TestSelectQuery_Distinct_WithAggregate tests DISTINCT with aggregate functions.
func TestSelectQuery_Distinct_WithAggregate(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("COUNT(DISTINCT user_id)").
		From("events").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// DISTINCT should be in SELECT clause even with aggregate
	if !strings.Contains(q.sql, `SELECT DISTINCT COUNT(DISTINCT user_id)`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT COUNT(DISTINCT user_id)`)
	}
}

// TestSelectQuery_Distinct_PostgreSQL tests PostgreSQL dialect.
func TestSelectQuery_Distinct_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("category").
		From("products").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// PostgreSQL uses double quotes
	if !strings.Contains(q.sql, `SELECT DISTINCT "category"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "category"`)
	}
	if !strings.Contains(q.sql, `FROM "products"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "products"`)
	}
}

// TestSelectQuery_Distinct_MySQL tests MySQL dialect.
func TestSelectQuery_Distinct_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select("brand").
		From("products").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// MySQL uses backticks
	if !strings.Contains(q.sql, "SELECT DISTINCT `brand`") {
		t.Errorf("%q does not contain %q", q.sql, "SELECT DISTINCT `brand`")
	}
	if !strings.Contains(q.sql, "FROM `products`") {
		t.Errorf("%q does not contain %q", q.sql, "FROM `products`")
	}
}

// TestSelectQuery_Distinct_SQLite tests SQLite dialect.
func TestSelectQuery_Distinct_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	query := qb.Select("color").
		From("items").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// SQLite uses double quotes (like PostgreSQL)
	if !strings.Contains(q.sql, `SELECT DISTINCT "color"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "color"`)
	}
	if !strings.Contains(q.sql, `FROM "items"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "items"`)
	}
}

// TestSelectQuery_Distinct_ComplexQuery tests DISTINCT in a complex query.
func TestSelectQuery_Distinct_ComplexQuery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("u.country", "u.city").
		From("users u").
		InnerJoin("orders o", "o.user_id = u.id").
		Where("o.status = ?", "completed").
		Where("o.total > ?", 50).
		Distinct().
		OrderBy("u.country ASC", "u.city ASC").
		Limit(100).
		Offset(20)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify all clauses are present
	if !strings.Contains(q.sql, `SELECT DISTINCT "u"."country", "u"."city"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "u"."country", "u"."city"`)
	}
	if !strings.Contains(q.sql, `FROM "users" AS "u"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "users" AS "u"`)
	}
	if !strings.Contains(q.sql, `INNER JOIN "orders" AS "o"`) {
		t.Errorf("%q does not contain %q", q.sql, `INNER JOIN "orders" AS "o"`)
	}
	if !strings.Contains(q.sql, `WHERE`) {
		t.Errorf("%q does not contain %q", q.sql, `WHERE`)
	}
	if !strings.Contains(q.sql, `ORDER BY "u"."country" ASC, "u"."city" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `ORDER BY "u"."country" ASC, "u"."city" ASC`)
	}
	if !strings.Contains(q.sql, `LIMIT 100`) {
		t.Errorf("%q does not contain %q", q.sql, `LIMIT 100`)
	}
	if !strings.Contains(q.sql, `OFFSET 20`) {
		t.Errorf("%q does not contain %q", q.sql, `OFFSET 20`)
	}

	// Verify parameters
	if len(q.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(q.params))
	}
	if q.params[0] != "completed" {
		t.Errorf("got %v, want %v", q.params[0], "completed")
	}
	if q.params[1] != 50 {
		t.Errorf("got %v, want %v", q.params[1], 50)
	}

	// Verify clause order: SELECT < FROM < JOIN < WHERE < ORDER BY < LIMIT < OFFSET
	selectIdx := indexOf(q.sql, "SELECT DISTINCT")
	fromIdx := indexOf(q.sql, "FROM")
	joinIdx := indexOf(q.sql, "INNER JOIN")
	whereIdx := indexOf(q.sql, "WHERE")
	orderIdx := indexOf(q.sql, "ORDER BY")
	limitIdx := indexOf(q.sql, "LIMIT")
	offsetIdx := indexOf(q.sql, "OFFSET")

	if selectIdx >= fromIdx {
		t.Errorf("expected selectIdx < fromIdx, got %d >= %d", selectIdx, fromIdx)
	}
	if fromIdx >= joinIdx {
		t.Errorf("expected fromIdx < joinIdx, got %d >= %d", fromIdx, joinIdx)
	}
	if joinIdx >= whereIdx {
		t.Errorf("expected joinIdx < whereIdx, got %d >= %d", joinIdx, whereIdx)
	}
	if whereIdx >= orderIdx {
		t.Errorf("expected whereIdx < orderIdx, got %d >= %d", whereIdx, orderIdx)
	}
	if orderIdx >= limitIdx {
		t.Errorf("expected orderIdx < limitIdx, got %d >= %d", orderIdx, limitIdx)
	}
	if limitIdx >= offsetIdx {
		t.Errorf("expected limitIdx < offsetIdx, got %d >= %d", limitIdx, offsetIdx)
	}
}

// TestSelectQuery_Distinct_WithGroupBy tests DISTINCT with GROUP BY.
func TestSelectQuery_Distinct_WithGroupBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("category", "COUNT(*) as cnt").
		From("products").
		GroupBy("category").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "category", COUNT(*) as cnt`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "category", COUNT(*) as cnt`)
	}
	if !strings.Contains(q.sql, `GROUP BY "category"`) {
		t.Errorf("%q does not contain %q", q.sql, `GROUP BY "category"`)
	}
}

// TestSelectQuery_Distinct_WithSelectExpr tests DISTINCT with SelectExpr.
func TestSelectQuery_Distinct_WithSelectExpr(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("name").
		SelectExpr("UPPER(email) as upper_email").
		From("users").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "name", UPPER(email) as upper_email`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "name", UPPER(email) as upper_email`)
	}
	if !strings.Contains(q.sql, `FROM "users"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "users"`)
	}
}

// TestSelectQuery_Distinct_EmptySelect tests DISTINCT with default columns.
func TestSelectQuery_Distinct_EmptySelect(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Should default to SELECT DISTINCT *
	if !strings.Contains(q.sql, "SELECT DISTINCT *") {
		t.Errorf("%q does not contain %q", q.sql, "SELECT DISTINCT *")
	}
	if !strings.Contains(q.sql, `FROM "users"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "users"`)
	}
}

// TestSelectQuery_Distinct_WithAlias tests DISTINCT with column aliases.
func TestSelectQuery_Distinct_WithAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("status as order_status").
		From("orders").
		Distinct()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT DISTINCT "status" AS "order_status"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT DISTINCT "status" AS "order_status"`)
	}
	if !strings.Contains(q.sql, `FROM "orders"`) {
		t.Errorf("%q does not contain %q", q.sql, `FROM "orders"`)
	}
}
