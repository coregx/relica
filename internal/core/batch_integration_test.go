package core

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/coregx/relica/internal/cache"
	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// setupTestDB creates a temporary SQLite database for integration testing.
func setupBatchTestDB(t *testing.T) *DB {
	// Create in-memory SQLite database (faster than file-based for tests)
	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	// Clean up on test completion
	t.Cleanup(func() {
		_ = sqlDB.Close() // Ignore close error in cleanup
	})

	// Create DB wrapper
	db := &DB{
		sqlDB:      sqlDB,
		driverName: "sqlite",
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialects.GetDialect("sqlite"),
		logger:     &logger.NoopLogger{},
		sanitizer:  logger.NewSanitizer(nil),
		ctx:        context.Background(),
	}

	// Create test tables
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT,
			age INTEGER,
			status TEXT
		)
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			price REAL,
			stock INTEGER
		)
	`)
	require.NoError(t, err)

	return db
}

// TestBatchInsertIntegration_SQLite tests batch INSERT with actual SQLite database.
func TestBatchInsertIntegration_SQLite(t *testing.T) {
	db := setupBatchTestDB(t)

	// Insert 3 users in a single batch
	result, err := db.Builder().
		BatchInsert("users", []string{"name", "email", "age"}).
		Values("Alice", "alice@example.com", 30).
		Values("Bob", "bob@example.com", 25).
		Values("Charlie", "charlie@example.com", 35).
		Execute()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result
	sqlResult, ok := result.(sql.Result)
	require.True(t, ok)

	rowsAffected, err := sqlResult.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(3), rowsAffected)

	// Verify data was inserted correctly
	var count int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify individual records
	var name, email string
	var age int
	err = db.QueryRowContext(context.Background(), "SELECT name, email, age FROM users WHERE name = ?", "Bob").
		Scan(&name, &email, &age)
	require.NoError(t, err)
	assert.Equal(t, "Bob", name)
	assert.Equal(t, "bob@example.com", email)
	assert.Equal(t, 25, age)
}

// TestBatchInsertIntegration_LargeDataset tests batch INSERT with many rows.
func TestBatchInsertIntegration_LargeDataset(t *testing.T) {
	db := setupBatchTestDB(t)

	// Insert 100 products in a single batch
	batch := db.Builder().BatchInsert("products", []string{"name", "price", "stock"})
	for i := 1; i <= 100; i++ {
		batch.Values(
			fmt.Sprintf("Product %d", i),
			float64(i)*9.99,
			i*10,
		)
	}

	result, err := batch.Execute()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify all rows were inserted
	var count int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM products").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 100, count)

	// Verify a sample record
	var name string
	var price float64
	var stock int
	err = db.QueryRowContext(context.Background(), "SELECT name, price, stock FROM products WHERE name = ?", "Product 50").
		Scan(&name, &price, &stock)
	require.NoError(t, err)
	assert.Equal(t, "Product 50", name)
	assert.InDelta(t, 50*9.99, price, 0.01)
	assert.Equal(t, 500, stock)
}

// TestBatchInsertIntegration_ValuesMap tests batch INSERT with map-based values.
func TestBatchInsertIntegration_ValuesMap(t *testing.T) {
	db := setupBatchTestDB(t)

	result, err := db.Builder().
		BatchInsert("users", []string{"name", "email", "age"}).
		ValuesMap(map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
			"age":   30,
		}).
		ValuesMap(map[string]interface{}{
			"name":  "Bob",
			"email": "bob@example.com",
			"age":   25,
		}).
		Execute()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify data
	var count int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// TestBatchInsertIntegration_NullValues tests batch INSERT with NULL values.
func TestBatchInsertIntegration_NullValues(t *testing.T) {
	db := setupBatchTestDB(t)

	result, err := db.Builder().
		BatchInsert("users", []string{"name", "email", "age"}).
		Values("Alice", "alice@example.com", nil). // age is NULL
		Values("Bob", nil, 30).                    // email is NULL
		Execute()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify Alice has NULL age
	var name string
	var email sql.NullString
	var age sql.NullInt64
	err = db.QueryRowContext(context.Background(), "SELECT name, email, age FROM users WHERE name = ?", "Alice").
		Scan(&name, &email, &age)
	require.NoError(t, err)
	assert.Equal(t, "Alice", name)
	assert.True(t, email.Valid)
	assert.Equal(t, "alice@example.com", email.String)
	assert.False(t, age.Valid) // age should be NULL

	// Verify Bob has NULL email
	err = db.QueryRowContext(context.Background(), "SELECT name, email, age FROM users WHERE name = ?", "Bob").
		Scan(&name, &email, &age)
	require.NoError(t, err)
	assert.Equal(t, "Bob", name)
	assert.False(t, email.Valid) // email should be NULL
	assert.True(t, age.Valid)
	assert.Equal(t, int64(30), age.Int64)
}

// TestBatchUpdateIntegration_SQLite tests batch UPDATE with actual SQLite database.
func TestBatchUpdateIntegration_SQLite(t *testing.T) {
	db := setupBatchTestDB(t)

	// Insert test data
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO users (name, email, age, status) VALUES
		('Alice', 'alice@old.com', 30, 'pending'),
		('Bob', 'bob@old.com', 25, 'pending'),
		('Charlie', 'charlie@old.com', 35, 'pending')
	`)
	require.NoError(t, err)

	// Update multiple rows with different values
	result, err := db.Builder().
		BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"email": "alice@new.com", "status": "active"}).
		Set(2, map[string]interface{}{"email": "bob@new.com", "status": "active"}).
		Set(3, map[string]interface{}{"email": "charlie@new.com", "status": "inactive"}).
		Execute()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result
	sqlResult, ok := result.(sql.Result)
	require.True(t, ok)

	rowsAffected, err := sqlResult.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(3), rowsAffected)

	// Verify Alice was updated
	var email, status string
	err = db.QueryRowContext(context.Background(), "SELECT email, status FROM users WHERE id = 1").Scan(&email, &status)
	require.NoError(t, err)
	assert.Equal(t, "alice@new.com", email)
	assert.Equal(t, "active", status)

	// Verify Bob was updated
	err = db.QueryRowContext(context.Background(), "SELECT email, status FROM users WHERE id = 2").Scan(&email, &status)
	require.NoError(t, err)
	assert.Equal(t, "bob@new.com", email)
	assert.Equal(t, "active", status)

	// Verify Charlie was updated
	err = db.QueryRowContext(context.Background(), "SELECT email, status FROM users WHERE id = 3").Scan(&email, &status)
	require.NoError(t, err)
	assert.Equal(t, "charlie@new.com", email)
	assert.Equal(t, "inactive", status)
}

