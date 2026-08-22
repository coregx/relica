package core

import (
	"strings"
	"testing"
)

// mockDB is defined in upsert_test.go to avoid duplication

// TestSelectQuery_InnerJoin_String tests INNER JOIN with string-based ON condition
func TestSelectQuery_InnerJoin_String(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		InnerJoin("users u", "m.user_id = u.id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, `SELECT * FROM "messages"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT * FROM "messages"`)
	}
	if !strings.Contains(q.sql, `INNER JOIN "users" AS "u" ON m.user_id = u.id`) {
		t.Errorf("%q does not contain %q", q.sql, `INNER JOIN "users" AS "u" ON m.user_id = u.id`)
	}
	if len(q.params) != 0 {
		t.Errorf("String JOIN should have no params: expected empty, got %d", len(q.params))
	}
}

// TestSelectQuery_LeftJoin_Expression tests LEFT JOIN with Expression-based ON condition
func TestSelectQuery_LeftJoin_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		LeftJoin("attachments a", Eq("m.id", NewExp("a.message_id")))

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, `SELECT * FROM "messages" AS "m"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT * FROM "messages" AS "m"`)
	}
	if !strings.Contains(q.sql, `LEFT JOIN "attachments" AS "a"`) {
		t.Errorf("%q does not contain %q", q.sql, `LEFT JOIN "attachments" AS "a"`)
	}
	if !strings.Contains(q.sql, `ON`) {
		t.Errorf("%q does not contain %q", q.sql, `ON`)
	}
	// Expression Eq() now correctly splits table alias: "m"."id"=(expression)
	if !strings.Contains(q.sql, `"m"."id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"m"."id"`)
	}
}

// TestSelectQuery_RightJoin_WithAlias tests RIGHT JOIN with table alias parsing
func TestSelectQuery_RightJoin_WithAlias(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		RightJoin("users u", "m.user_id = u.id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// MySQL uses backticks for quoting
	if !strings.Contains(q.sql, "SELECT * FROM `messages`") {
		t.Errorf("%q does not contain %q", q.sql, "SELECT * FROM `messages`")
	}
	if !strings.Contains(q.sql, "RIGHT JOIN `users` AS `u` ON m.user_id = u.id") {
		t.Errorf("%q does not contain %q", q.sql, "RIGHT JOIN `users` AS `u` ON m.user_id = u.id")
	}
}

// TestSelectQuery_FullJoin_PostgreSQL tests FULL OUTER JOIN (PostgreSQL-specific)
func TestSelectQuery_FullJoin_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		FullJoin("users u", "m.user_id = u.id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT * FROM "messages"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT * FROM "messages"`)
	}
	if !strings.Contains(q.sql, `FULL OUTER JOIN "users" AS "u" ON m.user_id = u.id`) {
		t.Errorf("%q does not contain %q", q.sql, `FULL OUTER JOIN "users" AS "u" ON m.user_id = u.id`)
	}
}

