# Advanced Patterns Guide

> **Advanced Query Building Techniques**

---

## Dynamic WHERE Conditions

```go
type UserFilter struct {
    Name   string
    MinAge int
    Roles  []string
}

func buildQuery(db *relica.DB, filter UserFilter) *relica.QueryBuilder {
    qb := db.Select().From("users")

    if filter.Name != "" {
        qb = qb.Where(relica.Like("name", filter.Name))
    }

    if filter.MinAge > 0 {
        qb = qb.Where(relica.GreaterOrEqual("age", filter.MinAge))
    }

    if len(filter.Roles) > 0 {
        qb = qb.Where(relica.In("role", filter.Roles...))
    }

    return qb
}
```

---

## Advanced JOINs

### Multiple JOINs with Aggregates

```go
db.Select(
    "users.id",
    "users.name",
    "COUNT(DISTINCT posts.id) as post_count",
    "AVG(posts.rating) as avg_rating",
).
From("users").
LeftJoin("posts", "posts.user_id = users.id").
GroupBy("users.id", "users.name").
Having("COUNT(posts.id) > ?", 10).
All(&stats)
```

---

## Subqueries

### IN Subquery

```go
subquery := db.Select("DISTINCT user_id").
    From("orders").
    Where("status = ?", "completed")

db.Select().
    From("users").
    Where(relica.In("id", subquery)).
    All(&users)
```

### EXISTS (Faster)

```go
orderCheck := db.Select("1").
    From("orders").
    Where("orders.user_id = users.id")

db.Select().
    From("users").
    Where(relica.Exists(orderCheck)).
    All(&users)
```

---

## Recursive CTEs

```go
// Organizational hierarchy
anchor := db.Select("id", "name", "manager_id", "1 as level").
    From("employees").
    Where("manager_id IS NULL")

recursive := db.Select("e.id", "e.name", "e.manager_id", "h.level + 1").
    From("employees e").
    InnerJoin("hierarchy h", "e.manager_id = h.id")

db.Select().
    WithRecursive("hierarchy", anchor.UnionAll(recursive)).
    From("hierarchy").
    OrderBy("level", "name").
    All(&orgChart)
```

---

## UPSERT Patterns

### Model Upsert (PREFERRED)

```go
// Insert or update all non-PK fields on conflict
user := User{Email: "alice@example.com", Name: "Alice", Status: "active"}
err := db.Model(&user).Upsert()

// Selective fields — only update name and status on conflict
err = db.Model(&user).Upsert("name", "status")
```

Works across PostgreSQL (`ON CONFLICT`), MySQL (`ON DUPLICATE KEY UPDATE`), and SQLite (`ON CONFLICT`). Dialect is detected automatically.

### Low-Level Upsert (map-based)

```go
// PostgreSQL/SQLite: ON CONFLICT
db.Upsert("users", map[string]interface{}{
    "id":    1,
    "name":  "Alice",
    "email": "alice@example.com",
}).
    OnConflict("id").
    DoUpdate("name", "email").
    Execute()
```

---

## UpdateChanged — Dirty Field Detection

Update only the fields that actually differ from the original struct. Generates a minimal `UPDATE` statement.

```go
// Load original
var user User
db.Select().From("users").Where(relica.Eq("id", 1)).One(&user)

// Snapshot before modifications
original := user

// Apply changes
user.Name = "New Name"
// user.Email unchanged

// UPDATE users SET name=? WHERE id=? — only name is updated
err := db.Model(&user).UpdateChanged(&original)
```

If nothing changed, `UpdateChanged` returns immediately without executing any query.

---

## ToSQL — Inspect Generated SQL

Preview the SQL and parameters without executing the query. Useful for debugging, logging, and testing:

```go
// Preview SELECT
sql, params := db.Select("id", "name").
    From("users").
    Where(relica.And(
        relica.Eq("status", "active"),
        relica.GreaterThan("age", 18),
    )).
    OrderBy("name").
    ToSQL()
// sql:    SELECT "id", "name" FROM "users" WHERE (status = $1 AND age > $2) ORDER BY "name"
// params: ["active", 18]

// Preview UPDATE
sql, params = db.Update("users").
    Set(map[string]interface{}{"status": "inactive"}).
    Where(relica.Eq("id", 1)).
    ToSQL()

// Preview DELETE
sql, params = db.Delete("users").
    Where(relica.LessThan("created_at", cutoff)).
    ToSQL()
```

`ToSQL()` is available on `SelectQuery`, `UpdateQuery`, and `DeleteQuery`.

---

## Exists and Count

```go
// Boolean existence check — no row fetching
exists, err := db.Select().From("users").
    Where(relica.Eq("email", email)).
    Exists()

// Row count
count, err := db.Select().From("users").
    Where(relica.Eq("status", "active")).
    Count()

// Count with JOIN
count, err = db.Select().
    From("orders o").
    InnerJoin("users u", "u.id = o.user_id").
    Where(relica.And(
        relica.Eq("u.status", "active"),
        relica.GreaterThan("o.total", 100),
    )).
    Count()
```

Prefer `Exists()` over `Count() > 0` — the database can stop at the first matching row.

---

## Transaction with Retry

```go
func executeWithRetry(db *relica.DB, fn func(*relica.Tx) error) error {
    maxRetries := 3

    for attempt := 0; attempt < maxRetries; attempt++ {
        tx, err := db.Begin(context.Background())
        if err != nil {
            return err
        }

        if err = fn(tx); err == nil {
            return tx.Commit()
        }

        tx.Rollback()

        if !isDeadlock(err) {
            return err
        }

        time.Sleep(time.Millisecond * time.Duration(100*(attempt+1)))
    }

    return fmt.Errorf("failed after %d retries", maxRetries)
}
```

---

## Cursor-Based Pagination

```go
func getUsersWithCursor(db *relica.DB, cursor *int, limit int) ([]User, *int, error) {
    qb := db.Select().From("users").OrderBy("id").Limit(limit + 1)

    if cursor != nil {
        qb = qb.Where("id > ?", *cursor)
    }

    var users []User
    err := qb.All(&users)
    if err != nil {
        return nil, nil, err
    }

    var nextCursor *int
    if len(users) > limit {
        users = users[:limit]
        next := users[limit-1].ID
        nextCursor = &next
    }

    return users, nextCursor, nil
}
```

---

## Soft Deletes

```go
// Soft delete
db.Update("records").
    Set(map[string]interface{}{"deleted_at": "NOW()"}).
    Where("id = ?", id).
    Execute()

// Query non-deleted
db.Select().
    From("records").
    Where("deleted_at IS NULL").
    All(&records)
```

---

## Full-Text Search (PostgreSQL)

```go
// Create tsvector index
db.ExecContext(ctx, `
    CREATE INDEX idx_articles_search
    ON articles USING GIN(to_tsvector('english', title || ' ' || content))
`)

// Search
db.Select().
    From("articles").
    Where("to_tsvector('english', title || ' ' || content) @@ to_tsquery('english', ?)",
        "database & query").
    All(&articles)
```

---

*For more examples, see [Subquery Guide](../SUBQUERY_GUIDE.md) and [CTE Guide](../CTE_GUIDE.md)*
