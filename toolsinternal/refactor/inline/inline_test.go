// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inline_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
	"github.com/tenntenn/exp/toolsinternal/diff"
	"github.com/tenntenn/exp/toolsinternal/expect"
	"github.com/tenntenn/exp/toolsinternal/refactor/inline"
	"github.com/tenntenn/exp/toolsinternal/testenv"
	"github.com/tenntenn/exp/toolsinternal/testfiles"
	"golang.org/x/tools/txtar"
)

// TestData executes test scenarios specified by files in testdata/*.txtar.
// Each txtar file describes two sets of files, some containing Go source
// and others expected results.
//
// The Go source files and go.mod are parsed and type-checked as a Go module.
// Some of these files contain marker comments (in a form described below) describing
// the inlinings to perform and whether they should succeed or fail. A marker
// indicating success refers to another file in the txtar, not a .go
// file, that should contain the contents of the first file after inlining.
//
// The marker format for success is
//
//	@inline(re"pat", wantfile)
//
// The first call in the marker's line that matches pat is inlined, and the contents
// of the resulting file must match the contents of wantfile.
//
// The marker format for failure is
//
//	@inline(re"pat", re"errpat")
//
// The first argument selects the call for inlining as before, and the second
// is a regular expression that must match the text of resulting error.
func TestData(t *testing.T) {
	testenv.NeedsGoPackages(t)

	files, err := filepath.Glob("testdata/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			// The few tests that use cgo should be in
			// files whose name includes "cgo".
			if strings.Contains(t.Name(), "cgo") {
				testenv.NeedsTool(t, "cgo")
			}

			// Extract archive to temporary tree.
			ar, err := txtar.ParseFile(file)
			if err != nil {
				t.Fatal(err)
			}
			fs, err := txtar.FS(ar)
			if err != nil {
				t.Fatal(err)
			}
			dir := testfiles.CopyToTmp(t, fs)

			// Load packages.
			cfg := &packages.Config{
				Dir:  dir,
				Mode: packages.LoadAllSyntax,
				Env: append(os.Environ(),
					"GO111MODULES=on",
					"GOPATH=",
					"GOWORK=off",
					"GOPROXY=off"),
			}
			pkgs, err := packages.Load(cfg, "./...")
			if err != nil {
				t.Errorf("Load: %v", err)
			}
			// Report parse/type errors; they may be benign.
			packages.Visit(pkgs, nil, func(pkg *packages.Package) {
				for _, err := range pkg.Errors {
					t.Log(err)
				}
			})

			// Process @inline notes in comments in initial packages.
			for _, pkg := range pkgs {
				for _, file := range pkg.Syntax {
					// Read file content (for @inline regexp, and inliner).
					content, err := os.ReadFile(pkg.Fset.File(file.FileStart).Name())
					if err != nil {
						t.Error(err)
						continue
					}

					// Read and process @inline notes.
					notes, err := expect.ExtractGo(pkg.Fset, file)
					if err != nil {
						t.Errorf("parsing notes in %q: %v", pkg.Fset.File(file.FileStart).Name(), err)
						continue
					}
					for _, note := range notes {
						posn := pkg.Fset.PositionFor(note.Pos, false)
						if note.Name != "inline" {
							t.Errorf("%s: invalid marker @%s", posn, note.Name)
							continue
						}
						if nargs := len(note.Args); nargs != 2 {
							t.Errorf("@inline: want 2 args, got %d", nargs)
							continue
						}
						pattern, ok := note.Args[0].(*regexp.Regexp)
						if !ok {
							t.Errorf("%s: @inline(rx, want): want regular expression rx", posn)
							continue
						}

						// want is a []byte (success) or *Regexp (failure)
						var want any
						switch x := note.Args[1].(type) {
						case string, expect.Identifier:
							name := fmt.Sprint(x)
							for _, file := range ar.Files {
								if file.Name == name {
									want = file.Data
									break
								}
							}
							if want == nil {
								t.Errorf("%s: @inline(rx, want): archive entry %q not found", posn, x)
								continue
							}
						case *regexp.Regexp:
							want = x
						default:
							t.Errorf("%s: @inline(rx, want): want file name (to assert success) or error message regexp (to assert failure)", posn)
							continue
						}
						if err := doInlineNote(t.Logf, pkg, file, content, pattern, posn, want); err != nil {
							t.Errorf("%s: @inline(%v, %v): %v", posn, note.Args[0], note.Args[1], err)
							continue
						}
					}
				}
			}
		})
	}
}

// doInlineNote executes an assertion specified by a single
// @inline(re"pattern", want) note in a comment. It finds the first
// match of regular expression 'pattern' on the same line, finds the
// innermost enclosing CallExpr, and inlines it.
//
// Finally it checks that, on success, the transformed file is equal
// to want (a []byte), or on failure that the error message matches
// want (a *Regexp).
func doInlineNote(logf func(string, ...any), pkg *packages.Package, file *ast.File, content []byte, pattern *regexp.Regexp, posn token.Position, want any) error {
	// Find extent of pattern match within commented line.
	var startPos, endPos token.Pos
	{
		tokFile := pkg.Fset.File(file.FileStart)
		lineStartOffset := int(tokFile.LineStart(posn.Line)) - tokFile.Base()
		line := content[lineStartOffset:]
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		matches := pattern.FindSubmatchIndex(line)
		var start, end int // offsets
		switch len(matches) {
		case 2:
			// no subgroups: return the range of the regexp expression
			start, end = matches[0], matches[1]
		case 4:
			// one subgroup: return its range
			start, end = matches[2], matches[3]
		default:
			return fmt.Errorf("invalid location regexp %q: expect either 0 or 1 subgroups, got %d",
				pattern, len(matches)/2-1)
		}
		startPos = tokFile.Pos(lineStartOffset + start)
		endPos = tokFile.Pos(lineStartOffset + end)
	}

	// Find innermost call enclosing the pattern match.
	var caller *inline.Caller
	{
		path, _ := astutil.PathEnclosingInterval(file, startPos, endPos)
		for _, n := range path {
			if call, ok := n.(*ast.CallExpr); ok {
				caller = &inline.Caller{
					Fset:    pkg.Fset,
					Types:   pkg.Types,
					Info:    pkg.TypesInfo,
					File:    file,
					Call:    call,
					Content: content,
				}
				break
			}
		}
		if caller == nil {
			return fmt.Errorf("no enclosing call")
		}
	}

	// Is it a static function call?
	fn := typeutil.StaticCallee(caller.Info, caller.Call)
	if fn == nil {
		return fmt.Errorf("cannot inline: not a static call")
	}

	// Find callee function.
	var calleePkg *packages.Package
	{
		// Is the call within the package?
		if fn.Pkg() == caller.Types {
			calleePkg = pkg // same as caller
		} else {
			// Different package. Load it now.
			// (The primary load loaded all dependencies,
			// but we choose to load it again, with
			// a distinct token.FileSet and types.Importer,
			// to keep the implementation honest.)
			cfg := &packages.Config{
				// TODO(adonovan): get the original module root more cleanly
				Dir:  filepath.Dir(filepath.Dir(pkg.GoFiles[0])),
				Fset: token.NewFileSet(),
				Mode: packages.LoadSyntax,
			}
			roots, err := packages.Load(cfg, fn.Pkg().Path())
			if err != nil {
				return fmt.Errorf("loading callee package: %v", err)
			}
			if packages.PrintErrors(roots) > 0 {
				return fmt.Errorf("callee package had errors") // (see log)
			}
			calleePkg = roots[0]
		}
	}

	calleeDecl, err := findFuncByPosition(calleePkg, caller.Fset.PositionFor(fn.Pos(), false))
	if err != nil {
		return err
	}

	// Do the inlining. For the purposes of the test,
	// AnalyzeCallee and Inline are a single operation.
	res, err := func() (*inline.Result, error) {
		filename := calleePkg.Fset.File(calleeDecl.Pos()).Name()
		content, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		callee, err := inline.AnalyzeCallee(
			logf,
			calleePkg.Fset,
			calleePkg.Types,
			calleePkg.TypesInfo,
			calleeDecl,
			content)
		if err != nil {
			return nil, err
		}

		if err := checkTranscode(callee); err != nil {
			return nil, err
		}

		check := checkNoMutation(caller.File)
		defer check()
		return inline.Inline(caller, callee, &inline.Options{Logf: logf})
	}()
	if err != nil {
		if wantRE, ok := want.(*regexp.Regexp); ok {
			if !wantRE.MatchString(err.Error()) {
				return fmt.Errorf("Inline failed with wrong error: %v (want error matching %q)", err, want)
			}
			return nil // expected error
		}
		return fmt.Errorf("Inline failed: %v", err) // success was expected
	}

	// Inline succeeded.
	got := res.Content
	if want, ok := want.([]byte); ok {
		got = append(bytes.TrimSpace(got), '\n')
		want = append(bytes.TrimSpace(want), '\n')
		// If the "want" file begins "...", it need only be a substring of the "got" result,
		// rather than an exact match.
		if rest, ok := bytes.CutPrefix(want, []byte("...\n")); ok {
			want = rest
			if !bytes.Contains(got, want) {
				return fmt.Errorf("Inline returned wrong output:\n%s\nWant substring:\n%s", got, want)
			}
		} else {
			if diff := diff.Unified("want", "got", string(want), string(got)); diff != "" {
				return fmt.Errorf("Inline returned wrong output:\n%s\nWant:\n%s\nDiff:\n%s",
					got, want, diff)
			}
		}
		return nil
	}
	return fmt.Errorf("Inline succeeded unexpectedly: want error matching %q, got <<%s>>", want, got)
}

