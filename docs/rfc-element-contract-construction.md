# RFC: Element-Contract Construction (Gon v1.7 / M2b)

**Status:** Accepted  
**Target:** Gon v1.7  
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`, `docs/roadmap-v1.3-plus.md`,
`docs/rfc-type-coverage.md` (M1b / v1.6), `docs/rfc-field-contracts.md` (M2a),
`docs/rfc-interface-semantics.md`, `docs/rfc-return-value-contracts.md`,
`docs/rfc-ecosystem-contract-expansion.md`  
**Date:** 2026-09-06  
**Accepted:** 2026-09-07 (owner review; E1–E12 locked, O1/O4/O5 deferred)

## 1. Summary

Gon v1.6 (Type Coverage / M1b) gave `!` a uniform meaning on every nilable
reference kind (`![]T`, `!map[K]V`, `!chan T`, `!func(…)`, named aliases of
those kinds). It deliberately reserved **element-position** `!` — forms such
as `[]!*T`, `map[string]!V`, `[N]!*T` — as a parse / load error (GN003).
That reservation is documented in Type Coverage C10 and in
`docs/gna-spec-v1.md`.

This RFC activates element-position contracts and defines the **construction
subcases** required to make them useful:

- parse and represent element `!` in local Gon type expressions and in
  `.gna` type strings;
- check composite-literal elements (and fixed-array zero-fill) against the
  element contract at the construction site;
- keep the existing outermost-binding rule for a leading `!` so that
  `![]!*T` is well-defined and compositional;
- preserve every semantic invariant (I1–I7) and the non-flow character of
  the checker.

Element contracts are **construction-time obligations**, not lifetime
storage invariants of the container. After a `[]!*T` value exists, later
mutation (`s[i] = nil`, `append`, map assignment) is out of scope for this
RFC; that remains ordinary Go and is not tracked (I1).

**Normative principle:**

> An element contract constrains the values written into a composite
> literal (or the zero-fill of a fixed array) at the construction site.
> It does not turn the container into a flow-sensitive or lifetime
> invariant, and it does not descend through indirection after the
> construction expression has been evaluated.

## 2. Motivation

Real APIs frequently want “a non-nil slice of non-nil pointers” or “a map
whose values are non-nil handles”:

```go
func NewHandlers() []!*Handler { … }
func Register(m map[string]!*Service) { … }
```

Authors can already write the *outer* contract (`![]*Handler`) after v1.6.
They cannot yet write the *element* contract. The only options today are
comments or runtime panics. M2b closes that gap with the same local,
syntax-driven style used for field contracts (v1.2) and type coverage
(v1.6).

M2b is deliberately a **follow-on** to M1b (roadmap R2). The hard semantic
choices about reference-value meaning, zero-value declarations, and the
outermost-binding rule were settled in v1.6; this RFC only adds the
element-position cases that were reserved.

## 3. Goals

1. Accept `!` in element / value position of array, slice, and map types
   (local AST and `.gna` strings).
2. At a composite-literal construction site whose element type carries `!`,
   reject an explicit `nil` element (GN001) and treat a missing fixed-array
   index that would be zero-filled as GN002 when the element type is a
   nilable kind under `!`.
3. Preserve the outermost-binding rule: a leading `!` still binds only the
   outermost constructor; element `!` is independent and may be combined
   (`![]!*T`).
4. Keep the conservative ordinary→`!` acceptance rule (C4 of Type Coverage)
   for element positions as well.
5. No schema bump (I7). No flow analysis (I1). No lifetime tracking of
   elements after construction.
6. Full regression coverage for the new construction subcases; existing
   M1b / M2a behaviour unchanged.

## 4. Non-Goals

- Lifetime / storage invariants for slice or map elements after the
  construction expression (no tracking of `s[i] = …`, `append`, map
  assignment, or range).
- Length, capacity, or non-emptiness contracts.
- Channel element contracts (send values) — channels remain reference-value
  only; element contracts on channel types are rejected.
- Function parameter / result element contracts beyond ordinary type
  notation (already covered by positional `.gna` matching once the type
  string is accepted).
- Flow-sensitive or path-sensitive reasoning.
- Inference of element contracts from function bodies.
- Tightening the ordinary→`!` rule (ordinary values remain accepted into
  `!` element positions; they do not become non-nil sources).
- Schema version bump.
- Generic type-parameter element contracts (owned by M3 / v1.8).

## 5. Decision Matrix

Each row records the **locked** decision unless noted. The normative rules
in §6 MUST NOT introduce semantics beyond these decisions.

### E1 — Which type constructors admit element contracts

| Constructor | Element `!` allowed? | Rationale |
|-------------|----------------------|-----------|
| Slice `[]E` | Yes | Primary use case; composite literal elements are local. |
| Array `[N]E` / `[…]E` | Yes | Fixed size; zero-fill is a local construction fact. |
| Map `map[K]V` | Yes (value side only) | Map value is the natural element; key contracts are rare and deferred. |
| Channel `chan E` | **No** | Channel “elements” are runtime sends; no local construction site. |
| Function / pointer / interface | N/A | No element position in the type constructor. |

**LOCKED.** Channel element `!` remains GN003.

### E2 — Meaning of an element contract

An annotation `[]!*T` (or `[N]!*T`, `map[K]!*T`) means:

> Every **explicitly written** element of a composite literal of that type
> must satisfy the element contract `!*T` at the construction site.

It does **not** mean:

- the slice/map/array is itself non-nil (that is the outer `![]…` contract);
- later mutation cannot introduce a nil element;
- range, index, or append results carry the element contract.

**LOCKED.** Construction-time only.

### E3 — Outermost binding remains compositional

```
![]*T     → outer reference non-nil; element ordinary *T
[]!*T     → outer ordinary; element !*T
![]!*T    → both
!map[K]!V → both (outer map + value)
```

A leading `!` continues to bind only the outermost constructor (Type
Coverage C10). Element `!` is an independent annotation on the element
type expression. Nested forms such as `[][]!*T` are accepted and mean
“slice of (slice of `!*T`)”; only the innermost element position is
constrained by the inner `!`.

**LOCKED.** Compositional, no new binding rules.

### E4 — Composite-literal element check (GN001)

At a composite literal whose type’s element (or map value) carries `!`:

**Normative algorithm:**

> If the element expression is the untyped nil literal, emit **GN001**.
> Otherwise accept it.

Non-nil-source classification is **not** required to accept an element; it
remains relevant only to existing outer-contract analysis. Ordinary
(unannotated) expressions are accepted (conservative, same as Type Coverage
C4). A keyed map entry `k: nil` where the value type is `!V` is GN001.

This deliberately accepts cases such as:

```go
func f() *T { return maybeNil() }
x := []!*T{f()}   // accepted — ordinary expression, not literal nil
```

Unkeyed and keyed slice/array forms are treated uniformly for the element
check.

**LOCKED.**

### E5 — Fixed-array zero-fill (GN002)

For a fixed array type `[N]E` where `E` carries `!` and `E` is a nilable
kind, coverage is defined by **index**, not by element count:

- An **unkeyed** element at position *i* in the literal covers index *i*.
- A **keyed** element `k: e` covers index *k* (where *k* is a constant
  integer in range).
- Any index in `[0, N)` that is not explicitly covered is zero-filled.
- If the element type is `!` + nilable, the literal receives **one GN002**
  when at least one such index is missing.

`[...]E` has its length determined by the literal itself and therefore has
**no shortfall**; an inferred-length array literal never produces GN002
under this rule.

Variable-length slices and maps have no zero-fill of elements; an empty
`[]!*T{}` or `map[K]!*T{}` is fine (the container may still be constrained
by an outer `!`).

**LOCKED.** One GN002 per array literal that has any uncovered index.
Slices/maps have no element zero-fill diagnostic.

### E6 — Interaction with outer reference contracts

Outer and element contracts are independent:

```go
var s ![]!*T = nil          // GN001 (outer)
var s ![]!*T = []!*T{}      // OK (outer satisfied by literal; no elements)
var s ![]!*T = []!*T{nil}   // GN001 (element)
var s []!*T = []!*T{nil}    // GN001 (element)
var s ![]!*T                // GN002 (outer zero value) — already v1.6
```

**LOCKED.** No interaction beyond independent application of the existing
outer rules and the new element rules.

### E7 — Index, append, make, conversion, assertion

None of these become element-contract sources or construction sites for
element obligations:

| Expression | Element contract carried? |
|------------|---------------------------|
| `s[i]`, `s[i:j]`, `append(s, …)` | No |
| `make([]!*T, n)`, `make(map[K]!*T)` | No element values to check |
| conversion / type assertion | No (I6) |

`make` produces a non-nil container (outer `!` satisfied) but does not
populate elements that could be checked.

**LOCKED.** Consistent with Type Coverage C6.

### E8 — `.gna` representation

- Element `!` is now legal in `.gna` type strings: `"[]!*T"`,
  `"map[string]!*Service"`, `"[8]!*Slot"`.
- Leading `!` still outermost-only; `"![]!*T"` is the composition of outer
  + element.
- Channel element forms (`"chan !*T"`) remain a load error / GN003.
- Schema stays **1** (I7). Syntax was already a string; only the acceptance
  rule changes.

**LOCKED.** No schema bump.

### E9 — Local Gon type expressions

The same forms are accepted in source:

```go
func F(s []!*T) {}
func G() map[string]!*T { … }
type Slots [4]!*Slot
```

The preprocessor already strips `!` for `go/types`; the checker records
element non-nil offsets exactly as it does for ordinary `!` positions.

**LOCKED.**

### E10 — Breaking-change classification

| Sub-feature | Breaking? | Notes |
|-------------|-----------|-------|
| Accept element `!` syntax (was GN003) | No | Previously rejected; now accepted. |
| GN001 on `nil` element in literal | No | New diagnostic only on newly-legal syntax. |
| GN002 on fixed-array zero-fill | No | New diagnostic only on newly-legal syntax. |
| Ordinary → element `!` still accepted | No | Monotonic. |

**LOCKED.** Entirely non-breaking for Gon source. Matches roadmap
classification for v1.7.

**Compatibility caveat:**

- **Gon source compatibility:** non-breaking. Programs that were valid
  under v1.6 remain valid; newly legal forms only add diagnostics on
  previously rejected syntax.
- **`.gna` semantic schema:** unchanged, schema **1**.
- **Older `.gna` tooling / loaders** built against v1.6 may still reject
  the newly legal element forms. Consumers that embed or re-validate
  `.gna` files should be updated; the schema version itself does not
  change.

### E11 — Channel and function positions

`!` immediately following a channel element position remains **GN003**.
Function parameter/result annotations remain governed exclusively by their
existing rules; this RFC introduces no new function-type annotation
positions.

**LOCKED.** Structural wording; no new surface.

### E12 — Element interface contracts (`[]!I`)

An element contract whose element type is an interface follows the v1.4
interface-value rule (M1a):

> `[]!I` means each explicitly written element’s **interface value** is
> non-nil; the dynamic value may still be a typed nil.

This is the same non-nil contract the element type already carries; no
special case is required beyond applying E4 to interface elements.

**LOCKED.**

## 6. Normative Semantics

### 6.1 Element non-nil annotation

A type expression has an **element contract** when:

- it is an array or slice type whose element type expression carries `!`,
  or
- it is a map type whose value type expression carries `!`.

The element contract is the non-nil contract of that element / value type
(pointer, interface, slice, map, chan, func, or named/alias of those).
For an interface element type, the contract is the v1.4 interface-value
rule (E12).

### 6.2 Construction sites for element contracts

The only construction sites that enforce element contracts are:

1. Composite literals of array, slice, or map type whose element / value
   type carries `!`.
2. Fixed-array composite literals that leave one or more indices
   zero-filled when the element type is a nilable kind under `!` (E5).

`new`, `make`, variable declarations without a composite literal, and
all other expression forms are **not** element-contract construction
sites.

### 6.3 Element check procedure

For each element (or map value) expression `e` in a qualifying composite
literal:

> If `e` is the untyped nil literal → emit **GN001**.  
> Otherwise → accept.

Non-nil-source classification is not consulted for this check. No
additional diagnostics. No attempt to evaluate whether a non-literal
expression is “really” nil.

### 6.4 Fixed-array zero-fill

For `lit` of type `[N]E` with `E` carrying `!` and `E` nilable:

1. Build the set of **covered indices**:
   - For each unkeyed element at position *i* in `lit.Elts`, add *i*.
   - For each keyed element `k: e` where *k* is a constant integer, add *k*.
2. If any index in `[0, N)` is not covered, emit **one GN002** for the
   literal.

`[...]E` (inferred length) never takes this path: its length equals the
number of elements in the literal, so every index is covered by
construction.

Slices and maps never take this path.

### 6.5 Zero-value containment (unchanged)

The field-contract zero-value walk (v1.2 §6.1) continues to **stop** at
every slice, map, channel, function, and pointer boundary. Element
contracts do **not** cause the walk to descend into container elements.
A struct field of type `[]!*T` is still only checked for the outer
reference contract of the field itself; the element contract is enforced
only when a composite literal of that field’s type is written.

### 6.6 Non-nil sources

An expression that is a composite literal of type `[]!*T` (etc.) is a
non-nil source for an *outer* `![]…` contract exactly when the literal
itself is non-nil (always true for a composite literal). It does **not**
become a non-nil source *for the element type*; individual elements are
checked only at the literal’s own construction site.

Selecting, indexing, or ranging over a `[]!*T` value does not produce
element non-nil sources (E7).

### 6.7 Diagnostics

| Code | When |
|------|------|
| **GN001** | Explicit `nil` written as an element / map value under an element contract. |
| **GN002** | Fixed-array composite literal leaves at least one `!` nilable element index at zero. |
| **GN003** | Element `!` on a channel type, or any other malformed element placement still reserved. |

Existing outer-reference diagnostics (GN001/GN002 for `![]T` etc.) are
unchanged.

## 7. Examples

### 7.1 Slice element contracts

```go
func ok() []!*T {
    return []!*T{&T{}, &T{}}     // OK
}

