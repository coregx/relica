package util

import (
	"context"
	"testing"
	"time"
)

// TestIsCanceled tests context cancellation detection.
func TestIsCanceled(t *testing.T) {
	t.Run("active context is not canceled", func(t *testing.T) {
		ctx := context.Background()
		if IsCanceled(ctx) {
			t.Error("expected false")
		}
	})

	t.Run("canceled context is detected", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if !IsCanceled(ctx) {
			t.Error("expected true")
		}
	})

	t.Run("context canceled after delay", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		if IsCanceled(ctx) {
			t.Error("should not be canceled before cancel()")
		}
		cancel()
		if !IsCanceled(ctx) {
			t.Error("should be canceled after cancel()")
		}
	})

	t.Run("expired deadline context is canceled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		time.Sleep(5 * time.Millisecond)
		if !IsCanceled(ctx) {
			t.Error("expected true")
		}
	})

	t.Run("context with future deadline is not canceled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if IsCanceled(ctx) {
			t.Error("expected false")
		}
	})
}

// TestWithTimeout tests context creation with timeout.
func TestWithTimeout(t *testing.T) {
	t.Run("returns cancellable context", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.Background(), 10*time.Second)
		if ctx == nil {
			t.Fatal("expected non-nil")
		}
		if cancel == nil {
			t.Fatal("expected non-nil")
		}
		defer cancel()

		if IsCanceled(ctx) {
			t.Error("expected false")
		}
	})

	t.Run("context expires after timeout", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		time.Sleep(5 * time.Millisecond)
		if !IsCanceled(ctx) {
			t.Error("expected true")
		}
	})

	t.Run("cancel function stops context before timeout", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.Background(), 10*time.Second)
		if IsCanceled(ctx) {
			t.Error("expected false")
		}
		cancel()
		if !IsCanceled(ctx) {
			t.Error("expected true")
		}
	})

	t.Run("deadline is set correctly", func(t *testing.T) {
		timeout := 5 * time.Second
		before := time.Now()
		ctx, cancel := WithTimeout(context.Background(), timeout)
		defer cancel()

		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected true")
		}
		if !deadline.After(before) {
			t.Error("expected true")
		}
		if !deadline.Before(before.Add(timeout + 100*time.Millisecond)) {
			t.Error("expected true")
		}
	})

	t.Run("inherits parent context values", func(t *testing.T) {
		type key string
		parent := context.WithValue(context.Background(), key("k"), "v")
		ctx, cancel := WithTimeout(parent, 10*time.Second)
		defer cancel()

		if got := ctx.Value(key("k")); got != "v" {
			t.Errorf("got %v, want %v", got, "v")
		}
	})
}
