package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// mockDB creates a minimal DB for SQL generation testing
func mockDB(dialectName string) *DB {
	return &DB{
		dialect: dialects.GetDialect(dialectName),
	}
}

func TestUpsertQuery_PostgreSQL_DoUpdate(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":    1,
		"name":  "Alice",
		"email": "alice@example.com",
	}).OnConflict("id").DoUpdate("name", "email")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	sql := q.sql
	if !strings.Contains(sql, `INSERT INTO "users"`) {
		t.Errorf("%q does not contain %q", sql, `INSERT INTO "users"`)
	}
	if !strings.Contains(sql, `ON CONFLICT ("id")`) {
		t.Errorf("%q does not contain %q", sql, `ON CONFLICT ("id")`)
	}
	if !strings.Contains(sql, "DO UPDATE SET") {
		t.Errorf("%q does not contain %q", sql, "DO UPDATE SET")
	}
	if !strings.Contains(sql, `"name" = EXCLUDED."name"`) {
		t.Errorf("%q does not contain %q", sql, `"name" = EXCLUDED."name"`)
	}
	if !strings.Contains(sql, `"email" = EXCLUDED."email"`) {
		t.Errorf("%q does not contain %q", sql, `"email" = EXCLUDED."email"`)
	}

	// Verify parameters
	if len(q.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(q.params))
	}
}

func TestUpsertQuery_PostgreSQL_DoNothing(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":   1,
		"name": "Alice",
	}).OnConflict("id").DoNothing()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	if !strings.Contains(sql, `INSERT INTO "users"`) {
		t.Errorf("%q does not contain %q", sql, `INSERT INTO "users"`)
	}
	if !strings.Contains(sql, `ON CONFLICT ("id") DO NOTHING`) {
		t.Errorf("%q does not contain %q", sql, `ON CONFLICT ("id") DO NOTHING`)
	}
	if strings.Contains(sql, "UPDATE") {
		t.Errorf("%q should not contain %q", sql, "UPDATE")
	}
}

func TestUpsertQuery_PostgreSQL_AutoUpdateColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// When DoUpdate() not called but OnConflict is specified,
	// it should update all columns except conflict columns
	query := qb.Upsert("users", map[string]interface{}{
		"id":    1,
		"name":  "Alice",
		"email": "alice@example.com",
	}).OnConflict("id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	if !strings.Contains(sql, `ON CONFLICT ("id") DO UPDATE SET`) {
		t.Errorf("%q does not contain %q", sql, `ON CONFLICT ("id") DO UPDATE SET`)
	}
	// Should update email and name, but not id
	if !strings.Contains(sql, `"email" = EXCLUDED."email"`) {
		t.Errorf("%q does not contain %q", sql, `"email" = EXCLUDED."email"`)
	}
	if !strings.Contains(sql, `"name" = EXCLUDED."name"`) {
		t.Errorf("%q does not contain %q", sql, `"name" = EXCLUDED."name"`)
	}
	if strings.Contains(sql, `"id" = EXCLUDED."id"`) {
		t.Errorf("%q should not contain %q", sql, `"id" = EXCLUDED."id"`)
	}
}

func TestUpsertQuery_MySQL_DoUpdate(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":   1,
		"name": "Alice",
	}).OnConflict("id").DoUpdate("name")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	if !strings.Contains(sql, "INSERT INTO `users`") {
		t.Errorf("%q does not contain %q", sql, "INSERT INTO `users`")
	}
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("%q does not contain %q", sql, "ON DUPLICATE KEY UPDATE")
	}
	if !strings.Contains(sql, "`name` = VALUES(`name`)") {
		t.Errorf("%q does not contain %q", sql, "`name` = VALUES(`name`)")
	}

	// Verify placeholders
	if got := strings.Count(sql, "?"); got != 2 {
		t.Errorf("got %v, want %v", got, 2)
	}
}

func TestUpsertQuery_MySQL_AutoUpdateColumns(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":    1,
		"name":  "Alice",
		"email": "alice@example.com",
	}).OnConflict("id")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("%q does not contain %q", sql, "ON DUPLICATE KEY UPDATE")
	}
	if !strings.Contains(sql, "`email` = VALUES(`email`)") {
		t.Errorf("%q does not contain %q", sql, "`email` = VALUES(`email`)")
	}
	if !strings.Contains(sql, "`name` = VALUES(`name`)") {
		t.Errorf("%q does not contain %q", sql, "`name` = VALUES(`name`)")
	}
}

