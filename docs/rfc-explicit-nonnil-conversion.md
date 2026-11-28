# RFC: Explicit Non-Nil Conversion `(!T)(x)`

**Status:** **Part A: ACCEPTED** (formal design decision, 2026-11-21), including the extension of the OQ-1 decision to channel and function. **Part B: accepted as a design decision (2026-11-27), implemented (PR #107), not yet released; see below.** The acceptance is a design decision only: it is not approval to implement or to merge, and nothing is scheduled (§5.1). **Implementation:** Part A is implemented and **released in v1.21.0 (2026-11-22)** (syntax, the checks of a conversion, GW006, the conformance harness `testdata/conformance`); the migration of Gon's own sources and Part B remain open.  
**Amendment (2026-11-22, decision of the owner):** any `&operand` is a recognised non-nil source of Part A (§4 note, §6.6, §10.3): the address of an addressable operand or of a composite literal is never nil, and evaluating it either gives that address or panics, as in Go.
**Amendment A2 (2026-11-22): ACCEPTED by the owner, IMPLEMENTED and RELEASED in v1.22.0 (2026-11-22):** an ordinary `T → !T` flow into a non-interface slot is an error (GN001) instead of the warning GW006; `(!T)(x)` is the only way through (§9, A2). The checker changed in PR #85; this text and the other documents changed after it. **Reading note:** where the text below says that the flow is the warning GW006 (§1, §6.4, §6.6, §10.1, §11 and the measurement sections), it describes v1.21.0 and the measurements made on it; since A2 the same flow is GN001, an error, and GW006 is no longer emitted. The release number is v1.22.0, as the next minor release shipped alone (R1).
**Part B (accepted as a design decision, 2026-11-27; the tool `gon convert` released in v1.25.0, the binding implemented in PR #107 and not yet released):** see `docs/rfc-part-b-binding-and-convert.md` (binding by inference with a migration tool, `gon convert`).
**Target:** Part A: v1.21.0 (2026-11-22); amendment A2: v1.22.0 (2026-11-22); Part B: unassigned (the owner decides the release number, date and shape)  
**Related:** `docs/roadmap-v1.3-plus.md` (I1–I7), `docs/rfc-interface-semantics.md` (D3, D6),
`docs/rfc-generic-semantics.md`, `docs/rfc-element-contract-construction.md` (E7),
`docs/rfc-return-value-contracts.md`, `docs/diagnostic-protocol-v1.md`  
**Date:** 2026-11-21  
**Measured against:** Gon v1.20.0 (`main` c207b8c)

## 1. Summary

I6 says a `T → !T` flow "requires a source that satisfies the contract at that
point". The checker does not enforce that for any non-interface nilable kind (pointer,
slice, map, channel, function): a plain variable, call result or map lookup can
be stored in a `!*T`, `![]T`, `!map[K]V`, `!chan T` or `!func()` slot without any
diagnostic (measured for all five on v1.20.0). Only `!I` is enforced (D3b, with the D3a
exception for concrete operands). Gon has no way for an author to state that a
guard they wrote makes a value non-nil.

This RFC proposes:

1. **Explicit non-nil conversion**, written `(!T)(x)`: an author's static claim
   that `x`, converted to `T`, satisfies `!T`. It is valid exactly when `(T)(x)`
   is valid Go, it performs **no runtime check**, and it is a recognised non-nil
   source.
2. **A warning** (new code, exit 0) for an ordinary `T → !T` flow into a `!T`
   slot of a non-interface kind, silenced by the conversion. *(v1.21.0. Since
   amendment A2 (§9) the flow is an error, GN001, and there is no warning.)*
3. **Idempotence of `!` in type positions**: `!!T` is `!T`, and `gon fmt`
   writes the canonical form.

It changes no Go type rule, adds no runtime representation, and adds no flow
sensitivity.

## 2. Motivation and evidence

`!T` is a static guarantee that reaches only the cases the checker covers
(README). For the non-interface nilable kinds, that is literal `nil`, required struct
fields and a few construction forms. A value that arrives through a variable is
not covered, so the author's knowledge ("I checked `err`", "this map lookup
cannot miss") has no form the checker can see.

### 2.1 Measurements (read-only, Gon v1.20.0)

**Enforcement gap.** In a scratch module, each of the following is accepted with
no diagnostic, for `*int`, `[]int` and `map[string]int` alike: `var b ![]int = a`,
`b = a`, an argument to a `![]int` parameter, `return a` from a `![]int`
function, and a keyed struct field `S{xs: a}`. Only a literal `nil`, an omitted
required field and an unassigned zero value are reported. For interfaces
`var a !io.Reader = r` (ordinary `r`) is GN001 (D3b).

The same was measured for the two other non-interface kinds, in a scratch module on
v1.20.0, with a plain `chan int` and a plain `func()`: `var a !chan int = c`,
`var b !func() = f`, `a = c` and `b = f` on declared `!` variables, an argument to a
`!chan int` and to a `!func()` parameter, `return c` and `return f` from a function
with such a result, and `S{c: c, f: f}` with `!chan int` and `!func()` fields are all
accepted with no diagnostic. In this probe, the tested `return nil` case into a
`!chan int` result was reported as GN001; other nil-literal forms for channel and
function were not tested.

**Why this matters.** `protocolEnvelope.Diagnostics` must be `[]`, never `null`.
Annotating the field `![]protocolDiagnostic`, annotating `writeProtocolJSON`'s
parameter `!`, and removing both nil guards, `gon check ./...` stays `ok` while a
clean run prints `{"schemaVersion":1,"diagnostics":null}`, and
`TestProtocol_Clean_Exit0_EmptyDiagnostics` fails. The contract rejects the
literal `nil` and nothing else, so it cannot replace the runtime guards. The
other direction was also measured: `&Chain{Resolvers: []Resolver{}}` and
`var rs []Resolver; &Chain{Resolvers: rs}` pass a `![]Resolver` field.

**Migration estimate.** A scan of every flow into a `!T` slot (variable
initialiser, assignment, argument, return, field literal, element literal) in
the `main` sources of `gon`, `gon-examples` and `gon-vscode`:

| Scope | Flows into `!T` | Already a recognised source | Needing classification |
|-------|-----------------|-----------------------------|------------------------|
| gon, production `.gon` | 25 (47 with §6.8) | 15 (36) | 10 (11 with §6.8 and C15: the 10 plus `x = c.pkg.files`; 9 of the 10 from guards and external constructors, plus `&cp`, see the `&x` note in §4; **as measured then: after the amendment of 2026-11-22 `&cp` is recognised, so 9 and 10**) |
| gon-examples (5 modules) | 11 | 10 | 0 genuine **in this scan, which does not read `.gna`** (it counted 1 false positive, `.gna`-annotated `store.Open()`, an effect of that limit); see "Re-measurement with `.gna`" below |
| gon-vscode fixtures | 2 | 1 | 0 in this scan; see "Re-measurement with `.gna`" below |

The nine production sites are all pointer destinations and all of the form
"knowledge from a guard or an external constructor that Gon does not track":
`token.NewFileSet()` ×3, `parser.ParseFile` after an `err` check ×3, a map
lookup ×1, `t.Clone()` after a receiver guard ×1, and a parameter after a nil
guard at its caller ×1. None is an `append`, a slice or a `make`. The
`testdata/` fixtures of `gon` are **not** a migration estimate. The first scan flagged
13; the re-measurement below separates the categories and leaves **9** that
would be GW006 (§2.1, "Re-measurement").

**Binding by inference.** v1.20.0 binds a `:=` variable `!` only from a call whose
result is `!T`: `d := mk()` (with `mk() !*T`) gives GW001 on `d == nil`, while
`a := []int{}`, `e := &T{}` and `var b []int = []int{}` do not. To size the rule
of §6.8, every `:=` and every untyped `var x = e` that defines a variable of a
nilable kind was classified by its initialiser, in the `main` sources of the three
repositories (counts are definitions; `.gna` contracts are not modelled):

| Repository | Definitions | Bound today (call `!T`) | Newly bound by §6.8 | Not bound |
|------------|-------------|-------------------------|---------------------|-----------|
| gon, production | 225 | 12 | 76 (`make`/`new` 41, `&T{}` 21, composite literal 7, function literal 6, `append` with an explicit element 1) | 137 (external call 44, other call 64, field 8, index 15, `&x` 2, plain variable 2, conversion 2) |
| gon, fixtures | 7 | 1 | 3 (one is an untyped `var`) | 3 |
| gon-examples | 12 | 2 | 5 | 5 |
| gon-vscode | 4 | 1 | 1 | 2 |

Coverage of the inventory against the recognised-source set of §6.8: the scan
classified initialisers that are composite literals, `&literal`, `make`, `new`,
function literals, `append` and slice expressions by the rules above, plus the
sources that bind today (an identifier bound `!`, a call whose first result is
`!T`, a selector of a `!` field) as far as they are declared in `.gon`. Not
covered: calls and selectors whose `!` comes from a **`.gna`** contract of an
external package (the 44 external calls counted under "not bound" may include
some that bind today, so the "not bound" column is an upper bound; the "newly
bound" column is unaffected, since those forms bind under v1.20.0 already), and
generic instantiations. `(!T)(x)` does not occur in any source yet, so it adds
nothing to the count. The "newly bound" figures are therefore complete for the
construction forms and for `append`/slice expressions in `.gon` sources, and are
not a complete count of every binding §6.8 can produce.

The untyped `var x = e` form occurs once (a fixture). There is no `var x T = <form>`
with a written type anywhere.

Uses of the bound locals: in `gon`'s production sources, 22 later assignments
(`x = append(x, …)` 18: 14 with explicit elements and 4 with a spread, `x = literal` 2, `x = queries[:0]` 1, `x = c.pkg.files` 1)
and **no** `x == nil` and **no** `x = nil`. Of the 22, the first 21 are silent
under §6.6; `x = c.pkg.files` gives GW006. In the other repositories the only
`x == nil` on a bound local (three) are on locals bound today by a `!T` call, so
GW001 there is not new. New GW001 and new GN001 from §6.8: **none** in the three
repositories.

**Existing identifiers in a short declaration (v1.20.0).** With `two() (!*T, error)`:
`var x *T; x, err := two()` leaves `x` ordinary in Go, but the checker rebinds it
`!`, so `x == nil` is a false GW001; `var y !*T = &T{}; y, err := plain()` loses
`y`'s `!` (no GW001 on `y == nil`); and `z, err := two()` in an inner block binds a
new `z` correctly (GW001). Binding must therefore distinguish a declared identifier
from an assigned one (§6.8); fixing it is part of this proposal and changes today's
behaviour in the first two cases. No dependency was identified from the
inventory; the compatibility impact was not measured separately.

**Re-measurement M0–M4 (2026-11-21; `main` of `gon` c207b8c, `gon-examples` 8399eea,
`gon-vscode` c90a39a; Gon v1.20.0 for the control runs).** The measurements were
produced using a temporary measurement instrument that is not included in this
repository, so a reader cannot reproduce or inspect it from here. Its first version
skipped single-call multi-value forms and did not separate new from existing
identifiers explicitly; a second version corrects both (positional result mapping,
`Defs` versus `Uses`) and was run again. Results, for the covered repositories and
commits only:

- The headline figures **stand**: 11 GW006, 76 newly bound locals, 0 new GW001, 0 new
  GN001 in `gon`'s production sources; the 11 sites are the same. (Figures of that
  run, before the amendment of 2026-11-22: `&cp` has since become a recognised source,
  so the same rules now give 10 on the real checker, and 9 under Part A alone.)
- Inventory: `gon` production has **409** definitions of nilable variables by `:=` or
  untyped `var` (225 single-value as before, plus **184** single-call multi-value
  definitions, none of which has a `!T` result position and so none is bound).
- Existing identifiers assigned by a short declaration: **18** in `gon` production, all
  ordinary variables at plain result positions, so the v1.20.0 rebinding defect cannot
  trigger on them; none in the other repositories. The repositories therefore contain no
  existing `!T` identifier in such a declaration; that case rests on the control
  fixtures, not on the inventory.
- Fixtures of `gon` (`testdata/`): 31 flows = **9 GW006** (`valid/basic.gon:13,14,15`,
  `valid/element_contracts.gon:11` ×2 and `:15`, `valid/generics.gon:16`,
  `valid/regression.gon:8,10`) + 6 interface destinations (D3a/D3b/D3d, unchanged) +
  12 `nil` literals (GN001 today) + 1 type-parameter destination (§10.2) + 3 silent.
- Controls: positional multi-return, the three MF-2 conditions (existing ordinary,
  existing `!T`, shadowing), `_` and a written type were exercised on twelve control
  cases and passed; they show the instrument is correct for those rules, not that the
  checker is. Predicted GW001 under the v1.20.0 rules was compared with the real
  `gon check`: it **matched in six modules** (`gon`, `gon` fixtures, `package-check`,
  `source-contracts`, `transpile-dir`, `gon-vscode` `sourcecontracts`) and could **not be
  assessed in four** whose GW001 comes from `.gna` contracts, which that instrument did
  not model (assessed afterwards, see "Re-measurement with `.gna`"). This validates the GW001 prediction under the old rules; it does not
  validate GW006, which no checker implements yet.

**Re-measurement with `.gna` (plan §5; 2026-11-21; `gon` `main` 97bba38, `gon-examples` and `gon-vscode` as
in the M0–M4 run; Gon built from `main` for the control runs).** This block **corrects** the migration
rows above, which were produced by an instrument that did not read `.gna`. A third version of the temporary
instrument reads the `.gna` files through the repository's `gna` registry (function, method and field
contracts, results as recognised sources, explicit generic instantiation). It is not part of any repository; its
source and raw output are kept for audit on the branch `docs/gna-slot-measurement` and are not on `main`, and the
result is UNAUDITED as a measurement of the checker, which does not implement GW006. It does not reopen any
semantic decision.

