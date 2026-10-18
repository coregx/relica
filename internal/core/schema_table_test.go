package core

import (
	"strings"
	"testing"
)

// Schema-qualified table names are quoted per part in every statement, as in
// SELECT: "app"."users", not "app.users" (one identifier, "relation does not
// exist"). Found when the PostgreSQL integration suite first ran in CI.
func TestDML_SchemaQualifiedTable(t *testing.T) {
	dialects := []struct {
		name, want string
	}{
		{"postgres", `"app"."users"`},
		{"mysql", "`app`.`users`"},
		{"sqlite", `"app"."users"`},
	}
	for _, d := range dialects {
		qb := &QueryBuilder{db: mockDB(d.name)}
		stmts := map[string]string{
			"Insert": qb.Insert("app.users", map[string]any{"name": "a"}).sql,
			"Upsert": qb.Upsert("app.users", map[string]any{"id": 1, "name": "a"}).
				OnConflict("id").DoUpdate("name").Build().sql,
			"Update":      qb.Update("app.users").Set(map[string]any{"name": "a"}).Where("id = ?", 1).Build().sql,
			"Delete":      qb.Delete("app.users").Where("id = ?", 1).Build().sql,
			"BatchInsert": qb.BatchInsert("app.users", []string{"name"}).Values("a").Build().sql,
			"BatchUpdate": qb.BatchUpdate("app.users", "id").Set(1, map[string]any{"name": "a"}).Build().sql,
		}
		for stmt, sql := range stmts {
			if !strings.Contains(sql, d.want) {
				t.Errorf("%s %s: %q does not contain %s", d.name, stmt, sql, d.want)
			}
		}
	}
}

// Quoting stays injection-safe: every part is quoted and embedded quote
// characters are doubled; nothing is passed through raw.
func TestQuoteTableName(t *testing.T) {
	pg := mockDB("postgres").dialect
	my := mockDB("mysql").dialect
	tests := []struct {
		in, wantPG, wantMy string
	}{
		{"users", `"users"`, "`users`"},
		{"app.users", `"app"."users"`, "`app`.`users`"},
		{`x"y`, `"x""y"`, "`x\"y`"},
		{"a`b.c", "\"a`b\".\"c\"", "`a``b`.`c`"},
		{"t(1)", `"t(1)"`, "`t(1)`"},
	}
	for _, tc := range tests {
		if got := quoteTableName(tc.in, pg); got != tc.wantPG {
			t.Errorf("postgres %q: got %s, want %s", tc.in, got, tc.wantPG)
		}
		if got := quoteTableName(tc.in, my); got != tc.wantMy {
			t.Errorf("mysql %q: got %s, want %s", tc.in, got, tc.wantMy)
		}
	}
}
