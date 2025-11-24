package util

import (
	"reflect"
	"testing"
)

// TestFindPrimaryKeyField_Basic tests basic PK field detection.
func TestFindPrimaryKeyField_Basic(t *testing.T) {
	type User struct {
		ID   int64  `db:"id"`
		Name string `db:"name"`
	}

	user := User{ID: 123, Name: "Alice"}
	v := reflect.ValueOf(&user).Elem()

	field, val, err := FindPrimaryKeyField(v)
	if err != nil {
		t.Fatalf("FindPrimaryKeyField() error = %v", err)
	}

	if field.Name != "ID" {
		t.Errorf("field.Name = %s, want ID", field.Name)
	}

	if val.Int() != 123 {
		t.Errorf("val.Int() = %d, want 123", val.Int())
	}
}

// TestIsPrimaryKeyZero_Basic tests zero detection.
func TestIsPrimaryKeyZero_Basic(t *testing.T) {
	if !IsPrimaryKeyZero(reflect.ValueOf(int64(0))) {
		t.Error("IsPrimaryKeyZero(0) should return true")
	}

	if IsPrimaryKeyZero(reflect.ValueOf(int64(123))) {
		t.Error("IsPrimaryKeyZero(123) should return false")
	}
}

// TestSetPrimaryKeyValue_Basic tests value setting.
func TestSetPrimaryKeyValue_Basic(t *testing.T) {
	var id int64
	v := reflect.ValueOf(&id).Elem()

	err := SetPrimaryKeyValue(v, 999)
	if err != nil {
		t.Fatalf("SetPrimaryKeyValue() error = %v", err)
	}

	if id != 999 {
		t.Errorf("id = %d, want 999", id)
	}
}
