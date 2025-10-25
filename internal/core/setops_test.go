package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelectQuery_Union_PostgreSQL tests basic UNION operation with PostgreSQL
func TestSelectQuery_Union_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("users").Where("status = ?", 1)
	q2 := qb.Select("name").From("archived_users").Where("status = ?", 1)

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// Verify SQL structure (column names are quoted)
	assert.Contains(t, query.sql, `SELECT "name" FROM "users" WHERE status = $1`)
	assert.Contains(t, query.sql, `UNION`)
	assert.Contains(t, query.sql, `SELECT "name" FROM "archived_users" WHERE status = $2`)
	assert.Len(t, query.params, 2)
	assert.Equal(t, 1, query.params[0])
	assert.Equal(t, 1, query.params[1])
}

// TestSelectQuery_UnionAll_PostgreSQL tests UNION ALL (keeps duplicates)
func TestSelectQuery_UnionAll_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("orders_2023")
	q2 := qb.Select("id").From("orders_2024")

	query := q1.UnionAll(q2).Build()
	require.NotNil(t, query)

	// Verify UNION ALL keyword (column names are quoted)
	assert.Contains(t, query.sql, `UNION ALL`)
	assert.Contains(t, query.sql, `SELECT "id" FROM "orders_2023"`)
	assert.Contains(t, query.sql, `SELECT "id" FROM "orders_2024"`)
	assert.Empty(t, query.params, "No parameters expected")
}

// TestSelectQuery_Union_MySQL tests UNION with MySQL syntax (backticks)
func TestSelectQuery_Union_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("email").From("customers").Where("active = ?", true)
	q2 := qb.Select("email").From("subscribers").Where("active = ?", true)

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// MySQL uses backticks for identifiers (column names are quoted)
	assert.Contains(t, query.sql, "SELECT `email` FROM `customers` WHERE active = ?")
	assert.Contains(t, query.sql, "UNION")
	assert.Contains(t, query.sql, "SELECT `email` FROM `subscribers` WHERE active = ?")
	assert.Len(t, query.params, 2)
}

// TestSelectQuery_Union_SQLite tests UNION with SQLite (double quotes)
func TestSelectQuery_Union_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("products").Where("price > ?", 100)
	q2 := qb.Select("name").From("premium_products")

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// SQLite uses double quotes (column names are quoted)
	assert.Contains(t, query.sql, `SELECT "name" FROM "products" WHERE price > ?`)
	assert.Contains(t, query.sql, `UNION`)
	assert.Contains(t, query.sql, `SELECT "name" FROM "premium_products"`)
	assert.Len(t, query.params, 1)
	assert.Equal(t, 100, query.params[0])
}

// TestSelectQuery_Intersect_PostgreSQL tests INTERSECT operation
func TestSelectQuery_Intersect_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("users")
	q2 := qb.Select("user_id").From("orders")

	query := q1.Intersect(q2).Build()
	require.NotNil(t, query)

	// Verify INTERSECT keyword (column names are quoted)
	assert.Contains(t, query.sql, `SELECT "id" FROM "users"`)
	assert.Contains(t, query.sql, `INTERSECT`)
	assert.Contains(t, query.sql, `SELECT "user_id" FROM "orders"`)
	assert.Empty(t, query.params)
}

// TestSelectQuery_Intersect_SQLite tests INTERSECT with SQLite
func TestSelectQuery_Intersect_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("email").From("newsletter_subscribers")
	q2 := qb.Select("email").From("active_users")

	query := q1.Intersect(q2).Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, "INTERSECT")
	assert.Contains(t, query.sql, `"newsletter_subscribers"`)
	assert.Contains(t, query.sql, `"active_users"`)
}

// TestSelectQuery_Except_PostgreSQL tests EXCEPT operation
func TestSelectQuery_Except_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("all_users")
	q2 := qb.Select("user_id").From("banned_users")

	query := q1.Except(q2).Build()
	require.NotNil(t, query)

	// Verify EXCEPT keyword (column names are quoted)
	assert.Contains(t, query.sql, `SELECT "id" FROM "all_users"`)
	assert.Contains(t, query.sql, `EXCEPT`)
	assert.Contains(t, query.sql, `SELECT "user_id" FROM "banned_users"`)
}

