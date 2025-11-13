# Security Guide

> **Relica Security Features** - SQL Injection Prevention & Audit Logging
>
> **Version**: v0.5.0-beta
> **Last Updated**: 2025-11-13

---

## 📋 Overview

Relica provides enterprise-grade security features for protecting your database operations:

- **SQL Injection Prevention**: Pattern-based detection of OWASP Top 10 SQL injection attacks
- **Audit Logging**: Comprehensive operation tracking for compliance and forensics
- **Context Tracking**: User, IP, and request ID propagation through database calls
- **Privacy-First**: Parameter hashing instead of logging raw values

All security features are **opt-in** with **zero overhead** when disabled.

---

## 🔒 SQL Injection Prevention

### Quick Start

```go
import (
    "github.com/coregx/relica"
    "github.com/coregx/relica/internal/security"
)

// Create validator
validator := security.NewValidator()

// Enable validation on DB connection
db, err := relica.NewDB("postgres", dsn,
    relica.WithValidator(validator),
)
if err != nil {
    return err
}
defer db.Close()

// All ExecContext and QueryContext calls are now validated
_, err = db.ExecContext(ctx, "SELECT * FROM users WHERE id = ?", userID)
// Malicious queries are blocked before execution
```

### Validation Modes

#### Standard Mode (Default)

Blocks dangerous patterns while allowing legitimate queries:

```go
validator := security.NewValidator()

// ✅ ALLOWED: Legitimate queries
db.ExecContext(ctx, "SELECT * FROM users WHERE status = ? OR role = ?", 1, 2)
db.ExecContext(ctx, "SELECT * FROM orders UNION SELECT * FROM archived_orders")

// ❌ BLOCKED: SQL injection attempts
db.ExecContext(ctx, "SELECT * FROM users; DROP TABLE users")              // Stacked queries
db.ExecContext(ctx, "SELECT * FROM users WHERE id = 1 OR 1=1")            // Tautology
db.ExecContext(ctx, "SELECT * FROM users -- comment")                     // Comment injection
db.ExecContext(ctx, "EXEC xp_cmdshell 'dir'")                            // Command execution
```

#### Strict Mode

Maximum security - blocks even legitimate OR/AND/UNION queries:

```go
validator := security.NewValidator(security.WithStrictMode())

db, err := relica.NewDB("postgres", dsn,
    relica.WithValidator(validator),
)

// ❌ BLOCKED in strict mode:
db.ExecContext(ctx, "SELECT * FROM users WHERE status = ? OR role = ?", 1, 2)
db.ExecContext(ctx, "SELECT * FROM orders UNION SELECT * FROM archived_orders")
```

**Use strict mode when:**
- Handling untrusted user input
- Processing external API requests
- Maximum security is required
- Your application doesn't need OR/AND/UNION clauses

### Attack Vectors Detected

Relica's validator detects all OWASP Top 10 SQL injection patterns:

| Attack Type | Example | Blocked |
|-------------|---------|---------|
| **Tautology** | `1 OR 1=1` | ✅ |
| **Comment Injection** | `admin'--` | ✅ |
| **Stacked Queries** | `; DROP TABLE` | ✅ |
| **UNION Attacks** | `UNION SELECT password` | ✅ |
| **Command Execution** | `xp_cmdshell`, `exec()` | ✅ |
| **Information Schema** | `information_schema.tables` | ✅ |
| **Timing Attacks** | `pg_sleep()`, `benchmark()` | ✅ |
| **Boolean Injection** | `AND 1=0` | ✅ |

Full pattern list: see `internal/security/validator.go`

### Performance Impact

Validation overhead is **< 2%** of total query execution time:

```
BenchmarkValidateQuery/safe_query     12.9 µs/op  (queries take ~10ms)
BenchmarkValidateQuery/malicious       812 ns/op
BenchmarkValidateParams                1.1 µs/op
```

### API Limitations

**QueryRowContext does NOT support validation** due to database/sql API constraints:

