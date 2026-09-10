package core

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"
)

// Query represents a database query.
// When tx is not nil, the query executes within that transaction.
type Query struct {
	sql      string
	params   []any
	db       *DB
	tx       *sql.Tx // nil for non-transactional queries
	ctx      context.Context
	stmt     *sql.Stmt // manually prepared statement (bypasses cache)
	prepared bool      // true if Prepare() was called
	prepErr  error     // error from Prepare() call
}

// appendSQL appends a suffix to the SQL query.
// This is used internally for PostgreSQL RETURNING clause.
func (q *Query) appendSQL(suffix string) {
	q.sql += suffix
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
//	    q.Bind(relica.Params{"status": status}).All(&users)
//	}
func (q *Query) Prepare() *Query {
	if q.prepared {
		return q
	}

	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	var stmt *sql.Stmt
	var err error

	if q.tx != nil {
		stmt, err = q.tx.PrepareContext(ctx, q.sql)
	} else {
		stmt, err = q.db.sqlDB.PrepareContext(ctx, q.sql)
	}

	if err != nil {
		q.prepErr = err
		return q
	}

	q.stmt = stmt
	q.prepared = true
	return q
}

// Close releases the prepared statement.
// Safe to call multiple times or on non-prepared queries.
// Returns nil if query was not prepared or already closed.
func (q *Query) Close() error {
	if q.stmt != nil {
		err := q.stmt.Close()
		q.stmt = nil
		q.prepared = false
		return err
	}
	return nil
}

// IsPrepared returns true if Prepare() was called successfully.
func (q *Query) IsPrepared() bool {
	return q.prepared && q.stmt != nil
}

// prepareStatement prepares a SQL statement using the statement cache.
// For manually prepared queries (Prepare() called), returns the stored statement.
// For regular queries, uses LRU statement cache for better performance.
// Transactions skip this path entirely — they use direct tx.QueryContext/tx.ExecContext.
func (q *Query) prepareStatement(ctx context.Context) (*sql.Stmt, error) {
	// Check for preparation error from Prepare() call
	if q.prepErr != nil {
		return nil, q.prepErr
	}

	// Validate query and parameters if a validator is configured.
	// This covers builder-generated queries (Select/Insert/Update/Delete) which bypass
	// the raw DB.ExecContext/QueryContext paths where validation already occurs.
	if q.db != nil && q.db.validator != nil {
		if err := q.db.validateQueryAndParams(ctx, q.sql, q.params); err != nil {
			return nil, err
		}
	}

	// Use manually prepared statement if available
	if q.prepared && q.stmt != nil {
		return q.stmt, nil
	}

	// Use statement cache for non-transactional queries
	if stmt, ok := q.db.stmtCache.Get(q.sql); ok {
		return stmt, nil
	}

	stmt, err := q.db.sqlDB.PrepareContext(ctx, q.sql)
	if err != nil {
		return nil, err
	}
	cached, inserted := q.db.stmtCache.GetOrSet(q.sql, stmt)
	if !inserted {
		_ = stmt.Close() // lost the race; our stmt is unobserved, safe to close
		return cached, nil
	}
	return stmt, nil
}

// useDirectTx returns true when the query should use direct tx.Exec/Query
// instead of Prepare+Exec (saves 2 round-trips per query).
func (q *Query) useDirectTx() bool {
	return q.tx != nil && !q.prepared
}

// getContext returns the query context, defaulting to context.Background().
func (q *Query) getContext() context.Context {
	if q.ctx != nil {
		return q.ctx
	}
	return context.Background()
}

// validateBeforeExec runs validator and checks for build errors.
// Returns error if validation fails, nil otherwise.
func (q *Query) validateBeforeExec(ctx context.Context) error {
	if q.prepErr != nil {
		return q.prepErr
	}
	if q.db != nil && q.db.validator != nil {
		return q.db.validateQueryAndParams(ctx, q.sql, q.params)
	}
	return nil
}

// Execute runs the query and returns results.
// For transactions, uses direct tx.ExecContext (1 round-trip).
// For non-tx queries, uses prepared statement cache.
func (q *Query) Execute() (sql.Result, error) {
	ctx := q.getContext()
	start := time.Now()

	if err := q.validateBeforeExec(ctx); err != nil {
		return nil, err
	}

	// Direct execution for transactions (1 round-trip, no Prepare overhead)
	if q.useDirectTx() {
		result, err := q.tx.ExecContext(ctx, q.sql, q.params...)
		elapsed := time.Since(start)
		var rowsAffected int64
		if result != nil {
			rowsAffected, _ = result.RowsAffected()
		}
		q.db.invokeHook(ctx, QueryEvent{
			SQL:          q.sql,
			Args:         q.params,
			Duration:     elapsed,
			RowsAffected: rowsAffected,
			Error:        err,
			Operation:    DetectOperation(q.sql),
		})
		return result, err
	}

	// Standard path: prepare + execute (with cache for non-tx)
	stmt, err := q.prepareStatement(ctx)
	if err != nil {
		return nil, err
	}

	result, err := stmt.ExecContext(ctx, q.params...)
	elapsed := time.Since(start)

	var rowsAffected int64
	if result != nil {
		rowsAffected, _ = result.RowsAffected()
	}
	q.db.invokeHook(ctx, QueryEvent{
		SQL:          q.sql,
		Args:         q.params,
		Duration:     elapsed,
		RowsAffected: rowsAffected,
		Error:        err,
		Operation:    DetectOperation(q.sql),
	})

	return result, err
}

// One fetches a single row into a struct.
// If query is part of a transaction, uses transaction connection.
func (q *Query) One(dest any) error {
	ctx := q.getContext()
	start := time.Now()

	if err := q.validateBeforeExec(ctx); err != nil {
		return err
	}

	// Execute query — direct for tx, prepared for non-tx
	var rows *sql.Rows
	var err error
	if q.useDirectTx() {
		rows, err = q.tx.QueryContext(ctx, q.sql, q.params...)
	} else {
		var stmt *sql.Stmt
		stmt, err = q.prepareStatement(ctx)
		if err != nil {
			return err
		}
		rows, err = stmt.QueryContext(ctx, q.params...)
	}
	if err != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}
	defer func() { _ = rows.Close() }()

	// Check if row exists — must check rows.Err() first to distinguish
	// "no rows" from real errors (context cancellation, network, driver).
	if !rows.Next() {
		if rowErr := rows.Err(); rowErr != nil {
			elapsed := time.Since(start)
			q.db.invokeHook(ctx, QueryEvent{
				SQL:       q.sql,
				Args:      q.params,
				Duration:  elapsed,
				Error:     rowErr,
				Operation: DetectOperation(q.sql),
			})
			return rowErr
		}
		err := wrapErrNotFound()
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}

	// Scan into dest - detect NullStringMap for dynamic scanning
	var scanErr error
	if destMap, ok := dest.(*NullStringMap); ok {
		scanErr = globalScanner.scanMapRow(rows, destMap)
	} else {
		scanErr = globalScanner.scanRow(rows, dest)
	}
	if scanErr != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     scanErr,
			Operation: DetectOperation(q.sql),
		})
		return scanErr
	}

	elapsed := time.Since(start)

	// Invoke query hook
	q.db.invokeHook(ctx, QueryEvent{
		SQL:       q.sql,
		Args:      q.params,
		Duration:  elapsed,
		Operation: DetectOperation(q.sql),
	})

	// Analyze query performance if optimizer is enabled (async to not block)
	if q.db.optimizer != nil {
		go q.analyzeQuery(ctx, elapsed)
	}

	return nil
}

