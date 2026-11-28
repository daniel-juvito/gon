package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

// gon convert, the analysis (RFC Part B with gon convert, accepted 2026-11-27).
//
// Part B binds a local `x := e` as `!T` when `e` has a structural non-nil form. That
// breaks correct Go when the variable is later given a value that is not one
// (`x = nil`, `x = f()`), or when its address is taken. The plan below finds those
// variables, in one pass over the file and without following control flow, and says
// where to write `x := (T)(e)` so that the variable stays nilable. A conversion without
// `!` is not a recognised source, so the binding does not apply to it.
//
// Nothing here changes what Check reports: the plan is computed from the same type
// information and the same notion of a recognised source.

// ConvertEdit inserts Text into the original source at byte offset Offset.
type ConvertEdit struct {
	Offset int
	Text   string
}

// ConvertReport is a case the plan cannot rewrite and says so, with its position in
// the original source (1-based line and column).
type ConvertReport struct {
	Line, Column int
	Message      string
}

// ConvertPlan is the result of PlanConvert: the edits to apply to the original
// source (in the order they must be applied, by offset), the reports, and how many
// locals the binding would keep bound and how many it would rewrite.
type ConvertPlan struct {
	Edits     []ConvertEdit
	Reports   []ConvertReport
	Bound     int
	Kept      int              // locals rewritten to stay nilable
	Converted []ConvertedLocal // the locals rewritten, for the gate of gon convert
}

// ConvertedLocal names a local that the plan rewrites to stay nilable, with the
// position (1-based) of the variable, which is the position that the hint of its
// diagnostics names.
type ConvertedLocal struct {
	Name         string
	Line, Column int
}

type convertCandidate struct {
	obj types.Object
	id  *ast.Ident
	rhs ast.Expr
}

// PlanConvert computes the plan for the file of this checker. Check must have been run
// (it collects the declarations that the recognition of a call or a field needs).
// cleanToOrig maps offsets of the clean source to offsets of the original, as
// preproc.Result.CleanToOrig does.
func (c *Checker) PlanConvert(cleanToOrig []int) ConvertPlan {
	var plan ConvertPlan
	if c.info == nil {
		return plan
	}
	bang := c.convertDeclaredBang()
	cands := c.convertCandidates(bang)

	// Fixpoint: a candidate stays bound until a use of it breaks; breaking one can make
	// another stop being a candidate (it was initialised from the first), or break
	// (it was assigned from the first).
	bound := make(map[types.Object]bool)
	for _, cd := range cands {
		bound[cd.obj] = true
	}
	demoted := make(map[types.Object]bool)
	uses := c.convertUses(cands)
	for changed := true; changed; {
		changed = false
		for _, cd := range cands {
			if !bound[cd.obj] {
				continue
			}
			// No longer a candidate: its initialiser stopped being a recognised source.
			if !c.convertRecognised(cd.rhs, bang, bound) {
				bound[cd.obj] = false
				changed = true
				continue
			}
			if c.convertBreaks(cd, uses[cd.obj], bang, bound) {
				bound[cd.obj] = false
				demoted[cd.obj] = true
				changed = true
			}
		}
	}
	for _, cd := range cands {
		if bound[cd.obj] {
			plan.Bound++
			continue
		}
		if !demoted[cd.obj] {
			continue
		}
		text, ok := c.convertTypeText(cd.obj.Type())
		pos := c.fset.Position(cd.id.Pos())
		if !ok {
			plan.Reports = append(plan.Reports, ConvertReport{
				Line: pos.Line, Column: pos.Column,
				Message: fmt.Sprintf("cannot write the type of %s (%s) in this file; make it nilable by hand", cd.id.Name, cd.obj.Type()),
			})
			continue
		}
		start := c.fset.Position(cd.rhs.Pos()).Offset
		end := c.fset.Position(cd.rhs.End()).Offset
		if start < 0 || end <= start || end > len(cleanToOrig) {
			continue
		}
		plan.Edits = append(plan.Edits,
			ConvertEdit{Offset: cleanToOrig[start], Text: "(" + text + ")("},
			ConvertEdit{Offset: cleanToOrig[end-1] + 1, Text: ")"})
		plan.Kept++
		plan.Converted = append(plan.Converted, ConvertedLocal{Name: cd.id.Name, Line: pos.Line, Column: pos.Column})
	}
	// Apply order: by offset; at one offset the opening edits keep the order in which
	// they were found (an outer one first), which a stable sort preserves.
	sort.SliceStable(plan.Edits, func(i, j int) bool { return plan.Edits[i].Offset < plan.Edits[j].Offset })
	return plan
}

