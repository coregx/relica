# AGENTS.md

> **Instructions for AI coding agents working with Relica.**
> **Read this BEFORE generating any code.**
>
> Agent skill: `skills/relica/` (Agent Skills format) — how to find the docs for your version, an audit script for
> a project's Relica code (`skills/relica/scripts/audit.sh`), the upgrade procedure. Claude Code:
> `/plugin install relica --marketplace coregx/relica`; any agent: `<module Dir>/skills/relica/SKILL.md`.

---

## API Priority (MUST FOLLOW)

### 1. RECOMMENDED: Model() API for CRUD

**ALWAYS use Model() API for struct-based operations:**

```go
// FIND by primary key (one-liner)
var user User
err := db.Model(&user).Find(42)
// Composite PK: db.Model(&item).Find(orderId, itemId)

// FIND with column selection and filters (PK from struct)
user = User{ID: 42}
err = db.Select("name", "email").Where(relica.IsNull("deleted_at")).Model(&user)
// SELECT name, email FROM users WHERE deleted_at IS NULL AND id = 42 LIMIT 1

// INSERT - CORRECT
user := User{Name: "Alice", Email: "alice@example.com"}
err := db.Model(&user).Insert()
// user.ID is auto-populated!

// UPDATE - CORRECT
user.Name = "Alice Updated"
err := db.Model(&user).Update()

// DELETE - CORRECT
err := db.Model(&user).Delete()

// UPSERT - CORRECT (INSERT or UPDATE on conflict; target = primary key)
err := db.Model(&user).Upsert()              // all non-PK fields
err = db.Model(&user).Upsert("name", "email") // selective fields (column names)
err = db.Model(&user).Exclude("created_at").Upsert() // created_at neither inserted nor updated

// Builder upsert (v0.20.0+): exactly ONE action, or Execute returns an error
_, err = db.Upsert("users", values).OnConflict("id").DoUpdateAll().Execute()     // every non-target column
_, err = db.Upsert("users", values).OnConflict("id").DoUpdate("name").Execute()  // listed columns only
_, err = db.Upsert("users", values).OnConflict("id").DoNothing().Execute()       // keep existing row
// WRONG: db.Upsert("users", values).Execute()  /  .OnConflict("id").Execute()  → error, nothing sent

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

// CORRECT - NULL checks (explicit, preferred over Eq(col, nil))
db.Select().From("users").
    Where(relica.IsNull("deleted_at")).
    All(&activeUsers)

db.Select().From("orders").
    Where(relica.IsNotNull("shipped_at")).
    All(&shippedOrders)

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

> `{:name}` (not `:name` or `@name`) — curly braces are not SQL syntax, so no conflicts with PostgreSQL `::` casts, MySQL `@var`, or JSONB `?` operators. Inherited from ozzo-dbx.

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

**All JOIN types are supported. The `on` parameter accepts both strings and Expressions:**

### String ON (simple, most common)

```go
// INNER JOIN — static ON condition, no user input
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
```

### Expression ON (parameterized, type-safe)

Use `Expression` when the ON condition contains **user-supplied values** (SQL injection protection)
or when you need **composable conditions**:

```go
// EqCol — type-safe column-to-column (PREFERRED for simple ON)
db.Select("u.name", "o.total").
    From("users u").
    InnerJoin("orders o", relica.EqCol("o.user_id", "u.id")).
    All(&results)

// NewExp with parameters — ON condition with user values (SQL injection safe)
db.Select().
    From("orders o").
    InnerJoin("users u", relica.NewExp("o.user_id = u.id AND u.tenant_id = ?", tenantID)).
    All(&results)

// And/Or — complex composite ON conditions
db.Select().
    From("messages m").
    InnerJoin("users u", relica.And(
        relica.EqCol("m.user_id", "u.id"),
        relica.GreaterThan("u.status", 0),
    )).
    All(&results)
```

### When to use which

| Pattern | When to use |
|---------|-------------|
| `"a.id = b.id"` (string) | Static ON, no user input — simplest |
| `relica.EqCol("a.id", "b.id")` | Column-to-column — type-safe, quoting handled |
| `relica.NewExp("a.id = b.id AND a.x = ?", val)` | ON with user values — SQL injection protection |
| `relica.And(expr1, expr2)` | Multiple conditions — composable |

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
- `relica.NewExp("...", args...)` — parameterized (SQL injection safe)
- `relica.And(...)` / `relica.Or(...)` — composite conditions

### Common Mistakes

```go
// WRONG — DISTINCT is NOT a column name, use .Distinct() method
db.Select("DISTINCT u.name").From("users u").InnerJoin(...)

