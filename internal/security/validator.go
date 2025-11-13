// Package security provides SQL injection prevention, parameter validation,
// and query security features for Relica.
package security

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator validates SQL queries and parameters against dangerous patterns.
type Validator struct {
	patterns []*regexp.Regexp
	strict   bool
}

// ValidatorOption configures the Validator.
type ValidatorOption func(*Validator)

// WithStrict enables strict validation mode (more aggressive).
func WithStrict(strict bool) ValidatorOption {
	return func(v *Validator) {
		v.strict = strict
	}
}

// NewValidator creates a new SQL injection validator with default dangerous patterns.
func NewValidator(opts ...ValidatorOption) *Validator {
	v := &Validator{
		patterns: compilePatterns(dangerousPatterns),
		strict:   false,
	}

	for _, opt := range opts {
		opt(v)
	}

	if v.strict {
		v.patterns = append(v.patterns, compilePatterns(strictPatterns)...)
	}

	return v
}

// dangerousPatterns contains SQL injection patterns to block.
// These are patterns commonly used in SQL injection attacks (OWASP Top 10).
var dangerousPatterns = []string{
	// SQL comment indicators (used to bypass security)
	`--[\s]`,   // SQL comment (with space after)
	`/\*.*\*/`, // C-style comment
	`#[\s]`,    // MySQL comment (with space after)

	// Stacked queries (multiple statements)
	`;\s*DROP\s+`,     // ; DROP TABLE/DATABASE
	`;\s*DELETE\s+`,   // ; DELETE FROM
	`;\s*TRUNCATE\s+`, // ; TRUNCATE TABLE
	`;\s*ALTER\s+`,    // ; ALTER TABLE
	`;\s*CREATE\s+`,   // ; CREATE TABLE

	// UNION-based attacks
	`UNION\s+ALL\s+SELECT`, // UNION ALL SELECT
	`UNION\s+SELECT`,       // UNION SELECT

	// Database-specific dangerous functions
	`XP_CMDSHELL`,    // SQL Server command execution (case insensitive)
	`\bEXEC\s*\(`,    // EXEC() function with word boundary
	`\bEXECUTE\s*\(`, // EXECUTE() function with word boundary
	`SP_EXECUTESQL`,  // SQL Server dynamic SQL
	`\bEXEC\s+XP_`,   // EXEC xp_* procedures
	`\bEXEC\s+SP_`,   // EXEC sp_* procedures

	// Information schema queries (data exfiltration)
	`INFORMATION_SCHEMA`, // Access to metadata
	`PG_SLEEP\s*\(`,      // PostgreSQL sleep (timing attacks)
	`BENCHMARK\s*\(`,     // MySQL benchmark (timing attacks)
	`WAITFOR\s+DELAY`,    // SQL Server delay (timing attacks)

	// Boolean-based blind injection
	`\s+OR\s+1\s*=\s*1\b`,   // OR 1=1 (with word boundary to avoid false positives)
	`\s+OR\s+'1'\s*=\s*'1'`, // OR '1'='1'
	`\s+AND\s+1\s*=\s*0\b`,  // AND 1=0 (with word boundary)
}

// strictPatterns contains additional patterns for strict mode.
// These may have false positives but provide maximum security.
var strictPatterns = []string{
	`\bOR\b`,      // Any OR (may catch legitimate queries)
	`\bAND\b`,     // Any AND (may catch legitimate queries)
	`\bUNION\b`,   // Any UNION
	`\bEXEC\b`,    // Any EXEC
	`\bEXECUTE\b`, // Any EXECUTE
}

// ValidateQuery checks if a query contains dangerous SQL injection patterns.
// Returns an error if a dangerous pattern is detected.
func (v *Validator) ValidateQuery(query string) error {
	// Normalize query for pattern matching
	normalized := strings.ToUpper(query)

	for _, pattern := range v.patterns {
		if pattern.MatchString(normalized) {
			return fmt.Errorf("dangerous SQL pattern detected: query contains unsafe construct")
		}
	}

	return nil
}

// ValidateParams checks query parameters for SQL injection attempts.
// Looks for suspicious string patterns that may bypass prepared statements.
func (v *Validator) ValidateParams(params []interface{}) error {
	for i, param := range params {
		str, ok := param.(string)
		if !ok {
			continue
		}

		// Check for SQL injection attempts in string parameters
		if containsSQLInjection(str) {
			return fmt.Errorf("suspicious parameter value at index %d: contains SQL injection patterns", i)
		}
	}

	return nil
}

// containsSQLInjection checks if a string parameter contains SQL injection patterns.
func containsSQLInjection(value string) bool {
	// Common SQL injection indicators
	indicators := []string{
		"'--",      // Quote and comment
		"';",       // Quote and semicolon
		"' OR ",    // Quote OR
		"' AND ",   // Quote AND
		"/*",       // Comment start
		"*/",       // Comment end
		"' UNION ", // Quote UNION
		"' DROP ",  // Quote DROP
		"xp_",      // SQL Server extended procedures
	}

	upper := strings.ToUpper(value)
	for _, indicator := range indicators {
		if strings.Contains(upper, strings.ToUpper(indicator)) {
			return true
		}
	}

	return false
}

// compilePatterns compiles string patterns to regexp.Regexp.
func compilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			// Skip invalid patterns (shouldn't happen with hardcoded patterns)
			continue
		}
		compiled = append(compiled, re)
	}
	return compiled
}
