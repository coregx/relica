package core

import (
	"context"
	"database/sql"
	"testing"

	"github.com/coregx/relica/internal/cache"
	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestUpdateIntegration_SQLite tests UPDATE operations with real SQLite database.
func TestUpdateIntegration_SQLite(t *testing.T) {
	// Setup: Create in-memory database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer sqlDB.Close()

	db := &DB{
		sqlDB:      sqlDB,
		driverName: "sqlite",
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialects.GetDialect("sqlite"),
		tracer:     NewNoOpTracer(),
		ctx:        context.Background(),
	}

	// Create users table
	_, err = sqlDB.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			age INTEGER
		)
	`)
	require.NoError(t, err)

	// Insert test data
	_, err = sqlDB.Exec(`
		INSERT INTO users (id, name, email, status, age) VALUES
		(1, 'Alice', 'alice@example.com', 'active', 25),
		(2, 'Bob', 'bob@example.com', 'active', 30),
		(3, 'Charlie', 'charlie@example.com', 'inactive', 35)
	`)
	require.NoError(t, err)

	t.Run("update single row", func(t *testing.T) {
		// Update Alice's email
		result, err := db.Builder().
			Update("users").
			Set(map[string]interface{}{
				"email": "alice.new@example.com",
			}).
			Where("id = ?", 1).
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected)

		// Verify update
		var email string
		err = sqlDB.QueryRow("SELECT email FROM users WHERE id = 1").Scan(&email)
		require.NoError(t, err)
		assert.Equal(t, "alice.new@example.com", email)
	})

	t.Run("update multiple columns", func(t *testing.T) {
		// Update Bob's name and age
		result, err := db.Builder().
			Update("users").
			Set(map[string]interface{}{
				"name": "Robert",
				"age":  31,
			}).
			Where("id = ?", 2).
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected)

		// Verify update
		var name string
		var age int
		err = sqlDB.QueryRow("SELECT name, age FROM users WHERE id = 2").Scan(&name, &age)
		require.NoError(t, err)
		assert.Equal(t, "Robert", name)
		assert.Equal(t, 31, age)
	})

	t.Run("update with multiple WHERE conditions", func(t *testing.T) {
		// Update status for active users older than 28
		result, err := db.Builder().
			Update("users").
			Set(map[string]interface{}{
				"status": "senior",
			}).
			Where("status = ?", "active").
			Where("age > ?", 28).
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected) // Only Bob (age 31)

		// Verify Bob's status changed
		var status string
		err = sqlDB.QueryRow("SELECT status FROM users WHERE id = 2").Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "senior", status)

		// Verify Alice's status unchanged (age 25 < 28)
		err = sqlDB.QueryRow("SELECT status FROM users WHERE id = 1").Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "active", status)
	})

	t.Run("update no matching rows", func(t *testing.T) {
		// Update non-existent user
		result, err := db.Builder().
			Update("users").
			Set(map[string]interface{}{
				"name": "Nobody",
			}).
			Where("id = ?", 999).
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(0), rowsAffected)
	})

	t.Run("update all rows", func(t *testing.T) {
		// Update all users' status
		result, err := db.Builder().
			Update("users").
			Set(map[string]interface{}{
				"status": "verified",
			}).
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(3), rowsAffected) // All 3 users

		// Verify all updated
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM users WHERE status = 'verified'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})
}

// TestDeleteIntegration_SQLite tests DELETE operations with real SQLite database.
func TestDeleteIntegration_SQLite(t *testing.T) {
	// Setup: Create in-memory database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer sqlDB.Close()

	db := &DB{
		sqlDB:      sqlDB,
		driverName: "sqlite",
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialects.GetDialect("sqlite"),
		tracer:     NewNoOpTracer(),
		ctx:        context.Background(),
	}
	require.NoError(t, err)

	// Create users table
	_, err = sqlDB.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			created_at TEXT
		)
	`)
	require.NoError(t, err)

	// Insert test data
	insertTestData := func() {
		_, err = sqlDB.Exec("DELETE FROM users") // Clear table
		require.NoError(t, err)
		_, err = sqlDB.Exec(`
			INSERT INTO users (id, name, email, status, created_at) VALUES
			(1, 'Alice', 'alice@example.com', 'active', '2025-01-01'),
			(2, 'Bob', 'bob@example.com', 'active', '2024-01-01'),
			(3, 'Charlie', 'charlie@example.com', 'deleted', '2023-01-01'),
			(4, 'Diana', 'diana@example.com', 'inactive', '2022-01-01')
		`)
		require.NoError(t, err)
	}

	t.Run("delete single row by ID", func(t *testing.T) {
		insertTestData()

		// Delete Alice
		result, err := db.Builder().
			Delete("users").
			Where("id = ?", 1).
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected)

		// Verify deletion
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM users WHERE id = 1").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify total count
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 3, count) // 3 remaining
	})

	t.Run("delete multiple rows with WHERE", func(t *testing.T) {
		insertTestData()

		// Delete users with status 'deleted' or 'inactive'
		result, err := db.Builder().
			Delete("users").
			Where("status IN ('deleted', 'inactive')").
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(2), rowsAffected) // Charlie and Diana

		// Verify only active users remain
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM users WHERE status = 'active'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count) // Alice and Bob
	})

	t.Run("delete with multiple WHERE conditions", func(t *testing.T) {
		insertTestData()

		// Delete old deleted users
		result, err := db.Builder().
			Delete("users").
			Where("status = ?", "deleted").
			Where("created_at < ?", "2024-01-01").
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected) // Only Charlie (2023-01-01)

		// Verify remaining users
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 3, count) // All except Charlie
	})

	t.Run("delete no matching rows", func(t *testing.T) {
		insertTestData()

		// Delete non-existent user
		result, err := db.Builder().
			Delete("users").
			Where("id = ?", 999).
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(0), rowsAffected)

		// Verify count unchanged
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 4, count) // All 4 still there
	})

	t.Run("delete all rows", func(t *testing.T) {
		insertTestData()

		// Delete all users
		result, err := db.Builder().
			Delete("users").
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(4), rowsAffected) // All 4 users

		// Verify table empty
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// TestUpdateDelete_Consistency tests that UPDATE and DELETE work correctly together.
func TestUpdateDelete_Consistency(t *testing.T) {
	// Setup: Create in-memory database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer sqlDB.Close()

	db := &DB{
		sqlDB:      sqlDB,
		driverName: "sqlite",
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialects.GetDialect("sqlite"),
		tracer:     NewNoOpTracer(),
		ctx:        context.Background(),
	}
	require.NoError(t, err)

	// Create orders table
	_, err = sqlDB.Exec(`
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY,
			customer_id INTEGER NOT NULL,
			status TEXT DEFAULT 'pending',
			total REAL,
			notes TEXT
		)
	`)
	require.NoError(t, err)

	// Insert test orders
	_, err = sqlDB.Exec(`
		INSERT INTO orders (id, customer_id, status, total, notes) VALUES
		(1, 100, 'pending', 50.00, 'Rush order'),
		(2, 100, 'pending', 75.00, NULL),
		(3, 101, 'completed', 100.00, 'Gift wrap'),
		(4, 102, 'canceled', 25.00, 'Refunded')
	`)
	require.NoError(t, err)

	// Scenario: Mark old pending orders as canceled, then delete canceled orders
	t.Run("update then delete workflow", func(t *testing.T) {
		// Step 1: Mark pending orders from customer 100 as canceled
		result, err := db.Builder().
			Update("orders").
			Set(map[string]interface{}{
				"status": "canceled",
				"notes":  "Auto-canceled",
			}).
			Where("customer_id = ?", 100).
			Where("status = ?", "pending").
			Execute()

		require.NoError(t, err)
		rowsAffected, err := result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(2), rowsAffected) // Orders 1 and 2

		// Verify status updated
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM orders WHERE status = 'canceled'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 3, count) // Orders 1, 2, and 4

		// Step 2: Delete all canceled orders
		result, err = db.Builder().
			Delete("orders").
			Where("status = ?", "canceled").
			Execute()

		require.NoError(t, err)
		rowsAffected, err = result.(sql.Result).RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(3), rowsAffected) // Orders 1, 2, and 4

		// Verify only completed order remains
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count) // Only order 3

		var status string
		err = sqlDB.QueryRow("SELECT status FROM orders WHERE id = 3").Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "completed", status)
	})
}

