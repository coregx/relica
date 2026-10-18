package core

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"syscall"
	"testing"
)

// Look-alike driver error types. Each one copies the exported shape of a real
// driver error (verified against the driver sources named in the comment), so
// the extraction code is tested without importing any driver. The integration
// suite (test/errclass_integration_test.go) repeats the checks with the real
// drivers against real servers.

// sqlStateError mimics *pgconn.PgError (pgx v5) and *pq.Error (lib/pq >= v1.10.9).
type sqlStateError struct{ code, msg string }

func (e *sqlStateError) Error() string    { return e.msg }
func (e *sqlStateError) SQLState() string { return e.code }

// legacyPQError mimics *pq.Error of lib/pq < v1.10.9 (v1.2.0): Code is a
// string-kinded field, there is no SQLState method.
type legacyPQErrorCode string

type legacyPQError struct {
	Severity string
	Code     legacyPQErrorCode
	Message  string
}

func (e *legacyPQError) Error() string { return "pq: " + e.Message }

// mysqlError mimics *mysql.MySQLError (go-sql-driver/mysql v1.9.3).
type mysqlError struct {
	Number   uint16
	SQLState [5]byte
	Message  string
}

func (e *mysqlError) Error() string {
	return "Error " + strconv.Itoa(int(e.Number)) + ": " + e.Message
}

// moderncError mimics *sqlite.Error (modernc.org/sqlite v1.39.1).
type moderncError struct {
	msg  string
	code int
}

func (e *moderncError) Error() string { return e.msg }
func (e *moderncError) Code() int     { return e.code }

// mattnError mimics sqlite3.Error (mattn/go-sqlite3 v1.14.50), a value type.
type mattnErrNo int
type mattnErrNoExtended int

type mattnError struct {
	Code         mattnErrNo
	ExtendedCode mattnErrNoExtended
	SystemErrno  syscall.Errno
}

func (e mattnError) Error() string { return "sqlite3 error " + strconv.Itoa(int(e.ExtendedCode)) }

// testDrivers registers the look-alike types the way knownDrivers registers
// the real ones.
var testDrivers = func() []driverShape {
	pkg := reflect.TypeOf(sqlStateError{}).PkgPath()
	return []driverShape{
		{pkgPath: pkg, typeName: "legacyPQError", extract: pqCodeField},
		{pkgPath: pkg, typeName: "mysqlError", extract: mysqlNumberField},
		{pkgPath: pkg, typeName: "moderncError", extract: sqliteCodeMethod},
		{pkgPath: pkg, typeName: "mattnError", extract: mattnExtendedCodeField},
	}
}()

func mysqlState(s string) [5]byte {
	var b [5]byte
	copy(b[:], s)
	return b
}

// classNames lists every class with its public helper name, for messages.
var classNames = []struct {
	name  string
	class *errClass
}{
	{"IsUniqueViolation", &uniqueViolation},
	{"IsForeignKeyViolation", &foreignKeyViolation},
	{"IsNotNullViolation", &notNullViolation},
	{"IsCheckViolation", &checkViolation},
	{"IsDeadlock", &deadlock},
	{"IsSerializationFailure", &serializationFailure},
	{"IsLockTimeout", &lockTimeout},
}

