# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0-beta] - 2025-02-03

### Added

#### JOIN Operations (Phase 1)

**Full JOIN support** for solving N+1 query problems and building complex relational queries.

- `Join(type, table, on)` - Generic JOIN method accepting any JOIN type
- `InnerJoin(table, on)` - INNER JOIN convenience method
- `LeftJoin(table, on)` - LEFT OUTER JOIN
- `RightJoin(table, on)` - RIGHT OUTER JOIN
- `FullJoin(table, on)` - FULL OUTER JOIN (PostgreSQL, SQLite only)
- `CrossJoin(table)` - CROSS JOIN (no ON condition)

**Features**:
- Table alias support: `"users u"`, `"messages m"` automatically parsed as `"users" AS "u"`
- Three ON condition styles:
  - String: `"m.user_id = u.id"` (simple, familiar SQL)
  - Expression: `relica.And(relica.Raw("m.user_id = u.id"), relica.Eq("u.status", 1))` (type-safe)
  - nil: CROSS JOIN only (no condition)
- Multi-JOIN support: Chain multiple JOINs in single query
- Proper SQL generation with dialect-specific quoting

**Example**:
```go
db.Builder().
    Select("m.subject", "u.name", "COUNT(a.id) as attachment_count").
    From("messages m").
    InnerJoin("users u", "m.user_id = u.id").
    LeftJoin("attachments a", "m.id = a.message_id").
    Where("m.status = ?", 1).
    GroupBy("m.id").
    All(&results)
```

#### Sorting and Pagination (Phase 2)

**Database-side sorting and pagination** for efficient data retrieval.

- `OrderBy(columns...)` - ORDER BY clause with multiple columns
  - Direction support: ASC/DESC (case-insensitive, defaults to ASC)
  - Multiple columns: `OrderBy("age DESC", "name ASC")`
  - Table prefixes: `OrderBy("users.age DESC", "messages.created_at")`
  - Chainable: `OrderBy("col1").OrderBy("col2")` appends columns
- `Limit(n)` - LIMIT clause for result size
  - Pointer-based (nil = not set, distinguishes from LIMIT 0)
- `Offset(n)` - OFFSET clause for pagination
  - Pointer-based (nil = not set)

**Features**:
- Automatic column name quoting with `quoteColumnName()` helper
- Table.column format support: `"users.age"` → `"users"."age"`
- Correct SQL order: WHERE → ORDER BY → LIMIT → OFFSET
- Zero value handling: LIMIT 0, OFFSET 0 generate valid SQL

**Example**:
```go
const pageSize = 100
const pageNumber = 2 // Third page (0-indexed)

db.Builder().
    Select().
    From("messages").
    OrderBy("created_at DESC", "id ASC").
    Limit(pageSize).
    Offset(pageNumber * pageSize).
    All(&messages)
```

#### Aggregate Functions (Phase 3)

**Database-side aggregations** for COUNT, SUM, AVG, MIN, MAX with GROUP BY and HAVING support.

**Aggregate Detection**:
- Automatic detection: Column contains `(` → treated as aggregate function
- No special syntax required: Just use `"COUNT(*)"`, `"SUM(size)"`, etc.
- Supports: COUNT, SUM, AVG, MIN, MAX, and database-specific functions
- Alias support: `"COUNT(*) as total"`, `"SUM(size) as total_size"`

**GROUP BY**:
- `GroupBy(columns...)` - GROUP BY clause
  - Single column: `GroupBy("user_id")`
  - Multiple columns: `GroupBy("user_id", "mailbox_id")`
  - Table prefixes: `GroupBy("messages.mailbox_id")`
  - Chainable: `GroupBy("col1").GroupBy("col2")`

**HAVING**:
- `Having(condition, args...)` - HAVING clause (WHERE for aggregates)
  - Accepts string: `Having("COUNT(*) > ?", 100)`
  - Accepts Expression: `Having(relica.GreaterThan("COUNT(*)", 100))`
  - Multiple HAVING clauses combined with AND
  - Proper placeholder handling for PostgreSQL ($1, $2) vs MySQL/SQLite (?)

