// Package relica provides a lightweight, type-safe database query builder for Go.
//
// Relica offers a fluent API for building SQL queries with support for:
//   - Multiple databases (PostgreSQL, MySQL, SQLite)
//   - Zero production dependencies
//   - Prepared statement caching
//   - Transaction management
//   - Advanced SQL features (JOINs, aggregates, subqueries, CTEs)
//
// # Quick Start
//
// Install:
//
//	go get github.com/coregx/relica
//
// Basic usage:
//
//	db, err := relica.Open("postgres", "user=postgres dbname=myapp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	var users []User
//	err = db.Builder().Select("*").From("users").All(&users)
//
// # Features
//
// CRUD Operations:
//
//	// SELECT
//	db.Builder().Select("*").From("users").Where("id = ?", 123).One(&user)
//
//	// INSERT
//	db.Builder().Insert("users", map[string]interface{}{
//	    "name": "Alice",
//	    "email": "alice@example.com",
//	}).Execute()
//
//	// UPDATE
//	db.Builder().Update("users").
//	    Set(map[string]interface{}{"status": "active"}).
//	    Where("id = ?", 123).
//	    Execute()
//
//	// DELETE
//	db.Builder().Delete("users").Where("id = ?", 123).Execute()
package relica

import (
	"context"
	"database/sql"

	"github.com/coregx/relica/internal/core"
	"github.com/coregx/relica/internal/logger"
	"github.com/coregx/relica/internal/tracer"
)

// DB represents a database connection with query building capabilities.
//
// DB provides a fluent API for constructing and executing SQL queries
// in a type-safe manner. It wraps the underlying database/sql connection
// and adds features like:
//   - Prepared statement caching (LRU eviction, <60ns hit latency)
//   - Query builder with method chaining
//   - Transaction management (all isolation levels)
//   - Multi-database support (PostgreSQL, MySQL, SQLite)
//
// Example:
//
//	db, err := relica.Open("postgres", "user=postgres dbname=myapp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	var users []User
//	err = db.Builder().
//	    Select("id", "name", "email").
//	    From("users").
//	    Where("active = ?", true).
//	    OrderBy("name").
//	    All(&users)
type DB struct {
	db *core.DB
}

// QueryBuilder constructs type-safe queries.
//
// The query builder provides a fluent interface for building
// SELECT, INSERT, UPDATE, DELETE, UPSERT, and batch operations.
// All queries are cached and executed with prepared statements.
//
// Example:
//
//	qb := db.Builder()
//	qb.Select("*").From("users").Where("status = ?", 1).All(&users)
type QueryBuilder struct {
	qb *core.QueryBuilder
}

// SelectQuery represents a SELECT query being built.
//
// SelectQuery supports a wide range of SQL features including:
//   - JOINs (INNER, LEFT, RIGHT, FULL, CROSS)
//   - Aggregates (COUNT, SUM, AVG, MIN, MAX)
//   - GROUP BY and HAVING
//   - ORDER BY, LIMIT, OFFSET
//   - Set operations (UNION, INTERSECT, EXCEPT)
//   - Common Table Expressions (WITH, WITH RECURSIVE)
//   - Subqueries (in FROM, WHERE, SELECT clauses)
//
// Example:
//
//	sq := db.Builder().
//	    Select("u.name", "COUNT(*) as order_count").
//	    From("users u").
//	    InnerJoin("orders o", "o.user_id = u.id").
//	    GroupBy("u.id", "u.name").
//	    Having("COUNT(*) > ?", 10).
//	    OrderBy("order_count DESC")
//	sq.All(&results)
type SelectQuery struct {
	sq *core.SelectQuery
}

// Tx represents a database transaction.
//
// Transactions provide ACID guarantees and support all standard
// isolation levels. All queries executed through a transaction's
// builder automatically participate in that transaction.
//
// Example:
//
//	tx, err := db.Begin(ctx)
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback() // Safe to call even after Commit
//
//	_, err = tx.Builder().Insert("users", data).Execute()
//	if err != nil {
//	    return err
//	}
//
//	return tx.Commit()
type Tx struct {
	tx *core.Tx
}

// Query represents a built query ready for execution.
//
// Query encapsulates the SQL string, parameters, and execution context.
// It provides methods for executing the query and scanning results.
//
// Example:
//
//	q := db.Builder().Select("*").From("users").Where("id = ?", 123).Build()
//	var user User
//	err := q.One(&user)
type Query struct {
	q *core.Query
}

// TxOptions represents transaction options including isolation level.
//
// Example:
//
//	opts := &relica.TxOptions{
//	    Isolation: sql.LevelSerializable,
//	    ReadOnly:  true,
//	}
//	tx, err := db.BeginTx(ctx, opts)
type TxOptions = core.TxOptions

// PoolStats represents database connection pool statistics.
// It provides insights into connection pool health and usage patterns.
type PoolStats = core.PoolStats

// Option is a functional option for configuring DB.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithMaxOpenConns(100),
//	    relica.WithMaxIdleConns(50))
type Option = core.Option

// Expression represents a database expression for building complex WHERE clauses.
//
// Expressions provide a type-safe way to construct SQL conditions without
// writing raw SQL strings. They support nesting and composition.
//
// Example:
//
//	expr := relica.And(
//	    relica.Eq("status", 1),
//	    relica.Or(
//	        relica.GreaterThan("age", 18),
//	        relica.Eq("verified", true),
//	    ),
//	)
//	db.Builder().Select("*").From("users").Where(expr).All(&users)
type Expression = core.Expression

