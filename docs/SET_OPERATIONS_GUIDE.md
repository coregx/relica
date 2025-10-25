# Set Operations Guide

> Complete guide to combining queries with UNION, INTERSECT, and EXCEPT in Relica

## Table of Contents
- [Introduction](#introduction)
- [UNION Operations](#union-operations)
- [UNION ALL Operations](#union-all-operations)
- [INTERSECT Operations](#intersect-operations)
- [EXCEPT Operations](#except-operations)
- [Chaining Set Operations](#chaining-set-operations)
- [When to Use Set Operations](#when-to-use-set-operations)
- [Performance Considerations](#performance-considerations)
- [Database Compatibility](#database-compatibility)
- [Best Practices](#best-practices)
- [Common Patterns](#common-patterns)
- [Troubleshooting](#troubleshooting)

## Introduction

Set operations combine results from two or more SELECT queries. Think of them as mathematical set operations (∪ union, ∩ intersection, − difference) applied to database result sets.

**Key Benefits**:
- Combine data from multiple tables with similar structure
- Remove duplicates automatically (UNION)
- Find common records (INTERSECT)
- Find differences between datasets (EXCEPT)
- Simplify complex OR conditions

**Version Support**: Relica v0.3.0-beta and later

## UNION Operations

UNION combines results from multiple queries and **removes duplicates**.

### Basic UNION

```go
package main

import (
    "github.com/coregx/relica"
)

// Get all unique names from both active and archived users
func GetAllUserNames(db *relica.DB) ([]string, error) {
    q1 := db.Builder().
        Select("name").
        From("users").
        Where("status = ?", 1)

    q2 := db.Builder().
        Select("name").
        From("archived_users").
        Where("status = ?", 1)

    var names []string
    query := q1.Union(q2).Build()
    err := query.All(&names)

    return names, err
}
```

**Generated SQL** (PostgreSQL):
```sql
SELECT "name" FROM "users" WHERE status = $1
UNION
SELECT "name" FROM "archived_users" WHERE status = $2
```

**Result**: Unique names from both tables (duplicates removed).

### UNION with Different Tables

```go
// Combine customer emails from multiple sources
func GetAllCustomerEmails(db *relica.DB) ([]string, error) {
    customers := db.Builder().
        Select("email").
        From("customers").
        Where("active = ?", true)

    subscribers := db.Builder().
        Select("email").
        From("newsletter_subscribers").
        Where("subscribed = ?", true)

    partners := db.Builder().
        Select("contact_email").
        From("business_partners")

    var emails []string
    query := customers.Union(subscribers).Union(partners).Build()
    err := query.All(&emails)

    return emails, err
}
```

**Generated SQL**:
```sql
SELECT "email" FROM "customers" WHERE active = $1
UNION
SELECT "email" FROM "newsletter_subscribers" WHERE subscribed = $2
UNION
SELECT "contact_email" FROM "business_partners"
```

### UNION with ORDER BY and LIMIT

```go
// Get top 10 unique product names from current and legacy catalogs
func GetTopProductNames(db *relica.DB) ([]Product, error) {
    type Product struct {
        Name  string `db:"name"`
        Price float64 `db:"price"`
    }

    current := db.Builder().
        Select("name", "price").
        From("products").
        Where("active = ?", true)

    legacy := db.Builder().
        Select("name", "price").
        From("legacy_products").
        Where("available = ?", true)

    var products []Product
    query := current.Union(legacy).
        OrderBy("name").
        Limit(10).
        Build()

    err := query.All(&products)
    return products, err
}
```

**Generated SQL**:
```sql
SELECT "name", "price" FROM "products" WHERE active = $1
UNION
SELECT "name", "price" FROM "legacy_products" WHERE available = $2
ORDER BY "name"
LIMIT 10
```

**💡 Tip**: ORDER BY and LIMIT apply to the **entire combined result**, not individual queries.

### When to Use UNION

**✅ Use UNION when**:
- Need unique results from multiple sources
- Data structure is similar across tables
- Want to remove duplicates
- Combining historical and current data
- Merging partitioned tables

**❌ Use UNION ALL instead when**:
- Performance is critical (UNION ALL is faster)
- Duplicates are impossible or desired
- Large result sets (duplicate removal is expensive)

## UNION ALL Operations

UNION ALL combines results and **keeps all duplicates**. Much faster than UNION.

### Basic UNION ALL

```go
// Get all order IDs from 2023 and 2024 (including duplicates if any)
func GetAllOrderIDs(db *relica.DB) ([]int, error) {
    orders2023 := db.Builder().
        Select("id").
        From("orders_2023")

    orders2024 := db.Builder().
        Select("id").
        From("orders_2024")

    var ids []int
    query := orders2023.UnionAll(orders2024).Build()
    err := query.All(&ids)

    return ids, err
}
```

**Generated SQL**:
```sql
SELECT "id" FROM "orders_2023"
UNION ALL
SELECT "id" FROM "orders_2024"
```

**Result**: All IDs from both tables, duplicates included.

### UNION ALL for Partitioned Tables

```go
// Query across time-partitioned tables
func GetOrdersDateRange(db *relica.DB, startDate, endDate time.Time) ([]Order, error) {
    q2023 := db.Builder().
        Select("*").
        From("orders_2023").
        Where("created_at BETWEEN ? AND ?", startDate, endDate)

    q2024 := db.Builder().
        Select("*").
        From("orders_2024").
        Where("created_at BETWEEN ? AND ?", startDate, endDate)

    q2025 := db.Builder().
        Select("*").
        From("orders_2025").
        Where("created_at BETWEEN ? AND ?", startDate, endDate)

    var orders []Order
    query := q2023.UnionAll(q2024).UnionAll(q2025).Build()
    err := query.All(&orders)

    return orders, err
}
```

**Performance**: 3-5x faster than UNION for large datasets.

### UNION ALL with Aggregation

```go
// Total sales across all regions
func GetTotalSalesByRegion(db *relica.DB) ([]RegionSales, error) {
    type RegionSales struct {
        Region string `db:"region"`
        Total  float64 `db:"total"`
    }

    north := db.Builder().
        Select("'North' as region", "SUM(amount) as total").
        From("sales_north")

    south := db.Builder().
        Select("'South' as region", "SUM(amount) as total").
        From("sales_south")

    east := db.Builder().
        Select("'East' as region", "SUM(amount) as total").
        From("sales_east")

    west := db.Builder().
        Select("'West' as region", "SUM(amount) as total").
        From("sales_west")

    var sales []RegionSales
    query := north.UnionAll(south).UnionAll(east).UnionAll(west).Build()
    err := query.All(&sales)

    return sales, err
}
```

### Performance Comparison

**Benchmark** (1M rows each table):

| Operation | Time | Memory | Notes |
|-----------|------|--------|-------|
| UNION | ~3500ms | High | Sorts + deduplicates |
| UNION ALL | ~800ms | Low | Direct concatenation |

**Speedup**: UNION ALL is **3-5x faster** for large datasets.

### When to Use UNION ALL

**✅ Use UNION ALL when**:
- Performance is important
- Duplicates are impossible (different partitions)
- Duplicates don't matter
- Combining disjoint datasets
- Temporary intermediate results

**❌ Use UNION instead when**:
- Must guarantee unique results
- Small result sets (performance difference negligible)
- Business logic requires duplicate removal

## INTERSECT Operations

INTERSECT returns only rows that appear in **both** queries (set intersection ∩).

**Database Support**:
- PostgreSQL: ✓ All versions
- MySQL: ✓ 8.0.31+ only
- SQLite: ✓ 3.25+

### Basic INTERSECT

```go
// Find users who are both premium members AND forum participants
func GetEngagedPremiumUsers(db *relica.DB) ([]int, error) {
    premiumMembers := db.Builder().
        Select("user_id").
        From("premium_memberships").
        Where("active = ?", true)

    forumParticipants := db.Builder().
        Select("user_id").
        From("forum_posts").
        GroupBy("user_id").
        Having("COUNT(*) >= ?", 10)

    var userIDs []int
    query := premiumMembers.Intersect(forumParticipants).Build()
    err := query.All(&userIDs)

    return userIDs, err
}
```

**Generated SQL**:
```sql
SELECT "user_id" FROM "premium_memberships" WHERE active = $1
INTERSECT
SELECT "user_id" FROM "forum_posts" GROUP BY "user_id" HAVING COUNT(*) >= $2
```

**Result**: Only user IDs present in **both** result sets.

### INTERSECT for Common Records

```go
// Find products sold in both stores
func GetCommonProducts(db *relica.DB) ([]Product, error) {
    store1Products := db.Builder().
        Select("product_id", "name").
        From("store1_inventory").
        Where("stock > ?", 0)

    store2Products := db.Builder().
        Select("product_id", "name").
        From("store2_inventory").
        Where("stock > ?", 0)

    var products []Product
    query := store1Products.Intersect(store2Products).Build()
    err := query.All(&products)

    return products, err
}
```

### When to Use INTERSECT

**✅ Use INTERSECT when**:
- Finding records present in multiple datasets
- Validating data consistency
- Finding overlapping categories
- Business logic requires "AND" across tables

**❌ Use INNER JOIN instead when**:
- Need columns from both sides
- More complex join conditions
- Better performance (often faster than INTERSECT)
- MySQL < 8.0.31 (no INTERSECT support)

**Alternative with EXISTS**:
```go
// Equivalent to INTERSECT using EXISTS (works on all databases)
db.Builder().
    Select("user_id").
    From("premium_memberships").
    Where("active = ?", true).
    Where(relica.Exists(
        db.Builder().
            Select("1").
            From("forum_posts").
            Where("forum_posts.user_id = premium_memberships.user_id").
            GroupBy("user_id").
            Having("COUNT(*) >= ?", 10),
    ))
```

## EXCEPT Operations

EXCEPT returns rows from first query that are **not** in second query (set difference −).

**Database Support**:
- PostgreSQL: ✓ All versions
- MySQL: ✓ 8.0.31+ only
- SQLite: ✓ 3.25+

**Note**: SQL standard uses `EXCEPT`, but some databases call it `MINUS` (Oracle, older MySQL).

### Basic EXCEPT

```go
// Find users who registered but never placed an order
func GetInactiveUsers(db *relica.DB) ([]int, error) {
    allUsers := db.Builder().
        Select("id").
        From("users")

    usersWithOrders := db.Builder().
        Select("user_id").
        From("orders")

    var inactiveUserIDs []int
    query := allUsers.Except(usersWithOrders).Build()
    err := query.All(&inactiveUserIDs)

    return inactiveUserIDs, err
}
```

**Generated SQL**:
```sql
SELECT "id" FROM "users"
EXCEPT
SELECT "user_id" FROM "orders"
```

**Result**: User IDs from `users` table that don't appear in `orders` table.

### EXCEPT with Complex Queries

```go
// Find products in inventory but not in active orders
func GetUnorderedProducts(db *relica.DB) ([]Product, error) {
    inventory := db.Builder().
        Select("product_id", "name").
        From("inventory").
        Where("stock > ?", 0)

    orderedProducts := db.Builder().
        Select("product_id", "name").
        From("order_items oi").
        InnerJoin("orders o", "oi.order_id = o.id").
        Where("o.status IN (?, ?)", "pending", "processing")

    var products []Product
    query := inventory.Except(orderedProducts).Build()
    err := query.All(&products)

    return products, err
}
```

### Multiple EXCEPT Operations

```go
// Find active users excluding banned and suspended users
func GetActiveNonBannedUsers(db *relica.DB) ([]int, error) {
    allUsers := db.Builder().
        Select("id").
        From("users").
        Where("active = ?", true)

    bannedUsers := db.Builder().
        Select("user_id").
        From("banned_accounts")

    suspendedUsers := db.Builder().
        Select("user_id").
        From("suspended_accounts")

    var userIDs []int
    query := allUsers.Except(bannedUsers).Except(suspendedUsers).Build()
    err := query.All(&userIDs)

    return userIDs, err
}
```

**Generated SQL**:
```sql
SELECT "id" FROM "users" WHERE active = $1
EXCEPT
SELECT "user_id" FROM "banned_accounts"
EXCEPT
SELECT "user_id" FROM "suspended_accounts"
```

### When to Use EXCEPT

**✅ Use EXCEPT when**:
- Finding records in one set but not another
- Data validation (comparing expected vs actual)
- Excluding specific subsets
- Set difference logic

**❌ Use NOT EXISTS instead when**:
- Better performance needed
- MySQL < 8.0.31 (no EXCEPT support)
- More complex exclusion logic
- NULL-safe operations required

**Alternative with NOT EXISTS**:
```go
// Equivalent to EXCEPT using NOT EXISTS (works on all databases)
db.Builder().
    Select("id").
    From("users").
    Where(relica.NotExists(
        db.Builder().
            Select("1").
            From("orders").
            Where("orders.user_id = users.id"),
    ))
```

## Chaining Set Operations

You can chain multiple set operations to create complex queries.

### Mixing UNION and EXCEPT

```go
// (Products from store1 UNION products from store2) EXCEPT discontinued products
func GetAvailableProducts(db *relica.DB) ([]int, error) {
    store1 := db.Builder().
        Select("product_id").
        From("store1_inventory")

    store2 := db.Builder().
        Select("product_id").
        From("store2_inventory")

    discontinued := db.Builder().
        Select("id").
        From("discontinued_products")

    var productIDs []int
    query := store1.Union(store2).Except(discontinued).Build()
    err := query.All(&productIDs)

    return productIDs, err
}
```

**Generated SQL**:
```sql
SELECT "product_id" FROM "store1_inventory"
UNION
SELECT "product_id" FROM "store2_inventory"
EXCEPT
SELECT "id" FROM "discontinued_products"
```

### Complex Chaining

```go
// Users active in forums OR purchased recently, AND have premium membership
func GetEngagedPremiumUsers(db *relica.DB, since time.Time) ([]int, error) {
    forumUsers := db.Builder().
        Select("user_id").
        From("forum_posts").
        Where("created_at > ?", since).
        GroupBy("user_id")

    buyers := db.Builder().
        Select("user_id").
        From("orders").
        Where("created_at > ?", since).
        GroupBy("user_id")

    premiumMembers := db.Builder().
        Select("user_id").
        From("premium_memberships").
        Where("active = ?", true)

    var userIDs []int
    // (forum users UNION buyers) INTERSECT premium members
    query := forumUsers.Union(buyers).Intersect(premiumMembers).Build()
    err := query.All(&userIDs)

    return userIDs, err
}
```

**Evaluation Order**: Left to right (like math operations without parentheses).

### Precedence Rules

SQL set operations have equal precedence and evaluate left-to-right:

```sql
A UNION B INTERSECT C EXCEPT D
-- Evaluates as: ((A UNION B) INTERSECT C) EXCEPT D
```

**💡 Tip**: Use subqueries for complex precedence control:

```go
// Force evaluation order with subqueries
unionResult := q1.Union(q2)
intersectResult := unionResult.Intersect(q3)
finalResult := intersectResult.Except(q4)
```

## When to Use Set Operations

### Decision Tree

```
Need to combine query results?
├─ Remove duplicates? → UNION
├─ Keep duplicates? → UNION ALL (faster)
├─ Find common records?
│  ├─ Simple match → INTERSECT
│  └─ Need joined columns → INNER JOIN
└─ Find differences?
   ├─ Simple exclusion → EXCEPT
   └─ Complex logic → NOT EXISTS
```

### Use Cases

**UNION**:
- Merging similar tables (current + archived)
- Combining multiple data sources
- Deduplicating across datasets
- Historical + current data

**UNION ALL**:
- Time-partitioned tables (guaranteed no duplicates)
- Performance-critical queries
- Temporary result sets
- Log aggregation

**INTERSECT**:
- Finding overlaps
- Data validation
- "AND" logic across tables
- Set membership tests

**EXCEPT**:
- Finding missing records
- Exclusion logic
- Data diff operations
- "NOT IN" alternative

### Set Operations vs Alternatives

| Scenario | Set Operation | Alternative | Recommendation |
|----------|--------------|-------------|----------------|
| Combine unique records | UNION | DISTINCT + UNION ALL | UNION (clearer) |
| Find common records | INTERSECT | INNER JOIN | JOIN (need columns) |
| Exclude records | EXCEPT | NOT EXISTS | NOT EXISTS (faster) |
| Combine partitions | UNION ALL | — | UNION ALL (best) |

## Performance Considerations

### 1. UNION vs UNION ALL

**Benchmark** (1M rows × 2 tables):

| Operation | CPU Time | Memory | I/O |
|-----------|----------|--------|-----|
| UNION | ~3500ms | 450 MB | High (sorts) |
| UNION ALL | ~800ms | 120 MB | Low |

**Why UNION ALL is faster**:
- No deduplication (no sort/hash)
- Streaming results (no buffering)
- Lower memory usage

**Use UNION ALL when possible**.

### 2. Column Count and Types

**Rule**: All queries in set operation must have:
- Same number of columns
- Compatible column types
- Same column order

```go
// ❌ BAD: Column count mismatch
q1 := db.Builder().Select("id", "name").From("users")
q2 := db.Builder().Select("id").From("archived_users")
q1.Union(q2) // Error: column count mismatch

// ✅ GOOD: Same columns
q1 := db.Builder().Select("id", "name").From("users")
q2 := db.Builder().Select("id", "name").From("archived_users")
q1.Union(q2) // OK
```

### 3. Indexing

**Index the WHERE clauses** in each query:

```sql
-- Index columns used in WHERE clauses
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_archived_status ON archived_users(status);
```

**Impact**: 10-100x faster for filtered queries.

### 4. ORDER BY and LIMIT

**Apply to final result only**:

```go
// ORDER BY applies to combined result
query := q1.Union(q2).
    OrderBy("name").  // Sorts entire UNION result
    Limit(10)         // Top 10 from combined result
```

**Performance**: Sorting happens **after** set operation (can be slow for large results).

### 5. Parallelization

Modern databases can parallelize set operations:

- PostgreSQL 9.6+: Parallel UNION/UNION ALL
- MySQL 8.0.14+: Parallel set operations
- SQLite: Single-threaded

**Benefit**: 2-4x speedup on multi-core systems.

## Database Compatibility

| Operation | PostgreSQL | MySQL 5.7 | MySQL 8.0.0-30 | MySQL 8.0.31+ | SQLite 3.25+ |
|-----------|-----------|-----------|----------------|---------------|--------------|
| UNION | ✓ All | ✓ | ✓ | ✓ | ✓ |
| UNION ALL | ✓ All | ✓ | ✓ | ✓ | ✓ |
| INTERSECT | ✓ All | ❌ | ❌ | ✓ | ✓ |
| EXCEPT | ✓ All | ❌ | ❌ | ✓ | ✓ |
| ORDER BY | ✓ | ✓ | ✓ | ✓ | ✓ |
| LIMIT | ✓ | ✓ | ✓ | ✓ | ✓ |

**MySQL Version Notes**:
- **MySQL 5.7**: UNION and UNION ALL only
- **MySQL 8.0.0-8.0.30**: UNION and UNION ALL only
- **MySQL 8.0.31+**: Full support (INTERSECT, EXCEPT added)
- **MySQL workarounds**: Use JOIN/EXISTS for older versions

**Workaround for MySQL < 8.0.31**:

```go
// INTERSECT workaround using EXISTS
func intersectWorkaround(db *relica.DB) {
    // Instead of: q1.Intersect(q2)
    // Use: q1 WHERE EXISTS (SELECT ... FROM q2)
    q1 := db.Builder().
        Select("user_id").
        From("premium_memberships").
        Where(relica.Exists(
            db.Builder().
                Select("1").
                From("forum_posts").
                Where("forum_posts.user_id = premium_memberships.user_id"),
        ))
}

// EXCEPT workaround using NOT EXISTS
func exceptWorkaround(db *relica.DB) {
    // Instead of: q1.Except(q2)
    // Use: q1 WHERE NOT EXISTS (SELECT ... FROM q2)
    q1 := db.Builder().
        Select("id").
        From("users").
        Where(relica.NotExists(
            db.Builder().
                Select("1").
                From("orders").
                Where("orders.user_id = users.id"),
        ))
}
```

## Best Practices

### ✅ DO

1. **Use UNION ALL when duplicates don't matter** (3-5x faster)
   ```go
   q1.UnionAll(q2) // Faster than Union(q2)
   ```

2. **Match column count and types**
   ```go
   q1 := db.Builder().Select("id", "name").From("users")
   q2 := db.Builder().Select("id", "name").From("archived_users")
   ```

3. **Apply ORDER BY to final result**
   ```go
   q1.Union(q2).OrderBy("name")
   ```

4. **Use meaningful column aliases**
   ```go
   q1 := db.Builder().Select("'active' as source", "id", "name").From("users")
   q2 := db.Builder().Select("'archived' as source", "id", "name").From("archived_users")
   ```

5. **Index WHERE clause columns**
   ```sql
   CREATE INDEX idx_status ON users(status);
   ```

### ❌ DON'T

1. **Don't use UNION when UNION ALL works** (unnecessary overhead)
   ```go
   // ❌ Bad: Slow
   q1.Union(q2) // Removes duplicates you don't need

   // ✅ Good: Fast
   q1.UnionAll(q2)
   ```

2. **Don't mix incompatible column types**
   ```go
   // ❌ Bad: Type mismatch
   q1 := db.Builder().Select("id", "created_at").From("users")
   q2 := db.Builder().Select("id", "name").From("archived") // String vs timestamp
   ```

3. **Don't use set operations when JOIN is clearer**
   ```go
   // ❌ Bad: Unclear intent
   INTERSECT for simple matching

   // ✅ Good: Clear intent
   INNER JOIN
   ```

4. **Don't forget column aliases for literals**
   ```go
   // ❌ Bad: Unclear column name
   Select("'North'")

   // ✅ Good: Named column
   Select("'North' as region")
   ```

5. **Don't rely on column position** (use explicit aliases)
   ```go
   // ❌ Bad: Fragile
   Select("name", "email") // What if column order changes?

   // ✅ Good: Explicit
   Select("name as user_name", "email as user_email")
   ```

## Common Patterns

### Pattern 1: Merging Current and Historical Data

```go
// Combine current orders with archived orders
func GetAllOrders(db *relica.DB, userID int) ([]Order, error) {
    current := db.Builder().
        Select("*").
        From("orders").
        Where("user_id = ?", userID)

    archived := db.Builder().
        Select("*").
        From("orders_archive").
        Where("user_id = ?", userID)

    var orders []Order
    query := current.UnionAll(archived).
        OrderBy("created_at DESC").
        Build()

    err := query.All(&orders)
    return orders, err
}
```

### Pattern 2: Deduplicating Across Sources

```go
// Get unique email addresses from multiple sources
func GetAllUniqueEmails(db *relica.DB) ([]string, error) {
    customers := db.Builder().Select("email").From("customers")
    subscribers := db.Builder().Select("email").From("newsletter")
    partners := db.Builder().Select("email").From("partners")

    var emails []string
    query := customers.Union(subscribers).Union(partners).Build()
    err := query.All(&emails)

    return emails, err
}
```

### Pattern 3: Partitioned Table Queries

```go
// Query time-partitioned tables
func GetOrdersByDateRange(db *relica.DB, start, end time.Time) ([]Order, error) {
    var queries []*relica.SelectQuery

    // Determine which partitions to query
    years := []int{2023, 2024, 2025}
    for _, year := range years {
        q := db.Builder().
            Select("*").
            From(fmt.Sprintf("orders_%d", year)).
            Where("created_at BETWEEN ? AND ?", start, end)
        queries = append(queries, q)
    }

    // Combine all partitions with UNION ALL
    result := queries[0]
    for i := 1; i < len(queries); i++ {
        result = result.UnionAll(queries[i])
    }

    var orders []Order
    query := result.OrderBy("created_at DESC").Build()
    err := query.All(&orders)

    return orders, err
}
```

### Pattern 4: Complex Filtering with Set Operations

```go
// Find VIP customers: (high spenders OR frequent buyers) AND active
func GetVIPCustomers(db *relica.DB) ([]int, error) {
    highSpenders := db.Builder().
        Select("user_id").
        From("orders").
        GroupBy("user_id").
        Having("SUM(total) > ?", 10000)

    frequentBuyers := db.Builder().
        Select("user_id").
        From("orders").
        GroupBy("user_id").
        Having("COUNT(*) >= ?", 50)

    activeUsers := db.Builder().
        Select("id").
        From("users").
        Where("active = ? AND last_login > ?", true, time.Now().AddDate(0, -1, 0))

    var vipIDs []int
    query := highSpenders.Union(frequentBuyers).Intersect(activeUsers).Build()
    err := query.All(&vipIDs)

    return vipIDs, err
}
```

### Pattern 5: Data Validation with EXCEPT

```go
// Find orphaned order items (items without valid orders)
func FindOrphanedItems(db *relica.DB) ([]int, error) {
    allItemOrderIDs := db.Builder().
        Select("order_id").
        From("order_items").
        GroupBy("order_id")

    validOrderIDs := db.Builder().
        Select("id").
        From("orders")

    var orphanedIDs []int
    query := allItemOrderIDs.Except(validOrderIDs).Build()
    err := query.All(&orphanedIDs)

    return orphanedIDs, err
}
```

## Troubleshooting

### Issue: Column Count Mismatch

**Problem**: Queries have different number of columns
```go
// ❌ ERROR: The used SELECT statements have a different number of columns
q1 := db.Builder().Select("id", "name").From("users")
q2 := db.Builder().Select("id").From("archived")
q1.Union(q2)
```

**Solution**: Match column counts
```go
// ✅ GOOD: Same column count
q1 := db.Builder().Select("id", "name").From("users")
q2 := db.Builder().Select("id", "NULL as name").From("archived") // Add placeholder
q1.Union(q2)
```

### Issue: Type Mismatch

**Problem**: Column types are incompatible
```go
// ❌ ERROR: Types don't match (INT vs VARCHAR)
q1 := db.Builder().Select("id").From("users") // INT
q2 := db.Builder().Select("name").From("archived") // VARCHAR
```

**Solution**: Cast types
```go
// ✅ GOOD: Cast to compatible type
q1 := db.Builder().Select("CAST(id AS VARCHAR)").From("users")
q2 := db.Builder().Select("name").From("archived")
```

### Issue: INTERSECT/EXCEPT Not Supported

**Problem**: MySQL < 8.0.31 doesn't support INTERSECT/EXCEPT
```go
// ❌ ERROR: You have an error in your SQL syntax (MySQL 8.0.30)
q1.Intersect(q2)
```

**Solution**: Use JOIN or EXISTS
```go
// ✅ GOOD: INTERSECT alternative using EXISTS
q1 := db.Builder().
    Select("user_id").
    From("premium_memberships").
    Where(relica.Exists(
        db.Builder().
            Select("1").
            From("forum_posts").
            Where("forum_posts.user_id = premium_memberships.user_id"),
    ))

// ✅ GOOD: EXCEPT alternative using NOT EXISTS
q1 := db.Builder().
    Select("id").
    From("users").
    Where(relica.NotExists(
        db.Builder().
            Select("1").
            From("orders").
            Where("orders.user_id = users.id"),
    ))
```

### Issue: Slow UNION Performance

**Problem**: UNION is slow on large datasets
```go
// ❌ SLOW: Deduplication overhead
q1.Union(q2) // 3500ms for 2M rows
```

**Solution**: Use UNION ALL if duplicates don't matter
```go
// ✅ FAST: No deduplication
q1.UnionAll(q2) // 800ms for 2M rows (4.3x faster)
```

### Issue: ORDER BY Not Working

**Problem**: ORDER BY in individual queries ignored
```go
// ❌ ORDER BY in q1 is ignored
q1 := db.Builder().Select("name").From("users").OrderBy("name")
q2 := db.Builder().Select("name").From("archived")
q1.Union(q2) // q1's ORDER BY has no effect
```

**Solution**: Apply ORDER BY to final result
```go
// ✅ GOOD: ORDER BY on union result
q1 := db.Builder().Select("name").From("users")
q2 := db.Builder().Select("name").From("archived")
q1.Union(q2).OrderBy("name")
```

## Examples Repository

Complete working examples available in Relica repository:

- `examples/set_operations/union.go` - UNION examples
- `examples/set_operations/union_all.go` - UNION ALL patterns
- `examples/set_operations/intersect.go` - INTERSECT examples
- `examples/set_operations/except.go` - EXCEPT patterns
- `examples/set_operations/chaining.go` - Complex chaining

## Further Reading

- [Subquery Guide](./SUBQUERY_GUIDE.md) - Alternative to set operations for filtering
- [CTE Guide](./CTE_GUIDE.md) - Simplify complex set operations with CTEs
- [Performance Tuning](./PERFORMANCE.md) - Optimize query performance

---

**Last Updated**: 2025-01-25
**Relica Version**: v0.3.0-beta
**Minimum Go Version**: 1.25+
