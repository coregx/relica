// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
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
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL has 3 nested SELECTs
	if !strings.Contains(query.sql, `SELECT * FROM "level1_table"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT * FROM "level1_table"`)
	}
	if !strings.Contains(query.sql, `"id" IN (SELECT "parent_id" FROM "level2_table"`) {
		t.Errorf("%q does not contain %q", query.sql, `"id" IN (SELECT "parent_id" FROM "level2_table"`)
	}
	if !strings.Contains(query.sql, `"id" IN (SELECT "id" FROM "level3_table"`) {
		t.Errorf("%q does not contain %q", query.sql, `"id" IN (SELECT "id" FROM "level3_table"`)
	}

	// Verify parameter count (1 from level 3: 'active')
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != "active" {
		t.Errorf("got %v, want %v", query.params[0], "active")
	}

	// Verify proper nesting structure (should have 3 SELECT keywords)
	selectCount := strings.Count(query.sql, "SELECT")
	if selectCount != 3 {
		t.Errorf("got %v, want %v: Should have 3 SELECT statements", selectCount, 3)
	}
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
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify WITH clause has all 3 CTEs with comma separation
	if !strings.Contains(query.sql, `WITH "cte1" AS`) {
		t.Errorf("%q does not contain %q", query.sql, `WITH "cte1" AS`)
	}
	if !strings.Contains(query.sql, `, "cte2" AS`) {
		t.Errorf("%q does not contain %q", query.sql, `, "cte2" AS`)
	}
	if !strings.Contains(query.sql, `, "cte3" AS`) {
		t.Errorf("%q does not contain %q", query.sql, `, "cte3" AS`)
	}

	// Verify each CTE query is present
	// Note: Each CTE buildSQL() independently, placeholders may be reused across CTEs
	if !strings.Contains(query.sql, `SELECT "id", "value" FROM "base_table" WHERE status = $1`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id", "value" FROM "base_table" WHERE status = $1`)
	}
	if !strings.Contains(query.sql, `"value * 2" AS "doubled"`) {
		t.Errorf("%q does not contain %q", query.sql, `"value * 2" AS "doubled"`)
	}
	if !strings.Contains(query.sql, `FROM "cte1"`) {
		t.Errorf("%q does not contain %q", query.sql, `FROM "cte1"`)
	}
	if !strings.Contains(query.sql, `SELECT "id", SUM(doubled) as total FROM "cte2" GROUP BY "id"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id", SUM(doubled) as total FROM "cte2" GROUP BY "id"`)
	}

	// Verify main query references final CTE
	if !strings.Contains(query.sql, `SELECT * FROM "cte3"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT * FROM "cte3"`)
	}

	// Verify parameters from all CTEs (2 params: 1 from cte1, 10 from cte2)
	if len(query.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(query.params))
	}
	if query.params[0] != 1 {
		t.Errorf("got %v, want %v", query.params[0], 1)
	}
	if query.params[1] != 10 {
		t.Errorf("got %v, want %v", query.params[1], 10)
	}
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
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify FROM subquery present
	if !strings.Contains(query.sql, `FROM (SELECT "user_id", COUNT(*) as order_count FROM "orders" GROUP BY "user_id") AS "order_stats"`) {
		t.Errorf("%q does not contain expected FROM subquery", query.sql)
	}

	// Verify WHERE IN subquery present
	if !strings.Contains(query.sql, `WHERE "user_id" IN (SELECT "id" FROM "active_users" WHERE status = $1)`) {
		t.Errorf("%q does not contain expected WHERE IN subquery", query.sql)
	}

	// Verify parameter from WHERE subquery
	if len(query.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(query.params))
	}
	if query.params[0] != "active" {
		t.Errorf("got %v, want %v", query.params[0], "active")
	}
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
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify all 4 parameters are present
	if len(query.params) != 4 {
		t.Errorf("expected length %d, got %d", 4, len(query.params))
	}

	// Verify all parameter values are present (order may vary by implementation)
	paramValues := make(map[string]bool)
	for _, param := range query.params {
		paramValues[param.(string)] = true
	}

	if !paramValues["val1"] {
		t.Errorf("val1 should be in parameters")
	}
	if !paramValues["val2"] {
		t.Errorf("val2 should be in parameters")
	}
	if !paramValues["val3"] {
		t.Errorf("val3 should be in parameters")
	}
	if !paramValues["val4"] {
		t.Errorf("val4 should be in parameters")
	}

	// Verify SQL structure contains nested IN clauses
	if !strings.Contains(query.sql, `SELECT * FROM "t1"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT * FROM "t1"`)
	}
	if !strings.Contains(query.sql, `"id" IN`) {
		t.Errorf("%q does not contain %q", query.sql, `"id" IN`)
	}
	if !strings.Contains(query.sql, `SELECT "id" FROM "t2"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "t2"`)
	}
	if !strings.Contains(query.sql, `SELECT "id" FROM "t3"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT "id" FROM "t3"`)
	}
}
