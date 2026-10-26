package core

import (
	"strings"
	"testing"
)

// InsertStruct / BatchInsertStruct leave a zero single-column primary key out
// of the INSERT so the database generates it, as Model().Insert() does.

func TestInsertStruct_SkipsZeroPK(t *testing.T) {
	type User struct {
		ID    int64  `db:"id,pk"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}
	qb := &QueryBuilder{db: mockDB("postgres")}
	q := qb.InsertStruct("users", User{Name: "Alice", Email: "alice@example.com"})
	buildOK(t, q, `INSERT INTO "users" ("email", "name") VALUES ($1, $2)`)
}

func TestInsertStruct_KeepsNonZeroPK(t *testing.T) {
	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}
	qb := &QueryBuilder{db: mockDB("postgres")}
	buildOK(t, qb.InsertStruct("users", &User{ID: 42, Name: "Bob"}),
		`INSERT INTO "users" ("id", "name") VALUES ($1, $2)`)
}

func TestInsertStruct_CompositePK_NotSkipped(t *testing.T) {
	type OrderItem struct {
		OrderID int `db:"order_id,pk"`
		ItemID  int `db:"item_id,pk"`
		Qty     int `db:"qty"`
	}
	qb := &QueryBuilder{db: mockDB("postgres")}
	buildOK(t, qb.InsertStruct("order_items", OrderItem{Qty: 1}),
		`INSERT INTO "order_items" ("item_id", "order_id", "qty") VALUES ($1, $2, $3)`)
}

func TestInsertStruct_EmbeddedAndUntaggedColumns(t *testing.T) {
	// FIX-011: embedded structs were written as one column named after the
	// type, untagged fields under their Go name.
	type Audit struct {
		CreatedAt string
		CreatedBy string
	}
	type Doc struct {
		Audit
		ID    int `db:"id,pk"`
		Title string
	}
	qb := &QueryBuilder{db: mockDB("sqlite")}
	buildOK(t, qb.InsertStruct("docs", Doc{Audit: Audit{CreatedAt: "t", CreatedBy: "u"}, Title: "x"}),
		"INSERT INTO `docs` (`created_at`, `created_by`, `title`) VALUES (?, ?, ?)")
}

func TestBatchInsertStruct_SkipsZeroPKInEveryRow(t *testing.T) {
	type Item struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}
	qb := &QueryBuilder{db: mockDB("postgres")}
	buildOK(t, qb.BatchInsertStruct("items", []Item{{Name: "a"}, {Name: "b"}}),
		`INSERT INTO "items" ("name") VALUES ($1), ($2)`)
	buildOK(t, qb.BatchInsertStruct("items", []*Item{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}),
		`INSERT INTO "items" ("id", "name") VALUES ($1, $2), ($3, $4)`)
}

func TestBatchInsertStruct_MixedZeroAndExplicitPK_IsError(t *testing.T) {
	// v0.19 dropped the id column when the FIRST row's key was zero, so the
	// explicit keys of later rows were silently lost.
	type Item struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}
	qb := &QueryBuilder{db: mockDB("postgres")}
	q := qb.BatchInsertStruct("items", []Item{{Name: "a"}, {ID: 7, Name: "b"}})
	if q.prepErr == nil || !strings.Contains(q.prepErr.Error(), "1 of 2 rows have a zero primary key") {
		t.Fatalf("err = %v, sql = %q", q.prepErr, q.sql)
	}
}

func TestBatchInsertStruct_InvalidInput(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	for _, in := range []any{42, []int{}, []string{"x"}} {
		if q := qb.BatchInsertStruct("t", in); q.prepErr == nil {
			t.Errorf("%T %v: expected an error", in, in)
		}
	}
}
