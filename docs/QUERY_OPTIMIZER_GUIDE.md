# Query Optimizer Guide

> **Relica v0.5.0+** - Automatic query performance optimization and analysis

The Query Optimizer automatically analyzes query performance, detects issues, and provides actionable optimization suggestions.

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [Understanding Suggestions](#understanding-suggestions)
- [Database-Specific Details](#database-specific-details)
- [Best Practices](#best-practices)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Query Optimizer runs **asynchronously** after each SELECT query execution, analyzing:
- Query execution time
- EXPLAIN plan output
- Index usage
- Full table scans

It provides **zero-overhead** optimization suggestions without blocking query execution.

### How It Works

```
┌─────────────┐
│ Query.All() │─────► Execute query (normal flow)
└─────────────┘            │
                           ▼
                    ┌──────────────┐
                    │ Measure time │
                    └──────────────┘
                           │
                           ▼
              ┌─────────────────────────┐
              │ Optimizer (async)       │
              │ 1. Get EXPLAIN plan     │
              │ 2. Detect issues        │
              │ 3. Generate suggestions │
              └─────────────────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │ Log to stderr │
                    └──────────────┘
```

---

## Features

### Phase 1 (Current)

- ✅ **Slow Query Detection** - Identifies queries exceeding threshold
- ✅ **Full Scan Detection** - Detects table scans without indexes
- ✅ **Index Recommendations** - Suggests missing indexes with CREATE INDEX SQL
- ✅ **Multi-Database Support** - PostgreSQL, MySQL, SQLite
- ✅ **Async Analysis** - Zero impact on query performance
- ✅ **Configurable Thresholds** - Customize slow query detection

### Coming Soon (Phase 2)

- 🚧 Advanced index selection (composite indexes, covering indexes)
- 🚧 JOIN optimization suggestions
- 🚧 Query rewriting recommendations
- 🚧 N+1 query detection
- 🚧 Structured logging integration
- 🚧 Metrics export (Prometheus, OpenTelemetry)

---

## Getting Started

### 1. Enable Optimizer

```go
import (
    "time"
    "github.com/coregx/relica"
)

func main() {
    // Create optimizer with 100ms slow query threshold
    db, err := relica.Open("postgres", dsn,
        relica.WithOptimizer(100 * time.Millisecond),
    )
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // Queries now automatically analyzed!
    var users []User
    err = db.Builder().
        Select("*").
        From("users").
        Where("status = ?", 1).
        All(&users)
}
```

### 2. See Suggestions

Optimizer outputs to `stderr`:

```
[RELICA OPTIMIZER] warning: Query took 250ms (threshold: 100ms)
[RELICA OPTIMIZER] warning: Query is performing a full table scan
[RELICA OPTIMIZER] warning: Consider adding index on users(status): WHERE clause filtering without index usage
  Fix: CREATE INDEX idx_users_status ON users(status);
```

---

## Configuration

### Slow Query Threshold

Control when queries are flagged as slow:

```go
// Default: 100ms
db, _ := relica.Open("postgres", dsn,
    relica.WithOptimizer(100 * time.Millisecond),
)

// Stricter: 50ms
db, _ := relica.Open("postgres", dsn,
    relica.WithOptimizer(50 * time.Millisecond),
)

// Lenient: 500ms
db, _ := relica.Open("postgres", dsn,
    relica.WithOptimizer(500 * time.Millisecond),
)
```

### Disable Optimizer

Simply don't pass `WithOptimizer` option:

```go
// No optimizer
db, _ := relica.Open("postgres", dsn)
```

---

## Understanding Suggestions

### Suggestion Types

| Type | Severity | Description | Action |
|------|----------|-------------|--------|
| **slow_query** | warning | Query exceeded threshold | Optimize query or add indexes |
| **full_scan** | warning | Table scan without index | Add appropriate index |
| **index_missing** | warning | Recommended index for WHERE clause | Run provided CREATE INDEX SQL |

### Suggestion Format

```
[RELICA OPTIMIZER] <severity>: <message>
  Fix: <optional SQL>
```

**Example:**
```
[RELICA OPTIMIZER] warning: Consider adding index on orders(user_id, status): WHERE clause filtering without index usage
  Fix: CREATE INDEX idx_orders_user_id_status ON orders(user_id, status);
```

---

## Database-Specific Details

### PostgreSQL

**Supported:**
- ✅ EXPLAIN (JSON format)
- ✅ EXPLAIN ANALYZE with BUFFERS
- ✅ Index scan detection (Index Scan, Index Only Scan, Bitmap Index Scan)
- ✅ Sequential scan detection

**Example EXPLAIN Output:**
```json
{
  "Plan": {
    "Node Type": "Seq Scan",
    "Relation Name": "users",
    "Total Cost": 1234.56,
    "Plan Rows": 10000
  }
}
```

### MySQL

**Supported:**
- ✅ EXPLAIN (JSON format, MySQL 8.0+)
- ✅ Index usage detection (type: "index", "ref", "range")
- ✅ Full scan detection (type: "ALL")
- ✅ Rows examined metrics

**Example EXPLAIN Output:**
```json
{
  "query_block": {
    "table": {
      "table_name": "users",
      "access_type": "ALL",
      "rows_examined_per_scan": 10000
    }
  }
}
```

### SQLite

**Supported:**
- ✅ EXPLAIN QUERY PLAN
- ✅ SCAN vs SEARCH detection
- ✅ Index usage detection ("USING INDEX")
- ✅ Cost-based optimization metrics

**Example EXPLAIN Output:**
```
SCAN users
```

---

## Best Practices

### 1. Set Appropriate Thresholds

Choose thresholds based on your application's performance requirements:

```go
// Web API (fast response required)
relica.WithOptimizer(50 * time.Millisecond)

// Background job (can be slower)
relica.WithOptimizer(500 * time.Millisecond)

// Data analytics (complex queries expected)
relica.WithOptimizer(5 * time.Second)
```

### 2. Review Suggestions Regularly

Monitor optimizer output in development:

```bash
# Capture optimizer suggestions
./myapp 2>&1 | grep "RELICA OPTIMIZER"
```

### 3. Apply Index Recommendations Carefully

**Before creating indexes:**

1. **Verify the suggestion** with manual EXPLAIN
2. **Consider index overhead** (write performance impact)
3. **Test in staging** environment first
4. **Monitor query performance** after creating index

**Example workflow:**

```sql
-- 1. Check current plan
EXPLAIN SELECT * FROM users WHERE status = 1;

-- 2. Create recommended index
CREATE INDEX idx_users_status ON users(status);

-- 3. Verify improvement
EXPLAIN SELECT * FROM users WHERE status = 1;
```

### 4. Disable in Production (Optional)

If you don't want optimizer overhead in production:

```go
var optimizerThreshold time.Duration
if os.Getenv("ENV") == "production" {
    // Disabled (no optimizer)
    db, _ := relica.Open("postgres", dsn)
} else {
    // Enabled in dev/staging
    db, _ := relica.Open("postgres", dsn,
        relica.WithOptimizer(100 * time.Millisecond),
    )
}
```

---

## Examples

### Example 1: Slow Query with Full Scan

**Code:**
```go
var users []User
err := db.Builder().
    Select("*").
    From("users").
    Where("status = ?", 1).
    All(&users)
```

**Optimizer Output:**
```
[RELICA OPTIMIZER] warning: Query took 250ms (threshold: 100ms)
[RELICA OPTIMIZER] warning: Query is performing a full table scan
[RELICA OPTIMIZER] warning: Consider adding index on users(status): WHERE clause filtering without index usage
  Fix: CREATE INDEX idx_users_status ON users(status);
```

**Fix:**
```sql
CREATE INDEX idx_users_status ON users(status);
```

### Example 2: Multiple WHERE Conditions

**Code:**
```go
var orders []Order
err := db.Builder().
    Select("*").
    From("orders").
    Where("user_id = ? AND status = ?", 123, "pending").
    All(&orders)
```

**Optimizer Output:**
```
[RELICA OPTIMIZER] warning: Consider adding index on orders(user_id, status): WHERE clause filtering without index usage
  Fix: CREATE INDEX idx_orders_user_id_status ON orders(user_id, status);
```

**Fix:**
```sql
-- Composite index for better performance
CREATE INDEX idx_orders_user_id_status ON orders(user_id, status);
```

### Example 3: Fast Query (No Suggestions)

**Code:**
```go
// Assuming email has index
var user User
err := db.Builder().
    Select("*").
    From("users").
    Where("email = ?", "alice@example.com").
    One(&user)
```

**Optimizer Output:**
```
(no output - query is optimal)
```

---

## Troubleshooting

### Issue: No Suggestions Appearing

**Possible causes:**

1. **Optimizer not enabled**
   ```go
   // Check you're using WithOptimizer
   db, _ := relica.Open("postgres", dsn,
       relica.WithOptimizer(100 * time.Millisecond),
   )
   ```

2. **Queries too fast**
   ```go
   // Lower threshold to see more suggestions
   relica.WithOptimizer(10 * time.Millisecond)
   ```

3. **stderr not visible**
   ```bash
   # Ensure stderr is captured
   ./myapp 2>&1 | tee app.log
   ```

### Issue: Too Many Suggestions

**Solutions:**

1. **Increase threshold**
   ```go
   relica.WithOptimizer(500 * time.Millisecond)
   ```

2. **Apply recommended indexes**
   ```sql
   -- Create suggested indexes
   CREATE INDEX idx_users_status ON users(status);
   ```

3. **Disable for certain queries**
   ```go
   // Optimizer is per-DB, not per-query
   // Create separate DB instance without optimizer
   ```

### Issue: Incorrect Index Recommendations

The Phase 1 optimizer uses **basic WHERE clause parsing**. For complex queries, manually verify:

```sql
-- Always verify with EXPLAIN before creating index
EXPLAIN SELECT * FROM users WHERE ...;
```

**Known limitations (Phase 1):**
- Only analyzes WHERE clause (not JOIN, ORDER BY, GROUP BY)
- Simple pattern matching (may miss complex conditions)
- Doesn't consider existing indexes

---

## Phase 2: Advanced Index Analysis (v0.5.1+)

### New Features

Phase 2 introduces sophisticated query analysis:

#### 1. Smart WHERE Clause Parsing
Accurately extracts:
- Column names and operators (=, >, <, IN, LIKE, etc.)
- AND/OR logic
- Function calls (UPPER, LOWER, etc.)
- Multiple conditions

#### 2. Composite Index Recommendations
Detects multi-column AND conditions:

```go
// Query with multiple AND conditions
db.Builder().
    Select("*").
    From("users").
    Where("status = ? AND country = ?", 1, "US").
    All(&users)
```

**Optimizer Output:**
```
[RELICA OPTIMIZER] warning: Composite index recommended on users(status, country): Composite index for multiple AND conditions
  Fix: CREATE INDEX idx_users_status_country ON users(status, country);
```

**Benefits:**
- Faster filtering (single index scan vs multiple)
- Reduces rows read significantly
- Optimal for queries with multiple filters

**Column Order Matters:**
1. Place equality conditions first
2. Range conditions last
3. Most selective column first

#### 3. JOIN Optimization

Automatically detects missing foreign key indexes:

```go
// Query with JOIN
db.Builder().
    Select("u.*, o.total").
    From("users u").
    Join("orders o", "u.id = o.user_id").
    Where("u.status = ?", 1).
    All(&results)
```

**Optimizer Output:**
```
[RELICA OPTIMIZER] warning: Index recommended on orders(user_id): JOIN condition - index on foreign key
  Fix: CREATE INDEX idx_orders_user_id ON orders(user_id);
```

**Benefits:**
- Eliminates N+1 JOIN problems
- Faster JOIN operations (nested loop → index scan)
- Critical for normalized schemas

#### 4. Covering Index Detection

Identifies opportunities for index-only scans:

```go
// Query selecting specific columns
db.Builder().
    Select("id", "name").
    From("users").
    Where("status = ?", 1).
    All(&users)
```

**Optimizer Output:**
```
[RELICA OPTIMIZER] info: Covering index recommended on users(status, id, name): Covering index: Index-only scan (no table access needed)
  Fix: CREATE INDEX idx_users_status_id_name ON users(status, id, name);
```

**Benefits:**
- Database reads only index (no table access)
- Reduces I/O significantly
- Faster query execution

**Sweet Spot**: 2-5 columns (WHERE + SELECT)

**Trade-offs:**
- Larger index size
- Slower writes (more index data to update)
- Best for read-heavy workloads

#### 5. Function-Based Index Warnings

Detects functions in WHERE preventing index use:

```go
// Function in WHERE clause
db.Builder().
    Select("*").
    From("users").
    Where("UPPER(email) = ?", "ALICE@EXAMPLE.COM").
    All(&users)
```

**Optimizer Output:**
```
[RELICA OPTIMIZER] warning: Function-based index recommended on users(email): Function UPPER() in WHERE prevents index use - consider function-based index
  Fix: CREATE INDEX idx_users_email ON users(email);
```

**Solutions:**
1. **Function-based index** (PostgreSQL):
   ```sql
   CREATE INDEX idx_users_email_upper ON users(UPPER(email));
   ```
2. **Generated column** (MySQL 8.0+):
   ```sql
   ALTER TABLE users ADD COLUMN email_upper VARCHAR(255) AS (UPPER(email)) STORED;
   CREATE INDEX idx_users_email_upper ON users(email_upper);
   ```
3. **Query rewriting**:
   ```go
   // Store normalized values in application
   db.Builder().Where("email = ?", strings.ToUpper(email))
   ```

### Phase 2 Suggestion Types

| Type | Severity | Description |
|------|----------|-------------|
| `composite_index` | Warning | Multi-column index for AND conditions |
| `covering_index` | Info | Index-only scan optimization |
| `join_optimize` | Warning | Foreign key index for JOINs |
| `function_index` | Warning | Function in WHERE prevents index use |
| `query_rewrite` | Info | Suggests query rewriting (future) |

### Phase 2 Best Practices

#### 1. Composite Index Guidelines

**Good:**
```sql
-- Equality conditions first
CREATE INDEX idx_users_status_country ON users(status, country);
```

**Bad:**
```sql
-- Range condition first (less selective)
CREATE INDEX idx_users_created_country ON users(created_at, country);
```

#### 2. Covering Index When to Use

**Use When:**
- Read-heavy workload (80%+ reads)
- Query runs frequently (thousands/day)
- 2-5 columns total
- SELECT column list is stable

**Avoid When:**
- Write-heavy workload
- Too many columns (>5)
- Rarely-run queries
- SELECT * queries

#### 3. JOIN Index Priority

**High Priority:**
- Foreign key columns in child tables
- Frequently joined tables
- Large tables (1M+ rows)

**Low Priority:**
- Rarely joined tables
- Small lookup tables (<1000 rows)
- Tables with existing indexes

### Phase 2 Examples

#### Example: Multi-Column Filtering

**Before optimization:**
```go
// No indexes
var users []User
db.Builder().
    Select("*").
    From("users").
    Where("status = ? AND country = ? AND age > ?", 1, "US", 18).
    All(&users)
// Execution: 500ms (full scan)
```

**After composite index:**
```sql
CREATE INDEX idx_users_status_country_age ON users(status, country, age);
```

```go
// Same query
var users []User
db.Builder().
    Select("*").
    From("users").
    Where("status = ? AND country = ? AND age > ?", 1, "US", 18).
    All(&users)
// Execution: 5ms (index scan) - 100x faster!
```

#### Example: JOIN Performance

**Before optimization:**
```go
// No index on orders.user_id
var results []struct {
    UserID int
    Orders int
}
db.Builder().
    Select("u.id", "COUNT(o.id)").
    From("users u").
    Join("orders o", "u.id = o.user_id").
    GroupBy("u.id").
    All(&results)
// Execution: 2000ms (nested loop scan)
```

**After JOIN index:**
```sql
CREATE INDEX idx_orders_user_id ON orders(user_id);
```

```go
// Same query
// Execution: 50ms (index nested loop) - 40x faster!
```

#### Example: Covering Index

**Before optimization:**
```go
// Query with index on status, but accesses table for id, name
var users []struct {
    ID     int    `db:"id"`
    Name   string `db:"name"`
    Status int    `db:"status"`
}
db.Builder().
    Select("id", "name", "status").
    From("users").
    Where("status = ?", 1).
    All(&users)
// Execution: 50ms (index scan + table lookup)
```

**After covering index:**
```sql
CREATE INDEX idx_users_status_id_name ON users(status, id, name);
```

```go
// Same query
// Execution: 10ms (index-only scan) - 5x faster!
```

---

---

## Performance Impact

### Optimizer Overhead

| Operation | Time | Impact |
|-----------|------|--------|
| Query execution | Normal | ✅ Zero (async) |
| EXPLAIN query | ~1-5ms | ⚡ Async (non-blocking) |
| Suggestion generation | ~0.1ms | ⚡ Async (non-blocking) |
| Total overhead | **~5ms async** | ✅ No impact on response time |

### Memory Usage

- Minimal: ~1KB per analyzed query
- Goroutine per query (cleaned up after analysis)
- No persistent storage

---

## API Reference

### Constructor

```go
func WithOptimizer(threshold time.Duration) Option
```

Creates an optimizer option for `relica.Open()`.

**Parameters:**
- `threshold`: Slow query threshold (0 = default 100ms)

**Returns:**
- `Option`: Configuration option for `Open()`

**Example:**
```go
db, _ := relica.Open("postgres", dsn,
    relica.WithOptimizer(100 * time.Millisecond),
)
```

### Analyzer Interface (Advanced)

For custom optimizers:

```go
type Analyzer interface {
    Explain(ctx context.Context, query string, args []interface{}) (*QueryPlan, error)
    ExplainAnalyze(ctx context.Context, query string, args []interface{}) (*QueryPlan, error)
}
```

---

## Related Documentation

- [EXPLAIN Guide](./EXPLAIN_GUIDE.md) - Understanding EXPLAIN output
- [Performance Tuning](./PERFORMANCE_TUNING.md) - General optimization tips
- [Indexing Strategies](./INDEXING_STRATEGIES.md) - Best practices for indexes

---

## Changelog

### v0.5.1 (Current)
- ✅ Phase 2: Advanced Index Analysis
  - Smart WHERE clause parsing (AND/OR logic, operators, functions)
  - Composite index recommendations
  - JOIN optimization (foreign key indexes)
  - Covering index detection
  - Function-based index warnings
  - 89.9% test coverage

### v0.5.0
- ✅ Phase 1: Foundation
  - Slow query detection
  - Full scan detection
  - Basic index recommendations
  - PostgreSQL, MySQL, SQLite support

### v0.6.0 (Planned)
- 🚧 Phase 3: Query Rewriting
  - Automatic query optimization
  - N+1 query detection
  - Subquery optimization
  - Partition recommendations

---

## Feedback & Support

Found a bug or have a suggestion?

- **GitHub Issues**: https://github.com/coregx/relica/issues
- **Discussions**: https://github.com/coregx/relica/discussions

---

*Last Updated: 2025-01-13*
*Relica Version: v0.5.0-beta*
