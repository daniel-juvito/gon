package checker

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniel-juvito/gon/internal/gna"
	"golang.org/x/tools/go/packages"
)

// NewWithAnnotations is like New but attaches a .gna annotation resolver.
// Type-check failure is non-fatal: local and package-function checks still
// run; method resolution is simply unavailable when types info is missing.
//
// resolver may be nil (no external annotations), a *gna.Registry, a
// *gna.Chain, or any other gna.Resolver.
func NewWithAnnotations(filename string, cleanSrc []byte, nonNilOffsets map[int]bool, resolver gna.Resolver) (*Checker, error) {
	c, err := New(filename, cleanSrc, nonNilOffsets)
	if err != nil {
		return nil, err
	}
	c.resolver = resolver
	c.imports = make(map[string]string)
	c.resolved = make(map[string]*gna.File)
	c.resolvedMiss = make(map[string]bool)
	c.typeCheck()
	// Imports from the (possibly replaced) AST.
	c.collectImports()
	return c, nil
}

func (c *Checker) collectImports() {
	c.imports = make(map[string]string)
	if c.file == nil {
		return
	}
	for _, imp := range c.file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
			if name == "_" || name == "." {
				continue
			}
		} else {
			if i := strings.LastIndex(path, "/"); i >= 0 {
				name = path[i+1:]
			} else {
				name = path
			}
		}
		c.imports[name] = path
	}
}

// typeCheck populates c.info. Prefers go/packages (module-aware) so external
// imports resolve; falls back to go/types.Check with importer.Default().
func (c *Checker) typeCheck() {
	if c.typeCheckPackages() {
		return
	}
	c.typeCheckFallback()
}

// typeCheckPackages uses go/packages with an Overlay so in-memory clean
// source is type-checked against the real module graph. On success it
// replaces c.file with the package AST so Selections keys match.
func (c *Checker) typeCheckPackages() bool {
	if len(c.cleanSrc) == 0 {
		return false
	}
	start := "."
	if c.filename != "" {
		if abs, err := filepath.Abs(c.filename); err == nil {
			start = filepath.Dir(abs)
		}
	}
	if wd, err := os.Getwd(); err == nil && start == "." {
		start = wd
	}
	root := findModuleRoot(start)
	if root == "" {
		root = start
	}

	// Place overlay under a synthetic package dir inside the module so
	// go/packages treats it as a loadable package (root-level file=
	// patterns are unreliable across toolchains).
	overlayPath := filepath.Join(root, "__gon_overlay__", "check.go")

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax |
			packages.NeedImports,
		Fset: c.fset,
		Dir:  root,
		Overlay: map[string][]byte{
			overlayPath: c.cleanSrc,
		},
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "file="+overlayPath)
	if err != nil || len(pkgs) == 0 {
		return false
	}
	pkg := pkgs[0]
	if pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
		return false
	}
	var file *ast.File
	for _, syn := range pkg.Syntax {
		pos := c.fset.Position(syn.Pos())
		if filepath.Clean(pos.Filename) == filepath.Clean(overlayPath) {
			file = syn
			break
		}
	}
	if file == nil {
		file = pkg.Syntax[0]
	}
	c.file = file
	c.info = pkg.TypesInfo
	return true
}

func (c *Checker) typeCheckFallback() {
	info := &types.Info{
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Uses:       make(map[*ast.Ident]types.Object),
		Types:      make(map[ast.Expr]types.TypeAndValue),
	}
	conf := types.Config{
		Importer: importer.Default(),
		Error:    func(err error) {},
	}
	if len(c.cleanSrc) > 0 {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, c.filename, c.cleanSrc, parser.AllErrors)
		if err == nil {
			c.fset = fset
			c.file = f
		}
	}
	if c.file == nil {
		return
	}
	pkgPath := "main"
	if c.file.Name != nil {
		pkgPath = c.file.Name.Name
	}
	if _, err := conf.Check(pkgPath, c.fset, []*ast.File{c.file}, info); err != nil {
		if len(info.Selections) == 0 {
			return
		}
	}
	c.info = info
}

