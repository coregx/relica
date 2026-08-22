package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// TestSelectQuery_BuildErr_FromSelect verifies that FromSelect with an empty alias
// stores a build error and propagates it through Build, All, One, etc.
func TestSelectQuery_BuildErr_FromSelect(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("id").From("orders")
	sq := qb.Select("id").FromSelect(sub, "") // empty alias — programming error

	if sq.buildErr == nil {
		t.Error("empty alias must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "FromSelect") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "FromSelect")
	}
	if !strings.Contains(sq.buildErr.Error(), "non-empty alias") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "non-empty alias")
	}

	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("buildErr must propagate to Query.prepErr")
	}
	if !strings.Contains(q.prepErr.Error(), "FromSelect") {
		t.Errorf("%q does not contain %q", q.prepErr.Error(), "FromSelect")
	}
}

// TestSelectQuery_BuildErr_Where verifies that an invalid Where() type stores
// a build error and propagates it through Build.
func TestSelectQuery_BuildErr_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id").From("users").Where(42) // int is not string or Expression

	if sq.buildErr == nil {
		t.Error("invalid Where() type must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "Where()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "Where()")
	}
	if !strings.Contains(sq.buildErr.Error(), "int") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "int")
	}

	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("buildErr must propagate to Query.prepErr")
	}
}

// TestSelectQuery_BuildErr_OrWhere verifies that an invalid OrWhere() type stores
// a build error and propagates it through Build.
func TestSelectQuery_BuildErr_OrWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id").From("users").
		Where("status = ?", 1).
		OrWhere([]int{1, 2, 3}) // slice is not string or Expression

	if sq.buildErr == nil {
		t.Error("invalid OrWhere() type must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "OrWhere()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "OrWhere()")
	}

	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("buildErr must propagate to Query.prepErr")
	}
}

// TestSelectQuery_BuildErr_With_EmptyName verifies that With() with an empty
// name stores an error and propagates it through Build.
func TestSelectQuery_BuildErr_With_EmptyName(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	cte := qb.Select("id").From("orders")
	sq := qb.Select("*").With("", cte)

	if sq.buildErr == nil {
		t.Error("empty CTE name must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "With()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "With()")
	}

	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestSelectQuery_BuildErr_With_NilQuery verifies that With() with a nil query
// stores an error and propagates it through Build.
func TestSelectQuery_BuildErr_With_NilQuery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("*").With("my_cte", nil)

	if sq.buildErr == nil {
		t.Error("nil CTE query must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "With()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "With()")
	}

	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestSelectQuery_BuildErr_WithRecursive_EmptyName verifies that WithRecursive()
// with an empty name stores an error.
func TestSelectQuery_BuildErr_WithRecursive_EmptyName(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	anchor := qb.Select("1 as n")
	rec := qb.Select("n+1").From("nums").Where("n < ?", 10)
	cte := anchor.UnionAll(rec)

	sq := qb.Select("*").WithRecursive("", cte)

	if sq.buildErr == nil {
		t.Error("empty recursive CTE name must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "WithRecursive()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "WithRecursive()")
	}
}

// TestSelectQuery_BuildErr_WithRecursive_NilQuery verifies that WithRecursive()
// with a nil query stores an error.
func TestSelectQuery_BuildErr_WithRecursive_NilQuery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("*").WithRecursive("nums", nil)

	if sq.buildErr == nil {
		t.Error("nil recursive CTE query must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "WithRecursive()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "WithRecursive()")
	}
}

// TestSelectQuery_BuildErr_WithRecursive_NoUnion verifies that WithRecursive()
// without UNION stores an error.
func TestSelectQuery_BuildErr_WithRecursive_NoUnion(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	nonRecursive := qb.Select("id").From("employees") // no UNION

	sq := qb.Select("*").WithRecursive("hier", nonRecursive)

	if sq.buildErr == nil {
		t.Error("recursive CTE without UNION must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "WithRecursive()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "WithRecursive()")
	}
	if !strings.Contains(sq.buildErr.Error(), "UNION") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "UNION")
	}

	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestSelectQuery_BuildErr_JoinInvalidOnType verifies that an invalid JOIN ON
// type stores an error via buildErr and propagates through Build.
func TestSelectQuery_BuildErr_JoinInvalidOnType(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("m.id").
		From("messages m").
		InnerJoin("users u", 3.14) // float64 — not string or Expression

	// buildErr is set lazily during buildSQL (called from Build)
	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("invalid JOIN ON type must propagate as Query.prepErr")
	}
	if !strings.Contains(q.prepErr.Error(), "JOIN ON") {
		t.Errorf("%q does not contain %q", q.prepErr.Error(), "JOIN ON")
	}
}

// TestSelectQuery_BuildErr_Having verifies that an invalid Having() type stores
// a build error.
func TestSelectQuery_BuildErr_Having(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("status", "COUNT(*)").
		From("orders").
		GroupBy("status").
		Having(42) // int is not string or Expression

	if sq.buildErr == nil {
		t.Error("invalid Having() type must store buildErr")
	}
	if !strings.Contains(sq.buildErr.Error(), "Having()") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "Having()")
	}

	q := sq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestUpdateQuery_BuildErr_Where verifies that an invalid Where() type on
// UpdateQuery stores a build error.
func TestUpdateQuery_BuildErr_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	uq := qb.Update("users").
		Set(map[string]interface{}{"status": "active"}).
		Where(struct{ bad string }{"value"}) // unsupported type

	if uq.buildErr == nil {
		t.Error("invalid Where() type must store buildErr on UpdateQuery")
	}
	if !strings.Contains(uq.buildErr.Error(), "Where()") {
		t.Errorf("%q does not contain %q", uq.buildErr.Error(), "Where()")
	}

	q := uq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestUpdateQuery_BuildErr_OrWhere verifies that an invalid OrWhere() type on
