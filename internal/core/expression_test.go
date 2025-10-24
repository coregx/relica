package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
)

// Helper to create dialects for testing
func getDialects() map[string]dialects.Dialect {
	return map[string]dialects.Dialect{
		"postgres": dialects.GetDialect("postgres"),
		"mysql":    dialects.GetDialect("mysql"),
		"sqlite":   dialects.GetDialect("sqlite"),
	}
}

// TestRawExp_Build tests raw SQL expressions with and without args
func TestRawExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		sql      string
		args     []interface{}
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "without args",
			dialect:  "postgres",
			sql:      "age > 18 AND status = 'active'",
			args:     nil,
			wantSQL:  "age > 18 AND status = 'active'",
			wantArgs: nil,
		},
		{
			name:     "with args",
			dialect:  "postgres",
			sql:      "age > ? AND status = ?",
			args:     []interface{}{18, "active"},
			wantSQL:  "age > ? AND status = ?",
			wantArgs: []interface{}{18, "active"},
		},
		{
			name:     "empty sql",
			dialect:  "postgres",
			sql:      "",
			args:     []interface{}{},
			wantSQL:  "",
			wantArgs: []interface{}{},
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := NewExp(tt.sql, tt.args...)
			sql, args := exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestHashExp_Build tests hash-based expressions
func TestHashExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		hash     HashExp
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "empty hash",
			dialect:  "postgres",
			hash:     HashExp{},
			wantSQL:  "",
			wantArgs: nil,
		},
		{
			name:     "single value postgres",
			dialect:  "postgres",
			hash:     HashExp{"status": 1},
			wantSQL:  `"status"=?`,
			wantArgs: []interface{}{1},
		},
		{
			name:     "single value mysql",
			dialect:  "mysql",
			hash:     HashExp{"status": 1},
			wantSQL:  "`status`=?",
			wantArgs: []interface{}{1},
		},
		{
			name:     "single value sqlite",
			dialect:  "sqlite",
			hash:     HashExp{"status": 1},
			wantSQL:  `"status"=?`,
			wantArgs: []interface{}{1},
		},
		{
			name:    "multiple values postgres",
			dialect: "postgres",
			hash: HashExp{
				"status": 1,
				"age":    18,
				"role":   "admin",
			},
			wantSQL:  `"age"=? AND "role"=? AND "status"=?`,
			wantArgs: []interface{}{18, "admin", 1}, // sorted by keys
		},
		{
			name:    "nil value postgres",
			dialect: "postgres",
			hash: HashExp{
				"deleted_at": nil,
				"status":     1,
			},
			wantSQL:  `"deleted_at" IS NULL AND "status"=?`,
			wantArgs: []interface{}{1},
		},
		{
			name:    "slice value IN clause postgres",
			dialect: "postgres",
			hash: HashExp{
				"age":    []interface{}{18, 19, 20},
				"status": 1,
			},
			wantSQL:  `"age" IN (?, ?, ?) AND "status"=?`,
			wantArgs: []interface{}{18, 19, 20, 1},
		},
		{
			name:    "slice value IN clause mysql",
			dialect: "mysql",
			hash: HashExp{
				"age":    []interface{}{18, 19, 20},
				"status": 1,
			},
			wantSQL:  "`age` IN (?, ?, ?) AND `status`=?",
			wantArgs: []interface{}{18, 19, 20, 1},
		},
		{
			name:    "empty slice postgres",
			dialect: "postgres",
			hash: HashExp{
				"age": []interface{}{},
			},
			wantSQL:  "0=1",
			wantArgs: nil,
		},
		{
			name:    "nested expression",
			dialect: "postgres",
			hash: HashExp{
				"age":    Eq("age", 18),
				"status": 1,
			},
			wantSQL:  `("age"=?) AND "status"=?`,
			wantArgs: []interface{}{18, 1},
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.hash.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestCompareExp_Build tests comparison expressions
func TestCompareExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		exp      Expression
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "Eq postgres",
			dialect:  "postgres",
			exp:      Eq("status", 1),
			wantSQL:  `"status"=?`,
			wantArgs: []interface{}{1},
		},
		{
			name:     "Eq mysql",
			dialect:  "mysql",
			exp:      Eq("status", 1),
			wantSQL:  "`status`=?",
			wantArgs: []interface{}{1},
		},
		{
			name:     "Eq sqlite",
			dialect:  "sqlite",
			exp:      Eq("status", 1),
			wantSQL:  `"status"=?`,
			wantArgs: []interface{}{1},
		},
		{
			name:     "Eq with nil postgres",
			dialect:  "postgres",
			exp:      Eq("deleted_at", nil),
			wantSQL:  `"deleted_at" IS NULL`,
			wantArgs: nil,
		},
		{
			name:     "NotEq postgres",
			dialect:  "postgres",
			exp:      NotEq("status", 0),
			wantSQL:  `"status"<>?`,
			wantArgs: []interface{}{0},
		},
		{
			name:     "NotEq with nil postgres",
			dialect:  "postgres",
			exp:      NotEq("deleted_at", nil),
			wantSQL:  `"deleted_at" IS NOT NULL`,
			wantArgs: nil,
		},
		{
			name:     "GreaterThan postgres",
			dialect:  "postgres",
			exp:      GreaterThan("age", 18),
			wantSQL:  `"age">?`,
			wantArgs: []interface{}{18},
		},
		{
			name:     "LessThan mysql",
			dialect:  "mysql",
			exp:      LessThan("age", 65),
			wantSQL:  "`age`<?",
			wantArgs: []interface{}{65},
		},
		{
			name:     "GreaterOrEqual sqlite",
			dialect:  "sqlite",
			exp:      GreaterOrEqual("score", 80),
			wantSQL:  `"score">=?`,
			wantArgs: []interface{}{80},
		},
		{
			name:     "LessOrEqual postgres",
			dialect:  "postgres",
			exp:      LessOrEqual("price", 100.50),
			wantSQL:  `"price"<=?`,
			wantArgs: []interface{}{100.50},
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestInExp_Build tests IN and NOT IN expressions
func TestInExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		exp      Expression
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "IN empty postgres",
			dialect:  "postgres",
			exp:      In("status"),
			wantSQL:  "0=1",
			wantArgs: nil,
		},
		{
			name:     "IN single value postgres",
			dialect:  "postgres",
			exp:      In("status", 1),
			wantSQL:  `"status"=?`,
			wantArgs: []interface{}{1},
		},
		{
			name:     "IN multiple values postgres",
			dialect:  "postgres",
			exp:      In("status", 1, 2, 3),
			wantSQL:  `"status" IN (?, ?, ?)`,
			wantArgs: []interface{}{1, 2, 3},
		},
		{
			name:     "IN multiple values mysql",
			dialect:  "mysql",
			exp:      In("status", 1, 2, 3),
			wantSQL:  "`status` IN (?, ?, ?)",
			wantArgs: []interface{}{1, 2, 3},
		},
		{
			name:     "IN with NULL postgres",
			dialect:  "postgres",
			exp:      In("status", 1, nil, 3),
			wantSQL:  `"status" IN (?, NULL, ?)`,
			wantArgs: []interface{}{1, 3},
		},
		{
			name:     "IN single NULL postgres",
			dialect:  "postgres",
			exp:      In("deleted_at", nil),
			wantSQL:  `"deleted_at" IS NULL`,
			wantArgs: nil,
		},
		{
			name:     "NOT IN empty postgres",
			dialect:  "postgres",
			exp:      NotIn("status"),
			wantSQL:  "",
			wantArgs: nil,
		},
		{
			name:     "NOT IN single value postgres",
			dialect:  "postgres",
			exp:      NotIn("status", 0),
			wantSQL:  `"status"<>?`,
			wantArgs: []interface{}{0},
		},
		{
			name:     "NOT IN multiple values mysql",
			dialect:  "mysql",
			exp:      NotIn("role", "admin", "moderator"),
			wantSQL:  "`role` NOT IN (?, ?)",
			wantArgs: []interface{}{"admin", "moderator"},
		},
		{
			name:     "NOT IN single NULL sqlite",
			dialect:  "sqlite",
			exp:      NotIn("deleted_at", nil),
			wantSQL:  `"deleted_at" IS NOT NULL`,
			wantArgs: nil,
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestBetweenExp_Build tests BETWEEN and NOT BETWEEN expressions
func TestBetweenExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		exp      Expression
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "BETWEEN postgres",
			dialect:  "postgres",
			exp:      Between("age", 18, 65),
			wantSQL:  `"age" BETWEEN ? AND ?`,
			wantArgs: []interface{}{18, 65},
		},
		{
			name:     "BETWEEN mysql",
			dialect:  "mysql",
			exp:      Between("price", 100, 500),
			wantSQL:  "`price` BETWEEN ? AND ?",
			wantArgs: []interface{}{100, 500},
		},
		{
			name:     "NOT BETWEEN sqlite",
			dialect:  "sqlite",
			exp:      NotBetween("score", 0, 50),
			wantSQL:  `"score" NOT BETWEEN ? AND ?`,
			wantArgs: []interface{}{0, 50},
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestLikeExp_Build tests LIKE expressions with escaping
func TestLikeExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		exp      *LikeExp
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "LIKE single value postgres",
			dialect:  "postgres",
			exp:      Like("name", "john"),
			wantSQL:  `"name" LIKE ?`,
			wantArgs: []interface{}{"%john%"},
		},
		{
			name:     "LIKE multiple values AND postgres",
			dialect:  "postgres",
			exp:      Like("description", "keyword", "phrase"),
			wantSQL:  `"description" LIKE ? AND "description" LIKE ?`,
			wantArgs: []interface{}{"%keyword%", "%phrase%"},
		},
		{
			name:     "LIKE multiple values OR mysql",
			dialect:  "mysql",
			exp:      OrLike("title", "foo", "bar"),
			wantSQL:  "`title` LIKE ? OR `title` LIKE ?",
			wantArgs: []interface{}{"%foo%", "%bar%"},
		},
		{
			name:     "NOT LIKE sqlite",
			dialect:  "sqlite",
			exp:      NotLike("email", "spam"),
			wantSQL:  `"email" NOT LIKE ?`,
			wantArgs: []interface{}{"%spam%"},
		},
		{
			name:     "OrNotLike postgres",
			dialect:  "postgres",
			exp:      OrNotLike("text", "bad", "ugly"),
			wantSQL:  `"text" NOT LIKE ? OR "text" NOT LIKE ?`,
			wantArgs: []interface{}{"%bad%", "%ugly%"},
		},
		{
			name:     "LIKE with custom Match postgres",
			dialect:  "postgres",
			exp:      Like("filename", ".txt").Match(false, true),
			wantSQL:  `"filename" LIKE ?`,
			wantArgs: []interface{}{".txt%"},
		},
		{
			name:     "LIKE with escaping postgres",
			dialect:  "postgres",
			exp:      Like("path", "50%_discount"),
			wantSQL:  `"path" LIKE ?`,
			wantArgs: []interface{}{"%50\\%\\_discount%"},
		},
		{
			name:     "LIKE empty values",
			dialect:  "postgres",
			exp:      Like("name"),
			wantSQL:  "",
			wantArgs: nil,
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestAndOrExp_Build tests AND/OR combination expressions
func TestAndOrExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		exp      Expression
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "AND empty",
			dialect:  "postgres",
			exp:      And(),
			wantSQL:  "",
			wantArgs: nil,
		},
		{
			name:     "AND single expression",
			dialect:  "postgres",
			exp:      And(Eq("status", 1)),
			wantSQL:  `"status"=?`,
			wantArgs: []interface{}{1},
		},
		{
			name:    "AND multiple expressions postgres",
			dialect: "postgres",
			exp: And(
				Eq("status", 1),
				GreaterThan("age", 18),
				Like("name", "john"),
			),
			wantSQL:  `("status"=?) AND ("age">?) AND ("name" LIKE ?)`,
			wantArgs: []interface{}{1, 18, "%john%"},
		},
		{
			name:    "OR multiple expressions mysql",
			dialect: "mysql",
			exp: Or(
				Eq("role", "admin"),
				Eq("role", "moderator"),
			),
			wantSQL:  "(`role`=?) OR (`role`=?)",
			wantArgs: []interface{}{"admin", "moderator"},
		},
		{
			name:    "AND with nil filtering",
			dialect: "postgres",
			exp: And(
				Eq("status", 1),
				nil,
				GreaterThan("age", 18),
			),
			wantSQL:  `("status"=?) AND ("age">?)`,
			wantArgs: []interface{}{1, 18},
		},
		{
			name:    "nested AND/OR postgres",
			dialect: "postgres",
			exp: And(
				Eq("active", true),
				Or(
					Eq("role", "admin"),
					Eq("role", "moderator"),
				),
			),
			wantSQL:  `("active"=?) AND (("role"=?) OR ("role"=?))`,
			wantArgs: []interface{}{true, "admin", "moderator"},
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestNotExp_Build tests NOT expressions
func TestNotExp_Build(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		exp      Expression
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:     "NOT nil expression",
			dialect:  "postgres",
			exp:      Not(nil),
			wantSQL:  "",
			wantArgs: nil,
		},
		{
			name:     "NOT simple expression postgres",
			dialect:  "postgres",
			exp:      Not(Eq("active", true)),
			wantSQL:  `NOT ("active"=?)`,
			wantArgs: []interface{}{true},
		},
		{
			name:     "NOT IN expression mysql",
			dialect:  "mysql",
			exp:      Not(In("status", 0, 1, 2)),
			wantSQL:  "NOT (`status` IN (?, ?, ?))",
			wantArgs: []interface{}{0, 1, 2},
		},
		{
			name:    "NOT complex AND expression sqlite",
			dialect: "sqlite",
			exp: Not(And(
				Eq("deleted", false),
				GreaterThan("age", 18),
			)),
			wantSQL:  `NOT (("deleted"=?) AND ("age">?))`,
			wantArgs: []interface{}{false, 18},
		},
	}

	dialects := getDialects()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.exp.Build(dialects[tt.dialect])
			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestLikeExp_EscapeChars tests custom escape character configuration
