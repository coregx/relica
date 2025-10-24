package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBatchInsert_PostgreSQL tests batch INSERT SQL generation for PostgreSQL.
func TestBatchInsert_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com").
		Values("Charlie", "charlie@example.com")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	sql := q.sql
	assert.Contains(t, sql, `INSERT INTO "users"`)
	assert.Contains(t, sql, `("name", "email")`)
	assert.Contains(t, sql, "VALUES")
	assert.Contains(t, sql, "($1, $2)")
	assert.Contains(t, sql, "($3, $4)")
	assert.Contains(t, sql, "($5, $6)")

	// Verify parameters
	assert.Len(t, q.params, 6)
	assert.Equal(t, "Alice", q.params[0])
	assert.Equal(t, "alice@example.com", q.params[1])
	assert.Equal(t, "Bob", q.params[2])
	assert.Equal(t, "bob@example.com", q.params[3])
	assert.Equal(t, "Charlie", q.params[4])
	assert.Equal(t, "charlie@example.com", q.params[5])
}

// TestBatchInsert_MySQL tests batch INSERT SQL generation for MySQL.
func TestBatchInsert_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com")

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	sql := q.sql
	assert.Contains(t, sql, "INSERT INTO `users`")
	assert.Contains(t, sql, "(`name`, `email`)")
	assert.Contains(t, sql, "VALUES (?, ?), (?, ?)")

	// Verify parameters
	assert.Len(t, q.params, 4)
	assert.Equal(t, "Alice", q.params[0])
	assert.Equal(t, "alice@example.com", q.params[1])
}

// TestBatchInsert_SQLite tests batch INSERT SQL generation for SQLite.
func TestBatchInsert_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("products", []string{"name", "price", "stock"}).
		Values("Widget", 9.99, 100).
		Values("Gadget", 19.99, 50)

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	sql := q.sql
	assert.Contains(t, sql, `INSERT INTO "products"`)
	assert.Contains(t, sql, `("name", "price", "stock")`)
	assert.Contains(t, sql, "VALUES (?, ?, ?), (?, ?, ?)")

	// Verify parameters
	assert.Len(t, q.params, 6)
	assert.Equal(t, "Widget", q.params[0])
	assert.Equal(t, 9.99, q.params[1])
	assert.Equal(t, 100, q.params[2])
}

// TestBatchInsert_SingleRow tests batch INSERT with a single row.
func TestBatchInsert_SingleRow(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name"}).
		Values("Alice")

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, "VALUES ($1)")
	assert.Len(t, q.params, 1)
}

// TestBatchInsert_MultipleRows tests batch INSERT with many rows.
func TestBatchInsert_MultipleRows(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("logs", []string{"message", "level"})
	for i := 1; i <= 100; i++ {
		query.Values("Log message", "INFO")
	}

	q := query.Build()
	require.NotNil(t, q)

	// Should have 100 rows
	assert.Len(t, q.params, 200) // 100 rows * 2 columns
}

// TestBatchInsert_ValuesMap tests batch INSERT with map-based values.
func TestBatchInsert_ValuesMap(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"email", "name"}).
		ValuesMap(map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
		}).
		ValuesMap(map[string]interface{}{
			"name":  "Bob",
			"email": "bob@example.com",
		})

	q := query.Build()
	require.NotNil(t, q)

	// Verify columns are in correct order
	assert.Contains(t, q.sql, `("email", "name")`)
	// Verify parameters are in correct order (email, name)
	assert.Equal(t, "alice@example.com", q.params[0])
	assert.Equal(t, "Alice", q.params[1])
	assert.Equal(t, "bob@example.com", q.params[2])
	assert.Equal(t, "Bob", q.params[3])
}

// TestBatchInsert_ValuesMap_MissingColumn tests map with missing columns (should use nil).
func TestBatchInsert_ValuesMap_MissingColumn(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email", "age"}).
		ValuesMap(map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
			// age is missing, should be nil
		})

	q := query.Build()
	require.NotNil(t, q)

	assert.Len(t, q.params, 3)
	assert.Equal(t, "Alice", q.params[0])
	assert.Equal(t, "alice@example.com", q.params[1])
	assert.Nil(t, q.params[2]) // Missing age should be nil
}

// TestBatchInsert_EmptyPanic tests that building without rows panics.
func TestBatchInsert_EmptyPanic(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name"})

	assert.Panics(t, func() {
		query.Build()
	}, "Should panic when no rows added")
}

// TestBatchInsert_WrongValueCount tests that wrong value count panics.
func TestBatchInsert_WrongValueCount(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email"})

	assert.Panics(t, func() {
		query.Values("Alice") // Only 1 value, expected 2
	}, "Should panic with wrong value count")

	assert.Panics(t, func() {
		query.Values("Alice", "alice@example.com", "extra") // 3 values, expected 2
	}, "Should panic with too many values")
}

