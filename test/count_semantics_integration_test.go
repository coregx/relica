//go:build integration

package test

import (
	"context"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Count is the number of rows All returns (FIX-009). A select list with an
// aggregate and no GROUP BY returns one row; Count replaced the list with
// COUNT(*) and returned the number of matching rows instead (F2).
func runCountSemanticsTests(t *testing.T, db *relica.DB) {
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS cs_emp",
		"CREATE TABLE cs_emp (id INTEGER PRIMARY KEY, dept INTEGER NOT NULL, salary INTEGER NOT NULL)",
		"INSERT INTO cs_emp (id, dept, salary) VALUES (1, 1, 10), (2, 1, 30), (3, 2, 20), (4, 2, 40)",
	} {
		_, err := db.ExecContext(ctx, s)
		require.NoError(t, err, s)
	}

	count := func(q *relica.SelectQuery) int64 {
		t.Helper()
		n, err := q.Count()
		require.NoError(t, err)
		return n
	}
	rowsOf := func(q *relica.SelectQuery) int64 {
		t.Helper()
		var rows []relica.NullStringMap
		require.NoError(t, q.All(&rows))
		return int64(len(rows))
	}

	for name, q := range map[string]func() *relica.SelectQuery{
		"Select aggregate":          func() *relica.SelectQuery { return db.Select("MAX(salary)").From("cs_emp") },
		"Select aggregate AS alias": func() *relica.SelectQuery { return db.Select("MAX(salary) AS top").From("cs_emp") },
		"SelectExpr aggregate":      func() *relica.SelectQuery { return db.Select().SelectExpr("SUM(salary) AS total").From("cs_emp") },
		"aggregate with Where": func() *relica.SelectQuery {
			return db.Select("COUNT(*) AS n").From("cs_emp").Where(relica.Eq("dept", 1))
		},
		"plain columns": func() *relica.SelectQuery {
			return db.Select("id", "salary").From("cs_emp").Where(relica.Eq("dept", 2))
		},
		"SelectExpr arithmetic": func() *relica.SelectQuery { return db.Select("id").SelectExpr("salary * ? AS s", 2).From("cs_emp") },
		"GroupBy with aggregate": func() *relica.SelectQuery {
			return db.Select("dept", "MAX(salary) AS m").From("cs_emp").GroupBy("dept")
		},
	} {
		assert.Equal(t, rowsOf(q()), count(q()), name)
	}
	assert.Equal(t, int64(1), count(db.Select("MAX(salary)").From("cs_emp")))

	// F4: a grouped/distinct/compound Count is a derived table. SELECT * over
	// a join has two "id" columns; MySQL rejects duplicate names in a derived
	// table (error 1060), PostgreSQL and SQLite accept them. Listing the
	// columns works everywhere.
	_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS cs_dept")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "CREATE TABLE cs_dept (id INTEGER PRIMARY KEY, name VARCHAR(20) NOT NULL)")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO cs_dept (id, name) VALUES (1, 'eng'), (2, 'ops')")
	require.NoError(t, err)
	joined := func() *relica.SelectQuery {
		return db.Select().From("cs_emp e").InnerJoin("cs_dept d", "d.id = e.dept").Distinct()
	}
	n, err := joined().Count()
	if db.DriverName() == dialectMySQL {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Duplicate column", "MySQL error 1060")
	} else {
		require.NoError(t, err)
		assert.Equal(t, int64(4), n)
	}
	assert.Equal(t, int64(4), count(db.Select("e.id", "d.name").From("cs_emp e").
		InnerJoin("cs_dept d", "d.id = e.dept").Distinct()), "listed columns count on every database")
}

func TestCountSemantics_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runCountSemanticsTests(t, ds.DB)
}

func TestCountSemantics_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runCountSemanticsTests(t, ds.DB)
}

func TestCountSemantics_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runCountSemanticsTests(t, ds.DB)
}
