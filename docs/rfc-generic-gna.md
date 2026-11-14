# RFC: Generic `.gna` and External Generic Contract Application (Gon v1.9 / M5b)

**Status:** Draft — awaiting owner review. Decisions X1–X11 are *proposals*;
§9 lists the questions that need an explicit owner answer. Depends on v1.8
(M3) owner confirmation of RFC `rfc-generic-semantics.md` §13.1.
**Target:** Gon v1.9
**Related:** `docs/gna-spec-v1.md`, `docs/roadmap-v1.3-plus.md`,
`docs/rfc-generic-semantics.md` (M3 / v1.8),
`docs/rfc-ecosystem-contract-expansion.md` (M4a/M4b/M5a / v1.5),
`docs/rfc-type-coverage.md` (M1b / v1.6),
`docs/rfc-interface-semantics.md` (M1a / v1.4)
**Date:** 2026-11-14

## 1. Summary

M3 (v1.8) specified generics for **source-level** `!` and deliberately left
generic `.gna` out (its G10): an instantiation of an *external* generic
(`pkg.F[T]`, `pkg.Box[T]`) is not resolved. M5b is its direct consumer: make
`.gna` contracts on external generic symbols behave exactly like local
generic contracts do in v1.8, and validate them against `go/types`.

Measured against the v1.8 branch, the picture is **not** "generics are
unsupported in `.gna`" — most of it already works by accident, and the rest
is inconsistent:

| Case (external generic with a `.gna` entry) | Today | Problem |
|---|---|---|
| Inferred call `slices.Clone(xs)`; nil arg to a `!` param; result as source | applied (GN001 / GW001) | works — matching is by name + position, type strings are documentation |
| Explicit instantiation `slices.Clone[[]int](nil)` | **not applied** | inconsistent with the inferred form; local `F[T](nil)` is applied since v1.8 |
| Method on an instantiated receiver `p.Load()` (`atomic.Pointer[T]`) | applied (GW001) | works — receiver resolved by origin type name |
| Composite literal / zero var of external generic `atomic.Pointer[T]{}` | no diagnostics | not resolved; also see the stack overflow fixed in `e5f42f8` |
| `.gna` string `"!E"` where `E` is a type parameter (`comparable`, `any`) | silently accepted | no nilability check — the `.gna` analogue of v1.8 G2 |
| `.gna` string `"!int"`, `"!string"` (non-nilable concrete kind) | silently accepted | `gna-spec-v1.md` promises GN003 (v1.6) but it is enforced for local source only |

So M5b is a **consistency and validation** release, not a new mechanism:

> A `.gna` contract is attached to a *declaration position* of the symbol's
> **origin** (generic) signature. Instantiation — explicit or inferred —
> substitutes types and never changes which positions carry a contract. A `!`
> on a position is valid only if the origin's type at that position is a
> nilable kind, or a type parameter restricted to nilable kinds (M3 G2).

## 2. Motivation

Generic APIs are exactly where ecosystem contracts matter and where
inconsistency hurts: `slices`, `maps`, `sync/atomic.Pointer[T]`,
`sync.OnceValue`, third-party container libraries. Today a user who writes
`slices.Clone(nil)` is warned but `slices.Clone[[]int](nil)` — the same call —
is not, and a `.gna` author who writes `"!E"` for an `any`-constrained `E`
gets no feedback that the claim is meaningless.

## 3. Terminology

- **Origin signature** — the generic declaration's signature from
  `go/types` (`types.Func` of the uninstantiated function, or the method of
  `Named.Origin()`), before any type arguments are substituted.
- **Position kind** — the `go/types` type at a `params` / `results` position
  of the origin signature: a type parameter, or a concrete type with an
  underlying kind.
- **Nilable type parameter** — as defined in `rfc-generic-semantics.md` §3
  (some constraint element is a non-empty union of all-nilable terms).

## 4. Decision Matrix

### X1 — Scope: consistency + validation, no new syntax

M5b adds **no `.gna` syntax and no schema bump** (I7). Type parameters are
not declared in `.gna` (no `typeparams:` key); strings such as `"![]E"`
keep their v1 meaning — documentation plus positional matching. The checker
never re-types a string (I4).

### X2 — Explicit instantiation resolves like inference