// findFuncByPosition returns the FuncDecl at the specified (package-agnostic) position.
func findFuncByPosition(pkg *packages.Package, posn token.Position) (*ast.FuncDecl, error) {
	same := func(decl *ast.FuncDecl) bool {
		// We can't rely on columns in export data:
		// some variants replace it with 1.
		// We can't expect file names to have the same prefix.
		// export data for go1.20 std packages have  $GOROOT written in
		// them, so how are we supposed to find the source? Yuck!
		// Ugh. need to samefile? Nope $GOROOT just won't work
		// This is highly client specific anyway.
		posn2 := pkg.Fset.PositionFor(decl.Name.Pos(), false)
		return posn.Filename == posn2.Filename &&
			posn.Line == posn2.Line
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if decl, ok := decl.(*ast.FuncDecl); ok && same(decl) {
				return decl, nil
			}
		}
	}
	return nil, fmt.Errorf("can't find FuncDecl at %v in package %q", posn, pkg.PkgPath)
}

// Each callee must declare a function or method named f,
// and each caller must call it.
const funcName = "f"

// A testcase is an item in a table-driven test.
//
// The table-driven tests are less flexible, but enable more compact
// expression of single-package test cases than is possible with the
// txtar notation.
//
// TODO(adonovan): improve coverage of the cross product of each
// strategy with the checklist of concerns enumerated in the package
// doc comment.
type testcase struct {
	descr          string // description; substrings enable options (e.g. "IgnoreEffects")
	callee, caller string // Go source files (sans package decl) of caller, callee
	want           string // expected new portion of caller file, or "error: regexp"
}

func TestErrors(t *testing.T) {
	runTests(t, []testcase{
		{
			"Inference of type parameters is not yet supported.",
			`func f[T any](x T) T { return x }`,
			`var _ = f(0)`,
			`error: type parameter inference is not yet supported`,
		},
		{
			"Methods on generic types are not yet supported.",
			`type G[T any] struct{}; func (G[T]) f(x T) T { return x }`,
			`var _ = G[int]{}.f(0)`,
			`error: generic methods not yet supported`,
		},
	})
}

func TestBasics(t *testing.T) {
	runTests(t, []testcase{
		{
			"Basic",
			`func f(x int) int { return x }`,
			`var _ = f(0)`,
			`var _ = 0`,
		},
		{
			"Empty body, no arg effects.",
			`func f(x, y int) {}`,
			`func _() { f(1, 2) }`,
			`func _() {}`,
		},
		{
			"Empty body, some arg effects.",
			`func f(x, y, z int) {}`,
			`func _() { f(1, recover().(int), 3) }`,
			`func _() { _ = recover().(int) }`,
		},
		{
			"Non-duplicable arguments are not substituted even if pure.",
			`func f(s string, i int) { print(s, s, i, i) }`,
			`func _() { f("hi", 0)  }`,
			`func _() {
	var s string = "hi"
	print(s, s, 0, 0)
}`,
		},
		{
			"Workaround for T(x) misformatting (#63362).",
			`func f(ch <-chan int) { <-ch }`,
			`func _(ch chan int) { f(ch) }`,
			`func _(ch chan int) { <-(<-chan int)(ch) }`,
		},
		{
			// (a regression test for unnecessary braces)
			"In block elision, blank decls don't count when computing name conflicts.",
			`func f(x int) { var _ = x; var _ = 3 }`,
			`func _() { var _ = 1; f(2) }`,
			`func _() {
	var _ = 1
	var _ = 2
	var _ = 3
}`,
		},
		{
			// (a regression test for a missing conversion)
			"Implicit return conversions are inserted in expr-context reduction.",
			`func f(x int) error { return nil }`,
			`func _() { if err := f(0); err != nil {} }`,
			`func _() {
	if err := error(nil); err != nil {
	}
}`,
		},
		{
			"Explicit type parameters.",
			`func f[T any](x T) T { return x }`,
			`var _ = f[int](0)`,
			// TODO(jba): remove the unnecessary conversion.
			`var _ = int(0)`,
		},
	})
}

func TestDuplicable(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		runTests(t, []testcase{
			{
				"Empty strings are duplicable.",
				`func f(s string) { print(s, s) }`,
				`func _() { f("")  }`,
				`func _() { print("", "") }`,
			},
			{
				"Non-empty string literals are not duplicable.",
				`func f(s string) { print(s, s) }`,
				`func _() { f("hi")  }`,
				`func _() {
	var s string = "hi"
	print(s, s)
}`,
			},
			{
				"Empty array literals are duplicable.",
				`func f(a [2]int) { print(a, a) }`,
				`func _() { f([2]int{})  }`,
				`func _() { print([2]int{}, [2]int{}) }`,
			},
			{
				"Non-empty array literals are not duplicable.",
				`func f(a [2]int) { print(a, a) }`,
				`func _() { f([2]int{1, 2})  }`,
				`func _() {
	var a [2]int = [2]int{1, 2}
	print(a, a)
}`,
			},
			{
				"Empty struct literals are duplicable.",
				`func f(s S) { print(s, s) }; type S struct { x int }`,
				`func _() { f(S{})  }`,
				`func _() { print(S{}, S{}) }`,
			},
			{
				"Non-empty struct literals are not duplicable.",
				`func f(s S) { print(s, s) }; type S struct { x int }`,
				`func _() { f(S{x: 1})  }`,
				`func _() {
	var s S = S{x: 1}
	print(s, s)
}`,
			},
		})
	})

	t.Run("conversions", func(t *testing.T) {
		runTests(t, []testcase{
			{
				"Conversions to integer are duplicable.",
				`func f(i int) { print(i, i) }`,
				`func _() { var i int8 = 1; f(int(i))  }`,
				`func _() { var i int8 = 1; print(int(i), int(i)) }`,
			},
			{
				"Implicit conversions from underlying types are duplicable.",
				`func f(i I) { print(i, i) }; type I int; func print(args ...any) {}`,
				`func _() { f(1)  }`,
				`func _() { print(I(1), I(1)) }`,
			},
			{
				"Conversions to array are duplicable.",
				`func f(a [2]int) { print(a, a) }; type A [2]int`,
				`func _() { var a A; f([2]int(a)) }`,
				`func _() { var a A; print([2]int(a), [2]int(a)) }`,
			},
			{
				"Conversions from array are duplicable.",
				`func f(a A) { print(a, a) }; type A [2]int`,
				`func _() { var a [2]int; f(A(a)) }`,
				`func _() { var a [2]int; print(A(a), A(a)) }`,
			},
			{
				"Conversions from byte slice to string are duplicable.",
				`func f(s string) { print(s, s) }`,
				`func _() { var b []byte; f(string(b)) }`,
				`func _() { var b []byte; print(string(b), string(b)) }`,
			},
			{
				"Conversions from string to byte slice are not duplicable.",
				`func f(b []byte) { print(b, b) }`,
				`func _() { var s string; f([]byte(s)) }`,
				`func _() {
	var s string
	var b []byte = []byte(s)
	print(b, b)
}`,
			},
			{
				"Conversions from string to uint8 slice are not duplicable.",
				`func f(b []uint8) { print(b, b) }`,
				`func _() { var s string; f([]uint8(s)) }`,
				`func _() {
	var s string
	var b []uint8 = []uint8(s)
	print(b, b)
}`,
			},
			{
				"Conversions from string to rune slice are not duplicable.",
				`func f(r []rune) { print(r, r) }`,
				`func _() { var s string; f([]rune(s)) }`,
				`func _() {
	var s string
	var r []rune = []rune(s)
	print(r, r)
}`,
			},
			{
				"Conversions from string to named type with underlying byte slice are not duplicable.",
				`func f(b B) { print(b, b) }; type B []byte`,
				`func _() { var s string; f(B(s)) }`,
				`func _() {
	var s string
	var b B = B(s)
	print(b, b)
}`,
			},
			{
				"Conversions from string to named type of string are duplicable.",
				`func f(s S) { print(s, s) }; type S string`,
				`func _() { var s string; f(S(s)) }`,
				`func _() { var s string; print(S(s), S(s)) }`,
			},
			{
				"Built-in function calls are not duplicable.",
				`func f(i int) { print(i, i) }`,
				`func _() { f(len(""))  }`,
				`func _() {
	var i int = len("")
	print(i, i)
}`,
			},
			{
				"Built-in function calls are not duplicable.",
				`func f(c complex128) { print(c, c) }`,
				`func _() { f(complex(1.0, 2.0)) }`,
				`func _() {
	var c complex128 = complex(1.0, 2.0)
	print(c, c)
}`,
			},
			{
				"Non built-in function calls are not duplicable.",
				`func f(i int) { print(i, i) }
//go:noinline
func f1(i int) int { return i + 1 }`,
				`func _() { f(f1(1))  }`,
				`func _() {
	var i int = f1(1)
	print(i, i)
}`,
			},
			{
				"Conversions between function types are duplicable.",
				`func f(f F) { print(f, f) }; type F func(); func f1() {}`,
				`func _() { f(F(f1))  }`,
				`func _() { print(F(f1), F(f1)) }`,
			},
		})
	})
}

