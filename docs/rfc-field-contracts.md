# RFC: Field Annotations / Field Contracts (Gon v1.2)

**Status:** Draft  
**Target:** Gon v1.2  
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`, `docs/rfc-return-value-contracts.md`  
**Date:** 2026-08-17

## 1. Summary

Gon v1.0 and v1.1 already support `!T` on parameters, results, and receivers.
They do **not** yet treat fields declared `!T` as invariants of the storage
location itself.

This RFC makes a field annotation an **invariant of its declared storage
location**. The checker enforces the invariant at three local sites only:

- construction of a value that contains the field (GN002),
- mutation of the field (GN001),
- use of the field as a non-nil source (selector).

No flow analysis is performed. The checker never reconstructs object state
from control flow.

**Anchor statements (normative):**

> A `!T` field is an invariant of its declared storage location, not a
> one-time property of its construction.

> GN002 recursively inspects types according to zero-value containment.
> A nested type is traversed when the containing type's zero value contains
> an actual value of that nested type. Traversal stops at indirection
> boundaries and never follows reachable objects.

> Gon validates contracts locally at construction, mutation, and use sites;
> it does not reconstruct object state from control flow.

## 2. Motivation

- Many real-world types have fields that are required to be non-nil for the
  rest of the object's lifetime (clients, loggers, configuration handles,
  etc.).
- Authors already want to write these contracts; without field support the
  only options are comments or runtime panics.
- The change stays local and non-flow-sensitive, preserving the character
  of Gon established in v1.0/v1.1.

## 3. Goals

1. A field declared `!T` is an invariant of every value of the containing
   type (and of every type that structurally contains it under the
   zero-value containment rule).
2. Construction of a value that leaves a `!T` field at its zero value is a
   hard error (GN002).
3. Assignment of an explicit nil into a `!T` field is a hard error (GN001).
4. Selecting a `!T` field yields a non-nil source that may be consumed in
   immediate contexts (same rule as return-value contracts).
5. Structural traversal is purely type-driven and stops at every
   indirection boundary.
6. The checker remains non-flow-sensitive and non-path-sensitive.

## 4. Non-Goals

- Flow-sensitive or path-sensitive tracking of field values after
  construction.
- Conditional field contracts (`non-nil when \ldots`).
- Automatic inference of field contracts from constructors or methods.
- Tracking of values behind pointers, slices, maps, channels, interfaces,
  or function values.
- Changing the zero value of any Go type.
- Tightening the existing conservative rule that allows ordinary values to
  be assigned to `!T` destinations.
- Schema version bump (syntax already allows field annotations under
  schema 1; only semantics are activated).

## 5. Normative Semantics

### 5.1 Field `!T` as an invariant

A field annotated `!T` (in a `.gna` file or in a local Gon type definition)
is an invariant of the storage location. Every constructed or mutated value
of the containing type must satisfy the invariant.

The invariant is attached to the field itself, not to any particular
constructor.

### 5.2 Zero-value containment principle

GN002 inspects a type by asking a single question:

> Does the zero value of this type contain an actual value of a nested type
> that itself has a `!` field?

If the answer is yes, the nested type is traversed.
If the answer is no, traversal stops.

This is a pure structural property of the type; it does not depend on
reachability, allocation, or control flow.

### 5.3 Structural traversal

Traversal is defined recursively on the type of the value being constructed.
The precise table appears in §6.1. The principle is structural, not nominal:
anonymous struct types are traversed identically to named struct types.

### 5.4 Construction sites → GN002

A **construction site** is a form whose field initialization can be determined
locally from the construction syntax or type, without reconstructing the state
of any existing value.

The following are construction sites:

- a declaration whose initializer is absent or whose initialization is
  locally known to produce the zero value
  (`var s S`, `var s S = S{}`, etc.)
- composite literal
- `new(T)`
- other construction forms whose initialization is locally observable from
  syntax/type

At a construction site the checker walks the type according to the
zero-value containment rules. Every `!T` field that is left at its zero
value produces **GN002**.

**Explicit non-sites:**

- Assignment from an existing value (`b := a`, `dst = src`) is a **mutation**,
  not a construction site. GN002 does not reconstruct the source object's
  field state.
- A range-loop variable that receives a copy of an existing element
  (`for _, v := range items`) is likewise a copy of an existing value and
  is not a construction site.
- Conversion is not a construction site for the purposes of this RFC
  (consistent with v1.1: conversion does not create or propagate a non-nil
  source).

### 5.5 Mutation sites → GN001

Direct assignment to a field that is declared `!T` is a mutation site.

```go
s.Client = nil            // GN001
s.Client = ordinary       // accepted (conservative rule, same as v1.1)
s.Client = nonNilSource   // OK
```

Field assignment through an embedded selector is treated identically.

Only an explicit nil (or a value that is known to be nil under existing
rules) produces GN001. Ordinary values remain accepted under the same
conservative policy used for other `!T` destinations in v1.0/v1.1.

### 5.6 Selector use → non-nil source

Selecting a field declared `!T` produces a non-nil source. The source may
be consumed in an immediate context exactly as defined for return-value
contracts (assignment to `!U`, argument to `!U` parameter, method
receiver).

No further propagation occurs.

## 6. Structural Traversal

### 6.1 Derived table

| Type form              | Traversed? | Reason |
|------------------------|------------|--------|
| `struct { \ldots }` (named or anonymous) | yes | zero value contains the fields |
| embedded struct        | yes        | same as ordinary field of struct type |
| `[N]T`                 | yes        | zero value contains N values of T |
| `T` (named, underlying is above) | yes | underlying type decides |
| `*T`                   | **no**     | zero value is nil; no contained value |
| `[]T`                  | **no**     | zero value is nil |
| `map[K]V`              | **no**     | zero value is nil |
| `chan T`               | **no**     | zero value is nil |
| `interface{…}`         | **no**     | zero value is nil |
| `func(…)`              | **no**     | zero value is nil |
| `unsafe.Pointer`       | **no**     | treated as indirection |

### 6.2 Positive generative examples

```go
type Inner struct {
    Client !*Client   // ! field
}

