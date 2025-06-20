// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imports

import (
	"archive/zip"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/mod/module"
	"github.com/tenntenn/exp/toolsinternal/gocommand"
	"github.com/tenntenn/exp/toolsinternal/gopathwalk"
	"github.com/tenntenn/exp/toolsinternal/proxydir"
	"github.com/tenntenn/exp/toolsinternal/testenv"
	"golang.org/x/tools/txtar"
	"maps"
	"slices"
)

// Tests that we can find packages in the stdlib.
func TestScanStdlib(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x
`, "")
	defer mt.cleanup()

	mt.assertScanFinds("fmt", "fmt")
}

// Tests that we handle a nested module. This is different from other tests
// where the module is in scope -- here we have to figure out the import path
// without any help from go list.
func TestScanOutOfScopeNestedModule(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

-- x.go --
package x

-- v2/go.mod --
module x

-- v2/x.go --
package x`, "")
	defer mt.cleanup()

	pkg := mt.assertScanFinds("x/v2", "x")
	if pkg != nil && !strings.HasSuffix(filepath.ToSlash(pkg.dir), "main/v2") {
		t.Errorf("x/v2 was found in %v, wanted .../main/v2", pkg.dir)
	}
	// We can't load the package name from the import path, but that should
	// be okay -- if we end up adding this result, we'll add it with a name
	// if necessary.
}

// Tests that we don't find a nested module contained in a local replace target.
// The code for this case is too annoying to write, so it's just ignored.
func TestScanNestedModuleInLocalReplace(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

require y v0.0.0
replace y => ./y

-- x.go --
package x

-- y/go.mod --
module y

-- y/y.go --
package y

-- y/z/go.mod --
module y/z

-- y/z/z.go --
package z
`, "")
	defer mt.cleanup()

	mt.assertFound("y", "y")

	scan, err := scanToSlice(mt.env.resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range scan {
		if strings.HasSuffix(filepath.ToSlash(pkg.dir), "main/y/z") {
			t.Errorf("scan found a package %v in dir main/y/z, wanted none", pkg.importPathShort)
		}
	}
}

// Tests that path encoding is handled correctly. Adapted from mod_case.txt.
func TestModCase(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

require rsc.io/QUOTE v1.5.2

-- x.go --
package x

import _ "rsc.io/QUOTE/QUOTE"
`, "")
	defer mt.cleanup()
	mt.assertFound("rsc.io/QUOTE/QUOTE", "QUOTE")
}

// Not obviously relevant to goimports. Adapted from mod_domain_root.txt anyway.
func TestModDomainRoot(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

require example.com v1.0.0

-- x.go --
package x
import _ "example.com"
`, "")
	defer mt.cleanup()
	mt.assertFound("example.com", "x")
}

// Tests that scanning the module cache > 1 time is able to find the same module.
func TestModMultipleScans(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

require example.com v1.0.0

-- x.go --
package x
import _ "example.com"
`, "")
	defer mt.cleanup()

	mt.assertScanFinds("example.com", "x")
	mt.assertScanFinds("example.com", "x")
}