// ApplyConvert returns src with the edits of a plan applied.
func ApplyConvert(src []byte, edits []ConvertEdit) []byte {
	var out strings.Builder
	last := 0
	for _, e := range edits {
		if e.Offset < last || e.Offset > len(src) {
			continue
		}
		out.Write(src[last:e.Offset])
		out.WriteString(e.Text)
		last = e.Offset
	}
	out.Write(src[last:])
	return []byte(out.String())
}

// convertDeclaredBang returns the variables declared `!T` in the file: parameters,
// results, receivers and `var` declarations with a written `!` type.
func (c *Checker) convertDeclaredBang() map[types.Object]bool {
	bang := make(map[types.Object]bool)
	ast.Inspect(c.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			if c.isNonNil(x.Type) {
				for _, name := range x.Names {
					if obj := c.info.Defs[name]; obj != nil {
						bang[obj] = true
					}
				}
			}
		case *ast.ValueSpec:
			if x.Type != nil && c.isNonNil(x.Type) {
				for _, name := range x.Names {
					if obj := c.info.Defs[name]; obj != nil {
						bang[obj] = true
					}
				}
			}
		}
		return true
	})
	return bang
}

// convertCandidates finds the new locals that Part B would bind: a `:=` or untyped
// `var` that declares a variable of a pointer, slice, map, channel or function kind
// and whose initialiser is a recognised source of a form that is not already bound
// by today's rules (a call whose result is declared `!T` is).
func (c *Checker) convertCandidates(bang map[types.Object]bool) []convertCandidate {
	var cands []convertCandidate
	known := make(map[types.Object]bool)
	add := func(id *ast.Ident, rhs ast.Expr) {
		if id == nil || id.Name == "_" || rhs == nil {
			return
		}
		obj := c.info.Defs[id]
		if obj == nil || !nilableNonInterfaceKind(obj.Type()) {
			return
		}
		cands = append(cands, convertCandidate{obj: obj, id: id, rhs: rhs})
		known[obj] = true
	}
	// Found in source order, so that an initialiser that names an earlier candidate
	// sees it.
	ast.Inspect(c.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE && len(x.Lhs) == len(x.Rhs) {
				for i, l := range x.Lhs {
					id, _ := l.(*ast.Ident)
					if id != nil && c.convertNewForm(x.Rhs[i], bang, known) {
						add(id, x.Rhs[i])
					}
				}
			}
		case *ast.ValueSpec:
			if x.Type == nil && len(x.Names) == len(x.Values) {
				for i, id := range x.Names {
					if c.convertNewForm(x.Values[i], bang, known) {
						add(id, x.Values[i])
					}
				}
			}
		}
		return true
	})
	return cands
}

// convertNewForm reports whether e is a recognised source of a form that Part B binds
// and today's rules do not: a call whose result is declared `!T` is already bound.
func (c *Checker) convertNewForm(e ast.Expr, bang, bound map[types.Object]bool) bool {
	e = unparenExpr(e)
	if call, ok := e.(*ast.CallExpr); ok {
		if _, builtin := c.convertBuiltin(call); !builtin && !c.isConversionToBang(call) {
			return false // an ordinary call: bound today when its result is !T, never new
		}
	}
	return c.convertRecognised(e, bang, bound)
}

func (c *Checker) isConversionToBang(call *ast.CallExpr) bool {
	_, ok := c.nonNilConversionTarget(call)
	return ok
}

// convertBuiltin returns the name of the builtin new, make or append that call is.
func (c *Checker) convertBuiltin(call *ast.CallExpr) (string, bool) {
	id, ok := unparenExpr(call.Fun).(*ast.Ident)
	if !ok || c.info == nil {
		return "", false
	}
	b, ok := c.info.Uses[id].(*types.Builtin)
	if !ok {
		return "", false
	}
	switch b.Name() {
	case "new", "make", "append":
		return b.Name(), true
	}
	return "", false
}