// TestBatchUpdate_PostgreSQL tests batch UPDATE SQL generation for PostgreSQL.
func TestBatchUpdate_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice Updated", "status": "active"}).
		Set(2, map[string]interface{}{"name": "Bob Updated", "status": "active"}).
		Set(3, map[string]interface{}{"name": "Charlie Updated", "status": "inactive"})

	q := query.Build()
	require.NotNil(t, q)

	// Verify SQL structure
	sql := q.sql
	assert.Contains(t, sql, `UPDATE "users"`)
	assert.Contains(t, sql, `SET`)
	assert.Contains(t, sql, `"name" = CASE "id"`)
	assert.Contains(t, sql, `"status" = CASE "id"`)
	assert.Contains(t, sql, `WHEN $1 THEN $2`)
	assert.Contains(t, sql, `WHERE "id" IN ($13, $14, $15)`)

	// Verify we have the right number of parameters
	// 3 rows * 2 columns * 2 params (key + value) + 3 WHERE IN params = 15
	assert.Len(t, q.params, 15)
}

// TestBatchUpdate_MySQL tests batch UPDATE SQL generation for MySQL.
func TestBatchUpdate_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice", "email": "alice@new.com"}).
		Set(2, map[string]interface{}{"name": "Bob", "email": "bob@new.com"})

	q := query.Build()
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, "UPDATE `users`")
	assert.Contains(t, sql, "`name` = CASE `id`")
	assert.Contains(t, sql, "`email` = CASE `id`")
	assert.Contains(t, sql, "WHERE `id` IN (?, ?)")
}

// TestBatchUpdate_SQLite tests batch UPDATE SQL generation for SQLite.
func TestBatchUpdate_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("products", "id").
		Set(1, map[string]interface{}{"price": 10.99}).
		Set(2, map[string]interface{}{"price": 20.99})

	q := query.Build()
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, `UPDATE "products"`)
	assert.Contains(t, sql, `"price" = CASE "id"`)
}

// TestBatchUpdate_SingleRow tests batch UPDATE with a single row.
func TestBatchUpdate_SingleRow(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice"})

	q := query.Build()
	require.NotNil(t, q)

	assert.Contains(t, q.sql, `UPDATE "users"`)
	assert.Contains(t, q.sql, `WHERE "id" IN ($3)`)
	// 1 row * 1 column * 2 params (key + value) + 1 WHERE IN = 3 params
	assert.Len(t, q.params, 3)
}

// TestBatchUpdate_MultipleRows tests batch UPDATE with many rows.
func TestBatchUpdate_MultipleRows(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("logs", "id")
	for i := 1; i <= 50; i++ {
		query.Set(i, map[string]interface{}{"processed": true})
	}

	q := query.Build()
	require.NotNil(t, q)

	// 50 rows * 1 column * 2 params (key + value) + 50 WHERE IN params = 150
	assert.Len(t, q.params, 150)
}

// TestBatchUpdate_MultipleColumns tests batch UPDATE with multiple columns.
func TestBatchUpdate_MultipleColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{
			"name":   "Alice",
			"email":  "alice@example.com",
			"age":    30,
			"status": "active",
		}).
		Set(2, map[string]interface{}{
			"name":   "Bob",
			"email":  "bob@example.com",
			"age":    25,
			"status": "inactive",
		})

	q := query.Build()
	require.NotNil(t, q)

	sql := q.sql
	// Should have CASE for each column
	assert.Contains(t, sql, `"age" = CASE "id"`)
	assert.Contains(t, sql, `"email" = CASE "id"`)
	assert.Contains(t, sql, `"name" = CASE "id"`)
	assert.Contains(t, sql, `"status" = CASE "id"`)
}

// TestBatchUpdate_DifferentColumns tests batch UPDATE where rows update different columns.
func TestBatchUpdate_DifferentColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice", "email": "alice@example.com"}). // 2 columns
		Set(2, map[string]interface{}{"age": 25}).                                     // 1 column
		Set(3, map[string]interface{}{"name": "Charlie", "age": 35})                   // 2 columns (different from row 1)

	q := query.Build()
	require.NotNil(t, q)

	sql := q.sql
	// Should have CASE for all unique columns (age, email, name)
	assert.Contains(t, sql, `"age" = CASE "id"`)
	assert.Contains(t, sql, `"email" = CASE "id"`)
	assert.Contains(t, sql, `"name" = CASE "id"`)

	// Row 2 only updates age, so only row 2's key should appear in age CASE
	// This is complex to verify in generated SQL, but we can check param count
	// Row 1: name+email (2*2=4 params)
	// Row 2: age (1*2=2 params)
	// Row 3: name+age (2*2=4 params)
	// WHERE IN: 3 params
	// Total: 4+2+4+3 = 13 params
	assert.Len(t, q.params, 13)
}

