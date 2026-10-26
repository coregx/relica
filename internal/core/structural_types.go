package core

import (
	"database/sql"
	"reflect"
)

// Package relica declares its own Params and NullStringMap (ADR-016): distinct
// named types whose underlying types equal core's. core recognizes them by
// structure at the only places that need them — named-parameter resolution and
// map scanning — so the public types need no conversion at the API boundary.

var (
	paramsType        = reflect.TypeOf(Params(nil))
	nullStringMapType = reflect.TypeOf(NullStringMap(nil))
)

// asParams reports whether v is a named-parameter map — core.Params or any type
// whose underlying type is map[string]any — and returns it as Params.
func asParams(v any) (Params, bool) {
	if p, ok := v.(Params); ok {
		return p, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map || !rv.Type().ConvertibleTo(paramsType) {
		return nil, false
	}
	return rv.Convert(paramsType).Interface().(Params), true
}

// isNullStringMapType reports whether t is NullStringMap or another named or
// unnamed type with underlying map[string]sql.NullString.
func isNullStringMapType(t reflect.Type) bool {
	return t.Kind() == reflect.Map && t.ConvertibleTo(nullStringMapType)
}

// nullStringMapTarget recognizes a scan destination that is a non-nil pointer
// to a NullStringMap-shaped map and returns a function that stores a scanned
// row into it.
func nullStringMapTarget(dest any) (func(NullStringMap), bool) {
	if p, ok := dest.(*NullStringMap); ok && p != nil {
		return func(m NullStringMap) { *p = m }, true
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || !isNullStringMapType(rv.Elem().Type()) {
		return nil, false
	}
	elem := rv.Elem()
	return func(m NullStringMap) { elem.Set(reflect.ValueOf(m).Convert(elem.Type())) }, true
}

// nullStringMapSliceTarget recognizes a scan destination that is a non-nil
// pointer to a slice of NullStringMap-shaped maps and returns a function that
// appends the scanned rows to it (All appends, as it always has for maps).
func nullStringMapSliceTarget(dest any) (func([]NullStringMap), bool) {
	if p, ok := dest.(*[]NullStringMap); ok && p != nil {
		return func(ms []NullStringMap) { *p = append(*p, ms...) }, true
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, false
	}
	sliceType := rv.Elem().Type()
	if sliceType.Kind() != reflect.Slice || !isNullStringMapType(sliceType.Elem()) {
		return nil, false
	}
	elem := rv.Elem()
	return func(ms []NullStringMap) {
		out := elem
		for _, m := range ms {
			out = reflect.Append(out, reflect.ValueOf(m).Convert(sliceType.Elem()))
		}
		elem.Set(out)
	}, true
}

// scanOne scans the current row into dest: a NullStringMap-shaped map (any
// named type) or anything the struct scanner accepts.
func scanOne(rows *sql.Rows, dest any, s scanSettings) error {
	store, ok := nullStringMapTarget(dest)
	if !ok {
		return s.scanRow(rows, dest)
	}
	var m NullStringMap
	if err := scanMapRow(rows, &m); err != nil {
		return err
	}
	store(m)
	return nil
}

// scanAll scans all remaining rows into dest: a slice of NullStringMap-shaped
// maps or anything the struct scanner accepts.
func scanAll(rows *sql.Rows, dest any, s scanSettings) error {
	store, ok := nullStringMapSliceTarget(dest)
	if !ok {
		return s.scanRows(rows, dest)
	}
	var ms []NullStringMap
	if err := scanMapRows(rows, &ms); err != nil {
		return err
	}
	store(ms)
	return nil
}
