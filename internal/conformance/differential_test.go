package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/preproc"
	"github.com/daniel-juvito/gon/transpiler"
)

// repoRoot is the module root, two levels above this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// walkSources calls fn for every file of the repository with the given suffix,
// leaving out testdata (its fixtures hold invalid and new syntax on purpose).
func walkSources(t *testing.T, suffix string, fn func(path string, src []byte)) int {
	t.Helper()
	root := repoRoot(t)
	n := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, suffix) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		n++
		fn(path, src)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// lf returns b with CRLF line endings replaced by LF.
func lf(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// TestDifferentialGoSourcesAreUntouched is the "identical" criterion of the plan
// (WP1): the repository's own Go files are valid Go and use no Gon syntax, so
// the preprocessor must return each one byte for byte with no offset. A new rule
// that misreads an ordinary Go shape such as (!a) && b or f(!a)(b) fails here.
func TestDifferentialGoSourcesAreUntouched(t *testing.T) {
	n := walkSources(t, ".go", func(path string, src []byte) {
		r := preproc.Process(path, src)
		if !bytes.Equal(r.Clean, src) || len(r.NonNilOffsets) != 0 {
			t.Errorf("%s: the preprocessor changed a plain Go file (%d offsets)", path, len(r.NonNilOffsets))
		}
	})
	if n < 50 {
		t.Fatalf("only %d Go files found: the walk is not reaching the repository", n)
	}
}

// TestDifferentialGonSourcesTranspileToTheCommittedGo is the "expected output"
// criterion: every .gon of the production packages, processed by the code under
// test, must transpile to exactly the committed .go next to it (the check that
// scripts/check-gon-sources.sh makes with the pinned gon).
func TestDifferentialGonSourcesTranspileToTheCommittedGo(t *testing.T) {
	n := walkSources(t, ".gon", func(path string, src []byte) {
		if strings.HasSuffix(path, "_test.gon") {
			return
		}
		want, err := os.ReadFile(strings.TrimSuffix(path, ".gon") + ".go")
		if err != nil {
			t.Errorf("%s: no committed .go next to it: %v", path, err)
			return
		}
		got, err := transpiler.Transpile(preproc.Process(path, src).Clean)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return
		}
		// Only the line ending is normalised: a checkout with autocrlf (Windows)
		// turns the committed .go and the .gon into CRLF. Any other difference
		// still fails.
		if !bytes.Equal(lf(got), lf(want)) {
			t.Errorf("%s: the transpile output differs from the committed .go", path)
		}
	})
	if n < 20 {
		t.Fatalf("only %d .gon files found: the walk is not reaching the repository", n)
	}
}
