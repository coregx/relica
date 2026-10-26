//go:build cgo

package drivercompat

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/coregx/relica"
	"github.com/mattn/go-sqlite3"
)

// mattn/go-sqlite3 returns sqlite3.Error by value with an ExtendedCode field.
// SQLITE_BUSY and SQLITE_BUSY_SNAPSHOT share the message "database is
// locked", so the snapshot case passes only when the code is read.
func TestMattnSQLite_ExtendedCode(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "m.db") + "?_foreign_keys=1&_journal_mode=WAL&_busy_timeout=200"
	db, err := relica.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	for _, q := range []string{
		"CREATE TABLE p (id INT PRIMARY KEY)",
		"CREATE TABLE c (id INT PRIMARY KEY, p INT NOT NULL REFERENCES p(id), e TEXT UNIQUE, q INT CHECK (q > 0))",
		"INSERT INTO p VALUES (1)",
		"INSERT INTO c VALUES (1, 1, 'a', 1)",
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(q, err)
		}
	}

	insert := func(id, p int, e string, q int) error {
		_, err := db.Insert("c", map[string]any{"id": id, "p": p, "e": e, "q": q}).Execute()
		return err
	}
	cases := []struct {
		name string
		err  error
		fn   func(error) bool
	}{
		{"unique", insert(2, 1, "a", 1), relica.IsUniqueViolation},
		{"foreign key", insert(2, 9, "b", 1), relica.IsForeignKeyViolation},
		{"check", insert(2, 1, "b", 0), relica.IsCheckViolation},
	}
	for _, tc := range cases {
		var se sqlite3.Error
		if !errors.As(tc.err, &se) {
			t.Fatalf("%s: want sqlite3.Error, got %T: %v", tc.name, tc.err, tc.err)
		}
		if !tc.fn(tc.err) {
			t.Errorf("%s not classified: %v (extended code %d)", tc.name, tc.err, se.ExtendedCode)
		}
	}

	// Stale WAL snapshot: SQLITE_BUSY_SNAPSHOT.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.NewQuery("SELECT COUNT(*) FROM p").Row(&n); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO p VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	_, err = tx.NewQuery("INSERT INTO p VALUES (3)").Execute()
	if !relica.IsSerializationFailure(err) || relica.IsLockTimeout(err) {
		t.Errorf("stale snapshot: IsSerializationFailure=%v IsLockTimeout=%v, err %v",
			relica.IsSerializationFailure(err), relica.IsLockTimeout(err), err)
	}
}
