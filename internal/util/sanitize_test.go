package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSanitizeString tests removal of dangerous SQL patterns.
func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean string passes through",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "removes SELECT keyword",
			input: "SELECT * FROM users",
			want:  " * FROM users",
		},
		{
			name:  "removes INSERT keyword",
			input: "INSERT INTO users",
			want:  " INTO users",
		},
		{
			name:  "removes UPDATE keyword",
			input: "UPDATE users SET",
			want:  " users SET",
		},
		{
			name:  "removes DELETE keyword",
			input: "DELETE FROM users",
			want:  " FROM users",
		},
		{
			name:  "removes DROP keyword",
			input: "DROP TABLE users",
			want:  " TABLE users",
		},
		{
			name:  "removes ALTER keyword",
			input: "ALTER TABLE users",
			want:  " TABLE users",
		},
		{
			name:  "removes UNION keyword",
			input: "1 UNION SELECT",
			want:  "1  ",
		},
		{
			name:  "removes SQL comment --",
			input: "1--comment",
			want:  "1comment",
		},
		{
			name:  "removes semicolon",
			input: "value; DROP TABLE",
			want:  "value  TABLE",
		},
		{
			name:  "removes block comment /*",
			input: "value /* comment",
			want:  "value  comment",
		},
		{
			name:  "case insensitive - lowercase select",
			input: "select * from users",
			want:  " * from users",
		},
		{
			name:  "case insensitive - mixed case",
			input: "SeLeCt * from users",
			want:  " * from users",
		},
		{
			name:  "normal identifier untouched",
			input: "username123",
			want:  "username123",
		},
		{
			name:  "numbers only",
			input: "12345",
			want:  "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeString(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSanitizeIdentifier tests removal of non-word characters from identifiers.
func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean identifier passes through",
			input: "users",
			want:  "users",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "identifier with underscore",
			input: "user_name",
			want:  "user_name",
		},
		{
			name:  "identifier with numbers",
			input: "table123",
			want:  "table123",
		},
		{
			name:  "removes dot",
			input: "schema.table",
			want:  "schematable",
		},
		{
			name:  "removes hyphen",
			input: "my-table",
			want:  "mytable",
		},
		{
			name:  "removes spaces",
			input: "my table",
			want:  "mytable",
		},
		{
			name:  "removes SQL injection attempt with semicolon",
			input: "users; DROP TABLE users",
			want:  "usersDROPTABLEusers",
		},
		{
			name:  "removes single quote",
			input: "user's",
			want:  "users",
		},
		{
			name:  "removes double quote",
			input: `"users"`,
			want:  "users",
		},
		{
			name:  "removes backtick",
			input: "`users`",
			want:  "users",
		},
		{
			name:  "removes parentheses",
			input: "func()",
			want:  "func",
		},
		{
			name:  "removes asterisk",
			input: "table*",
			want:  "table",
		},
		{
			name:  "uppercase letters preserved",
			input: "UserName",
			want:  "UserName",
		},
		{
			name:  "mixed alphanumeric and underscore",
			input: "my_Table_123",
			want:  "my_Table_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeIdentifier(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
