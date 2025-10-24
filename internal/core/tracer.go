package core

import (
	"context"
	"time"
)

// Tracer defines the interface for distributed tracing integration.
// Implementations can provide OpenTelemetry, Jaeger, or custom tracing.
type Tracer interface {
	Start(ctx context.Context, name string) (context.Context, Span)
	Record(ctx context.Context, duration time.Duration, err error)
}

// Span represents an active tracing span that must be ended when complete.
type Span interface {
	End()
}

// NoOpTracer is a zero-cost tracer that performs no operations.
// It is used by default when tracing is not needed.
type NoOpTracer struct{}

type noOpSpan struct{}

// NewNoOpTracer creates a tracer that performs no operations and has zero allocations.
func NewNoOpTracer() Tracer {
	return &NoOpTracer{}
}

// Start returns the context unchanged with a no-op span.
func (t *NoOpTracer) Start(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, &noOpSpan{}
}

// Record does nothing in the no-op implementation.
func (t *NoOpTracer) Record(_ context.Context, _ time.Duration, _ error) {}

// End does nothing in the no-op implementation.
func (s *noOpSpan) End() {}
