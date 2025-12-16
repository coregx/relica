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
	params   []interface{}
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

// prepareStatement prepares a SQL statement, using transaction or statement cache.
// For manually prepared queries (Prepare() called), uses the stored statement.
// For transactions, bypasses cache to avoid conflicts.
// For regular queries, uses LRU statement cache for better performance.
func (q *Query) prepareStatement(ctx context.Context) (*sql.Stmt, bool, error) {
	// Check for preparation error from Prepare() call
	if q.prepErr != nil {
		return nil, false, q.prepErr
	}

	// Use manually prepared statement if available
	if q.prepared && q.stmt != nil {
		return q.stmt, false, nil // false = don't close, user manages lifecycle
	}

	if q.tx != nil {
		// Transactions bypass statement cache
		stmt, err := q.tx.PrepareContext(ctx, q.sql)
		if err != nil {
			return nil, false, err
		}
		return stmt, true, nil // true = needs close
	}

	// Use statement cache for non-transactional queries
	if stmt, ok := q.db.stmtCache.Get(q.sql); ok {
		return stmt, false, nil // false = cached, don't close
	}

	stmt, err := q.db.sqlDB.PrepareContext(ctx, q.sql)
	if err != nil {
		return nil, false, err
	}
	q.db.stmtCache.Set(q.sql, stmt)
	return stmt, false, nil // false = cached, don't close
}

// logExecutionResult logs query execution results if logger is enabled.
func (q *Query) logExecutionResult(result sql.Result, err error, elapsed time.Duration) {
	if q.db.logger == nil {
		return
	}

	maskedParams := q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params))

	if err != nil {
		q.db.logger.Error("query execution failed",
			"sql", q.sql,
			"params", maskedParams,
			"duration_ms", elapsed.Milliseconds(),
			"database", q.db.driverName,
			"error", err,
		)
		return
	}

	var rowsAffected int64
	if result != nil {
		rowsAffected, _ = result.RowsAffected()
	}
	q.db.logger.Info("query executed",
		"sql", q.sql,
		"params", maskedParams,
		"duration_ms", elapsed.Milliseconds(),
		"rows_affected", rowsAffected,
		"database", q.db.driverName,
	)
}

// Execute runs the query and returns results.
// If query is part of a transaction, bypasses statement cache and uses transaction connection.
func (q *Query) Execute() (sql.Result, error) {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		if q.db.logger != nil {
			q.db.logger.Error("query preparation failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"error", err,
			)
		}
		return nil, err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	result, err := stmt.ExecContext(ctx, q.params...)
	elapsed := time.Since(start)

	// Log query execution
	q.logExecutionResult(result, err, elapsed)

	// Invoke query hook
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
//
//nolint:cyclop,funlen // Query execution requires comprehensive error handling and logging
func (q *Query) One(dest interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		if q.db.logger != nil {
			q.db.logger.Error("query preparation failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"error", err,
			)
		}
		return err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	// Execute query
	rows, err := stmt.QueryContext(ctx, q.params...)
	if err != nil {
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Error("query execution failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
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
		err := sql.ErrNoRows
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Warn("query returned no rows",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
			)
		}
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
		if q.db.logger != nil {
			q.db.logger.Error("row scanning failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", scanErr,
			)
		}
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

	// Log success
	if q.db.logger != nil {
		q.db.logger.Info("query executed",
			"sql", q.sql,
			"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
			"duration_ms", elapsed.Milliseconds(),
			"rows", 1,
			"database", q.db.driverName,
		)
	}

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
// Returns sql.ErrNoRows if no rows are found.
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
//
//nolint:cyclop,funlen // Query execution requires comprehensive error handling and logging
func (q *Query) Row(dest ...interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		if q.db.logger != nil {
			q.db.logger.Error("query preparation failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"error", err,
			)
		}
		return err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	// Execute query
	rows, err := stmt.QueryContext(ctx, q.params...)
	if err != nil {
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Error("query execution failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
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
			err = sql.ErrNoRows
		}
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Warn("query returned no rows",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
			)
		}
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
		if q.db.logger != nil {
			q.db.logger.Error("row scanning failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
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

	// Log success
	if q.db.logger != nil {
		q.db.logger.Info("query executed",
			"sql", q.sql,
			"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
			"duration_ms", elapsed.Milliseconds(),
			"rows", 1,
			"database", q.db.driverName,
		)
	}

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
//nolint:gocognit,gocyclo,cyclop,funlen // Query execution requires comprehensive error handling and logging
func (q *Query) Column(slice interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()

	// Validate slice parameter
	sliceVal := reflect.ValueOf(slice)
	if sliceVal.Kind() != reflect.Ptr || sliceVal.IsNil() {
		return fmt.Errorf("relica: Column() requires a non-nil pointer to a slice, got %T", slice)
	}

	sliceVal = sliceVal.Elem()
	if sliceVal.Kind() != reflect.Slice {
		return fmt.Errorf("relica: Column() requires a pointer to a slice, got pointer to %s", sliceVal.Kind())
	}

	elemType := sliceVal.Type().Elem()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		if q.db.logger != nil {
			q.db.logger.Error("query preparation failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"error", err,
			)
		}
		return err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	// Execute query
	rows, err := stmt.QueryContext(ctx, q.params...)
	if err != nil {
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Error("query execution failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
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
			if q.db.logger != nil {
				q.db.logger.Error("column scanning failed",
					"sql", q.sql,
					"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
					"duration_ms", elapsed.Milliseconds(),
					"row", rowCount,
					"error", err,
				)
			}
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
		if q.db.logger != nil {
			q.db.logger.Error("row iteration failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
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

	// Log success
	if q.db.logger != nil {
		q.db.logger.Info("query executed",
			"sql", q.sql,
			"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
			"duration_ms", elapsed.Milliseconds(),
			"rows", rowCount,
			"database", q.db.driverName,
		)
	}

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
//
//nolint:cyclop // Query execution requires comprehensive error handling and logging
func (q *Query) All(dest interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		if q.db.logger != nil {
			q.db.logger.Error("query preparation failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"error", err,
			)
		}
		return err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	// Execute query
	rows, err := stmt.QueryContext(ctx, q.params...)
	if err != nil {
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Error("query execution failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
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
		if q.db.logger != nil {
			q.db.logger.Error("row scanning failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", scanErr,
			)
		}
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

	// Log success
	if q.db.logger != nil {
		q.db.logger.Info("query executed",
			"sql", q.sql,
			"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
			"duration_ms", elapsed.Milliseconds(),
			"database", q.db.driverName,
		)
	}

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
func (q *Query) Bind(params ...interface{}) *Query {
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

// SQL returns the SQL query string.
func (q *Query) SQL() string {
	return q.sql
}

// Params returns the query parameters.
func (q *Query) Params() []interface{} {
	return q.params
}
