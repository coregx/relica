package cache

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
)

// setupTestDB creates a mock database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := registerMockDriver()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

// createTestStmt creates a prepared statement for testing.
func createTestStmt(t *testing.T, db *sql.DB, query string) *sql.Stmt {
	t.Helper()
	stmt, err := db.Prepare(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return stmt
}

func TestNewStmtCache(t *testing.T) {
	cache := NewStmtCache()
	if cache == nil {
		t.Fatal("expected non-nil")
	}
	if got, want := cache.capacity, DefaultStmtCacheCapacity; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := cache.lruList.Len(); got != 0 {
		t.Errorf("got %v, want %v", got, 0)
	}
	if got := len(cache.items); got != 0 {
		t.Errorf("got %v, want %v", got, 0)
	}
}

func TestNewStmtCacheWithCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		expected int
	}{
		{
			name:     "positive capacity",
			capacity: 100,
			expected: 100,
		},
		{
			name:     "zero capacity defaults to default",
			capacity: 0,
			expected: DefaultStmtCacheCapacity,
		},
		{
			name:     "negative capacity defaults to default",
			capacity: -10,
			expected: DefaultStmtCacheCapacity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewStmtCacheWithCapacity(tt.capacity)
			if cache == nil {
				t.Fatal("expected non-nil")
			}
			if got := cache.capacity; got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStmtCache_GetSet(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCache()

	// Test miss on empty cache.
	stmt, found := cache.Get("SELECT 1")
	if stmt != nil {
		t.Errorf("expected nil, got %v", stmt)
	}
	if found {
		t.Error("expected false")
	}

	// Add statement to cache.
	testStmt := createTestStmt(t, db, "SELECT 1")
	cache.Set("SELECT 1", testStmt)

	// Test hit.
	stmt, found = cache.Get("SELECT 1")
	if stmt == nil {
		t.Error("expected non-nil")
	}
	if !found {
		t.Error("expected true")
	}
	if got := stmt; got != testStmt {
		t.Errorf("got %v, want %v", got, testStmt)
	}

	// Verify cache size.
	stats := cache.Stats()
	if got := stats.Size; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}
	if got := stats.Hits; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}
	if got := stats.Misses; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}
}

func TestStmtCache_LRUEviction(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(3)

	// Fill cache to capacity.
	stmt1 := createTestStmt(t, db, "SELECT 1")
	stmt2 := createTestStmt(t, db, "SELECT 2")
	stmt3 := createTestStmt(t, db, "SELECT 3")

	cache.Set("query1", stmt1)
	cache.Set("query2", stmt2)
	cache.Set("query3", stmt3)

	stats := cache.Stats()
	if got := stats.Size; got != 3 {
		t.Errorf("got %v, want %v", got, 3)
	}
	if got := stats.Evictions; got != uint64(0) {
		t.Errorf("got %v, want %v", got, uint64(0))
	}

	// Add one more statement - should evict oldest (query1).
	stmt4 := createTestStmt(t, db, "SELECT 4")
	cache.Set("query4", stmt4)

	stats = cache.Stats()
	if got := stats.Size; got != 3 {
		t.Errorf("got %v, want %v", got, 3)
	}
	if got := stats.Evictions; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}

	// Verify query1 was evicted.
	_, found := cache.Get("query1")
	if found {
		t.Error("expected false")
	}

	// Verify others still exist.
	_, found = cache.Get("query2")
	if !found {
		t.Error("expected true")
	}
	_, found = cache.Get("query3")
	if !found {
		t.Error("expected true")
	}
	_, found = cache.Get("query4")
	if !found {
		t.Error("expected true")
	}
}

func TestStmtCache_LRUOrdering(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(3)

	// Add three statements.
	stmt1 := createTestStmt(t, db, "SELECT 1")
	stmt2 := createTestStmt(t, db, "SELECT 2")
	stmt3 := createTestStmt(t, db, "SELECT 3")

	cache.Set("query1", stmt1)
	cache.Set("query2", stmt2)
	cache.Set("query3", stmt3)

	// Access query1 to make it most recently used.
	_, found := cache.Get("query1")
	if !found {
		t.Fatal("expected true")
	}

	// Add new statement - should evict query2 (now least recently used).
	stmt4 := createTestStmt(t, db, "SELECT 4")
	cache.Set("query4", stmt4)

	// Verify query2 was evicted, not query1.
	_, found = cache.Get("query2")
	if found {
		t.Error("expected false")
	}

	_, found = cache.Get("query1")
	if !found {
		t.Error("expected true")
	}
}

func TestStmtCache_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCache()

	// Add initial statement.
	stmt1 := createTestStmt(t, db, "SELECT 1")
	cache.Set("query", stmt1)

	stats := cache.Stats()
	if got := stats.Size; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}

	// Update with new statement (same key).
	stmt2 := createTestStmt(t, db, "SELECT 2")
	cache.Set("query", stmt2)

	// Cache size should remain 1.
	stats = cache.Stats()
	if got := stats.Size; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}

	// Retrieved statement should be the new one.
	retrieved, found := cache.Get("query")
	if !found {
		t.Fatal("expected true")
	}
	if got := retrieved; got != stmt2 {
		t.Errorf("got %v, want %v", got, stmt2)
	}
}

