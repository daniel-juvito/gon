package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daniel-juvito/gon/internal/checker"
	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

// runConvert is `gon convert` (RFC Part B with gon convert, accepted 2026-11-27): it
// rewrites `x := e` to `x := (T)(e)` where Part B's binding would otherwise break
// correct code, so that the variable stays nilable (see checker.PlanConvert).
//
// Unlike gon fmt and gon transpile, which write the files, the default here is a
// preview: the diff is printed and nothing is written. --write applies it. A file is
// written only after the rewritten source has been checked again and shown to have no
// diagnostic that the original did not, and to need no further rewrite.
//
// Exit codes: 0 done (also a preview that shows changes), 1 a file cannot be converted
// safely (a Go type error, a rewrite that changes the diagnostics), 2 usage or input.
func runConvert(args []string) int {
	write := false
	var files []string
	for _, a := range args {
		switch {
		case a == "--write":
			write = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "gon: convert: unknown flag %s\n", a)
			return 2
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "gon: convert requires a .gon file (a preview is printed; --write applies it)")
		return 2
	}
	targets, err := expandCheckOperands(files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		return 2
	}
	status := 0
	for _, t := range targets {
		var code int
		if t.dir {
			code = convertPackage(t, write)
		} else {
			if !strings.HasSuffix(t.path, ".gon") {
				fmt.Fprintf(os.Stderr, "gon: input must be a .gon file, got %s\n", t.path)
				return 2
			}
			code = convertFile(t.path, write)
		}
		if code == 2 {
			return 2
		}
		if code != 0 {
			status = code
		}
	}
	return status
}

// convertPackage converts the .gon files of one directory, checked together as a package
// (a file that uses the declarations of its siblings cannot be checked alone). The
// rewritten package must check the same way, file by file, and need no second pass,
// before anything is written.
func convertPackage(t checkTarget, write bool) int {
	lp, skipped, code := loadPackagePlans(t, true)
	if lp == nil {
		if skipped {
			return 0
		}
		return code
	}
	res := lp.res
	if reportInvalidFiles(res.InvalidFiles) || len(res.TypeErrors) > 0 {
		for _, te := range res.TypeErrors {
			fmt.Fprintf(os.Stderr, "gon: convert: %s: Go type error: %s\n", t.path, te)
		}
		return 1
	}
	status := 0
	rewritten := make(map[string][]byte)
	kept := make(map[string]int)
	for _, f := range res.Files {
		if f.Plan == nil {
			continue
		}
		for _, r := range f.Plan.Reports {
			fmt.Fprintf(os.Stderr, "%s:%d:%d: gon convert: %s\n", filepath.ToSlash(f.Path), r.Line, r.Column, r.Message)
			status = 1
		}
		if len(f.Plan.Edits) > 0 {
			rewritten[f.Path] = checker.ApplyConvert(lp.srcByPath[f.Path], f.Plan.Edits)
			kept[f.Path] = f.Plan.Kept
		}
	}
	if len(rewritten) == 0 {
		return status
	}

	// Check the rewritten package as the original was checked.
	var files []checker.PackageFile
	srcs := make(map[string][]byte, len(lp.files))
	for _, pf := range lp.files {
		src := lp.srcByPath[pf.Path]
		if out, ok := rewritten[pf.Path]; ok {
			src = out
		}
		srcs[pf.Path] = src
		r := preproc.Process(pf.Path, src)
		files = append(files, checker.PackageFile{Path: pf.Path, Clean: r.Clean, NonNilOffsets: r.NonNilOffsets, CleanToOrig: r.CleanToOrig})
	}
	res2, err := checker.CheckPackage(lp.dir, files, lp.resolver)
	if err != nil || len(res2.TypeErrors) > 0 {
		fmt.Fprintf(os.Stderr, "gon: convert: the rewritten package %s cannot be checked (%v %v); nothing was written\n", t.path, err, res2TypeErrors(res2))
		return 1
	}
	byPath := make(map[string]checker.FileDiagnostics, len(res2.Files))
	for _, f := range res2.Files {
		byPath[f.Path] = f
	}
	for _, f := range res.Files {
		after := byPath[f.Path]
		if a, b := diagnosticSet(f.Diagnostics), diagnosticSet(after.Diagnostics); a != b {
			fmt.Fprintf(os.Stderr, "gon: convert: the rewritten %s has other diagnostics than the original; nothing was written\n  before: %s\n  after:  %s\n", f.Path, a, b)
			return 1
		}
		if after.Plan != nil && len(after.Plan.Edits) != 0 {
			fmt.Fprintf(os.Stderr, "gon: convert: the rewrite of %s is not stable (a second pass would change it again); nothing was written\n", f.Path)
			return 1
		}
	}

	paths := make([]string, 0, len(rewritten))
	for p := range rewritten {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if !write {
		for _, p := range paths {
			printConvertDiff(filepath.ToSlash(p), lp.srcByPath[p], rewritten[p])
		}
		return status
	}
	if err := writeAll(paths, rewritten); err != nil {
		fmt.Fprintf(os.Stderr, "gon: write: %v; no file was changed\n", err)
		return 2
	}
	for _, p := range paths {
		fmt.Fprintf(os.Stderr, "gon: convert: %s: %d variable(s) kept nilable\n", filepath.ToSlash(p), kept[p])
	}
	return status
}

func res2TypeErrors(res *checker.PackageResult) []string {
	if res == nil {
		return nil
	}
	return res.TypeErrors
}

// convertDiagnostics checks src as `gon check <file>` does and returns the checker and
// the diagnostics. The error is the reason the file cannot be checked at all.
func convertDiagnostics(filename string, src []byte) (*checker.Checker, *preproc.Result, []*checker.Diagnostic, error) {
	result := preproc.Process(filename, src)
	goCtx, err := loadGoContext(filename, result.Clean)
	if err != nil {
		return nil, nil, nil, err
	}
	if reason, bad := checker.MalformedConstraint(result.Clean); bad {
		return nil, nil, nil, fmt.Errorf("%s: %s", filename, reason)
	}
	startDir := filepath.Dir(filename)
	if abs, err := filepath.Abs(startDir); err == nil {
		startDir = abs
	}
	wd, err := os.Getwd()
	if err != nil {
		wd = startDir
	}
	c, err := checker.NewWithAnnotations(filename, result.Clean, result.NonNilOffsets, gna.DefaultChain(wd, startDir))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse error: %w", err)
	}
	diags := c.Check()
	if err := goCtx.validate(filename, result.Clean); err != nil {
		return nil, nil, nil, fmt.Errorf("Go type error: %v", err)
	}
	return c, result, diags, nil
}

