# RFC: Type Coverage for Non-Nil Contracts (Gon v1.6 / M1b)

**Status:** Draft — decision matrix NOT yet confirmed by the owner
**Target:** Gon v1.6
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`, `docs/roadmap-v1.3-plus.md`,
`docs/rfc-interface-semantics.md`, `docs/rfc-field-contracts.md`,
`docs/rfc-return-value-contracts.md`, `docs/rfc-ecosystem-contract-expansion.md`
**Date:** 2026-09-06

## 1. Motivation

Gon's `!` modifier has been defined, one Go type kind at a time:

- **v1.0–v1.3** — `!*T` (and, structurally, `!T` on any type that reduces to
  a pointer). The contract is "the pointer value is non-nil".
- **v1.4 (M1a)** — `!I` for interface types. The contract is "the interface
  value is non-nil"; the dynamic value is explicitly *not* constrained
  (`docs/rfc-interface-semantics.md`).

The remaining Go type kinds whose zero value is `nil` have never been given
a normative meaning under `!`:

| Kind | Example | Zero value | `!` meaning today |
|------|---------|-----------|-------------------|
| slice | `[]T` | `nil` | undefined |
| map | `map[K]V` | `nil` | undefined |
| channel | `chan T`, `<-chan T`, `chan<- T` | `nil` | undefined |
| function value | `func(...)` | `nil` | undefined |
| named nilable type | `type Handler func()` | `nil` | undefined |
| alias of the above | `type Bytes = []byte` | `nil` | undefined |

The preprocessor already *strips* `!` from these positions (it keys off the
token preceding `!`, not the type that follows), and the checker already
carries a generic `nonNilOffsets` set. As a result the current behaviour is
**accidental, not specified**: `var s ![]byte = nil` happens to produce
GN001, `var s ![]byte` (no initializer) happens to produce nothing, and
`s == nil` where `s` is a `![]byte` field happens to produce GW001 — none of
this is written down, tested as a contract, or guaranteed to be coherent
across the six kinds above.

M1b — "Type Coverage" — closes that gap. It gives `!` a single, uniform
meaning across every nilable Go type kind, defines the construction /
mutation / use sites for each, and fixes the named-type and alias behaviour
that M3 (generics) and M2b (element contracts) both depend on.

This RFC deliberately mirrors the structure and the boundary of
`docs/rfc-interface-semantics.md`: `!` constrains **the value represented by
the annotated Go type**, and nothing reachable *through* that value. For a
slice that is the slice header; for a map, the map itself; for a channel,
the channel value; for a function value, the function value. Element
nilability, length, emptiness, channel buffering / direction / open-state,
and any form of flow analysis remain out of scope.

## 2. Terminology

- **Reference value** — the value directly represented by a slice, map,
  channel, or function type: the slice header, the map, the channel, or the
  function value. This is the thing `!` constrains.
- **Backing / contained data** — the backing array of a slice, the entries
  of a map, the elements sent over a channel, the closure of a function.
  `!` says nothing about these.
- **Nilable kind** — a Go type whose underlying type is a pointer, slice,
  map, channel, function, or interface type. These are the only kinds on
  which `!` is meaningful. (`unsafe.Pointer` is treated as a pointer.)
- **Empty vs nil** — a slice or map may be non-nil and have length 0
  (`[]T{}`, `make(map[K]V)`). `!` distinguishes nil from non-nil; it does
  not distinguish empty from non-empty.

## 3. Decision Matrix

Each row records a **proposed** decision and the alternative(s) considered.
Nothing here is locked until the owner signs off (see §10). The normative
rules in §4 MUST NOT introduce semantics beyond the confirmed decisions.

### C1 — Meaning of `!` per kind

| Annotated type | Proposed meaning of `!` | Alternative — rejected |
|----------------|------------------------|------------------------|
| `![]T` | the slice header is non-nil (length unconstrained, may be 0) | "non-empty slice" — this is a length contract, not a nilability contract; deferred as a non-goal |
| `!map[K]V` | the map is non-nil (allocated; entry count unconstrained) | "map has ≥1 entry" — length contract; non-goal |
| `!chan T` | the channel value is non-nil (allocated) | "channel is open" / "buffered" — runtime/stateful; non-goal |
| `!func(...)` | the function value is non-nil (bound to a function or closure) | n/a |
| `!Named` (named nilable type) | the value of `Named` is non-nil; **which** rule above applies is decided by `Named`'s underlying kind | treat `!Named` as opaque — rejected; breaks M3 dependency |
| `!Alias` (alias) | identical to `!` on the aliased type (Go alias = type identity) | give aliases independent semantics — rejected; contradicts I3 |

**Proposed: LOCKED as written.** `!` is a reference-value nilability
contract for every nilable kind, exactly as `!I` is an interface-value
nilability contract. The guarantee is uniform; only the Go value it names
differs.

### C2 — Not a length, emptiness, or state contract

`![]T` and `!map[K]V` guarantee non-nil only. The checker MUST NOT treat a
`!` slice/map as non-empty, and MUST NOT report `len(x) == 0`,
`for range x {}`, or an empty composite literal as a violation.

`!chan T` guarantees non-nil only — never open-ness, buffering, or
direction.

**Proposed: LOCKED.**

### C3 — Nil literal → GN001

Assigning, passing, returning, or writing the `nil` literal into a `!`
position of any nilable kind is **GN001**. This already fires for `![]T`
etc. via the shared nil-literal path; this RFC makes it a specified
guarantee for all six kinds and adds regression coverage.

**Proposed: LOCKED.**

### C4 — Sources that satisfy a `!` reference contract

| Source expression | Satisfies `!`? | Rule |
|-------------------|----------------|------|
| composite literal `[]T{…}`, `[]T{}`, `map[K]V{…}`, `map[K]V{}` | yes | a composite literal always produces a non-nil reference value |
| `make([]T, …)`, `make(map[K]V, …)`, `make(chan T, …)` | yes | `make` never returns nil |
| function literal `func(…){…}` | yes | a function literal is always non-nil |
| a declared function or method name, or a method value `x.M` | yes | a function object is non-nil |
| `&T{}` where the target is `!*T` | yes (pre-existing pointer rule) | unchanged |
| another `!S` value of an **identical** Go type (`types.Identical`) | yes | same as `!I` → `!I` (D3d in the interface RFC) |
| an ordinary (unannotated) value of the same kind | **accepted, conservatively** | monotonic with v1.0–v1.5: ordinary → `!` is not a diagnostic; only literal `nil` is. This is the absence of a contract, not flow analysis |
| `append(s, …)`, `s[i:j]`, `<-ch`-derived, conversion `[]T(x)`, type assertion | **no `!` propagation** | see C6 |

**Proposed: LOCKED.** Note the deliberate asymmetry retained from v1.4: an
ordinary value is *accepted* into a `!` slot (conservative, no tightening),
but it does **not** become a `!` source for onward propagation.

### C5 — Zero-value declaration (`var x !S` with no initializer)

This is the one materially breaking decision and the core of "type
coverage".

```go
var s ![]byte        // proposed: GN002 (reference value is nil at the zero value)
var m !map[string]int // proposed: GN002
var p !*Config        // proposed: GN002 — closes a pre-existing silent gap
```

Today a bare `var x !S` at file or block scope produces **no diagnostic**
for any nilable kind (the zero-value walk in `internal/checker/fields.go`
only descends into struct fields; it returns immediately for
`*ast.StarExpr`, `*ast.MapType`, `*ast.ChanType`, `*ast.FuncType`, and a
lenless `*ast.ArrayType`). Field contracts (v1.2, RFC §7) already took the
hard line for the *field* case — `var s S` where `S` has a `!` field is
GN002 — but the top-level value case was never covered.

| Option | Assessment |
|--------|------------|
| **P: `var x !S` with no non-nil initializer → GN002**, uniformly for pointer/slice/map/chan/func/named/alias | Consistent with the field-contract "declare-then-populate is GN002" precedent. Closes the `!*T` gap as a side effect. Materially breaking — see §9. **Proposed.** |
| Q: leave bare `var x !S` silent (status quo) | Keeps v1.6 non-breaking, but "type coverage" then adds no *value-level* guarantee — `!` on a local slice would mean nothing until first use. Rejected. |
| R: GN002 for slice/map/chan/func but keep `!*T` silent | Inconsistent for no principled reason. Rejected. |

An explicit initializer is evaluated as usual: `var s ![]byte = f()` is
accepted (conservative rule, C4), `var s ![]byte = nil` is GN001 (C3),
`var s ![]byte = []byte{}` is accepted.

**Proposed: P (GN002, uniform, closes the `!*T` gap).**

### C6 — No propagation through derived expressions

The following produce an **ordinary** value of the kind, never a `!` value,
regardless of operand:

- `append(s, …)` — even if `s` is `![]T`
- slice expressions `s[i:j]`, `s[i:j:k]`
- conversion `[]T(x)`, `map[K]V(x)`, `T(x)` for a named nilable `T`,
  and directional channel conversion `(<-chan T)(ch)`
- type assertion `x.(S)`

**Proposed: LOCKED.** This is the same "conversion / assertion do not
propagate" boundary already locked in v1.1, v1.2, and v1.4 (interface RFC
§3.9, D6). `append` is included explicitly because it is the tempting
special case: `append` on a non-nil slice does stay non-nil at runtime, but
tracking that is flow analysis and is refused (I1).

### C7 — `!S` as a type-assertion target

```go
x.(![]T)     // proposed: GN001 + strip (assertion does not establish !)
x.(!*T)      // proposed: GN001 — folds in the amendment the v1.4 RFC deferred
```

The interface RFC (§3.8, implementation note in §10) already rejects
`x.(!I)` and explicitly *deferred* `x.(!*T)` "for a possible future RFC
amendment". This RFC is that amendment: `!` on **any** assertion target is
GN001, with the preprocessor stripping the `!` so the checker reports
cleanly instead of the parser erroring.

**Proposed: LOCKED (adopt the deferred amendment).**

### C8 — `!` reference value as a non-nil source at use sites

Selecting a `!` field, or binding a `!`-annotated result, of a nilable kind
yields a non-nil source under the **existing** return-value / field-contract
machinery: consumable at an immediate `!U` assignment, `!U` argument, or
receiver position, and `x == nil` / `x != nil` on it is **GW001**.

No new propagation engine. `!` slices/maps/etc. reuse exactly the source
mechanism concrete `!*T` already uses.

**Proposed: LOCKED.**

### C9 — Named types and aliases

- **Alias** (`type B = []byte`): `!B` is `types.Identical` to `![]byte`.
  There is nothing to decide — Go already makes them the same type. The
  preprocessor and checker treat the `!` as binding whatever the alias
  expands to.
- **Named defined type** (`type Set map[string]struct{}`): `!Set`
  constrains the value of `Set`. The underlying kind (`map`) selects the C1
  rule. `!Set` and `Set` remain the same Go type with the same method set
  and identity (I3) — `!` adds only the nilability contract.
- **Named type whose underlying is a struct/array** (not a nilable kind):
  `!Named` is **GN003** (a `!` on a non-nilable type is a malformed
  contract). This matches the spirit of "`!` is only meaningful on nilable
  kinds". *Open question O4: warning vs error vs silently-ignored.*

**Proposed: LOCKED for alias + named-nilable; O4 open for non-nilable named.**

### C10 — `.gna` representation and the outermost-binding rule

- `.gna` uses the same `Type.NonNil` representation. **No schema bump**
  (I7 — syntax is unchanged; `"![]byte"`, `"!map[string]*T"`,
  `"!func() error"` were already parseable strings).
- In a `.gna` type string, a leading `!` binds **only the outermost type
  constructor**. `"!map[string]*T"` means "the map is non-nil"; it says
  nothing about the `*T` values. `"![]*T"` means "the slice is non-nil".
  Element-level contracts (`"[]!*T"`, `"map[string]!*T"`) are **M2b /
  v1.7** and are a parse error / GN003 in v1.6.
- Authoring principle carries over verbatim from `docs/gna-spec-v1.md`: a
  `[]byte` parameter must not be marked `"![]byte"` merely because callers
  "usually" pass a non-nil slice. Mark it `!` only when the API contract
  guarantees it.

**Proposed: LOCKED.**

### C11 — Interaction with field contracts (v1.2 / v1.5)

A struct field whose own type is `![]T` / `!map` / `!chan` / `!func` is a
**direct `!` field**: construction leaving it at zero is GN002, `f = nil` is
GN001, `f` is a non-nil source. This already works today via
`isNonNil(field.Type)` and needs only specification + tests.

The zero-value **containment walk** is unchanged: the checker still does
**not** descend through a slice/map/chan/func/pointer field looking for
nested `!` fields (RFC field-contracts §6.1 table stands). `!` on the field
*itself* is checked; values *reachable through* the field are not.

**Proposed: LOCKED.**

### C12 — Channel direction

`!<-chan T` and `!chan<- T` are valid: `!` binds the channel reference
value non-nil irrespective of direction. A directional conversion
(`(<-chan T)(ch)`) is an ordinary conversion and does not propagate `!`
(C6). Send / receive / `close` on a `!` channel are not rewritten and get
no runtime check (I5).

**Proposed: LOCKED.**

### C13 — Breaking-change classification (per sub-feature)

Per the roadmap's provisional-isolation note for v1.6, each sub-feature is
classified so the owner can decide release slicing:

| Sub-feature | Compatibility impact | Isolation needed? |
|-------------|---------------------|-------------------|
| C3 — `nil` literal → `!` slice/map/chan/func | Already emitted (accidental); making it a guarantee is **non-breaking** in practice | No |
| C5 (P) — bare `var x !S` → GN002, incl. `!*T` | **Breaking.** Any declare-then-populate on a `!` local/global starts failing. Largest surface | **Yes** — this is what keeps v1.6 solo |
| C6 — no `append` / reslice / conversion propagation | Non-breaking (nothing propagated before) | No |
| C7 — `x.(!*T)` / `x.(!S)` → GN001 | Breaking only for code that wrote `x.(!*T)`, which was a **parse error** before v1.4 and undefined after. Negligible | No |
| C9 — named / alias `!` semantics | Non-breaking (previously undefined; no code could rely on it) | No |
| C11 — `![]T` etc. as a field | `nil`-into-field and zero-construction already fire for any `!` field; **non-breaking** | No |

**Proposed recommendation:** ship C5(P) as the semantic core of a **solo
v1.6** (R1). The non-breaking rows (C3, C6, C7, C9, C11) ride along in the
same release because they form the coherent "`!` now means the same thing
on every nilable type" story (R2) — they are not independently releasable
in a way that would matter. If the owner rejects C5(P) in favour of C5(Q)
(status quo), v1.6 becomes fully non-breaking and the roadmap note's
"bundle with M2b" option (fold M1b into v1.7) is on the table — see O1.

### C14 — Non-Goals

`!` on nilable kinds does **not** introduce:

1. element nilability (`[]!T`, `map[K]!V`, `chan !T`) — M2b / v1.7;
2. length / non-emptiness contracts for slices or maps;
3. channel state: open/closed, buffered/unbuffered, capacity;
4. flow tracking through `append`, reslice, `close`, `delete`, send/receive;
5. runtime nil-checks or any code rewrite (I5 — `!` is stripped);
6. propagation of `!` through conversion or type assertion;
7. generic `!T` type parameters or instantiated generic contracts — M3 / v1.8;
8. inference of `!` from a function body that returns `make(...)` or a literal;
9. any change to Go's zero values, assignability, identity, or method sets (I3).

## 4. Normative Rules

### 4.1 Uniform reference-value semantics

For a type `S` whose underlying kind is slice, map, channel, or function,
the declaration `!S` means:

> The reference value represented by `S` is guaranteed to be non-nil.

It does **not** mean that the backing array, entries, buffered elements, or
closure environment are non-nil, non-empty, or in any particular state.

For a pointer type the existing rule is unchanged: `!*T` means the pointer
is non-nil. For an interface type the v1.4 rule is unchanged: `!I` means the
interface value is non-nil.

### 4.2 Nil literal

`nil` never satisfies a `!` contract of any kind:

```go
var s ![]byte              = nil   // GN001
func f(m !map[string]int)          // f(nil) → GN001
func g() !chan struct{} { return nil } // GN001
type H func(); var h !H    = nil   // GN001
```

### 4.3 Satisfying sources

A `!S` position is satisfied by:

- a composite literal of a slice or map type (`[]T{…}`, `map[K]V{…}`,
  including the empty forms);
- `make([]T, …)`, `make(map[K]V, …)`, `make(chan T, …)`;
- a function literal or a function/method name or method value;
- an existing `!S` value whose static type is `types.Identical` to the
  target;
- an ordinary value of the same kind — **accepted conservatively**, but
  such a value is not itself a `!` source for onward use.

The checker MUST evaluate the static type of the *immediate* right-hand
expression. It MUST NOT look through a conversion or assertion to recover a
more favourable operand type (identical to the interface RFC §3.9 rule).

### 4.4 Zero-value declaration

*(Normative only if C5 option P is confirmed.)*

A declaration `var x !S` (file or block scope) with no initializer, or with
an initializer that is locally known to produce the zero value, is a
construction site. If `S` is a nilable kind, the reference value is nil at
the zero value, and the checker reports **GN002** at the declaration.

This applies uniformly to `!*T`, `![]T`, `!map[K]V`, `!chan T`, `!func(…)`,
and named / alias types whose underlying kind is nilable. It closes the
pre-existing gap in which `var p !*T` was silent.

An initializer that is not the zero value is evaluated under §4.2–§4.3.

### 4.5 No propagation through derived expressions

`append(s, …)`, slice expressions, conversions, and type assertions produce
ordinary values. A `!` contract is never carried onto their result even
when an operand is `!`-annotated.

### 4.6 Type-assertion target

A type-assertion target MUST NOT carry `!`. `x.(!T)` — for any `T`,
interface or concrete — is **GN001**. The preprocessor strips the `!` and
records the offset; the checker reports GN001 at that position.

### 4.7 Use as a non-nil source

A `!`-annotated field selector or result binding of a nilable kind is a
non-nil source under the existing return-value / field-contract rules
(immediate `!U` assignment, `!U` argument, receiver; `== nil` / `!= nil` →
GW001). No further propagation.

### 4.8 Named types and aliases

- An alias is the same type as its target; `!` binds the expansion.
- A named defined type with a nilable underlying kind: `!Named` constrains
  the `Named` value; the underlying kind selects §4.1's rule; identity and
  method set are unchanged.
- A named defined type with a non-nilable underlying kind (struct, array,
  basic): `!Named` is a malformed contract — **GN003** *(pending O4)*.

### 4.9 `.gna` outermost-binding rule

In a `.gna` type string a leading `!` binds only the outermost type
constructor. Element-position `!` inside a slice/map/channel type string is
rejected in v1.6 (reserved for M2b). Schema version remains **1**.

## 5. Examples

### 5.1 Slices

```go
func Handlers() ![]Handler {
    return []Handler{}          // OK — non-nil, length 0 is fine (C2)
}

