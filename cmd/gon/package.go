package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daniel-juvito/gon/internal/checker"
	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

// Package mode (v1.10 / M8, docs/rfc-package-check.md).
//
// An operand of `gon check` that names a DIRECTORY, or a `./...` pattern,
// selects package mode: the directory's .gon files are type-checked together as
// one package. An operand that names a FILE keeps the single-file behaviour
// exactly (a directory was an error before, so the new form is additive).

// checkTarget is one operand of `gon check` after classification.
type checkTarget struct {
	path string
	dir  bool // package mode: path is a directory holding .gon files
	// pattern is the `...` operand the directory was found by, empty for a
	// directory named explicitly. A package found by a pattern none of whose
	// sources is in the build is skipped, like `go list ./...` does (v1.16).
	pattern string
}

// expandCheckOperands classifies the operands. A file operand (or anything that
// is not a directory or a pattern) is returned as-is so the existing file path
// reports its own errors with its own wording. `dir/...` expands to every
// directory below dir that holds .gon files, following the `go` tool's
// conventions: testdata, vendor and names starting with `_` or `.` are skipped,
// and so is a directory with its own go.mod (a nested module is not part of the
// enclosing one; v1.16). A directory named explicitly is checked whatever
// module it is in.
func expandCheckOperands(operands []string) ([]checkTarget, error) {
	var out []checkTarget
	for _, op := range operands {
		if root, ok := patternRoot(op); ok {
			dirs, err := packageDirsUnder(root)
			if err != nil {
				return nil, err
			}
			if len(dirs) == 0 {
				return nil, fmt.Errorf("no .gon files found under %s", op)
			}
			for _, d := range dirs {
				out = append(out, checkTarget{path: d, dir: true, pattern: op})
			}
			continue
		}
		if fi, err := os.Stat(op); err == nil && fi.IsDir() {
			has, err := hasGonFiles(op)
			if err != nil {
				return nil, err
			}
			if !has {
				if onlyTestGon(op) {
					return nil, fmt.Errorf("no .gon files in %s (only *_test.gon files, which are not checked in package mode)", op)
				}
				return nil, fmt.Errorf("no .gon files in %s", op)
			}
			out = append(out, checkTarget{path: op, dir: true})
			continue
		}
		out = append(out, checkTarget{path: op})
	}
	return out, nil
}

// patternRoot reports whether op is a `...` pattern and returns its root.
func patternRoot(op string) (string, bool) {
	switch {
	case op == "...":
		return ".", true
	case strings.HasSuffix(op, "/..."):
		root := strings.TrimSuffix(op, "/...")
		if root == "" {
			root = "/"
		}
		return root, true
	}
	return "", false
}

func packageDirsUnder(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			if fi, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && !fi.IsDir() {
				return filepath.SkipDir // a nested module
			}
			name := d.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
		}
		if has, err := hasGonFiles(path); err != nil {
			return err
		} else if has {
			dirs = append(dirs, path)
		}
		return nil
	})
	return dirs, err
}

// onlyTestGon reports whether dir holds *_test.gon files and nothing else that
// package mode would check, so the "no .gon files" error can say why.
func onlyTestGon(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.gon") {
			return true
		}
	}
	return false
}

func hasGonFiles(dir string) (bool, error) {
	names, err := gonFilesIn(dir)
	return len(names) > 0, err
}

// gonFilesIn lists the .gon files directly inside dir, sorted by name.
// `*_test.gon` files are not part of package mode, like `*_test.go` for the go
// tool's non-test package: the package is loaded without tests, so such a file
// could never join it. Naming one explicitly as a FILE operand still works.
func gonFilesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".gon") && !strings.HasSuffix(e.Name(), "_test.gon") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// packageOutcome is what checking one package contributes to the invocation.
type packageOutcome struct {
	protocol   []protocolDiagnostic
	hasErrors  bool
	anyDiag    bool
	typeErrors []string
}