// Tests that scanning the module cache > 1 time is able to find the same module
// in the module cache.
func TestModMultipleScansWithSubdirs(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

require rsc.io/quote v1.5.2

-- x.go --
package x
import _ "rsc.io/quote"
`, "")
	defer mt.cleanup()

	mt.assertScanFinds("rsc.io/quote", "quote")
	mt.assertScanFinds("rsc.io/quote", "quote")
}

// Tests that scanning the module cache > 1 after changing a package in module cache to make it unimportable
// is able to find the same module.
func TestModCacheEditModFile(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

require rsc.io/quote v1.5.2
-- x.go --
package x
import _ "rsc.io/quote"
`, "")
	defer mt.cleanup()
	found := mt.assertScanFinds("rsc.io/quote", "quote")
	if found == nil {
		t.Fatal("rsc.io/quote not found in initial scan.")
	}

	// Update the go.mod file of example.com so that it changes its module path (not allowed).
	if err := os.Chmod(filepath.Join(found.dir, "go.mod"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(found.dir, "go.mod"), []byte("module bad.com\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test that with its cache of module packages it still finds the package.
	mt.assertScanFinds("rsc.io/quote", "quote")

	// Rewrite the main package so that rsc.io/quote is not in scope.
	if err := os.WriteFile(filepath.Join(mt.env.WorkingDir, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mt.env.WorkingDir, "x.go"), []byte("package x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Uninitialize the go.mod dependent cached information and make sure it still finds the package.
	mt.env.ClearModuleInfo()
	mt.assertScanFinds("rsc.io/quote", "quote")
}

// Tests that -mod=vendor works. Adapted from mod_vendor_build.txt.
func TestModVendorBuild(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module m
go 1.12
require rsc.io/sampler v1.3.1
-- x.go --
package x
import _ "rsc.io/sampler"
`, "")
	defer mt.cleanup()

	// Sanity-check the setup.
	mt.assertModuleFoundInDir("rsc.io/sampler", "sampler", `pkg.*mod.*/sampler@.*$`)

	// Populate vendor/ and clear out the mod cache so we can't cheat.
	if _, err := mt.env.invokeGo(context.Background(), "mod", "vendor"); err != nil {
		t.Fatal(err)
	}
	if _, err := mt.env.invokeGo(context.Background(), "clean", "-modcache"); err != nil {
		t.Fatal(err)
	}

	// Clear out the resolver's cache, since we've changed the environment.
	mt.env.Env["GOFLAGS"] = "-mod=vendor"
	mt.env.ClearModuleInfo()
	mt.env.UpdateResolver(mt.env.resolver.ClearForNewScan())
	mt.assertModuleFoundInDir("rsc.io/sampler", "sampler", `/vendor/`)
}

// Tests that -mod=vendor is auto-enabled only for go1.14 and higher.
// Vaguely inspired by mod_vendor_auto.txt.
func TestModVendorAuto(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module m
go 1.14
require rsc.io/sampler v1.3.1
-- x.go --
package x
import _ "rsc.io/sampler"
`, "")
	defer mt.cleanup()

	// Populate vendor/.
	if _, err := mt.env.invokeGo(context.Background(), "mod", "vendor"); err != nil {
		t.Fatal(err)
	}

	wantDir := `/vendor/`

	// Clear out the resolver's module info, since we've changed the environment.
	// (the presence of a /vendor directory affects `go list -m`).
	mt.env.ClearModuleInfo()
	mt.assertModuleFoundInDir("rsc.io/sampler", "sampler", wantDir)
}

// Tests that a module replace works. Adapted from mod_list.txt. We start with
// go.mod2; the first part of the test is irrelevant.
func TestModList(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x
require rsc.io/quote v1.5.1
replace rsc.io/sampler v1.3.0 => rsc.io/sampler v1.3.1

-- x.go --
package x
import _ "rsc.io/quote"
`, "")
	defer mt.cleanup()

	mt.assertModuleFoundInDir("rsc.io/sampler", "sampler", `pkg.mod.*/sampler@v1.3.1$`)
}

// Tests that a local replace works. Adapted from mod_local_replace.txt.
func TestModLocalReplace(t *testing.T) {
	mt := setup(t, nil, `
-- x/y/go.mod --
module x/y
require zz v1.0.0
replace zz v1.0.0 => ../z

-- x/y/y.go --
package y
import _ "zz"

-- x/z/go.mod --
module x/z

-- x/z/z.go --
package z
`, "x/y")
	defer mt.cleanup()

	mt.assertFound("zz", "z")
}

// Tests that the package at the root of the main module can be found.
// Adapted from the first part of mod_multirepo.txt.
func TestModMultirepo1(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module rsc.io/quote

-- x.go --
package quote
`, "")
	defer mt.cleanup()

	mt.assertModuleFoundInDir("rsc.io/quote", "quote", `/main`)
}

// Tests that a simple module dependency is found. Adapted from the third part
// of mod_multirepo.txt (We skip the case where it doesn't have a go.mod
// entry -- we just don't work in that case.)
func TestModMultirepo3(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module rsc.io/quote

require rsc.io/quote/v2 v2.0.1
-- x.go --
package quote

import _ "rsc.io/quote/v2"
`, "")
	defer mt.cleanup()

	mt.assertModuleFoundInDir("rsc.io/quote", "quote", `/main`)
	mt.assertModuleFoundInDir("rsc.io/quote/v2", "quote", `pkg.mod.*/v2@v2.0.1$`)
}

// Tests that a nested module is found in the module cache, even though
// it's checked out. Adapted from the fourth part of mod_multirepo.txt.
func TestModMultirepo4(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module rsc.io/quote
require rsc.io/quote/v2 v2.0.1

-- x.go --
package quote
import _ "rsc.io/quote/v2"

-- v2/go.mod --
package rsc.io/quote/v2

-- v2/x.go --
package quote
import _ "rsc.io/quote/v2"
`, "")
	defer mt.cleanup()

	mt.assertModuleFoundInDir("rsc.io/quote", "quote", `/main`)
	mt.assertModuleFoundInDir("rsc.io/quote/v2", "quote", `pkg.mod.*/v2@v2.0.1$`)
}

// Tests a simple module dependency. Adapted from the first part of mod_replace.txt.
func TestModReplace1(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module quoter

require rsc.io/quote/v3 v3.0.0

-- main.go --

package main
`, "")
	defer mt.cleanup()
	mt.assertFound("rsc.io/quote/v3", "quote")
}

// Tests a local replace. Adapted from the second part of mod_replace.txt.
func TestModReplace2(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module quoter

require rsc.io/quote/v3 v3.0.0
replace rsc.io/quote/v3 => ./local/rsc.io/quote/v3
-- main.go --
package main

-- local/rsc.io/quote/v3/go.mod --
module rsc.io/quote/v3

require rsc.io/sampler v1.3.0

-- local/rsc.io/quote/v3/quote.go --
package quote

import "rsc.io/sampler"
`, "")
	defer mt.cleanup()
	mt.assertModuleFoundInDir("rsc.io/quote/v3", "quote", `/local/rsc.io/quote/v3`)
}

