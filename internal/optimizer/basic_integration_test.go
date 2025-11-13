//go:build integration

package optimizer

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	_ "modernc.org/sqlite"
)

// setupPostgresOptimizer creates optimizer with real PostgreSQL connection.
func setupPostgresOptimizer(t *testing.T) (*BasicOptimizer, *sql.DB) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5432/test?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		t.Skipf("PostgreSQL not reachable: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Setup test table
	setupTestTable(t, db, "postgres")

	// Create optimizer
	opt, err := NewOptimizerForDB(db, "postgres", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create optimizer: %v", err)
	}

	return opt, db
}

// setupMySQLOptimizer creates optimizer with real MySQL connection.
func setupMySQLOptimizer(t *testing.T) (*BasicOptimizer, *sql.DB) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(localhost:3306)/test?parseTime=true"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("MySQL not available: %v", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		t.Skipf("MySQL not reachable: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Setup test table
	setupTestTable(t, db, "mysql")

	// Create optimizer
	opt, err := NewOptimizerForDB(db, "mysql", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create optimizer: %v", err)
	}

	return opt, db
}

// setupSQLiteOptimizer creates optimizer with real SQLite connection.
func setupSQLiteOptimizer(t *testing.T) (*BasicOptimizer, *sql.DB) {
	// Use in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("SQLite not available: %v", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		t.Skipf("SQLite not reachable: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Setup test table
	setupTestTable(t, db, "sqlite")

	// Create optimizer
	opt, err := NewOptimizerForDB(db, "sqlite", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create optimizer: %v", err)
	}

	return opt, db
}

// setupTestTable creates test table and data for integration tests.
func setupTestTable(t *testing.T, db *sql.DB, dialect string) {
	ctx := context.Background()

	// Drop existing table
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS optimizer_test_users")

	// Create table (syntax compatible across databases)
	var createSQL string
	switch dialect {
	case "postgres":
		createSQL = `
			CREATE TABLE optimizer_test_users (
				id SERIAL PRIMARY KEY,
				email VARCHAR(255) NOT NULL,
				name VARCHAR(255) NOT NULL,
				age INTEGER,
				status INTEGER DEFAULT 1
			)
		`
	case "mysql":
		createSQL = `
			CREATE TABLE optimizer_test_users (
				id INT AUTO_INCREMENT PRIMARY KEY,
				email VARCHAR(255) NOT NULL,
				name VARCHAR(255) NOT NULL,
				age INT,
				status INT DEFAULT 1
			)
		`
	case "sqlite":
		createSQL = `
			CREATE TABLE optimizer_test_users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				email TEXT NOT NULL,
				name TEXT NOT NULL,
				age INTEGER,
				status INTEGER DEFAULT 1
			)
		`
	}

	_, err := db.ExecContext(ctx, createSQL)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create index on email for indexed query tests
	var indexSQL string
	switch dialect {
	case "postgres", "mysql":
		indexSQL = "CREATE INDEX optimizer_test_users_email_idx ON optimizer_test_users(email)"
	case "sqlite":
		indexSQL = "CREATE INDEX optimizer_test_users_email_idx ON optimizer_test_users(email)"
	}

	_, err = db.ExecContext(ctx, indexSQL)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Insert test data (use compatible syntax)
	insertSQL := `INSERT INTO optimizer_test_users (email, name, age, status) VALUES (?, ?, ?, ?)`
	if dialect == "postgres" {
		insertSQL = `INSERT INTO optimizer_test_users (email, name, age, status) VALUES ($1, $2, $3, $4)`
	}

	users := []struct {
		email  string
		name   string
		age    int
		status int
	}{
		{"alice@example.com", "Alice", 30, 1},
		{"bob@example.com", "Bob", 25, 1},
		{"charlie@example.com", "Charlie", 35, 2},
	}

	for _, u := range users {
		_, err = db.ExecContext(ctx, insertSQL, u.email, u.name, u.age, u.status)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}
}