**Features**:
- Correct SQL order: WHERE → GROUP BY → HAVING → ORDER BY → LIMIT → OFFSET
- Multi-database support: PostgreSQL ($1), MySQL (?), SQLite (?)
- Automatic quoting for GROUP BY columns
- Placeholder renumbering for HAVING in PostgreSQL

**Example**:
```go
type UserStats struct {
    UserID       int   `db:"user_id"`
    MessageCount int   `db:"message_count"`
    TotalSize    int64 `db:"total_size"`
}

var stats []UserStats
db.Builder().
    Select("user_id", "COUNT(*) as message_count", "SUM(size) as total_size").
    From("messages").
    GroupBy("user_id").
    Having("COUNT(*) > ?", 100).
    OrderBy("message_count DESC").
    Limit(50).
    All(&stats)
```

### Performance

**Validated in integration tests** across PostgreSQL, MySQL, SQLite:

#### N+1 Query Reduction (JOIN)
- **Before**: 101 queries (1 parent + 100 children)
- **After**: 1 query (with JOIN)
- **Improvement**: 100x query reduction

**Benchmarks**:
- SQLite: 31ms (101 queries) → 4.7ms (1 query) = **6.6x faster**
- PostgreSQL: 163ms (101 queries) → 9ms (1 query) = **18x faster**
- MySQL: 279ms (101 queries) → 90ms (1 query) = **3x faster**

#### Memory Reduction (LIMIT)
- **Before**: Fetch 10,000 messages (20MB memory)
- **After**: Fetch 100 messages with LIMIT (200KB memory)
- **Improvement**: 100x memory reduction

**Benchmarks**:
- SQLite: 40ms, 20MB → 13ms, 200KB = **3x faster, 100x less memory**
- PostgreSQL: 30ms, 20MB → 5ms, 200KB = **6x faster, 100x less memory**

#### Memory Reduction (COUNT Aggregates)
- **Before**: Fetch 10,000 messages, count in memory (20MB)
- **After**: Database COUNT(*) (8 bytes)
- **Improvement**: 2,500,000x memory reduction

**Benchmarks**:
- SQLite: 40ms, 20MB → <1ms, 8 bytes = **40x faster, 2.5M less memory**
- PostgreSQL: 30ms, 20MB → <1ms, 8 bytes = **30x faster, 2.5M less memory**

#### Real-World Impact (IrisMX mail server, 10K+ users)
- Message listing: 200ms → 10ms (**20x faster**)
- Mailbox stats: 200MB memory → 8 bytes (**200,000x reduction**)
- N+1 problem eliminated: 101 queries → 1 query

### Multi-Database Support

All features validated on:
- ✅ **PostgreSQL 15+** (full support including FULL OUTER JOIN)
- ✅ **MySQL 8.0+** (full support except FULL OUTER JOIN - database limitation)
- ✅ **SQLite 3.x** (full support including FULL OUTER JOIN as of SQLite 3.39+)

### Breaking Changes

**None** - Fully backward compatible with v0.1.2-beta.

All existing code works without modifications. New features are additive only.

### Documentation

**New Guides**:
- [JOIN Operations Guide](docs/dev/reports/JOIN_GUIDE.md) - Comprehensive JOIN guide (~450 lines)
  - All JOIN types with examples
  - ON condition styles (string, Expression, nil)
  - Table aliases
  - Performance benchmarks
  - N+1 problem migration guide
  - Best practices and troubleshooting
  - Real use cases from IrisMX mail server

- [Aggregate Functions Guide](docs/dev/reports/AGGREGATES_GUIDE.md) - Complete aggregates guide (~450 lines)
  - COUNT, SUM, AVG, MIN, MAX functions
  - GROUP BY patterns
  - HAVING vs WHERE
  - Performance comparisons (fetch all vs aggregate)
  - Common patterns (mailbox stats, user quotas, daily analytics)
  - Best practices and troubleshooting

**Updated**:
- README.md - Added JOIN, ORDER BY, Aggregates sections with examples
- CHANGELOG.md - v0.2.0-beta release notes
- Feature list updated (11 features, up from 8)
- Project status: 88.9% coverage (up from 83%)

### Testing