func TestStmtCache_Clear(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCache()

	// Add multiple statements.
	for i := 1; i <= 5; i++ {
		stmt := createTestStmt(t, db, fmt.Sprintf("SELECT %d", i))
		cache.Set(fmt.Sprintf("query%d", i), stmt)
	}

	stats := cache.Stats()
	if got := stats.Size; got != 5 {
		t.Errorf("got %v, want %v", got, 5)
	}

	// Clear cache.
	cache.Clear()

	stats = cache.Stats()
	if got := stats.Size; got != 0 {
		t.Errorf("got %v, want %v", got, 0)
	}

	// Verify all statements are gone.
	for i := 1; i <= 5; i++ {
		_, found := cache.Get(fmt.Sprintf("query%d", i))
		if found {
			t.Error("expected false")
		}
	}
}

func TestStmtCache_Stats(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(2)

	// Initial stats.
	stats := cache.Stats()
	if got := stats.Size; got != 0 {
		t.Errorf("got %v, want %v", got, 0)
	}
	if got := stats.Capacity; got != 2 {
		t.Errorf("got %v, want %v", got, 2)
	}
	if got := stats.Hits; got != uint64(0) {
		t.Errorf("got %v, want %v", got, uint64(0))
	}
	if got := stats.Misses; got != uint64(0) {
		t.Errorf("got %v, want %v", got, uint64(0))
	}
	if got := stats.Evictions; got != uint64(0) {
		t.Errorf("got %v, want %v", got, uint64(0))
	}
	if got := stats.HitRate; got != 0.0 {
		t.Errorf("got %v, want %v", got, 0.0)
	}

	// Add statement and test miss.
	stmt1 := createTestStmt(t, db, "SELECT 1")
	cache.Set("query1", stmt1)

	_, found := cache.Get("nonexistent")
	if found {
		t.Error("expected false")
	}

	stats = cache.Stats()
	if got := stats.Size; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}
	if got := stats.Hits; got != uint64(0) {
		t.Errorf("got %v, want %v", got, uint64(0))
	}
	if got := stats.Misses; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}
	if got := stats.HitRate; got != 0.0 {
		t.Errorf("got %v, want %v", got, 0.0)
	}

	// Test hit.
	_, found = cache.Get("query1")
	if !found {
		t.Error("expected true")
	}

	stats = cache.Stats()
	if got := stats.Hits; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}
	if got := stats.Misses; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}
	if got := stats.HitRate; got != 0.5 {
		t.Errorf("got %v, want %v", got, 0.5)
	}

	// Test eviction.
	stmt2 := createTestStmt(t, db, "SELECT 2")
	stmt3 := createTestStmt(t, db, "SELECT 3")
	cache.Set("query2", stmt2)
	cache.Set("query3", stmt3) // Should evict query1.

	stats = cache.Stats()
	if got := stats.Size; got != 2 {
		t.Errorf("got %v, want %v", got, 2)
	}
	if got := stats.Evictions; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}
}

func TestStmtCache_Concurrent(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(100)

	const goroutines = 10
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Run concurrent Get/Set operations.
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()

			for i := 0; i < operations; i++ {
				key := fmt.Sprintf("query_%d_%d", id, i%10)

				// Try to get.
				if _, found := cache.Get(key); !found {
					// If not found, create and set.
					stmt := createTestStmt(t, db, fmt.Sprintf("SELECT %d", i))
					cache.Set(key, stmt)
				}
			}
		}(g)
	}

	wg.Wait()

	// Verify cache is in valid state.
	stats := cache.Stats()
	if stats.Size > 100 {
		t.Errorf("expected %v <= %v", stats.Size, 100)
	}
	if stats.Hits+stats.Misses <= uint64(0) {
		t.Errorf("expected %v > %v", stats.Hits+stats.Misses, uint64(0))
	}
}

func TestStmtCache_ConcurrentEviction(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(10)

	const goroutines = 5
	const operations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Force many evictions by adding more items than capacity.
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()

			for i := 0; i < operations; i++ {
				key := fmt.Sprintf("query_%d_%d", id, i)
				stmt := createTestStmt(t, db, fmt.Sprintf("SELECT %d", i))
				cache.Set(key, stmt)
			}
		}(g)
	}

	wg.Wait()

	// Verify cache respects capacity.
	stats := cache.Stats()
	if stats.Size > 10 {
		t.Errorf("expected %v <= %v", stats.Size, 10)
	}
	if stats.Evictions <= uint64(0) {
		t.Errorf("expected %v > %v", stats.Evictions, uint64(0))
	}
}

func TestStmtCache_EmptyCache(t *testing.T) {
	cache := NewStmtCache()

	// Test operations on empty cache.
	_, found := cache.Get("anything")
	if found {
		t.Error("expected false")
	}

	cache.Clear() // Should not panic.

	stats := cache.Stats()
	if got := stats.Size; got != 0 {
		t.Errorf("got %v, want %v", got, 0)
	}
	if got := stats.HitRate; got != 0.0 {
		t.Errorf("got %v, want %v", got, 0.0)
	}
}

func TestStmtCache_SingleItemCache(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(1)

	stmt1 := createTestStmt(t, db, "SELECT 1")
	cache.Set("query1", stmt1)

	stats := cache.Stats()
	if got := stats.Size; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}
	if got := stats.Evictions; got != uint64(0) {
		t.Errorf("got %v, want %v", got, uint64(0))
	}

	// Add second item - should evict first.
	stmt2 := createTestStmt(t, db, "SELECT 2")
	cache.Set("query2", stmt2)

	stats = cache.Stats()
	if got := stats.Size; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}
	if got := stats.Evictions; got != uint64(1) {
		t.Errorf("got %v, want %v", got, uint64(1))
	}

	// First item should be gone.
	_, found := cache.Get("query1")
	if found {
		t.Error("expected false")
	}

	// Second item should exist.
	_, found = cache.Get("query2")
	if !found {
		t.Error("expected true")
	}
}
