# Changelog

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
