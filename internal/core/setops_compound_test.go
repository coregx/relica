package core

import (
	"reflect"
	"strings"
	"testing"
)

// Set operations (v0.20.0): call order decides what ORDER BY, LIMIT and OFFSET
// belong to. Called before the first Union/UnionAll/Intersect/Except they
// belong to the first branch; called after it, to the combined result — as the
// SQL text reads and as jOOQ, Laravel, goqu and SQLAlchemy define it.

func setOpsBuilder(dialect string) *QueryBuilder {
	return &QueryBuilder{db: mockDB(dialect)}
}

func TestSetOps_OrderLimitOffsetAfterUnionApplyToTheCompound(t *testing.T) {
	for _, tc := range []struct {
		dialect, want string
	}{
		{"postgres", `(SELECT "name" FROM "a" WHERE x = $1) UNION (SELECT "name" FROM "b" WHERE y = $2) ` +
			`ORDER BY "name" LIMIT 10 OFFSET 5`},
		{"mysql", "(SELECT `name` FROM `a` WHERE x = ?) UNION (SELECT `name` FROM `b` WHERE y = ?) " +
			"ORDER BY `name` LIMIT 10 OFFSET 5"},
		{"sqlite", "SELECT `name` FROM `a` WHERE x = ? UNION SELECT `name` FROM `b` WHERE y = ? " +
			"ORDER BY `name` LIMIT 10 OFFSET 5"},
	} {
		qb := setOpsBuilder(tc.dialect)
		q := qb.Select("name").From("a").Where("x = ?", 1).
			Union(qb.Select("name").From("b").Where("y = ?", 2)).
			OrderBy("name").Limit(10).Offset(5)
		got, args := q.ToSQL()
		if got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.dialect, got, tc.want)
		}
		if !reflect.DeepEqual(args, []any{1, 2}) {
			t.Errorf("%s: args %v, want [1 2]", tc.dialect, args)
		}
	}
}

func TestSetOps_BranchAndCompoundLimitsTogether(t *testing.T) {
	for _, tc := range []struct {
		dialect, want string
	}{
		{"postgres", `(SELECT "id" FROM "a" ORDER BY "id" DESC LIMIT 1) UNION (SELECT "id" FROM "b") ORDER BY "id" LIMIT 5`},
		{"sqlite", "SELECT * FROM (SELECT `id` FROM `a` ORDER BY `id` DESC LIMIT 1) UNION SELECT `id` FROM `b` " +
			"ORDER BY `id` LIMIT 5"},
	} {
		qb := setOpsBuilder(tc.dialect)
		got, _ := qb.Select("id").From("a").OrderBy("id DESC").Limit(1).
			Union(qb.Select("id").From("b")).
			OrderBy("id").Limit(5).ToSQL()
		if got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.dialect, got, tc.want)
		}
	}
}

