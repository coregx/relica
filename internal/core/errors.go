package core

import "errors"

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
)

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
