// Package logger provides logging abstractions for Relica.
// It supports standard library log/slog and allows custom logger implementations.
package logger

import "log/slog"

// Logger defines the logging interface for Relica.
// Implementations should handle structured logging with key-value pairs.
type Logger interface {
	// Debug logs debug-level messages with optional key-value pairs
	Debug(msg string, args ...any)
	// Info logs informational messages with optional key-value pairs
	Info(msg string, args ...any)
	// Warn logs warning messages with optional key-value pairs
	Warn(msg string, args ...any)
	// Error logs error messages with optional key-value pairs
	Error(msg string, args ...any)
}

// NoopLogger is a logger that does nothing (zero overhead when logging is disabled).
// This is the default logger used when no logger is configured.
type NoopLogger struct{}

// Debug does nothing.
func (n *NoopLogger) Debug(_ string, _ ...any) {}

// Info does nothing.
func (n *NoopLogger) Info(_ string, _ ...any) {}

// Warn does nothing.
func (n *NoopLogger) Warn(_ string, _ ...any) {}

// Error does nothing.
func (n *NoopLogger) Error(_ string, _ ...any) {}

// SlogAdapter wraps log/slog.Logger to implement the Logger interface.
// This allows seamless integration with the standard library's structured logging.
type SlogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter creates a new logger adapter wrapping an slog.Logger.
// The provided logger must not be nil.
func NewSlogAdapter(logger *slog.Logger) *SlogAdapter {
	return &SlogAdapter{logger: logger}
}

// Debug logs a debug-level message with structured key-value pairs.
func (a *SlogAdapter) Debug(msg string, args ...any) {
	a.logger.Debug(msg, args...)
}

// Info logs an info-level message with structured key-value pairs.
func (a *SlogAdapter) Info(msg string, args ...any) {
	a.logger.Info(msg, args...)
}

// Warn logs a warning-level message with structured key-value pairs.
func (a *SlogAdapter) Warn(msg string, args ...any) {
	a.logger.Warn(msg, args...)
}

// Error logs an error-level message with structured key-value pairs.
func (a *SlogAdapter) Error(msg string, args ...any) {
	a.logger.Error(msg, args...)
}
