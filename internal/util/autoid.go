package util

import (
	"errors"
	"strings"
	"sync"
)

// ErrAutoIDPrefixMismatch is returned when FindByPublicID receives an ID
// whose prefix doesn't match the model's autoid tag.
var ErrAutoIDPrefixMismatch = errors.New("autoid: prefix mismatch")

var (
	generatorsMu sync.RWMutex
	generators   = map[string]func() string{
		"":      GenerateUUIDv7,
		"uuid7": GenerateUUIDv7,
	}
)

// RegisterIDGenerator registers a custom ID generator under the given name.
// Use gen=name in the autoid tag to reference it: db:"col,autoid:prefix,gen=name"
func RegisterIDGenerator(name string, fn func() string) {
	generatorsMu.Lock()
	generators[name] = fn
	generatorsMu.Unlock()
}

// GenerateAutoID generates a full ID string with optional prefix using the named generator.
// Empty genName uses the default UUID v7 generator.
func GenerateAutoID(prefix, genName string) string {
	generatorsMu.RLock()
	gen, ok := generators[genName]
	generatorsMu.RUnlock()

	if !ok {
		gen = GenerateUUIDv7
	}

	id := gen()
	if prefix != "" {
		return prefix + "_" + id
	}
	return id
}

// ParseAutoID splits a prefixed ID into prefix and body.
// "usr_019078fa-..." → ("usr", "019078fa-...")
// "019078fa-..."     → ("", "019078fa-...")
func ParseAutoID(id string) (prefix, body string) {
	idx := strings.Index(id, "_")
	if idx < 0 {
		return "", id
	}
	return id[:idx], id[idx+1:]
}

// ValidateAutoIDPrefix checks that the ID starts with the expected prefix.
// Returns ErrAutoIDPrefixMismatch if it doesn't match.
// If expectedPrefix is empty, validation is skipped (any ID accepted).
func ValidateAutoIDPrefix(id, expectedPrefix string) error {
	if expectedPrefix == "" {
		return nil
	}
	prefix, _ := ParseAutoID(id)
	if prefix != expectedPrefix {
		return ErrAutoIDPrefixMismatch
	}
	return nil
}
