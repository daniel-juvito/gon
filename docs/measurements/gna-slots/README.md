# `.gna` slot measurement (plan section 5)

Material of the measurement requested by section 5 of
`docs/plan-explicit-nonnil-conversion-part-a.md`, kept for audit. It is evidence, not a specification and not an
implementation.

- `report.md`: the report (method, controls, results, comparison with the RFC, limits).
- `g9measure-main.go.txt`: the instrument. It is a scratch tool that was built inside a copy of this module
  (it imports `internal/gna` and `internal/preproc` and `golang.org/x/tools/go/packages`), and it is stored
  with a `.txt` suffix so that it is not part of the Go build.
- `raw/`: its output per module, mode and `.gna` setting, `<module>.m<0|1>.<with|without>.txt`.

Usage of the instrument (from the root of the module to measure): `G8_MODE=0` for Part A binding rules, default
for Part A+B, `G9_NOGNA=1` to switch the `.gna` model off, `G8_VERBOSE=1` for the decision log.
- `actual/`: `gon check --json .` of the five control modules (the "Actual" column of the controls).
