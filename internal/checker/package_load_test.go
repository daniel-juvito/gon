// Package loading (v1.15, Protocol v1 §7) and package membership (v1.16).
//
// CheckPackage returns an error only when the package cannot be loaded at all;
// a file the Go loader could not parse is a TypeError of the result instead.
// Inside a module the go command decides which .gon files belong to the
// package: an excluded one is not checked, whatever the file order, and a
// package none of whose sources is in the build is an *ExcludedError.
package checker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	loadOK      = "package main\n\nfunc main() {}\n"
	loadIgnored = "//go:build ignore\n\npackage main\n"
	loadNoPkg   = "pakage main\n"
	loadBroken  = "package main\n\nfunc broken( {\n"
	loadGN001   = "package main\n\nfunc f() !*int {\n\treturn nil\n}\n"

	loadIgnoredGN001 = "//go:build ignore\n\npackage main\n\nfunc f() !*int {\n\treturn nil\n}\n"
)

// checkedPaths are the base names of the files CheckPackage checked.
func checkedPaths(res *PackageResult) []string {
	var out []string
	for _, f := range res.Files {
		out = append(out, filepath.Base(f.Path))
	}
	return out
}

// D1g / D1h: an excluded file is not checked and does not fail the load,
// whichever of the two files sorts first.
func TestPackageExcludedFileIsSkippedWhateverTheOrder(t *testing.T) {
	for _, tc := range []struct{ name, a, b, checked string }{
		{"excluded file first", loadIgnoredGN001, loadGN001, "b.gon"},
		{"excluded file last", loadGN001, loadIgnoredGN001, "a.gon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := pkgDir(t, map[string]string{"a.gon": tc.a, "b.gon": tc.b})
			res, err := checkPkg(t, dir, "a.gon", "b.gon")
			if err != nil {
				t.Fatalf("an excluded file must not fail the load: %v", err)
			}
			if got := checkedPaths(res); len(got) != 1 || got[0] != tc.checked {
				t.Fatalf("checked %v, want only %s", got, tc.checked)
			}
			wantCodes(t, res, tc.checked, "GN001")
			if len(res.TypeErrors) != 0 {
				t.Fatalf("type errors: %v", res.TypeErrors)
			}
		})
	}
}

// D1a / D1f: no source of the package is in the build. Every .gon is named,
// sorted, whatever the order they were given in.
func TestPackageAllSourcesExcludedIsExcludedError(t *testing.T) {
	dir := pkgDir(t, map[string]string{"c.gon": loadIgnored, "a_windows.gon": loadOK, "b.gon": loadIgnoredGN001})
	_, err := checkPkg(t, dir, "c.gon", "b.gon", "a_windows.gon")
	var ex *ExcludedError
	if !errors.As(err, &ex) {
		t.Fatalf("want *ExcludedError, got %v", err)
	}
	want := []string{filepath.Join(dir, "a_windows.gon"), filepath.Join(dir, "b.gon"), filepath.Join(dir, "c.gon")}
	if strings.Join(ex.Files, "|") != strings.Join(want, "|") {
		t.Fatalf("files %v, want %v", ex.Files, want)
	}
	if !strings.Contains(err.Error(), "build constraints exclude all Go files in ") {
		t.Fatalf("message: %v", err)
	}
}

// D1d: the package's only .gon is excluded, a plain .go file is in the build.
// The package is loaded and type-checked; no .gon is checked; not an error.
func TestPackageWithOnlyExcludedGonButGoFilesIsChecked(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadIgnoredGN001, "b.go": loadOK})
	res, err := checkPkg(t, dir, "a.gon")
	if err != nil {
		t.Fatalf("the package has b.go: %v", err)
	}
	if len(res.Files) != 0 || len(res.TypeErrors) != 0 {
		t.Fatalf("files %v, type errors %v", checkedPaths(res), res.TypeErrors)
	}
	// b.go is really type-checked: a type error in it is reported.
	dir = pkgDir(t, map[string]string{"a.gon": loadIgnoredGN001, "b.go": "package main\n\nfunc main() { var x int = \"s\"; _ = x }\n"})
	res, err = checkPkg(t, dir, "a.gon")
	if err != nil || len(res.TypeErrors) == 0 {
		t.Fatalf("b.go's type error must be reported: err=%v type errors=%v", err, res.TypeErrors)
	}
}

// The overlay of a .gon and the package's own .go files are told apart: an
// included .gon, a plain .go file, an excluded .gon and an excluded plain .go
// file in one directory. Only the included .gon is checked.
func TestPackageMembershipMapsOverlaysToTheirGon(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon":        loadGN001,
		"b.go":         "package main\n\nfunc helper() {}\n",
		"c.gon":        loadIgnoredGN001,
		"d_windows.go": "package main\n\nfunc win() {}\n",
		"e_plan9.gon":  "package main\n\nfunc g() !*int {\n\treturn nil\n}\n",
	})
	res, err := checkPkg(t, dir, "e_plan9.gon", "c.gon", "a.gon")
	if err != nil {
		t.Fatal(err)
	}
	if got := checkedPaths(res); len(got) != 1 || got[0] != "a.gon" {
		t.Fatalf("checked %v, want only a.gon", got)
	}
	wantCodes(t, res, "a.gon", "GN001")
}

