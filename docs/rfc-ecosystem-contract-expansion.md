# RFC: Ecosystem Contract Expansion (Gon v1.5)

**Status:** Implemented — shipped in Gon v1.5.0 (2026-09-06). Decision matrix
(E1–E14) locked 2026-09-06; §10 resolved.  
**Target:** Gon v1.5  
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`, `docs/roadmap-v1.3-plus.md`, `docs/rfc-field-contracts.md`, `docs/rfc-return-value-contracts.md`, `docs/rfc-interface-semantics.md`  
**Date:** 2026-09-05

## 1. Summary

Through v1.4 Gon enforces `!T` contracts that originate in **local** source
(parameters, results, variables, struct fields, receivers) and applies a
narrow slice of **external** `.gna` contracts: function / method **parameter**
nil-rejection, and function / method **result** promotion to a non-nil source.

This RFC completes the cross-package story bundled as roadmap milestones
**M4a + M4b + M5a**:

- **M4a — non-interface application.** External `.gna` `types:` field
  contracts become enforceable at construction, mutation, and use sites of
  values of an external type — the same three sites as local field contracts
  (`docs/rfc-field-contracts.md`), extended across the package boundary.
- **M4b — interface-typed application.** When an external contract position
  is an interface type, it carries the semantics locked in
  `docs/rfc-interface-semantics.md` (`!I` = interface value non-nil only).
- **M5a — `.gna` validation + contract boundary.** The checker validates a
  `.gna` file against the **real** package it annotates: signature arity
  must match `go/types`, and every annotated symbol must exist. A precise
  statement of what a `.gna` contract may and may not do completes the
  boundary.

**Anchor statements (normative):**

> A `.gna` contract may only **add** a non-nil claim. It never asserts Go
> type identity, assignability, method sets, or any flow-dependent property.
> `go/types` remains the sole authority for what a symbol *is*.

> An external `!T` field is an invariant of its declared storage location,
> enforced at exactly the three sites defined for local field contracts.
> Structural traversal across the boundary is type-driven and never
> reconstructs external field **order**.

> A `.gna` file is checked against the package it annotates. A contract that
> disagrees with `go/types` about arity is dropped (GN003); a contract that
> names a symbol the package does not export is reported (GW004).

## 2. Motivation

- Field contracts (v1.2) and interface semantics (v1.4) are only useful for
  imported APIs if the corresponding `.gna` annotations are actually applied
  at the call site. Today a `.gna` `types:` block is honoured only for a
  keyed composite literal's *missing* fields (GN002) — not for explicit
  `nil`, not for mutation, not as a non-nil source.
- `.gna` files are hand-written and drift. A misspelled method name or a
  stale parameter list currently fails **silently** or, worse, applies a
  positional contract to the wrong argument. Validation against `go/types`
  turns drift into a diagnostic.
- The interface gate (v1.4) is a local-source feature until external `!I`
  contracts are applied with the same rules.

## 3. Decision Matrix

The following decisions are locked. The normative rules in §4 MUST NOT
introduce semantics beyond these decisions.

### M4a — Non-interface cross-package application

#### E1 — External keyed construction

| Option | Decision |
|--------|----------|
| A keyed literal `pkg.T{...}` that omits a `!` field declared in `.gna` `types:` is GN002. | **LOCKED** (already implemented; restated) |
| An explicit `nil` written for a `!` field of `pkg.T` in a keyed literal is GN001. | **LOCKED** (new) |

#### E2 — External unkeyed construction

| Option | Decision |
|--------|----------|
| Unkeyed / positional `pkg.T{a, b}` is checked against `.gna` field order. | **Rejected** — Gon does not reconstruct external field order. |
| Unkeyed external literals are left unchecked (firewall stays). | **LOCKED** |

Positional external construction remains a separate future concern and is
out of scope for v1.5 (§6).

#### E3 — External `!` field as a non-nil source

| Option | Decision |
|--------|----------|
| A selector `x.F` where `x` has external type `pkg.T` and `.gna` marks `T.F` as `!` is a non-nil source (GW001 on nil comparison; usable where `!U` is required at an immediate site). | **LOCKED** (new) |

This is the exact rule already applied to local `!` fields
(`rfc-field-contracts.md` §Goal 4), extended across the boundary. It is not
flow analysis: the property comes from the declared contract, not from
control flow.

#### E4 — External `!` field mutation

| Option | Decision |
|--------|----------|
| An explicit `nil` assigned into an external `!` field (`x.F = nil`) is GN001. | **LOCKED** (new) |
| Non-`nil`, non-literal assignment into an external `!` field is checked. | **Rejected** — monotonic with local field contracts; no flow analysis. |

#### E5 — External result contracts

| Option | Decision |
|--------|----------|
| A function / method result annotated `!T` in `.gna` is a non-nil source at the immediate use site. | **LOCKED** (already implemented; restated, unchanged) |

#### E6 — Zero-value construction sites for external types

| Option | Decision |
|--------|----------|
| `new(pkg.T)`, `&pkg.T{}`, and `pkg.T{}` are zero-value construction sites: a `!` field of `pkg.T` (per `.gna` `types:`) left at zero is GN002. | **LOCKED** |

The pre-v1.5 `new` / selector firewall is lifted **only** for the
zero-value containment walk, which is type-driven and needs no field-order
knowledge. Positional literals (E2) stay behind the firewall.

#### E7 — Receiver contracts

| Option | Decision |
|--------|----------|
| `receivers: { T: "!" }` changes checking at a call site of a `T` method. | **Rejected for v1.5** — Gon performs no receiver-nil check at call sites. `receivers:` remains parsed, validated (E11/E12), and reserved. |

### M4b — Interface-typed cross-package application

#### E8 — Interface semantics apply to external `!I` positions

| Option | Decision |
|--------|----------|
| An external `.gna` position (param, result, or `types:` field) whose Go type is an interface carries `docs/rfc-interface-semantics.md` semantics: `!I` = interface value non-nil; a typed-nil dynamic value does not violate it; an ordinary interface-typed argument / value cannot satisfy it (D3b). | **LOCKED** |

#### E9 — `.gna` does not classify interface vs concrete

| Option | Decision |
|--------|----------|
| The `.gna` author writes `!I` exactly as `!*T`; the checker classifies the position as interface or concrete from `go/types` at the use site. | **LOCKED** (restates Interface RFC D7) |

#### E10 — Signature arity validation

| Option | Decision |
|--------|----------|
| When `go/types` resolves the annotated function / method, the length of `.gna` `params` / `results` MUST equal the Go signature's parameter / result count. A mismatch is **GN003** and the contract for that symbol is dropped (treated as ordinary). | **LOCKED** |

- A variadic `...T` parameter counts as **one** position.
- "Dropped" means: no parameter or result flag from that symbol's `.gna`
  entry is applied. This prevents a stale annotation from silently shifting
  a `!` claim onto the wrong argument.
- Arity is the only shape property checked. Type *strings* in `.gna` remain
  documentation only (`gna-spec-v1.md`); the checker does not compare them.

#### E11 — Unknown annotated symbol

| Option | Decision |
|--------|----------|
| A `.gna` key under `functions:`, `methods:`, or `types:` that does not correspond to an exported symbol of the real package is **GW004** (warning). Non-fatal; other contracts in the file still apply. | **LOCKED** |
| `methods: T.M` validation resolves `M` against `T`'s **full `go/types` method set**, accepting methods promoted from embedded fields. | **LOCKED** (§10 Q3) |

GW004 is distinct from GW002. GW002 means *"the package has a `.gna` file
but this call target is not annotated"* (silence would hide an
under-annotated dependency). GW004 means *"the `.gna` file annotates a name
the package does not have"* (drift / typo).

#### E12 — Duplicate keys and one-file rule

| Option | Decision |
|--------|----------|
| Duplicate `functions:` / `methods:` / `types:` keys within a file, and more than one `.gna` per package, remain **hard load errors**. | **LOCKED** (already implemented; restated) |

### M5a — Contract boundary

#### E13 — What a `.gna` contract may do

A `.gna` contract MAY:

- add a non-nil claim to a parameter, result, or field position;
- do so for concrete, pointer, or interface Go types.

A `.gna` contract MUST NOT (and the loader / checker does not honour any
attempt to):

- weaken or remove a nil claim (there is nothing local to weaken for an
  external symbol; a `"T"` entry is simply "no claim");
- assert or alter Go type identity, assignability, conversions, method
  sets, or interface satisfaction — these are `go/types` only (I3);
- introduce conditional (`non-nil when err == nil`) or flow-dependent
  contracts (I1);
- annotate generic / type-parameter positions (deferred to v1.8/v1.9).

#### E14 — Schema version

| Option | Decision |
|--------|----------|
| `.gna` schema stays **1**. M4a/M4b/M5a add checker behaviour and validation, not file-format fields. | **LOCKED** (I7 — versioning is syntax-driven) |

## 4. Normative Rules

### 4.1 External field contracts — construction (E1, E6)

For a value literal or allocation whose type resolves through `go/types` to
a named type `pkg.T` for which the resolver supplies a `types:` entry:

1. `pkg.T{}` (empty), `new(pkg.T)`, `&pkg.T{}`, and a **keyed** `pkg.T{k: v,
   …}` are construction sites.
2. Every field of `T` marked `!` in `.gna` that is not given a value at the
   site is **GN002 — missing required non-nil field `T.F`**.
3. A field marked `!` that is given the literal `nil` at the site is
   **GN001 — cannot assign nil to non-nil field `T.F`**.
4. Unkeyed `pkg.T{a, b, …}` is **not** a construction site for external
   field contracts (E2). No diagnostic, in either direction.
5. The zero-value containment walk (`rfc-field-contracts.md`) applies across
   the boundary using `go/types` structure, **one hop deep** for v1.5: the
   directly-constructed `pkg.T`'s own `!` fields are checked, **including `!`
   fields promoted from an embedded (external or local) type** — `go/types`
   supplies the promoted set. It stops at every indirection boundary (`*T`,
   `[]T`, `map`, `chan`, `interface`, `func`). It does **not** recurse into a
   non-embedded external struct field that itself carries `.gna` field
   contracts (external→external→… multi-hop traversal is deferred to a
   follow-on; §6). A fixed-array-contained external type is treated as the
   one contained hop, not a recursion root.

   The distinction is **depth, not lookup count**. "One hop" covers the
   constructed `pkg.T` and any type *embedded directly in `pkg.T`* — an
   embedded type's own `types:` entry is consulted for its `!` fields
   (those fields are promoted onto `pkg.T` by `go/types`). It does **not**
   cover a type reached through a *named, non-embedded* field of `pkg.T`:
   that would be a second structural hop into an unrelated external
   contract, and is deferred.

### 4.2 External field contracts — mutation (E4)

An assignment statement whose left-hand side is a selector `x.F` where the
static type of `x` resolves to `pkg.T` and `.gna` marks `T.F` as `!`:

- right-hand side is the literal `nil` → **GN001 — cannot assign nil to
  non-nil field `F`**.
- `T.F` has an interface type and the right-hand side is an ordinary
  interface-typed expression → **GN001** (§4.5 / D3b) — the same rule as
  a `!I` local variable on plain assignment.
- any other right-hand side → no diagnostic (monotonic with local field
  contracts; Gon does not analyse the assigned value).

### 4.3 External field contracts — use as source (E3)

A selector `x.F` where the static type of `x` resolves to `pkg.T` and `.gna`
marks `T.F` as `!` is a **non-nil source**:

- `x.F == nil` / `x.F != nil` → **GW001** (comparison is always
  false / true).
- `x.F` used to initialise or assign a `!U` target, or passed as a `!U`
  argument, at the immediate site → accepted with no diagnostic. This
  requires the non-nil-source recogniser to accept a `!`-field selector,
  not only a `!`-bound identifier or a `!T`-result call; the same
  extension applies to local `!`-field selectors (previously a latent gap).

External `types:`-field resolution is **exact**: it matches on the
resolved `*types.Named`'s package path, type name, and field name. There
is no field-name-only fallback (the local path's name-only heuristic is
not carried across the boundary).

If `T.F` has an interface type, the source is an **interface-value** non-nil
source (§4.5): it does not certify the dynamic value.

### 4.4 `.gna` validation against the real package (E10, E11)

When the resolver supplies a `.gna` file for an imported package and
`go/types` has loaded that package:

1. For each `functions: N` entry, `N` must be an exported function of the
   package. For each `methods: T.M` entry, `T` must be an exported named
   type and `M` a method in its **full `go/types` method set** —
   **methods promoted from an embedded field are accepted** (§10 Q3). For
   each `types: T` entry, `T` must be an exported named type. A key with no
   corresponding symbol → **GW004 — `.gna` for `pkg` annotates `X`, which
   the package does not provide**.
2. For a resolved function or method, `len(params)` must equal the Go
   parameter count and `len(results)` the Go result count (a variadic
   final parameter counts as one). Otherwise → **GN003 — `.gna` for `pkg`:
   signature arity mismatch for `X` (annotation N, package M)**, and every
   parameter and result flag from that entry is discarded for the rest of
   the check.
3. Validation is best-effort: it runs only for packages `go/types` actually
   resolved. A package that fails to load is treated as before (contracts
   applied unvalidated, method resolution simply unavailable). Validation
   never blocks the local checks.
4. Each distinct GW004 / GN003 condition is reported **once per annotated
   symbol per check run**, not once per call site: a drifted `.gna` entry
   invoked twenty times yields one diagnostic.
5. Validation runs **after `go/types` type-checking and before any contract
   is consumed by call/field resolution**: it is a single pass at the start
   of the check, so the "dropped" set (point 2) is already populated when
   parameter, result, and field flags are looked up.

### 4.5 Interface-typed external positions (E8, E9)

When an external contract position (parameter, result, or `types:` field)
is annotated `!` and its Go type, per `go/types`, has an interface
underlying type:

- The contract means the **interface value** is non-nil
  (`rfc-interface-semantics.md` §3.1). It never constrains the dynamic
  value.
- Supplying an **ordinary interface-typed** expression for that position —
  an argument to a `!I` parameter, a value assigned to a `!I` field, a
  value returned where an external contract's `!I` result flows into a
  local `!I` — is **GN001** (D3b), unconditionally and without flow-sensitive
  narrowing.
- Supplying a concrete / typed operand assignable to `I` is accepted even
  when that operand is itself typed-nil (D3a).
- An existing `!I` source (a `!`-bound identifier, or a result annotated
  `!T`) satisfies the position (D3d / D4).

### 4.6 Contract boundary (E13)

`go/types` determines what an external symbol is. The resolver determines
only whether Gon assumes a non-nil claim about a position. No `.gna` entry
changes assignability, identity, method sets, or interface satisfaction,
and none introduces a conditional or flow-dependent contract. A `"T"` (or
omitted) entry is the absence of a claim, never a negative claim.

## 5. Examples

### 5.1 External keyed construction (E1)

```go
// annotations/demo.gna
// types: { Conn: { fields: { DB: "!*sql.DB", Log: "!*log.Logger" } } }

