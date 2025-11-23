# Security Testing Guide

> **Testing Relica's Security Features** - Validator & Auditor Integration
>
> **Last Updated**: 2025-11-13

---

## 📋 Overview

This guide demonstrates how to test Relica's security features in your applications:

- **Validator Testing**: Verify SQL injection prevention
- **Auditor Testing**: Validate audit logging completeness
- **Integration Testing**: Test security in real-world scenarios
- **Penetration Testing**: OWASP-based attack simulation

---

## 🔒 Validator Testing

### Basic Validation Tests

```go
package security_test

import (
    "context"
    "testing"

    "github.com/coregx/relica"
    "github.com/coregx/relica/internal/security"
    _ "modernc.org/sqlite"
)

func TestSQLInjectionPrevention(t *testing.T) {
    // Setup
    validator := security.NewValidator()
    db, err := relica.NewDB("sqlite", ":memory:",
        relica.WithValidator(validator),
    )
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    // Create test table
    _, err = db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT, email TEXT)")
    if err != nil {
        t.Fatal(err)
    }

    ctx := context.Background()

    tests := []struct {
        name      string
        query     string
        args      []interface{}
        shouldFail bool
    }{
        {
            name:      "legitimate_select",
            query:     "SELECT * FROM users WHERE id = ?",
            args:      []interface{}{123},
            shouldFail: false,
        },
        {
            name:      "legitimate_insert",
            query:     "INSERT INTO users (id, name, email) VALUES (?, ?, ?)",
            args:      []interface{}{1, "Alice", "alice@example.com"},
            shouldFail: false,
        },
        {
            name:      "sql_injection_stacked_queries",
            query:     "SELECT * FROM users; DROP TABLE users",
            args:      nil,
            shouldFail: true,
        },
        {
            name:      "sql_injection_tautology",
            query:     "SELECT * FROM users WHERE id = 1 OR 1=1",
            args:      nil,
            shouldFail: true,
        },
        {
            name:      "sql_injection_comment",
            query:     "SELECT * FROM users WHERE name = 'admin' --",
            args:      nil,
            shouldFail: true,
        },
        {
            name:      "sql_injection_union",
            query:     "SELECT name FROM users UNION SELECT password FROM admin",
            args:      nil,
            shouldFail: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := db.ExecContext(ctx, tt.query, tt.args...)

            if tt.shouldFail && err == nil {
                t.Errorf("Expected query to be blocked but it succeeded: %s", tt.query)
            }

            if !tt.shouldFail && err != nil {
                t.Errorf("Expected query to succeed but it failed: %s, error: %v", tt.query, err)
            }
        })
    }
}
```

### OWASP Top 10 Attack Simulation

```go
func TestOWASP_SQLInjection_Attacks(t *testing.T) {
    validator := security.NewValidator()
    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithValidator(validator),
    )
    defer db.Close()

    ctx := context.Background()

    // OWASP A03:2021 – Injection
    owaspAttacks := []struct {
        category string
        attack   string
    }{
        // Tautology-based attacks
        {"A03_Tautology", "SELECT * FROM users WHERE username = 'admin' OR '1'='1'"},
        {"A03_Tautology", "SELECT * FROM products WHERE id = 1 OR 1=1"},
        {"A03_Tautology", "SELECT * FROM orders WHERE user_id = 1 AND 1=1"},

        // Comment-based attacks
        {"A03_Comments", "SELECT * FROM users WHERE username = 'admin' --"},
        {"A03_Comments", "SELECT * FROM users WHERE id = 1 /* injected */"},
        {"A03_Comments", "SELECT * FROM users WHERE name = 'test' #"},

        // Union-based attacks
        {"A03_Union", "SELECT name FROM users UNION SELECT password FROM admin"},
        {"A03_Union", "SELECT id, name FROM users UNION ALL SELECT credit_card, cvv FROM payments"},

        // Stacked queries
        {"A03_Stacked", "SELECT * FROM users; DROP TABLE users"},
        {"A03_Stacked", "SELECT * FROM users; DELETE FROM admin"},
        {"A03_Stacked", "SELECT * FROM users; UPDATE users SET role = 'admin'"},

        // Command execution (database-specific)
        {"A03_Execution", "EXEC xp_cmdshell 'dir'"},
        {"A03_Execution", "SELECT * FROM users; EXEC sp_executesql @cmd"},

        // Information schema attacks
        {"A03_InfoSchema", "SELECT table_name FROM information_schema.tables"},
        {"A03_InfoSchema", "SELECT column_name FROM information_schema.columns WHERE table_name = 'users'"},

        // Timing attacks
        {"A03_Timing", "SELECT * FROM users WHERE id = 1 AND pg_sleep(10)"},
        {"A03_Timing", "SELECT * FROM users WHERE id = 1 AND benchmark(1000000, md5('test'))"},
        {"A03_Timing", "SELECT * FROM users WHERE id = 1; WAITFOR DELAY '00:00:10'"},
    }

    for _, attack := range owaspAttacks {
        t.Run(attack.category, func(t *testing.T) {
            _, err := db.ExecContext(ctx, attack.attack)
            if err == nil {
                t.Errorf("OWASP attack not detected: %s - %s", attack.category, attack.attack)
            }
        })
    }
}
```

