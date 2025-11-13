// Package analyzer provides database query plan analysis using EXPLAIN functionality.
// It supports PostgreSQL, MySQL, and SQLite with unified QueryPlan structure.
package analyzer

import (
	"context"
	"database/sql"
	"time"
)

// QueryPlan represents a unified query execution plan across different databases.
// It provides performance metrics, index usage analysis, and database-specific details.
type QueryPlan struct {
	// Common fields across all databases
	Cost          float64       // Estimated query cost (database-specific units)
	EstimatedRows int64         // Estimated number of rows to be processed
	ActualRows    int64         // Actual rows processed (EXPLAIN ANALYZE only, 0 if not available)
	ActualTime    time.Duration // Actual execution time (EXPLAIN ANALYZE only, 0 if not available)

	// Index analysis
	UsesIndex bool   // true if query uses any index
	IndexName string // Primary index name (empty if multiple or none)
	FullScan  bool   // true if full table scan is performed

	// Database-specific metadata
	RawOutput string // Full EXPLAIN output from database
	Database  string // Database type: "postgres", "mysql", "sqlite"

	// Performance indicators (database-specific)
	BuffersHit   int64 // PostgreSQL: buffer cache hits
	BuffersMiss  int64 // PostgreSQL: buffer cache misses
	RowsExamined int64 // MySQL: rows examined during execution
	RowsProduced int64 // MySQL: rows produced by the query
}

// Analyzer provides query plan analysis for specific database dialects.
type Analyzer interface {
	// Explain analyzes query execution plan without executing the query.
	// Returns QueryPlan with estimated metrics.
	Explain(ctx context.Context, query string, args []interface{}) (*QueryPlan, error)

	// ExplainAnalyze analyzes query execution plan AND executes the query.
	// Returns QueryPlan with actual execution metrics.
	// Not all databases support this (e.g., older MySQL versions).
	ExplainAnalyze(ctx context.Context, query string, args []interface{}) (*QueryPlan, error)
}

// ExplainMode indicates the type of EXPLAIN operation.
type ExplainMode int

const (
	// ExplainNone indicates no EXPLAIN operation.
	ExplainNone ExplainMode = iota
	// ExplainPlan indicates EXPLAIN without execution (estimates only).
	ExplainPlan
	// ExplainAnalyze indicates EXPLAIN with execution (actual metrics).
	ExplainAnalyze
)

// Factory creates database-specific analyzers.
type Factory interface {
	// NewAnalyzer creates an analyzer for the given database connection.
	NewAnalyzer(db *sql.DB, driverName string) (Analyzer, error)
}
