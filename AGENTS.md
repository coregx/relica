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

// UPSERT - CORRECT (INSERT or UPDATE on conflict)
err := db.Model(&user).Upsert()              // all non-PK fields
err = db.Model(&user).Upsert("name", "email") // selective fields

// UPDATE CHANGED - CORRECT (only modified fields)
original := user
user.Name = "Updated"
err = db.Model(&user).UpdateChanged(&original)

// SELECTIVE INSERT - CORRECT
err = db.Model(&user).Insert("name", "email") // Only these fields
```

### 2. RECOMMENDED: Expression API for WHERE

**ALWAYS use typed expressions for conditions:**

```go
// CORRECT - Type-safe expressions
db.Select().From("users").
    Where(relica.Eq("status", 1)).
    Where(relica.GreaterThan("age", 18)).
    All(&users)

// CORRECT - HashExp for simple equality
db.Select().From("users").
    Where(relica.HashExp{"status": "active", "role": "admin"}).
    All(&users)

// CORRECT - Logical combinators
db.Select().From("users").
    Where(relica.And(
        relica.Eq("status", 1),
        relica.Or(
            relica.Eq("role", "admin"),
            relica.GreaterThan("age", 30),
        ),
    )).
    All(&users)

// CORRECT - IN clause
db.Select().From("users").
    Where(relica.In("status", 1, 2, 3)).
    All(&users)

// CORRECT - LIKE with escaping
db.Select().From("users").
    Where(relica.Like("name", "john")).
    All(&users)

// CORRECT - BETWEEN
db.Select().From("orders").
    Where(relica.Between("created_at", start, end)).
    All(&orders)

// CORRECT - Column-to-column comparison (for JOINs and correlated subqueries)
db.Select().From("orders o").
    Where(relica.EqCol("o.user_id", "u.id")).
    All(&orders)

// CORRECT - Type-safe scalar subquery in SELECT
sub := db.Select("COUNT(*)").From("orders").
    Where(relica.EqCol("orders.user_id", "users.id"))
db.Select("id", "name").
    SelectSub(sub.AsExpression(), "order_count").
    From("users").All(&results)
```

### 3. ACCEPTABLE: Named Placeholders

**Use `{:name}` syntax with `relica.Params` for readable parameterized queries:**

```go
// Named parameters - readable and safe
db.Select().From("users").
    Where("id = {:id} AND status = {:status}", relica.Params{
        "id":     userID,
        "status": "active",
    }).
    All(&users)

// Same parameter reused
db.Select().From("categories").
    Where("parent_id = {:id} OR id = {:id}", relica.Params{"id": catID}).
    All(&categories)
```

Works in `Where`, `AndWhere`, `OrWhere` on Select, Update, and Delete.

### 4. FALLBACK ONLY: Positional Placeholders

**Use ONLY when named params or expressions don't fit:**

```go
// ACCEPTABLE - Simple positional query
db.Select().From("users").
    Where("id = ?", userID).
    One(&user)

// ACCEPTABLE - Complex custom SQL
db.Select().From("users").
    Where("LOWER(email) = LOWER(?)", email).
    All(&users)
```

### 5. AVOID: map[string]interface{}

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

## JOIN Operations

**All JOIN types are supported. Use `EqCol()` for type-safe ON conditions:**

```go
// INNER JOIN
db.Select("u.name", "o.total").
    From("users u").
    InnerJoin("orders o", "o.user_id = u.id").
    All(&results)

// LEFT JOIN (include users without orders)
db.Select("u.name", "COALESCE(SUM(o.total), 0) as total").
    From("users u").
    LeftJoin("orders o", "o.user_id = u.id").
    GroupBy("u.id", "u.name").
    All(&results)

// Multiple JOINs
db.Select("u.name", "o.total", "p.name as product").
    From("users u").
    InnerJoin("orders o", "o.user_id = u.id").
    InnerJoin("products p", "p.id = o.product_id").
    Where(relica.Eq("o.status", "paid")).
    All(&results)

// JOIN with Expression API (complex ON conditions)
db.Select().
    From("messages m").
    InnerJoin("users u", relica.And(
        relica.NewExp("m.user_id = u.id"),
        relica.GreaterThan("u.status", 0),
    )).
    All(&results)

