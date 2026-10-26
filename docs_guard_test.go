package relica_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The public documentation must not name API that does not exist (AUDIT-001
// phase 4): guides once described a SQL validator, an optimizer and
// db.StmtCacheStats() that users could not call. These tests read every
// public Markdown file and llms.txt; docs/dev is private and not checked.

// docsAllowed lists relica.X names that may appear although X is not
// exported, with the files they may appear in and why.
var docsAllowed = map[string]struct {
	files  []string
	reason string
}{
	"NewDB": {[]string{"docs/MIGRATION_GUIDE.md"}, "removed in v0.20.0; named in the migration path"},
}

// publicDocs returns the public documentation files, relative to the module root.
func publicDocs(t *testing.T) []string {
	t.Helper()
	files := []string{"llms.txt"}
	root, err := filepath.Glob("*.md")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, root...)
	err = filepath.WalkDir("docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && filepath.ToSlash(path) == "docs/dev" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// exportedNames returns the package-level exported identifiers of package
// relica, read from its non-test source files.
func exportedNames(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	fset := token.NewFileSet()
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					names[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							names[s.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								names[n.Name] = true
							}
						}
					}
				}
			}
		}
	}
	return names
}

// relicaRef matches relica.X; a trailing * (relica.Is*) names a family of
// functions, not one, and is skipped.
var relicaRef = regexp.MustCompile(`\brelica\.([A-Z][A-Za-z0-9_]*)(\*?)`)

func TestDocs_RelicaNamesExist(t *testing.T) {
	names := exportedNames(t)
	if !names["Open"] || !names["DB"] {
		t.Fatalf("exportedNames found no Open/DB (%d names): the source parse is broken", len(names))
	}
	missing := map[string][]string{} // name → files
	for _, file := range publicDocs(t) {
		if file == "CHANGELOG.md" {
			continue // history: names of past versions
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range relicaRef.FindAllStringSubmatch(string(data), -1) {
			name := m[1]
			if names[name] || m[2] == "*" {
				continue
			}
			if a, ok := docsAllowed[name]; ok && contains(a.files, file) {
				continue
			}
			if !contains(missing[name], file) {
				missing[name] = append(missing[name], file)
			}
		}
	}
	keys := make([]string, 0, len(missing))
	for k := range missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Errorf("relica.%s is not exported by package relica; named in %s", k, strings.Join(missing[k], ", "))
	}
}

// methodNames returns the exported method names of the given receiver type
// (pointer or value receiver) in package relica.
func methodNames(t *testing.T, typeName string) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	fset := token.NewFileSet()
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || !fd.Name.IsExported() {
				continue
			}
			recv := fd.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok && id.Name == typeName {
				names[fd.Name.Name] = true
			}
		}
	}
	return names
}

// foreignMethods lists, per file, db./tx. methods of other libraries shown
// for comparison, which *relica.DB and *relica.Tx do not have.
var foreignMethods = map[string]struct {
	names  []string
	reason string
}{
	"docs/guides/MIGRATION_FROM_GORM.md": {[]string{"Raw", "AutoMigrate", "Preload", "Find", "First", "Where", "Create",
		"Order", "Transaction", "Exec"}, "GORM code being migrated"},
	"docs/guides/MIGRATION_FROM_SQLX.md": {[]string{"Get", "GetContext", "SelectContext", "Exec", "NamedExec", "Rebind",
		"Beginx", "Preparex"}, "sqlx code being migrated"},
	"docs/MIGRATION_GUIDE.md": {[]string{"Unwrap"}, "v0.4–v0.18 API, removed in v0.19.0 (ADR-016)"},
}

// TestDocs_DBAndTxMethodsExist checks db.X( and tx.X( calls in the public
// docs against the methods of *relica.DB and *relica.Tx: the guides used to
// call db.StmtCacheStats() and db.SetQueryLogger(), which never existed.
// A field access (r.db.With(ctx), a user's dbcontext type) is not checked.
// Variables named db/tx that are *sql.DB or *sql.Tx in an example would
// show up here too; the docs use sqlDB/sqlTx for those.
func TestDocs_DBAndTxMethodsExist(t *testing.T) {
	methods := map[string]map[string]bool{"db": methodNames(t, "DB"), "tx": methodNames(t, "Tx")}
	if !methods["db"]["Select"] || !methods["tx"]["Commit"] {
		t.Fatal("methodNames found no DB.Select/Tx.Commit: the source parse is broken")
	}
	call := regexp.MustCompile(`(?:^|[^.\w])(db|tx)\.([A-Z][A-Za-z0-9_]*)\(`)
	for _, file := range publicDocs(t) {
		if file == "CHANGELOG.md" {
			continue // history: methods of past versions
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range call.FindAllStringSubmatch(line, -1) {
				if !methods[m[1]][m[2]] && !contains(foreignMethods[file].names, m[2]) {
					t.Errorf("%s:%d calls %s.%s(, which *relica.%s does not have: %s",
						file, i+1, m[1], m[2], map[string]string{"db": "DB", "tx": "Tx"}[m[1]], strings.TrimSpace(line))
				}
			}
		}
	}
}

