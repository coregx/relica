# Migration Guide: sqlx → Relica

> **Migrating from sqlx to Relica** - A Practical Guide
>
> **Version**: v0.5.0
> **Last Updated**: 2025-11-13

---

## 📋 Overview

This guide helps you migrate from [sqlx](https://github.com/jmoiron/sqlx) to Relica. We'll cover:

- **Similarities** - Both extend database/sql
- **API comparisons** - Side-by-side examples
- **Migration strategies** - Why and how to switch
- **Advantages** - What Relica adds over sqlx
- **Compatibility** - Using both together

---

## 🎯 Quick Comparison

| Feature | sqlx | Relica |
|---------|------|--------|
| **Type** | database/sql extension | Query builder + database/sql |
| **Dependencies** | 1 (database/sql) | 0 (production) |
| **Struct Scanning** | ✅ StructScan, Select | ✅ All(), One() |
| **Query Builder** | ❌ No (raw SQL only) | ✅ Fluent API |
| **Named Queries** | ✅ NamedExec, NamedQuery | ❌ No (use builder) |
| **IN Clause** | ✅ In() helper | ✅ Expression API |
| **Transactions** | ✅ Beginx() | ✅ Begin() |
| **Prepared Statements** | Manual | ✅ Auto-cached (LRU) |
| **Multi-Database** | ✅ PostgreSQL, MySQL, SQLite | ✅ Same |
| **Performance** | Excellent | Excellent + cache |
| **Learning Curve** | Low | Low |

---

## 🔄 Philosophy Similarity

**Both sqlx and Relica:**
- ✅ Extend database/sql (not replace it)
- ✅ Support struct scanning
- ✅ Keep SQL close to the code
- ✅ Minimal abstraction overhead
- ✅ Multi-database support

**Key Difference:**
- **sqlx**: Raw SQL with struct scanning
- **Relica**: Query builder + struct scanning + statement cache

---

## 📚 API Migration Guide

### 1. Connection Setup

#### Opening a Database

**sqlx:**
```go
import "github.com/jmoiron/sqlx"
import _ "github.com/lib/pq"

db, err := sqlx.Connect("postgres", dsn)
defer db.Close()

// Or with existing *sql.DB
sqlDB, _ := sql.Open("postgres", dsn)
db := sqlx.NewDb(sqlDB, "postgres")
```

**Relica:**
```go
import "github.com/coregx/relica"
import _ "github.com/lib/pq"

db, err := relica.Open("postgres", dsn)
defer db.Close()

// Or wrap existing *sql.DB
sqlDB, _ := sql.Open("postgres", dsn)
db := relica.WrapDB(sqlDB, "postgres")
```

**Migration:** Nearly identical - just change package name!

---

### 2. Basic Queries

#### SELECT - Single Row

**sqlx:**
```go
var user User
err := db.Get(&user, "SELECT * FROM users WHERE id = $1", 1)
```

**Relica:**
```go
var user User
err := db.Select("*").From("users").Where("id = ?", 1).One(&user)

// Or raw SQL (like sqlx)
row := db.QueryRowContext(ctx, "SELECT * FROM users WHERE id = ?", 1)
// Manual scanning
```

**Migration:**
- `db.Get()` → `db.Select().From().Where().One()`
- Or keep raw SQL with QueryRowContext

#### SELECT - Multiple Rows

**sqlx:**
```go
var users []User
err := db.Select(&users, "SELECT * FROM users WHERE age > $1", 18)
```

**Relica:**
```go
var users []User
err := db.Select("*").From("users").Where("age > ?", 18).All(&users)

// Or raw SQL (like sqlx)
rows, err := db.QueryContext(ctx, "SELECT * FROM users WHERE age > ?", 18)
// Manual scanning
```

**Migration:**
- `db.Select()` → `db.Select().From().Where().All()`
- Column selection explicit in builder

---

### 3. INSERT, UPDATE, DELETE

#### INSERT

**sqlx:**
```go
result, err := db.Exec(
    "INSERT INTO users (name, email) VALUES ($1, $2)",
    "Alice", "alice@example.com",
)
```

**Relica:**
```go
// Builder (safer, dialect-aware)
result, err := db.Insert("users", map[string]interface{}{
    "name":  "Alice",
    "email": "alice@example.com",
}).Execute()

// Or raw SQL (like sqlx)
result, err := db.ExecContext(ctx,
    "INSERT INTO users (name, email) VALUES (?, ?)",
    "Alice", "alice@example.com",
)
```

**Migration:**
- Use builder for automatic placeholder conversion (`?` → `$1` for PostgreSQL)
- Raw SQL still works (ExecContext)

#### UPDATE

**sqlx:**
```go
result, err := db.Exec(
    "UPDATE users SET name = $1 WHERE id = $2",
    "Alice Updated", 1,
)
```

**Relica:**
```go
// Builder
result, err := db.Update("users").
    Set(map[string]interface{}{"name": "Alice Updated"}).
    Where("id = ?", 1).
    Execute()

// Or raw SQL
result, err := db.ExecContext(ctx,
    "UPDATE users SET name = ? WHERE id = ?",
    "Alice Updated", 1,
)
```

#### DELETE

**sqlx:**
```go
result, err := db.Exec("DELETE FROM users WHERE id = $1", 1)
```

**Relica:**
```go
// Builder
result, err := db.Delete("users").Where("id = ?", 1).Execute()

// Or raw SQL
result, err := db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", 1)
```

---

### 4. Named Queries

#### sqlx Named Queries

**sqlx:**
```go
query := `INSERT INTO users (name, email) VALUES (:name, :email)`

_, err := db.NamedExec(query, map[string]interface{}{
    "name":  "Alice",
    "email": "alice@example.com",
})

// Or with struct
user := User{Name: "Alice", Email: "alice@example.com"}
_, err := db.NamedExec(query, &user)
```

**Relica:**
```go
// Use builder (no named params needed)
_, err := db.Insert("users", map[string]interface{}{
    "name":  "Alice",
    "email": "alice@example.com",
}).Execute()

// Or use standard placeholders
_, err := db.ExecContext(ctx,
    "INSERT INTO users (name, email) VALUES (?, ?)",
    "Alice", "alice@example.com",
)
```

**Migration:**
- Relica doesn't support named params (`:name`, `:email`)
- Use builder API instead (cleaner and type-safe)
- Or convert to positional params (`?`)

---

### 5. IN Clause

#### sqlx IN Helper

**sqlx:**
```go
import "github.com/jmoiron/sqlx"

query, args, err := sqlx.In(
    "SELECT * FROM users WHERE id IN (?)",
    []int{1, 2, 3},
)
query = db.Rebind(query)  // Convert ? to $1, $2, $3 for PostgreSQL
err = db.Select(&users, query, args...)
```

**Relica:**
```go
// Expression API (type-safe)
var users []User
err := db.Select("*").
    From("users").
    Where(relica.In("id", 1, 2, 3)).
    All(&users)

// Or HashExp with slice
err := db.Select("*").
    From("users").
    Where(relica.HashExp{"id": []interface{}{1, 2, 3}}).
    All(&users)

// Automatically converts to: WHERE id IN ($1, $2, $3)
```

**Migration:**
- `sqlx.In()` + `Rebind()` → `relica.In()` or `HashExp`
- Relica handles dialect conversion automatically

---

### 6. Transactions

**sqlx:**
```go
tx, err := db.Beginx()
if err != nil {
    return err
}
defer tx.Rollback()

_, err = tx.Exec("INSERT INTO users (name) VALUES ($1)", "Alice")
if err != nil {
    return err
}

_, err = tx.Exec("UPDATE accounts SET balance = balance - $1", 100)
if err != nil {
    return err
}

return tx.Commit()
```

**Relica:**
```go
tx, err := db.Begin(ctx)
if err != nil {
    return err
}
defer tx.Rollback()

_, err = tx.Insert("users", map[string]interface{}{"name": "Alice"}).Execute()
if err != nil {
    return err
}

_, err = tx.Update("accounts").
    Set(map[string]interface{}{"balance": "balance - ?"}).
    Execute()
if err != nil {
    return err
}

return tx.Commit()
```

**Migration:**
- `db.Beginx()` → `db.Begin(ctx)`
- Transaction API nearly identical
- Use builder methods on `tx` instead of raw SQL

---

### 7. Context Support

**sqlx:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Context methods
err := db.GetContext(ctx, &user, "SELECT * FROM users WHERE id = $1", 1)
err = db.SelectContext(ctx, &users, "SELECT * FROM users")
_, err = db.ExecContext(ctx, "INSERT INTO users (name) VALUES ($1)", "Alice")
```

**Relica:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Context in builder
err := db.Select("*").
    From("users").
    Where("id = ?", 1).
    WithContext(ctx).
    One(&user)

// Or context on query level
err := db.ExecContext(ctx, "INSERT INTO users (name) VALUES (?)", "Alice")
```

**Migration:**
- sqlx: Context methods (`GetContext`, `SelectContext`)
- Relica: `.WithContext(ctx)` in builder or `...Context()` methods

---

## 🚀 Why Migrate from sqlx to Relica?

### Advantage 1: Query Builder (No String Concatenation)

**sqlx (manual string building):**
```go
query := "SELECT * FROM users WHERE 1=1"
args := []interface{}{}

if name != "" {
    query += " AND name = ?"
    args = append(args, name)
}

if age > 0 {
    query += " AND age > ?"
    args = append(args, age)
}

err := db.Select(&users, query, args...)
```

**Relica (fluent builder):**
```go
qb := db.Select("*").From("users")

if name != "" {
    qb = qb.Where("name = ?", name)
}

if age > 0 {
    qb = qb.Where("age > ?", age)
}

err := qb.All(&users)
```

### Advantage 2: Statement Cache (Automatic)

**sqlx:**
```go
// Manual prepared statement caching
stmt, err := db.Preparex("SELECT * FROM users WHERE id = $1")
defer stmt.Close()

// Reuse statement
var user User
err = stmt.Get(&user, 1)
err = stmt.Get(&user, 2)  // Faster (reuses prepared statement)
```

**Relica:**
```go
// Automatic LRU caching (<60ns hit latency)
err := db.Select("*").From("users").Where("id = ?", 1).One(&user)
err = db.Select("*").From("users").Where("id = ?", 2).One(&user)  // Auto-cached!
```

**Benefit:** Relica caches 1000 prepared statements automatically (configurable).

### Advantage 3: Dialect Awareness

**sqlx:**
```go
// Must manually handle placeholders
query := "SELECT * FROM users WHERE id = ?"

// PostgreSQL requires $1, not ?
query = db.Rebind(query)  // Converts ? to $1
```

**Relica:**
```go
// Automatic placeholder conversion
db.Select("*").From("users").Where("id = ?", 1)
// PostgreSQL: WHERE id = $1
// MySQL: WHERE id = ?
// SQLite: WHERE id = ?
```

### Advantage 4: Type-Safe Expressions

**sqlx:**
```go
// String-based (error-prone)
err := db.Select(&users, "SELECT * FROM users WHERE age > ? AND status = ?", 18, "active")
```

**Relica:**
```go
// Expression API (type-safe)
err := db.Select("*").
    From("users").
    Where(relica.And(
        relica.GreaterThan("age", 18),
        relica.Eq("status", "active"),
    )).
    All(&users)
```

### Advantage 5: Zero Dependencies

**sqlx:**
- Depends on: database/sql (standard library)

**Relica:**
- **Zero production dependencies** (only database/sql from stdlib)
- No external packages in production
- Smaller binary, fewer security risks

---

## 🔀 Migration Strategies

### Strategy 1: Drop-In Replacement (Easy)

Replace sqlx queries with Relica one-by-one:

```go
// Before (sqlx)
err := db.Get(&user, "SELECT * FROM users WHERE id = $1", 1)

// After (Relica)
err := db.Select("*").From("users").Where("id = ?", 1).One(&user)
```

**Benefits:**
- ✅ Low risk, gradual migration
- ✅ Test each change individually
- ✅ No big-bang rewrite

### Strategy 2: Use Both Together

sqlx and Relica can coexist:

```go
import (
    "github.com/jmoiron/sqlx"
    "github.com/coregx/relica"
)

// Open once, wrap both
sqlDB, _ := sql.Open("postgres", dsn)

sqlxDB := sqlx.NewDb(sqlDB, "postgres")
relicaDB := relica.WrapDB(sqlDB, "postgres")

// Use sqlx for simple queries
sqlxDB.Get(&user, "SELECT * FROM users WHERE id = $1", 1)

// Use Relica for complex queries
relicaDB.Select("*").
    From("users").
    InnerJoin("posts", "posts.user_id = users.id").
    Where("users.status = ?", "active").
    All(&results)
```

**Benefits:**
- ✅ Best of both worlds
- ✅ Migrate gradually
- ✅ Keep familiar sqlx patterns where needed

---

## 📋 Migration Checklist

### Phase 1: Evaluation

- [ ] Identify complex queries that benefit from builder (JOINs, dynamic WHERE)
- [ ] Check if you use sqlx-specific features (NamedQuery, In)
- [ ] Review performance requirements (statement caching)
- [ ] Add Relica: `go get github.com/coregx/relica`

### Phase 2: Simple Queries

- [ ] Migrate `db.Get()` to `.Select().From().One()`
- [ ] Migrate `db.Select()` to `.Select().From().All()`
- [ ] Migrate `db.Exec()` to `.Insert()`, `.Update()`, `.Delete()`
- [ ] Test each migrated query

### Phase 3: Advanced Queries

- [ ] Replace `sqlx.In()` with `relica.In()` or `HashExp`
- [ ] Migrate `NamedExec` to builder API
- [ ] Migrate dynamic query building to fluent API
- [ ] Replace manual prepared statements with auto-cached queries

### Phase 4: Testing

- [ ] Unit tests for all migrated code
- [ ] Integration tests with real database
- [ ] Performance tests (compare sqlx vs Relica)
- [ ] Verify struct scanning works identically

### Phase 5: Optimization

- [ ] Use statement cache metrics to tune capacity
- [ ] Replace string concatenation with builder
- [ ] Use Expression API for type safety
- [ ] Monitor cache hit rate

---

## 💡 Tips and Tricks

### Tip 1: Reuse sqlx Struct Tags

sqlx and Relica use the same `db` tag:

```go
type User struct {
    ID    int    `db:"id"`      // Works in both
    Name  string `db:"name"`    // Works in both
    Email string `db:"email"`   // Works in both
}

// sqlx
db.Get(&user, "SELECT * FROM users WHERE id = $1", 1)

// Relica (same struct!)
db.Select("*").From("users").Where("id = ?", 1).One(&user)
```

### Tip 2: Gradual Builder Adoption

Start with raw SQL, move to builder when beneficial:

```go
// Phase 1: Keep raw SQL (like sqlx)
db.ExecContext(ctx, "INSERT INTO users (name) VALUES (?)", "Alice")

// Phase 2: Adopt builder for complex queries
db.Builder().
    Select("*").
    From("users").
    InnerJoin("posts", "posts.user_id = users.id").
    Where(relica.And(
        relica.Eq("users.status", 1),
        relica.GreaterThan("posts.views", 1000),
    )).
    All(&results)
```

### Tip 3: Batch Operations (Relica Advantage)

sqlx doesn't have batch helpers, Relica does:

```go
// sqlx (manual loop, N queries)
for _, user := range users {
    db.Exec("INSERT INTO users (name, email) VALUES ($1, $2)", user.Name, user.Email)
}

// Relica (single query, 3.3x faster)
batch := db.Builder().BatchInsert("users", []string{"name", "email"})
for _, user := range users {
    batch.Values(user.Name, user.Email)
}
batch.Execute()
```

### Tip 4: Use Query Builder for JOINs

sqlx requires raw SQL for JOINs, Relica has fluent API:

```go
// sqlx (raw SQL)
query := `
    SELECT users.*, posts.title
    FROM users
    INNER JOIN posts ON posts.user_id = users.id
    WHERE users.status = $1
`
db.Select(&results, query, "active")

// Relica (builder)
db.Select("users.*", "posts.title").
    From("users").
    InnerJoin("posts", "posts.user_id = users.id").
    Where("users.status = ?", "active").
    All(&results)
```

---

## ⚖️ When to Keep sqlx vs Migrate to Relica

### Keep sqlx When:

✅ **You only use simple queries**
- Basic SELECT, INSERT, UPDATE, DELETE
- No dynamic query building

✅ **You heavily rely on NamedQuery**
- Lots of `:name`, `:email` params
- Would require significant refactoring

✅ **Your team prefers raw SQL**
- sqlx is closer to database/sql
- Less abstraction

### Migrate to Relica When:

✅ **You build dynamic queries**
- Conditional WHERE clauses
- Dynamic JOIN logic
- Filter combinations

✅ **Performance is critical**
- Automatic statement caching (no manual Preparex)
- Batch operations (3.3x faster)

✅ **You want zero dependencies**
- Smaller binaries
- Fewer security vulnerabilities

✅ **You want type-safe expressions**
- Avoid string concatenation errors
- Compile-time safety

---

## 📖 Code Examples

### Example 1: Dynamic Search Query

**sqlx:**
```go
func searchUsers(db *sqlx.DB, name string, minAge int) ([]User, error) {
    query := "SELECT * FROM users WHERE 1=1"
    args := []interface{}{}

    if name != "" {
        query += " AND name LIKE ?"
        args = append(args, "%"+name+"%")
    }

    if minAge > 0 {
        query += " AND age >= ?"
        args = append(args, minAge)
    }

    var users []User
    err := db.Select(&users, db.Rebind(query), args...)
    return users, err
}
```

**Relica:**
```go
func searchUsers(db *relica.DB, name string, minAge int) ([]User, error) {
    qb := db.Select("*").From("users")

    if name != "" {
        qb = qb.Where(relica.Like("name", name))  // Auto % wrapping
    }

    if minAge > 0 {
        qb = qb.Where(relica.GreaterOrEqual("age", minAge))
    }

    var users []User
    err := qb.All(&users)
    return users, err
}
```

**Benefits:** Cleaner, type-safe, no manual string building.

### Example 2: Bulk Insert

**sqlx:**
```go
func bulkInsert(db *sqlx.DB, users []User) error {
    tx, _ := db.Beginx()
    defer tx.Rollback()

    stmt, _ := tx.Preparex("INSERT INTO users (name, email) VALUES ($1, $2)")
    defer stmt.Close()

    for _, user := range users {
        _, err := stmt.Exec(user.Name, user.Email)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

**Relica:**
```go
func bulkInsert(db *relica.DB, users []User) error {
    batch := db.Builder().BatchInsert("users", []string{"name", "email"})

    for _, user := range users {
        batch.Values(user.Name, user.Email)
    }

    _, err := batch.Execute()
    return err
}
```

**Benefits:** Simpler, faster (3.3x), automatic transaction handling.

---

## 📊 Performance Comparison

| Operation | sqlx | Relica | Speedup |
|-----------|------|--------|---------|
| Single SELECT (uncached) | ~10ms | ~10ms | 1x (same) |
| Single SELECT (cached) | ~10ms | ~60ns | **166,666x** |
| Bulk INSERT (100 rows) | ~1s | ~300ms | **3.3x** |
| Bulk UPDATE (100 rows) | ~2.5s | ~2s | **1.25x** |

**Note:** Cached queries reuse prepared statements (<60ns lookup).

---

## 📖 Additional Resources

- **Relica Documentation**: [github.com/coregx/relica](https://github.com/coregx/relica)
- **sqlx Documentation**: [github.com/jmoiron/sqlx](https://github.com/jmoiron/sqlx)
- **database/sql Guide**: [go.dev/doc/database/sql](https://go.dev/doc/database/sql)

---

## ❓ FAQ

**Q: Can I use both sqlx and Relica together?**
A: Yes! Wrap the same `*sql.DB` with both:
```go
sqlDB, _ := sql.Open("postgres", dsn)
sqlxDB := sqlx.NewDb(sqlDB, "postgres")
relicaDB := relica.WrapDB(sqlDB, "postgres")
```

**Q: Do I need to change my struct tags?**
A: No! Both use `db:"column_name"` tags.

**Q: What about NamedQuery in Relica?**
A: Relica doesn't support named params (`:name`). Use builder API or convert to `?` placeholders.

**Q: Performance difference?**
A: Relica is faster for repeated queries (statement cache) and bulk operations (batch API).

**Q: Can I still write raw SQL?**
A: Yes! Use `db.ExecContext()` and `db.QueryContext()` like database/sql.

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
