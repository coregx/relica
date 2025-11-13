package tracer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNoopTracer(t *testing.T) {
	tracer := &NoopTracer{}
	ctx := context.Background()

	// Should not panic
	_, span := tracer.StartSpan(ctx, "test.operation")
	assert.NotNil(t, span)

	span.SetAttributes(attribute.String("key", "value"))
	span.RecordError(errors.New("test error"))
	span.SetStatus(codes.Error, "error")
	span.End()
}

func TestNoopSpan(t *testing.T) {
	span := &NoopSpan{}

	// Should not panic
	span.SetAttributes(
		attribute.String("string", "value"),
		attribute.Int("int", 42),
		attribute.Bool("bool", true),
	)
	span.RecordError(errors.New("test error"))
	span.SetStatus(codes.Error, "error")
	span.End()
}

func TestOtelTracer(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)

	otelTracer := otel.Tracer("test")
	tracer := NewOtelTracer(otelTracer)

	ctx := context.Background()
	ctx, span := tracer.StartSpan(ctx, "test.operation")
	assert.NotNil(t, span)

	span.SetAttributes(attribute.String("key", "value"))
	span.End()

	// Force flush
	_ = tp.ForceFlush(ctx)

	// Verify span was recorded
	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "test.operation", spans[0].Name)
	assert.Equal(t, "value", spans[0].Attributes[0].Value.AsString())
}

func TestOtelSpan_SetAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	otelTracer := otel.Tracer("test")
	tracer := NewOtelTracer(otelTracer)

	ctx := context.Background()
	ctx, span := tracer.StartSpan(ctx, "test.attributes")

	span.SetAttributes(
		attribute.String("db.system", "postgres"),
		attribute.String("db.operation", "SELECT"),
		attribute.Int64("db.rows_affected", 42),
		attribute.Float64("db.duration_ms", 15.5),
	)
	span.End()

	_ = tp.ForceFlush(ctx)

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	attrs := spans[0].Attributes

	// Find attributes by key
	attrMap := make(map[string]interface{})
	for _, attr := range attrs {
		attrMap[string(attr.Key)] = attr.Value.AsInterface()
	}

	assert.Equal(t, "postgres", attrMap["db.system"])
	assert.Equal(t, "SELECT", attrMap["db.operation"])
	assert.Equal(t, int64(42), attrMap["db.rows_affected"])
	assert.Equal(t, 15.5, attrMap["db.duration_ms"])
}

func TestOtelSpan_RecordError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	otelTracer := otel.Tracer("test")
	tracer := NewOtelTracer(otelTracer)

	ctx := context.Background()
	ctx, span := tracer.StartSpan(ctx, "test.error")

	testErr := errors.New("database connection failed")
	span.RecordError(testErr)
	span.SetStatus(codes.Error, testErr.Error())
	span.End()

	_ = tp.ForceFlush(ctx)

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Len(t, spans[0].Events, 1)
	assert.Equal(t, "exception", spans[0].Events[0].Name)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
}

func TestAddQueryAttributes_Success(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	otelTracer := otel.Tracer("test")
	tracer := NewOtelTracer(otelTracer)

	ctx := context.Background()
	ctx, span := tracer.StartSpan(ctx, "query.select")

	meta := &QueryMetadata{
		SQL:          "SELECT * FROM users WHERE id = ?",
		Args:         []interface{}{123},
		Duration:     15 * time.Millisecond,
		RowsAffected: 1,
		Error:        nil,
		Database:     "postgres",
		Operation:    "SELECT",
		Table:        "users",
	}

	AddQueryAttributes(span, meta)
	span.End()

	_ = tp.ForceFlush(ctx)

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)

	attrMap := make(map[string]interface{})
	for _, attr := range spans[0].Attributes {
		attrMap[string(attr.Key)] = attr.Value.AsInterface()
	}

	assert.Equal(t, "postgres", attrMap["db.system"])
	assert.Equal(t, "SELECT * FROM users WHERE id = ?", attrMap["db.statement"])
	assert.Equal(t, "SELECT", attrMap["db.operation"])
	assert.Equal(t, "users", attrMap["db.table"])
	assert.Equal(t, int64(1), attrMap["db.rows_affected"])
	assert.InDelta(t, 15.0, attrMap["db.duration_ms"], 0.1)
	assert.Equal(t, codes.Ok, spans[0].Status.Code)
}

func TestAddQueryAttributes_WithError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	otelTracer := otel.Tracer("test")
	tracer := NewOtelTracer(otelTracer)

	ctx := context.Background()
	ctx, span := tracer.StartSpan(ctx, "query.error")

	testErr := errors.New("syntax error")
	meta := &QueryMetadata{
		SQL:       "SELECT * FORM users", // Intentional typo
		Args:      []interface{}{},
		Duration:  5 * time.Millisecond,
		Error:     testErr,
		Database:  "postgres",
		Operation: "SELECT",
	}

	AddQueryAttributes(span, meta)
	span.End()

	_ = tp.ForceFlush(ctx)

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "syntax error", spans[0].Status.Description)
	assert.Len(t, spans[0].Events, 1) // Error event
}

func TestDetectOperation(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "SELECT query",
			sql:  "SELECT * FROM users WHERE id = ?",
			want: "SELECT",
		},
		{
			name: "SELECT with whitespace",
			sql:  "  \n  SELECT name FROM users",
			want: "SELECT",
		},
		{
			name: "WITH CTE",
			sql:  "WITH stats AS (SELECT ...) SELECT * FROM stats",
			want: "SELECT",
		},
		{
			name: "INSERT query",
			sql:  "INSERT INTO users (name) VALUES (?)",
			want: "INSERT",
		},
		{
			name: "UPDATE query",
			sql:  "UPDATE users SET name = ? WHERE id = ?",
			want: "UPDATE",
		},
		{
			name: "DELETE query",
			sql:  "DELETE FROM users WHERE id = ?",
			want: "DELETE",
		},
		{
			name: "Unknown query",
			sql:  "EXPLAIN SELECT * FROM users",
			want: "UNKNOWN",
		},
		{
			name: "Lowercase SELECT",
			sql:  "select * from users",
			want: "SELECT",
		},
		{
			name: "Mixed case INSERT",
			sql:  "InSeRt INTO users VALUES (?)",
			want: "INSERT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectOperation(tt.sql)
			assert.Equal(t, tt.want, got)
		})
	}
}

func BenchmarkNoopTracer(b *testing.B) {
	tracer := &NoopTracer{}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := tracer.StartSpan(ctx, "test.operation")
		span.SetAttributes(attribute.String("key", "value"))
		span.End()
	}
}

func BenchmarkOtelTracer(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)

	otelTracer := otel.Tracer("benchmark")
	tracer := NewOtelTracer(otelTracer)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := tracer.StartSpan(ctx, "test.operation")
		span.SetAttributes(attribute.String("key", "value"))
		span.End()
	}
}

func BenchmarkAddQueryAttributes(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)

	otelTracer := otel.Tracer("benchmark")
	tracer := NewOtelTracer(otelTracer)
	ctx := context.Background()

	meta := &QueryMetadata{
		SQL:          "SELECT * FROM users WHERE id = ?",
		Args:         []interface{}{123},
		Duration:     15 * time.Millisecond,
		RowsAffected: 1,
		Database:     "postgres",
		Operation:    "SELECT",
		Table:        "users",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := tracer.StartSpan(ctx, "query")
		AddQueryAttributes(span, meta)
		span.End()
	}
}
