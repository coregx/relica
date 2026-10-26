//go:build integration

package test

import (
	"context"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A column name Relica quotes must reach the database as an identifier, so a
// typo fails instead of changing the result. SQLite reads an unknown
// double-quoted identifier as a string literal ("x" = 'x' is true), so
// Relica quotes with backticks there, as ozzo-dbx does. OrderBy takes
// "col [ASC|DESC]" only: anything after the direction was dropped silently.
func runIdentifierTypoTests(t *testing.T, db *relica.DB) {
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS it_emp",
		"CREATE TABLE it_emp (id INTEGER PRIMARY KEY, name VARCHAR(30) NOT NULL, salary INTEGER NOT NULL)",
		"INSERT INTO it_emp (id, name, salary) VALUES (1, 'alice', 10), (2, 'bob', 30), (3, 'carol', 20)",
	} {
		_, err := db.ExecContext(ctx, s)
		require.NoError(t, err, s)
	}

	t.Run("TypoInColumnName_IsAnError", func(t *testing.T) {
		var rows []relica.NullStringMap
		for name, q := range map[string]*relica.SelectQuery{
			"Select":  db.Select("no_such_col").From("it_emp"),
			"Eq":      db.Select("id").From("it_emp").Where(relica.Eq("no_such_col", "no_such_col")),
			"HashExp": db.Select("id").From("it_emp").Where(relica.HashExp{"no_such_col": "no_such_col"}),
			"In":      db.Select("id").From("it_emp").Where(relica.In("no_such_col", "no_such_col")),
			"OrderBy": db.Select("id").From("it_emp").OrderBy("no_such_col"),
			"GroupBy": db.Select("name").From("it_emp").GroupBy("no_such_col"),
		} {
			rows = nil
			err := q.All(&rows)
			assert.Error(t, err, "%s: a misspelled column must fail, got rows %v", name, rows)
		}
		_, err := db.Update("it_emp").Set(map[string]any{"no_such_col": "x"}).Where(relica.Eq("id", 1)).Execute()
		assert.Error(t, err, "Update.Set")
		_, err = db.Delete("it_emp").Where(relica.Eq("no_such_col", "no_such_col")).Execute()
		assert.Error(t, err, "Delete.Where")

		n, err := db.Select().From("it_emp").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n, "the failed DELETE removed nothing")
	})

	t.Run("OrderBy_ColumnAndDirectionOnly", func(t *testing.T) {
		for _, item := range []string{
			"salary + 1 DESC", "salary DESC NULLS LAST", "salary DESC, name", "name COLLATE NOCASE", "salary DOWN",
		} {
			var ids []int
			err := db.Select("id").From("it_emp").OrderBy(item).Column(&ids)
			require.Error(t, err, "OrderBy(%q) must not run as a different ORDER BY (got %v)", item, ids)
			assert.Contains(t, err.Error(), "OrderByExpr", item)
		}
		var ids []int
		require.NoError(t, db.Select("id").From("it_emp").OrderBy("salary DESC", "it_emp.name asc").Column(&ids))
		assert.Equal(t, []int{2, 3, 1}, ids)
		ids = nil
		require.NoError(t, db.Select("id").From("it_emp").OrderByExpr("salary * -1").Column(&ids))
		assert.Equal(t, []int{2, 3, 1}, ids)
	})

	t.Run("SelectLiteral_IsNotAString", func(t *testing.T) {
		// Select quotes its arguments as column names; a literal goes through
		// SelectExpr. On SQLite "1" used to come back as the string '1'.
		var rows []relica.NullStringMap
		err := db.Select("id", "1 as level").From("it_emp").All(&rows)
		assert.Error(t, err, "Select(\"1 as level\") is the column 1, which does not exist (got %v)", rows)
		var levels []int
		require.NoError(t, db.Select().SelectExpr("1 AS level").From("it_emp").Column(&levels))
		assert.Equal(t, []int{1, 1, 1}, levels)
	})
}

func TestIdentifierTypos_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runIdentifierTypoTests(t, ds.DB)
}

func TestIdentifierTypos_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runIdentifierTypoTests(t, ds.DB)
}

func TestIdentifierTypos_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runIdentifierTypoTests(t, ds.DB)
}