// CORRECT
db.Select("u.name").From("users u").InnerJoin(...).Distinct()
```

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

Read this section before writing code that changes more than one row. In projects that use
Relica, the data-integrity bugs were here, not in query building: writes that survived a
rollback, row locks that protected nothing, request deadlines that never reached SQL.

### Rules

1. **One business operation, one transaction, opened at the top** — HTTP handler,
   application service, CLI command or job. Repositories never begin, commit or roll back.
2. **Inside a transaction every statement goes through it**: `tx.Select(...)`, or a
   repository that receives a `relica.Executor` (or calls a dbcontext `With(ctx)`, below).
   A statement sent through `*relica.DB` runs on another pool connection in autocommit
   mode: it is not rolled back and does not see the transaction's uncommitted rows.
3. **Prefer `Transactional` / `TransactionalTx`.** They commit when the function returns
   nil, roll back when it returns an error, and roll back and re-panic on a panic. Use
   manual `Begin` only when the work cannot be written as one function.
4. **Row locks only inside a transaction.** `ForUpdate`, `ForShare` and
   `ForUpdateSkipLocked` on `*relica.DB` autocommit: the lock is released as soon as the
   SELECT finishes, and the read-then-write race it was meant to stop is still there.
5. **Never open a second transaction inside the first.** `DB.Transactional` always begins a
   new transaction; it does not look for one in the context. A nested `Begin` takes a
   second connection, and with a small pool it waits for itself until the context
   deadline. Join the outer transaction instead (dbcontext pattern below).
6. **Pass the request context to `Begin`/`Transactional`.** The transaction is bound to it:
   when the context is canceled, `database/sql` rolls the transaction back and `Commit`
   returns `context.Canceled`. Do not use `context.Background()` for request work.
7. **Keep transactions short.** No HTTP/RPC calls, e-mails, message publishing or waiting
   for a user inside. Do side effects after the commit, or write them to an outbox table in
   the same transaction.
8. **Compute in SQL with expressions.** `Set(map[string]any{"balance": relica.NewExp("balance - ?", amount)})`.
   A plain string value is data: `Set(map[string]any{"balance": "balance - ?"})` binds the
   text `"balance - ?"` as the new value (SQLite stores it even in an INTEGER column).
9. **Check every error, including `Commit`.** With manual `Begin`, check `Begin`'s error
   before `defer tx.Rollback()` — on failure `tx` is nil and the deferred call panics.
   `Rollback` after `Commit` returns `sql.ErrTxDone`; ignoring it is fine only in that
   deferred call.
10. **Lock rows in a fixed order** (for example ascending id) when one transaction locks
    several rows. Two transactions locking the same rows in opposite order deadlock.
11. **Test that a rollback really discards writes** made through your data layer
    (see "Testing transactions").
12. **Retry the whole transaction, never one statement**, and only when
    `relica.IsDeadlock(err) || relica.IsSerializationFailure(err)` — the database has already
    rolled it back. Retry at the outermost transaction boundary, with backoff, a bounded number
    of attempts and the request context (see "Retrying deadlocks and serialization failures").

### Which API

| Situation | Use |
|---|---|
| The whole operation fits in one function | `db.Transactional(ctx, func(tx *relica.Tx) error { ... })` |
| Needs an isolation level or read-only | `db.TransactionalTx(ctx, &relica.TxOptions{Isolation: sql.LevelSerializable}, f)` |
| Repository used both inside and outside transactions | parameter of type `relica.Executor` |
| Application keeps the transaction in `context.Context` (go-rest-api style) | dbcontext: `With(ctx) relica.Executor` + `Transactional` that joins |
| Transaction begun by another library (migration tool, sqlc) | `db.WrapTx(ctx, sqlTx)` |
| Work cannot be one function | `db.Begin(ctx)` / `db.BeginTx(ctx, opts)` + `defer tx.Rollback()` + `tx.Commit()` |

### Transactional — the default

A money transfer: both rows locked in id order, balance checked, arithmetic done in SQL.

```go
func Transfer(ctx context.Context, db *relica.DB, fromID, toID int64, amount int64) error {
    return db.Transactional(ctx, func(tx *relica.Tx) error {
        var accounts []Account
        if err := tx.Select("id", "balance").From("account").
            Where(relica.In("id", fromID, toID)).
            OrderBy("id"). // lock order: ascending id
            ForUpdate().
            All(&accounts); err != nil {
            return fmt.Errorf("lock accounts: %w", err)
        }
        if len(accounts) != 2 {
            return ErrAccountNotFound
        }
        for _, a := range accounts {
            if a.ID == fromID && a.Balance < amount {
                return ErrInsufficientFunds // rollback
            }
        }

        if _, err := tx.Update("account").
            Set(map[string]any{"balance": relica.NewExp("balance - ?", amount)}).
            Where(relica.Eq("id", fromID)).
            Execute(); err != nil {
            return fmt.Errorf("debit: %w", err)
        }
        if _, err := tx.Update("account").
            Set(map[string]any{"balance": relica.NewExp("balance + ?", amount)}).
            Where(relica.Eq("id", toID)).
            Execute(); err != nil {
            return fmt.Errorf("credit: %w", err)
        }
        return nil // commit; a Commit error is returned by Transactional
    })
}
```

### Manual Begin

```go
tx, err := db.Begin(ctx)
if err != nil {
    return fmt.Errorf("begin: %w", err) // tx is nil here
}
defer func() { _ = tx.Rollback() }() // sql.ErrTxDone after Commit — expected

