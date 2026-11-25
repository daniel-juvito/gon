# Backlog notes (2026-11-22)

A note kept at the owner's request, not a decision or a schedule. The owner's words: "simpen dulu list2 nya buat v2 ... sekarang fokus v1 dulu". 

## v2 candidates (owner direction, later)

- **Runtime checking.** A `(!T)(x)` that checks at run time (panic on nil) instead of being only a claim. Today RFC 6.2 says no
  run-time check, README says Gon does not enforce at run time, and the generated Go is `(T)(x)` (RFC 6.9). It would change the generated
  Go and add a cost, so it needs its own RFC.
- **Static analysis** beyond structural forms (the owner's word: "static analysis, dll"): flow-sensitive nil analysis is a non-goal of v1
  (README); whether and how it comes is open.
- Part B items that the owner may decide to move here instead of v1 (see below).

- **Contracts on mutable containers and inner pointer levels** (discussed 2026-11-22, not decided). Today an element contract (`[]!*T`, `map[K]!*V`) is a
  construction-time obligation, and `!T` flows into `T` silently, so a write through an alias (`var u []*T = s; u[0] = nil`) breaks it unnoticed (checked on
  v1.22.0). Closing it needs the container types to be invariant in their contract, and a contract tree that follows the type tree (`!*!*int`, an inner
  `!` after `*`, which the parser rejects today). A larger change, R1. Not started.
- **Nilability overlay, invariance, and the backdoor register** (discussed 2026-11-22/23, not decided): the goal "support nilability without introducing a backdoor", the register of
  paths found by testing on v1.22.0, the two-layer design (`!T1 <: T2` iff `T1 = T2`, an inner contract is invariant, only the outermost `!` can be dropped) and its surface are in
  `docs/nilability-backdoors.md`. Whether it ships as v1.x minors or as v2 is the owner's decision.

- **Four local backdoors (zero-valued arrays, `make`, named `!T` results)** (measured 2026-11-23/25; `docs/rfc-cheap-backdoors.md`, accepted 2026-11-25 with the owner's four answers: C2 strict, A+B+C1 first and C2 later, GN002, the reslice limit documented with B).
  **Status (2026-11-25): rules A, B and C1 are implemented on `main` (PR #94) and documented, not released; rule C2 (a bare `return` with a named `!T` result) is not implemented.**

## v1 open items (focus now)

- **Nilable to non-nil must be explicit: an error, in v1** (owner, 2026-11-22; corrects the earlier note that put this in v2).
  An ordinary value of a nilable kind flowing into a `!T` slot (`x = p`, `x = (*int)(nil)`) stops being the warning GW006 and
  becomes an error, so the only way through is `(!T)(p)`. Consistent with interfaces, where an ordinary value into `!I` is already
  GN001 (D3b). It reverses the severity decision of Part A (RFC OQ-1, section 6.6, "GW006 is a warning"), which shipped in v1.21.0,
  so it is an amendment of an accepted RFC, a breaking change (R1) and its own release; it also closes the `(*int)(nil)` gap below.
  Needed, and done: an RFC amendment, a measurement (0 sites in gon production, 14 GW006 in gon-examples, 2 in gon-vscode fixtures, on
  v1.21.0) and the migration plan for the examples (their pin follows the release). **Status (2026-11-22): accepted as amendment A2, code GN001, implemented (checker PR #85, documents after it) and released in v1.22.0; the pin and the examples of the other two repositories follow the release.**

- **Part B with `gon convert`** (owner's proposal, 2026-11-22): written as `docs/rfc-part-b-binding-and-convert.md` (PROPOSED, not accepted). It keeps §6.8 and adds a migration tool, an order of release without a flag day, and one tightening (`&x` is not bound).
- **Part B** of the explicit-conversion RFC: audits are in `docs/measurements/partb-initial/` (#81) and the follow-up in
  `docs/measurements/partb-followup/`. Open design points found there: `x = nil` on an inferred local becomes
  GN001 (release idiom), traversal loops `for p := &head; p != nil; p = p.next` get GW001 "always true" and GW006 (GN001 since A2), interaction of
  section 6.8 with the `&operand` amendment, package-level `var x = e`, `make`/`new` with a type-parameter core type.
- **The `(*int)(nil)` gap** (found 2026-11-22): `x = (*int)(nil)` into a `!*int` slot was only GW006 (**closed by A2: it is GN001**). Option B, no longer needed: GN001 when the operand of a
  conversion is the literal `nil`. Not breaking in intent (the code already violates the contract). Superseded by A2.
- **`--strict`** and **GW007** (hint for `var x T = e`): undecided.
- **RFC text**: no known stale figure left (the "2 and 2" and "10 and 4" markers were added in #82).
