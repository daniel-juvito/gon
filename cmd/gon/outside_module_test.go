package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Package mode outside any module (v1.18, package-discovery audit, proposal
// 3). go/build decides which files form the package (the toolchain's default
// context; GOFLAGS tags are not read) and all of them are loaded as one ad-hoc
// package. Import resolution is what it was: only the standard library
// resolves.

const outGN001 = "func f() !*int {\n\treturn nil\n}\n"

// outsideModule writes files under a fresh directory with no go.mod above it
// and makes it the working directory.
func outsideModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for d := root; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			t.Skipf("a go.mod above the temp dir (%s): not outside a module", d)
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, root)
	return root
}

func outEnvelope(t *testing.T, args ...string) (int, protocolEnvelope, string) {
	t.Helper()
	exit, out, errOut := captureRun(args)
	if out == "" {
		return exit, protocolEnvelope{}, errOut
	}
	return exit, decodeProtocolEnvelope(t, out), errOut
}

// D3b, the main regression: a contract declared in one file is applied in the
// other (it was exit 2, "not part of the loaded package").
func TestOutsideModuleMultiFilePackage(t *testing.T) {
	outsideModule(t, map[string]string{
		"two/a.gon": "package two\n\nfunc Take(p !*int) {}\n",
		"two/b.gon": "package two\n\nfunc use() { Take(nil) }\n",
	})
	for _, cmd := range []string{"check", "vet"} {
		exit, env, errOut := outEnvelope(t, cmd, "--json", "./two")
		if exit != 1 || len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" || filepath.Base(env.Diagnostics[0].File) != "b.gon" {
			t.Fatalf("%s: exit %d, %+v\n%s", cmd, exit, env.Diagnostics, errOut)
		}
		if strings.Contains(errOut, "not part of") {
			t.Fatalf("%s: %s", cmd, errOut)
		}
	}
	exit, _, errOut := captureRun([]string{"check", "./two"})
	if exit != 1 || !strings.Contains(errOut, "two/b.gon:3:19: error GN001") {
		t.Fatalf("human: exit %d\n%s", exit, errOut)
	}
}

// D3b2: a plain .go file of the directory is part of the package: its
// declarations are visible and its own errors are reported.
func TestOutsideModulePlainGoFilesAreMembers(t *testing.T) {
	outsideModule(t, map[string]string{
		"mix/a.gon": "package mix\n\nfunc F() { helper() }\n",
		"mix/b.go":  "package mix\n\nfunc helper() {}\n",
		"bad/a.gon": "package bad\n\nfunc F() {}\n",
		"bad/b.go":  "package bad\n\nfunc g() int { return \"x\" }\n",
	})
	if exit, env, errOut := outEnvelope(t, "check", "--json", "./mix"); exit != 0 || len(env.Diagnostics) != 0 || errOut != "" {
		t.Fatalf("mix: exit %d %+v\n%s", exit, env.Diagnostics, errOut)
	}
	if exit, _, errOut := outEnvelope(t, "check", "--json", "./bad"); exit != 1 || !strings.Contains(errOut, "gon: Go type error: ") || !strings.Contains(errOut, "b.go") {
		t.Fatalf("bad: exit %d\n%s", exit, errOut)
	}
}

// D3c, P7 and a GOPATH package: loading every file does not widen import
// resolution; each import is still a Go type error, as in v1.17.
func TestOutsideModuleImportsUnchanged(t *testing.T) {
	root := outsideModule(t, map[string]string{
		"imp/a.gon":              "package imp\n\nimport \"lib\"\n\nvar _ = lib.G\n",
		"imp/b.gon":              "package imp\n",
		"lib/l.gon":              "package lib\n\nfunc G() int { return 1 }\n",
		"rel/a.gon":              "package rel\n\nimport \"./sub\"\n\nvar _ = sub.G\n",
		"rel/b.gon":              "package rel\n",
		"rel/sub/s.go":           "package sub\n\nfunc G() int { return 1 }\n",
		"gp/a.gon":               "package gp\n\nimport \"gpkg/x\"\n\nvar _ = x.G\n",
		"gp/b.gon":               "package gp\n",
		"gopath/src/gpkg/x/x.go": "package x\n\nfunc G() int { return 1 }\n",
		"std/a.gon":              "package std\n\nimport \"strings\"\n\nvar _ = strings.ToUpper\n",
		"std/b.gon":              "package std\n",
	})
	t.Setenv("GOPATH", filepath.Join(root, "gopath"))
	for dir, imp := range map[string]string{"./imp": "lib", "./rel": "./sub", "./gp": "gpkg/x"} {
		exit, _, errOut := outEnvelope(t, "check", "--json", dir)
		if exit != 1 || !strings.Contains(errOut, "gon: Go type error: ") || !strings.Contains(errOut, "could not import "+imp) {
			t.Fatalf("%s: exit %d (want 1, could not import %s)\n%s", dir, exit, imp, errOut)
		}
	}
	if exit, _, errOut := outEnvelope(t, "check", "--json", "./std"); exit != 0 {
		t.Fatalf("std: exit %d\n%s", exit, errOut)
	}
}

