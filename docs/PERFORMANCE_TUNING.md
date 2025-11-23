# Performance Tuning Guide

> **Optimize Relica for production workloads**
>
> This guide covers connection pooling, health monitoring, cache warming, and best practices.

---

## Table of Contents

1. [Connection Pool Configuration](#connection-pool-configuration)
2. [Health Monitoring](#health-monitoring)
3. [Statement Cache Optimization](#statement-cache-optimization)
4. [Production Checklist](#production-checklist)
5. [Troubleshooting](#troubleshooting)

---

## Connection Pool Configuration

### Basic Pool Settings

Configure connection pool for optimal resource usage:

```go
db, err := relica.Open("postgres", dsn,
    relica.WithMaxOpenConns(100),      // Maximum connections
    relica.WithMaxIdleConns(50),       // Idle connections in pool
    relica.WithConnMaxLifetime(5*time.Minute),   // Connection reuse limit
    relica.WithConnMaxIdleTime(1*time.Minute),   // Idle connection timeout
)
```

### Recommended Settings by Workload

#### High-Traffic Web Application (1000+ req/s)
```go
relica.WithMaxOpenConns(100)           // Match database max_connections / 2
relica.WithMaxIdleConns(50)            // 50% of max open
relica.WithConnMaxLifetime(5*time.Minute)
relica.WithConnMaxIdleTime(30*time.Second)
```

#### Microservice (Moderate Load)
```go
relica.WithMaxOpenConns(25)
relica.WithMaxIdleConns(10)
relica.WithConnMaxLifetime(10*time.Minute)
relica.WithConnMaxIdleTime(2*time.Minute)
```

#### Background Worker (Low Concurrency)
```go
relica.WithMaxOpenConns(5)
relica.WithMaxIdleConns(2)
relica.WithConnMaxLifetime(15*time.Minute)
relica.WithConnMaxIdleTime(5*time.Minute)
```

### Monitoring Pool Health

Get real-time pool statistics:

```go
stats := db.Stats()
fmt.Printf("Open: %d/%d, Idle: %d, InUse: %d, WaitCount: %d\n",
    stats.OpenConnections,
    stats.MaxOpenConnections,
    stats.Idle,
    stats.InUse,
    stats.WaitCount,
)

// Check if connections are being closed due to limits
if stats.MaxIdleClosed > 1000 {
    log.Warn("Many connections closed due to MaxIdleConns - consider increasing")
}

if stats.MaxLifetimeClosed > 1000 {
    log.Info("Connections rotated due to MaxLifetime - this is normal")
}
```

---

## Health Monitoring

### Enable Health Checks

Periodic database pings detect dead connections early:

```go
db, err := relica.Open("postgres", dsn,
    relica.WithHealthCheck(30*time.Second),  // Ping every 30 seconds
)

// Check health status
if !db.IsHealthy() {
    log.Error("Database unhealthy - consider reconnecting")
    // Trigger alerts, circuit breaker, etc.
}
```

### Recommended Intervals

- **Production**: 30-60 seconds (balance between detection speed and overhead)
- **Development**: 10 seconds (faster problem detection)
- **Critical Systems**: 15 seconds (early warning)

### Health Check Overhead

- Each ping: ~1-5ms (negligible)
- Background goroutine: ~1KB memory
- **Disable** if using external health monitoring (e.g., Kubernetes liveness probes)

---

## Statement Cache Optimization

### Cache Warming at Startup

Pre-load frequently-used queries to eliminate startup cache misses:

```go
func warmDatabase(db *relica.DB) error {
    queries := []string{
        // User queries
        "SELECT * FROM users WHERE id = ?",
        "SELECT * FROM users WHERE email = ?",

        // Session queries
        "SELECT * FROM sessions WHERE token = ?",
        "INSERT INTO sessions (user_id, token, expires) VALUES (?, ?, ?)",

        // Logging
        "INSERT INTO audit_log (user_id, action, details) VALUES (?, ?, ?)",
    }

    n, err := db.WarmCache(queries)
    if err != nil {
        return fmt.Errorf("cache warming failed: %w", err)
    }

    log.Info("Cache warmed", "queries", n)
    return nil
}
```

### Pinning Critical Queries

Prevent eviction of hot-path queries:

```go
// Warm cache
db.WarmCache([]string{
    "SELECT * FROM users WHERE id = ?",
    "SELECT * FROM sessions WHERE token = ?",
})

// Pin critical queries
db.PinQuery("SELECT * FROM users WHERE id = ?")
db.PinQuery("SELECT * FROM sessions WHERE token = ?")

// These will NEVER be evicted from cache
```

### When to Pin Queries

✅ **Pin these:**
- Authentication queries (session lookup, user lookup)
- Authorization checks (permission queries)
- Hot-path queries (executed >100 times/second)

❌ **Don't pin:**
- Report queries (low frequency)
- Admin queries (not performance-critical)
- Dynamic queries with many variations

### Cache Size Tuning

```go
// Default: 1000 statements
db, err := relica.Open("postgres", dsn,
    relica.WithStmtCacheCapacity(2000),  // Increase for large apps
)
```

**Guidelines:**
- Small app (<10 query types): 100-500
- Medium app (10-50 query types): 1000 (default)
- Large app (50+ query types): 2000-5000
- Microservice: 200-500 (focused domain)

**Monitoring:**
```go
cache := db.Stats()  // Assuming cache stats exposed
if cache.HitRate < 0.95 {
    log.Warn("Cache hit rate low - consider increasing capacity",
        "hitRate", cache.HitRate)
}
```

---

## Production Checklist

### Before Deployment

- [ ] Connection pool configured for workload
- [ ] Health checks enabled (if not using external monitoring)
- [ ] Cache warmed at startup
- [ ] Critical queries pinned
- [ ] Monitoring/alerting on pool stats
- [ ] Database `max_connections` sufficient (app pool × 2)

### Configuration Example

```go
func NewDatabase(dsn string) (*relica.DB, error) {
    db, err := relica.Open("postgres", dsn,
        // Connection pool
        relica.WithMaxOpenConns(100),
        relica.WithMaxIdleConns(50),
        relica.WithConnMaxLifetime(5*time.Minute),
        relica.WithConnMaxIdleTime(1*time.Minute),

        // Health monitoring
        relica.WithHealthCheck(30*time.Second),

        // Cache
        relica.WithStmtCacheCapacity(2000),

        // Logging (optional)
        relica.WithLogger(logger.NewSlogAdapter(slog.Default())),
    )
    if err != nil {
        return nil, err
    }

    // Warm cache
    if err := warmDatabase(db); err != nil {
        db.Close()
        return nil, err
    }

    return db, nil
}
```

---

## Troubleshooting

### High Connection Wait Times

**Symptom:** `stats.WaitCount` increasing rapidly, `stats.WaitDuration` high

**Solutions:**
1. Increase `MaxOpenConns` (if database allows)
2. Reduce query execution time (add indexes, optimize queries)
3. Implement connection pooling per-service (if microservices)

```go
stats := db.Stats()
if stats.WaitDuration > 100*time.Millisecond {
    log.Warn("High connection wait time",
        "waitDuration", stats.WaitDuration,
        "waitCount", stats.WaitCount,
        "maxOpen", stats.MaxOpenConnections,
    )
}
```

### Connection Exhaustion

**Symptom:** "Too many connections" errors from database

**Solutions:**
1. Lower `MaxOpenConns` across all services
2. Check for connection leaks (transactions not committed/rolled back)
3. Verify database `max_connections` setting

```bash
# PostgreSQL: Check max_connections
SHOW max_connections;

# MySQL: Check max_connections
SHOW VARIABLES LIKE 'max_connections';
```

### Low Cache Hit Rate

**Symptom:** Cache hit rate < 90%

**Solutions:**
1. Increase cache capacity
2. Pin more frequently-used queries
3. Reduce query variations (use parameterized queries)

### Health Check Failures

**Symptom:** `db.IsHealthy()` returns false

**Possible Causes:**
- Network issues
- Database restart
- Firewall blocking connections
- Connection pool exhausted

**Response:**
```go
if !db.IsHealthy() {
    stats := db.Stats()
    log.Error("Database unhealthy",
        "healthy", stats.Healthy,
        "lastCheck", stats.LastHealthCheck,
        "openConns", stats.OpenConnections,
    )

    // Implement circuit breaker, retry logic, etc.
}
```

---

## Advanced Topics

### Database-Specific Tuning

#### PostgreSQL
```go
// Use connection pooler (pgBouncer)
dsn := "host=pgbouncer port=6432 user=app dbname=mydb pool_mode=transaction"

// Lower pool size (pgBouncer handles pooling)
relica.WithMaxOpenConns(20)
relica.WithMaxIdleConns(10)
```

#### MySQL
```go
// Increase connection lifetime (MySQL has higher connection overhead)
relica.WithConnMaxLifetime(15*time.Minute)

// Tune thread cache
// SET GLOBAL thread_cache_size = 100;
```

#### SQLite
```go
// Single connection for writes (SQLite limitation)
relica.WithMaxOpenConns(1)  // Write connection
relica.WithMaxIdleConns(1)

// Read-only connections can scale
```

### Load Testing

Test pool configuration under load:

```bash
# Use wrk or similar
wrk -t12 -c400 -d30s http://localhost:8080/api/users

# Monitor during test
watch -n1 'psql -c "SELECT count(*) FROM pg_stat_activity"'
```

---

**See Also:**
- [LOGGING_GUIDE.md](./LOGGING_GUIDE.md) - Query logging and tracing
- [QUERY_OPTIMIZER_GUIDE.md](./QUERY_OPTIMIZER_GUIDE.md) - Query optimization

---

*Last Updated: 2025-11-13*
