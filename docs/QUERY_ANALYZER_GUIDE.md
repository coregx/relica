# Query Analyzer Guide

> **Query Analyzer Feature**
> **Supported databases**: PostgreSQL, MySQL, SQLite

---

## Overview

The Query Analyzer allows you to analyze database query execution plans using `EXPLAIN` functionality. This helps you understand query performance, verify index usage, and optimize slow queries.

---

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "github.com/coregx/relica"
)

func main() {
    db, err := relica.NewDB("postgres", "postgres://...")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Analyze a query without executing it
    plan, err := db.Builder().
        Select("*").
        From("users").
        Where("email = ?", "alice@example.com").
        Explain()
    if err != nil {
        log.Fatal(err)
    }

    // Check the results
    fmt.Printf("Cost: %.2f\n", plan.Cost)
    fmt.Printf("Estimated Rows: %d\n", plan.EstimatedRows)
    fmt.Printf("Uses Index: %v\n", plan.UsesIndex)
    fmt.Printf("Full Scan: %v\n", plan.FullScan)
}
```

---

## API Methods

### Explain() - Analyze Without Execution

Analyzes the query plan **without executing the query**. Safe for production.

```go
plan, err := db.Builder().
    Select("*").
    From("users").
    Where("status = ?", 1).
    Explain()
```

**Returns:**
- Estimated cost
- Estimated rows
- Index usage
- Full scan detection

**Use when:**
- Testing queries before deployment
- Optimizing slow queries
- Verifying index usage
- Safe analysis (no side effects)

---

### ExplainAnalyze() - Analyze With Execution

Analyzes the query plan **and executes the query**. Returns actual metrics.

```go
plan, err := db.Builder().
    Select("*").
    From("orders").
    Where("total > ?", 1000).
    ExplainAnalyze()
```

**Returns:**
- All fields from `Explain()`
- Actual rows processed
- Actual execution time
- Buffer cache statistics (PostgreSQL)

**⚠️ WARNING**: This method **EXECUTES** the query, including:
- Side effects (triggers, etc.)
- INSERT/UPDATE/DELETE in CTEs
- Performance impact on production

**Use when:**
- Analyzing query behavior in staging
- Debugging performance issues
- Comparing estimated vs actual metrics
- Need actual execution data

---

## QueryPlan Structure

```go
type QueryPlan struct {
    // Common metrics
    Cost          float64       // Estimated total cost
    EstimatedRows int64         // Estimated rows to process
    ActualRows    int64         // Actual rows (ExplainAnalyze only)
    ActualTime    time.Duration // Actual time (ExplainAnalyze only)

    // Index analysis
    UsesIndex bool   // true if any index used
    IndexName string // Primary index name (if single)
    FullScan  bool   // true if full table scan

    // Metadata
    RawOutput string // Full EXPLAIN output
    Database  string // "postgres", "mysql", "sqlite"

    // PostgreSQL-specific
    BuffersHit  int64 // Buffer cache hits
    BuffersMiss int64 // Buffer cache misses

    // MySQL-specific (future)
    RowsExamined int64 // Rows examined
    RowsProduced int64 // Rows produced
}
```

---

## Common Use Cases

### 1. Verify Index Usage

```go
plan, err := db.Builder().
    Select("*").
    From("users").
    Where("email = ?", "alice@example.com").
    Explain()

if !plan.UsesIndex {
    log.Println("WARNING: Query not using index!")
    log.Printf("Consider: CREATE INDEX users_email_idx ON users(email)")
}
```

### 2. Detect Full Table Scans

```go
plan, err := db.Builder().
    Select("*").
    From("large_table").
    Where("status = ?", 1).
    Explain()

if plan.FullScan {
    log.Printf("WARNING: Full table scan detected!")
    log.Printf("Estimated cost: %.2f", plan.Cost)
    log.Printf("Estimated rows: %d", plan.EstimatedRows)
}
```

### 3. Compare Before/After Optimization

```go
// Before optimization
planBefore, _ := db.Builder().
    Select("*").
    From("users").
    Where("status = ?", 1).
    Explain()

// Create index
db.Exec("CREATE INDEX users_status_idx ON users(status)")

// After optimization
planAfter, _ := db.Builder().
    Select("*").
    From("users").
    Where("status = ?", 1).
    Explain()

fmt.Printf("Cost reduction: %.2f -> %.2f (%.1f%%)\n",
    planBefore.Cost,
    planAfter.Cost,
    (1 - planAfter.Cost/planBefore.Cost) * 100)
