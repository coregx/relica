package core

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TestTransactionIntegration_CRUD tests full CRUD operations within transactions.
func TestTransactionIntegration_CRUD(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Setup: Create table
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			balance INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create accounts table: %v", err)
	}

	// Test 1: INSERT within transaction and verify data exists
	t.Run("InsertInTransaction", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		// Insert data
		_, err = tx.Builder().
			Insert("accounts", map[string]interface{}{
				"username": "alice",
				"balance":  1000,
			}).
			Execute()
		if err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}

		// Verify data exists in transaction
		var account struct {
			ID       int64  `db:"id"`
			Username string `db:"username"`
			Balance  int    `db:"balance"`
		}
		err = tx.Builder().
			Select().
			From("accounts").
			Where("username = ?", "alice").
			One(&account)
		if err != nil {
			t.Fatalf("Failed to select within transaction: %v", err)
		}

		if account.Username != "alice" || account.Balance != 1000 {
			t.Errorf("Expected alice with balance 1000, got %s with balance %d",
				account.Username, account.Balance)
		}

		// Rollback and verify data doesn't exist
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Failed to rollback: %v", err)
		}

		// Verify data doesn't exist after rollback
		err = db.Builder().
			Select().
			From("accounts").
			Where("username = ?", "alice").
			One(&account)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("Expected sql.ErrNoRows after rollback, got %v", err)
		}
	})

	// Test 2: INSERT + Commit and verify data persists
	t.Run("InsertAndCommit", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		// Insert data
		_, err = tx.Builder().
			Insert("accounts", map[string]interface{}{
				"username": "bob",
				"balance":  2000,
			}).
			Execute()
		if err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}

		// Commit transaction
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Verify data persists after commit
		var account struct {
			ID       int64  `db:"id"`
			Username string `db:"username"`
			Balance  int    `db:"balance"`
		}
		err = db.Builder().
			Select().
			From("accounts").
			Where("username = ?", "bob").
			One(&account)
		if err != nil {
			t.Fatalf("Failed to select after commit: %v", err)
		}

		if account.Username != "bob" || account.Balance != 2000 {
			t.Errorf("Expected bob with balance 2000, got %s with balance %d",
				account.Username, account.Balance)
		}
	})

	// Test 3: UPDATE within transaction
	t.Run("UpdateInTransaction", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		// Update bob's balance
		_, err = tx.Builder().
			Update("accounts").
			Set(map[string]interface{}{"balance": 2500}).
			Where("username = ?", "bob").
			Execute()
		if err != nil {
			t.Fatalf("Failed to update: %v", err)
		}

		// Verify update within transaction
		var account struct {
			ID       int64  `db:"id"`
			Username string `db:"username"`
			Balance  int    `db:"balance"`
		}
		err = tx.Builder().
			Select().
			From("accounts").
			Where("username = ?", "bob").
			One(&account)
		if err != nil {
			t.Fatalf("Failed to select: %v", err)
		}

		if account.Balance != 2500 {
			t.Errorf("Expected balance 2500, got %d", account.Balance)
		}

		// Commit
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	})

	// Test 4: DELETE within transaction
	t.Run("DeleteInTransaction", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		// Delete bob
		_, err = tx.Builder().
			Delete("accounts").
			Where("username = ?", "bob").
			Execute()
		if err != nil {
			t.Fatalf("Failed to delete: %v", err)
		}

		// Verify deletion within transaction
		var account struct {
			ID       int64  `db:"id"`
			Username string `db:"username"`
			Balance  int    `db:"balance"`
		}
		err = tx.Builder().
			Select().
			From("accounts").
			Where("username = ?", "bob").
			One(&account)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("Expected sql.ErrNoRows, got %v", err)
		}

		// Commit
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	})
}

// TestTransactionIntegration_Atomicity tests that multiple operations are atomic.
func TestTransactionIntegration_Atomicity(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Setup: Create accounts table
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			balance INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create accounts table: %v", err)
	}

	// Insert initial accounts
	_, err = db.Builder().
		Insert("accounts", map[string]interface{}{
			"username": "alice",
			"balance":  1000,
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert alice: %v", err)
	}

	_, err = db.Builder().
		Insert("accounts", map[string]interface{}{
			"username": "bob",
			"balance":  500,
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert bob: %v", err)
	}

	// Test: Transfer money from alice to bob
	// This should be atomic: either both updates succeed or both fail
	t.Run("AtomicTransfer_Success", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		// Deduct from alice
		_, err = tx.Builder().
			Update("accounts").
			Set(map[string]interface{}{"balance": 700}).
			Where("username = ?", "alice").
			Execute()
		if err != nil {
			t.Fatalf("Failed to deduct from alice: %v", err)
		}

		// Add to bob
		_, err = tx.Builder().
			Update("accounts").
			Set(map[string]interface{}{"balance": 800}).
			Where("username = ?", "bob").
			Execute()
		if err != nil {
			t.Fatalf("Failed to add to bob: %v", err)
		}

		// Commit transaction
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Verify balances
		var aliceAccount, bobAccount struct {
			Balance int `db:"balance"`
		}

		err = db.Builder().
			Select().
			From("accounts").
			Where("username = ?", "alice").
			One(&aliceAccount)
		if err != nil {
			t.Fatalf("Failed to select alice: %v", err)
		}

		err = db.Builder().
			Select().
			From("accounts").
			Where("username = ?", "bob").
			One(&bobAccount)
		if err != nil {
			t.Fatalf("Failed to select bob: %v", err)
		}

		if aliceAccount.Balance != 700 {
			t.Errorf("Expected alice balance 700, got %d", aliceAccount.Balance)
		}
		if bobAccount.Balance != 800 {
			t.Errorf("Expected bob balance 800, got %d", bobAccount.Balance)
		}
	})

	// Test: Rollback on error
	t.Run("AtomicTransfer_Rollback", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		// Deduct from alice
		_, err = tx.Builder().
			Update("accounts").
			Set(map[string]interface{}{"balance": 400}).
			Where("username = ?", "alice").
			Execute()
		if err != nil {
			t.Fatalf("Failed to deduct from alice: %v", err)
		}

		// Simulate error by rolling back
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Failed to rollback: %v", err)
		}

		// Verify alice's balance hasn't changed
		var aliceAccount struct {
			Balance int `db:"balance"`
		}
		err = db.Builder().
			Select().
			From("accounts").
			Where("username = ?", "alice").
			One(&aliceAccount)
		if err != nil {
			t.Fatalf("Failed to select alice: %v", err)
		}

		if aliceAccount.Balance != 700 {
			t.Errorf("Expected alice balance 700 (unchanged), got %d", aliceAccount.Balance)
		}
	})
}

