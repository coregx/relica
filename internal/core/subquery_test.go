// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
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

	assert.Contains(t, sql, `"id" IN (SELECT`)
	assert.Contains(t, sql, `FROM "orders"`)
	assert.Contains(t, sql, `WHERE total >`) // PostgreSQL converts ? to $1
	assert.Equal(t, []interface{}{100}, args)
}

func TestInExp_Subquery_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("total > ?", 100)
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	assert.Contains(t, sql, "`id` IN (SELECT")
	assert.Contains(t, sql, "FROM `orders`")
	assert.Contains(t, sql, "WHERE total > ?")
	assert.Equal(t, []interface{}{100}, args)
}

func TestInExp_Subquery_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite3")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("total > ?", 100)
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	assert.Contains(t, sql, `"id" IN (SELECT`)
	assert.Contains(t, sql, `FROM "orders"`)
	assert.Contains(t, sql, `WHERE total > ?`)
	assert.Equal(t, []interface{}{100}, args)
}

func TestNotInExp_Subquery(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("status = ?", "deleted")
	exp := NotIn("id", sub)
	sql, args := exp.Build(dialect)

	assert.Contains(t, sql, `"id" NOT IN (SELECT`)
	assert.Contains(t, sql, `FROM "orders"`)
	assert.Contains(t, sql, `WHERE status =`) // PostgreSQL converts ? to $1
	assert.Equal(t, []interface{}{"deleted"}, args)
}

func TestInExp_Subquery_EmptyResult(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Subquery with no conditions (valid but may return many rows)
	sub := qb.Select("user_id").From("orders")
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	assert.Contains(t, sql, `"id" IN (SELECT`)
	assert.Contains(t, sql, `FROM "orders"`)
	assert.Empty(t, args)
}

func TestInExp_Subquery_WithRawExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Use RawExp as subquery
	sub := NewExp("SELECT user_id FROM orders WHERE total > ?", 200)
	exp := In("id", sub)
	sql, args := exp.Build(dialect)

	assert.Equal(t, `"id" IN (SELECT user_id FROM orders WHERE total > ?)`, sql)
	assert.Equal(t, []interface{}{200}, args)
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

	assert.Contains(t, sql, `"id" IN (SELECT`)
	assert.Contains(t, sql, `WHERE total >`) // PostgreSQL converts ? to $1, $2
	assert.Contains(t, sql, ` AND status `)
	assert.Equal(t, []interface{}{100, "completed"}, args)
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

	assert.Contains(t, sql, `"id" IN (SELECT`)
	assert.Contains(t, sql, `INNER JOIN`)
	assert.Equal(t, []interface{}{"active"}, args)
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

	assert.Contains(t, sql, `SELECT`)
	assert.Contains(t, sql, `FROM (SELECT`)
	assert.Contains(t, sql, `GROUP BY`)
	assert.Contains(t, sql, `) AS "order_counts"`)
	assert.Contains(t, sql, `WHERE cnt >`) // PostgreSQL converts ? to $1
	assert.Equal(t, []interface{}{10}, args)
}

func TestSelectQuery_FromSelect_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id", "SUM(total) as total").From("orders").GroupBy("user_id")
	outer := qb.Select("*").FromSelect(sub, "user_totals").Where("total > ?", 1000)
	sql, args := outer.buildSQL(dialect)

	assert.Contains(t, sql, "FROM (SELECT")
	assert.Contains(t, sql, ") AS `user_totals`")
	assert.Contains(t, sql, "WHERE total > ?")
	assert.Equal(t, []interface{}{1000}, args)
}

func TestSelectQuery_FromSelect_RequiresAlias(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("*").From("users")

	// Should panic without alias
	assert.Panics(t, func() {
		qb.Select("*").FromSelect(sub, "")
	})
}

