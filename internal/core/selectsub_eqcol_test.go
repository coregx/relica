// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// SelectSub Tests
// ============================================================================

// TestSelectSub_ScalarSubquery_PostgreSQL tests a simple scalar COUNT subquery in SELECT.
func TestSelectSub_ScalarSubquery_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("COUNT(*)").From("orders").Where("user_id = ?", 42)
	q := qb.Select("id", "name").SelectSub(sub.AsExpression(), "order_count").From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `"id"`)
	assert.Contains(t, query.sql, `"name"`)
	assert.Contains(t, query.sql, `(SELECT COUNT(*) FROM "orders" WHERE user_id = $1) AS "order_count"`)
	assert.Contains(t, query.sql, `FROM "users"`)
	assert.Equal(t, []interface{}{42}, query.params)
}

// TestSelectSub_ScalarSubquery_MySQL tests a scalar subquery with MySQL backtick quoting.
func TestSelectSub_ScalarSubquery_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("COUNT(*)").From("orders").Where("user_id = ?", 42)
	q := qb.Select("id", "name").SelectSub(sub.AsExpression(), "order_count").From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, "`id`")
	assert.Contains(t, query.sql, "`name`")
	assert.Contains(t, query.sql, "(SELECT COUNT(*) FROM `orders` WHERE user_id = ?) AS `order_count`")
	assert.Contains(t, query.sql, "FROM `users`")
	assert.Equal(t, []interface{}{42}, query.params)
}

// TestSelectSub_ScalarSubquery_SQLite tests a scalar subquery with SQLite double-quote quoting.
func TestSelectSub_ScalarSubquery_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("COUNT(*)").From("orders").Where("user_id = ?", 42)
	q := qb.Select("id", "name").SelectSub(sub.AsExpression(), "order_count").From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `"id"`)
	assert.Contains(t, query.sql, `"name"`)
	assert.Contains(t, query.sql, `(SELECT COUNT(*) FROM "orders" WHERE user_id = ?) AS "order_count"`)
	assert.Contains(t, query.sql, `FROM "users"`)
	assert.Equal(t, []interface{}{42}, query.params)
}

// TestSelectSub_AggregateSubquery tests SUM/AVG aggregate subqueries in SELECT.
func TestSelectSub_AggregateSubquery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sumSub := qb.Select("SUM(amount)").From("payments").Where("status = ?", "paid")
	q := qb.Select("id").SelectSub(sumSub.AsExpression(), "total_paid").From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `(SELECT SUM(amount) FROM "payments" WHERE status = $1) AS "total_paid"`)
	assert.Equal(t, []interface{}{"paid"}, query.params)
}

// TestSelectSub_MultipleSubqueries tests multiple SelectSub calls in one query.
func TestSelectSub_MultipleSubqueries(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	countSub := qb.Select("COUNT(*)").From("orders").Where("user_id = users.id")
	sumSub := qb.Select("SUM(amount)").From("payments").Where("user_id = users.id")

	q := qb.Select("id", "name").
		SelectSub(countSub.AsExpression(), "order_count").
		SelectSub(sumSub.AsExpression(), "total_amount").
		From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `(SELECT COUNT(*) FROM "orders" WHERE user_id = users.id) AS "order_count"`)
	assert.Contains(t, query.sql, `(SELECT SUM(amount) FROM "payments" WHERE user_id = users.id) AS "total_amount"`)
	// Both subqueries present in SELECT
	assert.Contains(t, query.sql, `"id"`)
	assert.Contains(t, query.sql, `"name"`)
}

// TestSelectSub_CombinedWithSelectColumns tests mixing SelectSub with regular Select columns.
func TestSelectSub_CombinedWithSelectColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("COUNT(*)").From("orders")
	q := qb.Select("id", "name", "email").
		SelectSub(sub.AsExpression(), "cnt").
		From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `"id", "name", "email"`)
	assert.Contains(t, query.sql, `(SELECT COUNT(*) FROM "orders") AS "cnt"`)
}

// TestSelectSub_EmptyAlias_StoresError tests that empty alias is rejected gracefully.
func TestSelectSub_EmptyAlias_StoresError(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("COUNT(*)").From("orders")
	q := qb.Select("id").SelectSub(sub.AsExpression(), "").From("users")

	query := q.Build()
	// The query should record a build error for empty alias
	require.NotNil(t, query)
	assert.NotNil(t, q.buildErr)
}

// TestSelectSub_CorrelatedWithEqCol tests the canonical correlated subquery pattern
// combining SelectSub with EqCol for proper column referencing.
func TestSelectSub_CorrelatedWithEqCol(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Correlated subquery: SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id
	sub := qb.Select("COUNT(*)").From("orders").Where(EqCol("orders.user_id", "users.id"))
	q := qb.Select("id", "name").
		SelectSub(sub.AsExpression(), "order_count").
		From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `SELECT "id", "name"`)
	assert.Contains(t, query.sql, `(SELECT COUNT(*) FROM "orders" WHERE "orders"."user_id" = "users"."id") AS "order_count"`)
	assert.Contains(t, query.sql, `FROM "users"`)
	assert.Empty(t, query.params) // EqCol produces no bind parameters
}

