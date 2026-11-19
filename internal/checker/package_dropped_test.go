// A .gon the loader drops from a package inside a module (v1.19).
//
// A file the loader lists in neither GoFiles nor IgnoredFiles and that has no
// parse error of its own is explained by go/build, asked about that file only:
//
//	an error                    -> a malformed //go:build line, a source error
//	no error, no match          -> a name the go tool ignores (`_x`, `.x`)
//	a match                     -> nothing explains the drop (generic failure)
//
// go/build's verdict is never applied to a file the loader loaded or listed as
// ignored.
package checker

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const (
	dropMalformed = "//go:build (linux\n\npackage main\n\nfunc f() !*int {\n\treturn nil\n}\n"
	dropWindows   = "//go:build windows\n\npackage main\n\nfunc w() !*int {\n\treturn nil\n}\n"
)

func TestClassifyDropped(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name   string
		src    string
		kind   dropKind
		reason string
	}{
		{"a.go", dropMalformed, dropInvalid, "parsing //go:build line: missing close paren"},
		{"b.go", "//go:build linux &&\n\npackage p\n", dropInvalid, "parsing //go:build line: unexpected end of expression"},
		{"c.go", "//go:build ignore\n//go:build linux\n\npackage p\n", dropInvalid, "multiple //go:build comments"},
		{"_x.go", "package p\n", dropIgnored, ""},
		{".y.go", "package p\n", dropIgnored, ""},
		// Not a rule about names: a valid constraint that does not match is the
		// same verdict from go/build. It never reaches this function for a real
		// package, because the loader lists such a file in IgnoredFiles
		// (TestPackageValidConstraintThatDoesNotMatchIsNotInvalid).
		{"w.go", "//go:build windows\n\npackage p\n", dropIgnored, ""},
		{"ok.go", "package p\n", dropUnexplained, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op := filepath.Join(dir, tc.name)
			d := classifyDropped(dir, map[string][]byte{op: []byte(tc.src)}, op)
			if d.kind() != tc.kind {
				t.Fatalf("kind = %v, want %v (%+v)", d.kind(), tc.kind, d)
			}
			if tc.kind == dropInvalid {
				if d.reason != tc.reason {
					t.Fatalf("reason = %q, want %q", d.reason, tc.reason)
				}
				if d.text != tc.name+": "+tc.reason {
					t.Fatalf("text = %q", d.text)
				}
			}
		})
	}
}

func TestReasonOf(t *testing.T) {
	if got := reasonOf("a.go", errors.New("a.go: parsing //go:build line: missing close paren")); got != "parsing //go:build line: missing close paren" {
		t.Fatalf("with the prefix: %q", got)
	}
	// Without the prefix nothing is cut.
	if got := reasonOf("a.go", errors.New("something else entirely")); got != "something else entirely" {
		t.Fatalf("without the prefix: %q", got)
	}
	// Only the exact "<name>: " prefix is removed.
	if got := reasonOf("a.go", errors.New("b.go: a.go: x")); got != "b.go: a.go: x" {
		t.Fatalf("another name: %q", got)
	}
}

// The loader's own error is dropped only when it is character for character
// go/build's error for an invalid file; anything else is kept.
func TestRestatesInvalidFile(t *testing.T) {
	restated := map[string]bool{"a.go: parsing //go:build line: missing close paren": true}
	same := packages.Error{Kind: packages.ListError, Msg: "a.go: parsing //go:build line: missing close paren"}
	for name, tc := range map[string]struct {
		e    packages.Error
		want bool
	}{
		"identical list error":   {same, true},
		"other wording":          {packages.Error{Kind: packages.ListError, Msg: "a.go: parsing //go:build line: missing )"}, false},
		"other file":             {packages.Error{Kind: packages.ListError, Msg: "b.go: parsing //go:build line: missing close paren"}, false},
		"with a position":        {packages.Error{Kind: packages.ListError, Pos: "a.go:1:1", Msg: same.Msg}, false},
		"a type error":           {packages.Error{Kind: packages.TypeError, Msg: same.Msg}, false},
		"a parse error":          {packages.Error{Kind: packages.ParseError, Msg: same.Msg}, false},
		"the text with a prefix": {packages.Error{Kind: packages.ListError, Msg: "-: " + same.Msg}, false},
		"the text with a suffix": {packages.Error{Kind: packages.ListError, Msg: same.Msg + "."}, false},
	} {
		if got := restatesInvalidFile(tc.e, restated); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
	if restatesInvalidFile(same, nil) {
		t.Error("nothing restated: nothing is dropped")
	}
}

// A malformed constraint is a source error of the package, not a load failure:
// the file is not checked, its siblings are, in either order.
func TestPackageMalformedConstraintIsAnInvalidFile(t *testing.T) {
	for _, tc := range []struct{ name, a, b, checked string }{
		{"malformed first", dropMalformed, loadGN001, "b.gon"},
		{"malformed last", loadGN001, dropMalformed, "a.gon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := pkgDir(t, map[string]string{"a.gon": tc.a, "b.gon": tc.b})
			res, err := checkPkg(t, dir, "a.gon", "b.gon")
			if err != nil {
				t.Fatalf("a malformed constraint is not a load failure: %v", err)
			}
			bad := "a.gon"
			if tc.checked == "a.gon" {
				bad = "b.gon"
			}
			if len(res.InvalidFiles) != 1 || res.InvalidFiles[0].Path != filepath.Join(dir, bad) ||
				res.InvalidFiles[0].Reason != "parsing //go:build line: missing close paren" {
				t.Fatalf("InvalidFiles = %+v", res.InvalidFiles)
			}
			if got := checkedPaths(res); len(got) != 1 || got[0] != tc.checked {
				t.Fatalf("checked %v, want only %s", got, tc.checked)
			}
			wantCodes(t, res, tc.checked, "GN001") // the sibling's diagnostic is kept
			// The loader's own restatement is not repeated.
			if len(res.TypeErrors) != 0 {
				t.Fatalf("type errors: %v", res.TypeErrors)
			}
		})
	}
}

