package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A malformed //go:build line in a .gon inside a module (v1.19, package
// discovery audit, proposal a1). It is a source error of the package, as the
// go command treats it: the file is not checked and the package's other files
// are; exit 1 with the envelope of those; the file named on stderr by its .gon
// path with go/build's reason. It is not a Diagnostic in Protocol v1 (no code,
// no range: go/build gives no position), so the envelope never holds an entry
// for it. Nothing is written for the package that has it.

const (
	mcOK2    = "package p\n\nfunc G() {}\n"
	mcTake   = "package p\n\nfunc Take(p !*int) {}\n"
	mcUseNil = "package p\n\nfunc use() { Take(nil) }\n"
)

var malformedForms = []struct{ name, header, reason string }{
	{"unbalanced parenthesis", "//go:build (linux\n", "parsing //go:build line: missing close paren"},
	{"incomplete expression", "//go:build linux &&\n", "parsing //go:build line: unexpected end of expression"},
	{"two //go:build lines", "//go:build ignore\n//go:build linux\n", "multiple //go:build comments"},
	{"empty //go:build after // +build", "// +build linux,\n//go:build\n", "parsing //go:build line: unexpected end of expression"},
}

func invalidLine(path, reason string) string {
	return "gon: invalid build constraint in " + path + ": " + reason + "\n"
}

func notTranspiled(dir string) string {
	return "gon: " + dir + " not transpiled: nothing was written for this package\n"
}

// Every command, every malformed form, the malformed file sorting first or
// last: exit 1, the same envelope, the exact stderr, nothing written.
func TestMalformedConstraintInModuleIsASourceError(t *testing.T) {
	for _, form := range malformedForms {
		for _, order := range []struct{ bad, good string }{{"a.gon", "b.gon"}, {"b.gon", "a.gon"}} {
			t.Run(form.name+"/bad is "+order.bad, func(t *testing.T) {
				root := pkgModule(t, map[string]string{
					"p/" + order.bad:  form.header + "\npackage p\n",
					"p/" + order.good: mcOK2,
				})
				chdir(t, root)
				want := invalidLine("p/"+order.bad, form.reason)
				for _, args := range [][]string{{"check", "--json", "./p"}, {"vet", "--json", "./p"}} {
					exit, out, errOut := captureRun(args)
					if exit != 1 || strings.TrimSpace(out) != emptyEnv || errOut != want {
						t.Fatalf("%v: exit %d (want 1), stdout %q\nstderr %q\nwant   %q", args, exit, out, errOut, want)
					}
				}
				if exit, out, errOut := captureRun([]string{"check", "./p"}); exit != 1 || out != "" || errOut != want {
					t.Fatalf("human: exit %d, stdout %q\nstderr %q", exit, out, errOut)
				}
				for _, cmd := range []string{"transpile", "build"} {
					exit, out, errOut := captureRun([]string{cmd, "./p"})
					if exit != 1 || out != "" || errOut != want+notTranspiled("./p") {
						t.Fatalf("%s: exit %d, stdout %q\nstderr %q", cmd, exit, out, errOut)
					}
				}
				entries, _ := os.ReadDir(filepath.Join(root, "p"))
				if len(entries) != 2 {
					t.Fatalf("nothing may be written: %v", entries)
				}
			})
		}
	}
}

// The package's other files are still checked: their diagnostics stay in the
// envelope, the malformed file's own contents are not checked, and a contract
// declared in the malformed file's neighbours is applied across them.
func TestMalformedConstraintKeepsSiblingDiagnostics(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"p/a.gon": mcTake,
		"p/b.gon": "//go:build (linux\n\npackage p\n\nfunc own() !*int {\n\treturn nil\n}\n", // its own GN001 is not reported
		"p/c.gon": mcUseNil,
	})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	env := decodeProtocolEnvelope(t, out)
	if exit != 1 || len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" || filepath.Base(env.Diagnostics[0].File) != "c.gon" ||
		errOut != invalidLine("p/b.gon", "parsing //go:build line: missing close paren") {
		t.Fatalf("exit %d, %+v\nstderr %q", exit, env.Diagnostics, errOut)
	}
}