// P1-P3, P8: build constraints as in a module. An excluded .gon is not
// checked; no source in the build is exit 2 named explicitly and skipped
// under `...`; an excluded .gon next to a plain .go file is checked from it.
func TestOutsideModuleBuildConstraints(t *testing.T) {
	outsideModule(t, map[string]string{
		"bc/a.gon":        "package bc\n\n" + outGN001,
		"bc/b.gon":        "//go:build ignore\n\npackage bc\n\nfunc g() !*int {\n\treturn nil\n}\n",
		"all/a.gon":       "//go:build ignore\n\npackage all\n\n" + outGN001,
		"all/b_plan9.gon": "package all\n\n" + "func g() {}\n",
		"gogo/a.gon":      "//go:build ignore\n\npackage gogo\n\n" + outGN001,
		"gogo/b.go":       "package gogo\n\nfunc G() {}\n",
		"pat/x/x.gon":     "package x\n\n" + outGN001,
		"pat/z/z.gon":     "//go:build ignore\n\npackage z\n\n" + outGN001,
		"none/z/z.gon":    "//go:build ignore\n\npackage z\n",
	})
	exit, env, errOut := outEnvelope(t, "check", "--json", "./bc")
	if exit != 1 || len(env.Diagnostics) != 1 || filepath.Base(env.Diagnostics[0].File) != "a.gon" || errOut != "" {
		t.Fatalf("P1: exit %d %+v\n%s", exit, env.Diagnostics, errOut)
	}
	want := "gon: build constraints exclude all Go files in ./all (.gon files: " + filepath.FromSlash("all/a.gon") + ", " + filepath.FromSlash("all/b_plan9.gon") + ")\n"
	for _, cmd := range []string{"check", "vet", "transpile", "build"} {
		exit, out, errOut := captureRun([]string{cmd, "./all"})
		if exit != 2 || out != "" || errOut != want {
			t.Fatalf("P2 %s: exit %d, stdout %q\nstderr %q\nwant   %q", cmd, exit, out, errOut, want)
		}
	}
	if exit, env, errOut := outEnvelope(t, "check", "--json", "./gogo"); exit != 0 || len(env.Diagnostics) != 0 || errOut != "" {
		t.Fatalf("P3: exit %d %+v\n%s", exit, env.Diagnostics, errOut)
	}
	exit, env, errOut = outEnvelope(t, "check", "--json", "./pat/...")
	if exit != 1 || len(env.Diagnostics) != 1 || filepath.Base(env.Diagnostics[0].File) != "x.gon" || errOut != "" {
		t.Fatalf("P8: exit %d %+v\n%s", exit, env.Diagnostics, errOut)
	}
	if exit, _, errOut := outEnvelope(t, "check", "--json", "./none/..."); exit != 0 || errOut != "gon: warning: \"./none/...\" matched no packages with .gon files in the build\n" {
		t.Fatalf("all skipped: exit %d\n%q", exit, errOut)
	}
}

// P4, P5: two package names and an invalid package clause are Go errors of the
// package (exit 1, envelope), as in a module.
func TestOutsideModulePackageErrors(t *testing.T) {
	outsideModule(t, map[string]string{
		"names/a.gon": "package x\n",
		"names/b.gon": "package y\n",
		"nopkg/a.gon": "pakage p\n",
		"nopkg/b.gon": "package p\n\n" + outGN001,
	})
	if exit, env, errOut := outEnvelope(t, "check", "--json", "./names"); exit != 1 || len(env.Diagnostics) != 0 || !strings.Contains(errOut, "found packages x (a.go) and y (b.go)") {
		t.Fatalf("P4: exit %d\n%s", exit, errOut)
	}
	exit, env, errOut := outEnvelope(t, "check", "--json", "./nopkg")
	if exit != 1 || len(env.Diagnostics) != 1 || filepath.Base(env.Diagnostics[0].File) != "b.gon" || !strings.Contains(errOut, "expected 'package', found pakage") {
		t.Fatalf("P5: exit %d %+v\n%s", exit, env.Diagnostics, errOut)
	}
}

// P10 and file names: a .go shadowed by a .gon, a test file, and files whose
// names start with `_` or `.` are not loaded (each would break the package).
func TestOutsideModuleFilesThatAreNotMembers(t *testing.T) {
	outsideModule(t, map[string]string{
		"p/a.gon":     "package p\n\nfunc Take(p !*int) {}\n",
		"p/a.go":      "package p\n\nfunc Take(p *int, extra int) {}\n",
		"p/b.gon":     "package p\n\nfunc use() { Take(nil) }\n",
		"p/c_test.go": "package p\n\nfunc broken( {\n",
		"p/_d.go":     "package p\n\nfunc broken( {\n",
		"p/.e.go":     "package p\n\nfunc broken( {\n",
	})
	exit, env, errOut := outEnvelope(t, "check", "--json", "./p")
	if exit != 1 || len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" || errOut != "" {
		t.Fatalf("exit %d %+v\n%s", exit, env.Diagnostics, errOut)
	}
}