func TestExprStmtReduction(t *testing.T) {
	runTests(t, []testcase{
		{
			"A call in an unrestricted ExprStmt may be replaced by the body stmts.",
			`func f() { var _ = len("") }`,
			`func _() { f() }`,
			`func _() { var _ = len("") }`,
		},
		{
			"ExprStmts in the body of a switch case are unrestricted.",
			`func f() { x := 1; print(x) }`,
			`func _() { switch { case true: f() } }`,
			`func _() {
	switch {
	case true:
		x := 1
		print(x)
	}
}`,
		},
		{
			"ExprStmts in the body of a select case are unrestricted.",
			`func f() { x := 1; print(x) }`,
			`func _() { select { default: f() } }`,
			`func _() {
	select {
	default:
		x := 1
		print(x)
	}
}`,
		},
		{
			"Some ExprStmt contexts are restricted to simple statements.",
			`func f() { var _ = len("") }`,
			`func _(cond bool) { if f(); cond {} }`,
			`func _(cond bool) {
	if func() { var _ = len("") }(); cond {
	}
}`,
		},
		{
			"Braces must be preserved to avoid a name conflict (decl before).",
			`func f() { x := 1; print(x) }`,
			`func _() { x := 2; print(x); f() }`,
			`func _() {
	x := 2
	print(x)
	{
		x := 1
		print(x)
	}
}`,
		},
		{
			"Braces must be preserved to avoid a name conflict (decl after).",
			`func f() { x := 1; print(x) }`,
			`func _() { f(); x := 2; print(x) }`,
			`func _() {
	{
		x := 1
		print(x)
	}
	x := 2
	print(x)
}`,
		},
		{
			"Braces must be preserved to avoid a forward jump across a decl.",
			`func f() { x := 1; print(x) }`,
			`func _() { goto label; f(); label: }`,
			`func _() {
	goto label
	{
		x := 1
		print(x)
	}
label:
}`,
		},
	})
}

func TestPrecedenceParens(t *testing.T) {
	// Ensure that parens are inserted when (and only when) necessary
	// around the replacement for the call expression. (This is a special
	// case in the way the inliner uses a combination of AST formatting
	// for the call and text splicing for the rest of the file.)
	runTests(t, []testcase{
		{
			"Multiplication in addition context (no parens).",
			`func f(x, y int) int { return x * y }`,
			`func _() { _ = 1 + f(2, 3) }`,
			`func _() { _ = 1 + 2*3 }`,
		},
		{
			"Addition in multiplication context (parens).",
			`func f(x, y int) int { return x + y }`,
			`func _() { _ = 1 * f(2, 3) }`,
			`func _() { _ = 1 * (2 + 3) }`,
		},
		{
			"Addition in negation context (parens).",
			`func f(x, y int) int { return x + y }`,
			`func _() { _ = -f(1, 2) }`,
			`func _() { _ = -(1 + 2) }`,
		},
		{
			"Addition in call context (no parens).",
			`func f(x, y int) int { return x + y }`,
			`func _() { println(f(1, 2)) }`,
			`func _() { println(1 + 2) }`,
		},
		{
			"Addition in slice operand context (parens).",
			`func f(x, y string) string { return x + y }`,
			`func _() { _ = f("x",  "y")[1:2] }`,
			`func _() { _ = ("x" + "y")[1:2] }`,
		},
		{
			"String literal in slice operand context (no parens).",
			`func f(x string) string { return x }`,
			`func _() { _ = f("xy")[1:2] }`,
			`func _() { _ = "xy"[1:2] }`,
		},
	})
}

func TestSubstitution(t *testing.T) {
	runTests(t, []testcase{
		{
			"Arg to unref'd param can be eliminated if has no effects.",
			`func f(x, y int) {}; var global int`,
			`func _() { f(0, global) }`,
			`func _() {}`,
		},
		{
			"But not if it may contain last reference to a caller local var.",
			`func f(int) {}`,
			`func _() { var local int; f(local) }`,
			`func _() { var local int; _ = local }`,
		},
		{
			"Arguments that are used are detected",
			`func f(int) {}`,
			`func _() { var local int; _ = local; f(local) }`,
			`func _() { var local int; _ = local }`,
		},
		{
			"Arguments that are used by other arguments are detected",
			`func f(x, y int) { print(x) }`,
			`func _() { var z int; f(z, z) }`,
			`func _() { var z int; print(z) }`,
		},
		{
			"Arguments that are used by other variadic arguments are detected",
			`func f(x int, ys ...int) { print(ys) }`,
			`func _() { var z int; f(z, 1, 2, 3, z) }`,
			`func _() { var z int; print([]int{1, 2, 3, z}) }`,
		},
		{
			"Arguments that are used by other variadic arguments are detected, 2",
			`func f(x int, ys ...int) { print(ys) }`,
			`func _() { var z int; f(z) }`,
			`func _() {
	var z int
	var _ int = z
	print([]int{})
}`,
		},
		{
			"Function parameters are always used",
			`func f(int) {}`,
			`func _() {
	func(local int) {
		f(local)
	}(1)
}`,
			`func _() {
	func(local int) {

	}(1)
}`,
		},
		{
			"Regression test for detection of shadowing in nested functions.",
			`func f(x int) { _ = func() { y := 1; print(y); print(x) } }`,
			`func _(y int) { f(y) } `,
			`func _(y int) {
	var x int = y
	_ = func() { y := 1; print(y); print(x) }
}`,
		},
	})
}

func TestTailCallStrategy(t *testing.T) {
	runTests(t, []testcase{
		{
			"simple",
			`func f() int { return 1 }`,
			`func _() int { return f() }`,
			`func _() int { return 1 }`,
		},
		{
			"void",
			`func f() { println() }`,
			`func _() { f() }`,
			`func _() { println() }`,
		},
		{
			"void with defer", // => literalized
			`func f() { defer f(); println() }`,
			`func _() { f() }`,
			`func _() { func() { defer f(); println() }() }`,
		},
		// Tests for issue #63336:
		{
			"non-trivial return conversion (caller.sig = callee.sig)",
			`func f() error { if true { return nil } else { return e } }; var e struct{error}`,
			`func _() error { return f() }`,
			`func _() error {
	if true {
		return nil
	} else {
		return e
	}
}`,
		},
		{
			"non-trivial return conversion (caller.sig != callee.sig)",
			`func f() error { return E{} }; type E struct{error}`,
			`func _() any { return f() }`,
			`func _() any { return error(E{}) }`,
		},
	})
}

