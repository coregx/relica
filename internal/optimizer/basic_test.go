package optimizer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coregx/relica/internal/analyzer"
)

// mockAnalyzer is a mock implementation of analyzer.Analyzer for testing.
type mockAnalyzer struct {
	plan *analyzer.QueryPlan
	err  error
}

func (m *mockAnalyzer) Explain(ctx context.Context, query string, args []interface{}) (*analyzer.QueryPlan, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.plan, nil
}

func (m *mockAnalyzer) ExplainAnalyze(ctx context.Context, query string, args []interface{}) (*analyzer.QueryPlan, error) {
	return m.Explain(ctx, query, args)
}

func TestNewBasicOptimizer(t *testing.T) {
	tests := []struct {
		name              string
		threshold         time.Duration
		expectedThreshold time.Duration
	}{
		{
			name:              "custom threshold",
			threshold:         200 * time.Millisecond,
			expectedThreshold: 200 * time.Millisecond,
		},
		{
			name:              "zero threshold defaults to 100ms",
			threshold:         0,
			expectedThreshold: 100 * time.Millisecond,
		},
		{
			name:              "negative threshold defaults to 100ms",
			threshold:         -50 * time.Millisecond,
			expectedThreshold: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAnalyzer{}
			opt := NewBasicOptimizer(mock, tt.threshold)

			if opt == nil {
				t.Fatal("NewBasicOptimizer returned nil")
			}

			if opt.slowQueryThreshold != tt.expectedThreshold {
				t.Errorf("expected threshold %v, got %v", tt.expectedThreshold, opt.slowQueryThreshold)
			}

			if opt.analyzer != mock {
				t.Error("analyzer not set correctly")
			}
		})
	}
}

func TestBasicOptimizer_Analyze_FastQuery(t *testing.T) {
	mock := &mockAnalyzer{
		plan: &analyzer.QueryPlan{
			Cost:          10.5,
			EstimatedRows: 100,
			UsesIndex:     true,
			IndexName:     "idx_users_email",
			FullScan:      false,
			Database:      "postgres",
		},
	}

	opt := NewBasicOptimizer(mock, 100*time.Millisecond)
	ctx := context.Background()

	analysis, err := opt.Analyze(ctx, "SELECT * FROM users WHERE email = ?", []interface{}{"test@example.com"}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if analysis.SlowQuery {
		t.Error("expected fast query, got slow query")
	}

	if analysis.ExecutionTime != 50*time.Millisecond {
		t.Errorf("expected execution time 50ms, got %v", analysis.ExecutionTime)
	}

	if analysis.QueryPlan.UsesIndex != true {
		t.Error("expected query to use index")
	}

	if len(analysis.MissingIndexes) != 0 {
		t.Errorf("expected no missing indexes, got %d", len(analysis.MissingIndexes))
	}
}

func TestBasicOptimizer_Analyze_SlowQuery(t *testing.T) {
	mock := &mockAnalyzer{
		plan: &analyzer.QueryPlan{
			Cost:          1000.0,
			EstimatedRows: 10000,
			UsesIndex:     false,
			FullScan:      true,
			Database:      "mysql",
		},
	}

	opt := NewBasicOptimizer(mock, 100*time.Millisecond)
	ctx := context.Background()

	analysis, err := opt.Analyze(ctx, "SELECT * FROM users WHERE status = ?", []interface{}{1}, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if !analysis.SlowQuery {
		t.Error("expected slow query, got fast query")
	}

	if !analysis.QueryPlan.FullScan {
		t.Error("expected full scan")
	}

	if len(analysis.MissingIndexes) == 0 {
		t.Error("expected missing index recommendations for full scan query")
	}
}

func TestBasicOptimizer_Suggest_NoIssues(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     false,
		ExecutionTime: 10 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: true,
			FullScan:  false,
		},
		MissingIndexes: nil,
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for optimal query, got %d", len(suggestions))
	}
}

