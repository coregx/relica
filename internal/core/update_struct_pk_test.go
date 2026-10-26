package core

import (
	"strings"
	"testing"
)

// TestUpdateStruct_LeavesPrimaryKeyOut verifies U1: UpdateStruct never writes
// the primary key (a pk tag or the ID fallback), as Model().Update() does not,
// and a struct with nothing but its key is an error.
func TestUpdateStruct_LeavesPrimaryKeyOut(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}

	type byName struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	type byTag struct {
		Key   int    `db:"item_key,pk"`
		Label string `db:"label"`
	}
	for name, tc := range map[string]struct {
		data any
		want string
	}{
		"ID fallback": {&byName{Name: "a"}, `UPDATE "t" SET "name" = $1 WHERE "id" = $2`},
		"pk tag":      {&byTag{Label: "b"}, `UPDATE "t" SET "label" = $1 WHERE "id" = $2`},
	} {
		q := qb.UpdateStruct("t", tc.data).Where(Eq("id", 1)).Build()
		if q.prepErr != nil {
			t.Fatalf("%s: %v", name, q.prepErr)
		}
		if q.sql != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", name, q.sql, tc.want)
		}
	}

	type onlyKey struct {
		ID int `db:"id"`
	}
	q := qb.UpdateStruct("t", &onlyKey{ID: 1}).Where(Eq("id", 1)).Build()
	if q.prepErr == nil || !strings.Contains(q.prepErr.Error(), "no column besides its primary key") {
		t.Errorf("only a key: prepErr = %v", q.prepErr)
	}
}
