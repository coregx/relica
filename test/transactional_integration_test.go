//go:build integration

package test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Transactional error reporting on real databases: the error that caused the
// rollback comes back unchanged and matchable, the rollback really happened,
// and sql.ErrTxDone from an already-ended transaction is not reported.

func createTxProbeTable(t *testing.T, db *relica.DB) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS tx_probe")
	_, err := db.ExecContext(ctx, "CREATE TABLE tx_probe (id INT PRIMARY KEY, note VARCHAR(50) NOT NULL)")
	require.NoError(t, err)
}

func txProbeCount(t *testing.T, db *relica.DB) int64 {
	t.Helper()
	n, err := db.Select().From("tx_probe").Count()
	require.NoError(t, err)
	return n
}

func runTransactionalTests(t *testing.T, db *relica.DB) {
	createTxProbeTable(t, db)
	reset := func(t *testing.T) {
		t.Helper()
		_, err := db.Delete("tx_probe").Execute()
		require.NoError(t, err)
	}
	insert := func(tx *relica.Tx, id int) error {
		_, err := tx.Insert("tx_probe", map[string]any{"id": id, "note": "x"}).Execute()
		return err
	}
	errWork := errors.New("work failed")

	t.Run("WorkError_RolledBackAndReturned", func(t *testing.T) {
		reset(t)
		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			if err := insert(tx, 1); err != nil {
				return err
			}
			return errWork
		})
		require.ErrorIs(t, err, errWork)
		assert.Zero(t, txProbeCount(t, db), "the insert must be rolled back")
	})

	// database/sql rolls a transaction back asynchronously when its context is
	// canceled, so either it or Transactional's deferred Rollback ends it
	// (the ErrTxDone filter itself is covered by a deterministic unit test).
	// Either way the caller sees context.Canceled and nothing survives.
	t.Run("CanceledContext_ReportsCanceledAndRollsBack", func(t *testing.T) {
		reset(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := db.Transactional(ctx, func(tx *relica.Tx) error {
			if err := insert(tx, 1); err != nil {
				return err
			}
			cancel()
			return insert(tx, 2)
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, sql.ErrTxDone, "the deferred rollback of an ended tx is not an error")
		assert.Zero(t, txProbeCount(t, db), "nothing from the canceled transaction may survive")
	})

	t.Run("TransactionalTx_WorkError", func(t *testing.T) {
		reset(t)
		err := db.TransactionalTx(context.Background(), &relica.TxOptions{}, func(tx *relica.Tx) error {
			if err := insert(tx, 1); err != nil {
				return err
			}
			return errWork
		})
		require.ErrorIs(t, err, errWork)
		assert.Zero(t, txProbeCount(t, db))
	})

	t.Run("Success_Commits", func(t *testing.T) {
		reset(t)
		err := db.Transactional(context.Background(), func(tx *relica.Tx) error { return insert(tx, 1) })
		require.NoError(t, err)
		assert.Equal(t, int64(1), txProbeCount(t, db))
	})
}

// ---------------------------------------------------------------------------
// Entry points — names must contain PostgreSQL / MySQL / SQLite for CI filters.
// ---------------------------------------------------------------------------

func TestTransactionalErrors_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runTransactionalTests(t, ds.DB)
}

func TestTransactionalErrors_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runTransactionalTests(t, ds.DB)
}

func TestTransactionalErrors_SQLite(t *testing.T) {
	db, err := relica.NewDB("sqlite", filepath.Join(t.TempDir(), "tx.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runTransactionalTests(t, db)
}
