> Upgrading to **v0.20.0**? Jump to [v0.19.x → v0.20.0](#migration-guide-v019x--v0200): upserts, naming,
> scanning, whole-table DELETE/UPDATE and row locks now fail loudly instead of doing something else.
> Older steps: [v0.18.x → v0.19.0](#migration-guide-v018x--v0190), [v0.19.0 → v0.19.1](#migration-guide-v0190--v0191).

# Migration Guide: v0.3.0-beta → v0.4.0-beta

**Date**: October 26, 2025
**Type**: Breaking changes (acceptable in beta)
**Impact**: High (API structure), Low (user code)

---

## 🎯 Overview

Relica v0.4.0-beta replaces **type aliases** with **wrapper types** to improve pkg.go.dev documentation and follow Go best practices 2025.

### What Changed

```go
// v0.3.0-beta (type aliases)
type DB = core.DB
type QueryBuilder = core.QueryBuilder

// v0.4.0-beta (wrapper types)
type DB struct {
    db *core.DB
}

type QueryBuilder struct {
    qb *core.QueryBuilder
}
```

### Why?

1. **pkg.go.dev documentation** - Type aliases don't show methods for internal packages
2. **Go best practices** - All popular libraries (sqlx, pgx, GORM) use wrappers
3. **Stable public API** - Internal implementation can change without breaking users
4. **Better IDE support** - Full autocomplete and documentation

---

## ✅ What Keeps Working (95% of code)

### All Basic Operations - ZERO Changes Required

```go
// ✅ Database connection
db, err := relica.Open("postgres", dsn)
defer db.Close()

// ✅ Query building
db.Select().From("users").All(&users)

// ✅ Transactions
err = db.Transactional(ctx, func(tx *relica.Tx) error {
    _, err := tx.Insert("users", userData).Execute()
    return err
})

// ✅ WrapDB() - External connection integration
sqlDB, _ := sql.Open("postgres", dsn)
db := relica.WrapDB(sqlDB, "postgres")
db.Select().From("users").All(&users)
defer sqlDB.Close()  // Still works!

// ✅ All query operations
db.Select("u.id", "u.name").
    From("users u").
    InnerJoin("orders o", "o.user_id = u.id").
    Where("u.status = ?", 1).
    GroupBy("u.id").
    Having("COUNT(o.id) > ?", 10).
    OrderBy("u.name").
    Limit(100).
    All(&results)
```

**If your code looks like this, you're done! No migration needed.**

---

## ⚠️ What Might Break (5% of code)

> **v0.19.0:** `Unwrap()` was removed from `DB`, `Tx`, `SelectQuery` and `QueryBuilder` — it exposed
> internal types. The advice below applies to v0.4.0–v0.18.x only. See
> [v0.18.x → v0.19.0](#migration-guide-v018x--v0190); use `DB.SqlDB()` for the underlying `*sql.DB`.

### 1. Type Assertions to Internal Types

**Problem**: Direct type assertions to `*core.DB` will fail

```go
// ❌ v0.3.0 (worked with type aliases):
db, _ := relica.Open("postgres", dsn)
coreDB := (*core.DB)(db)  // ❌ Compile error in v0.4.0
coreDB.InternalMethod()

// ✅ v0.4.0 (use Unwrap):
db, _ := relica.Open("postgres", dsn)
coreDB := db.Unwrap()  // ✅ Returns *core.DB
coreDB.InternalMethod()
```

**Why you might be doing this**: Accessing internal core package features

**Solution**: Use the new `Unwrap()` method:

```go
// All major types have Unwrap()
func (d *DB) Unwrap() *core.DB
func (qb *QueryBuilder) Unwrap() *core.QueryBuilder
func (sq *SelectQuery) Unwrap() *core.SelectQuery
func (tx *Tx) Unwrap() *core.Tx
```

### 2. Type Checks in Tests

**Problem**: Tests checking internal types will fail

```go
// ❌ v0.3.0:
db, _ := relica.Open("postgres", dsn)
assert.IsType(t, &core.DB{}, db)  // ❌ Fails in v0.4.0

// ✅ v0.4.0 (check public type):
db, _ := relica.Open("postgres", dsn)
assert.IsType(t, &relica.DB{}, db)  // ✅ Correct

// Or if you need core type:
assert.IsType(t, &core.DB{}, db.Unwrap())
```

### 3. Function Signatures Expecting `*core.DB`

**Problem**: Functions taking internal types won't compile

```go
// ❌ v0.3.0 (worked with type aliases):
func processDB(db *core.DB) {
    db.Select().From("users").All(&users)
}

db, _ := relica.Open("postgres", dsn)
processDB(db)  // ❌ Type mismatch in v0.4.0

// ✅ v0.4.0 Option 1 (use public type):
func processDB(db *relica.DB) {
    db.Select().From("users").All(&users)
}

// ✅ v0.4.0 Option 2 (use Unwrap if you need core):
func processDB(db *core.DB) {
    // ... internal core operations
}

db, _ := relica.Open("postgres", dsn)
processDB(db.Unwrap())  // ✅ Explicitly unwrap
```

### 4. Storing `*core.DB` References

**Problem**: Struct fields storing internal types

```go
// ❌ v0.3.0:
type Repository struct {
    db *core.DB  // ❌ Won't work in v0.4.0
}

// ✅ v0.4.0 (use public type):
type Repository struct {
    db *relica.DB  // ✅ Use public wrapper
}

// Or if you really need core:
type Repository struct {
    db *core.DB  // Store unwrapped version
}

func NewRepository(db *relica.DB) *Repository {
    return &Repository{db: db.Unwrap()}
}
```

---

## 🔍 How to Find Breaking Changes

### Quick Check

```bash
# Search for internal type usage
grep -r "core\.DB" .
grep -r "core\.QueryBuilder" .
grep -r "core\.SelectQuery" .
grep -r "core\.Tx" .

# Search for type assertions
grep -r "(\*core\." .
```

### Common Patterns to Look For

1. **Imports**: `import "github.com/coregx/relica/internal/core"`
2. **Type assertions**: `(*core.DB)(variable)`
3. **Function parameters**: `func foo(db *core.DB)`
4. **Struct fields**: `db *core.DB`
5. **Type checks**: `reflect.TypeOf(db) == reflect.TypeOf(&core.DB{})`

---

## 🧪 Testing Your Migration

### Step 1: Update Dependency

```bash
go get github.com/coregx/relica@v0.4.0-beta
```

### Step 2: Compile

```bash
go build ./...
```

**If this succeeds, you're 90% done!** Most breaking changes are compile-time errors.

### Step 3: Run Tests

```bash
go test ./...
```

Look for:
- Type assertion failures
- Type check failures in test assertions
- Mock/stub compatibility issues

### Step 4: Check Warnings

```bash
go vet ./...
golangci-lint run ./...
```

---

## 📋 Migration Checklist

- [ ] Update dependency: `go get github.com/coregx/relica@v0.4.0-beta`
- [ ] Run `go build ./...` - fix any compile errors
- [ ] Search for `core.DB`, `core.QueryBuilder`, `core.Tx` in your code
- [ ] Replace type assertions with `Unwrap()` calls
- [ ] Update function signatures to use public types (`*relica.DB`)
- [ ] Update test assertions to check `*relica.DB` instead of `*core.DB`
- [ ] Run full test suite
- [ ] Review any `internal/core` imports (should be rare)
- [ ] Test in staging environment

---

## 🎁 What You Gain

### Better Documentation

**Before (v0.3.0)**:
```
pkg.go.dev shows:
  type DB = core.DB
  (methods not visible)
```

**After (v0.4.0)**:
```
pkg.go.dev shows:
  type DB struct { ... }

  func (d *DB) Builder() *QueryBuilder
      Builder returns a new QueryBuilder for constructing queries.

      Example:
        db.Select().From("users").All(&users)

  func (d *DB) Close() error
      Close closes the database connection...

  (all 15 methods with examples)
```

### Better IDE Support

- Full autocomplete for all methods
- Inline documentation
- Go to definition works correctly
- Better refactoring support

### Future-Proof API

- Internal implementation can change without breaking your code
- Public API is now stable for v1.0.0
- Follows industry best practices (sqlx, pgx, GORM patterns)

---

## 💬 Real-World Example: IrisMX Migration

**IrisMX** (10K+ concurrent users) uses WrapDB() in production.

### Their v0.3.0 code:
```go
package persistence

import (
    "database/sql"
    "github.com/coregx/relica"
)

type Database struct {
    db       *sql.DB
    relicaDB *relica.DB
}

func Open(dsn string) (*Database, error) {
    sqlDB, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, err
    }

    sqlDB.SetMaxOpenConns(100)
    sqlDB.SetMaxIdleConns(50)

    relicaDB := relica.WrapDB(sqlDB, "postgres")

    return &Database{
        db:       sqlDB,
        relicaDB: relicaDB,
    }, nil
}

func (d *Database) GetUserMessages(ctx context.Context, email string) ([]MessageView, error) {
    var results []MessageView

    err := d.relicaDB.Select("m.id", "m.subject", "mb.name as mailbox_name").
        From("messages m").
        InnerJoin("mailboxes mb", "m.mailbox_id = mb.id").
        InnerJoin("users u", "mb.user_id = u.id").
        Where("u.email = ?", email).
        OrderBy("m.internal_date DESC").
        Limit(100).
        WithContext(ctx).
        All(&results)

    return results, err
}

func (d *Database) Close() error {
    return d.db.Close()
}
```

### After v0.4.0 migration:

**ZERO CHANGES NEEDED!** The exact same code works perfectly.

```go
// Exactly the same code - still works!
sqlDB, _ := sql.Open("postgres", dsn)
sqlDB.SetMaxOpenConns(100)

relicaDB := relica.WrapDB(sqlDB, "postgres")  // ✅ Still works!

relicaDB.Select().
    From("users").
    Where("email = ?", email).
    All(&users)  // ✅ Still works!

defer sqlDB.Close()  // ✅ Still works!
```

---

## 🚨 Need Help?

### Common Issues

**Issue**: "cannot convert db (type *relica.DB) to type *core.DB"

**Solution**: Use `db.Unwrap()` instead of type assertion

---

**Issue**: "cannot use db (type *relica.DB) as type *core.DB in argument to function"

**Solution**: Either:
1. Change function signature to accept `*relica.DB`
2. Or call `function(db.Unwrap())`

---

**Issue**: Tests failing with type mismatches

**Solution**: Update test assertions:
```go
// Before
assert.IsType(t, &core.DB{}, db)

// After
assert.IsType(t, &relica.DB{}, db)
```

---

### Getting Support

1. **GitHub Issues**: https://github.com/coregx/relica/issues
2. **Discussions**: https://github.com/coregx/relica/discussions
3. **Email**: support@coregx.dev

When reporting issues, please include:
- Code snippet showing the problem
- Error message
- Go version
- Relica version

---

## 📊 Migration Statistics

Based on internal testing and IrisMX review:

- **95%** of code requires ZERO changes
- **4%** requires simple `Unwrap()` calls
- **1%** requires function signature updates

**Average migration time**: 15-30 minutes for typical projects

---

## ✅ Success Criteria

You've successfully migrated when:

- [x] `go build ./...` succeeds
- [x] `go test ./...` passes
- [x] No type assertion errors
- [x] IDE autocomplete works
- [x] Application runs in staging
- [x] All integration tests pass

---

## 🎯 Next Steps After Migration

1. **Explore better documentation**: https://pkg.go.dev/github.com/coregx/relica@v0.4.0-beta
2. **Review new examples**: Check updated godoc comments in autocomplete
3. **Prepare for v1.0.0**: v0.4.0-beta API will be frozen for v1.0.0 (Q2 2026)

---

*Migration support provided until v1.0.0 stable release*
*Last updated: October 26, 2025*

---

# Migration Guide: v0.10.x → v0.11.0

**Date**: 2026-03-17
**Type**: Additive (no breaking changes)
**Impact**: Low — all existing code continues to work

---

## What's New in v0.11.0

v0.11.0 adds 10 new features. **No existing code needs to change.** All additions are backwards-compatible.

### New Methods

| Feature | Signature | Returns |
|---------|-----------|---------|
| `Exists()` | `SelectQuery.Exists()` | `(bool, error)` |
| `Count()` | `SelectQuery.Count()` | `(int64, error)` |
| `ToSQL()` | `SelectQuery/UpdateQuery/DeleteQuery.ToSQL()` | `(string, []interface{})` |
| `Model.Upsert()` | `ModelQuery.Upsert(fields...)` | `error` |
| `Model.UpdateChanged()` | `ModelQuery.UpdateChanged(original)` | `error` |

### New Errors and Functions

| Symbol | Purpose |
|--------|---------|
| `relica.ErrNotFound` | Returned by `One()` when no row matches |
| `relica.IsUniqueViolation(err)` | Detect duplicate key constraint |
| `relica.IsForeignKeyViolation(err)` | Detect FK constraint violation |
| `relica.IsNotNullViolation(err)` | Detect NOT NULL constraint violation |
| `relica.IsCheckViolation(err)` | Detect CHECK constraint violation |

---

## Upgrade Steps

### Step 1: Update Dependency

```bash
go get github.com/coregx/relica@v0.11.0
go mod tidy
```

### Step 2: Build and Test

```bash
go build ./...
go test ./...
```

Both should pass without changes.

---

## Recommended Code Updates

These are optional improvements — your existing code still works.

### Replace sql.ErrNoRows with relica.ErrNotFound

```go
// Before (still works — errors.Is handles both)
if err == sql.ErrNoRows { }

// After (preferred — clear intent)
if errors.Is(err, relica.ErrNotFound) { }
```

### Replace Manual COUNT with Count()

```go
// Before
var result struct{ Total int `db:"total"` }
db.Select("COUNT(*) as total").From("users").One(&result)
count := result.Total

// After
count, err := db.Select().From("users").Count()
```

### Replace Existence Pattern with Exists()

```go
// Before
var exists int
err := db.Select("1").From("users").Where(relica.Eq("email", email)).Row(&exists)
found := err == nil

// After
found, err := db.Select().From("users").
    Where(relica.Eq("email", email)).Exists()
```

### Classify Constraint Errors

```go
// Before — manual string parsing
if strings.Contains(err.Error(), "duplicate") { }

// After — cross-database, reliable
if relica.IsUniqueViolation(err) { }
```

---

## No Action Required

The following continue to work exactly as before:

- All SELECT/INSERT/UPDATE/DELETE queries
- `db.Model().Insert()` / `Update()` / `Delete()`
- Transactions (`Begin`, `Transactional`)
- Expression API (`Eq`, `And`, `Or`, `In`, etc.)
- Named placeholders (`{:name}`)
- Batch operations
- Statement cache
- Connection pool configuration

---

# Migration Guide: v0.18.x → v0.19.0

**Type**: MINOR (pre-v1.0 API cleanup). Most code compiles unchanged.

## What Changed

Twenty exported types were aliases to internal packages (`type HashExp = core.HashExp`), so
pkg.go.dev and IDEs showed no fields, methods or docs for them. They are now declared in package
`relica`. See CHANGELOG `[0.19.0]` and the godoc of each type.

## Upgrade Steps

1. `go get github.com/coregx/relica@v0.19.0`
2. Fix the compile errors below, if any. Each has a mechanical fix.

| Compile error | Fix |
|---------------|-----|
| `cannot use relica.Case(...) (value of interface type relica.CaseExp) as *relica.CaseExp value` | Drop the `*`: `var c relica.CaseExp`. Same for `LikeExp`, `CoalesceExp`, `NullIfExp`, `GreatestLeastExp`, `ConcatExp` |
| `db.Unwrap undefined` (also `Tx`, `SelectQuery`, `QueryBuilder`) | Use the public API; for the raw connection use `db.SqlDB()` |
| `invalid operation: opt == nil (mismatched types relica.Option and untyped nil)` | Options are values now; drop the nil check. The zero `Option` is a no-op |
| `ilike.Like undefined (type relica.LikeExp has no field or method Like)` (code that did `exp.Like = "ILIKE"`) | Use `relica.ILike(col, values...)` (also `NotILike`, `OrILike`, `OrNotILike`), added in **v0.19.1**. v0.19.0 has no replacement: this case was missed in v0.19.0 — upgrade to v0.19.1 directly |

## Behavior Fixes You May Notice

- A query whose condition contains an invalid expression (`Like(...).EscapeChars` with an odd number
  of arguments) now **returns an error instead of running without the filter**.
- `CaseWhen().When(relica.Eq(...), ...)` now renders the condition; Expressions as `THEN`/`ELSE` values
  are rendered instead of bound as parameters.
- `Like` on SQLite now matches escaped `%`/`_` literally (Relica adds `ESCAPE '\'`).
- `Having("... {:name}", relica.Params{...})` now resolves named parameters.
- `NullStringMap.Keys()` returns sorted keys.
- `WithQueryHook(nil)`, `WithLogger(nil)` are ignored instead of panicking; `NewSlogAdapter(nil)` uses
  `slog.Default()`.

## No Action Required

Struct literals (`relica.TxOptions{...}`), map literals (`relica.HashExp{...}`, `relica.Params{...}`),
`NullStringMap` methods, `QueryHook` functions, custom `Logger` implementations and
`BeforeInserter` models compile and behave as before.

---

# Migration Guide: v0.19.0 → v0.19.1

Patch release: nothing stops compiling. Fixes and additions:

| Change | What you may notice |
|---|---|
| `relica.ILike`, `NotILike`, `OrILike`, `OrNotILike` | Case-insensitive LIKE on every database (replaces `exp.Like = "ILIKE"`, which v0.19.0 removed) |
| Error helpers read driver error codes | `IsUniqueViolation`, `IsForeignKeyViolation`, `IsNotNullViolation`, `IsCheckViolation` now also match on a localized PostgreSQL, on MariaDB CHECK errors (4025) and on MySQL 1263/1364 (NOT NULL in strict mode). The message is no longer consulted when the error carries a code |
| `relica.IsDeadlock`, `IsSerializationFailure`, `IsLockTimeout` | New helpers for retry decisions — see [AGENTS.md](../AGENTS.md#retrying-deadlocks-and-serialization-failures) |
| Schema-qualified table names in INSERT/UPDATE/DELETE/upsert/batch | `"app.users"` is quoted as `"app"."users"`, as SELECT always did. If you worked around the bug with `NewQuery`, the builder works now |
| `CASE` results on PostgreSQL | All-number (or all-bool) results are sent as `CAST($n AS INTEGER/BIGINT/NUMERIC/BOOLEAN)`; such a `CASE` now has a numeric type instead of text (it can be assigned to integer columns and sorts numerically). Scanning it into a `string` still works |
| ORDER BY / GROUP BY follow the call order | `OrderBy`, `OrderByExpr` and `OrderBySub` (and the `GroupBy` family) render in the order you call them. Before, plain columns always came first. If you called `OrderBySub(...)` / `OrderByExpr(...)` before `OrderBy(...)`, your query now sorts the way it reads; if you relied on the old order, move the `OrderBy(...)` call first |
| `GroupByExpr` / `GroupBySub` parameters with `Having` | Their parameters were bound after `Having`'s, so a query with parameters in both clauses returned wrong rows. They are now bound in statement order. No code change needed |
| `Transactional` / `TransactionalTx` report a failed rollback | When `f` returns an error and the rollback also fails, the result is `errors.Join(err, rollbackErr)`; `errors.Is`/`errors.As` on `err` keep working, a direct `err == errX` comparison no longer matches in that case — use `errors.Is` |

If you matched `err.Error()` text yourself, switch to the helpers: the text depends on the server
language and version, the code does not.

---

# Migration Guide: v0.19.x → v0.20.0

**Type**: MINOR (pre-v1.0). Relica now refuses statements it used to run with a different meaning than the code
asked for. Most of the changes below turn a silent wrong result into an error returned by `Execute`/`All`/`One`
before anything is sent to the database. Each section says how to find affected code and how to keep the old
behaviour where that is still possible.

## Upsert: exactly one explicit conflict action (ADR-019)

`Upsert(table, values)` and `BatchInsert(...).OnConflict(...)` need exactly one of `DoUpdate(cols...)`,
`DoUpdateAll()`, `DoNothing()`.

| v0.19 code | v0.19 behaviour | v0.20.0 | Change your code to |
|---|---|---|---|
| `db.Upsert(t, v).Execute()` (no OnConflict, no action) | plain `INSERT`, failed on a duplicate | error: "an upsert takes exactly one of …" | add the action you meant, or use `db.Insert` |
| `db.Upsert(t, v).OnConflict("id").Execute()` | updated every non-target column | same error | `.OnConflict("id").DoUpdateAll()` |
| `BatchInsert(...).OnConflict("id")` without an action | updated every non-target column | same error | `.DoUpdateAll()` |
| `.DoUpdate()` with no columns | updated every non-target column | error | `.DoUpdateAll()` |
| `.DoUpdate("x")` where `x` is not inserted | set `x` to NULL/default | error | insert `x` or drop it from `DoUpdate` |
| `.DoUpdate(...).DoNothing()` (or any two actions) | the last call won | error | keep one |
| PostgreSQL/SQLite `DoUpdate`/`DoUpdateAll` without `OnConflict` | `ON CONFLICT () …` syntax error at run time; batch: DoUpdate silently dropped | error at build | add `OnConflict(cols...)` |
| MySQL `DoNothing()` | plain `INSERT`, failed on a duplicate | `ON DUPLICATE KEY UPDATE c = c` (first conflict column, else first inserted column): the row is kept | nothing; note the row is still locked, an `AUTO_INCREMENT` value is consumed, `BEFORE UPDATE` triggers fire, affected rows is 0 |
| `OnConflict()` with no columns, or called twice | accepted | error | pass the columns once |
| `Upsert` with an empty values map | `INSERT INTO t () VALUES ()` | error | — |

**Model upsert** (`db.Model(&m).Upsert(fields...)`, `UpsertOn(conflict, fields...)`) keeps "save the struct": the
target is the primary key (or `conflict`), the PK is never updated, and with no `fields` every other column is
updated. Changes:

- `Exclude(cols...)` now works for upserts: the excluded columns are left out of both the INSERT and the update
  (as in `Insert`). Before, it was ignored and the columns were overwritten on every upsert.
- Unknown names are errors: a `fields`/`Exclude`/`UpsertOn` name that is not a column of the model, a field that is
  in the conflict target, or a field that is also excluded. `fields` and `Exclude` take **column names** (the `db`
  tag), not Go field names.
- A model with nothing to update (only PK columns) is an error instead of an invalid `SET`.

**MySQL proposed-row syntax.** Updates read the row proposed for insertion — after `BEFORE INSERT` triggers, like
PostgreSQL's `EXCLUDED`. The default stays `col = VALUES(col)` (MySQL 5.7–9.x, MariaDB; deprecated by MySQL since
8.0.20 with warning 1287). On MySQL 8.0.19+ you can switch to the row alias:
`relica.Open("mysql", dsn, relica.WithMySQLUpsertSyntax(relica.MySQLUpsertRowAlias))`. MariaDB rejects the alias.

**Batch duplicates.** Keys repeated inside one batch are not detected by Relica: with `DoNothing` the first row wins
everywhere; with `DoUpdate`, PostgreSQL rejects the statement (error 21000) and MySQL/SQLite keep the last row.

**Find affected code:** `grep -rn "\.Upsert(\|OnConflict(" --include=*.go .` and check each chain ends in one action.

## Table names, column names, struct scanning (ADR-018)

| v0.19 | v0.20.0 | Keep the old behavior |
|---|---|---|
| model without `TableName()` → lower case + `s` (`UserProfile` → `userprofiles`) | snake_case singular (`user_profile`) | add `func (UserProfile) TableName() string { return "userprofiles" }` |
| `func (*T) TableName()` ignored for `db.Model(T{})` | honoured | — |
| untagged field written as `"CreatedAt"`, read case-insensitively | `created_at` on write and read | add `db:"CreatedAt"` tags, or `WithNaming(relica.Naming{Column: func(c relica.ColumnSource) string { return c.Field.Name }})` |
| embedded struct written as one column (or "unsupported type") | fields flattened | — (an embedded `*T` or a tagged embedded struct is now an error: embed `T`, tag its fields) |
| `db:"pk"` wrote column `pk` | primary key, column from the naming rule | `db:"pk_column,pk"` for an explicit column |
| result without a column for a struct field → zero, `err == nil` | error (`ScanRequireFields`) | list the columns in `Select(...)`, tag the field `db:"-"`, or `relica.WithScanMode(relica.ScanLenient)` (per query: `.ScanMode(relica.ScanLenient)`) |
| `BatchInsertStruct` with some zero and some explicit keys → explicit keys dropped | error | split the batch |

**Find affected code:**
- models used with `Model(...)`, `SelectQuery.Model(...)` or `InsertStruct`/`BatchInsertStruct`/`UpdateStruct` that have
  no `TableName()` and no `Table("x")`: the derived name changed unless it was already a single lower-case word ending
  in `s`;
- struct fields without a `db` tag in such models: the column name changed unless the field name was one lower-case
  word (`Name` → `name` both before and after, since reads were case-insensitive);
- run the test suite: every struct read whose query does not return a column for each field now fails with
  `field X expects column "x", which the result does not have` — the field was silently zero before.

The rule (ozzo-dbx, `regexp ([^A-Z_])([A-Z])` → `${1}_$2`, lower case) and its alternatives (`SplitAcronyms()`,
`Initialisms(GoInitialisms()...)`, `TablePrefix`, `PluralTables`, a custom `Naming`) are described in
[AGENTS.md](../AGENTS.md#table-and-column-names-v0200-adr-018) and on `relica.Snake`.

`relica.WrapDB(sqlDB, driver, opts...)` now accepts the same options as `Open`; existing calls compile unchanged.

## Row locks (ADR-020)

| v0.19 code | v0.20.0 | Change your code to |
|---|---|---|
| `db.Select()...ForUpdate()` (any lock) on `*relica.DB` | error "require a transaction" (every database) | run it in `db.Transactional(ctx, func(tx *relica.Tx) error { ... })` or on a `*relica.Tx` |
| lock + `Union`/`Intersect`/`Except`/`GroupBy`/`Distinct` | error | lock the base rows in a separate query |
| lock + `Count()` / `Exists()` | error (the lock was dropped) | `All`/`One` with the lock, or no lock |

**Find affected code:** `grep -rn "ForUpdate\|ForShare\|ForUpdateSkipLocked" --include=*.go .` and check that each
runs on a transaction. A `*relica.DB` from a `dbcontext.With(ctx)` helper outside a transaction errors too.

## UPDATE / DELETE without a condition (ADR-020)

| v0.19 code | v0.20.0 | Change your code to |
|---|---|---|
| `db.Delete("t").Execute()`, `db.Update("t").Set(v).Execute()` | error "without Where would affect every row" | add the `Where` you meant, or `.AllRows()` for the whole table |
| `.Where(cond)` where `cond` renders empty — `relica.And()` with no parts, `relica.HashExp{}`, `relica.Like("c")` with no values, `""` | error "renders empty" (was: no WHERE, every row) | build the condition only when there is something to filter, or use `AllRows()` |
| `AllRows()` together with `Where` | error | keep one |

`Model(&m).Update()` / `Delete()` and `UpdateChanged` always use the primary key and are unaffected; `UpdateStruct`
needs a `Where` like `Update`. SELECT keeps its semantics: an empty condition selects every row.

**Find affected code:** search for `.Delete(` / `.Update(` chains that end in `.Execute()` with no `.Where(`, and for
conditions assembled from optional filters (`relica.And(conds...)` where `conds` may be empty).

## Names Relica cannot find

| v0.19 code | v0.20.0 | Change your code to |
|---|---|---|
| `db.Model(&m).Insert("name", "emial")` / `.Update("emial")` | error "field "emial" is not a column of the model" (was: skipped) | fix the name; names are column names (`db` tag), not Go field names |
| `db.Model(&m).Exclude("crated_at")...` | error "Exclude name ... is not a column of the model" (was: ignored) | fix the name |
| `BatchInsert(t, cols).ValuesMap(map[string]any{"typo": v})` | error "key ... is not one of the batch columns" (was: dropped, NULL written) | fix the key or add the column to `cols` |
| `db:"public_id,autoid:usr,gen=ulid"` with no `RegisterIDGenerator("ulid", ...)` | error "autoid generator "ulid" is not registered" (was: UUID v7) | register the generator at start-up |

Column names in SQL (`Select("col")`, `Where`/`HashExp` keys, `OrderBy`) are not checked by Relica — the database
rejects unknown columns itself.