// TestClassifyByCode checks every code of every class, per driver shape, and
// that the code matches exactly one class (want == "" means none).
func TestClassifyByCode(t *testing.T) {
	// German server messages: lc_messages=de_DE must not change the result.
	pg := func(code string) error { return &sqlStateError{code: code, msg: "FEHLER: etwas ist schiefgegangen"} }
	oldPQ := func(code string) error { return &legacyPQError{Code: legacyPQErrorCode(code), Message: "FEHLER"} }
	my := func(n uint16, state string) error {
		return &mysqlError{Number: n, SQLState: mysqlState(state), Message: "Fehler"}
	}
	modernc := func(code int) error { return &moderncError{msg: "Fehler", code: code} }
	mattn := func(code int) error {
		return mattnError{Code: mattnErrNo(code & 0xff), ExtendedCode: mattnErrNoExtended(code)}
	}

	tests := []struct {
		name string
		err  error
		want string
	}{
		// PostgreSQL SQLSTATE (pgx, lib/pq >= v1.10.9)
		{"pg 23505", pg("23505"), "IsUniqueViolation"},
		{"pg 23503", pg("23503"), "IsForeignKeyViolation"},
		{"pg 23502", pg("23502"), "IsNotNullViolation"},
		{"pg 23514", pg("23514"), "IsCheckViolation"},
		{"pg 40P01", pg("40P01"), "IsDeadlock"},
		{"pg 40001", pg("40001"), "IsSerializationFailure"},
		{"pg 55P03", pg("55P03"), "IsLockTimeout"},
		{"pg 23P01 exclusion is none of them", pg("23P01"), ""},
		{"pg 57014 statement timeout is not a lock timeout", pg("57014"), ""},
		{"pg 42601 syntax error", pg("42601"), ""},

		// lib/pq < v1.10.9 Code field
		{"old pq 23505", oldPQ("23505"), "IsUniqueViolation"},
		{"old pq 40P01", oldPQ("40P01"), "IsDeadlock"},
		{"old pq 42P01", oldPQ("42P01"), ""},

		// MySQL / MariaDB error numbers
		{"mysql 1062", my(1062, "23000"), "IsUniqueViolation"},
		{"mysql 1022", my(1022, "23000"), "IsUniqueViolation"},
		{"mysql 1169", my(1169, "23000"), "IsUniqueViolation"},
		{"mysql 1586", my(1586, "23000"), "IsUniqueViolation"},
		{"mysql 1859", my(1859, "23000"), "IsUniqueViolation"},
		{"mysql 1216", my(1216, "23000"), "IsForeignKeyViolation"},
		{"mysql 1217", my(1217, "23000"), "IsForeignKeyViolation"},
		{"mysql 1451", my(1451, "23000"), "IsForeignKeyViolation"},
		{"mysql 1452", my(1452, "23000"), "IsForeignKeyViolation"},
		{"mysql 1048", my(1048, "23000"), "IsNotNullViolation"},
		{"mysql 1364", my(1364, "HY000"), "IsNotNullViolation"},
		{"mysql 1263", my(1263, "22004"), "IsNotNullViolation"},
		{"mysql 3819", my(3819, "HY000"), "IsCheckViolation"},
		{"mariadb 4025 with 23000", my(4025, "23000"), "IsCheckViolation"},
		{"4025 without SQLSTATE 23000 is not MariaDB CHECK", my(4025, "HY000"), ""},
		{"mysql 1213", my(1213, "40001"), "IsDeadlock"},
		{"mysql 1205", my(1205, "HY000"), "IsLockTimeout"},
		{"mysql 3572", my(3572, "HY000"), "IsLockTimeout"},
		{"mysql 1146 no such table", my(1146, "42S02"), ""},
		{"mysql SQLSTATE 40001 alone is not a serialization failure", my(1999, "40001"), ""},

		// SQLite extended result codes (modernc)
		{"modernc 2067 UNIQUE", modernc(2067), "IsUniqueViolation"},
		{"modernc 1555 PRIMARYKEY", modernc(1555), "IsUniqueViolation"},
		{"modernc 787 FOREIGNKEY", modernc(787), "IsForeignKeyViolation"},
		{"modernc 1299 NOTNULL", modernc(1299), "IsNotNullViolation"},
		{"modernc 275 CHECK", modernc(275), "IsCheckViolation"},
		{"modernc 517 BUSY_SNAPSHOT", modernc(517), "IsSerializationFailure"},
		{"modernc 5 BUSY", modernc(5), "IsLockTimeout"},
		{"modernc 261 BUSY_RECOVERY", modernc(261), "IsLockTimeout"},
		{"modernc 773 BUSY_TIMEOUT", modernc(773), "IsLockTimeout"},
		{"modernc 6 LOCKED", modernc(6), "IsLockTimeout"},
		{"modernc 262 LOCKED_SHAREDCACHE", modernc(262), "IsLockTimeout"},
		{"modernc 1 ERROR", modernc(1), ""},
		{"modernc 2579 CONSTRAINT_TRIGGER", modernc(2579), ""},

		// SQLite extended result codes (mattn)
		{"mattn 2067", mattn(2067), "IsUniqueViolation"},
		{"mattn 787", mattn(787), "IsForeignKeyViolation"},
		{"mattn 517", mattn(517), "IsSerializationFailure"},
		{"mattn 5", mattn(5), "IsLockTimeout"},
		{"mattn 1", mattn(1), ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, c := range classNames {
				got := classifyWith(tc.err, c.class, testDrivers)
				if want := c.name == tc.want; got != want {
					t.Errorf("%s(%v) = %v, want %v", c.name, tc.err, got, want)
				}
			}
		})
	}
}

