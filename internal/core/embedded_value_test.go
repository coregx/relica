package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Embedded single-value types (time.Time, sql.Scanner, driver.Valuer) are one
// column named by the db tag, else by Naming from the field name; only an
// embedded plain struct is flattened.

// EvMeta is a JSON column: Scan on *EvMeta, Value on EvMeta.
type EvMeta struct {
	Note string   `json:"note"`
	Tags []string `json:"tags"`
}

func (m *EvMeta) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), m)
	case []byte:
		return json.Unmarshal(v, m)
	case nil:
		*m = EvMeta{}
		return nil
	}
	return fmt.Errorf("EvMeta: cannot scan %T", src)
}

func (m EvMeta) Value() (driver.Value, error) {
	b, err := json.Marshal(m)
	return string(b), err
}

// EvValuerOnly declares only driver.Valuer (value receiver).
type EvValuerOnly struct{ A, B int }

func (v EvValuerOnly) Value() (driver.Value, error) { return fmt.Sprintf("%d:%d", v.A, v.B), nil }

// EvScannerOnly declares only sql.Scanner (pointer receiver).
type EvScannerOnly struct{ A, B int }

func (s *EvScannerOnly) Scan(any) error { return nil }

// EvBase gets Scan and Value promoted from sql.NullString, so Go sees it as
// one value.
type EvBase struct{ sql.NullString }

// EvAudit is a plain struct whose fields are value types: flattened, each
// field one column.
type EvAudit struct {
	CreatedAt time.Time
	Note      sql.NullString
}

// EvStamp wraps time.Time without Scan or Value of its own: time.Time has
// neither, so EvStamp is a plain struct, flattened to its embedded field, which
// is the single value time.Time → column "time". Deliberate: the predicate
// follows what the type declares, and a wrapper that wants to be one column
// declares Scan/Value or is tagged on the embedding field's own type.
type EvStamp struct{ time.Time }

// evNote is an unexported single-value type.
type evNote struct{ V string }

func (n *evNote) Scan(any) error              { return nil }
func (n evNote) Value() (driver.Value, error) { return n.V, nil }

type (
	evNullString struct {
		ID int
		sql.NullString
	}
	evNullStringTagged struct {
		ID             int
		sql.NullString `db:"note"`
	}
	evNullStringSkipped struct {
		ID             int
		sql.NullString `db:"-"`
	}
	evTime struct {
		ID int
		time.Time
	}
	evNullGen struct {
		ID int
		sql.Null[string]
	}
	evNullTime struct {
		ID int
		sql.NullTime
	}
	evMetaModel struct {
		ID int
		EvMeta
	}
	evValuerOnly struct {
		ID int
		EvValuerOnly
	}
	evScannerOnly struct {
		ID int
		EvScannerOnly
	}
	evBaseModel struct {
		ID int
		EvBase
	}
	evPtrNull struct {
		ID int
		*sql.NullString
	}
	evPtrTime struct {
		ID int
		*time.Time
	}
	evUnexported struct {
		ID int
		evNote
	}
	evAuditModel struct {
		ID int
		EvAudit
	}
	evStampModel struct {
		ID int
		EvStamp
	}
	evPKValue struct {
		sql.NullString `db:"code,pk"`
		Name           string
	}
)

func TestEmbeddedValue_Columns(t *testing.T) {
	db := mockDB("postgres")
	cases := []struct {
		model any
		want  string
	}{
		{&evNullString{}, "id,null_string"},
		{&evNullStringTagged{}, "id,note"},
		{&evNullStringSkipped{}, "id"},
		{&evTime{}, "id,time"},
		{&evNullGen{}, "id,null"}, // Go names an embedded generic field after the type without arguments
		{&evNullTime{}, "id,null_time"},
		{&evMetaModel{}, "ev_meta,id"},
		{&evValuerOnly{}, "ev_valuer_only,id"},
		{&evScannerOnly{}, "ev_scanner_only,id"},
		{&evBaseModel{}, "ev_base,id"},
		{&evPtrNull{}, "id,null_string"},
		{&evPtrTime{}, "id,time"},
		{&evUnexported{}, "id"},                                 // skipped like any unexported field
		{&miUser{}, "created_at,http_log,id,updated,user_name"}, // unexported embedded plain structs still promote
		{&evAuditModel{}, "created_at,id,note"},
		{&evStampModel{}, "id,time"},
		{&evPKValue{}, "code,name"},
	}
	for _, c := range cases {
		got := strings.Join(columnsOf(t, db, c.model), ",")
		if got != c.want {
			t.Errorf("%T: columns = %s, want %s", c.model, got, c.want)
		}
	}
}

func TestEmbeddedValue_PKAndErrors(t *testing.T) {
	db := mockDB("postgres")
	pk, err := db.primaryKey(reflect.ValueOf(evPKValue{NullString: sql.NullString{String: "x", Valid: true}}))
	if err != nil {
		t.Fatal(err)
	}
	if pk.Columns[0] != "code" || pk.Values[0].Interface() != (sql.NullString{String: "x", Valid: true}) {
		t.Fatalf("pk = %v %v", pk.Columns, pk.Values[0])
	}
	// Embedded pointer to a plain struct and a tag on an embedded plain struct
	// stay errors: flattening through nil cannot be written or read, and a tag
	// there is ambiguous (prefix in ozzo-dbx/sqlx/scany, one field in json).
	for _, c := range []struct {
		model any
		want  string
	}{
		{&miEmbedPtr{}, "embedded pointer"},
		{&miEmbedTagged{}, "db tag on an embedded struct"},
	} {
		if _, err := db.structValues(c.model); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%T: err = %v, want %q", c.model, err, c.want)
		}
	}
}