- **Coverage.** Nine directories were measured: `gon`, the five modules of `gon-examples` that have a `go.mod`
  (`ecosystem-contracts`, `generic-gna`, `package-check`, `source-contracts`, `transpile-dir`) and the three
  subdirectories of `gon-vscode/fixtures`. The fourteen single-file demos of `gon-examples` have no `go.mod`, so the
  loader fails on them and nothing was measured for them; none has a `.gna` file. The five "modules" of the rows
  above are the five that were measured.
- **Control.** Predicted GW001 under the v1.20.0 rules against the real `gon check --json` agrees on the five
  modules that carry a `.gna` contract: `ecosystem-contracts` 3 and 3, `generic-gna` 2 and 2,
  `gon-vscode/fixtures/ecosystem` 1 and 1, `genericgna` 2 and 2, `sourcecontracts` 1 and 1 (a stale `.gna`
  that `.gon` sources shadow). For `gon` itself the `.gna` model changes nothing: 19 flows with GW006 under the
  Part A rules (10 in production, 9 in fixtures) and 20 under Part A and Part B, with and without `.gna`.
- **Result.** Flows into a `.gna` slot (counted as flows, not as lines): Part A alone, **10** GW006 in `gon-examples`
  (6 in `ecosystem-contracts`, on 4 lines, since lines 28 and 31 hold two flows each; 4 in `generic-gna`); Part A and
  Part B, **2**, both `cache.Must(&x)` and `cache.Must[int](&x)` in `generic-gna`, which is `&x`
  (decided on 2026-11-22: any `&operand` is recognised, so these two and the corresponding two of
  Part A no longer warn; see the note below and the "Note on `&x`" of §4).
  `gon-vscode/fixtures/genericgna` is a copy of `generic-gna` and repeats the same flows (4 and 2), so across both
  repositories the totals are 14 and 4 if the copy is counted and 10 and 2 if it is counted once. The 8 other
  Part A flows are locals built from a literal (`db := &store.DB{}`) and passed to a `.gna` field: Part A does not
  bind them and Part B does, which is the difference the split of §5.1 is meant to show. Results of `.gna`
  functions (`store.Open()`, `store.Connect()`) are recognised sources: the false positive of the rows above
  does not exist.
- **Comparison with this RFC.** GW006 applies to a `.gna` slot as §6.6 and Q1 of the plan state. No flow was found
  that the rule warns on for a reason other than the Part A and Part B split or the `&x` limit. There is no unexplained
  difference.
- **Limits.** Only the three repositories at the named commits; `receivers:` claims, embedded structs and aliases
  are not modelled, and the result does not validate GW006.
