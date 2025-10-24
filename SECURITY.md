# Security Policy

## Supported Versions

Relica is currently in beta. We provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.1.0-beta | :white_check_mark: |
| < 0.1.0 | :x:                |

After 1.0.0 release, we will support:
- Latest major version
- Previous major version (security fixes only)

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in Relica, please report it responsibly.

### How to Report

**DO NOT** open a public GitHub issue for security vulnerabilities.

Instead, please report security issues by emailing:

**security@coregx.dev**

Or open a **private security advisory** on GitHub:
https://github.com/coregx/relica/security/advisories/new

### What to Include

Please include the following information in your report:

- **Description** of the vulnerability
- **Steps to reproduce** the issue
- **Affected versions** (which versions are impacted)
- **Potential impact** (what can an attacker do?)
- **Proof of concept** (code sample, if possible)
- **Suggested fix** (if you have one)
- **Your contact information** (for follow-up questions)

### Response Timeline

- **Initial Response**: Within 48 hours
- **Triage & Assessment**: Within 1 week
- **Fix & Disclosure**: Coordinated with reporter

We aim to:
1. Acknowledge receipt within 48 hours
2. Provide an initial assessment within 1 week
3. Work with you on a coordinated disclosure timeline
4. Credit you in the security advisory (unless you prefer to remain anonymous)
5. Release a patch as soon as possible

## Security Best Practices

When using Relica in your application:

### 1. SQL Injection Prevention

Relica uses prepared statements by default, but you must use them correctly:

```go
// ✅ GOOD - Parameterized queries (safe)
db.Builder().
    Select().
    From("users").
    Where("email = ?", userEmail).  // Safe - parameterized
    One(&user)

// ❌ BAD - String concatenation (VULNERABLE!)
query := "SELECT * FROM users WHERE email = '" + userEmail + "'"
db.DB().Query(query)  // UNSAFE!

// ❌ BAD - Direct string interpolation
db.Builder().
    Select().
    From("users").
    Where(fmt.Sprintf("email = '%s'", userEmail)).  // VULNERABLE!
    One(&user)
```

**Golden Rule**: Always use `?` placeholders, never string concatenation!

### 2. Connection String Security

**Never** hardcode credentials:

```go
// ❌ BAD - Hardcoded credentials
dsn := "postgres://user:password@localhost/db"

// ✅ GOOD - Environment variables
dsn := os.Getenv("DATABASE_URL")
if dsn == "" {
    log.Fatal("DATABASE_URL not set")
}

// ✅ BETTER - Secrets management
dsn := secretsManager.GetSecret("db/connection-string")
```

### 3. Transaction Isolation

Choose appropriate isolation level for your security requirements:

```go
// For financial operations, use Serializable
tx, err := db.BeginTx(ctx, &relica.TxOptions{
    Isolation: sql.LevelSerializable,  // Strongest isolation
})

// Default (ReadCommitted) may allow race conditions
tx, err := db.BeginTx(ctx, nil)  // Be aware of implications
```

### 4. Context Timeouts

Always set timeouts to prevent resource exhaustion:

```go
// ✅ GOOD - With timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

db.Builder().
    WithContext(ctx).
    Select().
    From("users").
    All(&users)

// ❌ BAD - No timeout
db.Builder().
    Select().
    From("large_table").  // Could hang forever
    All(&data)
```

### 5. Error Handling

Don't leak sensitive information in errors:

```go
// ❌ BAD - Exposes SQL in error messages
if err != nil {
    return fmt.Errorf("query failed: %w", err)  // May leak SQL
}

// ✅ GOOD - Generic error for users
if err != nil {
    log.Error("DB error", "error", err)  // Log details
    return errors.New("database operation failed")  // Generic to user
}
```

### 6. Statement Cache

Be aware of statement cache implications:

```go
// Statement cache is bounded (default 1000)
// But can still consume memory with many unique queries

// ✅ GOOD - Reuse query patterns
for _, id := range ids {
    db.Builder().Select().From("users").Where("id = ?", id).One(&user)
    // Same SQL pattern, cache reused
}

// ⚠️ WARNING - Cache pollution
for _, table := range dynamicTables {
    db.Builder().Select().From(table).All(&data)
    // Different SQL each time, fills cache
}
```

## Known Security Considerations

### 1. SQL Injection via Dynamic Table/Column Names

Relica does **not** parameterize table/column names (this is a database limitation):

