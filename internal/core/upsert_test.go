package core

import (
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, q)

	// Verify SQL structure
	sql := q.sql
	assert.Contains(t, sql, `INSERT INTO "users"`)
	assert.Contains(t, sql, "ON CONFLICT (id)")
	assert.Contains(t, sql, "DO UPDATE SET")
	assert.Contains(t, sql, "name = EXCLUDED.name")
	assert.Contains(t, sql, "email = EXCLUDED.email")

	// Verify parameters
	assert.Len(t, q.params, 3)
}

func TestUpsertQuery_PostgreSQL_DoNothing(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":   1,
		"name": "Alice",
	}).OnConflict("id").DoNothing()

	q := query.Build()
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, `INSERT INTO "users"`)
	assert.Contains(t, sql, "ON CONFLICT (id) DO NOTHING")
	assert.NotContains(t, sql, "UPDATE")
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
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, "ON CONFLICT (id) DO UPDATE SET")
	// Should update email and name, but not id
	assert.Contains(t, sql, "email = EXCLUDED.email")
	assert.Contains(t, sql, "name = EXCLUDED.name")
	assert.NotContains(t, sql, "id = EXCLUDED.id")
}

func TestUpsertQuery_MySQL_DoUpdate(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":   1,
		"name": "Alice",
	}).OnConflict("id").DoUpdate("name")

	q := query.Build()
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, "INSERT INTO `users`")
	assert.Contains(t, sql, "ON DUPLICATE KEY UPDATE")
	assert.Contains(t, sql, "name = VALUES(name)")

	// Verify placeholders
	assert.Equal(t, 2, strings.Count(sql, "?"))
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
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, "ON DUPLICATE KEY UPDATE")
	assert.Contains(t, sql, "email = VALUES(email)")
	assert.Contains(t, sql, "name = VALUES(name)")
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
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, `INSERT INTO "users"`)
	assert.Contains(t, sql, "ON CONFLICT (id)")
	assert.Contains(t, sql, "DO UPDATE SET")
	assert.Contains(t, sql, "name = excluded.name")
	assert.Contains(t, sql, "email = excluded.email")
}

func TestUpsertQuery_SQLite_DoNothing(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Upsert("users", map[string]interface{}{
		"id":   1,
		"name": "Alice",
	}).OnConflict("id").DoNothing()

	q := query.Build()
	require.NotNil(t, q)

	sql := q.sql
	assert.Contains(t, sql, `INSERT INTO "users"`)
	assert.Contains(t, sql, "ON CONFLICT (id) DO NOTHING")
	assert.NotContains(t, sql, "UPDATE")
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
			expectSQL:   []string{"ON CONFLICT (email, username)", "DO UPDATE SET", "name = EXCLUDED.name"},
		},
		{
			name:        "SQLite",
			dialectName: "sqlite",
			expectSQL:   []string{"ON CONFLICT (email, username)", "DO UPDATE SET", "name = excluded.name"},
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
			require.NotNil(t, q)

			for _, expected := range tt.expectSQL {
				assert.Contains(t, q.sql, expected)
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
	require.NotNil(t, q)

	// Parameters should be ordered: aaa, mmm, zzz
	assert.Equal(t, []interface{}{"first", "middle", "last"}, q.params)

	// SQL should have columns in alphabetical order
	sql := q.sql
	aIdx := strings.Index(sql, "aaa")
	mIdx := strings.Index(sql, "mmm")
	zIdx := strings.Index(sql, "zzz")

	assert.Less(t, aIdx, mIdx, "aaa should come before mmm")
	assert.Less(t, mIdx, zIdx, "mmm should come before zzz")
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
			assert.Equal(t, tt.expected, result)
		})
	}
}
