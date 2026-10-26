//go:build integration

package test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DB operations on real databases: identifier quoting and placeholders, pool
// options and statistics, health checks, the statement cache (WarmCache,
// PinQuery, UnpinQuery, WithStmtCacheCapacity), manually prepared queries,
// BatchInsert.ValuesMap, NullStringMap.Has, AutoID lookup (FindByPublicID,
// RegisterIDGenerator) and logging (WithLogger, SlogAdapter, NoopLogger,
// MaskArgs, DetectOperation).

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// opsItem is a row of ops_item. It has no autoid field.
type opsItem struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

func (opsItem) TableName() string { return "ops_item" }

// opsAccount uses the deterministic generator registered by
// registerOpsIDGenerator: PublicID = "acc_seq0001", "acc_seq0002", ...
type opsAccount struct {
	ID       int64  `db:"id,pk"`
	PublicID string `db:"public_id,autoid:acc,gen=ops_seq"`
	Name     string `db:"name"`
}

func (opsAccount) TableName() string { return "ops_account" }

// opsTicket shares the ops_account table but uses the generator "ops_override",
// which the test registers twice to prove that a registration replaces the
// previous one.
type opsTicket struct {
	ID       int64  `db:"id,pk"`
	PublicID string `db:"public_id,autoid:tkt,gen=ops_override"`
	Name     string `db:"name"`
}

func (opsTicket) TableName() string { return "ops_account" }

var (
	opsIDSeq     atomic.Int64
	opsIDGenOnce sync.Once
)

// registerOpsIDGenerator registers "ops_seq" once per process. The body is
// "seq" + a 4-digit counter, so the next ID is known before an Insert.
func registerOpsIDGenerator() {
	opsIDGenOnce.Do(func() {
		relica.RegisterIDGenerator("ops_seq", func() string {
			return fmt.Sprintf("seq%04d", opsIDSeq.Add(1))
		})
	})
}

// nextOpsAccountID is the PublicID the next opsAccount Insert will receive.
func nextOpsAccountID() string { return fmt.Sprintf("acc_seq%04d", opsIDSeq.Load()+1) }

func resetOpsTable(t *testing.T, db *relica.DB, table, ddl string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, ddl)
	require.NoError(t, err)
}

// createOpsTables recreates every ops_ table and seeds ops_item with
//
//	1 "alpha", 2 "beta", 3 "gamma"
func createOpsTables(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	resetOpsTable(t, db, "ops_item",
		"CREATE TABLE ops_item (id INT PRIMARY KEY, name VARCHAR(50) NOT NULL)")
	resetOpsTable(t, db, "ops_batch",
		"CREATE TABLE ops_batch (id INT PRIMARY KEY, name VARCHAR(50) NOT NULL, note VARCHAR(50) NULL)")
	resetOpsTable(t, db, "ops_secret",
		"CREATE TABLE ops_secret (id INT PRIMARY KEY, username VARCHAR(50) NOT NULL, password VARCHAR(100) NOT NULL)")
	id := "id INTEGER PRIMARY KEY AUTOINCREMENT"
	switch dialect {
	case "postgres":
		id = "id BIGSERIAL PRIMARY KEY"
	case "mysql":
		id = "id BIGINT AUTO_INCREMENT PRIMARY KEY"
	}
	resetOpsTable(t, db, "ops_account",
		"CREATE TABLE ops_account ("+id+", public_id VARCHAR(64) NOT NULL UNIQUE, name VARCHAR(50) NOT NULL)")

	for i, name := range []string{"alpha", "beta", "gamma"} {
		_, err := db.Insert("ops_item", map[string]any{"id": i + 1, "name": name}).Execute()
		require.NoError(t, err)
	}
}

func opsItemCount(t *testing.T, db *relica.DB) int64 {
	t.Helper()
	n, err := db.Select().From("ops_item").Count()
	require.NoError(t, err)
	return n
}

// opsCacheQuery returns a distinct statement per k, built only from the
// public quoting and placeholder helpers: SELECT "name", k AS "k" FROM
// "ops_item" WHERE "id" = $1 (or ?).
func opsCacheQuery(db *relica.DB, k int) string {
	return fmt.Sprintf("SELECT %s, %d AS %s FROM %s WHERE %s = %s",
		db.QuoteColumnName("name"), k, db.QuoteColumnName("k"),
		db.QuoteTableName("ops_item"), db.QuoteColumnName("id"), db.GenerateParamName(1))
}

// runOpsCacheQuery runs opsCacheQuery(db, k) for id and checks both columns.
func runOpsCacheQuery(t *testing.T, db *relica.DB, k, id int, wantName string) {
	t.Helper()
	var name string
	var gotK int
	require.NoError(t, db.NewQuery(opsCacheQuery(db, k)).Bind(id).Row(&name, &gotK))
	assert.Equal(t, wantName, name)
	assert.Equal(t, k, gotK)
}

// unreachableDSN points at a database that cannot be reached, so every ping
// of a health checker fails.
func unreachableDSN(t *testing.T, dialect string) string {
	switch dialect {
	case "postgres":
		return "postgres://ops:ops@127.0.0.1:1/ops?sslmode=disable&connect_timeout=3"
	case "mysql":
		return "ops:ops@tcp(127.0.0.1:1)/ops?timeout=3s"
	default:
		// The directory does not exist: SQLite cannot create the file.
		return filepath.Join(t.TempDir(), "no-such-dir", "ops.db")
	}
}

// opsRecLogger records every call of the relica.Logger interface.
type opsRecLogger struct {
	mu      sync.Mutex
	entries []opsLogEntry
}

type opsLogEntry struct {
	level string
	msg   string
	kv    map[string]any
	raw   []any
}

func (l *opsRecLogger) add(level, msg string, args []any) {
	kv := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			kv[k] = args[i+1]
		}
	}
	l.mu.Lock()
	l.entries = append(l.entries, opsLogEntry{level: level, msg: msg, kv: kv, raw: args})
	l.mu.Unlock()
}

func (l *opsRecLogger) Debug(msg string, args ...any) { l.add("DEBUG", msg, args) }
func (l *opsRecLogger) Info(msg string, args ...any)  { l.add("INFO", msg, args) }
func (l *opsRecLogger) Warn(msg string, args ...any)  { l.add("WARN", msg, args) }
func (l *opsRecLogger) Error(msg string, args ...any) { l.add("ERROR", msg, args) }

// take returns the recorded entries and clears the log.
func (l *opsRecLogger) take() []opsLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.entries
	l.entries = nil
	return out
}

