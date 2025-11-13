//go:build integration

package analyzer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// setupMySQLTestDB creates a test database with sample data.
func setupMySQLTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	// MySQL connection string from environment or default to Docker
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		dsn = "root:testpass@tcp(localhost:3306)/testdb?parseTime=true"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to MySQL: %v", err)
	}

	// Wait for database to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			db.Close()
			t.Fatalf("Timeout waiting for MySQL to be ready")
		case <-ticker.C:
			if err := db.PingContext(ctx); err == nil {
				goto connected
			}
		}
	}

connected:
	// Create test table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INT PRIMARY KEY AUTO_INCREMENT,
			email VARCHAR(255) NOT NULL,
			status INT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX email_idx (email),
			INDEX status_idx (status)
		)
	`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create users table: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO users (email, status) VALUES
		('alice@example.com', 1),
		('bob@example.com', 1),
		('charlie@example.com', 0)
	`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to insert test data: %v", err)
	}

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Close()
	}

	return db, cleanup
}

func TestMySQLAnalyzer_Integration_Explain(t *testing.T) {
	db, cleanup := setupMySQLTestDB(t)
	defer cleanup()

	analyzer := NewMySQLAnalyzer(db)
	ctx := context.Background()

	t.Run("index scan on email", func(t *testing.T) {
		query := "SELECT * FROM users WHERE email = ?"
		args := []interface{}{"alice@example.com"}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v, IndexName=%s, FullScan=%v",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex, plan.IndexName, plan.FullScan)

		if !plan.UsesIndex {
			t.Error("Expected index usage for email lookup")
		}

		if plan.IndexName == "" {
			t.Error("Expected index name to be set")
		}

		if plan.FullScan {
			t.Error("Expected no full table scan for indexed column")
		}

		if plan.Database != "mysql" {
			t.Errorf("Expected Database = 'mysql', got '%s'", plan.Database)
		}

		if plan.RawOutput == "" {
			t.Error("Expected RawOutput to be populated")
		}
	})

	t.Run("full table scan", func(t *testing.T) {
		query := "SELECT * FROM users"
		args := []interface{}{}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v, FullScan=%v",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex, plan.FullScan)

		if !plan.FullScan {
			t.Error("Expected full table scan for SELECT *")
		}

		if plan.EstimatedRows == 0 {
			t.Error("Expected non-zero estimated rows")
		}

		if plan.Cost == 0 {
			t.Error("Expected non-zero cost estimate")
		}
	})

	t.Run("index scan on status", func(t *testing.T) {
		query := "SELECT * FROM users WHERE status = ?"
		args := []interface{}{1}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v, IndexName=%s",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex, plan.IndexName)

		// MySQL may choose index or full scan depending on statistics
		// We just verify the analysis completed successfully
		if plan.Database != "mysql" {
			t.Errorf("Expected Database = 'mysql', got '%s'", plan.Database)
		}
	})

	t.Run("range scan", func(t *testing.T) {
		query := "SELECT * FROM users WHERE id > ? AND id < ?"
		args := []interface{}{1, 100}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v, FullScan=%v",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex, plan.FullScan)

		// Range scan typically uses PRIMARY index
		if !plan.UsesIndex {
			t.Error("Expected index usage for range query on PRIMARY KEY")
		}
	})

	t.Run("ORDER BY with index", func(t *testing.T) {
		query := "SELECT * FROM users ORDER BY email"
		args := []interface{}{}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v, IndexName=%s",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex, plan.IndexName)

		// MySQL may use email_idx for ordering
		// We just verify analysis works
		if plan.EstimatedRows == 0 {
			t.Error("Expected non-zero estimated rows")
		}
	})
}

func TestMySQLAnalyzer_Integration_ExplainAnalyze(t *testing.T) {
	db, cleanup := setupMySQLTestDB(t)
	defer cleanup()

	analyzer := NewMySQLAnalyzer(db)
	ctx := context.Background()

	// Check MySQL version (EXPLAIN ANALYZE requires 8.0.18+)
	var version string
	err := db.QueryRow("SELECT VERSION()").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to get MySQL version: %v", err)
	}
	t.Logf("MySQL version: %s", version)

	t.Run("explain analyze - may not be supported", func(t *testing.T) {
		query := "SELECT * FROM users WHERE email = ?"
		args := []interface{}{"alice@example.com"}

		plan, err := analyzer.ExplainAnalyze(ctx, query, args)

		// EXPLAIN ANALYZE may not be supported in older MySQL versions
		if err != nil {
			t.Logf("EXPLAIN ANALYZE not supported (expected for MySQL < 8.0.18): %v", err)
			t.Skip("Skipping EXPLAIN ANALYZE test")
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, ActualRows=%d, ActualTime=%v",
			plan.Cost, plan.EstimatedRows, plan.ActualRows, plan.ActualTime)

		if plan.Database != "mysql" {
			t.Errorf("Expected Database = 'mysql', got '%s'", plan.Database)
		}

		// EXPLAIN ANALYZE executes the query
		// Verify data wasn't modified (SELECT is safe)
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count rows: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3 rows after EXPLAIN ANALYZE, got %d", count)
		}
	})
}

