package core

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TestTransactional_Commit tests successful commit scenario.
func TestTransactional_Commit(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create test table.
	_, err := db.sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Execute transaction.
	err = db.Transactional(ctx, func(tx *Tx) error {
		_, err := tx.builder.Insert("users", map[string]interface{}{
			"id":   1,
			"name": "Alice",
		}).Execute()
		return err
	})

	if err != nil {
		t.Fatalf("Transactional failed: %v", err)
	}

	// Verify data was committed.
	var name string
	err = db.sqlDB.QueryRow("SELECT name FROM users WHERE id = 1").Scan(&name)
	if err != nil {
		t.Fatalf("Failed to verify commit: %v", err)
	}
	if name != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", name)
	}
}

// TestTransactional_Rollback tests rollback on error.
func TestTransactional_Rollback(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create test table.
	_, err := db.sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Execute transaction that returns an error.
	testErr := errors.New("test error")
	err = db.Transactional(ctx, func(tx *Tx) error {
		_, err := tx.builder.Insert("users", map[string]interface{}{
			"id":   1,
			"name": "Alice",
		}).Execute()
		if err != nil {
			return err
		}
		return testErr // Force rollback
	})

	if !errors.Is(err, testErr) {
		t.Fatalf("Expected error '%v', got '%v'", testErr, err)
	}

	// Verify data was rolled back.
	var count int
	err = db.sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rows, got %d (rollback failed)", count)
	}
}

// TestTransactional_Panic tests rollback on panic.
func TestTransactional_Panic(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create test table.
	_, err := db.sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Execute transaction that panics.
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic to be re-raised")
		}
	}()

	_ = db.Transactional(ctx, func(tx *Tx) error {
		_, err := tx.builder.Insert("users", map[string]interface{}{
			"id":   1,
			"name": "Alice",
		}).Execute()
		if err != nil {
			return err
		}
		panic("test panic") // Force rollback via panic
	})

	t.Error("Should not reach here")
}

// TestTransactional_PanicRollback verifies rollback happens on panic.
func TestTransactional_PanicRollback(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create test table.
	_, err := db.sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Execute transaction that panics.
	func() {
		defer func() {
			recover() // Catch the panic
		}()

		_ = db.Transactional(ctx, func(tx *Tx) error {
			_, err := tx.builder.Insert("users", map[string]interface{}{
				"id":   1,
				"name": "Alice",
			}).Execute()
			if err != nil {
				return err
			}
			panic("test panic")
		})
	}()

	// Verify data was rolled back.
	var count int
	err = db.sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rows after panic, got %d (rollback failed)", count)
	}
}

// TestTransactional_ContextCancellation tests context cancellation.
func TestTransactional_ContextCancellation(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	// Create canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute transaction with canceled context.
	err := db.Transactional(ctx, func(tx *Tx) error {
		return nil
	})

	// Should fail to begin transaction with canceled context.
	if err == nil {
		t.Error("Expected error with canceled context, got nil")
	}
}

// TestTransactional_MultipleOperations tests multiple operations in one transaction.
func TestTransactional_MultipleOperations(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create test tables.
	_, err := db.sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create users table: %v", err)
	}
	_, err = db.sqlDB.Exec("CREATE TABLE accounts (id INTEGER PRIMARY KEY, user_id INTEGER, balance INTEGER)")
	if err != nil {
		t.Fatalf("Failed to create accounts table: %v", err)
	}

	// Execute transaction with multiple operations.
	err = db.Transactional(ctx, func(tx *Tx) error {
		// Insert user.
		result, err := tx.builder.Insert("users", map[string]interface{}{
			"id":   1,
			"name": "Alice",
		}).Execute()
		if err != nil {
			return err
		}

		// Verify insert succeeded.
		rows, _ := result.RowsAffected()
		if rows != 1 {
			return errors.New("failed to insert user")
		}

		// Insert account.
		_, err = tx.builder.Insert("accounts", map[string]interface{}{
			"id":      1,
			"user_id": 1,
			"balance": 100,
		}).Execute()
		return err
	})

	if err != nil {
		t.Fatalf("Transactional failed: %v", err)
	}

	// Verify both operations were committed.
	var userName string
	err = db.sqlDB.QueryRow("SELECT name FROM users WHERE id = 1").Scan(&userName)
	if err != nil {
		t.Fatalf("Failed to verify user: %v", err)
	}
	if userName != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", userName)
	}

	var balance int
	err = db.sqlDB.QueryRow("SELECT balance FROM accounts WHERE user_id = 1").Scan(&balance)
	if err != nil {
		t.Fatalf("Failed to verify account: %v", err)
	}
	if balance != 100 {
		t.Errorf("Expected balance 100, got %d", balance)
	}
}

// TestTransactionalTx_WithOptions tests transaction with custom options.
func TestTransactionalTx_WithOptions(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create test table.
	_, err := db.sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Execute transaction with custom options.
	opts := &TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	}

	err = db.TransactionalTx(ctx, opts, func(tx *Tx) error {
		_, err := tx.builder.Insert("users", map[string]interface{}{
			"id":   1,
			"name": "Alice",
		}).Execute()
		return err
	})

	if err != nil {
		t.Fatalf("TransactionalTx failed: %v", err)
	}

	// Verify data was committed.
	var name string
	err = db.sqlDB.QueryRow("SELECT name FROM users WHERE id = 1").Scan(&name)
	if err != nil {
		t.Fatalf("Failed to verify commit: %v", err)
	}
	if name != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", name)
	}
}

// TestTransactionalTx_Rollback tests rollback with custom options.
func TestTransactionalTx_Rollback(t *testing.T) {
	db := setupReplicaDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create test table.
	_, err := db.sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Execute transaction with custom options that returns error.
	opts := &TxOptions{
		Isolation: sql.LevelReadCommitted,
	}

	testErr := errors.New("test error")
	err = db.TransactionalTx(ctx, opts, func(tx *Tx) error {
		_, err := tx.builder.Insert("users", map[string]interface{}{
			"id":   1,
			"name": "Alice",
		}).Execute()
		if err != nil {
			return err
		}
		return testErr
	})

	if !errors.Is(err, testErr) {
		t.Fatalf("Expected error '%v', got '%v'", testErr, err)
	}

	// Verify data was rolled back.
	var count int
	err = db.sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rows, got %d (rollback failed)", count)
	}
}

// TestTransactional_BeginError tests error handling when Begin fails.
func TestTransactional_BeginError(t *testing.T) {
	db := setupReplicaDB(t)
	db.Close() // Close DB to force Begin error

	ctx := context.Background()

	// Execute transaction on closed DB.
	err := db.Transactional(ctx, func(tx *Tx) error {
		return nil
	})

	// Should fail to begin transaction.
	if err == nil {
		t.Error("Expected error when beginning transaction on closed DB, got nil")
	}
}

// TestTransactional_CommitError tests error handling when Commit fails.
func TestTransactional_CommitError(t *testing.T) {
	// This test is database-specific and may not be portable.
	// Skipping for now as it requires specific setup to force commit failure.
	t.Skip("Commit error testing requires specific database state manipulation")
}

// setupReplicaDB creates a Relica DB instance for testing.
func setupReplicaDB(t *testing.T) *DB {
	t.Helper()

	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}

	return db
}