// TestClassify_CodeWinsOverText: once a driver code is found, the message is
// never consulted — neither to add a match nor to override the code.
func TestClassify_CodeWinsOverText(t *testing.T) {
	// FK code, message of a unique violation.
	err := &sqlStateError{code: "23503", msg: `duplicate key value violates unique constraint "u"`}
	if classifyWith(err, &uniqueViolation, testDrivers) {
		t.Error("unique matched by text although SQLSTATE 23503 says foreign key")
	}
	if !classifyWith(err, &foreignKeyViolation, testDrivers) {
		t.Error("foreign key not matched by SQLSTATE 23503")
	}

	// Unknown code, message that every text pattern would match.
	err2 := &mysqlError{Number: 1146, Message: "Duplicate entry; Deadlock found; Lock wait timeout exceeded"}
	for _, c := range classNames {
		if classifyWith(err2, c.class, testDrivers) {
			t.Errorf("%s matched text although error number 1146 was found", c.name)
		}
	}
}

// TestClassify_ErrorTree: codes are found through %w chains, errors.Join
// branches and Relica's own wrappers; the outermost code wins.
func TestClassify_ErrorTree(t *testing.T) {
	unique := &sqlStateError{code: "23505", msg: "x"}
	deadlockErr := &mysqlError{Number: 1213, Message: "x"}

	tests := []struct {
		name  string
		err   error
		class *errClass
		want  bool
	}{
		{"fmt %w", fmt.Errorf("insert user: %w", unique), &uniqueViolation, true},
		{"double %w", fmt.Errorf("a: %w", fmt.Errorf("b: %w", unique)), &uniqueViolation, true},
		{"WrapError", WrapError(unique, "insert"), &uniqueViolation, true},
		{"errors.Join second branch", errors.Join(errors.New("first"), deadlockErr), &deadlock, true},
		{"multi %w", fmt.Errorf("%w; %w", errors.New("a"), deadlockErr), &deadlock, true},
		{"rollback error joined after work error", errors.Join(
			fmt.Errorf("work: %w", deadlockErr), fmt.Errorf("relica: rollback: %w", errors.New("conn reset"))),
			&deadlock, true},
		{"value type through %w", fmt.Errorf("q: %w", mattnError{Code: 19, ExtendedCode: 2067}), &uniqueViolation, true},
		{"outermost code wins", errors.Join(&sqlStateError{code: "40P01", msg: "x"}, unique), &uniqueViolation, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyWith(tc.err, tc.class, testDrivers); got != tc.want {
				t.Errorf("got %v, want %v for %v", got, tc.want, tc.err)
			}
		})
	}
}