// A tag in GOFLAGS is honoured by the go command, which decides membership:
// the file it leaves out is skipped in either order (go/build alone would not
// know about the tag).
func TestPackageGOFLAGSTagDecidesMembership(t *testing.T) {
	t.Setenv("GOFLAGS", "-tags=gonloadtesttag")
	const tagged = "//go:build !gonloadtesttag\n\npackage main\n\nfunc f() !*int {\n\treturn nil\n}\n"
	for _, tc := range []struct{ name, a, b, checked string }{
		{"excluded file first", tagged, loadOK, "b.gon"},
		{"excluded file last", loadOK, tagged, "a.gon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := pkgDir(t, map[string]string{"a.gon": tc.a, "b.gon": tc.b})
			res, err := checkPkg(t, dir, "a.gon", "b.gon")
			if err != nil {
				t.Fatal(err)
			}
			if got := checkedPaths(res); len(got) != 1 || got[0] != tc.checked {
				t.Fatalf("checked %v, want only %s", got, tc.checked)
			}
		})
	}
}

// An excluded file next to a file with a parse error: the excluded one is
// skipped, the parse error is reported.
func TestPackageExcludedFileWithSiblingParseError(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadOK, "b.gon": loadBroken, "c.gon": loadIgnored})
	res, err := checkPkg(t, dir, "a.gon", "b.gon", "c.gon")
	if err != nil {
		t.Fatal(err)
	}
	if !anyContains(res.TypeErrors, "b.go:3:14: expected ')'") {
		t.Fatalf("b.gon's parse error: %v", res.TypeErrors)
	}
	for _, p := range checkedPaths(res) {
		if p == "c.gon" {
			t.Fatal("c.gon is excluded and must not be checked")
		}
	}
}

// The other way round: go/build rejects the file, the loader (with the GOFLAGS
// tag) includes it. The loader decides; nothing fails.
func TestPackageLoaderIncludesWhatBuildContextRejects(t *testing.T) {
	t.Setenv("GOFLAGS", "-tags=gonloadtesttag")
	dir := pkgDir(t, map[string]string{
		"a.gon": loadOK,
		"b.gon": "//go:build gonloadtesttag\n\npackage main\n\nfunc g() {}\n",
	})
	res, err := checkPkg(t, dir, "a.gon", "b.gon")
	if err != nil {
		t.Fatalf("the loader included the file; the check must run: %v", err)
	}
	if len(res.Files) != 2 || len(res.TypeErrors) != 0 {
		t.Fatalf("both files must be checked: files=%d type errors=%v", len(res.Files), res.TypeErrors)
	}
}

// An invalid package clause is a parse error of the result, not a load failure
// and not a build constraint.
func TestPackageInvalidPackageClauseIsATypeError(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadNoPkg})
	res, err := checkPkg(t, dir, "a.gon")
	if err != nil {
		t.Fatalf("must not be a load failure: %v", err)
	}
	if !anyContains(res.TypeErrors, "expected 'package', found pakage") {
		t.Fatalf("the loader's parse error must be reported: %v", res.TypeErrors)
	}
	if anyContains(res.TypeErrors, "build constraint") {
		t.Fatalf("must not mention build constraints: %v", res.TypeErrors)
	}
}

func TestPackageInvalidPackageClauseStillChecksSiblings(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadNoPkg, "b.gon": loadGN001})
	res, err := checkPkg(t, dir, "a.gon", "b.gon")
	if err != nil {
		t.Fatal(err)
	}
	wantCodes(t, res, "b.gon", "GN001")
	if !anyContains(res.TypeErrors, "expected 'package'") {
		t.Fatalf("a.gon's parse error must be reported: %v", res.TypeErrors)
	}
}

// Two parse errors in two files: both are reported, neither hides the other.
func TestPackageSeveralParseErrorsAreAllReported(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadNoPkg, "b.gon": loadBroken})
	res, err := checkPkg(t, dir, "a.gon", "b.gon")
	if err != nil {
		t.Fatal(err)
	}
	if !anyContains(res.TypeErrors, "a.go:1:1: expected 'package'") || !anyContains(res.TypeErrors, "b.go:3:14: expected ')'") {
		t.Fatalf("both files' parse errors must be reported: %v", res.TypeErrors)
	}
}

func TestErrorFile(t *testing.T) {
	for in, want := range map[string]string{
		"/x/a.go:1:2":   "/x/a.go",
		"/x/a.go:3":     "/x/a.go",
		"/x/a.go":       "/x/a.go",
		"-":             "-",
		"":              "",
		`C:\x\a.go:1:2`: `C:\x\a.go`,
	} {
		if got := errorFile(in); got != want {
			t.Errorf("errorFile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSamePath(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{filepath.Join(real, "a.go"), filepath.Join(real, ".", "a.go"), true},
		{filepath.Join(link, "a.go"), filepath.Join(real, "a.go"), true},
		{filepath.Join(real, "a.go"), filepath.Join(real, "b.go"), false},
		{filepath.Join(link, "a.go"), filepath.Join(real, "b.go"), false},
		{"-", filepath.Join(real, "a.go"), false},
		{"", filepath.Join(real, "a.go"), false},
	} {
		if got := samePath(tc.a, tc.b); got != tc.want {
			t.Errorf("samePath(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func anyContains(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}
