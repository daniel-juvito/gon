package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/daniel-juvito/gon/internal/gna"
)

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Ecosystem contract expansion (v1.5 / M4a + M4b + M5a).
//
// Executable counterpart of docs/rfc-ecosystem-contract-expansion.md.
//
//   - M4a: external `.gna` `types:` field contracts applied at construction
//     (keyed GN001/GN002, incl. `new(pkg.T)` and one-hop embedded promotion),
//     mutation (`x.F = nil` GN001), and use (`x.F` is a non-nil source).
//   - M4b: interface-typed external positions carry rfc-interface-semantics.md
//     (`!I` = interface value non-nil; ordinary interface → GN001, D3b).
//   - M5a: a `.gna` file is validated against the real package — signature
//     arity must match go/types (mismatch → GN003, contract dropped) and every
//     annotated symbol must exist (absent → GW004).
//
// External `types:` resolution is exact: it matches on the resolved
// *types.Named's package path, type name, and field name. The local
// name-only field heuristic is never carried across the boundary.

// namedOf unwraps pointers and returns the *types.Named of t, or nil.
func namedOf(t types.Type) *types.Named {
	switch u := t.(type) {
	case *types.Pointer:
		return namedOf(u.Elem())
	case *types.Named:
		return u
	default:
		return nil
	}
}

// isLocalStructName reports whether name is a struct declared in the file
// under check (collected into c.structFields). Such types are handled by the
// local field-contract path; everything else is treated as external.
func (c *Checker) isLocalStructName(name string) bool {
	_, ok := c.structFields[name]
	return ok
}

// externalTypeAnn returns the `.gna` `types:` annotation for the external
// named type, resolved by exact import path (with a package-name fallback for
// the overlay/local-test case, matching resolveMethodParams). It never emits a
// diagnostic; malformed `.gna` is reported on the use path and by
// validateAnnotations.
func (c *Checker) externalTypeAnn(named *types.Named) *gna.TypeAnnotation {
	if named == nil || c.resolver == nil {
		return nil
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return nil
	}
	pkgPath := obj.Pkg().Path()
	file, err := c.resolvePackage(pkgPath)
	if (err != nil || file == nil) && obj.Pkg().Name() != "" && obj.Pkg().Name() != pkgPath {
		file, err = c.resolvePackage(obj.Pkg().Name())
	}
	if err != nil || file == nil || file.Types == nil {
		return nil
	}
	return file.Types[obj.Name()]
}

// externalFieldNonNil reports whether fieldName carries a `!` claim on the
// external named type: via the type's own `.gna` `types:` entry, or (one hop,
// E6) an embedded external type's entry. Exact match only.
func (c *Checker) externalFieldNonNil(named *types.Named, fieldName string) bool {
	if named == nil {
		return false
	}
	if ann := c.externalTypeAnn(named); ann != nil && ann.Fields[fieldName] {
		return true
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Embedded() {
			continue
		}
		if en := namedOf(f.Type()); en != nil {
			if ann := c.externalTypeAnn(en); ann != nil && ann.Fields[fieldName] {
				return true
			}
		}
	}
	return false
}

// namedFromTypeExpr resolves a type expression (`pkg.T`, `T`) to its
// *types.Named via go/types, or nil.
func (c *Checker) namedFromTypeExpr(expr ast.Expr) *types.Named {
	if c.info == nil || expr == nil {
		return nil
	}
	if tv, ok := c.info.Types[expr]; ok && tv.Type != nil {
		if n := namedOf(tv.Type); n != nil {
			return n
		}
	}
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if obj := c.info.Uses[sel.Sel]; obj != nil {
			if n := namedOf(obj.Type()); n != nil {
				return n
			}
		}
	}
	if id, ok := expr.(*ast.Ident); ok {
		if obj := c.info.Uses[id]; obj != nil {
			if n := namedOf(obj.Type()); n != nil {
				return n
			}
		}
	}
	return nil
}

// selectorTypeAnnotation returns the `.gna` `types:` annotation for the type
// named by a `pkg.T` selector expression, resolved through the file's imports.
func (c *Checker) selectorTypeAnnotation(sel *ast.SelectorExpr) *gna.TypeAnnotation {
	if c.resolver == nil || sel == nil || sel.Sel == nil {
		return nil
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}
	pkgPath, ok := c.imports[x.Name]
	if !ok {
		return nil
	}
	file, err := c.resolvePackage(pkgPath)
	if err != nil || file == nil || file.Types == nil {
		return nil
	}
	return file.Types[sel.Sel.Name]
}

