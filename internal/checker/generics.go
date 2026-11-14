package checker

import (
	"fmt"
	"go/ast"
	"go/types"
)

// Generic semantics (v1.8 / M3).
//
// Executable counterpart of docs/rfc-generic-semantics.md.
//
//	G2  — `!X` on a type parameter is valid iff X is a nilable type parameter
//	      (typeParamNilable); otherwise GN003.
//	G3  — field contracts of a generic struct apply through instantiation
//	      (`Box[T]{}`): genericOrigin resolves the instantiated type
//	      expression to the generic declaration's name.
//	G4  — `!` anywhere inside a type argument is GN003 (reserved).
//	G5  — `F[T](…)` resolves to F's declaration (genericOrigin); inferred
//	      calls `F(x)` already resolve by name.
//	G7  — `var v !P` with no initializer is GN002 for a nilable P.
//	G9  — `!` anywhere inside a constraint is GN003 (reserved).

// genericOrigin strips a generic instantiation (`Box[T]`, `Reg[K, V]`) of a
// **local** declaration down to the identifier naming the generic declaration.
// Contracts are positional in the declaration (G3/G5); instantiation
// substitutes types only. An instantiation of an external generic
// (`pkg.Box[T]`) is returned unchanged: generic `.gna` application is M5b
// (v1.9) and v1.8 behaviour for it is pinned (G10). Any other expression is
// returned unchanged.
func genericOrigin(expr ast.Expr) ast.Expr {
	base := expr
	instantiated := false
	for {
		switch e := base.(type) {
		case *ast.IndexExpr:
			base, instantiated = e.X, true
			continue
		case *ast.IndexListExpr:
			base, instantiated = e.X, true
			continue
		case *ast.ParenExpr:
			base = e.X
			continue
		}
		break
	}
	if !instantiated {
		return expr
	}
	if _, ok := base.(*ast.Ident); ok {
		return base
	}
	return expr
}

// typeParamOf returns the type parameter named by a type expression (a bare
// identifier resolving to a *types.TypeParam), or nil.
func (c *Checker) typeParamOf(typeExpr ast.Expr) *types.TypeParam {
	if c.info == nil {
		return nil
	}
	for {
		p, ok := typeExpr.(*ast.ParenExpr)
		if !ok {
			break
		}
		typeExpr = p.X
	}
	id, ok := typeExpr.(*ast.Ident)
	if !ok {
		return nil
	}
	if obj := c.info.Uses[id]; obj != nil {
		if tp, ok := obj.Type().(*types.TypeParam); ok {
			return tp
		}
	}
	if tv, ok := c.info.Types[id]; ok && tv.Type != nil {
		if tp, ok := tv.Type.(*types.TypeParam); ok {
			return tp
		}
	}
	return nil
}

// typeParamNilable implements RFC §3: a type parameter is nilable iff SOME
// element of its constraint (an intersection) is a non-empty union whose every
// term has a nilable underlying kind. The type set is a subset of that
// union, so every possible type argument is nilable. Method terms never
// establish nilability.
func typeParamNilable(tp *types.TypeParam) bool {
	if tp == nil {
		return false
	}
	iface, ok := tp.Constraint().Underlying().(*types.Interface)
	if !ok {
		return false
	}
	return interfaceRestrictsToNilable(iface, 0)
}

func interfaceRestrictsToNilable(iface *types.Interface, depth int) bool {
	if depth > 8 { // defensive: embedded-interface cycles are a type error
		return false
	}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		switch e := iface.EmbeddedType(i).(type) {
		case *types.Union:
			if unionAllNilable(e) {
				return true
			}
		default:
			// A single term (`~*T`, `*T`) is presented by go/types as a
			// Union of one; an embedded interface (named or literal) is
			// examined recursively; anything else is a plain type element.
			if sub, ok := e.Underlying().(*types.Interface); ok {
				if _, isTP := e.(*types.TypeParam); isTP {
					continue
				}
				if interfaceRestrictsToNilable(sub, depth+1) {
					return true
				}
				continue
			}
			if nilableUnderlying(e) {
				return true
			}
		}
	}
	return false
}

func unionAllNilable(u *types.Union) bool {
	if u.Len() == 0 {
		return false
	}
	for i := 0; i < u.Len(); i++ {
		if !nilableUnderlying(u.Term(i).Type()) {
			return false
		}
	}
	return true
}

// nilableUnderlying reports whether t's underlying kind is nilable: pointer,
// slice, map, chan, function, or interface (v1.4 / v1.6). A bare type
// parameter term is not nilable here (constraints cannot name one).
func nilableUnderlying(t types.Type) bool {
	if _, ok := t.(*types.TypeParam); ok {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	}
	return false
}

// checkBangInTypeArgsAndConstraints reports GN003 for a `!` written inside a
// type argument (G4) or inside a type-parameter constraint (G9). Neither is a
// value-level contract position: a type argument's identity is go/types' (I3)
// and constraints describe type sets.
func (c *Checker) checkBangInTypeArgsAndConstraints() {
	ast.Inspect(c.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IndexExpr:
			c.reportBangIn(x.Index, "a type argument",
				"type arguments carry no contract; the declaration's own `!` positions apply (RFC G4)")
		case *ast.IndexListExpr:
			for _, idx := range x.Indices {
				c.reportBangIn(idx, "a type argument",
					"type arguments carry no contract; the declaration's own `!` positions apply (RFC G4)")
			}
		case *ast.FuncDecl:
			c.checkTypeParamConstraints(x.Type.TypeParams)
		case *ast.TypeSpec:
			c.checkTypeParamConstraints(x.TypeParams)
		}
		return true
	})
}

func (c *Checker) checkTypeParamConstraints(fl *ast.FieldList) {
	if fl == nil {
		return
	}
	for _, f := range fl.List {
		c.reportBangIn(f.Type, "a type constraint",
			"constraints describe type sets, not values; nilability is read from the constraint (RFC G9)")
	}
}

// reportBangIn emits one GN003 per `!` found anywhere inside expr.
func (c *Checker) reportBangIn(expr ast.Expr, where, why string) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		e, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if c.isNonNil(e) {
			c.addError(e.Pos(), "GN003", fmt.Sprintf(
				"non-nil modifier ! is not valid in %s; %s", where, why))
			return false // report the outermost `!` of each chain once
		}
		return true
	})
}

func typeParamBangMessage(name string) string {
	return fmt.Sprintf(
		"non-nil modifier ! is not valid on type parameter %s: its constraint does not restrict it to nilable kinds (pointer, slice, map, channel, function, interface)",
		name)
}

// reportBadElementBang emits the GN003 for `!` on an element / value type
// that is not a meaningful nilable kind; a type-parameter element gets the
// G2 wording.
func (c *Checker) reportBadElementBang(elt ast.Expr) {
	if c.typeParamOf(elt) != nil {
		c.addError(elt.Pos(), "GN003", typeParamBangMessage(formatType(elt)))
		return
	}
	c.addError(elt.Pos(), "GN003", fmt.Sprintf(
		"non-nil modifier ! is not valid on element type %s; ! applies only to pointer, slice, map, channel, function, and interface types",
		formatType(elt)))
}
