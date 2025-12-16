// Package util provides utility functions for reflection and struct operations.
package util

import (
	"errors"
	"reflect"
	"sort"
	"strings"
)

// PrimaryKeyInfo holds information about primary key fields.
// Supports both single PK and composite PK (CPK).
type PrimaryKeyInfo struct {
	Fields  []reflect.StructField // Struct fields in declaration order
	Values  []reflect.Value       // Field values in declaration order
	Columns []string              // DB column names in declaration order
}

// IsSingle returns true if this is a single-column primary key.
func (pk *PrimaryKeyInfo) IsSingle() bool {
	return len(pk.Columns) == 1
}

// IsComposite returns true if this is a composite primary key.
func (pk *PrimaryKeyInfo) IsComposite() bool {
	return len(pk.Columns) > 1
}

// parseDBTag parses db tag to extract column name and pk flag.
//
// Supported formats:
//   - "pk"           -> column="pk", isPK=true (legacy single PK)
//   - "column"       -> column="column", isPK=false
//   - "column,pk"    -> column="column", isPK=true (composite PK)
//   - "-"            -> column="-", isPK=false (skip field)
func parseDBTag(tag string) (column string, isPK bool) {
	parts := strings.Split(tag, ",")
	column = strings.TrimSpace(parts[0])

	// Check for pk in additional parts
	for _, part := range parts[1:] {
		if strings.TrimSpace(part) == "pk" {
			isPK = true
			break
		}
	}

	// Legacy: db:"pk" means column IS "pk" AND it's a primary key
	if column == "pk" {
		isPK = true
	}

	return column, isPK
}

// FindPrimaryKeyFields finds all primary key fields in a struct.
//
// Priority for single PK (backwards compatible):
//  1. Field with db:"pk" tag (explicit single PK)
//  2. Fields with db:"column,pk" tags (composite PK)
//  3. Field named "ID" (fallback)
//  4. Field named "Id" (last resort)
//
// For composite PK, fields are returned in struct declaration order.
//
//nolint:cyclop,gocognit,gocyclo,funlen // Acceptable complexity for PK field search with multiple priorities.
func FindPrimaryKeyFields(v reflect.Value) (*PrimaryKeyInfo, error) {
	// Handle pointer
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, errors.New("FindPrimaryKeyFields: nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, errors.New("FindPrimaryKeyFields: not a struct")
	}

	t := v.Type()

	// Collect all PK fields with their indices for ordering
	type pkField struct {
		index  int
		field  reflect.StructField
		value  reflect.Value
		column string
	}
	var pkFields []pkField
	var legacyPKField *pkField // db:"pk" (legacy single PK)
	var idFieldIndex = -1
	var idcaseFieldIndex = -1

	// First pass: find all PK fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		tag, hasTag := field.Tag.Lookup("db")
		if !hasTag {
			// Track "ID" field as fallback
			if field.Name == "ID" {
				idFieldIndex = i
			}
			// Track "Id" field as last resort
			if field.Name == "Id" {
				idcaseFieldIndex = i
			}
			continue
		}

		column, isPK := parseDBTag(tag)

		// Skip db:"-" fields
		if column == "-" {
			continue
		}

		if isPK {
			pf := pkField{
				index:  i,
				field:  field,
				value:  v.Field(i),
				column: column,
			}

			// Legacy db:"pk" is special - column name is "pk"
			if column == "pk" {
				legacyPKField = &pf
			} else {
				pkFields = append(pkFields, pf)
			}
		}

		// Track "ID" field as fallback (even with db tag)
		if field.Name == "ID" && idFieldIndex == -1 {
			idFieldIndex = i
		}
	}

	// Decision logic:
	// 1. If we have composite PKs (db:"col,pk"), use them
	// 2. Else if we have legacy PK (db:"pk"), use it alone
	// 3. Else fallback to ID/Id field

	if len(pkFields) > 0 {
		// Composite PK or single PK with explicit column name
		// Sort by struct field index to maintain declaration order
		sort.Slice(pkFields, func(i, j int) bool {
			return pkFields[i].index < pkFields[j].index
		})

		info := &PrimaryKeyInfo{
			Fields:  make([]reflect.StructField, len(pkFields)),
			Values:  make([]reflect.Value, len(pkFields)),
			Columns: make([]string, len(pkFields)),
		}
		for i := range pkFields {
			info.Fields[i] = pkFields[i].field
			info.Values[i] = pkFields[i].value
			info.Columns[i] = pkFields[i].column
		}
		return info, nil
	}

	if legacyPKField != nil {
		// Legacy single PK: db:"pk"
		// Column name defaults to field name lowercased
		column := strings.ToLower(legacyPKField.field.Name)
		return &PrimaryKeyInfo{
			Fields:  []reflect.StructField{legacyPKField.field},
			Values:  []reflect.Value{legacyPKField.value},
			Columns: []string{column},
		}, nil
	}

	// Fallback to "ID" field
	if idFieldIndex >= 0 {
		field := t.Field(idFieldIndex)
		column := "id"
		if tag, ok := field.Tag.Lookup("db"); ok && tag != "" && tag != "-" {
			col, _ := parseDBTag(tag)
			if col != "-" {
				column = col
			}
		}
		return &PrimaryKeyInfo{
			Fields:  []reflect.StructField{field},
			Values:  []reflect.Value{v.Field(idFieldIndex)},
			Columns: []string{column},
		}, nil
	}

	// Last resort: "Id" field
	if idcaseFieldIndex >= 0 {
		field := t.Field(idcaseFieldIndex)
		column := "id"
		if tag, ok := field.Tag.Lookup("db"); ok && tag != "" && tag != "-" {
			col, _ := parseDBTag(tag)
			if col != "-" {
				column = col
			}
		}
		return &PrimaryKeyInfo{
			Fields:  []reflect.StructField{field},
			Values:  []reflect.Value{v.Field(idcaseFieldIndex)},
			Columns: []string{column},
		}, nil
	}

	return nil, errors.New("FindPrimaryKeyFields: no primary key found")
}

// ModelToColumns extracts database columns from struct tags.
// Handles composite PK syntax: db:"column_name,pk" -> column_name.
func ModelToColumns(model interface{}) map[string]string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	columns := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if tag, ok := field.Tag.Lookup("db"); ok {
			// Parse db tag to extract only column name
			column, _ := parseDBTag(tag)
			if column != "-" {
				columns[field.Name] = column
			}
		}
	}
	return columns
}

// StructToMap converts a struct to map[string]interface{} using db tags.
//
// Rules:
//   - Unexported fields are skipped.
//   - db:"-" fields are skipped.
//   - db:"column_name" or db:"column_name,pk" maps to column_name.
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
			// Parse db tag: "column" or "column,pk" or "-"
			column, _ := parseDBTag(tag)
			if column == "-" {
				continue // Skip db:"-" fields.
			}
			dbName = column
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
// For composite PKs, use FindPrimaryKeyFields instead.
// This function returns error for composite PKs to maintain backwards compatibility.
func FindPrimaryKeyField(v reflect.Value) (reflect.StructField, reflect.Value, error) {
	pkInfo, err := FindPrimaryKeyFields(v)
	if err != nil {
		return reflect.StructField{}, reflect.Value{}, err
	}

	// Return error for composite PK (backwards compatibility)
	if pkInfo.IsComposite() {
		return reflect.StructField{}, reflect.Value{},
			errors.New("FindPrimaryKeyField: composite primary keys not supported, use FindPrimaryKeyFields")
	}

	return pkInfo.Fields[0], pkInfo.Values[0], nil
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
