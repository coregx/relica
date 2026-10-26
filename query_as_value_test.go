package relica_test

import (
	"strings"
	"testing"

	"github.com/coregx/relica"
)

// A *SelectQuery passed where a value goes (In, Eq, …) was bound as a query
// argument and failed only in the driver ("unsupported type relica.SelectQuery,
// a struct"), with ToSQL showing a plain "IN (?)". It is a build error that
// names AsExpression, the way to pass a subquery.
func TestQueryAsValue_IsABuildError(t *testing.T) {
	db, _ := wrapTestDB(t)
	sub := func() *relica.SelectQuery { return db.Select("user_id").From("orders") }
	for name, cond := range map[string]relica.Expression{
		"In":       relica.In("id", sub()),
		"NotIn":    relica.NotIn("id", 1, sub()),
		"Eq":       relica.Eq("id", sub()),
		"Between":  relica.Between("id", sub(), 10),
		"HashExp":  relica.HashExp{"id": sub()},
		"Nested":   relica.And(relica.Eq("a", 1), relica.In("id", sub())),
		"Coalesce": relica.Eq("x", relica.Coalesce("y", sub())),
	} {
		q := db.Select("id").From("users").Where(cond)
		if got, _ := q.ToSQL(); got != "" {
			t.Errorf("%s: ToSQL = %q, want empty (build error)", name, got)
		}
		var ids []int
		err := q.Column(&ids)
		if err == nil || !strings.Contains(err.Error(), "AsExpression") {
			t.Errorf("%s: err = %v, want the AsExpression build error", name, err)
		}
	}
	// A SET value too.
	_, err := db.Update("users").Set(map[string]any{"best": sub()}).Where(relica.Eq("id", 1)).Execute()
	if err == nil || !strings.Contains(err.Error(), "AsExpression") {
		t.Errorf("Set: err = %v, want the AsExpression build error", err)
	}
	// The supported form keeps working.
	got, _ := db.Select("id").From("users").Where(relica.In("id", sub().AsExpression())).ToSQL()
	if !strings.Contains(got, "IN (SELECT") {
		t.Errorf("AsExpression: ToSQL = %q, want a subquery", got)
	}
}