func TestUpsertQuery_SQLite_DoUpdate(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":    1,
		"name":  "Alice",
		"email": "alice@example.com",
	}).OnConflict("id").DoUpdate("name", "email")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	if !strings.Contains(sql, `INSERT INTO "users"`) {
		t.Errorf("%q does not contain %q", sql, `INSERT INTO "users"`)
	}
	if !strings.Contains(sql, `ON CONFLICT ("id")`) {
		t.Errorf("%q does not contain %q", sql, `ON CONFLICT ("id")`)
	}
	if !strings.Contains(sql, "DO UPDATE SET") {
		t.Errorf("%q does not contain %q", sql, "DO UPDATE SET")
	}
	if !strings.Contains(sql, `"name" = excluded."name"`) {
		t.Errorf("%q does not contain %q", sql, `"name" = excluded."name"`)
	}
	if !strings.Contains(sql, `"email" = excluded."email"`) {
		t.Errorf("%q does not contain %q", sql, `"email" = excluded."email"`)
	}
}

func TestUpsertQuery_SQLite_DoNothing(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":   1,
		"name": "Alice",
	}).OnConflict("id").DoNothing()

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	sql := q.sql
	if !strings.Contains(sql, `INSERT INTO "users"`) {
		t.Errorf("%q does not contain %q", sql, `INSERT INTO "users"`)
	}
	if !strings.Contains(sql, `ON CONFLICT ("id") DO NOTHING`) {
		t.Errorf("%q does not contain %q", sql, `ON CONFLICT ("id") DO NOTHING`)
	}
	if strings.Contains(sql, "UPDATE") {
		t.Errorf("%q should not contain %q", sql, "UPDATE")
	}
}

func TestUpsertQuery_MultipleConflictColumns(t *testing.T) {
	tests := []struct {
		name        string
		dialectName string
		expectSQL   []string
	}{
		{
			name:        "PostgreSQL",
			dialectName: "postgres",
			expectSQL:   []string{`ON CONFLICT ("email", "username")`, "DO UPDATE SET", `"name" = EXCLUDED."name"`},
		},
		{
			name:        "SQLite",
			dialectName: "sqlite",
			expectSQL:   []string{`ON CONFLICT ("email", "username")`, "DO UPDATE SET", `"name" = excluded."name"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialectName)
			qb := &QueryBuilder{db: db}

			query := qb.Upsert("users", map[string]interface{}{
				"name":     "Alice",
				"email":    "alice@example.com",
				"username": "alice",
			}).OnConflict("email", "username").DoUpdate("name")

			q := query.Build()
			if q == nil {
				t.Fatal("expected non-nil")
			}

			for _, expected := range tt.expectSQL {
				if !strings.Contains(q.sql, expected) {
					t.Errorf("%q does not contain %q", q.sql, expected)
				}
			}
		})
	}
}

func TestUpsertQuery_ParameterOrdering(t *testing.T) {
	// Parameters should be in sorted key order for deterministic SQL
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"zzz": "last",
		"aaa": "first",
		"mmm": "middle",
	}).OnConflict("aaa").DoUpdate("mmm", "zzz")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Parameters should be ordered: aaa, mmm, zzz
	want := []interface{}{"first", "middle", "last"}
	if got := q.params; len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got %v, want %v", got, want)
				break
			}
		}
	}

	// SQL should have columns in alphabetical order
	sql := q.sql
	aIdx := strings.Index(sql, "aaa")
	mIdx := strings.Index(sql, "mmm")
	zIdx := strings.Index(sql, "zzz")

	if aIdx >= mIdx {
		t.Errorf("expected %v > %v", mIdx, aIdx)
	}
	if mIdx >= zIdx {
		t.Errorf("expected %v > %v", zIdx, mIdx)
	}
}

func TestFilterKeys(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		exclude  []string
		expected []string
	}{
		{
			name:     "filter single key",
			keys:     []string{"a", "b", "c"},
			exclude:  []string{"b"},
			expected: []string{"a", "c"},
		},
		{
			name:     "filter multiple keys",
			keys:     []string{"id", "name", "email", "created_at"},
			exclude:  []string{"id", "created_at"},
			expected: []string{"name", "email"},
		},
		{
			name:     "no exclusions",
			keys:     []string{"a", "b", "c"},
			exclude:  []string{},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "exclude all",
			keys:     []string{"a", "b"},
			exclude:  []string{"a", "b"},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterKeys(tt.keys, tt.exclude)
			if len(result) != len(tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
				return
			}
			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Errorf("got %v, want %v", result, tt.expected)
					break
				}
			}
		})
	}
}
