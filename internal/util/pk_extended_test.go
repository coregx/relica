package util

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── parseDBTag ────────────────────────────────────────────────────────────────

// TestParseDBTag covers all tag format variants for parseDBTag (internal function,
// tested via the exported API that exercises every branch).
// Since parseDBTag is unexported we call it directly within the same package.
func TestParseDBTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		wantCol  string
		wantIsPK bool
	}{
		{
			name:     "plain column name",
			tag:      "user_id",
			wantCol:  "user_id",
			wantIsPK: false,
		},
		{
			name:     "legacy pk tag",
			tag:      "pk",
			wantCol:  "pk",
			wantIsPK: true,
		},
		{
			name:     "column with pk option",
			tag:      "id,pk",
			wantCol:  "id",
			wantIsPK: true,
		},
		{
			name:     "skip tag dash",
			tag:      "-",
			wantCol:  "-",
			wantIsPK: false,
		},
		{
			name:     "empty tag",
			tag:      "",
			wantCol:  "",
			wantIsPK: false,
		},
		{
			name:     "column with spaces in pk option",
			tag:      "order_id, pk",
			wantCol:  "order_id",
			wantIsPK: true,
		},
		{
			name:     "column with extra options besides pk",
			tag:      "name,omitempty,pk",
			wantCol:  "name",
			wantIsPK: true,
		},
		{
			name:     "column with omitempty only",
			tag:      "name,omitempty",
			wantCol:  "name",
			wantIsPK: false,
		},
		{
			name:     "column name with leading/trailing spaces",
			tag:      " email ",
			wantCol:  "email",
			wantIsPK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, isPK := parseDBTag(tt.tag)
			assert.Equal(t, tt.wantCol, col)
			assert.Equal(t, tt.wantIsPK, isPK)
		})
	}
}

// ─── IsPrimaryKeyZero ──────────────────────────────────────────────────────────

