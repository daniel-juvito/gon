package main

import (
	"github.com/daniel-juvito/gon/internal/checker"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gon convert (RFC Part B with gon convert, accepted 2026-11-27): a preview by default,
// --write applies it, and a file is written only if the rewrite changes none of the
// diagnostics and needs no second pass.

const convSrc = "package main\n\n" +
	"type T struct{ next *T }\n\n" +
	"func use(p *T) {}\n\n" +
	"func f() {\n" +
	"\tbuf := make([]byte, 8)\n" +
	"\tbuf = nil\n" +
	"\t_ = buf\n" +
	"\tok := &T{}\n" +
	"\tuse(ok)\n" +
	"}\n\n" +
	"func main() { f() }\n"

const convWant = "package main\n\n" +
	"type T struct{ next *T }\n\n" +
	"func use(p *T) {}\n\n" +
	"func f() {\n" +
	"\tbuf := ([]byte)(make([]byte, 8))\n" +
	"\tbuf = nil\n" +
	"\t_ = buf\n" +
	"\tok := &T{}\n" +
	"\tuse(ok)\n" +
	"}\n\n" +
	"func main() { f() }\n"

func convRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConvertPreviewWritesNothing(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{"main.gon": convSrc})
	chdir(t, root)
	exit, stdout, errOut := captureRun([]string{"convert", "main.gon"})
	if exit != 0 {
		t.Fatalf("exit = %d\n%s", exit, errOut)
	}
	if got := convRead(t, "main.gon"); got != convSrc {
		t.Fatalf("a preview changed the file:\n%s", got)
	}
	for _, want := range []string{"--- main.gon", "+++ main.gon", "@@ line 8 @@", "-\tbuf := make([]byte, 8)", "+\tbuf := ([]byte)(make([]byte, 8))"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the diff lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "ok :=") {
		t.Errorf("the diff mentions a line that does not change:\n%s", stdout)
	}
}

func TestConvertWriteAppliesAndIsStable(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{"main.gon": convSrc})
	chdir(t, root)
	exit, _, errOut := captureRun([]string{"convert", "--write", "main.gon"})
	if exit != 0 {
		t.Fatalf("exit = %d\n%s", exit, errOut)
	}
	if got := convRead(t, "main.gon"); got != convWant {
		t.Fatalf("written file:\n%s\nwant:\n%s", got, convWant)
	}
	if !strings.Contains(errOut, "1 variable(s) kept nilable") {
		t.Errorf("stderr = %q", errOut)
	}
	// A second run has nothing to say and nothing to do.
	exit, stdout, errOut := captureRun([]string{"convert", "--write", "main.gon"})
	if exit != 0 || stdout != "" || errOut != "" {
		t.Fatalf("second run: exit %d, stdout %q, stderr %q", exit, stdout, errOut)
	}
	// And the file checks as it did before.
	if exit, _, errOut := captureRun([]string{"check", "main.gon"}); exit != 0 {
		t.Fatalf("check after convert: exit %d\n%s", exit, errOut)
	}
}

func TestConvertPackageOperand(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"types.gon": "package main\n\ntype T struct{ next *T }\n\nfunc use(p *T) {}\n",
		"a.gon":     "package main\n\nfunc f() {\n\tbuf := make([]byte, 8)\n\tbuf = nil\n\t_ = buf\n}\n",
		"b.gon":     "package main\n\nfunc g() {\n\tok := &T{}\n\tuse(ok)\n\tfor p := new(T); p != nil; p = p.next {\n\t\tuse(p)\n\t}\n}\n\nfunc main() { f(); g() }\n",
	})
	chdir(t, root)
	exit, stdout, errOut := captureRun([]string{"convert", "./..."})
	if exit != 0 {
		t.Fatalf("exit = %d\n%s", exit, errOut)
	}
	for _, want := range []string{"--- a.gon", "([]byte)(make([]byte, 8))", "--- b.gon", "(*T)(new(T))"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the preview lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "--- types.gon") {
		t.Errorf("a file with nothing to rewrite is in the preview:\n%s", stdout)
	}
	if exit, _, errOut := captureRun([]string{"convert", "--write", "./..."}); exit != 0 {
		t.Fatalf("write: exit %d\n%s", exit, errOut)
	}
	if !strings.Contains(convRead(t, filepath.Join(root, "a.gon")), "([]byte)(make([]byte, 8))") ||
		!strings.Contains(convRead(t, filepath.Join(root, "b.gon")), "for p := (*T)(new(T)); p != nil") {
		t.Fatal("the files were not rewritten")
	}
	if exit, stdout, _ := captureRun([]string{"convert", "./..."}); exit != 0 || stdout != "" {
		t.Fatalf("a second run: exit %d, stdout %q", exit, stdout)
	}
	if exit, _, errOut := captureRun([]string{"check", "./..."}); exit != 0 {
		t.Fatalf("check after convert: exit %d\n%s", exit, errOut)
	}
}

