package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, `SELECT * FROM "messages"`)
	assert.Contains(t, q.sql, `INNER JOIN "users" AS "u" ON m.user_id = u.id`)
	assert.Empty(t, q.params, "String JOIN should have no params")
}

// TestSelectQuery_LeftJoin_Expression tests LEFT JOIN with Expression-based ON condition
func TestSelectQuery_LeftJoin_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		LeftJoin("attachments a", Eq("m.id", NewExp("a.message_id")))

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, `SELECT * FROM "messages" AS "m"`)
	assert.Contains(t, q.sql, `LEFT JOIN "attachments" AS "a"`)
	assert.Contains(t, q.sql, `ON`)
	// Expression Eq() generates: "m.id"=(expression)
	assert.Contains(t, q.sql, `"m.id"`)
}

// TestSelectQuery_RightJoin_WithAlias tests RIGHT JOIN with table alias parsing
func TestSelectQuery_RightJoin_WithAlias(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		RightJoin("users u", "m.user_id = u.id")

	q := query.Build()
	require.NotNil(t, q)

	// MySQL uses backticks for quoting
	assert.Contains(t, q.sql, "SELECT * FROM `messages`")
	assert.Contains(t, q.sql, "RIGHT JOIN `users` AS `u` ON m.user_id = u.id")
}

// TestSelectQuery_FullJoin_PostgreSQL tests FULL OUTER JOIN (PostgreSQL-specific)
func TestSelectQuery_FullJoin_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		FullJoin("users u", "m.user_id = u.id")

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT * FROM "messages"`)
	assert.Contains(t, q.sql, `FULL OUTER JOIN "users" AS "u" ON m.user_id = u.id`)
}

// TestSelectQuery_CrossJoin_NoCondition tests CROSS JOIN without ON condition
func TestSelectQuery_CrossJoin_NoCondition(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages").
		CrossJoin("attachments")

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT * FROM "messages"`)
	assert.Contains(t, q.sql, `CROSS JOIN "attachments"`)
	assert.NotContains(t, q.sql, "ON", "CROSS JOIN should not have ON clause")
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
	require.NotNil(t, q)

	// Verify all JOINs are present
	assert.Contains(t, q.sql, `INNER JOIN "users" AS "u"`)
	assert.Contains(t, q.sql, `LEFT JOIN "attachments" AS "a"`)
	assert.Contains(t, q.sql, `LEFT JOIN "tags" AS "t"`)

	// Verify order is preserved
	innerIdx := indexOf(q.sql, "INNER JOIN")
	leftIdx1 := indexOf(q.sql, "LEFT JOIN")
	leftIdx2 := lastIndexOf(q.sql, "LEFT JOIN")
	assert.Less(t, innerIdx, leftIdx1, "INNER JOIN should come before LEFT JOIN")
	assert.Less(t, leftIdx1, leftIdx2, "First LEFT JOIN should come before second")
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
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `INNER JOIN "users" AS "u" ON`)
	// HashExp generates: "deleted" IS NULL AND "status"=?
	assert.Contains(t, q.sql, `"deleted"`)
	assert.Contains(t, q.sql, `"status"`)
	assert.Contains(t, q.sql, `AND`)
	assert.Contains(t, q.sql, `IS NULL`)
	// Should have one param for status = 1
	assert.Len(t, q.params, 1)
	assert.Equal(t, 1, q.params[0])
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
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `LEFT JOIN "users" AS "u" ON`)
	// And() wraps conditions in parentheses
	assert.Contains(t, q.sql, "(")
	assert.Contains(t, q.sql, ")")
	assert.Contains(t, q.sql, "AND")
	// Should have one param for status > 0
	assert.Len(t, q.params, 1)
	assert.Equal(t, 0, q.params[0])
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
	require.NotNil(t, q)

	// JOIN should come before WHERE
	joinIdx := indexOf(q.sql, "INNER JOIN")
	whereIdx := indexOf(q.sql, "WHERE")
	assert.Less(t, joinIdx, whereIdx, "JOIN should come before WHERE")

	// WHERE param should be renumbered correctly
	assert.Contains(t, q.sql, "WHERE m.status = $1")
	assert.Len(t, q.params, 1)
	assert.Equal(t, 1, q.params[0])
}

// TestSelectQuery_Join_TableWithoutAlias tests JOIN with table name without alias
func TestSelectQuery_Join_TableWithoutAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages").
		InnerJoin("users", "messages.user_id = users.id")

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `SELECT * FROM "messages"`)
	// Table without alias should not have AS
	assert.Contains(t, q.sql, `INNER JOIN "users" ON`)
	assert.NotContains(t, q.sql, "AS", "Table without alias should not have AS keyword")
}

// TestSelectQuery_Join_InvalidOnType tests that invalid ON type panics
func TestSelectQuery_Join_InvalidOnType(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().
		From("messages m").
		InnerJoin("users u", 123) // Invalid: int instead of string or Expression

	assert.Panics(t, func() {
		query.Build()
	}, "Invalid ON type should panic")
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