func TestSpreadCalls(t *testing.T) {
	runTests(t, []testcase{
		{
			"Edge case: cannot literalize spread method call.",
			`type I int
 			func g() (I, I)
			func (r I) f(x, y I) I {
				defer g() // force literalization
				return x + y + r
			}`,
			`func _() I { return recover().(I).f(g()) }`,
			`error: can't yet inline spread call to method`,
		},
		{
			"Spread argument evaluated for effect.",
			`func f(int, int) {}; func g() (int, int)`,
			`func _() { f(g())  }`,
			`func _() { _, _ = g() }`,
		},
		{
			"Edge case: receiver and spread argument, both evaluated for effect.",
			`type T int; func (T) f(int, int) {}; func g() (int, int)`,
			`func _() { T(0).f(g())  }`,
			`func _() {
	var (
		_    = T(0)
		_, _ = g()
	)
}`,
		},
		{
			"Spread call in return (#63398).",
			`func f() (int, error) { return 0, nil }`,
			`func _() (int, error) { return f() }`,
			`func _() (int, error) { return 0, nil }`,
		},
	})
}

func TestAssignmentCallStrategy(t *testing.T) {
	runTests(t, []testcase{
		{
			"splice: basic",
			`func f(x int) (int, int) { return x, 2 }`,
			`func _() { x, y := f(1); _, _ = x, y }`,
			`func _() { x, y := 1, 2; _, _ = x, y }`,
		},
		{
			"spread: basic",
			`func f(x int) (any, any) { return g() }; func g() (error, error) { return nil, nil }`,
			`func _() {
	var x any
	x, y := f(0)
	_, _ = x, y
}`,
			`func _() {
	var x any
	var y any
	x, y = g()
	_, _ = x, y
}`,
		},
		{
			"spread: free var conflict",
			`func f(x int) (any, any) { return g(x) }; func g(x int) (int, int) { return x, x }`,
			`func _() {
	y := 2
	{
		var x any
		x, y := f(y)
		_, _ = x, y
	}
}`,
			`func _() {
	y := 2
	{
		var x any
		x, y := func() (any, any) { return g(y) }()
		_, _ = x, y
	}
}`,
		},
		{
			"convert: basic",
			`func f(x int) (int32, int8) { return 1, 2 }`,
			`func _() {
	var x int32
  x, y := f(0)
	_, _ = x, y
}`,
			`func _() {
	var x int32
	x, y := 1, int8(2)
	_, _ = x, y
}`,
		},
		{
			"convert: rune and byte",
			`func f(x int) (rune, byte) { return 0, 0 }`,
			`func _() {
	x, y := f(0)
	_, _ = x, y
}`,
			`func _() {
	x, y := rune(0), byte(0)
	_, _ = x, y
}`,
		},
		{
			"convert: interface conversions",
			`func f(x int) (_, _ error) { return nil, nil }`,
			`func _() {
  x, y := f(0)
	_, _ = x, y
}`,
			`func _() {
	x, y := error(nil), error(nil)
	_, _ = x, y
}`,
		},
		{
			"convert: implicit nil conversions",
			`func f(x int) (_, _ error) { return nil, nil }`,
			`func _() { x, y := f(0); _, _ = x, y }`,
			`func _() { x, y := error(nil), error(nil); _, _ = x, y }`,
		},
		{
			"convert: pruning nil assignments left",
			`func f(x int) (_, _ error) { return nil, nil }`,
			`func _() { _, y := f(0); _ = y }`,
			`func _() { y := error(nil); _ = y }`,
		},
		{
			"convert: pruning nil assignments right",
			`func f(x int) (_, _ error) { return nil, nil }`,
			`func _() { x, _ := f(0); _ = x }`,
			`func _() { x := error(nil); _ = x }`,
		},
		{
			"convert: partial assign",
			`func f(x int) (_, _ error) { return nil, nil }`,
			`func _() {
	var x error
  x, y := f(0)
	_, _ = x, y
}`,
			`func _() {
	var x error
	x, y := nil, error(nil)
	_, _ = x, y
}`,
		},
		{
			"convert: single assignment left",
			`func f() int { return 0 }`,
			`func _() {
	x, y := f(), "hello"
	_, _ = x, y
}`,
			`func _() {
	x, y := 0, "hello"
	_, _ = x, y
}`,
		},
		{
			"convert: single assignment left with conversion",
			`func f() int32 { return 0 }`,
			`func _() {
	x, y := f(), "hello"
	_, _ = x, y
}`,
			`func _() {
	x, y := int32(0), "hello"
	_, _ = x, y
}`,
		},
		{
			"convert: single assignment right",
			`func f() int32 { return 0 }`,
			`func _() {
	x, y := "hello", f()
	_, _ = x, y
}`,
			`func _() {
	x, y := "hello", int32(0)
	_, _ = x, y
}`,
		},
		{
			"convert: single assignment middle",
			`func f() int32 { return 0 }`,
			`func _() {
	x, y, z := "hello", f(), 1.56
	_, _, _ = x, y, z
}`,
			`func _() {
	x, y, z := "hello", int32(0), 1.56
	_, _, _ = x, y, z
}`,
		},
	})
}

func TestVariadic(t *testing.T) {
	runTests(t, []testcase{
		{
			"Variadic cancellation (basic).",
			`func f(args ...any) { defer f(&args); println(args) }`,
			`func _(slice []any) { f(slice...) }`,
			`func _(slice []any) { func() { var args []any = slice; defer f(&args); println(args) }() }`,
		},
		{
			"Variadic cancellation (literalization with parameter elimination).",
			`func f(args ...any) { defer f(); println(args) }`,
			`func _(slice []any) { f(slice...) }`,
			`func _(slice []any) { func() { defer f(); println(slice) }() }`,
		},
		{
			"Variadic cancellation (reduction).",
			`func f(args ...any) { println(args) }`,
			`func _(slice []any) { f(slice...) }`,
			`func _(slice []any) { println(slice) }`,
		},
		{
			"Undo variadic elimination",
			`func f(args ...int) []int { return append([]int{1}, args...) }`,
			`func _(a, b int) { f(a, b) }`,
			`func _(a, b int) { _ = append([]int{1}, a, b) }`,
		},
		{
			"Variadic elimination (literalization).",
			`func f(x any, rest ...any) { defer println(x, rest) }`, // defer => literalization
			`func _() { f(1, 2, 3) }`,
			`func _() { func() { defer println(1, []any{2, 3}) }() }`,
		},
		{
			"Variadic elimination (reduction).",
			`func f(x int, rest ...int) { println(x, rest) }`,
			`func _() { f(1, 2, 3) }`,
			`func _() { println(1, []int{2, 3}) }`,
		},
		{
			"Spread call to variadic (1 arg, 1 param).",
			`func f(rest ...int) { println(rest) }; func g() (a, b int)`,
			`func _() { f(g()) }`,
			`func _() { func(rest ...int) { println(rest) }(g()) }`,
		},
		{
			"Spread call to variadic (1 arg, 2 params).",
			`func f(x int, rest ...int) { println(x, rest) }; func g() (a, b int)`,
			`func _() { f(g()) }`,
			`func _() { func(x int, rest ...int) { println(x, rest) }(g()) }`,
		},
		{
			"Spread call to variadic (1 arg, 3 params).",
			`func f(x, y int, rest ...int) { println(x, y, rest) }; func g() (a, b, c int)`,
			`func _() { f(g()) }`,
			`func _() { func(x, y int, rest ...int) { println(x, y, rest) }(g()) }`,
		},
	})
}

