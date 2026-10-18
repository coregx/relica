package core

import (
	"database/sql"
	"errors"
	"fmt"
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

// isErrNotFound checks if an error is ErrNotFound.
func isErrNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

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
// Each helper classifies err by the driver error code found in its error tree
// (see errclass.go for the codes and drivers), and by message text only when
// the tree holds no code. The public documentation is on the wrappers in
// package relica.

// IsUniqueViolation reports whether err is a unique or primary key violation.
func IsUniqueViolation(err error) bool { return classify(err, &uniqueViolation) }

// IsForeignKeyViolation reports whether err is a foreign key violation.
func IsForeignKeyViolation(err error) bool { return classify(err, &foreignKeyViolation) }

// IsNotNullViolation reports whether err is a NOT NULL violation.
func IsNotNullViolation(err error) bool { return classify(err, &notNullViolation) }

// IsCheckViolation reports whether err is a CHECK constraint violation.
func IsCheckViolation(err error) bool { return classify(err, &checkViolation) }

// IsDeadlock reports whether err says the transaction was chosen as a
// deadlock victim and rolled back.
func IsDeadlock(err error) bool { return classify(err, &deadlock) }

// IsSerializationFailure reports whether err is a serialization failure of a
// transaction that must be retried from the start.
func IsSerializationFailure(err error) bool { return classify(err, &serializationFailure) }

// IsLockTimeout reports whether err says a lock could not be acquired in time
// (lock timeout, NOWAIT, or SQLite busy timeout).
func IsLockTimeout(err error) bool { return classify(err, &lockTimeout) }
