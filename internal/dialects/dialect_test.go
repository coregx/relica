// Package dialects provides database-specific SQL dialect implementations.
// This test file covers GetDialect, RegisterDialect, and all dialect methods.
package dialects

import (
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// GetDialect / RegisterDialect
// ---------------------------------------------------------------------------

func TestGetDialect_BuiltinDialects(t *testing.T) {
	tests := []struct {
		name       string
		dialectKey string
	}{
		{"postgres", "postgres"},
		{"postgresql alias", "postgresql"},
		{"mysql", "mysql"},
		{"sqlite", "sqlite"},
		{"sqlite3 alias", "sqlite3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := GetDialect(tt.dialectKey)
			if d == nil {
				t.Fatal("expected non-nil")
			}
		})
	}
}

func TestGetDialect_UnknownPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown dialect")
		}
	}()
	GetDialect("unknown_db")
}

func TestGetDialect_EmptyNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty dialect name")
		}
	}()
	GetDialect("")
}

func TestRegisterDialect_CustomDialect(t *testing.T) {
	// Register a custom stub dialect and verify it is retrievable.
	RegisterDialect("stub_custom", &stubDialect{quote: "[", unquote: "]", ph: "@@"})
	d := GetDialect("stub_custom")
	if d == nil {
		t.Fatal("expected non-nil")
	}
	if got, want := d.QuoteIdentifier("users"), "[users]"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := d.Placeholder(1), "@@"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRegisterDialect_Overwrite(t *testing.T) {
	// Registering under the same name should replace the previous entry.
	RegisterDialect("overwrite_test", &stubDialect{ph: "first"})
	RegisterDialect("overwrite_test", &stubDialect{ph: "second"})
	d := GetDialect("overwrite_test")
	if got, want := d.Placeholder(1), "second"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Helper: stubDialect used in registration tests
// ---------------------------------------------------------------------------

type stubDialect struct {
	quote   string
	unquote string
	ph      string
}

func (s *stubDialect) QuoteIdentifier(id string) string {
	return s.quote + id + s.unquote
}

func (s *stubDialect) Placeholder(_ int) string {
	return s.ph
}

func (s *stubDialect) UpsertSQL(_ Upsert) string {
	return ""
}

// ---------------------------------------------------------------------------
// PostgresDialect — QuoteIdentifier
// ---------------------------------------------------------------------------

func TestPostgresDialect_QuoteIdentifier(t *testing.T) {
	d := &PostgresDialect{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple identifier",
			input: "users",
			want:  `"users"`,
		},
		{
			name:  "identifier with space",
			input: "user name",
			want:  `"user name"`,
		},
		{
			name:  "identifier with double quote",
			input: `user"name`,
			want:  `"user""name"`,
		},
		{
			name:  "identifier with multiple double quotes",
			input: `a"b"c`,
			want:  `"a""b""c"`,
		},
		{
			name:  "empty string",
			input: "",
			want:  `""`,
		},
		{
			name:  "reserved word",
			input: "select",
			want:  `"select"`,
		},
		{
			name:  "identifier with backtick",
			input: "my`col",
			want:  `"my` + "`" + `col"`,
		},
		{
			name:  "identifier with only quotes",
			input: `""`,
			// Each " is escaped to "", so "" -> """", then wrapped: "" + """" + "" = """"""
			want: `""""""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.QuoteIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PostgresDialect — Placeholder
// ---------------------------------------------------------------------------

func TestPostgresDialect_Placeholder(t *testing.T) {
	d := &PostgresDialect{}

	tests := []struct {
		index int
		want  string
	}{
		{1, "$1"},
		{2, "$2"},
		{10, "$10"},
		{100, "$100"},
		{0, "$0"},
		{-1, "$-1"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("index_%d", tt.index), func(t *testing.T) {
			got := d.Placeholder(tt.index)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresDialect_Placeholder_Sequential(t *testing.T) {
	d := &PostgresDialect{}

	// Verify sequential placeholders produce distinct values — critical for
	// PostgreSQL's positional parameter style.
	previous := ""
	for i := 1; i <= 20; i++ {
		got := d.Placeholder(i)
		if got == previous {
			t.Errorf("placeholder at index %d must differ from previous", i)
		}
		if want := fmt.Sprintf("$%d", i); got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		previous = got
	}
}

// ---------------------------------------------------------------------------
// MySQLDialect — QuoteIdentifier
// ---------------------------------------------------------------------------

func TestMySQLDialect_QuoteIdentifier(t *testing.T) {
	d := &MySQLDialect{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple identifier",
			input: "users",
			want:  "`users`",
		},
		{
			name:  "identifier with space",
			input: "user name",
			want:  "`user name`",
		},
		{
			name:  "identifier with backtick",
			input: "my`col",
			want:  "`my``col`",
		},
		{
			name:  "identifier with multiple backticks",
			input: "a`b`c",
			want:  "`a``b``c`",
		},
		{
			name:  "empty string",
			input: "",
			want:  "``",
		},
		{
			name:  "reserved word",
			input: "select",
			want:  "`select`",
		},
		{
			name:  "identifier with double quote (not escaped)",
			input: `user"name`,
			want:  "`user\"name`",
		},
		{
			name:  "only backticks",
			input: "``",
			// Each ` is escaped to ``, so `` -> ````, then wrapped: ` + ```` + ` = ``````
			want: "``````",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.QuoteIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MySQLDialect — Placeholder
// ---------------------------------------------------------------------------

func TestMySQLDialect_Placeholder(t *testing.T) {
	d := &MySQLDialect{}

	// MySQL always returns "?" regardless of the index.
	tests := []int{1, 2, 10, 100, 0, -1}

	for _, idx := range tests {
		t.Run(fmt.Sprintf("index_%d", idx), func(t *testing.T) {
			got := d.Placeholder(idx)
			if got != "?" {
				t.Errorf("got %v, want %v", got, "?")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SQLiteDialect — QuoteIdentifier
// ---------------------------------------------------------------------------

func TestSQLiteDialect_QuoteIdentifier(t *testing.T) {
	d := &SQLiteDialect{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple identifier",
			input: "users",
			want:  "`users`",
		},
		{
			name:  "identifier with space",
			input: "user name",
			want:  "`user name`",
		},
		{
			name:  "identifier with double quote (not escaped)",
			input: `user"name`,
			want:  "`user\"name`",
		},
		{
			name:  "identifier with multiple double quotes (not escaped)",
			input: `a"b"c`,
			want:  "`a\"b\"c`",
		},
		{
			name:  "empty string",
			input: "",
			want:  "``",
		},
		{
			name:  "reserved word",
			input: "table",
			want:  "`table`",
		},
		{
			name:  "identifier with backtick",
			input: "my`col",
			want:  "`my``col`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.QuoteIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SQLiteDialect — Placeholder
// ---------------------------------------------------------------------------

func TestSQLiteDialect_Placeholder(t *testing.T) {
	d := &SQLiteDialect{}

	// SQLite always returns "?" regardless of the index.
	tests := []int{1, 2, 10, 100, 0, -1}

	for _, idx := range tests {
		t.Run(fmt.Sprintf("index_%d", idx), func(t *testing.T) {
			got := d.Placeholder(idx)
			if got != "?" {
				t.Errorf("got %v, want %v", got, "?")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cross-dialect: Placeholder distinctness
// ---------------------------------------------------------------------------

// TestPlaceholder_DialectDifferences verifies that PostgreSQL produces unique
// positional parameters while MySQL and SQLite always return "?".
func TestPlaceholder_DialectDifferences(t *testing.T) {
	pg := GetDialect("postgres")
	my := GetDialect("mysql")
	sq := GetDialect("sqlite")

	for i := 1; i <= 5; i++ {
		t.Run(fmt.Sprintf("index_%d", i), func(t *testing.T) {
			pgPh := pg.Placeholder(i)
			myPh := my.Placeholder(i)
			sqPh := sq.Placeholder(i)

			if want := fmt.Sprintf("$%d", i); pgPh != want {
				t.Errorf("postgres must use positional $N: got %v, want %v", pgPh, want)
			}
			if myPh != "?" {
				t.Errorf("mysql must use ?: got %v, want %v", myPh, "?")
			}
			if sqPh != "?" {
				t.Errorf("sqlite must use ?: got %v, want %v", sqPh, "?")
			}

			// Postgres placeholders must be unique across indices.
			if i > 1 {
				if pg.Placeholder(i-1) == pgPh {
					t.Errorf("postgres placeholder at %d must differ from index %d", i, i-1)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cross-dialect: QuoteIdentifier consistency
// ---------------------------------------------------------------------------

// TestQuoteIdentifier_DialectDifferences checks that postgres uses double
// quotes while mysql and sqlite use backticks.
func TestQuoteIdentifier_DialectDifferences(t *testing.T) {
	tests := []struct {
		dialectKey string
		input      string
		wantPrefix byte
		wantSuffix byte
	}{
		{"postgres", "users", '"', '"'},
		{"postgresql", "orders", '"', '"'},
		{"mysql", "users", '`', '`'},
		{"sqlite", "users", '`', '`'},
		{"sqlite3", "orders", '`', '`'},
	}

	for _, tt := range tests {
		t.Run(tt.dialectKey+"_"+tt.input, func(t *testing.T) {
			d := GetDialect(tt.dialectKey)
			got := d.QuoteIdentifier(tt.input)
			if len(got) < 2 {
				t.Fatalf("quoted identifier must be at least 2 chars")
			}
			if got[0] != tt.wantPrefix {
				t.Errorf("wrong opening quote character: got %v, want %v", got[0], tt.wantPrefix)
			}
			if got[len(got)-1] != tt.wantSuffix {
				t.Errorf("wrong closing quote character: got %v, want %v", got[len(got)-1], tt.wantSuffix)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Dialect interface compliance
// ---------------------------------------------------------------------------

// TestDialectInterface verifies that all concrete types satisfy the Dialect
// interface at compile time (also checked statically, but explicit here for
// documentation value in test output).
func TestDialectInterface_Compliance(t *testing.T) {
	var _ Dialect = (*PostgresDialect)(nil)
	var _ Dialect = (*MySQLDialect)(nil)
	var _ Dialect = (*SQLiteDialect)(nil)

	// If the above compile-time assertions pass, the test passes.
	t.Log("all dialect types satisfy the Dialect interface")
}

// ---------------------------------------------------------------------------
// init() registration verification
// ---------------------------------------------------------------------------

// TestInit_AllDialectsRegistered ensures every dialect alias registered in
// init() functions is available via GetDialect without panicking.
func TestInit_AllDialectsRegistered(t *testing.T) {
	aliases := []string{
		"postgres",
		"postgresql",
		"mysql",
		"sqlite",
		"sqlite3",
	}

	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("unexpected panic for alias %q: %v", alias, r)
					}
				}()
				d := GetDialect(alias)
				if d == nil {
					t.Fatal("expected non-nil")
				}
			}()
		})
	}
}
