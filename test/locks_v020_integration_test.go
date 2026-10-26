//go:build integration

package test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FIX-012 / ADR-020: row locks really lock on PostgreSQL and MySQL; outside a
// transaction or combined with set operations / GROUP BY / DISTINCT / Count /
// Exists they fail before anything is sent, on every database.

type lockJob struct {
	ID    int    `db:"id"`
	State string `db:"state"`
}

func createLockJobs(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS lock_jobs")
	idType := "INTEGER"
	if dialect == "mysql" {
		idType = "INT"
	}
	_, err := db.ExecContext(ctx, "CREATE TABLE lock_jobs (id "+idType+" PRIMARY KEY, state VARCHAR(20) NOT NULL)")
	require.NoError(t, err)
	for i := 1; i <= 3; i++ {
		_, err = db.Insert("lock_jobs", map[string]any{"id": i, "state": "new"}).Execute()
		require.NoError(t, err)
	}
}

// shortLockWait makes the second transaction give up quickly instead of waiting.
func shortLockWait(t *testing.T, tx *relica.Tx, dialect string) {
	t.Helper()
	ctx := context.Background()
	var err error
	switch dialect {
	case "postgres":
		_, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout = '200ms'")
	case "mysql":
		_, err = tx.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout = 1")
	}
	require.NoError(t, err)
}

func runLockRuleTests(t *testing.T, db *relica.DB) {
	t.Run("outside a transaction", func(t *testing.T) {
		var jobs []lockJob
		for name, q := range map[string]*relica.SelectQuery{
			"ForUpdate":           db.Select("id", "state").From("lock_jobs").ForUpdate(),
			"ForShare":            db.Select("id", "state").From("lock_jobs").ForShare(),
			"ForUpdateSkipLocked": db.Select("id", "state").From("lock_jobs").ForUpdateSkipLocked(),
		} {
			require.ErrorContains(t, q.All(&jobs), "require a transaction", name)
		}
	})

	t.Run("invalid combinations inside a transaction", func(t *testing.T) {
		tx, err := db.Begin(context.Background())
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		var jobs []lockJob
		other := tx.Select("id", "state").From("lock_jobs")
		require.ErrorContains(t, tx.Select("id", "state").From("lock_jobs").ForUpdate().Union(other).All(&jobs), "cannot be combined")
		require.ErrorContains(t, tx.Select("state").From("lock_jobs").GroupBy("state").ForUpdate().All(&jobs), "cannot be combined")
		require.ErrorContains(t, tx.Select("state").From("lock_jobs").Distinct().ForShare().All(&jobs), "cannot be combined")
		_, err = tx.Select().From("lock_jobs").ForUpdate().Count()
		require.ErrorContains(t, err, "cannot be")
		_, err = tx.Select().From("lock_jobs").ForUpdate().Exists()
		require.ErrorContains(t, err, "cannot be")
	})
}

func runLockBehaviorTests(t *testing.T, db *relica.DB, dialect string) {
	ctx := context.Background()

	t.Run("FOR UPDATE blocks a second locker", func(t *testing.T) {
		tx1, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx1.Rollback() }()
		var job lockJob
		require.NoError(t, tx1.Select("id", "state").From("lock_jobs").Where(relica.Eq("id", 1)).ForUpdate().One(&job))

		tx2, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx2.Rollback() }()
		shortLockWait(t, tx2, dialect)
		err = tx2.Select("id", "state").From("lock_jobs").Where(relica.Eq("id", 1)).ForUpdate().One(&job)
		require.Error(t, err)
		assert.True(t, relica.IsLockTimeout(err), "want a lock timeout, got %v", err)
	})

	t.Run("FOR UPDATE SKIP LOCKED skips rows another transaction holds", func(t *testing.T) {
		tx1, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx1.Rollback() }()
		var first []lockJob
		require.NoError(t, tx1.Select("id", "state").From("lock_jobs").OrderBy("id").Limit(1).ForUpdateSkipLocked().All(&first))
		require.Len(t, first, 1)
		assert.Equal(t, 1, first[0].ID)

		tx2, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx2.Rollback() }()
		var next []lockJob
		require.NoError(t, tx2.Select("id", "state").From("lock_jobs").OrderBy("id").Limit(1).ForUpdateSkipLocked().All(&next))
		require.Len(t, next, 1)
		assert.Equal(t, 2, next[0].ID, "the second worker claims the next free row")
	})

	t.Run("FOR SHARE allows another share lock but blocks its update", func(t *testing.T) {
		tx1, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx1.Rollback() }()
		var job lockJob
		require.NoError(t, tx1.Select("id", "state").From("lock_jobs").Where(relica.Eq("id", 3)).ForShare().One(&job))

		tx2, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx2.Rollback() }()
		shortLockWait(t, tx2, dialect)
		require.NoError(t, tx2.Select("id", "state").From("lock_jobs").Where(relica.Eq("id", 3)).ForShare().One(&job))
		_, err = tx2.Update("lock_jobs").Set(map[string]any{"state": "taken"}).Where(relica.Eq("id", 3)).Execute()
		require.Error(t, err)
		assert.True(t, relica.IsLockTimeout(err) || relica.IsDeadlock(err), "want a lock timeout, got %v", err)
	})
}

func TestLocksV020_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	createLockJobs(t, ds.DB, "postgres")
	runLockRuleTests(t, ds.DB)
	runLockBehaviorTests(t, ds.DB, "postgres")
}

func TestLocksV020_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	createLockJobs(t, ds.DB, "mysql")
	runLockRuleTests(t, ds.DB)
	runLockBehaviorTests(t, ds.DB, "mysql")
}

func TestLocksV020_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "locks.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	createLockJobs(t, db, "sqlite")
	// SQLite has no row locks: only the rules are checked here.
	runLockRuleTests(t, db)
}
