# Relica Roadmap

> **Current Version**: v0.6.0 (Released: November 24, 2025)
> **Previous Release**: v0.5.0 (Released: November 14, 2025)
> **Production Ready**: v1.0.0 (Target: Q3-Q4 2026)

---

## 🎯 Vision

**Relica** aims to be the **best query builder for Go** - lightweight, fast, and type-safe, while maintaining zero production dependencies.

**Philosophy**: *"If you want magic, use GORM. If you want control, use Relica."*

---

## 📍 Current State (v0.6.0 - Struct Operations)

### ✅ Completed Features

- **CRUD Operations**: SELECT, INSERT, UPDATE, DELETE, UPSERT
- **Struct Operations**: InsertStruct, BatchInsertStruct, UpdateStruct, Model() API
- **Type-Safe Scanning**: Struct tags, reflection-based
- **Transactions**: All isolation levels, context support
- **Batch Operations**: 3.3x faster INSERT, 2.5x faster UPDATE
- **Expression API**: Fluent WHERE clause builder (HashExp, comparison operators, LIKE, IN, BETWEEN, logical combinators)
- **JOIN Operations**: INNER, LEFT, RIGHT, FULL, CROSS with hybrid API (string, Expression, nil)
- **Sorting & Pagination**: ORDER BY, LIMIT, OFFSET
- **Aggregate Functions**: COUNT, SUM, AVG, MIN, MAX with GROUP BY, HAVING
- **Subqueries**: IN (SELECT...), EXISTS/NOT EXISTS, FROM subqueries, scalar subqueries
- **Set Operations**: UNION, UNION ALL, INTERSECT, EXCEPT
- **Common Table Expressions**: WITH clause, WITH RECURSIVE for hierarchical data
- **Connection Management**: WrapDB() for integrating with external connection pools
- **Multi-Database**: PostgreSQL, MySQL 8.0+, SQLite 3.25+ support
- **Performance**: LRU statement cache (<60ns hit), zero allocations in hot paths
- **Zero Dependencies**: Production code uses only Go standard library
- **🛡️ Enterprise Security** (v0.5.0): SQL injection prevention, audit logging, compliance (GDPR, HIPAA, PCI-DSS, SOC2)
- **🎯 Query Optimizer** (v0.5.0): 4-phase optimization with database-specific hints
- **📊 Query Analyzer** (v0.5.0): EXPLAIN integration for PostgreSQL, MySQL, SQLite
- **📝 SQL Logging & Tracing** (v0.5.0): Structured logging (slog), OpenTelemetry support
- **⚡ Performance Monitoring** (v0.5.0): Health checks, cache warming, query pinning
- **📚 Professional Documentation** (v0.5.0): 10,000+ lines, migration guides, user guides

### 📊 Metrics

- **Test Coverage**: 93.3% (326+ tests) - improved from 92.9%
- **Dependencies**: 0 (production), 2 (tests only)
- **Performance**:
  - Batch operations: 3.3x faster INSERT, 2.5x UPDATE
  - N+1 queries: 3-18x faster (SQLite 6.6x, PostgreSQL 18x, MySQL 3x)
  - Memory: 100x reduction with LIMIT, 2,500,000x with COUNT
  - Subqueries: EXISTS 5x faster than IN (109ns vs 516ns)
  - Set operations: UNION ALL 2-3x faster than UNION
  - Wrapper calls: Zero overhead (0ns)
- **Go Version**: 1.25+

---

## 🚀 Upcoming Releases

### v0.2.0-beta ✅ (Ready for Release)

**Driver**: IrisMX feature request (first production user - 10K+ concurrent users)

**Goal**: Transform from basic CRUD to production-ready query builder

**Features Implemented**:
- ✅ **JOIN Operations** (INNER, LEFT, RIGHT, FULL, CROSS)
  - Hybrid API: string, Expression, or nil ON conditions
  - Table aliases support
  - **Real Performance**: SQLite 6.6x, PostgreSQL 18x, MySQL 3x faster (N+1 → single query)
