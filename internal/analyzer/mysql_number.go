package analyzer

import (
	"bytes"
	"fmt"
	"strconv"
)

// MySQL prints some EXPLAIN FORMAT=JSON numbers as JSON strings ("filtered":
// "33.33", every cost) and others as JSON numbers, and the choice differs
// between MySQL and MariaDB versions. mysqlFloat and mysqlInt accept both forms.

// mysqlFloat is a float that may be quoted in the JSON input.
type mysqlFloat float64

// UnmarshalJSON accepts 33.33, "33.33" and null (zero).
func (f *mysqlFloat) UnmarshalJSON(b []byte) error {
	s, ok := unquoteNumber(b)
	if !ok {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("mysql explain: invalid number %s: %w", b, err)
	}
	*f = mysqlFloat(v)
	return nil
}

// mysqlInt is an integer that may be quoted in the JSON input.
type mysqlInt int64

// UnmarshalJSON accepts 7, "7" and null (zero). A fractional value is
// truncated: row estimates are integral, but some versions print "7.0".
func (n *mysqlInt) UnmarshalJSON(b []byte) error {
	s, ok := unquoteNumber(b)
	if !ok {
		*n = 0
		return nil
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		*n = mysqlInt(v)
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("mysql explain: invalid number %s: %w", b, err)
	}
	*n = mysqlInt(v)
	return nil
}

// unquoteNumber strips JSON string quotes around a number. It reports false
// for null and for an empty string.
func unquoteNumber(b []byte) (string, bool) {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return "", false
	}
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		b = b[1 : len(b)-1]
	}
	if len(b) == 0 {
		return "", false
	}
	return string(b), true
}
