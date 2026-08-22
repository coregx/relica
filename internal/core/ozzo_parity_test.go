package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
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
			if !strings.Contains(q.sql, tt.want) {
				t.Errorf("%q does not contain %q", q.sql, tt.want)
			}
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
			if !strings.Contains(q.sql, tt.want) {
				t.Errorf("%q does not contain %q", q.sql, tt.want)
			}
		})
	}
}

func TestSelectAliasQuoting_CaseInsensitive(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// lowercase "as"
	q := qb.Select("name as display").From("users").Build()
	if !strings.Contains(q.sql, `"name" AS "display"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name" AS "display"`)
	}

	// uppercase "AS"
	q = qb.Select("name AS display").From("users").Build()
	if !strings.Contains(q.sql, `"name" AS "display"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name" AS "display"`)
	}

	// mixed "As"
	q = qb.Select("name As display").From("users").Build()
	if !strings.Contains(q.sql, `"name" AS "display"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name" AS "display"`)
	}
}

func TestSelectAliasQuoting_NoAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Regular column — no AS, should be quoted normally
	q := qb.Select("name").From("users").Build()
	if !strings.Contains(q.sql, `"name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name"`)
	}
	if strings.Contains(q.sql, "AS") {
		t.Errorf("%q should not contain %q", q.sql, "AS")
	}
}

func TestSelectAliasQuoting_FunctionWithAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Function call with AS — should pass through (has parentheses)
	q := qb.Select("COUNT(*) AS total").From("users").Build()
	if !strings.Contains(q.sql, "COUNT(*) AS total") {
		t.Errorf("%q does not contain %q", q.sql, "COUNT(*) AS total")
	}
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
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
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

	if !strings.Contains(q.sql, "ORDER BY COUNT(*) DESC") {
		t.Errorf("%q does not contain %q", q.sql, "ORDER BY COUNT(*) DESC")
	}
	if strings.Contains(q.sql, `"COUNT(*)"`) {
		t.Errorf("%q should not contain %q", q.sql, `"COUNT(*)"`)
	}
}

func TestGroupBy_FunctionCall_NotQuoted(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("DATE(created_at)", "COUNT(*)").
		From("orders").
		GroupBy("DATE(created_at)").
		Build()

	if !strings.Contains(q.sql, "GROUP BY DATE(created_at)") {
		t.Errorf("%q does not contain %q", q.sql, "GROUP BY DATE(created_at)")
	}
	if strings.Contains(q.sql, `"DATE(created_at)"`) {
		t.Errorf("%q should not contain %q", q.sql, `"DATE(created_at)"`)
	}
}

// =============================================================================
// OFFSET without LIMIT — MySQL compatibility
// =============================================================================

func TestOffsetWithoutLimit_EmitsMaxLimit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select().From("users").Offset(100).Build()

	if !strings.Contains(q.sql, "LIMIT 9223372036854775807") {
		t.Errorf("%q does not contain %q", q.sql, "LIMIT 9223372036854775807")
	}
	if !strings.Contains(q.sql, "OFFSET 100") {
		t.Errorf("%q does not contain %q", q.sql, "OFFSET 100")
	}
}

func TestOffsetWithLimit_NoMaxLimit(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select().From("users").Limit(10).Offset(20).Build()

	if !strings.Contains(q.sql, "LIMIT 10") {
		t.Errorf("%q does not contain %q", q.sql, "LIMIT 10")
	}
	if !strings.Contains(q.sql, "OFFSET 20") {
		t.Errorf("%q does not contain %q", q.sql, "OFFSET 20")
	}
	if strings.Contains(q.sql, "9223372036854775807") {
		t.Errorf("%q should not contain %q", q.sql, "9223372036854775807")
	}
}

func TestLimitOnly_NoOffset(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select().From("users").Limit(10).Build()

	if !strings.Contains(q.sql, "LIMIT 10") {
		t.Errorf("%q does not contain %q", q.sql, "LIMIT 10")
	}
	if strings.Contains(q.sql, "OFFSET") {
		t.Errorf("%q should not contain %q", q.sql, "OFFSET")
	}
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

	if !strings.Contains(q.sql, `"id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"id"`)
	}
	if !strings.Contains(q.sql, `"name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name"`)
	}
	if !strings.Contains(q.sql, `"email"`) {
		t.Errorf("%q does not contain %q", q.sql, `"email"`)
	}
	if !strings.Contains(q.sql, `"phone"`) {
		t.Errorf("%q does not contain %q", q.sql, `"phone"`)
	}
	if !strings.Contains(q.sql, `"address"`) {
		t.Errorf("%q does not contain %q", q.sql, `"address"`)
	}
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

	if !strings.Contains(built.sql, `"email"`) {
		t.Errorf("%q does not contain %q", built.sql, `"email"`)
	}
	if strings.Contains(built.sql, `"phone"`) {
		t.Errorf("%q should not contain %q", built.sql, `"phone"`)
	}
}

func TestAndSelect_WithTableAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Select("u.id").From("users u").
		AndSelect("u.name AS display_name").
		Build()

	if !strings.Contains(q.sql, `"u"."id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"u"."id"`)
	}
	if !strings.Contains(q.sql, `"u"."name" AS "display_name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"u"."name" AS "display_name"`)
	}
}

func TestAndSelect_EmptyInitialSelect(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Start with Select() (no cols = *), then add specific columns
	q := qb.Select().From("users").
		AndSelect("id", "name").
		Build()

	// When AndSelect adds columns, they should appear (no more *)
	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, `"id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"id"`)
	}
	if !strings.Contains(q.sql, `"name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name"`)
	}
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
	if !strings.Contains(q.sql, `"c"."name" AS "company_name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"c"."name" AS "company_name"`)
	}

	// Function call NOT quoted in ORDER BY
	if !strings.Contains(q.sql, "ORDER BY COUNT(e.id) DESC") {
		t.Errorf("%q does not contain %q", q.sql, "ORDER BY COUNT(e.id) DESC")
	}
	if strings.Contains(q.sql, `"COUNT(e.id)"`) {
		t.Errorf("%q should not contain %q", q.sql, `"COUNT(e.id)"`)
	}

	// Function call with AS in SELECT passed through
	if !strings.Contains(q.sql, "COUNT(e.id) AS employee_count") {
		t.Errorf("%q does not contain %q", q.sql, "COUNT(e.id) AS employee_count")
	}

	// GROUP BY columns quoted
	if !strings.Contains(q.sql, `"c"."id"`) {
		t.Errorf("%q does not contain %q", q.sql, `"c"."id"`)
	}

	// HAVING works
	if !strings.Contains(q.sql, "HAVING COUNT(e.id) > ") {
		t.Errorf("%q does not contain %q", q.sql, "HAVING COUNT(e.id) > ")
	}
}
