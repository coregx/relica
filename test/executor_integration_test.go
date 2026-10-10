//go:build integration
// +build integration

package test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file exercises relica.Executor against real databases. Every scenario
// runs on PostgreSQL, MySQL and SQLite through the three entry points at the
// bottom, which match the -run filters used by CI.
//
// The negative cases reproduce the failure modes that motivated the
// interface: a dbcontext.With(ctx) that ignores the transaction in the
// context (the gridex/GODE bug), a nested Begin on an exhausted pool (the
// GODE deadlock), a canceled context surfacing as ErrNotFound, and an
// Executor used after its transaction ended.

// ---------------------------------------------------------------------------
// Repository code under test: sees only relica.Executor, never *DB or *Tx.
// ---------------------------------------------------------------------------

// The ctx parameter is accepted for the usual repository signature; the
// Executor itself already carries the context — a *Tx is bound to the ctx of
// Begin, a *DB came from db.WithContext(ctx) — so queries do not re-bind it.
func insertRow(_ context.Context, ex relica.Executor, name string, value int) error {
	_, err := ex.Insert("tx_test", map[string]any{"name": name, "value": value}).Execute()
	return err
}

func countRows(_ context.Context, ex relica.Executor) (int64, error) {
	return ex.Select().From("tx_test").Count()
}

func countNamed(_ context.Context, ex relica.Executor, name string) (int64, error) {
	return ex.Select().From("tx_test").Where(relica.Eq("name", name)).Count()
}

// execRow is a Model API row bound to the shared tx_test table.
type execRow struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Value int    `db:"value"`
}

func (execRow) TableName() string { return "tx_test" }

// ---------------------------------------------------------------------------
// dbcontext pattern (go-rest-api / ozzo-dbx), expressed with relica.Executor.
// ---------------------------------------------------------------------------

type txCtxKey struct{}

type dbContext struct{ db *relica.DB }

// With returns the transaction stored in ctx, otherwise the connection.
func (d dbContext) With(ctx context.Context) relica.Executor {
	if tx, ok := ctx.Value(txCtxKey{}).(*relica.Tx); ok {
		return tx
	}
	return d.db.WithContext(ctx)
}

// Transactional joins an already-open transaction instead of nesting one.
func (d dbContext) Transactional(ctx context.Context, f func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txCtxKey{}).(*relica.Tx); ok {
		return f(ctx)
	}
	return d.db.Transactional(ctx, func(tx *relica.Tx) error {
		return f(context.WithValue(ctx, txCtxKey{}, tx))
	})
}

// wrongWith is the gridex implementation: it never looks at ctx, so every
// write it makes runs with autocommit on a pool connection and survives the
// surrounding rollback. Kept here as a regression oracle for that bug class.
func wrongWith(ctx context.Context, db *relica.DB) relica.Executor {
	return db.WithContext(ctx)
}

// ---------------------------------------------------------------------------
// Pool helpers for the exhaustion scenarios.
// ---------------------------------------------------------------------------

// limitPoolToOne shrinks the pool to a single connection and returns a
// restore func. Idle connections are dropped first: database/sql hands out
// free connections before it checks MaxOpenConns, so leftover idle
// connections would let a second Begin succeed and hide the exhaustion.
func limitPoolToOne(db *relica.DB) func() {
	raw := db.SqlDB()
	raw.SetMaxIdleConns(0)
	raw.SetMaxOpenConns(1)
	return func() {
		raw.SetMaxOpenConns(0)
		raw.SetMaxIdleConns(2)
	}
}

// ---------------------------------------------------------------------------
// Scenarios
// ---------------------------------------------------------------------------

