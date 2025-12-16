package core

import (
	"database/sql"
)

// NullStringMap represents a map of nullable string values scanned from database rows.
// Each value is a sql.NullString that can be checked for NULL.
// This type is useful for dynamic queries where the schema is not known at compile time.
//
// Example:
//
//	var result NullStringMap
//	db.Select("*").From("users").Where("id = ?", 1).One(&result)
//	name := result.String("name")   // returns empty string if NULL
//	if !result.IsNull("email") {
//	    email := result.String("email")
//	}
type NullStringMap map[string]sql.NullString

// String returns the string value for the given key.
// Returns empty string if key doesn't exist or value is NULL.
func (m NullStringMap) String(key string) string {
	if v, ok := m[key]; ok && v.Valid {
		return v.String
	}
	return ""
}

// IsNull checks if the value for the given key is NULL or doesn't exist.
func (m NullStringMap) IsNull(key string) bool {
	v, ok := m[key]
	return !ok || !v.Valid
}

// Has checks if the key exists in the map (regardless of NULL status).
func (m NullStringMap) Has(key string) bool {
	_, ok := m[key]
	return ok
}

// Keys returns all column names in the map.
func (m NullStringMap) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Get returns the sql.NullString value for the given key and whether it exists.
func (m NullStringMap) Get(key string) (sql.NullString, bool) {
	v, ok := m[key]
	return v, ok
}
