package relica

import "github.com/coregx/relica/internal/core"

// ============================================================================
// CASE
// ============================================================================

// CaseExp is a SQL CASE expression created by Case (simple form) or CaseWhen
// (searched form):
//
//	relica.Case("status").When(1, "active").When(2, "banned").Else("unknown")
//	// CASE "status" WHEN $1 THEN $2 WHEN $3 THEN $4 ELSE $5 END
//
//	relica.CaseWhen().
//	    When(relica.GreaterThan("balance", 1000), "gold").
//	    When("balance > 100", "silver"). // raw SQL condition — never user input
//	    Else("basic")
//
// In the searched form a condition is an Expression (recommended) or a raw
// SQL string. Results may be Expressions, e.g. When(1, relica.Coalesce(...)).
type CaseExp interface {
	Expression

	// When adds a WHEN … THEN … branch. For Case(column) the condition is the
	// value compared with the column; for CaseWhen() it is an Expression or a
	// raw SQL string.
	When(condition, result any) CaseExp

	// Else sets the ELSE value. Without Else, unmatched rows give NULL.
	Else(value any) CaseExp

	// As appends AS alias. Use it in a SELECT list only.
	As(alias string) CaseExp
}

type caseExp struct{ *core.CaseExp }

func (c caseExp) coreExpression() core.Expression { return c.CaseExp }

func (c caseExp) When(condition, result any) CaseExp {
	c.CaseExp.When(coreArg(condition), coreArg(result))
	return c
}

func (c caseExp) Else(value any) CaseExp {
	c.CaseExp.Else(coreArg(value))
	return c
}

func (c caseExp) As(alias string) CaseExp {
	c.CaseExp.As(alias)
	return c
}

func (c caseExp) Err() error { return core.ExpressionError(c.CaseExp) }

// Case creates a simple CASE expression that compares column with each WHEN
// value.
func Case(column string) CaseExp { return caseExp{core.Case(column)} }

// CaseWhen creates a searched CASE expression: each WHEN has its own
// condition.
//
// WARNING: a string condition is inserted as raw SQL. Never pass user input as
// a condition; use an Expression such as relica.Eq instead.
func CaseWhen() CaseExp { return caseExp{core.CaseWhen()} }

// ============================================================================
// COALESCE
// ============================================================================

// CoalesceExp is a SQL COALESCE expression: the first non-NULL argument.
//
//	relica.Coalesce("nickname", "first_name", "'Anonymous'").As("display_name")
//	// COALESCE("nickname", "first_name", 'Anonymous') AS "display_name"
type CoalesceExp interface {
	Expression

	// As appends AS alias. Use it in a SELECT list only.
	As(alias string) CoalesceExp
}

type coalesceExp struct{ *core.CoalesceExp }

func (c coalesceExp) coreExpression() core.Expression { return c.CoalesceExp }

func (c coalesceExp) As(alias string) CoalesceExp {
	c.CoalesceExp.As(alias)
	return c
}

func (c coalesceExp) Err() error { return core.ExpressionError(c.CoalesceExp) }

// Coalesce creates a COALESCE expression.
// A string argument is a column name; a string starting with ' or " is a SQL
// literal used verbatim; an Expression is rendered in place; any other value
// is a parameter.
func Coalesce(values ...any) CoalesceExp { return coalesceExp{core.Coalesce(coreArgs(values)...)} }

// ============================================================================
// NULLIF
// ============================================================================

// NullIfExp is a SQL NULLIF expression: NULL when both arguments are equal,
// otherwise the first.
//
//	relica.NullIf("email", "''").As("email_or_null")
//	// NULLIF("email", '') AS "email_or_null"
type NullIfExp interface {
	Expression

	// As appends AS alias. Use it in a SELECT list only.
	As(alias string) NullIfExp
}

type nullIfExp struct{ *core.NullIfExp }

func (n nullIfExp) coreExpression() core.Expression { return n.NullIfExp }

func (n nullIfExp) As(alias string) NullIfExp {
	n.NullIfExp.As(alias)
	return n
}

func (n nullIfExp) Err() error { return core.ExpressionError(n.NullIfExp) }

// NullIf creates a NULLIF expression.
// A string argument is a column name; a string starting with ' or " is a SQL
// literal used verbatim; an Expression is rendered in place; any other value
// is a parameter.
func NullIf(expr1, expr2 any) NullIfExp {
	return nullIfExp{core.NullIf(coreArg(expr1), coreArg(expr2))}
}

// ============================================================================
// GREATEST / LEAST
// ============================================================================

// GreatestLeastExp is a SQL GREATEST or LEAST expression. SQLite has neither
// function; there it renders as the multi-argument MAX or MIN.
//
//	relica.Greatest("price", "min_price").As("effective_price")
type GreatestLeastExp interface {
	Expression

	// As appends AS alias. Use it in a SELECT list only.
	As(alias string) GreatestLeastExp
}

type greatestLeastExp struct{ *core.GreatestLeastExp }

func (g greatestLeastExp) coreExpression() core.Expression { return g.GreatestLeastExp }

func (g greatestLeastExp) As(alias string) GreatestLeastExp {
	g.GreatestLeastExp.As(alias)
	return g
}

func (g greatestLeastExp) Err() error { return core.ExpressionError(g.GreatestLeastExp) }

// Greatest creates a GREATEST expression (MAX on SQLite).
// A string argument is a column name; a string starting with ' or " is a SQL
// literal used verbatim; an Expression is rendered in place; any other value
// is a parameter.
func Greatest(values ...any) GreatestLeastExp {
	return greatestLeastExp{core.Greatest(coreArgs(values)...)}
}

// Least creates a LEAST expression (MIN on SQLite). Arguments follow the same
// rule as Greatest.
func Least(values ...any) GreatestLeastExp {
	return greatestLeastExp{core.Least(coreArgs(values)...)}
}

// ============================================================================
// Concatenation
// ============================================================================

// ConcatExp is a string concatenation: CONCAT(...) on MySQL, the || operator
// on PostgreSQL and SQLite.
//
//	relica.Concat("first_name", "' '", "last_name").As("full_name")
//	// PostgreSQL: "first_name" || ' ' || "last_name" AS "full_name"
type ConcatExp interface {
	Expression

	// As appends AS alias. Use it in a SELECT list only.
	As(alias string) ConcatExp
}

type concatExp struct{ *core.ConcatExp }

func (c concatExp) coreExpression() core.Expression { return c.ConcatExp }

func (c concatExp) As(alias string) ConcatExp {
	c.ConcatExp.As(alias)
	return c
}

func (c concatExp) Err() error { return core.ExpressionError(c.ConcatExp) }

// Concat creates a string concatenation expression.
// A string argument is a column name; a string starting with ' or " is a SQL
// literal used verbatim; an Expression is rendered in place; any other value
// is a parameter.
func Concat(values ...any) ConcatExp { return concatExp{core.Concat(coreArgs(values)...)} }
