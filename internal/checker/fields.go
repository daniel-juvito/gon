package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkCompositeLitFields is retained as a v1.2 compatibility alias.
// All composite-literal construction analysis goes through M2a.
func (c *Checker) checkCompositeLitFields(lit *ast.CompositeLit) {
	c.checkCompositeLitConstruction(lit)
}

func (c *Checker) reportMissingFromStructAST(pos token.Pos, st *ast.StructType, provided map[string]bool, pathPrefix string) {
	if st == nil || st.Fields == nil {
		return
	}
	for _, field := range st.Fields.List {
		isNN := c.isNonNil(field.Type)
		if len(field.Names) == 0 {
			switch ft := field.Type.(type) {
			case *ast.Ident:
				name := ft.Name
				p := pathPrefix
				if p != "" {
					p = p + "." + name
				} else {
					p = name
				}
				if provided == nil || !provided[name] {
					c.reportMissingNonNilFields(pos, ft, nil, p)
				}
			case *ast.StructType:
				c.reportMissingFromStructAST(pos, ft, nil, pathPrefix)
			case *ast.StarExpr:
			}
			continue
		}
		for _, name := range field.Names {
			p := name.Name
			if pathPrefix != "" {
				p = pathPrefix + "." + name.Name
			}
			if isNN && (provided == nil || !provided[name.Name]) {
				c.addError(pos, "GN002", fmt.Sprintf("missing required non-nil field %s", p))
			}
			if provided == nil || !provided[name.Name] {
				c.reportMissingNonNilFields(pos, field.Type, nil, p)
			}
		}
	}
}

func (c *Checker) reportMissingNonNilFields(pos token.Pos, typExpr ast.Expr, provided map[string]bool, pathPrefix string) {
	if typExpr == nil {
		return
	}
	switch t := typExpr.(type) {
	case *ast.Ident:
		fields, ok := c.structFields[t.Name]
		st := c.lookupStructAST(t.Name)
		// Prefer declaration-order AST walk when available (same source as
		// collectGenDecl). Map-only iteration is non-deterministic and is
		// not used as a fallback under v1.3 local-file invariants.
		if ok && st != nil {
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					nn := fields[name.Name]
					p := name.Name
					if pathPrefix != "" {
						p = pathPrefix + "." + name.Name
					}
					if nn && (provided == nil || !provided[name.Name]) {
						c.addError(pos, "GN002", fmt.Sprintf("missing required non-nil field %s", p))
					}
				}
			}
		}
		if st != nil {
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					if id, ok := field.Type.(*ast.Ident); ok {
						name := id.Name
						if provided != nil && provided[name] {
							continue
						}
						p := pathPrefix
						if p != "" {
							p = p + "." + name
						} else {
							p = name
						}
						c.reportMissingNonNilFields(pos, field.Type, nil, p)
					}
					continue
				}
				for _, name := range field.Names {
					if provided != nil && provided[name.Name] {
						continue
					}
					p := pathPrefix
					if p != "" {
						p = p + "." + name.Name
					} else {
						p = name.Name
					}
					if fields != nil && fields[name.Name] {
						continue
					}
					c.reportMissingNonNilFields(pos, field.Type, nil, p)
				}
			}
		}
	case *ast.StructType:
		c.reportMissingFromStructAST(pos, t, provided, pathPrefix)
	case *ast.ArrayType:
		if t.Len == nil {
			return
		}
		c.reportMissingNonNilFields(pos, t.Elt, nil, pathPrefix)
	case *ast.StarExpr, *ast.MapType, *ast.ChanType, *ast.InterfaceType, *ast.FuncType:
		return
	case *ast.SelectorExpr:
		c.reportExternalTypeFields(pos, t, provided)
	}
}

func (c *Checker) reportExternalTypeFields(pos token.Pos, sel *ast.SelectorExpr, provided map[string]bool) {
	if sel == nil || c.resolver == nil {
		return
	}
	// External field contracts come from .gna (M4). Positional mapping is
	// intentionally not applied here.
	_ = pos
	_ = provided
}

func (c *Checker) lookupStructAST(name string) *ast.StructType {
	if c.file == nil {
		return nil
	}
	for _, decl := range c.file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name == nil || ts.Name.Name != name {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				return st
			}
		}
	}
	return nil
}

func (c *Checker) selectorFieldIsNonNil(sel *ast.SelectorExpr) bool {
	if sel == nil || sel.Sel == nil {
		return false
	}
	// Local struct field non-nil lookup by receiver type name when available.
	if id, ok := sel.X.(*ast.Ident); ok {
		if fields, ok := c.structFields[id.Name]; ok {
			return fields[sel.Sel.Name]
		}
		// Variable whose type is a local named struct — conservative: use
		// field map if any local type declares this field name as !T.
		for _, fields := range c.structFields {
			if fields[sel.Sel.Name] {
				return true
			}
		}
	}
	return false
}
