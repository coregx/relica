# AGENTS.md

> **Instructions for AI coding agents working with Relica.**
> **Read this BEFORE generating any code.**

---

## API Priority (MUST FOLLOW)

### 1. RECOMMENDED: Model() API for CRUD

**ALWAYS use Model() API for struct-based operations:**

```go
// INSERT - CORRECT
user := User{Name: "Alice", Email: "alice@example.com"}
err := db.Model(&user).Insert()
// user.ID is auto-populated!

// UPDATE - CORRECT
user.Name = "Alice Updated"
err := db.Model(&user).Update()

// DELETE - CORRECT
err := db.Model(&user).Delete()

// SELECTIVE INSERT - CORRECT
err := db.Model(&user).Insert("name", "email") // Only these fields
```

### 2. RECOMMENDED: Expression API for WHERE

**ALWAYS use typed expressions for conditions:**

```go
// CORRECT - Type-safe expressions
db.Select("*").From("users").
    Where(relica.Eq("status", 1)).
    Where(relica.GreaterThan("age", 18)).
    All(&users)

// CORRECT - HashExp for simple equality
db.Select("*").From("users").
    Where(relica.HashExp{"status": "active", "role": "admin"}).
    All(&users)

// CORRECT - Logical combinators
db.Select("*").From("users").
    Where(relica.And(
        relica.Eq("status", 1),
        relica.Or(
            relica.Eq("role", "admin"),
            relica.GreaterThan("age", 30),
        ),
    )).
    All(&users)

// CORRECT - IN clause
db.Select("*").From("users").
    Where(relica.In("status", 1, 2, 3)).
    All(&users)

// CORRECT - LIKE with escaping
db.Select("*").From("users").
    Where(relica.Like("name", "john")).
    All(&users)

// CORRECT - BETWEEN
db.Select("*").From("orders").
    Where(relica.Between("created_at", start, end)).
    All(&orders)
```

### 3. FALLBACK ONLY: Raw Strings

**Use ONLY when dynamic SQL is absolutely required:**

```go
// ACCEPTABLE - Simple parameterized query
db.Select("*").From("users").
    Where("id = ?", userID).
    One(&user)

// ACCEPTABLE - Complex custom SQL
db.Select("*").From("users").
    Where("LOWER(email) = LOWER(?)", email).
    All(&users)
```

### 4. AVOID: map[string]interface{}

**DO NOT use map[string]interface{} for CRUD operations!**

```go
// WRONG - Don't do this!
db.Insert("users", map[string]interface{}{
    "name":  "Alice",
    "email": "alice@example.com",
}).Execute()

// CORRECT - Use Model() API instead
user := User{Name: "Alice", Email: "alice@example.com"}
db.Model(&user).Insert()
```

**map[string]interface{} is acceptable ONLY for:**
- Dynamic data from external sources (JSON API payloads)
- Schema-less operations where struct is not available
- Migration scripts with unknown column sets

---

## Expression API Reference

### Comparison Operators

| Function | SQL | Example |
|----------|-----|---------|
| `Eq(col, val)` | `col = ?` | `Eq("status", 1)` |
| `NotEq(col, val)` | `col != ?` | `NotEq("status", 0)` |
| `GreaterThan(col, val)` | `col > ?` | `GreaterThan("age", 18)` |
| `LessThan(col, val)` | `col < ?` | `LessThan("price", 100)` |
| `GreaterOrEqual(col, val)` | `col >= ?` | `GreaterOrEqual("score", 70)` |
| `LessOrEqual(col, val)` | `col <= ?` | `LessOrEqual("qty", 10)` |

### Set Operators

| Function | SQL | Example |
|----------|-----|---------|
| `In(col, vals...)` | `col IN (?, ?, ?)` | `In("status", 1, 2, 3)` |
| `NotIn(col, vals...)` | `col NOT IN (?, ?)` | `NotIn("role", "guest")` |
| `Between(col, a, b)` | `col BETWEEN ? AND ?` | `Between("age", 18, 65)` |

### String Operators

| Function | SQL | Example |
|----------|-----|---------|
| `Like(col, pattern)` | `col LIKE '%pattern%'` | `Like("name", "john")` |
| `OrLike(col, patterns...)` | `col LIKE ? OR col LIKE ?` | `OrLike("email", "gmail", "yahoo")` |

### Logical Operators