```

### 4. Analyze Join Performance

```go
plan, err := db.Builder().
    Select("u.name", "COUNT(o.id) as order_count").
    From("users u").
    LeftJoin("orders o", "u.id = o.user_id").
    GroupBy("u.id", "u.name").
    Explain()

if err != nil {
    log.Fatal(err)
}

fmt.Printf("Join cost: %.2f\n", plan.Cost)
fmt.Printf("Uses index: %v\n", plan.UsesIndex)
if plan.IndexName != "" {
    fmt.Printf("Index: %s\n", plan.IndexName)
}
```

### 5. Test Query Performance

```go
import "time"

// Analyze without execution
explainStart := time.Now()
plan, err := db.Builder().
    Select("*").
    From("large_table").
    Where("column = ?", value).
    Explain()
explainTime := time.Since(explainStart)

fmt.Printf("EXPLAIN overhead: %v\n", explainTime)
fmt.Printf("Estimated cost: %.2f\n", plan.Cost)

// Execute with analysis
analyzeStart := time.Now()
analyzePlan, err := db.Builder().
    Select("*").
    From("large_table").
    Where("column = ?", value).
    ExplainAnalyze()
analyzeTime := time.Since(analyzeStart)

fmt.Printf("Actual execution: %v\n", analyzePlan.ActualTime)
fmt.Printf("Total time (with EXPLAIN): %v\n", analyzeTime)
```

---

## Best Practices

### 1. Use Explain() in Development

```go
// During development
plan, err := query.Explain()
if err != nil {
    log.Printf("Query analysis failed: %v", err)
}
if plan.Cost > 1000 {
    log.Printf("WARNING: High cost query (%.2f)", plan.Cost)
}

// Execute the actual query
var results []Result
err = query.All(&results)
```

### 2. Test Queries Before Production

```go
func TestExpensiveQuery(t *testing.T) {
    plan, err := db.Builder().
        Select("*").
        From("users").
        InnerJoin("orders", "users.id = orders.user_id").
        Where("orders.total > ?", 1000).
        Explain()

    require.NoError(t, err)
    assert.True(t, plan.UsesIndex, "Query should use index")
    assert.False(t, plan.FullScan, "Query should not full scan")
    assert.Less(t, plan.Cost, 100.0, "Query cost should be < 100")
}
```

### 3. Monitor Production Queries

```go
func executeWithMonitoring(query *relica.SelectQuery) error {
    // Analyze first
    plan, err := query.Explain()
    if err != nil {
        log.Printf("EXPLAIN failed: %v", err)
    } else if plan.Cost > threshold {
        log.Printf("High cost query: %.2f", plan.Cost)
        metrics.RecordSlowQuery(plan)
    }

    // Execute
    return query.All(&results)
}
```

### 4. Debug Slow Queries

```go
// Get actual execution metrics
plan, err := db.Builder().
    Select("*").
    From("users").
    Where("created_at > ?", time.Now().Add(-24*time.Hour)).
    ExplainAnalyze()

if err != nil {
    log.Fatal(err)
}

fmt.Printf("Estimated: %d rows, Cost: %.2f\n",
    plan.EstimatedRows, plan.Cost)
fmt.Printf("Actual: %d rows, Time: %v\n",
    plan.ActualRows, plan.ActualTime)