// TestIsPrimaryKeyZero_AllTypes covers all supported kinds plus edge cases.
func TestIsPrimaryKeyZero_AllTypes(t *testing.T) {
	tests := []struct {
		name  string
		value reflect.Value
		want  bool
	}{
		// int family — zero
		{"int zero", reflect.ValueOf(int(0)), true},
		{"int8 zero", reflect.ValueOf(int8(0)), true},
		{"int16 zero", reflect.ValueOf(int16(0)), true},
		{"int32 zero", reflect.ValueOf(int32(0)), true},
		{"int64 zero", reflect.ValueOf(int64(0)), true},
		// int family — non-zero
		{"int non-zero", reflect.ValueOf(int(1)), false},
		{"int8 non-zero", reflect.ValueOf(int8(1)), false},
		{"int16 non-zero", reflect.ValueOf(int16(1)), false},
		{"int32 non-zero", reflect.ValueOf(int32(1)), false},
		{"int64 non-zero", reflect.ValueOf(int64(1)), false},
		// uint family — zero
		{"uint zero", reflect.ValueOf(uint(0)), true},
		{"uint8 zero", reflect.ValueOf(uint8(0)), true},
		{"uint16 zero", reflect.ValueOf(uint16(0)), true},
		{"uint32 zero", reflect.ValueOf(uint32(0)), true},
		{"uint64 zero", reflect.ValueOf(uint64(0)), true},
		// uint family — non-zero
		{"uint non-zero", reflect.ValueOf(uint(5)), false},
		{"uint8 non-zero", reflect.ValueOf(uint8(5)), false},
		{"uint16 non-zero", reflect.ValueOf(uint16(5)), false},
		{"uint32 non-zero", reflect.ValueOf(uint32(5)), false},
		{"uint64 non-zero", reflect.ValueOf(uint64(5)), false},
		// string: always false regardless of value
		{"string empty — not zero", reflect.ValueOf(""), false},
		{"string non-empty — not zero", reflect.ValueOf("abc"), false},
		// bool: always false
		{"bool false — not zero", reflect.ValueOf(false), false},
		{"bool true — not zero", reflect.ValueOf(true), false},
		// invalid value
		{"invalid reflect.Value", reflect.Value{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPrimaryKeyZero(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIsPrimaryKeyZero_Pointer tests pointer dereference logic.
func TestIsPrimaryKeyZero_Pointer(t *testing.T) {
	t.Run("nil pointer is zero", func(t *testing.T) {
		var p *int64
		assert.True(t, IsPrimaryKeyZero(reflect.ValueOf(p)))
	})

	t.Run("pointer to zero int64 is zero", func(t *testing.T) {
		v := int64(0)
		assert.True(t, IsPrimaryKeyZero(reflect.ValueOf(&v)))
	})

	t.Run("pointer to non-zero int64 is not zero", func(t *testing.T) {
		v := int64(42)
		assert.False(t, IsPrimaryKeyZero(reflect.ValueOf(&v)))
	})

	t.Run("pointer to zero int is zero", func(t *testing.T) {
		v := int(0)
		assert.True(t, IsPrimaryKeyZero(reflect.ValueOf(&v)))
	})

	t.Run("pointer to non-zero uint is not zero", func(t *testing.T) {
		v := uint(7)
		assert.False(t, IsPrimaryKeyZero(reflect.ValueOf(&v)))
	})
}

// ─── SetPrimaryKeyValue ────────────────────────────────────────────────────────

// TestSetPrimaryKeyValue_AllIntTypes covers every signed integer variant.
func TestSetPrimaryKeyValue_AllIntTypes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() reflect.Value
		id      int64
		wantErr bool
	}{
		{
			name:  "int",
			setup: func() reflect.Value { var v int; return reflect.ValueOf(&v).Elem() },
			id:    100,
		},
		{
			name:  "int8 valid",
			setup: func() reflect.Value { var v int8; return reflect.ValueOf(&v).Elem() },
			id:    127,
		},
		{
			name:    "int8 overflow",
			setup:   func() reflect.Value { var v int8; return reflect.ValueOf(&v).Elem() },
			id:      128,
			wantErr: true,
		},
		{
			name:    "int8 underflow",
			setup:   func() reflect.Value { var v int8; return reflect.ValueOf(&v).Elem() },
			id:      -129,
			wantErr: true,
		},
		{
			name:  "int16 valid",
			setup: func() reflect.Value { var v int16; return reflect.ValueOf(&v).Elem() },
			id:    32767,
		},
		{
			name:    "int16 overflow",
			setup:   func() reflect.Value { var v int16; return reflect.ValueOf(&v).Elem() },
			id:      32768,
			wantErr: true,
		},
		{
			name:    "int16 underflow",
			setup:   func() reflect.Value { var v int16; return reflect.ValueOf(&v).Elem() },
			id:      -32769,
			wantErr: true,
		},
		{
			name:  "int32 valid",
			setup: func() reflect.Value { var v int32; return reflect.ValueOf(&v).Elem() },
			id:    2147483647,
		},
		{
			name:    "int32 overflow",
			setup:   func() reflect.Value { var v int32; return reflect.ValueOf(&v).Elem() },
			id:      2147483648,
			wantErr: true,
		},
		{
			name:    "int32 underflow",
			setup:   func() reflect.Value { var v int32; return reflect.ValueOf(&v).Elem() },
			id:      -2147483649,
			wantErr: true,
		},
		{
			name:  "int64 valid",
			setup: func() reflect.Value { var v int64; return reflect.ValueOf(&v).Elem() },
			id:    9223372036854775807,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := tt.setup()
			err := SetPrimaryKeyValue(field, tt.id)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.id, field.Int())
			}
		})
	}
}

// TestSetPrimaryKeyValue_AllUintTypes covers every unsigned integer variant.
func TestSetPrimaryKeyValue_AllUintTypes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() reflect.Value
		id      int64
		wantErr bool
	}{
		{
			name:  "uint valid",
			setup: func() reflect.Value { var v uint; return reflect.ValueOf(&v).Elem() },
			id:    100,
		},
		{
			name:    "uint negative",
			setup:   func() reflect.Value { var v uint; return reflect.ValueOf(&v).Elem() },
			id:      -1,
			wantErr: true,
		},
		{
			name:  "uint8 valid",
			setup: func() reflect.Value { var v uint8; return reflect.ValueOf(&v).Elem() },
			id:    255,
		},
		{
			name:    "uint8 overflow",
			setup:   func() reflect.Value { var v uint8; return reflect.ValueOf(&v).Elem() },
			id:      256,
			wantErr: true,
		},
		{
			name:    "uint8 negative",
			setup:   func() reflect.Value { var v uint8; return reflect.ValueOf(&v).Elem() },
			id:      -1,
			wantErr: true,
		},
		{
			name:  "uint16 valid",
			setup: func() reflect.Value { var v uint16; return reflect.ValueOf(&v).Elem() },
			id:    65535,
		},
		{
			name:    "uint16 overflow",
			setup:   func() reflect.Value { var v uint16; return reflect.ValueOf(&v).Elem() },
			id:      65536,
			wantErr: true,
		},
		{
			name:    "uint16 negative",
			setup:   func() reflect.Value { var v uint16; return reflect.ValueOf(&v).Elem() },
			id:      -1,
			wantErr: true,
		},
		{
			name:  "uint32 valid",
			setup: func() reflect.Value { var v uint32; return reflect.ValueOf(&v).Elem() },
			id:    4294967295,
		},
		{
			name:    "uint32 overflow",
			setup:   func() reflect.Value { var v uint32; return reflect.ValueOf(&v).Elem() },
			id:      4294967296,
			wantErr: true,
		},
		{
			name:    "uint32 negative",
			setup:   func() reflect.Value { var v uint32; return reflect.ValueOf(&v).Elem() },
			id:      -1,
			wantErr: true,
		},
		{
			name:  "uint64 valid",
			setup: func() reflect.Value { var v uint64; return reflect.ValueOf(&v).Elem() },
			id:    1000000,
		},
		{
			name:    "uint64 negative",
			setup:   func() reflect.Value { var v uint64; return reflect.ValueOf(&v).Elem() },
			id:      -1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := tt.setup()
			err := SetPrimaryKeyValue(field, tt.id)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, uint64(tt.id), field.Uint())
			}
		})
	}
}

