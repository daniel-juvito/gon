package checker

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daniel-juvito/gon/internal/gna"
	"golang.org/x/tools/go/packages"
)

// Package-aware checking (v1.10 / M8, docs/rfc-package-check.md).
//
// A single-file check places the file in a synthetic one-file package, so a
// declaration in a sibling file is invisible to it. CheckPackage instead loads
// the REAL package with every .gon file overlaid at its real location, shares
// the declaration contracts across the files, and reports each diagnostic
// against the file it belongs to.

// PackageFile is one preprocessed .gon file of a package.
type PackageFile struct {
	Path          string       // path of the .gon file
	Clean         []byte       // source with the `!` modifiers removed
	NonNilOffsets map[int]bool // byte offsets in Clean where a `!` type began
}

// FileDiagnostics are the diagnostics of one file of a package, in the order
// the checker produced them (source order).
type FileDiagnostics struct {
	Path        string
	Diagnostics []*Diagnostic
}

// PackageResult is the outcome of checking one package.
type PackageResult struct {
	// Files is sorted by Path, so output does not depend on map or directory
	// iteration order.
	Files []FileDiagnostics
	// TypeErrors are the Go parse / type / package errors of the package. They
	// do not hide contract diagnostics: both are reported (RFC P6).
	TypeErrors []string
}

// packageShared is what the files of one package check have in common.
type packageShared struct {
	// offsets maps a file's (cleaned) overlay path to the byte offsets of its
	// `!` types: they are offsets into one file's clean source, so they cannot
	// be one table for the package.
	offsets map[string]map[int]bool
	// files holds every syntax tree of the package, for cross-file lookups.
	files []*ast.File
	// The declaration tables, filled by every file's collect() before any file
	// is walked.
	funcParams   map[string][]bool
	funcReturns  map[string][]bool
	structFields map[string]map[string]bool
}

// CheckPackage checks the .gon files of one directory as a package.
//
// Each file's clean source is overlaid at <dir>/<base>.go: it replaces a
// same-named .go file (a `.gon` shadows the `.go` that `gon transpile` emitted
// next to it) or is added to the package when there is none. The package's other
// .go files are loaded as they are. Test files are not part of the check.
//
// A non-nil error means the package could not be loaded or set up at all (the
// go command failed, the module is misconfigured, a .gon file is excluded by
// build constraints, the loader returned no type information): no
// PackageResult exists, and `gon` exits 2 for it. Go parse, type and package
// errors that the loader reports are NOT errors here: they are in
// PackageResult.TypeErrors, next to the contract diagnostics.
func CheckPackage(dir string, files []PackageFile, resolver gna.Resolver) (*PackageResult, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no .gon files to check")
	}
	sorted := append([]PackageFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	root := findModuleRoot(dirAbs)
	if root == "" {
		root = dirAbs
	}

	overlayPath := func(f PackageFile) string {
		base := strings.TrimSuffix(filepath.Base(f.Path), ".gon")
		return filepath.Clean(filepath.Join(dirAbs, base+".go"))
	}
	overlay := make(map[string][]byte, len(sorted))
	for _, f := range sorted {
		overlay[overlayPath(f)] = f.Clean
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax | packages.NeedImports | packages.NeedDeps,
		Fset:    fset,
		Dir:     root,
		Overlay: overlay,
		Tests:   false,
	}
	pkgs, err := packages.Load(cfg, "file="+overlayPath(sorted[0]))
	if err != nil {
		return nil, fmt.Errorf("loading package %s: %w", dir, err)
	}
	if len(pkgs) == 0 || pkgs[0].TypesInfo == nil || pkgs[0].Types == nil {
		// The loader left every file out (the query names the first file; when
		// that one is excluded by build constraints the answer is empty).
		if err := excludedError(dirAbs, overlay, overlayPath, sorted); err != nil {
			return nil, fmt.Errorf("loading package %s: %w", dir, err)
		}
		return nil, fmt.Errorf("loading package %s: no type information", dir)
	}
	pkg := pkgs[0]

	byPath := make(map[string]*ast.File, len(pkg.Syntax))
	shared := &packageShared{
		offsets:      make(map[string]map[int]bool, len(sorted)),
		funcParams:   make(map[string][]bool),
		funcReturns:  make(map[string][]bool),
		structFields: make(map[string]map[string]bool),
	}
	for _, syn := range pkg.Syntax {
		name := fset.Position(syn.Pos()).Filename
		if name == "" {
			// The loader's placeholder for a file it could not parse at all
			// (e.g. an invalid package clause); that file's parse error is in
			// pkg.Errors.
			continue
		}
		byPath[filepath.Clean(name)] = syn
		shared.files = append(shared.files, syn)
	}

	// A file the loader did not return a syntax tree for is either unparsable
	// (its own parse error is in pkg.Errors and reaches TypeErrors below; the
	// other files are still checked) or was not loaded at all, which is a load
	// failure.
	var missing []PackageFile
	checked := make([]PackageFile, 0, len(sorted))
	for _, f := range sorted {
		op := overlayPath(f)
		if byPath[op] != nil {
			checked = append(checked, f)
			continue
		}
		if !hasOwnParseError(pkg.Errors, op) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		if err := excludedError(dirAbs, overlay, overlayPath, missing); err != nil {
			return nil, fmt.Errorf("loading package %s: %w", dir, err)
		}
		paths := make([]string, len(missing))
		for i, f := range missing {
			paths[i] = f.Path
		}
		return nil, fmt.Errorf("loading package %s: not part of the loaded package: %s", dir, strings.Join(paths, ", "))
	}

	checkers := make([]*Checker, 0, len(checked))
	for _, f := range checked {
		op := overlayPath(f)
		syn := byPath[op]
		shared.offsets[op] = f.NonNilOffsets

		c := &Checker{
			fset:          fset,
			file:          syn,
			filename:      f.Path,
			cleanSrc:      f.Clean,
			nonNilOffsets: f.NonNilOffsets,
			funcParams:    shared.funcParams,
			funcReturns:   shared.funcReturns,
			structFields:  shared.structFields,
			resolver:      resolver,
			info:          pkg.TypesInfo,
			resolved:      make(map[string]*gna.File),
			resolvedMiss:  make(map[string]bool),
			importPkgs:    make(map[string]*types.Package),
			importPos:     make(map[string]token.Pos),
			pkg:           shared,
		}
		c.collectImports()
		// Only the imports THIS file declares are validated against `.gna`
		// (M5a), so a package imported by a sibling is not reported twice.
		for _, imp := range pkg.Types.Imports() {
			if _, ok := c.importPos[imp.Path()]; ok {
				c.importPkgs[imp.Path()] = imp
			}
		}
		checkers = append(checkers, c)
	}

	// Phase 1: every file's declarations, so a contract written in one file is
	// known to all. Phase 2: walk each file.
	for _, c := range checkers {
		c.collect()
	}
	res := &PackageResult{}
	for i, c := range checkers {
		res.Files = append(res.Files, FileDiagnostics{Path: checked[i].Path, Diagnostics: c.walk()})
	}

	for _, e := range pkg.Errors {
		switch e.Kind {
		case packages.TypeError, packages.ParseError, packages.ListError:
			res.TypeErrors = append(res.TypeErrors, e.Error())
		}
	}
	return res, nil
}

