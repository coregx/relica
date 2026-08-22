// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// ============================================================================
// IN (SELECT ...) Subquery Tests
// ============================================================================

func TestInExp_Subquery_PostgreSQL(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Create subquery
	sub := qb.Select("user_id").From("orders").Where("total > ?", 100)

	// Create IN expression with subquery
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	if !strings.Contains(sql, `"id" IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `"id" IN (SELECT`)
	}
	if !strings.Contains(sql, `FROM "orders"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "orders"`)
	}
	if !strings.Contains(sql, `WHERE total >`) { // PostgreSQL converts ? to $1
		t.Errorf("%q does not contain %q", sql, `WHERE total >`)
	}
	want := []interface{}{100}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestInExp_Subquery_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("total > ?", 100)
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	if !strings.Contains(sql, "`id` IN (SELECT") {
		t.Errorf("%q does not contain %q", sql, "`id` IN (SELECT")
	}
	if !strings.Contains(sql, "FROM `orders`") {
		t.Errorf("%q does not contain %q", sql, "FROM `orders`")
	}
	if !strings.Contains(sql, "WHERE total > ?") {
		t.Errorf("%q does not contain %q", sql, "WHERE total > ?")
	}
	want := []interface{}{100}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestInExp_Subquery_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite3")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("total > ?", 100)
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	if !strings.Contains(sql, `"id" IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `"id" IN (SELECT`)
	}
	if !strings.Contains(sql, `FROM "orders"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "orders"`)
	}
	if !strings.Contains(sql, `WHERE total > ?`) {
		t.Errorf("%q does not contain %q", sql, `WHERE total > ?`)
	}
	want := []interface{}{100}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestNotInExp_Subquery(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("status = ?", "deleted")
	exp := NotIn("id", sub)
	sql, args := exp.Build(dialect)

	if !strings.Contains(sql, `"id" NOT IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `"id" NOT IN (SELECT`)
	}
	if !strings.Contains(sql, `FROM "orders"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "orders"`)
	}
	if !strings.Contains(sql, `WHERE status =`) { // PostgreSQL converts ? to $1
		t.Errorf("%q does not contain %q", sql, `WHERE status =`)
	}
	want := []interface{}{"deleted"}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestInExp_Subquery_EmptyResult(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Subquery with no conditions (valid but may return many rows)
	sub := qb.Select("user_id").From("orders")
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	if !strings.Contains(sql, `"id" IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `"id" IN (SELECT`)
	}
	if !strings.Contains(sql, `FROM "orders"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "orders"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestInExp_Subquery_WithRawExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Use RawExp as subquery
	sub := NewExp("SELECT user_id FROM orders WHERE total > ?", 200)
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	if sql != `"id" IN (SELECT user_id FROM orders WHERE total > ?)` {
		t.Errorf("got %v, want %v", sql, `"id" IN (SELECT user_id FROM orders WHERE total > ?)`)
	}
	want := []interface{}{200}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestInExp_Subquery_MultipleParams(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").
		Where("total > ?", 100).
		Where("status = ?", "completed")
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	if !strings.Contains(sql, `"id" IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `"id" IN (SELECT`)
	}
	if !strings.Contains(sql, `WHERE total >`) { // PostgreSQL converts ? to $1, $2
		t.Errorf("%q does not contain %q", sql, `WHERE total >`)
	}
	if !strings.Contains(sql, ` AND status `) {
		t.Errorf("%q does not contain %q", sql, ` AND status `)
	}
	want := []interface{}{100, "completed"}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestInExp_Subquery_WithJoin(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("o.user_id").From("orders o").
		InnerJoin("users u", "o.user_id = u.id").
		Where("u.status = ?", "active")
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	if !strings.Contains(sql, `"id" IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `"id" IN (SELECT`)
	}
	if !strings.Contains(sql, `INNER JOIN`) {
		t.Errorf("%q does not contain %q", sql, `INNER JOIN`)
	}
	want := []interface{}{"active"}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

// ============================================================================
// FROM (SELECT ...) Subquery Tests
// ============================================================================

func TestSelectQuery_FromSelect_PostgreSQL(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Create subquery
	sub := qb.Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")

	// Create outer query with FROM subquery
	outer := qb.Select("user_id", "cnt").FromSelect(sub, "order_counts").Where("cnt > ?", 10)
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `SELECT`) {
		t.Errorf("%q does not contain %q", sql, `SELECT`)
	}
	if !strings.Contains(sql, `FROM (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `FROM (SELECT`)
	}
	if !strings.Contains(sql, `GROUP BY`) {
		t.Errorf("%q does not contain %q", sql, `GROUP BY`)
	}
	if !strings.Contains(sql, `) AS "order_counts"`) {
		t.Errorf("%q does not contain %q", sql, `) AS "order_counts"`)
	}
	if !strings.Contains(sql, `WHERE cnt >`) { // PostgreSQL converts ? to $1
		t.Errorf("%q does not contain %q", sql, `WHERE cnt >`)
	}
	want := []interface{}{10}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestSelectQuery_FromSelect_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id", "SUM(total) as total").From("orders").GroupBy("user_id")
	outer := qb.Select("*").FromSelect(sub, "user_totals").Where("total > ?", 1000)
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, "FROM (SELECT") {
		t.Errorf("%q does not contain %q", sql, "FROM (SELECT")
	}
	if !strings.Contains(sql, ") AS `user_totals`") {
		t.Errorf("%q does not contain %q", sql, ") AS `user_totals`")
	}
	if !strings.Contains(sql, "WHERE total > ?") {
		t.Errorf("%q does not contain %q", sql, "WHERE total > ?")
	}
	want := []interface{}{1000}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestSelectQuery_FromSelect_RequiresAlias(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("*").From("users")

	// Should store an error (not panic) without alias
	sq := qb.Select("*").FromSelect(sub, "")
	if sq.buildErr == nil {
		t.Error("FromSelect with empty alias must store a build error: expected non-nil")
	}
	if sq.buildErr != nil && !strings.Contains(sq.buildErr.Error(), "FromSelect") {
		t.Errorf("%q does not contain %q", sq.buildErr.Error(), "FromSelect")
	}
	q := sq.Build()
	if q.prepErr == nil {
		t.Error("build error must propagate through Build(): expected non-nil")
	}
}

func TestSelectQuery_FromSelect_WithWhere(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("status = ?", "pending")
	outer := qb.Select("user_id").FromSelect(sub, "pending_orders")
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `FROM (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `FROM (SELECT`)
	}
	if !strings.Contains(sql, `WHERE status =`) { // PostgreSQL converts ? to $1
		t.Errorf("%q does not contain %q", sql, `WHERE status =`)
	}
	if !strings.Contains(sql, `) AS "pending_orders"`) {
		t.Errorf("%q does not contain %q", sql, `) AS "pending_orders"`)
	}
	want := []interface{}{"pending"}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestSelectQuery_FromSelect_Nested(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Inner subquery
	inner := qb.Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")

	// Middle subquery
	middle := qb.Select("user_id", "cnt").FromSelect(inner, "order_counts").Where("cnt > ?", 5)

	// Outer query
	outer := qb.Select("user_id").FromSelect(middle, "active_users")
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, "FROM (SELECT") {
		t.Errorf("%q does not contain %q", sql, "FROM (SELECT")
	}
	if !strings.Contains(sql, `) AS "order_counts"`) {
		t.Errorf("%q does not contain %q", sql, `) AS "order_counts"`)
	}
	if !strings.Contains(sql, `) AS "active_users"`) {
		t.Errorf("%q does not contain %q", sql, `) AS "active_users"`)
	}
	want := []interface{}{5}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestSelectQuery_FromSelect_WithJoin(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id", "SUM(total) as total").From("orders").GroupBy("user_id")
	outer := qb.Select("u.name", "ot.total").
		FromSelect(sub, "ot").
		InnerJoin("users u", "ot.user_id = u.id")
	sql, _ := outer.buildSQL(dialect)

	if !strings.Contains(sql, `FROM (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `FROM (SELECT`)
	}
	if !strings.Contains(sql, `) AS "ot"`) {
		t.Errorf("%q does not contain %q", sql, `) AS "ot"`)
	}
	if !strings.Contains(sql, `INNER JOIN`) {
		t.Errorf("%q does not contain %q", sql, `INNER JOIN`)
	}
}

