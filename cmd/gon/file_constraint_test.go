package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// A malformed //go:build line in a .gon given as a FILE operand (file mode),
// the counterpart of the package-mode rule of v1.19: a source error of that
// operand. The file is not checked (its contract errors are not reported), the
// other operands are, the envelope has no entry for it, stderr names the file
// and go/build's reason, and the run is exit 1. `transpile` and `build` write
// nothing and do not call go build. It is not a Diagnostic. `fmt` is not
// affected, and a module that cannot be loaded is still exit 2.

const (
	fcClean = "package p\n\nfunc G() {}\n"
	fcGN001 = "package p\n\nfunc f() !*int {\n\treturn nil\n}\n"
	fcParse = "package p\n\nfunc g( {\n"
)

var fileForms = []struct{ name, header, reason string }{
	{"M1 unbalanced parenthesis", "//go:build (linux\n", "parsing //go:build line: missing close paren"},
	{"M2 incomplete expression", "//go:build linux &&\n", "parsing //go:build line: unexpected end of expression"},
	{"M3 two //go:build lines", "//go:build ignore\n//go:build linux\n", "multiple //go:build comments"},
	{"M4 empty //go:build", "//go:build\n", "parsing //go:build line: unexpected end of expression"},
	{"M5 two terms", "//go:build linux windows\n", "parsing //go:build line: unexpected token windows"},
}

// fileScopes runs a test inside a module and outside any module.
func fileScopes(t *testing.T, files map[string]string, f func(t *testing.T, root string)) {
	t.Helper()
	t.Run("in a module", func(t *testing.T) {
		root := pkgModule(t, copyFiles(files))
		chdir(t, root)
		f(t, root)
	})
	t.Run("outside a module", func(t *testing.T) {
		f(t, outsideModule(t, copyFiles(files)))
	})
}

