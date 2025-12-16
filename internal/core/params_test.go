// Package core provides the core database functionality including connection management,
// query building, statement caching, and result scanning for Relica.
package core

import (
	"regexp"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

func TestProcessSQL_SingleParameter(t *testing.T) {
	// Create a test DB with PostgreSQL dialect.
	db := &DB{
		dialect: dialects.GetDialect("postgres"),
	}

	sql := "SELECT * FROM users WHERE id={:id}"
	result, params := db.processSQL(sql)

	expectedSQL := "SELECT * FROM users WHERE id=$1"
	if result != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, result)
	}

	if len(params) != 1 || params[0] != "id" {
		t.Errorf("Expected params [id], got %v", params)
	}
}

func TestProcessSQL_MultipleParameters(t *testing.T) {
	db := &DB{
		dialect: dialects.GetDialect("postgres"),
	}

	sql := "SELECT * FROM users WHERE id={:id} AND status={:status} AND role={:role}"
	result, params := db.processSQL(sql)

	expectedSQL := "SELECT * FROM users WHERE id=$1 AND status=$2 AND role=$3"
	if result != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, result)
	}

	expectedParams := []string{"id", "status", "role"}
	if len(params) != len(expectedParams) {
		t.Fatalf("Expected %d params, got %d", len(expectedParams), len(params))
	}

	for i, expected := range expectedParams {
		if params[i] != expected {
			t.Errorf("Expected param[%d] = %q, got %q", i, expected, params[i])
		}
	}
}

func TestProcessSQL_RepeatedParameter(t *testing.T) {
	db := &DB{
		dialect: dialects.GetDialect("postgres"),
	}

	sql := "SELECT * FROM users WHERE id={:id} OR parent_id={:id}"
	result, params := db.processSQL(sql)

	expectedSQL := "SELECT * FROM users WHERE id=$1 OR parent_id=$2"
	if result != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, result)
	}

	// Parameter should appear twice in the list.
	expectedParams := []string{"id", "id"}
	if len(params) != len(expectedParams) {
		t.Fatalf("Expected %d params, got %d", len(expectedParams), len(params))
	}

	for i, expected := range expectedParams {
		if params[i] != expected {
			t.Errorf("Expected param[%d] = %q, got %q", i, expected, params[i])
		}
	}
}

func TestProcessSQL_NoParameters(t *testing.T) {
	db := &DB{
		dialect: dialects.GetDialect("postgres"),
	}

	sql := "SELECT * FROM users WHERE status = 'active'"
	result, params := db.processSQL(sql)

	if result != sql {
		t.Errorf("Expected SQL to be unchanged, got %q", result)
	}

	if len(params) != 0 {
		t.Errorf("Expected 0 params, got %d", len(params))
	}
}

func TestProcessSQL_MySQLDialect(t *testing.T) {
	db := &DB{
		dialect: dialects.GetDialect("mysql"),
	}

	sql := "SELECT * FROM users WHERE id={:id} AND status={:status}"
	result, params := db.processSQL(sql)

	// MySQL uses ? for placeholders.
	expectedSQL := "SELECT * FROM users WHERE id=? AND status=?"
	if result != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, result)
	}

	expectedParams := []string{"id", "status"}
	if len(params) != len(expectedParams) {
		t.Fatalf("Expected %d params, got %d", len(expectedParams), len(params))
	}

	for i, expected := range expectedParams {
		if params[i] != expected {
			t.Errorf("Expected param[%d] = %q, got %q", i, expected, params[i])
		}
	}
}

func TestProcessSQL_SQLiteDialect(t *testing.T) {
	db := &DB{
		dialect: dialects.GetDialect("sqlite3"),
	}

	sql := "SELECT * FROM users WHERE id={:id}"
	result, params := db.processSQL(sql)

	// SQLite uses ? for placeholders.
	expectedSQL := "SELECT * FROM users WHERE id=?"
	if result != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, result)
	}

	if len(params) != 1 || params[0] != "id" {
		t.Errorf("Expected params [id], got %v", params)
	}
}