// TestClassify_TextFallback: without any code in the tree the message decides,
// which keeps unknown drivers working.
func TestClassify_TextFallback(t *testing.T) {
	tests := []struct {
		msg   string
		class *errClass
	}{
		{`pq: duplicate key value violates unique constraint "users_email_key"`, &uniqueViolation},
		{"UNIQUE constraint failed: users.email", &uniqueViolation},
		{"ERROR: deadlock detected (SQLSTATE 40P01)", &deadlock},
		{"Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction", &deadlock},
		{"ERROR: could not serialize access due to concurrent update (SQLSTATE 40001)", &serializationFailure},
		{"ERROR: canceling statement due to lock timeout (SQLSTATE 55P03)", &lockTimeout},
		{`ERROR: could not obtain lock on row in relation "jobs" (SQLSTATE 55P03)`, &lockTimeout},
		{"Error 1205 (HY000): Lock wait timeout exceeded; try restarting transaction", &lockTimeout},
		{"database is locked", &lockTimeout},
		{"database table is locked: jobs", &lockTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.msg, func(t *testing.T) {
			err := errors.New(tc.msg)
			for _, c := range classNames {
				got := classifyWith(err, c.class, testDrivers)
				if want := c.class == tc.class; got != want {
					t.Errorf("%s = %v, want %v", c.name, got, want)
				}
			}
		})
	}
}

// TestClassify_NoCodeCases: values that look like a code holder but carry none
// fall back to text instead of being classified as "unknown code".
func TestClassify_NoCodeCases(t *testing.T) {
	const uniqueText = "UNIQUE constraint failed: users.email"

	t.Run("empty SQLSTATE", func(t *testing.T) {
		if !classifyWith(&sqlStateError{code: "", msg: uniqueText}, &uniqueViolation, testDrivers) {
			t.Error("empty SQLSTATE must fall back to text")
		}
	})
	t.Run("SQLite primary CONSTRAINT without extended code", func(t *testing.T) {
		// Extended codes are off: 19 alone does not say which constraint.
		err := &moderncError{msg: uniqueText, code: 19}
		if !classifyWith(err, &uniqueViolation, testDrivers) {
			t.Error("primary code 19 must fall back to text")
		}
		if classifyWith(err, &checkViolation, testDrivers) {
			t.Error("primary code 19 with a UNIQUE message matched CHECK")
		}
	})
	t.Run("typed nil pointer", func(t *testing.T) {
		// SQLState() on a nil *sqlStateError would panic; codeOf must not call it.
		var nilPG *sqlStateError
		if _, ok := codeOf(nilPG, testDrivers); ok {
			t.Error("typed nil reported a code")
		}
		var nilMySQL *mysqlError
		if _, ok := codeOf(nilMySQL, testDrivers); ok {
			t.Error("typed nil reported a code")
		}
	})
	t.Run("nil error", func(t *testing.T) {
		for _, c := range classNames {
			if classifyWith(nil, c.class, testDrivers) {
				t.Errorf("%s(nil) = true", c.name)
			}
		}
	})
}

// TestClassify_UnknownTypeIsNotReflected: reflection is bound to the exact
// driver package and type name, so an application type with the same field
// names is never read as a driver code.
func TestClassify_UnknownTypeIsNotReflected(t *testing.T) {
	// "Error 1213: app" matches no text pattern, so only reflection could match.
	err := &mysqlError{Number: 1213, Message: "app"}
	if !classifyWith(err, &deadlock, testDrivers) {
		t.Fatal("precondition: the look-alike is classified when registered")
	}
	if classifyWith(err, &deadlock, knownDrivers) {
		t.Error("a non-driver type with a Number field was classified by code")
	}
	if classifyWith(&moderncError{msg: "app", code: 6}, &lockTimeout, knownDrivers) {
		t.Error("a non-driver type with Code() int was classified by code")
	}
}

// changedShapeError is a driver type after a hypothetical breaking release: every
// field the extractors read is missing or has another kind.
type changedShapeError struct {
	Code         int    // was a string (lib/pq)
	Number       string // was uint16 (MySQL)
	ExtendedCode string // was int (mattn)
	msg          string
}

