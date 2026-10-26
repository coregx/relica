package relica_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// unwrappedCoreOptions lists internal/core options that package relica does
// not expose on purpose, each with its card. Every other core.With* option
// must be reachable through a public wrapper: an option users cannot pass is
// dead code that the docs then tend to describe (AUDIT-001 part B).
var unwrappedCoreOptions = map[string]string{
	"WithOptimizer": "OPT-001: optimizer groundwork, not public yet",
	"WithValidator": "BUG-006: SQL validator not reachable from the public API",
	"WithAuditLog":  "BUG-006: audit log not reachable from the public API",
}

func TestCoreOptions_HavePublicWrappers(t *testing.T) {
	fset := token.NewFileSet()
	coreFiles, err := filepath.Glob(filepath.Join("internal", "core", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var options []string
	for _, p := range coreFiles {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && strings.HasPrefix(fd.Name.Name, "With") {
				options = append(options, fd.Name.Name)
			}
		}
	}
	if len(options) < 10 {
		t.Fatalf("found %d core.With* options, want at least 10: the source parse is broken", len(options))
	}

	var public strings.Builder
	rootFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range rootFiles {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		public.Write(data)
	}

	sort.Strings(options)
	for _, name := range options {
		used := strings.Contains(public.String(), "core."+name+"(")
		reason, allowed := unwrappedCoreOptions[name]
		switch {
		case !used && !allowed:
			t.Errorf("core.%s has no public wrapper in package relica: wrap it or add it to unwrappedCoreOptions with its card", name)
		case used && allowed:
			t.Errorf("core.%s is wrapped now; remove it from unwrappedCoreOptions (%s)", name, reason)
		}
	}
}