func TestBindParams_Success(t *testing.T) {
	params := Params{
		"id":     1,
		"status": "active",
		"role":   "admin",
	}

	paramNames := []string{"id", "status", "role"}

	values, err := bindParams(params, paramNames)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}

	if values[0] != 1 {
		t.Errorf("Expected values[0] = 1, got %v", values[0])
	}
	if values[1] != "active" {
		t.Errorf("Expected values[1] = 'active', got %v", values[1])
	}
	if values[2] != "admin" {
		t.Errorf("Expected values[2] = 'admin', got %v", values[2])
	}
}

func TestBindParams_RepeatedParameter(t *testing.T) {
	params := Params{
		"id": 123,
	}

	paramNames := []string{"id", "id", "id"}

	values, err := bindParams(params, paramNames)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}

	// All values should be 123.
	for i, v := range values {
		if v != 123 {
			t.Errorf("Expected values[%d] = 123, got %v", i, v)
		}
	}
}

func TestBindParams_MissingParameter(t *testing.T) {
	params := Params{
		"id": 1,
	}

	paramNames := []string{"id", "status"}

	_, err := bindParams(params, paramNames)
	if err == nil {
		t.Fatal("Expected error for missing parameter, got nil")
	}

	expectedError := "missing parameter: status"
	if err.Error() != expectedError {
		t.Errorf("Expected error %q, got %q", expectedError, err.Error())
	}
}

func TestBindParams_EmptyParams(t *testing.T) {
	params := Params{}
	paramNames := []string{}

	values, err := bindParams(params, paramNames)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(values) != 0 {
		t.Errorf("Expected 0 values, got %d", len(values))
	}
}

func TestNamedPlaceholderRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single parameter",
			input:    "WHERE id={:id}",
			expected: []string{"{:id}"},
		},
		{
			name:     "multiple parameters",
			input:    "WHERE id={:id} AND status={:status}",
			expected: []string{"{:id}", "{:status}"},
		},
		{
			name:     "parameter with underscore",
			input:    "WHERE user_id={:user_id}",
			expected: []string{"{:user_id}"},
		},
		{
			name:     "parameter with numbers",
			input:    "WHERE field1={:field1}",
			expected: []string{"{:field1}"},
		},
		{
			name:     "no parameters",
			input:    "WHERE status = 'active'",
			expected: nil,
		},
		{
			name:     "repeated parameter",
			input:    "WHERE id={:id} OR parent_id={:id}",
			expected: []string{"{:id}", "{:id}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := namedPlaceholderRegex.FindAllString(tt.input, -1)

			if len(matches) != len(tt.expected) {
				t.Fatalf("Expected %d matches, got %d: %v", len(tt.expected), len(matches), matches)
			}

			for i, expected := range tt.expected {
				if matches[i] != expected {
					t.Errorf("Expected match[%d] = %q, got %q", i, expected, matches[i])
				}
			}
		})
	}
}

// TestNamedPlaceholderRegex_InvalidFormats tests that invalid formats are NOT matched.
func TestNamedPlaceholderRegex_InvalidFormats(t *testing.T) {
	invalidFormats := []string{
		"{:}",        // Empty name
		"{: id}",     // Space in name
		"{:id-name}", // Hyphen not allowed
		"{:id.name}", // Dot not allowed (unless in word boundary)
		"{ :id}",     // Space after {
		"{:id }",     // Space before }
		":id",        // Missing braces
		"{id}",       // Missing colon
	}

	regex := regexp.MustCompile(`\{:(\w+)\}`)

	for _, invalid := range invalidFormats {
		matches := regex.FindAllString(invalid, -1)
		if len(matches) > 0 {
			t.Errorf("Invalid format %q should not match, but found: %v", invalid, matches)
		}
	}
}