func TestParameterBindingDecl(t *testing.T) {
	runTests(t, []testcase{
		{
			"IncDec counts as assignment.",
			`func f(x int) { x++ }`,
			`func _() { f(1) }`,
			`func _() {
	var x int = 1
	x++
}`,
		},
		{
			"Binding declaration (x, y, z eliminated).",
			`func f(w, x, y any, z int) { println(w, y, z) }; func g(int) int`,
			`func _() { f(g(0), g(1), g(2), g(3)) }`,
			`func _() {
	var w, _ any = g(0), g(1)
	println(w, g(2), g(3))
}`,
		},
		{
			"Reduction of stmt-context call to { return exprs }, with substitution",
			`func f(ch chan int) int { return <-ch }; func g() chan int`,
			`func _() { f(g()) }`,
			`func _() { <-g() }`,
		},
		{
			// Same again, with callee effects:
			"Binding decl in reduction of stmt-context call to { return exprs }",
			`func f(x int) int { return <-h(g(2), x) }; func g(int) int; func h(int, int) chan int`,
			`func _() { f(g(1)) }`,
			`func _() {
	var x int = g(1)
	<-h(g(2), x)
}`,
		},
		{
			"No binding decl due to shadowing of int",
			`func f(int, y any, z int) { defer g(0); println(int, y, z) }; func g(int) int`,
			`func _() { f(g(1), g(2), g(3)) }`,
			`func _() { func(int, y any, z int) { defer g(0); println(int, y, z) }(g(1), g(2), g(3)) }`,
		},
		{
			"An indirect method selection (*x).g acts as a read.",
			`func f(x *T, y any) any { return x.g(y) }; type T struct{}; func (T) g(x any) any { return x }`,
			`func _(x *T) { f(x, recover()) }`,
			`func _(x *T) {
	var y any = recover()
	x.g(y)
}`,
		},
		{
			"A direct method selection x.g is pure.",
			`func f(x *T, y any) any { return x.g(y) }; type T struct{}; func (*T) g(x any) any { return x }`,
			`func _(x *T) { f(x, recover()) }`,
			`func _(x *T) { x.g(recover()) }`,
		},
		{
			"Literalization can make use of a binding decl (all params).",
			`func f(x, y int) int { defer println(); return y + x }; func g(int) int`,
			`func _() { println(f(g(1), g(2))) }`,
			`func _() { println(func() int { var x, y int = g(1), g(2); defer println(); return y + x }()) }`,
		},
		{
			"Literalization can make use of a binding decl (some params).",
			`func f(x, y int) int { z := y + x; defer println(); return z }; func g(int) int`,
			`func _() { println(f(g(1), g(2))) }`,
			`func _() { println(func() int { var x int = g(1); z := g(2) + x; defer println(); return z }()) }`,
		},
		{
			"Literalization can't yet use of a binding decl if named results.",
			`func f(x, y int) (z int) { z = y + x; defer println(); return }; func g(int) int`,
			`func _() { println(f(g(1), g(2))) }`,
			`func _() { println(func(x int) (z int) { z = g(2) + x; defer println(); return }(g(1))) }`,
		},
	})
}

func TestEmbeddedFields(t *testing.T) {
	runTests(t, []testcase{
		{
			"Embedded fields in x.f method selection (direct).",
			`type T int; func (t T) f() { print(t) }; type U struct{ T }`,
			`func _(u U) { u.f() }`,
			`func _(u U) { print(u.T) }`,
		},
		{
			"Embedded fields in x.f method selection (implicit *).",
			`type ( T int; U struct{*T}; V struct {U} ); func (t T) f() { print(t) }`,
			`func _(v V) { v.f() }`,
			`func _(v V) { print(*v.U.T) }`,
		},
		{
			"Embedded fields in x.f method selection (implicit &).",
			`type ( T int; U struct{T}; V struct {U} ); func (t *T) f() { print(t) }`,
			`func _(v V) { v.f() }`,
			`func _(v V) { print(&v.U.T) }`,
		},
		// Now the same tests again with T.f(recv).
		{
			"Embedded fields in T.f method selection.",
			`type T int; func (t T) f() { print(t) }; type U struct{ T }`,
			`func _(u U) { U.f(u) }`,
			`func _(u U) { print(u.T) }`,
		},
		{
			"Embedded fields in T.f method selection (implicit *).",
			`type ( T int; U struct{*T}; V struct {U} ); func (t T) f() { print(t) }`,
			`func _(v V) { V.f(v) }`,
			`func _(v V) { print(*v.U.T) }`,
		},
		{
			"Embedded fields in (*T).f method selection.",
			`type ( T int; U struct{T}; V struct {U} ); func (t *T) f() { print(t) }`,
			`func _(v V) { (*V).f(&v) }`,
			`func _(v V) { print(&(&v).U.T) }`,
		},
		{
			// x is a single-assign var, and x.f does not load through a pointer
			// (despite types.Selection.Indirect=true), so x is pure.
			"No binding decl is required for recv in method-to-method calls.",
			`type T struct{}; func (x *T) f() { g(); print(*x) }; func g()`,
			`func (x *T) _() { x.f() }`,
			`func (x *T) _() {
	g()
	print(*x)
}`,
		},
		{
			"Same, with implicit &recv.",
			`type T struct{}; func (x *T) f() { g(); print(*x) }; func g()`,
			`func (x T) _() { x.f() }`,
			`func (x T) _() {
	{
		var x *T = &x
		g()
		print(*x)
	}
}`,
		},
	})
}

func TestSubstitutionGroups(t *testing.T) {
	runTests(t, []testcase{
		{
			// b -> a
			"Basic",
			`func f(a, b int) { print(a, b) }`,
			`func _() { var a int; f(a, a) }`,
			`func _() { var a int; print(a, a) }`,
		},
		{
			// a <-> b
			"Cocycle",
			`func f(a, b int) { print(a, b) }`,
			`func _() { var a, b int; f(a+b, a+b) }`,
			`func _() { var a, b int; print(a+b, a+b) }`,
		},
		{
			// a <-> b
			// a -> c
			// Don't compute b as substitutable due to bad cycle traversal.
			"Middle cycle",
			`func f(a, b, c int) { var d int; print(a, b, c, d) }`,
			`func _() { var a, b, c, d int; f(a+b+c, a+b, d) }`,
			`func _() {
	var a, b, c, d int
	{
		var a, b, c int = a + b + c, a + b, d
		var d int
		print(a, b, c, d)
	}
}`,
		},
		{
			// a -> b
			// b -> c
			// b -> d
			// c
			//
			// Only c should be substitutable.
			"Singleton",
			`func f(a, b, c, d int) { var e int; print(a, b, c, d, e) }`,
			`func _() { var a, b, c, d, e int; f(a+b, c+d, c, e) }`,
			`func _() {
	var a, b, c, d, e int
	{
		var a, b, d int = a + b, c + d, e
		var e int
		print(a, b, c, d, e)
	}
}`,
		},
	})
}