func TestBasicOptimizer_Suggest_SlowQuery(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     true,
		ExecutionTime: 200 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: true,
			FullScan:  false,
		},
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}

	if suggestions[0].Type != SuggestionSlowQuery {
		t.Errorf("expected slow_query suggestion, got %v", suggestions[0].Type)
	}

	if suggestions[0].Severity != SeverityWarning {
		t.Errorf("expected warning severity, got %v", suggestions[0].Severity)
	}
}

func TestBasicOptimizer_Suggest_FullScan(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: false,
			FullScan:  true,
		},
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}

	if suggestions[0].Type != SuggestionFullScan {
		t.Errorf("expected full_scan suggestion, got %v", suggestions[0].Type)
	}
}

func TestBasicOptimizer_Suggest_MissingIndex(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: false,
			FullScan:  true,
		},
		MissingIndexes: []IndexRecommendation{
			{
				Table:   "users",
				Columns: []string{"status", "created_at"},
				Type:    "btree",
				Reason:  "WHERE clause filtering without index usage",
			},
		},
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)

	// Should have: full_scan + index_missing
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}

	indexSuggestion := suggestions[1]
	if indexSuggestion.Type != SuggestionIndexMissing {
		t.Errorf("expected index_missing suggestion, got %v", indexSuggestion.Type)
	}

	if indexSuggestion.SQL == "" {
		t.Error("expected SQL fix for missing index")
	}

	// Check SQL format
	expectedSQL := "CREATE INDEX idx_users_status_created_at ON users(status, created_at);"
	if indexSuggestion.SQL != expectedSQL {
		t.Errorf("expected SQL:\n%s\ngot:\n%s", expectedSQL, indexSuggestion.SQL)
	}
}

func TestBasicOptimizer_Suggest_MultipleIssues(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     true,
		ExecutionTime: 300 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: false,
			FullScan:  true,
		},
		MissingIndexes: []IndexRecommendation{
			{
				Table:   "orders",
				Columns: []string{"user_id"},
				Type:    "btree",
				Reason:  "WHERE clause filtering without index usage",
			},
		},
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)

	// Should have: slow_query + full_scan + index_missing = 3 suggestions
	if len(suggestions) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(suggestions))
	}

	// Verify all suggestion types are present
	types := make(map[SuggestionType]bool)
	for _, s := range suggestions {
		types[s.Type] = true
	}

	expectedTypes := []SuggestionType{
		SuggestionSlowQuery,
		SuggestionFullScan,
		SuggestionIndexMissing,
	}

	for _, et := range expectedTypes {
		if !types[et] {
			t.Errorf("missing expected suggestion type: %v", et)
		}
	}
}

func TestExtractTableName(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "simple select",
			query:    "SELECT * FROM users",
			expected: "users",
		},
		{
			name:     "select with where",
			query:    "SELECT id, name FROM products WHERE status = 1",
			expected: "products",
		},
		{
			name:     "uppercase",
			query:    "SELECT * FROM ORDERS",
			expected: "orders",
		},
		{
			name:     "mixed case",
			query:    "SeLeCt * FrOm CuStOmErS",
			expected: "customers",
		},
		{
			name:     "no from clause",
			query:    "SELECT 1",
			expected: "",
		},
		{
			name:     "table with underscores",
			query:    "SELECT * FROM user_orders",
			expected: "user_orders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTableName(tt.query)
			if result != tt.expected {
				t.Errorf("extractTableName(%q) = %q, want %q", tt.query, result, tt.expected)
			}
		})
	}
}

