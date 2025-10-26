package relica_test

import (
	"context"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // SQLite driver
)

// TestDB_ConvenienceMethods tests all 4 DB convenience methods
func TestDB_ConvenienceMethods(t *testing.T) {
	db, err := relica.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Setup test table
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE test_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT,
			status TEXT DEFAULT 'active'
		)
	`)
	require.NoError(t, err)

	t.Run("Select convenience method", func(t *testing.T) {
		// Insert test data via Builder (to test isolation)
		_, err := db.Builder().Insert("test_users", map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
		}).Execute()
		require.NoError(t, err)

		// Test convenience method
		var users []struct {
			ID     int    `db:"id"`
			Name   string `db:"name"`
			Email  string `db:"email"`
			Status string `db:"status"`
		}
		err = db.Select("id", "name", "email", "status").
			From("test_users").
			Where("name = ?", "Alice").
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 1)
		assert.Equal(t, "Alice", users[0].Name)
		assert.Equal(t, "alice@example.com", users[0].Email)
		assert.Equal(t, "active", users[0].Status)
	})

	t.Run("Select wildcard", func(t *testing.T) {
		var users []struct {
			ID     int    `db:"id"`
			Name   string `db:"name"`
			Email  string `db:"email"`
			Status string `db:"status"`
		}
		err := db.Select("*").
			From("test_users").
			Where("name = ?", "Alice").
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 1)
	})

	t.Run("Insert convenience method", func(t *testing.T) {
		result, err := db.Insert("test_users", map[string]interface{}{
			"name":  "Bob",
			"email": "bob@example.com",
		}).Execute()

		require.NoError(t, err)
		rows, _ := result.RowsAffected()
		assert.Equal(t, int64(1), rows)

		// Verify insertion
		var count int
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").
			From("test_users").
			Where("name = ?", "Bob").
			One(&countStruct)
		require.NoError(t, err)
		count = countStruct.Count
		assert.Equal(t, 1, count)
	})

	t.Run("Update convenience method", func(t *testing.T) {
		// Insert test record
		_, err := db.Insert("test_users", map[string]interface{}{
			"name":  "Charlie",
			"email": "charlie@old.com",
		}).Execute()
		require.NoError(t, err)

		// Test convenience method
		result, err := db.Update("test_users").
			Set(map[string]interface{}{"email": "charlie@new.com"}).
			Where("name = ?", "Charlie").
			Execute()

		require.NoError(t, err)
		rows, _ := result.RowsAffected()
		assert.Equal(t, int64(1), rows)

		// Verify update
		var emailStruct struct {
			Email string `db:"email"`
		}
		err = db.Select("email").
			From("test_users").
			Where("name = ?", "Charlie").
			One(&emailStruct)
		require.NoError(t, err)
		assert.Equal(t, "charlie@new.com", emailStruct.Email)
	})

	t.Run("Delete convenience method", func(t *testing.T) {
		// Insert test record
		_, err := db.Insert("test_users", map[string]interface{}{
			"name": "David",
		}).Execute()
		require.NoError(t, err)

		// Test convenience method
		result, err := db.Delete("test_users").
			Where("name = ?", "David").
			Execute()

		require.NoError(t, err)
		rows, _ := result.RowsAffected()
		assert.Equal(t, int64(1), rows)

		// Verify deletion
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").
			From("test_users").
			Where("name = ?", "David").
			One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 0, countStruct.Count)
	})

	t.Run("Chained operations", func(t *testing.T) {
		// Test multiple operations in sequence
		_, err := db.Insert("test_users", map[string]interface{}{
			"name":  "Eve",
			"email": "eve@example.com",
		}).Execute()
		require.NoError(t, err)

		_, err = db.Update("test_users").
			Set(map[string]interface{}{"status": "inactive"}).
			Where("name = ?", "Eve").
			Execute()
		require.NoError(t, err)

		var user struct {
			Name   string `db:"name"`
			Status string `db:"status"`
		}
		err = db.Select("name", "status").
			From("test_users").
			Where("name = ?", "Eve").
			One(&user)
		require.NoError(t, err)
		assert.Equal(t, "inactive", user.Status)

		_, err = db.Delete("test_users").
			Where("name = ?", "Eve").
			Execute()
		require.NoError(t, err)
	})
}

// TestTx_ConvenienceMethods tests all 4 Tx convenience methods
func TestTx_ConvenienceMethods(t *testing.T) {
	db, err := relica.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Setup test table
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE tx_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			status TEXT DEFAULT 'pending'
		)
	`)
	require.NoError(t, err)

	t.Run("Transaction with convenience methods", func(t *testing.T) {
		tx, err := db.Begin(context.Background())
		require.NoError(t, err)

		// Insert via convenience method
		_, err = tx.Insert("tx_test", map[string]interface{}{
			"name": "TxUser1",
		}).Execute()
		require.NoError(t, err)

		// Update via convenience method
		_, err = tx.Update("tx_test").
			Set(map[string]interface{}{"name": "TxUser1_Updated"}).
			Where("name = ?", "TxUser1").
			Execute()
		require.NoError(t, err)

		// Select via convenience method
		var names []struct {
			Name string `db:"name"`
		}
		err = tx.Select("name").
			From("tx_test").
			Where("name = ?", "TxUser1_Updated").
			All(&names)
		require.NoError(t, err)
		assert.Len(t, names, 1)
		assert.Equal(t, "TxUser1_Updated", names[0].Name)

		// Commit
		err = tx.Commit()
		require.NoError(t, err)

		// Verify outside transaction
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").
			From("tx_test").
			Where("name = ?", "TxUser1_Updated").
			One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 1, countStruct.Count)
	})

	t.Run("Transaction rollback with convenience methods", func(t *testing.T) {
		tx, err := db.Begin(context.Background())
		require.NoError(t, err)

		// Insert and delete via convenience methods
		_, err = tx.Insert("tx_test", map[string]interface{}{"name": "RollbackTest"}).Execute()
		require.NoError(t, err)

		_, err = tx.Delete("tx_test").Where("name = ?", "RollbackTest").Execute()
		require.NoError(t, err)

		// Rollback
		err = tx.Rollback()
		require.NoError(t, err)

		// Verify rollback (should have original count from previous test)
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").
			From("tx_test").
			One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 1, countStruct.Count) // Only TxUser1_Updated from previous test
	})

	t.Run("Transaction with multiple inserts", func(t *testing.T) {
		tx, err := db.Begin(context.Background())
		require.NoError(t, err)

		// Insert multiple records
		for i := 1; i <= 3; i++ {
			_, err = tx.Insert("tx_test", map[string]interface{}{
				"name": "BatchUser" + string(rune('0'+i)),
			}).Execute()
			require.NoError(t, err)
		}

		err = tx.Commit()
		require.NoError(t, err)

		// Verify all records inserted
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").
			From("tx_test").
			One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 4, countStruct.Count) // 1 from first test + 3 from this test
	})
}

