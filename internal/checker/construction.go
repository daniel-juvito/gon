package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkNewConstruction treats new(T) as a zero-value construction site when T
// resolves to a local struct. No general expression inference; non-struct T
// is ignored. External / selector types are not walked via new (M4 boundary).
func (c *Checker) checkNewConstruction(call *ast.CallExpr) {
	if call == nil || len(call.Args) != 1 {
		return
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "new" {
		return
	}
	arg := call.Args[0]
	switch arg.(type) {
	case *ast.Ident, *ast.StructType:
		trace := &ContractTrace{Origin: "new(" + typeExprName(arg) + ")"}
		c.reportMissingNonNilFieldsTraced(call.Pos(), arg, nil, "", trace)
	default:
		return
	}
}

// checkCompositeLitConstruction is the M2a entry for composite literals.
// Keyed and unkeyed forms share one semantic model for *local* structs.
// External types (SelectorExpr) keep the v1.2 keyed-only + .gna path and
// do not invent positional field order (M4 firewall).
func (c *Checker) checkCompositeLitConstruction(lit *ast.CompositeLit) {
	if lit == nil {
		return
	}
	origin := compositeOrigin(lit)
	provided := c.providedFieldsFromComposite(lit)

	switch t := lit.Type.(type) {
	case *ast.Ident:
		typeName := t.Name
		fields, ok := c.structFields[typeName]
		if ok {
			c.checkExplicitNilInComposite(lit, fields, typeName)
		}
		trace := &ContractTrace{Origin: origin}
		c.reportMissingNonNilFieldsTraced(lit.Pos(), t, provided, typeName, trace)
	case *ast.StructType:
		fields := c.fieldsFromStructAST(t)
		c.checkExplicitNilInComposite(lit, fields, "")
		trace := &ContractTrace{Origin: origin}
		c.reportMissingFromStructASTTraced(lit.Pos(), t, provided, "", trace)
	case *ast.ArrayType:
		if t.Len == nil {
			return
		}
		if len(lit.Elts) == 0 {
			trace := &ContractTrace{Origin: origin}
			c.reportMissingNonNilFieldsTraced(lit.Pos(), t.Elt, nil, "", trace)
		}
	case *ast.SelectorExpr:
		c.reportExternalTypeFields(lit.Pos(), t, providedKeyedOnly(lit))
	}
}

func (c *Checker) providedFieldsFromComposite(lit *ast.CompositeLit) map[string]bool {
	provided := make(map[string]bool)
	if lit == nil {
		return provided
	}
	keyed := false
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			keyed = true
			if key, ok := kv.Key.(*ast.Ident); ok {
				provided[key.Name] = true
			}
		}
	}
	if keyed {
		return provided
	}
	switch lit.Type.(type) {
	case *ast.Ident, *ast.StructType:
	default:
		return provided
	}
	fieldNames := c.structFieldNamesInOrder(lit.Type)
	for i := range lit.Elts {
		if i >= len(fieldNames) {
			break
		}
		provided[fieldNames[i]] = true
	}
	return provided
}

func providedKeyedOnly(lit *ast.CompositeLit) map[string]bool {
	provided := make(map[string]bool)
	if lit == nil {
		return provided
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok {
			provided[key.Name] = true
		}
	}
	return provided
}

func (c *Checker) structFieldNamesInOrder(typExpr ast.Expr) []string {
	if typExpr == nil {
		return nil
	}
	switch t := typExpr.(type) {
	case *ast.Ident:
		st := c.lookupStructAST(t.Name)
		if st == nil {
			return nil
		}
		return fieldNamesFromStructAST(st)
	case *ast.StructType:
		return fieldNamesFromStructAST(t)
	default:
		return nil
	}
}

func fieldNamesFromStructAST(st *ast.StructType) []string {
	if st == nil || st.Fields == nil {
		return nil
	}
	var names []string
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			switch ft := field.Type.(type) {
			case *ast.Ident:
				names = append(names, ft.Name)
			case *ast.StarExpr:
				if id, ok := ft.X.(*ast.Ident); ok {
					names = append(names, id.Name)
				}
			}
			continue
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

func (c *Checker) checkExplicitNilInComposite(lit *ast.CompositeLit, fields map[string]bool, typeName string) {
	if fields == nil {
		return
	}
	fieldNames := c.structFieldNamesInOrder(lit.Type)
	for i, elt := range lit.Elts {
		var fieldName string
		var val ast.Expr
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			fieldName = key.Name
			val = kv.Value
		} else {
			if i >= len(fieldNames) {
				continue
			}
			fieldName = fieldNames[i]
			val = elt
		}
		if fields[fieldName] && isNilIdent(val) {
			msg := fmt.Sprintf("cannot assign nil to non-nil field %s", fieldName)
			if typeName != "" {
				msg = fmt.Sprintf("cannot assign nil to non-nil field %s.%s", typeName, fieldName)
			}
			path := []string{fieldName}
			if typeName != "" {
				path = []string{typeName, fieldName}
			}
			c.addErrorTrace(val.Pos(), "GN001", msg, &ContractTrace{
				Origin: compositeOrigin(lit),
				Path:   path,
			})
		}
	}
}