`pkg.F[T](…)`, `pkg.F[A, B](…)`, `pkg.Box[T]{…}`, `var x pkg.Box[T]`, and
`new(pkg.Box[T])` resolve to the same `.gna` entry as the inferred /
uninstantiated forms. The v1.8 restriction in `genericOrigin` (external
bases returned unchanged, M3 G10) is lifted. The lookup key is the origin
symbol name, exactly as for the inferred form.

### X3 — Validate `!` positions against the origin signature

For every `!` in a `functions:` / `methods:` `params` / `results` string,
resolve the origin signature's type at that position (variadic final
parameter = slice):

- **type parameter** → valid iff nilable (M3 G2); else **GN003**
  ("`.gna` `!` on type parameter E of F: constraint does not restrict it to
  nilable kinds").
- **concrete type** → valid iff the underlying kind is nilable (pointer,
  slice, map, chan, func, interface); else **GN003** (the v1.6 `.gna` rule
  `gna-spec-v1.md` already states, now enforced).
- **element positions** (`"[]!*E"`) → the same rule applied to the element
  type, per v1.7.

On GN003 the **whole entry is dropped** (none of its `!` flags apply),
exactly as M5a does for an arity mismatch — a stale or wrong annotation
must not shift a claim onto another position. `types:` field strings are
validated the same way against the origin struct's field types.

### X4 — Generic receivers and `types:` through instantiation

`methods: Pointer.Load` is keyed by the **origin** type name; resolution
through an instantiated receiver (`atomic.Pointer[T]`) uses
`Named.Origin()` identity (package path + name). `types:` field contracts
for a generic external struct apply at the E1–E14 sites (construction,
mutation, use) with one-hop traversal into the origin struct — the
instantiated field type is never consulted for nilability, only the
declaration position (M3 G3).

### X5 — No contract in type-argument position inside `.gna`

A `!` inside a type-argument position of a `.gna` string
(`"Box[!*T]"`, `"map[string]Box[!*T]"`) is a **load error** — the `.gna`
mirror of M3 G4. A `!` in element position of slice / array / map value
(`"[]!*T"`) remains legal (v1.7); a type-argument list is not an element
position.

### X6 — Type-parameter names in strings are documentation

The identifier after `!` (`E`, `K`, `V`, `T`) is never matched against the
symbol's declared type-parameter names; only the position matters (X3).
This keeps `.gna` stable across upstream renames of type parameters and
avoids per-version coupling.

### X7 — Methods with their own type parameters (Go 1.27) are out of scope

Contracts on methods that declare their own type parameters
(`func (b Box[X]) Map[Y any](…)`) are not defined here
(`roadmap-v1.3-plus.md` §5 note, 2026-11-14). A `.gna` `methods:` entry
naming such a method is validated by arity only, as today; X3's
type-parameter rule does not apply to method-level type parameters until a
dedicated RFC.

### X8 — Arity and symbol validation

M5a is unchanged and runs against the **origin** signature; type
parameters are not value parameters, so they never count toward `params`
arity (verified: `slices.Clone` annotated with one param matches).
Unknown symbol → GW004 as before.

### X9 — Compatibility classification

| Sub-feature | Classification |
|---|---|
| X2 explicit instantiation now applies contracts | Breaking in practice: newly reports on code already violating a `.gna` contract that the inferred form would have caught |
| X3 GN003 on a `.gna` `!` over a type parameter not nilable | Breaking for `.gna` files with a meaningless claim (previously accepted) |
| X3 GN003 on a `.gna` `!` over a non-nilable concrete kind | Breaking for `.gna` files that contradict `gna-spec-v1.md` (previously accepted); applies to **non-generic** symbols too |
| X4 `types:` through instantiated receivers | New on previously unchecked input |
| X5 `!` in a `.gna` type argument | Previously accepted or a load error depending on form; now uniformly a load error |
| X1, X6, X7, X8 | Non-breaking |

By R1 this ships **solo** as v1.9. Ecosystem exposure is the point of the
release: an entry dropped by X3 silently loses its contract, so the CHANGELOG
must list the diagnostic and the migration (fix or delete the position).

### X10 — Bundled annotations audit

Before release, every `.gna` under `annotations/` is run through X3.
Any entry the new validation drops is either corrected in the same
release or called out in the CHANGELOG. This is a release gate, not a
design decision.

### X11 — Non-Goals

1. New `.gna` syntax, `typeparams:`, or constraints in `.gna` — X1.
2. Matching type-parameter *names* between `.gna` and Go — X6.
3. Per-instantiation contracts (`Box[*T]` vs `Box[!*T]`) — M3 G4.
4. Contracts on method-level type parameters — X7.
5. Source-level method contracts (untracked for every receiver; separate
   RFC, `rfc-generic-semantics.md` §13.1).
6. Flow-sensitive nilability, body inference, runtime checks — I1, I2, I5.
7. A strict mode that promotes GW004 / GW002 to errors.

## 5. Normative Rules

1. **Declaration-positional.** A `.gna` contract attaches to a position of
   the origin signature; instantiation substitutes types only.
2. **Explicit = inferred.** Explicit instantiation resolves to the same
   entry as the inferred form (X2).
3. **Validity.** A `!` position is valid iff the origin's type there is a
   nilable kind or a nilable type parameter (X3). Otherwise GN003 and the
   entry is dropped.
4. **No `!` in `.gna` type-argument position** (X5).
5. **Names are documentation** (X6).
6. **go/types is authoritative** for origin identity, signatures, type
   parameters, and constraints (I3); `.gna` is authority only for the
   nilability claim (I4).

## 6. Examples

```yaml
# annotations/slices.gna
schema: 1
package: slices
functions:
  Clone:
    params:  ["![]E"]     # S ~[]E is nilable (slice) — valid; X3 reads the origin's S
    results: ["![]E"]
  Index:
    params:  ["![]E", "!E"]   # GN003: E is `comparable`, not restricted to nilable kinds
    results: ["!int"]         # GN003: int is not a nilable kind
```

```go
slices.Clone(nil)               // GN001 — inferred (already today)
slices.Clone[[]int](nil)        // GN001 — explicit (X2; not reported today)
s := slices.Clone[[]int](xs)
if s == nil {}                  // GW001
var p atomic.Pointer[T]
q := p.Load()                   // `methods: Pointer.Load` results ["!*T"] -> GW001 (X4)
```

## 7. Implementation Sketch

- **X2** — drop the "base must be an identifier" restriction in
  `genericOrigin` (`internal/checker/generics.go`) for selector bases; keep
  the `typeExprName` fix (`e5f42f8`). `reportExternalTypeFields` and the
  `.gna` call resolvers then receive the unwrapped `*ast.SelectorExpr`.
  Update the pinned G10 tests.
- **X3** — in `validateAnnotations` (`internal/checker/ecosystem.go`, the
  M5a pass), after the arity check, walk each `!` position with the origin
  `*types.Signature` (`types.Func.Type()` for functions;
  `Named.Origin()` method for methods). Classify the position type:
  `*types.TypeParam` → `typeParamNilable`; otherwise `nilableUnderlying`.
  On failure emit GN003 anchored at the import spec (as M5a does) and add
  the symbol to `droppedContracts`. Extend to `types:` field strings via
  the origin struct's fields.
- **X4** — resolve instantiated receivers with `Named.Origin()`; pin the
  existing `atomic.Pointer[T]` method behaviour with a test.
- **X5** — `parseTypeAnn` (`internal/gna/gna.go`): reject `!` found inside a
  `[...]` type-argument list that is not an array length / element
  position (distinguish `Name[` from `[N]`/`[]`).
- **X10** — a test that loads every bundled `annotations/**/*.gna` and
  asserts the validator drops nothing (or lists exactly the expected set).
- **Docs** — `gna-spec-v1.md`, `v1-scope.md`, roadmap, README, CHANGELOG,
  `gon version` → `1.9.0`; examples + editor fixture follow.

## 8. Test Requirements

Executable spec in `internal/checker/generic_gna_test.go` and
`internal/gna/gna_test.go`. External cases use temporary modules with real
generic packages (`slices`, `maps`, `sync/atomic`).

1. X2: explicit vs inferred equivalence for params (GN001), results (GW001),
   multi-type-parameter lists (`F[A, B]`), nested instantiation, literal /
   `var` / `new` of an external generic struct.
2. X3: `!E` over `any` / `comparable` / method-only constraint → GN003 +
   entry dropped; over `~[]E`, `~*T`, `*A | *B` → accepted; `!int`,
   `!string`, `!struct` → GN003; element positions; variadic final
   parameter; whole-entry drop (a second valid position in the same entry
   no longer applies).
3. X4: `methods:` on an instantiated receiver; `types:` on a generic
   external struct (construction GN002/GN001, mutation, use GW001); embedded
   generic one-hop.
4. X5: `"Box[!*T]"` load error; `"[]!*T"` and `"map[string]!*T"` still load.
5. X6: renaming type parameters in the `.gna` string changes nothing.
6. X7: a `.gna` entry for a method-level-type-parameter method is
   validated by arity only (skip when the toolchain cannot parse it).
7. X8: type parameters never count toward arity.
8. X10: bundled `annotations/` audit.
9. Regression: every v1.1–v1.8 test passes unchanged; non-generic
   `.gna` behaviour is byte-identical except the X3 kind-validation cases,
   which are listed explicitly.
10. No panic / non-termination on any instantiation shape (explicit nested,
    external, aliased) — a fuzz-style table over `typeExprName` and the
    resolvers.

## 9. Open Questions for the Owner

1. **Q1 — X2.** Resolve explicit instantiation of external generics like the
   inferred form (proposed), or keep explicit-instantiation unchecked?
2. **Q2 — X3 scope.** Enforce `.gna` kind validation (`!int` → GN003) for
   **all** symbols (proposed: it is what `gna-spec-v1.md` already
   promises, and splitting generic from non-generic would be perverse), only
   generic symbols, or defer the non-generic part to its own patch?
3. **Q3 — X3 failure mode.** Drop the whole entry on GN003 (proposed, M5a
   precedent), or drop only the offending position?
4. **Q4 — X7.** Confirm method-level type parameters (Go 1.27) are out of
   scope for v1.9 and get their own RFC.
5. **Q5 — X10.** Confirm the bundled-annotations audit is a release gate.
6. **Q6 — Release shape.** Solo v1.9 (proposed). Also decide whether
   official release binaries are pinned to one Go version before v1.9
   (`roadmap-v1.3-plus.md` §5 Go 1.27 note).
7. **Q7 — Prerequisite.** Confirm the two `rfc-generic-semantics.md` §13.1
   corrections (source-level method contracts untracked; no method-set
   contract). X4 relies on the same reading.

## 10. Alternatives Considered

- **`typeparams:` / constraints in `.gna`** — new syntax and a schema
  bump; duplicates what `go/types` already knows (violates I4).
- **Match type-parameter names** — couples `.gna` to upstream spelling and
  breaks on a rename with no semantic change.
- **Instantiated-signature validation** — validating against the
  substituted type would make a contract valid for `Clone[[]int]` and
  invalid for `Clone[map...]`; contracts would depend on type arguments,
  contradicting M3 G4.
- **Leave explicit instantiation unchecked** — preserves today's
  inconsistency; a user gets different answers for the same call.

## 11. Invariant Check (I1–I7)

- **I1** — upheld: no flow state; validation and resolution are positional.
- **I2** — upheld: nilability comes only from explicit `.gna` / `!`.
- **I3** — upheld: origin signatures, type parameters, constraints, and
  instantiation identity all from `go/types`.
- **I4** — upheld: `.gna` stays authority only for the nilability claim;
  strings are never re-typed; no type-system syntax added.
- **I5** — upheld: no runtime or codegen change.
- **I6** — upheld: no contract carried through type arguments (X5, X6).
- **I7** — upheld: no new syntax, schema stays **1**.

---

**Proposed decisions (summary)**

| Decision | Proposal |
|----------|----------|
| New `.gna` syntax / schema bump | None (X1) |
| Explicit external instantiation resolves like inferred | Yes (X2, Q1) |
| `!` positions validated against the origin signature | Yes — type parameter via nilable rule, concrete via kind (X3, Q2) |
| Failure mode | GN003, drop whole entry (X3, Q3) |
| Generic receivers / `types:` | Resolved by origin identity (X4) |
| `!` inside a `.gna` type argument | Load error (X5) |
| Type-parameter names in strings | Documentation only (X6) |
| Method-level type parameters (Go 1.27) | Out of scope (X7, Q4) |
| Bundled annotations audit | Release gate (X10, Q5) |
| Release shape | Solo v1.9 (X9, Q6) |
