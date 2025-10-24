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
- 📦 **Batch Operations** - Efficient multi-row INSERT and UPDATE
- 🌐 **Multi-Database** - PostgreSQL, MySQL, SQLite support
- 🧪 **Well-Tested** - 123+ tests, 83% coverage
- 📝 **Clean API** - Fluent builder pattern with context support

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

    // SELECT - Query single row
    var user User
    err = db.Builder().
        Select().
        From("users").
        Where("id = ?", 1).
        WithContext(ctx).
        One(&user)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("User: %+v\n", user)

    // SELECT - Query multiple rows
    var users []User
    err = db.Builder().
        Select().
        From("users").
        Where("age > ?", 18).
        All(&users)

    // INSERT
    result, err := db.Builder().
        Insert("users", map[string]interface{}{
            "name":  "Alice",
            "email": "alice@example.com",
        }).
        Execute()

    // UPDATE
    result, err = db.Builder().
        Update("users").
        Set(map[string]interface{}{
            "name": "Alice Updated",
        }).
        Where("id = ?", 1).
        Execute()

    // DELETE
    result, err = db.Builder().
        Delete("users").
        Where("id = ?", 1).
        Execute()
}
```

## 📚 Core Features

### CRUD Operations

```go
// SELECT
var user User
db.Builder().Select().From("users").Where("id = ?", 1).One(&user)

// SELECT with multiple conditions
var users []User
db.Builder().
    Select("id", "name", "email").
    From("users").
    Where("age > ?", 18).
    Where("status = ?", "active").
    All(&users)

// INSERT
db.Builder().Insert("users", map[string]interface{}{
    "name": "Bob",
    "email": "bob@example.com",
}).Execute()

// UPDATE
db.Builder().
    Update("users").
    Set(map[string]interface{}{"status": "inactive"}).
    Where("last_login < ?", time.Now().AddDate(0, -6, 0)).
    Execute()

// DELETE
db.Builder().Delete("users").Where("id = ?", 123).Execute()

// UPSERT (INSERT ON CONFLICT)
db.Builder().
    Upsert("users", map[string]interface{}{
        "id":    1,
        "name":  "Alice",
        "email": "alice@example.com",
    }).
    OnConflict("id").
    DoUpdate("name", "email").
    Execute()
```

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

## 📖 Documentation

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

- **Version**: v0.1.0-beta
- **Go Version**: 1.25+
- **Production Ready**: Yes (beta)
- **Test Coverage**: 47.8%
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
