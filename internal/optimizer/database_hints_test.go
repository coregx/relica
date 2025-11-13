package optimizer

import (
	"testing"
	"time"

	"github.com/coregx/relica/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDatabaseHints(t *testing.T) {
	tests := []struct {
		name     string
		database string
	}{
		{"PostgreSQL", "postgres"},
		{"MySQL", "mysql"},
		{"SQLite", "sqlite"},
		{"Unknown", "oracle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := NewDatabaseHints(tt.database)
			require.NotNil(t, hints)
			assert.Equal(t, tt.database, hints.database)
		})
	}
}

func TestPostgreSQLHints_FullScan(t *testing.T) {
	hints := NewDatabaseHints("postgres")

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			FullScan:      true,
			EstimatedRows: 1000,
		},
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
	}

	suggestions := hints.SuggestPostgreSQLHints(analysis)

	// Should suggest ANALYZE
	assert.Greater(t, len(suggestions), 0, "Should suggest at least one optimization")

	// Find ANALYZE suggestion
	var analyzeFound bool
	for _, s := range suggestions {
		if s.Type == SuggestionPostgresAnalyze {
			analyzeFound = true
			assert.Equal(t, SeverityInfo, s.Severity)
			assert.Contains(t, s.SQL, "ANALYZE")
			assert.Contains(t, s.Message, "ANALYZE")
		}
	}
	assert.True(t, analyzeFound, "Should suggest ANALYZE for full scan")
}

func TestPostgreSQLHints_ParallelQuery(t *testing.T) {
	hints := NewDatabaseHints("postgres")

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			FullScan:      false,
			EstimatedRows: 150000, // > 100k threshold
		},
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
	}

	suggestions := hints.SuggestPostgreSQLHints(analysis)

	// Should suggest parallel query
	var parallelFound bool
	for _, s := range suggestions {
		if s.Type == SuggestionPostgresParallel {
			parallelFound = true
			assert.Equal(t, SeverityInfo, s.Severity)
			assert.Contains(t, s.SQL, "max_parallel_workers_per_gather")
			assert.Contains(t, s.Message, "parallel")
		}
	}
	assert.True(t, parallelFound, "Should suggest parallel query for large scans")
}

func TestPostgreSQLHints_CacheHitRatio(t *testing.T) {
	hints := NewDatabaseHints("postgres")

	tests := []struct {
		name        string
		buffersHit  int64
		buffersMiss int64
		expectWarn  bool
	}{
		{
			name:        "Good cache ratio (95%)",
			buffersHit:  9500,
			buffersMiss: 500,
			expectWarn:  false,
		},
		{
			name:        "Poor cache ratio (80%)",
			buffersHit:  8000,
			buffersMiss: 2000,
			expectWarn:  true,
		},
		{
			name:        "No buffer data",
			buffersHit:  0,
			buffersMiss: 0,
			expectWarn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					BuffersHit:  tt.buffersHit,
					BuffersMiss: tt.buffersMiss,
				},
			}

			suggestions := hints.SuggestPostgreSQLHints(analysis)

			cacheHintFound := false
			for _, s := range suggestions {
				if s.Type == SuggestionPostgresCacheHit {
					cacheHintFound = true
					assert.Equal(t, SeverityWarning, s.Severity)
					assert.Contains(t, s.Message, "cache hit ratio")
				}
			}

			if tt.expectWarn {
				assert.True(t, cacheHintFound, "Should warn about low cache hit ratio")
			} else {
				assert.False(t, cacheHintFound, "Should not warn about good cache hit ratio")
			}
		})
	}
}

func TestMySQLHints_IndexHint(t *testing.T) {
	hints := NewDatabaseHints("mysql")

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			FullScan: true,
		},
		MissingIndexes: []IndexRecommendation{
			{
				Table:   "users",
				Columns: []string{"email"},
				Type:    "btree",
				Reason:  "Single column index for WHERE filtering",
			},
		},
	}

	suggestions := hints.SuggestMySQLHints(analysis)

	// Should suggest USE INDEX
	var indexHintFound bool
	for _, s := range suggestions {
		if s.Type == SuggestionMySQLIndexHint {
			indexHintFound = true
			assert.Equal(t, SeverityInfo, s.Severity)
			assert.Contains(t, s.Message, "USE INDEX")
			assert.Contains(t, s.Message, "idx_users_email")
		}
	}
	assert.True(t, indexHintFound, "Should suggest USE INDEX hint")
}

func TestMySQLHints_OptimizeTable(t *testing.T) {
	hints := NewDatabaseHints("mysql")

	tests := []struct {
		name         string
		rowsExamined int64
		rowsProduced int64
		expectOpt    bool
	}{
		{
			name:         "High examination ratio (20x)",
			rowsExamined: 200000,
			rowsProduced: 10000,
			expectOpt:    true,
		},
		{
			name:         "Normal examination ratio (2x)",
			rowsExamined: 20000,
			rowsProduced: 10000,
			expectOpt:    false,
		},
		{
			name:         "Small dataset",
			rowsExamined: 2000,
			rowsProduced: 1000,
			expectOpt:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					RowsExamined: tt.rowsExamined,
					RowsProduced: tt.rowsProduced,
				},
			}

			suggestions := hints.SuggestMySQLHints(analysis)

			optimizeFound := false
			for _, s := range suggestions {
				if s.Type == SuggestionMySQLOptimize {
					optimizeFound = true
					assert.Equal(t, SeverityInfo, s.Severity)
					assert.Contains(t, s.SQL, "OPTIMIZE TABLE")
				}
			}

			if tt.expectOpt {
				assert.True(t, optimizeFound, "Should suggest OPTIMIZE TABLE")
			} else {
				assert.False(t, optimizeFound, "Should not suggest OPTIMIZE TABLE")
			}
		})
	}
}

