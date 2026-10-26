# Best Practices Guide

> **Production-Ready Patterns for Relica**
>
> **Last Updated**: 2026-10-18

> **🤖 AI Agents**: See [AGENTS.md](../../AGENTS.md) for correct API patterns. Prefer `Model()` API over `map[string]interface{}`.

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

Create the `*relica.DB` **once** and share it: it owns the connection pool and the statement
cache. Wrapping a `*sql.DB` with `relica.WrapDB` per request or per transaction creates a new,
empty cache every time.

### Transaction in Context (dbcontext)

Repositories should not know whether they run inside a transaction. Keep the transaction in
`context.Context` and let one small type hand out the right executor:

**internal/database/dbcontext.go:**
```go
package database

import (
    "context"

    "github.com/coregx/relica"
)

type txKey struct{}

// DB hands out the active transaction or the request-scoped connection.
type DB struct{ db *relica.DB }

func NewDBContext(db *relica.DB) *DB { return &DB{db: db} }

// With returns the transaction stored in ctx, otherwise the DB bound to ctx.
func (d *DB) With(ctx context.Context) relica.Executor {
    if tx, ok := ctx.Value(txKey{}).(*relica.Tx); ok {
        return tx
    }
    return d.db.WithContext(ctx) // not bare d.db: request deadlines must reach SQL
}

// Transactional runs f in a transaction, or joins the one already in ctx.
func (d *DB) Transactional(ctx context.Context, f func(ctx context.Context) error) error {
    if _, ok := ctx.Value(txKey{}).(*relica.Tx); ok {
        return f(ctx)
    }
    return d.db.Transactional(ctx, func(tx *relica.Tx) error {
        return f(context.WithValue(ctx, txKey{}, tx))
    })
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
    "errors"
    "fmt"

    "github.com/coregx/relica"
    "myapp/internal/database"
    "myapp/internal/models"
)

var (
    ErrUserNotFound = errors.New("user not found")
    ErrEmailTaken   = errors.New("email already exists")
)

type UserRepository struct {
    db *database.DB // dbcontext: joins the caller's transaction, if any
}

func NewUserRepository(db *database.DB) *UserRepository {
    return &UserRepository{db: db}
}

func (r *UserRepository) FindByID(ctx context.Context, id int) (*models.User, error) {
    var user models.User
    err := r.db.With(ctx).
        Select("id", "name", "email", "created_at", "updated_at"). // explicit columns in production code
        From("users").
        Where(relica.Eq("id", id)).
        One(&user)
    if errors.Is(err, relica.ErrNotFound) {
        return nil, fmt.Errorf("user %d: %w", id, ErrUserNotFound)
    }
    if err != nil {
        return nil, fmt.Errorf("find user %d: %w", id, err)
    }
    return &user, nil
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
    // Model API auto-populates user.ID after insert
    if err := r.db.With(ctx).Model(user).Insert(); err != nil {
        if relica.IsUniqueViolation(err) {
            return ErrEmailTaken
        }
        return fmt.Errorf("create user: %w", err)
    }
    return nil
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
    return r.db.With(ctx).Model(user).Update()
}

func (r *UserRepository) Delete(ctx context.Context, id int) error {
    return r.db.With(ctx).Model(&models.User{ID: id}).Delete()
}
```

Every method calls `r.db.With(ctx)`: outside a transaction it runs on the pool under the request
context; inside `Transactional` it runs on the transaction.

---

## 🔒 Transaction Best Practices

