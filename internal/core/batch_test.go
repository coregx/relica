package core

import (
	"testing"
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	sql := q.sql
	checks := []string{
		`INSERT INTO "users"`,
		`("name", "email")`,
		"VALUES",
		"($1, $2)",
		"($3, $4)",
		"($5, $6)",
	}
	for _, s := range checks {
		if !containsStr(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}

	// Verify parameters
	if len(q.params) != 6 {
		t.Errorf("expected length %d, got %d", 6, len(q.params))
	}
	wantParams := []interface{}{"Alice", "alice@example.com", "Bob", "bob@example.com", "Charlie", "charlie@example.com"}
	for i, want := range wantParams {
		if q.params[i] != want {
			t.Errorf("param[%d]: got %v, want %v", i, q.params[i], want)
		}
	}
}

// TestBatchInsert_MySQL tests batch INSERT SQL generation for MySQL.
func TestBatchInsert_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	sql := q.sql
	checks := []string{
		"INSERT INTO `users`",
		"(`name`, `email`)",
		"VALUES (?, ?), (?, ?)",
	}
	for _, s := range checks {
		if !containsStr(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}

	// Verify parameters
	if len(q.params) != 4 {
		t.Errorf("expected length %d, got %d", 4, len(q.params))
	}
	if q.params[0] != "Alice" {
		t.Errorf("got %v, want %v", q.params[0], "Alice")
	}
	if q.params[1] != "alice@example.com" {
		t.Errorf("got %v, want %v", q.params[1], "alice@example.com")
	}
}

// TestBatchInsert_SQLite tests batch INSERT SQL generation for SQLite.
func TestBatchInsert_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("products", []string{"name", "price", "stock"}).
		Values("Widget", 9.99, 100).
		Values("Gadget", 19.99, 50)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	sql := q.sql
	checks := []string{
		`INSERT INTO "products"`,
		`("name", "price", "stock")`,
		"VALUES (?, ?, ?), (?, ?, ?)",
	}
	for _, s := range checks {
		if !containsStr(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}

	// Verify parameters
	if len(q.params) != 6 {
		t.Errorf("expected length %d, got %d", 6, len(q.params))
	}
	if q.params[0] != "Widget" {
		t.Errorf("got %v, want %v", q.params[0], "Widget")
	}
	if q.params[1] != 9.99 {
		t.Errorf("got %v, want %v", q.params[1], 9.99)
	}
	if q.params[2] != 100 {
		t.Errorf("got %v, want %v", q.params[2], 100)
	}
}

// TestBatchInsert_SingleRow tests batch INSERT with a single row.
func TestBatchInsert_SingleRow(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name"}).
		Values("Alice")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !containsStr(q.sql, "VALUES ($1)") {
		t.Errorf("%q does not contain %q", q.sql, "VALUES ($1)")
	}
	if len(q.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(q.params))
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Should have 100 rows
	if len(q.params) != 200 { // 100 rows * 2 columns
		t.Errorf("expected length %d, got %d", 200, len(q.params))
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify columns are in correct order
	if !containsStr(q.sql, `("email", "name")`) {
		t.Errorf("%q does not contain %q", q.sql, `("email", "name")`)
	}
	// Verify parameters are in correct order (email, name)
	if q.params[0] != "alice@example.com" {
		t.Errorf("got %v, want %v", q.params[0], "alice@example.com")
	}
	if q.params[1] != "Alice" {
		t.Errorf("got %v, want %v", q.params[1], "Alice")
	}
	if q.params[2] != "bob@example.com" {
		t.Errorf("got %v, want %v", q.params[2], "bob@example.com")
	}
	if q.params[3] != "Bob" {
		t.Errorf("got %v, want %v", q.params[3], "Bob")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if len(q.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(q.params))
	}
	if q.params[0] != "Alice" {
		t.Errorf("got %v, want %v", q.params[0], "Alice")
	}
	if q.params[1] != "alice@example.com" {
		t.Errorf("got %v, want %v", q.params[1], "alice@example.com")
	}
	if q.params[2] != nil { // Missing age should be nil
		t.Errorf("expected nil, got %v", q.params[2])
	}
}

// TestBatchInsert_EmptyPanic tests that building without rows returns an error
// instead of panicking.
func TestBatchInsert_EmptyPanic(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name"})
	q := query.Build()
	if q.prepErr == nil {
		t.Error("Build with no rows must store an error")
	}
	if !containsStr(q.prepErr.Error(), "BatchInsert") {
		t.Errorf("%q does not contain %q", q.prepErr.Error(), "BatchInsert")
	}
}

// TestBatchInsert_WrongValueCount tests that wrong value count stores an error
// instead of panicking.
func TestBatchInsert_WrongValueCount(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email"})

	query.Values("Alice") // Only 1 value, expected 2
	if query.buildErr == nil {
		t.Error("wrong value count must store build error")
	}
	if !containsStr(query.buildErr.Error(), "BatchInsert.Values") {
		t.Errorf("%q does not contain %q", query.buildErr.Error(), "BatchInsert.Values")
	}

	query2 := qb.BatchInsert("users", []string{"name", "email"})
	query2.Values("Alice", "alice@example.com", "extra") // 3 values, expected 2
	if query2.buildErr == nil {
		t.Error("too many values must store build error")
	}
	if !containsStr(query2.buildErr.Error(), "BatchInsert.Values") {
		t.Errorf("%q does not contain %q", query2.buildErr.Error(), "BatchInsert.Values")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	sql := q.sql
	checks := []string{
		`UPDATE "users"`,
		`SET`,
		`"name" = CASE "id"`,
		`"status" = CASE "id"`,
		`WHEN $1 THEN $2`,
		`WHERE "id" IN ($13, $14, $15)`,
	}
	for _, s := range checks {
		if !containsStr(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}

	// Verify we have the right number of parameters
	// 3 rows * 2 columns * 2 params (key + value) + 3 WHERE IN params = 15
	if len(q.params) != 15 {
		t.Errorf("expected length %d, got %d", 15, len(q.params))
	}
}

// TestBatchUpdate_MySQL tests batch UPDATE SQL generation for MySQL.
func TestBatchUpdate_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice", "email": "alice@new.com"}).
		Set(2, map[string]interface{}{"name": "Bob", "email": "bob@new.com"})

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	checks := []string{
		"UPDATE `users`",
		"`name` = CASE `id`",
		"`email` = CASE `id`",
		"WHERE `id` IN (?, ?)",
	}
	for _, s := range checks {
		if !containsStr(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}
}

// TestBatchUpdate_SQLite tests batch UPDATE SQL generation for SQLite.
func TestBatchUpdate_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("products", "id").
		Set(1, map[string]interface{}{"price": 10.99}).
		Set(2, map[string]interface{}{"price": 20.99})

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	if !containsStr(sql, `UPDATE "products"`) {
		t.Errorf("%q does not contain %q", sql, `UPDATE "products"`)
	}
	if !containsStr(sql, `"price" = CASE "id"`) {
		t.Errorf("%q does not contain %q", sql, `"price" = CASE "id"`)
	}
}

// TestBatchUpdate_SingleRow tests batch UPDATE with a single row.
func TestBatchUpdate_SingleRow(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice"})

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if !containsStr(q.sql, `UPDATE "users"`) {
		t.Errorf("%q does not contain %q", q.sql, `UPDATE "users"`)
	}
	if !containsStr(q.sql, `WHERE "id" IN ($3)`) {
		t.Errorf("%q does not contain %q", q.sql, `WHERE "id" IN ($3)`)
	}
	// 1 row * 1 column * 2 params (key + value) + 1 WHERE IN = 3 params
	if len(q.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(q.params))
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// 50 rows * 1 column * 2 params (key + value) + 50 WHERE IN params = 150
	if len(q.params) != 150 {
		t.Errorf("expected length %d, got %d", 150, len(q.params))
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	// Should have CASE for each column
	checks := []string{
		`"age" = CASE "id"`,
		`"email" = CASE "id"`,
		`"name" = CASE "id"`,
		`"status" = CASE "id"`,
	}
	for _, s := range checks {
		if !containsStr(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	// Should have CASE for all unique columns (age, email, name)
	checks := []string{
		`"age" = CASE "id"`,
		`"email" = CASE "id"`,
		`"name" = CASE "id"`,
	}
	for _, s := range checks {
		if !containsStr(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}

	// Row 2 only updates age, so only row 2's key should appear in age CASE
	// This is complex to verify in generated SQL, but we can check param count
	// Row 1: name+email (2*2=4 params)
	// Row 2: age (1*2=2 params)
	// Row 3: name+age (2*2=4 params)
	// WHERE IN: 3 params
	// Total: 4+2+4+3 = 13 params
	if len(q.params) != 13 {
		t.Errorf("expected length %d, got %d", 13, len(q.params))
	}
}

// TestBatchUpdate_EmptyPanic tests that building without updates returns an error
// instead of panicking.
func TestBatchUpdate_EmptyPanic(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id")
	q := query.Build()
	if q.prepErr == nil {
		t.Error("Build with no updates must store an error")
	}
	if !containsStr(q.prepErr.Error(), "BatchUpdate") {
		t.Errorf("%q does not contain %q", q.prepErr.Error(), "BatchUpdate")
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}
	if len(q.params) != 6 { // 3 rows * 2 columns
		t.Errorf("expected length %d, got %d", 6, len(q.params))
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !containsStr(q.sql, "WHERE") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE")
	}
}

// TestBatchInsert_NullValues tests batch INSERT with NULL values.
func TestBatchInsert_NullValues(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchInsert("users", []string{"name", "email", "age"}).
		Values("Alice", "alice@example.com", nil). // age is NULL
		Values("Bob", nil, 30)                     // email is NULL

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	if len(q.params) != 6 {
		t.Errorf("expected length %d, got %d", 6, len(q.params))
	}
	if q.params[2] != nil { // Alice's age
		t.Errorf("expected nil, got %v", q.params[2])
	}
	if q.params[4] != nil { // Bob's email
		t.Errorf("expected nil, got %v", q.params[4])
	}
}

// TestBatchUpdate_NullValues tests batch UPDATE with NULL values.
func TestBatchUpdate_NullValues(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"email": nil}). // Set email to NULL
		Set(2, map[string]interface{}{"email": "bob@example.com"})

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Should contain NULL value in params
	foundNull := false
	for _, param := range q.params {
		if param == nil {
			foundNull = true
			break
		}
	}
	if !foundNull {
		t.Error("Should have NULL parameter")
	}
}

// TestBatchInsert_QuoteIdentifiers tests proper identifier quoting.
func TestBatchInsert_QuoteIdentifiers(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Use column names that need quoting
	query := qb.BatchInsert("user_table", []string{"user_name", "user_email"}).
		Values("Alice", "alice@example.com")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// PostgreSQL should quote with double quotes
	checks := []string{`"user_table"`, `"user_name"`, `"user_email"`}
	for _, s := range checks {
		if !containsStr(q.sql, s) {
			t.Errorf("%q does not contain %q", q.sql, s)
		}
	}
}

// TestBatchUpdate_QuoteIdentifiers tests proper identifier quoting in UPDATE.
func TestBatchUpdate_QuoteIdentifiers(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.BatchUpdate("user_table", "user_id").
		Set(1, map[string]interface{}{"user_name": "Alice"})

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// MySQL should quote with backticks
	checks := []string{"`user_table`", "`user_id`", "`user_name`"}
	for _, s := range checks {
		if !containsStr(q.sql, s) {
			t.Errorf("%q does not contain %q", q.sql, s)
		}
	}
}

// TestBatchInsert_ColumnOrder tests that column order is preserved.
func TestBatchInsert_ColumnOrder(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Specify columns in specific order
	query := qb.BatchInsert("users", []string{"email", "name", "age"}).
		Values("alice@example.com", "Alice", 30)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// SQL should have columns in the specified order
	if !containsStr(q.sql, `("email", "name", "age")`) {
		t.Errorf("%q does not contain %q", q.sql, `("email", "name", "age")`)
	}

	// Parameters should also be in correct order
	if q.params[0] != "alice@example.com" {
		t.Errorf("got %v, want %v", q.params[0], "alice@example.com")
	}
	if q.params[1] != "Alice" {
		t.Errorf("got %v, want %v", q.params[1], "Alice")
	}
	if q.params[2] != 30 {
		t.Errorf("got %v, want %v", q.params[2], 30)
	}
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
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Columns should be sorted alphabetically in SQL
	sql := q.sql
	aIndex := findIndex(sql, `"aaa"`)
	nameIndex := findIndex(sql, `"name"`)
	zIndex := findIndex(sql, `"zzz"`)

	if aIndex >= nameIndex {
		t.Errorf("aaa should come before name")
	}
	if nameIndex >= zIndex {
		t.Errorf("name should come before zzz")
	}
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

// containsStr is a local helper to avoid import of strings in this file.
func containsStr(s, sub string) bool {
	return findIndex(s, sub) >= 0
}
