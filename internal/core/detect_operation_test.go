package core

import "testing"

// TestDetectOperation_CTEsAndCompound verifies that a WITH statement is
// classified by the statement after its CTEs (P5: WITH ... DELETE was reported
// as SELECT) and that a compound select starting with a parenthesis is a
// SELECT (it was UNKNOWN).
func TestDetectOperation_CTEsAndCompound(t *testing.T) {
	for _, tc := range []struct{ sql, want string }{
		{"WITH old AS (SELECT id FROM t WHERE id = $1) DELETE FROM t WHERE id IN (SELECT id FROM old)", "DELETE"},
		{"with c as (select 1) update t set x = 1", "UPDATE"},
		{"WITH c AS (SELECT 1) INSERT INTO t SELECT * FROM c", "INSERT"},
		{"WITH c AS (SELECT 1 AS n) SELECT n FROM c", "SELECT"},
		{"WITH RECURSIVE \"tree\"(id, depth) AS (SELECT 1, 0 UNION ALL SELECT id + 1, depth + 1 FROM tree) SELECT * FROM tree", "SELECT"},
		{"WITH a AS MATERIALIZED (SELECT 1), b AS NOT MATERIALIZED (SELECT 2) DELETE FROM t", "DELETE"},
		{"WITH `a` AS (SELECT ')' AS p, \"(\" AS q /* ) */ -- )\n FROM t) UPDATE t SET x = 1", "UPDATE"},
		{"WITH c AS (SELECT 1) (SELECT n FROM c) UNION (SELECT 2)", "SELECT"},
		{`(SELECT "id" FROM "a") UNION (SELECT "id" FROM "b")`, "SELECT"},
		{"  ((SELECT 1))", "SELECT"},
		{"SELECTED_ROWS", "UNKNOWN"},
		{"DELETE_LOG", "UNKNOWN"},
		{"WITH broken AS SELECT 1", "UNKNOWN"},
		{"WITH c AS (SELECT 1", "UNKNOWN"},
		{"WITH", "UNKNOWN"},
		{"", "UNKNOWN"},
	} {
		if got := DetectOperation(tc.sql); got != tc.want {
			t.Errorf("DetectOperation(%q) = %s, want %s", tc.sql, got, tc.want)
		}
	}
}
