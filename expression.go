package relica

import "github.com/coregx/relica/internal/core"

// Expression is a SQL fragment together with its bound arguments. It can be
// used as a WHERE or HAVING condition, a JOIN ON clause, a computed column
// (SelectQuery.SelectSub), an ORDER BY or GROUP BY term, or a value in
// UpdateQuery.Set.
//
// Expressions come from this package's constructors — Eq, In, Between, Like,
// And, Or, Not, Exists, HashExp, Case, Coalesce, NewExp and the rest — and from
// SelectQuery.AsExpression. They compose freely:
//
//	db.Select().From("users").Where(relica.And(
//	    relica.Eq("status", 1),
//	    relica.Or(relica.GreaterThan("age", 18), relica.HashExp{"verified": true}),
//	)).All(&users)
//
// The interface is sealed: rendering depends on the database dialect, so it
// cannot be implemented outside Relica. For SQL that no constructor covers,
// use NewExp with ? placeholders.
//
// An expression built with invalid input (for example Like(...).EscapeChars
// with an odd number of arguments) makes the query that uses it fail with that
// error; nothing is sent to the database.
type Expression interface {
	// coreExpression returns the internal representation. It is unexported
	// to seal the interface (ADR-016).
	coreExpression() core.Expression
}

// expImpl is the Expression returned by the constructors. Embedding the internal
// expression promotes its Build method, so the value is still understood by
// the query builder where it is stored as any: a Set map value, a CASE branch,
// a Coalesce argument, a HashExp value. Build stays out of the public docs
// because expImpl is unexported.
type expImpl struct{ core.Expression }

func (e expImpl) coreExpression() core.Expression { return e.Expression }

// Err reports a construction error anywhere inside the expression; the query
// builder checks it before rendering.
func (e expImpl) Err() error { return core.ExpressionError(e.Expression) }

// wrapExp wraps an internal expression for the public API.
func wrapExp(e core.Expression) Expression {
	if e == nil {
		return nil
	}
	return expImpl{e}
}

// coreExp returns the internal form of a public expression (nil stays nil).
func coreExp(e Expression) core.Expression {
	if e == nil {
		return nil
	}
	return e.coreExpression()
}

// coreExps converts a list of public expressions.
func coreExps(es []Expression) []core.Expression {
	out := make([]core.Expression, len(es))
	for i, e := range es {
		out[i] = coreExp(e)
	}
	return out
}

// coreArg converts a value that may be a public expression before it is passed
// to the internal builder as any. HashExp is a map, not an internal
// expression, so it must be converted; other expressions are unwrapped so the
// builder sees its own types. Everything else passes through unchanged.
func coreArg(v any) any {
	if e, ok := v.(Expression); ok {
		return coreExp(e)
	}
	return v
}

// coreArgs converts every element with coreArg. It returns the input slice
// unchanged (no allocation) when it holds no public expression.
func coreArgs(vs []any) []any {
	for i, v := range vs {
		if !isExpression(v) {
			continue
		}
		out := make([]any, len(vs))
		copy(out, vs[:i])
		for j := i; j < len(vs); j++ {
			out[j] = coreArg(vs[j])
		}
		return out
	}
	return vs
}

func isExpression(v any) bool {
	_, ok := v.(Expression)
	return ok
}

// ============================================================================
// HashExp
// ============================================================================

// HashExp is a set of column conditions combined with AND, written as a map
// from column name to value:
//
//	relica.HashExp{
//	    "status":     1,                          // "status" = $1
//	    "role":       []any{"admin", "moderator"}, // "role" IN ($2, $3)
//	    "deleted_at": nil,                        // "deleted_at" IS NULL
//	    "team_id":    sub.AsExpression(),         // "team_id" = (SELECT ...)
//	}
//
// Keys are sorted, so the generated SQL is deterministic. An empty []any
// renders as an always-false condition, never as "no condition". A value that
// is an Expression is rendered in parentheses.
//
// HashExp is an Expression: pass it to Where, AndWhere, OrWhere, Having, a JOIN,
// And, Or or Not.
type HashExp map[string]any

func (h HashExp) coreExpression() core.Expression { return core.HashExp(h) }

// ============================================================================
// LikeExp
// ============================================================================