// TestSelectSub_ParameterOrdering_PostgreSQL verifies that subExpr params come before WHERE params.
func TestSelectSub_ParameterOrdering_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Subquery has its own param (status = 'active')
	sub := qb.Select("COUNT(*)").From("orders").Where("status = ?", "active")
	// Outer query has its own WHERE param (age > 18)
	q := qb.Select("id").SelectSub(sub.AsExpression(), "cnt").From("users").Where("age > ?", 18)

	query := q.Build()
	require.NotNil(t, query)

	// Params: [subquery param, WHERE param]
	require.Len(t, query.params, 2)
	assert.Equal(t, "active", query.params[0])
	assert.Equal(t, 18, query.params[1])

	// Subquery uses $1, WHERE uses $2
	assert.Contains(t, query.sql, "$1")
	assert.Contains(t, query.sql, "$2")
}

// ============================================================================
// EqCol / NotEqCol / GreaterThanCol / LessThanCol Tests
// ============================================================================

// TestEqCol_SimpleColumns tests basic column equality without table prefix.
func TestEqCol_SimpleColumns(t *testing.T) {
	dialects := getDialects()

	tests := []struct {
		name    string
		dialect string
		col1    string
		col2    string
		wantSQL string
	}{
		{
			name:    "postgres simple",
			dialect: "postgres",
			col1:    "id",
			col2:    "user_id",
			wantSQL: `"id" = "user_id"`,
		},
		{
			name:    "mysql simple",
			dialect: "mysql",
			col1:    "id",
			col2:    "user_id",
			wantSQL: "`id` = `user_id`",
		},
		{
			name:    "sqlite simple",
			dialect: "sqlite",
			col1:    "id",
			col2:    "user_id",
			wantSQL: `"id" = "user_id"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := EqCol(tt.col1, tt.col2)
			sql, args := exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Nil(t, args)
		})
	}
}

// TestEqCol_TableAliasedColumns tests column equality with table.column notation.
func TestEqCol_TableAliasedColumns(t *testing.T) {
	dialects := getDialects()

	tests := []struct {
		name    string
		dialect string
		col1    string
		col2    string
		wantSQL string
	}{
		{
			name:    "postgres table.column",
			dialect: "postgres",
			col1:    "o.user_id",
			col2:    "u.id",
			wantSQL: `"o"."user_id" = "u"."id"`,
		},
		{
			name:    "mysql table.column",
			dialect: "mysql",
			col1:    "o.user_id",
			col2:    "u.id",
			wantSQL: "`o`.`user_id` = `u`.`id`",
		},
		{
			name:    "sqlite table.column",
			dialect: "sqlite",
			col1:    "o.user_id",
			col2:    "u.id",
			wantSQL: `"o"."user_id" = "u"."id"`,
		},
		{
			name:    "postgres schema.table.column",
			dialect: "postgres",
			col1:    "orders.user_id",
			col2:    "users.id",
			wantSQL: `"orders"."user_id" = "users"."id"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := EqCol(tt.col1, tt.col2)
			sql, args := exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Nil(t, args)
		})
	}
}