// TestConvenienceMethods_BackwardCompatibility verifies Builder() still works
func TestConvenienceMethods_BackwardCompatibility(t *testing.T) {
	db, err := relica.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE compat_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	t.Run("Builder() method still works", func(t *testing.T) {
		// Old way (v0.4.0) should still work
		_, err := db.Builder().Insert("compat_test", map[string]interface{}{
			"name": "OldWay",
		}).Execute()
		require.NoError(t, err)

		var users []struct {
			Name string `db:"name"`
		}
		err = db.Builder().Select("name").
			From("compat_test").
			Where("name = ?", "OldWay").
			All(&users)
		require.NoError(t, err)
		assert.Len(t, users, 1)
		assert.Equal(t, "OldWay", users[0].Name)
	})

	t.Run("Both APIs work together", func(t *testing.T) {
		// Mix old and new API
		_, err := db.Insert("compat_test", map[string]interface{}{"name": "New1"}).Execute()
		require.NoError(t, err)

		_, err = db.Builder().Insert("compat_test", map[string]interface{}{"name": "Old1"}).Execute()
		require.NoError(t, err)

		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").From("compat_test").One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 3, countStruct.Count) // OldWay + New1 + Old1
	})

	t.Run("Transaction Builder() still works", func(t *testing.T) {
		tx, err := db.Begin(context.Background())
		require.NoError(t, err)

		// Old Builder() way
		_, err = tx.Builder().Insert("compat_test", map[string]interface{}{
			"name": "TxOldWay",
		}).Execute()
		require.NoError(t, err)

		// New convenience way
		_, err = tx.Insert("compat_test", map[string]interface{}{
			"name": "TxNewWay",
		}).Execute()
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)

		// Verify both inserted
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").From("compat_test").One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 5, countStruct.Count) // Previous 3 + 2 new
	})
}

// TestConvenienceMethods_ComplexQueries tests convenience methods with advanced features
func TestConvenienceMethods_ComplexQueries(t *testing.T) {
	db, err := relica.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			total REAL NOT NULL,
			status TEXT DEFAULT 'pending'
		)
	`)
	require.NoError(t, err)

	// Insert test data
	for i := 1; i <= 5; i++ {
		_, err = db.Insert("orders", map[string]interface{}{
			"user_id": i % 2, // Alternating 0 and 1
			"total":   float64(i * 100),
			"status":  "completed",
		}).Execute()
		require.NoError(t, err)
	}

	t.Run("Select with ORDER BY and LIMIT", func(t *testing.T) {
		var orders []struct {
			ID    int     `db:"id"`
			Total float64 `db:"total"`
		}
		err := db.Select("id", "total").
			From("orders").
			OrderBy("total DESC").
			Limit(3).
			All(&orders)

		require.NoError(t, err)
		assert.Len(t, orders, 3)
		assert.Equal(t, 500.0, orders[0].Total) // Highest first
		assert.Equal(t, 400.0, orders[1].Total)
		assert.Equal(t, 300.0, orders[2].Total)
	})

	t.Run("Select with WHERE and aggregates", func(t *testing.T) {
		var result struct {
			Count int     `db:"count"`
			Sum   float64 `db:"sum"`
		}
		err := db.Select("COUNT(*) as count", "SUM(total) as sum").
			From("orders").
			Where("status = ?", "completed").
			One(&result)

		require.NoError(t, err)
		assert.Equal(t, 5, result.Count)
		assert.Equal(t, 1500.0, result.Sum) // 100+200+300+400+500
	})

	t.Run("Update with complex WHERE", func(t *testing.T) {
		result, err := db.Update("orders").
			Set(map[string]interface{}{"status": "archived"}).
			Where("total >= ?", 300).
			Execute()

		require.NoError(t, err)
		rows, _ := result.RowsAffected()
		assert.Equal(t, int64(3), rows) // 300, 400, 500

		// Verify
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").
			From("orders").
			Where("status = ?", "archived").
			One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 3, countStruct.Count)
	})

	t.Run("Delete with WHERE", func(t *testing.T) {
		result, err := db.Delete("orders").
			Where("total < ?", 300).
			Execute()

		require.NoError(t, err)
		rows, _ := result.RowsAffected()
		assert.Equal(t, int64(2), rows) // 100, 200

		// Verify remaining
		var countStruct struct {
			Count int `db:"count"`
		}
		err = db.Select("COUNT(*) as count").
			From("orders").
			One(&countStruct)
		require.NoError(t, err)
		assert.Equal(t, 3, countStruct.Count) // 300, 400, 500
	})
}
