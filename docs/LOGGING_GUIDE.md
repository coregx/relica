# Logging and Tracing Guide for Relica

This guide covers SQL query logging (`relica.WithLogger`) and observability hooks (`relica.WithQueryHook`), including OpenTelemetry tracing written in your own code.

Relica stays zero-dependency: it imports no logging or tracing library. `WithLogger` takes any value with `Debug/Info/Warn/Error` methods, and tracing is a few lines of your code in a `QueryHook` — the OpenTelemetry import lives in your module, not in Relica.

## Table of Contents

- [Quick Start](#quick-start)
- [Logging with slog](#logging-with-slog)
- [Query Hooks](#query-hooks)
- [Tracing with OpenTelemetry via QueryHook](#tracing-with-opentelemetry-via-queryhook)
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
    _ = db.Select().From("users").All(&users)
    // Output: {"time":"...","level":"INFO","msg":"query executed",
    //          "sql":"SELECT * FROM \"users\"","duration_ms":15,"rows":0}
}
```

### Basic Tracing (OpenTelemetry)

Register a hook that turns each statement into a span — see [Tracing with OpenTelemetry via QueryHook](#tracing-with-opentelemetry-via-queryhook) for the hook itself:

```go
db, err := relica.Open("postgres", dsn,
    relica.WithQueryHook(otelHook(otel.Tracer("myapp/db"))))
```

---

## Logging with slog

`relica.WithLogger(l)` registers a query hook that logs every statement through `l`. `relica.NewSlogAdapter` adapts a `*slog.Logger` (a nil logger uses `slog.Default()`); any type with the four `relica.Logger` methods works too.

### Logger Levels

Relica logs at different levels depending on the outcome:

| Level | Message | When |
|---|---|---|
| INFO | `query executed` | Statement succeeded |
| WARN | `no rows` | Query found no row (`relica.ErrNotFound` / `sql.ErrNoRows`) |
| ERROR | `query failed` | Execution or scanning failed |

### Log Fields

| Field | Type | Description |
|---|---|---|
| `sql` | string | SQL text with dialect placeholders (`$1` or `?`) |
| `duration_ms` | int64 | Statement duration in milliseconds |
| `rows` | int64 | Rows affected (INSERT/UPDATE/DELETE); INFO only |
| `error` | error | The error; ERROR only |

Argument values are **not** logged. To log them, write your own hook and mask them with `relica.MaskArgs` (see [Sensitive Data Masking](#sensitive-data-masking)).

### Custom Log Handlers

```go
// Text handler (human-readable)
textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
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

A query hook is often simpler for this (next section).

---

## Query Hooks

`relica.WithQueryHook(hook)` registers a `relica.QueryHook`, called after each statement:

```go
type QueryHook func(ctx context.Context, event QueryEvent)

type QueryEvent struct {
    SQL          string        // executed SQL with dialect placeholders ($1 or ?)
    Args         []any         // bound arguments — may contain secrets, see MaskArgs
    Duration     time.Duration // how long the statement took
    RowsAffected int64         // INSERT/UPDATE/DELETE
    Error        error         // execution or scan error, nil on success
    Operation    string        // SELECT, INSERT, UPDATE, DELETE or UNKNOWN
}
```

- `ctx` is the query's context (from `WithContext`), so request-scoped values such as the active trace span are available.
- Hooks compose: each `WithQueryHook` adds one, and they run in registration order. `WithLogger` is itself a hook.
- A hook runs synchronously on the query's goroutine, after the statement completes: keep it fast.
- Without hooks there is no per-query overhead.

```go
// Slow query log
db, err := relica.Open("postgres", dsn,
    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
        if e.Duration >= 100*time.Millisecond {
            slog.WarnContext(ctx, "slow query", "sql", e.SQL, "duration", e.Duration)
        }
    }))
```

---

## Tracing with OpenTelemetry via QueryHook

Relica has no built-in tracer. A hook creates the span from the event. Because the hook runs **after** the statement, the span is back-dated: it starts at `now - Duration` and ends now. The span becomes a child of whatever span is in the query's context.

```go
package dbtrace

import (
    "context"
    "errors"
    "time"

    "github.com/coregx/relica"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

// otelHook returns a QueryHook that records one client span per statement.
func otelHook(tracer trace.Tracer) relica.QueryHook {
    return func(ctx context.Context, e relica.QueryEvent) {
        end := time.Now()
        _, span := tracer.Start(ctx, e.Operation,
            trace.WithSpanKind(trace.SpanKindClient),
            trace.WithTimestamp(end.Add(-e.Duration)),
            trace.WithAttributes(
                attribute.String("db.system.name", "postgresql"),
                attribute.String("db.operation.name", e.Operation),
                attribute.String("db.query.text", e.SQL),
                attribute.Int64("db.rows_affected", e.RowsAffected),
            ))
        // A query that found no row is a normal result, not a span error.
        if e.Error != nil && !errors.Is(e.Error, relica.ErrNotFound) {
            span.RecordError(e.Error)
            span.SetStatus(codes.Error, e.Error.Error())
        }
        span.End(trace.WithTimestamp(end))
    }
}
```

Notes:

- `db.query.text` is the SQL with placeholders; argument values are not recorded. Do not add `e.Args` to spans unless they pass through `relica.MaskArgs`.
- Attribute names follow the [OpenTelemetry database semantic conventions](https://opentelemetry.io/docs/specs/semconv/database/); set `db.system.name` to your database (`postgresql`, `mysql`, `sqlite`).
- Exporters, samplers and the `TracerProvider` are configured in your application as usual.

### Integrating with HTTP Tracing

Pass the request context to the query; the hook's span then nests under the HTTP span:

```go
func getUserHandler(w http.ResponseWriter, r *http.Request) {
    ctx, span := tracer.Start(r.Context(), "getUserHandler")
    defer span.End()

    var user User
    err := db.Select().
        From("users").
        Where(relica.Eq("id", r.URL.Query().Get("id"))).
        WithContext(ctx).
        One(&user)
    if err != nil {
        span.RecordError(err)
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(user)
}
```

### Metrics

The same event feeds metrics. With OpenTelemetry metrics (in your code):

```go
hist, _ := otel.Meter("myapp/db").Float64Histogram("db.client.operation.duration",
    metric.WithUnit("s"))

db, err := relica.Open("postgres", dsn,
    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
        hist.Record(ctx, e.Duration.Seconds(),
            metric.WithAttributes(attribute.String("db.operation.name", e.Operation)))
    }))
```

---

## Sensitive Data Masking

`WithLogger` never logs arguments. A custom hook that does must mask them with `relica.MaskArgs(sql, args)`: if the SQL references a sensitive column (password, token, api_key and similar), **all** arguments of that statement are masked — not just the sensitive one, to prevent correlation.

```go
db, err := relica.Open("postgres", dsn,
    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
        slog.DebugContext(ctx, "query",
            "sql", e.SQL,
            "args", relica.MaskArgs(e.SQL, e.Args))
    }))

// UPDATE users SET password = $1 WHERE id = $2  → every argument is masked
```

The list of sensitive column names is fixed. For other sensitive columns, filter `e.Args` yourself before logging, or do not log arguments at all.

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

`WithLogger` calls the logger without a context. To put the trace ID on query logs, log from your own hook, which receives the query's context:

```go
db, err := relica.Open("postgres", dsn,
    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
        sc := trace.SpanContextFromContext(ctx)
        slog.InfoContext(ctx, "query",
            "sql", e.SQL,
            "duration_ms", e.Duration.Milliseconds(),
            "trace_id", sc.TraceID().String())
    }))
```

### 5. Log Error Classification, Not Raw DB Errors

Avoid logging raw database errors directly to end users or external APIs. Use error classification to convert constraint errors into safe, descriptive messages:

```go
if err := db.Model(&user).Insert(); err != nil {
    switch {
    case relica.IsUniqueViolation(err):
        logger.Info("duplicate user rejected", "email", user.Email)
        return ErrEmailTaken
    case relica.IsForeignKeyViolation(err):
        logger.Warn("FK violation", "err", err)
        return ErrInvalidReference
    default:
        logger.Error("unexpected db error", "err", err)
        return ErrInternal
    }
}
```

This keeps database internals out of application responses and makes logs actionable.

### 6. Disable in Performance-Critical Paths

```go
opts := []relica.Option{relica.WithMaxOpenConns(100)}

// Enable only for debugging
if os.Getenv("DEBUG_SQL") == "true" {
    opts = append(opts, relica.WithLogger(relica.NewSlogAdapter(logger)))
}

db, err := relica.Open("postgres", dsn, opts...)
```

---

## Performance

- No logger and no hooks: no per-query overhead.
- Each hook (including `WithLogger`) runs synchronously after every statement, on the query's goroutine. Its cost is the cost of your logging handler or tracer: keep hooks short and let exporters batch (`sdktrace.WithBatcher`).
- Sampling belongs in the logger handler or the trace sampler, not in Relica.

---

## Examples

### Example 1: Production Setup (JSON logs + OTLP traces)

```go
func openDB(ctx context.Context, dsn string) (*relica.DB, func(), error) {
    // Logging
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    // Tracing (your code; Relica does not import OpenTelemetry)
    exporter, err := otlptracehttp.New(ctx)
    if err != nil {
        return nil, nil, err
    }
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)), // 10% sampling
    )
    otel.SetTracerProvider(tp)

    db, err := relica.Open("postgres", dsn,
        relica.WithLogger(relica.NewSlogAdapter(logger)),
        relica.WithQueryHook(otelHook(tp.Tracer("myapp/db"))))
    if err != nil {
        return nil, nil, err
    }

    cleanup := func() {
        db.Close()
        tp.Shutdown(context.Background())
    }
    return db, cleanup, nil
}
```

### Example 2: Development Setup (Console Output)

```go
func main() {
    // Human-readable logs
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))

    db, err := relica.Open("sqlite", ":memory:",
        relica.WithLogger(relica.NewSlogAdapter(logger)),
        // Arguments too, masked
        relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
            logger.DebugContext(ctx, "args", "values", relica.MaskArgs(e.SQL, e.Args))
        }))
    if err != nil {
        panic(err)
    }
    defer db.Close()
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

    // Tracing in all environments, if an exporter is configured
    if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
        opts = append(opts, relica.WithQueryHook(otelHook(otel.Tracer("myapp/db"))))
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
2. The hook is registered with `relica.WithQueryHook` in `relica.Open()`
3. Queries carry the request context (`WithContext(ctx)`); otherwise spans are roots, not children of the request span
4. TracerProvider is flushed on shutdown: `tp.Shutdown(ctx)`

### Sensitive data leaking

**Check**:
1. Custom hooks pass `e.Args` through `relica.MaskArgs` before logging
2. Spans record `e.SQL` (placeholders only), never raw `e.Args`
3. Columns outside the built-in sensitive list are filtered by your own code

---

## API Reference

All in package `relica` (`go doc github.com/coregx/relica <name>`):

| Symbol | Purpose |
|---|---|
| `WithLogger(l Logger) Option` | Log every statement (no arguments) |
| `Logger` | `Debug/Info/Warn/Error(msg string, args ...any)` |
| `NewSlogAdapter(l *slog.Logger) *SlogAdapter` | `Logger` backed by `log/slog` |
| `NoopLogger` | `Logger` that discards everything |
| `WithQueryHook(hook QueryHook) Option` | Register a hook; hooks compose |
| `QueryHook` | `func(ctx context.Context, event QueryEvent)` |
| `QueryEvent` | `SQL`, `Args`, `Duration`, `RowsAffected`, `Error`, `Operation` |
| `MaskArgs(query string, args []any) []any` | Mask arguments of statements touching sensitive columns |
| `DetectOperation(query string) string` | The value of `QueryEvent.Operation` |

---

## Further Reading

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
- [Go slog Package](https://pkg.go.dev/log/slog)
- [Database Semantic Conventions (OTel)](https://opentelemetry.io/docs/specs/semconv/database/)
- [Relica GitHub Repository](https://github.com/coregx/relica)