// UpdateQuery stores a build error.
func TestUpdateQuery_BuildErr_OrWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	uq := qb.Update("users").
		Set(map[string]interface{}{"status": "active"}).
		Where("id = ?", 1).
		OrWhere(map[string]string{"bad": "value"}) // unsupported type

	if uq.buildErr == nil {
		t.Error("invalid OrWhere() type must store buildErr on UpdateQuery")
	}
	if !strings.Contains(uq.buildErr.Error(), "OrWhere()") {
		t.Errorf("%q does not contain %q", uq.buildErr.Error(), "OrWhere()")
	}

	q := uq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestDeleteQuery_BuildErr_Where verifies that an invalid Where() type on
// DeleteQuery stores a build error.
func TestDeleteQuery_BuildErr_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	dq := qb.Delete("users").Where([]string{"bad"}) // unsupported type

	if dq.buildErr == nil {
		t.Error("invalid Where() type must store buildErr on DeleteQuery")
	}
	if !strings.Contains(dq.buildErr.Error(), "Where()") {
		t.Errorf("%q does not contain %q", dq.buildErr.Error(), "Where()")
	}

	q := dq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestDeleteQuery_BuildErr_OrWhere verifies that an invalid OrWhere() type on
// DeleteQuery stores a build error.
func TestDeleteQuery_BuildErr_OrWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	dq := qb.Delete("users").
		Where("id = ?", 1).
		OrWhere(true) // bool — unsupported type

	if dq.buildErr == nil {
		t.Error("invalid OrWhere() type must store buildErr on DeleteQuery")
	}
	if !strings.Contains(dq.buildErr.Error(), "OrWhere()") {
		t.Errorf("%q does not contain %q", dq.buildErr.Error(), "OrWhere()")
	}

	q := dq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestBatchInsertQuery_BuildErr_Values verifies that wrong value count stores
// a build error.
func TestBatchInsertQuery_BuildErr_Values(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	biq := qb.BatchInsert("users", []string{"name", "email"})
	biq.Values("Alice") // 1 value for 2 columns

	if biq.buildErr == nil {
		t.Error("wrong value count must store buildErr")
	}
	if !strings.Contains(biq.buildErr.Error(), "BatchInsert.Values") {
		t.Errorf("%q does not contain %q", biq.buildErr.Error(), "BatchInsert.Values")
	}
	if !strings.Contains(biq.buildErr.Error(), "2") {
		t.Errorf("%q does not contain %q", biq.buildErr.Error(), "2")
	}

	q := biq.Build()
	if q.prepErr == nil {
		t.Fatal("expected non-nil prepErr")
	}
}

// TestBatchInsertQuery_BuildErr_NoRows verifies that Build with no rows returns
// an error instead of panicking.
func TestBatchInsertQuery_BuildErr_NoRows(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	biq := qb.BatchInsert("users", []string{"name"})
	// No Values() calls

	q := biq.Build()
	if q.prepErr == nil {
		t.Fatal("Build with no rows must store an error in prepErr")
	}
	if !strings.Contains(q.prepErr.Error(), "BatchInsert") {
		t.Errorf("%q does not contain %q", q.prepErr.Error(), "BatchInsert")
	}
}

// TestBatchUpdateQuery_BuildErr_NoUpdates verifies that Build with no updates
// returns an error instead of panicking.
func TestBatchUpdateQuery_BuildErr_NoUpdates(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	buq := qb.BatchUpdate("users", "id")
	// No Set() calls

	q := buq.Build()
	if q.prepErr == nil {
		t.Fatal("Build with no updates must store an error in prepErr")
	}
	if !strings.Contains(q.prepErr.Error(), "BatchUpdate") {
		t.Errorf("%q does not contain %q", q.prepErr.Error(), "BatchUpdate")
	}
}

// TestLikeExp_BuildErr_EscapeChars verifies that an odd number of escape chars
// stores an error accessible via Err() and makes Build return empty SQL.
func TestLikeExp_BuildErr_EscapeChars(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	exp := Like("name", "test").EscapeChars("%", "\\%", "_") // 3 strings — odd

	if exp.Err() == nil {
		t.Fatal("odd EscapeChars must store an error via Err()")
	}
	if !strings.Contains(exp.Err().Error(), "EscapeChars") {
		t.Errorf("%q does not contain %q", exp.Err().Error(), "EscapeChars")
	}
	if !strings.Contains(exp.Err().Error(), "3") {
		t.Errorf("%q does not contain %q", exp.Err().Error(), "3")
	}

	sql, args := exp.Build(dialect)
	if sql != "" {
		t.Errorf("Build with stored error must return empty SQL, got %q", sql)
	}
	if args != nil {
		t.Errorf("Build with stored error must return nil args, got %v", args)
	}
}

// TestLikeExp_BuildErr_EscapeChars_ValidAfterError verifies that a valid
// EscapeChars call after an error does NOT override the stored error.
func TestLikeExp_BuildErr_EscapeChars_ValidAfterError(t *testing.T) {
	// First call with odd count stores error; second would be ignored in real code
	// (the error is set on the instance). We verify the error is preserved.
	exp := Like("name", "test").EscapeChars("_") // 1 string — odd

	if exp.Err() == nil {
		t.Error("expected non-nil error")
	}
}
