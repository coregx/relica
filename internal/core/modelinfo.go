package core

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/coregx/relica/internal/util"
)

// maxIdentifierBytes is the longest derived name Relica accepts: PostgreSQL
// silently truncates identifiers to 63 bytes (and can then collide), MySQL
// rejects more than 64.
const maxIdentifierBytes = 63

// WithNaming sets how table and column names are derived when the Go code
// gives none (ADR-018). The zero Naming is the default Snake().
func WithNaming(n Naming) Option {
	return func(db *DB) {
		db.naming = n
		// Names derived with the previous Naming must not be reused.
		db.models = &modelCache{}
	}
}

// modelField is one column of a model: a field reached through Index from the
// model type (embedded structs flattened).
type modelField struct {
	index  []int
	column string
	field  reflect.StructField
	tag    util.DBTagInfo
	pk     bool
}

// modelInfo is the one description of a struct used for writing, reading and
// the primary key (ADR-018 "one struct-info builder").
type modelInfo struct {
	typ       reflect.Type
	fields    []*modelField          // declaration order, embedded fields in place
	byColumn  map[string]*modelField // lower-case column → field
	pk        []*modelField          // declaration order
	pkAutoInc bool
}

type modelEntry struct {
	info *modelInfo
	err  error
}

// modelCache maps reflect.Type → *modelEntry for one DB. It is shared by the
// copies WithContext makes, by transactions and by Tx builders.
type modelCache struct {
	m sync.Map
}

// modelInfoOf returns the description of struct type t under db's Naming.
func (db *DB) modelInfoOf(t reflect.Type) (*modelInfo, error) {
	if db.models != nil {
		if e, ok := db.models.m.Load(t); ok {
			entry := e.(*modelEntry)
			return entry.info, entry.err
		}
	}
	info, err := buildModelInfo(t, db.naming)
	if db.models != nil {
		db.models.m.Store(t, &modelEntry{info: info, err: err})
	}
	return info, err
}

func buildModelInfo(t reflect.Type, naming Naming) (*modelInfo, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("relica: expected a struct, got %s", t)
	}
	mi := &modelInfo{typ: t, byColumn: make(map[string]*modelField)}
	if err := mi.walk(t, nil, naming); err != nil {
		return nil, err
	}
	mi.resolvePK()
	return mi, nil
}

func (mi *modelInfo) walk(owner reflect.Type, prefix []int, naming Naming) error {
	for i := 0; i < owner.NumField(); i++ {
		f := owner.Field(i)
		if !f.IsExported() && !isEmbeddedStruct(f) {
			continue
		}
		index := append(append([]int{}, prefix...), i)
		tagText, hasTag := f.Tag.Lookup("db")
		tag := util.ParseDBTagFull(tagText)
		if hasTag && tag.Column == "-" {
			continue
		}
		flatten, err := mi.checkEmbedded(f, hasTag, tag, tagText)
		if err != nil {
			return err
		}
		if flatten {
			err = mi.walk(f.Type, index, naming)
		} else {
			err = mi.add(owner, f, index, tag, naming)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// isEmbeddedStruct: as encoding/json, an embedded struct (or pointer to one)
// of an unexported type still promotes its exported fields.
func isEmbeddedStruct(f reflect.StructField) bool {
	t := f.Type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return f.Anonymous && t.Kind() == reflect.Struct
}

// checkEmbedded reports whether f is an embedded struct to flatten, and
// rejects the embedded forms Relica cannot write and read consistently.
func (mi *modelInfo) checkEmbedded(f reflect.StructField, hasTag bool, tag util.DBTagInfo, tagText string) (bool, error) {
	if !isEmbeddedStruct(f) {
		return false, nil
	}
	if f.Type.Kind() == reflect.Pointer {
		return false, fmt.Errorf("relica: %s embeds pointer %s: embedded pointers are not supported, embed the struct", mi.typ, f.Type)
	}
	if hasTag && tag.Column != "" {
		return false, fmt.Errorf("relica: %s: db tag on an embedded struct (%s `db:%q`) is not supported; tag its fields", mi.typ, f.Name, tagText)
	}
	return true, nil
}

func (mi *modelInfo) add(owner reflect.Type, f reflect.StructField, index []int, tag util.DBTagInfo, naming Naming) error {
	column := tag.Column
	if column == "" || column == "pk" { // legacy db:"pk" = "primary key, column from Naming"
		column = naming.column(ColumnSource{Model: mi.typ, Owner: owner, Field: f, Index: index})
		if column == "" {
			return fmt.Errorf(`relica: Naming.Column returned "" for field %s.%s: add a db tag`, mi.typ.Name(), f.Name)
		}
		if len(column) > maxIdentifierBytes {
			return fmt.Errorf("relica: derived column %q for field %s.%s is longer than %d bytes: add a db tag",
				column, mi.typ.Name(), f.Name, maxIdentifierBytes)
		}
	}
	key := strings.ToLower(column)
	if prev, ok := mi.byColumn[key]; ok {
		return fmt.Errorf("relica: %s: fields %s and %s both map to column %q (names are compared case-insensitively)",
			mi.typ.Name(), prev.field.Name, f.Name, prev.column)
	}
	mf := &modelField{index: index, column: column, field: f, tag: tag, pk: tag.IsPK}
	mi.fields = append(mi.fields, mf)
	mi.byColumn[key] = mf
	return nil
}

// resolvePK: fields tagged pk (db:"col,pk", db:",pk", legacy db:"pk") in
// declaration order; otherwise the first field named ID, then Id.
func (mi *modelInfo) resolvePK() {
	for _, f := range mi.fields {
		if f.pk {
			mi.pk = append(mi.pk, f)
		}
	}
	if len(mi.pk) == 0 {
		for _, name := range []string{"ID", "Id"} {
			for _, f := range mi.fields {
				if f.field.Name == name {
					mi.pk = []*modelField{f}
					break
				}
			}
			if len(mi.pk) > 0 {
				break
			}
		}
	}
	mi.pkAutoInc = len(mi.pk) == 1 && mi.pk[0].tag.AutoIncrement
}

// structValue dereferences model to its struct value.
func structValue(model any) (reflect.Value, error) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}, errors.New("relica: model is a nil pointer")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("relica: expected a struct or *struct, got %T", model)
	}
	return v, nil
}

