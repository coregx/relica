# Troubleshooting Guide

> **Common Issues and Solutions**

---

## Database Connection Issues

### Error: "connection refused"
**Cause**: Database server not running or incorrect host/port

**Solutions:**
```bash
# Check if PostgreSQL is running
sudo systemctl status postgresql

# Verify connection
psql -h localhost -U postgres -d mydb

# Check DSN format
postgres://user:password@localhost:5432/dbname?sslmode=disable
```

### Error: "authentication failed"
**Cause**: Incorrect credentials or missing permissions

**Solutions:**
- Verify username/password in DSN
- Check PostgreSQL `pg_hba.conf` for auth method
- Grant necessary permissions: `GRANT ALL ON DATABASE mydb TO user;`

---

## Error Handling

### ErrNotFound vs sql.ErrNoRows

**Problem**: `One()` returns an error when no row matches. Before v0.11.0, this was `sql.ErrNoRows`. Since v0.11.0 it is `relica.ErrNotFound`.

**Solution**: Use `errors.Is` — it matches both `relica.ErrNotFound` and the wrapped `sql.ErrNoRows`:

```go
var user User
err := db.Select().From("users").Where(relica.Eq("id", id)).One(&user)

if errors.Is(err, relica.ErrNotFound) {
    // handle not found
}
```

Do NOT compare with `==`:
```go
// ❌ fragile — breaks if error is wrapped
if err == sql.ErrNoRows { }

// ✅ correct — works with relica.ErrNotFound and sql.ErrNoRows
if errors.Is(err, relica.ErrNotFound) { }
```

### Constraint Violations

**Problem**: INSERT/UPDATE fails with a database constraint error and you need to return a user-friendly message.

**Solution**: Use error classification functions (PostgreSQL, MySQL, SQLite):

```go
if err := db.Model(&user).Insert(); err != nil {
    switch {
    case relica.IsUniqueViolation(err):
        return errors.New("email already in use")
    case relica.IsForeignKeyViolation(err):
        return errors.New("referenced record does not exist")
    case relica.IsNotNullViolation(err):
        return errors.New("required field is missing")
    case relica.IsCheckViolation(err):
        return errors.New("field value is out of allowed range")
    default:
        return fmt.Errorf("database error: %w", err)
    }
}
```

### Constraint check stopped matching (localized server, MariaDB, new driver)

**Symptom**: code that compares `err.Error()` with "duplicate key ..." works in development and
silently fails in production.
**Cause**: server messages depend on the server language (PostgreSQL `lc_messages`) and change
between MySQL, MariaDB and driver versions.
**Solution**: use `relica.IsUniqueViolation(err)` and the other helpers. Since v0.19.1 they read the
error code (SQLSTATE / MySQL error number / SQLite extended code) from pgx, lib/pq,
go-sql-driver/mysql, modernc.org/sqlite and mattn/go-sqlite3, wherever the error is wrapped. Only
other drivers still fall back to English message text.

---

## Query Errors

### Error: "sql: Scan error on column index X"
**Cause**: Struct field type mismatch

**Solutions:**
```go
// Check db tags match column names
type User struct {
    ID    int    `db:"id"`      // ✅ Matches column
    Name  string `db:"username"` // ✅ Maps to "username" column
}

// Use sql.Null* for nullable columns
type User struct {
    Email sql.NullString `db:"email"` // ✅ Handles NULL
}
```

### Error: "no such table"
**Cause**: Table doesn't exist

**Solutions:**
- Run migrations before queries
- Verify table name spelling
- Check database connection (wrong DB?)

---

## Performance Issues

### Slow Queries
**Diagnosis:**
```go
// Add query logging
start := time.Now()
db.Select().From("users").All(&users)
log.Printf("Query took: %v", time.Since(start))
```

**Solutions:**
- Add indexes on WHERE clause columns
- Use EXPLAIN to analyze query plan
- Reduce SELECT columns (avoid `SELECT *`)
- Use pagination (LIMIT/OFFSET)

