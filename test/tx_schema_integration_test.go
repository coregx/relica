//go:build integration
// +build integration

// Package test contains integration tests for the Relica query builder.
// This file covers:
//   - P2: Tx symmetry — BatchInsert, Upsert available on *Tx just like on *DB
//   - P3: schema-qualified table names (PostgreSQL only)
//   - P3: named parameters using Params{} across all dialects
package test

import (
	"context"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Tx symmetry — SQLite (always runs, no Docker)
// ============================================================================

// TestTxSymmetry_SQLite verifies that *Tx exposes the same convenience
// methods as *DB (BatchInsert, Upsert) and that committed transactions
// persist while rolled-back transactions leave no trace.
func TestTxSymmetry_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	SetupTestData(t, ds.DB, ds.Dialect)
	defer CleanupTestData(t, ds.DB)

	runTxSymmetryTests(t, ds)
}

// ============================================================================
// Tx symmetry — PostgreSQL
// ============================================================================

// TestTxSymmetry_PostgreSQL verifies Tx symmetry against a real PostgreSQL instance.
func TestTxSymmetry_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()

	SetupTestData(t, ds.DB, ds.Dialect)
	defer CleanupTestData(t, ds.DB)

	runTxSymmetryTests(t, ds)
}

// ============================================================================
// Tx symmetry — MySQL
// ============================================================================

// TestTxSymmetry_MySQL verifies Tx symmetry against a real MySQL instance.
func TestTxSymmetry_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()

	SetupTestData(t, ds.DB, ds.Dialect)
	defer CleanupTestData(t, ds.DB)

	runTxSymmetryTests(t, ds)
}

