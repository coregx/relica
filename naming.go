package relica

import (
	"reflect"

	"github.com/coregx/relica/internal/core"
)

// Naming derives SQL names when the Go code gives none: the table of a model
// without a TableName() method, and the column of a struct field without a db
// tag (ADR-018).
//
// The zero value is the default, Snake(). A nil func means "default for that
// kind". A func must be deterministic and safe for concurrent use: results are
// cached per type per DB. Returning "" refuses the name, and building the query
// fails with an error naming the type or field — so
//
//	relica.Naming{Column: func(relica.ColumnSource) string { return "" }}
//
// means "every field must carry a db tag".
//
// Precedence: an explicit Table("x") on the query > TableName() on the model
// (value or pointer receiver) > Naming.Table; a db:"col" tag > Naming.Column.
// db:"-" skips a field; db:"pk" (or db:",pk") marks the primary key and takes
// its column from Naming.Column.
type Naming struct {
	Table  func(TableSource) string
	Column func(ColumnSource) string
}

// TableSource describes the model whose table name is derived.
type TableSource struct {
	Type reflect.Type // the model's struct type
}

// ColumnSource describes the struct field whose column name is derived.
type ColumnSource struct {
	Model reflect.Type        // top-level model type
	Owner reflect.Type        // struct that declares Field (== Model unless reached through embedding)
	Field reflect.StructField // as returned by Owner.Field(i); the full tag is available
	Index []int               // path from Model, usable with Model.FieldByIndex
}

// WithNaming sets how the DB derives table and column names. Applies to Open
// and WrapDB; transactions share the DB's Naming.
//
// Example — keep v0.19 names for untagged fields while migrating:
//
//	db, err := relica.Open("postgres", dsn, relica.WithNaming(relica.Naming{
//	    Column: func(c relica.ColumnSource) string { return c.Field.Name },
//	}))
func WithNaming(n Naming) Option {
	return Option{core.WithNaming(n.core())}
}

func (n Naming) core() core.Naming {
	var c core.Naming
	if n.Table != nil {
		c.Table = func(ts core.TableSource) string { return n.Table(TableSource(ts)) }
	}
	if n.Column != nil {
		c.Column = func(cs core.ColumnSource) string { return n.Column(ColumnSource(cs)) }
	}
	return c
}

func namingFromCore(c core.Naming) Naming {
	return Naming{
		Table:  func(ts TableSource) string { return c.Table(core.TableSource(ts)) },
		Column: func(cs ColumnSource) string { return c.Column(core.ColumnSource(cs)) },
	}
}

// SnakeOption configures Snake. Every CaseOption is also a SnakeOption.
type SnakeOption interface {
	snakeOption() core.SnakeOption
}

// CaseOption configures SnakeCase and Snake.
type CaseOption struct {
	c core.CaseOption
}

func (o CaseOption) snakeOption() core.SnakeOption { return o.c }

type tableOption struct {
	o core.SnakeOption
}

func (o tableOption) snakeOption() core.SnakeOption { return o.o }

// Snake returns the snake_case Naming. With no options it is ozzo-dbx's rule
// (regexp `([^A-Z_])([A-Z])` → "${1}_$2", then lower case): an underscore
// before an ASCII capital that follows anything but an ASCII capital or '_'.
// Tables are singular and unprefixed:
//
//	UserProfile → user_profile    UserID → user_id    HTTPLog → httplog
//	IPv4Address → ipv4_address    S3Bucket → s3_bucket
//
// SplitAcronyms and Initialisms select the other rules; TablePrefix and
// PluralTables affect derived table names only.
func Snake(opts ...SnakeOption) Naming {
	c := make([]core.SnakeOption, 0, len(opts))
	for _, o := range opts {
		if o != nil {
			c = append(c, o.snakeOption())
		}
	}
	return namingFromCore(core.Snake(c...))
}

// SnakeCase converts one Go name with the Snake rules selected by opts, for
// use inside a custom Naming.
func SnakeCase(name string, opts ...CaseOption) string {
	c := make([]core.CaseOption, len(opts))
	for i, o := range opts {
		c[i] = o.c
	}
	return core.SnakeCase(name, c...)
}

// SplitAcronyms also starts a word at the last capital of a run of capitals
// that is followed by a lower-case letter:
//
//	HTTPLog → http_log    MyHTTPServer → my_http_server    IPv4Address → i_pv4_address
func SplitAcronyms() CaseOption { return CaseOption{core.SplitAcronyms()} }

// Initialisms keeps the listed initialisms whole where SplitAcronyms would
// split them before a lower-case letter, and implies SplitAcronyms. It
// replaces the list; GoInitialisms() is Go's standard one:
//
//	Snake(Initialisms(GoInitialisms()...)): IPv4Address → ipv4_address, UserIDs → user_ids
//
// Only whole runs of capitals are compared, never substrings (HIDE → hide). A
// run that is itself listed is not extended (MyHTTPServer → my_http_server
// although HTTPS is listed), so UIDs → ui_ds: use a db tag for such names.
func Initialisms(words ...string) CaseOption { return CaseOption{core.Initialisms(words...)} }

// GoInitialisms returns a copy of Go's initialism list (staticcheck ST1003
// default, honnef.co/go/tools v0.8.2): API, HTTP, ID, JSON, URL, UUID, …
func GoInitialisms() []string { return core.GoInitialisms() }

// TablePrefix prepends p to derived table names (never to TableName()
// results). A trailing "schema." is quoted per part: TablePrefix("app.") →
// "app"."user".
func TablePrefix(p string) SnakeOption { return tableOption{core.TablePrefix(p)} }

// PluralTables applies f to derived table names after the snake_case
// conversion and before TablePrefix. Relica ships no English pluralization.
func PluralTables(f func(string) string) SnakeOption { return tableOption{core.PluralTables(f)} }

// ScanMode selects how strictly result columns must match struct fields when
// a query scans into a struct (One, All, generic One[T]/All[T], Model).
type ScanMode int

const (
	// ScanRequireFields (default): a struct field without a matching result
	// column is an error — it would otherwise stay zero silently. When the
	// query lists its columns (Select("id", "name")) only those are required.
	// Fields tagged db:"-" are never required. Extra result columns are ignored.
	ScanRequireFields ScanMode = iota
	// ScanLenient is ozzo-dbx's behavior: unmatched fields keep their value,
	// unmatched columns are discarded, no error.
	ScanLenient
	// ScanStrict is ScanRequireFields plus an error for a result column that
	// matches no field.
	ScanStrict
)

// WithScanMode sets the DB's default ScanMode; SelectQuery.ScanMode and
// Query.ScanMode override it per query.
func WithScanMode(m ScanMode) Option { return Option{core.WithScanMode(core.ScanMode(m))} }
