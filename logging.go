package relica

import (
	"context"
	"log/slog"
	"time"

	"github.com/coregx/relica/internal/core"
	"github.com/coregx/relica/internal/logger"
)

// QueryEvent describes one executed statement. It is passed to every
// QueryHook after the statement completes.
type QueryEvent struct {
	// SQL is the executed SQL with dialect placeholders ($1 or ?).
	SQL string

	// Args are the bound arguments. They may contain secrets: pass them through
	// MaskArgs before logging.
	Args []any

	// Duration is how long the statement took.
	Duration time.Duration

	// RowsAffected is the number of rows affected (INSERT/UPDATE/DELETE).
	RowsAffected int64

	// Error is the execution or scan error, or nil on success.
	Error error

	// Operation is SELECT, INSERT, UPDATE, DELETE or UNKNOWN (see DetectOperation).
	Operation string
}

// QueryHook is called after each statement with its QueryEvent. Use it for
// logging, metrics or tracing. Several hooks registered with WithQueryHook
// run in registration order. A hook runs synchronously on the query's
// goroutine, so keep it fast.
type QueryHook func(ctx context.Context, event QueryEvent)

// WithQueryHook registers a callback invoked after each statement. Hooks
// compose: each call adds one. A nil hook is ignored. Without hooks there is
// no per-query overhead.
//
// Example:
//
//	db, _ := relica.Open("postgres", dsn,
//	    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
//	        slog.InfoContext(ctx, "query", "sql", e.SQL, "duration", e.Duration, "err", e.Error)
//	    }))
func WithQueryHook(hook QueryHook) Option {
	if hook == nil {
		return Option{}
	}
	return Option{core.WithQueryHook(func(ctx context.Context, e core.QueryEvent) {
		hook(ctx, QueryEvent(e))
	})}
}

// Logger is the logging interface used by WithLogger. Arguments after msg are
// alternating keys and values, as in log/slog. NewSlogAdapter adapts a
// *slog.Logger.
type Logger interface {
	// Debug logs a debug-level message with optional key-value pairs.
	Debug(msg string, args ...any)
	// Info logs an informational message with optional key-value pairs.
	Info(msg string, args ...any)
	// Warn logs a warning message with optional key-value pairs.
	Warn(msg string, args ...any)
	// Error logs an error message with optional key-value pairs.
	Error(msg string, args ...any)
}

// WithLogger logs every statement through l. It is a QueryHook: failed
// statements are logged at Error level, statements that found no row at Warn,
// others at Info, with keys "sql", "duration_ms", "rows" and "error". Arguments
// are not logged. A nil logger is ignored.
//
// Example:
//
//	l := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithLogger(relica.NewSlogAdapter(l)))
func WithLogger(l Logger) Option {
	if l == nil {
		return Option{}
	}
	return Option{core.WithLogger(l)}
}

// NoopLogger is a Logger that discards everything.
type NoopLogger struct{}

// Debug discards the message.
func (NoopLogger) Debug(string, ...any) {}

// Info discards the message.
func (NoopLogger) Info(string, ...any) {}

// Warn discards the message.
func (NoopLogger) Warn(string, ...any) {}

// Error discards the message.
func (NoopLogger) Error(string, ...any) {}

// SlogAdapter is a Logger that writes to a *slog.Logger. Create it with
// NewSlogAdapter.
type SlogAdapter struct {
	a *logger.SlogAdapter
}

// NewSlogAdapter returns a Logger writing to l. A nil l uses slog.Default().
func NewSlogAdapter(l *slog.Logger) *SlogAdapter {
	if l == nil {
		l = slog.Default()
	}
	return &SlogAdapter{a: logger.NewSlogAdapter(l)}
}

// Debug logs at slog.LevelDebug.
func (s *SlogAdapter) Debug(msg string, args ...any) { s.a.Debug(msg, args...) }

// Info logs at slog.LevelInfo.
func (s *SlogAdapter) Info(msg string, args ...any) { s.a.Info(msg, args...) }

// Warn logs at slog.LevelWarn.
func (s *SlogAdapter) Warn(msg string, args ...any) { s.a.Warn(msg, args...) }

// Error logs at slog.LevelError.
func (s *SlogAdapter) Error(msg string, args ...any) { s.a.Error(msg, args...) }

// Compile-time checks: the public Logger is interchangeable with the internal
// one, and both implementations satisfy it.
var (
	_ logger.Logger = Logger(nil)
	_ Logger        = logger.Logger(nil)
	_ Logger        = NoopLogger{}
	_ Logger        = (*NoopLogger)(nil)
	_ Logger        = (*SlogAdapter)(nil)
)

// defaultSanitizer is a package-level singleton to avoid recompiling 20 regexps per call.
var defaultSanitizer = logger.NewSanitizer(nil)

// MaskArgs masks sensitive parameter values (passwords, tokens, API keys) in query args.
// If the SQL references a sensitive column (password, token, api_key, etc.), ALL parameters
// of that statement are masked — not just the sensitive one. This is a conservative heuristic.
// Use this in custom QueryHook implementations to safely log parameters.
//
// Example:
//
//	relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
//	    masked := relica.MaskArgs(e.SQL, e.Args)
//	    slog.Info("query", "sql", e.SQL, "args", masked)
//	})
func MaskArgs(query string, args []any) []any {
	return defaultSanitizer.MaskParams(query, args)
}

// DetectOperation returns the SQL operation of a statement: SELECT, INSERT,
// UPDATE, DELETE or UNKNOWN. It is the value of QueryEvent.Operation.
func DetectOperation(query string) string { return core.DetectOperation(query) }