// structFieldType returns the go/types type of field fieldName on the named
// struct type (including promoted fields), or nil.
func structFieldType(named *types.Named, fieldName string) types.Type {
	if named == nil {
		return nil
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Name() == fieldName {
			return f.Type()
		}
		if f.Embedded() {
			if t := structFieldType(namedOf(f.Type()), fieldName); t != nil {
				return t
			}
		}
	}
	return nil
}

// selectorFieldType returns the go/types type of the field named by sel.
func (c *Checker) selectorFieldType(sel *ast.SelectorExpr) types.Type {
	if c.info == nil || sel == nil {
		return nil
	}
	if s, ok := c.info.Selections[sel]; ok && s.Type() != nil {
		return s.Type()
	}
	if tv, ok := c.info.Types[sel]; ok {
		return tv.Type
	}
	return nil
}

func isInterfaceType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// checkExternalKeyedFieldValues implements E1 + the E8 field-value slice of
// M4b: for a keyed `pkg.T{...}` literal, an explicit `nil` written for a `!`
// field is GN001, and an ordinary interface-typed value written for a `!`
// interface field is GN001 (D3b).
func (c *Checker) checkExternalKeyedFieldValues(lit *ast.CompositeLit, sel *ast.SelectorExpr) {
	ann := c.selectorTypeAnnotation(sel)
	if ann == nil {
		return
	}
	named := c.namedFromTypeExpr(sel)
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || !ann.Fields[key.Name] {
			continue
		}
		path := []string{sel.Sel.Name, key.Name}
		if isNilIdent(kv.Value) {
			c.addErrorTrace(kv.Value.Pos(), "GN001",
				fmt.Sprintf("cannot assign nil to non-nil field %s.%s", sel.Sel.Name, key.Name),
				&ContractTrace{Origin: compositeOrigin(lit), Path: path})
			continue
		}
		ft := structFieldType(named, key.Name)
		if isInterfaceType(ft) && c.cannotSatisfyBangInterface(kv.Value, ft) {
			c.addErrorTrace(kv.Value.Pos(), "GN001",
				fmt.Sprintf("cannot assign ordinary interface value to non-nil field %s.%s", sel.Sel.Name, key.Name),
				&ContractTrace{Origin: compositeOrigin(lit), Path: path})
		}
	}
}

// reportExternalEmbeddedFields implements the one-hop embedded promotion of
// E6: a `!` field of an embedded external type, left unset at a zero-value
// construction site for `pkg.T`, is GN002.
func (c *Checker) reportExternalEmbeddedFields(pos token.Pos, typeExpr ast.Expr, provided map[string]bool) {
	named := c.namedFromTypeExpr(typeExpr)
	if named == nil {
		return
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Embedded() {
			continue
		}
		if provided != nil && provided[f.Name()] {
			continue // embedded literal supplied; its own lit is checked separately
		}
		en := namedOf(f.Type())
		if en == nil {
			continue
		}
		ann := c.externalTypeAnn(en)
		if ann == nil {
			continue
		}
		for fn, nn := range ann.Fields {
			if nn {
				c.addError(pos, "GN002", fmt.Sprintf("missing required non-nil field %s.%s", en.Obj().Name(), fn))
			}
		}
	}
}

// validateAnnotations implements M5a (E10 + E11): check every resolved `.gna`
// file against the real package. Runs once per check; each condition is
// reported once per annotated symbol. Populates c.droppedContracts.
func (c *Checker) validateAnnotations() {
	if c.resolver == nil || len(c.importPkgs) == 0 {
		return
	}
	if c.droppedContracts == nil {
		c.droppedContracts = make(map[string]map[string]bool)
	}
	for _, path := range sortedKeys(c.importPkgs) {
		tpkg := c.importPkgs[path]
		if tpkg == nil || tpkg.Scope() == nil {
			continue
		}
		// Best-effort (§4.4.3): only validate packages go/types actually
		// resolved. An unresolved import yields an incomplete package, or —
		// via the fallback importer — a "complete" stub with an empty scope.
		// Either way its .gna is applied unvalidated, exactly as before v1.5.
		if !tpkg.Complete() || tpkg.Scope().Len() == 0 {
			continue
		}
		file, err := c.resolvePackage(path)
		if err != nil || file == nil {
			continue
		}
		c.validateGnaFile(path, file, tpkg)
	}
}

