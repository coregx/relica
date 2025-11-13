# Performance Tuning Guide

> **Optimize Relica for Maximum Performance**
> **Version**: v0.5.0

---

## Query Optimization

### 1. Use SELECT with Specific Columns

```go
// ❌ SLOW: Fetches all columns
db.Select("*").From("users").All(&users)

// ✅ FAST: Only needed columns
db.Select("id", "name", "email").From("users").All(&users)
```

### 2. Add Indexes

```sql
-- Frequently queried columns
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_posts_user_id ON posts(user_id);

-- Composite indexes for multiple WHERE clauses
CREATE INDEX idx_users_status_created ON users(status, created_at);
```

### 3. Use EXPLAIN to Analyze

```go
// PostgreSQL
rows, _ := db.QueryContext(ctx, "EXPLAIN ANALYZE SELECT * FROM users WHERE email = $1", email)

// Check execution plan for:
// - Seq Scan (bad) vs Index Scan (good)
// - High cost values
// - Missing indexes
```

---

## Statement Cache Optimization

### Understanding the Cache

```go
// First call: Prepares statement
db.Select("*").From("users").Where("id = ?", 1).One(&user) // ~10ms

// Subsequent calls: Uses cache
db.Select("*").From("users").Where("id = ?", 2).One(&user) // <60ns
db.Select("*").From("users").Where("id = ?", 3).One(&user) // <60ns
```

### Maximize Cache Hits

```go
// ✅ GOOD: Same query pattern
func getUserByID(db *relica.DB, id int) (*User, error) {
    var user User
    err := db.Select("*").From("users").Where("id = ?", id).One(&user)
    return &user, err
}

// ❌ BAD: Dynamic query patterns (cache misses)
func getUserDynamic(db *relica.DB, id int, includeEmail bool) (*User, error) {
    query := "SELECT id, name"
    if includeEmail {
        query += ", email"  // Different query = cache miss
    }
    // ...
}
```

### Tune Cache Capacity

```go
// Default: 1000 statements
db, err := relica.Open("postgres", dsn,
    relica.WithStmtCacheCapacity(2000), // Increase for high-traffic apps
)

// Monitor cache stats
stats := db.StmtCacheStats()
hitRate := float64(stats.Hits) / float64(stats.Hits + stats.Misses)
log.Printf("Cache hit rate: %.2f%%", hitRate*100)
```

---

## Batch Operations

### Batch INSERT (3.3x faster)

```go
// ❌ SLOW: Individual inserts
for _, user := range users {
    db.Insert("users", map[string]interface{}{
        "name":  user.Name,
        "email": user.Email,
    }).Execute() // N queries
}

// ✅ FAST: Batch insert
batch := db.Builder().BatchInsert("users", []string{"name", "email"})
for _, user := range users {
    batch.Values(user.Name, user.Email)
}
batch.Execute() // 1 query, 3.3x faster
```

### Batch UPDATE (2.5x faster)

```go
// ❌ SLOW: Individual updates
for id, status := range updates {
    db.Update("users").
        Set(map[string]interface{}{"status": status}).
        Where("id = ?", id).
        Execute() // N queries
}

// ✅ FAST: Batch update
batch := db.Builder().BatchUpdate("users", "id")
for id, status := range updates {
    batch.Set(id, map[string]interface{}{"status": status})
}
batch.Execute() // 1 query, 2.5x faster
```

---

## Connection Pool Tuning

### Optimal Settings

```go
// Formula: MaxOpenConns = (CPU cores * 2) + effective_spindle_count
// For most apps: 10-25 per CPU core

db, err := relica.Open("postgres", dsn,
    relica.WithMaxOpenConns(25),     // Tune based on workload
    relica.WithMaxIdleConns(5),      // 20% of MaxOpenConns
    relica.WithConnMaxLifetime(300), // 5 minutes
)
```

### Monitor Pool Stats

