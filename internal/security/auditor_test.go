package security

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// mockResult implements sql.Result for testing
type mockResult struct {
	rows int64
	id   int64
}

func (m *mockResult) LastInsertId() (int64, error) {
	return m.id, nil
}

func (m *mockResult) RowsAffected() (int64, error) {
	return m.rows, nil
}

func TestAuditor_LogOperation(t *testing.T) {
	tests := []struct {
		name      string
		level     AuditLevel
		operation string
		query     string
		args      []interface{}
		result    sql.Result
		err       error
		wantLog   bool
	}{
		{
			name:      "write_operation_audit_writes",
			level:     AuditWrites,
			operation: "INSERT",
			query:     "INSERT INTO users (name) VALUES (?)",
			args:      []interface{}{"Alice"},
			result:    &mockResult{rows: 1},
			err:       nil,
			wantLog:   true,
		},
		{
			name:      "read_operation_audit_writes",
			level:     AuditWrites,
			operation: "SELECT",
			query:     "SELECT * FROM users",
			args:      nil,
			result:    nil,
			err:       nil,
			wantLog:   false, // Reads not logged in AuditWrites mode
		},
		{
			name:      "read_operation_audit_reads",
			level:     AuditReads,
			operation: "SELECT",
			query:     "SELECT * FROM users WHERE id = ?",
			args:      []interface{}{123},
			result:    nil,
			err:       nil,
			wantLog:   true,
		},
		{
			name:      "failed_operation",
			level:     AuditWrites,
			operation: "UPDATE",
			query:     "UPDATE users SET status = ? WHERE id = ?",
			args:      []interface{}{1, 999},
			result:    nil,
			err:       errors.New("record not found"),
			wantLog:   true,
		},
		{
			name:      "audit_none",
			level:     AuditNone,
			operation: "DELETE",
			query:     "DELETE FROM users WHERE id = ?",
			args:      []interface{}{1},
			result:    &mockResult{rows: 1},
			err:       nil,
			wantLog:   false, // Nothing logged in AuditNone mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture log output
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			auditor := NewAuditor(logger, tt.level)
			ctx := context.Background()

			auditor.LogOperation(ctx, tt.operation, tt.query, tt.args, tt.result, tt.err, 10*time.Millisecond)

			// Check if log was written
			logOutput := buf.String()
			if tt.wantLog && logOutput == "" {
				t.Error("Expected audit log but got none")
			}
			if !tt.wantLog && logOutput != "" {
				t.Errorf("Expected no audit log but got: %s", logOutput)
			}

			// If log was expected, verify content
			if tt.wantLog && logOutput != "" {
				if !strings.Contains(logOutput, tt.operation) {
					t.Errorf("Log missing operation: %s", logOutput)
				}
				if !strings.Contains(logOutput, tt.query) {
					t.Errorf("Log missing query: %s", logOutput)
				}
			}
		})
	}
}

func TestAuditor_ContextMetadata(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	auditor := NewAuditor(logger, AuditAll)

	// Create context with metadata
	ctx := context.Background()
	ctx = WithUser(ctx, "john.doe@example.com")
	ctx = WithClientIP(ctx, "192.168.1.100")
	ctx = WithRequestID(ctx, "req-12345")

	auditor.LogOperation(ctx, "INSERT", "INSERT INTO logs (message) VALUES (?)",
		[]interface{}{"test message"}, &mockResult{rows: 1}, nil, 5*time.Millisecond)

	logOutput := buf.String()

	// Verify context metadata in log
	if !strings.Contains(logOutput, "john.doe@example.com") {
		t.Error("Log missing user from context")
	}
	if !strings.Contains(logOutput, "192.168.1.100") {
		t.Error("Log missing client IP from context")
	}
	if !strings.Contains(logOutput, "req-12345") {
		t.Error("Log missing request ID from context")
	}
}

func TestAuditor_ParamsHash(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	auditor := NewAuditor(logger, AuditWrites)
	ctx := context.Background()

	// Log operation with parameters
	auditor.LogOperation(ctx, "INSERT", "INSERT INTO users (name, email) VALUES (?, ?)",
		[]interface{}{"Alice", "alice@example.com"}, &mockResult{rows: 1}, nil, 10*time.Millisecond)

	logOutput := buf.String()

	// Verify params_hash is present (but actual values are not)
	if !strings.Contains(logOutput, "params_hash") {
		t.Error("Log missing params_hash")
	}
	// Ensure actual parameter values are NOT in the log
	if strings.Contains(logOutput, "alice@example.com") {
		t.Error("Log contains sensitive parameter value (should be hashed)")
	}

	// Verify hash is consistent
	hash1 := hashParams([]interface{}{"Alice", "alice@example.com"})
	hash2 := hashParams([]interface{}{"Alice", "alice@example.com"})
	if hash1 != hash2 {
		t.Error("Parameter hash is not consistent")
	}

	// Verify different params produce different hashes
	hash3 := hashParams([]interface{}{"Bob", "bob@example.com"})
	if hash1 == hash3 {
		t.Error("Different parameters produced same hash")
	}
}

