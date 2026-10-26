//go:build integration

package test

import (
	"go/types"
	"os"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

const relicaModule = "github.com/coregx/relica"

// unreachableAllowed lists internal code that no public API reaches on
// purpose, by package path or by function (ssa.Function.String()), each with
// its card. Everything else under internal/ must be reachable from package
// relica: code users cannot reach is dead weight that the docs then tend to
// describe as a feature (AUDIT-001 part B).
var unreachableAllowed = map[string]string{
	relicaModule + "/internal/optimizer":          "OPT-001: optimizer groundwork, not public yet",
	relicaModule + "/internal/security":           "BUG-006: validator and audit log not reachable from the public API",
	relicaModule + "/internal/core.WithOptimizer": "OPT-001: the option that would enable the optimizer",
	relicaModule + "/internal/core.WithValidator": "BUG-006: the option that would enable the validator",
	relicaModule + "/internal/core.WithAuditLog":  "BUG-006: the option that would enable the audit log",
}

// TestReachability_InternalCodeHasAPublicPath runs Rapid Type Analysis from
// every exported function and method of package relica (and every package
// initializer) and reports internal functions it does not reach.
//
// Generic functions (One[T], All[T], Scalar[T]) are not roots: RTA needs an
// instantiation. They delegate to non-generic methods that are roots, so code
// reached only through a generic function would be reported here — none is
// today; if that changes, add an instantiation as a root rather than an
// allowlist entry.
func TestReachability_InternalCodeHasAPublicPath(t *testing.T) {
	if selectedDB != "" && selectedDB != dialectSQLite {
		t.Skipf("RELICA_TEST_DB=%s: database-independent, runs in the sqlite job", selectedDB)
	}

	// GOEXPERIMENT is cleared so the analysis sees the standard library CI
	// builds: with jsonv2 on, encoding/json pulls in encoding/json/v2, whose
	// generics x/tools v0.48's SSA builder panics on.
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Env: append(os.Environ(), "GOEXPERIMENT=")}
	pkgs, err := packages.Load(cfg, relicaModule+"/...")
	if err != nil {
		t.Fatal(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("packages failed to load")
	}
	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	var roots []*ssa.Function
	for _, p := range ssaPkgs {
		if p == nil {
			continue
		}
		if init := p.Func("init"); init != nil {
			roots = append(roots, init)
		}
		if p.Pkg.Path() != relicaModule {
			continue
		}
		for _, mem := range p.Members {
			switch m := mem.(type) {
			case *ssa.Function:
				if m.Object() != nil && m.Object().Exported() && m.TypeParams().Len() == 0 {
					roots = append(roots, m)
				}
			case *ssa.Type:
				if !m.Object().Exported() {
					continue
				}
				// Unexported methods are roots too: users build these values
				// (HashExp{...}, SplitAcronyms()) and convert them to the sealed
				// interfaces (Expression, SnakeOption) in their own code, which
				// the analysis does not see; the library then calls the
				// unexported method through the interface.
				for _, typ := range []types.Type{m.Type(), types.NewPointer(m.Type())} {
					mset := prog.MethodSets.MethodSet(typ)
					for i := 0; i < mset.Len(); i++ {
						sel := mset.At(i)
						if fn := prog.MethodValue(sel); fn != nil {
							roots = append(roots, fn)
						}
					}
				}
			}
		}
	}
	if len(roots) < 100 {
		t.Fatalf("found %d roots in package relica, want at least 100: the load is broken", len(roots))
	}

	reachable := rta.Analyze(roots, false).Reachable

	var unreachable []string
	used := map[string]bool{}
	for fn := range ssautil.AllFunctions(prog) {
		if fn.Pkg == nil || fn.Synthetic != "" || fn.Parent() != nil {
			continue // wrappers, closures (reached with their parent)
		}
		path := fn.Pkg.Pkg.Path()
		if !strings.HasPrefix(path, relicaModule+"/internal/") {
			continue
		}
		if _, ok := reachable[fn]; ok {
			continue
		}
		if _, ok := unreachableAllowed[path]; ok {
			used[path] = true
			continue
		}
		if _, ok := unreachableAllowed[fn.String()]; ok {
			used[fn.String()] = true
			continue
		}
		pos := prog.Fset.Position(fn.Pos())
		unreachable = append(unreachable, fn.String()+"  ("+shortPos(pos.Filename)+")")
	}
	// An entry nothing needs any more (the code was wired up or deleted)
	// would hide the next unreachable function in that package: remove it.
	for key, reason := range unreachableAllowed {
		if !used[key] {
			t.Errorf("unreachableAllowed[%q] (%s) matches no unreachable function: remove the entry", key, reason)
		}
	}
	sort.Strings(unreachable)
	for _, u := range unreachable {
		t.Errorf("not reachable from the public API: %s", u)
	}
	if len(unreachable) > 0 {
		t.Logf("%d internal functions have no public path: call them from the API, delete them, "+
			"or add them to unreachableAllowed with a card", len(unreachable))
	}
}

func shortPos(file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	if i := strings.Index(file, "/internal/"); i >= 0 {
		return file[i+1:]
	}
	return file
}
