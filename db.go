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
//	err = db.Select("id", "name").From("users").All(&users)
//
// # Features
//
// CRUD Operations:
//
//	// SELECT with Expression API
//	db.Select().From("users").Where(relica.Eq("id", 123)).One(&user)
//
//	// INSERT via Model API (recommended)
//	user := User{Name: "Alice", Email: "alice@example.com"}
//	db.Model(&user).Insert()
//
//	// UPDATE
//	db.Update("users").
//	    Set(map[string]any{"status": "active"}).
//	    Where(relica.Eq("id", 123)).
//	    Execute()
//
//	// DELETE
//	db.Delete("users").Where(relica.Eq("id", 123)).Execute()
//
// # Classifying database errors
//
// [IsUniqueViolation], [IsForeignKeyViolation], [IsNotNullViolation],
// [IsCheckViolation], [IsDeadlock], [IsSerializationFailure] and [IsLockTimeout]
// classify an error by the code the driver attached to it, wherever it sits in
// the error tree (fmt.Errorf %w chains and errors.Join included):
//
//   - PostgreSQL SQLSTATE — pgx v5 and lib/pq (every version);
//   - MySQL/MariaDB error number — go-sql-driver/mysql;
//   - SQLite extended result code — modernc.org/sqlite and mattn/go-sqlite3.
//
// The first error in the tree that carries a code decides; the message text is
// consulted only when no code is present (other drivers). Codes do not depend
// on the server language, so the result is the same with a localized
// PostgreSQL (lc_messages) or a MariaDB server. Relica imports no driver: codes
// are read through the SQLState() method or, for the driver types listed
// above, by reflection.
//
// Retry the whole transaction, never a single statement, when IsDeadlock or
// IsSerializationFailure is true — the server has already rolled it back:
//
//	for attempt := 1; ; attempt++ {
//	    err := db.Transactional(ctx, transfer)
//	    if err == nil || attempt == 3 ||
//	        !(relica.IsDeadlock(err) || relica.IsSerializationFailure(err)) {
//	        return err
//	    }
//	    select { // jittered backoff, so the competing transactions spread out
//	    case <-time.After(time.Duration(10+rand.IntN(40*attempt)) * time.Millisecond):
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    }
//	}
package relica