The full rule set, the API choice table and the mistakes seen in real projects are in
[AGENTS.md → Transactions](../../AGENTS.md#transactions). The patterns below are the ones to reach for.

### Pattern 1: The Service Owns the Transaction

The transaction boundary is one business operation, opened in the application service (or HTTP
handler, CLI command, job). Repositories never begin or commit:

```go
type SignupService struct {
    db       *database.DB
    users    *repository.UserRepository
    profiles *repository.ProfileRepository
}

func (s *SignupService) Signup(ctx context.Context, user *models.User, profile *models.Profile) error {
    return s.db.Transactional(ctx, func(ctx context.Context) error {
        if err := s.users.Create(ctx, user); err != nil {
            return err // rollback
        }
        profile.UserID = user.ID // set by Model().Insert()
        return s.profiles.Create(ctx, profile)
    })
    // e-mails, events, HTTP calls: after Transactional returns nil, not inside it
}
```

### Pattern 2: Lock, Check, Write

Read-check-write on shared rows needs a row lock **inside** the transaction; arithmetic stays in SQL:

```go
func (s *WalletService) Withdraw(ctx context.Context, accountID, amount int64) error {
    return s.db.Transactional(ctx, func(ctx context.Context) error {
        var acc models.Account
        if err := s.db.With(ctx).Select("id", "balance").From("accounts").
            Where(relica.Eq("id", accountID)).
            ForUpdate(). // released at commit/rollback; useless outside a transaction
            One(&acc); err != nil {
            return err
        }
        if acc.Balance < amount {
            return ErrInsufficientFunds
        }
        _, err := s.db.With(ctx).Update("accounts").
            Set(map[string]any{"balance": relica.NewExp("balance - ?", amount)}).
            Where(relica.Eq("id", accountID)).
            Execute()
        return err
    })
}
```

`Set(map[string]any{"balance": "balance - ?"})` would store the text `balance - ?`; only
`relica.NewExp` is evaluated by the database.

### Pattern 3: Manual Begin (when the work is not one function)

```go
tx, err := db.BeginTx(ctx, &relica.TxOptions{Isolation: sql.LevelSerializable})
if err != nil {
    return err // tx is nil: check before deferring
}
defer func() { _ = tx.Rollback() }() // returns sql.ErrTxDone after Commit — expected

// ... every statement through tx, every error checked ...

return tx.Commit() // a failed Commit means nothing was saved
```

The transaction is bound to the context given to `Begin`/`BeginTx`/`Transactional`: when it is
canceled, `database/sql` rolls the transaction back.

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
    batch := db.BatchInsert("users", []string{"name", "email"})
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
    err := db.Select().From("users").Where("id = ?", id).One(&user)
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
    err := r.db.With(ctx).Select().
        From("users").
        Where(relica.Eq("email", email)).
        One(&user)

    if errors.Is(err, relica.ErrNotFound) {
        return nil, fmt.Errorf("user with email %s not found", email)
    }
    if err != nil {
        return nil, fmt.Errorf("find user by email: %w", err)
    }

    return &user, nil
}
```

### Pattern 2: Handle ErrNotFound (v0.11.0+)

`One()` returns `relica.ErrNotFound` (wrapping `sql.ErrNoRows`). Use `errors.Is` — it matches both:

```go
import (
    "errors"
    "github.com/coregx/relica"
)

func (r *UserRepository) FindByID(ctx context.Context, id int) (*User, error) {
    var user User
    err := r.db.With(ctx).Select().
        From("users").
        Where(relica.Eq("id", id)).
        One(&user)

    if errors.Is(err, relica.ErrNotFound) {
        return nil, fmt.Errorf("user %d not found", id)
    }

    if err != nil {
        return nil, fmt.Errorf("database error: %w", err)
    }

    return &user, nil
}
```

### Pattern 3: Classify Constraint Errors

Use error classification functions to return meaningful errors without exposing database details:

```go
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
    if err := r.db.With(ctx).Model(user).Insert(); err != nil {
        switch {
        case relica.IsUniqueViolation(err):
            return ErrEmailAlreadyExists
        case relica.IsNotNullViolation(err):
            return ErrMissingRequiredField
        case relica.IsForeignKeyViolation(err):
            return ErrInvalidReference
        case relica.IsCheckViolation(err):
            return ErrInvalidFieldValue
        default:
            return fmt.Errorf("create user: %w", err)
        }
    }
    return nil
}
```

The helpers read the driver's error code (SQLSTATE, MySQL/MariaDB error number, SQLite extended
code), so they keep working on a non-English server and through any `%w` wrapping. For deadlocks and
serialization failures use `relica.IsDeadlock` / `relica.IsSerializationFailure` and retry the whole
transaction — see `RetryTx` in [AGENTS.md](../../AGENTS.md#retrying-deadlocks-and-serialization-failures).

### Pattern 4: Existence Check Without Loading Data

Prefer `Exists()` over loading a full row when you only need a boolean:

```go
func (r *UserRepository) EmailTaken(ctx context.Context, email string) (bool, error) {
    return r.db.With(ctx).Select().From("users").
        Where(relica.Eq("email", email)).
        Exists()
}

