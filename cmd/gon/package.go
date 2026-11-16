package main

import (
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
}

// expandCheckOperands classifies the operands. A file operand (or anything that
// is not a directory or a pattern) is returned as-is so the existing file path
// reports its own errors with its own wording. `dir/...` expands to every
// directory below dir that holds .gon files, following the `go` tool's
// conventions: testdata, vendor and names starting with `_` or `.` are skipped.
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
				out = append(out, checkTarget{path: d, dir: true})
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

// checkPackageDir checks the .gon files of dir as one package. A non-zero exit
// code means the package could not be checked at all.
func checkPackageDir(dir string, jsonOut bool, source string) (packageOutcome, int) {
	var out packageOutcome

	names, err := gonFilesIn(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: read %s: %v\n", dir, err)
		return out, 2
	}
	srcByPath := make(map[string][]byte, len(names))
	var files []checker.PackageFile
	for _, n := range names {
		path := filepath.Join(dir, n)
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: read %s: %v\n", path, err)
			return out, 2
		}
		r := preproc.Process(path, src)
		srcByPath[path] = src
		files = append(files, checker.PackageFile{Path: path, Clean: r.Clean, NonNilOffsets: r.NonNilOffsets})
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

	res, err := checker.CheckPackage(dir, files, resolver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		return out, 1
	}

	for _, f := range res.Files {
		fileAbs, err := protocolFilePath(f.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: path %s: %v\n", f.Path, err)
			return out, 2
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
	return out, 0
}
