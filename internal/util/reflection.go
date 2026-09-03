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
	Fields        []reflect.StructField // Struct fields in declaration order
	Values        []reflect.Value       // Field values in declaration order
	Columns       []string              // DB column names in declaration order
	AutoIncrement bool                  // True if single PK has autoincrement tag (enables RETURNING for non-numeric PKs)
}

// IsSingle returns true if this is a single-column primary key.
func (pk *PrimaryKeyInfo) IsSingle() bool {
	return len(pk.Columns) == 1
}

// IsComposite returns true if this is a composite primary key.
func (pk *PrimaryKeyInfo) IsComposite() bool {
	return len(pk.Columns) > 1
}

// parseDBTag parses db tag to extract column name, pk flag, and autoid config.
//
// Supported formats:
//   - "pk"                              -> column="pk", isPK=true (legacy single PK)
//   - "column"                          -> column="column", isPK=false
//   - "column,pk"                       -> column="column", isPK=true (composite PK)
//   - "column,pk,autoincrement"         -> column="column", isPK=true, isAutoIncrement=true
//   - "column,autoid"                   -> column="column", autoIDPrefix="" (no prefix)
//   - "column,autoid:usr"               -> column="column", autoIDPrefix="usr"
//   - "column,autoid:usr,gen=ulid"      -> column="column", autoIDPrefix="usr", autoIDGen="ulid"
//   - "-"                               -> column="-", isPK=false (skip field)
//
// DBTagInfo holds parsed struct tag information.
type DBTagInfo struct {
	Column        string
	IsPK          bool
	AutoIncrement bool
	IsAutoID      bool   // true if autoid or autoid:prefix is present
	AutoIDPrefix  string // "usr", "ord", "" (no prefix)
	AutoIDGen     string // "uuid7" (default), "ulid", custom
}

func parseDBTag(tag string) (column string, isPK, isAutoIncrement bool) {
	info := ParseDBTagFull(tag)
	return info.Column, info.IsPK, info.AutoIncrement
}

// ParseDBTagFull parses db tag into DBTagInfo with all modifiers including autoid.
func ParseDBTagFull(tag string) DBTagInfo {
	parts := strings.Split(tag, ",")
	info := DBTagInfo{Column: strings.TrimSpace(parts[0])}

	for _, part := range parts[1:] {
		trimmed := strings.TrimSpace(part)
		switch {
		case trimmed == "pk":
			info.IsPK = true
		case trimmed == "autoincrement":
			info.AutoIncrement = true
		case trimmed == "autoid":
			info.IsAutoID = true
		case strings.HasPrefix(trimmed, "autoid:"):
			info.IsAutoID = true
			info.AutoIDPrefix = strings.TrimPrefix(trimmed, "autoid:")
		case strings.HasPrefix(trimmed, "gen="):
			info.AutoIDGen = strings.TrimPrefix(trimmed, "gen=")
		}
	}

	if info.Column == "pk" {
		info.IsPK = true
	}

	return info
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
	if v.Kind() == reflect.Pointer {
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
		index         int
		field         reflect.StructField
		value         reflect.Value
		column        string
		autoIncrement bool
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

		column, isPK, isAutoInc := parseDBTag(tag)

		// Skip db:"-" fields
		if column == "-" {
			continue
		}

		if isPK {
			pf := pkField{
				index:         i,
				field:         field,
				value:         v.Field(i),
				column:        column,
				autoIncrement: isAutoInc,
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
		// AutoIncrement applies to single PK only.
		if len(pkFields) == 1 {
			info.AutoIncrement = pkFields[0].autoIncrement
		}
		return info, nil
	}

	if legacyPKField != nil {
		// Legacy single PK: db:"pk"
		// Column name defaults to field name lowercased
		column := strings.ToLower(legacyPKField.field.Name)
		return &PrimaryKeyInfo{
			Fields:        []reflect.StructField{legacyPKField.field},
			Values:        []reflect.Value{legacyPKField.value},
			Columns:       []string{column},
			AutoIncrement: legacyPKField.autoIncrement,
		}, nil
	}

	// Fallback to "ID" field
	if idFieldIndex >= 0 {
		field := t.Field(idFieldIndex)
		column := "id"
		if tag, ok := field.Tag.Lookup("db"); ok && tag != "" && tag != "-" {
			col, _, _ := parseDBTag(tag)
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
			col, _, _ := parseDBTag(tag)
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

// AutoIDInfo holds information about a field with the autoid tag.
type AutoIDInfo struct {
	FieldIndex int
	Column     string
	Prefix     string // "usr", "ord", "" (no prefix)
	Generator  string // "", "uuid7", "ulid", custom
}

// FindAutoIDFields finds all fields with the autoid tag in a struct.
// Returns nil if no autoid fields are found.
func FindAutoIDFields(v reflect.Value) []AutoIDInfo {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	var fields []AutoIDInfo

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag, ok := field.Tag.Lookup("db")
		if !ok {
			continue
		}

		info := ParseDBTagFull(tag)
		if !info.IsAutoID {
			continue
		}

		fields = append(fields, AutoIDInfo{
			FieldIndex: i,
			Column:     info.Column,
			Prefix:     info.AutoIDPrefix,
			Generator:  info.AutoIDGen,
		})
	}

	return fields
}

// ModelToColumns extracts database columns from struct tags.
// Handles composite PK syntax: db:"column_name,pk" -> column_name.
func ModelToColumns(model any) map[string]string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	columns := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if tag, ok := field.Tag.Lookup("db"); ok {
			// Parse db tag to extract only column name
			column, _, _ := parseDBTag(tag)
			if column != "-" {
				columns[field.Name] = column
			}
		}
	}
	return columns
}

// StructToMap converts a struct to map[string]any using db tags.
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
func StructToMap(data any) (map[string]any, error) {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, errors.New("StructToMap: nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, errors.New("StructToMap: expected struct, got " + v.Kind().String())
	}

	t := v.Type()
	result := make(map[string]any)

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
			column, _, _ := parseDBTag(tag)
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
// Handles all types via reflect.IsZero():
//   - int/uint: == 0
//   - string: == ""
//   - [16]byte (uuid.UUID): all bytes zero
//   - pointers: nil = zero, otherwise checks pointed value
//   - structs: all fields zero
func IsPrimaryKeyZero(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		return IsPrimaryKeyZero(v.Elem())
	}
	return v.IsZero()
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
	if field.Kind() == reflect.Pointer {
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
