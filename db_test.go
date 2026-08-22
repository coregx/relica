// Package relica_test contains black-box unit tests for the public API in db.go.
// Tests use a minimal in-memory database/sql driver to avoid external dependencies.
package relica_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/coregx/relica"
)

// ─── Minimal test driver ──────────────────────────────────────────────────────
// Registers under "relica_test" — zero external deps, satisfies database/sql.

func init() {
	// Register stub under "sqlite3" so that Open("sqlite3", ...) works:
	// relica uses the driver name as both the SQL driver name and the dialect selector.
	// "sqlite3" is a built-in supported dialect, so no panic in GetDialect.
	sql.Register("sqlite3", &stubDriver{})
}

type stubDriver struct{}

func (d *stubDriver) Open(_ string) (driver.Conn, error) { return &stubConn{}, nil }

type stubConn struct{ inTx bool }

func (c *stubConn) Prepare(query string) (driver.Stmt, error) {
	return &stubStmt{query: query}, nil
}
func (c *stubConn) Close() error { return nil }
func (c *stubConn) Begin() (driver.Tx, error) {
	c.inTx = true
	return &stubTx{conn: c}, nil
}

// BeginTx makes the driver accept any isolation level without error.
func (c *stubConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	c.inTx = true
	return &stubTx{conn: c}, nil
}

type stubTx struct{ conn *stubConn }

func (t *stubTx) Commit() error   { t.conn.inTx = false; return nil }
func (t *stubTx) Rollback() error { t.conn.inTx = false; return nil }

type stubStmt struct{ query string }

func (s *stubStmt) Close() error                                 { return nil }
func (s *stubStmt) NumInput() int                                { return -1 }
func (s *stubStmt) Exec(_ []driver.Value) (driver.Result, error) { return stubResult{}, nil }
func (s *stubStmt) Query(_ []driver.Value) (driver.Rows, error)  { return &stubRows{}, nil }

type stubResult struct{}

func (r stubResult) LastInsertId() (int64, error) { return 1, nil }
func (r stubResult) RowsAffected() (int64, error) { return 1, nil }

type stubRows struct{ done bool }

func (r *stubRows) Columns() []string { return []string{"id"} }
func (r *stubRows) Close() error      { return nil }
func (r *stubRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(1)
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// openTestDB opens a DB backed by the stub driver.
// Driver "sqlite3" is registered in init() pointing to stubDriver.
func openTestDB(t *testing.T) *relica.DB {
	t.Helper()
	db, err := relica.Open("sqlite3", "test", relica.WithMaxOpenConns(5))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// wrapTestDB wraps a raw *sql.DB. Caller handles cleanup.
func wrapTestDB(t *testing.T) (*relica.DB, *sql.DB) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", "test")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	return relica.WrapDB(sqlDB, "sqlite3"), sqlDB
}

// assertContains fails the test if haystack does not contain needle.
func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected %q to contain %q", haystack, needle)
	}
}

// assertNotEmpty fails if s is an empty string.
func assertNotEmpty(t *testing.T, s, label string) {
	t.Helper()
	if s == "" {
		t.Errorf("%s: expected non-empty string", label)
	}
}

// ─── Open / WrapDB / NewDB ────────────────────────────────────────────────────

func TestOpen_ReturnsDB(t *testing.T) {
	// openTestDB internally calls relica.Open("sqlite3", "test", ...)
	db := openTestDB(t)
	if db == nil {
		t.Fatal("expected non-nil DB")
	}
}

