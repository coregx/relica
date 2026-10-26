package core

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// ADR-018 read path: default ScanRequireFields — a mapped field without a
// result column is an error unless the query lists its columns; extra result
// columns are ignored. ScanLenient = ozzo-dbx (silent). ScanStrict = also an
// error for an extra result column.

type scanItem struct {
	ID    int64  `db:"id"`
	Name  string `db:"name"`
	Score int64  `db:"score"`
}

type scanItemExtra struct {
	ID      int64  `db:"id"`
	Name    string `db:"name"`
	Score   int64  `db:"score"`
	Missing string `db:"missing"`
	Ignored string `db:"-"`
}

type scanItemLessFields struct {
	ID int64 `db:"id"`
}

func scanDB(t *testing.T, opts ...Option) *DB {
	t.Helper()
	sqlDB, err := sql.Open("memdb", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := WrapDB(sqlDB, "sqlite")
	for _, o := range opts {
		o(db)
	}
	ctx := context.Background()
	for _, s := range []string{
		`CREATE TABLE cov_items (id INTEGER, name TEXT, score INTEGER)`,
		`INSERT INTO cov_items (id, name, score) VALUES (1, 'alpha', 10)`,
	} {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestScanMode_RequireFields_Default(t *testing.T) {
	db := scanDB(t)
	var full scanItem
	if err := db.Builder().Select().From("cov_items").One(&full); err != nil || full.Name != "alpha" {
		t.Fatalf("matching struct: %+v, %v", full, err)
	}

	// A field without a column in a SELECT * result: error (was a silent zero).
	var extra scanItemExtra
	err := db.Builder().Select().From("cov_items").One(&extra)
	if err == nil || !strings.Contains(err.Error(), `field Missing expects column "missing"`) {
		t.Fatalf("missing column: err = %v", err)
	}
	var all []scanItemExtra
	if err := db.Builder().Select().From("cov_items").All(&all); err == nil {
		t.Fatal("All: expected the missing-column error")
	}
	if err := db.NewQuery("SELECT * FROM cov_items").One(&extra); err == nil {
		t.Fatal("NewQuery: expected the missing-column error")
	}

	// Listed columns: the caller asked for a subset.
	var subset scanItemExtra
	if err := db.Builder().Select("id", "name").From("cov_items").One(&subset); err != nil || subset.Name != "alpha" {
		t.Fatalf("listed columns: %+v, %v", subset, err)
	}

	// Extra result columns are ignored by default.
	var less scanItemLessFields
	if err := db.Builder().Select().From("cov_items").One(&less); err != nil || less.ID != 1 {
		t.Fatalf("extra columns: %+v, %v", less, err)
	}
}

func TestScanMode_Lenient(t *testing.T) {
	db := scanDB(t, WithScanMode(ScanLenient))
	var extra scanItemExtra
	if err := db.Builder().Select().From("cov_items").One(&extra); err != nil || extra.Name != "alpha" || extra.Missing != "" {
		t.Fatalf("lenient DB: %+v, %v", extra, err)
	}
	// Per-query override back to the strict default.
	if err := db.Builder().Select().From("cov_items").ScanMode(ScanRequireFields).One(&extra); err == nil {
		t.Fatal("per-query ScanRequireFields on a lenient DB: expected an error")
	}
}

func TestScanMode_Strict(t *testing.T) {
	db := scanDB(t, WithScanMode(ScanStrict))
	var less scanItemLessFields
	err := db.Builder().Select().From("cov_items").One(&less)
	if err == nil || !strings.Contains(err.Error(), `result column "name" matches no field`) {
		t.Fatalf("strict extra column: err = %v", err)
	}
	var full scanItem
	if err := db.Builder().Select().From("cov_items").One(&full); err != nil {
		t.Fatalf("strict exact match: %v", err)
	}
	// Per-query Lenient on NewQuery.
	if err := db.NewQuery("SELECT * FROM cov_items").ScanMode(ScanLenient).One(&less); err != nil || less.ID != 1 {
		t.Fatalf("per-query lenient: %+v, %v", less, err)
	}
}

func TestScanMode_SelectStarIsNotAList(t *testing.T) {
	db := scanDB(t)
	var extra scanItemExtra
	if err := db.Builder().Select("*").From("cov_items").One(&extra); err == nil {
		t.Fatal(`Select("*") must be checked like Select()`)
	}
}
