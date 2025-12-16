// Package core provides the core database functionality including connection management,
// query building, statement caching, and result scanning for Relica.
package core

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/coregx/relica/internal/cache"
	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/logger"
	"github.com/coregx/relica/internal/security"
)

// Optimizer interface for query optimization analysis.
// Forward declaration to avoid import cycle.
type Optimizer interface {
	Analyze(ctx context.Context, query string, args []interface{}, executionTime time.Duration) (interface{}, error)
	Suggest(analysis interface{}) []interface{}
}

// DB represents the main database connection with caching and query hooks.
type DB struct {
	sqlDB         *sql.DB
	driverName    string
	stmtCache     *cache.StmtCache
	dialect       dialects.Dialect
	logger        logger.Logger       // Structured logger for query logging
	queryHook     QueryHook           // Query hook for logging/metrics/tracing
	sanitizer     *logger.Sanitizer   // Sanitizes sensitive data in logs
	optimizer     Optimizer           // Query optimizer (nil = disabled)
	healthChecker *healthChecker      // Health checker for connection monitoring (nil = disabled)
	validator     *security.Validator // SQL injection validator (nil = disabled)
	auditor       *security.Auditor   // Audit logger for security compliance (nil = disabled)
	params        []string
	ctx           context.Context
}

// Tx represents a database transaction.
type Tx struct {
	tx      *sql.Tx
	builder *QueryBuilder
	ctx     context.Context
}

// TxOptions represents transaction options including isolation level.
type TxOptions struct {
	// Isolation level for the transaction (e.g., sql.LevelReadCommitted)
	Isolation sql.IsolationLevel
	// ReadOnly indicates whether the transaction is read-only
	ReadOnly bool
}

// Option is a functional option for configuring DB.
type Option func(*DB)

// WithMaxOpenConns sets the maximum number of open connections.
func WithMaxOpenConns(n int) Option {
	return func(db *DB) {
		db.sqlDB.SetMaxOpenConns(n)
	}
}

// WithMaxIdleConns sets the maximum number of idle connections.
func WithMaxIdleConns(n int) Option {
	return func(db *DB) {
		db.sqlDB.SetMaxIdleConns(n)
	}
}

// WithConnMaxLifetime sets the maximum amount of time a connection may be reused.
// Expired connections may be closed lazily before reuse.
// If duration <= 0, connections are not closed due to a connection's age.
func WithConnMaxLifetime(d time.Duration) Option {
	return func(db *DB) {
		db.sqlDB.SetConnMaxLifetime(d)
	}
}

// WithConnMaxIdleTime sets the maximum amount of time a connection may be idle.
// Expired connections may be closed lazily before reuse.
// If duration <= 0, connections are not closed due to a connection's idle time.
func WithConnMaxIdleTime(d time.Duration) Option {
	return func(db *DB) {
		db.sqlDB.SetConnMaxIdleTime(d)
	}
}

// WithHealthCheck enables periodic health checks on database connections.
// The health checker pings the database at the specified interval to detect dead connections.
// If interval <= 0, health checks are disabled.
func WithHealthCheck(interval time.Duration) Option {
	return func(db *DB) {
		if interval > 0 {
			db.healthChecker = newHealthChecker(db.sqlDB, db.logger, interval)
			db.healthChecker.start()
		}
	}
}

// WithStmtCacheCapacity sets the prepared statement cache capacity.
func WithStmtCacheCapacity(capacity int) Option {
	return func(db *DB) {
		db.stmtCache = cache.NewStmtCacheWithCapacity(capacity)
	}
}

// WithOptimizer enables query optimization analysis with the given optimizer.
// The optimizer will analyze query execution plans and provide suggestions for improvements.
func WithOptimizer(optimizer Optimizer) Option {
	return func(db *DB) {
		db.optimizer = optimizer
	}
}

// WithValidator enables SQL injection prevention with the given validator.
// If not set, no SQL validation is performed (queries execute as-is).
// Use security.NewValidator() for default validation or security.NewValidator(security.WithStrict(true)) for strict mode.
func WithValidator(validator *security.Validator) Option {
	return func(db *DB) {
		db.validator = validator
	}
}