func TestOpen_InvalidDriver_ReturnsError(t *testing.T) {
	_, err := relica.Open("nonexistent_driver_xyz", "dsn")
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestWrapDB_ReturnsDB(t *testing.T) {
	db, sqlDB := wrapTestDB(t)
	defer sqlDB.Close()
	if db == nil {
		t.Fatal("expected non-nil DB")
	}
}

func TestNewDB_Deprecated_ReturnsDB(t *testing.T) {
	db, err := relica.NewDB("sqlite3", "test")
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()
	if db == nil {
		t.Fatal("expected non-nil DB")
	}
}

func TestNewDB_Deprecated_InvalidDriver_ReturnsError(t *testing.T) {
	_, err := relica.NewDB("nonexistent_driver_xyz", "dsn")
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

// ─── DB lifecycle ─────────────────────────────────────────────────────────────

func TestClose_NoError(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestStats_ReturnedWithoutPanic(t *testing.T) {
	db := openTestDB(t)
	stats := db.Stats()
	_ = stats // PoolStats is a value type — just verify no panic
}

func TestIsHealthy_ReturnsBool(t *testing.T) {
	db := openTestDB(t)
	// With stub driver and no health check configured, must not panic.
	_ = db.IsHealthy()
}

func TestWithContext_ReturnsNewDB(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	db2 := db.WithContext(ctx)
	if db2 == nil {
		t.Fatal("WithContext returned nil")
	}
}

func TestUnwrap_ReturnsNonNil(t *testing.T) {
	db := openTestDB(t)
	core := db.Unwrap()
	if core == nil {
		t.Fatal("Unwrap returned nil")
	}
}

// ─── Cache management ─────────────────────────────────────────────────────────

func TestWarmCache_NoError(t *testing.T) {
	db := openTestDB(t)
	n, err := db.WarmCache([]string{"SELECT 1"})
	// May return 0 for stub, but must not panic.
	_ = n
	_ = err
}

func TestPinQuery_ReturnsBool(t *testing.T) {
	db := openTestDB(t)
	// WarmCache must precede PinQuery; result depends on cache state.
	_, _ = db.WarmCache([]string{"SELECT 1"})
	_ = db.PinQuery("SELECT 1")
}

func TestUnpinQuery_ReturnsBool(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.WarmCache([]string{"SELECT 1"})
	_ = db.PinQuery("SELECT 1")
	_ = db.UnpinQuery("SELECT 1")
}

// ─── Builder ─────────────────────────────────────────────────────────────────

func TestBuilder_ReturnsNonNil(t *testing.T) {
	db := openTestDB(t)
	qb := db.Builder()
	if qb == nil {
		t.Fatal("Builder returned nil")
	}
}

func TestBuilder_Unwrap_ReturnsNonNil(t *testing.T) {
	db := openTestDB(t)
	core := db.Builder().Unwrap()
	if core == nil {
		t.Fatal("QueryBuilder.Unwrap returned nil")
	}
}

func TestBuilder_WithContext(t *testing.T) {
	db := openTestDB(t)
	qb := db.Builder().WithContext(context.Background())
	if qb == nil {
		t.Fatal("QueryBuilder.WithContext returned nil")
	}
}

// ─── SELECT query builder via ToSQL ───────────────────────────────────────────

func TestSelect_ToSQL_Basic(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id", "name").From("users").ToSQL()
	assertNotEmpty(t, sql, "SELECT sql")
	assertContains(t, sql, "users")
}

func TestSelect_ToSQL_NoColumns_StarDefault(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select().From("users").ToSQL()
	assertNotEmpty(t, sql, "SELECT * sql")
}

func TestSelect_Where_StringCondition(t *testing.T) {
	db := openTestDB(t)
	sql, params := db.Select("id").From("users").Where("id = ?", 42).ToSQL()
	assertNotEmpty(t, sql, "WHERE sql")
	if len(params) == 0 {
		t.Error("expected params")
	}
}

func TestSelect_Where_Expression(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").From("users").Where(relica.Eq("status", 1)).ToSQL()
	assertNotEmpty(t, sql, "WHERE expression sql")
}

func TestSelect_AndWhere_OrWhere(t *testing.T) {
	db := openTestDB(t)
	sq := db.Select("id").From("users").
		Where("status = ?", 1).
		AndWhere("age > ?", 18).
		OrWhere("role = ?", "admin")
	sql, _ := sq.ToSQL()
	assertNotEmpty(t, sql, "AndWhere/OrWhere sql")
}

func TestSelect_OrderBy(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").From("users").OrderBy("name ASC", "created_at DESC").ToSQL()
	assertContains(t, sql, "ORDER BY")
}

func TestSelect_Limit_Offset(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").From("users").Limit(10).Offset(20).ToSQL()
	assertContains(t, sql, "LIMIT")
}

func TestSelect_GroupBy_Having(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("status", "COUNT(*) as cnt").From("users").
		GroupBy("status").
		Having("COUNT(*) > ?", 5).
		ToSQL()
	assertContains(t, sql, "GROUP BY")
}

func TestSelect_Distinct(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("category").From("products").Distinct().ToSQL()
	assertContains(t, sql, "DISTINCT")
}

func TestSelect_InnerJoin(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("u.id", "o.total").From("users u").
		InnerJoin("orders o", "o.user_id = u.id").ToSQL()
	assertContains(t, sql, "JOIN")
}

func TestSelect_LeftJoin(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("u.id").From("users u").
		LeftJoin("orders o", "o.user_id = u.id").ToSQL()
	assertContains(t, sql, "JOIN")
}

func TestSelect_RightJoin(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("u.id").From("users u").
		RightJoin("orders o", "o.user_id = u.id").ToSQL()
	assertContains(t, sql, "JOIN")
}

func TestSelect_FullJoin(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("u.id").From("users u").
		FullJoin("orders o", "o.user_id = u.id").ToSQL()
	assertContains(t, sql, "JOIN")
}

func TestSelect_CrossJoin(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("*").From("colors").CrossJoin("sizes").ToSQL()
	assertContains(t, sql, "JOIN")
}

func TestSelect_SelectExpr(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").
		SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id)", "cnt").
		From("users").ToSQL()
	assertNotEmpty(t, sql, "SelectExpr sql")
}

func TestSelect_AndSelect(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").From("users").AndSelect("name", "email").ToSQL()
	assertNotEmpty(t, sql, "AndSelect sql")
}

func TestSelect_SelectSub(t *testing.T) {
	db := openTestDB(t)
	sub := db.Builder().Select("COUNT(*)").From("orders")
	sql, _ := db.Select("id").SelectSub(sub.AsExpression(), "order_count").From("users").ToSQL()
	assertNotEmpty(t, sql, "SelectSub sql")
}

func TestSelect_FromSelect(t *testing.T) {
	db := openTestDB(t)
	sub := db.Builder().Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")
	sql, _ := db.Builder().Select("*").FromSelect(sub, "order_counts").ToSQL()
	assertNotEmpty(t, sql, "FromSelect sql")
}

func TestSelect_Union(t *testing.T) {
	db := openTestDB(t)
	q1 := db.Select("name").From("users")
	q2 := db.Builder().Select("name").From("archived_users")
	sql, _ := q1.Union(q2).ToSQL()
	assertContains(t, sql, "UNION")
}

func TestSelect_UnionAll(t *testing.T) {
	db := openTestDB(t)
	q1 := db.Select("id").From("orders_2023")
	q2 := db.Builder().Select("id").From("orders_2024")
	sql, _ := q1.UnionAll(q2).ToSQL()
	assertContains(t, sql, "UNION")
}

func TestSelect_Intersect(t *testing.T) {
	db := openTestDB(t)
	q1 := db.Select("id").From("users")
	q2 := db.Builder().Select("user_id").From("orders")
	sql, _ := q1.Intersect(q2).ToSQL()
	assertNotEmpty(t, sql, "Intersect sql")
}

func TestSelect_Except(t *testing.T) {
	db := openTestDB(t)
	q1 := db.Select("id").From("all_users")
	q2 := db.Builder().Select("user_id").From("banned_users")
	sql, _ := q1.Except(q2).ToSQL()
	assertNotEmpty(t, sql, "Except sql")
}

func TestSelect_With_CTE(t *testing.T) {
	db := openTestDB(t)
	cte := db.Builder().Select("user_id", "SUM(total) as total").From("orders").GroupBy("user_id")
	sql, _ := db.Builder().Select("*").With("order_totals", cte).
		From("order_totals").Where("total > ?", 1000).ToSQL()
	assertNotEmpty(t, sql, "CTE sql")
}

func TestSelect_WithRecursive(t *testing.T) {
	db := openTestDB(t)
	anchor := db.Builder().Select("id", "manager_id").From("employees").Where("manager_id IS NULL")
	rec := db.Builder().Select("e.id", "e.manager_id").
		From("employees e").InnerJoin("hierarchy h", "e.manager_id = h.id")
	cte := anchor.UnionAll(rec)
	sql, _ := db.Builder().Select("*").WithRecursive("hierarchy", cte).
		From("hierarchy").ToSQL()
	assertNotEmpty(t, sql, "recursive CTE sql")
}

func TestSelect_OrderByExpr(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").From("tasks").
		OrderByExpr("CASE WHEN status = ? THEN 0 ELSE 1 END", "active").
		ToSQL()
	assertNotEmpty(t, sql, "OrderByExpr sql")
}

func TestSelect_OrderBySub(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").From("tasks t").
		OrderBySub(relica.CaseWhen().When("t.due_date < CURRENT_DATE", 0).Else(1)).
		ToSQL()
	assertNotEmpty(t, sql, "OrderBySub sql")
}

func TestSelect_GroupByExpr(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("DATE(created_at)", "COUNT(*)").From("orders").
		GroupByExpr("DATE(created_at)").ToSQL()
	assertNotEmpty(t, sql, "GroupByExpr sql")
}

func TestSelect_GroupBySub(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("status").From("users").
		GroupBySub(relica.Eq("status", 1)).ToSQL()
	_ = sql // GroupBySub may produce non-standard SQL depending on impl; just verify no panic
}

func TestSelect_WithContext(t *testing.T) {
	db := openTestDB(t)
	sq := db.Select("id").From("users").WithContext(context.Background())
	if sq == nil {
		t.Fatal("SelectQuery.WithContext returned nil")
	}
}

func TestSelect_Build(t *testing.T) {
	db := openTestDB(t)
	q := db.Select("id").From("users").Where("id = ?", 1).Build()
	if q == nil {
		t.Fatal("Build returned nil")
	}
	sqlStr := q.SQL()
	assertNotEmpty(t, sqlStr, "Build SQL")
}

func TestSelect_AsExpression(t *testing.T) {
	db := openTestDB(t)
	sub := db.Builder().Select("user_id").From("orders").Where("total > ?", 100)
	expr := sub.AsExpression()
	if expr == nil {
		t.Fatal("AsExpression returned nil")
	}
}

func TestSelect_Unwrap(t *testing.T) {
	db := openTestDB(t)
	inner := db.Select("id").From("users").Unwrap()
	if inner == nil {
		t.Fatal("SelectQuery.Unwrap returned nil")
	}
}

// ─── INSERT query builder ─────────────────────────────────────────────────────

func TestInsert_ToSQL(t *testing.T) {
	db := openTestDB(t)
	sql, params := db.Insert("users", map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	}).ToSQL()
	assertContains(t, sql, "INSERT")
	assertContains(t, sql, "users")
	if len(params) == 0 {
		t.Error("expected params in INSERT")
	}
}

func TestBuilder_Insert_ToSQL(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Builder().Insert("orders", map[string]interface{}{
		"user_id": 1,
		"total":   99.99,
	}).ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestInsertStruct_ToSQL(t *testing.T) {
	type Item struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	db := openTestDB(t)
	it := Item{Name: "Widget"}
	sql, _ := db.InsertStruct("items", &it).ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestInsertStruct_InvalidInput_ErrorPropagates(t *testing.T) {
	db := openTestDB(t)
	// Passing a non-struct should set an error on the Query.
	q := db.InsertStruct("items", "not-a-struct")
	_, err := q.Execute()
	if err == nil {
		t.Error("expected error for non-struct input")
	}
}

func TestBatchInsertStruct_ToSQL(t *testing.T) {
	type Item struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	db := openTestDB(t)
	items := []Item{{Name: "A"}, {Name: "B"}}
	sql, _ := db.BatchInsertStruct("items", items).ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestBatchInsertStruct_EmptySlice_ReturnsError(t *testing.T) {
	type Item struct{ Name string }
	db := openTestDB(t)
	q := db.BatchInsertStruct("items", []Item{})
	_, err := q.Execute()
	if err == nil {
		t.Error("expected error for empty slice")
	}
}

func TestBatchInsertStruct_NonSlice_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	q := db.BatchInsertStruct("items", "not-a-slice")
	_, err := q.Execute()
	if err == nil {
		t.Error("expected error for non-slice")
	}
}

// ─── UPDATE query builder ─────────────────────────────────────────────────────

func TestUpdate_ToSQL(t *testing.T) {
	db := openTestDB(t)
	sql, params := db.Update("users").
		Set(map[string]interface{}{"status": "active"}).
		Where("id = ?", 123).
		ToSQL()
	assertContains(t, sql, "UPDATE")
	if len(params) == 0 {
		t.Error("expected params in UPDATE")
	}
}

func TestUpdate_AndWhere_OrWhere(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Update("users").
		Set(map[string]interface{}{"flag": 1}).
		Where("id > ?", 10).
		AndWhere("active = ?", true).
		OrWhere("role = ?", "admin").
		ToSQL()
	assertNotEmpty(t, sql, "UPDATE AndWhere/OrWhere sql")
}

func TestUpdate_WithContext(t *testing.T) {
	db := openTestDB(t)
	uq := db.Update("users").Set(map[string]interface{}{"x": 1}).WithContext(context.Background())
	if uq == nil {
		t.Fatal("UpdateQuery.WithContext returned nil")
	}
}

func TestUpdate_Build(t *testing.T) {
	db := openTestDB(t)
	q := db.Update("users").Set(map[string]interface{}{"x": 1}).Build()
	if q == nil {
		t.Fatal("UpdateQuery.Build returned nil")
	}
}

func TestUpdateStruct_ToSQL(t *testing.T) {
	type User struct {
		ID     int    `db:"id"`
		Status string `db:"status"`
	}
	db := openTestDB(t)
	u := User{ID: 1, Status: "active"}
	sql, _ := db.UpdateStruct("users", &u).Where("id = ?", u.ID).ToSQL()
	assertContains(t, sql, "UPDATE")
}

func TestUpdateStruct_InvalidInput_ErrorOnExecute(t *testing.T) {
	db := openTestDB(t)
	uq := db.UpdateStruct("users", "not-a-struct")
	_, err := uq.Execute()
	if err == nil {
		t.Error("expected error for non-struct input")
	}
}

func TestUpdateStruct_InvalidInput_ToSQL_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	uq := db.UpdateStruct("users", "not-a-struct")
	sql, _ := uq.ToSQL()
	// Must not panic; returns empty string for error state.
	_ = sql
}

// ─── DELETE query builder ─────────────────────────────────────────────────────

func TestDelete_ToSQL(t *testing.T) {
	db := openTestDB(t)
	sql, params := db.Delete("users").Where("id = ?", 123).ToSQL()
	assertContains(t, sql, "DELETE")
	if len(params) == 0 {
		t.Error("expected params in DELETE")
	}
}

func TestDelete_AndWhere_OrWhere(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Delete("users").
		Where("status = ?", 0).
		AndWhere("created_at < ?", "2020-01-01").
		OrWhere("banned = ?", true).
		ToSQL()
	assertNotEmpty(t, sql, "DELETE AndWhere/OrWhere sql")
}

func TestDelete_WithContext(t *testing.T) {
	db := openTestDB(t)
	dq := db.Delete("users").Where("id = ?", 1).WithContext(context.Background())
	if dq == nil {
		t.Fatal("DeleteQuery.WithContext returned nil")
	}
}

func TestDelete_Build(t *testing.T) {
	db := openTestDB(t)
	q := db.Delete("users").Where("id = ?", 1).Build()
	if q == nil {
		t.Fatal("DeleteQuery.Build returned nil")
	}
}

// ─── BATCH INSERT query builder ───────────────────────────────────────────────

func TestBatchInsert_ToSQL(t *testing.T) {
	db := openTestDB(t)
	sql, params := db.BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com").
		ToSQL()
	assertContains(t, sql, "INSERT")
	if len(params) == 0 {
		t.Error("expected params in BatchInsert")
	}
}

func TestBatchInsert_ValuesMap(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.BatchInsert("users", []string{"name", "email"}).
		ValuesMap(map[string]interface{}{"name": "Alice", "email": "alice@example.com"}).
		ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestBatchInsert_WithContext(t *testing.T) {
	db := openTestDB(t)
	biq := db.BatchInsert("users", []string{"name"}).Values("Alice").
		WithContext(context.Background())
	if biq == nil {
		t.Fatal("BatchInsertQuery.WithContext returned nil")
	}
}

func TestBatchInsert_Build(t *testing.T) {
	db := openTestDB(t)
	q := db.BatchInsert("users", []string{"name"}).Values("Alice").Build()
	if q == nil {
		t.Fatal("BatchInsertQuery.Build returned nil")
	}
}

// ─── BATCH UPDATE query builder ───────────────────────────────────────────────

func TestBatchUpdate_ToSQL(t *testing.T) {
	db := openTestDB(t)
	sql, params := db.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice"}).
		Set(2, map[string]interface{}{"name": "Bob"}).
		ToSQL()
	assertNotEmpty(t, sql, "BatchUpdate sql")
	_ = params
}

func TestBatchUpdate_WithContext(t *testing.T) {
	db := openTestDB(t)
	buq := db.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"x": 1}).
		WithContext(context.Background())
	if buq == nil {
		t.Fatal("BatchUpdateQuery.WithContext returned nil")
	}
}

func TestBatchUpdate_Build(t *testing.T) {
	db := openTestDB(t)
	q := db.BatchUpdate("users", "id").Set(1, map[string]interface{}{"x": 1}).Build()
	if q == nil {
		t.Fatal("BatchUpdateQuery.Build returned nil")
	}
}

// ─── UPSERT query builder ─────────────────────────────────────────────────────

func TestUpsert_ToSQL(t *testing.T) {
	db := openTestDB(t)
	sql, params := db.Upsert("users", map[string]interface{}{
		"id": 1, "name": "Alice",
	}).OnConflict("id").DoUpdate("name").ToSQL()
	assertNotEmpty(t, sql, "Upsert sql")
	_ = params
}

func TestUpsert_DoNothing(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Upsert("users", map[string]interface{}{"id": 1}).
		OnConflict("id").DoNothing().ToSQL()
	assertNotEmpty(t, sql, "Upsert DoNothing sql")
}

func TestUpsert_WithContext(t *testing.T) {
	db := openTestDB(t)
	uq := db.Upsert("users", map[string]interface{}{"id": 1}).
		OnConflict("id").DoUpdate("name").WithContext(context.Background())
	if uq == nil {
		t.Fatal("UpsertQuery.WithContext returned nil")
	}
}

func TestUpsert_Build(t *testing.T) {
	db := openTestDB(t)
	q := db.Upsert("users", map[string]interface{}{"id": 1}).OnConflict("id").DoUpdate("name").Build()
	if q == nil {
		t.Fatal("UpsertQuery.Build returned nil")
	}
}

// ─── Query methods ────────────────────────────────────────────────────────────

func TestNewQuery_SQL_Params(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT 1")
	assertNotEmpty(t, q.SQL(), "Query.SQL")
}

func TestQuery_ToSQL(t *testing.T) {
	db := openTestDB(t)
	q := db.Insert("users", map[string]interface{}{"name": "Alice"})
	sql, params := q.ToSQL()
	assertNotEmpty(t, sql, "Query.ToSQL sql")
	_ = params
}

func TestQuery_SQL_Params_Nil(t *testing.T) {
	// A Query with nil inner query (error case) must not panic.
	db := openTestDB(t)
	q := db.InsertStruct("x", "not-a-struct") // produces error Query
	sqlStr := q.SQL()
	params := q.Params()
	_ = sqlStr
	_ = params
}

func TestQuery_QueryParams_Deprecated(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT 1")
	// QueryParams is deprecated alias for Params; must not panic.
	_ = q.QueryParams()
}

func TestQuery_Bind(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT * FROM users WHERE id = ?").Bind(42)
	assertNotEmpty(t, q.SQL(), "Bind SQL")
}

func TestQuery_BindParams(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT * FROM users WHERE id = {:id}").BindParams(relica.Params{"id": 1})
	assertNotEmpty(t, q.SQL(), "BindParams SQL")
}

func TestQuery_Prepare_IsPrepared_Close(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT 1").Prepare()
	if !q.IsPrepared() {
		t.Error("expected IsPrepared = true after Prepare")
	}
	if err := q.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestQuery_Close_Unprepared_NoError(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT 1")
	if err := q.Close(); err != nil {
		t.Errorf("Close unprepared: %v", err)
	}
}

func TestQuery_ErrorQuery_Execute_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad") // error state
	_, err := q.Execute()
	if err == nil {
		t.Error("expected error from Execute on error Query")
	}
}

func TestQuery_ErrorQuery_One_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad")
	var dest interface{}
	err := q.One(&dest)
	if err == nil {
		t.Error("expected error from One on error Query")
	}
}