func TestAuditor_LogSecurityEvent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	auditor := NewAuditor(logger, AuditAll)

	ctx := context.Background()
	ctx = WithUser(ctx, "attacker@evil.com")
	ctx = WithClientIP(ctx, "10.0.0.1")

	auditor.LogSecurityEvent(ctx, "query_blocked",
		"SELECT * FROM users WHERE id = 1 OR 1=1",
		errors.New("dangerous SQL pattern detected"))

	logOutput := buf.String()

	// Verify security event is logged
	if !strings.Contains(logOutput, "security_event") {
		t.Error("Log missing security_event marker")
	}
	if !strings.Contains(logOutput, "query_blocked") {
		t.Error("Log missing event type")
	}
	if !strings.Contains(logOutput, "attacker@evil.com") {
		t.Error("Log missing user")
	}
	if !strings.Contains(logOutput, "dangerous SQL pattern") {
		t.Error("Log missing error message")
	}
}

func TestAuditor_NilLogger(t *testing.T) {
	// Auditor with nil logger should not panic
	auditor := NewAuditor(nil, AuditAll)
	ctx := context.Background()

	// Should not panic
	auditor.LogOperation(ctx, "INSERT", "INSERT INTO test VALUES (?)",
		[]interface{}{1}, &mockResult{rows: 1}, nil, 1*time.Millisecond)

	auditor.LogSecurityEvent(ctx, "test_event", "SELECT 1", errors.New("test error"))
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	// Test WithUser and GetUser
	ctx = WithUser(ctx, "test.user@example.com")
	if user := GetUser(ctx); user != "test.user@example.com" {
		t.Errorf("GetUser() = %s, want test.user@example.com", user)
	}

	// Test WithClientIP and GetClientIP
	ctx = WithClientIP(ctx, "172.16.0.1")
	if ip := GetClientIP(ctx); ip != "172.16.0.1" {
		t.Errorf("GetClientIP() = %s, want 172.16.0.1", ip)
	}

	// Test WithRequestID and GetRequestID
	ctx = WithRequestID(ctx, "req-xyz-789")
	if reqID := GetRequestID(ctx); reqID != "req-xyz-789" {
		t.Errorf("GetRequestID() = %s, want req-xyz-789", reqID)
	}

	// Test empty context
	emptyCtx := context.Background()
	if user := GetUser(emptyCtx); user != "" {
		t.Errorf("GetUser(empty) = %s, want empty string", user)
	}
}

func TestHashParams(t *testing.T) {
	tests := []struct {
		name   string
		params []interface{}
		want   string
	}{
		{
			name:   "empty_params",
			params: []interface{}{},
			want:   "",
		},
		{
			name:   "single_param",
			params: []interface{}{"test"},
			want:   "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", // SHA256 of "test"
		},
		{
			name:   "multiple_params",
			params: []interface{}{123, "test", true},
			want:   "", // Just verify it returns non-empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashParams(tt.params)

			if tt.want == "" && len(tt.params) == 0 {
				if got != "" {
					t.Errorf("hashParams() = %s, want empty string for empty params", got)
				}
			} else if tt.want != "" {
				if got != tt.want {
					t.Errorf("hashParams() = %s, want %s", got, tt.want)
				}
			} else if len(tt.params) > 0 {
				if got == "" {
					t.Error("hashParams() returned empty string for non-empty params")
				}
				if len(got) != 64 { // SHA256 produces 64 hex characters
					t.Errorf("hashParams() produced hash of length %d, want 64", len(got))
				}
			}
		})
	}
}

func TestAuditEvent_JSONSerialization(t *testing.T) {
	event := AuditEvent{
		Timestamp:    time.Date(2025, 1, 24, 10, 0, 0, 0, time.UTC),
		User:         "test@example.com",
		Operation:    "INSERT",
		Table:        "users",
		AffectedRows: 1,
		SQL:          "INSERT INTO users (name) VALUES (?)",
		ParamsHash:   "abc123",
		ClientIP:     "192.168.1.1",
		RequestID:    "req-001",
		Success:      true,
		Duration:     15,
	}

	// Serialize to JSON
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal AuditEvent: %v", err)
	}

	// Deserialize back
	var decoded AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal AuditEvent: %v", err)
	}

	// Verify key fields
	if decoded.User != event.User {
		t.Errorf("User mismatch: got %s, want %s", decoded.User, event.User)
	}
	if decoded.Operation != event.Operation {
		t.Errorf("Operation mismatch: got %s, want %s", decoded.Operation, event.Operation)
	}
	if decoded.Success != event.Success {
		t.Errorf("Success mismatch: got %v, want %v", decoded.Success, event.Success)
	}
}