// ============================================================================
// SelectExpr() Scalar Subquery Tests
// ============================================================================

func TestSelectQuery_SelectExpr_ScalarSubquery(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id", "name").
		SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) as order_count").
		From("users")
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `SELECT "id", "name", (SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) as order_count`) {
		t.Errorf("%q does not contain %q", sql, `SELECT "id", "name", (SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) as order_count`)
	}
	if !strings.Contains(sql, `FROM "users"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "users"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestSelectQuery_SelectExpr_WithParams(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id", "name").
		SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id AND status = ?) as order_count", "completed").
		From("users")
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id AND status = ?) as order_count`) {
		t.Errorf("%q does not contain %q", sql, `(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id AND status = ?) as order_count`)
	}
	want := []interface{}{"completed"}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestSelectQuery_SelectExpr_Multiple(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id", "name").
		SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) as order_count").
		SelectExpr("(SELECT SUM(total) FROM orders WHERE orders.user_id = users.id) as total_spent").
		From("users")
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `(SELECT COUNT(*) FROM orders`) {
		t.Errorf("%q does not contain %q", sql, `(SELECT COUNT(*) FROM orders`)
	}
	if !strings.Contains(sql, `(SELECT SUM(total) FROM orders`) {
		t.Errorf("%q does not contain %q", sql, `(SELECT SUM(total) FROM orders`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestSelectQuery_SelectExpr_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id").
		SelectExpr("(SELECT MAX(created_at) FROM orders WHERE user_id = users.id) as last_order").
		From("users")
	sql, _ := outer.buildSQL(dialect)

	if !strings.Contains(sql, "SELECT `id`, (SELECT MAX(created_at)") {
		t.Errorf("%q does not contain %q", sql, "SELECT `id`, (SELECT MAX(created_at)")
	}
	if !strings.Contains(sql, "FROM `users`") {
		t.Errorf("%q does not contain %q", sql, "FROM `users`")
	}
}

// ============================================================================
// Combined Subquery Tests
// ============================================================================

func TestSelectQuery_Combined_FromSelect_And_IN(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Subquery for FROM
	fromSub := qb.Select("user_id", "COUNT(*) as cnt").From("orders").GroupBy("user_id")

	// Subquery for IN
	inSub := qb.Select("id").From("users").Where("status = ?", "active")

	// Outer query
	outer := qb.Select("user_id", "cnt").
		FromSelect(fromSub, "oc").
		Where(In("user_id", inSub))
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `FROM (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `FROM (SELECT`)
	}
	if !strings.Contains(sql, `) AS "oc"`) {
		t.Errorf("%q does not contain %q", sql, `) AS "oc"`)
	}
	if !strings.Contains(sql, `WHERE "user_id" IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `WHERE "user_id" IN (SELECT`)
	}
	want := []interface{}{"active"}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestSelectQuery_Combined_All_Features(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Complex query with FROM subquery, SelectExpr, and IN subquery
	fromSub := qb.Select("user_id", "SUM(total) as total").From("orders").GroupBy("user_id")
	inSub := qb.Select("id").From("categories").Where("type = ?", "premium")

	outer := qb.Select("ot.user_id", "ot.total").
		SelectExpr("(SELECT name FROM users WHERE id = ot.user_id) as username").
		FromSelect(fromSub, "ot").
		Where(In("ot.user_id", inSub)).
		Where("ot.total > ?", 1000)
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `FROM (SELECT`) {
		t.Errorf("%q does not contain %q", sql, `FROM (SELECT`)
	}
	if !strings.Contains(sql, ` IN (SELECT`) {
		t.Errorf("%q does not contain %q", sql, ` IN (SELECT`)
	}
	if !strings.Contains(sql, `AND ot.total >`) {
		t.Errorf("%q does not contain %q", sql, `AND ot.total >`)
	}
	// Args: premium (IN subquery), 1000 (WHERE)
	want := []interface{}{"premium", 1000}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Errorf("got %v, want %v", args, want)
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestInExp_Subquery_Nil(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// IN with nil subquery should be treated as error-prone
	// but let's ensure it doesn't crash
	exp := In("id", nil)
	sql, args := exp.Build(dialect)

	// nil value should generate IS NULL
	if sql != `"id" IS NULL` {
		t.Errorf("got %v, want %v", sql, `"id" IS NULL`)
	}
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestSelectQuery_FromSelect_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Empty subquery (no WHERE)
	sub := qb.Select("*").From("users")
	outer := qb.Select("*").FromSelect(sub, "all_users")
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, `FROM (SELECT`) { // "*" gets quoted as "*" in PostgreSQL
		t.Errorf("%q does not contain %q", sql, `FROM (SELECT`)
	}
	if !strings.Contains(sql, `FROM "users") AS "all_users"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "users") AS "all_users"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestSelectQuery_SelectExpr_NoParams(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id").
		SelectExpr("CURRENT_TIMESTAMP as created").
		From("users")
	sql, args := outer.buildSQL(dialect)

	if !strings.Contains(sql, "CURRENT_TIMESTAMP as created") {
		t.Errorf("%q does not contain %q", sql, "CURRENT_TIMESTAMP as created")
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestInExp_Regular_Values_Still_Work(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Ensure regular IN still works after subquery support
	exp := In("id", 1, 2, 3)
	sql, args := exp.Build(dialect)

	if sql != `"id" IN (?, ?, ?)` {
		t.Errorf("got %v, want %v", sql, `"id" IN (?, ?, ?)`)
	}
	want := []interface{}{1, 2, 3}
	if len(args) != len(want) {
		t.Errorf("got %v, want %v", args, want)
	} else {
		for i := range want {
			if args[i] != want[i] {
				t.Errorf("got %v, want %v", args, want)
				break
			}
		}
	}
}

func TestInExp_Single_Regular_Value(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Single value optimization should still work
	exp := In("id", 123)
	sql, args := exp.Build(dialect)

	if sql != `"id" = ?` {
		t.Errorf("got %v, want %v", sql, `"id" = ?`)
	}
	want := []interface{}{123}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestSelectQuery_From_Backward_Compatibility(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Old From() API should still work
	sq := qb.Select("*").From("users").Where("id = ?", 1)
	sql, args := sq.buildSQL(dialect)

	if !strings.Contains(sql, `FROM "users"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "users"`)
	}
	want := []interface{}{1}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}