**Unit Tests**:
- Phase 1 (JOIN): 11 tests, 87.7% coverage
- Phase 2 (ORDER BY, LIMIT, OFFSET): 16 tests, 88.4% coverage
- Phase 3 (Aggregates, GROUP BY, HAVING): 21 tests, 88.9% coverage
- **Total**: 48 new unit tests

**Integration Tests**:
- 5 IrisMX use case tests (SQLite, PostgreSQL, MySQL)
- 6 multi-database test suites × 3 databases = 18 integration tests
- Complex query test (JOIN + WHERE + GROUP BY + HAVING + ORDER BY + LIMIT)
- **Total**: 277 tests (up from 123)

**Coverage**:
- Overall: 88.9% (up from 83%)
- Target: 70% (exceeded by 18.9 percentage points)
- Business logic: 90%+ (on track)

### Migration

**No migration required.** All v0.1.2-beta code works without changes.

**Optional adoption**:
- Gradually replace N+1 queries with JOINs
- Replace in-memory sorting with ORDER BY
- Replace fetch-all-then-count with aggregates
- Mix old and new approaches in same codebase

### Known Limitations

- **FULL OUTER JOIN** not supported on MySQL (database limitation, not Relica)
  - Workaround: Use UNION of LEFT JOIN and RIGHT JOIN
  - Supported on PostgreSQL and SQLite
- **Subqueries** not yet supported (planned for v0.3.0)
  - Use `relica.NewExp("(SELECT ...)")` as workaround
- **Window functions** not yet supported (planned for v0.3.0)

### Technical Details

**Implementation**:
- JOIN infrastructure: `JoinInfo` struct with type, table, ON condition
- Table alias parsing: `"users u"` → `"users" AS "u"`
- ORDER BY: `quoteColumnName()` helper for table.column quoting
- Aggregate detection: Parentheses `(` in column name → use as-is
- HAVING: Reuses `whereClause` struct, same signature as WHERE
- PostgreSQL placeholder renumbering for HAVING ($N after WHERE params)

**Code Quality**:
- Zero production dependencies maintained
- Comprehensive godoc comments
- Table-driven tests with subtests
- Full integration test suite across all databases
- golangci-lint passing (0 warnings)

### Contributors

- COREGX Team
- IrisMX Project (feature request and beta testing)
- Community contributors

---

## [0.1.2-beta] - 2025-10-24

### Added

#### Expression API for Fluent WHERE Clauses

Type-safe, composable expression builders for complex database queries.

**HashExp - Map-based Conditions**
```go
Where(relica.HashExp{
    "status":     []interface{}{1, 2, 3},  // Automatic IN clause
    "deleted_at": nil,                      // Automatic IS NULL
    "role":       "admin",                  // Simple equality
})
// Generates: WHERE "deleted_at" IS NULL AND "role" = $1 AND "status" IN ($2, $3, $4)
```

**Comparison Operators**
- `Eq(col, val)` - Equality with automatic NULL → IS NULL conversion
- `NotEq(col, val)` - Inequality with automatic NULL → IS NOT NULL conversion
- `GreaterThan(col, val)`, `LessThan(col, val)` - Comparison operators
- `GreaterOrEqual(col, val)`, `LessOrEqual(col, val)` - Inclusive comparisons

**IN and BETWEEN Expressions**
- `In(col, vals...)` - IN clause with variadic arguments
  - Single value optimization: `IN (val)` → `= val`
  - Empty values: `IN ()` → `0=1` (always false)
- `NotIn(col, vals...)` - NOT IN clause
  - Single value optimization: `NOT IN (val)` → `<> val`
  - Empty values: `NOT IN ()` → empty (always true)
- `Between(col, from, to)` - Range queries
- `NotBetween(col, from, to)` - Exclusive range

