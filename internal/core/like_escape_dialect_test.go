package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// SQLite LIKE has no default escape character, so the backslash escaping that
// LikeExp applies (50% → 50\%) only works with an explicit ESCAPE '\'.
// PostgreSQL and MySQL use backslash by default; MySQL must not get the clause
// ('\' is not a complete string literal there).
func TestLikeExp_EscapeClausePerDialect(t *testing.T) {
	tests := []struct {
		dialect string
		exp     *LikeExp
		want    string
	}{
		{"sqlite", Like("note", "50%"), "`note` LIKE ? ESCAPE '\\'"},
		{"sqlite", OrNotLike("note", "a", "b"), "`note` NOT LIKE ? ESCAPE '\\' OR `note` NOT LIKE ? ESCAPE '\\'"},
		{"postgres", Like("note", "50%"), `"note" LIKE ?`},
		{"mysql", Like("note", "50%"), "`note` LIKE ?"},
	}
	for _, tt := range tests {
		t.Run(tt.dialect, func(t *testing.T) {
			got, args := tt.exp.Build(dialects.GetDialect(tt.dialect))
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
			if len(args) == 0 {
				t.Error("no args")
			}
		})
	}
}
