# Relica Roadmap

> **Current Version**: v0.20.1 (October 2026)
> **Previous Release**: v0.20.0 (Released: October 27, 2026)
> **Production Ready**: v1.0.0 (Target: Q4 2026)

---

## 🎯 Vision

**Relica** aims to be the **best query builder for Go** - lightweight, fast, and type-safe, while maintaining zero production dependencies.

**Philosophy**: *"If you want magic, use GORM. If you want control, use Relica."*

---

## 📍 Current State (v0.20.1)

### ✅ What's New in v0.20.1

- **Batch upsert from structs** (v0.20.1, #47): `BatchInsert(table, nil).ValuesStructs(rows).OnConflict(...).DoUpdateAll()`
- **Bind-parameter limit** (v0.20.1): a batch over 65535 parameters (PostgreSQL, MySQL) or 32766 (SQLite) is an error before anything is sent; Relica does not split it or open a transaction on its own
- **Fixes** (v0.20.1): `BatchInsertStruct` with rows of two types or an empty autoid field is an error; the audit's Where check sees non-literal table names

### ✅ Shipped in v0.20.0

A release of breaking fixes: where Relica returned a different result than the code asked for, with no error,
it now does what the code says or fails before sending anything. Upgrade path: [MIGRATION_GUIDE](docs/MIGRATION_GUIDE.md).

- **Upsert needs exactly one conflict action** (v0.20.0): `DoUpdate(cols...)`, the new `DoUpdateAll()` or `DoNothing()`; MySQL `DoNothing()` keeps the existing row; `WithMySQLUpsertSyntax` for the 8.0.19+ row alias
- **Table and column names** (v0.20.0): snake_case singular tables and snake_case untagged columns as in ozzo-dbx, `WithNaming`; embedded structs flattened on write, an embedded single value (`time.Time`, `sql.NullString`, a Scanner/Valuer) is one column
- **Reads fail instead of leaving zero values** (v0.20.0): `ScanRequireFields` by default (`ScanLenient`, `ScanStrict`, `WithScanMode`); `One`/`All` into a single value is an error
- **No silent whole-table statements** (v0.20.0): `Update`/`Delete` need `Where` or the explicit `AllRows()`; row locks only inside a transaction
- **Set operations read left to right** (v0.20.0): `OrderBy`/`Limit` after `Union` apply to the combined result; `a.Union(b).Intersect(c)` means the same on every database
- **`Count()`/`Exists()` follow what `All` returns** (v0.20.0) with `GroupBy`, `Distinct`, set operations and CTEs
- **SQLite identifiers in backticks** (v0.20.0): a misspelled column fails instead of comparing a string
- **Agent skill in the repository** (v0.20.0): `skills/relica/` with an audit script for the v0.20.0 behaviour changes; installable as a Claude Code plugin

### ✅ Shipped in v0.19.1

- **Case-insensitive LIKE on every database** (v0.19.1): `ILike`, `NotILike`, `OrILike`, `OrNotILike` — `ILIKE` on PostgreSQL, `LOWER(col) LIKE LOWER(?)` on MySQL and SQLite
- **Error classification by driver code** (v0.19.1): the `Is*Violation` helpers read SQLSTATE / MySQL error number / SQLite extended code instead of message text (works on localized servers and MariaDB); new `IsDeadlock`, `IsSerializationFailure`, `IsLockTimeout`
- **Query-building fixes** (v0.19.1): schema-qualified tables in DML, typed `CASE` results on PostgreSQL, ORDER BY / GROUP BY in call order, GROUP BY parameters bound before HAVING's; `Transactional` reports a failed rollback
- **CI runs the whole integration suite per database** (v0.19.1): previously name filters skipped 16 PostgreSQL tests and every multi-database test

### ✅ Shipped in v0.19.0

- **No internal types in the public API** (v0.19.0): all 20 types that were aliases to internal packages are declared in `relica` with full docs on pkg.go.dev; `Expression` is a sealed interface, `LikeExp`/`CaseExp`/`CoalesceExp`/`NullIfExp`/`GreatestLeastExp`/`ConcatExp` are interfaces, `Option` is opaque; `Unwrap()` removed; a test guards the whole class
- **Invalid expressions fail instead of dropping the filter** (v0.19.0): an invalid `Like(...).EscapeChars` no longer turns a `DELETE`/`UPDATE` into an unconditional one
- **CASE, SQLite LIKE, HAVING fixes** (v0.19.0): Expression operands in CASE are rendered; `ESCAPE '\'` on SQLite; `{:name}` in `Having`
- **MySQL `Explain`/`ExplainAnalyze`** (v0.19.0): did not work on any real MySQL server (quoted numbers in the JSON plan; `EXPLAIN ANALYZE` is TREE text) — both fixed
- **Integration suite** (v0.19.0): 20 scenarios for the public types on PostgreSQL, MySQL, SQLite, 7 of them negative

### ✅ Shipped in v0.18.0

- **`relica.Executor`** (v0.18.0): common interface for `*DB` and `*Tx` — the `dbx.Builder` role for dbcontext-style `With(ctx)`; frozen method set, compile-time assertions
- **`DB.WithContext` fix** (v0.18.0): the stored context now reaches every query built from the returned `*DB`; previously cancellation and deadlines were silently ignored on the non-transactional path
- **`Query.WithContext`, `DB.WrapTx`** (v0.18.0): per-query context for `Insert`/`NewQuery`; join a `*sql.Tx` started by another library (ozzo-dbx `Wrap` parity)
- **Integration suite on real databases** (v0.18.0): 14 transaction/Executor scenarios on PostgreSQL, MySQL, SQLite, 7 of them negative

### ✅ Shipped in v0.17.x

- **`SelectQuery.Model(&struct)`** (v0.17.0): fetch by PK read from struct fields, composite PK, `.Where()`/`.ForUpdate()` composable
- **External audit fixes** (v0.17.2): `One()` no longer masks errors as `ErrNotFound`; PostgreSQL `$N` renumbering via single-pass `renderSQL`; placeholder lexer for JSONB `?|`/`?&`/`??`, string literals and comments; statement-cache race (`GetOrSet`); logger removed from the query hot path (`WithLogger` = `QueryHook`)
- **Wildcard quoting** (v0.17.1): `Select("n.*")` no longer emits `"n"."*"`

### ✅ Earlier

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

### Next minor — Go 1.27 + Generic Methods

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
- **v0.5.0** (2025-11-14) - Comprehensive documentation; security validator, audit log, query optimizer and tracing: planned (internal groundwork, not enabled)
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
- **v0.16.1** (2026-08-07) - Tx.Model() context propagation, ModelQuery.WithContext immutability, Row() returns ErrNotFound
- **v0.16.2** (2026-09-03) - UpsertOn custom conflict columns, BatchInsert OnConflict/DoUpdate/DoNothing
- **v0.16.3** (2026-09-03) - SQLite upsert ID population via RETURNING (#48)
- **v0.16.4** (2026-09-03) - SQLite upsert zero-PK fix, `interface{}` → `any`
- **v0.16.5** (2026-09-03) - BatchInsertQuery.OnConflict exported in public API
- **v0.16.6** (2026-09-03) - SqlDB, PingContext, DriverName, Tx raw SQL, ForUpdate/ForShare/SkipLocked, IsNull, Find
- **v0.16.7** (2026-09-03) - Scanner strips table alias prefix (silent data loss on SQLite JOINs)
- **v0.17.0** (2026-09-03) - SelectQuery.Model(&struct) fetch by PK, JOIN documentation
- **v0.17.1** (2026-09-04) - Wildcard quoting fix for Select("n.*")
- **v0.17.2** (2026-09-11) - External audit: One() error masking, $N renumbering, placeholder lexer, cache race, logger off hot path, MaskArgs
- **v0.17.3** (2026-10-03) - Docs: successor to ozzo-dbx, One()/AsExpression docstrings
- **v0.18.0** (2026-10-11) - relica.Executor, DB.WithContext fix, Query.WithContext, DB.WrapTx, integration suite on real databases
- **v0.19.0** (2026-10-18) - No internal types in the public API (20 aliases → real types, Unwrap removed), invalid-expression filter drop fixed, CASE/SQLite LIKE/HAVING fixes, MySQL Explain/ExplainAnalyze fixed
- **v0.19.1** (2026-10-18) - ILike family, error classification by driver code (+ IsDeadlock/IsSerializationFailure/IsLockTimeout), schema-qualified DML, PostgreSQL CASE typing, ORDER BY/GROUP BY call order, Transactional rollback error, full integration suite in CI
- **v1.0.0** (Target: Q4 2026) - Production stable release

---

## 🙏 Acknowledgments

- Inspired by [ozzo-dbx](https://github.com/go-ozzo/ozzo-dbx)
- Community feedback and contributions
- **IrisMX** for real-world validation
- **Professor Ancha Baranova** for invaluable support

---

*Last Updated: 2026-10-18*
*Maintained by: Andrey Kolkov and CoreGX contributors*
