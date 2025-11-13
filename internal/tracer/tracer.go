// Package tracer provides distributed tracing abstractions for Relica.
// It supports OpenTelemetry and allows custom tracer implementations.
package tracer

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Tracer defines the tracing interface for Relica.
// Implementations can provide OpenTelemetry, Jaeger, or custom tracing.
type Tracer interface {
	// StartSpan starts a new tracing span with the given name
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// Span represents a tracing span that captures the execution of an operation.
type Span interface {
	// SetAttributes sets key-value attributes on the span
	SetAttributes(attrs ...attribute.KeyValue)
	// RecordError records an error that occurred during the span
	RecordError(err error)
	// SetStatus sets the status code and description of the span
	SetStatus(code codes.Code, description string)
	// End marks the span as complete
	End()
}

// NoopTracer is a tracer that does nothing (zero overhead when tracing is disabled).
// This is the default tracer used when no tracing is configured.
type NoopTracer struct{}

// StartSpan returns the context unchanged with a no-op span.
func (n *NoopTracer) StartSpan(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, &NoopSpan{}
}

// NoopSpan is a span that does nothing.
type NoopSpan struct{}

// SetAttributes does nothing.
func (n *NoopSpan) SetAttributes(_ ...attribute.KeyValue) {}

// RecordError does nothing.
func (n *NoopSpan) RecordError(_ error) {}

// SetStatus does nothing.
func (n *NoopSpan) SetStatus(_ codes.Code, _ string) {}

// End does nothing.
func (n *NoopSpan) End() {}

// OtelTracer wraps an OpenTelemetry tracer to implement the Tracer interface.
// This allows seamless integration with OpenTelemetry-based observability systems.
type OtelTracer struct {
	tracer trace.Tracer
}

// NewOtelTracer creates a new OpenTelemetry tracer adapter.
// The provided tracer must not be nil.
func NewOtelTracer(tracer trace.Tracer) *OtelTracer {
	return &OtelTracer{tracer: tracer}
}

// StartSpan starts a new OpenTelemetry span.
func (t *OtelTracer) StartSpan(ctx context.Context, name string) (context.Context, Span) {
	ctx, span := t.tracer.Start(ctx, name)
	return ctx, &OtelSpan{span: span}
}

// OtelSpan wraps an OpenTelemetry span.
type OtelSpan struct {
	span trace.Span
}

// SetAttributes sets OpenTelemetry attributes on the span.
func (s *OtelSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.span.SetAttributes(attrs...)
}

// RecordError records an error on the OpenTelemetry span.
func (s *OtelSpan) RecordError(err error) {
	s.span.RecordError(err)
}

// SetStatus sets the status of the OpenTelemetry span.
func (s *OtelSpan) SetStatus(code codes.Code, description string) {
	s.span.SetStatus(code, description)
}

// End completes the OpenTelemetry span.
func (s *OtelSpan) End() {
	s.span.End()
}

// QueryMetadata contains information about a database query for tracing purposes.
// It follows OpenTelemetry database semantic conventions.
type QueryMetadata struct {
	// SQL is the SQL query string
	SQL string
	// Args are the query parameters
	Args []interface{}
	// Duration is how long the query took to execute
	Duration time.Duration
	// RowsAffected is the number of rows affected (for INSERT/UPDATE/DELETE)
	RowsAffected int64
	// Error is any error that occurred during query execution
	Error error
	// Database is the database system name (postgres, mysql, sqlite)
	Database string
	// Operation is the SQL operation type (SELECT, INSERT, UPDATE, DELETE)
	Operation string
	// Table is the primary table being queried (optional)
	Table string
}

// AddQueryAttributes adds database semantic convention attributes to a span.
// This follows OpenTelemetry semantic conventions for database operations.
// See: https://opentelemetry.io/docs/specs/semconv/database/
func AddQueryAttributes(span Span, meta *QueryMetadata) {
	attrs := []attribute.KeyValue{
		attribute.String("db.system", meta.Database),
		attribute.String("db.statement", meta.SQL),
		attribute.String("db.operation", meta.Operation),
		attribute.Float64("db.duration_ms", float64(meta.Duration.Microseconds())/1000.0),
	}

	if meta.Table != "" {
		attrs = append(attrs, attribute.String("db.table", meta.Table))
	}

	if meta.RowsAffected > 0 {
		attrs = append(attrs, attribute.Int64("db.rows_affected", meta.RowsAffected))
	}

	span.SetAttributes(attrs...)

	if meta.Error != nil {
		span.RecordError(meta.Error)
		span.SetStatus(codes.Error, meta.Error.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

// DetectOperation attempts to detect the SQL operation type from the query string.
// Returns one of: SELECT, INSERT, UPDATE, DELETE, or UNKNOWN.
func DetectOperation(sql string) string {
	sql = strings.TrimSpace(strings.ToUpper(sql))
	if strings.HasPrefix(sql, "SELECT") || strings.HasPrefix(sql, "WITH") {
		return "SELECT"
	}
	if strings.HasPrefix(sql, "INSERT") {
		return "INSERT"
	}
	if strings.HasPrefix(sql, "UPDATE") {
		return "UPDATE"
	}
	if strings.HasPrefix(sql, "DELETE") {
		return "DELETE"
	}
	return "UNKNOWN"
}
