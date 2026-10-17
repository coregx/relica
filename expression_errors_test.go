package relica_test

import (
	"context"
	"strings"
	"testing"

	"github.com/coregx/relica"
)

// An expression that carries a construction error (Like(...).EscapeChars with
// an odd number of arguments) used to render as an empty SQL fragment. An empty
// condition is skipped, so the query ran WITHOUT the filter: a DELETE removed
// every row. The error must reach the caller and nothing may be executed.

func invalidLike() relica.Expression {
	return relica.Like("name", "x").EscapeChars("!")
}

const escapeCharsErr = "EscapeChars requires an even number"

func requireBuildError(t *testing.T, err error, st *captureState) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), escapeCharsErr) {
		t.Fatalf("err = %v, want the EscapeChars construction error", err)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.queries) != 0 {
		t.Fatalf("statement executed despite the construction error: %+v", st.queries)
	}
}

func TestInvalidExpression_DeleteIsNotExecuted(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", nil)
	_, err := db.Delete("account").Where(invalidLike()).Execute()
	requireBuildError(t, err, st)
}

func TestInvalidExpression_AllConditionEntryPoints(t *testing.T) {
	cases := map[string]func(db *relica.DB) error{
		"Select.Where": func(db *relica.DB) error {
			return db.Select().From("t").Where(invalidLike()).All(&[]struct{}{})
		},
		"Select.AndWhere": func(db *relica.DB) error {
			return db.Select().From("t").Where(relica.Eq("a", 1)).AndWhere(invalidLike()).All(&[]struct{}{})
		},
		"Select.OrWhere": func(db *relica.DB) error {
			return db.Select().From("t").Where(relica.Eq("a", 1)).OrWhere(invalidLike()).All(&[]struct{}{})
		},
		"Select.Having": func(db *relica.DB) error {
			return db.Select("a").From("t").GroupBy("a").Having(invalidLike()).All(&[]struct{}{})
		},
		"Select.InnerJoin": func(db *relica.DB) error {
			return db.Select().From("t").InnerJoin("u", invalidLike()).All(&[]struct{}{})
		},
		"Select.SelectSub": func(db *relica.DB) error {
			return db.Select("a").SelectSub(invalidLike(), "x").From("t").All(&[]struct{}{})
		},
		"Update.Where": func(db *relica.DB) error {
			_, err := db.Update("t").Set(map[string]any{"a": 1}).Where(invalidLike()).Execute()
			return err
		},
		"Update.AndWhere": func(db *relica.DB) error {
			_, err := db.Update("t").Set(map[string]any{"a": 1}).Where(relica.Eq("a", 1)).AndWhere(invalidLike()).Execute()
			return err
		},
		"Update.OrWhere": func(db *relica.DB) error {
			_, err := db.Update("t").Set(map[string]any{"a": 1}).Where(relica.Eq("a", 1)).OrWhere(invalidLike()).Execute()
			return err
		},
		"Delete.AndWhere": func(db *relica.DB) error {
			_, err := db.Delete("t").Where(relica.Eq("a", 1)).AndWhere(invalidLike()).Execute()
			return err
		},
		"Delete.OrWhere": func(db *relica.DB) error {
			_, err := db.Delete("t").Where(relica.Eq("a", 1)).OrWhere(invalidLike()).Execute()
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			db, st := newCaptureDB(t, "postgres", nil)
			requireBuildError(t, run(db), st)
		})
	}
}

func TestInvalidExpression_NestedAnywhere(t *testing.T) {
	cases := map[string]relica.Expression{
		"And":         relica.And(relica.Eq("a", 1), invalidLike()),
		"Or":          relica.Or(relica.Eq("a", 1), invalidLike()),
		"Not":         relica.Not(invalidLike()),
		"deep":        relica.And(relica.Or(relica.Not(invalidLike()), relica.Eq("b", 2)), relica.Eq("c", 3)),
		"HashExp":     relica.HashExp{"a": 1, "name": invalidLike()},
		"Eq value":    relica.Eq("a", relica.Coalesce(invalidLike(), 0)),
		"Case cond":   relica.CaseWhen().When(invalidLike(), 1).Else(0),
		"Case result": relica.Case("s").When(1, relica.Coalesce(invalidLike())),
	}
	for name, cond := range cases {
		t.Run(name, func(t *testing.T) {
			db, st := newCaptureDB(t, "postgres", nil)
			_, err := db.Delete("t").Where(cond).Execute()
			requireBuildError(t, err, st)
		})
	}
}

func TestInvalidExpression_InUpdateSetMap(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", nil)
	_, err := db.Update("t").Set(map[string]any{
		"flag": relica.CaseWhen().When(invalidLike(), 1).Else(0),
	}).Where(relica.Eq("id", 1)).Execute()
	requireBuildError(t, err, st)
}

func TestInvalidExpression_InTransaction(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", nil)
	err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
		_, err := tx.Delete("t").Where(invalidLike()).Execute()
		return err
	})
	requireBuildError(t, err, st)
}

func TestInvalidExpression_SubqueryBuildErrorPropagates(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", nil)
	sub := db.Select("id").From("vip").Where(invalidLike())
	_, err := db.Delete("t").Where(relica.In("id", sub.AsExpression())).Execute()
	requireBuildError(t, err, st)
}

func TestLikeExp_ValidStillWorksAfterErrorChecks(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", nil)
	if _, err := db.Delete("t").Where(relica.Like("name", "a%b")).Execute(); err != nil {
		t.Fatal(err)
	}
	q := st.last(t)
	if q.SQL != `DELETE FROM "t" WHERE "name" LIKE $1` {
		t.Errorf("SQL = %s", q.SQL)
	}
}