// excludedError explains why the loader left files out, when that can be shown:
// the candidates that the build context rejects (file name suffix or
// `//go:build` line, read from the overlaid clean source) are reported as
// excluded by build constraints, sorted, in one message whatever their names.
// It is only a diagnosis of a load that already failed; the loader decides what
// the package is. go/build's context can differ from the go command's (GOFLAGS
// tags, toolchain version), so a candidate it accepts is never called excluded,
// and nil is returned when no candidate is rejected: the caller then reports a
// generic load failure.
func excludedError(dirAbs string, overlay map[string][]byte, overlayPath func(PackageFile) string, candidates []PackageFile) error {
	ctx := build.Default
	ctx.OpenFile = func(path string) (io.ReadCloser, error) {
		if src, ok := overlay[filepath.Clean(path)]; ok {
			return io.NopCloser(bytes.NewReader(src)), nil
		}
		return os.Open(path)
	}
	var excluded []string
	for _, f := range candidates {
		match, err := ctx.MatchFile(dirAbs, filepath.Base(overlayPath(f)))
		if err == nil && !match {
			excluded = append(excluded, f.Path)
		}
	}
	if len(excluded) == 0 {
		return nil
	}
	sort.Strings(excluded)
	return fmt.Errorf("excluded by build constraints: %s (package mode checks every .gon file in the directory)", strings.Join(excluded, ", "))
}

// hasOwnParseError reports whether errs holds a parse error positioned in the
// file at overlay path op. Only such an error makes a missing syntax tree a
// reportable source error; a parse error of another file does not.
func hasOwnParseError(errs []packages.Error, op string) bool {
	for _, e := range errs {
		if e.Kind != packages.ParseError {
			continue
		}
		if samePath(errorFile(e.Pos), op) {
			return true
		}
	}
	return false
}

// errorFile is the file part of a packages.Error position ("file:line:col",
// "file:line", "file" or "-").
func errorFile(pos string) string {
	for i := 0; i < 2; i++ {
		j := strings.LastIndex(pos, ":")
		if j < 0 {
			break
		}
		if _, err := fmt.Sscanf(pos[j+1:], "%d", new(int)); err != nil {
			break
		}
		pos = pos[:j]
	}
	return pos
}

// samePath compares two file paths after cleaning them and, when they still
// differ, after resolving symbolic links in their directories (the file itself
// may exist only in the overlay).
func samePath(a, b string) bool {
	if a == "" || a == "-" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	if !filepath.IsAbs(a) || !filepath.IsAbs(b) || filepath.Base(a) != filepath.Base(b) {
		return false
	}
	da, errA := filepath.EvalSymlinks(filepath.Dir(a))
	db, errB := filepath.EvalSymlinks(filepath.Dir(b))
	return errA == nil && errB == nil && da == db
}
