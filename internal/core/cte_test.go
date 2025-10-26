package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, query)

	// Verify WITH clause structure
	assert.Contains(t, query.sql, `WITH "order_totals" AS`)
	assert.Contains(t, query.sql, `SELECT "user_id", SUM(total) as total FROM "orders" GROUP BY "user_id"`)
	assert.Contains(t, query.sql, `SELECT * FROM "order_totals" WHERE total > $1`)
	assert.Len(t, query.params, 1)
	assert.Equal(t, 1000, query.params[0])
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
	require.NotNil(t, query)

	// Verify multiple CTEs with comma separation
	assert.Contains(t, query.sql, `WITH "active_users" AS`)
	assert.Contains(t, query.sql, `, "recent_orders" AS`)
	assert.Contains(t, query.sql, `FROM "active_users" AS "u"`)
	assert.Contains(t, query.sql, `INNER JOIN "recent_orders" AS "o"`)
	assert.Len(t, query.params, 2)
	assert.Equal(t, "active", query.params[0])
	assert.Equal(t, "2024-01-01", query.params[1])
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
	require.NotNil(t, query)

	// Verify parameter order: CTE params come first, then main query params
	assert.Len(t, query.params, 2)
	assert.Equal(t, 100, query.params[0])           // CTE param
	assert.Equal(t, "electronics", query.params[1]) // Main query param

	// Verify placeholder numbering
	assert.Contains(t, query.sql, "price > $1")    // CTE uses $1
	assert.Contains(t, query.sql, "category = $2") // Main uses $2
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
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `WITH "high_value_customers" AS`)
	assert.Contains(t, query.sql, `SELECT "customer_id" FROM "orders" GROUP BY "customer_id" HAVING SUM(total) > $1`)
	assert.Contains(t, query.sql, `FROM "customers" WHERE id IN (SELECT customer_id FROM high_value_customers)`)
	assert.Len(t, query.params, 1)
	assert.Equal(t, 10000, query.params[0])
}

// TestWith_EmptyName_Panics tests panic when CTE name is empty
func TestWith_EmptyName_Panics(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	cte := qb.Select("id").From("users")

	assert.PanicsWithValue(t, "CTE name cannot be empty", func() {
		qb.Select("*").With("", cte)
	})
}

// TestWith_NilQuery_Panics tests panic when CTE query is nil
func TestWith_NilQuery_Panics(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	assert.PanicsWithValue(t, "CTE query cannot be nil", func() {
		qb.Select("*").With("my_cte", nil)
	})
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
			require.NotNil(t, query)

			// Verify quoting style
			assert.Contains(t, query.sql, "WITH "+tt.expectedQuote+"adults"+tt.expectedQuote+" AS")

			// Verify placeholder style
			if tt.dialectName == "postgres" {
				assert.Contains(t, query.sql, "age > $1")
			} else {
				assert.Contains(t, query.sql, "age > ?")
			}

			assert.Len(t, query.params, 1)
			assert.Equal(t, 18, query.params[0])
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
	require.NotNil(t, query)

	// Verify WITH RECURSIVE keyword
	assert.Contains(t, query.sql, `WITH RECURSIVE "hierarchy" AS`)
	assert.Contains(t, query.sql, `WHERE manager_id IS NULL`)
	assert.Contains(t, query.sql, `UNION ALL`)
	assert.Contains(t, query.sql, `INNER JOIN "hierarchy" AS "h"`)
	assert.Contains(t, query.sql, `ORDER BY "level", "name"`)
}

// TestWithRecursive_WithoutUnion_Panics tests panic when recursive CTE lacks UNION
func TestWithRecursive_WithoutUnion_Panics(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Query without UNION (invalid for recursive CTE)
	invalidCTE := qb.Select("id", "name").From("employees")

	assert.PanicsWithValue(t, "recursive CTE requires UNION or UNION ALL", func() {
		qb.Select("*").WithRecursive("hierarchy", invalidCTE)
	})
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
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `WITH RECURSIVE "numbers" AS`)
	assert.Contains(t, query.sql, `SELECT 1 as n`)
	assert.Contains(t, query.sql, `UNION ALL`)
	assert.Contains(t, query.sql, `SELECT "n + 1" FROM "numbers" WHERE n < $1`)
	assert.Len(t, query.params, 1)
	assert.Equal(t, 10, query.params[0])
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
	require.NotNil(t, query)

	// Verify parameter order: anchor → recursive → main
	assert.Len(t, query.params, 3)
	assert.Equal(t, 0, query.params[0]) // Anchor param
	assert.Equal(t, 5, query.params[1]) // Recursive param
	assert.Equal(t, 2, query.params[2]) // Main query param

	// Verify placeholders
	assert.Contains(t, query.sql, "parent_id = $1") // Anchor
	assert.Contains(t, query.sql, "t.depth < $2")   // Recursive
	assert.Contains(t, query.sql, "depth >= $3")    // Main
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
			require.NotNil(t, query)

			// Verify WITH RECURSIVE with proper quoting
			assert.Contains(t, query.sql, "WITH RECURSIVE "+tt.expectedQuote+"seq"+tt.expectedQuote+" AS")
			assert.Contains(t, query.sql, "UNION ALL")
			assert.Len(t, query.params, 1)
			assert.Equal(t, 5, query.params[0])
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
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `WITH "top_products" AS`)
	assert.Contains(t, query.sql, `INNER JOIN "top_products" AS "t"`)
	assert.Contains(t, query.sql, `ORDER BY "t"."total_sold" DESC`)
	assert.Len(t, query.params, 1)
	assert.Equal(t, 100, query.params[0])
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
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `WITH "active_users" AS`)
	assert.Contains(t, query.sql, `status = $1`)
	assert.Contains(t, query.sql, `created_at > $2`)
	assert.Contains(t, query.sql, `WHERE id IN (SELECT user_id FROM orders WHERE created_at > $2)`)
	assert.Len(t, query.params, 2)
	assert.Equal(t, "active", query.params[0])
	assert.Equal(t, "2024-01-01", query.params[1])
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
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `WITH "expensive" AS`)
	assert.Contains(t, query.sql, `price > $1`)
	assert.Contains(t, query.sql, `UNION`)
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
	assert.Equal(t, firstIndex, lastIndex, "CTE should appear only once")
	assert.Len(t, query.params, 3)
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
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `WITH "user_stats" AS`)
	assert.Contains(t, query.sql, `, "high_spenders" AS`)
	assert.Contains(t, query.sql, `FROM "user_stats" WHERE total_spent > $1`)
	assert.Contains(t, query.sql, `INNER JOIN "high_spenders"`)
	assert.Len(t, query.params, 1)
	assert.Equal(t, 5000, query.params[0])
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
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `WITH RECURSIVE "tree" AS`)
	assert.Contains(t, query.sql, `UNION ALL`)
	assert.Contains(t, query.sql, `level < $2`)
	assert.Contains(t, query.sql, `level > $3`)
	assert.Contains(t, query.sql, `LIMIT 100`)
	assert.Len(t, query.params, 3)
	assert.Equal(t, 1, query.params[0])
	assert.Equal(t, 10, query.params[1])
	assert.Equal(t, 1, query.params[2])
}
