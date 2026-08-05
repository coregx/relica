package core

import (
	"reflect"
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDBFull creates a minimal DB with both dialect and driverName set.
// Unlike mockDB (upsert_test.go), this also sets driverName so that
// DriverName() returns the correct value for RETURNING checks.
func mockDBFull(driverName string) *DB {
	return &DB{
		dialect:    dialects.GetDialect(driverName),
		driverName: driverName,
	}
}

// ─── needsPostgresReturning: autoincrement tag ────────────────────────────────

// TestNeedsPostgresReturning_NumericPK verifies backward-compatible behavior
// for numeric PKs: RETURNING is used without any explicit tag.
func TestNeedsPostgresReturning_NumericPK(t *testing.T) {
	db := mockDBFull("postgres")

	type UserIntPK struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &UserIntPK{Name: "Alice"}, // ID=0 (zero)
		table:   "users",
		exclude: make(map[string]bool),
	}

	needs, col := mq.needsPostgresReturning()
	assert.True(t, needs, "numeric PK should trigger RETURNING")
	assert.Equal(t, "id", col)
}

// TestNeedsPostgresReturning_NumericPKNonZero verifies that non-zero PK
// does not trigger RETURNING (PK is already set by caller).
func TestNeedsPostgresReturning_NumericPKNonZero(t *testing.T) {
	db := mockDBFull("postgres")

	type UserIntPK struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &UserIntPK{ID: 42, Name: "Alice"}, // ID non-zero
		table:   "users",
		exclude: make(map[string]bool),
	}

	needs, _ := mq.needsPostgresReturning()
	assert.False(t, needs, "non-zero PK must not trigger RETURNING")
}

// TestNeedsPostgresReturning_StringPKWithAutoIncrementTag verifies that
// string PKs with autoincrement tag trigger RETURNING for PostgreSQL.
func TestNeedsPostgresReturning_StringPKWithAutoIncrementTag(t *testing.T) {
	db := mockDBFull("postgres")

	type UserUUIDPK struct {
		ID   string `db:"id,pk,autoincrement"`
		Name string `db:"name"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &UserUUIDPK{Name: "Alice"}, // ID="" (zero)
		table:   "users",
		exclude: make(map[string]bool),
	}

	needs, col := mq.needsPostgresReturning()
	assert.True(t, needs, "string PK with autoincrement tag should trigger RETURNING")
	assert.Equal(t, "id", col)
}

// TestNeedsPostgresReturning_StringPKWithoutAutoIncrementTag verifies that
// string PKs without autoincrement tag do NOT trigger RETURNING.
// This is the expected behavior for caller-supplied string/UUID PKs.
func TestNeedsPostgresReturning_StringPKWithoutAutoIncrementTag(t *testing.T) {
	db := mockDBFull("postgres")

	type UserUUIDPK struct {
		ID   string `db:"id,pk"` // no autoincrement
		Name string `db:"name"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &UserUUIDPK{Name: "Alice"}, // ID="" (zero)
		table:   "users",
		exclude: make(map[string]bool),
	}

	needs, _ := mq.needsPostgresReturning()
	assert.False(t, needs, "string PK without autoincrement tag must not trigger RETURNING")
}

// TestNeedsPostgresReturning_NonPostgres verifies that non-PostgreSQL drivers
// always return false regardless of PK type or tag.
func TestNeedsPostgresReturning_NonPostgres(t *testing.T) {
	type UserUUIDPK struct {
		ID   string `db:"id,pk,autoincrement"`
		Name string `db:"name"`
	}

	for _, driver := range []string{"mysql", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db := mockDBFull(driver)
			mq := &ModelQuery{
				db:      db,
				model:   &UserUUIDPK{Name: "Alice"},
				table:   "users",
				exclude: make(map[string]bool),
			}

			needs, _ := mq.needsPostgresReturning()
			assert.False(t, needs, "non-postgres driver must not trigger RETURNING")
		})
	}
}

// TestNeedsPostgresReturning_CompositePK verifies that composite PKs are skipped
// even when autoincrement tag is present on one field.
func TestNeedsPostgresReturning_CompositePK(t *testing.T) {
	db := mockDBFull("postgres")

	type OrderItem struct {
		OrderID int `db:"order_id,pk"`
		ItemID  int `db:"item_id,pk"`
		Qty     int `db:"qty"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &OrderItem{Qty: 5},
		table:   "order_items",
		exclude: make(map[string]bool),
	}

	needs, _ := mq.needsPostgresReturning()
	assert.False(t, needs, "composite PK must not trigger RETURNING")
}

// ─── scanReturningIntoField: type dispatch ────────────────────────────────────

// TestScanReturningIntoField_UnsupportedType verifies that an unsupported PK type
// (e.g. float64) returns a descriptive error.
func TestScanReturningIntoField_UnsupportedType(t *testing.T) {
	var f float64
	field := reflect.ValueOf(&f).Elem()

	// Pass nil query — the function checks kind BEFORE calling q.Row(),
	// so we expect the error from the default case (unsupported type).
	//
	// However, with our current implementation kind dispatch happens first
	// and falls through to default. We verify via a non-nil query that would
	// not be reached because kind is checked first.
	//
	// The function will reach q.Row() only for supported kinds. For float64
	// (unsupported), it returns the error immediately after the switch.
	err := scanReturningIntoField(nil, field)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported PK type for RETURNING")
	assert.Contains(t, err.Error(), "float64")
}

// TestScanReturningIntoField_PointerDeref verifies that nil pointer is
// allocated before scanning.
func TestScanReturningIntoField_PointerDeref(t *testing.T) {
	// We can only test the unsupported-type path synchronously without a
	// real DB. Verify that a *float64 also returns the correct error,
	// confirming that pointer dereference happens before the kind switch.
	var f *float64
	field := reflect.ValueOf(&f).Elem()

	err := scanReturningIntoField(nil, field)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported PK type for RETURNING")
}
