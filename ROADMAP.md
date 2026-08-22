# Relica Roadmap

> **Current Version**: v0.16.0 (August 2026)
> **Previous Release**: v0.15.0 (Released: August 5, 2026)
> **Production Ready**: v1.0.0 (Target: Q4 2026)

---

## 🎯 Vision

**Relica** aims to be the **best query builder for Go** - lightweight, fast, and type-safe, while maintaining zero production dependencies.

**Philosophy**: *"If you want magic, use GORM. If you want control, use Relica."*

---

## 📍 Current State (v0.16.0)

### ✅ What's New in v0.16.0

- **AutoID — Native Dual-Key Pattern** (v0.16.0): Stripe-like prefixed IDs via `autoid:prefix` struct tag, `FindByPublicID()`, `BeforeInserter` interface, pluggable generators
- **Truly Zero Dependencies** (v0.16.0): Empty go.mod — no require blocks. All test deps isolated in sub-modules. Tests use stdlib `testing` only
- **Explain() / ExplainAnalyze() Exported** (v0.16.0): Query plan analysis via public API
- **Query.ToSQL()** (v0.16.0): Consistent SQL preview on ALL query types including Insert
- **Generic API** (v0.15.0): `One[T]`, `All[T]`, `Scalar[T]` free functions
- **UUID PK Auto-Populate** (v0.15.0): `autoincrement` tag for server-generated string PKs via RETURNING

### 📊 Metrics

- **Test Coverage**: 85%+
- **Dependencies**: 0 (truly zero — empty go.mod)
- **Lint**: 0 issues (golangci-lint)
- **Go Version**: 1.25+

---

## 🚀 Upcoming Releases

### v0.17.0 — Go 1.27 + Generic Methods

**Goal**: Leverage Go 1.27 generic methods for fluent type-safe API.

**Planned**:
- **Go 1.27 minimum** — bump from Go 1.25
- **Generic methods on SelectQuery** — fluent API instead of free functions
- **stdlib uuid** — replace internal UUID v7 generator with Go 1.27 stdlib `uuid.NewV7()`

**In progress**: Separate research on method naming and API consistency (ADR pending). Current naming (`One`/`All`/`Scalar`, free functions vs methods, number of API styles) under review to ensure enterprise-grade consistency before API freeze.

### v1.0.0 (Q4 2026) — Stability Release

**Goal**: Production-ready stable release with long-term compatibility guarantee.

**Criteria**:
- API freeze (no breaking changes after v1.0.0)
- Test coverage > 90%
- Performance benchmarks validated and published
- Full documentation with migration guides
- Security audit complete
- 5-year backward compatibility commitment

**What v1.0.0 means for users**:
- Safe to use in production without fear of breaking upgrades
- `go get -u` will never break your code
- Deprecation-first policy: old methods stay for 2+ years before removal

**Prerequisites**: v0.13.0 benchmarks + community feedback cycle

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

| Feature | v0.12.0 | v1.0.0 | GORM | sqlc |
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

| Metric | v0.12.0 Actual | v1.0.0 Target |
|--------|----------------|---------------|
| **Statement Cache Hit** | <60ns ✅ | <50ns |
| **Batch INSERT (100 rows)** | 327ms ✅ | <200ms |
| **N+1 Query Reduction** | 3-18x ✅ | Maintained |
| **Pagination Memory** | 100x reduction ✅ | Maintained |
| **Aggregate Memory** | 2,500,000x reduction ✅ | Maintained |
| **EXISTS vs IN** | 5x faster ✅ | Maintained |
| **UNION ALL vs UNION** | 2-3x faster ✅ | Maintained |
| **Test Coverage** | 88%+ ✅ | >90% |
| **Lint Issues** | 0 ✅ | 0 |
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
- **v0.9.0** (2025-12-16) - NullStringMap, Prepare/Close, Composite PK, Functional Expressions
- **v0.9.1** (2025-12-23) - AI Agent Documentation (AGENTS.md, llms.txt, README updates)
- **v0.10.0** (2026-03-05) - BatchInsert/BatchUpdate/Upsert shortcuts, 1500+ tests, 88%+ coverage, GitHub Flow
- **v0.10.1** (2026-03-05) - Named placeholders `{:name}` in fluent builder Where
- **v0.11.0** (2026-03-17) - Exists, Count, ToSQL, ErrNotFound, error classification, Model Upsert, UpdateChanged
- **v0.11.1** (2026-06-19) - Expression table alias quoting fix, golangci-lint zero issues
- **v0.11.2** (2026-07-03) - Security: full identifier quoting, null-byte defense, func expression table aliases
- **v0.12.0** (2026-07-03) - Enterprise audit: zero panics, API pre-freeze, correctness fixes, integration test infrastructure
- **v0.12.1** (2026-07-05) - Transaction 3x performance (direct exec)
- **v0.13.0** (2026-07-05) - SelectSub, EqCol, type-safe scalar subqueries
- **v0.13.1** (2026-07-17) - ozzo-dbx parity: alias quoting, function guard, OFFSET/LIMIT, AndSelect
- **v0.14.0** (2026-07-17) - OrderByExpr/GroupByExpr for raw SQL in ORDER/GROUP BY
- **v0.14.1** (2026-07-17) - OrderBySub/GroupBySub type-safe expression ordering
- **v0.14.2** (2026-07-17) - scanReturningID fix (One→Row for scalar PK)
- **v0.14.3** (2026-07-17) - InsertStruct/BatchInsertStruct zero PK skip
- **v0.15.0** (2026-08-05) - Generic One[T]/All[T]/Scalar[T], UUID PK autoincrement
- **v0.16.0** (2026-08-07) - AutoID dual-key pattern, truly zero deps, Explain export, 85%+ coverage
- **v1.0.0** (Target: Q4 2026) - Production stable release

---

## 🙏 Acknowledgments

- Inspired by [ozzo-dbx](https://github.com/go-ozzo/ozzo-dbx)
- Community feedback and contributions
- **IrisMX** for real-world validation
- **Professor Ancha Baranova** for invaluable support

---

*Last Updated: 2026-08-07*
*Maintained by: Andrey Kolkov and CoreGX contributors*
