//go:build integration

package test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SELECT features on real databases: subqueries in FROM, raw and appended
// select columns, RIGHT/FULL/CROSS joins, ORDER BY and GROUP BY expressions,
// set operations, recursive CTEs, EXISTS, AsExpression and the column-to-column,
// IS NOT NULL and OR-LIKE expressions.
//
// Fixture (tables sf_dept, sf_emp, sf_tree):
//
//	sf_dept: 1 Eng, 2 Sales, 3 Legal (no employees)
//
//	sf_emp:  id name   dept_id salary target email
//	         1  Alice  1       100    90     a@x.com
//	         2  Bob    1       80     80     NULL
//	         3  Carol  2       70     75     c@y.org
//	         4  Dave   NULL    60     50     d@x.com
//
//	sf_tree: 1 (root) ← 2, 3; 2 ← 4; 5 (second root) ← 6

func createSelectFeatureTables(t *testing.T, db *relica.DB) {
	t.Helper()
	ctx := context.Background()
	for _, table := range []string{"sf_emp", "sf_dept", "sf_tree"} {
		_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table)
		require.NoError(t, err)
	}
	// INTEGER and VARCHAR are valid on all three databases.
	for _, ddl := range []string{
		`CREATE TABLE sf_dept (id INTEGER PRIMARY KEY, name VARCHAR(50) NOT NULL)`,
		`CREATE TABLE sf_emp (
			id INTEGER PRIMARY KEY,
			name VARCHAR(50) NOT NULL,
			dept_id INTEGER NULL,
			salary INTEGER NOT NULL,
			target INTEGER NOT NULL,
			email VARCHAR(100) NULL
		)`,
		`CREATE TABLE sf_tree (id INTEGER PRIMARY KEY, parent_id INTEGER NULL, name VARCHAR(50) NOT NULL)`,
	} {
		_, err := db.ExecContext(ctx, ddl)
		require.NoError(t, err)
	}

	for _, d := range []map[string]any{
		{"id": 1, "name": "Eng"},
		{"id": 2, "name": "Sales"},
		{"id": 3, "name": "Legal"},
	} {
		_, err := db.Insert("sf_dept", d).Execute()
		require.NoError(t, err)
	}
	for _, e := range []map[string]any{
		{"id": 1, "name": "Alice", "dept_id": 1, "salary": 100, "target": 90, "email": "a@x.com"},
		{"id": 2, "name": "Bob", "dept_id": 1, "salary": 80, "target": 80, "email": nil},
		{"id": 3, "name": "Carol", "dept_id": 2, "salary": 70, "target": 75, "email": "c@y.org"},
		{"id": 4, "name": "Dave", "dept_id": nil, "salary": 60, "target": 50, "email": "d@x.com"},
	} {
		_, err := db.Insert("sf_emp", e).Execute()
		require.NoError(t, err)
	}
	for _, n := range []map[string]any{
		{"id": 1, "parent_id": nil, "name": "root"},
		{"id": 2, "parent_id": 1, "name": "a"},
		{"id": 3, "parent_id": 1, "name": "b"},
		{"id": 4, "parent_id": 2, "name": "a1"},
		{"id": 5, "parent_id": nil, "name": "other"},
		{"id": 6, "parent_id": 5, "name": "o1"},
	} {
		_, err := db.Insert("sf_tree", n).Execute()
		require.NoError(t, err)
	}
}

// sfQ quotes an identifier the way the dialect renders it.
func sfQ(dialect, ident string) string {
	if dialect == "mysql" || dialect == "sqlite" {
		return "`" + ident + "`"
	}
	return `"` + ident + `"`
}

