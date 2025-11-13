package analyzer

import (
	"testing"
	"time"
)

// TestParsePostgresExplain tests parsing of PostgreSQL EXPLAIN JSON output.
func TestParsePostgresExplain(t *testing.T) {
	tests := []struct {
		name        string
		jsonInput   string
		withAnalyze bool
		want        *QueryPlan
		wantErr     bool
	}{
		{
			name: "simple_seq_scan",
			jsonInput: `[{
				"Plan": {
					"Node Type": "Seq Scan",
					"Relation Name": "users",
					"Total Cost": 12.50,
					"Startup Cost": 0.00,
					"Plan Rows": 100,
					"Plan Width": 32
				},
				"Planning Time": 0.123,
				"Execution Time": 0
			}]`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          12.50,
				EstimatedRows: 100,
				UsesIndex:     false,
				FullScan:      true,
				Database:      "postgres",
			},
			wantErr: false,
		},
		{
			name: "index_scan",
			jsonInput: `[{
				"Plan": {
					"Node Type": "Index Scan",
					"Relation Name": "users",
					"Index Name": "users_email_idx",
					"Total Cost": 8.27,
					"Startup Cost": 0.29,
					"Plan Rows": 1,
					"Plan Width": 32
				},
				"Planning Time": 0.156
			}]`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          8.27,
				EstimatedRows: 1,
				UsesIndex:     true,
				IndexName:     "users_email_idx",
				FullScan:      false,
				Database:      "postgres",
			},
			wantErr: false,
		},
		{
			name: "index_only_scan",
			jsonInput: `[{
				"Plan": {
					"Node Type": "Index Only Scan",
					"Relation Name": "users",
					"Index Name": "users_status_idx",
					"Total Cost": 5.15,
					"Startup Cost": 0.15,
					"Plan Rows": 50,
					"Plan Width": 4
				},
				"Planning Time": 0.089
			}]`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          5.15,
				EstimatedRows: 50,
				UsesIndex:     true,
				IndexName:     "users_status_idx",
				FullScan:      false,
				Database:      "postgres",
			},
			wantErr: false,
		},
		{
			name: "bitmap_index_scan",
			jsonInput: `[{
				"Plan": {
					"Node Type": "Bitmap Heap Scan",
					"Relation Name": "users",
					"Total Cost": 15.42,
					"Startup Cost": 4.33,
					"Plan Rows": 25,
					"Plan Width": 32,
					"Plans": [{
						"Node Type": "Bitmap Index Scan",
						"Index Name": "users_age_idx",
						"Total Cost": 0.00,
						"Startup Cost": 0.00,
						"Plan Rows": 25,
						"Plan Width": 0
					}]
				},
				"Planning Time": 0.145
			}]`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          15.42,
				EstimatedRows: 25,
				UsesIndex:     true,
				IndexName:     "users_age_idx",
				FullScan:      false,
				Database:      "postgres",
			},
			wantErr: false,
		},
		{
			name: "explain_analyze_with_buffers",
			jsonInput: `[{
				"Plan": {
					"Node Type": "Index Scan",
					"Relation Name": "users",
					"Index Name": "users_email_idx",
					"Total Cost": 8.27,
					"Startup Cost": 0.29,
					"Plan Rows": 1,
					"Plan Width": 32,
					"Actual Rows": 1,
					"Actual Loops": 1,
					"Actual Total Time": 0.045,
					"Shared Hit Blocks": 3,
					"Shared Read Blocks": 1
				},
				"Planning Time": 0.156,
				"Execution Time": 0.067
			}]`,
			withAnalyze: true,
			want: &QueryPlan{
				Cost:          8.27,
				EstimatedRows: 1,
				ActualRows:    1,
				ActualTime:    67 * time.Microsecond,
				UsesIndex:     true,
				IndexName:     "users_email_idx",
				FullScan:      false,
				BuffersHit:    3,
				BuffersMiss:   1,
				Database:      "postgres",
			},
			wantErr: false,
		},
		{
			name: "nested_loop_join",
			jsonInput: `[{
				"Plan": {
					"Node Type": "Nested Loop",
					"Total Cost": 25.67,
					"Startup Cost": 0.29,
					"Plan Rows": 10,
					"Plan Width": 64,
					"Plans": [
						{
							"Node Type": "Index Scan",
							"Relation Name": "users",
							"Index Name": "users_pkey",
							"Total Cost": 8.27,
							"Startup Cost": 0.29,
							"Plan Rows": 1,
							"Plan Width": 32
						},
						{
							"Node Type": "Seq Scan",
							"Relation Name": "orders",
							"Total Cost": 15.00,
							"Startup Cost": 0.00,
							"Plan Rows": 10,
							"Plan Width": 32
						}
					]
				},
				"Planning Time": 0.234
			}]`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          25.67,
				EstimatedRows: 10,
				UsesIndex:     true,
				IndexName:     "users_pkey",
				FullScan:      true,
				Database:      "postgres",
			},
			wantErr: false,
		},
		{
			name: "multiple_loops_explain_analyze",
			jsonInput: `[{
				"Plan": {
					"Node Type": "Aggregate",
					"Total Cost": 100.50,
					"Startup Cost": 100.00,
					"Plan Rows": 1,
					"Plan Width": 8,
					"Actual Rows": 1,
					"Actual Loops": 1,
					"Actual Total Time": 5.234,
					"Plans": [{
						"Node Type": "Seq Scan",
						"Relation Name": "large_table",
						"Total Cost": 100.00,
						"Startup Cost": 0.00,
						"Plan Rows": 1000,
						"Plan Width": 4,
						"Actual Rows": 500,
						"Actual Loops": 2,
						"Actual Total Time": 5.000,
						"Shared Hit Blocks": 100,
						"Shared Read Blocks": 50
					}]
				},
				"Planning Time": 0.456,
				"Execution Time": 5.678
			}]`,
			withAnalyze: true,
			want: &QueryPlan{
				Cost:          100.50,
				EstimatedRows: 1,
				ActualRows:    1001, // Aggregate (1*1) + Seq Scan (500*2) = 1001
				ActualTime:    5678 * time.Microsecond,
				UsesIndex:     false,
				FullScan:      true,
				BuffersHit:    100,
				BuffersMiss:   50,
				Database:      "postgres",
			},
			wantErr: false,
		},
		{
			name:        "empty_json_array",
			jsonInput:   `[]`,
			withAnalyze: false,
			want:        nil,
			wantErr:     true,
		},
		{
			name:        "invalid_json",
			jsonInput:   `{invalid json}`,
			withAnalyze: false,
			want:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePostgresExplain(tt.jsonInput, tt.withAnalyze)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePostgresExplain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Compare QueryPlan fields
			if got.Cost != tt.want.Cost {
				t.Errorf("Cost = %v, want %v", got.Cost, tt.want.Cost)
			}
			if got.EstimatedRows != tt.want.EstimatedRows {
				t.Errorf("EstimatedRows = %v, want %v", got.EstimatedRows, tt.want.EstimatedRows)
			}
			if got.ActualRows != tt.want.ActualRows {
				t.Errorf("ActualRows = %v, want %v", got.ActualRows, tt.want.ActualRows)
			}
			if got.ActualTime != tt.want.ActualTime {
				t.Errorf("ActualTime = %v, want %v", got.ActualTime, tt.want.ActualTime)
			}
			if got.UsesIndex != tt.want.UsesIndex {
				t.Errorf("UsesIndex = %v, want %v", got.UsesIndex, tt.want.UsesIndex)
			}
			if got.IndexName != tt.want.IndexName {
				t.Errorf("IndexName = %v, want %v", got.IndexName, tt.want.IndexName)
			}
			if got.FullScan != tt.want.FullScan {
				t.Errorf("FullScan = %v, want %v", got.FullScan, tt.want.FullScan)
			}
			if got.BuffersHit != tt.want.BuffersHit {
				t.Errorf("BuffersHit = %v, want %v", got.BuffersHit, tt.want.BuffersHit)
			}
			if got.BuffersMiss != tt.want.BuffersMiss {
				t.Errorf("BuffersMiss = %v, want %v", got.BuffersMiss, tt.want.BuffersMiss)
			}
			if got.Database != tt.want.Database {
				t.Errorf("Database = %v, want %v", got.Database, tt.want.Database)
			}
		})
	}
}

