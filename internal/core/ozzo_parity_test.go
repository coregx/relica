package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// SELECT alias quoting (ozzo-dbx parity)
// =============================================================================

func TestSelectAliasQuoting_SimpleAlias(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		col     string
		want    string
	}{
		{"postgres simple", "postgres", "status AS order_status", `"status" AS "order_status"`},
		{"mysql simple", "mysql", "status AS order_status", "`status` AS `order_status`"},
		{"sqlite simple", "sqlite", "status AS order_status", `"status" AS "order_status"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			q := qb.Select(tt.col).From("orders").Build()
			assert.Contains(t, q.sql, tt.want)
		})
	}
}

func TestSelectAliasQuoting_TableDotColumn(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		col     string
		want    string
	}{
		{"postgres dot alias", "postgres", "u.full_name AS display_name", `"u"."full_name" AS "display_name"`},
		{"mysql dot alias", "mysql", "u.full_name AS display_name", "`u`.`full_name` AS `display_name`"},
		{"postgres schema.table.col", "postgres", "public.users.name AS user_name", `"public"."users"."name" AS "user_name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			q := qb.Select(tt.col).From("users u").Build()
			assert.Contains(t, q.sql, tt.want)
		})
	}
}

func TestSelectAliasQuoting_CaseInsensitive(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// lowercase "as"
	q := qb.Select("name as display").From("users").Build()
	assert.Contains(t, q.sql, `"name" AS "display"`)

	// uppercase "AS"
	q = qb.Select("name AS display").From("users").Build()
	assert.Contains(t, q.sql, `"name" AS "display"`)

	// mixed "As"
	q = qb.Select("name As display").From("users").Build()
	assert.Contains(t, q.sql, `"name" AS "display"`)
}

func TestSelectAliasQuoting_NoAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Regular column — no AS, should be quoted normally
	q := qb.Select("name").From("users").Build()
	assert.Contains(t, q.sql, `"name"`)
	assert.NotContains(t, q.sql, "AS")
}

func TestSelectAliasQuoting_FunctionWithAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Function call with AS — should pass through (has parentheses)
	q := qb.Select("COUNT(*) AS total").From("users").Build()
	assert.Contains(t, q.sql, "COUNT(*) AS total")
}

// =============================================================================
// quoteColumn function-call guard
// =============================================================================

func TestQuoteColumn_FunctionCallGuard(t *testing.T) {
	d := dialects.GetDialect("postgres")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"COUNT(*)", "COUNT(*)", "COUNT(*)"},
		{"MAX(price)", "MAX(price)", "MAX(price)"},
		{"SUM(o.total)", "SUM(o.total)", "SUM(o.total)"},
		{"COALESCE(name, 'N/A')", "COALESCE(name, 'N/A')", "COALESCE(name, 'N/A')"},
		{"simple column", "name", `"name"`},
		{"dotted column", "u.name", `"u"."name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteColumn(tt.input, d)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOrderBy_FunctionCall_NotQuoted(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("user_id", "COUNT(*) as cnt").
		From("orders").
		GroupBy("user_id").
		OrderBy("COUNT(*) DESC").
		Build()

	assert.Contains(t, q.sql, "ORDER BY COUNT(*) DESC")
	assert.NotContains(t, q.sql, `"COUNT(*)"`)
}

func TestGroupBy_FunctionCall_NotQuoted(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("DATE(created_at)", "COUNT(*)").
		From("orders").
		GroupBy("DATE(created_at)").
		Build()

	assert.Contains(t, q.sql, "GROUP BY DATE(created_at)")
	assert.NotContains(t, q.sql, `"DATE(created_at)"`)
}

// =============================================================================
// OFFSET without LIMIT — MySQL compatibility
// =============================================================================

func TestOffsetWithoutLimit_EmitsMaxLimit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select().From("users").Offset(100).Build()

	assert.Contains(t, q.sql, "LIMIT 9223372036854775807")
	assert.Contains(t, q.sql, "OFFSET 100")
}

func TestOffsetWithLimit_NoMaxLimit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select().From("users").Limit(10).Offset(20).Build()

	assert.Contains(t, q.sql, "LIMIT 10")
	assert.Contains(t, q.sql, "OFFSET 20")
	assert.NotContains(t, q.sql, "9223372036854775807")
}

func TestLimitOnly_NoOffset(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select().From("users").Limit(10).Build()

	assert.Contains(t, q.sql, "LIMIT 10")
	assert.NotContains(t, q.sql, "OFFSET")
}

// =============================================================================
// AndSelect — conditional column building
// =============================================================================

func TestAndSelect_AppendsColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("id", "name").From("users").
		AndSelect("email").
		AndSelect("phone", "address").
		Build()

	assert.Contains(t, q.sql, `"id"`)
	assert.Contains(t, q.sql, `"name"`)
	assert.Contains(t, q.sql, `"email"`)
	assert.Contains(t, q.sql, `"phone"`)
	assert.Contains(t, q.sql, `"address"`)
}

func TestAndSelect_ConditionalPattern(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	includeEmail := true
	includePhone := false

	q := qb.Select("id", "name").From("users")
	if includeEmail {
		q = q.AndSelect("email")
	}
	if includePhone {
		q = q.AndSelect("phone")
	}
	built := q.Build()

	assert.Contains(t, built.sql, `"email"`)
	assert.NotContains(t, built.sql, `"phone"`)
}

func TestAndSelect_WithTableAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("u.id").From("users u").
		AndSelect("u.name AS display_name").
		Build()

	assert.Contains(t, q.sql, `"u"."id"`)
	assert.Contains(t, q.sql, `"u"."name" AS "display_name"`)
}

func TestAndSelect_EmptyInitialSelect(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Start with Select() (no cols = *), then add specific columns
	q := qb.Select().From("users").
		AndSelect("id", "name").
		Build()

	// When AndSelect adds columns, they should appear (no more *)
	require.NotNil(t, q)
	assert.Contains(t, q.sql, `"id"`)
	assert.Contains(t, q.sql, `"name"`)
}

// =============================================================================
// Combined: alias quoting + function guard + AndSelect
// =============================================================================

func TestCombined_RealWorldQuery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("c.id", "c.name AS company_name").
		From("companies c").
		LeftJoin("employees e", "e.company_id = c.id").
		AndSelect("COUNT(e.id) AS employee_count").
		GroupBy("c.id", "c.name").
		OrderBy("COUNT(e.id) DESC").
		Having("COUNT(e.id) > ?", 5).
		Build()

	// Column with alias properly quoted
	assert.Contains(t, q.sql, `"c"."name" AS "company_name"`)

	// Function call NOT quoted in ORDER BY
	assert.Contains(t, q.sql, "ORDER BY COUNT(e.id) DESC")
	assert.NotContains(t, q.sql, `"COUNT(e.id)"`)

	// Function call with AS in SELECT passed through
	assert.Contains(t, q.sql, "COUNT(e.id) AS employee_count")

	// GROUP BY columns quoted
	assert.Contains(t, q.sql, `"c"."id"`)

	// HAVING works
	assert.Contains(t, q.sql, "HAVING COUNT(e.id) > ")
}