func copyFiles(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func dirNames(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return strings.Join(names, " ")
}

// Every command, every form, a clean body and a body with a contract error
// (which is not reported: the file is not checked), in and out of a module.
func TestFileModeMalformedConstraintIsASourceError(t *testing.T) {
	for _, form := range fileForms {
		for _, body := range []struct{ name, src string }{{"clean", fcClean}, {"GN001", fcGN001}} {
			t.Run(form.name+"/"+body.name, func(t *testing.T) {
				fileScopes(t, map[string]string{"p/a.gon": form.header + "\n" + body.src}, func(t *testing.T, root string) {
					want := invalidLine("p/a.gon", form.reason)
					for _, args := range [][]string{{"check", "--json", "p/a.gon"}, {"vet", "--json", "p/a.gon"}} {
						exit, out, errOut := captureRun(args)
						if exit != 1 || strings.TrimSpace(out) != emptyEnv || errOut != want {
							t.Fatalf("%v: exit %d (want 1), stdout %q\nstderr %q\nwant   %q", args, exit, out, errOut, want)
						}
					}
					if exit, out, errOut := captureRun([]string{"check", "p/a.gon"}); exit != 1 || out != "" || errOut != want {
						t.Fatalf("human: exit %d, stdout %q (no ok), stderr %q", exit, out, errOut)
					}
					for _, cmd := range []string{"transpile", "build"} {
						exit, out, errOut := captureRun([]string{cmd, "p/a.gon"})
						if exit != 1 || out != "" || errOut != want {
							t.Fatalf("%s: exit %d, stdout %q (no transpiled:/built:), stderr %q", cmd, exit, out, errOut)
						}
					}
					if got := dirNames(t, filepath.Join(root, "p")); got != "a.gon" {
						t.Fatalf("nothing may be written beside the .gon: %q", got)
					}
					if _, err := os.Stat(filepath.Join(root, "a")); err == nil {
						t.Fatal("build must not run go build: a binary exists")
					}
				})
			})
		}
	}
}

// The verdict is about the contents: a name the go tool ignores, or with a
// GOOS/GOARCH suffix that does not match, is checked like any other operand.
// (go/build asked about the real name would let a_windows and a_plan9 through.)
func TestFileModeMalformedConstraintWhateverTheOperandName(t *testing.T) {
	for _, name := range []string{"a", "_a", ".a", "a_windows", "a_plan9", "a_linux", "a_test"} {
		t.Run(name, func(t *testing.T) {
			fileScopes(t, map[string]string{"p/" + name + ".gon": "//go:build (linux\n\n" + fcGN001}, func(t *testing.T, root string) {
				want := invalidLine("p/"+name+".gon", "parsing //go:build line: missing close paren")
				exit, out, errOut := captureRun([]string{"check", "--json", "p/" + name + ".gon"})
				if exit != 1 || strings.TrimSpace(out) != emptyEnv || errOut != want {
					t.Fatalf("exit %d, stdout %q\nstderr %q\nwant   %q", exit, out, errOut, want)
				}
				if exit, _, errOut := captureRun([]string{"transpile", "p/" + name + ".gon"}); exit != 1 || errOut != want {
					t.Fatalf("transpile: exit %d, stderr %q", exit, errOut)
				}
				if got := dirNames(t, filepath.Join(root, "p")); got != name+".gon" {
					t.Fatalf("nothing may be written: %q", got)
				}
			})
		})
	}
}

// The same name with a VALID constraint (matching or not) is unchanged: the
// contract error is reported and a clean file passes, whatever the platform.
func TestFileModeValidConstraintsAreUnchanged(t *testing.T) {
	for _, header := range []string{"", "//go:build linux\n", "//go:build windows\n", "//go:build ignore\n", "// +build linux,,\n", "//go:build linux\n// +build windows\n"} {
		name := strings.TrimSpace(strings.ReplaceAll(header, "\n", " | "))
		t.Run("["+name+"]", func(t *testing.T) {
			fileScopes(t, map[string]string{"p/a.gon": header + "\n" + fcGN001, "p/b.gon": header + "\n" + fcClean}, func(t *testing.T, root string) {
				exit, out, errOut := captureRun([]string{"check", "--json", "p/a.gon"})
				env := decodeProtocolEnvelope(t, out)
				if exit != 1 || len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" || strings.Contains(errOut, "invalid build constraint") {
					t.Fatalf("exit %d, %+v\nstderr %q", exit, env.Diagnostics, errOut)
				}
				if exit, out, errOut := captureRun([]string{"check", "p/b.gon"}); exit != 0 || strings.TrimSpace(out) != "ok" || errOut != "" {
					t.Fatalf("clean file: exit %d, stdout %q, stderr %q", exit, out, errOut)
				}
				if exit, out, _ := captureRun([]string{"transpile", "p/b.gon"}); exit != 0 || !strings.HasPrefix(out, "transpiled: ") {
					t.Fatalf("transpile: exit %d, stdout %q", exit, out)
				}
			})
		})
	}
}

// A .go beside the .gon is never touched when the constraint is rejected:
// same bytes, same modification time.
func TestFileModeMalformedConstraintLeavesAnExistingGoFileAlone(t *testing.T) {
	const old = "package p\n\n// the previous output\nfunc Old() {}\n"
	fileScopes(t, map[string]string{"p/a.gon": "//go:build (linux\n\n" + fcClean, "p/a.go": old}, func(t *testing.T, root string) {
		path := filepath.Join(root, "p", "a.go")
		then := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		if err := os.Chtimes(path, then, then); err != nil {
			t.Fatal(err)
		}
		for _, cmd := range []string{"transpile", "build"} {
			if exit, _, _ := captureRun([]string{cmd, "p/a.gon"}); exit != 1 {
				t.Fatalf("%s: exit %d, want 1", cmd, exit)
			}
			got, err := os.ReadFile(path)
			fi, serr := os.Stat(path)
			if err != nil || serr != nil || string(got) != old || !fi.ModTime().Equal(then) {
				t.Fatalf("%s changed a.go: %q, mtime %v", cmd, got, fi.ModTime())
			}
		}
	})
}