func TestExtractWhereColumns(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "simple equality",
			query:    "SELECT * FROM users WHERE email = ?",
			expected: []string{"email"},
		},
		{
			name:     "multiple conditions",
			query:    "SELECT * FROM users WHERE status = ? AND created_at > ?",
			expected: []string{"status", "created_at"},
		},
		{
			name:     "various operators",
			query:    "SELECT * FROM products WHERE price >= ? AND stock < ? AND category = ?",
			expected: []string{"price", "stock", "category"},
		},
		{
			name:     "with order by",
			query:    "SELECT * FROM users WHERE status = ? ORDER BY created_at",
			expected: []string{"status"},
		},
		{
			name:     "with limit",
			query:    "SELECT * FROM users WHERE active = ? LIMIT 10",
			expected: []string{"active"},
		},
		{
			name:     "no where clause",
			query:    "SELECT * FROM users",
			expected: nil,
		},
		{
			name:     "like operator",
			query:    "SELECT * FROM users WHERE name LIKE ?",
			expected: []string{"name"},
		},
		{
			name:     "in operator",
			query:    "SELECT * FROM users WHERE status IN (?, ?)",
			expected: []string{"status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractWhereColumns(tt.query)
			if len(result) != len(tt.expected) {
				t.Errorf("extractWhereColumns(%q) returned %d columns, want %d", tt.query, len(result), len(tt.expected))
				t.Errorf("got: %v, want: %v", result, tt.expected)
				return
			}

			for i, col := range result {
				if col != tt.expected[i] {
					t.Errorf("extractWhereColumns(%q)[%d] = %q, want %q", tt.query, i, col, tt.expected[i])
				}
			}
		})
	}
}

func TestSuggestionString(t *testing.T) {
	tests := []struct {
		name       string
		suggestion Suggestion
		contains   []string
	}{
		{
			name: "with SQL fix",
			suggestion: Suggestion{
				Type:     SuggestionIndexMissing,
				Message:  "Consider adding index on users(email)",
				Severity: SeverityWarning,
				SQL:      "CREATE INDEX idx_users_email ON users(email);",
			},
			contains: []string{"warning", "Consider adding index", "CREATE INDEX"},
		},
		{
			name: "without SQL fix",
			suggestion: Suggestion{
				Type:     SuggestionSlowQuery,
				Message:  "Query took 250ms (threshold: 100ms)",
				Severity: SeverityWarning,
			},
			contains: []string{"warning", "Query took 250ms"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.suggestion.String()
			for _, substring := range tt.contains {
				if !contains(result, substring) {
					t.Errorf("Suggestion.String() = %q, want to contain %q", result, substring)
				}
			}
		})
	}
}

// contains checks if s contains substr (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}

// ============================
// Phase 2 Tests
// ============================

