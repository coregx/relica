package core

import (
	"context"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func BenchmarkHealthCheck(b *testing.B) {
	db, err := Open("sqlite", ":memory:",
		WithHealthCheck(100*time.Millisecond))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// Wait for first health check
	time.Sleep(150 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.IsHealthy()
	}
}

func BenchmarkStats(b *testing.B) {
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.Stats()
	}
}

func BenchmarkWarmCache(b *testing.B) {
	queries := []string{
		"SELECT 1",
		"SELECT 2",
		"SELECT 3",
		"SELECT 4",
		"SELECT 5",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db, err := NewDB("sqlite", ":memory:")
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		_, err = db.WarmCache(queries)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		db.Close()
	}
}

func BenchmarkWarmCache_100Queries(b *testing.B) {
	// Generate 100 distinct queries
	queries := make([]string, 100)
	for i := 0; i < 100; i++ {
		queries[i] = "SELECT ?"
	}

	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = db.WarmCache(queries)
		if err != nil {
			b.Fatal(err)
		}

		// Clear cache for next iteration
		b.StopTimer()
		db.stmtCache.Clear()
		b.StartTimer()
	}
}

func BenchmarkPinQuery(b *testing.B) {
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	query := "SELECT 1"
	db.WarmCache([]string{query})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.PinQuery(query)
		db.UnpinQuery(query)
	}
}

func BenchmarkConnectionPool_Concurrent(b *testing.B) {
	db, err := Open("sqlite", ":memory:",
		WithMaxOpenConns(10),
		WithMaxIdleConns(5))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE test (id INTEGER, value TEXT)")
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Simulate concurrent queries
			rows, err := db.QueryContext(ctx, "SELECT * FROM test WHERE id = ?", 1)
			if err != nil {
				b.Fatal(err)
			}
			rows.Close()
		}
	})
}