func (c *Checker) reportMissingNonNilFieldsTraced(pos token.Pos, typExpr ast.Expr, provided map[string]bool, pathPrefix string, trace *ContractTrace) {
	if typExpr == nil {
		return
	}
	switch t := typExpr.(type) {
	case *ast.Ident:
		fields, ok := c.structFields[t.Name]
		if ok {
			st := c.lookupStructAST(t.Name)
			if st != nil {
				for _, field := range st.Fields.List {
					for _, name := range field.Names {
						nn := fields[name.Name]
						p := name.Name
						if pathPrefix != "" {
							p = pathPrefix + "." + name.Name
						}
						if nn && (provided == nil || !provided[name.Name]) {
							tr := trace.withPath(name.Name)
							if pathPrefix != "" && len(tr.Path) == 1 {
								tr.Path = splitPath(p)
							}
							c.addErrorTrace(pos, "GN002", fmt.Sprintf("missing required non-nil field %s", p), tr)
						}
					}
				}
			} else {
				for name, nn := range fields {
					p := name
					if pathPrefix != "" {
						p = pathPrefix + "." + name
					}
					if nn && (provided == nil || !provided[name]) {
						c.addErrorTrace(pos, "GN002", fmt.Sprintf("missing required non-nil field %s", p), trace.withPath(name))
					}
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
						c.reportMissingNonNilFieldsTraced(pos, field.Type, nil, p, trace.withPath(name))
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
					c.reportMissingNonNilFieldsTraced(pos, field.Type, nil, p, trace.withPath(name.Name))
				}
			}
		}
	case *ast.StructType:
		c.reportMissingFromStructASTTraced(pos, t, provided, pathPrefix, trace)
	case *ast.ArrayType:
		if t.Len == nil {
			return
		}
		c.reportMissingNonNilFieldsTraced(pos, t.Elt, nil, pathPrefix, trace)
	case *ast.StarExpr, *ast.MapType, *ast.ChanType, *ast.InterfaceType, *ast.FuncType:
		return
	case *ast.SelectorExpr:
		c.reportExternalTypeFields(pos, t, provided)
	}
}

func (c *Checker) reportMissingFromStructASTTraced(pos token.Pos, st *ast.StructType, provided map[string]bool, pathPrefix string, trace *ContractTrace) {
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
					c.reportMissingNonNilFieldsTraced(pos, ft, nil, p, trace.withPath(name))
				}
			case *ast.StructType:
				c.reportMissingFromStructASTTraced(pos, ft, nil, pathPrefix, trace)
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
				c.addErrorTrace(pos, "GN002", fmt.Sprintf("missing required non-nil field %s", p), trace.withPath(name.Name))
			}
			if provided == nil || !provided[name.Name] {
				c.reportMissingNonNilFieldsTraced(pos, field.Type, nil, p, trace.withPath(name.Name))
			}
		}
	}
}

func (c *Checker) checkTypeAssert(ta *ast.TypeAssertExpr) {
	if ta == nil || ta.Type == nil {
		return
	}
	id, ok := ta.X.(*ast.Ident)
	if !ok {
		return
	}
	nn, exists := c.lookup(id.Name)
	if !exists || !nn {
		return
	}
	c.addWarning(ta.Pos(), "GW003", fmt.Sprintf("type assertion on non-nil %s is redundant for nilability", id.Name))
}

func compositeOrigin(lit *ast.CompositeLit) string {
	if lit == nil {
		return "{}"
	}
	name := typeExprName(lit.Type)
	if name == "" {
		return "{}"
	}
	return name + "{}"
}

func typeExprName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeExprName(t.X) + "." + t.Sel.Name
	case *ast.StructType:
		return "struct"
	case *ast.StarExpr:
		return "*" + typeExprName(t.X)
	default:
		return ""
	}
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	var out []string
	cur := ""
	for i := 0; i < len(p); i++ {
		if p[i] == '.' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(p[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

var _ = token.NoPos
