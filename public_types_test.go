package relica_test

// Behavioral contract for the 20 public types that were type aliases to
// internal packages up to v0.18.0 (ADR-016). Each test drives the type only
// through the public API, so it must pass unchanged before and after the
// types became real declarations in package relica.

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coregx/relica"
)

// ─── Capture driver ──────────────────────────────────────────────────────────
// Records every executed statement with its arguments and every BeginTx option
// set, and returns a configurable result set. Opened via relica.WrapDB so the
// dialect is chosen independently of the driver name.

func init() { sql.Register("relica_capture", &captureDriver{}) }

type capturedQuery struct {
	SQL  string
	Args []driver.Value
}

type captureState struct {
	mu      sync.Mutex
	queries []capturedQuery
	txOpts  []driver.TxOptions
	columns []string
	rows    [][]driver.Value
}

func (s *captureState) record(query string, args []driver.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries = append(s.queries, capturedQuery{SQL: query, Args: append([]driver.Value(nil), args...)})
}

// last returns the most recent statement.
func (s *captureState) last(t *testing.T) capturedQuery {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queries) == 0 {
		t.Fatal("no statement was executed")
	}
	return s.queries[len(s.queries)-1]
}

var (
	captureStates sync.Map // dsn → *captureState
	captureSeq    atomic.Int64
)

type captureDriver struct{}

func (captureDriver) Open(dsn string) (driver.Conn, error) {
	st, ok := captureStates.Load(dsn)
	if !ok {
		return nil, fmt.Errorf("capture driver: unknown dsn %q", dsn)
	}
	return &captureConn{st: st.(*captureState)}, nil
}

type captureConn struct{ st *captureState }

func (c *captureConn) Prepare(query string) (driver.Stmt, error) {
	return &captureStmt{st: c.st, query: query}, nil
}
func (c *captureConn) Close() error              { return nil }
func (c *captureConn) Begin() (driver.Tx, error) { return captureTx{}, nil }
func (c *captureConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.st.mu.Lock()
	c.st.txOpts = append(c.st.txOpts, opts)
	c.st.mu.Unlock()
	return captureTx{}, nil
}

type captureTx struct{}

func (captureTx) Commit() error   { return nil }
func (captureTx) Rollback() error { return nil }

type captureStmt struct {
	st    *captureState
	query string
}

func (s *captureStmt) Close() error  { return nil }
func (s *captureStmt) NumInput() int { return -1 }
func (s *captureStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.st.record(s.query, args)
	return stubResult{}, nil
}
func (s *captureStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.st.record(s.query, args)
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	return &captureRows{columns: s.st.columns, rows: s.st.rows}, nil
}

type captureRows struct {
	columns []string
	rows    [][]driver.Value
	i       int
}

func (r *captureRows) Columns() []string { return r.columns }
func (r *captureRows) Close() error      { return nil }
func (r *captureRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}

// newCaptureDB returns a DB with the given dialect whose driver returns the
// given result set for every query.
func newCaptureDB(t *testing.T, dialect string, columns []string, rows ...[]driver.Value) (*relica.DB, *captureState) {
	t.Helper()
	dsn := fmt.Sprintf("capture-%d", captureSeq.Add(1))
	st := &captureState{columns: columns, rows: rows}
	captureStates.Store(dsn, st)
	sqlDB, err := sql.Open("relica_capture", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		captureStates.Delete(dsn)
	})
	return relica.WrapDB(sqlDB, dialect), st
}

func requireSQL(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("SQL mismatch\n got: %s\nwant: %s", got, want)
	}
}

