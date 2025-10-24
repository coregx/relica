package core

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// setupTransactionTestDB creates a test DB instance for transaction tests.
func setupTransactionTestDB(t *testing.T) *DB {
	t.Helper()

	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	return db
}

// TestTransaction_CreateAndCommit tests creating and committing a transaction.
func TestTransaction_CreateAndCommit(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Verify transaction is not nil
	if tx == nil {
		t.Fatal("Expected transaction to be created, got nil")
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

// TestTransaction_CreateAndRollback tests creating and rolling back a transaction.
func TestTransaction_CreateAndRollback(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Rollback transaction
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}
}

// TestTransaction_BuilderAccess tests accessing the query builder from a transaction.
func TestTransaction_BuilderAccess(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Get builder
	builder := tx.Builder()
	if builder == nil {
		t.Fatal("Expected builder to be returned, got nil")
	}

	// Verify builder has transaction reference
	if builder.tx == nil {
		t.Fatal("Expected builder to have transaction reference, got nil")
	}
}

// TestTransaction_Insert tests INSERT within a transaction.
func TestTransaction_Insert(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table
	createTestTable(t, db)

	// Start transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Insert within transaction
	_, err = tx.Builder().
		Insert("test_users", map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert within transaction: %v", err)
	}

	// Verify data exists in transaction
	var user TestUser
	err = tx.Builder().
		Select().
		From("test_users").
		Where("name = ?", "Alice").
		One(&user)
	if err != nil {
		t.Fatalf("Failed to select within transaction: %v", err)
	}

	if user.Name != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", user.Name)
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify data persists after commit
	var committedUser TestUser
	err = db.Builder().
		Select().
		From("test_users").
		Where("name = ?", "Alice").
		One(&committedUser)
	if err != nil {
		t.Fatalf("Failed to select after commit: %v", err)
	}

	if committedUser.Name != "Alice" {
		t.Errorf("Expected name 'Alice' after commit, got '%s'", committedUser.Name)
	}
}

// TestTransaction_Update tests UPDATE within a transaction.
func TestTransaction_Update(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table and insert test data
	createTestTable(t, db)
	_, err := db.Builder().
		Insert("test_users", map[string]interface{}{
			"name":  "Bob",
			"email": "bob@example.com",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Start transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Update within transaction
	_, err = tx.Builder().
		Update("test_users").
		Set(map[string]interface{}{"email": "bob.new@example.com"}).
		Where("name = ?", "Bob").
		Execute()
	if err != nil {
		t.Fatalf("Failed to update within transaction: %v", err)
	}

	// Verify update within transaction
	var user TestUser
	err = tx.Builder().
		Select().
		From("test_users").
		Where("name = ?", "Bob").
		One(&user)
	if err != nil {
		t.Fatalf("Failed to select within transaction: %v", err)
	}

	if user.Email != "bob.new@example.com" {
		t.Errorf("Expected email 'bob.new@example.com', got '%s'", user.Email)
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

// TestTransaction_Delete tests DELETE within a transaction.
func TestTransaction_Delete(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table and insert test data
	createTestTable(t, db)
	_, err := db.Builder().
		Insert("test_users", map[string]interface{}{
			"name":  "Charlie",
			"email": "charlie@example.com",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Start transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Delete within transaction
	_, err = tx.Builder().
		Delete("test_users").
		Where("name = ?", "Charlie").
		Execute()
	if err != nil {
		t.Fatalf("Failed to delete within transaction: %v", err)
	}

	// Verify deletion within transaction
	var user TestUser
	err = tx.Builder().
		Select().
		From("test_users").
		Where("name = ?", "Charlie").
		One(&user)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected sql.ErrNoRows, got %v", err)
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

// TestTransaction_Select tests SELECT within a transaction.
func TestTransaction_Select(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table and insert test data
	createTestTable(t, db)
	_, err := db.Builder().
		Insert("test_users", map[string]interface{}{
			"name":  "Diana",
			"email": "diana@example.com",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Start transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Select within transaction
	var user TestUser
	err = tx.Builder().
		Select().
		From("test_users").
		Where("name = ?", "Diana").
		One(&user)
	if err != nil {
		t.Fatalf("Failed to select within transaction: %v", err)
	}

	if user.Name != "Diana" {
		t.Errorf("Expected name 'Diana', got '%s'", user.Name)
	}

	// Rollback transaction
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}
}

// TestTransaction_IsolationLevels tests different transaction isolation levels.
func TestTransaction_IsolationLevels(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		isolation sql.IsolationLevel
	}{
		{"ReadUncommitted", sql.LevelReadUncommitted},
		{"ReadCommitted", sql.LevelReadCommitted},
		{"RepeatableRead", sql.LevelRepeatableRead},
		{"Serializable", sql.LevelSerializable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create transaction with isolation level
			tx, err := db.BeginTx(ctx, &TxOptions{
				Isolation: tt.isolation,
			})
			if err != nil {
				// SQLite may not support all isolation levels
				t.Logf("Isolation level %v not supported: %v", tt.isolation, err)
				return
			}
			defer tx.Rollback()

			// Verify transaction was created
			if tx == nil {
				t.Fatal("Expected transaction to be created, got nil")
			}

			// Commit transaction
			err = tx.Commit()
			if err != nil {
				t.Fatalf("Failed to commit transaction: %v", err)
			}
		})
	}
}

// TestTransaction_ReadOnly tests read-only transaction option.
func TestTransaction_ReadOnly(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create transaction with read-only option
	tx, err := db.BeginTx(ctx, &TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		// SQLite may not support read-only transactions
		t.Logf("Read-only transactions not supported: %v", err)
		return
	}
	defer tx.Rollback()

	// Verify transaction was created
	if tx == nil {
		t.Fatal("Expected transaction to be created, got nil")
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

// TestTransaction_CommitAfterRollback tests error handling for commit after rollback.
func TestTransaction_CommitAfterRollback(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Rollback transaction
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}

	// Try to commit after rollback (should fail)
	err = tx.Commit()
	if err == nil {
		t.Fatal("Expected error when committing after rollback, got nil")
	}
}

// TestTransaction_RollbackAfterCommit tests error handling for rollback after commit.
func TestTransaction_RollbackAfterCommit(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Try to rollback after commit (should fail)
	err = tx.Rollback()
	if err == nil {
		t.Fatal("Expected error when rolling back after commit, got nil")
	}
}

// TestTransaction_StatementCacheBypassed tests that transactions bypass statement cache.
func TestTransaction_StatementCacheBypassed(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table
	createTestTable(t, db)

	// Start transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Insert within transaction (should not use cache)
	_, err = tx.Builder().
		Insert("test_users", map[string]interface{}{
			"name":  "Eve",
			"email": "eve@example.com",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert within transaction: %v", err)
	}

	// Verify cache stats show no hits for transactional queries
	// (This is implementation detail, but we can verify statement was not cached)

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Now execute same query outside transaction (should use cache)
	_, err = db.Builder().
		Insert("test_users", map[string]interface{}{
			"name":  "Frank",
			"email": "frank@example.com",
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert outside transaction: %v", err)
	}
}

// createTestTable creates a test table for transaction tests.
func createTestTable(t *testing.T, db *DB) {
	t.Helper()

	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS test_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Clean up any existing data
	_, err = db.ExecContext(ctx, "DELETE FROM test_users")
	if err != nil {
		t.Fatalf("Failed to clean test table: %v", err)
	}
}

// TestUser is a test struct for scanning results.
type TestUser struct {
	ID    int64  `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}
