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
| M1a | Interface semantics + method-set contracts | Core |
| M1b | Type Coverage semantics | Core |
| M2a | Struct-only construction + diagnostics + warnings | Core |
| M2b | Element-contract construction subcases | M1b — element-level semantics |
| M3 | Generic `!T` + instantiated generic contracts | M1a (full) + M1b named-type nilability semantics |
| M4a | Cross-package / External resolution + non-interface application | Core + basic `.gna` model |
| M4b | Interface-typed contract application | M1a + M4a application model |
| M5a | `.gna` validation + external contract boundary | M4a |
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

### v1.4 — Interface Semantic Gate
**Scope:** M1a — Interface RFC (`!I` semantics, typed-nil behavior,
method-set contracts).
**Dependency:** Core.
**Breaking risk:** Possibly Yes — highest fan-out of any pending RFC.
**Rationale (R1):** Solo release, mandatory. This decision gates M3, M4b,
and downstream ecosystem work; its compatibility impact must be observable
in isolation before anything is built on top of it.

### v1.5 — Ecosystem Contract Expansion
**Scope:** M4a + M4b + M5a — cross-package and external `.gna`
resolution/application (non-interface, then interface-typed), plus `.gna`
conflict/duplicate validation and external contract boundary.
**Dependency:** M4a: Core + basic `.gna` model. M4b: M1a (v1.4). M5a: M4a.
**Breaking risk:** M4a/M4b: Possibly Yes (ecosystem compatibility
exposure). M5a: non-breaking.
**Rationale (R1, applied correctly):** M4b is *application* of a semantic
decision already locked in v1.4, not a new semantic RFC — it does not
independently trigger R1 isolation. Bundling M4a+M4b+M5a as one coherent
"contracts cross package boundaries" story is intentional sequencing, not
readiness-driven bundling. M4a was deliberately not pulled into v1.3
despite being dependency-eligible from Core alone, to preserve each
release's single clear character.

### v1.6 — Type Coverage (provisional isolation)
**Scope:** M1b — Type Coverage RFC (`!func`, `!map[K]V`, `![]T`,
`!chan T`, named nilable types, aliases).
**Dependency:** Core.
**Breaking risk:** TBD after RFC compatibility analysis.
**Rationale:** Defaults to solo per R1, but this is provisional. The RFC's
own compatibility analysis will classify each sub-feature; fully
backward-compatible portions may be eligible for bundling with M2b in a
later release, while any genuinely breaking portion remains isolated.

### v1.7 — Element-Contract Construction
**Scope:** M2b — element-contract construction subcases.
**Dependency:** M1b element-level semantics (v1.6).
**Breaking risk:** Non-breaking, assuming M1b semantics are unchanged.
**Rationale (R2):** Follow-on extension; semantic risk was already
absorbed in v1.6, so no isolation needed here.

### v1.8 — Generic Semantics
**Scope:** M3 — Generic RFC (`!T` type parameters, instantiated generic
contracts).
**Dependency:** M1a full (v1.4) + M1b named-type nilability semantics (v1.6).
**Breaking risk:** Possibly Yes.
**Rationale (R1):** Solo release. Independent compatibility exposure even
though it depends on prior RFCs.

### v1.9 — Generic `.gna` Ecosystem
**Scope:** M5b — generic `.gna` declarations + generic contract
application.
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
