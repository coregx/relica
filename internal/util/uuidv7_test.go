package util

import (
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
	"time"
)

var uuidv7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestGenerateUUIDv7_Format(t *testing.T) {
	id := GenerateUUIDv7()

	if len(id) != 36 {
		t.Errorf("expected length %d, got %d", 36, len(id))
	}
	if !uuidv7Pattern.MatchString(id) {
		t.Errorf("%q does not match %v", id, uuidv7Pattern)
	}
}

func TestGenerateUUIDv7_VersionNibble(t *testing.T) {
	for range 50 {
		id := GenerateUUIDv7()
		if id[14] != byte('7') {
			t.Errorf("version nibble must be 7, got %s", id)
		}
	}
}

func TestGenerateUUIDv7_VariantBits(t *testing.T) {
	for range 50 {
		id := GenerateUUIDv7()
		if !strings.Contains("89ab", string(id[19])) {
			t.Errorf("variant must be RFC 9562 (8/9/a/b), got %c in %s", id[19], id)
		}
	}
}

func TestGenerateUUIDv7_Uniqueness(t *testing.T) {
	const n = 10000
	seen := make(map[string]bool, n)
	for range n {
		id := GenerateUUIDv7()
		if seen[id] {
			t.Fatalf("duplicate UUID: %s", id)
		}
		seen[id] = true
	}
}

func TestGenerateUUIDv7_TimeOrdering(t *testing.T) {
	id1 := GenerateUUIDv7()
	time.Sleep(2 * time.Millisecond)
	id2 := GenerateUUIDv7()

	if id2 <= id1 {
		t.Errorf("later UUID must sort higher: %s should be > %s", id2, id1)
	}
}

func TestGenerateUUIDv7_TimestampEmbedded(t *testing.T) {
	before := time.Now().UnixMilli()
	id := GenerateUUIDv7()
	after := time.Now().UnixMilli()

	raw := strings.ReplaceAll(id, "-", "")
	b, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ms := uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 |
		uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5])

	if ms < uint64(before) {
		t.Errorf("expected %v >= %v", ms, uint64(before))
	}
	if ms > uint64(after) {
		t.Errorf("expected %v <= %v", ms, uint64(after))
	}
}

func TestGenerateUUIDv7_Lowercase(t *testing.T) {
	for range 20 {
		id := GenerateUUIDv7()
		if strings.ToLower(id) != id {
			t.Errorf("got %v, want %v", id, strings.ToLower(id))
		}
	}
}

func TestGenerateUUIDv7_DashPositions(t *testing.T) {
	id := GenerateUUIDv7()
	if id[8] != byte('-') {
		t.Errorf("got %v, want %v", id[8], byte('-'))
	}
	if id[13] != byte('-') {
		t.Errorf("got %v, want %v", id[13], byte('-'))
	}
	if id[18] != byte('-') {
		t.Errorf("got %v, want %v", id[18], byte('-'))
	}
	if id[23] != byte('-') {
		t.Errorf("got %v, want %v", id[23], byte('-'))
	}
}

func BenchmarkGenerateUUIDv7(b *testing.B) {
	for range b.N {
		GenerateUUIDv7()
	}
}