// Row scans a single row into individual variables.
// Returns ErrNotFound if no rows are found.
//
// Example:
//
//	var name string
//	var age int
//	err := db.Select("name", "age").From("users").Where("id = ?", 1).Row(&name, &age)
//
//	// For scalar queries
//	var count int
//	err := db.NewQuery("SELECT COUNT(*) FROM users").Row(&count)
func (q *Query) Row(dest ...any) error {
	ctx := q.getContext()
	start := time.Now()

	if err := q.validateBeforeExec(ctx); err != nil {
		return err
	}

	// Execute query — direct for tx, prepared for non-tx
	var rows *sql.Rows
	var err error
	if q.useDirectTx() {
		rows, err = q.tx.QueryContext(ctx, q.sql, q.params...)
	} else {
		var stmt *sql.Stmt
		stmt, err = q.prepareStatement(ctx)
		if err != nil {
			return err
		}
		rows, err = stmt.QueryContext(ctx, q.params...)
	}
	if err != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}
	defer func() { _ = rows.Close() }()

	// Check if row exists
	if !rows.Next() {
		err := rows.Err()
		if err == nil {
			err = wrapErrNotFound()
		}
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}

	// Scan into dest variables
	if err := rows.Scan(dest...); err != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}

	elapsed := time.Since(start)

	// Invoke query hook
	q.db.invokeHook(ctx, QueryEvent{
		SQL:       q.sql,
		Args:      q.params,
		Duration:  elapsed,
		Operation: DetectOperation(q.sql),
	})

	return nil
}

