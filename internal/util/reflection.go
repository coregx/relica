package util

import (
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