func TestLikeExp_EscapeChars(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Custom escape chars (only escape %)
	exp := Like("text", "50%").EscapeChars("%", "\\%")
	sql, args := exp.Build(dialect)

	assert.Equal(t, `"text" LIKE ?`, sql)
	assert.Equal(t, []interface{}{"%50\\%%"}, args)
}

// TestLikeExp_EscapeChars_Panic tests panic on invalid escape chars
func TestLikeExp_EscapeChars_Panic(t *testing.T) {
	assert.Panics(t, func() {
		Like("name", "test").EscapeChars("%", "\\%", "_") // odd number
	})
}

// TestCompareExp_WithExpressionValue tests comparison with Expression values
func TestCompareExp_WithExpressionValue(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// Subquery-like expression (using RawExp)
	exp := Eq("total", NewExp("(SELECT SUM(amount) FROM orders WHERE user_id = ?)", 123))
	sql, args := exp.Build(dialect)

	assert.Equal(t, `"total"=((SELECT SUM(amount) FROM orders WHERE user_id = ?))`, sql)
	assert.Equal(t, []interface{}{123}, args)
}

// TestHashExp_AllDialects tests HashExp across all three dialects
func TestHashExp_AllDialects(t *testing.T) {
	hash := HashExp{
		"status": 1,
		"age":    []interface{}{18, 19, 20},
	}

	testCases := []struct {
		dialectName string
		wantSQL     string
	}{
		{
			dialectName: "postgres",
			wantSQL:     `"age" IN (?, ?, ?) AND "status"=?`,
		},
		{
			dialectName: "mysql",
			wantSQL:     "`age` IN (?, ?, ?) AND `status`=?",
		},
		{
			dialectName: "sqlite",
			wantSQL:     `"age" IN (?, ?, ?) AND "status"=?`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.dialectName, func(t *testing.T) {
			dialect := dialects.GetDialect(tc.dialectName)
			sql, args := hash.Build(dialect)
			assert.Equal(t, tc.wantSQL, sql)
			assert.Equal(t, []interface{}{18, 19, 20, 1}, args)
		})
	}
}
