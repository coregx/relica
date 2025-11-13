package optimizer

import (
	"fmt"
)

// DatabaseHints provides database-specific optimization suggestions.
// It enhances the basic optimizer with database-aware recommendations
// for PostgreSQL, MySQL, and SQLite.
type DatabaseHints struct {
	database string
}

// NewDatabaseHints creates a new DatabaseHints instance for the specified database.
// database must be one of: "postgres", "mysql", "sqlite".
func NewDatabaseHints(database string) *DatabaseHints {
	return &DatabaseHints{database: database}
}

// SuggestPostgreSQLHints returns PostgreSQL-specific optimization suggestions.
//
// PostgreSQL-specific optimizations include:
// - ANALYZE table for statistics updates
// - Parallel query configuration for large scans
// - GIN/GiST indexes for text search and JSONB
// - Partial indexes for filtered subsets
//
// Example:
//
//	hints := NewDatabaseHints("postgres")
//	suggestions := hints.SuggestPostgreSQLHints(analysis)
//	for _, s := range suggestions {
//	    fmt.Println(s.String())
//	}
func (h *DatabaseHints) SuggestPostgreSQLHints(analysis *Analysis) []Suggestion {
	var suggestions []Suggestion

	// ANALYZE table suggestion if full scan detected
	if analysis.QueryPlan.FullScan {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionPostgresAnalyze,
			Severity: SeverityInfo,
			Message:  "Full scan detected - consider running ANALYZE to update table statistics",
			SQL:      "ANALYZE table_name;",
		})
	}

	// Parallel query suggestion for large scans
	if analysis.QueryPlan.EstimatedRows > 100000 {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionPostgresParallel,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Large scan detected (%d estimated rows) - verify parallel query is enabled", analysis.QueryPlan.EstimatedRows),
			SQL:      "SET max_parallel_workers_per_gather = 4;",
		})
	}

	// Buffer cache analysis (PostgreSQL-specific)
	if analysis.QueryPlan.BuffersMiss > 0 && analysis.QueryPlan.BuffersHit > 0 {
		hitRatio := float64(analysis.QueryPlan.BuffersHit) / float64(analysis.QueryPlan.BuffersHit+analysis.QueryPlan.BuffersMiss)
		if hitRatio < 0.90 { // Less than 90% cache hit ratio
			suggestions = append(suggestions, Suggestion{
				Type:     SuggestionPostgresCacheHit,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Low buffer cache hit ratio: %.1f%% - consider increasing shared_buffers", hitRatio*100),
				SQL:      "-- ALTER SYSTEM SET shared_buffers = '4GB'; (requires restart)",
			})
		}
	}

	return suggestions
}

// SuggestMySQLHints returns MySQL-specific optimization suggestions.
//
// MySQL-specific optimizations include:
// - Index hints (USE INDEX, FORCE INDEX)
// - OPTIMIZE TABLE for fragmentation
// - Query cache considerations
// - InnoDB buffer pool tuning
//
// Example:
//
//	hints := NewDatabaseHints("mysql")
//	suggestions := hints.SuggestMySQLHints(analysis)
func (h *DatabaseHints) SuggestMySQLHints(analysis *Analysis) []Suggestion {
	var suggestions []Suggestion

	// Index hint suggestion: USE INDEX
	if len(analysis.MissingIndexes) > 0 && analysis.QueryPlan.FullScan {
		idx := analysis.MissingIndexes[0]
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionMySQLIndexHint,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("MySQL index hint: After creating index, use USE INDEX (%s) to force usage", idx.IndexName()),
		})
	}

	// OPTIMIZE TABLE suggestion for large row examinations
	if analysis.QueryPlan.RowsExamined > analysis.QueryPlan.RowsProduced*10 && analysis.QueryPlan.RowsExamined > 10000 {
		divisor := analysis.QueryPlan.RowsProduced
		if divisor == 0 {
			divisor = 1
		}
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionMySQLOptimize,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Examining %dx more rows than produced - consider OPTIMIZE TABLE for defragmentation", analysis.QueryPlan.RowsExamined/divisor),
			SQL:      "OPTIMIZE TABLE table_name;",
		})
	}

	// InnoDB buffer pool hint for large scans
	if analysis.QueryPlan.EstimatedRows > 500000 {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionMySQLBufferPool,
			Severity: SeverityInfo,
			Message:  "Large table scan detected - ensure InnoDB buffer pool is adequately sized",
			SQL:      "-- SET GLOBAL innodb_buffer_pool_size = 8G; (requires restart for optimal effect)",
		})
	}

	return suggestions
}

// SuggestSQLiteHints returns SQLite-specific optimization suggestions.
//
// SQLite-specific optimizations include:
// - ANALYZE for query planner statistics
// - VACUUM for database defragmentation
// - Pragma optimizations (page_size, cache_size)
// - WAL mode for concurrency
//
// Example:
//
//	hints := NewDatabaseHints("sqlite")
//	suggestions := hints.SuggestSQLiteHints(analysis)
func (h *DatabaseHints) SuggestSQLiteHints(analysis *Analysis) []Suggestion {
	var suggestions []Suggestion

	// ANALYZE suggestion for SQLite
	if analysis.QueryPlan.FullScan {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionSQLiteAnalyze,
			Severity: SeverityInfo,
			Message:  "Full scan detected - run ANALYZE to improve query planner decisions",
			SQL:      "ANALYZE;",
		})
	}

	// VACUUM suggestion (periodic maintenance)
	// Note: This is a general suggestion, not based on specific metrics
	if analysis.SlowQuery {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionSQLiteVacuum,
			Severity: SeverityInfo,
			Message:  "Slow query detected - consider periodic VACUUM for optimal performance",
			SQL:      "VACUUM;",
		})
	}

	// WAL mode suggestion for write-heavy workloads
	if analysis.QueryPlan.EstimatedRows > 10000 {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestionSQLiteWAL,
			Severity: SeverityInfo,
			Message:  "Large dataset - consider enabling WAL mode for better concurrency",
			SQL:      "PRAGMA journal_mode = WAL;",
		})
	}

	return suggestions
}

// GetAllHints returns all database-specific hints for the current database.
// This is the primary method to call from the optimizer.
func (h *DatabaseHints) GetAllHints(analysis *Analysis) []Suggestion {
	switch h.database {
	case "postgres":
		return h.SuggestPostgreSQLHints(analysis)
	case "mysql":
		return h.SuggestMySQLHints(analysis)
	case "sqlite":
		return h.SuggestSQLiteHints(analysis)
	default:
		return nil // Unknown database
	}
}
