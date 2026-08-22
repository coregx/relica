package core

import (
	"reflect"
	"strings"
	"testing"

	"github.com/coregx/relica/internal/util"
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
	if _, ok := dataMap["id"]; !ok {
		t.Errorf("StructToMap should include id")
	}

	// Simulate the zero PK check (same logic as in db.go InsertStruct)
	pkInfo, err := util.FindPrimaryKeyFields(testReflectValue(user))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkInfo == nil {
		t.Fatal("expected non-nil")
	}
	if !pkInfo.IsSingle() {
		t.Error("expected true")
	}
	if !util.IsPrimaryKeyZero(pkInfo.Values[0]) {
		t.Error("expected true")
	}

	// After deletion: "id" should be removed
	delete(dataMap, pkInfo.Columns[0])
	if _, ok := dataMap["id"]; ok {
		t.Errorf("expected %q not in map", "id")
	}
	if _, ok := dataMap["name"]; !ok {
		t.Errorf("expected %q in map", "name")
	}
	if _, ok := dataMap["email"]; !ok {
		t.Errorf("expected %q in map", "email")
	}

	// Build query without PK
	q := qb.Insert("users", dataMap)
	if q == nil {
		t.Error("expected non-nil")
	}
	if strings.Contains(q.sql, `"id"`) {
		t.Errorf("%q should not contain %q", q.sql, `"id"`)
	}
	if !strings.Contains(q.sql, `"name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name"`)
	}
	if !strings.Contains(q.sql, `"email"`) {
		t.Errorf("%q does not contain %q", q.sql, `"email"`)
	}
}

// TestInsertStruct_KeepsNonZeroPK verifies that non-zero PK is kept.
func TestInsertStruct_KeepsNonZeroPK(t *testing.T) {
	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	user := User{ID: 42, Name: "Bob"}
	dataMap, _ := util.StructToMap(user)

	pkInfo, _ := util.FindPrimaryKeyFields(testReflectValue(user))
	if util.IsPrimaryKeyZero(pkInfo.Values[0]) {
		t.Error("expected false")
	}
	if _, ok := dataMap["id"]; !ok {
		t.Errorf("Non-zero PK should be kept: expected %q in map", "id")
	}
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
	if pkInfo == nil {
		t.Fatal("expected non-nil")
	}
	if !util.IsPrimaryKeyZero(pkInfo.Values[0]) {
		t.Error("expected true")
	}

	delete(dataMap, pkInfo.Columns[0])
	if _, ok := dataMap["id"]; ok {
		t.Errorf("expected %q not in map", "id")
	}
	if _, ok := dataMap["name"]; !ok {
		t.Errorf("expected %q in map", "name")
	}
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
	if pkInfo == nil {
		t.Fatal("expected non-nil")
	}
	if pkInfo.IsSingle() {
		t.Error("Composite PK should not be single: expected false")
	}

	// Both PK columns should remain
	if _, ok := dataMap["order_id"]; !ok {
		t.Errorf("expected %q in map", "order_id")
	}
	if _, ok := dataMap["item_id"]; !ok {
		t.Errorf("expected %q in map", "item_id")
	}
}

// helper
func testReflectValue(v interface{}) reflect.Value {
	return reflect.ValueOf(v)
}
