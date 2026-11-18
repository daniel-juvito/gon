package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/daniel-juvito/gon/internal/checker"
	"github.com/daniel-juvito/gon/internal/preproc"
	"github.com/daniel-juvito/gon/transpiler"
)

// Directory form of `transpile` and `build` (v1.12 / M9,
// docs/rfc-transpile-dir.md).
//
// A directory operand (or `dir/...`) selects package mode, as for `check`: the
// package is checked as a whole with the M8 machinery, and a package that does
// not pass has NO file written. A package that passes gets `<stem>.go` next to
// each `.gon`, replacing the same-named `.go` and touching nothing else. A file
// operand keeps the single-file behaviour.

// isDirOperand reports whether op selects package mode.
func isDirOperand(op string) bool {
	if _, ok := patternRoot(op); ok {
		return true
	}
	fi, err := os.Stat(op)
	return err == nil && fi.IsDir()
}

// runEmitDir implements `transpile` and `build` for directory operands.
func runEmitDir(cmd string, operands []string) int {
	// build: the first operand is the target, the rest is passed to go build.
	var goFlags []string
	if cmd == "build" {
		goFlags = operands[1:]
		operands = operands[:1]
	}
	for _, op := range operands {
		if !isDirOperand(op) {
			fmt.Fprintf(os.Stderr, "gon: %s cannot mix a file operand (%s) with directories\n", cmd, op)
			return 2
		}
	}

	targets, err := expandCheckOperands(operands)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		return 2
	}

	exit := 0
	checkedDirs := make(map[string]bool)
	for _, t := range orderByDependencies(targets) {
		code, skipped := emitPackage(t)
		if code > exit {
			exit = code
		}
		if !skipped {
			checkedDirs[t.path] = true
		}
	}
	warnEmptyPatterns(targets, checkedDirs)
	if exit != 0 || cmd == "transpile" {
		return exit
	}

	// build: every package is transpiled; now build what was named.
	pkg := goPackageArg(operands[0])
	goArgs := append(append([]string{"build"}, goFlags...), pkg)
	goCmd := exec.Command("go", goArgs...)
	goCmd.Stdout = os.Stdout
	goCmd.Stderr = os.Stderr
	if err := goCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gon: go build failed: %v\n", err)
		return 1
	}
	fmt.Printf("built: %s\n", pkg)
	return 0
}

// goPackageArg turns an operand into something `go build` reads as a directory
// pattern: `internal/foo` would otherwise be an import path.
func goPackageArg(op string) string {
	if op == "..." || op == "." || op == ".." ||
		strings.HasPrefix(op, "./") || strings.HasPrefix(op, "../") || filepath.IsAbs(op) {
		return op
	}
	return "./" + op
}

// emitPackage checks one package and, only if it passes, writes its .go files.
// The return value is an exit code: 0 written, 1 did not pass, 2 unreadable or
// not loadable (see loadPackage); skipped is loadPackage's.
func emitPackage(t checkTarget) (code int, skipped bool) {
	dir := t.path
	lp, skipped, code := loadPackage(t)
	if lp == nil {
		return code, skipped
	}

	hasErrors := false
	for _, f := range lp.res.Files {
		for _, d := range f.Diagnostics {
			if d.IsError() {
				hasErrors = true
			}
			// Several packages can hold files with the same base name.
			fmt.Fprintln(os.Stderr, filepath.ToSlash(f.Path)+strings.TrimPrefix(d.String(), d.File))
		}
	}
	for _, te := range lp.res.TypeErrors {
		fmt.Fprintf(os.Stderr, "gon: Go type error: %s\n", te)
	}
	if hasErrors || len(lp.res.TypeErrors) > 0 {
		fmt.Fprintf(os.Stderr, "gon: %s not transpiled: nothing was written for this package\n", filepath.ToSlash(dir))
		return 1, false
	}

	if err := writePackage(checkedFiles(lp)); err != nil {
		fmt.Fprintf(os.Stderr, "gon: transpile error: %v\n", err)
		return 1, false
	}
	return 0, false
}

