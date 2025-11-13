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
// Phase 2: Enhanced with composite, covering, JOIN, and function-based index suggestions.
func (o *BasicOptimizer) Suggest(analysis *Analysis) []Suggestion {
	// Pre-allocate slice with estimated capacity
	suggestions := make([]Suggestion, 0, 10)

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

	// Index recommendations (Phase 2: categorize by type)
	for _, idx := range analysis.MissingIndexes {
		suggestionType := categorizeIndexRecommendation(idx)

		suggestions = append(suggestions, Suggestion{
			Type:     suggestionType,
			Severity: determineSeverity(suggestionType),
			Message:  fmt.Sprintf("%s on %s(%s): %s", suggestionTypeMessage(suggestionType), idx.Table, strings.Join(idx.Columns, ", "), idx.Reason),
			SQL:      generateIndexSQL(idx),
		})
	}

	return suggestions
}

// categorizeIndexRecommendation determines the suggestion type based on index recommendation.
func categorizeIndexRecommendation(idx IndexRecommendation) SuggestionType {
	reason := strings.ToLower(idx.Reason)

	// Covering index
	if strings.Contains(reason, "covering index") {
		return SuggestionCoveringIndex
	}

	// Composite index
	if len(idx.Columns) > 1 && strings.Contains(reason, "composite") {
		return SuggestionCompositeIndex
	}

	// JOIN optimization
	if strings.Contains(reason, "join") || strings.Contains(reason, "foreign key") {
		return SuggestionJoinOptimize
	}

	// Function-based index
	if strings.Contains(reason, "function") {
		return SuggestionFunctionIndex
	}

	// Default: missing index
	return SuggestionIndexMissing
}

// determineSeverity determines severity based on suggestion type.
func determineSeverity(suggestionType SuggestionType) Severity {
	switch suggestionType {
	case SuggestionFunctionIndex:
		return SeverityWarning
	case SuggestionCompositeIndex, SuggestionJoinOptimize:
		return SeverityWarning
	case SuggestionCoveringIndex:
		return SeverityInfo
	default:
		return SeverityWarning
	}
}

// suggestionTypeMessage returns a human-readable prefix for suggestion type.
func suggestionTypeMessage(suggestionType SuggestionType) string {
	switch suggestionType {
	case SuggestionCompositeIndex:
		return "Composite index recommended"
	case SuggestionCoveringIndex:
		return "Covering index recommended"
	case SuggestionJoinOptimize:
		return "Index recommended"
	case SuggestionFunctionIndex:
		return "Function-based index recommended"
	default:
		return "Index recommended"
	}
}

// detectMissingIndexes analyzes the query to recommend indexes.
// Phase 2: Enhanced with composite index, JOIN, and covering index analysis.
func (o *BasicOptimizer) detectMissingIndexes(query string, _ *analyzer.QueryPlan) []IndexRecommendation {
	var recommendations []IndexRecommendation

	// Extract table name from query
	table := extractTableName(query)
	if table == "" {
		return recommendations
	}

	// Parse WHERE clause
	whereText := extractWhereClauseText(strings.ToLower(query))
	if whereText != "" {
		whereClause, err := ParseWhereClause("where " + whereText)
		if err == nil && whereClause != nil {
			// Analyze WHERE conditions
			recommendations = append(recommendations, o.analyzeWhereIndexes(whereClause, table)...)
		}
	}

	// Analyze JOIN clauses
	joins := extractJoinClauses(query)
	for _, join := range joins {
		recommendations = append(recommendations, o.analyzeJoinIndexes(join)...)
	}

	// Analyze covering index opportunities
	coveringAnalysis := AnalyzeCoveringIndex(query)
	if coveringAnalysis.Recommended {
		recommendations = append(recommendations, IndexRecommendation{
			Table:   table,
			Columns: coveringAnalysis.Columns,
			Type:    "btree",
			Reason:  fmt.Sprintf("Covering index: %s", coveringAnalysis.Benefit),
		})
	}

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

// analyzeWhereIndexes recommends indexes based on WHERE conditions.
// Phase 2: Detects single-column, composite, and function-based index opportunities.
func (o *BasicOptimizer) analyzeWhereIndexes(where *WhereClause, table string) []IndexRecommendation {
	if len(where.Conditions) == 0 {
		return nil
	}

	var recommendations []IndexRecommendation
	compositeColumns, hasFunctions := o.analyzeConditions(where.Conditions, table, &recommendations)

	// Add composite or single column index recommendations
	recommendations = append(recommendations, o.buildCompositeRecommendation(where.Logic, compositeColumns, hasFunctions, table)...)

	return recommendations
}

// analyzeConditions analyzes individual conditions and returns columns for composite index.
func (o *BasicOptimizer) analyzeConditions(conditions []Condition, table string, recommendations *[]IndexRecommendation) ([]string, bool) {
	var compositeColumns []string
	hasFunctions := false

	for _, cond := range conditions {
		if cond.Function != "" {
			hasFunctions = true
			*recommendations = append(*recommendations, IndexRecommendation{
				Table:   table,
				Columns: []string{cond.Column},
				Type:    "btree",
				Reason:  fmt.Sprintf("Function %s() in WHERE prevents index use - consider function-based index", cond.Function),
			})
			continue
		}

		if cond.Operator == "=" || cond.Operator == "IN" {
			compositeColumns = append(compositeColumns, cond.Column)
		}
	}

	return compositeColumns, hasFunctions
}

// buildCompositeRecommendation creates composite or single column index recommendations.
func (o *BasicOptimizer) buildCompositeRecommendation(logic LogicType, columns []string, hasFunctions bool, table string) []IndexRecommendation {
	if logic != LogicAND || len(columns) == 0 || hasFunctions {
		return nil
	}

	reason := "Single column index for WHERE filtering"
	if len(columns) >= 2 {
		reason = "Composite index for multiple AND conditions"
	}

	return []IndexRecommendation{{
		Table:   table,
		Columns: columns,
		Type:    "btree",
		Reason:  reason,
	}}
}

// analyzeJoinIndexes recommends indexes for JOIN columns.
// Phase 2: Detects missing foreign key indexes.
func (o *BasicOptimizer) analyzeJoinIndexes(join string) []IndexRecommendation {
	var recommendations []IndexRecommendation

	// Extract JOIN ON columns
	leftCol, rightCol := extractJoinColumns(join)
	if leftCol == "" || rightCol == "" {
		return recommendations
	}

	// Recommend index on the right table (foreign key side)
	// Example: users.id = orders.user_id -> index on orders(user_id)
	rightTable := extractTableNameFromColumn(rightCol)
	rightColumn := extractColumnName(rightCol)

	if rightTable != "" && rightColumn != "" {
		recommendations = append(recommendations, IndexRecommendation{
			Table:   rightTable,
			Columns: []string{rightColumn},
			Type:    "btree",
			Reason:  "JOIN condition - index on foreign key",
		})
	}

	return recommendations
}

// generateIndexSQL generates a CREATE INDEX statement for the recommendation.
func generateIndexSQL(idx IndexRecommendation) string {
	indexName := fmt.Sprintf("idx_%s_%s", idx.Table, strings.Join(idx.Columns, "_"))
	columnList := strings.Join(idx.Columns, ", ")

	return fmt.Sprintf("CREATE INDEX %s ON %s(%s);", indexName, idx.Table, columnList)
}
