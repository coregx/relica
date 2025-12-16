package core

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// scanner handles reflection-based scanning of SQL rows into structs.
type scanner struct {
	mu    sync.RWMutex
	cache map[reflect.Type]*structInfo
}

// structInfo contains cached metadata about a struct type.
type structInfo struct {
	fields []*fieldInfo
}

// fieldInfo describes how to scan into a struct field.
type fieldInfo struct {
	index  []int  // field index path for nested structs
	dbName string // column name from db:"" tag or field name
	field  reflect.StructField
}

// newScanner creates a new scanner with empty cache.
func newScanner() *scanner {
	return &scanner{
		cache: make(map[reflect.Type]*structInfo),
	}
}

// globalScanner is the global scanner instance.
var globalScanner = newScanner()

// getStructInfo returns cached struct metadata or builds it.
func (s *scanner) getStructInfo(typ reflect.Type) (*structInfo, error) {
	// Fast path: check cache with read lock
	s.mu.RLock()
	info, ok := s.cache[typ]
	s.mu.RUnlock()

	if ok {
		return info, nil
	}

	// Slow path: build struct info with write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if info, ok := s.cache[typ]; ok {
		return info, nil
	}

	// Build struct info
	info, err := s.buildStructInfo(typ, nil)
	if err != nil {
		return nil, err
	}

	s.cache[typ] = info
	return info, nil
}

// buildStructInfo analyzes struct type and extracts field information.
func (s *scanner) buildStructInfo(typ reflect.Type, index []int) (*structInfo, error) {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("scanner: expected struct, got %s", typ.Kind())
	}

	info := &structInfo{}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Build field index path
		fieldIndex := append(append([]int{}, index...), i)

		// Check if field is embedded struct (recurse)
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			nested, err := s.buildStructInfo(field.Type, fieldIndex)
			if err != nil {
				return nil, err
			}
			info.fields = append(info.fields, nested.fields...)
			continue
		}

		// Get column name from db:"" tag or use field name
		dbName := field.Name
		if tag, ok := field.Tag.Lookup("db"); ok {
			if tag == "-" {
				// Skip this field
				continue
			}
			dbName = tag
		}

		info.fields = append(info.fields, &fieldInfo{
			index:  fieldIndex,
			dbName: strings.ToLower(dbName), // normalize to lowercase
			field:  field,
		})
	}

	return info, nil
}

// scanRow scans a single SQL row into dest struct.
func (s *scanner) scanRow(rows *sql.Rows, dest interface{}) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("scanner: dest must be pointer to struct, got %T", dest)
	}

	destValue = destValue.Elem()
	if destValue.Kind() != reflect.Struct {
		return fmt.Errorf("scanner: dest must be pointer to struct, got pointer to %s", destValue.Kind())
	}

	// Get struct metadata
	info, err := s.getStructInfo(destValue.Type())
	if err != nil {
		return err
	}

	// Get column names from SQL result
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}

	// Build mapping from column name to field
	fieldMap := make(map[string]*fieldInfo, len(info.fields))
	for _, f := range info.fields {
		fieldMap[strings.ToLower(f.dbName)] = f
	}

	// Prepare scan destinations
	scanDests := make([]interface{}, len(columns))
	for i, colName := range columns {
		colName = strings.ToLower(colName)

		if fieldInfo, ok := fieldMap[colName]; ok {
			// Get field value by index path
			fieldValue := destValue
			for _, idx := range fieldInfo.index {
				fieldValue = fieldValue.Field(idx)
			}

			// Use field address as scan destination
			scanDests[i] = fieldValue.Addr().Interface()
		} else {
			// Column not mapped to any field - scan into dummy variable
			var dummy interface{}
			scanDests[i] = &dummy
		}
	}

	// Scan the row
	if err := rows.Scan(scanDests...); err != nil {
		return fmt.Errorf("scanner: scan failed: %w", err)
	}

	return nil
}