// createExclusive creates a new, uniquely named temporary file next to dst. It
// is opened with O_EXCL (an existing file is never reused or truncated) and mode
// 0644, so the process umask applies exactly as it does to os.WriteFile; a
// forced chmod would ignore the umask.
func createExclusive(dst string) (*os.File, error) {
	dir, base := filepath.Dir(dst), filepath.Base(dst)
	for try := 0; try < 100; try++ {
		var b [6]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		name := filepath.Join(dir, "."+base+".gon-"+hex.EncodeToString(b[:]))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("cannot create a temporary file next to %s", dst)
}

// writePackage formats every file first, so a formatting failure writes
// nothing, then writes each through a uniquely named temporary file created
// (exclusively, createExclusive) in the destination directory and renames it
// into place. The
// temporary name is never derived from a file that may already exist, and only
// temporaries this call created are ever removed.
// checkedFiles are the files of lp that were checked: a .gon the go command
// left out of the package (build constraints) is never written (v1.16).
func checkedFiles(lp *loadedPackage) []checker.PackageFile {
	checked := make(map[string]bool, len(lp.res.Files))
	for _, f := range lp.res.Files {
		checked[f.Path] = true
	}
	var out []checker.PackageFile
	for _, f := range lp.files {
		if checked[f.Path] {
			out = append(out, f)
		}
	}
	return out
}

func writePackage(files []checker.PackageFile) error {
	type out struct {
		path string
		data []byte
		tmp  string // set once this call has created it
	}
	var outs []out
	for _, f := range files {
		data, err := transpiler.Transpile(f.Clean)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		outs = append(outs, out{path: strings.TrimSuffix(f.Path, ".gon") + ".go", data: data})
	}
	cleanup := func() {
		for _, o := range outs {
			if o.tmp != "" {
				_ = os.Remove(o.tmp)
			}
		}
	}
	for i := range outs {
		o := &outs[i]
		tf, err := createExclusive(o.path)
		if err != nil {
			cleanup()
			return err
		}
		o.tmp = tf.Name()
		_, werr := tf.Write(o.data)
		if cerr := tf.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			cleanup()
			return werr
		}
	}
	for i := range outs {
		o := &outs[i]
		if err := os.Rename(o.tmp, o.path); err != nil {
			cleanup()
			return err
		}
		o.tmp = "" // renamed: nothing left to remove
		fmt.Printf("transpiled: %s\n", o.path)
	}
	return nil
}

// orderByDependencies sorts the packages of a `./...` run so that a package comes
// after every package of the same module that it imports. A package is checked
// against the `.go` files of the packages it imports; with the `.gon` of a
// dependency not yet transpiled (a fresh tree), alphabetical order would check an
// importer against nothing and fail. Independent packages keep their order.
// Outside a module the order is unchanged.
func orderByDependencies(targets []checkTarget) []checkTarget {
	if len(targets) < 2 {
		return targets
	}
	abs := make([]string, len(targets))
	for i, t := range targets {
		a, err := filepath.Abs(t.path)
		if err != nil {
			return targets
		}
		abs[i] = a
	}
	root := findGoModRoot(abs[0])
	if root == "" {
		return targets
	}
	modPath := modulePath(filepath.Join(root, "go.mod"))
	if modPath == "" {
		return targets
	}
	importPath := func(dir string) string {
		rel, err := filepath.Rel(root, dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		if rel == "." {
			return modPath
		}
		return modPath + "/" + filepath.ToSlash(rel)
	}
	index := make(map[string]int, len(targets))
	for i, d := range abs {
		if p := importPath(d); p != "" {
			index[p] = i
		}
	}
	// deps[i]: packages (by index) that package i imports.
	deps := make([]map[int]bool, len(targets))
	for i, d := range abs {
		deps[i] = map[int]bool{}
		names, err := gonFilesIn(d)
		if err != nil {
			continue
		}
		for _, n := range names {
			src, err := os.ReadFile(filepath.Join(d, n))
			if err != nil {
				continue
			}
			clean := preproc.Process(filepath.Join(d, n), src).Clean
			f, err := parser.ParseFile(token.NewFileSet(), n, clean, parser.ImportsOnly)
			if err != nil {
				continue
			}
			for _, imp := range f.Imports {
				p, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					continue
				}
				if j, ok := index[p]; ok && j != i {
					deps[i][j] = true
				}
			}
		}
	}
	// Kahn's algorithm, picking the lowest index among the ready packages so the
	// result is deterministic.
	done := make([]bool, len(targets))
	out := make([]checkTarget, 0, len(targets))
	for len(out) < len(targets) {
		progressed := false
		for i := range targets {
			if done[i] {
				continue
			}
			ready := true
			for j := range deps[i] {
				if !done[j] {
					ready = false
					break
				}
			}
			if ready {
				done[i] = true
				out = append(out, targets[i])
				progressed = true
				break
			}
		}
		if !progressed { // an import cycle cannot compile; keep the rest in order
			for i := range targets {
				if !done[i] {
					out = append(out, targets[i])
				}
			}
			break
		}
	}
	return out
}

// modulePath reads the `module` line of a go.mod.
func modulePath(gomod string) string {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module ")), `"`)
		}
	}
	return ""
}
