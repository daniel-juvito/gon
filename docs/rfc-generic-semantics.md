# RFC: Generic Semantics for Non-Nil Contracts (Gon v1.8 / M3)

**Status:** Draft — owner decisions O1–O7 accepted. Technical amendments
are incorporated below; implementation and audit remain pending. No
implementation or release is implied by these decisions.
**Target:** Gon v1.8
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`,
`docs/roadmap-v1.3-plus.md`, `docs/rfc-interface-semantics.md` (M1a / v1.4),
`docs/rfc-type-coverage.md` (M1b / v1.6),
`docs/rfc-element-contract-construction.md` (M2b / v1.7),
`docs/rfc-field-contracts.md`, `docs/rfc-return-value-contracts.md`,
`docs/rfc-ecosystem-contract-expansion.md`
**Date:** 2026-11-13

## 1. Summary

Gon has never specified how `!` interacts with Go generics. `docs/v1-scope.md`
and `docs/gna-spec-v1.md` list "generics / type-parameter nilability" as
deferred, and the roadmap gates M3 on M1a (interfaces, v1.4) and M1b's
named-type nilability semantics (v1.6) — both shipped.

The current behaviour is therefore **accidental**, not designed. Measured
against v1.7.1:

| Program | Today | Problem |
|---|---|---|
| `func Bad[X any](x !X)` | accepted | `X` may be `int`; `!` is meaningless |
| `func Z[X any]() { var v !X; _ = v }` | accepted | no GN002, no GN003 |
| `type Box[X any] struct{ v !*X }` + `Box[T]{}` | accepted | field contract lost through instantiation |
| `Box[T]{v: nil}` | accepted | GN001 lost through instantiation |
| `func NN[X any]() !*X` + `p := NN[T](); p == nil` | no GW001 | result contract lost through instantiation |
| `func Nil[X any]() !*X { return nil }` | GN001 | works — the body is checked |
| `[]!*T{nil}` inside a generic type's instantiation | GN001 | works — literal is concrete |

So `!` already works wherever the **outermost constructor is concrete**
(`!*X`, `![]X`, `[]!*X`) *inside a generic body*, and silently disappears at
every **instantiation boundary**. This RFC fixes that and defines the one
genuinely new question: `!X` where `X` is a type parameter.

**Normative principle:**

> A contract is attached to a *declaration position*, never to a type
> argument. Instantiation substitutes types; it never adds, removes, or
> carries a `!`. A `!` on a bare type parameter is valid only when every type
> in the parameter's type set is nilable, so the contract is meaningful for
> all instantiations.

## 2. Motivation

Generic containers and helpers are exactly where nil contracts are most
useful and most easily lost:

```go
type Registry[K comparable, V any] struct {
	items map[K]!*V      // field contract — must survive Registry[string, Svc]
}