- ✅ **ORDER BY, LIMIT, OFFSET**
  - **Real Performance**: 100x memory reduction (20MB → 200KB)
- ✅ **Aggregate Functions** (COUNT, SUM, AVG, MIN, MAX)
  - **Real Performance**: 2,500,000x memory reduction (20MB → 8 bytes)
- ✅ **GROUP BY, HAVING**

**Status**: ✅ Implementation Complete (88.9% coverage, 277 tests, all checks passed)
**Implementation Time**: 10 days (as planned)
**Documentation**: [JOIN_GUIDE.md](docs/dev/reports/JOIN_GUIDE.md), [AGGREGATES_GUIDE.md](docs/dev/reports/AGGREGATES_GUIDE.md)

---

### v0.3.0-beta ✅ (Ready for Release)

**Goal**: Advanced SQL query building

**Features Implemented**:
- ✅ **Subqueries** (IN (SELECT ...), EXISTS/NOT EXISTS, FROM subqueries, scalar subqueries)
  - **Real Performance**: EXISTS 5x faster than IN (109ns vs 516ns)
  - **Zero allocations** in hot paths
- ✅ **Set Operations** (UNION, UNION ALL, INTERSECT, EXCEPT)
  - **Real Performance**: UNION ALL 2-3x faster than UNION
  - Full database compatibility (PostgreSQL, MySQL 8.0.31+, SQLite)
- ✅ **Common Table Expressions** (WITH clause, WITH RECURSIVE)
  - Recursive CTEs for hierarchical data (org charts, trees, BOM)
  - Multiple CTEs with dependency resolution
- ✅ **Window Functions** (documentation only - use SelectExpr())
  - Support via raw SQL expressions
  - Full guide with examples

**Status**: ✅ Implementation Complete (89.5% coverage, 310+ tests, all checks passed)
**Implementation Time**: 6 weeks (as planned)
**Documentation**: [SUBQUERY_GUIDE.md](docs/SUBQUERY_GUIDE.md), [SET_OPERATIONS_GUIDE.md](docs/SET_OPERATIONS_GUIDE.md), [CTE_GUIDE.md](docs/CTE_GUIDE.md), [WINDOW_FUNCTIONS_GUIDE.md](docs/WINDOW_FUNCTIONS_GUIDE.md)

---

### v0.4.0-beta ✅ (Released: October 26, 2025)

**Goal**: Better documentation & API stability

**Features Implemented**:
- ✅ **Wrapper Types Migration** (breaking change - acceptable in beta)
  - Replaced type aliases with wrapper types
  - All 81 methods wrapped with comprehensive godoc
  - Zero performance overhead
  - **Result**: pkg.go.dev now shows all methods with examples
  - **Impact**: 95% of user code requires ZERO changes
- ✅ **Unwrap() Methods** - Access internal types when needed
- ✅ **MIGRATION_GUIDE.md** - Detailed v0.3.0 → v0.4.0 migration guide
- ✅ **Critical Bug Fix**: SELECT "*" quoting issue resolved

**Status**: ✅ Released (92.9% coverage, all tests passing, 0 linting issues)
**Implementation Time**: 1 week
**Documentation**: [MIGRATION_GUIDE.md](docs/MIGRATION_GUIDE.md)

---

### v0.5.0 ✅ (Released: November 14, 2025)

**Goal**: Enterprise-ready database query builder

**Completed Features**:
- ✅ **🛡️ Enterprise Security**: SQL injection prevention (OWASP Top 10), audit logging, compliance support
- ✅ **🎯 Query Optimizer**: 4-phase optimization with database-specific hints
- ✅ **📊 Query Analyzer**: EXPLAIN integration for PostgreSQL, MySQL, SQLite
- ✅ **📝 SQL Logging & Tracing**: Structured logging (slog), OpenTelemetry support
- ✅ **⚡ Performance Monitoring**: Health checks, cache warming, query pinning
- ✅ **📚 Professional Documentation**: 10,000+ lines, migration guides (GORM, sqlx), 6 user guides
- ✅ **CI/CD Enhancements**: Codecov integration, release/hotfix branch support