- **Measured with the real checker, before and after the amendment of 2026-11-22** (a `gon` built from
  `main` at `ae2ca4d` and one with `&operand` recognised (measured first with `&identifier` only, with the same
  result); `gon check --json`, every file of
  `gon-examples` in single-file mode, which includes the fourteen demos that the instrument could not load):
  GW006 in `gon-examples` 35 to 26 (the nine `&x` flows); the examples that differ from their
  `expected.txt` 11 to 7 of 20 (`hello`, `cli`, `return-value` and `construction` are clean again);
  the fixtures of `gon-vscode` that differ 3 to 2 of 11; `gon` production 10 to 9 (`&cp`); `generic-gna`
  4 to 2. Across 61 runs (the nine modules, every example and fixture, and `testdata/`) the only changes
  are 18 GW006 removed (the module and the single-file runs of one example overlap), none added, and
  no diagnostic other than GW006 changed. The instrument was not rerun.

Unchanged limits: `.gna` of external packages that users write, channel sends,
`range` and embedded structs are not modelled; users outside the three repositories
are not measured. The instrument has not been validated universally: its results hold
only for the three repositories and commits named above, and it does not cover every
form of contract or value flow.

**Baseline count.** On `gon`'s `main`, without any migration, the rules of §6.4,
§6.6 and §6.8 (with the slice-expression rule C15) produce **10 GW006**, all in
production sources: the nine flows into `!T` slots measured before the binding rule
(derived from guards and external constructors), and one assignment,
`x = c.pkg.files`, to a bound local. Without C15 the baseline is 11
(`x = queries[:0]` too). *Before the amendment of 2026-11-22 these were 11 and 12:
the tenth Part A flow, `&cp`, warned until any `&operand` became a recognised source.* `gon-examples` and `gon-vscode` produced none that were
genuine in the scan that did not read `.gna`; with `.gna` modelled they produce 10 and
4 flows under Part A (2 and 2 under Part A and Part B; see "Re-measurement with `.gna`"; *after the amendment of 2026-11-22 these are 8 and 2 under Part A and 0 and 0 under Part A and Part B (the Part B figure from the scratch instrument, `docs/measurements/partb-initial`): the two flows `cache.Must(&x)` and `cache.Must[int](&x)` of `generic-gna`, and the same two flows in its copy in `gon-vscode`, no longer warn; see "Re-measurement with `.gna`"*). These are diagnostic counts; the migration counts of §10.7 are
separate.

**Syntax.** `(!T)(x)` is not recognised today: for `(!*int)(a)`, `(![]int)(a)`,
`(!map[string]int)(a)`, `(!io.Reader)(a)`, generic and nested forms alike, the
preprocessor leaves the `!` in the clean source and records no offset. `!T(x)`
fails as a Go type error. `!!*int` strips one `!` and leaves a stray `!` in the
clean source (a Go syntax error, not a Gon diagnostic). `type P = !*A` is not
parsed. `!!` occurs in none of the `.gon` files of the three repositories, and
`(!…)(` occurs in none.

**Generics (v1.20.0).** `!T` on a type parameter is accepted when the
constraint restricts it to nilable kinds (`~*A`, `~*A | ~[]int`) and is GN003
for `any`, `comparable` and `~*A | ~int`. The flow of an ordinary `T` value into
`!T` (`return x`, `var v !T = x`) was measured in the two groups separately:

- Constraints for which the target `!T` is valid (`~*A`, `~*A | ~[]int`): the flow is
  **GN001** "ordinary interface value".
- Constraints for which the target is invalid (`any`, `comparable`, `~*A | ~int`):
  the target is GN003, and the same flow is also reported as GN001 in the same
  run. That GN001 is reported next to the GN003 and is not a result about a valid
  target.

The GN001 comes from D3b, which reads the underlying type of a type parameter,
that is, its constraint interface. The non-generic `*A → !*A` is accepted.
go/types validates a conversion to a type parameter against every member of its
type set (`*A → T` with `~*A | ~[]int` is rejected).

### 2.2 Limits of the evidence

Not measured: channel sends, closures capturing a variable, `map[K]!V` written
by assignment, embedded structs, generic type declarations and generic methods,
instantiation. The `.gna` contracts of external packages are not modelled in the
scan of the inventory (counts are an upper bound; the flows into `.gna` slots were re-measured separately, §2.1); the inventory of §2.1 covers `:=` and untyped `var` definitions of nilable kinds only, in the three repositories. Gon's own `.go` tests are not checked by Gon.
Users outside the three repositories are not measured. The expression-position
nilability check for `(!T)(x)` does not exist yet and is reasoned, not measured.

## 3. Goals

- G1. Give the author an explicit, greppable form to state a non-nil claim.
- G2. Report an ordinary `T → !T` flow of a non-interface kind as a warning.
- G3. Leave Go the authority on type identity and conversion validity (I3).
- G4. Keep `!T` a static contract (I5), with no runtime check and no new type.
- G5. Keep Gon non-flow-sensitive (I1).

## 4. Non-Goals

- Runtime nil checks.
- Flow narrowing after a comparison or a guard.
- Automatic promotion from a map lookup, an index, a function result (other than one declared `!T`), or a slice expression whose base is not a recognised non-nil source; `append` and a slice expression of a recognised non-nil base are promoted only by the rules of §6.6.
- Named non-nil types (`type P = !*A`).
- Additional `.gna` for the standard library (for example `go/token`).
- `!` in a type argument or constraint (stays GN003).
- Generic type declarations and generic methods in the initial scope.
- A recognition rule for anything that is not an `&operand` (the `&x` note in §4).
- A `--strict` mode (§10.5).

### Note on `&x`

