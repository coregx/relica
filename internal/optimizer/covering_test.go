package optimizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnalyzeCoveringIndex(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		recommended bool
		minColumns  int
		maxColumns  int
	}{
		{
			name:        "simple SELECT with WHERE - good candidate",
			query:       "SELECT id, name FROM users WHERE status = ?",
			recommended: true,
			minColumns:  2,
			maxColumns:  5,
		},
		{
			name:        "SELECT * - not suitable",
			query:       "SELECT * FROM users WHERE status = ?",
			recommended: false,
		},
		{
			name:        "no WHERE clause",
			query:       "SELECT id, name FROM users",
			recommended: false,
		},
		{
			name:        "multiple WHERE conditions - good",
			query:       "SELECT id, name, email FROM users WHERE status = ? AND country = ?",
			recommended: true,
			minColumns:  2,
			maxColumns:  5,
		},
		{
			name:        "too many columns",
			query:       "SELECT id, name, email, phone, address, city FROM users WHERE status = ?",
			recommended: false, // More than 5 columns
		},
		{
			name:        "JOIN query with WHERE",
			query:       "SELECT u.id, u.name FROM users u JOIN orders o ON u.id = o.user_id WHERE u.status = ?",
			recommended: true,
			minColumns:  2,
			maxColumns:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnalyzeCoveringIndex(tt.query)
			assert.Equal(t, tt.recommended, result.Recommended, "Recommendation mismatch")

			if tt.recommended {
				assert.GreaterOrEqual(t, len(result.Columns), tt.minColumns, "Too few columns")
				assert.LessOrEqual(t, len(result.Columns), tt.maxColumns, "Too many columns")
				assert.NotEmpty(t, result.Benefit, "Benefit should be set")
			} else {
				assert.NotEmpty(t, result.Benefit, "Benefit explanation should be provided")
			}
		})
	}
}

func TestCombineColumns(t *testing.T) {
	tests := []struct {
		name       string
		whereCols  []string
		selectCols []string
		expected   []string
	}{
		{
			name:       "no overlap",
			whereCols:  []string{"status", "country"},
			selectCols: []string{"id", "name"},
			expected:   []string{"status", "country", "id", "name"},
		},
		{
			name:       "with overlap",
			whereCols:  []string{"status", "id"},
			selectCols: []string{"id", "name"},
			expected:   []string{"status", "id", "name"},
		},
		{
			name:       "empty WHERE",
			whereCols:  []string{},
			selectCols: []string{"id", "name"},
			expected:   []string{"id", "name"},
		},
		{
			name:       "empty SELECT",
			whereCols:  []string{"status", "country"},
			selectCols: []string{},
			expected:   []string{"status", "country"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := combineColumns(tt.whereCols, tt.selectCols)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractWhereColumnsForCovering(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "simple WHERE",
			sql:      "SELECT * FROM users WHERE status = ?",
			expected: []string{"status"},
		},
		{
			name:     "multiple conditions",
			sql:      "SELECT * FROM users WHERE status = ? AND country = ?",
			expected: []string{"status", "country"},
		},
		{
			name:     "no WHERE",
			sql:      "SELECT * FROM users",
			expected: nil,
		},
		{
			name:     "WHERE with function",
			sql:      "SELECT * FROM users WHERE UPPER(email) = ?",
			expected: []string{"email"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractWhereColumnsForCovering(tt.sql)
			assert.Equal(t, tt.expected, result)
		})
	}
}
