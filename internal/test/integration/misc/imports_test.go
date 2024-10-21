// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package misc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnoverse/gnopls/internal/test/compare"
	. "github.com/gnoverse/gnopls/internal/test/integration"

	"github.com/gnoverse/gnopls/internal/protocol"
)

// Tests golang/go#38815.
func TestIssue38815(t *testing.T) {
	const needs = `
-- go.mod --
module foo

go 1.12
-- a.go --
package main
func f() {}
`
	const ntest = `package main
func TestZ(t *testing.T) {
	f()
}
`
	const want = `package main

import "testing"

func TestZ(t *testing.T) {
	f()
}
`

	// it was returning
	// "package main\nimport \"testing\"\npackage main..."
	Run(t, needs, func(t *testing.T, env *Env) {
		env.CreateBuffer("a_test.go", ntest)
		env.SaveBuffer("a_test.go")
		got := env.BufferText("a_test.go")
		if want != got {
			t.Errorf("got\n%q, wanted\n%q", got, want)
		}
	})
}

func TestIssue59124(t *testing.T) {
	const stuff = `
-- go.mod --
module foo
go 1.19
-- a.go --
//line foo.y:102
package main

import "fmt"

//this comment is necessary for failure
func a() {
	fmt.Println("hello")
}
`
	Run(t, stuff, func(t *testing.T, env *Env) {
		env.OpenFile("a.go")
		was := env.BufferText("a.go")
		env.AfterChange(NoDiagnostics())
		env.OrganizeImports("a.go")
		is := env.BufferText("a.go")
		if diff := compare.Text(was, is); diff != "" {
			t.Errorf("unexpected diff after organizeImports:\n%s", diff)
		}
	})
}

func TestIssue66407(t *testing.T) {
	const files = `
-- go.mod --
module foo
go 1.21
-- a.go --
package foo

func f(x float64) float64 {
	return x +  rand.Float64()
}
-- b.go --
package foo

func g() {
	_ = rand.Int63()
}
`
	WithOptions(Modes(Default)).
		Run(t, files, func(t *testing.T, env *Env) {
			env.OpenFile("a.go")
			was := env.BufferText("a.go")
			env.OrganizeImports("a.go")
			is := env.BufferText("a.go")
			// expect complaint that module is before 1.22
			env.AfterChange(Diagnostics(ForFile("a.go")))
			diff := compare.Text(was, is)
			// check that it found the 'right' rand
			if !strings.Contains(diff, `import "math/rand/v2"`) {
				t.Errorf("expected rand/v2, got %q", diff)
			}
			env.OpenFile("b.go")
			was = env.BufferText("b.go")
			env.OrganizeImports("b.go")
			// a.go still has its module problem but b.go is fine
			env.AfterChange(Diagnostics(ForFile("a.go")),
				NoDiagnostics(ForFile("b.go")))
			is = env.BufferText("b.go")
			diff = compare.Text(was, is)
			if !strings.Contains(diff, `import "math/rand"`) {
				t.Errorf("expected math/rand, got %q", diff)
			}
		})
}

func TestVim1(t *testing.T) {
	const vim1 = `package main

import "fmt"

var foo = 1
var bar = 2

func main() {
	fmt.Printf("This is a test %v\n", foo)
	fmt.Printf("This is another test %v\n", foo)
	fmt.Printf("This is also a test %v\n", foo)
}
`

	// The file remains unchanged, but if there any quick fixes
	// are returned, they confuse vim (according to CL 233117).
	// Therefore check for no QuickFix CodeActions.
	Run(t, "", func(t *testing.T, env *Env) {
		env.CreateBuffer("main.go", vim1)
		env.OrganizeImports("main.go")

		// Assert no quick fixes.
		for _, act := range env.CodeActionForFile("main.go", nil) {
			if act.Kind == protocol.QuickFix {
				t.Errorf("unexpected quick fix action: %#v", act)
			}
		}
		if t.Failed() {
			got := env.BufferText("main.go")
			if got == vim1 {
				t.Errorf("no changes")
			} else {
				t.Errorf("got\n%q", got)
				t.Errorf("was\n%q", vim1)
			}
		}
	})
}

func TestVim2(t *testing.T) {
	const vim2 = `package main

import (
	"fmt"

	"example.com/blah"

	"rubbish.com/useless"
)

func main() {
	fmt.Println(blah.Name, useless.Name)
}
`

	Run(t, "", func(t *testing.T, env *Env) {
		env.CreateBuffer("main.go", vim2)
		env.OrganizeImports("main.go")

		// Assert no quick fixes.
		for _, act := range env.CodeActionForFile("main.go", nil) {
			if act.Kind == protocol.QuickFix {
				t.Errorf("unexpected quick-fix action: %#v", act)
			}
		}
	})
}

