# Query Optimizer

> **Status**: Not available in the public API: the query optimizer exists as internal groundwork and cannot be enabled by users (planned).

There is no option that turns on automatic optimization or index suggestions. What you can use today:

**See the SQL a builder produces** — `ToSQL()` returns the SQL and its arguments without running it:

```go
sql, args := db.Select("id", "name").
    From("users").
    Where(relica.Eq("status", "active")).
    OrderBy("name").
    ToSQL()
// PostgreSQL: SELECT "id", "name" FROM "users" WHERE "status" = $1 ORDER BY "name"  [active]
```

**Inspect the execution plan** — `Explain()` and `ExplainAnalyze()` on a SELECT return a `*relica.QueryPlan` (cost, estimated rows, index use, full scan, raw EXPLAIN output):

```go
plan, err := db.Select().From("users").Where(relica.Eq("email", email)).Explain()
if err == nil && plan.FullScan {
    log.Printf("full table scan, consider an index: %s", plan.RawOutput)
}
```

`ExplainAnalyze()` executes the query. Details: [Query Analyzer Guide](QUERY_ANALYZER_GUIDE.md).

**Measure real queries** — a `relica.WithQueryHook` receives each statement's SQL and duration; see the [Logging Guide](LOGGING_GUIDE.md). Back to the [README](../README.md).
