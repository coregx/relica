package util

import (
	"reflect"
	"testing"
)

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