// Several operands, one envelope for the whole call: a malformed operand does
// not stop the next ones, and its contract errors are not reported.
func TestFileModeMalformedConstraintWithOtherOperands(t *testing.T) {
	bad := "//go:build (linux\n\n" + fcGN001
	fileScopes(t, map[string]string{"bad.gon": bad, "bad2.gon": "//go:build linux &&\n\n" + fcGN001, "good.gon": fcGN001, "ok.gon": fcClean}, func(t *testing.T, root string) {
		line := invalidLine("bad.gon", "parsing //go:build line: missing close paren")
		line2 := invalidLine("bad2.gon", "parsing //go:build line: unexpected end of expression")

		for _, order := range [][]string{{"bad.gon", "good.gon"}, {"good.gon", "bad.gon"}} {
			exit, out, errOut := captureRun(append([]string{"check", "--json"}, order...))
			env := decodeProtocolEnvelope(t, out)
			if exit != 1 || len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" ||
				!strings.HasSuffix(env.Diagnostics[0].File, "good.gon") || errOut != line {
				t.Fatalf("%v: exit %d, %+v\nstderr %q", order, exit, env.Diagnostics, errOut)
			}
			if strings.Count(out, "schemaVersion") != 1 {
				t.Fatalf("%v: exactly one envelope:\n%s", order, out)
			}
		}

		// A malformed operand and a clean one: exit 1 only because of the former.
		exit, out, errOut := captureRun([]string{"check", "--json", "bad.gon", "ok.gon"})
		if exit != 1 || strings.TrimSpace(out) != emptyEnv || errOut != line {
			t.Fatalf("bad+ok: exit %d, stdout %q, stderr %q", exit, out, errOut)
		}

		// Two malformed operands: both named once, in order, one envelope.
		exit, out, errOut = captureRun([]string{"check", "--json", "bad.gon", "bad2.gon", "good.gon"})
		env := decodeProtocolEnvelope(t, out)
		if exit != 1 || len(env.Diagnostics) != 1 || errOut != line+line2 {
			t.Fatalf("bad+bad2+good: exit %d, %+v\nstderr %q", exit, env.Diagnostics, errOut)
		}

		// Human mode: the next operand is still checked, its error printed.
		exit, out, errOut = captureRun([]string{"check", "bad.gon", "good.gon"})
		if exit != 1 || out != "" || !strings.HasPrefix(errOut, line) || !strings.Contains(errOut, "good.gon:4:9: error GN001") {
			t.Fatalf("human: exit %d, stdout %q\nstderr %q", exit, out, errOut)
		}
	})
}

// A malformed constraint together with an operand that fails the pre-check
// parse (Protocol v1 §7: exit 1, no envelope): the order of the operands
// decides what is reported, and no partial envelope ever is.
func TestFileModeMalformedConstraintWithAPreCheckParseFailure(t *testing.T) {
	fileScopes(t, map[string]string{"bad.gon": "//go:build (linux\n\n" + fcGN001, "good.gon": fcGN001, "parse.gon": fcParse}, func(t *testing.T, root string) {
		line := invalidLine("bad.gon", "parsing //go:build line: missing close paren")
		for _, tc := range []struct {
			name      string
			operands  []string
			wantLine  bool
			wantParse bool
		}{
			{"constraint then parse error", []string{"bad.gon", "parse.gon"}, true, true},
			{"parse error then constraint", []string{"parse.gon", "bad.gon"}, false, true},
			{"contract error, constraint, parse error", []string{"good.gon", "bad.gon", "parse.gon"}, true, true},
		} {
			for _, jsonFlag := range []bool{true, false} {
				args := []string{"check"}
				if jsonFlag {
					args = append(args, "--json")
				}
				exit, out, errOut := captureRun(append(args, tc.operands...))
				if exit != 1 || out != "" {
					t.Fatalf("%s %v: exit %d (want 1), stdout %q (want none: no partial envelope)\n%s", tc.name, args, exit, out, errOut)
				}
				if strings.Contains(errOut, line) != tc.wantLine || strings.Contains(errOut, "gon: parse error:") != tc.wantParse {
					t.Fatalf("%s %v: stderr\n%s", tc.name, args, errOut)
				}
				if tc.wantLine && tc.wantParse && strings.Index(errOut, line) > strings.Index(errOut, "gon: parse error:") {
					t.Fatalf("%s: the constraint line comes first:\n%s", tc.name, errOut)
				}
				if !jsonFlag && strings.Contains(errOut, "error GN001") && tc.operands[0] == "parse.gon" {
					t.Fatalf("%s: nothing after the parse failure is checked:\n%s", tc.name, errOut)
				}
			}
		}
	})
}

