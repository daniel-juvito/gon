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
// typed-nil) and existing `!I` sources (D3d / D4) are accepted unchanged.

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

// staticTypeIsInterface reports whether the static type of expr, as recorded by
// go/types for the immediate expression, has an interface underlying type.
//
// It reads the immediate expression's static type only: an explicit conversion
// `I(x)` has static type `I` here even though its operand is concrete (D6c,
// §3.9). Requires type information; when unavailable the check degrades to
// silent, matching the rest of the checker.
func (c *Checker) staticTypeIsInterface(expr ast.Expr) bool {
	if c.info == nil {
		return false
	}
	tv, ok := c.info.Types[expr]
	if !ok || tv.Type == nil {
		return false
	}
	_, isIface := tv.Type.Underlying().(*types.Interface)
	return isIface
}

// ordinaryInterfaceValue reports whether expr is an ordinary interface-typed
// value that cannot satisfy a `!I` contract (RFC D3b / §3.3). The nil literal
// is excluded here; it is handled by the existing GN001 nil-assignment path
// (D3c / §3.4).
func (c *Checker) ordinaryInterfaceValue(expr ast.Expr) bool {
	if expr == nil || isNilIdent(expr) {
		return false
	}
	if !c.staticTypeIsInterface(expr) {
		return false // concrete / typed operand: D3a
	}
	return !c.isNonNilSource(expr) // existing !I source: D3d / D4
}
