package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ModelUser is a test model for integration tests.
type ModelUser struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Status    string    `db:"status"`
	CreatedAt time.Time `db:"created_at"`
}

func (ModelUser) TableName() string {
	return "model_users"
}

// setupModelTestDB creates an in-memory SQLite database for testing.
func setupModelTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	return db
}

func TestModel_Insert_Simple(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS model_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Insert model.
	user := ModelUser{
		Name:   "Alice",
		Email:  "alice@example.com",
		Status: "active",
	}

	err = db.Model(&user).Insert()
	require.NoError(t, err)

	// Verify insertion.
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM model_users").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify data.
	var result ModelUser
	err = db.Builder().Select().From("model_users").One(&result)
	require.NoError(t, err)
	assert.Equal(t, "Alice", result.Name)
	assert.Equal(t, "alice@example.com", result.Email)
}

func TestModel_Insert_WithExclude(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS model_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Insert model with excluded fields.
	user := ModelUser{
		Name:      "Bob",
		Email:     "bob@example.com",
		Status:    "inactive",
		CreatedAt: time.Now(),
	}

	err = db.Model(&user).Exclude("created_at").Insert()
	require.NoError(t, err)

	// Verify status was included (not excluded).
	var result ModelUser
	err = db.Builder().Select().From("model_users").One(&result)
	require.NoError(t, err)
	assert.Equal(t, "inactive", result.Status)
}

func TestModel_Insert_OnlyFields(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS model_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Insert only specific fields.
	user := ModelUser{
		Name:   "Charlie",
		Email:  "charlie@example.com",
		Status: "pending",
	}

	err = db.Model(&user).Insert("name", "email")
	require.NoError(t, err)

	// Verify status is default (not from struct).
	var result ModelUser
	err = db.Builder().Select().From("model_users").One(&result)
	require.NoError(t, err)
	assert.Equal(t, "Charlie", result.Name)
	assert.Equal(t, "charlie@example.com", result.Email)
	assert.Equal(t, "active", result.Status, "Status should be default 'active', not 'pending'")
}

func TestModel_Update_Simple(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS model_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active'
		)
	`)
	require.NoError(t, err)

	// Insert initial data.
	_, err = db.ExecContext(ctx, `
		INSERT INTO model_users (name, email, status) VALUES ('Dave', 'dave@example.com', 'active')
	`)
	require.NoError(t, err)

	// Get the inserted ID.
	var id int
	err = db.QueryRowContext(ctx, "SELECT id FROM model_users WHERE name = 'Dave'").Scan(&id)
	require.NoError(t, err)

	// Update model.
	user := ModelUser{
		ID:     id,
		Name:   "Dave Updated",
		Email:  "dave.new@example.com",
		Status: "inactive",
	}

	err = db.Model(&user).Exclude("created_at").Update()
	require.NoError(t, err)

	// Verify update.
	var result ModelUser
	err = db.Builder().Select().From("model_users").Where("id = ?", id).One(&result)
	require.NoError(t, err)
	assert.Equal(t, "Dave Updated", result.Name)
	assert.Equal(t, "dave.new@example.com", result.Email)
	assert.Equal(t, "inactive", result.Status)
}

func TestModel_Update_OnlyFields(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS model_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active'
		)
	`)
	require.NoError(t, err)

	// Insert initial data.
	_, err = db.ExecContext(ctx, `
		INSERT INTO model_users (name, email, status) VALUES ('Eve', 'eve@example.com', 'active')
	`)
	require.NoError(t, err)

	// Get the inserted ID.
	var id int
	err = db.QueryRowContext(ctx, "SELECT id FROM model_users WHERE name = 'Eve'").Scan(&id)
	require.NoError(t, err)

	// Update only status.
	user := ModelUser{
		ID:     id,
		Name:   "Eve Updated",
		Email:  "eve.new@example.com",
		Status: "inactive",
	}

	err = db.Model(&user).Update("status")
	require.NoError(t, err)

	// Verify only status changed.
	var result ModelUser
	err = db.Builder().Select().From("model_users").Where("id = ?", id).One(&result)
	require.NoError(t, err)
	assert.Equal(t, "Eve", result.Name, "Name should not change")
	assert.Equal(t, "eve@example.com", result.Email, "Email should not change")
	assert.Equal(t, "inactive", result.Status, "Status should be updated")
}

func TestModel_Delete_Simple(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS model_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert data.
	_, err = db.ExecContext(ctx, `
		INSERT INTO model_users (name, email) VALUES ('Frank', 'frank@example.com')
	`)
	require.NoError(t, err)

	// Get the inserted ID.
	var id int
	err = db.QueryRowContext(ctx, "SELECT id FROM model_users WHERE name = 'Frank'").Scan(&id)
	require.NoError(t, err)

	// Delete model.
	user := ModelUser{
		ID: id,
	}

	err = db.Model(&user).Delete()
	require.NoError(t, err)

	// Verify deletion.
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM model_users WHERE id = ?", id).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestModel_WithTransaction(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS model_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	t.Run("Rollback", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback()

		user := ModelUser{
			Name:  "Grace",
			Email: "grace@example.com",
		}

		err = tx.Model(&user).Exclude("created_at", "status").Insert()
		require.NoError(t, err)

		// Rollback.
		err = tx.Rollback()
		require.NoError(t, err)

		// Verify data doesn't exist after rollback.
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM model_users WHERE name = 'Grace'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("Commit", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback()

		user := ModelUser{
			Name:  "Hank",
			Email: "hank@example.com",
		}

		err = tx.Model(&user).Exclude("created_at", "status").Insert()
		require.NoError(t, err)

		// Commit.
		err = tx.Commit()
		require.NoError(t, err)

		// Verify data persists after commit.
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM model_users WHERE name = 'Hank'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func TestModel_TableOverride(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create archive table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users_archive (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert into archive table.
	user := ModelUser{
		Name:  "Ian",
		Email: "ian@example.com",
	}

	err = db.Model(&user).Exclude("created_at", "status").Table("users_archive").Insert()
	require.NoError(t, err)

	// Verify in archive table.
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users_archive WHERE name = 'Ian'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestModel_ErrorNoPrimaryKey(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	// Model without primary key.
	type NoPK struct {
		Name string `db:"name"`
	}

	noPK := NoPK{Name: "Test"}

	// Update should fail (no PK).
	err := db.Model(&noPK).Update()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "primary key not found")

	// Delete should fail (no PK).
	err = db.Model(&noPK).Delete()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "primary key not found")
}
