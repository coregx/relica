package core

import (
	"database/sql"
	"reflect"
	"testing"
)

// Package relica declares its own Params and NullStringMap (ADR-016). They are
// distinct named types with the same underlying types as core's, so core must
// recognize them by structure, not by identity. These local types stand in for
// the public ones.
type (
	extParams        map[string]any
	extNullStringMap map[string]sql.NullString
)

func TestAsParams(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want Params
		ok   bool
	}{
		{"core.Params", Params{"id": 1}, Params{"id": 1}, true},
		{"foreign named map", extParams{"id": 2}, Params{"id": 2}, true},
		{"plain map", map[string]any{"id": 3}, Params{"id": 3}, true},
		{"nil foreign map", extParams(nil), Params(nil), true},
		{"wrong value type", map[string]int{"id": 4}, nil, false},
		{"wrong key type", map[int]any{1: 5}, nil, false},
		{"scalar", 6, nil, false},
		{"nil", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asParams(tt.v)
			if ok != tt.ok || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("asParams(%#v) = %#v, %v; want %#v, %v", tt.v, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestResolveNamedParams_ForeignParamsType(t *testing.T) {
	sql, args, err := resolveNamedParams("a = {:a} AND b = {:b}", []any{extParams{"a": 1, "b": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if sql != "a = ? AND b = ?" || !reflect.DeepEqual(args, []any{1, "x"}) {
		t.Errorf("got %q %v", sql, args)
	}
}

func TestNullStringMapTargets(t *testing.T) {
	row := NullStringMap{"a": {String: "1", Valid: true}}

	t.Run("core pointer", func(t *testing.T) {
		var m NullStringMap
		set, ok := nullStringMapTarget(&m)
		if !ok {
			t.Fatal("not recognized")
		}
		set(row)
		if m["a"].String != "1" {
			t.Errorf("m = %v", m)
		}
	})
	t.Run("foreign pointer", func(t *testing.T) {
		var m extNullStringMap
		set, ok := nullStringMapTarget(&m)
		if !ok {
			t.Fatal("not recognized")
		}
		set(row)
		if m["a"].String != "1" {
			t.Errorf("m = %v", m)
		}
	})
	t.Run("not a target", func(t *testing.T) {
		for _, dest := range []any{nil, NullStringMap{}, &map[string]string{}, new(int), (*extNullStringMap)(nil)} {
			if _, ok := nullStringMapTarget(dest); ok {
				t.Errorf("%T recognized as NullStringMap target", dest)
			}
		}
	})

	rows := []NullStringMap{row, {"a": {}}}
	t.Run("core slice pointer", func(t *testing.T) {
		var ms []NullStringMap
		set, ok := nullStringMapSliceTarget(&ms)
		if !ok {
			t.Fatal("not recognized")
		}
		set(rows)
		if len(ms) != 2 || ms[0]["a"].String != "1" || ms[1]["a"].Valid {
			t.Errorf("ms = %v", ms)
		}
	})
	t.Run("foreign slice pointer", func(t *testing.T) {
		var ms []extNullStringMap
		set, ok := nullStringMapSliceTarget(&ms)
		if !ok {
			t.Fatal("not recognized")
		}
		set(rows)
		if len(ms) != 2 || ms[0]["a"].String != "1" || ms[1]["a"].Valid {
			t.Errorf("ms = %v", ms)
		}
	})
	t.Run("appends to existing rows", func(t *testing.T) {
		existing := NullStringMap{"a": {String: "0", Valid: true}}
		core := []NullStringMap{existing}
		foreign := []extNullStringMap{extNullStringMap(existing)}
		setCore, _ := nullStringMapSliceTarget(&core)
		setForeign, _ := nullStringMapSliceTarget(&foreign)
		setCore(rows)
		setForeign(rows)
		if len(core) != 3 || core[0]["a"].String != "0" || core[1]["a"].String != "1" {
			t.Errorf("core = %v", core)
		}
		if len(foreign) != 3 || foreign[0]["a"].String != "0" || foreign[1]["a"].String != "1" {
			t.Errorf("foreign = %v", foreign)
		}
	})
	t.Run("not a slice target", func(t *testing.T) {
		for _, dest := range []any{nil, []NullStringMap{}, &[]map[string]string{}, &[]int{}, (*[]extNullStringMap)(nil)} {
			if _, ok := nullStringMapSliceTarget(dest); ok {
				t.Errorf("%T recognized as []NullStringMap target", dest)
			}
		}
	})
}
