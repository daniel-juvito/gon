package checker

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/types"
	"strings"

	"github.com/daniel-juvito/gon/internal/gna"
)

// NewWithAnnotations is like New but attaches a .gna annotation registry.
// It also runs go/types for method identity resolution. Type-check failure
// is non-fatal: local and package-function checks still run; method
// resolution is simply unavailable.
func NewWithAnnotations(filename string, cleanSrc []byte, nonNilOffsets map[int]bool, ann *gna.Registry) (*Checker, error) {
	c, err := New(filename, cleanSrc, nonNilOffsets)
	if err != nil {
		return nil, err
	}
	c.ann = ann
	c.imports = make(map[string]string)
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
		// Soft: collect errors but still return partial info when possible.
		Error: func(err error) {},
	}
	// Use the file's package name as the path; sufficient for identity lookup.
	pkgPath := c.file.Name.Name
	if _, err := conf.Check(pkgPath, c.fset, []*ast.File{c.file}, info); err != nil {
		// Even with Error handler, Check may return error. Keep info if Selections populated.
		if len(info.Selections) == 0 {
			return
		}
	}
	c.info = info
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
		// Prefer method identity from go/types when available.
		if params, display, ok := c.resolveMethodParams(f); ok {
			return params, display
		}
		// Package-qualified function: pkg.Func(...)
		pkgIdent, ok := f.X.(*ast.Ident)
		if !ok || c.ann == nil {
			return nil, ""
		}
		pkgPath, ok := c.imports[pkgIdent.Name]
		if !ok {
			return nil, ""
		}
		if !c.ann.HasPackage(pkgPath) {
			return nil, ""
		}
		sig, ok := c.ann.LookupFunc(pkgPath, f.Sel.Name)
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
	if c.info == nil || c.ann == nil {
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
	// Local package checked with path == package name (see typeCheck).
	if pkgPath == "" {
		pkgPath = c.file.Name.Name
	}

	if !c.ann.HasPackage(pkgPath) {
		// No .gna for this package → ordinary, no diagnostic.
		return nil, "", true // handled; do not fall through to package-func path
	}

	methodName := fn.Name()
	msig, found := c.ann.LookupMethod(pkgPath, typeName, methodName)
	if !found {
		c.addWarning(selExpr.Pos(), "GW002",
			fmt.Sprintf("no .gna annotation for %s.%s.%s; treating as ordinary", pkgPath, typeName, methodName))
		return nil, "", true
	}
	display = pkgPath + "." + typeName + "." + methodName
	return msig.Params, display, true
}