func TestSubstitutionPreservesArgumentEffectOrder(t *testing.T) {
	runTests(t, []testcase{
		{
			"Arguments have effects, but parameters are evaluated in order.",
			`func f(a, b, c int) { print(a, b, c) }; func g(int) int`,
			`func _() { f(g(1), g(2), g(3)) }`,
			`func _() { print(g(1), g(2), g(3)) }`,
		},
		{
			"Arguments have effects, and parameters are evaluated out of order.",
			`func f(a, b, c int) { print(a, c, b) }; func g(int) int`,
			`func _() { f(g(1), g(2), g(3)) }`,
			`func _() {
	var a, b int = g(1), g(2)
	print(a, g(3), b)
}`,
		},
		{
			"Pure arguments may commute with argument that have effects.",
			`func f(a, b, c int) { print(a, c, b) }; func g(int) int`,
			`func _() { f(g(1), 2, g(3)) }`,
			`func _() { print(g(1), g(3), 2) }`,
		},
		{
			"Impure arguments may commute with each other.",
			`func f(a, b, c, d int) { print(a, c, b, d) }; func g(int) int; var x, y int`,
			`func _() { f(g(1), x, y, g(2)) }`,
			`func _() { print(g(1), y, x, g(2)) }`,
		},
		{
			"Impure arguments do not commute with arguments that have effects (1)",
			`func f(a, b, c, d int) { print(a, c, b, d) }; func g(int) int; var x, y int`,
			`func _() { f(g(1), g(2), y, g(3)) }`,
			`func _() {
	var a, b int = g(1), g(2)
	print(a, y, b, g(3))
}`,
		},
		{
			"Impure arguments do not commute with those that have effects (2).",
			`func f(a, b, c, d int) { print(a, c, b, d) }; func g(int) int; var x, y int`,
			`func _() { f(g(1), y, g(2), g(3)) }`,
			`func _() {
	var a, b int = g(1), y
	print(a, g(2), b, g(3))
}`,
		},
		{
			"Callee effects commute with pure arguments.",
			`func f(a, b, c int) { print(a, c, recover().(int), b) }; func g(int) int`,
			`func _() { f(g(1), 2, g(3)) }`,
			`func _() { print(g(1), g(3), recover().(int), 2) }`,
		},
		{
			"Callee reads may commute with impure arguments.",
			`func f(a, b int) { print(a, x, b) }; func g(int) int; var x, y int`,
			`func _() { f(g(1), y) }`,
			`func _() { print(g(1), x, y) }`,
		},
		{
			"All impure parameters preceding a read hazard must be kept.",
			`func f(a, b, c int) { print(a, b, recover().(int), c) }; var x, y, z int`,
			`func _() { f(x, y, z) }`,
			`func _() {
	var c int = z
	print(x, y, recover().(int), c)
}`,
		},
		{
			"All parameters preceding a write hazard must be kept.",
			`func f(a, b, c int) { print(a, b, recover().(int), c) }; func g(int) int; var x, y, z int`,
			`func _() { f(x, y, g(0))  }`,
			`func _() {
	var a, b, c int = x, y, g(0)
	print(a, b, recover().(int), c)
}`,
		},
		{
			"[W1 R0 W2 W4 R3] -- test case for second iteration of effect loop",
			`func f(a, b, c, d, e int) { print(b, a, c, e, d) }; func g(int) int; var x, y int`,
			`func _() { f(x, g(1), g(2), y, g(3))  }`,
			`func _() {
	var a, b, c, d int = x, g(1), g(2), y
	print(b, a, c, g(3), d)
}`,
		},
		{
			// In this example, the set() call is rejected as a substitution
			// candidate due to a shadowing conflict (z). This must entail that the
			// selection x.y (R) is also rejected, because it is lower numbered.
			//
			// Incidentally this program (which panics when executed) illustrates
			// that although effects occur left-to-right, read operations such
			// as x.y are not ordered wrt writes, depending on the compiler.
			// Changing x.y to identity(x).y forces the ordering and avoids the panic.
			"Hazards with args already rejected (e.g. due to shadowing) are detected too.",
			`func f(x, y int) (z int) { return x + y }; func set[T any](ptr *T, old, new T) int { println(old); *ptr = new; return 0; }`,
			`func _() { x := new(struct{ y int }); z := x; f(x.y, set(&x, z, nil)) }`,
			`func _() {
	x := new(struct{ y int })
	z := x
	{
		var x, y int = x.y, set(&x, z, nil)
		_ = x + y
	}
}`,
		},
		{
			// Rejection of a later parameter for reasons other than callee
			// effects (e.g. escape) may create hazards with lower-numbered
			// parameters that require them to be rejected too.
			"Hazards with already eliminated parameters (variant)",
			`func f(x, y int) { _ = &y }; func g(int) int`,
			`func _() { f(g(1), g(2)) }`,
			`func _() {
	var _, y int = g(1), g(2)
	_ = &y
}`,
		},
		{
			// In this case g(2) is rejected for substitution because it is
			// unreferenced but has effects, so parameter x must also be rejected
			// so that its argument v can be evaluated earlier in the binding decl.
			"Hazards with already eliminated parameters (unreferenced fx variant)",
			`func f(x, y int) { _ = x }; func g(int) int; var v int`,
			`func _() { f(v, g(2)) }`,
			`func _() {
	var x, _ int = v, g(2)
	_ = x
}`,
		},
		{
			"Defer f() evaluates f() before unknown effects",
			`func f(int, y any, z int) { defer println(int, y, z) }; func g(int) int`,
			`func _() { f(g(1), g(2), g(3)) }`,
			`func _() { func() { defer println(g(1), g(2), g(3)) }() }`,
		},
		{
			"Effects are ignored when IgnoreEffects",
			`func f(x, y int) { println(y, x) }; func g(int) int`,
			`func _() { f(g(1), g(2)) }`,
			`func _() { println(g(2), g(1)) }`,
		},
	})
}

func TestNamedResultVars(t *testing.T) {
	runTests(t, []testcase{
		{
			"Stmt-context call to {return g()} that mentions named result.",
			`func f() (x int) { return g(x) }; func g(int) int`,
			`func _() { f() }`,
			`func _() {
	var x int
	g(x)
}`,
		},
		{
			"Ditto, with binding decl again.",
			`func f(y string) (x int) { return x+x+len(y+y) }`,
			`func _() { f(".") }`,
			`func _() {
	var (
		y string = "."
		x int
	)
	_ = x + x + len(y+y)
}`,
		},

		{
			"Ditto, with binding decl (due to repeated y refs).",
			`func f(y string) (x string) { return x+y+y }`,
			`func _() { f(".") }`,
			`func _() {
	var (
		y string = "."
		x string
	)
	_ = x + y + y
}`,
		},
		{
			"Stmt-context call to {return binary} that mentions named result.",
			`func f() (x int) { return x+x }`,
			`func _() { f() }`,
			`func _() {
	var x int
	_ = x + x
}`,
		},
		{
			"Tail call to {return expr} that mentions named result.",
			`func f() (x int) { return x }`,
			`func _() int { return f() }`,
			`func _() int { return func() (x int) { return x }() }`,
		},
		{
			"Tail call to {return} that implicitly reads named result.",
			`func f() (x int) { return }`,
			`func _() int { return f() }`,
			`func _() int { return func() (x int) { return }() }`,
		},
		{
			"Spread-context call to {return expr} that mentions named result.",
			`func f() (x, y int) { return x, y }`,
			`func _() { var _, _ = f() }`,
			`func _() { var _, _ = func() (x, y int) { return x, y }() }`,
		},
		{
			"Shadowing in binding decl for named results => literalization.",
			`func f(y string) (x y) { return x+x+len(y+y) }; type y = int`,
			`func _() { f(".") }`,
			`func _() { func(y string) (x y) { return x + x + len(y+y) }(".") }`,
		},
	})
}

func TestSubstitutionPreservesParameterType(t *testing.T) {
	runTests(t, []testcase{
		{
			"Substitution preserves argument type (#63193).",
			`func f(x int16) { y := x; _ = (*int16)(&y) }`,
			`func _() { f(1) }`,
			`func _() {
	y := int16(1)
	_ = (*int16)(&y)
}`,
		},
		{
			"Same, with non-constant (unnamed to named struct) conversion.",
			`func f(x T) { y := x; _ = (*T)(&y) }; type T struct{}`,
			`func _() { f(struct{}{}) }`,
			`func _() {
	y := T(struct{}{})
	_ = (*T)(&y)
}`,
		},
		{
			"Same, with non-constant (chan to <-chan) conversion.",
			`func f(x T) { y := x; _ = (*T)(&y) }; type T = <-chan int; var ch chan int`,
			`func _() { f(ch) }`,
			`func _() {
	y := T(ch)
	_ = (*T)(&y)
}`,
		},
		{
			"Same, with untyped nil to typed nil conversion.",
			`func f(x *int) { y := x; _ = (**int)(&y) }`,
			`func _() { f(nil) }`,
			`func _() {
	y := (*int)(nil)
	_ = (**int)(&y)
}`,
		},
		{
			"Conversion of untyped int to named type is made explicit.",
			`type T int; func (x T) f() { x.g() }; func (T) g() {}`,
			`func _() { T.f(1) }`,
			`func _() { T(1).g() }`,
		},
		{
			"Implicit reference is made explicit outside of selector",
			`type T int; func (x *T) f() bool { return x == x.id() }; func (x *T) id() *T { return x }`,
			`func _() { var t T; _ = t.f() }`,
			`func _() { var t T; _ = &t == t.id() }`,
		},
		{
			"Implicit parenthesized reference is not made explicit in selector",
			`type T int; func (x *T) f() bool { return x == (x).id() }; func (x *T) id() *T { return x }`,
			`func _() { var t T; _ = t.f() }`,
			`func _() { var t T; _ = &t == (t).id() }`,
		},
		{
			"Implicit dereference is made explicit outside of selector", // TODO(rfindley): avoid unnecessary literalization here
			`type T int; func (x T) f() bool { return x == x.id() }; func (x T) id() T { return x }`,
			`func _() { var t *T; _ = t.f() }`,
			`func _() { var t *T; _ = func() bool { var x T = *t; return x == x.id() }() }`,
		},
		{
			"Check for shadowing error on type used in the conversion.",
			`func f(x T) { _ = &x == (*T)(nil) }; type T int16`,
			`func _() { type T bool; f(1) }`,
			`error: T.*shadowed.*by.*type`,
		},
	})
}

