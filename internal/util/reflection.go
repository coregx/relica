// Package util provides utility functions for reflection and struct operations.
package util

import (
	"errors"
	"reflect"
)

// ModelToColumns extracts database columns from struct tags.
func ModelToColumns(model interface{}) map[string]string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	columns := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if tag, ok := field.Tag.Lookup("db"); ok {
			columns[field.Name] = tag
		}
	}
	return columns
}

// StructToMap converts a struct to map[string]interface{} using db tags.
//
// Rules:
//   - Unexported fields are skipped.
//   - db:"-" fields are skipped.
//   - db:"column_name" maps to column_name.
//   - Fields without db tag use field name.
//   - Zero values are included.
//
// Returns error if:
//   - data is not a struct or *struct.
//   - data is nil pointer.
func StructToMap(data interface{}) (map[string]interface{}, error) {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, errors.New("StructToMap: nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, errors.New("StructToMap: expected struct, got " + v.Kind().String())
	}

	t := v.Type()
	result := make(map[string]interface{})

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields.
		if !field.IsExported() {
			continue
		}

		// Get column name from db tag.
		dbName := field.Name
		if tag, ok := field.Tag.Lookup("db"); ok {
			if tag == "-" {
				continue // Skip db:"-" fields.
			}
			dbName = tag
		}

		// Get field value.
		fieldValue := v.Field(i)
		if !fieldValue.IsValid() {
			continue
		}

		result[dbName] = fieldValue.Interface()
	}

	return result, nil
}

// FindPrimaryKeyField finds the primary key field in a struct.
//
// Priority:
//  1. Field with db:"pk" tag (for explicit PK marking)
//  2. Field named "ID"
//  3. Field named "Id"
//
// Returns:
//   - StructField: metadata about the field
//   - Value: reflect.Value of the field
//   - error: if no PK found or composite PK detected
//
// Composite PKs (multiple fields with db:"pk") are not supported.
//
//nolint:cyclop // Acceptable complexity for PK field search with multiple priorities.
func FindPrimaryKeyField(v reflect.Value) (reflect.StructField, reflect.Value, error) {
	// Handle pointer.
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.StructField{}, reflect.Value{}, errors.New("FindPrimaryKeyField: nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return reflect.StructField{}, reflect.Value{}, errors.New("FindPrimaryKeyField: not a struct")
	}

	t := v.Type()
	var idFieldIndex = -1
	var idcaseFieldIndex = -1
	pkCount := 0

	// Search for primary key field.
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Check for db:"pk" tag.
		if tag, ok := field.Tag.Lookup("db"); ok {
			if tag == "pk" {
				pkCount++
				if pkCount > 1 {
					return reflect.StructField{}, reflect.Value{}, errors.New("FindPrimaryKeyField: composite primary keys not supported")
				}
				return field, v.Field(i), nil
			}
		}

		// Track "ID" field as fallback.
		if field.Name == "ID" {
			idFieldIndex = i
		}

		// Track "Id" field as last resort.
		if field.Name == "Id" {
			idcaseFieldIndex = i
		}
	}

	// Fallback to "ID" field.
	if idFieldIndex >= 0 {
		return t.Field(idFieldIndex), v.Field(idFieldIndex), nil
	}

	// Last resort: "Id" field.
	if idcaseFieldIndex >= 0 {
		return t.Field(idcaseFieldIndex), v.Field(idcaseFieldIndex), nil
	}

	return reflect.StructField{}, reflect.Value{}, errors.New("FindPrimaryKeyField: no primary key found")
}

// IsPrimaryKeyZero checks if primary key value is zero (needs auto-population).
//
// Handles:
//   - int types: v.Int() == 0
//   - uint types: v.Uint() == 0
//   - pointers: v.IsNil() || (deref and check)
//
// Returns false for non-numeric types (string, UUID, etc).
func IsPrimaryKeyZero(v reflect.Value) bool {
	// Handle invalid values.
	if !v.IsValid() {
		return true
	}

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Ptr:
		if v.IsNil() {
			return true
		}
		// Recursively check dereferenced value.
		return IsPrimaryKeyZero(v.Elem())
	default:
		// Non-numeric types (string, UUID, etc) don't auto-populate.
		return false
	}
}

// SetPrimaryKeyValue sets primary key value using reflection.
//
// Handles:
//   - int types: int, int8, int16, int32, int64
//   - uint types: uint, uint8, uint16, uint32, uint64
//   - pointers: allocate if nil, then set
//
// Returns error on:
//   - overflow (e.g., int64(1000000) → int8)
//   - unsupported type
//   - non-settable field
//
//nolint:gocognit,cyclop,gocyclo,funlen // Acceptable complexity for handling all numeric types with overflow checks.
func SetPrimaryKeyValue(field reflect.Value, id int64) error {
	if !field.IsValid() {
		return errors.New("SetPrimaryKeyValue: invalid field")
	}

	if !field.CanSet() {
		return errors.New("SetPrimaryKeyValue: field is not settable")
	}

	// Handle pointers.
	if field.Kind() == reflect.Ptr {
		// Allocate if nil.
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		// Recursively set on dereferenced value.
		return SetPrimaryKeyValue(field.Elem(), id)
	}

	// Handle numeric types.
	switch field.Kind() {
	case reflect.Int:
		// Check overflow for platform-specific int size.
		// On 32-bit: -2147483648 to 2147483647
		// On 64-bit: full int64 range
		const maxInt = int(^uint(0) >> 1)
		const minInt = -maxInt - 1
		if id < int64(minInt) || id > int64(maxInt) {
			return errors.New("SetPrimaryKeyValue: int overflow")
		}
		field.SetInt(id)
	case reflect.Int8:
		if id < -128 || id > 127 {
			return errors.New("SetPrimaryKeyValue: int8 overflow")
		}
		field.SetInt(id)
	case reflect.Int16:
		if id < -32768 || id > 32767 {
			return errors.New("SetPrimaryKeyValue: int16 overflow")
		}
		field.SetInt(id)
	case reflect.Int32:
		if id < -2147483648 || id > 2147483647 {
			return errors.New("SetPrimaryKeyValue: int32 overflow")
		}
		field.SetInt(id)
	case reflect.Int64:
		field.SetInt(id)
	case reflect.Uint:
		if id < 0 {
			return errors.New("SetPrimaryKeyValue: uint overflow")
		}
		field.SetUint(uint64(id))
	case reflect.Uint8:
		if id < 0 || id > 255 {
			return errors.New("SetPrimaryKeyValue: uint8 overflow")
		}
		field.SetUint(uint64(id))
	case reflect.Uint16:
		if id < 0 || id > 65535 {
			return errors.New("SetPrimaryKeyValue: uint16 overflow")
		}
		field.SetUint(uint64(id))
	case reflect.Uint32:
		if id < 0 || id > 4294967295 {
			return errors.New("SetPrimaryKeyValue: uint32 overflow")
		}
		field.SetUint(uint64(id))
	case reflect.Uint64:
		if id < 0 {
			return errors.New("SetPrimaryKeyValue: uint64 overflow (negative value)")
		}
		field.SetUint(uint64(id))
	default:
		return errors.New("SetPrimaryKeyValue: unsupported type " + field.Kind().String())
	}

	return nil
}
