# Gon Roadmap: v1.3 and Beyond

**Status:** Provisional working release plan. Release boundaries beyond v1.4
may shift based on RFC outcomes, but the sequencing principles (R1–R4) and
dependency graph are locked.

## 1. Semantic Invariants (I1–I7)

These are the normative gate for every feature on this roadmap. A feature
that violates an invariant is REJECTED or must propose an invariant change
via its own RFC.

- **I1 — No flow-sensitive nilability.** Gon does not change nilability
  state based on control-flow path. Scope: nilability-state only; generic
  reachability/dead-code analysis is out of scope for this invariant.
- **I2 — Explicit contracts are authoritative.** Non-nil guarantees come
  from explicit commitment, never body inference.
- **I3 — Go semantics remain authoritative.** `go/types` is the authority
  for type identity, assignability, conversions, method sets, and ordinary
  Go semantics.
- **I4 — `.gna` is authority only for external nilability contracts.**
  Identity and ordinary type semantics remain under I3, including for
  packages whose contracts are described by `.gna`.
- **I5 — `!T` is a compile-time guarantee**, not a new runtime
  representation or runtime nil-check.
- **I6 — No implicit contract propagation** through conversion, assertion,
  or ordinary assignment. `!T → T` may lose the guarantee; `T → !T`
  requires a source that satisfies the contract at that point.
- **I7 — Schema versioning is syntax-driven.** Semantic/feature changes do
  not by themselves bump the `.gna` schema version. `Requires .gna change
  = Yes` and `Schema bump = No` is a valid combination.

## 2. Release Sequencing Principles (R1–R4)

- **R1 — Semantic Isolation.** A potentially breaking semantic RFC MUST be
  released independently from unrelated semantic RFCs so its compatibility
  impact can be observed in isolation.
- **R2 — Non-breaking Bundling.** Non-breaking extensions MAY be bundled
  when they form a coherent maintenance/tooling/construction story.
- **R3 — Dependency Preservation.** Release slicing MUST NOT weaken or
  bypass dependencies established by the locked dependency graph.
- **R4 — No Artificial Versioning.** Independent non-breaking work does
  not require a dedicated minor release merely because it constitutes a
  separate milestone.

A feature's `Breaking = Possibly Yes` label does not automatically mean a
solo release — that determination is made by the RFC's own compatibility
analysis (see v1.6, §4).

## 3. Milestone Map

| ID | Scope | Dependency / Gate |
|---|---|---|
| M1a | Interface semantics + method-set contracts | Core — **shipped v1.4** |
| M1b | Type Coverage semantics | Core — **shipped v1.6** |
| M2a | Struct-only construction + diagnostics + warnings | Core |
| M2b | Element-contract construction subcases | M1b — **shipped v1.7** |
| M3 | Generic `!T` + instantiated generic contracts | M1a (full) + M1b named-type nilability semantics — **shipped v1.8** |
| M4a | Cross-package / External resolution + non-interface application | Core + basic `.gna` model — **shipped v1.5** |
| M4b | Interface-typed contract application | M1a + M4a application model — **shipped v1.5** |
| M5a | `.gna` validation + external contract boundary | M4a — **shipped v1.5** |
| M5b | Generic `.gna` + generic contract application | M3 + M5a infrastructure |
| M6a | `gon fmt` + `gon check` | Core |
| M6b | LSP / editor support | M2a — contract tracing |

Full feature inventory and dependency graph rationale are maintained
separately in the project's design-session history and are not duplicated
here.

## 4. Release Plan

### v1.3 — Foundation & Tooling
**Scope:** M2a + M6a + M6b — construction completeness, diagnostics,
warnings, CLI (`gon fmt`, `gon check`), and LSP/editor support.
**Dependency:** Core only.
**Breaking risk:** None.
**Rationale (R2/R4):** All non-breaking. No reason to withhold or split
into separate releases — this is a coherent "strengthen the existing
checker/toolchain" story.

### v1.4 — Interface Semantic Gate — SHIPPED (2026-09-05)
**Scope:** M1a — Interface RFC (`!I` semantics, typed-nil behavior,
method-set contracts). Implemented per `docs/rfc-interface-semantics.md`
(Status: Accepted). Observed compatibility impact: one new rejection —
ordinary `I` → `!I` (with `nil` and `x.(!I)` in the same position). No
`.gna` schema bump. No change to code that does not annotate an interface
type with `!`.
**Dependency:** Core.
**Breaking risk:** Possibly Yes — highest fan-out of any pending RFC.
**Rationale (R1):** Solo release, mandatory. This decision gates M3, M4b,
and downstream ecosystem work; its compatibility impact must be observable
in isolation before anything is built on top of it.

