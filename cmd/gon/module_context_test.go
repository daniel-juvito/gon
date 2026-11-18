package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// File mode and the module a file is in (v1.17, package-discovery audit,
// proposal 2). A .gon inside a module is checked against that module, loaded
// once before the file is checked. When a go.mod is found but its module cannot
// be loaded, the run is exit 2 with no envelope and nothing is written:
//
//	K0 no go.mod above the file: the GOROOT/GOPATH importer, unchanged
//	K1 the load failed and golang.org/x/mod shows go.mod is invalid
//	K2 the load failed for another reason: the go command's error as it is
//	K3 the load succeeded: unchanged

const (
	mcPlain = "package p\n\nfunc F() {}\n"
	mcGN001 = "package p\n\nfunc f() !*int {\n\treturn nil\n}\n"
)

// mcModule writes a directory with the given go.mod (none when gomod is "")
// and a.gon, gn.gon and imp.gon (which imports the module-local pk/lib).
func mcModule(t *testing.T, gomod string, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"a.gon":      mcPlain,
		"gn.gon":     mcGN001,
		"imp.gon":    "package p\n\nimport \"pk/lib\"\n\nvar _ = lib.G\n",
		"lib/lib.go": "package lib\n\nfunc G() int { return 1 }\n",
	}
	if gomod != "" {
		files["go.mod"] = gomod
	}
	for k, v := range extra {
		files[k] = v
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
	return root
}

// K1: go.mod that does not parse, has no module directive, or has a module
// path that is not an import path. Every file-mode command is exit 2 with no
// stdout, and a GN001 in the file does not win over it.
func TestFileModeInvalidGoModIsExit2(t *testing.T) {
	for _, tc := range []struct{ name, gomod, want string }{
		{"F1 does not parse", "garbage\n", "go.mod:1: unknown directive: garbage"},
		{"F2 malformed module path", "module \"a b\"\n\ngo 1.22\n", `go.mod: malformed import path "a b": invalid char ' '`},
		{"F3 no module directive", "go 1.22\n", "go.mod: no module directive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mcModule(t, tc.gomod, nil)
			chdir(t, root)
			t.Setenv("GOTOOLCHAIN", "local")
			for _, args := range [][]string{
				{"check", "--json", "a.gon"}, {"check", "--json", "gn.gon"}, {"check", "--json", "imp.gon"},
				{"check", "gn.gon"}, {"vet", "--json", "gn.gon"},
			} {
				exit, out, errOut := captureRun(args)
				if exit != 2 || out != "" || !strings.HasPrefix(errOut, "gon: invalid go.mod: ") || !strings.Contains(errOut, tc.want) {
					t.Fatalf("%v: exit %d (want 2), stdout %q\nstderr %q", args, exit, out, errOut)
				}
				if strings.Contains(errOut, "GN001") || strings.Contains(errOut, "Go type error") {
					t.Fatalf("%v: nothing may be checked:\n%s", args, errOut)
				}
			}
		})
	}
}

// K2: the module cannot be loaded for a reason that is not an invalid go.mod.
// Exit 2 with the go command's own words, never "invalid go.mod".
func TestFileModeModuleNotLoadableIsExit2(t *testing.T) {
	valid := "module pk\n\ngo 1.22\n"
	for _, tc := range []struct {
		name  string
		gomod string
		extra map[string]string
		env   map[string]string
		want  string
	}{
		{"F4 newer go than the toolchain", "module pk\n\ngo 9.99\n", nil, map[string]string{"GOTOOLCHAIN": "local"}, "go.mod requires go >= 9.99"},
		{"F7 broken go.work", valid, map[string]string{"go.work": "garbage\n"}, map[string]string{"GOTOOLCHAIN": "local"}, "errors parsing go.work"},
		{"F8 inconsistent vendoring", "module pk\n\ngo 1.22\n\nrequire example.com/v v1.0.0\n", map[string]string{"vendor/modules.txt": "# example.com/w v1.0.0\n"}, map[string]string{"GOTOOLCHAIN": "local"}, "inconsistent vendoring"},
		{"F10 no go command", valid, nil, map[string]string{"PATH": "<empty>"}, "go command required"},
		{"F11 invalid GOFLAGS", valid, nil, map[string]string{"GOFLAGS": "-nonsense", "GOTOOLCHAIN": "local"}, "unknown flag -nonsense"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mcModule(t, tc.gomod, tc.extra)
			chdir(t, root)
			for k, v := range tc.env {
				if v == "<empty>" {
					v = t.TempDir()
				}
				t.Setenv(k, v)
			}
			for _, args := range [][]string{{"check", "--json", "a.gon"}, {"check", "--json", "gn.gon"}, {"check", "imp.gon"}} {
				exit, out, errOut := captureRun(args)
				prefix := "gon: cannot load the module of " + args[len(args)-1] + ": "
				if exit != 2 || out != "" || !strings.HasPrefix(errOut, prefix) || !strings.Contains(errOut, tc.want) {
					t.Fatalf("%v: exit %d (want 2), stdout %q\nstderr %q", args, exit, out, errOut)
				}
				if strings.Contains(errOut, "invalid go.mod") || strings.Contains(errOut, "GN001") {
					t.Fatalf("%v: no guessed cause, nothing checked:\n%s", args, errOut)
				}
			}
		})
	}
}