func badNil() []!*T {
    return []!*T{nil}            // GN001
}

func outerAndElement() ![]!*T {
    return []!*T{}               // OK — outer satisfied, no elements
}

func outerNil() ![]!*T {
    return nil                   // GN001 (outer)
}

func ordinaryCall() {
    f := func() *T { return maybeNil() }
    _ = []!*T{f()}               // OK — ordinary expression, not literal nil
}
```

### 7.2 Map value contracts

```go
m := map[string]!*Svc{
    "a": &Svc{},                 // OK
    "b": nil,                    // GN001
}
```

### 7.3 Fixed array (keyed and unkeyed)

```go
var a [3]!*T = [3]!*T{&T{}}           // GN002 — indices 1,2 zero-filled
var b [3]!*T = [3]!*T{2: &T{}}        // GN002 — indices 0,1 zero-filled
var c [3]!*T = [3]!*T{0: &T{}, 2: &T{}} // GN002 — index 1 zero-filled
var d [3]!*T = [3]!*T{0: &T{}, 1: &T{}, 2: &T{}} // OK
var e [2]!*T = [2]!*T{&T{}, &T{}}     // OK
var f [0]!*T = [0]!*T{}               // OK — no elements

var g [...]!*T = [...]!*T{&T{}}       // OK — inferred length 1, no shortfall
```

### 7.4 Composition with outer `!`

```go
var s ![]!*T = []!*T{&T{}}      // OK
var t ![]!*T = []!*T{nil}       // GN001 (element)
var u ![]!*T                    // GN002 (outer zero) — v1.6
```

### 7.5 Element interface contracts (E12)

```go
var x []!I = []!I{nil}              // GN001
var y []!I = []!I{someInterface}    // OK — ordinary / non-nil interface value
```

### 7.6 Non-goals illustrated

```go
s := []!*T{&T{}}
s[0] = nil                       // no diagnostic (lifetime not tracked)
s = append(s, nil)               // no diagnostic
for _, p := range s {            // p is ordinary *T
    _ = p
}
```

### 7.7 `.gna`

```yaml
# annotations/example.gna
schema: 1
package: example
functions:
  Handlers:
    results:
      - "[]!*Handler"
  Register:
    params:
      - "map[string]!*Service"