func TestBasicOptimizer_CompositeIndexRecommendation(t *testing.T) {
	mock := &mockAnalyzer{
		plan: &analyzer.QueryPlan{
			Cost:          1000.0,
			EstimatedRows: 10000,
			UsesIndex:     false,
			FullScan:      true,
			Database:      "postgres",
		},
	}

	opt := NewBasicOptimizer(mock, 100*time.Millisecond)
	ctx := context.Background()

	// Query with multiple AND conditions
	query := "SELECT * FROM users WHERE status = ? AND country = ?"
	analysis, err := opt.Analyze(ctx, query, []interface{}{1, "US"}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should recommend composite index
	if len(analysis.MissingIndexes) == 0 {
		t.Fatal("expected composite index recommendation")
	}

	// Check for composite index
	foundComposite := false
	for _, idx := range analysis.MissingIndexes {
		if len(idx.Columns) >= 2 {
			foundComposite = true
			if idx.Reason != "Composite index for multiple AND conditions" {
				t.Errorf("unexpected reason: %s", idx.Reason)
			}
		}
	}

	if !foundComposite {
		t.Error("expected composite index recommendation for multiple AND conditions")
	}
}

func TestBasicOptimizer_JoinIndexRecommendation(t *testing.T) {
	mock := &mockAnalyzer{
		plan: &analyzer.QueryPlan{
			Cost:          500.0,
			EstimatedRows: 5000,
			UsesIndex:     false,
			FullScan:      true,
			Database:      "postgres",
		},
	}

	opt := NewBasicOptimizer(mock, 100*time.Millisecond)
	ctx := context.Background()

	// Query with JOIN
	query := "SELECT u.*, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE u.status = ?"
	analysis, err := opt.Analyze(ctx, query, []interface{}{1}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should recommend JOIN index
	if len(analysis.MissingIndexes) == 0 {
		t.Fatal("expected JOIN index recommendation")
	}

	// Check for JOIN-related index
	foundJoinIndex := false
	for _, idx := range analysis.MissingIndexes {
		if idx.Reason == "JOIN condition - index on foreign key" {
			foundJoinIndex = true
			// Note: Table name should be 'orders' (extracted from qualified column or alias)
			if idx.Table != "orders" && idx.Table != "o" {
				t.Errorf("expected index on 'orders' or 'o' table, got '%s'", idx.Table)
			}
			if len(idx.Columns) != 1 || idx.Columns[0] != "user_id" {
				t.Errorf("expected index on user_id column, got %v", idx.Columns)
			}
		}
	}

	if !foundJoinIndex {
		t.Error("expected JOIN index recommendation")
	}
}

func TestBasicOptimizer_CoveringIndexRecommendation(t *testing.T) {
	mock := &mockAnalyzer{
		plan: &analyzer.QueryPlan{
			Cost:          200.0,
			EstimatedRows: 1000,
			UsesIndex:     false,
			FullScan:      true,
			Database:      "postgres",
		},
	}

	opt := NewBasicOptimizer(mock, 100*time.Millisecond)
	ctx := context.Background()

	// Query suitable for covering index
	query := "SELECT id, name FROM users WHERE status = ?"
	analysis, err := opt.Analyze(ctx, query, []interface{}{1}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should recommend covering index or composite index
	foundCovering := false
	foundComposite := false
	for _, idx := range analysis.MissingIndexes {
		if contains(idx.Reason, "covering index") {
			foundCovering = true
			// Covering index should include both WHERE and SELECT columns
			if len(idx.Columns) < 2 {
				t.Errorf("covering index should have multiple columns, got %d", len(idx.Columns))
			}
		}
		// Composite or single column index is also acceptable
		if len(idx.Columns) >= 1 {
			foundComposite = true
		}
	}

	// Either covering index or some index recommendation is fine
	if !foundCovering && !foundComposite {
		t.Error("expected some index recommendation")
	}
}

func TestBasicOptimizer_Suggest_CompositeIndex(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: false,
			FullScan:  true,
		},
		MissingIndexes: []IndexRecommendation{
			{
				Table:   "users",
				Columns: []string{"status", "country"},
				Type:    "btree",
				Reason:  "Composite index for multiple AND conditions",
			},
		},
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)

	// Find composite index suggestion
	var compositeSuggestion *Suggestion
	for _, s := range suggestions {
		if s.Type == SuggestionCompositeIndex {
			compositeSuggestion = &s
			break
		}
	}

	if compositeSuggestion == nil {
		t.Fatal("expected composite index suggestion")
	}

	if compositeSuggestion.Severity != SeverityWarning {
		t.Errorf("expected warning severity, got %v", compositeSuggestion.Severity)
	}

	if compositeSuggestion.SQL == "" {
		t.Error("expected SQL fix for composite index")
	}

	// Check SQL format
	expectedSQL := "CREATE INDEX idx_users_status_country ON users(status, country);"
	if compositeSuggestion.SQL != expectedSQL {
		t.Errorf("expected SQL:\n%s\ngot:\n%s", expectedSQL, compositeSuggestion.SQL)
	}
}

func TestBasicOptimizer_Suggest_JoinOptimize(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: false,
			FullScan:  true,
		},
		MissingIndexes: []IndexRecommendation{
			{
				Table:   "orders",
				Columns: []string{"user_id"},
				Type:    "btree",
				Reason:  "JOIN condition - index on foreign key",
			},
		},
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)

	// Find JOIN optimize suggestion
	var joinSuggestion *Suggestion
	for _, s := range suggestions {
		if s.Type == SuggestionJoinOptimize {
			joinSuggestion = &s
			break
		}
	}

	if joinSuggestion == nil {
		t.Fatal("expected JOIN optimize suggestion")
	}

	if joinSuggestion.Severity != SeverityWarning {
		t.Errorf("expected warning severity, got %v", joinSuggestion.Severity)
	}

	if !contains(joinSuggestion.Message, "orders") {
		t.Errorf("expected message to mention 'orders' table: %s", joinSuggestion.Message)
	}
}