// JOIN with EqCol (type-safe column-to-column)
db.Select("u.name", "o.total").
    From("users u").
    InnerJoin("orders o", relica.EqCol("o.user_id", "u.id")).
    All(&results)
```

### All JOIN Types

| Method | SQL | Notes |
|--------|-----|-------|
| `InnerJoin(table, on)` | `INNER JOIN` | Most common |
| `LeftJoin(table, on)` | `LEFT OUTER JOIN` | Include unmatched left rows |
| `RightJoin(table, on)` | `RIGHT OUTER JOIN` | Include unmatched right rows |
| `FullJoin(table, on)` | `FULL OUTER JOIN` | PostgreSQL, SQLite only (not MySQL) |
| `CrossJoin(table)` | `CROSS JOIN` | Cartesian product, no ON |

The `on` parameter accepts:
- `string` — raw SQL condition: `"o.user_id = u.id"`
- `Expression` — type-safe: `relica.EqCol("o.user_id", "u.id")`
- `relica.And(...)` / `relica.Or(...)` — complex conditions

---

## Generic API (Go 1.21+)

**Type-safe scan without manual variable declarations:**

```go
// One[T] — scan single row into struct
user, err := relica.One[User](db.Select().From("users").Where(relica.Eq("id", 1)))

// All[T] — scan multiple rows into slice
users, err := relica.All[User](db.Select().From("users").OrderBy("name"))

// Scalar[T] — scan single value (COUNT, MAX, etc.)
count, err := relica.Scalar[int64](db.Select("COUNT(*)").From("users"))
total, err := relica.Scalar[float64](db.Select("SUM(total)").From("orders"))
name, err := relica.Scalar[string](db.Select("name").From("users").Where(relica.Eq("id", 1)))
```

**Important**: Generic functions are free functions in the `relica` package, not methods on SelectQuery.

---

## Batch Operations

```go
// BatchInsert — multi-row INSERT (3.3x faster than loop)
batch := db.BatchInsert("users", []string{"name", "email"})
for _, u := range users {
    batch.Values(u.Name, u.Email)
}
_, err := batch.Execute()

// BatchInsertStruct — batch from structs (auto-detects columns)
users := []User{{Name: "Alice"}, {Name: "Bob"}}
_, err := db.BatchInsertStruct("users", users)

// BatchUpdate — multi-row UPDATE
batch := db.BatchUpdate("users", []string{"name", "status"}, "id")
for _, u := range users {
    batch.Values(u.Name, u.Status, u.ID) // last value = WHERE id = ?
}
_, err := batch.Execute()

// InsertStruct — single struct INSERT (auto-detects columns, skips zero PK)
_, err := db.InsertStruct("users", user)
```

All batch operations work on both `DB` and `Tx`.

---

## Transactions

```go
// Transactional() — auto commit/rollback with panic recovery
err := db.Transactional(ctx, func(tx *relica.Tx) error {
    user := User{Name: "Alice"}
    if err := tx.Model(&user).Insert(); err != nil {
        return err // auto-rollback
    }

    order := Order{UserID: user.ID, Total: 99.99}
    if err := tx.Model(&order).Insert(); err != nil {
        return err // auto-rollback
    }

    return nil // auto-commit
})

// Manual transaction control
tx, err := db.Begin(ctx)
defer tx.Rollback() // safe: no-op after Commit

err = tx.Model(&user).Insert()
if err != nil {
    return err // auto-rollback via defer
}
return tx.Commit()
```

**All methods available on Tx**: Select, Insert, Update, Delete, Model, BatchInsert, BatchUpdate, Upsert, InsertStruct, BatchInsertStruct, NewQuery.

---

## Ordering with Expressions

```go
// OrderBySub — type-safe expression in ORDER BY
db.Select().From("tasks t").
    OrderBySub(relica.CaseWhen().
        When("t.due_date < CURRENT_DATE", 0).
        When("t.due_date IS NULL", 3).
        Else(1)).
    All(&tasks)

// GroupBySub — type-safe expression in GROUP BY
db.Select("category", "COUNT(*)").
    From("products").
    GroupBySub(relica.CaseWhen().
        When("price < 10", "budget").
        When("price < 100", "mid").
        Else("premium")).
    All(&results)