var hs ![]Handler = Handlers()  // OK — result is a !source
if hs == nil {}                 // GW001 (C8)

var buf ![]byte                 // GN002 (C5-P) — nil at the zero value
var buf2 ![]byte = nil          // GN001 (C3)
var buf3 ![]byte = make([]byte, 0) // OK
x := append(hs, h)              // x is ordinary []Handler (C6)
var y ![]Handler = x            // OK — conservative (C4), y not a !source
```

### 5.2 Maps

```go
type Registry struct {
    byName !map[string]*Plugin  // direct ! field (C11)
}

_ = Registry{}                              // GN002 — byName is nil
r := Registry{byName: map[string]*Plugin{}} // OK — non-nil, empty is fine
r.byName = nil                              // GN001
for range r.byName {}                       // no diagnostic (C2)
```

### 5.3 Channels

```go
func Events() !<-chan Event {
    return make(chan Event)      // OK
}
var ch !chan struct{}           // GN002 (C5-P)
var done !chan struct{} = make(chan struct{}) // OK
close(done)                      // no rewrite, no check (C12 / I5)
```

### 5.4 Function values

```go
type Middleware func(Handler) Handler

func Chain(mw !Middleware) Handler { /* … */ }

Chain(nil)                       // GN001
Chain(func(h Handler) Handler { return h }) // OK — function literal
var mw !Middleware               // GN002 (C5-P)
```

### 5.5 Named types and aliases

```go
type Bytes = []byte             // alias
type JSON []byte                // defined type

