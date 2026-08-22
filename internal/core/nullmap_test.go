package core

import (
	"database/sql"
	"testing"
)

func TestNullStringMap_BasicMethods(t *testing.T) {
	m := NullStringMap{
		"name":  sql.NullString{String: "Alice", Valid: true},
		"email": sql.NullString{String: "alice@example.com", Valid: true},
		"phone": sql.NullString{String: "", Valid: false}, // NULL value
	}

	// Test String method
	if got := m.String("name"); got != "Alice" {
		t.Errorf("String(\"name\") = %q, want \"Alice\"", got)
	}
	if got := m.String("email"); got != "alice@example.com" {
		t.Errorf("String(\"email\") = %q, want \"alice@example.com\"", got)
	}
	if got := m.String("phone"); got != "" {
		t.Errorf("String(\"phone\") = %q, want \"\" (NULL)", got)
	}
	if got := m.String("nonexistent"); got != "" {
		t.Errorf("String(\"nonexistent\") = %q, want \"\"", got)
	}

	// Test IsNull method
	if m.IsNull("name") {
		t.Error("IsNull(\"name\") = true, want false")
	}
	if m.IsNull("email") {
		t.Error("IsNull(\"email\") = true, want false")
	}
	if !m.IsNull("phone") {
		t.Error("IsNull(\"phone\") = false, want true")
	}
	if !m.IsNull("nonexistent") {
		t.Error("IsNull(\"nonexistent\") = false, want true")
	}

	// Test Has method
	if !m.Has("name") {
		t.Error("Has(\"name\") = false, want true")
	}
	if !m.Has("phone") {
		t.Error("Has(\"phone\") = false, want true (key exists even if NULL)")
	}
	if m.Has("nonexistent") {
		t.Error("Has(\"nonexistent\") = true, want false")
	}

	// Test Keys method
	keys := m.Keys()
	if len(keys) != 3 {
		t.Errorf("Keys() returned %d keys, want 3", len(keys))
	}

	// Test Get method
	val, ok := m.Get("name")
	if !ok {
		t.Error("Get(\"name\") ok = false, want true")
	}
	if val.String != "Alice" || !val.Valid {
		t.Errorf("Get(\"name\") = %+v, want {String:\"Alice\", Valid:true}", val)
	}

	val, ok = m.Get("nonexistent")
	if ok {
		t.Error("Get(\"nonexistent\") ok = true, want false")
	}
}