**Achievements**:
- **72 files changed**, 19,809+ lines added
- **32 commits** across 5 major features (TASK-001 to TASK-005)
- **326+ tests**, 93.3% coverage
- **Zero production dependencies**
- **Professional Git Flow** release process

**Timeline**: Completed in 3 weeks (November 2025)
**Status**: ✅ Production Ready

---

### v0.6.0 ✅ (Released: November 24, 2025)

**Goal**: Type-safe struct operations for improved developer experience

**Completed Features**:
- ✅ **InsertStruct/UpdateStruct** (Phase 1):
  - `InsertStruct(table, &struct)` - eliminate manual map construction
  - `BatchInsertStruct(table, []struct)` - batch operations with structs
  - `UpdateStruct(table, &struct)` - struct-based updates
  - Uses existing scanner infrastructure (zero duplication)

- ✅ **Model() API** (Phase 2):
  - `Model(&struct).Insert()` - elegant CRUD operations
  - `Model(&struct).Update()` - auto WHERE by primary key
  - `Model(&struct).Delete()` - auto WHERE by primary key
  - `Exclude(fields...)` - field control
  - `Table(name)` - table override
  - `TableName()` interface support
  - Inspired by ozzo-dbx (our reference implementation)

**Achievements**:
- **Type safety**: Compile-time struct validation
- **Less boilerplate**: No manual map construction
- **DX improvement**: Requested by real production user (PubSub-Go)
- **Backward compatible**: All new methods, zero breaking changes
- **48 tests** (27 integration + 21 unit), **86% coverage**
- **Zero breaking changes**

**Implementation Time**: 8.5 hours (within 8-11 hour estimate)
**Status**: ✅ Implementation Complete
**Tasks**: TASK-006 (Phase 1), TASK-007 (Phase 2) - both completed

---

### v0.7.0 - v0.9.0 (Q1-Q3 2026)

**Goal**: API stabilization & community feedback

**Planned Features**:
- Named queries (like sqlx NamedExec)
- Auto-increment primary key population
- Composite primary key support
- Advanced query helpers (CASE, COALESCE)
- Error message improvements
- Read replica support (optional)
- Performance optimizations based on feedback

**Timeline**: 9 months
**Focus**: Real-world production validation

---

### v1.0.0 (Q3-Q4 2026)

**Goal**: Production-ready stable release

**Criteria**:
- ✅ API freeze (no breaking changes after v1.0.0)
- ✅ Test coverage > 90%
- ✅ Performance benchmarks validated
- ✅ Production use by 5+ companies
- ✅ Full documentation
- ✅ Security audit complete
- ✅ LTS support commitment

**Timeline**: 2-4 weeks stabilization
**Dependencies**: v0.4.0-beta + community validation

---

## 🚫 Out of Scope (ORM Features - NEVER)

Relica is a **query builder**, NOT an ORM. We will **NEVER** add:

- ❌ Relations (HasMany, BelongsTo, ManyToMany)
- ❌ Eager loading (Preload, With)
- ❌ Model associations
- ❌ Hooks/Callbacks (use middleware instead)
- ❌ Active Record pattern
- ❌ Schema migrations (use separate tool)
- ❌ Automatic JOIN generation from models

