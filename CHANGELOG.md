# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed (breaking) — upsert needs exactly one explicit conflict action (ADR-019)

- **`Upsert` / `BatchInsert(...).OnConflict(...)` take exactly one of `DoUpdate(cols...)`, `DoUpdateAll()`, `DoNothing()`.** No action, two actions, `DoUpdate()` with no columns, a `DoUpdate` column that is not inserted, `OnConflict()` with no columns or called twice, and PostgreSQL/SQLite `DoUpdate`/`DoUpdateAll` without `OnConflict` are now errors returned by `Execute` before anything is sent. Before, an upsert without an action ran as a plain `INSERT`, `OnConflict` alone updated every column, a later action silently replaced an earlier one, and a batch `DoUpdate` without a target was dropped. A `BatchInsert` without `OnConflict` or an action is still a plain INSERT
- **New `DoUpdateAll()`** on `UpsertQuery` and `BatchInsertQuery`: updates every inserted column except the conflict target, on every dialect (on MySQL a row matched through another unique key keeps its target columns)
- **MySQL `DoNothing()` keeps the existing row** — it rendered no conflict clause and failed on the duplicate. It now renders `ON DUPLICATE KEY UPDATE c = c` (the first conflict column, else the first inserted column); not `INSERT IGNORE`, which also turns unrelated errors into warnings
- **New `WithMySQLUpsertSyntax(MySQLUpsertRowAlias)`** selects `VALUES (...) AS excluded ... col = excluded.col` (MySQL 8.0.19+) instead of the default `col = VALUES(col)`. Both read the proposed row after `BEFORE INSERT` triggers, like PostgreSQL's `EXCLUDED`; the server version is never probed
- **`Model(&m).Exclude(...).Upsert()` honours `Exclude`** — excluded columns were inserted and overwritten on every upsert; now they are left out of both the INSERT and the update, as in `Insert`. Unknown `fields`/`Exclude`/`UpsertOn` names, a field in the conflict target, an excluded field and a model with nothing to update are errors instead of being ignored or producing an empty `SET`

### Changed (breaking) — table and column names, struct scanning (ADR-018)

- **Derived table names are snake_case singular** — a model without `TableName()` used the lower-cased type name plus `s` (`UserProfile` → `userprofiles`, `Category` → `categorys`); it is now `user_profile`, `category`, as in ozzo-dbx. Add `TableName()` to keep a plural or legacy name
- **`TableName()` with a pointer receiver works on value models** — `db.Model(User{})` ignored `func (*User) TableName()`; both receivers are honoured for `T`, `*T`, `[]T` and `[]*T`, and on a non-nil `*T` the method runs on that value (instance-dependent names)
- **Untagged fields are snake_case columns** — a field without a `db` tag was written under its Go name (`"CreatedAt"`) and read case-insensitively; it is now `created_at` on write and read (ozzo-dbx rule: `UserID` → `user_id`, `HTTPLog` → `httplog`)
- **Embedded structs are flattened on write** — `InsertStruct`, `Model().Insert/Update/Upsert` and `UpdateChanged` wrote an embedded struct as one column named after its type (or failed with "unsupported type"); its fields are now columns, as the reader always did. Embedded pointers and `db` tags on embedded structs are errors
- **Legacy `db:"pk"` tag** wrote a column named `pk` while the primary-key code used the lower-cased field name; it now marks the primary key and takes the column from the naming rule (`UserKey int db:"pk"` → `user_key`)
- **Reading into a struct fails when a field has no result column** (`ScanRequireFields`, the new default) — such fields stayed zero with `err == nil`. Queries that list their columns (`Select("id", "name")`) require only those; `db:"-"` fields are never required; extra result columns are still ignored. `ScanLenient` restores the old behavior, `ScanStrict` also rejects extra columns — per DB (`WithScanMode`) or per query (`SelectQuery.ScanMode`, `Query.ScanMode`)
- **Errors for names that would silently collide** — two fields mapping to the same column (case-insensitive), a derived name over 63 bytes (PostgreSQL truncates silently) and a `Naming` that refuses a name are errors before anything is sent
- `BatchInsertStruct` with a zero primary key in some rows and explicit keys in others is an error — the key column was dropped when the first row's key was zero, losing the explicit keys
- Removed the unused internal `GetTableName` / `DefaultFieldMapFunc` (a third, different naming rule) and the reflection helpers they duplicated

### Changed (breaking) — UPDATE/DELETE that would hit every row (ADR-020 rules 1–2)

- **`Update(...)` / `Delete(...)` without `Where` are errors** — `db.Delete("t").Execute()` deleted every row; it now returns an error and sends nothing. Whole-table statements need the new explicit **`AllRows()`** on `UpdateQuery` / `DeleteQuery` (`AllRows()` together with `Where` is an error). Precedents: GORM `ErrMissingWhereClause`, bun "require at least one Where", MySQL `sql_safe_updates`
- **A condition that renders empty is an error in UPDATE/DELETE** — `Where(relica.And())`, `HashExp{}`, `Like("c")` with no values, `Where("")` dropped the WHERE clause and hit the whole table (also through `AndWhere`/`OrWhere`). SELECT keeps the old meaning (an empty condition selects every row)

### Changed (breaking) — row locks (ADR-020 rules 3–4, FIX-012)

- **`ForUpdate` / `ForShare` / `ForUpdateSkipLocked` outside a transaction are errors on every database** — on `*DB` the statement autocommits and the lock ends with it, so `ForUpdate` protected nothing and two `ForUpdateSkipLocked` workers could claim the same row. SQLite included, so a SQLite test suite catches the bug that would hit PostgreSQL in production; inside a transaction SQLite still omits the clause
- **A lock combined with UNION/INTERSECT/EXCEPT (in any branch), GROUP BY, DISTINCT, `Count()` or `Exists()` is an error** — PostgreSQL rejects these, MySQL and SQLite accepted or silently dropped the lock
- SQLite is recognised by its dialect, not by the driver name (`sqlite`/`sqlite3`), when deciding to omit the lock clause

### Changed (breaking) — names Relica cannot find are errors

- **`Model(&m).Insert(fields...)`, `Update(fields...)` and `Exclude(names...)`** skipped a name that is not a column of the model (a typo silently inserted or updated the wrong set of columns); it is now an error and nothing is sent. Names are column names (the `db` tag)
- **`BatchInsert(...).ValuesMap(m)`** dropped keys that are not batch columns (a typo wrote NULL); such a key is now an error
- **`autoid` tag with `gen=<name>` that is not registered** fell back to UUID v7 silently; it is now an error naming the generator — register it with `RegisterIDGenerator` before inserting

### Changed (breaking) — `Count()` and `Exists()` follow what `All` returns (FIX-009)

- **`Count()` with `GroupBy` returned the first group's row count**; it now returns the number of groups (the rows `All` would return), as GORM does. `GroupByExpr`/`GroupBySub` items were dropped from the count while `Having` stayed; both now apply
- **`Count()` ignored `Distinct`, set operations and CTEs**: `Distinct` counted every row, `Union`/`Intersect`/`Except` counted only the first branch, a CTE was dropped (the query failed with "no such table"). They now apply: grouped, distinct and compound queries are counted as a derived table `SELECT COUNT(*) FROM (...) AS relica_count`
- **`Exists()` dropped `GroupBy`, `Having`, `Distinct`, set operations and CTEs** (it sent `SELECT 1` with only FROM, JOIN and WHERE), so a `Having` that filtered every group still returned true and a match in a later `Union` branch returned false. It now sends the query itself inside `SELECT EXISTS(...)`
- **`Count()` on a select list that aggregates without `GroupBy`** (`Select("MAX(x)")`, `SelectExpr("SUM(x) AS s")`) returned the number of matching rows instead of 1; a select list with anything but plain columns is now counted as a derived table
- `OrderBy`, `Limit` and `Offset` stay ignored by both (a paginated query reports its total); in a set operation they belong to the first branch and are kept

### Changed (breaking) — SQLite identifiers are quoted with backticks

- **A misspelled column on SQLite changed the result instead of failing.** Relica quoted SQLite identifiers with double quotes, and SQLite reads a double-quoted name that matches no column as a string literal: `Where(relica.Eq("nmae", "x"))` compared the string `'nmae'` with `'x'`, `HashExp{"stauts": 1}`/`In(...)` filtered nothing or everything, `Select("nmae")` returned the text `nmae` in every row, `OrderBy`/`GroupBy` sorted or grouped by a constant, and `Delete(...).Where(Eq("typo", "typo"))` deleted every row. Relica now quotes SQLite identifiers with backticks, as ozzo-dbx does; SQLite never reads those as literals, so the same code fails with `no such column`. PostgreSQL and MySQL are unchanged. Generated SQL on SQLite now reads `` SELECT `id` FROM `users` ``; code that compares generated SQL text, or relied on `Select("1")` coming back as the string `'1'`, needs updating

### Changed (breaking) — `OrderBy` takes "column [ASC|DESC]" only

- **`OrderBy` kept the first word and the direction and dropped the rest silently**: `OrderBy("salary + 1 DESC")` ran `ORDER BY "salary"`, `"created_at DESC NULLS LAST"` lost NULLS LAST, `"age DESC, name"` lost `name`, `"name COLLATE NOCASE"` lost the collation — a different order, no error, on every database. Each argument is now `"column"` or `"column ASC|DESC"` (any case); an argument containing `(` (a function call) is used as is; anything else, and an empty argument, is an error returned when the query runs. Use `OrderByExpr` for expressions, NULLS FIRST/LAST and COLLATE, and one argument per column. (ozzo-dbx quoted the whole text before the direction as one name, which at least failed in the database.)

### Removed

- **`relica.NewDB`** (deprecated since v0.12.0) — use `relica.Open(driver, dsn, opts...)`, same arguments
- **`Query.QueryParams()`** (deprecated) — use `Query.Params()`

### Fixed