// TestSetPrimaryKeyValue_PointerField tests pointer field allocation and setting.
func TestSetPrimaryKeyValue_PointerField(t *testing.T) {
	t.Run("nil pointer to int64 is allocated and set", func(t *testing.T) {
		var p *int64
		field := reflect.ValueOf(&p).Elem()
		require.NoError(t, SetPrimaryKeyValue(field, 42))
		require.NotNil(t, p)
		assert.Equal(t, int64(42), *p)
	})

	t.Run("nil pointer to int32 is allocated and set", func(t *testing.T) {
		var p *int32
		field := reflect.ValueOf(&p).Elem()
		require.NoError(t, SetPrimaryKeyValue(field, 7))
		require.NotNil(t, p)
		assert.Equal(t, int32(7), *p)
	})

	t.Run("existing non-nil pointer is overwritten", func(t *testing.T) {
		existing := int64(100)
		p := &existing
		field := reflect.ValueOf(&p).Elem()
		require.NoError(t, SetPrimaryKeyValue(field, 999))
		assert.Equal(t, int64(999), *p)
	})
}

// TestSetPrimaryKeyValue_ErrorCases tests all error paths.
func TestSetPrimaryKeyValue_ErrorCases(t *testing.T) {
	t.Run("invalid reflect.Value returns error", func(t *testing.T) {
		err := SetPrimaryKeyValue(reflect.Value{}, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid field")
	})

	t.Run("non-settable field returns error", func(t *testing.T) {
		type S struct{ ID int64 }
		s := S{}
		// Field obtained from non-pointer value is not settable.
		field := reflect.ValueOf(s).Field(0)
		err := SetPrimaryKeyValue(field, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not settable")
	})

	t.Run("unsupported type string returns error", func(t *testing.T) {
		var s string
		field := reflect.ValueOf(&s).Elem()
		err := SetPrimaryKeyValue(field, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported type")
	})

	t.Run("unsupported type float64 returns error", func(t *testing.T) {
		var f float64
		field := reflect.ValueOf(&f).Elem()
		err := SetPrimaryKeyValue(field, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported type")
	})

	t.Run("unsupported type bool returns error", func(t *testing.T) {
		var b bool
		field := reflect.ValueOf(&b).Elem()
		err := SetPrimaryKeyValue(field, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported type")
	})
}

// ─── FindPrimaryKeyFields ──────────────────────────────────────────────────────

// TestFindPrimaryKeyFields_AllPriorities tests the priority chain described in
// the function's godoc.
func TestFindPrimaryKeyFields_AllPriorities(t *testing.T) {
	t.Run("explicit db:pk tag (legacy)", func(t *testing.T) {
		type Article struct {
			ID   int    `db:"pk"`
			Body string `db:"body"`
		}
		a := Article{ID: 5}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(a))
		require.NoError(t, err)
		assert.Equal(t, []string{"id"}, info.Columns) // legacy: column = lowercase field name
		assert.Equal(t, int64(5), info.Values[0].Int())
	})

	t.Run("composite PK with db:col,pk syntax", func(t *testing.T) {
		type OrderItem struct {
			OrderID int `db:"order_id,pk"`
			ItemID  int `db:"item_id,pk"`
			Qty     int `db:"qty"`
		}
		oi := OrderItem{OrderID: 1, ItemID: 2, Qty: 3}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(oi))
		require.NoError(t, err)
		assert.True(t, info.IsComposite())
		assert.False(t, info.IsSingle())
		assert.Equal(t, []string{"order_id", "item_id"}, info.Columns)
		assert.Equal(t, int64(1), info.Values[0].Int())
		assert.Equal(t, int64(2), info.Values[1].Int())
	})

	t.Run("fallback to field named ID", func(t *testing.T) {
		type Product struct {
			ID   int64
			Name string
		}
		p := Product{ID: 99}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(p))
		require.NoError(t, err)
		assert.True(t, info.IsSingle())
		assert.Equal(t, []string{"id"}, info.Columns)
		assert.Equal(t, int64(99), info.Values[0].Int())
	})

	t.Run("fallback to field named Id", func(t *testing.T) {
		type Widget struct {
			Id   int
			Name string
		}
		w := Widget{Id: 77}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(w))
		require.NoError(t, err)
		assert.Equal(t, []string{"id"}, info.Columns)
		assert.Equal(t, int64(77), info.Values[0].Int())
	})

	t.Run("ID field with custom db tag uses tag column name", func(t *testing.T) {
		type Order struct {
			ID   int `db:"order_id"`
			Name string
		}
		o := Order{ID: 55}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(o))
		require.NoError(t, err)
		assert.Equal(t, []string{"order_id"}, info.Columns)
	})

	t.Run("skip field with db:-", func(t *testing.T) {
		type Ghost struct {
			Hidden int `db:"-"`
			ID     int64
		}
		g := Ghost{Hidden: 1, ID: 10}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(g))
		require.NoError(t, err)
		assert.Equal(t, []string{"id"}, info.Columns)
		assert.Equal(t, int64(10), info.Values[0].Int())
	})

	t.Run("no PK found returns error", func(t *testing.T) {
		type NoPK struct {
			Name  string `db:"name"`
			Email string `db:"email"`
		}
		_, err := FindPrimaryKeyFields(reflect.ValueOf(NoPK{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no primary key found")
	})

	t.Run("nil pointer returns error", func(t *testing.T) {
		type S struct{ ID int }
		var s *S
		_, err := FindPrimaryKeyFields(reflect.ValueOf(s))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil pointer")
	})

	t.Run("non-struct returns error", func(t *testing.T) {
		_, err := FindPrimaryKeyFields(reflect.ValueOf(42))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a struct")
	})

	t.Run("pointer to struct is dereferenced", func(t *testing.T) {
		type Thing struct {
			ID int64 `db:"id,pk"`
		}
		thing := &Thing{ID: 13}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(thing))
		require.NoError(t, err)
		assert.Equal(t, []string{"id"}, info.Columns)
		assert.Equal(t, int64(13), info.Values[0].Int())
	})

	t.Run("unexported fields are skipped", func(t *testing.T) {
		type HasUnexported struct {
			ID       int64  `db:"id"`
			secret   string //nolint:unused
			Name     string `db:"name"`
		}
		h := HasUnexported{ID: 21}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(h))
		require.NoError(t, err)
		// ID fallback: field name "ID" has db tag "id", so composite pkFields will
		// not include it (no ,pk), but idFieldIndex will be set.
		assert.Equal(t, []string{"id"}, info.Columns)
	})

	t.Run("composite PK fields returned in declaration order", func(t *testing.T) {
		// Composite fields should be ordered by struct declaration, not alphabet.
		type MultiPK struct {
			Z int `db:"z_col,pk"`
			A int `db:"a_col,pk"`
			M int `db:"m_col,pk"`
		}
		mp := MultiPK{Z: 1, A: 2, M: 3}
		info, err := FindPrimaryKeyFields(reflect.ValueOf(mp))
		require.NoError(t, err)
		assert.Equal(t, []string{"z_col", "a_col", "m_col"}, info.Columns)
	})
}

