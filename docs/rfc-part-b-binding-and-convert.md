# RFC: Binding by inference (Part B) with `gon convert`

**Status: PROPOSED, 2026-11-22. Not accepted, not scheduled, not implemented.** This is the proposal of the owner (Part B in full,
plus a migration tool) written down so that it can be reviewed; nothing in the checker changes until it is accepted and an implementation
is separately authorised.
**Amends / continues:** `docs/rfc-explicit-nonnil-conversion.md` (Part B: C12 to C15, §6.6, §6.8, §9 I2, OQ 9 and 10) and its amendment A2
(an ordinary flow into a `!T` slot is GN001, released in v1.22.0). Related: `docs/backlog.md`,
`docs/measurements/partb-initial/`, `docs/measurements/partb-followup/`.
**Measured against:** gon v1.22.0 (`main` ca5bd18), `gon-examples@6f838d4`, `gon-vscode@9ab196a`.

## 1. Summary

Part B binds a local variable `!` (by `:=` or by `var x = e` with no written type) from the structural form of its initialiser (`x := make(...)`, `&T{}`, `new`, a literal, a function literal,
`append` with elements): the initialiser is certainly non-nil, so the variable is a non-nil source and flows from it into `!T` slots need no
conversion. Part B was left open because, after A2, the binding **breaks correct Go** whenever the variable is later reassigned to something nilable
(`buf = nil`, `p = p.next`, `x = f()`): the follow-up audit counted 1% to 3% of such bindings in ordinary Go.

This RFC keeps Part B as written in §6.8 and adds what makes it safe to adopt:

1. **`gon convert`**, a migration tool that rewrites `x := e` to `x := (T)(e)` exactly where the binding would break the code, so that the
   variable stays nilable. It is a flow-insensitive, one-pass rewrite per function, and the checker validates its output.
2. **An order of release that needs no flag day**: the tool ships first and is useful on today's Gon, because `x := (T)(e)` is valid Go and
   changes nothing today; the binding ships later, alone (R1), when users have had the tool.
3. **One tightening of §6.8**: a local whose address is taken (`&x`) is not bound, because an alias can write `nil`.
4. **A hint in the diagnostic**: the GN001 that Part B adds on a bound local says how to keep it nilable.

`T` stays nilable and `!T` stays non-nil; the meaning of no existing type changes, and no runtime check is added.

## 2. Why now

