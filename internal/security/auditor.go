package security

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

// AuditLevel defines the level of audit logging.
type AuditLevel int

const (
	// AuditNone disables audit logging.
	AuditNone AuditLevel = iota
	// AuditWrites logs only write operations (INSERT, UPDATE, DELETE).
	AuditWrites
	// AuditReads logs read operations (SELECT) in addition to writes.
	AuditReads
	// AuditAll logs all database operations including utility commands.
	AuditAll
)

// AuditEvent represents a single database operation for audit logging.
type AuditEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	User         string    `json:"user,omitempty"`        // User from context
	Operation    string    `json:"operation"`             // SELECT, INSERT, UPDATE, DELETE
	Table        string    `json:"table,omitempty"`       // Target table (if detectable)
	AffectedRows int64     `json:"affected_rows"`         // Number of rows affected
	SQL          string    `json:"sql"`                   // Query string
	ParamsHash   string    `json:"params_hash,omitempty"` // SHA256 hash of parameters
	ClientIP     string    `json:"client_ip,omitempty"`   // Client IP from context
	RequestID    string    `json:"request_id,omitempty"`  // Request ID from context
	Success      bool      `json:"success"`               // Whether operation succeeded
	Error        string    `json:"error,omitempty"`       // Error message if failed
	Duration     int64     `json:"duration_ms,omitempty"` // Query execution time in milliseconds
}

// Auditor handles audit logging of database operations.
type Auditor struct {
	logger *slog.Logger
	level  AuditLevel
}

// NewAuditor creates a new audit logger.
func NewAuditor(logger *slog.Logger, level AuditLevel) *Auditor {
	return &Auditor{
		logger: logger,
		level:  level,
	}
}

// LogOperation logs a database operation to the audit log.
func (a *Auditor) LogOperation(ctx context.Context, operation, query string, args []interface{}, result sql.Result, err error, duration time.Duration) {
	// Check if this operation should be logged based on audit level
	if !a.shouldLog(operation) {
		return
	}

	event := AuditEvent{
		Timestamp: time.Now().UTC(),
		Operation: operation,
		SQL:       query,
		Success:   err == nil,
		Duration:  duration.Milliseconds(),
	}

	// Extract context values
	if user, ok := ctx.Value(userKey).(string); ok {
		event.User = user
	}
	if clientIP, ok := ctx.Value(clientIPKey).(string); ok {
		event.ClientIP = clientIP
	}
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		event.RequestID = requestID
	}

	// Hash parameters for privacy (don't log actual values)
	if len(args) > 0 {
		event.ParamsHash = hashParams(args)
	}

	// Get affected rows if available
	if result != nil && err == nil {
		if rows, rowErr := result.RowsAffected(); rowErr == nil {
			event.AffectedRows = rows
		}
	}

	// Add error if present
	if err != nil {
		event.Error = err.Error()
	}

	// Attempt to extract table name from query (basic heuristic)
	event.Table = extractTableName(query)

	// Log the event
	a.logEvent(event)
}

// LogSecurityEvent logs a security-related event (blocked query, validation failure, etc.).
func (a *Auditor) LogSecurityEvent(ctx context.Context, eventType, query string, err error) {
	if a.logger == nil {
		return
	}

	event := AuditEvent{
		Timestamp: time.Now().UTC(),
		Operation: eventType, // e.g., "query_blocked", "params_blocked", "whitelist_violation"
		SQL:       query,
		Success:   false,
		Error:     err.Error(),
	}

	// Extract context values
	if user, ok := ctx.Value(userKey).(string); ok {
		event.User = user
	}
	if clientIP, ok := ctx.Value(clientIPKey).(string); ok {
		event.ClientIP = clientIP
	}
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		event.RequestID = requestID
	}

	// Log as security event
	a.logger.Warn("security_event",
		"event_type", eventType,
		"timestamp", event.Timestamp,
		"user", event.User,
		"client_ip", event.ClientIP,
		"request_id", event.RequestID,
		"query", query,
		"error", err.Error(),
	)
}

// shouldLog determines if an operation should be logged based on audit level.
func (a *Auditor) shouldLog(operation string) bool {
	if a.logger == nil || a.level == AuditNone {
		return false
	}

	switch a.level {
	case AuditWrites:
		// Log only write operations
		return operation == "INSERT" || operation == "UPDATE" || operation == "DELETE" || operation == "UPSERT"
	case AuditReads:
		// Log reads and writes
		return true
	case AuditAll:
		// Log everything
		return true
	default:
		return false
	}
}

// logEvent writes the audit event to the logger.
func (a *Auditor) logEvent(event AuditEvent) {
	if a.logger == nil {
		return
	}

	// Use Info level for successful operations, Warn for failures
	logFunc := a.logger.Info
	if !event.Success {
		logFunc = a.logger.Warn
	}

	logFunc("audit_event",
		"timestamp", event.Timestamp,
		"user", event.User,
		"operation", event.Operation,
		"table", event.Table,
		"affected_rows", event.AffectedRows,
		"sql", event.SQL,
		"params_hash", event.ParamsHash,
		"client_ip", event.ClientIP,
		"request_id", event.RequestID,
		"success", event.Success,
		"error", event.Error,
		"duration_ms", event.Duration,
	)
}

// hashParams creates a SHA256 hash of parameters for audit trail.
// This allows tracking which parameters were used without logging sensitive data.
func hashParams(params []interface{}) string {
	if len(params) == 0 {
		return ""
	}

	h := sha256.New()
	for _, param := range params {
		_, _ = fmt.Fprintf(h, "%v", param) // hash.Hash.Write never returns error
	}
	return hex.EncodeToString(h.Sum(nil))
}

// extractTableName attempts to extract the table name from a SQL query.
// This is a simple heuristic and may not work for complex queries.
func extractTableName(_ string) string {
	// TODO: Implement proper SQL parsing for table extraction
	// For now, return empty string - table extraction can be improved later
	return ""
}

// Context keys for audit metadata
type contextKey string

const (
	userKey      contextKey = "relica:user"
	clientIPKey  contextKey = "relica:client_ip"
	requestIDKey contextKey = "relica:request_id"
)

// WithUser adds user information to the context for audit logging.
func WithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// WithClientIP adds client IP to the context for audit logging.
func WithClientIP(ctx context.Context, clientIP string) context.Context {
	return context.WithValue(ctx, clientIPKey, clientIP)
}

// WithRequestID adds request ID to the context for audit logging.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetUser retrieves user from context (for testing/debugging).
func GetUser(ctx context.Context) string {
	user, _ := ctx.Value(userKey).(string)
	return user
}

// GetClientIP retrieves client IP from context (for testing/debugging).
func GetClientIP(ctx context.Context) string {
	clientIP, _ := ctx.Value(clientIPKey).(string)
	return clientIP
}

// GetRequestID retrieves request ID from context (for testing/debugging).
func GetRequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}
