package util

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
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
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			time.Sleep(time.Second)
			synctest.Wait() // let the timer's cancel goroutine finish
			if !IsCanceled(ctx) {
				t.Error("expected true")
			}
		})
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

	// The deadline is enforced by time.AfterFunc, which cancels the context from
	// its own goroutine. On a real clock a test that sleeps past the deadline can
	// wake before that goroutine runs (both timers fire in one scheduler pass when
	// the process is descheduled), so the old Sleep-based version was flaky.
	// synctest uses a fake clock, and synctest.Wait returns only after the cancel
	// goroutine is done, which makes the check exact at the boundary.
	t.Run("context expires exactly at the timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const timeout = 5 * time.Second
			ctx, cancel := WithTimeout(context.Background(), timeout)
			defer cancel()

			time.Sleep(timeout - time.Nanosecond)
			synctest.Wait()
			if IsCanceled(ctx) {
				t.Fatalf("canceled before the timeout: %v", ctx.Err())
			}

			time.Sleep(time.Nanosecond)
			synctest.Wait()
			if !IsCanceled(ctx) {
				t.Fatal("not canceled at the timeout")
			}
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Errorf("ctx.Err() = %v, want %v", ctx.Err(), context.DeadlineExceeded)
			}
		})
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
		synctest.Test(t, func(t *testing.T) {
			const timeout = 5 * time.Second
			start := time.Now() // the fake clock does not move until we block
			ctx, cancel := WithTimeout(context.Background(), timeout)
			defer cancel()

			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("expected a deadline")
			}
			if want := start.Add(timeout); !deadline.Equal(want) {
				t.Errorf("deadline = %v, want %v", deadline, want)
			}
		})
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