// runTxSymmetryTests is the shared test body executed for every dialect.
// It verifies that Tx.BatchInsert and Tx.Upsert behave identically to their
// DB-level counterparts and that commit/rollback semantics are correct.
func runTxSymmetryTests(t *testing.T, ds *DatabaseSetup) {
	t.Helper()

	db := ds.DB
	ctx := context.Background()

	// ---- BatchInsert inside a committed transaction ----
	t.Run("BatchInsert_Committed", func(t *testing.T) {
		// Count companies before the transaction.
		var before struct {
			N int `db:"n"`
		}
		err := db.Select("COUNT(*) AS n").From("test_companies").One(&before)
		require.NoError(t, err)

		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback()

		_, err = tx.BatchInsert("test_companies", []string{"name", "status"}).
			Values("TxCo Alpha", "active").
			Values("TxCo Beta", "active").
			Values("TxCo Gamma", "inactive").
			Execute()
		require.NoError(t, err, "Tx.BatchInsert must succeed (%s)", ds.Dialect)

		// Rows must be visible within the same transaction.
		var duringTx struct {
			N int `db:"n"`
		}
		err = tx.Select("COUNT(*) AS n").
			From("test_companies").
			Where(relica.Like("name", "TxCo")).
			One(&duringTx)
		require.NoError(t, err)
		assert.Equal(t, 3, duringTx.N, "3 TxCo rows must be visible within the open transaction")

		require.NoError(t, tx.Commit())

		// After commit, TxCo rows must persist for other connections.
		var afterTxCo struct {
			N int `db:"n"`
		}
		err = db.Select("COUNT(*) AS n").
			From("test_companies").
			Where(relica.Like("name", "TxCo")).
			One(&afterTxCo)
		require.NoError(t, err)
		assert.Equal(t, 3, afterTxCo.N, "committed BatchInsert rows must persist (%s)", ds.Dialect)

		// Total company count must equal seed + 3 new rows.
		var afterTotal struct {
			N int `db:"n"`
		}
		err = db.Select("COUNT(*) AS n").From("test_companies").One(&afterTotal)
		require.NoError(t, err)
		assert.Equal(t, before.N+3, afterTotal.N,
			"total company count must increase by 3 after commit (%s)", ds.Dialect)
	})

	// ---- Upsert (insert path + conflict path) inside a transaction ----
	t.Run("Upsert_InsertAndConflict_Committed", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback()

		// Insert path — SKU does not exist yet.
		_, err = tx.Upsert("test_products", map[string]interface{}{
			"sku":      "SKU-TX-SYMM-001",
			"name":     "Tx Symmetry Widget",
			"price":    4444,
			"category": "tx-test",
		}).OnConflict("sku").DoUpdate("price", "name").Execute()
		require.NoError(t, err, "Tx.Upsert (insert path) must succeed (%s)", ds.Dialect)

		// Conflict path — same SKU, updated values.
		_, err = tx.Upsert("test_products", map[string]interface{}{
			"sku":      "SKU-TX-SYMM-001",
			"name":     "Tx Symmetry Widget v2",
			"price":    8888,
			"category": "tx-test",
		}).OnConflict("sku").DoUpdate("price", "name").Execute()
		require.NoError(t, err, "Tx.Upsert (conflict path) must succeed (%s)", ds.Dialect)

		require.NoError(t, tx.Commit())

		// Verify the conflict path result persists.
		var p struct {
			Name  string `db:"name"`
			Price int    `db:"price"`
		}
		err = db.Select("name", "price").
			From("test_products").
			Where(relica.Eq("sku", "SKU-TX-SYMM-001")).
			One(&p)
		require.NoError(t, err)
		assert.Equal(t, "Tx Symmetry Widget v2", p.Name)
		assert.Equal(t, 8888, p.Price)
	})

	// ---- Rollback must leave no trace ----
	t.Run("BatchInsert_RolledBack", func(t *testing.T) {
		var before struct {
			N int `db:"n"`
		}
		err := db.Select("COUNT(*) AS n").From("test_companies").One(&before)
		require.NoError(t, err)

		tx, err := db.Begin(ctx)
		require.NoError(t, err)

		_, err = tx.BatchInsert("test_companies", []string{"name", "status"}).
			Values("Ghost Co", "active").
			Execute()
		require.NoError(t, err)

		require.NoError(t, tx.Rollback())

		var after struct {
			N int `db:"n"`
		}
		err = db.Select("COUNT(*) AS n").From("test_companies").One(&after)
		require.NoError(t, err)
		assert.Equal(t, before.N, after.N, "rollback must leave total count unchanged (%s)", ds.Dialect)
	})

	// ---- Transactional helper (auto commit/rollback) ----
	t.Run("Transactional_Helper_Commit", func(t *testing.T) {
		var before struct {
			N int `db:"n"`
		}
		err := db.Select("COUNT(*) AS n").From("test_employees").One(&before)
		require.NoError(t, err)

		err = db.Transactional(ctx, func(tx *relica.Tx) error {
			_, insertErr := tx.Insert("test_employees", map[string]interface{}{
				"company_id": 1,
				"name":       "Transactional Worker",
				"role":       "engineer",
				"salary":     70000,
			}).Execute()
			return insertErr
		})
		require.NoError(t, err, "Transactional helper must succeed (%s)", ds.Dialect)

		var after struct {
			N int `db:"n"`
		}
		err = db.Select("COUNT(*) AS n").From("test_employees").One(&after)
		require.NoError(t, err)
		assert.Equal(t, before.N+1, after.N, "Transactional helper must commit its inserts (%s)", ds.Dialect)
	})
}

// ============================================================================
// Schema-qualified table names (PostgreSQL only — P3 fix)
// ============================================================================

