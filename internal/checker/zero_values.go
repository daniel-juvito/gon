package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// Zero-valued containers of `!` elements (RFC cheap backdoors, rules A and B,
// accepted 2026-11-25). An array `[N]!E` (N a constant greater than 0, E a
// nilable kind) whose value is created at its zero value, and a slice made by
// `make([]!E, n)` with `n` other than the constant 0, hold nil elements: GN002,
// as a fixed-array literal that leaves an index uncovered already is (E5).

// typeSpecOf returns the declaration of the type that the identifier id names, found
// through go/types so that scope is respected: a type declared inside a function, a local
// type that shadows a package type, and a type of a sibling file are each the declaration
// that Go's own resolution picks. It is nil without type information (a name cannot be
// told from a shadowing one by its spelling) and for anything that is not a declared type.
func (c *Checker) typeSpecOf(id *ast.Ident) *ast.TypeSpec {
	if c.info == nil || id == nil {
		return nil
	}
	obj, ok := c.info.Uses[id].(*types.TypeName)
	if !ok || obj == nil {
		return nil
	}
	files := []*ast.File{c.file}
	if c.pkg != nil {
		files = append(files, c.pkg.files...)
	}
	var found *ast.TypeSpec
	for _, f := range files {
		if f == nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			if ts, ok := n.(*ast.TypeSpec); ok && ts.Name != nil && c.info.Defs[ts.Name] == obj {
				found = ts
				return false
			}
			return true
		})
		if found != nil {
			return found
		}
	}
	return nil
}

// arrayTypeOf resolves a type expression to an array or slice type literal: the
// literal itself, or the declaration of a named type whose underlying type is one,
// however long the chain of defined types and aliases (`type Slots [4]!*T; type More
// Slots; type Alias = More`), wherever the types are declared and whatever shadows
// them. It is nil for anything else and without type information. There is no depth
// limit: a declaration already visited ends the walk, so a declaration that is not
// valid Go cannot loop.
func (c *Checker) arrayTypeOf(e ast.Expr) *ast.ArrayType {
	seen := make(map[*ast.TypeSpec]bool)
	for e != nil {
		switch t := unparenExpr(genericOrigin(e)).(type) {
		case *ast.ArrayType:
			return t
		case *ast.Ident:
			ts := c.typeSpecOf(t)
			if ts == nil || seen[ts] {
				return nil
			}
			seen[ts] = true
			e = ts.Type
		default:
			return nil
		}
	}
	return nil
}

// zeroValuedArrayElements reports whether t is a fixed array of a constant length
// greater than 0 whose element carries `!` and is a nilable kind: its zero value
// holds nil elements. `[...]E` has no zero value of its own and `[0]E` no element.
func (c *Checker) zeroValuedArrayElements(t *ast.ArrayType) (int64, bool) {
	if t == nil || t.Len == nil {
		return 0, false
	}
	if _, ok := t.Len.(*ast.Ellipsis); ok {
		return 0, false
	}
	if !c.isNonNil(t.Elt) || !c.elemTypeIsNilable(t.Elt) {
		return 0, false
	}
	n, ok := c.constIntValue(t.Len)
	if !ok || n <= 0 {
		return 0, false
	}
	return n, true
}

// reportZeroValuedArray emits one GN002 for a zero-valued array of `!` elements
// (rule A). pathPrefix names the field that holds the array when it is reached
// through a struct. It reports whether it emitted.
func (c *Checker) reportZeroValuedArray(pos token.Pos, t *ast.ArrayType, pathPrefix string, trace *ContractTrace) bool {
	n, ok := c.zeroValuedArrayElements(t)
	if !ok {
		return false
	}
	arr := fmt.Sprintf("[%d]!%s", n, formatType(t.Elt))
	msg := fmt.Sprintf("zero-valued array %s leaves %d non-nil element(s) at nil", arr, n)
	if pathPrefix != "" {
		msg = fmt.Sprintf("zero-valued array field %s of type %s leaves %d non-nil element(s) at nil", pathPrefix, arr, n)
	}
	if trace != nil {
		tr := trace.Clone()
		if pathPrefix != "" {
			tr.Path = splitPath(pathPrefix)
		}
		c.addErrorTrace(pos, "GN002", msg, tr)
	} else {
		c.addError(pos, "GN002", msg)
	}
	return true
}

// checkMakeConstruction is rule B: `make([]!E, n)` and `make([]!E, n, cap)` create
// n elements at nil, so the length must be the constant 0 (the capacity is free:
// `make([]!*T, 0, n)` followed by `append` is the usual way to preallocate).
// Maps and channels have no elements at make time; a slice with no element `!` is
// not a subject of the rule.
func (c *Checker) checkMakeConstruction(call *ast.CallExpr) {
	if call == nil || len(call.Args) < 2 {
		return
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "make" {
		return
	}
	// Without type information a function of this package called `make` cannot be told
	// from the builtin, so the rule is silent rather than risk a wrong GN002.
	if c.info == nil {
		return
	}
	if _, isBuiltin := c.info.Uses[fun].(*types.Builtin); !isBuiltin {
		return
	}
	at := c.arrayTypeOf(call.Args[0])
	if at == nil || at.Len != nil {
		return // not a slice
	}
	if !c.isNonNil(at.Elt) || !c.elemTypeIsNilable(at.Elt) {
		return
	}
	if n, ok := c.constIntValue(call.Args[1]); ok && n == 0 {
		return
	}
	c.addErrorTrace(call.Pos(), "GN002", fmt.Sprintf(
		"make of a slice of non-nil elements %s with a length other than the constant 0 creates nil elements; make it with length 0 and append",
		"[]!"+formatType(at.Elt)), &ContractTrace{Origin: "make([]!" + formatType(at.Elt) + ")"})
}
