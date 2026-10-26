package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
)

// A field of type T whose Value method is on *T only: database/sql calls
// driver.Valuer on the argument itself, so T passed by value skipped Value —
// a struct failed with "unsupported type", and a defined basic type silently
// wrote its underlying value (EvLevel 3 as the integer 3).

// EvLevel writes as "level-N"; Value and Scan on the pointer.
type EvLevel int

func (l *EvLevel) Value() (driver.Value, error) { return fmt.Sprintf("level-%d", int(*l)), nil }

func (l *EvLevel) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("EvLevel: cannot scan %T", src)
	}
	_, err := fmt.Sscanf(s, "level-%d", (*int)(l))
	return err
}

// EvPair writes as "A/B"; Value and Scan on the pointer.
type EvPair struct{ A, B int }

func (p *EvPair) Value() (driver.Value, error) { return fmt.Sprintf("%d/%d", p.A, p.B), nil }

func (p *EvPair) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("EvPair: cannot scan %T", src)
	}
	_, err := fmt.Sscanf(s, "%d/%d", &p.A, &p.B)
	return err
}

type evPtrValuerRow struct {
	ID      int            `db:"id,pk"`
	Level   EvLevel        `db:"note"`
	Pair    EvPair         `db:"at"`
	EvLevel `db:"ev_meta"` // embedded: same rule as a named field
}

