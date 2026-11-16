package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Directory form of `transpile` and `build` (v1.12 / M9).
//
// Executable specification for docs/rfc-transpile-dir.md.

const tdTypes = "package pk\n\ntype T struct{}\n\ntype Box struct{ P  !*T }\n"
const tdUse = "package pk\n\nfunc Make() Box { return Box{P: &T{}} }\n"

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// T1/T3: the files of a package use each other, so they can only be checked
// together; each gets a formatted .go next to it.
func TestTranspileDirWritesEveryFileOfTheName(t *testing.T) {
	root := pkgModule(t, map[string]string{"a.gon": tdTypes, "b.gon": tdUse})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"transpile", "."})
	if exit != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", exit, out, errOut)
	}
	for _, f := range []string{"a.go", "b.go"} {
		if !exists(filepath.Join(root, f)) {
			t.Fatalf("%s was not written", f)
		}
	}
	// the `!` is gone and the source is gofmt-formatted
	if got := readFile(t, filepath.Join(root, "a.go")); strings.Contains(got, "!") || !strings.Contains(got, "type Box struct{ P *T }") {
		t.Fatalf("a.go:\n%s", got)
	}
	if !strings.Contains(out, "transpiled: a.go") || !strings.Contains(out, "transpiled: b.go") {
		t.Fatalf("one `transpiled:` line per file expected:\n%s", out)
	}
}

// The same files transpiled one at a time fail (the other file is invisible);
// this is why the directory form exists.
func TestTranspileFileFormStillChecksOneFileAlone(t *testing.T) {
	root := pkgModule(t, map[string]string{"a.gon": tdTypes, "b.gon": tdUse})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"transpile", "b.gon"})
	if exit != 1 || !strings.Contains(errOut, "Go type error") {
		t.Fatalf("file form must be unchanged: exit %d\n%s", exit, errOut)
	}
	if exists(filepath.Join(root, "b.go")) {
		t.Fatal("the file form must not have written b.go")
	}
}

// T3: a same-named .go is replaced, any other .go is untouched, a stale .go with
// no .gon is not deleted, and *_test.gon is skipped.
func TestTranspileDirReplacesOnlyTheNamedPairs(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon":           tdTypes,
		"a.go":            "package pk\n\n// the old emitted file\n",
		"hand.go":         "package pk\n\nfunc Hand() {}\n",
		"gone.go":         "package pk\n\nfunc Gone() {}\n",
		"helper_test.gon": "package pk\n\nfunc helper() { var p !*T = nil; _ = p }\n",
	})
	chdir(t, root)

	if exit, out, errOut := captureRun([]string{"transpile", "."}); exit != 0 {
		t.Fatalf("exit %d\n%s\n%s", exit, out, errOut)
	}
	if got := readFile(t, filepath.Join(root, "a.go")); strings.Contains(got, "old emitted") {
		t.Fatalf("a.go must be replaced:\n%s", got)
	}
	if got := readFile(t, filepath.Join(root, "hand.go")); !strings.Contains(got, "func Hand()") {
		t.Fatalf("an unrelated .go must not be touched:\n%s", got)
	}
	if !exists(filepath.Join(root, "gone.go")) {
		t.Fatal("a .go without a .gon must not be deleted")
	}
	if exists(filepath.Join(root, "helper_test.go")) {
		t.Fatal("*_test.gon must not be transpiled in directory mode")
	}
}

// T2: a contract error anywhere in the package writes NOTHING, not even the
// files that are fine, and an existing same-named .go stays as it was.
func TestTranspileDirContractErrorWritesNothing(t *testing.T) {
	old := "package pk\n\n// previous version\n"
	root := pkgModule(t, map[string]string{
		"a.gon": tdTypes,
		"b.gon": "package pk\n\nfunc Bad() { _ = Box{P: nil} }\n",
		"a.go":  old,
	})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"transpile", "."})
	if exit != 1 || !strings.Contains(errOut, "GN001") || !strings.Contains(errOut, "nothing was written") {
		t.Fatalf("exit %d\n%s", exit, errOut)
	}
	if got := readFile(t, filepath.Join(root, "a.go")); got != old {
		t.Fatalf("a.go must be untouched:\n%s", got)
	}
	if exists(filepath.Join(root, "b.go")) {
		t.Fatal("b.go must not exist")
	}
}

// T2: a Go type error writes nothing either.
func TestTranspileDirGoTypeErrorWritesNothing(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": tdTypes,
		"b.gon": "package pk\n\nfunc Broken() { undefinedThing() }\n",
	})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"transpile", "."})
	if exit != 1 || !strings.Contains(errOut, "Go type error") {
		t.Fatalf("exit %d\n%s", exit, errOut)
	}
	if exists(filepath.Join(root, "a.go")) || exists(filepath.Join(root, "b.go")) {
		t.Fatal("nothing may be written for a package with a Go type error")
	}
}

// Warnings are printed and do not block the write.
func TestTranspileDirWarningDoesNotBlock(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": "package pk\n\nfunc F(p !*int) { if p == nil {} }\n",
	})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"transpile", "."})
	if exit != 0 || !strings.Contains(errOut, "GW001") || !exists(filepath.Join(root, "a.go")) {
		t.Fatalf("exit %d\n%s", exit, errOut)
	}
}