if err := tx.Model(&user).Insert(); err != nil {
    return err // deferred Rollback discards the transaction
}
if err := tx.Model(&order).Insert(); err != nil {
    return err
}
return tx.Commit() // the Commit error matters: nothing is saved if it fails
```

**Methods on `*relica.Tx`**: Select, Insert, InsertStruct, BatchInsertStruct, Update,
UpdateStruct, Delete, Upsert, BatchInsert, BatchUpdate, Model, NewQuery, ExecContext,
QueryContext, QueryRowContext, Commit, Rollback.

```go
// Raw SQL within transaction (for migrations, DDL, etc.)
tx.ExecContext(ctx, "CREATE INDEX idx_users_email ON users(email)")
```

**Joining a transaction started elsewhere** — `WrapTx` turns a raw `*sql.Tx` (from another
library, a migration tool, sqlc) into a `*relica.Tx`. The caller keeps ownership: commit or
roll back on either handle, both act on the same transaction.

```go
sqlTx, err := sqlDB.BeginTx(ctx, nil)
tx := db.WrapTx(ctx, sqlTx)           // pass the ctx the tx was begun with
err = repo.Save(ctx, tx, &user)       // repo takes relica.Executor
```

### Executor — one type for *DB and *Tx

`relica.Executor` is implemented by both `*relica.DB` and `*relica.Tx`. Repository
code should accept it instead of a concrete type, so the same function runs inside
or outside a transaction:

```go
func (r *UserRepo) Save(ctx context.Context, ex relica.Executor, u *User) error {
    return ex.Model(u).WithContext(ctx).Insert()
}

// Works with both:
repo.Save(ctx, db, &u)
db.Transactional(ctx, func(tx *relica.Tx) error { return repo.Save(ctx, tx, &u) })
```

**dbcontext pattern (transaction in context).** This is the go-rest-api / ozzo-dbx
`dbx.Builder` pattern. `With(ctx)` MUST check the context for a stored transaction
and return it; returning `*relica.DB` unconditionally silently breaks every
`Transactional` block (writes commit on their own and survive the rollback):

```go
type contextKey struct{}

// CORRECT — repositories calling With(ctx) join the active transaction
func (d *DB) With(ctx context.Context) relica.Executor {
    if tx, ok := ctx.Value(contextKey{}).(*relica.Tx); ok {
        return tx
    }
    return d.db.WithContext(ctx)
}

func (d *DB) Transactional(ctx context.Context, f func(ctx context.Context) error) error {
    if _, ok := ctx.Value(contextKey{}).(*relica.Tx); ok {
        return f(ctx) // already in a tx — join it, never nest
    }
    return d.db.Transactional(ctx, func(tx *relica.Tx) error {
        return f(context.WithValue(ctx, contextKey{}, tx))
    })
}

// WRONG — tx in context is ignored, Transactional does nothing
func (d *DB) With(ctx context.Context) *relica.DB {
    return d.db.WithContext(ctx)
}
```

How the pieces fit — `*relica.DB` opened once at startup, repositories only call `With(ctx)`,
the transaction boundary lives in the application service:

```go
// main.go — one *relica.DB for the whole process (it owns the pool and the statement cache).
rdb, err := relica.Open("postgres", dsn)
dbc := dbcontext.New(rdb)

// repository — never begins or commits; works inside and outside a transaction.
func (r *AccountRepo) Debit(ctx context.Context, id, amount int64) error {
    _, err := r.db.With(ctx).Update("account").
        Set(map[string]any{"balance": relica.NewExp("balance - ?", amount)}).
        Where(relica.Eq("id", id)).
        Execute()
    return err
}

// application service — owns the transaction boundary.
func (s *PaymentService) Pay(ctx context.Context, from, to, amount int64) error {
    return s.db.Transactional(ctx, func(ctx context.Context) error {
        if err := s.accounts.Debit(ctx, from, amount); err != nil {
            return err // rollback
        }
        return s.accounts.Credit(ctx, to, amount)
    })
}
```

Do not wrap `*sql.DB` with `relica.WrapDB` per request or per transaction: each call creates a new
statement cache, so nothing is ever reused. Wrap once at startup.

Executor deliberately excludes `Begin`, `Commit`, `Rollback`, `Transactional`,
`Builder()` and `WithContext()`. Its method set is frozen — new `*DB`/`*Tx`
methods are not added to it, so test doubles keep compiling.

### Context propagation — the invariant

Every object derived from a `*DB` inherits its context; every narrower `WithContext`
overrides it. Priority: **per-query ctx > DB/builder ctx > `context.Background()`**.

```go
reqDB := db.WithContext(ctx)              // request-scoped copy; db itself is untouched
reqDB.Select().From("users").All(&us)     // runs under ctx
reqDB.Insert("users", data).Execute()     // runs under ctx
reqDB.NewQuery("DELETE ...").Execute()    // runs under ctx
reqDB.Model(&u).Insert()                  // runs under ctx