### Strict Mode Testing

```go
func TestStrictMode(t *testing.T) {
    // Standard mode allows OR/AND/UNION in legitimate queries
    t.Run("standard_mode", func(t *testing.T) {
        validator := security.NewValidator() // Standard mode
        db, _ := relica.NewDB("sqlite", ":memory:",
            relica.WithValidator(validator),
        )
        defer db.Close()

        ctx := context.Background()

        // These should PASS in standard mode
        legitimateQueries := []string{
            "SELECT * FROM users WHERE status = ? OR role = ?",
            "SELECT * FROM orders WHERE date > ? AND status = ?",
            "SELECT * FROM current_orders UNION SELECT * FROM archived_orders",
        }

        for _, query := range legitimateQueries {
            _, err := db.ExecContext(ctx, query, 1, 2)
            if err != nil {
                t.Errorf("Legitimate query blocked in standard mode: %s", query)
            }
        }
    })

    // Strict mode blocks OR/AND/UNION even in legitimate queries
    t.Run("strict_mode", func(t *testing.T) {
        validator := security.NewValidator(security.WithStrictMode())
        db, _ := relica.NewDB("sqlite", ":memory:",
            relica.WithValidator(validator),
        )
        defer db.Close()

        ctx := context.Background()

        // These should FAIL in strict mode
        blockedQueries := []string{
            "SELECT * FROM users WHERE status = ? OR role = ?",
            "SELECT * FROM orders WHERE date > ? AND status = ?",
            "SELECT * FROM current_orders UNION SELECT * FROM archived_orders",
        }

        for _, query := range blockedQueries {
            _, err := db.ExecContext(ctx, query, 1, 2)
            if err == nil {
                t.Errorf("Query not blocked in strict mode: %s", query)
            }
        }
    })
}
```

### Parameter Injection Testing

```go
func TestParameterInjection(t *testing.T) {
    validator := security.NewValidator()
    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithValidator(validator),
    )
    defer db.Close()

    // Create test table
    db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")

    ctx := context.Background()

    // Test parameter injection attempts
    maliciousParams := []struct {
        name  string
        value interface{}
    }{
        {"sql_comment", "admin'--"},
        {"boolean_injection", "1 OR 1=1"},
        {"stacked_query", "1; DROP TABLE users"},
        {"union_injection", "1 UNION SELECT password FROM admin"},
    }

    for _, param := range maliciousParams {
        t.Run(param.name, func(t *testing.T) {
            _, err := db.ExecContext(ctx,
                "INSERT INTO users (id, name) VALUES (?, ?)",
                1, param.value,
            )

            if err == nil {
                t.Logf("NOTE: Parameter '%v' was not blocked (prepared statements provide protection)", param.value)
                // This is OK - prepared statements prevent injection even if validator doesn't block
            }
        })
    }
}
```

---

## 📊 Auditor Testing

