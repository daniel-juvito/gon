package checker

import (
	"go/ast"
	"go/types"
)

// Call-site contracts for source-declared methods (v1.11 / M7,
// docs/rfc-method-contracts.md).
//
// A `!` written on a method's parameter or result is enforced at the callers,
// like a plain function's. A call is matched to its declaration through
// go/types object identity (Selection.Obj(), then Origin()), never by name, so
// two types sharing a method name, or a method sharing a plain function's name,
// cannot collide. A method value carries nothing (S6): only a call that
// go/types resolves to a method declaration is checked.

// sourceMethod is the contract written on one method (or interface method)
// declaration.
type sourceMethod struct {
	params  []bool
	results []bool
	display string // "Type.Method", for diagnostics
}

// hasContract reports whether any position carries a `!`.
func (m *sourceMethod) hasContract() bool {
	for _, f := range m.params {
		if f {
			return true
		}
	}
	for _, f := range m.results {
		if f {
			return true
		}
	}
	return false
}

// sourceMethodContracts indexes, by object, the methods declared in the files
// being checked together: the one file, or every file of the package in package
// mode. Built once, on first use.
func (c *Checker) sourceMethodContracts() map[*types.Func]*sourceMethod {
	if c.methodContracts != nil {
		return c.methodContracts
	}
	c.methodContracts = make(map[*types.Func]*sourceMethod)
	if c.info == nil {
		return c.methodContracts
	}
	files := ([]*ast.File)([]*ast.File{c.file})
	if c.pkg != nil {
		files = c.pkg.files
	}
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					c.indexMethodDecl(d)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						c.indexInterfaces(ts.Type, ts.Name.Name)
					}
				}
			}
		}
		// Interface types written anywhere else (a parameter type, a field).
		ast.Inspect(f, func(n ast.Node) bool {
			if it, ok := n.(*ast.InterfaceType); ok {
				c.indexInterfaces(it, "interface")
			}
			return true
		})
	}
	return c.methodContracts
}

func (c *Checker) indexMethodDecl(d *ast.FuncDecl) {
	obj, ok := c.info.Defs[d.Name].(*types.Func)
	if !ok {
		return
	}
	c.methodContracts[obj] = &sourceMethod{
		params:  nonNilFlags(c, d.Type.Params),
		results: nonNilFlags(c, d.Type.Results),
		display: recvTypeName(d.Recv) + "." + d.Name.Name,
	}
}

// indexInterfaces indexes the methods of t if it is an interface type. An
// interface already indexed under a named TypeSpec keeps that name.
func (c *Checker) indexInterfaces(t ast.Expr, name string) {
	it, ok := t.(*ast.InterfaceType)
	if !ok || it.Methods == nil {
		return
	}
	for _, field := range it.Methods.List {
		ft, ok := field.Type.(*ast.FuncType)
		if !ok {
			continue // an embedded interface or a type-set term
		}
		for _, id := range field.Names {
			obj, ok := c.info.Defs[id].(*types.Func)
			if !ok {
				continue
			}
			if _, done := c.methodContracts[obj]; done {
				continue
			}
			c.methodContracts[obj] = &sourceMethod{
				params:  nonNilFlags(c, ft.Params),
				results: nonNilFlags(c, ft.Results),
				display: name + "." + id.Name,
			}
		}
	}
}

// recvTypeName is the receiver's type name without `*` or type arguments.
func recvTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	e := recv.List[0].Type
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}

// sourceMethodFor returns the contract written on the declaration that the
// method call `sel` resolves to, if that declaration is in the checked files
// and carries at least one `!`. A declaration without any `!` is treated as
// absent so the `.gna` path (a local type annotated by package) keeps working
// exactly as before.
func (c *Checker) sourceMethodFor(sel *ast.SelectorExpr) (*sourceMethod, bool) {
	if c.info == nil {
		return nil, false
	}
	s, found := c.info.Selections[sel]
	if !found || (s.Kind() != types.MethodVal && s.Kind() != types.MethodExpr) {
		return nil, false
	}
	fn, ok := s.Obj().(*types.Func)
	if !ok {
		return nil, false
	}
	m, ok := c.sourceMethodContracts()[fn.Origin()]
	if !ok || !m.hasContract() {
		return nil, false
	}
	return m, true
}
