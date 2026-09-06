package checker

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"strconv"
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
	switch a := arg.(type) {
	case *ast.Ident, *ast.StructType:
		trace := &ContractTrace{Origin: "new(" + typeExprName(arg) + ")"}
		c.reportMissingNonNilFieldsTraced(call.Pos(), arg, nil, "", trace)
	case *ast.SelectorExpr:
		// E6: new(pkg.T) is a zero-value construction site for external
		// `.gna` `types:` field contracts (own fields + one-hop embedded).
		c.reportExternalTypeFields(call.Pos(), a, nil)
		c.reportExternalEmbeddedFields(call.Pos(), a, nil)
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
		// v1.7 / M2b: element `!` contracts (GN001 explicit-nil, GN002
		// fixed-array zero-fill).
		c.checkElementContracts(lit, nil)
		if t.Len == nil {
			return // slice: no element zero-fill
		}
		c.checkFixedArrayElementShortfall(lit, t)
		if len(lit.Elts) == 0 {
			trace := &ContractTrace{Origin: origin}
			c.reportMissingNonNilFieldsTraced(lit.Pos(), t.Elt, nil, "", trace)
		}
	case *ast.MapType:
		// v1.7 / M2b: map value `!` contract (GN001 explicit-nil).
		c.checkElementContracts(lit, nil)
	case *ast.SelectorExpr:
		// E2 firewall: an unkeyed external literal with elements is not a
		// construction site — Gon does not reconstruct external field order.
		// `pkg.T{}` (empty) and keyed forms are still checked.
		if isUnkeyedWithElts(lit) {
			return
		}
		provided := providedKeyedOnly(lit)
		c.reportExternalTypeFields(lit.Pos(), t, provided)
		c.reportExternalEmbeddedFields(lit.Pos(), t, provided)
		c.checkExternalKeyedFieldValues(lit, t)
	}
}

// checkElementContracts enforces element `!` contracts at a composite-literal
// construction site (v1.7 / M2b, RFC E4 / E12): an explicitly written element
// (or map value) that is the untyped nil literal is GN001. Non-literal
// expressions — including ordinary calls that may return nil — are accepted
// (conservative, mirrors Type Coverage C4).
//
// elemTypeAST is the element / value type expression to check against; when nil
// it is derived from lit.Type. It is passed explicitly when recursing into an
// elided nested composite literal (`[][]!*T{{nil}}`).
func (c *Checker) checkElementContracts(lit *ast.CompositeLit, elemTypeAST ast.Expr) {
	if lit == nil {
		return
	}
	isMap := false
	if elemTypeAST == nil {
		switch t := lit.Type.(type) {
		case *ast.ArrayType:
			elemTypeAST = t.Elt
		case *ast.MapType:
			elemTypeAST = t.Value
			isMap = true
		default:
			return
		}
	}
	if elemTypeAST == nil {
		return
	}
	elemCarriesBang := c.isNonNil(elemTypeAST)

	// When the element type is itself an array/map, an elided child literal is
	// a nested construction site: recurse with the element's own element type,
	// and (for a fixed array) re-run the zero-fill check against the element
	// type itself.
	var innerElemType ast.Expr
	var childArrayType *ast.ArrayType
	switch et := elemTypeAST.(type) {
	case *ast.ArrayType:
		innerElemType = et.Elt
		if et.Len != nil {
			childArrayType = et
		}
	case *ast.MapType:
		innerElemType = et.Value
	}

	kind := "element"
	if isMap {
		kind = "map value"
	}
	for _, elt := range lit.Elts {
		val := elt
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			val = kv.Value
		}
		if elemCarriesBang && isNilIdent(val) {
			c.addErrorTrace(val.Pos(), "GN001", fmt.Sprintf(
				"cannot use nil as non-nil %s %s", kind, formatType(elemTypeAST)),
				&ContractTrace{Origin: compositeOrigin(lit)})
			continue
		}
		if innerElemType != nil {
			if child, ok := val.(*ast.CompositeLit); ok && child.Type == nil {
				c.checkElementContracts(child, innerElemType)
				if childArrayType != nil {
					c.checkFixedArrayElementShortfall(child, childArrayType)
				}
			}
		}
	}
}

// checkFixedArrayElementShortfall emits one GN002 when a fixed-array composite
// literal leaves at least one index at its zero value and the element type is a
// nilable kind carrying `!` (v1.7 / M2b, RFC E5). Coverage is by index, not by
// element count: keyed entries cover their key, unkeyed entries cover the
// running position. `[...]E` never reaches here (length == element count).
func (c *Checker) checkFixedArrayElementShortfall(lit *ast.CompositeLit, t *ast.ArrayType) {
	if lit == nil || t == nil || t.Len == nil {
		return
	}
	if _, ok := t.Len.(*ast.Ellipsis); ok {
		return
	}
	if !c.isNonNil(t.Elt) || !c.elemTypeIsNilable(t.Elt) {
		return
	}
	n, ok := c.constIntValue(t.Len)
	if !ok || n <= 0 {
		return
	}
	covered := make(map[int64]bool)
	next := int64(0)
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if k, ok := c.constIntValue(kv.Key); ok {
				covered[k] = true
				next = k + 1
			}
			continue
		}
		covered[next] = true
		next++
	}
	for i := int64(0); i < n; i++ {
		if !covered[i] {
			c.addErrorTrace(lit.Pos(), "GN002", fmt.Sprintf(
				"fixed-array literal leaves non-nil element index %d at its zero value", i),
				&ContractTrace{Origin: compositeOrigin(lit)})
			return
		}
	}
}