// TestBatchUpdateIntegration_LargeDataset tests batch UPDATE with many rows.
func TestBatchUpdateIntegration_LargeDataset(t *testing.T) {
	db := setupBatchTestDB(t)

	// Insert 50 products
	batch := db.Builder().BatchInsert("products", []string{"name", "price", "stock"})
	for i := 1; i <= 50; i++ {
		batch.Values(fmt.Sprintf("Product %d", i), float64(i)*10.0, 100)
	}
	_, err := batch.Execute()
	require.NoError(t, err)

	// Update all 50 products with new prices
	updateBatch := db.Builder().BatchUpdate("products", "id")
	for i := 1; i <= 50; i++ {
		updateBatch.Set(i, map[string]interface{}{
			"price": float64(i) * 15.0, // Increase price by 50%
			"stock": 200,               // Double stock
		})
	}

	result, err := updateBatch.Execute()
	require.NoError(t, err)
	require.NotNil(t, result)

	sqlResult, ok := result.(sql.Result)
	require.True(t, ok)

	rowsAffected, err := sqlResult.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(50), rowsAffected)

	// Verify a sample product was updated
	var price float64
	var stock int
	err = db.QueryRowContext(context.Background(), "SELECT price, stock FROM products WHERE id = 25").Scan(&price, &stock)
	require.NoError(t, err)
	assert.InDelta(t, 25*15.0, price, 0.01)
	assert.Equal(t, 200, stock)
}