func TestRedundantConversions(t *testing.T) {
	runTests(t, []testcase{
		{
			"Type conversion must be added if the constant is untyped.",
			`func f(i int32) { print(i) }; func print(x any) {}`,
			`func _() { f(1)  }`,
			`func _() { print(int32(1)) }`,
		},
		{
			"Type conversion must not be added if the constant is typed.",
			`func f(i int32) { print(i) }; func print(x any) {}`,
			`func _() { f(int32(1))  }`,
			`func _() { print(int32(1)) }`,
		},
		{
			"No type conversion for argument to interface parameter",
			`type T int; func f(x any) { g(x) }; func g(any) {}`,
			`func _() { f(T(1)) }`,
			`func _() { g(T(1)) }`,
		},
		{
			"No type conversion for parenthesized argument to interface parameter",
			`type T int; func f(x any) { g((x)) }; func g(any) {}`,
			`func _() { f(T(1)) }`,
			`func _() { g((T(1))) }`,
		},
		{
			"Type conversion for argument to type parameter",
			`type T int; func f(x any) { g(x) }; func g[P any](P) {}`,
			`func _() { f(T(1)) }`,
			`func _() { g(any(T(1))) }`,
		},
		{
			"Strip redundant interface conversions",
			`type T interface{ M() }; func f(x any) { g(x) }; func g[P any](P) {}`,
			`func _() { f(T(nil)) }`,
			`func _() { g(any(nil)) }`,
		},
		{
			"No type conversion for argument to variadic interface parameter",
			`type T int; func f(x ...any) { g(x...) }; func g(...any) {}`,
			`func _() { f(T(1)) }`,
			`func _() { g(T(1)) }`,
		},
		{
			"Type conversion for variadic argument",
			`type T int; func f(x ...any) { g(x...) }; func g(...any) {}`,
			`func _() { f([]any{T(1)}...) }`,
			`func _() { g([]any{T(1)}...) }`,
		},
		{
			"Type conversion for argument to interface channel",
			`type T int; var c chan any; func f(x T) { c <- x }`,
			`func _() { f(1) }`,
			`func _() { c <- T(1) }`,
		},
		{
			"No type conversion for argument to concrete channel",
			`type T int32; var c chan T; func f(x T) { c <- x }`,
			`func _() { f(1) }`,
			`func _() { c <- 1 }`,
		},
		{
			"Type conversion for interface map key",
			`type T int; var m map[any]any; func f(x T) { m[x] = 1 }`,
			`func _() { f(1) }`,
			`func _() { m[T(1)] = 1 }`,
		},
		{
			"No type conversion for interface to interface map key",
			`type T int; var m map[any]any; func f(x any) { m[x] = 1 }`,
			`func _() { f(T(1)) }`,
			`func _() { m[T(1)] = 1 }`,
		},
		{
			"No type conversion for concrete map key",
			`type T int; var m map[T]any; func f(x T) { m[x] = 1 }`,
			`func _() { f(1) }`,
			`func _() { m[1] = 1 }`,
		},
		{
			"Type conversion for interface literal key/value",
			`type T int; type m map[any]any; func f(x, y T) { _ = m{x: y} }`,
			`func _() { f(1, 2) }`,
			`func _() { _ = m{T(1): T(2)} }`,
		},
		{
			"No type conversion for concrete literal key/value",
			`type T int; type m map[T]T; func f(x, y T) { _ = m{x: y} }`,
			`func _() { f(1, 2) }`,
			`func _() { _ = m{1: 2} }`,
		},
		{
			"Type conversion for interface literal element",
			`type T int; type s []any; func f(x T) { _ = s{x} }`,
			`func _() { f(1) }`,
			`func _() { _ = s{T(1)} }`,
		},
		{
			"No type conversion for concrete literal element",
			`type T int; type s []T; func f(x T) { _ = s{x} }`,
			`func _() { f(1) }`,
			`func _() { _ = s{1} }`,
		},
		{
			"Type conversion for interface unkeyed struct field",
			`type T int; type s struct{any}; func f(x T) { _ = s{x} }`,
			`func _() { f(1) }`,
			`func _() { _ = s{T(1)} }`,
		},
		{
			"No type conversion for concrete unkeyed struct field",
			`type T int; type s struct{T}; func f(x T) { _ = s{x} }`,
			`func _() { f(1) }`,
			`func _() { _ = s{1} }`,
		},
		{
			"Type conversion for interface field value",
			`type T int; type S struct{ F any }; func f(x T) { _ = S{F: x} }`,
			`func _() { f(1) }`,
			`func _() { _ = S{F: T(1)} }`,
		},
		{
			"No type conversion for concrete field value",
			`type T int; type S struct{ F T }; func f(x T) { _ = S{F: x} }`,
			`func _() { f(1) }`,
			`func _() { _ = S{F: 1} }`,
		},
		{
			"Type conversion for argument to interface channel",
			`type T int; var c chan any; func f(x any) { c <- x }`,
			`func _() { f(T(1)) }`,
			`func _() { c <- T(1) }`,
		},
		{
			"No type conversion for argument to concrete channel",
			`type T int32; var c chan T; func f(x T) { c <- x }`,
			`func _() { f(1) }`,
			`func _() { c <- 1 }`,
		},
		{
			"No type conversion for assignment to an explicit interface type",
			`type T int; func f(x any) { var y any; y = x; _ = y }`,
			`func _() { f(T(1)) }`,
			`func _() {
	var y any
	y = T(1)
	_ = y
}`,
		},
		{
			"No type conversion for short variable assignment to an explicit interface type",
			`type T int; func f(e error) { var err any; i, err := 1, e; _, _ = i, err }`,
			`func _() { f(nil) }`,
			`func _() {
	var err any
	i, err := 1, nil
	_, _ = i, err
}`,
		},
		{
			"No type conversion for initializer of an explicit interface type",
			`type T int; func f(x any) { var y any = x; _ = y }`,
			`func _() { f(T(1)) }`,
			`func _() {
	var y any = T(1)
	_ = y
}`,
		},
		{
			"No type conversion for use as a composite literal key",
			`type T int; func f(x any) { _ = map[any]any{x: 1} }`,
			`func _() { f(T(1)) }`,
			`func _() { _ = map[any]any{T(1): 1} }`,
		},
		{
			"No type conversion for use as a composite literal value",
			`type T int; func f(x any) { _ = []any{x} }`,
			`func _() { f(T(1)) }`,
			`func _() { _ = []any{T(1)} }`,
		},
		{
			"No type conversion for use as a composite literal field",
			`type T int; func f(x any) { _ = struct{ F any }{F: x} }`,
			`func _() { f(T(1)) }`,
			`func _() { _ = struct{ F any }{F: T(1)} }`,
		},
		{
			"No type conversion for use in a send statement",
			`type T int; func f(x any) { var c chan any; c <- x }`,
			`func _() { f(T(1)) }`,
			`func _() {
	var c chan any
	c <- T(1)
}`,
		},
	})
}

