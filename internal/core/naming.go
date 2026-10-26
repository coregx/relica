package core

import (
	"reflect"
	"strings"
	"unicode"
)

// Naming derives SQL names when the Go code gives none: the table of a model
// without TableName(), the column of a field without a db tag (ADR-018).
// The zero value is the default (Snake()). A nil func means "default for that
// kind". A func must be deterministic and safe for concurrent use; results are
// cached per type per DB. Returning "" refuses the name: building the query
// fails with an error naming the type or field.
type Naming struct {
	Table  func(TableSource) string
	Column func(ColumnSource) string
}

// TableSource describes the model whose table name is derived.
type TableSource struct {
	Type reflect.Type // the model's struct type
}

// ColumnSource describes the field whose column name is derived.
type ColumnSource struct {
	Model reflect.Type        // top-level model type
	Owner reflect.Type        // struct that declares Field (== Model unless reached through embedding)
	Field reflect.StructField // as returned by Owner.Field(i); full tag available
	Index []int               // path from Model, usable with Model.FieldByIndex
}

var defaultNaming = Snake()

func (n Naming) table(ts TableSource) string {
	if n.Table == nil {
		return defaultNaming.Table(ts)
	}
	return n.Table(ts)
}

func (n Naming) column(cs ColumnSource) string {
	if n.Column == nil {
		return defaultNaming.Column(cs)
	}
	return n.Column(cs)
}

type caseConfig struct {
	split       bool
	initialisms map[string]bool
}

type snakeConfig struct {
	caseConfig
	prefix string
	plural func(string) string
}

// SnakeOption configures Snake. Every CaseOption is also a SnakeOption.
type SnakeOption interface {
	applySnake(*snakeConfig)
}

// CaseOption configures SnakeCase (and Snake).
type CaseOption struct {
	apply func(*caseConfig)
}

func (o CaseOption) applySnake(c *snakeConfig) {
	if o.apply != nil {
		o.apply(&c.caseConfig)
	}
}

type tableOption func(*snakeConfig)

func (o tableOption) applySnake(c *snakeConfig) {
	if o != nil {
		o(c)
	}
}

// SplitAcronyms also starts a word at the last capital of a run of capitals that
// is followed by a lower-case letter: HTTPLog → http_log, MyHTTPServer →
// my_http_server.
func SplitAcronyms() CaseOption {
	return CaseOption{apply: func(c *caseConfig) { c.split = true }}
}

// Initialisms replaces the list of initialisms kept whole when SplitAcronyms
// would split them before a lower-case letter: with "IP" listed, IPv4Address →
// ipv4_address instead of i_pv4_address. It implies SplitAcronyms. Only whole
// runs of capitals are compared, never substrings (HIDE stays hide). A run that
// is itself listed is never extended (MyHTTPServer → my_http_server although
// HTTPS is listed), so UIDs → ui_ds: use a db tag there.
func Initialisms(words ...string) CaseOption {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[strings.ToUpper(w)] = true
	}
	return CaseOption{apply: func(c *caseConfig) {
		c.split = true
		c.initialisms = set
	}}
}

// goInitialisms is staticcheck's ST1003 default list (honnef.co/go/tools
// v0.8.2, config.DefaultConfig.Initialisms). Frozen: a change renames columns.
var goInitialisms = []string{
	"ACL", "API", "ASCII", "CPU", "CSS", "DNS",
	"EOF", "GUID", "HTML", "HTTP", "HTTPS", "ID",
	"IP", "JSON", "QPS", "RAM", "RPC", "SLA",
	"SMTP", "SQL", "SSH", "TCP", "TLS", "TTL",
	"UDP", "UI", "GID", "UID", "UUID", "URI",
	"URL", "UTF8", "VM", "XML", "XMPP", "XSRF",
	"XSS", "SIP", "RTP", "AMQP", "DB", "TS",
}

// GoInitialisms returns a copy of the initialisms of Go's naming convention
// (staticcheck ST1003 default list), for Initialisms(GoInitialisms()...).
func GoInitialisms() []string {
	return append([]string(nil), goInitialisms...)
}

// TablePrefix prepends p to derived table names (not to TableName() results).
// A trailing "schema." is quoted per part.
func TablePrefix(p string) SnakeOption {
	return tableOption(func(c *snakeConfig) { c.prefix = p })
}

// PluralTables applies f to derived table names after SnakeCase and before
// TablePrefix. Relica has no built-in English rules.
func PluralTables(f func(string) string) SnakeOption {
	return tableOption(func(c *snakeConfig) { c.plural = f })
}

// Snake returns the snake_case Naming. With no options it is ozzo-dbx's rule:
// an underscore before an ASCII capital that follows a rune other than an ASCII
// capital or '_', then lower case; tables are singular and unprefixed.
func Snake(opts ...SnakeOption) Naming {
	var cfg snakeConfig
	for _, o := range opts {
		if o != nil {
			o.applySnake(&cfg)
		}
	}
	cc := cfg.caseConfig
	return Naming{
		Table: func(ts TableSource) string {
			name := typeBaseName(ts.Type)
			if name == "" {
				return ""
			}
			name = snakeCase(name, cc)
			if cfg.plural != nil {
				name = cfg.plural(name)
			}
			return cfg.prefix + name
		},
		Column: func(cs ColumnSource) string {
			return snakeCase(cs.Field.Name, cc)
		},
	}
}

// typeBaseName is the type's name without type arguments ("Page[int]" → "Page").
func typeBaseName(t reflect.Type) string {
	if t == nil {
		return ""
	}
	name := t.Name()
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	return name
}

// SnakeCase converts a Go name with the Snake rules selected by opts.
func SnakeCase(name string, opts ...CaseOption) string {
	var cc caseConfig
	for _, o := range opts {
		if o.apply != nil {
			o.apply(&cc)
		}
	}
	return snakeCase(name, cc)
}

func isASCIIUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

func snakeCase(name string, cc caseConfig) string {
	rs := []rune(name)
	var b strings.Builder
	b.Grow(len(name) + 4)
	runStart := 0 // start of the current run of ASCII capitals
	for i, r := range rs {
		if i == 0 || !isASCIIUpper(rs[i-1]) {
			runStart = i
		}
		if i > 0 && isASCIIUpper(r) && wordStart(rs, i, runStart, cc) {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	// strings.ToLower on the whole string, as ozzo-dbx does (special Unicode cases).
	return strings.ToLower(b.String())
}

// wordStart reports whether the capital rs[i] (i > 0) starts a word.
func wordStart(rs []rune, i, runStart int, cc caseConfig) bool {
	prev := rs[i-1]
	if !isASCIIUpper(prev) {
		return prev != '_'
	}
	// Inside a run of capitals: only SplitAcronyms splits, before the last
	// capital when a lower-case letter follows.
	if !cc.split || i+1 >= len(rs) || !unicode.IsLower(rs[i+1]) {
		return false
	}
	if cc.initialisms != nil {
		before := string(rs[runStart:i])
		with := string(rs[runStart : i+1])
		if cc.initialisms[with] && !cc.initialisms[before] {
			return false
		}
	}
	return true
}