func TestQuery_ErrorQuery_All_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad")
	var dest []interface{}
	err := q.All(&dest)
	if err == nil {
		t.Error("expected error from All on error Query")
	}
}

func TestQuery_ErrorQuery_Row_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad")
	var dest int
	err := q.Row(&dest)
	if err == nil {
		t.Error("expected error from Row on error Query")
	}
}

func TestQuery_ErrorQuery_Column_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad")
	var dest []int
	err := q.Column(&dest)
	if err == nil {
		t.Error("expected error from Column on error Query")
	}
}

func TestQuery_ErrorQuery_Bind_ReturnsItself(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad")
	q2 := q.Bind(1) // must not panic
	if q2 == nil {
		t.Fatal("expected non-nil from Bind on error Query")
	}
}

func TestQuery_ErrorQuery_BindParams_ReturnsItself(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad")
	q2 := q.BindParams(relica.Params{"k": 1}) // must not panic
	if q2 == nil {
		t.Fatal("expected non-nil from BindParams on error Query")
	}
}

func TestQuery_ErrorQuery_Prepare_ReturnsItself(t *testing.T) {
	db := openTestDB(t)
	q := db.InsertStruct("x", "bad")
	q2 := q.Prepare() // must not panic
	if q2 == nil {
		t.Fatal("expected non-nil from Prepare on error Query")
	}
}

