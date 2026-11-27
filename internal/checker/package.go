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
	// CleanToOrig, when set, asks for the plan of gon convert (ConvertPlan) of this
	// file, computed after the file is walked: it maps offsets of Clean to offsets of
	// the original source, as preproc.Result.CleanToOrig does.
	CleanToOrig []int
}

// FileDiagnostics are the diagnostics of one file of a package, in the order
// the checker produced them (source order).
type FileDiagnostics struct {
	Path        string
	Diagnostics []*Diagnostic
	// Plan is the plan of gon convert for the file, set only when the PackageFile
	// carried CleanToOrig.
	Plan *ConvertPlan
}

// PackageResult is the outcome of checking one package.
type PackageResult struct {
	// Files is sorted by Path, so output does not depend on map or directory
	// iteration order.
	Files []FileDiagnostics
	// TypeErrors are the Go parse / type / package errors of the package. They
	// do not hide contract diagnostics: both are reported (RFC P6).
	TypeErrors []string
	// InvalidFiles are the .gon files that were not checked because their
	// //go:build line is malformed (v1.19), in file order. Like TypeErrors they
	// are source errors of the package that are not Diagnostics in Protocol
	// v1: no code, no range (go/build gives no position), so they never reach
	// the JSON envelope; the command reports them on stderr and exits 1.
	InvalidFiles []InvalidFile
}

