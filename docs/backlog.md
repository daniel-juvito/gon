# Backlog notes (2026-11-22)

A note kept at the owner's request, not a decision or a schedule. The owner's words: "simpen dulu list2 nya buat v2 ... sekarang fokus v1 dulu". 

## v2 candidates (owner direction, later)

- **Runtime checking.** A `(!T)(x)` that checks at run time (panic on nil) instead of being only a claim. Today RFC 6.2 says no
  run-time check, README says Gon does not enforce at run time, and the generated Go is `(T)(x)` (RFC 6.9). It would change the generated
  Go and add a cost, so it needs its own RFC.
- **Static analysis** beyond structural forms (the owner's word: "static analysis, dll"): flow-sensitive nil analysis is a non-goal of v1
  (README); whether and how it comes is open.
- Part B items that the owner may decide to move here instead of v1 (see below).

## v1 open items (focus now)

- **Nilable to non-nil must be explicit: an error, in v1** (owner, 2026-11-22; corrects the earlier note that put this in v2).
  An ordinary value of a nilable kind flowing into a `!T` slot (`x = p`, `x = (*int)(nil)`) stops being the warning GW006 and
  becomes an error, so the only way through is `(!T)(p)`. Consistent with interfaces, where an ordinary value into `!I` is already
  GN001 (D3b). It reverses the severity decision of Part A (RFC OQ-1, section 6.6, "GW006 is a warning"), which shipped in v1.21.0,
  so it is an amendment of an accepted RFC, a breaking change (R1) and its own release; it also closes the `(*int)(nil)` gap below.
  Needs: an RFC amendment, a measurement (today 0 sites in gon production, 14 GW006 in gon-examples, 2 in gon-vscode fixtures) and a
  migration plan for the examples. **Status (2026-11-22): accepted as amendment A2, code GN001, implemented on `main` (checker PR #85, documents after it), not released; the pin and the examples of the other two repositories follow the release.**

- **Part B** of the explicit-conversion RFC: audits are in `docs/measurements/partb-initial/` (#81) and the follow-up in
  `docs/measurements/partb-followup/`. Open design points found there: `x = nil` on an inferred local becomes
  GN001 (release idiom), traversal loops `for p := &head; p != nil; p = p.next` get GW001 "always true" and GW006, interaction of
  section 6.8 with the `&operand` amendment, package-level `var x = e`, `make`/`new` with a type-parameter core type.
- **The `(*int)(nil)` gap** (found 2026-11-22): `x = (*int)(nil)` into a `!*int` slot was only GW006 (**closed by A2: it is GN001**). Option B, no longer needed: GN001 when the operand of a
  conversion is the literal `nil`. Not breaking in intent (the code already violates the contract). Not decided.
- **`--strict`** and **GW007** (hint for `var x T = e`): undecided.
- **RFC text**: no known stale figure left (the "2 and 2" and "10 and 4" markers were added in #82).