// TestSelectQuery_Except_SQLite tests EXCEPT with SQLite
func TestSelectQuery_Except_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("registered_users")
	q2 := qb.Select("user_id").From("deleted_accounts")

	query := q1.Except(q2).Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, "EXCEPT")
	assert.Contains(t, query.sql, `"registered_users"`)
	assert.Contains(t, query.sql, `"deleted_accounts"`)
}

// TestSelectQuery_Multiple_Unions tests chaining multiple UNION operations
func TestSelectQuery_Multiple_Unions(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("table1")
	q2 := qb.Select("name").From("table2")
	q3 := qb.Select("name").From("table3")

	query := q1.Union(q2).Union(q3).Build()
	require.NotNil(t, query)

	// Verify all three queries are present (column names are quoted)
	assert.Contains(t, query.sql, `SELECT "name" FROM "table1"`)
	assert.Contains(t, query.sql, `SELECT "name" FROM "table2"`)
	assert.Contains(t, query.sql, `SELECT "name" FROM "table3"`)

	// Count UNION keywords (should be 2 for 3 queries)
	unionCount := 0
	for i := 0; i < len(query.sql)-5; i++ {
		if query.sql[i:i+5] == "UNION" {
			unionCount++
		}
	}
	assert.Equal(t, 2, unionCount, "Expected 2 UNION keywords for 3 queries")
}

// TestSelectQuery_Mixed_Set_Operations tests mixing UNION, INTERSECT, EXCEPT
func TestSelectQuery_Mixed_Set_Operations(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("set_a")
	q2 := qb.Select("id").From("set_b")
	q3 := qb.Select("id").From("set_c")

	query := q1.Union(q2).Except(q3).Build()
	require.NotNil(t, query)

	// Verify operation order (column names are quoted)
	assert.Contains(t, query.sql, `SELECT "id" FROM "set_a"`)
	assert.Contains(t, query.sql, `UNION`)
	assert.Contains(t, query.sql, `SELECT "id" FROM "set_b"`)
	assert.Contains(t, query.sql, `EXCEPT`)
	assert.Contains(t, query.sql, `SELECT "id" FROM "set_c"`)
}

// TestSelectQuery_Union_Parameter_Merging tests that parameters from all queries are merged correctly
func TestSelectQuery_Union_Parameter_Merging(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("*").From("users").Where("age > ?", 18)
	q2 := qb.Select("*").From("users").Where("status = ? AND country = ?", "active", "US")

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// Verify parameters are in correct order
	assert.Len(t, query.params, 3)
	assert.Equal(t, 18, query.params[0])
	assert.Equal(t, "active", query.params[1])
	assert.Equal(t, "US", query.params[2])

	// Verify placeholders are correctly numbered in PostgreSQL
	assert.Contains(t, query.sql, "$1") // age > 18
	assert.Contains(t, query.sql, "$2") // status = active
	assert.Contains(t, query.sql, "$3") // country = US (renumbered from $2 in second query)
}

// TestSelectQuery_Union_With_Complex_Where tests UNION with complex WHERE clauses
func TestSelectQuery_Union_With_Complex_Where(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("product_id", "name").
		From("products").
		Where("category = ? AND price > ?", "electronics", 100)

	q2 := qb.Select("product_id", "name").
		From("legacy_products").
		Where("status = ?", "active")

	query := q1.UnionAll(q2).Build()
	require.NotNil(t, query)

	// Verify parameters
	assert.Len(t, query.params, 3)
	assert.Equal(t, "electronics", query.params[0])
	assert.Equal(t, 100, query.params[1])
	assert.Equal(t, "active", query.params[2])

	// Verify UNION ALL
	assert.Contains(t, query.sql, "UNION ALL")
}

// TestSelectQuery_Union_With_OrderBy tests that ORDER BY is preserved in base query
func TestSelectQuery_Union_With_OrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("users").OrderBy("name ASC")
	q2 := qb.Select("name").From("archived_users")

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// Base query should have ORDER BY (column name is quoted)
	assert.Contains(t, query.sql, "ORDER BY")
	assert.Contains(t, query.sql, `"name"`) // ORDER BY uses quoted column names
}

