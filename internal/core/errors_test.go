package core

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ============================================================================
// ErrNotFound and wrapErrNotFound tests (Issue #14)
// ============================================================================

func TestErrNotFound_IsSentinel(t *testing.T) {
	if ErrNotFound == nil {
		t.Fatal("expected non-nil ErrNotFound")
	}
	if ErrNotFound.Error() != "relica: record not found" {
		t.Errorf("got %q, want %q", ErrNotFound.Error(), "relica: record not found")
	}
}

func TestWrapErrNotFound_IsErrNotFound(t *testing.T) {
	err := wrapErrNotFound()
	if !errors.Is(err, ErrNotFound) {
		t.Error("wrapped error must satisfy errors.Is(err, ErrNotFound)")
	}
}

func TestWrapErrNotFound_IsSqlErrNoRows(t *testing.T) {
	err := wrapErrNotFound()
	if !errors.Is(err, sql.ErrNoRows) {
		t.Error("wrapped error must satisfy errors.Is(err, sql.ErrNoRows)")
	}
}

func TestWrapErrNotFound_ErrorMessage(t *testing.T) {
	err := wrapErrNotFound()
	msg := err.Error()
	if !strings.Contains(msg, "relica: record not found") {
		t.Errorf("%q does not contain %q", msg, "relica: record not found")
	}
	if !strings.Contains(msg, "sql: no rows in result set") {
		t.Errorf("%q does not contain %q", msg, "sql: no rows in result set")
	}
}

func TestErrNotFound_IsNotSqlErrNoRows(t *testing.T) {
	// The sentinel itself must NOT match sql.ErrNoRows —
	// only the wrapped version produced by wrapErrNotFound() should.
	if errors.Is(ErrNotFound, sql.ErrNoRows) {
		t.Error("ErrNotFound sentinel must not equal sql.ErrNoRows")
	}
}

func TestWrapErrNotFound_ChainedWrapping(t *testing.T) {
	// Simulate caller wrapping the error further (e.g. fmt.Errorf("%w", err)).
	inner := wrapErrNotFound()
	outer := fmt.Errorf("find user: %w", inner)

	if !errors.Is(outer, ErrNotFound) {
		t.Error("outer-wrapped error must still satisfy errors.Is(_, ErrNotFound)")
	}
	if !errors.Is(outer, sql.ErrNoRows) {
		t.Error("outer-wrapped error must still satisfy errors.Is(_, sql.ErrNoRows)")
	}
}

// ============================================================================
// IsUniqueViolation tests (Issue #15)
// ============================================================================

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		// PostgreSQL
		{
			name: "postgres duplicate key",
			err:  errors.New(`pq: duplicate key value violates unique constraint "users_email_key"`),
			want: true,
		},
		// MySQL
		{
			name: "mysql duplicate entry",
			err:  errors.New("Error 1062: Duplicate entry 'alice@example.com' for key 'users.email'"),
			want: true,
		},
		{
			name: "mysql error code 1062",
			err:  errors.New("Error 1062 (23000): Duplicate entry '1' for key 'PRIMARY'"),
			want: true,
		},
		// SQLite
		{
			name: "sqlite unique constraint",
			err:  errors.New("UNIQUE constraint failed: users.email"),
			want: true,
		},
		// Unrelated errors
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "foreign key error is not unique",
			err:  errors.New("FOREIGN KEY constraint failed"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsUniqueViolation(tc.err)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ============================================================================
// IsForeignKeyViolation tests (Issue #15)
// ============================================================================

func TestIsForeignKeyViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		// PostgreSQL
		{
			name: "postgres fk violation",
			err:  errors.New(`pq: insert or update on table "orders" violates foreign key constraint "orders_user_id_fkey"`),
			want: true,
		},
		// MySQL insert/update
		{
			name: "mysql fk insert fails",
			err:  errors.New("Error 1452: Cannot add or update a child row: a foreign key constraint fails"),
			want: true,
		},
		// MySQL delete
		{
			name: "mysql fk delete fails error 1451",
			err:  errors.New("Error 1451: Cannot delete or update a parent row: a foreign key constraint fails"),
			want: true,
		},
		{
			name: "mysql error code 1452",
			err:  errors.New("Error 1452 (23000): Cannot add or update a child row"),
			want: true,
		},
		// SQLite
		{
			name: "sqlite fk constraint",
			err:  errors.New("FOREIGN KEY constraint failed"),
			want: true,
		},
		// Unrelated errors
		{
			name: "unrelated error",
			err:  errors.New("syntax error near 'FROM'"),
			want: false,
		},
		{
			name: "unique violation is not fk",
			err:  errors.New("UNIQUE constraint failed: users.email"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsForeignKeyViolation(tc.err)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ============================================================================
// IsNotNullViolation tests (Issue #15)
// ============================================================================

func TestIsNotNullViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		// PostgreSQL
		{
			name: "postgres not null",
			err:  errors.New(`pq: null value in column "email" violates not-null constraint`),
			want: true,
		},
		// MySQL
		{
			name: "mysql cannot be null",
			err:  errors.New("Error 1048: Column 'email' cannot be null"),
			want: true,
		},
		{
			name: "mysql error code 1048",
			err:  errors.New("Error 1048 (23000): Column 'name' cannot be null"),
			want: true,
		},
		// SQLite
		{
			name: "sqlite not null constraint",
			err:  errors.New("NOT NULL constraint failed: users.email"),
			want: true,
		},
		// Unrelated errors
		{
			name: "unrelated error",
			err:  errors.New("table not found"),
			want: false,
		},
		{
			name: "check violation is not not-null",
			err:  errors.New("CHECK constraint failed: age_positive"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsNotNullViolation(tc.err)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ============================================================================
// IsCheckViolation tests (Issue #15)
// ============================================================================

func TestIsCheckViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		// PostgreSQL
		{
			name: "postgres check violation",
			err:  errors.New(`pq: new row for relation "products" violates check constraint "price_positive"`),
			want: true,
		},
		// MySQL 8.0.16+
		{
			name: "mysql check constraint",
			err:  errors.New("Error 3819: Check constraint 'price_positive' is violated"),
			want: true,
		},
		{
			name: "mysql error code 3819",
			err:  errors.New("Error 3819 (HY000): Check constraint 'chk_age' is violated"),
			want: true,
		},
		// SQLite
		{
			name: "sqlite check constraint",
			err:  errors.New("CHECK constraint failed: age_positive"),
			want: true,
		},
		// Unrelated errors
		{
			name: "unrelated error",
			err:  errors.New("no such table: users"),
			want: false,
		},
		{
			name: "unique violation is not check",
			err:  errors.New("UNIQUE constraint failed: users.email"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsCheckViolation(tc.err)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ============================================================================
// WrapError tests (existing functionality, regression guard)
// ============================================================================

func TestWrapError_NilInput(t *testing.T) {
	if WrapError(nil, "context") != nil {
		t.Error("expected nil for nil input")
	}
}

func TestWrapError_WrapsMessage(t *testing.T) {
	base := errors.New("base error")
	wrapped := WrapError(base, "operation failed")
	if wrapped.Error() != "operation failed: base error" {
		t.Errorf("got %q, want %q", wrapped.Error(), "operation failed: base error")
	}
	if !errors.Is(wrapped, base) {
		t.Error("wrapped error must satisfy errors.Is(wrapped, base)")
	}
}
