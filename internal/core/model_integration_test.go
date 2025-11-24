package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // CGO-free SQLite driver
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

// TestModel_Insert_AutoPopulateID tests auto-population of ID after INSERT.
func TestModel_Insert_AutoPopulateID(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS auto_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Test auto-populate ID.
	type AutoUser struct {
		ID    int64  `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	// First insert.
	user := AutoUser{Name: "Alice", Email: "alice@example.com"}
	err = db.Model(&user).Table("auto_users").Insert()
	require.NoError(t, err)
	assert.NotZero(t, user.ID, "ID should be auto-populated")
	assert.Equal(t, int64(1), user.ID)

	// Second insert.
	user2 := AutoUser{Name: "Bob", Email: "bob@example.com"}
	err = db.Model(&user2).Table("auto_users").Insert()
	require.NoError(t, err)
	assert.Equal(t, int64(2), user2.ID, "ID should be auto-incremented")
}

// TestModel_Insert_PresetID tests that pre-set ID is not overwritten.
func TestModel_Insert_PresetID(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table WITHOUT auto-increment.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS manual_users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Test pre-set ID (should not overwrite).
	type ManualUser struct {
		ID   int64  `db:"id"`
		Name string `db:"name"`
	}

	user := ManualUser{ID: 999, Name: "Charlie"}
	err = db.Model(&user).Table("manual_users").Insert()
	require.NoError(t, err)
	assert.Equal(t, int64(999), user.ID, "ID should remain 999")
}

// TestModel_Insert_NonNumericPK tests that non-numeric PKs are skipped.
func TestModel_Insert_NonNumericPK(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table with UUID primary key.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Test non-numeric PK (should not auto-populate).
	type Document struct {
		ID    string `db:"id"`
		Title string `db:"title"`
	}

	doc := Document{ID: "uuid-123", Title: "Test Doc"}
	err = db.Model(&doc).Table("documents").Insert()
	require.NoError(t, err)
	assert.Equal(t, "uuid-123", doc.ID, "UUID should remain unchanged")
}

// TestModel_Insert_PointerPK tests auto-population for pointer PK fields.
func TestModel_Insert_PointerPK(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Test pointer PK (should allocate and populate).
	type Item struct {
		ID   *int64 `db:"id"`
		Name string `db:"name"`
	}

	item := Item{Name: "Widget"}
	err = db.Model(&item).Table("items").Insert()
	require.NoError(t, err)
	assert.NotNil(t, item.ID, "ID pointer should be allocated")
	if item.ID != nil {
		assert.Equal(t, int64(1), *item.ID)
	}
}

// TestModel_Insert_IntTypes tests auto-population for different int types.
func TestModel_Insert_IntTypes(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		tableName string
		model     interface{}
		checkID   func(t *testing.T, model interface{})
	}{
		{
			name:      "int",
			tableName: "int_items",
			model: &struct {
				ID   int    `db:"id"`
				Name string `db:"name"`
			}{Name: "Test"},
			checkID: func(t *testing.T, model interface{}) {
				m := model.(*struct {
					ID   int    `db:"id"`
					Name string `db:"name"`
				})
				assert.Equal(t, 1, m.ID)
			},
		},
		{
			name:      "int32",
			tableName: "int32_items",
			model: &struct {
				ID   int32  `db:"id"`
				Name string `db:"name"`
			}{Name: "Test"},
			checkID: func(t *testing.T, model interface{}) {
				m := model.(*struct {
					ID   int32  `db:"id"`
					Name string `db:"name"`
				})
				assert.Equal(t, int32(1), m.ID)
			},
		},
		{
			name:      "uint64",
			tableName: "uint64_items",
			model: &struct {
				ID   uint64 `db:"id"`
				Name string `db:"name"`
			}{Name: "Test"},
			checkID: func(t *testing.T, model interface{}) {
				m := model.(*struct {
					ID   uint64 `db:"id"`
					Name string `db:"name"`
				})
				assert.Equal(t, uint64(1), m.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create table.
			_, err := db.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS `+tt.tableName+` (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL
				)
			`)
			require.NoError(t, err)

			// Insert model.
			err = db.Model(tt.model).Table(tt.tableName).Insert()
			require.NoError(t, err)

			// Check ID.
			tt.checkID(t, tt.model)
		})
	}
}

