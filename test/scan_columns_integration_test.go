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

// Result columns and struct fields (v0.20.0, ScanRequireFields): a column the
// code named in Select must land in a field, and two result columns must not
// land in one field — both used to be silent (the column was dropped, or the
// later column overwrote the earlier one). Select() of a wider table still
// ignores the columns the struct does not have.
func runScanColumnsTests(t *testing.T, db *relica.DB) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS sc_b",
		"DROP TABLE IF EXISTS sc_a",
		"CREATE TABLE sc_a (id INT PRIMARY KEY, name VARCHAR(50))",
		"CREATE TABLE sc_b (id INT PRIMARY KEY, a_id INT, email VARCHAR(50))",
		"INSERT INTO sc_a (id, name) VALUES (1, 'alice')",
		"INSERT INTO sc_b (id, a_id, email) VALUES (10, 1, 'a@x')",
	} {
		_, err := db.ExecContext(ctx, s)
		require.NoError(t, err, s)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS sc_b")
		_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS sc_a")
	}()

	type idName struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	type idOnly struct {
		ID int `db:"id"`
	}

	t.Run("ListedColumnWithoutField_Error", func(t *testing.T) {
		var rows []struct {
			ID   int    `db:"id"`
			Mail string `db:"mail"` // renamed tag; the Select list still says name
		}
		err := db.Select("id", "name").From("sc_a").All(&rows)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `result column "name" matches no field`)
	})

	t.Run("ListedColumnsAllMatched_OK", func(t *testing.T) {
		var row idName
		require.NoError(t, db.Select("id", "name").From("sc_a").One(&row))
		assert.Equal(t, idName{1, "alice"}, row)
	})

	t.Run("SelectAllOfWiderTable_ExtraColumnsIgnored", func(t *testing.T) {
		var row idOnly
		require.NoError(t, db.Select().From("sc_a").One(&row))
		assert.Equal(t, 1, row.ID)
	})

	t.Run("TwoColumnsOneField_Error", func(t *testing.T) {
		var rows []idOnly
		err := db.Select("sc_a.id", "sc_b.id").From("sc_a").
			InnerJoin("sc_b", "sc_b.a_id = sc_a.id").All(&rows)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "appears more than once")
	})

	t.Run("JoinSelectAll_SameColumnTwice_Error", func(t *testing.T) {
		// SELECT * over a join returns id twice: which one is the user's id?
		// Before v0.20.0 the second (sc_b.id = 10) silently won.
		var row idName
		err := db.Select().From("sc_a").InnerJoin("sc_b", "sc_b.a_id = sc_a.id").One(&row)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "appears more than once")
	})

	t.Run("AliasedSecondColumn_OK", func(t *testing.T) {
		var row struct {
			ID  int `db:"id"`
			BID int `db:"b_id"`
		}
		require.NoError(t, db.Select("sc_a.id", "sc_b.id AS b_id").From("sc_a").
			InnerJoin("sc_b", "sc_b.a_id = sc_a.id").One(&row))
		assert.Equal(t, 1, row.ID)
		assert.Equal(t, 10, row.BID)
	})

	t.Run("Lenient_KeepsOzzoBehaviour", func(t *testing.T) {
		var row idOnly
		require.NoError(t, db.Select("id", "name").From("sc_a").ScanMode(relica.ScanLenient).One(&row))
		assert.Equal(t, 1, row.ID)
	})
}

func TestScanColumns_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runScanColumnsTests(t, ds.DB)
}

func TestScanColumns_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runScanColumnsTests(t, ds.DB)
}

func TestScanColumns_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "sc.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runScanColumnsTests(t, db)
}
