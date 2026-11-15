package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Package mode for `gon check` (v1.10 / M8), docs/rfc-package-check.md.
//
//	P1 — a directory or `...` operand selects package mode; a file operand is
//	     unchanged
//	P5 — one --json document, every file's diagnostics, per-file attribution
//	P6 — a Go type error does not hide contract diagnostics, and exits 1
//	P8 — `...` skips testdata, vendor, and names starting with `_` or `.`

const (
	pkgA = "package main\n\ntype T struct{}\n\ntype Conn struct{ DB !*T }\n\nfunc Take(p !*T) {}\n\nfunc main() {}\n"
	pkgB = "package main\n\nfunc use() {\n\tTake(nil)\n\t_ = Conn{}\n}\n"
)

// pkgModule creates a module under a fresh temp dir; files maps a relative path
// to its content.
func pkgModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if _, ok := files["go.mod"]; !ok {
		files["go.mod"] = "module pk\n\ngo 1.21\n"
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

// chdir runs the test with dir as the working directory.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestPackageModeChecksFilesTogether(t *testing.T) {
	root := pkgModule(t, map[string]string{"a.gon": pkgA, "b.gon": pkgB})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1\n%s", exit, errOut)
	}
	if strings.Contains(errOut, "Go type error") {
		t.Fatalf("a package whose files use each other must type-check:\n%s", errOut)
	}
	for _, want := range []string{"b.gon:4:7: error GN001", "b.gon:5:6: error GN002"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in:\n%s", want, errOut)
		}
	}
}

// The same file given as a FILE keeps today's single-file semantics.
func TestFileOperandKeepsSingleFileSemantics(t *testing.T) {
	root := pkgModule(t, map[string]string{"a.gon": pkgA, "b.gon": pkgB})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "b.gon"})
	if exit != 1 || !strings.Contains(errOut, "Go type error") || !strings.Contains(errOut, "undefined") {
		t.Fatalf("a file operand must still be checked alone (and fail on the sibling's names): exit %d\n%s", exit, errOut)
	}
}

