package core

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/coregx/relica/internal/security"
	_ "modernc.org/sqlite"
)

func TestDB_WithAuditLog_ExecContext(t *testing.T) {
	// Create audit logger
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	auditor := security.NewAuditor(logger, security.AuditWrites)

	// Create DB with auditor
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.auditor = auditor
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	ctx = security.WithUser(ctx, "test.user@example.com")
	ctx = security.WithClientIP(ctx, "192.168.1.1")
	ctx = security.WithRequestID(ctx, "req-001")

	// Execute INSERT
	_, err = db.ExecContext(ctx, "INSERT INTO users (id, name) VALUES (?, ?)", 1, "Alice")
	if err != nil {
		t.Fatal(err)
	}

	logOutput := buf.String()

	// Verify audit log contains expected fields
	if !strings.Contains(logOutput, "audit_event") {
		t.Error("Audit log missing audit_event marker")
	}
	if !strings.Contains(logOutput, "INSERT") {
		t.Error("Audit log missing operation type")
	}
	if !strings.Contains(logOutput, "test.user@example.com") {
		t.Error("Audit log missing user from context")
	}
	if !strings.Contains(logOutput, "192.168.1.1") {
		t.Error("Audit log missing client IP from context")
	}
	if !strings.Contains(logOutput, "req-001") {
		t.Error("Audit log missing request ID from context")
	}
	if !strings.Contains(logOutput, "params_hash") {
		t.Error("Audit log missing params_hash")
	}
	// Verify actual parameter values are NOT in log
	if strings.Contains(logOutput, "\"Alice\"") {
		t.Error("Audit log contains raw parameter value (should be hashed)")
	}
}

func TestDB_WithAuditLog_QueryContext(t *testing.T) {
	// Create audit logger with AuditReads level
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	auditor := security.NewAuditor(logger, security.AuditReads)

	// Create DB
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.auditor = auditor
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.sqlDB.Exec("INSERT INTO users (id, name) VALUES (1, 'Alice')")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	ctx = security.WithUser(ctx, "analyst@example.com")

	// Execute SELECT
	rows, err := db.QueryContext(ctx, "SELECT * FROM users WHERE id = ?", 1)
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()

	logOutput := buf.String()

	// Verify SELECT is logged (AuditReads level)
	if !strings.Contains(logOutput, "SELECT") {
		t.Error("Audit log missing SELECT operation")
	}
	if !strings.Contains(logOutput, "analyst@example.com") {
		t.Error("Audit log missing user")
	}
}

func TestDB_WithAuditLog_FailedOperation(t *testing.T) {
	// Create audit logger
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	auditor := security.NewAuditor(logger, security.AuditWrites)

	// Create DB
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.auditor = auditor
	defer db.Close()

	ctx := context.Background()

	// Execute query on non-existent table (will fail)
	_, err = db.ExecContext(ctx, "INSERT INTO nonexistent (id) VALUES (?)", 1)
	if err == nil {
		t.Error("Expected error for non-existent table")
	}

	logOutput := buf.String()

	// Verify failure is logged
	if !strings.Contains(logOutput, "audit_event") {
		t.Error("Failed operation not logged")
	}
	if !strings.Contains(logOutput, "success") {
		t.Error("Audit log missing success field")
	}
	// Should be at WARN level due to failure
	if !strings.Contains(logOutput, "WARN") && !strings.Contains(logOutput, "warn") {
		t.Error("Failed operation not logged at WARN level")
	}
}

func TestDB_WithAuditLog_SecurityEvent(t *testing.T) {
	// Create audit logger
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	auditor := security.NewAuditor(logger, security.AuditAll)
	validator := security.NewValidator()

	// Create DB with both validator and auditor
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.auditor = auditor
	db.validator = validator
	defer db.Close()

	ctx := context.Background()
	ctx = security.WithUser(ctx, "attacker@evil.com")
	ctx = security.WithClientIP(ctx, "10.0.0.666")

	// Attempt SQL injection (should be blocked)
	_, err = db.ExecContext(ctx, "SELECT * FROM users; DROP TABLE users", nil)
	if err == nil {
		t.Error("Expected SQL injection to be blocked")
	}

	logOutput := buf.String()

	// Verify security event is logged
	if !strings.Contains(logOutput, "security_event") {
		t.Error("Security event not logged")
	}
	if !strings.Contains(logOutput, "query_blocked") {
		t.Error("Log missing event type")
	}
	if !strings.Contains(logOutput, "attacker@evil.com") {
		t.Error("Log missing attacker user")
	}
	if !strings.Contains(logOutput, "10.0.0.666") {
		t.Error("Log missing attacker IP")
	}
}

func TestDB_WithoutAuditLog(t *testing.T) {
	// Create DB WITHOUT auditor
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create test table
	_, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Execute query (should work without auditing)
	_, err = db.ExecContext(ctx, "INSERT INTO users (id, name) VALUES (?, ?)", 1, "Alice")
	if err != nil {
		t.Fatalf("Query failed without auditor: %v", err)
	}

	// No audit log expected - just verify it doesn't crash
}

func TestDB_AuditLevel_Writes(t *testing.T) {
	// Create audit logger with AuditWrites level
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	auditor := security.NewAuditor(logger, security.AuditWrites)

	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.auditor = auditor
	defer db.Close()

	_, err = db.sqlDB.Exec("CREATE TABLE test (id INTEGER)")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// INSERT should be logged (write operation)
	buf.Reset()
	_, err = db.ExecContext(ctx, "INSERT INTO test (id) VALUES (?)", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "INSERT") {
		t.Error("INSERT not logged in AuditWrites mode")
	}

	// SELECT should NOT be logged (read operation)
	buf.Reset()
	rows, err := db.QueryContext(ctx, "SELECT * FROM test")
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if strings.Contains(buf.String(), "SELECT") {
		t.Error("SELECT logged in AuditWrites mode (should be skipped)")
	}
}

func TestDetectOperation(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"INSERT INTO users VALUES (1)", "INSERT"},
		{"  insert into users values (1)", "INSERT"},
		{"UPDATE users SET name = ?", "UPDATE"},
		{"DELETE FROM users WHERE id = ?", "DELETE"},
		{"SELECT * FROM users", "SELECT"},
		{"CREATE TABLE test (id INT)", "CREATE"},
		{"DROP TABLE test", "DROP"},
		{"ALTER TABLE users ADD COLUMN age INT", "ALTER"},
		{"TRUNCATE TABLE logs", "TRUNCATE"},
		{"UNKNOWN COMMAND", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := detectOperation(tt.query)
			if got != tt.want {
				t.Errorf("detectOperation(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}
