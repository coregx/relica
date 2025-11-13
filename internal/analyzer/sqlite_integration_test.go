//go:build integration

package analyzer

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// TestSQLiteAnalyzer_Explain tests SQLite EXPLAIN QUERY PLAN with real database.
func TestSQLiteAnalyzer_Explain(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Create test table and indexes
	setupSQLiteTestDB(t, db)

	analyzer := NewSQLiteAnalyzer(db)
	ctx := context.Background()

	tests := []struct {
		name          string
		query         string
		args          []interface{}
		wantUsesIndex bool
		wantIndexName string // Optional: specific index name (empty if multiple or don't care)
		wantFullScan  bool
	}{
		{
			name:          "full_table_scan",
			query:         "SELECT * FROM users WHERE status = ?",
			args:          []interface{}{1},
			wantUsesIndex: false,
			wantIndexName: "",
			wantFullScan:  true,
		},
		{
			name:          "index_scan_on_email",
			query:         "SELECT * FROM users WHERE email = ?",
			args:          []interface{}{"test@example.com"},
			wantUsesIndex: true,
			wantIndexName: "idx_email", // SQLite should use this index
			wantFullScan:  false,
		},
		{
			name:          "primary_key_lookup",
			query:         "SELECT * FROM users WHERE id = ?",
			args:          []interface{}{1},
			wantUsesIndex: true,
			wantIndexName: "PRIMARY KEY", // INTEGER PRIMARY KEY
			wantFullScan:  false,
		},
		{
			name:          "covering_index",
			query:         "SELECT email FROM users WHERE email = ?",
			args:          []interface{}{"test@example.com"},
			wantUsesIndex: true,
			// SQLite may use covering index optimization
			wantFullScan: false,
		},
		{
			name:          "join_with_indexes",
			query:         "SELECT u.name, o.total FROM users u INNER JOIN orders o ON u.id = o.user_id WHERE u.email = ?",
			args:          []interface{}{"test@example.com"},
			wantUsesIndex: true,
			// Should use indexes on both tables
			wantFullScan: false,
		},
		{
			name:          "count_with_index",
			query:         "SELECT COUNT(*) FROM users WHERE email = ?",
			args:          []interface{}{"test@example.com"},
			wantUsesIndex: true,
			wantIndexName: "idx_email",
			wantFullScan:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := analyzer.Explain(ctx, tt.query, tt.args)
			if err != nil {
				t.Fatalf("Explain() error = %v", err)
			}

			// Validate database type
			if plan.Database != "sqlite" {
				t.Errorf("Database = %v, want sqlite", plan.Database)
			}

			// Validate index usage
			if plan.UsesIndex != tt.wantUsesIndex {
				t.Errorf("UsesIndex = %v, want %v", plan.UsesIndex, tt.wantUsesIndex)
			}

			// Validate full scan detection
			if plan.FullScan != tt.wantFullScan {
				t.Errorf("FullScan = %v, want %v", plan.FullScan, tt.wantFullScan)
			}

			// Validate specific index name (if provided)
			if tt.wantIndexName != "" && plan.IndexName != tt.wantIndexName {
				t.Errorf("IndexName = %v, want %v", plan.IndexName, tt.wantIndexName)
			}

			// Validate raw output is populated
			if plan.RawOutput == "" {
				t.Error("RawOutput should not be empty")
			}

			// SQLite doesn't provide cost or row estimates
			if plan.Cost != 0 {
				t.Errorf("Cost should be 0 for SQLite, got %v", plan.Cost)
			}
			if plan.EstimatedRows != 0 {
				t.Errorf("EstimatedRows should be 0 for SQLite, got %v", plan.EstimatedRows)
			}

			t.Logf("Query plan: %s", plan.RawOutput)
		})
	}
}

