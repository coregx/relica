package core

import (
	"database/sql"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Issue #47: BatchInsert(...).ValuesStructs(rows) builds a multi-row INSERT
// (and upsert) from a slice of structs; a batch over the database's
// bind-parameter limit is a build error instead of a statement the database
// rejects.

type bsItem struct {
	ID    int64  `db:"id,pk"`
	Name  string `db:"name"`
	Label string `db:"label"`
}

type bsOther struct {
	ID   int64  `db:"id,pk"`
	Name string `db:"name"`
}

type bsPublic struct {
	ID       int64  `db:"id,pk"`
	PublicID string `db:"public_id,autoid:usr"`
	Name     string `db:"name"`
}

func buildErrHas(t *testing.T, q *Query, want string) {
	t.Helper()
	if q.prepErr == nil || !strings.Contains(q.prepErr.Error(), want) {
		sqlText := q.sql
		if len(sqlText) > 200 {
			sqlText = sqlText[:200] + "…"
		}
		t.Fatalf("err = %v, want it to contain %q (sql %q)", q.prepErr, want, sqlText)
	}
}

func TestValuesStructs_DerivedColumns_Upsert(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	q := qb.BatchInsert("items", nil).
		ValuesStructs([]bsItem{{Name: "a", Label: "x"}, {Name: "b", Label: "y"}}).
		OnConflict("name").DoUpdateAll().Build()
	buildOK(t, q, `INSERT INTO "items" ("label", "name") VALUES ($1, $2), ($3, $4) `+
		`ON CONFLICT ("name") DO UPDATE SET "label" = EXCLUDED."label"`)
	if want := []any{"x", "a", "y", "b"}; !reflect.DeepEqual(q.params, want) {
		t.Fatalf("params = %v, want %v", q.params, want)
	}
}

func TestValuesStructs_DerivedColumns_SameAsBatchInsertStruct(t *testing.T) {
	for _, d := range allDialects {
		qb := &QueryBuilder{db: mockDB(d)}
		rows := []*bsItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}
		got := qb.BatchInsert("items", nil).ValuesStructs(rows).Build()
		want := qb.BatchInsertStruct("items", rows)
		if got.prepErr != nil || want.prepErr != nil || got.sql != want.sql || !reflect.DeepEqual(got.params, want.params) {
			t.Fatalf("%s: ValuesStructs %q %v (%v) != BatchInsertStruct %q %v (%v)",
				d, got.sql, got.params, got.prepErr, want.sql, want.params, want.prepErr)
		}
	}
}

func TestValuesStructs_ExplicitColumns(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("mysql")}
	q := qb.BatchInsert("items", []string{"name", "label"}).
		ValuesStructs([]*bsItem{{Name: "a", Label: "x"}, {Name: "b", Label: "y"}}).
		OnConflict("name").DoNothing().Build()
	buildOK(t, q, "INSERT INTO `items` (`name`, `label`) VALUES (?, ?), (?, ?) "+
		"ON DUPLICATE KEY UPDATE `name` = `name`")
	if want := []any{"a", "x", "b", "y"}; !reflect.DeepEqual(q.params, want) {
		t.Fatalf("params = %v, want %v", q.params, want)
	}
	// Mixed with Values: explicit columns accept both.
	q = qb.BatchInsert("items", []string{"name", "label"}).
		Values("c", "z").ValuesStructs([]bsItem{{Name: "d", Label: "w"}}).Build()
	buildOK(t, q, "INSERT INTO `items` (`name`, `label`) VALUES (?, ?), (?, ?)")
}

func TestValuesStructs_Errors(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	cases := map[string]struct {
		q    *Query
		want string
	}{
		"not a slice": {qb.BatchInsert("items", nil).ValuesStructs(bsItem{}).Build(), "expects a slice"},
		"array":       {qb.BatchInsert("items", nil).ValuesStructs([1]bsItem{}).Build(), "expects a slice"},
		"empty slice": {qb.BatchInsert("items", nil).ValuesStructs([]bsItem{}).Build(), "empty slice"},
		"nil element": {qb.BatchInsert("items", nil).ValuesStructs([]*bsItem{nil}).Build(), "row 0"},
		"unknown column": {qb.BatchInsert("items", []string{"name", "nope"}).
			ValuesStructs([]bsItem{{Name: "a"}}).Build(), `"nope"`},
		"zero pk listed": {qb.BatchInsert("items", []string{"id", "name"}).
			ValuesStructs([]bsItem{{Name: "a"}}).Build(), "zero primary key"},
		"mixed types": {qb.BatchInsert("items", nil).
			ValuesStructs([]any{bsItem{Name: "a"}, bsOther{Name: "b"}}).Build(), "row 1"},
		"derived after Values": {qb.BatchInsert("items", nil).Values().
			ValuesStructs([]bsItem{{Name: "a"}}).Build(), "without columns"},
		"derived twice": {qb.BatchInsert("items", nil).ValuesStructs([]bsItem{{Name: "a"}}).
			ValuesStructs([]bsItem{{Name: "b"}}).Build(), "without columns"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { buildErrHas(t, c.q, c.want) })
	}
}