// WithAuditLog enables audit logging with the given auditor.
// If not set, no audit logging is performed.
// Use security.NewAuditor(logger, security.AuditWrites) for write-only auditing,
// or security.NewAuditor(logger, security.AuditAll) for complete audit trail.
func WithAuditLog(auditor *security.Auditor) Option {
	return func(db *DB) {
		db.auditor = auditor
	}
}

// WithLogger sets the logger for the database.
// If not set, a NoopLogger is used (zero overhead when logging is disabled).
func WithLogger(l logger.Logger) Option {
	return func(db *DB) {
		db.logger = l
	}
}

// WithQueryHook sets a callback function that is invoked after each query execution.
// Use this for logging, metrics, distributed tracing, or debugging.
// If not set, no hook is called (zero overhead).
//
// Example:
//
//	db, _ := relica.Open("postgres", dsn,
//	    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
//	        slog.Info("query", "sql", e.SQL, "duration", e.Duration)
//	    }))
func WithQueryHook(hook QueryHook) Option {
	return func(db *DB) {
		db.queryHook = hook
	}
}

// WithSensitiveFields sets the list of sensitive field names for parameter masking.
// If not set, default sensitive field patterns are used (password, token, api_key, etc.).
func WithSensitiveFields(fields []string) Option {
	return func(db *DB) {
		db.sanitizer = logger.NewSanitizer(fields)
	}
}

// NewDB creates a new DB instance.
func NewDB(driverName, dsn string) (*DB, error) {
	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	dialect := dialects.GetDialect(driverName)
	return &DB{
		sqlDB:      sqlDB,
		driverName: driverName,
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialect,
		logger:     &logger.NoopLogger{},
		sanitizer:  logger.NewSanitizer(nil),
	}, nil
}

// Open creates a new DB instance with options.
func Open(driverName, dsn string, opts ...Option) (*DB, error) {
	db, err := NewDB(driverName, dsn)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(db)
	}

	return db, nil
}

// WrapDB wraps an existing *sql.DB connection with Relica's query builder.
// The caller is responsible for managing the connection lifecycle (including Close()).
//
// This is useful when you need to:
//   - Use Relica with an externally managed connection pool
//   - Integrate with existing code that already has a *sql.DB instance
//   - Apply custom connection pool settings before wrapping
//
// Example:
//
//	db, _ := sql.Open("postgres", dsn)
//	db.SetMaxOpenConns(100)
//	db.SetConnMaxLifetime(time.Hour)
//	relicaDB := relica.WrapDB(db, "postgres")
//	defer db.Close() // Caller's responsibility
//
// Note: Each wrapped DB instance gets its own statement cache for isolation.
func WrapDB(sqlDB *sql.DB, driverName string) *DB {
	dialect := dialects.GetDialect(driverName)
	return &DB{
		sqlDB:      sqlDB,
		driverName: driverName,
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialect,
		logger:     &logger.NoopLogger{},
		sanitizer:  logger.NewSanitizer(nil),
	}
}

// Close releases all database resources.
func (db *DB) Close() error {
	// Stop health checker if running
	if db.healthChecker != nil {
		db.healthChecker.shutdown()
	}

	db.stmtCache.Clear()
	return db.sqlDB.Close()
}

// DriverName returns the name of the DB driver.
func (db *DB) DriverName() string {
	return db.driverName
}

// WithContext returns a new DB with the given context.
func (db *DB) WithContext(ctx context.Context) *DB {
	newDB := *db
	newDB.ctx = ctx
	return &newDB
}

// Builder returns a query builder for this database.
func (db *DB) Builder() *QueryBuilder {
	return &QueryBuilder{db: db}
}

// NewQueryBuilder creates a new query builder with optional transaction support.
// When tx is not nil, all queries built by this builder execute within that transaction.
func NewQueryBuilder(db *DB, tx *sql.Tx) *QueryBuilder {
	return &QueryBuilder{db: db, tx: tx}
}

// Begin starts a transaction with default options.
func (db *DB) Begin(ctx context.Context) (*Tx, error) {
	return db.BeginTx(ctx, nil)
}

