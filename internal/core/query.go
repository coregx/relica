package core

import (
	"context"
	"database/sql"
	"time"

	"github.com/coregx/relica/internal/tracer"
)

// Query represents a database query with tracing.
// When tx is not nil, the query executes within that transaction.
type Query struct {
	sql    string
	params []interface{}
	db     *DB
	tx     *sql.Tx // nil for non-transactional queries
	ctx    context.Context
}

// appendSQL appends a suffix to the SQL query.
// This is used internally for PostgreSQL RETURNING clause.
func (q *Query) appendSQL(suffix string) {
	q.sql += suffix
}

// prepareStatement prepares a SQL statement, using transaction or statement cache.
// For transactions, bypasses cache to avoid conflicts.
// For regular queries, uses LRU statement cache for better performance.
func (q *Query) prepareStatement(ctx context.Context) (*sql.Stmt, bool, error) {
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

	// Start tracing span
	ctx, span := q.db.oldTracer.Start(ctx, "Query.Execute")
	defer span.End()

	// Start new tracer span
	var newSpan tracer.Span
	if q.db.tracer != nil {
		ctx, newSpan = q.db.tracer.StartSpan(ctx, "relica.query.execute")
		defer newSpan.End()
	}

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		// Log error
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

	// Add tracing attributes
	if newSpan != nil {
		var rowsAffected int64
		if result != nil {
			rowsAffected, _ = result.RowsAffected()
		}
		tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
			SQL:          q.sql,
			Args:         q.params,
			Duration:     elapsed,
			RowsAffected: rowsAffected,
			Error:        err,
			Database:     q.db.driverName,
			Operation:    tracer.DetectOperation(q.sql),
		})
	}

	q.db.oldTracer.Record(ctx, elapsed, err)
	return result, err
}

// One fetches a single row into a struct.
// If query is part of a transaction, uses transaction connection.
//
//nolint:cyclop,gocyclo,gocognit,funlen // Query execution requires comprehensive error handling, logging, and tracing
func (q *Query) One(dest interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Old tracer (backward compatibility)
	ctx, span := q.db.oldTracer.Start(ctx, "Query.One")
	defer span.End()

	// Start new tracer span
	var newSpan tracer.Span
	if q.db.tracer != nil {
		ctx, newSpan = q.db.tracer.StartSpan(ctx, "relica.query.one")
		defer newSpan.End()
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
		if newSpan != nil {
			tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
				SQL:       q.sql,
				Args:      q.params,
				Duration:  elapsed,
				Error:     err,
				Database:  q.db.driverName,
				Operation: tracer.DetectOperation(q.sql),
			})
		}
		q.db.oldTracer.Record(ctx, elapsed, err)
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
		if newSpan != nil {
			tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
				SQL:       q.sql,
				Args:      q.params,
				Duration:  elapsed,
				Error:     err,
				Database:  q.db.driverName,
				Operation: tracer.DetectOperation(q.sql),
			})
		}
		q.db.oldTracer.Record(ctx, elapsed, err)
		return err
	}

	// Scan into dest
	if err := globalScanner.scanRow(rows, dest); err != nil {
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Error("row scanning failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
		if newSpan != nil {
			tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
				SQL:       q.sql,
				Args:      q.params,
				Duration:  elapsed,
				Error:     err,
				Database:  q.db.driverName,
				Operation: tracer.DetectOperation(q.sql),
			})
		}
		q.db.oldTracer.Record(ctx, elapsed, err)
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

	// Add tracing attributes
	if newSpan != nil {
		tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Database:  q.db.driverName,
			Operation: tracer.DetectOperation(q.sql),
		})
	}

	q.db.oldTracer.Record(ctx, elapsed, nil)

	// Analyze query performance if optimizer is enabled (async to not block)
	if q.db.optimizer != nil {
		go q.analyzeQuery(ctx, elapsed)
	}

	return nil
}

// All fetches all rows into a slice of structs.
// If query is part of a transaction, uses transaction connection.
//
//nolint:cyclop,funlen // Query execution requires comprehensive error handling, logging, and tracing
func (q *Query) All(dest interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Old tracer (backward compatibility)
	ctx, span := q.db.oldTracer.Start(ctx, "Query.All")
	defer span.End()

	// Start new tracer span
	var newSpan tracer.Span
	if q.db.tracer != nil {
		ctx, newSpan = q.db.tracer.StartSpan(ctx, "relica.query.all")
		defer newSpan.End()
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
		if newSpan != nil {
			tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
				SQL:       q.sql,
				Args:      q.params,
				Duration:  elapsed,
				Error:     err,
				Database:  q.db.driverName,
				Operation: tracer.DetectOperation(q.sql),
			})
		}
		q.db.oldTracer.Record(ctx, elapsed, err)
		return err
	}
	defer func() { _ = rows.Close() }()

	// Scan all rows
	if err := globalScanner.scanRows(rows, dest); err != nil {
		elapsed := time.Since(start)
		if q.db.logger != nil {
			q.db.logger.Error("row scanning failed",
				"sql", q.sql,
				"params", q.db.sanitizer.FormatParams(q.db.sanitizer.MaskParams(q.sql, q.params)),
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
		}
		if newSpan != nil {
			tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
				SQL:       q.sql,
				Args:      q.params,
				Duration:  elapsed,
				Error:     err,
				Database:  q.db.driverName,
				Operation: tracer.DetectOperation(q.sql),
			})
		}
		q.db.oldTracer.Record(ctx, elapsed, err)
		return err
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

	// Add tracing attributes
	if newSpan != nil {
		tracer.AddQueryAttributes(newSpan, &tracer.QueryMetadata{
			SQL:       q.sql,
			Args:      q.params,
			Duration:  elapsed,
			Database:  q.db.driverName,
			Operation: tracer.DetectOperation(q.sql),
		})
	}

	q.db.oldTracer.Record(ctx, elapsed, nil)

	// Analyze query performance if optimizer is enabled (async to not block)
	if q.db.optimizer != nil {
		go q.analyzeQuery(ctx, elapsed)
	}

	return nil
}