c := demo.Conn{DB: db}            // GN002 — Log missing
c := demo.Conn{DB: db, Log: nil}  // GN001 — Log is !, nil written
c := demo.Conn{DB: db, Log: lg}   // OK
```

### 5.2 External field as source and mutation (E3, E4)

```go
func use(c *demo.Conn) {
    if c.DB == nil { }  // GW001 — c.DB is a non-nil source
    c.DB = nil          // GN001
    var h !*sql.DB = c.DB // OK — non-nil source into !*sql.DB
}
```

### 5.3 External interface result (E8)

```go
// functions: { Open: { results: [ "!io.Reader" ] } }

r := demo.Open()          // r is an interface-value non-nil source
if r == nil { }           // GW001

var x !io.Reader = r      // OK — existing !I source (D3d)
var y !io.Reader = plainReader() // GN001 — ordinary interface (D3b)
```

### 5.4 Interface field, typed-nil dynamic value (E8)

```go
// types: { Box: { fields: { W: "!io.Writer" } } }

b := demo.Box{W: (*bytes.Buffer)(nil)} // OK — interface value is non-nil;
                                       // dynamic value nilness is not checked
b := demo.Box{}                        // GN002 — W missing
b.W = nil                              // GN001
```

### 5.5 Signature arity mismatch (E10)

```go
// .gna:  Take: { params: [ "!string", "!string" ] }
// real:  func Take(s string)

