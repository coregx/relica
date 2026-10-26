// Package dialects provides database-specific SQL dialect implementations for
// PostgreSQL, MySQL, and SQLite, handling identifier quoting, placeholders, and
// UPSERT operations.
package dialects

import "fmt"

// Dialect defines database-specific behaviors.
type Dialect interface {
	QuoteIdentifier(string) string
	Placeholder(int) string
	// UpsertSQL renders the conflict clause appended to an INSERT ... VALUES
	// statement. The caller has validated u (ADR-019); the dialect only renders.
	UpsertSQL(u Upsert) string
}

// Upsert describes a validated conflict clause. Every column name is already
// quoted by the dialect's QuoteIdentifier.
type Upsert struct {
	// Target is the conflict target; empty only with DoNothing on PostgreSQL/SQLite
	// (any constraint) and on MySQL (which always uses every unique key).
	Target []string
	// Update lists the columns set from the proposed row. Empty with DoNothing.
	Update []string
	// DoNothing keeps the existing row.
	DoNothing bool
	// NoOp is the column a MySQL DoNothing assigns to itself (MySQL has no DO NOTHING).
	NoOp string
	// RowAlias selects the MySQL 8.0.19+ form "VALUES (...) AS excluded" with
	// "col = excluded.col" instead of the default "col = VALUES(col)".
	RowAlias bool
}

var dialects = make(map[string]Dialect)

// RegisterDialect registers a database dialect by driver name.
func RegisterDialect(name string, d Dialect) {
	dialects[name] = d
}

// GetDialect retrieves a registered dialect by driver name.
// Panics with an actionable message if the dialect is not registered.
// Supported built-in names: "postgres", "postgresql", "pgx", "mysql", "sqlite", "sqlite3".
// Custom dialects can be added via RegisterDialect.
func GetDialect(name string) Dialect {
	d, err := LookupDialect(name)
	if err != nil {
		panic(err.Error())
	}
	return d
}

// LookupDialect returns the dialect registered for a driver name, or an error
// naming the supported ones.
func LookupDialect(name string) (Dialect, error) {
	if d, ok := dialects[name]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("relica: no SQL dialect for driver %q; supported driver names: "+
		"postgres, postgresql, pgx, mysql, sqlite, sqlite3", name)
}
