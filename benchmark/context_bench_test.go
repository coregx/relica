package benchmark

import (
	"context"
	"testing"

	"github.com/coregx/relica/internal/core"
	_ "modernc.org/sqlite"
)

// setupContextBenchDB creates a test database for benchmarking
func setupContextBenchDB(b *testing.B) *core.DB {
	db, err := core.NewDB("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("Failed to create test database: %v", err)
	}

	// Create test table
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE bench_users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	if err != nil {
		b.Fatalf("Failed to create test table: %v", err)
	}

	// Insert test data
	for i := 1; i <= 1000; i++ {
		_, err = db.ExecContext(context.Background(),
			"INSERT INTO bench_users (id, name, email) VALUES (?, ?, ?)",
			i, "User"+string(rune(i)), "user"+string(rune(i))+"@example.com")
		if err != nil {
			b.Fatalf("Failed to insert test data: %v", err)
		}
	}

	return db
}

// BenchmarkQuery_WithContext benchmarks queries with context
func BenchmarkQuery_WithContext(b *testing.B) {
	db := setupContextBenchDB(b)
	defer db.Close()

	ctx := context.Background()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var user User
		_ = db.Builder().
			WithContext(ctx).
			Select().
			From("bench_users").
			Where("id = ?", 1).
			One(&user)
	}
}

// BenchmarkQuery_WithoutContext benchmarks queries without context
func BenchmarkQuery_WithoutContext(b *testing.B) {
	db := setupContextBenchDB(b)
	defer db.Close()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var user User
		_ = db.Builder().
			Select().
			From("bench_users").
			Where("id = ?", 1).
			One(&user)
	}
}

// BenchmarkQuery_ContextOverhead measures the overhead of context
func BenchmarkQuery_ContextOverhead(b *testing.B) {
	db := setupContextBenchDB(b)
	defer db.Close()

	ctx := context.Background()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	b.Run("WithContext", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var user User
			_ = db.Builder().
				WithContext(ctx).
				Select().
				From("bench_users").
				Where("id = ?", 1).
				One(&user)
		}
	})

	b.Run("WithoutContext", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var user User
			_ = db.Builder().
				Select().
				From("bench_users").
				Where("id = ?", 1).
				One(&user)
		}
	})
}

// BenchmarkBuilder_WithContext benchmarks builder context setting
func BenchmarkBuilder_WithContext(b *testing.B) {
	db, _ := core.NewDB("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = db.Builder().WithContext(ctx)
	}
}

// BenchmarkSelectQuery_WithContext benchmarks query context setting
func BenchmarkSelectQuery_WithContext(b *testing.B) {
	db, _ := core.NewDB("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	builder := db.Builder()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = builder.Select().From("bench_users").WithContext(ctx)
	}
}

// BenchmarkContextPropagation benchmarks context propagation through query chain
func BenchmarkContextPropagation(b *testing.B) {
	db := setupContextBenchDB(b)
	defer db.Close()

	ctx := context.Background()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	b.Run("BuilderContext", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var user User
			_ = db.Builder().
				WithContext(ctx).
				Select().
				From("bench_users").
				Where("id = ?", 1).
				One(&user)
		}
	})

	b.Run("QueryContext", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var user User
			_ = db.Builder().
				Select().
				From("bench_users").
				Where("id = ?", 1).
				WithContext(ctx).
				One(&user)
		}
	})

	b.Run("NoContext", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var user User
			_ = db.Builder().
				Select().
				From("bench_users").
				Where("id = ?", 1).
				One(&user)
		}
	})
}
