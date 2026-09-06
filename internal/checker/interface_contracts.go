package checker

import (
	"go/ast"
	"go/types"
)

// Interface non-nil contracts (v1.4 / M1a).
//
// Executable counterpart of docs/rfc-interface-semantics.md. The RFC locks
// `!I` to mean "the interface value is non-nil" — never a claim about the
// dynamic value. The only new enforcement relative to concrete `!T` is D3b:
// an ordinary interface-typed expression cannot satisfy `!I`, unconditionally
// and without flow-sensitive narrowing. Concrete/typed operands (D3a, even
// typed-nil) and existing `!I` sources of the *same* interface type (D3d / D4)
// are accepted. Embedding does not propagate contracts (§3.7).

// isNonNilSource reports whether expr is already an established non-nil source
// under existing Gon contract rules: a `!`-bound identifier (D3d) or an
// immediate call whose first result position is annotated `!T` (D4). No other
// form — conversion, assertion, arbitrary expression — qualifies (D6).
func (c *Checker) isNonNilSource(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return c.isNonNilSource(e.X)
	case *ast.Ident:
		nn, ok := c.lookup(e.Name)
		return ok && nn
	case *ast.CallExpr:
		results := c.resolveCallResults(e.Fun)
		return len(results) > 0 && results[0]
	default:
		return false
	}
}

// staticTypeOf returns the go/types type of the immediate expression, or nil
// when type information is unavailable.
func (c *Checker) staticTypeOf(expr ast.Expr) types.Type {
	if c.info == nil || expr == nil {
		return nil
	}
	tv, ok := c.info.Types[expr]
	if !ok || tv.Type == nil {
		return nil
	}
	return tv.Type
}

// staticTypeIsInterface reports whether the static type of expr, as recorded by
// go/types for the immediate expression, has an interface underlying type.
//
// It reads the immediate expression's static type only: an explicit conversion
// `I(x)` has static type `I` here even though its operand is concrete (D6c,
// §3.9). Requires type information; when unavailable the check degrades to
// silent, matching the rest of the checker.
func (c *Checker) staticTypeIsInterface(expr ast.Expr) bool {
	t := c.staticTypeOf(expr)
	if t == nil {
		return false
	}
	_, isIface := t.Underlying().(*types.Interface)
	return isIface
}

// cannotSatisfyBangInterface reports whether expr cannot satisfy a `!target`
// interface contract (RFC D3b + D3d same-type rule + §3.7).
//
// Rules:
//   - nil literal is handled by the existing GN001 path (D3c)
//   - concrete / non-interface static type → allowed (D3a), even typed-nil
//   - ordinary interface (no ! source) → reject (D3b)
//   - ! source whose static interface type is not types.Identical to target
//     → reject (D3d "same interface type"; embedding does not propagate)
//   - ! source with Identical type → allowed
//
// When target is nil (type info missing), a non-nil interface source is
// accepted so the check degrades silently like the rest of the checker.
func (c *Checker) cannotSatisfyBangInterface(expr ast.Expr, target types.Type) bool {
	if expr == nil || isNilIdent(expr) {
		return false
	}
	srcType := c.staticTypeOf(expr)
	if srcType == nil {
		return false // degrade silent without type info
	}
	if _, isIface := srcType.Underlying().(*types.Interface); !isIface {
		return false // concrete / typed operand: D3a
	}
	if !c.isNonNilSource(expr) {
		return true // ordinary I → D3b
	}
	if target == nil {
		return false // non-nil source, no target type to compare
	}
	// D3d + §3.7: the contract is attached to the particular interface type.
	// ReadCloser and Reader are not Identical; embedding does not transfer !.
	return !types.Identical(srcType, target)
}

// ordinaryInterfaceValue is the target-agnostic form used when no target type
// is available. Prefer cannotSatisfyBangInterface when the target type is known.
func (c *Checker) ordinaryInterfaceValue(expr ast.Expr) bool {
	return c.cannotSatisfyBangInterface(expr, nil)
}

// callArgParamType returns the Go type of the formal parameter at position
// paramIndex (receiver excluded, matching resolveCallParams indexing) for a
// call whose function expression is fun, or nil when it cannot be determined.
// A variadic final parameter contributes its element type for every trailing
// argument.
func (c *Checker) callArgParamType(fun ast.Expr, paramIndex int) types.Type {
	if c.info == nil || fun == nil || paramIndex < 0 {
		return nil
	}
	var sig *types.Signature
	if sel, ok := fun.(*ast.SelectorExpr); ok {
		if s, found := c.info.Selections[sel]; found {
			if fn, ok := s.Obj().(*types.Func); ok {
				sig, _ = fn.Type().(*types.Signature)
			}
		}
	}
	if sig == nil {
		if tv, ok := c.info.Types[fun]; ok && tv.Type != nil {
			sig, _ = tv.Type.Underlying().(*types.Signature)
		}
	}
	if sig == nil {
		var obj types.Object
		switch f := fun.(type) {
		case *ast.Ident:
			obj = c.info.ObjectOf(f)
		case *ast.SelectorExpr:
			obj = c.info.ObjectOf(f.Sel)
		}
		if obj != nil {
			sig, _ = obj.Type().Underlying().(*types.Signature)
		}
	}
	if sig == nil {
		return nil
	}
	params := sig.Params()
	n := params.Len()
	if n == 0 {
		return nil
	}
	if sig.Variadic() && paramIndex >= n-1 {
		last := params.At(n - 1).Type()
		if s, ok := last.(*types.Slice); ok {
			return s.Elem()
		}
		return last
	}
	if paramIndex < n {
		return params.At(paramIndex).Type()
	}
	return nil
}