### v1.5 — Ecosystem Contract Expansion — SHIPPED (2026-09-06)
**Scope:** M4a + M4b + M5a — cross-package and external `.gna`
resolution/application (non-interface, then interface-typed), plus `.gna`
validation against `go/types` (arity → GN003 + contract drop; unknown
symbol → new **GW004**) and the external contract boundary.
**RFC:** `docs/rfc-ecosystem-contract-expansion.md` (Status: Accepted —
E1–E14 + §10 locked 2026-09-06). One-hop external structural traversal;
`--strict` deferred to a follow-on; `receivers:` still reserved.
**Dependency:** M4a: Core + basic `.gna` model. M4b: M1a (v1.4, shipped). M5a: M4a.
**Breaking risk:** M4a/M4b: Possibly Yes (ecosystem compatibility
exposure). M5a: non-breaking.
**Rationale (R1, applied correctly):** M4b is *application* of a semantic
decision already locked in v1.4, not a new semantic RFC — it does not
independently trigger R1 isolation. Bundling M4a+M4b+M5a as one coherent
"contracts cross package boundaries" story is intentional sequencing, not
readiness-driven bundling. M4a was deliberately not pulled into v1.3
despite being dependency-eligible from Core alone, to preserve each
release's single clear character.

### v1.6 — Type Coverage — SHIPPED (2026-09-06)
**Scope:** M1b — Type Coverage RFC (`!func`, `!map[K]V`, `![]T`,
`!chan T`, named nilable types, aliases).
**RFC:** `docs/rfc-type-coverage.md` (Status: Accepted — decision matrix
C1–C14 + §10 O1–O7 locked 2026-09-06, owner confirmed all recommendations).
**Dependency:** Core.
**Breaking risk:** One materially breaking sub-feature (C5 option P: bare
`var x !S` with no initializer → GN002, uniform across nilable kinds,
closes the pre-existing silent `!*T` gap). Every other sub-feature (C3,
C6, C7, C9, C11) is non-breaking in practice.
**Rationale:** Defaults to solo per R1, and the RFC's §13 recommendation is
to keep it solo: C5(P) is the isolation-worthy change; the non-breaking
rows ride along as the coherent "`!` means the same thing on every nilable
type" story (R2). If the owner takes C5 option Q (status quo) the release
becomes fully non-breaking and folding M1b into v1.7 with M2b is on the
table (RFC O1/O7).

### v1.7 — Element-Contract Construction — SHIPPED (2026-09-07)
**Scope:** M2b — element-contract construction subcases.
**RFC:** `docs/rfc-element-contract-construction.md` (Status: Accepted —
E1–E12 locked, O1/O4/O5 deferred, 2026-09-07).
**Dependency:** M1b element-level semantics (v1.6).
**Breaking risk:** Non-breaking — every new diagnostic (GN001 nil element,
GN002 fixed-array element zero-fill, GN003 malformed element `!`) fires
only on syntax v1.6 rejected. `.gna` schema stays 1.
**Rationale (R2):** Follow-on extension; semantic risk was already
absorbed in v1.6, so no isolation needed here.

### v1.8 — Generic Semantics — SHIPPED (2026-11-14)
**Scope:** M3 — Generic RFC (`!T` type parameters, instantiated generic
contracts).
**RFC:** `docs/rfc-generic-semantics.md` (owner decisions O1–O7 accepted
2026-11-13; implemented 2026-11-14 — two corrections in RFC §13.1 confirmed
by the owner 2026-11-14).
**Dependency:** M1a full (v1.4) + M1b named-type nilability semantics (v1.6).
**Breaking risk:** Possibly Yes.
**Rationale (R1):** Solo release. Independent compatibility exposure even
though it depends on prior RFCs.

### v1.9 — Generic `.gna` Ecosystem
**Scope:** M5b — generic `.gna` declarations + generic contract
application.
**RFC:** `docs/rfc-generic-gna.md` (Status: Draft — owner decisions
all owner decisions recorded 2026-11-14 (Q2/Q3 after an exercise); implementation pending). Measured baseline: most generic
`.gna` application already works by name + position; the release closes the
explicit-instantiation inconsistency and validates `.gna` `!` positions
against `go/types` (type parameters via the M3 nilable rule; concrete kinds
per `gna-spec-v1.md`).
**Dependency:** M3 (v1.8) + M5a infrastructure (v1.5).
**Breaking risk:** Possibly Yes (inherited from M3).
**Rationale (R1):** Follows the isolation of M3 as its direct consumer.

