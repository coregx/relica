package core

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestQuery_Prepare_Basic(t *testing.T) {
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
		CREATE TABLE prep_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			status INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	for i := 1; i <= 5; i++ {
		_, err = db.ExecContext(ctx, `INSERT INTO prep_test (name, status) VALUES (?, ?)`, "User"+string(rune('0'+i)), i)
		if err != nil {
			t.Fatalf("Failed to insert data: %v", err)
		}
	}

	// Test Prepare/Execute cycle
	q := db.NewQuery("SELECT * FROM prep_test WHERE status = ?")
	q = q.Prepare()
	defer q.Close()

	if !q.IsPrepared() {
		t.Error("Expected query to be prepared")
	}

	// Execute multiple times with different parameters
	for status := 1; status <= 3; status++ {
		var result struct {
			ID     int    `db:"id"`
			Name   string `db:"name"`
			Status int    `db:"status"`
		}
		q.params = []interface{}{status}
		err = q.One(&result)
		if err != nil {
			t.Fatalf("Failed to execute prepared query for status %d: %v", status, err)
		}
		if result.Status != status {
			t.Errorf("Expected status %d, got %d", status, result.Status)
		}
	}
}

func TestQuery_Prepare_CloseMultipleTimes(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")

	// Create prepared query
	q := db.NewQuery("SELECT 1")
	q = q.Prepare()

	if !q.IsPrepared() {
		t.Error("Expected query to be prepared")
	}

	// Close multiple times - should not panic
	err = q.Close()
	if err != nil {
		t.Errorf("First Close() returned error: %v", err)
	}

	if q.IsPrepared() {
		t.Error("Expected query to not be prepared after Close()")
	}

	// Second close should return nil
	err = q.Close()
	if err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}

	// Third close should also return nil
	err = q.Close()
	if err != nil {
		t.Errorf("Third Close() returned error: %v", err)
	}
}

func TestQuery_Prepare_NonPreparedClose(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")

	// Create non-prepared query
	q := db.NewQuery("SELECT 1")

	if q.IsPrepared() {
		t.Error("Expected query to not be prepared initially")
	}

	// Close on non-prepared query should return nil
	err = q.Close()
	if err != nil {
		t.Errorf("Close() on non-prepared query returned error: %v", err)
	}
}

func TestQuery_Prepare_AlreadyPrepared(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")

	// Create and prepare query
	q := db.NewQuery("SELECT 1")
	q = q.Prepare()
	defer q.Close()

	// Get statement pointer
	stmt := q.stmt

	// Prepare again - should be no-op
	q = q.Prepare()

	// Statement should be the same
	if q.stmt != stmt {
		t.Error("Expected Prepare() on already prepared query to be no-op")
	}

	if !q.IsPrepared() {
		t.Error("Expected query to still be prepared")
	}
}

func TestQuery_Prepare_ErrorOnExecution(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")

	// Create query referencing non-existent table
	// Note: SQLite doesn't validate table existence during prepare,
	// but will fail during execution
	q := db.NewQuery("SELECT * FROM nonexistent_table_xyz")
	q = q.Prepare()
	defer q.Close()

	// SQLite may or may not prepare this (depends on version/implementation)
	// The important test is that execution fails

	// Execute should return an error (table doesn't exist)
	var result struct {
		ID int `db:"id"`
	}
	err = q.One(&result)
	if err == nil {
		t.Error("Expected error when executing query with non-existent table")
	}
}

func TestQuery_Prepare_Execute(t *testing.T) {
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
		CREATE TABLE exec_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Prepare insert query
	q := db.NewQuery("INSERT INTO exec_test (value) VALUES (?)")
	q = q.Prepare()
	defer q.Close()

	if !q.IsPrepared() {
		t.Error("Expected query to be prepared")
	}

	// Execute multiple times
	for i := 0; i < 3; i++ {
		q.params = []interface{}{"value" + string(rune('A'+i))}
		_, err = q.Execute()
		if err != nil {
			t.Fatalf("Failed to execute prepared insert: %v", err)
		}
	}

	// Verify data
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM exec_test").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 rows, got %d", count)
	}
}

func TestQuery_Prepare_All(t *testing.T) {
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
		CREATE TABLE all_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			value INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	_, err = db.ExecContext(ctx, `
		INSERT INTO all_test (category, value) VALUES
		('A', 1), ('A', 2), ('B', 3), ('B', 4), ('C', 5)
	`)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Prepare select query
	q := db.NewQuery("SELECT * FROM all_test WHERE category = ?")
	q = q.Prepare()
	defer q.Close()

	type Row struct {
		ID       int    `db:"id"`
		Category string `db:"category"`
		Value    int    `db:"value"`
	}

	// Test category A
	var resultsA []Row
	q.params = []interface{}{"A"}
	err = q.All(&resultsA)
	if err != nil {
		t.Fatalf("Failed to fetch category A: %v", err)
	}
	if len(resultsA) != 2 {
		t.Errorf("Expected 2 rows for category A, got %d", len(resultsA))
	}

	// Test category B
	var resultsB []Row
	q.params = []interface{}{"B"}
	err = q.All(&resultsB)
	if err != nil {
		t.Fatalf("Failed to fetch category B: %v", err)
	}
	if len(resultsB) != 2 {
		t.Errorf("Expected 2 rows for category B, got %d", len(resultsB))
	}

	// Test category C
	var resultsC []Row
	q.params = []interface{}{"C"}
	err = q.All(&resultsC)
	if err != nil {
		t.Fatalf("Failed to fetch category C: %v", err)
	}
	if len(resultsC) != 1 {
		t.Errorf("Expected 1 row for category C, got %d", len(resultsC))
	}
}

func TestQuery_Prepare_BypassesCache(t *testing.T) {
	// Open test database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	db := WrapDB(sqlDB, "sqlite")

	query := "SELECT 42"

	// Get initial cache stats
	initialStats := db.stmtCache.Stats()

	// Execute prepared query (should NOT add to cache)
	q := db.NewQuery(query)
	q = q.Prepare()
	var result int
	q.params = nil
	err = q.Row(&result)
	if err != nil {
		t.Fatalf("Failed to execute prepared query: %v", err)
	}
	q.Close()

	// Cache should not have increased
	afterPrepareStats := db.stmtCache.Stats()
	if afterPrepareStats.Size > initialStats.Size {
		t.Error("Prepared query should not add to statement cache")
	}

	// Now execute without Prepare() - should use cache
	q2 := db.NewQuery(query)
	err = q2.Row(&result)
	if err != nil {
		t.Fatalf("Failed to execute non-prepared query: %v", err)
	}

	// Cache should now have the query
	afterNormalStats := db.stmtCache.Stats()
	if afterNormalStats.Size <= initialStats.Size {
		t.Error("Non-prepared query should add to statement cache")
	}
}