import (
	"context"
	"database/sql"
	"time"

	"github.com/coregx/relica/internal/core"
	"github.com/coregx/relica/internal/util"
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

// QueryPlan is a database-independent summary of an execution plan, returned
// by [SelectQuery.Explain] and [SelectQuery.ExplainAnalyze].
//
// Fields populated by EXPLAIN (always available):
//   - Cost, EstimatedRows, UsesIndex, IndexName, FullScan, RawOutput, Database
//
// Fields populated only by EXPLAIN ANALYZE (require actual execution):
//   - ActualRows, ActualTime, BuffersHit, BuffersMiss, RowsExamined, RowsProduced
type QueryPlan struct {
	// Cost is the estimated query cost in database-specific units.
	Cost float64
	// EstimatedRows is the estimated number of rows to be processed.
	EstimatedRows int64
	// ActualRows is the number of rows actually processed (EXPLAIN ANALYZE only, else 0).
	ActualRows int64
	// ActualTime is the actual execution time (EXPLAIN ANALYZE only, else 0).
	ActualTime time.Duration

	// UsesIndex is true if the plan uses any index.
	UsesIndex bool
	// IndexName is the primary index used (empty if several or none).
	IndexName string
	// FullScan is true if a full table scan is performed.
	FullScan bool

	// RawOutput is the full EXPLAIN output from the database.
	RawOutput string
	// Database is "postgres", "mysql" or "sqlite".
	Database string

	// BuffersHit is the number of buffer cache hits (PostgreSQL, EXPLAIN ANALYZE).
	BuffersHit int64
	// BuffersMiss is the number of buffer cache misses (PostgreSQL, EXPLAIN ANALYZE).
	BuffersMiss int64
	// RowsExamined is the number of rows examined (MySQL).
	RowsExamined int64
	// RowsProduced is the number of rows produced (MySQL).
	RowsProduced int64
}

// queryPlan converts an internal plan (nil stays nil).
func queryPlan(p *core.QueryPlan) *QueryPlan {
	if p == nil {
		return nil
	}
	plan := QueryPlan(*p)
	return &plan
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

// core returns the wrapped query, or nil for a nil *SelectQuery argument, so
// that core reports a build error instead of the wrapper panicking.
func (sq *SelectQuery) core() *core.SelectQuery {
	if sq == nil {
		return nil
	}
	return sq.sq
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
//	_, err = tx.Insert("users", data).Execute()
//	if err != nil {
//	    return err
//	}
//
//	return tx.Commit()
type Tx struct {
	tx *core.Tx
}

// Executor is the set of query-building and raw-SQL methods shared by *DB and
// *Tx. Code that must run either inside or outside a transaction without
// knowing which — repositories, the dbcontext pattern — should accept an
// Executor instead of a concrete type.
//
// It plays the role of dbx.Builder in ozzo-dbx: a single return type for
// "the transaction from the context, or the plain connection":
//
//	func (d *DB) With(ctx context.Context) relica.Executor {
//	    if tx, ok := ctx.Value(txKey).(*relica.Tx); ok {
//	        return tx
//	    }
//	    return d.db.WithContext(ctx)
//	}
//
// Lifecycle methods (Begin, Commit, Rollback, Transactional) are deliberately
// absent: a holder of an Executor must not be able to end a transaction it did
// not start. WithContext is absent because a *Tx is already bound to the
// context passed to Begin.
//
// The method set is frozen. Methods added to *DB and *Tx in the future are not
// added here, so external implementations (test doubles) keep compiling.
type Executor interface {
	Select(cols ...string) *SelectQuery
	Insert(table string, data map[string]any) *Query
	InsertStruct(table string, data any) *Query
	BatchInsertStruct(table string, data any) *Query
	Update(table string) *UpdateQuery
	UpdateStruct(table string, data any) *UpdateQuery
	Delete(table string) *DeleteQuery
	Upsert(table string, values map[string]any) *UpsertQuery
	BatchInsert(table string, columns []string) *BatchInsertQuery
	BatchUpdate(table, keyColumn string) *BatchUpdateQuery
	Model(model any) *ModelQuery
	NewQuery(query string) *Query

	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Compile-time guarantees that both execution contexts satisfy Executor.
// If a method is ever removed from either type, the build fails here.
var (
	_ Executor = (*DB)(nil)
	_ Executor = (*Tx)(nil)
)

// ModelQuery provides CRUD operations for struct models.
//
// ModelQuery simplifies database operations by automatically inferring
// table names and primary keys from struct definitions, similar to ozzo-dbx.
//
// The table name is determined by (ADR-018):
//  1. Table("x") on the query
//  2. a TableName() string method on the model type, value or pointer receiver
//  3. otherwise the DB's Naming (WithNaming); default snake_case singular:
//     UserProfile → user_profile, User → user
//
// Columns: the db tag, otherwise the DB's Naming (default snake_case of the
// field name: CreatedAt → created_at). db:"-" skips a field; embedded structs
// are flattened.
//
// The primary key is:
//  1. every field tagged pk — db:"col,pk", db:",pk" or the legacy db:"pk" —
//     in declaration order (several = composite key)
//  2. otherwise the field named ID, then Id (embedded fields included)
//
// Example:
//
//	type User struct {
//	    ID    int    `db:"id"`
//	    Name  string `db:"name"`
//	    Email string `db:"email"`
//	}
//
//	func (User) TableName() string { return "users" }
//
//	user := User{Name: "Alice", Email: "alice@example.com"}
//	err := db.Model(&user).Insert()
//
//	user.Status = "active"
//	err = db.Model(&user).Update() // Auto WHERE by primary key.
//
//	err = db.Model(&user).Delete() // Auto WHERE by primary key.
type ModelQuery struct {
	mq *core.ModelQuery
}

// BeforeInserter is implemented by models that need pre-insert logic.
// If a struct implements this interface, Model().Insert() and Model().Upsert()
// call BeforeInsert() before autoid generation and query execution.
//
// Use cases: setting timestamps, generating custom IDs, validation.
//
// Example:
//
//	type User struct {
//	    ID        int64     `db:"id,pk"`
//	    PublicID  string    `db:"public_id,autoid:usr"`
//	    CreatedAt time.Time `db:"created_at"`
//	}
//
//	func (u *User) BeforeInsert() error {
//	    u.CreatedAt = time.Now()
//	    return nil
//	}
//
// An error returned by BeforeInsert aborts the insert and is returned to the
// caller; nothing is sent to the database.
type BeforeInserter interface {
	BeforeInsert() error
}

// The public and internal hook interfaces must stay interchangeable: the
// internal Model API detects the hook with its own interface.
var (
	_ core.BeforeInserter = BeforeInserter(nil)
	_ BeforeInserter      = core.BeforeInserter(nil)
)

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
	q   *core.Query
	err error // Error from query construction (e.g., struct conversion).
}

// ScanMode overrides the DB's ScanMode (see WithScanMode) for this query.
func (q *Query) ScanMode(m ScanMode) *Query {
	if q.q != nil {
		q.q.ScanMode(core.ScanMode(m))
	}
	return q
}

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
	coreDB, err := core.Open(driverName, dsn, coreOptions(opts)...)
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
//	db := relica.WrapDB(sqlDB, "postgres", relica.WithQueryHook(hook))
//	defer sqlDB.Close() // Caller's responsibility
//
// Options apply as in Open: hooks, logger, Naming, ScanMode, statement cache,
// health check. Pool options (WithMaxOpenConns, …) change the caller's
// *sql.DB. WrapDB panics if sqlDB is nil or the driver name has no dialect —
// programmer errors, like a bad pattern in regexp.MustCompile.
func WrapDB(sqlDB *sql.DB, driverName string, opts ...Option) *DB {
	coreDB := core.WrapDB(sqlDB, driverName, coreOptions(opts)...)
	return &DB{db: coreDB}
}

// WrapTx adapts an existing *sql.Tx so Relica's transaction API — and the
// Executor interface — can run on a transaction that was started elsewhere
// (another library, a migration tool, sqlc). This is the counterpart of
// ozzo-dbx's DB.Wrap.
//
// ctx is attached to every query built from the returned Tx; pass the context
// the transaction was begun with. Ownership stays with the caller: Commit and
// Rollback may be called on the returned Tx or on the original *sql.Tx, both
// act on the same transaction. Panics if sqlTx is nil (programmer error, same
// as WrapDB).
//
// Example:
//
//	sqlTx, _ := sqlDB.BeginTx(ctx, nil)
//	tx := db.WrapTx(ctx, sqlTx)
//	if err := repo.Save(ctx, tx, &user); err != nil { // repo takes relica.Executor
//	    return sqlTx.Rollback()
//	}
//	return sqlTx.Commit()
func (d *DB) WrapTx(ctx context.Context, sqlTx *sql.Tx) *Tx {
	return &Tx{tx: d.db.WrapTx(ctx, sqlTx)}
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

// SqlDB returns the underlying *sql.DB connection.
// Useful for connection pool tuning, health checks, or database/sql methods.
// Do NOT call Close() on the returned value — use [DB.Close] instead.
func (d *DB) SqlDB() *sql.DB {
	return d.db.SqlDB()
}

// PingContext verifies the database connection is alive.
// Use for health checks and connection validation at startup.
func (d *DB) PingContext(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// DriverName returns the database driver name (e.g., "postgres", "mysql", "sqlite3").
func (d *DB) DriverName() string {
	return d.db.DriverName()
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

// WarmCache pre-warms the statement cache by preparing frequently-used queries.
//
// This improves performance at startup by avoiding cache misses for common queries.
// The queries are prepared synchronously in the order provided.
// Returns the number of successfully prepared queries and any error encountered.
//
// The cache is keyed by the exact SQL a statement sends — the dialect's
// quoting and placeholders ($1 on PostgreSQL) included — so pass the text
// ToSQL returns for the builder queries you want warm. Hand-written text such
// as "SELECT * FROM users WHERE id = ?" never matches a builder query, and on
// PostgreSQL it does not even prepare.
//
// Example:
//
//	byID, _ := db.Select().From("users").Where(relica.Eq("id", 0)).ToSQL()
//	byEmail, _ := db.Select().From("users").Where(relica.Eq("email", "")).ToSQL()
//	n, err := db.WarmCache([]string{byID, byEmail})
//	if err != nil {
//	    log.Warn("Failed to warm cache", "error", err, "warmed", n)
//	}
func (d *DB) WarmCache(queries []string) (int, error) {
	return d.db.WarmCache(queries)
}

// PinQuery marks a query as pinned in the statement cache, preventing eviction.
//
// Pinned queries remain in cache indefinitely, useful for frequently-used queries.
// Returns false if the query is not in cache (call WarmCache first).
//
// Example:
//
//	// Warm and pin critical queries (keys are the ToSQL text, see WarmCache)
//	byID, _ := db.Select().From("users").Where(relica.Eq("id", 0)).ToSQL()
//	db.WarmCache([]string{byID})
//	db.PinQuery(byID) // Will never be evicted
func (d *DB) PinQuery(query string) bool {
	return d.db.PinQuery(query)
}

// UnpinQuery removes the pin from a cached query, allowing normal LRU eviction.
//
// Returns false if the query is not in cache or not pinned.
//
// Example:
//
//	db.UnpinQuery("SELECT * FROM users WHERE id = ?")
func (d *DB) UnpinQuery(query string) bool {
	return d.db.UnpinQuery(query)
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

// NewQuery creates a raw SQL query for execution.
// Use this for queries that don't fit the query builder pattern,
// or when you need manual control over prepared statement lifecycle.
//
// Example:
//
//	var count int
//	err := db.NewQuery("SELECT COUNT(*) FROM users").Row(&count)
//
//	// With parameters
//	var user User
//	err := db.NewQuery("SELECT * FROM users WHERE id = ?").Bind(1).One(&user)
//
//	// With Prepare for repeated execution
//	q := db.NewQuery("SELECT * FROM users WHERE status = ?").Prepare()
//	defer q.Close()
//	for _, status := range statuses {
//	    q.Bind(status).All(&users)
//	}
func (d *DB) NewQuery(query string) *Query {
	return &Query{q: d.db.NewQuery(query)}
}

// Model creates a ModelQuery for performing CRUD operations on a struct model.
//
// The model must be a pointer to a struct. The table name and primary key
// are automatically inferred from the struct definition.
//
// Table name, columns and primary key are resolved as described on
// ModelQuery: TableName() (value or pointer receiver) or the DB's Naming
// (default snake_case singular, User → user); fields tagged pk, else ID/Id.
//
// Example:
//
//	type User struct {
//	    ID    int    `db:"id"`
//	    Name  string `db:"name"`
//	    Email string `db:"email"`
//	}
//
//	func (User) TableName() string { return "users" }
//
//	// INSERT - auto table name.
//	user := User{Name: "Alice", Email: "alice@example.com"}
//	err := db.Model(&user).Insert()
//
//	// UPDATE - auto WHERE by primary key.
//	user.Status = "active"
//	err = db.Model(&user).Update()
//
//	// DELETE - auto WHERE by primary key.
//	err = db.Model(&user).Delete()
//
//	// Field control.
//	err = db.Model(&user).Exclude("CreatedAt", "UpdatedAt").Insert()
//	err = db.Model(&user).Table("users_archive").Insert()
func (d *DB) Model(model any) *ModelQuery {
	return &ModelQuery{mq: d.db.Model(model)}
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
//	result, err := db.Insert("users", map[string]any{
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
func (d *DB) Insert(table string, data map[string]any) *Query {
	return d.Builder().Insert(table, data)
}

// InsertStruct builds an INSERT query from a struct using db tags.
//
// The struct fields are mapped to database columns using the `db` struct tag.
// Fields without a `db` tag use the field name.
// Fields tagged with `db:"-"` are ignored.
// Unexported fields are automatically skipped.
//
// This method provides type-safe struct-based inserts without manually
// constructing maps. For batch struct inserts, use BatchInsertStruct.
//
// Example:
//
//	type User struct {
//	    ID    int    `db:"id"`
//	    Name  string `db:"name"`
//	    Email string `db:"email"`
//	    Skip  int    `db:"-"`  // ignored
//	}
//
//	user := User{Name: "Alice", Email: "alice@example.com"}
//	result, err := db.InsertStruct("users", &user).Execute()
//	if err != nil {
//	    return err
//	}
func (d *DB) InsertStruct(table string, data any) *Query {
	return d.Builder().InsertStruct(table, data)
}

// BatchInsertStruct builds a batch INSERT query from a slice of structs.
//
// This method performs batch insertion of multiple structs in a single query,
// which is significantly faster than individual inserts. All structs must
// have the same type and will be inserted with the same column set.
//
// The slice element type must be a struct or pointer to struct.
// Fields are mapped using the same rules as InsertStruct.
//
// Example:
//
//	users := []User{
//	    {Name: "Alice", Email: "alice@example.com"},
//	    {Name: "Bob", Email: "bob@example.com"},
//	}
//	result, err := db.BatchInsertStruct("users", users).Execute()
//	if err != nil {
//	    return err
//	}
//
// For single struct inserts, use InsertStruct instead.
func (d *DB) BatchInsertStruct(table string, data any) *Query {
	return d.Builder().BatchInsertStruct(table, data)
}

// UpdateStruct builds an UPDATE query from a struct using db tags.
//
// Similar to InsertStruct, but for UPDATE operations. The struct fields
// are converted to SET clauses, except the primary key (a pk tag, or a field
// named ID/Id), which is never written, as in Model().Update(). You must
// chain a Where() call to specify which rows to update.
//
// This method provides type-safe struct-based updates without manually
// constructing maps.
//
// Example:
//
//	user := User{ID: 42, Name: "Alice Updated", Status: "active"}
//	result, err := db.UpdateStruct("users", &user).
//	    Where(relica.Eq("id", user.ID)).
//	    Execute()
//	if err != nil {
//	    return err
//	}
//
// For automatic WHERE clause based on primary key, consider using
// the Model() API (available in v0.6.0+).
func (d *DB) UpdateStruct(table string, data any) *UpdateQuery {
	return d.Builder().UpdateStruct(table, data)
}

// Update creates a new UPDATE query.
//
// This is a convenience method equivalent to db.Builder().Update(table).
// For batch updates, use db.Builder().BatchUpdate().
//
// Example:
//
//	_, err := db.Update("users").
//	    Set(map[string]any{"status": "active"}).
//	    Where("id = ?", 123).
//	    Execute()
//	if err != nil {
//	    return err
//	}
//
//	// For batch operations, use Builder()
//	_, err := db.Builder().
//	    BatchUpdate("users", "id").
//	    Set(1, map[string]any{"status": "active"}).
//	    Set(2, map[string]any{"status": "inactive"}).
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

// BatchInsert creates a new batch INSERT query for inserting multiple rows efficiently.
//
// This is a convenience method equivalent to db.Builder().BatchInsert(table, columns).
//
// Example:
//
//	result, err := db.BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").
//	    Values("Bob", "bob@example.com").
//	    Execute()
func (d *DB) BatchInsert(table string, columns []string) *BatchInsertQuery {
	return d.Builder().BatchInsert(table, columns)
}

// BatchUpdate creates a new batch UPDATE query for updating multiple rows with different values.
//
// This is a convenience method equivalent to db.Builder().BatchUpdate(table, keyColumn).
//
// Example:
//
//	result, err := db.BatchUpdate("users", "id").
//	    Set(1, map[string]any{"name": "Alice"}).
//	    Set(2, map[string]any{"name": "Bob"}).
//	    Execute()
func (d *DB) BatchUpdate(table, keyColumn string) *BatchUpdateQuery {
	return d.Builder().BatchUpdate(table, keyColumn)
}

// Upsert creates a new UPSERT query (INSERT ... ON CONFLICT).
//
// This is a convenience method equivalent to db.Builder().Upsert(table, values).
//
// Example:
//
//	result, err := db.Upsert("users", map[string]any{
//	    "email": "alice@example.com",
//	    "name":  "Alice",
//	}).OnConflict("email").DoUpdate("name").Execute()
func (d *DB) Upsert(table string, values map[string]any) *UpsertQuery {
	return d.Builder().Upsert(table, values)
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
//	_, err = tx.Insert("users", data).Execute()
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
	coreTx, err := d.db.BeginTx(ctx, coreTxOptions(opts))
	if err != nil {
		return nil, err
	}
	return &Tx{tx: coreTx}, nil
}

// Transactional executes f within a transaction with automatic commit/rollback.
//
// If f returns an error, the transaction is rolled back and the error is returned;
// if the rollback itself fails, both errors are returned (errors.Is matches either).
// If f panics, the transaction is rolled back and the panic is re-raised.
// If f completes successfully, the transaction is committed and a Commit error is returned.
//
// Transactional always begins a new transaction; it does not join one carried in ctx.
// Do not call Commit or Rollback on tx inside f.
//
// This helper simplifies transaction management and ensures proper cleanup
// in all code paths, including panics.
//
// Example:
//
//	err := db.Transactional(ctx, func(tx *relica.Tx) error {
//	    user := User{Name: "Alice", Email: "alice@example.com"}
//	    if err := tx.Model(&user).Insert(); err != nil {
//	        return err  // Auto rollback
//	    }
//
//	    account := Account{UserID: user.ID, Balance: 100}
//	    if err := tx.Model(&account).Insert(); err != nil {
//	        return err  // Auto rollback
//	    }
//
//	    return nil  // Auto commit
//	})
func (d *DB) Transactional(ctx context.Context, f func(*Tx) error) error {
	return d.db.Transactional(ctx, func(coreTx *core.Tx) error {
		return f(&Tx{tx: coreTx})
	})
}

// TransactionalTx executes f within a transaction with custom options.
//
// Options can specify isolation level and read-only mode. Errors, panics and
// commit behave as in Transactional.
//
// Example:
//
//	opts := &relica.TxOptions{
//	    Isolation: sql.LevelSerializable,
//	    ReadOnly:  false,
//	}
//	err := db.TransactionalTx(ctx, opts, func(tx *relica.Tx) error {
//	    // Perform operations within serializable transaction.
//	    return tx.Model(&user).Update()
//	})
func (d *DB) TransactionalTx(ctx context.Context, opts *TxOptions, f func(*Tx) error) error {
	return d.db.TransactionalTx(ctx, coreTxOptions(opts), func(coreTx *core.Tx) error {
		return f(&Tx{tx: coreTx})
	})
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
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
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
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
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
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
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

// GenerateParamName generates a dialect-specific parameter placeholder for the given index.
// For PostgreSQL, returns "$1", "$2", etc. For MySQL/SQLite, returns "?".
//
// This is useful when building dynamic SQL queries where you need
// the correct placeholder syntax for the active database driver.
//
// Example:
//
//	ph1 := db.GenerateParamName(1) // PostgreSQL: "$1", MySQL/SQLite: "?"
//	ph2 := db.GenerateParamName(2) // PostgreSQL: "$2", MySQL/SQLite: "?"
func (d *DB) GenerateParamName(index int) string {
	return d.db.GenerateParamName(index)
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
//	_, err := tx.Insert("users", map[string]any{
//	    "name":  "Alice",
//	    "email": "alice@example.com",
//	}).Execute()
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
func (t *Tx) Insert(table string, data map[string]any) *Query {
	return t.Builder().Insert(table, data)
}

// InsertStruct creates an INSERT query from a struct within the transaction.
//
// This is a convenience method equivalent to tx.Builder().InsertStruct(table, data).
// See DB.InsertStruct for full documentation.
//
// Example:
//
//	user := User{Name: "Alice", Email: "alice@example.com"}
//	_, err := tx.InsertStruct("users", &user).Execute()
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
func (t *Tx) InsertStruct(table string, data any) *Query {
	return t.Builder().InsertStruct(table, data)
}

// BatchInsertStruct creates a batch INSERT query from structs within the transaction.
//
// This is a convenience method equivalent to tx.Builder().BatchInsertStruct(table, data).
// See DB.BatchInsertStruct for full documentation.
//
// Example:
//
//	users := []User{
//	    {Name: "Alice", Email: "alice@example.com"},
//	    {Name: "Bob", Email: "bob@example.com"},
//	}
//	_, err := tx.BatchInsertStruct("users", users).Execute()
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
func (t *Tx) BatchInsertStruct(table string, data any) *Query {
	return t.Builder().BatchInsertStruct(table, data)
}

// UpdateStruct creates an UPDATE query from a struct within the transaction.
//
// This is a convenience method equivalent to tx.Builder().UpdateStruct(table, data).
// See DB.UpdateStruct for full documentation.
//
// Example:
//
//	user := User{ID: 42, Name: "Alice Updated", Status: "active"}
//	_, err := tx.UpdateStruct("users", &user).
//	    Where(relica.Eq("id", user.ID)).
//	    Execute()
//	if err != nil {
//	    tx.Rollback()
//	    return err
//	}
func (t *Tx) UpdateStruct(table string, data any) *UpdateQuery {
	return t.Builder().UpdateStruct(table, data)
}

// Update creates a new UPDATE query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().Update(table).
//
// Example:
//
//	_, err := tx.Update("users").
//	    Set(map[string]any{"status": "active"}).
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
// Rollback after Commit (or a second Rollback) does nothing and returns
// sql.ErrTxDone, so a deferred Rollback is safe; ignore its error there.
//
// Example:
//
//	tx, err := db.Begin(ctx)
//	if err != nil {
//	    return err // tx is nil: check before deferring
//	}
//	defer func() { _ = tx.Rollback() }() // sql.ErrTxDone after Commit
func (t *Tx) Rollback() error {
	return t.tx.Rollback()
}

// ExecContext executes a raw SQL query within the transaction.
func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

// QueryContext executes a raw SQL query within the transaction and returns rows.
func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a raw SQL query within the transaction returning one row.
func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}

// Model creates a ModelQuery within transaction context.
//
// All operations performed through this ModelQuery will execute
// within the transaction.
//
// Example:
//
//	err := db.Transactional(ctx, func(tx *relica.Tx) error {
//	    user := User{Name: "Alice"}
//	    return tx.Model(&user).Insert()
//	})
func (t *Tx) Model(model any) *ModelQuery {
	return &ModelQuery{mq: t.tx.Model(model)}
}

// BatchInsert creates a new batch INSERT query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().BatchInsert(table, columns).
//
// Example:
//
//	result, err := tx.BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").
//	    Values("Bob", "bob@example.com").
//	    Execute()
func (t *Tx) BatchInsert(table string, columns []string) *BatchInsertQuery {
	return t.Builder().BatchInsert(table, columns)
}

// BatchUpdate creates a new batch UPDATE query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().BatchUpdate(table, keyColumn).
//
// Example:
//
//	result, err := tx.BatchUpdate("users", "id").
//	    Set(1, map[string]any{"name": "Alice"}).
//	    Set(2, map[string]any{"name": "Bob"}).
//	    Execute()
func (t *Tx) BatchUpdate(table, keyColumn string) *BatchUpdateQuery {
	return t.Builder().BatchUpdate(table, keyColumn)
}

// Upsert creates a new UPSERT query within the transaction.
//
// This is a convenience method equivalent to tx.Builder().Upsert(table, values).
//
// Example:
//
//	result, err := tx.Upsert("users", map[string]any{
//	    "email": "alice@example.com",
//	    "name":  "Alice",
//	}).OnConflict("email").DoUpdate("name").Execute()
func (t *Tx) Upsert(table string, values map[string]any) *UpsertQuery {
	return t.Builder().Upsert(table, values)
}

// NewQuery creates a raw SQL query within the transaction.
//
// This is a convenience method equivalent to tx.Builder()-based raw query execution.
//
// Example:
//
//	var count int
//	err := tx.NewQuery("SELECT COUNT(*) FROM users").Row(&count)
func (t *Tx) NewQuery(query string) *Query {
	return &Query{q: t.tx.NewQuery(query)}
}

// ============================================================================
// ModelQuery methods
// ============================================================================

// Insert inserts the model into the table.
//
// By default, all public fields are inserted. You can specify which fields
// to insert by passing their names, or use Exclude() to exclude specific fields.
//
// Example:
//
//	// Insert all fields.
//	err := db.Model(&user).Insert()
//
//	// Insert only specific fields.
//	err := db.Model(&user).Insert("name", "email")
//
//	// Exclude fields.
//	err := db.Model(&user).Exclude("created_at").Insert()
func (mq *ModelQuery) Insert(attrs ...string) error {
	return mq.mq.Insert(attrs...)
}

// Update updates the model in the table.
//
// The WHERE clause is automatically generated using the primary key.
// The primary key is detected from:
//  1. Field with db:"id" tag
//  2. Field with db:"*_id" tag (e.g., db:"user_id")
//  3. Field named "ID"
//
// Example:
//
//	user.Status = "active"
//	err := db.Model(&user).Update()
//	// UPDATE users SET name=?, email=?, status=? WHERE id=?
//
//	// Update only specific fields.
//	err := db.Model(&user).Update("status")
//	// UPDATE users SET status=? WHERE id=?
func (mq *ModelQuery) Update(attrs ...string) error {
	return mq.mq.Update(attrs...)
}

// Delete deletes the model from the table.
//
// The WHERE clause is automatically generated using the primary key.
//
// Example:
//
//	err := db.Model(&user).Delete()
//	// DELETE FROM users WHERE id=?
func (mq *ModelQuery) Delete() error {
	return mq.mq.Delete()
}

// Upsert performs an INSERT ... ON CONFLICT DO UPDATE for the model.
//
// The conflict target is the primary key; the primary key is never updated.
// If fields are specified, only those columns are updated on conflict.
// If no fields are specified, every other inserted column is updated.
// Exclude(cols...) leaves columns out of both the INSERT and the update.
// fields and Exclude take column names (the db tag). A name that is not a
// column of the model, a field in the target or an excluded field, and a model
// with nothing left to update, are errors; nothing is sent.
//
// For PostgreSQL, the primary key is auto-populated after upsert (using RETURNING).
// For MySQL/SQLite, LastInsertId() is used when the PK is zero.
//
// Example:
//
//	user := User{ID: 1, Name: "Alice", Email: "alice@example.com"}
//	err := db.Model(&user).Upsert()
//	// INSERT INTO users (email, id, name) VALUES (?, ?, ?)
//	// ON CONFLICT (id) DO UPDATE SET email=EXCLUDED.email, name=EXCLUDED.name
//
//	// Update only specific fields on conflict:
//	err = db.Model(&user).Upsert("name")
//	// ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name
func (mq *ModelQuery) Upsert(fields ...string) error {
	return mq.mq.Upsert(fields...)
}

// UpsertOn performs INSERT ... ON CONFLICT with custom conflict columns.
// Use when the conflict target is a UNIQUE constraint, not the primary key.
//
// The first argument specifies conflict columns (the UNIQUE constraint).
// Remaining arguments specify which fields to update on conflict.
// If no fields are specified, all non-conflict columns are updated.
// Name checks and Exclude work as in Upsert; a conflict column that is not a
// column of the model is an error.
//
// Example:
//
//	// Schema: UNIQUE(project_id, qualified_name)
//	err := db.Model(&node).UpsertOn([]string{"project_id", "qualified_name"})
//	// ON CONFLICT ("project_id","qualified_name") DO UPDATE SET name=EXCLUDED.name, ...
//
//	// Selective update on conflict:
//	err := db.Model(&node).UpsertOn([]string{"email"}, "name", "status")
//	// ON CONFLICT ("email") DO UPDATE SET name=EXCLUDED.name, status=EXCLUDED.status
func (mq *ModelQuery) UpsertOn(conflictColumns []string, fields ...string) error {
	return mq.mq.UpsertOn(conflictColumns, fields...)
}

// Find fetches a record by primary key and populates the model.
// For single PK, pass one value. For composite PK, pass values
// in struct field declaration order. Returns ErrNotFound if not found.
//
// Example:
//
//	var user User
//	err := db.Model(&user).Find(42)
//	// SELECT * FROM users WHERE id = ? LIMIT 1
//
//	// Composite PK:
//	var item OrderItem
//	err := db.Model(&item).Find(orderId, itemId)
func (mq *ModelQuery) Find(pk ...any) error {
	return mq.mq.Find(pk...)
}

// UpdateChanged updates only the fields that differ between the current model
// and the original snapshot.
//
// It compares each field of the current model against original.
// Only changed fields are included in the UPDATE SET clause.
// Primary key fields are never included in the SET clause.
//
// If nothing has changed, no query is executed and nil is returned.
// The original parameter must be the same type as the model struct.
//
// Example:
//
//	var user User
//	db.Select().From("users").Where(relica.Eq("id", 1)).One(&user)
//
//	original := user
//	user.Name = "Alice Updated"
//	user.Status = "inactive"
//
//	err := db.Model(&user).UpdateChanged(&original)
//	// UPDATE users SET name=?, status=? WHERE id=?
func (mq *ModelQuery) UpdateChanged(original any) error {
	return mq.mq.UpdateChanged(original)
}

// Exclude excludes the specified fields from the operation.
//
// This is useful for auto-managed fields like timestamps.
//
// Example:
//
//	err := db.Model(&user).Exclude("created_at", "updated_at").Insert()
func (mq *ModelQuery) Exclude(attrs ...string) *ModelQuery {
	return &ModelQuery{mq: mq.mq.Exclude(attrs...)}
}

// Table overrides the table name for this operation.
//
// This is useful for partitioned tables or archival operations.
//
// Example:
//
//	err := db.Model(&user).Table("users_archive").Insert()
func (mq *ModelQuery) Table(name string) *ModelQuery {
	return &ModelQuery{mq: mq.mq.Table(name)}
}

// FindByPublicID finds a record by its autoid-tagged field value.
// Auto-detects the autoid column from struct tags and validates the prefix.
//
// This is the key method for the dual-key pattern: API handlers receive
// a public ID from the request and use this one-liner to find the record.
//
// Example:
//
//	var user User
//	err := db.Model(&user).FindByPublicID("usr_019078fa-b37e-7abc-...")
//	// Equivalent to: SELECT * FROM users WHERE public_id = ? LIMIT 1
//
//	// Prefix mismatch returns ErrAutoIDPrefixMismatch:
//	err = db.Model(&user).FindByPublicID("ord_019078fa-...")
//	// Error: autoid: prefix mismatch
func (mq *ModelQuery) FindByPublicID(publicID string) error {
	return mq.mq.FindByPublicID(publicID)
}

// WithContext sets the context for this model operation.
//
// The context is propagated to the underlying query execution,
// enabling cancellation and deadline support.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	err := db.Model(&user).WithContext(ctx).Insert()
func (mq *ModelQuery) WithContext(ctx context.Context) *ModelQuery {
	return &ModelQuery{mq: mq.mq.WithContext(ctx)}
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
//	result, err := db.Builder().Insert("users", map[string]any{
//	    "name": "Alice",
//	    "email": "alice@example.com",
//	    "status": 1,
//	}).Execute()
func (qb *QueryBuilder) Insert(table string, values map[string]any) *Query {
	return &Query{q: qb.qb.Insert(table, values)}
}

// InsertStruct builds an INSERT query from a struct using db tags.
//
// This is a convenience wrapper around Insert that converts the struct
// to a map using reflection. See DB.InsertStruct for full documentation.
//
// Example:
//
//	user := User{Name: "Alice", Email: "alice@example.com"}
//	result, err := db.Builder().InsertStruct("users", &user).Execute()
func (qb *QueryBuilder) InsertStruct(table string, data any) *Query {
	return &Query{q: qb.qb.InsertStruct(table, data)}
}

// BatchInsertStruct builds a batch INSERT query from a slice of structs.
//
// This is a convenience wrapper that converts each struct in the slice
// to column values. See DB.BatchInsertStruct for full documentation.
//
// Example:
//
//	users := []User{
//	    {Name: "Alice", Email: "alice@example.com"},
//	    {Name: "Bob", Email: "bob@example.com"},
//	}
//	result, err := db.Builder().BatchInsertStruct("users", users).Execute()
func (qb *QueryBuilder) BatchInsertStruct(table string, data any) *Query {
	return &Query{q: qb.qb.BatchInsertStruct(table, data)}
}

// UpdateStruct builds an UPDATE query from a struct using db tags.
//
// This is a convenience wrapper around Update that converts the struct
// to a map using reflection. See DB.UpdateStruct for full documentation.
//
// Example:
//
//	user := User{ID: 42, Name: "Alice Updated", Status: "active"}
//	result, err := db.Builder().UpdateStruct("users", &user).
//	    Where(relica.Eq("id", user.ID)).
//	    Execute()
func (qb *QueryBuilder) UpdateStruct(table string, data any) *UpdateQuery {
	return &UpdateQuery{uq: qb.qb.UpdateStruct(table, data)}
}

// Update creates an UPDATE query for the specified table.
//
// Use Set() to specify column values and Where() to filter rows.
//
// Example:
//
//	db.Builder().Update("users").
//	    Set(map[string]any{"status": 2}).
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
//	    Set(1, map[string]any{"status": 2}).
//	    Set(2, map[string]any{"status": 3}).
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
//	db.Builder().Upsert("users", map[string]any{
//	    "id": 1,
//	    "name": "Alice",
//	    "email": "alice@example.com",
//	}).OnConflict("id").DoUpdate("name", "email").Execute()
func (qb *QueryBuilder) Upsert(table string, values map[string]any) *UpsertQuery {
	return &UpsertQuery{uq: qb.qb.Upsert(table, values)}
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
	sq.sq.FromSelect(subquery.core(), alias)
	return sq
}

// SelectExpr adds a raw SQL expression to the SELECT clause, used as is: an
// alias is part of expr ("price * qty AS total"), and args bind the ? in expr.
//
// Select quotes its arguments as column names ("t.col", "col AS alias"), so
// literals and arithmetic belong here: Select("1 as level") renders the
// identifier "1", Select("h.level + 1") the identifier "h"."level + 1".
//
// Example:
//
//	db.Builder().Select("id", "name").
//	    SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) AS order_count").
//	    From("users").All(&results)
func (sq *SelectQuery) SelectExpr(expr string, args ...any) *SelectQuery {
	sq.sq.SelectExpr(expr, args...)
	return sq
}

// AndSelect appends additional columns to the SELECT clause.
// Useful for conditional column building where columns are added based on runtime conditions.
//
// Example:
//
//	q := db.Select("id", "name").From("users")
//	if includeEmail {
//	    q = q.AndSelect("email")
//	}
//	if includePhone {
//	    q = q.AndSelect("phone")
//	}
func (sq *SelectQuery) AndSelect(cols ...string) *SelectQuery {
	sq.sq.AndSelect(cols...)
	return sq
}

// SelectSub adds a type-safe subquery or computed expression to the SELECT clause with a quoted alias.
// The expression is built with the correct dialect so identifiers are properly quoted.
// Prefer SelectSub over SelectExpr when you have an Expression (e.g. a correlated subquery).
//
// Example:
//
//	sub := db.Builder().Select("COUNT(*)").From("orders").Where(relica.EqCol("orders.user_id", "users.id"))
//	db.Builder().Select("id", "name").SelectSub(sub.AsExpression(), "order_count").From("users").All(&results)
//
// Generates (PostgreSQL):
//
//	SELECT "id", "name", (SELECT COUNT(*) FROM "orders" WHERE "orders"."user_id" = "users"."id") AS "order_count" FROM "users"
func (sq *SelectQuery) SelectSub(exp Expression, alias string) *SelectQuery {
	sq.sq.SelectSub(coreExp(exp), alias)
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
func (sq *SelectQuery) Where(condition any, params ...any) *SelectQuery {
	sq.sq.Where(coreArg(condition), params...)
	return sq
}

// AndWhere adds a WHERE condition with AND logic.
// If no existing WHERE clause exists, behaves like Where().
//
// Example:
//
//	db.Builder().Select("*").From("users").
//	    Where("status = ?", 1).
//	    AndWhere("age > ?", 18)
func (sq *SelectQuery) AndWhere(condition any, params ...any) *SelectQuery {
	sq.sq.AndWhere(coreArg(condition), params...)
	return sq
}

// OrWhere adds a WHERE condition with OR logic.
// If no existing WHERE clause exists, behaves like Where().
//
// Example:
//
//	db.Builder().Select("*").From("users").
//	    Where("status = ?", 1).
//	    OrWhere("role = ?", "admin")
func (sq *SelectQuery) OrWhere(condition any, params ...any) *SelectQuery {
	sq.sq.OrWhere(coreArg(condition), params...)
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
func (sq *SelectQuery) InnerJoin(table string, on any) *SelectQuery {
	sq.sq.InnerJoin(table, coreArg(on))
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
func (sq *SelectQuery) LeftJoin(table string, on any) *SelectQuery {
	sq.sq.LeftJoin(table, coreArg(on))
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
func (sq *SelectQuery) RightJoin(table string, on any) *SelectQuery {
	sq.sq.RightJoin(table, coreArg(on))
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
func (sq *SelectQuery) FullJoin(table string, on any) *SelectQuery {
	sq.sq.FullJoin(table, coreArg(on))
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
// Supports multiple columns. OrderBy, OrderByExpr and OrderBySub add to the
// same clause, in the order they are called.
//
// Each argument is "column" or "column ASC|DESC" (one column per argument);
// an argument containing "(" (a function call) is used as is. Anything else —
// arithmetic, NULLS FIRST/LAST, COLLATE, "a, b" — is an error returned when
// the query runs: use OrderByExpr.
//
// Example:
//
//	OrderBy("age DESC", "name ASC")
//	OrderByExpr("created_at DESC NULLS LAST")
func (sq *SelectQuery) OrderBy(columns ...string) *SelectQuery {
	sq.sq.OrderBy(columns...)
	return sq
}

// OrderByExpr adds a raw SQL expression to the ORDER BY clause.
// The expression is used as-is without quoting — useful for CASE WHEN,
// complex functions, or any expression that shouldn't be treated as a column name.
//
// Example:
//
//	OrderByExpr("CASE WHEN status = ? THEN 0 ELSE 1 END", "active")
//	OrderByExpr("FIELD(id, ?, ?, ?)", 3, 1, 2)
func (sq *SelectQuery) OrderByExpr(expr string, args ...any) *SelectQuery {
	sq.sq.OrderByExpr(expr, args...)
	return sq
}

// OrderBySub adds a type-safe expression to the ORDER BY clause.
// Use with CaseWhen() or any Expression builder instead of raw SQL strings.
//
// Example:
//
//	db.Select("id", "title").From("tasks t").
//	    OrderBySub(relica.CaseWhen().
//	        When("t.due_date < CURRENT_DATE", 0).
//	        When("t.due_date IS NULL", 3).
//	        Else(1)).
//	    OrderBy("t.due_date ASC").
//	    All(&rows)
func (sq *SelectQuery) OrderBySub(exp Expression) *SelectQuery {
	sq.sq.OrderBySub(coreExp(exp))
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
// Multiple columns supported. GroupBy, GroupByExpr and GroupBySub add to the
// same clause, in the order they are called.
//
// Example:
//
//	GroupBy("user_id", "status")
func (sq *SelectQuery) GroupBy(columns ...string) *SelectQuery {
	sq.sq.GroupBy(columns...)
	return sq
}

// GroupByExpr adds a raw SQL expression to the GROUP BY clause.
// The expression is used as-is without quoting.
//
// Example:
//
//	GroupByExpr("DATE(created_at)")
//	GroupByExpr("EXTRACT(YEAR FROM order_date)")
func (sq *SelectQuery) GroupByExpr(expr string, args ...any) *SelectQuery {
	sq.sq.GroupByExpr(expr, args...)
	return sq
}

// GroupBySub adds a type-safe expression to the GROUP BY clause.
func (sq *SelectQuery) GroupBySub(exp Expression) *SelectQuery {
	sq.sq.GroupBySub(coreExp(exp))
	return sq
}

// Having adds HAVING clause (WHERE for aggregates).
//
// Accepts string or Expression. Multiple calls are combined with AND.
//
// Example:
//
//	Having("COUNT(*) > ?", 100)
func (sq *SelectQuery) Having(condition any, args ...any) *SelectQuery {
	sq.sq.Having(coreArg(condition), args...)
	return sq
}

// Distinct adds the DISTINCT keyword to the SELECT clause, eliminating duplicate rows.
//
// Example:
//
//	db.Builder().Select("category").From("products").Distinct().All(&categories)
//	// SELECT DISTINCT "category" FROM "products"
func (sq *SelectQuery) Distinct() *SelectQuery {
	sq.sq.Distinct()
	return sq
}

// ScanMode overrides the DB's ScanMode (see WithScanMode) for this query.
//
// Example:
//
//	db.Select().From("users").ScanMode(relica.ScanLenient).All(&partial)
func (sq *SelectQuery) ScanMode(m ScanMode) *SelectQuery {
	sq.sq.ScanMode(core.ScanMode(m))
	return sq
}

// ForUpdate adds FOR UPDATE clause for pessimistic row-level exclusive locking.
// Silently ignored for SQLite (uses database-level locking).
func (sq *SelectQuery) ForUpdate() *SelectQuery {
	sq.sq.ForUpdate()
	return sq
}

// ForShare adds FOR SHARE clause for shared row-level locking.
// Silently ignored for SQLite.
func (sq *SelectQuery) ForShare() *SelectQuery {
	sq.sq.ForShare()
	return sq
}

// ForUpdateSkipLocked adds FOR UPDATE SKIP LOCKED for non-blocking lock acquisition.
// Useful for job queue patterns. PostgreSQL 9.5+ and MySQL 8.0+. Silently ignored for SQLite.
func (sq *SelectQuery) ForUpdateSkipLocked() *SelectQuery {
	sq.sq.ForUpdateSkipLocked()
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
	sq.sq.Union(other.core())
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
	sq.sq.UnionAll(other.core())
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
	sq.sq.Intersect(other.core())
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
	sq.sq.Except(other.core())
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
	sq.sq.With(name, query.core())
	return sq
}

// WithRecursive adds a recursive Common Table Expression.
//
// The query MUST use UNION or UNION ALL.
// Database support: PostgreSQL (all), MySQL 8.0+, SQLite 3.25+
//
// Example:
//
//	anchor := db.Builder().Select("id", "name", "manager_id").SelectExpr("1 AS level").
//	    From("employees").Where("manager_id IS NULL")
//	recursive := db.Builder().Select("e.id", "e.name", "e.manager_id").SelectExpr("h.level + 1").
//	    From("employees e").InnerJoin("hierarchy h", "e.manager_id = h.id")
//	cte := anchor.UnionAll(recursive)
//	db.Builder().Select("*").WithRecursive("hierarchy", cte).
//	    From("hierarchy").OrderBy("level", "name").All(&employees)
func (sq *SelectQuery) WithRecursive(name string, query *SelectQuery) *SelectQuery {
	sq.sq.WithRecursive(name, query.core())
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

// Model reads PK values from dest struct, auto-detects table (if From not set),
// appends PK WHERE conditions, and scans a single row into dest.
// Terminal method — executes the query and returns error.
//
// Example:
//
//	// Simple PK fetch
//	user := User{ID: 42}
//	err := db.Select().Model(&user)
//
//	// Partial read with WHERE filter
//	user := User{ID: 42}
//	err := db.Select("name", "email").Where(relica.IsNull("deleted_at")).Model(&user)
//
//	// Composite PK
//	item := OrderItem{OrderID: 100, ItemID: 20}
//	err := db.Select().Model(&item)
func (sq *SelectQuery) Model(dest any) error {
	return sq.sq.Model(dest)
}

// One scans a single row into dest.
//
// Returns ErrNotFound if no row is found (wraps sql.ErrNoRows).
//
// Example:
//
//	var user User
//	err := db.Builder().Select("*").From("users").
//	    Where("id = ?", 123).One(&user)
func (sq *SelectQuery) One(dest any) error {
	return sq.sq.One(dest)
}

// All scans all rows into dest slice.
//
// Example:
//
//	var users []User
//	err := db.Builder().Select("*").From("users").All(&users)
func (sq *SelectQuery) All(dest any) error {
	return sq.sq.All(dest)
}

// Row scans a single row into individual variables.
// Returns sql.ErrNoRows if no rows are found.
//
// Example:
//
//	var name string
//	var age int
//	err := db.Builder().Select("name", "age").From("users").
//	    Where("id = ?", 1).Row(&name, &age)
func (sq *SelectQuery) Row(dest ...any) error {
	return sq.sq.Row(dest...)
}

// Column scans the first column of all rows into a slice.
// The slice parameter must be a pointer to a slice of the appropriate type.
//
// Example:
//
//	var ids []int
//	err := db.Builder().Select("id").From("users").
//	    Where("status = ?", "active").Column(&ids)
func (sq *SelectQuery) Column(slice any) error {
	return sq.sq.Column(slice)
}

// Count returns the number of rows All would return: with GroupBy the number
// of groups, and a select list that aggregates without GroupBy counts 1;
// Having, Distinct, set operations and CTEs apply. OrderBy, Limit and Offset
// are ignored, so a paginated query reports its total — except in a set
// operation, where they belong to the first branch and are kept.
//
// Example:
//
//	count, err := db.Select().From("users").Where(relica.Eq("status", 1)).Count()
func (sq *SelectQuery) Count() (int64, error) {
	return sq.sq.Count()
}

// Exists reports whether the query returns at least one row, by the same rule
// as Count (GroupBy, Having, Distinct, set operations and CTEs apply; OrderBy,
// Limit and Offset are ignored except in the first branch of a set operation).
// The query is sent inside SELECT EXISTS(...).
//
// Example:
//
//	exists, err := db.Select().From("users").Where(relica.Eq("email", email)).Exists()
func (sq *SelectQuery) Exists() (bool, error) {
	return sq.sq.Exists()
}

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
// A query with a construction error returns "" and nil (running it returns
// the error); the transaction rule for row locks is checked when it runs.
//
// Example:
//
//	sql, params := db.Select().From("users").Where(relica.Eq("id", 1)).ToSQL()
func (sq *SelectQuery) ToSQL() (string, []any) {
	return sq.sq.ToSQL()
}

// Explain analyzes the query execution plan without executing the query.
// Returns [QueryPlan] with estimated metrics (cost, rows, index usage).
// Supported databases: PostgreSQL, MySQL, SQLite.
//
// Example:
//
//	plan, err := db.Select("*").From("users").Where(relica.Eq("email", "alice@example.com")).Explain()
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Cost: %.2f, Rows: %d, Uses Index: %v\n",
//	    plan.Cost, plan.EstimatedRows, plan.UsesIndex)
func (sq *SelectQuery) Explain() (*QueryPlan, error) {
	plan, err := sq.sq.Explain()
	return queryPlan(plan), err
}

// ExplainAnalyze analyzes the query execution plan AND executes the query.
// Returns [QueryPlan] with both estimated and actual execution metrics.
// Supported databases: PostgreSQL, MySQL 8.0.18+.
//
// WARNING: This method ACTUALLY EXECUTES the query, including any side effects
// (INSERT, UPDATE, DELETE in CTEs, triggers, etc.). Use with caution in production.
//
// Example:
//
//	plan, err := db.Select("*").From("users").Where(relica.Eq("status", 1)).ExplainAnalyze()
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Actual Time: %v, Actual Rows: %d, Buffers Hit: %d\n",
//	    plan.ActualTime, plan.ActualRows, plan.BuffersHit)
func (sq *SelectQuery) ExplainAnalyze() (*QueryPlan, error) {
	plan, err := sq.sq.ExplainAnalyze()
	return queryPlan(plan), err
}

// AsExpression converts a SelectQuery to an Expression for subquery use.
//
// Where, AndWhere, OrWhere and Having render their condition — and an In or
// Exists subquery inside it — when it is added: changes made to sub afterwards
// are not seen. SelectSub, OrderBySub, GroupBySub (and FromSelect, Union, With)
// keep the query and render it when the outer query runs. Either way a
// construction error in sub fails the outer query.
//
// Example:
//
//	sub := db.Select("user_id").From("orders").Where(relica.GreaterThan("total", 100))
//	db.Select().From("users").
//	    Where(relica.In("id", sub.AsExpression())).All(&users)
func (sq *SelectQuery) AsExpression() Expression {
	return wrapExp(sq.sq.AsExpression())
}

// ============================================================================
// UpdateQuery Methods
// ============================================================================

// UpdateQuery represents an UPDATE query being built.
type UpdateQuery struct {
	uq  *core.UpdateQuery
	err error // Error from query construction (e.g., struct conversion).
}

// WithContext sets the context for this UPDATE query.
func (uq *UpdateQuery) WithContext(ctx context.Context) *UpdateQuery {
	return &UpdateQuery{uq: uq.uq.WithContext(ctx), err: uq.err}
}

// Set specifies the columns and values to update.
//
// Example:
//
//	Update("users").Set(map[string]any{"status": 2})
func (uq *UpdateQuery) Set(values map[string]any) *UpdateQuery {
	uq.uq.Set(values)
	return uq
}

// Where adds a WHERE condition to the UPDATE query.
//
// An UPDATE needs Where or AllRows: without either, or with a condition that
// renders empty (And(), HashExp{}, Like with no values), Execute returns an
// error and nothing is sent (ADR-020).
//
// Example:
//
//	Update("users").Set(...).Where("id = ?", 123)
func (uq *UpdateQuery) Where(condition any, params ...any) *UpdateQuery {
	uq.uq.Where(coreArg(condition), params...)
	return uq
}

// AllRows updates every row of the table on purpose. It cannot be combined
// with Where.
//
// Example:
//
//	db.Update("settings").Set(map[string]any{"dirty": false}).AllRows().Execute()
func (uq *UpdateQuery) AllRows() *UpdateQuery {
	uq.uq.AllRows()
	return uq
}

// AndWhere adds a WHERE condition with AND logic.
// If no existing WHERE clause exists, behaves like Where().
//
// Example:
//
//	db.Builder().Update("users").
//	    Set(map[string]any{"status": 2}).
//	    Where("id > ?", 100).
//	    AndWhere("active = ?", true)
func (uq *UpdateQuery) AndWhere(condition any, params ...any) *UpdateQuery {
	uq.uq.AndWhere(coreArg(condition), params...)
	return uq
}

// OrWhere adds a WHERE condition with OR logic.
// If no existing WHERE clause exists, behaves like Where().
//
// Example:
//
//	db.Builder().Update("users").
//	    Set(map[string]any{"status": 0}).
//	    Where("banned = ?", true).
//	    OrWhere("deleted = ?", true)
func (uq *UpdateQuery) OrWhere(condition any, params ...any) *UpdateQuery {
	uq.uq.OrWhere(coreArg(condition), params...)
	return uq
}

// Build constructs the Query object.
func (uq *UpdateQuery) Build() *Query {
	if uq.err != nil {
		return &Query{q: nil, err: uq.err}
	}
	return &Query{q: uq.uq.Build()}
}

// Execute executes the UPDATE query.
func (uq *UpdateQuery) Execute() (sql.Result, error) {
	if uq.err != nil {
		return nil, uq.err
	}
	return uq.Build().Execute()
}

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
//
// Example:
//
//	sql, params := db.Update("users").Set(map[string]any{"status": 1}).Where(relica.Eq("id", 1)).ToSQL()
func (uq *UpdateQuery) ToSQL() (string, []any) {
	if uq.err != nil {
		return "", nil
	}
	return uq.uq.ToSQL()
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
// A DELETE needs Where or AllRows: without either, or with a condition that
// renders empty (And(), HashExp{}, Like with no values), Execute returns an
// error and nothing is sent (ADR-020).
//
// Example:
//
//	Delete("users").Where("id = ?", 123)
func (dq *DeleteQuery) Where(condition any, params ...any) *DeleteQuery {
	dq.dq.Where(coreArg(condition), params...)
	return dq
}

// AllRows deletes every row of the table on purpose. It cannot be combined
// with Where.
//
// Example:
//
//	db.Delete("sessions").AllRows().Execute()
func (dq *DeleteQuery) AllRows() *DeleteQuery {
	dq.dq.AllRows()
	return dq
}

// AndWhere adds a WHERE condition with AND logic.
// If no existing WHERE clause exists, behaves like Where().
//
// Example:
//
//	db.Builder().Delete("users").
//	    Where("status = ?", 0).
//	    AndWhere("created_at < ?", "2020-01-01")
func (dq *DeleteQuery) AndWhere(condition any, params ...any) *DeleteQuery {
	dq.dq.AndWhere(coreArg(condition), params...)
	return dq
}

// OrWhere adds a WHERE condition with OR logic.
// If no existing WHERE clause exists, behaves like Where().
//
// Example:
//
//	db.Builder().Delete("users").
//	    Where("banned = ?", true).
//	    OrWhere("deleted = ?", true)
func (dq *DeleteQuery) OrWhere(condition any, params ...any) *DeleteQuery {
	dq.dq.OrWhere(coreArg(condition), params...)
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

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
//
// Example:
//
//	sql, params := db.Delete("users").Where(relica.Eq("id", 1)).ToSQL()
func (dq *DeleteQuery) ToSQL() (string, []any) {
	return dq.dq.ToSQL()
}

// ============================================================================
// UpsertQuery Methods
// ============================================================================

// UpsertQuery represents an UPSERT query being built: INSERT with conflict
// handling. It needs exactly one conflict action — DoUpdate(cols...),
// DoUpdateAll() or DoNothing(); none, two, or an invalid one is an error
// returned by Execute (nothing is sent). Updated columns take their value from
// the row proposed for insertion (PostgreSQL/SQLite EXCLUDED, MySQL VALUES() or
// the row alias, see WithMySQLUpsertSyntax).
type UpsertQuery struct {
	uq *core.UpsertQuery
}

// WithContext sets the context for this UPSERT query.
func (uq *UpsertQuery) WithContext(ctx context.Context) *UpsertQuery {
	return &UpsertQuery{uq: uq.uq.WithContext(ctx)}
}

// OnConflict sets the conflict target: the columns of a PRIMARY KEY or UNIQUE
// constraint. Required with DoUpdate/DoUpdateAll on PostgreSQL and SQLite.
// On MySQL it is advisory: ON DUPLICATE KEY UPDATE fires on any unique key, but
// the target columns are still left out of DoUpdateAll. No columns, or a second
// call, is an error.
//
// Example:
//
//	Upsert(...).OnConflict("id", "email")
func (uq *UpsertQuery) OnConflict(columns ...string) *UpsertQuery {
	uq.uq.OnConflict(columns...)
	return uq
}

// DoUpdate updates exactly the listed columns from the proposed row on
// conflict. Each column must be one of the inserted columns; no columns is an
// error (use DoUpdateAll).
//
// Example:
//
//	Upsert(...).OnConflict("id").DoUpdate("name", "email")
func (uq *UpsertQuery) DoUpdate(columns ...string) *UpsertQuery {
	uq.uq.DoUpdate(columns...)
	return uq
}

// DoUpdateAll updates every inserted column except the conflict target from the
// proposed row on conflict. It is an error when no column is left to update.
//
// Example:
//
//	Upsert(...).OnConflict("id").DoUpdateAll()
func (uq *UpsertQuery) DoUpdateAll() *UpsertQuery {
	uq.uq.DoUpdateAll()
	return uq
}

// DoNothing keeps the existing row on conflict. PostgreSQL/SQLite: ON CONFLICT
// DO NOTHING (without OnConflict: any constraint). MySQL has no DO NOTHING:
// Relica assigns the first conflict column (or the first inserted column) to
// itself in ON DUPLICATE KEY UPDATE, so the row is still locked, an
// AUTO_INCREMENT value is consumed and UPDATE triggers fire.
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

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
//
// Example:
//
//	sql, params := db.Upsert("users", map[string]any{"id": 1, "name": "Alice"}).
//	    OnConflict("id").DoUpdate("name").ToSQL()
func (uq *UpsertQuery) ToSQL() (string, []any) {
	q := uq.Build()
	return q.SQL(), q.Params()
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
func (biq *BatchInsertQuery) Values(values ...any) *BatchInsertQuery {
	biq.biq.Values(values...)
	return biq
}

// ValuesMap adds a row from a map.
//
// Example:
//
//	BatchInsert("users", []string{"name", "email"}).
//	    ValuesMap(map[string]any{"name": "Alice", "email": "alice@example.com"})
func (biq *BatchInsertQuery) ValuesMap(values map[string]any) *BatchInsertQuery {
	biq.biq.ValuesMap(values)
	return biq
}

// Build constructs the Query object.
func (biq *BatchInsertQuery) Build() *Query {
	return &Query{q: biq.biq.Build()}
}

// OnConflict sets the conflict target and turns the batch into an upsert, which
// then needs exactly one of DoUpdate, DoUpdateAll, DoNothing (see UpsertQuery).
// A batch without OnConflict or an action is a plain INSERT.
//
// Example:
//
//	db.BatchInsert("nodes", []string{"name", "label"}).
//	    Values("Foo", "struct").
//	    OnConflict("project_id", "qualified_name").
//	    DoUpdate("name", "label").
//	    Execute()
func (biq *BatchInsertQuery) OnConflict(columns ...string) *BatchInsertQuery {
	biq.biq.OnConflict(columns...)
	return biq
}

// DoUpdate updates exactly the listed batch columns from each proposed row on
// conflict (see UpsertQuery.DoUpdate). Duplicate keys inside one batch are not
// detected client-side: PostgreSQL rejects the statement (error 21000, "cannot
// affect row a second time"), MySQL and SQLite keep the last row.
func (biq *BatchInsertQuery) DoUpdate(columns ...string) *BatchInsertQuery {
	biq.biq.DoUpdate(columns...)
	return biq
}

// DoUpdateAll updates every batch column except the conflict target from each
// proposed row on conflict (see UpsertQuery.DoUpdateAll).
func (biq *BatchInsertQuery) DoUpdateAll() *BatchInsertQuery {
	biq.biq.DoUpdateAll()
	return biq
}

// DoNothing keeps existing rows on conflict (see UpsertQuery.DoNothing for
// MySQL). With duplicate keys inside one batch the first row wins.
func (biq *BatchInsertQuery) DoNothing() *BatchInsertQuery {
	biq.biq.DoNothing()
	return biq
}

// Execute executes the batch INSERT query.
func (biq *BatchInsertQuery) Execute() (sql.Result, error) {
	return biq.Build().Execute()
}

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
//
// Example:
//
//	sql, params := db.BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").ToSQL()
func (biq *BatchInsertQuery) ToSQL() (string, []any) {
	q := biq.Build()
	return q.SQL(), q.Params()
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
//	    Set(1, map[string]any{"status": 2}).
//	    Set(2, map[string]any{"status": 3})
func (buq *BatchUpdateQuery) Set(keyValue any, values map[string]any) *BatchUpdateQuery {
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

// ToSQL returns the SQL string and parameters without executing the query.
// This is useful for debugging, logging, or passing the query to another layer.
//
// Example:
//
//	sql, params := db.BatchUpdate("users", "id").
//	    Set(1, map[string]any{"status": 2}).ToSQL()
func (buq *BatchUpdateQuery) ToSQL() (string, []any) {
	q := buq.Build()
	return q.SQL(), q.Params()
}

// ============================================================================
// Query Methods
// ============================================================================

// WithContext sets the context for this query, overriding the context
// inherited from DB.WithContext or from the transaction. Use it to attach a
// per-query deadline to an INSERT or raw query:
//
//	_, err := db.Insert("users", data).WithContext(ctx).Execute()
//	err = db.NewQuery("DELETE FROM sessions WHERE expired").WithContext(ctx).Execute()
func (q *Query) WithContext(ctx context.Context) *Query {
	if q.err != nil {
		return q
	}
	q.q.WithContext(ctx)
	return q
}

// Execute runs the query and returns results.
func (q *Query) Execute() (sql.Result, error) {
	if q.err != nil {
		return nil, q.err
	}
	result, err := q.q.Execute()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// One fetches a single row into dest.
func (q *Query) One(dest any) error {
	if q.err != nil {
		return q.err
	}
	return q.q.One(dest)
}

// All fetches all rows into dest slice.
func (q *Query) All(dest any) error {
	if q.err != nil {
		return q.err
	}
	return q.q.All(dest)
}

// Row scans a single row into individual variables.
// Returns sql.ErrNoRows if no rows are found.
//
// Example:
//
//	var name string
//	var age int
//	err := db.Select("name", "age").From("users").Where("id = ?", 1).Row(&name, &age)
func (q *Query) Row(dest ...any) error {
	if q.err != nil {
		return q.err
	}
	return q.q.Row(dest...)
}

// Column scans the first column of all rows into a slice.
// The slice parameter must be a pointer to a slice of the appropriate type.
//
// Example:
//
//	var ids []int
//	err := db.Select("id").From("users").Where("status = ?", "active").Column(&ids)
func (q *Query) Column(slice any) error {
	if q.err != nil {
		return q.err
	}
	return q.q.Column(slice)
}

// Prepare prepares the query for repeated execution.
// Call Close() when done to release the prepared statement.
// The prepared statement bypasses the automatic statement cache,
// giving you full control over the statement lifecycle.
//
// Example:
//
//	q := db.NewQuery("SELECT * FROM users WHERE status = ?").Prepare()
//	defer q.Close()
//
//	for _, status := range statuses {
//	    var users []User
//	    q.Bind(status).All(&users)
//	    // process users...
//	}
func (q *Query) Prepare() *Query {
	if q.err != nil {
		return q
	}
	q.q.Prepare()
	return q
}

// Close releases the prepared statement.
// Safe to call multiple times or on non-prepared queries.
// Returns nil if query was not prepared or already closed.
func (q *Query) Close() error {
	if q.q != nil {
		return q.q.Close()
	}
	return nil
}

// IsPrepared returns true if Prepare() was called successfully.
func (q *Query) IsPrepared() bool {
	if q.q == nil {
		return false
	}
	return q.q.IsPrepared()
}

// Bind sets positional parameters for the query.
// Parameters replace ? placeholders in order.
//
// Example:
//
//	db.NewQuery("SELECT * FROM users WHERE id = ? AND status = ?").
//	    Bind(1, "active").
//	    One(&user)
func (q *Query) Bind(params ...any) *Query {
	if q.err != nil {
		return q
	}
	q.q.Bind(params...)
	return q
}

// BindParams binds named parameters using Params map.
// Named parameters are specified using {:name} syntax.
//
// Example:
//
//	db.NewQuery("SELECT * FROM users WHERE id = {:id}").
//	    BindParams(relica.Params{"id": 1}).
//	    One(&user)
func (q *Query) BindParams(params Params) *Query {
	if q.err != nil {
		return q
	}
	q.q.BindParams(core.Params(params))
	return q
}

// ToSQL returns the SQL string and parameters without executing the query.
// This provides a consistent interface across all query types (Select, Insert, Update, Delete, Upsert).
//
// Example:
//
//	sql, params := db.Insert("users", map[string]any{
//	    "name": "Alice", "email": "alice@example.com",
//	}).ToSQL()
//	// sql: INSERT INTO "users" ("email", "name") VALUES ($1, $2)
//	// params: [alice@example.com Alice]
func (q *Query) ToSQL() (string, []any) {
	if q.q == nil {
		return "", nil
	}
	return q.q.ToSQL()
}

// SQL returns the SQL query string.
func (q *Query) SQL() string {
	if q.q == nil {
		return ""
	}
	return q.q.SQL()
}

// Params returns the query parameters.
func (q *Query) Params() []any {
	if q.q == nil {
		return nil
	}
	return q.q.Params()
}

// ============================================================================
// Re-export errors and error classification helpers
// ============================================================================

// ErrNotFound is returned by One() when no rows match the query.
// It wraps sql.ErrNoRows so both errors.Is(err, relica.ErrNotFound)
// and errors.Is(err, sql.ErrNoRows) return true.
//
// Example:
//
//	var user User
//	err := db.Select().From("users").Where(relica.Eq("id", 999)).One(&user)
//	if errors.Is(err, relica.ErrNotFound) {
//	    // handle not found
//	}
var ErrNotFound = core.ErrNotFound

// ErrAutoIDPrefixMismatch is returned by FindByPublicID when the provided ID's
// prefix doesn't match the model's autoid tag prefix.
// This prevents accidentally looking up an order ID in the users table.
//
// Example:
//
//	// User has autoid:usr, but we pass an order ID:
//	err := db.Model(&user).FindByPublicID("ord_019078fa-...")
//	if errors.Is(err, relica.ErrAutoIDPrefixMismatch) {
//	    // wrong ID type for this model
//	}
var ErrAutoIDPrefixMismatch = util.ErrAutoIDPrefixMismatch

// RegisterIDGenerator registers a custom ID generator for use with the autoid tag.
// The default generator is UUID v7. Custom generators enable ULID, Snowflake, or any format.
//
// The generator function returns the ID body (without prefix).
// The prefix is added automatically by Relica based on the autoid tag.
//
// Example:
//
//	// Register once at startup:
//	relica.RegisterIDGenerator("ulid", func() string {
//	    return ulid.Make().String()
//	})
//
//	// Use in struct tag:
//	type Event struct {
//	    ID       int64  `db:"id,pk"`
//	    PublicID string `db:"public_id,autoid:evt,gen=ulid"`
//	}
//
//	event := Event{}
//	db.Model(&event).Insert()
//	// event.PublicID = "evt_01H9XZ..." (ULID with prefix)
func RegisterIDGenerator(name string, fn func() string) {
	util.RegisterIDGenerator(name, fn)
}

// IsUniqueViolation reports whether err is a unique or primary key violation:
// PostgreSQL SQLSTATE 23505; MySQL/MariaDB 1062, 1022, 1169, 1586, 1859;
// SQLite SQLITE_CONSTRAINT_UNIQUE, SQLITE_CONSTRAINT_PRIMARYKEY.
// Returns false for nil. See "Classifying database errors" in the package doc.
//
// Example:
//
//	err := db.Model(&user).Insert()
//	if relica.IsUniqueViolation(err) {
//	    return ErrEmailTaken
//	}
func IsUniqueViolation(err error) bool { return core.IsUniqueViolation(err) }

// IsForeignKeyViolation reports whether err is a foreign key violation:
// PostgreSQL SQLSTATE 23503; MySQL/MariaDB 1216, 1217, 1451, 1452;
// SQLite SQLITE_CONSTRAINT_FOREIGNKEY (needs PRAGMA foreign_keys = ON).
// Returns false for nil.
//
// Example:
//
//	err := db.Model(&order).Insert()
//	if relica.IsForeignKeyViolation(err) {
//	    return ErrUnknownCustomer
//	}
func IsForeignKeyViolation(err error) bool { return core.IsForeignKeyViolation(err) }

// IsNotNullViolation reports whether err is a NOT NULL violation:
// PostgreSQL SQLSTATE 23502; MySQL/MariaDB 1048, 1263, 1364 (strict mode);
// SQLite SQLITE_CONSTRAINT_NOTNULL. Returns false for nil.
//
// Example:
//
//	err := db.Model(&user).Insert()
//	if relica.IsNotNullViolation(err) {
//	    return ErrMissingField
//	}
func IsNotNullViolation(err error) bool { return core.IsNotNullViolation(err) }

// IsCheckViolation reports whether err is a CHECK constraint violation:
// PostgreSQL SQLSTATE 23514; MySQL 3819 (8.0.16+); MariaDB 4025;
// SQLite SQLITE_CONSTRAINT_CHECK. Returns false for nil.
//
// Example:
//
//	err := db.Model(&product).Insert()
//	if relica.IsCheckViolation(err) {
//	    return ErrInvalidPrice
//	}
func IsCheckViolation(err error) bool { return core.IsCheckViolation(err) }

// IsDeadlock reports whether the server aborted the transaction as a deadlock
// victim: PostgreSQL SQLSTATE 40P01; MySQL/MariaDB 1213 (InnoDB has no
// separate serialization error; conflicts under SERIALIZABLE end here). The
// transaction has been rolled back; retry it as a whole. SQLite has no
// deadlock detection and never matches. Returns false for nil.
//
// Example:
//
//	err := db.Transactional(ctx, transfer)
//	if relica.IsDeadlock(err) || relica.IsSerializationFailure(err) {
//	    // retry the whole transaction with backoff
//	}
func IsDeadlock(err error) bool { return core.IsDeadlock(err) }

// IsSerializationFailure reports whether a transaction failed because it
// could not be serialized with concurrent ones: PostgreSQL SQLSTATE 40001
// (REPEATABLE READ, SERIALIZABLE; also CockroachDB and YugabyteDB);
// SQLite SQLITE_BUSY_SNAPSHOT (a WAL read transaction tried to write after
// another connection committed). Retry the whole transaction. MySQL reports
// such conflicts as deadlocks — see [IsDeadlock]. Returns false for nil.
func IsSerializationFailure(err error) bool { return core.IsSerializationFailure(err) }

// IsLockTimeout reports whether a lock could not be acquired in time:
// PostgreSQL SQLSTATE 55P03 (lock_timeout, FOR UPDATE NOWAIT); MySQL/MariaDB
// 1205 (innodb_lock_wait_timeout) and 3572 (NOWAIT); SQLite SQLITE_BUSY
// (busy timeout expired; not SQLITE_BUSY_SNAPSHOT) and SQLITE_LOCKED.
// PostgreSQL statement_timeout (57014) is not a lock timeout.
//
// Whether to retry is a policy decision: the lock holder may be a long job.
// On MySQL a lock wait timeout rolls back only the statement unless
// innodb_rollback_on_timeout is set; return the error from the function
// passed to [DB.Transactional] and the whole transaction is rolled back.
// Returns false for nil.
func IsLockTimeout(err error) bool { return core.IsLockTimeout(err) }