// TestExtractPostgresMetrics tests metric extraction from PostgreSQL plan tree.
func TestExtractPostgresMetrics(t *testing.T) {
	tests := []struct {
		name        string
		node        *postgresExplainPlan
		withAnalyze bool
		wantPlan    *QueryPlan
	}{
		{
			name: "seq_scan_sets_full_scan",
			node: &postgresExplainPlan{
				NodeType:  "Seq Scan",
				TotalCost: 10.0,
				PlanRows:  50,
			},
			withAnalyze: false,
			wantPlan: &QueryPlan{
				UsesIndex: false,
				FullScan:  true,
			},
		},
		{
			name: "index_scan_sets_index_usage",
			node: &postgresExplainPlan{
				NodeType:  "Index Scan",
				IndexName: "test_idx",
				TotalCost: 5.0,
				PlanRows:  1,
			},
			withAnalyze: false,
			wantPlan: &QueryPlan{
				UsesIndex: true,
				IndexName: "test_idx",
				FullScan:  false,
			},
		},
		{
			name: "actual_rows_accumulate_with_loops",
			node: &postgresExplainPlan{
				NodeType:    "Seq Scan",
				ActualRows:  100,
				ActualLoops: 3,
				TotalCost:   20.0,
				PlanRows:    300,
			},
			withAnalyze: true,
			wantPlan: &QueryPlan{
				ActualRows: 300,
				FullScan:   true,
			},
		},
		{
			name: "buffer_statistics_accumulate",
			node: &postgresExplainPlan{
				NodeType:         "Index Scan",
				IndexName:        "test_idx",
				SharedHitBlocks:  50,
				SharedReadBlocks: 10,
				TotalCost:        8.0,
				PlanRows:         5,
			},
			withAnalyze: true,
			wantPlan: &QueryPlan{
				UsesIndex:   true,
				IndexName:   "test_idx",
				BuffersHit:  50,
				BuffersMiss: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &QueryPlan{}
			extractPostgresMetrics(tt.node, plan, tt.withAnalyze)

			if plan.UsesIndex != tt.wantPlan.UsesIndex {
				t.Errorf("UsesIndex = %v, want %v", plan.UsesIndex, tt.wantPlan.UsesIndex)
			}
			if plan.FullScan != tt.wantPlan.FullScan {
				t.Errorf("FullScan = %v, want %v", plan.FullScan, tt.wantPlan.FullScan)
			}
			if plan.IndexName != tt.wantPlan.IndexName {
				t.Errorf("IndexName = %v, want %v", plan.IndexName, tt.wantPlan.IndexName)
			}
			if tt.withAnalyze {
				if plan.ActualRows != tt.wantPlan.ActualRows {
					t.Errorf("ActualRows = %v, want %v", plan.ActualRows, tt.wantPlan.ActualRows)
				}
				if plan.BuffersHit != tt.wantPlan.BuffersHit {
					t.Errorf("BuffersHit = %v, want %v", plan.BuffersHit, tt.wantPlan.BuffersHit)
				}
				if plan.BuffersMiss != tt.wantPlan.BuffersMiss {
					t.Errorf("BuffersMiss = %v, want %v", plan.BuffersMiss, tt.wantPlan.BuffersMiss)
				}
			}
		})
	}
}