// HashExp represents a hash-based expression using column-value pairs.
//
// HashExp provides a convenient map syntax for simple equality conditions.
// Special values are handled automatically:
//   - nil → "column IS NULL"
//   - []interface{} → "column IN (...)"
//
// Example:
//
//	db.Builder().Select("*").From("users").Where(relica.HashExp{
//	    "status": 1,
//	    "role": []string{"admin", "moderator"},
//	    "deleted_at": nil,
//	}).All(&users)
type HashExp = core.HashExp

// LikeExp represents a LIKE expression with automatic escaping.
//
// LikeExp provides pattern matching with automatic escaping of
// SQL wildcard characters (%, _).
//
// Example:
//
//	db.Builder().Select("*").From("users").Where(
//	    relica.Like("name", "john%"),
//	).All(&users)
type LikeExp = core.LikeExp

// ============================================================================
// DB Methods
// ============================================================================

// Open creates a new database connection with optional configuration.
//
// The driverName parameter specifies the database driver:
//   - "postgres" - PostgreSQL
//   - "mysql" - MySQL
//   - "sqlite3" - SQLite
//
// The dsn parameter is the database-specific connection string.
//
// Example:
//
//	db, err := relica.Open("postgres", "user=postgres dbname=myapp",
//	    relica.WithMaxOpenConns(100),
//	    relica.WithMaxIdleConns(50))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
func Open(driverName, dsn string, opts ...Option) (*DB, error) {
	coreDB, err := core.Open(driverName, dsn, opts...)
	if err != nil {
		return nil, err
	}
	return &DB{db: coreDB}, nil
}

// NewDB creates a database connection (deprecated: use Open).
//
// This function exists for backward compatibility. New code should use Open.
//
// Example:
//
//	db, err := relica.NewDB("postgres", dsn)
func NewDB(driverName, dsn string) (*DB, error) {
	coreDB, err := core.NewDB(driverName, dsn)
	if err != nil {
		return nil, err
	}
	return &DB{db: coreDB}, nil
}

// WrapDB wraps an existing *sql.DB connection with Relica's query builder.
//
// The caller is responsible for managing the connection lifecycle (including Close()).
// This is useful when you need to:
//   - Use Relica with an externally managed connection pool
//   - Integrate with existing code that already has a *sql.DB instance
//   - Apply custom connection pool settings before wrapping
//
// Example:
//
//	sqlDB, _ := sql.Open("postgres", dsn)
//	sqlDB.SetMaxOpenConns(100)
//	sqlDB.SetConnMaxLifetime(time.Hour)
//	db := relica.WrapDB(sqlDB, "postgres")
//	defer sqlDB.Close() // Caller's responsibility
func WrapDB(sqlDB *sql.DB, driverName string) *DB {
	coreDB := core.WrapDB(sqlDB, driverName)
	return &DB{db: coreDB}
}

// Close releases all database resources including the connection pool
// and statement cache.
//
// After calling Close, the DB instance should not be used.
//
// Example:
//
//	db, _ := relica.Open("postgres", dsn)
//	defer db.Close()
func (d *DB) Close() error {
	return d.db.Close()
}

// WithContext returns a new DB with the given context.
//
// The context will be used for all subsequent query operations
// unless overridden at the query level.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	db := db.WithContext(ctx)
//	db.Builder().Select("*").From("users").All(&users)
func (d *DB) WithContext(ctx context.Context) *DB {
	return &DB{db: d.db.WithContext(ctx)}
}

// Stats returns database connection pool statistics.
//
// Stats provides insights into connection pool usage including:
//   - Number of open/idle/in-use connections
//   - Wait count and duration
//   - Connections closed due to max lifetime/idle time
//   - Health check status (if enabled)
//
// Example:
//
//	stats := db.Stats()
//	fmt.Printf("Open: %d, Idle: %d, InUse: %d\n",
//	    stats.OpenConnections, stats.Idle, stats.InUse)
//	if !stats.Healthy {
//	    log.Warn("Database health check failed")
//	}
func (d *DB) Stats() PoolStats {
	return PoolStats(d.db.Stats())
}

// IsHealthy returns true if the database connection is healthy.
// Always returns true if health checks are disabled.
//
// This is a convenience method that calls Stats() internally.
//
// Example:
//
//	if !db.IsHealthy() {
//	    log.Error("Database connection unhealthy")
//	    // Attempt reconnection or alert
//	}
func (d *DB) IsHealthy() bool {
	return d.db.IsHealthy()
}

// Builder returns a new QueryBuilder for constructing queries.
//
// The query builder provides a fluent interface for building
// SELECT, INSERT, UPDATE, DELETE, and UPSERT queries.
//
// Example:
//
//	db.Builder().
//	    Select("*").
//	    From("users").
//	    Where("id = ?", 123).
//	    One(&user)
func (d *DB) Builder() *QueryBuilder {
	return &QueryBuilder{qb: d.db.Builder()}
}

// Select creates a new SELECT query.
//
// This is a convenience method equivalent to db.Builder().Select(cols...).
// For advanced queries (CTEs, subqueries, UNION), use db.Builder() directly.
//
// Example:
//
//	var users []User
//	err := db.Select("id", "name", "email").
//	    From("users").
//	    Where("active = ?", true).
//	    OrderBy("name").
//	    All(&users)
//
//	// For wildcard selection
//	err := db.Select("*").From("users").All(&users)
//
//	// For advanced features, use Builder()
//	err := db.Builder().
//	    With("stats", statsQuery).
//	    Select("*").
//	    From("stats").
//	    All(&results)
func (d *DB) Select(cols ...string) *SelectQuery {
	return d.Builder().Select(cols...)
}