// structValues returns column → value for every column of the model.
func (db *DB) structValues(model any) (map[string]any, error) {
	v, err := structValue(model)
	if err != nil {
		return nil, err
	}
	mi, err := db.modelInfoOf(v.Type())
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(mi.fields))
	for _, f := range mi.fields {
		out[f.column] = v.FieldByIndex(f.index).Interface()
	}
	return out, nil
}

// structValuesExceptPK is structValues without the primary-key columns
// (including the ID/Id fallback): UpdateStruct never writes the key, as
// Model().Update() does not.
func (db *DB) structValuesExceptPK(model any) (map[string]any, error) {
	v, err := structValue(model)
	if err != nil {
		return nil, err
	}
	mi, err := db.modelInfoOf(v.Type())
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(mi.fields))
	for _, f := range mi.fields {
		if !isPKField(mi, f) {
			out[f.column] = v.FieldByIndex(f.index).Interface()
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("relica: UpdateStruct: %s has no column besides its primary key", v.Type())
	}
	return out, nil
}

// primaryKey describes the primary key of struct value v (not a pointer).
func (db *DB) primaryKey(v reflect.Value) (*util.PrimaryKeyInfo, error) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, errors.New("relica: model is a nil pointer")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("relica: expected a struct, got %s", v.Kind())
	}
	mi, err := db.modelInfoOf(v.Type())
	if err != nil {
		return nil, err
	}
	if len(mi.pk) == 0 {
		return nil, fmt.Errorf("relica: %s has no primary key (tag a field db:\"col,pk\" or name it ID)", v.Type().Name())
	}
	info := &util.PrimaryKeyInfo{AutoIncrement: mi.pkAutoInc}
	for _, f := range mi.pk {
		info.Fields = append(info.Fields, f.field)
		info.Values = append(info.Values, v.FieldByIndex(f.index))
		info.Columns = append(info.Columns, f.column)
	}
	return info, nil
}

// autoIDField is a field tagged autoid.
type autoIDField struct {
	index     []int
	column    string
	prefix    string
	generator string
}

func (db *DB) autoIDFields(t reflect.Type) ([]autoIDField, error) {
	mi, err := db.modelInfoOf(t)
	if err != nil {
		return nil, err
	}
	var out []autoIDField
	for _, f := range mi.fields {
		if f.tag.IsAutoID {
			out = append(out, autoIDField{index: f.index, column: f.column, prefix: f.tag.AutoIDPrefix, generator: f.tag.AutoIDGen})
		}
	}
	return out, nil
}

// --- table names ------------------------------------------------------------

type tableNamer interface{ TableName() string }

// elementType strips pointers, slices and arrays: *[]*T → T.
func elementType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	return t
}

// tableNameMethod returns the element type of model and, if T or *T has a
// TableName method, its result. On a non-nil *T the method runs on that value,
// so an instance-dependent name (tenant tables) works; otherwise on a zero T or
// a new *T.
func tableNameMethod(model any) (reflect.Type, string, bool) {
	if v := reflect.ValueOf(model); v.Kind() == reflect.Pointer && !v.IsNil() && v.Elem().Kind() == reflect.Struct {
		if tn, ok := model.(tableNamer); ok {
			return v.Elem().Type(), tn.TableName(), true
		}
	}
	t := elementType(reflect.TypeOf(model))
	if t.Kind() != reflect.Struct {
		return t, "", false
	}
	switch {
	case t.Implements(tableNamerType):
		return t, reflect.Zero(t).Interface().(tableNamer).TableName(), true
	case reflect.PointerTo(t).Implements(tableNamerType):
		return t, reflect.New(t).Interface().(tableNamer).TableName(), true
	}
	return t, "", false
}

var tableNamerType = reflect.TypeOf((*tableNamer)(nil)).Elem()

// tableNameOf returns the table of a model: TableName() on T or *T, else
// Naming.Table. model may be T, *T, []T, []*T or a pointer to those.
// Precedence: an explicit Table("x") on the query > TableName() > Naming.Table.
func (db *DB) tableNameOf(model any) (string, error) {
	if model == nil {
		return "", errors.New("relica: model is nil")
	}
	t, name, ok := tableNameMethod(model)
	if ok {
		return name, nil
	}
	if t.Kind() != reflect.Struct {
		return "", fmt.Errorf("relica: cannot derive a table name from %T", model)
	}
	name = db.naming.table(TableSource{Type: t})
	if name == "" {
		return "", fmt.Errorf(`relica: Naming.Table returned "" for %s: add a TableName() method`, t.Name())
	}
	if len(name) > maxIdentifierBytes {
		return "", fmt.Errorf("relica: derived table %q for %s is longer than %d bytes: add a TableName() method",
			name, t.Name(), maxIdentifierBytes)
	}
	return name, nil
}
