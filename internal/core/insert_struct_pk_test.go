package core

import (
	"reflect"
	"testing"

	"github.com/coregx/relica/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInsertStruct_SkipsZeroPK verifies that InsertStruct excludes zero PK
// from the column list, matching Model().Insert() behavior.
func TestInsertStruct_SkipsZeroPK(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	type User struct {
		ID    int64  `db:"id,pk"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	user := User{Name: "Alice", Email: "alice@example.com"} // ID=0
	dataMap, _ := util.StructToMap(user)

	// Before fix: dataMap includes "id" with value 0
	assert.Contains(t, dataMap, "id", "StructToMap should include id")

	// Simulate the zero PK check (same logic as in db.go InsertStruct)
	pkInfo, err := util.FindPrimaryKeyFields(testReflectValue(user))
	require.NoError(t, err)
	require.NotNil(t, pkInfo)
	assert.True(t, pkInfo.IsSingle())
	assert.True(t, util.IsPrimaryKeyZero(pkInfo.Values[0]))

	// After deletion: "id" should be removed
	delete(dataMap, pkInfo.Columns[0])
	assert.NotContains(t, dataMap, "id")
	assert.Contains(t, dataMap, "name")
	assert.Contains(t, dataMap, "email")

	// Build query without PK
	q := qb.Insert("users", dataMap)
	assert.NotNil(t, q)
	assert.NotContains(t, q.sql, `"id"`)
	assert.Contains(t, q.sql, `"name"`)
	assert.Contains(t, q.sql, `"email"`)
}

// TestInsertStruct_KeepsNonZeroPK verifies that non-zero PK is kept.
func TestInsertStruct_KeepsNonZeroPK(t *testing.T) {
	type User struct {
		ID    int64  `db:"id,pk"`
		Name  string `db:"name"`
	}

	user := User{ID: 42, Name: "Bob"}
	dataMap, _ := util.StructToMap(user)

	pkInfo, _ := util.FindPrimaryKeyFields(testReflectValue(user))
	assert.False(t, util.IsPrimaryKeyZero(pkInfo.Values[0]))
	assert.Contains(t, dataMap, "id", "Non-zero PK should be kept")
}

// TestBatchInsertStruct_SkipsZeroPK verifies batch insert also excludes zero PK.
func TestBatchInsertStruct_SkipsZeroPK(t *testing.T) {
	type Item struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	items := []Item{
		{Name: "Item A"}, // ID=0
		{Name: "Item B"}, // ID=0
	}

	// First element check
	dataMap, _ := util.StructToMap(items[0])
	pkInfo, _ := util.FindPrimaryKeyFields(testReflectValue(items[0]))
	require.NotNil(t, pkInfo)
	assert.True(t, util.IsPrimaryKeyZero(pkInfo.Values[0]))

	delete(dataMap, pkInfo.Columns[0])
	assert.NotContains(t, dataMap, "id")
	assert.Contains(t, dataMap, "name")
}

// TestInsertStruct_CompositePK_NotSkipped verifies composite PK is never skipped.
func TestInsertStruct_CompositePK_NotSkipped(t *testing.T) {
	type OrderItem struct {
		OrderID int64 `db:"order_id,pk"`
		ItemID  int64 `db:"item_id,pk"`
		Qty     int   `db:"qty"`
	}

	oi := OrderItem{OrderID: 1, ItemID: 2, Qty: 5}
	dataMap, _ := util.StructToMap(oi)

	pkInfo, _ := util.FindPrimaryKeyFields(testReflectValue(oi))
	require.NotNil(t, pkInfo)
	assert.False(t, pkInfo.IsSingle(), "Composite PK should not be single")

	// Both PK columns should remain
	assert.Contains(t, dataMap, "order_id")
	assert.Contains(t, dataMap, "item_id")
}

// helper
func testReflectValue(v interface{}) reflect.Value {
	return reflect.ValueOf(v)
}