// ─── PrimaryKeyInfo.IsSingle / IsComposite ────────────────────────────────────

// TestPrimaryKeyInfo_IsSingle_IsComposite tests both methods exhaustively.
func TestPrimaryKeyInfo_IsSingle_IsComposite(t *testing.T) {
	t.Run("single column", func(t *testing.T) {
		pk := &PrimaryKeyInfo{Columns: []string{"id"}}
		assert.True(t, pk.IsSingle())
		assert.False(t, pk.IsComposite())
	})

	t.Run("two columns — composite", func(t *testing.T) {
		pk := &PrimaryKeyInfo{Columns: []string{"order_id", "item_id"}}
		assert.False(t, pk.IsSingle())
		assert.True(t, pk.IsComposite())
	})

	t.Run("three columns — composite", func(t *testing.T) {
		pk := &PrimaryKeyInfo{Columns: []string{"a", "b", "c"}}
		assert.False(t, pk.IsSingle())
		assert.True(t, pk.IsComposite())
	})

	t.Run("empty columns — neither single nor composite", func(t *testing.T) {
		pk := &PrimaryKeyInfo{Columns: []string{}}
		assert.False(t, pk.IsSingle())
		assert.False(t, pk.IsComposite())
	})
}

// ─── FindPrimaryKeyField ───────────────────────────────────────────────────────

