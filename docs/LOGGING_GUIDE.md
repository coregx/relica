# Logging and Tracing Guide for Relica

This guide covers SQL query logging and distributed tracing capabilities in Relica.

## Table of Contents

- [Quick Start](#quick-start)
- [Logging with slog](#logging-with-slog)
- [OpenTelemetry Tracing](#opentelemetry-tracing)
- [Sensitive Data Masking](#sensitive-data-masking)
- [Best Practices](#best-practices)
- [Performance](#performance)
- [Examples](#examples)

---

## Quick Start

### Basic Logging (slog)

```go
package main

import (
    "log/slog"
    "os"

    "github.com/coregx/relica"
)

func main() {
    // Create slog logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    // Open database with logging
    db, err := relica.Open("postgres", dsn,
        relica.WithLogger(relica.NewSlogAdapter(logger)))
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // All queries are now logged
    var users []User
    _ = db.Select("*").From("users").All(&users)
    // Output: {"time":"...","level":"INFO","msg":"query executed",
    //          "sql":"SELECT * FROM users","duration_ms":15,"rows":10}
}
```

### Basic Tracing (OpenTelemetry)

```go
package main

import (
    "context"

    "github.com/coregx/relica"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
    // Setup OpenTelemetry
    exporter, _ := jaeger.New(jaeger.WithCollectorEndpoint())
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
    )
    otel.SetTracerProvider(tp)
    defer tp.Shutdown(context.Background())

    // Create tracer
    tracer := otel.Tracer("relica")

    // Open database with tracing
    db, err := relica.Open("postgres", dsn,
        relica.WithTracer(relica.NewOtelTracer(tracer)))
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // All queries are now traced
    ctx := context.Background()
    var user User
    _ = db.WithContext(ctx).
        Select("*").
        From("users").
        Where("id = ?", 123).
        One(&user)
    // Span created: relica.query.one with db.* attributes
}
```

---

## Logging with slog

### Logger Levels

Relica logs at different levels depending on the outcome:

- **INFO**: Successful query execution
- **WARN**: Query returned no rows (sql.ErrNoRows)
- **ERROR**: Query preparation failed, execution failed, or scanning failed

### Log Fields

Standard fields logged for each query:

| Field          | Type    | Description                         |
|---------------|---------|-------------------------------------|
| `msg`          | string  | Log message                         |
| `sql`          | string  | SQL query string                    |
| `params`       | string  | Formatted parameters (sanitized)    |
| `duration_ms`  | int64   | Query execution time in milliseconds |
| `database`     | string  | Database driver name (postgres/mysql/sqlite) |
| `rows_affected`| int64   | Number of rows affected (INSERT/UPDATE/DELETE) |
| `rows`         | int     | Number of rows returned (SELECT)    |
| `error`        | string  | Error message (if failed)           |

### Custom Log Handlers

```go
// Text handler (human-readable)
textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug, // Include debug logs
})
logger := slog.New(textHandler)

// JSON handler (structured logging for production)
jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
logger := slog.New(jsonHandler)

// Custom handler (e.g., send to external service)
type MyCustomHandler struct {
    // ... implementation
}
logger := slog.New(MyCustomHandler{})

db, _ := relica.Open("postgres", dsn,
    relica.WithLogger(relica.NewSlogAdapter(logger)))
```

### Filtering Logs

```go
// Only log slow queries (>=100ms)
type SlowQueryHandler struct {
    handler slog.Handler
}

func (h *SlowQueryHandler) Handle(ctx context.Context, r slog.Record) error {
    // Extract duration_ms from record
    var durationMs int64
    r.Attrs(func(a slog.Attr) bool {
        if a.Key == "duration_ms" {
            durationMs = a.Value.Int64()
            return false // Stop iteration
        }
        return true
    })

    // Only log if slow
    if durationMs >= 100 {
        return h.handler.Handle(ctx, r)
    }
    return nil
}
```

---

## OpenTelemetry Tracing

### Span Names

Relica creates spans with these names:

- `relica.query.execute` - INSERT/UPDATE/DELETE operations
- `relica.query.one` - SELECT returning single row
- `relica.query.all` - SELECT returning multiple rows

### Span Attributes (OpenTelemetry Semantic Conventions)

| Attribute          | Type    | Example                         |
|-------------------|---------|----------------------------------|
| `db.system`        | string  | `"postgres"`, `"mysql"`, `"sqlite"` |
| `db.statement`     | string  | `"SELECT * FROM users WHERE id = $1"` |
| `db.operation`     | string  | `"SELECT"`, `"INSERT"`, `"UPDATE"`, `"DELETE"` |
| `db.table`         | string  | `"users"` (optional)             |
| `db.duration_ms`   | float64 | `15.5`                           |
| `db.rows_affected` | int64   | `1` (for INSERT/UPDATE/DELETE)   |

### Integrating with HTTP Tracing

```go
func getUserHandler(w http.ResponseWriter, r *http.Request) {
    // Extract trace context from HTTP request
    ctx := otel.GetTextMapPropagator().Extract(r.Context(),
        propagation.HeaderCarrier(r.Header))

    // Start HTTP span
    ctx, span := tracer.Start(ctx, "getUserHandler")
    defer span.End()

    // Database query inherits trace context
    var user User
    err := db.WithContext(ctx).
        Select("*").
        From("users").
        Where("id = ?", r.URL.Query().Get("id")).
        One(&user)

    // Query span is child of HTTP span in trace
    if err != nil {
        span.RecordError(err)
        http.Error(w, err.Error(), 500)
        return
    }

    json.NewEncoder(w).Encode(user)
}
```

### Multiple Tracing Backends

```go
// Jaeger
import "go.opentelemetry.io/otel/exporters/jaeger"

exporter, _ := jaeger.New(jaeger.WithCollectorEndpoint(
    jaeger.WithEndpoint("http://localhost:14268/api/traces"),
))

// Zipkin
import "go.opentelemetry.io/otel/exporters/zipkin"

exporter, _ := zipkin.New("http://localhost:9411/api/v2/spans")

// OTLP (OpenTelemetry Protocol)
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

exporter, _ := otlptracehttp.New(context.Background(),
    otlptracehttp.WithEndpoint("localhost:4318"),
)

// Console (debugging)
import "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"

exporter, _ := stdouttrace.New(
    stdouttrace.WithPrettyPrint(),
)

// Use exporter with TracerProvider
tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter),
)
```

---

## Sensitive Data Masking

### Default Sensitive Fields

Relica automatically masks these field names in query logs:

- Passwords: `password`, `passwd`, `pwd`
- Tokens: `token`, `api_key`, `apikey`, `api_token`
- Secrets: `secret`, `auth`, `authorization`
- Payment: `credit_card`, `card_number`, `cvv`, `cvc`
- Personal: `ssn`, `social_security`
- Keys: `private_key`, `priv_key`

### Custom Sensitive Fields

```go
db, err := relica.Open("postgres", dsn,
    relica.WithLogger(logger),
    relica.WithSensitiveFields([]string{
        "internal_api_key",
        "encryption_key",
        "oauth_secret",
    }))
```

### Masking Behavior

```go
// SQL with sensitive field
sql := "UPDATE users SET password = ? WHERE id = ?"
params := []interface{}{"mySecretPassword", 123}

// Logged as:
// {"sql":"UPDATE users SET password = ? WHERE id = ?",
//  "params":"[***REDACTED***, ***REDACTED***]"}
```

**Note**: Masking uses pattern matching on SQL column names. If a sensitive field is detected in the SQL, all parameters are masked for that query to prevent correlation attacks.

---

## Best Practices

### 1. Use Structured Logging in Production

```go
// ✅ Good: JSON for machine parsing
jsonHandler := slog.NewJSONHandler(os.Stdout, nil)

// ❌ Avoid: Text for production (use for development only)
textHandler := slog.NewTextHandler(os.Stdout, nil)
```

### 2. Set Appropriate Log Levels

```go
// Development
&slog.HandlerOptions{Level: slog.LevelDebug}

// Production
&slog.HandlerOptions{Level: slog.LevelInfo}

// Critical systems
&slog.HandlerOptions{Level: slog.LevelWarn} // Only warnings/errors
```

### 3. Sample High-Volume Queries

```go
type SamplingHandler struct {
    handler slog.Handler
    rate    float64 // 0.0 - 1.0
}

func (h *SamplingHandler) Handle(ctx context.Context, r slog.Record) error {
    if rand.Float64() < h.rate {
        return h.handler.Handle(ctx, r)
    }
    return nil
}

// Only log 10% of queries
samplingHandler := &SamplingHandler{
    handler: jsonHandler,
    rate:    0.1,
}
logger := slog.New(samplingHandler)
```

### 4. Correlate Logs and Traces

```go
// Add trace ID to logs
type TraceIDHandler struct {
    handler slog.Handler
}

func (h *TraceIDHandler) Handle(ctx context.Context, r slog.Record) error {
    spanCtx := trace.SpanContextFromContext(ctx)
    if spanCtx.IsValid() {
        r = r.Clone()
        r.AddAttrs(slog.String("trace_id", spanCtx.TraceID().String()))
    }
    return h.handler.Handle(ctx, r)
}
```

### 5. Disable in Performance-Critical Paths

```go
// For ultra-high-throughput services
db, _ := relica.Open("postgres", dsn)
// No logger = NoopLogger (zero overhead)

// Enable only for debugging
if os.Getenv("DEBUG_SQL") == "true" {
    db, _ = relica.Open("postgres", dsn,
        relica.WithLogger(logger))
}
```

---

## Performance

### Overhead Measurements

| Configuration       | Overhead  | Notes                              |
|--------------------|-----------|-------------------------------------|
| No logging         | 0%        | NoopLogger (zero allocations)      |
| slog (text)        | <1%       | Minimal impact                      |
| slog (JSON)        | <2%       | Slightly slower due to marshaling   |
| OpenTelemetry      | <5%       | Depends on sampling rate            |
| Both (log + trace) | <7%       | Combined overhead                   |

### Benchmarks

```bash
go test -bench=. -benchmem ./internal/logger/...
go test -bench=. -benchmem ./internal/tracer/...
```

Results:
```
BenchmarkNoopLogger-8           1000000000  0.28 ns/op   0 B/op  0 allocs/op
BenchmarkSlogAdapter-8              500000  2500 ns/op  128 B/op  3 allocs/op
BenchmarkNoopTracer-8           100000000  10.5 ns/op    0 B/op  0 allocs/op
BenchmarkOtelTracer-8             5000000   300 ns/op   64 B/op  2 allocs/op
```

---

## Examples

### Example 1: Production Setup (JSON + Jaeger)

```go
func main() {
    // Logging
    logFile, _ := os.OpenFile("sql.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    // Tracing
    exporter, _ := jaeger.New(jaeger.WithCollectorEndpoint())
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)), // 10% sampling
    )
    otel.SetTracerProvider(tp)
    tracer := otel.Tracer("myapp")

    // Database
    db, _ := relica.Open("postgres", dsn,
        relica.WithLogger(relica.NewSlogAdapter(logger)),
        relica.WithTracer(relica.NewOtelTracer(tracer)),
        relica.WithSensitiveFields([]string{"api_key", "secret"}))

    defer db.Close()
    defer tp.Shutdown(context.Background())
    defer logFile.Close()
}
```

### Example 2: Development Setup (Console Output)

```go
func main() {
    // Human-readable logs
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))

    // Console tracing (pretty print)
    exporter, _ := stdouttrace.New(stdouttrace.WithPrettyPrint())
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(exporter), // Sync for debugging
    )
    otel.SetTracerProvider(tp)
    tracer := otel.Tracer("dev")

    db, _ := relica.Open("sqlite", ":memory:",
        relica.WithLogger(relica.NewSlogAdapter(logger)),
        relica.WithTracer(relica.NewOtelTracer(tracer)))

    defer db.Close()
    defer tp.Shutdown(context.Background())
}
```

### Example 3: Conditional Logging (Environment-Based)

```go
func openDB() (*relica.DB, error) {
    opts := []relica.Option{
        relica.WithMaxOpenConns(100),
    }

    // Enable logging only in dev/staging
    if env := os.Getenv("ENV"); env != "production" {
        logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
        opts = append(opts, relica.WithLogger(relica.NewSlogAdapter(logger)))
    }

    // Enable tracing in all environments
    if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
        exporter, _ := otlptracehttp.New(context.Background(),
            otlptracehttp.WithEndpoint(endpoint))
        tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
        otel.SetTracerProvider(tp)

        tracer := otel.Tracer("myapp")
        opts = append(opts, relica.WithTracer(relica.NewOtelTracer(tracer)))
    }

    return relica.Open("postgres", os.Getenv("DATABASE_URL"), opts...)
}
```

---

## Troubleshooting

### No logs appearing

**Check**:
1. Logger is configured with correct level (e.g., `slog.LevelInfo`)
2. `WithLogger` option is passed to `relica.Open()`
3. Queries are actually executing (check for errors)

### Traces not appearing in backend

**Check**:
1. Exporter is correctly configured with endpoint
2. `WithTracer` option is passed to `relica.Open()`
3. TracerProvider is flushed on shutdown: `tp.Shutdown(ctx)`
4. Firewall/network allows connection to trace collector

### Sensitive data leaking

**Check**:
1. Field names match your schema (case-insensitive)
2. Use `WithSensitiveFields()` for custom field names
3. Verify log output: sensitive values should show `***REDACTED***`

---

## API Reference

### Logger Interface

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
}
```

### Tracer Interface

```go
type Tracer interface {
    StartSpan(ctx context.Context, name string) (context.Context, Span)
}

type Span interface {
    SetAttributes(attrs ...attribute.KeyValue)
    RecordError(err error)
    SetStatus(code codes.Code, description string)
    End()
}
```

### Configuration Options

```go
// Logger
relica.WithLogger(logger relica.Logger)

// Tracer
relica.WithTracer(tracer relica.Tracer)

// Sensitive fields
relica.WithSensitiveFields(fields []string)
```

---

## Further Reading

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
- [Go slog Package](https://pkg.go.dev/log/slog)
- [Database Semantic Conventions (OTel)](https://opentelemetry.io/docs/specs/semconv/database/)
- [Relica GitHub Repository](https://github.com/coregx/relica)

---

**Version**: 0.5.0-beta
**Last Updated**: 2025-01-24
