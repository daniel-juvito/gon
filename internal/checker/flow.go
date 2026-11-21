package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// GW006: an ordinary flow into a `!T` slot of a non-interface nilable kind
// (RFC explicit non-nil conversion, sections 6.4 and 6.6).
//
// flowOutcome is the single decision, over the whole matrix of section 6.4; the
// slot sites (variable initialiser, assignment, argument, result, field and
// element of a composite literal) only supply the expression and the Go type of
// the slot and call reportOrdinaryFlow. The order of the checks and the place
// of the slot do not change the outcome.

// flowOutcome reports whether expr flowing into a slot declared `!U`, where
// slot is the Go type U, is an ordinary flow: a GW006 warning, together with the
// static type S of the value (for the message). It is silent when there is no
// type information, for the nil literal (GN001 elsewhere), for a slot that is
// an interface (D3a, D3b and D3d apply) or a type parameter (OQ-2: D3b is
// unchanged), and for a recognised non-nil source whose static type is
// identical to U.
func (c *Checker) flowOutcome(expr ast.Expr, slot types.Type) (bool, types.Type) {
	if c.info == nil || expr == nil || slot == nil {
		return false, nil
	}
	if isNilIdent(unparenExpr(expr)) {
		return false, nil
	}
	if !nilableNonInterfaceKind(slot) {
		return false, nil
	}
	src := c.staticTypeOf(expr)
	if src == nil {
		return false, nil
	}
	switch t := src.(type) {
	case *types.Tuple:
		return false, nil // a multi-value call is positional; it is not one value
	case *types.Basic:
		if t.Kind() == types.UntypedNil {
			return false, nil
		}
	}
	if c.isRecognisedSource(expr) {
		// A slot that still mentions a type parameter cannot be compared with
		// the instantiated type of the value; identity is then not decidable
		// here and a recognised source is accepted.
		if types.Identical(src, slot) || mentionsTypeParam(slot) {
			return false, nil
		}
	}
	return true, src
}

// nilableNonInterfaceKind reports whether t is a pointer, slice, map, channel or
// function type. An interface and a type parameter are not (RFC section 6.6,
// OQ-1 and OQ-2).
func nilableNonInterfaceKind(t types.Type) bool {
	if _, isParam := t.(*types.TypeParam); isParam {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return true
	}
	return false
}

// mentionsTypeParam reports whether t contains a type parameter that has not
// been instantiated.
func mentionsTypeParam(t types.Type) bool {
	switch x := t.(type) {
	case *types.TypeParam:
		return true
	case *types.Pointer:
		return mentionsTypeParam(x.Elem())
	case *types.Slice:
		return mentionsTypeParam(x.Elem())
	case *types.Array:
		return mentionsTypeParam(x.Elem())
	case *types.Chan:
		return mentionsTypeParam(x.Elem())
	case *types.Map:
		return mentionsTypeParam(x.Key()) || mentionsTypeParam(x.Elem())
	case *types.Signature:
		return tupleMentionsTypeParam(x.Params()) || tupleMentionsTypeParam(x.Results())
	case *types.Named:
		args := x.TypeArgs()
		for i := 0; i < args.Len(); i++ {
			if mentionsTypeParam(args.At(i)) {
				return true
			}
		}
	}
	return false
}

func tupleMentionsTypeParam(t *types.Tuple) bool {
	for i := 0; i < t.Len(); i++ {
		if mentionsTypeParam(t.At(i).Type()) {
			return true
		}
	}
	return false
}

// isRecognisedSource is the Part A set of recognised non-nil sources (RFC
// section 6.6): an identifier declared `!T` or bound by the rules that exist
// today, a call whose first result is `!T`, a selector of a `!` field, the
// construction forms of a direct flow (a composite literal, `&literal`, `new`,
// `make`, a function literal) and the non-nil conversion (!T)(x). Parentheses
// never change recognition. There is no `append`, no slice expression, no
// binding by inference and no propagation across types: those are Part B.
func (c *Checker) isRecognisedSource(expr ast.Expr) bool {
	e := unparenExpr(expr)
	switch x := e.(type) {
	case *ast.CompositeLit, *ast.FuncLit:
		return true
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			_, isLit := unparenExpr(x.X).(*ast.CompositeLit)
			return isLit
		}
		return false
	case *ast.CallExpr:
		if c.isConstructionBuiltin(x) {
			return true
		}
	}
	return c.isNonNilSource(e)
}

