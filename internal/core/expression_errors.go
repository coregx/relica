package core

// errCarrier is implemented by expressions that may hold a construction error:
// *LikeExp (EscapeChars with an odd argument count) and the public wrappers in
// package relica, which delegate to ExpressionError.
type errCarrier interface {
	Err() error
}

// composite is implemented by expressions that contain other values, which
// may themselves be expressions.
type composite interface {
	nested() []any
}

// ExpressionError returns the first construction error carried by v or by any
// expression nested inside it, or nil.
//
// Build cannot return an error, so an invalid expression renders as an empty
// SQL fragment, and an empty condition is skipped. Without this check a
// DELETE with an invalid filter ran as an unconditional DELETE. Every builder
// method that accepts a condition, a computed column or a SET value calls this
// and stores the result as the query's build error, so execution returns the
// error and sends nothing to the database.
func ExpressionError(v any) error {
	switch x := v.(type) {
	case nil:
		return nil
	case *selectQueryExpression:
		return x.query.err()
	case *SelectQuery:
		return x.err()
	case errCarrier:
		if err := x.Err(); err != nil {
			return err
		}
	}
	if c, ok := v.(composite); ok {
		for _, child := range c.nested() {
			if err := ExpressionError(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *AndOrExp) nested() []any {
	out := make([]any, len(e.Exps))
	for i, x := range e.Exps {
		out[i] = x
	}
	return out
}

func (e *NotExp) nested() []any    { return []any{e.Exp} }
func (e *ExistsExp) nested() []any { return []any{e.Exp} }

func (e HashExp) nested() []any {
	out := make([]any, 0, len(e))
	for _, v := range e {
		out = append(out, v)
	}
	return out
}

func (e *CompareExp) nested() []any { return []any{e.Value} }
func (e *InExp) nested() []any      { return e.Values }
func (e *BetweenExp) nested() []any { return []any{e.From, e.To} }

func (c *CaseExp) nested() []any {
	out := make([]any, 0, len(c.whens)*2+1)
	for _, w := range c.whens {
		out = append(out, w.condition, w.result)
	}
	return append(out, c.elseValue)
}

func (c *CoalesceExp) nested() []any      { return c.values }
func (n *NullIfExp) nested() []any        { return []any{n.expr1, n.expr2} }
func (g *GreatestLeastExp) nested() []any { return g.values }
func (c *ConcatExp) nested() []any        { return c.values }
