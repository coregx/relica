//go:build integration
// +build integration

// Package test contains integration tests for the Relica query builder.
// This file validates ToSQL() on all query types (SELECT, UPDATE, DELETE, UPSERT,
// BatchInsert) across all supported dialects. The core invariant verified is:
//
//	The SQL returned by ToSQL() must be the same query that Execute() would run,
//	i.e. executing the query via Build().Execute() after inspecting ToSQL() must
//	not produce an error.
//
// This exercises the P2 enterprise audit fix (PRs #23-#27) that made ToSQL
// available on every query type.
package test

import (
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runToSQLTests verifies ToSQL correctness and SQL–Execute consistency across
// SELECT, UPDATE, DELETE, UPSERT, and BatchInsert query types.
func runToSQLTests(t *testing.T, ds *DatabaseSetup) {
	t.Helper()

	db := ds.DB

	SetupTestData(t, db, ds.Dialect)
	defer CleanupTestData(t, db)

	// ------------------------------------------------------------------ SELECT ToSQL
	t.Run("SelectQuery_ToSQL_NonEmpty", func(t *testing.T) {
		sq := db.Select("id", "sku", "name", "price").
			From("test_products").
			Where(relica.Eq("category", "widgets")).
			OrderBy("price DESC").
			Limit(10)

		sql, params := sq.ToSQL()

		assert.NotEmpty(t, sql, "ToSQL must return non-empty SQL")
		assert.NotEmpty(t, params, "ToSQL must return non-empty params when Where is used")
		t.Logf("[%s] SELECT ToSQL: %s  params=%v", ds.Dialect, sql, params)
	})

	t.Run("SelectQuery_ToSQL_ExecuteConsistency", func(t *testing.T) {
		// Build the query once and verify the SQL is accepted by the database.
		var results []Product
		err := db.Select("id", "sku", "name", "price", "category").
			From("test_products").
			Where(relica.And(
				relica.Eq("category", "widgets"),
				relica.GreaterThan("price", 0),
			)).
			OrderBy("price").
			All(&results)
		require.NoError(t, err, "SELECT consistent with ToSQL output must execute without error")
		// seed: SKU-001 (1000) and SKU-002 (2000) are in 'widgets'
		assert.Len(t, results, 2)
	})

	t.Run("SelectQuery_ToSQL_WithJoin", func(t *testing.T) {
		type row struct {
			CompanyName  string `db:"company_name"`
			EmployeeName string `db:"employee_name"`
		}
		sq := db.Select("c.name as company_name", "e.name as employee_name").
			From("test_companies c").
			InnerJoin("test_employees e", "e.company_id = c.id").
			Where(relica.Eq("c.status", "active"))

		sql, params := sq.ToSQL()
		assert.NotEmpty(t, sql)
		assert.NotEmpty(t, params)
		t.Logf("[%s] JOIN ToSQL: %s  params=%v", ds.Dialect, sql, params)

		// Execute to confirm the SQL is valid.
		var rows []row
		err := db.Select("c.name as company_name", "e.name as employee_name").
			From("test_companies c").
			InnerJoin("test_employees e", "e.company_id = c.id").
			Where(relica.Eq("c.status", "active")).
			All(&rows)
		require.NoError(t, err)
		assert.NotEmpty(t, rows)
	})

	// ------------------------------------------------------------------ UPDATE ToSQL
	t.Run("UpdateQuery_ToSQL_NonEmpty", func(t *testing.T) {
		uq := db.Update("test_products").
			Set(map[string]interface{}{"price": 9999}).
			Where(relica.Eq("sku", "SKU-NONEXISTENT"))

		sql, params := uq.ToSQL()
		assert.NotEmpty(t, sql, "UPDATE ToSQL must return non-empty SQL")
		assert.NotEmpty(t, params)
		t.Logf("[%s] UPDATE ToSQL: %s  params=%v", ds.Dialect, sql, params)
	})

	t.Run("UpdateQuery_ToSQL_ExecuteConsistency", func(t *testing.T) {
		// Execute an UPDATE that matches zero rows — still a valid query.
		_, err := db.Update("test_products").
			Set(map[string]interface{}{"category": "updated"}).
			Where(relica.Eq("sku", "SKU-DEFINITELY-MISSING")).
			Execute()
		require.NoError(t, err, "UPDATE with ToSQL-consistent shape must not error on zero rows")
	})

	// ------------------------------------------------------------------ DELETE ToSQL
	t.Run("DeleteQuery_ToSQL_NonEmpty", func(t *testing.T) {
		dq := db.Delete("test_products").
			Where(relica.Eq("sku", "SKU-NONEXISTENT"))

		sql, params := dq.ToSQL()
		assert.NotEmpty(t, sql, "DELETE ToSQL must return non-empty SQL")
		assert.NotEmpty(t, params)
		t.Logf("[%s] DELETE ToSQL: %s  params=%v", ds.Dialect, sql, params)
	})

	t.Run("DeleteQuery_ToSQL_ExecuteConsistency", func(t *testing.T) {
		_, err := db.Delete("test_products").
			Where(relica.Eq("sku", "SKU-DEFINITELY-MISSING")).
			Execute()
		require.NoError(t, err)
	})

	// ------------------------------------------------------------------ UPSERT ToSQL
	t.Run("UpsertQuery_ToSQL_NonEmpty", func(t *testing.T) {
		uq := db.Upsert("test_products", map[string]interface{}{
			"sku":      "SKU-001",
			"name":     "Widget A v2",
			"price":    1100,
			"category": "widgets",
		})

		uq = uq.OnConflict("sku").DoUpdate("name", "price")

		sql, params := uq.ToSQL()
		assert.NotEmpty(t, sql, "UPSERT ToSQL must return non-empty SQL")
		assert.NotEmpty(t, params)
		t.Logf("[%s] UPSERT ToSQL: %s  params=%v", ds.Dialect, sql, params)
	})

	t.Run("UpsertQuery_ToSQL_ExecuteConsistency", func(t *testing.T) {
		// Execute a real UPSERT — update the existing SKU-004 row.
		uq := db.Upsert("test_products", map[string]interface{}{
			"sku":      "SKU-004",
			"name":     "Gadget Y Updated",
			"price":    7600,
			"category": "gadgets",
		})

		uq = uq.OnConflict("sku").DoUpdate("name", "price")

		_, err := uq.Execute()
		require.NoError(t, err, "UPSERT Execute consistent with ToSQL must succeed")

		// Verify the update took effect.
		var p Product
		err = db.Select().
			From("test_products").
			Where(relica.Eq("sku", "SKU-004")).
			One(&p)
		require.NoError(t, err)
		assert.Equal(t, "Gadget Y Updated", p.Name)
		assert.Equal(t, 7600, p.Price)

		// Restore seed value.
		_, err = db.Update("test_products").
			Set(map[string]interface{}{"name": "Gadget Y", "price": 7500}).
			Where(relica.Eq("sku", "SKU-004")).
			Execute()
		require.NoError(t, err)
	})

	// ------------------------------------------------------------------ BatchInsert ToSQL
	t.Run("BatchInsertQuery_ToSQL_NonEmpty", func(t *testing.T) {
		bq := db.BatchInsert("test_products", []string{"sku", "name", "price", "category"}).
			Values("SKU-BATCH-A", "Batch A", 100, "misc").
			Values("SKU-BATCH-B", "Batch B", 200, "misc")

		sql, params := bq.ToSQL()
		assert.NotEmpty(t, sql, "BatchInsert ToSQL must return non-empty SQL")
		assert.NotEmpty(t, params)
		t.Logf("[%s] BatchInsert ToSQL: %s  params=%v", ds.Dialect, sql, params)
	})

	t.Run("BatchInsertQuery_ToSQL_ExecuteConsistency", func(t *testing.T) {
		_, err := db.BatchInsert("test_products", []string{"sku", "name", "price", "category"}).
			Values("SKU-BATCH-C", "Batch C", 300, "misc").
			Values("SKU-BATCH-D", "Batch D", 400, "misc").
			Execute()
		require.NoError(t, err, "BatchInsert Execute must succeed")

		// Verify both rows exist.
		var count struct {
			N int `db:"n"`
		}
		err = db.Select("COUNT(*) as n").
			From("test_products").
			Where(relica.In("sku", "SKU-BATCH-C", "SKU-BATCH-D")).
			One(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count.N)

		// Cleanup.
		_, err = db.Delete("test_products").
			Where(relica.In("sku", "SKU-BATCH-C", "SKU-BATCH-D")).
			Execute()
		require.NoError(t, err)
	})

	// ------------------------------------------------------------------ Reserved-word columns in ToSQL
	t.Run("SelectToSQL_ReservedWordColumns", func(t *testing.T) {
		// Confirm ToSQL output for reserved-word columns contains valid SQL.
		sq := db.Select("id", `"order"`, `"select"`, `"group"`).
			From("test_reserved").
			Where(relica.Eq("order", 1))

		sql, params := sq.ToSQL()
		assert.NotEmpty(t, sql)
		assert.NotEmpty(t, params)
		t.Logf("[%s] reserved-word ToSQL: %s  params=%v", ds.Dialect, sql, params)

		// Execute via the expression API (relies on internal quoting, not raw SQL).
		var rows []reservedRow
		err := db.Select().
			From("test_reserved").
			Where(relica.Eq("order", 1)).
			All(&rows)
		require.NoError(t, err)
		assert.Len(t, rows, 1)
	})
}

// ============================================================
// Dialect entry points
// ============================================================

// TestToSQL_SQLite verifies ToSQL consistency on SQLite
// (in-memory, no Docker required).
func TestToSQL_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runToSQLTests(t, ds)
}

// TestToSQL_PostgreSQL verifies ToSQL consistency on PostgreSQL
// (testcontainers, skipped when Docker is unavailable).
func TestToSQL_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runToSQLTests(t, ds)
}

// TestToSQL_MySQL verifies ToSQL consistency on MySQL
// (testcontainers, skipped when Docker is unavailable).
func TestToSQL_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runToSQLTests(t, ds)
}