// elemTypeIsNilable reports whether an element type expression names a kind
// whose zero value is nil (pointer, slice, map, chan, func, interface, or a
// named/alias of those).
func (c *Checker) elemTypeIsNilable(e ast.Expr) bool {
	if e == nil {
		return false
	}
	if p, ok := e.(*ast.ParenExpr); ok {
		return c.elemTypeIsNilable(p.X)
	}
	if _, ok := e.(*ast.InterfaceType); ok {
		return true
	}
	return c.nilableRefKind(e) || c.staticTypeIsInterface(e)
}

// constIntValue extracts a non-negative constant integer value from an
// expression (array length or composite-literal key). Prefers go/types
// constant folding; falls back to a literal INT.
func (c *Checker) constIntValue(e ast.Expr) (int64, bool) {
	if c.info != nil {
		if tv, ok := c.info.Types[e]; ok && tv.Value != nil {
			if i, ok := constant.Int64Val(tv.Value); ok {
				return i, true
			}
		}
	}
	if bl, ok := e.(*ast.BasicLit); ok && bl.Kind == token.INT {
		if i, err := strconv.ParseInt(bl.Value, 0, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

// isUnkeyedWithElts reports whether lit has elements, none of which are
// key:value pairs (a positional external literal).
func isUnkeyedWithElts(lit *ast.CompositeLit) bool {
	if lit == nil || len(lit.Elts) == 0 {
		return false
	}
	for _, elt := range lit.Elts {
		if _, ok := elt.(*ast.KeyValueExpr); ok {
			return false
		}
	}
	return true
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
	pos := 0
	for _, elt := range lit.Elts {
		if _, ok := elt.(*ast.KeyValueExpr); ok {
			continue
		}
		if pos < len(fieldNames) {
			provided[fieldNames[pos]] = true
			pos++
		}
	}
	return provided
}

func providedKeyedOnly(lit *ast.CompositeLit) map[string]bool {
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

func (c *Checker) structFieldNamesInOrder(typ ast.Expr) []string {
	var names []string
	switch t := typ.(type) {
	case *ast.Ident:
		st := c.lookupStructAST(t.Name)
		if st == nil {
			return names
		}
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				if id, ok := field.Type.(*ast.Ident); ok {
					names = append(names, id.Name)
				}
				continue
			}
			for _, n := range field.Names {
				names = append(names, n.Name)
			}
		}
	case *ast.StructType:
		if t.Fields == nil {
			return names
		}
		for _, field := range t.Fields.List {
			if len(field.Names) == 0 {
				if id, ok := field.Type.(*ast.Ident); ok {
					names = append(names, id.Name)
				}
				continue
			}
			for _, n := range field.Names {
				names = append(names, n.Name)
			}
		}
	}
	return names
}

func (c *Checker) checkExplicitNilInComposite(lit *ast.CompositeLit, fields map[string]bool, typeName string) {
	if lit == nil || fields == nil {
		return
	}
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
	// Unkeyed explicit nils
	fieldNames := c.structFieldNamesInOrder(lit.Type)
	pos := 0
	for _, elt := range lit.Elts {
		if _, ok := elt.(*ast.KeyValueExpr); ok {
			continue
		}
		if pos >= len(fieldNames) {
			break
		}
		fieldName := fieldNames[pos]
		pos++
		if fields[fieldName] && isNilIdent(elt) {
			msg := fmt.Sprintf("cannot assign nil to non-nil field %s", fieldName)
			if typeName != "" {
				msg = fmt.Sprintf("cannot assign nil to non-nil field %s.%s", typeName, fieldName)
			}
			path := []string{fieldName}
			if typeName != "" {
				path = []string{typeName, fieldName}
			}
			c.addErrorTrace(elt.Pos(), "GN001", msg, &ContractTrace{
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
	// A type assertion never establishes a non-nil contract. v1.4 locked this
	// for interface targets (rfc-interface-semantics.md D6a / §3.8) and
	// deferred concrete targets; v1.6 (rfc-type-coverage.md C7 / §4.6) adopts
	// the amendment: `!` on ANY assertion target is GN001. The preprocessor
	// has already stripped the `!` and recorded its offset.
	if c.isNonNil(ta.Type) {
		msg := "type assertion target cannot carry a non-nil contract (!T); assertions do not establish a non-nil guarantee"
		if c.staticTypeIsInterface(ta.Type) {
			msg = "type assertion target cannot carry a non-nil contract (!I); assertions do not establish !I"
		}
		c.addError(ta.Type.Pos(), "GN001", msg)
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