// TestSelectQuery_Union_With_Limit tests UNION with LIMIT in base query
func TestSelectQuery_Union_With_Limit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	limit := int64(10)
	q1 := qb.Select("id").From("recent_orders").Limit(limit)
	q2 := qb.Select("id").From("pending_orders")

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// Base query should have LIMIT
	assert.Contains(t, query.sql, "LIMIT 10")
}

// TestSelectQuery_Union_Nil_Query tests that nil queries are safely ignored
func TestSelectQuery_Union_Nil_Query(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("users")
	query := q1.Union(nil).Build()

	require.NotNil(t, query)

	// Should only contain the first query (column name is quoted)
	assert.Contains(t, query.sql, `SELECT "id" FROM "users"`)
	assert.NotContains(t, query.sql, "UNION")
}

// TestSelectQuery_Intersect_With_Parameters tests INTERSECT with WHERE parameters
func TestSelectQuery_Intersect_With_Parameters(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("user_id").From("premium_members").Where("subscription_active = ?", true)
	q2 := qb.Select("user_id").From("forum_participants").Where("posts_count > ?", 10)

	query := q1.Intersect(q2).Build()
	require.NotNil(t, query)

	// Verify parameters
	assert.Len(t, query.params, 2)
	assert.Equal(t, true, query.params[0])
	assert.Equal(t, 10, query.params[1])

	assert.Contains(t, query.sql, "INTERSECT")
}

// TestSelectQuery_Union_With_JOINs tests UNION where queries contain JOINs
func TestSelectQuery_Union_With_JOINs(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("u.name", "o.total").
		From("users u").
		InnerJoin("orders o", "u.id = o.user_id")

	q2 := qb.Select("c.name", "p.amount").
		From("customers c").
		LeftJoin("payments p", "c.id = p.customer_id")

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// Verify both JOINs are present
	assert.Contains(t, query.sql, "INNER JOIN")
	assert.Contains(t, query.sql, "LEFT JOIN")
	assert.Contains(t, query.sql, "UNION")
}

// TestSelectQuery_Union_With_Subqueries tests UNION where queries use subqueries in FROM
func TestSelectQuery_Union_With_Subqueries(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub1 := qb.Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")
	q1 := qb.Select("user_id", "cnt").FromSelect(sub1, "order_counts").Where("cnt > ?", 5)

	q2 := qb.Select("id", "purchase_count").From("high_value_customers")

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// Verify subquery is present
	assert.Contains(t, query.sql, "FROM (SELECT")
	assert.Contains(t, query.sql, "GROUP BY")
	assert.Contains(t, query.sql, "UNION")
	assert.Len(t, query.params, 1)
	assert.Equal(t, 5, query.params[0])
}

// TestSelectQuery_Except_Multiple tests multiple EXCEPT operations
func TestSelectQuery_Except_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("all_users")
	q2 := qb.Select("id").From("banned_users")
	q3 := qb.Select("id").From("suspended_users")

	query := q1.Except(q2).Except(q3).Build()
	require.NotNil(t, query)

	// Should have 2 EXCEPT keywords
	exceptCount := 0
	for i := 0; i < len(query.sql)-6; i++ {
		if i+6 <= len(query.sql) && query.sql[i:i+6] == "EXCEPT" {
			exceptCount++
		}
	}
	assert.Equal(t, 2, exceptCount)
}

// TestSelectQuery_UnionAll_Performance_Note tests UnionAll documentation
func TestSelectQuery_UnionAll_Performance_Note(t *testing.T) {
	// This test documents that UNION ALL is faster than UNION
	// because it doesn't remove duplicates
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("large_table_1")
	q2 := qb.Select("id").From("large_table_2")

	unionAll := q1.UnionAll(q2).Build()
	require.NotNil(t, unionAll)

	// UnionAll should use "UNION ALL" keyword
	assert.Contains(t, unionAll.sql, "UNION ALL")
	assert.NotContains(t, unionAll.sql, "UNION (") // Not plain UNION
}

// TestSelectQuery_Union_Empty_Result tests UNION with queries that may return empty results
func TestSelectQuery_Union_Empty_Result(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("users").Where("1 = ?", 0) // Always false
	q2 := qb.Select("id").From("users").Where("active = ?", true)

	query := q1.Union(q2).Build()
	require.NotNil(t, query)

	// Both queries should be in SQL even if one is empty
	assert.Contains(t, query.sql, "UNION")
	assert.Len(t, query.params, 2)
}
