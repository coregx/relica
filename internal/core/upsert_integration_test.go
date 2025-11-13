package core

import (
	"context"
	"database/sql"
	"testing"

	"github.com/coregx/relica/internal/cache"
	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/logger"
	"github.com/coregx/relica/internal/tracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestUpsertIntegration_SQLite tests actual UPSERT execution with SQLite (pure Go, no Docker)
func TestUpsertIntegration_SQLite(t *testing.T) {
	// Create in-memory SQLite database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer sqlDB.Close()

	db := &DB{
		sqlDB:      sqlDB,
		driverName: "sqlite",
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialects.GetDialect("sqlite"),
		oldTracer:  NewNoOpTracer(),
		logger:     &logger.NoopLogger{},
		tracer:     &tracer.NoopTracer{},
		sanitizer:  logger.NewSanitizer(nil),
		ctx:        context.Background(),
	}

	// Create table
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	t.Run("insert new record", func(t *testing.T) {
		result, err := db.Builder().
			Upsert("users", map[string]interface{}{
				"id":    1,
				"name":  "Alice",
				"email": "alice@example.com",
			}).
			OnConflict("id").
			DoUpdate("name", "email").
			Execute()

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify inserted
		var count int
		err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("update existing record", func(t *testing.T) {
		// Update the same ID
		result, err := db.Builder().
			Upsert("users", map[string]interface{}{
				"id":    1,
				"name":  "Alice Updated",
				"email": "alice.new@example.com",
			}).
			OnConflict("id").
			DoUpdate("name", "email").
			Execute()

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify still only 1 record
		var count int
		err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Verify updated values
		var name, email string
		err = db.QueryRowContext(context.Background(),
			"SELECT name, email FROM users WHERE id = 1").Scan(&name, &email)
		require.NoError(t, err)
		assert.Equal(t, "Alice Updated", name)
		assert.Equal(t, "alice.new@example.com", email)
	})

	t.Run("do nothing on conflict", func(t *testing.T) {
		// Try to insert with same ID but DoNothing
		result, err := db.Builder().
			Upsert("users", map[string]interface{}{
				"id":    1,
				"name":  "Should Not Update",
				"email": "should.not@update.com",
			}).
			OnConflict("id").
			DoNothing().
			Execute()

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify values NOT changed
		var name, email string
		err = db.QueryRowContext(context.Background(),
			"SELECT name, email FROM users WHERE id = 1").Scan(&name, &email)
		require.NoError(t, err)
		assert.Equal(t, "Alice Updated", name)
		assert.Equal(t, "alice.new@example.com", email)
	})

	t.Run("auto-update all columns except conflict", func(t *testing.T) {
		// Insert new user
		_, err := db.ExecContext(context.Background(), `
			INSERT INTO users (id, name, email) VALUES (2, 'Bob', 'bob@example.com')
		`)
		require.NoError(t, err)

		// Upsert without specifying update columns
		result, err := db.Builder().
			Upsert("users", map[string]interface{}{
				"id":    2,
				"name":  "Bob Updated",
				"email": "bob.updated@example.com",
			}).
			OnConflict("id").
			Execute() // No DoUpdate() call - should update all except "id"

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify both name and email updated
		var name, email string
		err = db.QueryRowContext(context.Background(),
			"SELECT name, email FROM users WHERE id = 2").Scan(&name, &email)
		require.NoError(t, err)
		assert.Equal(t, "Bob Updated", name)
		assert.Equal(t, "bob.updated@example.com", email)
	})
}

// TestUpsertIntegration_MultipleConflictColumns tests composite unique constraints
func TestUpsertIntegration_MultipleConflictColumns(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer sqlDB.Close()

	db := &DB{
		sqlDB:      sqlDB,
		driverName: "sqlite",
		stmtCache:  cache.NewStmtCache(),
		dialect:    dialects.GetDialect("sqlite"),
		oldTracer:  NewNoOpTracer(),
		logger:     &logger.NoopLogger{},
		tracer:     &tracer.NoopTracer{},
		sanitizer:  logger.NewSanitizer(nil),
		ctx:        context.Background(),
	}

	// Create table with composite unique constraint
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE subscriptions (
			user_id INTEGER NOT NULL,
			topic_id INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			UNIQUE(user_id, topic_id)
		)
	`)
	require.NoError(t, err)

	// Insert initial record
	result, err := db.Builder().
		Upsert("subscriptions", map[string]interface{}{
			"user_id":  1,
			"topic_id": 10,
			"enabled":  1,
		}).
		OnConflict("user_id", "topic_id").
		DoUpdate("enabled").
		Execute()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Update with same composite key
	result, err = db.Builder().
		Upsert("subscriptions", map[string]interface{}{
			"user_id":  1,
			"topic_id": 10,
			"enabled":  0,
		}).
		OnConflict("user_id", "topic_id").
		DoUpdate("enabled").
		Execute()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify updated
	var enabled int
	err = db.QueryRowContext(context.Background(),
		"SELECT enabled FROM subscriptions WHERE user_id = 1 AND topic_id = 10").Scan(&enabled)
	require.NoError(t, err)
	assert.Equal(t, 0, enabled)

	// Verify still only 1 record
	var count int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM subscriptions").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
