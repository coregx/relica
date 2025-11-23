package util

import (
	"database/sql"
	"testing"
	"time"
)

// TestUser is a test struct with various field types.
type TestUser struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Status    string    `db:"status"`
	Age       int       `db:"age"`
	internal  string    // Unexported - should be skipped.
	Ignored   int       `db:"-"` // Explicitly ignored.
	CreatedAt time.Time `db:"created_at"`
}

// TestUserNoTags is a struct without db tags.
type TestUserNoTags struct {
	ID   int
	Name string
}

// TestUserMixedTags is a struct with mixed db tags.
type TestUserMixedTags struct {
	ID    int    `db:"user_id"`
	Name  string // No tag - should use field name.
	Email string `db:"email_address"`
}

// TestStructToMap_SimpleStruct tests basic struct conversion.
func TestStructToMap_SimpleStruct(t *testing.T) {
	user := TestUser{
		ID:     123,
		Name:   "Alice",
		Email:  "alice@example.com",
		Status: "active",
		Age:    30,
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	// Check all expected fields.
	if result["id"] != 123 {
		t.Errorf("id = %v, want 123", result["id"])
	}
	if result["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", result["name"])
	}
	if result["email"] != "alice@example.com" {
		t.Errorf("email = %v, want alice@example.com", result["email"])
	}
	if result["status"] != "active" {
		t.Errorf("status = %v, want active", result["status"])
	}
	if result["age"] != 30 {
		t.Errorf("age = %v, want 30", result["age"])
	}

	// Check that unexported and ignored fields are not present.
	if _, ok := result["internal"]; ok {
		t.Error("internal field should not be present")
	}
	if _, ok := result["Ignored"]; ok {
		t.Error("Ignored field should not be present")
	}
}

// TestStructToMap_WithPointer tests struct pointer conversion.
func TestStructToMap_WithPointer(t *testing.T) {
	user := &TestUser{
		ID:    456,
		Name:  "Bob",
		Email: "bob@example.com",
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	if result["id"] != 456 {
		t.Errorf("id = %v, want 456", result["id"])
	}
	if result["name"] != "Bob" {
		t.Errorf("name = %v, want Bob", result["name"])
	}
}

// TestStructToMap_WithDbTags tests db tag mapping.
func TestStructToMap_WithDbTags(t *testing.T) {
	user := TestUserMixedTags{
		ID:    789,
		Name:  "Charlie",
		Email: "charlie@example.com",
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	// Check db tag mapping.
	if result["user_id"] != 789 {
		t.Errorf("user_id = %v, want 789", result["user_id"])
	}
	// Name has no tag, should use field name.
	if result["Name"] != "Charlie" {
		t.Errorf("Name = %v, want Charlie", result["Name"])
	}
	// Email has tag.
	if result["email_address"] != "charlie@example.com" {
		t.Errorf("email_address = %v, want charlie@example.com", result["email_address"])
	}
}

// TestStructToMap_ExcludeFields tests db:"-" exclusion.
func TestStructToMap_ExcludeFields(t *testing.T) {
	user := TestUser{
		ID:      111,
		Ignored: 999, // Should be excluded.
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	if _, ok := result["Ignored"]; ok {
		t.Error("Ignored field with db:\"-\" should not be present")
	}
	if result["id"] != 111 {
		t.Errorf("id = %v, want 111", result["id"])
	}
}

// TestStructToMap_UnexportedFields tests unexported field skipping.
func TestStructToMap_UnexportedFields(t *testing.T) {
	user := TestUser{
		ID:       222,
		internal: "secret", // Should be skipped.
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	if _, ok := result["internal"]; ok {
		t.Error("unexported field should not be present")
	}
	if result["id"] != 222 {
		t.Errorf("id = %v, want 222", result["id"])
	}
}

// TestStructToMap_NilPointer tests nil pointer error.
func TestStructToMap_NilPointer(t *testing.T) {
	var user *TestUser

	_, err := StructToMap(user)
	if err == nil {
		t.Fatal("StructToMap() should return error for nil pointer")
	}
	if err.Error() != "StructToMap: nil pointer" {
		t.Errorf("error = %v, want 'StructToMap: nil pointer'", err)
	}
}

// TestStructToMap_NotStruct tests non-struct type error.
func TestStructToMap_NotStruct(t *testing.T) {
	notStruct := "not a struct"

	_, err := StructToMap(notStruct)
	if err == nil {
		t.Fatal("StructToMap() should return error for non-struct")
	}
	if err.Error() != "StructToMap: expected struct, got string" {
		t.Errorf("error = %v, want 'expected struct, got string'", err)
	}
}

// TestStructToMap_ZeroValues tests that zero values are included.
func TestStructToMap_ZeroValues(t *testing.T) {
	user := TestUser{
		ID: 0, // Zero value.
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	// Zero values should be present.
	if result["id"] != 0 {
		t.Errorf("id = %v, want 0 (zero value should be included)", result["id"])
	}
	if result["name"] != "" {
		t.Errorf("name = %v, want empty string", result["name"])
	}
	if result["age"] != 0 {
		t.Errorf("age = %v, want 0", result["age"])
	}
}

// TestStructToMap_NoTags tests struct without db tags.
func TestStructToMap_NoTags(t *testing.T) {
	user := TestUserNoTags{
		ID:   333,
		Name: "David",
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	// Without tags, field names should be used.
	if result["ID"] != 333 {
		t.Errorf("ID = %v, want 333", result["ID"])
	}
	if result["Name"] != "David" {
		t.Errorf("Name = %v, want David", result["Name"])
	}
}

// TestStructToMap_ComplexTypes tests complex field types.
func TestStructToMap_ComplexTypes(t *testing.T) {
	now := time.Now()
	user := TestUser{
		ID:        444,
		CreatedAt: now,
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	if result["created_at"] != now {
		t.Errorf("created_at = %v, want %v", result["created_at"], now)
	}
}

// TestStructToMap_NullTypes tests sql.Null* types.
func TestStructToMap_NullTypes(t *testing.T) {
	type UserWithNulls struct {
		ID    int            `db:"id"`
		Name  sql.NullString `db:"name"`
		Age   sql.NullInt64  `db:"age"`
		Valid sql.NullBool   `db:"valid"`
	}

	user := UserWithNulls{
		ID:    555,
		Name:  sql.NullString{String: "Eve", Valid: true},
		Age:   sql.NullInt64{Int64: 25, Valid: true},
		Valid: sql.NullBool{Bool: true, Valid: true},
	}

	result, err := StructToMap(user)
	if err != nil {
		t.Fatalf("StructToMap() error = %v", err)
	}

	if result["id"] != 555 {
		t.Errorf("id = %v, want 555", result["id"])
	}

	// sql.Null* types should be passed through as-is.
	if name, ok := result["name"].(sql.NullString); !ok || name.String != "Eve" {
		t.Errorf("name = %v, want sql.NullString{String: Eve, Valid: true}", result["name"])
	}
	if age, ok := result["age"].(sql.NullInt64); !ok || age.Int64 != 25 {
		t.Errorf("age = %v, want sql.NullInt64{Int64: 25, Valid: true}", result["age"])
	}
}
