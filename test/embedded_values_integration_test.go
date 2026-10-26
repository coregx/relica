//go:build integration

package test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Embedded single-value types (time.Time, sql.Scanner, driver.Valuer) are one
// column each, on every database: written through their Value, read through
// their Scan, NULL through an embedded pointer.

// Meta is a JSON column: Scan on *Meta, Value on Meta.
type Meta struct {
	Note string   `json:"note"`
	Tags []string `json:"tags"`
}

func (m *Meta) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), m)
	case []byte:
		return json.Unmarshal(v, m)
	case nil:
		*m = Meta{}
		return nil
	}
	return fmt.Errorf("Meta: cannot scan %T", src)
}

func (m Meta) Value() (driver.Value, error) {
	b, err := json.Marshal(m)
	return string(b), err
}

// f3Note: the embedded NullString and Time are tagged, Meta takes its column
// name from Naming (meta).
type f3Note struct {
	ID             int `db:"id,pk"`
	sql.NullString `db:"note"`
	time.Time      `db:"at"`
	Meta
}

func (f3Note) TableName() string { return "f3_note" }

// f3NotePtr reads and writes note through an embedded pointer: nil is NULL.
type f3NotePtr struct {
	ID              int `db:"id,pk"`
	*sql.NullString `db:"note"`
	time.Time       `db:"at"`
	Meta
}

func (f3NotePtr) TableName() string { return "f3_note" }

func createEmbeddedValueTables(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS f3_note")
	require.NoError(t, err)
	ts := "TIMESTAMP"
	if dialect == dialectMySQL {
		ts = "DATETIME" // MySQL TIMESTAMP converts through the session time zone
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE f3_note (
		id INTEGER PRIMARY KEY,
		note VARCHAR(50) NULL,
		at `+ts+` NOT NULL,
		meta TEXT NOT NULL
	)`)
	require.NoError(t, err)
}

func runEmbeddedValueTests(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	createEmbeddedValueTables(t, db, dialect)
	defer func() { _, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS f3_note") }()

	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("RoundTrip", func(t *testing.T) {
		in := f3Note{ID: 1, NullString: sql.NullString{String: "hello", Valid: true}, Time: at,
			Meta: Meta{Note: "n", Tags: []string{"a", "b"}}}
		require.NoError(t, db.Model(&in).Insert())

		var got f3Note
		require.NoError(t, db.Select().From("f3_note").Where(relica.Eq("id", 1)).One(&got))
		assert.Equal(t, 1, got.ID)
		assert.Equal(t, in.NullString, got.NullString)
		assert.True(t, got.Time.Equal(at), "at = %v, want %v", got.Time, at)
		assert.Equal(t, in.Meta, got.Meta)
	})

	t.Run("NullThroughEmbeddedPointer", func(t *testing.T) {
		require.NoError(t, db.Model(&f3NotePtr{ID: 2, Time: at, Meta: Meta{Note: "m"}}).Insert())

		var viaPtr f3NotePtr
		require.NoError(t, db.Select().From("f3_note").Where(relica.Eq("id", 2)).One(&viaPtr))
		assert.Nil(t, viaPtr.NullString)

		var viaValue f3Note
		require.NoError(t, db.Select().From("f3_note").Where(relica.Eq("id", 2)).One(&viaValue))
		assert.False(t, viaValue.NullString.Valid)
		assert.Equal(t, "m", viaValue.Meta.Note)

		var withValue f3NotePtr
		require.NoError(t, db.Select().From("f3_note").Where(relica.Eq("id", 1)).One(&withValue))
		require.NotNil(t, withValue.NullString)
		assert.Equal(t, "hello", withValue.NullString.String)
	})

	t.Run("UpdateChangedEmbeddedColumn", func(t *testing.T) {
		var orig f3Note
		require.NoError(t, db.Select().From("f3_note").Where(relica.Eq("id", 1)).One(&orig))
		changed := orig
		changed.NullString = sql.NullString{String: "bye", Valid: true}
		require.NoError(t, db.Model(&changed).UpdateChanged(&orig))

		var got f3Note
		require.NoError(t, db.Select().From("f3_note").Where(relica.Eq("id", 1)).One(&got))
		assert.Equal(t, "bye", got.NullString.String)
		assert.True(t, got.Time.Equal(at))
		assert.Equal(t, orig.Meta, got.Meta)
	})

	t.Run("SingleValueIsNotARow", func(t *testing.T) {
		const hint = " is a single value, not a row: use Row(&v) or Column(&[]v)"
		_, err := relica.One[time.Time](db.Select("at").From("f3_note").Where(relica.Eq("id", 1)))
		assert.EqualError(t, err, "relica: time.Time"+hint)
		_, err = relica.All[sql.NullString](db.Select("note").From("f3_note"))
		assert.EqualError(t, err, "relica: sql.NullString"+hint)
		var none sql.NullString
		err = db.Select("note").From("f3_note").Where(relica.Eq("id", 99)).One(&none)
		assert.EqualError(t, err, "relica: sql.NullString"+hint, "reported before the query, not as ErrNotFound")

		var got time.Time
		require.NoError(t, db.Select("at").From("f3_note").Where(relica.Eq("id", 1)).Row(&got))
		assert.True(t, got.Equal(at))
		var notes []sql.NullString
		require.NoError(t, db.Select("note").From("f3_note").OrderBy("id").Column(&notes))
		assert.Equal(t, []sql.NullString{{String: "bye", Valid: true}, {}}, notes)
	})
}

func TestEmbeddedValues_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runEmbeddedValueTests(t, ds.DB, dialectPostgres)
}

func TestEmbeddedValues_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runEmbeddedValueTests(t, ds.DB, dialectMySQL)
}

func TestEmbeddedValues_SQLite(t *testing.T) {
	requireDB(t, dialectSQLite)
	db, err := relica.Open(dialectSQLite, filepath.Join(t.TempDir(), "ev.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runEmbeddedValueTests(t, db, dialectSQLite)
}
