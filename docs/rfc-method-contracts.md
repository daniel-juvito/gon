# RFC: Call-Site Contracts for Source-Declared Methods (Gon v1.11 / M7)

**Status:** Draft — awaiting owner review. Decisions S1–S10 are *proposals*;
§10 lists the questions that need an explicit owner answer. The baseline and
feasibility figures in §12 were measured on 2026-11-14, before any decision.
**Target:** Gon v1.11 (sequenced after M8, package-aware checking, v1.10 — `docs/rfc-package-check.md`, accepted 2026-11-15)
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`,
`docs/roadmap-v1.3-plus.md`, `docs/rfc-return-value-contracts.md` (v1.1),
`docs/rfc-field-contracts.md` (v1.2), `docs/rfc-interface-semantics.md`
(M1a / v1.4, D5 / I3), `docs/rfc-ecosystem-contract-expansion.md` (M4/M5a /
v1.5), `docs/rfc-generic-semantics.md` §13.1 (M3 / v1.8),
`docs/rfc-generic-gna.md` (M5b / v1.9)
**Date:** 2026-11-14

## 1. Summary

A `!` written on a **plain function's** parameter or result is enforced at its
callers (v1.0 / v1.1): `Put(nil)` is GN001, and a `!T` result is a non-nil
source (`GW001` on `x == nil`). A `!` written on a **method's** parameter or
result is not. Measured against v1.9.1, a method declared in the checked file
as

```go
func (b *Box) Set(p !*T)   {}
func (b *Box) Get() !*T    { return &T{} }
```

is checked *inside its body* (`return nil` is GN001; `p` is a non-nil local)
but **none** of the seven ordinary ways to call it is checked: `b.Set(nil)`,
a value-receiver call, a call promoted through embedding, a call through an
interface, a method expression, and the result of `b.Get()` all produce no
diagnostic (§12.1). This was recorded as an open gap in
`rfc-generic-semantics.md` §13.1.

The machinery is already there for the *external* case: `.gna` `methods:`
entries are resolved through `go/types` selections
(`resolveMethodParams` / `resolveMethodResults`). What is missing is the
other source of the contract — the declaration in the checked file.

**Normative principle:**

> A method call is checked against the contract written on **the declaration
> the call resolves to**, found through `go/types` object identity — never by
> name. A contract is not carried through a value that merely holds the method
> (a method value), consistent with I1 and I6.

## 2. Motivation

1. **A silent asymmetry.** `Put(p !*T)` and `(b *Box) Put(p !*T)` look the same
   to an author, and only one is enforced at its callers. A reader cannot tell
   from the source that the second `!` is documentation only.
2. **Methods are most of the API surface.** In a corpus of 540 standard-library
   files, methods are 2,733 of 4,600 function declarations (59%), and 31.6% of
   the `x.M(…)` calls in a file resolve to a method declared in that same file
   (§12.3) — the part a single-file checker can see.
3. **A name-keyed shortcut already cost a bug.** The tables behind call
   resolution were keyed by bare declaration name for functions and methods
   alike; a method overwrote a same-named function's contract and produced
   false GN001 / GW001 (fixed in v1.9.2).
   In the same corpus, 10 of 540 files declare a method with the name of a
   plain function and 55 declare one method name on two or more receiver types
   (§12.2) — so the collision is common structure, not an accident. This RFC
   removes the need for any name-keyed method table by resolving by object.

## 3. Terminology

- **Origin method** — the `*types.Func` of the method as declared, before any
  instantiation (`Func.Origin()`), as in `rfc-generic-gna.md` §3.
- **Call form** — how a method is reached: direct call (`x.M(a)`), method
  expression (`T.M(x, a)` / `(*T).M(x, a)`), promotion through an embedded
  field, call through an interface, call through a generic receiver, or a
  *method value* (`f := x.M; f(a)`).
- **Same-file declaration** — a method whose `*ast.FuncDecl` (or interface
  method `*ast.Field`) is in the file being checked. Gon checks one file at a
  time; this is the same limit plain functions already have.

## 4. Decision Matrix

### S1 — Resolve by object identity, not by name

For a call `x.M(…)`, `go/types` gives the `Selection`, whose `Obj()` is the
`*types.Func`. Take its origin method; the contract is the `!` flags written
on that method's declaration in the checked file. Methods **never** enter a
name-keyed table. (The `.gna` path is unchanged and still applies to methods
of external types.)

### S2 — Direct calls: parameters and results

For a direct call, the same rules as a plain function apply, positionally:

- a literal `nil` passed to a `!` parameter → **GN001**, as for functions; an
  ordinary interface value passed to a `!I` parameter → **GN001** (D3b of
  `rfc-interface-semantics.md`);
- a `!` result is a non-nil source at the immediate use site: `g := b.Get();
  g == nil` → **GW001**.

No new diagnostic codes.

### S3 — Method expressions

`T.M(x, a)` and `(*T).M(x, a)` resolve like a direct call with the receiver as
an extra first argument; contract positions are offset by one, exactly as the
`.gna` path already does for `types.MethodExpr`.

### S4 — Promotion through embedding

A call promoted through an embedded field resolves (via the `Selection`) to the
method that actually declares it. The contract of that declaration applies,
wherever the call is spelled. If that declaration is in another file it is not
visible (S9) and the call is unchecked.

### S5 — Calls through an interface

A `!` written on an **interface method's** signature applies to calls made
**through that interface type**: `var s Setter = b; s.Set(nil)` is GN001 when
`Set(p !*T)` is declared on `Setter`. This is a *call-site* rule only. Per
`rfc-interface-semantics.md` D5 / I3, `!` does not alter method sets or the
implementation relation: an implementing method need not repeat the `!`, and
Gon does not require it to. A call through the **concrete** type uses the
concrete method's own declaration.

### S6 — Method values are not tracked

`f := x.M; f(nil)` is **not** checked. `f` is an ordinary function value; the
contract is not carried through an assignment (I1, I6). The same applies to a
method passed as a callback. This is a documented gap, not a diagnostic.

### S7 — Generic receivers

A call on an instantiated generic type (`Box[int].Put`) resolves through
`Func.Origin()` to the generic declaration, so the contract positions are those
of the declaration (`rfc-generic-semantics.md` G3 / G5). Methods that declare
their own type parameters (Go 1.27) resolve the same way.

### S8 — Receiver contracts are out of scope

`func (b !*Box) F()` — a contract on the receiver itself — is not part of this
RFC. `.gna` `receivers:` stays reserved. Calling a method on a possibly-nil
receiver is a separate question with its own interaction with I1 and the
zero-value rules, and deserves its own RFC if wanted.

### S9 — Scope: the files being checked together

Only contracts declared **in the files checked together** are applied: the one
file in file mode, and **every `.gon` file of the package** in package mode
(`rfc-package-check.md`, M8, which ships first as v1.10). A method declared in
a file that is not part of the check, or in another package, is invisible to
the source path (an *external* package's contract still comes from `.gna`).
This is the same limit plain functions have, and it moves with M8: M7 adds no
cross-file mechanism of its own.

### S10 — Degradation and compatibility

- Without type information (the `go/types` run failed) nothing is resolved and
  no method contract is applied — never a name-based guess.
- **Classification (R1):** possibly breaking in the same sense as the v1.8 and
  v1.9 releases. It reports GN001 / GW001 on code that already violates a
  documented, written contract but was previously not checked. No new syntax,
  no `.gna` schema change, no new diagnostic code. Ships **solo** as v1.11, after M8.

### S11 — Non-Goals

1. Contracts carried through method values or function values — S6.
2. Variance rules between an interface method and its implementations — D5.
3. Receiver (`!` on the receiver) contracts — S8.
4. Cross-file or cross-package source contracts — S9.
5. Inferring a contract from a method body — I2.
6. Flow-sensitive nilability — I1.
7. Any runtime check or code-generation change — I5.

## 5. Normative Rules

1. **By identity.** A call is matched to a declaration through `go/types`
   (`Selection.Obj()`, then `Origin()`); a method name is never a lookup key.
2. **Same rules as functions.** Once matched, parameters and results follow the
   plain-function rules positionally (S2, S3).
3. **Interface calls use the interface's declaration; concrete calls use the
   concrete declaration** (S5).
4. **No carrying.** A method value, a callback, or any assignment of a method
   to a variable does not carry its contract (S6).
5. **Checked files only.** Only declarations in the files being checked
   together are consulted: one file, or the package in package mode (S9).
6. **Absent type information means absent contract** (S10).

## 6. Examples

```go
type T struct{}
type Box struct{}
type Wrapper struct{ Box }
type Setter interface{ Set(p !*T) }

