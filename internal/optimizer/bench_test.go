package optimizer

import (
	"context"
	"testing"
	"time"

	"github.com/coregx/relica/internal/analyzer"
)

// BenchmarkParseWhereClause benchmarks the WHERE clause parser.
func BenchmarkParseWhereClause(b *testing.B) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "simple",
			query: "WHERE status = ?",
		},
		{
			name:  "composite",
			query: "WHERE status = ? AND country = ?",
		},
		{
			name:  "complex",
			query: "WHERE status = ? AND country = ? AND email LIKE ? AND age > ?",
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = ParseWhereClause(tt.query)
			}
		})
	}
}

// BenchmarkDetectMissingIndexes benchmarks index detection.
func BenchmarkDetectMissingIndexes(b *testing.B) {
	opt := NewBasicOptimizer(&mockAnalyzer{plan: &analyzer.QueryPlan{Database: "postgres"}}, 100*time.Millisecond)

	tests := []struct {
		name  string
		query string
		args  []interface{}
	}{
		{
			name:  "simple_where",
			query: "SELECT * FROM users WHERE status = ?",
			args:  []interface{}{1},
		},
		{
			name:  "composite_where",
			query: "SELECT * FROM users WHERE status = ? AND country = ?",
			args:  []interface{}{1, "US"},
		},
		{
			name:  "join_query",
			query: "SELECT * FROM users JOIN orders ON users.id = orders.user_id WHERE status = ?",
			args:  []interface{}{1},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			plan := &analyzer.QueryPlan{
				FullScan:      true,
				EstimatedRows: 1000,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = opt.detectMissingIndexes(tt.query, plan)
			}
		})
	}
}

// BenchmarkAnalyzeCoveringIndex benchmarks covering index analysis.
func BenchmarkAnalyzeCoveringIndex(b *testing.B) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "simple",
			query: "SELECT email FROM users WHERE status = ?",
		},
		{
			name:  "composite",
			query: "SELECT email, name FROM users WHERE status = ? AND country = ?",
		},
		{
			name:  "complex",
			query: "SELECT email, name, created_at FROM users WHERE status = ? AND country = ? AND age > ?",
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = AnalyzeCoveringIndex(tt.query)
			}
		})
	}
}

// BenchmarkDatabaseHints benchmarks database-specific hints.
func BenchmarkDatabaseHints(b *testing.B) {
	tests := []struct {
		name     string
		database string
		analysis *Analysis
	}{
		{
			name:     "postgres_full_scan",
			database: "postgres",
			analysis: &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					FullScan:      true,
					EstimatedRows: 150000,
					BuffersHit:    8000,
					BuffersMiss:   2000,
				},
				SlowQuery:     true,
				ExecutionTime: 200 * time.Millisecond,
			},
		},
		{
			name:     "mysql_large_scan",
			database: "mysql",
			analysis: &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					EstimatedRows: 600000,
					RowsExamined:  200000,
					RowsProduced:  10000,
				},
				MissingIndexes: []IndexRecommendation{
					{Table: "users", Columns: []string{"email"}, Type: "btree"},
				},
			},
		},
		{
			name:     "sqlite_slow_query",
			database: "sqlite",
			analysis: &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					FullScan:      true,
					EstimatedRows: 15000,
				},
				SlowQuery:     true,
				ExecutionTime: 150 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			hints := NewDatabaseHints(tt.database)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = hints.GetAllHints(tt.analysis)
			}
		})
	}
}

// BenchmarkOptimizerSuggest benchmarks the complete suggestion pipeline.
func BenchmarkOptimizerSuggest(b *testing.B) {
	opt := NewBasicOptimizer(&mockAnalyzer{plan: &analyzer.QueryPlan{Database: "postgres"}}, 100*time.Millisecond)

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			FullScan:      true,
			EstimatedRows: 150000,
			BuffersHit:    8000,
			BuffersMiss:   2000,
			Database:      "postgres",
		},
		MissingIndexes: []IndexRecommendation{
			{
				Table:   "users",
				Columns: []string{"status", "country"},
				Type:    "btree",
				Reason:  "Composite index for multiple AND conditions",
			},
			{
				Table:   "users",
				Columns: []string{"status", "country", "email"},
				Type:    "btree",
				Reason:  "Covering index: enables index-only scan",
			},
		},
		SlowQuery:     true,
		ExecutionTime: 200 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = opt.Suggest(analysis)
	}
}

// BenchmarkOptimizer_Analyze benchmarks the full analysis pipeline.
func BenchmarkOptimizer_Analyze(b *testing.B) {
	opt := NewBasicOptimizer(&mockAnalyzer{plan: &analyzer.QueryPlan{Database: "postgres", FullScan: true, EstimatedRows: 1000}}, 100*time.Millisecond)
	ctx := context.Background()
	query := "SELECT email, name FROM users WHERE status = ? AND country = ?"
	args := []interface{}{1, "US"}
	execTime := 150 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = opt.Analyze(ctx, query, args, execTime)
	}
}
