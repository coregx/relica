package util

import (
	"reflect"
	"strings"
	"testing"
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
			col, isPK, _ := parseDBTag(tt.tag)
			if col != tt.wantCol {
				t.Errorf("got %v, want %v", col, tt.wantCol)
			}
			if isPK != tt.wantIsPK {
				t.Errorf("got %v, want %v", isPK, tt.wantIsPK)
			}
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
		// string: empty = zero (for UUID/string PK auto-populate)
		{"string empty — zero", reflect.ValueOf(""), true},
		{"string non-empty — not zero", reflect.ValueOf("abc"), false},
		{"string nil UUID — not zero", reflect.ValueOf("00000000-0000-0000-0000-000000000000"), false},
		// [16]byte (uuid.UUID): all-zeros = zero
		{"[16]byte zero — zero", reflect.ValueOf([16]byte{}), true},
		{"[16]byte non-zero — not zero", reflect.ValueOf([16]byte{1}), false},
		// bool: false = zero (pathological case, acceptable)
		{"bool false — zero", reflect.ValueOf(false), true},
		{"bool true — not zero", reflect.ValueOf(true), false},
		// invalid value
		{"invalid reflect.Value", reflect.Value{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPrimaryKeyZero(tt.value)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsPrimaryKeyZero_Pointer tests pointer dereference logic.
func TestIsPrimaryKeyZero_Pointer(t *testing.T) {
	t.Run("nil pointer is zero", func(t *testing.T) {
		var p *int64
		if !IsPrimaryKeyZero(reflect.ValueOf(p)) {
			t.Error("expected true")
		}
	})

	t.Run("pointer to zero int64 is zero", func(t *testing.T) {
		v := int64(0)
		if !IsPrimaryKeyZero(reflect.ValueOf(&v)) {
			t.Error("expected true")
		}
	})

	t.Run("pointer to non-zero int64 is not zero", func(t *testing.T) {
		v := int64(42)
		if IsPrimaryKeyZero(reflect.ValueOf(&v)) {
			t.Error("expected false")
		}
	})

	t.Run("pointer to zero int is zero", func(t *testing.T) {
		v := int(0)
		if !IsPrimaryKeyZero(reflect.ValueOf(&v)) {
			t.Error("expected true")
		}
	})

	t.Run("pointer to non-zero uint is not zero", func(t *testing.T) {
		v := uint(7)
		if IsPrimaryKeyZero(reflect.ValueOf(&v)) {
			t.Error("expected false")
		}
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
				if err == nil {
					t.Fatal("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got := field.Int(); got != tt.id {
					t.Errorf("got %v, want %v", got, tt.id)
				}
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
				if err == nil {
					t.Fatal("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got := field.Uint(); got != uint64(tt.id) {
					t.Errorf("got %v, want %v", got, uint64(tt.id))
				}
			}
		})
	}
}

// TestSetPrimaryKeyValue_PointerField tests pointer field allocation and setting.
func TestSetPrimaryKeyValue_PointerField(t *testing.T) {
	t.Run("nil pointer to int64 is allocated and set", func(t *testing.T) {
		var p *int64
		field := reflect.ValueOf(&p).Elem()
		if err := SetPrimaryKeyValue(field, 42); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil")
		}
		if *p != int64(42) {
			t.Errorf("got %v, want %v", *p, int64(42))
		}
	})

	t.Run("nil pointer to int32 is allocated and set", func(t *testing.T) {
		var p *int32
		field := reflect.ValueOf(&p).Elem()
		if err := SetPrimaryKeyValue(field, 7); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil")
		}
		if *p != int32(7) {
			t.Errorf("got %v, want %v", *p, int32(7))
		}
	})

	t.Run("existing non-nil pointer is overwritten", func(t *testing.T) {
		existing := int64(100)
		p := &existing
		field := reflect.ValueOf(&p).Elem()
		if err := SetPrimaryKeyValue(field, 999); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *p != int64(999) {
			t.Errorf("got %v, want %v", *p, int64(999))
		}
	})
}

// TestSetPrimaryKeyValue_ErrorCases tests all error paths.
func TestSetPrimaryKeyValue_ErrorCases(t *testing.T) {
	t.Run("invalid reflect.Value returns error", func(t *testing.T) {
		err := SetPrimaryKeyValue(reflect.Value{}, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid field") {
			t.Errorf("expected error containing %q, got %v", "invalid field", err)
		}
	})

	t.Run("non-settable field returns error", func(t *testing.T) {
		type S struct{ ID int64 }
		s := S{}
		// Field obtained from non-pointer value is not settable.
		field := reflect.ValueOf(s).Field(0)
		err := SetPrimaryKeyValue(field, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "not settable") {
			t.Errorf("expected error containing %q, got %v", "not settable", err)
		}
	})

	t.Run("unsupported type string returns error", func(t *testing.T) {
		var s string
		field := reflect.ValueOf(&s).Elem()
		err := SetPrimaryKeyValue(field, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unsupported type") {
			t.Errorf("expected error containing %q, got %v", "unsupported type", err)
		}
	})

	t.Run("unsupported type float64 returns error", func(t *testing.T) {
		var f float64
		field := reflect.ValueOf(&f).Elem()
		err := SetPrimaryKeyValue(field, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unsupported type") {
			t.Errorf("expected error containing %q, got %v", "unsupported type", err)
		}
	})

	t.Run("unsupported type bool returns error", func(t *testing.T) {
		var b bool
		field := reflect.ValueOf(&b).Elem()
		err := SetPrimaryKeyValue(field, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unsupported type") {
			t.Errorf("expected error containing %q, got %v", "unsupported type", err)
		}
	})
}

// ─── FindPrimaryKeyFields ──────────────────────────────────────────────────────

// ─── PrimaryKeyInfo.IsSingle / IsComposite ────────────────────────────────────

// ─── FindPrimaryKeyField ───────────────────────────────────────────────────────

// ─── ModelToColumns ────────────────────────────────────────────────────────────

// ─── parseDBTag: autoincrement option ─────────────────────────────────────────

// TestParseDBTag_AutoIncrement covers all formats that include autoincrement.
func TestParseDBTag_AutoIncrement(t *testing.T) {
	tests := []struct {
		name        string
		tag         string
		wantCol     string
		wantIsPK    bool
		wantAutoInc bool
	}{
		{
			name:        "column,pk,autoincrement",
			tag:         "id,pk,autoincrement",
			wantCol:     "id",
			wantIsPK:    true,
			wantAutoInc: true,
		},
		{
			name:        "column,autoincrement without pk",
			tag:         "id,autoincrement",
			wantCol:     "id",
			wantIsPK:    false,
			wantAutoInc: true,
		},
		{
			name:        "plain column — no autoincrement",
			tag:         "id",
			wantCol:     "id",
			wantIsPK:    false,
			wantAutoInc: false,
		},
		{
			name:        "column,pk without autoincrement",
			tag:         "id,pk",
			wantCol:     "id",
			wantIsPK:    true,
			wantAutoInc: false,
		},
		{
			name:        "autoincrement with spaces around option",
			tag:         "uid, pk, autoincrement",
			wantCol:     "uid",
			wantIsPK:    true,
			wantAutoInc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, isPK, isAutoInc := parseDBTag(tt.tag)
			if col != tt.wantCol {
				t.Errorf("got %v, want %v", col, tt.wantCol)
			}
			if isPK != tt.wantIsPK {
				t.Errorf("got %v, want %v", isPK, tt.wantIsPK)
			}
			if isAutoInc != tt.wantAutoInc {
				t.Errorf("got %v, want %v", isAutoInc, tt.wantAutoInc)
			}
		})
	}
}

// ─── FindPrimaryKeyFields: AutoIncrement propagation ─────────────────────────

// ─── IsPrimaryKeyZero: string type ────────────────────────────────────────────

// TestIsPrimaryKeyZero_StringType validates string zero/non-zero detection
// specifically for the UUID/string PK use-case.
func TestIsPrimaryKeyZero_StringType(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty string is zero", "", true},
		{"non-empty string is not zero", "some-uuid", false},
		{"whitespace string is not zero", " ", false},
		{"all-zero UUID string is not zero", "00000000-0000-0000-0000-000000000000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPrimaryKeyZero(reflect.ValueOf(tt.value))
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
