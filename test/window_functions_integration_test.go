//go:build integration

package test

import (
	"path/filepath"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The forms docs/WINDOW_FUNCTIONS_GUIDE.md and README teach, run on every
// database: one SelectExpr per expression with the alias inside it, top-N per
// group through FromSelect, and the two traps the guide warns about (an alias
// passed as SelectExpr's argument, MySQL 8's reserved window function names).
func runWindowFunctionTests(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	SetupTestData(t, db, dialect)
	defer CleanupTestData(t, db)

	type ranked struct {
		Name   string `db:"name"`
		Price  int    `db:"price"`
		RankNo int    `db:"rank_no"`
	}

	t.Run("RowNumber", func(t *testing.T) {
		var rows []ranked
		err := db.Select("name", "price").
			SelectExpr("ROW_NUMBER() OVER (ORDER BY price DESC) AS rank_no").
			From("test_products").
			OrderBy("rank_no").
			All(&rows)
		require.NoError(t, err)
		require.Len(t, rows, 5)
		assert.Equal(t, ranked{"Gadget Y", 7500, 1}, rows[0])
		assert.Equal(t, ranked{"Doohickey Z", 500, 5}, rows[4])
	})

	t.Run("RankDenseRankNtile", func(t *testing.T) {
		var rows []struct {
			Name      string `db:"name"`
			RankNo    int    `db:"rank_no"`
			DenseNo   int    `db:"dense_no"`
			Half      int    `db:"half"`
			Category  string `db:"category"`
			CatRankNo int    `db:"cat_rank_no"`
		}
		err := db.Select("name", "category").
			SelectExpr("RANK() OVER (ORDER BY category) AS rank_no").
			SelectExpr("DENSE_RANK() OVER (ORDER BY category) AS dense_no").
			SelectExpr("NTILE(2) OVER (ORDER BY price) AS half").
			SelectExpr("RANK() OVER (PARTITION BY category ORDER BY price DESC) AS cat_rank_no").
			From("test_products").
			OrderBy("name").
			All(&rows)
		require.NoError(t, err)
		require.Len(t, rows, 5)
		// Sorted by name: Doohickey Z (misc), Gadget X, Gadget Y (gadgets), Widget A, Widget B (widgets).
		assert.Equal(t, 1, rows[1].RankNo, "gadgets sort first")
		assert.Equal(t, 3, rows[0].RankNo, "misc: rank skips past the two gadgets")
		assert.Equal(t, 2, rows[0].DenseNo, "misc: dense rank has no gap")
		assert.Equal(t, 2, rows[1].CatRankNo, "Gadget X is the cheaper gadget")
		assert.Equal(t, 1, rows[0].Half, "Doohickey Z is the cheapest")
	})

	t.Run("TopNPerGroupViaFromSelect", func(t *testing.T) {
		inner := db.Select("category", "name", "price").
			SelectExpr("ROW_NUMBER() OVER (PARTITION BY category ORDER BY price DESC) AS rank_no").
			From("test_products")
		var rows []struct {
			Category string `db:"category"`
			Name     string `db:"name"`
			Price    int    `db:"price"`
			RankNo   int    `db:"rank_no"`
		}
		err := db.Select("category", "name", "price", "rank_no").
			FromSelect(inner, "r").
			Where(relica.LessOrEqual("rank_no", 1)).
			OrderBy("category").
			All(&rows)
		require.NoError(t, err)
		require.Len(t, rows, 3)
		assert.Equal(t, "Gadget Y", rows[0].Name)
		assert.Equal(t, "Doohickey Z", rows[1].Name)
		assert.Equal(t, "Widget B", rows[2].Name)
	})

	t.Run("LagLeadRunningTotal", func(t *testing.T) {
		var rows []struct {
			Name         string `db:"name"`
			PrevPrice    *int   `db:"prev_price"`
			NextPrice    *int   `db:"next_price"`
			RunningTotal int    `db:"running_total"`
		}
		err := db.Select("name").
			SelectExpr("LAG(price) OVER (ORDER BY price) AS prev_price").
			SelectExpr("LEAD(price) OVER (ORDER BY price) AS next_price").
			SelectExpr("SUM(price) OVER (ORDER BY price ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS running_total").
			From("test_products").
			OrderBy("price").
			All(&rows)
		require.NoError(t, err)
		require.Len(t, rows, 5)
		assert.Nil(t, rows[0].PrevPrice)
		require.NotNil(t, rows[0].NextPrice)
		assert.Equal(t, 1000, *rows[0].NextPrice)
		assert.Nil(t, rows[4].NextPrice)
		assert.Equal(t, 16000, rows[4].RunningTotal)
	})

	t.Run("AliasAsArgumentFails", func(t *testing.T) {
		var rows []ranked
		err := db.Select("name", "price").
			SelectExpr("ROW_NUMBER() OVER (ORDER BY price DESC)", "rank_no").
			From("test_products").
			All(&rows)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "0 placeholders but 1 args")
	})

	t.Run("ReservedAliasesOnMySQL", func(t *testing.T) {
		for _, alias := range []string{"rank", "row_number", "dense_rank", "lag", "lead"} {
			var n []int
			err := db.Select().
				SelectExpr("ROW_NUMBER() OVER (ORDER BY price) AS " + alias).
				From("test_products").
				Column(&n)
			if dialect == "mysql" {
				assert.Error(t, err, "MySQL 8 reserves %q", alias)
			} else {
				assert.NoError(t, err, "%s accepts %q as an alias", dialect, alias)
			}
		}
	})
}

func TestWindowFunctions_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runWindowFunctionTests(t, ds.DB, "postgres")
}

func TestWindowFunctions_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runWindowFunctionTests(t, ds.DB, "mysql")
}

func TestWindowFunctions_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "wf.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runWindowFunctionTests(t, db, "sqlite")
}
