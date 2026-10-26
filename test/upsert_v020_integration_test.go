//go:build integration

package test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ADR-019 on real databases: exactly one explicit conflict action, invalid
// upserts fail before anything is sent, updates read the proposed row.

type upItem struct {
	ID        int     `db:"id"`
	Code      *string `db:"code"`
	Name      string  `db:"name"`
	CreatedAt string  `db:"created_at"`
}

func (upItem) TableName() string { return "up_items" }

func createUpItems(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS up_items")
	idType := "INTEGER"
	if dialect == "mysql" {
		idType = "INT"
	}
	_, err := db.ExecContext(ctx, "CREATE TABLE up_items (id "+idType+" PRIMARY KEY, "+
		"code VARCHAR(20) UNIQUE, name VARCHAR(50) NOT NULL, "+
		"created_at VARCHAR(20) NOT NULL DEFAULT 'db-default')")
	require.NoError(t, err)
}

func resetUpItems(t *testing.T, db *relica.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), "DELETE FROM up_items")
	require.NoError(t, err)
	_, err = db.Insert("up_items", map[string]any{"id": 1, "code": "A", "name": "first", "created_at": "t0"}).Execute()
	require.NoError(t, err)
}

func getUpItem(t *testing.T, db *relica.DB, id int) (upItem, bool) {
	t.Helper()
	var it upItem
	err := db.Select("id", "code", "name", "created_at").From("up_items").Where(relica.Eq("id", id)).One(&it)
	if errors.Is(err, relica.ErrNotFound) {
		return it, false
	}
	require.NoError(t, err)
	return it, true
}

func countUpItems(t *testing.T, db *relica.DB) int64 {
	t.Helper()
	n, err := db.Select().From("up_items").Count()
	require.NoError(t, err)
	return n
}

func strp(s string) *string { return &s }

