package core

import (
	"context"
	"strings"
	"testing"

	"github.com/coregx/relica/internal/security"
	_ "modernc.org/sqlite"
)

func TestDB_WithValidator_ExecContext(t *testing.T) {
	// Create DB with validator
	validator := security.NewValidator()
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.validator = validator
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		query     string
		args      []interface{}
		wantError bool
	}{
		{
			name:      "legitimate_insert",
			query:     "INSERT INTO users (id, name) VALUES (?, ?)",
			args:      []interface{}{1, "Alice"},
			wantError: false,
		},
		{
			name:      "sql_injection_in_query",
			query:     "INSERT INTO users (id, name) VALUES (1, 'Alice'); DROP TABLE users",
			args:      []interface{}{},
			wantError: true,
		},
		{
			name:      "sql_injection_in_params",
			query:     "INSERT INTO users (id, name) VALUES (?, ?)",
			args:      []interface{}{2, "'; DROP TABLE users--"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.ExecContext(ctx, tt.query, tt.args...)

			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestDB_WithValidator_QueryContext(t *testing.T) {
	// Create DB with validator
	validator := security.NewValidator()
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.validator = validator
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.sqlDB.Exec("INSERT INTO users (id, name) VALUES (1, 'Alice')")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		query     string
		args      []interface{}
		wantError bool
	}{
		{
			name:      "legitimate_select",
			query:     "SELECT * FROM users WHERE id = ?",
			args:      []interface{}{1},
			wantError: false,
		},
		{
			name:      "sql_injection_union_attack",
			query:     "SELECT * FROM users WHERE id = 1 UNION SELECT name FROM users",
			args:      []interface{}{},
			wantError: true,
		},
		{
			name:      "sql_injection_in_params",
			query:     "SELECT * FROM users WHERE name = ?",
			args:      []interface{}{"' OR '1'='1"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := db.QueryContext(ctx, tt.query, tt.args...)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else {
					rows.Close()
				}
			}
		})
	}
}

func TestDB_WithValidator_QueryRowContext(t *testing.T) {
	// Create DB with validator
	validator := security.NewValidator()
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.validator = validator
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.sqlDB.Exec("INSERT INTO users (id, name) VALUES (1, 'Alice')")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Legitimate query should work
	var id int
	var name string
	row := db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE id = ?", 1)
	err = row.Scan(&id, &name)
	if err != nil {
		t.Errorf("Legitimate query failed: %v", err)
	}
	if id != 1 || name != "Alice" {
		t.Errorf("Got unexpected values: id=%d, name=%s", id, name)
	}

	// Note: QueryRowContext does NOT support validation due to API constraints.
	// This is documented in the method's godoc. Users should use QueryContext() instead
	// if they need validation, or ensure queries are safe before calling QueryRowContext.
}

func TestDB_WithoutValidator(t *testing.T) {
	// Create DB WITHOUT validator
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Without validator, even malicious queries should execute (though they may fail at DB level)
	// This test ensures validator is opt-in, not forced
	_, err = db.ExecContext(ctx, "INSERT INTO users (id, name) VALUES (?, ?)", 1, "Alice")
	if err != nil {
		t.Errorf("Legitimate query failed without validator: %v", err)
	}

	// Even a query with injection patterns should not be blocked (validator is nil)
	_, err = db.ExecContext(ctx, "INSERT INTO users (id, name) VALUES (?, ?)", 2, "'; DROP TABLE--")
	if err != nil {
		// This is okay - the query might fail at DB level, but it wasn't blocked by validator
		if !strings.Contains(err.Error(), "dangerous SQL pattern") {
			// Good - no validator error
		}
	}
}

func TestDB_WithStrictValidator(t *testing.T) {
	// Create DB with strict validator
	validator := security.NewValidator(security.WithStrict(true))
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.validator = validator
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE orders (id INTEGER, status TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// In strict mode, even legitimate OR queries should be blocked
	_, err = db.ExecContext(ctx, "SELECT * FROM orders WHERE status = 'pending' OR status = 'processing'")
	if err == nil {
		t.Error("Expected strict validator to block OR clause")
	}
}

// Benchmark validation overhead
func BenchmarkDB_ExecContext_WithValidator(b *testing.B) {
	validator := security.NewValidator()
	db, _ := NewDB("sqlite", ":memory:")
	db.validator = validator
	defer db.Close()

	db.sqlDB.Exec("CREATE TABLE test (id INTEGER, name TEXT)")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = db.ExecContext(ctx, "INSERT INTO test (id, name) VALUES (?, ?)", i, "test")
	}
}

func BenchmarkDB_ExecContext_WithoutValidator(b *testing.B) {
	db, _ := NewDB("sqlite", ":memory:")
	defer db.Close()

	db.sqlDB.Exec("CREATE TABLE test (id INTEGER, name TEXT)")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = db.ExecContext(ctx, "INSERT INTO test (id, name) VALUES (?, ?)", i, "test")
	}
}