| Function | SQL | Example |
|----------|-----|---------|
| `And(exprs...)` | `(expr1) AND (expr2)` | `And(Eq("a", 1), Eq("b", 2))` |
| `Or(exprs...)` | `(expr1) OR (expr2)` | `Or(Eq("role", "admin"), Eq("role", "mod"))` |
| `Not(expr)` | `NOT (expr)` | `Not(In("status", 0, 99))` |

### HashExp (Simple Equality)

```go
// Multiple equalities (AND)
HashExp{"status": 1, "active": true}
// → status = 1 AND active = true

// NULL check
HashExp{"deleted_at": nil}
// → deleted_at IS NULL

// IN clause (slice)
HashExp{"status": []interface{}{1, 2, 3}}
// → status IN (1, 2, 3)
```

---

## Complete Examples

### Example 1: User Registration

```go
// CORRECT
func CreateUser(db *relica.DB, name, email string) (*User, error) {
    user := &User{
        Name:      name,
        Email:     email,
        Status:    "pending",
        CreatedAt: time.Now(),
    }

    if err := db.Model(user).Insert(); err != nil {
        return nil, fmt.Errorf("insert user: %w", err)
    }

    return user, nil // user.ID is auto-populated
}
```

### Example 2: Query with Conditions

```go
// CORRECT
func FindActiveAdmins(db *relica.DB, minAge int) ([]User, error) {
    var users []User

    err := db.Select("*").From("users").
        Where(relica.And(
            relica.Eq("status", "active"),
            relica.Eq("role", "admin"),
            relica.GreaterOrEqual("age", minAge),
        )).
        OrderBy("name ASC").
        All(&users)

    if err != nil {
        return nil, fmt.Errorf("find active admins: %w", err)
    }

    return users, nil
}
```

### Example 3: Update with Transaction

```go
// CORRECT
func UpdateUserStatus(db *relica.DB, userID int, status string) error {
    return db.Transactional(func(tx *relica.Tx) error {
        var user User

        // Find user
        err := tx.Builder().Select("*").From("users").
            Where(relica.Eq("id", userID)).
            One(&user)
        if err != nil {
            return fmt.Errorf("find user: %w", err)
        }

        // Update
        user.Status = status
        user.UpdatedAt = time.Now()

        return tx.Model(&user).Update("status", "updated_at")
    })
}
```

### Example 4: Search with LIKE

```go
// CORRECT
func SearchUsers(db *relica.DB, query string) ([]User, error) {
    var users []User

    err := db.Select("*").From("users").
        Where(relica.Or(
            relica.Like("name", query),
            relica.Like("email", query),
        )).
        Where(relica.Eq("status", "active")).
        Limit(20).
        All(&users)

    return users, err
}
```

---

## Anti-Patterns (DO NOT USE)

### Anti-Pattern 1: map for INSERT

```go
// WRONG
db.Insert("users", map[string]interface{}{
    "name": name,
    "email": email,
}).Execute()

// CORRECT
user := User{Name: name, Email: email}
db.Model(&user).Insert()
```

### Anti-Pattern 2: String concatenation for WHERE

```go
// WRONG - SQL injection risk!
db.Select("*").From("users").
    Where("name = '" + name + "'").
    All(&users)

// CORRECT - Parameterized
db.Select("*").From("users").
    Where(relica.Eq("name", name)).
    All(&users)
```

### Anti-Pattern 3: map for UPDATE

```go
// WRONG
db.Update("users").
    Set(map[string]interface{}{"status": "active"}).
    Where("id = ?", id).
    Execute()

// CORRECT - Load, modify, save
var user User
db.Select("*").From("users").Where(relica.Eq("id", id)).One(&user)
user.Status = "active"
db.Model(&user).Update("status")
```

---

## Summary

| Operation | RECOMMENDED | AVOID |
|-----------|-------------|-------|
| INSERT | `db.Model(&struct).Insert()` | `db.Insert(table, map)` |
| UPDATE | `db.Model(&struct).Update()` | `db.Update(table).Set(map)` |
| DELETE | `db.Model(&struct).Delete()` | - |
| WHERE | `relica.Eq()`, `relica.And()`, `HashExp{}` | String concatenation |
| Complex WHERE | `relica.And(relica.Or(...))` | Nested string conditions |

---

*This guide is optimized for AI code generation accuracy.*