// Tests that a module can be replaced by a different module path. Adapted
// from the third part of mod_replace.txt.
func TestModReplace3(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module quoter

require not-rsc.io/quote/v3 v3.1.0
replace not-rsc.io/quote/v3 v3.1.0 => ./local/rsc.io/quote/v3

-- usenewmodule/main.go --
package main

-- local/rsc.io/quote/v3/go.mod --
module rsc.io/quote/v3

require rsc.io/sampler v1.3.0

-- local/rsc.io/quote/v3/quote.go --
package quote

-- local/not-rsc.io/quote/v3/go.mod --
module not-rsc.io/quote/v3

-- local/not-rsc.io/quote/v3/quote.go --
package quote
`, "")
	defer mt.cleanup()
	mt.assertModuleFoundInDir("not-rsc.io/quote/v3", "quote", "local/rsc.io/quote/v3")
}

// Tests more local replaces, notably the case where an outer module provides
// a package that could also be provided by an inner module. Adapted from
// mod_replace_import.txt, with example.com/v changed to /vv because Go 1.11
// thinks /v is an invalid major version.
func TestModReplaceImport(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module example.com/m

replace (
	example.com/a => ./a
	example.com/a/b => ./b
)

replace (
	example.com/x => ./x
	example.com/x/v3 => ./v3
)

replace (
	example.com/y/z/w => ./w
	example.com/y => ./y
)

replace (
	example.com/vv v1.11.0 => ./v11
	example.com/vv v1.12.0 => ./v12
	example.com/vv => ./vv
)

require (
	example.com/a/b v0.0.0
	example.com/x/v3 v3.0.0
	example.com/y v0.0.0
	example.com/y/z/w v0.0.0
	example.com/vv v1.12.0
)
-- m.go --
package main
import (
	_ "example.com/a/b"
	_ "example.com/x/v3"
	_ "example.com/y/z/w"
	_ "example.com/vv"
)
func main() {}

-- a/go.mod --
module a.localhost
-- a/a.go --
package a
-- a/b/b.go--
package b

-- b/go.mod --
module a.localhost/b
-- b/b.go --
package b

-- x/go.mod --
module x.localhost
-- x/x.go --
package x
-- x/v3.go --
package v3
import _ "x.localhost/v3"

-- v3/go.mod --
module x.localhost/v3
-- v3/x.go --
package x

-- w/go.mod --
module w.localhost
-- w/skip/skip.go --
// Package skip is nested below nonexistent package w.
package skip

-- y/go.mod --
module y.localhost
-- y/z/w/w.go --
package w

-- v12/go.mod --
module v.localhost
-- v12/v.go --
package v

-- v11/go.mod --
module v.localhost
-- v11/v.go --
package v

-- vv/go.mod --
module v.localhost
-- vv/v.go --
package v
`, "")
	defer mt.cleanup()

	mt.assertModuleFoundInDir("example.com/a/b", "b", `main/b$`)
	mt.assertModuleFoundInDir("example.com/x/v3", "x", `main/v3$`)
	mt.assertModuleFoundInDir("example.com/y/z/w", "w", `main/y/z/w$`)
	mt.assertModuleFoundInDir("example.com/vv", "v", `main/v12$`)
}

