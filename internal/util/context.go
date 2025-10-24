// Package util provides utility functions for context handling, string sanitization,
// and reflection helpers used throughout the Relica library.
package util

import (
	"context"
	"time"
)

// IsCanceled checks if the context has been canceled.
func IsCanceled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// WithTimeout creates a context with the specified timeout duration.
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
