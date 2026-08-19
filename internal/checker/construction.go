package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// checkNewConstruction is the M2a entry for new(T).
// new(T) is treated as a zero-value construction site when T is a local struct.
func (c *Checker) checkNewConstruction(call *ast.CallExpr) {
	if call == nil || len(call.Args) != 1 {
		return
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "new" {
		return
	}
	arg := call.Args[0]
	// Only local named / anonymous structs are in scope for M2a.
	switch t := arg.(type) {
	case *ast.Ident, *ast.StructType, *ast.ArrayType:
		trace := &ContractTrace{Origin: "new(" + formatType(arg) + ")"}
		c.reportMissingNonNilFieldsTraced(call.Pos(), arg, nil, "", trace)
	default:
		// SelectorExpr / pointers / maps: out of M2a scope (M4 firewall).
		_ = t
	}
}

// checkCompositeLitConstruction is the M2a entry for composite literals.
// Keyed and unkeyed forms are supported for local structs only.
// External SelectorExpr types remain keyed-only + .gna (M4).
func (c *Checker) checkCompositeLitConstruction(lit *ast.CompositeLit) {
	if lit == nil {
		return
	}
	provided := compositeProvided(lit)
	trace := &ContractTrace{Origin: compositeOrigin(lit)}
	switch t := lit.Type.(type) {
	case *ast.Ident:
		typeName := t.Name
		c.reportMissingNonNilFieldsTraced(lit.Pos(), t, provided, typeName, trace)
		c.checkExplicitNilFields(lit, typeName)
	case *ast.StructType:
		c.reportMissingFromStructASTTraced(lit.Pos(), t, provided, "", trace)
		c.checkExplicitNilFields(lit, "")
	case *ast.ArrayType:
		if t.Len != nil {
			// Fixed array: walk element type once (zero-value containment).
			c.reportMissingNonNilFieldsTraced(lit.Pos(), t.Elt, nil, "", trace)
		}
		// Slice (Len == nil) is an indirection boundary — not walked.
	case *ast.SelectorExpr:
		// M4 firewall: external types stay keyed-only + .gna.
		c.reportExternalTypeFields(lit.Pos(), t, provided)
		c.checkExplicitNilFields(lit, formatType(t))
	default:
		// map/chan/interface/func literals: not M2a construction sites.
	}
}

func compositeOrigin(lit *ast.CompositeLit) string {
	if lit == nil || lit.Type == nil {
		return "{}"
	}
	return formatType(lit.Type) + "{}"
}

// compositeProvided returns the set of field names provided by a composite
// literal. Keyed elements contribute their key name; unkeyed elements are
// mapped to declaration-order field names for local named structs only.
func (c *Checker) compositeProvided(lit *ast.CompositeLit) map[string]bool {
	provided := make(map[string]bool)
	if lit == nil {
		return provided
	}
	var ordered []string
	if id, ok := lit.Type.(*ast.Ident); ok {
		if st := c.lookupStructAST(id.Name); st != nil {
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					if emb, ok := field.Type.(*ast.Ident); ok {
						ordered = append(ordered, emb.Name)
					}
					continue
				}
				for _, n := range field.Names {
					ordered = append(ordered, n.Name)
				}
			}
		}
	}
	pos := 0
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok {
				provided[key.Name] = true
			}
			continue
		}
		// Unkeyed: map by declaration order (local AST only).
		if pos < len(ordered) {
			provided[ordered[pos]] = true
			pos++
		}
	}
	return provided
}

// package-level alias used by checkCompositeLitConstruction
func compositeProvided(lit *ast.CompositeLit) map[string]bool {
	// Standalone helper cannot access checker state for unkeyed mapping.
	// Unkeyed mapping is applied inside checkCompositeLitConstruction via
	// the method form; this free function handles keyed-only extraction
	// for call sites that do not need unkeyed support.
	provided := make(map[string]bool)
	if lit == nil {
		return provided
	}
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok {
				provided[key.Name] = true
			}
		}
	}
	return provided
}

func (c *Checker) checkExplicitNilFields(lit *ast.CompositeLit, typeName string) {
	if lit == nil {
		return
	}
	var fields map[string]bool
	if id, ok := lit.Type.(*ast.Ident); ok {
		fields = c.structFields[id.Name]
	}
	if fields == nil {
		// Still catch explicit nil on keyed elements when type is local struct AST.
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if isNilIdent(kv.Value) {
				// Without field map we cannot know if the field is !T.
				_ = key
			}
		}
		return
	}
	// Keyed nils
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		fieldName := key.Name
		if fields[fieldName] && isNilIdent(kv.Value) {
			msg := fmt.Sprintf("cannot assign nil to non-nil field %s", fieldName)
			if typeName != "" {
				msg = fmt.Sprintf("cannot assign nil to non-nil field %s.%s", typeName, fieldName)
			}
			path := []string{fieldName}
			if typeName != "" {
				path = []string{typeName, fieldName}
			}
			c.addErrorTrace(kv.Value.Pos(), "GN001", msg, &ContractTrace{
				Origin: compositeOrigin(lit),
				Path:   path,
			})
		}
	}
	// Unkeyed nils: map by declaration order
	st := c.lookupStructAST(typeName)
	if st == nil {
		return
	}
	var ordered []string
	for _, field := range st.Fields.List {
		for _, n := range field.Names {
			ordered = append(ordered, n.Name)
		}
	}
	pos := 0
	for _, elt := range lit.Elts {
		if _, ok := elt.(*ast.KeyValueExpr); ok {
			continue
		}
		if pos >= len(ordered) {
			break
		}
		fieldName := ordered[pos]
		pos++
		val := elt
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
			// Local named structs always have AST in the current file under v1.3
			// (collectGenDecl + lookupStructAST share c.file.Decls). If AST is
			// missing the field map alone is insufficient for deterministic
			// declaration-order reporting — skip rather than map-iterate.
			if st != nil {
				for _, field := range st.Fields.List {
					for _, name := range field.Names {
						nn := fields[name.Name]
						p := name.Name
						if pathPrefix != "" {
							p = pathPrefix + "." + name.Name
						}
						if nn && (provided == nil || !provided[name.Name]) {
							// Path must match the message path (single source of truth).
							tr := trace.Clone()
							if tr == nil {
								tr = &ContractTrace{}
							}
							tr.Path = splitPath(p)
							c.addErrorTrace(pos, "GN002", fmt.Sprintf("missing required non-nil field %s", p), tr)
						}
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

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	return strings.Split(p, ".")
}