var (
	// columnCall matches a Select/AndSelect/GroupBy/OrderBy call whose
	// argument list has no nested parentheses.
	columnCall = regexp.MustCompile(`\b(Select|AndSelect|GroupBy|OrderBy)\(([^()]*)\)`)
	stringArg  = regexp.MustCompile(`"([^"]*)"`)
	ident      = `[\p{L}_][\p{L}\p{N}_$]*`
	columnRef  = regexp.MustCompile(`^` + ident + `(\.` + ident + `)*(\.\*)?$`)
	aliasedRef = regexp.MustCompile(`(?i)^(` + ident + `(\.` + ident + `)*) AS ` + ident + `$`)
	orderedRef = regexp.MustCompile(`(?i)^(` + ident + `(\.` + ident + `)*) (ASC|DESC)$`)
)

// columnArgOK reports whether arg is what method takes: Select/AndSelect a
// column (optionally "AS alias") or "*", GroupBy a column, OrderBy a column
// with an optional direction; an argument with "(" is used as is everywhere.
func columnArgOK(method, arg string) bool {
	if arg == "..." || strings.Contains(arg, "(") || columnRef.MatchString(arg) {
		return true
	}
	switch method {
	case "Select", "AndSelect":
		return arg == "*" || aliasedRef.MatchString(arg)
	case "OrderBy":
		return orderedRef.MatchString(arg)
	}
	return false
}

// columnDocsForeign lists files whose Select/GroupBy/OrderBy calls belong to
// another library shown for comparison.
var columnDocsForeign = map[string]string{
	"docs/guides/MIGRATION_FROM_GORM.md": "GORM's Select/Order take SQL fragments",
	"docs/guides/MIGRATION_FROM_SQLX.md": "sqlx's Select takes a destination and a query",
	"docs/MIGRATION_GUIDE.md":            "shows the old code that v0.20.0 rejects",
}

// TestDocs_ColumnArgumentsAreColumns keeps expressions out of the column
// methods in examples: Select("1 as level") and Select("h.level + 1") quote
// the text as one name, and OrderBy rejects anything beyond "col [ASC|DESC]".
// Expressions go through SelectExpr/OrderByExpr/GroupByExpr. Counter-examples
// are skipped: a call written as "not Select(...)", and the line right after
// a "WRONG" or "❌" comment; so is the "..." placeholder.
func TestDocs_ColumnArgumentsAreColumns(t *testing.T) {
	for _, file := range publicDocs(t) {
		if file == "CHANGELOG.md" || columnDocsForeign[file] != "" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if i > 0 && (strings.Contains(lines[i-1], "WRONG") || strings.Contains(lines[i-1], "❌")) {
				continue
			}
			for _, loc := range columnCall.FindAllStringSubmatchIndex(line, -1) {
				before := strings.ToLower(line[:loc[0]])
				if strings.HasSuffix(before, "not ") || strings.HasSuffix(before, "not `") {
					continue
				}
				method, args := line[loc[2]:loc[3]], line[loc[4]:loc[5]]
				for _, m := range stringArg.FindAllStringSubmatch(args, -1) {
					if !columnArgOK(method, m[1]) {
						t.Errorf("%s:%d %s(%q) is not a column argument; use %sExpr: %s",
							file, i+1, method, m[1], strings.TrimPrefix(method, "And"), strings.TrimSpace(line))
					}
				}
			}
		}
	}
}

// TestDocs_NoInternalImports keeps internal/ packages out of user-facing
// docs: users cannot import them. CONTRIBUTING describes the layout, and the
// v0.4 migration tells users how to find the internal imports they had.
func TestDocs_NoInternalImports(t *testing.T) {
	for _, file := range publicDocs(t) {
		if file == "CONTRIBUTING.md" || file == "docs/MIGRATION_GUIDE.md" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "github.com/coregx/relica/internal") {
				t.Errorf("%s:%d names an internal package: %s", file, i+1, strings.TrimSpace(line))
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
