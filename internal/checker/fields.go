package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// checkCompositeLitFields is retained as a v1.2 compatibility alias.
// v1.3 construction (keyed + unkeyed + new) lives in construction.go.
func (c *Checker) checkCompositeLitFields(lit *ast.CompositeLit) {
	c.checkCompositeLitConstruction(lit)
}

func (c *Checker) fieldsFromStructAST(st *ast.StructType) map[string]bool {
	fields := make(map[string]bool)
	if st == nil || st.Fields == nil {
		return fields
	}
	for _, field := range st.Fields.List {
		isNN := c.isNonNil(field.Type)
		for _, name := range field.Names {
			fields[name.Name] = isNN
		}
	}
	return fields
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
			return // slices: stop at indirection
		}
		c.reportMissingNonNilFields(pos, t.Elt, nil, pathPrefix)
	case *ast.StarExpr, *ast.MapType, *ast.ChanType, *ast.InterfaceType, *ast.FuncType:
		return
	case *ast.SelectorExpr:
		c.reportExternalTypeFields(pos, t, provided)
	}
}

func (c *Checker) lookupStructAST(name string) *ast.StructType {
	for _, decl := range c.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				return st
			}
		}
	}
	return nil
}

func (c *Checker) reportExternalTypeFields(pos token.Pos, sel *ast.SelectorExpr, provided map[string]bool) {
	if c.info == nil || c.resolver == nil {
		return
	}
	tv, ok := c.info.Types[sel]
	if !ok {
		return
	}
	named, ok := tv.Type.(*types.Named)
	if !ok {
		if ptr, ok := tv.Type.(*types.Pointer); ok {
			named, ok = ptr.Elem().(*types.Named)
			if !ok {
				return
			}
		} else {
			return
		}
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return
	}
	path := pkg.Path()
	g, err := c.resolver.Resolve(path)
	if err != nil || g == nil {
		return
	}
	typeName := named.Obj().Name()
	fields, ok := g.Types[typeName]
	if !ok {
		return
	}
	for fname, ftype := range fields.Fields {
		if provided != nil && provided[fname] {
			continue
		}
		if len(ftype) > 0 && ftype[0] == '!' {
			c.addError(pos, "GN002", fmt.Sprintf("missing required non-nil field %s.%s", typeName, fname))
		}
	}
}

func (c *Checker) checkBinaryExprNil(expr *ast.BinaryExpr) {
	if expr.Op != token.EQL && expr.Op != token.NEQ {
		return
	}
	var nonNilSide ast.Expr
	if isNilIdent(expr.Y) {
		nonNilSide = expr.X
	} else if isNilIdent(expr.X) {
		nonNilSide = expr.Y
	} else {
		return
	}
	if id, ok := nonNilSide.(*ast.Ident); ok {
		if nn, exists := c.lookup(id.Name); exists && nn {
			c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("comparison of non-nil %s with nil is always %v", id.Name, expr.Op == token.NEQ))
		}
		return
	}
	if sel, ok := nonNilSide.(*ast.SelectorExpr); ok {
		if c.selectorFieldIsNonNil(sel) {
			c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("comparison of non-nil field %s with nil is always %v", sel.Sel.Name, expr.Op == token.NEQ))
		}
	}
}

func (c *Checker) selectorFieldIsNonNil(sel *ast.SelectorExpr) bool {
	if sel == nil {
		return false
	}
	if id, ok := sel.X.(*ast.Ident); ok {
		if fields, ok := c.structFields[id.Name]; ok {
			return fields[sel.Sel.Name]
		}
		// Try type of variable from scopes — conservative: check all known struct field maps
		for _, fields := range c.structFields {
			if fields[sel.Sel.Name] {
				// ambiguous without types; prefer info-based if available
				break
			}
		}
	}
	if c.info != nil {
		if selection, ok := c.info.Selections[sel]; ok && selection != nil {
			obj := selection.Obj()
			if obj == nil {
				return false
			}
			if f, ok := obj.(*types.Var); ok && f.IsField() {
				recv := selection.Recv()
				if recv == nil {
					return false
				}
				if ptr, ok := recv.(*types.Pointer); ok {
					recv = ptr.Elem()
				}
				if named, ok := recv.(*types.Named); ok {
					if fields, ok := c.structFields[named.Obj().Name()]; ok {
						return fields[f.Name()]
					}
				}
			}
		}
	}
	return false
}