`&x` yields a non-nil pointer or panics. **Decided (owner, 2026-11-22): any
`&operand` is a recognised non-nil source of Part A**, like `&literal`, which already
was. The address of an addressable operand (a variable, a field, an element, `*p`) or of
a composite literal is never nil, and evaluating it either gives that address or panics
(`&p.f` with `p` nil, `&a[i]` out of range), exactly as Go does; `&f()` is not valid Go
at all (checked: "cannot take address of f()"). This is a structural rule on the form of
the expression, not an inference from a guard, and the static type of `&x` is `*T`, so
the identity rule of §6.4 applies as for any source: `&n` into a `!P` slot of a named
pointer type still warns. The first version of the decision was limited to
`&identifier` and `&literal`; the owner widened it the same day, because the reason (a
panic is Go's own behaviour) holds for every `&operand` and the rule is then simpler.

*Original reasoning, kept for the record.* The note was written when `&x` was out of
scope: the data covered only `&identifier` of a local (one production site, four
fixtures), and it was to be limited to forms proven safe; `&cp` was counted apart from the nine guard- and constructor-derived
sites and converted explicitly in the migration. The re-measurement with the real
checker (the "Re-measurement with `.gna`" block of §2.1 and the examples of the
downstream repositories) showed that `&x` is a quarter of the warnings in
`gon-examples` and all of them in its first examples, which is what the decision
answers.

## 5. Decision Matrix

| ID | Decision | Status |
|----|----------|--------|
| C1 | Syntax `(!T)(x)`; no postfix `!!` operator | Agreed direction |
| C2 | The conversion is an author's static claim; no runtime check | Agreed direction |
| C3 | Validity is exactly Go's: `(!T₂)(x)` valid iff `(T₂)(x)` valid | Agreed direction |
| C4 | The target must be a nilable kind (else GN003) | Agreed direction |
| C5 | `(!T)(nil)` is GN001 | Agreed direction |
| C6 | `(!T)(x)` is a recognised non-nil source | Agreed direction |
| C7 | Ordinary `T → !T`, non-interface kinds (pointer, slice, map, channel, function): warning | Decided and accepted with Part A (§5.1, §10.1, §10.2); chosen for compatibility, not because the kinds differ in contract semantics. **Reversed by A2 (§9, accepted 2026-11-22, released in v1.22.0): an error (GN001), not a warning** |
| C8 | `(!T)(x)` with `x` already `!T`: identity, no diagnostic | Agreed direction |
| C9 | `!!T` in type positions, including the target of a non-nil conversion, is `!T`; `gon fmt` canonicalises | Agreed direction |
| C10 | `!!x` in expression positions is Go, untouched | Agreed direction |
| C11 | `!T₁ → !T₂`, different types: explicit conversion | Agreed direction |
| C12 | `x := e` and untyped `var x = e` bind `x` `!` when `e` is a recognised non-nil source (§6.8); `x = nil` on such a local is GN001 | Agreed direction; I2 amendment; breaking for unmeasured code (§11) |
| C13 | `append(s, e1, …, en)` with `n ≥ 1` is a non-nil source; `append(s, xs...)` (spread) iff `s` is; `append(s)` follows `s` (§6.6) | Agreed direction |
| C14 | `var x T = e` (written type): `x` stays `T`, never overridden by inference; no hint in this RFC | Agreed direction; the hint (GW007) is deferred (§10.11) |
| C15 | A slice expression (two- or three-index) is a non-nil source iff its base is | Direction; rests on the base being recognised |

### 5.1 Acceptance structure (Part A and Part B)

**Status of acceptance.** Part A was formally accepted on 2026-11-21 as a design
decision. The acceptance includes the extension of the OQ-1 decision to every
non-interface nilable kind (pointer, slice, map, channel and function), so the
normative text that names those kinds stands as written. **Part B is OPEN and is not
accepted.** The acceptance of Part A is not approval to implement it, to merge it or
to release it, and it does not schedule anything: the release shape, `--strict`,
GW007 and the items G1–G6 of the roadmap are unchanged and remain open or on hold; any `&operand` was added to Part A by the amendment of 2026-11-22.

The proposal is accepted in two parts, separately. Accepting Part A does **not**
accept Part B.

**Part A: additive, compatibility-preserving in intent** (not an absolute
guarantee: a new diagnostic and a formatter change can still affect a user's
workflow). C1–C11 and the amendments of D6b and I6: the explicit non-nil
conversion `(!T)(x)`, validity as Go decides it, GW006 for an ordinary flow into a
`!T` slot of a non-interface kind, the contract-identity rule of §6.4, and `!!T`
in type positions (§6.1–§6.5, §6.7, §6.9, and the Part A reading of §6.6).

**Part B: binding inference, potentially breaking.** C12–C15: binding of `:=` and
untyped `var` locals from a structural initialiser (§6.8), the `append` and slice
expression sources (§6.6), `x = nil` on a bound local as GN001, and the I2
amendment. Part B can be postponed, narrowed or revised without cancelling Part A.

**Dependency.** Part A does not need Part B. The recognised sources of Part A are
the sources that exist today (an identifier declared `!T`, or bound today from a
call whose result is `!T`, a selector of a `!` field, a call whose first result is
`!T`), the construction forms of the existing RFCs for a direct flow (a composite
literal, `&literal`, `new`, `make`, a function literal), and the non-nil
conversion. Recognising `(!T)(x)` as a source MUST NOT activate any rule of Part B:
no local is bound by inference, and `append` and slice expressions are not
sources, until Part B is accepted. Part B builds on Part A (it adds sources to the
same GW006 rule).

**Measured effect of the split** (the second version of the temporary instrument of §2.1, run with the Part A rules,
same commits as §2.1): in `gon`'s production sources, Part A alone gave **10 GW006**
(25 flows; the nine guard- or constructor-derived sites and `&cp`); Part A and
Part B gave 11 (47 flows; the one extra is `x = c.pkg.files`, which is a flow only
because Part B binds the local). **After the amendment of 2026-11-22 (`&cp` is a
recognised source): 9 for Part A alone and 10 for Part A and Part B** (the real
checker gives 9 on Part A). Fixtures of `gon`: 9 either way. `gon-examples` and
`gon-vscode`: no genuine GW006 either way **in the scan that did not read `.gna`** (with `.gna` modelled: Part A 10 and 4,
Part A and Part B 2 and 2, see "Re-measurement with `.gna`"; *after the amendment of 2026-11-22 these are 8 and 2 under Part A and 0 and 0 under Part A and Part B (the Part B figure from the scratch instrument, `docs/measurements/partb-initial`): the two flows `cache.Must(&x)` and `cache.Must[int](&x)` of `generic-gna`, and the same two flows in its copy in `gon-vscode`, no longer warn; see "Re-measurement with `.gna`"*). Part A alone binds no new local
(76 are Part B). These counts were measured with the same limits as §2.1.

Items outside both parts and still OPEN: `--strict`, GW007, the other forms of `&`, the release shape,
and G1–G6 of the roadmap.

## 6. Normative Semantics

### 6.1 Form and recognition

A **non-nil conversion** is the Go conversion `(T)(x)` whose type operand,
written inside the parentheses, begins with `!`. The preprocessor MUST recognise
`(` `!` TypeExpr `)` `(` as a non-nil conversion, strip the `!`, and record the
byte offset where `!T` begins in `NonNilOffsets`, as it does for every other
`!T`, **when the first `(` is a grouping parenthesis** (see below).
TypeExpr is any type expression the existing type positions accept,
including qualified names (`!io.Reader`), pointer, slice, map, channel and
function types, parenthesised and nested forms.

**Grouping parenthesis (clarification, added after the acceptance of Part A; it does
not change the design).** The decision is taken on the **token stream of the Go
scanner, including the semicolons it inserts automatically at a line end**, and on
the previous significant token, comments excluded. The first `(` is a grouping
parenthesis exactly when that token is **not** one of the tokens that can end an
operand: an identifier, an integer, floating-point, imaginary, character or string
literal, `)`, `]` or `}`. A `(` that follows one of those tokens does not group: it
opens the argument list of a call, and the shape `(` `!` ... `)` `(` that follows is
then an ordinary call whose result is called again, not a conversion:

```go
v := (!*Node)(x)      // non-nil conversion: the first ( follows :=
w := adder(!flag)(3)  // not a conversion: adder(!flag) is a call, and its
                      // function result is called with 3
```

The second form is valid Go whenever `adder` returns a function, and its token
shape is the same as that of the conversion; it MUST keep its meaning, its clean
source and its offsets exactly as before Part A.

The token `}` needs no further context, because the automatic semicolon decides it. A
`}` that closes a block at the end of a line is followed by an inserted semicolon, so
the `(` that starts the next statement has a semicolon as its previous significant
token and is a grouping parenthesis (for example, a statement `(!*Node)(x).Visit()`
on the line after an `if` block). A `}` that is followed by `(` with no semicolon
between them, that is, on the same line, ends an operand in valid Go (a function
literal or a composite literal), so that `(` is not a grouping parenthesis. This was
checked by scanning the forms above. The look-alike fixtures of the test plan (§12)
include a block `}` followed by a newline and a grouping `(` statement, a function
literal called with such a shape (`func() {}(!a)(b)`), and a composite literal
followed by one.

**Conservative rule.** The preprocessor recognises a conversion only when the first
`(` is established as a grouping parenthesis by this rule. When it is not, or the
source does not scan as valid Go, the source is left unchanged and any problem is
reported by Go or by the checker as before; the preprocessor never guesses.

The preprocessor MUST NOT change the meaning of any program that is valid Go
with an ordinary meaning. With a grouping parenthesis, `(!a)(b)` with `a` a `bool`
is a call of a `bool` and does not type-check; this argument covers only the
grouping case, which is why the precondition above is required, and the test plan
(§12) requires a differential check against the existing sources and the
look-alike fixtures, not an argument alone.

### 6.2 Meaning

`(!T)(x)` has Go type `T`. It asserts, as the author's claim, that the value of
`x` converted to `T` is non-nil. Gon does not verify the non-nil claim itself, either
statically or at run time: it performs no nil check, and the conversion does not
change the value. Gon still checks what is not the claim: the operand is not the
`nil` literal (GN001), the target is a nilable kind (GN003), and Go decides that
the conversion is valid.

```go
var p *A
var q !*A = (!*A)(p) // accepted if (*A)(p) is valid Go; q may still be nil
```

The generated Go is `(T)(x)`. The conversion neither proves non-nil-ness nor
turns a nil value into a non-nil one. `!T` is not a new Go type: it does not
change the underlying type, type identity, assignability or conversion rules.

### 6.3 Validity

`(!T₂)(x)` is valid iff `(T₂)(x)` is valid under go/types (I3). Gon adds no
compatibility rule. Source and target may be any pair of types Go accepts:
named types with the same underlying type, a concrete type to an interface it
implements, an interface to an interface it satisfies, a type parameter whose
type set converts, and a slice to an array pointer. For the last, the Go
specification yields a nil pointer when the slice is nil, so the claim can be
false at run time; this is the author's responsibility, as for any claim.

Gon adds only these requirements:

- The target `T₂` MUST be a nilable kind (pointer, slice, map, channel,
  function, interface), otherwise GN003, with the existing wording.
- For a type parameter, the target is accepted under the existing rule: the
  constraint must restrict it to nilable kinds (otherwise GN003). A conversion
  that go/types rejects (for example `*A → T` with `~*A | ~[]int`) is reported by
  Go, not by Gon.
- A literal `nil` as the operand is GN001 (C5).

### 6.4 Non-nil source and the contract of the target type

A non-nil conversion is a non-nil source in the sense of D3d and §6.6 of the
element-contract RFC. Its static type is the conversion's type `T`.

A recognised non-nil source satisfies a slot `!U` **only when its static type is
identical to `U`** (`types.Identical`, as D3d already requires for interfaces).
A contract is never carried to a different type implicitly (C11), even when Go
accepts the assignment. The outcome for a slot `!U` receiving an expression `e`
of static type `S` is deterministic:

| `e` | `S` vs `U` | `U` kind | Outcome |
|-----|-----------|----------|---------|
| the `nil` literal | any | any | GN001 (existing) |
| recognised non-nil source (including `(!U)(x)`) | identical | any | silent |
| recognised non-nil source | not identical, Go-assignable | not interface | **GW006** |
| recognised non-nil source | not identical, Go-assignable | interface | GN001 (D3d, unchanged) |
| not a recognised source | any | not interface | **GW006** |
| not a recognised source, concrete operand | any | interface | silent (D3a, unchanged) |
| not a recognised source, ordinary interface value | any | interface | GN001 (D3b, unchanged) |
| not a recognised source, type parameter | any | any | per §10.2 |

*Since amendment A2 (§9) every row above marked GW006 is GN001, an error; nothing else in the table changes.*

Consequences, stated so that tests can fix the expected diagnostic:

- `(!U)(x)` is silent only if the conversion `(U)(x)` is itself valid Go: an
  explicit conversion never hides a Go type error.
- `(!U)(x)` always has static type `U`, so it is the form that states the
  contract on `U`: it is silent in a `!U` slot whatever the type of `x`, provided
  Go accepts the conversion.
- The rows marked GW006 for a source that is non-nil but of a different type are
  **new behaviour** for non-interface kinds (today they are silent) and are part
  of this proposal; they are listed in §11.

```go
var rc !io.ReadCloser
var r !io.Reader = rc                  // GN001 (D3d), unchanged
var r2 !io.Reader = (!io.Reader)(rc)   // explicit claim

type Reg map[string]int
var m !map[string]int
var a !Reg = m                         // Go-assignable; contract not carried: GW006
var b !Reg = (!Reg)(m)                 // explicit claim on Reg: silent
```

### 6.5 Identity

`(!T)(x)` with `x` already a non-nil source of type `T` is the identity:
no diagnostic, no warning. This matches the already accepted assignment of a
`!T` to a `!T`.

### 6.6 The warning for an ordinary flow

A value of a **non-interface** nilable kind (pointer, slice, map, channel,
function) that is not a recognised non-nil source and is not the `nil` literal,
flowing into a slot declared `!T`, produces a **warning** and the run keeps its exit
code (warnings alone never produce exit 1). A recognised source of a type not
identical to the slot's type is also a warning for these kinds; the full outcome
table is in §6.4. Interface kinds are not GW006 cases: an ordinary interface value
flowing into `!I` stays GN001 under D3b/D3d, and a concrete operand stays silent under
D3a.

*(Since amendment A2 (§9) the "warning" of this section is an error, GN001, with the message `cannot use value of type S as !U; use (!U)(x) to make an explicit claim`. The rest of the section is unchanged.)*

Slots (proposed): a variable initialiser with a `!T` type, an assignment to a
`!T` variable, parameter, result or field, an argument to a `!T` parameter, a
`return` into a `!T` result, a keyed or unkeyed struct field in a composite
literal, and an element of a composite literal with a `!` element type.

**Recognised non-nil sources, by part.** One definition, with the split of §5.1.

**Part A.** A recognised non-nil source is: an identifier declared `!T` (a variable,
parameter, result or field) or bound `!` by the rules that exist today (a `:=` or
untyped `var` variable that receives a call result declared `!T`, §6.8 "Multi-value
forms"), an immediate call whose first result is `!T`, a selector of a `!` field,
the construction forms of the existing RFCs for a direct flow (a composite literal,
`&literal`, `new`, `make`, a function literal), any `&operand` (amendment of 2026-11-22),
and the non-nil conversion `(!T)(x)` of this RFC. Parentheses never change recognition.

**Part B.** Part B adds to the Part A set: an identifier bound `!` by inference from
a structural initialiser (§6.8), an `append` that satisfies the rule below, and a
slice expression `s[a:b]` or `s[a:b:c]` whose **base `s` is a recognised non-nil
slice** (a non-nil slice sliced stays non-nil, `s[:0]` included; a nil base stays
nil, so the base decides). Slicing an array also yields a non-nil slice, but an
array base is not a source here: a deliberate under-approximation that adds no
recognition rule.

**Before Part B is accepted, the checker MUST NOT classify an `append`, a slice
expression, or a local bound by inference as a recognised non-nil source** (Part B is accepted and implemented since PR #107; the checker now does). The
complete list below is not the Part A list: the `append` and slice-expression rules,
and the binding of locals, are Part B. Recognising `(!T)(x)` as a Part A source does
not bind any local.

The `append` rule (C13, Part B) follows the Go result and needs no analysis of lengths:

- `append(s, e1, …, en)` with **at least one explicit element** (`n ≥ 1`, an
  argument not followed by `...`) is a source **whatever `s` is**: adding an element yields
  a slice of positive length, which is never nil. A new backing array is not
  required: `append` may reuse spare capacity of the existing one, and the rule
  does not depend on that. (Measured: `append(a, 1)` with a nil `a` is non-nil,
  also for a zero-size element type.)
- `append(s, xs...)` is a source **iff `s` is a recognised source**: `xs` may be
  empty, in which case the result is `s` (measured: `append([]int(nil), []int{}...)`
  and `append([]int(nil), nil...)` are nil), and the checker does not look at
  the length of `xs`.
- `append(s)` has the contract of `s` (measured: it is nil when `s` is nil).

The three forms cover every valid call. A spread is the **only** variadic
argument: `append(s, e, xs...)` is not valid Go ("too many arguments in call to
append", measured), so there is no mixed form, exactly like `append(nil, 1)`, which
is invalid because the first argument is always a typed slice. The checker keys on
the presence of the `...` (spread, one argument after `s`) versus explicit
element arguments; a call Go rejects is reported by Go, and Gon does not classify
it. Parentheses never change recognition: a parenthesised expression has the
recognition of the expression inside (`([]int{})`, `(a)`, `(append(a, 1))`,
`((!*T)(p))`). A map lookup, an index, a slice expression with any other base,
a conversion without `!`, an `append` that does not satisfy the rule above, and any
other call are **not** sources.

Proposed code **GW006** (to be confirmed), severity warning, message
`non-nil contract: value of type S does not satisfy !U; use (!U)(x) to make an explicit claim`,
where `S` is the static type of the value and `U` the type of the slot. One text
serves both reasons for the warning (the source is not recognised, or it is
recognised but of a type not identical to `U`, §6.4) without stating a fact about
the run-time value or about which of the two applies. The `nil` literal stays GN001. The diagnostic has a position, a code, a severity
and a message like every other Protocol v1 diagnostic and adds no field.

For interface kinds the existing D3b error is unchanged by this RFC (§10.1).

### 6.7 `!` in type positions

In a **type position**, a run of two or more `!` before a type is the same as
one: `!!T`, `!!!T` and `!T` are the same contract. The preprocessor records one
offset and the clean source contains no `!`. `gon fmt` writes one `!`. The
normalisation is limited to type positions: in an expression position `!!x` is
the Go double negation and is untouched. The preprocessor decides the position by
the same context rule it already uses; the run length never decides it.

The target type of a non-nil conversion is a type position. A run of `!` there is
canonicalised to one `!` under the same rules: `(!!*A)(x)` is read as
`(!*A)(x)` and `(!!!![]int)(x)` as `(![]int)(x)`. `gon fmt` writes the single-`!`
form, and the preprocessor records one offset.

Removing extra `!` characters MUST NOT make a diagnostic position wrong:
`CleanToOrig` and `NonNilOffsets` must still map to the original Gon source, since
Protocol v1 positions depend on them.

### 6.8 Binding by inference (Part B)

A short variable declaration `x := e`, and a `var x = e` with **no written type**,
where `e` is a recognised non-nil source of a nilable kind, bind `x` as if it were
declared `!T`: `x` is then an identifier bound `!`. This is inference from the
structural form of the initialiser only, never from the body, from control flow
or from later statements (I2 is amended, §9). Consequences, all by the existing
rules for a `!`-bound identifier:

- `x == nil` and `x != nil` are GW001.
- `x = nil` is GN001 (an error; new for such locals).
- `x = e'` is a flow into a `!T` slot: silent for a recognised `e'` (including
  `x = append(x, …)`), GW006 otherwise.
- `x` is a recognised source wherever it is used.

**New identifiers only.** Binding applies only to an identifier that the short
declaration (or untyped `var`) **declares**. A short declaration may also assign
to variables that already exist in the same scope (`x, err := f()` with `x`
declared earlier): those keep the contract of their declaration, whether ordinary
`T` or `!T`, and the position they occupy contributes nothing. An identifier
with the same name in an *outer* scope is shadowed by a new one and is bound like
any new identifier. `_` is never bound. v1.20.0 does not do this (§2.1,
"Existing identifiers"); the checker must tell a declared identifier from an
assigned one, which go/types records (`Defs` versus `Uses`).

**Multi-value forms.** Binding is per variable and per position, as the checker
already does for calls (measured on v1.20.0: with `two() (!*T, error)`,
`a, err := two()` and `var e, err4 = two()` bind `a` and `e` `!`; with
`three() (*T, !*T, error)`, `b, c, err2 := three()` binds only `c`, the second
result). In `x, y := e1, e2` (one initialiser per variable) each variable is bound
by its own initialiser. In `x, y := f()` (a single call) the result at each
position decides, and the construction forms and `append` rules do not apply,
since a call is a source only through its declared `!T` result. The blank
identifier is never bound.

The set of initialisers is **closed and structural**: a composite literal,
`&literal`, `make`, `new`, a function literal, `append` by the rule of §6.6, a slice expression with a recognised base, `(!T)(x)`, an identifier
bound `!`, a call whose first result is declared `!T`, and a selector of a `!`
field. An ordinary call is never a source because of how its body looks; a map
lookup or an index is not a source.

A `(!T)(x)` initialiser binds a local only under this section, that is, only when Part B
is accepted; in Part A it is a source for a direct flow and binds nothing.

`var x T = e` with a **written** nilable type does not bind `x`: the author chose
the nilable type, and the checker does not override it. `x` stays an ordinary `T`
whatever `e` is. A hint that suggests writing `!T` there is **not part of this
RFC** (§10.11).

Binding does not make a value non-nil at run time: it records what the
initialiser's form guarantees, and later assignments are checked as above.

### 6.9 Transpile

`gon transpile` and `gon build` write `(T)(x)` for `(!T)(x)`, as for every `!`.
No import, helper or file is added.

## 7. Examples

```go
// Guard the author knows about, now stated:
file, err := parser.ParseFile(fset, name, src, mode)
if err != nil {
	return nil, err
}
c.file = (!*ast.File)(file) // claim; no check

// An ordinary flow warns:
c.file = file // GW006

// Explicit carry to another interface type:
var r !io.Reader = (!io.Reader)(rc)

// Binding by inference:
a := []int{}
a = append(a, 1)       // silent: an explicit element makes the result non-nil
var t ![]int = a       // silent: a is bound !
var b []int = []int{}  // b stays []int; no hint in this RFC

// Idempotence:
var x !!*int = new(int) // same as: var x !*int = new(int)

// Not a type position: untouched.
ok := !!flag
```

## 8. Invariant Check (I1–I7)

- **I1** — upheld: no narrowing; a guard changes nothing for Gon.
- **I2** — amended (§9): explicit commitment, plus inference from the structural
  form of a `:=` / untyped `var` initialiser; never from the body.
- **I3** — upheld: Go decides conversion validity and type identity.
- **I4** — unaffected.
- **I5** — upheld: no runtime representation, helper or check.
- **I6** — amended (§9): the sentence "`T → !T` requires a source that satisfies
  the contract" gets an explicit meaning (warning, and the conversion as the
  explicit source).
- **I7** — unaffected: no `.gna` schema change.

## 9. Amendments

**I6** (Part A; `docs/roadmap-v1.3-plus.md`), proposed addition: *A `T → !T` flow of a
non-interface nilable kind is reported as a warning in two cases: when the source is
not a recognised non-nil source, and when the source is recognised but its static
type is not identical to the type of the `!U` slot even though Go accepts the
assignment (§6.4). The explicit non-nil conversion `(!U)(x)` is a recognised non-nil
source of type `U`; it is a claim by the author and is not checked.*

**I2** (Part B; `docs/roadmap-v1.3-plus.md`), proposed addition: *Inference is allowed only
from the structural form of an initialiser in `:=` or an untyped `var`: a
recognised non-nil source binds the variable `!`. Guarantees are never inferred
from a function body, from control flow or from later statements.*

**D6b** (Part A; `docs/rfc-interface-semantics.md`), proposed wording: *`!I(x)` (without
parentheses) remains not a conversion form. The explicit conversion `(!I)(x)`,
valid when `(I)(x)` is valid Go, is the claim form for an interface; like every
non-nil conversion it performs no runtime check, and an ordinary conversion to
`I` still produces an ordinary `I`.*

### A2: an ordinary flow into a `!T` slot is an error (accepted, 2026-11-22; implemented and released in v1.22.0)

**Status: ACCEPTED as a design decision (2026-11-22, the owner) and IMPLEMENTED (checker: PR #85; documents: the change after it) and RELEASED in v1.22.0.** It records the direction of the owner
(2026-11-22: "nilable to non-nil must be explicit; that is v1"). It amends C7 and OQ-1 of this RFC, which were accepted with Part A
and shipped in v1.21.0. It is released as v1.22.0, the next minor release, shipped alone (R1).

**Proposal.** For every slot of §6.6 whose type is a `!T` of a non-interface nilable kind (pointer, slice, map,
channel, function), an expression that is not a recognised non-nil source of the identical type `T` is an
**error** (exit 1) instead of the warning GW006. The only silent forms are the ones that are silent today:
a recognised non-nil source of identical type, and `(!T)(x)`. Nothing else in §6.4 changes: the rows that
are GN001 (the `nil` literal, interface slots under D3a, D3b and D3d) and the type-parameter row (§10.2)
stay as they are, and a recognised source of a different but Go-assignable type is now an error as well
(the contract is still never carried to another type, C11). `(*T)(nil)` and `x = p` with `p *T` are
therefore errors; the first is not "proved nil" by the checker, it is an ordinary value like the second.
With this the five nilable kinds have one meaning, the one `!I` already has. `--strict`, as OQ-1 describes it
(a way to make the severities uniform), has no purpose left in the default mode.

**Why.** A contract that a warning can leave violated (`var x !*int = new(int); x = (*int)(nil)` leaves `x` nil at
run time with exit 0, measured on v1.21.0) is not a contract unless every user also fails their CI on warnings.
The RFC chose the warning for compatibility alone (§10.1); the owner now prefers the stronger rule.

**Decisions of the owner (2026-11-22).**

1. *Diagnostic code: GN001.* An ordinary value into a `!T` slot of a non-interface kind is GN001, as an ordinary interface value
   into `!I` already is (D3b): one code, no addition to Protocol v1, and GW006 is no longer emitted once A2 is implemented. The
   code does not tell "provably nil" (the `nil` literal) from "not proved non-nil" (an ordinary value); the message does. The message
   keeps the text of GW006 with "cannot use" in place of "does not satisfy": `cannot use value of type S as !U; use (!U)(x) to
   make an explicit claim`. Considered and not chosen: a new code `GN004`, which would keep the two cases apart by code, and GW006 as an
   error, which would break the reading of the `GW` prefix as a warning.
2. *Release shape: alone, as the next minor release.* This is R1 (a potentially breaking semantic change) and ships by itself, as
   v1.21.0 did, with a compatibility paragraph in the CHANGELOG, after the measurement and the migration of the examples. The number and
   the date are decided when the release is made.
3. *Order with Part B: A2 first, Part B is reassessed after.* C12 and §6.8 bind inferred locals, so `x = e'` on a bound local would be an
   error too, and the two idioms found in the follow-up audit (`docs/measurements/partb-followup/`: `p = p.next` in a traversal loop,
   release by `x = nil`) become errors on correct Go. Part B is not accepted, and is discussed again only after A2 is implemented, with A2 as its basis.

**Measured effect (v1.21.0 as the proxy: every GW006 of today is one error under A2; the real checker, not an
instrument).**

| Where | GW006 today | What changes |
|---|---|---|
| `gon` production (after the nine migrated sites) | 0 | nothing; `gon check ./...` stays `ok` |
| `gon-examples` (20 examples, `check-examples.sh`) | 14 in 7 examples (flow-ok 1, field-contracts 2, ecosystem-contracts 6, type-coverage 1, element-contracts 1, generics 1, generic-gna 2) | only `flow-ok` changes exit code (0 to 1); the other six already exit 1; `expected.txt` records GN001 in place of GW006 |
| `gon-vscode` fixtures | 2 (`genericgna`) | exit code unchanged (already 1) |
| `.gna` slots among those | 8 in `gon-examples` and 2 in `gon-vscode` (§2.1) | each needs `(!T)(x)` or a `!T`-declared source in the caller |
| the conformance harness (`testdata/conformance`) | 18 entries of `manifest.json` expect GW006 | each expectation changes code and severity; the `// W` markers stay |
| users outside the three repositories | not measured | the amount of code that flows a plain pointer, slice, map, channel or function into a `!T` slot is unknown; GW006 has been public since v1.21.0 |

**Migration of a user's code.** Where the value is known to be non-nil: `(!T)(x)`; better, declare the function that returns
it `!T` (its `return` is then checked and its callers are silent), or build it with a form that is recognised already
(`&operand`, a literal, `new`, `make`). No automatic fix is proposed.

**Consequences for the other documents (written only when the amendment is accepted and implemented):** the C7 row, §1, §6.4
(the GW006 rows), §6.6, OQ-1, §11 of this RFC; the GW006 note of `docs/diagnostic-protocol-v1.md` and its exit-code table;
`docs/rfc-interface-semantics.md`; `docs/roadmap-v1.3-plus.md`; the README and the CHANGELOG; the examples
(`flow-ok` documents "a non-guarantee" and would change its purpose) and the editor fixtures.

**Limits of the evidence.** Three repositories; the figures come from the real `gon check` of v1.21.0 and the repositories' own
runners; no figure for users outside them; the effect on a Part B world was not measured with a checker.

**Alternatives.** Keep the warning and gate it in CI (today, not enforced for anyone who does not); `--strict` as an opt-in
(deferred since v1.5, leaves the default weak); the error in the default mode (this amendment).

## 10. Open Questions

1. **Severity across kinds. Decided: ordinary `T → !T` is GW006 for every
   non-interface nilable kind (pointer, slice, map, channel and function), and an
   ordinary interface value into `!I` stays GN001.** The decision was first taken for
   pointer, slice and map. Its scope was then **extended to channel and function**,
   because `!` has had a uniform meaning on every nilable kind since v1.6 and v1.20.0
   accepts a plain channel or function value into a `!` slot without a diagnostic
   (§2.1). The extension was accepted together with Part A on 2026-11-21 (§5.1), so
   the normative text that names the five kinds stands as written. The difference is a
   **compatibility decision**. It is not a claim that the two groups have different
   contract semantics and therefore deserve different severities. `!I` is already
   enforced by D3b/D3d, and D3a and D3d govern when an interface contract can be
   carried; weakening it would be breaking in the other direction. For the
   non-interface kinds the rule has never been enforced, and an error would break code that
   is accepted today. The warning does not prove that a value satisfies the contract.
   `--strict` (deferred since v1.5, not designed here) is mentioned only as a possible
   future way to make the severities uniform; this RFC neither promises nor relies on
   it. *Amendment A2 (§9, accepted 2026-11-22, released in v1.22.0) makes the severities uniform in the
   default mode instead: the owner's direction is that an ordinary value of a nilable kind
   must be converted explicitly to enter a `!T` slot.*
2. **Type parameters (decided: D3b is unchanged).** For a type-parameter target
   accepted under the existing GN003 nilability rule, an ordinary value of that type
   parameter flowing to `!T` remains GN001 under D3b. This does not change the
   existing target-constraint rule (GN003), and the decision does not cover mixed
   nilable-kind constraints. A
   non-nil conversion `(!T)(x)` is a recognised source like any other (§6.4): it
   satisfies a `!T` slot through the same-type rule D3b/D3d already apply to `!`
   sources, and it is not a way around D3b for ordinary type-parameter values. The
   target `T` stays restricted by the existing GN003 rule. The GN001 text "ordinary
   interface value" is misleading for a type parameter; correcting it is a separate
   change with no change of semantics. For mixed nilable-kind
   constraints such as `~*A | ~[]int`, this RFC makes **no new decision** under OQ-2.
   The GN001 observed for `~*A | ~[]int` in §2.1 records the v1.20.0 checker
   behaviour only; it is not a separate normative decision of this RFC, and this RFC
   does not propose changing that existing behaviour. Normative example:

   ```go
   func f[T ~*A](x T) {
       var a !T = x        // GN001: ordinary T → !T, D3b unchanged
       var b !T = (!T)(x)  // silent: explicit claim, if (T)(x) is valid Go
   }
   ```
3. **`&x`.** **Decided (2026-11-22):** any `&operand` is a recognised source of Part A (the `&x` note in §4).
4. **D6b / I6 wording** in §9.
5. **Diagnostic and Protocol v1.** The final code, the set of slots and kinds in
   §6.6, and the consumers' view of a new warning. `--strict` (deferred since
   v1.5) does not exist and has no defined behaviour; this RFC does not assume
   it and mentions it only as a possible future way to make severities uniform. Escalating the warning to an error through it needs its own decision on
   Protocol v1, exit codes and CLI and LSP consumers.
6. **Preprocessor and formatter.** The context rule for `(!T)(x)` including
   nested and complex types; stripping a run of `!` only in type positions;
   `CleanToOrig` and `NonNilOffsets` staying correct after the extra `!` are
   removed (a position bug, not a cosmetic one); whether the formatter or the
   preprocessor owns the canonical form. Today `gon fmt` has not been measured on
   a run of `!`, since the preprocessor currently fails on it.
7. **Migration.** *Migration count*: the sites to change so that `gon check ./...`
   on Gon has no GW006 (and the generated-source gate passes). **Part A alone: nine**
   explicit conversions, all derived from guards and external constructors. **Part A+B:
   ten**, the nine and the assignment `x = c.pkg.files` to a local bound by §6.8
   (eleven if C15 is not adopted: `x = queries[:0]` too). *Diagnostic count*: 9 GW006
   for Part A alone and 10 for Part A+B. Here the two counts coincide, one conversion
   per diagnostic; one site (`validateGnaFile`'s parameter) moves the conversion to its
   caller instead of removing it. *History:* before the amendment of 2026-11-22 these
   were ten, eleven (twelve) and 10 and 11, because `&cp` was not a recognised source
   and was converted like the others; the figures were measured on `main` with the
   instrument (§2.1, §5.1) and the 9 for Part A was confirmed with the real checker.
   The dogfood gate and `gon-vscode` must be checked against the added warnings; the
   `testdata/` fixtures were recounted with D3a, `nil` literals and type parameters
   separated (§2.1, "Re-measurement"): 9 GW006 by the instrument (14 with the real
   checker, which also counts map values, fixed-array elements and an instantiated
   generic field; none is a false positive); no migration estimate rests on them.
8. **Evidence limits** in §2.2, in particular the expression-position nilability
   check for `(!T)(x)`.
9. **Binding by inference (Part B).** Direction taken: the closed set of §6.8, an untyped
   `var x = e` treated like `:=`, and `x = nil` on a bound local is GN001. That is a
   **breaking error for code outside the three repositories** (none measured
   inside them): inference changes the storage contract of the identifier. The
   wording of the I2 amendment (§9) remains to be confirmed.
10. **Slice expressions (Part B).** Direction taken (C15): `s[a:b]` and `s[a:b:c]` are
    sources iff the base `s` is a recognised non-nil source; a nil or ordinary
    base is not promoted; no narrowing by guards. Strings are not nilable, so the
    rule concerns slices only.
11. **GW007 (hint for `var x T = e`).** Out of the initial scope. GW006 closes the
    enforcement gap; the hint is an ergonomic addition to be evaluated once users
    have met the inference rule, possibly as a hint severity if Protocol v1 and
    the tooling support one. If it is kept, its code, wording, severity and
    editor effect need a formal decision.
12. **Release shape.** Whether and how each part ships (alone, number, date) is the
   owner's decision.

## 11. Breaking-Change Classification

- **Exit codes:** unchanged (warnings alone never produce exit 1).
- **Protocol v1:** no new field and no new severity; a new code and new warning
  entries in `diagnostics`. The schema version is unchanged (I7). Consumers that
  treat any diagnostic as a failure, or compare the count of diagnostics, see a
  difference.
- **`gon check` output (GW006 in v1.21.0; since A2, GN001 errors).** Diagnostics on existing sources that flow a
  value that is not a recognised non-nil source into a `!T` slot of a non-interface
  kind, including a recognised source of a different, Go-assignable type (§6.4;
  today silent). Measured on `main` of the three repositories: 11 in `gon`'s
  production sources; in `gon-examples` and `gon-vscode`, 10 and 4 flows under
  Part A (2 and 2 under Part A and Part B), all into `.gna` slots (see "Re-measurement
  with `.gna`" in §2.1); *after the amendment of 2026-11-22 these are 8 and 2 under Part A and 0 and 0 under Part A and Part B (the Part B figure from the scratch instrument, `docs/measurements/partb-initial`): the two flows `cache.Must(&x)` and `cache.Must[int](&x)` of `generic-gna`, and the same two flows in its copy in `gon-vscode`, no longer warn; see "Re-measurement with `.gna`"*. The different-type case was not measured separately. Users
  outside the three repositories are not measured.
- **Binding by inference (§6.8, Part B) changes the contract of identifiers**, so its
  effects are different in kind from GW006 and are listed separately:
  - *New bindings:* 76 locals in `gon`'s production sources, 3 in its fixtures,
    5 in `gon-examples`, 1 in `gon-vscode` (§2.1).
  - *GW001* on `x == nil` for such a local: none in the three repositories.
  - *GN001* on `x = nil` for such a local: **an error that was accepted before**;
    none in the three repositories.
  - *GW006* on an assignment to such a local from an unrecognised source: 1
    (`x = c.pkg.files`).
  - *Existing identifiers in `:=`:* v1.20.0 rebinds an existing variable from a
    call result position (a false GW001 on an ordinary `x`, a lost `!` on a `!T`
    `y`); §6.8 corrects both, which can add or remove a diagnostic on such code.
  - *Propagation:* such a local is a recognised source wherever used, so flows that
    were silent stay silent and flows from it into `!T` slots are silent.
  - *Interaction with `append` and the conversion:* `x = append(x, …)` is silent;
    `x = (!T)(y)` is silent.
- **Generated Go:** unchanged for existing sources; new sources using `(!T)(x)`
  produce `(T)(x)`.
- **Possibly breaking (code that was accepted):** a source that already contains
  `!!T`, `(!…)(` or a stray `!` in those positions; none exists in the three
  repositories.

## 12. Test Plan (outline)

- **Preprocessor:** `(!T)(x)` for simple, pointer, slice, map, channel, function,
  qualified, parenthesised, nested and generic type operands; `!!T`, `!!!T` in
  every type position (variable, field, parameter, result, element) and in the
  target of a conversion (`(!!*A)(x)`, `(!!!![]int)(x)`); and `!!x` in
  expression positions (including inside parentheses, arguments and function
  literals) untouched; offsets and `CleanToOrig` after stripping.
- **Differential check, with three criteria.** (1) An input that does **not** use
  the new syntax, including every existing `.gon` and `.go` source of the three
  repositories, must produce a clean source and offsets **identical** to the
  baseline (the version before the change). (2) An input that uses the new syntax
  (`(!T)(x)`, a run of `!` in a type position) has no equivalent baseline, since the
  old version does not recognise it; it must produce the **expected** clean source
  and offsets, fixed in the fixture. (3) A construct that only resembles the new
  syntax but keeps its ordinary Go meaning (the contexts listed below) must keep its
  old clean source, offsets and behaviour, as in (1). The existing sources are a
  regression baseline, not proof of the §6.1 requirement; a **synthetic suite of
  Go-valid inputs** targets the grammar boundary: parenthesised expressions,
  unary `!` and `!!` before parenthesised operands, a call on a parenthesised
  expression, nested parentheses, conversion versus call, the pattern `(!T)(x)`
  inside comments, strings and raw strings, function literals and argument lists,
  and qualified, nested and complex type expressions; each is classified under
  (1), (2) or (3) in its fixture.
- **Checker:** GW006 for each slot and each non-interface nilable kind (interface
  kinds are tested separately under D3a/D3b/D3d); silence for recognised
  sources, for the conversion and for the identity case; GN001 for `(!T)(nil)`;
  GN003 for a non-nilable or `any`-constrained target; D3d with and without the
  conversion; type parameters per §10.2; the cross-package and `.gna` paths.
- **Binding by inference (§6.8):** one fixture per initialiser form (composite
  literal, `&literal`, `make`, `new`, function literal, `append` with explicit
  elements, `append` with a spread of a recognised and of an unrecognised first
  argument, `append(s)`, a slice expression of a recognised and of an
  unrecognised base, two- and three-index, an identifier bound `!`, a selector of
  a `!` field, a call with a `!T` result, `(!T)(x)`); the same with `var x = e`;
  `x == nil` (GW001), `x = nil` (GN001), `x = <unrecognised>` (GW006),
  `x = append(x, v)` (silent); multi-value `x, err := f()` per result position,
  `x, y := e1, e2` per initialiser, the blank identifier; a written type
  (`var x T = e`) that stays `T`; and locals captured by closures.
- **Contract identity (§6.4), as separate fixtures:** with `type A struct{}` and
  `type B A`: a recognised source of the same type is silent; a recognised source of
  a different but Go-assignable type (`var a !A`, `var b !B = a`) is GW006; the same
  with `(!B)(a)` is silent; a source whose type is not Go-assignable is a Go error
  and gives no GW006 in its place; interface kinds keep D3a/D3b/D3d.
- **Short declarations and existing identifiers (§6.8):** `x, err := f()` with an
  existing ordinary `x` (x keeps its contract), with an existing `!T` `x` (keeps
  `!T`), with a new `x` at a `!T` result position (bound), with a same-name `x` in an
  outer scope (new, bound), and `_`.
- **Closures:** a closure creates no binding and changes no contract: with
  `x := &T{}` and `func() { use(x) }`, `x` stays a recognised source inside the
  closure; a local declared inside the closure is bound by the same rules.
- **Parentheses:** `([]int{})`, `(a)` with `a` bound, `(append(a, 1))`, `((!*T)(p))`
  and nested parentheses keep recognition.
- **`append`:** each of the three forms; `append(s, e, xs...)` and `append(nil, 1)`
  are reported by Go, not classified by Gon.
- **Go validity:** an invalid conversion is reported by Go, not by Gon.
- **Protocol:** a conformance fixture for a warning-only run (exit 0) and a mixed
  run; envelope unchanged.
- **Formatter and editor:** `gon fmt` idempotence and canonicalisation; LSP
  diagnostic positions; the VS Code grammar.
- **Mutation tests** on the generated `.go` (the mutant must compile) and the
  cross-OS `test-os` job.
- **Dogfood, with separate criteria for Part A and for Part A+B** (§5.1). The gate
  checks the diagnostics of `gon check ./...`, not the exit code alone, because
  warnings do not change it, and the generated-source gate must pass.
  - *Part A:* after the **nine** Part A sites (all derived from guards and external
    constructors; `&cp` is recognised since 2026-11-22) are converted, `gon check ./...` on Gon with the
    Part A rules reports no GW006. `x = c.pkg.files` is not a flow under Part A, so it
    is not part of this criterion. This criterion is the only dogfood condition for
    accepting Part A.
  - *Part A+B:* after all **ten** flagged sites (the nine and `x = c.pkg.files`) are
    converted, `gon check ./...` with the combined rules and C15 reports no GW006
    (eleven sites if C15 is not adopted). This criterion belongs to Part B and must
    not be a precondition for accepting Part A.

## 13. Alternatives Considered

| Alternative | Reason not chosen |
|-------------|-------------------|
| Postfix `x!!` | Reads as a runtime assertion (as in other languages) while this RFC adds no check; carries no target type, so a mistaken claim is not caught by Go; adds an operator Go does not have |
| `!T(x)` | Fails as a Go type error today and conflicts with D6b |
| `nonnil(x)` builtin | Needs special recognition and can be shadowed by a local name |
| A conversion with a runtime check | Needs an I5 amendment, a typed or reflective helper, and a place for it (a generated file per package, or a dependency of every consumer); transpile is textual today; not chosen |
| Making `T → !T` an error | Breaking for existing sources; no way to acknowledge a claim yet |
| Narrowing after guards | Violates I1 |