// Column scans the first column of all rows into a slice.
// The slice parameter must be a pointer to a slice of the appropriate type.
//
// Example:
//
//	var ids []int
//	err := db.Select("id").From("users").Where("status = ?", "active").Column(&ids)
//
//	var emails []string
//	err := db.Select("email").From("users").Column(&emails)
//
//nolint:cyclop // Column scanning requires sequential steps
func (q *Query) Column(slice any) error {
	ctx := q.getContext()
	start := time.Now()

	if err := q.validateBeforeExec(ctx); err != nil {
		return err
	}

	// Validate slice parameter
	sliceVal := reflect.ValueOf(slice)
	if sliceVal.Kind() != reflect.Pointer || sliceVal.IsNil() {
		return fmt.Errorf("relica: Column() requires a non-nil pointer to a slice, got %T", slice)
	}

	sliceVal = sliceVal.Elem()
	if sliceVal.Kind() != reflect.Slice {
		return fmt.Errorf("relica: Column() requires a pointer to a slice, got pointer to %s", sliceVal.Kind())
	}

	elemType := sliceVal.Type().Elem()

	// Execute query — direct for tx, prepared for non-tx
	var rows *sql.Rows
	var err error
	if q.useDirectTx() {
		rows, err = q.tx.QueryContext(ctx, q.sql, q.params...)
	} else {
		var stmt *sql.Stmt
		stmt, err = q.prepareStatement(ctx)
		if err != nil {
			return err
		}
		rows, err = stmt.QueryContext(ctx, q.params...)
	}
	if err != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}
	defer func() { _ = rows.Close() }()

	// Scan all rows into slice
	rowCount := 0
	for rows.Next() {
		// Create a new element for this row
		elem := reflect.New(elemType)

		// Scan first column into element
		if err := rows.Scan(elem.Interface()); err != nil {
			elapsed := time.Since(start)
			q.db.invokeHook(ctx, QueryEvent{
				SQL:       q.sql,
				Args:      q.params,
				Duration:  elapsed,
				Error:     err,
				Operation: DetectOperation(q.sql),
			})
			return err
		}

		// Append dereferenced value to slice
		sliceVal.Set(reflect.Append(sliceVal, elem.Elem()))
		rowCount++
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}

	elapsed := time.Since(start)

	// Invoke query hook
	q.db.invokeHook(ctx, QueryEvent{
		SQL:       q.sql,
		Args:      q.params,
		Duration:  elapsed,
		Operation: DetectOperation(q.sql),
	})

	return nil
}

// All fetches all rows into a slice of structs.
// If query is part of a transaction, uses transaction connection.
func (q *Query) All(dest any) error {
	ctx := q.getContext()
	start := time.Now()

	if err := q.validateBeforeExec(ctx); err != nil {
		return err
	}

	// Execute query — direct for tx, prepared for non-tx
	var rows *sql.Rows
	var err error
	if q.useDirectTx() {
		rows, err = q.tx.QueryContext(ctx, q.sql, q.params...)
	} else {
		var stmt *sql.Stmt
		stmt, err = q.prepareStatement(ctx)
		if err != nil {
			return err
		}
		rows, err = stmt.QueryContext(ctx, q.params...)
	}
	if err != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     err,
			Operation: DetectOperation(q.sql),
		})
		return err
	}
	defer func() { _ = rows.Close() }()

	// Scan all rows - detect []NullStringMap for dynamic scanning
	var scanErr error
	if destSlice, ok := dest.(*[]NullStringMap); ok {
		scanErr = globalScanner.scanMapRows(rows, destSlice)
	} else {
		scanErr = globalScanner.scanRows(rows, dest)
	}
	if scanErr != nil {
		elapsed := time.Since(start)
		q.db.invokeHook(ctx, QueryEvent{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Error:     scanErr,
			Operation: DetectOperation(q.sql),
		})
		return scanErr
	}

	elapsed := time.Since(start)

	// Invoke query hook
	q.db.invokeHook(ctx, QueryEvent{
		SQL:       q.sql,
		Args:      q.params,
		Duration:  elapsed,
		Operation: DetectOperation(q.sql),
	})

	// Analyze query performance if optimizer is enabled (async to not block)
	if q.db.optimizer != nil {
		go q.analyzeQuery(ctx, elapsed)
	}

	return nil
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
	q.params = params
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
	// Process SQL to replace named placeholders with positional ones
	processedSQL, paramNames := q.db.processSQL(q.sql)
	q.sql = processedSQL

	// Bind parameters in order
	values, err := bindParams(params, paramNames)
	if err != nil {
		// Store error - will be returned on execution
		q.prepErr = err
		return q
	}

	q.params = values
	return q
}

// ToSQL returns the SQL string and parameters without executing the query.
func (q *Query) ToSQL() (string, []any) {
	return q.sql, q.params
}

// SQL returns the SQL query string.
func (q *Query) SQL() string {
	return q.sql
}

// Params returns the query parameters.
func (q *Query) Params() []any {
	return q.params
}