// LikeExp is a LIKE or NOT LIKE condition created by Like, NotLike, OrLike or
// OrNotLike, or a case-insensitive one created by ILike, NotILike, OrILike or
// OrNotILike. Values are escaped (%, _ and \ by default) and wrapped in % on
// both sides unless Match says otherwise:
//
//	relica.Like("name", "50%")                 // "name" LIKE '%50\%%'
//	relica.Like("file", ".txt").Match(false, true) // "file" LIKE '.txt%'
//	relica.Like("code", "a!b").EscapeChars("!", "!!")
//
// Several values are combined with AND (Like, NotLike) or OR (OrLike,
// OrNotLike).
type LikeExp interface {
	Expression

	// Match sets whether a % wildcard is added on the left and on the right
	// of each value. The default is Match(true, true): "%value%".
	Match(left, right bool) LikeExp

	// EscapeChars replaces the default escaping with explicit pairs:
	// EscapeChars(special1, replacement1, special2, replacement2, ...).
	// An odd number of arguments is a construction error: Err returns it and
	// any query using the expression fails with it.
	EscapeChars(chars ...string) LikeExp

	// Err returns the construction error, if any.
	Err() error
}

type likeExp struct{ *core.LikeExp }

func (e likeExp) coreExpression() core.Expression { return e.LikeExp }

func (e likeExp) Match(left, right bool) LikeExp {
	e.LikeExp.Match(left, right)
	return e
}

func (e likeExp) EscapeChars(chars ...string) LikeExp {
	e.LikeExp.EscapeChars(chars...)
	return e
}

// ============================================================================
// Expression constructors
// ============================================================================

// NewExp creates an expression from raw SQL with ? placeholders:
//
//	relica.NewExp("balance + ?", 10)
//	relica.NewExp("created_at > NOW() - INTERVAL '1 day'")
//
// The SQL is used verbatim: never build it from user input.
func NewExp(rawSQL string, args ...any) Expression { return wrapExp(core.NewExp(rawSQL, args...)) }

// Eq creates an equality expression (column = value). A nil value renders
// as IS NULL; an Expression value is rendered in parentheses.
func Eq(col string, value any) Expression { return wrapExp(core.Eq(col, coreArg(value))) }

// NotEq creates a not-equal expression (column <> value). A nil value renders
// as IS NOT NULL.
func NotEq(col string, value any) Expression { return wrapExp(core.NotEq(col, coreArg(value))) }

// GreaterThan creates a greater-than expression (column > value).
func GreaterThan(col string, value any) Expression {
	return wrapExp(core.GreaterThan(col, coreArg(value)))
}

// LessThan creates a less-than expression (column < value).
func LessThan(col string, value any) Expression { return wrapExp(core.LessThan(col, coreArg(value))) }

// GreaterOrEqual creates a greater-or-equal expression (column >= value).
func GreaterOrEqual(col string, value any) Expression {
	return wrapExp(core.GreaterOrEqual(col, coreArg(value)))
}

// LessOrEqual creates a less-or-equal expression (column <= value).
func LessOrEqual(col string, value any) Expression {
	return wrapExp(core.LessOrEqual(col, coreArg(value)))
}

// EqCol creates a column-to-column equality expression (col1 = col2).
// Both identifiers are quoted using the current dialect.
// Useful for JOIN ON conditions and correlated subquery WHERE clauses.
//
// Example:
//
//	relica.EqCol("orders.user_id", "users.id")  →  "orders"."user_id" = "users"."id"
func EqCol(col1, col2 string) Expression { return wrapExp(core.EqCol(col1, col2)) }

// NotEqCol creates a column-to-column inequality expression (col1 <> col2).
func NotEqCol(col1, col2 string) Expression { return wrapExp(core.NotEqCol(col1, col2)) }

// GreaterThanCol creates a column-to-column greater-than expression (col1 > col2).
func GreaterThanCol(col1, col2 string) Expression { return wrapExp(core.GreaterThanCol(col1, col2)) }

// LessThanCol creates a column-to-column less-than expression (col1 < col2).
func LessThanCol(col1, col2 string) Expression { return wrapExp(core.LessThanCol(col1, col2)) }

// GreaterOrEqualCol creates a column-to-column >= expression (col1 >= col2).
func GreaterOrEqualCol(col1, col2 string) Expression {
	return wrapExp(core.GreaterOrEqualCol(col1, col2))
}

