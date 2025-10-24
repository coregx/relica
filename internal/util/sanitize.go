package util

import (
	"regexp"
)

var sqlInjectionRegex = regexp.MustCompile(`(?i)(\b(select|insert|update|delete|drop|alter|union)\b|\-\-|;|\/\*)`)

// SanitizeString removes potentially dangerous SQL characters from input.
func SanitizeString(input string) string {
	return sqlInjectionRegex.ReplaceAllString(input, "")
}

// SanitizeIdentifier validates and sanitizes database identifiers by removing non-word characters.
func SanitizeIdentifier(ident string) string {
	// Remove everything except letters, numbers, and underscores
	return regexp.MustCompile(`\W`).ReplaceAllString(ident, "")
}