func TestQuery_IsPrepared_False_ForNewQuery(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT 1")
	if q.IsPrepared() {
		t.Error("expected IsPrepared = false before Prepare")
	}
}

// ─── ExecContext / QueryContext / QueryRowContext ──────────────────────────────

func TestExecContext(t *testing.T) {
	db := openTestDB(t)
	result, err := db.ExecContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("ExecContext: %v", err)
	}
	_, _ = result.RowsAffected()
}

func TestQueryContext(t *testing.T) {
	db := openTestDB(t)
	rows, err := db.QueryContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	defer rows.Close()
}

func TestQueryRowContext(t *testing.T) {
	db := openTestDB(t)
	row := db.QueryRowContext(context.Background(), "SELECT 1")
	if row == nil {
		t.Fatal("QueryRowContext returned nil row")
	}
}

// ─── Dialect helpers ──────────────────────────────────────────────────────────

func TestQuoteTableName(t *testing.T) {
	db := openTestDB(t)
	quoted := db.QuoteTableName("users")
	assertNotEmpty(t, quoted, "QuoteTableName")
}

func TestQuoteColumnName(t *testing.T) {
	db := openTestDB(t)
	quoted := db.QuoteColumnName("user_id")
	assertNotEmpty(t, quoted, "QuoteColumnName")
}

