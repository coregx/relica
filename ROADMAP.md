# Relica Roadmap

> **Current Version**: v0.8.0 (Released: December 16, 2025)
> **Previous Release**: v0.7.0 (Released: November 24, 2025)
> **Production Ready**: v1.0.0 (Target: Q3-Q4 2026)

---

## 🎯 Vision

**Relica** aims to be the **best query builder for Go** - lightweight, fast, and type-safe, while maintaining zero production dependencies.

**Philosophy**: *"If you want magic, use GORM. If you want control, use Relica."*

---

## 📍 Current State (v0.8.0 - ozzo-dbx API Parity)

### ✅ Completed Features

- **CRUD Operations**: SELECT, INSERT, UPDATE, DELETE, UPSERT
- **Struct Operations**: InsertStruct, BatchInsertStruct, UpdateStruct, Model() API
- **Model() API Parity** (v0.7.0): Auto-populate ID, selective fields Insert/Update
- **Named Placeholders** (v0.8.0): `{:name}` syntax with `Bind(Params{})`
- **Quoting Syntax** (v0.8.0): `{{table}}` and `[[column]]` for dialect-aware quoting
- **Row() / Column()** (v0.8.0): Scalar and single-column query helpers
- **Transactional()** (v0.8.0): Auto commit/rollback with panic recovery
- **Distinct()** (v0.8.0): SELECT DISTINCT support
- **AndWhere() / OrWhere()** (v0.8.0): Dynamic WHERE clause building
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
- **Enterprise Security** (v0.5.0): SQL injection prevention, audit logging, compliance
- **Query Optimizer** (v0.5.0): 4-phase optimization with database-specific hints
- **Query Analyzer** (v0.5.0): EXPLAIN integration for PostgreSQL, MySQL, SQLite
- **SQL Logging & Tracing** (v0.5.0): Structured logging (slog), OpenTelemetry support

### 📊 Metrics

- **Test Coverage**: 85%+ (600+ tests)
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

### v0.9.0 (Q1 2026)

**Goal**: API stabilization & advanced features

**Planned Features**:
- NullStringMap for dynamic scanning (TASK-016)
- Query.Prepare() / Query.Close() manual control (TASK-017)
- Composite primary key support
- Advanced query helpers (CASE, COALESCE)
- Error message improvements

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

| Feature | v0.8.0 | v1.0.0 | GORM | sqlc |
|---------|--------|--------|------|------|
| **CRUD** | ✅ | ✅ | ✅ | ✅ |
| **Expression API** | ✅ | ✅ | ✅ | ❌ |
| **JOIN** | ✅ | ✅ | ✅ | ✅ |
| **Aggregates** | ✅ | ✅ | ✅ | ✅ |
| **Subqueries** | ✅ | ✅ | ✅ | ✅ |
| **Window Functions** | ✅ | ✅ | ✅ | ✅ |
| **Model API** | ✅ | ✅ | ✅ | ❌ |
| **Named Placeholders** | ✅ | ✅ | ❌ | ❌ |
| **Transactional Helper** | ✅ | ✅ | ✅ | ❌ |
| **Relations** | ❌ | ❌ | ✅ | ❌ |
| **Zero Dependencies** | ✅ | ✅ | ❌ | ❌ |
| **Type Safety** | ✅ | ✅ | Partial | ✅✅ |
| **Dynamic Queries** | ✅ | ✅ | ✅ | ❌ |

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

| Metric | v0.8.0 Actual | v1.0.0 Target |
|--------|---------------|---------------|
| **Statement Cache Hit** | <60ns ✅ | <50ns |
| **Batch INSERT (100 rows)** | 327ms ✅ | <200ms |
| **N+1 Query Reduction** | 3-18x ✅ | Maintained |
| **Pagination Memory** | 100x reduction ✅ | Maintained |
| **Aggregate Memory** | 2,500,000x reduction ✅ | Maintained |
| **EXISTS vs IN** | 5x faster ✅ | Maintained |
| **UNION ALL vs UNION** | 2-3x faster ✅ | Maintained |
| **Test Coverage** | 85%+ ✅ | >90% |
| **Dependencies** | 0 ✅ | 0 |

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
- **v0.7.0** (2025-11-24) - Model() API parity (auto-populate ID, selective fields)
- **v0.8.0** (2025-12-16) - Named placeholders, quoting, Row/Column, Transactional, Distinct, AndWhere/OrWhere
- **v1.0.0** (Target: Q3-Q4 2026) - Production stable release

---

## 🙏 Acknowledgments

- Inspired by [ozzo-dbx](https://github.com/go-ozzo/ozzo-dbx)
- Community feedback and contributions
- **IrisMX** for real-world validation
- **Professor Ancha Baranova** for invaluable support

---

*Last Updated: 2025-12-16*
*Maintained by: COREGX Team*