func TestConvertRefusesWhatItCannotCheck(t *testing.T) {
	// A Go type error: nothing is converted, nothing is written.
	bad := convSrc + "\nvar _ int = \"x\"\n"
	root := scModule(t, "1.21", map[string]string{"main.gon": bad})
	chdir(t, root)
	exit, _, errOut := captureRun([]string{"convert", "--write", "main.gon"})
	if exit != 1 || !strings.Contains(errOut, "Go type error") {
		t.Fatalf("exit = %d, stderr %q", exit, errOut)
	}
	if got := convRead(t, "main.gon"); got != bad {
		t.Fatalf("a file with a type error was written:\n%s", got)
	}
}

func TestConvertUsage(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{"main.gon": convSrc})
	chdir(t, root)
	for name, args := range map[string][]string{
		"no operand":   {"convert"},
		"unknown flag": {"convert", "--check", "main.gon"},
		"not a .gon":   {"convert", "main.go"},
	} {
		if exit, _, _ := captureRun(args); exit != 2 {
			t.Errorf("%s: exit = %d, want 2", name, exit)
		}
	}
	if got := convRead(t, "main.gon"); got != convSrc {
		t.Fatal("a usage error changed the file")
	}
}

func TestConvertLeavesAFileWithNothingToDoAlone(t *testing.T) {
	src := "package main\n\nfunc main() { x := []int{1}; x = append(x, 2); _ = x }\n"
	root := scModule(t, "1.21", map[string]string{"main.gon": src})
	chdir(t, root)
	exit, stdout, errOut := captureRun([]string{"convert", "--write", "main.gon"})
	if exit != 0 || stdout != "" || errOut != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", exit, stdout, errOut)
	}
	if got := convRead(t, "main.gon"); got != src {
		t.Fatalf("the file changed:\n%s", got)
	}
}

func TestConvertWriteIsAllOrNone(t *testing.T) {
	files := map[string]string{
		"types.gon": "package main\n\ntype T struct{ next *T }\n\nfunc use(p *T) {}\n",
		"a.gon":     "package main\n\nfunc f() {\n\tbuf := make([]byte, 8)\n\tbuf = nil\n\t_ = buf\n}\n",
		"b.gon":     "package main\n\nfunc g() {\n\tbuf := make([]byte, 4)\n\tbuf = nil\n\t_ = buf\n}\n\nfunc main() { f(); g() }\n",
	}
	root := scModule(t, "1.21", files)
	chdir(t, root)
	calls := 0
	orig := renameFile
	renameFile = func(from, to string) error {
		calls++
		if calls == 2 {
			return os.ErrPermission
		}
		return orig(from, to)
	}
	defer func() { renameFile = orig }()
	exit, _, errOut := captureRun([]string{"convert", "--write", "./..."})
	if exit != 2 || !strings.Contains(errOut, "no file was changed") {
		t.Fatalf("exit = %d, stderr %q", exit, errOut)
	}
	for name, want := range files {
		if got := convRead(t, filepath.Join(root, name)); got != want {
			t.Errorf("%s changed after a failed write:\n%s", name, got)
		}
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".gon-convert") {
			t.Errorf("a staging file was left: %s", e.Name())
		}
	}
	// With the failure gone, the same run succeeds.
	renameFile = orig
	if exit, _, errOut := captureRun([]string{"convert", "--write", "./..."}); exit != 0 {
		t.Fatalf("retry: exit %d\n%s", exit, errOut)
	}
	if !strings.Contains(convRead(t, filepath.Join(root, "a.gon")), "([]byte)(") || !strings.Contains(convRead(t, filepath.Join(root, "b.gon")), "([]byte)(") {
		t.Fatal("the retry did not rewrite both files")
	}
}

func TestDiagnosticSetTellsApartTwoOnOneLine(t *testing.T) {
	d1 := &checker.Diagnostic{Code: "GN001", Line: 20, Message: "cannot assign nil to non-nil variable !p"}
	d2 := &checker.Diagnostic{Code: "GN001", Line: 20, Message: "cannot assign nil to non-nil variable !q"}
	if diagnosticSet([]*checker.Diagnostic{d1}) == diagnosticSet([]*checker.Diagnostic{d2}) {
		t.Fatal("two diagnostics with other messages compare equal")
	}
	if diagnosticSet([]*checker.Diagnostic{d1, d2}) != diagnosticSet([]*checker.Diagnostic{d2, d1}) {
		t.Fatal("the order of the diagnostics matters")
	}
	moved := *d1
	moved.Col = 9
	if diagnosticSet([]*checker.Diagnostic{d1}) != diagnosticSet([]*checker.Diagnostic{&moved}) {
		t.Fatal("a moved column makes the sets differ")
	}
}