### Basic Audit Logging Tests

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

    ctx := context.Background()
    ctx = security.WithUser(ctx, "test@example.com")
    ctx = security.WithClientIP(ctx, "192.168.1.100")
    ctx = security.WithRequestID(ctx, "req-12345")

    // Execute audited operation
    _, err := db.ExecContext(ctx, "INSERT INTO users (id, name) VALUES (?, ?)", 1, "Alice")
    if err != nil {
        t.Fatal(err)
    }

    logOutput := buf.String()

    // Verify audit log contains required fields
    requiredFields := []string{
        "audit_event",
        "INSERT",
        "test@example.com",
        "192.168.1.100",
        "req-12345",
        "params_hash",
    }

    for _, field := range requiredFields {
        if !strings.Contains(logOutput, field) {
            t.Errorf("Audit log missing required field: %s", field)
        }
    }

    // Verify raw parameter values are NOT in log
    if strings.Contains(logOutput, "Alice") {
        t.Error("Audit log contains raw parameter value (should be hashed)")
    }
}
```

### Audit Level Testing

```go
func TestAuditLevels(t *testing.T) {
    tests := []struct {
        name       string
        level      security.AuditLevel
        operation  string
        query      string
        shouldLog  bool
    }{
        {
            name:      "writes_logs_insert",
            level:     security.AuditWrites,
            operation: "INSERT",
            query:     "INSERT INTO users (id) VALUES (?)",
            shouldLog: true,
        },
        {
            name:      "writes_skips_select",
            level:     security.AuditWrites,
            operation: "SELECT",
            query:     "SELECT * FROM users",
            shouldLog: false,
        },
        {
            name:      "reads_logs_select",
            level:     security.AuditReads,
            operation: "SELECT",
            query:     "SELECT * FROM users",
            shouldLog: true,
        },
        {
            name:      "none_skips_everything",
            level:     security.AuditNone,
            operation: "INSERT",
            query:     "INSERT INTO users (id) VALUES (?)",
            shouldLog: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var buf bytes.Buffer
            logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
                Level: slog.LevelInfo,
            }))

            auditor := security.NewAuditor(logger, tt.level)
            db, _ := relica.NewDB("sqlite", ":memory:",
                relica.WithAuditLog(auditor),
            )
            defer db.Close()

            db.sqlDB.Exec("CREATE TABLE users (id INTEGER)")

            ctx := context.Background()

            if tt.operation == "SELECT" {
                db.QueryContext(ctx, tt.query)
            } else {
                db.ExecContext(ctx, tt.query, 1)
            }

            logOutput := buf.String()
            hasLog := strings.Contains(logOutput, tt.operation)

            if tt.shouldLog && !hasLog {
                t.Errorf("Expected operation to be logged but wasn't: %s", tt.operation)
            }
            if !tt.shouldLog && hasLog {
                t.Errorf("Expected operation to be skipped but was logged: %s", tt.operation)
            }
        })
    }
}
```

### Security Event Logging

```go
func TestSecurityEventLogging(t *testing.T) {
    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
        Level: slog.LevelWarn,
    }))

    validator := security.NewValidator()
    auditor := security.NewAuditor(logger, security.AuditAll)

    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithValidator(validator),
        relica.WithAuditLog(auditor),
    )
    defer db.Close()

    ctx := context.Background()
    ctx = security.WithUser(ctx, "attacker@evil.com")
    ctx = security.WithClientIP(ctx, "10.0.0.666")
    ctx = security.WithRequestID(ctx, "suspicious-request-123")

    // Attempt SQL injection (should be blocked and logged)
    _, err := db.ExecContext(ctx, "SELECT * FROM users; DROP TABLE users")
    if err == nil {
        t.Fatal("Expected SQL injection to be blocked")
    }

    logOutput := buf.String()

    // Verify security event is logged
    requiredFields := []string{
        "security_event",
        "query_blocked",
        "attacker@evil.com",
        "10.0.0.666",
        "suspicious-request-123",
    }

    for _, field := range requiredFields {
        if !strings.Contains(logOutput, field) {
            t.Errorf("Security event log missing field: %s", field)
        }
    }

    // Verify log level is WARN
    if !strings.Contains(logOutput, "WARN") && !strings.Contains(logOutput, "warn") {
        t.Error("Security event not logged at WARN level")
    }
}
```

### Failed Operation Logging

```go
func TestFailedOperationLogging(t *testing.T) {
    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
        Level: slog.LevelWarn,
    }))

    auditor := security.NewAuditor(logger, security.AuditWrites)
    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithAuditLog(auditor),
    )
    defer db.Close()

    ctx := context.Background()

    // Execute query on non-existent table (will fail)
    _, err := db.ExecContext(ctx, "INSERT INTO nonexistent (id) VALUES (?)", 1)
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

    // Verify logged at WARN level (failures)
    if !strings.Contains(logOutput, "WARN") && !strings.Contains(logOutput, "warn") {
        t.Error("Failed operation not logged at WARN level")
    }
}
```

---

## 🛡️ Integration Testing

### Web Server Integration

```go
func TestWebServerSecurity(t *testing.T) {
    // Setup secure DB
    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    validator := security.NewValidator()
    auditor := security.NewAuditor(logger, security.AuditReads)

    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithValidator(validator),
        relica.WithAuditLog(auditor),
    )
    defer db.Close()

    db.sqlDB.Exec("CREATE TABLE users (id INTEGER, name TEXT)")

    // Simulate HTTP handler
    handler := func(w http.ResponseWriter, r *http.Request) {
        // Extract metadata from request
        ctx := r.Context()
        ctx = security.WithUser(ctx, getUserFromToken(r))
        ctx = security.WithClientIP(ctx, r.RemoteAddr)
        ctx = security.WithRequestID(ctx, r.Header.Get("X-Request-ID"))

        // Get user input (potentially malicious)
        userInput := r.URL.Query().Get("id")

        // Execute query (validator protects against injection)
        _, err := db.ExecContext(ctx,
            "SELECT * FROM users WHERE id = ?",
            userInput, // Could be "1; DROP TABLE users"
        )

        if err != nil {
            w.WriteHeader(http.StatusBadRequest)
            w.Write([]byte("Invalid request"))
            return
        }

        w.WriteHeader(http.StatusOK)
    }

    // Test malicious request
    req := httptest.NewRequest("GET", "/users?id=1%3B+DROP+TABLE+users", nil)
    req.Header.Set("X-Request-ID", "test-123")
    w := httptest.NewRecorder()

    handler(w, req)

    // Verify response (should be 400)
    if w.Code != http.StatusBadRequest {
        t.Errorf("Expected status 400, got %d", w.Code)
    }

    // Verify security event logged
    logOutput := buf.String()
    if !strings.Contains(logOutput, "security_event") {
        t.Error("Security event not logged for malicious request")
    }
}