// Prefer Exists() over Count() > 0 for existence checks
// Prefer Count() when you need the actual number
count, err := r.db.With(ctx).Select().From("users").
    Where(relica.Eq("status", "active")).
    Count()
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
    qb := db.Select().From("users")

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

func openDB(dsn string) (*relica.DB, error) {
    var opts []relica.Option

    // Use in development
    if isDevelopment() {
        logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
        opts = append(opts, relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
            logger.InfoContext(ctx, "query executed",
                "query", e.SQL,
                "duration_ms", e.Duration.Milliseconds(),
                "args_count", len(e.Args),
            )
        }))
    }

    return relica.Open("postgres", dsn, opts...)
}
```

For the standard log line (SQL, duration, rows, error) use `relica.WithLogger(relica.NewSlogAdapter(logger))` instead. See the [Logging Guide](../LOGGING_GUIDE.md).

---

## 🔄 Model API Patterns

### UpdateChanged — Only Save What Changed

When updating a record, use `UpdateChanged` to generate a minimal UPDATE statement that touches only the fields that actually differ from the original:

```go
func (r *UserRepository) UpdateProfile(ctx context.Context, id int, req UpdateRequest) error {
    var user models.User
    if err := r.db.With(ctx).Select().From("users").
        Where(relica.Eq("id", id)).One(&user); err != nil {
        return err
    }

    original := user // snapshot before changes

    user.Name = req.Name
    user.Email = req.Email

    // UPDATE users SET name=? WHERE id=? — only changed fields
    return r.db.With(ctx).Model(&user).UpdateChanged(&original)
}
```

This avoids unnecessary column writes, and columns this request did not change are not overwritten
with stale values. It is not optimistic locking: two requests changing the same column still race —
read with `ForUpdate()` inside a transaction (Pattern 2 above) or compare a version column.

### Upsert — Insert or Update on Conflict

```go
func (r *UserRepository) SaveSettings(ctx context.Context, settings *UserSettings) error {
    // Insert or update name/value if email conflicts
    return r.db.With(ctx).Model(settings).Upsert("name", "value")
    // All non-PK fields: db.Model(settings).Upsert()
}
```

---

## 🔑 Primary Key Strategy

Choosing the right primary key type is one of the most impactful decisions for long-term database performance. This section covers common strategies, their trade-offs, and recommended patterns with Relica.

### The UUID v4 Problem

> **Warning**: Random UUIDs (v4) as primary keys cause severe B-tree index fragmentation at scale. A single-row lookup can degrade from 20ms to 3.8 seconds at 200M+ rows due to 94,000+ disk block reads.

**Why this happens**: B-tree indexes store keys in sorted order. Sequential keys (autoincrement, time-ordered) insert at the end — page fill rate stays ~94%. Random UUID v4 inserts into random positions, causing page splits. Result: ~50-60% fill rate, index bloat of 1.5-2x, and increasingly cold cache as the table grows.

**Scale of impact**:

| Table Size | UUID v4 INSERT/s | Autoincrement INSERT/s | Degradation |
|-----------|-----------------|----------------------|-------------|
| < 1M rows | ~50,000 | ~55,000 | Negligible |
| 10M rows | ~35,000 | ~50,000 | ~30% |
| 100M rows | ~8,000 | ~50,000 | **6x slower** |
| 200M+ rows | lookup 3.8s | lookup 20ms | **190x slower** |

**PostgreSQL vs MySQL**: In PostgreSQL (heap tables), fragmentation affects only the B-tree index. In MySQL/InnoDB (clustered index), the **entire table** is stored in PK order — UUID v4 fragments the row storage itself plus all secondary indexes.

### Recommended Strategies

#### 1. Integer Autoincrement (Simplest, Best Performance)

Best for single-database applications without external API exposure:

```go
type User struct {
    ID    int64  `db:"id,pk"`     // 8 bytes, optimal B-tree
    Name  string `db:"name"`
    Email string `db:"email"`
}

