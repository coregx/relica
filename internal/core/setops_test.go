package core

import (
	"strings"
	"testing"
)

// TestSelectQuery_Union_PostgreSQL tests basic UNION operation with PostgreSQL
func TestSelectQuery_Union_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("users").Where("status = ?", 1)
	q2 := qb.Select("name").From("archived_users").Where("status = ?", 1)

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure (column names are quoted)
	if !strings.Contains(query.sql, `SELECT "name" FROM "users" WHERE status = $1`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "name" FROM "users" WHERE status = $1`)
	}
	if !strings.Contains(query.sql, `UNION`) {
		t.Errorf("%q does not contain %q", query.sql, `UNION`)
	}
	if !strings.Contains(query.sql, `SELECT "name" FROM "archived_users" WHERE status = $2`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "name" FROM "archived_users" WHERE status = $2`)
	}
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
	if query.params[0] != 1 {
		t.Errorf("got %v, want %v", query.params[0], 1)
	}
	if query.params[1] != 1 {
		t.Errorf("got %v, want %v", query.params[1], 1)
	}
}

// TestSelectQuery_UnionAll_PostgreSQL tests UNION ALL (keeps duplicates)
func TestSelectQuery_UnionAll_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("orders_2023")
	q2 := qb.Select("id").From("orders_2024")

	query := q1.UnionAll(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify UNION ALL keyword (column names are quoted)
	if !strings.Contains(query.sql, `UNION ALL`) {
		t.Errorf("%q does not contain %q", query.sql, `UNION ALL`)
	}
	if !strings.Contains(query.sql, `SELECT "id" FROM "orders_2023"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "orders_2023"`)
	}
	if !strings.Contains(query.sql, `SELECT "id" FROM "orders_2024"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "orders_2024"`)
	}
	if len(query.params) != 0 {
		t.Errorf("No parameters expected: expected empty, got %d", len(query.params))
	}
}

// TestSelectQuery_Union_MySQL tests UNION with MySQL syntax (backticks)
func TestSelectQuery_Union_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("email").From("customers").Where("active = ?", true)
	q2 := qb.Select("email").From("subscribers").Where("active = ?", true)

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// MySQL uses backticks for identifiers (column names are quoted)
	if !strings.Contains(query.sql, "SELECT `email` FROM `customers` WHERE active = ?") {
		t.Errorf("%q does not contain %q", query.sql, "SELECT `email` FROM `customers` WHERE active = ?")
	}
	if !strings.Contains(query.sql, "UNION") {
		t.Errorf("%q does not contain %q", query.sql, "UNION")
	}
	if !strings.Contains(query.sql, "SELECT `email` FROM `subscribers` WHERE active = ?") {
		t.Errorf("%q does not contain %q", query.sql, "SELECT `email` FROM `subscribers` WHERE active = ?")
	}
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
}

