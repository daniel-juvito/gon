package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// Binding by inference (Part B, RFC docs/rfc-part-b-binding-and-convert.md, section 3).
//
// A local declared by `x := e` or by `var x = e` (no written type) is bound `!` when e
// is a recognised non-nil source of a form that is certainly non-nil (a literal, a
// function literal, &operand, new, make, append with an element or with a recognised
// base, a slice of a recognised value, (!T)(x), an identifier or field that is `!`) and
// x is of a pointer, slice, map, channel or function kind. A call whose result is
// declared `!T` binds as it did before. P1: a local whose address is taken (&x
// anywhere in the file) is not bound, because an alias can write nil.
//
// A bound local is a `!` variable like a declared one: `x = nil` is GN001, `x = e'` is
// the flow rule (GN001 for a value that is not recognised), `x == nil` is GW001. Its
// GN001 says how to keep it nilable. `gon convert` rewrites the initialiser so that the
// binding does not apply.

// bindsByInference reports whether the local id, defined from rhs, is bound `!` by the
// structural form of its initialiser. It records the position of the initialiser's
// variable for the hint of a later error.
func (c *Checker) bindsByInference(id *ast.Ident, rhs ast.Expr) bool {
	if c.info == nil || id == nil || rhs == nil || id.Name == "_" {
		return false
	}
	obj := c.info.Defs[id]
	if obj == nil || !nilableNonInterfaceKind(obj.Type()) {
		return false
	}
	if !c.isNewBindingForm(rhs) {
		return false
	}
	if c.addressTaken()[obj] {
		return false
	}
	if c.inferred == nil {
		c.inferred = make(map[types.Object]token.Pos)
	}
	c.inferred[obj] = id.Pos()
	return true
}

// isNewBindingForm is a recognised source of a form that Part B binds: not an ordinary
// call (a call whose result is `!T` is bound by the rules of return-value contracts).
func (c *Checker) isNewBindingForm(e ast.Expr) bool {
	e = unparenExpr(e)
	if call, ok := e.(*ast.CallExpr); ok {
		if _, builtin := c.convertBuiltin(call); !builtin && !c.isConversionToBang(call) {
			return false
		}
	}
	return c.isRecognisedSource(e)
}

// addressTaken returns the variables of the file whose address is taken as `&x`
// (parentheses ignored), closures included. Flow-insensitive and syntactic (P1).
func (c *Checker) addressTaken() map[types.Object]bool {
	if c.addrTaken != nil {
		return c.addrTaken
	}
	c.addrTaken = make(map[types.Object]bool)
	if c.info == nil {
		return c.addrTaken
	}
	ast.Inspect(c.file, func(n ast.Node) bool {
		if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.AND {
			if id, ok := unparenExpr(u.X).(*ast.Ident); ok {
				if obj := c.info.Uses[id]; obj != nil {
					c.addrTaken[obj] = true
				}
			}
		}
		return true
	})
	return c.addrTaken
}

// boundHint is the sentence that a GN001 on a local bound by inference ends with, or ""
// when the target is not such a local.
func (c *Checker) boundHint(lhs ast.Expr) string {
	if c.info == nil || c.inferred == nil {
		return ""
	}
	id, ok := unparenExpr(lhs).(*ast.Ident)
	if !ok {
		return ""
	}
	obj := c.info.ObjectOf(id)
	pos, bound := c.inferred[obj]
	if obj == nil || !bound {
		return ""
	}
	p := c.fset.Position(pos)
	return fmt.Sprintf("; %s was bound non-nil by its initialiser at %d:%d: declare it with a written type (var %s T = ...) or convert the initialiser ((T)(e)) to keep it nilable",
		id.Name, p.Line, p.Column, id.Name)
}

// isAppendRecognised is the rule of append (RFC C13): with at least one explicit element
// the result is never nil; a spread keeps the recognition of its base; append(x) is x.
func (c *Checker) isAppendRecognised(call *ast.CallExpr) bool {
	switch {
	case len(call.Args) == 0:
		return false
	case len(call.Args) == 1:
		return c.isRecognisedSource(call.Args[0])
	case call.Ellipsis == token.NoPos:
		return true
	default:
		return c.isRecognisedSource(call.Args[0])
	}
}
