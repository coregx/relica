package core

import (
	"context"
	"strings"
	"testing"
)

// Phase 1e: a name Relica maps to a struct field or map entry and cannot
// find is an error, never ignored (AUDIT-001 silent-ignore class).

type unItem struct {
	ID    int64  `db:"id,pk"`
	Name  string `db:"name"`
	Score int64  `db:"score"`
}

func (unItem) TableName() string { return "cov_items" }

func wantErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want it to contain %q", err, want)
	}
}

func TestModelInsert_UnknownNames(t *testing.T) {
	db := openCovDB(t)
	seedCovTable(t, db)
	// Insert(attrs) used to skip a misspelled name and insert the rest.
	wantErr(t, db.Model(&unItem{ID: 9, Name: "x"}).Insert("name", "scroe"), `Insert field "scroe" is not a column of the model`)
	wantErr(t, db.Model(&unItem{ID: 9, Name: "x"}).Exclude("scroe").Insert(), `Exclude name "scroe" is not a column of the model`)
	// Nothing was inserted.
	n, err := db.Builder().Select().From("cov_items").Where(Eq("id", 9)).Count()
	if err != nil || n != 0 {
		t.Fatalf("rows with id 9 = %d, %v", n, err)
	}
}

func TestModelUpdate_UnknownNames(t *testing.T) {
	db := openCovDB(t)
	seedCovTable(t, db)
	wantErr(t, db.Model(&unItem{ID: 1, Name: "changed"}).Update("title"), `Update field "title" is not a column of the model`)
	wantErr(t, db.Model(&unItem{ID: 1, Name: "changed"}).Exclude("scroe").Update(), `Exclude name "scroe" is not a column of the model`)
	var got unItem
	if err := db.Builder().Select().From("cov_items").Where(Eq("id", 1)).One(&got); err != nil || got.Name == "changed" {
		t.Fatalf("row 1 = %+v, %v (must be unchanged)", got, err)
	}
}

func TestModelInsertUpdate_KnownNamesStillWork(t *testing.T) {
	db := openCovDB(t)
	seedCovTable(t, db)
	if err := db.Model(&unItem{ID: 9, Name: "x", Score: 5}).Insert("id", "name"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&unItem{ID: 9, Name: "y"}).Exclude("score").Update(); err != nil {
		t.Fatal(err)
	}
}

func TestBatchInsert_ValuesMap_UnknownKey(t *testing.T) {
	// AUDIT-001 P2: unknown keys were dropped silently (data loss on a typo).
	qb := &QueryBuilder{db: mockDB("postgres")}
	q := qb.BatchInsert("t", []string{"id", "note"}).ValuesMap(map[string]any{"id": 1, "note_typo": "lost"}).Build()
	buildErrContains(t, q, `ValuesMap key "note_typo" is not one of the batch columns`)
	// A missing key is still NULL.
	buildOK(t, qb.BatchInsert("t", []string{"id", "note"}).ValuesMap(map[string]any{"id": 1}).Build(),
		`INSERT INTO "t" ("id", "note") VALUES ($1, $2)`)
}

type unAutoID struct {
	ID       int64  `db:"id,pk"`
	PublicID string `db:"public_id,autoid:x,gen=never_registered"`
	Name     string `db:"name"`
}

func (unAutoID) TableName() string { return "cov_items" }

func TestAutoID_UnregisteredGenerator_IsError(t *testing.T) {
	// AUDIT-001 O2: an unknown gen= silently fell back to UUIDv7.
	db := openCovDB(t)
	wantErr(t, db.Model(&unAutoID{Name: "a"}).Insert(), `autoid generator "never_registered" is not registered`)
	wantErr(t, db.Model(&unAutoID{ID: 1, Name: "a"}).Upsert(), `autoid generator "never_registered" is not registered`)
	_ = context.Background()
}
