package relica_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestPublicAPI_NoInternalTypes enforces ADR-016: package relica declares every
// exported type itself, and no exported signature mentions an internal package.
//
// A type alias to an internal type (type HashExp = core.HashExp) renders on
// pkg.go.dev as an empty declaration, and a method returning an internal type
// (Unwrap() *core.DB) hands callers the whole unversioned internal API. Both
// slipped in repeatedly between v0.4.0 and v0.18.0 because nothing checked them.
//
// Sentinel error values (var ErrNotFound = core.ErrNotFound) are allowed: their
// declared type is error, and identity must be shared with the internal package
// for errors.Is to work.
func TestPublicAPI_NoInternalTypes(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	var violations []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		violations = append(violations, inspectPublicAPI(fset, f)...)
	}

	sort.Strings(violations)
	for _, v := range violations {
		t.Error(v)
	}
	if len(violations) > 0 {
		t.Logf("%d violation(s). Declare the type in package relica (see docs ADR-016).", len(violations))
	}
}

// TestInspectPublicAPI proves the guard above catches every violation class and
// accepts the allowed patterns — otherwise a green run would prove nothing.
func TestInspectPublicAPI(t *testing.T) {
	const header = "package relica\nimport (\n\t\"errors\"\n\t\"github.com/coregx/relica/internal/core\"\n\tlg \"github.com/coregx/relica/internal/logger\"\n)\nvar _ = errors.New\n"
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"alias to internal", "type X = core.X", 1},
		{"alias to stdlib is still an alias", "type X = errors.X", 1},
		{"renamed internal import", "type L interface{ Log(lg.Level) }", 1},
		{"func result", "func F() *core.DB { return nil }", 1},
		{"func param", "func F(o core.Option) {}", 1},
		{"method of exported type", "type T struct{}\nfunc (T) M() core.X { return core.X{} }", 1},
		{"exported field", "type T struct{ F core.X }", 1},
		{"embedded internal type", "type T struct{ *core.DB }", 1},
		{"exported interface method", "type I interface{ M(core.X) }", 1},
		{"func type", "type H func(core.Event)", 1},
		{"map value type", "type M map[string]core.X", 1},
		{"var with internal type", "var V core.X", 1},

		{"unexported field allowed", "type T struct{ db *core.DB }", 0},
		{"unexported interface method allowed", "type I interface{ coreExpression() core.Expression }", 0},
		{"method of unexported type allowed", "type t struct{}\nfunc (t) Build(core.Dialect) {}", 0},
		{"unexported func allowed", "func toCore(e core.Expression) core.Expression { return e }", 0},
		{"sentinel value allowed", "var ErrNotFound = core.ErrNotFound", 0},
		{"defined type over stdlib", "type M map[string]any", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "x.go", header+tt.src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			got := inspectPublicAPI(fset, f)
			if len(got) != tt.want {
				t.Errorf("got %d violation(s), want %d: %v", len(got), tt.want, got)
			}
		})
	}
}

// inspectPublicAPI returns one message per exported declaration in f that is an
// alias or whose type expression selects from an internal package.
func inspectPublicAPI(fset *token.FileSet, f *ast.File) []string {
	internal := internalImports(f)
	var out []string
	report := func(pos token.Pos, format string, args ...any) {
		out = append(out, fset.Position(pos).String()+": "+fmt.Sprintf(format, args...))
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.VAR || d.Tok == token.CONST {
				// Values may come from internal packages (sentinel errors);
				// an explicitly declared internal *type* may not.
				for _, spec := range d.Specs {
					vs := spec.(*ast.ValueSpec)
					if vs.Type == nil || !anyExported(vs.Names) {
						continue
					}
					for _, ref := range internalRefs(vs.Type, internal, true) {
						report(vs.Pos(), "exported %s %s has internal type %s", d.Tok, vs.Names[0].Name, ref)
					}
				}
				continue
			}
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts := spec.(*ast.TypeSpec)
				if !ts.Name.IsExported() {
					continue
				}
				if ts.Assign.IsValid() {
					report(ts.Pos(), "exported type alias %s = %s", ts.Name.Name, exprString(ts.Type))
					continue
				}
				for _, ref := range internalRefs(ts.Type, internal, true) {
					report(ts.Pos(), "exported type %s uses internal type %s", ts.Name.Name, ref)
				}
			}

		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := receiverName(d.Recv.List[0].Type)
				if !ast.IsExported(recv) {
					continue // methods of unexported types are not public API
				}
				name = recv + "." + name
			}
			for _, ref := range internalRefs(d.Type, internal, false) {
				report(d.Pos(), "exported func %s uses internal type %s", name, ref)
			}
		}
	}
	return out
}

// internalImports maps the local package name of every internal import to true.
func internalImports(f *ast.File) map[string]bool {
	m := map[string]bool{}
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if !strings.Contains(path, "/internal/") {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		m[name] = true
	}
	return m
}

// internalRefs lists selector expressions pkg.Name inside expr where pkg is an
// internal import. With exportedFieldsOnly, unexported struct fields and
// unexported interface methods are skipped: they are implementation detail.
func internalRefs(expr ast.Expr, internal map[string]bool, exportedFieldsOnly bool) []string {
	var refs []string
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.StructType:
			walkFields(x.Fields, exportedFieldsOnly, walk)
			return false
		case *ast.InterfaceType:
			walkFields(x.Methods, exportedFieldsOnly, walk)
			return false
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && internal[id.Name] {
				refs = append(refs, id.Name+"."+x.Sel.Name)
			}
			return false
		}
		return true
	}
	ast.Inspect(expr, walk)
	return refs
}

func walkFields(fields *ast.FieldList, exportedOnly bool, walk func(ast.Node) bool) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		if exportedOnly && len(field.Names) > 0 && !anyExported(field.Names) {
			continue
		}
		if exportedOnly && len(field.Names) == 0 && !embeddedExported(field.Type) {
			continue
		}
		ast.Inspect(field.Type, walk)
	}
}

func anyExported(names []*ast.Ident) bool {
	for _, n := range names {
		if n.IsExported() {
			return true
		}
	}
	return false
}

// embeddedExported reports whether an embedded field is visible to callers.
// An embedded internal type (core.X) is exported by name and therefore checked.
func embeddedExported(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.StarExpr:
		return embeddedExported(x.X)
	case *ast.Ident:
		return x.IsExported()
	case *ast.SelectorExpr:
		return x.Sel.IsExported()
	}
	return true
}

func receiverName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.StarExpr:
		return receiverName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return receiverName(x.X)
	case *ast.IndexListExpr:
		return receiverName(x.X)
	}
	return ""
}

func exprString(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return "*" + exprString(x.X)
	}
	return "…"
}