func TestBatchInsertStruct_MixedTypes_IsError(t *testing.T) {
	// v0.20.0 wrote the second type's missing columns as NULL.
	qb := &QueryBuilder{db: mockDB("sqlite")}
	buildErrHas(t, qb.BatchInsertStruct("items", []any{bsItem{Name: "a"}, bsOther{Name: "b"}}), "row 1")
}

func TestBatchInsertStruct_EmptyAutoID_IsError(t *testing.T) {
	// v0.20.0 inserted "" into the autoid column; Model().Insert() generates
	// the ID, a batch does not.
	qb := &QueryBuilder{db: mockDB("postgres")}
	buildErrHas(t, qb.BatchInsertStruct("users", []bsPublic{{Name: "a"}}), "public_id")
	buildErrHas(t, qb.BatchInsert("users", nil).ValuesStructs([]bsPublic{{Name: "a"}}).Build(), "public_id")
	buildOK(t, qb.BatchInsertStruct("users", []bsPublic{{PublicID: "usr_1", Name: "a"}}),
		`INSERT INTO "users" ("name", "public_id") VALUES ($1, $2)`)
}

// paramBatch builds a batch of rows×cols parameters.
func paramBatch(qb *QueryBuilder, rows, cols int) *Query {
	columns := make([]string, cols)
	for i := range columns {
		columns[i] = "c" + string(rune('a'+i))
	}
	biq := qb.BatchInsert("t", columns)
	row := make([]any, cols)
	for i := 0; i < rows; i++ {
		biq.Values(row...)
	}
	return biq.Build()
}

func TestBatchInsert_BindParamLimit(t *testing.T) {
	limits := map[string]int{"postgres": 65535, "mysql": 65535, "sqlite": 32766}
	for d, limit := range limits {
		t.Run(d, func(t *testing.T) {
			qb := &QueryBuilder{db: mockDB(d)}
			const cols = 2
			maxRows := limit / cols
			if q := paramBatch(qb, maxRows, cols); q.prepErr != nil {
				t.Fatalf("%d rows (%d params) at the limit: %v", maxRows, maxRows*cols, q.prepErr)
			}
			q := paramBatch(qb, maxRows+1, cols)
			buildErrHas(t, q, "limit")
			for _, want := range []string{
				strconv.Itoa(limit), strconv.Itoa(maxRows) + " rows", "Transactional",
			} {
				if !strings.Contains(q.prepErr.Error(), want) {
					t.Errorf("error %q does not name %q", q.prepErr, want)
				}
			}
		})
	}
}

func TestBatchInsert_BindParamLimit_InTx(t *testing.T) {
	// In a transaction the statement goes to tx.ExecContext unprepared, and the
	// MySQL and PostgreSQL drivers may interpolate it (interpolateParams, pgx
	// simple protocol), so a batch over the limit can work there today: no
	// build error. SQLite binds every parameter: the limit stands.
	for d, wantErr := range map[string]bool{"postgres": false, "mysql": false, "sqlite": true} {
		qb := &QueryBuilder{db: mockDB(d), tx: &sql.Tx{}}
		q := paramBatch(qb, 40000, 2)
		if (q.prepErr != nil) != wantErr {
			t.Errorf("%s in a transaction: err = %v, want error %v", d, q.prepErr, wantErr)
		}
	}
}

func TestBatchInsertStruct_BindParamLimit(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("sqlite")}
	rows := make([]bsOther, 16384) // 2 columns: 32768 parameters
	for i := range rows {
		rows[i] = bsOther{ID: int64(i + 1), Name: "n"}
	}
	buildErrHas(t, qb.BatchInsertStruct("t", rows), "32766")
}
