package core

import (
	"reflect"
	"strings"
)

// Database error classification by driver error code (ADR-017).
//
// Codes are read without importing any driver:
//
//   - SQLSTATE through the SQLState() string method — pgx v5 *pgconn.PgError,
//     lib/pq >= v1.10.9 *pq.Error, and other drivers following the convention;
//   - by reflection, only from the exact driver types in knownDrivers — lib/pq
//     before v1.10.9 (Code field), go-sql-driver/mysql (Number field),
//     modernc.org/sqlite (Code() int), mattn/go-sqlite3 (ExtendedCode field).
//
// The whole error tree is searched (Unwrap() error and Unwrap() []error), in
// the same pre-order as errors.As, and the first error that carries a code
// decides. The message text is matched only when the tree holds no code at
// all: server messages are localized (PostgreSQL lc_messages) and change
// between versions, codes do not.

// codeKind tells which numbering a dbCode uses.
type codeKind uint8

const (
	codeSQLState codeKind = iota + 1 // PostgreSQL-style SQLSTATE in state
	codeMySQL                        // MySQL/MariaDB error number + SQLSTATE
	codeSQLite                       // SQLite extended result code
)

// dbCode is the code carried by a driver error.
type dbCode struct {
	kind   codeKind
	state  string // SQLSTATE (codeSQLState, codeMySQL; may be empty for MySQL)
	number int    // MySQL error number or SQLite extended result code
}

// SQLite result codes used below. Extended codes are primary | (n << 8);
// https://www.sqlite.org/rescode.html.
const (
	sqliteBusy         = 5
	sqliteLocked       = 6
	sqliteConstraint   = 19
	sqliteBusySnapshot = 517 // SQLITE_BUSY | 2<<8
)

// errClass describes one error class across databases.
type errClass struct {
	sqlStates []string                            // PostgreSQL SQLSTATE values
	mysql     func(number int, state string) bool // MySQL/MariaDB error numbers
	sqlite    func(extended int) bool             // SQLite extended result codes
	texts     []string                            // fallback message substrings
}

func (c *errClass) matchCode(code dbCode) bool {
	switch code.kind {
	case codeSQLState:
		for _, s := range c.sqlStates {
			if code.state == s {
				return true
			}
		}
	case codeMySQL:
		return c.mysql != nil && c.mysql(code.number, code.state)
	case codeSQLite:
		return c.sqlite != nil && c.sqlite(code.number)
	}
	return false
}