// Per-query override, including the *Query returned by Insert/InsertStruct/NewQuery:
db.Insert("users", data).WithContext(ctx).Execute()
db.NewQuery("SELECT ...").WithContext(ctx).Row(&n)

// A *Tx is bound to the ctx passed to Begin/Transactional, never to the DB ctx.
```

A canceled context fails with `context.Canceled` — it is never reported as `ErrNotFound`.
In the dbcontext pattern this is why `With(ctx)` must return `d.db.WithContext(ctx)` on the
non-transactional path and not the bare `d.db`: otherwise request deadlines never reach SQL.

Inside a transaction the context passed to `Begin`/`Transactional` governs everything: when it is
canceled, `database/sql` rolls the transaction back and `Commit` returns `context.Canceled`.

### Isolation levels

The default is the database's (READ COMMITTED on PostgreSQL, REPEATABLE READ on MySQL InnoDB,
SERIALIZABLE on SQLite). Raise it only for a reason, and per transaction:

```go
err := db.TransactionalTx(ctx, &relica.TxOptions{Isolation: sql.LevelSerializable}, func(tx *relica.Tx) error {
    // read, decide, write — the database aborts the transaction on a conflicting write
    return nil
})

// Reports: a read-only transaction (writes fail on PostgreSQL and MySQL).
err = db.TransactionalTx(ctx, &relica.TxOptions{ReadOnly: true}, func(tx *relica.Tx) error { ... })
```

At SERIALIZABLE (and REPEATABLE READ on PostgreSQL) the database may abort a transaction with a
serialization failure, and any level can end in a deadlock. Such a transaction must be retried as a
whole — the error can come from any statement or from `Commit`.

### Retrying deadlocks and serialization failures

```go
// RetryTx runs f in a transaction and runs the whole transaction again when the
// database aborted it as a deadlock victim or for a serialization failure.
// f may run several times: it must not have side effects outside the database.
func RetryTx(ctx context.Context, db *relica.DB, opts *relica.TxOptions, f func(*relica.Tx) error) error {
    const maxAttempts = 5
    backoff := 20 * time.Millisecond
    for attempt := 1; ; attempt++ {
        err := db.TransactionalTx(ctx, opts, f)
        if err == nil || attempt == maxAttempts ||
            !(relica.IsDeadlock(err) || relica.IsSerializationFailure(err)) {
            return err
        }
        select { // full jitter: wait a random time up to the current backoff
        case <-time.After(rand.N(backoff)): // math/rand/v2
        case <-ctx.Done():
            return errors.Join(err, ctx.Err())
        }
        backoff = min(2*backoff, time.Second)
    }
}
```

- **Retry only at the outermost boundary** — the service method or handler that owns the
  transaction. Inside a joined (dbcontext) transaction the outer transaction is already aborted;
  return the error and let the owner retry.
- **f must be repeatable**: rebuild its in-memory state on every call; no e-mails, HTTP calls or
  message publishing inside (rule 7) — they would happen once per attempt.
- **What to retry**:

| Error | Retry the transaction? |
|---|---|
| `IsDeadlock` — PostgreSQL 40P01, MySQL 1213 | yes |
| `IsSerializationFailure` — PostgreSQL 40001 (also from `Commit`), SQLite `SQLITE_BUSY_SNAPSHOT` | yes |
| `IsLockTimeout` — PostgreSQL 55P03, MySQL 1205/3572, SQLite `SQLITE_BUSY` | policy: usually report "busy, try again" (HTTP 409/503); the lock holder may be a long job |
| `IsUniqueViolation`, `IsForeignKeyViolation`, `IsNotNullViolation`, `IsCheckViolation` | no — map to a domain error |
| `context.Canceled`, `context.DeadlineExceeded` | no |

- MySQL: a deadlock rolls back the whole transaction; a lock wait timeout (1205) only the
  statement — returning the error from f rolls back the rest.
- SQLite: prefer `_txlock=immediate` (no BUSY_SNAPSHOT at all) to retrying.

### SQLite

- One writer at a time for the whole database. `ForUpdate`/`ForShare`/`ForUpdateSkipLocked` are left
  out of the SQL on SQLite: there are no row locks to take.
- A transaction that reads and then writes starts as a reader (`BEGIN DEFERRED`) and can fail with
  `database is locked` (SQLITE_BUSY) when it upgrades to a writer. With modernc.org/sqlite, add
  `_txlock=immediate` to the DSN so every transaction takes the write lock at `BEGIN`.
- A read-only connection (a second pool for readers, a hook, a reporting tool) needs a `file:` URI:
  `file:/data/app.db?mode=ro` (Windows: `file:C:/data/app.db?mode=ro`). Both modernc.org/sqlite and
  mattn/go-sqlite3 cut the query off a plain path, so `/data/app.db?mode=ro` opens the database
  **read-write** (and creates it when the file is missing). Such a connection with `_txlock=immediate`
  starts every transaction with `BEGIN IMMEDIATE`, waits behind the writer and fails with
  `database is locked`. Leave `_txlock` out of reader DSNs. On a real `mode=ro` connection SQLite
  starts `BEGIN IMMEDIATE` as a read transaction, and modernc.org/sqlite uses a plain `BEGIN` for
  `TxOptions{ReadOnly: true}` (mattn/go-sqlite3 does not).
- `:memory:` gives every pool connection its own empty database; tests that use more than one
  connection need a file (for example in `t.TempDir()`).

### Testing transactions

Unit tests with mocks cannot show that a rollback discards writes. Test it on a real database
through your own data layer — this test fails for every `With(ctx)` that ignores the transaction:

```go
func TestTransactional_RollbackDiscardsWrites(t *testing.T) {
    ctx := context.Background()
    errStop := errors.New("stop")

    err := dbc.Transactional(ctx, func(ctx context.Context) error {
        if err := repo.Create(ctx, &Account{ID: 1, Balance: 100}); err != nil {
            return err
        }
        return errStop // force a rollback
    })
    require.ErrorIs(t, err, errStop)

    n, err := rdb.Select().From("account").Where(relica.Eq("id", 1)).Count()
    require.NoError(t, err)
    require.Zero(t, n, "write made through With(ctx) survived the rollback")
}
```

Add a nested case as well: an inner `Transactional` must see the outer transaction's rows, and the
outer rollback must remove the inner writes too.

### Mistakes found in real projects

| Symptom | Cause | Fix |
|---|---|---|
| Writes survive a rollback; "transaction" does nothing | `With(ctx)` returns `*relica.DB` and ignores the tx in ctx | return the `*relica.Tx` from ctx (dbcontext above) |
| Double spending / lost updates despite `ForUpdate()` | the locking SELECT ran on `*relica.DB`, outside the transaction | run it on `tx` (or `With(ctx)` inside `Transactional`) |
| Request timeouts do not stop SQL | non-tx path returns bare `d.db`, or `context.Background()` is used | `d.db.WithContext(ctx)`; pass the request ctx to `Begin`/`Transactional` |
| Hangs until the deadline under load | a second transaction opened inside the first (nested `Begin`/`Transactional`) | join the outer transaction; never nest |
| Statement cache never hits; high prepare count | `relica.WrapDB(sqlDB, ...)` per request or per transaction | create the `*relica.DB` once at startup |
| Column contains the text `balance - ?` | string value in `Set` | `relica.NewExp("balance - ?", amount)` |
| nil-pointer panic when the database is down | `defer tx.Rollback()` before checking `Begin`'s error | check the error first |
| Half of the changes saved | errors of `Execute` ignored, then `Commit` | check every error; prefer `Transactional` |
| Sporadic "deadlock detected" / "could not serialize access" errors reach users under load | no retry | retry the whole transaction (`RetryTx` above) |
| Duplicate e-mails or charges after a retry | side effects inside the retried function | do them after the commit, or through an outbox table |
| Duplicate-key handling silently stopped working on a non-English PostgreSQL / MariaDB | own `strings.Contains(err.Error(), ...)` checks | `relica.IsUniqueViolation(err)` and friends — they read the error code |

---

## UPDATE / DELETE need Where (v0.20.0+, ADR-020)

```go
// WRONG — error since v0.20.0: "without Where would affect every row"
db.Delete("sessions").Execute()
// WRONG — error: the condition renders empty (would also hit every row)
db.Update("users").Set(values).Where(relica.HashExp{}).Execute()

