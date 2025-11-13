package analyzer

import (
	"testing"
)

// TestParseSQLiteExplain tests parsing of SQLite EXPLAIN QUERY PLAN output.
func TestParseSQLiteExplain(t *testing.T) {
	tests := []struct {
		name      string
		planLines []string
		want      *QueryPlan
	}{
		{
			name: "full_table_scan",
			planLines: []string{
				"SCAN users",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     false,
				FullScan:      true,
				Database:      "sqlite",
			},
		},
		{
			name: "index_scan",
			planLines: []string{
				"SEARCH users USING INDEX email_idx (email=?)",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "email_idx",
				FullScan:      false,
				Database:      "sqlite",
			},
		},
		{
			name: "primary_key_scan",
			planLines: []string{
				"SEARCH users USING INTEGER PRIMARY KEY (rowid=?)",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "PRIMARY KEY",
				FullScan:      false,
				Database:      "sqlite",
			},
		},
		{
			name: "covering_index",
			planLines: []string{
				"SEARCH users USING COVERING INDEX idx_email_status (email=?)",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "idx_email_status",
				FullScan:      false,
				Database:      "sqlite",
			},
		},
		{
			name: "automatic_index",
			planLines: []string{
				"SEARCH users USING AUTOMATIC COVERING INDEX (email=?)",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "AUTOMATIC INDEX",
				FullScan:      false,
				Database:      "sqlite",
			},
		},
		{
			name: "join_with_indexes",
			planLines: []string{
				"SEARCH users USING INDEX idx_user (id=?)",
				"SEARCH orders USING INDEX idx_order_user (user_id=?)",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "idx_user", // First index encountered
				FullScan:      false,
				Database:      "sqlite",
			},
		},
		{
			name: "join_mixed_scan_and_index",
			planLines: []string{
				"SCAN users",
				"SEARCH orders USING INDEX idx_order_user (user_id=?)",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "idx_order_user",
				FullScan:      true, // One table has full scan
				Database:      "sqlite",
			},
		},
		{
			name: "complex_query_with_subqueries",
			planLines: []string{
				"SEARCH users USING INDEX email_idx (email=?)",
				"EXECUTE LIST SUBQUERY 1",
				"SCAN orders",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "email_idx",
				FullScan:      true, // Subquery has full scan
				Database:      "sqlite",
			},
		},
		{
			name:      "empty_plan",
			planLines: []string{},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     false,
				FullScan:      false,
				Database:      "sqlite",
			},
		},
		{
			name: "whitespace_handling",
			planLines: []string{
				"  SEARCH users USING INDEX   email_idx   (email=?)  ",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "email_idx",
				FullScan:      false,
				Database:      "sqlite",
			},
		},
		{
			name: "lowercase_plan",
			planLines: []string{
				"search users using index email_idx (email=?)",
			},
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     true,
				IndexName:     "email_idx",
				FullScan:      false,
				Database:      "sqlite",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSQLiteExplain(tt.planLines)

			// Compare fields
			if got.Cost != tt.want.Cost {
				t.Errorf("Cost = %v, want %v", got.Cost, tt.want.Cost)
			}
			if got.EstimatedRows != tt.want.EstimatedRows {
				t.Errorf("EstimatedRows = %v, want %v", got.EstimatedRows, tt.want.EstimatedRows)
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
			if got.Database != tt.want.Database {
				t.Errorf("Database = %v, want %v", got.Database, tt.want.Database)
			}
		})
	}
}

// TestExtractIndexName tests index name extraction from SQLite EXPLAIN output.
func TestExtractIndexName(t *testing.T) {
	tests := []struct {
		name   string
		detail string
		want   string
	}{
		{
			name:   "simple_index",
			detail: "SEARCH users USING INDEX email_idx (email=?)",
			want:   "email_idx",
		},
		{
			name:   "covering_index",
			detail: "SEARCH users USING COVERING INDEX idx_email_status (email=?)",
			want:   "idx_email_status",
		},
		{
			name:   "index_with_underscores",
			detail: "SEARCH users USING INDEX idx_user_email_status (email=?)",
			want:   "idx_user_email_status",
		},
		{
			name:   "lowercase",
			detail: "search users using index email_idx (email=?)",
			want:   "email_idx",
		},
		{
			name:   "no_index",
			detail: "SCAN users",
			want:   "",
		},
		{
			name:   "primary_key",
			detail: "SEARCH users USING INTEGER PRIMARY KEY (rowid=?)",
			want:   "",
		},
		{
			name:   "whitespace",
			detail: "  SEARCH users USING INDEX   email_idx   (email=?)  ",
			want:   "email_idx",
		},
		{
			name:   "complex_condition",
			detail: "SEARCH users USING INDEX email_idx (email=? AND status=?)",
			want:   "email_idx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIndexName(tt.detail)
			if got != tt.want {
				t.Errorf("extractIndexName() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractFirstWord tests word extraction helper function.
func TestExtractFirstWord(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple_word",
			input: "email_idx (email=?)",
			want:  "email_idx",
		},
		{
			name:  "word_with_space",
			input: "email_idx some other text",
			want:  "email_idx",
		},
		{
			name:  "word_with_parenthesis",
			input: "email_idx(email=?)",
			want:  "email_idx",
		},
		{
			name:  "leading_whitespace",
			input: "  email_idx (email=?)",
			want:  "email_idx",
		},
		{
			name:  "single_word",
			input: "email_idx",
			want:  "email_idx",
		},
		{
			name:  "empty_string",
			input: "",
			want:  "",
		},
		{
			name:  "only_whitespace",
			input: "   ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFirstWord(tt.input)
			if got != tt.want {
				t.Errorf("extractFirstWord() = %v, want %v", got, tt.want)
			}
		})
	}
}
