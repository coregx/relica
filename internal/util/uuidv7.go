package util

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// GenerateUUIDv7 generates an RFC 9562 UUID v7 string.
// 48-bit Unix timestamp (ms) for time-ordering + crypto/rand for randomness.
// Zero external dependencies.
func GenerateUUIDv7() string {
	var b [16]byte

	ms := uint64(time.Now().UnixMilli())
	binary.BigEndian.PutUint16(b[0:2], uint16(ms>>32)) //nolint:gosec // G115: intentional 48-bit truncation for UUID v7 timestamp
	binary.BigEndian.PutUint32(b[2:6], uint32(ms))     //nolint:gosec // G115: same — extracting low 32 bits of timestamp

	_, _ = rand.Read(b[6:])

	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 9562

	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])

	return string(buf[:])
}