// A module that cannot be loaded is exit 2 with no envelope, malformed
// constraint or not, and the constraint is not reported.
func TestFileModeModuleFailureStillWinsOverAMalformedConstraint(t *testing.T) {
	root := mcModule(t, "garbage\n", map[string]string{"bad.gon": "//go:build (linux\n\n" + fcClean})
	chdir(t, root)
	for _, args := range [][]string{{"check", "--json", "bad.gon"}, {"check", "bad.gon"}, {"transpile", "bad.gon"}, {"build", "bad.gon"}} {
		exit, out, errOut := captureRun(args)
		if exit != 2 || out != "" || !strings.Contains(errOut, "invalid go.mod") || strings.Contains(errOut, "invalid build constraint") {
			t.Fatalf("%v: exit %d (want 2), stdout %q\n%s", args, exit, out, errOut)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "bad.go")); err == nil {
		t.Fatal("nothing may be written")
	}
}

// `gon fmt` is not part of this check: it formats the file as in v1.19.0,
// including the rewrite go/format (and gofmt) make of `linux windows`.
func TestFileModeFmtIsUnchangedOnMalformedConstraints(t *testing.T) {
	for _, tc := range []struct{ name, header, want string }{
		{"M1", "//go:build (linux", "//go:build (linux"},
		{"M2", "//go:build linux &&", "//go:build linux &&"},
		{"M3", "//go:build ignore\n//go:build linux", "//go:build ignore\n//go:build linux"},
		{"M4", "//go:build", "//go:build"},
		{"M5", "//go:build linux windows", "//go:build linux"}, // go/format's own rewrite
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "a.gon")
			if err := os.WriteFile(path, []byte(tc.header+"\n\n"+fcClean), 0o644); err != nil {
				t.Fatal(err)
			}
			exit, out, errOut := captureRun([]string{"fmt", path})
			got, _ := os.ReadFile(path)
			if exit != 0 || out != "" || errOut != "" || !strings.HasPrefix(string(got), tc.want+"\n") {
				t.Fatalf("exit %d, stdout %q, stderr %q, file %q", exit, out, errOut, got)
			}
		})
	}
}

// The check reads the clean Go form (what is transpiled): preprocessing leaves
// the header as it is, so the verdict on it is the verdict on the source.
func TestPreprocLeavesTheConstraintHeaderAlone(t *testing.T) {
	for _, header := range []string{"//go:build (linux", "//go:build linux windows", "// Copyright\n\n//go:build (linux"} {
		src := []byte(header + "\n\npackage p\n\nfunc f() !*int {\n\tv := 1\n\treturn &v\n}\n")
		r := preproc.Process("a.gon", src)
		if !strings.HasPrefix(string(r.Clean), header+"\n\npackage p") {
			t.Fatalf("header of %q changed:\n%s", header, r.Clean)
		}
	}
}