// Tests that go.work files are respected.
func TestModWorkspace(t *testing.T) {
	mt := setup(t, nil, `
-- go.work --
go 1.18

use (
	./a
	./b
)
-- a/go.mod --
module example.com/a

go 1.18
-- a/a.go --
package a
-- b/go.mod --
module example.com/b

go 1.18
-- b/b.go --
package b
`, "")
	defer mt.cleanup()

	mt.assertModuleFoundInDir("example.com/a", "a", `main/a$`)
	mt.assertModuleFoundInDir("example.com/b", "b", `main/b$`)
	mt.assertScanFinds("example.com/a", "a")
	mt.assertScanFinds("example.com/b", "b")
}

// Tests replaces in workspaces. Uses the directory layout in the cmd/go
// work_replace test. It tests both that replaces in go.work files are
// respected and that a wildcard replace in go.work overrides a versioned replace
// in go.mod.
func TestModWorkspaceReplace(t *testing.T) {
	mt := setup(t, nil, `
-- go.work --
use m

replace example.com/dep => ./dep
replace example.com/other => ./other2

-- m/go.mod --
module example.com/m

require example.com/dep v1.0.0
require example.com/other v1.0.0

replace example.com/other v1.0.0 => ./other
-- m/m.go --
package m

import "example.com/dep"
import "example.com/other"

func F() {
	dep.G()
	other.H()
}
-- dep/go.mod --
module example.com/dep
-- dep/dep.go --
package dep

func G() {
}
-- other/go.mod --
module example.com/other
-- other/dep.go --
package other

func G() {
}
-- other2/go.mod --
module example.com/other
-- other2/dep.go --
package other2

func G() {
}
`, "")
	defer mt.cleanup()

	mt.assertScanFinds("example.com/m", "m")
	mt.assertScanFinds("example.com/dep", "dep")
	mt.assertModuleFoundInDir("example.com/other", "other2", "main/other2$")
	mt.assertScanFinds("example.com/other", "other2")
}

// Tests a case where conflicting replaces are overridden by a replace
// in the go.work file.
func TestModWorkspaceReplaceOverride(t *testing.T) {
	mt := setup(t, nil, `-- go.work --
use m
use n
replace example.com/dep => ./dep3
-- m/go.mod --
module example.com/m

require example.com/dep v1.0.0
replace example.com/dep => ./dep1
-- m/m.go --
package m

import "example.com/dep"

func F() {
	dep.G()
}
-- n/go.mod --
module example.com/n

require example.com/dep v1.0.0
replace example.com/dep => ./dep2
-- n/n.go --
package n

import "example.com/dep"

func F() {
	dep.G()
}
-- dep1/go.mod --
module example.com/dep
-- dep1/dep.go --
package dep

func G() {
}
-- dep2/go.mod --
module example.com/dep
-- dep2/dep.go --
package dep

func G() {
}
-- dep3/go.mod --
module example.com/dep
-- dep3/dep.go --
package dep

func G() {
}
`, "")

	mt.assertScanFinds("example.com/m", "m")
	mt.assertScanFinds("example.com/n", "n")
	mt.assertScanFinds("example.com/dep", "dep")
	mt.assertModuleFoundInDir("example.com/dep", "dep", "main/dep3$")
}

