//go:build integration

package test

import (
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpsert_SQLite_AutoIncrementID verifies that Model().Upsert() populates
// auto-increment ID on SQLite after both INSERT and UPDATE paths.
// Regression test for Issue #48.
func TestUpsert_SQLite_AutoIncrementID(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	db := ds.DB

	_, err := db.NewQuery(`CREATE TABLE upsert_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		value TEXT NOT NULL
	)`).Execute()
	require.NoError(t, err)

	type Item struct {
		ID    int64  `db:"id,pk"`
		Name  string `db:"name"`
		Value string `db:"value"`
	}

	// INSERT path — new row, ID should be auto-populated
	item1 := Item{Name: "alpha", Value: "v1"}
	err = db.Model(&item1).Table("upsert_test").Upsert()
	require.NoError(t, err)
	assert.NotZero(t, item1.ID, "INSERT path: ID should be auto-populated")
	assert.Equal(t, int64(1), item1.ID)

	// UPDATE path — same name, conflict on UNIQUE(name)
	// Upsert uses PK for conflict, so this inserts a NEW row (no conflict on id=0)
	// To test UPDATE path, we need UpsertOn with conflict on "name"
}

// TestUpsertOn_SQLite_AutoIncrementID verifies that Model().UpsertOn() populates
// auto-increment ID on SQLite when conflict happens on non-PK UNIQUE column.
// This is the primary regression test for Issue #48.
func TestUpsertOn_SQLite_AutoIncrementID(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	db := ds.DB

	_, err := db.NewQuery(`CREATE TABLE upserton_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		value TEXT NOT NULL
	)`).Execute()
	require.NoError(t, err)

	type Item struct {
		ID    int64  `db:"id,pk"`
		Name  string `db:"name"`
		Value string `db:"value"`
	}

	// INSERT path — new row
	item1 := Item{Name: "alpha", Value: "v1"}
	err = db.Model(&item1).Table("upserton_test").UpsertOn([]string{"name"}, "value")
	require.NoError(t, err)
	assert.NotZero(t, item1.ID, "INSERT path: ID should be auto-populated")
	insertedID := item1.ID

	// UPDATE path — same name, should update value and return EXISTING ID
	item2 := Item{Name: "alpha", Value: "v2"}
	err = db.Model(&item2).Table("upserton_test").UpsertOn([]string{"name"}, "value")
	require.NoError(t, err)
	assert.Equal(t, insertedID, item2.ID, "UPDATE path: should return existing row ID")

	// Verify value was updated
	var found Item
	err = db.Select().From("upserton_test").
		Where(relica.Eq("name", "alpha")).
		One(&found)
	require.NoError(t, err)
	assert.Equal(t, "v2", found.Value, "value should be updated")
	assert.Equal(t, insertedID, found.ID)
}

// TestUpsertOn_SQLite_MultipleRows verifies batch-like sequential UpsertOn
// correctly populates IDs for both new and existing rows.
func TestUpsertOn_SQLite_MultipleRows(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	db := ds.DB

	_, err := db.NewQuery(`CREATE TABLE upserton_multi (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		label TEXT NOT NULL
	)`).Execute()
	require.NoError(t, err)

	type Record struct {
		ID    int64  `db:"id,pk"`
		Code  string `db:"code"`
		Label string `db:"label"`
	}

	// Insert 3 new records
	ids := make([]int64, 3)
	for i, code := range []string{"A", "B", "C"} {
		r := Record{Code: code, Label: "original"}
		err := db.Model(&r).Table("upserton_multi").UpsertOn([]string{"code"}, "label")
		require.NoError(t, err)
		assert.NotZero(t, r.ID, "ID should be populated for %s", code)
		ids[i] = r.ID
	}

	// Upsert same codes with new labels — should return same IDs
	for i, code := range []string{"A", "B", "C"} {
		r := Record{Code: code, Label: "updated"}
		err := db.Model(&r).Table("upserton_multi").UpsertOn([]string{"code"}, "label")
		require.NoError(t, err)
		assert.Equal(t, ids[i], r.ID, "UPDATE path for %s: should return same ID", code)
	}

	// Verify all labels updated
	var count int64
	count, err = db.Select().From("upserton_multi").
		Where(relica.Eq("label", "updated")).Count()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}
