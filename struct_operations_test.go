package relica

import (
	"context"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestStructUser is a test struct for struct operations.
type TestStructUser struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Status    string    `db:"status"`
	Age       int       `db:"age"`
	CreatedAt time.Time `db:"created_at"`
	Ignored   int       `db:"-"` // Explicitly ignored.
}

// TestStructUserInsert is a struct for inserts (excludes auto-increment ID).
type TestStructUserInsert struct {
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Status    string    `db:"status"`
	Age       int       `db:"age"`
	CreatedAt time.Time `db:"created_at"`
}

// setupStructOpsTestDB creates an in-memory SQLite database for testing.
func setupStructOpsTestDB(t *testing.T) *DB {
	t.Helper()

	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create test table.
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			age INTEGER DEFAULT 0,
			created_at DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	return db
}

// TestDB_InsertStruct_Simple tests basic struct insert.
func TestDB_InsertStruct_Simple(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	user := TestStructUserInsert{
		Name:      "Alice",
		Email:     "alice@example.com",
		Status:    "active",
		Age:       30,
		CreatedAt: time.Now(),
	}

	// Insert using InsertStruct.
	result, err := db.InsertStruct("users", &user).Execute()
	if err != nil {
		t.Fatalf("InsertStruct() failed: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() failed: %v", err)
	}

	if id < 1 {
		t.Errorf("expected ID >= 1, got %d", id)
	}

	// Verify the data was inserted.
	var inserted TestStructUser
	err = db.Select("*").From("users").Where("id = ?", id).One(&inserted)
	if err != nil {
		t.Fatalf("Select() failed: %v", err)
	}

	if inserted.Name != user.Name {
		t.Errorf("Name = %v, want %v", inserted.Name, user.Name)
	}
	if inserted.Email != user.Email {
		t.Errorf("Email = %v, want %v", inserted.Email, user.Email)
	}
	if inserted.Status != user.Status {
		t.Errorf("Status = %v, want %v", inserted.Status, user.Status)
	}
	if inserted.Age != user.Age {
		t.Errorf("Age = %v, want %v", inserted.Age, user.Age)
	}
}

// TestDB_InsertStruct_WithTransaction tests struct insert within a transaction.
func TestDB_InsertStruct_WithTransaction(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() failed: %v", err)
	}
	defer tx.Rollback()

	user := TestStructUserInsert{
		Name:   "Bob",
		Email:  "bob@example.com",
		Status: "pending",
		Age:    25,
	}

	// Test InsertStruct in transaction.
	result, err := tx.InsertStruct("users", &user).Execute()
	if err != nil {
		t.Fatalf("tx.InsertStruct() failed: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() failed: %v", err)
	}

	// Commit transaction.
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	// Verify data was committed.
	var inserted TestStructUser
	err = db.Select("*").From("users").Where("id = ?", id).One(&inserted)
	if err != nil {
		t.Fatalf("Select() failed: %v", err)
	}

	if inserted.Name != user.Name {
		t.Errorf("Name = %v, want %v", inserted.Name, user.Name)
	}
}

// TestDB_InsertStruct_WithContext tests struct insert with context.
func TestDB_InsertStruct_WithContext(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user := TestStructUserInsert{
		Name:   "Charlie",
		Email:  "charlie@example.com",
		Status: "active",
		Age:    35,
	}

	result, err := db.Builder().WithContext(ctx).InsertStruct("users", &user).Execute()
	if err != nil {
		t.Fatalf("InsertStruct() with context failed: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() failed: %v", err)
	}

	if id < 1 {
		t.Errorf("expected ID >= 1, got %d", id)
	}
}

// TestDB_BatchInsertStruct_MultipleRows tests batch insert of structs.
func TestDB_BatchInsertStruct_MultipleRows(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	// Prepare batch data.
	users := []TestStructUserInsert{
		{Name: "Alice", Email: "alice@example.com", Status: "active", Age: 30},
		{Name: "Bob", Email: "bob@example.com", Status: "pending", Age: 25},
		{Name: "Charlie", Email: "charlie@example.com", Status: "active", Age: 35},
	}

	// Test batch insert using BatchInsertStruct.
	result, err := db.BatchInsertStruct("users", users).Execute()
	if err != nil {
		t.Fatalf("BatchInsertStruct() failed: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected() failed: %v", err)
	}

	if rows != 3 {
		t.Errorf("expected 3 rows affected, got %d", rows)
	}

	// Verify all users were inserted.
	var inserted []TestStructUser
	err = db.Select("*").From("users").OrderBy("id").All(&inserted)
	if err != nil {
		t.Fatalf("Select() failed: %v", err)
	}

	if len(inserted) != 3 {
		t.Fatalf("expected 3 users, got %d", len(inserted))
	}

	for i, user := range users {
		if inserted[i].Name != user.Name {
			t.Errorf("User[%d].Name = %v, want %v", i, inserted[i].Name, user.Name)
		}
		if inserted[i].Email != user.Email {
			t.Errorf("User[%d].Email = %v, want %v", i, inserted[i].Email, user.Email)
		}
	}
}

