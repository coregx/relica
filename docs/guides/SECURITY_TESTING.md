# Security Testing Guide

> **Status**: The SQL-injection validator and audit log are internal and cannot be enabled by users (planned).

Relica's protection is parameterized SQL: values passed through `?`, `{:name}` with `relica.Params`, or the Expression API (`relica.Eq`, `relica.In`, `relica.Like`, ...) are sent to the database as arguments, never as SQL text. Test your own data layer for the one way around it: string concatenation.

**Record every statement with a QueryHook** and assert that hostile input arrives only as an argument:

```go
func TestFindByNameKeepsInputOutOfSQL(t *testing.T) {
    var events []relica.QueryEvent
    db, err := relica.Open("sqlite", testDSN, relica.WithQueryHook(
        func(_ context.Context, e relica.QueryEvent) { events = append(events, e) }))
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    const attack = "x' OR '1'='1"
    _, _ = NewUserRepository(db).FindByName(context.Background(), attack)

    if len(events) == 0 {
        t.Fatal("no statement executed")
    }
    for _, e := range events {
        if strings.Contains(e.SQL, attack) {
            t.Fatalf("input concatenated into SQL: %s", e.SQL)
        }
    }
}
```

`QueryEvent` carries `SQL`, `Args`, `Duration`, `RowsAffected`, `Error` and `Operation`. For a check without a database round trip, call `ToSQL()` on the builder and assert on the SQL and arguments it returns.

**What to cover**: every repository method that accepts user input (search, sort column, filters, LIKE patterns); dynamic identifiers go through an allowlist or `db.QuoteColumnName`, never straight from the request. See the [Security Guide](SECURITY.md).