var a !Bytes = nil              // GN001 — identical to ![]byte
var b !JSON                     // GN002 (C5-P) — underlying kind is slice
```

### 5.6 `.gna`

```yaml
functions:
  NewRouter:
    results: ["!*Router"]
  DefaultMiddleware:
    results: ["![]Middleware"]   # the slice is non-nil; elements unconstrained
types:
  Router:
    fields:
      routes: "!map[string]Handler"   # non-nil map; Handler values unconstrained
```

```go
mw := pkg.DefaultMiddleware()
if mw == nil {}                  // GW001 — annotated !result is a source
```

### 5.7 Assertion target (C7)

```go
v := x.([]byte)                 // OK
v := x.(![]byte)                // GN001
p := y.(!*T)                    // GN001 — the v1.4-deferred amendment
```

## 6. Compatibility

- **Schema version remains 1.** No `.gna` syntax change; `"![]T"`,
  `"!map[K]V"`, `"!func(…)"` were always legal strings whose semantics are
  now defined.
- Programs accepted by v1.5 remain accepted by v1.6 **except**:
  1. a bare `var x !S` declaration of a nilable kind with no non-nil
     initializer — now **GN002** (C5, option P), including the previously
     silent `!*T` case;
  2. `x.(!T)` as a type-assertion target — now **GN001** (was a parser
     error pre-v1.4, undefined since);
  3. an element-position `!` inside a `.gna` slice/map/channel type string —
     now a **GN003** parse rejection (reserved for M2b).
- Adding a `!` annotation to a slice/map/channel/function type or field is a
  deliberate strengthening by the annotation author, with the same
  monotonicity rule used since v1.1.
- Ordinary values assigned to `!` destinations of any kind remain accepted
  (no tightening — C4).
- Conversion, `append`, reslice, and assertion continue to neither create
  nor propagate `!` (C6).

## 7. Implementation Sketch

- **Preprocessor** — already strips `!` positionally for these kinds. Two
  additions: (a) recognise `!` after `.(` for a *concrete* target (already
  done for the interface case) and route all `x.(!T)` to the GN001 path
  (C7); (b) reject element-position `!` in `.gna` type strings during
  parse (C10).
- **`nonNilKind` helper** — given an `ast.Expr` type (or a `types.Type`),
  classify as pointer / slice / map / chan / func / interface / named-nilable
  / alias / non-nilable. Drives both C5 and C9/O4.
- **Zero-value walk** (`reportMissingNonNilFields`) — when C5(P) is
  confirmed, the top-level `var x !S` path in `registerPackageVars` /
  `checkLocalVarDecl` emits GN002 when `nonNilKind(S)` is a nilable kind and
  there is no non-nil initializer. The struct-field containment table
  (`fields.go` §6.1) is **unchanged** — do not start descending through
  slice/map/chan/func.
- **Source machinery** — reuse the existing return-value / field-contract
  non-nil-source path unchanged; a `![]T` result or field is a source
  exactly as `!*T` is.
- **`make` / composite-literal / func-literal recognisers** — small
  additions to the "is this expression a non-nil source" predicate so C4's
  positive list is complete (mostly for GW001 accuracy and `!S → !S`).
- **`go/types` stays authoritative** for identity, alias expansion, method
  sets, and directional-channel assignability. `!` never consults or alters
  any of that.
- **Named non-nilable target (O4)** — one check in annotation validation
  (`internal/checker/ecosystem.go` / `.gna` load) and in local type
  handling: `!` on a struct/array/basic underlying → the O4 outcome.

## 8. Test Requirements

Minimum regression suite (executable spec, mirroring
`interface_semantics_test.go` and `field_contracts_test.go`):

- `nil` literal → `![]T` / `!map` / `!chan` / `!func` at var-init,
  assignment, return, call-arg, and keyed struct-literal positions → GN001.
- `make(...)`, empty composite literal, function literal, function name,
  method value → each accepted into the matching `!` position.
- `[]T{}` / `map[K]V{}` accepted (C2 — empty is not a violation);
  `for range` and `len(x)==0` on a `!` slice/map produce no diagnostic.
- Bare `var x !S` for each of pointer/slice/map/chan/func/named/alias →
  GN002 (C5-P); with a non-nil initializer → accepted; with `= nil` →
  GN001.
- `var p !*T` with no initializer → GN002 (regression for the closed gap).
- `append(s, …)`, `s[i:j]`, `[]T(x)`, `x.(S)` → result is ordinary; a
  following `!U` assignment from it is accepted but the value is not a
  `!` source (no GW001 on its later `== nil`).
- `x.(![]T)` and `x.(!*T)` → GN001 (C7).
- `![]T` / `!map` / `!func` struct field: zero construction → GN002;
  `f = nil` → GN001; `f` selector → non-nil source (GW001 on `== nil`).
- containment walk unchanged: a `[]Inner` / `map[K]Inner` / `chan Inner` /
  `func()Inner` field where `Inner` has a `!` field → **no** GN002.
- alias: `type B = []byte`; `var x !B = nil` → GN001; `!B` and `![]byte`
  interchangeable at assignment.
- named defined type with nilable underlying: rules follow the underlying
  kind.
- named defined type with struct/array underlying + `!` → O4 outcome.
- `.gna`: `"![]T"` / `"!map[K]V"` / `"!func() error"` in `functions:` /
  `methods:` / `types:` load and apply; element-position `!`
  (`"[]!*T"`) → GN003.
- all existing v1.0–v1.5 tests remain green.

## 9. Migration Guidance

The only migration cost is C5(P): code that declares a `!` reference-value
local and populates it later.

```go
var routes !map[string]Handler   // GN002 under v1.6
routes = map[string]Handler{}
// …
```

becomes one of:

```go
routes := map[string]Handler{}   // construct non-nil up front
// or
var routes map[string]Handler    // drop the ! if late population is required
```

This is the same trade-off, and the same guidance, as the v1.2
declare-then-populate rule for `!` struct fields (field-contracts RFC §7.2).
Mechanical, one-time, and localised to code that has *opted in* by writing
`!`.

## 10. Open Questions for the Owner

The decision matrix in §3 records proposed answers; these are the points
that genuinely need a ruling before the RFC can move to Accepted. Where the
answer is "no preference", the §3 proposal is taken (same protocol as the
v1.5 RFC §10).

1. **O1 — C5: option P (GN002 for bare `var x !S`, breaking, closes the
   `!*T` gap, keeps v1.6 solo) vs option Q (status quo, non-breaking, opens
   the door to folding M1b into v1.7 with M2b)?** This is the release-shape
   decision. Proposed: **P**.
2. **O2 — Does C7 fold in the `x.(!*T)` amendment the v1.4 RFC explicitly
   deferred, or leave concrete assertion targets alone for now?** Proposed:
   fold it in (uniform "`!` on any assertion target is GN001").
3. **O3 — `append` on a `![]T`:** confirmed ordinary result (C6, no
   propagation), or is there appetite for the single narrow exception
   "`append` of a `!` slice stays `!`"? Proposed: **no exception** — it is
   flow analysis and violates I1.
4. **O4 — `!` on a named type whose underlying kind is non-nilable**
   (`type Point struct{X,Y int}` → `!Point`): GN003 error, GW-warning, or
   silently ignored? Proposed: **GN003** (malformed contract), consistent
   with "`!` is only meaningful on nilable kinds".
5. **O5 — GW001 wording** for `![]T` / `!map` / `!chan` sources — reuse the
   existing "x is non-nil; comparison with nil is always false", or
   kind-specific phrasing ("slice header is non-nil")? Proposed: reuse the
   existing wording; kind-specific text adds noise for no clarity.
6. **O6 — `!chan` and `close`:** confirm Gon emits **nothing** for
   `close(ch)` / send / receive on a `!` channel (no "close of a
   guaranteed-non-nil channel" lint). Proposed: **nothing** — state
   tracking is a non-goal (C2, C14.3).
7. **O7 — provisional roadmap slicing:** if O1 = P, confirm v1.6 stays
   **solo** (M1b only) and M2b remains v1.7. If O1 = Q, confirm the
   preference between "still ship M1b solo as a spec-only + tests release"
   vs "hold M1b and release M1b+M2b together as v1.6".

## 11. Alternatives Considered

| Alternative | Reason rejected |
|-------------|-----------------|
| `![]T` means "non-empty slice" | Conflates nilability with length; a different and much larger feature; breaks the "`!` = not nil" mental model established since v1.0 |
| `!map` means "map has been sized / has entries" | Same conflation; unknowable without flow analysis |
| Track `!` through `append` / reslice | Flow analysis; violates I1; makes diagnostics statement-order-dependent (field-contracts RFC §8) |
| Give `!chan` an open-state or buffering meaning | Runtime/stateful; `!` is a compile-time guarantee only (I5) |
| Leave `var x !S` silent (C5-Q) and call M1b "done" | `!` on a local reference value would then guarantee nothing until first use — no coverage, just syntax |
| Separate diagnostic codes per kind (`GN00x` for slices, another for maps) | Unnecessary; GN001/GN002/GN003 already carry a clear message with the type rendered |
| Schema bump to 2 for element-position `!` | Element contracts are M2b's problem; v1.6 only *reserves* the syntax |
| Independent semantics for type aliases | Contradicts Go type identity and I3 |

## 12. Invariant Check (I1–I7)

- **I1 (no flow-sensitivity)** — upheld: C6 refuses `append`/reslice
  tracking; C5 is a pure type-driven check at the declaration.
- **I2 (explicit contracts authoritative)** — upheld: no body inference
  (C14.8).
- **I3 (Go semantics authoritative)** — upheld: identity, aliases, method
  sets, channel direction all deferred to `go/types` (§4.8, §7).
- **I4 (`.gna` is authority only for external nilability)** — upheld: C10
  adds no new `.gna` power beyond nilability-by-position.
- **I5 (`!T` is compile-time only)** — upheld: C12, C14.5 — no rewrite of
  send/receive/close/range.
- **I6 (no implicit propagation)** — upheld: C6, C4 (ordinary → `!`
  accepted but not a source).
- **I7 (schema versioning is syntax-driven)** — upheld: semantics change,
  syntax does not, schema stays **1**.

---

**Proposed locked decisions (summary — pending owner sign-off)**

| Decision | Proposal |
|----------|----------|
| `!` on slice/map/chan/func = reference value non-nil only | Yes (C1) |
| Length / emptiness / channel-state contract | No (C2, C14) |
| `nil` literal → `!S` | GN001 (C3) |
| Ordinary value → `!S` | Accepted, not a source (C4) |
| Bare `var x !S` (no init) → GN002, incl. `!*T` | Yes — option P (C5, O1) |
| Propagation through `append` / reslice / conversion / assertion | No (C6) |
| `x.(!T)` any target | GN001 — folds in v1.4's deferred amendment (C7, O2) |
| Named nilable type / alias `!` semantics | Follow underlying kind / identity (C9) |
| `!` on non-nilable named type | GN003 (C9, O4) |
| `.gna` leading `!` binds outermost only; element `!` reserved | Yes (C10) |
| Schema bump | No (remain 1) |
| Runtime checks | No (I5) |
| Release shape | Solo v1.6 if O1 = P (C13) |