func (c *errClass) matchText(msg string) bool {
	for _, s := range c.texts {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func mysqlNumbers(numbers ...int) func(int, string) bool {
	return func(n int, _ string) bool {
		for _, x := range numbers {
			if n == x {
				return true
			}
		}
		return false
	}
}

func sqliteCodes(codes ...int) func(int) bool {
	return func(code int) bool {
		for _, x := range codes {
			if code == x {
				return true
			}
		}
		return false
	}
}

// Error classes. Sources: PostgreSQL Appendix A "Error Codes"; MySQL Server
// Error Message Reference; MariaDB error codes; SQLite "Result and Error Codes".
var (
	uniqueViolation = errClass{
		sqlStates: []string{"23505"},
		// ER_DUP_ENTRY, ER_DUP_KEY, ER_DUP_UNIQUE, ER_DUP_ENTRY_WITH_KEY_NAME,
		// ER_DUP_UNKNOWN_IN_INDEX
		mysql:  mysqlNumbers(1062, 1022, 1169, 1586, 1859),
		sqlite: sqliteCodes(2067, 1555), // CONSTRAINT_UNIQUE, CONSTRAINT_PRIMARYKEY
		texts: []string{
			"duplicate key value violates unique constraint",
			"Duplicate entry",
			"UNIQUE constraint failed",
			"Error 1062",
		},
	}

	foreignKeyViolation = errClass{
		sqlStates: []string{"23503"},
		// ER_NO_REFERENCED_ROW, ER_ROW_IS_REFERENCED, ER_ROW_IS_REFERENCED_2,
		// ER_NO_REFERENCED_ROW_2
		mysql:  mysqlNumbers(1216, 1217, 1451, 1452),
		sqlite: sqliteCodes(787), // CONSTRAINT_FOREIGNKEY
		texts: []string{
			"violates foreign key constraint",
			"a foreign key constraint fails",
			"FOREIGN KEY constraint failed",
			"Error 1451",
			"Error 1452",
		},
	}

	notNullViolation = errClass{
		sqlStates: []string{"23502"},
		// ER_BAD_NULL_ERROR, ER_WARN_NULL_TO_NOTNULL (multi-row INSERT),
		// ER_NO_DEFAULT_FOR_FIELD (NOT NULL column omitted, strict mode)
		mysql:  mysqlNumbers(1048, 1263, 1364),
		sqlite: sqliteCodes(1299), // CONSTRAINT_NOTNULL
		texts: []string{
			"violates not-null constraint",
			"cannot be null",
			"NOT NULL constraint failed",
			"Error 1048",
		},
	}

	checkViolation = errClass{
		sqlStates: []string{"23514"},
		// MySQL ER_CHECK_CONSTRAINT_VIOLATED. MariaDB ER_CONSTRAINT_FAILED is 4025
		// with SQLSTATE 23000; the MySQL and MariaDB number spaces diverge above
		// 4000, so 4025 counts only together with that SQLSTATE.
		mysql: func(n int, state string) bool {
			return n == 3819 || (n == 4025 && state == "23000")
		},
		sqlite: sqliteCodes(275), // CONSTRAINT_CHECK
		texts: []string{
			"violates check constraint",
			"Check constraint",
			"CHECK constraint failed",
			"Error 3819",
		},
	}

	deadlock = errClass{
		sqlStates: []string{"40P01"},  // deadlock_detected
		mysql:     mysqlNumbers(1213), // ER_LOCK_DEADLOCK
		// SQLite has a single writer and no deadlock detection.
		texts: []string{
			"deadlock detected",
			"Deadlock found",
		},
	}

	serializationFailure = errClass{
		sqlStates: []string{"40001"}, // serialization_failure
		// InnoDB reports write conflicts as ER_LOCK_DEADLOCK (IsDeadlock).
		sqlite: sqliteCodes(sqliteBusySnapshot), // a WAL snapshot went stale
		texts: []string{
			"could not serialize access",
		},
	}

	lockTimeout = errClass{
		sqlStates: []string{"55P03"}, // lock_not_available: lock_timeout, NOWAIT
		// ER_LOCK_WAIT_TIMEOUT, ER_LOCK_NOWAIT
		mysql: mysqlNumbers(1205, 3572),
		// Every SQLITE_BUSY (the busy timeout expired) except BUSY_SNAPSHOT, and
		// every SQLITE_LOCKED.
		sqlite: func(code int) bool {
			primary := code & 0xff
			return (primary == sqliteBusy && code != sqliteBusySnapshot) || primary == sqliteLocked
		},
		texts: []string{
			"lock timeout",
			"could not obtain lock",
			"Lock wait timeout exceeded",
			"database is locked",
			"database table is locked",
		},
	}
)

// driverShape binds a driver error type, by package path and type name, to the
// function that reads its code.
type driverShape struct {
	pkgPath  string
	typeName string
	extract  func(err error, v reflect.Value) (dbCode, bool)
}

// knownDrivers are the only types whose code is read by reflection or by a
// non-standard method. Field and method shapes are taken from the driver
// sources at the versions noted.
var knownDrivers = []driverShape{
	{pkgPath: "github.com/lib/pq", typeName: errorTypeName, extract: pqCodeField},                      // v1.2.0
	{pkgPath: "github.com/go-sql-driver/mysql", typeName: "MySQLError", extract: mysqlNumberField},     // v1.9.3
	{pkgPath: "modernc.org/sqlite", typeName: errorTypeName, extract: sqliteCodeMethod},                // v1.39.1
	{pkgPath: "github.com/mattn/go-sqlite3", typeName: errorTypeName, extract: mattnExtendedCodeField}, // v1.14.50
}

const errorTypeName = "Error"

// pqCodeField reads lib/pq's `Code ErrorCode` (a string type). Used only when
// the type has no SQLState method, i.e. lib/pq < v1.10.9.
func pqCodeField(_ error, v reflect.Value) (dbCode, bool) {
	f := v.FieldByName("Code")
	if !f.IsValid() || f.Kind() != reflect.String || f.String() == "" {
		return dbCode{}, false
	}
	return dbCode{kind: codeSQLState, state: f.String()}, true
}

// mysqlNumberField reads MySQLError{Number uint16; SQLState [5]byte}.
func mysqlNumberField(_ error, v reflect.Value) (dbCode, bool) {
	n := v.FieldByName("Number")
	if !n.IsValid() || n.Kind() < reflect.Uint || n.Kind() > reflect.Uint64 || n.Uint() == 0 {
		return dbCode{}, false
	}
	code := dbCode{kind: codeMySQL, number: int(n.Uint())} //nolint:gosec // MySQL error numbers are uint16
	if s := v.FieldByName("SQLState"); s.IsValid() && s.CanInterface() {
		if state, ok := s.Interface().([5]byte); ok && state != [5]byte{} {
			code.state = string(state[:])
		}
	}
	return code, true
}

// sqliteCodeMethod reads modernc.org/sqlite's Code() int, which returns the
// extended result code (the driver enables extended codes on every connection).
func sqliteCodeMethod(err error, _ reflect.Value) (dbCode, bool) {
	c, ok := err.(interface{ Code() int })
	if !ok {
		return dbCode{}, false
	}
	return sqliteCode(c.Code())
}

// mattnExtendedCodeField reads mattn/go-sqlite3's `ExtendedCode ErrNoExtended`.
func mattnExtendedCodeField(_ error, v reflect.Value) (dbCode, bool) {
	f := v.FieldByName("ExtendedCode")
	if !f.IsValid() || f.Kind() < reflect.Int || f.Kind() > reflect.Int64 {
		return dbCode{}, false
	}
	return sqliteCode(int(f.Int()))
}

// sqliteCode rejects codes that do not identify a class: 0 (no error) and the
// bare primary SQLITE_CONSTRAINT, which leaves the constraint kind to the text.
func sqliteCode(code int) (dbCode, bool) {
	if code == 0 || code == sqliteConstraint {
		return dbCode{}, false
	}
	return dbCode{kind: codeSQLite, number: code}, true
}

// codeOf returns the code carried by err itself (not by wrapped errors).
func codeOf(err error, drivers []driverShape) (dbCode, bool) {
	v := reflect.ValueOf(err)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return dbCode{}, false // a typed nil: calling its methods may panic
		}
		v = v.Elem()
	}

	if s, ok := err.(interface{ SQLState() string }); ok {
		if state := s.SQLState(); state != "" {
			return dbCode{kind: codeSQLState, state: state}, true
		}
		return dbCode{}, false
	}

	if v.Kind() != reflect.Struct {
		return dbCode{}, false
	}
	t := v.Type()
	for _, d := range drivers {
		if t.Name() == d.typeName && t.PkgPath() == d.pkgPath {
			return d.extract(err, v)
		}
	}
	return dbCode{}, false
}

// findCode walks the error tree in errors.As order and returns the first code.
func findCode(err error, drivers []driverShape) (dbCode, bool) {
	for err != nil {
		if code, ok := codeOf(err, drivers); ok {
			return code, true
		}
		switch x := err.(type) { //nolint:errorlint // this is the tree walk errors.As would do
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, e := range x.Unwrap() {
				if code, ok := findCode(e, drivers); ok {
					return code, true
				}
			}
			return dbCode{}, false
		default:
			return dbCode{}, false
		}
	}
	return dbCode{}, false
}

// classifyWith reports whether err belongs to class c: by the first driver
// code in the error tree, or by message text when the tree holds no code.
func classifyWith(err error, c *errClass, drivers []driverShape) bool {
	if err == nil {
		return false
	}
	if code, ok := findCode(err, drivers); ok {
		return c.matchCode(code)
	}
	return c.matchText(err.Error())
}

func classify(err error, c *errClass) bool {
	return classifyWith(err, c, knownDrivers)
}
