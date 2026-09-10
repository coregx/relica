package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// ─── Unit tests for replacePlaceholders ──────────────────────────────────────

// TestReplacePlaceholders_JSONB verifies that the JSONB key-existence operator ?
// is not replaced, while a normal positional ? IS replaced.
func TestReplacePlaceholders_JSONB(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	// "data ? 'key'" — JSONB operator, should stay as "data ? 'key'"
	// "id = ?"       — positional param, should become "id = $1"
	input := "data ? 'key' AND id = ?"
	want := "data ? 'key' AND id = $1"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("JSONB operator: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_StringLiteral verifies that ? inside a single-quoted
// string literal is not replaced.
func TestReplacePlaceholders_StringLiteral(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	input := "name = 'why?' AND id = ?"
	want := "name = 'why?' AND id = $1"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("string literal: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_EscapedQuote verifies that ” inside a string literal
// (SQL escaped apostrophe) is handled correctly and does not confuse the parser.
func TestReplacePlaceholders_EscapedQuote(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	// "name = 'it''s' AND id = ?"
	// The '' is an escaped ' inside the string; the ? after it is a real param.
	input := "name = 'it''s' AND id = ?"
	want := "name = 'it''s' AND id = $1"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("escaped quote: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_EscapedQuoteWithQuestionMark verifies that a ?
// inside a string with escaped quotes (e.g. 'it”s ok?') is NOT replaced.
func TestReplacePlaceholders_EscapedQuoteWithQuestionMark(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	// 'it''s ok?' — the ? is inside the string literal.
	input := "name = 'it''s ok?' AND id = ?"
	want := "name = 'it''s ok?' AND id = $1"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("escaped quote with ?: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_NoParams verifies that a SQL fragment with no ?
// is returned unchanged.
func TestReplacePlaceholders_NoParams(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	input := "status = 'active' AND deleted_at IS NULL"
	got := replacePlaceholders(input, 1, pg)
	if got != input {
		t.Errorf("no params: got %q, want %q", got, input)
	}
}

// TestReplacePlaceholders_Multiple verifies sequential numbering of multiple params.
func TestReplacePlaceholders_Multiple(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	input := "a = ? AND b = ? AND c = ?"
	want := "a = $1 AND b = $2 AND c = $3"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("multiple: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_StartIndexOffset verifies that startIndex is respected.
func TestReplacePlaceholders_StartIndexOffset(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	// WHERE clause starts at $3 because there are 2 SET params before it.
	input := "id = ? AND status = ?"
	want := "id = $3 AND status = $4"
	got := replacePlaceholders(input, 3, pg)
	if got != want {
		t.Errorf("start index offset: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_DoubleQuestion verifies ?? emits single ? (client-side escape).
func TestReplacePlaceholders_DoubleQuestion(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	input := "data ?? 'key' AND id = ?"
	want := "data ? 'key' AND id = $1"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("double question: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_JSONBArrayOps verifies ?| and ?& operators preserved.
func TestReplacePlaceholders_JSONBArrayOps(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	input := "data ?| array['a','b'] AND id = ?"
	want := "data ?| array['a','b'] AND id = $1"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("?| operator: got %q, want %q", got, want)
	}

	input2 := "data ?& array['a'] AND id = ?"
	want2 := "data ?& array['a'] AND id = $1"
	got2 := replacePlaceholders(input2, 1, pg)
	if got2 != want2 {
		t.Errorf("?& operator: got %q, want %q", got2, want2)
	}
}

// TestReplacePlaceholders_SQLComment verifies ? inside comments not replaced.
func TestReplacePlaceholders_SQLComment(t *testing.T) {
	pg := dialects.GetDialect("postgres")

	input := "id = ? -- why? this is a comment"
	want := "id = $1 -- why? this is a comment"
	got := replacePlaceholders(input, 1, pg)
	if got != want {
		t.Errorf("comment: got %q, want %q", got, want)
	}
}

// TestReplacePlaceholders_MySQL_Unchanged verifies that MySQL dialect (which
// uses ? natively) leaves the SQL untouched.
func TestReplacePlaceholders_MySQL_Unchanged(t *testing.T) {
	mysql := dialects.GetDialect("mysql")

	input := "id = ? AND status = ?"
	got := replacePlaceholders(input, 1, mysql)
	if got != input {
		t.Errorf("MySQL unchanged: got %q, want %q", got, input)
	}
}

// TestReplacePlaceholders_SQLite_Unchanged verifies that SQLite dialect (which
// uses ? natively) leaves the SQL untouched.
func TestReplacePlaceholders_SQLite_Unchanged(t *testing.T) {
	sqlite := dialects.GetDialect("sqlite")

	input := "id = ? AND status = ?"
	got := replacePlaceholders(input, 1, sqlite)
	if got != input {
		t.Errorf("SQLite unchanged: got %q, want %q", got, input)
	}
}

// ─── Integration tests via query builders ────────────────────────────────────

// TestSelectQuery_JSONB_Where verifies that a SELECT with a JSONB operator in
// WHERE builds correct SQL for PostgreSQL without corrupting the operator.
func TestSelectQuery_JSONB_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "data").
		From("docs").
		Where("data ? 'key'").
		Where("id = ?", 42).
		Build()

	if q == nil {
		t.Fatal("expected non-nil Query")
	}

	// The JSONB ? must remain as ? (not $1), the param ? must become $1.
	wantSQL := `SELECT "id", "data" FROM "docs" WHERE data ? 'key' AND id = $1`
	if q.sql != wantSQL {
		t.Errorf("got  %q\nwant %q", q.sql, wantSQL)
	}
	if len(q.params) != 1 || q.params[0] != 42 {
		t.Errorf("params: got %v, want [42]", q.params)
	}
}

// TestSelectQuery_StringLiteral_Where verifies that ? inside a string literal
// in a WHERE clause is not mistakenly renumbered.
func TestSelectQuery_StringLiteral_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id").
		From("users").
		Where("name = 'why?' AND id = ?", 1).
		Build()

	if q == nil {
		t.Fatal("expected non-nil Query")
	}

	wantSQL := `SELECT "id" FROM "users" WHERE name = 'why?' AND id = $1`
	if q.sql != wantSQL {
		t.Errorf("got  %q\nwant %q", q.sql, wantSQL)
	}
}

// TestUpdateQuery_JSONB_Where verifies UPDATE with JSONB operator in WHERE.
func TestUpdateQuery_JSONB_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Update("docs").
		Set(map[string]interface{}{"status": "processed"}).
		Where("data ? 'key'").
		Where("id = ?", 7).
		Build()

	if q == nil {
		t.Fatal("expected non-nil Query")
	}

	// SET uses $1, WHERE JSONB ? stays as ?, id param becomes $2.
	if !strings.Contains(q.sql, `"status" = $1`) {
		t.Errorf("SET clause: %q does not contain %q", q.sql, `"status" = $1`)
	}
	if !strings.Contains(q.sql, "data ? 'key' AND id = $2") {
		t.Errorf("WHERE clause: %q does not contain %q", q.sql, "data ? 'key' AND id = $2")
	}
	// Params: [processed, 7]
	want := []interface{}{"processed", 7}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i, w := range want {
			if q.params[i] != w {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], w)
			}
		}
	}
}

// TestDeleteQuery_JSONB_Where verifies DELETE with JSONB operator in WHERE.
func TestDeleteQuery_JSONB_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Delete("docs").
		Where("data ? 'stale'").
		Where("id = ?", 99).
		Build()

	if q == nil {
		t.Fatal("expected non-nil Query")
	}

	// JSONB ? stays, id = ? becomes $1.
	wantSQL := `DELETE FROM "docs" WHERE data ? 'stale' AND id = $1`
	if q.sql != wantSQL {
		t.Errorf("got  %q\nwant %q", q.sql, wantSQL)
	}
	want := []interface{}{99}
	if len(q.params) != len(want) || q.params[0] != want[0] {
		t.Errorf("params: got %v, want %v", q.params, want)
	}
}

// TestSelectQuery_JSONB_NoRegression verifies that the existing multi-param
// WHERE with PostgreSQL still works correctly after the refactor.
func TestSelectQuery_JSONB_NoRegression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "name").
		From("users").
		Where("status = ?", "active").
		Where("age > ?", 18).
		Build()

	if q == nil {
		t.Fatal("expected non-nil Query")
	}

	wantSQL := `SELECT "id", "name" FROM "users" WHERE status = $1 AND age > $2`
	if q.sql != wantSQL {
		t.Errorf("regression: got  %q\nwant %q", q.sql, wantSQL)
	}
	want := []interface{}{"active", 18}
	if len(q.params) != len(want) {
		t.Fatalf("params length: got %d, want %d", len(q.params), len(want))
	}
	for i, w := range want {
		if q.params[i] != w {
			t.Errorf("params[%d]: got %v, want %v", i, q.params[i], w)
		}
	}
}
