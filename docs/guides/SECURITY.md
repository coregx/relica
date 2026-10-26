# Security Guide

> **Relica Security** - SQL injection prevention by design
>
> This guide describes only protections you can use today from the public `relica` package.
> An SQL-injection validator and an audit log exist as internal groundwork and cannot be
> enabled by users (planned).

---

## Overview

| Protection | How you get it |
|---|---|
| Values never become SQL text | Query builder, `?` placeholders, `{:name}` + `relica.Params`, Expression API |
| Identifiers are quoted | Automatic in the builder; `{{table}}` / `[[column]]` in raw SQL; `db.QuoteTableName` / `db.QuoteColumnName` |
| LIKE wildcards in input are escaped | `relica.Like` and friends escape `%`, `_` and `\` by default |
| Database details stay out of responses | `relica.IsUniqueViolation` and the other error classifiers |
| Logs do not contain argument values | `relica.WithLogger` logs SQL without arguments; `relica.MaskArgs` for custom hooks |
| Runaway queries are cut off | `context.WithTimeout` + `WithContext` |

---

## Parameterized Queries

Every builder method sends values to the database as arguments. The SQL text contains only placeholders (`$1` on PostgreSQL, `?` on MySQL and SQLite):

```go
// Hostile input stays an argument
email := "x' OR 1=1 --"
var users []User
err := db.Select().From("users").Where(relica.Eq("email", email)).All(&users)
// PostgreSQL: SELECT * FROM "users" WHERE "email" = $1   args: [x' OR 1=1 --]

// Single-row writes through the Model API are parameterized too
err = db.Model(&user).Insert()
```

### Named placeholders

`{:name}` with `relica.Params` works in `Where`, `AndWhere`, `OrWhere`, `Having` and raw queries:

```go
err := db.Select("id", "name").
    From("users").
    Where("status = {:status} AND (owner_id = {:id} OR author_id = {:id})",
        relica.Params{"status": "active", "id": userID}).
    All(&users)

err = db.NewQuery("SELECT * FROM users WHERE id = {:id}").
    BindParams(relica.Params{"id": userID}).
    One(&user)
```

A name missing from `Params` makes the query fail with an error naming it.

### Raw SQL

Raw SQL is safe only when values go through placeholders:

```go
// ✅ Values as arguments
err := db.NewQuery("SELECT * FROM users WHERE id = ? AND status = ?").
    Bind(userID, "active").
    One(&user)

_, err = db.ExecContext(ctx, "UPDATE users SET status = ? WHERE id = ?", "blocked", userID)

// ❌ Never build SQL text from input
query := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", name) // SQL injection
```

The same rule applies to every method that takes raw SQL text: `relica.NewExp`, `SelectExpr`, `OrderByExpr`, `GroupByExpr` and string conditions in `Where`. Their SQL is used verbatim; pass values as their `args`.

---

## Expression API

Expression constructors build the SQL themselves and pass every value as an argument:

```go
err := db.Select().
    From("users").
    Where(relica.And(
        relica.Eq("status", "active"),
        relica.In("role", "admin", "editor"),
        relica.GreaterThan("age", minAge),
    )).
    All(&users)
// PostgreSQL: ... WHERE ("status" = $1) AND ("role" IN ($2, $3)) AND ("age" > $4)
```

Prefer the Expression API over hand-written condition strings: there is no SQL text for input to reach.

---

## Identifier Quoting

The builder quotes table and column names for the dialect (`"users"` on PostgreSQL and SQLite, `` `users` `` on MySQL), including schema-qualified names (`"public"."users"`).

In raw SQL, write `{{table}}` and `[[column]]`:

```go
err := db.NewQuery("SELECT [[id]], [[name]] FROM {{users}} WHERE [[id]] = {:id}").
    BindParams(relica.Params{"id": userID}).
    One(&user)
// PostgreSQL: SELECT "id", "name" FROM "users" WHERE "id" = $1
```

For identifiers chosen at runtime (a sort column from a request), check them against an allowlist first. `db.QuoteTableName` and `db.QuoteColumnName` quote a name and escape quote characters inside it, so a hostile name stays one identifier — but an allowlist is what guarantees it is a column you meant to expose:

```go
var sortable = map[string]bool{"name": true, "created_at": true}