```go
func monitorConnectionPool(db *relica.DB) {
    stats := db.Stats()

    log.Printf("Open connections: %d", stats.OpenConnections)
    log.Printf("In use: %d", stats.InUse)
    log.Printf("Idle: %d", stats.Idle)
    log.Printf("Wait count: %d", stats.WaitCount)
    log.Printf("Wait duration: %v", stats.WaitDuration)

    // Alert if wait count is high
    if stats.WaitCount > 100 {
        log.Println("WARNING: High wait count, consider increasing MaxOpenConns")
    }
}
```

---

## Query Performance Patterns

### Use WHERE with Indexed Columns

```go
// ✅ FAST: Indexed column
db.Select("*").From("users").Where("email = ?", email) // Uses idx_users_email

// ❌ SLOW: Function on indexed column (can't use index)
db.Select("*").From("users").Where("LOWER(email) = ?", strings.ToLower(email))
```

### Limit Result Sets

```go
// ❌ BAD: Fetch all rows
db.Select("*").From("large_table").All(&results)

// ✅ GOOD: Paginate
db.Select("*").
    From("large_table").
    OrderBy("id").
    Limit(100).
    Offset(0).
    All(&results)
```

### Use JOINs Instead of N+1 Queries

```go
// ❌ SLOW: N+1 queries
users := getUsers()
for _, user := range users {
    posts := getPostsByUserID(user.ID) // N additional queries
}

// ✅ FAST: Single JOIN
db.Select("users.*", "posts.title").
    From("users").
    LeftJoin("posts", "posts.user_id = users.id").
    All(&results) // 1 query
```

---

## Memory Optimization

### Stream Large Result Sets

```go
// For very large datasets, scan rows manually
rows, err := db.QueryContext(ctx, "SELECT * FROM large_table")
defer rows.Close()

for rows.Next() {
    var record Record
    rows.Scan(&record.ID, &record.Data)
    processRecord(record) // Process one at a time
}
```

### Avoid Loading Unnecessary Data

```go
// ❌ BAD: Load everything into memory
db.Select("*").From("logs").All(&logs) // 1M rows = OOM

// ✅ GOOD: Process in chunks
const batchSize = 1000
offset := 0

for {
    var batch []Log
    db.Select("*").From("logs").Limit(batchSize).Offset(offset).All(&batch)

    if len(batch) == 0 {
        break
    }

    processBatch(batch)
    offset += batchSize
}
```

---

## Benchmarking

### Measure Query Performance

```go
func benchmarkQuery(db *relica.DB) {
    start := time.Now()

    var users []User
    db.Select("*").From("users").Where("status = ?", 1).All(&users)

    duration := time.Since(start)
    log.Printf("Query took: %v, Rows: %d", duration, len(users))
}
```

### Use Go Benchmarks

```go
func BenchmarkSelect(b *testing.B) {
    db := setupTestDB()
    defer db.Close()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        var user User
        db.Select("*").From("users").Where("id = ?", 1).One(&user)
    }
}
```

---

## Database-Specific Optimizations

### PostgreSQL

```go
// Use COPY for bulk inserts (fastest)
// Use RETURNING instead of LastInsertId()
// Enable connection pooling: pgbouncer

// Analyze tables after bulk operations
db.ExecContext(ctx, "ANALYZE users")
```

### MySQL

```go
// Use InnoDB (not MyISAM)
// Enable query cache (if workload is read-heavy)
// Use LOAD DATA INFILE for bulk inserts
```

### SQLite

```go
// Use WAL mode for better concurrency
db.ExecContext(ctx, "PRAGMA journal_mode=WAL")

// Increase cache size
db.ExecContext(ctx, "PRAGMA cache_size=-64000") // 64MB
```

---

## Performance Checklist

- [ ] Queries use specific columns (not `SELECT *`)
- [ ] Indexes on frequently queried columns
- [ ] Batch operations for bulk inserts/updates
- [ ] Connection pool tuned (MaxOpenConns, MaxIdleConns)
- [ ] Statement cache enabled and monitored
- [ ] Large result sets paginated
- [ ] JOINs used instead of N+1 queries
- [ ] Context timeouts set
- [ ] Query performance monitored
- [ ] Database-specific optimizations applied

---

*For more optimization techniques, see [Best Practices Guide](BEST_PRACTICES.md)*