### Connection Pool Exhausted
**Symptoms**: Slow response times, timeouts

**Solutions:**
```go
// Increase pool size
db, err := relica.Open("postgres", dsn,
    relica.WithMaxOpenConns(50),  // Increase from default
    relica.WithMaxIdleConns(10),
)

// Add connection timeout
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
```

---

## Transaction Issues

### Error: "sql: transaction has already been committed or rolled back"
**Cause**: the transaction was used after it ended — a second `Commit`, a query after `Commit`/`Rollback`,
`Commit`/`Rollback` called inside `Transactional`, or its context was canceled (`database/sql` then
rolls it back).

**Solutions:**
```go
// ✅ Let Transactional end the transaction
err := db.Transactional(ctx, func(tx *relica.Tx) error {
    // work on tx; do not call tx.Commit() or tx.Rollback() here
    return nil
})

// ✅ Manual: check Begin's error, then defer Rollback
tx, err := db.Begin(ctx)
if err != nil {
    return err
}
defer func() { _ = tx.Rollback() }() // sql.ErrTxDone after Commit — expected here only
// work on tx...
return tx.Commit()
```

### Writes survive a rollback
**Cause**: some statements did not run on the transaction — they went through `*relica.DB` (for example
a dbcontext `With(ctx)` that ignores the transaction in the context). Such statements autocommit.
**Solution**: every statement goes through `tx` or a `relica.Executor` that is the transaction; see
[AGENTS.md → Transactions](../../AGENTS.md#transactions).

### Deadlocks, serialization failures, lock timeouts
**Symptoms**: `deadlock detected` (PostgreSQL 40P01) / `Deadlock found` (MySQL 1213),
`could not serialize access` (40001), lock wait timeouts (55P03, MySQL 1205), SQLite
`database is locked`

**Detect**: `relica.IsDeadlock(err)`, `relica.IsSerializationFailure(err)`, `relica.IsLockTimeout(err)`.

**Solutions:**
- Lock rows in the same order everywhere (for example ascending id)
- Keep transactions short: no network calls or waiting inside
- Never open a second transaction inside the first (it waits for a connection or a lock held by the first)
- Use an isolation level only as high as needed
- Retry the whole transaction when `IsDeadlock || IsSerializationFailure` — see the `RetryTx`
  helper in [AGENTS.md](../../AGENTS.md#retrying-deadlocks-and-serialization-failures)
- Lock timeouts: find the long lock holder; report "busy, try again" instead of waiting longer
- SQLite: `_txlock=immediate` in the DSN and a `busy_timeout` pragma
- SQLite readers that hit `database is locked`: open them with a `file:…?mode=ro` URI and without
  `_txlock` — on a plain path `?mode=ro` is ignored and the reader is a writer
  ([AGENTS.md › SQLite](../../AGENTS.md#sqlite))

---

## Migration Issues

### Error: "column already exists"
**Cause**: Migration ran multiple times

**Solutions:**
- Use migration tool with versioning (golang-migrate, goose)
- Add IF NOT EXISTS checks
- Track migration state in database

---

## Platform-Specific

### PostgreSQL: LastInsertId not supported
**Problem:**
```go
result, _ := db.Insert("users", data).Execute()
id, _ := result.LastInsertId() // ❌ ERROR with lib/pq
```

**Solution:**
```go
var id int
db.QueryRowContext(ctx,
    `INSERT INTO users (name) VALUES ($1) RETURNING id`,
    name,
).Scan(&id) // ✅ CORRECT
```

### MySQL: parseTime parameter
**Problem**: time.Time fields not scanning

**Solution:**
```go
// Add parseTime=true to DSN
dsn := "user:pass@tcp(localhost:3306)/mydb?parseTime=true"
```

---

## Getting Help

1. Check error message carefully
2. Search [GitHub Issues](https://github.com/coregx/relica/issues)
3. Enable query logging for debugging
4. Create minimal reproducible example
5. Open new issue with details

---

*For more help, see [GitHub Discussions](https://github.com/coregx/relica/discussions)*