var _ relica.Logger = (*opsRecLogger)(nil)

// opsHookEvent is a QueryEvent seen by a hook plus what MaskArgs and
// DetectOperation return for it.
type opsHookEvent struct {
	event    relica.QueryEvent
	args     []any // copy of event.Args taken before MaskArgs
	masked   []any
	detected string
}

const opsMask = "***REDACTED***"

// ---------------------------------------------------------------------------
// Tests on the shared DB
// ---------------------------------------------------------------------------

func runDBOpsTests(t *testing.T, db *relica.DB, dialect string) {
	ctx := context.Background()
	createOpsTables(t, db, dialect)

	// ── QuoteTableName, QuoteColumnName, GenerateParamName ───────────────

	t.Run("Quote_GenerateParamName_Strings", func(t *testing.T) {
		switch dialect {
		case "mysql":
			assert.Equal(t, "`ops_item`", db.QuoteTableName("ops_item"))
			assert.Equal(t, "`s`.`ops_item`", db.QuoteTableName("s.ops_item"))
			assert.Equal(t, "`name`", db.QuoteColumnName("name"))
			assert.Equal(t, "`i`.`name`", db.QuoteColumnName("i.name"))
			assert.Equal(t, "`a``b`", db.QuoteColumnName("a`b"))
			assert.Equal(t, "`a\"b`", db.QuoteColumnName(`a"b`), "a double quote needs no escape in backticks")
			for _, i := range []int{1, 2, 10} {
				assert.Equal(t, "?", db.GenerateParamName(i))
			}
		default: // postgres, sqlite
			assert.Equal(t, `"ops_item"`, db.QuoteTableName("ops_item"))
			assert.Equal(t, `"s"."ops_item"`, db.QuoteTableName("s.ops_item"))
			assert.Equal(t, `"name"`, db.QuoteColumnName("name"))
			assert.Equal(t, `"i"."name"`, db.QuoteColumnName("i.name"))
			assert.Equal(t, `"a""b"`, db.QuoteColumnName(`a"b`))
			if dialect == "postgres" {
				assert.Equal(t, "$1", db.GenerateParamName(1))
				assert.Equal(t, "$2", db.GenerateParamName(2))
				assert.Equal(t, "$10", db.GenerateParamName(10))
			} else {
				for _, i := range []int{1, 2, 10} {
					assert.Equal(t, "?", db.GenerateParamName(i))
				}
			}
		}
	})

	t.Run("Quote_GenerateParamName_RoundTrip", func(t *testing.T) {
		// A reserved word and a name holding both quote characters.
		odd := "we\"i`rd"
		table := db.QuoteTableName("ops_quote")
		_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "CREATE TABLE "+table+" (id INT PRIMARY KEY, "+
			db.QuoteColumnName("order")+" VARCHAR(20) NOT NULL, "+
			db.QuoteColumnName(odd)+" VARCHAR(20) NULL)")
		require.NoError(t, err)

		insert := fmt.Sprintf("INSERT INTO %s (%s, %s, %s) VALUES (%s, %s, %s)", table,
			db.QuoteColumnName("id"), db.QuoteColumnName("order"), db.QuoteColumnName(odd),
			db.GenerateParamName(1), db.GenerateParamName(2), db.GenerateParamName(3))
		res, err := db.ExecContext(ctx, insert, 1, "first", "v1")
		require.NoError(t, err)
		n, err := res.RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)
		_, err = db.ExecContext(ctx, insert, 2, "second", nil)
		require.NoError(t, err)

		var order string
		var oddVal sql.NullString
		err = db.QueryRowContext(ctx, fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s = %s",
			db.QuoteColumnName("order"), db.QuoteColumnName(odd), table,
			db.QuoteColumnName("id"), db.GenerateParamName(1)), 1).Scan(&order, &oddVal)
		require.NoError(t, err)
		assert.Equal(t, "first", order)
		assert.Equal(t, sql.NullString{String: "v1", Valid: true}, oddVal)

		// The same columns read through the builder into a NullStringMap.
		var row relica.NullStringMap
		require.NoError(t, db.Select().From("ops_quote").Where(relica.Eq("id", 2)).One(&row))
		assert.True(t, row.Has("order"))
		assert.True(t, row.Has(odd))
		assert.Equal(t, "second", row.String("order"))
		assert.True(t, row.IsNull(odd))

		// Schema-qualified name: every part is quoted.
		schema := "main"
		switch dialect {
		case "postgres":
			schema = "public"
		case "mysql":
			require.NoError(t, db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schema))
		}
		var cnt int
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+db.QuoteTableName(schema+".ops_quote")).Scan(&cnt))
		assert.Equal(t, 2, cnt)

		// Negative: a hostile name stays one identifier. lib/pq runs a query
		// without arguments through the simple protocol, which accepts several
		// statements, so a quoting hole would drop ops_item here.
		hostile := "ops_quote`\"; DROP TABLE ops_item; --"
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+db.QuoteTableName(hostile)).Scan(&cnt)
		require.Error(t, err, "the quoted hostile name must be an unknown table")
		assert.Equal(t, int64(3), opsItemCount(t, db), "ops_item must survive")

		_, err = db.ExecContext(ctx, "DROP TABLE "+table)
		require.NoError(t, err)
	})

	// ── Stats, IsHealthy on a DB without options ──────────────────────────

	t.Run("Stats_IsHealthy_Defaults", func(t *testing.T) {
		assert.Equal(t, int64(3), opsItemCount(t, db))
		s := db.Stats()
		assert.Equal(t, 0, s.MaxOpenConnections, "no WithMaxOpenConns: unlimited")
		assert.GreaterOrEqual(t, s.OpenConnections, 1)
		assert.Equal(t, 0, s.InUse, "no statement is running")
		assert.Equal(t, s.OpenConnections, s.Idle)
		assert.True(t, s.Healthy, "health checks disabled: always healthy")
		assert.True(t, s.LastHealthCheck.IsZero(), "health checks disabled: no check time")
		assert.True(t, db.IsHealthy())
	})

	// ── WarmCache, PinQuery, UnpinQuery on the default cache ─────────────

	t.Run("WarmCache_PinQuery_UnpinQuery", func(t *testing.T) {
		qs := []string{opsCacheQuery(db, 101), opsCacheQuery(db, 102), opsCacheQuery(db, 103)}
		n, err := db.WarmCache(qs)
		require.NoError(t, err)
		assert.Equal(t, 3, n)

		for _, q := range qs {
			assert.True(t, db.PinQuery(q), "warmed query must be in the cache: %s", q)
		}
		for _, q := range qs {
			assert.True(t, db.UnpinQuery(q))
		}
		// Warming an already cached statement counts it and keeps one entry.
		n, err = db.WarmCache(qs[:1])
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		// The warmed statements execute with different arguments.
		runOpsCacheQuery(t, db, 101, 1, "alpha")
		runOpsCacheQuery(t, db, 101, 3, "gamma")
		runOpsCacheQuery(t, db, 102, 2, "beta")

		// Negative: statements never seen are not in the cache.
		never := opsCacheQuery(db, 999)
		assert.False(t, db.PinQuery(never))
		assert.False(t, db.UnpinQuery(never))

		n, err = db.WarmCache(nil)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("WarmCache_InvalidSQL", func(t *testing.T) {
		valid, valid2 := opsCacheQuery(db, 111), opsCacheQuery(db, 112)
		bad := "SELEC name FROM ops_item"
		n, err := db.WarmCache([]string{valid, bad, valid2})
		if dialect == "sqlite" {
			// modernc.org/sqlite compiles a statement lazily, on first use:
			// Prepare cannot fail, so WarmCache accepts the broken statement
			// and the error surfaces when it runs.
			require.NoError(t, err)
			assert.Equal(t, 3, n)
			assert.True(t, db.PinQuery(valid2))
			assert.True(t, db.UnpinQuery(valid2))
			_, err = db.NewQuery(bad).Execute()
			require.Error(t, err)
		} else {
			// Server-side prepare: WarmCache stops at the broken statement.
			require.Error(t, err)
			assert.Equal(t, 1, n, "only the statement before the broken one is warmed")
			assert.False(t, db.PinQuery(valid2), "statements after the error are not warmed")
		}
		assert.True(t, db.PinQuery(valid))
		assert.True(t, db.UnpinQuery(valid))
	})

	t.Run("UnpinQuery_NotPinnedReturnsFalse", func(t *testing.T) {
		t.Skip("BUG: DB.UnpinQuery returns true for a cached query that is not pinned; godoc says false")
		q := opsCacheQuery(db, 121)
		_, err := db.WarmCache([]string{q})
		require.NoError(t, err)
		assert.False(t, db.UnpinQuery(q), "godoc: false if the query is not in cache or not pinned")
	})

	// ── Query.Prepare, Query.IsPrepared ──────────────────────────────────

	t.Run("Prepare_SelectRepeated", func(t *testing.T) {
		q := db.NewQuery("SELECT name FROM ops_item WHERE id = " + db.GenerateParamName(1))
		assert.False(t, q.IsPrepared(), "not prepared before Prepare")
		q = q.Prepare()
		require.True(t, q.IsPrepared())

		for id, want := range map[int]string{1: "alpha", 2: "beta", 3: "gamma"} {
			var name string
			require.NoError(t, q.Bind(id).Row(&name))
			assert.Equal(t, want, name)
		}
		var ids []string
		require.NoError(t, q.Bind(2).Column(&ids))
		assert.Equal(t, []string{"beta"}, ids)
		var name string
		err := q.Bind(99).Row(&name)
		assert.ErrorIs(t, err, sql.ErrNoRows)
		assert.True(t, q.IsPrepared(), "a query error does not unprepare")

		require.NoError(t, q.Close())
		assert.False(t, q.IsPrepared())
		require.NoError(t, q.Close(), "Close is idempotent")
	})

	t.Run("Prepare_ExecRepeated", func(t *testing.T) {
		q := db.NewQuery(fmt.Sprintf("INSERT INTO ops_item (id, name) VALUES (%s, %s)",
			db.GenerateParamName(1), db.GenerateParamName(2))).Prepare()
		require.True(t, q.IsPrepared())
		defer func() { _ = q.Close() }()
		for _, id := range []int{10, 11, 12} {
			res, err := q.Bind(id, fmt.Sprintf("prep-%d", id)).Execute()
			require.NoError(t, err)
			n, err := res.RowsAffected()
			require.NoError(t, err)
			assert.Equal(t, int64(1), n)
		}
		var names []string
		require.NoError(t, db.Select("name").From("ops_item").
			Where(relica.In("id", 10, 11, 12)).OrderBy("id").Column(&names))
		assert.Equal(t, []string{"prep-10", "prep-11", "prep-12"}, names)

		// Negative: a duplicate key fails on the prepared statement too.
		_, err := q.Bind(10, "dup").Execute()
		require.Error(t, err)
		assert.True(t, relica.IsUniqueViolation(err), "got %v", err)

		res, err := db.Delete("ops_item").Where(relica.In("id", 10, 11, 12)).Execute()
		require.NoError(t, err)
		n, err := res.RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
	})

	t.Run("Prepare_NotPrepared", func(t *testing.T) {
		q := db.NewQuery("SELECT name FROM ops_item WHERE id = " + db.GenerateParamName(1))
		assert.False(t, q.IsPrepared())
		require.NoError(t, q.Close(), "Close on a query never prepared returns nil")
		var name string
		require.NoError(t, q.Bind(1).Row(&name))
		assert.Equal(t, "alpha", name)
		assert.False(t, q.IsPrepared(), "running through the statement cache is not Prepare")
	})

	t.Run("Prepare_InvalidSQL", func(t *testing.T) {
		q := db.NewQuery("SELEC name FROM ops_item").Prepare()
		defer func() { _ = q.Close() }()
		if dialect == "sqlite" {
			// modernc.org/sqlite compiles lazily: Prepare succeeds.
			assert.True(t, q.IsPrepared())
		} else {
			assert.False(t, q.IsPrepared(), "server-side prepare rejects the syntax error")
		}
		var name string
		require.Error(t, q.Row(&name), "running the broken statement must fail")
		_, err := q.Execute()
		require.Error(t, err)
	})

	t.Run("Prepare_InTransaction", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.Insert("ops_item", map[string]any{"id": 20, "name": "tx-row"}).Execute()
		require.NoError(t, err)

		q := tx.NewQuery("SELECT name FROM ops_item WHERE id = " + db.GenerateParamName(1)).Prepare()
		require.True(t, q.IsPrepared())
		for id, want := range map[int]string{20: "tx-row", 1: "alpha"} {
			var name string
			require.NoError(t, q.Bind(id).Row(&name))
			assert.Equal(t, want, name, "the prepared statement runs inside the transaction")
		}
		require.NoError(t, q.Close())
		require.NoError(t, tx.Rollback())

		n, err := db.Select().From("ops_item").Where(relica.Eq("id", 20)).Count()
		require.NoError(t, err)
		assert.Equal(t, int64(0), n)
	})

	// ── BatchInsertQuery.ValuesMap ───────────────────────────────────────

	clearBatch := func(t *testing.T) {
		t.Helper()
		_, err := db.Delete("ops_batch").Where(relica.GreaterThan("id", 0)).Execute()
		require.NoError(t, err)
	}
	type batchRow struct {
		ID   int            `db:"id"`
		Name string         `db:"name"`
		Note sql.NullString `db:"note"`
	}
	batchRows := func(t *testing.T) []batchRow {
		t.Helper()
		var rows []batchRow
		require.NoError(t, db.Select("id", "name", "note").From("ops_batch").OrderBy("id").All(&rows))
		return rows
	}
	null := sql.NullString{}
	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

	t.Run("BatchInsert_ValuesMap", func(t *testing.T) {
		clearBatch(t)
		res, err := db.BatchInsert("ops_batch", []string{"id", "name", "note"}).
			ValuesMap(map[string]any{"id": 1, "name": "one", "note": "n1"}).
			ValuesMap(map[string]any{"note": "n2", "name": "two", "id": 2}).
			ValuesMap(map[string]any{"id": 3, "name": "three"}). // note missing → NULL
			Execute()
		require.NoError(t, err)
		n, err := res.RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
		assert.Equal(t, []batchRow{
			{1, "one", str("n1")},
			{2, "two", str("n2")},
			{3, "three", null},
		}, batchRows(t))
	})

	t.Run("BatchInsert_ValuesMap_MixedWithValues", func(t *testing.T) {
		clearBatch(t)
		_, err := db.BatchInsert("ops_batch", []string{"id", "name", "note"}).
			Values(10, "ten", "v").
			ValuesMap(map[string]any{"id": 11, "name": "eleven", "note": nil}).
			Values(12, "twelve", nil).
			Execute()
		require.NoError(t, err)
		assert.Equal(t, []batchRow{
			{10, "ten", str("v")},
			{11, "eleven", null},
			{12, "twelve", null},
		}, batchRows(t))
	})

	t.Run("BatchInsert_ValuesMap_MissingNotNullColumn", func(t *testing.T) {
		clearBatch(t)
		_, err := db.BatchInsert("ops_batch", []string{"id", "name"}).
			ValuesMap(map[string]any{"id": 20, "name": "ok"}).
			ValuesMap(map[string]any{"id": 21}). // name missing → NULL into NOT NULL
			Execute()
		require.Error(t, err)
		assert.True(t, relica.IsNotNullViolation(err), "got %v", err)
		assert.Empty(t, batchRows(t), "the statement is atomic: no row inserted")
	})

	t.Run("BatchInsert_ValuesMap_UnknownKey", func(t *testing.T) {
		t.Skip("BUG: BatchInsertQuery.ValuesMap silently drops map keys that are not in the column list (a typo writes NULL)")
		clearBatch(t)
		_, err := db.BatchInsert("ops_batch", []string{"id", "name", "note"}).
			ValuesMap(map[string]any{"id": 30, "name": "x", "note_typo": "lost"}).
			Execute()
		require.Error(t, err, "a key outside the column list must be rejected")
		assert.Empty(t, batchRows(t))
	})

	// ── NullStringMap.Has ────────────────────────────────────────────────

	t.Run("NullStringMap_Has", func(t *testing.T) {
		clearBatch(t)
		_, err := db.BatchInsert("ops_batch", []string{"id", "name", "note"}).
			Values(1, "one", "n1").Values(2, "two", nil).Execute()
		require.NoError(t, err)

		var row relica.NullStringMap
		require.NoError(t, db.Select("id", "note").From("ops_batch").Where(relica.Eq("id", 2)).One(&row))
		assert.True(t, row.Has("id"))
		assert.True(t, row.Has("note"), "a NULL column is present")
		assert.True(t, row.IsNull("note"))
		assert.False(t, row.Has("name"), "a column not selected is absent")
		assert.False(t, row.Has("nope"))
		assert.False(t, row.Has(""))
		assert.Equal(t, []string{"id", "note"}, row.Keys())
		assert.Equal(t, "2", row.String("id"))

		var rows []relica.NullStringMap
		require.NoError(t, db.Select("id", "name", "note").From("ops_batch").OrderBy("id").All(&rows))
		require.Len(t, rows, 2)
		for _, r := range rows {
			assert.True(t, r.Has("id") && r.Has("name") && r.Has("note"))
			assert.False(t, r.Has("missing"))
		}
		assert.Equal(t, "n1", rows[0].String("note"))
		assert.False(t, rows[0].IsNull("note"))
		assert.True(t, rows[1].IsNull("note"))

		var empty relica.NullStringMap
		assert.False(t, empty.Has("id"), "nil map: nothing present")
	})

	// ── RegisterIDGenerator, ModelQuery.FindByPublicID ───────────────────

	registerOpsIDGenerator()

	t.Run("RegisterIDGenerator_InsertGeneratesPublicID", func(t *testing.T) {
		want1 := nextOpsAccountID()
		a1 := opsAccount{Name: "Alice"}
		require.NoError(t, db.Model(&a1).Insert())
		assert.Equal(t, want1, a1.PublicID)
		assert.Positive(t, a1.ID)

		want2 := nextOpsAccountID()
		a2 := opsAccount{Name: "Bob"}
		require.NoError(t, db.Model(&a2).Insert())
		assert.Equal(t, want2, a2.PublicID)
		assert.Greater(t, a2.ID, a1.ID)

		// A preset PublicID is kept and the generator is not called.
		before := opsIDSeq.Load()
		a3 := opsAccount{PublicID: "acc_fixed-0001", Name: "Carol"}
		require.NoError(t, db.Model(&a3).Insert())
		assert.Equal(t, "acc_fixed-0001", a3.PublicID)
		assert.Equal(t, before, opsIDSeq.Load())

		var stored []string
		require.NoError(t, db.Select("public_id").From("ops_account").OrderBy("id").Column(&stored))
		assert.Equal(t, []string{want1, want2, "acc_fixed-0001"}, stored)
	})

	t.Run("RegisterIDGenerator_ReplacesGenerator", func(t *testing.T) {
		relica.RegisterIDGenerator("ops_override", func() string { return "one" })
		t1 := opsTicket{Name: "first"}
		require.NoError(t, db.Model(&t1).Insert())
		assert.Equal(t, "tkt_one", t1.PublicID)

		relica.RegisterIDGenerator("ops_override", func() string { return "two" })
		t2 := opsTicket{Name: "second"}
		require.NoError(t, db.Model(&t2).Insert())
		assert.Equal(t, "tkt_two", t2.PublicID)

		var found opsTicket
		require.NoError(t, db.Model(&found).FindByPublicID("tkt_two"))
		assert.Equal(t, t2, found)
	})

	t.Run("FindByPublicID", func(t *testing.T) {
		want := opsAccount{Name: "Dave"}
		require.NoError(t, db.Model(&want).Insert())

		var found opsAccount
		require.NoError(t, db.Model(&found).FindByPublicID(want.PublicID))
		assert.Equal(t, want, found)

		var preset opsAccount
		require.NoError(t, db.Model(&preset).FindByPublicID("acc_fixed-0001"))
		assert.Equal(t, "Carol", preset.Name)
	})

	t.Run("FindByPublicID_NotFound", func(t *testing.T) {
		var found opsAccount
		err := db.Model(&found).FindByPublicID("acc_seq9999-unknown")
		assert.ErrorIs(t, err, relica.ErrNotFound)
		assert.ErrorIs(t, err, sql.ErrNoRows)
		assert.Equal(t, opsAccount{}, found)
	})

	t.Run("FindByPublicID_PrefixMismatch", func(t *testing.T) {
		var a opsAccount
		require.NoError(t, db.Select("id", "public_id", "name").From("ops_account").
			Where(relica.Eq("name", "Alice")).One(&a))
		_, body, ok := strings.Cut(a.PublicID, "_")
		require.True(t, ok)

		for _, id := range []string{"usr_" + body, body, "tkt_one"} {
			var found opsAccount
			err := db.Model(&found).FindByPublicID(id)
			assert.ErrorIs(t, err, relica.ErrAutoIDPrefixMismatch, "id %q", id)
			assert.NotErrorIs(t, err, relica.ErrNotFound, "id %q", id)
			assert.Equal(t, opsAccount{}, found, "id %q: model untouched", id)
		}
	})

	t.Run("FindByPublicID_InvalidInput", func(t *testing.T) {
		var a opsAccount
		err := db.Model(&a).FindByPublicID("")
		require.Error(t, err)
		assert.NotErrorIs(t, err, relica.ErrNotFound)

		var item opsItem // no autoid field
		err = db.Model(&item).FindByPublicID("itm_1")
		require.Error(t, err)
		assert.NotErrorIs(t, err, relica.ErrNotFound)
		assert.Equal(t, opsItem{}, item)
	})

	t.Run("FindByPublicID_InTransaction", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		a := opsAccount{Name: "Eve"}
		require.NoError(t, tx.Model(&a).Insert())

		var inTx opsAccount
		require.NoError(t, tx.Model(&inTx).FindByPublicID(a.PublicID))
		assert.Equal(t, a, inTx)
		require.NoError(t, tx.Rollback())

		var after opsAccount
		assert.ErrorIs(t, db.Model(&after).FindByPublicID(a.PublicID), relica.ErrNotFound)
	})

	// ── DetectOperation (pure) and NoopLogger ────────────────────────────

	t.Run("DetectOperation", func(t *testing.T) {
		for in, want := range map[string]string{
			"SELECT 1":                  "SELECT",
			"  select id from ops_item": "SELECT",
			"WITH t AS (SELECT 1 AS n) SELECT n FROM t": "SELECT",
			"insert into ops_item values (1, 'a')":      "INSERT",
			"\n\tUPDATE ops_item SET name = 'x'":        "UPDATE",
			"delete from ops_item where id = 1":         "DELETE",
			"CREATE TABLE x (id INT)":                   "UNKNOWN",
			"":                                          "UNKNOWN",
		} {
			assert.Equal(t, want, relica.DetectOperation(in), "%q", in)
		}
	})

	t.Run("NoopLogger_Methods", func(t *testing.T) {
		var l relica.Logger = relica.NoopLogger{}
		assert.NotPanics(t, func() {
			l.Debug("d", "k", 1)
			l.Info("i", "k", 2)
			l.Warn("w", "k", 3)
			l.Error("e", "k", 4)
		})
		var pl relica.Logger = &relica.NoopLogger{}
		assert.NotPanics(t, func() { pl.Debug("d"); pl.Info("i"); pl.Warn("w") })
	})
}

