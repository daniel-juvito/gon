package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Package-loading failures (v1.15, Protocol v1 §7, inventory B).
//
// A package that cannot be loaded at all (the go command or the module is
// broken, a .gon file is excluded by build constraints) is exit 2 with no
// envelope, in every command that loads packages. A package-mode parse error
// the loader reports, including an invalid package clause, is a Go error of a
// checked package: envelope, exit 1, the loader's message on stderr.

const (
	exitOK      = "package p\n\nfunc F() {}\n"
	exitIgnored = "//go:build ignore\n\npackage p\n"
	exitNoPkg   = "pakage p\n"
	exitGN001   = "package p\n\nfunc f() !*int {\n\treturn nil\n}\n"
	emptyEnv    = `{"schemaVersion":1,"diagnostics":[]}`
)

// R4 / R4b: the excluded file sorts first or last; both are exit 2 with the
// same message, in --json and human mode.
func TestCheckExcludedFileIsExit2WhateverTheOrder(t *testing.T) {
	var msgs []string
	for _, tc := range []struct{ name, a, b, excluded string }{
		{"R4 excluded first", exitIgnored, exitOK, "p/a.gon"},
		{"R4b excluded last", exitOK, exitIgnored, "p/b.gon"},
	} {
		root := pkgModule(t, map[string]string{"p/a.gon": tc.a, "p/b.gon": tc.b})
		chdir(t, root)
		for _, args := range [][]string{{"check", "--json", "./p"}, {"check", "./p"}, {"vet", "--json", "./p"}} {
			exit, out, errOut := captureRun(args)
			if exit != 2 || out != "" {
				t.Fatalf("%s %v: exit %d (want 2), stdout %q (want none)\n%s", tc.name, args, exit, out, errOut)
			}
			if !strings.Contains(errOut, "excluded by build constraints: "+tc.excluded) {
				t.Fatalf("%s %v: stderr must name %s:\n%s", tc.name, args, tc.excluded, errOut)
			}
		}
		_, _, errOut := captureRun([]string{"check", "--json", "./p"})
		msgs = append(msgs, strings.Replace(errOut, tc.excluded, "<excluded>", 1))
	}
	if msgs[0] != msgs[1] {
		t.Fatalf("the message must not depend on file order:\n%s\n%s", msgs[0], msgs[1])
	}
}

// R12: an invalid go.mod is a configuration failure, not a Go type error.
func TestCheckBrokenGoModIsExit2(t *testing.T) {
	root := pkgModule(t, map[string]string{"go.mod": "garbage\n", "p/a.gon": exitOK})
	chdir(t, root)
	for _, args := range [][]string{{"check", "--json", "./p"}, {"check", "./p"}} {
		exit, out, errOut := captureRun(args)
		if exit != 2 || out != "" || !strings.Contains(errOut, "errors parsing go.mod") {
			t.Fatalf("%v: exit %d (want 2), stdout %q\n%s", args, exit, out, errOut)
		}
	}
}

// R5: no go command is an environment failure.
func TestCheckWithoutGoCommandIsExit2(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitOK})
	chdir(t, root)
	t.Setenv("PATH", t.TempDir())
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	if exit != 2 || out != "" || !strings.Contains(errOut, "go command required") {
		t.Fatalf("exit %d (want 2), stdout %q\n%s", exit, out, errOut)
	}
}

// R13: an invalid package clause is reported like any package-mode parse
// error: envelope, exit 1, the loader's message on stderr; never as a build
// constraint.
func TestCheckInvalidPackageClauseIsExit1WithEnvelope(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitNoPkg})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	if exit != 1 {
		t.Fatalf("exit %d, want 1\n%s", exit, errOut)
	}
	env := decodeProtocolEnvelope(t, out)
	assertEnvelopeBasics(t, env)
	if len(env.Diagnostics) != 0 {
		t.Fatalf("a Go parse error is not a Protocol diagnostic: %+v", env.Diagnostics)
	}
	if !strings.Contains(errOut, "expected 'package', found pakage") {
		t.Fatalf("stderr must carry the loader's parse error:\n%s", errOut)
	}
	if strings.Contains(errOut, "build constraint") || strings.Contains(errOut, "not part of") {
		t.Fatalf("stderr must not blame a build constraint:\n%s", errOut)
	}
}

func TestCheckInvalidPackageClauseKeepsSiblingDiagnostics(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitNoPkg, "p/b.gon": exitGN001})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	if exit != 1 {
		t.Fatalf("exit %d, want 1\n%s", exit, errOut)
	}
	env := decodeProtocolEnvelope(t, out)
	if len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" || !strings.HasSuffix(env.Diagnostics[0].File, "b.gon") {
		t.Fatalf("b.gon's GN001 must be in the envelope: %+v", env.Diagnostics)
	}
	if !strings.Contains(errOut, "expected 'package', found pakage") {
		t.Fatalf("a.gon's parse error must be on stderr:\n%s", errOut)
	}
}