// Insert creates a new INSERT query.
//
// This is a convenience method equivalent to db.Builder().Insert(table, data).
// For batch inserts, use db.Builder().BatchInsert().
//
// Example:
//
//	result, err := db.Insert("users", map[string]interface{}{
//	    "name":  "Alice",
//	    "email": "alice@example.com",
//	}).Execute()
//	if err != nil {
//	    return err
//	}
//	rows, _ := result.RowsAffected()
//	fmt.Printf("Inserted %d row(s)\n", rows)
//
//	// For batch operations, use Builder()
//	result, err := db.Builder().
//	    BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").
//	    Values("Bob", "bob@example.com").
//	    Execute()
func (d *DB) Insert(table string, data map[string]interface{}) *Query {
	return d.Builder().Insert(table, data)
}

// Update creates a new UPDATE query.
//
// This is a convenience method equivalent to db.Builder().Update(table).
// For batch updates, use db.Builder().BatchUpdate().
//
// Example:
//
//	_, err := db.Update("users").
//	    Set(map[string]interface{}{"status": "active"}).
//	    Where("id = ?", 123).
//	    Execute()
//	if err != nil {
//	    return err
//	}
//
//	// For batch operations, use Builder()
//	_, err := db.Builder().
//	    BatchUpdate("users", "id").
//	    Set(1, map[string]interface{}{"status": "active"}).
//	    Set(2, map[string]interface{}{"status": "inactive"}).
//	    Execute()
func (d *DB) Update(table string) *UpdateQuery {
	return d.Builder().Update(table)
}

// Delete creates a new DELETE query.
//
// This is a convenience method equivalent to db.Builder().Delete(table).
//
// Example:
//
//	_, err := db.Delete("users").
//	    Where("id = ?", 123).
//	    Execute()
//	if err != nil {
//	    return err
//	}
//
//	// Delete multiple rows
//	_, err := db.Delete("users").
//	    Where("status = ?", "inactive").
//	    Execute()
func (d *DB) Delete(table string) *DeleteQuery {
	return d.Builder().Delete(table)
}

// Begin starts a transaction with default options.
//
// The transaction must be committed or rolled back to release resources.
// It's safe to call Rollback() even after Commit().
//
// Example:
//
//	tx, err := db.Begin(ctx)
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback() // Safe even after Commit
//
//	// Use transaction
//	_, err = tx.Builder().Insert("users", data).Execute()
//	if err != nil {
//	    return err
//	}
//
//	return tx.Commit()
func (d *DB) Begin(ctx context.Context) (*Tx, error) {
	coreTx, err := d.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: coreTx}, nil
}

// BeginTx starts a transaction with specified options.
//
// Options can specify isolation level and read-only mode:
//   - Isolation: sql.LevelReadUncommitted, sql.LevelReadCommitted,
//     sql.LevelRepeatableRead, sql.LevelSerializable
//   - ReadOnly: true for read-only transactions (some databases optimize these)
//
// Example:
//
//	opts := &relica.TxOptions{
//	    Isolation: sql.LevelSerializable,
//	    ReadOnly:  false,
//	}
//	tx, err := db.BeginTx(ctx, opts)
func (d *DB) BeginTx(ctx context.Context, opts *TxOptions) (*Tx, error) {
	coreTx, err := d.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: coreTx}, nil
}

