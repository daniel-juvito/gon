package checker

import (
	"fmt"
	"go/ast"
	"go/types"
)

// Type Coverage (v1.6 / M1b).
//
// Executable counterpart of docs/rfc-type-coverage.md. `!` is given a single
// uniform meaning across every nilable Go kind: it constrains the *reference
// value* (slice header, map, channel, function value, pointer) to be non-nil,
// and says nothing about backing data, length, or channel state.
//
// Most of the surface already worked accidentally: the preprocessor strips `!`
// positionally regardless of the following type, the checker carries a generic
// nonNilOffsets set, GN001 already fires for a literal nil into any `!`
// position, and GN002 already fires for a `!` field of any kind left at its
// zero value. v1.6 adds two enforcement points and generalises a third:
//
//   C5 / §4.4 — a bare `var x !S` of a nilable reference kind with no
//               initializer is GN002 (the reference value is nil at the zero
//               value). Closes the pre-existing silent `!*T` gap. Interface
//               types are excluded (owned by M1a, rfc-interface-semantics.md).
//   O4 / §4.8 — `!` on a named type whose underlying kind is not nilable
//               (struct, array, basic) is a malformed contract: GN003.
//   C7 / §4.6 — `x.(!T)` for ANY target → GN001 (generalised from the v1.4
//               interface-only rule; lives in construction.go).

// nilableRefKind reports whether `!` on typeExpr constrains a reference value
// whose zero value is nil — pointer, slice, map, chan, or func. Interface types
// are deliberately excluded (RFC §4.4). Named types and aliases resolve through
// go/types to their underlying kind; without type information a bare type name
// is treated conservatively as not-a-ref-kind (no GN002).
func (c *Checker) nilableRefKind(typeExpr ast.Expr) bool {
	switch t := typeExpr.(type) {
	case *ast.ParenExpr:
		return c.nilableRefKind(t.X)
	case *ast.StarExpr, *ast.MapType, *ast.ChanType, *ast.FuncType:
		return true
	case *ast.ArrayType:
		return t.Len == nil // slice: yes; fixed array: no
	case *ast.InterfaceType:
		return false
	case *ast.Ident, *ast.SelectorExpr:
		switch c.resolvedUnderlying(typeExpr).(type) {
		case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature:
			return true
		}
		return false
	}
	return false
}

// resolvedUnderlying returns the underlying go/types type of a named or
// selector type expression, or nil when type information is unavailable.
func (c *Checker) resolvedUnderlying(typeExpr ast.Expr) types.Type {
	if c.info == nil {
		return nil
	}
	if tv, ok := c.info.Types[typeExpr]; ok && tv.Type != nil {
		return tv.Type.Underlying()
	}
	switch e := typeExpr.(type) {
	case *ast.Ident:
		if obj := c.info.Uses[e]; obj != nil && obj.Type() != nil {
			return obj.Type().Underlying()
		}
	case *ast.SelectorExpr:
		if obj := c.info.Uses[e.Sel]; obj != nil && obj.Type() != nil {
			return obj.Type().Underlying()
		}
	}
	return nil
}

// reportZeroValueNilableVar emits GN002 for a `var x !S` with no initializer
// when S is a nilable reference kind (RFC C5 / §4.4).
func (c *Checker) reportZeroValueNilableVar(names []*ast.Ident, typeExpr ast.Expr) {
	if typeExpr == nil || !c.nilableRefKind(typeExpr) {
		return
	}
	for _, name := range names {
		if name.Name == "_" {
			continue
		}
		c.addError(typeExpr.Pos(), "GN002", fmt.Sprintf(
			"non-nil variable %s declared without an initializer; the zero value of !%s is nil",
			name.Name, formatType(typeExpr)))
	}
}

