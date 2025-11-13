// Package optimizer provides query optimization analysis and suggestions.
// It integrates with the analyzer package to detect performance issues and recommend fixes.
package optimizer

import (
	"context"
	"fmt"
	"time"

	"github.com/coregx/relica/internal/analyzer"
)

// Optimizer analyzes query performance and provides optimization suggestions.
type Optimizer interface {
	// Analyze examines a query's execution metrics and generates an analysis report.
	// It uses the analyzer package to get query plans and combines them with runtime metrics.
	Analyze(ctx context.Context, query string, args []interface{}, executionTime time.Duration) (*Analysis, error)

	// Suggest generates actionable optimization suggestions based on analysis results.
	Suggest(analysis *Analysis) []Suggestion
}

// Analysis represents the result of query optimization analysis.
type Analysis struct {
	// SlowQuery indicates if execution time exceeded the configured threshold
	SlowQuery bool

	// ExecutionTime is the actual query execution time
	ExecutionTime time.Duration

	// QueryPlan contains the EXPLAIN output from the analyzer
	QueryPlan *analyzer.QueryPlan

	// MissingIndexes contains recommended indexes to improve performance
	MissingIndexes []IndexRecommendation
}

// Suggestion represents an actionable optimization recommendation.
type Suggestion struct {
	// Type categorizes the suggestion (e.g., index_missing, slow_query)
	Type SuggestionType

	// Message is a human-readable description of the issue
	Message string

	// Severity indicates the importance of addressing this issue
	Severity Severity

	// SQL is optional SQL code to fix the issue (e.g., CREATE INDEX statement)
	SQL string
}

// String returns a formatted string representation of the suggestion.
func (s Suggestion) String() string {
	if s.SQL != "" {
		return fmt.Sprintf("%s: %s\n  Fix: %s", s.Severity, s.Message, s.SQL)
	}
	return fmt.Sprintf("%s: %s", s.Severity, s.Message)
}

// SuggestionType categorizes optimization suggestions.
type SuggestionType string

const (
	// SuggestionIndexMissing indicates a missing index that could improve performance
	SuggestionIndexMissing SuggestionType = "index_missing"

	// SuggestionSlowQuery indicates a query exceeded the slow query threshold
	SuggestionSlowQuery SuggestionType = "slow_query"

	// SuggestionFullScan indicates a full table scan is being performed
	SuggestionFullScan SuggestionType = "full_scan"
)

// Severity indicates the importance of an optimization suggestion.
type Severity string

const (
	// SeverityInfo is for informational suggestions with low priority
	SeverityInfo Severity = "info"

	// SeverityWarning is for moderate issues that should be addressed
	SeverityWarning Severity = "warning"

	// SeverityError is for critical issues requiring immediate attention
	SeverityError Severity = "error"
)

// IndexRecommendation represents a suggested index to improve query performance.
type IndexRecommendation struct {
	// Table is the table name where the index should be created
	Table string

	// Columns are the columns to include in the index (order matters)
	Columns []string

	// Type is the index type (e.g., "btree", "hash", "gin")
	Type string

	// Reason explains why this index is recommended
	Reason string
}
