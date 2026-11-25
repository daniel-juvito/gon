# RFC: Closing four local backdoors (zero-valued arrays, `make`, named `!T` results)

**Status: ACCEPTED as a design decision (2026-11-25, the owner). Rules A, B and C1 are IMPLEMENTED on `main` (PR #94), not released; rule C2 is not implemented.** The four rules and the owner's answers to the questions of §9 are recorded below. The acceptance was a design decision only (it did not approve an implementation or a release): the implementation of A, B and C1 was authorised separately and merged in #94, and rule C2 and any release need their own go-ahead; the release number and date remain the owner's.
**Amends:** `docs/rfc-element-contract-construction.md` (E5, "Fixed-array zero-fill"; the construction-time principle of §1) and
`docs/rfc-return-value-contracts.md` (§5.4, "Named results: names are documentation only"). **Related:** `docs/nilability-backdoors.md` (the register),
`docs/backlog.md`.
**Measured against:** gon v1.22.0 (`main` 8534ed7), `gon-examples@6f838d4`, `gon-vscode@9ab196a`.

## 1. Summary

The register (`docs/nilability-backdoors.md`, §2.2) lists paths where a nil enters a non-nil position with no diagnostic. Most need a contract that follows the type
(the overlay of its §4), which is a large change. Four do not: each is decided by one declaration or one call, with no alias and no flow analysis.

| Rule | Position | New diagnostic |
|---|---|---|
| **A** | a fixed array whose element carries `!`, created at its zero value: `var a [N]!E`, `new([N]!E)`, a struct field of such a type reached by a zero value | GN002 |
| **B** | `make([]!E, n[, cap])` with `n` not the constant 0 | GN002 |
| **C1** | a named result declared `!T` is a `!` variable in the function body | GN001 (as for a parameter) |
| **C2** | a bare `return` in a function that has a named result declared `!T` | GN002 |

A and B are the zero-value containment that E5 already applies to array **literals**, extended to the other ways of making the same zero-filled array or slice. C1 treats a
named `!T` result like the parameters and local variables declared `!T`, which the checker already tracks. C2 is the only rule that rejects a form that is common in Go.
Each rule can be reviewed, accepted, and shipped on its own.

## 2. Rule A: zero-valued arrays of `!` elements

**Rule.** Where a value of an array type `[N]E` is created at its zero value, and `N` is a constant greater than 0, and `E` carries `!` of a nilable kind, the creation is **GN002**
(one per creation site, message of E5's family: `fixed array of non-nil elements is zero-valued`). The sites:

- `var a [N]!*T` with no initialiser, local or package level;
- `new([N]!*T)`;
- a named array type: `type Slots [4]!*T; var s Slots`, `new(Slots)`;
- a struct whose field has such an array type, when the struct is created at its zero value: `var s S`, `S{}` (the field not listed), `new(S)`. This is the nested walk
  that GN002 already performs for `!` fields (v1.3); it descends into an array field whose element is `!` and N is greater than 0.

**Not rejected.** `[0]!*T` (no element); `[...]E` (the literal decides); a literal that covers every index (E5); a variable with an initialiser that is not a zero value
(`var a [3]!*T = f()`, which is the flow rules' business).

**Measured on v1.22.0 (one file per case):** `var a [3]!*T`, `new([3]!*T)`, `type Slots [4]!*T; var s Slots`, and a struct with a `[2]!*T` field with `var s S`, `S{}`, `new(S)`
are all silent today; `[3]!*T{nn()}` and `&[2]!*T{}` are GN002 (E5).
**Corpus (syntax only, Go code without `!`, lower bounds):** the standard library has 25 `var [N]E` of a nilable element with no initialiser and 29 struct fields of such a type;
consul and five modules have 0 and 2. In the three repositories no `.gon` source declares a bare `var a [N]!*T`.

## 3. Rule B: `make` of a slice of `!` elements

**Rule.** `make([]!E, n)` and `make([]!E, n, cap)`, where `E` is a nilable kind, are **GN002** unless `n` is a **constant expression equal to 0** (`go/types` constant value, so `const z = 0` counts). The
capacity is free: `make([]!*T, 0, n)` followed by `append` is the usual way to preallocate and is accepted.

**Not rejected.** `make(map[K]!V)` and `make(map[K]!V, n)` (no elements are created); `make(chan ...)`; a slice with no element contract (`make([]*T, n)`); only an element `!` is the subject of this rule.

**Known limit, stated and not closed by this rule.** `s := make([]!*T, 0, 3); s = s[:3]` makes three nil elements visible; the array behind the capacity was created at its zero value.
The rule does not look at reslicing (that needs the contract to follow the slice value, the overlay). Measured on v1.22.0: silent. It is a limit to document, not a case to forbid.

**Measured on v1.22.0:** `make([]!*T, 3)`, `make([]!*T, n)` and `make([]!*T, 2, 8)` are silent today; `make([]!*T, 0)` and `make([]!*T, 0, 8)` are silent and stay so.
**Corpus (syntax only, lower bounds):** `make([]E, n)` of a nilable element with `n` other than a literal 0: standard library 210, consul 150, cobra 1, client_golang 5, `gon` 1. These
are shapes in plain Go; the rule fires only where the element is declared `!`.

## 4. Rule C1: a named `!T` result is a `!` variable in the body

**Rule.** A named result declared `!T` is treated, inside its function, like a parameter or local variable declared `!T`: `x = nil` is GN001 (`cannot assign nil to non-nil variable !x`), `x = e`
with `e` an ordinary value is GN001 (A2), `x == nil` is GW001, and `x` is a recognised non-nil source where it is used.

**Known limit (not closed by C1).** A named result holds its zero value, nil, until it is first assigned, and the rule treats it as a `!` variable from the first statement of the body, as it treats a parameter. Without flow analysis,
any use of the result before its first assignment (a comparison, `consume(x)`, a `return x`) is taken for a use of a non-nil value. Rule C2 and a later pass can narrow this; C1 does not.

**Why it is a change to the return-value RFC.** §5.4 says "Names are documentation only". For a result declared `!T` the name now also names a variable that the checker tracks.

**Measured on v1.22.0:** in `func f() (x !*T)`, the statements `x = nil`, `x = ord()`, `x = nn(); if x == nil {}` are all silent (no GN001, no GW001); `p = nil` on a parameter `p !*T` and
`x = nil` on a local `var x !*T` are GN001. The explicit forms `return nil` and `return ord()` are GN001 today.
**In the repositories:** no `.gon` source declares a named result with `!` (text search, 0 in `gon`, `gon-examples`, `gon-vscode`).

## 5. Rule C2: a bare `return` with a named `!T` result

**Rule (conservative, no flow analysis).** In a function that has at least one named result declared `!T`, a `return` with no operands is **GN002** (`bare return with a named non-nil result`). `return e` is
unchanged. A function whose results are named but none is `!T` is unchanged.

**Why it is the only form that rejects code common in Go.** Named results and bare returns are ordinary Go; this rule applies to them only when the author wrote `!` on a named result, so the rule
is on a position the author marked. Without flow analysis the checker cannot tell whether the result was assigned on every path (`x = nn()` on one branch only), so it asks for `return x`.

**Alternative (not chosen, §9):** allow a bare `return` when the result is assigned somewhere in the function (flow-insensitive). It is more permissive and less safe: `if c { x = nn() }; return` stays accepted.

**Measured on v1.22.0:** a bare `return` is silent in `func f() (x !*T) { return }`, with `x` assigned or not, assigned `nil`, assigned an ordinary value, assigned on one path only, assigned in a `defer`,
and with two results.
**Corpus (syntax only, lower bounds; named result of a nilable type with a bare `return`, which is a superset of the shape this rule touches because it counts results that are not `!`):** standard library 209
of 44,941 functions (0.5%), gin 12 of 424 (2.8%), consul 5, cobra 2, testify 2, zap 1, client_golang 1, `gon` 0. The count of functions that would be rejected is the count of those whose result is
declared `!`; no Gon source in the three repositories is.

## 6. What this RFC does not change

Aliases and the contract of a container after construction (the overlay); generics; interface and function-value signatures; `append`, map and index writes; `range`; reslicing. Each stays as
registered in `docs/nilability-backdoors.md`, §2.2, and each is a separate R1 change. `make(map...)`, `make(chan ...)` and slices with no element `!` are unchanged.

## 7. Compatibility and release

All four rules reject code that is accepted today, so the release is R1 and ships alone, with a compatibility paragraph in the CHANGELOG, as v1.21.0 and v1.22.0 did. A, B and C1 are narrow and
mechanical (the position is a declaration or a call with a constant); C2 can reject correct Go and has its own test matrix. They can ship together or in two releases; the owner decides.
No diagnostic changes for code with no `!` on an array element, a slice element or a named result.

**Evidence and its limits.** Single files on gon v1.22.0; counts of shapes in Go code without `!` (the standard library, consul and five modules, plus `gon` itself), by syntax only and by name,
named types not counted; no figure for Gon users who annotate these positions. The counts bound where an annotation could meet the rule; they are not counts of errors.

## 8. Tests the implementation must have (written before the change, red without it)

- **A:** every site of §2 (`var`, `new`, named array type, struct field with `var s S`, `S{}`, `new(S)`), `[0]!*T`, `[...]`, a full literal, an array of arrays, and package-level `var`.
- **B:** `make` with `n` = 3, a variable, `const z = 0`, a literal 0, with and without capacity; `make(map...)`; `make([]*T, n)` unchanged; the reslice case documented as accepted.
- **C1:** `x = nil`, `x = ord()`, `x = nn()`, `x == nil`, `x` used as a source, a result captured by a closure and assigned in a `defer`, and a result that is not `!` unchanged.
- **C2:** a bare `return` in each of the shapes measured in §5, `return x`, `return nn()`, a function literal with its own results, and a function with two results of which one is `!`.
- Mutation tests that only count mutations that compile (drop the constant-0 exception, treat a named result as an ordinary variable, forget the nested array walk of a struct).
- A differential run over the three repositories: 0 new diagnostics expected.

## 9. Decisions of the owner (2026-11-25)

1. **C2 is strict.** A bare `return` is rejected whenever the function has a named result declared `!T`; `return e` is unchanged. The permissive alternative (accepted when the result is assigned
   somewhere in the function) is not chosen: it would leave `if c { x = nn() }; return` accepted, which is a path already shown to be open.
2. **Release shape: A, B and C1 first, C2 later.** The first release carries A (zero-valued arrays), B (`make`) and C1 (a named `!T` result is a `!` variable in the body); C2 follows in its own
   release, because it is the one rule that rejects a form common in Go and has its own test matrix. Each is R1 and ships alone; the numbers and dates are decided when they are made.
3. **The diagnostic of C2 is GN002** (a required non-nil position, the named result, is left at its zero value by a bare `return`).
4. **The known limit of reslicing is documented together with rule B** (a slice of `!` elements created with a capacity can expose nil elements when it is resliced over its length; the
   rule looks at the length only).

## 10. Alternatives

- **Do nothing now** and wait for the contract overlay (`docs/nilability-backdoors.md`, §4). The four rules close paths that the overlay would also close, so none of this work is thrown away.
- **Only A and B.** They are the smallest and the closest to E5; C1 and C2 touch a rule of the return-value RFC.
- **Forbid named `!T` results.** Simpler to say, and it rejects code that has no problem.