func TestSelectQuery_FromSelect_WithWhere(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	sub := qb.Select("user_id").From("orders").Where("status = ?", "pending")
	outer := qb.Select("user_id").FromSelect(sub, "pending_orders")
	sql, args := outer.buildSQL(dialect)

	assert.Contains(t, sql, `FROM (SELECT`)
	assert.Contains(t, sql, `WHERE status =`) // PostgreSQL converts ? to $1
	assert.Contains(t, sql, `) AS "pending_orders"`)
	assert.Equal(t, []interface{}{"pending"}, args)
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

	assert.Contains(t, sql, "FROM (SELECT")
	assert.Contains(t, sql, "FROM (SELECT")
	assert.Contains(t, sql, `) AS "order_counts"`)
	assert.Contains(t, sql, `) AS "active_users"`)
	assert.Equal(t, []interface{}{5}, args)
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

	assert.Contains(t, sql, `FROM (SELECT`)
	assert.Contains(t, sql, `) AS "ot"`)
	assert.Contains(t, sql, `INNER JOIN`)
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

	assert.Contains(t, sql, `SELECT "id", "name", (SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) as order_count`)
	assert.Contains(t, sql, `FROM "users"`)
	assert.Empty(t, args)
}

func TestSelectQuery_SelectExpr_WithParams(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id", "name").
		SelectExpr("(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id AND status = ?) as order_count", "completed").
		From("users")
	sql, args := outer.buildSQL(dialect)

	assert.Contains(t, sql, `(SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id AND status = ?) as order_count`)
	assert.Equal(t, []interface{}{"completed"}, args)
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

	assert.Contains(t, sql, `(SELECT COUNT(*) FROM orders`)
	assert.Contains(t, sql, `(SELECT SUM(total) FROM orders`)
	assert.Empty(t, args)
}

func TestSelectQuery_SelectExpr_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id").
		SelectExpr("(SELECT MAX(created_at) FROM orders WHERE user_id = users.id) as last_order").
		From("users")
	sql, _ := outer.buildSQL(dialect)

	assert.Contains(t, sql, "SELECT `id`, (SELECT MAX(created_at)")
	assert.Contains(t, sql, "FROM `users`")
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

	assert.Contains(t, sql, `FROM (SELECT`)
	assert.Contains(t, sql, `) AS "oc"`)
	assert.Contains(t, sql, `WHERE "user_id" IN (SELECT`)
	assert.Equal(t, []interface{}{"active"}, args)
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

	assert.Contains(t, sql, `FROM (SELECT`)
	assert.Contains(t, sql, ` IN (SELECT`)
	assert.Contains(t, sql, `AND ot.total >`)
	// Args: premium (IN subquery), 1000 (WHERE)
	assert.Equal(t, []interface{}{"premium", 1000}, args)
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
	assert.Equal(t, `"id" IS NULL`, sql)
	assert.Nil(t, args)
}

func TestSelectQuery_FromSelect_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Empty subquery (no WHERE)
	sub := qb.Select("*").From("users")
	outer := qb.Select("*").FromSelect(sub, "all_users")
	sql, args := outer.buildSQL(dialect)

	assert.Contains(t, sql, `FROM (SELECT`) // "*" gets quoted as "*" in PostgreSQL
	assert.Contains(t, sql, `FROM "users") AS "all_users"`)
	assert.Empty(t, args)
}

func TestSelectQuery_SelectExpr_NoParams(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	outer := qb.Select("id").
		SelectExpr("CURRENT_TIMESTAMP as created").
		From("users")
	sql, args := outer.buildSQL(dialect)

	assert.Contains(t, sql, "CURRENT_TIMESTAMP as created")
	assert.Empty(t, args)
}

func TestInExp_Regular_Values_Still_Work(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Ensure regular IN still works after subquery support
	exp := In("id", 1, 2, 3)
	sql, args := exp.Build(dialect)

	assert.Equal(t, `"id" IN (?, ?, ?)`, sql)
	assert.Equal(t, []interface{}{1, 2, 3}, args)
}

func TestInExp_Single_Regular_Value(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Single value optimization should still work
	exp := In("id", 123)
	sql, args := exp.Build(dialect)

	assert.Equal(t, `"id"=?`, sql)
	assert.Equal(t, []interface{}{123}, args)
}

func TestSelectQuery_From_Backward_Compatibility(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	db := &DB{dialect: dialect}
	qb := &QueryBuilder{db: db}

	// Old From() API should still work
	sq := qb.Select("*").From("users").Where("id = ?", 1)
	sql, args := sq.buildSQL(dialect)

	assert.Contains(t, sql, `FROM "users"`)
	assert.Equal(t, []interface{}{1}, args)
}