// TestModel_Insert_SelectiveWithExclude tests combination of selective fields and Exclude().
func TestModel_Insert_SelectiveWithExclude(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS selective_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'pending'
		)
	`)
	require.NoError(t, err)

	// Test: Insert("name", "email", "status").Exclude("status")
	// Expected: Only name and email inserted, status remains default.
	user := ModelUser{
		Name:   "TestUser",
		Email:  "test@example.com",
		Status: "active",
	}

	err = db.Model(&user).Table("selective_users").Exclude("status").Insert("name", "email", "status")
	require.NoError(t, err)

	// Verify exclusion took precedence.
	var result ModelUser
	err = db.Builder().Select().From("selective_users").Where("name = ?", "TestUser").One(&result)
	require.NoError(t, err)
	assert.Equal(t, "TestUser", result.Name)
	assert.Equal(t, "test@example.com", result.Email)
	assert.Equal(t, "pending", result.Status, "Status should be default 'pending' (excluded from INSERT)")
}

// TestModel_Update_SelectiveWithExclude tests combination of selective fields and Exclude().
func TestModel_Update_SelectiveWithExclude(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS update_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active'
		)
	`)
	require.NoError(t, err)

	// Insert initial data.
	_, err = db.ExecContext(ctx, `
		INSERT INTO update_users (name, email, status) VALUES ('Original', 'original@example.com', 'active')
	`)
	require.NoError(t, err)

	// Get ID.
	var id int
	err = db.QueryRowContext(ctx, "SELECT id FROM update_users WHERE name = 'Original'").Scan(&id)
	require.NoError(t, err)

	// Test: Update("name", "status").Exclude("status")
	// Expected: Only name updated, status and email unchanged.
	user := ModelUser{
		ID:     id,
		Name:   "Updated",
		Email:  "newemail@example.com",
		Status: "inactive",
	}

	err = db.Model(&user).Table("update_users").Exclude("status").Update("name", "status")
	require.NoError(t, err)

	// Verify exclusion took precedence.
	var result ModelUser
	err = db.Builder().Select().From("update_users").Where("id = ?", id).One(&result)
	require.NoError(t, err)
	assert.Equal(t, "Updated", result.Name, "Name should be updated")
	assert.Equal(t, "original@example.com", result.Email, "Email should remain unchanged")
	assert.Equal(t, "active", result.Status, "Status should remain unchanged (excluded from UPDATE)")
}

// TestModel_Insert_SelectiveWithAutopopulateID tests that auto-populate ID works with selective fields.
func TestModel_Insert_SelectiveWithAutopopulateID(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS auto_selective (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert only "name" (omit "email" intentionally, should use struct value).
	user := ModelUser{
		Name:  "Alice",
		Email: "alice@example.com",
	}

	err = db.Model(&user).Table("auto_selective").Insert("name", "email")
	require.NoError(t, err)

	// Verify ID was auto-populated (TASK-008).
	assert.NotZero(t, user.ID, "ID should be auto-populated even with selective fields")
	assert.Equal(t, int64(1), int64(user.ID))

	// Verify data.
	var result ModelUser
	err = db.Builder().Select().From("auto_selective").Where("id = ?", user.ID).One(&result)
	require.NoError(t, err)
	assert.Equal(t, "Alice", result.Name)
	assert.Equal(t, "alice@example.com", result.Email)
}

// TestModel_Update_MultipleFields tests updating multiple selective fields.
func TestModel_Update_MultipleFields(t *testing.T) {
	db := setupModelTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create table.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS multi_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'active'
		)
	`)
	require.NoError(t, err)

	// Insert initial data.
	_, err = db.ExecContext(ctx, `
		INSERT INTO multi_users (name, email, status) VALUES ('John', 'john@example.com', 'active')
	`)
	require.NoError(t, err)

	// Get ID.
	var id int
	err = db.QueryRowContext(ctx, "SELECT id FROM multi_users WHERE name = 'John'").Scan(&id)
	require.NoError(t, err)

	// Update multiple fields selectively.
	user := ModelUser{
		ID:     id,
		Name:   "John Doe",
		Email:  "johndoe@example.com",
		Status: "inactive",
	}

	err = db.Model(&user).Table("multi_users").Update("name", "email")
	require.NoError(t, err)

	// Verify only name and email updated.
	var result ModelUser
	err = db.Builder().Select().From("multi_users").Where("id = ?", id).One(&result)
	require.NoError(t, err)
	assert.Equal(t, "John Doe", result.Name, "Name should be updated")
	assert.Equal(t, "johndoe@example.com", result.Email, "Email should be updated")
	assert.Equal(t, "active", result.Status, "Status should remain unchanged")
}
