package core

import (
	"reflect"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// CASE operands that are Expressions must be built into SQL, not formatted
// with fmt (searched WHEN rendered "&{balance > 100}") or bound as a parameter
// (THEN/ELSE expressions reached the driver as Go values). Found while writing
// the v0.19.0 public-type contract tests.

func TestCase_SearchedWhenExpressionCondition(t *testing.T) {
	d := dialects.GetDialect("postgres")
	sql, args := CaseWhen().
		When(GreaterThan("balance", 100), "gold").
		When(And(Eq("vip", true), LessThan("age", 30)), "young-vip").
		Else("basic").
		Build(d)

	wantSQL := `CASE WHEN "balance" > ? THEN ? WHEN ("vip" = ?) AND ("age" < ?) THEN ? ELSE ? END`
	if sql != wantSQL {
		t.Errorf("sql\n got: %s\nwant: %s", sql, wantSQL)
	}
	wantArgs := []any{100, "gold", true, 30, "young-vip", "basic"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args\n got: %#v\nwant: %#v", args, wantArgs)
	}
}

func TestCase_SearchedWhenHashExpCondition(t *testing.T) {
	d := dialects.GetDialect("postgres")
	sql, args := CaseWhen().When(HashExp{"status": 1}, "on").Build(d)
	if want := `CASE WHEN "status" = ? THEN ? END`; sql != want {
		t.Errorf("sql\n got: %s\nwant: %s", sql, want)
	}
	if want := []any{1, "on"}; !reflect.DeepEqual(args, want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
}

func TestCase_SearchedWhenStringIsRawSQL(t *testing.T) {
	d := dialects.GetDialect("postgres")
	sql, args := CaseWhen().When("age < 18", "minor").Else("adult").Build(d)
	if want := `CASE WHEN age < 18 THEN ? ELSE ? END`; sql != want {
		t.Errorf("sql\n got: %s\nwant: %s", sql, want)
	}
	if want := []any{"minor", "adult"}; !reflect.DeepEqual(args, want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
}

func TestCase_ExpressionResultsAndElse(t *testing.T) {
	d := dialects.GetDialect("postgres")
	sql, args := Case("state").
		When(1, Greatest("a", "b")).
		When(2, NewExp("price * ?", 2)).
		Else(Coalesce("fallback", 0)).
		Build(d)

	wantSQL := `CASE "state" WHEN ? THEN GREATEST("a", "b") WHEN ? THEN price * ? ELSE COALESCE("fallback", ?) END`
	if sql != wantSQL {
		t.Errorf("sql\n got: %s\nwant: %s", sql, wantSQL)
	}
	if want := []any{1, 2, 2, 0}; !reflect.DeepEqual(args, want) {
		t.Errorf("args\n got: %#v\nwant: %#v", args, want)
	}
}

func TestCase_SimpleWhenExpressionValue(t *testing.T) {
	d := dialects.GetDialect("postgres")
	sql, args := Case("level").When(NewExp("max_level - ?", 1), "almost").Build(d)
	if want := `CASE "level" WHEN max_level - ? THEN ? END`; sql != want {
		t.Errorf("sql\n got: %s\nwant: %s", sql, want)
	}
	if want := []any{1, "almost"}; !reflect.DeepEqual(args, want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
}

func TestCase_PlainValuesStayParameters(t *testing.T) {
	d := dialects.GetDialect("postgres")
	// A string result is a value, not a column: CASE results are parameterised.
	sql, args := Case("status").When("active", "name").Else("other").Build(d)
	if want := `CASE "status" WHEN ? THEN ? ELSE ? END`; sql != want {
		t.Errorf("sql\n got: %s\nwant: %s", sql, want)
	}
	if want := []any{"active", "name", "other"}; !reflect.DeepEqual(args, want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
}