func TestPackageModeJSONIsOneDocumentWithAllFiles(t *testing.T) {
	root := pkgModule(t, map[string]string{"a.gon": pkgA, "b.gon": pkgB})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"check", "--json", "."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1\n%s", exit, errOut)
	}
	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
		Diagnostics   []struct {
			Code string `json:"code"`
			File string `json:"file"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	if doc.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d", doc.SchemaVersion)
	}
	files := map[string]int{}
	for _, d := range doc.Diagnostics {
		files[filepath.Base(d.File)]++
		if !filepath.IsAbs(d.File) {
			t.Fatalf("diagnostic file must be absolute, got %q", d.File)
		}
	}
	if files["b.gon"] != 2 || files["a.gon"] != 0 {
		t.Fatalf("per-file attribution wrong: %v", files)
	}
}

func TestPackageModeTypeErrorDoesNotHideContractDiagnostics(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": pkgA,
		"b.gon": pkgB,
		"c.gon": "package main\n\nfunc broken() { undefinedThing() }\n",
	})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(errOut, "GN001") || !strings.Contains(errOut, "Go type error") {
		t.Fatalf("both the contract diagnostics and the type error must be reported:\n%s", errOut)
	}
}

func TestPackageModeCleanPackageIsOK(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": "package main\n\nfunc main() { helper() }\n",
		"b.gon": "package main\n\nfunc helper() {}\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"check", "."})
	if exit != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", exit, out, errOut)
	}
}

// A folder of independent programs is NOT a package unless named as one, and
// when named, its redeclaration is reported (which is correct).
func TestFolderOfIndependentProgramsStaysFileMode(t *testing.T) {
	prog := "package main\n\nfunc main() {}\n"
	root := pkgModule(t, map[string]string{"one.gon": prog, "two.gon": prog})
	chdir(t, root)

	if exit, _, errOut := captureRun([]string{"check", "one.gon", "two.gon"}); exit != 0 {
		t.Fatalf("file operands must stay independent: exit %d\n%s", exit, errOut)
	}
	exit, _, errOut := captureRun([]string{"check", "."})
	if exit != 1 || !strings.Contains(errOut, "redeclared") {
		t.Fatalf("named as a directory it is one package, so main is redeclared: exit %d\n%s", exit, errOut)
	}
}

func TestPackageModeRejectsDirectoryWithoutGonFiles(t *testing.T) {
	root := pkgModule(t, map[string]string{"only.go": "package main\n"})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "."})
	if exit != 2 || !strings.Contains(errOut, "no .gon files") {
		t.Fatalf("exit %d, want 2 with an explanation\n%s", exit, errOut)
	}
}

func TestEllipsisSkipsGoToolConventionDirectories(t *testing.T) {
	bad := "package main\n\nfunc f(p !*int) {}\n\nfunc g() { f(nil) }\n"
	ok := "package main\n\nfunc main() {}\n"
	root := pkgModule(t, map[string]string{
		"app/main.gon":         ok,
		"app/sub/x.gon":        ok,
		"testdata/x/bad.gon":   bad,
		"vendor/v/bad.gon":     bad,
		"_hidden/bad.gon":      bad,
		".dot/bad.gon":         bad,
		"app/testdata/bad.gon": bad,
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"check", "./..."})
	if exit != 0 {
		t.Fatalf("testdata, vendor, _x and .x must be skipped: exit %d\n%s", exit, errOut)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected ok, got %q", out)
	}
}

func TestEllipsisFindsNestedPackagesEachOnTheirOwn(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a/a.gon": "package a\n\nfunc F(p !*int) {}\n\nfunc G() { F(nil) }\n",
		"b/b.gon": "package b\n\nfunc H(p !*int) {}\n\nfunc I() { H(nil) }\n",
	})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "./..."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1\n%s", exit, errOut)
	}
	// Same base-name situation as two main.gon files: the path tells them apart.
	if !strings.Contains(errOut, "a/a.gon:") || !strings.Contains(errOut, "b/b.gon:") {
		t.Fatalf("each package's diagnostics must name its file by path:\n%s", errOut)
	}
}

func TestEllipsisWithNothingToCheckIsAnError(t *testing.T) {
	root := pkgModule(t, map[string]string{"x.go": "package main\n"})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "./..."})
	if exit != 2 || !strings.Contains(errOut, "no .gon files") {
		t.Fatalf("exit %d, want 2\n%s", exit, errOut)
	}
}

func TestMixedFileAndDirectoryOperands(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"pkg/a.gon": pkgA,
		"pkg/b.gon": pkgB,
		"solo.gon":  "package main\n\nfunc f(p !*int) {}\n\nfunc g() { f(nil) }\n",
	})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "solo.gon", "pkg"})
	if exit != 1 {
		t.Fatalf("exit = %d\n%s", exit, errOut)
	}
	if !strings.Contains(errOut, "solo.gon:") || !strings.Contains(errOut, "pkg/b.gon:") {
		t.Fatalf("both operands must be reported:\n%s", errOut)
	}
}

// --- Go type errors and the exit code in --json mode -------------------------
//
// Protocol v1 has no field for a Go type error, so --json never carries one. The
// exit code still has to reflect that the package does not type-check, or CI
// would pass a package that does not compile. (Package mode is new surface, so
// this adds no compatibility burden; FILE mode is unchanged, see the last test.)

func TestPackageJSONPureTypeErrorExitsOneWithEmptyDiagnostics(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": "package main\n\nfunc main() { undefinedThing() }\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"check", "--json", "."})
	if exit != 1 {
		t.Fatalf("a package that does not type-check must exit 1, got %d\nstdout: %s\nstderr: %s", exit, out, errOut)
	}
	var doc struct {
		SchemaVersion int               `json:"schemaVersion"`
		Diagnostics   []json.RawMessage `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout must still be one valid Protocol v1 document: %v\n%s", err, out)
	}
	if doc.SchemaVersion != 1 || len(doc.Diagnostics) != 0 {
		t.Fatalf("the envelope carries only Protocol v1 diagnostics: %+v", doc)
	}
	if !strings.Contains(errOut, "Go type error") {
		t.Fatalf("exit 1 with an empty document needs its reason on stderr:\n%s", errOut)
	}
}