// TestProcessSQL_QuotingTableNames tests {{table}} quoting syntax.
func TestProcessSQL_QuotingTableNames(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		sql      string
		expected string
	}{
		{
			name:     "PostgreSQL single table",
			dialect:  "postgres",
			sql:      "SELECT * FROM {{users}}",
			expected: `SELECT * FROM "users"`,
		},
		{
			name:     "MySQL single table",
			dialect:  "mysql",
			sql:      "SELECT * FROM {{users}}",
			expected: "SELECT * FROM `users`",
		},
		{
			name:     "SQLite single table",
			dialect:  "sqlite3",
			sql:      "SELECT * FROM {{users}}",
			expected: `SELECT * FROM "users"`,
		},
		{
			name:     "PostgreSQL schema.table",
			dialect:  "postgres",
			sql:      "SELECT * FROM {{public.users}}",
			expected: `SELECT * FROM "public"."users"`,
		},
		{
			name:     "MySQL schema.table",
			dialect:  "mysql",
			sql:      "SELECT * FROM {{mydb.users}}",
			expected: "SELECT * FROM `mydb`.`users`",
		},
		{
			name:     "Multiple tables",
			dialect:  "postgres",
			sql:      "SELECT * FROM {{users}} JOIN {{orders}} ON {{users}}.id = {{orders}}.user_id",
			expected: `SELECT * FROM "users" JOIN "orders" ON "users".id = "orders".user_id`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &DB{
				dialect: dialects.GetDialect(tt.dialect),
			}

			result, params := db.processSQL(tt.sql)

			if result != tt.expected {
				t.Errorf("Expected SQL:\n%s\nGot:\n%s", tt.expected, result)
			}

			if len(params) != 0 {
				t.Errorf("Expected 0 params, got %d: %v", len(params), params)
			}
		})
	}
}

// TestProcessSQL_QuotingColumnNames tests [[column]] quoting syntax.
func TestProcessSQL_QuotingColumnNames(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		sql      string
		expected string
	}{
		{
			name:     "PostgreSQL single column",
			dialect:  "postgres",
			sql:      "SELECT [[name]] FROM users",
			expected: `SELECT "name" FROM users`,
		},
		{
			name:     "MySQL single column",
			dialect:  "mysql",
			sql:      "SELECT [[name]] FROM users",
			expected: "SELECT `name` FROM users",
		},
		{
			name:     "SQLite single column",
			dialect:  "sqlite3",
			sql:      "SELECT [[name]] FROM users",
			expected: `SELECT "name" FROM users`,
		},
		{
			name:     "Multiple columns",
			dialect:  "postgres",
			sql:      "SELECT [[id]], [[name]], [[email]] FROM users",
			expected: `SELECT "id", "name", "email" FROM users`,
		},
		{
			name:     "Column in WHERE",
			dialect:  "postgres",
			sql:      "SELECT * FROM users WHERE [[status]] = 'active'",
			expected: `SELECT * FROM users WHERE "status" = 'active'`,
		},
		{
			name:     "Column in ORDER BY",
			dialect:  "mysql",
			sql:      "SELECT * FROM users ORDER BY [[name]] ASC",
			expected: "SELECT * FROM users ORDER BY `name` ASC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &DB{
				dialect: dialects.GetDialect(tt.dialect),
			}

			result, params := db.processSQL(tt.sql)

			if result != tt.expected {
				t.Errorf("Expected SQL:\n%s\nGot:\n%s", tt.expected, result)
			}

			if len(params) != 0 {
				t.Errorf("Expected 0 params, got %d: %v", len(params), params)
			}
		})
	}
}