// TestSQLiteAnalyzer_ExplainAnalyze tests that ExplainAnalyze returns error.
func TestSQLiteAnalyzer_ExplainAnalyze(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	analyzer := NewSQLiteAnalyzer(db)
	ctx := context.Background()

	_, err = analyzer.ExplainAnalyze(ctx, "SELECT 1", nil)
	if err == nil {
		t.Error("ExplainAnalyze() should return error for SQLite")
	}

	expectedErrMsg := "EXPLAIN ANALYZE not supported by SQLite"
	if err.Error() != expectedErrMsg {
		t.Errorf("ExplainAnalyze() error = %v, want %v", err.Error(), expectedErrMsg)
	}
}

// TestSQLiteAnalyzer_ComplexQueries tests complex query scenarios.
func TestSQLiteAnalyzer_ComplexQueries(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	setupSQLiteTestDB(t, db)

	analyzer := NewSQLiteAnalyzer(db)
	ctx := context.Background()

	tests := []struct {
		name  string
		query string
		args  []interface{}
	}{
		{
			name:  "subquery",
			query: "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders WHERE total > ?)",
			args:  []interface{}{100},
		},
		{
			name:  "aggregate",
			query: "SELECT email, COUNT(*) FROM users GROUP BY email",
			args:  nil,
		},
		{
			name:  "order_by",
			query: "SELECT * FROM users ORDER BY email",
			args:  nil,
		},
		{
			name:  "limit_offset",
			query: "SELECT * FROM users LIMIT ? OFFSET ?",
			args:  []interface{}{10, 20},
		},
		{
			name:  "left_join",
			query: "SELECT u.*, o.total FROM users u LEFT JOIN orders o ON u.id = o.user_id WHERE u.status = ?",
			args:  []interface{}{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := analyzer.Explain(ctx, tt.query, tt.args)
			if err != nil {
				t.Fatalf("Explain() error = %v", err)
			}

			// Basic validation
			if plan.Database != "sqlite" {
				t.Errorf("Database = %v, want sqlite", plan.Database)
			}

			if plan.RawOutput == "" {
				t.Error("RawOutput should not be empty")
			}

			t.Logf("Query plan: %s", plan.RawOutput)
		})
	}
}

// TestSQLiteAnalyzer_ErrorHandling tests error scenarios.
func TestSQLiteAnalyzer_ErrorHandling(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	analyzer := NewSQLiteAnalyzer(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		query     string
		args      []interface{}
		wantError bool
	}{
		{
			name:      "invalid_sql",
			query:     "SELECT * FROM nonexistent_table",
			args:      nil,
			wantError: true,
		},
		{
			name:      "syntax_error",
			query:     "SELECT * FROMM users",
			args:      nil,
			wantError: true,
		},
		{
			name:      "wrong_arg_count",
			query:     "SELECT * FROM users WHERE id = ?",
			args:      []interface{}{}, // Missing argument
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := analyzer.Explain(ctx, tt.query, tt.args)
			if tt.wantError && err == nil {
				t.Error("Explain() should return error")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Explain() unexpected error = %v", err)
			}
		})
	}
}

// setupSQLiteTestDB creates test tables and indexes for integration tests.
func setupSQLiteTestDB(t *testing.T, db *sql.DB) {
	t.Helper()

	// Create users table
	_, err := db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL,
			name TEXT NOT NULL,
			status INTEGER NOT NULL DEFAULT 1
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create users table: %v", err)
	}

	// Create index on email
	_, err = db.Exec("CREATE INDEX idx_email ON users(email)")
	if err != nil {
		t.Fatalf("Failed to create idx_email: %v", err)
	}

	// Create orders table
	_, err = db.Exec(`
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			total REAL NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create orders table: %v", err)
	}

	// Create index on user_id
	_, err = db.Exec("CREATE INDEX idx_orders_user ON orders(user_id)")
	if err != nil {
		t.Fatalf("Failed to create idx_orders_user: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO users (email, name, status) VALUES
		('alice@example.com', 'Alice', 1),
		('bob@example.com', 'Bob', 1),
		('charlie@example.com', 'Charlie', 2)
	`)
	if err != nil {
		t.Fatalf("Failed to insert test users: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO orders (user_id, total) VALUES
		(1, 100.50),
		(1, 250.00),
		(2, 75.25)
	`)
	if err != nil {
		t.Fatalf("Failed to insert test orders: %v", err)
	}
}
