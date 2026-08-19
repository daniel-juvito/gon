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
		if len(field.Names) == 0 {
			continue
		}
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
		if ok {
			for name, nn := range fields {
				p := name
				if pathPrefix != "" {
					p = pathPrefix + "." + name
				}
				if nn && (provided == nil || !provided[name]) {
					c.addError(pos, "GN002", fmt.Sprintf("missing required non-nil field %s", p))
				}
			}
		}
		st := c.lookupStructAST(t.Name)
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

func (c *Checker) reportExternalTypeFields(pos token.Pos, sel *ast.SelectorExpr, provided map[string]bool) {
	if c.resolver == nil || sel == nil || sel.X == nil || sel.Sel == nil {
		return
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}
	pkgPath, ok := c.imports[x.Name]
	if !ok {
		return
	}
	file, err := c.resolvePackage(pkgPath)
	if err != nil || file == nil || file.Types == nil {
		return
	}
	typeAnn, ok := file.Types[sel.Sel.Name]
	if !ok || typeAnn == nil {
		return
	}
	for name, nn := range typeAnn.Fields {
		if nn && (provided == nil || !provided[name]) {
			c.addError(pos, "GN002", fmt.Sprintf("missing required non-nil field %s.%s", sel.Sel.Name, name))
		}
	}
}

func (c *Checker) checkBinaryExprNil(expr *ast.BinaryExpr) {
	if expr.Op != token.EQL && expr.Op != token.NEQ {
		return
	}
	var x ast.Expr
	if isNilIdent(expr.X) {
		x = expr.Y
	} else if isNilIdent(expr.Y) {
		x = expr.X
	} else {
		return
	}
	if id, ok := x.(*ast.Ident); ok {
		nn, exists := c.lookup(id.Name)
		if !exists || !nn {
			return
		}
		if expr.Op == token.EQL {
			c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("%s is non-nil; comparison with nil is always false", id.Name))
		} else {
			c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("%s is non-nil; comparison with nil is always true", id.Name))
		}
		return
	}
	if sel, ok := x.(*ast.SelectorExpr); ok {
		if !c.selectorFieldIsNonNil(sel) {
			return
		}
		name := sel.Sel.Name
		if expr.Op == token.EQL {
			c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("%s is non-nil; comparison with nil is always false", name))
		} else {
			c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("%s is non-nil; comparison with nil is always true", name))
		}
	}
}

func (c *Checker) selectorFieldIsNonNil(sel *ast.SelectorExpr) bool {
	fieldName := sel.Sel.Name
	if c.info != nil {
		if tv, ok := c.info.Types[sel.X]; ok && tv.Type != nil {
			if c.namedTypeFieldNonNil(tv.Type, fieldName) {
				return true
			}
		}
	}
	for _, fields := range c.structFields {
		if fields[fieldName] {
			return true
		}
	}
	return false
}

func (c *Checker) namedTypeFieldNonNil(typ types.Type, fieldName string) bool {
	if typ == nil {
		return false
	}
	if p, ok := typ.(*types.Pointer); ok {
		typ = p.Elem()
	}
	switch t := typ.(type) {
	case *types.Named:
		if fields, ok := c.structFields[t.Obj().Name()]; ok {
			if fields[fieldName] {
				return true
			}
		}
		return c.namedTypeFieldNonNil(t.Underlying(), fieldName)
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			f := t.Field(i)
			if f.Name() == fieldName {
				for _, fields := range c.structFields {
					if fields[fieldName] {
						return true
					}
				}
				return false
			}
			if f.Embedded() && c.namedTypeFieldNonNil(f.Type(), fieldName) {
				return true
			}
		}
	}
	return false
}
