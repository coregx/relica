package cache

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates a mock database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := registerMockDriver()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

// createTestStmt creates a prepared statement for testing.
func createTestStmt(t *testing.T, db *sql.DB, query string) *sql.Stmt {
	t.Helper()
	stmt, err := db.Prepare(query)
	require.NoError(t, err)
	return stmt
}

func TestNewStmtCache(t *testing.T) {
	cache := NewStmtCache()
	require.NotNil(t, cache)
	assert.Equal(t, DefaultStmtCacheCapacity, cache.capacity)
	assert.Equal(t, 0, cache.lruList.Len())
	assert.Equal(t, 0, len(cache.items))
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
			require.NotNil(t, cache)
			assert.Equal(t, tt.expected, cache.capacity)
		})
	}
}

func TestStmtCache_GetSet(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCache()

	// Test miss on empty cache.
	stmt, found := cache.Get("SELECT 1")
	assert.Nil(t, stmt)
	assert.False(t, found)

	// Add statement to cache.
	testStmt := createTestStmt(t, db, "SELECT 1")
	cache.Set("SELECT 1", testStmt)

	// Test hit.
	stmt, found = cache.Get("SELECT 1")
	assert.NotNil(t, stmt)
	assert.True(t, found)
	assert.Equal(t, testStmt, stmt)

	// Verify cache size.
	stats := cache.Stats()
	assert.Equal(t, 1, stats.Size)
	assert.Equal(t, uint64(1), stats.Hits)
	assert.Equal(t, uint64(1), stats.Misses)
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
	assert.Equal(t, 3, stats.Size)
	assert.Equal(t, uint64(0), stats.Evictions)

	// Add one more statement - should evict oldest (query1).
	stmt4 := createTestStmt(t, db, "SELECT 4")
	cache.Set("query4", stmt4)

	stats = cache.Stats()
	assert.Equal(t, 3, stats.Size)
	assert.Equal(t, uint64(1), stats.Evictions)

	// Verify query1 was evicted.
	_, found := cache.Get("query1")
	assert.False(t, found)

	// Verify others still exist.
	_, found = cache.Get("query2")
	assert.True(t, found)
	_, found = cache.Get("query3")
	assert.True(t, found)
	_, found = cache.Get("query4")
	assert.True(t, found)
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
	require.True(t, found)

	// Add new statement - should evict query2 (now least recently used).
	stmt4 := createTestStmt(t, db, "SELECT 4")
	cache.Set("query4", stmt4)

	// Verify query2 was evicted, not query1.
	_, found = cache.Get("query2")
	assert.False(t, found)

	_, found = cache.Get("query1")
	assert.True(t, found)
}

func TestStmtCache_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCache()

	// Add initial statement.
	stmt1 := createTestStmt(t, db, "SELECT 1")
	cache.Set("query", stmt1)

	stats := cache.Stats()
	assert.Equal(t, 1, stats.Size)

	// Update with new statement (same key).
	stmt2 := createTestStmt(t, db, "SELECT 2")
	cache.Set("query", stmt2)

	// Cache size should remain 1.
	stats = cache.Stats()
	assert.Equal(t, 1, stats.Size)

	// Retrieved statement should be the new one.
	retrieved, found := cache.Get("query")
	require.True(t, found)
	assert.Equal(t, stmt2, retrieved)
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
	assert.Equal(t, 5, stats.Size)

	// Clear cache.
	cache.Clear()

	stats = cache.Stats()
	assert.Equal(t, 0, stats.Size)

	// Verify all statements are gone.
	for i := 1; i <= 5; i++ {
		_, found := cache.Get(fmt.Sprintf("query%d", i))
		assert.False(t, found)
	}
}

func TestStmtCache_Stats(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(2)

	// Initial stats.
	stats := cache.Stats()
	assert.Equal(t, 0, stats.Size)
	assert.Equal(t, 2, stats.Capacity)
	assert.Equal(t, uint64(0), stats.Hits)
	assert.Equal(t, uint64(0), stats.Misses)
	assert.Equal(t, uint64(0), stats.Evictions)
	assert.Equal(t, 0.0, stats.HitRate)

	// Add statement and test miss.
	stmt1 := createTestStmt(t, db, "SELECT 1")
	cache.Set("query1", stmt1)

	_, found := cache.Get("nonexistent")
	assert.False(t, found)

	stats = cache.Stats()
	assert.Equal(t, 1, stats.Size)
	assert.Equal(t, uint64(0), stats.Hits)
	assert.Equal(t, uint64(1), stats.Misses)
	assert.Equal(t, 0.0, stats.HitRate)

	// Test hit.
	_, found = cache.Get("query1")
	assert.True(t, found)

	stats = cache.Stats()
	assert.Equal(t, uint64(1), stats.Hits)
	assert.Equal(t, uint64(1), stats.Misses)
	assert.Equal(t, 0.5, stats.HitRate)

	// Test eviction.
	stmt2 := createTestStmt(t, db, "SELECT 2")
	stmt3 := createTestStmt(t, db, "SELECT 3")
	cache.Set("query2", stmt2)
	cache.Set("query3", stmt3) // Should evict query1.

	stats = cache.Stats()
	assert.Equal(t, 2, stats.Size)
	assert.Equal(t, uint64(1), stats.Evictions)
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
	assert.LessOrEqual(t, stats.Size, 100)
	assert.Greater(t, stats.Hits+stats.Misses, uint64(0))
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
	assert.LessOrEqual(t, stats.Size, 10)
	assert.Greater(t, stats.Evictions, uint64(0))
}

func TestStmtCache_EmptyCache(t *testing.T) {
	cache := NewStmtCache()

	// Test operations on empty cache.
	_, found := cache.Get("anything")
	assert.False(t, found)

	cache.Clear() // Should not panic.

	stats := cache.Stats()
	assert.Equal(t, 0, stats.Size)
	assert.Equal(t, 0.0, stats.HitRate)
}

func TestStmtCache_SingleItemCache(t *testing.T) {
	db := setupTestDB(t)
	cache := NewStmtCacheWithCapacity(1)

	stmt1 := createTestStmt(t, db, "SELECT 1")
	cache.Set("query1", stmt1)

	stats := cache.Stats()
	assert.Equal(t, 1, stats.Size)
	assert.Equal(t, uint64(0), stats.Evictions)

	// Add second item - should evict first.
	stmt2 := createTestStmt(t, db, "SELECT 2")
	cache.Set("query2", stmt2)

	stats = cache.Stats()
	assert.Equal(t, 1, stats.Size)
	assert.Equal(t, uint64(1), stats.Evictions)

	// First item should be gone.
	_, found := cache.Get("query1")
	assert.False(t, found)

	// Second item should exist.
	_, found = cache.Get("query2")
	assert.True(t, found)
}