// The malformed file next to every other kind of file.
func TestMalformedConstraintBesideOtherFiles(t *testing.T) {
	bad := "//go:build (linux\n\npackage p\n"
	wantBad := invalidLine("p/a.gon", "parsing //go:build line: missing close paren")
	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"alone", map[string]string{"p/a.gon": bad}},
		{"with a valid plain .go", map[string]string{"p/a.gon": bad, "p/b.go": mcOK2}},
		{"with a file excluded by a valid constraint", map[string]string{"p/a.gon": bad, "p/b.gon": "//go:build ignore\n\npackage p\n"}},
		{"with a file excluded by its name suffix", map[string]string{"p/a.gon": bad, "p/b_plan9.gon": mcOK2}},
		{"with a name the go tool ignores", map[string]string{"p/a.gon": bad, "p/_x.gon": mcOK2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := pkgModule(t, tc.files)
			chdir(t, root)
			exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
			if exit != 1 || strings.TrimSpace(out) != emptyEnv || errOut != wantBad {
				t.Fatalf("exit %d (want 1), stdout %q\nstderr %q", exit, out, errOut)
			}
		})
	}
}

// A .go that a .gon shadows keeps its content, and the malformed file is not
// written, when the package fails.
func TestMalformedConstraintLeavesExistingGoFilesAlone(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"p/a.gon": "package p\n\nfunc F() {}\n",
		"p/a.go":  "// previous output\npackage p\n\nfunc F() {}\n",
		"p/b.gon": "//go:build (linux\n\npackage p\n",
	})
	chdir(t, root)
	for _, cmd := range []string{"transpile", "build"} {
		if exit, _, _ := captureRun([]string{cmd, "./p"}); exit != 1 {
			t.Fatalf("%s: exit %d", cmd, exit)
		}
	}
	if got := readFile(t, filepath.Join(root, "p", "a.go")); got != "// previous output\npackage p\n\nfunc F() {}\n" {
		t.Fatalf("a.go changed: %q", got)
	}
	if exists(filepath.Join(root, "p", "b.go")) {
		t.Fatal("b.go must not be written")
	}
}

