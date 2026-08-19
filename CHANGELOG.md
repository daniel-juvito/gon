# Changelog

## [1.3.0] — 2026-08-19

### Added

- **M2a construction completeness**
  - `new(T)` is a zero-value construction site when `T` is a local struct (GN002).
  - Unkeyed composite literals use declaration order from the *local* struct AST only.
  - External (`SelectorExpr`) types remain keyed-only + `.gna` (M4 firewall).
  - Unified construction entry points in `internal/checker/construction.go`.
- **ContractTrace** — first-class diagnostic data on GN001/GN002 (wire `String()` unchanged).
- **GW003** — redundant type-assertion warning on declared `!T` identifiers only.
- **`gon fmt`** — format Gon source in place.
- **`gon lsp`** — minimal stdio LSP.

### Changed

- `gon check` remains canonical; `vet` stays a compatibility alias.
- `gon version` reports `1.3.0`.

### Compatibility

- No `.gna` schema bump (schema remains **1**).
- Existing GN001/GN002/GW001 codes and severities preserved.
- Scope firewall: no interface `!I`, no type coverage, no generics, no flow-sensitive nilability.

## [1.2.1] — 2026-08-18

### Fixed

- **Preprocessor: `!` in multi-result function signatures.** Local forms such
  as `func Open() (!*string, error)`, `func Open() (*string, !error)`, and
  `func F(a !*T) (!*U, error)` are recognized as type modifiers. Parameter
  and parenthesized result lists (including interface methods) are tracked so
  unary `!` in expressions (`f(a, !b)`, `(!flag)`) is not stripped.

### Compatibility

- No checker semantics change relative to v1.2.0.
- Field contracts and return-value contracts unchanged.
- `.gna` schema remains **1**.

> Gon v1.2.1 is a preprocessor bugfix so valid local multi-return `!`
> syntax works as intended since v1.1.

### Changed

- `gon version` reports `1.2.1`.

## [1.2.0] — 2026-08-17

### Added

- **Field contracts.** A `!T` field is an invariant of its declared storage
  location. Enforced at three local sites only:
  - **Construction (GN002):** zero-value containment walk over struct,
    embedded, fixed array, and anonymous types; stops at every indirection
    boundary (`*T`, `[]T`, `map`, `chan`, `interface`, `func`).
  - **Mutation (GN001):** explicit `nil` assigned into a `!T` field
    (`s.Client = nil`). Ordinary values remain accepted (conservative).
  - **Selector (non-nil source):** selecting a `!T` field yields a non-nil
    source for immediate use (GW001 on nil comparison).
- **`.gna` `types:` section.** External packages may declare field contracts:
  ```yaml
  types:
    Config:
      fields:
        Client: "!*http.Client"
  ```
- Spec: [docs/rfc-field-contracts.md](docs/rfc-field-contracts.md).

### Compatibility

- `.gna` schema remains **1** (no format change; `types:` was already
  syntactically open under schema 1).
- Programs accepted by v1.1 remain accepted unless they construct or mutate
  a value that violates a newly annotated `!` field.
- No flow analysis, no path sensitivity, no reconstruction of object state.
- Ordinary value into `!T` field remains accepted.

> Gon v1.2 makes `!T` fields storage invariants checked at construction,
> mutation, and selector use — still local and non-flow-sensitive.

### Changed

- `gon version` reports `1.2.0`.
- Scope document updated for v1.2 semantics.

## [1.1.0] — 2026-08-17

### Added

- **Return-value contracts.** An explicitly annotated `!T` result position
  (in `.gna` or a local Gon signature) becomes a non-nil source at its
  immediate use site (`:=` or `var` without an explicit type). Observable via
  GW001 on nil comparison.
- Spec: [docs/rfc-return-value-contracts.md](docs/rfc-return-value-contracts.md).

### Compatibility

- `.gna` schema remains **1** (no format change).
- Ordinary (unannotated) values assigned to `!T` remain accepted.
- No flow analysis, no conditional contracts, no propagation through
  conversion / type assertion / assignment from names.
- Explicit type on a binding always wins over a return contract.

> Gon v1.1 adds explicit return-value contracts without introducing flow
> analysis. Annotated `!T` return positions become non-nil sources at their
> immediate use sites; existing ordinary-value acceptance remains unchanged.

### Changed

- `gon version` reports `1.1.0`.
- Scope document updated for v1.1 semantics.

## [1.0.0] — 2026-08-16

Initial public release.

- Local non-flow-sensitive nilability checks (`!T`).
- External packages via `.gna` + module-aware `go/packages`.
- Conservative reference annotations for `io` and `os`.