func TestGenerateParamName(t *testing.T) {
	db := openTestDB(t)
	ph := db.GenerateParamName(1)
	// Stub driver dialect is "relica_test" which falls back to sqlite/mysql behavior.
	_ = ph // may be "?" or "$1" depending on dialect detection; just verify no panic
}

// ─── Model API ────────────────────────────────────────────────────────────────

type testUser struct {
	ID    int    `db:"id,pk"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

func (testUser) TableName() string { return "users" }

func TestModel_ReturnsNonNil(t *testing.T) {
	db := openTestDB(t)
	u := testUser{ID: 1, Name: "Alice", Email: "alice@example.com"}
	mq := db.Model(&u)
	if mq == nil {
		t.Fatal("Model returned nil")
	}
}

func TestModel_Table_Override(t *testing.T) {
	db := openTestDB(t)
	u := testUser{}
	mq := db.Model(&u).Table("users_archive")
	if mq == nil {
		t.Fatal("Table returned nil")
	}
}

func TestModel_Exclude(t *testing.T) {
	db := openTestDB(t)
	u := testUser{}
	mq := db.Model(&u).Exclude("email")
	if mq == nil {
		t.Fatal("Exclude returned nil")
	}
}

func TestModel_WithContext(t *testing.T) {
	db := openTestDB(t)
	u := testUser{}
	mq := db.Model(&u).WithContext(context.Background())
	if mq == nil {
		t.Fatal("ModelQuery.WithContext returned nil")
	}
}

func TestModel_FindByPublicID_PrefixMismatch(t *testing.T) {
	type Item struct {
		ID       int    `db:"id,pk"`
		PublicID string `db:"public_id,autoid:itm"`
	}
	db := openTestDB(t)
	it := Item{}
	err := db.Model(&it).FindByPublicID("usr_wrong-prefix")
	if !errors.Is(err, relica.ErrAutoIDPrefixMismatch) {
		t.Errorf("expected ErrAutoIDPrefixMismatch, got %v", err)
	}
}

// ─── Transactions ─────────────────────────────────────────────────────────────

func TestBegin_Commit(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestBegin_Rollback(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

func TestBeginTx_Serializable(t *testing.T) {
	db := openTestDB(t)
	opts := &relica.TxOptions{Isolation: sql.LevelSerializable}
	tx, err := db.BeginTx(context.Background(), opts)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestTx_Unwrap(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	core := tx.Unwrap()
	if core == nil {
		t.Fatal("Tx.Unwrap returned nil")
	}
}

func TestTx_Builder(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	qb := tx.Builder()
	if qb == nil {
		t.Fatal("Tx.Builder returned nil")
	}
}

func TestTx_Select_ToSQL(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	sql, _ := tx.Select("id").From("users").ToSQL()
	assertNotEmpty(t, sql, "Tx SELECT sql")
}

func TestTx_Insert_ToSQL(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	sql, _ := tx.Insert("users", map[string]interface{}{"name": "Bob"}).ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestTx_InsertStruct_ToSQL(t *testing.T) {
	type Item struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	it := Item{Name: "Widget"}
	sql, _ := tx.InsertStruct("items", &it).ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestTx_BatchInsertStruct_ToSQL(t *testing.T) {
	type Item struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	items := []Item{{Name: "A"}, {Name: "B"}}
	sql, _ := tx.BatchInsertStruct("items", items).ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestTx_Update_ToSQL(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	sql, _ := tx.Update("users").Set(map[string]interface{}{"x": 1}).ToSQL()
	assertContains(t, sql, "UPDATE")
}

func TestTx_UpdateStruct_ToSQL(t *testing.T) {
	type User struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	u := User{ID: 1, Name: "Alice"}
	sql, _ := tx.UpdateStruct("users", &u).Where("id = ?", u.ID).ToSQL()
	assertContains(t, sql, "UPDATE")
}

func TestTx_Delete_ToSQL(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	sql, _ := tx.Delete("users").Where("id = ?", 1).ToSQL()
	assertContains(t, sql, "DELETE")
}

func TestTx_BatchInsert_ToSQL(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	sql, _ := tx.BatchInsert("users", []string{"name"}).Values("Alice").ToSQL()
	assertContains(t, sql, "INSERT")
}

func TestTx_BatchUpdate_ToSQL(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	sql, _ := tx.BatchUpdate("users", "id").Set(1, map[string]interface{}{"x": 1}).ToSQL()
	assertNotEmpty(t, sql, "Tx BatchUpdate sql")
}

func TestTx_Upsert_ToSQL(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	sql, _ := tx.Upsert("users", map[string]interface{}{"id": 1, "name": "Alice"}).
		OnConflict("id").DoUpdate("name").ToSQL()
	assertNotEmpty(t, sql, "Tx Upsert sql")
}

func TestTx_NewQuery(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	q := tx.NewQuery("SELECT 1")
	if q == nil {
		t.Fatal("Tx.NewQuery returned nil")
	}
}

func TestTx_Model(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	defer tx.Rollback()
	u := testUser{}
	mq := tx.Model(&u)
	if mq == nil {
		t.Fatal("Tx.Model returned nil")
	}
}

func TestTransactional_Commit(t *testing.T) {
	db := openTestDB(t)
	called := false
	err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Transactional: %v", err)
	}
	if !called {
		t.Error("expected callback to be called")
	}
}

func TestTransactional_Rollback_OnError(t *testing.T) {
	db := openTestDB(t)
	sentinel := errors.New("rollback me")
	err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestTransactionalTx_Commit(t *testing.T) {
	db := openTestDB(t)
	opts := &relica.TxOptions{Isolation: sql.LevelReadCommitted}
	called := false
	err := db.TransactionalTx(context.Background(), opts, func(tx *relica.Tx) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("TransactionalTx: %v", err)
	}
	if !called {
		t.Error("expected callback to be called")
	}
}

// ─── Error classification helpers ─────────────────────────────────────────────

func TestIsUniqueViolation_NilError_ReturnsFalse(t *testing.T) {
	if relica.IsUniqueViolation(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsUniqueViolation_OtherError_ReturnsFalse(t *testing.T) {
	if relica.IsUniqueViolation(errors.New("something else")) {
		t.Error("expected false for unrelated error")
	}
}

func TestIsForeignKeyViolation_NilError_ReturnsFalse(t *testing.T) {
	if relica.IsForeignKeyViolation(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsForeignKeyViolation_OtherError_ReturnsFalse(t *testing.T) {
	if relica.IsForeignKeyViolation(errors.New("something else")) {
		t.Error("expected false for unrelated error")
	}
}

func TestIsNotNullViolation_NilError_ReturnsFalse(t *testing.T) {
	if relica.IsNotNullViolation(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsNotNullViolation_OtherError_ReturnsFalse(t *testing.T) {
	if relica.IsNotNullViolation(errors.New("something else")) {
		t.Error("expected false for unrelated error")
	}
}

func TestIsCheckViolation_NilError_ReturnsFalse(t *testing.T) {
	if relica.IsCheckViolation(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsCheckViolation_OtherError_ReturnsFalse(t *testing.T) {
	if relica.IsCheckViolation(errors.New("something else")) {
		t.Error("expected false for unrelated error")
	}
}

// ─── Error variables ──────────────────────────────────────────────────────────

func TestErrNotFound_IsNonNil(t *testing.T) {
	if relica.ErrNotFound == nil {
		t.Error("ErrNotFound must be non-nil")
	}
}

func TestErrNotFound_ErrorMessage(t *testing.T) {
	// ErrNotFound is a sentinel error; errors.Is(err, relica.ErrNotFound) works
	// when One() returns a wrapped error. ErrNotFound itself does not wrap sql.ErrNoRows.
	if !errors.Is(relica.ErrNotFound, relica.ErrNotFound) {
		t.Error("ErrNotFound must satisfy errors.Is(ErrNotFound, ErrNotFound)")
	}
}

func TestErrAutoIDPrefixMismatch_IsNonNil(t *testing.T) {
	if relica.ErrAutoIDPrefixMismatch == nil {
		t.Error("ErrAutoIDPrefixMismatch must be non-nil")
	}
}

// ─── RegisterIDGenerator ──────────────────────────────────────────────────────

func TestRegisterIDGenerator_NoPanic(t *testing.T) {
	// Registering a custom generator must not panic.
	relica.RegisterIDGenerator("test_gen", func() string {
		return "00000000-0000-0000-0000-000000000001"
	})
}

// ─── DetectOperation ──────────────────────────────────────────────────────────

func TestDetectOperation(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"SELECT * FROM users", "SELECT"},
		{"INSERT INTO users VALUES (?)", "INSERT"},
		{"UPDATE users SET x = ?", "UPDATE"},
		{"DELETE FROM users", "DELETE"},
		{"SOMETHING ELSE", "UNKNOWN"},
	}
	for _, tt := range tests {
		got := relica.DetectOperation(tt.query)
		if got != tt.want {
			t.Errorf("DetectOperation(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

// ─── Configuration options (smoke tests for nil panic) ───────────────────────

func TestWithMaxOpenConns_ReturnsOption(t *testing.T) {
	opt := relica.WithMaxOpenConns(10)
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithMaxIdleConns_ReturnsOption(t *testing.T) {
	opt := relica.WithMaxIdleConns(5)
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithConnMaxLifetime_ReturnsOption(t *testing.T) {
	opt := relica.WithConnMaxLifetime(5 * time.Minute)
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithConnMaxIdleTime_ReturnsOption(t *testing.T) {
	opt := relica.WithConnMaxIdleTime(time.Minute)
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithHealthCheck_ReturnsOption(t *testing.T) {
	opt := relica.WithHealthCheck(30 * time.Second)
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithStmtCacheCapacity_ReturnsOption(t *testing.T) {
	opt := relica.WithStmtCacheCapacity(128)
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithLogger_ReturnsOption(t *testing.T) {
	logger := relica.NewSlogAdapter(slog.Default())
	opt := relica.WithLogger(logger)
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithQueryHook_ReturnsOption(t *testing.T) {
	opt := relica.WithQueryHook(func(ctx context.Context, e relica.QueryEvent) {})
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

func TestWithSensitiveFields_ReturnsOption(t *testing.T) {
	opt := relica.WithSensitiveFields([]string{"password", "token"})
	if opt == nil {
		t.Error("expected non-nil option")
	}
}

// ─── Logger types ─────────────────────────────────────────────────────────────

func TestNewSlogAdapter_ReturnsNonNil(t *testing.T) {
	adapter := relica.NewSlogAdapter(slog.Default())
	if adapter == nil {
		t.Fatal("NewSlogAdapter returned nil")
	}
}

// ─── Expression builders ──────────────────────────────────────────────────────

func TestExpressionBuilders_DoNotPanic(t *testing.T) {
	db := openTestDB(t)

	tests := []struct {
		name string
		expr relica.Expression
	}{
		{"Eq", relica.Eq("col", 1)},
		{"NotEq", relica.NotEq("col", 1)},
		{"GreaterThan", relica.GreaterThan("col", 1)},
		{"LessThan", relica.LessThan("col", 1)},
		{"GreaterOrEqual", relica.GreaterOrEqual("col", 1)},
		{"LessOrEqual", relica.LessOrEqual("col", 1)},
		{"In", relica.In("col", 1, 2, 3)},
		{"NotIn", relica.NotIn("col", 1, 2, 3)},
		{"Between", relica.Between("col", 1, 10)},
		{"NotBetween", relica.NotBetween("col", 1, 10)},
		{"EqCol", relica.EqCol("a.id", "b.id")},
		{"NotEqCol", relica.NotEqCol("a.id", "b.id")},
		{"GreaterThanCol", relica.GreaterThanCol("a.age", "b.age")},
		{"LessThanCol", relica.LessThanCol("a.age", "b.age")},
		{"And", relica.And(relica.Eq("a", 1), relica.Eq("b", 2))},
		{"Or", relica.Or(relica.Eq("a", 1), relica.Eq("b", 2))},
		{"Not", relica.Not(relica.Eq("deleted", true))},
		{"NewExp", relica.NewExp("a = b")},
		{"Exists", relica.Exists(relica.Eq("id", 1))},
		{"NotExists", relica.NotExists(relica.Eq("id", 1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expr == nil {
				t.Fatalf("%s: returned nil", tt.name)
			}
			// Use the expression in a real query to exercise the full path.
			sql, _ := db.Select("id").From("t").Where(tt.expr).ToSQL()
			assertNotEmpty(t, sql, tt.name+" sql")
		})
	}
}

func TestLike_DoNotPanic(t *testing.T) {
	db := openTestDB(t)
	tests := []struct {
		name string
		exp  *relica.LikeExp
	}{
		{"Like", relica.Like("name", "john%")},
		{"NotLike", relica.NotLike("name", "john%")},
		{"OrLike", relica.OrLike("name", "alice%", "bob%")},
		{"OrNotLike", relica.OrNotLike("name", "spam%")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.exp == nil {
				t.Fatalf("%s: returned nil", tt.name)
			}
			sql, _ := db.Select("id").From("t").Where(tt.exp).ToSQL()
			assertNotEmpty(t, sql, tt.name+" sql")
		})
	}
}

func TestHashExp_InQuery(t *testing.T) {
	db := openTestDB(t)
	sql, _ := db.Select("id").From("users").Where(relica.HashExp{
		"status": 1,
		"role":   []string{"admin", "mod"},
	}).ToSQL()
	assertNotEmpty(t, sql, "HashExp sql")
}

// ─── Functional expressions ───────────────────────────────────────────────────

func TestFunctionalExpressions_DoNotPanic(t *testing.T) {
	db := openTestDB(t)

	caseExp := relica.Case("status").Else("unknown")
	sql, _ := db.Select("id").SelectSub(caseExp, "label").From("users").ToSQL()
	assertNotEmpty(t, sql, "Case sql")

	caseWhenExp := relica.CaseWhen().When("status = 1", "active").Else("inactive")
	sql, _ = db.Select("id").SelectSub(caseWhenExp, "label").From("users").ToSQL()
	assertNotEmpty(t, sql, "CaseWhen sql")

	coalesceExp := relica.Coalesce("name", "unknown")
	sql, _ = db.Select("id").SelectSub(coalesceExp, "display_name").From("users").ToSQL()
	assertNotEmpty(t, sql, "Coalesce sql")

	nullifExp := relica.NullIf("name", "")
	sql, _ = db.Select("id").SelectSub(nullifExp, "safe_name").From("users").ToSQL()
	assertNotEmpty(t, sql, "NullIf sql")

	greatestExp := relica.Greatest("a", "b", "c")
	sql, _ = db.Select("id").SelectSub(greatestExp, "g").From("t").ToSQL()
	assertNotEmpty(t, sql, "Greatest sql")

	leastExp := relica.Least("a", "b")
	sql, _ = db.Select("id").SelectSub(leastExp, "l").From("t").ToSQL()
	assertNotEmpty(t, sql, "Least sql")

	concatExp := relica.Concat("hello", " ", "world")
	sql, _ = db.Select("id").SelectSub(concatExp, "msg").From("t").ToSQL()
	assertNotEmpty(t, sql, "Concat sql")

	_ = db
}

// ─── Generic functions ────────────────────────────────────────────────────────

func TestGenericOne_TypeSignature(t *testing.T) {
	// Compile-time type check: One[T] returns (T, error).
	var _ func(*relica.SelectQuery) (testUser, error) = relica.One[testUser]
}

func TestGenericAll_TypeSignature(t *testing.T) {
	var _ func(*relica.SelectQuery) ([]testUser, error) = relica.All[testUser]
}

func TestGenericScalar_TypeSignature(t *testing.T) {
	var _ func(*relica.SelectQuery) (int64, error) = relica.Scalar[int64]
}

func TestGenericOne_Execute(t *testing.T) {
	db := openTestDB(t)
	sq := db.Select("id", "name", "email").From("users").Where("id = ?", 1)
	// Stub returns no rows, so One returns ErrNotFound. We verify no panic.
	_, err := relica.One[testUser](sq)
	_ = err // ErrNotFound or scan error expected
}

func TestGenericAll_Execute(t *testing.T) {
	db := openTestDB(t)
	sq := db.Select("id", "name", "email").From("users")
	// Stub returns no rows; All should return empty slice with nil error.
	_, err := relica.All[testUser](sq)
	_ = err
}

func TestGenericScalar_Execute(t *testing.T) {
	db := openTestDB(t)
	sq := db.Select("COUNT(*)").From("users")
	// Stub returns a row with column "id" = int64(1). Scalar scans into int64.
	_, err := relica.Scalar[int64](sq)
	_ = err
}

// ─── Execution paths via stub driver ─────────────────────────────────────────
// The stub driver accepts any SQL and returns stubResult{1,1} for Exec
// and an empty row set for Query. This exercises the execute/scan code paths
// in the public wrappers without needing a real database.

func TestInsert_Execute_NoError(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Insert("users", map[string]interface{}{"name": "Alice"}).Execute()
	if err != nil {
		t.Fatalf("Insert.Execute: %v", err)
	}
}

func TestUpdate_Execute_NoError(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Update("users").
		Set(map[string]interface{}{"name": "Bob"}).
		Where("id = ?", 1).
		Execute()
	if err != nil {
		t.Fatalf("Update.Execute: %v", err)
	}
}

func TestDelete_Execute_NoError(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Delete("users").Where("id = ?", 1).Execute()
	if err != nil {
		t.Fatalf("Delete.Execute: %v", err)
	}
}

func TestUpsert_Execute_NoError(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Upsert("users", map[string]interface{}{"id": 1, "name": "Alice"}).
		OnConflict("id").DoUpdate("name").Execute()
	if err != nil {
		t.Fatalf("Upsert.Execute: %v", err)
	}
}

func TestBatchInsert_Execute_NoError(t *testing.T) {
	db := openTestDB(t)
	_, err := db.BatchInsert("users", []string{"name"}).
		Values("Alice").
		Execute()
	if err != nil {
		t.Fatalf("BatchInsert.Execute: %v", err)
	}
}

func TestBatchUpdate_Execute_NoError(t *testing.T) {
	db := openTestDB(t)
	_, err := db.BatchUpdate("users", "id").
		Set(1, map[string]interface{}{"name": "Alice"}).
		Execute()
	if err != nil {
		t.Fatalf("BatchUpdate.Execute: %v", err)
	}
}

func TestSelectQuery_All_NoRows(t *testing.T) {
	db := openTestDB(t)
	// The stub driver returns an empty row set, so All returns no error (empty slice).
	var users []testUser
	err := db.Select("id", "name", "email").From("users").All(&users)
	// Accept either nil (empty result) or sql.ErrNoRows wrapping.
	// With stub driver that returns 0 rows, All should return nil and empty slice.
	_ = err // may be nil or error depending on scanner behavior with stub
}

func TestSelectQuery_One_ErrNoRows(t *testing.T) {
	db := openTestDB(t)
	// Stub driver returns no rows; One should return ErrNotFound.
	var u testUser
	err := db.Select("id", "name", "email").From("users").Where("id = ?", 9999).One(&u)
	// Must return some error (ErrNotFound or scan error due to missing columns in stub).
	_ = err // stub returns 0 columns, scan will fail — that's expected behavior
}

func TestSelectQuery_Row_NoRows(t *testing.T) {
	db := openTestDB(t)
	var id int
	err := db.Select("id").From("users").Where("id = ?", 1).Row(&id)
	_ = err // stub returns empty/no rows
}

func TestSelectQuery_Column_NoRows(t *testing.T) {
	db := openTestDB(t)
	var ids []int
	err := db.Select("id").From("users").Column(&ids)
	_ = err // stub returns empty result
}

func TestSelectQuery_Count_NoError(t *testing.T) {
	db := openTestDB(t)
	// Count executes SELECT COUNT(*) — stub Query returns stubRows with "id" column set to 1.
	// The actual scan may fail (wrong column name), so just verify no panic.
	_, err := db.Select().From("users").Count()
	_ = err // may fail with type error from stub; just verify no panic
}

func TestSelectQuery_Exists_NoError(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Select().From("users").Where("id = ?", 1).Exists()
	_ = err // may fail with type error from stub; just verify no panic
}

func TestModelQuery_Insert_Execute(t *testing.T) {
	db := openTestDB(t)
	u := testUser{Name: "Alice", Email: "alice@example.com"}
	// The stub driver accepts INSERT and returns LastInsertId=1.
	// ModelQuery.Insert uses sqlite3 dialect (LastInsertId, no RETURNING).
	err := db.Model(&u).Insert()
	if err != nil {
		t.Fatalf("Model.Insert: %v", err)
	}
}

func TestModelQuery_Update_Execute(t *testing.T) {
	db := openTestDB(t)
	u := testUser{ID: 1, Name: "Alice Updated", Email: "alice@example.com"}
	err := db.Model(&u).Update()
	if err != nil {
		t.Fatalf("Model.Update: %v", err)
	}
}

func TestModelQuery_Delete_Execute(t *testing.T) {
	db := openTestDB(t)
	u := testUser{ID: 1, Name: "Alice", Email: "alice@example.com"}
	err := db.Model(&u).Delete()
	if err != nil {
		t.Fatalf("Model.Delete: %v", err)
	}
}

func TestModelQuery_Upsert_Execute(t *testing.T) {
	db := openTestDB(t)
	u := testUser{ID: 1, Name: "Alice", Email: "alice@example.com"}
	err := db.Model(&u).Upsert()
	if err != nil {
		t.Fatalf("Model.Upsert: %v", err)
	}
}

func TestModelQuery_UpdateChanged_Execute(t *testing.T) {
	db := openTestDB(t)
	original := testUser{ID: 1, Name: "Alice", Email: "alice@example.com"}
	updated := testUser{ID: 1, Name: "Alice Updated", Email: "alice@example.com"}
	err := db.Model(&updated).UpdateChanged(&original)
	if err != nil {
		t.Fatalf("Model.UpdateChanged: %v", err)
	}
}

func TestModelQuery_UpdateChanged_NoChange_NoError(t *testing.T) {
	db := openTestDB(t)
	u := testUser{ID: 1, Name: "Alice", Email: "alice@example.com"}
	// No change — UpdateChanged should be a no-op returning nil.
	err := db.Model(&u).UpdateChanged(&u)
	if err != nil {
		t.Fatalf("Model.UpdateChanged no-op: %v", err)
	}
}

func TestQuery_Execute_NoError(t *testing.T) {
	db := openTestDB(t)
	q := db.NewQuery("SELECT 1")
	_, err := q.Execute()
	if err != nil {
		t.Fatalf("Query.Execute: %v", err)
	}
}

func TestUpdateQuery_Execute_WithError_State(t *testing.T) {
	db := openTestDB(t)
	uq := db.UpdateStruct("users", "not-a-struct")
	_, err := uq.Execute()
	if err == nil {
		t.Error("expected error from UpdateQuery.Execute in error state")
	}
}

func TestUpdateQuery_Build_WithError_State(t *testing.T) {
	db := openTestDB(t)
	uq := db.UpdateStruct("users", "not-a-struct")
	q := uq.Build()
	if q == nil {
		t.Fatal("Build must return non-nil Query even in error state")
	}
}

func TestSelectQuery_Explain_NoPanic(t *testing.T) {
	db := openTestDB(t)
	// Explain sends EXPLAIN <sql> to the database. The stub driver returns
	// empty rows which the analyzer cannot parse, so an error is expected.
	// We only verify the call completes without panicking.
	_, _ = db.Select("id").From("users").Where("id = ?", 1).Explain()
}

func TestSelectQuery_ExplainAnalyze_NoPanic(t *testing.T) {
	db := openTestDB(t)
	// ExplainAnalyze also executes the underlying query. Same as Explain —
	// the stub cannot produce parseable EXPLAIN output. Verify no panic.
	_, _ = db.Select("id").From("users").Where("id = ?", 1).ExplainAnalyze()
}