// The plain .go form is unchanged (the loader's own error text).
func TestMalformedPlainGoConstraintUnchangedInModule(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"p/a.gon": "package p\n\nfunc F() {}\n",
		"p/c.go":  "//go:build (linux\n\npackage p\n\nfunc broken( {\n",
	})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	if exit != 1 || strings.TrimSpace(out) != emptyEnv || errOut != "gon: Go type error: -: c.go: parsing //go:build line: missing close paren\n" {
		t.Fatalf("exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// `...`: a package with a malformed file is that package's error; the others
// are checked. The exit is 1 once, one envelope holds the other packages'
// diagnostics, no warning says nothing matched, and under transpile and build
// only the packages that pass are written.
func TestMalformedConstraintInPattern(t *testing.T) {
	files := map[string]string{
		"pat/x/x.gon": "//go:build (linux\n\npackage x\n",
		"pat/y/y.gon": "package y\n\nfunc f() !*int {\n\treturn nil\n}\n",
		"pat/z/z.gon": "package z\n\nfunc Z() {}\n",
		"pat/w/w.gon": "//go:build ignore\n\npackage w\n",
	}
	root := pkgModule(t, files)
	chdir(t, root)
	wantBad := invalidLine("pat/x/x.gon", "parsing //go:build line: missing close paren")
	for _, args := range [][]string{{"check", "--json", "./..."}, {"vet", "--json", "./pat/..."}} {
		exit, out, errOut := captureRun(args)
		env := decodeProtocolEnvelope(t, out)
		if exit != 1 || len(env.Diagnostics) != 1 || filepath.Base(env.Diagnostics[0].File) != "y.gon" || errOut != wantBad {
			t.Fatalf("%v: exit %d, %+v\nstderr %q", args, exit, env.Diagnostics, errOut)
		}
	}
	exit, out, errOut := captureRun([]string{"check", "./..."})
	if exit != 1 || out != "" || !strings.HasPrefix(errOut, wantBad) || !strings.Contains(errOut, "pat/y/y.gon:4:9: error GN001") {
		t.Fatalf("human: exit %d\n%s", exit, errOut)
	}
	for _, cmd := range []string{"transpile", "build"} {
		exit, _, errOut := captureRun([]string{cmd, "./..."})
		if exit != 1 || !strings.Contains(errOut, wantBad) || !strings.Contains(errOut, notTranspiled("pat/x")) || !strings.Contains(errOut, notTranspiled("pat/y")) {
			t.Fatalf("%s: exit %d\n%s", cmd, exit, errOut)
		}
		if !exists(filepath.Join(root, "pat", "z", "z.go")) || exists(filepath.Join(root, "pat", "x", "x.go")) ||
			exists(filepath.Join(root, "pat", "y", "y.go")) || exists(filepath.Join(root, "pat", "w", "w.go")) {
			t.Fatalf("%s: only pat/z may be written", cmd)
		}
		if err := os.Remove(filepath.Join(root, "pat", "z", "z.go")); err != nil {
			t.Fatal(err)
		}
	}
	// A pattern whose only package is malformed is that package's error: it
	// is not skipped and does not warn.
	root = pkgModule(t, map[string]string{"q/q.gon": "//go:build (linux\n\npackage q\n"})
	chdir(t, root)
	exit, out, errOut = captureRun([]string{"check", "--json", "./..."})
	if exit != 1 || strings.TrimSpace(out) != emptyEnv || errOut != invalidLine("q/q.gon", "parsing //go:build line: missing close paren") {
		t.Fatalf("only a malformed package: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// Names the go tool ignores (`_x.gon`, `.x.gon`) are not part of the package:
// no error, nothing written, as outside a module.
func TestNamesTheGoToolIgnoresInModule(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"p/a.gon":  "package p\n\nfunc F() {}\n",
		"p/_x.gon": mcOK2,
		"p/.y.gon": "package p\n\nfunc H() {}\n",
	})
	chdir(t, root)
	for _, args := range [][]string{{"check", "--json", "./p"}, {"vet", "--json", "./p"}} {
		if exit, out, errOut := captureRun(args); exit != 0 || strings.TrimSpace(out) != emptyEnv || errOut != "" {
			t.Fatalf("%v: exit %d, stdout %q\nstderr %q", args, exit, out, errOut)
		}
	}
	exit, out, errOut := captureRun([]string{"transpile", "./p"})
	wantErr := "gon: p/.y.gon not transpiled: excluded by build constraints\n" +
		"gon: p/_x.gon not transpiled: excluded by build constraints\n"
	if exit != 0 || out != "transpiled: p/a.go\n" || errOut != wantErr {
		t.Fatalf("transpile: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
	if exists(filepath.Join(root, "p", "_x.go")) || exists(filepath.Join(root, "p", ".y.go")) {
		t.Fatal("an ignored name must not be written")
	}
}

// With nothing else in the build the package has no source: exit 2 named, and
// skipped under `...` with the warning (the contract of package discovery).
func TestOnlyIgnoredNamesIsNoSourceInTheBuild(t *testing.T) {
	osSuffixed := "a_" + otherThan("windows") + ".gon" // excluded by its name, on every OS but Windows
	root := pkgModule(t, map[string]string{
		"p/_x.gon":        mcOK2,
		"p/" + osSuffixed: mcOK2,
	})
	chdir(t, root)
	want := "gon: build constraints exclude all Go files in ./p (.gon files: p/_x.gon, p/" + osSuffixed + ")\n"
	for _, cmd := range []string{"check", "vet", "transpile", "build"} {
		exit, out, errOut := captureRun([]string{cmd, "./p"})
		if exit != 2 || out != "" || errOut != want {
			t.Fatalf("%s: exit %d (want 2), stdout %q\nstderr %q", cmd, exit, out, errOut)
		}
	}
	exit, out, errOut := captureRun([]string{"check", "--json", "./..."})
	if exit != 0 || strings.TrimSpace(out) != emptyEnv || errOut != "gon: warning: \"./...\" matched no packages with .gon files in the build\n" {
		t.Fatalf("./...: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// A valid constraint that does not match the platform is neither a malformed
// constraint nor an ignored name: it is excluded as before (no error, not
// checked), by //go:build line and by file name suffix.
func TestValidConstraintThatDoesNotMatchIsExcludedInModule(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"p/a.gon":                              "package p\n\nfunc F() {}\n",
		"p/w.gon":                              "//go:build " + otherThan("windows") + "\n\npackage p\n\nfunc w() !*int {\n\treturn nil\n}\n",
		"p/b_" + otherThan("windows") + ".gon": "package p\n\nfunc b() !*int {\n\treturn nil\n}\n",
	})
	chdir(t, root)
	if exit, out, errOut := captureRun([]string{"check", "--json", "./p"}); exit != 0 || strings.TrimSpace(out) != emptyEnv || errOut != "" {
		t.Fatalf("exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// The unexplained case stays a load failure: nothing here can produce a .gon
// the loader drops while go/build accepts it, so the decision is pinned in
// internal/checker (TestClassifyDropped, "ok.go"); this pins that a genuine
// load failure next to a malformed file is still exit 2, with no invalid-file
// line.
func TestLoaderFailureStaysExit2WithAMalformedFile(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"go.mod":  "garbage\n",
		"p/a.gon": "//go:build (linux\n\npackage p\n",
	})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p"})
	if exit != 2 || out != "" || strings.Contains(errOut, "invalid build constraint") || !strings.Contains(errOut, "errors parsing go.mod") {
		t.Fatalf("exit %d (want 2)\nstdout %q\nstderr %q", exit, out, errOut)
	}
}
