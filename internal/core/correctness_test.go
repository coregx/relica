package core

import (
	"context"
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// ─── Fix 1: HAVING placeholder renumbering ────────────────────────────────────

// TestHavingPlaceholderRenumber_MultiArgClauses verifies that HAVING clauses with
// multiple args each are renumbered correctly on PostgreSQL (positional placeholders).
func TestHavingPlaceholderRenumber_MultiArgClauses(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// WHERE has 1 arg ($1), HAVING has 2 clauses: first with 2 args, second with 1 arg.
	// Expected placeholder numbering: WHERE=$1, HAVING-1a=$2, HAVING-1b=$3, HAVING-2=$4.
	sq := qb.Select("status", "COUNT(*) as cnt", "SUM(amount) as total").
		From("orders").
		Where("region = ?", "west").
		GroupBy("status").
		Having("COUNT(*) > ? AND COUNT(*) < ?", 5, 100).
		Having("SUM(amount) > ?", 1000)

	sql, params := sq.buildSQL(dialects.GetDialect("postgres"))

	assert.Contains(t, sql, "$1") // WHERE arg
	assert.Contains(t, sql, "$2") // first HAVING, first arg
	assert.Contains(t, sql, "$3") // first HAVING, second arg
	assert.Contains(t, sql, "$4") // second HAVING arg

	// Verify no duplicate numbering: $2 should appear only in the HAVING clause.
	havingStart := strings.Index(sql, "HAVING")
	require.Greater(t, havingStart, 0)
	havingClause := sql[havingStart:]
	assert.Contains(t, havingClause, "$2")
	assert.Contains(t, havingClause, "$3")
	assert.Contains(t, havingClause, "$4")

	// Parameters must be in correct order: WHERE, HAVING-1a, HAVING-1b, HAVING-2.
	require.Len(t, params, 4)
	assert.Equal(t, "west", params[0])
	assert.Equal(t, 5, params[1])
	assert.Equal(t, 100, params[2])
	assert.Equal(t, 1000, params[3])
}

// TestHavingPlaceholderRenumber_SingleArgEach verifies HAVING with single-arg clauses
// still works correctly (regression guard for the existing code path).
func TestHavingPlaceholderRenumber_SingleArgEach(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("status", "COUNT(*) as cnt").
		From("orders").
		GroupBy("status").
		Having("COUNT(*) > ?", 5).
		Having("COUNT(*) < ?", 100)

	sql, params := sq.buildSQL(dialects.GetDialect("postgres"))

	// No WHERE, so HAVING args start at $1.
	havingStart := strings.Index(sql, "HAVING")
	require.Greater(t, havingStart, 0)
	havingClause := sql[havingStart:]
	assert.Contains(t, havingClause, "$1")
	assert.Contains(t, havingClause, "$2")

	require.Len(t, params, 2)
	assert.Equal(t, 5, params[0])
	assert.Equal(t, 100, params[1])
}

// ─── Fix 2: QuoteTableName / QuoteColumnName / GenerateParamName ──────────────

// TestQuoteTableName_UsesDialect verifies that QuoteTableName delegates to the dialect
// instead of always using double-quotes.
func TestQuoteTableName_UsesDialect(t *testing.T) {
	tests := []struct {
		dialect  string
		table    string
		wantOpen byte
	}{
		{"postgres", "users", '"'},
		{"mysql", "users", '`'},
		{"sqlite", "users", '"'},
	}
	for _, tt := range tests {
		t.Run(tt.dialect, func(t *testing.T) {
			db := mockDB(tt.dialect)
			result := db.QuoteTableName(tt.table)
			assert.Equal(t, tt.wantOpen, result[0],
				"QuoteTableName(%q) on %s: got %q", tt.table, tt.dialect, result)
		})
	}
}

// TestQuoteColumnName_UsesDialectAndSplitsDots verifies that QuoteColumnName uses the
// dialect quoting style and correctly handles dotted (table.column) identifiers.
func TestQuoteColumnName_UsesDialectAndSplitsDots(t *testing.T) {
	tests := []struct {
		dialect string
		col     string
		want    string
	}{
		{"postgres", "user_id", `"user_id"`},
		{"mysql", "user_id", "`user_id`"},
		{"postgres", "t.col", `"t"."col"`},
		{"mysql", "t.col", "`t`.`col`"},
	}
	for _, tt := range tests {
		t.Run(tt.dialect+"/"+tt.col, func(t *testing.T) {
			db := mockDB(tt.dialect)
			result := db.QuoteColumnName(tt.col)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestGenerateParamName_UsesDialect verifies that GenerateParamName returns the
// dialect-specific placeholder rather than the old "p1, p2" style.
func TestGenerateParamName_UsesDialect(t *testing.T) {
	tests := []struct {
		dialect string
		index   int
		want    string
	}{
		{"postgres", 1, "$1"},
		{"postgres", 3, "$3"},
		{"mysql", 1, "?"},
		{"mysql", 5, "?"},
		{"sqlite", 1, "?"},
	}
	for _, tt := range tests {
		t.Run(tt.dialect+"/"+tt.want, func(t *testing.T) {
			db := mockDB(tt.dialect)
			result := db.GenerateParamName(tt.index)
			assert.Equal(t, tt.want, result)
		})
	}
}

// ─── Fix 3: Validator applied to builder queries ──────────────────────────────

// TestValidator_AppliedToBuilderQueries verifies that a configured validator is called
// for queries built via the fluent builder (Select/Insert/Update/Delete), not only
// for raw DB.ExecContext/QueryContext calls.
func TestValidator_AppliedToBuilderQueries(t *testing.T) {
	db, err := NewDB("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.sqlDB.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	_, err = db.sqlDB.Exec("INSERT INTO items (id, name) VALUES (1, 'normal')")
	require.NoError(t, err)

	// Enable validator.
	db.validator = security.NewValidator()

	ctx := context.Background()
	qb := &QueryBuilder{db: db, ctx: ctx}

	t.Run("legitimate select passes validator", func(t *testing.T) {
		var items []struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}
		err := qb.Select().From("items").Where("id = ?", 1).All(&items)
		assert.NoError(t, err)
	})

	t.Run("injected param blocked by validator", func(t *testing.T) {
		var items []struct {
			Name string `db:"name"`
		}
		// The validator catches injection attempts in parameters.
		err := qb.Select().From("items").Where("name = ?", "'; DROP TABLE items--").All(&items)
		assert.Error(t, err, "validator should have blocked the injected param")
	})
}

// ─── Fix 4: Empty Insert/Update returns clean error ───────────────────────────

// TestInsert_EmptyValues returns a propagated error rather than producing invalid SQL.
func TestInsert_EmptyValues(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	t.Run("nil map", func(t *testing.T) {
		q := qb.Insert("users", nil)
		require.NotNil(t, q)
		assert.NotNil(t, q.prepErr)
		assert.Contains(t, q.prepErr.Error(), "Insert requires a non-empty values map")
	})

	t.Run("empty map", func(t *testing.T) {
		q := qb.Insert("users", map[string]interface{}{})
		require.NotNil(t, q)
		assert.NotNil(t, q.prepErr)
		assert.Contains(t, q.prepErr.Error(), "Insert requires a non-empty values map")
	})
}

// TestUpdate_NoSet returns a propagated error when Build() is called without Set().
func TestUpdate_NoSet(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	t.Run("Update without Set", func(t *testing.T) {
		q := qb.Update("users").Where("id = ?", 1).Build()
		require.NotNil(t, q)
		assert.NotNil(t, q.prepErr)
		assert.Contains(t, q.prepErr.Error(), "Update requires values")
	})

	t.Run("Update with nil Set values", func(t *testing.T) {
		q := qb.Update("users").Set(nil).Where("id = ?", 1).Build()
		require.NotNil(t, q)
		assert.NotNil(t, q.prepErr)
		assert.Contains(t, q.prepErr.Error(), "Update requires values")
	})
}

// ─── Fix 5: Missing named param returns error ─────────────────────────────────

// TestResolveNamedParams_MissingParam verifies that a missing named parameter
// causes resolveNamedParams to return a non-nil error.
func TestResolveNamedParams_MissingParam(t *testing.T) {
	_, _, err := resolveNamedParams(
		"id = {:id} AND status = {:status}",
		[]interface{}{Params{"id": 1}}, // :status is missing
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ":status")
}

// TestSelectQuery_Where_MissingNamedParam verifies that a missing named param causes
// the build error to propagate through the fluent chain.
func TestSelectQuery_Where_MissingNamedParam(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select().From("users").
		Where("id = {:id} AND status = {:status}", Params{"id": 1}) // :status missing

	q := sq.Build()
	require.NotNil(t, q)
	assert.NotNil(t, q.prepErr, "build error should be set for missing named param")
	assert.Contains(t, q.prepErr.Error(), ":status")
}

// TestUpdateQuery_Where_MissingNamedParam verifies error propagation in UpdateQuery.
func TestUpdateQuery_Where_MissingNamedParam(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Update("users").
		Set(map[string]interface{}{"name": "Alice"}).
		Where("id = {:id} AND status = {:status}", Params{"id": 1}).
		Build()

	require.NotNil(t, q)
	assert.NotNil(t, q.prepErr, "build error should be set for missing named param")
	assert.Contains(t, q.prepErr.Error(), ":status")
}

// TestDeleteQuery_Where_MissingNamedParam verifies error propagation in DeleteQuery.
func TestDeleteQuery_Where_MissingNamedParam(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Delete("users").
		Where("id = {:id} AND status = {:status}", Params{"id": 1}).
		Build()

	require.NotNil(t, q)
	assert.NotNil(t, q.prepErr, "build error should be set for missing named param")
	assert.Contains(t, q.prepErr.Error(), ":status")
}

// ─── Fix 6: Schema-qualified table with alias ─────────────────────────────────

// TestBuildTableWithAlias_SchemaQualified verifies that schema-qualified table names
// ("schema.table") are quoted per-part rather than as a single identifier, and that
// the alias is correctly appended.
func TestBuildTableWithAlias_SchemaQualified(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		input   string
		want    string
	}{
		{
			name:    "postgres schema.table with alias",
			dialect: "postgres",
			input:   "public.users u",
			want:    `"public"."users" AS "u"`,
		},
		{
			name:    "mysql schema.table with alias",
			dialect: "mysql",
			input:   "mydb.users u",
			want:    "`mydb`.`users` AS `u`",
		},
		{
			name:    "postgres schema.table no alias",
			dialect: "postgres",
			input:   "public.users",
			want:    `"public"."users"`,
		},
		{
			name:    "postgres simple table with alias",
			dialect: "postgres",
			input:   "users u",
			want:    `"users" AS "u"`,
		},
		{
			name:    "postgres simple table no alias",
			dialect: "postgres",
			input:   "users",
			want:    `"users"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			sq := &SelectQuery{builder: &QueryBuilder{db: db}}
			result := sq.buildTableWithAlias(tt.input, db.dialect)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestFrom_SchemaQualifiedTable verifies the end-to-end SQL generation when
// From() receives a schema-qualified table name.
func TestFrom_SchemaQualifiedTable(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select("id", "name").From("public.users").buildSQL(db.dialect)

	// Should produce "public"."users" not "public.users"
	assert.Contains(t, sql, `"public"."users"`)
	assert.NotContains(t, sql, `"public.users"`)
}

// TestFrom_SchemaQualifiedTableWithAlias verifies end-to-end SQL generation
// for schema-qualified table with alias.
func TestFrom_SchemaQualifiedTableWithAlias(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select("u.id", "u.name").From("public.users u").buildSQL(db.dialect)

	assert.Contains(t, sql, `"public"."users" AS "u"`)
	assert.NotContains(t, sql, `"public.users u"`)
}
