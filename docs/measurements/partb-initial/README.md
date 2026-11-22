# Part B, initial audit (scratch, UNAUDITED)

Basis: gon `main` 53ba48a (v1.21.0 + #78, #79), gon-examples ed6f6f1, gon-vscode bb8689c, real `gon v1.21.0`
for Part A controls. Part B is not implemented anywhere; every Part B number comes from a scratch instrument
(`pbm`, the g9measure tool updated: any `&operand` is a source, `(!T)(x)` is a source). Nothing in a repository
was changed.

## Controls (instrument mode 0 = Part A, against the real `gon check`)

| Module | Real | Instrument | Explained |
|---|---|---|---|
| gon `./...` production | 0 (`ok`) | 0 | |
| flow-ok, field-contracts, ecosystem-contracts, type-coverage, element-contracts, generics | 1, 2, 6, 1, 1, 1 | same | |
| generic-gna, vscode genericgna | 2, 2 | 4, 4 | the instrument does not exclude a type-parameter slot of a `.gna` parameter (`cache.Must(&x)`); the real checker is silent (OQ-2) |
| method-contracts | 0 | 1 | the instrument shifts the argument index of a method expression `(*Queue).Push(q, nil)` (the real checker fixed this in #75) |

After the two explained artefacts mode 0 equals the real checker on every module (examples 14, vscode 2).

## Part B (mode 1), corrected for the same two artefacts

| | Part A | Part A+B |
|---|---|---|
| gon production GW006 | 0 | 1 (`internal/checker/method_contracts.gon:54`, `x = c.pkg.files`, assignment into a bound local; the same site as in the RFC) |
| gon-examples GW006 | 14 | 3 (flow-ok, element-contracts, generics: calls and `ident` that Part B does not bind) |
| gon-vscode GW006 | 2 | 0 |
| new bindings (`:=`/untyped var) | 0 | gon production 91 (12 already bound today), gon-examples 9, gon-vscode fixtures 1 |
| new GN001 (`x = nil` on a bound local) | 0 | 0 (the one the instrument labels "NEW" is `c.DB = nil` on a `.gna` field, already GN001 today at demo.gon:34) |
| new GW001 (`x == nil` on a bound local) | | none beyond those that exist today |

## Differences with the RFC text, and their reconciliation

Reconciled after the first version of this report (read-only reruns of the same instrument, scratch):

- **gon production, 76 (RFC) against 91.** The instrument with the old rule (`&x` of a non-literal is not a source) on
  `v1.20.0` gives **76**, the RFC figure. With the current rule (any `&operand` is a source, decided 2026-11-22) it gives
  **78** on `v1.20.0` (+2). On `main` it gives **91** (+13: the code added by Part A, for example `flow.gon` with 40 `:=`).
  At `ae2ca4d` it also gives 91. So 76 -> 78 -> 91 is the rule change and new code; no unexplained residue.
- **gon-examples, 5 (RFC) against 9.** The earlier instrument could load only the five modules that have a `go.mod`; their
  figures now are 3 + 1 + 0 + 0 + 1 = **5**, the RFC figure. The other 14 single-file demos, which are now measured in a
  synthesised module each, add **4**. The `&operand` rule does not change this figure (9 with either rule). gon-vscode: 1 and 1.
- **Not reconciled: RFC text "2 and 2" and "10 and 4".** With `&operand` recognised, the `cache.Must(&x)` flows are silent
  by the real rule (gon-examples 14 and 3, gon-vscode 2 and 0 in the table above); the RFC section 5.1 and 11 still print the
  old figures in places. This report does not change the RFC.

## Error of mine found during this audit (not Part B)

PR #20's description and section 0 of `docs/release-readiness-part-a.md` (#79) say 15 GW006 in gon-examples. The sum of the seven
figures is 14 (1+2+6+1+1+1+2), as expected.txt and the real checker give. The per-example figures are right; the total is not.

## Integrity of the evidence

`sha256sum -c raw.sha256` on this commit: 46 of 46 files OK, none failed (46 raw files + `README.md`, `pbmeasure-main.go.txt`
and `raw.sha256` = the 49 files of the change). The raw output was not regenerated from scratch; only the counts quoted in the
reconciliation above were re-run.

## Limits

- **Variadic parameters are not analysed by the instrument.** The loop over the arguments of a call stops at the position of a
  variadic parameter (`pbmeasure-main.go.txt`, `if i >= len(s.params) || sg.Variadic() && i >= sg.Params().Len()-1 { break }`),
  so an argument that goes into a variadic `!` parameter would be missed. It changes no figure here: no `.gon` source of the
  three repositories has a `...!T` parameter and no `.gna` file has a variadic. Parameters before the variadic one are analysed.
- Part A mode 0 equals the real checker on every module; that is not a proof of mode 1, which no checker implements. The
  Part B classification was not read site by site.

Instrument, not a checker; no type-identity rule except where noted; `range`, closures and channel sends not modelled; only
the three repositories; classification read from the output, not every site by eye.
