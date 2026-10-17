package core

import (
	"errors"
	"testing"
)

type carrier struct{ err error }

func (c carrier) Err() error { return c.err }

func TestExpressionError_FindsNestedConstructionErrors(t *testing.T) {
	bad := Like("n", "x").EscapeChars("!")
	wantErr := bad.Err()
	if wantErr == nil {
		t.Fatal("precondition: odd EscapeChars must set Err()")
	}
	sqErr := errors.New("subquery build error")
	brokenSub := &SelectQuery{buildErr: sqErr}

	tests := []struct {
		name string
		v    any
		want error
	}{
		{"LikeExp", bad, wantErr},
		{"And", And(Eq("a", 1), bad), wantErr},
		{"Or", Or(bad), wantErr},
		{"Not", Not(bad), wantErr},
		{"Exists", Exists(bad), wantErr},
		{"HashExp value", HashExp{"a": 1, "b": bad}, wantErr},
		{"Compare value", Eq("a", bad), wantErr},
		{"In values", In("a", 1, bad), wantErr},
		{"Between", Between("a", 1, bad), wantErr},
		{"Case condition", CaseWhen().When(bad, 1), wantErr},
		{"Case result", Case("c").When(1, bad), wantErr},
		{"Case else", Case("c").When(1, 2).Else(bad), wantErr},
		{"Coalesce", Coalesce("a", bad), wantErr},
		{"NullIf", NullIf(bad, "b"), wantErr},
		{"Greatest", Greatest("a", bad), wantErr},
		{"Concat", Concat("a", bad), wantErr},
		{"subquery expression", brokenSub.AsExpression(), sqErr},
		{"subquery value", In("id", brokenSub), sqErr},
		{"external carrier", carrier{err: wantErr}, wantErr},

		{"nil", nil, nil},
		{"plain value", 42, nil},
		{"valid tree", And(Eq("a", 1), Or(Like("n", "x"), HashExp{"b": In("c", 1, 2)})), nil},
		{"carrier without error", carrier{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpressionError(tt.v); !errors.Is(got, tt.want) {
				t.Errorf("ExpressionError() = %v, want %v", got, tt.want)
			}
		})
	}
}
