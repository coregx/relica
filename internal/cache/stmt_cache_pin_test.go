package cache

import (
	"testing"
)

func TestStmtCache_Pin(t *testing.T) {
	cache := NewStmtCache()
	db := setupTestDB(t)

	stmt := createTestStmt(t, db, "SELECT 1")

	// Add to cache
	cache.Set("query1", stmt)

	// Pin it
	if !cache.Pin("query1") {
		t.Error("Pin should return true for existing query")
	}

	// Check it's pinned
	if !cache.IsPinned("query1") {
		t.Error("Query should be pinned")
	}

	// Try to pin non-existent query
	if cache.Pin("nonexistent") {
		t.Error("Pin should return false for non-existent query")
	}
}

func TestStmtCache_Unpin(t *testing.T) {
	cache := NewStmtCache()
	db := setupTestDB(t)

	stmt := createTestStmt(t, db, "SELECT 1")

	cache.Set("query1", stmt)
	cache.Pin("query1")

	// Unpin it
	if !cache.Unpin("query1") {
		t.Error("Unpin should return true for pinned query")
	}

	// Check it's unpinned
	if cache.IsPinned("query1") {
		t.Error("Query should not be pinned after unpin")
	}

	// Try to unpin again
	if !cache.Unpin("query1") {
		t.Error("Unpin should return true even if already unpinned")
	}
}

func TestStmtCache_PinnedNotEvicted(t *testing.T) {
	// Create small cache (capacity 3)
	cache := NewStmtCacheWithCapacity(3)
	db := setupTestDB(t)

	// Add 3 statements and pin the first one
	keys := []string{"1", "2", "3"}
	for i, key := range keys {
		stmt := createTestStmt(t, db, "SELECT 1")
		cache.Set(key, stmt)
		if i == 0 {
			cache.Pin(key)
		}
	}

	// Add 4th statement - should evict oldest unpinned (not the pinned one)
	stmt4 := createTestStmt(t, db, "SELECT 1")
	cache.Set("4", stmt4)

	// Pinned query should still be in cache
	if _, exists := cache.Get("1"); !exists {
		t.Error("Pinned query should not be evicted")
	}

	// Second query (oldest unpinned) should be evicted
	if _, exists := cache.Get("2"); exists {
		t.Error("Oldest unpinned query should be evicted")
	}
}

func TestStmtCache_IsPinned(t *testing.T) {
	cache := NewStmtCache()

	// Non-existent query
	if cache.IsPinned("nonexistent") {
		t.Error("Non-existent query should not be pinned")
	}

	db := setupTestDB(t)
	stmt := createTestStmt(t, db, "SELECT 1")

	cache.Set("query1", stmt)

	// Unpinned query
	if cache.IsPinned("query1") {
		t.Error("New query should not be pinned by default")
	}

	// Pinned query
	cache.Pin("query1")
	if !cache.IsPinned("query1") {
		t.Error("Pinned query should be pinned")
	}
}
