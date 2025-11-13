# Relica

[![CI](https://github.com/coregx/relica/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/coregx/relica/actions/workflows/test.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/coregx/relica)](https://goreportcard.com/report/github.com/coregx/relica)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/coregx/relica?include_prereleases&style=flat)](https://github.com/coregx/relica/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/coregx/relica.svg)](https://pkg.go.dev/github.com/coregx/relica)

**Relica** is a lightweight, type-safe database query builder for Go with zero production dependencies.

## ✨ Features

- 🚀 **Zero Production Dependencies** - Uses only Go standard library
- ⚡ **High Performance** - LRU statement cache, batch operations (3.3x faster)
- 🎯 **Type-Safe** - Reflection-based struct scanning with compile-time checks
- 🔒 **Transaction Support** - Full ACID with all isolation levels
- 🛡️ **Enterprise Security** - SQL injection prevention, audit logging, compliance (v0.5.0+)
- 📦 **Batch Operations** - Efficient multi-row INSERT and UPDATE
- 🔗 **JOIN Operations** - INNER, LEFT, RIGHT, FULL, CROSS JOIN support (v0.2.0+)
- 📊 **Sorting & Pagination** - ORDER BY, LIMIT, OFFSET (v0.2.0+)
- 🔢 **Aggregate Functions** - COUNT, SUM, AVG, MIN, MAX, GROUP BY, HAVING (v0.2.0+)
- 🔍 **Subqueries** - IN, EXISTS, FROM subqueries, scalar subqueries (v0.3.0+)
- 🔀 **Set Operations** - UNION, UNION ALL, INTERSECT, EXCEPT (v0.3.0+)
- 🌳 **Common Table Expressions** - WITH clause, recursive CTEs (v0.3.0+)
- 🌐 **Multi-Database** - PostgreSQL, MySQL 8.0+, SQLite 3.25+ support
- 🧪 **Well-Tested** - 326+ tests, 93.3% coverage
- 📝 **Clean API** - Fluent builder pattern with context support

> **Latest Release:** See [CHANGELOG.md](CHANGELOG.md) for version history and [GitHub Releases](https://github.com/coregx/relica/releases) for release notes.

## 🚀 Quick Start

### Installation

```bash
go get github.com/coregx/relica
```

> **Note**: Always import only the main `relica` package. Internal packages are protected and not part of the public API.

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/coregx/relica"
    _ "github.com/lib/pq" // PostgreSQL driver
)

type User struct {
    ID    int    `db:"id"`
    Name  string `db:"name"`
    Email string `db:"email"`
}

func main() {
    // Connect to database
    db, err := relica.Open("postgres", "postgres://user:pass@localhost/db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    ctx := context.Background()

    // SELECT - Query single row (v0.4.1+ convenience method)
    var user User
    err = db.Select("*").
        From("users").
        Where("id = ?", 1).
        WithContext(ctx).
        One(&user)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("User: %+v\n", user)

    // SELECT - Query multiple rows (convenience method)
    var users []User
    err = db.Select("*").
        From("users").
        Where("age > ?", 18).
        All(&users)

    // INSERT (convenience method)
    result, err := db.Insert("users", map[string]interface{}{
        "name":  "Alice",
        "email": "alice@example.com",
    }).Execute()

    // UPDATE (convenience method)
    result, err = db.Update("users").
        Set(map[string]interface{}{
            "name": "Alice Updated",
        }).
        Where("id = ?", 1).
        Execute()

    // DELETE (convenience method)
    result, err = db.Delete("users").
        Where("id = ?", 1).
        Execute()

    // For advanced queries (CTEs, UNION, etc.), use Builder()
    err = db.Builder().
        With("stats", statsQuery).
        Select("*").
        From("stats").
        All(&results)
}
```

## 📚 Core Features

### CRUD Operations

**New in v0.4.1**: Convenience methods for shorter, more intuitive code!

```go
// SELECT (v0.4.1+ convenience method)
var user User
db.Select("*").From("users").Where("id = ?", 1).One(&user)

// SELECT with multiple conditions
var users []User
db.Select("id", "name", "email").
    From("users").
    Where("age > ?", 18).
    Where("status = ?", "active").
    All(&users)

// INSERT (convenience method)
db.Insert("users", map[string]interface{}{
    "name": "Bob",
    "email": "bob@example.com",
}).Execute()

// UPDATE (convenience method)
db.Update("users").
    Set(map[string]interface{}{"status": "inactive"}).
    Where("last_login < ?", time.Now().AddDate(0, -6, 0)).
    Execute()

// DELETE (convenience method)
db.Delete("users").Where("id = ?", 123).Execute()

// For advanced operations, use Builder()
db.Builder().
    Upsert("users", map[string]interface{}{
        "id":    1,
        "name":  "Alice",
        "email": "alice@example.com",
    }).
    OnConflict("id").
    DoUpdate("name", "email").
    Execute()

// Builder() is still fully supported for all operations
db.Builder().Select("*").From("users").All(&users)
```

### Expression API (v0.1.2+)

Relica supports fluent expression builders for type-safe, complex WHERE clauses:

#### HashExp - Simple Conditions

```go
// Simple equality
db.Builder().Select().From("users").
    Where(relica.HashExp{"status": 1}).
    All(&users)

// Multiple conditions (AND)
db.Builder().Select().From("users").
    Where(relica.HashExp{
        "status": 1,
        "age":    30,
    }).
    All(&users)

// IN clause (slice values)
db.Builder().Select().From("users").
    Where(relica.HashExp{
        "status": []interface{}{1, 2, 3},
    }).
    All(&users)

// NULL handling
db.Builder().Select().From("users").
    Where(relica.HashExp{
        "deleted_at": nil,  // IS NULL
    }).
    All(&users)

// Combined: IN + NULL + equality
db.Builder().Select().From("users").
    Where(relica.HashExp{
        "status":     []interface{}{1, 2},
        "deleted_at": nil,
        "role":       "admin",
    }).
    All(&users)
```

#### Comparison Operators

```go
// Greater than
db.Builder().Select().From("users").
    Where(relica.GreaterThan("age", 18)).
    All(&users)

// Less than or equal
db.Builder().Select().From("users").
    Where(relica.LessOrEqual("price", 100.0)).
    All(&products)

// Available: Eq, NotEq, GreaterThan, LessThan, GreaterOrEqual, LessOrEqual
```

#### IN and BETWEEN

```go
// IN
db.Builder().Select().From("users").
    Where(relica.In("role", "admin", "moderator")).
    All(&users)

// NOT IN
db.Builder().Select().From("users").
    Where(relica.NotIn("status", 0, 99)).
    All(&users)

// BETWEEN
db.Builder().Select().From("orders").
    Where(relica.Between("created_at", startDate, endDate)).
    All(&orders)
```

#### LIKE with Automatic Escaping

```go
// Default: %value% (partial match)
db.Builder().Select().From("users").
    Where(relica.Like("name", "john")).  // name LIKE '%john%'
    All(&users)

// Multiple values (AND)
db.Builder().Select().From("articles").
    Where(relica.Like("title", "go", "database")).  // title LIKE '%go%' AND title LIKE '%database%'
    All(&articles)

// Custom matching (prefix/suffix)
db.Builder().Select().From("files").
    Where(relica.Like("filename", ".txt").Match(false, true)).  // filename LIKE '%.txt'
    All(&files)

// OR logic
db.Builder().Select().From("users").
    Where(relica.OrLike("email", "gmail", "yahoo")).  // email LIKE '%gmail%' OR email LIKE '%yahoo%'
    All(&users)
```

#### Logical Combinators

```go
// AND
db.Builder().Select().From("users").
    Where(relica.And(
        relica.Eq("status", 1),
        relica.GreaterThan("age", 18),
    )).
    All(&users)

// OR
db.Builder().Select().From("users").
    Where(relica.Or(
        relica.Eq("role", "admin"),
        relica.Eq("role", "moderator"),
    )).
    All(&users)

// NOT
db.Builder().Select().From("users").
    Where(relica.Not(
        relica.In("status", 0, 99),
    )).
    All(&users)

// Nested combinations
db.Builder().Select().From("users").
    Where(relica.And(
        relica.Eq("status", 1),
        relica.Or(
            relica.Eq("role", "admin"),
            relica.GreaterThan("age", 30),
        ),
    )).
    All(&users)
```

#### Backward Compatibility

String-based WHERE still works:

```go
// Old style (still supported)
db.Builder().Select().From("users").
    Where("status = ? AND age > ?", 1, 18).
    All(&users)

// Can mix both styles
db.Builder().Select().From("users").
    Where("status = ?", 1).
    Where(relica.GreaterThan("age", 18)).
    All(&users)
```

### JOIN Operations (v0.2.0+)

**Solve N+1 query problems with JOIN support** - reduces 101 queries to 1 query (100x improvement).

```go
// Simple INNER JOIN
var results []struct {
    UserID   int    `db:"user_id"`
    UserName string `db:"user_name"`
    PostID   int    `db:"post_id"`
    Title    string `db:"title"`
}

db.Builder().
    Select("u.id as user_id", "u.name as user_name", "p.id as post_id", "p.title").
    From("users u").
    InnerJoin("posts p", "p.user_id = u.id").
    All(&results)

// Multiple JOINs with aggregates
db.Builder().
    Select("messages.*", "users.name", "COUNT(attachments.id) as attachment_count").
    From("messages m").
    InnerJoin("users u", "m.user_id = u.id").
    LeftJoin("attachments a", "m.id = a.message_id").
    Where("m.status = ?", 1).
    GroupBy("messages.id").
    All(&results)

// All JOIN types supported
db.Builder().InnerJoin(table, on)  // INNER JOIN
db.Builder().LeftJoin(table, on)   // LEFT OUTER JOIN
db.Builder().RightJoin(table, on)  // RIGHT OUTER JOIN
db.Builder().FullJoin(table, on)   // FULL OUTER JOIN (PostgreSQL, SQLite)
db.Builder().CrossJoin(table)      // CROSS JOIN (no ON condition)

// JOIN with Expression API
db.Builder().
    Select().
    From("messages m").
    InnerJoin("users u", relica.And(
        relica.Raw("m.user_id = u.id"),
        relica.GreaterThan("u.status", 0),
    )).
    All(&results)
```

**Performance**: 100x query reduction (N+1 problem solved), 6-25x faster depending on database.

See [JOIN Guide](docs/dev/reports/JOIN_GUIDE.md) for comprehensive examples and best practices.

### Sorting and Pagination (v0.2.0+)

**Database-side sorting and pagination** for efficient data retrieval - 100x memory reduction.

```go
// ORDER BY with multiple columns
db.Builder().
    Select().
    From("messages").
    OrderBy("created_at DESC", "id ASC").
    All(&messages)

// Pagination with LIMIT and OFFSET
const pageSize = 100
const pageNumber = 2 // Third page (0-indexed)

db.Builder().
    Select().
    From("users").
    OrderBy("age DESC").
    Limit(pageSize).
    Offset(pageNumber * pageSize).
    All(&users)

// Table column references
db.Builder().
    Select().
    From("messages m").
    InnerJoin("users u", "m.user_id = u.id").
    OrderBy("m.created_at DESC", "u.name ASC").
    Limit(50).
    All(&results)
```

**Performance**: 100x memory reduction (fetch only what you need vs all rows), 6x faster.

### Aggregate Functions (v0.2.0+)

**Database-side aggregations** for COUNT, SUM, AVG, MIN, MAX - 2,500,000x memory reduction.

```go
// Simple COUNT
var count struct{ Total int `db:"total"` }
db.Builder().
    Select("COUNT(*) as total").
    From("messages").
    One(&count)

// Multiple aggregates
type Stats struct {
    Count int     `db:"count"`
    Sum   int64   `db:"sum"`
    Avg   float64 `db:"avg"`
    Min   int     `db:"min"`
    Max   int     `db:"max"`
}

var stats Stats
db.Builder().
    Select("COUNT(*) as count", "SUM(size) as sum", "AVG(size) as avg", "MIN(size) as min", "MAX(size) as max").
    From("messages").
    One(&stats)

// GROUP BY with HAVING
type UserStats struct {
    UserID       int `db:"user_id"`
    MessageCount int `db:"message_count"`
}

var userStats []UserStats
db.Builder().
    Select("user_id", "COUNT(*) as message_count").
    From("messages").
    GroupBy("user_id").
    Having("COUNT(*) > ?", 100).
    OrderBy("message_count DESC").
    All(&userStats)
```

**Performance**: 2,500,000x memory reduction (database aggregation vs fetching all rows), 20x faster.

See [Aggregates Guide](docs/dev/reports/AGGREGATES_GUIDE.md) for comprehensive examples and patterns.

### Advanced SQL Features (v0.3.0+)

Relica v0.3.0 adds powerful SQL features for complex queries.

#### Subqueries

**IN/EXISTS Subqueries**:
```go
// Find users who have placed orders
sub := db.Builder().Select("user_id").From("orders").Where("status = ?", "completed")
db.Builder().Select("*").From("users").Where(relica.In("id", sub)).All(&users)

// Find users with at least one order (EXISTS is often faster)
orderCheck := db.Builder().Select("1").From("orders").Where("orders.user_id = users.id")
db.Builder().Select("*").From("users").Where(relica.Exists(orderCheck)).All(&users)
```

**FROM Subqueries**:
```go
// Calculate aggregates, then filter
stats := db.Builder().
    Select("user_id", "COUNT(*) as order_count", "SUM(total) as total_spent").
    From("orders").
    GroupBy("user_id")

db.Builder().
    FromSelect(stats, "order_stats").
    Select("user_id", "order_count", "total_spent").
    Where("order_count > ? AND total_spent > ?", 10, 5000).
    All(&topCustomers)
```

See [Subquery Guide](docs/SUBQUERY_GUIDE.md) for complete examples and performance tips.

#### Set Operations

**UNION/UNION ALL**:
```go
// Combine active and archived users (UNION removes duplicates)
active := db.Builder().Select("name").From("users").Where("status = ?", 1)
archived := db.Builder().Select("name").From("archived_users").Where("status = ?", 1)
active.Union(archived).All(&allNames)

// UNION ALL is 2-3x faster (keeps duplicates)
active.UnionAll(archived).All(&allNames)
```

**INTERSECT/EXCEPT** (PostgreSQL, MySQL 8.0.31+, SQLite):
```go
// Find users who have placed orders (INTERSECT)
allUsers := db.Builder().Select("id").From("users")
orderUsers := db.Builder().Select("user_id").From("orders")
allUsers.Intersect(orderUsers).All(&activeUsers)

// Find users without orders (EXCEPT)
allUsers.Except(orderUsers).All(&inactiveUsers)
```

See [Set Operations Guide](docs/SET_OPERATIONS_GUIDE.md) for database compatibility and workarounds.

#### Common Table Expressions (CTEs)

**Basic CTEs**:
```go
// Define reusable query
orderTotals := db.Builder().
    Select("user_id", "SUM(total) as total").
    From("orders").
    GroupBy("user_id")

// Use CTE in main query
db.Builder().
    With("order_totals", orderTotals).
    Select("*").
    From("order_totals").
    Where("total > ?", 1000).
    All(&premiumUsers)
```

**Recursive CTEs** (organizational hierarchies, trees):
```go
// Anchor: top-level employees
anchor := db.Builder().
    Select("id", "name", "manager_id", "1 as level").
    From("employees").
    Where("manager_id IS NULL")

// Recursive: children
recursive := db.Builder().
    Select("e.id", "e.name", "e.manager_id", "h.level + 1").
    From("employees e").
    InnerJoin("hierarchy h", "e.manager_id = h.id")

// Build hierarchy
db.Builder().
    WithRecursive("hierarchy", anchor.UnionAll(recursive)).
    Select("*").
    From("hierarchy").
    OrderBy("level", "name").
    All(&orgChart)
```

See [CTE Guide](docs/CTE_GUIDE.md) for hierarchical data examples (org charts, bill of materials, category trees).

#### Window Functions

Relica supports window functions via `SelectExpr()` for advanced analytics:

```go
// Rank users by order total within each country
db.Builder().
    SelectExpr("user_id", "country", "total",
        "RANK() OVER (PARTITION BY country ORDER BY total DESC) as rank").
    From("orders").
    All(&rankedOrders)

// Running totals with frame specification
db.Builder().
    SelectExpr("date", "amount",
        "SUM(amount) OVER (ORDER BY date ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as running_total").
    From("transactions").
    OrderBy("date").
    All(&runningTotals)
```

See [Window Functions Guide](docs/WINDOW_FUNCTIONS_GUIDE.md) for complete reference with RANK(), ROW_NUMBER(), LAG(), LEAD(), and frame specifications.

### Transactions

```go
// Start transaction
tx, err := db.BeginTx(ctx, &relica.TxOptions{
    Isolation: sql.LevelSerializable,
})
if err != nil {
    return err
}
defer tx.Rollback() // Rollback if not committed

// Execute queries within transaction
_, err = tx.Builder().Insert("users", userData).Execute()
if err != nil {
    return err
}

_, err = tx.Builder().
    Update("accounts").
    Set(map[string]interface{}{"balance": newBalance}).
    Where("user_id = ?", userID).
    Execute()
if err != nil {
    return err
}

// Commit transaction
return tx.Commit()
```

### Batch Operations

**Batch INSERT** (3.3x faster than individual inserts):

```go
result, err := db.Builder().
    BatchInsert("users", []string{"name", "email"}).
    Values("Alice", "alice@example.com").
    Values("Bob", "bob@example.com").
    Values("Charlie", "charlie@example.com").
    Execute()

// Or from a slice
users := []User{
    {Name: "Alice", Email: "alice@example.com"},
    {Name: "Bob", Email: "bob@example.com"},
}

batch := db.Builder().BatchInsert("users", []string{"name", "email"})
for _, user := range users {
    batch.Values(user.Name, user.Email)
}
result, err := batch.Execute()
```

**Batch UPDATE** (updates multiple rows with different values):

```go
result, err := db.Builder().
    BatchUpdate("users", "id").
    Set(1, map[string]interface{}{"name": "Alice Updated", "status": "active"}).
    Set(2, map[string]interface{}{"name": "Bob Updated", "status": "active"}).
    Set(3, map[string]interface{}{"age": 30}).
    Execute()
```

### Context Support

```go
// Query with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

var users []User
err := db.Builder().
    WithContext(ctx).
    Select().
    From("users").
    All(&users)

// Context on query level
err = db.Builder().
    Select().
    From("users").
    WithContext(ctx).
    One(&user)

// Transaction context auto-propagates
tx, _ := db.BeginTx(ctx, nil)
tx.Builder().Select().From("users").One(&user) // Uses ctx automatically
```

## 🏗️ Database Support

| Database | Status | Placeholders | Identifiers | UPSERT |
|----------|--------|--------------|-------------|--------|
| **PostgreSQL** | ✅ Full | `$1, $2, $3` | `"users"` | `ON CONFLICT` |
| **MySQL** | ✅ Full | `?, ?, ?` | `` `users` `` | `ON DUPLICATE KEY` |
| **SQLite** | ✅ Full | `?, ?, ?` | `"users"` | `ON CONFLICT` |

## ⚡ Performance

### Statement Cache

- **Default capacity**: 1000 prepared statements
- **Hit latency**: <60ns
- **Thread-safe**: Concurrent access optimized
- **Metrics**: Hit rate, evictions, cache size

```go
// Configure cache capacity
db, err := relica.Open("postgres", dsn,
    relica.WithStmtCacheCapacity(2000),
    relica.WithMaxOpenConns(25),
    relica.WithMaxIdleConns(5),
)

// Check cache statistics
stats := db.stmtCache.Stats()
fmt.Printf("Cache hit rate: %.2f%%\n", stats.HitRate*100)
```

### Batch Operations Performance

| Operation | Rows | Time | vs Single | Memory |
|-----------|------|------|-----------|--------|
| Batch INSERT | 100 | 327ms | **3.3x faster** | -15% |
| Single INSERT | 100 | 1094ms | Baseline | Baseline |
| Batch UPDATE | 100 | 1370ms | **2.5x faster** | -55% allocs |

## 🔧 Configuration

```go
db, err := relica.Open("postgres", dsn,
    // Connection pool
    relica.WithMaxOpenConns(25),
    relica.WithMaxIdleConns(5),

    // Statement cache
    relica.WithStmtCacheCapacity(1000),
)
```

### Connection Management

#### Standard Connection

```go
// Create new connection with Relica managing the pool
db, err := relica.Open("postgres", dsn)
defer db.Close()
```

#### Wrap Existing Connection (v0.3.0+)

Use `WrapDB()` when you need to integrate Relica with an existing `*sql.DB` connection:

```go
import (
    "database/sql"
    "time"

    "github.com/coregx/relica"
    _ "github.com/lib/pq"
)

// Create and configure external connection pool
sqlDB, err := sql.Open("postgres", dsn)
if err != nil {
    log.Fatal(err)
}

// Apply custom pool settings
sqlDB.SetMaxOpenConns(100)
sqlDB.SetMaxIdleConns(50)
sqlDB.SetConnMaxLifetime(time.Hour)
sqlDB.SetConnMaxIdleTime(10 * time.Minute)

// Wrap with Relica query builder
db := relica.WrapDB(sqlDB, "postgres")

// Use Relica's fluent API
var users []User
err = db.Builder().
    Select().
    From("users").
    Where("status = ?", 1).
    All(&users)

// Caller is responsible for closing the connection
defer sqlDB.Close()  // NOT db.Close()
```

**Use Cases for WrapDB:**

- **Existing Codebase Integration**: Add Relica to projects with established `*sql.DB` connections
- **Custom Pool Configuration**: Apply advanced connection pool settings before wrapping
- **Shared Connections**: Multiple parts of your application can share the same pool
- **Testing**: Wrap test database connections without managing lifecycle

**Important Notes:**

- Each `WrapDB()` call creates a new Relica instance with its own statement cache
- The caller is responsible for closing the underlying `*sql.DB` connection
- Multiple wraps of the same connection are isolated (separate caches)

## 🛡️ Enterprise Security (v0.5.0+)

Relica provides enterprise-grade security features for protecting your database operations:

### SQL Injection Prevention

**Pattern-based detection** of OWASP Top 10 SQL injection attacks with <2% overhead:

```go
import "github.com/coregx/relica/internal/security"

// Create validator
validator := security.NewValidator()

// Enable validation on DB connection
db, err := relica.Open("postgres", dsn,
    relica.WithValidator(validator),
)

// All ExecContext and QueryContext calls are now validated
_, err = db.ExecContext(ctx, "SELECT * FROM users WHERE id = ?", userID)
// Malicious queries blocked: stacked queries, UNION attacks, comment injection, etc.
```

**Detected attack vectors:**
- Tautology attacks (`1 OR 1=1`)
- Comment injection (`admin'--`)
- Stacked queries (`; DROP TABLE`)
- UNION attacks
- Command execution (`xp_cmdshell`, `exec()`)
- Information schema access
- Timing attacks (`pg_sleep`, `benchmark`)

### Audit Logging

**Comprehensive operation tracking** for GDPR, HIPAA, PCI-DSS, SOC2 compliance:

```go
// Create logger
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))

// Create auditor with desired level
auditor := security.NewAuditor(logger, security.AuditReads)

// Enable auditing
db, err := relica.Open("postgres", dsn,
    relica.WithAuditLog(auditor),
)

// Add context metadata for forensics
ctx := security.WithUser(ctx, "john.doe@example.com")
ctx = security.WithClientIP(ctx, "192.168.1.100")
ctx = security.WithRequestID(ctx, "req-12345")

// All operations are logged with metadata
_, err = db.ExecContext(ctx, "UPDATE users SET status = ? WHERE id = ?", 2, 123)
```

**Audit log includes:**
- Timestamp, user, client IP, request ID
- Operation (INSERT, UPDATE, DELETE, SELECT)
- Query execution time
- Success/failure status
- **Parameter hashing** (NOT raw values) for GDPR compliance

### Security Guides

- **[Security Guide](docs/guides/SECURITY.md)** - Complete security features overview
- **[Security Testing Guide](docs/guides/SECURITY_TESTING.md)** - OWASP-based testing examples

## 📖 Documentation

### Migration Guides (v0.5.0+)

Switching from another library? We've got you covered:

- **[Migration from GORM](docs/guides/MIGRATION_FROM_GORM.md)** - Complete guide for GORM users
  - ORM vs Query Builder philosophy
  - Side-by-side API comparisons
  - Association handling (Preload → JOIN)
  - Gradual migration strategies

- **[Migration from sqlx](docs/guides/MIGRATION_FROM_SQLX.md)** - Complete guide for sqlx users
  - Drop-in replacement patterns
  - Query builder advantages
  - Statement caching benefits
  - Using both together

### Comprehensive User Guides (v0.5.0+)

**Getting Started:**
- **[Getting Started Guide](docs/guides/GETTING_STARTED.md)** - Installation, first query, CRUD operations, common patterns
- **[Best Practices Guide](docs/guides/BEST_PRACTICES.md)** - Repository pattern, error handling, testing strategies

**Production:**
- **[Production Deployment Guide](docs/guides/PRODUCTION_DEPLOYMENT.md)** - Configuration, health checks, Docker/Kubernetes, monitoring
- **[Performance Tuning Guide](docs/guides/PERFORMANCE_TUNING.md)** - Query optimization, connection pooling, caching strategies
- **[Troubleshooting Guide](docs/guides/TROUBLESHOOTING.md)** - Common errors and solutions

**Advanced:**
- **[Advanced Patterns Guide](docs/guides/ADVANCED_PATTERNS.md)** - Complex queries, CTEs, window functions, UPSERT

### SQL Feature Guides (v0.3.0+)

- **[Subquery Guide](docs/SUBQUERY_GUIDE.md)** - IN, EXISTS, FROM, scalar subqueries with performance tips
- **[Set Operations Guide](docs/SET_OPERATIONS_GUIDE.md)** - UNION, INTERSECT, EXCEPT with database compatibility
- **[CTE Guide](docs/CTE_GUIDE.md)** - WITH clauses, recursive CTEs for hierarchical data
- **[Window Functions Guide](docs/WINDOW_FUNCTIONS_GUIDE.md)** - Analytics with RANK(), ROW_NUMBER(), LAG(), LEAD()

### Additional Resources

- **[Performance Comparison](docs/PERFORMANCE_COMPARISON.md)** - Benchmarks vs GORM, sqlx, sqlc, database/sql
- [Transaction Guide](docs/reports/TRANSACTION_IMPLEMENTATION_REPORT.md)
- [UPSERT Examples](docs/reports/UPSERT_EXAMPLES.md)
- [Batch Operations](docs/reports/BATCH_OPERATIONS.md)
- [Zero Dependencies Achievement](docs/reports/ZERO_DEPS_ACHIEVEMENT.md)
- [API Reference](https://pkg.go.dev/github.com/coregx/relica)

## 🧪 Testing

```bash
# Run unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests (requires Docker)
go test -tags=integration ./test/...

# Run benchmarks
go test -bench=. -benchmem ./benchmark/...
```

## 🎯 Design Philosophy

1. **Zero Dependencies** - Production code uses only Go standard library
2. **Type Safety** - Compile-time checks, runtime safety
3. **Performance** - Statement caching, batch operations, zero allocations in hot paths
4. **Simplicity** - Clean API, easy to learn, hard to misuse
5. **Correctness** - ACID transactions, proper error handling
6. **Observability** - Built-in metrics, context support for tracing

## 📊 Project Status

- **Version**: v0.4.1-beta
- **Go Version**: 1.25+
- **Production Ready**: Yes (beta)
- **Test Coverage**: 93.3%
- **Dependencies**: 0 (production), 2 (tests only)
- **API**: Stable public API, internal packages protected

## 🤝 Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) first.

## 📝 License

Relica is released under the [MIT License](LICENSE).

## 🙏 Acknowledgments

- Inspired by [ozzo-dbx](https://github.com/go-ozzo/ozzo-dbx)
- Built with Go 1.25+ features
- Zero-dependency philosophy inspired by Go standard library

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/coregx/relica/issues)
- **Discussions**: [GitHub Discussions](https://github.com/coregx/relica/discussions)
- **Email**: support@coregx.dev

## ✨ Special Thanks

**Professor Ancha Baranova** - This project would not have been possible without her invaluable help and support. Her assistance was crucial in bringing Relica to life.

---

**Made with ❤️ by COREGX Team**

*Relica - Lightweight, Fast, Zero-Dependency Database Query Builder for Go*