// BeginTx starts a transaction with specified options.
// Options can specify isolation level and read-only mode.
func (db *DB) BeginTx(ctx context.Context, opts *TxOptions) (*Tx, error) {
	var sqlOpts *sql.TxOptions
	if opts != nil {
		sqlOpts = &sql.TxOptions{
			Isolation: opts.Isolation,
			ReadOnly:  opts.ReadOnly,
		}
	}

	tx, err := db.sqlDB.BeginTx(ctx, sqlOpts)
	if err != nil {
		return nil, err
	}

	return &Tx{
		tx:      tx,
		builder: NewQueryBuilder(db, tx),
		ctx:     ctx,
	}, nil
}

// Builder returns the query builder for this transaction.
// All queries built using this builder will execute within the transaction.
// The builder automatically inherits the transaction's context.
func (tx *Tx) Builder() *QueryBuilder {
	// Return a new builder that inherits the transaction context
	return &QueryBuilder{
		db:  tx.builder.db,
		tx:  tx.tx,
		ctx: tx.ctx,
	}
}

// Commit commits the transaction.
func (tx *Tx) Commit() error {
	return tx.tx.Commit()
}

// Rollback rolls back the transaction.
func (tx *Tx) Rollback() error {
	return tx.tx.Rollback()
}

// Transactional executes f within a transaction with automatic commit/rollback.
// If f returns an error, the transaction is rolled back and the error is returned.
// If f panics, the transaction is rolled back and the panic is re-raised.
// If f completes successfully, the transaction is committed.
func (db *DB) Transactional(ctx context.Context, f func(*Tx) error) (err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback() //nolint:errcheck,gosec
			panic(p)      // Re-panic after rollback
		} else if err != nil {
			tx.Rollback() //nolint:errcheck,gosec
		} else {
			err = tx.Commit()
		}
	}()

	err = f(tx)
	return err
}

// TransactionalTx executes f within a transaction with custom options.
// Options can specify isolation level and read-only mode.
// If f returns an error, the transaction is rolled back and the error is returned.
// If f panics, the transaction is rolled back and the panic is re-raised.
// If f completes successfully, the transaction is committed.
func (db *DB) TransactionalTx(ctx context.Context, opts *TxOptions, f func(*Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback() //nolint:errcheck,gosec
			panic(p)      // Re-panic after rollback
		} else if err != nil {
			tx.Rollback() //nolint:errcheck,gosec
		} else {
			err = tx.Commit()
		}
	}()

	err = f(tx)
	return err
}

// PoolStats represents database connection pool statistics.
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

	// Healthy indicates whether the last health check was successful.
	// Always true if health checks are disabled.
	Healthy bool

	// LastHealthCheck is the time of the most recent health check.
	// Zero if health checks are disabled.
	LastHealthCheck time.Time
}

// Stats returns database connection pool statistics.
func (db *DB) Stats() PoolStats {
	stats := db.sqlDB.Stats()

	poolStats := PoolStats{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
		Healthy:            true,
	}

	if db.healthChecker != nil {
		poolStats.Healthy = db.healthChecker.isHealthy()
		poolStats.LastHealthCheck = db.healthChecker.lastCheck()
	}

	return poolStats
}

// IsHealthy returns true if the database connection is healthy.
// Always returns true if health checks are disabled.
func (db *DB) IsHealthy() bool {
	if db.healthChecker == nil {
		return true
	}
	return db.healthChecker.isHealthy()
}

// WarmCache pre-warms the statement cache by preparing frequently-used queries.
// This improves performance at startup by avoiding cache misses for common queries.
// The queries are prepared synchronously in the order provided.
// Returns the number of successfully prepared queries and any error encountered.
func (db *DB) WarmCache(queries []string) (int, error) {
	// Use background context since this is typically called at startup
	ctx := context.Background()

	warmed := 0
	for _, query := range queries {
		stmt, err := db.sqlDB.PrepareContext(ctx, query)
		if err != nil {
			return warmed, err
		}
		db.stmtCache.Set(query, stmt)
		warmed++
	}

	return warmed, nil
}

