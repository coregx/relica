package util

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// --- parseDBTag / ParseDBTagFull ---

func TestParseDBTagFull_AutoID(t *testing.T) {
	tests := []struct {
		tag        string
		wantCol    string
		wantAutoID bool
		wantPrefix string
		wantGen    string
		wantPK     bool
	}{
		{"public_id,autoid", "public_id", true, "", "", false},
		{"public_id,autoid:usr", "public_id", true, "usr", "", false},
		{"public_id,autoid:ord,gen=ulid", "public_id", true, "ord", "ulid", false},
		{"public_id,autoid,gen=snowflake", "public_id", true, "", "snowflake", false},
		{"id,pk", "id", false, "", "", true},
		{"id,pk,autoincrement", "id", false, "", "", true},
		{"name", "name", false, "", "", false},
		{"-", "-", false, "", "", false},
		{"trace_id,autoid:trc,gen=ulid", "trace_id", true, "trc", "ulid", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			info := ParseDBTagFull(tt.tag)
			if info.Column != tt.wantCol {
				t.Errorf("got %v, want %v", info.Column, tt.wantCol)
			}
			if info.IsAutoID != tt.wantAutoID {
				t.Errorf("IsAutoID: got %v, want %v", info.IsAutoID, tt.wantAutoID)
			}
			if info.AutoIDPrefix != tt.wantPrefix {
				t.Errorf("AutoIDPrefix: got %v, want %v", info.AutoIDPrefix, tt.wantPrefix)
			}
			if info.AutoIDGen != tt.wantGen {
				t.Errorf("AutoIDGen: got %v, want %v", info.AutoIDGen, tt.wantGen)
			}
			if info.IsPK != tt.wantPK {
				t.Errorf("IsPK: got %v, want %v", info.IsPK, tt.wantPK)
			}
		})
	}
}

func TestParseDBTag_BackwardCompatible(t *testing.T) {
	col, isPK, isAutoInc := parseDBTag("id,pk,autoincrement")
	if col != "id" {
		t.Errorf("got %v, want %v", col, "id")
	}
	if !isPK {
		t.Error("expected true")
	}
	if !isAutoInc {
		t.Error("expected true")
	}

	col, isPK, isAutoInc = parseDBTag("name")
	if col != "name" {
		t.Errorf("got %v, want %v", col, "name")
	}
	if isPK {
		t.Error("expected false")
	}
	if isAutoInc {
		t.Error("expected false")
	}

	col, isPK, isAutoInc = parseDBTag("public_id,autoid:usr")
	if col != "public_id" {
		t.Errorf("got %v, want %v", col, "public_id")
	}
	if isPK {
		t.Error("expected false")
	}
	if isAutoInc {
		t.Error("expected false")
	}
}

// --- FindAutoIDFields ---

func TestFindAutoIDFields_Single(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
		Name     string `db:"name"`
	}

	v := reflect.ValueOf(User{})
	fields := FindAutoIDFields(v)
	if len(fields) != 1 {
		t.Fatalf("expected length %d, got %d", 1, len(fields))
	}
	if fields[0].Column != "public_id" {
		t.Errorf("got %v, want %v", fields[0].Column, "public_id")
	}
	if fields[0].Prefix != "usr" {
		t.Errorf("got %v, want %v", fields[0].Prefix, "usr")
	}
	if fields[0].Generator != "" {
		t.Errorf("got %v, want %v", fields[0].Generator, "")
	}
	if fields[0].FieldIndex != 1 {
		t.Errorf("got %v, want %v", fields[0].FieldIndex, 1)
	}
}

func TestFindAutoIDFields_Multiple(t *testing.T) {
	type Order struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:ord"`
		TraceID  string `db:"trace_id,autoid:trc,gen=ulid"`
	}

	v := reflect.ValueOf(Order{})
	fields := FindAutoIDFields(v)
	if len(fields) != 2 {
		t.Fatalf("expected length %d, got %d", 2, len(fields))
	}
	if fields[0].Column != "public_id" {
		t.Errorf("got %v, want %v", fields[0].Column, "public_id")
	}
	if fields[0].Prefix != "ord" {
		t.Errorf("got %v, want %v", fields[0].Prefix, "ord")
	}
	if fields[1].Column != "trace_id" {
		t.Errorf("got %v, want %v", fields[1].Column, "trace_id")
	}
	if fields[1].Prefix != "trc" {
		t.Errorf("got %v, want %v", fields[1].Prefix, "trc")
	}
	if fields[1].Generator != "ulid" {
		t.Errorf("got %v, want %v", fields[1].Generator, "ulid")
	}
}

func TestFindAutoIDFields_None(t *testing.T) {
	type Simple struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	v := reflect.ValueOf(Simple{})
	fields := FindAutoIDFields(v)
	if fields != nil {
		t.Errorf("expected nil, got %v", fields)
	}
}

func TestFindAutoIDFields_NoPrefix(t *testing.T) {
	type Event struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid"`
	}

	v := reflect.ValueOf(Event{})
	fields := FindAutoIDFields(v)
	if len(fields) != 1 {
		t.Fatalf("expected length %d, got %d", 1, len(fields))
	}
	if fields[0].Column != "public_id" {
		t.Errorf("got %v, want %v", fields[0].Column, "public_id")
	}
	if fields[0].Prefix != "" {
		t.Errorf("got %v, want %v", fields[0].Prefix, "")
	}
	if !ParseDBTagFull("public_id,autoid").IsAutoID {
		t.Error("expected true")
	}
}

func TestFindAutoIDFields_Pointer(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
	}

	u := &User{}
	fields := FindAutoIDFields(reflect.ValueOf(u))
	if len(fields) != 1 {
		t.Fatalf("expected length %d, got %d", 1, len(fields))
	}
	if fields[0].Prefix != "usr" {
		t.Errorf("got %v, want %v", fields[0].Prefix, "usr")
	}
}

