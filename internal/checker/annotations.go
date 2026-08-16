package checker

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/types"
	"strings"

	"github.com/daniel-juvito/gon/internal/gna"
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
	c.collectImports()
	c.typeCheck()
	return c, nil
}

func (c *Checker) collectImports() {
	if c.imports == nil {
		c.imports = make(map[string]string)
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

// typeCheck populates c.info using go/types. Failures leave c.info nil.
func (c *Checker) typeCheck() {
	info := &types.Info{
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Uses:       make(map[*ast.Ident]types.Object),
		Types:      make(map[ast.Expr]types.TypeAndValue),
	}
	conf := types.Config{
		Importer: importer.Default(),
		Error:    func(err error) {},
	}
	pkgPath := c.file.Name.Name
	if _, err := conf.Check(pkgPath, c.fset, []*ast.File{c.file}, info); err != nil {
		if len(info.Selections) == 0 {
			return
		}
	}
	c.info = info
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
		// Package-qualified function: pkg.Func(...)
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
	if pkg := named.Obj().Pkg(); pkg != nil {
		pkgPath = pkg.Path()
	}
	if pkgPath == "" {
		pkgPath = c.file.Name.Name
	}

	file, err := c.resolvePackage(pkgPath)
	if err != nil {
		c.addError(selExpr.Pos(), "GN003", "malformed .gna for "+pkgPath+": "+err.Error())
		return nil, "", true
	}
	if file == nil {
		// No .gna for this package → ordinary, no diagnostic.
		return nil, "", true
	}

	methodName := fn.Name()
	msig, found := file.Methods[typeName+"."+methodName]
	if !found {
		c.addWarning(selExpr.Pos(), "GW002",
			fmt.Sprintf("no .gna annotation for %s.%s.%s; treating as ordinary", pkgPath, typeName, methodName))
		return nil, "", true
	}
	display = pkgPath + "." + typeName + "." + methodName
	return msig.Params, display, true
}