func Must[V any](p *V) !*V { … }        // result contract — callers should see a non-nil source
func Ptr[P ~*E, E any](p !P) { … }     // contract on a type-parameter-typed parameter
```

Without this RFC, `Registry[string, Svc]{}` gets no GN002 and `Must[Svc](x)`
gets no GW001, which makes `!` unreliable the moment code is generic.
Deferring further would also block M5b (generic `.gna`, v1.9), which
depends on this application model.

## 3. Terminology

- **Type set** — the set of types a type parameter's constraint permits, as
  represented by `go/types`.
- **Nilable kind** — pointer, slice, map, channel, function, interface
  (underlying kind), as in v1.4 / v1.6.
- **Nilable type parameter** — a type parameter whose constraint contains at
  least one specific type term and whose every specific term has a nilable
  underlying kind (e.g. `~*T`, `~[]E`, `*A | *B`). Method terms do not
  independently establish nilability. A constraint consisting only of method
  terms (including `any`, `comparable`, and `fmt.Stringer`-style
  constraints) is not nilable.

  For this rule the checker conservatively examines the specific terms
  exposed by the constraint's type-set expression. It does not need to prove
  that the intersection of all constraint elements is non-empty. If there is
  no specific term, or if any specific term has a non-nilable underlying
  kind, the type parameter is not nilable.
- **Instantiation boundary** — an explicit instantiation (`F[T]`, `Box[T]`)
  or an inferred call (`F(x)`).

## 4. Decision Matrix

### G1 — Concrete outer constructor is already well-defined

`!*X`, `![]X`, `!map[K]V`, `!chan X`, `!func(X)`, and element forms
`[]!*X`, `map[K]!*V` mean exactly what they mean today, for any `X`. The
outermost constructor is concrete, so the contract is meaningful for every
instantiation. **No change in semantics; the checker now applies them through
instantiation (G3, G5).**

### G2 — `!X` on a bare type parameter

`!X` is accepted **iff `X` is a nilable type parameter** (§3). Otherwise
**GN003** ("`!` on type parameter X: constraint `any` does not restrict X to
a nilable kind"). The check is made once, on the generic declaration, from
the constraint — never per instantiation. Method terms do not make an
otherwise unrestricted type parameter nilable; specific terms determine
admissibility under the conservative rule in §3. This guarantees `!X` is
meaningful for every possible type argument, so no instantiation can fail on
a contract it did not declare.

Alternatives (rejected): forbid `!X` entirely (loses `func Ptr[P ~*E, E any](p !P)`);
defer to instantiation like C++ templates (errors at a distance from the
declaration, violates the "check the declaration" model of every earlier RFC).

### G3 — Instantiated generic types keep their field contracts

A struct field's `!` is positional in the declaration. The generic
declaration establishes the contract position; instantiated use sites
resolve that position through type substitution and apply the existing
construction, mutation, and selector rules.

Thus `Box[T]{}` and `Box[T]{v: nil}` are checked against `Box`'s
declaration: zero construction → GN002, `nil` → GN001, selector use →
non-nil source (GW001). This applies to explicit instantiation (`Box[T]`),
inferred instantiation, nested instantiation (`Box[Box[T]]`), and embedded
instantiated fields. A `!X` field (G2) is enforced on instantiation with a
nilable type argument (guaranteed by G2).

These instantiated use-site checks do not re-check the generic declaration
body or perform monomorphisation. They resolve declared contract positions
and apply the relevant existing rules at the concrete use site.

### G4 — `!` is not allowed in type-argument position

`Box[!*T]`, `F[!*T](…)` → **GN003** (reserved). A type argument's identity
is `go/types`' (I3); `Box[!*T]` and `Box[*T]` must be the same type, so `!`
cannot ride along. Consequently a type parameter never *carries* a contract:
`func Id[X any](x X) X` called with a `!*T` argument returns a plain `*T`
(I6, no propagation).

### G5 — Calls to generic functions and methods

Contracts on a generic function's parameters and results are resolved
**through the instantiation** (explicit or inferred): `NN[T]()` and `NN(x)`
both expose `NN`'s `!*X` result as a non-nil source; passing `nil` to a
`!*X` / `!P` parameter → GN001.

After substitution, the existing parameter-contract compatibility rules
apply to the declared parameter position. Type inference and the caller's
argument type do not promote an ordinary value into a non-nil source, nor
create, remove, or transfer a contract. The contract is supplied solely by
the generic declaration's parameter or result annotation (G4).

Methods of generic types resolve via the instantiated receiver
(`(*Box[X]).Get() !*X`).

### G6 — Bodies are checked once, opaquely

A generic function or method body is checked a single time with type
parameters opaque. No monomorphisation and no per-instantiation body
re-check (I1, I3).

This rule governs checks inside generic bodies. It does not prohibit the
instantiated use-site checks required by G3 or G5: those resolve contract
positions from the generic declaration and apply existing rules to
construction, assignment, selector, and call sites, without re-analysing
the generic body.

Inside the body:

- `var v !X` with no initializer → GN002 (X nilable by G2);
- `nil` assigned/returned/written where `!X` / `!*X` is required → GN001;
- `x == nil` on a `!`-typed value → GW001, as elsewhere.

### G7 — Zero-value rule on generic declarations

C5 (v1.6) extends uniformly: bare `var x !S` is GN002 whenever `S`'s
underlying kind is nilable, including a nilable type parameter. `var p !*X`
is already GN002 today and stays so.

### G8 — Generic interfaces and method-set contracts

Interface method-set contracts (v1.4) apply over the instantiated method
set obtained from `go/types`: `type Getter[X any] interface{ Get() !*X }` is
satisfied by a type whose method has the `!` result under the v1.4 rules,
with `X` substituted.

This is an additional Gon contract validation. It does not modify Go
interface satisfaction, method identity, type identity, or assignability.
`go/types` remains authoritative for the instantiated method set and Go
signatures; Gon resolves the `!` annotations from the origin declarations
and applies the existing v1.4 contract rules.

### G9 — Constraints are untouched

`!` inside a constraint or type-set term (`~*T`, `interface{ ~[]!*E }`,
`P ~!*T`) → **GN003** (reserved). Constraints are type-level (I3); contracts
are value-level. Nilability of a type parameter is *read from* the
constraint (G2), never *written into* it.

### G10 — `.gna` and external generics

Generic `.gna` declarations and generic external-contract application
remain **out of scope** (M5b, v1.9). v1.8 behaviour for a `.gna` entry
naming a generic symbol is pinned by tests and remains as in v1.7.1. No
`.gna` schema bump is introduced (I7).

This deferral applies only to generic contracts originating in `.gna`. It
does not disable or defer source-level generic contracts written as `!` in Go
declarations; G3 and G5 apply to those in v1.8. The application model
defined here is what M5b will reuse.

### G11 — Compatibility classification

| Sub-feature | Classification |
|---|---|
| G2 `!X` on non-nilable type parameter → GN003 | **Breaking** (previously accepted) |
| G3 field contracts through instantiation (GN002/GN001/GW001) | Breaking in practice for code that violates an already-documented contract and was missed |
| G5 call-site contracts through instantiation | Same as G3 |
| G4, G9 `!` in type-argument / constraint position → GN003 | Breaking only for code that was already meaningless |
| G1, G6, G7, G8 | Non-breaking (already checked, or new on previously-rejected input) |

By R1 this ships **solo** as v1.8. v1.7.1 is the precedent for how
"documented-but-unchecked" gaps are treated: surfaced, called out in the
CHANGELOG, not versioned as a semantic change.

### G12 — Non-Goals

1. Contract-carrying type arguments (`Box[!*T]`) — G4.
2. Per-instantiation re-checking / monomorphisation — G6.
3. Inferring contracts from bodies or from argument types — I2, I6.
4. `!` in constraints — G9.
5. Variance, subtyping between `Box[*T]` and `Box[!*T]` — no such types exist.
6. Generic `.gna` and generic external contract application — M5b / v1.9.
7. Flow-sensitive nilability inside generic bodies — I1.
8. Runtime checks or any code generation change — I5.
9. A new `nilable` constraint keyword or predeclared constraint — would be
   new syntax; out of scope.

## 5. Normative Rules

1. **Declaration-positional.** A `!` binds to the type expression at which it
   is written (outermost-binding rule, C10 / E-series). Instantiation
   substitutes types and leaves contracts where they were declared.
2. **`!X` admissibility.** For a type parameter `X`, `!X` is valid iff every
   term of `X`'s constraint type set has a nilable underlying kind, and the
   type set has at least one specific term. Else GN003.
3. **Application through instantiation.** Wherever a rule fires on a
   declared contract (GN001, GN002, GW001, interface method-set check), it
   fires identically on any instantiation of the declaring generic type,
   function, or method.
4. **Type arguments and constraints carry no `!`.** GN003.
5. **One check per declaration.** Generic bodies are checked once.
6. **No propagation** of a contract through type-argument substitution
   (I6).

## 6. Examples

```go
// G2
func Ptr[P ~*E, E any](p !P) {}        // OK  — P's type set is {~*E}: nilable
func Slc[S ~[]E, E any](s !S) {}       // OK
func Bad[X any](x !X) {}               // GN003 — any is not nilable
func Cmp[X comparable](x !X) {}        // GN003