// TestSchemaQualifiedTable_PostgreSQL verifies that Relica correctly handles
// schema-qualified table references ("schema.table" and "schema.table alias")
// without quoting or SQL generation errors.
func TestSchemaQualifiedTable_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()

	ctx := context.Background()
	db := ds.DB

	// Create an isolated schema and table for this test.
	_, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS relica_test`)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Best-effort schema cleanup — failure is not a test error.
		_, _ = db.ExecContext(ctx, `DROP SCHEMA IF EXISTS relica_test CASCADE`)
	})

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS relica_test.accounts (
			id      SERIAL PRIMARY KEY,
			owner   TEXT    NOT NULL,
			balance INTEGER NOT NULL DEFAULT 0
		)
	`)
	require.NoError(t, err)

	// Seed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO relica_test.accounts (owner, balance) VALUES
			('alice', 1000),
			('bob',   2500),
			('carol', 500)
	`)
	require.NoError(t, err)

	t.Run("Select_SchemaTable_WithAlias", func(t *testing.T) {
		var results []struct {
			Owner   string `db:"owner"`
			Balance int    `db:"balance"`
		}

		err := db.Select("a.owner", "a.balance").
			From("relica_test.accounts a").
			Where(relica.GreaterThan("a.balance", 600)).
			OrderBy("a.balance ASC").
			All(&results)

		require.NoError(t, err, "SELECT from schema-qualified table with alias must succeed")
		assert.Len(t, results, 2, "expected alice (1000) and bob (2500)")
		assert.Equal(t, "alice", results[0].Owner)
		assert.Equal(t, "bob", results[1].Owner)
	})

	t.Run("Select_SchemaTable_ExpressionWhere", func(t *testing.T) {
		var results []struct {
			Owner string `db:"owner"`
		}

		err := db.Select("owner").
			From("relica_test.accounts").
			Where(relica.And(
				relica.GreaterOrEqual("balance", 1000),
				relica.NotEq("owner", "carol"),
			)).
			All(&results)

		require.NoError(t, err, "schema-qualified table with Expression API WHERE must succeed")
		assert.Len(t, results, 2)
	})

	t.Run("Insert_SchemaTable", func(t *testing.T) {
		_, err := db.Insert("relica_test.accounts", map[string]interface{}{
			"owner":   "dave",
			"balance": 3000,
		}).Execute()

		require.NoError(t, err, "INSERT into schema-qualified table must succeed")

		var count struct {
			N int `db:"n"`
		}
		err = db.Select("COUNT(*) AS n").
			From("relica_test.accounts").
			Where(relica.Eq("owner", "dave")).
			One(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count.N)
	})

	t.Run("Update_SchemaTable", func(t *testing.T) {
		result, err := db.Update("relica_test.accounts").
			Set(map[string]interface{}{"balance": 9999}).
			Where(relica.Eq("owner", "alice")).
			Execute()

		require.NoError(t, err, "UPDATE on schema-qualified table must succeed")
		rows, _ := result.RowsAffected()
		assert.EqualValues(t, 1, rows)
	})

	t.Run("Join_SchemaTable_WithRegularTable", func(t *testing.T) {
		// This tests that schema-qualified tables work correctly in JOIN context.
		// We create a second table in the same schema to join with.
		_, err := db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS relica_test.transactions (
				id         SERIAL PRIMARY KEY,
				account_id INTEGER NOT NULL,
				amount     INTEGER NOT NULL
			)
		`)
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `
			INSERT INTO relica_test.transactions (account_id, amount) VALUES
				(1, 100), (1, 200), (2, 500)
		`)
		require.NoError(t, err)

		var results []struct {
			Owner string `db:"owner"`
			Total int    `db:"total"`
		}

		err = db.Select("a.owner", "SUM(t.amount) AS total").
			From("relica_test.accounts a").
			InnerJoin("relica_test.transactions t", "t.account_id = a.id").
			GroupBy("a.owner").
			OrderBy("a.owner ASC").
			All(&results)

		require.NoError(t, err, "JOIN between two schema-qualified tables must succeed")
		assert.Len(t, results, 2, "expected alice and bob only (carol has no transactions)")
	})
}

// ============================================================================
// Named parameters (Params{}) — all dialects
// ============================================================================

// TestNamedParams_SQLite verifies Params{} named placeholder syntax on SQLite.
func TestNamedParams_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	SetupTestData(t, ds.DB, ds.Dialect)
	defer CleanupTestData(t, ds.DB)

	runNamedParamTests(t, ds)
}

// TestNamedParams_PostgreSQL verifies Params{} named placeholder syntax on PostgreSQL.
func TestNamedParams_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()

	SetupTestData(t, ds.DB, ds.Dialect)
	defer CleanupTestData(t, ds.DB)

	runNamedParamTests(t, ds)
}