// Test with PostgreSQL
func TestBasicOptimizer_PostgreSQL_Integration(t *testing.T) {
	opt, db := setupPostgresOptimizer(t)
	ctx := context.Background()

	t.Run("fast_query_with_index", func(t *testing.T) {
		query := "SELECT * FROM optimizer_test_users WHERE email = $1"
		args := []interface{}{"alice@example.com"}

		// Simulate fast execution
		analysis, err := opt.Analyze(ctx, query, args, 10*time.Millisecond)
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}

		if analysis.SlowQuery {
			t.Error("expected fast query")
		}

		// Should use index (email has index)
		if !analysis.QueryPlan.UsesIndex {
			t.Error("expected query to use index")
		}
	})

	t.Run("slow_query_full_scan", func(t *testing.T) {
		query := "SELECT * FROM optimizer_test_users WHERE status = $1"
		args := []interface{}{1}

		// Simulate slow execution
		analysis, err := opt.Analyze(ctx, query, args, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}

		if !analysis.SlowQuery {
			t.Error("expected slow query")
		}

		// Should detect full scan (status has no index)
		if !analysis.QueryPlan.FullScan {
			t.Error("expected full scan")
		}

		// Should recommend index
		if len(analysis.MissingIndexes) == 0 {
			t.Error("expected index recommendation")
		}

		suggestions := opt.Suggest(analysis)
		if len(suggestions) < 2 {
			t.Errorf("expected at least 2 suggestions (slow + full_scan), got %d", len(suggestions))
		}
	})

	_ = db
}

// Test with MySQL
func TestBasicOptimizer_MySQL_Integration(t *testing.T) {
	opt, db := setupMySQLOptimizer(t)
	ctx := context.Background()

	t.Run("fast_query_with_index", func(t *testing.T) {
		query := "SELECT * FROM optimizer_test_users WHERE email = ?"
		args := []interface{}{"alice@example.com"}

		analysis, err := opt.Analyze(ctx, query, args, 10*time.Millisecond)
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}

		if analysis.SlowQuery {
			t.Error("expected fast query")
		}

		// MySQL should detect index usage
		if !analysis.QueryPlan.UsesIndex {
			t.Error("expected query to use index")
		}
	})

	t.Run("slow_query_full_scan", func(t *testing.T) {
		query := "SELECT * FROM optimizer_test_users WHERE age > ?"
		args := []interface{}{20}

		analysis, err := opt.Analyze(ctx, query, args, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}

		if !analysis.SlowQuery {
			t.Error("expected slow query")
		}

		suggestions := opt.Suggest(analysis)
		if len(suggestions) == 0 {
			t.Error("expected optimization suggestions")
		}
	})

	_ = db
}

// Test with SQLite
func TestBasicOptimizer_SQLite_Integration(t *testing.T) {
	opt, db := setupSQLiteOptimizer(t)
	ctx := context.Background()

	t.Run("fast_query_with_index", func(t *testing.T) {
		query := "SELECT * FROM optimizer_test_users WHERE email = ?"
		args := []interface{}{"alice@example.com"}

		analysis, err := opt.Analyze(ctx, query, args, 10*time.Millisecond)
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}

		if analysis.SlowQuery {
			t.Error("expected fast query")
		}

		// SQLite should detect index usage
		if !analysis.QueryPlan.UsesIndex {
			t.Error("expected query to use index")
		}
	})

	t.Run("slow_query_full_scan", func(t *testing.T) {
		query := "SELECT * FROM optimizer_test_users WHERE name LIKE ?"
		args := []interface{}{"%Alice%"}

		analysis, err := opt.Analyze(ctx, query, args, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}

		if !analysis.SlowQuery {
			t.Error("expected slow query")
		}

		if !analysis.QueryPlan.FullScan {
			t.Error("expected full scan for LIKE query")
		}

		suggestions := opt.Suggest(analysis)
		if len(suggestions) == 0 {
			t.Error("expected optimization suggestions")
		}
	})

	_ = db
}

// Test adapter functionality
func TestOptimizerAdapter_Integration(t *testing.T) {
	opt, db := setupPostgresOptimizer(t)
	adapter := NewOptimizerAdapter(opt)
	ctx := context.Background()

	query := "SELECT * FROM optimizer_test_users WHERE status = $1"
	args := []interface{}{1}

	// Test Analyze through adapter
	analysis, err := adapter.Analyze(ctx, query, args, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Adapter.Analyze failed: %v", err)
	}

	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}

	// Test Suggest through adapter
	suggestions := adapter.Suggest(analysis)
	if len(suggestions) == 0 {
		t.Error("expected suggestions from adapter")
	}

	_ = db
}

// Test NewOptimizerForDB with different drivers
func TestNewOptimizerForDB_UnsupportedDriver(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("SQLite not available: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Test with unsupported driver name
	_, err = NewOptimizerForDB(db, "unsupported_driver", 100*time.Millisecond)
	if err == nil {
		t.Error("expected error for unsupported driver")
	}
}