```go
// ❌ NOT VALIDATED (cannot return error):
row := db.QueryRowContext(ctx, maliciousQuery)

// ✅ VALIDATED (use this instead):
rows, err := db.QueryContext(ctx, query)
if err != nil {
    return err
}
defer rows.Close()
if rows.Next() {
    err = rows.Scan(&result)
}
```

**Recommendation**: Use QueryContext() or QueryBuilder for validated queries.

---

## 📊 Audit Logging

### Quick Start

```go
import (
    "log/slog"
    "github.com/coregx/relica"
    "github.com/coregx/relica/internal/security"
)

// Create logger
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))

// Create auditor with desired level
auditor := security.NewAuditor(logger, security.AuditWrites)

// Enable auditing on DB connection
db, err := relica.NewDB("postgres", dsn,
    relica.WithAuditLog(auditor),
)
if err != nil {
    return err
}
defer db.Close()

// All operations are now logged
_, err = db.ExecContext(ctx, "INSERT INTO users (name) VALUES (?)", "Alice")
```

### Audit Levels

Choose the appropriate level for your compliance requirements:

```go
// AuditNone - No logging (default, zero overhead)
auditor := security.NewAuditor(logger, security.AuditNone)

// AuditWrites - Log only write operations (INSERT, UPDATE, DELETE, UPSERT)
auditor := security.NewAuditor(logger, security.AuditWrites)

// AuditReads - Log reads AND writes (SELECT + write operations)
auditor := security.NewAuditor(logger, security.AuditReads)

// AuditAll - Log everything (including utility commands)
auditor := security.NewAuditor(logger, security.AuditAll)
```

**Recommendation for compliance:**
- **HIPAA**: AuditReads (track all data access)
- **PCI-DSS**: AuditReads (audit all cardholder data access)
- **GDPR**: AuditWrites (track data modifications)
- **SOC2**: AuditReads (comprehensive audit trail)

### Context Metadata

Track user, IP, and request ID through database operations:

```go
import "github.com/coregx/relica/internal/security"

// Add metadata to context
ctx := context.Background()
ctx = security.WithUser(ctx, "john.doe@example.com")
ctx = security.WithClientIP(ctx, "192.168.1.100")
ctx = security.WithRequestID(ctx, "req-12345")

// Execute query - metadata automatically logged
_, err := db.ExecContext(ctx, "UPDATE users SET status = ? WHERE id = ?", 2, 123)
```

**Typical web server integration:**

```go
func middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract metadata from request
        ctx := r.Context()
        ctx = security.WithUser(ctx, getUserFromToken(r))
        ctx = security.WithClientIP(ctx, r.RemoteAddr)
        ctx = security.WithRequestID(ctx, r.Header.Get("X-Request-ID"))

        // Pass enriched context to handlers
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Audit Log Format

Audit events are logged as structured JSON:

```json
{
  "level": "INFO",
  "msg": "audit_event",
  "timestamp": "2025-11-13T10:30:45.123Z",
  "user": "john.doe@example.com",
  "operation": "INSERT",
  "table": "users",
  "affected_rows": 1,
  "sql": "INSERT INTO users (name, email) VALUES (?, ?)",
  "params_hash": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
  "client_ip": "192.168.1.100",
  "request_id": "req-12345",
  "success": true,
  "error": "",
  "duration_ms": 15
}
```

**Key fields:**
- `timestamp`: UTC timestamp of operation
- `user`: User from context (empty if not set)
- `operation`: INSERT, UPDATE, DELETE, SELECT, etc.
- `table`: Target table (if detectable)
- `affected_rows`: Number of rows modified
- `sql`: Executed query
- `params_hash`: SHA256 hash of parameters (NOT raw values)
- `client_ip`: Client IP from context
- `request_id`: Request ID for distributed tracing
- `success`: true if operation succeeded, false if error
- `error`: Error message (if failed)
- `duration_ms`: Query execution time in milliseconds

### Parameter Hashing (Privacy)

**Relica NEVER logs raw parameter values** to protect sensitive data:

```go
// Query with sensitive data
db.ExecContext(ctx, "INSERT INTO users (email, password_hash) VALUES (?, ?)",
    "alice@example.com", "$2a$10$...")