func TestSetOps_CompoundOffsetWithoutLimit(t *testing.T) {
	qb := setOpsBuilder("mysql")
	got, _ := qb.Select("id").From("a").Union(qb.Select("id").From("b")).Offset(3).ToSQL()
	want := "(SELECT `id` FROM `a`) UNION (SELECT `id` FROM `b`) LIMIT 9223372036854775807 OFFSET 3"
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

func TestSetOps_CompoundOrderByExprArgsComeLast(t *testing.T) {
	qb := setOpsBuilder("postgres")
	got, args := qb.Select("id").From("a").Where("x = ?", 1).
		Union(qb.Select("id").From("b").Where("y = ?", 2)).
		OrderByExpr("CASE WHEN id = ? THEN 0 ELSE 1 END", 7).ToSQL()
	want := `(SELECT "id" FROM "a" WHERE x = $1) UNION (SELECT "id" FROM "b" WHERE y = $2) ` +
		`ORDER BY CASE WHEN id = $3 THEN 0 ELSE 1 END`
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
	if !reflect.DeepEqual(args, []any{1, 2, 7}) {
		t.Errorf("args %v, want [1 2 7]", args)
	}
}

func TestSetOps_SetOperationAfterCompoundOrderIsAnError(t *testing.T) {
	qb := setOpsBuilder("postgres")
	for name, q := range map[string]*SelectQuery{
		"Union after OrderBy": qb.Select("id").From("a").Union(qb.Select("id").From("b")).
			OrderBy("id").Union(qb.Select("id").From("c")),
		"Intersect after Limit": qb.Select("id").From("a").Union(qb.Select("id").From("b")).
			Limit(1).Intersect(qb.Select("id").From("c")),
		"Except after Offset": qb.Select("id").From("a").UnionAll(qb.Select("id").From("b")).
			Offset(1).Except(qb.Select("id").From("c")),
	} {
		err := q.err()
		if err == nil || !strings.Contains(err.Error(), "combined result") {
			t.Errorf("%s: err = %v, want the combined-result error", name, err)
		}
		if got, _ := q.ToSQL(); got != "" {
			t.Errorf("%s: ToSQL = %q, want empty", name, got)
		}
	}
}

func TestSetOps_BranchOrderWithoutLimitIsAnError(t *testing.T) {
	qb := setOpsBuilder("postgres")
	for name, q := range map[string]*SelectQuery{
		"first branch": qb.Select("id").From("a").OrderBy("id").Union(qb.Select("id").From("b")),
		"later branch": qb.Select("id").From("a").Union(qb.Select("id").From("b").OrderBy("id")),
		"OrderByExpr":  qb.Select("id").From("a").OrderByExpr("id % 2").Intersect(qb.Select("id").From("b")),
		"nested compound branch": qb.Select("id").From("a").
			Union(qb.Select("id").From("b").Union(qb.Select("id").From("c")).OrderBy("id")),
	} {
		err := q.err()
		if err == nil || !strings.Contains(err.Error(), "has no effect") {
			t.Errorf("%s: err = %v, want the branch ORDER BY error", name, err)
		}
	}
	// With LIMIT or OFFSET the branch ORDER BY chooses the rows: allowed.
	for name, q := range map[string]*SelectQuery{
		"first branch limit": qb.Select("id").From("a").OrderBy("id").Limit(1).Union(qb.Select("id").From("b")),
		"branch offset":      qb.Select("id").From("a").Union(qb.Select("id").From("b").OrderBy("id").Offset(2)),
		"nested compound limit": qb.Select("id").From("a").
			Union(qb.Select("id").From("b").Union(qb.Select("id").From("c")).OrderBy("id").Limit(2)),
	} {
		if err := q.err(); err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
	}
	// A query that is not a branch keeps its ORDER BY as before.
	if err := qb.Select("id").From("a").OrderBy("id").err(); err != nil {
		t.Errorf("plain query: unexpected error %v", err)
	}
}

func TestSetOps_IntersectAfterUnionGroupsLeftToRight(t *testing.T) {
	a := func(qb *QueryBuilder) *SelectQuery { return qb.Select("id").From("a") }
	b := func(qb *QueryBuilder) *SelectQuery { return qb.Select("id").From("b") }
	c := func(qb *QueryBuilder) *SelectQuery { return qb.Select("id").From("c") }
	d := func(qb *QueryBuilder) *SelectQuery { return qb.Select("id").From("d") }
	for _, tc := range []struct {
		name, dialect, want string
		build               func(qb *QueryBuilder) *SelectQuery
	}{
		{"union then intersect", "postgres",
			`((SELECT "id" FROM "a") UNION (SELECT "id" FROM "b")) INTERSECT (SELECT "id" FROM "c")`,
			func(qb *QueryBuilder) *SelectQuery { return a(qb).Union(b(qb)).Intersect(c(qb)) }},
		{"union then intersect", "mysql",
			"((SELECT `id` FROM `a`) UNION (SELECT `id` FROM `b`)) INTERSECT (SELECT `id` FROM `c`)",
			func(qb *QueryBuilder) *SelectQuery { return a(qb).Union(b(qb)).Intersect(c(qb)) }},
		{"union then intersect", "sqlite",
			"SELECT `id` FROM `a` UNION SELECT `id` FROM `b` INTERSECT SELECT `id` FROM `c`",
			func(qb *QueryBuilder) *SelectQuery { return a(qb).Union(b(qb)).Intersect(c(qb)) }},
		{"except then intersect, ordered", "postgres",
			`((SELECT "id" FROM "a") EXCEPT (SELECT "id" FROM "b")) INTERSECT (SELECT "id" FROM "c") ORDER BY "id"`,
			func(qb *QueryBuilder) *SelectQuery { return a(qb).Except(b(qb)).Intersect(c(qb)).OrderBy("id") }},
		{"intersect, union, intersect", "postgres",
			`((SELECT "id" FROM "a") INTERSECT (SELECT "id" FROM "b") UNION (SELECT "id" FROM "c")) ` +
				`INTERSECT (SELECT "id" FROM "d")`,
			func(qb *QueryBuilder) *SelectQuery { return a(qb).Intersect(b(qb)).Union(c(qb)).Intersect(d(qb)) }},
		{"union then except: same precedence", "postgres",
			`(SELECT "id" FROM "a") UNION (SELECT "id" FROM "b") EXCEPT (SELECT "id" FROM "c")`,
			func(qb *QueryBuilder) *SelectQuery { return a(qb).Union(b(qb)).Except(c(qb)) }},
		{"right-nested on purpose", "postgres",
			`(SELECT "id" FROM "a") UNION ((SELECT "id" FROM "b") INTERSECT (SELECT "id" FROM "c"))`,
			func(qb *QueryBuilder) *SelectQuery { return a(qb).Union(b(qb).Intersect(c(qb))) }},
	} {
		got, _ := tc.build(setOpsBuilder(tc.dialect)).ToSQL()
		if got != tc.want {
			t.Errorf("%s (%s):\n got %s\nwant %s", tc.name, tc.dialect, got, tc.want)
		}
	}
}

func TestSetOps_CountAndExistsDropTheCompoundTailOnly(t *testing.T) {
	qb := setOpsBuilder("postgres")
	q := qb.Select("id").From("a").OrderBy("id DESC").Limit(3).
		Union(qb.Select("id").From("b")).
		OrderBy("id").Limit(1).Offset(1)
	count, _ := q.countStatement()
	wantCount := `SELECT COUNT(*) FROM ((SELECT "id" FROM "a" ORDER BY "id" DESC LIMIT 3) UNION (SELECT "id" FROM "b")) AS relica_count`
	if count != wantCount {
		t.Errorf("count:\n got %s\nwant %s", count, wantCount)
	}
	exists, _ := q.existsStatement()
	wantExists := `SELECT EXISTS((SELECT "id" FROM "a" ORDER BY "id" DESC LIMIT 3) UNION (SELECT "id" FROM "b"))`
	if exists != wantExists {
		t.Errorf("exists:\n got %s\nwant %s", exists, wantExists)
	}
}

func TestSetOps_CompoundWithTailNestsAsOneQuery(t *testing.T) {
	for _, tc := range []struct {
		dialect, fromSelect, branch string
	}{
		{"postgres",
			`SELECT * FROM ((SELECT "id" FROM "a") UNION (SELECT "id" FROM "b") ORDER BY "id" LIMIT 2) AS "u"`,
			`(SELECT "id" FROM "x") UNION ((SELECT "id" FROM "a") UNION (SELECT "id" FROM "b") ORDER BY "id" LIMIT 2)`},
		{"sqlite",
			"SELECT * FROM (SELECT `id` FROM `a` UNION SELECT `id` FROM `b` ORDER BY `id` LIMIT 2) AS `u`",
			"SELECT `id` FROM `x` UNION SELECT * FROM (SELECT `id` FROM `a` UNION SELECT `id` FROM `b` ORDER BY `id` LIMIT 2)"},
	} {
		qb := setOpsBuilder(tc.dialect)
		top := func() *SelectQuery {
			return qb.Select("id").From("a").Union(qb.Select("id").From("b")).OrderBy("id").Limit(2)
		}
		if got, _ := qb.Select().FromSelect(top(), "u").ToSQL(); got != tc.fromSelect {
			t.Errorf("%s FromSelect:\n got %s\nwant %s", tc.dialect, got, tc.fromSelect)
		}
		if got, _ := qb.Select("id").From("x").Union(top()).ToSQL(); got != tc.branch {
			t.Errorf("%s branch:\n got %s\nwant %s", tc.dialect, got, tc.branch)
		}
	}
}