// sfPH is the n-th (1-based) placeholder as the dialect renders it.
func sfPH(dialect string, n int) string {
	if dialect == "postgres" {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

// sfIDs runs q (whose first column is an integer id) and returns the ids in
// ascending order. Set operations are wrapped by the caller, since ORDER BY on
// the builder would land inside the first branch.
func sfIDs(t *testing.T, q *relica.SelectQuery) []int {
	t.Helper()
	var ids []int
	require.NoError(t, q.Column(&ids))
	sort.Ints(ids)
	return ids
}

// sfOrderedIDs returns the first column in the order the database returned it.
func sfOrderedIDs(t *testing.T, q *relica.SelectQuery) []int {
	t.Helper()
	var ids []int
	require.NoError(t, q.Column(&ids))
	return ids
}

// sfBadLike is an expression carrying a construction error.
func sfBadLike() relica.LikeExp {
	return relica.Like("name", "x").EscapeChars("!")
}

// sfPanics reports whether f panics.
func sfPanics(f func()) (panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	f()
	return false
}

func runSelectFeatureTests(t *testing.T, db *relica.DB, dialect string) {
	createSelectFeatureTables(t, db)
	q := func(ident string) string { return sfQ(dialect, ident) }
	ph := func(n int) string { return sfPH(dialect, n) }

	// -----------------------------------------------------------------------
	// FromSelect
	// -----------------------------------------------------------------------

	t.Run("FromSelect_AggregateSubquery", func(t *testing.T) {
		sub := db.Select("dept_id").SelectExpr("COUNT(*) AS cnt").From("sf_emp").
			Where(relica.GreaterOrEqual("salary", 70)).
			AndWhere(relica.IsNotNull("dept_id")).
			GroupBy("dept_id")
		outer := db.Select("dept_id", "cnt").FromSelect(sub, "c").
			Where(relica.GreaterThan("cnt", 1))

		sqlStr, args := outer.ToSQL()
		assert.Contains(t, sqlStr, "FROM (SELECT "+q("dept_id")+", COUNT(*) AS cnt FROM "+q("sf_emp"))
		assert.Contains(t, sqlStr, ") AS "+q("c")+" WHERE "+q("cnt")+" > "+ph(2))
		assert.Contains(t, sqlStr, q("salary")+" >= "+ph(1))
		assert.Equal(t, []any{70, 1}, args, "subquery params come before outer params")

		type row struct {
			DeptID int   `db:"dept_id"`
			Cnt    int64 `db:"cnt"`
		}
		var rows []row
		require.NoError(t, outer.All(&rows))
		assert.Equal(t, []row{{DeptID: 1, Cnt: 2}}, rows)
	})

	t.Run("FromSelect_OuterOrderAndCount", func(t *testing.T) {
		sub := db.Select("id", "salary").From("sf_emp").Where(relica.LessThan("salary", 90))
		ids := sfOrderedIDs(t, db.Select("id").FromSelect(sub, "cheap").OrderBy("salary DESC"))
		assert.Equal(t, []int{2, 3, 4}, ids)

		n, err := db.Select().FromSelect(sub, "cheap").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
	})

	t.Run("FromSelect_EmptyAlias_Error", func(t *testing.T) {
		sub := db.Select("id").From("sf_emp")
		var ids []int
		err := db.Select("id").FromSelect(sub, "").Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FromSelect requires a non-empty alias")
		assert.Empty(t, ids)
	})

	t.Run("FromSelect_SubqueryBuildError_Propagates", func(t *testing.T) {
		sub := db.Select("id").From("sf_emp").Where(sfBadLike())
		var ids []int
		err := db.Select("id").FromSelect(sub, "s").Column(&ids)
		require.Error(t, err, "an invalid subquery must fail the outer query")
		assert.Contains(t, err.Error(), "EscapeChars")
	})

	t.Run("FromSelect_NilSubquery_NoPanic", func(t *testing.T) {
		var err error
		panicked := sfPanics(func() {
			var ids []int
			err = db.Select("id").FromSelect(nil, "s").Column(&ids)
		})
		assert.False(t, panicked)
		assert.Error(t, err)
	})

	// -----------------------------------------------------------------------
	// SelectExpr
	// -----------------------------------------------------------------------

	t.Run("SelectExpr_ComputedColumnsWithParams", func(t *testing.T) {
		query := db.Select("id").
			SelectExpr("salary - target AS diff").
			SelectExpr("CASE WHEN salary > ? THEN 1 ELSE 0 END AS high", 75).
			From("sf_emp").
			Where(relica.LessOrEqual("id", 4)).
			OrderBy("id")

		sqlStr, args := query.ToSQL()
		assert.Contains(t, sqlStr, "SELECT "+q("id")+", salary - target AS diff, CASE WHEN salary > "+ph(1)+" THEN 1 ELSE 0 END AS high FROM")
		assert.Contains(t, sqlStr, "WHERE "+q("id")+" <= "+ph(2))
		assert.Equal(t, []any{75, 4}, args, "SELECT params come before WHERE params")

		type row struct {
			ID   int `db:"id"`
			Diff int `db:"diff"`
			High int `db:"high"`
		}
		var rows []row
		require.NoError(t, query.All(&rows))
		assert.Equal(t, []row{
			{ID: 1, Diff: 10, High: 1},
			{ID: 2, Diff: 0, High: 1},
			{ID: 3, Diff: -5, High: 0},
			{ID: 4, Diff: 10, High: 0},
		}, rows)
	})

	t.Run("SelectExpr_CorrelatedScalarSubquery", func(t *testing.T) {
		type row struct {
			ID       int   `db:"id"`
			EmpCount int64 `db:"emp_count"`
		}
		var rows []row
		err := db.Select("id").
			SelectExpr("(SELECT COUNT(*) FROM sf_emp e WHERE e.dept_id = d.id) AS emp_count").
			From("sf_dept d").OrderBy("id").All(&rows)
		require.NoError(t, err)
		assert.Equal(t, []row{{ID: 1, EmpCount: 2}, {ID: 2, EmpCount: 1}, {ID: 3, EmpCount: 0}}, rows)
	})

	t.Run("SelectExpr_PlaceholderArgMismatch_Error", func(t *testing.T) {
		var ids []int
		err := db.Select("id").SelectExpr("? AS x").From("sf_emp").Column(&ids)
		require.Error(t, err, "placeholder without an argument")
		assert.Contains(t, err.Error(), "placeholders but")

		err = db.Select("id").SelectExpr("salary AS s", 5).From("sf_emp").Column(&ids)
		require.Error(t, err, "argument without a placeholder")
		assert.Contains(t, err.Error(), "placeholders but")
	})

	// -----------------------------------------------------------------------
	// AndSelect
	// -----------------------------------------------------------------------

	t.Run("AndSelect_AppendsColumns", func(t *testing.T) {
		query := db.Select("id").AndSelect("name").AndSelect("salary AS pay").
			From("sf_emp").Where(relica.Eq("id", 3))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, "SELECT "+q("id")+", "+q("name")+", "+q("salary")+" AS "+q("pay")+" FROM")

		type row struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
			Pay  int    `db:"pay"`
		}
		var r row
		require.NoError(t, query.One(&r))
		assert.Equal(t, row{ID: 3, Name: "Carol", Pay: 70}, r)
	})

	t.Run("AndSelect_OnEmptySelect_ReplacesStar", func(t *testing.T) {
		query := db.Select().AndSelect("name").From("sf_emp").Where(relica.Eq("id", 2))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, "SELECT "+q("name")+" FROM")
		assert.NotContains(t, sqlStr, "*")

		var names []string
		require.NoError(t, query.Column(&names))
		assert.Equal(t, []string{"Bob"}, names)
	})

	t.Run("AndSelect_UnknownColumn_ServerError", func(t *testing.T) {
		var ids []int
		err := db.Select("id").AndSelect("no_such_col").From("sf_emp").Column(&ids)
		require.Error(t, err)
	})

	// -----------------------------------------------------------------------
	// RightJoin / FullJoin / CrossJoin
	// -----------------------------------------------------------------------

	type joinRow struct {
		EmpID  sql.NullInt64 `db:"emp_id"`
		DeptID sql.NullInt64 `db:"dept_id"`
	}
	nullInt := func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }
	sortJoinRows := func(rows []joinRow) {
		// NULL sorts first; ORDER BY NULL placement differs between databases.
		key := func(n sql.NullInt64) int64 {
			if !n.Valid {
				return -1
			}
			return n.Int64
		}
		sort.Slice(rows, func(i, j int) bool {
			if key(rows[i].DeptID) != key(rows[j].DeptID) {
				return key(rows[i].DeptID) < key(rows[j].DeptID)
			}
			return key(rows[i].EmpID) < key(rows[j].EmpID)
		})
	}

	t.Run("RightJoin_KeepsUnmatchedRightRows", func(t *testing.T) {
		query := db.Select("e.id AS emp_id", "d.id AS dept_id").
			From("sf_emp e").
			RightJoin("sf_dept d", relica.EqCol("e.dept_id", "d.id"))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, "RIGHT JOIN "+q("sf_dept")+" AS "+q("d")+" ON "+q("e")+"."+q("dept_id")+" = "+q("d")+"."+q("id"))

		var rows []joinRow
		require.NoError(t, query.All(&rows))
		sortJoinRows(rows)
		assert.Equal(t, []joinRow{
			{EmpID: nullInt(1), DeptID: nullInt(1)},
			{EmpID: nullInt(2), DeptID: nullInt(1)},
			{EmpID: nullInt(3), DeptID: nullInt(2)},
			{EmpID: sql.NullInt64{}, DeptID: nullInt(3)}, // Legal: no employees
		}, rows)
	})

	t.Run("RightJoin_StringOnWithWhereParam", func(t *testing.T) {
		var ids []int
		err := db.Select("d.id").From("sf_emp e").
			RightJoin("sf_dept d", "e.dept_id = d.id").
			Where(relica.IsNull("e.id")).Column(&ids)
		require.NoError(t, err)
		assert.Equal(t, []int{3}, ids)
	})

	t.Run("RightJoin_InvalidOn_Error", func(t *testing.T) {
		var ids []int
		err := db.Select("d.id").From("sf_emp e").RightJoin("sf_dept d", 42).Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JOIN ON must be string, Expression, or nil")

		err = db.Select("d.id").From("sf_emp e").RightJoin("sf_dept d", sfBadLike()).Column(&ids)
		require.Error(t, err, "an invalid ON expression must fail the query")
	})

	t.Run("FullJoin_KeepsUnmatchedBothSides", func(t *testing.T) {
		query := db.Select("e.id AS emp_id", "d.id AS dept_id").
			From("sf_emp e").
			FullJoin("sf_dept d", relica.EqCol("e.dept_id", "d.id"))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, "FULL OUTER JOIN "+q("sf_dept")+" AS "+q("d")+" ON ")

		var rows []joinRow
		err := query.All(&rows)
		if dialect == "mysql" {
			// Documented: MySQL has no FULL OUTER JOIN; the server rejects the
			// statement and nothing is scanned.
			require.Error(t, err)
			assert.Empty(t, rows)
			return
		}
		require.NoError(t, err)
		sortJoinRows(rows)
		assert.Equal(t, []joinRow{
			{EmpID: nullInt(4), DeptID: sql.NullInt64{}}, // Dave: no department
			{EmpID: nullInt(1), DeptID: nullInt(1)},
			{EmpID: nullInt(2), DeptID: nullInt(1)},
			{EmpID: nullInt(3), DeptID: nullInt(2)},
			{EmpID: sql.NullInt64{}, DeptID: nullInt(3)}, // Legal: no employees
		}, rows)
	})

	t.Run("FullJoin_InvalidOn_Error", func(t *testing.T) {
		var ids []int
		err := db.Select("d.id").From("sf_emp e").FullJoin("sf_dept d", 3.14).Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JOIN ON must be string, Expression, or nil")
	})

	t.Run("CrossJoin_CartesianProduct", func(t *testing.T) {
		query := db.Select("e.id AS emp_id", "d.id AS dept_id").From("sf_emp e").CrossJoin("sf_dept d")
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, "CROSS JOIN "+q("sf_dept")+" AS "+q("d"))
		assert.NotContains(t, sqlStr, " ON ")

		n, err := db.Select().From("sf_emp e").CrossJoin("sf_dept d").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(12), n, "4 employees x 3 departments")

		var rows []joinRow
		require.NoError(t, query.Where(relica.Eq("e.id", 1)).
			AndWhere(relica.NotEqCol("e.dept_id", "d.id")).All(&rows))
		sortJoinRows(rows)
		assert.Equal(t, []joinRow{
			{EmpID: nullInt(1), DeptID: nullInt(2)},
			{EmpID: nullInt(1), DeptID: nullInt(3)},
		}, rows)
	})

	t.Run("CrossJoin_UnknownTable_ServerError", func(t *testing.T) {
		var ids []int
		err := db.Select("e.id").From("sf_emp e").CrossJoin("sf_missing m").Column(&ids)
		require.Error(t, err)
	})

	// -----------------------------------------------------------------------
	// OrderByExpr
	// -----------------------------------------------------------------------

	t.Run("OrderByExpr_CaseWithParamAfterWhereParam", func(t *testing.T) {
		query := db.Select("id").From("sf_emp").
			Where(relica.GreaterThan("salary", 0)).
			OrderByExpr("CASE WHEN id = ? THEN 0 ELSE 1 END", 3).
			OrderBy("id")
		sqlStr, args := query.ToSQL()
		assert.Contains(t, sqlStr, "WHERE "+q("salary")+" > "+ph(1))
		assert.Contains(t, sqlStr, "ORDER BY CASE WHEN id = "+ph(2)+" THEN 0 ELSE 1 END, "+q("id"))
		assert.Equal(t, []any{0, 3}, args)
		assert.Equal(t, []int{3, 1, 2, 4}, sfOrderedIDs(t, query))
	})

	t.Run("OrderByExpr_ArithmeticDescThenColumn", func(t *testing.T) {
		// salary - target: 1→10, 2→0, 3→-5, 4→10; ties broken by id.
		ids := sfOrderedIDs(t, db.Select("id").From("sf_emp").
			OrderByExpr("salary - target DESC").OrderBy("id"))
		assert.Equal(t, []int{1, 4, 2, 3}, ids)
	})

	t.Run("OrderByExpr_Invalid_Error", func(t *testing.T) {
		var ids []int
		err := db.Select("id").From("sf_emp").OrderByExpr("CASE WHEN id = ? THEN 0 END").Column(&ids)
		require.Error(t, err, "placeholder without an argument")
		assert.Contains(t, err.Error(), "placeholders but")

		err = db.Select("id").From("sf_emp").OrderByExpr("no_such_col").Column(&ids)
		require.Error(t, err, "unknown column is a server error")
	})

	// -----------------------------------------------------------------------
	// GroupBySub
	// -----------------------------------------------------------------------

	t.Run("GroupBySub_ExpressionWithParamBeforeHaving", func(t *testing.T) {
		// salary >= 75: {100, 80} → 180; otherwise {70, 60} → 130.
		build := func() *relica.SelectQuery {
			return db.Select().SelectExpr("COUNT(*) AS n").SelectExpr("SUM(salary) AS total").
				From("sf_emp").
				GroupBySub(relica.NewExp("CASE WHEN salary >= ? THEN 1 ELSE 0 END", 75))
		}
		type row struct {
			N     int64 `db:"n"`
			Total int64 `db:"total"`
		}

		var rows []row
		require.NoError(t, build().OrderBy("total").All(&rows))
		assert.Equal(t, []row{{N: 2, Total: 130}, {N: 2, Total: 180}}, rows)

		withHaving := build().Having("SUM(salary) > ?", 150)
		sqlStr, args := withHaving.ToSQL()
		assert.Contains(t, sqlStr, "GROUP BY CASE WHEN salary >= "+ph(1)+" THEN 1 ELSE 0 END HAVING SUM(salary) > "+ph(2))
		assert.Equal(t, []any{75, 150}, args)
		rows = nil
		require.NoError(t, withHaving.All(&rows))
		assert.Equal(t, []row{{N: 2, Total: 180}}, rows)
	})

	t.Run("GroupBySub_MixedWithGroupBy", func(t *testing.T) {
		type row struct {
			DeptID sql.NullInt64 `db:"dept_id"`
			N      int64         `db:"n"`
		}
		var rows []row
		err := db.Select("dept_id").SelectExpr("COUNT(*) AS n").From("sf_emp").
			Where(relica.IsNotNull("dept_id")).
			GroupBySub(relica.NewExp("dept_id")).
			OrderBy("dept_id").All(&rows)
		require.NoError(t, err)
		assert.Equal(t, []row{{DeptID: nullInt(1), N: 2}, {DeptID: nullInt(2), N: 1}}, rows)
	})

	t.Run("GroupBySub_InvalidExpression_Error", func(t *testing.T) {
		var ns []int64
		err := db.Select().SelectExpr("COUNT(*)").From("sf_emp").GroupBySub(sfBadLike()).Column(&ns)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "EscapeChars")
	})

	// -----------------------------------------------------------------------
	// Union / UnionAll / Intersect / Except
	//
	// Most results are read through FromSelect with an outer ORDER BY (the
	// way to sort by an expression); OrderBy after Union sorts the combined
	// result directly (SetOperations_CombinedOrderLimit).
	// -----------------------------------------------------------------------

	lowIDs := func() *relica.SelectQuery { // {1, 2}
		return db.Select("id").From("sf_emp").Where(relica.LessOrEqual("id", 2))
	}
	midIDs := func() *relica.SelectQuery { // {2, 3}
		return db.Select("id").From("sf_emp").Where(relica.Between("id", 2, 3))
	}
	ordered := func(compound *relica.SelectQuery) *relica.SelectQuery {
		return db.Select("id").FromSelect(compound, "u").OrderBy("id")
	}
	// setOp is how an operator joins two plain branches: PostgreSQL and MySQL
	// parenthesize every branch, SQLite's compound-select grammar takes none.
	setOp := func(op string) string {
		if dialect == "sqlite" {
			return " " + op + " SELECT "
		}
		return " " + op + " (SELECT "
	}

	t.Run("Union_RemovesDuplicates", func(t *testing.T) {
		u := lowIDs().Union(midIDs())
		sqlStr, args := u.ToSQL()
		assert.Contains(t, sqlStr, setOp("UNION"))
		assert.NotContains(t, sqlStr, "UNION ALL")
		assert.Equal(t, []any{2, 2, 3}, args)
		if dialect == "postgres" {
			assert.Contains(t, sqlStr, "<= $1) UNION (")
			assert.Contains(t, sqlStr, "BETWEEN $2 AND $3)")
		}
		assert.Equal(t, []int{1, 2, 3}, sfOrderedIDs(t, ordered(u)))
	})

	t.Run("UnionAll_KeepsDuplicates", func(t *testing.T) {
		u := lowIDs().UnionAll(midIDs())
		sqlStr, _ := u.ToSQL()
		assert.Contains(t, sqlStr, setOp("UNION ALL"))
		assert.Equal(t, []int{1, 2, 2, 3}, sfOrderedIDs(t, ordered(u)))
		// Chained: three branches.
		u3 := lowIDs().UnionAll(midIDs()).UnionAll(db.Select("id").From("sf_emp").Where(relica.Eq("id", 4)))
		assert.Equal(t, []int{1, 2, 2, 3, 4}, sfOrderedIDs(t, ordered(u3)))
	})

	t.Run("Intersect_RowsInBoth", func(t *testing.T) {
		// mysql:8.0 in CI resolves to the latest 8.0.x (>= 8.0.31), which has INTERSECT.
		u := lowIDs().Intersect(midIDs())
		sqlStr, _ := u.ToSQL()
		assert.Contains(t, sqlStr, setOp("INTERSECT"))
		assert.Equal(t, []int{2}, sfOrderedIDs(t, ordered(u)))
	})

	t.Run("Except_RowsOnlyInFirst", func(t *testing.T) {
		u := lowIDs().Except(midIDs())
		sqlStr, _ := u.ToSQL()
		assert.Contains(t, sqlStr, setOp("EXCEPT"))
		assert.Equal(t, []int{1}, sfOrderedIDs(t, ordered(u)))
		// Reversed operands.
		assert.Equal(t, []int{3}, sfOrderedIDs(t, ordered(midIDs().Except(lowIDs()))))
	})

	t.Run("SetOperations_DirectExecution", func(t *testing.T) {
		// Without the FromSelect wrapper: the compound statement is sent as built.
		assert.Equal(t, []int{1, 2, 3}, sfIDs(t, lowIDs().Union(midIDs())))
		assert.Equal(t, []int{1, 2, 2, 3}, sfIDs(t, lowIDs().UnionAll(midIDs())))
		assert.Equal(t, []int{2}, sfIDs(t, lowIDs().Intersect(midIDs())))
		assert.Equal(t, []int{1}, sfIDs(t, lowIDs().Except(midIDs())))
	})

	t.Run("SetOperations_BranchOrderLimit", func(t *testing.T) {
		// ORDER BY/LIMIT belong to their own branch: the first two ids and the
		// last one. SQLite takes them only inside a subquery operand.
		first2 := db.Select("id").From("sf_emp").OrderBy("id").Limit(2)
		last1 := db.Select("id").From("sf_emp").OrderBy("id DESC").Limit(1)
		assert.Equal(t, []int{1, 2, 4}, sfOrderedIDs(t, ordered(first2.UnionAll(last1))))
		skip1 := db.Select("id").From("sf_emp").OrderBy("id").Limit(10).Offset(3)
		assert.Equal(t, []int{2, 4}, sfOrderedIDs(t, ordered(midIDs().Except(db.Select("id").From("sf_emp").
			Where(relica.Eq("id", 3))).Union(skip1))))
	})

	t.Run("SetOperations_NestedBranchKeepsGrouping", func(t *testing.T) {
		// A EXCEPT (B UNION C) = {1,2} - {2,3,4} = {1}; read left to right without
		// the grouping it would be (A EXCEPT B) UNION C = {1,4}.
		four := db.Select("id").From("sf_emp").Where(relica.Eq("id", 4))
		assert.Equal(t, []int{1}, sfOrderedIDs(t, ordered(lowIDs().Except(midIDs().Union(four)))))
	})

	t.Run("SetOperations_CombinedOrderLimit", func(t *testing.T) {
		// OrderBy/Limit/Offset after Union apply to the combined result {1,2,3}.
		assert.Equal(t, []int{3, 2}, sfOrderedIDs(t, lowIDs().Union(midIDs()).OrderBy("id DESC").Limit(2)))
		assert.Equal(t, []int{2}, sfOrderedIDs(t, lowIDs().Union(midIDs()).OrderBy("id").Limit(1).Offset(1)))
		// Before Union they choose the first branch's rows ({4}), after it sort the whole.
		last1 := db.Select("id").From("sf_emp").OrderBy("id DESC").Limit(1)
		assert.Equal(t, []int{1, 2, 4}, sfOrderedIDs(t, last1.Union(lowIDs()).OrderBy("id")))
		// Sorting by an expression goes through FromSelect: PostgreSQL takes only
		// output column names or numbers in a compound's ORDER BY.
		assert.Equal(t, []int{2, 1, 3}, sfOrderedIDs(t,
			db.Select("id").FromSelect(lowIDs().Union(midIDs()), "u").OrderByExpr("id % 2, id")))
	})

	t.Run("SetOperations_CountIgnoresCombinedPaging", func(t *testing.T) {
		paged := lowIDs().Union(midIDs()).OrderBy("id").Limit(1)
		n, err := paged.Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n, "a paged union reports its total")
		ok, err := paged.Exists()
		require.NoError(t, err)
		assert.True(t, ok)
		// The first branch's LIMIT changes the set, so it counts: {4} ∪ {1,2}.
		n, err = db.Select("id").From("sf_emp").OrderBy("id DESC").Limit(1).Union(lowIDs()).Limit(1).Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
	})

	t.Run("SetOperations_IntersectAfterUnion_LeftToRight", func(t *testing.T) {
		// ({1,2} ∪ {4}) ∩ {2,3} = {2} on every database. Sent flat, PostgreSQL
		// and MySQL bind INTERSECT first: {1,2} ∪ ({4} ∩ {2,3}) = {1,2}.
		four := db.Select("id").From("sf_emp").Where(relica.Eq("id", 4))
		assert.Equal(t, []int{2}, sfOrderedIDs(t, lowIDs().Union(four).Intersect(midIDs()).OrderBy("id")))
		// The other grouping is written as a nested branch.
		assert.Equal(t, []int{1, 2}, sfOrderedIDs(t, lowIDs().Union(four.Intersect(midIDs())).OrderBy("id")))
	})

	t.Run("SetOperations_AmbiguousOrder_Errors", func(t *testing.T) {
		var ids []int
		err := db.Select("id").From("sf_emp").OrderBy("id").Union(midIDs()).Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "has no effect")
		err = lowIDs().Union(midIDs()).OrderBy("id").Union(lowIDs()).Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "combined result")
		assert.Empty(t, ids)
	})

	t.Run("SetOperations_ColumnCountMismatch_ServerError", func(t *testing.T) {
		two := func() *relica.SelectQuery { return db.Select("id", "name").From("sf_emp") }
		for name, compound := range map[string]*relica.SelectQuery{
			"Union":     two().Union(lowIDs()),
			"UnionAll":  two().UnionAll(lowIDs()),
			"Intersect": two().Intersect(lowIDs()),
			"Except":    two().Except(lowIDs()),
		} {
			var rows []relica.NullStringMap
			err := compound.All(&rows)
			assert.Error(t, err, name)
			assert.Empty(t, rows, name)
		}
	})

	t.Run("SetOperations_BranchBuildError_Propagates", func(t *testing.T) {
		bad := func() *relica.SelectQuery { return db.Select("id").From("sf_emp").Where(sfBadLike()) }
		for name, compound := range map[string]*relica.SelectQuery{
			"Union":     lowIDs().Union(bad()),
			"UnionAll":  lowIDs().UnionAll(bad()),
			"Intersect": lowIDs().Intersect(bad()),
			"Except":    lowIDs().Except(bad()),
		} {
			var ids []int
			err := compound.Column(&ids)
			require.Error(t, err, "%s: an invalid branch must fail the query before it is sent", name)
			assert.Contains(t, err.Error(), "EscapeChars", name)
		}
	})

	t.Run("SetOperations_NilOther_NoPanic", func(t *testing.T) {
		for name, op := range map[string]func(*relica.SelectQuery) *relica.SelectQuery{
			"Union":     func(o *relica.SelectQuery) *relica.SelectQuery { return lowIDs().Union(o) },
			"UnionAll":  func(o *relica.SelectQuery) *relica.SelectQuery { return lowIDs().UnionAll(o) },
			"Intersect": func(o *relica.SelectQuery) *relica.SelectQuery { return lowIDs().Intersect(o) },
			"Except":    func(o *relica.SelectQuery) *relica.SelectQuery { return lowIDs().Except(o) },
		} {
			var err error
			assert.False(t, sfPanics(func() {
				var ids []int
				err = op(nil).Column(&ids)
			}), name)
			require.Error(t, err, "%s(nil) must fail the query, not drop the branch", name)
			assert.Contains(t, err.Error(), "non-nil", name)
		}
	})

	// -----------------------------------------------------------------------
	// WithRecursive
	// -----------------------------------------------------------------------

	subtree := func(rootID, maxDepth int) *relica.SelectQuery {
		anchor := db.Select("id", "parent_id").SelectExpr("0 AS depth").
			From("sf_tree").Where(relica.Eq("id", rootID))
		recursive := db.Select("t.id", "t.parent_id").SelectExpr("c.depth + 1").
			From("sf_tree t").
			InnerJoin("sf_subtree c", relica.EqCol("t.parent_id", "c.id")).
			Where(relica.LessThan("c.depth", maxDepth))
		return anchor.UnionAll(recursive)
	}
	type treeRow struct {
		ID    int `db:"id"`
		Depth int `db:"depth"`
	}

	t.Run("WithRecursive_Subtree", func(t *testing.T) {
		query := db.Select("id", "depth").WithRecursive("sf_subtree", subtree(1, 10)).
			From("sf_subtree").OrderBy("depth", "id")
		sqlStr, args := query.ToSQL()
		assert.Contains(t, sqlStr, "WITH RECURSIVE "+q("sf_subtree")+" AS (")
		assert.Equal(t, []any{1, 10}, args)
		if dialect == "postgres" {
			assert.Contains(t, sqlStr, `"id" = $1`)
			assert.Contains(t, sqlStr, `"c"."depth" < $2`)
		}

		var rows []treeRow
		require.NoError(t, query.All(&rows))
		assert.Equal(t, []treeRow{{1, 0}, {2, 1}, {3, 1}, {4, 2}}, rows)
	})

	t.Run("WithRecursive_DepthLimitAndOuterParam", func(t *testing.T) {
		// Params: anchor id ($1), recursive depth ($2), outer WHERE ($3).
		query := db.Select("id", "depth").WithRecursive("sf_subtree", subtree(1, 1)).
			From("sf_subtree").Where(relica.NotEq("id", 3)).OrderBy("id")
		_, args := query.ToSQL()
		assert.Equal(t, []any{1, 1, 3}, args)
		var rows []treeRow
		require.NoError(t, query.All(&rows))
		assert.Equal(t, []treeRow{{1, 0}, {2, 1}}, rows)

		// The other root's subtree.
		rows = nil
		require.NoError(t, db.Select("id", "depth").WithRecursive("sf_subtree", subtree(5, 10)).
			From("sf_subtree").OrderBy("id").All(&rows))
		assert.Equal(t, []treeRow{{5, 0}, {6, 1}}, rows)
	})

	t.Run("WithRecursive_InvalidInput_Error", func(t *testing.T) {
		var ids []int
		noUnion := db.Select("id").From("sf_tree")
		err := db.Select("id").WithRecursive("sf_subtree", noUnion).From("sf_subtree").Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a query with UNION or UNION ALL")

		err = db.Select("id").WithRecursive("", subtree(1, 10)).From("sf_subtree").Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-empty CTE name")
	})

	t.Run("WithRecursive_CTEBuildError_Propagates", func(t *testing.T) {
		anchor := db.Select("id", "parent_id").SelectExpr("0 AS depth").From("sf_tree").Where(relica.Eq("id", 1))
		badRecursive := db.Select("t.id", "t.parent_id").SelectExpr("c.depth + 1").
			From("sf_tree t").InnerJoin("sf_subtree c", relica.EqCol("t.parent_id", "c.id")).
			Where(sfBadLike())
		var ids []int
		err := db.Select("id").WithRecursive("sf_subtree", anchor.UnionAll(badRecursive)).
			From("sf_subtree").Column(&ids)
		require.Error(t, err, "an invalid CTE must fail the query before it is sent")
		assert.Contains(t, err.Error(), "EscapeChars")
	})

	t.Run("WithRecursive_NilQuery_NoPanic", func(t *testing.T) {
		var err error
		panicked := sfPanics(func() {
			var ids []int
			err = db.Select("id").WithRecursive("sf_subtree", nil).From("sf_subtree").Column(&ids)
		})
		assert.False(t, panicked)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-nil CTE query")

		panicked = sfPanics(func() {
			var ids []int
			err = db.Select("id").With("sf_subtree", nil).From("sf_subtree").Column(&ids)
		})
		assert.False(t, panicked)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-nil CTE query")
	})

	t.Run("With_CTEBuildError_Propagates", func(t *testing.T) {
		var ids []int
		bad := db.Select("id").From("sf_emp").Where(sfBadLike())
		err := db.Select("id").With("sf_e", bad).From("sf_e").Column(&ids)
		require.Error(t, err, "an invalid CTE must fail the query before it is sent")
		assert.Contains(t, err.Error(), "EscapeChars")
	})

	t.Run("NestedBuildError_Propagates", func(t *testing.T) {
		// The error sits two levels down: a set-operation branch inside a
		// FromSelect subquery, and the same inside an IN subquery.
		bad := db.Select("id").From("sf_emp").Where(sfBadLike())
		var ids []int
		err := db.Select("id").FromSelect(lowIDs().Union(bad), "u").Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "EscapeChars")

		bad = db.Select("id").From("sf_emp").Where(sfBadLike())
		err = db.Select("id").From("sf_emp").Where(relica.In("id", lowIDs().Union(bad).AsExpression())).Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "EscapeChars")
	})

	// -----------------------------------------------------------------------
	// SelectQuery.Exists (method)
	// -----------------------------------------------------------------------

	t.Run("ExistsMethod_TrueAndFalse", func(t *testing.T) {
		ok, err := db.Select().From("sf_emp").Where(relica.Eq("dept_id", 2)).Exists()
		require.NoError(t, err)
		assert.True(t, ok)

		ok, err = db.Select().From("sf_emp").Where(relica.Eq("dept_id", 99)).Exists()
		require.NoError(t, err)
		assert.False(t, ok)

		// With a join and a param in the ON clause and in WHERE.
		ok, err = db.Select().From("sf_dept d").
			LeftJoin("sf_emp e", relica.And(relica.EqCol("e.dept_id", "d.id"), relica.GreaterThan("e.salary", 0))).
			Where(relica.Eq("d.name", "Legal")).AndWhere(relica.IsNotNull("e.id")).Exists()
		require.NoError(t, err)
		assert.False(t, ok, "Legal has no employees")

		ok, err = db.Select().From("sf_dept d").
			LeftJoin("sf_emp e", relica.And(relica.EqCol("e.dept_id", "d.id"), relica.GreaterThan("e.salary", 0))).
			Where(relica.Eq("d.name", "Eng")).AndWhere(relica.IsNotNull("e.id")).Exists()
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("ExistsMethod_UnknownTable_ServerError", func(t *testing.T) {
		ok, err := db.Select().From("sf_missing").Exists()
		require.Error(t, err)
		assert.False(t, ok)
	})

	t.Run("ExistsMethod_InvalidFilter_Error", func(t *testing.T) {
		ok, err := db.Select().From("sf_emp").Where(sfBadLike()).Exists()
		assert.Error(t, err, "an invalid filter must fail, not run an unfiltered EXISTS")
		assert.False(t, ok)

		bad := db.Select("id").From("sf_emp").Where(sfBadLike())
		ok, err = db.Select("id").FromSelect(bad, "s").Exists()
		assert.Error(t, err, "an invalid subquery must fail EXISTS")
		assert.False(t, ok)
	})

	t.Run("CountMethod_InvalidFilter_Error", func(t *testing.T) {
		n, err := db.Select().From("sf_emp").Where(sfBadLike()).Count()
		require.Error(t, err, "an invalid filter must fail, not count every row")
		assert.Contains(t, err.Error(), "EscapeChars")
		assert.Zero(t, n)

		bad := db.Select("id").From("sf_emp").Where(sfBadLike())
		n, err = db.Select().FromSelect(bad, "s").Count()
		require.Error(t, err, "an invalid subquery must fail Count")
		assert.Zero(t, n)
	})

	// Count is the number of rows All would return, and Exists whether there
	// is one; ORDER BY, LIMIT and OFFSET are ignored (pagination keeps its
	// total). Data: dept 1 = Alice, Bob; dept 2 = Carol; NULL = Dave.
	t.Run("CountMethod_FollowsWhatAllReturns", func(t *testing.T) {
		count := func(q *relica.SelectQuery) int64 {
			t.Helper()
			n, err := q.Count()
			require.NoError(t, err)
			return n
		}
		emp := func(cols ...string) *relica.SelectQuery { return db.Select(cols...).From("sf_emp") }

		assert.Equal(t, int64(3), count(emp("dept_id").GroupBy("dept_id")), "GROUP BY: number of groups")
		assert.Equal(t, int64(1), count(emp("dept_id").GroupBy("dept_id").Having("COUNT(*) > ?", 1)), "HAVING")
		assert.Equal(t, int64(3), count(emp("dept_id").Distinct()), "DISTINCT")
		assert.Equal(t, int64(2), count(emp("id").Where(relica.Eq("dept_id", 1)).OrderBy("id").Limit(1).Offset(1)),
			"LIMIT/OFFSET ignored")
		assert.Equal(t, int64(2), count(emp("id").SelectExpr("salary + ?", 1).Where(relica.Eq("dept_id", 1))),
			"select-list params do not shift the WHERE params")
		assert.Equal(t, int64(2), count(db.Select().
			With("sf_hi", emp("id").Where(relica.GreaterThan("salary", 75))).From("sf_hi")), "CTE")

		assert.Equal(t, int64(3), count(lowIDs().Union(midIDs())), "UNION")
		assert.Equal(t, int64(4), count(lowIDs().UnionAll(midIDs())), "UNION ALL")
	})

	t.Run("ExistsMethod_FollowsWhatAllReturns", func(t *testing.T) {
		exists := func(q *relica.SelectQuery) bool {
			t.Helper()
			ok, err := q.Exists()
			require.NoError(t, err)
			return ok
		}
		emp := func(cols ...string) *relica.SelectQuery { return db.Select(cols...).From("sf_emp") }

		assert.False(t, exists(emp("dept_id").GroupBy("dept_id").Having("COUNT(*) > ?", 5)), "HAVING filters every group")
		assert.True(t, exists(emp("dept_id").GroupBy("dept_id").Having("COUNT(*) > ?", 1)))
		assert.True(t, exists(emp("id").Where(relica.Eq("dept_id", 1)).Offset(5)), "OFFSET ignored")
		assert.True(t, exists(db.Select().
			With("sf_hi", emp("id").Where(relica.GreaterThan("salary", 75))).From("sf_hi")), "CTE")
		assert.False(t, exists(db.Select().
			With("sf_hi", emp("id").Where(relica.GreaterThan("salary", 500))).From("sf_hi")), "empty CTE")

		none := emp("id").Where(relica.Eq("id", -1))
		assert.True(t, exists(none.Union(emp("id").Where(relica.Eq("id", 1)))), "a later branch has rows")
	})

	// -----------------------------------------------------------------------
	// AsExpression + Exists / NotExists (expressions)
	// -----------------------------------------------------------------------

	correlated := func() *relica.SelectQuery {
		return db.Select().SelectExpr("1").From("sf_emp e").Where(relica.EqCol("e.dept_id", "d.id"))
	}

	t.Run("Exists_Correlated", func(t *testing.T) {
		query := db.Select("id").From("sf_dept d").Where(relica.Exists(correlated().AsExpression()))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, "WHERE EXISTS (SELECT 1 FROM "+q("sf_emp")+" AS "+q("e")+" WHERE "+
			q("e")+"."+q("dept_id")+" = "+q("d")+"."+q("id")+")")
		assert.Equal(t, []int{1, 2}, sfIDs(t, query))
	})

	t.Run("NotExists_Correlated", func(t *testing.T) {
		query := db.Select("id").From("sf_dept d").Where(relica.NotExists(correlated().AsExpression()))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, "WHERE NOT EXISTS (SELECT 1 FROM ")
		assert.Equal(t, []int{3}, sfIDs(t, query))
	})

	t.Run("Exists_ParamsInsideAndOutside", func(t *testing.T) {
		sub := correlated().AndWhere(relica.GreaterThan("e.salary", 75))
		query := db.Select("id").From("sf_dept d").
			Where(relica.NotEq("d.name", "Sales")).
			AndWhere(relica.Exists(sub.AsExpression())).
			AndWhere(relica.LessThan("d.id", 10))
		sqlStr, args := query.ToSQL()
		assert.Equal(t, []any{"Sales", 75, 10}, args)
		if dialect == "postgres" {
			assert.Contains(t, sqlStr, `"d"."name" <> $1`)
			assert.Contains(t, sqlStr, `"e"."salary" > $2`)
			assert.Contains(t, sqlStr, `"d"."id" < $3`)
		}
		assert.Equal(t, []int{1}, sfIDs(t, query))

		notQuery := db.Select("id").From("sf_dept d").Where(relica.NotExists(sub.AsExpression()))
		assert.Equal(t, []int{2, 3}, sfIDs(t, notQuery))
	})

	t.Run("Exists_NilSubquery", func(t *testing.T) {
		// EXISTS of nothing is false; NOT EXISTS of nothing is no condition.
		assert.Empty(t, sfIDs(t, db.Select("id").From("sf_dept").Where(relica.Exists(nil))))
		assert.Equal(t, []int{1, 2, 3}, sfIDs(t, db.Select("id").From("sf_dept").Where(relica.NotExists(nil))))
	})

	t.Run("Exists_SubqueryBuildError_Propagates", func(t *testing.T) {
		bad := db.Select().SelectExpr("1").From("sf_emp").Where(sfBadLike())
		var ids []int
		err := db.Select("id").From("sf_dept").Where(relica.Exists(bad.AsExpression())).Column(&ids)
		require.Error(t, err)
		err = db.Select("id").From("sf_dept").Where(relica.NotExists(bad.AsExpression())).Column(&ids)
		require.Error(t, err, "an invalid NOT EXISTS subquery must fail, not match every row")
	})

	t.Run("AsExpression_InSubquery", func(t *testing.T) {
		sub := db.Select("dept_id").From("sf_emp").Where(relica.GreaterThan("salary", 75))
		query := db.Select("id").From("sf_dept").Where(relica.In("id", sub.AsExpression()))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, q("id")+" IN (SELECT "+q("dept_id")+" FROM "+q("sf_emp"))
		assert.Equal(t, []int{1}, sfIDs(t, query))

		notIn := db.Select("id").From("sf_dept").Where(relica.NotIn("id",
			db.Select("dept_id").From("sf_emp").Where(relica.IsNotNull("dept_id")).AsExpression()))
		assert.Equal(t, []int{3}, sfIDs(t, notIn))
	})

	t.Run("AsExpression_ScalarComparisonAndSelectSub", func(t *testing.T) {
		maxDept := db.Select().SelectExpr("MAX(dept_id)").From("sf_emp")
		assert.Equal(t, []int{2}, sfIDs(t, db.Select("id").From("sf_dept").Where(relica.Eq("id", maxDept.AsExpression()))))

		type row struct {
			ID     int   `db:"id"`
			MaxPay int64 `db:"max_pay"`
		}
		var rows []row
		maxPay := db.Select().SelectExpr("MAX(salary)").From("sf_emp e").Where(relica.EqCol("e.dept_id", "d.id"))
		err := db.Select("id").SelectSub(maxPay.AsExpression(), "max_pay").From("sf_dept d").
			Where(relica.Exists(correlated().AsExpression())).OrderBy("id").All(&rows)
		require.NoError(t, err)
		assert.Equal(t, []row{{ID: 1, MaxPay: 100}, {ID: 2, MaxPay: 70}}, rows)
	})

	t.Run("AsExpression_BuildError_Propagates", func(t *testing.T) {
		bad := db.Select("dept_id").FromSelect(db.Select("dept_id").From("sf_emp"), "")
		var ids []int
		err := db.Select("id").From("sf_dept").Where(relica.In("id", bad.AsExpression())).Column(&ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FromSelect requires a non-empty alias")
	})

	// -----------------------------------------------------------------------
	// Column-to-column comparisons
	// -----------------------------------------------------------------------

	emp := func() *relica.SelectQuery { return db.Select("id").From("sf_emp") }

	t.Run("NotEqCol", func(t *testing.T) {
		query := emp().Where(relica.NotEqCol("salary", "target"))
		sqlStr, args := query.ToSQL()
		assert.Contains(t, sqlStr, q("salary")+" <> "+q("target"))
		assert.Empty(t, args)
		assert.Equal(t, []int{1, 3, 4}, sfIDs(t, query))
	})

	t.Run("GreaterThanCol", func(t *testing.T) {
		query := emp().Where(relica.GreaterThanCol("salary", "target"))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, q("salary")+" > "+q("target"))
		assert.Equal(t, []int{1, 4}, sfIDs(t, query))
	})

	t.Run("LessThanCol", func(t *testing.T) {
		query := emp().Where(relica.LessThanCol("salary", "target"))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, q("salary")+" < "+q("target"))
		assert.Equal(t, []int{3}, sfIDs(t, query))
	})

	t.Run("LessOrEqualCol", func(t *testing.T) {
		query := emp().Where(relica.LessOrEqualCol("salary", "target"))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, q("salary")+" <= "+q("target"))
		assert.Equal(t, []int{2, 3}, sfIDs(t, query))
	})

	t.Run("ColumnCompare_QualifiedSelfJoin", func(t *testing.T) {
		// Pairs (a, b) in the same department where a earns more than b.
		type pair struct {
			A int `db:"a"`
			B int `db:"b"`
		}
		query := db.Select("a.id AS a", "b.id AS b").From("sf_emp a").
			InnerJoin("sf_emp b", relica.And(relica.EqCol("a.dept_id", "b.dept_id"), relica.NotEqCol("a.id", "b.id"))).
			Where(relica.GreaterThanCol("a.salary", "b.salary"))
		sqlStr, _ := query.ToSQL()
		assert.Contains(t, sqlStr, q("a")+"."+q("salary")+" > "+q("b")+"."+q("salary"))
		var pairs []pair
		require.NoError(t, query.All(&pairs))
		assert.Equal(t, []pair{{A: 1, B: 2}}, pairs)

		// LessThanCol and LessOrEqualCol are the mirror images.
		assert.Equal(t, []int{2}, sfIDs(t, db.Select("a.id").From("sf_emp a").
			InnerJoin("sf_emp b", relica.And(relica.EqCol("a.dept_id", "b.dept_id"), relica.NotEqCol("a.id", "b.id"))).
			Where(relica.LessThanCol("a.salary", "b.salary"))))
		assert.Equal(t, []int{1, 2, 3}, sfIDs(t, db.Select("a.id").From("sf_emp a").
			InnerJoin("sf_emp b", relica.EqCol("a.id", "b.id")).
			Where(relica.And(relica.LessOrEqualCol("a.salary", "b.salary"), relica.IsNotNull("a.dept_id")))))
	})

	t.Run("ColumnCompare_UnknownColumn_ServerError", func(t *testing.T) {
		// Qualified: an unknown table-qualified column fails on every database
		// (bare ones too since SQLite quotes with backticks, see identifier_typos).
		var ids []int
		err := emp().Where(relica.GreaterThanCol("salary", "sf_emp.no_such_col")).Column(&ids)
		require.Error(t, err)
		err = emp().Where(relica.NotEqCol("sf_emp.no_such_col", "target")).Column(&ids)
		require.Error(t, err)
	})

	// -----------------------------------------------------------------------
	// IsNotNull
	// -----------------------------------------------------------------------

	t.Run("IsNotNull", func(t *testing.T) {
		query := emp().Where(relica.IsNotNull("email"))
		sqlStr, args := query.ToSQL()
		assert.Contains(t, sqlStr, q("email")+" IS NOT NULL")
		assert.Empty(t, args)
		assert.Equal(t, []int{1, 3, 4}, sfIDs(t, query))

		assert.Equal(t, []int{1, 2, 3}, sfIDs(t, emp().Where(relica.IsNotNull("dept_id"))))
		// Combined with a parameterized condition.
		assert.Equal(t, []int{1, 4}, sfIDs(t, emp().Where(relica.And(
			relica.IsNotNull("email"), relica.Like("email", "x.com")))))
	})

	// -----------------------------------------------------------------------
	// OrLike / OrNotLike
	// -----------------------------------------------------------------------

	t.Run("OrLike", func(t *testing.T) {
		query := emp().Where(relica.OrLike("name", "li", "ar"))
		sqlStr, args := query.ToSQL()
		assert.Contains(t, sqlStr, " OR ")
		assert.Equal(t, []any{"%li%", "%ar%"}, args)
		assert.Equal(t, []int{1, 3}, sfIDs(t, query))

		// Prefix match on either value.
		assert.Equal(t, []int{1, 4}, sfIDs(t, emp().Where(relica.OrLike("name", "A", "D").Match(false, true))))
		// One value behaves like Like.
		assert.Equal(t, []int{2}, sfIDs(t, emp().Where(relica.OrLike("name", "Bo"))))
		// Wildcards in values are literals.
		assert.Empty(t, sfIDs(t, emp().Where(relica.OrLike("email", "a%x", "_@"))))
	})

	t.Run("OrNotLike", func(t *testing.T) {
		// email NOT LIKE '%x.com%' OR email NOT LIKE '%a@%':
		// Alice a@x.com matches both patterns → false; Bob NULL → unknown;
		// Carol → true; Dave d@x.com → true via the second pattern.
		query := emp().Where(relica.OrNotLike("email", "x.com", "a@"))
		_, args := query.ToSQL()
		assert.Equal(t, []any{"%x.com%", "%a@%"}, args)
		assert.Equal(t, []int{3, 4}, sfIDs(t, query))

		// Contrast: NotLike combines with AND.
		assert.Equal(t, []int{3}, sfIDs(t, emp().Where(relica.NotLike("email", "x.com", "a@"))))
	})

	t.Run("OrLike_OrNotLike_InvalidEscape_Error", func(t *testing.T) {
		for name, exp := range map[string]relica.LikeExp{
			"OrLike":    relica.OrLike("name", "a", "b").EscapeChars("!"),
			"OrNotLike": relica.OrNotLike("name", "a", "b").EscapeChars("!"),
		} {
			require.Error(t, exp.Err(), name)
			var ids []int
			err := emp().Where(exp).Column(&ids)
			assert.Error(t, err, name)
			assert.Empty(t, ids, name)
		}
	})
}

// ---------------------------------------------------------------------------
// Entry points — one per database; CI selects the database with RELICA_TEST_DB.
// ---------------------------------------------------------------------------

func TestSelectFeatures_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runSelectFeatureTests(t, ds.DB, "postgres")
}

func TestSelectFeatures_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runSelectFeatureTests(t, ds.DB, "mysql")
}

func TestSelectFeatures_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "sf.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runSelectFeatureTests(t, db, "sqlite")
}