// ExecContext executes a raw SQL query (INSERT/UPDATE/DELETE).
//
// This bypasses the query builder and executes SQL directly.
// Use this for queries that aren't supported by the query builder
// or when you need maximum control.
//
// Example:
//
//	result, err := db.ExecContext(ctx,
//	    "UPDATE users SET status = ? WHERE id = ?",
//	    1, 123)
//	if err != nil {
//	    return err
//	}
//	rowsAffected, _ := result.RowsAffected()
func (d *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

// QueryContext executes a raw SQL query and returns rows.
//
// This bypasses the query builder and executes SQL directly.
// You are responsible for closing the returned rows.
//
// Example:
//
//	rows, err := db.QueryContext(ctx,
//	    "SELECT * FROM users WHERE status = ?", 1)
//	if err != nil {
//	    return err
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//	    // Process rows
//	}
func (d *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a raw SQL query expected to return at most one row.
//
// This bypasses the query builder and executes SQL directly.
//
// Example:
//
//	var count int
//	err := db.QueryRowContext(ctx,
//	    "SELECT COUNT(*) FROM users").Scan(&count)
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return d.db.QueryRowContext(ctx, query, args...)
}

// QuoteTableName quotes a table name using the database's identifier quoting style.
//
// This is useful when building dynamic SQL queries.
//
// Example:
//
//	quoted := db.QuoteTableName("users")
//	// PostgreSQL: "users"
//	// MySQL: `users`
func (d *DB) QuoteTableName(table string) string {
	return d.db.QuoteTableName(table)
}

// QuoteColumnName quotes a column name using the database's identifier quoting style.
//
// This is useful when building dynamic SQL queries.
//
// Example:
//
//	quoted := db.QuoteColumnName("user_id")
//	// PostgreSQL: "user_id"
//	// MySQL: `user_id`
func (d *DB) QuoteColumnName(column string) string {
	return d.db.QuoteColumnName(column)
}

// GenerateParamName generates a unique parameter placeholder name.
//
// This is useful when building dynamic SQL queries.
//
// Example:
//
//	ph := db.GenerateParamName()
//	// Returns: p1, p2, p3, etc.
func (d *DB) GenerateParamName() string {
	return d.db.GenerateParamName()
}

// Unwrap returns the underlying core.DB for advanced use cases.
//
// This method is provided for edge cases where direct access to
// internal types is needed. Most users should not need this.
//
// Example:
//
//	coreDB := db.Unwrap()
//	// Use coreDB for advanced operations
func (d *DB) Unwrap() *core.DB {
	return d.db
}

// ============================================================================
// Tx Methods
// ============================================================================

// Builder returns the query builder for this transaction.
//
// All queries built using this builder will execute within the transaction.
// The builder automatically inherits the transaction's context.
//
// Example:
//
//	tx.Builder().Insert("users", data).Execute()
func (t *Tx) Builder() *QueryBuilder {
	return &QueryBuilder{qb: t.tx.Builder()}
}

// Select creates a new SELECT query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().Select(cols...).
//
// Example:
//
//	var users []User
//	err := tx.Select("*").From("users").Where("id = ?", 123).All(&users)
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
//
//	// For advanced features, use Builder()
//	err := tx.Builder().
//	    With("stats", statsQuery).
//	    Select("*").
//	    From("stats").
//	    All(&results)
func (t *Tx) Select(cols ...string) *SelectQuery {
	return t.Builder().Select(cols...)
}

// Insert creates a new INSERT query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().Insert(table, data).
//
// Example:
//
//	_, err := tx.Insert("users", map[string]interface{}{
//	    "name":  "Alice",
//	    "email": "alice@example.com",
//	}).Execute()
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
func (t *Tx) Insert(table string, data map[string]interface{}) *Query {
	return t.Builder().Insert(table, data)
}

// Update creates a new UPDATE query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().Update(table).
//
// Example:
//
//	_, err := tx.Update("users").
//	    Set(map[string]interface{}{"status": "active"}).
//	    Where("id = ?", 123).
//	    Execute()
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
func (t *Tx) Update(table string) *UpdateQuery {
	return t.Builder().Update(table)
}

// Delete creates a new DELETE query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().Delete(table).
//
// Example:
//
//	_, err := tx.Delete("users").Where("id = ?", 123).Execute()
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
func (t *Tx) Delete(table string) *DeleteQuery {
	return t.Builder().Delete(table)
}

// Commit commits the transaction.
//
// After calling Commit, the transaction cannot be used for further queries.
//
// Example:
//
//	if err := tx.Commit(); err != nil {
//	    return err
//	}
func (t *Tx) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction.
//
// After calling Rollback, the transaction cannot be used for further queries.
// It's safe to call Rollback even after Commit (it will be a no-op).
//
// Example:
//
//	defer tx.Rollback() // Safe even after Commit
func (t *Tx) Rollback() error {
	return t.tx.Rollback()
}

// Unwrap returns the underlying core.Tx for advanced use cases.
//
// This method is provided for edge cases where direct access to
// internal types is needed. Most users should not need this.
func (t *Tx) Unwrap() *core.Tx {
	return t.tx
}

// ============================================================================
// QueryBuilder Methods
// ============================================================================

// WithContext sets the context for all queries built by this builder.
//
// The context will be used for all subsequent query operations unless
// overridden at the query level.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	qb := db.Builder().WithContext(ctx)
//	qb.Select("*").From("users").All(&users)
func (qb *QueryBuilder) WithContext(ctx context.Context) *QueryBuilder {
	return &QueryBuilder{qb: qb.qb.WithContext(ctx)}
}

// Select starts a SELECT query with the specified columns.
//
// If no columns are provided, defaults to "*" (all columns).
//
// Example:
//
//	db.Builder().Select("id", "name", "email").From("users").All(&users)
func (qb *QueryBuilder) Select(cols ...string) *SelectQuery {
	return &SelectQuery{sq: qb.qb.Select(cols...)}
}

// Insert builds an INSERT query for a single row.
//
// The values parameter is a map of column names to values.
// Column order is deterministic (alphabetically sorted) for cache efficiency.
//
// Example:
//
//	result, err := db.Builder().Insert("users", map[string]interface{}{
//	    "name": "Alice",
//	    "email": "alice@example.com",
//	    "status": 1,
//	}).Execute()
func (qb *QueryBuilder) Insert(table string, values map[string]interface{}) *Query {
	return &Query{q: qb.qb.Insert(table, values)}
}

// Update creates an UPDATE query for the specified table.
//
// Use Set() to specify column values and Where() to filter rows.
//
// Example:
//
//	db.Builder().Update("users").
//	    Set(map[string]interface{}{"status": 2}).
//	    Where("id = ?", 123).
//	    Execute()
func (qb *QueryBuilder) Update(table string) *UpdateQuery {
	return &UpdateQuery{uq: qb.qb.Update(table)}
}

// Delete creates a DELETE query for the specified table.
//
// Use Where() to filter rows to delete.
//
// Example:
//
//	db.Builder().Delete("users").
//	    Where("id = ?", 123).
//	    Execute()
func (qb *QueryBuilder) Delete(table string) *DeleteQuery {
	return &DeleteQuery{dq: qb.qb.Delete(table)}
}

// BatchInsert creates a batch INSERT query for multiple rows.
//
// This is 3.3x faster than individual INSERTs for 100 rows.
// Use Values() or ValuesMap() to add rows.
//
// Example:
//
//	db.Builder().BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").
//	    Values("Bob", "bob@example.com").
//	    Execute()
func (qb *QueryBuilder) BatchInsert(table string, columns []string) *BatchInsertQuery {
	return &BatchInsertQuery{biq: qb.qb.BatchInsert(table, columns)}
}

// BatchUpdate creates a batch UPDATE query for multiple rows.
//
// This is 2.5x faster than individual UPDATEs for 100 rows.
// Uses CASE-WHEN logic to update multiple rows with different values.
//
// Example:
//
//	db.Builder().BatchUpdate("users", "id").
//	    Set(1, map[string]interface{}{"status": 2}).
//	    Set(2, map[string]interface{}{"status": 3}).
//	    Execute()
func (qb *QueryBuilder) BatchUpdate(table, keyColumn string) *BatchUpdateQuery {
	return &BatchUpdateQuery{buq: qb.qb.BatchUpdate(table, keyColumn)}
}

// Upsert creates an UPSERT query (INSERT with conflict resolution).
//
// Supported strategies:
//   - PostgreSQL/SQLite: ON CONFLICT ... DO UPDATE
//   - MySQL: ON DUPLICATE KEY UPDATE
//
// Example:
//
//	db.Builder().Upsert("users", map[string]interface{}{
//	    "id": 1,
//	    "name": "Alice",
//	    "email": "alice@example.com",
//	}).OnConflict("id").DoUpdate("name", "email").Execute()
func (qb *QueryBuilder) Upsert(table string, values map[string]interface{}) *UpsertQuery {
	return &UpsertQuery{uq: qb.qb.Upsert(table, values)}
}

// Unwrap returns the underlying core.QueryBuilder for advanced use cases.
//
// This method is provided for edge cases where direct access to
// internal types is needed. Most users should not need this.
func (qb *QueryBuilder) Unwrap() *core.QueryBuilder {
	return qb.qb
}

// ============================================================================
// SelectQuery Methods (40+ methods)
// ============================================================================

// WithContext sets the context for this SELECT query.
//
// This overrides any context set on the QueryBuilder.
//
// Example:
//
//	sq.WithContext(ctx).All(&users)
func (sq *SelectQuery) WithContext(ctx context.Context) *SelectQuery {
	return &SelectQuery{sq: sq.sq.WithContext(ctx)}
}

// From specifies the table to select from.
//
// Supports table aliases: From("users u")
//
// Example:
//
//	db.Builder().Select("*").From("users").All(&users)
func (sq *SelectQuery) From(table string) *SelectQuery {
	sq.sq.From(table)
	return sq
}

// FromSelect specifies a subquery as the FROM source.
//
// The alias parameter is required for the subquery.
//
// Example:
//
//	sub := db.Builder().Select("user_id", "COUNT(*) as cnt").
//	    From("orders").GroupBy("user_id")
//	db.Builder().Select("*").FromSelect(sub, "order_counts").
//	    Where("cnt > ?", 10).All(&results)
func (sq *SelectQuery) FromSelect(subquery *SelectQuery, alias string) *SelectQuery {
	sq.sq.FromSelect(subquery.sq, alias)
	return sq
}

// SelectExpr adds a raw SQL expression to the SELECT clause.
//
// Useful for scalar subqueries, window functions, or complex expressions.
//
// Example:
//
//	db.Builder().Select("id", "name").
//	    SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id)", "order_count").
//	    From("users").All(&results)
func (sq *SelectQuery) SelectExpr(expr string, args ...interface{}) *SelectQuery {
	sq.sq.SelectExpr(expr, args...)
	return sq
}

// Where adds a WHERE condition.
//
// Accepts either a string with placeholders or an Expression.
// Multiple Where() calls are combined with AND.
//
// String example:
//
//	Where("status = ? AND age > ?", 1, 18)
//
// Expression example:
//
//	Where(relica.And(
//	    relica.Eq("status", 1),
//	    relica.GreaterThan("age", 18),
//	))
func (sq *SelectQuery) Where(condition interface{}, params ...interface{}) *SelectQuery {
	sq.sq.Where(condition, params...)
	return sq
}

// InnerJoin adds an INNER JOIN clause.
//
// Example:
//
//	db.Builder().Select("u.name", "o.total").
//	    From("users u").
//	    InnerJoin("orders o", "o.user_id = u.id").
//	    All(&results)
func (sq *SelectQuery) InnerJoin(table string, on interface{}) *SelectQuery {
	sq.sq.InnerJoin(table, on)
	return sq
}

// LeftJoin adds a LEFT JOIN clause.
//
// Example:
//
//	db.Builder().Select("u.name", "o.total").
//	    From("users u").
//	    LeftJoin("orders o", "o.user_id = u.id").
//	    All(&results)
func (sq *SelectQuery) LeftJoin(table string, on interface{}) *SelectQuery {
	sq.sq.LeftJoin(table, on)
	return sq
}

// RightJoin adds a RIGHT JOIN clause.
//
// Example:
//
//	db.Builder().Select("u.name", "o.total").
//	    From("users u").
//	    RightJoin("orders o", "o.user_id = u.id").
//	    All(&results)
func (sq *SelectQuery) RightJoin(table string, on interface{}) *SelectQuery {
	sq.sq.RightJoin(table, on)
	return sq
}

// FullJoin adds a FULL OUTER JOIN clause.
//
// Note: Not supported by MySQL.
//
// Example:
//
//	db.Builder().Select("u.name", "o.total").
//	    From("users u").
//	    FullJoin("orders o", "o.user_id = u.id").
//	    All(&results)
func (sq *SelectQuery) FullJoin(table string, on interface{}) *SelectQuery {
	sq.sq.FullJoin(table, on)
	return sq
}

// CrossJoin adds a CROSS JOIN clause (Cartesian product).
//
// Example:
//
//	db.Builder().Select("*").
//	    From("colors").
//	    CrossJoin("sizes").
//	    All(&results)
func (sq *SelectQuery) CrossJoin(table string) *SelectQuery {
	sq.sq.CrossJoin(table)
	return sq
}

// OrderBy adds ORDER BY clause with optional direction (ASC/DESC).
//
// Supports multiple columns. Multiple OrderBy() calls are additive.
//
// Example:
//
//	OrderBy("age DESC", "name ASC")
func (sq *SelectQuery) OrderBy(columns ...string) *SelectQuery {
	sq.sq.OrderBy(columns...)
	return sq
}

// Limit sets the LIMIT clause.
//
// Example:
//
//	Limit(100)  // Return at most 100 rows
func (sq *SelectQuery) Limit(limit int64) *SelectQuery {
	sq.sq.Limit(limit)
	return sq
}

// Offset sets the OFFSET clause.
//
// Example:
//
//	Offset(200)  // Skip first 200 rows
func (sq *SelectQuery) Offset(offset int64) *SelectQuery {
	sq.sq.Offset(offset)
	return sq
}

// GroupBy adds GROUP BY clause.
//
// Multiple columns supported. Multiple GroupBy() calls are additive.
//
// Example:
//
//	GroupBy("user_id", "status")
func (sq *SelectQuery) GroupBy(columns ...string) *SelectQuery {
	sq.sq.GroupBy(columns...)
	return sq
}

// Having adds HAVING clause (WHERE for aggregates).
//
// Accepts string or Expression. Multiple calls are combined with AND.
//
// Example:
//
//	Having("COUNT(*) > ?", 100)
func (sq *SelectQuery) Having(condition interface{}, args ...interface{}) *SelectQuery {
	sq.sq.Having(condition, args...)
	return sq
}

// Union combines this query with another using UNION (removes duplicates).
//
// Example:
//
//	q1 := db.Builder().Select("name").From("users")
//	q2 := db.Builder().Select("name").From("archived_users")
//	q1.Union(q2).All(&names)
func (sq *SelectQuery) Union(other *SelectQuery) *SelectQuery {
	sq.sq.Union(other.sq)
	return sq
}

// UnionAll combines this query with another using UNION ALL (keeps duplicates).
//
// Example:
//
//	q1 := db.Builder().Select("id").From("orders_2023")
//	q2 := db.Builder().Select("id").From("orders_2024")
//	q1.UnionAll(q2).All(&orderIDs)
func (sq *SelectQuery) UnionAll(other *SelectQuery) *SelectQuery {
	sq.sq.UnionAll(other.sq)
	return sq
}

// Intersect combines queries using INTERSECT (rows in both).
//
// Database support: PostgreSQL 9.1+, MySQL 8.0.31+, SQLite 3.25+
//
// Example:
//
//	q1 := db.Builder().Select("id").From("users")
//	q2 := db.Builder().Select("user_id").From("orders")
//	q1.Intersect(q2).All(&ids)  // Users who have placed orders
func (sq *SelectQuery) Intersect(other *SelectQuery) *SelectQuery {
	sq.sq.Intersect(other.sq)
	return sq
}

// Except combines queries using EXCEPT (rows in first but not second).
//
// Database support: PostgreSQL 9.1+, MySQL 8.0.31+, SQLite 3.25+
//
// Example:
//
//	q1 := db.Builder().Select("id").From("all_users")
//	q2 := db.Builder().Select("user_id").From("banned_users")
//	q1.Except(q2).All(&activeUsers)
func (sq *SelectQuery) Except(other *SelectQuery) *SelectQuery {
	sq.sq.Except(other.sq)
	return sq
}

// With adds a Common Table Expression (CTE).
//
// Example:
//
//	cte := db.Builder().Select("user_id", "SUM(total) as total").
//	    From("orders").GroupBy("user_id")
//	db.Builder().Select("*").With("order_totals", cte).
//	    From("order_totals").Where("total > ?", 1000).All(&users)
func (sq *SelectQuery) With(name string, query *SelectQuery) *SelectQuery {
	sq.sq.With(name, query.sq)
	return sq
}

// WithRecursive adds a recursive Common Table Expression.
//
// The query MUST use UNION or UNION ALL.
// Database support: PostgreSQL (all), MySQL 8.0+, SQLite 3.25+
//
// Example:
//
//	anchor := db.Builder().Select("id", "name", "manager_id", "1 as level").
//	    From("employees").Where("manager_id IS NULL")
//	recursive := db.Builder().Select("e.id", "e.name", "e.manager_id", "h.level + 1").
//	    From("employees e").InnerJoin("hierarchy h", "e.manager_id = h.id")
//	cte := anchor.UnionAll(recursive)
//	db.Builder().Select("*").WithRecursive("hierarchy", cte).
//	    From("hierarchy").OrderBy("level", "name").All(&employees)
func (sq *SelectQuery) WithRecursive(name string, query *SelectQuery) *SelectQuery {
	sq.sq.WithRecursive(name, query.sq)
	return sq
}

// Build constructs the Query object from SelectQuery.
//
// Example:
//
//	q := db.Builder().Select("*").From("users").Where("id = ?", 123).Build()
//	sql, params := q.SQL(), q.Params()
func (sq *SelectQuery) Build() *Query {
	return &Query{q: sq.sq.Build()}
}

// One scans a single row into dest.
//
// Returns sql.ErrNoRows if no row is found.
//
// Example:
//
//	var user User
//	err := db.Builder().Select("*").From("users").
//	    Where("id = ?", 123).One(&user)
func (sq *SelectQuery) One(dest interface{}) error {
	return sq.sq.One(dest)
}

// All scans all rows into dest slice.
//
// Example:
//
//	var users []User
//	err := db.Builder().Select("*").From("users").All(&users)
func (sq *SelectQuery) All(dest interface{}) error {
	return sq.sq.All(dest)
}

// AsExpression converts a SelectQuery to an Expression for subquery use.
//
// Example:
//
//	sub := db.Builder().Select("user_id").From("orders").Where("total > ?", 100)
//	db.Builder().Select("*").From("users").
//	    Where(relica.In("id", sub.AsExpression())).All(&users)
func (sq *SelectQuery) AsExpression() Expression {
	return sq.sq.AsExpression()
}

// Unwrap returns the underlying core.SelectQuery for advanced use cases.
//
// This method is provided for edge cases where direct access to
// internal types is needed. Most users should not need this.
func (sq *SelectQuery) Unwrap() *core.SelectQuery {
	return sq.sq
}

// ============================================================================
// UpdateQuery Methods
// ============================================================================

// UpdateQuery represents an UPDATE query being built.
type UpdateQuery struct {
	uq *core.UpdateQuery
}

// WithContext sets the context for this UPDATE query.
func (uq *UpdateQuery) WithContext(ctx context.Context) *UpdateQuery {
	return &UpdateQuery{uq: uq.uq.WithContext(ctx)}
}

// Set specifies the columns and values to update.
//
// Example:
//
//	Update("users").Set(map[string]interface{}{"status": 2})
func (uq *UpdateQuery) Set(values map[string]interface{}) *UpdateQuery {
	uq.uq.Set(values)
	return uq
}

// Where adds a WHERE condition to the UPDATE query.
//
// Example:
//
//	Update("users").Set(...).Where("id = ?", 123)
func (uq *UpdateQuery) Where(condition interface{}, params ...interface{}) *UpdateQuery {
	uq.uq.Where(condition, params...)
	return uq
}

// Build constructs the Query object.
func (uq *UpdateQuery) Build() *Query {
	return &Query{q: uq.uq.Build()}
}

// Execute executes the UPDATE query.
func (uq *UpdateQuery) Execute() (sql.Result, error) {
	return uq.Build().Execute()
}

// ============================================================================
// DeleteQuery Methods
// ============================================================================

// DeleteQuery represents a DELETE query being built.
type DeleteQuery struct {
	dq *core.DeleteQuery
}

// WithContext sets the context for this DELETE query.
func (dq *DeleteQuery) WithContext(ctx context.Context) *DeleteQuery {
	return &DeleteQuery{dq: dq.dq.WithContext(ctx)}
}

// Where adds a WHERE condition to the DELETE query.
//
// Example:
//
//	Delete("users").Where("id = ?", 123)
func (dq *DeleteQuery) Where(condition interface{}, params ...interface{}) *DeleteQuery {
	dq.dq.Where(condition, params...)
	return dq
}

// Build constructs the Query object.
func (dq *DeleteQuery) Build() *Query {
	return &Query{q: dq.dq.Build()}
}

// Execute executes the DELETE query.
func (dq *DeleteQuery) Execute() (sql.Result, error) {
	return dq.Build().Execute()
}

// ============================================================================
// UpsertQuery Methods
// ============================================================================

// UpsertQuery represents an UPSERT query being built.
type UpsertQuery struct {
	uq *core.UpsertQuery
}

// WithContext sets the context for this UPSERT query.
func (uq *UpsertQuery) WithContext(ctx context.Context) *UpsertQuery {
	return &UpsertQuery{uq: uq.uq.WithContext(ctx)}
}

// OnConflict specifies the columns that determine a conflict.
//
// Example:
//
//	Upsert(...).OnConflict("id", "email")
func (uq *UpsertQuery) OnConflict(columns ...string) *UpsertQuery {
	uq.uq.OnConflict(columns...)
	return uq
}

// DoUpdate specifies which columns to update on conflict.
//
// Example:
//
//	Upsert(...).OnConflict("id").DoUpdate("name", "email")
func (uq *UpsertQuery) DoUpdate(columns ...string) *UpsertQuery {
	uq.uq.DoUpdate(columns...)
	return uq
}

// DoNothing ignores conflicts (no update).
//
// Example:
//
//	Upsert(...).OnConflict("id").DoNothing()
func (uq *UpsertQuery) DoNothing() *UpsertQuery {
	uq.uq.DoNothing()
	return uq
}

// Build constructs the Query object.
func (uq *UpsertQuery) Build() *Query {
	return &Query{q: uq.uq.Build()}
}

// Execute executes the UPSERT query.
func (uq *UpsertQuery) Execute() (sql.Result, error) {
	return uq.Build().Execute()
}

// ============================================================================
// BatchInsertQuery Methods
// ============================================================================

// BatchInsertQuery represents a batch INSERT query being built.
type BatchInsertQuery struct {
	biq *core.BatchInsertQuery
}

// WithContext sets the context for this batch INSERT query.
func (biq *BatchInsertQuery) WithContext(ctx context.Context) *BatchInsertQuery {
	return &BatchInsertQuery{biq: biq.biq.WithContext(ctx)}
}

// Values adds a row of values to the batch insert.
//
// Example:
//
//	BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").
//	    Values("Bob", "bob@example.com")
func (biq *BatchInsertQuery) Values(values ...interface{}) *BatchInsertQuery {
	biq.biq.Values(values...)
	return biq
}

// ValuesMap adds a row from a map.
//
// Example:
//
//	BatchInsert("users", []string{"name", "email"}).
//	    ValuesMap(map[string]interface{}{"name": "Alice", "email": "alice@example.com"})
func (biq *BatchInsertQuery) ValuesMap(values map[string]interface{}) *BatchInsertQuery {
	biq.biq.ValuesMap(values)
	return biq
}

// Build constructs the Query object.
func (biq *BatchInsertQuery) Build() *Query {
	return &Query{q: biq.biq.Build()}
}

// Execute executes the batch INSERT query.
func (biq *BatchInsertQuery) Execute() (sql.Result, error) {
	return biq.Build().Execute()
}

// ============================================================================
// BatchUpdateQuery Methods
// ============================================================================

// BatchUpdateQuery represents a batch UPDATE query being built.
type BatchUpdateQuery struct {
	buq *core.BatchUpdateQuery
}

// WithContext sets the context for this batch UPDATE query.
func (buq *BatchUpdateQuery) WithContext(ctx context.Context) *BatchUpdateQuery {
	return &BatchUpdateQuery{buq: buq.buq.WithContext(ctx)}
}

// Set adds a row update to the batch.
//
// Example:
//
//	BatchUpdate("users", "id").
//	    Set(1, map[string]interface{}{"status": 2}).
//	    Set(2, map[string]interface{}{"status": 3})
func (buq *BatchUpdateQuery) Set(keyValue interface{}, values map[string]interface{}) *BatchUpdateQuery {
	buq.buq.Set(keyValue, values)
	return buq
}

// Build constructs the Query object.
func (buq *BatchUpdateQuery) Build() *Query {
	return &Query{q: buq.buq.Build()}
}

// Execute executes the batch UPDATE query.
func (buq *BatchUpdateQuery) Execute() (sql.Result, error) {
	return buq.Build().Execute()
}

// ============================================================================
// Query Methods
// ============================================================================

// Execute runs the query and returns results.
func (q *Query) Execute() (sql.Result, error) {
	result, err := q.q.Execute()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// One fetches a single row into dest.
func (q *Query) One(dest interface{}) error {
	return q.q.One(dest)
}

// All fetches all rows into dest slice.
func (q *Query) All(dest interface{}) error {
	return q.q.All(dest)
}

// ============================================================================
// Re-export configuration options
// ============================================================================

// WithMaxOpenConns sets the maximum number of open connections.
var WithMaxOpenConns = core.WithMaxOpenConns

// WithMaxIdleConns sets the maximum number of idle connections.
var WithMaxIdleConns = core.WithMaxIdleConns

// WithConnMaxLifetime sets the maximum amount of time a connection may be reused.
// Expired connections may be closed lazily before reuse.
// If duration <= 0, connections are not closed due to a connection's age.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithConnMaxLifetime(5*time.Minute))
var WithConnMaxLifetime = core.WithConnMaxLifetime

// WithConnMaxIdleTime sets the maximum amount of time a connection may be idle.
// Expired connections may be closed lazily before reuse.
// If duration <= 0, connections are not closed due to a connection's idle time.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithConnMaxIdleTime(1*time.Minute))
var WithConnMaxIdleTime = core.WithConnMaxIdleTime

// WithHealthCheck enables periodic health checks on database connections.
// The health checker pings the database at the specified interval to detect dead connections.
// If interval <= 0, health checks are disabled.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithHealthCheck(30*time.Second))
var WithHealthCheck = core.WithHealthCheck

// WithStmtCacheCapacity sets the prepared statement cache capacity.
var WithStmtCacheCapacity = core.WithStmtCacheCapacity

// WithLogger sets the logger for database query logging.
// If not set, a NoopLogger is used (zero overhead when logging is disabled).
//
// Example:
//
//	import "log/slog"
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithLogger(logger.NewSlogAdapter(logger)))
var WithLogger = core.WithLogger

// WithTracer sets the distributed tracer for query observability.
// If not set, a NoopTracer is used (zero overhead when tracing is disabled).
//
// Example:
//
//	import "go.opentelemetry.io/otel"
//	tracer := otel.Tracer("relica")
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithTracer(tracer.NewOtelTracer(tracer)))
var WithTracer = core.WithTracer