## 5. Open Sequencing Notes

- M4a is dependency-eligible as early as Core, independent of M1a. It is
  placed in v1.5 for release coherence (R2/R4), not because the graph
  requires it. This could be revisited if ecosystem demand justifies an
  earlier standalone release.
- v1.6 onward are planned, not committed. RFC outcomes, scope changes, or
  implementation reality may shift these boundaries without violating R1–R4
  or the underlying dependency graph.
- **Go 1.27 generic methods (toolchain resolved in v1.8.1; contracts open).**
  Go 1.27 allows methods with their own type parameters
  (`func (b Box[X]) Map[Y any](…)`). Gon parses with the `go/parser` of the
  toolchain that builds it, so a Gon built with Go 1.25 rejected such source
  at parse time while one built with 1.27 accepted it — the same Gon source
  behaved differently depending on the builder's Go version (verified
  2026-11-14 against v1.8.0). **v1.8.1 raises the build minimum to Go 1.27.0**
  (`go.mod`, CI, release), so every Gon build parses the syntax. This is a
  build-level change, not a language-compatibility one: accepting the syntax
  is not breaking (it was a parse error before), and a user's *own* Go
  version does not gate it — checking was observed to produce identical
  diagnostics with the user's `go` at 1.21.13 through 1.27.0.
  - **What already works** with a 1.27 build (checked 2026-11-14): body-level
    checks on a generic method — `!Y` on a method type parameter follows the
    G2 nilable rule (`Y any` → GN003) and `return nil` into a `!*Y` result is
    GN001.
  - **What remains open**: call-site contracts for methods (parameters and
    results) are untracked for *every* receiver, generic or not
    (`rfc-generic-semantics.md` §13.1), so a generic method's contracts are
    not enforced at its callers. Defining that needs its own RFC; candidate
    slot: with or after M5b (v1.9). Once contracts start being checked on code
    that was previously unchecked, R1/G11-style compatibility analysis
    applies.
- **A "nilable" constraint (open, unscheduled — noted 2026-11-14).** Go has no
  named constraint or supertype for "a type whose zero value is `nil`":
  `nil` is typed per kind (pointer, slice, map, chan, func, interface), and
  the only way to say it is to enumerate kinds in a type set. Verified
  against Go 1.27.0: `x == nil` compiles for a type parameter whose type set
  holds only nilable kinds (`*A | []int | map[string]int`, `~*A`) and is
  rejected for `any` and `comparable`. Gon's nilable-type-parameter rule
  (`rfc-generic-semantics.md` §3) is the same boundary, so `!X` on `X any`
  being GN003 is not an arbitrary Gon rule.
  - **Consequence.** A generic `!` over a pointer / slice / map value is
    written `[P ~*E, E any](p !P)`, `[S ~[]E, E any](s !S)`,
    `[M ~map[K]V, K comparable, V any](m !M)` — the idiom the standard
    library uses (in `slices` / `maps`, 64 of the 79 bare type-parameter
    positions satisfy the rule). When the caller's named type need not be
    preserved, the plain forms `!*X`, `![]E` need no type parameter at all.
  - **A library can name the common shapes — no Gon change needed.** Verified
    2026-11-14 against Gon v1.8.1: named constraints such as
    `type Ptr[E any] interface{ ~*E }`, `Slice[E]`, `Map[K, V]` and `Chan[E]`
    are recognised by the nilable-type-parameter rule (no GN003 on
    `func F[P Ptr[E], E any](p !P)`), `E` is still inferred, and a named slice
    type is preserved. It shortens `P ~*E` to `P Ptr[E]` and gives the intent
    a name; it does **not** remove the second type parameter.
  - **What a library cannot do.** One `Nilable` constraint spanning every kind
    and element type: element types must be written out, so a union such as
    `~*int | ~[]int | …` only admits those exact elements (Go rejects
    `[]string` as `does not satisfy Nilable`). Covering the general case would
    need a language-level constraint (not Gon's to add) or new Gon syntax in a
    constraint position, which is reserved as GN003 (G9 / O5 of
    `rfc-generic-semantics.md`) and needs its own RFC; it also interacts with
    I3 (Go owns type identity and constraints) and I7 (schema bumps are
    syntax-driven).
  - **Open design questions for the library route.** Where it lives (a tiny
    separate module vs a package in the Gon module, which would pull Gon's
    dependencies into users' `go.mod`); it carries no runtime code, so I5
    (`!T` is compile-time only) holds. No candidate release; revisit if the
    `[P ~*E, E any]` form proves a real adoption cost in practice.
