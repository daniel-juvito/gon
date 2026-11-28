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
		fmt.Fprintf(os.Stderr, "gon: write: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "gon: write: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "gon: convert: %s: %d variable(s) kept nilable\n", filename, plan.Kept)
	return status
}

// renameFile and openFile are os.Rename and os.OpenFile; a test replaces them to make a
// commit or a staging fail half way.
var renameFile = os.Rename
var openFile = os.OpenFile

// writeError is the failure of writeAll. Incomplete lists what the restore could not put
// back, as "target (the old content is in backup)"; it is empty when every file is as it
// was before the call.
type writeError struct {
	cause      error
	incomplete []string
}

func (e *writeError) Error() string {
	if len(e.incomplete) == 0 {
		return fmt.Sprintf("%v; no file was changed", e.cause)
	}
	return fmt.Sprintf("%v; the restore is INCOMPLETE, check: %s", e.cause, strings.Join(e.incomplete, ", "))
}

// stageFile creates a new file next to path, under a name that did not exist (created
// with O_EXCL, so it is never a file of the user), holding data and the permissions of
// fi (0644 if it is nil) exactly (not as the umask leaves them). It returns the name.
func stageFile(path string, data []byte, fi os.FileInfo) (string, error) {
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("%s.gon-convert-%d", path, i)
		f, err := openFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", err
		}
		_, werr := f.Write(data)
		if werr == nil {
			if fi != nil {
				werr = f.Chmod(fi.Mode().Perm())
			} else {
				werr = f.Chmod(0o644)
			}
		}
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			os.Remove(name)
			return "", werr
		}
		return name, nil
	}
	return "", fmt.Errorf("no free staging name next to %s", path)
}

// writeAll replaces every file of paths with its new content, all or none, for the
// failures of an operation (not for the death of the process). Every new content, and a
// copy of every old one, is first written to a file of its own next to its target, with
// the permissions of the target. Only when all are staged are the new files renamed over
// the targets; a target that no longer holds the content that was read, or a rename that
// fails, undoes the renames already done by renaming the copies back. Only files that
// this call created are ever removed. Limits, stated: a symbolic link is refused (a
// rename would replace the link, not the file it points to); only content and mode are
// compared before a rename, so the owner, extended attributes and hard links of a file
// are not preserved; and a change made between the last comparison and the rename is not
// seen (no lock is taken). If a copy cannot be put back it is left where it
// is and named in the error.
func writeAll(paths []string, data map[string][]byte) error {
	type entry struct {
		path, tmp, backup string
		old               []byte
		existed           bool
		info              os.FileInfo
	}
	for _, p := range paths {
		if li, err := os.Lstat(p); err == nil && li.Mode()&os.ModeSymlink != 0 {
			return &writeError{cause: fmt.Errorf("%s is a symbolic link; convert the file it points to (a write would replace the link)", p)}
		}
	}
	var es []*entry
	var created []string
	cleanup := func(keep map[string]bool) {
		for _, n := range created {
			if !keep[n] {
				os.Remove(n)
			}
		}
	}
	for _, p := range paths {
		e := &entry{path: p}
		if fi, err := os.Stat(p); err == nil {
			e.existed = true
			e.info = fi
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				cleanup(nil)
				return &writeError{cause: rerr}
			}
			e.old = b
		}
		tmp, err := stageFile(p, data[p], e.info)
		if err != nil {
			cleanup(nil)
			return &writeError{cause: err}
		}
		e.tmp = tmp
		created = append(created, tmp)
		if e.existed {
			bk, err := stageFile(p, e.old, e.info)
			if err != nil {
				cleanup(nil)
				return &writeError{cause: err}
			}
			e.backup = bk
			created = append(created, bk)
		}
		es = append(es, e)
	}
	var replaced []*entry
	fail := func(cause error) error {
		var incomplete []string
		keep := map[string]bool{}
		for i := len(replaced) - 1; i >= 0; i-- {
			e := replaced[i]
			var rerr error
			if e.existed {
				rerr = renameFile(e.backup, e.path)
			} else {
				rerr = os.Remove(e.path)
			}
			if rerr != nil {
				incomplete = append(incomplete, fmt.Sprintf("%s (the old content is in %s): %v", e.path, e.backup, rerr))
				keep[e.backup] = true
			}
		}
		cleanup(keep)
		return &writeError{cause: cause, incomplete: incomplete}
	}
	for _, e := range es {
		if e.existed {
			now, err := os.ReadFile(e.path)
			st, serr := os.Stat(e.path)
			if err != nil || serr != nil || string(now) != string(e.old) || st.Mode() != e.info.Mode() {
				return fail(fmt.Errorf("%s changed while it was being converted", e.path))
			}
		} else if _, err := os.Stat(e.path); err == nil {
			return fail(fmt.Errorf("%s appeared while it was being converted", e.path))
		}
		if err := renameFile(e.tmp, e.path); err != nil {
			return fail(err)
		}
		replaced = append(replaced, e)
	}
	cleanup(nil)
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
