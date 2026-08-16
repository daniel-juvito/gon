package checker

import (
	"go/ast"
	"strings"

	"github.com/daniel-juvito/gon/internal/gna"
)

// NewWithAnnotations is like New but attaches a .gna annotation registry.
func NewWithAnnotations(filename string, cleanSrc []byte, nonNilOffsets map[int]bool, ann *gna.Registry) (*Checker, error) {
	c, err := New(filename, cleanSrc, nonNilOffsets)
	if err != nil {
		return nil, err
	}
	c.ann = ann
	c.imports = make(map[string]string)
	c.collectImports()
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

// resolveCallParams returns non-nil param flags and a display name for diagnostics.
// Local functions use in-file maps; package functions use .gna registry.
func (c *Checker) resolveCallParams(fun ast.Expr) ([]bool, string) {
	switch f := fun.(type) {
	case *ast.Ident:
		params, ok := c.funcParams[f.Name]
		if !ok {
			return nil, ""
		}
		return params, f.Name
	case *ast.SelectorExpr:
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