// TestNotEqCol tests column inequality expression.
func TestNotEqCol_AllDialects(t *testing.T) {
	dialects := getDialects()

	tests := []struct {
		name    string
		dialect string
		col1    string
		col2    string
		wantSQL string
	}{
		{
			name:    "postgres",
			dialect: "postgres",
			col1:    "a.status",
			col2:    "b.status",
			wantSQL: `"a"."status" <> "b"."status"`,
		},
		{
			name:    "mysql",
			dialect: "mysql",
			col1:    "a.status",
			col2:    "b.status",
			wantSQL: "`a`.`status` <> `b`.`status`",
		},
		{
			name:    "sqlite",
			dialect: "sqlite",
			col1:    "a.status",
			col2:    "b.status",
			wantSQL: `"a"."status" <> "b"."status"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := NotEqCol(tt.col1, tt.col2)
			sql, args := exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Nil(t, args)
		})
	}
}

// TestGreaterThanCol tests column greater-than expression.
func TestGreaterThanCol(t *testing.T) {
	dialect := getDialects()["postgres"]

	exp := GreaterThanCol("a.score", "b.score")
	sql, args := exp.Build(dialect)

	assert.Equal(t, `"a"."score" > "b"."score"`, sql)
	assert.Nil(t, args)
}

// TestLessThanCol tests column less-than expression.
func TestLessThanCol(t *testing.T) {
	dialect := getDialects()["postgres"]

	exp := LessThanCol("a.created_at", "b.updated_at")
	sql, args := exp.Build(dialect)

	assert.Equal(t, `"a"."created_at" < "b"."updated_at"`, sql)
	assert.Nil(t, args)
}

// TestEqCol_InWhereClause tests EqCol used inside a WHERE clause.
func TestEqCol_InWhereClause(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("o.id", "o.total").
		From("orders o").
		Where(EqCol("o.user_id", "u.id")).
		Where("o.status = ?", "active")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `"o"."user_id" = "u"."id"`)
	assert.Contains(t, query.sql, `o.status = $1`)
	assert.Equal(t, []interface{}{"active"}, query.params)
}

// TestEqCol_InJoinON tests EqCol used as an Expression in JOIN ON condition.
func TestEqCol_InJoinON(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("u.id", "u.name", "o.total").
		From("users u").
		InnerJoin("orders o", EqCol("o.user_id", "u.id"))

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `INNER JOIN "orders" AS "o" ON "o"."user_id" = "u"."id"`)
}

// TestEqCol_InJoinON_MySQL tests EqCol in JOIN ON with MySQL dialect.
func TestEqCol_InJoinON_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	q := qb.Select("u.id", "u.name").
		From("users u").
		InnerJoin("orders o", EqCol("o.user_id", "u.id"))

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, "INNER JOIN `orders` AS `o` ON `o`.`user_id` = `u`.`id`")
}

// ============================================================================
// Combined SelectSub + EqCol Tests (canonical pattern from issue #31)
// ============================================================================

// TestSelectSub_CorrelatedSubquery_FullPattern tests the full correlated subquery pattern
// combining SelectSub and EqCol as described in ADR-007.
func TestSelectSub_CorrelatedSubquery_FullPattern(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// SELECT COUNT(*) FROM "orders" WHERE "orders"."user_id" = "users"."id"
	sub := qb.Select("COUNT(*)").From("orders").Where(EqCol("orders.user_id", "users.id"))

	// SELECT "id", "name", (...) AS "order_count" FROM "users"
	q := qb.Select("id", "name").
		SelectSub(sub.AsExpression(), "order_count").
		From("users")

	query := q.Build()
	require.NotNil(t, query)

	expectedSQL := `SELECT "id", "name", ` +
		`(SELECT COUNT(*) FROM "orders" WHERE "orders"."user_id" = "users"."id") AS "order_count" ` +
		`FROM "users"`
	assert.Equal(t, expectedSQL, query.sql)
	assert.Empty(t, query.params)
}

// TestSelectSub_CorrelatedSubquery_MySQL tests the full pattern with MySQL dialect.
func TestSelectSub_CorrelatedSubquery_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("COUNT(*)").From("orders").Where(EqCol("orders.user_id", "users.id"))
	q := qb.Select("id", "name").
		SelectSub(sub.AsExpression(), "order_count").
		From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, "(SELECT COUNT(*) FROM `orders` WHERE `orders`.`user_id` = `users`.`id`) AS `order_count`")
	assert.Contains(t, query.sql, "FROM `users`")
}

// TestSelectSub_CorrelatedSubquery_SQLite tests the full pattern with SQLite dialect.
func TestSelectSub_CorrelatedSubquery_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("COUNT(*)").From("orders").Where(EqCol("orders.user_id", "users.id"))
	q := qb.Select("id", "name").
		SelectSub(sub.AsExpression(), "order_count").
		From("users")

	query := q.Build()
	require.NotNil(t, query)

	assert.Contains(t, query.sql, `(SELECT COUNT(*) FROM "orders" WHERE "orders"."user_id" = "users"."id") AS "order_count"`)
	assert.Contains(t, query.sql, `FROM "users"`)
}

// TestSelectSub_MultipleWithParams tests multiple subqueries each with parameters,
// verifying correct PostgreSQL $N placeholder renumbering.
func TestSelectSub_MultipleWithParams_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub1 := qb.Select("COUNT(*)").From("orders").Where("status = ?", "paid")
	sub2 := qb.Select("SUM(amount)").From("payments").Where("type = ?", "credit")

	q := qb.Select("id").
		SelectSub(sub1.AsExpression(), "paid_orders").
		SelectSub(sub2.AsExpression(), "credit_total").
		From("users").
		Where("active = ?", true)

	query := q.Build()
	require.NotNil(t, query)

	// Params: [sub1.param, sub2.param, WHERE.param]
	require.Len(t, query.params, 3)
	assert.Equal(t, "paid", query.params[0])
	assert.Equal(t, "credit", query.params[1])
	assert.Equal(t, true, query.params[2])

	// Each placeholder renumbered correctly
	assert.Contains(t, query.sql, "$1")
	assert.Contains(t, query.sql, "$2")
	assert.Contains(t, query.sql, "$3")
}
