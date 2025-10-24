package core

import (
	"context"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupContextTestDB creates an in-memory SQLite database for context testing
func setupContextTestDB(t *testing.T) *DB {
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create test table
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE test_users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Insert test data
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO test_users (id, name, email) VALUES
		(1, 'Alice', 'alice@example.com'),
		(2, 'Bob', 'bob@example.com'),
		(3, 'Charlie', 'charlie@example.com')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	return db
}

// TestContextIntegration_QueryWithTimeout tests query with timeout
func TestContextIntegration_QueryWithTimeout(t *testing.T) {
	db := setupContextTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	var users []User
	err := db.Builder().
		WithContext(ctx).
		Select().
		From("test_users").
		All(&users)

	if err != nil {
		t.Errorf("Query with long timeout should succeed: %v", err)
	}

	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}
}

// TestContextIntegration_QueryLevelContext tests query-level context override
func TestContextIntegration_QueryLevelContext(t *testing.T) {
	db := setupContextTestDB(t)
	defer db.Close()

	builderCtx, builderCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer builderCancel()

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	var user User
	err := db.Builder().
		WithContext(builderCtx).
		Select().
		From("test_users").
		Where("id = ?", 1).
		WithContext(queryCtx).
		One(&user)

	if err != nil {
		t.Errorf("Query with overridden context should succeed: %v", err)
	}

	if user.ID != 1 {
		t.Errorf("Expected user ID 1, got %d", user.ID)
	}
}

// TestContextIntegration_TransactionContext tests transaction context propagation
func TestContextIntegration_TransactionContext(t *testing.T) {
	db := setupContextTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	var users []User
	err = tx.Builder().
		Select().
		From("test_users").
		All(&users)

	if err != nil {
		t.Errorf("Query in transaction should succeed: %v", err)
	}

	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}

	if err := tx.Commit(); err != nil {
		t.Errorf("Failed to commit transaction: %v", err)
	}
}

// TestContextIntegration_NoContext tests that queries work without explicit context
func TestContextIntegration_NoContext(t *testing.T) {
	db := setupContextTestDB(t)
	defer db.Close()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	var users []User
	err := db.Builder().
		Select().
		From("test_users").
		All(&users)

	if err != nil {
		t.Errorf("Query without context should succeed: %v", err)
	}

	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}
}

// TestContextIntegration_UpdateWithContext tests UPDATE with context
func TestContextIntegration_UpdateWithContext(t *testing.T) {
	db := setupContextTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.Builder().
		WithContext(ctx).
		Update("test_users").
		Set(map[string]interface{}{
			"name": "Alice Updated",
		}).
		Where("id = ?", 1).
		Execute()

	if err != nil {
		t.Errorf("UPDATE with context should succeed: %v", err)
	}

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	var user User
	err = db.Builder().
		Select().
		From("test_users").
		Where("id = ?", 1).
		One(&user)

	if err != nil {
		t.Errorf("SELECT after UPDATE should succeed: %v", err)
	}

	if user.Name != "Alice Updated" {
		t.Errorf("Expected name 'Alice Updated', got '%s'", user.Name)
	}
}

// TestContextIntegration_CanceledContext tests query with already canceled context
func TestContextIntegration_CanceledContext(t *testing.T) {
	db := setupContextTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	type User struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	var users []User
	err := db.Builder().
		WithContext(ctx).
		Select().
		From("test_users").
		All(&users)

	if err == nil {
		t.Error("Query with canceled context should fail")
	}
}