// PinQuery marks a query as pinned in the statement cache, preventing eviction.
// Pinned queries remain in cache indefinitely, useful for frequently-used queries.
// Returns false if the query is not in cache (call WarmCache first).
func (db *DB) PinQuery(query string) bool {
	return db.stmtCache.Pin(query)
}

// UnpinQuery removes the pin from a cached query, allowing normal LRU eviction.
// Returns false if the query is not in cache or not pinned.
func (db *DB) UnpinQuery(query string) bool {
	return db.stmtCache.Unpin(query)
}

// validateQueryAndParams validates query and parameters if validator is enabled.
// Logs security events if auditor is enabled.
// Returns error if validation fails.
func (db *DB) validateQueryAndParams(ctx context.Context, query string, args []interface{}) error {
	if db.validator == nil {
		return nil
	}

	if err := db.validator.ValidateQuery(query); err != nil {
		if db.auditor != nil {
			db.auditor.LogSecurityEvent(ctx, "query_blocked", query, err)
		}
		return err
	}

	if err := db.validator.ValidateParams(args); err != nil {
		if db.auditor != nil {
			db.auditor.LogSecurityEvent(ctx, "params_blocked", query, err)
		}
		return err
	}

	return nil
}

// ExecContext executes a raw SQL query (INSERT/UPDATE/DELETE).
// If a validator is configured, the query and parameters are validated before execution.
// If an auditor is configured, the operation is logged to the audit trail.
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	// Track execution time for audit log
	start := time.Now()

	// Validate query and parameters if validator is enabled
	if err := db.validateQueryAndParams(ctx, query, args); err != nil {
		return nil, err
	}

	// Execute query
	result, err := db.sqlDB.ExecContext(ctx, query, args...)
	duration := time.Since(start)

	// Audit log if enabled
	if db.auditor != nil {
		// Detect operation type (simplified)
		operation := detectOperation(query)
		db.auditor.LogOperation(ctx, operation, query, args, result, err, duration)
	}

	return result, err
}

// QueryContext executes a raw SQL query and returns rows.
// If a validator is configured, the query and parameters are validated before execution.
// If an auditor is configured, the operation is logged to the audit trail.
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	// Track execution time for audit log
	start := time.Now()

	// Validate query and parameters if validator is enabled
	if err := db.validateQueryAndParams(ctx, query, args); err != nil {
		return nil, err
	}

	// Execute query
	rows, err := db.sqlDB.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	// Audit log if enabled
	if db.auditor != nil {
		// For SELECT queries, result is nil (we can't get row count without consuming rows)
		db.auditor.LogOperation(ctx, "SELECT", query, args, nil, err, duration)
	}

	return rows, err
}

// QueryRowContext executes a raw SQL query expected to return at most one row.
// Note: Due to database/sql API constraints, QueryRowContext does NOT support validation.
// Use QueryContext() instead if you need validation, or ensure the query is safe before calling this method.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	// Note: We cannot validate here because QueryRowContext cannot return errors.
	// Validation must be done at a higher level (QueryBuilder) or users should use QueryContext.
	return db.sqlDB.QueryRowContext(ctx, query, args...)
}

// detectOperation attempts to detect the SQL operation type from a query string.
// Returns the operation type (INSERT, UPDATE, DELETE, SELECT, etc.) for audit logging.
func detectOperation(query string) string {
	// Convert to uppercase for matching
	upper := strings.ToUpper(strings.TrimSpace(query))

	// Match common operations
	if strings.HasPrefix(upper, "INSERT") {
		return "INSERT"
	}
	if strings.HasPrefix(upper, "UPDATE") {
		return "UPDATE"
	}
	if strings.HasPrefix(upper, "DELETE") {
		return "DELETE"
	}
	if strings.HasPrefix(upper, "SELECT") {
		return "SELECT"
	}
	if strings.HasPrefix(upper, "CREATE") {
		return "CREATE"
	}
	if strings.HasPrefix(upper, "DROP") {
		return "DROP"
	}
	if strings.HasPrefix(upper, "ALTER") {
		return "ALTER"
	}
	if strings.HasPrefix(upper, "TRUNCATE") {
		return "TRUNCATE"
	}

	// Unknown operation
	return "UNKNOWN"
}
