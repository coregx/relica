//go:build integration
// +build integration

package test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/require"
)

// testdataDir returns the absolute path to the test/testdata directory.
// It uses the caller's file location so the path is correct regardless
// of the working directory used when running tests.
func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

// SetupTestData reads the dialect-specific SQL file and executes all statements
// against the given database. Statements are split on ";" so multi-statement
// schema files work correctly.
//
// Supported dialects: "postgres", "mysql", "sqlite".
func SetupTestData(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()

	sqlFile := filepath.Join(testdataDir(), dialect+".sql")
	data, err := os.ReadFile(sqlFile)
	require.NoError(t, err, "read testdata file %s", sqlFile)

	ctx := context.Background()

	// Split on semicolons and execute each non-empty statement individually.
	// This is necessary because database/sql does not support multi-statement
	// execution in a single ExecContext call (driver-dependent, avoid it).
	statements := strings.Split(string(data), ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)

		// Skip comment-only blocks and empty tokens produced by trailing ";"
		if stmt == "" || isCommentOnly(stmt) {
			continue
		}

		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err, "execute testdata statement: %s", truncate(stmt, 120))
	}
}

// CleanupTestData drops all tables that SetupTestData creates.
// Safe to call even when some tables do not exist (uses IF EXISTS).
func CleanupTestData(t *testing.T, db *relica.DB) {
	t.Helper()

	ctx := context.Background()

	// Drop in reverse-FK order so constraints are not violated.
	tables := []string{
		"test_employees",
		"test_companies",
		"test_reserved",
		"test_products",
	}

	for _, tbl := range tables {
		_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+tbl)
		// Best-effort: log but do not fail the test on cleanup errors.
		if err != nil {
			t.Logf("CleanupTestData: drop %s: %v", tbl, err)
		}
	}
}

// isCommentOnly reports whether s contains only SQL line comments (-- ...).
func isCommentOnly(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		return false
	}
	return true
}

// truncate returns s truncated to at most n bytes with "..." appended when cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
