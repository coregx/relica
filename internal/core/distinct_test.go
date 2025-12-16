package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelectQuery_Distinct_True tests DISTINCT with true flag.
func TestSelectQuery_Distinct_True(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("category").
		From("products").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL contains DISTINCT keyword
	assert.Contains(t, q.sql, `SELECT DISTINCT "category"`)
	assert.Contains(t, q.sql, `FROM "products"`)
	assert.Empty(t, q.params, "DISTINCT should have no params")
}

// TestSelectQuery_Distinct_False tests DISTINCT with false flag.
func TestSelectQuery_Distinct_False(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("name").
		From("users").
		Distinct(false)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL does NOT contain DISTINCT
	assert.NotContains(t, q.sql, "DISTINCT")
	assert.Contains(t, q.sql, `SELECT "name"`)
	assert.Contains(t, q.sql, `FROM "users"`)
}

// TestSelectQuery_Distinct_Default tests default behavior (no DISTINCT).
func TestSelectQuery_Distinct_Default(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("email").
		From("contacts")

	q := query.Build()
	require.NotNil(t, q)

	// By default, DISTINCT should not be present
	assert.NotContains(t, q.sql, "DISTINCT")
	assert.Contains(t, q.sql, `SELECT "email"`)
}

// TestSelectQuery_Distinct_MultipleColumns tests DISTINCT with multiple columns.
func TestSelectQuery_Distinct_MultipleColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("country", "city").
		From("locations").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "country", "city"`)
	assert.Contains(t, q.sql, `FROM "locations"`)
}

// TestSelectQuery_Distinct_Wildcard tests DISTINCT with wildcard selector.
func TestSelectQuery_Distinct_Wildcard(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("*").
		From("logs").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT *`)
	assert.Contains(t, q.sql, `FROM "logs"`)
}

// TestSelectQuery_Distinct_WithWhere tests DISTINCT with WHERE clause.
func TestSelectQuery_Distinct_WithWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("status").
		From("orders").
		Where("total > ?", 100).
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "status"`)
	assert.Contains(t, q.sql, `FROM "orders"`)
	assert.Contains(t, q.sql, `WHERE`)
	assert.Len(t, q.params, 1)
	assert.Equal(t, 100, q.params[0])
}

// TestSelectQuery_Distinct_WithOrderBy tests DISTINCT with ORDER BY.
func TestSelectQuery_Distinct_WithOrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("department").
		From("employees").
		Distinct(true).
		OrderBy("department ASC")

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "department"`)
	assert.Contains(t, q.sql, `ORDER BY "department" ASC`)
}

// TestSelectQuery_Distinct_WithLimit tests DISTINCT with LIMIT.
func TestSelectQuery_Distinct_WithLimit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("tag").
		From("posts").
		Distinct(true).
		Limit(10)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "tag"`)
	assert.Contains(t, q.sql, `LIMIT 10`)
}

// TestSelectQuery_Distinct_WithJoin tests DISTINCT with JOIN.
func TestSelectQuery_Distinct_WithJoin(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("u.country").
		From("users u").
		InnerJoin("orders o", "o.user_id = u.id").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "u"."country"`)
	assert.Contains(t, q.sql, `INNER JOIN`)
}

// TestSelectQuery_Distinct_Toggle tests toggling DISTINCT on and off.
func TestSelectQuery_Distinct_Toggle(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Enable DISTINCT
	query := qb.Select("role").
		From("users").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)
	assert.Contains(t, q.sql, "DISTINCT")

	// Disable DISTINCT (override)
	query2 := qb.Select("role").
		From("users").
		Distinct(true).
		Distinct(false)

	q2 := query2.Build()
	require.NotNil(t, q2)
	assert.NotContains(t, q2.sql, "DISTINCT")
}

