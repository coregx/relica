package core

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// ScanMode selects how strictly result columns must match struct fields when
// scanning into a struct (ADR-018).
type ScanMode int

const (
	// ScanRequireFields (default): a mapped struct field without a result column
	// is an error, unless the query lists its columns (Select("id", "name")) —
	// then every listed column must have a field instead. Extra columns of an
	// unlisted result are ignored; two result columns for one field are an error.
	ScanRequireFields ScanMode = iota
	// ScanLenient is ozzo-dbx's behavior: unmatched fields keep their value,
	// unmatched columns are discarded, a repeated column overwrites, no error.
	ScanLenient
	// ScanStrict is ScanRequireFields plus an error for any result column that
	// matches no field, listed or not.
	ScanStrict
)

// WithScanMode sets the DB's default scan mode; SelectQuery.ScanMode and
// Query.ScanMode override it per query.
func WithScanMode(m ScanMode) Option {
	return func(db *DB) {
		db.scanMode = m
	}
}

// scanSettings carries what struct scanning needs from the query.
type scanSettings struct {
	db     *DB
	mode   ScanMode
	listed bool // the SELECT lists its columns: fields outside the list are not required
}

func (q *Query) scanSettings() scanSettings {
	mode := q.db.scanMode
	if q.scanMode != nil {
		mode = *q.scanMode
	}
	return scanSettings{db: q.db, mode: mode, listed: q.listedColumns}
}

// plan maps every result column to a field of struct type t (nil = discard).
func (s scanSettings) plan(t reflect.Type, columns []string) ([]*modelField, error) {
	mi, err := s.db.modelInfoOf(t)
	if err != nil {
		return nil, err
	}
	plan := make([]*modelField, len(columns))
	matched := make(map[*modelField]bool, len(columns))
	for i, col := range columns {
		key := strings.ToLower(col)
		// A qualified result name ("src.qualified_name") matches its last part.
		if dot := strings.LastIndex(key, "."); dot >= 0 {
			key = key[dot+1:]
		}
		if f, ok := mi.byColumn[key]; ok {
			if matched[f] && s.mode != ScanLenient {
				// Two result columns for one field: the later scan overwrote the
				// earlier one without a word (Select("a.id", "b.id"), or SELECT *
				// over a join where both tables have id).
				return nil, fmt.Errorf("relica: scanning into %s: result column %q (field %s) appears more than once; "+
					"alias one of them (AS) or drop it", t, col, f.field.Name)
			}
			plan[i] = f
			matched[f] = true
			continue
		}
		if s.mode == ScanStrict || (s.mode == ScanRequireFields && s.listed) {
			// The code named this column (Select("id", "email")) and no field
			// takes it: it would be read and dropped. Select() of a wider table,
			// or Select("t.*"), lists nothing, so its extra columns stay ignored.
			return nil, fmt.Errorf("relica: scanning into %s: result column %q matches no field "+
				"(add a field, drop the column from the query, or use ScanLenient)", t, col)
		}
	}
	if s.mode != ScanLenient && !s.listed {
		for _, f := range mi.fields {
			if !matched[f] {
				return nil, fmt.Errorf("relica: scanning into %s: field %s expects column %q, which the result "+
					"does not have (ScanRequireFields: tag the field db:\"-\", list the columns in Select, or use ScanLenient)",
					t, f.field.Name, f.column)
			}
		}
	}
	return plan, nil
}

// scanInto scans the current row into struct value v using plan.
func scanInto(rows *sql.Rows, v reflect.Value, plan []*modelField) error {
	dests := make([]any, len(plan))
	for i, f := range plan {
		if f == nil {
			var discard any
			dests[i] = &discard
			continue
		}
		dests[i] = v.FieldByIndex(f.index).Addr().Interface()
	}
	if err := rows.Scan(dests...); err != nil {
		return fmt.Errorf("scanner: scan failed: %w", err)
	}
	return nil
}

