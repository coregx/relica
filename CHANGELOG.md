# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