// Tests that the correct versions of modules are found in
// workspaces with module pruning. This is based on the
// cmd/go mod_prune_all script test.
func TestModWorkspacePrune(t *testing.T) {
	mt := setup(t, nil, `
-- go.work --
go 1.18

use (
	./a
	./p
)

replace example.com/b v1.0.0 => ./b
replace example.com/q v1.0.0 => ./q1_0_0
replace example.com/q v1.0.5 => ./q1_0_5
replace example.com/q v1.1.0 => ./q1_1_0
replace example.com/r v1.0.0 => ./r
replace example.com/w v1.0.0 => ./w
replace example.com/x v1.0.0 => ./x
replace example.com/y v1.0.0 => ./y
replace example.com/z v1.0.0 => ./z1_0_0
replace example.com/z v1.1.0 => ./z1_1_0

-- a/go.mod --
module example.com/a

go 1.18

require example.com/b v1.0.0
require example.com/z v1.0.0
-- a/foo.go --
package main

import "example.com/b"

func main() {
	b.B()
}
-- b/go.mod --
module example.com/b

go 1.18

require example.com/q v1.1.0
-- b/b.go --
package b

func B() {
}
-- p/go.mod --
module example.com/p

go 1.18

require example.com/q v1.0.0

replace example.com/q v1.0.0 => ../q1_0_0
replace example.com/q v1.1.0 => ../q1_1_0
-- p/main.go --
package main

import "example.com/q"

func main() {
	q.PrintVersion()
}
-- q1_0_0/go.mod --
module example.com/q

go 1.18
-- q1_0_0/q.go --
package q

import "fmt"

func PrintVersion() {
	fmt.Println("version 1.0.0")
}
-- q1_0_5/go.mod --
module example.com/q

go 1.18

require example.com/r v1.0.0
-- q1_0_5/q.go --
package q

import _ "example.com/r"
-- q1_1_0/go.mod --
module example.com/q

require example.com/w v1.0.0
require example.com/z v1.1.0

go 1.18
-- q1_1_0/q.go --
package q

import _ "example.com/w"
import _ "example.com/z"

import "fmt"

func PrintVersion() {
	fmt.Println("version 1.1.0")
}
-- r/go.mod --
module example.com/r

go 1.18

require example.com/r v1.0.0
-- r/r.go --
package r
-- w/go.mod --
module example.com/w

go 1.18

require example.com/x v1.0.0
-- w/w.go --
package w
-- w/w_test.go --
package w

import _ "example.com/x"
-- x/go.mod --
module example.com/x

go 1.18
-- x/x.go --
package x
-- x/x_test.go --
package x
import _ "example.com/y"
-- y/go.mod --
module example.com/y

go 1.18
-- y/y.go --
package y
-- z1_0_0/go.mod --
module example.com/z

go 1.18

require example.com/q v1.0.5
-- z1_0_0/z.go --
package z

import _ "example.com/q"
-- z1_1_0/go.mod --
module example.com/z

go 1.18
-- z1_1_0/z.go --
package z
`, "")

	mt.assertScanFinds("example.com/w", "w")
	mt.assertScanFinds("example.com/q", "q")
	mt.assertScanFinds("example.com/x", "x")
	mt.assertScanFinds("example.com/z", "z")
	mt.assertModuleFoundInDir("example.com/w", "w", "main/w$")
	mt.assertModuleFoundInDir("example.com/q", "q", "main/q1_1_0$")
	mt.assertModuleFoundInDir("example.com/x", "x", "main/x$")
	mt.assertModuleFoundInDir("example.com/z", "z", "main/z1_1_0$")
}

// Tests that we handle GO111MODULE=on with no go.mod file. See #30855.
func TestNoMainModule(t *testing.T) {
	mt := setup(t, map[string]string{"GO111MODULE": "on"}, `
-- x.go --
package x
`, "")
	defer mt.cleanup()
	if _, err := mt.env.invokeGo(context.Background(), "mod", "download", "rsc.io/quote@v1.5.1"); err != nil {
		t.Fatal(err)
	}

	mt.assertScanFinds("rsc.io/quote", "quote")
}