A2 made the rule one-sided: a value that is not a recognised source cannot enter a `!T` slot without `(!T)(x)`. A recognised source needs no
conversion. The more locals are recognised, the fewer conversions an author writes, which is the direction of the owner ("the more non-nil the
better"). The measured cost of Part B was never the flows it silences (11 flows in `gon-examples`, 0 in `gon`) but the correct code it rejects.
`gon convert` moves that cost from the author's keyboard to a one-time, reviewable, machine-checked rewrite.

## 3. The rule (unchanged from §6.8, with one addition)

This RFC concerns **local variables**, declared in a function body. A package-level `var x = e` is outside it: it is not bound by Part B and the tool does not touch it.

`x := e` and `var x = e` (no written type), with `e` a recognised non-nil source of a nilable kind, bind `x` `!`, per variable and per position
(§6.8, "New identifiers only" and "Multi-value forms"). On a bound local, `x == nil` is GW001, `x = nil` is GN001, and `x = e'` is a flow into a `!T`
slot: silent for a recognised `e'`, GN001 otherwise (A2).

**Addition P1 (new): a local whose address is taken (`&x` anywhere in its function, closures included) is not bound.** Reason: `p := &x; *p = nil`
makes `x == nil` possible and the GW001 "always false" wrong; for a declared `!` local the hole exists today and is not widened by this RFC.
The test is flow-insensitive and syntactic.

Two ways to keep a local nilable stay as written in §6.8, and both are used by the tool:

- a written type: `var x T = e` (C14);
- a conversion without `!`: `x := (T)(e)`. A conversion without `!` is not a recognised source (§6.6), so `x` is not bound. Unlike `var`, it
  can be written in the init of `for`, `if` and `switch`, and it keeps the scope and the per-iteration semantics of a loop variable (Go 1.22), which
  moving the declaration out of the loop would not.

Checked on gon v1.22.0 (Part B not implemented, so only the Go validity and the conversion's silence): `for p := (*N)(&head); p != nil; p = p.next { ... }`
and `buf := ([]byte)(make([]byte, 8)); buf = nil` produce no diagnostic.

## 4. `gon convert`

### 4.1 What it does

For every function body of the input, find each variable `v` declared by `:=` (or untyped `var`) whose initialiser is a recognised structural form of a nilable
kind (§6.8), and decide, **by syntax and `go/types` objects, in one pass, without following control flow**, whether the binding would break the code:

`v` is **kept nilable** (its initialiser is rewritten to `(T)(e)`) if, anywhere in the function, closures included:

| Use of `v` | Why the binding would break |
|---|---|
| `v = nil` | GN001 |
| `v = e'` where `e'` is not a recognised non-nil source (an ordinary call, a selector, an index, an identifier, a conversion without `!`) | GN001 (A2) |
| `v` on the left of a multi-value assignment `v, ok = ...`, `v, err = f()`, `v = <-ch`, `for v = range` | the value is not a recognised source |
| `&v` | an alias can write nil (P1) |

`v` is **left as it is** (it becomes bound when Part B ships) if every assignment to it is a recognised source (`v = make(...)`, `v = &T{}`), or
`v = append(v, ...)`, or `v = v[a:b]` (C13, C15), and `&v` is never taken. A variable that is never assigned again is the common case (80% to 88% of the
bindings measured).

`T` is the type of `v` printed from `go/types` with the qualifiers of the file's imports. The tool changes the **initialiser only** and keeps the form of the declaration:
`v := e` becomes `v := (T)(e)`, and `var v = e` becomes `var v = (T)(e)` (also inside a `var ( ... )` group). For `a, b := e1, e2` each variable is rewritten on its
own (`a, b := (T)(e1), e2`). The tool never turns a `:=` into a `var` or the reverse.

**Only the identifiers that the declaration declares.** As in §6.8 ("New identifiers only"), a short declaration that also assigns to variables which already exist in the
same scope (`x, err := f()` with `x` declared earlier) declares only the new ones. The tool decides from `go/types` (`Defs` against `Uses`), not from the spelling: an existing
identifier, ordinary `T` or `!T`, is never rewritten and its contract is never changed; an identifier of an inner scope with the same name as an outer variable is a new
identifier and is treated like any other; a blank `_` is never bound. In a declaration that mixes existing and new identifiers, only the new ones are candidates.

### 4.2 What it does not do, and says so

- It does not guess. A case it cannot rewrite is **reported** with its position and left unchanged: a `:=` from a multi-value call whose result position is a
  declared `!T` (`x, err := f()`, which would be bound), a type that cannot be written in the file (an unexported or anonymous type of another package), and any
  use it cannot classify. The exit code is non-zero if a report remains.
- It does not add `!` anywhere. It does not infer `!` for parameters, results or fields, does not touch `!T` declarations, and does not change a `.go` file's
  meaning: `(T)(e)` of an expression already of type `T` is an identity conversion.
- It is flow-insensitive on purpose: it never reasons that "this assignment is not reached", so it may keep a variable nilable that a path analysis would let
  bind. That is the safe side.

### 4.3 Input, output, command

Open, to be decided by the owner (§8): `gon convert <file|dir|./...>` reading `.gon` (the users of v1.22 who already wrote `!`) and, as a second mode,
`.go`, which is renamed to `.gon` with the rewrite applied. The recommendation is `.gon` in place first (it is `gon fmt`'s contract: it writes the file, the
diff is the review) and `.go` input later.

### 4.4 Validation

The tool runs the checker on its own output (a built-in `gon check`) and fails if the output has any diagnostic that the input did not have. Positions move when the source is
rewritten, so the comparison does not use raw line and column: the tool keeps the map from each edit (an initialiser wrapped in `(T)( ... )`) to the original range, translates
every diagnostic position of the output back through that map, and compares the code, the severity and the original position. It is idempotent: a second run changes nothing. Both are properties that tests fix (§7).

## 5. Release order (no flag day)

1. **Release A: `gon convert`.** On today's Gon the rewrite is a no-op semantically: `x := (T)(e)` is valid Go and, with no binding, `x` is nilable either way.
   Users run it, review the diff, commit.
2. **Release B: the binding (§3), alone, R1**, with a compatibility paragraph in the CHANGELOG that names the tool and the hint of §6. Code that was converted
   does not change behaviour; code that was not gets the GN001 with the hint.

This is the part of the design that answers "are we going to change other people's code": the tool changes it, on request, visibly, before the rule exists.

## 6. The diagnostic on a bound local

The GN001 that Part B adds on a bound local is worded so that the fix is in the message, as for the `(!T)(x)` hint of A2:

`cannot use value of type S as !U; x was bound non-nil by its initialiser at <position>: declare it with a written type (var x T = ...) or convert the initialiser ((T)(e)) to keep it nilable`

The initialiser's position would be an entry of `relatedInformation`, the optional field that Protocol v1 defines (`docs/diagnostic-protocol-v1.md`, "Related Locations"). Protocol v1 does not require
the checker to produce it, and today's checker does not (the field is a schema with no producer); using it here is a separate decision, and until it is taken the message text carries the
position. Consumers must still not rely on the wording.

## 7. Evidence and test plan

**Measured (scratch, syntax only, by name; upper bounds), `docs/measurements/partb-followup/` and the re-audit after A2:**

| Corpus | Bindings of a recognised form | Kept nilable by §4.1 (error without it) |
|---|---|---|
| `gon` production | 91 | 1 (`x = c.pkg.files`, `method_contracts.go:54`); `goArgs = append([]string{"build"}, args[2:]...)` has a recognised base and stays bound |
| Go 1.24.7 standard library | 5826 | at most 184 (3.2%), of which `x = nil` 12 |
| consul 1.19.2 | 2812 | at most 40 (1.4%) and 38 with `&x` |
| cobra, testify, gin, zap, client_golang | 43, 10, 19, 24, 100 | 1, 0, 0, 0, 1 |

The "at most" figures count `append(spread)` with a recognised non-variable base as an error, which §4.1 does not; the true counts are lower. They are not Gon
sources; they show how Go is written.

**Tests the implementation must have (written before the change, red without it):** the conformance harness gets entries for each row of §4.1 (a rewrite, and a
variable left alone), for the `for`, `if` and `switch` init forms, closures, shadowing (an object of another scope with the same name), generics, multi-value
forms, `&v`, and the reports of §4.2; idempotence and "no new diagnostic" as properties over the whole `testdata/`; mutation tests that only count mutations that
compile (drop the `&v` rule, treat `nil` as a recognised source, ignore closures); the three repositories as a differential run (`gon convert` then `gon check`: 0 new
diagnostics, 0 rewrites in `gon` production but the one genuine site).

## 8. Questions for the owner (open)

1. The command and its modes (§4.3): `.gon` in place first, `.go` input later, or both at once; flags (`--check` to only report, `--write`).
2. P1 (`&x` is not bound): accepted as an addition to §6.8?
3. The wording of the hint (§6), and whether the initialiser position is sent as `relatedInformation` (the checker has no producer for that field yet).
4. Whether Release A (the tool alone) ships before the binding, as §5 proposes, or the tool and the binding ship together.
5. Whether the tool should, later, also propose `!` for parameters, results and fields. This RFC does not.

## 9. Alternatives

- **Do not accept Part B.** Authors write `(!T)(x)` or declare the local `!T`. Simplest; the 11 flows of `gon-examples` need a conversion each.
- **Bind only locals that are never assigned again** (80% to 88% of the bindings; no new error on correct code by construction). No tool needed. It binds
  less than this RFC, which binds a variable whose every assignment is a recognised source (about 96% to 97%).
- **Bind everything and let the author fix the errors** (§6.8 as it stands). 1% to 3% of bindings become errors on correct Go with no tool; rejected in the
  re-audit.
- **Default non-nil with a nilable marker** (v2, `docs/backlog.md`). Changes the meaning of `T`; out of scope.

## 10. Limits

A proposal; no code. The measured figures are syntax-only upper bounds on Go, not Gon, code; Part B and the tool have no implementation, so every claim about their
behaviour is a design claim except the two conversions checked in §3. The variables that §4.2 cannot rewrite have not been counted. Users outside the three repositories
are not measured.