// `./...`: every package is checked and reported; only the ones that pass are
// written; the exit code is 1 if any failed.
func TestTranspileDirPatternWritesOnlyPassingPackages(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"good/g.gon": "package good\n\nfunc G() int { return 1 }\n",
		"bad/b.gon":  "package bad\n\nfunc B() { undefinedThing() }\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"transpile", "./..."})
	if exit != 1 {
		t.Fatalf("exit %d, want 1\n%s\n%s", exit, out, errOut)
	}
	if !exists(filepath.Join(root, "good", "g.go")) {
		t.Fatal("the passing package must be written")
	}
	if exists(filepath.Join(root, "bad", "b.go")) {
		t.Fatal("the failing package must not be written")
	}
	if !strings.Contains(errOut, "bad") {
		t.Fatalf("the failing package should be reported:\n%s", errOut)
	}
}

// A fresh tree has no .go, so a package must be transpiled after the packages it
// imports: alphabetical order would check `app` against nothing and fail.
func TestTranspileDirPatternOrdersByImports(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"app/app.gon": "package app\n\nimport \"pk/zlib\"\n\nfunc Run() int { return zlib.Answer() }\n",
		"zlib/z.gon":  "package zlib\n\nfunc Answer() int { return 42 }\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"transpile", "./..."})
	if exit != 0 {
		t.Fatalf("exit %d\n%s\n%s", exit, out, errOut)
	}
	if !exists(filepath.Join(root, "app", "app.go")) || !exists(filepath.Join(root, "zlib", "z.go")) {
		t.Fatalf("both packages must be written:\n%s", out)
	}
}

// T5/T6: nothing to do is exit 2, with the `check` wording.
func TestTranspileDirWithoutGonFilesIsExit2(t *testing.T) {
	root := pkgModule(t, map[string]string{"main.go": "package main\n\nfunc main() {}\n"})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"transpile", "."})
	if exit != 2 || !strings.Contains(errOut, "no .gon files") {
		t.Fatalf("exit %d\n%s", exit, errOut)
	}
}

func TestTranspileDirCannotMixFilesAndDirectories(t *testing.T) {
	root := pkgModule(t, map[string]string{"a.gon": tdTypes, "sub/s.gon": "package sub\n"})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"transpile", "sub", "a.gon"})
	if exit != 2 || !strings.Contains(errOut, "cannot mix") {
		t.Fatalf("exit %d\n%s", exit, errOut)
	}
}

// --- build ----------------------------------------------------------------

// T4: transpile, then `go build <dir>`; a main package leaves a binary named
// after the directory (go build's own rule), nothing is forced.
func TestBuildDirMainPackage(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"cmd/app/main.gon": "package main\n\nfunc main() { println(helper()) }\n",
		"cmd/app/help.gon": "package main\n\nfunc helper() int { return 7 }\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"build", "./cmd/app"})
	if exit != 0 {
		t.Fatalf("exit %d\n%s\n%s", exit, out, errOut)
	}
	if !exists(filepath.Join(root, "app")) && !exists(filepath.Join(root, "app.exe")) {
		t.Fatalf("go build should have written ./app:\n%s", out)
	}
	if !strings.Contains(out, "built: ./cmd/app") {
		t.Fatalf("missing `built:` line:\n%s", out)
	}
}

// Flags after the target go to `go build`: here -o.
func TestBuildDirPassesFlagsThrough(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"main.gon": "package main\n\nfunc main() {}\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"build", ".", "-o", filepath.Join(root, "out", "prog")})
	if exit != 0 {
		t.Fatalf("exit %d\n%s\n%s", exit, out, errOut)
	}
	if !exists(filepath.Join(root, "out", "prog")) {
		t.Fatal("-o must reach go build")
	}
}

// A package that does not pass is not built: no .go, no go build.
func TestBuildDirDoesNotBuildAFailingPackage(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"main.gon": "package main\n\nfunc main() { undefinedThing() }\n",
	})
	chdir(t, root)

	exit, out, _ := captureRun([]string{"build", "."})
	if exit != 1 || strings.Contains(out, "built:") || exists(filepath.Join(root, "main.go")) {
		t.Fatalf("exit %d\n%s", exit, out)
	}
}

// A library package compiles without leaving an output.
func TestBuildDirLibraryPackage(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"lib/l.gon": "package lib\n\nfunc L() int { return 1 }\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"build", "./lib"})
	if exit != 0 {
		t.Fatalf("exit %d\n%s\n%s", exit, out, errOut)
	}
}

// `go build` reads `internal/foo` as an import path; the target must be a
// directory pattern.
func TestGoPackageArg(t *testing.T) {
	for in, want := range map[string]string{
		"internal/foo": "./internal/foo",
		"./x":          "./x",
		"../x":         "../x",
		".":            ".",
		"...":          "...",
		"./...":        "./...",
		"a/...":        "./a/...",
	} {
		if got := goPackageArg(in); got != want {
			t.Errorf("goPackageArg(%q) = %q, want %q", in, got, want)
		}
	}
}