// TestBatchUpdate_EmptyPanic tests that building without updates panics.
func TestBatchUpdate_EmptyPanic(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id")

	assert.Panics(t, func() {
		query.Build()
	}, "Should panic when no updates added")
}

// TestBatchInsert_ChainedCalls tests method chaining for batch insert.
func TestBatchInsert_ChainedCalls(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// All methods should return *BatchInsertQuery for chaining
	query := qb.BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com").
		ValuesMap(map[string]interface{}{
			"name":  "Charlie",
			"email": "charlie@example.com",
		})

	q := query.Build()
	require.NotNil(t, q)
	assert.Len(t, q.params, 6) // 3 rows * 2 columns
}

// TestBatchUpdate_ChainedCalls tests method chaining for batch update.
func TestBatchUpdate_ChainedCalls(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// All methods should return *BatchUpdateQuery for chaining
	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice"}).
		Set(2, map[string]interface{}{"name": "Bob"}).
		Set(3, map[string]interface{}{"name": "Charlie"})

	q := query.Build()
	require.NotNil(t, q)
	assert.Contains(t, q.sql, "WHERE")
}

// TestBatchInsert_NullValues tests batch INSERT with NULL values.
func TestBatchInsert_NullValues(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email", "age"}).
		Values("Alice", "alice@example.com", nil). // age is NULL
		Values("Bob", nil, 30)                     // email is NULL

	q := query.Build()
	require.NotNil(t, q)

	assert.Len(t, q.params, 6)
	assert.Nil(t, q.params[2]) // Alice's age
	assert.Nil(t, q.params[4]) // Bob's email
}

// TestBatchUpdate_NullValues tests batch UPDATE with NULL values.
func TestBatchUpdate_NullValues(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"email": nil}). // Set email to NULL
		Set(2, map[string]interface{}{"email": "bob@example.com"})

	q := query.Build()
	require.NotNil(t, q)

	// Should contain NULL value in params
	foundNull := false
	for _, param := range q.params {
		if param == nil {
			foundNull = true
			break
		}
	}
	assert.True(t, foundNull, "Should have NULL parameter")
}

// TestBatchInsert_QuoteIdentifiers tests proper identifier quoting.
func TestBatchInsert_QuoteIdentifiers(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Use column names that need quoting
	query := qb.BatchInsert("user_table", []string{"user_name", "user_email"}).
		Values("Alice", "alice@example.com")

	q := query.Build()
	require.NotNil(t, q)

	// PostgreSQL should quote with double quotes
	assert.Contains(t, q.sql, `"user_table"`)
	assert.Contains(t, q.sql, `"user_name"`)
	assert.Contains(t, q.sql, `"user_email"`)
}

// TestBatchUpdate_QuoteIdentifiers tests proper identifier quoting in UPDATE.
func TestBatchUpdate_QuoteIdentifiers(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("user_table", "user_id").
		Set(1, map[string]interface{}{"user_name": "Alice"})

	q := query.Build()
	require.NotNil(t, q)

	// MySQL should quote with backticks
	assert.Contains(t, q.sql, "`user_table`")
	assert.Contains(t, q.sql, "`user_id`")
	assert.Contains(t, q.sql, "`user_name`")
}

// TestBatchInsert_ColumnOrder tests that column order is preserved.
func TestBatchInsert_ColumnOrder(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Specify columns in specific order
	query := qb.BatchInsert("users", []string{"email", "name", "age"}).
		Values("alice@example.com", "Alice", 30)

	q := query.Build()
	require.NotNil(t, q)

	// SQL should have columns in the specified order
	assert.Contains(t, q.sql, `("email", "name", "age")`)

	// Parameters should also be in correct order
	assert.Equal(t, "alice@example.com", q.params[0])
	assert.Equal(t, "Alice", q.params[1])
	assert.Equal(t, 30, q.params[2])
}

// TestBatchUpdate_ColumnOrder tests that columns are sorted for consistency.
func TestBatchUpdate_ColumnOrder(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Add columns in non-alphabetic order
	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{
			"zzz":  "last",
			"aaa":  "first",
			"name": "middle",
		})

	q := query.Build()
	require.NotNil(t, q)

	// Columns should be sorted alphabetically in SQL
	sql := q.sql
	aIndex := findIndex(sql, `"aaa"`)
	nameIndex := findIndex(sql, `"name"`)
	zIndex := findIndex(sql, `"zzz"`)

	assert.True(t, aIndex < nameIndex, "aaa should come before name")
	assert.True(t, nameIndex < zIndex, "name should come before zzz")
}

// Helper function to find index of substring.
func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
