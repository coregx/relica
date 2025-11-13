package optimizer

import (
	"strings"
)

// CoveringIndexAnalysis represents the result of covering index analysis.
// A covering index includes all columns needed by a query (both WHERE and SELECT),
// allowing the database to satisfy the query using only the index without accessing the table.
type CoveringIndexAnalysis struct {
	// Recommended indicates if a covering index would be beneficial
	Recommended bool

	// Columns are all columns needed: WHERE columns + SELECT columns
	Columns []string

	// Benefit explains why a covering index would help
	Benefit string
}

// AnalyzeCoveringIndex checks if a covering index would improve query performance.
// It examines both WHERE and SELECT clauses to determine if an index-only scan is possible.
//
// Sweet spot: 2-5 columns (enough to be useful, not too wide to be inefficient)
func AnalyzeCoveringIndex(query string) *CoveringIndexAnalysis {
	query = strings.ToLower(query)

	// Extract WHERE columns
	whereCols := extractWhereColumnsForCovering(query)

	// Extract SELECT columns
	selectCols := extractSelectColumns(query)

	// Filter out SELECT * (can't create covering index for all columns)
	if len(selectCols) == 1 && selectCols[0] == "*" {
		return &CoveringIndexAnalysis{
			Recommended: false,
			Benefit:     "SELECT * cannot be covered by index",
		}
	}

	// Combine WHERE and SELECT columns (deduplication)
	allCols := combineColumns(whereCols, selectCols)

	// Covering index is beneficial when:
	// 1. We have WHERE conditions (index will be used for filtering)
	// 2. Total columns are between 2-5 (sweet spot)
	// 3. Not too many columns (wide indexes are less efficient)
	if len(whereCols) > 0 && len(allCols) >= 2 && len(allCols) <= 5 {
		return &CoveringIndexAnalysis{
			Recommended: true,
			Columns:     allCols,
			Benefit:     "Index-only scan (no table access needed)",
		}
	}

	// Too few columns
	if len(allCols) < 2 {
		return &CoveringIndexAnalysis{
			Recommended: false,
			Benefit:     "Too few columns for covering index",
		}
	}

	// Too many columns
	if len(allCols) > 5 {
		return &CoveringIndexAnalysis{
			Recommended: false,
			Benefit:     "Too many columns (wide index would be inefficient)",
		}
	}

	// No WHERE clause (index won't be used for filtering)
	return &CoveringIndexAnalysis{
		Recommended: false,
		Benefit:     "No WHERE clause to benefit from index",
	}
}

// extractWhereColumnsForCovering extracts WHERE columns for covering index analysis.
// It uses the WHERE clause parser to get columns.
func extractWhereColumnsForCovering(sql string) []string {
	sql = strings.ToLower(sql)

	// Find WHERE keyword
	whereIdx := strings.Index(sql, " where ")
	if whereIdx == -1 {
		return nil
	}

	// Extract WHERE clause
	whereSQL := sql[whereIdx:]
	whereClause, err := ParseWhereClause(whereSQL)
	if err != nil || whereClause == nil {
		return nil
	}

	// Extract column names from conditions
	var columns []string
	seen := make(map[string]bool)

	for _, cond := range whereClause.Conditions {
		col := cond.Column
		if !seen[col] {
			columns = append(columns, col)
			seen[col] = true
		}
	}

	return columns
}

// combineColumns merges WHERE and SELECT columns, removing duplicates.
// WHERE columns come first (used for filtering), then SELECT columns (for coverage).
func combineColumns(whereCols, selectCols []string) []string {
	seen := make(map[string]bool)
	var result []string

	// Add WHERE columns first (order matters for composite indexes)
	for _, col := range whereCols {
		if !seen[col] {
			result = append(result, col)
			seen[col] = true
		}
	}

	// Add SELECT columns
	for _, col := range selectCols {
		if !seen[col] {
			result = append(result, col)
			seen[col] = true
		}
	}

	return result
}
