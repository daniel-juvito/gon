# Nilability: the goal, the backdoor register and the test points

**Status: notes and measurements, 2026-11-22/23. Not a decision, not a schedule.** Written at the owner's request ("catet semua tadi"). Every result below was
obtained with gon **v1.22.0** (`go install ...@v1.22.0`), one small `.gon` file per case, unless the row says *not tested*. A row that says *not tested* is not a negative
result.

## 1. The goal (the owner's words, 2026-11-23)

> Support nilability without introducing a backdoor.

Read as a statement that can be checked: **every path from a nilable value to a non-nil slot is either explicit (`(!T)(x)`, which can be found with grep) or checked
by the checker.** A path that is neither is a backdoor. `(!T)(x)` is a door, not a backdoor: it is explicit and the author answers for it (RFC §6.2, no run-time check).

## 2. Register

### 2.1 Closed (checked), v1.22.0

| Path | Result |
|---|---|
| an ordinary value of a pointer, slice, map, channel or function kind into a `!T` slot | GN001 (A2) |
| the `nil` literal, and `(!T)(nil)` | GN001 |
| a bare `var x !*T`, a required `!` field left at zero | GN002 |
| a write of `nil` to a `!` field, also through a pointer to the struct (`pp.P = nil`) | GN001 |
| a `range` variable, or a value received from a channel, into a `!T` slot | GN001 (they are ordinary values) |
| the interface rows (D3a, D3b, D3d) | unchanged |

### 2.2 Open: silent paths found by testing (no diagnostic)

| Path | Example | Result |
|---|---|---|
| a container with an element contract, through an alias | `var b []*int = a` (`a` is `[]!*int`), then `b[0] = nil` | accepted; `a[0]` is nil. The same for `map[K]!*V` |
| a container back into an element contract | `var c []!*int = b` | accepted |
| elements created by `make` | `make([]!*T, 3)` | accepted; the elements are nil |
| a fixed array at its zero value | `var arr [3]!*T` | accepted |
| a write to a map element | `m["a"] = nil` (`map[string]!*T`) | accepted |
| `append` | `append(s, nil)` on `[]!*T` | accepted |
| an ordinary value into an element | `s[0] = ord()` | accepted |
| a function value | `var f func() !*T = ord` (`ord` returns `*T`) | accepted |
| an interface whose method result is `!T` | `Impl.M() *T` assigned to `I` with `M() !*T` | accepted |
| a named result with a bare `return` | `func f() (x !*T) { return }` | accepted; `x` is nil |

### 2.3 By design (rejected, GN003), not a blind spot