type evWrite struct {
	ID             int `db:"id,pk"`
	sql.NullString `db:"note"`
	time.Time      `db:"at"`
	EvMeta
}

func TestEmbeddedValue_InsertStructSQL(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := evWrite{ID: 7, NullString: sql.NullString{String: "n", Valid: true}, Time: at, EvMeta: EvMeta{Note: "x"}}
	cases := map[string]string{
		"postgres": `INSERT INTO "t" ("at", "ev_meta", "id", "note") VALUES ($1, $2, $3, $4)`,
		"mysql":    "INSERT INTO `t` (`at`, `ev_meta`, `id`, `note`) VALUES (?, ?, ?, ?)",
		"sqlite":   "INSERT INTO `t` (`at`, `ev_meta`, `id`, `note`) VALUES (?, ?, ?, ?)",
	}
	for dialect, want := range cases {
		q := mockDB(dialect).Builder().InsertStruct("t", &m)
		if q.prepErr != nil {
			t.Fatalf("%s: %v", dialect, q.prepErr)
		}
		if q.sql != want {
			t.Errorf("%s: sql = %s\nwant %s", dialect, q.sql, want)
		}
		if len(q.params) != 4 {
			t.Fatalf("%s: params = %v", dialect, q.params)
		}
		if q.params[0] != at || q.params[2] != 7 || q.params[3] != m.NullString {
			t.Errorf("%s: params = %#v", dialect, q.params)
		}
		if meta, ok := q.params[1].(EvMeta); !ok || meta.Note != "x" {
			t.Errorf("%s: ev_meta param = %#v, want the EvMeta value (its Value method writes it)", dialect, q.params[1])
		}
	}
}

func TestEmbeddedValue_DiffFieldsOnlyEmbeddedColumn(t *testing.T) {
	db := mockDB("postgres")
	original := evWrite{ID: 1, NullString: sql.NullString{String: "a", Valid: true}, Time: time.Unix(0, 0).UTC()}
	current := original
	current.NullString = sql.NullString{String: "b", Valid: true}
	changed, err := newTestMQ(db, &current, "t").diffFields(&original)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed["note"] != current.NullString {
		t.Fatalf("changed = %#v, want only note", changed)
	}
}

// evDB is a memdb-backed sqlite-dialect DB with table ev_rows.
func evDB(t *testing.T, opts ...Option) *DB {
	t.Helper()
	sqlDB, err := sql.Open("memdb", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1) // memdb keeps one database per connection
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := WrapDB(sqlDB, "sqlite")
	for _, o := range opts {
		o(db)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE ev_rows (id INTEGER, note TEXT, at TEXT, ev_meta TEXT)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEmbeddedValue_RoundTrip(t *testing.T) {
	var events []QueryEvent
	db := evDB(t, WithQueryHook(func(_ context.Context, e QueryEvent) { events = append(events, e) }))
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	in := evWrite{ID: 1, NullString: sql.NullString{String: "hello", Valid: true}, Time: at,
		EvMeta: EvMeta{Note: "n", Tags: []string{"a", "b"}}}
	if err := db.Model(&in).Table("ev_rows").Insert(); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var got evWrite
	if err := db.Builder().Select().From("ev_rows").Where(Eq("id", 1)).One(&got); err != nil {
		t.Fatalf("one: %v", err)
	}
	if got.ID != 1 || got.NullString != in.NullString || !got.Time.Equal(at) ||
		got.Note != "n" || strings.Join(got.Tags, ",") != "a,b" {
		t.Fatalf("round trip = %+v, want %+v", got, in)
	}

	var all []evWrite
	if err := db.Builder().Select().From("ev_rows").All(&all); err != nil || len(all) != 1 || all[0].NullString != in.NullString {
		t.Fatalf("all = %+v, %v", all, err)
	}

	events = nil
	changed := got
	changed.NullString = sql.NullString{String: "bye", Valid: true}
	if err := db.Model(&changed).Table("ev_rows").UpdateChanged(&got); err != nil {
		t.Fatalf("update changed: %v", err)
	}
	if len(events) != 1 || events[0].SQL != "UPDATE `ev_rows` SET `note` = ? WHERE `id` = ?" {
		t.Fatalf("UpdateChanged SQL = %v, want only the embedded note column", events)
	}
}

// evPtrRow reads and writes an embedded *sql.NullString like a named pointer
// field: nil is NULL.
type evPtrRow struct {
	ID              int `db:"id,pk"`
	*sql.NullString `db:"note"`
}

func TestEmbeddedValue_PointerNullRoundTrip(t *testing.T) {
	db := evDB(t)
	if err := db.Model(&evPtrRow{ID: 1}).Table("ev_rows").Insert(); err != nil {
		t.Fatalf("insert nil: %v", err)
	}
	if err := db.Model(&evPtrRow{ID: 2, NullString: &sql.NullString{String: "x", Valid: true}}).Table("ev_rows").Insert(); err != nil {
		t.Fatalf("insert value: %v", err)
	}
	var rows []evPtrRow
	if err := db.Builder().Select("id", "note").From("ev_rows").All(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].NullString != nil || rows[1].NullString == nil || rows[1].String != "x" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestEmbeddedValue_RequireFieldsNamesColumn(t *testing.T) {
	db := evDB(t)
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE ev_ids (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO ev_ids (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	var got evNullString
	err := db.Builder().Select().From("ev_ids").One(&got)
	if err == nil || !strings.Contains(err.Error(), `expects column "null_string"`) {
		t.Fatalf("err = %v, want the missing null_string column named", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		t.Fatal("not a missing-row error")
	}
}