// WithSensitiveFields sets the list of sensitive field names for parameter masking.
// If not set, default sensitive field patterns are used.
//
// Example:
//
//	db, err := relica.Open("postgres", dsn,
//	    relica.WithSensitiveFields([]string{"password", "token", "api_key"}))
var WithSensitiveFields = core.WithSensitiveFields

// Logger defines the logging interface for Relica.
// Implementations should handle structured logging with key-value pairs.
type Logger = logger.Logger

// NoopLogger is a logger that does nothing (zero overhead when logging is disabled).
type NoopLogger = logger.NoopLogger

// SlogAdapter wraps log/slog.Logger to implement the Logger interface.
type SlogAdapter = logger.SlogAdapter

// NewSlogAdapter creates a new logger adapter wrapping an slog.Logger.
var NewSlogAdapter = logger.NewSlogAdapter

// Tracer defines the tracing interface for Relica.
type Tracer = tracer.Tracer

// NoopTracer is a tracer that does nothing (zero overhead when tracing is disabled).
type NoopTracer = tracer.NoopTracer

// OtelTracer wraps an OpenTelemetry tracer to implement the Tracer interface.
type OtelTracer = tracer.OtelTracer

// NewOtelTracer creates a new OpenTelemetry tracer adapter.
var NewOtelTracer = tracer.NewOtelTracer