func runTests(t *testing.T, tests []testcase) {
	for _, test := range tests {
		t.Run(test.descr, func(t *testing.T) {
			fset := token.NewFileSet()
			mustParse := func(filename string, content any) *ast.File {
				f, err := parser.ParseFile(fset, filename, content, parser.ParseComments|parser.SkipObjectResolution)
				if err != nil {
					t.Fatalf("ParseFile: %v", err)
				}
				return f
			}

			// Parse callee file and find first func decl named f.
			calleeContent := "package p\n" + test.callee
			calleeFile := mustParse("callee.go", calleeContent)
			var decl *ast.FuncDecl
			for _, d := range calleeFile.Decls {
				if d, ok := d.(*ast.FuncDecl); ok && d.Name.Name == funcName {
					decl = d
					break
				}
			}
			if decl == nil {
				t.Fatalf("declaration of func %s not found: %s", funcName, test.callee)
			}

			// Parse caller file and find first call to f().
			callerContent := "package p\n" + test.caller
			callerFile := mustParse("caller.go", callerContent)
			var call *ast.CallExpr
			ast.Inspect(callerFile, func(n ast.Node) bool {
				if n, ok := n.(*ast.CallExpr); ok {
					switch fun := n.Fun.(type) {
					case *ast.SelectorExpr:
						if fun.Sel.Name == funcName {
							call = n
						}
					case *ast.Ident:
						if fun.Name == funcName {
							call = n
						}
					case *ast.IndexExpr:
						if id, ok := fun.X.(*ast.Ident); ok && id.Name == funcName {
							call = n
						}
					case *ast.IndexListExpr:
						if id, ok := fun.X.(*ast.Ident); ok && id.Name == funcName {
							call = n
						}
					}
				}
				return call == nil
			})
			if call == nil {
				t.Fatalf("call to %s not found: %s", funcName, test.caller)
			}

			// Type check both files as one package.
			info := &types.Info{
				Defs:       make(map[*ast.Ident]types.Object),
				Uses:       make(map[*ast.Ident]types.Object),
				Types:      make(map[ast.Expr]types.TypeAndValue),
				Implicits:  make(map[ast.Node]types.Object),
				Selections: make(map[*ast.SelectorExpr]*types.Selection),
				Scopes:     make(map[ast.Node]*types.Scope),
			}
			conf := &types.Config{Error: func(err error) { t.Error(err) }}
			pkg, err := conf.Check("p", fset, []*ast.File{callerFile, calleeFile}, info)
			if err != nil {
				t.Fatal("transformation introduced type errors")
			}

			// Analyze callee and inline call.
			doIt := func() (*inline.Result, error) {
				callee, err := inline.AnalyzeCallee(t.Logf, fset, pkg, info, decl, []byte(calleeContent))
				if err != nil {
					return nil, err
				}
				if err := checkTranscode(callee); err != nil {
					t.Fatal(err)
				}

				caller := &inline.Caller{
					Fset:    fset,
					Types:   pkg,
					Info:    info,
					File:    callerFile,
					Call:    call,
					Content: []byte(callerContent),
				}
				check := checkNoMutation(caller.File)
				defer check()
				return inline.Inline(caller, callee, &inline.Options{
					Logf:          t.Logf,
					IgnoreEffects: strings.Contains(test.descr, "IgnoreEffects"),
				})
			}
			res, err := doIt()

			// Want error?
			if rest, ok := strings.CutPrefix(test.want, "error: "); ok {
				if err == nil {
					t.Fatalf("unexpected success: want error matching %q", rest)
				}
				msg := err.Error()
				if ok, err := regexp.MatchString(rest, msg); err != nil {
					t.Fatalf("invalid regexp: %v", err)
				} else if !ok {
					t.Fatalf("wrong error: %s (want match for %q)", msg, rest)
				}
				return
			}

			// Want success.
			if err != nil {
				t.Fatal(err)
			}

			gotContent := res.Content

			// Compute a single-hunk line-based diff.
			srcLines := strings.Split(callerContent, "\n")
			gotLines := strings.Split(string(gotContent), "\n")
			for len(srcLines) > 0 && len(gotLines) > 0 &&
				srcLines[0] == gotLines[0] {
				srcLines = srcLines[1:]
				gotLines = gotLines[1:]
			}
			for len(srcLines) > 0 && len(gotLines) > 0 &&
				srcLines[len(srcLines)-1] == gotLines[len(gotLines)-1] {
				srcLines = srcLines[:len(srcLines)-1]
				gotLines = gotLines[:len(gotLines)-1]
			}
			got := strings.Join(gotLines, "\n")

			if strings.TrimSpace(got) != strings.TrimSpace(test.want) {
				t.Fatalf("\nInlining this call:\t%s\nof this callee:    \t%s\nproduced:\n%s\nWant:\n\n%s",
					test.caller,
					test.callee,
					got,
					test.want)
			}

			// Check that resulting code type-checks.
			newCallerFile := mustParse("newcaller.go", gotContent)
			if _, err := conf.Check("p", fset, []*ast.File{newCallerFile, calleeFile}, nil); err != nil {
				t.Fatalf("modified source failed to typecheck: <<%s>>", gotContent)
			}
		})
	}
}

// -- helpers --

// checkNoMutation returns a function that, when called,
// asserts that file was not modified since the checkNoMutation call.
func checkNoMutation(file *ast.File) func() {
	pre := deepHash(file)
	return func() {
		post := deepHash(file)
		if pre != post {
			panic("Inline mutated caller.File")
		}
	}
}

// checkTranscode replaces *callee by the results of gob-encoding and
// then decoding it, to test that these operations are lossless.
func checkTranscode(callee *inline.Callee) error {
	// Perform Gob transcoding so that it is exercised by the test.
	var enc bytes.Buffer
	if err := gob.NewEncoder(&enc).Encode(callee); err != nil {
		return fmt.Errorf("internal error: gob encoding failed: %v", err)
	}
	*callee = inline.Callee{}
	if err := gob.NewDecoder(&enc).Decode(callee); err != nil {
		return fmt.Errorf("internal error: gob decoding failed: %v", err)
	}
	return nil
}

// deepHash computes a cryptographic hash of an ast.Node so that
// if the data structure is mutated, the hash changes.
// It assumes Go variables do not change address.
//
// TODO(adonovan): consider publishing this in the astutil package.
//
// TODO(adonovan): consider a variant that reports where in the tree
// the mutation occurred (obviously at a cost in space).
func deepHash(n ast.Node) any {
	seen := make(map[unsafe.Pointer]bool) // to break cycles

	hasher := sha256.New()
	le := binary.LittleEndian
	writeUint64 := func(v uint64) {
		var bs [8]byte
		le.PutUint64(bs[:], v)
		hasher.Write(bs[:])
	}

	var visit func(reflect.Value)
	visit = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			ptr := v.UnsafePointer()
			writeUint64(uint64(uintptr(ptr)))
			if !v.IsNil() {
				if !seen[ptr] {
					seen[ptr] = true
					// Skip types we don't handle yet, but don't care about.
					switch v.Interface().(type) {
					case *ast.Scope:
						return // involves a map
					}

					visit(v.Elem())
				}
			}

		case reflect.Struct:
			for i := 0; i < v.Type().NumField(); i++ {
				visit(v.Field(i))
			}

		case reflect.Slice:
			ptr := v.UnsafePointer()
			// We may encounter different slices at the same address,
			// so don't mark ptr as "seen".
			writeUint64(uint64(uintptr(ptr)))
			writeUint64(uint64(v.Len()))
			writeUint64(uint64(v.Cap()))
			for i := 0; i < v.Len(); i++ {
				visit(v.Index(i))
			}

		case reflect.Interface:
			if v.IsNil() {
				writeUint64(0)
			} else {
				rtype := reflect.ValueOf(v.Type()).UnsafePointer()
				writeUint64(uint64(uintptr(rtype)))
				visit(v.Elem())
			}

		case reflect.String:
			writeUint64(uint64(v.Len()))
			hasher.Write([]byte(v.String()))

		case reflect.Int:
			writeUint64(uint64(v.Int()))

		case reflect.Uint:
			writeUint64(uint64(v.Uint()))

		case reflect.Bool, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			// Bools and fixed width numbers can be handled by binary.Write.
			binary.Write(hasher, le, v.Interface())

		default: // reflect.Array, reflect.Chan, reflect.Func, reflect.Map, reflect.UnsafePointer, reflect.Uintptr
			panic(v) // unreachable in AST
		}
	}
	visit(reflect.ValueOf(n))

	var hash [sha256.Size]byte
	hasher.Sum(hash[:0])
	return hash
}

func TestDeepHash(t *testing.T) {
	// This test reproduces a bug in DeepHash that was encountered during work on
	// the inliner.
	//
	// TODO(rfindley): consider replacing this with a fuzz test.
	id := &ast.Ident{
		NamePos: 2,
		Name:    "t",
	}
	c := &ast.CallExpr{
		Fun: id,
	}
	h1 := deepHash(c)
	id.NamePos = 1
	h2 := deepHash(c)
	if h1 == h2 {
		t.Fatal("bad")
	}
}
