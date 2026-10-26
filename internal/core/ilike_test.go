package core

import (
	"reflect"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// Case-insensitive LIKE renders per dialect: PostgreSQL has ILIKE; MySQL and
// SQLite have no ILIKE, so both sides are folded by the server with LOWER().
// LIKE on MySQL follows the column collation (case-sensitive on *_bin) and
// SQLite folds ASCII only, so plain LIKE is not a case-insensitive match there.
func TestILike_SQLPerDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		exp     *LikeExp
		want    string
	}{
		{"pg ILike", "postgres", ILike("name", "al"), `"name" ILIKE ?`},
		{"pg NotILike", "postgres", NotILike("name", "al"), `"name" NOT ILIKE ?`},
		{"pg OrILike", "postgres", OrILike("name", "al", "be"), `"name" ILIKE ? OR "name" ILIKE ?`},
		{"pg OrNotILike", "postgres", OrNotILike("name", "al", "be"), `"name" NOT ILIKE ? OR "name" NOT ILIKE ?`},
		{"pg qualified", "postgres", ILike("h.cpu_model", "xeon"), `"h"."cpu_model" ILIKE ?`},
		{"mysql ILike", "mysql", ILike("name", "al"), "LOWER(`name`) LIKE LOWER(?)"},
		{"mysql NotILike", "mysql", NotILike("name", "al"), "LOWER(`name`) NOT LIKE LOWER(?)"},
		{"mysql qualified", "mysql", ILike("h.cpu_model", "xeon"), "LOWER(`h`.`cpu_model`) LIKE LOWER(?)"},
		{"sqlite ILike", "sqlite", ILike("name", "al"), "LOWER(`name`) LIKE LOWER(?) ESCAPE '\\'"},
		{"sqlite OrNotILike", "sqlite", OrNotILike("name", "al", "be"),
			"LOWER(`name`) NOT LIKE LOWER(?) ESCAPE '\\' OR LOWER(`name`) NOT LIKE LOWER(?) ESCAPE '\\'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := tt.exp.Build(dialects.GetDialect(tt.dialect))
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

// Values are escaped and wrapped exactly like Like: case-insensitivity must not
// turn an escaped % or _ back into a wildcard.
func TestILike_ArgsEscapedAndWrapped(t *testing.T) {
	for _, d := range []string{"postgres", "mysql", "sqlite"} {
		t.Run(d, func(t *testing.T) {
			_, args := ILike("note", "50%", "x_Y").Build(dialects.GetDialect(d))
			want := []any{`%50\%%`, `%x\_Y%`}
			if !reflect.DeepEqual(args, want) {
				t.Errorf("args = %#v, want %#v", args, want)
			}

			_, args = ILike("file", ".TXT").Match(false, true).Build(dialects.GetDialect(d))
			if want := []any{`.TXT%`}; !reflect.DeepEqual(args, want) {
				t.Errorf("Match(false, true) args = %#v, want %#v", args, want)
			}
		})
	}
}

// Construction errors behave as for Like: Err reports them and Build renders
// nothing (the query layer turns the error into a failed query).
func TestILike_EscapeCharsError(t *testing.T) {
	e := ILike("name", "x").EscapeChars("!")
	if e.Err() == nil {
		t.Fatal("expected an error for an odd number of escape strings")
	}
	if sql, args := e.Build(dialects.GetDialect("postgres")); sql != "" || args != nil {
		t.Errorf("Build = %q, %v; want empty", sql, args)
	}
}

// No values means no condition, as for Like.
func TestILike_NoValues(t *testing.T) {
	if sql, _ := ILike("name").Build(dialects.GetDialect("mysql")); sql != "" {
		t.Errorf("Build = %q, want empty", sql)
	}
}
