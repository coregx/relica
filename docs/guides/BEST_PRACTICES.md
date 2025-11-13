# Best Practices Guide

> **Production-Ready Patterns for Relica**
>
> **Version**: v0.5.0
> **Last Updated**: 2025-11-13

---

## 📋 Overview

This guide covers battle-tested patterns for using Relica in production environments. Follow these practices to build robust, performant, and maintainable database applications.

---

## 🏗️ Project Structure

### Recommended Layout

```
myapp/
├── cmd/
│   └── api/
│       └── main.go           # Application entry point
├── internal/
│   ├── database/
│   │   ├── db.go             # Database initialization
│   │   ├── migrations/       # SQL migration files
│   │   └── queries/          # Complex queries
│   ├── models/
│   │   └── user.go           # Data models
│   └── repository/
│       └── user_repository.go # Database operations
├── config/
│   └── database.yaml         # Database configuration
└── go.mod
```

### Database Initialization

**internal/database/db.go:**
```go
package database

import (
    "context"
    "fmt"
    "time"

    "github.com/coregx/relica"
)

type Config struct {
    Driver          string
    DSN             string
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime int // seconds
}

func NewDB(cfg Config) (*relica.DB, error) {
    db, err := relica.Open(cfg.Driver, cfg.DSN,
        relica.WithMaxOpenConns(cfg.MaxOpenConns),
        relica.WithMaxIdleConns(cfg.MaxIdleConns),
        relica.WithConnMaxLifetime(cfg.ConnMaxLifetime),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }

    // Verify connection
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := db.PingContext(ctx); err != nil {
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }

    return db, nil
}
```

---

## 🎯 Repository Pattern

### Why Use Repositories?

✅ **Separation of concerns** - Database logic separate from business logic
✅ **Testability** - Easy to mock for unit tests
✅ **Reusability** - Centralized database operations
✅ **Maintainability** - Single place to update queries

### Implementation

**internal/models/user.go:**
```go
package models

import "time"

type User struct {
    ID        int       `db:"id"`
    Name      string    `db:"name"`
    Email     string    `db:"email"`
    CreatedAt time.Time `db:"created_at"`
    UpdatedAt time.Time `db:"updated_at"`
}
```

**internal/repository/user_repository.go:**
```go
package repository

import (
    "context"
    "fmt"

    "github.com/coregx/relica"
    "myapp/internal/models"
)

type UserRepository struct {
    db *relica.DB
}

func NewUserRepository(db *relica.DB) *UserRepository {
    return &UserRepository{db: db}
}

func (r *UserRepository) FindByID(ctx context.Context, id int) (*models.User, error) {
    var user models.User
    err := r.db.Select("*").
        From("users").
        Where("id = ?", id).
        WithContext(ctx).
        One(&user)

    if err != nil {
        return nil, fmt.Errorf("user not found: %w", err)
    }

    return &user, nil
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
    var id int
    err := r.db.QueryRowContext(ctx,
        `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`,
        user.Name, user.Email,
    ).Scan(&id)

    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }

    user.ID = id
    return nil
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
    _, err := r.db.Update("users").
        Set(map[string]interface{}{
            "name":       user.Name,
            "email":      user.Email,
            "updated_at": "NOW()",
        }).
        Where("id = ?", user.ID).
        WithContext(ctx).
        Execute()

    return err
}

func (r *UserRepository) Delete(ctx context.Context, id int) error {
    _, err := r.db.Delete("users").
        Where("id = ?", id).
        WithContext(ctx).
        Execute()

    return err
}
```

---

## 🔒 Transaction Best Practices

### Pattern 1: Defer Rollback

Always use `defer tx.Rollback()`:

```go
func transferMoney(db *relica.DB, fromID, toID int, amount float64) error {
    tx, err := db.Begin(context.Background())
    if err != nil {
        return err
    }
    defer tx.Rollback() // Safe: Rollback after Commit is no-op

    // Deduct from sender
    _, err = tx.Update("accounts").
        Set(map[string]interface{}{"balance": "balance - ?"}).
        Where("id = ?", fromID).
        Execute()
    if err != nil {
        return err // Auto-rollback via defer
    }

    // Add to receiver
    _, err = tx.Update("accounts").
        Set(map[string]interface{}{"balance": "balance + ?"}).
        Where("id = ?", toID).
        Execute()
    if err != nil {
        return err // Auto-rollback via defer
    }

    return tx.Commit() // Only commit if all operations succeed
}
```

### Pattern 2: Context Propagation

Pass context through transaction:

```go
func createUserWithProfile(ctx context.Context, db *relica.DB, user User, profile Profile) error {
    tx, err := db.BeginTx(ctx, nil) // Context auto-propagates
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Create user
    var userID int
    err = tx.QueryRowContext(ctx, // Use same context
        `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`,
        user.Name, user.Email,
    ).Scan(&userID)
    if err != nil {
        return err
    }

    // Create profile
    _, err = tx.Insert("profiles", map[string]interface{}{
        "user_id": userID,
        "bio":     profile.Bio,
    }).Execute()
    if err != nil {
        return err
    }

    return tx.Commit()
}
```

---

## ⚡ Performance Patterns

### Pattern 1: Batch Operations

Replace loops with batch operations:

```go
// ❌ SLOW: N queries
func createUsersSlowly(db *relica.DB, users []User) error {
    for _, user := range users {
        _, err := db.Insert("users", map[string]interface{}{
            "name":  user.Name,
            "email": user.Email,
        }).Execute()
        if err != nil {
            return err
        }
    }
    return nil
}

// ✅ FAST: 1 query (3.3x faster)
func createUsersFast(db *relica.DB, users []User) error {
    batch := db.Builder().BatchInsert("users", []string{"name", "email"})
    for _, user := range users {
        batch.Values(user.Name, user.Email)
    }
    _, err := batch.Execute()
    return err
}
```

### Pattern 2: Statement Cache Awareness

Reuse query patterns for cache hits:

```go
// ✅ GOOD: Cache-friendly (same query pattern)
func getUserByID(db *relica.DB, id int) (*User, error) {
    var user User
    err := db.Select("*").From("users").Where("id = ?", id).One(&user)
    return &user, err
}

// First call: prepares statement
getUserByID(db, 1)

// Subsequent calls: <60ns cache lookup
getUserByID(db, 2)
getUserByID(db, 3)
```

### Pattern 3: Connection Pool Tuning

```go
// Production settings
db, err := relica.Open("postgres", dsn,
    relica.WithMaxOpenConns(25),     // ~25 per CPU core
    relica.WithMaxIdleConns(5),      // 20% of MaxOpenConns
    relica.WithConnMaxLifetime(300), // 5 minutes
)
```

**Guidelines:**
- `MaxOpenConns`: 10-25 per CPU core
- `MaxIdleConns`: 20% of MaxOpenConns
- `ConnMaxLifetime`: 5-10 minutes (prevents stale connections)

---

## 🛡️ Error Handling

### Pattern 1: Wrap Errors with Context

```go
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
    var user User
    err := r.db.Select("*").
        From("users").
        Where("email = ?", email).
        WithContext(ctx).
        One(&user)

    if err != nil {
        return nil, fmt.Errorf("failed to find user by email %s: %w", email, err)
    }

    return &user, nil
}
```

### Pattern 2: Handle sql.ErrNoRows

```go
import "database/sql"

func (r *UserRepository) FindByID(ctx context.Context, id int) (*User, error) {
    var user User
    err := r.db.Select("*").
        From("users").
        Where("id = ?", id).
        One(&user)

    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("user %d not found", id)
    }

    if err != nil {
        return nil, fmt.Errorf("database error: %w", err)
    }

    return &user, nil
}
```

---

## 🔍 Query Building Best Practices

### Use Expression API for Type Safety

```go
// ❌ Error-prone: Manual string building
func searchUsers(db *relica.DB, filters map[string]interface{}) ([]User, error) {
    query := "SELECT * FROM users WHERE 1=1"
    args := []interface{}{}

    if name, ok := filters["name"].(string); ok {
        query += " AND name = ?"
        args = append(args, name)
    }

    // ... complex and error-prone
}

// ✅ Type-safe: Expression API
func searchUsers(db *relica.DB, name string, minAge int) ([]User, error) {
    qb := db.Select("*").From("users")

    if name != "" {
        qb = qb.Where(relica.Eq("name", name))
    }

    if minAge > 0 {
        qb = qb.Where(relica.GreaterThan("age", minAge))
    }

    var users []User
    err := qb.All(&users)
    return users, err
}
```

---

## 🧪 Testing Patterns

### Use Testcontainers for Integration Tests

```go
import (
    "testing"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *relica.DB {
    ctx := context.Background()

    req := testcontainers.ContainerRequest{
        Image:        "postgres:15",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_PASSWORD": "test",
            "POSTGRES_DB":       "testdb",
        },
        WaitingFor: wait.ForLog("database system is ready to accept connections"),
    }

    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        t.Fatal(err)
    }

    t.Cleanup(func() { container.Terminate(ctx) })

    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "5432")

    dsn := fmt.Sprintf("postgres://postgres:test@%s:%s/testdb?sslmode=disable", host, port.Port())
    db, err := relica.Open("postgres", dsn)
    if err != nil {
        t.Fatal(err)
    }

    return db
}
```

---

## 📊 Monitoring and Observability

### Log Queries in Development

```go
import "log/slog"

type QueryLogger struct {
    logger *slog.Logger
}

func (ql *QueryLogger) LogQuery(query string, args []interface{}, duration time.Duration) {
    ql.logger.Info("query executed",
        "query", query,
        "duration_ms", duration.Milliseconds(),
        "args_count", len(args),
    )
}

// Use in development
if isDevelopment() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    db.SetQueryLogger(&QueryLogger{logger: logger})
}
```

---

## 🚀 Production Checklist

Before deploying to production:

- [ ] **Connection pooling configured** (MaxOpenConns, MaxIdleConns)
- [ ] **Timeouts set** on all context operations
- [ ] **Transactions use defer Rollback()**
- [ ] **Errors wrapped with context** (`fmt.Errorf("context: %w", err)`)
- [ ] **Sensitive data masked** in logs
- [ ] **Migrations tested** on staging environment
- [ ] **Indexes created** for frequently queried columns
- [ ] **Query performance tested** under load
- [ ] **Security features enabled** (validator, auditor if needed)
- [ ] **Health checks implemented** (db.PingContext)

---

## 📖 Additional Resources

- **[Getting Started Guide](GETTING_STARTED.md)** - Basics
- **[Performance Tuning Guide](PERFORMANCE_TUNING.md)** - Optimization
- **[Security Guide](SECURITY.md)** - SQL injection prevention
- **[Troubleshooting Guide](TROUBLESHOOTING.md)** - Common issues

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
