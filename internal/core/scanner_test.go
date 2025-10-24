package core

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

const (
	testUserName  = "Alice"
	testUserEmail = "alice@example.com"
)

// Test types.
type SimpleUser struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

type UserWithTags struct {
	UserID   int    `db:"id"`
	FullName string `db:"name"`
	Contact  string `db:"email"`
}

type UserNoTags struct {
	ID    int
	Name  string
	Email string
}

type UserWithIgnored struct {
	ID       int    `db:"id"`
	Name     string `db:"name"`
	Email    string `db:"email"`
	Password string `db:"-"` // Should be ignored
}

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO users (id, name, email) VALUES
		(1, 'Alice', 'alice@example.com'),
		(2, 'Bob', 'bob@example.com'),
		(3, 'Charlie', 'charlie@example.com')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	return db
}

func TestScannerSimpleStruct(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users WHERE id = 1")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("No rows returned")
	}

	var user SimpleUser
	if err := globalScanner.scanRow(rows, &user); err != nil {
		t.Fatalf("scanRow failed: %v", err)
	}

	if user.ID != 1 {
		t.Errorf("Expected ID=1, got %d", user.ID)
	}
	if user.Name != testUserName {
		t.Errorf("Expected Name='%s', got '%s'", testUserName, user.Name)
	}
	if user.Email != testUserEmail {
		t.Errorf("Expected Email='%s', got '%s'", testUserEmail, user.Email)
	}
}

func TestScannerWithTags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users WHERE id = 2")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("No rows returned")
	}

	var user UserWithTags
	if err := globalScanner.scanRow(rows, &user); err != nil {
		t.Fatalf("scanRow failed: %v", err)
	}

	if user.UserID != 2 {
		t.Errorf("Expected UserID=2, got %d", user.UserID)
	}
	if user.FullName != "Bob" {
		t.Errorf("Expected FullName='Bob', got '%s'", user.FullName)
	}
	if user.Contact != "bob@example.com" {
		t.Errorf("Expected Contact='bob@example.com', got '%s'", user.Contact)
	}
}

func TestScannerNoTags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users WHERE id = 3")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("No rows returned")
	}

	var user UserNoTags
	if err := globalScanner.scanRow(rows, &user); err != nil {
		t.Fatalf("scanRow failed: %v", err)
	}

	if user.ID != 3 {
		t.Errorf("Expected ID=3, got %d", user.ID)
	}
	if user.Name != "Charlie" {
		t.Errorf("Expected Name='Charlie', got '%s'", user.Name)
	}
	if user.Email != "charlie@example.com" {
		t.Errorf("Expected Email='charlie@example.com', got '%s'", user.Email)
	}
}

func TestScannerMultipleRows(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users ORDER BY id")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var users []SimpleUser
	if err := globalScanner.scanRows(rows, &users); err != nil {
		t.Fatalf("scanRows failed: %v", err)
	}

	if len(users) != 3 {
		t.Fatalf("Expected 3 users, got %d", len(users))
	}

	expected := []SimpleUser{
		{ID: 1, Name: testUserName, Email: testUserEmail},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
		{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
	}

	for i, user := range users {
		if user != expected[i] {
			t.Errorf("User %d: expected %+v, got %+v", i, expected[i], user)
		}
	}
}

func TestScannerPointerSlice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users ORDER BY id")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var users []*SimpleUser
	if err := globalScanner.scanRows(rows, &users); err != nil {
		t.Fatalf("scanRows failed: %v", err)
	}

	if len(users) != 3 {
		t.Fatalf("Expected 3 users, got %d", len(users))
	}

	if users[0].ID != 1 || users[0].Name != testUserName {
		t.Errorf("Unexpected user[0]: %+v", users[0])
	}
}

func TestScannerIgnoredField(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users WHERE id = 1")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("No rows returned")
	}

	var user UserWithIgnored
	if err := globalScanner.scanRow(rows, &user); err != nil {
		t.Fatalf("scanRow failed: %v", err)
	}

	// Password should remain empty (ignored field)
	if user.Password != "" {
		t.Errorf("Expected Password to be empty, got '%s'", user.Password)
	}

	// Other fields should be populated
	if user.ID != 1 || user.Name != testUserName {
		t.Errorf("Expected ID=1, Name='%s', got ID=%d, Name='%s'", testUserName, user.ID, user.Name)
	}
}

func TestScannerEmptyResult(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users WHERE id = 999")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var users []SimpleUser
	if err := globalScanner.scanRows(rows, &users); err != nil {
		t.Fatalf("scanRows failed: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(users))
	}
}

func TestScannerErrorNotPointer(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.Query("SELECT id, name, email FROM users WHERE id = 1")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("No rows returned")
	}

	var user SimpleUser
	// Pass value instead of pointer - should error
	err = globalScanner.scanRow(rows, user)
	if err == nil {
		t.Error("Expected error when passing non-pointer, got nil")
	}
}