col := r.URL.Query().Get("sort")
if !sortable[col] {
    col = "name"
}
err := db.Select().From("users").OrderBy(col).All(&users)
```

---

## LIKE Escaping

`relica.Like`, `NotLike`, `OrLike`, `OrNotLike` and the case-insensitive `ILike` family escape `%`, `_` and `\` in the values, so user input cannot add wildcards:

```go
err := db.Select().From("products").Where(relica.Like("name", search)).All(&products)
// search = "50%_off" → LIKE '%50\%\_off%' (literal match, value passed as an argument)

// Prefix match only
relica.Like("path", prefix).Match(false, true)

// Custom escaping: pairs of special character and replacement
relica.Like("code", input).EscapeChars("!", "!!")
```

An odd number of `EscapeChars` arguments is a construction error: `Err()` returns it and any query using the expression fails with it.

---

## Error Classification Without Leaking Details

Raw database errors can contain table, column and constraint names. Classify them and return your own errors to callers:

```go
if err := db.Model(&record).Insert(); err != nil {
    switch {
    case relica.IsUniqueViolation(err):
        return ErrAlreadyExists
    case relica.IsForeignKeyViolation(err):
        return ErrInvalidReference
    case relica.IsNotNullViolation(err), relica.IsCheckViolation(err):
        return ErrInvalidInput
    default:
        logger.Error("unexpected db error", "err", err) // full error stays in internal logs
        return ErrInternal
    }
}
```

The classifiers read the driver's error code (PostgreSQL SQLSTATE, MySQL error number, SQLite extended result code); the message text is consulted only when a driver attaches no code. `IsDeadlock`, `IsSerializationFailure` and `IsLockTimeout` classify concurrency errors the same way (see the package documentation for retrying a transaction).

---

## Logging Without Secrets

`relica.WithLogger` logs the SQL text (with placeholders), the duration, rows and the error. It does **not** log argument values:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
db, err := relica.Open("postgres", dsn,
    relica.WithLogger(relica.NewSlogAdapter(logger)))
```

A custom `relica.WithQueryHook` receives the arguments in `QueryEvent.Args`. Pass them through `relica.MaskArgs` before logging: if the SQL references a sensitive column (password, token, api_key and similar), all arguments of that statement are masked:

```go
db, err := relica.Open("postgres", dsn,
    relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {
        slog.InfoContext(ctx, "query", "sql", e.SQL, "args", relica.MaskArgs(e.SQL, e.Args))
    }))
```

Details: [Logging Guide](../LOGGING_GUIDE.md).

---

## Context Timeouts

Bound every request-driven query with a deadline, so a slow or abusive query cannot hold a connection indefinitely:

```go
ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
defer cancel()

err := db.Select().
    From("orders").
    Where(relica.Eq("user_id", userID)).
    WithContext(ctx).
    All(&orders)
```

`db.WithContext(ctx)` sets the context for every query built from the returned `*relica.DB`.

---

## Checklist

- [ ] No `fmt.Sprintf` or string concatenation of input into SQL text
- [ ] Raw SQL (`NewQuery`, `ExecContext`, `NewExp`, `*Expr` methods) uses placeholders for every value
- [ ] Dynamic identifiers (sort columns, table names) checked against an allowlist
- [ ] Search input goes through `relica.Like` / `relica.ILike`, not a hand-built pattern
- [ ] Errors classified with `relica.Is*`; raw database errors never reach API responses
- [ ] Custom query hooks mask arguments with `relica.MaskArgs`
- [ ] Request queries carry a context with a deadline
- [ ] TLS to the database enabled in the DSN (`sslmode=require` / `tls=true`)
- [ ] The application's database user has only the privileges it needs

Testing your data layer for injection: [Security Testing Guide](SECURITY_TESTING.md).

---

## Additional Resources

- **OWASP SQL Injection**: https://owasp.org/www-community/attacks/SQL_Injection
- **OWASP Query Parameterization Cheat Sheet**: https://cheatsheetseries.owasp.org/cheatsheets/Query_Parameterization_Cheat_Sheet.html
- **CWE-89 (SQL Injection)**: https://cwe.mitre.org/data/definitions/89.html

---

*For issues or questions, see [GitHub Issues](https://github.com/coregx/relica/issues)*
