// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
)

func TestCase_Simple(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Case("status").
		When("active", 1).
		When("inactive", 0).
		Else(-1).
		As("status_code")

	sql, args := expr.Build(dialect)

	assert.Equal(t, `CASE "status" WHEN ? THEN ? WHEN ? THEN ? ELSE ? END AS "status_code"`, sql)
	assert.Equal(t, []interface{}{"active", 1, "inactive", 0, -1}, args)
}

func TestCase_SimpleWithoutElse(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Case("type").
		When("A", "Alpha").
		When("B", "Beta")

	sql, args := expr.Build(dialect)

	assert.Equal(t, `CASE "type" WHEN ? THEN ? WHEN ? THEN ? END`, sql)
	assert.Equal(t, []interface{}{"A", "Alpha", "B", "Beta"}, args)
}

func TestCase_SimpleWithoutAlias(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Case("status").
		When("active", 1).
		Else(0)

	sql, args := expr.Build(dialect)

	assert.Equal(t, "CASE `status` WHEN ? THEN ? ELSE ? END", sql)
	assert.Equal(t, []interface{}{"active", 1, 0}, args)
}

func TestCaseWhen_Searched(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := CaseWhen().
		When("age < 18", "minor").
		When("age >= 18 AND age < 65", "adult").
		Else("senior").
		As("age_group")

	sql, args := expr.Build(dialect)

	assert.Equal(t, `CASE WHEN age < 18 THEN ? WHEN age >= 18 AND age < 65 THEN ? ELSE ? END AS "age_group"`, sql)
	assert.Equal(t, []interface{}{"minor", "adult", "senior"}, args)
}

func TestCaseWhen_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := CaseWhen()
	sql, args := expr.Build(dialect)

	assert.Equal(t, "", sql)
	assert.Nil(t, args)
}

func TestCoalesce_Columns(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Coalesce("nickname", "first_name", "username").As("display_name")
	sql, args := expr.Build(dialect)

	assert.Equal(t, `COALESCE("nickname", "first_name", "username") AS "display_name"`, sql)
	assert.Empty(t, args)
}

func TestCoalesce_WithLiteral(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Coalesce("nickname", "'Anonymous'").As("display_name")
	sql, args := expr.Build(dialect)

	assert.Equal(t, `COALESCE("nickname", 'Anonymous') AS "display_name"`, sql)
	assert.Empty(t, args)
}

func TestCoalesce_WithValue(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Coalesce("price", 0)
	sql, args := expr.Build(dialect)

	assert.Equal(t, "COALESCE(`price`, ?)", sql)
	assert.Equal(t, []interface{}{0}, args)
}

func TestCoalesce_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Coalesce()
	sql, args := expr.Build(dialect)

	assert.Equal(t, "", sql)
	assert.Nil(t, args)
}

func TestNullIf_Columns(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := NullIf("email", "backup_email").As("primary_email")
	sql, args := expr.Build(dialect)

	assert.Equal(t, `NULLIF("email", "backup_email") AS "primary_email"`, sql)
	assert.Empty(t, args)
}

func TestNullIf_WithLiteral(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := NullIf("email", "''").As("valid_email")
	sql, args := expr.Build(dialect)

	assert.Equal(t, `NULLIF("email", '') AS "valid_email"`, sql)
	assert.Empty(t, args)
}

func TestNullIf_WithValue(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := NullIf("count", 0)
	sql, args := expr.Build(dialect)

	assert.Equal(t, "NULLIF(`count`, ?)", sql)
	assert.Equal(t, []interface{}{0}, args)
}

func TestGreatest_Postgres(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest("price", "discount_price", "sale_price").As("max_price")
	sql, args := expr.Build(dialect)

	assert.Equal(t, `GREATEST("price", "discount_price", "sale_price") AS "max_price"`, sql)
	assert.Empty(t, args)
}

func TestGreatest_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Greatest("a", "b", "c")
	sql, args := expr.Build(dialect)

	assert.Equal(t, "GREATEST(`a`, `b`, `c`)", sql)
	assert.Empty(t, args)
}

func TestGreatest_SQLite_FallbackToMAX(t *testing.T) {
	dialect := dialects.GetDialect("sqlite")

	expr := Greatest("col1", "col2").As("max_val")
	sql, args := expr.Build(dialect)

	// SQLite doesn't have GREATEST, so we use MAX
	assert.Equal(t, `MAX("col1", "col2") AS "max_val"`, sql)
	assert.Empty(t, args)
}

