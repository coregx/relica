// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

func TestExists_WithRawExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("SELECT 1 FROM orders WHERE user_id = ?", 123)
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	if sql != `EXISTS (SELECT 1 FROM orders WHERE user_id = ?)` {
		t.Errorf("got %q, want %q", sql, `EXISTS (SELECT 1 FROM orders WHERE user_id = ?)`)
	}
	if len(args) != 1 || args[0] != 123 {
		t.Errorf("got %v, want %v", args, []interface{}{123})
	}
}

func TestNotExists_WithRawExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("SELECT 1 FROM orders WHERE user_id = ?", 123)
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	if sql != `NOT EXISTS (SELECT 1 FROM orders WHERE user_id = ?)` {
		t.Errorf("got %q, want %q", sql, `NOT EXISTS (SELECT 1 FROM orders WHERE user_id = ?)`)
	}
	if len(args) != 1 || args[0] != 123 {
		t.Errorf("got %v, want %v", args, []interface{}{123})
	}
}

func TestExists_WithNilExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	exp := Exists(nil)

	sql, args := exp.Build(dialect)
	if sql != "0=1" { // EXISTS (NULL) → always false
		t.Errorf("got %q, want %q", sql, "0=1")
	}
	if args != nil {
		t.Errorf("got %v, want nil", args)
	}
}

func TestNotExists_WithNilExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	exp := NotExists(nil)

	sql, args := exp.Build(dialect)
	if sql != "" { // NOT EXISTS (NULL) → always true (empty WHERE clause)
		t.Errorf("got %q, want %q", sql, "")
	}
	if args != nil {
		t.Errorf("got %v, want nil", args)
	}
}

func TestExists_WithEmptyExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Empty expression returns empty SQL
	sub := NewExp("")
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	if sql != "0=1" { // EXISTS (empty) → always false
		t.Errorf("got %q, want %q", sql, "0=1")
	}
	if args != nil {
		t.Errorf("got %v, want nil", args)
	}
}

func TestNotExists_WithEmptyExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("")
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	if sql != "" { // NOT EXISTS (empty) → always true
		t.Errorf("got %q, want %q", sql, "")
	}
	if args != nil {
		t.Errorf("got %v, want nil", args)
	}
}

func TestExists_WithHashExp(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Simulate a subquery built with HashExp
	sub := HashExp{"user_id": 123, "status": "active"}
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	// HashExp keys are sorted: status, user_id
	if !strings.Contains(sql, `EXISTS (`) {
		t.Errorf("%q does not contain %q", sql, `EXISTS (`)
	}
	if !strings.Contains(sql, `"status" = ?`) {
		t.Errorf("%q does not contain %q", sql, `"status" = ?`)
	}
	if !strings.Contains(sql, `"user_id" = ?`) {
		t.Errorf("%q does not contain %q", sql, `"user_id" = ?`)
	}
	if !strings.Contains(sql, ` AND `) {
		t.Errorf("%q does not contain %q", sql, ` AND `)
	}
	if len(args) != 2 || args[0] != "active" || args[1] != 123 {
		t.Errorf("got %v, want %v", args, []interface{}{"active", 123})
	}
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
	if !strings.Contains(sql, `EXISTS (`) {
		t.Errorf("%q does not contain %q", sql, `EXISTS (`)
	}
	if !strings.Contains(sql, `"user_id" = ?`) {
		t.Errorf("%q does not contain %q", sql, `"user_id" = ?`)
	}
	if !strings.Contains(sql, `"amount" > ?`) {
		t.Errorf("%q does not contain %q", sql, `"amount" > ?`)
	}
	if !strings.Contains(sql, `) AND (`) {
		t.Errorf("%q does not contain %q", sql, `) AND (`)
	}
	if len(args) != 2 || args[0] != 123 || args[1] != 100 {
		t.Errorf("got %v, want %v", args, []interface{}{123, 100})
	}
}

func TestNotExists_WithComplexExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := Or(
		Eq("status", "pending"),
		Eq("status", "failed"),
	)
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	if !strings.Contains(sql, `NOT EXISTS (`) {
		t.Errorf("%q does not contain %q", sql, `NOT EXISTS (`)
	}
	if !strings.Contains(sql, `"status" = ?`) {
		t.Errorf("%q does not contain %q", sql, `"status" = ?`)
	}
	if !strings.Contains(sql, `) OR (`) {
		t.Errorf("%q does not contain %q", sql, `) OR (`)
	}
	if len(args) != 2 || args[0] != "pending" || args[1] != "failed" {
		t.Errorf("got %v, want %v", args, []interface{}{"pending", "failed"})
	}
}

func TestExists_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	sub := NewExp("SELECT 1 FROM `orders` WHERE `user_id` = ?", 456)
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	want := "EXISTS (SELECT 1 FROM `orders` WHERE `user_id` = ?)"
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
	if len(args) != 1 || args[0] != 456 {
		t.Errorf("got %v, want %v", args, []interface{}{456})
	}
}

func TestNotExists_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	sub := NewExp("SELECT 1 FROM `orders`")
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	want := "NOT EXISTS (SELECT 1 FROM `orders`)"
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
	if args != nil {
		t.Errorf("got %v, want nil", args)
	}
}

func TestExists_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite3")

	sub := NewExp(`SELECT 1 FROM "orders" WHERE "user_id" = ?`, 789)
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	want := `EXISTS (SELECT 1 FROM "orders" WHERE "user_id" = ?)`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
	if len(args) != 1 || args[0] != 789 {
		t.Errorf("got %v, want %v", args, []interface{}{789})
	}
}

func TestNotExists_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite3")

	sub := NewExp(`SELECT 1 FROM "orders"`)
	exp := NotExists(sub)

	sql, args := exp.Build(dialect)
	want := `NOT EXISTS (SELECT 1 FROM "orders")`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
	if args != nil {
		t.Errorf("got %v, want nil", args)
	}
}

func TestExists_MultipleParameters(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	sub := NewExp("SELECT 1 FROM orders WHERE user_id = ? AND total > ? AND status = ?", 123, 100.50, "completed")
	exp := Exists(sub)

	sql, args := exp.Build(dialect)
	want := `EXISTS (SELECT 1 FROM orders WHERE user_id = ? AND total > ? AND status = ?)`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
	if len(args) != 3 || args[0] != 123 || args[1] != 100.50 || args[2] != "completed" {
		t.Errorf("got %v, want %v", args, []interface{}{123, 100.50, "completed"})
	}
}

func TestExists_Type(t *testing.T) {
	// Verify that Exists returns an Expression interface
	var exp Expression
	exp = Exists(NewExp("SELECT 1"))
	if exp == nil {
		t.Fatal("expected non-nil")
	}

	// Verify underlying type
	existsExp, ok := exp.(*ExistsExp)
	if !ok {
		t.Fatal("expected *ExistsExp type assertion to succeed")
	}
	if existsExp.Not {
		t.Error("expected false: existsExp.Not should be false for Exists()")
	}
}

func TestNotExists_Type(t *testing.T) {
	// Verify that NotExists returns an Expression interface
	var exp Expression
	exp = NotExists(NewExp("SELECT 1"))
	if exp == nil {
		t.Fatal("expected non-nil")
	}

	// Verify underlying type
	existsExp, ok := exp.(*ExistsExp)
	if !ok {
		t.Fatal("expected *ExistsExp type assertion to succeed")
	}
	if !existsExp.Not {
		t.Error("expected true: existsExp.Not should be true for NotExists()")
	}
}