**Why?** These features add complexity, magic, and implicit behavior. If you need them, use [GORM](https://gorm.io/).

**Our motto**: *"Explicit is better than implicit. Control is better than magic."*

---

## 📊 Feature Comparison

| Feature | v0.1.2 | v0.2.0 | v0.3.0 | v1.0.0 | GORM | sqlc |
|---------|--------|--------|--------|--------|------|------|
| **CRUD** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Expression API** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| **JOIN** | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Aggregates** | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Subqueries** | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ |
| **Window Functions** | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ |
| **Relations** | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| **Zero Dependencies** | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| **Type Safety** | ✅ | ✅ | ✅ | ✅ | Partial | ✅✅ |
| **Dynamic Queries** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |

---

## 🎓 Design Principles

1. **Zero Dependencies** - Production code uses only Go standard library
2. **Type Safety** - Compile-time checks where possible, runtime safety always
3. **Performance First** - Statement caching, batch operations, zero allocations
4. **Simplicity** - Clean API, easy to learn, hard to misuse
5. **Correctness** - ACID transactions, proper error handling
6. **Explicit Over Implicit** - No magic, clear control flow
7. **Query Builder NOT ORM** - Clear boundaries, no feature creep

---

## 📈 Performance Goals

| Metric | v0.1.2 | v0.2.0 Actual | v0.3.0 Actual | v1.0.0 Target |
|--------|--------|---------------|---------------|---------------|
| **Statement Cache Hit** | <60ns | <60ns ✅ | <60ns ✅ | <50ns |
| **Batch INSERT (100 rows)** | 327ms | 327ms ✅ | 327ms ✅ | <200ms |
| **N+1 Query Reduction** | N/A | 3-18x ✅ | 3-18x ✅ | Maintained |
| **Pagination Memory** | N/A | 100x reduction ✅ | 100x ✅ | Maintained |
| **Aggregate Memory** | N/A | 2,500,000x reduction ✅ | 2,500,000x ✅ | Maintained |
| **EXISTS vs IN** | N/A | N/A | 5x faster ✅ | Maintained |
| **UNION ALL vs UNION** | N/A | N/A | 2-3x faster ✅ | Maintained |
| **Test Coverage** | 87.4% | 88.9% ✅ | 89.5% ✅ | >90% |
| **Dependencies** | 0 | 0 ✅ | 0 ✅ | 0 |

---

## 🤝 Community & Feedback

### Current Users

- **IrisMX** (production) - Mail server, 10K+ concurrent users
- Community contributors via GitHub

### How to Influence Roadmap

1. **Feature Requests** - Open GitHub issue with use case
2. **Bug Reports** - Detailed reproduction steps
3. **Performance Reports** - Benchmark results, profiling
4. **Pull Requests** - Follow [CONTRIBUTING.md](CONTRIBUTING.md)

**Note**: Features must align with Query Builder philosophy. ORM features will be declined.

---

## 📞 Support & Resources

- **GitHub**: [coregx/relica](https://github.com/coregx/relica)
- **Documentation**: [pkg.go.dev](https://pkg.go.dev/github.com/coregx/relica)
- **Issues**: [GitHub Issues](https://github.com/coregx/relica/issues)
- **Discussions**: [GitHub Discussions](https://github.com/coregx/relica/discussions)
- **Email**: support@coregx.dev

---

## 📝 Release History

- **v0.1.0-beta** (2025-10-24) - Initial release (CRUD, transactions, batch)
- **v0.1.2-beta** (2025-10-24) - Expression API (type-safe WHERE clauses)
- **v0.2.0-beta** (2025-10-24) - JOIN, ORDER BY, Aggregates (production-ready query builder)
- **v0.3.0-beta** (2025-10-25) - Subqueries, Set Operations, CTEs, WrapDB() (advanced SQL features)
- **v0.4.0-beta** (2025-10-26) - Wrapper types migration, better documentation, API stability
- **v0.4.1-beta** (2025-10-26) - Convenience methods (Select, Insert, Update, Delete)
- **v0.5.0** (2025-11-14) - Enterprise security, query optimizer, comprehensive documentation
- **v0.6.0** (2025-11-24) - Struct operations (InsertStruct, Model API)
- **v1.0.0** (Target: Q3-Q4 2026) - Production stable release

---

## 🙏 Acknowledgments

- Inspired by [ozzo-dbx](https://github.com/go-ozzo/ozzo-dbx)
- Community feedback and contributions
- **IrisMX** for real-world validation
- **Professor Ancha Baranova** for invaluable support

---

*Last Updated: 2025-11-24*
*Maintained by: COREGX Team*
