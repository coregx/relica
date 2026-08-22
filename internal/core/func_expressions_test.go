// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

func TestCase_Simple(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Case("status").
		When("active", 1).
		When("inactive", 0).
		Else(-1).
		As("status_code")

	sql, args := expr.Build(dialect)

	if sql != `CASE "status" WHEN ? THEN ? WHEN ? THEN ? ELSE ? END AS "status_code"` {
		t.Errorf("got %v, want %v", sql, `CASE "status" WHEN ? THEN ? WHEN ? THEN ? ELSE ? END AS "status_code"`)
	}
	want := []interface{}{"active", 1, "inactive", 0, -1}
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

func TestCase_SimpleWithoutElse(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Case("type").
		When("A", "Alpha").
		When("B", "Beta")

	sql, args := expr.Build(dialect)

	if sql != `CASE "type" WHEN ? THEN ? WHEN ? THEN ? END` {
		t.Errorf("got %v, want %v", sql, `CASE "type" WHEN ? THEN ? WHEN ? THEN ? END`)
	}
	want := []interface{}{"A", "Alpha", "B", "Beta"}
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

func TestCase_SimpleWithoutAlias(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Case("status").
		When("active", 1).
		Else(0)

	sql, args := expr.Build(dialect)

	if sql != "CASE `status` WHEN ? THEN ? ELSE ? END" {
		t.Errorf("got %v, want %v", sql, "CASE `status` WHEN ? THEN ? ELSE ? END")
	}
	want := []interface{}{"active", 1, 0}
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

func TestCaseWhen_Searched(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := CaseWhen().
		When("age < 18", "minor").
		When("age >= 18 AND age < 65", "adult").
		Else("senior").
		As("age_group")

	sql, args := expr.Build(dialect)

	if sql != `CASE WHEN age < 18 THEN ? WHEN age >= 18 AND age < 65 THEN ? ELSE ? END AS "age_group"` {
		t.Errorf("got %v, want %v", sql, `CASE WHEN age < 18 THEN ? WHEN age >= 18 AND age < 65 THEN ? ELSE ? END AS "age_group"`)
	}
	want := []interface{}{"minor", "adult", "senior"}
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

func TestCaseWhen_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := CaseWhen()
	sql, args := expr.Build(dialect)

	if sql != "" {
		t.Errorf("got %v, want %v", sql, "")
	}
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestCoalesce_Columns(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Coalesce("nickname", "first_name", "username").As("display_name")
	sql, args := expr.Build(dialect)

	if sql != `COALESCE("nickname", "first_name", "username") AS "display_name"` {
		t.Errorf("got %v, want %v", sql, `COALESCE("nickname", "first_name", "username") AS "display_name"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestCoalesce_WithLiteral(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Coalesce("nickname", "'Anonymous'").As("display_name")
	sql, args := expr.Build(dialect)

	if sql != `COALESCE("nickname", 'Anonymous') AS "display_name"` {
		t.Errorf("got %v, want %v", sql, `COALESCE("nickname", 'Anonymous') AS "display_name"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestCoalesce_WithValue(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Coalesce("price", 0)
	sql, args := expr.Build(dialect)

	if sql != "COALESCE(`price`, ?)" {
		t.Errorf("got %v, want %v", sql, "COALESCE(`price`, ?)")
	}
	want := []interface{}{0}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestCoalesce_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Coalesce()
	sql, args := expr.Build(dialect)

	if sql != "" {
		t.Errorf("got %v, want %v", sql, "")
	}
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestNullIf_Columns(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := NullIf("email", "backup_email").As("primary_email")
	sql, args := expr.Build(dialect)

	if sql != `NULLIF("email", "backup_email") AS "primary_email"` {
		t.Errorf("got %v, want %v", sql, `NULLIF("email", "backup_email") AS "primary_email"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestNullIf_WithLiteral(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := NullIf("email", "''").As("valid_email")
	sql, args := expr.Build(dialect)

	if sql != `NULLIF("email", '') AS "valid_email"` {
		t.Errorf("got %v, want %v", sql, `NULLIF("email", '') AS "valid_email"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestNullIf_WithValue(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := NullIf("count", 0)
	sql, args := expr.Build(dialect)

	if sql != "NULLIF(`count`, ?)" {
		t.Errorf("got %v, want %v", sql, "NULLIF(`count`, ?)")
	}
	want := []interface{}{0}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestGreatest_Postgres(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest("price", "discount_price", "sale_price").As("max_price")
	sql, args := expr.Build(dialect)

	if sql != `GREATEST("price", "discount_price", "sale_price") AS "max_price"` {
		t.Errorf("got %v, want %v", sql, `GREATEST("price", "discount_price", "sale_price") AS "max_price"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestGreatest_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Greatest("a", "b", "c")
	sql, args := expr.Build(dialect)

	if sql != "GREATEST(`a`, `b`, `c`)" {
		t.Errorf("got %v, want %v", sql, "GREATEST(`a`, `b`, `c`)")
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestGreatest_SQLite_FallbackToMAX(t *testing.T) {
	dialect := dialects.GetDialect("sqlite")

	expr := Greatest("col1", "col2").As("max_val")
	sql, args := expr.Build(dialect)

	// SQLite doesn't have GREATEST, so we use MAX
	if sql != `MAX("col1", "col2") AS "max_val"` {
		t.Errorf("got %v, want %v", sql, `MAX("col1", "col2") AS "max_val"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestLeast_Postgres(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Least("price", "discount_price").As("min_price")
	sql, args := expr.Build(dialect)

	if sql != `LEAST("price", "discount_price") AS "min_price"` {
		t.Errorf("got %v, want %v", sql, `LEAST("price", "discount_price") AS "min_price"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestLeast_SQLite_FallbackToMIN(t *testing.T) {
	dialect := dialects.GetDialect("sqlite")

	expr := Least("col1", "col2")
	sql, args := expr.Build(dialect)

	// SQLite doesn't have LEAST, so we use MIN
	if sql != `MIN("col1", "col2")` {
		t.Errorf("got %v, want %v", sql, `MIN("col1", "col2")`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestGreatest_WithValues(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest("price", 100)
	sql, args := expr.Build(dialect)

	if sql != `GREATEST("price", ?)` {
		t.Errorf("got %v, want %v", sql, `GREATEST("price", ?)`)
	}
	want := []interface{}{100}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestGreatest_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest()
	sql, args := expr.Build(dialect)

	if sql != "" {
		t.Errorf("got %v, want %v", sql, "")
	}
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestConcat_Postgres(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Concat("first_name", "' '", "last_name").As("full_name")
	sql, args := expr.Build(dialect)

	// PostgreSQL uses || operator
	if sql != `"first_name" || ' ' || "last_name" AS "full_name"` {
		t.Errorf("got %v, want %v", sql, `"first_name" || ' ' || "last_name" AS "full_name"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestConcat_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Concat("first_name", "' '", "last_name").As("full_name")
	sql, args := expr.Build(dialect)

	// MySQL uses CONCAT function
	if sql != "CONCAT(`first_name`, ' ', `last_name`) AS `full_name`" {
		t.Errorf("got %v, want %v", sql, "CONCAT(`first_name`, ' ', `last_name`) AS `full_name`")
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestConcat_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite")

	expr := Concat("a", "b", "c")
	sql, args := expr.Build(dialect)

	// SQLite uses || operator
	if sql != `"a" || "b" || "c"` {
		t.Errorf("got %v, want %v", sql, `"a" || "b" || "c"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestConcat_WithValues(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Concat("prefix", 123, "suffix")
	sql, args := expr.Build(dialect)

	if sql != `"prefix" || ? || "suffix"` {
		t.Errorf("got %v, want %v", sql, `"prefix" || ? || "suffix"`)
	}
	want := []interface{}{123}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestConcat_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Concat()
	sql, args := expr.Build(dialect)

	if sql != "" {
		t.Errorf("got %v, want %v", sql, "")
	}
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestCoalesce_WithNestedExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Coalesce with nested NullIf
	innerExpr := NullIf("email", "''")
	expr := Coalesce(innerExpr, "'no-email'").As("safe_email")

	sql, args := expr.Build(dialect)

	if sql != `COALESCE(NULLIF("email", ''), 'no-email') AS "safe_email"` {
		t.Errorf("got %v, want %v", sql, `COALESCE(NULLIF("email", ''), 'no-email') AS "safe_email"`)
	}
	if len(args) != 0 {
		t.Errorf("expected empty, got %d", len(args))
	}
}

func TestCase_MultipleDialects(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{"postgres", "postgres", `CASE "status" WHEN ? THEN ? END`},
		{"mysql", "mysql", "CASE `status` WHEN ? THEN ? END"},
		{"sqlite", "sqlite", `CASE "status" WHEN ? THEN ? END`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect := dialects.GetDialect(tt.dialect)
			expr := Case("status").When("active", 1)

			sql, _ := expr.Build(dialect)
			if sql != tt.want {
				t.Errorf("got %v, want %v", sql, tt.want)
			}
		})
	}
}

// TestCase_TableAlias tests CASE with table-aliased column names
func TestCase_TableAlias(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		col     string
		want    string
	}{
		{
			"postgres table.column",
			"postgres",
			"u.status",
			`CASE "u"."status" WHEN ? THEN ? WHEN ? THEN ? END`,
		},
		{
			"mysql table.column",
			"mysql",
			"o.type",
			"CASE `o`.`type` WHEN ? THEN ? WHEN ? THEN ? END",
		},
		{
			"sqlite table.column",
			"sqlite",
			"t.category",
			`CASE "t"."category" WHEN ? THEN ? WHEN ? THEN ? END`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect := dialects.GetDialect(tt.dialect)
			expr := Case(tt.col).When("a", 1).When("b", 2)
			sql, _ := expr.Build(dialect)
			if sql != tt.want {
				t.Errorf("got %v, want %v", sql, tt.want)
			}
		})
	}
}

// TestCoalesce_TableAlias tests COALESCE with table-aliased column names
func TestCoalesce_TableAlias(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{
			"postgres",
			"postgres",
			`COALESCE("u"."nickname", "u"."name", ?)`,
		},
		{
			"mysql",
			"mysql",
			"COALESCE(`u`.`nickname`, `u`.`name`, ?)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect := dialects.GetDialect(tt.dialect)
			expr := Coalesce("u.nickname", "u.name", 42)
			sql, args := expr.Build(dialect)
			if sql != tt.want {
				t.Errorf("got %v, want %v", sql, tt.want)
			}
			want := []interface{}{42}
			if len(args) != len(want) || args[0] != want[0] {
				t.Errorf("got %v, want %v", args, want)
			}
		})
	}
}

// TestNullIf_TableAlias tests NULLIF with table-aliased column names
func TestNullIf_TableAlias(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := NullIf("u.score", 0)
	sql, args := expr.Build(dialect)
	if sql != `NULLIF("u"."score", ?)` {
		t.Errorf("got %v, want %v", sql, `NULLIF("u"."score", ?)`)
	}
	want := []interface{}{0}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("got %v, want %v", args, want)
	}
}

// TestGreatest_TableAlias tests GREATEST with table-aliased column names
func TestGreatest_TableAlias(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest("p.price", "p.sale_price", "p.wholesale_price")
	sql, _ := expr.Build(dialect)
	if sql != `GREATEST("p"."price", "p"."sale_price", "p"."wholesale_price")` {
		t.Errorf("got %v, want %v", sql, `GREATEST("p"."price", "p"."sale_price", "p"."wholesale_price")`)
	}
}

// TestLeast_TableAlias tests LEAST with table-aliased column names
func TestLeast_TableAlias(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Least("o.subtotal", "o.discount_total")
	sql, _ := expr.Build(dialect)
	if sql != "LEAST(`o`.`subtotal`, `o`.`discount_total`)" {
		t.Errorf("got %v, want %v", sql, "LEAST(`o`.`subtotal`, `o`.`discount_total`)")
	}
}

// TestConcat_TableAlias tests CONCAT with table-aliased column names
func TestConcat_TableAlias(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{
			"postgres uses ||",
			"postgres",
			`"u"."first_name" || "u"."last_name"`,
		},
		{
			"mysql uses CONCAT()",
			"mysql",
			"CONCAT(`u`.`first_name`, `u`.`last_name`)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect := dialects.GetDialect(tt.dialect)
			expr := Concat("u.first_name", "u.last_name")
			sql, _ := expr.Build(dialect)
			if sql != tt.want {
				t.Errorf("got %v, want %v", sql, tt.want)
			}
		})
	}
}
