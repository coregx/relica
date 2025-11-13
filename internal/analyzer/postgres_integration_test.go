//go:build integration

package analyzer

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// setupPostgresTestDB creates a test PostgreSQL connection for integration testing.
// Requires PostgreSQL to be running (e.g., via Docker or local install).
// Set POSTGRES_DSN environment variable or uses default localhost connection.
func setupPostgresTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5432/test?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	// Verify connection
	if err := db.PingContext(context.Background()); err != nil {
		t.Skipf("PostgreSQL not reachable: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Create test table
	_, err = db.ExecContext(context.Background(), `
		DROP TABLE IF EXISTS test_users CASCADE
	`)
	if err != nil {
		t.Fatalf("Failed to drop table: %v", err)
	}

	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE test_users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			age INTEGER,
			status INTEGER DEFAULT 1
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create index for testing index usage detection
	_, err = db.ExecContext(context.Background(), `
		CREATE INDEX test_users_email_idx ON test_users(email)
	`)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Insert test data
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO test_users (email, name, age, status) VALUES
		('alice@example.com', 'Alice', 30, 1),
		('bob@example.com', 'Bob', 25, 1),
		('charlie@example.com', 'Charlie', 35, 2)
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	return db
}

// TestPostgresAnalyzer_Explain_Integration tests EXPLAIN functionality with real PostgreSQL.
func TestPostgresAnalyzer_Explain_Integration(t *testing.T) {
	db := setupPostgresTestDB(t)
	analyzer := NewPostgresAnalyzer(db)
	ctx := context.Background()

	t.Run("simple_select_with_index", func(t *testing.T) {
		query := "SELECT * FROM test_users WHERE email = $1"
		args := []interface{}{"alice@example.com"}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		// Verify basic fields
		if plan == nil {
			t.Fatal("Expected non-nil QueryPlan")
		}
		if plan.Database != "postgres" {
			t.Errorf("Database = %v, want postgres", plan.Database)
		}
		if plan.Cost <= 0 {
			t.Errorf("Cost = %v, expected > 0", plan.Cost)
		}
		if plan.EstimatedRows <= 0 {
			t.Errorf("EstimatedRows = %v, expected > 0", plan.EstimatedRows)
		}

		// Should use index (email has index)
		if !plan.UsesIndex {
			t.Error("Expected UsesIndex = true for indexed column")
		}
		if plan.IndexName == "" {
			t.Error("Expected IndexName to be set")
		}

		// Should not be full scan
		if plan.FullScan {
			t.Error("Expected FullScan = false for indexed query")
		}

		// EXPLAIN (not ANALYZE) should not have actual metrics
		if plan.ActualRows != 0 {
			t.Errorf("ActualRows = %v, expected 0 for EXPLAIN (not ANALYZE)", plan.ActualRows)
		}
		if plan.ActualTime != 0 {
			t.Errorf("ActualTime = %v, expected 0 for EXPLAIN (not ANALYZE)", plan.ActualTime)
		}

		// Raw output should be non-empty JSON
		if plan.RawOutput == "" {
			t.Error("Expected non-empty RawOutput")
		}
	})

	t.Run("full_table_scan", func(t *testing.T) {
		query := "SELECT * FROM test_users WHERE age > $1"
		args := []interface{}{20}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		// Should detect full scan (age has no index)
		if !plan.FullScan {
			t.Error("Expected FullScan = true for unindexed column")
		}

		// Cost should be higher than index scan
		if plan.Cost <= 0 {
			t.Errorf("Cost = %v, expected > 0", plan.Cost)
		}
	})
}

// TestPostgresAnalyzer_ExplainAnalyze_Integration tests EXPLAIN ANALYZE with real execution.
func TestPostgresAnalyzer_ExplainAnalyze_Integration(t *testing.T) {
	db := setupPostgresTestDB(t)
	analyzer := NewPostgresAnalyzer(db)
	ctx := context.Background()

	t.Run("explain_analyze_with_actual_metrics", func(t *testing.T) {
		query := "SELECT * FROM test_users WHERE email = $1"
		args := []interface{}{"alice@example.com"}

		plan, err := analyzer.ExplainAnalyze(ctx, query, args)
		if err != nil {
			t.Fatalf("ExplainAnalyze failed: %v", err)
		}

		// Verify basic fields (same as Explain)
		if plan == nil {
			t.Fatal("Expected non-nil QueryPlan")
		}
		if plan.Database != "postgres" {
			t.Errorf("Database = %v, want postgres", plan.Database)
		}

		// EXPLAIN ANALYZE should have actual metrics
		if plan.ActualRows == 0 {
			t.Error("Expected ActualRows > 0 for EXPLAIN ANALYZE")
		}
		if plan.ActualTime == 0 {
			t.Error("Expected ActualTime > 0 for EXPLAIN ANALYZE")
		}

		// Should still detect index usage
		if !plan.UsesIndex {
			t.Error("Expected UsesIndex = true")
		}

		// Buffer statistics should be present (BUFFERS option)
		// Note: These might be 0 if data is already in cache, which is OK
		totalBuffers := plan.BuffersHit + plan.BuffersMiss
		if totalBuffers == 0 {
			t.Log("Warning: No buffer statistics (data might be fully cached)")
		}
	})

	t.Run("explain_analyze_count_query", func(t *testing.T) {
		query := "SELECT COUNT(*) FROM test_users WHERE status = $1"
		args := []interface{}{1}

		plan, err := analyzer.ExplainAnalyze(ctx, query, args)
		if err != nil {
			t.Fatalf("ExplainAnalyze failed: %v", err)
		}

		// Should execute and return actual rows
		if plan.ActualRows == 0 {
			t.Error("Expected ActualRows > 0")
		}

		// Should have execution time
		if plan.ActualTime == 0 {
			t.Error("Expected ActualTime > 0")
		}
	})
}

// TestPostgresAnalyzer_InvalidQuery tests error handling.
func TestPostgresAnalyzer_InvalidQuery(t *testing.T) {
	db := setupPostgresTestDB(t)
	analyzer := NewPostgresAnalyzer(db)
	ctx := context.Background()

	t.Run("syntax_error", func(t *testing.T) {
		query := "SELECT * FORM invalid_syntax"
		args := []interface{}{}

		_, err := analyzer.Explain(ctx, query, args)
		if err == nil {
			t.Error("Expected error for invalid SQL syntax")
		}
	})

	t.Run("nonexistent_table", func(t *testing.T) {
		query := "SELECT * FROM nonexistent_table WHERE id = $1"
		args := []interface{}{1}

		_, err := analyzer.Explain(ctx, query, args)
		if err == nil {
			t.Error("Expected error for nonexistent table")
		}
	})
}
