package core

import (
	"strings"
	"testing"
)

// TestOrderBy_ColumnAndDirectionOnly verifies F1 (Fable review #2a): OrderBy
// took the first word and the direction and dropped the rest silently. It now
// takes "col" or "col ASC|DESC" (any case), uses an argument with "(" as is,
// and rejects everything else before anything is sent.
func TestOrderBy_ColumnAndDirectionOnly(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"age"}, `ORDER BY "age"`},
		{[]string{"age desc", "t.name Asc"}, `ORDER BY "age" DESC, "t"."name" ASC`},
		{[]string{"  age   DESC "}, `ORDER BY "age" DESC`},
		{[]string{"LOWER(name) DESC", "COALESCE(a, b)"}, `ORDER BY LOWER(name) DESC, COALESCE(a, b)`},
	} {
		got, _ := qb.Select("id").From("t").OrderBy(tc.args...).ToSQL()
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("OrderBy(%q):\n got %s\nwant suffix %s", tc.args, got, tc.want)
		}
	}
	for _, bad := range []string{
		"salary + 1 DESC", "created_at DESC NULLS LAST", "age DESC, name", "name COLLATE NOCASE", "age DOWN", "", "   ",
	} {
		q := qb.Select("id").From("t").OrderBy("id", bad).Build()
		if q.prepErr == nil || !strings.Contains(q.prepErr.Error(), "OrderByExpr") {
			t.Errorf("OrderBy(%q): prepErr = %v, sql %q", bad, q.prepErr, q.sql)
		}
	}
}