// TestDB_BatchInsertStruct_EmptySlice tests batch insert with empty slice (should error).
func TestDB_BatchInsertStruct_EmptySlice(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	// Empty slice should return error.
	var users []TestStructUserInsert

	_, err := db.BatchInsertStruct("users", users).Execute()
	if err == nil {
		t.Fatal("BatchInsertStruct() with empty slice should return error")
	}

	expectedErr := "BatchInsertStruct: empty slice"
	if err.Error() != expectedErr {
		t.Errorf("error = %v, want %v", err.Error(), expectedErr)
	}
}

// TestDB_UpdateStruct_Simple tests basic struct update.
func TestDB_UpdateStruct_Simple(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	// Insert initial data.
	user := TestStructUserInsert{
		Name:   "Alice",
		Email:  "alice@example.com",
		Status: "pending",
		Age:    30,
	}

	result, err := db.InsertStruct("users", &user).Execute()
	if err != nil {
		t.Fatalf("InsertStruct() failed: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() failed: %v", err)
	}

	// Update using struct.
	updatedUser := TestStructUserInsert{
		Name:   "", // Include all fields for update.
		Email:  "",
		Status: "active",
		Age:    31,
	}

	result, err = db.UpdateStruct("users", &updatedUser).Where("id = ?", id).Execute()
	if err != nil {
		t.Fatalf("UpdateStruct() failed: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected() failed: %v", err)
	}

	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	// Verify the update.
	var updated TestStructUser
	err = db.Select("*").From("users").Where("id = ?", id).One(&updated)
	if err != nil {
		t.Fatalf("Select() failed: %v", err)
	}

	if updated.Status != "active" {
		t.Errorf("Status = %v, want active", updated.Status)
	}
	if updated.Age != 31 {
		t.Errorf("Age = %v, want 31", updated.Age)
	}
}

// TestDB_UpdateStruct_WithWhere tests struct update with WHERE clause.
func TestDB_UpdateStruct_WithWhere(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	// Insert multiple users.
	users := []TestStructUserInsert{
		{Name: "Alice", Email: "alice@example.com", Status: "pending", Age: 30},
		{Name: "Bob", Email: "bob@example.com", Status: "pending", Age: 25},
	}

	for _, user := range users {
		_, err := db.InsertStruct("users", &user).Execute()
		if err != nil {
			t.Fatalf("InsertStruct() failed: %v", err)
		}
	}

	// Update only pending users.
	updatedUser := TestStructUserInsert{
		Name:   "",
		Email:  "",
		Status: "active",
		Age:    0,
	}

	result, err := db.UpdateStruct("users", &updatedUser).Where("status = ?", "pending").Execute()
	if err != nil {
		t.Fatalf("UpdateStruct() failed: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected() failed: %v", err)
	}

	if rows != 2 {
		t.Errorf("expected 2 rows affected, got %d", rows)
	}

	// Verify all users are now active.
	var updated []TestStructUser
	err = db.Select("*").From("users").All(&updated)
	if err != nil {
		t.Fatalf("Select() failed: %v", err)
	}

	for i, user := range updated {
		if user.Status != "active" {
			t.Errorf("User[%d].Status = %v, want active", i, user.Status)
		}
	}
}

// TestDB_StructOperations_ZeroValues tests that zero values are correctly handled.
func TestDB_StructOperations_ZeroValues(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	// Insert user with zero values.
	user := TestStructUserInsert{
		Name:   "Zero",
		Email:  "zero@example.com",
		Status: "", // Zero value.
		Age:    0,  // Zero value.
	}

	result, err := db.InsertStruct("users", &user).Execute()
	if err != nil {
		t.Fatalf("InsertStruct() failed: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() failed: %v", err)
	}

	// Verify zero values were inserted.
	var inserted TestStructUser
	err = db.Select("*").From("users").Where("id = ?", id).One(&inserted)
	if err != nil {
		t.Fatalf("Select() failed: %v", err)
	}

	if inserted.Status != "" {
		t.Errorf("Status = %v, want empty string", inserted.Status)
	}
	if inserted.Age != 0 {
		t.Errorf("Age = %v, want 0", inserted.Age)
	}
}

// TestDB_InsertStruct_NilPointer tests error handling for nil pointer.
func TestDB_InsertStruct_NilPointer(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	var user *TestStructUser // nil pointer.

	_, err := db.InsertStruct("users", user).Execute()
	if err == nil {
		t.Fatal("InsertStruct() with nil pointer should return error")
	}

	if err.Error() != "StructToMap: nil pointer" {
		t.Errorf("error = %v, want 'StructToMap: nil pointer'", err.Error())
	}
}

// TestDB_InsertStruct_NotStruct tests error handling for non-struct type.
func TestDB_InsertStruct_NotStruct(t *testing.T) {
	db := setupStructOpsTestDB(t)
	defer db.Close()

	notStruct := "not a struct"

	_, err := db.InsertStruct("users", notStruct).Execute()
	if err == nil {
		t.Fatal("InsertStruct() with non-struct should return error")
	}

	if err.Error() != "StructToMap: expected struct, got string" {
		t.Errorf("error = %v, want 'expected struct, got string'", err.Error())
	}
}
