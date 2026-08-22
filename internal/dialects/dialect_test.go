// Package dialects provides database-specific SQL dialect implementations.
// This test file covers GetDialect, RegisterDialect, and all dialect methods.
package dialects

import (
	"fmt"
	"strings"
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

func (s *stubDialect) UpsertSQL(_ string, _, _ []string) string {
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
// PostgresDialect — UpsertSQL
// ---------------------------------------------------------------------------

func TestPostgresDialect_UpsertSQL(t *testing.T) {
	d := &PostgresDialect{}

	tests := []struct {
		name            string
		table           string
		conflictColumns []string
		updateCols      []string
		want            string
	}{
		{
			name:            "do nothing with conflict columns",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      nil,
			want:            " ON CONFLICT (id) DO NOTHING",
		},
		{
			name:            "do nothing without conflict columns",
			table:           "users",
			conflictColumns: []string{},
			updateCols:      nil,
			want:            " ON CONFLICT DO NOTHING",
		},
		{
			name:            "do nothing nil conflict columns",
			table:           "users",
			conflictColumns: nil,
			updateCols:      nil,
			want:            " ON CONFLICT DO NOTHING",
		},
		{
			name:            "do update single column",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{"name"},
			want:            " ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name",
		},
		{
			name:            "do update multiple columns",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{"name", "email"},
			want:            " ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email",
		},
		{
			name:            "do update multiple conflict columns",
			table:           "users",
			conflictColumns: []string{"email", "username"},
			updateCols:      []string{"name"},
			want:            " ON CONFLICT (email, username) DO UPDATE SET name = EXCLUDED.name",
		},
		{
			name:            "table argument is ignored",
			table:           "orders",
			conflictColumns: []string{"order_id"},
			updateCols:      []string{"status"},
			want:            " ON CONFLICT (order_id) DO UPDATE SET status = EXCLUDED.status",
		},
		{
			name:            "do update empty update cols slice",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{},
			want:            " ON CONFLICT (id) DO UPDATE SET ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.UpsertSQL(tt.table, tt.conflictColumns, tt.updateCols)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
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
// MySQLDialect — UpsertSQL
// ---------------------------------------------------------------------------

func TestMySQLDialect_UpsertSQL(t *testing.T) {
	d := &MySQLDialect{}

	tests := []struct {
		name            string
		table           string
		conflictColumns []string
		updateCols      []string
		want            string
	}{
		{
			name:            "do nothing returns empty string",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      nil,
			want:            "",
		},
		{
			name:            "do nothing nil conflict and update cols",
			table:           "users",
			conflictColumns: nil,
			updateCols:      nil,
			want:            "",
		},
		{
			name:            "do update single column",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{"name"},
			want:            " ON DUPLICATE KEY UPDATE name = VALUES(name)",
		},
		{
			name:            "do update multiple columns",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{"name", "email"},
			want:            " ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email)",
		},
		{
			name:  "conflict columns are ignored in MySQL syntax",
			table: "users",
			// MySQL ON DUPLICATE KEY does not reference conflict columns in the clause.
			conflictColumns: []string{"email", "username"},
			updateCols:      []string{"name"},
			want:            " ON DUPLICATE KEY UPDATE name = VALUES(name)",
		},
		{
			name:            "table argument is ignored",
			table:           "products",
			conflictColumns: []string{"sku"},
			updateCols:      []string{"price", "stock"},
			want:            " ON DUPLICATE KEY UPDATE price = VALUES(price), stock = VALUES(stock)",
		},
		{
			name:            "do update empty update cols",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{},
			want:            " ON DUPLICATE KEY UPDATE ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.UpsertSQL(tt.table, tt.conflictColumns, tt.updateCols)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
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
			input: "table",
			want:  `"table"`,
		},
		{
			name:  "identifier with backtick (not escaped)",
			input: "my`col",
			want:  `"my` + "`" + `col"`,
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
// SQLiteDialect — UpsertSQL
// ---------------------------------------------------------------------------

func TestSQLiteDialect_UpsertSQL(t *testing.T) {
	d := &SQLiteDialect{}

	tests := []struct {
		name            string
		table           string
		conflictColumns []string
		updateCols      []string
		want            string
	}{
		{
			name:            "do nothing with conflict columns",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      nil,
			want:            " ON CONFLICT (id) DO NOTHING",
		},
		{
			name:            "do nothing without conflict columns",
			table:           "users",
			conflictColumns: []string{},
			updateCols:      nil,
			want:            " ON CONFLICT DO NOTHING",
		},
		{
			name:            "do nothing nil conflict columns",
			table:           "users",
			conflictColumns: nil,
			updateCols:      nil,
			want:            " ON CONFLICT DO NOTHING",
		},
		{
			name:            "do update single column",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{"name"},
			want:            " ON CONFLICT (id) DO UPDATE SET name = excluded.name",
		},
		{
			name:            "do update multiple columns",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{"name", "email"},
			want:            " ON CONFLICT (id) DO UPDATE SET name = excluded.name, email = excluded.email",
		},
		{
			name:            "do update multiple conflict columns",
			table:           "users",
			conflictColumns: []string{"email", "username"},
			updateCols:      []string{"name"},
			want:            " ON CONFLICT (email, username) DO UPDATE SET name = excluded.name",
		},
		{
			name:            "table argument is ignored",
			table:           "events",
			conflictColumns: []string{"event_id"},
			updateCols:      []string{"payload"},
			want:            " ON CONFLICT (event_id) DO UPDATE SET payload = excluded.payload",
		},
		{
			name:  "sqlite uses lowercase excluded unlike postgres EXCLUDED",
			table: "users",
			// SQLite uses lowercase "excluded", Postgres uses uppercase "EXCLUDED"
			conflictColumns: []string{"id"},
			updateCols:      []string{"score"},
			want:            " ON CONFLICT (id) DO UPDATE SET score = excluded.score",
		},
		{
			name:            "do update empty update cols",
			table:           "users",
			conflictColumns: []string{"id"},
			updateCols:      []string{},
			want:            " ON CONFLICT (id) DO UPDATE SET ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.UpsertSQL(tt.table, tt.conflictColumns, tt.updateCols)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
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

// TestQuoteIdentifier_DialectDifferences checks that postgres and sqlite both
// use double quotes while mysql uses backticks.
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
		{"sqlite", "users", '"', '"'},
		{"sqlite3", "orders", '"', '"'},
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
// Cross-dialect: UpsertSQL — DO NOTHING semantics
// ---------------------------------------------------------------------------

// TestUpsertSQL_DoNothingSemantics checks dialect-specific DO NOTHING behavior.
func TestUpsertSQL_DoNothingSemantics(t *testing.T) {
	conflictCols := []string{"id"}

	t.Run("postgres do nothing", func(t *testing.T) {
		d := GetDialect("postgres")
		got := d.UpsertSQL("t", conflictCols, nil)
		if got != " ON CONFLICT (id) DO NOTHING" {
			t.Errorf("got %v, want %v", got, " ON CONFLICT (id) DO NOTHING")
		}
	})

	t.Run("sqlite do nothing", func(t *testing.T) {
		d := GetDialect("sqlite")
		got := d.UpsertSQL("t", conflictCols, nil)
		if got != " ON CONFLICT (id) DO NOTHING" {
			t.Errorf("got %v, want %v", got, " ON CONFLICT (id) DO NOTHING")
		}
	})

	t.Run("mysql do nothing returns empty", func(t *testing.T) {
		d := GetDialect("mysql")
		got := d.UpsertSQL("t", conflictCols, nil)
		if got != "" {
			t.Errorf("mysql has no native DO NOTHING support: got %v, want %v", got, "")
		}
	})
}

// ---------------------------------------------------------------------------
// Cross-dialect: EXCLUDED vs excluded keyword
// ---------------------------------------------------------------------------

// TestUpsertSQL_ExcludedKeywordCase verifies that postgres uses uppercase
// EXCLUDED while sqlite uses lowercase excluded.
func TestUpsertSQL_ExcludedKeywordCase(t *testing.T) {
	conflictCols := []string{"id"}
	updateCols := []string{"name"}

	t.Run("postgres uses EXCLUDED uppercase", func(t *testing.T) {
		d := GetDialect("postgres")
		got := d.UpsertSQL("t", conflictCols, updateCols)
		if !strings.Contains(got, "EXCLUDED.name") {
			t.Errorf("postgres must use uppercase EXCLUDED: %q does not contain %q", got, "EXCLUDED.name")
		}
		if strings.Contains(got, "excluded.name") {
			t.Errorf("%q should not contain %q", got, "excluded.name")
		}
	})

	t.Run("sqlite uses excluded lowercase", func(t *testing.T) {
		d := GetDialect("sqlite")
		got := d.UpsertSQL("t", conflictCols, updateCols)
		if !strings.Contains(got, "excluded.name") {
			t.Errorf("sqlite must use lowercase excluded: %q does not contain %q", got, "excluded.name")
		}
		if strings.Contains(got, "EXCLUDED.name") {
			t.Errorf("%q should not contain %q", got, "EXCLUDED.name")
		}
	})
}

// ---------------------------------------------------------------------------
// buildUpdateSet (internal helper — tested via UpsertSQL output)
// ---------------------------------------------------------------------------

// TestBuildUpdateSet_ViaPostgresUpsert verifies the internal buildUpdateSet
// helper produces correct col = EXCLUDED.col pairs via the public API.
func TestBuildUpdateSet_ViaPostgresUpsert(t *testing.T) {
	d := &PostgresDialect{}

	tests := []struct {
		name       string
		updateCols []string
		want       string
	}{
		{
			name:       "single column",
			updateCols: []string{"name"},
			want:       " ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name",
		},
		{
			name:       "two columns",
			updateCols: []string{"name", "email"},
			want:       " ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email",
		},
		{
			name:       "three columns",
			updateCols: []string{"a", "b", "c"},
			want:       " ON CONFLICT (id) DO UPDATE SET a = EXCLUDED.a, b = EXCLUDED.b, c = EXCLUDED.c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.UpsertSQL("users", []string{"id"}, tt.updateCols)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
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