// LessOrEqualCol creates a column-to-column <= expression (col1 <= col2).
func LessOrEqualCol(col1, col2 string) Expression { return wrapExp(core.LessOrEqualCol(col1, col2)) }

// IsNull creates an IS NULL expression. Equivalent to Eq(col, nil) but more explicit.
func IsNull(col string) Expression { return wrapExp(core.Eq(col, nil)) }

// IsNotNull creates an IS NOT NULL expression. Equivalent to NotEq(col, nil) but more explicit.
func IsNotNull(col string) Expression { return wrapExp(core.NotEq(col, nil)) }

// In creates an IN expression (column IN (values...)). With no values it
// renders as an always-false condition. A single subquery value
// (SelectQuery.AsExpression) renders as column IN (SELECT ...).
func In(col string, values ...any) Expression { return wrapExp(core.In(col, coreArgs(values)...)) }

// NotIn creates a NOT IN expression. With no values it renders as no condition
// (NOT IN of an empty set is always true).
func NotIn(col string, values ...any) Expression {
	return wrapExp(core.NotIn(col, coreArgs(values)...))
}

// Between creates a BETWEEN expression (column BETWEEN low AND high).
func Between(col string, from, to any) Expression {
	return wrapExp(core.Between(col, coreArg(from), coreArg(to)))
}

// NotBetween creates a NOT BETWEEN expression.
func NotBetween(col string, from, to any) Expression {
	return wrapExp(core.NotBetween(col, coreArg(from), coreArg(to)))
}

// Like creates a LIKE expression; several values are combined with AND.
func Like(col string, values ...string) LikeExp { return likeExp{core.Like(col, values...)} }

// NotLike creates a NOT LIKE expression; several values are combined with AND.
func NotLike(col string, values ...string) LikeExp { return likeExp{core.NotLike(col, values...)} }

// OrLike creates a LIKE expression matching any of the values (OR).
func OrLike(col string, values ...string) LikeExp { return likeExp{core.OrLike(col, values...)} }

// OrNotLike creates a NOT LIKE expression combined with OR.
func OrNotLike(col string, values ...string) LikeExp {
	return likeExp{core.OrNotLike(col, values...)}
}

// ILike creates a case-insensitive LIKE expression; several values are
// combined with AND. Escaping and Match work as for Like.
//
// PostgreSQL renders "col" ILIKE ?. MySQL and SQLite have no ILIKE, and their
// LIKE is not reliably case-insensitive (MySQL follows the column collation,
// *_bin is case-sensitive; SQLite folds ASCII only and honors
// PRAGMA case_sensitive_like), so both sides are folded by the server:
// LOWER(col) LIKE LOWER(?). LOWER(col) cannot use a plain index on col.
func ILike(col string, values ...string) LikeExp { return likeExp{core.ILike(col, values...)} }

// NotILike creates a case-insensitive NOT LIKE expression; several values are
// combined with AND.
func NotILike(col string, values ...string) LikeExp {
	return likeExp{core.NotILike(col, values...)}
}

// OrILike creates a case-insensitive LIKE expression matching any of the
// values (OR).
func OrILike(col string, values ...string) LikeExp { return likeExp{core.OrILike(col, values...)} }

// OrNotILike creates a case-insensitive NOT LIKE expression combined with OR.
func OrNotILike(col string, values ...string) LikeExp {
	return likeExp{core.OrNotILike(col, values...)}
}

// And combines expressions with AND, each in parentheses. Nil and empty
// expressions are skipped.
func And(exps ...Expression) Expression { return wrapExp(core.And(coreExps(exps)...)) }

// Or combines expressions with OR, each in parentheses. Nil and empty
// expressions are skipped.
func Or(exps ...Expression) Expression { return wrapExp(core.Or(coreExps(exps)...)) }

// Not negates an expression: NOT (expression).
func Not(e Expression) Expression { return wrapExp(core.Not(coreExp(e))) }

// Exists creates an EXISTS subquery expression:
//
//	relica.Exists(db.Select("1").From("orders").Where(relica.EqCol("orders.user_id", "users.id")).AsExpression())
func Exists(e Expression) Expression { return wrapExp(core.Exists(coreExp(e))) }

// NotExists creates a NOT EXISTS subquery expression.
func NotExists(e Expression) Expression { return wrapExp(core.NotExists(coreExp(e))) }