- **`relica.Open` with a driver that has no SQL dialect panicked**; it returns an error naming the supported driver names, before the pool is opened. The message no longer points at the internal `dialects.RegisterDialect`
- **An error inside an embedded query was dropped** — a `FromSelect` subquery, a `Union`/`UnionAll`/`Intersect`/`Except` branch or a `With`/`WithRecursive` CTE with an invalid condition (for example a `Like(...).EscapeChars("!")`) lost that condition, and the statement ran unfiltered. The error now fails the outer statement — `All`, `One`, `Column`, `Count`, `Exists`, `Explain` — at any depth, including subqueries passed to `In`/`Exists`/`SelectSub` via `AsExpression()`. `Count()`, `Exists()` and `Explain()` also ignored the query's own construction error
- **A subquery in `SelectSub`, `OrderBySub` or `GroupBySub` lost an error it got after being added** (the check ran only when it was added, and the subquery rendered without its invalid condition); the error now fails the outer query. **`OrderBySub`/`GroupBySub` with a subquery rendered `ORDER BY SELECT …`** without parentheses; they now wrap it like `SelectSub`
- **`nil` passed to `FromSelect`, `Union`, `UnionAll`, `Intersect`, `Except`, `With` or `WithRecursive` panicked**; it is now an error returned when the query runs
- **A query that contains itself** (`q.Union(q)`, or two queries embedding each other) overflowed the stack when built; it is now an error
- **`UpdateStruct` wrote the primary key into SET** — the godoc example (a struct without an ID plus `Where("id = ?", …)`) ran `SET "id" = 0`, and `Where` on another column changed that row's key to 0. Like `Model().Update()`, `UpdateStruct` (on `DB`, `Tx` and `Builder()`) now leaves the primary-key columns (a `pk` tag, or a field named `ID`/`Id`) out of SET; a struct with no other column is an error
- **`MaskArgs` missed snake_case columns** such as `password_hash`, `access_token`, `refresh_token`, `user_password`, `stripe_api_key`: the word-boundary match treated `_` as a letter. A sensitive name now also matches as a part of a snake_case name; a column like `auth_method` is masked too (the safe direction)
- **`SelectQuery.ToSQL()` returned SQL for a query that cannot be built** (an invalid `Where`, a nil branch, an `OrderBy` that is not a column): text no statement will ever send, and a `WarmCache` key that is never hit. Like `UpdateQuery`/`DeleteQuery.ToSQL()`, it now returns `""` and nil params; running the query returns the error
- **`UnpinQuery` returned true for a cached query that was not pinned**; it returns false, as documented
- **`IsHealthy()` and `Stats().Healthy` reported true after `Close`**; a closed DB is not healthy (with or without `WithHealthCheck`)
- **A second `Close` on a DB opened `WithHealthCheck` panicked** (close of a closed channel); `Close` is now idempotent and returns nil on later calls
- **Statements the server rejected at prepare time never reached `QueryHook`/`WithLogger`** on PostgreSQL and MySQL (a missing table, a syntax error): the error returned before the hook. They are now reported with `Error` set, like any failed statement
- **Raw `ExecContext`, `QueryContext` and `QueryRowContext` on `DB` and `Tx` bypassed `QueryHook` and `WithLogger`**, which promise every statement; they are now reported (`QueryContext` when the rows are returned, `QueryRowContext` with the row's error). Expect log lines for raw SQL, DDL included, where there were none
- **`DetectOperation` (and `QueryEvent.Operation`) reported every `WITH …` statement as SELECT**; `WITH … DELETE/UPDATE/INSERT` is now classified by the statement after the CTEs. A compound select that starts with a parenthesis (set operations on PostgreSQL/MySQL) is SELECT instead of UNKNOWN, leading `--` and `/* */` comments (query tags) are skipped instead of making the statement UNKNOWN, and a word that only starts with a keyword (`SELECTED…`) is no longer taken for it
- **`Union`, `UnionAll`, `Intersect`, `Except` and `WithRecursive` failed on SQLite** with a syntax error: every operand was parenthesized, which SQLite's compound-select grammar does not accept. On SQLite plain operands are now written bare, and an operand with its own `OrderBy`/`Limit`/`Offset`, CTE or set operation becomes `SELECT * FROM (...)`, so it keeps its rows and grouping. PostgreSQL and MySQL are unchanged

### Added

- **`WithNaming(Naming)`** — how a DB derives names: `Naming{Table, Column}` funcs over `TableSource` / `ColumnSource` (model, declaring struct, field, index path). Built-in `Snake(...)` with `SplitAcronyms()` (`HTTPLog` → `http_log`), `Initialisms(GoInitialisms()...)` (`IPv4Address` → `ipv4_address`, whole tokens only), `TablePrefix`, `PluralTables`; `SnakeCase` for custom rules
- **`ScanMode`** — `ScanRequireFields` (default), `ScanLenient`, `ScanStrict`; `WithScanMode`, `SelectQuery.ScanMode`, `Query.ScanMode`
- **Agent skill in the repository** — `skills/relica/` (Agent Skills format) with `scripts/audit.sh`, which audits a project's Relica code for mistakes found in real projects and for every v0.20.0 behaviour change (OrderBy/Select/GroupBy arguments, UPDATE/DELETE without Where, upsert actions, row locks, naming, Count/Exists), with tests on fixtures. Installable as a Claude Code plugin: `/plugin install relica --marketplace coregx/relica`; any agent can read `<module Dir>/skills/relica/SKILL.md` at its version
- **`WrapDB(sqlDB, driver, opts...)` takes options** (hooks, logger, naming, scan mode, cache, health check) — existing calls compile unchanged

### Documentation

- **Examples that put expressions into `Select()` or an alias into `SelectExpr`'s args did not run** — the `WithRecursive` examples (godoc, README, ADVANCED_PATTERNS, CTE_GUIDE) used `Select(..., "1 as level")` and `Select(..., "h.level + 1")`, which `Select` quotes as identifiers (`"1"`, `"h"."level + 1"`); the `SelectExpr` godoc and SUBQUERY_GUIDE passed the alias as an argument (`SelectExpr(expr, "order_count")`, "0 placeholders but 1 args"). They now use `SelectExpr("… AS alias")`; the rule is in AGENTS.md › Ordering with Expressions and on `SelectExpr`
- **`WarmCache`/`PinQuery` examples warmed text no query sends** (`"SELECT * FROM users WHERE id = ?"`): the cache is keyed by the exact SQL with the dialect's quoting and placeholders, so these never matched a builder query, and `?` does not prepare on PostgreSQL. The godoc and PERFORMANCE_TUNING now take the key from `ToSQL()`; an integration test pins that the `ToSQL` text is the cache key
- **PRODUCTION_DEPLOYMENT's health endpoint returned `err.Error()` to the client** (host and user names in driver errors); it logs the error and answers `{"status":"unhealthy"}`. QUERY_ANALYZER_GUIDE no longer matches error text
- **Guides no longer show API users cannot call** — SECURITY, SECURITY_TESTING, QUERY_OPTIMIZER_GUIDE, LOGGING_GUIDE, PRODUCTION_DEPLOYMENT, PERFORMANCE_TUNING and BEST_PRACTICES described the SQL validator, audit log, optimizer and an OpenTelemetry tracer that exist only as internal groundwork, plus `db.StmtCacheStats()` and `db.SetQueryLogger()`, which never existed. They now document the public equivalents (parameters, quoting, `WithLogger`, `WithQueryHook`, `MaskArgs`, `Stats()`), tracing as a user-side `QueryHook` with OpenTelemetry (Relica stays dependency-free), and mark the internal features as planned. `PRODUCTION_DEPLOYMENT` also showed `WithConnMaxLifetime(300)`, which is 300 ns

- **SQLite read-only connections** (AGENTS.md › SQLite, TROUBLESHOOTING) — `?mode=ro` works only in a `file:` URI; both modernc.org/sqlite and mattn/go-sqlite3 drop the query of a plain path, so the "reader" opens read-write and, with `_txlock=immediate`, waits behind the writer with `database is locked`
- **`LikeExp.EscapeChars()` with no arguments** turns escaping off (AGENTS.md › String Operators) — only for patterns the code builds, never for user input
- **MIGRATION_GUIDE v0.19.1**: after a failed rollback `Transactional` returns `errors.Join(err, rollbackErr)`, so compare with `errors.Is`, not `==`

## [0.19.1] - 2026-10-18

Patch release: fixes for gaps in v0.19.0 and in the error helpers. The new functions below close
those gaps; nothing stops compiling.

### Fixed

- **Case-insensitive `LIKE` was lost in v0.19.0** — code that set `exp.Like = "ILIKE"` on the value returned by `relica.Like` no longer compiled (ADR-016 made `LikeExp` an interface), and no replacement existed. New `relica.ILike`, `NotILike`, `OrILike`, `OrNotILike` work on every database: `ILIKE` on PostgreSQL, `LOWER(col) LIKE LOWER(?)` on MySQL and SQLite (wildcard escaping as in `Like`; on SQLite `LOWER` folds only ASCII letters)
- **Error helpers classified by message text** — `IsUniqueViolation`, `IsForeignKeyViolation`, `IsNotNullViolation` and `IsCheckViolation` matched substrings of `err.Error()`, so they returned false on a PostgreSQL server with non-English `lc_messages`, missed MariaDB CHECK errors and MySQL NOT NULL errors 1263/1364. They now read the driver's error code anywhere in the error tree (`%w` chains, `errors.Join`): PostgreSQL SQLSTATE (pgx v5 and lib/pq, every version), MySQL/MariaDB error number (go-sql-driver/mysql), SQLite extended result code (modernc.org/sqlite, mattn/go-sqlite3). No driver is imported. The message is used only when the error carries no code (other drivers) (ADR-017)
- **`Transactional` / `TransactionalTx` dropped a failed rollback** — when `f` returned an error and the rollback failed too (for example the connection was lost), the rollback error was discarded. It is now returned as `errors.Join(err, rollbackErr)`; `sql.ErrTxDone` (the transaction already ended, for example by context cancellation) is not reported
- **Wrong transaction and error-handling examples in the documentation** — `Set(map[string]any{"balance": "balance - ?"})` (binds the text as the value; now `relica.NewExp`), `ForUpdate()` on `*relica.DB` outside a transaction, `defer tx.Rollback()` before checking `Begin`'s error, ignored `Execute`/`Commit` errors, repositories that bypassed the caller's transaction, `_, err := db.Model(&x).Insert()` (it returns only `error`), a retry example calling an undefined `isDeadlock`, and the claim that `Rollback` after `Commit` is a no-op (it returns `sql.ErrTxDone`). Every changed example was compiled and run against SQLite
- **Schema-qualified tables in INSERT, UPDATE, DELETE, upsert and batch statements** — `db.Insert("app.users", ...)` quoted the name as one identifier (`"app.users"`, PostgreSQL: `relation "app.users" does not exist`); SELECT already quoted it per part. All statements now produce `"app"."users"` / `` `app`.`users` ``, as does `DB.QuoteTableName`
- **`CASE` with number or bool results on PostgreSQL** — PostgreSQL types a `CASE` whose results are all untyped parameters as text, so `Update("t").Set(map[string]any{"n": relica.CaseWhen().When(cond, 9).Else(0)})` failed on an integer column (`column "n" is of type integer but expression is of type text`) and `OrderBySub(relica.CaseWhen()...)` sorted `10` before `2`. When every result is a Go number (or every one a bool), Relica now casts the parameters to the type a literal would have (`CAST($2 AS INTEGER)`, `BIGINT`, `NUMERIC`, `BOOLEAN`). Results that are Expressions or strings are unchanged
- **ORDER BY and GROUP BY ignored the call order** — `OrderBy` columns were always rendered before `OrderByExpr` and `OrderBySub` expressions, so `OrderBySub(priority).OrderBy("created_at")` sorted by `created_at` first. The same held for `GroupBy`, `GroupByExpr` and `GroupBySub`. Items are now rendered in the order the methods are called
- **`GroupByExpr` / `GroupBySub` parameters were bound after `Having` parameters** — GROUP BY comes before HAVING in the statement, but its parameters were appended after HAVING's, so `GroupByExpr("CASE WHEN price > ? ...", 60).Having("COUNT(*) >= ?", 2)` ran with `price > 2` and `COUNT(*) >= 60` and returned wrong results without an error. Parameters now follow the statement order: WHERE, GROUP BY, HAVING, ORDER BY

### Added

- **`relica.IsDeadlock`, `relica.IsSerializationFailure`, `relica.IsLockTimeout`** — classify PostgreSQL 40P01 / 40001 / 55P03, MySQL 1213 / 1205, 3572 and SQLite `SQLITE_BUSY_SNAPSHOT` / `SQLITE_BUSY`, `SQLITE_LOCKED`, for deciding whether to retry a transaction
- **Transaction guide for people and agents** — AGENTS.md "Transactions": rules, which API to use when, the dbcontext pattern end to end, context and isolation behaviour, SQLite specifics, a test that proves rollbacks discard writes, a `RetryTx` helper, and a table of mistakes found in real projects
- **Real-database tests** — error classification on PostgreSQL (through lib/pq and pgx), MySQL and SQLite, each class provoked for real (deadlock, serialization failure, lock timeout, NOWAIT); lib/pq v1.2.0 and mattn/go-sqlite3 in a separate CI module; `ILike` on case-sensitive collations; `Transactional` error paths

### Internal

- Timer-dependent unit tests use `testing/synctest` and no longer depend on scheduler timing (a flaky `TestWithTimeout` failed CI on macOS)
- **CI ran only part of the integration suite** — jobs selected tests with `-run Postgres` / `-run MySQL` / `-run SQLite`, which is case-sensitive and matches top-level names only: 16 `*_PostgreSQL` tests and every multi-database test (`*_AllDatabases`, IrisMX, WrapDB) never ran. Jobs now run the whole suite with `RELICA_TEST_DB=postgres|mysql|sqlite`; the other databases are skipped, and a container that does not start fails the job instead of skipping silently. The database matrix no longer cancels the other jobs when one fails. The first full run found the two library bugs above and two broken tests (concurrent queries on SQLite `:memory:`, a string written to an integer column)

---

## [0.19.0] - 2026-10-18

### Fixed

- **An invalid expression no longer drops the filter** — `Like(...).EscapeChars(...)` with an odd number of arguments stored an error and rendered as an empty condition; an empty condition is skipped, so `db.Delete("t").Where(invalidLike).Execute()` ran as an unconditional `DELETE`. Construction errors are now detected anywhere inside a condition tree (`And`/`Or`/`Not`, `HashExp` values, `CASE` branches, function arguments, subqueries) and in `Where`/`AndWhere`/`OrWhere`/`Having`/joins/`SelectSub`/`OrderBySub`/`GroupBySub`/`Update().Set` values. The query returns the error and nothing is sent to the database
- **`CASE` with Expression operands** — a searched-`CASE` condition that was an Expression (`CaseWhen().When(relica.GreaterThan("balance", 100), "gold")`) was printed with `fmt` into the SQL (`CASE WHEN &{balance > 100} …`) and its arguments were lost; Expressions used as `THEN`/`ELSE` values (`When(1, relica.Coalesce(…))`) were bound as parameters instead of rendered. Operands that are Expressions are now built into SQL; other values are parameters; a string condition in `CaseWhen` is still raw SQL as documented
- **SQLite `LIKE` escaping** — `Like("note", "50%")` escaped the `%` as `\%`, but SQLite has no default `LIKE` escape character, so the pattern never matched on SQLite. Relica now emits `ESCAPE '\'` on SQLite; PostgreSQL and MySQL already use backslash
- **`SelectQuery.Explain()` and `ExplainAnalyze()` failed on every MySQL server** — MySQL prints `filtered` and costs in `EXPLAIN FORMAT=JSON` as JSON strings, which the parser rejected (`cannot unmarshal string into ... filtered of type float64`); and `EXPLAIN ANALYZE` has no JSON form on MySQL 8.0 at all (it prints a TREE-format text plan), which was fed to the JSON parser. Numbers are now accepted quoted or unquoted, and `ExplainAnalyze` on MySQL parses the TREE plan (cost, estimated and actual rows, actual time, full scan, index used). Found by the new real-database suite
- **Named parameters in `Having`** — `Having("COUNT(*) > {:min}", relica.Params{"min": 5})` sent `{:min}` to the database. `Having` now resolves `{:name}` like `Where`
- **`NullStringMap.Keys()` is sorted** — it returned map iteration order, different on every call
- **`WithQueryHook(nil)` and `WithLogger(nil)` no longer panic** — a nil hook registered after another hook, or a nil logger, panicked on the first query. Both are now ignored. `NewSlogAdapter(nil)` uses `slog.Default()`

### Changed — no internal types in the public API (ADR-016)

Twenty exported types were aliases to internal packages (`type HashExp = core.HashExp`): pkg.go.dev, IDE hovers and AI agents saw an empty declaration with no fields, methods or docs. All of them are now declared in package `relica` with full documentation. A test (`api_surface_test.go`) fails the build if any exported identifier is an alias or mentions an internal package again.

- **Plain data types** — `TxOptions`, `PoolStats`, `QueryEvent`, `QueryPlan` are structs with the same fields; `QueryHook` is `func(ctx, QueryEvent)`. Source compatible
- **`Logger`, `BeforeInserter`** — interfaces with the same methods; `NoopLogger` and `SlogAdapter` are structs. Source compatible (`NoopLogger{}` and `&NoopLogger{}` both satisfy `Logger`)
- **`Params`, `NullStringMap`, `HashExp`** — defined map types; literals, indexing and methods work as before
- **`Expression` is a sealed interface** — it could never be implemented outside Relica (its method took an internal type); this is now explicit and documented. Use `NewExp` for custom SQL
- **`LikeExp`, `CaseExp`, `CoalesceExp`, `NullIfExp`, `GreatestLeastExp`, `ConcatExp` are interfaces** — constructors return them instead of pointers to internal structs. Method chaining is unchanged; **a declaration such as `var c *relica.CaseExp` becomes `var c relica.CaseExp`**
- **`Option` is an opaque struct** — build options only with the `With…` functions (the only possible way before as well: the old func type took an internal `*core.DB`). The zero `Option` is a no-op

### Removed

- **`DB.Unwrap()`, `Tx.Unwrap()`, `SelectQuery.Unwrap()`, `QueryBuilder.Unwrap()`** — they returned internal types and exposed the entire unversioned internal API. Use `DB.SqlDB()` for the underlying `*sql.DB`. No known consumer used them

---

## [0.18.0] - 2026-10-11

### Fixed

- **`DB.WithContext()` context was never applied to queries** — `Select`, `Insert`, `Update`, `Delete`, `Upsert`, `BatchInsert`, `BatchUpdate`, `NewQuery` and `Model` built from the returned `*DB` ran with `context.Background()`: `Builder()` and `NewQuery()` did not carry the stored context. Cancellation and deadlines passed through `WithContext` had no effect on non-transactional queries (transactions were unaffected — `Tx` binds the `Begin` context). The context now flows into every query; a per-query `WithContext` still takes priority

### Added

- **`Query.WithContext(ctx)`** — per-query context for the `*Query` returned by `Insert`, `InsertStruct`, `BatchInsertStruct` and `NewQuery`. Every other query type already had it; this closes the gap and matches `ozzo-dbx`
- **`DB.WrapTx(ctx, *sql.Tx)`** — adapt a transaction started outside Relica (another library, a migration tool, sqlc) into a `*relica.Tx`, so repository code taking `relica.Executor` can join it. Counterpart of `ozzo-dbx` `DB.Wrap`; complements `WrapDB`. Ownership stays with the caller — Commit/Rollback on either handle end the same transaction
- **`relica.Executor`** — exported interface implemented by both `*DB` and `*Tx`: all query builders (`Select`, `Insert`, `InsertStruct`, `BatchInsertStruct`, `Update`, `UpdateStruct`, `Delete`, `Upsert`, `BatchInsert`, `BatchUpdate`), `Model`, `NewQuery`, and raw `ExecContext`/`QueryContext`/`QueryRowContext`. Repositories and dbcontext-style `With(ctx)` helpers can now return one type for "the transaction from the context, or the plain connection" — the role `dbx.Builder` plays in ozzo-dbx. Compile-time assertions guarantee both types satisfy it. Lifecycle methods (`Begin`, `Commit`, `Rollback`, `Transactional`), `Builder()`, `WithContext()` and `Unwrap()` are intentionally excluded. The method set is frozen: future `*DB`/`*Tx` methods are not added to the interface, so external test doubles keep compiling

---

## [0.17.3] - 2026-10-03

### Fixed

- **README**: "Inspired by ozzo-dbx" → "Successor to ozzo-dbx" with maintainer info (Andrey Kolkov maintains both go-ozzo and coregx)
- **`One()` docstring**: corrected "Returns sql.ErrNoRows" → "Returns ErrNotFound (wraps sql.ErrNoRows)"
- **`AsExpression()` comment**: removed misleading "can pass without AsExpression" note — public `*SelectQuery` does NOT implement Expression

---

## [0.17.2] - 2026-09-11

### Breaking Changes (pre-v1.0 API cleanup)

- **`WithSensitiveFields` removed** — use `MaskArgs()` in custom hooks instead
- **`WithLogger` no longer logs parameters** — parameters are not logged by default. Use `MaskArgs()` in a custom `WithQueryHook` to log masked parameters

### Added

- **`MaskArgs(query, args)`** — exported safe parameter masking for custom QueryHook implementations. Note: masks ALL args of a statement that references a sensitive column (conservative heuristic)

### Changed

- **Logger removed from query hot-path** — `WithLogger()` now wraps Logger as a `QueryHook`. Zero overhead when no hook configured (~30µs → 0). `WithQueryHook` composes: multiple hooks chain instead of replacing

### Fixed

- **`One()` no longer masks errors as `ErrNotFound`** — `rows.Err()` is now checked before returning not-found. Context cancellation, network errors, and driver errors are returned as-is instead of being silently converted to ErrNotFound
- **Sanitizer overhead eliminated for default logger** — `MaskParams`/`FormatParams` (~60µs of regex per query) now skipped entirely when using NoopLogger (default). 7x speedup for default configuration
- **Statement cache race condition fixed** — new `GetOrSet()` atomic cache-or-insert prevents both the concurrent-close race and the orphaned-stmt leak. Loser of prepare race closes its own unobserved stmt. Note: cache capacity should be >= expected concurrency level
- **SQL placeholder lexer** — `?` inside single-quoted strings, SQL comments (`--`, `/* */`), and PostgreSQL JSONB operators (`?|`, `?&`) no longer incorrectly replaced with `$N`. `??` now emits single `?` (client-side escape convention)
- **PostgreSQL $N placeholder renumbering** — subqueries in IN/EXISTS, UNION branches, CTEs, GROUP BY/ORDER BY expressions now correctly renumbered. Previously `$1` was reused across outer and inner queries causing `bind message supplies N parameters, but prepared statement requires M`. New `renderSQL` + single-pass `replacePlaceholders` architecture
- **Documentation corrections** — all `Exists()`/`In()` subquery examples now use `.AsExpression()` (17 locations). Security/optimizer/tracing guides marked as internal-only where they reference unexported API

---

## [0.17.1] - 2026-09-04

### Fixed

- **Wildcard quoting in `Select("table.*")`** — `Select("n.*")` incorrectly generated `"n"."*"` (quoting wildcard as identifier), causing `no such column: n.*` on SQLite. Now correctly generates `"n".*`. Also fixed in `quoteColumn()` for WHERE/JOIN expressions containing wildcards

---

## [0.17.0] - 2026-09-03

### Added

- **`SelectQuery.Model(&struct)`** — fetch by PK directly from SelectQuery chain. PK values read from struct fields (no separate arguments). Supports composite PK, auto-table detection, WHERE append, partial column reads via `.Select("col1", "col2")`. Compatible with `ForUpdate()`, `WithContext()`, transactions. Port of ozzo-dbx `SelectQuery.Model()` with composite PK support and improved zero-value safety (explicit error instead of silent skip)

### Documentation

- Comprehensive JOIN documentation: string ON vs Expression ON with decision table
- `DISTINCT` common mistake warning (`Select("DISTINCT col")` → `.Distinct()`)
- All v0.16.6 API methods documented (SqlDB, PingContext, ForUpdate, Find, IsNull)

---

## [0.16.7] - 2026-09-03

### Fixed

- **Scanner table prefix stripping** — `One()` and `All()` now strip table alias prefix from column names (e.g., `src.qualified_name` → `qualified_name`) before matching to struct `db:"..."` tags. SQLite returns column names with table prefix when using JOINs with table aliases; this caused silent data loss (fields not scanned). Inherited bug from ozzo-dbx
- **DISTINCT documentation** — fixed incorrect `Select("DISTINCT col")` example in ADVANCED_PATTERNS.md to use correct `.Distinct()` method

---

## [0.16.6] - 2026-09-03

### Added

- **`DB.SqlDB() *sql.DB`** — access underlying connection for pool tuning, migrations, prepared statements
- **`DB.PingContext(ctx) error`** — health check (complements existing `IsHealthy()`)
- **`DB.DriverName() string`** — returns driver name ("postgres", "mysql", "sqlite3")
- **`Tx.ExecContext()` / `QueryContext()` / `QueryRowContext()`** — raw SQL within transactions (parity with DB)
- **`SelectQuery.ForUpdate()`** — pessimistic row-level exclusive locking (silently ignored for SQLite)
- **`SelectQuery.ForShare()`** — shared row-level locking (silently ignored for SQLite)
- **`SelectQuery.ForUpdateSkipLocked()`** — non-blocking lock for job queue patterns (PostgreSQL 9.5+, MySQL 8.0+)
- **`IsNull(col)` / `IsNotNull(col)`** — explicit IS NULL / IS NOT NULL expressions (alias for `Eq(col, nil)`)
- **`GreaterOrEqualCol(col1, col2)` / `LessOrEqualCol(col1, col2)`** — column-to-column >= and <= comparisons (completes Col set)
- **`ModelQuery.Find(pk ...any)`** — one-liner fetch by primary key, supports composite PKs

---

## [0.16.5] - 2026-09-03

### Fixed

- **`BatchInsert().OnConflict()` public API missing** — internal methods existed (PR #49) but public wrapper methods on `BatchInsertQuery` were not exported in `db.go`. Users could not call `db.BatchInsert().OnConflict().DoUpdate().Execute()` from outside the package

---

## [0.16.4] - 2026-09-03

### Fixed

- **SQLite upsert zero PK insertion** — `Upsert()` and `UpsertOn()` included zero PK (`id=0`) in INSERT, preventing SQLite auto-increment. Now removes zero PK from dataMap before building upsert SQL (same logic as `Insert()`)
- **Integration tests for SQLite upsert** — 3 new tests on real SQLite verifying INSERT path, UPDATE path (conflict), and multi-row sequential upsert all populate correct auto-increment IDs

### Changed

- **`interface{}` → `any`** — migrated 326 occurrences in production code to modern Go style (`any` is alias for `interface{}` since Go 1.18). Zero behavioral change

---

## [0.16.3] - 2026-09-03

### Fixed

- **SQLite upsert ID population** (#48) — `Model().Upsert()` and `UpsertOn()` now use RETURNING clause on SQLite (same as PostgreSQL), fixing `last_insert_rowid()` returning 0 when ON CONFLICT triggers UPDATE path. Industry standard approach (GORM, bun, ent all use RETURNING for SQLite)
- **`needsPostgresReturning()` renamed to `needsReturning()`** — now covers both PostgreSQL and SQLite drivers. MySQL path unchanged (uses LastInsertId which works for upsert)

---

## [0.16.2] - 2026-09-03

### Fixed

- **`Model().Upsert()` with non-PK UNIQUE constraints** — `Upsert()` hardcoded PK as conflict target, failing when actual conflict happens on a UNIQUE column (e.g., `UNIQUE(email)` or composite `UNIQUE(project_id, qualified_name)`). Added `UpsertOn(conflictColumns, fields...)` to specify custom conflict columns
- **Batch upsert missing** — `BatchInsert` had no conflict handling. Added `OnConflict()`, `DoUpdate()`, `DoNothing()` methods on `BatchInsertQuery` for multi-row INSERT with ON CONFLICT support across all three databases
- **Upsert RETURNING/LastInsertId duplication** — extracted shared `executeUpsertQuery()` helper, eliminating code duplication between `Upsert()` and `UpsertOn()`

---

## [0.16.1] - 2026-08-07

### Fixed

- **`Tx.Model()` context propagation** — transaction context was not passed to `ModelQuery`, causing `tx.Model(&user).Insert()` to use `context.Background()` instead of the transaction's context with deadline/cancellation
- **`ModelQuery.WithContext()` immutability** — now returns a new copy (like `SelectQuery.WithContext()`), previously mutated in place causing shared state between `WithContext` calls
- **`Row()` error consistency** — now returns `ErrNotFound` (matching `One()`) instead of raw `sql.ErrNoRows`. All "no rows" terminal methods now consistently return `ErrNotFound`

---

## [0.16.0] - 2026-08-07

### Added

- **AutoID — Native Dual-Key Pattern** — Stripe-like prefixed IDs via struct tags. First query builder in any ecosystem with native dual-key support.

  ```go
  type User struct {
      ID       int64  `db:"id,pk"`
      PublicID string `db:"public_id,autoid:usr"` // auto-generated "usr_019078fa-..."
      Name     string `db:"name"`
  }

  user := User{Name: "Alice"}
  db.Model(&user).Insert()
  fmt.Println(user.PublicID) // "usr_019078fa-b37e-7abc-..."

  var found User
  db.Model(&found).FindByPublicID("usr_019078fa-b37e-7abc-...")
  ```

- **`autoid:prefix` struct tag** — declares a field as auto-generated external ID. UUID v7 by default, pluggable generators via `gen=name`. Prefix added automatically (`usr_`, `ord_`, etc.)
- **`FindByPublicID(publicID string) error`** — one-liner lookup by external ID with automatic prefix validation. Returns `ErrAutoIDPrefixMismatch` if wrong prefix, `ErrNotFound` if not found
- **`BeforeInserter` interface** — lifecycle hook called before Insert/Upsert. For timestamps, validation, custom ID strategies. Called before autoid generation
- **`RegisterIDGenerator(name, fn)`** — register custom ID generators (ULID, Snowflake, etc.) for use with `gen=name` in autoid tag
- **`ErrAutoIDPrefixMismatch`** — sentinel error for prefix validation in FindByPublicID. Works with `errors.Is()`
- **`Query.ToSQL()`** — consistent SQL preview on all query types including Insert and NewQuery. Previously only available on Select/Update/Delete/Upsert/Batch
- **Zero-dependency UUID v7 generator** — RFC 9562 compliant, ~300ns/op, stdlib only (`crypto/rand` + `time`)

### Example

```go
// Define model with dual-key (1 line per model):
type User struct {
    ID       int64  `db:"id,pk"`
    PublicID string `db:"public_id,autoid:usr"`
    Name     string `db:"name"`
}

// Insert — auto-generates PublicID, auto-populates ID:
user := User{Name: "Alice"}
db.Model(&user).Insert()
// user.ID = 42, user.PublicID = "usr_019078fa-b37e-7abc-..."

// Lookup by public ID (one-liner):
var found User
db.Model(&found).FindByPublicID("usr_019078fa-b37e-7abc-...")

// Prefix validation (safety):
err := db.Model(&found).FindByPublicID("ord_019078fa-...")
// errors.Is(err, relica.ErrAutoIDPrefixMismatch) == true

// Pre-set ID for testing (not overwritten):
user := User{PublicID: "usr_test-id", Name: "Test"}
db.Model(&user).Insert() // PublicID stays "usr_test-id"

// BeforeInserter hook:
func (u *User) BeforeInsert() error {
    u.CreatedAt = time.Now()
    return nil
}

// Custom generator:
relica.RegisterIDGenerator("ulid", func() string {
    return ulid.Make().String()
})
type Event struct {
    PublicID string `db:"public_id,autoid:evt,gen=ulid"`
}

// ToSQL on Insert (new):
sql, params := db.Insert("users", data).ToSQL()

// Explain (new export):
plan, err := db.Select().From("users").Where(relica.Eq("id", 1)).Explain()
```

### Changed

- **Truly zero dependencies** — go.mod has no `require` blocks. All test dependencies (testify, database drivers, modernc.org/sqlite) removed from main module and isolated in `test/` and `benchmark/` sub-modules. Tests rewritten to use Go stdlib `testing` only
- **`Explain()` / `ExplainAnalyze()` exported** — query plan analysis now available through public API on `SelectQuery`
- **`QueryPlan` type exported** — unified query plan structure for PostgreSQL, MySQL, SQLite EXPLAIN output

---

## [0.15.0] - 2026-08-05

### Added

- **Generic `One[T]`, `All[T]`, `Scalar[T]`** — Type-safe scan destination via Go generics. Additive API alongside existing `interface{}`-based methods. Follows GORM `gorm.G[T]` and sqlx-v2 `GetG[T]` industry pattern.
- **`autoincrement` tag for UUID/string PK** — `db:"id,pk,autoincrement"` enables PostgreSQL RETURNING for server-generated non-integer primary keys (e.g., `gen_random_uuid()`)
- **`scanReturningIntoField`** — shared type-switch helper for int/uint/string PK scanning in RETURNING clause

### Changed

- **`IsPrimaryKeyZero`** — uses `reflect.IsZero()` for all types. Empty string `""` and `[16]byte{}` (nil UUID) now correctly detected as zero PK. Backward compatible for int/uint.

### Example

```go
// Generic API (new):
user, err := relica.One[User](db.Select().From("users").Where(relica.Eq("id", 1)))
users, err := relica.All[User](db.Select().From("users").OrderBy("name"))
count, err := relica.Scalar[int64](db.Select("COUNT(*)").From("users"))

// UUID PK auto-populate (new):
type Post struct {
    ID    string `db:"id,pk,autoincrement"`  // server gen_random_uuid()
    Title string `db:"title"`
}
post := Post{Title: "Hello"}
db.Model(&post).Insert()
fmt.Println(post.ID)  // "a1b2c3d4-..." — auto-populated via RETURNING
```

---

## [0.14.3] - 2026-07-17

### Fixed

- **`InsertStruct` and `BatchInsertStruct` skip zero PK** — both methods now detect zero primary key (e.g., `ID=0`) and exclude it from INSERT, letting the database auto-increment. Matches `Model().Insert()` behavior. ([#39](https://github.com/coregx/relica/issues/39))

---

## [0.14.2] - 2026-07-17

### Fixed

- **`scanReturningID` used `One()` instead of `Row()` for scalar PK** — PostgreSQL RETURNING clause scanning would fail with `"dest must be pointer to struct, got pointer to int64"` on Upsert with auto-increment PK. Now uses `Row(&pkValue)` matching `insertWithReturning` and ozzo-dbx patterns.

---

## [0.14.1] - 2026-07-17

### Added

- **`OrderBySub(exp Expression)`** — type-safe expression in ORDER BY using CaseWhen() and other builders instead of raw SQL
- **`GroupBySub(exp Expression)`** — type-safe expression in GROUP BY

### Example

```go
// Instead of raw SQL:
OrderByExpr("CASE WHEN t.due_date < CURRENT_DATE THEN 0 WHEN t.due_date IS NULL THEN 3 ELSE 1 END")

// Type-safe builder:
OrderBySub(relica.CaseWhen().
    When("t.due_date < CURRENT_DATE", 0).
    When("t.due_date IS NULL", 3).
    Else(1))
```

---

## [0.14.0] - 2026-07-17

### Added

- **`OrderByExpr(expr string, args...)`** — raw SQL expressions in ORDER BY without quoting. Supports parameterized CASE WHEN, COALESCE, and complex ordering expressions ([#34](https://github.com/coregx/relica/issues/34))
- **`GroupByExpr(expr string, args...)`** — raw SQL expressions in GROUP BY. Supports DATE(), EXTRACT(), and computed grouping expressions

### Example

```go
// Before: OrderBy("CASE WHEN ...") produced "CASE" (quoted) — syntax error
// After: OrderByExpr passes through without quoting
db.Select("id", "title").
    From("tasks t").
    OrderByExpr("CASE WHEN t.due_date < CURRENT_DATE THEN 0 ELSE 1 END").
    OrderBy("t.due_date ASC").
    All(&rows)

// GroupByExpr for date grouping
db.Select("DATE(created_at) AS day", "COUNT(*)").
    From("orders").
    GroupByExpr("DATE(created_at)").
    All(&stats)
```

---

## [0.13.1] - 2026-07-17

### Fixed (ozzo-dbx parity)

- **SELECT alias quoting** — `Select("u.name AS display")` now properly quotes both column and alias: `"u"."name" AS "display"` (was passed through raw)
- **Function-call guard in quoteColumn** — `COUNT(*)`, `MAX(price)`, `SUM(total)` are no longer wrapped in quotes. Fixes invalid SQL in ORDER BY and GROUP BY with aggregate functions
- **OFFSET without LIMIT** — auto-emits `LIMIT 9223372036854775807` for MySQL compatibility (MySQL requires LIMIT before OFFSET)

### Added

- **`AndSelect(cols...)`** — append columns to existing SELECT for conditional query building, matching ozzo-dbx's `AndSelect` pattern

---

## [0.13.0] - 2026-07-05

### Added

- **`SelectSub(exp Expression, alias string)`** — Type-safe scalar subqueries in SELECT clause. No more raw SQL for correlated subqueries — use the builder API to construct subqueries and embed them with proper quoting and parameterization.
- **`EqCol(col1, col2 string)`** — Column-to-column equality comparison with proper identifier quoting. Essential for correlated subqueries and JOIN conditions: `EqCol("o.user_id", "u.id")` → `"o"."user_id" = "u"."id"`.
- **`NotEqCol`, `GreaterThanCol`, `LessThanCol`** — Column comparison variants for all common operators.

### Example

```go
// Before: raw SQL (not type-safe)
db.Select("id", "name").
    SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) AS order_count").
    From("users")

// After: type-safe builder API
sub := db.Select("COUNT(*)").From("orders").
    Where(relica.EqCol("orders.user_id", "users.id"))
db.Select("id", "name").
    SelectSub(sub.AsExpression(), "order_count").
    From("users")
```

---

## [0.12.1] - 2026-07-05

### Performance

- **Transaction queries 3x faster** — transactions now use direct `tx.ExecContext()`/`tx.QueryContext()` (1 network round-trip) instead of `PrepareContext()` + `ExecContext()` + `Close()` (3 round-trips). Matches ozzo-dbx, sqlx, bun, and GORM default behavior.
- Explicit `Prepare()` remains available as opt-in for batch scenarios within transactions.

### Added

- **13 transaction integration tests** — MultipleOps, Rollback (error + panic), ReadYourOwnWrites, BatchInsert (+rollback), Update+Delete, Upsert, manual Begin/Commit/Rollback, 50-op stress test. Runs on PostgreSQL, MySQL, SQLite in CI.

---

## [0.12.0] - 2026-07-03

### Breaking Changes (pre-v1.0 API cleanup)

- **`var` → `func`** — All 32 exported expression builders and error helpers are now proper functions (`func Eq(col string, value interface{}) Expression` instead of `var Eq = core.Eq`). Improves godoc, prevents accidental reassignment. `ErrNotFound` remains `var` (sentinel error value).
- **`Distinct(bool)` → `Distinct()`** — No parameter, always enables DISTINCT. Remove `Distinct(false)` calls (omit the method instead).
- **`QueryParams()` deprecated** — Use `Params()` instead. `QueryParams()` still works but marked deprecated.

### Added

- **`Tx.BatchInsert()`** — Batch insert operations now available in transactions
- **`Tx.BatchUpdate()`** — Batch update operations now available in transactions
- **`Tx.Upsert()`** — Upsert operations now available in transactions
- **`Tx.NewQuery()`** — Raw SQL queries now available in transactions
- **`ToSQL()`** on `UpsertQuery`, `BatchInsertQuery`, `BatchUpdateQuery` — All 6 query types now support SQL preview for debugging
- **`ModelQuery.WithContext(ctx)`** — Per-operation context support on Model API
- **`Query.Params()`** — Canonical method for retrieving query parameters

### Fixed (Correctness)

- **HAVING placeholder renumber** — Multi-arg HAVING clauses on PostgreSQL now get correct `$N` numbering (was subtracting clause count instead of arg count)
- **QuoteTableName/QuoteColumnName** — Now delegate to dialect (`"table"` for PostgreSQL, `` `table` `` for MySQL) instead of hardcoding double-quotes
- **Validator applied to builder queries** — `WithValidator()` now validates all query paths, not just raw SQL
- **Insert/Update with empty values** — Returns clean error instead of generating broken `() VALUES ()` SQL
- **Missing named params** — `Where("id = {:id}", Params{})` now returns error about missing `:id` instead of silently producing broken query
- **Schema-qualified tables** — `From("public.users u")` now generates `"public"."users" AS "u"` instead of `"public.users" AS "u"`
- **GenerateParamName** — Now dialect-aware (`$1` for PostgreSQL, `?` for MySQL) instead of hardcoded `p1`

---

## [0.11.2] - 2026-07-03

### Security

- **INSERT column names now quoted** — column names from `map[string]interface{}` keys are now properly quoted in INSERT statements, preventing SQL injection via map keys and syntax errors with reserved word columns (`order`, `select`, `group`, etc.)
- **UPDATE SET column names now quoted** — same fix applied to UPDATE SET clause
- **UPSERT conflict/update columns now quoted** — ON CONFLICT and DO UPDATE SET columns are quoted at the call site before passing to dialect
- **Model API WHERE clause** — `Update()`, `UpdateChanged()`, `Delete()` now use `Eq()` expressions for PK columns instead of raw string interpolation
- **Null-byte defense** — `QuoteIdentifier()` in all three dialects now strips `\x00` bytes before quoting

### Fixed

- **Functional expressions with table aliases** — `Case("u.status")`, `Coalesce("t.name", ...)`, `NullIf("u.score", 0)`, `Greatest("p.price", ...)`, `Least(...)`, `Concat("u.first_name", ...)` now correctly split table-aliased column names
- **N-part identifier support** — `quoteColumn()` upgraded from `SplitN` (2-part) to `Split` (N-part) for `schema.table.column` support
- **DRY: unified quoting** — `DB.quoteIdentifier()` in params.go now delegates to shared `quoteColumn()` helper

### Robustness

- **Zero panics from public API** — all 17 panic paths in query builder converted to stored errors via `buildErr` pattern. Errors propagate through `Build()`/`Execute()`/`All()`/`One()` instead of crashing the process
- **Affected methods**: `Where()`, `OrWhere()`, `Having()`, `FromSelect()`, `With()`, `WithRecursive()`, `JOIN ON`, `BatchInsert.Values()`, `BatchInsert.Build()`, `BatchUpdate.Build()`, `LikeExp.EscapeChars()`
- **`Model(nil)`** — nil guard returns clean error instead of nil pointer dereference
- **`WrapDB(nil)`** — immediate panic with clear message at construction (acceptable for constructor)
- **`pgx` driver** — registered as PostgreSQL dialect, `Open("pgx", dsn)` now works
- **`GetDialect`** — actionable panic message listing all supported dialects

### Example

```go
// Before v0.11.2: reserved word columns caused syntax errors
db.Insert("events", map[string]interface{}{
    "order":  1,        // ← SQL keyword, was unquoted → syntax error
    "select": "vip",    // ← SQL keyword, was unquoted → syntax error
}).Execute()

// After v0.11.2: all column names properly quoted
// PostgreSQL: INSERT INTO "events" ("order", "select") VALUES ($1, $2)
// MySQL:      INSERT INTO `events` (`order`, `select`) VALUES (?, ?)

// Before v0.11.2: wrong type in Where() crashed the process
db.Select().From("users").Where(42).All(&users) // PANIC!

// After v0.11.2: clean error returned
err := db.Select().From("users").Where(42).All(&users)
// err: "relica: Where() expects string or Expression, got int"
```

---

## [0.11.1] - 2026-06-19

### Fixed

- **Expression API: table-aliased column quoting** — `relica.Eq("c.deleted_at", nil)` now correctly generates `"c"."deleted_at" IS NULL` instead of `"c.deleted_at" IS NULL` ([#21](https://github.com/coregx/relica/issues/21))
- Affected expressions: `Eq`, `NotEq`, `GreaterThan`, `LessThan`, `GreaterOrEqual`, `LessOrEqual`, `In`, `NotIn`, `Between`, `NotBetween`, `Like`, `NotLike`, `HashExp`
- All three dialects fixed: PostgreSQL (`"t"."col"`), MySQL (`` `t`.`col` ``), SQLite (`"t"."col"`)

### Improved

- Extracted shared `quoteColumn()` helper to eliminate quoting logic duplication between query builder and expressions
- Replaced deprecated `reflect.Ptr` with `reflect.Pointer` across the codebase
- Extracted repeated string literals into package-level constants (`golangci-lint`: 49 → 0 issues)

### Example

```go
// Before v0.11.1 (broken): generates "c.deleted_at" IS NULL → PostgreSQL error
db.Select("c.id", "c.name").
    From("companies c").
    Where(relica.Eq("c.deleted_at", nil)).
    All(&rows)

// After v0.11.1 (fixed): generates "c"."deleted_at" IS NULL
db.Select("c.id", "c.name").
    From("companies c").
    Where(relica.Eq("c.deleted_at", nil)).  // ✓ works correctly
    All(&rows)

// All expression types now handle table aliases:
relica.In("u.role", "admin", "editor")           // "u"."role" IN (?, ?)
relica.Between("o.created_at", from, to)          // "o"."created_at" BETWEEN ? AND ?
relica.Like("p.name", "widget")                   // "p"."name" LIKE ?
relica.HashExp{"c.status": "active"}              // "c"."status"=?
```

---

## [0.11.0] - 2026-03-17

### Added

**Query Convenience Methods**

- `Exists()` on SelectQuery — wraps query in `SELECT EXISTS(...)`, returns `(bool, error)`
- `Count()` on SelectQuery — generates `SELECT COUNT(*)`, returns `(int64, error)`
- `ToSQL()` on SelectQuery, UpdateQuery, DeleteQuery — returns generated SQL and params without executing

**Error Handling**

- `relica.ErrNotFound` — sentinel error wrapping `sql.ErrNoRows` (returned by `One()` only)
- `relica.IsUniqueViolation(err)` — detects duplicate key errors (PostgreSQL, MySQL, SQLite)
- `relica.IsForeignKeyViolation(err)` — detects FK constraint violations
- `relica.IsNotNullViolation(err)` — detects NOT NULL constraint violations
- `relica.IsCheckViolation(err)` — detects CHECK constraint violations

**Model API Extensions**

- `db.Model(&user).Upsert(fields...)` — struct-based INSERT ON CONFLICT with auto PK detection
- `db.Model(&user).UpdateChanged(&original)` — UPDATE only fields that differ from original struct

### Example

```go
// Exists
exists, _ := db.Select().From("users").Where(relica.Eq("email", email)).Exists()

// Count
count, _ := db.Select().From("users").Where(relica.Eq("status", 1)).Count()

// ToSQL (preview without execution)
sql, params := db.Select().From("users").Where(relica.Eq("id", 1)).ToSQL()

// ErrNotFound
err := db.Select().From("users").Where(relica.Eq("id", 999)).One(&user)
if errors.Is(err, relica.ErrNotFound) { /* not found */ }

// Error classification
_, err = db.Model(&user).Insert()
if relica.IsUniqueViolation(err) { /* duplicate */ }

// Model Upsert
err = db.Model(&user).Upsert()           // all fields
err = db.Model(&user).Upsert("name")     // selective

// UpdateChanged
original := user
user.Name = "Updated"
err = db.Model(&user).UpdateChanged(&original) // only changed fields
```

---

## [0.10.1] - 2026-03-05

### Fixed

- Named placeholders `{:name}` now work in fluent builder `Where`, `AndWhere`, `OrWhere` on Select, Update, and Delete queries
- Previously `{:name}` only worked with `NewQuery()` + `BindParams()`, now consistent across the entire API

### Example

```go
db.Select().From("users").
    Where("id = {:id} AND status = {:status}", relica.Params{"id": 1, "status": "active"}).
    All(&users)
```

---

## [0.10.0] - 2026-03-05

### Added

**Direct API Shortcuts** - All operations available without `Builder()`

- `db.BatchInsert(table, columns)` — batch INSERT without `db.Builder().BatchInsert()`
- `db.BatchUpdate(table, keyColumn)` — batch UPDATE without `db.Builder().BatchUpdate()`
- `db.Upsert(table, values)` — UPSERT without `db.Builder().Upsert()`
- All query types now accessible directly: `db.Select()`, `db.Insert()`, `db.Update()`, `db.Delete()`, `db.BatchInsert()`, `db.BatchUpdate()`, `db.Upsert()`, `db.Model()`

**Comprehensive Test Coverage** - Enterprise-grade test suite for awesome-go submission

- **internal/dialects**: 0% → **100%** coverage
- **internal/util**: 41.2% → **97.1%** coverage
- **db.go (public API)**: 66.3% → **99.2%** coverage
- Total coverage: **80.7% → 88.3%**, test cases: **650+ → 1500+**

### Fixed

- `extractTableName()` in security/auditor now parses SQL instead of returning empty string
- All golangci-lint issues resolved (misspell, prealloc, staticcheck, cyclop)

### Changed

- **Documentation**: all examples use direct API (`db.Select()`) instead of `db.Builder().Select()`
- **Documentation**: fixed incorrect method signatures, non-compiling examples
- **Documentation**: validated all public docs against actual code

---

## [0.9.1] - 2025-12-23

### Added

**AI Agent Documentation** - Official documentation for AI coding assistants

- **AGENTS.md** - Official standard file for AI coding agents
  - API Priority guide: Model() API > Expression API > map[string]interface{}
  - Correct usage examples for INSERT, UPDATE, DELETE, SELECT
  - Anti-patterns to avoid (map[string]interface{} for CRUD)
  - Complete Expression API reference table
  - Production-ready code examples

- **llms.txt** - LLM documentation index following industry standard
  - Quick reference for AI agents
  - Links to key documentation files
  - API priority summary

### Changed

- **README.md** - Updated for AI agent compatibility
  - Added AI agent callout at the top linking to AGENTS.md
  - Added API Usage Priority table (PREFERRED/ACCEPTABLE/AVOID)
  - Quick Start examples now use Model() API instead of map[string]interface{}
  - CRUD Operations section reorganized with PREFERRED/AVOID labels
  - All examples follow Expression API for WHERE conditions

### Documentation

This release ensures AI coding agents (GitHub Copilot, Claude, Cursor, etc.) generate correct Relica code by providing clear guidance on API usage patterns.

---

## [0.9.0] - 2025-12-16

### Added

**4 New Features for Advanced Query Building**

**TASK-016: NullStringMap for Dynamic Scanning**
- `NullStringMap` type for scanning rows without predefined struct
- Helper methods: `String()`, `Int()`, `Float()`, `Bool()`, `Time()`, `IsNull()`
- Works with both `One()` and `All()` methods
- Useful for dynamic queries, admin panels, data exploration

```go
var result relica.NullStringMap
db.Select("*").From("users").Where("id = ?", 1).One(&result)
name := result.String("name")    // empty string if NULL
if !result.IsNull("email") { ... }

var results []relica.NullStringMap
db.Select("*").From("users").All(&results)
```

**TASK-017: Query.Prepare() / Query.Close() Manual Control**
- `Prepare()` - manually prepare statement for reuse
- `Close()` - release prepared statement
- `Bind()` - rebind parameters for execution
- Perfect for batch operations with same query structure

```go
q := db.NewQuery("SELECT * FROM users WHERE status = ?").Prepare()
defer q.Close()
for _, status := range statuses {
    q.Bind(status).All(&users)
}
```

**Composite Primary Key Support**
- `db:"column_name,pk"` syntax for composite PKs
- Works with Model API: Insert, Update, Delete
- Maintains field declaration order
- No auto-populate for CPK (by design, same as ozzo-dbx)

```go
type OrderItem struct {
    OrderID   int `db:"order_id,pk"`   // Part of composite PK
    ProductID int `db:"product_id,pk"` // Part of composite PK
    Quantity  int `db:"quantity"`
}

item := OrderItem{OrderID: 1, ProductID: 100, Quantity: 5}
db.Model(&item).Insert()  // INSERT with both PK values
db.Model(&item).Update()  // UPDATE WHERE order_id=1 AND product_id=100
db.Model(&item).Delete()  // DELETE WHERE order_id=1 AND product_id=100
```

**TASK-100: Functional Expressions (CASE, COALESCE, etc.)**
- `Case()` / `CaseWhen()` - CASE expressions (simple and searched)
- `Coalesce()` - first non-NULL value
- `NullIf()` - NULL if equal
- `Greatest()` / `Least()` - max/min from list (SQLite: MAX/MIN fallback)
- `Concat()` - cross-database string concatenation

```go
// Simple CASE
relica.Case("status").
    When("active", 1).
    When("inactive", 0).
    Else(-1).
    As("status_code")
// CASE "status" WHEN 'active' THEN 1 WHEN 'inactive' THEN 0 ELSE -1 END

// Searched CASE
relica.CaseWhen().
    When("age < 18", "minor").
    When("age >= 65", "senior").
    Else("adult").
    As("age_group")

// COALESCE
relica.Coalesce("nickname", "first_name", "'Anonymous'").As("display_name")

// NULLIF
relica.NullIf("email", "''").As("valid_email")

// GREATEST/LEAST (SQLite uses MAX/MIN)
relica.Greatest("price", "discount_price").As("max_price")
relica.Least("price", "sale_price").As("min_price")

// CONCAT (PostgreSQL/SQLite: ||, MySQL: CONCAT())
relica.Concat("first_name", "' '", "last_name").As("full_name")
```

**Quality Metrics**:
- 650+ tests passing
- 0 linter issues
- 86% test coverage
- 100% backward compatible

## [0.8.0] - 2025-12-16

### Added

**6 New Features from ozzo-dbx API Analysis**

**TASK-010: Named Placeholders**
- `{:name}` syntax for named parameters in queries
- `Bind(Params{})` method for parameter binding
- Works with all query types (SELECT, INSERT, UPDATE, DELETE)

```go
db.NewQuery("SELECT * FROM users WHERE id={:id} AND status={:status}").
    Bind(relica.Params{"id": 1, "status": "active"}).All(&users)
```

**TASK-011: Table/Column Quoting Syntax**
- `{{table}}` syntax for dialect-aware table quoting
- `[[column]]` syntax for dialect-aware column quoting
- Auto-converts to proper quotes (PostgreSQL: `"`, MySQL: `` ` ``, SQLite: `"`)

```go
db.NewQuery("SELECT [[name]], [[email]] FROM {{users}}").All(&users)
// PostgreSQL: SELECT "name", "email" FROM "users"
// MySQL: SELECT `name`, `email` FROM `users`
```

**TASK-012: Row() and Column() Methods**
- `Row()` - Scan single row into scalar value(s)
- `Column()` - Scan single column into slice

```go
var count int
db.Select("COUNT(*)").From("users").Row(&count)

var ids []int
db.Select("id").From("users").Where("active = ?", true).Column(&ids)
```

**TASK-013: Transactional() Helper**
- Auto-commit on success, auto-rollback on error
- Panic recovery with re-panic after rollback
- `TransactionalTx()` variant with custom TxOptions

```go
err := db.Transactional(ctx, func(tx *relica.Tx) error {
    if err := tx.Model(&user).Insert(); err != nil {
        return err  // Auto rollback
    }
    return tx.Model(&account).Insert()  // Auto commit on nil
})
```

**TASK-014: Distinct() Method**
- `Distinct(true)` adds SELECT DISTINCT
- Chainable with all SelectQuery methods

```go
db.Select("category").From("products").Distinct(true).All(&categories)
// SELECT DISTINCT "category" FROM "products"
```

**TASK-015: AndWhere() / OrWhere() Methods**
- Dynamic WHERE clause building
- Works with string conditions and Expression types
- Available on SelectQuery, UpdateQuery, DeleteQuery

```go
q := db.Select("*").From("users").Where("status = ?", 1)
if name != "" {
    q = q.AndWhere(relica.Like("name", name))
}
if role != "" {
    q = q.OrWhere(relica.Eq("role", role))
}
q.All(&users)
```

**Quality Metrics**:
- 600+ tests passing
- 0 linter issues
- 3874 lines of new code
- 100% backward compatible

## [0.7.0] - 2025-11-24

### Added

**100% ozzo-dbx Model() API Parity** - Complete ORM-style operations compatibility

**TASK-008: Auto-Populate ID After Insert**
- Auto-population of primary key after `Model().Insert()` across all databases
- PostgreSQL support via `RETURNING` clause (lib/pq doesn't support LastInsertId)
- MySQL/SQLite support via standard `LastInsertId()`
- Supports all numeric types: int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64
- Supports pointer variants: *int, *int64, etc. (allocates and populates)
- Pre-set IDs are not overwritten
- Non-numeric PKs (string, UUID) are gracefully skipped
- `DB.DriverName()` method for database detection (ozzo-dbx pattern)
- Zero-value PK removal from INSERT to support AUTO_INCREMENT

**TASK-009: Selective Fields API**
- `Insert(attrs ...string)` - Insert only specified fields
- `Update(attrs ...string)` - Update only specified fields
- `Exclude()` takes precedence over selective fields
- Empty args = all fields (100% backward compatible)
- Works seamlessly with auto-populate ID

**Example Usage**:
```go
// Auto-populate ID
user := User{Name: "Alice", Email: "alice@example.com"}
err := db.Model(&user).Insert()
fmt.Println(user.ID) // 1 (auto-populated!)

// Selective fields INSERT
user := User{Name: "Bob", Email: "bob@example.com", Status: "pending"}
err := db.Model(&user).Insert("name", "email") // Only name and email

// Selective fields UPDATE
user.Status = "active"
err := db.Model(&user).Update("status") // Only updates status

// Exclude takes precedence
db.Model(&user).Exclude("status").Insert("name", "email", "status")
// Result: Only name and email inserted
```

**Quality Metrics**:
- 85.5% average test coverage (target: ≥70%)
- 600+ tests passing
- 0 linter issues
- 100% backward compatible
- Professional git history with detailed commit messages

**Implementation Details**:
- `internal/util/reflection.go`: FindPrimaryKeyField, IsPrimaryKeyZero, SetPrimaryKeyValue
- `internal/core/model_query.go`: populatePrimaryKey, needsPostgresReturning, insertWithReturning
- `internal/core/db.go`: DriverName() method
- `internal/core/query.go`: appendSQL() for RETURNING clause
- 8 comprehensive tests for auto-populate ID
- 4 edge case tests for selective fields

### Changed
- Model() API now matches ozzo-dbx behavior 100%
- INSERT operations automatically remove zero-value PKs for AUTO_INCREMENT compatibility

### Fixed
- PostgreSQL auto-increment support (using RETURNING clause instead of LastInsertId)

## [0.6.0] - 2025-11-24

### Added

**Struct-Based Operations** - Type-safe database operations using Go structs

**Phase 1: InsertStruct/UpdateStruct**
- `InsertStruct(table, &struct)` - Insert using struct with db tags (eliminates manual map construction)
- `BatchInsertStruct(table, []struct)` - Batch insert for multiple structs
- `UpdateStruct(table, &struct)` - Update using struct with db tags
- Uses existing scanner infrastructure for zero duplication
- Full transaction and context support

**Phase 2: Model() API**
- `Model(&struct).Insert()` - Elegant CRUD with automatic table name detection
- `Model(&struct).Update()` - Auto WHERE by primary key
- `Model(&struct).Delete()` - Auto WHERE by primary key
- `TableName()` interface support - Custom table names via `func (T) TableName() string`
- `Table(name)` method - Override table name when needed
- `Exclude(fields...)` - Exclude specific fields from operations
- Primary key auto-detection - Searches for `db:"id"` tag or `ID` field
- Inspired by ozzo-dbx (our reference implementation)

**Example Usage**:
```go
// InsertStruct - explicit table name
user := User{Name: "Alice", Email: "alice@example.com"}
result, err := db.InsertStruct("users", &user).Execute()

// Model API - auto table name
func (User) TableName() string { return "users" }
err := db.Model(&user).Insert()

// Auto WHERE by primary key
user.Status = "active"
err = db.Model(&user).Update()  // WHERE id = ?
```

**Features**:
- Type safety with compile-time struct validation
- Zero manual map construction
- Automatic table name detection via TableName() interface
- Automatic primary key detection (db:"id" or ID field)
- Field control with Exclude()
- Works with transactions and context
- 48 tests (27 integration + 21 unit), 86% coverage
- Zero breaking changes (fully backward compatible)

### Documentation

- Updated [GETTING_STARTED.md](docs/guides/GETTING_STARTED.md) with comprehensive struct operations section
- Created [PUBSUB_FEATURE_RESPONSE.md](tmp/PUBSUB_FEATURE_RESPONSE.md) - Response to PubSub-Go feature requests

### Internal

- Created `internal/util/reflection.go` - StructToMap() utility for struct-to-map conversion
- Created `internal/core/model_query.go` - ModelQuery implementation with auto table/PK detection
- Added comprehensive test coverage (48 new tests)

---

## [0.5.0] - 2025-11-14

### Added

**Enterprise Security Features**
- SQL injection prevention with pattern-based validation (OWASP Top 10 coverage)
- Audit logging with context tracking (user, IP, request ID) and security events
- Parameter hashing for privacy compliance (GDPR, HIPAA, PCI-DSS, SOC2)
- Three audit levels: writes-only, reads+writes, all operations
- Strict mode for maximum security
- <2% performance overhead for protected queries

**Query Optimizer**
- Phase 1: Missing index detection and query cost analysis
- Phase 2: Advanced index analysis (covering indexes, index-only scans)
- Phase 3: Database-specific hints and recommendations
- Phase 4: Complete documentation and integration examples
- Automatic optimization suggestions for slow queries
- Support for PostgreSQL, MySQL, and SQLite dialects

**Query Analyzer**
- EXPLAIN plan integration for all three databases
- Automatic query performance analysis
- Execution time tracking and threshold-based alerts
- Integration with Query Optimizer for actionable recommendations
- Async analysis to avoid blocking query execution

**SQL Logging & Distributed Tracing**
- Structured logging with slog integration
- OpenTelemetry tracing support for distributed systems
- Automatic parameter sanitization (masks sensitive data)
- Query execution time tracking
- Context propagation for request correlation
- Support for both legacy and modern tracing APIs

**Performance Monitoring**
- Advanced connection pool management and health checks
- Statement cache warming for reduced cold-start latency
- Query pinning for critical queries (always cached)
- Connection pool statistics and metrics
- Health monitoring with automatic degradation detection
- Performance tuning recommendations

**Comprehensive Documentation** (10,000+ lines)
- Migration guides: GORM → Relica, sqlx → Relica (1,400 lines)
- User guides: Getting Started, Best Practices, Troubleshooting (2,000 lines)
- Advanced guides: Production Deployment, Performance Tuning, Advanced Patterns
- Security guides: Security Features, Security Testing, Compliance (1,360 lines)
- Performance docs: Performance Comparison, Tuning Guide, Benchmarks (450 lines)
- Feature guides: Query Optimizer, Query Analyzer, SQL Logging (5,000+ lines)

### Changed

- Removed "beta" suffix from version (v0.x allows breaking changes per semver)
- Upgraded codecov-action from v4 to v5 in CI/CD workflow
- Added minimum 70% coverage requirement with Codecov integration
- Organized private documentation into docs/dev/ (excluded from repository)

### Fixed

- Resolved 8 golangci-lint warnings:
  - Removed unnecessary type conversion in Stats() method
  - Renamed unused context parameter in analyzeQuery()
  - Extracted validation logic to reduce nesting complexity
  - Extracted logging logic to improve code maintainability
  - Added justified nolint directives for complex query execution paths
- Fixed code formatting issues across 14 files (gofmt compliance)
- Added .gitignore patterns for private documentation and navigation files

### Documentation

- Added comprehensive migration guides for GORM and sqlx users
- Created 6 detailed user guides covering all experience levels
- Added security documentation with compliance checklists
- Created performance comparison with industry benchmarks
- Updated README.md to follow 2025 best practices (removed version news)
- All guide headers updated to v0.5.0

### Internal

- Moved private documentation to docs/dev/ (COMPETITIVE_ANALYSIS.md, PERFORMANCE_BASELINE.md, architecture/)
- Added INDEX.md pattern to .gitignore for internal navigation files
- Created backup branches for safe Git history rewriting
- Cleaned all commit messages (removed AI attribution for professional output)

## [0.4.1-beta] - 2025-10-26

### Added

**API Convenience Methods** - Shorter syntax for common CRUD operations

Added convenience methods to `DB` and `Tx` for improved developer ergonomics:
- `Select(cols...)` - Create SELECT query without Builder()
- `Insert(table, data)` - Create INSERT query without Builder()
- `Update(table)` - Create UPDATE query without Builder()
- `Delete(table)` - Create DELETE query without Builder()

**Before (v0.4.0)**:
```go
db.Builder().Select("*").From("users").All(&users)
db.Builder().Insert("users", data).Execute()
db.Builder().Update("users").Set(data).Where("id = ?", 123).Execute()
db.Builder().Delete("users").Where("id = ?", 123).Execute()
```

**After (v0.4.1)**:
```go
db.Select("*").From("users").All(&users)  // 10 chars shorter!
db.Insert("users", data).Execute()
db.Update("users").Set(data).Where("id = ?", 123).Execute()
db.Delete("users").Where("id = ?", 123).Execute()
```

**Benefits**:
- **Shorter code**: Saves ~10 characters per query
- **More intuitive**: Matches patterns from ozzo-dbx and ActiveRecord
- **Preserves power**: All query features still available (WHERE, JOIN, ORDER BY, LIMIT, etc.)
- **Clear guidance**: Godoc recommends Builder() for advanced features (CTEs, UNION, batch ops)
- **100% backward compatible**: Builder() continues working unchanged

**When to use each approach**:
- **Convenience methods** (`db.Select()`, `db.Insert()`, etc.): Common CRUD operations
- **Builder()**: Advanced features (WITH, UNION, BatchInsert, BatchUpdate, Upsert)

**Implementation details**:
- Zero overhead: Direct delegation to Builder() (inlined by compiler)
- Same signatures as QueryBuilder methods
- Works identically in transactions (`tx.Select()`, `tx.Insert()`, etc.)
- Comprehensive godoc with examples for each method

**Test Coverage**: 93.3% (up from 92.9%)
- 16 new test cases in `convenience_test.go`
- Tests cover: DB methods, Tx methods, backward compatibility, complex queries
- All existing tests passing (no breaking changes)

**Files Changed**:
- `db.go`: +8 public methods (4 on DB, 4 on Tx) with comprehensive godoc
- `convenience_test.go`: +430 lines of tests
- `README.md`: Updated Quick Start and CRUD sections
- `CHANGELOG.md`: This entry

---

## [0.4.0-beta] - 2025-10-26

### Changed - BREAKING

**Migrated from type aliases to wrapper types** for improved pkg.go.dev documentation and Go best practices compliance.

#### Why This Change?

1. **Better Documentation**: pkg.go.dev now shows all methods with examples (was empty for type aliases)
2. **Industry Best Practice**: All major Go libraries (sqlx, pgx, GORM, cobra) use wrapper types
3. **Stable Public API**: Internal implementation can change without breaking user code
4. **Better IDE Support**: Full autocomplete and inline documentation

#### What Changed

**Before (v0.3.0)**:
```go
type DB = core.DB              // type alias
type QueryBuilder = core.QueryBuilder
type SelectQuery = core.SelectQuery
```

**After (v0.4.0)**:
```go
type DB struct {               // wrapper type
    db *core.DB
}

type QueryBuilder struct {
    qb *core.QueryBuilder
}
// ... all major types wrapped
```

#### Impact on Your Code

**95% of code requires ZERO changes**:
```go
// ✅ All of this still works exactly the same:
db, err := relica.Open("postgres", dsn)
defer db.Close()

db.Builder().Select("*").From("users").All(&users)

tx, _ := db.Begin(ctx)
tx.Builder().Insert("users", data).Execute()
tx.Commit()

sqlDB, _ := sql.Open("postgres", dsn)
db := relica.WrapDB(sqlDB, "postgres")  // ✅ Still works!
```

**5% of code might need updates** (rare cases):

1. **Type assertions to internal types**:
```go
// ❌ Before (v0.3.0):
coreDB := (*core.DB)(db)

// ✅ After (v0.4.0):
coreDB := db.Unwrap()  // New method
```

2. **Function signatures expecting internal types**:
```go
// ❌ Before:
func process(db *core.DB) { }

// ✅ After (Option 1 - use public type):
func process(db *relica.DB) { }

// ✅ After (Option 2 - use Unwrap):
func process(db *core.DB) { }
// Call with: process(db.Unwrap())
```

3. **Test type checks**:
```go
// ❌ Before:
assert.IsType(t, &core.DB{}, db)

// ✅ After:
assert.IsType(t, &relica.DB{}, db)
```

### Added

- **Unwrap() methods** for accessing internal types when needed:
  - `DB.Unwrap() *core.DB`
  - `QueryBuilder.Unwrap() *core.QueryBuilder`
  - `SelectQuery.Unwrap() *core.SelectQuery`
  - `Tx.Unwrap() *core.Tx`
  - All query types support Unwrap()

- **Comprehensive godoc** for all 81 public methods
- **Code examples** in godoc comments
- **docs/MIGRATION_GUIDE.md** - Detailed migration guide from v0.3.0 to v0.4.0

### Fixed

- **Critical Bug**: SELECT "*" was being quoted as SELECT "*" causing scan failures
  - Fixed in `internal/core/builder.go` line 634
  - Added check to not quote wildcard "*"

### Performance

- **Zero overhead**: Wrapper calls have 0ns overhead (inline optimization)
- All benchmarks passing with same performance as v0.3.0

### Quality

- **Test coverage**: 92.9% (improved from 89.9%)
- **Tests**: All 310+ tests passing
- **golangci-lint**: 0 issues
- **Integration tests**: SQLite + PostgreSQL passing

### Migration

See **[docs/MIGRATION_GUIDE.md](docs/MIGRATION_GUIDE.md)** for detailed migration instructions.

**Quick check**: `go build ./...` - if it compiles, you're 90% done!

**Migration time**: 15-30 minutes for typical projects

**Support**: https://github.com/coregx/relica/issues

---

## [0.3.0-beta] - 2025-10-25

### Added

#### Subquery Support (Phase 1)

**Full subquery support** for advanced SQL queries including IN, EXISTS, and FROM subqueries.

**Exists/NotExists Expressions**:
- `Exists(subquery)` - EXISTS expression for existence checks
- `NotExists(subquery)` - NOT EXISTS expression
- Works with any Expression or SelectQuery
- Proper parameter merging from nested queries

**Example**:
```go
sub := db.Builder().Select("1").From("orders").Where("orders.user_id = users.id")
db.Builder().Select("*").From("users").Where(relica.Exists(sub)).All(&users)
// Generates: SELECT * FROM "users" WHERE EXISTS (SELECT 1 FROM "orders" WHERE orders.user_id = users.id)
```

**IN/NOT IN with Subqueries**:
- `In(column, subquery)` - IN (SELECT ...) for filtering by subquery results
- `NotIn(column, subquery)` - NOT IN (SELECT ...)
- Automatic detection of subquery vs value list
- Backward compatible with value lists

**Example**:
```go
sub := db.Builder().Select("user_id").From("orders").Where("total > ?", 100)
db.Builder().Select("*").From("users").Where(relica.In("id", sub)).All(&users)
// Generates: SELECT * FROM "users" WHERE "id" IN (SELECT "user_id" FROM "orders" WHERE total > $1)
```

**FROM Subqueries**:
- `FromSelect(subquery, alias)` - Use subquery in FROM clause
- Alias is required (SQL standard)
- Supports complex nested queries
- Works with WHERE, JOIN, ORDER BY, etc.

**Example**:
```go
sub := db.Builder().Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")
db.Builder().FromSelect(sub, "order_counts").
    Select("user_id", "cnt").
    Where("cnt > ?", 10).
    All(&results)
// Generates: SELECT user_id, cnt FROM (SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id) AS order_counts WHERE cnt > $1
```

**Scalar Subqueries**:
- `SelectExpr(expression, args...)` - Raw SQL expressions in SELECT clause
- Supports scalar subqueries, window functions, complex calculations
- Parameter binding support

**Example**:
```go
db.Builder().
    Select("id", "name").
    SelectExpr("(SELECT COUNT(*) FROM orders WHERE user_id = users.id) as order_count").
    From("users").
    All(&users)
// Generates: SELECT id, name, (SELECT COUNT(*) FROM orders WHERE user_id = users.id) as order_count FROM users
```

**Features**:
- Multi-database support: PostgreSQL, MySQL 8.0+, SQLite 3.25+
- Nested subqueries (subquery within subquery)
- Proper parameter ordering and merging
- Zero breaking changes (fully backward compatible)

**Performance Notes**:
- Subqueries execute on database side (efficient)
- Parameter caching maintained
- LRU statement cache still applies

**Test Coverage**: 88.6% (26 new unit tests, all passing)

---

#### Set Operations (Phase 2)

**Full set operation support** for combining results from multiple queries (UNION, INTERSECT, EXCEPT).

**UNION / UNION ALL**:
- `Union(other)` - Combines results and removes duplicates
- `UnionAll(other)` - Combines results and keeps duplicates (faster)
- Supports chaining: `q1.Union(q2).Union(q3)`
- Works with all databases: PostgreSQL, MySQL 8.0+, SQLite 3.25+

**Example**:
```go
q1 := db.Builder().Select("name").From("users").Where("status = ?", 1)
q2 := db.Builder().Select("name").From("archived_users").Where("status = ?", 1)
q1.Union(q2).All(&names)
// Generates: (SELECT "name" FROM "users" WHERE status = $1) UNION (SELECT "name" FROM "archived_users" WHERE status = $2)
```

**INTERSECT**:
- `Intersect(other)` - Returns rows present in both queries
- Useful for finding overlapping data sets
- Supported: PostgreSQL, MySQL 8.0.31+, SQLite 3.25+

**Example**:
```go
q1 := db.Builder().Select("id").From("users")
q2 := db.Builder().Select("user_id").From("orders")
q1.Intersect(q2).All(&activeUsers)
// Finds users who have placed orders
```

**EXCEPT**:
- `Except(other)` - Returns rows in first query but not in second
- Useful for finding differences between data sets
- Supported: PostgreSQL, MySQL 8.0.31+, SQLite 3.25+

**Example**:
```go
q1 := db.Builder().Select("id").From("all_users")
q2 := db.Builder().Select("user_id").From("banned_users")
q1.Except(q2).All(&activeUsers)
// Finds all users except banned ones
```

**Features**:
- Automatic parentheses wrapping: `(SELECT ...) UNION (SELECT ...)`
- Correct parameter merging across queries
- PostgreSQL placeholder renumbering ($1, $2, $3...)
- Mix operations: `q1.Union(q2).Except(q3).Intersect(q4)`
- Nil safety: `Union(nil)` safely ignored
- Works with JOINs, subqueries, WHERE clauses

**Database Compatibility**:
- **PostgreSQL 9.1+**: All operations ✓
- **MySQL 8.0+**: UNION, UNION ALL ✓
- **MySQL 8.0.31+**: All operations ✓ (INTERSECT/EXCEPT added)
- **SQLite 3.25+**: All operations ✓

**Performance**:
- UNION ALL is 2-3x faster than UNION (no duplicate removal)
- Use UNION ALL when duplicates acceptable
- Consider EXISTS/NOT EXISTS instead of INTERSECT/EXCEPT for better performance

**MySQL < 8.0.31 Workarounds**:
```go
// Instead of INTERSECT, use WHERE IN:
db.Builder().Select("id").From("users").
    Where("id IN (SELECT user_id FROM orders)").All(&users)

// Instead of EXCEPT, use NOT EXISTS:
db.Builder().Select("*").From("users u").
    Where(NotExists(
        db.Builder().Select("1").From("banned b").Where("b.user_id = u.id")
    )).All(&users)
```

**Test Coverage**: 88.9% (21 new unit tests, all passing)

---

#### Common Table Expressions - CTEs (Phase 3)

**Full CTE support** for reusable query expressions and recursive hierarchical queries.

**Basic CTEs (WITH clause)**:
- `With(name, subquery)` - Adds a named CTE to the query
- Multiple CTEs: Chain `.With("cte1", q1).With("cte2", q2)`
- Automatic parameter merging from CTE queries
- Reusable query expressions (better performance than repeated subqueries)

**Example**:
```go
// Define reusable CTE
cte := db.Builder().
    Select("user_id", "SUM(total) as total").
    From("orders").
    GroupBy("user_id")

// Use CTE in main query
db.Builder().
    With("order_totals", cte).
    Select("*").
    From("order_totals").
    Where("total > ?", 1000).
    All(&users)
// Generates: WITH "order_totals" AS (SELECT user_id, SUM(total) as total FROM "orders" GROUP BY user_id) SELECT * FROM "order_totals" WHERE total > $1
```

**Multiple CTEs**:
```go
cte1 := db.Builder().Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")
cte2 := db.Builder().Select("user_id", "AVG(amount) as avg").From("payments").GroupBy("user_id")

db.Builder().
    With("order_counts", cte1).
    With("payment_averages", cte2).
    Select("o.user_id", "o.cnt", "p.avg").
    From("order_counts o").
    InnerJoin("payment_averages p", "o.user_id = p.user_id").
    All(&stats)
```

**Recursive CTEs (WITH RECURSIVE)**:
- `WithRecursive(name, subquery)` - Adds a recursive CTE
- Requires UNION or UNION ALL (anchor + recursive parts)
- Perfect for hierarchical data: org charts, trees, graphs
- Built-in validation: panics if UNION missing

**Example (Organizational Hierarchy)**:
```go
// Anchor: Top-level employees (no manager)
anchor := db.Builder().
    Select("id", "name", "manager_id", "1 as level").
    From("employees").
    Where("manager_id IS NULL")

// Recursive: Children of current level
recursive := db.Builder().
    Select("e.id", "e.name", "e.manager_id", "h.level + 1").
    From("employees e").
    InnerJoin("hierarchy h", "e.manager_id = h.id")

// Combine with UNION ALL
cte := anchor.UnionAll(recursive)

// Query the hierarchy
db.Builder().
    WithRecursive("hierarchy", cte).
    Select("*").
    From("hierarchy").
    OrderBy("level", "name").
    All(&employees)
// Generates: WITH RECURSIVE "hierarchy" AS ((SELECT id, name, manager_id, 1 as level FROM employees WHERE manager_id IS NULL) UNION ALL (SELECT e.id, e.name, e.manager_id, h.level + 1 FROM employees e INNER JOIN hierarchy h ON e.manager_id = h.id)) SELECT * FROM "hierarchy" ORDER BY level, name
```

**Example (Bill of Materials)**:
```go
// Anchor: Top-level product
anchor := db.Builder().
    Select("part_id", "qty", "1 as depth").
    From("bom").
    Where("product_id = ?", productID)

// Recursive: Sub-parts
recursive := db.Builder().
    Select("b.part_id", "b.qty * p.qty", "p.depth + 1").
    From("bom b").
    InnerJoin("parts_tree p", "b.product_id = p.part_id")

cte := anchor.UnionAll(recursive)

db.Builder().
    WithRecursive("parts_tree", cte).
    Select("part_id", "SUM(qty) as total_qty").
    From("parts_tree").
    GroupBy("part_id").
    All(&parts)
```

**Features**:
- **Validation**: Empty name, nil query, missing UNION all validated
- **Clear errors**: Panic messages like "recursive CTE requires UNION or UNION ALL"
- **Parameter safety**: Correct ordering (CTE params → SELECT params → WHERE params)
- **Dialect compatibility**: All 3 dialects with proper quoting
- **Combined features**: Works with JOINs, subqueries, set operations
- **Nested CTEs**: CTEs can reference other CTEs

**Database Compatibility**:
- **PostgreSQL 8.4+**: Full CTE support (basic and recursive) ✓
- **MySQL 8.0+**: WITH clause support ✓
- **SQLite 3.8.3+**: Basic WITH ✓
- **SQLite 3.25.0+**: Recursive WITH ✓

**When to Use CTEs**:
- **Reusable subqueries**: CTE defined once, used multiple times (better than repeating subquery)
- **Complex queries**: Break down complex logic into readable steps
- **Hierarchical data**: Organization charts, category trees, bill of materials
- **Recursive queries**: Graph traversal, path finding
- **Readability**: Named CTEs are self-documenting

**Performance Notes**:
- CTEs are materialized once (better performance when reused)
- Some databases optimize CTEs as inline views
- Recursive CTEs: Use LIMIT to prevent infinite recursion
- For single-use subqueries, inline subquery may be faster

**Test Coverage**: 89.5% (17 new unit tests, all passing)

---

#### Connection Management - WrapDB() (Phase 4)

**Wrap existing `*sql.DB` connections** with Relica's query builder for seamless integration with established database layers.

**API**:
- `WrapDB(sqlDB *sql.DB, driverName string) *DB` - Wrap external connection

**Use Cases**:
- **Enterprise Integration**: Add Relica to projects with existing connection pools
- **Custom Pool Configuration**: Apply advanced settings before wrapping
- **Gradual Migration**: Use Relica where it adds value, keep existing code elsewhere
- **Testing**: Wrap mock `*sql.DB` for testing

**Example**:
```go
// Create external connection with custom settings
sqlDB, _ := sql.Open("postgres", dsn)
sqlDB.SetMaxOpenConns(100)
sqlDB.SetMaxIdleConns(50)
sqlDB.SetConnMaxLifetime(time.Hour)

// Wrap with Relica query builder
db := relica.WrapDB(sqlDB, "postgres")

// Use Relica's fluent API
var users []User
db.Builder().
    Select("u.id", "u.name", "COUNT(o.id) as order_count").
    From("users u").
    LeftJoin("orders o", "o.user_id = u.id").
    GroupBy("u.id", "u.name").
    All(&users)

// Caller owns connection lifecycle
defer sqlDB.Close()  // NOT db.Close()
```

**Features**:
- **Single connection pool**: No duplicate resources
- **Isolated caches**: Each wrap gets its own statement cache
- **Full query builder**: All Relica features (JOINs, aggregates, subqueries, CTEs)
- **Transaction support**: Begin/Commit/Rollback works identically
- **Context support**: WithContext() propagates correctly
- **Zero overhead**: Lightweight wrapper, just adds query builder + cache

**Important Notes**:
- Caller is responsible for closing the underlying `*sql.DB` connection
- Each `WrapDB()` call creates a new instance with isolated cache
- Multiple wraps of same connection are supported (separate caches)

**Database Compatibility**:
- **PostgreSQL**: Full support ✓
- **MySQL 8.0+**: Full support ✓
- **SQLite 3.25+**: Full support ✓

**Production Validation**:
- **IrisMX** (first production user, 10K+ concurrent users) requested and validated this feature
- Enables adoption by enterprises with established database infrastructure
- Removes barrier for projects with existing connection pool management

**Test Coverage**: 89.9% (8 new unit tests + integration tests, all passing)

---

## [0.2.0-beta] - 2025-10-24

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

- Andrey Kolkov and CoreGX contributors
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

- Andrey Kolkov and CoreGX contributors
- Community contributors

---

[0.3.0-beta]: https://github.com/coregx/relica/releases/tag/v0.3.0-beta
[0.2.0-beta]: https://github.com/coregx/relica/releases/tag/v0.2.0-beta
[0.1.2-beta]: https://github.com/coregx/relica/releases/tag/v0.1.2-beta
[0.1.0-beta]: https://github.com/coregx/relica/releases/tag/v0.1.0-beta