func getUserFromToken(r *http.Request) string {
    // Mock implementation
    return "test@example.com"
}
```

### Transaction Security

```go
func TestTransactionSecurity(t *testing.T) {
    validator := security.NewValidator()
    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    auditor := security.NewAuditor(logger, security.AuditWrites)

    db, _ := relica.NewDB("sqlite", ":memory:",
        relica.WithValidator(validator),
        relica.WithAuditLog(auditor),
    )
    defer db.Close()

    db.sqlDB.Exec("CREATE TABLE accounts (id INTEGER, balance INTEGER)")
    db.sqlDB.Exec("INSERT INTO accounts (id, balance) VALUES (1, 1000)")

    ctx := security.WithUser(context.Background(), "attacker@example.com")

    // Start transaction
    tx, _ := db.Begin(ctx)

    // Attempt SQL injection in transaction
    _, err := tx.ExecContext(ctx,
        "UPDATE accounts SET balance = balance - ?; DROP TABLE accounts",
        100,
    )

    if err == nil {
        t.Fatal("SQL injection not blocked in transaction")
    }

    // Rollback (transaction should be clean)
    tx.Rollback()

    // Verify table still exists
    var count int
    db.sqlDB.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count)
    if count != 1 {
        t.Error("Transaction security failed - table was compromised")
    }

    // Verify security event logged
    logOutput := buf.String()
    if !strings.Contains(logOutput, "security_event") {
        t.Error("Security event not logged for transaction attack")
    }
}
```

---

## 🎯 Performance Testing

### Validation Overhead Benchmark

```go
func BenchmarkValidationOverhead(b *testing.B) {
    // Baseline: No validation
    b.Run("no_validation", func(b *testing.B) {
        db, _ := relica.NewDB("sqlite", ":memory:")
        defer db.Close()

        db.sqlDB.Exec("CREATE TABLE users (id INTEGER)")
        ctx := context.Background()

        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            db.ExecContext(ctx, "INSERT INTO users (id) VALUES (?)", i)
        }
    })

    // With validation
    b.Run("with_validation", func(b *testing.B) {
        validator := security.NewValidator()
        db, _ := relica.NewDB("sqlite", ":memory:",
            relica.WithValidator(validator),
        )
        defer db.Close()

        db.sqlDB.Exec("CREATE TABLE users (id INTEGER)")
        ctx := context.Background()

        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            db.ExecContext(ctx, "INSERT INTO users (id) VALUES (?)", i)
        }
    })
}
```

### Audit Logging Overhead

```go
func BenchmarkAuditOverhead(b *testing.B) {
    // Baseline: No auditing
    b.Run("no_auditing", func(b *testing.B) {
        db, _ := relica.NewDB("sqlite", ":memory:")
        defer db.Close()

        db.sqlDB.Exec("CREATE TABLE users (id INTEGER)")
        ctx := context.Background()

        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            db.ExecContext(ctx, "INSERT INTO users (id) VALUES (?)", i)
        }
    })

    // With auditing
    b.Run("with_auditing", func(b *testing.B) {
        logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
            Level: slog.LevelInfo,
        }))
        auditor := security.NewAuditor(logger, security.AuditWrites)

        db, _ := relica.NewDB("sqlite", ":memory:",
            relica.WithAuditLog(auditor),
        )
        defer db.Close()

        db.sqlDB.Exec("CREATE TABLE users (id INTEGER)")
        ctx := security.WithUser(context.Background(), "test@example.com")

        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            db.ExecContext(ctx, "INSERT INTO users (id) VALUES (?)", i)
        }
    })
}
```

---

## 📚 Additional Resources

- **OWASP Testing Guide**: https://owasp.org/www-project-web-security-testing-guide/
- **SQL Injection Prevention Cheat Sheet**: https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html
- **Go Security Checklist**: https://github.com/guardrailsio/awesome-golang-security

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
