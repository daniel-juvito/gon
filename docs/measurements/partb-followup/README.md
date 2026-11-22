# Part B, follow-up audit (scratch, UNAUDITED)

Basis: `gon` main 5d82caa, gon-examples ed6f6f1, gon-vscode bb8689c, real gon v1.21.0. No source of the three measured
repositories was modified by the audit (this report and its tools were added to `gon` afterwards, as documents only). Three parts, as chosen: per-site classification, review of RFC section 6.8, measurement outside the three
repositories. Tools (scratch): `pbm` (the instrument of #81), `indep-main.go.txt` (a second, independent, syntax-only counter, no
types), `loop-main.go.txt` (traversal loops), real `gon check` for the controls. Their sources and the raw outputs are in this directory.

## 1. Per-site classification (gon production)

Independent check of the 91 new bindings. `indep_bin` reads the generated `.go` of every `.gon` source and counts, by syntax only,
every `x := e` whose `e` has a recognised structural form of a nilable kind. It found **91**, with the same split as the
instrument: `make`/`new` 51, `&operand` 24 (22 of a composite literal, 2 of another operand: the two that the old rule did not count, see the +2 of #81), slice/map literal 9, function literal 6, `append` with explicit elements 1.
Two tools that do not share code agree on the number and on the forms. The counter is syntax-only and counts any unary `&`; the split
22 and 2 above comes from a separate count of the operand of each `&` (not part of the counter's source), and the 2 equal the +2 of the reconciliation in #81.

Uses of these 91 variables later in the same function (by name, so an upper bound):

| Use | Count | Reading |
|---|---|---|
| `x == nil`, `x != nil` | 0 | no new GW001 |
| `x = nil` | 0 | no new GN001 (an error) |
| `x = <recognised form>` or `append` with elements | 20 | silent |
| `x = append(x, ys...)`, `x = append([]T{..}, ys...)`, `x = x[:0]` | 5 | silent: the base is recognised (C13, C15): `main.go:192`, `package.go:162,166`, `fmt.go:248,252` |
| `x = c.pkg.files` (`method_contracts.go:54`) | 1 | the one GW006 of Part B; a true positive (the field can be nil). Same site as in the RFC |

So Part B on gon production is 1 new warning and 0 new errors. Every one of the 91 is a form that Go makes non-nil.
Not read: the 12 `bound today` and the 441 `not bound`; the instrument only says why they are not bound.

gon-examples and gon-vscode: the Part B warnings (3 and 0) are the ordinary-call flows that Part A already reports and Part B does not
bind (flow-ok `get()`, element-contracts `maybe()`, generics). Part B adds no warning there. The one `GN001 NEW` of the instrument
(`c.DB = nil`) is a `.gna` field assignment that the real checker already rejects (demo.gon:34).

## 2. Review of section 6.8 (what the rule does to code that is correct Go)

Tested against the real checker with the declared-`!` equivalent (v1.21.0 treats a declared `!` local the way 6.8 would treat
an inferred one):

- **Release by `x = nil` (GN001, an error).** `buf := make(...)`, then `buf = nil`, is a standard Go idiom (let the GC free it; turn off a
  `select` case with `ch = nil`). With a declared `!` it is `GN001: cannot assign nil to non-nil variable` today
  (verified). Under 6.8 the same error would appear on inferred locals the author never marked. No such site in the three repositories.
  Outside: section 3.
- **Traversal loops `for p := &head; p != nil; p = p.next`.** Verified with a declared `!` local: `GW001: p is non-nil;
  comparison with nil is always true` (a false claim: the loop ends when `p` becomes nil) and `GW006` on `p = p.next`. This is a case
  of the `&operand` amendment of 2026-11-22 meeting 6.8: with `&literal` only, it was not bound. A correct program gets a wrong warning
  and an error-free but noisy loop. Rare in application code (0 in gon, 0 in consul), 16 in the Go standard library (15 in `runtime`).
- **Aliased writes** (`p := &x; *p = nil`) make `x == nil` possible even after the binding; the warning "always false" would be wrong.
  Not measured; the same hole exists for a declared `!` local today.
- **Open points of the text, not tested:** whether a package-level `var x = e` is meant to bind (6.8 speaks of locals);
  the rule for `make`/`new` with a type-parameter core type; whether closures that assign `nil` to a captured bound local are
  seen (the identifier is the same object, so they should be).

## 3. Outside the three repositories (syntax-only upper bounds, name-based, types not used)

A named type (`x := Foo{}` where `Foo` is a slice or map) and `make(T, n)` with a named `T` are not counted, so the binding
counts are lower bounds; the use counts match by name inside a function, so they are upper bounds.

| Corpus | Non-test `.go` files | Bindings of a recognised form | `x == nil` / `x != nil` by name | `x = nil` by name |
|---|---|---|---|---|
| Go 1.24.7 standard library | 3685 | 5826 | 55 | 12 |
| consul 1.19.2 | 1455 | 2812 | 8 | 1 |
| cobra 1.8.1, testify 1.9.0, gin 1.10.0, zap 1.27.0, client_golang 1.20.5 | 19, 25, 53, 59, 76 | 43, 10, 19, 24, 100 | 0 each | 0 each |

Read by eye, the 12 `x = nil` of the standard library are all of the release kind (`res = nil` in `go/types` and `types2`, `wholeArchive = nil` in
`cmd/go`, `cas = nil` in `runtime`, `merged = nil` in `regexp`, two in `crypto/x509`, `cmd/cgo`). Several of the 55 comparisons are
name collisions (a different variable of the same name), so I did not classify them one by one; the traversal loops above account for 16.
Order of magnitude: about 0.2 per 100 bindings is a `x = nil` that would become GN001, and about 1 per 100 a comparison that may become GW001.
These are not Gon sources: they show how Go programmers write, not how Gon users will migrate.

## 4. What this says about Part B

- On the three repositories Part B is small and clean: 1 new warning, 0 new errors, 91 new bindings that are all non-nil by construction.
- Its breaking surface is two idioms of ordinary Go: releasing a variable with `= nil` (an error) and traversal loops from `&x` (a wrong warning and a flow warning).
  Neither occurs in the measured Gon code; both occur in the standard library.
- Section 6.8 and the `&operand` amendment interact; the RFC does not mention it. This is a question for the owner, not a measured defect of Part A.

## 5. Limits

Instruments, not a checker; Part B is implemented nowhere; the second counter is syntax-only; corpora are samples (one standard library, six
modules); no Gon user code outside the three repositories; GC and select idioms were read from the standard library, not from Gon programs; the loop
count requires the exact shape `for x := &y; x != nil; x = ...`.
