package core

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Predefined errors returned by Relica database operations.
var (
	// ErrNoRows is returned when a query that expects rows returns no results.
	ErrNoRows = errors.New("no rows in result set")
	// ErrTxDone is returned when operating on an already committed or rolled back transaction.
	ErrTxDone = errors.New("transaction has already been committed or rolled back")
	// ErrInvalidModelType is returned when an invalid model type is provided.
	ErrInvalidModelType = errors.New("invalid model type")
	// ErrUnsupportedDialect is returned when an unsupported database dialect is specified.
	ErrUnsupportedDialect = errors.New("unsupported database dialect")
	// ErrContextCanceled is returned when an operation is canceled by context.
	ErrContextCanceled = errors.New("operation canceled by context")

	// ErrNotFound is returned by One() when no rows match the query.
	// It wraps sql.ErrNoRows so both errors.Is(err, ErrNotFound) and
	// errors.Is(err, sql.ErrNoRows) return true on the wrapped error.
	//
	// Example:
	//
	//	var user User
	//	err := db.Select().From("users").Where(relica.Eq("id", 999)).One(&user)
	//	if errors.Is(err, relica.ErrNotFound) {
	//	    // handle not found
	//	}
	ErrNotFound = errors.New("relica: record not found")
)

// wrapErrNotFound returns an error that satisfies both:
//   - errors.Is(err, ErrNotFound) == true
//   - errors.Is(err, sql.ErrNoRows) == true
func wrapErrNotFound() error {
	return fmt.Errorf("%w: %w", ErrNotFound, sql.ErrNoRows)
}

// WrapError wraps an error with additional context message.
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return &wrappedError{
		msg: message,
		err: err,
	}
}

type wrappedError struct {
	msg string
	err error
}

func (e *wrappedError) Error() string {
	return e.msg + ": " + e.err.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.err
}

// ============================================================================
// Cross-database error classification helpers
// ============================================================================
//
// These functions classify database errors by inspecting error message strings.
// This approach is driver-agnostic and does not require importing any driver packages.
//
// Error messages are matched against known patterns from:
//   - PostgreSQL (lib/pq, pgx)
//   - MySQL / MariaDB (go-sql-driver/mysql)
//   - SQLite (mattn/go-sqlite3, modernc.org/sqlite)

// IsUniqueViolation reports whether err represents a unique constraint violation.
// Returns false for nil errors.
//
// Matches errors from:
//   - PostgreSQL: "duplicate key value violates unique constraint"
//   - MySQL: "Duplicate entry" or "Error 1062"
//   - SQLite: "UNIQUE constraint failed"
//
// Example:
//
//	_, err := db.Model(&user).Insert()
//	if relica.IsUniqueViolation(err) {
//	    // handle duplicate key
//	}
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value violates unique constraint") ||
		strings.Contains(msg, "Duplicate entry") ||
		strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "Error 1062")
}

// IsForeignKeyViolation reports whether err represents a foreign key constraint violation.
// Returns false for nil errors.
//
// Matches errors from:
//   - PostgreSQL: "violates foreign key constraint"
//   - MySQL: "a foreign key constraint fails" or "Error 1451"/"Error 1452"
//   - SQLite: "FOREIGN KEY constraint failed"
//
// Example:
//
//	_, err := db.Model(&order).Insert()
//	if relica.IsForeignKeyViolation(err) {
//	    // handle missing referenced row
//	}
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "violates foreign key constraint") ||
		strings.Contains(msg, "a foreign key constraint fails") ||
		strings.Contains(msg, "FOREIGN KEY constraint failed") ||
		strings.Contains(msg, "Error 1451") ||
		strings.Contains(msg, "Error 1452")
}

// IsNotNullViolation reports whether err represents a NOT NULL constraint violation.
// Returns false for nil errors.
//
// Matches errors from:
//   - PostgreSQL: "violates not-null constraint"
//   - MySQL: "cannot be null" or "Error 1048"
//   - SQLite: "NOT NULL constraint failed"
//
// Example:
//
//	_, err := db.Model(&user).Insert()
//	if relica.IsNotNullViolation(err) {
//	    // handle missing required field
//	}
func IsNotNullViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "violates not-null constraint") ||
		strings.Contains(msg, "cannot be null") ||
		strings.Contains(msg, "NOT NULL constraint failed") ||
		strings.Contains(msg, "Error 1048")
}

// IsCheckViolation reports whether err represents a CHECK constraint violation.
// Returns false for nil errors.
//
// Matches errors from:
//   - PostgreSQL: "violates check constraint"
//   - MySQL: "Check constraint" or "Error 3819" (MySQL 8.0.16+)
//   - SQLite: "CHECK constraint failed"
//
// Example:
//
//	_, err := db.Model(&product).Insert()
//	if relica.IsCheckViolation(err) {
//	    // handle check constraint failure
//	}
func IsCheckViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "violates check constraint") ||
		strings.Contains(msg, "Check constraint") ||
		strings.Contains(msg, "CHECK constraint failed") ||
		strings.Contains(msg, "Error 3819")
}