// TestUpdateDelete_StatementCaching tests that UPDATE and DELETE statements are cached properly.
func TestUpdateDelete_StatementCaching(t *testing.T) {
	// Setup: Create in-memory database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer sqlDB.Close()

	db := &DB{
		sqlDB:      sqlDB,
		driverName: "sqlite",
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialects.GetDialect("sqlite"),
		tracer:     NewNoOpTracer(),
		ctx:        context.Background(),
	}
	require.NoError(t, err)

	// Create test table
	_, err = sqlDB.Exec(`
		CREATE TABLE test (
			id INTEGER PRIMARY KEY,
			value TEXT
		)
	`)
	require.NoError(t, err)

	// Insert test data
	_, err = sqlDB.Exec("INSERT INTO test (id, value) VALUES (1, 'a'), (2, 'b'), (3, 'c')")
	require.NoError(t, err)

	t.Run("UPDATE statement caching", func(t *testing.T) {
		// Execute same UPDATE query twice
		for i := 0; i < 2; i++ {
			_, err := db.Builder().
				Update("test").
				Set(map[string]interface{}{"value": "updated"}).
				Where("id = ?", 1).
				Execute()
			require.NoError(t, err)
		}

		// Verify cache hit (query should be cached after first execution)
		// Note: We can't directly test cache hits without exposing cache stats,
		// but we verify the query executes successfully multiple times
		var value string
		err = sqlDB.QueryRow("SELECT value FROM test WHERE id = 1").Scan(&value)
		require.NoError(t, err)
		assert.Equal(t, "updated", value)
	})

	t.Run("DELETE statement caching", func(t *testing.T) {
		// Re-insert data
		_, err = sqlDB.Exec("INSERT INTO test (id, value) VALUES (4, 'd'), (5, 'e')")
		require.NoError(t, err)

		// Execute same DELETE query twice (on different data)
		for i := 0; i < 2; i++ {
			_, err := db.Builder().
				Delete("test").
				Where("id = ?", i+4). // Delete 4, then 5
				Execute()
			require.NoError(t, err)
		}

		// Verify deletions
		var count int
		err = sqlDB.QueryRow("SELECT COUNT(*) FROM test WHERE id >= 4").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}
