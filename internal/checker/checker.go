// Package checker performs Gon v1.0 nil-safety analysis on clean Go source.
package checker

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"

	"github.com/daniel-juvito/gon/internal/gna"
)

type Checker struct {
	fset          *token.FileSet
	file          *ast.File
	filename      string
	cleanSrc      []byte
	nonNilOffsets map[int]bool

	funcParams   map[string][]bool
	funcReturns  map[string][]bool
	structFields map[string]map[string]bool

	// scopes contain lexical bindings. The value is true only for !T bindings;
	// false is also stored so a nullable shadowing declaration hides an outer !T.
	scopes             []map[string]bool
	currentFuncReturns []bool

	// resolver supplies external package nilability contracts from .gna files.
	// Set via NewWithAnnotations; nil means no external annotations.
	resolver gna.Resolver
	// imports maps local import name -> package import path.
	imports map[string]string
	// info holds go/types selection data for method identity resolution.
	// nil when type-checking was skipped or failed.
	info *types.Info
	// resolved caches successful Resolve results for this check run.
	resolved map[string]*gna.File
	// resolvedMiss caches import paths known to be unannotated.
	resolvedMiss map[string]bool

	diagnostics []*Diagnostic
}

func New(filename string, cleanSrc []byte, nonNilOffsets map[int]bool) (*Checker, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, cleanSrc, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &Checker{
		fset:          fset,
		file:          file,
		filename:      filename,
		cleanSrc:      cleanSrc,
		nonNilOffsets: nonNilOffsets,
		funcParams:    make(map[string][]bool),
		funcReturns:   make(map[string][]bool),
		structFields:  make(map[string]map[string]bool),
	}, nil
}

func (c *Checker) Check() []*Diagnostic {
	c.collectDecls()
	c.pushScope() // package scope
	c.registerPackageVars()
	c.checkPackageLevelComposites()
	for _, decl := range c.file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok {
			c.checkFuncDecl(d)
		}
	}
	c.popScope()
	return c.diagnostics
}

func (c *Checker) isNonNil(typeExpr ast.Expr) bool {
	if typeExpr == nil {
		return false
	}
	offset := c.fset.Position(typeExpr.Pos()).Offset
	return c.nonNilOffsets[offset]
}

func (c *Checker) addError(pos token.Pos, code, msg string) {
	p := c.fset.Position(pos)
	c.diagnostics = append(c.diagnostics, &Diagnostic{Severity: SeverityError, Code: code, Message: msg, File: filepath.Base(p.Filename), Line: p.Line, Col: p.Column})
}

func (c *Checker) addWarning(pos token.Pos, code, msg string) {
	p := c.fset.Position(pos)
	c.diagnostics = append(c.diagnostics, &Diagnostic{Severity: SeverityWarning, Code: code, Message: msg, File: filepath.Base(p.Filename), Line: p.Line, Col: p.Column})
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func formatType(expr ast.Expr) string {
	if expr == nil {
		return "T"
	}
	switch t := expr.(type) {
	case *ast.StarExpr:
		return "*" + formatType(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + formatType(t.Elt)
		}
		return "[...]" + formatType(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", formatType(t.Key), formatType(t.Value))
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + formatType(t.Value)
		case ast.RECV:
			return "<-chan " + formatType(t.Value)
		default:
			return "chan " + formatType(t.Value)
		}
	case *ast.SelectorExpr:
		return formatType(t.X) + "." + t.Sel.Name
	case *ast.FuncType:
		return "func(...)"
	case *ast.InterfaceType:
		return "interface{...}"
	default:
		return "T"
	}
}

func (c *Checker) collectDecls() {
	for _, decl := range c.file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			c.collectGenDecl(d)
		case *ast.FuncDecl:
			c.collectFuncDecl(d)
		}
	}
}

func (c *Checker) collectGenDecl(d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			structType, ok := s.Type.(*ast.StructType)
			if !ok {
				continue
			}
			fields := make(map[string]bool)
			for _, field := range structType.Fields.List {
				isNN := c.isNonNil(field.Type)
				for _, name := range field.Names {
					fields[name.Name] = isNN
				}
			}
			c.structFields[s.Name.Name] = fields
		}
	}
}