func requireArgs(t *testing.T, got []any, want ...any) {
	t.Helper()
	if len(want) == 0 && len(got) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

// ─── HashExp ─────────────────────────────────────────────────────────────────

func TestHashExp_AllConditionEntryPoints(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	cond := relica.HashExp{"status": 1, "deleted_at": nil}
	const want = `"deleted_at" IS NULL AND "status" = $`

	cases := map[string]func() (string, []any){
		"Select.Where":    func() (string, []any) { return db.Select().From("item").Where(cond).ToSQL() },
		"Select.AndWhere": func() (string, []any) { return db.Select().From("item").AndWhere(cond).ToSQL() },
		"Select.OrWhere":  func() (string, []any) { return db.Select().From("item").OrWhere(cond).ToSQL() },
		"Update.Where": func() (string, []any) {
			return db.Update("item").Set(map[string]any{"name": "x"}).Where(cond).ToSQL()
		},
		"Update.AndWhere": func() (string, []any) {
			return db.Update("item").Set(map[string]any{"name": "x"}).AndWhere(cond).ToSQL()
		},
		"Update.OrWhere": func() (string, []any) {
			return db.Update("item").Set(map[string]any{"name": "x"}).OrWhere(cond).ToSQL()
		},
		"Delete.Where":    func() (string, []any) { return db.Delete("item").Where(cond).ToSQL() },
		"Delete.AndWhere": func() (string, []any) { return db.Delete("item").AndWhere(cond).ToSQL() },
		"Delete.OrWhere":  func() (string, []any) { return db.Delete("item").OrWhere(cond).ToSQL() },
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			sqlStr, args := build()
			if !strings.Contains(sqlStr, want) {
				t.Errorf("SQL %q does not contain %q", sqlStr, want)
			}
			if len(args) == 0 || args[len(args)-1] != 1 {
				t.Errorf("last arg = %v, want status value 1", args)
			}
		})
	}
}

func TestHashExp_JoinAndHaving(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)

	sqlStr, args := db.Select("u.id").From("users u").
		InnerJoin("orders o", relica.And(relica.EqCol("o.user_id", "u.id"), relica.HashExp{"o.state": "paid"})).
		GroupBy("u.id").
		Having(relica.HashExp{"u.id": 7}).
		ToSQL()
	assertContains(t, sqlStr, `INNER JOIN "orders" AS "o" ON ("o"."user_id" = "u"."id") AND ("o"."state" = $1)`)
	assertContains(t, sqlStr, `HAVING "u"."id" = $2`)
	requireArgs(t, args, "paid", 7)
}

func TestHashExp_AsExpressionArgument(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)

	var e relica.Expression = relica.HashExp{"a": 1}
	sqlStr, args := db.Select().From("t").Where(relica.Not(e)).ToSQL()
	requireSQL(t, sqlStr, `SELECT * FROM "t" WHERE NOT ("a" = $1)`)
	requireArgs(t, args, 1)

	sqlStr, args = db.Select().From("t").
		Where(relica.Or(relica.HashExp{"a": 1}, relica.HashExp{"b": []any{2, 3}})).ToSQL()
	requireSQL(t, sqlStr, `SELECT * FROM "t" WHERE ("a" = $1) OR ("b" IN ($2, $3))`)
	requireArgs(t, args, 1, 2, 3)
}

func TestHashExp_NestedExpressionValue(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	sub := db.Select("id").From("vip")

	sqlStr, args := db.Select().From("t").
		Where(relica.HashExp{"id": relica.In("id", sub.AsExpression())}).ToSQL()
	assertContains(t, sqlStr, `("id" IN (SELECT "id" FROM "vip"))`)
	requireArgs(t, args)
}

func TestHashExp_IsAMapLiteral(t *testing.T) {
	h := relica.HashExp{"a": 1}
	h["b"] = 2
	if len(h) != 2 {
		t.Fatalf("len = %d, want 2", len(h))
	}
}

// ─── Params ──────────────────────────────────────────────────────────────────

func TestParams_NamedPlaceholdersInConditions(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	p := relica.Params{"id": 7, "state": "on"}
	const cond = "id = {:id} AND state = {:state} OR parent_id = {:id}"

	cases := map[string]func() (string, []any){
		"Select.Where":    func() (string, []any) { return db.Select().From("t").Where(cond, p).ToSQL() },
		"Select.AndWhere": func() (string, []any) { return db.Select().From("t").AndWhere(cond, p).ToSQL() },
		"Select.OrWhere":  func() (string, []any) { return db.Select().From("t").OrWhere(cond, p).ToSQL() },
		"Update.Where": func() (string, []any) {
			return db.Update("t").Set(map[string]any{"name": "n"}).Where(cond, p).ToSQL()
		},
		"Delete.Where": func() (string, []any) { return db.Delete("t").Where(cond, p).ToSQL() },
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			sqlStr, args := build()
			if strings.Contains(sqlStr, "{:") {
				t.Fatalf("named placeholder left unresolved: %s", sqlStr)
			}
			n := len(args)
			if n < 3 || args[n-3] != 7 || args[n-2] != "on" || args[n-1] != 7 {
				t.Errorf("args = %v, want ... 7 on 7", args)
			}
		})
	}
}