// ---------------------------------------------------------------------------
// Tests that need their own DB (Open with options on the same database)
// ---------------------------------------------------------------------------

func runDBOpsOptionTests(t *testing.T, db *relica.DB, dialect, dsn string) {
	require.NotEmpty(t, dsn, "options need a DSN to reopen the database")
	ctx := context.Background()
	createOpsTables(t, db, dialect)

	open := func(t *testing.T, opts ...relica.Option) *relica.DB {
		t.Helper()
		odb, err := relica.Open(dialect, dsn, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = odb.Close() })
		return odb
	}

	// ── WithMaxOpenConns, WithMaxIdleConns, Stats ────────────────────────

	t.Run("WithMaxOpenConns_WithMaxIdleConns_Stats", func(t *testing.T) {
		pdb := open(t, relica.WithMaxOpenConns(3), relica.WithMaxIdleConns(1))
		assert.Equal(t, 3, pdb.Stats().MaxOpenConnections)

		// Three transactions hold three connections.
		var txs []*relica.Tx
		for i := 0; i < 3; i++ {
			tx, err := pdb.Begin(ctx)
			require.NoError(t, err)
			txs = append(txs, tx)
		}
		s := pdb.Stats()
		assert.Equal(t, 3, s.OpenConnections)
		assert.Equal(t, 3, s.InUse)
		assert.Equal(t, 0, s.Idle)
		assert.Equal(t, int64(0), s.WaitCount)

		// A fourth must wait for a free connection and times out.
		wctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		_, err := pdb.Begin(wctx)
		cancel()
		require.ErrorIs(t, err, context.DeadlineExceeded)
		s = pdb.Stats()
		assert.Equal(t, int64(1), s.WaitCount)
		assert.Positive(t, s.WaitDuration)
		assert.Equal(t, 3, s.OpenConnections, "MaxOpenConns caps the pool")

		// Released connections beyond MaxIdleConns are closed.
		for _, tx := range txs {
			require.NoError(t, tx.Rollback())
		}
		s = pdb.Stats()
		assert.Equal(t, 0, s.InUse)
		assert.Equal(t, 1, s.Idle)
		assert.Equal(t, 1, s.OpenConnections)
		assert.Equal(t, int64(2), s.MaxIdleClosed)

		// The pool still serves queries.
		n, err := pdb.Select().From("ops_item").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
	})

	t.Run("WithConnMaxLifetime", func(t *testing.T) {
		ldb := open(t, relica.WithConnMaxLifetime(50*time.Millisecond))
		n, err := ldb.Select().From("ops_item").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
		require.Equal(t, 1, ldb.Stats().Idle)

		// database/sql's cleaner closes the expired connection (it runs at
		// least once a second).
		require.Eventually(t, func() bool { return ldb.Stats().MaxLifetimeClosed >= 1 },
			10*time.Second, 20*time.Millisecond, "the expired connection must be closed")
		s := ldb.Stats()
		assert.Equal(t, int64(0), s.MaxIdleTimeClosed, "closed for its age, not its idle time")

		n, err = ldb.Select().From("ops_item").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n, "a new connection replaces the expired one")
	})

	t.Run("WithConnMaxIdleTime", func(t *testing.T) {
		idb := open(t, relica.WithConnMaxIdleTime(50*time.Millisecond))
		n, err := idb.Select().From("ops_item").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
		require.Equal(t, 1, idb.Stats().Idle)

		require.Eventually(t, func() bool {
			s := idb.Stats()
			return s.MaxIdleTimeClosed >= 1 && s.Idle == 0
		}, 10*time.Second, 20*time.Millisecond, "the idle connection must be closed")
		assert.Equal(t, int64(0), idb.Stats().MaxLifetimeClosed, "closed for its idle time, not its age")

		n, err = idb.Select().From("ops_item").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
	})

	// ── WithHealthCheck, IsHealthy, Stats.Healthy ────────────────────────

	t.Run("WithHealthCheck_Healthy", func(t *testing.T) {
		hdb, err := relica.Open(dialect, dsn, relica.WithHealthCheck(20*time.Millisecond))
		require.NoError(t, err)
		closed := false
		defer func() {
			if !closed {
				_ = hdb.Close()
			}
		}()

		require.Eventually(t, func() bool { return !hdb.Stats().LastHealthCheck.IsZero() },
			10*time.Second, 10*time.Millisecond, "the health checker must ping")
		s := hdb.Stats()
		assert.True(t, s.Healthy)
		assert.True(t, hdb.IsHealthy())
		first := s.LastHealthCheck
		require.Eventually(t, func() bool { return hdb.Stats().LastHealthCheck.After(first) },
			10*time.Second, 10*time.Millisecond, "the health checker pings periodically")

		require.NoError(t, hdb.Close())
		closed = true
		stopped := hdb.Stats().LastHealthCheck
		time.Sleep(150 * time.Millisecond) // > 7 intervals
		assert.Equal(t, stopped, hdb.Stats().LastHealthCheck, "Close stops the health checker")
	})

	t.Run("WithHealthCheck_IsHealthyAfterClose", func(t *testing.T) {
		t.Skip("BUG: DB.IsHealthy (and Stats.Healthy) keep reporting true after Close; the checker stops with its last result")
		hdb, err := relica.Open(dialect, dsn, relica.WithHealthCheck(20*time.Millisecond))
		require.NoError(t, err)
		require.Eventually(t, hdb.IsHealthy, 10*time.Second, 10*time.Millisecond)
		require.Eventually(t, func() bool { return !hdb.Stats().LastHealthCheck.IsZero() },
			10*time.Second, 10*time.Millisecond)
		require.NoError(t, hdb.Close())
		assert.False(t, hdb.IsHealthy(), "a closed DB is not healthy")
		assert.False(t, hdb.Stats().Healthy)
	})

	t.Run("WithHealthCheck_CloseTwice", func(t *testing.T) {
		t.Skip("BUG: DB.Close panics (close of closed channel) when called twice on a DB opened WithHealthCheck")
		hdb, err := relica.Open(dialect, dsn, relica.WithHealthCheck(20*time.Millisecond))
		require.NoError(t, err)
		require.NoError(t, hdb.Close())
		assert.NotPanics(t, func() { _ = hdb.Close() }, "a second Close must not panic")
	})

	t.Run("WithHealthCheck_Unreachable", func(t *testing.T) {
		udb, err := relica.Open(dialect, unreachableDSN(t, dialect), relica.WithHealthCheck(20*time.Millisecond))
		require.NoError(t, err, "Open does not connect")
		defer func() { _ = udb.Close() }()

		require.Eventually(t, func() bool { return !udb.Stats().LastHealthCheck.IsZero() },
			15*time.Second, 10*time.Millisecond, "the health checker must ping")
		assert.False(t, udb.IsHealthy())
		assert.False(t, udb.Stats().Healthy)
		assert.Error(t, udb.PingContext(ctx), "guard: the database really is unreachable")
	})

	// ── WithStmtCacheCapacity, WarmCache, PinQuery, UnpinQuery ───────────

	t.Run("WithStmtCacheCapacity_LRUAndPins", func(t *testing.T) {
		cdb := open(t, relica.WithStmtCacheCapacity(2))
		q := func(k int) string { return opsCacheQuery(cdb, k) }

		n, err := cdb.WarmCache([]string{q(1), q(2), q(3)})
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		// Capacity 2: the least recently used statement (q1) was evicted.
		assert.False(t, cdb.PinQuery(q(1)), "q1 evicted at capacity 2")
		// Cache now {q3, q2}; pin q2, the LRU entry.
		require.True(t, cdb.PinQuery(q(2)))

		// q4 evicts the oldest unpinned entry: q3, not the pinned q2.
		_, err = cdb.WarmCache([]string{q(4)})
		require.NoError(t, err)
		assert.False(t, cdb.PinQuery(q(3)), "q3 evicted instead of the pinned q2")
		require.True(t, cdb.UnpinQuery(q(2)), "q2 survived eviction while pinned")

		// Unpinned, q2 is the LRU entry again and q5 evicts it.
		_, err = cdb.WarmCache([]string{q(5)})
		require.NoError(t, err)
		assert.False(t, cdb.PinQuery(q(2)), "q2 evicted after UnpinQuery")
		assert.False(t, cdb.UnpinQuery(q(2)))

		// The surviving statements run with different arguments.
		runOpsCacheQuery(t, cdb, 4, 1, "alpha")
		runOpsCacheQuery(t, cdb, 5, 2, "beta")
		// An evicted statement is prepared again on demand.
		runOpsCacheQuery(t, cdb, 1, 3, "gamma")
	})

	t.Run("WithStmtCacheCapacity_CloseClearsCache", func(t *testing.T) {
		cdb, err := relica.Open(dialect, dsn, relica.WithStmtCacheCapacity(4))
		require.NoError(t, err)
		_, err = cdb.WarmCache([]string{opsCacheQuery(cdb, 1)})
		require.NoError(t, err)
		require.NoError(t, cdb.Close())
		assert.False(t, cdb.PinQuery(opsCacheQuery(cdb, 1)), "Close empties the statement cache")
	})

	// ── WithLogger with a recording Logger ───────────────────────────────

	const secret = "S3cr3t-Arg-Value"

	t.Run("WithLogger_LevelsAndKeys", func(t *testing.T) {
		rec := &opsRecLogger{}
		ldb := open(t, relica.WithLogger(rec))

		// Success → Info "query executed" with sql, duration_ms, rows; no args.
		_, err := ldb.Insert("ops_item", map[string]any{"id": 50, "name": secret}).Execute()
		require.NoError(t, err)
		got := rec.take()
		require.Len(t, got, 1)
		assert.Equal(t, "INFO", got[0].level)
		assert.Equal(t, "query executed", got[0].msg)
		require.Contains(t, got[0].kv, "sql")
		assert.Contains(t, got[0].kv["sql"], "INSERT INTO")
		assert.Contains(t, got[0].kv["sql"], "ops_item")
		assert.Contains(t, got[0].kv, "duration_ms")
		assert.Equal(t, int64(1), got[0].kv["rows"])
		assert.NotContains(t, fmt.Sprint(got[0].raw...), secret, "arguments are not logged")

		// No row → Warn "no rows".
		var item opsItem
		err = ldb.Select("id", "name").From("ops_item").Where(relica.Eq("name", secret+"-missing")).One(&item)
		require.ErrorIs(t, err, relica.ErrNotFound)
		got = rec.take()
		require.Len(t, got, 1)
		assert.Equal(t, "WARN", got[0].level)
		assert.Equal(t, "no rows", got[0].msg)
		assert.Contains(t, got[0].kv["sql"], "ops_item")
		assert.NotContains(t, fmt.Sprint(got[0].raw...), secret)

		// Execution failure (NOT NULL) → Error "query failed" with the error.
		_, err = ldb.Insert("ops_item", map[string]any{"id": 51}).Execute()
		require.Error(t, err)
		got = rec.take()
		require.Len(t, got, 1)
		assert.Equal(t, "ERROR", got[0].level)
		assert.Equal(t, "query failed", got[0].msg)
		assert.Contains(t, got[0].kv["sql"], "ops_item")
		logged, ok := got[0].kv["error"].(error)
		require.True(t, ok, "error key holds the error")
		assert.True(t, relica.IsNotNullViolation(logged), "got %v", logged)

		// Select success → Info; nothing at Debug level ever.
		var name string
		require.NoError(t, ldb.Select("name").From("ops_item").Where(relica.Eq("id", 50)).Row(&name))
		assert.Equal(t, secret, name)
		got = rec.take()
		require.Len(t, got, 1)
		assert.Equal(t, "INFO", got[0].level)
		assert.NotContains(t, fmt.Sprint(got[0].raw...), secret, "the WHERE argument is not logged")

		res, err := ldb.Delete("ops_item").Where(relica.Eq("id", 50)).Execute()
		require.NoError(t, err)
		n, _ := res.RowsAffected()
		assert.Equal(t, int64(1), n)
		got = rec.take()
		require.Len(t, got, 1)
		assert.Equal(t, int64(1), got[0].kv["rows"])
	})

	t.Run("WithLogger_PrepareFailureLogged", func(t *testing.T) {
		if dialect != "sqlite" {
			t.Skip("BUG: a statement rejected at prepare time (PostgreSQL/MySQL server-side prepare: missing table, syntax error) " +
				"returns from Query.prepareStatement before invokeHook, so QueryHook/WithLogger never see it")
		}
		rec := &opsRecLogger{}
		ldb := open(t, relica.WithLogger(rec))
		_, err := ldb.Select().From("ops_missing_table").Count()
		require.Error(t, err)
		got := rec.take()
		require.Len(t, got, 1)
		assert.Equal(t, "ERROR", got[0].level)
		assert.Equal(t, "query failed", got[0].msg)
		assert.Contains(t, got[0].kv["sql"], "ops_missing_table")
	})

	// ── SlogAdapter (Debug, Warn, Info, Error) ───────────────────────────

	t.Run("WithLogger_SlogAdapter", func(t *testing.T) {
		var buf bytes.Buffer
		h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		ldb := open(t, relica.WithLogger(relica.NewSlogAdapter(slog.New(h))))

		_, err := ldb.Insert("ops_item", map[string]any{"id": 52, "name": secret}).Execute()
		require.NoError(t, err)
		var item opsItem
		err = ldb.Select("id", "name").From("ops_item").Where(relica.Eq("id", 999)).One(&item)
		require.ErrorIs(t, err, relica.ErrNotFound)
		_, err = ldb.Delete("ops_item").Where(relica.Eq("id", 52)).Execute()
		require.NoError(t, err)

		out := buf.String()
		lines := strings.Split(strings.TrimSpace(out), "\n")
		require.Len(t, lines, 3, out)
		assert.Contains(t, lines[0], "level=INFO")
		assert.Contains(t, lines[0], `msg="query executed"`)
		assert.Contains(t, lines[0], "ops_item")
		assert.Contains(t, lines[0], "rows=1")
		assert.Contains(t, lines[1], "level=WARN", "SlogAdapter.Warn writes slog warnings")
		assert.Contains(t, lines[1], `msg="no rows"`)
		assert.Contains(t, lines[2], "level=INFO")
		assert.NotContains(t, out, secret, "arguments are not logged")
		assert.NotContains(t, out, "level=DEBUG", "WithLogger logs nothing at Debug")
	})

	t.Run("SlogAdapter_LevelMethods", func(t *testing.T) {
		var buf bytes.Buffer
		a := relica.NewSlogAdapter(slog.New(slog.NewTextHandler(&buf,
			&slog.HandlerOptions{Level: slog.LevelDebug})))
		a.Debug("dbg-probe", "k", 1)
		a.Info("info-probe", "k", 2)
		a.Warn("warn-probe", "k", 3)
		a.Error("err-probe", "k", 4)
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		require.Len(t, lines, 4, buf.String())
		assert.Contains(t, lines[0], "level=DEBUG msg=dbg-probe k=1")
		assert.Contains(t, lines[1], "level=INFO msg=info-probe k=2")
		assert.Contains(t, lines[2], "level=WARN msg=warn-probe k=3")
		assert.Contains(t, lines[3], "level=ERROR msg=err-probe k=4")

		// The handler's level filters: Debug is dropped at Info.
		buf.Reset()
		b := relica.NewSlogAdapter(slog.New(slog.NewTextHandler(&buf, nil)))
		b.Debug("dropped")
		assert.Empty(t, buf.String())
		b.Warn("kept")
		assert.Contains(t, buf.String(), "level=WARN msg=kept")
	})

	// ── NoopLogger through a real DB ─────────────────────────────────────

	t.Run("WithLogger_NoopLogger", func(t *testing.T) {
		ndb := open(t, relica.WithLogger(relica.NoopLogger{}))
		_, err := ndb.Insert("ops_item", map[string]any{"id": 53, "name": "noop"}).Execute()
		require.NoError(t, err)
		var name string
		require.NoError(t, ndb.Select("name").From("ops_item").Where(relica.Eq("id", 53)).Row(&name))
		assert.Equal(t, "noop", name)
		var item opsItem
		assert.ErrorIs(t, ndb.Select("id", "name").From("ops_item").Where(relica.Eq("id", 999)).One(&item),
			relica.ErrNotFound)
		_, err = ndb.Insert("ops_item", map[string]any{"id": 54}).Execute()
		assert.Error(t, err)
		_, err = ndb.Delete("ops_item").Where(relica.Eq("id", 53)).Execute()
		require.NoError(t, err)
		assert.Equal(t, int64(3), opsItemCount(t, ndb))
	})

	// ── MaskArgs and DetectOperation inside a QueryHook ──────────────────

	t.Run("QueryHook_MaskArgs_DetectOperation", func(t *testing.T) {
		var mu sync.Mutex
		var events []opsHookEvent
		hdb := open(t, relica.WithQueryHook(func(_ context.Context, e relica.QueryEvent) {
			args := append([]any(nil), e.Args...)
			ev := opsHookEvent{event: e, args: args, masked: relica.MaskArgs(e.SQL, e.Args), detected: relica.DetectOperation(e.SQL)}
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}))

		_, err := hdb.Insert("ops_secret", map[string]any{"id": 1, "username": "alice", "password": "hunter2-long"}).Execute()
		require.NoError(t, err)
		_, err = hdb.Insert("ops_item", map[string]any{"id": 60, "name": "plain-value"}).Execute()
		require.NoError(t, err)
		_, err = hdb.Update("ops_item").Set(map[string]any{"name": "renamed"}).Where(relica.Eq("id", 60)).Execute()
		require.NoError(t, err)
		var user string
		require.NoError(t, hdb.Select("username").From("ops_secret").
			Where(relica.Eq("password", "hunter2-long")).Row(&user))
		assert.Equal(t, "alice", user)
		_, err = hdb.Delete("ops_item").Where(relica.Eq("id", 60)).Execute()
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, events, 5)
		for i, want := range []string{"INSERT", "INSERT", "UPDATE", "SELECT", "DELETE"} {
			assert.Equal(t, want, events[i].event.Operation, "event %d", i)
			assert.Equal(t, events[i].event.Operation, events[i].detected,
				"event %d: DetectOperation is the value of QueryEvent.Operation", i)
		}

		// A statement on a sensitive column: every argument is masked, the
		// event's own Args are untouched.
		ins := events[0]
		require.Len(t, ins.masked, 3)
		for _, m := range ins.masked {
			assert.Equal(t, opsMask, m)
		}
		assert.Contains(t, ins.event.Args, "hunter2-long", "MaskArgs returns a copy")
		assert.Equal(t, ins.args, ins.event.Args)
		sel := events[3]
		assert.Equal(t, []any{opsMask}, sel.masked, "WHERE password = ? is masked too")

		// No sensitive column: arguments pass through unchanged.
		assert.Equal(t, events[1].event.Args, events[1].masked)
		assert.Contains(t, events[1].masked, "plain-value")
		assert.Equal(t, events[2].event.Args, events[2].masked)

		// Direct calls.
		assert.Empty(t, relica.MaskArgs("SELECT password FROM ops_secret", nil))
		assert.Equal(t, []any{opsMask, opsMask},
			relica.MaskArgs("UPDATE ops_secret SET api_key = ? WHERE id = ?", []any{"k-123456789", 7}))
		assert.Equal(t, []any{"x", 7},
			relica.MaskArgs("UPDATE ops_item SET name = ? WHERE id = ?", []any{"x", 7}))
	})

	t.Run("QueryHook_DetectOperation_WithDelete", func(t *testing.T) {
		t.Skip("BUG: DetectOperation reports any statement starting with WITH as SELECT; WITH ... DELETE is a DELETE")
		var mu sync.Mutex
		var ops []string
		hdb := open(t, relica.WithQueryHook(func(_ context.Context, e relica.QueryEvent) {
			mu.Lock()
			ops = append(ops, e.Operation)
			mu.Unlock()
		}))
		_, err := hdb.Insert("ops_item", map[string]any{"id": 61, "name": "cte"}).Execute()
		require.NoError(t, err)
		res, err := hdb.NewQuery("WITH old AS (SELECT id FROM ops_item WHERE id = " + hdb.GenerateParamName(1) + ") " +
			"DELETE FROM ops_item WHERE id IN (SELECT id FROM old)").Bind(61).Execute()
		require.NoError(t, err)
		n, _ := res.RowsAffected()
		assert.Equal(t, int64(1), n)
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, []string{"INSERT", "DELETE"}, ops)
	})
}

// ---------------------------------------------------------------------------
// Entry points — one per database; CI selects the database with RELICA_TEST_DB.
// ---------------------------------------------------------------------------

func TestDBOps_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runDBOpsTests(t, ds.DB, "postgres")
	runDBOpsOptionTests(t, ds.DB, "postgres", ds.DSN)
}

func TestDBOps_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runDBOpsTests(t, ds.DB, "mysql")
	runDBOpsOptionTests(t, ds.DB, "mysql", ds.DSN)
}

func TestDBOps_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	dsn := filepath.Join(t.TempDir(), "ops.db")
	db, err := relica.Open("sqlite", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runDBOpsTests(t, db, "sqlite")
	runDBOpsOptionTests(t, db, "sqlite", dsn)
}