| Path | Result | Why (RFC) |
|---|---|---|
| `!` on a channel element (`chan !*T`) | GN003 | element contracts are construction-time and a channel has no construction site (element contracts E1, locked) |
| `!` in a type argument (`Box[!*int]`) | GN003 | a type argument's identity is `go/types'`, so `!` cannot ride along (generics G4). A contract can still be declared in the generic declaration (`struct{ v !*X }`) |
| `!` on an inner level of a pointer (`*!*int`, `!*!*int`) | parse error | `!` is accepted only on the outermost type and on element positions; no syntax for inner pointer levels |

### 2.4 Outside the checker (cannot be closed from inside Gon), *not tested*

Data races and goroutines, `reflect`, `unsafe`, cgo, `json.Unmarshal` into a `!` field, a write through an address that escaped to another function (`json.Unmarshal(data, &x)`),
panic and `recover`, a callback that modifies a pointer or a container, shared global state. These are limits of the host language, to be stated, not tested with `gon check`.

## 3. The principle behind 2.2

A non-nil contract is safe for a value that is only read: `!T` may flow into `T` (a subtype). It is not safe for a **place that can be written through an alias** (a slice, a map, the
contents of a pointer, a variable whose address is taken): two names for one place can carry two contracts. Gon today does not model the contract of such a place after construction, so
`[]!*T` and `[]*T` are one Go type and one contract-free type for the checker (a write through the weaker name is silent). The `&x` rule of the Part B RFC (P1) is the same principle applied to a variable.

## 4. Design direction (not decided; v1.x in stages or v2, the owner decides)

Two layers. Layer 1 is the Go type system, unchanged (assignability, embedding, interface satisfaction, conversions, generics). Layer 2 is a nilability overlay, a tree of `!` marks on the Go
type, with its own relation: **`!T1 <: T2` iff `T1 = T2`**, where equality includes the inner `!` marks. Only the outermost `!` can be dropped; an inner contract is invariant. So
`!*!*T` is a subtype of `*!*T`, and neither `!*!*T` nor `*!*T` flows into `**T` or `!**T` without an explicit conversion. Where two Go types are different but assignable (a named type, a value into an
interface), the existing rule stays: the contract is not carried without `(!U)(x)` (C11).

What it would touch: the tree itself (`types.Identical` does not see `!`), every place a value moves (assignment, argument, result, field, element), generics (a contract on a type argument, `P ~*T`,
inference), function values (parameters contravariant, results covariant), interface method signatures, channels, named types and aliases, derived expressions (index, selector, dereference,
`range`, composite literals, type assertions), `.gna` (a string syntax for nested contracts), the preprocessor and `gon fmt` (an inner `!`). It is an R1 change and large; a possible order: the tree and its
comparison at one level, then containers, then generics and functions, then `.gna`. Before building it, measure how often element contracts and nested pointers are used.

Not decided: whether it ships as v1.x minors, each alone, or as v2 (which would also move the module path to `.../v2`).

## 5. Test points

Each point is a question with one answer: is there a diagnostic, yes or no. *Tested* means a case in section 2; every other point is *not tested*.

**P0, the blind spots most likely to mislead**

| Point | State |
|---|---|
| function value: assignment `func() !*T` from `func() *T` | tested: silent (2.2) |
| function value: parameter contract that differs; result that differs; a value from a variable or a selector | not tested |
| interface method `!T` result implemented by `T`; also the reverse (`T` in the interface, `!T` in the implementation) | first direction tested: silent; the reverse not tested |
| interface: a different parameter contract; interface to interface; pointer against value receiver; embedded interface | not tested |
| named result with a bare `return`; with `nil`, an ordinary value or a recognised source assigned first; several return paths | bare return tested: silent; the rest not tested |
| zero values: `var arr [N]!*T`, `make([]!*T, n)` | tested: silent. `var x !*T` is GN002 (2.1) |
| `make([]!*T, n, cap)`, `make(map[K]!*T)`, a struct with a nested contracted array or container | not tested |
| contract lost and restored: `[]!*T` to `[]*T` to `[]!*T` | tested: silent in both directions |

**P1, propagation**

| Point | State |
|---|---|
| a write through an alias: slice, map | tested: silent. Through a parameter, a returned pointer, a method receiver, a closure | not tested |
| `append(s, ord())`, `append(s, nil)`, `s[i] = ord()` | `append(s, nil)` and `s[i] = ord()` tested: silent; `append(s, ord())` not tested |
| `range` over a slice into `!T`; over a map; over a channel; a channel receive | slice and receive tested: GN001; map and channel not tested |
| index, selector, dereference, nested dereference, type assertion, type switch, tuple assignment, a short declaration with several variables, parentheses | not tested (except what the Part A conformance harness already covers) |
| generics: inference with and without `!`, explicit against inferred type arguments, a generic method, a generic receiver, a generic `.gna`, `P ~*T` | not tested here (the repository has tests of its own for G3 to G5) |
| `.gna` contract propagation | not tested here |

**P2, the boundary** (section 2.4): *not tested*, and not testable with `gon check`.

## 6. Limits of this note

One toolchain (gon v1.22.0), one file per case, no figure for how often any of these appears in real code. The two registers of "tested" and "not tested" are kept apart on purpose: do not read a
*not tested* row as safe or as unsafe.
