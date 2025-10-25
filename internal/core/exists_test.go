// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
)

func TestExists_WithRawExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("SELECT 1 FROM orders WHERE user_id = ?", 123)
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, `EXISTS (SELECT 1 FROM orders WHERE user_id = ?)`, sql)
	assert.Equal(t, []interface{}{123}, args)
}

func TestNotExists_WithRawExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("SELECT 1 FROM orders WHERE user_id = ?", 123)
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, `NOT EXISTS (SELECT 1 FROM orders WHERE user_id = ?)`, sql)
	assert.Equal(t, []interface{}{123}, args)
}

func TestExists_WithNilExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	exp := Exists(nil)

	sql, args := exp.Build(dialect)
	assert.Equal(t, "0=1", sql) // EXISTS (NULL) → always false
	assert.Nil(t, args)
}

func TestNotExists_WithNilExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	exp := NotExists(nil)

	sql, args := exp.Build(dialect)
	assert.Equal(t, "", sql) // NOT EXISTS (NULL) → always true (empty WHERE clause)
	assert.Nil(t, args)
}

func TestExists_WithEmptyExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Empty expression returns empty SQL
	sub := NewExp("")
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, "0=1", sql) // EXISTS (empty) → always false
	assert.Nil(t, args)
}

func TestNotExists_WithEmptyExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("")
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, "", sql) // NOT EXISTS (empty) → always true
	assert.Nil(t, args)
}

func TestExists_WithHashExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Simulate a subquery built with HashExp
	sub := HashExp{"user_id": 123, "status": "active"}
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	// HashExp keys are sorted: status, user_id
	assert.Contains(t, sql, `EXISTS (`)
	assert.Contains(t, sql, `"status"=?`)
	assert.Contains(t, sql, `"user_id"=?`)
	assert.Contains(t, sql, ` AND `)
	assert.Equal(t, []interface{}{"active", 123}, args)
}

func TestExists_WithComplexExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Complex nested expression
	sub := And(
		Eq("user_id", 123),
		GreaterThan("amount", 100),
	)
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	assert.Contains(t, sql, `EXISTS (`)
	assert.Contains(t, sql, `"user_id"=?`)
	assert.Contains(t, sql, `"amount">?`)
	assert.Contains(t, sql, `) AND (`)
	assert.Equal(t, []interface{}{123, 100}, args)
}

func TestNotExists_WithComplexExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := Or(
		Eq("status", "pending"),
		Eq("status", "failed"),
	)
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	assert.Contains(t, sql, `NOT EXISTS (`)
	assert.Contains(t, sql, `"status"=?`)
	assert.Contains(t, sql, `) OR (`)
	assert.Equal(t, []interface{}{"pending", "failed"}, args)
}

func TestExists_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	sub := NewExp("SELECT 1 FROM `orders` WHERE `user_id` = ?", 456)
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, `EXISTS (SELECT 1 FROM `+"`orders`"+` WHERE `+"`user_id`"+` = ?)`, sql)
	assert.Equal(t, []interface{}{456}, args)
}

func TestNotExists_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	sub := NewExp("SELECT 1 FROM `orders`")
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, `NOT EXISTS (SELECT 1 FROM `+"`orders`"+`)`, sql)
	assert.Nil(t, args)
}

func TestExists_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite3")

	sub := NewExp(`SELECT 1 FROM "orders" WHERE "user_id" = ?`, 789)
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, `EXISTS (SELECT 1 FROM "orders" WHERE "user_id" = ?)`, sql)
	assert.Equal(t, []interface{}{789}, args)
}

func TestNotExists_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite3")

	sub := NewExp(`SELECT 1 FROM "orders"`)
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, `NOT EXISTS (SELECT 1 FROM "orders")`, sql)
	assert.Nil(t, args)
}

func TestExists_MultipleParameters(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("SELECT 1 FROM orders WHERE user_id = ? AND total > ? AND status = ?", 123, 100.50, "completed")
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	assert.Equal(t, `EXISTS (SELECT 1 FROM orders WHERE user_id = ? AND total > ? AND status = ?)`, sql)
	assert.Equal(t, []interface{}{123, 100.50, "completed"}, args)
}

func TestExists_Type(t *testing.T) {
	// Verify that Exists returns an Expression interface
	var exp Expression
	exp = Exists(NewExp("SELECT 1"))
	assert.NotNil(t, exp)

	// Verify underlying type
	existsExp, ok := exp.(*ExistsExp)
	assert.True(t, ok)
	assert.False(t, existsExp.Not)
}

func TestNotExists_Type(t *testing.T) {
	// Verify that NotExists returns an Expression interface
	var exp Expression
	exp = NotExists(NewExp("SELECT 1"))
	assert.NotNil(t, exp)

	// Verify underlying type
	existsExp, ok := exp.(*ExistsExp)
	assert.True(t, ok)
	assert.True(t, existsExp.Not)
}
