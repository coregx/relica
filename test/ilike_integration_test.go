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

// ILike family on real databases. The label column is case-sensitive on
// PostgreSQL (always) and MySQL (utf8mb4_bin), so a match proves the
// expression itself is case-insensitive, not the column collation.

func createCiLabelTable(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS ci_label")
	ddl := `CREATE TABLE ci_label (id INTEGER PRIMARY KEY, label VARCHAR(100) NOT NULL)`
	if dialect == "mysql" {
		ddl = "CREATE TABLE ci_label (id INT PRIMARY KEY, " +
			"label VARCHAR(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL)"
	}
	_, err := db.ExecContext(ctx, ddl)
	require.NoError(t, err)
}

// seedCiLabels resets the table to:
//
//	1 "Alpha Xeon", 2 "BETA 50% off", 3 "gamma x_y", 4 "Привет Мир"
func seedCiLabels(t *testing.T, db *relica.DB) {
	t.Helper()
	_, err := db.Delete("ci_label").Execute()
	require.NoError(t, err)
	for i, label := range []string{"Alpha Xeon", "BETA 50% off", "gamma x_y", "Привет Мир"} {
		_, err := db.Insert("ci_label", map[string]any{"id": i + 1, "label": label}).Execute()
		require.NoError(t, err)
	}
}

func ciIDs(t *testing.T, q *relica.SelectQuery) []int {
	t.Helper()
	var ids []int
	require.NoError(t, q.OrderBy("id").Column(&ids))
	return ids
}

func runILikeTests(t *testing.T, db *relica.DB, dialect string) {
	createCiLabelTable(t, db, dialect)
	seedCiLabels(t, db)
	sel := func() *relica.SelectQuery { return db.Select("id").From("ci_label") }

	t.Run("ILike_MatchesAnyCase", func(t *testing.T) {
		assert.Equal(t, []int{1}, ciIDs(t, sel().Where(relica.ILike("label", "alpha"))))
		assert.Equal(t, []int{1}, ciIDs(t, sel().Where(relica.ILike("label", "XEON"))))
		assert.Equal(t, []int{2}, ciIDs(t, sel().Where(relica.ILike("label", "Beta"))))
		// Several values: AND.
		assert.Equal(t, []int{1}, ciIDs(t, sel().Where(relica.ILike("label", "ALPHA", "xeon"))))
		assert.Empty(t, ciIDs(t, sel().Where(relica.ILike("label", "alpha", "beta"))))
	})

	t.Run("ILike_ColumnIsCaseSensitive", func(t *testing.T) {
		// Guard for the test itself: on PostgreSQL and MySQL (utf8mb4_bin) plain
		// LIKE must miss, otherwise ILike_MatchesAnyCase proves nothing there.
		// SQLite LIKE folds ASCII by default, so there is nothing to guard.
		if dialect == "sqlite" {
			t.Skip("SQLite LIKE is ASCII case-insensitive by default")
		}
		assert.Empty(t, ciIDs(t, sel().Where(relica.Like("label", "alpha"))))
	})

	t.Run("NotILike_OrILike_OrNotILike", func(t *testing.T) {
		assert.Equal(t, []int{2, 3, 4}, ciIDs(t, sel().Where(relica.NotILike("label", "ALPHA"))))
		assert.Equal(t, []int{1, 3}, ciIDs(t, sel().Where(relica.OrILike("label", "alpha", "GAMMA"))))
		assert.Equal(t, []int{1, 2, 3, 4}, ciIDs(t, sel().Where(relica.OrNotILike("label", "alpha", "beta"))))
		assert.Equal(t, []int{3, 4}, ciIDs(t, sel().Where(relica.NotILike("label", "alpha", "BETA"))))
	})

	t.Run("ILike_EscapesWildcards", func(t *testing.T) {
		assert.Equal(t, []int{2}, ciIDs(t, sel().Where(relica.ILike("label", "50% OFF"))))
		assert.Equal(t, []int{3}, ciIDs(t, sel().Where(relica.ILike("label", "X_Y"))))
		// Negative: escaped % and _ are literals, not wildcards.
		assert.Empty(t, ciIDs(t, sel().Where(relica.ILike("label", "x%y"))))
		assert.Empty(t, ciIDs(t, sel().Where(relica.ILike("label", "5_%"))))
	})

	t.Run("ILike_Match", func(t *testing.T) {
		assert.Equal(t, []int{1}, ciIDs(t, sel().Where(relica.ILike("label", "ALPHA").Match(false, true))))
		assert.Empty(t, ciIDs(t, sel().Where(relica.ILike("label", "XEON").Match(false, true))))
		assert.Equal(t, []int{1}, ciIDs(t, sel().Where(relica.ILike("label", "XEON").Match(true, false))))
	})

	t.Run("ILike_QualifiedColumn", func(t *testing.T) {
		var ids []int
		err := db.Select("c.id").From("ci_label c").
			Where(relica.ILike("c.label", "beta")).Column(&ids)
		require.NoError(t, err)
		assert.Equal(t, []int{2}, ids)
	})

	t.Run("ILike_InTransaction", func(t *testing.T) {
		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			var ids []int
			if err := tx.Select("id").From("ci_label").
				Where(relica.ILike("label", "gamma")).Column(&ids); err != nil {
				return err
			}
			assert.Equal(t, []int{3}, ids)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("ILike_InvalidEscape_DeletesNothing", func(t *testing.T) {
		bad := relica.ILike("label", "zzz").EscapeChars("!")
		require.Error(t, bad.Err())
		_, err := db.Delete("ci_label").Where(bad).Execute()
		require.Error(t, err, "an invalid filter must fail, not run an unconditional DELETE")
		n, err := db.Select().From("ci_label").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(4), n)
	})

	t.Run("ILike_NonASCII", func(t *testing.T) {
		got := ciIDs(t, sel().Where(relica.ILike("label", "привет")))
		switch dialect {
		case "mysql":
			// LOWER() on utf8mb4 folds Unicode.
			assert.Equal(t, []int{4}, got)
		case "sqlite":
			// Documented limitation: SQLite LOWER() folds ASCII only (no ICU).
			assert.Empty(t, got)
		default:
			// PostgreSQL folds non-ASCII according to the database locale
			// (LC_CTYPE); the CI image's locale decides. Record, don't assert.
			t.Logf("PostgreSQL ILIKE 'привет' matched ids %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Entry points — one per database; CI selects the database with RELICA_TEST_DB.
// ---------------------------------------------------------------------------

func TestILike_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runILikeTests(t, ds.DB, "postgres")
}

func TestILike_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runILikeTests(t, ds.DB, "mysql")
}

func TestILike_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.NewDB("sqlite", filepath.Join(t.TempDir(), "ilike.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runILikeTests(t, db, "sqlite")
}
