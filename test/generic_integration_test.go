//go:build integration
// +build integration

package test

import (
	"context"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Generic One[T] / All[T] / Scalar[T] on real databases
// =============================================================================

func runGenericTests(t *testing.T, ds *DatabaseSetup) {
	db := ds.DB
	ctx := context.Background()

	// Create test table
	var ddl string
	switch ds.Dialect {
	case "postgres":
		ddl = `CREATE TABLE IF NOT EXISTS generic_test (
			id SERIAL PRIMARY KEY, name TEXT NOT NULL, score INTEGER NOT NULL DEFAULT 0)`
	case "mysql":
		ddl = "CREATE TABLE IF NOT EXISTS generic_test (" +
			"id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255) NOT NULL, score INT NOT NULL DEFAULT 0" +
			") ENGINE=InnoDB"
	case "sqlite":
		ddl = `CREATE TABLE IF NOT EXISTS generic_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, score INTEGER NOT NULL DEFAULT 0)`
	}
	_, err := db.ExecContext(ctx, ddl)
	require.NoError(t, err)
	defer func() { _, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS generic_test") }()

	// Seed data
	_, _ = db.ExecContext(ctx, "DELETE FROM generic_test")
	_, err = db.Insert("generic_test", map[string]interface{}{"name": "Alice", "score": 95}).Execute()
	require.NoError(t, err)
	_, err = db.Insert("generic_test", map[string]interface{}{"name": "Bob", "score": 87}).Execute()
	require.NoError(t, err)
	_, err = db.Insert("generic_test", map[string]interface{}{"name": "Charlie", "score": 92}).Execute()
	require.NoError(t, err)

	type GenericRow struct {
		ID    int    `db:"id"`
		Name  string `db:"name"`
		Score int    `db:"score"`
	}

	t.Run("One_ReturnsTypedResult", func(t *testing.T) {
		user, err := relica.One[GenericRow](
			db.Select("id", "name", "score").From("generic_test").Where(relica.Eq("name", "Alice")),
		)
		require.NoError(t, err)
		assert.Equal(t, "Alice", user.Name)
		assert.Equal(t, 95, user.Score)
		assert.Greater(t, user.ID, 0)
	})

	t.Run("All_ReturnsTypedSlice", func(t *testing.T) {
		users, err := relica.All[GenericRow](
			db.Select("id", "name", "score").From("generic_test").OrderBy("name"),
		)
		require.NoError(t, err)
		require.Len(t, users, 3)
		assert.Equal(t, "Alice", users[0].Name)
		assert.Equal(t, "Bob", users[1].Name)
		assert.Equal(t, "Charlie", users[2].Name)
	})

	t.Run("Scalar_Count", func(t *testing.T) {
		count, err := relica.Scalar[int64](
			db.Select("COUNT(*)").From("generic_test"),
		)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	t.Run("Scalar_MaxScore", func(t *testing.T) {
		max, err := relica.Scalar[int64](
			db.Select("MAX(score)").From("generic_test"),
		)
		require.NoError(t, err)
		assert.Equal(t, int64(95), max)
	})

	t.Run("One_NotFound", func(t *testing.T) {
		_, err := relica.One[GenericRow](
			db.Select().From("generic_test").Where(relica.Eq("name", "Nobody")),
		)
		assert.ErrorIs(t, err, relica.ErrNotFound)
	})

	t.Run("All_EmptyResult", func(t *testing.T) {
		users, err := relica.All[GenericRow](
			db.Select().From("generic_test").Where(relica.Eq("name", "Nobody")),
		)
		require.NoError(t, err)
		assert.Empty(t, users)
	})

	t.Run("All_WithWhereAndLimit", func(t *testing.T) {
		users, err := relica.All[GenericRow](
			db.Select("id", "name", "score").
				From("generic_test").
				Where(relica.GreaterThan("score", 90)).
				OrderBy("score DESC").
				Limit(2),
		)
		require.NoError(t, err)
		require.Len(t, users, 2)
		assert.Equal(t, "Alice", users[0].Name)
		assert.Equal(t, "Charlie", users[1].Name)
	})
}

func TestGeneric_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runGenericTests(t, ds)
}

func TestGeneric_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runGenericTests(t, ds)
}

func TestGeneric_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runGenericTests(t, ds)
}
