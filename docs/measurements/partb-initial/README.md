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

## Differences with the RFC text, not reconciled

- RFC says 76 new bindings in gon production; the instrument now says 91 (+12 already bound). The code grew, and the instrument
  counts `:=` of nilable kinds the same way as before, but I have not shown that 76 -> 91 is only growth.
- RFC says 5 new bindings in gon-examples and 1 in gon-vscode; the instrument says 9 and 1. Unexplained (the earlier count may have
  left out the single-file demos).
- RFC text says Part A+B leaves 2 and 2 in gon-examples and gon-vscode (the `&x` of generic-gna). With `&operand` recognised
  these are 0 by the real rule; the RFC section 5.1 and 11 still print "2 and 2" and "10 and 4" in places.

## Error of mine found during this audit (not Part B)

PR #20's description and section 0 of `docs/release-readiness-part-a.md` (#79) say 15 GW006 in gon-examples. The sum of the seven
figures is 14 (1+2+6+1+1+1+2), as expected.txt and the real checker give. The per-example figures are right; the total is not.

## Limits

Instrument, not a checker; no type-identity rule except where noted; `range`, closures and channel sends not modelled; only
the three repositories; classification read from the output, not every site by eye.