// K3: the load succeeds, also when a requirement or a replace target is not
// there (the go command loads lazily). Unchanged.
func TestFileModeLoadableModuleUnchanged(t *testing.T) {
	for _, tc := range []struct{ name, gomod string }{
		{"F0 valid", "module pk\n\ngo 1.22\n"},
		{"F5 missing requirement", "module pk\n\ngo 1.22\n\nrequire example.com/nope v1.0.0\n"},
		{"F6 replace to a missing directory", "module pk\n\ngo 1.22\n\nrequire example.com/x v0.0.0\n\nreplace example.com/x => ./nowhere\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mcModule(t, tc.gomod, nil)
			chdir(t, root)
			t.Setenv("GOTOOLCHAIN", "local")
			for _, c := range []struct {
				file string
				exit int
			}{{"a.gon", 0}, {"imp.gon", 0}, {"gn.gon", 1}} {
				exit, out, errOut := captureRun([]string{"check", "--json", c.file})
				if exit != c.exit || errOut != "" {
					t.Fatalf("%s: exit %d (want %d)\nstdout %s\nstderr %s", c.file, exit, c.exit, out, errOut)
				}
			}
		})
	}
}

// K0: no go.mod. The importer fallback is unchanged: a module-local import is
// a Go type error, and neither a missing go command nor bad GOFLAGS matters.
func TestFileModeWithoutGoModUnchanged(t *testing.T) {
	root := mcModule(t, "", nil)
	chdir(t, root)
	if exit, out, errOut := captureRun([]string{"check", "--json", "a.gon"}); exit != 0 || strings.TrimSpace(out) != `{"schemaVersion":1,"diagnostics":[]}` {
		t.Fatalf("a.gon: exit %d\n%s", exit, errOut)
	}
	if exit, _, errOut := captureRun([]string{"check", "--json", "imp.gon"}); exit != 1 || !strings.Contains(errOut, "gon: Go type error:") {
		t.Fatalf("imp.gon: exit %d\n%s", exit, errOut)
	}
	t.Setenv("GOFLAGS", "-nonsense")
	t.Setenv("PATH", t.TempDir())
	if exit, _, errOut := captureRun([]string{"check", "--json", "a.gon"}); exit != 0 {
		t.Fatalf("K0 with no go command and bad GOFLAGS: exit %d\n%s", exit, errOut)
	}
}

// K3 keeps the order of the output: the contract diagnostics in source order,
// then the Go type error, in human mode; the same diagnostics in the envelope.
func TestFileModeOutputOrderUnchanged(t *testing.T) {
	src := "package p\n\nfunc f() !*int {\n\treturn nil\n}\n\nfunc g(p !*int) {}\n\nfunc h() {\n\tg(nil)\n}\n\nfunc t() int {\n\treturn \"x\"\n}\n"
	root := mcModule(t, "module pk\n\ngo 1.22\n", map[string]string{"o.gon": src})
	chdir(t, root)
	exit, _, errOut := captureRun([]string{"check", "o.gon"})
	lines := strings.Split(strings.TrimSpace(errOut), "\n")
	if exit != 1 || len(lines) != 3 ||
		!strings.HasPrefix(lines[0], "o.gon:4:9: error GN001") ||
		!strings.HasPrefix(lines[1], "o.gon:10:4: error GN001") ||
		!strings.HasPrefix(lines[2], "gon: Go type error: ") {
		t.Fatalf("exit %d, stderr:\n%s", exit, errOut)
	}
	exit, out, errOut := captureRun([]string{"check", "--json", "o.gon"})
	env := decodeProtocolEnvelope(t, out)
	if exit != 1 || len(env.Diagnostics) != 2 || env.Diagnostics[0].Range.Start.Line != 3 || env.Diagnostics[1].Range.Start.Line != 9 ||
		!strings.HasPrefix(errOut, "gon: Go type error: ") {
		t.Fatalf("exit %d, %+v\n%s", exit, env.Diagnostics, errOut)
	}
}

// One load: the module a file is in is loaded once per file-mode run (the
// load that also type-checks the file), not once to decide the context and
// again to validate. Counted with a go wrapper that records the overlay file of
// each go command started with the file's validation overlay (one per
// packages.Load); no production code is involved.
func TestFileModeLoadsTheModuleOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper")
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "loads")
	wrapper := "#!/bin/sh\n" +
		"for a in \"$@\"; do case \"$a\" in -overlay=*) grep -q __gon_validate__ \"${a#-overlay=}\" && echo \"${a#-overlay=}\" >> " + log + ";; esac; done\n" +
		"exec " + realGo + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := mcModule(t, "module pk\n\ngo 1.22\n", nil)
	chdir(t, root)
	for _, args := range [][]string{{"check", "--json", "a.gon"}, {"check", "gn.gon"}, {"transpile", "a.gon"}} {
		_ = os.Remove(log)
		captureRun(args)
		data, _ := os.ReadFile(log)
		// One packages.Load can start several go commands; each Load writes its
		// own overlay file, so distinct overlay files count the loads.
		loads := make(map[string]bool)
		for _, f := range strings.Fields(string(data)) {
			loads[f] = true
		}
		if len(loads) != 1 {
			t.Fatalf("%v: the module was loaded %d times (%d go commands), want 1", args, len(loads), len(strings.Fields(string(data))))
		}
	}
}