// TestTransactionIntegration_Isolation tests transaction isolation and rollback visibility.
func TestTransactionIntegration_Isolation(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Setup: Create table
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS isolation_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert initial value
	_, err = db.Builder().
		Insert("isolation_test", map[string]interface{}{
			"value": 100,
		}).
		Execute()
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Test: Rollback makes changes invisible
	t.Run("RollbackMakesChangesInvisible", func(t *testing.T) {
		// Start transaction
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		// Update value in transaction
		_, err = tx.Builder().
			Update("isolation_test").
			Set(map[string]interface{}{"value": 200}).
			Where("id = ?", 1).
			Execute()
		if err != nil {
			t.Fatalf("Failed to update: %v", err)
		}

		// Verify update within transaction
		var resultInTx struct {
			Value int `db:"value"`
		}
		err = tx.Builder().
			Select().
			From("isolation_test").
			Where("id = ?", 1).
			One(&resultInTx)
		if err != nil {
			t.Fatalf("Failed to select in tx: %v", err)
		}

		if resultInTx.Value != 200 {
			t.Errorf("Expected value 200 in transaction, got %d", resultInTx.Value)
		}

		// Rollback transaction
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Failed to rollback: %v", err)
		}

		// Verify original value is still there after rollback
		var resultAfterRollback struct {
			Value int `db:"value"`
		}
		err = db.Builder().
			Select().
			From("isolation_test").
			Where("id = ?", 1).
			One(&resultAfterRollback)
		if err != nil {
			t.Fatalf("Failed to select after rollback: %v", err)
		}

		if resultAfterRollback.Value != 100 {
			t.Errorf("Expected value 100 after rollback, got %d", resultAfterRollback.Value)
		}

		t.Logf("Successfully verified: transaction update (200) was rolled back to original value (100)")
	})
}

// TestTransactionIntegration_MultipleQueries tests multiple queries in a transaction.
func TestTransactionIntegration_MultipleQueries(t *testing.T) {
	db := setupTransactionTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Setup: Create table
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS multi_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			status TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Test: Execute multiple queries in one transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Insert multiple records
	for i := 1; i <= 5; i++ {
		_, err = tx.Builder().
			Insert("multi_test", map[string]interface{}{
				"name":   "item" + string(rune('0'+i)),
				"status": "active",
			}).
			Execute()
		if err != nil {
			t.Fatalf("Failed to insert item %d: %v", i, err)
		}
	}

	// Update some records
	_, err = tx.Builder().
		Update("multi_test").
		Set(map[string]interface{}{"status": "inactive"}).
		Where("id <= ?", 2).
		Execute()
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	// Delete some records
	_, err = tx.Builder().
		Delete("multi_test").
		Where("id = ?", 5).
		Execute()
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify results
	var items []struct {
		ID     int64  `db:"id"`
		Name   string `db:"name"`
		Status string `db:"status"`
	}
	err = tx.Builder().
		Select().
		From("multi_test").
		All(&items)
	if err != nil {
		t.Fatalf("Failed to select: %v", err)
	}

	if len(items) != 4 {
		t.Errorf("Expected 4 items, got %d", len(items))
	}

	// Count inactive items
	inactiveCount := 0
	for _, item := range items {
		if item.Status == "inactive" {
			inactiveCount++
		}
	}
	if inactiveCount != 2 {
		t.Errorf("Expected 2 inactive items, got %d", inactiveCount)
	}

	// Commit
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Verify data persists
	var persistedItems []struct {
		ID     int64  `db:"id"`
		Name   string `db:"name"`
		Status string `db:"status"`
	}
	err = db.Builder().
		Select().
		From("multi_test").
		All(&persistedItems)
	if err != nil {
		t.Fatalf("Failed to select after commit: %v", err)
	}

	if len(persistedItems) != 4 {
		t.Errorf("Expected 4 items after commit, got %d", len(persistedItems))
	}
}
