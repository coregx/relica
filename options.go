package relica

import (
	"database/sql"
	"time"

	"github.com/coregx/relica/internal/core"
)

// Option configures a DB in Open. Create options only with the With…
// functions of this package:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithMaxOpenConns(100),
//	    relica.WithMaxIdleConns(50),
//	    relica.WithQueryHook(hook))
//
// Options apply in the order given. The zero Option does nothing.
type Option struct {
	apply core.Option
}

// coreOptions returns the internal options, skipping zero values.
func coreOptions(opts []Option) []core.Option {
	out := make([]core.Option, 0, len(opts))
	for _, o := range opts {
		if o.apply != nil {
			out = append(out, o.apply)
		}
	}
	return out
}

// WithMaxOpenConns sets the maximum number of open connections
// (database/sql DB.SetMaxOpenConns). n <= 0 means no limit.
func WithMaxOpenConns(n int) Option { return Option{core.WithMaxOpenConns(n)} }

// WithMaxIdleConns sets the maximum number of idle connections
// (database/sql DB.SetMaxIdleConns).
func WithMaxIdleConns(n int) Option { return Option{core.WithMaxIdleConns(n)} }

// WithConnMaxLifetime sets the maximum amount of time a connection may be reused.
// Expired connections may be closed lazily before reuse.
// If duration <= 0, connections are not closed due to a connection's age.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithConnMaxLifetime(5*time.Minute))
func WithConnMaxLifetime(d time.Duration) Option { return Option{core.WithConnMaxLifetime(d)} }

// WithConnMaxIdleTime sets the maximum amount of time a connection may be idle.
// Expired connections may be closed lazily before reuse.
// If duration <= 0, connections are not closed due to a connection's idle time.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithConnMaxIdleTime(1*time.Minute))
func WithConnMaxIdleTime(d time.Duration) Option { return Option{core.WithConnMaxIdleTime(d)} }

// WithHealthCheck enables periodic health checks on database connections.
// The health checker pings the database at the specified interval to detect dead connections.
// If interval <= 0, health checks are disabled.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithHealthCheck(30*time.Second))
func WithHealthCheck(interval time.Duration) Option { return Option{core.WithHealthCheck(interval)} }

// WithStmtCacheCapacity sets the capacity of the prepared statement cache
// (least recently used statements are evicted).
func WithStmtCacheCapacity(capacity int) Option {
	return Option{core.WithStmtCacheCapacity(capacity)}
}

// TxOptions holds the isolation level and read-only flag for a transaction
// started with DB.BeginTx or DB.TransactionalTx. A nil *TxOptions means the
// driver defaults.
//
// Example:
//
//	tx, err := db.BeginTx(ctx, &relica.TxOptions{
//	    Isolation: sql.LevelSerializable,
//	    ReadOnly:  true,
//	})
type TxOptions struct {
	// Isolation is the transaction isolation level, e.g. sql.LevelReadCommitted.
	// The zero value is the driver's default level.
	Isolation sql.IsolationLevel

	// ReadOnly requests a read-only transaction. Writes then fail on databases
	// that enforce it (PostgreSQL, MySQL).
	ReadOnly bool
}

// coreTxOptions converts public transaction options (nil stays nil).
func coreTxOptions(o *TxOptions) *core.TxOptions {
	if o == nil {
		return nil
	}
	c := core.TxOptions(*o)
	return &c
}

// PoolStats reports connection pool statistics, returned by DB.Stats. It
// mirrors database/sql DBStats.
type PoolStats struct {
	// MaxOpenConnections is the maximum number of open connections to the database.
	MaxOpenConnections int

	// OpenConnections is the number of established connections both in use and idle.
	OpenConnections int

	// InUse is the number of connections currently in use.
	InUse int

	// Idle is the number of idle connections.
	Idle int

	// WaitCount is the total number of connections waited for.
	WaitCount int64

	// WaitDuration is the total time blocked waiting for a new connection.
	WaitDuration time.Duration

	// MaxIdleClosed is the total number of connections closed due to SetMaxIdleConns.
	MaxIdleClosed int64

	// MaxIdleTimeClosed is the total number of connections closed due to SetConnMaxIdleTime.
	MaxIdleTimeClosed int64

	// MaxLifetimeClosed is the total number of connections closed due to SetConnMaxLifetime.
	MaxLifetimeClosed int64

	// Healthy indicates whether the last health check (WithHealthCheck) was
	// successful. Always true if health checks are disabled.
	Healthy bool

	// LastHealthCheck is the time of the most recent health check.
	// Zero if health checks are disabled.
	LastHealthCheck time.Time
}
