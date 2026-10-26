package core

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// Column scans the first column of every row, as documented and as ozzo-dbx
// did; the other columns are read and discarded. It failed with "expected 3
// destination arguments in Scan, not 1".

func columnDB(t *testing.T) *DB {
	t.Helper()
	db := evDB(t)
	ctx := context.Background()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for i, note := range []any{"a", nil} {
		if _, err := db.ExecContext(ctx, `INSERT INTO ev_rows (id, note, at, ev_meta) VALUES (?, ?, ?, ?)`,
			i+1, note, at, []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestColumn_FirstOfSeveralColumns(t *testing.T) {
	db := columnDB(t)

	var ids []int64
	if err := db.Builder().Select("id", "note", "at", "ev_meta").From("ev_rows").Column(&ids); err != nil {
		t.Fatalf("builder: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("ids = %v", ids)
	}

	// The discarded columns hold NULL, a time and bytes: they are not
	// converted, so their types do not matter.
	var notes []sql.NullString
	if err := db.NewQuery(`SELECT note, at, ev_meta, id FROM ev_rows`).Column(&notes); err != nil {
		t.Fatalf("raw: %v", err)
	}
	if len(notes) != 2 || notes[0].String != "a" || notes[1].Valid {
		t.Fatalf("notes = %+v", notes)
	}

	var all []any
	if err := db.NewQuery(`SELECT id, note FROM ev_rows`).Column(&all); err != nil || len(all) != 2 {
		t.Fatalf("[]any = %v, %v", all, err)
	}
}

func TestColumn_FirstColumnTypeStillChecked(t *testing.T) {
	db := columnDB(t)
	// The first column is converted as before: text into int64 fails.
	var notes []int64
	err := db.NewQuery(`SELECT note, id FROM ev_rows`).Column(&notes)
	if err == nil || !strings.Contains(err.Error(), `column index 0, name "note"`) {
		t.Fatalf("err = %v", err)
	}
}