// CORRECT
db.Delete("sessions").Where(relica.LessThan("expires_at", now)).Execute()
db.Delete("sessions").AllRows().Execute() // the whole table, on purpose
```

---

## Row Locking (FOR UPDATE / FOR SHARE)

**Pessimistic locking for concurrent access — only inside a transaction.** Since v0.20.0 a lock on
`*relica.DB` (outside a transaction) is an **error** on every database, SQLite included: the statement
would autocommit and release the lock at once (two `ForUpdateSkipLocked` workers would claim the same row).
A lock combined with `Union`/`UnionAll`/`Intersect`/`Except`, `GroupBy`, `Distinct`, `Count()` or `Exists()`
is an error too. Inside a transaction SQLite omits the clause (database-level locking). MariaDB has no
`FOR SHARE`. Aggregates written as column text (`Select("COUNT(*)")`) are not detected — PostgreSQL rejects
them with a lock.

```go
// Exclusive lock: other transactions wait to update, delete or lock these rows
// (plain SELECTs are not blocked on PostgreSQL / MySQL InnoDB)
tx.Select().From("accounts").
    Where(relica.Eq("id", accountID)).
    ForUpdate().
    One(&account)

// Shared lock (allow concurrent reads, block writes)
tx.Select().From("products").
    Where(relica.Eq("id", productID)).
    ForShare().
    One(&product)

