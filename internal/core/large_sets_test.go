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
	require.NotNil(t, query)

	// Verify 15 SELECTs are present
	selectCount := strings.Count(query.sql, "SELECT")
	assert.Equal(t, 15, selectCount, "Should have 15 SELECT statements")

	// Verify 14 UNION keywords (15 queries = 14 UNIONs)
	unionCount := strings.Count(query.sql, "UNION")
	// Note: UNION ALL also contains "UNION", so we need to be careful
	// Count only standalone UNION (not UNION ALL)
	unionAllCount := strings.Count(query.sql, "UNION ALL")
	actualUnionCount := unionCount - unionAllCount
	assert.Equal(t, 14, actualUnionCount, "Should have 14 UNION keywords")

	// Verify all parameters present (15 categories: 0-14)
	assert.Equal(t, 15, len(query.params))

	// Verify parameter values (categories 0-14)
	for i := 0; i < 15; i++ {
		assert.Contains(t, query.params, i, "Should contain category %d", i)
	}

	// Verify SQL structure contains table name and column names
	assert.Contains(t, query.sql, `"table"`)
	assert.Contains(t, query.sql, `"id"`)
	assert.Contains(t, query.sql, `"name"`)
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
	require.NotNil(t, query)

	// Verify WITH RECURSIVE keyword
	assert.Contains(t, query.sql, "WITH RECURSIVE")

	// Verify CTE name
	assert.Contains(t, query.sql, `"numbers" AS`)

	// Verify UNION ALL (recursive CTEs must use UNION ALL)
	assert.Contains(t, query.sql, "UNION ALL")

	// Verify anchor query (1 as level)
	assert.Contains(t, query.sql, "1 as level")

	// Verify recursive query structure
	assert.Contains(t, query.sql, "level + 1")
	assert.Contains(t, query.sql, `FROM "numbers"`)

	// Verify termination condition (level < 1000)
	assert.Contains(t, query.sql, "level <")

	// Verify LIMIT clause in main query (prevents excessive output)
	assert.Contains(t, query.sql, "LIMIT")

	// Verify parameter (1000 from WHERE clause)
	assert.Equal(t, 1, len(query.params))
	assert.Equal(t, 1000, query.params[0])
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
	require.NotNil(t, query)

	// Verify SQL generated correctly
	assert.Contains(t, query.sql, `SELECT "*" FROM "main_table"`)
	assert.Contains(t, query.sql, `"id" IN (SELECT "id" FROM "large_table")`)

	// No parameters in this case (no WHERE in subquery)
	assert.Equal(t, 0, len(query.params))

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
	require.NotNil(t, query)

	// Verify SQL structure
	assert.Contains(t, query.sql, `"id" IN (SELECT "id" FROM "large_table" WHERE status = $1 LIMIT 1000)`)

	// Verify parameter
	assert.Equal(t, 1, len(query.params))
	assert.Equal(t, "active", query.params[0])
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
	require.NotNil(t, query)

	// Verify 20 SELECTs
	selectCount := strings.Count(query.sql, "SELECT")
	assert.Equal(t, 20, selectCount)

	// Verify 19 UNION ALL keywords (20 queries = 19 UNION ALLs)
	unionAllCount := strings.Count(query.sql, "UNION ALL")
	assert.Equal(t, 19, unionAllCount)

	// Verify 20 parameters
	assert.Equal(t, 20, len(query.params))

	// Verify parameter values (partitions 0-19)
	for i := 0; i < 20; i++ {
		assert.Contains(t, query.params, i)
	}
}