// rawRow reads ev_rows row id with every column as stored by the driver.
func rawRow(t *testing.T, db *DB, id int) []any {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT note, at, ev_meta FROM ev_rows WHERE id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("row %d not found", id)
	}
	out := make([]any, 3)
	ptrs := []any{&out[0], &out[1], &out[2]}
	if err := rows.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPointerValuer_ModelInsertCallsValue(t *testing.T) {
	db := evDB(t)
	m := evPtrValuerRow{ID: 1, Level: 3, Pair: EvPair{1, 2}, EvLevel: 7}
	if err := db.Model(&m).Table("ev_rows").Insert(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := fmt.Sprint(rawRow(t, db, 1)); got != "[level-3 1/2 level-7]" {
		t.Fatalf("stored = %s, want [level-3 1/2 level-7]", got)
	}

	var back evPtrValuerRow
	if err := db.Builder().Select().From("ev_rows").Where(Eq("id", 1)).One(&back); err != nil {
		t.Fatal(err)
	}
	if back != m {
		t.Fatalf("read back %+v, want %+v", back, m)
	}
}

func TestPointerValuer_StructPassedByValue(t *testing.T) {
	db := evDB(t)
	// Not addressable: the field is copied, the copy's pointer passed.
	m := evPtrValuerRow{ID: 2, Level: 4, Pair: EvPair{5, 6}, EvLevel: 8}
	if _, err := db.Builder().InsertStruct("ev_rows", m).Execute(); err != nil {
		t.Fatalf("insert by value: %v", err)
	}
	if got := fmt.Sprint(rawRow(t, db, 2)); got != "[level-4 5/6 level-8]" {
		t.Fatalf("stored = %s", got)
	}
	// The argument is a copy: changing the model after building does not
	// change what was bound.
	m2 := evPtrValuerRow{ID: 3, Level: 1}
	q := db.Builder().InsertStruct("ev_rows", &m2)
	m2.Level = 9
	if _, err := q.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := rawRow(t, db, 3)[0]; got != "level-1" {
		t.Fatalf("stored note = %v, want level-1 (value at build time)", got)
	}
}

func TestPointerValuer_UpdateAndUpdateChanged(t *testing.T) {
	db := evDB(t)
	orig := evPtrValuerRow{ID: 1, Level: 1, Pair: EvPair{1, 1}, EvLevel: 1}
	if err := db.Model(&orig).Table("ev_rows").Insert(); err != nil {
		t.Fatal(err)
	}
	cur := orig
	cur.Level = 2
	if err := db.Model(&cur).Table("ev_rows").UpdateChanged(&orig); err != nil {
		t.Fatalf("update changed: %v", err)
	}
	if got := fmt.Sprint(rawRow(t, db, 1)); got != "[level-2 1/1 level-1]" {
		t.Fatalf("after UpdateChanged = %s", got)
	}
	cur.Pair = EvPair{3, 4}
	if err := db.Model(&cur).Table("ev_rows").Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := fmt.Sprint(rawRow(t, db, 1)); got != "[level-2 3/4 level-1]" {
		t.Fatalf("after Update = %s", got)
	}
}

type evLevelPK struct {
	Code EvLevel `db:"id,pk"`
	Name string  `db:"note"`
}

func TestPointerValuer_PrimaryKeyInWhere(t *testing.T) {
	db := evDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO ev_rows (id, note) VALUES (?, ?)`, "level-5", "x"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&evLevelPK{Code: 5, Name: "y"}).Table("ev_rows").Update(); err != nil {
		t.Fatalf("update by pk: %v", err)
	}
	var note sql.NullString
	if err := db.Builder().Select("note").From("ev_rows").Row(&note); err != nil || note.String != "y" {
		t.Fatalf("note = %+v, %v; the WHERE must bind level-5, not 5", note, err)
	}
}

func TestPointerValuer_SelectModelBindsValue(t *testing.T) {
	db := evDB(t)
	if _, err := db.ExecContext(context.Background(), `INSERT INTO ev_rows (id, note) VALUES (?, ?)`, "level-5", "x"); err != nil {
		t.Fatal(err)
	}
	m := evLevelPK{Code: 5}
	if err := db.Builder().Select().From("ev_rows").Model(&m); err != nil {
		t.Fatalf("Select().Model: %v; the WHERE must bind level-5, not 5", err)
	}
	if m.Name != "x" {
		t.Fatalf("model = %+v", m)
	}
}

func firstField(s any) reflect.Value { return reflect.ValueOf(s).Field(0) }

func TestArgValue(t *testing.T) {
	lv := EvLevel(3)
	cases := []struct {
		field reflect.Value
		want  string
	}{
		{firstField(struct{ F EvLevel }{3}), "*core.EvLevel"},          // Value on *T only: pointer to a copy
		{firstField(struct{ F EvPair }{EvPair{1, 2}}), "*core.EvPair"}, // same for a struct
		{firstField(struct{ F *EvLevel }{&lv}), "*core.EvLevel"},       // already a pointer
		{firstField(struct{ F EvMeta }{}), "core.EvMeta"},              // Value on T: as is
		{firstField(struct{ F sql.NullString }{}), "sql.NullString"},
		{firstField(struct{ F int }{7}), "int"},
		{firstField(struct{ F any }{EvLevel(3)}), "*core.EvLevel"}, // interface field: its dynamic value
		{firstField(struct{ F any }{7}), "int"},
	}
	for _, c := range cases {
		if got := fmt.Sprintf("%T", argValue(c.field)); got != c.want {
			t.Errorf("argValue(%s) = %s, want %s", c.field.Type(), got, c.want)
		}
	}
	if got := argValue(firstField(struct{ F any }{})); got != nil {
		t.Errorf("nil interface field = %v", got)
	}
	if got := argValue(firstField(struct{ F *EvLevel }{})); got != (*EvLevel)(nil) {
		t.Errorf("nil pointer field = %#v", got)
	}
	// The copy is taken at call time.
	src := struct{ F EvLevel }{1}
	arg := argValue(reflect.ValueOf(&src).Elem().Field(0))
	src.F = 2
	p, ok := arg.(*EvLevel)
	if !ok {
		t.Fatalf("argValue(EvLevel) = %T, want *EvLevel", arg)
	}
	if v, _ := p.Value(); v != "level-1" {
		t.Errorf("argument follows later changes: %v", v)
	}
}