// Non-blocking lock — skip already-locked rows (job queue pattern)
tx.Select().From("tasks").
    Where(relica.Eq("status", "pending")).
    ForUpdateSkipLocked().
    Limit(10).
    All(&tasks)
```

| Method | SQL | Use case |
|--------|-----|----------|
| `ForUpdate()` | `FOR UPDATE` | Exclusive row lock |
| `ForShare()` | `FOR SHARE` | Shared row lock |
| `ForUpdateSkipLocked()` | `FOR UPDATE SKIP LOCKED` | Job queues, non-blocking |

```go
// WRONG — error since v0.20.0: "require a transaction"
db.Select().From("tasks").ForUpdateSkipLocked().All(&tasks)
```

---

## Connection Inspection

```go
// Access underlying *sql.DB for pool tuning
sqlDB := db.SqlDB()
sqlDB.SetMaxOpenConns(100)

// Health check
err := db.PingContext(ctx)

// Driver name
db.DriverName() // "postgres", "mysql", "sqlite3"
```

---

## Ordering with Expressions

```go
// OrderBySub — type-safe expression in ORDER BY.
// OrderBy, OrderByExpr and OrderBySub render in call order (since v0.19.1):
// here the CASE is the primary key and due_date breaks ties.
db.Select().From("tasks t").
    OrderBySub(relica.CaseWhen().
        When("t.due_date < CURRENT_DATE", 0).
        When("t.due_date IS NULL", 3).
        Else(1)).
    OrderBy("t.due_date").
    All(&tasks)

// GroupBySub — type-safe expression in GROUP BY (one row per price tier)
db.Select("COUNT(*) AS n").
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

**Select takes column names, SelectExpr takes expressions.** `Select` quotes each argument as a column
(`"t.col"`, `"col AS alias"`; an argument with `(` such as `"COUNT(*) AS n"` is passed as is). Literals and arithmetic
without parentheses must go through `SelectExpr`, with the alias inside the string and `?` args bound:

```go
db.Select("e.id", "e.name").SelectExpr("h.level + 1 AS level").   // not Select("h.level + 1") → "h"."level + 1"
db.Select("id").SelectExpr("1 AS depth").                         // not Select("1 as depth") → "1" AS "depth"
db.Select("id").SelectExpr("price * ? AS gross", 1.2).            // not SelectExpr("price * 1.2", "gross")
```

---

## Set Operations (UNION / INTERSECT / EXCEPT) — v0.20.0 rules

The chain reads left to right, the way the SQL text does:

```go
// OrderBy/OrderByExpr/OrderBySub/Limit/Offset AFTER Union apply to the combined result
db.Select("name").From("users").
    Union(db.Select("name").From("archived_users")).
    OrderBy("name").Limit(20).                      // first 20 names of the union
    All(&names)

// BEFORE Union they choose the first branch's rows (here: its 5 newest)
db.Select("id").From("orders").OrderBy("id DESC").Limit(5).
    Union(db.Select("id").From("pinned_orders")).
    OrderBy("id").All(&ids)

// Each operation applies to everything before it: (a ∪ b) ∩ c on every database
a.Union(b).Intersect(c)
a.Union(b.Intersect(c))                             // the other grouping: a ∪ (b ∩ c)

// Sort by an expression or a qualified name: wrap the compound (PostgreSQL takes only
// output column names or numbers in a compound's ORDER BY; MySQL rejects table.col)
db.Select().FromSelect(a.Union(b), "u").OrderByExpr("id % 2, id").All(&rows)
```

Errors (returned when the query runs):
- `OrderBy` in a branch without `Limit`/`Offset` (`a.OrderBy("x").Union(b)`) orders nothing — PostgreSQL and MySQL
  ignore it — so it is an error: call `OrderBy` after `Union`.
- A set operation after the combined result's `OrderBy`/`Limit`/`Offset` (`a.Union(b).OrderBy("x").Union(c)`):
  pass the sorted compound as a query instead (`c.Union(sorted)`, `FromSelect(sorted, "u")`).

Before v0.20.0 (and in ozzo-dbx) `OrderBy`/`Limit` after `Union` went into the first branch, and
`a.Union(b).Intersect(c)` was sent flat, which PostgreSQL and MySQL read as `a ∪ (b ∩ c)`.

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