// ============================================================================
// Re-export expression builders
// ============================================================================

// NewExp creates a new raw SQL expression.
var NewExp = core.NewExp

// Eq creates an equality expression (column = value).
var Eq = core.Eq

// NotEq creates a not-equal expression (column != value).
var NotEq = core.NotEq

// GreaterThan creates a greater-than expression (column > value).
var GreaterThan = core.GreaterThan

// LessThan creates a less-than expression (column < value).
var LessThan = core.LessThan

// GreaterOrEqual creates a greater-or-equal expression (column >= value).
var GreaterOrEqual = core.GreaterOrEqual

// LessOrEqual creates a less-or-equal expression (column <= value).
var LessOrEqual = core.LessOrEqual

// In creates an IN expression (column IN (values...)).
var In = core.In

// NotIn creates a NOT IN expression (column NOT IN (values...)).
var NotIn = core.NotIn

// Between creates a BETWEEN expression (column BETWEEN low AND high).
var Between = core.Between

// NotBetween creates a NOT BETWEEN expression.
var NotBetween = core.NotBetween

// Like creates a LIKE expression with automatic escaping.
var Like = core.Like

// NotLike creates a NOT LIKE expression.
var NotLike = core.NotLike

// OrLike creates a LIKE expression combined with OR.
var OrLike = core.OrLike

// OrNotLike creates a NOT LIKE expression combined with OR.
var OrNotLike = core.OrNotLike

// And combines expressions with AND.
var And = core.And

// Or combines expressions with OR.
var Or = core.Or

// Not negates an expression.
var Not = core.Not

// Exists creates an EXISTS subquery expression.
var Exists = core.Exists

// NotExists creates a NOT EXISTS subquery expression.
var NotExists = core.NotExists