// Audit log contains SHA256 hash, NOT raw values:
// "params_hash": "a3c5f12..."  ← Cannot be reversed
// "alice@example.com" does NOT appear in logs
```

This ensures compliance with GDPR/HIPAA/PCI-DSS data protection requirements.

### Security Event Logging

When validator blocks a malicious query, it's logged as a security event:

```go
// Attempt SQL injection
_, err := db.ExecContext(ctx, "SELECT * FROM users; DROP TABLE users")
// Error: "dangerous SQL pattern detected: query contains unsafe construct"
```

**Security event log:**

```json
{
  "level": "WARN",
  "msg": "security_event",
  "event_type": "query_blocked",
  "timestamp": "2025-11-13T10:35:12.456Z",
  "user": "attacker@evil.com",
  "client_ip": "10.0.0.666",
  "request_id": "req-67890",
  "query": "SELECT * FROM users; DROP TABLE users",
  "error": "dangerous SQL pattern detected: query contains unsafe construct"
}
```

**Security events are always logged at WARN level**, even if normal operations use INFO.

---

## 🛡️ Combined Security Setup

### Production-Ready Configuration

```go
package main

import (
    "context"
    "log/slog"
    "os"

    "github.com/coregx/relica"
    "github.com/coregx/relica/internal/security"
)

func setupSecureDB(dsn string) (*relica.DB, error) {
    // Create structured logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
        AddSource: true,
    }))

    // Create validator with strict mode for maximum security
    validator := security.NewValidator(security.WithStrictMode())

    // Create auditor with reads+writes logging for compliance
    auditor := security.NewAuditor(logger, security.AuditReads)

    // Create DB with all security features enabled
    db, err := relica.NewDB("postgres", dsn,
        relica.WithValidator(validator),
        relica.WithAuditLog(auditor),
        relica.WithMaxOpenConns(25),
        relica.WithMaxIdleConns(5),
        relica.WithConnMaxLifetime(300), // 5 minutes
    )
    if err != nil {
        return nil, err
    }

    return db, nil
}