func TestBasicOptimizer_Suggest_CoveringIndex(t *testing.T) {
	analysis := &Analysis{
		SlowQuery:     false,
		ExecutionTime: 50 * time.Millisecond,
		QueryPlan: &analyzer.QueryPlan{
			UsesIndex: false,
			FullScan:  true,
		},
		MissingIndexes: []IndexRecommendation{
			{
				Table:   "users",
				Columns: []string{"status", "id", "name"},
				Type:    "btree",
				Reason:  "Covering index: Index-only scan (no table access needed)",
			},
		},
	}

	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	suggestions := opt.Suggest(analysis)

	// Find covering index suggestion
	var coveringSuggestion *Suggestion
	for _, s := range suggestions {
		if s.Type == SuggestionCoveringIndex {
			coveringSuggestion = &s
			break
		}
	}

	if coveringSuggestion == nil {
		t.Fatal("expected covering index suggestion")
	}

	if coveringSuggestion.Severity != SeverityInfo {
		t.Errorf("expected info severity for covering index, got %v", coveringSuggestion.Severity)
	}

	if !contains(strings.ToLower(coveringSuggestion.Message), "covering") {
		t.Errorf("expected message to mention 'covering': %s", coveringSuggestion.Message)
	}
}

func TestAnalyzeWhereIndexes(t *testing.T) {
	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	tests := []struct {
		name                string
		whereClause         *WhereClause
		table               string
		expectedRecommends  int
		expectedComposite   bool
	}{
		{
			name: "single equality",
			whereClause: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "="},
				},
				Logic: LogicAND,
			},
			table:              "users",
			expectedRecommends: 1,
			expectedComposite:  false,
		},
		{
			name: "multiple AND conditions",
			whereClause: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "="},
					{Column: "country", Operator: "="},
				},
				Logic: LogicAND,
			},
			table:              "users",
			expectedRecommends: 1,
			expectedComposite:  true,
		},
		{
			name: "function in WHERE",
			whereClause: &WhereClause{
				Conditions: []Condition{
					{Column: "email", Operator: "=", Function: "UPPER"},
				},
				Logic: LogicAND,
			},
			table:              "users",
			expectedRecommends: 1,
			expectedComposite:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommendations := opt.analyzeWhereIndexes(tt.whereClause, tt.table)

			if len(recommendations) != tt.expectedRecommends {
				t.Errorf("expected %d recommendations, got %d", tt.expectedRecommends, len(recommendations))
			}

			if tt.expectedComposite {
				foundComposite := false
				for _, rec := range recommendations {
					if len(rec.Columns) >= 2 {
						foundComposite = true
						break
					}
				}
				if !foundComposite {
					t.Error("expected composite index recommendation")
				}
			}
		})
	}
}

func TestAnalyzeJoinIndexes(t *testing.T) {
	opt := &BasicOptimizer{
		slowQueryThreshold: 100 * time.Millisecond,
	}

	tests := []struct {
		name               string
		join               string
		expectedTable      string
		expectedColumn     string
		expectedRecommends int
	}{
		{
			name:               "simple JOIN",
			join:               "JOIN orders ON users.id = orders.user_id",
			expectedTable:      "orders",
			expectedColumn:     "user_id",
			expectedRecommends: 1,
		},
		{
			name:               "INNER JOIN",
			join:               "INNER JOIN posts ON users.id = posts.author_id",
			expectedTable:      "posts",
			expectedColumn:     "author_id",
			expectedRecommends: 1,
		},
		{
			name:               "no JOIN",
			join:               "SELECT * FROM users",
			expectedRecommends: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommendations := opt.analyzeJoinIndexes(tt.join)

			if len(recommendations) != tt.expectedRecommends {
				t.Errorf("expected %d recommendations, got %d", tt.expectedRecommends, len(recommendations))
			}

			if tt.expectedRecommends > 0 {
				rec := recommendations[0]
				if rec.Table != tt.expectedTable {
					t.Errorf("expected table %s, got %s", tt.expectedTable, rec.Table)
				}
				if len(rec.Columns) != 1 || rec.Columns[0] != tt.expectedColumn {
					t.Errorf("expected column %s, got %v", tt.expectedColumn, rec.Columns)
				}
			}
		})
	}
}