// TestSelectQuery_CrossJoin_NoCondition tests CROSS JOIN without ON condition
func TestSelectQuery_CrossJoin_NoCondition(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages").
		CrossJoin("attachments")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT * FROM "messages"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT * FROM "messages"`)
	}
	if !strings.Contains(q.sql, `CROSS JOIN "attachments"`) {
		t.Errorf("%q does not contain %q", q.sql, `CROSS JOIN "attachments"`)
	}
	if strings.Contains(q.sql, "ON") {
		t.Errorf("CROSS JOIN should not have ON clause: %q should not contain %q", q.sql, "ON")
	}
}

// TestSelectQuery_MultipleJoins tests multiple JOINs in one query
func TestSelectQuery_MultipleJoins(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		InnerJoin("users u", "m.user_id = u.id").
		LeftJoin("attachments a", "m.id = a.message_id").
		LeftJoin("tags t", "m.id = t.message_id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify all JOINs are present
	if !strings.Contains(q.sql, `INNER JOIN "users" AS "u"`) {
		t.Errorf("%q does not contain %q", q.sql, `INNER JOIN "users" AS "u"`)
	}
	if !strings.Contains(q.sql, `LEFT JOIN "attachments" AS "a"`) {
		t.Errorf("%q does not contain %q", q.sql, `LEFT JOIN "attachments" AS "a"`)
	}
	if !strings.Contains(q.sql, `LEFT JOIN "tags" AS "t"`) {
		t.Errorf("%q does not contain %q", q.sql, `LEFT JOIN "tags" AS "t"`)
	}

	// Verify order is preserved
	innerIdx := indexOf(q.sql, "INNER JOIN")
	leftIdx1 := indexOf(q.sql, "LEFT JOIN")
	leftIdx2 := lastIndexOf(q.sql, "LEFT JOIN")
	if innerIdx >= leftIdx1 {
		t.Errorf("INNER JOIN should come before LEFT JOIN: expected %v < %v", innerIdx, leftIdx1)
	}
	if leftIdx1 >= leftIdx2 {
		t.Errorf("First LEFT JOIN should come before second: expected %v < %v", leftIdx1, leftIdx2)
	}
}

// TestSelectQuery_Join_WithHashExp tests JOIN with HashExp ON condition
func TestSelectQuery_Join_WithHashExp(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// HashExp works best with simple column names, not dotted ones
	// For JOIN ON with table prefixes, use string or And() with Eq()
	query := qb.Select().
		From("messages m").
		InnerJoin("users u", HashExp{
			"status":  1,
			"deleted": nil,
		})

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `INNER JOIN "users" AS "u" ON`) {
		t.Errorf("%q does not contain %q", q.sql, `INNER JOIN "users" AS "u" ON`)
	}
	// HashExp generates: "deleted" IS NULL AND "status"=?
	if !strings.Contains(q.sql, `"deleted"`) {
		t.Errorf("%q does not contain %q", q.sql, `"deleted"`)
	}
	if !strings.Contains(q.sql, `"status"`) {
		t.Errorf("%q does not contain %q", q.sql, `"status"`)
	}
	if !strings.Contains(q.sql, `AND`) {
		t.Errorf("%q does not contain %q", q.sql, `AND`)
	}
	if !strings.Contains(q.sql, `IS NULL`) {
		t.Errorf("%q does not contain %q", q.sql, `IS NULL`)
	}
	// Should have one param for status = 1
	if len(q.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(q.params))
	}
	if q.params[0] != 1 {
		t.Errorf("got %v, want %v", q.params[0], 1)
	}
}

// TestSelectQuery_Join_ComplexExpression tests JOIN with complex And/Or expressions
func TestSelectQuery_Join_ComplexExpression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		LeftJoin("users u", And(
			Eq("m.user_id", NewExp("u.id")),
			GreaterThan("u.status", 0),
		))

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `LEFT JOIN "users" AS "u" ON`) {
		t.Errorf("%q does not contain %q", q.sql, `LEFT JOIN "users" AS "u" ON`)
	}
	// And() wraps conditions in parentheses
	if !strings.Contains(q.sql, "(") {
		t.Errorf("%q does not contain %q", q.sql, "(")
	}
	if !strings.Contains(q.sql, ")") {
		t.Errorf("%q does not contain %q", q.sql, ")")
	}
	if !strings.Contains(q.sql, "AND") {
		t.Errorf("%q does not contain %q", q.sql, "AND")
	}
	// Should have one param for status > 0
	if len(q.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(q.params))
	}
	if q.params[0] != 0 {
		t.Errorf("got %v, want %v", q.params[0], 0)
	}
}

// TestSelectQuery_Join_WithWhere tests JOIN combined with WHERE clause
func TestSelectQuery_Join_WithWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		InnerJoin("users u", "m.user_id = u.id").
		Where("m.status = ?", 1)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// JOIN should come before WHERE
	joinIdx := indexOf(q.sql, "INNER JOIN")
	whereIdx := indexOf(q.sql, "WHERE")
	if joinIdx >= whereIdx {
		t.Errorf("JOIN should come before WHERE: expected %v < %v", joinIdx, whereIdx)
	}

	// WHERE param should be renumbered correctly
	if !strings.Contains(q.sql, "WHERE m.status = $1") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE m.status = $1")
	}
	if len(q.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(q.params))
	}
	if q.params[0] != 1 {
		t.Errorf("got %v, want %v", q.params[0], 1)
	}
}

// TestSelectQuery_Join_TableWithoutAlias tests JOIN with table name without alias
func TestSelectQuery_Join_TableWithoutAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages").
		InnerJoin("users", "messages.user_id = users.id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !strings.Contains(q.sql, `SELECT * FROM "messages"`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT * FROM "messages"`)
	}
	// Table without alias should not have AS
	if !strings.Contains(q.sql, `INNER JOIN "users" ON`) {
		t.Errorf("%q does not contain %q", q.sql, `INNER JOIN "users" ON`)
	}
	if strings.Contains(q.sql, "AS") {
		t.Errorf("Table without alias should not have AS keyword: %q should not contain %q", q.sql, "AS")
	}
}

// TestSelectQuery_Join_InvalidOnType tests that invalid ON type stores an error
// instead of panicking, and the error propagates through Build.
func TestSelectQuery_Join_InvalidOnType(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		InnerJoin("users u", 123) // Invalid: int instead of string or Expression

	q := query.Build()
	if q.prepErr == nil {
		t.Error("Invalid ON type must store a build error")
	}
	if q.prepErr != nil && !strings.Contains(q.prepErr.Error(), "JOIN ON") {
		t.Errorf("%q does not contain %q", q.prepErr.Error(), "JOIN ON")
	}
}

// Helper functions for tests
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func lastIndexOf(s, substr string) int {
	lastIdx := -1
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			lastIdx = i
		}
	}
	return lastIdx
}
