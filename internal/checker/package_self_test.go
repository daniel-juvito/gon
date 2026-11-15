package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RFC rfc-package-check.md §9.8: a package of Gon's own size must check cleanly
// in package mode. The ten non-test files of this very package are copied as
// .gon files (they contain no `!`, so they are valid .gon) into a scratch
// directory inside the module and checked together.
//
// A control file with one deliberate violation guards against a checker that
// silently sees nothing: it must be reported, and only in that file.
func TestPackageModeChecksGonsOwnCheckerPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks a ten-file package")
	}
	scratch, err := os.MkdirTemp(".", "_selfcheck_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(scratch)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		src, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		gon := strings.TrimSuffix(n, ".go") + ".gon"
		if err := os.WriteFile(filepath.Join(scratch, gon), src, 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, gon)
	}
	if len(names) < 8 {
		t.Fatalf("expected this package's source files, found %v", names)
	}

	control := "package checker\n\nfunc spikeTake(p !*int) {}\n\nfunc spikeCall() { spikeTake(nil) }\n"
	if err := os.WriteFile(filepath.Join(scratch, "zz_control.gon"), []byte(control), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := checkPkg(t, scratch, append(names, "zz_control.gon")...)
	if err != nil {
		t.Fatalf("CheckPackage: %v", err)
	}
	if len(res.TypeErrors) != 0 {
		t.Fatalf("the package must type-check in package mode: %v", res.TypeErrors)
	}
	for _, n := range names {
		if got := pkgCodes(res, n); len(got) != 0 {
			t.Errorf("%s: false positive(s) %v", n, got)
		}
	}
	wantCodes(t, res, "zz_control.gon", "GN001")
}
