package core

import (
	"strings"
	"testing"
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, `SELECT * FROM "users"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT * FROM "users"`)
	}
	if !strings.Contains(q.sql, ` ORDER BY "age" DESC`) {
		t.Errorf("%q does not contain %q", q.sql, ` ORDER BY "age" DESC`)
	}
	if len(q.params) != 0 {
		t.Errorf("ORDER BY should have no params: expected empty, got %d", len(q.params))
	}
}

// TestSelectQuery_OrderBy_Multiple tests ORDER BY with multiple columns in one call
func TestSelectQuery_OrderBy_Multiple(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("status ASC", "created_at DESC", "id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify all columns are present in ORDER BY
	if !strings.Contains(q.sql, `ORDER BY`) {
		t.Errorf("%q does not contain %q", q.sql, `ORDER BY`)
	}
	if !strings.Contains(q.sql, `"status" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `"status" ASC`)
	}
	if !strings.Contains(q.sql, `"created_at" DESC`) {
		t.Errorf("%q does not contain %q", q.sql, `"created_at" DESC`)
	}
	if !strings.Contains(q.sql, `"id"`) { // Default ASC (not explicitly shown)
		t.Errorf("%q does not contain %q", q.sql, `"id"`)
	}

	// Verify order is preserved
	statusIdx := indexOf(q.sql, `"status"`)
	createdIdx := indexOf(q.sql, `"created_at"`)
	idIdx := lastIndexOf(q.sql, `"id"`)
	if statusIdx >= createdIdx {
		t.Errorf("status should come before created_at: expected %v < %v", statusIdx, createdIdx)
	}
	if createdIdx >= idIdx {
		t.Errorf("created_at should come before id: expected %v < %v", createdIdx, idIdx)
	}
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
			if q == nil {
				t.Fatal("expected non-nil")
			}

			if !strings.Contains(q.sql, tt.expectedSQL) {
				t.Errorf("%q does not contain %q", q.sql, tt.expectedSQL)
			}
			if tt.expectedNoSQL != "" && strings.Contains(q.sql, tt.expectedNoSQL) {
				t.Errorf("%q should not contain %q", q.sql, tt.expectedNoSQL)
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// All columns should be present
	if !strings.Contains(q.sql, `"status" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `"status" ASC`)
	}
	if !strings.Contains(q.sql, `"age" DESC`) {
		t.Errorf("%q does not contain %q", q.sql, `"age" DESC`)
	}
	if !strings.Contains(q.sql, `"name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name"`)
	}

	// Verify order is preserved
	statusIdx := indexOf(q.sql, `"status"`)
	ageIdx := indexOf(q.sql, `"age"`)
	nameIdx := lastIndexOf(q.sql, `"name"`)
	if statusIdx >= ageIdx {
		t.Errorf("expected %v < %v", statusIdx, ageIdx)
	}
	if ageIdx >= nameIdx {
		t.Errorf("expected %v < %v", ageIdx, nameIdx)
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Both table and column should be quoted
	if !strings.Contains(q.sql, `"m"."created_at" DESC`) {
		t.Errorf("%q does not contain %q", q.sql, `"m"."created_at" DESC`)
	}
	if !strings.Contains(q.sql, `"u"."name" ASC`) {
		t.Errorf("%q does not contain %q", q.sql, `"u"."name" ASC`)
	}
}

// TestSelectQuery_Limit tests LIMIT clause
func TestSelectQuery_Limit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Limit(100)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, ` LIMIT 100`) {
		t.Errorf("%q does not contain %q", q.sql, ` LIMIT 100`)
	}
	if strings.Contains(q.sql, "OFFSET") {
		t.Errorf("%q should not contain %q", q.sql, "OFFSET")
	}
}

