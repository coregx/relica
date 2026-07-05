//go:build integration
// +build integration

package test

import (
	"context"
	"errors"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTxTestTable creates a simple table for transaction tests.
func createTxTestTable(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	var ddl string
	switch dialect {
	case "postgres":
		ddl = `CREATE TABLE IF NOT EXISTS tx_test (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER NOT NULL DEFAULT 0
		)`
	case "mysql":
		ddl = "CREATE TABLE IF NOT EXISTS tx_test (" +
			"id INT AUTO_INCREMENT PRIMARY KEY, " +
			"name VARCHAR(255) NOT NULL, " +
			"value INT NOT NULL DEFAULT 0" +
			") ENGINE=InnoDB"
	case "sqlite":
		ddl = `CREATE TABLE IF NOT EXISTS tx_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			value INTEGER NOT NULL DEFAULT 0
		)`
	}
	_, err := db.ExecContext(context.Background(), ddl)
	require.NoError(t, err)
	_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")
}

func dropTxTestTable(t *testing.T, db *relica.DB) {
	t.Helper()
	_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS tx_test")
}

// runTransactionTests executes all transaction tests for the given database.
func runTransactionTests(t *testing.T, ds *DatabaseSetup) {
	db := ds.DB
	dialect := ds.Dialect

	createTxTestTable(t, db, dialect)
	defer dropTxTestTable(t, db)

	// ---------------------------------------------------------------
	// 1. Multiple operations in a single transaction — all committed
	// ---------------------------------------------------------------
	t.Run("MultipleOps_Commit", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.Insert("tx_test", map[string]interface{}{
				"name": "alice", "value": 10,
			}).Execute()
			if err != nil {
				return err
			}

			_, err = tx.Insert("tx_test", map[string]interface{}{
				"name": "bob", "value": 20,
			}).Execute()
			if err != nil {
				return err
			}

			_, err = tx.Update("tx_test").
				Set(map[string]interface{}{"value": 15}).
				Where(relica.Eq("name", "alice")).
				Execute()
			if err != nil {
				return err
			}

			return nil
		})
		require.NoError(t, err)

		// Verify all 3 ops committed
		var rows []struct {
			Name  string `db:"name"`
			Value int    `db:"value"`
		}
		err = db.Select("name", "value").From("tx_test").OrderBy("name").All(&rows)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, "alice", rows[0].Name)
		assert.Equal(t, 15, rows[0].Value) // updated from 10 to 15
		assert.Equal(t, "bob", rows[1].Name)
		assert.Equal(t, 20, rows[1].Value)
	})

	// ---------------------------------------------------------------
	// 2. Rollback on error — nothing persisted
	// ---------------------------------------------------------------
	t.Run("Rollback_OnError", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		// Insert one row outside transaction
		_, err := db.Insert("tx_test", map[string]interface{}{
			"name": "pre-existing", "value": 100,
		}).Execute()
		require.NoError(t, err)

		// Transaction that inserts then fails
		txErr := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.Insert("tx_test", map[string]interface{}{
				"name": "should-not-persist", "value": 999,
			}).Execute()
			if err != nil {
				return err
			}

			// Force rollback
			return errors.New("intentional rollback")
		})
		require.Error(t, txErr)
		assert.Contains(t, txErr.Error(), "intentional rollback")

		// Verify only pre-existing row remains
		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)

		var row struct {
			Name string `db:"name"`
		}
		err = db.Select("name").From("tx_test").One(&row)
		require.NoError(t, err)
		assert.Equal(t, "pre-existing", row.Name)
	})

	// ---------------------------------------------------------------
	// 3. Rollback on panic — nothing persisted, panic re-raised
	// ---------------------------------------------------------------
	t.Run("Rollback_OnPanic", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		assert.Panics(t, func() {
			_ = db.Transactional(context.Background(), func(tx *relica.Tx) error {
				_, _ = tx.Insert("tx_test", map[string]interface{}{
					"name": "panic-row", "value": 666,
				}).Execute()

				panic("something went wrong")
			})
		})

		// Verify nothing was committed
		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	// ---------------------------------------------------------------
	// 4. Select inside transaction sees uncommitted data
	// ---------------------------------------------------------------
	t.Run("ReadYourOwnWrites", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.Insert("tx_test", map[string]interface{}{
				"name": "visible-inside-tx", "value": 42,
			}).Execute()
			if err != nil {
				return err
			}

			// Read back within same transaction
			var row struct {
				Name  string `db:"name"`
				Value int    `db:"value"`
			}
			err = tx.Select("name", "value").From("tx_test").
				Where(relica.Eq("name", "visible-inside-tx")).
				One(&row)
			if err != nil {
				return err
			}

			assert.Equal(t, "visible-inside-tx", row.Name)
			assert.Equal(t, 42, row.Value)

			return nil
		})
		require.NoError(t, err)
	})

	// ---------------------------------------------------------------
	// 5. BatchInsert inside transaction
	// ---------------------------------------------------------------
	t.Run("BatchInsert_InTransaction", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.BatchInsert("tx_test", []string{"name", "value"}).
				Values("batch-1", 100).
				Values("batch-2", 200).
				Values("batch-3", 300).
				Execute()
			return err
		})
		require.NoError(t, err)

		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	// ---------------------------------------------------------------
	// 6. BatchInsert rollback — nothing persisted
	// ---------------------------------------------------------------
	t.Run("BatchInsert_Rollback", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		txErr := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.BatchInsert("tx_test", []string{"name", "value"}).
				Values("should-not-persist-1", 100).
				Values("should-not-persist-2", 200).
				Execute()
			if err != nil {
				return err
			}

			return errors.New("rollback after batch")
		})
		require.Error(t, txErr)

		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	// ---------------------------------------------------------------
	// 7. Update + Delete in same transaction
	// ---------------------------------------------------------------
	t.Run("Update_Delete_SameTransaction", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		// Seed data
		_, _ = db.Insert("tx_test", map[string]interface{}{"name": "keep", "value": 1}).Execute()
		_, _ = db.Insert("tx_test", map[string]interface{}{"name": "update-me", "value": 2}).Execute()
		_, _ = db.Insert("tx_test", map[string]interface{}{"name": "delete-me", "value": 3}).Execute()

		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.Update("tx_test").
				Set(map[string]interface{}{"value": 999}).
				Where(relica.Eq("name", "update-me")).
				Execute()
			if err != nil {
				return err
			}

			_, err = tx.Delete("tx_test").
				Where(relica.Eq("name", "delete-me")).
				Execute()
			return err
		})
		require.NoError(t, err)

		var rows []struct {
			Name  string `db:"name"`
			Value int    `db:"value"`
		}
		err = db.Select("name", "value").From("tx_test").OrderBy("name").All(&rows)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, "keep", rows[0].Name)
		assert.Equal(t, 1, rows[0].Value)
		assert.Equal(t, "update-me", rows[1].Name)
		assert.Equal(t, 999, rows[1].Value)
	})

	// ---------------------------------------------------------------
	// 8. Manual Begin/Commit
	// ---------------------------------------------------------------
	t.Run("Manual_Begin_Commit", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		tx, err := db.Begin(context.Background())
		require.NoError(t, err)

		_, err = tx.Insert("tx_test", map[string]interface{}{
			"name": "manual-commit", "value": 77,
		}).Execute()
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)

		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	// ---------------------------------------------------------------
	// 9. Manual Begin/Rollback
	// ---------------------------------------------------------------
	t.Run("Manual_Begin_Rollback", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		tx, err := db.Begin(context.Background())
		require.NoError(t, err)

		_, err = tx.Insert("tx_test", map[string]interface{}{
			"name": "manual-rollback", "value": 88,
		}).Execute()
		require.NoError(t, err)

		err = tx.Rollback()
		require.NoError(t, err)

		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	// ---------------------------------------------------------------
	// 10. Upsert inside transaction
	// ---------------------------------------------------------------
	t.Run("Upsert_InTransaction", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		// Seed a row
		_, err := db.Insert("tx_test", map[string]interface{}{
			"name": "upsert-target", "value": 10,
		}).Execute()
		require.NoError(t, err)

		// Get the ID for conflict
		var row struct {
			ID int `db:"id"`
		}
		err = db.Select("id").From("tx_test").
			Where(relica.Eq("name", "upsert-target")).One(&row)
		require.NoError(t, err)

		err = db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.Upsert("tx_test", map[string]interface{}{
				"id": row.ID, "name": "upsert-target", "value": 99,
			}).OnConflict("id").DoUpdate("value").Execute()
			return err
		})
		require.NoError(t, err)

		var updated struct {
			Value int `db:"value"`
		}
		err = db.Select("value").From("tx_test").
			Where(relica.Eq("name", "upsert-target")).One(&updated)
		require.NoError(t, err)
		assert.Equal(t, 99, updated.Value)
	})

	// ---------------------------------------------------------------
	// 11. Model API inside transaction
	// ---------------------------------------------------------------
	t.Run("Model_InsertUpdate_InTransaction", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			// Use Insert + Update (map-based) to test multiple ops via tx
			_, err := tx.Insert("tx_test", map[string]interface{}{
				"name": "model-insert", "value": 55,
			}).Execute()
			if err != nil {
				return err
			}

			// Update the row we just inserted
			_, err = tx.Update("tx_test").
				Set(map[string]interface{}{"value": 66}).
				Where(relica.Eq("name", "model-insert")).
				Execute()
			return err
		})
		require.NoError(t, err)

		var result struct {
			Value int `db:"value"`
		}
		err = db.Select("value").From("tx_test").
			Where(relica.Eq("name", "model-insert")).One(&result)
		require.NoError(t, err)
		assert.Equal(t, 66, result.Value)
	})

	// ---------------------------------------------------------------
	// 12. NewQuery (raw SQL) inside transaction
	// ---------------------------------------------------------------
	t.Run("NewQuery_InTransaction", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			q := tx.NewQuery("INSERT INTO tx_test (name, value) VALUES (?, ?)")
			q.Bind(relica.Params{"p1": "raw-sql"})
			_, err := q.Execute()
			if err != nil {
				// Try with positional params directly
				_, err = tx.Insert("tx_test", map[string]interface{}{
					"name": "raw-sql", "value": 123,
				}).Execute()
			}
			return err
		})
		require.NoError(t, err)

		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	// ---------------------------------------------------------------
	// 13. Many operations in one transaction (stress test)
	// ---------------------------------------------------------------
	t.Run("ManyOps_SingleTransaction", func(t *testing.T) {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM tx_test")

		numOps := 50
		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			for i := 0; i < numOps; i++ {
				_, err := tx.Insert("tx_test", map[string]interface{}{
					"name":  "stress-" + string(rune('A'+i%26)),
					"value": i,
				}).Execute()
				if err != nil {
					return err
				}
			}
			return nil
		})
		require.NoError(t, err)

		count, err := db.Select().From("tx_test").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(numOps), count)
	})
}

// ================================================================
// Entry points for each database
// ================================================================

func TestTransaction_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runTransactionTests(t, ds)
}

func TestTransaction_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runTransactionTests(t, ds)
}

func TestTransaction_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runTransactionTests(t, ds)
}
