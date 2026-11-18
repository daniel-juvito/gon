// Package-aware checking (v1.10 / M8).
//
// Executable specification for docs/rfc-package-check.md.
//
//	P2/P3 — the files are type-checked together in their real directory; a .gon
//	        shadows the same-named .go; sibling .go files are loaded; tests are not
//	P4    — a contract written in one file applies in the others; `!` offsets are
//	        per file
//	P5    — diagnostics are attributed per file, in a deterministic order
//	P6    — a Go type error in one file does not hide diagnostics in another
package checker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// pkgDir writes a throwaway module containing the given files and returns it.
func pkgDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if _, ok := files["go.mod"]; !ok {
		files["go.mod"] = "module pk\n\ngo 1.21\n"
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// checkPkg preprocesses the named .gon files of dir and checks them together.
func checkPkg(t *testing.T, dir string, gon ...string) (*PackageResult, error) {
	t.Helper()
	var files []PackageFile
	for _, name := range gon {
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		r := preproc.Process(path, src)
		files = append(files, PackageFile{Path: path, Clean: r.Clean, NonNilOffsets: r.NonNilOffsets})
	}
	return CheckPackage(dir, files, nil)
}

// codes returns the sorted diagnostic codes of the file named base.
func pkgCodes(res *PackageResult, base string) []string {
	var out []string
	for _, f := range res.Files {
		if filepath.Base(f.Path) != base {
			continue
		}
		for _, d := range f.Diagnostics {
			out = append(out, d.Code)
		}
	}
	sort.Strings(out)
	return out
}

func mustPkg(t *testing.T, dir string, gon ...string) *PackageResult {
	t.Helper()
	res, err := checkPkg(t, dir, gon...)
	if err != nil {
		t.Fatalf("CheckPackage: %v", err)
	}
	return res
}

func wantCodes(t *testing.T, res *PackageResult, base string, want ...string) {
	t.Helper()
	got := pkgCodes(res, base)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s: diagnostics = %v, want %v\nall: %+v\ntype errors: %v", base, got, want, res.Files, res.TypeErrors)
	}
}

const sibA = `package main

type T struct{}

type Conn struct{ DB !*T }

func Take(p !*T) {}

func Make() !*T { return &T{} }

func main() {}
`

// --- P4: a contract written in one file applies in another -------------------

func TestPackageContractsAreSharedAcrossFiles(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": sibA,
		"b.gon": `package main

func use() {
	Take(nil)
	_ = Conn{}
	m := Make()
	if m == nil {
	}
}
`,
	})
	res := mustPkg(t, dir, "a.gon", "b.gon")
	if len(res.TypeErrors) != 0 {
		t.Fatalf("a package whose files use each other must type-check: %v", res.TypeErrors)
	}
	wantCodes(t, res, "a.gon") // nothing wrong in a
	wantCodes(t, res, "b.gon", "GN001", "GN002", "GW001")
}

// A struct declared in a sibling file is walked for its nested `!` fields.
func TestPackageEmbeddedStructFromSiblingFile(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": "package main\n\ntype Base struct{ Root !*int }\n\nfunc main() {}\n",
		"b.gon": "package main\n\ntype W struct{ Base }\n\nfunc f() { _ = W{} }\n",
	})
	res := mustPkg(t, dir, "a.gon", "b.gon")
	wantCodes(t, res, "b.gon", "GN002")
}

// `!` offsets are byte offsets into ONE file's clean source. Two files whose
// field types start at the same offset, one with a `!` and one without, keep
// their own meaning: each file's declarations are collected against its own
// table. (Reading a SIBLING file's offsets is covered by the nested-struct test
// below.)
func TestPackageNonNilOffsetsArePerFile(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon":    "package main\n\ntype A struct{ P !*int }\n\nfunc main() {}\n",
		"b.gon":    "package main\n\ntype B struct{ P *int }\n",
		"main.gon": "package main\n\nfunc f() { _ = A{}; _ = B{} }\n",
	})
	res := mustPkg(t, dir, "a.gon", "b.gon", "main.gon")
	// Exactly A's field is required; B's plain field is not a contract.
	wantCodes(t, res, "main.gon", "GN002")
}

// The one place a checker reads `!` offsets of ANOTHER file: walking a nested
// anonymous struct declared in a sibling file. Reading this file's own table
// there would miss the contract (or invent one), so the lookup is by file.
func TestPackageNestedStructFromSiblingFileUsesThatFilesOffsets(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": "package main\n\ntype Outer struct{ In struct{ P !*int } }\n\nfunc main() {}\n",
		"b.gon": "package main\n\nfunc f() { _ = Outer{} }\n",
	})
	res := mustPkg(t, dir, "a.gon", "b.gon")
	wantCodes(t, res, "b.gon", "GN002")
}

// --- P2/P3: how the package is assembled ------------------------------------

