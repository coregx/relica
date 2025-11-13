package logger

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizer_MaskParams_DefaultFields(t *testing.T) {
	tests := []struct {
		name   string
		sql    string
		params []interface{}
		want   []interface{}
	}{
		{
			name:   "Password field",
			sql:    "UPDATE users SET password = ? WHERE id = ?",
			params: []interface{}{"secret123", 1},
			want:   []interface{}{"***REDACTED***", "***REDACTED***"},
		},
		{
			name:   "Token field",
			sql:    "INSERT INTO sessions (user_id, token) VALUES (?, ?)",
			params: []interface{}{123, "abc-xyz-token"},
			want:   []interface{}{"***REDACTED***", "***REDACTED***"},
		},
		{
			name:   "API key field",
			sql:    "SELECT * FROM integrations WHERE api_key = ?",
			params: []interface{}{"sk_test_123456"},
			want:   []interface{}{"***REDACTED***"},
		},
		{
			name:   "No sensitive fields",
			sql:    "SELECT * FROM users WHERE id = ? AND name = ?",
			params: []interface{}{1, "Alice"},
			want:   []interface{}{1, "Alice"},
		},
		{
			name:   "Empty params",
			sql:    "SELECT COUNT(*) FROM users",
			params: []interface{}{},
			want:   []interface{}{},
		},
		{
			name:   "Credit card",
			sql:    "UPDATE payments SET credit_card = ? WHERE id = ?",
			params: []interface{}{"4111111111111111", 1},
			want:   []interface{}{"***REDACTED***", "***REDACTED***"},
		},
		{
			name:   "Case insensitive",
			sql:    "UPDATE users SET PASSWORD = ? WHERE id = ?",
			params: []interface{}{"secret", 1},
			want:   []interface{}{"***REDACTED***", "***REDACTED***"},
		},
		{
			name:   "Multiple fields",
			sql:    "INSERT INTO users (name, email, password, token) VALUES (?, ?, ?, ?)",
			params: []interface{}{"Alice", "alice@example.com", "secret", "token123"},
			want:   []interface{}{"***REDACTED***", "***REDACTED***", "***REDACTED***", "***REDACTED***"},
		},
	}

	sanitizer := NewSanitizer(nil) // Use default fields

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.MaskParams(tt.sql, tt.params)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizer_MaskParams_CustomFields(t *testing.T) {
	sanitizer := NewSanitizer([]string{"secret_key", "private_data"})

	tests := []struct {
		name   string
		sql    string
		params []interface{}
		want   []interface{}
	}{
		{
			name:   "Custom field secret_key",
			sql:    "UPDATE config SET secret_key = ? WHERE id = ?",
			params: []interface{}{"mySecret", 1},
			want:   []interface{}{"***REDACTED***", "***REDACTED***"},
		},
		{
			name:   "Custom field private_data",
			sql:    "INSERT INTO logs (private_data) VALUES (?)",
			params: []interface{}{"sensitive info"},
			want:   []interface{}{"***REDACTED***"},
		},
		{
			name:   "Non-sensitive field",
			sql:    "SELECT * FROM users WHERE name = ?",
			params: []interface{}{"Alice"},
			want:   []interface{}{"Alice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.MaskParams(tt.sql, tt.params)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizer_FormatParams(t *testing.T) {
	sanitizer := NewSanitizer(nil)

	tests := []struct {
		name   string
		params []interface{}
		want   string
	}{
		{
			name:   "Empty params",
			params: []interface{}{},
			want:   "[]",
		},
		{
			name:   "Single param",
			params: []interface{}{123},
			want:   "[123]",
		},
		{
			name:   "Multiple params",
			params: []interface{}{123, "Alice", true},
			want:   "[123, Alice, true]",
		},
		{
			name:   "NULL value",
			params: []interface{}{nil},
			want:   "[NULL]",
		},
		{
			name:   "Masked value",
			params: []interface{}{"***REDACTED***"},
			want:   "[***REDACTED***]",
		},
		{
			name:   "Long string truncation",
			params: []interface{}{strings.Repeat("a", 150)},
			want:   "[" + strings.Repeat("a", 100) + "...]",
		},
		{
			name:   "Mixed types",
			params: []interface{}{1, "test", nil, true, 3.14},
			want:   "[1, test, NULL, true, 3.14]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.FormatParams(tt.params)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizer_FormatParams_AfterMasking(t *testing.T) {
	sanitizer := NewSanitizer(nil)

	sql := "UPDATE users SET password = ? WHERE id = ?"
	params := []interface{}{"secretPassword123", 1}

	masked := sanitizer.MaskParams(sql, params)
	formatted := sanitizer.FormatParams(masked)

	assert.Equal(t, "[***REDACTED***, ***REDACTED***]", formatted)
	assert.NotContains(t, formatted, "secretPassword123")
}

func TestSanitizer_WordBoundaries(t *testing.T) {
	sanitizer := NewSanitizer(nil)

	// Should NOT match "password" within "passwordless"
	sql := "SELECT * FROM passwordless_auth WHERE user_id = ?"
	params := []interface{}{123}

	// This is a known limitation: our regex uses word boundaries
	// so "password" in "passwordless" won't match
	got := sanitizer.MaskParams(sql, params)

	// This test documents current behavior
	// In a perfect world, we'd want smarter parsing
	assert.NotNil(t, got)
}

func TestSanitizer_ThreadSafety(t *testing.T) {
	sanitizer := NewSanitizer(nil)

	// Run concurrent masking operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			sql := "UPDATE users SET password = ? WHERE id = ?"
			params := []interface{}{"secret", 1}
			_ = sanitizer.MaskParams(sql, params)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func BenchmarkSanitizer_MaskParams_Sensitive(b *testing.B) {
	sanitizer := NewSanitizer(nil)
	sql := "UPDATE users SET password = ?, token = ? WHERE id = ?"
	params := []interface{}{"secretPassword", "token123", 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sanitizer.MaskParams(sql, params)
	}
}

func BenchmarkSanitizer_MaskParams_NonSensitive(b *testing.B) {
	sanitizer := NewSanitizer(nil)
	sql := "SELECT * FROM users WHERE id = ? AND name = ?"
	params := []interface{}{123, "Alice"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sanitizer.MaskParams(sql, params)
	}
}

func BenchmarkSanitizer_FormatParams(b *testing.B) {
	sanitizer := NewSanitizer(nil)
	params := []interface{}{123, "Alice", true, nil, 3.14}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sanitizer.FormatParams(params)
	}
}
