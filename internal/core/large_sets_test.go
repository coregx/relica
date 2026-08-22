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
// Large Result Sets Tests (Task 4.7)
// Tests for handling large numbers of queries, CTEs, and result sets
// ============================================================================

// TestUnion_Many_Queries tests UNION of 10+ queries
// Verifies: SQL generation with many UNIONs, parameter ordering across queries
// Note: This is a UNIT test - verifies SQL generation, not actual DB execution
func TestUnion_Many_Queries(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Create 15 queries to UNION
	queries := make([]*SelectQuery, 15)
	for i := 0; i < 15; i++ {
		q := qb.Select("id", "name").
			From("table").
			Where("category = ?", i)
		queries[i] = q
	}

	// Chain UNIONs: q1 UNION q2 UNION q3 ... UNION q15
	main := queries[0]
	for i := 1; i < 15; i++ {
		main = main.Union(queries[i])
	}

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify 15 SELECTs are present
	selectCount := strings.Count(query.sql, "SELECT")
	if selectCount != 15 {
		t.Errorf("Should have 15 SELECT statements: got %v, want %v", selectCount, 15)
	}

	// Verify 14 UNION keywords (15 queries = 14 UNIONs)
	unionCount := strings.Count(query.sql, "UNION")
	// Note: UNION ALL also contains "UNION", so we need to be careful
	// Count only standalone UNION (not UNION ALL)
	unionAllCount := strings.Count(query.sql, "UNION ALL")
	actualUnionCount := unionCount - unionAllCount
	if actualUnionCount != 14 {
		t.Errorf("Should have 14 UNION keywords: got %v, want %v", actualUnionCount, 14)
	}

	// Verify all parameters present (15 categories: 0-14)
	if len(query.params) != 15 {
		t.Errorf("got %v, want %v", len(query.params), 15)
	}

	// Verify parameter values (categories 0-14)
	for i := 0; i < 15; i++ {
		found := false
		for _, p := range query.params {
			if p == i {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Should contain category %d: %q does not contain %q", i, query.params, i)
		}
	}

	// Verify SQL structure contains table name and column names
	if !strings.Contains(query.sql, `"table"`) {
		t.Errorf("%q does not contain %q", query.sql, `"table"`)
	}
	if !strings.Contains(query.sql, `"id"`) {
		t.Errorf("%q does not contain %q", query.sql, `"id"`)
	}
	if !strings.Contains(query.sql, `"name"`) {
		t.Errorf("%q does not contain %q", query.sql, `"name"`)
	}
}

// TestRecursiveCTE_ManyLevels tests recursive CTE with LIMIT to prevent infinite recursion
// Verifies: WITH RECURSIVE syntax, termination condition, parameter handling
// Note: This is a UNIT test - verifies SQL generation, not actual recursion execution
func TestRecursiveCTE_ManyLevels(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Anchor query: level 0 - SELECT 1 as level
	anchor := qb.Select("1 as level")

	// Recursive query: increment level - SELECT level + 1 FROM numbers WHERE level < 1000
	recursive := qb.Select("level + 1").
		From("numbers").
		Where("level < ?", 1000)

	// Combine with UNION ALL
	cte := anchor.UnionAll(recursive)

	// Main query: SELECT * FROM numbers LIMIT 1000
	main := qb.Select("*").
		WithRecursive("numbers", cte).
		From("numbers").
		Limit(1000)

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify WITH RECURSIVE keyword
	if !strings.Contains(query.sql, "WITH RECURSIVE") {
		t.Errorf("%q does not contain %q", query.sql, "WITH RECURSIVE")
	}

	// Verify CTE name
	if !strings.Contains(query.sql, `"numbers" AS`) {
		t.Errorf("%q does not contain %q", query.sql, `"numbers" AS`)
	}

	// Verify UNION ALL (recursive CTEs must use UNION ALL)
	if !strings.Contains(query.sql, "UNION ALL") {
		t.Errorf("%q does not contain %q", query.sql, "UNION ALL")
	}

	// Verify anchor query (1 AS "level" — now properly quoted)
	if !strings.Contains(query.sql, `AS "level"`) {
		t.Errorf("%q does not contain %q", query.sql, `AS "level"`)
	}

	// Verify recursive query structure
	if !strings.Contains(query.sql, "level + 1") {
		t.Errorf("%q does not contain %q", query.sql, "level + 1")
	}
	if !strings.Contains(query.sql, `FROM "numbers"`) {
		t.Errorf("%q does not contain %q", query.sql, `FROM "numbers"`)
	}

	// Verify termination condition (level < 1000)
	if !strings.Contains(query.sql, "level <") {
		t.Errorf("%q does not contain %q", query.sql, "level <")
	}

	// Verify LIMIT clause in main query (prevents excessive output)
	if !strings.Contains(query.sql, "LIMIT") {
		t.Errorf("%q does not contain %q", query.sql, "LIMIT")
	}

	// Verify parameter (1000 from WHERE clause)
	if len(query.params) != 1 {
		t.Errorf("got %v, want %v", len(query.params), 1)
	}
	if query.params[0] != 1000 {
		t.Errorf("got %v, want %v", query.params[0], 1000)
	}
}

// TestInSubquery_LargeList tests IN subquery that could return many values
// Verifies: SQL generation for potentially large IN lists, subquery handling
// Note: This is a UNIT test - verifies SQL generation, not actual large result set
func TestInSubquery_LargeList(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Create subquery that could return 1000+ IDs
	// In real usage, this might be: SELECT id FROM large_table WHERE ...
	// For testing, we just verify SQL generation works
	sub := qb.Select("id").From("large_table")
	// No WHERE clause - implies potentially large result set

	// Main query: SELECT * FROM main_table WHERE id IN (SELECT id FROM large_table)
	main := qb.Select("*").
		From("main_table").
		Where(In("id", sub))

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL generated correctly
	if !strings.Contains(query.sql, `SELECT * FROM "main_table"`) {
		t.Errorf("%q does not contain %q", query.sql, `SELECT * FROM "main_table"`)
	}
	if !strings.Contains(query.sql, `"id" IN (SELECT "id" FROM "large_table")`) {
		t.Errorf("%q does not contain %q", query.sql, `"id" IN (SELECT "id" FROM "large_table")`)
	}

	// No parameters in this case (no WHERE in subquery)
	if len(query.params) != 0 {
		t.Errorf("got %v, want %v", len(query.params), 0)
	}

	// Test passed if SQL generation completes without panic or error
	// This verifies the query builder can handle potentially large IN lists
}

// TestInSubquery_LargeList_WithFilter tests IN subquery with filtering
// This is a more realistic scenario with WHERE clause
func TestInSubquery_LargeList_WithFilter(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Subquery with filter (still potentially large)
	sub := qb.Select("id").
		From("large_table").
		Where("status = ?", "active").
		Limit(1000) // LIMIT to prevent excessive results

	// Main query
	main := qb.Select("*").
		From("main_table").
		Where(In("id", sub))

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(query.sql, `"id" IN (SELECT "id" FROM "large_table" WHERE status = $1 LIMIT 1000)`) {
		t.Errorf("%q does not contain %q", query.sql, `"id" IN (SELECT "id" FROM "large_table" WHERE status = $1 LIMIT 1000)`)
	}

	// Verify parameter
	if len(query.params) != 1 {
		t.Errorf("got %v, want %v", len(query.params), 1)
	}
	if query.params[0] != "active" {
		t.Errorf("got %v, want %v", query.params[0], "active")
	}
}

// TestUnionAll_Many_Queries tests UNION ALL with many queries (faster than UNION)
// UNION ALL doesn't remove duplicates, so it's faster for large result sets
func TestUnionAll_Many_Queries(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Create 20 queries to UNION ALL
	queries := make([]*SelectQuery, 20)
	for i := 0; i < 20; i++ {
		q := qb.Select("id").
			From("table").
			Where("partition = ?", i)
		queries[i] = q
	}

	// Chain UNION ALLs
	main := queries[0]
	for i := 1; i < 20; i++ {
		main = main.UnionAll(queries[i])
	}

	query := main.Build()
	if query == nil {
		t.Fatal("expected non-nil")
	}

	// Verify 20 SELECTs
	selectCount := strings.Count(query.sql, "SELECT")
	if selectCount != 20 {
		t.Errorf("got %v, want %v", selectCount, 20)
	}

	// Verify 19 UNION ALL keywords (20 queries = 19 UNION ALLs)
	unionAllCount := strings.Count(query.sql, "UNION ALL")
	if unionAllCount != 19 {
		t.Errorf("got %v, want %v", unionAllCount, 19)
	}

	// Verify 20 parameters
	if len(query.params) != 20 {
		t.Errorf("got %v, want %v", len(query.params), 20)
	}

	// Verify parameter values (partitions 0-19)
	for i := 0; i < 20; i++ {
		found := false
		for _, p := range query.params {
			if p == i {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q does not contain %q", query.params, i)
		}
	}
}
