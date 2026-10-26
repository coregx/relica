package core

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Primary-key rules of the struct-info builder (ported from the removed
// util.FindPrimaryKeyFields tests, plus the v0.20.0 changes).

func pkColumns(t *testing.T, model any) ([]string, *DB) {
	t.Helper()
	db := mockDB("postgres")
	pk, err := db.primaryKey(reflect.ValueOf(model))
	if err != nil {
		t.Fatalf("%T: %v", model, err)
	}
	return pk.Columns, db
}

func TestPrimaryKey_Priorities(t *testing.T) {
	type Legacy struct {
		ID   int    `db:"pk"`
		Body string `db:"body"`
	}
	type Composite struct {
		OrderID int `db:"order_id,pk"`
		ItemID  int `db:"item_id,pk"`
		Qty     int `db:"qty"`
	}
	type TaggedWins struct {
		ID   int `db:"id"`
		Code int `db:"code,pk"`
	}
	type IDUntagged struct {
		ID   int
		Name string
	}
	type IDTagged struct {
		ID      int    `db:"comment_id"`
		Message string `db:"message"`
	}
	type IdCase struct {
		Id   int
		Name string
	}
	cases := []struct {
		model any
		want  string
	}{
		{Legacy{ID: 5}, "id"},
		{Composite{}, "order_id,item_id"},
		{TaggedWins{}, "code"},
		{IDUntagged{}, "id"},
		{IDTagged{}, "comment_id"},
		{IdCase{}, "id"},
		{&IDUntagged{}, "id"},
	}
	for _, c := range cases {
		cols, _ := pkColumns(t, c.model)
		if got := strings.Join(cols, ","); got != c.want {
			t.Errorf("%T: pk = %s, want %s", c.model, got, c.want)
		}
	}
}

func TestPrimaryKey_ValuesAndAutoIncrement(t *testing.T) {
	type Single struct {
		Key string `db:"key,pk,autoincrement"`
	}
	type Composite struct {
		A int `db:"a,pk,autoincrement"`
		B int `db:"b,pk"`
	}
	db := mockDB("postgres")
	pk, err := db.primaryKey(reflect.ValueOf(Single{Key: "k"}))
	if err != nil || !pk.AutoIncrement || pk.Values[0].String() != "k" || !pk.IsSingle() {
		t.Fatalf("single autoincrement: %+v, %v", pk, err)
	}
	pk, err = db.primaryKey(reflect.ValueOf(Composite{A: 1, B: 2}))
	if err != nil || pk.AutoIncrement || !pk.IsComposite() || pk.Values[1].Int() != 2 {
		t.Fatalf("composite: %+v, %v", pk, err)
	}
}

func TestPrimaryKey_Errors(t *testing.T) {
	type NoPK struct {
		Name string
	}
	type SkippedID struct {
		ID   int `db:"-"`
		Name string
	}
	db := mockDB("postgres")
	for _, m := range []any{NoPK{}, SkippedID{}, 42} {
		if _, err := db.primaryKey(reflect.ValueOf(m)); err == nil {
			t.Errorf("%T: expected an error", m)
		}
	}
	var nilPtr *NoPK
	if _, err := db.primaryKey(reflect.ValueOf(nilPtr)); err == nil {
		t.Error("nil pointer: expected an error")
	}
}

func TestStructValues_TypesAndErrors(t *testing.T) {
	type Row struct {
		ID      int            `db:"id"`
		Note    sql.NullString `db:"note"`
		At      time.Time      `db:"at"`
		Tags    []string       `db:"tags"`
		Zero    int            `db:"zero"`
		private int            //nolint:unused // skipped
	}
	db := mockDB("postgres")
	m, err := db.structValues(&Row{ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(getKeys(m), ","); got != "at,id,note,tags,zero" {
		t.Fatalf("columns = %s (sql.NullString and time.Time are values, not flattened)", got)
	}
	if m["zero"] != 0 {
		t.Fatalf("zero values are included: %v", m["zero"])
	}
	var nilRow *Row
	for _, bad := range []any{nilRow, 42, "x"} {
		if _, err := db.structValues(bad); err == nil {
			t.Errorf("%T: expected an error", bad)
		}
	}
}
