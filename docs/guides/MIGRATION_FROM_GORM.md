# Migration Guide: GORM → Relica

> **Migrating from GORM to Relica** - A Practical Guide
>
> **Last Updated**: 2025-11-13

---

## 📋 Overview

This guide helps you migrate from [GORM](https://gorm.io/) to Relica. We'll cover:

- **Philosophy differences** - ORM vs Query Builder
- **API comparisons** - Side-by-side examples
- **Migration strategies** - Gradual vs complete rewrite
- **Common patterns** - How to translate GORM code to Relica
- **When to use which** - GORM vs Relica trade-offs

---

## 🎯 Quick Comparison

| Feature | GORM | Relica |
|---------|------|--------|
| **Type** | Full ORM | Query Builder |
| **Dependencies** | 10+ (production) | 0 (production) |
| **Model Definition** | Struct tags + hooks | Struct tags only |
| **Migrations** | Built-in | External (e.g., golang-migrate) |
| **Associations** | Auto-loading | Manual JOINs |
| **Query Builder** | Yes | Yes (primary interface) |
| **Raw SQL** | db.Raw() | db.ExecContext() / db.QueryContext() |
| **Performance** | Good | Excellent (zero deps, minimal overhead) |
| **Learning Curve** | Medium | Low |
| **Control** | Abstracted | Explicit |

---

## 🔄 Philosophy Differences

### GORM: Full ORM (Object-Relational Mapping)

```go
// GORM manages relationships, hooks, migrations
type User struct {
    gorm.Model
    Name    string
    Email   string
    Posts   []Post  // Auto-loaded associations
}

// Hooks
func (u *User) BeforeCreate(tx *gorm.DB) error {
    // Auto-executed before INSERT
}

// Auto-migrations
db.AutoMigrate(&User{})

// Preloading associations
db.Preload("Posts").Find(&users)
```

**GORM does a lot for you** - migrations, associations, hooks, soft deletes.

### Relica: Query Builder (Explicit Control)

```go
// Relica focuses on query building, not object mapping
type User struct {
    ID    int    `db:"id"`
    Name  string `db:"name"`
    Email string `db:"email"`
}

// No hooks - use explicit functions
func createUser(db *relica.DB, user *User) error {
    // Your logic before INSERT
    _, err := db.Insert("users", map[string]interface{}{
        "name":  user.Name,
        "email": user.Email,
    }).Execute()
    return err
}

// No auto-migrations - use migration tools
// (golang-migrate, goose, etc.)

// No preloading - use JOINs
db.Builder().
    Select("users.*", "posts.title").
    From("users").
    LeftJoin("posts", "posts.user_id = users.id").
    All(&results)
```

**Relica gives you control** - explicit queries, no magic, no hidden behavior.

---

## 📚 API Migration Guide

### 1. Basic CRUD Operations

#### SELECT - Find All

**GORM:**
```go
var users []User
db.Find(&users)
```

**Relica:**
```go
var users []User
db.Select("*").From("users").All(&users)
// Or shorter:
db.Select("*").From("users").All(&users)
```

#### SELECT - Find by ID

**GORM:**
```go
var user User
db.First(&user, 1)  // WHERE id = 1
```

**Relica:**
```go
var user User
db.Select("*").From("users").Where("id = ?", 1).One(&user)
```

#### SELECT - Find with Conditions

**GORM:**
```go
var users []User
db.Where("age > ?", 18).Find(&users)

// Multiple conditions
db.Where("age > ? AND status = ?", 18, "active").Find(&users)

// Map conditions
db.Where(map[string]interface{}{"name": "Alice", "age": 30}).Find(&users)
```

**Relica:**
```go
var users []User
db.Select("*").From("users").Where("age > ?", 18).All(&users)

// Multiple conditions (chaining)
db.Select("*").
    From("users").
    Where("age > ?", 18).
    Where("status = ?", "active").
    All(&users)

// Expression API (type-safe)
db.Select("*").
    From("users").
    Where(relica.HashExp{"name": "Alice", "age": 30}).
    All(&users)
```

#### INSERT

**GORM:**
```go
user := User{Name: "Alice", Email: "alice@example.com"}
db.Create(&user)
// user.ID is populated after insert
```

**Relica:**
```go
result, err := db.Insert("users", map[string]interface{}{
    "name":  "Alice",
    "email": "alice@example.com",
}).Execute()

// Get ID (PostgreSQL - use RETURNING)
var id int
err := db.QueryRowContext(ctx,
    `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`,
    "Alice", "alice@example.com",
).Scan(&id)
```

#### UPDATE

**GORM:**
```go
db.Model(&user).Update("name", "Alice Updated")

// Multiple fields
db.Model(&user).Updates(map[string]interface{}{
    "name":  "Alice Updated",
    "email": "alice.new@example.com",
})

// Update with conditions
db.Model(&User{}).Where("id = ?", 1).Updates(map[string]interface{}{
    "name": "Alice Updated",
})
```

**Relica:**
```go
// Single field
db.Update("users").
    Set(map[string]interface{}{"name": "Alice Updated"}).
    Where("id = ?", 1).
    Execute()

// Multiple fields
db.Update("users").
    Set(map[string]interface{}{
        "name":  "Alice Updated",
        "email": "alice.new@example.com",
    }).
    Where("id = ?", 1).
    Execute()
```

#### DELETE

**GORM:**
```go
db.Delete(&user, 1)  // WHERE id = 1

// Delete with conditions
db.Where("age < ?", 18).Delete(&User{})
```

**Relica:**
```go
db.Delete("users").Where("id = ?", 1).Execute()

// Delete with conditions
db.Delete("users").Where("age < ?", 18).Execute()
```

---

### 2. Advanced Queries

#### Ordering and Pagination

**GORM:**
```go
var users []User
db.Order("age desc").Limit(10).Offset(20).Find(&users)
```

**Relica:**
```go
var users []User
db.Select("*").
    From("users").
    OrderBy("age DESC").
    Limit(10).
    Offset(20).
    All(&users)
```

#### Aggregates

**GORM:**
```go
var count int64
db.Model(&User{}).Count(&count)

var avg float64
db.Model(&User{}).Select("AVG(age)").Scan(&avg)
```

**Relica:**
```go
var count struct{ Total int `db:"total"` }
db.Select("COUNT(*) as total").From("users").One(&count)

var avg struct{ Avg float64 `db:"avg"` }
db.Select("AVG(age) as avg").From("users").One(&avg)
```

#### GROUP BY and HAVING

**GORM:**
```go
type Result struct {
    Status string
    Count  int
}

var results []Result
db.Model(&User{}).
    Select("status, count(*) as count").
    Group("status").
    Having("count(*) > ?", 10).
    Scan(&results)
```

**Relica:**
```go
type Result struct {
    Status string `db:"status"`
    Count  int    `db:"count"`
}

var results []Result
db.Select("status", "COUNT(*) as count").
    From("users").
    GroupBy("status").
    Having("COUNT(*) > ?", 10).
    All(&results)
```

---

### 3. Associations and JOINs

#### One-to-Many (GORM Preload → Relica JOIN)

**GORM:**
```go
type User struct {
    gorm.Model
    Name  string
    Posts []Post  // Has many
}

type Post struct {
    gorm.Model
    Title  string
    UserID uint
}

// Preload posts
var users []User
db.Preload("Posts").Find(&users)
// Each user.Posts is populated
```

**Relica:**
```go
type UserWithPosts struct {
    UserID    int    `db:"user_id"`
    UserName  string `db:"user_name"`
    PostID    *int   `db:"post_id"`     // Nullable (LEFT JOIN)
    PostTitle *string `db:"post_title"`  // Nullable
}

var results []UserWithPosts
db.Select(
    "users.id as user_id",
    "users.name as user_name",
    "posts.id as post_id",
    "posts.title as post_title",
).
From("users").
LeftJoin("posts", "posts.user_id = users.id").
All(&results)

// Group by user manually
users := groupByUser(results)
```

**Trade-off:**
- GORM: Automatic, but generates N+1 queries (slow) or complex eager loading
- Relica: Manual JOIN, but single query (fast) and explicit control

#### Belongs To (GORM → Relica)

**GORM:**
```go
type Post struct {
    gorm.Model
    Title  string
    UserID uint
    User   User  // Belongs to
}

var posts []Post
db.Preload("User").Find(&posts)
```

**Relica:**
```go
type PostWithUser struct {
    PostID    int    `db:"post_id"`
    PostTitle string `db:"post_title"`
    UserID    int    `db:"user_id"`
    UserName  string `db:"user_name"`
}

var results []PostWithUser
db.Select(
    "posts.id as post_id",
    "posts.title as post_title",
    "users.id as user_id",
    "users.name as user_name",
).
From("posts").
InnerJoin("users", "posts.user_id = users.id").
All(&results)
```

---

### 4. Transactions

**GORM:**
```go
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&user).Error; err != nil {
        return err  // Rollback
    }
    if err := tx.Create(&post).Error; err != nil {
        return err  // Rollback
    }
    return nil  // Commit
})
```

**Relica:**
```go
tx, err := db.Begin(ctx)
if err != nil {
    return err
}
defer tx.Rollback()  // Auto-rollback if not committed

_, err = tx.Insert("users", userData).Execute()
if err != nil {
    return err
}

_, err = tx.Insert("posts", postData).Execute()
if err != nil {
    return err
}

return tx.Commit()
```

---

### 5. Raw SQL

**GORM:**
```go
var users []User
db.Raw("SELECT * FROM users WHERE age > ?", 18).Scan(&users)

// Exec (no result)
db.Exec("UPDATE users SET status = ? WHERE id = ?", "active", 1)
```

**Relica:**
```go
var users []User
rows, err := db.QueryContext(ctx, "SELECT * FROM users WHERE age > ?", 18)
// Manual scanning (or use internal/util helpers)

// Exec (no result)
_, err := db.ExecContext(ctx, "UPDATE users SET status = ? WHERE id = ?", "active", 1)
```

---

## 🔀 Migration Strategies

### Strategy 1: Gradual Migration (Recommended)

Use both GORM and Relica in the same project:

```go
import (
    "gorm.io/gorm"
    "github.com/coregx/relica"
)

type App struct {
    gormDB   *gorm.DB     // For complex associations
    relicaDB *relica.DB   // For performance-critical queries
}

// Use GORM for features you need (migrations, associations)
app.gormDB.AutoMigrate(&User{}, &Post{})

// Use Relica for fast, explicit queries
db.Select("*").
    From("users").
    InnerJoin("posts", "posts.user_id = users.id").
    Where("users.status = ?", "active").
    All(&results)
```

**Benefits:**
- ✅ Migrate one module at a time
- ✅ Keep GORM for migrations and complex associations
- ✅ Use Relica for performance-critical paths
- ✅ Low risk, gradual transition

### Strategy 2: Complete Rewrite

Replace GORM entirely:

**Migration Steps:**

1. **Set up migrations** (replace GORM AutoMigrate):
   ```bash
   go install github.com/pressly/goose/v3/cmd/goose@latest

   # Create migrations
   goose create add_users_table sql
   ```

2. **Remove GORM models** (gorm.Model, associations):
   ```go
   // Before (GORM)
   type User struct {
       gorm.Model
       Name  string
       Posts []Post
   }

   // After (Relica)
   type User struct {
       ID        int       `db:"id"`
       CreatedAt time.Time `db:"created_at"`
       UpdatedAt time.Time `db:"updated_at"`
       Name      string    `db:"name"`
   }
   ```

3. **Replace queries** one by one
4. **Test thoroughly** (behavior may differ)

---

## ⚖️ When to Use Which

### Use GORM When:

✅ **You need built-in migrations**
- GORM AutoMigrate handles schema changes
- Relica requires external migration tools

✅ **You have complex associations**
- Many-to-many, polymorphic associations
- Relica requires manual JOINs

✅ **You want hooks and callbacks**
- BeforeCreate, AfterUpdate, etc.
- Relica has no hooks (use explicit functions)

✅ **You prefer convention over configuration**
- GORM auto-detects table names, primary keys
- Relica is explicit

### Use Relica When:

✅ **You want zero dependencies**
- GORM has 10+ dependencies
- Relica: 0 production dependencies

✅ **Performance is critical**
- Relica: minimal overhead, explicit queries
- GORM: reflection overhead, hidden queries

✅ **You want explicit control**
- See exactly what SQL is generated
- No hidden preloading or lazy loading

✅ **You prefer query builders over models**
- Fluent API, type-safe expressions
- No model definition required

✅ **You're building a new project**
- Start simple, add complexity as needed
- GORM adds complexity upfront

---

## 🚀 Migration Checklist

### Phase 1: Preparation

- [ ] Identify GORM-specific features you use (hooks, associations, migrations)
- [ ] Choose migration strategy (gradual vs complete)
- [ ] Set up migration tool (if replacing AutoMigrate)
- [ ] Add Relica to project: `go get github.com/coregx/relica`

### Phase 2: Basic Queries

- [ ] Migrate simple SELECT queries (Find, First, Where)
- [ ] Migrate INSERT queries (Create)
- [ ] Migrate UPDATE queries (Update, Updates)
- [ ] Migrate DELETE queries (Delete)
- [ ] Test each migrated query

### Phase 3: Advanced Features

- [ ] Replace Preload with JOINs
- [ ] Migrate transactions (db.Transaction → db.Begin/Commit)
- [ ] Replace hooks with explicit functions
- [ ] Migrate aggregates and GROUP BY

### Phase 4: Testing

- [ ] Unit tests for all migrated code
- [ ] Integration tests with real database
- [ ] Performance tests (compare GORM vs Relica)
- [ ] Verify behavior matches (edge cases)

### Phase 5: Deployment

- [ ] Remove GORM dependency (if complete rewrite)
- [ ] Update documentation
- [ ] Monitor production performance
- [ ] Rollback plan ready

---

## 💡 Tips and Tricks

### Tip 1: Use Expression API for Type Safety

Instead of string concatenation:
```go
// ❌ Error-prone
db.Select("*").From("users").Where("name = ?", name)

// ✅ Type-safe
db.Select("*").From("users").Where(relica.Eq("name", name))
```

### Tip 2: Batch Operations for Performance

Replace multiple INSERTs:
```go
// GORM (N queries)
for _, user := range users {
    db.Create(&user)
}

// Relica (1 query, 3.3x faster)
batch := db.Builder().BatchInsert("users", []string{"name", "email"})
for _, user := range users {
    batch.Values(user.Name, user.Email)
}
batch.Execute()
```

### Tip 3: Use Transactions for Data Integrity

```go
// Always wrap related operations in transactions
tx, _ := db.Begin(ctx)
defer tx.Rollback()

// Multiple operations
tx.Insert("users", userData).Execute()
tx.Insert("profiles", profileData).Execute()

tx.Commit()
```

### Tip 4: Leverage Statement Cache

Relica caches prepared statements automatically:
```go
// First call: prepares statement
db.Select("*").From("users").Where("id = ?", 1).One(&user)

// Subsequent calls: uses cached statement (<60ns)
db.Select("*").From("users").Where("id = ?", 2).One(&user)
```

---

## 📖 Additional Resources

- **Relica Documentation**: [github.com/coregx/relica](https://github.com/coregx/relica)
- **GORM Documentation**: [gorm.io](https://gorm.io/)
- **Migration Tools**:
  - [golang-migrate](https://github.com/golang-migrate/migrate)
  - [goose](https://github.com/pressly/goose)
  - [sql-migrate](https://github.com/rubenv/sql-migrate)

---

## ❓ FAQ

**Q: Can I use GORM and Relica together?**
A: Yes! Use GORM for migrations and associations, Relica for performance-critical queries.

**Q: How do I handle migrations without AutoMigrate?**
A: Use external tools like golang-migrate or goose. They provide version control and rollback.

**Q: What about soft deletes?**
A: Implement manually with a `deleted_at` column and WHERE clauses.

**Q: How do I replace GORM hooks?**
A: Write explicit functions:
```go
func createUser(db *relica.DB, user *User) error {
    // Your "before create" logic
    _, err := db.Insert("users", user).Execute()
    // Your "after create" logic
    return err
}
```

**Q: Performance comparison?**
A: Relica is generally faster (zero deps, minimal overhead). See benchmarks in repo.

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