demo.Take(nil) // GN003 — arity mismatch (annotation 2, package 1);
               // the "!string" claims are dropped, so no GN001 here
```

### 5.6 Unknown annotated symbol (E11)

```go
// .gna annotates  methods: { Conn.Cloze: { ... } }
// real type Conn has Close, not Cloze

// GW004 — .gna for demo annotates Conn.Cloze, which the package does not provide
```

### 5.7 Unkeyed external literal stays unchecked (E2)

```go
c := demo.Conn{db, lg} // no diagnostic — Gon does not reconstruct
                       // external field order
```

## 6. Non-Goals

- **Positional / unkeyed external construction.** Deferred; needs its own
  RFC and a field-order source of truth.
- **Receiver-nil checking at call sites.** `receivers:` stays reserved.
- **`.gna` type-string checking.** The string after `!` remains
  documentation; only arity is validated.
- **Strict mode** (`--strict` promoting GW002 / GW004 to errors). A small
  follow-on; not required for v1.5.
- **Generic external contracts.** Deferred to v1.8 / v1.9 (M5b).
- **Cross-package embedded-field promotion beyond the zero-value walk.**
  Unchanged from `rfc-field-contracts.md` §Q4.
- **Multi-hop external structural traversal.** The zero-value walk is one
  hop across the boundary (§4.1.5). An external struct field whose own
  external type carries `.gna` field contracts is not recursed into.
  Deferred to a follow-on.
- **Any dynamic-value tracking for interfaces.** Excluded by
  `rfc-interface-semantics.md` §5.

## 7. Compatibility

- `.gna` schema remains **1**. No file-format change.
- Existing diagnostic codes and severities are preserved. One new code:
  **GW004** (warning — `.gna` names a symbol absent from the package).
- **Possibly breaking**, per roadmap classification, in three narrow ways,
  all of which surface a real defect that was previously silent:
  1. A `.gna` file whose `params` / `results` arity disagrees with the
     package now emits **GN003** instead of silently applying a shifted
     contract. Fix: correct the annotation.
  2. A `.gna` file that annotates a misspelled or removed symbol now emits
     **GW004**. Fix: correct or delete the entry.
  3. Code that writes an explicit `nil` into, or relies on flow reasoning
     about, an external `!` field now emits **GN001 / GW001** where it was
     silent. This is the field contract behaving as specified.
- Code that imports only unannotated packages, or annotated packages whose
  `.gna` is correct and whose external `!` fields are used correctly, is
  unaffected.
- `go/types` remains authoritative; no `.gna` entry gains identity or
  assignability power.

## 8. Implementation Sketch

- **Field source / mutation across the boundary.**
  `selectorFieldIsNonNil` (`internal/checker/fields.go`) already consults
  `c.info.Types[sel.X]` for the static type of the receiver expression;
  extend `namedTypeFieldNonNil` to consult the resolver's `types:` entry
  for a `*types.Named` whose `Obj().Pkg().Path()` has a `.gna` file, in
  addition to the local `c.structFields` map.
- **External keyed construction GN001 / GN002.**
  `reportExternalTypeFields` already emits GN002 for missing `!` fields;
  add the explicit-`nil`-in-keyed-field GN001 path (mirror
  `checkExplicitNilInComposite`). Route `new(pkg.T)` / `&pkg.T{}` / `pkg.T{}`
  through the same zero-value walk by removing the `SelectorExpr` early
  return in `checkNewConstruction` when a `types:` entry exists.
- **Interface classification.** Reuse `cannotSatisfyBangInterface(expr,
  target)` / `callArgParamType` (`internal/checker/interface_contracts.go`,
  as of v1.4.1) at the external field-value and result-flow sites. The
  call-argument site already resolves the parameter type and runs
  `cannotSatisfyBangInterface` for every non-nil parameter flag, so
  external `!I` parameters are already covered — add tests, not code.
- **`.gna` validation.** After `typeCheck` populates `c.info`, walk the
  resolver's file for each imported+resolved package: look up each
  `functions` / `methods` / `types` key via `types.Package.Scope()` and
  method sets; compare arities against `*types.Signature`. Emit GW004 /
  GN003 and record a "dropped" set consulted by `resolveCallParams` /
  `resolveCallResults`.
- Validation is one pass, guarded by `c.info != nil`, cached per check run.

## 9. Test Requirements

Minimum regression suite:

- External keyed literal: missing `!` field → GN002; explicit `nil` in `!`
  field → GN001; all fields provided → clean.
- `new(pkg.T)` / `&pkg.T{}` / `pkg.T{}` with a `!` field → GN002.
- Unkeyed `pkg.T{...}` → no diagnostic (firewall).
- External `!` field selector: `== nil` → GW001; into `!U` → clean.
- External `!` field mutation with `nil` → GN001.
- External interface result `!I`: non-nil source at call site; ordinary
  interface into that position → GN001; concrete typed-nil → clean.
- External interface `types:` field: typed-nil dynamic value at
  construction → clean; missing → GN002; `= nil` → GN001.
- `.gna` arity mismatch (params and results, incl. variadic) → GN003; the
  dropped contract produces no downstream GN001.
- `.gna` names a missing function / method / type → GW004; other entries in
  the file still apply.
- Correct `.gna` for a resolved package → no GW004 / GN003.
- A package that fails to type-check → contracts still applied, no
  validation crash.
- All existing local, field, return-value, and interface tests remain
  green.

## 10. Resolved Questions

- **GW004 vs. GW002 numbering.** **Resolved: new code GW004.** The two
  directions are distinct — GW002 fires per call site ("you called an
  un-annotated symbol"), GW004 fires once per `.gna` load ("the file names
  a symbol the package lacks"). Keeping them separate lets consumers
  filter and keeps each message precise.
- **`--strict`.** **Resolved: v1.5.x follow-on**, not v1.5. It stays a §6
  non-goal; the M4a+M4b+M5a bundle is already large.
- **Method-set scope for E11.** **Resolved: accept promoted methods.**
  `methods: T.M` validation uses the full `go/types` method set of `T`,
  including methods promoted from embedded fields. A caller's `x.M()` is
  an ordinary call regardless of where `M` is declared; requiring the
  `.gna` author to track the dependency's embedding layout would be
  hostile.
- **Arity-mismatch severity (E10).** **Resolved: GN003 (error).**
  Consistent with the existing "malformed `.gna` → GN003" behaviour. The
  contract is still dropped and the rest of the check proceeds
  (§4.4.2), so the blast radius is one exit code, not a halted check.

---

**Locked decisions (summary)**

| Decision | Choice |
|----------|--------|
| External keyed ctor: missing `!` field | GN002 |
| External keyed ctor: explicit `nil` in `!` field | GN001 |
| External unkeyed ctor | unchecked (firewall stays) |
| `new(pkg.T)` / `&pkg.T{}` / `pkg.T{}` zero-value walk | GN002 on `!` fields (one hop; promoted-from-embedded included) |
| External `!` field selector | non-nil source (GW001 / usable) |
| External `!` field `= nil` | GN001 |
| External result `!T` | non-nil source (unchanged) |
| `receivers:` at call sites | reserved, no effect in v1.5 |
| Interface external positions | Interface-RFC semantics (D1–D6) |
| `.gna` classifies interface vs concrete | No — `go/types` does |
| `.gna` arity ≠ Go arity | GN003, contract dropped |
| `.gna` names absent symbol | GW004 (warning, new code) |
| `.gna` `methods:` key vs. promoted methods | accepted (full method set) |
| Duplicate keys / one file per package | hard load error (unchanged) |
| Schema bump | No (remain 1) |
| Generic external contracts | out of scope |