// isConstructionBuiltin reports whether call is the builtin new or make (not a
// function of the same name that the program declares).
func (c *Checker) isConstructionBuiltin(call *ast.CallExpr) bool {
	id, ok := unparenExpr(call.Fun).(*ast.Ident)
	if !ok || c.info == nil {
		return false
	}
	b, ok := c.info.Uses[id].(*types.Builtin)
	return ok && (b.Name() == "new" || b.Name() == "make")
}

// reportOrdinaryFlow emits GW006 at expr when flowOutcome says so.
func (c *Checker) reportOrdinaryFlow(expr ast.Expr, slot types.Type) {
	warn, src := c.flowOutcome(expr, slot)
	if !warn {
		return
	}
	c.addWarning(expr.Pos(), "GW006", fmt.Sprintf(
		"non-nil contract: value of type %s does not satisfy !%s; use (!%s)(x) to make an explicit claim",
		c.typeString(src), c.typeString(slot), c.typeString(slot)))
}

// typeString writes a type relative to the package being checked.
func (c *Checker) typeString(t types.Type) string {
	local := c.localTypesPackage()
	return types.TypeString(t, func(p *types.Package) string {
		if p == local {
			return ""
		}
		return p.Name()
	})
}

// localTypesPackage returns the go/types package of the file being checked.
func (c *Checker) localTypesPackage() *types.Package {
	if c.info == nil {
		return nil
	}
	for _, decl := range c.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if obj := c.info.Defs[d.Name]; obj != nil {
				return obj.Pkg()
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if obj := c.info.Defs[s.Name]; obj != nil {
						return obj.Pkg()
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if obj := c.info.Defs[n]; obj != nil {
							return obj.Pkg()
						}
					}
				}
			}
		}
	}
	return nil
}

// litFieldType returns the Go type of a field of the struct built by a composite
// literal: the field called name, or, when name is empty, the field at index
// (an unkeyed literal). It is nil without type information.
func (c *Checker) litFieldType(lit *ast.CompositeLit, name string, index int) types.Type {
	t := c.staticTypeOf(lit)
	if t == nil {
		return nil
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	if name != "" {
		for i := 0; i < st.NumFields(); i++ {
			if st.Field(i).Name() == name {
				return st.Field(i).Type()
			}
		}
		return nil
	}
	if index >= 0 && index < st.NumFields() {
		return st.Field(index).Type()
	}
	return nil
}

// declaredParamIsTypeParam reports whether the parameter at paramIndex of the
// function or method called through fun is declared with a type parameter as its
// type (`func Ptr[P ~*T](p !P)`). go/types records the instantiated parameter
// type at the call (`*T`), but the slot is a type parameter, which GW006 leaves
// to D3b (RFC section 10.2, OQ-2).
func (c *Checker) declaredParamIsTypeParam(fun ast.Expr, paramIndex int) bool {
	if c.info == nil || fun == nil || paramIndex < 0 {
		return false
	}
	var fn *types.Func
	switch f := genericOrigin(fun).(type) {
	case *ast.Ident:
		fn, _ = c.info.ObjectOf(f).(*types.Func)
	case *ast.SelectorExpr:
		if s, ok := c.info.Selections[f]; ok {
			fn, _ = s.Obj().(*types.Func)
		} else {
			fn, _ = c.info.ObjectOf(f.Sel).(*types.Func)
		}
	}
	if fn == nil {
		return false
	}
	sig, ok := fn.Origin().Type().(*types.Signature)
	if !ok {
		return false
	}
	n := sig.Params().Len()
	if n == 0 {
		return false
	}
	t := sig.Params().At(minInt(paramIndex, n-1)).Type()
	if sig.Variadic() && paramIndex >= n-1 {
		if sl, ok := t.(*types.Slice); ok {
			t = sl.Elem()
		}
	}
	_, isParam := t.(*types.TypeParam)
	return isParam
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
