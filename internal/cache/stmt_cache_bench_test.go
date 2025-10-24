package cache

import (
	"database/sql"
	"fmt"
	"testing"
)

// setupBenchDB creates a mock database for benchmarking.
func setupBenchDB(b *testing.B) *sql.DB {
	b.Helper()
	db, err := registerMockDriver()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

// createBenchStmt creates a prepared statement for benchmarking.
func createBenchStmt(b *testing.B, db *sql.DB, query string) *sql.Stmt {
	b.Helper()
	stmt, err := db.Prepare(query)
	if err != nil {
		b.Fatal(err)
	}
	return stmt
}

func BenchmarkStmtCache_Get_Hit(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCache()

	// Pre-populate cache.
	stmt := createBenchStmt(b, db, "SELECT 1")
	cache.Set("SELECT 1", stmt)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cache.Get("SELECT 1")
	}
}

func BenchmarkStmtCache_Get_Miss(b *testing.B) {
	cache := NewStmtCache()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cache.Get("nonexistent")
	}
}

func BenchmarkStmtCache_Set_NoEviction(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCacheWithCapacity(10000) // Large enough to avoid evictions.

	// Pre-create statements.
	stmts := make([]*sql.Stmt, b.N)
	for i := 0; i < b.N; i++ {
		stmts[i] = createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cache.Set(fmt.Sprintf("query_%d", i), stmts[i])
	}
}

func BenchmarkStmtCache_Set_WithEviction(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCacheWithCapacity(100) // Small cache to force evictions.

	// Pre-create statements.
	stmts := make([]*sql.Stmt, b.N)
	for i := 0; i < b.N; i++ {
		stmts[i] = createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cache.Set(fmt.Sprintf("query_%d", i), stmts[i])
	}
}

func BenchmarkStmtCache_GetSet_Mixed(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCacheWithCapacity(1000)

	// Pre-populate with some statements.
	for i := 0; i < 100; i++ {
		stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
		cache.Set(fmt.Sprintf("query_%d", i), stmt)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("query_%d", i%200)
		if _, found := cache.Get(key); !found {
			stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
			cache.Set(key, stmt)
		}
	}
}

func BenchmarkStmtCache_Parallel_Get(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCache()

	// Pre-populate cache.
	for i := 0; i < 100; i++ {
		stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
		cache.Set(fmt.Sprintf("query_%d", i), stmt)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("query_%d", i%100)
			_, _ = cache.Get(key)
			i++
		}
	})
}

func BenchmarkStmtCache_Parallel_Set(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCacheWithCapacity(1000)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("query_%d", i)
			stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
			cache.Set(key, stmt)
			i++
		}
	})
}

func BenchmarkStmtCache_Parallel_Mixed(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCacheWithCapacity(1000)

	// Pre-populate cache.
	for i := 0; i < 500; i++ {
		stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
		cache.Set(fmt.Sprintf("query_%d", i), stmt)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("query_%d", i%1000)
			if _, found := cache.Get(key); !found {
				stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
				cache.Set(key, stmt)
			}
			i++
		}
	})
}

// BenchmarkStmtCache_Sizes tests different cache sizes.
func BenchmarkStmtCache_Sizes(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			db := setupBenchDB(b)
			cache := NewStmtCacheWithCapacity(size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("query_%d", i%size)
				if _, found := cache.Get(key); !found {
					stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
					cache.Set(key, stmt)
				}
			}
		})
	}
}

// BenchmarkStmtCache_HighContention simulates high lock contention.
func BenchmarkStmtCache_HighContention(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCacheWithCapacity(10) // Small cache for high eviction rate.

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("query_%d", i%5) // High key collision.
			if _, found := cache.Get(key); !found {
				stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
				cache.Set(key, stmt)
			}
			i++
		}
	})
}

// BenchmarkStmtCache_Stats measures stats collection overhead.
func BenchmarkStmtCache_Stats(b *testing.B) {
	db := setupBenchDB(b)
	cache := NewStmtCache()

	// Pre-populate cache.
	for i := 0; i < 100; i++ {
		stmt := createBenchStmt(b, db, fmt.Sprintf("SELECT %d", i))
		cache.Set(fmt.Sprintf("query_%d", i), stmt)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = cache.Stats()
	}
}