func runUpsertV020Tests(t *testing.T, db *relica.DB, dialect string) {
	createUpItems(t, db, dialect)
	row := func(name, created string) map[string]any {
		return map[string]any{"id": 1, "name": name, "created_at": created}
	}

	t.Run("no action is an error and sends nothing", func(t *testing.T) {
		resetUpItems(t, db)
		_, err := db.Upsert("up_items", row("changed", "t1")).Execute()
		require.ErrorContains(t, err, "exactly one of DoUpdate")
		_, err = db.Upsert("up_items", row("changed", "t1")).OnConflict("id").Execute()
		require.ErrorContains(t, err, "exactly one of DoUpdate")
		it, _ := getUpItem(t, db, 1)
		assert.Equal(t, "first", it.Name)
	})

	t.Run("two actions or a column not inserted are errors", func(t *testing.T) {
		resetUpItems(t, db)
		_, err := db.Upsert("up_items", row("changed", "t1")).OnConflict("id").DoUpdate("name").DoNothing().Execute()
		require.ErrorContains(t, err, "exactly one of DoUpdate")
		_, err = db.Upsert("up_items", row("changed", "t1")).OnConflict("id").DoUpdate("code").Execute()
		require.ErrorContains(t, err, `DoUpdate column "code" is not among the inserted columns`)
		it, _ := getUpItem(t, db, 1)
		assert.Equal(t, "first", it.Name)
	})

	t.Run("DoUpdateAll updates every inserted column except the target", func(t *testing.T) {
		resetUpItems(t, db)
		_, err := db.Upsert("up_items", row("second", "t1")).OnConflict("id").DoUpdateAll().Execute()
		require.NoError(t, err)
		it, _ := getUpItem(t, db, 1)
		assert.Equal(t, "second", it.Name)
		assert.Equal(t, "t1", it.CreatedAt)
		require.NotNil(t, it.Code)
		assert.Equal(t, "A", *it.Code, "a column not inserted keeps its value")
	})

	t.Run("DoUpdate updates exactly the listed columns", func(t *testing.T) {
		resetUpItems(t, db)
		_, err := db.Upsert("up_items", row("third", "t2")).OnConflict("id").DoUpdate("name").Execute()
		require.NoError(t, err)
		it, _ := getUpItem(t, db, 1)
		assert.Equal(t, "third", it.Name)
		assert.Equal(t, "t0", it.CreatedAt)
	})

	t.Run("DoUpdate without OnConflict", func(t *testing.T) {
		resetUpItems(t, db)
		_, err := db.Upsert("up_items", row("fourth", "t3")).DoUpdate("name").Execute()
		if dialect == "mysql" {
			require.NoError(t, err, "MySQL fires on any unique key; OnConflict is advisory")
			it, _ := getUpItem(t, db, 1)
			assert.Equal(t, "fourth", it.Name)
			return
		}
		require.ErrorContains(t, err, "needs OnConflict(cols...)")
	})

	t.Run("DoNothing keeps the existing row and inserts new keys", func(t *testing.T) {
		resetUpItems(t, db)
		_, err := db.Upsert("up_items", row("ignored", "t9")).OnConflict("id").DoNothing().Execute()
		require.NoError(t, err)
		_, err = db.Upsert("up_items", row("ignored", "t9")).DoNothing().Execute()
		require.NoError(t, err, "DoNothing without a target: any constraint")
		it, _ := getUpItem(t, db, 1)
		assert.Equal(t, "first", it.Name)
		assert.Equal(t, "t0", it.CreatedAt)

		_, err = db.Upsert("up_items", map[string]any{"id": 2, "name": "new", "created_at": "t1"}).
			OnConflict("id").DoNothing().Execute()
		require.NoError(t, err)
		it, ok := getUpItem(t, db, 2)
		require.True(t, ok)
		assert.Equal(t, "new", it.Name)
	})

	t.Run("conflict on a unique key other than the target", func(t *testing.T) {
		resetUpItems(t, db) // (1, 'A', 'first')
		_, err := db.Upsert("up_items", map[string]any{"id": 2, "code": "A", "name": "via code", "created_at": "t1"}).
			OnConflict("id").DoUpdateAll().Execute()
		if dialect == "mysql" {
			// ODKU fires on the code key; the target id stays out of SET, so the
			// matched row keeps id 1.
			require.NoError(t, err)
			it, ok := getUpItem(t, db, 1)
			require.True(t, ok)
			assert.Equal(t, "via code", it.Name)
			_, ok = getUpItem(t, db, 2)
			assert.False(t, ok)
			return
		}
		// PostgreSQL/SQLite: ON CONFLICT (id) does not cover the code key.
		require.Error(t, err)
		assert.True(t, relica.IsUniqueViolation(err), "got %v", err)
		assert.Equal(t, int64(1), countUpItems(t, db))
	})

	t.Run("NULL in the target never conflicts", func(t *testing.T) {
		resetUpItems(t, db)
		for _, id := range []int{2, 3} {
			_, err := db.Upsert("up_items", map[string]any{"id": id, "code": nil, "name": "null code", "created_at": "t1"}).
				OnConflict("code").DoNothing().Execute()
			require.NoError(t, err)
		}
		n, err := db.Select().From("up_items").Where(relica.IsNull("code")).Count()
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)
	})

	t.Run("Model Upsert honors Exclude on insert and update", func(t *testing.T) {
		_, err := db.ExecContext(context.Background(), "DELETE FROM up_items")
		require.NoError(t, err)
		m := upItem{ID: 5, Code: strp("M"), Name: "a", CreatedAt: "from struct"}
		require.NoError(t, db.Model(&m).Exclude("created_at").Upsert())
		it, _ := getUpItem(t, db, 5)
		assert.Equal(t, "db-default", it.CreatedAt, "excluded column is not inserted")

		m.Name, m.CreatedAt = "b", "overwrite?"
		require.NoError(t, db.Model(&m).Exclude("created_at").Upsert())
		it, _ = getUpItem(t, db, 5)
		assert.Equal(t, "b", it.Name)
		assert.Equal(t, "db-default", it.CreatedAt, "excluded column is not updated (FIX-010 #9)")

		err = db.Model(&m).Exclude("crated_at").Upsert()
		require.ErrorContains(t, err, `Exclude name "crated_at" is not a column of the model`)
		err = db.Model(&m).Upsert("title")
		require.ErrorContains(t, err, `Upsert field "title" is not a column of the model`)
	})

	t.Run("batch with duplicate keys", func(t *testing.T) {
		_, err := db.ExecContext(context.Background(), "DELETE FROM up_items")
		require.NoError(t, err)
		_, err = db.BatchInsert("up_items", []string{"id", "name", "created_at"}).
			Values(10, "d1", "t").Values(10, "d2", "t").
			OnConflict("id").DoNothing().Execute()
		require.NoError(t, err)
		it, _ := getUpItem(t, db, 10)
		assert.Equal(t, "d1", it.Name, "DoNothing: the first row wins")

		_, err = db.BatchInsert("up_items", []string{"id", "name", "created_at"}).
			Values(11, "e1", "t").Values(11, "e2", "t").
			OnConflict("id").DoUpdate("name").Execute()
		if dialect == "postgres" {
			require.ErrorContains(t, err, "second time")
			return
		}
		require.NoError(t, err)
		it, _ = getUpItem(t, db, 11)
		assert.Equal(t, "e2", it.Name, "MySQL/SQLite: the last row wins")
	})

	t.Run("batch DoUpdate without a target", func(t *testing.T) {
		resetUpItems(t, db)
		_, err := db.BatchInsert("up_items", []string{"id", "name", "created_at"}).
			Values(1, "batch", "t").DoUpdate("name").Execute()
		if dialect == "mysql" {
			require.NoError(t, err)
			it, _ := getUpItem(t, db, 1)
			assert.Equal(t, "batch", it.Name)
			return
		}
		require.ErrorContains(t, err, "needs OnConflict(cols...)", "FIX-010 #8: was silently a plain INSERT")
	})

	t.Run("concurrent upserts of one key leave one row", func(t *testing.T) {
		_, err := db.ExecContext(context.Background(), "DELETE FROM up_items")
		require.NoError(t, err)
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var err error
				for attempt := 0; attempt < 5; attempt++ {
					_, err = db.Upsert("up_items", map[string]any{"id": 20, "name": "g", "created_at": "t"}).
						OnConflict("id").DoUpdateAll().Execute()
					if err == nil || !(relica.IsDeadlock(err) || relica.IsSerializationFailure(err) || relica.IsLockTimeout(err)) {
						break
					}
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			assert.NoError(t, err)
		}
		assert.Equal(t, int64(1), countUpItems(t, db))
	})
}

