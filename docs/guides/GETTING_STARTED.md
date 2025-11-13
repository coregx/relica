# Getting Started with Relica

> **Quick Start Guide** - From Zero to Production in 15 Minutes
>
> **Version**: v0.5.0
> **Last Updated**: 2025-11-13

---

## 📋 What is Relica?

**Relica** is a lightweight, type-safe database query builder for Go with **zero production dependencies**.

**Key Features:**
- 🚀 Zero dependencies (only Go standard library)
- ⚡ High performance (LRU statement cache, batch operations)
- 🎯 Type-safe (compile-time checks, runtime safety)
- 🔒 ACID transactions with all isolation levels
- 🌐 Multi-database (PostgreSQL, MySQL, SQLite)
- 📝 Clean fluent API

**What Relica is NOT:**
- ❌ Not a full ORM (no auto-migrations, no model associations)
- ❌ Not a schema migration tool (use golang-migrate, goose, etc.)
- ❌ Not a replacement for SQL (it's a query builder)

**When to use Relica:**
- ✅ You want explicit control over queries
- ✅ Performance is critical
- ✅ You prefer SQL-like syntax
- ✅ You want zero dependencies

---

## 🚀 Installation

### Step 1: Install Relica

```bash
go get github.com/coregx/relica
```

### Step 2: Install Database Driver

Choose your database:

**PostgreSQL:**
```bash
go get github.com/lib/pq
```

**MySQL:**
```bash
go get github.com/go-sql-driver/mysql
```

**SQLite:**
```bash
go get modernc.org/sqlite
```

### Step 3: Verify Installation

```bash
go mod tidy
go mod verify
```

---

## 💻 Your First Query

### Basic Setup

Create `main.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/coregx/relica"
    _ "github.com/lib/pq" // PostgreSQL driver
)

func main() {
    // 1. Connect to database
    db, err := relica.Open("postgres",
        "postgres://user:password@localhost:5432/mydb?sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // 2. Define your struct
    type User struct {
        ID    int    `db:"id"`
        Name  string `db:"name"`
        Email string `db:"email"`
    }

    ctx := context.Background()

    // 3. Query data
    var users []User
    err = db.Select("*").
        From("users").
        Where("age > ?", 18).
        All(&users)

    if err != nil {
        log.Fatal(err)
    }

    // 4. Use the data
    for _, user := range users {
        fmt.Printf("User: %s (%s)\n", user.Name, user.Email)
    }
}
```

### Run It

```bash
go run main.go
```

**Congratulations!** You just ran your first Relica query.

---

## 📚 Core Concepts

### 1. Database Connection

**Open a new connection:**
```go
db, err := relica.Open("postgres", dsn)
defer db.Close()
```

**Wrap existing connection:**
```go
sqlDB, _ := sql.Open("postgres", dsn)
db := relica.WrapDB(sqlDB, "postgres")
```

**Configure connection pool:**
```go
db, err := relica.Open("postgres", dsn,
    relica.WithMaxOpenConns(25),
    relica.WithMaxIdleConns(5),
    relica.WithConnMaxLifetime(300), // 5 minutes
)
```

### 2. Struct Mapping

**Use `db` tags to map columns:**

```go
type User struct {
    ID        int       `db:"id"`
    Name      string    `db:"name"`
    Email     string    `db:"email"`
    CreatedAt time.Time `db:"created_at"`
}
```

**Important:**
- ✅ Fields WITH `db` tags will be scanned
- ❌ Fields WITHOUT `db` tags will be ignored

### 3. Query Building

**Fluent API (v0.4.1+):**
```go
// Convenience methods (shorter)
db.Select("*").From("users").All(&users)

// Traditional (still works)
db.Builder().Select("*").From("users").All(&users)
```

**Chaining methods:**
```go
db.Select("id", "name", "email").
    From("users").
    Where("status = ?", "active").
    OrderBy("created_at DESC").
    Limit(10).
    All(&users)
```

---

## 🔍 CRUD Operations

### SELECT - Query Multiple Rows

```go
var users []User
err := db.Select("*").
    From("users").
    Where("age > ?", 18).
    All(&users)
```

### SELECT - Query Single Row

```go
var user User
err := db.Select("*").
    From("users").
    Where("id = ?", 1).
    One(&user)
```

### INSERT

```go
result, err := db.Insert("users", map[string]interface{}{
    "name":  "Alice",
    "email": "alice@example.com",
    "age":   30,
}).Execute()

// Get rows affected
rows, _ := result.RowsAffected()
fmt.Printf("Inserted %d rows\n", rows)
```

**PostgreSQL - Get ID with RETURNING:**
```go
var id int
err := db.QueryRowContext(ctx,
    `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`,
    "Alice", "alice@example.com",
).Scan(&id)
```

### UPDATE

```go
result, err := db.Update("users").
    Set(map[string]interface{}{
        "name":  "Alice Updated",
        "email": "alice.new@example.com",
    }).
    Where("id = ?", 1).
    Execute()
```

### DELETE

```go
result, err := db.Delete("users").
    Where("id = ?", 1).
    Execute()
```

---

## 🎯 Common Patterns

### Pattern 1: Dynamic Filters

Build queries conditionally:

```go
func searchUsers(db *relica.DB, name string, minAge int) ([]User, error) {
    qb := db.Select("*").From("users")

    if name != "" {
        qb = qb.Where("name LIKE ?", "%"+name+"%")
    }

    if minAge > 0 {
        qb = qb.Where("age >= ?", minAge)
    }

    var users []User
    err := qb.All(&users)
    return users, err
}
```

### Pattern 2: Pagination

```go
func getUsers(db *relica.DB, page, pageSize int) ([]User, error) {
    offset := (page - 1) * pageSize

    var users []User
    err := db.Select("*").
        From("users").
        OrderBy("id ASC").
        Limit(pageSize).
        Offset(offset).
        All(&users)

    return users, err
}

// Usage
users, err := getUsers(db, 1, 20) // Page 1, 20 users per page
```

### Pattern 3: Counting Results

```go
func countUsers(db *relica.DB, status string) (int, error) {
    var count struct {
        Total int `db:"total"`
    }

    err := db.Select("COUNT(*) as total").
        From("users").
        Where("status = ?", status).
        One(&count)

    return count.Total, err
}
```

### Pattern 4: Bulk Insert

```go
func bulkInsertUsers(db *relica.DB, users []User) error {
    batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})

    for _, user := range users {
        batch.Values(user.Name, user.Email, user.Age)
    }

    _, err := batch.Execute()
    return err
}
```

---

## 🔒 Transactions

### Basic Transaction

```go
func transferMoney(db *relica.DB, fromID, toID int, amount float64) error {
    tx, err := db.Begin(context.Background())
    if err != nil {
        return err
    }
    defer tx.Rollback() // Auto-rollback if not committed

    // Deduct from sender
    _, err = tx.Update("accounts").
        Set(map[string]interface{}{"balance": "balance - ?"}).
        Where("id = ?", fromID).
        Execute()
    if err != nil {
        return err
    }

    // Add to receiver
    _, err = tx.Update("accounts").
        Set(map[string]interface{}{"balance": "balance + ?"}).
        Where("id = ?", toID).
        Execute()
    if err != nil {
        return err
    }

    // Commit transaction
    return tx.Commit()
}
```

### Transaction with Isolation Level

```go
tx, err := db.BeginTx(ctx, &relica.TxOptions{
    Isolation: sql.LevelSerializable,
})
```

---

## 🌐 Multi-Database Support

### PostgreSQL

```go
db, err := relica.Open("postgres",
    "postgres://user:pass@localhost:5432/mydb?sslmode=disable")
```

**Placeholders:** `$1, $2, $3`
**Identifiers:** `"users"`, `"table_name"`
**UPSERT:** `ON CONFLICT`

### MySQL

```go
db, err := relica.Open("mysql",
    "user:pass@tcp(localhost:3306)/mydb?parseTime=true")
```

**Placeholders:** `?, ?, ?`
**Identifiers:** `` `users` ``, `` `table_name` ``
**UPSERT:** `ON DUPLICATE KEY UPDATE`

### SQLite

```go
db, err := relica.Open("sqlite", "./mydb.db")
```

**Placeholders:** `?, ?, ?`
**Identifiers:** `"users"`, `"table_name"`
**UPSERT:** `ON CONFLICT`

**Note:** Relica automatically converts `?` to the correct placeholder for your database.

---

## ⚡ Performance Tips

### Tip 1: Use Statement Cache (Automatic)

Relica caches prepared statements automatically:

```go
// First call: prepares statement
db.Select("*").From("users").Where("id = ?", 1).One(&user)

// Subsequent calls: uses cached statement (<60ns lookup)
db.Select("*").From("users").Where("id = ?", 2).One(&user)
```

### Tip 2: Batch Operations

Replace loops with batch operations:

```go
// ❌ Slow (N queries)
for _, user := range users {
    db.Insert("users", map[string]interface{}{
        "name":  user.Name,
        "email": user.Email,
    }).Execute()
}

// ✅ Fast (1 query, 3.3x faster)
batch := db.Builder().BatchInsert("users", []string{"name", "email"})
for _, user := range users {
    batch.Values(user.Name, user.Email)
}
batch.Execute()
```

### Tip 3: Use Context for Timeouts

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := db.Select("*").
    From("users").
    WithContext(ctx).
    All(&users)
```

### Tip 4: Connection Pooling

```go
db, err := relica.Open("postgres", dsn,
    relica.WithMaxOpenConns(25),     // Max concurrent connections
    relica.WithMaxIdleConns(5),      // Idle connections in pool
    relica.WithConnMaxLifetime(300), // 5 minutes
)
```

---

## 🚨 Common Mistakes

### Mistake 1: Missing `db` Tags

```go
// ❌ WRONG: No db tags
type User struct {
    ID    int
    Name  string
    Email string
}

// ✅ CORRECT: db tags present
type User struct {
    ID    int    `db:"id"`
    Name  string `db:"name"`
    Email string `db:"email"`
}
```

### Mistake 2: Forgetting defer Close()

```go
// ❌ WRONG: Connection leak
db, err := relica.Open("postgres", dsn)
// ... use db

// ✅ CORRECT: Always close
db, err := relica.Open("postgres", dsn)
defer db.Close()
```

### Mistake 3: Not Checking Errors

```go
// ❌ WRONG: Ignoring errors
db.Select("*").From("users").All(&users)

// ✅ CORRECT: Check errors
err := db.Select("*").From("users").All(&users)
if err != nil {
    log.Fatal(err)
}
```

### Mistake 4: Using LastInsertId() on PostgreSQL

```go
// ❌ WRONG: PostgreSQL doesn't support LastInsertId with lib/pq
result, _ := db.Insert("users", data).Execute()
id, _ := result.LastInsertId() // ERROR!

// ✅ CORRECT: Use RETURNING clause
var id int
db.QueryRowContext(ctx,
    `INSERT INTO users (name) VALUES ($1) RETURNING id`,
    "Alice",
).Scan(&id)
```

---

## 🔧 Troubleshooting

### Error: "no such table"

**Problem:** Table doesn't exist in database.

**Solution:**
1. Create table manually or use migration tool
2. Verify database connection (check DSN)
3. Check table name spelling

### Error: "sql: Scan error on column index X"

**Problem:** Struct field type doesn't match database column type.

**Solution:**
1. Verify `db` tags match column names
2. Check field types (int vs string, etc.)
3. Use `sql.NullString`, `sql.NullInt64` for nullable columns

### Error: "pq: invalid input syntax for type integer"

**Problem:** Passing wrong type to placeholder.

**Solution:**
1. Verify placeholder values match expected types
2. Convert types before passing: `strconv.Atoi()`, etc.

---

## 📖 Next Steps

### Learn More

1. **[Best Practices Guide](BEST_PRACTICES.md)** - Production-ready patterns
2. **[Advanced Patterns Guide](ADVANCED_PATTERNS.md)** - Complex queries
3. **[Performance Tuning Guide](PERFORMANCE_TUNING.md)** - Optimization tips
4. **[Security Guide](SECURITY.md)** - SQL injection prevention, audit logging

### Explore Features

- **JOINs**: [JOIN Guide](../docs/dev/reports/JOIN_GUIDE.md)
- **Subqueries**: [Subquery Guide](../SUBQUERY_GUIDE.md)
- **CTEs**: [CTE Guide](../CTE_GUIDE.md)
- **Transactions**: [Transaction Guide](../docs/reports/TRANSACTION_IMPLEMENTATION_REPORT.md)

### Community

- **GitHub**: [github.com/coregx/relica](https://github.com/coregx/relica)
- **Issues**: [Report bugs](https://github.com/coregx/relica/issues)
- **Discussions**: [Ask questions](https://github.com/coregx/relica/discussions)

---

## 🎉 You're Ready!

You now know the basics of Relica:
- ✅ Installation and setup
- ✅ CRUD operations
- ✅ Query building
- ✅ Transactions
- ✅ Common patterns
- ✅ Performance tips

**Start building!** Relica is designed to be simple, fast, and safe.

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
