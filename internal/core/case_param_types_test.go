package core

import (
	"math"
	"strings"
	"testing"
)

// PostgreSQL resolves a CASE whose results are all untyped parameters to text,
// so `SET state = CASE ... THEN $2 ELSE $3 END` fails on an integer column
// ("column is of type integer but expression is of type text"), while the same
// CASE with literals (THEN 9 ELSE 0) works. Results that are all Go numbers or
// all bools are cast to the type a literal would have. Found when the
// PostgreSQL integration suite first ran in CI.
func TestCase_PostgresTypesAllParamResults(t *testing.T) {
	pg := mockDB("postgres").dialect
	tests := []struct {
		name string
		exp  *CaseExp
		want string
	}{
		{"ints", CaseWhen().When(GreaterThan("price", 60), 9).Else(0),
			`CASE WHEN "price" > ? THEN CAST(? AS INTEGER) ELSE CAST(? AS INTEGER) END`},
		{"int beyond int32", CaseWhen().When("a", int64(math.MaxInt32)+1).Else(int8(1)),
			`CASE WHEN a THEN CAST(? AS BIGINT) ELSE CAST(? AS INTEGER) END`},
		{"uint64 beyond int64", CaseWhen().When("a", uint64(math.MaxUint64)),
			`CASE WHEN a THEN CAST(? AS NUMERIC) END`},
		{"floats and ints", CaseWhen().When("a", 1.5).Else(2),
			`CASE WHEN a THEN CAST(? AS NUMERIC) ELSE CAST(? AS INTEGER) END`},
		{"bools", CaseWhen().When("a", true).Else(false),
			`CASE WHEN a THEN CAST(? AS BOOLEAN) ELSE CAST(? AS BOOLEAN) END`},
		{"simple CASE: WHEN values take the column type, not cast", Case("state").When(1, 10).When(2, 20),
			`CASE "state" WHEN ? THEN CAST(? AS INTEGER) WHEN ? THEN CAST(? AS INTEGER) END`},
		{"NULL takes the type of the cast results", CaseWhen().When("a", nil).When("b", 2),
			`CASE WHEN a THEN ? WHEN b THEN CAST(? AS INTEGER) END`},
		{"named integer type", CaseWhen().When("a", testStatus(3)),
			`CASE WHEN a THEN CAST(? AS INTEGER) END`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := tc.exp.Build(pg)
			if got != tc.want {
				t.Errorf("\ngot  %s\nwant %s", got, tc.want)
			}
		})
	}
}

type testStatus int

// No cast when PostgreSQL can type the CASE by itself, or when a cast could
// change a query that works today.
func TestCase_PostgresNoCast(t *testing.T) {
	pg := mockDB("postgres").dialect
	tests := []struct {
		name string
		exp  *CaseExp
	}{
		{"strings stay untyped, like literals", CaseWhen().When("a", "x").Else("y")},
		{"a result expression types the CASE", CaseWhen().When("a", 1).Else(Coalesce("nickname", "name"))},
		{"numbers mixed with strings", CaseWhen().When("a", 1).Else("x")},
		{"numbers mixed with bools", CaseWhen().When("a", 1).Else(true)},
		{"only NULL results", CaseWhen().When("a", nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := tc.exp.Build(pg)
			if strings.Contains(got, "CAST(") {
				t.Errorf("unexpected cast: %s", got)
			}
		})
	}
}

// MySQL and SQLite type parameters from the bound values: no casts.
func TestCase_OtherDialectsNoCast(t *testing.T) {
	for _, d := range []string{"mysql", "sqlite"} {
		got, args := CaseWhen().When(GreaterThan("price", 60), 9).Else(0).Build(mockDB(d).dialect)
		if strings.Contains(got, "CAST(") {
			t.Errorf("%s: unexpected cast: %s", d, got)
		}
		if len(args) != 3 {
			t.Errorf("%s: args %v", d, args)
		}
	}
}
