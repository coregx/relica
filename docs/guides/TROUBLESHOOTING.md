# Troubleshooting Guide

> **Common Issues and Solutions**
> **Version**: v0.5.0-beta

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
db.Select("*").From("users").All(&users)
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

### Error: "transaction has already been committed or rolled back"
**Cause**: Double commit or rollback

**Solutions:**
```go
// ✅ CORRECT: Use defer Rollback
tx, _ := db.Begin(ctx)
defer tx.Rollback() // Safe: no-op after Commit

// Do work...

return tx.Commit() // Only commit once
```

### Deadlocks
**Symptoms**: Timeout errors, hung transactions

**Solutions:**
- Always acquire locks in same order
- Keep transactions short
- Use appropriate isolation level
- Add retry logic for deadlock errors

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