// G3
type Box[X any] struct{ v !*X }
var a Box[T]                           // GN002 — v zero-valued
var b = Box[T]{v: nil}                 // GN001
var c = Box[T]{v: &T{}}                // OK

// G4
var d Box[!*T]                         // GN003 — ! in type-argument position

// G5
func NN[X any]() !*X { return new(X) }
func main() {
	p := NN[T]()
	if p == nil {}                     // GW001
	q := Id(p)                         // Id[*T] — result is plain *T: not a source
	if q == nil {}                     // no GW001 (G4 / I6)
}

// G6
func Z[P ~*E, E any]() {
	var v !P                           // GN002 — no initializer
	_ = v
}
```

## 7. Compatibility

See G11. No `.gna` schema bump (I7); no new diagnostic codes — GN001, GN002,
GN003, GW001 are reused. `gon fmt` is unaffected (`!` re-insertion is
offset-driven and does not depend on generics; to be confirmed by a fixture).

## 8. Implementation Sketch

- **Type-parameter nilability** — new `typeParamNilable(*types.TypeParam)` in
  `internal/checker/type_coverage.go`: walk `tp.Constraint().Underlying()` as
  a `*types.Interface`; flatten embedded unions via `EmbeddedType(i)` /
  `*types.Union` terms; true iff there is ≥1 specific term and every term's
  `Underlying()` is pointer / slice / map / chan / signature / interface.
  Extend `nilableRefKind` for `*types.TypeParam`. Wire into
  `checkNonNilTypeKinds` (GN003) and `reportZeroValueNilableVar` (G7).
- **Instantiation resolution** — unwrap `*ast.IndexExpr` /
  `*ast.IndexListExpr` in `resolveCallResults`, `resolveCallParams`, and
  composite-literal type resolution so the generic declaration's AST
  (`*ast.FuncDecl` / `*ast.TypeSpec` with `TypeParams`) is reached. For
  inferred calls use `info.Instances[ident]`; the declaration is identified
  by the `*types.Func` / `*types.TypeName` origin (`Origin()`), not by
  the instantiated type.
- **Field contracts** — `nonNilOffsets` / `reportMissingNonNilFields` /
  field-assignment checks key on the **origin** struct, so `Box[T]` resolves
  to `Box`'s struct AST. Embedded generic fields: same via the embedded
  field's origin.
- **Type-argument / constraint `!`** — `checkNonNilTypeKinds` gains a pass
  over `IndexExpr.Index` / `IndexListExpr.Indices` and `TypeParams` constraint
  expressions; any `!` → GN003. The preprocessor must not mis-strip `!` in
  type-argument lists (`F[!*T]`); verify offsets for `gon fmt`.
- **Generic interfaces** — run the v1.4 method-set contract check on
  `types.Named` instances using `types.NewMethodSet` over the instantiated
  type, comparing against the origin declaration's annotated methods.
- **Docs** — `docs/v1-scope.md`, `docs/gna-spec-v1.md` (generic section:
  still deferred to v1.9, but point to this RFC), roadmap v1.8 entry,
  README, CHANGELOG, `gon version` → `1.8.0`.

## 9. Test Requirements

Executable spec in `internal/checker/generics_test.go`; fixtures under
`testdata/{valid,invalid}`.

1. G2: `~*E`, `~[]E`, `*A | *B`, `~map[K]V`, `~func()`, interface-kind term
   accepted; `any`, `comparable`, `fmt.Stringer`-style method-only, mixed
   `*A | int`, `~int` → GN003.
2. G3: `Box[T]{}` GN002; `Box[T]{v: nil}` GN001; `Box[T]{v: &T{}}` OK;
   nested `Box[Box[T]]`; embedded `Box[T]`; selector `b.v == nil` GW001;
   inferred-instantiation constructors (`func NewBox[X any]() Box[X]`).
3. G4: `Box[!*T]`, `F[!*T]()` GN003; `Id(nonNil)` result is not a source.
4. G5: `NN[T]()` and inferred `NN(x)` results are non-nil sources; `nil`
   argument to `!*X` / `!P` parameter GN001; generic method results via
   instantiated receiver.
5. G6: body-level `var v !P` GN002; `return nil` GN001; body checked once
   (diagnostic count independent of the number of instantiations).
6. G8: generic interface method-set contract satisfied / violated, per the
   v1.4 matrix.
7. G9: `!` in a constraint → GN003.
8. G10: `.gna` naming a generic symbol — behaviour pinned (same as v1.7.1).
9. Regression: every existing v1.1–v1.7 test passes unchanged;
   non-generic programs produce byte-identical diagnostics.
10. `gon fmt` round-trip preserves `!` in every generic position above.

## 10. Owner Decisions

Owner decisions O1–O7 are accepted as recorded below. The RFC remains Draft
pending implementation and audit.

1. **O1 — G2: ACCEPT (a)**, nilable type parameters only, as defined by §3.
2. **O2 — G2: ACCEPT.** Interface-kind specific terms count as nilable,
   consistent with v1.4. Admissibility is determined conservatively from
   specific terms: at least one specific term must exist and every specific
   term must have a nilable underlying kind. Method terms alone do not
   establish nilability.
3. **O3 — G4: ACCEPT GN003.** `!` in a type argument is rejected; it is not
   silently stripped.
4. **O4 — G3/G5: ACCEPT v1.8.** Instantiation gaps are addressed as part of
   M3; no v1.7.2 semantic patch is planned.
5. **O5 — G9: ACCEPT GN003 reserved.** Any future constraint-level
   nilability marker requires a separate RFC and is not part of M3.
6. **O6 — G10 / release shape: ACCEPT.** M3 ships solo as v1.8; generic
   `.gna` application remains M5b / v1.9.
7. **O7 — G5: ACCEPT.** Inferred generic calls apply declared parameter and
   result contracts identically to explicit instantiations.

These owner decisions settle the alternatives originally listed in O1–O7.
They do not constitute implementation approval or release sign-off.

## 11. Alternatives Considered

- **C++-style instantiation-time check** — reports errors where the generic
  is *used*, not where the contract is written; contradicts the
  declaration-checked model of v1.1–v1.7.
- **`!` as part of type identity** (`Box[!*T] ≠ Box[*T]`) — violates I3;
  requires a parallel type system.
- **Contract inference through type arguments** — violates I2 and I6.
- **A `nilable` predeclared constraint** — useful but is new syntax and
  deserves its own RFC; G2 reads the same fact from existing constraints.

## 12. Invariant Check (I1–I7)

- **I1** — upheld: G6 checks bodies once, no flow state; no path-dependent
  nilability.
- **I2** — upheld: contracts only from explicit `!` (G12.3).
- **I3** — upheld: type-set computation, instantiation, identity, and method
  sets all from `go/types` (G4, G8, §8).
- **I4** — upheld: no `.gna` change; G10 defers it.
- **I5** — upheld: no runtime or codegen change (G12.8).
- **I6** — upheld: G4 — no contract carried by or through type arguments.
- **I7** — upheld: no syntax added to `.gna`; schema stays **1**.

---

**Decisions (summary) — owner-accepted O1–O7**

| Decision | Proposal |
|----------|----------|
| `!*X` / `![]X` / `[]!*X` meaning unchanged; applied through instantiation | Yes (G1, G3, G5) |
| `!X` on a bare type parameter | Only if every constraint term is nilable; else GN003 (G2, O1–O2: accepted) |
| Field contracts of generic structs through `Box[T]` | Enforced as declared (G3) |
| `!` in type arguments (`Box[!*T]`) | GN003, reserved (G4, O3) |
| Generic call results / params contracts through instantiation | Yes, explicit and inferred (G5, O7) |
| Generic bodies | Checked once, opaque (G6) |
| Generic interface method-set contracts | v1.4 rules on instantiated method sets (G8) |
| `!` in constraints | GN003, reserved (G9, O5) |
| Generic `.gna` | Deferred to v1.9 / M5b (G10) |
| Schema bump | No (stay 1) |
| New diagnostic codes | None |
| Release shape | Solo v1.8 (G11, O6) |
