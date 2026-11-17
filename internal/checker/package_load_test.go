// Package loading failures (v1.15, Protocol v1 §7).
//
// CheckPackage returns an error only when the package cannot be loaded at all;
// a file the Go loader could not parse is a TypeError of the result instead.
// A file the loader left out is reported as excluded by build constraints only
// when go/build rejects it too, and the message does not depend on file order.
package checker

import (
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
)

// The excluded file sorts first (the loader then returns nothing) or last (the
// loader returns the package without it): one message either way.
func TestPackageExcludedFileMessageDoesNotDependOnOrder(t *testing.T) {
	first := pkgDir(t, map[string]string{"a.gon": loadIgnored, "b.gon": loadOK})
	_, errFirst := checkPkg(t, first, "a.gon", "b.gon")
	last := pkgDir(t, map[string]string{"a.gon": loadOK, "b.gon": loadIgnored})
	_, errLast := checkPkg(t, last, "a.gon", "b.gon")
	if errFirst == nil || errLast == nil {
		t.Fatalf("an excluded file must fail the load: first=%v last=%v", errFirst, errLast)
	}
	msgFirst := strings.ReplaceAll(strings.ReplaceAll(errFirst.Error(), filepath.Join(first, "a.gon"), "<excluded>"), first, "<dir>")
	msgLast := strings.ReplaceAll(strings.ReplaceAll(errLast.Error(), filepath.Join(last, "b.gon"), "<excluded>"), last, "<dir>")
	if msgFirst != msgLast {
		t.Fatalf("the message must not depend on file order:\n first: %s\n last:  %s", msgFirst, msgLast)
	}
	if !strings.Contains(msgFirst, "excluded by build constraints: <excluded>") {
		t.Fatalf("unexpected message: %s", msgFirst)
	}
}

func TestPackageExcludedFilesAreListedSorted(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadOK, "c.gon": loadIgnored, "b_plan9_s390.gon": loadOK})
	_, err := checkPkg(t, dir, "c.gon", "a.gon", "b_plan9_s390.gon")
	want := "excluded by build constraints: " + filepath.Join(dir, "b_plan9_s390.gon") + ", " + filepath.Join(dir, "c.gon")
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q in %v", want, err)
	}
}

// go/build does not read GOFLAGS; the go command does. A file the loader leaves
// out because of a GOFLAGS tag is not one go/build rejects, so it must not be
// called excluded by build constraints: the failure is reported generically.
func TestPackageLoaderAndBuildContextDisagree(t *testing.T) {
	t.Setenv("GOFLAGS", "-tags=gonloadtesttag")
	const tagged = "//go:build !gonloadtesttag\n\npackage main\n"
	for _, tc := range []struct{ name, a, b string }{
		{"excluded file first", tagged, loadOK},
		{"excluded file last", loadOK, tagged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := pkgDir(t, map[string]string{"a.gon": tc.a, "b.gon": tc.b})
			_, err := checkPkg(t, dir, "a.gon", "b.gon")
			if err == nil {
				t.Fatal("the loader left a file out: the load must fail")
			}
			if strings.Contains(err.Error(), "excluded by build constraints") {
				t.Fatalf("go/build accepts the file, so it must not be called excluded: %v", err)
			}
		})
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

// A sibling's parse error does not excuse a file the loader left out.
func TestPackageSiblingParseErrorDoesNotExcuseAMissingFile(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadOK, "b.gon": loadBroken, "c.gon": loadIgnored})
	_, err := checkPkg(t, dir, "a.gon", "b.gon", "c.gon")
	if err == nil || !strings.Contains(err.Error(), "excluded by build constraints: "+filepath.Join(dir, "c.gon")) {
		t.Fatalf("c.gon is excluded whatever b.gon's parse error: %v", err)
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