// scanRows scans multiple SQL rows into dest slice.
func (s *scanner) scanRows(rows *sql.Rows, dest interface{}) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("scanner: dest must be pointer to slice, got %T", dest)
	}

	sliceValue := destValue.Elem()
	if sliceValue.Kind() != reflect.Slice {
		return fmt.Errorf("scanner: dest must be pointer to slice, got pointer to %s", sliceValue.Kind())
	}

	elemType := sliceValue.Type().Elem()

	// Determine if slice element is pointer or value
	isPtr := elemType.Kind() == reflect.Ptr
	if isPtr {
		elemType = elemType.Elem()
	}

	// Pre-check that element type is struct
	if elemType.Kind() != reflect.Struct {
		return fmt.Errorf("scanner: slice element must be struct or *struct, got %s", elemType.Kind())
	}

	// Get struct metadata once
	info, err := s.getStructInfo(elemType)
	if err != nil {
		return err
	}

	// Get column names from SQL result
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}

	// Build mapping from column name to field
	fieldMap := make(map[string]*fieldInfo, len(info.fields))
	for _, f := range info.fields {
		fieldMap[strings.ToLower(f.dbName)] = f
	}

	// Scan all rows
	for rows.Next() {
		// Create new element
		elemValue := reflect.New(elemType).Elem()

		// Prepare scan destinations
		scanDests := make([]interface{}, len(columns))
		for i, colName := range columns {
			colName = strings.ToLower(colName)

			if fieldInfo, ok := fieldMap[colName]; ok {
				// Get field value by index path
				fieldValue := elemValue
				for _, idx := range fieldInfo.index {
					fieldValue = fieldValue.Field(idx)
				}

				// Use field address as scan destination
				scanDests[i] = fieldValue.Addr().Interface()
			} else {
				// Column not mapped to any field
				var dummy interface{}
				scanDests[i] = &dummy
			}
		}

		// Scan the row
		if err := rows.Scan(scanDests...); err != nil {
			return fmt.Errorf("scanner: scan failed: %w", err)
		}

		// Append to slice
		if isPtr {
			sliceValue.Set(reflect.Append(sliceValue, elemValue.Addr()))
		} else {
			sliceValue.Set(reflect.Append(sliceValue, elemValue))
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("scanner: rows iteration failed: %w", err)
	}

	return nil
}

// scanMapRow scans a single SQL row into a NullStringMap.
// All values are scanned as sql.NullString regardless of actual column type.
func (s *scanner) scanMapRow(rows *sql.Rows, dest *NullStringMap) error {
	// Get column names from SQL result
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}

	// Prepare scan destinations - all as NullString
	values := make([]sql.NullString, len(columns))
	scanDests := make([]interface{}, len(columns))
	for i := range values {
		scanDests[i] = &values[i]
	}

	// Scan the row
	if err := rows.Scan(scanDests...); err != nil {
		return fmt.Errorf("scanner: scan failed: %w", err)
	}

	// Build the map
	*dest = make(NullStringMap, len(columns))
	for i, col := range columns {
		(*dest)[col] = values[i]
	}

	return nil
}

// scanMapRows scans multiple SQL rows into a slice of NullStringMap.
func (s *scanner) scanMapRows(rows *sql.Rows, dest *[]NullStringMap) error {
	// Get column names from SQL result
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}

	// Scan all rows
	for rows.Next() {
		// Prepare scan destinations - all as NullString
		values := make([]sql.NullString, len(columns))
		scanDests := make([]interface{}, len(columns))
		for i := range values {
			scanDests[i] = &values[i]
		}

		// Scan the row
		if err := rows.Scan(scanDests...); err != nil {
			return fmt.Errorf("scanner: scan failed: %w", err)
		}

		// Build the map for this row
		rowMap := make(NullStringMap, len(columns))
		for i, col := range columns {
			rowMap[col] = values[i]
		}

		*dest = append(*dest, rowMap)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("scanner: rows iteration failed: %w", err)
	}

	return nil
}
