package core

import (
	"strings"
	"testing"
)

// A struct that maps to no column at all is an error that names the type:
// read, it matched nothing and returned a zero value with err == nil; written,
// it reached the database as an empty INSERT or an UPDATE without SET.

type (
	evNoExported struct {
		a int    //nolint:unused // proves unexported fields give no column
		b string //nolint:unused
	}
	evAllSkipped struct {
		A int    `db:"-"`
		B string `db:"-"`
	}
	evEmpty          struct{}
	evOnlyUnexported struct{ evNote } // an unexported single value is skipped
)

func emptyModelErr(typ string) string {
	return "relica: core." + typ + ` has no columns: export fields or tag them with db:"..."`
}

func TestEmptyModel_StructValues(t *testing.T) {
	db := mockDB("postgres")
	for _, c := range []struct {
		model any
		typ   string
	}{
		{&evNoExported{}, "evNoExported"},
		{evAllSkipped{}, "evAllSkipped"},
		{&evEmpty{}, "evEmpty"},
		{&evOnlyUnexported{}, "evOnlyUnexported"},
	} {
		if _, err := db.structValues(c.model); err == nil || err.Error() != emptyModelErr(c.typ) {
			t.Errorf("%T: err = %v, want %q", c.model, err, emptyModelErr(c.typ))
		}
	}
}

func TestEmptyModel_EveryPathIsAnError(t *testing.T) {
	db := evDB(t)
	want := emptyModelErr("evAllSkipped")
	m := &evAllSkipped{A: 1}
	check := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, want)
		}
	}
	qb := db.Builder()
	check("InsertStruct", qb.InsertStruct("ev_rows", m).prepErr)
	check("BatchInsertStruct", qb.BatchInsertStruct("ev_rows", []evAllSkipped{{}, {}}).prepErr)
	check("UpdateStruct", qb.UpdateStruct("ev_rows", m).buildErr)
	check("Model.Insert", db.Model(m).Table("ev_rows").Insert())
	check("Model.Update", db.Model(m).Table("ev_rows").Update())
	check("Model.Upsert", db.Model(m).Table("ev_rows").Upsert())
	check("Model.Delete", db.Model(m).Table("ev_rows").Delete())
	check("Model.UpdateChanged", db.Model(m).Table("ev_rows").UpdateChanged(&evAllSkipped{}))

	// Reads: before the query runs, so an empty table reports it too.
	var one evAllSkipped
	check("One", qb.Select().From("ev_rows").One(&one))
	var all []evAllSkipped
	check("All", qb.Select().From("ev_rows").All(&all))
}