func main() {
    db, err := setupSecureDB(os.Getenv("DATABASE_URL"))
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // Add context metadata
    ctx := context.Background()
    ctx = security.WithUser(ctx, "admin@example.com")
    ctx = security.WithClientIP(ctx, "192.168.1.1")
    ctx = security.WithRequestID(ctx, "req-001")

    // Execute secure query
    _, err = db.ExecContext(ctx, "INSERT INTO users (name) VALUES (?)", "Alice")
    if err != nil {
        // Malicious queries are blocked, security events logged
        panic(err)
    }

    // Query is validated, executed, and audited
}
```

---

## 📋 Compliance Checklist

### GDPR (General Data Protection Regulation)

- ✅ **Article 32 - Security of Processing**: SQL injection prevention
- ✅ **Article 32 - Audit Trail**: Comprehensive operation logging
- ✅ **Article 25 - Data Protection by Design**: Privacy-first parameter hashing
- ✅ **Article 33 - Breach Notification**: Security event logging for incident detection

### HIPAA (Health Insurance Portability and Accountability Act)

- ✅ **§164.308(a)(1)(ii)(D) - Access Controls**: User tracking via context
- ✅ **§164.312(b) - Audit Controls**: Operation logging with timestamps
- ✅ **§164.308(a)(5)(ii)(C) - Log Review**: Structured JSON logs for analysis
- ✅ **§164.312(a)(2)(i) - Unique User Identification**: User field in audit logs

### PCI-DSS (Payment Card Industry Data Security Standard)

- ✅ **Requirement 6.5 - SQL Injection Prevention**: Pattern-based validation
- ✅ **Requirement 10 - Audit Trail**: Comprehensive logging of data access
- ✅ **Requirement 10.2.5 - User Identification**: User, IP, request ID tracking
- ✅ **Requirement 10.3 - Audit Log Entries**: Timestamp, user, operation, success/failure

### SOC2 (Service Organization Control 2)

- ✅ **CC6.1 - Logical Access Controls**: SQL injection prevention
- ✅ **CC7.2 - System Monitoring**: Audit logging with security events
- ✅ **CC7.3 - Incident Response**: Security event detection and logging
- ✅ **CC8.1 - Change Management**: Audit trail of data modifications

---

## 🧪 Testing Security Features

### Unit Testing with Validator

```go
func TestSecureQuery(t *testing.T) {
    validator := security.NewValidator()
    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithValidator(validator),
    )
    defer db.Close()

    ctx := context.Background()

    // Test legitimate query
    _, err := db.ExecContext(ctx, "SELECT * FROM users WHERE id = ?", 123)
    if err != nil {
        t.Errorf("Legitimate query blocked: %v", err)
    }

    // Test SQL injection attempt
    _, err = db.ExecContext(ctx, "SELECT * FROM users; DROP TABLE users")
    if err == nil {
        t.Error("SQL injection not detected!")
    }
}
```

### Integration Testing with Auditor

```go
func TestAuditLogging(t *testing.T) {
    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    auditor := security.NewAuditor(logger, security.AuditWrites)
    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithAuditLog(auditor),
    )
    defer db.Close()

    // Create test table
    db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")

    // Execute audited operation
    ctx := security.WithUser(context.Background(), "test@example.com")
    _, err := db.ExecContext(ctx, "INSERT INTO users (id, name) VALUES (?, ?)", 1, "Alice")
    if err != nil {
        t.Fatal(err)
    }

    // Verify audit log
    logOutput := buf.String()
    if !strings.Contains(logOutput, "audit_event") {
        t.Error("Audit event not logged")
    }
    if !strings.Contains(logOutput, "test@example.com") {
        t.Error("User not in audit log")
    }
    if !strings.Contains(logOutput, "params_hash") {
        t.Error("Parameter hash missing")
    }
    if strings.Contains(logOutput, "Alice") {
        t.Error("Raw parameter value leaked in audit log!")
    }
}
```

---

## ⚠️ Best Practices

### DO:

✅ **Enable validation for user-facing applications**
```go
validator := security.NewValidator()
db, _ := relica.NewDB("postgres", dsn, relica.WithValidator(validator))
```

✅ **Use audit logging for compliance requirements**
```go
auditor := security.NewAuditor(logger, security.AuditReads)
db, _ := relica.NewDB("postgres", dsn, relica.WithAuditLog(auditor))
```

✅ **Add context metadata for forensics**
```go
ctx = security.WithUser(ctx, getUserFromSession(r))
ctx = security.WithClientIP(ctx, r.RemoteAddr)
ctx = security.WithRequestID(ctx, getRequestID(r))
```

✅ **Use QueryContext() instead of QueryRowContext() when validation is required**
```go
rows, err := db.QueryContext(ctx, query, args...)
```

✅ **Monitor security events in production**
```go
// Set up alerts for WARN-level security events
logger := slog.New(slog.NewJSONHandler(alertingHandler, &slog.HandlerOptions{
    Level: slog.LevelWarn,
}))
```

### DON'T:

❌ **Don't rely solely on validation** - use prepared statements (default in Relica)
```go
// Validation is defense-in-depth, not primary protection
// Prepared statements are your first line of defense
```

❌ **Don't disable validation based on "trusted" sources**
```go
// Even internal APIs can be compromised
// Always validate user-controlled input
```

❌ **Don't log audit trails to the same database**
```go
// Use external log aggregation (CloudWatch, Elasticsearch, etc.)
// Database compromise shouldn't destroy audit trail
```

❌ **Don't use AuditAll in high-throughput systems**
```go
// AuditWrites or AuditReads are sufficient for most compliance
// AuditAll adds overhead for utility commands (PRAGMA, SHOW, etc.)
```

---

## 📚 Additional Resources

- **OWASP SQL Injection**: https://owasp.org/www-community/attacks/SQL_Injection
- **NIST Database Security**: https://csrc.nist.gov/publications/detail/sp/800-123/final
- **CWE-89 (SQL Injection)**: https://cwe.mitre.org/data/definitions/89.html

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
