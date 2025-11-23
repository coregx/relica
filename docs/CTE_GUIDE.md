# CTE Guide (Common Table Expressions)

> Complete guide to using CTEs (WITH clauses) in Relica for cleaner, more maintainable queries

## Table of Contents
- [Introduction](#introduction)
- [Basic CTEs](#basic-ctes)
- [Multiple CTEs](#multiple-ctes)
- [Recursive CTEs](#recursive-ctes)
- [CTE vs Subqueries](#cte-vs-subqueries)
- [When to Use CTEs](#when-to-use-cttes)
- [Performance Considerations](#performance-considerations)
- [Database Compatibility](#database-compatibility)
- [Best Practices](#best-practices)
- [Common Patterns](#common-patterns)
- [Troubleshooting](#troubleshooting)

## Introduction

Common Table Expressions (CTEs) are named temporary result sets defined using the `WITH` clause. Think of them as "query variables" that make complex queries more readable and maintainable.

**Key Benefits**:
- **Readability**: Break complex queries into logical steps
- **Reusability**: Reference CTE multiple times in same query
- **Recursion**: Traverse hierarchical data (org charts, trees)
- **Maintainability**: Easier to modify and debug
- **Self-documenting**: Named CTEs explain query logic


## Basic CTEs

### Simple CTE

```go
package main

import (
    "github.com/coregx/relica"
)

// Find high-value customers using CTE
func GetHighValueCustomers(db *relica.DB) ([]Customer, error) {
    // CTE: Calculate total spending per user
    orderTotals := db.Builder().
        Select("user_id", "SUM(total) as total_spent").
        From("orders").
        GroupBy("user_id")

    // Main query: Filter high-value customers
    var customers []Customer
    err := db.Builder().
        Select("*").
        With("order_totals", orderTotals).
        From("order_totals").
        Where("total_spent > ?", 1000).
        All(&customers)

    return customers, err
}
```

**Generated SQL** (PostgreSQL):
```sql
WITH "order_totals" AS (
    SELECT "user_id", SUM(total) as total_spent
    FROM "orders"
    GROUP BY "user_id"
)
SELECT * FROM "order_totals" WHERE total_spent > $1
```

**Explanation**:
1. Define CTE named `order_totals`
2. Reference it in main query like a table
3. Apply filtering on aggregated results

### CTE with JOIN

```go
// Join CTE results with another table
func GetTopCustomerDetails(db *relica.DB) ([]CustomerDetail, error) {
    // CTE: Top customers by sales
    topCustomers := db.Builder().
        Select("user_id", "SUM(total) as total_spent").
        From("orders").
        GroupBy("user_id").
        Having("SUM(total) > ?", 5000)

    type CustomerDetail struct {
        Name       string  `db:"name"`
        Email      string  `db:"email"`
        TotalSpent float64 `db:"total_spent"`
    }
    var details []CustomerDetail

    err := db.Builder().
        Select("u.name", "u.email", "tc.total_spent").
        With("top_customers", topCustomers).
        From("users u").
        InnerJoin("top_customers tc", "u.id = tc.user_id").
        OrderBy("tc.total_spent DESC").
        All(&details)

    return details, err
}
```

**Generated SQL**:
```sql
WITH "top_customers" AS (
    SELECT "user_id", SUM(total) as total_spent
    FROM "orders"
    GROUP BY "user_id"
    HAVING SUM(total) > $1
)
SELECT "u"."name", "u"."email", "tc"."total_spent"
FROM "users" AS "u"
INNER JOIN "top_customers" AS "tc" ON u.id = tc.user_id
ORDER BY "tc"."total_spent" DESC
```

### CTE Referenced in WHERE

```go
// Use CTE in subquery
func GetCustomersAboveAverage(db *relica.DB) ([]Customer, error) {
    // CTE: Customer spending
    customerSpending := db.Builder().
        Select("user_id", "SUM(total) as total_spent").
        From("orders").
        GroupBy("user_id")

    var customers []Customer
    err := db.Builder().
        Select("*").
        With("customer_spending", customerSpending).
        From("customers").
        Where("id IN (SELECT user_id FROM customer_spending WHERE total_spent > 1000)").
        All(&customers)

    return customers, err
}
```

## Multiple CTEs

You can define multiple CTEs and reference them in the main query or in other CTEs.

### Chaining CTEs

```go
// Multiple CTEs for complex analysis
func GetEngagedPremiumUsers(db *relica.DB) ([]UserEngagement, error) {
    // CTE 1: Active users
    activeUsers := db.Builder().
        Select("id", "name").
        From("users").
        Where("status = ?", "active")

    // CTE 2: Recent orders
    recentOrders := db.Builder().
        Select("user_id", "COUNT(*) as order_count").
        From("orders").
        Where("created_at > ?", "2024-01-01").
        GroupBy("user_id")

    // Main query: Join both CTEs
    type UserEngagement struct {
        Name       string `db:"name"`
        OrderCount int    `db:"order_count"`
    }
    var users []UserEngagement

    err := db.Builder().
        Select("u.name", "o.order_count").
        With("active_users", activeUsers).
        With("recent_orders", recentOrders).
        From("active_users u").
        InnerJoin("recent_orders o", "u.id = o.user_id").
        All(&users)

    return users, err
}
```

**Generated SQL**:
```sql
WITH "active_users" AS (
    SELECT "id", "name" FROM "users" WHERE status = $1
),
"recent_orders" AS (
    SELECT "user_id", COUNT(*) as order_count
    FROM "orders"
    WHERE created_at > $2
    GROUP BY "user_id"
)
SELECT "u"."name", "o"."order_count"
FROM "active_users" AS "u"
INNER JOIN "recent_orders" AS "o" ON u.id = o.user_id
```

**💡 Tip**: CTEs are separated by commas, not semicolons.

### Dependent CTEs

```go
// Second CTE references first CTE
func GetHighValueActiveCustomers(db *relica.DB) ([]Customer, error) {
    // CTE 1: Calculate user spending
    userSpending := db.Builder().
        Select("user_id", "SUM(total) as total_spent").
        From("orders").
        GroupBy("user_id")

    // CTE 2: Filter high spenders (references CTE 1)
    highSpenders := db.Builder().
        Select("user_id", "total_spent").
        From("user_spending").  // References first CTE!
        Where("total_spent > ?", 5000)

    // Main query: Get user details
    var customers []Customer
    err := db.Builder().
        Select("u.*", "hs.total_spent").
        With("user_spending", userSpending).
        With("high_spenders", highSpenders).
        From("users u").
        InnerJoin("high_spenders hs", "u.id = hs.user_id").
        All(&customers)

    return customers, err
}
```

**Generated SQL**:
```sql
WITH "user_spending" AS (
    SELECT "user_id", SUM(total) as total_spent
    FROM "orders"
    GROUP BY "user_id"
),
"high_spenders" AS (
    SELECT "user_id", "total_spent"
    FROM "user_spending"  -- References first CTE
    WHERE total_spent > $1
)
SELECT "u".*, "hs"."total_spent"
FROM "users" AS "u"
INNER JOIN "high_spenders" AS "hs" ON u.id = hs.user_id
```

## Recursive CTEs

Recursive CTEs traverse hierarchical data structures like organizational charts, category trees, or bill of materials.

**Structure**:
1. **Anchor query**: Base case (starting point)
2. **UNION ALL**: Combines anchor and recursive parts
3. **Recursive query**: References CTE itself

### Organization Hierarchy

```go
// Traverse employee hierarchy from CEO down
func GetOrgChart(db *relica.DB) ([]Employee, error) {
    // Anchor: Top-level employees (no manager)
    anchor := db.Builder().
        Select("id", "name", "manager_id", "1 as level").
        From("employees").
        Where("manager_id IS NULL")

    // Recursive: Employees with managers
    recursive := db.Builder().
        Select("e.id", "e.name", "e.manager_id", "h.level + 1").
        From("employees e").
        InnerJoin("hierarchy h", "e.manager_id = h.id")

    // Combine with UNION ALL
    cte := anchor.UnionAll(recursive)

    // Main query
    type Employee struct {
        ID        int    `db:"id"`
        Name      string `db:"name"`
        ManagerID *int   `db:"manager_id"`
        Level     int    `db:"level"`
    }
    var employees []Employee

    err := db.Builder().
        Select("*").
        WithRecursive("hierarchy", cte).
        From("hierarchy").
        OrderBy("level", "name").
        All(&employees)

    return employees, err
}
```

**Generated SQL**:
```sql
WITH RECURSIVE "hierarchy" AS (
    -- Anchor: Start with top-level employees
    SELECT "id", "name", "manager_id", 1 as level
    FROM "employees"
    WHERE manager_id IS NULL

    UNION ALL

    -- Recursive: Join with hierarchy to get subordinates
    SELECT "e"."id", "e"."name", "e"."manager_id", "h"."level" + 1
    FROM "employees" AS "e"
    INNER JOIN "hierarchy" AS "h" ON e.manager_id = h.id
)
SELECT * FROM "hierarchy"
ORDER BY "level", "name"
```

**How it works**:
1. Start with anchor (CEO with level=1)
2. Find direct reports (level=2)
3. Find their reports (level=3)
4. Continue until no more matches
5. UNION ALL combines all levels

### Category Tree

```go
// Traverse category hierarchy
func GetCategoryTree(db *relica.DB, rootID int) ([]Category, error) {
    // Anchor: Root category
    anchor := db.Builder().
        Select("id", "name", "parent_id", "1 as depth", "name as path").
        From("categories").
        Where("id = ?", rootID)

    // Recursive: Child categories
    recursive := db.Builder().
        Select("c.id", "c.name", "c.parent_id", "t.depth + 1", "t.path || '/' || c.name").
        From("categories c").
        InnerJoin("category_tree t", "c.parent_id = t.id")

    cte := anchor.UnionAll(recursive)

    type Category struct {
        ID       int    `db:"id"`
        Name     string `db:"name"`
        ParentID *int   `db:"parent_id"`
        Depth    int    `db:"depth"`
        Path     string `db:"path"`
    }
    var categories []Category

    err := db.Builder().
        Select("*").
        WithRecursive("category_tree", cte).
        From("category_tree").
        OrderBy("path").
        All(&categories)

    return categories, err
}
```

**Result**:
```
Depth  Path
1      Electronics
2      Electronics/Computers
3      Electronics/Computers/Laptops
3      Electronics/Computers/Desktops
2      Electronics/Phones
```

### Bill of Materials

```go
// Calculate total cost of product including all components
func GetBillOfMaterials(db *relica.DB, productID int) ([]BOM, error) {
    // Anchor: Top-level product
    anchor := db.Builder().
        Select("id", "name", "cost", "1 as quantity", "1 as level").
        From("parts").
        Where("id = ?", productID)

    // Recursive: Components
    recursive := db.Builder().
        Select("p.id", "p.name", "p.cost", "bom.quantity * c.quantity", "bom.level + 1").
        From("parts p").
        InnerJoin("components c", "p.id = c.part_id").
        InnerJoin("bom", "c.assembly_id = bom.id")

    cte := anchor.UnionAll(recursive)

    type BOM struct {
        ID       int     `db:"id"`
        Name     string  `db:"name"`
        Cost     float64 `db:"cost"`
        Quantity int     `db:"quantity"`
        Level    int     `db:"level"`
    }
    var bom []BOM

    err := db.Builder().
        Select("*").
        WithRecursive("bom", cte).
        From("bom").
        OrderBy("level", "name").
        All(&bom)

    return bom, err
}
```

### Limiting Recursion Depth

```go
// Prevent infinite recursion with depth limit
func GetCategoryTreeLimited(db *relica.DB, rootID int, maxDepth int) ([]Category, error) {
    anchor := db.Builder().
        Select("id", "name", "parent_id", "1 as depth").
        From("categories").
        Where("id = ?", rootID)

    // Add depth limit in recursive part
    recursive := db.Builder().
        Select("c.id", "c.name", "c.parent_id", "t.depth + 1").
        From("categories c").
        InnerJoin("category_tree t", "c.parent_id = t.id").
        Where("t.depth < ?", maxDepth)  // Stop at max depth

    cte := anchor.UnionAll(recursive)

    var categories []Category
    err := db.Builder().
        Select("*").
        WithRecursive("category_tree", cte).
        From("category_tree").
        All(&categories)

    return categories, err
}
```

### Cycle Detection

```go
// Detect cycles in hierarchical data
func FindCycles(db *relica.DB) ([]Cycle, error) {
    // Anchor: Start nodes
    anchor := db.Builder().
        Select("id", "parent_id", "ARRAY[id] as path", "false as is_cycle").
        From("nodes").
        Where("parent_id IS NULL")

    // Recursive: Traverse with cycle detection
    recursive := db.Builder().
        Select("n.id", "n.parent_id", "t.path || n.id", "n.id = ANY(t.path)").
        From("nodes n").
        InnerJoin("tree t", "n.parent_id = t.id").
        Where("NOT t.is_cycle")  // Stop traversing cycles

    cte := anchor.UnionAll(recursive)

    type Cycle struct {
        ID      int   `db:"id"`
        Path    []int `db:"path"`
        IsCycle bool  `db:"is_cycle"`
    }
    var cycles []Cycle

    err := db.Builder().
        Select("*").
        WithRecursive("tree", cte).
        From("tree").
        Where("is_cycle = ?", true).
        All(&cycles)

    return cycles, err
}
```

## CTE vs Subqueries

Both CTEs and subqueries achieve similar goals, but CTEs offer better readability and reusability.

### When to Use CTE

**✅ Use CTE when**:
- Query is complex (3+ levels of logic)
- Need to reference result multiple times
- Recursive traversal required
- Code readability is priority
- Debugging complex queries

**Example**: Complex multi-step query
```go
// ✅ CLEAR: Step-by-step logic with CTEs
activeUsers := db.Builder().Select("id").From("users").Where("active = ?", true)
highSpenders := db.Builder().Select("user_id").From("orders").GroupBy("user_id").Having("SUM(total) > ?", 1000)

db.Builder().
    With("active_users", activeUsers).
    With("high_spenders", highSpenders).
    Select("*").
    From("active_users").
    Where("id IN (SELECT user_id FROM high_spenders)")
```

### When to Use Subquery

**✅ Use Subquery when**:
- Simple one-off query
- Only used once
- Performance critical (some databases)
- Inline logic is clearer

**Example**: Simple filter
```go
// ✅ SIMPLE: Inline subquery
subquery := db.Builder().Select("user_id").From("orders")
db.Builder().Select("*").From("users").Where(relica.In("id", subquery))
```

### Performance Comparison

**Benchmark** (PostgreSQL, 1M rows):

| Operation | CTE Time | Subquery Time | Notes |
|-----------|----------|---------------|-------|
| Simple filter | ~120ms | ~115ms | Negligible difference |
| Multiple references | ~180ms | ~350ms | CTE faster (computed once) |
| Recursive | ~250ms | N/A | Only CTEs support recursion |

**Key Points**:
- Modern databases optimize both similarly
- CTEs are computed once if referenced multiple times
- Subqueries may be re-executed if referenced multiple times
- Readability usually outweighs minor performance differences

### CTE Optimization

**PostgreSQL 12+**: CTEs are now optimized like subqueries (inlined when beneficial)
**MySQL 8.0.14+**: CTE optimization improved
**SQLite 3.35+**: WITH clause optimization

## When to Use CTEs

### Decision Tree

```
Need to break down complex query?
├─ Recursive traversal? → Use WITH RECURSIVE
├─ Referenced multiple times? → Use CTE
├─ Complex multi-step logic? → Use CTE (readability)
├─ Simple one-off? → Use subquery
└─ Performance critical? → Test both (usually similar)
```

### Use Cases

**✅ Excellent for CTEs**:
- Hierarchical data (org charts, trees, graphs)
- Multi-step aggregations
- Complex business logic
- Data transformations
- Reporting queries

**❌ Not ideal for CTEs**:
- Simple filters (use WHERE)
- Single-use calculations (use subquery)
- Very simple queries (adds complexity)

## Performance Considerations

### 1. CTE Materialization

**Old behavior** (PostgreSQL < 12):
```sql
-- CTE was ALWAYS materialized (computed once, stored in memory)
WITH expensive_cte AS (SELECT ...)
SELECT * FROM expensive_cte WHERE id = 1;
-- Even with WHERE, entire CTE was computed
```

**New behavior** (PostgreSQL 12+):
```sql
-- CTE can be inlined and optimized
WITH expensive_cte AS (SELECT ...)
SELECT * FROM expensive_cte WHERE id = 1;
-- Only computes rows matching WHERE id = 1
```

### 2. Recursive CTE Performance

**Tips for fast recursive CTEs**:

1. **Add depth limit** (prevent runaway recursion)
   ```go
   Where("depth < ?", 10)
   ```

2. **Index join columns**
   ```sql
   CREATE INDEX idx_employees_manager_id ON employees(manager_id);
   ```

3. **Use WHERE in recursive part** (early termination)
   ```go
   Where("level < ? AND active = ?", 5, true)
   ```

4. **Monitor recursion depth**
   ```sql
   -- PostgreSQL: max_stack_depth
   -- MySQL: cte_max_recursion_depth (default 1000)
   SET SESSION cte_max_recursion_depth = 10000;
   ```

### 3. Memory Usage

**CTEs vs Subqueries**:
- Modern databases: Similar memory usage
- Multiple CTE references: More efficient (computed once)
- Large result sets: Consider LIMIT in CTE definition

### 4. Indexing

**Index columns used in**:
- CTE WHERE clauses
- JOIN conditions in recursive part
- Final query WHERE/ORDER BY

```sql
-- Example indexes for org hierarchy
CREATE INDEX idx_employees_manager_id ON employees(manager_id);
CREATE INDEX idx_employees_id ON employees(id);
```

## Database Compatibility

| Feature | PostgreSQL | MySQL | SQLite | Notes |
|---------|-----------|-------|---------|-------|
| Basic CTE (WITH) | ✓ 8.4+ | ✓ 8.0+ | ✓ 3.8.3+ | |
| Multiple CTEs | ✓ 8.4+ | ✓ 8.0+ | ✓ 3.8.3+ | |
| Recursive CTE | ✓ 8.4+ | ✓ 8.0+ | ✓ 3.8.3+ | |
| CTE in subquery | ✓ 8.4+ | ✓ 8.0+ | ✓ 3.8.3+ | |
| CTE optimization | ✓ 12+ | ✓ 8.0.14+ | ✓ 3.35+ | |

**MySQL Notes**:
- MySQL 5.7: No CTE support
- MySQL 8.0+: Full CTE support including recursive
- MySQL 8.0.1: Initial CTE implementation
- MySQL 8.0.14+: Improved CTE optimization

**PostgreSQL Notes**:
- PostgreSQL < 12: CTEs always materialized
- PostgreSQL 12+: CTEs inlined when beneficial
- Use `MATERIALIZED` hint to force materialization: `WITH cte AS MATERIALIZED (...)`

## Best Practices

### ✅ DO

1. **Use descriptive CTE names**
   ```go
   With("active_high_value_customers", cte) // Clear
   ```

2. **Break complex queries into multiple CTEs**
   ```go
   With("step1", cte1).With("step2", cte2).With("step3", cte3)
   ```

3. **Add depth limits to recursive CTEs**
   ```go
   Where("depth <= ?", 10)
   ```

4. **Index join columns in recursive CTEs**
   ```sql
   CREATE INDEX idx_parent_id ON categories(parent_id);
   ```

5. **Use UNION ALL in recursive CTEs** (not UNION)
   ```go
   anchor.UnionAll(recursive) // Correct for recursive
   ```

### ❌ DON'T

1. **Don't use CTE for simple queries**
   ```go
   // ❌ Overkill
   cte := db.Builder().Select("id").From("users")
   db.Builder().With("users_cte", cte).Select("*").From("users_cte")

   // ✅ Simple
   db.Builder().Select("id").From("users")
   ```

2. **Don't forget UNION ALL in recursive CTEs**
   ```go
   // ❌ Will panic: "recursive CTE requires UNION or UNION ALL"
   anchor.Union(recursive) // WRONG

   // ✅ Correct
   anchor.UnionAll(recursive)
   ```

3. **Don't create infinite recursion**
   ```go
   // ❌ No termination condition
   recursive := db.Builder().Select("*").From("nodes n").InnerJoin("tree t", "n.parent_id = t.id")

   // ✅ Add depth limit
   recursive := db.Builder().Select("*").From("nodes n").InnerJoin("tree t", "n.parent_id = t.id").Where("t.depth < ?", 100)
   ```

4. **Don't use empty CTE names**
   ```go
   // ❌ Panics
   With("", cte)

   // ✅ Descriptive name
   With("order_summary", cte)
   ```

5. **Don't pass nil CTE query**
   ```go
   // ❌ Panics
   With("my_cte", nil)
   ```

## Common Patterns

### Pattern 1: Data Transformation Pipeline

```go
// Multi-step data transformation
func TransformCustomerData(db *relica.DB) ([]CustomerMetrics, error) {
    // Step 1: Raw order data
    rawOrders := db.Builder().
        Select("user_id", "total", "created_at").
        From("orders").
        Where("status = ?", "completed")

    // Step 2: Aggregate by month
    monthlyStats := db.Builder().
        Select("user_id", "DATE_TRUNC('month', created_at) as month", "SUM(total) as monthly_total").
        From("raw_orders").
        GroupBy("user_id", "DATE_TRUNC('month', created_at)")

    // Step 3: Calculate growth
    growth := db.Builder().
        Select("user_id", "month", "monthly_total", "LAG(monthly_total) OVER (PARTITION BY user_id ORDER BY month) as prev_month").
        From("monthly_stats")

    type CustomerMetrics struct {
        UserID       int     `db:"user_id"`
        Month        string  `db:"month"`
        MonthlyTotal float64 `db:"monthly_total"`
        Growth       float64 `db:"growth"`
    }
    var metrics []CustomerMetrics

    err := db.Builder().
        With("raw_orders", rawOrders).
        With("monthly_stats", monthlyStats).
        With("growth", growth).
        Select("user_id", "month", "monthly_total", "(monthly_total - prev_month) / prev_month * 100 as growth").
        From("growth").
        Where("prev_month IS NOT NULL").
        All(&metrics)

    return metrics, err
}
```

### Pattern 2: Hierarchical Rollup

```go
// Calculate total sales for each department and all subdepartments
func GetDepartmentSalesRollup(db *relica.DB) ([]DeptSales, error) {
    // Recursive: Traverse department hierarchy
    anchor := db.Builder().
        Select("id", "parent_id", "name", "0 as level").
        From("departments").
        Where("parent_id IS NULL")

    recursive := db.Builder().
        Select("d.id", "d.parent_id", "d.name", "dh.level + 1").
        From("departments d").
        InnerJoin("dept_hierarchy dh", "d.parent_id = dh.id")

    hierarchy := anchor.UnionAll(recursive)

    // Calculate sales per department
    deptSales := db.Builder().
        Select("department_id", "SUM(amount) as sales").
        From("sales").
        GroupBy("department_id")

    type DeptSales struct {
        Name       string  `db:"name"`
        Level      int     `db:"level"`
        TotalSales float64 `db:"total_sales"`
    }
    var sales []DeptSales

    err := db.Builder().
        With("dept_hierarchy", hierarchy).
        With("dept_sales", deptSales).
        Select("dh.name", "dh.level", "COALESCE(SUM(ds.sales), 0) as total_sales").
        From("dept_hierarchy dh").
        LeftJoin("dept_sales ds", "dh.id = ds.department_id").
        GroupBy("dh.name", "dh.level").
        OrderBy("dh.level", "dh.name").
        All(&sales)

    return sales, err
}
```

### Pattern 3: Finding Gaps in Sequences

```go
// Find missing order numbers
func FindMissingOrderNumbers(db *relica.DB) ([]int, error) {
    // Generate expected sequence
    expectedSeq := db.Builder().
        Select("generate_series(1, (SELECT MAX(order_number) FROM orders)) as expected")

    // Find gaps
    var missing []int
    err := db.Builder().
        With("expected_sequence", expectedSeq).
        Select("expected").
        From("expected_sequence").
        Where("expected NOT IN (SELECT order_number FROM orders)").
        All(&missing)

    return missing, err
}
```

### Pattern 4: Cumulative Aggregation

```go
// Calculate running totals with CTE
func GetRunningTotals(db *relica.DB) ([]DailySales, error) {
    // Daily sales
    dailySales := db.Builder().
        Select("DATE(created_at) as sale_date", "SUM(total) as daily_total").
        From("orders").
        GroupBy("DATE(created_at)")

    type DailySales struct {
        Date         string  `db:"sale_date"`
        DailyTotal   float64 `db:"daily_total"`
        RunningTotal float64 `db:"running_total"`
    }
    var sales []DailySales

    err := db.Builder().
        With("daily_sales", dailySales).
        Select("sale_date", "daily_total", "SUM(daily_total) OVER (ORDER BY sale_date) as running_total").
        From("daily_sales").
        OrderBy("sale_date").
        All(&sales)

    return sales, err
}
```

## Troubleshooting

### Issue: "recursive CTE requires UNION or UNION ALL"

**Problem**: Using wrong set operation in recursive CTE
```go
// ❌ ERROR: UNION removes duplicates (breaks recursion)
cte := anchor.Union(recursive)
db.Builder().WithRecursive("tree", cte)
```

**Solution**: Always use UNION ALL for recursive CTEs
```go
// ✅ GOOD
cte := anchor.UnionAll(recursive)
db.Builder().WithRecursive("tree", cte)
```

### Issue: Infinite Recursion

**Problem**: No termination condition
```go
// ❌ BAD: No depth limit
recursive := db.Builder().
    Select("c.id", "t.depth + 1").
    From("categories c").
    InnerJoin("tree t", "c.parent_id = t.id")
```

**Solution**: Add depth limit or cycle detection
```go
// ✅ GOOD: Depth limit
recursive := db.Builder().
    Select("c.id", "t.depth + 1").
    From("categories c").
    InnerJoin("tree t", "c.parent_id = t.id").
    Where("t.depth < ?", 10)

// ✅ BETTER: Cycle detection (PostgreSQL)
recursive := db.Builder().
    Select("c.id", "t.path || c.id").
    From("categories c").
    InnerJoin("tree t", "c.parent_id = t.id").
    Where("NOT c.id = ANY(t.path)")
```

### Issue: CTE Not Optimized

**Problem**: CTE computed even when filtered (PostgreSQL < 12)
```sql
-- Entire CTE materialized, then filtered
WITH large_cte AS (SELECT * FROM huge_table)
SELECT * FROM large_cte WHERE id = 1;
```

**Solution**: Upgrade to PostgreSQL 12+ or use subquery
```go
// Alternative: Use subquery for older databases
subquery := db.Builder().Select("*").From("huge_table")
db.Builder().FromSelect(subquery, "t").Where("id = ?", 1)
```

### Issue: CTE Name Collision

**Problem**: Duplicate CTE names
```go
// ❌ Second "stats" overwrites first
db.Builder().
    With("stats", cte1).
    With("stats", cte2) // Overwrites!
```

**Solution**: Use unique names
```go
// ✅ GOOD
db.Builder().
    With("order_stats", cte1).
    With("user_stats", cte2)
```

## Examples Repository

Complete working examples available in Relica repository:

- `reference/cte/basic.go` - Simple CTE examples
- `reference/cte/multiple.go` - Multiple CTE patterns
- `reference/cte/recursive_hierarchy.go` - Org chart traversal
- `reference/cte/recursive_tree.go` - Category tree examples
- `reference/cte/bill_of_materials.go` - BOM calculations

## Further Reading

- [Subquery Guide](./SUBQUERY_GUIDE.md) - Alternative to CTEs for simple queries
- [Set Operations Guide](./SET_OPERATIONS_GUIDE.md) - Combining query results
- [Window Functions Guide](./WINDOW_FUNCTIONS_GUIDE.md) - Advanced analytics with CTEs

---

**Last Updated**: 2025-01-25
**Minimum Go Version**: 1.25+