// convertRecognised is the recognition of a non-nil source of Part A and Part B (RFC
// 6.6, 6.8), by objects instead of scopes: an identifier is a source when it is a
// variable declared `!T` or one that the plan still keeps bound.
func (c *Checker) convertRecognised(e ast.Expr, bang, bound map[types.Object]bool) bool {
	e = unparenExpr(e)
	switch x := e.(type) {
	case *ast.CompositeLit, *ast.FuncLit:
		return true
	case *ast.UnaryExpr:
		return x.Op == token.AND
	case *ast.SliceExpr:
		return c.convertRecognised(x.X, bang, bound)
	case *ast.Ident:
		obj := c.info.Uses[x]
		return obj != nil && (bang[obj] || bound[obj])
	case *ast.SelectorExpr:
		return c.selectorFieldIsNonNil(x)
	case *ast.CallExpr:
		if name, builtin := c.convertBuiltin(x); builtin {
			switch name {
			case "new", "make":
				return true
			case "append":
				switch {
				case len(x.Args) == 0:
					return false
				case len(x.Args) == 1:
					return c.convertRecognised(x.Args[0], bang, bound)
				case x.Ellipsis == token.NoPos:
					return true // at least one explicit element: never nil
				default:
					return c.convertRecognised(x.Args[0], bang, bound)
				}
			}
		}
		if c.isConversionToBang(x) {
			return true
		}
		return c.isNonNilSource(x)
	}
	return false
}

// convertUse is one way a candidate is used that can break its binding.
type convertUse struct {
	kind string // "assign" (with rhs), "tuple", "addr", "range"
	rhs  ast.Expr
	lhs  ast.Expr
}

// convertUses records, for each candidate, the uses that can break the binding.
func (c *Checker) convertUses(cands []convertCandidate) map[types.Object][]convertUse {
	want := make(map[types.Object]bool)
	for _, cd := range cands {
		want[cd.obj] = true
	}
	uses := make(map[types.Object][]convertUse)
	objOf := func(e ast.Expr) types.Object {
		id, ok := unparenExpr(e).(*ast.Ident)
		if !ok {
			return nil
		}
		if obj := c.info.Uses[id]; want[obj] {
			return obj
		}
		return nil
	}
	ast.Inspect(c.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok != token.ASSIGN {
				return true
			}
			for i, l := range x.Lhs {
				obj := objOf(l)
				if obj == nil {
					continue
				}
				if len(x.Lhs) != len(x.Rhs) {
					uses[obj] = append(uses[obj], convertUse{kind: "tuple", lhs: l})
				} else {
					uses[obj] = append(uses[obj], convertUse{kind: "assign", rhs: x.Rhs[i], lhs: l})
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.ASSIGN {
				for _, l := range []ast.Expr{x.Key, x.Value} {
					if l != nil {
						if obj := objOf(l); obj != nil {
							uses[obj] = append(uses[obj], convertUse{kind: "range", lhs: l})
						}
					}
				}
			}
		case *ast.UnaryExpr:
			if x.Op == token.AND {
				if obj := objOf(x.X); obj != nil {
					uses[obj] = append(uses[obj], convertUse{kind: "addr", lhs: x.X})
				}
			}
		}
		return true
	})
	return uses
}

// convertBreaks reports whether a use of the candidate would be an error, or a false
// claim, once the variable is bound `!`: its address is taken, it is a target of a
// range or of a multi-value assignment, or it is assigned `nil` or a value that is not a
// recognised source of its own type (`x = append(x, ...)` and `x = x[a:b]` are, by the
// rules of append and of the slice expression).
func (c *Checker) convertBreaks(cd convertCandidate, uses []convertUse, bang, bound map[types.Object]bool) bool {
	for _, u := range uses {
		switch u.kind {
		case "addr", "range", "tuple":
			return true
		case "assign":
			if isNilIdent(unparenExpr(u.rhs)) {
				return true
			}
			if !c.convertRecognised(u.rhs, bang, bound) {
				return true
			}
			rt := c.staticTypeOf(u.rhs)
			if rt == nil || !types.Identical(rt, cd.obj.Type()) {
				return true
			}
		}
	}
	return false
}

// convertTypeText writes a type as it can be written in this file: a type of this
// package without a qualifier, a type of an imported package with the name that the
// file gives to the import. It is false when a package of the type is not imported here.
func (c *Checker) convertTypeText(t types.Type) (string, bool) {
	local := c.localTypesPackage()
	ok := true
	text := types.TypeString(t, func(p *types.Package) string {
		if p == local {
			return ""
		}
		// A path imported under two names is written under the first of them in
		// alphabetical order, so that the output does not depend on map order.
		var names []string
		for name, path := range c.imports {
			if path == p.Path() {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			sort.Strings(names)
			return names[0]
		}
		ok = false
		return p.Name()
	})
	return text, ok
}