// --- BEFORE INSERT triggers: the update copies the proposed row as the trigger left it.

func runUpsertTriggerTests(t *testing.T, db *relica.DB, dialect string, rowAliasDB *relica.DB) {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS up_trig")
	idType := "INTEGER"
	if dialect == "mysql" {
		idType = "INT"
	}
	_, err := db.ExecContext(ctx, "CREATE TABLE up_trig (id "+idType+" PRIMARY KEY, email VARCHAR(50) NOT NULL)")
	require.NoError(t, err)

	switch dialect {
	case "postgres":
		_, err = db.ExecContext(ctx, `CREATE OR REPLACE FUNCTION up_trig_lower() RETURNS trigger AS $$
BEGIN NEW.email := lower(NEW.email); RETURN NEW; END $$ LANGUAGE plpgsql`)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `CREATE TRIGGER up_trig_lower BEFORE INSERT ON up_trig FOR EACH ROW EXECUTE FUNCTION up_trig_lower()`)
	case "mysql":
		_, err = db.ExecContext(ctx, "CREATE TRIGGER up_trig_lower BEFORE INSERT ON up_trig FOR EACH ROW SET NEW.email = LOWER(NEW.email)")
	default:
		t.Skip("SQLite triggers cannot assign NEW columns, so there is no BEFORE INSERT rewrite to observe")
	}
	require.NoError(t, err)

	check := func(t *testing.T, d *relica.DB) {
		_, err := d.ExecContext(ctx, "DELETE FROM up_trig")
		require.NoError(t, err)
		_, err = d.Insert("up_trig", map[string]any{"id": 1, "email": "old"}).Execute()
		require.NoError(t, err)
		_, err = d.Upsert("up_trig", map[string]any{"id": 1, "email": "MiXeD@Example.COM"}).
			OnConflict("id").DoUpdateAll().Execute()
		require.NoError(t, err)
		var email string
		require.NoError(t, d.Select("email").From("up_trig").Where(relica.Eq("id", 1)).Row(&email))
		assert.Equal(t, "mixed@example.com", email)
	}
	t.Run("default syntax", func(t *testing.T) { check(t, db) })
	if rowAliasDB != nil {
		t.Run("MySQL row alias", func(t *testing.T) {
			sqlText, _ := rowAliasDB.Upsert("up_trig", map[string]any{"id": 1, "email": "x"}).OnConflict("id").DoUpdateAll().ToSQL()
			require.Contains(t, sqlText, "AS excluded ON DUPLICATE KEY UPDATE `email` = excluded.`email`")
			check(t, rowAliasDB)
		})
	}
}

