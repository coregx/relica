package util

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsCanceled tests context cancellation detection.
func TestIsCanceled(t *testing.T) {
	t.Run("active context is not canceled", func(t *testing.T) {
		ctx := context.Background()
		assert.False(t, IsCanceled(ctx))
	})

	t.Run("canceled context is detected", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.True(t, IsCanceled(ctx))
	})

	t.Run("context canceled after delay", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		assert.False(t, IsCanceled(ctx), "should not be canceled before cancel()")
		cancel()
		assert.True(t, IsCanceled(ctx), "should be canceled after cancel()")
	})

	t.Run("expired deadline context is canceled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		time.Sleep(5 * time.Millisecond)
		assert.True(t, IsCanceled(ctx))
	})

	t.Run("context with future deadline is not canceled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		assert.False(t, IsCanceled(ctx))
	})
}

// TestWithTimeout tests context creation with timeout.
func TestWithTimeout(t *testing.T) {
	t.Run("returns cancellable context", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.Background(), 10*time.Second)
		require.NotNil(t, ctx)
		require.NotNil(t, cancel)
		defer cancel()

		assert.False(t, IsCanceled(ctx))
	})

	t.Run("context expires after timeout", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		time.Sleep(5 * time.Millisecond)
		assert.True(t, IsCanceled(ctx))
	})

	t.Run("cancel function stops context before timeout", func(t *testing.T) {
		ctx, cancel := WithTimeout(context.Background(), 10*time.Second)
		assert.False(t, IsCanceled(ctx))
		cancel()
		assert.True(t, IsCanceled(ctx))
	})

	t.Run("deadline is set correctly", func(t *testing.T) {
		timeout := 5 * time.Second
		before := time.Now()
		ctx, cancel := WithTimeout(context.Background(), timeout)
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok, "deadline should be set")
		assert.True(t, deadline.After(before))
		assert.True(t, deadline.Before(before.Add(timeout+100*time.Millisecond)))
	})

	t.Run("inherits parent context values", func(t *testing.T) {
		type key string
		parent := context.WithValue(context.Background(), key("k"), "v")
		ctx, cancel := WithTimeout(parent, 10*time.Second)
		defer cancel()

		assert.Equal(t, "v", ctx.Value(key("k")))
	})
}