func (c *Checker) markDropped(pkgPath, symbol string) {
	if c.droppedContracts == nil {
		c.droppedContracts = make(map[string]map[string]bool)
	}
	if c.droppedContracts[pkgPath] == nil {
		c.droppedContracts[pkgPath] = make(map[string]bool)
	}
	c.droppedContracts[pkgPath][symbol] = true
}

func (c *Checker) contractDropped(pkgPath, symbol string) bool {
	m := c.droppedContracts[pkgPath]
	if m == nil {
		return false
	}
	return m[symbol]
}

func (c *Checker) validateGnaFile(pkgPath string, file *gna.File, tpkg *types.Package) {
	pos := c.importPos[pkgPath]
	scope := tpkg.Scope()

	for _, name := range sortedKeys(file.Functions) {
		sig := file.Functions[name]
		obj := scope.Lookup(name)
		fn, isFunc := obj.(*types.Func)
		if obj == nil || !isFunc {
			c.addWarning(pos, "GW004",
				fmt.Sprintf(".gna for %s annotates %s, which the package does not provide", pkgPath, name))
			continue
		}
		if s, ok := fn.Type().(*types.Signature); ok {
			c.checkArity(pkgPath, name, s, sig, pos)
		}
	}

	for _, key := range sortedKeys(file.Methods) {
		sig := file.Methods[key]
		typeName, methodName, ok := splitMethodKey(key)
		if !ok {
			continue
		}
		named := lookupNamed(scope, typeName)
		if named == nil {
			c.addWarning(pos, "GW004",
				fmt.Sprintf(".gna for %s annotates %s, which the package does not provide", pkgPath, key))
			continue
		}
		m := lookupMethod(named, methodName)
		if m == nil {
			c.addWarning(pos, "GW004",
				fmt.Sprintf(".gna for %s annotates %s, which the package does not provide", pkgPath, key))
			continue
		}
		if s, ok := m.Type().(*types.Signature); ok {
			c.checkArity(pkgPath, key, s, sig, pos)
		}
	}

	for _, typeName := range sortedKeys(file.Types) {
		if lookupNamed(scope, typeName) == nil {
			c.addWarning(pos, "GW004",
				fmt.Sprintf(".gna for %s annotates %s, which the package does not provide", pkgPath, typeName))
		}
	}
}

// checkArity compares the `.gna` params/results length against the Go
// signature (a variadic final parameter counts as one). On mismatch it emits
// GN003 and drops the whole entry.
func (c *Checker) checkArity(pkgPath, symbol string, s *types.Signature, sig *gna.Signature, pos token.Pos) {
	goParams := s.Params().Len()
	goResults := s.Results().Len()
	if len(sig.Params) != goParams {
		c.addError(pos, "GN003",
			fmt.Sprintf(".gna for %s: signature arity mismatch for %s (annotation %d params, package %d)",
				pkgPath, symbol, len(sig.Params), goParams))
		c.markDropped(pkgPath, symbol)
		return
	}
	if len(sig.Results) != goResults {
		c.addError(pos, "GN003",
			fmt.Sprintf(".gna for %s: signature arity mismatch for %s (annotation %d results, package %d)",
				pkgPath, symbol, len(sig.Results), goResults))
		c.markDropped(pkgPath, symbol)
	}
}

func splitMethodKey(key string) (typeName, methodName string, ok bool) {
	i := strings.LastIndex(key, ".")
	if i <= 0 || i == len(key)-1 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}

func lookupNamed(scope *types.Scope, name string) *types.Named {
	obj := scope.Lookup(name)
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return nil
	}
	named, _ := tn.Type().(*types.Named)
	return named
}

// lookupMethod finds a method by name in the full method set of *T (which
// includes value-receiver and promoted methods).
func lookupMethod(named *types.Named, name string) *types.Func {
	ms := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < ms.Len(); i++ {
		if fn, ok := ms.At(i).Obj().(*types.Func); ok && fn.Name() == name {
			return fn
		}
	}
	return nil
}