type Embedded struct {
    Inner             // must be traversed
}

type Arrayed struct {
    Items [3]Inner    // must be traversed
}

var _ = Embedded{}            // GN002 (Inner.Client is zero)
var _ = Arrayed{}             // GN002 (three zero Clients)
var _ = Arrayed{Items: [3]Inner{
    {Client: c}, {Client: c}, {Client: c},
}}                            // OK

// anonymous struct — traversed identically
var _ = struct{ Client !*Client }{}  // GN002
```

### 6.3 Indirection counter-examples

```go
type Indirect struct {
    Ptr   *Inner      // not traversed
    Slice []Inner     // not traversed
}

var _ = Indirect{}            // accepted — no GN002
var _ = Indirect{Ptr: &Inner{}} // accepted — the pointed-to value is
                                // not part of the zero-value containment
                                // of Indirect
```

### 6.4 Recursive / cycle boundary

Valid Go types cannot contain an infinite recursive value cycle without an
indirection boundary. Implementations must nevertheless maintain a
visited-type guard when recursively inspecting types. The guard is an
implementation safety invariant; it is not a semantic rule that assumes
the existence of invalid type graphs.

## 7. Declare-Then-Populate

A common Go pattern is:

```go
var s S
s.Client = mustClient()
```

Under this RFC the declaration `var s S` is a construction site that
produces the zero value. If `S` contains a `!` field, **GN002** is reported
at the declaration.

This is intentional. The diagnostic forces the author either to:

- construct the value in one expression that satisfies every `!` field, or
- change the field to ordinary `T` if the declare-then-populate pattern is
  required.

When an explicit initializer is present:

```go
var s S = S{Client: client}   // evaluates the composite literal
```

the checker inspects the initializer, not the bare type of the declaration.

### 7.1 Intentional diagnostic

GN002 at the declaration site is not a false positive; it is the direct
consequence of treating the field as an invariant of the storage location.

### 7.2 Migration guidance

- Prefer composite literals or constructor functions that return a fully
  initialised value.
- If a type must support partial initialisation, leave the field ordinary
  and document the required sequencing in comments or runtime checks.
- Existing code that uses declare-then-populate on types that will receive
  `!` annotations will need a one-time mechanical change.

## 8. Why Not Flow Analysis?

Gon deliberately refuses to reconstruct object state from control flow.
Doing so would:

- re-introduce the path-sensitivity problems that the project has
  consistently rejected,
- make diagnostics dependent on the order of statements,
- turn every later mutation into a potential new analysis obligation,
- destroy the simple mental model “contracts are checked at the three
  local sites only”.

The three local sites (construction, mutation, use) are sufficient to
make `!` fields useful while keeping the checker predictable and fast.

## 9. Compatibility

- Schema version remains **1**. Field annotations were already
  syntactically legal; this RFC only activates their semantics.
- Programs accepted by v1.1 remain accepted by v1.2 unless they construct
  or mutate a value that violates a newly annotated `!` field.
- Adding a `!` annotation to a field is a deliberate strengthening by the
  annotation author; it may turn previously accepted programs into
  diagnostics. This is the same monotonicity rule used for return-value
  contracts.
- Ordinary values assigned to `!T` fields remain accepted (no tightening).
- Conversion continues to neither create nor propagate non-nil sources.

## 10. Examples

### Positive

```yaml
# annotations/example.gna
types:
  Config:
    fields:
      Client: "!*http.Client"
      Log:    "!*log.Logger"