const exampleProxy = `
-- example.com@v1.2.3/go.mod --
module example.com

go 1.12
-- example.com@v1.2.3/x/x.go --
package x

const X = 1
-- example.com@v1.2.3/y/y.go --
package y

const Y = 2
`

func TestGOMODCACHE(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.12

require example.com v1.2.3
-- main.go --
package main

import "example.com/x"

var _, _ = x.X, y.Y
`
	modcache := t.TempDir()
	defer cleanModCache(t, modcache) // see doc comment of cleanModCache

	WithOptions(
		EnvVars{"GOMODCACHE": modcache},
		ProxyFiles(exampleProxy),
		WriteGoSum("."),
	).Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("main.go")
		env.AfterChange(Diagnostics(env.AtRegexp("main.go", `y.Y`)))
		env.SaveBuffer("main.go")
		env.AfterChange(NoDiagnostics(ForFile("main.go")))
		loc := env.GoToDefinition(env.RegexpSearch("main.go", `y.(Y)`))
		path := env.Sandbox.Workdir.URIToPath(loc.URI)
		if !strings.HasPrefix(path, filepath.ToSlash(modcache)) {
			t.Errorf("found module dependency outside of GOMODCACHE: got %v, wanted subdir of %v", path, filepath.ToSlash(modcache))
		}
	})
}

func TestRelativeReplace(t *testing.T) {
	const files = `
-- go.mod --
module mod.com/a

go 1.20

require (
	example.com   v1.2.3
)

replace example.com/b => ../b
-- main.go --
package main

import "example.com/x"

var _, _ = x.X, y.Y
`
	modcache := t.TempDir()
	base := filepath.Base(modcache)
	defer cleanModCache(t, modcache) // see doc comment of cleanModCache

	// Construct a very unclean module cache whose length exceeds the length of
	// the clean directory path, to reproduce the crash in golang/go#67156
	const sep = string(filepath.Separator)
	modcache += strings.Repeat(sep+".."+sep+base, 10)

	WithOptions(
		EnvVars{"GOMODCACHE": modcache},
		ProxyFiles(exampleProxy),
		WriteGoSum("."),
	).Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("main.go")
		env.AfterChange(Diagnostics(env.AtRegexp("main.go", `y.Y`)))
		env.SaveBuffer("main.go")
		env.AfterChange(NoDiagnostics(ForFile("main.go")))
	})
}

// TODO(rfindley): this is only necessary as the module cache cleaning of the
// sandbox does not respect GOMODCACHE set via EnvVars. We should fix this, but
// that is probably part of a larger refactoring of the sandbox that I'm not
// inclined to undertake.
func cleanModCache(t *testing.T, modcache string) {
	cmd := exec.Command("go", "clean", "-modcache")
	cmd.Env = append(os.Environ(), "GOMODCACHE="+modcache)
	if err := cmd.Run(); err != nil {
		t.Errorf("cleaning modcache: %v", err)
	}
}

// Tests golang/go#40685.
func TestAcceptImportsQuickFixTestVariant(t *testing.T) {
	const pkg = `
-- go.mod --
module mod.com

go 1.12
-- a/a.go --
package a

import (
	"fmt"
)

func _() {
	fmt.Println("")
	os.Stat("")
}
-- a/a_test.go --
package a

import (
	"os"
	"testing"
)

func TestA(t *testing.T) {
	os.Stat("")
}
`
	Run(t, pkg, func(t *testing.T, env *Env) {
		env.OpenFile("a/a.go")
		var d protocol.PublishDiagnosticsParams
		env.AfterChange(
			Diagnostics(env.AtRegexp("a/a.go", "os.Stat")),
			ReadDiagnostics("a/a.go", &d),
		)
		env.ApplyQuickFixes("a/a.go", d.Diagnostics)
		env.AfterChange(
			NoDiagnostics(ForFile("a/a.go")),
		)
	})
}

// Test for golang/go#52784
func TestGoWorkImports(t *testing.T) {
	const pkg = `
-- go.work --
go 1.19

use (
        ./caller
        ./mod
)
-- caller/go.mod --
module caller.com

go 1.18

require mod.com v0.0.0

replace mod.com => ../mod
-- caller/caller.go --
package main

func main() {
        a.Test()
}
-- mod/go.mod --
module mod.com

go 1.18
-- mod/a/a.go --
package a

func Test() {
}
`
	Run(t, pkg, func(t *testing.T, env *Env) {
		env.OpenFile("caller/caller.go")
		env.AfterChange(Diagnostics(env.AtRegexp("caller/caller.go", "a.Test")))

		// Saving caller.go should trigger goimports, which should find a.Test in
		// the mod.com module, thanks to the go.work file.
		env.SaveBuffer("caller/caller.go")
		env.AfterChange(NoDiagnostics(ForFile("caller/caller.go")))
	})
}
