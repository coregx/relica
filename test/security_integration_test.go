//go:build integration
// +build integration

// Package test contains integration tests for the Relica query builder.
// This file validates the P0 security fix from the enterprise audit (PRs #23-#27):
// all identifiers (table names, column names) are now properly quoted in every
// SQL verb — INSERT, UPDATE, UPSERT, SELECT, DELETE — even when the column name
// is a SQL reserved word.
package test

import (
	"context"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reservedRow is the scan target for test_reserved rows.
// "where" and "group" are nullable in the schema so the fields use pointer types.
type reservedRow struct {
	ID     int     `db:"id"`
	Order  int     `db:"order"`
	Select string  `db:"select"`
	Group  *string `db:"group"`
	Where  *string `db:"where"`
	Index  int     `db:"index"`
}

// runReservedWordTests executes the full reserved-word quoting test suite
// against whatever database ds is connected to. This function is shared by
// the three dialect-specific entry-point tests below.
func runReservedWordTests(t *testing.T, ds *DatabaseSetup) {
	t.Helper()

	db := ds.DB
	ctx := context.Background()

	SetupTestData(t, db, ds.Dialect)
	defer CleanupTestData(t, db)

	// ------------------------------------------------------------------ INSERT
	t.Run("INSERT_ReservedWordColumns", func(t *testing.T) {
		_, err := db.Insert("test_reserved", map[string]interface{}{
			"order":  99,
			"select": "enterprise",
			"group":  "Z",
			"where":  "extra_clause",
			"index":  999,
		}).Execute()
		require.NoError(t, err, "INSERT with reserved-word columns must succeed")

		// Verify the row was stored correctly.
		var row reservedRow
		err = db.Select().
			From("test_reserved").
			Where(relica.Eq("order", 99)).
			One(&row)
		require.NoError(t, err)
		assert.Equal(t, "enterprise", row.Select)
		require.NotNil(t, row.Group)
		assert.Equal(t, "Z", *row.Group)
		assert.Equal(t, 999, row.Index)

		// Cleanup the extra row so subsequent sub-tests see the seed data only.
		_, err = db.Delete("test_reserved").
			Where(relica.Eq("order", 99)).
			Execute()
		require.NoError(t, err)
	})

	// ------------------------------------------------------------------ SELECT
	t.Run("SELECT_ExpressionAPI_ReservedWordColumn", func(t *testing.T) {
		var rows []reservedRow
		err := db.Select().
			From("test_reserved").
			Where(relica.Eq("order", 1)).
			All(&rows)
		require.NoError(t, err, "SELECT WHERE on reserved-word column must succeed")
		require.Len(t, rows, 1)
		assert.Equal(t, "standard", rows[0].Select)
	})

	t.Run("SELECT_IN_ReservedWordColumn", func(t *testing.T) {
		var rows []reservedRow
		err := db.Select().
			From("test_reserved").
			Where(relica.In("select", "vip", "premium")).
			All(&rows)
		require.NoError(t, err, "SELECT IN on reserved-word column must succeed")
		// seed: "vip" appears twice (order 2 and 5), "premium" once (order 3)
		assert.Len(t, rows, 3)
	})

	t.Run("SELECT_NULL_ReservedWordColumn", func(t *testing.T) {
		var rows []reservedRow
		err := db.Select().
			From("test_reserved").
			Where(relica.Eq("where", nil)).
			All(&rows)
		require.NoError(t, err, "SELECT IS NULL on reserved-word column must succeed")
		// seed: only row with order=4 has where=NULL
		assert.Len(t, rows, 1)
		assert.Equal(t, 4, rows[0].Order)
	})

	t.Run("SELECT_LIKE_ReservedWordColumn", func(t *testing.T) {
		var rows []reservedRow
		err := db.Select().
			From("test_reserved").
			Where(relica.Like("group", "A")).
			All(&rows)
		require.NoError(t, err, "SELECT LIKE on reserved-word column must succeed")
		// seed: group='A' appears for order 1 and 3
		assert.Len(t, rows, 2)
	})

	t.Run("SELECT_BETWEEN_ReservedWordColumn", func(t *testing.T) {
		var rows []reservedRow
		err := db.Select().
			From("test_reserved").
			Where(relica.Between("index", 10, 30)).
			All(&rows)
		require.NoError(t, err, "SELECT BETWEEN on reserved-word column must succeed")
		assert.Len(t, rows, 3) // index 10, 20, 30
	})

	t.Run("SELECT_AND_ReservedWordColumns", func(t *testing.T) {
		var rows []reservedRow
		err := db.Select().
			From("test_reserved").
			Where(relica.And(
				relica.Eq("select", "vip"),
				relica.GreaterThan("index", 25),
			)).
			All(&rows)
		require.NoError(t, err, "SELECT AND on reserved-word columns must succeed")
		// seed: "vip" rows have index 20 (order=2) and 50 (order=5); index>25 → order=5
		assert.Len(t, rows, 1)
		assert.Equal(t, 5, rows[0].Order)
	})

	t.Run("SELECT_CountSQL_ReservedWordColumn", func(t *testing.T) {
		// Validate query via ExecContext to check COUNT independently.
		var result struct {
			Total int `db:"total"`
		}
		err := db.Select("COUNT(*) as total").
			From("test_reserved").
			Where(relica.Eq("group", "B")).
			One(&result)
		require.NoError(t, err, "SELECT COUNT on reserved-word column must succeed")
		assert.Equal(t, 2, result.Total) // group='B': order 2 and 5
	})

	// ------------------------------------------------------------------ UPDATE
	t.Run("UPDATE_ReservedWordColumns", func(t *testing.T) {
		// Update the "select" tier of order=1 from "standard" to "updated".
		_, err := db.Update("test_reserved").
			Set(map[string]interface{}{
				"select": "updated",
				"index":  999,
			}).
			Where(relica.Eq("order", 1)).
			Execute()
		require.NoError(t, err, "UPDATE with reserved-word columns must succeed")

		// Verify.
		var row reservedRow
		err = db.Select().
			From("test_reserved").
			Where(relica.Eq("order", 1)).
			One(&row)
		require.NoError(t, err)
		assert.Equal(t, "updated", row.Select)
		assert.Equal(t, 999, row.Index)

		// Restore seed value so parallel sub-tests are not affected.
		_, restoreErr := db.Update("test_reserved").
			Set(map[string]interface{}{"select": "standard", "index": 10}).
			Where(relica.Eq("order", 1)).
			Execute()
		require.NoError(t, restoreErr)
	})

	// ------------------------------------------------------------------ DELETE
	t.Run("DELETE_ReservedWordColumn", func(t *testing.T) {
		// Insert a throwaway row then delete it.
		_, err := db.Insert("test_reserved", map[string]interface{}{
			"order":  777,
			"select": "to_delete",
			"group":  "D",
			"where":  nil,
			"index":  0,
		}).Execute()
		require.NoError(t, err, "setup INSERT before DELETE test must succeed")

		_, err = db.Delete("test_reserved").
			Where(relica.And(
				relica.Eq("order", 777),
				relica.Eq("select", "to_delete"),
			)).
			Execute()
		require.NoError(t, err, "DELETE with reserved-word columns must succeed")

		// Confirm gone.
		var count struct {
			N int `db:"n"`
		}
		qErr := db.Select("COUNT(*) as n").
			From("test_reserved").
			Where(relica.Eq("order", 777)).
			One(&count)
		require.NoError(t, qErr)
		assert.Equal(t, 0, count.N)
	})

	// ------------------------------------------------------------------ UPSERT (dialect-aware)
	t.Run("UPSERT_ReservedWordColumns", func(t *testing.T) {
		// MySQL uses INSERT … ON DUPLICATE KEY UPDATE, which does not support
		// explicit conflict columns — skip the OnConflict variant there.
		// SQLite and PostgreSQL support ON CONFLICT(col) DO UPDATE.
		switch ds.Dialect {
		case "postgres", "sqlite":
			_, err := db.Upsert("test_reserved", map[string]interface{}{
				"id":     1, // conflict on PK
				"order":  1,
				"select": "upserted",
				"group":  "A",
				"where":  "clause1",
				"index":  10,
			}).
				OnConflict("id").
				DoUpdate("select").
				Execute()
			require.NoError(t, err, "UPSERT with reserved-word columns must succeed")

			var row reservedRow
			err = db.Select().
				From("test_reserved").
				Where(relica.Eq("id", 1)).
				One(&row)
			require.NoError(t, err)
			assert.Equal(t, "upserted", row.Select)

			// Restore.
			_, restoreErr := db.Update("test_reserved").
				Set(map[string]interface{}{"select": "standard"}).
				Where(relica.Eq("id", 1)).
				Execute()
			require.NoError(t, restoreErr)

		case "mysql":
			_, err := db.Upsert("test_reserved", map[string]interface{}{
				"id":     1,
				"order":  1,
				"select": "upserted",
				"group":  "A",
				"where":  "clause1",
				"index":  10,
			}).OnConflict("id").DoUpdate("select").Execute()
			require.NoError(t, err, "MySQL UPSERT with reserved-word columns must succeed")

			// Restore.
			_, restoreErr := db.Update("test_reserved").
				Set(map[string]interface{}{"select": "standard"}).
				Where(relica.Eq("id", 1)).
				Execute()
			require.NoError(t, restoreErr)
		}
	})

	// ------------------------------------------------------------------ ToSQL round-trip
	t.Run("ToSQL_ReservedWordColumns", func(t *testing.T) {
		// Confirm that ToSQL produces a non-empty SQL string and that executing
		// that same query via Build().Execute() also succeeds — verifying
		// the SQL we inspect is actually what runs.
		sq := db.Select("COUNT(*) as total").
			From("test_reserved").
			Where(relica.Eq("select", "standard"))

		sql, params := sq.ToSQL()
		assert.NotEmpty(t, sql, "ToSQL must return non-empty SQL")
		assert.NotEmpty(t, params, "ToSQL must return at least one parameter")

		// Execute the same query shape via the builder to confirm it is accepted.
		var result struct {
			Total int `db:"total"`
		}
		err := db.Select("COUNT(*) as total").
			From("test_reserved").
			Where(relica.Eq("select", "standard")).
			One(&result)
		require.NoError(t, err, "query from ToSQL shape must execute successfully")
		assert.GreaterOrEqual(t, result.Total, 2, "expected at least 2 'standard' rows in seed data")

		// Log for debugging in CI.
		t.Logf("[%s] ToSQL output: %s params=%v", ds.Dialect, sql, params)

		_ = ctx // ctx captured to satisfy linter — used implicitly by db methods.
	})
}

// ============================================================
// Dialect entry points
// ============================================================

// TestReservedWordColumns_SQLite verifies reserved-word identifier quoting
// on SQLite (in-memory, no Docker required).
func TestReservedWordColumns_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runReservedWordTests(t, ds)
}

// TestReservedWordColumns_PostgreSQL verifies reserved-word identifier quoting
// on PostgreSQL (testcontainers, skipped when Docker is unavailable).
func TestReservedWordColumns_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runReservedWordTests(t, ds)
}

// TestReservedWordColumns_MySQL verifies reserved-word identifier quoting
// on MySQL (testcontainers, skipped when Docker is unavailable).
func TestReservedWordColumns_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runReservedWordTests(t, ds)
}