```

```go
cfg := Config{
    Client: http.DefaultClient,
    Log:    log.Default(),
}                                 // OK

cfg.Client.Do(req)                // OK — selector is non-nil source
```

### GN002 (construction)

```go
var c Config                      // GN002 — both fields zero
_ = Config{}                      // GN002
_ = Config{Client: http.DefaultClient} // GN002 — Log still zero
```

### GN001 (mutation)

```go
cfg.Client = nil                  // GN001
cfg.Log = ordinaryLogger          // accepted (conservative)
```

### Structural cases

```go
type Wrapper struct {
    C Config                      // traversed
}
_ = Wrapper{}                     // GN002

type Box struct {
    P *Config                     // not traversed
}
_ = Box{}                         // accepted
```

## 11. Alternatives Considered

| Alternative | Reason rejected |
|-------------|-----------------|
| Treating every expression of type `T` as a construction site | Would require reconstructing existing object state; violates locality |
| Making conversion a construction site | Inconsistent with v1.1 (conversion does not create/propagate non-nil sources) |
| Flow-sensitive field tracking | Violates the “local sites only” principle |
| Treating `*T` / `[]T` as containing values | Contradicts zero-value containment |
| Making declare-then-populate silently accepted | Would turn the invariant into a one-time construction property |
| Tightening ordinary → `!T` field to an error | Significant compatibility change; deferred |
| New diagnostic code for every nested level | Unnecessary; a single GN002 with a clear path is enough |
| Schema bump to 1.2 | Syntax did not change |

## 12. Open Questions

| # | Question | Tentative decision |
|---|----------|--------------------|
| 1 | Exact wording of GN002 message (include field path?) | Yes — include the field path that is zero |
| 2 | Interaction with type aliases | Follow the underlying type |
| 3 | Blank fields (`_ !*T`) | Still an invariant; construction must supply a non-nil value |
| 4 | Promotion of embedded fields across package boundaries | Same rules; annotation must be present on the concrete type |

These can be finalised during implementation review; none of them change the
core semantics locked by this RFC.

## 13. Implementation Sketch

- Extend the type-annotation map to record fields that carry `!`.
- At every construction site that produces a type containing `!` fields,
  run a recursive zero-value walk (cycle-aware via visited-type guard).
- On `AssignStmt` whose LHS is a selector ending in a `!` field, require
  the RHS to be a non-nil source or accept an ordinary value under the
  existing conservative rule; only explicit nil yields GN001.
- On selector expressions that end in a `!` field, mark the result as a
  non-nil source for immediate consumption.
- Re-use the existing non-nil-source machinery introduced for return-value
  contracts; no new propagation engine is required.

## 14. Test Requirements

Minimum regression suite:

- Direct `!` field construction (positive + GN002).
- Embedded struct traversal.
- Fixed-size array traversal (including nested arrays such as `[3][2]Inner`).
- Anonymous struct types (traversed identically; structural, not nominal).
- Pointer / slice / map / interface / channel / func non-traversal
  (must stay accepted).
- Mutation of `!` field: explicit nil → GN001; ordinary → accepted.
- Selector as non-nil source (assignment, argument, receiver).
- Declare-then-populate (`var s S` → GN002).
- Explicit initializer (`var s S = S{...}`) evaluates the literal.
- Nested embedding + array combination.
- Range-loop variable (`for _, v := range items`) is a copy of an existing
  value and must not trigger GN002.
- Cycle guard (implementation safety).
- Cross-package field annotation via `.gna`.

All existing v1.1 tests must remain green.

---

**Next step after acceptance of this RFC:** implement the structural walk
and the three local checks, starting from the test suite as executable
specification, then tag v1.2.0.