// TestProcessSQL_CombinedQuotingAndParams tests combined usage of {{table}}, [[column]], and {:param}.
func TestProcessSQL_CombinedQuotingAndParams(t *testing.T) {
	tests := []struct {
		name           string
		dialect        string
		sql            string
		expectedSQL    string
		expectedParams []string
	}{
		{
			name:           "PostgreSQL combined",
			dialect:        "postgres",
			sql:            "SELECT [[name]], [[email]] FROM {{users}} WHERE [[status]]={:status}",
			expectedSQL:    `SELECT "name", "email" FROM "users" WHERE "status"=$1`,
			expectedParams: []string{"status"},
		},
		{
			name:           "MySQL combined",
			dialect:        "mysql",
			sql:            "SELECT [[name]] FROM {{users}} WHERE [[id]]={:id} AND [[status]]={:status}",
			expectedSQL:    "SELECT `name` FROM `users` WHERE `id`=? AND `status`=?",
			expectedParams: []string{"id", "status"},
		},
		{
			name:           "SQLite with schema",
			dialect:        "sqlite3",
			sql:            "SELECT [[name]] FROM {{public.users}} WHERE [[id]]={:id}",
			expectedSQL:    `SELECT "name" FROM "public"."users" WHERE "id"=?`,
			expectedParams: []string{"id"},
		},
		{
			name:    "Complex query",
			dialect: "postgres",
			sql: "SELECT [[u.name]], [[o.total]] FROM {{users}} u " +
				"JOIN {{orders}} o ON [[u.id]] = [[o.user_id]] " +
				"WHERE [[u.status]]={:status} AND [[o.created_at]] > {:date}",
			expectedSQL: `SELECT "u"."name", "o"."total" FROM "users" u ` +
				`JOIN "orders" o ON "u"."id" = "o"."user_id" ` +
				`WHERE "u"."status"=$1 AND "o"."created_at" > $2`,
			expectedParams: []string{"status", "date"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &DB{
				dialect: dialects.GetDialect(tt.dialect),
			}

			result, params := db.processSQL(tt.sql)

			if result != tt.expectedSQL {
				t.Errorf("Expected SQL:\n%s\nGot:\n%s", tt.expectedSQL, result)
			}

			if len(params) != len(tt.expectedParams) {
				t.Fatalf("Expected %d params, got %d: %v", len(tt.expectedParams), len(params), params)
			}

			for i, expected := range tt.expectedParams {
				if params[i] != expected {
					t.Errorf("Expected param[%d] = %q, got %q", i, expected, params[i])
				}
			}
		})
	}
}

// TestQuoteRegex tests the quote regex pattern matching.
func TestQuoteRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single table",
			input:    "SELECT * FROM {{users}}",
			expected: []string{"{{users}}"},
		},
		{
			name:     "single column",
			input:    "SELECT [[name]] FROM users",
			expected: []string{"[[name]]"},
		},
		{
			name:     "multiple tables and columns",
			input:    "SELECT [[id]], [[name]] FROM {{users}} JOIN {{orders}}",
			expected: []string{"[[id]]", "[[name]]", "{{users}}", "{{orders}}"},
		},
		{
			name:     "schema.table format",
			input:    "FROM {{public.users}}",
			expected: []string{"{{public.users}}"},
		},
		{
			name:     "table.column format",
			input:    "WHERE [[users.id]] = 1",
			expected: []string{"[[users.id]]"},
		},
		{
			name:     "no quoting syntax",
			input:    "SELECT * FROM users WHERE id = 1",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := quoteRegex.FindAllString(tt.input, -1)

			if len(matches) != len(tt.expected) {
				t.Fatalf("Expected %d matches, got %d: %v", len(tt.expected), len(matches), matches)
			}

			for i, expected := range tt.expected {
				if matches[i] != expected {
					t.Errorf("Expected match[%d] = %q, got %q", i, expected, matches[i])
				}
			}
		})
	}
}

// TestQuoteIdentifier tests the quoteIdentifier helper function.
func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		dialect    string
		identifier string
		expected   string
	}{
		{
			name:       "PostgreSQL simple",
			dialect:    "postgres",
			identifier: "users",
			expected:   `"users"`,
		},
		{
			name:       "PostgreSQL schema.table",
			dialect:    "postgres",
			identifier: "public.users",
			expected:   `"public"."users"`,
		},
		{
			name:       "MySQL simple",
			dialect:    "mysql",
			identifier: "users",
			expected:   "`users`",
		},
		{
			name:       "MySQL schema.table",
			dialect:    "mysql",
			identifier: "mydb.users",
			expected:   "`mydb`.`users`",
		},
		{
			name:       "SQLite simple",
			dialect:    "sqlite3",
			identifier: "users",
			expected:   `"users"`,
		},
		{
			name:       "SQLite schema.table",
			dialect:    "sqlite3",
			identifier: "main.users",
			expected:   `"main"."users"`,
		},
		{
			name:       "Identifier with spaces",
			dialect:    "postgres",
			identifier: " users ",
			expected:   `"users"`,
		},
		{
			name:       "Schema.table with spaces",
			dialect:    "postgres",
			identifier: " public . users ",
			expected:   `"public"."users"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &DB{
				dialect: dialects.GetDialect(tt.dialect),
			}

			result := db.quoteIdentifier(tt.identifier)

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
