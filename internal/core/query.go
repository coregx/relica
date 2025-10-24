package core

import (
	"context"
	"database/sql"
	"time"
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

// Execute runs the query and returns results.
// If query is part of a transaction, bypasses statement cache and uses transaction connection.
func (q *Query) Execute() (sql.Result, error) {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, span := q.db.tracer.Start(ctx, "Query.Execute")
	defer span.End()

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		return nil, err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	result, err := stmt.ExecContext(ctx, q.params...)
	q.db.tracer.Record(ctx, time.Since(start), err)
	return result, err
}

// One fetches a single row into a struct.
// If query is part of a transaction, uses transaction connection.
func (q *Query) One(dest interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, span := q.db.tracer.Start(ctx, "Query.One")
	defer span.End()

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		return err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	// Execute query
	rows, err := stmt.QueryContext(ctx, q.params...)
	if err != nil {
		q.db.tracer.Record(ctx, time.Since(start), err)
		return err
	}
	defer func() { _ = rows.Close() }()

	// Check if row exists
	if !rows.Next() {
		err := sql.ErrNoRows
		q.db.tracer.Record(ctx, time.Since(start), err)
		return err
	}

	// Scan into dest
	if err := globalScanner.scanRow(rows, dest); err != nil {
		q.db.tracer.Record(ctx, time.Since(start), err)
		return err
	}

	q.db.tracer.Record(ctx, time.Since(start), nil)
	return nil
}

// All fetches all rows into a slice of structs.
// If query is part of a transaction, uses transaction connection.
func (q *Query) All(dest interface{}) error {
	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, span := q.db.tracer.Start(ctx, "Query.All")
	defer span.End()

	start := time.Now()

	stmt, needsClose, err := q.prepareStatement(ctx)
	if err != nil {
		return err
	}
	if needsClose {
		defer func() { _ = stmt.Close() }()
	}

	// Execute query
	rows, err := stmt.QueryContext(ctx, q.params...)
	if err != nil {
		q.db.tracer.Record(ctx, time.Since(start), err)
		return err
	}
	defer func() { _ = rows.Close() }()

	// Scan all rows
	if err := globalScanner.scanRows(rows, dest); err != nil {
		q.db.tracer.Record(ctx, time.Since(start), err)
		return err
	}

	q.db.tracer.Record(ctx, time.Since(start), nil)
	return nil
}
