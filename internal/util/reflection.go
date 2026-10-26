// Package util provides utility functions for reflection and struct operations.
package util

import (
	"errors"
	"reflect"
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
