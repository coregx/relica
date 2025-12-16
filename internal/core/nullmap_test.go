package core

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNullStringMap_BasicMethods(t *testing.T) {
	m := NullStringMap{
		"name":  sql.NullString{String: "Alice", Valid: true},
		"email": sql.NullString{String: "alice@example.com", Valid: true},
		"phone": sql.NullString{String: "", Valid: false}, // NULL value
	}

	// Test String method
	if got := m.String("name"); got != "Alice" {
		t.Errorf("String(\"name\") = %q, want \"Alice\"", got)
	}
	if got := m.String("email"); got != "alice@example.com" {
		t.Errorf("String(\"email\") = %q, want \"alice@example.com\"", got)
	}
	if got := m.String("phone"); got != "" {
		t.Errorf("String(\"phone\") = %q, want \"\" (NULL)", got)
	}
	if got := m.String("nonexistent"); got != "" {
		t.Errorf("String(\"nonexistent\") = %q, want \"\"", got)
	}

	// Test IsNull method
	if m.IsNull("name") {
		t.Error("IsNull(\"name\") = true, want false")
	}
	if m.IsNull("email") {
		t.Error("IsNull(\"email\") = true, want false")
	}
	if !m.IsNull("phone") {
		t.Error("IsNull(\"phone\") = false, want true")
	}
	if !m.IsNull("nonexistent") {
		t.Error("IsNull(\"nonexistent\") = false, want true")
	}

	// Test Has method
	if !m.Has("name") {
		t.Error("Has(\"name\") = false, want true")
	}
	if !m.Has("phone") {
		t.Error("Has(\"phone\") = false, want true (key exists even if NULL)")
	}
	if m.Has("nonexistent") {
		t.Error("Has(\"nonexistent\") = true, want false")
	}

	// Test Keys method
	keys := m.Keys()
	if len(keys) != 3 {
		t.Errorf("Keys() returned %d keys, want 3", len(keys))
	}

	// Test Get method
	val, ok := m.Get("name")
	if !ok {
		t.Error("Get(\"name\") ok = false, want true")
	}
	if val.String != "Alice" || !val.Valid {
		t.Errorf("Get(\"name\") = %+v, want {String:\"Alice\", Valid:true}", val)
	}

	val, ok = m.Get("nonexistent")
	if ok {
		t.Error("Get(\"nonexistent\") ok = true, want false")
	}
}

func TestNullStringMap_ScanOne(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")
	ctx := context.Background()

	// Create test table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE test_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT,
			phone TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data with NULL value
	_, err = db.ExecContext(ctx, `
		INSERT INTO test_users (name, email, phone) VALUES ('Alice', 'alice@example.com', NULL)
	`)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Test scanning into NullStringMap
	var result NullStringMap
	err = db.Builder().
		Select("*").
		From("test_users").
		Where("name = ?", "Alice").
		One(&result)
	if err != nil {
		t.Fatalf("Failed to scan row: %v", err)
	}

	// Verify results
	if result.String("name") != "Alice" {
		t.Errorf("name = %q, want \"Alice\"", result.String("name"))
	}
	if result.String("email") != "alice@example.com" {
		t.Errorf("email = %q, want \"alice@example.com\"", result.String("email"))
	}
	if !result.IsNull("phone") {
		t.Error("phone should be NULL")
	}
	if !result.Has("id") {
		t.Error("result should have 'id' column")
	}
}

func TestNullStringMap_ScanAll(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")
	ctx := context.Background()

	// Create test table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE test_products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			price TEXT,
			category TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	_, err = db.ExecContext(ctx, `
		INSERT INTO test_products (name, price, category) VALUES
		('Product A', '10.99', 'Electronics'),
		('Product B', '25.50', NULL),
		('Product C', NULL, 'Books')
	`)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Test scanning multiple rows into []NullStringMap
	var results []NullStringMap
	err = db.Builder().
		Select("*").
		From("test_products").
		OrderBy("id").
		All(&results)
	if err != nil {
		t.Fatalf("Failed to scan rows: %v", err)
	}

	// Verify results
	if len(results) != 3 {
		t.Fatalf("Expected 3 rows, got %d", len(results))
	}

	// Check first row
	if results[0].String("name") != "Product A" {
		t.Errorf("Row 0 name = %q, want \"Product A\"", results[0].String("name"))
	}
	if results[0].IsNull("price") {
		t.Error("Row 0 price should not be NULL")
	}
	if results[0].IsNull("category") {
		t.Error("Row 0 category should not be NULL")
	}

	// Check second row
	if results[1].String("name") != "Product B" {
		t.Errorf("Row 1 name = %q, want \"Product B\"", results[1].String("name"))
	}
	if !results[1].IsNull("category") {
		t.Error("Row 1 category should be NULL")
	}

	// Check third row
	if results[2].String("name") != "Product C" {
		t.Errorf("Row 2 name = %q, want \"Product C\"", results[2].String("name"))
	}
	if !results[2].IsNull("price") {
		t.Error("Row 2 price should be NULL")
	}
}

func TestNullStringMap_EmptyResult(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")
	ctx := context.Background()

	// Create empty test table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE empty_table (
			id INTEGER PRIMARY KEY,
			name TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Test One() returns ErrNoRows for empty result
	var result NullStringMap
	err = db.Builder().
		Select("*").
		From("empty_table").
		One(&result)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected sql.ErrNoRows, got %v", err)
	}

	// Test All() returns empty slice for empty result
	var results []NullStringMap
	err = db.Builder().
		Select("*").
		From("empty_table").
		All(&results)
	if err != nil {
		t.Errorf("All() returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty slice, got %d rows", len(results))
	}
}

func TestNullStringMap_NumericValues(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")
	ctx := context.Background()

	// Create test table with numeric columns
	_, err = db.ExecContext(ctx, `
		CREATE TABLE numeric_test (
			id INTEGER PRIMARY KEY,
			int_val INTEGER,
			float_val REAL,
			bool_val INTEGER
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	_, err = db.ExecContext(ctx, `
		INSERT INTO numeric_test (id, int_val, float_val, bool_val) VALUES (1, 42, 3.14, 1)
	`)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Test scanning numeric values as strings
	var result NullStringMap
	err = db.Builder().
		Select("*").
		From("numeric_test").
		One(&result)
	if err != nil {
		t.Fatalf("Failed to scan row: %v", err)
	}

	// Values should be converted to strings
	if result.String("id") != "1" {
		t.Errorf("id = %q, want \"1\"", result.String("id"))
	}
	if result.String("int_val") != "42" {
		t.Errorf("int_val = %q, want \"42\"", result.String("int_val"))
	}
	// Float representation may vary
	floatVal := result.String("float_val")
	if floatVal != "3.14" && floatVal != "3.140000" {
		t.Logf("float_val = %q (representation may vary)", floatVal)
	}
}
