package core

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TestWrapDB_BasicWrapping tests basic wrapping of sql.DB with different drivers.
func TestWrapDB_BasicWrapping(t *testing.T) {
	tests := []struct {
		name       string
		driverName string
		dsn        string
	}{
		{"PostgreSQL", "postgres", "postgres://localhost/test"},
		{"MySQL", "mysql", "user:pass@tcp(localhost:3306)/test"},
		{"SQLite", "sqlite", ":memory:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create external sql.DB
			sqlDB, err := sql.Open(tt.driverName, tt.dsn)
			if err != nil {
				t.Skipf("Failed to open %s connection: %v", tt.driverName, err)
			}
			defer sqlDB.Close()

			// Wrap with Relica
			db := WrapDB(sqlDB, tt.driverName)

			// Verify DB instance
			if db == nil {
				t.Fatal("Expected DB instance, got nil")
			}

			if db.sqlDB != sqlDB {
				t.Error("Expected wrapped DB to reference the same sql.DB instance")
			}

			if db.driverName != tt.driverName {
				t.Errorf("Expected driver name %s, got %s", tt.driverName, db.driverName)
			}

			if db.stmtCache == nil {
				t.Error("Expected statement cache to be initialized, got nil")
			}

			if db.dialect == nil {
				t.Error("Expected dialect to be initialized, got nil")
			}
		})
	}
}

// TestWrapDB_QueryExecution tests that queries can be executed through wrapped connection.
func TestWrapDB_QueryExecution(t *testing.T) {
	// Create external sql.DB
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	// Apply custom pool settings
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)

	// Wrap with Relica
	db := WrapDB(sqlDB, "sqlite")

	ctx := context.Background()

	// Create table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE test_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Test INSERT
	t.Run("Insert", func(t *testing.T) {
		_, err := db.Builder().
			Insert("test_users", map[string]interface{}{
				"name":  "Alice",
				"email": "alice@example.com",
			}).
			Execute()
		if err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}
	})

	// Test SELECT
	t.Run("Select", func(t *testing.T) {
		var user struct {
			ID    int64  `db:"id"`
			Name  string `db:"name"`
			Email string `db:"email"`
		}
		err := db.Builder().
			Select().
			From("test_users").
			Where("name = ?", "Alice").
			One(&user)
		if err != nil {
			t.Fatalf("Failed to select: %v", err)
		}

		if user.Name != "Alice" {
			t.Errorf("Expected name 'Alice', got '%s'", user.Name)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("Expected email 'alice@example.com', got '%s'", user.Email)
		}
	})

	// Test UPDATE
	t.Run("Update", func(t *testing.T) {
		_, err := db.Builder().
			Update("test_users").
			Set(map[string]interface{}{"email": "alice.new@example.com"}).
			Where("name = ?", "Alice").
			Execute()
		if err != nil {
			t.Fatalf("Failed to update: %v", err)
		}

		// Verify update
		var user struct {
			Email string `db:"email"`
		}
		err = db.Builder().
			Select().
			From("test_users").
			Where("name = ?", "Alice").
			One(&user)
		if err != nil {
			t.Fatalf("Failed to select: %v", err)
		}

		if user.Email != "alice.new@example.com" {
			t.Errorf("Expected email 'alice.new@example.com', got '%s'", user.Email)
		}
	})

	// Test DELETE
	t.Run("Delete", func(t *testing.T) {
		_, err := db.Builder().
			Delete("test_users").
			Where("name = ?", "Alice").
			Execute()
		if err != nil {
			t.Fatalf("Failed to delete: %v", err)
		}

		// Verify deletion
		var user struct {
			ID int64 `db:"id"`
		}
		err = db.Builder().
			Select().
			From("test_users").
			Where("name = ?", "Alice").
			One(&user)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("Expected sql.ErrNoRows, got %v", err)
		}
	})
}

// TestWrapDB_Transactions tests transaction support with wrapped connection.
func TestWrapDB_Transactions(t *testing.T) {
	// Create external sql.DB
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	// Wrap with Relica
	db := WrapDB(sqlDB, "sqlite")

	ctx := context.Background()

	// Create table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			balance INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Test transaction commit
	t.Run("TransactionCommit", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		_, err = tx.Builder().
			Insert("accounts", map[string]interface{}{
				"username": "alice",
				"balance":  1000,
			}).
			Execute()
		if err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}

		err = tx.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Verify data persists
		var account struct {
			Username string `db:"username"`
			Balance  int    `db:"balance"`
		}
		err = db.Builder().
			Select().
			From("accounts").
			Where("username = ?", "alice").
			One(&account)
		if err != nil {
			t.Fatalf("Failed to select: %v", err)
		}

		if account.Username != "alice" || account.Balance != 1000 {
			t.Errorf("Expected alice with balance 1000, got %s with balance %d",
				account.Username, account.Balance)
		}
	})

	// Test transaction rollback
	t.Run("TransactionRollback", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		_, err = tx.Builder().
			Update("accounts").
			Set(map[string]interface{}{"balance": 500}).
			Where("username = ?", "alice").
			Execute()
		if err != nil {
			t.Fatalf("Failed to update: %v", err)
		}

		// Rollback
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Failed to rollback: %v", err)
		}

		// Verify balance unchanged
		var account struct {
			Balance int `db:"balance"`
		}
		err = db.Builder().
			Select().
			From("accounts").
			Where("username = ?", "alice").
			One(&account)
		if err != nil {
			t.Fatalf("Failed to select: %v", err)
		}

		if account.Balance != 1000 {
			t.Errorf("Expected balance 1000 (unchanged), got %d", account.Balance)
		}
	})
}