func TestMySQLAnalyzer_Integration_ComplexQueries(t *testing.T) {
	db, cleanup := setupMySQLTestDB(t)
	defer cleanup()

	// Create orders table for JOIN tests
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS orders (
			id INT PRIMARY KEY AUTO_INCREMENT,
			user_id INT NOT NULL,
			total DECIMAL(10,2) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX user_id_idx (user_id)
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create orders table: %v", err)
	}
	defer db.Exec("DROP TABLE IF EXISTS orders")

	// Insert test orders
	_, err = db.Exec(`
		INSERT INTO orders (user_id, total) VALUES
		(1, 100.00),
		(1, 200.00),
		(2, 150.00)
	`)
	if err != nil {
		t.Fatalf("Failed to insert orders: %v", err)
	}

	analyzer := NewMySQLAnalyzer(db)
	ctx := context.Background()

	t.Run("JOIN with indexes", func(t *testing.T) {
		query := "SELECT u.*, o.total FROM users u INNER JOIN orders o ON u.id = o.user_id WHERE u.status = ?"
		args := []interface{}{1}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v, IndexName=%s",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex, plan.IndexName)

		// JOIN should use indexes
		if !plan.UsesIndex {
			t.Error("Expected index usage for JOIN query")
		}

		if plan.EstimatedRows == 0 {
			t.Error("Expected non-zero estimated rows for JOIN")
		}
	})

	t.Run("GROUP BY with index", func(t *testing.T) {
		query := "SELECT user_id, COUNT(*), SUM(total) FROM orders GROUP BY user_id"
		args := []interface{}{}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex)

		// GROUP BY may use index for optimization
		if plan.EstimatedRows == 0 {
			t.Error("Expected non-zero estimated rows")
		}
	})

	t.Run("subquery", func(t *testing.T) {
		query := "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders WHERE total > ?)"
		args := []interface{}{100.00}

		plan, err := analyzer.Explain(ctx, query, args)
		if err != nil {
			t.Fatalf("Explain failed: %v", err)
		}

		t.Logf("Plan: Cost=%.2f, Rows=%d, Index=%v",
			plan.Cost, plan.EstimatedRows, plan.UsesIndex)

		// Subquery analysis should complete successfully
		if plan.Database != "mysql" {
			t.Errorf("Expected Database = 'mysql', got '%s'", plan.Database)
		}
	})
}

func TestMySQLAnalyzer_Integration_ErrorCases(t *testing.T) {
	db, cleanup := setupMySQLTestDB(t)
	defer cleanup()

	analyzer := NewMySQLAnalyzer(db)
	ctx := context.Background()

	t.Run("invalid query", func(t *testing.T) {
		query := "SELECT * FROM nonexistent_table"
		args := []interface{}{}

		_, err := analyzer.Explain(ctx, query, args)
		if err == nil {
			t.Error("Expected error for invalid query")
		}
	})

	t.Run("syntax error", func(t *testing.T) {
		query := "SELECT FROM WHERE"
		args := []interface{}{}

		_, err := analyzer.Explain(ctx, query, args)
		if err == nil {
			t.Error("Expected error for syntax error")
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		query := "SELECT * FROM users"
		args := []interface{}{}

		_, err := analyzer.Explain(ctx, query, args)
		if err == nil {
			t.Error("Expected error for cancelled context")
		}
	})
}

// TestMySQLAnalyzer_Integration_Docker verifies test setup instructions.
func TestMySQLAnalyzer_Integration_Docker(t *testing.T) {
	t.Log("To run MySQL integration tests, start MySQL container:")
	t.Log("  docker run -d --name relica-mysql-test \\")
	t.Log("    -e MYSQL_ROOT_PASSWORD=testpass \\")
	t.Log("    -e MYSQL_DATABASE=testdb \\")
	t.Log("    -p 3306:3306 \\")
	t.Log("    mysql:8.0")
	t.Log("")
	t.Log("Or set MYSQL_TEST_DSN environment variable:")
	t.Log("  export MYSQL_TEST_DSN='root:password@tcp(host:3306)/database?parseTime=true'")
	t.Log("")

	// This test always passes - it's just documentation
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Log("Using default DSN: root:testpass@tcp(localhost:3306)/testdb")
	} else {
		t.Logf("Using custom DSN: %s", maskPassword(dsn))
	}
}

// maskPassword masks the password in DSN for logging.
func maskPassword(dsn string) string {
	// Simple masking: root:password@... → root:****@...
	// This is just for test logging, not production security
	return fmt.Sprintf("%s:****@...", "root")
}