// A .gon replaces the same-named .go that `gon transpile` emitted next to it.
func TestPackageGonShadowsSameNamedGo(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.go":  "package main\n\ntype T struct{}\n\nfunc Take(p *T) {}\n\nfunc main() {}\n",
		"a.gon": "package main\n\ntype T struct{}\n\nfunc Take(p !*T) {}\n\nfunc main() {}\n",
		"b.gon": "package main\n\nfunc use() { Take(nil) }\n",
	})
	res := mustPkg(t, dir, "a.gon", "b.gon")
	if len(res.TypeErrors) != 0 {
		t.Fatalf("a.gon must replace a.go, not redeclare it: %v", res.TypeErrors)
	}
	wantCodes(t, res, "b.gon", "GN001")
}

// Plain .go siblings of the package are loaded as they are.
func TestPackageLoadsSiblingGoFiles(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"helper.go": "package main\n\nfunc helper() int { return 1 }\n",
		"a.gon":     "package main\n\nfunc main() { _ = helper() }\n",
	})
	res := mustPkg(t, dir, "a.gon")
	if len(res.TypeErrors) != 0 {
		t.Fatalf("a sibling .go declaration must be visible: %v", res.TypeErrors)
	}
}

// *_test.go files are not part of the check, even when they would not compile.
func TestPackageExcludesTestFiles(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon":      "package main\n\nfunc main() {}\n",
		"a_test.go":  "package main\n\nfunc broken( {\n",
		"b_test.gon": "",
	})
	_ = os.Remove(filepath.Join(dir, "b_test.gon"))
	res := mustPkg(t, dir, "a.gon")
	if len(res.TypeErrors) != 0 {
		t.Fatalf("test files must be excluded: %v", res.TypeErrors)
	}
}

// A file that a build constraint excludes is not part of the package: it is
// not checked (v1.16; it was a load failure in v1.15).
func TestPackageBuildConstraintExcludesFile(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon":            "package main\n\nfunc main() {}\n",
		"b_plan9_s390.gon": "package main\n\nfunc other() !*int { return nil }\n",
	})
	res, err := checkPkg(t, dir, "a.gon", "b_plan9_s390.gon")
	if err != nil {
		t.Fatalf("an excluded file must not fail the load: %v", err)
	}
	if len(res.Files) != 1 || filepath.Base(res.Files[0].Path) != "a.gon" {
		t.Fatalf("only a.gon is checked: %+v", res.Files)
	}
}

// --- P5: attribution and order ---------------------------------------------

func TestPackageDiagnosticsAreAttributedAndOrdered(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"z.gon": "package main\n\nfunc z() { Take(nil) }\n",
		"a.gon": sibA,
		"m.gon": "package main\n\nfunc m() { Take(nil) }\n",
	})
	// Input order deliberately not sorted.
	res := mustPkg(t, dir, "z.gon", "a.gon", "m.gon")
	var order []string
	for _, f := range res.Files {
		order = append(order, filepath.Base(f.Path))
		for _, d := range f.Diagnostics {
			if d.File != filepath.Base(f.Path) {
				t.Fatalf("diagnostic %v attributed to %q, belongs to %q", d, d.File, filepath.Base(f.Path))
			}
		}
	}
	if strings.Join(order, ",") != "a.gon,m.gon,z.gon" {
		t.Fatalf("files must be reported sorted by name, got %v", order)
	}
	wantCodes(t, res, "m.gon", "GN001")
	wantCodes(t, res, "z.gon", "GN001")
}

func TestPackageOrderIsDeterministic(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": sibA,
		"b.gon": "package main\n\nfunc u() { Take(nil); _ = Conn{}; Take(nil) }\n",
		"c.gon": "package main\n\nfunc v() { _ = Conn{} }\n",
	})
	render := func() string {
		res := mustPkg(t, dir, "a.gon", "b.gon", "c.gon")
		var sb strings.Builder
		for _, f := range res.Files {
			for _, d := range f.Diagnostics {
				sb.WriteString(d.String() + "\n")
			}
		}
		return sb.String()
	}
	first := render()
	for i := 0; i < 15; i++ {
		if got := render(); got != first {
			t.Fatalf("run %d differs:\n%s\n--- vs ---\n%s", i, first, got)
		}
	}
}

// --- P6: a type error does not hide diagnostics ------------------------------

func TestPackageTypeErrorDoesNotHideContractDiagnostics(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": sibA,
		"b.gon": "package main\n\nfunc use() { Take(nil) }\n",
		"c.gon": "package main\n\nfunc broken() { undefinedThing() }\n",
	})
	res := mustPkg(t, dir, "a.gon", "b.gon", "c.gon")
	if len(res.TypeErrors) == 0 {
		t.Fatal("the undefined reference in c.gon must be reported as a type error")
	}
	wantCodes(t, res, "b.gon", "GN001")
}

// --- the package must be checkable at all ------------------------------------

func TestPackageNeedsFiles(t *testing.T) {
	if _, err := CheckPackage(t.TempDir(), nil, nil); err == nil {
		t.Fatal("an empty file list must be an error")
	}
}
