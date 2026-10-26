package core

import (
	"strings"
	"testing"
)

// reflect cannot read or write an unexported field, so it is no column. One
// the code marked as a column — a db tag, or an embedded single value whose
// Scan/Value say "store me as one value" — was dropped without a word (a
// `db:",pk"` key vanished, and Update then failed far from the cause or used
// another key). Untagged unexported fields stay skipped: they are not data.

type (
	evTaggedUnexported struct {
		ID   int    `db:"id"`
		name string `db:"name"` //nolint:unused // proves the tag is an error
	}
	evTaggedUnexportedSkip struct {
		ID   int    `db:"id"`
		note string `db:"-"` //nolint:unused // db:"-" says "not a column": still fine
	}
	evUnexportedPK struct {
		evNote `db:",pk"`
		Name   string
	}
	evOnlyUnexported struct{ evNote }
)

func TestUnexportedFields_MarkedAsColumnsAreErrors(t *testing.T) {
	db := mockDB("postgres")
	for _, c := range []struct {
		model any
		want  string
	}{
		{&evTaggedUnexported{}, "unexported field name has a db tag"},
		{&evUnexported{}, "embeds unexported core.evNote"},
		{&evUnexportedPK{}, "unexported field evNote has a db tag"},
		{&evOnlyUnexported{}, "embeds unexported core.evNote"},
	} {
		_, err := db.structValues(c.model)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%T: err = %v, want it to contain %q", c.model, err, c.want)
		}
	}
}

func TestUnexportedFields_UntaggedOrSkippedStayIgnored(t *testing.T) {
	db := mockDB("postgres")
	if got := strings.Join(columnsOf(t, db, &evTaggedUnexportedSkip{}), ","); got != "id" {
		t.Errorf("db:\"-\" unexported: columns = %s, want id", got)
	}
	if got := strings.Join(columnsOf(t, db, &evNoExported2{}), ","); got != "id" {
		t.Errorf("untagged unexported: columns = %s, want id", got)
	}
}

type evNoExported2 struct {
	ID int `db:"id"`
	mu int //nolint:unused // an untagged unexported field is not data
}