// convertFile converts one file; it returns 0, 1 or 2 as runConvert does.
func convertFile(filename string, write bool) int {
	src, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: read %s: %v\n", filename, err)
		return 2
	}
	c, result, before, err := convertDiagnostics(filename, src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: convert: %s: %v\n", filename, err)
		return 1
	}
	plan := c.PlanConvert(result.CleanToOrig)
	for _, r := range plan.Reports {
		fmt.Fprintf(os.Stderr, "%s:%d:%d: gon convert: %s\n", filename, r.Line, r.Column, r.Message)
	}
	status := 0
	if len(plan.Reports) > 0 {
		status = 1
	}
	if len(plan.Edits) == 0 {
		return status
	}
	out := checker.ApplyConvert(src, plan.Edits)

	// The rewrite is accepted only if it changes nothing the checker says, and needs no
	// second pass. A rewrite never adds a line, so a diagnostic is compared by its code,
	// its severity and its line; a column moves when a conversion is inserted before it.
	c2, result2, after, err := convertDiagnostics(filename, out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: convert: the rewritten %s cannot be checked (%v); nothing was written\n", filename, err)
		return 1
	}
	if a, b := diagnosticSet(before), diagnosticSet(after); a != b {
		fmt.Fprintf(os.Stderr, "gon: convert: the rewritten %s has other diagnostics than the original; nothing was written\n  before: %s\n  after:  %s\n", filename, a, b)
		return 1
	}
	if again := c2.PlanConvert(result2.CleanToOrig); len(again.Edits) != 0 {
		fmt.Fprintf(os.Stderr, "gon: convert: the rewrite of %s is not stable (a second pass would change it again); nothing was written\n", filename)
		return 1
	}

	if !write {
		printConvertDiff(filename, src, out)
		return status
	}
	if err := writeAll([]string{filename}, map[string][]byte{filename: out}); err != nil {
		fmt.Fprintf(os.Stderr, "gon: write: %v; no file was changed\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "gon: convert: %s: %d variable(s) kept nilable\n", filename, plan.Kept)
	return status
}

// renameFile is os.Rename; a test replaces it to make a commit fail half way.
var renameFile = os.Rename

// writeAll replaces every file of paths with its new content, all or none. Each new
// content is first written to a temporary file next to its target, with the permissions
// of the target as the umask leaves them (0644 for a file that does not exist); only when all of them are staged
// are they renamed over the targets, and if a rename fails the targets already replaced
// are restored from their old content. The restore is best effort: a failure of the
// restore itself is reported, not hidden.
func writeAll(paths []string, data map[string][]byte) error {
	type staged struct{ path, tmp string }
	var done []staged
	cleanup := func() {
		for _, s := range done {
			os.Remove(s.tmp)
		}
	}
	for _, p := range paths {
		tmp := p + ".gon-convert"
		done = append(done, staged{p, tmp})
		var werr error
		if fi, err := os.Stat(p); err == nil {
			werr = os.WriteFile(tmp, data[p], fi.Mode().Perm())
		} else {
			werr = os.WriteFile(tmp, data[p], 0o644)
		}
		if werr != nil {
			cleanup()
			return werr
		}
	}
	var replaced []string
	old := make(map[string][]byte, len(paths))
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			old[p] = b
		}
	}
	for _, s := range done {
		if err := renameFile(s.tmp, s.path); err != nil {
			for _, r := range replaced {
				if b, ok := old[r]; ok {
					if rerr := os.WriteFile(r, b, 0o644); rerr != nil {
						fmt.Fprintf(os.Stderr, "gon: could not restore %s: %v\n", r, rerr)
					}
				} else {
					os.Remove(r)
				}
			}
			cleanup()
			return err
		}
		replaced = append(replaced, s.path)
	}
	return nil
}

// diagnosticSet is the sorted list of "code severity line message" of diagnostics (the column is left out: the rewrite moves it), as one string.
func diagnosticSet(diags []*checker.Diagnostic) string {
	var items []string
	for _, d := range diags {
		level := "error"
		if !d.IsError() {
			level = "warning"
		}
		items = append(items, fmt.Sprintf("%s %s %d %s", d.Code, level, d.Line, d.Message))
	}
	sort.Strings(items)
	return strings.Join(items, "; ")
}

// printConvertDiff prints, for each line that the rewrite changed, the old and the new
// text. A rewrite adds no line, so lines correspond.
func printConvertDiff(filename string, oldSrc, newSrc []byte) {
	o := strings.Split(string(oldSrc), "\n")
	n := strings.Split(string(newSrc), "\n")
	fmt.Printf("--- %s\n+++ %s\n", filename, filename)
	for i := range o {
		if i < len(n) && o[i] != n[i] {
			fmt.Printf("@@ line %d @@\n-%s\n+%s\n", i+1, o[i], n[i])
		}
	}
}