func (c *Checker) collectFuncDecl(d *ast.FuncDecl) {
	var params []bool
	if d.Type.Params != nil {
		for _, field := range d.Type.Params.List {
			isNN := c.isNonNil(field.Type)
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				params = append(params, isNN)
			}
		}
	}
	c.funcParams[d.Name.Name] = params

	var returns []bool
	if d.Type.Results != nil {
		for _, field := range d.Type.Results.List {
			isNN := c.isNonNil(field.Type)
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				returns = append(returns, isNN)
			}
		}
	}
	c.funcReturns[d.Name.Name] = returns
}

func (c *Checker) registerPackageVars() {
	for _, decl := range c.file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			nn := vs.Type != nil && c.isNonNil(vs.Type)
			for _, name := range vs.Names {
				c.define(name.Name, nn)
			}
			if nn {
				for i, val := range vs.Values {
					if i < len(vs.Names) && isNilIdent(val) {
						c.addError(val.Pos(), "GN001", fmt.Sprintf("cannot assign nil to non-nil type !%s", formatType(vs.Type)))
					}
				}
			}
		}
	}
}

func (c *Checker) checkPackageLevelComposites() {
	for _, decl := range c.file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, val := range vs.Values {
				ast.Inspect(val, func(n ast.Node) bool {
					if cl, ok := n.(*ast.CompositeLit); ok {
						c.checkCompositeLit(cl)
					}
					return true
				})
			}
		}
	}
}

func (c *Checker) checkFuncDecl(d *ast.FuncDecl) {
	prev := c.currentFuncReturns
	c.currentFuncReturns = c.funcReturns[d.Name.Name]
	c.pushScope()

	if d.Recv != nil {
		for _, field := range d.Recv.List {
			nn := c.isNonNil(field.Type)
			for _, name := range field.Names {
				c.define(name.Name, nn)
			}
		}
	}
	if d.Type.Params != nil {
		for _, field := range d.Type.Params.List {
			nn := c.isNonNil(field.Type)
			for _, name := range field.Names {
				c.define(name.Name, nn)
			}
		}
	}

	if d.Body != nil {
		c.checkBlockScope(d.Body)
	}

	c.popScope()
	c.currentFuncReturns = prev
}

func (c *Checker) checkBlockScope(block *ast.BlockStmt) {
	c.pushScope()
	for _, stmt := range block.List {
		c.checkStmt(stmt)
	}
	c.popScope()
}

func (c *Checker) checkStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		c.checkBlockScope(s)
	case *ast.DeclStmt:
		if gd, ok := s.Decl.(*ast.GenDecl); ok {
			c.checkLocalVarDecl(gd)
		}
	case *ast.AssignStmt:
		c.checkAssignStmt(s)
	case *ast.ReturnStmt:
		c.checkReturnStmt(s)
	case *ast.ExprStmt:
		c.checkExpr(s.X)
	case *ast.IfStmt:
		if s.Init != nil {
			c.checkStmt(s.Init)
		}
		c.checkExpr(s.Cond)
		c.checkBlockScope(s.Body)
		if s.Else != nil {
			c.checkStmt(s.Else)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			c.checkStmt(s.Init)
		}
		c.checkExpr(s.Cond)
		c.checkBlockScope(s.Body)
		if s.Post != nil {
			c.checkStmt(s.Post)
		}
	case *ast.RangeStmt:
		c.pushScope()
		if s.Key != nil {
			c.registerRangeIdent(s.Key)
		}
		if s.Value != nil {
			c.registerRangeIdent(s.Value)
		}
		c.checkExpr(s.X)
		c.checkBlockScope(s.Body)
		c.popScope()
	default:
		ast.Inspect(s, func(n ast.Node) bool {
			if e, ok := n.(ast.Expr); ok {
				c.checkExpr(e)
				return false
			}
			return true
		})
	}
}

func (c *Checker) registerRangeIdent(expr ast.Expr) {
	if id, ok := expr.(*ast.Ident); ok && id.Name != "_" {
		c.define(id.Name, false)
	}
}

func (c *Checker) checkLocalVarDecl(d *ast.GenDecl) {
	if d.Tok != token.VAR {
		return
	}
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		nn := vs.Type != nil && c.isNonNil(vs.Type)
		for i, name := range vs.Names {
			c.define(name.Name, nn)
			if nn && i < len(vs.Values) && isNilIdent(vs.Values[i]) {
				c.addError(vs.Values[i].Pos(), "GN001", fmt.Sprintf("cannot assign nil to non-nil type !%s", formatType(vs.Type)))
			}
		}
	}
}