// OrderByExpr — raw SQL in ORDER BY (when builder doesn't fit)
db.Select().From("users").
    OrderByExpr("FIELD(status, 'active', 'pending', 'disabled')").
    All(&users)
```

---

## Primary Key Configuration

```go
// Integer PK (default, auto-populated on Insert)
type User struct {
    ID    int64  `db:"id,pk"`
    Name  string `db:"name"`
}

// UUID/String PK (server-generated, requires autoincrement tag for RETURNING)
type Post struct {
    ID    string `db:"id,pk,autoincrement"`
    Title string `db:"title"`
}

// Composite PK (no auto-populate)
type OrderItem struct {
    OrderID int `db:"order_id,pk"`
    ItemID  int `db:"item_id,pk"`
}
```

**PK detection priority**: `db:"col,pk"` tag → field named `ID` → field named `Id`.

---

## AutoID — Dual-Key Pattern

**Stripe-like prefixed IDs with auto-generation and lookup:**

```go
// Model definition — one tag:
type User struct {
    ID       int64  `db:"id,pk"`
    PublicID string `db:"public_id,autoid:usr"`
    Name     string `db:"name"`
}

// Insert — PublicID auto-generated:
user := User{Name: "Alice"}
db.Model(&user).Insert()
// user.PublicID = "usr_019078fa-..."

// Lookup by public ID:
var found User
db.Model(&found).FindByPublicID("usr_019078fa-...")

// Wrong prefix → ErrAutoIDPrefixMismatch:
db.Model(&found).FindByPublicID("ord_019078fa-...")

// BeforeInserter hook:
func (u *User) BeforeInsert() error {
    u.CreatedAt = time.Now()
    return nil
}

// Custom generator:
relica.RegisterIDGenerator("ulid", func() string { return ulid.Make().String() })
// Use: db:"public_id,autoid:evt,gen=ulid"
```

**Tag syntax**: `db:"column,autoid"` | `db:"column,autoid:prefix"` | `db:"column,autoid:prefix,gen=name"`

---

## Query Helpers

### Exists / Count

```go
// Check existence — returns bool
exists, err := db.Select().From("users").
    Where(relica.Eq("email", email)).Exists()

// Count rows — returns int64
count, err := db.Select().From("users").
    Where(relica.Eq("status", 1)).Count()
```

### ToSQL (Query Preview)

```go
// Get SQL without executing
sql, params := db.Select().From("users").Where(relica.Eq("id", 1)).ToSQL()
// Works on Select, Update, Delete
```

### Error Handling

```go
// ErrNotFound — One() wraps sql.ErrNoRows
err := db.Select().From("users").Where(relica.Eq("id", 999)).One(&user)
if errors.Is(err, relica.ErrNotFound) { /* not found */ }

// Error classification — works with PostgreSQL, MySQL, SQLite
if relica.IsUniqueViolation(err) { /* duplicate key */ }
if relica.IsForeignKeyViolation(err) { /* FK violation */ }
if relica.IsNotNullViolation(err) { /* NOT NULL violation */ }
if relica.IsCheckViolation(err) { /* CHECK violation */ }
```

---

## Expression API Reference

### Comparison Operators

| Function | SQL | Example |
|----------|-----|---------|
| `Eq(col, val)` | `col = ?` | `Eq("status", 1)` |
| `Eq(col, nil)` | `col IS NULL` | `Eq("deleted_at", nil)` |
| `NotEq(col, val)` | `col != ?` | `NotEq("status", 0)` |
| `NotEq(col, nil)` | `col IS NOT NULL` | `NotEq("deleted_at", nil)` |
| `GreaterThan(col, val)` | `col > ?` | `GreaterThan("age", 18)` |
| `LessThan(col, val)` | `col < ?` | `LessThan("price", 100)` |
| `GreaterOrEqual(col, val)` | `col >= ?` | `GreaterOrEqual("score", 70)` |
| `LessOrEqual(col, val)` | `col <= ?` | `LessOrEqual("qty", 10)` |

### NULL Checks (IMPORTANT!)

```go
// IS NULL - use Eq with nil
db.Select().From("users").
    Where(relica.Eq("deleted_at", nil)).  // → deleted_at IS NULL
    All(&users)

// IS NOT NULL - use NotEq with nil
db.Select().From("users").
    Where(relica.NotEq("deleted_at", nil)).  // → deleted_at IS NOT NULL
    All(&users)

