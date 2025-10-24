package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateQuery_PostgreSQL tests UPDATE SQL generation for PostgreSQL.
func TestUpdateQuery_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
		}).
		Where("id = ?", 1)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure - columns should be in alphabetical order
	expectedSQL := `UPDATE "users" SET email = $1, name = $2 WHERE id = $3`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters - should be in alphabetical order, then WHERE params
	assert.Equal(t, []interface{}{"alice@example.com", "Alice", 1}, q.params)
}

// TestUpdateQuery_MySQL tests UPDATE SQL generation for MySQL.
func TestUpdateQuery_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"name":  "Bob",
			"email": "bob@example.com",
		}).
		Where("id = ?", 2)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := "UPDATE `users` SET email = ?, name = ? WHERE id = ?"
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{"bob@example.com", "Bob", 2}, q.params)
}

// TestUpdateQuery_SQLite tests UPDATE SQL generation for SQLite.
func TestUpdateQuery_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"name":   "Charlie",
			"status": "active",
		}).
		Where("id = ?", 3)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := `UPDATE "users" SET name = ?, status = ? WHERE id = ?`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{"Charlie", "active", 3}, q.params)
}

// TestUpdateQuery_MultipleWhere tests UPDATE with multiple WHERE conditions.
func TestUpdateQuery_MultipleWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"status": "inactive",
		}).
		Where("created_at < ?", "2024-01-01").
		Where("last_login IS NULL")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, `UPDATE "users" SET status = $1`)
	assert.Contains(t, q.sql, "WHERE created_at < $2 AND last_login IS NULL")

	// Verify parameters
	assert.Equal(t, []interface{}{"inactive", "2024-01-01"}, q.params)
}

// TestUpdateQuery_NoWhere tests UPDATE without WHERE clause (updates all rows).
func TestUpdateQuery_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"last_check": "2025-01-01",
		})

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure - no WHERE clause
	expectedSQL := `UPDATE "users" SET last_check = $1`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{"2025-01-01"}, q.params)
}

// TestUpdateQuery_ParameterOrdering tests that parameters are in deterministic order.
func TestUpdateQuery_ParameterOrdering(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"zzz": "last",
			"aaa": "first",
			"mmm": "middle",
		}).
		Where("id = ?", 1)

	q := query.Build()
	require.NotNil(t, q)

	// Parameters should be in sorted key order, then WHERE params
	assert.Equal(t, []interface{}{"first", "middle", "last", 1}, q.params)

	// SQL should have columns in alphabetical order
	sql := q.sql
	aIdx := strings.Index(sql, "aaa")
	mIdx := strings.Index(sql, "mmm")
	zIdx := strings.Index(sql, "zzz")

	assert.Less(t, aIdx, mIdx, "aaa should come before mmm")
	assert.Less(t, mIdx, zIdx, "mmm should come before zzz")
}

// TestUpdateQuery_ComplexWhere tests UPDATE with complex WHERE conditions.
func TestUpdateQuery_ComplexWhere(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Update("orders").
		Set(map[string]interface{}{
			"status":     "canceled",
			"updated_at": "2025-01-15",
		}).
		Where("status = ?", "pending").
		Where("created_at < ?", "2025-01-01").
		Where("amount < ?", 100)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, "UPDATE `orders` SET status = ?, updated_at = ?")
	assert.Contains(t, q.sql, "WHERE status = ? AND created_at < ? AND amount < ?")

	// Verify parameters (sorted columns, then WHERE params in order)
	assert.Equal(t, []interface{}{"canceled", "2025-01-15", "pending", "2025-01-01", 100}, q.params)
}

// TestDeleteQuery_PostgreSQL tests DELETE SQL generation for PostgreSQL.
func TestDeleteQuery_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where("id = ?", 1)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := `DELETE FROM "users" WHERE id = $1`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{1}, q.params)
}

// TestDeleteQuery_MySQL tests DELETE SQL generation for MySQL.
func TestDeleteQuery_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where("id = ?", 2)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := "DELETE FROM `users` WHERE id = ?"
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{2}, q.params)
}

// TestDeleteQuery_SQLite tests DELETE SQL generation for SQLite.
func TestDeleteQuery_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where("id = ?", 3)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := `DELETE FROM "users" WHERE id = ?`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{3}, q.params)
}

// TestDeleteQuery_MultipleWhere tests DELETE with multiple WHERE conditions.
func TestDeleteQuery_MultipleWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").
		Where("status = ?", "deleted").
		Where("created_at < ?", "2024-01-01")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := `DELETE FROM "users" WHERE status = $1 AND created_at < $2`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{"deleted", "2024-01-01"}, q.params)
}

// TestDeleteQuery_NoWhere tests DELETE without WHERE clause (deletes all rows).
func TestDeleteQuery_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure - no WHERE clause
	expectedSQL := `DELETE FROM "users"`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify no parameters
	assert.Len(t, q.params, 0)
}

// TestDeleteQuery_ComplexWhere tests DELETE with complex WHERE conditions.
func TestDeleteQuery_ComplexWhere(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("logs").
		Where("level = ?", "debug").
		Where("created_at < ?", "2025-01-01").
		Where("user_id IS NULL")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	assert.Contains(t, q.sql, "DELETE FROM `logs`")
	assert.Contains(t, q.sql, "WHERE level = ? AND created_at < ? AND user_id IS NULL")

	// Verify parameters
	assert.Equal(t, []interface{}{"debug", "2025-01-01"}, q.params)
}

// TestUpdateQuery_SingleColumn tests UPDATE with single column.
func TestUpdateQuery_SingleColumn(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"last_login": "2025-01-15 10:30:00",
		}).
		Where("id = ?", 42)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := `UPDATE "users" SET last_login = $1 WHERE id = $2`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{"2025-01-15 10:30:00", 42}, q.params)
}

// TestDeleteQuery_SingleCondition tests DELETE with single WHERE condition.
func TestDeleteQuery_SingleCondition(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("sessions").Where("expired = ?", true)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	expectedSQL := `DELETE FROM "sessions" WHERE expired = ?`
	assert.Equal(t, expectedSQL, q.sql)

	// Verify parameters
	assert.Equal(t, []interface{}{true}, q.params)
}

// TestUpdateQuery_MultiColumnMultiWhere tests UPDATE with many columns and conditions.
func TestUpdateQuery_MultiColumnMultiWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("products").
		Set(map[string]interface{}{
			"price":      99.99,
			"stock":      50,
			"updated_at": "2025-01-15",
			"discount":   10,
		}).
		Where("category = ?", "electronics").
		Where("in_stock = ?", true).
		Where("price < ?", 150.00)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL contains all columns (alphabetically sorted)
	assert.Contains(t, q.sql, "discount = $1")
	assert.Contains(t, q.sql, "price = $2")
	assert.Contains(t, q.sql, "stock = $3")
	assert.Contains(t, q.sql, "updated_at = $4")

	// Verify WHERE clause
	assert.Contains(t, q.sql, "WHERE category = $5 AND in_stock = $6 AND price < $7")

	// Verify parameters (sorted SET columns, then WHERE params in order)
	assert.Equal(t, []interface{}{10, 99.99, 50, "2025-01-15", "electronics", true, 150.00}, q.params)
}
