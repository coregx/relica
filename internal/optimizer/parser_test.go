package optimizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWhereClause(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected *WhereClause
	}{
		{
			name: "simple equality",
			sql:  "WHERE status = ?",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "=", Function: ""},
				},
				Logic: LogicAND,
			},
		},
		{
			name: "composite AND",
			sql:  "WHERE status = ? AND email = ?",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "=", Function: ""},
					{Column: "email", Operator: "=", Function: ""},
				},
				Logic: LogicAND,
			},
		},
		{
			name: "OR logic",
			sql:  "WHERE status = ? OR name = ?",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "=", Function: ""},
					{Column: "name", Operator: "=", Function: ""},
				},
				Logic: LogicOR,
			},
		},
		{
			name: "function in WHERE",
			sql:  "WHERE UPPER(email) = ?",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "email", Operator: "=", Function: "UPPER"},
				},
				Logic: LogicAND,
			},
		},
		{
			name: "multiple operators",
			sql:  "WHERE age > ? AND status = ? AND country LIKE ?",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "age", Operator: ">", Function: ""},
					{Column: "status", Operator: "=", Function: ""},
					{Column: "country", Operator: "LIKE", Function: ""},
				},
				Logic: LogicAND,
			},
		},
		{
			name: "IN operator",
			sql:  "WHERE status IN (?, ?, ?)",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "IN", Function: ""},
				},
				Logic: LogicAND,
			},
		},
		{
			name: "NOT IN operator",
			sql:  "WHERE status NOT IN (?, ?)",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "NOT_IN", Function: ""},
				},
				Logic: LogicAND,
			},
		},
		{
			name: "comparison operators",
			sql:  "WHERE price >= ? AND quantity < ?",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "price", Operator: ">=", Function: ""},
					{Column: "quantity", Operator: "<", Function: ""},
				},
				Logic: LogicAND,
			},
		},
		{
			name: "empty WHERE",
			sql:  "",
			expected: &WhereClause{
				Conditions: []Condition{},
				Logic:      LogicAND,
			},
		},
		{
			name: "WHERE with ORDER BY",
			sql:  "WHERE status = ? ORDER BY created_at DESC",
			expected: &WhereClause{
				Conditions: []Condition{
					{Column: "status", Operator: "=", Function: ""},
				},
				Logic: LogicAND,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWhereClause(tt.sql)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Logic, result.Logic, "Logic type mismatch")
			assert.Equal(t, len(tt.expected.Conditions), len(result.Conditions), "Condition count mismatch")

			for i, expectedCond := range tt.expected.Conditions {
				if i < len(result.Conditions) {
					assert.Equal(t, expectedCond.Column, result.Conditions[i].Column, "Column mismatch at index %d", i)
					assert.Equal(t, expectedCond.Operator, result.Conditions[i].Operator, "Operator mismatch at index %d", i)
					assert.Equal(t, expectedCond.Function, result.Conditions[i].Function, "Function mismatch at index %d", i)
				}
			}
		})
	}
}

func TestExtractJoinColumns(t *testing.T) {
	tests := []struct {
		name     string
		join     string
		leftCol  string
		rightCol string
	}{
		{
			name:     "simple JOIN",
			join:     "JOIN orders ON users.id = orders.user_id",
			leftCol:  "users.id",
			rightCol: "orders.user_id",
		},
		{
			name:     "INNER JOIN",
			join:     "INNER JOIN posts ON users.id = posts.author_id",
			leftCol:  "users.id",
			rightCol: "posts.author_id",
		},
		{
			name:     "LEFT JOIN with alias",
			join:     "LEFT JOIN orders o ON u.id = o.user_id",
			leftCol:  "u.id",
			rightCol: "orders.user_id", // Alias 'o' mapped to 'orders'
		},
		{
			name:     "no JOIN",
			join:     "SELECT * FROM users",
			leftCol:  "",
			rightCol: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leftCol, rightCol := extractJoinColumns(tt.join)
			assert.Equal(t, tt.leftCol, leftCol, "Left column mismatch")
			assert.Equal(t, tt.rightCol, rightCol, "Right column mismatch")
		})
	}
}

func TestExtractTableNameFromColumn(t *testing.T) {
	tests := []struct {
		name     string
		column   string
		expected string
	}{
		{
			name:     "qualified column",
			column:   "users.id",
			expected: "users",
		},
		{
			name:     "unqualified column",
			column:   "id",
			expected: "",
		},
		{
			name:     "multiple dots",
			column:   "schema.users.id",
			expected: "schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTableNameFromColumn(tt.column)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractColumnName(t *testing.T) {
	tests := []struct {
		name     string
		column   string
		expected string
	}{
		{
			name:     "qualified column",
			column:   "users.id",
			expected: "id",
		},
		{
			name:     "unqualified column",
			column:   "id",
			expected: "id",
		},
		{
			name:     "multiple dots",
			column:   "schema.users.id",
			expected: "id", // Returns last part
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractColumnName(tt.column)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractJoinClauses(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected int // Number of JOIN clauses
	}{
		{
			name:     "single JOIN",
			sql:      "SELECT * FROM users JOIN orders ON users.id = orders.user_id",
			expected: 1,
		},
		{
			name:     "multiple JOINs",
			sql:      "SELECT * FROM users JOIN orders ON users.id = orders.user_id LEFT JOIN posts ON users.id = posts.author_id",
			expected: 2,
		},
		{
			name:     "no JOIN",
			sql:      "SELECT * FROM users WHERE status = 1",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joins := extractJoinClauses(tt.sql)
			assert.Equal(t, tt.expected, len(joins), "JOIN count mismatch")
		})
	}
}

func TestExtractSelectColumns(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "SELECT *",
			sql:      "SELECT * FROM users",
			expected: []string{"*"},
		},
		{
			name:     "single column",
			sql:      "SELECT id FROM users",
			expected: []string{"id"},
		},
		{
			name:     "multiple columns",
			sql:      "SELECT id, name, email FROM users",
			expected: []string{"id", "name", "email"},
		},
		{
			name:     "qualified columns",
			sql:      "SELECT u.id, u.name FROM users u",
			expected: []string{"id", "name"},
		},
		{
			name:     "columns with aliases",
			sql:      "SELECT id as user_id, name as user_name FROM users",
			expected: []string{"id", "name"},
		},
		{
			name:     "no SELECT",
			sql:      "UPDATE users SET status = 1",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSelectColumns(tt.sql)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeOperator(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		expected string
	}{
		{
			name:     "not equal <>",
			operator: "<>",
			expected: "!=",
		},
		{
			name:     "not equal !=",
			operator: "!=",
			expected: "!=",
		},
		{
			name:     "NOT IN",
			operator: "not in",
			expected: "NOT_IN",
		},
		{
			name:     "equal",
			operator: "=",
			expected: "=",
		},
		{
			name:     "LIKE",
			operator: "like",
			expected: "LIKE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeOperator(tt.operator)
			assert.Equal(t, tt.expected, result)
		})
	}
}
