package core

import (
	"testing"

	_ "modernc.org/sqlite"
)

func TestDB_WarmCache(t *testing.T) {
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE test (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	// Warm cache with queries
	queries := []string{
		"SELECT * FROM test WHERE id = ?",
		"INSERT INTO test (id, name) VALUES (?, ?)",
		"UPDATE test SET name = ? WHERE id = ?",
	}

	n, err := db.WarmCache(queries)
	if err != nil {
		t.Fatalf("WarmCache failed: %v", err)
	}

	if n != len(queries) {
		t.Errorf("Expected %d queries warmed, got %d", len(queries), n)
	}

	// Verify queries are in cache
	for _, query := range queries {
		if _, exists := db.stmtCache.Get(query); !exists {
			t.Errorf("Query not in cache: %s", query)
		}
	}
}

func TestDB_PinQuery(t *testing.T) {
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.sqlDB.Exec("CREATE TABLE test (id INTEGER)")
	if err != nil {
		t.Fatal(err)
	}

	query := "SELECT * FROM test WHERE id = ?"

	// Warm cache first
	_, err = db.WarmCache([]string{query})
	if err != nil {
		t.Fatal(err)
	}

	// Pin the query
	if !db.PinQuery(query) {
		t.Error("PinQuery should return true for cached query")
	}

	// Verify it's pinned
	if !db.stmtCache.IsPinned(query) {
		t.Error("Query should be pinned")
	}

	// Try to pin non-cached query
	if db.PinQuery("SELECT * FROM another_table") {
		t.Error("PinQuery should return false for non-cached query")
	}
}

func TestDB_UnpinQuery(t *testing.T) {
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.sqlDB.Exec("CREATE TABLE test (id INTEGER)")
	if err != nil {
		t.Fatal(err)
	}

	query := "SELECT * FROM test WHERE id = ?"

	// Warm and pin
	db.WarmCache([]string{query})
	db.PinQuery(query)

	// Unpin
	if !db.UnpinQuery(query) {
		t.Error("UnpinQuery should return true for pinned query")
	}

	// Verify it's unpinned
	if db.stmtCache.IsPinned(query) {
		t.Error("Query should not be pinned after UnpinQuery")
	}
}
