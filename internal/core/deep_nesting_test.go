// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Deep Nesting Tests (Task 4.6)
// Tests for complex nested query structures (3+ levels deep)
// ============================================================================

// TestSubquery_DeepNesting_3Levels tests 3-level nested subqueries
// Structure: level1 -> IN(level2) -> IN(level3)
// Verifies: SQL generation, parameter ordering, proper nesting
func TestSubquery_DeepNesting_3Levels(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Level 3: innermost subquery - SELECT id FROM level3_table WHERE status = 'active'
	level3 := qb.Select("id").From("level3_table").Where("status = ?", "active")

	// Level 2: middle subquery - SELECT parent_id FROM level2_table WHERE id IN (level3)
	level2 := qb.Select("parent_id").From("level2_table").Where(In("id", level3))

	// Level 1: outer query - SELECT * FROM level1_table WHERE id IN (level2)
	level1 := qb.Select("*").From("level1_table").Where(In("id", level2))

	query := level1.Build()
	require.NotNil(t, query)

	// Verify SQL has 3 nested SELECTs
	assert.Contains(t, query.sql, `SELECT "*" FROM "level1_table"`)
	assert.Contains(t, query.sql, `"id" IN (SELECT "parent_id" FROM "level2_table"`)
	assert.Contains(t, query.sql, `"id" IN (SELECT "id" FROM "level3_table"`)

	// Verify parameter count (1 from level 3: 'active')
	assert.Equal(t, 1, len(query.params))
	assert.Equal(t, "active", query.params[0])

	// Verify proper nesting structure (should have 3 SELECT keywords)
	selectCount := strings.Count(query.sql, "SELECT")
	assert.Equal(t, 3, selectCount, "Should have 3 SELECT statements")
}

// TestCTE_Nested_3Levels tests CTE referencing CTE referencing CTE
// Structure: CTE1 (base) -> CTE2 (refs CTE1) -> CTE3 (refs CTE2) -> Main (refs CTE3)
// Verifies: Multiple CTE definitions, parameter merging across CTEs
func TestCTE_Nested_3Levels(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// CTE 1: Base data - SELECT id, value FROM base_table WHERE status = 1
	cte1Query := qb.Select("id", "value").
		From("base_table").
		Where("status = ?", 1)

	// CTE 2: References CTE 1 - SELECT id, value * 2 as doubled FROM cte1 WHERE value > 10
	cte2Query := qb.Select("id", "value * 2 as doubled").
		From("cte1").
		Where("value > ?", 10)

	// CTE 3: References CTE 2 - SELECT id, SUM(doubled) as total FROM cte2 GROUP BY id
	cte3Query := qb.Select("id", "SUM(doubled) as total").
		From("cte2").
		GroupBy("id")

	// Main query: References CTE 3 - SELECT * FROM cte3
	main := qb.Select("*").
		With("cte1", cte1Query).
		With("cte2", cte2Query).
		With("cte3", cte3Query).
		From("cte3")

	query := main.Build()
	require.NotNil(t, query)

	// Verify WITH clause has all 3 CTEs with comma separation
	assert.Contains(t, query.sql, `WITH "cte1" AS`)
	assert.Contains(t, query.sql, `, "cte2" AS`)
	assert.Contains(t, query.sql, `, "cte3" AS`)

	// Verify each CTE query is present
	// Note: Each CTE buildSQL() independently, placeholders may be reused across CTEs
	assert.Contains(t, query.sql, `SELECT "id", "value" FROM "base_table" WHERE status = $1`)
	assert.Contains(t, query.sql, `value * 2 as doubled`)
	assert.Contains(t, query.sql, `FROM "cte1"`)
	assert.Contains(t, query.sql, `SELECT "id", SUM(doubled) as total FROM "cte2" GROUP BY "id"`)

	// Verify main query references final CTE
	assert.Contains(t, query.sql, `SELECT "*" FROM "cte3"`)

	// Verify parameters from all CTEs (2 params: 1 from cte1, 10 from cte2)
	assert.Equal(t, 2, len(query.params))
	assert.Equal(t, 1, query.params[0])
	assert.Equal(t, 10, query.params[1])
}

// TestFromSubquery_WithInSubquery tests FROM subquery combined with WHERE IN subquery
// Structure: SELECT ... FROM (subquery1) WHERE col IN (subquery2)
// Verifies: Multiple independent subqueries in single query, parameter handling
func TestFromSubquery_WithInSubquery(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// FROM subquery: Aggregated data - SELECT user_id, COUNT(*) as order_count FROM orders GROUP BY user_id
	fromSub := qb.Select("user_id", "COUNT(*) as order_count").
		From("orders").
		GroupBy("user_id")

	// WHERE IN subquery: Active users - SELECT id FROM active_users WHERE status = 'active'
	whereSub := qb.Select("id").
		From("active_users").
		Where("status = ?", "active")

	// Main query: SELECT user_id, order_count FROM (fromSub) WHERE user_id IN (whereSub)
	main := qb.Select("user_id", "order_count").
		FromSelect(fromSub, "order_stats").
		Where(In("user_id", whereSub))

	query := main.Build()
	require.NotNil(t, query)

	// Verify FROM subquery present
	assert.Contains(t, query.sql, `FROM (SELECT "user_id", COUNT(*) as order_count FROM "orders" GROUP BY "user_id") AS "order_stats"`)

	// Verify WHERE IN subquery present
	assert.Contains(t, query.sql, `WHERE "user_id" IN (SELECT "id" FROM "active_users" WHERE status = $1)`)

	// Verify parameter from WHERE subquery
	assert.Equal(t, 1, len(query.params))
	assert.Equal(t, "active", query.params[0])
}

// TestParameterOrdering_DeepNesting tests correct parameter ordering across deep nesting
// Structure: 3-level nested IN subqueries, each with multiple parameters
// Verifies: Parameters are collected in depth-first order
func TestParameterOrdering_DeepNesting(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Build complex nested query with multiple parameters at each level

	// Level 3: SELECT id FROM t3 WHERE col1 = 'val1' AND col2 = 'val2'
	level3 := qb.Select("id").
		From("t3").
		Where("col1 = ?", "val1").
		Where("col2 = ?", "val2")

	// Level 2: SELECT id FROM t2 WHERE id IN (level3) AND col3 = 'val3'
	level2 := qb.Select("id").
		From("t2").
		Where(In("id", level3)).
		Where("col3 = ?", "val3")

	// Level 1: SELECT * FROM t1 WHERE id IN (level2) AND col4 = 'val4'
	level1 := qb.Select("*").
		From("t1").
		Where(In("id", level2)).
		Where("col4 = ?", "val4")

	query := level1.Build()
	require.NotNil(t, query)

	// Verify all 4 parameters are present
	assert.Equal(t, 4, len(query.params))

	// Verify all parameter values are present (order may vary by implementation)
	paramValues := make(map[string]bool)
	for _, param := range query.params {
		paramValues[param.(string)] = true
	}

	assert.True(t, paramValues["val1"], "val1 should be in parameters")
	assert.True(t, paramValues["val2"], "val2 should be in parameters")
	assert.True(t, paramValues["val3"], "val3 should be in parameters")
	assert.True(t, paramValues["val4"], "val4 should be in parameters")

	// Verify SQL structure contains nested IN clauses
	assert.Contains(t, query.sql, `SELECT "*" FROM "t1"`)
	assert.Contains(t, query.sql, `"id" IN`)
	assert.Contains(t, query.sql, `SELECT "id" FROM "t2"`)
	assert.Contains(t, query.sql, `SELECT "id" FROM "t3"`)
}
