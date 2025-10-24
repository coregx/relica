package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDB is defined in upsert_test.go to avoid duplication

// TestSelectQuery_OrderBy_Single tests ORDER BY with single column
func TestSelectQuery_OrderBy_Single(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("age DESC")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, `SELECT * FROM "users"`)
	assert.Contains(t, q.sql, ` ORDER BY "age" DESC`)
	assert.Empty(t, q.params, "ORDER BY should have no params")
}

// TestSelectQuery_OrderBy_Multiple tests ORDER BY with multiple columns in one call
func TestSelectQuery_OrderBy_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("status ASC", "created_at DESC", "id")

	q := query.Build()
	require.NotNil(t, q)

	// Verify all columns are present in ORDER BY
	assert.Contains(t, q.sql, `ORDER BY`)
	assert.Contains(t, q.sql, `"status" ASC`)
	assert.Contains(t, q.sql, `"created_at" DESC`)
	assert.Contains(t, q.sql, `"id"`) // Default ASC (not explicitly shown)

	// Verify order is preserved
	statusIdx := indexOf(q.sql, `"status"`)
	createdIdx := indexOf(q.sql, `"created_at"`)
	idIdx := lastIndexOf(q.sql, `"id"`)
	assert.Less(t, statusIdx, createdIdx, "status should come before created_at")
	assert.Less(t, createdIdx, idIdx, "created_at should come before id")
}

// TestSelectQuery_OrderBy_WithDirection tests ORDER BY with explicit ASC/DESC
func TestSelectQuery_OrderBy_WithDirection(t *testing.T) {
	tests := []struct {
		name          string
		orderBy       string
		expectedSQL   string
		expectedNoSQL string
	}{
		{
			name:        "ASC explicit",
			orderBy:     "name ASC",
			expectedSQL: `"name" ASC`,
		},
		{
			name:        "DESC explicit",
			orderBy:     "age DESC",
			expectedSQL: `"age" DESC`,
		},
		{
			name:          "No direction (defaults to ASC)",
			orderBy:       "created_at",
			expectedSQL:   `"created_at"`,
			expectedNoSQL: " DESC", // Should NOT have DESC
		},
		{
			name:        "lowercase asc",
			orderBy:     "status asc",
			expectedSQL: `"status" ASC`, // Should be normalized to uppercase
		},
		{
			name:        "lowercase desc",
			orderBy:     "priority desc",
			expectedSQL: `"priority" DESC`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB("postgres")
			qb := &QueryBuilder{db: db}

			query := qb.Select().
				From("users").
				OrderBy(tt.orderBy)

			q := query.Build()
			require.NotNil(t, q)

			assert.Contains(t, q.sql, tt.expectedSQL)
			if tt.expectedNoSQL != "" {
				assert.NotContains(t, q.sql, tt.expectedNoSQL)
			}
		})
	}
}

// TestSelectQuery_OrderBy_Chained tests multiple OrderBy() calls (chainable)
func TestSelectQuery_OrderBy_Chained(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("status ASC").
		OrderBy("age DESC").
		OrderBy("name")

	q := query.Build()
	require.NotNil(t, q)

	// All columns should be present
	assert.Contains(t, q.sql, `"status" ASC`)
	assert.Contains(t, q.sql, `"age" DESC`)
	assert.Contains(t, q.sql, `"name"`)

	// Verify order is preserved
	statusIdx := indexOf(q.sql, `"status"`)
	ageIdx := indexOf(q.sql, `"age"`)
	nameIdx := lastIndexOf(q.sql, `"name"`)
	assert.Less(t, statusIdx, ageIdx)
	assert.Less(t, ageIdx, nameIdx)
}

// TestSelectQuery_OrderBy_WithTablePrefix tests ORDER BY with table.column format
func TestSelectQuery_OrderBy_WithTablePrefix(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		InnerJoin("users u", "m.user_id = u.id").
		OrderBy("m.created_at DESC", "u.name ASC")

	q := query.Build()
	require.NotNil(t, q)

	// Both table and column should be quoted
	assert.Contains(t, q.sql, `"m"."created_at" DESC`)
	assert.Contains(t, q.sql, `"u"."name" ASC`)
}

// TestSelectQuery_Limit tests LIMIT clause
func TestSelectQuery_Limit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Limit(100)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, ` LIMIT 100`)
	assert.NotContains(t, q.sql, "OFFSET")
}

// TestSelectQuery_Offset tests OFFSET clause
func TestSelectQuery_Offset(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Offset(200)

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, ` OFFSET 200`)
	assert.NotContains(t, q.sql, "LIMIT")
}

// TestSelectQuery_Limit_And_Offset tests LIMIT and OFFSET together
func TestSelectQuery_Limit_And_Offset(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Limit(50).
		Offset(100)

	q := query.Build()
	require.NotNil(t, q)

	// LIMIT should come before OFFSET
	assert.Contains(t, q.sql, ` LIMIT 50`)
	assert.Contains(t, q.sql, ` OFFSET 100`)

	limitIdx := indexOf(q.sql, "LIMIT")
	offsetIdx := indexOf(q.sql, "OFFSET")
	assert.Less(t, limitIdx, offsetIdx, "LIMIT should come before OFFSET")
}