// Alternative: HashExp
db.Select().From("users").
    Where(relica.HashExp{"deleted_at": nil}).  // → deleted_at IS NULL
    All(&users)
```

### Set Operators

| Function | SQL | Example |
|----------|-----|---------|
| `In(col, vals...)` | `col IN (?, ?, ?)` | `In("status", 1, 2, 3)` |
| `NotIn(col, vals...)` | `col NOT IN (?, ?)` | `NotIn("role", "guest")` |
| `Between(col, a, b)` | `col BETWEEN ? AND ?` | `Between("age", 18, 65)` |

**Empty slice behavior:**
```go
// In() with no values → 0=1 (always false, returns no rows)
db.Select().From("users").Where(relica.In("id")).All(&users)  // → WHERE 0=1

// NotIn() with no values → ignored (returns all rows)
db.Select().From("users").Where(relica.NotIn("id")).All(&users)  // → no WHERE

// To pass a dynamic slice, use HashExp with []interface{}:
ids := []interface{}{1, 2, 3}
db.Select().From("users").Where(relica.HashExp{"id": ids}).All(&users)  // → WHERE id IN (1, 2, 3)
```

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

## Scalar Values: Use Row(), NOT One()

**CRITICAL: One() expects struct, Row() is for primitives!**

```go
// WRONG - One() requires struct pointer
var count int
err := db.Select("COUNT(*)").From("users").One(&count)  // ERROR!

// CORRECT - Use Row() for scalar values
var count int
err := db.Select("COUNT(*)").From("users").Row(&count)  // ✅ Works!

// CORRECT - Multiple scalars
var name string
var age int
err := db.Select("name", "age").From("users").
    Where(relica.Eq("id", 1)).
    Row(&name, &age)  // ✅ Works!

// CORRECT - Check existence
var exists int
err := db.Select("1").From("users").
    Where(relica.Eq("id", userID)).
    Row(&exists)  // exists = 1 if found, sql.ErrNoRows if not
```

---

## Testing with sqlmock

**Relica uses prepared statements internally!** When testing with sqlmock:

```go
// WRONG - Missing ExpectPrepare
mock.ExpectQuery("SELECT .+ FROM users").
    WillReturnRows(rows)

// CORRECT - Include ExpectPrepare
mock.ExpectPrepare("SELECT .+ FROM users").
    ExpectQuery().
    WillReturnRows(rows)

// For INSERT/UPDATE/DELETE:
mock.ExpectPrepare("INSERT INTO users").
    ExpectExec().
    WillReturnResult(sqlmock.NewResult(1, 1))
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

    err := db.Select().From("users").
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
func UpdateUserStatus(ctx context.Context, db *relica.DB, userID int, status string) error {
    return db.Transactional(ctx, func(tx *relica.Tx) error {
        var user User

        // Find user
        err := tx.Select().From("users").
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

    err := db.Select().From("users").
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
db.Select().From("users").
    Where("name = '" + name + "'").
    All(&users)

// CORRECT - Parameterized
db.Select().From("users").
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
db.Select().From("users").Where(relica.Eq("id", id)).One(&user)
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
| UPSERT | `db.Model(&struct).Upsert()` | Manual INSERT ON CONFLICT |
| WHERE | `relica.Eq()`, `relica.And()`, `HashExp{}` | String concatenation |
| Complex WHERE | `relica.And(relica.Or(...))` | Nested string conditions |
| JOIN | `.InnerJoin(table, on)`, `.LeftJoin(table, on)` | Raw SQL joins |
| JOIN ON | `relica.EqCol("a.col", "b.col")` | String conditions |
| Single row | `relica.One[T](query)` or `query.One(&struct)` | Manual scan |
| Multiple rows | `relica.All[T](query)` or `query.All(&slice)` | Manual loop |
| Scalar value | `relica.Scalar[T](query)` or `query.Row(&val)` | `query.One(&val)` |
| Batch INSERT | `db.BatchInsertStruct(table, slice)` | Loop of single inserts |
| Transaction | `db.Transactional(ctx, func(tx) error)` | Manual Begin/Commit |

---

*This guide is optimized for AI code generation accuracy. Updated: 2026-08-07.*