**PK detection priority**: every field tagged pk (`db:"col,pk"`, `db:",pk"`, legacy `db:"pk"`) → field named `ID` → field named `Id`. Embedded structs count.

---

## Table and Column Names (v0.20.0+, ADR-018)

| Go code | Name used |
|---|---|
| `db:"col"` tag | `col` |
| no tag | snake_case of the field name: `CreatedAt` → `created_at`, `UserID` → `user_id`, `HTTPLog` → `httplog` |
| `db:"-"` | field skipped |
| embedded struct (no tag) | its fields are flattened into the model |
| table: `Table("x")` | `x` |
| table: `TableName()` method (value **or pointer** receiver) | its result |
| table: neither | snake_case singular of the type: `UserProfile` → `user_profile`, `User` → `user` (NOT `users`) |

```go
// Plural or legacy table names: say so explicitly
func (User) TableName() string { return "users" }

// Other rules, per DB:
db, err := relica.Open("postgres", dsn,
    relica.WithNaming(relica.Snake(relica.Initialisms(relica.GoInitialisms()...)))) // HTTPLog → http_log
// relica.Snake(relica.TablePrefix("app."), relica.PluralTables(f)) — prefix/plural for derived tables only
// relica.Naming{Column: func(relica.ColumnSource) string { return "" }} — every field must have a db tag
```

**Reading (ScanMode)**: by default a struct field that has no column in the result is an **error**
(`ScanRequireFields`) instead of a silent zero. `Select("id", "name")` requires only the listed columns;
`db:"-"` fields are never required; extra result columns are ignored. Per DB: `relica.WithScanMode(...)`;
per query: `.ScanMode(relica.ScanLenient)` (ozzo-dbx behavior) or `relica.ScanStrict` (extra columns are errors too).

**Common mistake after upgrading**: a model without `TableName()` that relied on the old `+ "s"` rule now
queries `user` instead of `users` → add `TableName()`.

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

`Count()` is the number of rows `All` would return and `Exists()` whether there is one (v0.20.0): with `GroupBy`
the number of groups, with `Having`/`Distinct`/`Union`/CTEs and aggregates (`Select("MAX(total)")` counts 1)
applied. The result's `OrderBy`, `Limit` and `Offset` are ignored, so a paginated query reports its total — in a
set operation those called after `Union`; the first branch's (`a.Limit(1).Union(b)`) choose its rows and are kept:

```go
page := db.Select().From("orders").Where(relica.Eq("user_id", id)).OrderBy("id DESC").Limit(20).Offset(40)
total, err := page.Count() // all of the user's orders, not 20
```

A grouped, distinct, compound or expression-list query is counted as a derived table
(`SELECT COUNT(*) FROM (...) AS relica_count`). MySQL rejects a derived table with two columns of the same name
(error 1060), which `Select()` (`*`) over a join produces: list the columns (`Select("e.id", "d.name")`) for such a
`Count()` on MySQL.

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

// Constraint violations — map them to domain errors
err = db.Model(&user).Insert()
switch {
case relica.IsUniqueViolation(err):     // duplicate key or primary key
    return ErrEmailTaken
case relica.IsForeignKeyViolation(err): // referenced row missing / still referenced
    return ErrUnknownTeam
case relica.IsNotNullViolation(err), relica.IsCheckViolation(err):
    return ErrInvalidInput
case err != nil:
    return fmt.Errorf("create user: %w", err)
}