func TestPackageJSONTypeErrorAlongsideContractDiagnostics(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": pkgA,
		"b.gon": pkgB,
		"c.gon": "package main\n\nfunc broken() { undefinedThing() }\n",
	})
	chdir(t, root)

	exit, out, _ := captureRun([]string{"check", "--json", "."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(out, `"GN001"`) || !strings.Contains(out, `"GN002"`) {
		t.Fatalf("contract diagnostics must still be in the document:\n%s", out)
	}
}

func TestPackageJSONCleanPackageStillExitsZero(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": "package main\n\nfunc main() { helper() }\n",
		"b.gon": "package main\n\nfunc helper() {}\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"check", "--json", "."})
	if exit != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", exit, errOut, out)
	}
}

// FILE mode and package mode agree on a pure Go type error under --json
// (Protocol v1 §7 as clarified): exit 1, a valid envelope with empty
// diagnostics, the reason on stderr. File mode got there in the separate fix
// for #22; this pins that the two modes cannot drift apart again.
func TestFileAndPackageJSONAgreeOnGoTypeError(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon": "package main\n\nfunc main() { undefinedThing() }\n",
	})
	chdir(t, root)

	fileExit, fileOut, fileErr := captureRun([]string{"check", "--json", "a.gon"})
	pkgExit, pkgOut, pkgErr := captureRun([]string{"check", "--json", "."})
	if fileExit != 1 || pkgExit != 1 {
		t.Fatalf("both modes must exit 1: file %d, package %d", fileExit, pkgExit)
	}
	if fileOut != pkgOut || !strings.Contains(fileOut, `"diagnostics":[]`) {
		t.Fatalf("envelopes differ:\nfile: %s\npackage: %s", fileOut, pkgOut)
	}
	if !strings.Contains(fileErr, "Go type error") || !strings.Contains(pkgErr, "Go type error") {
		t.Fatalf("the reason must be on stderr in both modes:\nfile: %s\npackage: %s", fileErr, pkgErr)
	}
}

// --- *_test.gon files are not part of package mode ---------------------------

const testGonWithContractError = "package main\n\nfunc helper() { f(nil) }\n\nfunc f(p !*int) {}\n"

// A *_test.gon next to production files is skipped, not an error: the package is
// loaded without tests, so it could never join it (it used to fail with "is not
// part of the package (excluded by a build constraint?)").
func TestPackageModeSkipsTestGonFiles(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon":           "package main\n\nfunc main() {}\n",
		"helper_test.gon": testGonWithContractError,
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"check", "."})
	if exit != 0 || strings.Contains(errOut, "not part of the package") || strings.Contains(errOut, "GN001") {
		t.Fatalf("the test file must be ignored: exit %d\nstdout: %s\nstderr: %s", exit, out, errOut)
	}
	exit, out, errOut = captureRun([]string{"check", "--json", "."})
	if exit != 0 || strings.Contains(out, "GN001") {
		t.Fatalf("--json: exit %d\nstdout: %s\nstderr: %s", exit, out, errOut)
	}
}

// A directory holding only *_test.gon files has nothing to check, like a
// directory with no .gon file at all: exit 2.
func TestPackageModeOnlyTestGonFilesIsNothingToCheck(t *testing.T) {
	root := pkgModule(t, map[string]string{"helper_test.gon": testGonWithContractError})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "."})
	if exit != 2 || !strings.Contains(errOut, "no .gon files") || !strings.Contains(errOut, "_test.gon") {
		t.Fatalf("exit %d, stderr:\n%s", exit, errOut)
	}
}

// `./...` does not count a directory that holds only *_test.gon files.
func TestPackagePatternIgnoresDirectoriesWithOnlyTestGonFiles(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a.gon":              "package main\n\nfunc main() {}\n",
		"only/only_test.gon": "package only\n\n" + "func broken() { undefinedThing() }\n",
	})
	chdir(t, root)

	exit, out, errOut := captureRun([]string{"check", "./..."})
	if exit != 0 || strings.Contains(errOut, "only_test.gon") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", exit, out, errOut)
	}
}

// Naming a *_test.gon as a FILE operand keeps the single-file behaviour.
func TestFileOperandStillChecksATestGonFile(t *testing.T) {
	root := pkgModule(t, map[string]string{"helper_test.gon": testGonWithContractError})
	chdir(t, root)

	exit, _, errOut := captureRun([]string{"check", "helper_test.gon"})
	if exit != 1 || !strings.Contains(errOut, "GN001") {
		t.Fatalf("file operand must be unchanged: exit %d\n%s", exit, errOut)
	}
}