// assertFound asserts that the package at importPath is found to have pkgName,
// and that scanning for pkgName finds it at importPath.
func (t *modTest) assertFound(importPath, pkgName string) (string, *pkg) {
	t.Helper()

	names, err := t.env.resolver.loadPackageNames([]string{importPath}, t.env.WorkingDir)
	if err != nil {
		t.Errorf("loading package name for %v: %v", importPath, err)
	}
	if names[importPath] != pkgName {
		t.Errorf("package name for %v = %v, want %v", importPath, names[importPath], pkgName)
	}
	pkg := t.assertScanFinds(importPath, pkgName)

	_, foundDir := t.env.resolver.(*ModuleResolver).findPackage(importPath)
	return foundDir, pkg
}

func (t *modTest) assertScanFinds(importPath, pkgName string) *pkg {
	t.Helper()
	scan, err := scanToSlice(t.env.resolver, nil)
	if err != nil {
		t.Errorf("scan failed: %v", err)
	}
	for _, pkg := range scan {
		if pkg.importPathShort == importPath {
			return pkg
		}
	}
	t.Errorf("scanning for %v did not find %v", pkgName, importPath)
	return nil
}

func scanToSlice(resolver Resolver, exclude []gopathwalk.RootType) ([]*pkg, error) {
	var mu sync.Mutex
	var result []*pkg
	filter := &scanCallback{
		rootFound: func(root gopathwalk.Root) bool {
			return !slices.Contains(exclude, root.Type)
		},
		dirFound: func(pkg *pkg) bool {
			return true
		},
		packageNameLoaded: func(pkg *pkg) bool {
			mu.Lock()
			defer mu.Unlock()
			result = append(result, pkg)
			return false
		},
	}
	err := resolver.scan(context.Background(), filter)
	return result, err
}

// assertModuleFoundInDir is the same as assertFound, but also checks that the
// package was found in an active module whose Dir matches dirRE.
func (t *modTest) assertModuleFoundInDir(importPath, pkgName, dirRE string) {
	t.Helper()
	dir, pkg := t.assertFound(importPath, pkgName)
	re, err := regexp.Compile(dirRE)
	if err != nil {
		t.Fatal(err)
	}

	if dir == "" {
		t.Errorf("import path %v not found in active modules", importPath)
	} else {
		if !re.MatchString(filepath.ToSlash(dir)) {
			t.Errorf("finding dir for %s: dir = %q did not match regex %q", importPath, dir, dirRE)
		}
	}
	if pkg != nil {
		if !re.MatchString(filepath.ToSlash(pkg.dir)) {
			t.Errorf("scanning for %s: dir = %q did not match regex %q", pkgName, pkg.dir, dirRE)
		}
	}
}

var proxyOnce sync.Once
var proxyDir string

type modTest struct {
	*testing.T
	env     *ProcessEnv
	gopath  string
	cleanup func()
}