**LIKE Expressions with Automatic Escaping**
- `Like(col, vals...)` - Pattern matching with automatic wildcard injection
  - Default: `%value%` (partial match)
  - Special character escaping: `%`, `\`, `_`
  - Multiple values combined with AND
- `NotLike(col, vals...)` - Negative pattern matching
- `OrLike(col, vals...)` - Multiple values combined with OR
- `OrNotLike(col, vals...)` - Negative pattern matching with OR
- `Match(left, right)` - Custom wildcard placement
  - `Match(true, false)` → `%value` (suffix matching)
  - `Match(false, true)` → `value%` (prefix matching)
  - `Match(false, false)` → `value` (exact match)

**Logical Combinators**
- `And(exps...)` - Combine expressions with AND
  - Automatic nil expression filtering
  - Proper parentheses for precedence
- `Or(exps...)` - Combine expressions with OR
- `Not(exp)` - Negate expression

**Raw Expressions**
- `NewExp(sql, args...)` - Custom SQL fragments for unsupported cases
  - Subqueries
  - Database-specific functions
  - Complex expressions

### Changed

**WHERE Method Signature**
- Updated `Where()` to accept `interface{}` (string or Expression)
- ✅ **Backward compatible** - string-based WHERE still works
- ✅ **Can mix both styles** in same query
- Applied to: `SelectQuery`, `UpdateQuery`, `DeleteQuery`

### Technical Details

**Performance**
- Zero allocations in Expression.Build() hot paths
- Deterministic SQL generation (sorted map keys in HashExp)
- Same performance as string-based WHERE (<1% overhead)
- Statement cache hit rate improved with deterministic SQL

**Multi-Dialect Support**
- PostgreSQL: `$1, $2, $3` placeholders, `"` identifier quoting
- MySQL: `?, ?, ?` placeholders, `` ` `` identifier quoting
- SQLite: `?, ?, ?` placeholders, `"` identifier quoting
- Consistent Expression API across all databases

**Test Coverage**
- 87.4% overall coverage (up from 83%)
- 80+ unit tests for expressions
- 20+ integration tests across all databases
- 100% coverage for expression types

**Code Quality**
- Comprehensive godoc comments for all expressions
- Example-driven documentation
- Table-driven tests with subtests
- Full integration test suite

### Documentation

**Updated Documentation**
- README.md - Added Expression API section with quick reference
- CHANGELOG.md - Detailed v0.1.1 release notes

### Security