// InvalidFile is a .gon file whose //go:build line go/build cannot evaluate.
type InvalidFile struct {
	Path   string // the .gon path, as the diagnostics of the package name it
	Reason string // go/build's reason, without the overlay file name
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
// Inside a module (v1.16) the package is loaded by its directory, so the go
// command decides which files belong to it for the current build
// configuration, as `go list ./dir` does: a .gon whose overlaid file the loader
// lists in IgnoredFiles (build constraints, file name suffix, GOFLAGS tags) is
// not part of the package and is not checked. Only the files that are checked
// appear in PackageResult.Files.
//
// Outside a module (v1.18) there is no go command package for a directory, so
// go/build decides membership with the toolchain's default build context
// (GOOS, GOARCH, release tags; GOFLAGS tags are not read): every .gon whose
// overlaid source it accepts, and every plain non-test .go file of the
// directory it accepts that no .gon shadows, is named to the loader, and they
// form one ad-hoc package. Import resolution is unchanged by this: the files
// are loaded as before, only all of them now.
//
// A non-nil error means the package could not be loaded or set up at all (the
// go command failed, the module is misconfigured, every source of the package
// is excluded by build constraints (*ExcludedError), the loader returned no
// type information): no PackageResult exists, and `gon` exits 2 for it, or
// skips an *ExcludedError package found by a `...` pattern. Go parse, type and
// package errors that the loader reports are NOT errors here: they are in
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
	inModule := root != ""
	if !inModule {
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
			packages.NeedSyntax | packages.NeedImports | packages.NeedDeps |
			packages.NeedFiles,
		Fset:    fset,
		Dir:     root,
		Overlay: overlay,
		Tests:   false,
	}
	queries := []string{dirAbs}
	var invalid []string           // outside a module: plain .go files whose build constraint is malformed
	var invalidFiles []InvalidFile // .gon files whose build constraint is malformed
	if !inModule {
		kept, excluded, plain, badGon, badPlain := outsideModuleMembers(dir, dirAbs, sorted, overlay, overlayPath)
		invalid = badPlain
		invalidFiles = badGon
		if len(kept) == 0 && len(plain) == 0 {
			if len(invalid) > 0 || len(invalidFiles) > 0 {
				// The only sources left have a malformed constraint: a source
				// error of the package (as the go command reports it), not a
				// package with no source in the build. Nothing can be loaded.
				return &PackageResult{TypeErrors: invalid, InvalidFiles: invalidFiles}, nil
			}
			// No source of the package is in the build: as in a module.
			return nil, &ExcludedError{Dir: dir, Files: excluded}
		}
		sorted = kept
		queries = queries[:0]
		for _, f := range kept {
			queries = append(queries, overlayPath(f))
		}
		queries = append(queries, plain...)
	}
	pkgs, err := packages.Load(cfg, queries...)
	if err != nil {
		return nil, fmt.Errorf("loading package %s: %w", dir, err)
	}
	var loaderIgnored []string // in a module: .gon files the go command lists in IgnoredFiles
	if inModule && len(pkgs) > 0 {
		// Keep only the .gon files the go command put in the package.
		kept, excluded := splitIgnored(sorted, overlayPath, pkgs[0].IgnoredFiles)
		if len(kept) == 0 && len(pkgs[0].GoFiles) == 0 && len(pkgs[0].CompiledGoFiles) == 0 {
			// Every .gon is ignored and no other Go file is in the build: the
			// package has no source for this configuration.
			return nil, &ExcludedError{Dir: dir, Files: excluded}
		}
		loaderIgnored = excluded
		sorted = kept
	}
	if len(pkgs) == 0 || pkgs[0].TypesInfo == nil || pkgs[0].Types == nil {
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
	//
	// In a module, a .gon the loader dropped without any of that (it is in
	// neither GoFiles nor IgnoredFiles, and has no parse error of its own) is
	// explained by go/build, asked about this file only and on the same overlaid
	// source (classifyDropped): a malformed //go:build line is a source error of
	// the package (InvalidFile: the file is not checked, the others are), a file
	// go/build does not accept without any error is one the go tool ignores by
	// name (`_x`, `.x`: not part of the package, no error), and a file it
	// accepts is unexplained: the generic load failure below, exit 2. go/build's
	// verdict is never applied to a file the loader did load (a tag in GOFLAGS
	// can include a file go/build alone would reject).
	var missing []PackageFile
	var ignoredByName []string
	restated := make(map[string]bool) // the loader's own text for each InvalidFile
	checked := make([]PackageFile, 0, len(sorted))
	for _, f := range sorted {
		op := overlayPath(f)
		if byPath[op] != nil {
			checked = append(checked, f)
			continue
		}
		if hasOwnParseError(pkg.Errors, op) {
			continue
		}
		if inModule {
			d := classifyDropped(dirAbs, overlay, op)
			switch d.kind() {
			case dropInvalid:
				invalidFiles = append(invalidFiles, InvalidFile{Path: f.Path, Reason: d.reason})
				restated[d.text] = true
				continue
			case dropIgnored:
				ignoredByName = append(ignoredByName, f.Path)
				continue
			}
		}
		missing = append(missing, f)
	}
	if len(checked) == 0 && len(invalidFiles) == 0 && len(missing) == 0 && len(pkg.GoFiles) == 0 {
		// Every .gon is out of the package (ignored by the go command or by
		// its name) and no other Go file is in the build.
		files := append(append([]string(nil), loaderIgnored...), ignoredByName...)
		sort.Strings(files)
		return nil, &ExcludedError{Dir: dir, Files: files}
	}
	if len(missing) > 0 {
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
			fset:          (*token.FileSet)(fset),
			file:          (*ast.File)(syn), // every checked file is in byPath: `missing` is empty here
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
	res := &PackageResult{TypeErrors: invalid, InvalidFiles: invalidFiles}
	for i, c := range checkers {
		fd := FileDiagnostics{Path: checked[i].Path, Diagnostics: c.walk()}
		if checked[i].CleanToOrig != nil {
			plan := c.PlanConvert(checked[i].CleanToOrig)
			fd.Plan = &plan
		}
		res.Files = append(res.Files, fd)
	}

	for _, e := range pkg.Errors {
		switch e.Kind {
		case packages.TypeError, packages.ParseError, packages.ListError:
			if restatesInvalidFile(e, restated) {
				continue
			}
			res.TypeErrors = append(res.TypeErrors, e.Error())
		}
	}
	return res, nil
}

// ExcludedError reports a package none of whose sources is in the build: the go
// command lists every .gon of the directory in IgnoredFiles and the package has
// no other Go file (what `go list ./dir` reports as "build constraints exclude
// all Go files"). It is recognised from the loader's file lists, not from its
// error text. Files are the .gon paths, sorted.
type ExcludedError struct {
	Dir   string
	Files []string
}

func (e *ExcludedError) Error() string {
	return fmt.Sprintf("build constraints exclude all Go files in %s (.gon files: %s)", e.Dir, strings.Join(e.Files, ", "))
}

// splitIgnored separates the files (sorted by path) whose overlay path the
// loader lists as ignored from the others. Paths are compared with samePath,
// so a directory reached through a symbolic link still matches.
func splitIgnored(files []PackageFile, overlayPath func(PackageFile) string, ignored []string) (kept []PackageFile, excluded []string) {
	for _, f := range files {
		op := overlayPath(f)
		isIgnored := false
		for _, ig := range ignored {
			if samePath(ig, op) {
				isIgnored = true
				break
			}
		}
		if isIgnored {
			excluded = append(excluded, f.Path)
		} else {
			kept = append(kept, f)
		}
	}
	return kept, excluded
}

// outsideModuleMembers decides, outside a module, which files form the
// package: go/build's default context judges each .gon by its overlaid clean
// source (file name suffix, `//go:build` line) and each plain .go file of the
// directory. kept are the .gon files it accepts, in their order; excluded the
// paths of those it rejects; plain the absolute paths of the accepted plain .go
// files that are not test files, not ignored by name (`_x`, `.x`) and not
// shadowed by a .gon of the same base name. A file MatchFile cannot judge (a
// malformed `//go:build` line, which the go command reports as an invalid file
// of the package) is neither kept nor excluded: it is not loaded and is
// reported as a source error of the package, a .gon as an InvalidFile (badGon)
// and a plain .go file as a Go error text (badPlain), named by dir (the
// operand) joined with its name.
func outsideModuleMembers(dir, dirAbs string, files []PackageFile, overlay map[string][]byte, overlayPath func(PackageFile) string) (kept []PackageFile, excluded []string, plain []string, badGon []InvalidFile, badPlain []string) {
	ctx := overlayContext(overlay)
	for _, f := range files {
		name := filepath.Base(overlayPath(f))
		match, err := ctx.MatchFile(dirAbs, name)
		switch {
		case err != nil:
			badGon = append(badGon, InvalidFile{Path: f.Path, Reason: reasonOf(name, err)})
		case match:
			kept = append(kept, f)
		default:
			excluded = append(excluded, f.Path)
		}
	}
	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		return kept, excluded, nil, badGon, badPlain
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dirAbs, name)
		if _, shadowed := overlay[filepath.Clean(path)]; shadowed {
			continue
		}
		match, err := ctx.MatchFile(dirAbs, name)
		switch {
		case err != nil:
			badPlain = append(badPlain, invalidConstraint(filepath.Join(dir, name), err))
		case match:
			plain = append(plain, path)
		}
	}
	return kept, excluded, plain, badGon, badPlain
}

