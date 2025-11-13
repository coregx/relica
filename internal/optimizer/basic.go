package optimizer

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/coregx/relica/internal/analyzer"
)

// BasicOptimizer provides fundamental query optimization analysis.
// It detects slow queries, full table scans, and recommends missing indexes.
type BasicOptimizer struct {
	analyzer           analyzer.Analyzer
	slowQueryThreshold time.Duration
}

// NewBasicOptimizer creates a new BasicOptimizer with the given analyzer and slow query threshold.
// If threshold is 0 or negative, defaults to 100ms.
func NewBasicOptimizer(queryAnalyzer analyzer.Analyzer, threshold time.Duration) *BasicOptimizer {
	if threshold <= 0 {
		threshold = 100 * time.Millisecond
	}
	return &BasicOptimizer{
		analyzer:           queryAnalyzer,
		slowQueryThreshold: threshold,
	}
}

// Analyze examines query performance and generates an analysis report.
func (o *BasicOptimizer) Analyze(ctx context.Context, query string, args []interface{}, executionTime time.Duration) (*Analysis, error) {
	// Get query plan from analyzer
	plan, err := o.analyzer.Explain(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get query plan: %w", err)
	}

	// Build analysis
	analysis := &Analysis{
		SlowQuery:     executionTime > o.slowQueryThreshold,
		ExecutionTime: executionTime,
		QueryPlan:     plan,
	}

	// Detect missing indexes if full scan detected
	if plan.FullScan {
		analysis.MissingIndexes = o.detectMissingIndexes(query, plan)
	}

	return analysis, nil
}

// Suggest generates optimization suggestions based on analysis results.
func (o *BasicOptimizer) Suggest(analysis *Analysis) []Suggestion {
	// Pre-allocate slice with estimated capacity
	suggestions := make([]Suggestion, 0, 3)

	// Slow query warning
	if analysis.SlowQuery {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionSlowQuery,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("Query took %v (threshold: %v)", analysis.ExecutionTime, o.slowQueryThreshold),
		})
	}

	// Full scan warning
	if analysis.QueryPlan.FullScan {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionFullScan,
			Severity: SeverityWarning,
			Message:  "Query is performing a full table scan",
		})
	}

	// Index recommendations
	for _, idx := range analysis.MissingIndexes {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionIndexMissing,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("Consider adding index on %s(%s): %s", idx.Table, strings.Join(idx.Columns, ", "), idx.Reason),
			SQL:      generateIndexSQL(idx),
		})
	}

	return suggestions
}

// detectMissingIndexes analyzes the query to recommend indexes.
// This is a basic implementation that parses WHERE clauses.
func (o *BasicOptimizer) detectMissingIndexes(query string, _ *analyzer.QueryPlan) []IndexRecommendation {
	var recommendations []IndexRecommendation

	// Extract table name from query
	table := extractTableName(query)
	if table == "" {
		return recommendations
	}

	// Extract WHERE clause columns
	whereColumns := extractWhereColumns(query)
	if len(whereColumns) == 0 {
		return recommendations
	}

	// Recommend index on WHERE columns
	recommendations = append(recommendations, IndexRecommendation{
		Table:   table,
		Columns: whereColumns,
		Type:    "btree", // Default to btree for most databases
		Reason:  "WHERE clause filtering without index usage",
	})

	return recommendations
}

// extractTableName extracts the table name from a SELECT query.
// This is a simplified parser for basic queries.
func extractTableName(query string) string {
	query = strings.ToLower(query)

	// Match: SELECT ... FROM table_name
	re := regexp.MustCompile(`\bfrom\s+([a-z_][a-z0-9_]*)\b`)
	matches := re.FindStringSubmatch(query)
	if len(matches) >= 2 {
		return matches[1]
	}

	return ""
}

// extractWhereColumns extracts column names from WHERE clause.
// This is a simplified parser that handles basic equality and comparison operators.
func extractWhereColumns(query string) []string {
	query = strings.ToLower(query)

	// Find WHERE clause
	whereIdx := strings.Index(query, "where")
	if whereIdx == -1 {
		return nil
	}

	// Extract everything after WHERE until ORDER BY, GROUP BY, LIMIT, or end
	whereClause := query[whereIdx+5:]
	for _, terminator := range []string{"order by", "group by", "limit", ";"} {
		if idx := strings.Index(whereClause, terminator); idx != -1 {
			whereClause = whereClause[:idx]
		}
	}

	// Extract column names (before operators like =, >, <, etc.)
	// Pattern: column_name [operator]
	re := regexp.MustCompile(`\b([a-z_][a-z0-9_]*)\s*(?:=|>|<|>=|<=|!=|<>|like|in)\s*`)
	matches := re.FindAllStringSubmatch(whereClause, -1)

	// Collect unique columns
	seen := make(map[string]bool)
	var columns []string
	for _, match := range matches {
		if len(match) >= 2 {
			col := match[1]
			// Filter out SQL keywords
			if !isSQLKeyword(col) && !seen[col] {
				columns = append(columns, col)
				seen[col] = true
			}
		}
	}

	return columns
}

// isSQLKeyword checks if a word is a common SQL keyword.
func isSQLKeyword(word string) bool {
	keywords := map[string]bool{
		"and": true, "or": true, "not": true, "null": true,
		"true": true, "false": true, "case": true, "when": true,
		"then": true, "else": true, "end": true, "exists": true,
	}
	return keywords[word]
}

// generateIndexSQL generates a CREATE INDEX statement for the recommendation.
func generateIndexSQL(idx IndexRecommendation) string {
	indexName := fmt.Sprintf("idx_%s_%s", idx.Table, strings.Join(idx.Columns, "_"))
	columnList := strings.Join(idx.Columns, ", ")

	return fmt.Sprintf("CREATE INDEX %s ON %s(%s);", indexName, idx.Table, columnList)
}
