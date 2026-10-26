// Package core provides the core database functionality for Relica.
package core

import (
	"context"
	"strings"
	"time"
)

// SQL operation type constants used by DetectOperation and detectOperation.
const (
	opSelect  = "SELECT"
	opInsert  = "INSERT"
	opUpdate  = "UPDATE"
	opDelete  = "DELETE"
	opUnknown = "UNKNOWN"
)

// QueryEvent contains information about an executed query.
// This is passed to QueryHook callbacks for logging, metrics, or tracing.
type QueryEvent struct {
	// SQL is the executed SQL query string
	SQL string
	// Args are the query parameters
	Args []any
	// Duration is how long the query took to execute
	Duration time.Duration
	// RowsAffected is the number of rows affected (for INSERT/UPDATE/DELETE)
	RowsAffected int64
	// Error is any error that occurred during query execution (nil on success)
	Error error
	// Operation is the SQL operation type (SELECT, INSERT, UPDATE, DELETE, UNKNOWN)
	Operation string
}

// QueryHook is a callback function invoked after each query execution.
// Use this for logging, metrics, distributed tracing, or debugging.
//
// Example:
//
//	db, _ := relica.Open("postgres", dsn,
//	    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
//	        slog.Info("query", "sql", e.SQL, "duration", e.Duration, "err", e.Error)
//	    }))
type QueryHook func(ctx context.Context, event QueryEvent)

// DetectOperation attempts to detect the SQL operation type from the query string.
// Returns one of: SELECT, INSERT, UPDATE, DELETE, or UNKNOWN. A statement that
// starts with WITH is classified by the statement after its CTEs (WITH ...
// DELETE is a DELETE); a compound select that starts with a parenthesis, as
// set operations render on PostgreSQL and MySQL, is a SELECT.
func DetectOperation(sql string) string {
	s := strings.ToUpper(sql)
	i := skipSpaceAndParens(s, 0)
	if hasWord(s, i, "WITH") {
		i = skipCTEs(s, i+len("WITH"))
		if i < 0 {
			return opUnknown
		}
		i = skipSpaceAndParens(s, i)
	}
	for _, op := range []string{opSelect, opInsert, opUpdate, opDelete} {
		if hasWord(s, i, op) {
			return op
		}
	}
	return opUnknown
}

// skipSpaceAndParens returns the index of the first byte at or after i that
// is neither white space, a comment nor an opening parenthesis.
func skipSpaceAndParens(s string, i int) int {
	for {
		i = skipSpace(s, i)
		if i >= len(s) || s[i] != '(' {
			return i
		}
		i++
	}
}

func isSQLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// hasWord reports whether the keyword w starts at s[i] and is not the prefix
// of a longer identifier (SELECTED, DELETE_LOG). s is upper case.
func hasWord(s string, i int, w string) bool {
	if !strings.HasPrefix(s[i:], w) {
		return false
	}
	end := i + len(w)
	return end == len(s) || !isIdentByte(s[end])
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c >= 0x80
}

// skipCTEs skips "[RECURSIVE] name [(cols)] AS [[NOT] MATERIALIZED] (body)"
// entries separated by commas, starting after WITH, and returns the index
// where the main statement begins, or -1 if the text does not have that shape.
func skipCTEs(s string, i int) int {
	i = skipSpace(s, i)
	if hasWord(s, i, "RECURSIVE") {
		i += len("RECURSIVE")
	}
	for {
		if i = skipCTE(s, skipSpace(s, i)); i < 0 {
			return -1
		}
		i = skipSpace(s, i)
		if i < len(s) && s[i] == ',' {
			i++
			continue
		}
		return i
	}
}

// skipCTE skips one "name [(cols)] AS [[NOT] MATERIALIZED] (body)" entry
// and returns the index after its body, or -1.
func skipCTE(s string, i int) int {
	if i = skipIdent(s, i); i < 0 {
		return -1
	}
	i = skipSpace(s, i)
	if i < len(s) && s[i] == '(' { // column list
		if i = skipBalanced(s, i); i < 0 {
			return -1
		}
		i = skipSpace(s, i)
	}
	if !hasWord(s, i, "AS") {
		return -1
	}
	i = skipSpace(s, i+len("AS"))
	if hasWord(s, i, "NOT") {
		i = skipSpace(s, i+len("NOT"))
	}
	if hasWord(s, i, "MATERIALIZED") {
		i = skipSpace(s, i+len("MATERIALIZED"))
	}
	if i >= len(s) || s[i] != '(' {
		return -1
	}
	return skipBalanced(s, i)
}

// skipSpace skips white space and -- / /* */ comments (query tags such as
// sqlcommenter's are written as leading comments). An unterminated comment
// runs to the end of the text.
func skipSpace(s string, i int) int {
	for i < len(s) {
		switch {
		case isSQLSpace(s[i]):
			i++
		case strings.HasPrefix(s[i:], "--"), strings.HasPrefix(s[i:], "/*"):
			next, _ := skipLiteralOrComment(s, i)
			if next < 0 {
				return len(s)
			}
			i = next
		default:
			return i
		}
	}
	return i
}

// skipIdent skips a bare or quoted ("x", `x`, [x]) identifier; -1 if none.
func skipIdent(s string, i int) int {
	if i >= len(s) {
		return -1
	}
	switch s[i] {
	case '"', '`':
		end := strings.IndexByte(s[i+1:], s[i])
		if end < 0 {
			return -1
		}
		return i + 1 + end + 1
	case '[':
		end := strings.IndexByte(s[i+1:], ']')
		if end < 0 {
			return -1
		}
		return i + 1 + end + 1
	}
	start := i
	for i < len(s) && isIdentByte(s[i]) {
		i++
	}
	if i == start {
		return -1
	}
	return i
}

// skipBalanced skips the parenthesized group that opens at s[i], ignoring
// parentheses inside string literals, quoted identifiers and comments; it
// returns the index after the closing parenthesis, or -1 if it is missing.
func skipBalanced(s string, i int) int {
	depth := 0
	for i < len(s) {
		if next, ok := skipLiteralOrComment(s, i); ok {
			if next < 0 {
				return -1
			}
			i = next
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return -1
}

// skipLiteralOrComment reports whether a string literal, quoted identifier or
// comment starts at s[i] and returns the index after it (-1 if unterminated).
func skipLiteralOrComment(s string, i int) (int, bool) {
	var from int
	var term string
	switch rest := s[i:]; {
	case rest[0] == '\'' || rest[0] == '"' || rest[0] == '`':
		from, term = i+1, rest[:1]
	case strings.HasPrefix(rest, "--"):
		from, term = i+2, "\n"
	case strings.HasPrefix(rest, "/*"):
		from, term = i+2, "*/"
	default:
		return i, false
	}
	end := strings.Index(s[from:], term)
	if end < 0 {
		return -1, true
	}
	return from + end + len(term), true
}

// invokeHook calls the query hook if set.
func (db *DB) invokeHook(ctx context.Context, event QueryEvent) {
	if db.queryHook != nil {
		db.queryHook(ctx, event)
	}
}