// transpile and build use the same files as check: every member .gon is
// written, an excluded one is named and not written; when the package does
// not pass, nothing is written.
func TestOutsideModuleTranspileUsesTheSameFiles(t *testing.T) {
	root := outsideModule(t, map[string]string{
		"tp/a.gon":       "package tp\n\nfunc F() {}\n",
		"tp/b.gon":       "package tp\n\nfunc G() { F() }\n",
		"tp/c_plan9.gon": "package tp\n\nfunc P() {}\n",
		"fail/a.gon":     "package fail\n\nfunc Take(p !*int) {}\n",
		"fail/b.gon":     "package fail\n\nfunc use() { Take(nil) }\n",
	})
	exit, out, errOut := captureRun([]string{"transpile", "./tp"})
	if exit != 0 || out != "transpiled: "+filepath.FromSlash("tp/a.go")+"\ntranspiled: "+filepath.FromSlash("tp/b.go")+"\n" || errOut != "gon: tp/c_plan9.gon not transpiled: excluded by build constraints\n" {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", exit, out, errOut)
	}
	if exists(filepath.Join(root, "tp", "c_plan9.go")) {
		t.Fatal("the excluded file must not be written")
	}
	for _, cmd := range []string{"transpile", "build"} {
		exit, _, errOut := captureRun([]string{cmd, "./fail"})
		if exit != 1 || exists(filepath.Join(root, "fail", "a.go")) || exists(filepath.Join(root, "fail", "b.go")) {
			t.Fatalf("%s: exit %d (want 1), nothing written\n%s", cmd, exit, errOut)
		}
	}
}

// A malformed build constraint (go/build's MatchFile cannot evaluate it; the go
// command reports such a file as an invalid file of the package) is neither a
// member nor excluded: the file is not loaded and its error is a Go error of
// the package (exit 1, envelope, message on stderr naming the file), for a
// .gon and for a plain .go file alike. A .gon is named by its .gon path with
// go/build's reason ("gon: invalid build constraint in m/a.gon: ..."), the same
// text as inside a module (v1.19); a plain .go keeps its Go error text. It is never "no source in the build"
// (exit 2) and a `...` pattern does not skip the package.
func TestOutsideModuleMalformedBuildConstraint(t *testing.T) {
	root := outsideModule(t, map[string]string{
		"m/a.gon":     "//go:build (linux\n\npackage m\n\n" + outGN001,
		"m/b.gon":     "package m\n\nfunc g() !*int {\n\treturn nil\n}\n",
		"g/a.gon":     "package g\n\nfunc F() {}\n",
		"g/c.go":      "//go:build linux &&\n\npackage g\n\nfunc broken( {\n",
		"two/a.gon":   "//go:build ignore\n//go:build linux\n\npackage two\n",
		"only/a.gon":  "//go:build (linux\n\npackage only\n",
		"only/b.gon":  "//go:build ignore\n\npackage only\n",
		"pat/x/a.gon": "//go:build (linux\n\npackage x\n",
	})
	exit, env, errOut := outEnvelope(t, "check", "--json", "./m")
	if exit != 1 || len(env.Diagnostics) != 1 || filepath.Base(env.Diagnostics[0].File) != "b.gon" ||
		errOut != "gon: invalid build constraint in m/a.gon: parsing //go:build line: missing close paren\n" {
		t.Fatalf(".gon: exit %d %+v\nstderr %q", exit, env.Diagnostics, errOut)
	}
	exit, env, errOut = outEnvelope(t, "check", "--json", "./g")
	if exit != 1 || len(env.Diagnostics) != 0 ||
		errOut != "gon: Go type error: g/c.go: invalid build constraint: c.go: parsing //go:build line: unexpected end of expression\n" {
		t.Fatalf("plain .go: exit %d\nstderr %q", exit, errOut)
	}
	if exit, _, errOut := outEnvelope(t, "check", "--json", "./two"); exit != 1 || !strings.Contains(errOut, "gon: invalid build constraint in two/a.gon: multiple //go:build comments") {
		t.Fatalf("multiple //go:build: exit %d\n%s", exit, errOut)
	}
	for _, args := range [][]string{{"check", "--json", "./only"}, {"vet", "--json", "./only"}, {"check", "--json", "./pat/..."}} {
		exit, env, errOut := outEnvelope(t, args...)
		if exit != 1 || len(env.Diagnostics) != 0 || !strings.Contains(errOut, "gon: invalid build constraint in ") || strings.Contains(errOut, "exclude all Go files") || strings.Contains(errOut, "matched no packages") {
			t.Fatalf("%v: exit %d (want 1, not exit 2 and not skipped)\n%s", args, exit, errOut)
		}
	}
	for _, cmd := range []string{"transpile", "build"} {
		exit, _, errOut := captureRun([]string{cmd, "./m"})
		if exit != 1 || exists(filepath.Join(root, "m", "a.go")) || exists(filepath.Join(root, "m", "b.go")) {
			t.Fatalf("%s: exit %d (want 1), nothing written\n%s", cmd, exit, errOut)
		}
	}
}