func TestParams_MissingNameIsAnError(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	err := db.Select().From("t").Where("id = {:id}", relica.Params{"other": 1}).One(&struct{}{})
	if err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("err = %v, want missing parameter error naming id", err)
	}
}

func TestParams_BindParamsOnRawQuery(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", []string{"id"}, []driver.Value{int64(1)})

	var id int64
	err := db.NewQuery("SELECT id FROM t WHERE id = {:id} AND kind = {:kind}").
		BindParams(relica.Params{"id": 5, "kind": "a"}).
		Row(&id)
	if err != nil {
		t.Fatal(err)
	}
	q := st.last(t)
	requireSQL(t, q.SQL, "SELECT id FROM t WHERE id = $1 AND kind = $2")
	if !reflect.DeepEqual(q.Args, []driver.Value{int64(5), "a"}) {
		t.Errorf("driver args = %#v", q.Args)
	}
}

// ─── NullStringMap ───────────────────────────────────────────────────────────

func nullStringMapDB(t *testing.T) *relica.DB {
	t.Helper()
	db, _ := newCaptureDB(t, "postgres", []string{"id", "name", "email"},
		[]driver.Value{int64(1), "alice", nil},
		[]driver.Value{int64(2), "bob", "bob@example.com"},
	)
	return db
}

func checkAlice(t *testing.T, m relica.NullStringMap) {
	t.Helper()
	if got := m.String("name"); got != "alice" {
		t.Errorf("String(name) = %q", got)
	}
	if !m.IsNull("email") {
		t.Error("IsNull(email) = false, want true")
	}
	if m.IsNull("name") {
		t.Error("IsNull(name) = true")
	}
	if !m.Has("id") || m.Has("missing") {
		t.Error("Has() wrong")
	}
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"email", "id", "name"}) {
		t.Errorf("Keys() = %v", got)
	}
	if v, ok := m.Get("id"); !ok || v.String != "1" || !v.Valid {
		t.Errorf("Get(id) = %+v, %v", v, ok)
	}
}

func TestNullStringMap_One(t *testing.T) {
	db := nullStringMapDB(t)
	var m relica.NullStringMap
	if err := db.Select().From("users").One(&m); err != nil {
		t.Fatal(err)
	}
	checkAlice(t, m)
}

func TestNullStringMap_All(t *testing.T) {
	db := nullStringMapDB(t)
	var ms []relica.NullStringMap
	if err := db.Select().From("users").All(&ms); err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 {
		t.Fatalf("len = %d, want 2", len(ms))
	}
	checkAlice(t, ms[0])
	if ms[1].String("email") != "bob@example.com" {
		t.Errorf("row 2 email = %q", ms[1].String("email"))
	}
}

func TestNullStringMap_RawQuery(t *testing.T) {
	db := nullStringMapDB(t)
	var m relica.NullStringMap
	if err := db.NewQuery("SELECT * FROM users").One(&m); err != nil {
		t.Fatal(err)
	}
	checkAlice(t, m)

	var ms []relica.NullStringMap
	if err := db.NewQuery("SELECT * FROM users").All(&ms); err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 {
		t.Fatalf("len = %d, want 2", len(ms))
	}
}

