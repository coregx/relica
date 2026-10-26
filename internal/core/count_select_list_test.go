package core

import "testing"

// TestCount_SelectListWithExpression verifies F2 (Fable review #2a): only a
// list of plain column references is replaced by COUNT(*); a list with a
// function call, SelectExpr or SelectSub is counted as a derived table,
// because it may aggregate (MAX(x) without GROUP BY returns one row).
func TestCount_SelectListWithExpression(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	for _, tc := range []struct {
		name string
		q    *SelectQuery
		want string
	}{
		{"plain", qb.Select("id", "t.name AS n").From("t").Where(Eq("a", 1)),
			`SELECT COUNT(*) FROM "t" WHERE "a" = $1`},
		{"function call", qb.Select("MAX(x)").From("t"),
			`SELECT COUNT(*) FROM (SELECT MAX(x) FROM "t") AS relica_count`},
		{"SelectExpr with param", qb.Select("id").SelectExpr("x * ? AS y", 2).From("t").Where(Eq("a", 1)),
			`SELECT COUNT(*) FROM (SELECT "id", x * $1 AS y FROM "t" WHERE "a" = $2) AS relica_count`},
	} {
		got, _ := tc.q.countStatement()
		if got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.name, got, tc.want)
		}
	}
}
