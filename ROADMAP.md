# Relica Roadmap

> **Current Version**: v0.2.0-beta (Ready for Release)
> **Next Release**: v0.3.0-beta (Target: Q4 2025)
> **Production Ready**: v1.0.0 (Target: Q1 2026)

---

## 🎯 Vision

**Relica** aims to be the **best query builder for Go** - lightweight, fast, and type-safe, while maintaining zero production dependencies.

**Philosophy**: *"If you want magic, use GORM. If you want control, use Relica."*

---

## 📍 Current State (v0.2.0-beta)

### ✅ Completed Features

- **CRUD Operations**: SELECT, INSERT, UPDATE, DELETE, UPSERT
- **Type-Safe Scanning**: Struct tags, reflection-based
- **Transactions**: All isolation levels, context support
- **Batch Operations**: 3.3x faster INSERT, 2.5x faster UPDATE
- **Expression API**: Fluent WHERE clause builder (HashExp, comparison operators, LIKE, IN, BETWEEN, logical combinators)
- **JOIN Operations**: INNER, LEFT, RIGHT, FULL, CROSS with hybrid API (string, Expression, nil)
- **Sorting & Pagination**: ORDER BY, LIMIT, OFFSET
- **Aggregate Functions**: COUNT, SUM, AVG, MIN, MAX with GROUP BY, HAVING
- **Multi-Database**: PostgreSQL, MySQL, SQLite
- **Performance**: LRU statement cache (<60ns hit), zero allocations in hot paths
- **Zero Dependencies**: Production code uses only Go standard library

### 📊 Metrics

- **Test Coverage**: 88.9% (277 tests)
- **Dependencies**: 0 (production), 2 (tests only)
- **Performance**:
  - Batch operations: 3.3x faster INSERT, 2.5x UPDATE
  - N+1 queries: 3-18x faster (SQLite 6.6x, PostgreSQL 18x, MySQL 3x)
  - Memory: 100x reduction with LIMIT, 2,500,000x with COUNT
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

### v0.3.0-beta (Q4 2025)

**Goal**: Advanced query building

**Features**:
- 🔍 **Subqueries** (IN (SELECT ...), EXISTS, NOT EXISTS)
- 🔀 **UNION, INTERSECT, EXCEPT**
- 📐 **Window Functions** (OVER, PARTITION BY)
- 📝 **Common Table Expressions (WITH)**
- 🔄 **Query Hooks** (logging, metrics)

**Timeline**: 4-6 weeks
**Dependencies**: v0.2.0-beta

---

### v0.4.0-beta (Q1 2026)

**Goal**: Production hardening & performance

**Features**:
- ⚡ **Query Optimizer** (auto-index hints)
- 📊 **Query Analyzer** (EXPLAIN integration)
- 🔍 **Query Debugging** (SQL logging, tracing)
- 🚀 **Performance Tuning** (connection pooling, prepare caching)
- 🛡️ **Security Hardening** (SQL injection prevention, audit logging)

**Timeline**: 6-8 weeks
**Dependencies**: v0.3.0-beta

---

### v1.0.0 (Q1 2026)

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

| Metric | v0.1.2 | v0.2.0 Actual | v1.0.0 Target |
|--------|--------|---------------|---------------|
| **Statement Cache Hit** | <60ns | <60ns ✅ | <50ns |
| **Batch INSERT (100 rows)** | 327ms | 327ms ✅ | <200ms |
| **N+1 Query Reduction** | N/A | 3-18x ✅ | Maintained |
| **Pagination Memory** | N/A | 100x reduction ✅ | Maintained |
| **Aggregate Memory** | N/A | 2,500,000x reduction ✅ | Maintained |
| **Test Coverage** | 87.4% | 88.9% ✅ | >90% |
| **Dependencies** | 0 | 0 ✅ | 0 |

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

- **v0.1.0-beta** (2025-01-24) - Initial release (CRUD, transactions, batch)
- **v0.1.2-beta** (2025-01-24) - Expression API (type-safe WHERE clauses)
- **v0.2.0-beta** (2025-02-03) - JOIN, ORDER BY, Aggregates (production-ready query builder)
- **v0.3.0-beta** (Target: Q4 2025) - Subqueries, UNION, window functions
- **v1.0.0** (Target: Q1 2026) - Production stable release

---

## 🙏 Acknowledgments

- Inspired by [ozzo-dbx](https://github.com/go-ozzo/ozzo-dbx)
- Community feedback and contributions
- **IrisMX** for real-world validation
- **Professor Ancha Baranova** for invaluable support

---

*Last Updated: 2025-02-03*
*Maintained by: COREGX Team*