// TestNamedParams_MySQL verifies Params{} named placeholder syntax on MySQL.
func TestNamedParams_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()

	SetupTestData(t, ds.DB, ds.Dialect)
	defer CleanupTestData(t, ds.DB)

	runNamedParamTests(t, ds)
}

// runNamedParamTests is the shared test body for all Params{} named parameter tests.
func runNamedParamTests(t *testing.T, ds *DatabaseSetup) {
	t.Helper()

	db := ds.DB

	// ---- Simple single-param WHERE ----
	t.Run("SingleParam_Status", func(t *testing.T) {
		var results []struct {
			Name   string `db:"name"`
			Status string `db:"status"`
		}

		err := db.Select("name", "status").
			From("test_companies").
			Where("status = {:status}", relica.Params{"status": "active"}).
			OrderBy("id ASC").
			All(&results)

		require.NoError(t, err, "single Params{} named placeholder must succeed (%s)", ds.Dialect)
		assert.NotEmpty(t, results)
		for _, r := range results {
			assert.Equal(t, "active", r.Status, "only active companies expected")
		}
	})

	// ---- Multiple params in one WHERE clause ----
	t.Run("MultiParam_SalaryRange", func(t *testing.T) {
		var results []struct {
			Name   string `db:"name"`
			Salary int    `db:"salary"`
		}

		err := db.Select("name", "salary").
			From("test_employees").
			Where("salary >= {:min} AND salary <= {:max}", relica.Params{
				"min": 85000,
				"max": 110000,
			}).
			OrderBy("salary ASC").
			All(&results)

		require.NoError(t, err, "multi-param salary range query must succeed (%s)", ds.Dialect)
		assert.NotEmpty(t, results)
		for _, r := range results {
			assert.GreaterOrEqual(t, r.Salary, 85000)
			assert.LessOrEqual(t, r.Salary, 110000)
		}
	})

	// ---- Named params chained across AndWhere ----
	t.Run("ChainedNamedParams", func(t *testing.T) {
		var results []struct {
			Name   string `db:"name"`
			Role   string `db:"role"`
			Salary int    `db:"salary"`
		}

		err := db.Select("name", "role", "salary").
			From("test_employees").
			Where("role = {:role}", relica.Params{"role": "engineer"}).
			Where("salary > {:min_salary}", relica.Params{"min_salary": 80000}).
			OrderBy("salary DESC").
			All(&results)

		require.NoError(t, err, "chained named params must succeed (%s)", ds.Dialect)
		assert.NotEmpty(t, results)
		for _, r := range results {
			assert.Equal(t, "engineer", r.Role)
			assert.Greater(t, r.Salary, 80000)
		}
	})

	// ---- Named params in UPDATE ----
	t.Run("NamedParams_Update", func(t *testing.T) {
		// Use a unique SKU so this test is self-contained.
		_, err := db.Insert("test_products", map[string]interface{}{
			"sku":      "SKU-NAMED-PARAM-TEST",
			"name":     "Named Param Product",
			"price":    1111,
			"category": "test",
		}).Execute()
		require.NoError(t, err, "setup INSERT before named-param UPDATE must succeed")

		result, err := db.Update("test_products").
			Set(map[string]interface{}{"price": 2222}).
			Where("sku = {:sku}", relica.Params{"sku": "SKU-NAMED-PARAM-TEST"}).
			Execute()

		require.NoError(t, err, "named params in UPDATE WHERE must succeed (%s)", ds.Dialect)
		rows, _ := result.RowsAffected()
		assert.EqualValues(t, 1, rows)

		// Verify.
		var product struct {
			Price int `db:"price"`
		}
		err = db.Select("price").
			From("test_products").
			Where(relica.Eq("sku", "SKU-NAMED-PARAM-TEST")).
			One(&product)
		require.NoError(t, err)
		assert.Equal(t, 2222, product.Price)
	})
}
