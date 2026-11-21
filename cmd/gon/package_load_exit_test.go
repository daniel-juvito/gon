package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Package-loading failures (v1.15, Protocol v1 §7, inventory B) and package
// membership (v1.16, package-discovery audit).
//
// A package that cannot be loaded at all (the go command or the module is
// broken, no source of an explicitly named package is in the build) is exit 2
// with no envelope, in every command that loads packages. A .gon excluded by
// build constraints is not part of the package and is not checked; a package
// found by `...` none of whose sources is in the build is skipped. A
// package-mode parse error the loader reports, including an invalid package
// clause, is a Go error of a checked package: envelope, exit 1, the loader's
// message on stderr.

const (
	exitOK      = "package p\n\nfunc F() {}\n"
	exitIgnored = "//go:build ignore\n\npackage p\n"
	exitNoPkg   = "pakage p\n"
	exitGN001   = "package p\n\nfunc f() !*int {\n\treturn nil\n}\n"
	emptyEnv    = `{"schemaVersion":1,"diagnostics":[]}`

	exitIgnoredGN001 = "//go:build ignore\n\npackage p\n\nfunc f() !*int {\n\treturn nil\n}\n"
	exitGN001g       = "package p\n\nfunc g() !*int {\n\treturn nil\n}\n"
)

// R4 / R4b, D1g / D1h: the excluded file sorts first or last. It is not
// checked (its GN001 is never reported), the other file is, and the result is
// the same in both orders, in --json, human and vet mode.
func TestCheckExcludedFileIsSkippedWhateverTheOrder(t *testing.T) {
	var results []string
	for _, tc := range []struct{ name, a, b, checked string }{
		{"excluded file first", exitIgnoredGN001, exitGN001g, "b.gon"},
		{"excluded file last", exitGN001g, exitIgnoredGN001, "a.gon"},
	} {
		root := pkgModule(t, map[string]string{"p/a.gon": tc.a, "p/b.gon": tc.b})
		chdir(t, root)
		exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
		if exit != 1 || errOut != "" {
			t.Fatalf("%s: exit %d (want 1)\n%s", tc.name, exit, errOut)
		}
		env := decodeProtocolEnvelope(t, out)
		if len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" || filepath.Base(env.Diagnostics[0].File) != tc.checked {
			t.Fatalf("%s: want one GN001 in %s, got %+v", tc.name, tc.checked, env.Diagnostics)
		}
		// Compare the decoded values, not the raw JSON: JSON escapes the
		// backslashes of a Windows path, so replacing the directory in the text
		// would miss them. The path is made relative to the directory of this
		// iteration, with the checked file's name blanked.
		norm := env
		norm.Diagnostics = append([]protocolDiagnostic(nil), env.Diagnostics...)
		for i := range norm.Diagnostics {
			rel, err := filepath.Rel(root, norm.Diagnostics[i].File)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			norm.Diagnostics[i].File = strings.ReplaceAll(filepath.ToSlash(rel), tc.checked, "<checked>")
		}
		normJSON, err := json.Marshal(norm)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, string(normJSON))
		for _, args := range [][]string{{"check", "./p"}, {"vet", "--json", "./p"}} {
			if exit, _, errOut := captureRun(args); exit != 1 || strings.Contains(errOut, "excluded") {
				t.Fatalf("%s %v: exit %d (want 1)\n%s", tc.name, args, exit, errOut)
			}
		}
	}
	if results[0] != results[1] {
		t.Fatalf("the result must not depend on file order:\n%s\n%s", results[0], results[1])
	}
}