user := User{Name: "Alice", Email: "alice@example.com"}
db.Model(&user).Insert()
fmt.Println(user.ID) // auto-populated
```

```sql
CREATE TABLE users (
    id    BIGSERIAL PRIMARY KEY,
    name  TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE
);
```

**Pros**: Optimal B-tree (8 bytes, sequential), simplest implementation, best JOIN performance.
**Cons**: Predictable (information disclosure in APIs), not suitable for distributed ID generation.

#### 2. UUID v7 — Time-Ordered UUID (RFC 9562)

Best when you need distributed ID generation or client-side ID creation:

```go
type Event struct {
    ID   string `db:"id,pk,autoincrement"` // server-generated UUID v7
    Type string `db:"type"`
    Data string `db:"data"`
}

event := Event{Type: "user.created", Data: `{"userId": 42}`}
db.Model(&event).Insert()
fmt.Println(event.ID) // "019078fa-b37e-7abc-..." (auto-populated via RETURNING)
```

```sql
-- PostgreSQL 18+:
CREATE TABLE events (
    id   UUID PRIMARY KEY DEFAULT uuidv7(),
    type TEXT NOT NULL,
    data JSONB
);

-- PostgreSQL 13-17 (extension required):
-- CREATE EXTENSION IF NOT EXISTS pg_uuidv7;
-- ... DEFAULT uuid_generate_v7()
```

**Pros**: Time-ordered (solves B-tree fragmentation), globally unique, no coordination needed, embeds creation timestamp.
**Cons**: 16 bytes (2x integer), timestamp leaks creation time, requires PG 18+ for native support.

> **Note**: The `autoincrement` tag tells Relica to use PostgreSQL `RETURNING` clause to read back the server-generated value. For MySQL/SQLite, generate the UUID in your application code before inserting.

#### 3. Dual-Key Pattern with AutoID (Recommended for APIs)

Best for production APIs — optimal performance internally, safe identifiers externally. Used by Stripe (`cus_...`), Shopify, GitHub. **Relica supports this natively via `autoid` tag:**

```go
type User struct {
    ID       int64  `db:"id,pk"`                    // internal: fast joins, compact FK
    PublicID string `db:"public_id,autoid:usr"`      // external: auto-generated "usr_019078fa-..."
    Name     string `db:"name"`
    Email    string `db:"email"`
}

// Insert — PublicID auto-generated, ID auto-populated:
func CreateUser(db *relica.DB, name, email string) (User, error) {
    user := User{Name: name, Email: email}
    err := db.Model(&user).Insert()
    return user, err
    // user.ID = 42 (auto-populated)
    // user.PublicID = "usr_019078fa-b37e-..." (auto-generated)
}

// API handler: one-liner lookup by public ID
func GetUser(db *relica.DB, publicID string) (User, error) {
    var user User
    err := db.Model(&user).FindByPublicID(publicID)
    return user, err
}

// Wrong prefix → safe error (prevents cross-model lookups):
err := db.Model(&user).FindByPublicID("ord_019078fa-...")
// errors.Is(err, relica.ErrAutoIDPrefixMismatch) == true