func (b *Box) Set(p !*T)  {}
func (b *Box) Get() !*T   { return &T{} }

func f(b *Box, w Wrapper, s Setter) {
	b.Set(nil)           // GN001  — S2
	w.Set(nil)           // GN001  — S4: promoted, resolves to Box.Set
	s.Set(nil)           // GN001  — S5: the interface declares the contract
	(*Box).Set(b, nil)   // GN001  — S3: method expression
	g := b.Get()
	if g == nil {}       // GW001  — S2: a !-result is a non-nil source

	set := b.Set
	set(nil)             // no diagnostic — S6: a method value carries nothing
}
```

## 7. Compatibility

See S10. A project that already writes `!` on method parameters and results is
the only one affected, and only where a call or comparison contradicts what the
author wrote. The fix is the same as for a plain function: pass a non-nil
value, or remove the `!`.

## 8. Implementation Sketch

- **Index the declarations.** After type-checking, build
  `localMethodDecls map[*types.Func]*ast.FuncDecl` from `info.Defs` (each
  `*ast.FuncDecl` with a receiver) and `ifaceMethodDecls map[*types.Func]*ast.Field`
  (interface method signatures). Keyed by object, so two types sharing a method
  name, or a method sharing a plain function's name, cannot collide.
- **Resolve a call.** Extend `resolveCallParams` / `resolveCallResults`
  (`internal/checker/annotations.go`): for a `*ast.SelectorExpr`, look up
  `info.Selections[sel]`, take `Func.Origin()`, and read the flags from the
  declaration's `!` offsets (`isNonNil` on each parameter / result type).
  Consult the source declaration **before** the `.gna` path; if the object is
  from another package, fall through to `.gna` as today.
- **Method expressions.** Reuse the `argOffset` already computed for
  `types.MethodExpr` in `checkCallExpr`.
- **Results.** `defineFromCallResults` already calls `resolveCallResults`; no
  change beyond the resolver.
- **No change** to `collectFuncDecl` (methods stay out of the name-keyed tables
  after v1.9.2).
- **Docs / version:** `v1-scope.md`, `gna-spec-v1.md` (receivers still
  reserved), README diagnostics table, CHANGELOG, roadmap, `gon version` →
  `1.10.0`; examples and the editor fixture follow.

## 9. Test Requirements

Executable spec in `internal/checker/method_contracts_test.go`.

1. S2: nil to a `!` parameter (pointer, slice, map, interface kinds); `!I`
   parameter with an ordinary interface value; a `!` result is a source
   (`:=`, `var`, multi-value `a, err :=`); `== nil` / `!= nil` → GW001.
2. S1: two types sharing a method name with different contracts; a method and
   a plain function sharing a name; declaration order swapped — each call
   checked against its own declaration only.
3. S3: `T.M(x, nil)` and `(*T).M(x, nil)` with the one-position offset.
4. S4: promotion through one and two levels of embedding; a promoted method
   declared in another file is unchecked.
5. S5: a call through the interface is checked against the interface's `!`; a
   call through the concrete type against the concrete declaration; an
   implementing method **without** the `!` is accepted (D5 unchanged).
6. S6: `f := b.Set; f(nil)` and a method passed as a callback produce no
   diagnostic — pinned.
7. S7: generic receiver (`Box[int].Put(nil)`), and a method that declares its
   own type parameters.
8. S9/S10: with no type information nothing is reported; a method declared in
   another file is not consulted.
9. Regression: every v1.0–v1.9.2 test passes; code with no `!` on any method
   produces byte-identical diagnostics (differential run over the repository's
   `.gon` files and the 540-file corpus).

## 10. Open Questions for the Owner

1. **Q1 — S2 scope.** Parameters **and** results together (proposed), or
   results first and parameters in a follow-up?
2. **Q2 — S5 interface calls.** A `!` on an interface method signature applies
   to calls through that interface (proposed), or interface method signatures
   stay documentation only?
3. **Q3 — S6 method values.** Leave untracked and documented (proposed), or
   add a warning when a method with a contract is taken as a value?
4. **Q4 — S8 receivers.** Defer receiver contracts to their own RFC
   (proposed)?
5. **Q5 — S9 scope.** Scope is the files checked together — one file, or the
   whole package under M8's package mode (proposed), with module-local
   cross-package contracts as a later, separate design?
6. **Q6 — Release shape.** Solo v1.11 after M8 (proposed, R1; sequencing already
   decided in `rfc-package-check.md` Q6).

## 11. Alternatives Considered

- **Keep method contracts documentation-only.** Honest, but it leaves the
  `Put` vs `(b *Box) Put` asymmetry and makes the source misleading.
- **Add methods to the name-keyed tables with a type prefix**
  (`Box.Set`). Fixes the collision for direct calls but not for promotion,
  interfaces, generics, or an aliased receiver; every one of those is something
  `go/types` already resolves by object.
- **Track method values.** Needs value flow (I1); rejected.
- **Require implementers to repeat an interface method's `!`.** A new
  satisfaction rule; contradicts v1.4 D5 / I3.

## 12. Exercise: measurements taken before any decision (2026-11-14)

### 12.1 Baseline: what is checked today

A file with a method `Set(p !*T)` and a method `Get() !*T`, called seven ways,
run with Gon v1.9.1:

| Call form | Diagnostic today |
|---|---|
| A. `b.Set(nil)` direct | none |
| B. `b.ValSet(nil)` value receiver | none |
| C. `w.Set(nil)` promoted through an embedded field | none |
| D. `s.Set(nil)` through an interface declaring `Set(p !*T)` | none |
| E. `f := b.Set; f(nil)` method value | none |
| F. `(*Box).Set(b, nil)` method expression | none |
| G. `g := b.Get(); g == nil` result | none |

Inside the bodies the contracts do hold: `return nil` in `Get() !*T` is GN001,
and `p` is a non-nil local in `Set`.

### 12.2 Name collisions in real code

Syntactic scan of the 540-file standard-library corpus (one file at a time, as
Gon checks):

- 2,733 methods and 1,867 plain functions;
- **21** method-named-like-a-function pairs in **10** files;
- **161** method names declared on two or more receiver types in **55** files.

This is the structure that made the name-keyed tables unsound.

### 12.3 What a single-file checker can see

Of 12,435 `x.M(…)` selector calls in the corpus, **3,924 (31.6%)** name a
method that is declared in the same file. The remainder need a declaration in
another file or package.

### 12.4 Feasibility: can `go/types` find the declaration?

A throwaway test resolved each call form through `info.Selections` and
`info.Defs`:

| Form | `Selection` | Resolves to its declaration |
|---|---|---|
| A direct, B value receiver | MethodVal | yes |
| C promoted through embedding | MethodVal | yes (the embedded type's method) |
| D through an interface | MethodVal | yes (the interface method) |
| F method expression | MethodExpr | yes |
| G result of a method call | MethodVal | yes |
| H generic receiver (`G[int].Put`) | MethodVal | yes, via `Func.Origin()` |
| E method value (`f(nil)`) | none — `f` is an ordinary identifier | **no** |

Seven of eight forms resolve by object identity; the eighth cannot, by
construction, which is S6.

Limits: one corpus (the standard library, which contains no `!`), a purely
syntactic scan for §12.2 and §12.3, and a feasibility test on a single
hand-written file. None of it measures how many *annotated* methods exist in
user code.

## 13. Invariant Check (I1–I7)

- **I1** — upheld: S6 refuses to follow a method through a value; no flow.
- **I2** — upheld: only explicit `!` on a declaration is a contract (S11.5).
- **I3** — upheld: matching is by `go/types` identity; `!` does not change
  method sets or implementation (S5, D5).
- **I4** — upheld: no `.gna` change; the `.gna` method path is untouched.
- **I5** — upheld: no runtime or code-generation change.
- **I6** — upheld: a method value does not carry its contract (S6).
- **I7** — upheld: no new syntax; schema stays **1**.

---

**Proposed decisions (summary)**

| Decision | Proposal |
|----------|----------|
| Match a call to a contract | By `go/types` object identity, never by name (S1) |
| Direct calls | Parameters (GN001) and results (non-nil source, GW001), as for functions (S2) |
| Method expressions | Same, with the receiver offset (S3) |
| Promotion through embedding | Resolves to the declaring method (S4) |
| Calls through an interface | Interface's declaration applies; no satisfaction rule (S5, Q2) |
| Method values | Not tracked, documented (S6, Q3) |
| Generic receivers | Through `Origin()` (S7) |
| Receiver contracts | Out of scope (S8, Q4) |
| Scope | The files checked together: a file, or the package under M8 (S9, Q5) |
| New diagnostic codes / schema bump | None |
| Release shape | Solo v1.11, after M8 (S10, Q6) |