```go
// ⚠️ DANGEROUS - User input as table name
tableName := getUserInput()
db.Builder().Select().From(tableName).All(&data)  // Vulnerable!

// ✅ MITIGATION - Whitelist table names
allowedTables := map[string]bool{"users": true, "posts": true}
if !allowedTables[tableName] {
    return errors.New("invalid table")
}
db.Builder().Select().From(tableName).All(&data)  // Safe
```

**Recommendation**: Never use user input directly for table/column names. Use a whitelist.

### 2. Connection Pool Exhaustion

Without proper timeouts, connections can be exhausted:

```go
// ⚠️ RISK - No limits
db, err := relica.Open("postgres", dsn)  // Defaults may not fit your needs

// ✅ MITIGATION - Configure pool
db, err := relica.Open("postgres", dsn,
    relica.WithMaxOpenConns(25),      // Limit total connections
    relica.WithMaxIdleConns(5),       // Limit idle connections
)
```

**Recommendation**: Configure connection pool based on your application's needs.

### 3. Transaction Deadlocks

Improper transaction handling can cause deadlocks:

```go
// ⚠️ RISK - Long-running transaction
tx, _ := db.BeginTx(ctx, nil)
// ... many operations ...
// ... sleep or external API call ...
tx.Commit()  // May cause deadlocks

// ✅ MITIGATION - Keep transactions short
tx, _ := db.BeginTx(ctx, nil)
defer tx.Rollback()
// Only DB operations here
tx.Commit()
```

**Recommendation**: Keep transactions short and focused.

### 4. Denial of Service via Large Queries

Unbounded queries can exhaust resources:

```go
// ⚠️ RISK - Unbounded query
db.Builder().Select().From("huge_table").All(&data)  // May load millions of rows

// ✅ MITIGATION - Pagination
db.Builder().
    Select().
    From("huge_table").
    Where("id > ?", lastID).
    Limit(100).  // Process in batches
    All(&data)
```

**Recommendation**: Always use LIMIT for large datasets.

### 5. Dependency Security

Relica has **zero production dependencies** (only Go stdlib).

Test dependencies:
- `github.com/stretchr/testify` - Testing assertions
- `modernc.org/sqlite` - Pure Go SQLite (tests only)

**Monitoring**:
- Dependabot enabled
- Weekly dependency audit
- No transitive dependencies in production

### 6. Driver Security

Relica requires external database drivers at runtime:

```go
import _ "github.com/lib/pq"  // PostgreSQL
import _ "github.com/go-sql-driver/mysql"  // MySQL
```

**Important**: Keep these drivers updated! Relica doesn't bundle them.

## Security Disclosure History

No security vulnerabilities have been reported or fixed yet (project is in beta).

When vulnerabilities are addressed, they will be listed here with:
- **CVE ID** (if assigned)
- **Affected versions**
- **Fixed in version**
- **Severity** (Critical/High/Medium/Low)
- **Credit** to reporter

## Security Audit Status

- **Last Audit**: Not yet audited
- **Planned**: After 1.0.0 release
- **Scope**: SQL injection, statement caching, connection pooling

## Security Contact

- **Email**: security@coregx.dev
- **GitHub**: https://github.com/coregx/relica/security
- **Response Time**: Within 48 hours

## Bug Bounty Program

Relica does not currently have a bug bounty program. We rely on responsible disclosure from the security community.

If you report a valid security vulnerability:
- ✅ Public credit in security advisory (if desired)
- ✅ Acknowledgment in CHANGELOG
- ✅ Listed as security contributor
- ✅ Our gratitude 🙏

## Compliance

Relica is designed to be used in compliance with:
- **OWASP Top 10** - Prevents SQL injection (#3)
- **CWE-89** - Parameterized queries prevent SQL injection
- **CWE-400** - Connection pool limits prevent resource exhaustion
- **CWE-209** - Careful error handling prevents information disclosure

## Recommended Security Tools

When using Relica, consider:

1. **Static Analysis**: `gosec`, `golangci-lint`
2. **Dependency Scanning**: `govulncheck`
3. **Database Auditing**: Enable query logging in production
4. **Connection Encryption**: Use SSL/TLS for database connections
5. **Secrets Management**: Use vault/secret manager for credentials

---

**Thank you for helping keep Relica secure!** 🔒

*If you have security questions that are not vulnerabilities, feel free to open a GitHub Discussion.*
