package analyzer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SQLiteAnalyzer implements query analysis for SQLite databases.
type SQLiteAnalyzer struct {
	db *sql.DB
}

// NewSQLiteAnalyzer creates a new SQLite query analyzer.
func NewSQLiteAnalyzer(db *sql.DB) *SQLiteAnalyzer {
	return &SQLiteAnalyzer{db: db}
}

// Explain analyzes the query execution plan without executing the query.
// SQLite uses EXPLAIN QUERY PLAN which returns TEXT output (not JSON).
func (sa *SQLiteAnalyzer) Explain(ctx context.Context, query string, args []interface{}) (*QueryPlan, error) {
	explainQuery := fmt.Sprintf("EXPLAIN QUERY PLAN %s", query)
	return sa.executeExplain(ctx, explainQuery, args)
}

// ExplainAnalyze is not supported by SQLite.
// SQLite does not have EXPLAIN ANALYZE functionality.
func (sa *SQLiteAnalyzer) ExplainAnalyze(_ context.Context, _ string, _ []interface{}) (*QueryPlan, error) {
	return nil, fmt.Errorf("EXPLAIN ANALYZE not supported by SQLite (use Explain instead)")
}

// executeExplain runs the EXPLAIN QUERY PLAN and parses the result.
func (sa *SQLiteAnalyzer) executeExplain(ctx context.Context, explainQuery string, args []interface{}) (*QueryPlan, error) {
	rows, err := sa.db.QueryContext(ctx, explainQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute EXPLAIN QUERY PLAN: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close rows: %w", closeErr)
		}
	}()

	// Collect all plan rows
	var planLines []string
	for rows.Next() {
		var id, parent, notused int
		var detail string

		// SQLite EXPLAIN QUERY PLAN returns 4 columns: id, parent, notused, detail
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			return nil, fmt.Errorf("failed to scan EXPLAIN output: %w", err)
		}

		planLines = append(planLines, detail)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading EXPLAIN output: %w", err)
	}

	// Parse the plan lines
	plan := parseSQLiteExplain(planLines)
	plan.RawOutput = strings.Join(planLines, "\n")
	plan.Database = "sqlite"

	return plan, nil
}

// parseSQLiteExplain parses SQLite EXPLAIN QUERY PLAN text output.
// SQLite output examples:
//   - "SCAN users" (full table scan)
//   - "SEARCH users USING INDEX email_idx (email=?)" (index scan)
//   - "SEARCH users USING INTEGER PRIMARY KEY (rowid=?)" (primary key lookup)
func parseSQLiteExplain(planLines []string) *QueryPlan {
	plan := &QueryPlan{
		Cost:          0, // SQLite doesn't provide cost estimates
		EstimatedRows: 0, // SQLite doesn't provide row estimates
		UsesIndex:     false,
		FullScan:      false,
		Database:      "sqlite",
	}

	for _, line := range planLines {
		parseSQLitePlanLine(line, plan)
	}

	return plan
}

// parseSQLitePlanLine parses a single line from SQLite EXPLAIN QUERY PLAN output.
func parseSQLitePlanLine(line string, plan *QueryPlan) {
	// Normalize line: trim whitespace and convert to uppercase for matching
	upperLine := strings.ToUpper(strings.TrimSpace(line))

	// Check for various index usage patterns
	if detectIndexUsage(upperLine, line, plan) {
		return
	}

	// Check for full table scan
	if strings.HasPrefix(upperLine, "SCAN ") && !strings.Contains(upperLine, "USING") {
		plan.FullScan = true
	}
}

// detectIndexUsage checks if the line indicates index usage and updates plan.
// Returns true if index usage was detected.
func detectIndexUsage(upperLine, originalLine string, plan *QueryPlan) bool {
	// Check for covering index (index only scan)
	if strings.Contains(upperLine, "USING COVERING INDEX") {
		plan.UsesIndex = true
		setIndexNameIfEmpty(plan, extractIndexName(originalLine))
		return true
	}

	// Check for regular index usage
	if strings.Contains(upperLine, "USING INDEX") {
		plan.UsesIndex = true
		setIndexNameIfEmpty(plan, extractIndexName(originalLine))
		return true
	}

	// Check for primary key usage
	if strings.Contains(upperLine, "USING INTEGER PRIMARY KEY") {
		plan.UsesIndex = true
		setIndexNameIfEmpty(plan, "PRIMARY KEY")
		return true
	}

	// Check for automatic index (created by SQLite optimizer)
	if strings.Contains(upperLine, "USING AUTOMATIC") {
		plan.UsesIndex = true
		setIndexNameIfEmpty(plan, "AUTOMATIC INDEX")
		return true
	}

	return false
}

// setIndexNameIfEmpty sets the index name only if it hasn't been set yet.
func setIndexNameIfEmpty(plan *QueryPlan, indexName string) {
	if indexName != "" && plan.IndexName == "" {
		plan.IndexName = indexName
	}
}

// extractIndexName extracts the index name from SQLite EXPLAIN QUERY PLAN detail.
// Example: "SEARCH users USING INDEX email_idx (email=?)" -> "email_idx"
// Example: "USING COVERING INDEX idx_name" -> "idx_name"
func extractIndexName(detail string) string {
	// Look for "USING INDEX index_name" or "USING COVERING INDEX index_name"
	upperDetail := strings.ToUpper(detail)

	// Handle "USING COVERING INDEX"
	if idx := strings.Index(upperDetail, "USING COVERING INDEX "); idx != -1 {
		remainder := detail[idx+len("USING COVERING INDEX "):]
		return extractFirstWord(remainder)
	}

	// Handle "USING INDEX"
	if idx := strings.Index(upperDetail, "USING INDEX "); idx != -1 {
		remainder := detail[idx+len("USING INDEX "):]
		return extractFirstWord(remainder)
	}

	return ""
}

// extractFirstWord extracts the first word (index name) from a string.
// It stops at whitespace or opening parenthesis.
func extractFirstWord(s string) string {
	s = strings.TrimSpace(s)

	// Find first space or opening parenthesis
	endIdx := len(s)
	for i, ch := range s {
		if ch == ' ' || ch == '(' {
			endIdx = i
			break
		}
	}

	return s[:endIdx]
}