func runExecutorTests(t *testing.T, ds *DatabaseSetup) {
	db := ds.DB
	ctx := context.Background()
	reset := func(t *testing.T) {
		t.Helper()
		_, err := db.ExecContext(ctx, "DELETE FROM tx_test")
		require.NoError(t, err)
	}

	createTxTestTable(t, db, ds.Dialect)
	defer dropTxTestTable(t, db)

	// Positive: the same repository function commits through *DB and *Tx.
	t.Run("Commit_ViaExecutor", func(t *testing.T) {
		reset(t)

		require.NoError(t, insertRow(ctx, db, "via-db", 1))
		require.NoError(t, db.Transactional(ctx, func(tx *relica.Tx) error {
			return insertRow(ctx, tx, "via-tx", 2)
		}))

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)
	})

	// Negative: an error from the callback discards writes made through the
	// Executor. This is the guarantee gridex lost.
	t.Run("Rollback_DiscardsExecutorWrites", func(t *testing.T) {
		reset(t)
		boom := errors.New("abort")

		err := db.Transactional(ctx, func(tx *relica.Tx) error {
			var ex relica.Executor = tx
			if err := insertRow(ctx, ex, "gone", 1); err != nil {
				return err
			}
			return boom
		})
		require.ErrorIs(t, err, boom)

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(0), n, "write through Executor(tx) must not survive rollback")
	})

	// Negative: a panic rolls back Executor writes and is re-raised.
	t.Run("Panic_RollsBackExecutorWrites", func(t *testing.T) {
		reset(t)

		assert.PanicsWithValue(t, "boom", func() {
			_ = db.Transactional(ctx, func(tx *relica.Tx) error {
				var ex relica.Executor = tx
				require.NoError(t, insertRow(ctx, ex, "gone", 1))
				panic("boom")
			})
		})

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(0), n)
	})

	// Negative (regression oracle): With(ctx) that ignores the context writes
	// with autocommit on another pool connection. The row outlives the
	// rollback while the correctly routed row does not. If this test ever
	// starts failing, something made autocommit writes transactional — which
	// would be a far bigger surprise than the bug itself.
	t.Run("WrongWith_LeaksWriteThroughRollback", func(t *testing.T) {
		reset(t)
		boom := errors.New("abort")

		err := db.Transactional(ctx, func(tx *relica.Tx) error {
			if err := insertRow(ctx, wrongWith(ctx, db), "leaked", 1); err != nil {
				return err
			}
			if err := insertRow(ctx, tx, "correct", 2); err != nil {
				return err
			}
			return boom
		})
		require.ErrorIs(t, err, boom)

		leaked, err := countNamed(ctx, db, "leaked")
		require.NoError(t, err)
		assert.Equal(t, int64(1), leaked, "autocommit write bypassed the transaction")

		correct, err := countNamed(ctx, db, "correct")
		require.NoError(t, err)
		assert.Equal(t, int64(0), correct, "write through the real tx was rolled back")
	})

	// Positive: the dbcontext pattern routes repository writes into the
	// transaction and rolls them back together.
	t.Run("DBContext_RoutesWritesIntoTransaction", func(t *testing.T) {
		reset(t)
		dc := dbContext{db: db}
		boom := errors.New("abort")

		_, isDB := dc.With(ctx).(*relica.DB)
		assert.True(t, isDB, "outside Transactional With must return the connection")

		err := dc.Transactional(ctx, func(txCtx context.Context) error {
			ex := dc.With(txCtx)
			_, isTx := ex.(*relica.Tx)
			require.True(t, isTx, "inside Transactional With must return the stored tx")
			if err := insertRow(txCtx, ex, "repo-write", 1); err != nil {
				return err
			}
			return boom
		})
		require.ErrorIs(t, err, boom)

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(0), n)

		require.NoError(t, dc.Transactional(ctx, func(txCtx context.Context) error {
			return insertRow(txCtx, dc.With(txCtx), "repo-write", 1)
		}))
		n, err = countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)
	})

	// Positive: with a single-connection pool, nested Transactional calls
	// that join the outer transaction complete and commit exactly once.
	t.Run("NestedTransactional_JoinsOnPoolOfOne", func(t *testing.T) {
		reset(t)
		restore := limitPoolToOne(db)
		defer restore()
		dc := dbContext{db: db}

		deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := dc.Transactional(deadline, func(outer context.Context) error {
			if err := insertRow(outer, dc.With(outer), "outer", 1); err != nil {
				return err
			}
			return dc.Transactional(outer, func(inner context.Context) error {
				return insertRow(inner, dc.With(inner), "inner", 2)
			})
		})
		require.NoError(t, err, "joined nested transaction must not wait for a second connection")

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)
	})

	// Negative: the GODE deadlock. A second Begin on a one-connection pool
	// blocks until the context deadline instead of returning a transaction.
	t.Run("NestedBegin_OnPoolOfOne_TimesOut", func(t *testing.T) {
		reset(t)
		restore := limitPoolToOne(db)
		defer restore()

		outer, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = outer.Rollback() }()

		short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()

		start := time.Now()
		inner, err := db.Begin(short)
		elapsed := time.Since(start)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, inner)
		assert.GreaterOrEqual(t, elapsed, 400*time.Millisecond, "Begin must have waited for a connection")
	})

	// Negative: a canceled context fails the query with context.Canceled.
	// It must never be reported as "row not found".
	t.Run("CanceledContext_IsNotErrNotFound", func(t *testing.T) {
		reset(t)
		require.NoError(t, insertRow(ctx, db, "present", 1))

		canceled, cancel := context.WithCancel(ctx)
		cancel()

		var ex relica.Executor = db.WithContext(canceled)
		var row execRow
		err := ex.Select("id", "name", "value").From("tx_test").One(&row)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, relica.ErrNotFound, "infrastructure failure masked as not-found")
	})

	// Negative: the same guarantee for writes, through both ways of attaching
	// a context — on the Executor (DB.WithContext) and on the query itself
	// (Query.WithContext). Nothing may reach the table.
	t.Run("CanceledContext_BlocksWrites", func(t *testing.T) {
		reset(t)

		canceled, cancel := context.WithCancel(ctx)
		cancel()

		var ex relica.Executor = db.WithContext(canceled)
		err := insertRow(canceled, ex, "via-executor-ctx", 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)

		_, err = db.Insert("tx_test", map[string]any{"name": "via-query-ctx", "value": 2}).
			WithContext(canceled).Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)

		err = db.Model(&execRow{Name: "via-model-ctx", Value: 3}).WithContext(canceled).Insert()
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(0), n, "no write may slip through a canceled context")
	})

	// Negative: an Executor backed by a finished transaction fails fast.
	t.Run("AfterCommit_ExecutorTxFails", func(t *testing.T) {
		reset(t)

		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		var ex relica.Executor = tx
		require.NoError(t, insertRow(ctx, ex, "committed", 1))
		require.NoError(t, tx.Commit())

		err = insertRow(ctx, ex, "too-late", 2)
		require.Error(t, err)
		assert.ErrorIs(t, err, sql.ErrTxDone)

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)
	})

	// Positive: a write through Executor(tx) is invisible to a *DB reader on
	// another connection until commit.
	t.Run("Isolation_UncommittedInvisibleToDB", func(t *testing.T) {
		reset(t)

		err := db.Transactional(ctx, func(tx *relica.Tx) error {
			if err := insertRow(ctx, tx, "pending", 1); err != nil {
				return err
			}
			inside, err := countRows(ctx, tx)
			if err != nil {
				return err
			}
			assert.Equal(t, int64(1), inside, "tx sees its own write")

			outside, err := countRows(ctx, db)
			if err != nil {
				return err
			}
			assert.Equal(t, int64(0), outside, "other connection must not see uncommitted write")
			return nil
		})
		require.NoError(t, err)

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(1), n, "visible after commit")
	})

	// Model API through the interface: commit populates the PK, rollback
	// discards the row.
	t.Run("Model_ViaExecutor", func(t *testing.T) {
		reset(t)
		boom := errors.New("abort")

		err := db.Transactional(ctx, func(tx *relica.Tx) error {
			var ex relica.Executor = tx
			if err := ex.Model(&execRow{Name: "model-gone", Value: 1}).WithContext(ctx).Insert(); err != nil {
				return err
			}
			return boom
		})
		require.ErrorIs(t, err, boom)

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(0), n)

		var kept execRow
		require.NoError(t, db.Transactional(ctx, func(tx *relica.Tx) error {
			var ex relica.Executor = tx
			kept = execRow{Name: "model-kept", Value: 2}
			return ex.Model(&kept).WithContext(ctx).Insert()
		}))
		assert.Greater(t, kept.ID, 0, "PK populated through Executor(tx)")

		var found execRow
		require.NoError(t, db.Select("id", "name", "value").From("tx_test").
			Where(relica.Eq("id", kept.ID)).One(&found))
		assert.Equal(t, "model-kept", found.Name)
	})

	// Raw SQL through the interface on both implementations.
	t.Run("RawSQL_ViaExecutor", func(t *testing.T) {
		reset(t)

		run := func(ex relica.Executor, name string) error {
			if _, err := ex.ExecContext(ctx, "INSERT INTO tx_test (name, value) VALUES (?, ?)", name, 1); err != nil {
				return err
			}
			var n int64
			return ex.QueryRowContext(ctx, "SELECT COUNT(*) FROM tx_test WHERE name = ?", name).Scan(&n)
		}

		if ds.Dialect == "postgres" {
			// Raw SQL bypasses the builder, so placeholders are the driver's.
			run = func(ex relica.Executor, name string) error {
				if _, err := ex.ExecContext(ctx, "INSERT INTO tx_test (name, value) VALUES ($1, $2)", name, 1); err != nil {
					return err
				}
				var n int64
				return ex.QueryRowContext(ctx, "SELECT COUNT(*) FROM tx_test WHERE name = $1", name).Scan(&n)
			}
		}

		require.NoError(t, run(db, "raw-db"))
		require.NoError(t, db.Transactional(ctx, func(tx *relica.Tx) error {
			return run(tx, "raw-tx")
		}))

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)
	})

	// Interop: a transaction started outside Relica (raw database/sql, as
	// another library or sqlc would) is joined via WrapTx. Repository code
	// sees an Executor; the external owner decides commit vs rollback.
	t.Run("WrapTx_JoinsExternalTransaction", func(t *testing.T) {
		reset(t)
		raw := db.SqlDB()

		// Rollback by the external owner discards Relica's writes.
		sqlTx, err := raw.BeginTx(ctx, nil)
		require.NoError(t, err)
		var ex relica.Executor = db.WrapTx(ctx, sqlTx)
		require.NoError(t, insertRow(ctx, ex, "external-rollback", 1))
		require.NoError(t, sqlTx.Rollback())

		n, err := countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(0), n)

		// Commit by the external owner persists them.
		sqlTx, err = raw.BeginTx(ctx, nil)
		require.NoError(t, err)
		ex = db.WrapTx(ctx, sqlTx)
		require.NoError(t, insertRow(ctx, ex, "external-commit", 2))
		require.NoError(t, ex.Model(&execRow{Name: "external-model", Value: 3}).Insert())
		require.NoError(t, sqlTx.Commit())

		n, err = countRows(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)

		// The wrapper is bound to the finished transaction, not to a new one.
		err = insertRow(ctx, ex, "too-late", 4)
		assert.ErrorIs(t, err, sql.ErrTxDone)
	})
}

// ---------------------------------------------------------------------------
// Entry points — names must contain Postgres / MySQL / SQLite for the CI
// -run filters.
// ---------------------------------------------------------------------------

func TestExecutor_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runExecutorTests(t, ds)
}

func TestExecutor_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runExecutorTests(t, ds)
}

// TestExecutor_SQLite uses a file-backed database rather than ":memory:".
// With ":memory:" every pool connection opens its own private database, so
// the cross-connection scenarios (isolation, pool exhaustion, the autocommit
// leak) could not run. A file in t.TempDir() behaves like production SQLite.
func TestExecutor_SQLite(t *testing.T) {
	db, err := relica.NewDB("sqlite", filepath.Join(t.TempDir(), "executor.db"))
	require.NoError(t, err)
	ds := &DatabaseSetup{DB: db, Dialect: "sqlite"}
	defer ds.Close()
	runExecutorTests(t, ds)
}