func TestLeast_Postgres(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Least("price", "discount_price").As("min_price")
	sql, args := expr.Build(dialect)

	assert.Equal(t, `LEAST("price", "discount_price") AS "min_price"`, sql)
	assert.Empty(t, args)
}

func TestLeast_SQLite_FallbackToMIN(t *testing.T) {
	dialect := dialects.GetDialect("sqlite")

	expr := Least("col1", "col2")
	sql, args := expr.Build(dialect)

	// SQLite doesn't have LEAST, so we use MIN
	assert.Equal(t, `MIN("col1", "col2")`, sql)
	assert.Empty(t, args)
}

func TestGreatest_WithValues(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest("price", 100)
	sql, args := expr.Build(dialect)

	assert.Equal(t, `GREATEST("price", ?)`, sql)
	assert.Equal(t, []interface{}{100}, args)
}

func TestGreatest_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest()
	sql, args := expr.Build(dialect)

	assert.Equal(t, "", sql)
	assert.Nil(t, args)
}

func TestConcat_Postgres(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Concat("first_name", "' '", "last_name").As("full_name")
	sql, args := expr.Build(dialect)

	// PostgreSQL uses || operator
	assert.Equal(t, `"first_name" || ' ' || "last_name" AS "full_name"`, sql)
	assert.Empty(t, args)
}

func TestConcat_MySQL(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Concat("first_name", "' '", "last_name").As("full_name")
	sql, args := expr.Build(dialect)

	// MySQL uses CONCAT function
	assert.Equal(t, "CONCAT(`first_name`, ' ', `last_name`) AS `full_name`", sql)
	assert.Empty(t, args)
}

func TestConcat_SQLite(t *testing.T) {
	dialect := dialects.GetDialect("sqlite")

	expr := Concat("a", "b", "c")
	sql, args := expr.Build(dialect)

	// SQLite uses || operator
	assert.Equal(t, `"a" || "b" || "c"`, sql)
	assert.Empty(t, args)
}

func TestConcat_WithValues(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Concat("prefix", 123, "suffix")
	sql, args := expr.Build(dialect)

	assert.Equal(t, `"prefix" || ? || "suffix"`, sql)
	assert.Equal(t, []interface{}{123}, args)
}

func TestConcat_Empty(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Concat()
	sql, args := expr.Build(dialect)

	assert.Equal(t, "", sql)
	assert.Nil(t, args)
}

func TestCoalesce_WithNestedExpression(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Coalesce with nested NullIf
	innerExpr := NullIf("email", "''")
	expr := Coalesce(innerExpr, "'no-email'").As("safe_email")

	sql, args := expr.Build(dialect)

	assert.Equal(t, `COALESCE(NULLIF("email", ''), 'no-email') AS "safe_email"`, sql)
	assert.Empty(t, args)
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
			assert.Equal(t, tt.want, sql)
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
			assert.Equal(t, tt.want, sql)
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
			assert.Equal(t, tt.want, sql)
			assert.Equal(t, []interface{}{42}, args)
		})
	}
}

// TestNullIf_TableAlias tests NULLIF with table-aliased column names
func TestNullIf_TableAlias(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := NullIf("u.score", 0)
	sql, args := expr.Build(dialect)
	assert.Equal(t, `NULLIF("u"."score", ?)`, sql)
	assert.Equal(t, []interface{}{0}, args)
}

// TestGreatest_TableAlias tests GREATEST with table-aliased column names
func TestGreatest_TableAlias(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	expr := Greatest("p.price", "p.sale_price", "p.wholesale_price")
	sql, _ := expr.Build(dialect)
	assert.Equal(t, `GREATEST("p"."price", "p"."sale_price", "p"."wholesale_price")`, sql)
}

// TestLeast_TableAlias tests LEAST with table-aliased column names
func TestLeast_TableAlias(t *testing.T) {
	dialect := dialects.GetDialect("mysql")

	expr := Least("o.subtotal", "o.discount_total")
	sql, _ := expr.Build(dialect)
	assert.Equal(t, "LEAST(`o`.`subtotal`, `o`.`discount_total`)", sql)
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
			assert.Equal(t, tt.want, sql)
		})
	}
}