// S3, D1a / D1b / D1c / D1f: no source of an explicitly named package is in the
// build. Exit 2, no envelope, every .gon named and sorted; an excluded file's
// GN001 is not reported.
func TestCheckAllSourcesExcludedIsExit2(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"D1a one ignored file", map[string]string{"p/a.gon": exitIgnored}, "p/a.gon"},
		{"D1b one ignored file with GN001", map[string]string{"p/a.gon": exitIgnoredGN001}, "p/a.gon"},
		{"D1c GOOS suffix", map[string]string{"p/x_plan9.gon": exitGN001}, "p/x_plan9.gon"},
		{"D1f two ignored files", map[string]string{"p/b.gon": exitIgnoredGN001, "p/a.gon": exitIgnored}, "p/a.gon, p/b.gon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := pkgModule(t, tc.files)
			chdir(t, root)
			want := "gon: build constraints exclude all Go files in ./p (.gon files: " + tc.want + ")\n"
			for _, args := range [][]string{{"check", "--json", "./p"}, {"check", "./p"}, {"vet", "--json", "./p"}} {
				exit, out, errOut := captureRun(args)
				if exit != 2 || out != "" || errOut != want {
					t.Fatalf("%v: exit %d (want 2), stdout %q\nstderr %q\nwant   %q", args, exit, out, errOut, want)
				}
			}
		})
	}
}

// S2, D1d: the only .gon is excluded, a plain .go file is in the build. The
// package is checked from its .go file: exit 0, empty envelope, nothing on
// stderr.
func TestCheckPackageWithOnlyExcludedGonIsCheckedFromItsGoFiles(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitIgnoredGN001, "p/b.go": "package p\n\nfunc G() {}\n"})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	if exit != 0 || strings.TrimSpace(out) != emptyEnv || errOut != "" {
		t.Fatalf("exit %d, stdout %q\n%s", exit, out, errOut)
	}
}

// D1e: `./...` skips the package none of whose sources is in the build and
// checks the others, with no warning.
func TestCheckPatternSkipsPackageWithNoSourceInTheBuild(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitIgnoredGN001, "q/q.gon": "package q\n\nfunc f() !*int {\n\treturn nil\n}\n"})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./..."})
	if exit != 1 || errOut != "" {
		t.Fatalf("exit %d (want 1)\n%s", exit, errOut)
	}
	env := decodeProtocolEnvelope(t, out)
	if len(env.Diagnostics) != 1 || filepath.Base(env.Diagnostics[0].File) != "q.gon" {
		t.Fatalf("only q.gon's GN001: %+v", env.Diagnostics)
	}
}

// D1j: every package `./...` finds is skipped: exit 0, empty envelope, and a
// warning like the go command's.
func TestCheckPatternMatchingNoPackageInTheBuildWarns(t *testing.T) {
	root := pkgModule(t, map[string]string{"p/a.gon": exitIgnored, "q/b.gon": "//go:build ignore\n\npackage q\n"})
	chdir(t, root)
	for _, args := range [][]string{{"check", "--json", "./..."}, {"check", "./..."}} {
		exit, out, errOut := captureRun(args)
		if exit != 0 || errOut != "gon: warning: \"./...\" matched no packages with .gon files in the build\n" {
			t.Fatalf("%v: exit %d, stdout %q\nstderr %q", args, exit, out, errOut)
		}
		if args[1] == "--json" && strings.TrimSpace(out) != emptyEnv {
			t.Fatalf("stdout %q", out)
		}
	}
}

// D1k: the go command decides membership, so a GOFLAGS tag is honoured.
func TestCheckGOFLAGSTagDecidesMembership(t *testing.T) {
	t.Setenv("GOFLAGS", "-tags=gonprobetag")
	root := pkgModule(t, map[string]string{"p/a.gon": "//go:build !gonprobetag\n\npackage p\n\nfunc f() !*int {\n\treturn nil\n}\n", "p/b.gon": exitGN001g})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	env := decodeProtocolEnvelope(t, out)
	if exit != 1 || len(env.Diagnostics) != 1 || filepath.Base(env.Diagnostics[0].File) != "b.gon" {
		t.Fatalf("exit %d, %+v\n%s", exit, env.Diagnostics, errOut)
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
		{"S3 no source in the build", map[string]string{"p/a.gon": exitIgnored}, "build constraints exclude all Go files in ./p (.gon files: p/a.gon)"},
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
