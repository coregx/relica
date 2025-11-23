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