// TestSelectQuery_Distinct_Chainable tests that Distinct() returns SelectQuery.
func TestSelectQuery_Distinct_Chainable(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Verify method chaining works
	query := qb.Select("type").
		From("items").
		Distinct(true).
		Where("active = ?", true).
		OrderBy("type").
		Limit(5)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "type"`)
	assert.Contains(t, q.sql, `WHERE`)
	assert.Contains(t, q.sql, `ORDER BY`)
	assert.Contains(t, q.sql, `LIMIT 5`)
}

// TestSelectQuery_Distinct_WithAggregate tests DISTINCT with aggregate functions.
func TestSelectQuery_Distinct_WithAggregate(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("COUNT(DISTINCT user_id)").
		From("events").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	// DISTINCT should be in SELECT clause even with aggregate
	assert.Contains(t, q.sql, `SELECT DISTINCT COUNT(DISTINCT user_id)`)
}

// TestSelectQuery_Distinct_PostgreSQL tests PostgreSQL dialect.
func TestSelectQuery_Distinct_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("category").
		From("products").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	// PostgreSQL uses double quotes
	assert.Contains(t, q.sql, `SELECT DISTINCT "category"`)
	assert.Contains(t, q.sql, `FROM "products"`)
}

// TestSelectQuery_Distinct_MySQL tests MySQL dialect.
func TestSelectQuery_Distinct_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select("brand").
		From("products").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	// MySQL uses backticks
	assert.Contains(t, q.sql, "SELECT DISTINCT `brand`")
	assert.Contains(t, q.sql, "FROM `products`")
}

// TestSelectQuery_Distinct_SQLite tests SQLite dialect.
func TestSelectQuery_Distinct_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	query := qb.Select("color").
		From("items").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	// SQLite uses double quotes (like PostgreSQL)
	assert.Contains(t, q.sql, `SELECT DISTINCT "color"`)
	assert.Contains(t, q.sql, `FROM "items"`)
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
		Distinct(true).
		OrderBy("u.country ASC", "u.city ASC").
		Limit(100).
		Offset(20)

	q := query.Build()
	require.NotNil(t, q)

	// Verify all clauses are present
	assert.Contains(t, q.sql, `SELECT DISTINCT "u"."country", "u"."city"`)
	assert.Contains(t, q.sql, `FROM "users" AS "u"`)
	assert.Contains(t, q.sql, `INNER JOIN "orders" AS "o"`)
	assert.Contains(t, q.sql, `WHERE`)
	assert.Contains(t, q.sql, `ORDER BY "u"."country" ASC, "u"."city" ASC`)
	assert.Contains(t, q.sql, `LIMIT 100`)
	assert.Contains(t, q.sql, `OFFSET 20`)

	// Verify parameters
	assert.Len(t, q.params, 2)
	assert.Equal(t, "completed", q.params[0])
	assert.Equal(t, 50, q.params[1])

	// Verify clause order: SELECT < FROM < JOIN < WHERE < ORDER BY < LIMIT < OFFSET
	selectIdx := indexOf(q.sql, "SELECT DISTINCT")
	fromIdx := indexOf(q.sql, "FROM")
	joinIdx := indexOf(q.sql, "INNER JOIN")
	whereIdx := indexOf(q.sql, "WHERE")
	orderIdx := indexOf(q.sql, "ORDER BY")
	limitIdx := indexOf(q.sql, "LIMIT")
	offsetIdx := indexOf(q.sql, "OFFSET")

	assert.Less(t, selectIdx, fromIdx)
	assert.Less(t, fromIdx, joinIdx)
	assert.Less(t, joinIdx, whereIdx)
	assert.Less(t, whereIdx, orderIdx)
	assert.Less(t, orderIdx, limitIdx)
	assert.Less(t, limitIdx, offsetIdx)
}

// TestSelectQuery_Distinct_WithGroupBy tests DISTINCT with GROUP BY.
func TestSelectQuery_Distinct_WithGroupBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("category", "COUNT(*) as cnt").
		From("products").
		GroupBy("category").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "category", COUNT(*) as cnt`)
	assert.Contains(t, q.sql, `GROUP BY "category"`)
}

// TestSelectQuery_Distinct_WithSelectExpr tests DISTINCT with SelectExpr.
func TestSelectQuery_Distinct_WithSelectExpr(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("name").
		SelectExpr("UPPER(email) as upper_email").
		From("users").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT DISTINCT "name", UPPER(email) as upper_email`)
	assert.Contains(t, q.sql, `FROM "users"`)
}

// TestSelectQuery_Distinct_EmptySelect tests DISTINCT with default columns.
func TestSelectQuery_Distinct_EmptySelect(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	// Should default to SELECT DISTINCT *
	assert.Contains(t, q.sql, "SELECT DISTINCT *")
	assert.Contains(t, q.sql, `FROM "users"`)
}

// TestSelectQuery_Distinct_WithAlias tests DISTINCT with column aliases.
func TestSelectQuery_Distinct_WithAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("status as order_status").
		From("orders").
		Distinct(true)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, "SELECT DISTINCT status as order_status")
	assert.Contains(t, q.sql, `FROM "orders"`)
}
