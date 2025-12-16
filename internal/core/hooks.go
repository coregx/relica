// Package core provides the core database functionality for Relica.
package core

import (
	"context"
	"strings"
	"time"
)

// QueryEvent contains information about an executed query.
// This is passed to QueryHook callbacks for logging, metrics, or tracing.
type QueryEvent struct {
	// SQL is the executed SQL query string
	SQL string
	// Args are the query parameters
	Args []interface{}
	// Duration is how long the query took to execute
	Duration time.Duration
	// RowsAffected is the number of rows affected (for INSERT/UPDATE/DELETE)
	RowsAffected int64
	// Error is any error that occurred during query execution (nil on success)
	Error error
	// Operation is the SQL operation type (SELECT, INSERT, UPDATE, DELETE, UNKNOWN)
	Operation string
}

// QueryHook is a callback function invoked after each query execution.
// Use this for logging, metrics, distributed tracing, or debugging.
//
// Example:
//
//	db, _ := relica.Open("postgres", dsn,
//	    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
//	        slog.Info("query", "sql", e.SQL, "duration", e.Duration, "err", e.Error)
//	    }))
type QueryHook func(ctx context.Context, event QueryEvent)

// DetectOperation attempts to detect the SQL operation type from the query string.
// Returns one of: SELECT, INSERT, UPDATE, DELETE, or UNKNOWN.
func DetectOperation(sql string) string {
	sql = strings.TrimSpace(strings.ToUpper(sql))
	if strings.HasPrefix(sql, "SELECT") || strings.HasPrefix(sql, "WITH") {
		return "SELECT"
	}
	if strings.HasPrefix(sql, "INSERT") {
		return "INSERT"
	}
	if strings.HasPrefix(sql, "UPDATE") {
		return "UPDATE"
	}
	if strings.HasPrefix(sql, "DELETE") {
		return "DELETE"
	}
	return "UNKNOWN"
}

// invokeHook calls the query hook if set.
func (db *DB) invokeHook(ctx context.Context, event QueryEvent) {
	if db.queryHook != nil {
		db.queryHook(ctx, event)
	}
}