func (e *changedShapeError) Error() string { return e.msg }

type stringError string

func (e stringError) Error() string { return string(e) }

// TestClassify_DriverShapeChanged: if a driver changes the shape the extractor
// expects, classification degrades to text matching instead of misreading.
func TestClassify_DriverShapeChanged(t *testing.T) {
	pkg := reflect.TypeOf(changedShapeError{}).PkgPath()
	for _, extract := range []func(error, reflect.Value) (dbCode, bool){
		pqCodeField, mysqlNumberField, sqliteCodeMethod, mattnExtendedCodeField,
	} {
		drivers := []driverShape{{pkgPath: pkg, typeName: "changedShapeError", extract: extract}}
		err := &changedShapeError{Code: 23503, Number: "1452", ExtendedCode: "787", msg: "UNIQUE constraint failed: t.c"}
		if _, ok := findCode(err, drivers); ok {
			t.Errorf("%T: a code was read from a changed shape", extract)
		}
		if !classifyWith(err, &uniqueViolation, drivers) {
			t.Error("text fallback did not run after the shape changed")
		}
	}

	t.Run("MySQL zero number or missing SQLState", func(t *testing.T) {
		if _, ok := findCode(&mysqlError{Number: 0, Message: "x"}, testDrivers); ok {
			t.Error("error number 0 read as a code")
		}
		code, ok := findCode(&mysqlError{Number: 1213, Message: "x"}, testDrivers)
		if !ok || code.state != "" {
			t.Errorf("got %+v, %v; want number 1213 with empty SQLSTATE", code, ok)
		}
	})
	t.Run("non-struct error type", func(t *testing.T) {
		if !classifyWith(stringError("Deadlock found when trying to get lock"), &deadlock, testDrivers) {
			t.Error("a string-kinded error must go to the text fallback")
		}
	})
	t.Run("chain without a code", func(t *testing.T) {
		err := errors.Join(fmt.Errorf("a: %w", errors.New("b")), errors.New("c"))
		if _, ok := findCode(err, testDrivers); ok {
			t.Error("code found in a tree without one")
		}
	})
}

// TestKnownDrivers pins the driver types that are read by reflection.
func TestKnownDrivers(t *testing.T) {
	want := map[string]bool{
		"github.com/lib/pq.Error":                   true,
		"github.com/go-sql-driver/mysql.MySQLError": true,
		"modernc.org/sqlite.Error":                  true,
		"github.com/mattn/go-sqlite3.Error":         true,
	}
	if len(knownDrivers) != len(want) {
		t.Fatalf("knownDrivers has %d entries, want %d", len(knownDrivers), len(want))
	}
	for _, d := range knownDrivers {
		if !want[d.pkgPath+"."+d.typeName] {
			t.Errorf("unexpected driver %s.%s", d.pkgPath, d.typeName)
		}
		if d.extract == nil {
			t.Errorf("%s.%s has no extractor", d.pkgPath, d.typeName)
		}
	}
}

// TestPublicHelpers_UseSQLStateInterface: the exported helpers classify the
// interface-based shapes (pgx, current lib/pq) without any registration.
func TestPublicHelpers_UseSQLStateInterface(t *testing.T) {
	tests := []struct {
		code string
		fn   func(error) bool
	}{
		{"23505", IsUniqueViolation},
		{"23503", IsForeignKeyViolation},
		{"23502", IsNotNullViolation},
		{"23514", IsCheckViolation},
		{"40P01", IsDeadlock},
		{"40001", IsSerializationFailure},
		{"55P03", IsLockTimeout},
	}
	for _, tc := range tests {
		err := fmt.Errorf("op: %w", &sqlStateError{code: tc.code, msg: "Fehler"})
		if !tc.fn(err) {
			t.Errorf("SQLSTATE %s not classified", tc.code)
		}
	}
}