// loadedPackage is a directory's .gon files, preprocessed and checked together
// (the M8 machinery), shared by `check`, `transpile` and `build`.
type loadedPackage struct {
	files     []checker.PackageFile
	srcByPath map[string][]byte
	res       *checker.PackageResult
}

// loadPackage reads and checks the .gon files of t's directory as one package.
// A non-zero exit code means the package could not be checked at all and is
// always 2 (Protocol v1 §7): a .gon file is unreadable, or the package could
// not be loaded (the go command or the module is broken, no source of the
// package is in the build). checker.CheckPackage reports parse, type and
// contract errors in its result, never as an error. The message is already
// printed. skipped is true, with no message, for a package found by a `...`
// pattern none of whose sources is in the build.
func loadPackage(t checkTarget) (lp *loadedPackage, skipped bool, code int) {
	dir := t.path
	names, err := gonFilesIn(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: read %s: %v\n", dir, err)
		return nil, false, 2
	}
	lp = &loadedPackage{srcByPath: make(map[string][]byte, len(names))}
	for _, n := range names {
		path := filepath.Join(dir, n)
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: read %s: %v\n", path, err)
			return nil, false, 2
		}
		r := preproc.Process(path, src)
		lp.srcByPath[path] = src
		lp.files = append(lp.files, checker.PackageFile{Path: path, Clean: r.Clean, NonNilOffsets: r.NonNilOffsets})
	}

	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		dirAbs = dir
	}
	wd, err := os.Getwd()
	if err != nil {
		wd = dirAbs
	}
	resolver := gna.DefaultChain(wd, dirAbs)

	res, err := checker.CheckPackage(dir, lp.files, resolver)
	var excluded *checker.ExcludedError
	if errors.As(err, &excluded) && t.pattern != "" {
		return nil, true, 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		return nil, false, 2
	}
	lp.res = res
	return lp, false, 0
}

// warnEmptyPatterns prints, like the go command, a warning for each `...`
// operand all of whose packages were skipped because none of their sources is
// in the build. checked holds the directories that were not skipped.
func warnEmptyPatterns(targets []checkTarget, checked map[string]bool) {
	found := make(map[string]bool)
	var order []string
	for _, t := range targets {
		if t.pattern == "" {
			continue
		}
		if _, seen := found[t.pattern]; !seen {
			order = append(order, t.pattern)
			found[t.pattern] = false
		}
		if checked[t.path] {
			found[t.pattern] = true
		}
	}
	for _, p := range order {
		if !found[p] {
			fmt.Fprintf(os.Stderr, "gon: warning: %q matched no packages with .gon files in the build\n", p)
		}
	}
}

// checkPackageDir checks the .gon files of t's directory as one package. A
// non-zero exit code means the package could not be checked at all; skipped
// is loadPackage's.
func checkPackageDir(t checkTarget, jsonOut bool, source string) (out packageOutcome, skipped bool, code int) {
	lp, skipped, code := loadPackage(t)
	if lp == nil {
		return out, skipped, code
	}
	res, srcByPath := lp.res, lp.srcByPath

	for _, f := range res.Files {
		fileAbs, err := protocolFilePath(f.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: path %s: %v\n", f.Path, err)
			return out, false, 2
		}
		for _, d := range f.Diagnostics {
			out.anyDiag = true
			if d.IsError() {
				out.hasErrors = true
			}
			if jsonOut {
				out.protocol = append(out.protocol, toProtocolDiagnostic(d, fileAbs, source, srcByPath[f.Path]))
			} else {
				// Several packages can hold files with the same base name, so
				// the human line names the file by its path, not its base.
				line := d.String()
				fmt.Fprintln(os.Stderr, filepath.ToSlash(f.Path)+strings.TrimPrefix(line, d.File))
			}
		}
	}
	// Recorded in --json mode too: Protocol v1 has no field for a Go type error,
	// so the document does not carry it, but the exit code must still say the
	// package does not compile.
	out.typeErrors = res.TypeErrors
	return out, false, 0
}