// Internal queries: use integer PK for joins (8 bytes, optimal)
db.Select("u.name", "o.total").
    From("users u").
    InnerJoin("orders o", relica.EqCol("o.user_id", "u.id")).
    Where(relica.Eq("u.id", internalUserID)).
    All(&results)
```

```sql
CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    public_id  TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL UNIQUE
);
CREATE INDEX idx_users_public_id ON users(public_id);
```

**Pros**: Optimal B-tree for PK, compact 8-byte FK references, external ID can change strategy without PK migration, no information disclosure, zero boilerplate with `autoid` tag.
**Cons**: Extra column + index (24 bytes/row overhead), two lookup paths.

### Decision Matrix

| Criteria | `int64` autoincrement | UUID v7 | Dual-key (int64 + UUID) |
|----------|----------------------|---------|------------------------|
| B-tree efficiency | Optimal | Good | Optimal |
| PK index size | 8 bytes | 16 bytes | 8 bytes (+16 for external) |
| Distributed generation | No | Yes | Yes (for external ID) |
| API-safe | No (sequential) | Partial (timestamp leak) | **Yes** |
| FK join performance | Best | Good | Best |
| Complexity | Low | Low | Medium |

**Quick guide**:
- **Internal tool, no API**: `int64` autoincrement
- **Public API, < 50M rows**: UUID v7 as PK
- **Public API, 50M+ rows or performance-critical**: Dual-key pattern
- **Distributed/microservices**: UUID v7 or Snowflake IDs

### What to Avoid

```go
// ❌ NEVER: Random UUID v4 as primary key
// This WILL degrade at scale. No exceptions.
type BadExample struct {
    ID string `db:"id,pk"` // uuid.New() = random v4 = B-tree fragmentation
}

// ❌ AVOID: Exposing autoincrement IDs in public APIs
// Sequential integers leak information (total count, creation order, scraping)
// GET /api/users/42 → attacker tries /api/users/43, /api/users/44...

// ✅ INSTEAD: Use UUID v7 or dual-key pattern (see above)
```

### Table Partitioning (Complementary)

At 50M+ rows, consider table partitioning regardless of PK strategy:

```sql
-- Range partitioning by date (PostgreSQL)
CREATE TABLE events (
    id         BIGSERIAL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    data       JSONB,
    PRIMARY KEY (id, created_at)  -- must include partition key
) PARTITION BY RANGE (created_at);

CREATE TABLE events_2026_q3 PARTITION OF events
    FOR VALUES FROM ('2026-07-01') TO ('2026-10-01');
```

Partitioning reduces the working set per query, improving cache hit rates independently of PK type.

---

## 🚀 Production Checklist

Before deploying to production:

- [ ] **Connection pooling configured** (MaxOpenConns, MaxIdleConns)
- [ ] **Timeouts set** on all context operations
- [ ] **Transactions use defer Rollback()**
- [ ] **Errors wrapped with context** (`fmt.Errorf("context: %w", err)`)
- [ ] **Sensitive data masked** in logs
- [ ] **errors.Is(err, relica.ErrNotFound)** used for not-found checks (not sql.ErrNoRows comparison)
- [ ] **Constraint errors classified** (IsUniqueViolation, IsForeignKeyViolation, etc.)
- [ ] **Migrations tested** on staging environment
- [ ] **Primary key strategy chosen** (see [Primary Key Strategy](#-primary-key-strategy) — avoid UUID v4 as PK)
- [ ] **Indexes created** for frequently queried columns
- [ ] **Query performance tested** under load
- [ ] **No SQL built from input** (placeholders, `relica.Params`, Expression API; identifiers from an allowlist)
- [ ] **Health checks implemented** (db.PingContext)

---

## 📖 Additional Resources

- **[Getting Started Guide](GETTING_STARTED.md)** - Basics
- **[Performance Tuning Guide](PERFORMANCE_TUNING.md)** - Optimization
- **[Security Guide](SECURITY.md)** - SQL injection prevention
- **[Troubleshooting Guide](TROUBLESHOOTING.md)** - Common issues

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