// Concurrency — see "Retrying deadlocks and serialization failures"
relica.IsDeadlock(err)             // retry the whole transaction
relica.IsSerializationFailure(err) // retry the whole transaction
relica.IsLockTimeout(err)          // lock not acquired in time: policy decision
```

The helpers read the **error code** the driver attached, anywhere in the error tree (`%w`,
`errors.Join`), so they work with a localized PostgreSQL (`lc_messages`), MariaDB, and errors your
code has wrapped. Never match `err.Error()` text yourself.

| Helper | PostgreSQL SQLSTATE | MySQL / MariaDB | SQLite |
|---|---|---|---|
| `IsUniqueViolation` | 23505 | 1062, 1022, 1169, 1586, 1859 | CONSTRAINT_UNIQUE, CONSTRAINT_PRIMARYKEY |
| `IsForeignKeyViolation` | 23503 | 1216, 1217, 1451, 1452 | CONSTRAINT_FOREIGNKEY (`PRAGMA foreign_keys = ON`) |
| `IsNotNullViolation` | 23502 | 1048, 1263, 1364 | CONSTRAINT_NOTNULL |
| `IsCheckViolation` | 23514 | 3819; MariaDB 4025 | CONSTRAINT_CHECK |
| `IsDeadlock` | 40P01 | 1213 | — (no deadlock detection) |
| `IsSerializationFailure` | 40001 | — (reported as 1213) | BUSY_SNAPSHOT |
| `IsLockTimeout` | 55P03 | 1205, 3572 | BUSY, LOCKED |

Drivers: pgx v5 and lib/pq (every version), go-sql-driver/mysql, modernc.org/sqlite,
mattn/go-sqlite3. With other drivers the helpers fall back to matching the English message.

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
| `Like(col, patterns...)` | `col LIKE ?` (several values: AND) | `Like("name", "john")` → `'%john%'` |
| `NotLike(col, patterns...)` | `col NOT LIKE ?` (AND) | `NotLike("email", "test")` |
| `OrLike(col, patterns...)` | `col LIKE ? OR col LIKE ?` | `OrLike("email", "gmail", "yahoo")` |
| `OrNotLike(col, patterns...)` | `col NOT LIKE ? OR ...` | `OrNotLike("tag", "a", "b")` |
| `ILike`, `NotILike`, `OrILike`, `OrNotILike` | case-insensitive versions (v0.19.1+) | `ILike("cpu_model", "xeon")` |

All of them escape `%`, `_` and `\` in the values and wrap them in `%`; `.Match(left, right)`
controls the wildcards: `Match(false, true)` → `'value%'` (prefix), `Match(true, false)` → `'%value'` (suffix).
`.EscapeChars()` with no arguments turns the escaping off: `%` and `_` in the value act as
wildcards. Use it only for a pattern your code builds, never for user input.

**Case sensitivity.** `Like` is as case-sensitive as the database makes it: PostgreSQL — always;
MySQL — depends on the column collation (`*_ci` no, `*_bin` yes); SQLite — ASCII letters fold.
Use `ILike` when the match must ignore case on every database:

| Database | `ILike("name", "al")` renders | Notes |
|---|---|---|
| PostgreSQL | `"name" ILIKE ?` | non-ASCII folding follows the database locale (`LC_CTYPE`) |
| MySQL | ``LOWER(`name`) LIKE LOWER(?)`` | works for any non-binary character set and collation |
| SQLite | ``LOWER(`name`) LIKE LOWER(?) ESCAPE '\'`` | `LOWER` folds ASCII only (no ICU extension) |

`LOWER(col)` cannot use a plain index on `col`. For prefix search on a large MySQL table whose
column is already `*_ci`, plain `Like(...).Match(false, true)` is case-insensitive and can use the index.

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

// Empty slice → always-false condition (never "no condition")
HashExp{"status": []any{}}
```

### Functional Expressions (CASE, COALESCE, NULLIF, GREATEST/LEAST, CONCAT)

Use as computed columns (`SelectSub(exp, "alias")`), in conditions, in `OrderBySub`/`GroupBySub`,
or as values in `Update().Set(...)`. They nest.

```go
db.Select("id").
    SelectSub(relica.CaseWhen().
        When(relica.GreaterThan("balance", 1000), "gold"). // condition: Expression (preferred)
        Else("basic"), "tier").
    SelectSub(relica.Coalesce("nickname", "first_name", "'Anonymous'"), "display_name").
    From("account").All(&rows)

db.Update("account").Set(map[string]any{
    "balance": relica.NewExp("balance + ?", 10),
    "label":   relica.Coalesce("nickname", "name"),
}).Where(relica.Eq("id", id)).Execute()
```

String arguments differ by function — get this right:

| Function | A plain string means | Literal string |
|----------|---------------------|----------------|
| `Coalesce`, `NullIf`, `Greatest`, `Least`, `Concat` | column name (quoted) | `"'text'"` (single quotes inside) |
| `Case(...).When(value, result)` / `.Else(v)` | a **value** (bound parameter) | — |
| `CaseWhen().When(cond, ...)` condition | **raw SQL** — never user input | — |

Dialect differences are handled for you: `Greatest`/`Least` → `MAX`/`MIN` on SQLite, `Concat` → `CONCAT()`
on MySQL and `||` elsewhere, `Like` → explicit `ESCAPE '\'` on SQLite.

### Expression Types (for declarations)

All types are declared in package `relica` (no aliases to internal packages, enforced by a test):

```go
var e relica.Expression   = relica.Eq("a", 1)      // sealed: values only from relica constructors
var l relica.LikeExp      = relica.Like("name", "x") // LikeExp, CaseExp, CoalesceExp, ... are interfaces
var c relica.CaseExp      = relica.Case("s").When(1, "a")
var h relica.HashExp      = relica.HashExp{"a": 1}   // map type
var p relica.Params       = relica.Params{"id": 1}   // map type
// WRONG (pre-v0.19.0 shape): var c *relica.CaseExp
```

`Expression` cannot be implemented outside Relica. For SQL no constructor covers, use
`relica.NewExp("sql with ? placeholders", args...)`.

An expression built with invalid input (e.g. `Like(...).EscapeChars("!")` — odd count) makes the
query return that error; nothing is executed. Check `LikeExp.Err()` early if you build it from config.

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
err := db.Select().SelectExpr("1").From("users").
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