// setup builds a test environment from a txtar and supporting modules
// in testdata/mod, along the lines of TestScript in cmd/go.
//
// extraEnv is applied on top of the default test env.
func setup(t *testing.T, extraEnv map[string]string, main, wd string) *modTest {
	t.Helper()
	testenv.NeedsTool(t, "go")

	proxyOnce.Do(func() {
		var err error
		proxyDir, err = os.MkdirTemp("", "proxy-")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeProxy(proxyDir, "testdata/mod"); err != nil {
			t.Fatal(err)
		}
	})

	dir, err := os.MkdirTemp("", t.Name())
	if err != nil {
		t.Fatal(err)
	}

	mainDir := filepath.Join(dir, "main")
	if err := writeModule(mainDir, main); err != nil {
		t.Fatal(err)
	}

	env := &ProcessEnv{
		Env: map[string]string{
			"GOPATH":      filepath.Join(dir, "gopath"),
			"GOMODCACHE":  "",
			"GO111MODULE": "auto",
			"GOSUMDB":     "off",
			"GOPROXY":     proxydir.ToURL(proxyDir),
		},
		WorkingDir:  filepath.Join(mainDir, wd),
		GocmdRunner: &gocommand.Runner{},
	}
	maps.Copy(env.Env, extraEnv)
	if *testDebug {
		env.Logf = log.Printf
	}
	// go mod download gets mad if we don't have a go.mod, so make sure we do.
	_, err = os.Stat(filepath.Join(mainDir, "go.mod"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("checking if go.mod exists: %v", err)
	}
	if err == nil {
		if _, err := env.invokeGo(context.Background(), "mod", "download", "all"); err != nil {
			t.Fatal(err)
		}
	}

	// Ensure the resolver is set for tests that (unsafely) access env.resolver
	// directly.
	//
	// TODO(rfindley): fix this after addressing the TODO in the ProcessEnv
	// docstring.
	if _, err := env.GetResolver(); err != nil {
		t.Fatal(err)
	}

	return &modTest{
		T:       t,
		gopath:  env.Env["GOPATH"],
		env:     env,
		cleanup: func() { removeDir(dir) },
	}
}

// writeModule writes the module in the ar, a txtar, to dir.
func writeModule(dir, ar string) error {
	a := txtar.Parse([]byte(ar))

	for _, f := range a.Files {
		fpath := filepath.Join(dir, f.Name)
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		if err := os.WriteFile(fpath, f.Data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// writeProxy writes all the txtar-formatted modules in arDir to a proxy
// directory in dir.
func writeProxy(dir, arDir string) error {
	files, err := os.ReadDir(arDir)
	if err != nil {
		return err
	}

	for _, fi := range files {
		if err := writeProxyModule(dir, filepath.Join(arDir, fi.Name())); err != nil {
			return err
		}
	}
	return nil
}

// writeProxyModule writes a txtar-formatted module at arPath to the module
// proxy in base.
func writeProxyModule(base, arPath string) error {
	arName := filepath.Base(arPath)
	i := strings.LastIndex(arName, "_v")
	ver := strings.TrimSuffix(arName[i+1:], ".txt")
	modDir := strings.ReplaceAll(arName[:i], "_", "/")
	modPath, err := module.UnescapePath(modDir)
	if err != nil {
		return err
	}

	dir := filepath.Join(base, modDir, "@v")
	a, err := txtar.ParseFile(arPath)

	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(dir, ver+".zip"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	for _, f := range a.Files {
		if f.Name[0] == '.' {
			if err := os.WriteFile(filepath.Join(dir, ver+f.Name), f.Data, 0644); err != nil {
				return err
			}
		} else {
			zf, err := z.Create(modPath + "@" + ver + "/" + f.Name)
			if err != nil {
				return err
			}
			if _, err := zf.Write(f.Data); err != nil {
				return err
			}
		}
	}
	if err := z.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	list, err := os.OpenFile(filepath.Join(dir, "list"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(list, "%s\n", ver); err != nil {
		return err
	}
	if err := list.Close(); err != nil {
		return err
	}
	return nil
}

func removeDir(dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			_ = os.Chmod(path, 0777)
		}
		return nil
	})
	_ = os.RemoveAll(dir) // ignore errors
}

// Tests that findModFile can find the mod files from a path in the module cache.
func TestFindModFileModCache(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module x

require rsc.io/quote v1.5.2
-- x.go --
package x
import _ "rsc.io/quote"
`, "")
	defer mt.cleanup()
	want := filepath.Join(mt.gopath, "pkg/mod", "rsc.io/quote@v1.5.2")

	found := mt.assertScanFinds("rsc.io/quote", "quote")
	modDir, _ := mt.env.resolver.(*ModuleResolver).modInfo(found.dir)
	if modDir != want {
		t.Errorf("expected: %s, got: %s", want, modDir)
	}
}

// Tests that crud in the module cache is ignored.
func TestInvalidModCache(t *testing.T) {
	testenv.NeedsTool(t, "go")

	dir, err := os.MkdirTemp("", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer removeDir(dir)

	// This doesn't have module@version like it should.
	if err := os.MkdirAll(filepath.Join(dir, "gopath/pkg/mod/sabotage"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gopath/pkg/mod/sabotage/x.go"), []byte("package foo\n"), 0777); err != nil {
		t.Fatal(err)
	}
	env := &ProcessEnv{
		Env: map[string]string{
			"GOPATH":      filepath.Join(dir, "gopath"),
			"GO111MODULE": "on",
			"GOSUMDB":     "off",
		},
		GocmdRunner: &gocommand.Runner{},
		WorkingDir:  dir,
	}
	resolver, err := env.GetResolver()
	if err != nil {
		t.Fatal(err)
	}
	scanToSlice(resolver, nil)
}

func TestGetCandidatesRanking(t *testing.T) {
	mt := setup(t, nil, `
-- go.mod --
module example.com

require rsc.io/quote v1.5.1
require rsc.io/quote/v3 v3.0.0

-- rpackage/x.go --
package rpackage
import (
	_ "rsc.io/quote"
	_ "rsc.io/quote/v3"
)
`, "")
	defer mt.cleanup()

	if _, err := mt.env.invokeGo(context.Background(), "mod", "download", "rsc.io/quote/v2@v2.0.1"); err != nil {
		t.Fatal(err)
	}

	type res struct {
		relevance  float64
		name, path string
	}
	want := []res{
		// Stdlib
		{7, "bytes", "bytes"},
		{7, "http", "net/http"},
		// Main module
		{6, "rpackage", "example.com/rpackage"},
		// Direct module deps with v2+ major version
		{5.003, "quote", "rsc.io/quote/v3"},
		// Direct module deps
		{5, "quote", "rsc.io/quote"},
		// Indirect deps
		{4, "language", "golang.org/x/text/language"},
		// Out of scope modules
		{3, "quote", "rsc.io/quote/v2"},
	}
	var mu sync.Mutex
	var got []res
	add := func(c ImportFix) {
		mu.Lock()
		defer mu.Unlock()
		for _, w := range want {
			if c.StmtInfo.ImportPath == w.path {
				got = append(got, res{c.Relevance, c.IdentName, c.StmtInfo.ImportPath})
			}
		}
	}
	if err := GetAllCandidates(context.Background(), add, "", "foo.go", "foo", mt.env); err != nil {
		t.Fatalf("getAllCandidates() = %v", err)
	}
	sort.Slice(got, func(i, j int) bool {
		ri, rj := got[i], got[j]
		if ri.relevance != rj.relevance {
			return ri.relevance > rj.relevance // Highest first.
		}
		return ri.name < rj.name
	})
	if !reflect.DeepEqual(want, got) {
		t.Errorf("wanted candidates in order %v, got %v", want, got)
	}
}

func BenchmarkModuleResolver_RescanModCache(b *testing.B) {
	env := &ProcessEnv{
		GocmdRunner: &gocommand.Runner{},
		// Uncomment for verbose logging (too verbose to enable by default).
		// Logf:        b.Logf,
	}
	exclude := []gopathwalk.RootType{gopathwalk.RootGOROOT}
	resolver, err := env.GetResolver()
	if err != nil {
		b.Fatal(err)
	}
	start := time.Now()
	scanToSlice(resolver, exclude)
	b.Logf("warming the mod cache took %v", time.Since(start))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanToSlice(resolver, exclude)
		resolver = resolver.ClearForNewScan()
	}
}

func BenchmarkModuleResolver_InitialScan(b *testing.B) {
	for i := 0; i < b.N; i++ {
		env := &ProcessEnv{
			GocmdRunner: &gocommand.Runner{},
		}
		exclude := []gopathwalk.RootType{gopathwalk.RootGOROOT}
		resolver, err := env.GetResolver()
		if err != nil {
			b.Fatal(err)
		}
		scanToSlice(resolver, exclude)
	}
}

// Tests that go.work files and vendor directory are respected.
func TestModWorkspaceVendoring(t *testing.T) {
	mt := setup(t, nil, `
-- go.work --
go 1.22

use (
	./a
	./b
)
-- a/go.mod --
module example.com/a

go 1.22

require rsc.io/sampler v1.3.1
-- a/a.go --
package a

import _ "rsc.io/sampler"
-- b/go.mod --
module example.com/b

go 1.22
-- b/b.go --
package b
`, "")
	defer mt.cleanup()

	// generate vendor directory
	if _, err := mt.env.invokeGo(context.Background(), "work", "vendor"); err != nil {
		t.Fatal(err)
	}

	// update module resolver
	mt.env.ClearModuleInfo()
	mt.env.UpdateResolver(mt.env.resolver.ClearForNewScan())

	mt.assertModuleFoundInDir("example.com/a", "a", `main/a$`)
	mt.assertScanFinds("example.com/a", "a")
	mt.assertModuleFoundInDir("example.com/b", "b", `main/b$`)
	mt.assertScanFinds("example.com/b", "b")
	mt.assertModuleFoundInDir("rsc.io/sampler", "sampler", `/vendor/`)
}