// Check accuracy
ratio := float64(plan.ActualRows) / float64(plan.EstimatedRows)
if ratio > 10 || ratio < 0.1 {
    fmt.Printf("WARNING: Estimate inaccurate (ratio: %.2f)\n", ratio)
    fmt.Println("Consider: ANALYZE table")
}
```

---

## Database-Specific Notes

### PostgreSQL

**Supported:**
- ✅ EXPLAIN (FORMAT JSON)
- ✅ EXPLAIN (ANALYZE, FORMAT JSON, BUFFERS)
- ✅ Index detection (Index Scan, Index Only Scan, Bitmap Index Scan)
- ✅ Buffer statistics (Shared Hit/Read Blocks)
- ✅ Nested plans (joins, subqueries, CTEs)

**Metrics:**
- `Cost`: PostgreSQL cost units (arbitrary, relative)
- `EstimatedRows`: Planner's row estimate
- `ActualRows`: Real rows processed (ExplainAnalyze)
- `ActualTime`: Real execution time (ExplainAnalyze)
- `BuffersHit`: Pages found in cache
- `BuffersMiss`: Pages read from disk

**Example Output:**
```go
plan, _ := query.Explain()
fmt.Printf("Cost: %.2f\n", plan.Cost)                // 8.27
fmt.Printf("Estimated Rows: %d\n", plan.EstimatedRows) // 1
fmt.Printf("Uses Index: %v\n", plan.UsesIndex)         // true
fmt.Printf("Index Name: %s\n", plan.IndexName)         // users_email_idx
```

### MySQL

**Supported:**
- ✅ EXPLAIN FORMAT=JSON
- ✅ Index detection (ref, range, index, eq_ref, const)
- ✅ Full table scan detection (access_type = ALL)
- ✅ Rows examined/produced tracking
- ✅ JOIN analysis (nested loop)
- ⚠️ EXPLAIN ANALYZE (MySQL 8.0.18+ only)

**Metrics:**
- `Cost`: MySQL cost units (query_cost in JSON)
- `EstimatedRows`: Rows examined per scan
- `RowsExamined`: Total rows scanned
- `RowsProduced`: Total rows produced
- `UsesIndex`: true if index used (key != "")
- `IndexName`: Name of index used
- `FullScan`: true if access_type = "ALL"

**Example Output:**
```go
db, _ := relica.NewDB("mysql", "user:pass@tcp(localhost:3306)/db")
plan, _ := db.Builder().
    Select("*").
    From("users").
    Where("email = ?", "alice@example.com").
    Explain()

fmt.Printf("Cost: %.2f\n", plan.Cost)                  // 1.20
fmt.Printf("Estimated Rows: %d\n", plan.EstimatedRows) // 1
fmt.Printf("Uses Index: %v\n", plan.UsesIndex)         // true
fmt.Printf("Index Name: %s\n", plan.IndexName)         // email_idx
fmt.Printf("Full Scan: %v\n", plan.FullScan)           // false
```

**MySQL-Specific Notes:**
- EXPLAIN ANALYZE requires MySQL 8.0.18+
- Older MySQL versions only support EXPLAIN (estimates only)
- Access types: `ALL` (full scan), `index`, `range`, `ref`, `eq_ref`, `const`, `system`
- Cost is in MySQL-specific units (not comparable to PostgreSQL)

### SQLite

**Supported:**
- ✅ EXPLAIN QUERY PLAN (text format)
- ✅ Index detection (USING INDEX, COVERING INDEX)
- ✅ Full table scan detection (SCAN without index)
- ✅ Primary key usage detection
- ✅ Automatic index detection
- ❌ Cost estimates (not provided by SQLite)
- ❌ Row estimates (not provided by SQLite)
- ❌ EXPLAIN ANALYZE (SQLite doesn't support this)

**Metrics:**
- `Cost`: Always 0 (SQLite doesn't provide cost)
- `EstimatedRows`: Always 0 (SQLite doesn't provide estimates)
- `UsesIndex`: true if "USING INDEX" or "USING INTEGER PRIMARY KEY" found
- `IndexName`: Name of index used (or "PRIMARY KEY")
- `FullScan`: true if "SCAN" without "USING"
- `RawOutput`: Text output from EXPLAIN QUERY PLAN

**Example Output:**
```go
db, _ := relica.NewDB("sqlite3", "file:test.db")
plan, _ := db.Builder().
    Select("*").
    From("users").
    Where("email = ?", "alice@example.com").
    Explain()

fmt.Printf("Uses Index: %v\n", plan.UsesIndex)    // true
fmt.Printf("Index Name: %s\n", plan.IndexName)    // email_idx
fmt.Printf("Full Scan: %v\n", plan.FullScan)      // false
fmt.Printf("Database: %s\n", plan.Database)       // sqlite

// Raw output example:
// SEARCH users USING INDEX email_idx (email=?)
fmt.Println(plan.RawOutput)
```

**SQLite-Specific Notes:**
- EXPLAIN QUERY PLAN returns text, not JSON (unlike PostgreSQL/MySQL)
- No cost estimates or row counts available
- ExplainAnalyze() returns error (not supported by SQLite)
- Index usage patterns:
  - `SEARCH ... USING INDEX idx_name` - Index scan
  - `SEARCH ... USING COVERING INDEX idx_name` - Covering index (no table lookup)
  - `SEARCH ... USING INTEGER PRIMARY KEY` - Primary key lookup
  - `SEARCH ... USING AUTOMATIC ...` - Automatically created temporary index
  - `SCAN table_name` - Full table scan (no index)
- SQLite's output format is simpler but less detailed than PostgreSQL/MySQL

**Example Use Cases:**

1. **Verify index usage:**
```go
plan, _ := db.Builder().
    Select("*").
    From("users").
    Where("email = ?", "alice@example.com").
    Explain()