// TestSelectQuery_OrderBy_Limit_Offset_Combined tests all three features together
func TestSelectQuery_OrderBy_Limit_Offset_Combined(t *testing.T) {
	tests := []struct {
		name   string
		build  func(*QueryBuilder) *SelectQuery
		checks []string
	}{
		{
			name: "ORDER BY + LIMIT",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select().
					From("users").
					OrderBy("age DESC").
					Limit(100)
			},
			checks: []string{
				`ORDER BY "age" DESC`,
				` LIMIT 100`,
			},
		},
		{
			name: "ORDER BY + OFFSET",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select().
					From("users").
					OrderBy("name ASC").
					Offset(50)
			},
			checks: []string{
				`ORDER BY "name" ASC`,
				` OFFSET 50`,
			},
		},
		{
			name: "ORDER BY + LIMIT + OFFSET (all three)",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select().
					From("users").
					OrderBy("status", "created_at DESC").
					Limit(25).
					Offset(75)
			},
			checks: []string{
				`ORDER BY "status"`,
				`"created_at" DESC`,
				` LIMIT 25`,
				` OFFSET 75`,
			},
		},
		{
			name: "Full query: JOIN + WHERE + ORDER BY + LIMIT + OFFSET",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select().
					From("messages m").
					InnerJoin("users u", "m.user_id = u.id").
					Where("m.status = ?", 1).
					OrderBy("m.created_at DESC", "m.id").
					Limit(100).
					Offset(200)
			},
			checks: []string{
				`SELECT * FROM "messages"`,
				`INNER JOIN "users" AS "u"`,
				` WHERE `,
				`ORDER BY "m"."created_at" DESC`,
				`"m"."id"`,
				` LIMIT 100`,
				` OFFSET 200`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB("postgres")
			qb := &QueryBuilder{db: db}

			query := tt.build(qb)
			q := query.Build()
			require.NotNil(t, q)

			for _, check := range tt.checks {
				assert.Contains(t, q.sql, check)
			}

			// Verify SQL clause order: WHERE < ORDER BY < LIMIT < OFFSET
			if indexOf(q.sql, "WHERE") != -1 && indexOf(q.sql, "ORDER BY") != -1 {
				assert.Less(t, indexOf(q.sql, "WHERE"), indexOf(q.sql, "ORDER BY"))
			}
			if indexOf(q.sql, "ORDER BY") != -1 && indexOf(q.sql, "LIMIT") != -1 {
				assert.Less(t, indexOf(q.sql, "ORDER BY"), indexOf(q.sql, "LIMIT"))
			}
			if indexOf(q.sql, "LIMIT") != -1 && indexOf(q.sql, "OFFSET") != -1 {
				assert.Less(t, indexOf(q.sql, "LIMIT"), indexOf(q.sql, "OFFSET"))
			}
		})
	}
}

// TestSelectQuery_OrderBy_PostgreSQL_Quoting tests PostgreSQL-specific quoting
func TestSelectQuery_OrderBy_PostgreSQL_Quoting(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("age DESC", "name ASC")

	q := query.Build()
	require.NotNil(t, q)

	// PostgreSQL uses double quotes
	assert.Contains(t, q.sql, `"users"`)
	assert.Contains(t, q.sql, `"age"`)
	assert.Contains(t, q.sql, `"name"`)
}

// TestSelectQuery_OrderBy_MySQL_Quoting tests MySQL-specific quoting
func TestSelectQuery_OrderBy_MySQL_Quoting(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("age DESC", "name ASC")

	q := query.Build()
	require.NotNil(t, q)

	// MySQL uses backticks
	assert.Contains(t, q.sql, "`users`")
	assert.Contains(t, q.sql, "`age`")
	assert.Contains(t, q.sql, "`name`")
}

// TestSelectQuery_OrderBy_SQLite_Quoting tests SQLite-specific quoting
func TestSelectQuery_OrderBy_SQLite_Quoting(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("age DESC")

	q := query.Build()
	require.NotNil(t, q)

	// SQLite uses double quotes (like PostgreSQL)
	assert.Contains(t, q.sql, `"users"`)
	assert.Contains(t, q.sql, `"age"`)
}

// TestSelectQuery_Limit_Zero tests edge case: LIMIT 0
func TestSelectQuery_Limit_Zero(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Limit(0)

	q := query.Build()
	require.NotNil(t, q)

	// LIMIT 0 is valid (returns no rows)
	assert.Contains(t, q.sql, ` LIMIT 0`)
}

// TestSelectQuery_Offset_Zero tests edge case: OFFSET 0
func TestSelectQuery_Offset_Zero(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Offset(0)

	q := query.Build()
	require.NotNil(t, q)

	// OFFSET 0 is valid (skip no rows)
	assert.Contains(t, q.sql, ` OFFSET 0`)
}

// TestSelectQuery_OrderBy_EmptyString tests edge case: empty string in OrderBy
func TestSelectQuery_OrderBy_EmptyString(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("") // Empty string

	q := query.Build()
	require.NotNil(t, q)

	// Empty string should be ignored (no ORDER BY clause)
	assert.NotContains(t, q.sql, "ORDER BY")
}