// --- MySQL DoNothing is a self-assignment: its documented side effects.

func runMySQLDoNothingTests(t *testing.T, db *relica.DB) {
	ctx := context.Background()
	for _, s := range []string{
		"DROP TRIGGER IF EXISTS up_auto_bu",
		"DROP TABLE IF EXISTS up_auto",
		"DROP TABLE IF EXISTS up_auto_log",
		"CREATE TABLE up_auto (id INT AUTO_INCREMENT PRIMARY KEY, code VARCHAR(20) NOT NULL UNIQUE, " +
			"updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP)",
		"CREATE TABLE up_auto_log (n INT NOT NULL)",
	} {
		_, err := db.ExecContext(ctx, s)
		require.NoError(t, err, s)
	}
	_, err := db.ExecContext(ctx, "INSERT INTO up_auto (code, updated_at) VALUES ('A', '2000-01-01 00:00:00')")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "CREATE TRIGGER up_auto_bu BEFORE UPDATE ON up_auto FOR EACH ROW INSERT INTO up_auto_log VALUES (1)")
	require.NoError(t, err)

	res, err := db.Upsert("up_auto", map[string]any{"code": "A"}).OnConflict("code").DoNothing().Execute()
	require.NoError(t, err)
	affected, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected, "unchanged row: 0 affected rows (1 with clientFoundRows=true)")

	var updatedAt string
	require.NoError(t, db.Select("updated_at").From("up_auto").Where(relica.Eq("code", "A")).Row(&updatedAt))
	assert.Contains(t, updatedAt, "2000-01-01", "ON UPDATE CURRENT_TIMESTAMP does not fire for an unchanged row")

	var fired int64
	require.NoError(t, db.Select("COUNT(*)").From("up_auto_log").Row(&fired))
	assert.Equal(t, int64(1), fired, "the BEFORE UPDATE trigger fires")

	_, err = db.ExecContext(ctx, "INSERT INTO up_auto (code) VALUES ('B')")
	require.NoError(t, err)
	var idB int64
	require.NoError(t, db.Select("id").From("up_auto").Where(relica.Eq("code", "B")).Row(&idB))
	assert.Equal(t, int64(3), idB, "the DoNothing upsert consumed AUTO_INCREMENT value 2")
}

// ---------------------------------------------------------------------------
// Entry points — one per database; CI selects the database with RELICA_TEST_DB.
// ---------------------------------------------------------------------------

func TestUpsertV020_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runUpsertV020Tests(t, ds.DB, "postgres")
	t.Run("triggers", func(t *testing.T) { runUpsertTriggerTests(t, ds.DB, "postgres", nil) })
}

func TestUpsertV020_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runUpsertV020Tests(t, ds.DB, "mysql")

	alias, err := relica.Open("mysql", ds.DSN, relica.WithMySQLUpsertSyntax(relica.MySQLUpsertRowAlias))
	require.NoError(t, err)
	defer func() { _ = alias.Close() }()
	t.Run("triggers", func(t *testing.T) { runUpsertTriggerTests(t, ds.DB, "mysql", alias) })
	t.Run("DoNothing side effects", func(t *testing.T) { runMySQLDoNothingTests(t, ds.DB) })
}

func TestUpsertV020_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	// One connection: SQLite serializes writers, and the concurrency case then
	// checks the result, not "database is locked".
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "upsert.db"), relica.WithMaxOpenConns(1))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runUpsertV020Tests(t, db, "sqlite")
	t.Run("triggers", func(t *testing.T) { runUpsertTriggerTests(t, db, "sqlite", nil) })
}

var _ = sql.ErrNoRows