func findModuleRoot(start string) string {
	dir := start
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return ""
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// resolvePackage asks the resolver for importPath, caching results.
// Returns (nil, nil) when unannotated. Returns error on malformed .gna.
func (c *Checker) resolvePackage(importPath string) (*gna.File, error) {
	if c.resolver == nil || importPath == "" {
		return nil, nil
	}
	if c.resolved == nil {
		c.resolved = make(map[string]*gna.File)
	}
	if c.resolvedMiss == nil {
		c.resolvedMiss = make(map[string]bool)
	}
	if f, ok := c.resolved[importPath]; ok {
		return f, nil
	}
	if c.resolvedMiss[importPath] {
		return nil, nil
	}
	f, err := c.resolver.Resolve(importPath)
	if err != nil {
		return nil, err
	}
	if f == nil {
		c.resolvedMiss[importPath] = true
		return nil, nil
	}
	c.resolved[importPath] = f
	return f, nil
}

// resolveCallParams returns non-nil param flags and a display name for diagnostics.
// Order: method selection (go/types) → package-level function (.gna) → local function.
func (c *Checker) resolveCallParams(fun ast.Expr) ([]bool, string) {
	switch f := fun.(type) {
	case *ast.Ident:
		params, ok := c.funcParams[f.Name]
		if !ok {
			return nil, ""
		}
		return params, f.Name

	case *ast.SelectorExpr:
		if params, display, ok := c.resolveMethodParams(f); ok {
			return params, display
		}
		pkgIdent, ok := f.X.(*ast.Ident)
		if !ok || c.resolver == nil {
			return nil, ""
		}
		pkgPath, ok := c.imports[pkgIdent.Name]
		if !ok {
			return nil, ""
		}
		file, err := c.resolvePackage(pkgPath)
		if err != nil {
			c.addError(f.Pos(), "GN003", "malformed .gna for "+pkgPath+": "+err.Error())
			return nil, ""
		}
		if file == nil {
			return nil, ""
		}
		sig, ok := file.Functions[f.Sel.Name]
		if !ok {
			c.addWarning(f.Pos(), "GW002", "no .gna annotation for "+pkgPath+"."+f.Sel.Name+"; treating as ordinary")
			return nil, ""
		}
		return sig.Params, pkgPath + "." + f.Sel.Name

	default:
		return nil, ""
	}
}

// resolveMethodParams uses go/types selection info to map a call to
// TypeName.MethodName in the .gna registry. ok=false means "not a method
// we can resolve" (caller may try package-function path).
//
// go/types is used only for identity. Nilability still comes solely from .gna.
func (c *Checker) resolveMethodParams(selExpr *ast.SelectorExpr) (params []bool, display string, ok bool) {
	if c.info == nil || c.resolver == nil {
		return nil, "", false
	}
	sel, found := c.info.Selections[selExpr]
	if !found || (sel.Kind() != types.MethodVal && sel.Kind() != types.MethodExpr) {
		return nil, "", false
	}
	fn, isFunc := sel.Obj().(*types.Func)
	if !isFunc {
		return nil, "", false
	}
	sig, isSig := fn.Type().(*types.Signature)
	if !isSig || sig.Recv() == nil {
		return nil, "", false
	}

	recvType := sig.Recv().Type()
	if p, isPtr := recvType.(*types.Pointer); isPtr {
		recvType = p.Elem()
	}
	named, isNamed := recvType.(*types.Named)
	if !isNamed {
		return nil, "", false
	}

	typeName := named.Obj().Name()
	pkgPath := ""
	pkgName := ""
	if pkg := named.Obj().Pkg(); pkg != nil {
		pkgPath = pkg.Path()
		pkgName = pkg.Name()
	}
	if pkgPath == "" {
		pkgPath = c.file.Name.Name
		pkgName = pkgPath
	}

	// Resolve by import path first; fall back to package name so local tests
	// that annotate package: main still match when go/packages loads the
	// overlay as <module>/__gon_overlay__.
	file, err := c.resolvePackage(pkgPath)
	if err != nil {
		c.addError(selExpr.Pos(), "GN003", "malformed .gna for "+pkgPath+": "+err.Error())
		return nil, "", true
	}
	resolvedAs := pkgPath
	if file == nil && pkgName != "" && pkgName != pkgPath {
		file, err = c.resolvePackage(pkgName)
		if err != nil {
			c.addError(selExpr.Pos(), "GN003", "malformed .gna for "+pkgName+": "+err.Error())
			return nil, "", true
		}
		if file != nil {
			resolvedAs = pkgName
		}
	}
	if file == nil {
		return nil, "", true
	}

	methodName := fn.Name()
	msig, found := file.Methods[typeName+"."+methodName]
	if !found {
		c.addWarning(selExpr.Pos(), "GW002",
			fmt.Sprintf("no .gna annotation for %s.%s.%s; treating as ordinary", resolvedAs, typeName, methodName))
		return nil, "", true
	}
	display = resolvedAs + "." + typeName + "." + methodName
	return msig.Params, display, true
}
