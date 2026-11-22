package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// An ordinary flow into a `!T` slot of a non-interface nilable kind is GN001 (RFC
// explicit non-nil conversion, sections 6.4 and 6.6 and the amendment A2, decided
// 2026-11-22: the explicit conversion is the only way through). Until v1.21 the same
// flow was the warning GW006.
//
// flowOutcome is the single decision, over the whole matrix of section 6.4; the
// slot sites (variable initialiser, assignment, argument, result, field and
// element of a composite literal) only supply the expression and the Go type of
// the slot and call reportOrdinaryFlow. The order of the checks and the place
// of the slot do not change the outcome.

// flowOutcome reports whether expr flowing into a slot declared `!U`, where
// slot is the Go type U, is an ordinary flow: a GN001 error, together with the
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
	src := c.staticTypeOf(expr)
	if src == nil {
		return false, nil
	}
	if _, isTuple := src.(*types.Tuple); isTuple {
		return false, nil // a multi-value call is positional: see reportTupleFlows
	}
	return c.flowDecision(src, c.isRecognisedSource(expr), slot), src
}

// flowDecision is the matrix of RFC section 6.4 for one value of static type
// src, known to be a recognised non-nil source or not, flowing into a slot of
// Go type slot. It is shared by a single expression and by one result of a
// multi-value call.
func (c *Checker) flowDecision(src types.Type, recognised bool, slot types.Type) bool {
	if src == nil || slot == nil || !nilableNonInterfaceKind(slot) {
		return false
	}
	if b, ok := src.(*types.Basic); ok && b.Kind() == types.UntypedNil {
		return false
	}
	if recognised {
		// A slot that still mentions a type parameter cannot be compared with
		// the instantiated type of the value; identity is then not decidable
		// here and a recognised source is accepted.
		if types.Identical(src, slot) || mentionsTypeParam(slot) {
			return false
		}
	}
	return true
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
// `make`, a function literal), any `&operand` and the non-nil conversion (!T)(x).
// Parentheses never change recognition. There is no `append`, no slice expression, no
// binding by inference and no propagation across types: those are Part B.
func (c *Checker) isRecognisedSource(expr ast.Expr) bool {
	e := unparenExpr(expr)
	switch x := e.(type) {
	case *ast.CompositeLit, *ast.FuncLit:
		return true
	case *ast.UnaryExpr:
		// Any &operand: the address of an addressable operand, or of a composite
		// literal, is never nil; evaluating it either gives that address or panics
		// (&p.f with p nil, &a[i] out of range), exactly as Go does (RFC section 4,
		// decided 2026-11-22). The static type is *T, so the identity rule applies.
		return x.Op == token.AND
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

// reportOrdinaryFlow emits GN001 at expr when flowOutcome says so.
func (c *Checker) reportOrdinaryFlow(expr ast.Expr, slot types.Type) {
	warn, src := c.flowOutcome(expr, slot)
	if !warn {
		return
	}
	c.addError(expr.Pos(), "GN001", fmt.Sprintf(
		"cannot use value of type %s as !%s; use (!%s)(x) to make an explicit claim",
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
// type at the call (`*T`), but the slot is a type parameter, which this rule leaves
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

// reportTupleFlows is the positional form of reportOrdinaryFlow for a call whose
// results flow into several slots at once: `a, b = f()`, `var a, b !T = f()`,
// `return f()` and `g(f())`. slots[i] is the Go type of the `!` slot that receives
// result i, or nil when that result goes to no `!` slot. A result is a recognised
// source exactly when its position in the contract of the callee is `!T`.
func (c *Checker) reportTupleFlows(call *ast.CallExpr, slots []types.Type) {
	if c.info == nil || call == nil {
		return
	}
	tup, ok := c.staticTypeOf(call).(*types.Tuple)
	if !ok {
		return
	}
	results := c.resolveCallResults(call.Fun)
	for i := 0; i < tup.Len() && i < len(slots); i++ {
		if slots[i] == nil {
			continue
		}
		src := tup.At(i).Type()
		recognised := i < len(results) && results[i]
		if c.flowDecision(src, recognised, slots[i]) {
			c.addError(call.Pos(), "GN001", fmt.Sprintf(
				"cannot use value of type %s as !%s; use (!%s)(x) to make an explicit claim",
				c.typeString(src), c.typeString(slots[i]), c.typeString(slots[i])))
		}
	}
}

// assignSlotType returns the Go type of the `!` slot an assignment target names
// (a variable declared or bound `!`, or a `!` field), and nil for anything else.
func (c *Checker) assignSlotType(lhs ast.Expr) types.Type {
	switch x := unparenExpr(lhs).(type) {
	case *ast.Ident:
		if x.Name == "_" || c.info == nil {
			return nil
		}
		if nn, exists := c.lookup(x.Name); exists && nn {
			if obj := c.info.ObjectOf(x); obj != nil {
				return obj.Type()
			}
		}
	case *ast.SelectorExpr:
		if c.selectorFieldIsNonNil(x) {
			return c.selectorFieldType(x)
		}
	}
	return nil
}
