package drivercompat

import (
	"fmt"
	"testing"

	"github.com/coregx/relica"
	"github.com/lib/pq"
)

// lib/pq v1.2.0 has no SQLState method; the code comes from the Code field,
// read by reflection. The message is German, so text matching cannot help.
func TestLegacyLibPQ_CodeField(t *testing.T) {
	if _, ok := any(&pq.Error{}).(interface{ SQLState() string }); ok {
		t.Fatal("lib/pq in this module has SQLState(): the reflection path is not exercised")
	}

	tests := []struct {
		code string
		fn   func(error) bool
		name string
	}{
		{"23505", relica.IsUniqueViolation, "IsUniqueViolation"},
		{"23503", relica.IsForeignKeyViolation, "IsForeignKeyViolation"},
		{"23502", relica.IsNotNullViolation, "IsNotNullViolation"},
		{"23514", relica.IsCheckViolation, "IsCheckViolation"},
		{"40P01", relica.IsDeadlock, "IsDeadlock"},
		{"40001", relica.IsSerializationFailure, "IsSerializationFailure"},
		{"55P03", relica.IsLockTimeout, "IsLockTimeout"},
	}
	for _, tc := range tests {
		err := fmt.Errorf("query: %w", &pq.Error{Code: pq.ErrorCode(tc.code), Message: "Fehler"})
		if !tc.fn(err) {
			t.Errorf("%s(SQLSTATE %s) = false", tc.name, tc.code)
		}
	}

	if relica.IsUniqueViolation(&pq.Error{Code: "23503", Message: "duplicate key value violates unique constraint"}) {
		t.Error("the message overrode the code")
	}
}
