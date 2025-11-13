package logger

import (
	"fmt"
	"regexp"
	"strings"
)

// Sanitizer masks sensitive data in query parameters to prevent accidental logging of secrets.
// It detects sensitive fields based on SQL column names and parameter patterns.
type Sanitizer struct {
	sensitiveFields []string
	maskValue       string
	// Compiled patterns for faster matching
	patterns []*regexp.Regexp
}

// NewSanitizer creates a new sanitizer with the specified sensitive field names.
// If no fields are provided, a default set of common sensitive field names is used.
func NewSanitizer(sensitiveFields []string) *Sanitizer {
	if len(sensitiveFields) == 0 {
		// Default sensitive field names (common patterns)
		sensitiveFields = []string{
			"password", "passwd", "pwd",
			"token", "api_key", "apikey", "api_token",
			"secret", "auth", "authorization",
			"credit_card", "card_number", "cvv", "cvc",
			"ssn", "social_security",
			"private_key", "priv_key",
		}
	}

	// Compile patterns for efficient matching
	patterns := make([]*regexp.Regexp, 0, len(sensitiveFields))
	for _, field := range sensitiveFields {
		// Match field name in SQL (case-insensitive, with word boundaries)
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(field) + `\b`)
		patterns = append(patterns, pattern)
	}

	return &Sanitizer{
		sensitiveFields: sensitiveFields,
		maskValue:       "***REDACTED***",
		patterns:        patterns,
	}
}

// MaskParams masks sensitive parameters based on field names detected in the SQL query.
// It returns a new slice with sensitive values replaced by the mask value.
// Original parameters are not modified.
func (s *Sanitizer) MaskParams(sql string, params []interface{}) []interface{} {
	if len(params) == 0 {
		return params
	}

	// Check if SQL contains any sensitive field names
	hasSensitive := false
	sqlLower := strings.ToLower(sql)
	for _, pattern := range s.patterns {
		if pattern.MatchString(sqlLower) {
			hasSensitive = true
			break
		}
	}

	// If no sensitive fields detected, return original params
	if !hasSensitive {
		return params
	}

	// Create masked copy
	masked := make([]interface{}, len(params))
	for i, param := range params {
		if s.isSensitiveParam(sql, i, param) {
			masked[i] = s.maskValue
		} else {
			masked[i] = param
		}
	}

	return masked
}

// isSensitiveParam determines if a parameter should be masked.
// It uses heuristics based on SQL structure and parameter value types.
func (s *Sanitizer) isSensitiveParam(sql string, _ int, param interface{}) bool {
	// For now, mask all params if sensitive fields are detected in SQL
	// Future enhancement: parse SQL to determine exact parameter positions
	sqlLower := strings.ToLower(sql)

	// Check if this is likely a sensitive value based on type and content
	if v, ok := param.(string); ok {
		// Mask long strings that might be passwords/tokens (heuristic)
		if len(v) > 8 && s.containsSensitivePattern(sqlLower) {
			return true
		}
	}

	return s.containsSensitivePattern(sqlLower)
}

// containsSensitivePattern checks if SQL contains any sensitive field patterns.
func (s *Sanitizer) containsSensitivePattern(sql string) bool {
	for _, pattern := range s.patterns {
		if pattern.MatchString(sql) {
			return true
		}
	}
	return false
}

// FormatParams converts parameters to a safe string representation for logging.
// Sensitive values should be masked using MaskParams before calling this.
func (s *Sanitizer) FormatParams(params []interface{}) string {
	if len(params) == 0 {
		return "[]"
	}

	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = s.formatValue(p)
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

// formatValue formats a single parameter value for logging.
// Truncates very long strings to prevent log pollution.
func (s *Sanitizer) formatValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}

	str := fmt.Sprintf("%v", v)

	// Truncate very long values
	const maxLen = 100
	if len(str) > maxLen {
		return str[:maxLen] + "..."
	}

	return str
}