```

## 8. Invariant Check (I1–I7)

- **I1 (no flow-sensitivity)** — upheld: element checks are purely local to
  the composite-literal syntax; later mutation is ignored.
- **I2 (explicit contracts authoritative)** — upheld: only explicitly
  annotated element positions are checked; no inference from bodies.
- **I3 (Go semantics authoritative)** — upheld: type identity, assignability,
  and composite-literal typing remain under `go/types`.
- **I4 (`.gna` authority only for nilability)** — upheld: element `!` is
  additional nilability metadata only.
- **I5 (`!T` is compile-time only)** — upheld: no runtime checks, no rewrite
  of index/append/send.
- **I6 (no implicit propagation)** — upheld: index, append, conversion, and
  assertion never carry element contracts (E7).
- **I7 (schema versioning is syntax-driven)** — upheld: syntax of type
  strings is unchanged; only the acceptance of previously-reserved forms
  changes. Schema stays **1**.

## 9. Compatibility & Release Shape

- **Breaking risk:** None (E10). All new diagnostics fire only on syntax
  that was previously rejected.
- **Gon source compatibility:** non-breaking.
- **`.gna` semantic schema:** unchanged, schema **1**.
- **Older `.gna` tooling:** may not understand newly legal element forms
  (see E10 caveat).
- **Roadmap fit:** Exactly M2b as written in `docs/roadmap-v1.3-plus.md`.
  Dependency on M1b (v1.6) is satisfied; R2 non-breaking bundling applies.
- **Recommended release:** v1.7 as a focused, non-breaking follow-on to
  type coverage. No isolation requirement (R1 does not apply).

## 10. Implementation Notes (proposed)

- Preprocessor / annotation loader: lift the v1.6 rejection of element-position
  `!` (the GN003 path added for C10). Record element non-nil information
  alongside existing `Type.NonNil` / offset sets.
- `checkCompositeLitConstruction`: after the existing struct / external
  paths, add an element walk for `*ast.ArrayType` and map types whose
  element / value AST carries `!`. The element check is only “is this the
  untyped nil literal?” (E4); do not consult non-nil-source classification
  for acceptance.
- Fixed-array shortfall (E5): compute the set of covered indices from
  unkeyed positions and constant keys; if any index in `[0, N)` is missing
  and the element type is `!` + nilable, emit one GN002. Do not use
  `len(lit.Elts)` as a proxy for coverage.
- `[...]E` literals: skip the shortfall path entirely.
- Tests (required before ship):

  ```go
  // Keyed / unkeyed fixed-array coverage
  var a [3]!*T = [3]!*T{&T{}}              // GN002
  var b [3]!*T = [3]!*T{2: &T{}}           // GN002
  var c [3]!*T = [3]!*T{0: &T{}, 2: &T{}}  // GN002
  var d [3]!*T = [3]!*T{0: &T{}, 1: &T{}, 2: &T{}} // OK
  var e [...]!*T = [...]!*T{&T{}}          // OK

  // Interface element
  var x []!I = []!I{nil}                   // GN001
  var y []!I = []!I{someInterface}         // OK

  // Composition
  var z ![]!*T = []!*T{nil}                // GN001

  // Ordinary call still accepted
  var w []!*T = []!*T{f()}                 // OK
  ```

  Plus: slice/map positive and negative cases; outer+element composition;
  `.gna` round-trip; confirmation that `s[i] = nil` and `append` remain
  silent; channel element forms still GN003.
- No change to the zero-value containment walk in `fields.go`.

## 11. Deferred Items (explicitly non-blocking)

| ID | Item | Decision |
|----|------|----------|
| O1 | Map **key** contracts (`map[!*K]V`) | **Deferred.** Value side only (E1). |
| O4 | Nested element depth limit | **No artificial limit;** ordinary Go nesting applies. |
| O5 | Interaction with external `.gna` composite literals (M4) | **Deferred / inherits M4 firewall.** Element checks apply to local literals of external types only when the type string is known from `.gna`; unkeyed external firewall (E2 of ecosystem RFC) is unchanged. |

O2 (one GN002 per array literal) and O3 (`[]!I` follows v1.4) were locked
as E5 and E12 prior to acceptance.

## 12. References

- `docs/roadmap-v1.3-plus.md` — M2b, v1.7
- `docs/rfc-type-coverage.md` — C10 reservation of element contracts
- `docs/rfc-field-contracts.md` — construction-site model, zero-value containment
- `docs/gna-spec-v1.md` — current outermost-binding rule and M2b reservation
- `docs/rfc-interface-semantics.md` — v1.4 interface-value rule (E12)