// invalidConstraint is the Go error text reported for a plain .go file whose
// build constraint go/build cannot evaluate, named by the file the user wrote.
func invalidConstraint(path string, err error) string {
	return fmt.Sprintf("%s: invalid build constraint: %v", filepath.ToSlash(path), err)
}

// overlayContext is go/build's default context reading the overlaid sources
// instead of the files on disk.
func overlayContext(overlay map[string][]byte) *build.Context {
	ctx := build.Default
	ctx.OpenFile = func(path string) (io.ReadCloser, error) {
		if src, ok := overlay[filepath.Clean(path)]; ok {
			return io.NopCloser(bytes.NewReader(src)), nil
		}
		return os.Open(path)
	}
	return &ctx
}

// reasonOf is go/build's reason for err without the file name it prefixes
// ("<name>: parsing //go:build line: ..."): the name is the one that was passed
// to MatchFile. When the text does not start with it, the whole text is kept.
func reasonOf(name string, err error) string {
	text := err.Error()
	if reason, ok := strings.CutPrefix(text, name+": "); ok {
		return reason
	}
	return text
}

// droppedFile is go/build's verdict on a .gon the loader dropped from a
// package without a parse error of its own.
type droppedFile struct {
	invalid bool   // go/build cannot evaluate the build constraint
	reason  string // go/build's reason without the file name (invalid only)
	text    string // go/build's whole error text (invalid only)
	match   bool   // go/build accepts the file: nothing explains the drop
}

// dropKind is what CheckPackage does with a .gon the loader dropped.
type dropKind int

const (
	// dropUnexplained: go/build accepts the file, so nothing shows why the
	// loader left it out. It stays the generic load failure (exit 2).
	dropUnexplained dropKind = iota
	// dropInvalid: the //go:build line is malformed. A source error of the
	// package: the file is not checked, the others are (exit 1).
	dropInvalid
	// dropIgnored: go/build rejects the file without an error, which for a file
	// the loader listed nowhere is a name the go tool ignores (`_x`, `.x`). Not
	// part of the package, not an error.
	dropIgnored
)

// kind maps go/build's verdict to what is done with the file. (false, nil) is
// only read as "ignored" here, for a file already known to be dropped by the
// loader; it is not a rule about files the loader loaded.
func (d droppedFile) kind() dropKind {
	switch {
	case d.invalid:
		return dropInvalid
	case !d.match:
		return dropIgnored
	}
	return dropUnexplained
}

// classifyDropped asks go/build, on the overlaid source, about the .gon at
// overlay path op. Three outcomes: an error (a malformed //go:build line:
// invalid), no error and no match (a name or rule the go tool ignores), and a
// match (no cause is shown). It is only meaningful for a file the loader
// dropped: the result is never applied to a file the loader loaded or listed
// as ignored.
func classifyDropped(dirAbs string, overlay map[string][]byte, op string) droppedFile {
	name := filepath.Base(op)
	match, err := overlayContext(overlay).MatchFile(dirAbs, name)
	if err != nil {
		return droppedFile{invalid: true, reason: reasonOf(name, err), text: err.Error()}
	}
	return droppedFile{match: match}
}

// restatesInvalidFile reports whether e is the loader's own error for an
// InvalidFile: a list error with no position whose text is, character for
// character, go/build's error for that file. Anything else is kept, so a
// difference between the go command's wording and go/build's costs a repeated
// line, never a lost error.
func restatesInvalidFile(e packages.Error, restated map[string]bool) bool {
	return e.Kind == packages.ListError && e.Pos == "" && restated[e.Msg]
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