if !plan.UsesIndex {
    log.Println("WARNING: Query not using index!")
    log.Printf("Consider: CREATE INDEX idx_email ON users(email)")
}
```

2. **Detect full table scans:**
```go
plan, _ := db.Builder().
    Select("*").
    From("large_table").
    Where("status = ?", 1).
    Explain()

if plan.FullScan {
    log.Printf("WARNING: Full table scan detected on large_table")
    log.Printf("Raw plan: %s", plan.RawOutput)
}
```

3. **Verify covering index:**
```go
// Query only indexed columns
plan, _ := db.Builder().
    Select("email").
    From("users").
    Where("email = ?", "alice@example.com").
    Explain()

// Check for "COVERING INDEX" in raw output
if strings.Contains(strings.ToUpper(plan.RawOutput), "COVERING INDEX") {
    log.Println("Using covering index (no table access needed)")
}
```

4. **Analyze JOIN queries:**
```go
plan, _ := db.Builder().
    Select("u.name", "o.total").
    From("users u").
    InnerJoin("orders o", "u.id = o.user_id").
    Where("u.email = ?", "alice@example.com").
    Explain()

// SQLite shows plan for each table
fmt.Println(plan.RawOutput)
// Output:
// SEARCH users USING INDEX idx_email (email=?)
// SEARCH orders USING INDEX idx_user_id (user_id=?)

if plan.UsesIndex {
    log.Printf("JOIN uses indexes: %s", plan.IndexName)
}
```

---

## Error Handling

```go
plan, err := db.Builder().
    Select("*").
    From("users").
    Where("email = ?", email).
    Explain()

if err != nil {
    // Handle errors
    switch {
    case strings.Contains(err.Error(), "not supported"):
        log.Printf("EXPLAIN not supported for this database")
    case strings.Contains(err.Error(), "syntax error"):
        log.Printf("Invalid SQL: %v", err)
    default:
        log.Printf("EXPLAIN failed: %v", err)
    }

    // Fall back to direct execution
    return query.All(&results)
}

// Use plan
if plan.Cost > 1000 {
    log.Printf("WARNING: Expensive query (cost: %.2f)", plan.Cost)
}
```

---

## Performance Considerations

### EXPLAIN Overhead

- **Postgres EXPLAIN**: ~1-10ms (planning only, no execution)
- **Postgres EXPLAIN ANALYZE**: Same as query execution + ~1ms
- **Parsing overhead**: ~25μs (JSON parsing)

### When to Use

✅ **Good use cases:**
- Development/testing
- Pre-production optimization
- Debugging slow queries
- Automated testing

❌ **Avoid:**
- Every production query (adds overhead)
- High-frequency queries (unless debugging)
- Queries with side effects (use Explain, not ExplainAnalyze)

---

## Troubleshooting

### "EXPLAIN not supported for driver: X"

```go
plan, err := query.Explain()
// Error: EXPLAIN not supported for driver: sqlite
```

**Solution**: Check database support matrix. Use PostgreSQL for now, MySQL/SQLite coming soon.

### Empty IndexName Even Though UsesIndex = true

This happens when multiple indexes are used (e.g., Bitmap Index Scan combining multiple indexes).

```go
if plan.UsesIndex && plan.IndexName == "" {
    // Check RawOutput for details
    fmt.Println("Multiple indexes used:")
    fmt.Println(plan.RawOutput)
}
```

### ActualRows Much Higher Than EstimatedRows

Database statistics may be outdated.

```sql
-- PostgreSQL
ANALYZE users;

-- MySQL
ANALYZE TABLE users;
```

---

## Examples Repository

See `docs/examples/query_analyzer/` for more examples:
- `basic_usage.go` - Simple queries
- `join_analysis.go` - Complex joins
- `optimization_workflow.go` - Before/after comparisons
- `production_monitoring.go` - Real-time monitoring

---

*Guide updated*: 2025-01-24
*Database support*: PostgreSQL ✅, MySQL ✅, SQLite ✅
