// Package core provides the core database functionality including connection management,
// query building, statement caching, and result scanning for Relica.
package core

import (
	"context"
	"database/sql"
	"time"

	"github.com/coregx/relica/internal/cache"
	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/logger"
	"github.com/coregx/relica/internal/tracer"
)

// Optimizer interface for query optimization analysis.
// Forward declaration to avoid import cycle.
type Optimizer interface {
	Analyze(ctx context.Context, query string, args []interface{}, executionTime time.Duration) (interface{}, error)
	Suggest(analysis interface{}) []interface{}
}

// DB represents the main database connection with caching and tracing.
type DB struct {
	sqlDB      *sql.DB
	driverName string
	stmtCache  *cache.StmtCache
	dialect    dialects.Dialect
	oldTracer  Tracer             // Deprecated: kept for backward compatibility
	logger     logger.Logger      // Structured logger for query logging
	tracer     tracer.Tracer      // Distributed tracer for observability
	sanitizer  *logger.Sanitizer  // Sanitizes sensitive data in logs
	optimizer  Optimizer          // Query optimizer (nil = disabled)
	params     []string
	ctx        context.Context
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

// WithLogger sets the logger for the database.
// If not set, a NoopLogger is used (zero overhead when logging is disabled).
func WithLogger(l logger.Logger) Option {
	return func(db *DB) {
		db.logger = l
	}
}

// WithTracer sets the distributed tracer for the database.
// If not set, a NoopTracer is used (zero overhead when tracing is disabled).
func WithTracer(t tracer.Tracer) Option {
	return func(db *DB) {
		db.tracer = t
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
		oldTracer:  NewNoOpTracer(),
		logger:     &logger.NoopLogger{},
		tracer:     &tracer.NoopTracer{},
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
		oldTracer:  NewNoOpTracer(),
		logger:     &logger.NoopLogger{},
		tracer:     &tracer.NoopTracer{},
		sanitizer:  logger.NewSanitizer(nil),
	}
}

// Close releases all database resources.
func (db *DB) Close() error {
	db.stmtCache.Clear()
	return db.sqlDB.Close()
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

// ExecContext executes a raw SQL query (INSERT/UPDATE/DELETE).
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.sqlDB.ExecContext(ctx, query, args...)
}

// QueryContext executes a raw SQL query and returns rows.
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.sqlDB.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a raw SQL query expected to return at most one row.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.sqlDB.QueryRowContext(ctx, query, args...)
}
