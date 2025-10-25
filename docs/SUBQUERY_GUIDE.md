# Subquery Guide

> Complete guide to using subqueries in Relica

## Table of Contents
- [Introduction](#introduction)
- [Types of Subqueries](#types-of-subqueries)
- [IN Subqueries](#in-subqueries)
- [EXISTS Subqueries](#exists-subqueries)
- [FROM Subqueries](#from-subqueries)
- [Scalar Subqueries](#scalar-subqueries)
- [Correlated vs Non-Correlated](#correlated-vs-non-correlated)
- [When to Use Subqueries](#when-to-use-subqueries)
- [Performance Considerations](#performance-considerations)
- [Database Compatibility](#database-compatibility)
- [Best Practices](#best-practices)
- [Common Patterns](#common-patterns)
- [Troubleshooting](#troubleshooting)

## Introduction

Subqueries are SELECT queries nested within another SQL statement. Relica provides full subquery support for PostgreSQL, MySQL 8.0+, and SQLite 3.25+.

**Key Benefits**:
- Write complex queries step-by-step
- Filter data based on related tables
- Calculate derived values
- Avoid N+1 query problems
- Improve code maintainability

**Version Support**: Relica v0.3.0-beta and later

## Types of Subqueries

### 1. IN Subqueries
Returns rows where column value matches any value in subquery results.

**Use case**: Find users who have placed orders
```go
WHERE id IN (SELECT user_id FROM orders)
```

### 2. EXISTS Subqueries
Checks if subquery returns any rows (boolean test).

**Use case**: Find users with at least one order
```go
WHERE EXISTS (SELECT 1 FROM orders WHERE user_id = users.id)
```

### 3. FROM Subqueries
Uses subquery as a table source (derived table).

**Use case**: Filter aggregated results
```go
SELECT * FROM (SELECT user_id, COUNT(*) FROM orders GROUP BY user_id) AS stats WHERE cnt > 10
```

### 4. Scalar Subqueries
Returns single value for use in SELECT or WHERE clause.

**Use case**: Show order count for each user
```go
SELECT id, (SELECT COUNT(*) FROM orders WHERE user_id = users.id) as order_count FROM users
```

## IN Subqueries

### Basic Usage

```go
package main

import (
    "github.com/coregx/relica"
)

// Find users who have placed orders
func GetActiveUsers(db *relica.DB) ([]User, error) {
    subquery := db.Builder().
        Select("user_id").
        From("orders").
        Where("status = ?", "completed")

    var users []User
    err := db.Builder().
        Select("*").
        From("users").
        Where(relica.In("id", subquery)).
        All(&users)

    return users, err
}
```

**Generated SQL** (PostgreSQL):
```sql
SELECT * FROM "users"
WHERE "id" IN (SELECT "user_id" FROM "orders" WHERE status = $1)
```

### With Additional Filters

```go
// Find users with high-value orders
func GetPremiumUsers(db *relica.DB) ([]User, error) {
    subquery := db.Builder().
        Select("user_id").
        From("orders").
        Where("total > ? AND status = ?", 1000, "completed")

    var premiumUsers []User
    err := db.Builder().
        Select("id", "name", "email").
        From("users").
        Where(relica.In("id", subquery)).
        OrderBy("name").
        All(&premiumUsers)

    return premiumUsers, err
}
```

**Generated SQL**:
```sql
SELECT "id", "name", "email" FROM "users"
WHERE "id" IN (SELECT "user_id" FROM "orders" WHERE total > $1 AND status = $2)
ORDER BY "name"
```

### NOT IN

```go
// Find users who have never ordered
func GetInactiveUsers(db *relica.DB) ([]User, error) {
    subquery := db.Builder().
        Select("user_id").
        From("orders")

    var inactiveUsers []User
    err := db.Builder().
        Select("*").
        From("users").
        Where(relica.NotIn("id", subquery)).
        All(&inactiveUsers)

    return inactiveUsers, err
}
```

**Generated SQL**:
```sql
SELECT * FROM "users"
WHERE "id" NOT IN (SELECT "user_id" FROM "orders")
```

**⚠️ Warning**: NOT IN with NULL values can return unexpected results. Use NOT EXISTS instead when NULLs are possible.

### When to Use IN

**✅ Use IN when**:
- Filtering by known set of values from related table
- Subquery returns distinct values
- Small to medium result sets (< 1000 values)
- Non-NULL columns only

**❌ Use EXISTS instead when**:
- Large result sets (> 1000 values)
- NULL values possible in subquery result
- Just checking existence (don't need actual values)
- Better performance needed for large datasets

## EXISTS Subqueries

### Basic Usage

```go
// Find users with at least one order
func GetUsersWithOrders(db *relica.DB) ([]User, error) {
    orderCheck := db.Builder().
        Select("1").
        From("orders").
        Where("orders.user_id = users.id")

    var activeUsers []User
    err := db.Builder().
        Select("*").
        From("users").
        Where(relica.Exists(orderCheck)).
        All(&activeUsers)

    return activeUsers, err
}
```

**Generated SQL**:
```sql
SELECT * FROM "users"
WHERE EXISTS (SELECT 1 FROM "orders" WHERE orders.user_id = users.id)
```

**💡 Tip**: Use `SELECT 1` instead of `SELECT *` in EXISTS - it's clearer and may be slightly faster.

### NOT EXISTS

```go
// Find users with no orders (NULL-safe)
func GetUsersWithoutOrders(db *relica.DB) ([]User, error) {
    orderCheck := db.Builder().
        Select("1").
        From("orders").
        Where("orders.user_id = users.id")

    var inactiveUsers []User
    err := db.Builder().
        Select("*").
        From("users").
        Where(relica.NotExists(orderCheck)).
        All(&inactiveUsers)

    return inactiveUsers, err
}
```

**Generated SQL**:
```sql
SELECT * FROM "users"
WHERE NOT EXISTS (SELECT 1 FROM "orders" WHERE orders.user_id = users.id)
```

### Correlated EXISTS

```go
// Find users with recent high-value orders
func GetHighValueRecentCustomers(db *relica.DB, since time.Time) ([]User, error) {
    recentOrders := db.Builder().
        Select("1").
        From("orders o").
        Where("o.user_id = u.id AND o.total > ? AND o.created_at > ?",
            1000, since)

    var users []User
    err := db.Builder().
        Select("*").
        From("users u").
        Where(relica.Exists(recentOrders)).
        All(&users)

    return users, err
}
```

**Generated SQL**:
```sql
SELECT * FROM "users" AS "u"
WHERE EXISTS (
    SELECT 1 FROM "orders" AS "o"
    WHERE o.user_id = u.id AND o.total > $1 AND o.created_at > $2
)
```

### Multiple EXISTS Conditions

```go
// Find users who have both orders AND reviews
func GetEngagedUsers(db *relica.DB) ([]User, error) {
    hasOrders := db.Builder().
        Select("1").
        From("orders").
        Where("orders.user_id = users.id")

    hasReviews := db.Builder().
        Select("1").
        From("reviews").
        Where("reviews.user_id = users.id")

    var users []User
    err := db.Builder().
        Select("*").
        From("users").
        Where(relica.And(
            relica.Exists(hasOrders),
            relica.Exists(hasReviews),
        )).
        All(&users)

    return users, err
}
```

### When to Use EXISTS

**✅ Use EXISTS when**:
- Checking for existence (don't need actual values)
- NULL-safe filtering required
- Often faster than IN for large datasets
- Correlated queries (referencing parent table)
- Early termination optimization (stops at first match)

**❌ Use JOIN instead when**:
- Need columns from both tables
- One-to-one or one-to-many relationships with data needed

## FROM Subqueries

FROM subqueries (derived tables) allow you to use query results as a table source. An alias is **always required**.

### Basic Usage

```go
// Calculate order statistics per user, then filter
func GetTopCustomers(db *relica.DB) ([]CustomerStats, error) {
    stats := db.Builder().
        Select("user_id", "COUNT(*) as order_count", "SUM(total) as total_spent").
        From("orders").
        GroupBy("user_id")

    type CustomerStats struct {
        UserID      int     `db:"user_id"`
        OrderCount  int     `db:"order_count"`
        TotalSpent  float64 `db:"total_spent"`
    }
    var topCustomers []CustomerStats

    err := db.Builder().
        FromSelect(stats, "order_stats").
        Select("user_id", "order_count", "total_spent").
        Where("order_count > ? AND total_spent > ?", 10, 5000).
        OrderBy("total_spent DESC").
        All(&topCustomers)

    return topCustomers, err
}
```

**Generated SQL**:
```sql
SELECT "user_id", "order_count", "total_spent"
FROM (
    SELECT "user_id", COUNT(*) as order_count, SUM(total) as total_spent
    FROM "orders"
    GROUP BY "user_id"
) AS "order_stats"
WHERE order_count > $1 AND total_spent > $2
ORDER BY "total_spent" DESC
```

### With JOIN

```go
// Join aggregated data with users table
func GetUsersWithAvgRating(db *relica.DB, minRating float64) ([]UserRating, error) {
    stats := db.Builder().
        Select("user_id", "AVG(rating) as avg_rating").
        From("reviews").
        GroupBy("user_id")

    type UserRating struct {
        Name      string  `db:"name"`
        AvgRating float64 `db:"avg_rating"`
    }
    var users []UserRating

    err := db.Builder().
        Select("u.name", "r.avg_rating").
        From("users u").
        InnerJoin("("+stats.Build().SQL()+") AS r", "u.id = r.user_id").
        Where("r.avg_rating >= ?", minRating).
        All(&users)

    return users, err
}
```

**Alternative using FromSelect** (cleaner):
```go
// Better approach: use FromSelect for main subquery, then JOIN
func GetUsersWithAvgRating(db *relica.DB, minRating float64) ([]UserRating, error) {
    stats := db.Builder().
        Select("user_id", "AVG(rating) as avg_rating").
        From("reviews").
        GroupBy("user_id")

    type UserRating struct {
        Name      string  `db:"name"`
        AvgRating float64 `db:"avg_rating"`
    }
    var users []UserRating

    err := db.Builder().
        Select("u.name", "s.avg_rating").
        FromSelect(stats, "s").
        InnerJoin("users u", "u.id = s.user_id").
        Where("s.avg_rating >= ?", minRating).
        All(&users)

    return users, err
}
```

### Nested Subqueries

```go
// Multi-level aggregation
func GetTopSellingCategories(db *relica.DB) ([]CategorySales, error) {
    // Inner: product sales
    productSales := db.Builder().
        Select("product_id", "SUM(quantity) as total_sold").
        From("order_items").
        GroupBy("product_id")

    // Middle: join with products to get categories
    categorySales := db.Builder().
        Select("p.category_id", "SUM(ps.total_sold) as category_total").
        FromSelect(productSales, "ps").
        InnerJoin("products p", "p.id = ps.product_id").
        GroupBy("p.category_id")

    // Outer: filter top categories
    type CategorySales struct {
        CategoryID int `db:"category_id"`
        Total      int `db:"category_total"`
    }
    var result []CategorySales

    err := db.Builder().
        Select("category_id", "category_total").
        FromSelect(categorySales, "cs").
        Where("category_total > ?", 1000).
        OrderBy("category_total DESC").
        Limit(10).
        All(&result)

    return result, err
}
```

### When to Use FROM Subqueries

**✅ Use FROM subqueries when**:
- Need to filter or order aggregated results (HAVING is limited)
- Complex calculations before main query
- Simplify complex queries into readable steps
- Need to aggregate aggregated data
- Want to apply LIMIT/OFFSET before joining

**❌ Use CTE instead when**:
- Need to reference subquery multiple times
- Recursive queries required
- Better readability with multiple steps

**Requirement**: Alias is **always required** (SQL standard).

## Scalar Subqueries

Scalar subqueries return a single value and can be used in SELECT or WHERE clauses.

### In SELECT Clause

```go
// Show each user with their order count
func GetUsersWithOrderCount(db *relica.DB) ([]UserWithStats, error) {
    type UserWithStats struct {
        ID         int    `db:"id"`
        Name       string `db:"name"`
        OrderCount int    `db:"order_count"`
    }
    var users []UserWithStats

    err := db.Builder().
        Select("id", "name").
        SelectExpr("(SELECT COUNT(*) FROM orders WHERE user_id = users.id)", "order_count").
        From("users").
        All(&users)

    return users, err
}
```

**Generated SQL**:
```sql
SELECT "id", "name", (SELECT COUNT(*) FROM orders WHERE user_id = users.id) as order_count
FROM "users"
```

### Multiple Scalar Subqueries

```go
// Multiple calculated columns
func GetUserStats(db *relica.DB) ([]UserStats, error) {
    type UserStats struct {
        ID         int        `db:"id"`
        Name       string     `db:"name"`
        OrderCount int        `db:"order_count"`
        TotalSpent float64    `db:"total_spent"`
        LastOrder  *time.Time `db:"last_order"`
    }
    var users []UserStats

    err := db.Builder().
        Select("id", "name").
        SelectExpr("(SELECT COUNT(*) FROM orders WHERE user_id = users.id)", "order_count").
        SelectExpr("(SELECT COALESCE(SUM(total), 0) FROM orders WHERE user_id = users.id)", "total_spent").
        SelectExpr("(SELECT MAX(created_at) FROM orders WHERE user_id = users.id)", "last_order").
        From("users").
        All(&users)

    return users, err
}
```

### In WHERE Clause

```go
// Find users who spent more than average
func GetAboveAverageSpenders(db *relica.DB) ([]User, error) {
    var users []User

    err := db.Builder().
        Select("*").
        SelectExpr("(SELECT SUM(total) FROM orders WHERE user_id = users.id)", "total_spent").
        From("users").
        Where("(SELECT SUM(total) FROM orders WHERE user_id = users.id) > (SELECT AVG(total) FROM orders)").
        All(&users)

    return users, err
}
```

**💡 Tip**: For complex conditions, consider using FROM subquery or CTE for better readability.

### When to Use Scalar Subqueries

**✅ Use scalar subqueries when**:
- Need single calculated value per row
- Avoid LEFT JOIN with GROUP BY complexity
- Simple correlated calculation
- One or two calculations

**❌ Use JOIN + GROUP BY instead when**:
- Many calculations needed (more efficient)
- Need multiple columns from subquery
- Performance is critical (scalar subqueries can be slow)

**⚠️ Warning**: Scalar subqueries must return exactly one row and one column, or NULL.

## Correlated vs Non-Correlated

Understanding the difference helps with performance optimization.

### Non-Correlated Subqueries

Subquery can run independently (no reference to outer query).

```go
// Non-correlated: subquery doesn't reference parent table
subquery := db.Builder().
    Select("user_id").
    From("orders").
    Where("total > ?", 1000)

db.Builder().
    Select("*").
    From("users").
    Where(relica.In("id", subquery))
```

**Performance**: Database executes subquery **once**, caches results, then filters outer query.

**SQL**:
```sql
SELECT * FROM "users"
WHERE "id" IN (SELECT "user_id" FROM "orders" WHERE total > $1)
```

### Correlated Subqueries

Subquery references columns from outer query.

```go
// Correlated: subquery references users.id from parent
orderCheck := db.Builder().
    Select("1").
    From("orders").
    Where("orders.user_id = users.id")  // References parent!

db.Builder().
    Select("*").
    From("users").
    Where(relica.Exists(orderCheck))
```

**Performance**: Database may execute subquery **for each outer row** (can be optimized by database).

**SQL**:
```sql
SELECT * FROM "users"
WHERE EXISTS (SELECT 1 FROM "orders" WHERE orders.user_id = users.id)
```

**💡 Optimization**: Modern databases (PostgreSQL 12+, MySQL 8.0.16+) can optimize correlated subqueries into semi-joins.

## When to Use Subqueries

### Decision Tree

```
Need data from related table?
├─ Yes, need columns from both → Use JOIN
└─ No, just checking/filtering
   ├─ Checking existence → Use EXISTS
   ├─ Filtering by values
   │  ├─ Small list (< 100) → Use IN
   │  ├─ Large list (> 1000) → Use EXISTS or JOIN
   │  └─ NULL possible → Use EXISTS
   └─ Aggregating first → Use FROM subquery or CTE
```

### ✅ Use Subqueries When

1. **Complex filtering based on aggregates**
   ```go
   // Users with more than average orders
   avgOrderCount := db.Builder().Select("AVG(order_count)").From("user_stats")
   ```

2. **Checking existence**
   ```go
   // EXISTS is clearer than JOIN when you don't need joined data
   exists := db.Builder().Select("1").From("orders").Where("user_id = users.id")
   ```

3. **Filtering aggregated results**
   ```go
   // FROM subquery allows WHERE on aggregated columns
   stats := db.Builder().Select("user_id, COUNT(*) as cnt").From("orders").GroupBy("user_id")
   db.Builder().FromSelect(stats, "s").Where("cnt > 10")
   ```

4. **Simplifying complex queries**
   ```go
   // Break complex logic into readable steps
   ```

5. **Avoiding N+1 queries**
   ```go
   // One query with subquery vs. N queries in loop
   ```

### ❌ Use JOIN Instead When

1. **Need columns from both tables**
   ```go
   // JOIN is more efficient when you need user.name AND order.total
   ```

2. **Simple one-to-many relationships**
   ```go
   // INNER JOIN is clearer for simple relationships
   ```

3. **Better performance for large datasets**
   ```go
   // Database can optimize JOINs better in many cases
   ```

## Performance Considerations

### 1. IN vs EXISTS

**Benchmark** (1M users, 5M orders):

| Operation | Time | Notes |
|-----------|------|-------|
| IN with 1000 values | ~150ms | Good for small lists |
| IN with 100K values | ~800ms | Degrades with size |
| EXISTS | ~120ms | Consistent performance |

**Use EXISTS when**:
- Large result sets
- NULL values possible
- Just checking existence

**Use IN when**:
- Small, distinct value lists (< 1000)
- Need specific values

### 2. Subquery Execution

**Non-correlated**: Database executes once, caches result
```sql
-- Executes subquery once
WHERE id IN (SELECT user_id FROM orders WHERE total > 1000)
```

**Correlated**: May execute for each row (but databases optimize)
```sql
-- May execute for each user (but optimized by database)
WHERE EXISTS (SELECT 1 FROM orders WHERE user_id = users.id)
```

**💡 Tip**: Modern databases convert correlated EXISTS to semi-joins automatically.

### 3. Indexing

**Critical**: Ensure subquery WHERE clauses use indexed columns.

```sql
-- ✅ Good: user_id is indexed
WHERE user_id = users.id

-- ❌ Bad: function call prevents index use
WHERE UPPER(email) = UPPER(users.email)

-- ✅ Good: use functional index
CREATE INDEX idx_email_upper ON users(UPPER(email));
```

### 4. Limit Subquery Results

Use LIMIT when checking existence:

```go
// Check if ANY orders exist (stop at first match)
orderCheck := db.Builder().
    Select("1").
    From("orders").
    Where("user_id = users.id").
    Limit(1)  // Stop after first row

db.Builder().
    Select("*").
    From("users").
    Where(relica.Exists(orderCheck))
```

**Note**: Most databases optimize EXISTS automatically, but explicit LIMIT doesn't hurt.

### 5. Avoid Scalar Subqueries in Loops

```go
// ❌ BAD: Scalar subquery executes for each row
db.Builder().
    Select("id").
    SelectExpr("(SELECT COUNT(*) FROM orders WHERE user_id = users.id)").
    From("users") // Executes subquery 1M times for 1M users

// ✅ GOOD: Use JOIN + GROUP BY
db.Builder().
    Select("u.id", "COUNT(o.id) as order_count").
    From("users u").
    LeftJoin("orders o", "u.id = o.user_id").
    GroupBy("u.id")
```

## Database Compatibility

| Feature | PostgreSQL | MySQL | SQLite | Notes |
|---------|-----------|-------|---------|-------|
| IN subqueries | ✓ All | ✓ 8.0+ | ✓ 3.25+ | |
| NOT IN subqueries | ✓ All | ✓ 8.0+ | ✓ 3.25+ | Watch for NULLs |
| EXISTS/NOT EXISTS | ✓ All | ✓ 8.0+ | ✓ 3.25+ | |
| FROM subqueries | ✓ All | ✓ 8.0+ | ✓ 3.25+ | Alias required |
| Scalar subqueries | ✓ All | ✓ 8.0+ | ✓ 3.25+ | Must return 1 value |
| Correlated subqueries | ✓ All | ✓ 8.0+ | ✓ 3.25+ | |

**MySQL Version Notes**:
- MySQL 5.7: Limited subquery support (slow, avoid if possible)
- MySQL 8.0+: Full subquery support with optimization (recommended)
- MySQL 8.0.16+: Correlated subquery optimization improved

## Best Practices

### ✅ DO

1. **Use EXISTS for existence checks**
   ```go
   relica.Exists(db.Builder().Select("1").From("orders").Where("user_id = users.id"))
   ```

2. **Add LIMIT 1 to EXISTS subqueries** (clarity)
   ```go
   db.Builder().Select("1").From("orders").Where("user_id = users.id").Limit(1)
   ```

3. **Index subquery WHERE columns**
   ```sql
   CREATE INDEX idx_orders_user_id ON orders(user_id);
   ```

4. **Use FROM subquery for complex aggregations**
   ```go
   stats := db.Builder().Select("...").From("orders").GroupBy("user_id")
   db.Builder().FromSelect(stats, "s").Where("total > 1000")
   ```

5. **Keep subqueries simple and readable**
   ```go
   // Break complex queries into variables
   subquery := db.Builder().Select("...").From("...").Where("...")
   ```

### ❌ DON'T

1. **Don't use SELECT * in subqueries** (specify columns)
   ```go
   // ❌ Bad
   db.Builder().Select("*").From("orders")

   // ✅ Good
   db.Builder().Select("user_id").From("orders")
   ```

2. **Don't use NOT IN with NULLable columns** (use NOT EXISTS)
   ```go
   // ❌ Bad: returns no rows if NULL exists
   relica.NotIn("id", subquery)

   // ✅ Good: NULL-safe
   relica.NotExists(subquery)
   ```

3. **Don't nest too deeply** (2-3 levels max)
   ```go
   // ❌ Bad: hard to read and debug
   // Use CTE instead for deep nesting
   ```

4. **Don't use correlated subqueries in tight loops**
   ```go
   // ❌ Bad: slow for large datasets
   SelectExpr("(SELECT COUNT(*) FROM orders WHERE user_id = users.id)")

   // ✅ Good: use JOIN instead
   LeftJoin("orders", "orders.user_id = users.id")
   ```

5. **Don't forget aliases for FROM subqueries** (required)
   ```go
   // ❌ Bad: will panic
   FromSelect(subquery, "")

   // ✅ Good
   FromSelect(subquery, "stats")
   ```

## Common Patterns

### Pattern 1: Find Records Without Related Data

```go
// Users with no orders (NOT EXISTS pattern)
func GetUsersWithoutOrders(db *relica.DB) ([]User, error) {
    orderCheck := db.Builder().
        Select("1").
        From("orders").
        Where("orders.user_id = users.id")

    var users []User
    err := db.Builder().
        Select("*").
        From("users").
        Where(relica.NotExists(orderCheck)).
        All(&users)

    return users, err
}
```

### Pattern 2: Top N Per Group

```go
// Top 3 products per category
func GetTopProductsPerCategory(db *relica.DB) ([]Product, error) {
    subquery := db.Builder().
        Select("*").
        SelectExpr("ROW_NUMBER() OVER (PARTITION BY category_id ORDER BY sales DESC)", "rn").
        From("products")

    var topProducts []Product
    err := db.Builder().
        FromSelect(subquery, "ranked").
        Select("*").
        Where("rn <= ?", 3).
        All(&topProducts)

    return topProducts, err
}
```

### Pattern 3: Filtering by Aggregate Threshold

```go
// Users with total spent > $1000
func GetHighSpenders(db *relica.DB) ([]User, error) {
    spendingQuery := db.Builder().
        Select("user_id").
        From("orders").
        GroupBy("user_id").
        Having("SUM(total) > ?", 1000)

    var highSpenders []User
    err := db.Builder().
        Select("u.*").
        From("users u").
        Where(relica.In("u.id", spendingQuery)).
        All(&highSpenders)

    return highSpenders, err
}
```

### Pattern 4: Latest Record Per Group

```go
// Latest order for each user
func GetLatestOrders(db *relica.DB) ([]Order, error) {
    latestOrderIds := db.Builder().
        Select("MAX(id) as order_id").
        From("orders").
        GroupBy("user_id")

    var orders []Order
    err := db.Builder().
        Select("*").
        From("orders").
        Where(relica.In("id", latestOrderIds)).
        All(&orders)

    return orders, err
}
```

### Pattern 5: Conditional Aggregation

```go
// Users with orders in multiple categories
func GetDiverseShoppers(db *relica.DB) ([]User, error) {
    diverseUsers := db.Builder().
        Select("o.user_id").
        From("orders o").
        InnerJoin("order_items oi", "o.id = oi.order_id").
        InnerJoin("products p", "oi.product_id = p.id").
        GroupBy("o.user_id").
        Having("COUNT(DISTINCT p.category_id) >= ?", 3)

    var users []User
    err := db.Builder().
        Select("*").
        From("users").
        Where(relica.In("id", diverseUsers)).
        All(&users)

    return users, err
}
```

## Troubleshooting

### Issue: Subquery Returns NULL

**Problem**: NOT IN with NULL returns no rows
```go
// ❌ BAD: Returns nothing if subquery has NULL
db.Builder().
    Select("*").
    From("users").
    Where(relica.NotIn("id", subquery))
```

**Explanation**: `NULL NOT IN (1, 2, NULL)` evaluates to `UNKNOWN`, which filters out all rows.

**Solution**: Use NOT EXISTS instead
```go
// ✅ GOOD: NULL-safe
orderCheck := db.Builder().
    Select("1").
    From("orders").
    Where("orders.user_id = users.id")

db.Builder().
    Select("*").
    From("users").
    Where(relica.NotExists(orderCheck))
```

### Issue: Slow Correlated Subquery

**Problem**: Correlated subquery executes for each row
```go
// ❌ SLOW: Executes subquery 1M times for 1M users
db.Builder().
    Select("id", "name").
    SelectExpr("(SELECT COUNT(*) FROM orders WHERE user_id = users.id)", "order_count").
    From("users")
```

**Solution**: Use LEFT JOIN + GROUP BY
```go
// ✅ FASTER: Single query with join
db.Builder().
    Select("u.id", "u.name", "COUNT(o.id) as order_count").
    From("users u").
    LeftJoin("orders o", "u.id = o.user_id").
    GroupBy("u.id", "u.name")
```

### Issue: Subquery Returns Multiple Rows in Scalar Context

**Problem**: Scalar subquery returns > 1 row
```go
// ❌ ERROR: Subquery may return multiple rows
SelectExpr("(SELECT name FROM products WHERE category_id = c.id)", "product_name")
```

**Solution**: Add LIMIT or use aggregate
```go
// ✅ GOOD: Limit to one row
SelectExpr("(SELECT name FROM products WHERE category_id = c.id LIMIT 1)", "product_name")

// ✅ BETTER: Use aggregate
SelectExpr("(SELECT COUNT(*) FROM products WHERE category_id = c.id)", "product_count")
```

### Issue: Missing Alias for FROM Subquery

**Problem**: No alias provided for derived table
```go
// ❌ PANIC: Alias required
db.Builder().FromSelect(subquery, "")
```

**Solution**: Always provide alias
```go
// ✅ GOOD
db.Builder().FromSelect(subquery, "stats")
```

### Issue: Parameter Order Confusion

**Problem**: Parameters in wrong order with multiple subqueries
```go
// Complex query with multiple subqueries
```

**Solution**: Build step-by-step and test SQL output
```go
query := db.Builder().
    Select("*").
    Where(relica.In("id", subquery1)).
    Where(relica.Exists(subquery2)).
    Build()

// Print SQL to verify parameter order
fmt.Println(query.SQL())
fmt.Println(query.Params())
```

## Examples Repository

Complete working examples available in Relica repository:

- `examples/subqueries/in_subquery.go` - IN and NOT IN examples
- `examples/subqueries/exists.go` - EXISTS and NOT EXISTS patterns
- `examples/subqueries/from_subquery.go` - Derived table examples
- `examples/subqueries/scalar.go` - Scalar subquery patterns
- `examples/subqueries/correlated.go` - Correlated subquery optimization

## Further Reading

- [CTE Guide](./CTE_GUIDE.md) - Alternative to subqueries for complex queries
- [Set Operations Guide](./SET_OPERATIONS_GUIDE.md) - Combining query results
- [Window Functions Guide](./WINDOW_FUNCTIONS_GUIDE.md) - Advanced analytics

---

**Last Updated**: 2025-01-25
**Relica Version**: v0.3.0-beta
**Minimum Go Version**: 1.25+