**Automatic SQL Injection Protection**
- LIKE expressions escape special characters (`%`, `_`, `\`)
- NULL values safely handled (no direct comparison)
- Parameterized queries with prepared statements
- Input validation for expression types

### Migration

**No breaking changes.** All v0.1.0 code works without modifications.

**Optional adoption:**
- Gradually migrate to Expression API for type safety
- Mix string-based and expression-based WHERE in same codebase

### Known Limitations

- Expression API doesn't support JOINs (planned for v0.2.0)
- Subqueries require `NewExp()` (native support planned)
- Column references must be string literals (no variable column names for cache optimization)

---

## [0.1.0-beta] - 2025-10-24

### Added

#### Core Features
- **CRUD Operations**: Complete SELECT, INSERT, UPDATE, DELETE, UPSERT support
- **Type-Safe Scanning**: Reflection-based struct scanning with metadata caching
- **Query Builder**: Fluent API with method chaining for query construction
- **Multi-Database Support**: PostgreSQL, MySQL, SQLite dialects with proper placeholder handling

#### SELECT Operations
- `One()` - Query single row with type-safe scanning
- `All()` - Query multiple rows into slice
- Struct tag support (`db:"column_name"`)
- Nested struct scanning
- NULL value handling

#### INSERT Operations
- Simple INSERT with map values
- Batch INSERT with 3.3x performance improvement
- Multi-row INSERT support
- LastInsertId and RowsAffected support

#### UPSERT Operations
- PostgreSQL: `ON CONFLICT ... DO UPDATE`
- MySQL: `ON DUPLICATE KEY UPDATE`
- SQLite: `ON CONFLICT ... DO UPDATE`
- Conflict column specification
- Selective column updates
- DO NOTHING support

#### UPDATE Operations
- Single row UPDATE with WHERE conditions
- Batch UPDATE for multiple rows with different values
- Key-based batch updates
- 2.5x performance improvement for batch operations

#### DELETE Operations
- Simple DELETE with WHERE conditions
- Multiple condition support
- Safe deletion with explicit WHERE requirement

#### Transaction Support
- Full ACID transaction support
- All isolation levels: ReadUncommitted, ReadCommitted, RepeatableRead, Serializable
- Context propagation in transactions
- Automatic rollback on error
- Nested transaction detection

#### Performance Features
- **LRU Statement Cache**: Configurable capacity (default 1000)
- Cache hit latency: <60ns
- Thread-safe concurrent access
- Cache metrics: hit rate, evictions, size
- Zero allocations in hot paths

#### Context Support
- Context propagation throughout query chain
- Timeout support
- Cancellation support
- Transaction context auto-propagation
- `WithContext()` at any builder stage

#### Batch Operations
- **BatchInsert**: Multi-row INSERT, 3.3x faster than individual inserts
- **BatchUpdate**: Update multiple rows with different values, 2.5x faster
- Memory-efficient batch processing
- 55% fewer allocations

#### Documentation & Governance
- **CONTRIBUTING.md**: Comprehensive contribution guidelines with pre-commit checklist
- **CODE_OF_CONDUCT.md**: Community standards (Contributor Covenant v2.1)
- **SECURITY.md**: Security policy and vulnerability reporting process
- **RELEASE_GUIDE.md**: Step-by-step release process documentation

#### Project Structure
- **Internal packages**: All implementation moved to `internal/` for clear API boundary
  - `internal/core/` - Query building logic
  - `internal/cache/` - Statement caching
  - `internal/dialects/` - SQL dialect implementations
  - `internal/util/` - Utility functions
- **Public API**: Only `github.com/coregx/relica` package is exposed
- **Pre-release automation**: Validation scripts for quality checks (`scripts/pre-release.sh`)

### Changed
- Replaced unbounded `sync.Map` with LRU cache to prevent memory leaks
- Improved error messages with context information
- Enhanced SQL generation for better readability
- Optimized struct metadata caching

### Removed
- OpenTelemetry dependency (replaced with NoOpTracer interface)
- Moved testcontainers to separate test module
- Zero production dependencies achieved

### Fixed
- **TxOptions export**: Fixed missing `TxOptions` type export in public API (was referenced in README but not accessible)
- Memory leak in statement cache
- Race conditions in concurrent cache access
- Context cancellation edge cases
- Transaction rollback safety

### Performance
- 3.3x faster batch INSERT (100 rows: 327ms vs 1094ms)
- 2.5x faster batch UPDATE (100 rows: 1370ms vs baseline)
- Sub-60ns statement cache hits
- 15% memory reduction in batch operations
- 55% fewer allocations in batch updates

### Documentation
- Comprehensive README.md with Quick Start guide
- Transaction implementation report
- UPSERT examples for all databases
- Batch operations guide
- API documentation with examples
- Performance benchmarks

### Testing
- 123+ unit tests
- Integration tests for all databases
- Benchmark suite
- 47.8% code coverage
- Separate test module for integration tests

### Security
- SQL injection protection via prepared statements
- Safe identifier quoting
- Input validation and sanitization
- Transaction isolation guarantees

## Architecture

### Zero Dependencies
- Production code uses only Go standard library
- Test dependencies isolated in separate module
- Pure Go SQLite driver for tests
- No external runtime dependencies

### Database Dialects
| Database | Placeholders | Identifiers | UPSERT Support |
|----------|-------------|-------------|----------------|
| PostgreSQL | $1, $2, $3 | "users" | ON CONFLICT |
| MySQL | ?, ?, ? | \`users\` | ON DUPLICATE KEY |
| SQLite | ?, ?, ? | "users" | ON CONFLICT |

### Statement Caching
- Default capacity: 1000 prepared statements
- LRU eviction policy
- Thread-safe with RWMutex
- Performance metrics tracking
- Configurable capacity

## Requirements

- Go 1.25+
- Database drivers (runtime, not production dependency):
  - PostgreSQL: `github.com/lib/pq`
  - MySQL: `github.com/go-sql-driver/mysql`
  - SQLite: `modernc.org/sqlite` (pure Go)

## Migration Notes

This is the initial beta release. No migration required.

## Known Limitations

- Complex JOIN queries not yet supported (planned for v0.2.0)
- Query builder doesn't support subqueries (planned for v0.2.0)
- Raw SQL execution requires direct `db.DB()` access
- Limited aggregate function support

## Contributors

- COREGX Team
- Community contributors

---

[0.1.0-beta]: https://github.com/coregx/relica/releases/tag/v0.1.0-beta