// TestBatchUpdateIntegration_DifferentColumns tests batch UPDATE where rows update different columns.
func TestBatchUpdateIntegration_DifferentColumns(t *testing.T) {
	db := setupBatchTestDB(t)

	// Insert test data
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO users (name, email, age, status) VALUES
		('Alice', 'alice@example.com', 30, 'pending'),
		('Bob', 'bob@example.com', 25, 'pending'),
		('Charlie', 'charlie@example.com', 35, 'pending')
	`)
	require.NoError(t, err)

	// Update different columns for each row
	result, err := db.Builder().
		BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice Updated", "email": "alice@new.com"}). // Update name and email
		Set(2, map[string]interface{}{"age": 26}).                                         // Update only age
		Set(3, map[string]interface{}{"status": "active", "age": 36}).                     // Update status and age
		Execute()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify row 1 updates
	var name, email, status string
	var age int
	err = db.QueryRowContext(context.Background(), "SELECT name, email, age, status FROM users WHERE id = 1").
		Scan(&name, &email, &age, &status)
	require.NoError(t, err)
	assert.Equal(t, "Alice Updated", name)
	assert.Equal(t, "alice@new.com", email)
	assert.Equal(t, 30, age)           // unchanged
	assert.Equal(t, "pending", status) // unchanged

	// Verify row 2 updates
	err = db.QueryRowContext(context.Background(), "SELECT name, email, age, status FROM users WHERE id = 2").
		Scan(&name, &email, &age, &status)
	require.NoError(t, err)
	assert.Equal(t, "Bob", name)              // unchanged
	assert.Equal(t, "bob@example.com", email) // unchanged
	assert.Equal(t, 26, age)                  // updated
	assert.Equal(t, "pending", status)        // unchanged

	// Verify row 3 updates
	err = db.QueryRowContext(context.Background(), "SELECT name, email, age, status FROM users WHERE id = 3").
		Scan(&name, &email, &age, &status)
	require.NoError(t, err)
	assert.Equal(t, "Charlie", name)              // unchanged
	assert.Equal(t, "charlie@example.com", email) // unchanged
	assert.Equal(t, 36, age)                      // updated
	assert.Equal(t, "active", status)             // updated
}

// TestBatchOperations_Transaction tests batch operations within a transaction.
func TestBatchOperations_Transaction(t *testing.T) {
	db := setupBatchTestDB(t)

	// Test rollback
	tx, err := db.Begin(context.Background())
	require.NoError(t, err)

	_, err = tx.Builder().
		BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com").
		Execute()
	require.NoError(t, err)

	// Rollback the transaction
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify data was NOT persisted
	var count int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Test commit
	tx, err = db.Begin(context.Background())
	require.NoError(t, err)

	_, err = tx.Builder().
		BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com").
		Execute()
	require.NoError(t, err)

	// Commit the transaction
	err = tx.Commit()
	require.NoError(t, err)

	// Verify data WAS persisted
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// TestBatchUpdateIntegration_Transaction tests batch UPDATE within transaction.
func TestBatchUpdateIntegration_Transaction(t *testing.T) {
	db := setupBatchTestDB(t)

	// Insert test data
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO users (name, email, status) VALUES
		('Alice', 'alice@example.com', 'pending'),
		('Bob', 'bob@example.com', 'pending')
	`)
	require.NoError(t, err)

	// Test rollback
	tx, err := db.Begin(context.Background())
	require.NoError(t, err)

	_, err = tx.Builder().
		BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"status": "active"}).
		Set(2, map[string]interface{}{"status": "active"}).
		Execute()
	require.NoError(t, err)

	// Rollback
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify status is still 'pending'
	var status string
	err = db.QueryRowContext(context.Background(), "SELECT status FROM users WHERE id = 1").Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "pending", status)
}

// TestBatchInsertIntegration_MixedOperations tests combining batch insert with other operations.
func TestBatchInsertIntegration_MixedOperations(t *testing.T) {
	db := setupBatchTestDB(t)

	// Use transaction to combine operations
	tx, err := db.Begin(context.Background())
	require.NoError(t, err)

	// Insert users
	_, err = tx.Builder().
		BatchInsert("users", []string{"name", "email", "status"}).
		Values("Alice", "alice@example.com", "pending").
		Values("Bob", "bob@example.com", "pending").
		Execute()
	require.NoError(t, err)

	// Insert products
	_, err = tx.Builder().
		BatchInsert("products", []string{"name", "price"}).
		Values("Product A", 19.99).
		Values("Product B", 29.99).
		Execute()
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify both tables have data
	var userCount, productCount int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&userCount)
	require.NoError(t, err)
	assert.Equal(t, 2, userCount)

	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM products").Scan(&productCount)
	require.NoError(t, err)
	assert.Equal(t, 2, productCount)
}