// A package whose only source is malformed is that source error, never "no
// source in the build" and never a load failure.
func TestPackageOnlyMalformedConstraint(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": dropMalformed})
	res, err := checkPkg(t, dir, "a.gon")
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if len(res.InvalidFiles) != 1 || len(res.Files) != 0 || len(res.TypeErrors) != 0 {
		t.Fatalf("%+v", res)
	}
}

// (Owner audit, strengthening 1.) A VALID constraint that does not match the
// platform is the loader's IgnoredFiles, not a name the go tool ignores and
// not a malformed constraint: no InvalidFile, no error, and with nothing else
// in the build it is the package-level "no source" (ExcludedError) listing it.
func TestPackageValidConstraintThatDoesNotMatchIsNotInvalid(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": dropWindows, "b.gon": loadGN001, "c_plan9.gon": loadGN001})
	res, err := checkPkg(t, dir, "a.gon", "b.gon", "c_plan9.gon")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.InvalidFiles) != 0 || len(res.TypeErrors) != 0 {
		t.Fatalf("invalid %+v, type errors %v", res.InvalidFiles, res.TypeErrors)
	}
	if got := checkedPaths(res); len(got) != 1 || got[0] != "b.gon" {
		t.Fatalf("checked %v, want only b.gon", got)
	}

	// Alone (plus a name the go tool ignores): the package has no source, and
	// every file out of the build is listed, sorted, from both lists.
	dir = pkgDir(t, map[string]string{"a_windows.gon": loadGN001, "_x.gon": loadGN001, "c.gon": dropWindows})
	_, err = checkPkg(t, dir, "a_windows.gon", "_x.gon", "c.gon")
	var ex *ExcludedError
	if !errors.As(err, &ex) {
		t.Fatalf("want *ExcludedError, got %v", err)
	}
	want := []string{filepath.Join(dir, "_x.gon"), filepath.Join(dir, "a_windows.gon"), filepath.Join(dir, "c.gon")}
	if strings.Join(ex.Files, "|") != strings.Join(want, "|") {
		t.Fatalf("files %v, want %v", ex.Files, want)
	}
}

// (Owner audit, strengthening 2.) go/build's "no match" is not a rule about
// every file: a tag in GOFLAGS makes the go command include a file go/build
// alone rejects, and that file, which the loader loaded, is checked, not
// treated as ignored.
func TestPackageNoMatchIsNotAGlobalRule(t *testing.T) {
	const tagged = "//go:build gondroppedtag\n\npackage main\n\nfunc t() !*int {\n\treturn nil\n}\n"
	dir := pkgDir(t, map[string]string{"a.gon": loadOK, "b.gon": tagged})

	// Without the tag the loader lists b.gon in IgnoredFiles: not checked.
	res, err := checkPkg(t, dir, "a.gon", "b.gon")
	if err != nil {
		t.Fatal(err)
	}
	if got := checkedPaths(res); len(got) != 1 || got[0] != "a.gon" || len(res.InvalidFiles) != 0 {
		t.Fatalf("without the tag: checked %v, invalid %+v", got, res.InvalidFiles)
	}

	// With the tag (GOFLAGS, which go/build does not read) the loader includes
	// it, go/build still says no match, and the file is checked: its GN001 shows.
	t.Setenv("GOFLAGS", "-tags=gondroppedtag")
	res, err = checkPkg(t, dir, "a.gon", "b.gon")
	if err != nil {
		t.Fatal(err)
	}
	if got := checkedPaths(res); len(got) != 2 {
		t.Fatalf("with the tag: checked %v, want both", got)
	}
	wantCodes(t, res, "b.gon", "GN001")
	if len(res.InvalidFiles) != 0 {
		t.Fatalf("invalid %+v", res.InvalidFiles)
	}
}

// Names the go tool ignores are not part of the package and are not errors; a
// package with nothing else is the package-level "no source".
func TestPackageNamesTheGoToolIgnores(t *testing.T) {
	dir := pkgDir(t, map[string]string{"a.gon": loadGN001, "_x.gon": loadGN001, ".y.gon": loadGN001})
	res, err := checkPkg(t, dir, "a.gon", "_x.gon", ".y.gon")
	if err != nil {
		t.Fatalf("ignored names are not an error: %v", err)
	}
	if got := checkedPaths(res); len(got) != 1 || got[0] != "a.gon" || len(res.InvalidFiles) != 0 {
		t.Fatalf("checked %v, invalid %+v", got, res.InvalidFiles)
	}

	dir = pkgDir(t, map[string]string{"_x.gon": loadGN001})
	_, err = checkPkg(t, dir, "_x.gon")
	var ex *ExcludedError
	if !errors.As(err, &ex) || len(ex.Files) != 1 || ex.Files[0] != filepath.Join(dir, "_x.gon") {
		t.Fatalf("only an ignored name: want *ExcludedError naming it, got %v", err)
	}
}
