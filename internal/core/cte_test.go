package core

import (
	"strings"
	"testing"
)

// ============================================================================
// Basic CTE Tests (Task 3.5)
// ============================================================================

// TestWith_SingleCTE tests a single WITH clause with PostgreSQL
func TestWith_SingleCTE(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// CTE: SELECT user_id, SUM(total) as total FROM orders GROUP BY user_id
	cte := qb.Select("user_id", "SUM(total) as total").
		From("orders").
		GroupBy("user_id")

	// Main: SELECT * FROM order_totals WHERE total > 1000
	main := qb.Select("*").
		With("order_totals", cte).
		From("order_totals").
		Where("total > ?", 1000)

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify WITH clause structure
	checks := []string{
		`WITH "order_totals" AS`,
		`SELECT "user_id", SUM(total) as total FROM "orders" GROUP BY "user_id"`,
		`SELECT * FROM "order_totals" WHERE total > $1`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != 1000 {
		t.Errorf("got %v, want %v", query.params[0], 1000)
	}
}

// TestWith_MultipleCTEs tests chaining multiple CTEs
func TestWith_MultipleCTEs(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// First CTE: active users
	cte1 := qb.Select("id", "name").
		From("users").
		Where("status = ?", "active")

	// Second CTE: recent orders
	cte2 := qb.Select("user_id", "COUNT(*) as order_count").
		From("orders").
		Where("created_at > ?", "2024-01-01").
		GroupBy("user_id")

	// Main query joins both CTEs
	main := qb.Select("u.name", "o.order_count").
		With("active_users", cte1).
		With("recent_orders", cte2).
		From("active_users u").
		InnerJoin("recent_orders o", "u.id = o.user_id")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify multiple CTEs with comma separation
	checks := []string{
		`WITH "active_users" AS`,
		`, "recent_orders" AS`,
		`FROM "active_users" AS "u"`,
		`INNER JOIN "recent_orders" AS "o"`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
	if query.params[0] != "active" {
		t.Errorf("got %v, want %v", query.params[0], "active")
	}
	if query.params[1] != "2024-01-01" {
		t.Errorf("got %v, want %v", query.params[1], "2024-01-01")
	}
}

// TestWith_ParameterMerging tests correct parameter ordering across CTE and main query
func TestWith_ParameterMerging(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// CTE with parameter
	cte := qb.Select("id").
		From("products").
		Where("price > ?", 100)

	// Main query with parameter
	main := qb.Select("*").
		With("expensive_products", cte).
		From("expensive_products").
		Where("category = ?", "electronics")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify parameter order: CTE params come first, then main query params
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
	if query.params[0] != 100 {
		t.Errorf("got %v, want %v", query.params[0], 100) // CTE param
	}
	if query.params[1] != "electronics" {
		t.Errorf("got %v, want %v", query.params[1], "electronics") // Main query param
	}

	// Verify placeholder numbering
	if !strings.Contains(query.sql, "price > $1") { // CTE uses $1
		t.Errorf("%q does not contain %q", query.sql, "price > $1")
	}
	if !strings.Contains(query.sql, "category = $2") { // Main uses $2
		t.Errorf("%q does not contain %q", query.sql, "category = $2")
	}
}

// TestWith_CTEReferencedInWhere tests CTE referenced in WHERE clause
func TestWith_CTEReferencedInWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// CTE: high value customers
	cte := qb.Select("customer_id").
		From("orders").
		GroupBy("customer_id").
		Having("SUM(total) > ?", 10000)

	// Main: get customer details for high value customers
	main := qb.Select("*").
		With("high_value_customers", cte).
		From("customers").
		Where("id IN (SELECT customer_id FROM high_value_customers)")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	checks := []string{
		`WITH "high_value_customers" AS`,
		`SELECT "customer_id" FROM "orders" GROUP BY "customer_id" HAVING SUM(total) > $1`,
		`FROM "customers" WHERE id IN (SELECT customer_id FROM high_value_customers)`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != 10000 {
		t.Errorf("got %v, want %v", query.params[0], 10000)
	}
}

// TestWith_EmptyName_Panics tests that an empty CTE name stores an error
// instead of panicking, and the error propagates through Build.
func TestWith_EmptyName_Panics(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	cte := qb.Select("id").From("users")

	sq := qb.Select("*").With("", cte)
	if sq.buildErr == nil {
		t.Error("empty CTE name must store a build error")
	}
	if !strings.Contains(sq.buildErr.Error(), "With()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "With()")
	}
	q := sq.Build()
	if q.prepErr == nil {
		t.Error("build error must propagate through Build()")
	}
}

// TestWith_NilQuery_Panics tests that a nil CTE query stores an error
// instead of panicking, and the error propagates through Build.
func TestWith_NilQuery_Panics(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("*").With("my_cte", nil)
	if sq.buildErr == nil {
		t.Error("nil CTE query must store a build error")
	}
	if !strings.Contains(sq.buildErr.Error(), "With()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "With()")
	}
	q := sq.Build()
	if q.prepErr == nil {
		t.Error("build error must propagate through Build()")
	}
}

// TestWith_AllDialects tests CTE with all three dialects
func TestWith_AllDialects(t *testing.T) {
	tests := []struct {
		name                string
		dialectName         string
		expectedQuote       string
		expectedPlaceholder string
	}{
		{
			name:                "PostgreSQL",
			dialectName:         "postgres",
			expectedQuote:       `"`,
			expectedPlaceholder: "$1",
		},
		{
			name:                "MySQL",
			dialectName:         "mysql",
			expectedQuote:       "`",
			expectedPlaceholder: "?",
		},
		{
			name:                "SQLite",
			dialectName:         "sqlite3",
			expectedQuote:       `"`,
			expectedPlaceholder: "?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialectName)
			qb := &QueryBuilder{db: db}

			cte := qb.Select("id").From("users").Where("age > ?", 18)
			main := qb.Select("*").With("adults", cte).From("adults")

			query := main.Build()
			if query == nil {
				t.Fatal("expected non-nil")
			}

			// Verify quoting style
			withClause := "WITH " + tt.expectedQuote + "adults" + tt.expectedQuote + " AS"
			if !strings.Contains(query.sql, withClause) {
				t.Errorf("%q does not contain %q", query.sql, withClause)
			}

			// Verify placeholder style
			if tt.dialectName == "postgres" {
				if !strings.Contains(query.sql, "age > $1") {
					t.Errorf("%q does not contain %q", query.sql, "age > $1")
				}
			} else {
				if !strings.Contains(query.sql, "age > ?") {
					t.Errorf("%q does not contain %q", query.sql, "age > ?")
				}
			}

			if len(query.params) != 1 {
				t.Errorf("expected length %d, got %d", 1, len(query.params))
			}
			if query.params[0] != 18 {
				t.Errorf("got %v, want %v", query.params[0], 18)
			}
		})
	}
}

// ============================================================================
// Recursive CTE Tests (Task 3.6)
// ============================================================================

// TestWithRecursive_OrganizationHierarchy tests classic recursive hierarchy query
func TestWithRecursive_OrganizationHierarchy(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Anchor: top-level employees (no manager)
	anchor := qb.Select("id", "name", "manager_id", "1 as level").
		From("employees").
		Where("manager_id IS NULL")

	// Recursive part: employees with managers
	recursive := qb.Select("e.id", "e.name", "e.manager_id", "h.level + 1").
		From("employees e").
		InnerJoin("hierarchy h", "e.manager_id = h.id")

	// Combine with UNION ALL
	cte := anchor.UnionAll(recursive)

	// Main query
	main := qb.Select("*").
		WithRecursive("hierarchy", cte).
		From("hierarchy").
		OrderBy("level", "name")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify WITH RECURSIVE keyword
	checks := []string{
		`WITH RECURSIVE "hierarchy" AS`,
		`WHERE manager_id IS NULL`,
		`UNION ALL`,
		`INNER JOIN "hierarchy" AS "h"`,
		`ORDER BY "level", "name"`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
}

// TestWithRecursive_WithoutUnion_Panics tests that a recursive CTE without UNION
// stores an error instead of panicking, and the error propagates through Build.
func TestWithRecursive_WithoutUnion_Panics(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Query without UNION (invalid for recursive CTE)
	invalidCTE := qb.Select("id", "name").From("employees")

	sq := qb.Select("*").WithRecursive("hierarchy", invalidCTE)
	if sq.buildErr == nil {
		t.Error("recursive CTE without UNION must store a build error")
	}
	if !strings.Contains(sq.buildErr.Error(), "WithRecursive()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "WithRecursive()")
	}
	q := sq.Build()
	if q.prepErr == nil {
		t.Error("build error must propagate through Build()")
	}
}

// TestWithRecursive_UnionAll tests recursive CTE with UNION ALL
func TestWithRecursive_UnionAll(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Number sequence: anchor = 1, recursive = n + 1
	anchor := qb.Select("1 as n")
	recursive := qb.Select("n + 1").From("numbers").Where("n < ?", 10)
	cte := anchor.UnionAll(recursive)

	main := qb.Select("*").
		WithRecursive("numbers", cte).
		From("numbers")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	checks := []string{
		`WITH RECURSIVE "numbers" AS`,
		`SELECT "1" AS "n"`,
		`UNION ALL`,
		`SELECT "n + 1" FROM "numbers" WHERE n < $1`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != 10 {
		t.Errorf("got %v, want %v", query.params[0], 10)
	}
}

// TestWithRecursive_ParameterMerging tests parameter ordering with recursive CTE
func TestWithRecursive_ParameterMerging(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Anchor with parameter
	anchor := qb.Select("id", "path", "1 as depth").
		From("categories").
		Where("parent_id = ?", 0) // Root categories

	// Recursive part with parameter
	recursive := qb.Select("c.id", "c.path", "t.depth + 1").
		From("categories c").
		InnerJoin("category_tree t", "c.parent_id = t.id").
		Where("t.depth < ?", 5) // Max depth

	cte := anchor.UnionAll(recursive)

	// Main query with its own parameter
	main := qb.Select("*").
		WithRecursive("category_tree", cte).
		From("category_tree").
		Where("depth >= ?", 2)

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify parameter order: anchor → recursive → main
	if len(query.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(query.params))
	}
	if query.params[0] != 0 {
		t.Errorf("got %v, want %v", query.params[0], 0) // Anchor param
	}
	if query.params[1] != 5 {
		t.Errorf("got %v, want %v", query.params[1], 5) // Recursive param
	}
	if query.params[2] != 2 {
		t.Errorf("got %v, want %v", query.params[2], 2) // Main query param
	}

	// Verify placeholders
	if !strings.Contains(query.sql, "parent_id = $1") { // Anchor
		t.Errorf("%q does not contain %q", query.sql, "parent_id = $1")
	}
	if !strings.Contains(query.sql, "t.depth < $2") { // Recursive
		t.Errorf("%q does not contain %q", query.sql, "t.depth < $2")
	}
	if !strings.Contains(query.sql, "depth >= $3") { // Main
		t.Errorf("%q does not contain %q", query.sql, "depth >= $3")
	}
}

// TestWithRecursive_AllDialects tests recursive CTE with all dialects
func TestWithRecursive_AllDialects(t *testing.T) {
	tests := []struct {
		name          string
		dialectName   string
		expectedQuote string
	}{
		{name: "PostgreSQL", dialectName: "postgres", expectedQuote: `"`},
		{name: "MySQL", dialectName: "mysql", expectedQuote: "`"},
		{name: "SQLite", dialectName: "sqlite3", expectedQuote: `"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialectName)
			qb := &QueryBuilder{db: db}

			anchor := qb.Select("1 as n")
			recursive := qb.Select("n + 1").From("seq").Where("n < ?", 5)
			cte := anchor.UnionAll(recursive)

			main := qb.Select("*").WithRecursive("seq", cte).From("seq")
			query := main.Build()
			if query == nil {
				t.Fatal("expected non-nil")
			}

			// Verify WITH RECURSIVE with proper quoting
			withClause := "WITH RECURSIVE " + tt.expectedQuote + "seq" + tt.expectedQuote + " AS"
			if !strings.Contains(query.sql, withClause) {
				t.Errorf("%q does not contain %q", query.sql, withClause)
			}
			if !strings.Contains(query.sql, "UNION ALL") {
				t.Errorf("%q does not contain %q", query.sql, "UNION ALL")
			}
			if len(query.params) != 1 {
				t.Errorf("expected length %d, got %d", 1, len(query.params))
			}
			if query.params[0] != 5 {
				t.Errorf("got %v, want %v", query.params[0], 5)
			}
		})
	}
}

// ============================================================================
// Combined Features Tests (Task 3.7)
// ============================================================================

// TestCTE_WithJoin tests CTE combined with JOIN in main query
func TestCTE_WithJoin(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// CTE: top products by sales
	cte := qb.Select("product_id", "SUM(quantity) as total_sold").
		From("order_items").
		GroupBy("product_id").
		Having("SUM(quantity) > ?", 100)

	// Main query: JOIN CTE with products table
	main := qb.Select("p.name", "t.total_sold").
		With("top_products", cte).
		From("products p").
		InnerJoin("top_products t", "p.id = t.product_id").
		OrderBy("t.total_sold DESC")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	checks := []string{
		`WITH "top_products" AS`,
		`INNER JOIN "top_products" AS "t"`,
		`ORDER BY "t"."total_sold" DESC`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != 100 {
		t.Errorf("got %v, want %v", query.params[0], 100)
	}
}

// TestCTE_WithSubquery tests CTE combined with subquery in WHERE clause
func TestCTE_WithSubquery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// CTE: active users
	cte := qb.Select("id", "name").
		From("users").
		Where("status = ?", "active")

	// Main: active users with recent orders (using plain SQL subquery for simplicity)
	main := qb.Select("*").
		With("active_users", cte).
		From("active_users").
		Where("id IN (SELECT user_id FROM orders WHERE created_at > ?)", "2024-01-01")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	checks := []string{
		`WITH "active_users" AS`,
		`status = $1`,
		`created_at > $2`,
		`WHERE id IN (SELECT user_id FROM orders WHERE created_at > $2)`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
	if query.params[0] != "active" {
		t.Errorf("got %v, want %v", query.params[0], "active")
	}
	if query.params[1] != "2024-01-01" {
		t.Errorf("got %v, want %v", query.params[1], "2024-01-01")
	}
}

// TestCTE_WithSetOperations tests CTE combined with UNION in main query
func TestCTE_WithSetOperations(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// CTE: expensive products
	cte := qb.Select("id", "name", "price").
		From("products").
		Where("price > ?", 1000)

	// Main query part 1: electronics from CTE
	q1 := qb.Select("*").
		With("expensive", cte).
		From("expensive").
		Where("category = ?", "electronics")

	// Main query part 2: furniture from CTE
	q2 := qb.Select("*").
		From("expensive").
		Where("category = ?", "furniture")

	// Combine with UNION
	query := q1.Union(q2).Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(query.sql, `WITH "expensive" AS`) {
		t.Errorf("%q does not contain %q", query.sql, `WITH "expensive" AS`)
	}
	if !strings.Contains(query.sql, `price > $1`) {
		t.Errorf("%q does not contain %q", query.sql, `price > $1`)
	}
	if !strings.Contains(query.sql, `UNION`) {
		t.Errorf("%q does not contain %q", query.sql, `UNION`)
	}
	// CTE appears only once at the beginning
	firstIndex := -1
	lastIndex := -1
	if idx := len(query.sql); idx > 0 {
		sql := query.sql
		for i := 0; i < len(sql)-len(`WITH "expensive"`); i++ {
			if sql[i:i+len(`WITH "expensive"`)] == `WITH "expensive"` {
				if firstIndex == -1 {
					firstIndex = i
				}
				lastIndex = i
			}
		}
	}
	// Verify CTE appears only once
	if firstIndex != lastIndex {
		t.Errorf("CTE should appear only once")
	}
	if len(query.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(query.params))
	}
}

// TestCTE_NestedCTEs tests CTE referencing another CTE
func TestCTE_NestedCTEs(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// First CTE: user statistics
	cte1 := qb.Select("user_id", "COUNT(*) as order_count", "SUM(total) as total_spent").
		From("orders").
		GroupBy("user_id")

	// Second CTE: references first CTE (high spenders)
	cte2 := qb.Select("user_id", "total_spent").
		From("user_stats").
		Where("total_spent > ?", 5000)

	// Main query: get user details for high spenders
	main := qb.Select("u.name", "h.total_spent").
		With("user_stats", cte1).
		With("high_spenders", cte2).
		From("users u").
		InnerJoin("high_spenders h", "u.id = h.user_id")

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	checks := []string{
		`WITH "user_stats" AS`,
		`, "high_spenders" AS`,
		`FROM "user_stats" WHERE total_spent > $1`,
		`INNER JOIN "high_spenders"`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != 5000 {
		t.Errorf("got %v, want %v", query.params[0], 5000)
	}
}

// TestCTE_ComplexRecursive tests complex recursive CTE with multiple features
func TestCTE_ComplexRecursive(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Anchor: start from specific node
	anchor := qb.Select("id", "parent_id", "name", "1 as level", "name as path").
		From("nodes").
		Where("id = ?", 1)

	// Recursive: traverse tree with path building
	recursive := qb.Select("n.id", "n.parent_id", "n.name", "t.level + 1", "t.path || '/' || n.name").
		From("nodes n").
		InnerJoin("tree t", "n.parent_id = t.id").
		Where("t.level < ?", 10)

	cte := anchor.UnionAll(recursive)

	// Main query with filtering and ordering
	main := qb.Select("*").
		WithRecursive("tree", cte).
		From("tree").
		Where("level > ?", 1).
		OrderBy("level", "name").
		Limit(100)

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	checks := []string{
		`WITH RECURSIVE "tree" AS`,
		`UNION ALL`,
		`level < $2`,
		`level > $3`,
		`LIMIT 100`,
	}
	for _, s := range checks {
		if !strings.Contains(query.sql, s) {
			t.Errorf("%q does not contain %q", query.sql, s)
		}
	}
	if len(query.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(query.params))
	}
	if query.params[0] != 1 {
		t.Errorf("got %v, want %v", query.params[0], 1)
	}
	if query.params[1] != 10 {
		t.Errorf("got %v, want %v", query.params[1], 10)
	}
	if query.params[2] != 1 {
		t.Errorf("got %v, want %v", query.params[2], 1)
	}
}
