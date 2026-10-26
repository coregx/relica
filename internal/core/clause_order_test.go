package core

import (
	"reflect"
	"strings"
	"testing"
)

// ORDER BY and GROUP BY items are rendered in the order the builder methods
// were called, whichever method added them, and their parameters are bound
// in the same order as their placeholders appear in the statement.

func TestOrderBy_CallOrder(t *testing.T) {
	tests := []struct {
		name     string
		build    func(qb *QueryBuilder) *SelectQuery
		want     map[string]string // dialect → ORDER BY clause
		wantArgs []any
	}{
		{
			name: "expression before column",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("id").From("tasks t").
					OrderBySub(CaseWhen().When("t.due_date IS NULL", "z").Else("a")).
					OrderBy("t.created_at DESC")
			},
			want: map[string]string{
				"postgres": ` ORDER BY CASE WHEN t.due_date IS NULL THEN $1 ELSE $2 END, "t"."created_at" DESC`,
				"mysql":    " ORDER BY CASE WHEN t.due_date IS NULL THEN ? ELSE ? END, `t`.`created_at` DESC",
				"sqlite":   " ORDER BY CASE WHEN t.due_date IS NULL THEN ? ELSE ? END, `t`.`created_at` DESC",
			},
			wantArgs: []any{"z", "a"},
		},
		{
			name: "raw expression before column",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("id").From("tasks").
					OrderByExpr("CASE WHEN due_date < CURRENT_DATE THEN 0 ELSE 1 END").
					OrderBy("due_date ASC")
			},
			want: map[string]string{
				"postgres": ` ORDER BY CASE WHEN due_date < CURRENT_DATE THEN 0 ELSE 1 END, "due_date" ASC`,
				"mysql":    " ORDER BY CASE WHEN due_date < CURRENT_DATE THEN 0 ELSE 1 END, `due_date` ASC",
				"sqlite":   " ORDER BY CASE WHEN due_date < CURRENT_DATE THEN 0 ELSE 1 END, `due_date` ASC",
			},
		},
		{
			name: "interleaved kinds after a WHERE parameter",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("id").From("tasks").
					Where(Eq("owner", 7)).
					OrderBySub(CaseWhen().When("done", "z").Else("a")).
					OrderBy("priority DESC").
					OrderByExpr("COALESCE(due, ?)", "x").
					OrderBy("id")
			},
			want: map[string]string{
				"postgres": ` ORDER BY CASE WHEN done THEN $2 ELSE $3 END, "priority" DESC, COALESCE(due, $4), "id"`,
				"mysql":    " ORDER BY CASE WHEN done THEN ? ELSE ? END, `priority` DESC, COALESCE(due, ?), `id`",
				"sqlite":   " ORDER BY CASE WHEN done THEN ? ELSE ? END, `priority` DESC, COALESCE(due, ?), `id`",
			},
			wantArgs: []any{7, "z", "a", "x"},
		},
		{
			name: "raw expression after type-safe expression",
			build: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("id").From("tasks").
					OrderBySub(CaseWhen().When("done", "z").Else("a")).
					OrderByExpr("COALESCE(due, ?)", "x")
			},
			want: map[string]string{
				"postgres": ` ORDER BY CASE WHEN done THEN $1 ELSE $2 END, COALESCE(due, $3)`,
				"mysql":    ` ORDER BY CASE WHEN done THEN ? ELSE ? END, COALESCE(due, ?)`,
				"sqlite":   ` ORDER BY CASE WHEN done THEN ? ELSE ? END, COALESCE(due, ?)`,
			},
			wantArgs: []any{"z", "a", "x"},
		},
	}

	for _, tt := range tests {
		for _, dialect := range []string{"postgres", "mysql", "sqlite"} {
			t.Run(tt.name+"/"+dialect, func(t *testing.T) {
				q := tt.build(&QueryBuilder{db: mockDB(dialect)}).Build()
				if !strings.HasSuffix(q.sql, tt.want[dialect]) {
					t.Errorf("SQL\n got: %s\nwant suffix: %s", q.sql, tt.want[dialect])
				}
				assertArgs(t, q.params, tt.wantArgs)
			})
		}
	}
}

func TestGroupBy_CallOrderAndParamsBeforeHaving(t *testing.T) {
	want := map[string]string{
		"postgres": ` GROUP BY DATE_TRUNC($1, created_at), "status", CASE WHEN total > 100 THEN $2 ELSE $3 END HAVING COUNT(*) > $4`,
		"mysql":    " GROUP BY DATE_TRUNC(?, created_at), `status`, CASE WHEN total > 100 THEN ? ELSE ? END HAVING COUNT(*) > ?",
		"sqlite":   " GROUP BY DATE_TRUNC(?, created_at), `status`, CASE WHEN total > 100 THEN ? ELSE ? END HAVING COUNT(*) > ?",
	}
	// GROUP BY precedes HAVING in the statement, so its parameters must be
	// bound first even though Having is called last.
	wantArgs := []any{"day", "high", "low", 5}

	for _, dialect := range []string{"postgres", "mysql", "sqlite"} {
		t.Run(dialect, func(t *testing.T) {
			q := (&QueryBuilder{db: mockDB(dialect)}).
				Select("COUNT(*)").From("orders").
				GroupByExpr("DATE_TRUNC(?, created_at)", "day").
				GroupBy("status").
				GroupBySub(CaseWhen().When("total > 100", "high").Else("low")).
				Having("COUNT(*) > ?", 5).
				Build()
			if !strings.HasSuffix(q.sql, want[dialect]) {
				t.Errorf("SQL\n got: %s\nwant suffix: %s", q.sql, want[dialect])
			}
			assertArgs(t, q.params, wantArgs)
		})
	}
}

// TestCount_GroupedQueryIsDerivedTable verifies that Count counts the groups
// of a grouped query, GROUP BY expressions included, without its ORDER BY
// (FIX-009: it used to return the first group's count).
func TestCount_GroupedQueryIsDerivedTable(t *testing.T) {
	sq := (&QueryBuilder{db: mockDB("postgres")}).
		Select("status").From("orders").
		GroupByExpr("DATE(created_at) + ?", 1).
		GroupBy("status").
		OrderBy("status")

	got, args := sq.countStatement()
	want := `SELECT COUNT(*) FROM (SELECT "status" FROM "orders" GROUP BY DATE(created_at) + $1, "status") AS relica_count`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	assertArgs(t, args, []any{1})
}

func assertArgs(t *testing.T, got, want []any) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %#v, want %#v", got, want)
	}
}
