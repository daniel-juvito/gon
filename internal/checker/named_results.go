package checker

import "go/ast"

// Rule C2 of the RFC on local backdoors (accepted 2026-11-25, strict): in a function that
// has a named result declared `!T`, a `return` with no operands is GN002. A named result
// holds its zero value, nil, until it is assigned, and a bare return hands it back as it
// is. The checker does not follow the order of statements (rule C1 treats the result as a
// `!` variable from the start of the body), so it cannot tell whether the result was
// assigned on every path to the return; it asks for `return x` instead.

// firstNamedBangResult returns the name of the first named result declared `!T` in
// results (`_` counts: a bare return gives back its zero value as well), or "" when no
// result is both named and `!`.
func (c *Checker) firstNamedBangResult(results *ast.FieldList) string {
	if results == nil {
		return ""
	}
	for _, field := range results.List {
		if !c.isNonNil(field.Type) {
			continue
		}
		for _, name := range field.Names {
			return name.Name
		}
	}
	return ""
}