// checkRowType rejects a single-value type (time.Time, sql.Scanner,
// driver.Valuer) as a row: walked as a struct it matched no column and came
// back zero with no error.
func checkRowType(t reflect.Type) error {
	if isValueType(t) {
		return fmt.Errorf("relica: %s is a single value, not a row: use Row(&v) or Column(&[]v)", t)
	}
	return nil
}

// rowDest validates a One destination and returns the struct it points to.
func rowDest(dest any) (reflect.Value, error) {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Pointer {
		return reflect.Value{}, fmt.Errorf("scanner: dest must be pointer to struct, got %T", dest)
	}
	if destValue.IsNil() {
		return reflect.Value{}, fmt.Errorf("scanner: dest is a nil pointer, pass a non-nil *%s", destValue.Type().Elem())
	}
	destValue = destValue.Elem()
	if destValue.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("scanner: dest must be pointer to struct, got pointer to %s", destValue.Kind())
	}
	if err := checkRowType(destValue.Type()); err != nil {
		return reflect.Value{}, err
	}
	return destValue, nil
}

// checkOneDest validates a One destination before the query runs, so a wrong
// destination is reported even when the result is empty (it was reported as
// ErrNotFound then, and only showed once a row existed).
func (s scanSettings) checkOneDest(dest any) error {
	if _, ok := nullStringMapTarget(dest); ok {
		return nil
	}
	v, err := rowDest(dest)
	if err != nil {
		return err
	}
	_, err = s.db.modelInfoOf(v.Type())
	return err
}

// scanRow scans a single SQL row into dest struct.
func (s scanSettings) scanRow(rows *sql.Rows, dest any) error {
	destValue, err := rowDest(dest)
	if err != nil {
		return err
	}

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}
	plan, err := s.plan(destValue.Type(), columns)
	if err != nil {
		return err
	}
	return scanInto(rows, destValue, plan)
}

// scanRows scans multiple SQL rows into dest slice.
func (s scanSettings) scanRows(rows *sql.Rows, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Pointer {
		return fmt.Errorf("scanner: dest must be pointer to slice, got %T", dest)
	}
	sliceValue := destValue.Elem()
	if sliceValue.Kind() != reflect.Slice {
		return fmt.Errorf("scanner: dest must be pointer to slice, got pointer to %s", sliceValue.Kind())
	}

	elemType := sliceValue.Type().Elem()
	isPtr := elemType.Kind() == reflect.Pointer
	if isPtr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		return fmt.Errorf("scanner: slice element must be struct or *struct, got %s", elemType.Kind())
	}
	if err := checkRowType(elemType); err != nil {
		return err
	}

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}
	// Checked before the first row: a mismatch is a programming error even
	// when the result is empty.
	plan, err := s.plan(elemType, columns)
	if err != nil {
		return err
	}

	for rows.Next() {
		elemValue := reflect.New(elemType).Elem()
		if err := scanInto(rows, elemValue, plan); err != nil {
			return err
		}
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
func scanMapRow(rows *sql.Rows, dest *NullStringMap) error {
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}

	values := make([]sql.NullString, len(columns))
	scanDests := make([]any, len(columns))
	for i := range values {
		scanDests[i] = &values[i]
	}

	if err := rows.Scan(scanDests...); err != nil {
		return fmt.Errorf("scanner: scan failed: %w", err)
	}

	*dest = make(NullStringMap, len(columns))
	for i, col := range columns {
		(*dest)[col] = values[i]
	}
	return nil
}

// scanMapRows scans multiple SQL rows into a slice of NullStringMap.
func scanMapRows(rows *sql.Rows, dest *[]NullStringMap) error {
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("scanner: failed to get columns: %w", err)
	}

	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		scanDests := make([]any, len(columns))
		for i := range values {
			scanDests[i] = &values[i]
		}

		if err := rows.Scan(scanDests...); err != nil {
			return fmt.Errorf("scanner: scan failed: %w", err)
		}

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
