// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"fmt"
	"testing"

	"github.com/coregx/relica/internal/core"
	"github.com/coregx/relica/internal/dialects"
)

// ============================================================================
// Advanced Query Benchmarks (Task 4.8)
// Benchmarks for subqueries, CTEs, EXISTS, IN, and set operations
// Note: These benchmarks measure SQL generation time, not actual DB execution
// ============================================================================

// BenchmarkExists_vs_In compares EXISTS vs IN performance for SQL generation
// Expected: Similar performance for SQL generation (~50-100ns)
func BenchmarkExists_vs_In(b *testing.B) {
	dialect := dialects.GetDialect("postgres")

	// Use RawExp for subquery (simpler than SelectQuery for benchmarking)
	subSQL := "SELECT user_id FROM orders WHERE status = ?"
	sub := core.NewExp(subSQL, "active")

	b.Run("EXISTS", func(b *testing.B) {
		exp := core.Exists(sub)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = exp.Build(dialect)
		}
	})

	b.Run("IN", func(b *testing.B) {
		// Use RawExp wrapped as Expression
		exp := core.In("id", sub)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = exp.Build(dialect)
		}
	})
}

// BenchmarkInSubquery_vs_InList compares IN (SELECT...) vs IN (1,2,3...)
// Expected: IN list should be faster (no subquery build overhead)
func BenchmarkInSubquery_vs_InList(b *testing.B) {
	dialect := dialects.GetDialect("postgres")

	// Subquery as RawExp
	sub := core.NewExp("SELECT id FROM users WHERE status = ?", "active")

	// Static list of 100 IDs
	ids := make([]interface{}, 100)
	for i := 0; i < 100; i++ {
		ids[i] = i + 1
	}

	b.Run("InSubquery", func(b *testing.B) {
		exp := core.In("id", sub)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = exp.Build(dialect)
		}
	})

	b.Run("InList", func(b *testing.B) {
		exp := core.In("id", ids...)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = exp.Build(dialect)
		}
	})
}

// BenchmarkNotExists_vs_NotIn compares NOT EXISTS vs NOT IN
// Expected: Similar performance for SQL generation
func BenchmarkNotExists_vs_NotIn(b *testing.B) {
	dialect := dialects.GetDialect("postgres")

	// Subquery as RawExp
	sub := core.NewExp("SELECT user_id FROM banned_users")

	b.Run("NOT_EXISTS", func(b *testing.B) {
		exp := core.NotExists(sub)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = exp.Build(dialect)
		}
	})

	b.Run("NOT_IN", func(b *testing.B) {
		exp := core.NotIn("id", sub)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = exp.Build(dialect)
		}
	})
}

// Benchmark complex queries using string concatenation to simulate SQL building
func BenchmarkComplexQueryBuild(b *testing.B) {
	counts := []int{1, 5, 10, 20}

	for _, count := range counts {
		b.Run(fmt.Sprintf("Queries_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Simulate building complex query with multiple parts
				query := "SELECT * FROM table WHERE "
				for j := 0; j < count; j++ {
					if j > 0 {
						query += " UNION "
					}
					query += fmt.Sprintf("id = %d", j)
				}
				_ = query
			}
		})
	}
}
