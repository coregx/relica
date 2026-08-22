package core

import (
	"strings"
	"testing"
)

// TestScanRow_RejectsScalarTypes verifies that scanRow (used by One())
// correctly rejects non-struct dest types.
// Regression test: scanReturningID previously called One(&id) instead of
// Row(&id) for RETURNING clause scanning — One would fail with this error.
func TestScanRow_RejectsScalarTypes(t *testing.T) {
	tests := []struct {
		name string
		dest interface{}
		want string
	}{
		{"int64", new(int64), "pointer to int64"},
		{"string", new(string), "pointer to string"},
		{"float64", new(float64), "pointer to float64"},
		{"bool", new(bool), "pointer to bool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := globalScanner.scanRow(nil, tt.dest)
			if err == nil {
				t.Error("expected error")
			}
			if err != nil && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestScanRow_RejectsNonPointer verifies non-pointer is rejected.
func TestScanRow_RejectsNonPointer(t *testing.T) {
	type User struct {
		ID int `db:"id"`
	}
	err := globalScanner.scanRow(nil, User{})
	if err == nil {
		t.Error("expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "pointer to struct") {
		t.Errorf("%q does not contain %q", err.Error(), "pointer to struct")
	}
}

// TestScanRow_RejectsNilPointer verifies nil pointer gives clear error.
func TestScanRow_RejectsNilPointer(t *testing.T) {
	type User struct {
		ID int `db:"id"`
	}
	var u *User
	err := globalScanner.scanRow(nil, u)
	if err == nil {
		t.Error("expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "nil pointer") {
		t.Errorf("%q does not contain %q", err.Error(), "nil pointer")
	}
}