func (c *Checker) checkAssignStmt(assign *ast.AssignStmt) {
	for i, lhs := range assign.Lhs {
		if i >= len(assign.Rhs) {
			break
		}
		if id, ok := lhs.(*ast.Ident); ok {
			if nn, exists := c.lookup(id.Name); exists && nn && isNilIdent(assign.Rhs[i]) {
				c.addError(assign.Rhs[i].Pos(), "GN001", fmt.Sprintf("cannot assign nil to non-nil variable !%s", id.Name))
			}
		}
	}
	for _, rhs := range assign.Rhs {
		c.checkExpr(rhs)
	}
}

func (c *Checker) checkReturnStmt(ret *ast.ReturnStmt) {
	for i, result := range ret.Results {
		if i < len(c.currentFuncReturns) && c.currentFuncReturns[i] && isNilIdent(result) {
			c.addError(result.Pos(), "GN001", "cannot return nil from function with non-nil return type")
		}
	}
}

func (c *Checker) checkCallExpr(call *ast.CallExpr) {
	params, display := c.resolveCallParams(call.Fun)
	if params == nil {
		return
	}
	// Method expressions (T.M(recv, args...)) pass the receiver as args[0].
	// .gna params describe only the declared method parameters, not the receiver.
	argOffset := 0
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && c.info != nil {
		if s, found := c.info.Selections[sel]; found && s.Kind() == types.MethodExpr {
			argOffset = 1
		}
	}
	for i, arg := range call.Args {
		pi := i - argOffset
		if pi >= 0 && pi < len(params) && params[pi] && isNilIdent(arg) {
			c.addError(arg.Pos(), "GN001", fmt.Sprintf("cannot pass nil as non-nil argument %d to %s", pi+1, display))
		}
	}
}

func (c *Checker) checkCompositeLit(lit *ast.CompositeLit) {
	typeName := ""
	switch t := lit.Type.(type) {
	case *ast.Ident:
		typeName = t.Name
	default:
		return
	}
	fields, ok := c.structFields[typeName]
	if !ok {
		return
	}

	provided := make(map[string]bool)
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		provided[key.Name] = true
		if fields[key.Name] && isNilIdent(kv.Value) {
			c.addError(kv.Value.Pos(), "GN001", fmt.Sprintf("cannot assign nil to non-nil field %s.%s", typeName, key.Name))
		}
	}
	for name, nn := range fields {
		if nn && !provided[name] {
			c.addError(lit.Pos(), "GN002", fmt.Sprintf("struct literal of %s missing required non-nil field %s", typeName, name))
		}
	}
}

func (c *Checker) checkBinaryExpr(expr *ast.BinaryExpr) {
	if expr.Op != token.EQL && expr.Op != token.NEQ {
		return
	}
	var x ast.Expr
	if isNilIdent(expr.X) {
		x = expr.Y
	} else if isNilIdent(expr.Y) {
		x = expr.X
	} else {
		return
	}
	id, ok := x.(*ast.Ident)
	if !ok {
		return
	}
	nn, exists := c.lookup(id.Name)
	if !exists || !nn {
		return
	}
	if expr.Op == token.EQL {
		c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("%s is non-nil; comparison with nil is always false", id.Name))
	} else {
		c.addWarning(expr.Pos(), "GW001", fmt.Sprintf("%s is non-nil; comparison with nil is always true", id.Name))
	}
}

func (c *Checker) checkExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		switch x := n.(type) {
		case *ast.CallExpr:
			c.checkCallExpr(x)
		case *ast.CompositeLit:
			c.checkCompositeLit(x)
		case *ast.BinaryExpr:
			c.checkBinaryExpr(x)
		}
		return true
	})
}

func (c *Checker) pushScope() { c.scopes = append(c.scopes, make(map[string]bool)) }
func (c *Checker) popScope() {
	if len(c.scopes) > 0 {
		c.scopes = c.scopes[:len(c.scopes)-1]
	}
}
func (c *Checker) define(name string, nn bool) {
	if name != "_" && len(c.scopes) > 0 {
		c.scopes[len(c.scopes)-1][name] = nn
	}
}
func (c *Checker) lookup(name string) (bool, bool) {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if v, ok := c.scopes[i][name]; ok {
			return v, true
		}
	}
	return false, false
}