// A dependency the go command cannot load does not fail the module load: the
// loader reports it as a ListError of the dependency with no load error, and
// as a TypeError of the file's package. It is the file's Go type error (exit
// 1, K3), never "cannot load the module" or "invalid go.mod". (A module that is
// not required or not in the cache behaves the same; it needs the network to
// fail, so it is not run here.)
func TestFileModeDependencyFailureIsAGoTypeError(t *testing.T) {
	for _, tc := range []struct{ name, gomod, imp string }{
		{"missing local package", "module pk\n\ngo 1.22\n", "pk/nope"},
		{"replace to a missing directory", "module pk\n\ngo 1.22\n\nrequire example.com/x v0.0.0\n\nreplace example.com/x => ./nowhere\n", "example.com/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mcModule(t, tc.gomod, map[string]string{
				"dep.gon": "package p\n\nimport \"" + tc.imp + "\"\n\nvar _ = " + filepath.Base(tc.imp) + ".G\n",
			})
			chdir(t, root)
			t.Setenv("GOTOOLCHAIN", "local")
			t.Setenv("GOFLAGS", "-mod=mod")
			exit, out, errOut := captureRun([]string{"check", "--json", "dep.gon"})
			if exit != 1 || strings.TrimSpace(out) != `{"schemaVersion":1,"diagnostics":[]}` ||
				!strings.HasPrefix(errOut, "gon: Go type error: ") || !strings.Contains(errOut, "could not import "+tc.imp) {
				t.Fatalf("exit %d (want 1)\nstdout %q\nstderr %q", exit, out, errOut)
			}
			if strings.Contains(errOut, "cannot load the module") || strings.Contains(errOut, "invalid go.mod") {
				t.Fatalf("a dependency failure is not a module failure:\n%s", errOut)
			}
		})
	}
}

// All-or-nothing with K1/K2 (Protocol v1 §7): a file in a module that cannot
// be loaded stops the run before emission, whichever operand it is; the
// diagnostics of the other operands are not sent.
func TestFileModeModuleFailureIsAllOrNothing(t *testing.T) {
	root := mcModule(t, "module pk\n\ngo 1.22\n", map[string]string{
		"broken/go.mod": "garbage\n",
		"broken/b.gon":  mcPlain,
		"q/q.gon":       "package q\n\nfunc f() !*int {\n\treturn nil\n}\n",
	})
	chdir(t, root)
	for _, args := range [][]string{
		{"check", "--json", "gn.gon", "broken/b.gon"},
		{"check", "--json", "broken/b.gon", "gn.gon"},
		{"check", "--json", "./q", "broken/b.gon"},
		{"vet", "--json", "gn.gon", "broken/b.gon"},
	} {
		exit, out, errOut := captureRun(args)
		if exit != 2 || out != "" || !strings.Contains(errOut, "gon: invalid go.mod: ") {
			t.Fatalf("%v: exit %d (want 2), stdout %q (want none)\n%s", args, exit, out, errOut)
		}
	}
}

// transpile and build, file form: with K1 or K2 nothing is written, and a .go
// already next to the file is left as it was, even when the file has a
// contract error.
func TestFileModeModuleFailureWritesNothing(t *testing.T) {
	for _, tc := range []struct{ name, gomod string }{
		{"K1 invalid go.mod", "garbage\n"},
		{"K2 broken go.work", "module pk\n\ngo 1.22\n"},
	} {
		for _, cmd := range []string{"transpile", "build"} {
			t.Run(tc.name+"/"+cmd, func(t *testing.T) {
				extra := map[string]string{"a.go": "// previous output\npackage p\n"}
				if strings.HasPrefix(tc.name, "K2") {
					extra["go.work"] = "garbage\n"
				}
				root := mcModule(t, tc.gomod, extra)
				chdir(t, root)
				t.Setenv("GOTOOLCHAIN", "local")
				for _, file := range []string{"a.gon", "gn.gon"} {
					exit, out, errOut := captureRun([]string{cmd, file})
					if exit != 2 || out != "" {
						t.Fatalf("%s %s: exit %d (want 2), stdout %q\n%s", cmd, file, exit, out, errOut)
					}
				}
				if got := readFile(t, filepath.Join(root, "a.go")); got != "// previous output\npackage p\n" {
					t.Fatalf("a.go changed: %q", got)
				}
				if exists(filepath.Join(root, "gn.go")) {
					t.Fatal("gn.go must not be written")
				}
				for _, bin := range []string{"a", "gn"} {
					if exists(filepath.Join(root, bin)) {
						t.Fatalf("binary %s must not be built", bin)
					}
				}
			})
		}
	}
}