// TestFindPrimaryKeyField_Extended covers composite PK error and all error paths.
func TestFindPrimaryKeyField_Extended(t *testing.T) {
	t.Run("composite PK returns error", func(t *testing.T) {
		type CPK struct {
			A int `db:"a_id,pk"`
			B int `db:"b_id,pk"`
		}
		c := CPK{A: 1, B: 2}
		_, _, err := FindPrimaryKeyField(reflect.ValueOf(c))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "composite primary keys not supported")
	})

	t.Run("no PK found propagates error", func(t *testing.T) {
		type NoPK struct {
			Name string `db:"name"`
		}
		_, _, err := FindPrimaryKeyField(reflect.ValueOf(NoPK{}))
		require.Error(t, err)
	})

	t.Run("nil pointer propagates error", func(t *testing.T) {
		type S struct{ ID int }
		var s *S
		_, _, err := FindPrimaryKeyField(reflect.ValueOf(s))
		require.Error(t, err)
	})

	t.Run("valid single PK by ID fallback", func(t *testing.T) {
		type Product struct {
			ID   int64
			Name string
		}
		p := Product{ID: 42}
		field, val, err := FindPrimaryKeyField(reflect.ValueOf(p))
		require.NoError(t, err)
		assert.Equal(t, "ID", field.Name)
		assert.Equal(t, int64(42), val.Int())
	})

	t.Run("valid single PK by db:pk tag", func(t *testing.T) {
		type Tag struct {
			MyID int `db:"pk"`
			Name string
		}
		tg := Tag{MyID: 7}
		field, val, err := FindPrimaryKeyField(reflect.ValueOf(tg))
		require.NoError(t, err)
		assert.Equal(t, "MyID", field.Name)
		assert.Equal(t, int64(7), val.Int())
	})
}

// ─── ModelToColumns ────────────────────────────────────────────────────────────

// TestModelToColumns covers all branches of ModelToColumns.
func TestModelToColumns(t *testing.T) {
	t.Run("basic struct with db tags", func(t *testing.T) {
		type User struct {
			ID   int    `db:"id"`
			Name string `db:"username"`
		}
		cols := ModelToColumns(User{})
		assert.Equal(t, map[string]string{"ID": "id", "Name": "username"}, cols)
	})

	t.Run("pointer to struct", func(t *testing.T) {
		type User struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}
		cols := ModelToColumns(&User{})
		assert.Equal(t, map[string]string{"ID": "id", "Name": "name"}, cols)
	})

	t.Run("fields without db tags are excluded", func(t *testing.T) {
		type Mixed struct {
			ID    int    `db:"id"`
			Plain string // no tag
		}
		cols := ModelToColumns(Mixed{})
		assert.Equal(t, map[string]string{"ID": "id"}, cols)
		_, hasPlain := cols["Plain"]
		assert.False(t, hasPlain)
	})

	t.Run("db:- fields are excluded", func(t *testing.T) {
		type WithSkip struct {
			ID      int    `db:"id"`
			Ignored string `db:"-"`
		}
		cols := ModelToColumns(WithSkip{})
		assert.Equal(t, map[string]string{"ID": "id"}, cols)
		_, hasIgnored := cols["Ignored"]
		assert.False(t, hasIgnored)
	})

	t.Run("composite PK tag — column extracted correctly", func(t *testing.T) {
		type CPK struct {
			OrderID int `db:"order_id,pk"`
			ItemID  int `db:"item_id,pk"`
			Qty     int `db:"qty"`
		}
		cols := ModelToColumns(CPK{})
		assert.Equal(t, map[string]string{
			"OrderID": "order_id",
			"ItemID":  "item_id",
			"Qty":     "qty",
		}, cols)
	})

	t.Run("empty struct returns empty map", func(t *testing.T) {
		type Empty struct{}
		cols := ModelToColumns(Empty{})
		assert.Empty(t, cols)
	})

	t.Run("struct with no db tags returns empty map", func(t *testing.T) {
		type NoTags struct {
			ID   int
			Name string
		}
		cols := ModelToColumns(NoTags{})
		assert.Empty(t, cols)
	})
}
