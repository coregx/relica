# Query Analyzer Guide

> **Available in**: Relica v0.5.0-beta+
> **Supported databases**: PostgreSQL (MySQL & SQLite coming soon)

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

### MySQL (Coming in v0.5.0)

**Planned:**
- 🚧 EXPLAIN FORMAT=JSON
- 🚧 Index usage detection
- 🚧 Rows examined tracking
- ❌ EXPLAIN ANALYZE (not in MySQL 8.0)

### SQLite (Coming in v0.6.0)

**Planned:**
- 🚧 EXPLAIN QUERY PLAN
- 🚧 Text format parsing
- 🚧 Basic index detection
- ❌ Cost estimates (not available)
- ❌ EXPLAIN ANALYZE (not available)

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

*Guide updated*: 2025-01-24 for v0.5.0-beta
*Database support*: PostgreSQL ✅, MySQL 🚧, SQLite 🚧