func TestMySQLHints_BufferPool(t *testing.T) {
	hints := NewDatabaseHints("mysql")

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			EstimatedRows: 600000, // > 500k threshold
		},
	}

	suggestions := hints.SuggestMySQLHints(analysis)

	// Should suggest buffer pool tuning
	var bufferFound bool
	for _, s := range suggestions {
		if s.Type == SuggestionMySQLBufferPool {
			bufferFound = true
			assert.Equal(t, SeverityInfo, s.Severity)
			assert.Contains(t, s.Message, "buffer pool")
		}
	}
	assert.True(t, bufferFound, "Should suggest buffer pool tuning for large scans")
}

func TestSQLiteHints_Analyze(t *testing.T) {
	hints := NewDatabaseHints("sqlite")

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			FullScan: true,
		},
		SlowQuery:     false,
		ExecutionTime: 30 * time.Millisecond,
	}

	suggestions := hints.SuggestSQLiteHints(analysis)

	// Should suggest ANALYZE
	var analyzeFound bool
	for _, s := range suggestions {
		if s.Type == SuggestionSQLiteAnalyze {
			analyzeFound = true
			assert.Equal(t, SeverityInfo, s.Severity)
			assert.Contains(t, s.SQL, "ANALYZE")
		}
	}
	assert.True(t, analyzeFound, "Should suggest ANALYZE for full scan")
}

func TestSQLiteHints_Vacuum(t *testing.T) {
	hints := NewDatabaseHints("sqlite")

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			EstimatedRows: 5000,
		},
		SlowQuery:     true, // Triggers VACUUM suggestion
		ExecutionTime: 200 * time.Millisecond,
	}

	suggestions := hints.SuggestSQLiteHints(analysis)

	// Should suggest VACUUM
	var vacuumFound bool
	for _, s := range suggestions {
		if s.Type == SuggestionSQLiteVacuum {
			vacuumFound = true
			assert.Equal(t, SeverityInfo, s.Severity)
			assert.Contains(t, s.SQL, "VACUUM")
		}
	}
	assert.True(t, vacuumFound, "Should suggest VACUUM for slow queries")
}

func TestSQLiteHints_WAL(t *testing.T) {
	hints := NewDatabaseHints("sqlite")

	analysis := &Analysis{
		QueryPlan: &analyzer.QueryPlan{
			EstimatedRows: 15000, // > 10k threshold
		},
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
	}

	suggestions := hints.SuggestSQLiteHints(analysis)

	// Should suggest WAL mode
	var walFound bool
	for _, s := range suggestions {
		if s.Type == SuggestionSQLiteWAL {
			walFound = true
			assert.Equal(t, SeverityInfo, s.Severity)
			assert.Contains(t, s.SQL, "WAL")
		}
	}
	assert.True(t, walFound, "Should suggest WAL mode for large datasets")
}

func TestGetAllHints(t *testing.T) {
	tests := []struct {
		name           string
		database       string
		analysis       *Analysis
		expectTypes    []SuggestionType
		expectMinCount int
	}{
		{
			name:     "PostgreSQL with full scan",
			database: "postgres",
			analysis: &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					FullScan:      true,
					EstimatedRows: 1000,
				},
			},
			expectTypes:    []SuggestionType{SuggestionPostgresAnalyze},
			expectMinCount: 1,
		},
		{
			name:     "MySQL with large scan",
			database: "mysql",
			analysis: &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					EstimatedRows: 600000,
				},
			},
			expectTypes:    []SuggestionType{SuggestionMySQLBufferPool},
			expectMinCount: 1,
		},
		{
			name:     "SQLite with slow query",
			database: "sqlite",
			analysis: &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					EstimatedRows: 15000,
				},
				SlowQuery: true,
			},
			expectTypes:    []SuggestionType{SuggestionSQLiteVacuum, SuggestionSQLiteWAL},
			expectMinCount: 2,
		},
		{
			name:     "Unknown database",
			database: "oracle",
			analysis: &Analysis{
				QueryPlan: &analyzer.QueryPlan{
					FullScan: true,
				},
			},
			expectTypes:    nil,
			expectMinCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := NewDatabaseHints(tt.database)
			suggestions := hints.GetAllHints(tt.analysis)

			assert.GreaterOrEqual(t, len(suggestions), tt.expectMinCount,
				"Should return at least %d suggestions", tt.expectMinCount)

			// Verify expected types are present
			for _, expectedType := range tt.expectTypes {
				found := false
				for _, s := range suggestions {
					if s.Type == expectedType {
						found = true
						break
					}
				}
				assert.True(t, found, "Should contain suggestion type: %s", expectedType)
			}
		})
	}
}