// TestSelectQuery_Offset tests OFFSET clause
// When OFFSET is set without LIMIT, a maximum LIMIT is emitted for MySQL compatibility.
func TestSelectQuery_Offset(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Offset(200)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, ` OFFSET 200`) {
		t.Errorf("%q does not contain %q", q.sql, ` OFFSET 200`)
	}
	// MySQL requires LIMIT before OFFSET; a max-value sentinel is emitted when
	// no explicit LIMIT is set, ensuring portability across all supported dialects.
	if !strings.Contains(q.sql, "LIMIT 9223372036854775807") {
		t.Errorf("%q does not contain %q", q.sql, "LIMIT 9223372036854775807")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// LIMIT should come before OFFSET
	if !strings.Contains(q.sql, ` LIMIT 50`) {
		t.Errorf("%q does not contain %q", q.sql, ` LIMIT 50`)
	}
	if !strings.Contains(q.sql, ` OFFSET 100`) {
		t.Errorf("%q does not contain %q", q.sql, ` OFFSET 100`)
	}

	limitIdx := indexOf(q.sql, "LIMIT")
	offsetIdx := indexOf(q.sql, "OFFSET")
	if limitIdx >= offsetIdx {
		t.Errorf("LIMIT should come before OFFSET: expected %v < %v", limitIdx, offsetIdx)
	}
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
			if q == nil {
				t.Fatal("expected non-nil")
			}

			for _, check := range tt.checks {
				if !strings.Contains(q.sql, check) {
					t.Errorf("%q does not contain %q", q.sql, check)
				}
			}

			// Verify SQL clause order: WHERE < ORDER BY < LIMIT < OFFSET
			if indexOf(q.sql, "WHERE") != -1 && indexOf(q.sql, "ORDER BY") != -1 {
				if indexOf(q.sql, "WHERE") >= indexOf(q.sql, "ORDER BY") {
					t.Errorf("expected WHERE before ORDER BY")
				}
			}
			if indexOf(q.sql, "ORDER BY") != -1 && indexOf(q.sql, "LIMIT") != -1 {
				if indexOf(q.sql, "ORDER BY") >= indexOf(q.sql, "LIMIT") {
					t.Errorf("expected ORDER BY before LIMIT")
				}
			}
			if indexOf(q.sql, "LIMIT") != -1 && indexOf(q.sql, "OFFSET") != -1 {
				if indexOf(q.sql, "LIMIT") >= indexOf(q.sql, "OFFSET") {
					t.Errorf("expected LIMIT before OFFSET")
				}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// PostgreSQL uses double quotes
	if !strings.Contains(q.sql, `"users"`) {
		t.Errorf("%q does not contain %q", q.sql, `"users"`)
	}
	if !strings.Contains(q.sql, `"age"`) {
		t.Errorf("%q does not contain %q", q.sql, `"age"`)
	}
	if !strings.Contains(q.sql, `"name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name"`)
	}
}

// TestSelectQuery_OrderBy_MySQL_Quoting tests MySQL-specific quoting
func TestSelectQuery_OrderBy_MySQL_Quoting(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("age DESC", "name ASC")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// MySQL uses backticks
	if !strings.Contains(q.sql, "`users`") {
		t.Errorf("%q does not contain %q", q.sql, "`users`")
	}
	if !strings.Contains(q.sql, "`age`") {
		t.Errorf("%q does not contain %q", q.sql, "`age`")
	}
	if !strings.Contains(q.sql, "`name`") {
		t.Errorf("%q does not contain %q", q.sql, "`name`")
	}
}

// TestSelectQuery_OrderBy_SQLite_Quoting tests SQLite-specific quoting
func TestSelectQuery_OrderBy_SQLite_Quoting(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("age DESC")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// SQLite uses double quotes (like PostgreSQL)
	if !strings.Contains(q.sql, `"users"`) {
		t.Errorf("%q does not contain %q", q.sql, `"users"`)
	}
	if !strings.Contains(q.sql, `"age"`) {
		t.Errorf("%q does not contain %q", q.sql, `"age"`)
	}
}

// TestSelectQuery_Limit_Zero tests edge case: LIMIT 0
func TestSelectQuery_Limit_Zero(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Limit(0)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// LIMIT 0 is valid (returns no rows)
	if !strings.Contains(q.sql, ` LIMIT 0`) {
		t.Errorf("%q does not contain %q", q.sql, ` LIMIT 0`)
	}
}

// TestSelectQuery_Offset_Zero tests edge case: OFFSET 0
func TestSelectQuery_Offset_Zero(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		Offset(0)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// OFFSET 0 is valid (skip no rows)
	if !strings.Contains(q.sql, ` OFFSET 0`) {
		t.Errorf("%q does not contain %q", q.sql, ` OFFSET 0`)
	}
}

// TestSelectQuery_OrderBy_EmptyString tests edge case: empty string in OrderBy
func TestSelectQuery_OrderBy_EmptyString(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("users").
		OrderBy("") // Empty string

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Empty string should be ignored (no ORDER BY clause)
	if strings.Contains(q.sql, "ORDER BY") {
		t.Errorf("%q should not contain %q", q.sql, "ORDER BY")
	}
}