func TestNullStringMap_Generic(t *testing.T) {
	db := nullStringMapDB(t)
	m, err := relica.One[relica.NullStringMap](db.Select().From("users"))
	if err != nil {
		t.Fatal(err)
	}
	checkAlice(t, m)

	ms, err := relica.All[relica.NullStringMap](db.Select().From("users"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 {
		t.Fatalf("len = %d, want 2", len(ms))
	}
}

func TestNullStringMap_NotFound(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", []string{"id"})
	var m relica.NullStringMap
	err := db.Select().From("users").One(&m)
	if !errors.Is(err, relica.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ─── Expression values inside maps and nested arguments ─────────────────────

func TestExpression_InUpdateSetMap(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	sqlStr, args := db.Update("account").Set(map[string]any{
		"balance": relica.NewExp("balance + ?", 10),
		"label":   relica.Coalesce("nickname", "name"),
		"tier":    relica.CaseWhen().When(relica.GreaterThan("balance", 100), "gold").Else("basic"),
	}).Where(relica.Eq("id", 1)).ToSQL()
	requireSQL(t, sqlStr, `UPDATE "account" SET "balance" = balance + $1, "label" = COALESCE("nickname", "name"), `+
		`"tier" = CASE WHEN "balance" > $2 THEN $3 ELSE $4 END WHERE "id" = $5`)
	requireArgs(t, args, 10, 100, "gold", "basic", 1)
}

func TestExpression_NestedInFunctionalExpressions(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	sqlStr, args := db.Select("id").
		SelectSub(relica.Coalesce(relica.NullIf("email", "backup_email"), relica.Concat("first", "last")), "contact").
		SelectSub(relica.Case("state").When(1, relica.Greatest("a", "b")).Else(relica.Least("a", "b")), "pick").
		From("person").ToSQL()
	requireSQL(t, sqlStr, `SELECT "id", (COALESCE(NULLIF("email", "backup_email"), "first" || "last")) AS "contact", `+
		`(CASE "state" WHEN $1 THEN GREATEST("a", "b") ELSE LEAST("a", "b") END) AS "pick" FROM "person"`)
	requireArgs(t, args, 1)
}

func TestExpression_ConstructorValueArguments(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	sqlStr, args := db.Select().From("t").Where(relica.Eq("total", relica.Coalesce("price", 0))).ToSQL()
	requireSQL(t, sqlStr, `SELECT * FROM "t" WHERE "total" = (COALESCE("price", $1))`)
	requireArgs(t, args, 0)
}

// ─── Functional expression builders ─────────────────────────────────────────

func TestCaseExp_Builders(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	c := relica.Case("status")
	c = c.When(1, "active").When(2, "banned").Else("unknown").As("label")
	sqlStr, args := db.Select("id").SelectSub(c, "x").From("u").ToSQL()
	requireSQL(t, sqlStr, `SELECT "id", (CASE "status" WHEN $1 THEN $2 WHEN $3 THEN $4 ELSE $5 END AS "label") AS "x" FROM "u"`)
	requireArgs(t, args, 1, "active", 2, "banned", "unknown")
}

func TestFunctionalExp_AsAliases(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	co := relica.Coalesce("a", "b").As("c")
	ni := relica.NullIf("a", "b").As("n")
	gl := relica.Greatest("a", "b").As("g")
	cc := relica.Concat("a", "b").As("s")
	for _, tc := range []struct {
		name string
		exp  relica.Expression
		want string
	}{
		{"Coalesce", co, `COALESCE("a", "b") AS "c"`},
		{"NullIf", ni, `NULLIF("a", "b") AS "n"`},
		{"Greatest", gl, `GREATEST("a", "b") AS "g"`},
		{"Concat", cc, `"a" || "b" AS "s"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlStr, _ := db.Select().SelectSub(tc.exp, "v").From("t").ToSQL()
			assertContains(t, sqlStr, tc.want)
		})
	}
}

func TestFunctionalExp_DialectSpecificRendering(t *testing.T) {
	sqlite, _ := newCaptureDB(t, "sqlite", nil)
	mysql, _ := newCaptureDB(t, "mysql", nil)

	s, _ := sqlite.Select().SelectSub(relica.Greatest("a", "b"), "g").SelectSub(relica.Least("a", "b"), "l").From("t").ToSQL()
	assertContains(t, s, `(MAX("a", "b")) AS "g"`)
	assertContains(t, s, `(MIN("a", "b")) AS "l"`)

	m, _ := mysql.Select().SelectSub(relica.Concat("a", "b"), "s").From("t").ToSQL()
	assertContains(t, m, "(CONCAT(`a`, `b`)) AS `s`")
}

func TestLikeExp_Builders(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)

	l := relica.Like("name", "50%_off").Match(true, false)
	sqlStr, args := db.Select().From("t").Where(l).ToSQL()
	requireSQL(t, sqlStr, `SELECT * FROM "t" WHERE "name" LIKE $1`)
	requireArgs(t, args, `%50\%\_off`)
	if err := l.Err(); err != nil {
		t.Errorf("Err() = %v", err)
	}

	custom := relica.Like("code", "a!b").EscapeChars("!", "!!")
	_, args = db.Select().From("t").Where(custom).ToSQL()
	requireArgs(t, args, "%a!!b%")

	bad := relica.Like("code", "x").EscapeChars("!")
	if bad.Err() == nil {
		t.Error("odd EscapeChars must set Err()")
	}

	for name, exp := range map[string]relica.Expression{
		"NotLike":   relica.NotLike("n", "a"),
		"OrLike":    relica.OrLike("n", "a", "b"),
		"OrNotLike": relica.OrNotLike("n", "a", "b"),
	} {
		t.Run(name, func(t *testing.T) {
			sqlStr, _ := db.Select().From("t").Where(exp).ToSQL()
			assertContains(t, sqlStr, "LIKE")
		})
	}
}

// ILike family: case-insensitive LIKE, lost in v0.19.0 when LikeExp stopped
// exposing its Like field (v0.18 callers wrote exp.Like = "ILIKE").
func TestILike_Builders(t *testing.T) {
	pg, _ := newCaptureDB(t, "postgres", nil)
	my, _ := newCaptureDB(t, "mysql", nil)
	lite, _ := newCaptureDB(t, "sqlite", nil)

	var l relica.LikeExp = relica.ILike("h.cpu_model", "Xeon 50%").Match(true, false)
	sqlStr, args := pg.Select("h.id").From("host h").Where(l).ToSQL()
	requireSQL(t, sqlStr, `SELECT "h"."id" FROM "host" AS "h" WHERE "h"."cpu_model" ILIKE $1`)
	requireArgs(t, args, `%Xeon 50\%`)

	sqlStr, _ = my.Select("h.id").From("host h").Where(relica.ILike("h.cpu_model", "xeon")).ToSQL()
	requireSQL(t, sqlStr, "SELECT `h`.`id` FROM `host` AS `h` WHERE LOWER(`h`.`cpu_model`) LIKE LOWER(?)")

	sqlStr, _ = lite.Select("id").From("host").Where(relica.NotILike("cpu_model", "xeon")).ToSQL()
	requireSQL(t, sqlStr, `SELECT "id" FROM "host" WHERE LOWER("cpu_model") NOT LIKE LOWER(?) ESCAPE '\'`)

	for name, tc := range map[string]struct {
		exp  relica.LikeExp
		want string
	}{
		"NotILike":   {relica.NotILike("n", "a"), `SELECT * FROM "t" WHERE "n" NOT ILIKE $1`},
		"OrILike":    {relica.OrILike("n", "a", "b"), `SELECT * FROM "t" WHERE "n" ILIKE $1 OR "n" ILIKE $2`},
		"OrNotILike": {relica.OrNotILike("n", "a", "b"), `SELECT * FROM "t" WHERE "n" NOT ILIKE $1 OR "n" NOT ILIKE $2`},
	} {
		t.Run(name, func(t *testing.T) {
			sqlStr, _ := pg.Select().From("t").Where(tc.exp).ToSQL()
			requireSQL(t, sqlStr, tc.want)
		})
	}

	// Negative: an invalid ILike is a query error, never a dropped filter.
	bad := relica.ILike("name", "x").EscapeChars("!")
	if bad.Err() == nil {
		t.Fatal("odd EscapeChars must set Err()")
	}
	if _, err := pg.Delete("t").Where(bad).Execute(); err == nil {
		t.Error("Delete with an invalid ILike must fail instead of deleting every row")
	}
}

// ─── QueryHook / QueryEvent ──────────────────────────────────────────────────

func TestQueryHook_ReceivesQueryEvent(t *testing.T) {
	var mu sync.Mutex
	var events []relica.QueryEvent
	var hook relica.QueryHook = func(_ context.Context, e relica.QueryEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}
	var second atomic.Int32
	db, err := relica.Open("sqlite3", "hook",
		relica.WithQueryHook(hook),
		relica.WithQueryHook(func(context.Context, relica.QueryEvent) { second.Add(1) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Update("t").Set(map[string]any{"a": 1}).Where(relica.Eq("id", 2)).Execute(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Operation != "UPDATE" || !strings.HasPrefix(e.SQL, `UPDATE "t" SET`) || e.Error != nil ||
		e.RowsAffected != 1 || e.Duration < 0 || !reflect.DeepEqual(e.Args, []any{1, 2}) {
		t.Errorf("event = %+v", e)
	}
	if second.Load() != 1 {
		t.Errorf("second hook calls = %d, want 1 (hooks compose)", second.Load())
	}
}

func TestQueryHook_NilIsNoop(t *testing.T) {
	db, err := relica.Open("sqlite3", "nilhook", relica.WithQueryHook(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Delete("t").Where(relica.Eq("id", 1)).Execute(); err != nil {
		t.Fatal(err)
	}
}

// ─── TxOptions ───────────────────────────────────────────────────────────────

func TestTxOptions_ReachDriver(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", nil)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, &relica.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()

	err = db.TransactionalTx(ctx, &relica.TxOptions{Isolation: sql.LevelReadCommitted}, func(*relica.Tx) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	want := []driver.TxOptions{
		{Isolation: driver.IsolationLevel(sql.LevelSerializable), ReadOnly: true},
		{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)},
	}
	if !reflect.DeepEqual(st.txOpts, want) {
		t.Errorf("driver TxOptions = %+v, want %+v", st.txOpts, want)
	}
}

func TestTxOptions_NilMeansDefaults(t *testing.T) {
	db, st := newCaptureDB(t, "postgres", nil)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.txOpts) != 1 || st.txOpts[0] != (driver.TxOptions{}) {
		t.Errorf("driver TxOptions = %+v, want one default set", st.txOpts)
	}
}

// ─── PoolStats and Option ────────────────────────────────────────────────────

func TestOption_AppliesAndPoolStatsReports(t *testing.T) {
	db, err := relica.Open("sqlite3", "opts",
		relica.WithMaxOpenConns(7),
		relica.WithMaxIdleConns(3),
		relica.WithConnMaxLifetime(time.Minute),
		relica.WithConnMaxIdleTime(time.Minute),
		relica.WithStmtCacheCapacity(16),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var stats relica.PoolStats = db.Stats()
	if stats.MaxOpenConnections != 7 {
		t.Errorf("MaxOpenConnections = %d, want 7", stats.MaxOpenConnections)
	}
	if stats.InUse < 0 || stats.Idle < 0 || stats.OpenConnections < stats.InUse {
		t.Errorf("inconsistent stats %+v", stats)
	}
}

func TestOption_ZeroValueIsNoop(t *testing.T) {
	var zero relica.Option
	db, err := relica.Open("sqlite3", "zero", zero, relica.WithMaxOpenConns(2))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if got := db.Stats().MaxOpenConnections; got != 2 {
		t.Errorf("MaxOpenConnections = %d, want 2", got)
	}
}

// ─── Logger, NoopLogger, SlogAdapter ────────────────────────────────────────

type recordingLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *recordingLogger) add(level, msg string) {
	l.mu.Lock()
	l.lines = append(l.lines, level+" "+msg)
	l.mu.Unlock()
}
func (l *recordingLogger) Debug(msg string, _ ...any) { l.add("DEBUG", msg) }
func (l *recordingLogger) Info(msg string, _ ...any)  { l.add("INFO", msg) }
func (l *recordingLogger) Warn(msg string, _ ...any)  { l.add("WARN", msg) }
func (l *recordingLogger) Error(msg string, _ ...any) { l.add("ERROR", msg) }

var (
	_ relica.Logger = (*recordingLogger)(nil)
	_ relica.Logger = (*relica.NoopLogger)(nil)
	_ relica.Logger = (*relica.SlogAdapter)(nil)
)

func TestLogger_CustomImplementationReceivesQueries(t *testing.T) {
	l := &recordingLogger{}
	db, err := relica.Open("sqlite3", "logger", relica.WithLogger(l))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Delete("t").Where(relica.Eq("id", 1)).Execute(); err != nil {
		t.Fatal(err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.lines) == 0 {
		t.Fatal("logger received nothing")
	}
}

func TestSlogAdapter_WritesThroughSlog(t *testing.T) {
	var buf bytes.Buffer
	a := relica.NewSlogAdapter(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	a.Debug("d", "k", 1)
	a.Info("i")
	a.Warn("w")
	a.Error("e")
	out := buf.String()
	for _, want := range []string{"level=DEBUG msg=d k=1", "level=INFO msg=i", "level=WARN msg=w", "level=ERROR msg=e"} {
		assertContains(t, out, want)
	}
}

func TestNoopLogger_Discards(t *testing.T) {
	var l relica.Logger = &relica.NoopLogger{}
	l.Debug("x")
	l.Info("x")
	l.Warn("x")
	l.Error("x")
}

// ─── BeforeInserter ──────────────────────────────────────────────────────────

type stampedRow struct {
	ID    int64  `db:"id,pk"`
	Stamp string `db:"stamp"`
}

func (stampedRow) TableName() string { return "stamped" }

func (r *stampedRow) BeforeInsert() error {
	r.Stamp = "set-by-hook"
	return nil
}

var _ relica.BeforeInserter = (*stampedRow)(nil)

type rejectingRow struct {
	ID int64 `db:"id,pk"`
}

func (rejectingRow) TableName() string { return "rejecting" }

var errRejected = errors.New("rejected by BeforeInsert")

func (*rejectingRow) BeforeInsert() error { return errRejected }

func TestBeforeInserter_RunsBeforeInsert(t *testing.T) {
	// SQLite inserts use RETURNING for the primary key, so the driver returns it.
	db, st := newCaptureDB(t, "sqlite", []string{"id"}, []driver.Value{int64(1)})
	row := stampedRow{}
	if err := db.Model(&row).Insert(); err != nil {
		t.Fatal(err)
	}
	if row.Stamp != "set-by-hook" {
		t.Errorf("Stamp = %q", row.Stamp)
	}
	q := st.last(t)
	if !reflect.DeepEqual(q.Args, []driver.Value{"set-by-hook"}) {
		t.Errorf("insert args = %#v, want the hook-set value", q.Args)
	}
}

func TestBeforeInserter_ErrorAbortsInsert(t *testing.T) {
	db, st := newCaptureDB(t, "sqlite", nil)
	err := db.Model(&rejectingRow{}).Insert()
	if !errors.Is(err, errRejected) {
		t.Fatalf("err = %v, want errRejected", err)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.queries) != 0 {
		t.Errorf("statements executed after BeforeInsert failed: %v", st.queries)
	}
}

// ─── QueryPlan ───────────────────────────────────────────────────────────────

// QueryPlan is produced by EXPLAIN on a real database; the integration suite
// covers the values. Here: the public type carries the documented fields.
func TestQueryPlan_Fields(t *testing.T) {
	p := relica.QueryPlan{
		Cost: 1.5, EstimatedRows: 10, ActualRows: 9, ActualTime: time.Millisecond,
		UsesIndex: true, IndexName: "ix", FullScan: false, RawOutput: "raw", Database: "postgres",
		BuffersHit: 1, BuffersMiss: 2, RowsExamined: 3, RowsProduced: 4,
	}
	if p.IndexName != "ix" || p.Database != "postgres" {
		t.Errorf("%+v", p)
	}
}

func TestParams_NamedPlaceholdersInHaving(t *testing.T) {
	db, _ := newCaptureDB(t, "postgres", nil)
	sqlStr, args := db.Select("dept").From("staff").GroupBy("dept").
		Having("COUNT(*) > {:min} AND MAX(age) < {:max}", relica.Params{"min": 5, "max": 60}).
		ToSQL()
	requireSQL(t, sqlStr, `SELECT "dept" FROM "staff" GROUP BY "dept" HAVING COUNT(*) > $1 AND MAX(age) < $2`)
	requireArgs(t, args, 5, 60)
}

// TestPublicTypes_DeclarableByName: callers can name every type in variable,
// field and parameter declarations — the v0.19.0 shapes (builders are
// interfaces, not pointers to internal structs).
func TestPublicTypes_DeclarableByName(t *testing.T) {
	var (
		e  relica.Expression       = relica.Eq("a", 1)
		h  relica.HashExp          = relica.HashExp{"a": 1}
		l  relica.LikeExp          = relica.Like("n", "x")
		c  relica.CaseExp          = relica.Case("s").When(1, "a")
		co relica.CoalesceExp      = relica.Coalesce("a", "b")
		ni relica.NullIfExp        = relica.NullIf("a", "b")
		gl relica.GreatestLeastExp = relica.Greatest("a", "b")
		cc relica.ConcatExp        = relica.Concat("a", "b")
		p  relica.Params           = relica.Params{"id": 1}
		m  relica.NullStringMap    = relica.NullStringMap{}
		o  relica.Option           = relica.WithMaxOpenConns(1)
		lg relica.Logger           = relica.NoopLogger{}
		hk relica.QueryHook        = func(context.Context, relica.QueryEvent) {}
		to                         = relica.TxOptions{ReadOnly: true}
		ps relica.PoolStats
		qp relica.QueryPlan
	)
	for _, x := range []relica.Expression{e, h, l, c, co, ni, gl, cc} {
		if x == nil {
			t.Fatal("nil expression")
		}
	}
	for _, v := range []any{p, m, o, lg, hk, to, ps, qp} {
		_ = v
	}
}
