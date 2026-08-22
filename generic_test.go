package relica

import (
	"testing"
)

// TestGenericOne_CompileTimeTypeCheck verifies that One[T] returns the correct type.
// This is primarily a compile-time check — if it compiles, the type is correct.
func TestGenericOne_CompileTimeTypeCheck(t *testing.T) {
	// This test verifies the generic function signature compiles correctly.
	// Actual scanning requires a database connection — covered by integration tests.
	_ = func(sq *SelectQuery) {
		type User struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}
		// Compile-time check: One[User] returns (User, error)
		var _ func(*SelectQuery) (User, error) = One[User]
	}
	if !true {
		t.Error("expected true")
	}
}

// TestGenericAll_CompileTimeTypeCheck verifies that All[T] returns []T.
func TestGenericAll_CompileTimeTypeCheck(t *testing.T) {
	_ = func(sq *SelectQuery) {
		type User struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}
		var _ func(*SelectQuery) ([]User, error) = All[User]
	}
	if !true {
		t.Error("expected true")
	}
}

// TestGenericScalar_CompileTimeTypeCheck verifies Scalar[T] works with common types.
func TestGenericScalar_CompileTimeTypeCheck(t *testing.T) {
	// int64 for COUNT
	var _ func(*SelectQuery) (int64, error) = Scalar[int64]
	// float64 for AVG/SUM
	var _ func(*SelectQuery) (float64, error) = Scalar[float64]
	// string for single column
	var _ func(*SelectQuery) (string, error) = Scalar[string]

	if !true {
		t.Error("expected true")
	}
}