// TestExtractPostgresMetricsRecursive tests recursive metric extraction.
func TestExtractPostgresMetricsRecursive(t *testing.T) {
	// Create a nested plan tree (join with two child scans)
	rootNode := &postgresExplainPlan{
		NodeType:  "Nested Loop",
		TotalCost: 50.0,
		PlanRows:  10,
		Plans: []postgresExplainPlan{
			{
				NodeType:         "Index Scan",
				IndexName:        "users_pkey",
				TotalCost:        8.0,
				PlanRows:         1,
				ActualRows:       1,
				ActualLoops:      1,
				SharedHitBlocks:  5,
				SharedReadBlocks: 2,
			},
			{
				NodeType:         "Seq Scan",
				TotalCost:        40.0,
				PlanRows:         100,
				ActualRows:       50,
				ActualLoops:      2,
				SharedHitBlocks:  20,
				SharedReadBlocks: 10,
			},
		},
	}

	plan := &QueryPlan{}
	extractPostgresMetrics(rootNode, plan, true)

	// Should detect index usage from child
	if !plan.UsesIndex {
		t.Error("Expected UsesIndex = true")
	}
	if plan.IndexName != "users_pkey" {
		t.Errorf("IndexName = %v, want users_pkey", plan.IndexName)
	}

	// Should detect full scan from second child
	if !plan.FullScan {
		t.Error("Expected FullScan = true")
	}

	// Should accumulate actual rows: 1*1 + 50*2 = 101
	expectedRows := int64(101)
	if plan.ActualRows != expectedRows {
		t.Errorf("ActualRows = %v, want %v", plan.ActualRows, expectedRows)
	}

	// Should accumulate buffers: 5+20 = 25 hits, 2+10 = 12 misses
	if plan.BuffersHit != 25 {
		t.Errorf("BuffersHit = %v, want 25", plan.BuffersHit)
	}
	if plan.BuffersMiss != 12 {
		t.Errorf("BuffersMiss = %v, want 12", plan.BuffersMiss)
	}
}

// TestNewPostgresAnalyzer tests analyzer creation.
func TestNewPostgresAnalyzer(t *testing.T) {
	analyzer := NewPostgresAnalyzer(nil)
	if analyzer == nil {
		t.Fatal("NewPostgresAnalyzer() returned nil")
	}
	if analyzer.db != nil {
		t.Error("Expected db to be nil for test analyzer")
	}
}

// TestPostgresAnalyzerInterface verifies PostgresAnalyzer implements Analyzer interface.
func TestPostgresAnalyzerInterface(t *testing.T) {
	var _ Analyzer = (*PostgresAnalyzer)(nil)
}

// BenchmarkParsePostgresExplain benchmarks EXPLAIN JSON parsing.
func BenchmarkParsePostgresExplain(b *testing.B) {
	jsonInput := `[{
		"Plan": {
			"Node Type": "Index Scan",
			"Relation Name": "users",
			"Index Name": "users_email_idx",
			"Total Cost": 8.27,
			"Startup Cost": 0.29,
			"Plan Rows": 1,
			"Plan Width": 32,
			"Actual Rows": 1,
			"Actual Loops": 1,
			"Actual Total Time": 0.045,
			"Shared Hit Blocks": 3,
			"Shared Read Blocks": 1
		},
		"Planning Time": 0.156,
		"Execution Time": 0.067
	}]`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parsePostgresExplain(jsonInput, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}