// TestSelectQuery_Union_SQLite tests UNION with SQLite (double quotes)
func TestSelectQuery_Union_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("products").Where("price > ?", 100)
	q2 := qb.Select("name").From("premium_products")

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// SQLite uses double quotes (column names are quoted)
	if !strings.Contains(query.sql, `SELECT "name" FROM "products" WHERE price > ?`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "name" FROM "products" WHERE price > ?`)
	}
	if !strings.Contains(query.sql, `UNION`) {
		t.Errorf("%q does not contain %q", query.sql, `UNION`)
	}
	if !strings.Contains(query.sql, `SELECT "name" FROM "premium_products"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "name" FROM "premium_products"`)
	}
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != 100 {
		t.Errorf("got %v, want %v", query.params[0], 100)
	}
}

// TestSelectQuery_Intersect_PostgreSQL tests INTERSECT operation
func TestSelectQuery_Intersect_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("users")
	q2 := qb.Select("user_id").From("orders")

	query := q1.Intersect(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify INTERSECT keyword (column names are quoted)
	if !strings.Contains(query.sql, `SELECT "id" FROM "users"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "users"`)
	}
	if !strings.Contains(query.sql, `INTERSECT`) {
		t.Errorf("%q does not contain %q", query.sql, `INTERSECT`)
	}
	if !strings.Contains(query.sql, `SELECT "user_id" FROM "orders"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "user_id" FROM "orders"`)
	}
	if len(query.params) != 0 {
		t.Errorf("expected empty, got %d", len(query.params))
	}
}

// TestSelectQuery_Intersect_SQLite tests INTERSECT with SQLite
func TestSelectQuery_Intersect_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("email").From("newsletter_subscribers")
	q2 := qb.Select("email").From("active_users")

	query := q1.Intersect(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(query.sql, "INTERSECT") {
		t.Errorf("%q does not contain %q", query.sql, "INTERSECT")
	}
	if !strings.Contains(query.sql, `"newsletter_subscribers"`) {
		t.Errorf("%q does not contain %q", query.sql, `"newsletter_subscribers"`)
	}
	if !strings.Contains(query.sql, `"active_users"`) {
		t.Errorf("%q does not contain %q", query.sql, `"active_users"`)
	}
}

// TestSelectQuery_Except_PostgreSQL tests EXCEPT operation
func TestSelectQuery_Except_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("all_users")
	q2 := qb.Select("user_id").From("banned_users")

	query := q1.Except(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify EXCEPT keyword (column names are quoted)
	if !strings.Contains(query.sql, `SELECT "id" FROM "all_users"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "all_users"`)
	}
	if !strings.Contains(query.sql, `EXCEPT`) {
		t.Errorf("%q does not contain %q", query.sql, `EXCEPT`)
	}
	if !strings.Contains(query.sql, `SELECT "user_id" FROM "banned_users"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "user_id" FROM "banned_users"`)
	}
}

// TestSelectQuery_Except_SQLite tests EXCEPT with SQLite
func TestSelectQuery_Except_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("registered_users")
	q2 := qb.Select("user_id").From("deleted_accounts")

	query := q1.Except(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(query.sql, "EXCEPT") {
		t.Errorf("%q does not contain %q", query.sql, "EXCEPT")
	}
	if !strings.Contains(query.sql, `"registered_users"`) {
		t.Errorf("%q does not contain %q", query.sql, `"registered_users"`)
	}
	if !strings.Contains(query.sql, `"deleted_accounts"`) {
		t.Errorf("%q does not contain %q", query.sql, `"deleted_accounts"`)
	}
}

// TestSelectQuery_Multiple_Unions tests chaining multiple UNION operations
func TestSelectQuery_Multiple_Unions(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("table1")
	q2 := qb.Select("name").From("table2")
	q3 := qb.Select("name").From("table3")

	query := q1.Union(q2).Union(q3).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify all three queries are present (column names are quoted)
	if !strings.Contains(query.sql, `SELECT "name" FROM "table1"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "name" FROM "table1"`)
	}
	if !strings.Contains(query.sql, `SELECT "name" FROM "table2"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "name" FROM "table2"`)
	}
	if !strings.Contains(query.sql, `SELECT "name" FROM "table3"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "name" FROM "table3"`)
	}

	// Count UNION keywords (should be 2 for 3 queries)
	unionCount := 0
	for i := 0; i < len(query.sql)-5; i++ {
		if query.sql[i:i+5] == "UNION" {
			unionCount++
		}
	}
	if unionCount != 2 {
		t.Errorf("Expected 2 UNION keywords for 3 queries: got %v, want %v", unionCount, 2)
	}
}

// TestSelectQuery_Mixed_Set_Operations tests mixing UNION, INTERSECT, EXCEPT
func TestSelectQuery_Mixed_Set_Operations(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("set_a")
	q2 := qb.Select("id").From("set_b")
	q3 := qb.Select("id").From("set_c")

	query := q1.Union(q2).Except(q3).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify operation order (column names are quoted)
	if !strings.Contains(query.sql, `SELECT "id" FROM "set_a"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "set_a"`)
	}
	if !strings.Contains(query.sql, `UNION`) {
		t.Errorf("%q does not contain %q", query.sql, `UNION`)
	}
	if !strings.Contains(query.sql, `SELECT "id" FROM "set_b"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "set_b"`)
	}
	if !strings.Contains(query.sql, `EXCEPT`) {
		t.Errorf("%q does not contain %q", query.sql, `EXCEPT`)
	}
	if !strings.Contains(query.sql, `SELECT "id" FROM "set_c"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "set_c"`)
	}
}

// TestSelectQuery_Union_Parameter_Merging tests that parameters from all queries are merged correctly
func TestSelectQuery_Union_Parameter_Merging(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("*").From("users").Where("age > ?", 18)
	q2 := qb.Select("*").From("users").Where("status = ? AND country = ?", "active", "US")

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify parameters are in correct order
	if len(query.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(query.params))
	}
	if query.params[0] != 18 {
		t.Errorf("got %v, want %v", query.params[0], 18)
	}
	if query.params[1] != "active" {
		t.Errorf("got %v, want %v", query.params[1], "active")
	}
	if query.params[2] != "US" {
		t.Errorf("got %v, want %v", query.params[2], "US")
	}

	// Verify placeholders are correctly numbered in PostgreSQL
	if !strings.Contains(query.sql, "$1") { // age > 18
		t.Errorf("%q does not contain %q", query.sql, "$1")
	}
	if !strings.Contains(query.sql, "$2") { // status = active
		t.Errorf("%q does not contain %q", query.sql, "$2")
	}
	if !strings.Contains(query.sql, "$3") { // country = US (renumbered from $2 in second query)
		t.Errorf("%q does not contain %q", query.sql, "$3")
	}
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
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify parameters
	if len(query.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(query.params))
	}
	if query.params[0] != "electronics" {
		t.Errorf("got %v, want %v", query.params[0], "electronics")
	}
	if query.params[1] != 100 {
		t.Errorf("got %v, want %v", query.params[1], 100)
	}
	if query.params[2] != "active" {
		t.Errorf("got %v, want %v", query.params[2], "active")
	}

	// Verify UNION ALL
	if !strings.Contains(query.sql, "UNION ALL") {
		t.Errorf("%q does not contain %q", query.sql, "UNION ALL")
	}
}

// TestSelectQuery_Union_With_OrderBy tests that ORDER BY is preserved in base query
func TestSelectQuery_Union_With_OrderBy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("name").From("users").OrderBy("name ASC")
	q2 := qb.Select("name").From("archived_users")

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Base query should have ORDER BY (column name is quoted)
	if !strings.Contains(query.sql, "ORDER BY") {
		t.Errorf("%q does not contain %q", query.sql, "ORDER BY")
	}
	if !strings.Contains(query.sql, `"name"`) { // ORDER BY uses quoted column names
		t.Errorf("%q does not contain %q", query.sql, `"name"`)
	}
}

// TestSelectQuery_Union_With_Limit tests UNION with LIMIT in base query
func TestSelectQuery_Union_With_Limit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	limit := int64(10)
	q1 := qb.Select("id").From("recent_orders").Limit(limit)
	q2 := qb.Select("id").From("pending_orders")

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Base query should have LIMIT
	if !strings.Contains(query.sql, "LIMIT 10") {
		t.Errorf("%q does not contain %q", query.sql, "LIMIT 10")
	}
}

// TestSelectQuery_Union_Nil_Query tests that nil queries are safely ignored
func TestSelectQuery_Union_Nil_Query(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("users")
	query := q1.Union(nil).Build()

	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Should only contain the first query (column name is quoted)
	if !strings.Contains(query.sql, `SELECT "id" FROM "users"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "users"`)
	}
	if strings.Contains(query.sql, "UNION") {
		t.Errorf("%q should not contain %q", query.sql, "UNION")
	}
}

// TestSelectQuery_Intersect_With_Parameters tests INTERSECT with WHERE parameters
func TestSelectQuery_Intersect_With_Parameters(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("user_id").From("premium_members").Where("subscription_active = ?", true)
	q2 := qb.Select("user_id").From("forum_participants").Where("posts_count > ?", 10)

	query := q1.Intersect(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify parameters
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
	if query.params[0] != true {
		t.Errorf("got %v, want %v", query.params[0], true)
	}
	if query.params[1] != 10 {
		t.Errorf("got %v, want %v", query.params[1], 10)
	}

	if !strings.Contains(query.sql, "INTERSECT") {
		t.Errorf("%q does not contain %q", query.sql, "INTERSECT")
	}
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
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify both JOINs are present
	if !strings.Contains(query.sql, "INNER JOIN") {
		t.Errorf("%q does not contain %q", query.sql, "INNER JOIN")
	}
	if !strings.Contains(query.sql, "LEFT JOIN") {
		t.Errorf("%q does not contain %q", query.sql, "LEFT JOIN")
	}
	if !strings.Contains(query.sql, "UNION") {
		t.Errorf("%q does not contain %q", query.sql, "UNION")
	}
}

// TestSelectQuery_Union_With_Subqueries tests UNION where queries use subqueries in FROM
func TestSelectQuery_Union_With_Subqueries(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub1 := qb.Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")
	q1 := qb.Select("user_id", "cnt").FromSelect(sub1, "order_counts").Where("cnt > ?", 5)

	q2 := qb.Select("id", "purchase_count").From("high_value_customers")

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify subquery is present
	if !strings.Contains(query.sql, "FROM (SELECT") {
		t.Errorf("%q does not contain %q", query.sql, "FROM (SELECT")
	}
	if !strings.Contains(query.sql, "GROUP BY") {
		t.Errorf("%q does not contain %q", query.sql, "GROUP BY")
	}
	if !strings.Contains(query.sql, "UNION") {
		t.Errorf("%q does not contain %q", query.sql, "UNION")
	}
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != 5 {
		t.Errorf("got %v, want %v", query.params[0], 5)
	}
}

// TestSelectQuery_Except_Multiple tests multiple EXCEPT operations
func TestSelectQuery_Except_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("all_users")
	q2 := qb.Select("id").From("banned_users")
	q3 := qb.Select("id").From("suspended_users")

	query := q1.Except(q2).Except(q3).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Should have 2 EXCEPT keywords
	exceptCount := 0
	for i := 0; i < len(query.sql)-6; i++ {
		if i+6 <= len(query.sql) && query.sql[i:i+6] == "EXCEPT" {
			exceptCount++
		}
	}
	if exceptCount != 2 {
		t.Errorf("got %v, want %v", exceptCount, 2)
	}
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
	if unionAll == nil {
		t.Fatal("expected non-nil")
	}

	// UnionAll should use "UNION ALL" keyword
	if !strings.Contains(unionAll.sql, "UNION ALL") {
		t.Errorf("%q does not contain %q", unionAll.sql, "UNION ALL")
	}
	if strings.Contains(unionAll.sql, "UNION (") { // Not plain UNION
		t.Errorf("%q should not contain %q", unionAll.sql, "UNION (")
	}
}

// TestSelectQuery_Union_Empty_Result tests UNION with queries that may return empty results
func TestSelectQuery_Union_Empty_Result(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	q1 := qb.Select("id").From("users").Where("1 = ?", 0) // Always false
	q2 := qb.Select("id").From("users").Where("active = ?", true)

	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Both queries should be in SQL even if one is empty
	if !strings.Contains(query.sql, "UNION") {
		t.Errorf("%q does not contain %q", query.sql, "UNION")
	}
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
}
