// Package securecheck provides an analyzer that reports calls to
// secure.Value.Raw, which expose the underlying secret value.
package securecheck

import (
	"go/types"
	"path"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// pkgMarker exists only so its package path can be read at run time.
type pkgMarker struct{}

// securePkgPath and unsecurePkgPath come from this analyzer's own package path
// instead of a hard-coded module path. securecheck lives at <base>/securecheck,
// so secure is its parent and unsecure is its sibling.
var (
	securePkgPath   = path.Dir(reflect.TypeFor[pkgMarker]().PkgPath())
	unsecurePkgPath = path.Join(securePkgPath, "unsecure")
)

// Analyzer reports calls to secure.Value.Raw, which return the underlying secret
// value and so may leak it. It runs over SSA, so calls through a method value
// (f := v.Raw; f()) are caught too. Calls inside the unsecure package are
// skipped: that package is the allowed, easy-to-review way to unwrap a value.
var Analyzer = &analysis.Analyzer{
	Name:     "securecheck",
	Doc:      "report calls to secure.Value.Raw that expose the secret value",
	Requires: []*analysis.Analyzer{buildssa.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	// The unsecure package unwraps Value through Raw on purpose; its calls are
	// the allowed way out, so do not flag them.
	if pass.Pkg.Path() == unsecurePkgPath {
		return nil, nil
	}

	// Find the exact *types.Func for secure.Value.Raw from type info, so we
	// match by identity, not by name. If the package being checked does not
	// import secure, there is nothing to do.
	raw := lookupRawMethod(pass.Pkg)
	if raw == nil {
		return nil, nil
	}

	ssaPkg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	for _, fn := range ssaPkg.SrcFuncs {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				call, ok := instr.(*ssa.Call)
				if !ok {
					continue
				}
				// StaticCallee finds the target of direct calls and method
				// values (bound-method closures), but not dynamic interface
				// calls.
				callee := call.Common().StaticCallee()
				if callee == nil || !isRawMethod(callee, raw) {
					continue
				}
				// Some SSA calls have no source position; fall back to the
				// enclosing function so the report still points somewhere useful.
				pos := call.Pos()
				if !pos.IsValid() {
					pos = fn.Pos()
				}
				pass.Reportf(pos, "secure.Value.Raw exposes the underlying secret value; use unsecure.Value if intentional")
			}
		}
	}

	return nil, nil
}

// isRawMethod tells whether the SSA function fn is secure.Value.Raw. fn may be a
// generic method with its type filled in, or a bound-method wrapper; Object maps
// both to the declared method, and Origin maps a filled-in method to its origin.
func isRawMethod(fn *ssa.Function, raw *types.Func) bool {
	obj, ok := fn.Object().(*types.Func)
	if !ok {
		return false
	}
	return obj.Origin() == raw
}

// lookupRawMethod returns the *types.Func of secure.Value.Raw as seen from pkg,
// or nil when pkg does not import secure or the method is missing.
func lookupRawMethod(pkg *types.Package) *types.Func {
	for _, imp := range pkg.Imports() {
		if imp.Path() != securePkgPath {
			continue
		}
		obj, ok := imp.Scope().Lookup("Value").(*types.TypeName)
		if !ok {
			return nil
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			return nil
		}
		for m := range named.Methods() {
			if m.Name() == "Raw" {
				return m
			}
		}
		return nil
	}
	return nil
}
