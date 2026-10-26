package core

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

// One/All take a row; a single-value type (time.Time, sql.Scanner,
// driver.Valuer) as their destination was walked as a struct of columns and
// came back zero with err == nil. It is an error that names Row and Column,
// checked before the query runs so an empty result does not hide it.

func singleValueDB(t *testing.T) *DB {
	t.Helper()
	db := evDB(t)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO ev_rows (id, note, at, ev_meta) VALUES (?, ?, ?, ?)`, 1, "y", at, `{"note":"n"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE ev_empty (id INTEGER, note TEXT)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSingleValueDest_OneAndAllAreErrors(t *testing.T) {
	db := singleValueDB(t)
	cases := []struct {
		name string
		run  func(*SelectQuery) error
		typ  string
	}{
		{"One time.Time", func(q *SelectQuery) error { var v time.Time; return q.One(&v) }, "time.Time"},
		{"All time.Time", func(q *SelectQuery) error { var v []time.Time; return q.All(&v) }, "time.Time"},
		{"One NullString", func(q *SelectQuery) error { var v sql.NullString; return q.One(&v) }, "sql.NullString"},
		{"All *NullString", func(q *SelectQuery) error { var v []*sql.NullString; return q.All(&v) }, "sql.NullString"},
		{"One Null[string]", func(q *SelectQuery) error { var v sql.Null[string]; return q.One(&v) }, "sql.Null[string]"},
		{"One EvMeta", func(q *SelectQuery) error { var v EvMeta; return q.One(&v) }, "core.EvMeta"},
		{"All EvValuerOnly", func(q *SelectQuery) error { var v []EvValuerOnly; return q.All(&v) }, "core.EvValuerOnly"},
	}
	for _, c := range cases {
		for _, table := range []string{"ev_rows", "ev_empty"} {
			err := c.run(db.Builder().Select("note").From(table))
			want := "relica: " + c.typ + " is a single value, not a row: use Row(&v) or Column(&[]v)"
			if err == nil || err.Error() != want {
				t.Errorf("%s from %s: err = %v, want %q", c.name, table, err, want)
			}
			if errors.Is(err, sql.ErrNoRows) {
				t.Errorf("%s from %s: the type error must not be hidden by ErrNotFound", c.name, table)
			}
		}
	}
}

func TestSingleValueDest_RowAndColumnStillWork(t *testing.T) {
	db := singleValueDB(t)
	var note sql.NullString
	if err := db.Builder().Select("note").From("ev_rows").Row(&note); err != nil || note.String != "y" {
		t.Fatalf("Row = %+v, %v", note, err)
	}
	var notes []sql.NullString
	if err := db.Builder().Select("note").From("ev_rows").Column(&notes); err != nil || len(notes) != 1 || notes[0].String != "y" {
		t.Fatalf("Column = %+v, %v", notes, err)
	}
	var at time.Time
	if err := db.Builder().Select("at").From("ev_rows").Row(&at); err != nil || at.IsZero() {
		t.Fatalf("Row(time) = %v, %v", at, err)
	}
	var meta EvMeta
	if err := db.Builder().Select("ev_meta").From("ev_rows").Row(&meta); err != nil || meta.Note != "n" {
		t.Fatalf("Row(EvMeta) = %+v, %v", meta, err)
	}
}

func TestSingleValueDest_ModelsEmbeddingValuesStillRows(t *testing.T) {
	db := singleValueDB(t)
	// evWrite gets methods promoted from its embedded value types; it is still
	// a row because they come from fields that are columns.
	var w evWrite
	if err := db.Builder().Select().From("ev_rows").One(&w); err != nil || w.NullString.String != "y" {
		t.Fatalf("One(evWrite) = %+v, %v", w, err)
	}
	// EvBase as a whole row: its only field is the embedded NullString, one
	// column named null_string.
	var b EvBase
	err := db.Builder().Select().From("ev_rows").One(&b)
	if err == nil || !strings.Contains(err.Error(), `expects column "null_string"`) {
		t.Fatalf("One(EvBase) err = %v", err)
	}
}

func TestSingleValueDest_WriteIsError(t *testing.T) {
	db := mockDB("postgres")
	for _, model := range []any{time.Time{}, &sql.NullString{}, EvMeta{}} {
		q := db.Builder().InsertStruct("t", model)
		if q.prepErr == nil || !strings.Contains(q.prepErr.Error(), "is a single value, not a struct of columns") {
			t.Errorf("InsertStruct(%T): err = %v", model, q.prepErr)
		}
	}
	if err := singleValueDB(t).Model(&time.Time{}).Table("ev_rows").Insert(); err == nil ||
		!strings.Contains(err.Error(), "time.Time is a single value") {
		t.Fatalf("Model(time).Insert: %v", err)
	}
}