func TestFindAutoIDFields_NilPointer(t *testing.T) {
	var u *struct{ ID int64 }
	fields := FindAutoIDFields(reflect.ValueOf(u))
	if fields != nil {
		t.Errorf("expected nil, got %v", fields)
	}
}

func TestFindAutoIDFields_NonStruct(t *testing.T) {
	fields := FindAutoIDFields(reflect.ValueOf(42))
	if fields != nil {
		t.Errorf("expected nil, got %v", fields)
	}
}

// --- GenerateAutoID ---

func TestGenerateAutoID_WithPrefix(t *testing.T) {
	id := GenerateAutoID("usr", "")
	if len(id) <= 4 {
		t.Error("should have prefix + UUID")
	}
	if id[:4] != "usr_" {
		t.Errorf("got %v, want %v", id[:4], "usr_")
	}

	prefix, body := ParseAutoID(id)
	if prefix != "usr" {
		t.Errorf("got %v, want %v", prefix, "usr")
	}
	if len(body) != 36 { // UUID v7 length
		t.Errorf("expected length %d, got %d", 36, len(body))
	}
}

func TestGenerateAutoID_WithoutPrefix(t *testing.T) {
	id := GenerateAutoID("", "")
	if len(id) != 36 { // raw UUID v7
		t.Errorf("expected length %d, got %d", 36, len(id))
	}
	if strings.Contains(id[:8], "_") {
		t.Errorf("%q should not contain %q", id[:8], "_")
	}
}

func TestGenerateAutoID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for range 1000 {
		id := GenerateAutoID("usr", "")
		if seen[id] {
			t.Fatalf("duplicate: %s", id)
		}
		seen[id] = true
	}
}

func TestGenerateAutoID_CustomGenerator(t *testing.T) {
	RegisterIDGenerator("fixed", func() string { return "test-fixed-id" })
	defer func() {
		generatorsMu.Lock()
		delete(generators, "fixed")
		generatorsMu.Unlock()
	}()

	id := GenerateAutoID("evt", "fixed")
	if id != "evt_test-fixed-id" {
		t.Errorf("got %v, want %v", id, "evt_test-fixed-id")
	}

	id = GenerateAutoID("", "fixed")
	if id != "test-fixed-id" {
		t.Errorf("got %v, want %v", id, "test-fixed-id")
	}
}

func TestGenerateAutoID_UnknownGenerator(t *testing.T) {
	id := GenerateAutoID("x", "nonexistent")
	if len(id) <= 0 {
		t.Error("should fallback to UUID v7")
	}
	if id[:2] != "x_" {
		t.Errorf("got %v, want %v", id[:2], "x_")
	}
}

// --- ParseAutoID ---

func TestParseAutoID(t *testing.T) {
	tests := []struct {
		id         string
		wantPrefix string
		wantBody   string
	}{
		{"usr_019078fa-b37e", "usr", "019078fa-b37e"},
		{"ord_abc123", "ord", "abc123"},
		{"019078fa-b37e", "", "019078fa-b37e"},
		{"", "", ""},
		{"a_b_c", "a", "b_c"},
		{"_leading", "", "leading"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			prefix, body := ParseAutoID(tt.id)
			if prefix != tt.wantPrefix {
				t.Errorf("got %v, want %v", prefix, tt.wantPrefix)
			}
			if body != tt.wantBody {
				t.Errorf("got %v, want %v", body, tt.wantBody)
			}
		})
	}
}

// --- ValidateAutoIDPrefix ---

func TestValidateAutoIDPrefix_Match(t *testing.T) {
	err := ValidateAutoIDPrefix("usr_019078fa", "usr")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAutoIDPrefix_Mismatch(t *testing.T) {
	err := ValidateAutoIDPrefix("ord_019078fa", "usr")
	if !errors.Is(err, ErrAutoIDPrefixMismatch) {
		t.Errorf("expected error %v, got %v", ErrAutoIDPrefixMismatch, err)
	}
}

func TestValidateAutoIDPrefix_MissingPrefix(t *testing.T) {
	err := ValidateAutoIDPrefix("019078fa-b37e", "usr")
	if !errors.Is(err, ErrAutoIDPrefixMismatch) {
		t.Errorf("expected error %v, got %v", ErrAutoIDPrefixMismatch, err)
	}
}

func TestValidateAutoIDPrefix_EmptyExpected(t *testing.T) {
	err := ValidateAutoIDPrefix("anything", "")
	if err != nil {
		t.Errorf("empty expected prefix should accept any ID: unexpected error: %v", err)
	}
}

func TestValidateAutoIDPrefix_EmptyID(t *testing.T) {
	err := ValidateAutoIDPrefix("", "usr")
	if !errors.Is(err, ErrAutoIDPrefixMismatch) {
		t.Errorf("expected error %v, got %v", ErrAutoIDPrefixMismatch, err)
	}
}

// --- RegisterIDGenerator ---

func TestRegisterIDGenerator(t *testing.T) {
	counter := 0
	RegisterIDGenerator("seq", func() string {
		counter++
		return "seq-" + string(rune('0'+counter))
	})
	defer func() {
		generatorsMu.Lock()
		delete(generators, "seq")
		generatorsMu.Unlock()
	}()

	id1 := GenerateAutoID("", "seq")
	id2 := GenerateAutoID("", "seq")
	if id1 == id2 {
		t.Errorf("expected different, both %v", id1)
	}
}

func BenchmarkGenerateAutoID(b *testing.B) {
	for range b.N {
		GenerateAutoID("usr", "")
	}
}