// transpile / build share loadPackage: a package that cannot be loaded is
// exit 2 there too, and nothing is written.
func TestEmitDirLoadFailureIsExit2(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"R4 excluded file", map[string]string{"p/a.gon": exitIgnored, "p/b.gon": exitOK}, "excluded by build constraints: p/a.gon"},
		{"R12 broken go.mod", map[string]string{"go.mod": "garbage\n", "p/a.gon": exitOK}, "errors parsing go.mod"},
	} {
		for _, cmd := range []string{"transpile", "build"} {
			t.Run(tc.name+"/"+cmd, func(t *testing.T) {
				files := make(map[string]string, len(tc.files))
				for k, v := range tc.files {
					files[k] = v
				}
				root := pkgModule(t, files)
				chdir(t, root)
				exit, out, errOut := captureRun([]string{cmd, "./p"})
				if exit != 2 || !strings.Contains(errOut, tc.want) {
					t.Fatalf("exit %d (want 2)\nstdout: %s\nstderr: %s", exit, out, errOut)
				}
				if exists(filepath.Join(root, "p", "b.go")) || exists(filepath.Join(root, "p", "a.go")) {
					t.Fatal("nothing may be written for a package that could not be loaded")
				}
			})
		}
	}
}

func TestEmitDirWithoutGoCommandIsExit2(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitOK})
	chdir(t, root)
	t.Setenv("PATH", t.TempDir())
	exit, _, errOut := captureRun([]string{"transpile", "./p"})
	if exit != 2 || !strings.Contains(errOut, "go command required") {
		t.Fatalf("exit %d (want 2)\n%s", exit, errOut)
	}
}

// R13 in transpile: a parse error of a loaded package does not pass (exit 1).
func TestTranspileDirInvalidPackageClauseIsExit1(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitNoPkg})
	chdir(t, root)
	exit, _, errOut := captureRun([]string{"transpile", "./p"})
	if exit != 1 || !strings.Contains(errOut, "expected 'package', found pakage") || exists(filepath.Join(root, "p", "a.go")) {
		t.Fatalf("exit %d (want 1), a.go written: %v\n%s", exit, exists(filepath.Join(root, "p", "a.go")), errOut)
	}
}

// Behaviour the change must not touch (inventory B, measured on v1.14.0).
func TestCheckPackageOutcomesUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		args     []string
		exit     int
		stdout   string // exact, or "" to only require an envelope
		hasCodes []string
	}{
		{"R3 package syntax error", map[string]string{"p/a.gon": "package p\n\nfunc f() {\n"}, []string{"./p"}, 1, emptyEnv, nil},
		{"R6 two package names", map[string]string{"p/a.gon": exitOK, "p/b.gon": "package q\n"}, []string{"./p"}, 1, emptyEnv, nil},
		{"R10 package type error", map[string]string{"p/a.gon": "package p\n\nfunc f() int {\n\treturn \"x\"\n}\n"}, []string{"./p"}, 1, emptyEnv, nil},
		{"R9 file type error", map[string]string{"a.gon": "package p\n\nfunc f() int {\n\treturn \"x\"\n}\n"}, []string{"a.gon"}, 1, emptyEnv, nil},
		{"R15 broken imported .gon", map[string]string{"q/q.gon": "package q\n\nfunc F( {\n", "p/p.gon": "package p\n\nimport \"pk/q\"\n\nvar _ = q.F\n"}, []string{"./p"}, 1, "", []string{"GN003"}},
		{"R16 malformed .gna", map[string]string{"lib/lib.go": "package lib\n\nfunc G() *int { return nil }\n", "annotations/pk/lib.gna": "garbage ((( \n", "p/p.gon": "package p\n\nimport \"pk/lib\"\n\nfunc H() {\n\tx := lib.G()\n\t_ = x\n}\n"}, []string{"./p"}, 1, "", []string{"GN003"}},
		{"R17b only an ignored file", map[string]string{"p/a.gon": exitIgnored}, []string{"./p"}, 0, emptyEnv, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := pkgModule(t, tc.files)
			chdir(t, root)
			exit, out, errOut := captureRun(append([]string{"check", "--json"}, tc.args...))
			if exit != tc.exit {
				t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", exit, tc.exit, out, errOut)
			}
			if tc.stdout != "" && strings.TrimSpace(out) != tc.stdout {
				t.Fatalf("stdout %q, want %q", out, tc.stdout)
			}
			env := decodeProtocolEnvelope(t, out)
			assertEnvelopeBasics(t, env)
			for i, code := range tc.hasCodes {
				if i >= len(env.Diagnostics) || env.Diagnostics[i].Code != code {
					t.Fatalf("want %v, got %+v", tc.hasCodes, env.Diagnostics)
				}
			}
		})
	}
}

// R8: package mode without a go.mod keeps working (the reason the loader query
// stays file=).
func TestCheckPackageWithoutGoModUnchanged(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "p", "a.gon"), []byte(exitOK), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	if exit != 0 || strings.TrimSpace(out) != emptyEnv {
		t.Fatalf("exit %d, stdout %q\n%s", exit, out, errOut)
	}
}
