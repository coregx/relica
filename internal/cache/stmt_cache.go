// Package cache provides caching utilities for database prepared statements.
package cache

import (
	"container/list"
	"database/sql"
	"sync"
	"sync/atomic"
)

const (
	// DefaultStmtCacheCapacity is the default maximum number of cached prepared statements.
	DefaultStmtCacheCapacity = 1000
)

// StmtCache stores prepared statements with LRU eviction policy.
type StmtCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*list.Element
	lruList  *list.List

	// Metrics using atomic for lock-free access.
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// cacheEntry represents a single cached prepared statement.
type cacheEntry struct {
	key  string
	stmt *sql.Stmt
}

// NewStmtCache creates a new prepared statement cache with default capacity.
func NewStmtCache() *StmtCache {
	return NewStmtCacheWithCapacity(DefaultStmtCacheCapacity)
}

// NewStmtCacheWithCapacity creates a new prepared statement cache with specified capacity.
func NewStmtCacheWithCapacity(capacity int) *StmtCache {
	if capacity <= 0 {
		capacity = DefaultStmtCacheCapacity
	}
	return &StmtCache{
		capacity: capacity,
		items:    make(map[string]*list.Element, capacity),
		lruList:  list.New(),
	}
}

// Get retrieves a prepared statement from cache by SQL query string.
// Returns the statement and true if found, nil and false otherwise.
// Accessing a statement moves it to the front of the LRU list.
func (sc *StmtCache) Get(key string) (*sql.Stmt, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	elem, exists := sc.items[key]
	if !exists {
		sc.misses.Add(1)
		return nil, false
	}

	// Move to front (most recently used).
	sc.lruList.MoveToFront(elem)
	sc.hits.Add(1)

	entry := elem.Value.(*cacheEntry)
	return entry.stmt, true
}

// Set stores a prepared statement in cache with SQL query string as key.
// If the cache is at capacity, the least recently used statement is evicted and closed.
func (sc *StmtCache) Set(key string, stmt *sql.Stmt) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Check if key already exists.
	if elem, exists := sc.items[key]; exists {
		// Update existing entry and move to front.
		sc.lruList.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		// Close old statement before replacing.
		_ = entry.stmt.Close() // Best effort close.
		entry.stmt = stmt
		return
	}

	// Evict if at capacity.
	if sc.lruList.Len() >= sc.capacity {
		sc.evictOldest()
	}

	// Add new entry to front.
	entry := &cacheEntry{
		key:  key,
		stmt: stmt,
	}
	elem := sc.lruList.PushFront(entry)
	sc.items[key] = elem
}

// evictOldest removes and closes the least recently used statement.
// Must be called with lock held.
func (sc *StmtCache) evictOldest() {
	elem := sc.lruList.Back()
	if elem == nil {
		return
	}

	sc.lruList.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(sc.items, entry.key)

	// Close the evicted statement (best effort).
	_ = entry.stmt.Close()
	sc.evictions.Add(1)
}

// Clear closes and removes all cached prepared statements.
func (sc *StmtCache) Clear() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Close all statements.
	for elem := sc.lruList.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*cacheEntry)
		_ = entry.stmt.Close() // Best effort close.
	}

	// Reset cache state.
	sc.items = make(map[string]*list.Element, sc.capacity)
	sc.lruList.Init()
}

// Stats holds cache performance metrics.
type Stats struct {
	Size      int     // Current number of cached statements.
	Capacity  int     // Maximum capacity.
	Hits      uint64  // Number of successful cache lookups.
	Misses    uint64  // Number of cache misses.
	Evictions uint64  // Number of evicted statements.
	HitRate   float64 // Cache hit rate (hits / total requests).
}

// Stats returns cache statistics.
func (sc *StmtCache) Stats() Stats {
	sc.mu.RLock()
	size := sc.lruList.Len()
	sc.mu.RUnlock()

	hits := sc.hits.Load()
	misses := sc.misses.Load()
	evictions := sc.evictions.Load()

	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return Stats{
		Size:      size,
		Capacity:  sc.capacity,
		Hits:      hits,
		Misses:    misses,
		Evictions: evictions,
		HitRate:   hitRate,
	}
}