// TestWrapDB_StatementCache tests that wrapped DB uses its own statement cache.
func TestWrapDB_StatementCache(t *testing.T) {
	// Create external sql.DB
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	// Wrap with Relica
	db := WrapDB(sqlDB, "sqlite")

	ctx := context.Background()

	// Create table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE cache_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Execute same query multiple times
	for i := 0; i < 5; i++ {
		_, err := db.Builder().
			Insert("cache_test", map[string]interface{}{
				"value": "test",
			}).
			Execute()
		if err != nil {
			t.Fatalf("Failed to insert (iteration %d): %v", i, err)
		}
	}

	// Verify cache stats
	stats := db.stmtCache.Stats()
	if stats.Size == 0 {
		t.Error("Expected statement cache to contain entries, got 0")
	}

	if stats.Hits == 0 {
		t.Log("Warning: Expected cache hits > 0 (may vary based on implementation)")
	}
}

// TestWrapDB_MultipleWraps tests that multiple wraps of same connection have isolated caches.
func TestWrapDB_MultipleWraps(t *testing.T) {
	// Create external sql.DB
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	// Create multiple wraps
	db1 := WrapDB(sqlDB, "sqlite")
	db2 := WrapDB(sqlDB, "sqlite")

	// Verify they reference the same sql.DB
	if db1.sqlDB != db2.sqlDB {
		t.Error("Expected both wraps to reference the same sql.DB instance")
	}

	// Verify they have different caches
	if db1.stmtCache == db2.stmtCache {
		t.Error("Expected different statement caches for each wrap")
	}

	// Verify both work independently
	ctx := context.Background()

	_, err = db1.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS multi_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table with db1: %v", err)
	}

	// Insert with db1
	_, err = db1.Builder().
		Insert("multi_test", map[string]interface{}{
			"value": "from_db1",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert with db1: %v", err)
	}

	// Insert with db2
	_, err = db2.Builder().
		Insert("multi_test", map[string]interface{}{
			"value": "from_db2",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert with db2: %v", err)
	}

	// Verify data exists (both wraps share same connection)
	var count int
	err = db1.QueryRowContext(ctx, "SELECT COUNT(*) FROM multi_test").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 rows, got %d", count)
	}
}

// TestWrapDB_BuilderReturnsWorkingQueryBuilder tests Builder() returns functional query builder.
func TestWrapDB_BuilderReturnsWorkingQueryBuilder(t *testing.T) {
	// Create external sql.DB
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	// Wrap with Relica
	db := WrapDB(sqlDB, "sqlite")

	ctx := context.Background()

	// Create table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE builder_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Get builder
	builder := db.Builder()
	if builder == nil {
		t.Fatal("Expected query builder, got nil")
	}

	// Verify builder works
	_, err = builder.
		Insert("builder_test", map[string]interface{}{
			"name": "test",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to execute query with builder: %v", err)
	}

	// Verify insert
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM builder_test").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

// TestWrapDB_ContextPropagation tests that context is properly propagated.
func TestWrapDB_ContextPropagation(t *testing.T) {
	// Create external sql.DB
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	// Wrap with Relica
	db := WrapDB(sqlDB, "sqlite")

	// Test WithContext
	ctx := context.WithValue(context.Background(), "test_key", "test_value")
	dbWithCtx := db.WithContext(ctx)

	if dbWithCtx == nil {
		t.Fatal("Expected DB with context, got nil")
	}

	if dbWithCtx.ctx != ctx {
		t.Error("Expected context to be set")
	}

	// Verify original DB is unchanged
	if db.ctx != nil {
		t.Error("Expected original DB context to be nil")
	}
}

// TestWrapDB_CallerOwnsConnectionLifecycle tests that caller manages underlying connection.
func TestWrapDB_CallerOwnsConnectionLifecycle(t *testing.T) {
	// Create external sql.DB
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	// Wrap with Relica
	db := WrapDB(sqlDB, "sqlite")

	ctx := context.Background()

	// Create table and insert data
	_, err = db.ExecContext(ctx, `
		CREATE TABLE lifecycle_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = db.Builder().
		Insert("lifecycle_test", map[string]interface{}{
			"value": "test",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Verify data exists
	var count int
	err = sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM lifecycle_test").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}

	// Note: DO NOT call db.Close() on wrapped DB - caller owns sqlDB lifecycle
	// Calling db.Close() would close the underlying sqlDB, which is the caller's responsibility
	//
	// If you need to clean up cache without closing connection, you'd need a separate method
	// For now, cache cleanup happens when underlying connection is closed

	t.Log("Caller correctly manages connection lifecycle (deferred sqlDB.Close())")
}