// checkNonNilTypeKinds validates that every `!`-annotated type expression in
// the file names a kind on which a non-nil contract is meaningful — pointer,
// slice, map, channel, function, or interface. `!` on a struct, array, or
// basic type (directly or through a named type / alias) is GN003 (RFC O4 /
// §4.8). Without type information, named types degrade to accepted.
func (c *Checker) checkNonNilTypeKinds() {
	ast.Inspect(c.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			c.validateNonNilKind(x.Type)
			c.validateElementContracts(x.Type)
		case *ast.Field:
			c.validateNonNilKind(x.Type)
			c.validateElementContracts(x.Type)
		case *ast.TypeSpec:
			// `type Slots [4]!*Slot` — element `!` on a named type / alias.
			c.validateElementContracts(x.Type)
		}
		return true
	})
}

// validateElementContracts walks the element / value / key positions of a type
// expression and rejects malformed element `!` placements (v1.7 / M2b):
//
//   - `!` on a channel element type (`chan !*T`)      → GN003 (E11)
//   - `!` on a map key type (`map[!*K]V`)             → GN003 (O1 deferred)
//   - `!` on a non-nilable element kind (`[]!int`)    → GN003 (O4 rule)
//
// It recurses only through type-composition nodes that are not independently
// visited by checkNonNilTypeKinds (array/slice/map/chan element positions,
// pointer, parens). Struct fields and function parameters are `*ast.Field`
// and are validated directly by the visitor.
func (c *Checker) validateElementContracts(typeExpr ast.Expr) {
	switch t := typeExpr.(type) {
	case *ast.ParenExpr:
		c.validateElementContracts(t.X)
	case *ast.StarExpr:
		c.validateElementContracts(t.X)
	case *ast.ArrayType:
		if t.Elt != nil && c.isNonNil(t.Elt) && !c.nonNilKindMeaningful(t.Elt) {
			c.addError(t.Elt.Pos(), "GN003", fmt.Sprintf(
				"non-nil modifier ! is not valid on element type %s; ! applies only to pointer, slice, map, channel, function, and interface types",
				formatType(t.Elt)))
		}
		c.validateElementContracts(t.Elt)
	case *ast.MapType:
		if t.Key != nil && c.isNonNil(t.Key) {
			c.addError(t.Key.Pos(), "GN003",
				"non-nil modifier ! is not valid on a map key type; map key contracts are not supported in v1")
		}
		if t.Value != nil && c.isNonNil(t.Value) && !c.nonNilKindMeaningful(t.Value) {
			c.addError(t.Value.Pos(), "GN003", fmt.Sprintf(
				"non-nil modifier ! is not valid on element type %s; ! applies only to pointer, slice, map, channel, function, and interface types",
				formatType(t.Value)))
		}
		c.validateElementContracts(t.Key)
		c.validateElementContracts(t.Value)
	case *ast.ChanType:
		if t.Value != nil && c.isNonNil(t.Value) {
			c.addError(t.Value.Pos(), "GN003",
				"non-nil modifier ! is not valid on a channel element type; channel elements are runtime sends with no construction site")
		}
		c.validateElementContracts(t.Value)
	}
}

func (c *Checker) validateNonNilKind(typeExpr ast.Expr) {
	if typeExpr == nil || !c.isNonNil(typeExpr) {
		return
	}
	if c.nonNilKindMeaningful(typeExpr) {
		return
	}
	c.addError(typeExpr.Pos(), "GN003", fmt.Sprintf(
		"non-nil modifier ! is not valid on %s; ! applies only to pointer, slice, map, channel, function, and interface types",
		formatType(typeExpr)))
}

func (c *Checker) nonNilKindMeaningful(typeExpr ast.Expr) bool {
	switch t := typeExpr.(type) {
	case *ast.ParenExpr:
		return c.nonNilKindMeaningful(t.X)
	case *ast.StarExpr, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		return true
	case *ast.ArrayType:
		return t.Len == nil // slice ok; fixed array not
	case *ast.StructType:
		return false
	case *ast.Ident, *ast.SelectorExpr:
		u := c.resolvedUnderlying(typeExpr)
		if u == nil {
			return true // degrade to accepted without type info
		}
		switch u.(type) {
		case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
			return true
		}
		return false
	}
	return true
}
