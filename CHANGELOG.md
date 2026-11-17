# Changelog

## [1.15.0] — 2026-11-17

Exit codes of package loading, aligned with Diagnostic Protocol v1 §7 (audit of
every exit-1 path of `gon check --json`). Protocol v1 is unchanged:
`schemaVersion` stays 1, the envelope has the same shape, there is no new
diagnostic code or severity, and exit 0/1/2/3 keep their meaning.

### Changed

- **A package that cannot be loaded is now exit 2, not exit 1.** In package
  mode (a directory or `...` operand), these used to end in exit 1 with no
  envelope, the code §7 reserves for a check that completed; they are now
  exit 2 (input / configuration error), still with no envelope:
  - the `go` command is missing or fails;
  - the module configuration is invalid (for example an unparsable `go.mod`);
  - a `.gon` file in the directory is excluded by build constraints;
  - with `./...` or several operands, any of the above in one package: the
    whole invocation is exit 2 (it used to be exit 1).
  Scripts that read the exit code of `gon check` / `gon vet` on a directory
  should treat 2 as "could not check". The **directory form of `gon transpile`
  and `gon build`** shares the loading step and is exit 2 for the same
  packages (it was 1); nothing is written, as before.
- **One message for a file excluded by build constraints**, whatever the file
  order: `excluded by build constraints: <files> (package mode checks every
  .gon file in the directory)`, every such file listed, sorted. It used to be
  "no type information" when the excluded file sorted first and "is not part of
  the package (excluded by a build constraint?)" otherwise. The message is only
  used when Go's build rules (`go/build`) reject the file; when the go command
  left a file out for another reason (for example a tag in `GOFLAGS`), the
  failure is reported generically, still exit 2.

### Fixed

- **An invalid `package` clause in package mode is a parse error**, reported
  like any other package-mode syntax error: the envelope is written (with the
  diagnostics of the package's other files, which are still checked), exit 1,
  and the Go loader's message (`expected 'package', found …`) on stderr. It
  used to be exit 1 with no envelope and the misleading "is not part of the
  package (excluded by a build constraint?)". In `gon transpile` /
  `gon build` on a directory it is exit 1 and nothing is written.

### Documentation

- Protocol v1 §7 separates three cases: the file-mode pre-check parse failure
  (exit 1, no envelope; the only exit 1 without one), package-mode loader
  errors (exit 1, envelope, message on stderr, labelled `gon: Go type error:`
  and positioned in the overlaid `.go` path), and package-loading failures
  (exit 2, no envelope). It states the **all-or-nothing** rule: one envelope
  with every operand's diagnostics, or none when an operand fails before
  emission; diagnostics of earlier operands are then not sent.

## [1.14.0] — 2026-11-17

### Added

- **Cross-package contracts from module-local `.gon` sources** — M10
  (`docs/rfc-source-contracts.md`). A `!` on an exported declaration of a
  `.gon` package now reaches the packages that **import** it: for an import
  inside the **main module** whose directory holds `.gon` sources, the
  contracts are derived from those sources, so no `.gna` has to be written for
  your own packages. Until now an importer read an imported package's
  contracts from `.gna` only, so `c := gna.DefaultChain(…); c == nil` was not a
  GW001 in Gon's own `cmd/gon` although `DefaultChain` returns `!*Chain`.
  - What is derived is every member an importer can reach: exported functions,
    and exported methods, interface methods and struct fields **whatever the
    name of the type that holds them** (an exported method or field of an
    unexported type returned by an exported constructor is reachable). Keys are
    those of a hand-written `.gna` (`Type.Method` without type arguments, also
    for Go 1.27 generic methods), and the existing `.gna` machinery applies
    them: GN001 / GN002 / GW001 in the importer, GN003 / GW004 validation.
  - The package being checked never reads itself; its own contracts come from
    its source as before.
  - **GW005** (new warning): a `.gna` for such a package is ignored, because the
    `.gon` sources are authoritative; reported once per import declaration.
  - A `.gon` source of an imported package that cannot be parsed is **GN003**
    once at the import, and no contract of that package applies (no fallback to
    a `.gna`).
  - GW002 never applies to a package whose contracts come from its `.gon`
    sources (a symbol declared in a plain `.go` file of such a package is
    ordinary, not "unannotated").
  - A **defined type** whose underlying type is a struct of the same package
    (`type Derived Base`, two hops, `type GI G[int]`) carries that struct's
    field contracts under its own name (no methods: Go gives it none of the
    base's). A base declared in another package is not followed.
  - Unchanged: the standard library, dependencies in the module cache (even if
    they ship `.gon`) and main-module packages without `.gon` keep using
    `.gna`.
- File mode and package mode alike. `gon version` reports `1.14.0`.

### Fixed

- **An alias of an annotated external type had no field contract.** With a
  `.gna` stating `Base.Field` is `!*T`, `type Alias = Base` let `var a
  lib.Alias`, `a.Field = nil` and `lib.Alias{}` through (v1.13.0 reported
  none). Gon is built with Go 1.27, where `go/types` gives an alias its own
  type, and the external field lookup did not see through it. Now an alias
  finds the contracts of the type it denotes, on both the `.gna` and the `.gon`
  path. Found in the audit of the v1.14 work.

### Compatibility

- **Possibly breaking (R1).** An importer that contradicts an exported `!`
  contract of a main-module `.gon` package is reported now; a `.gna` you kept
  for such a package is ignored (GW005) and, if it stated other contracts, they
  no longer apply. Invariant I4 is amended accordingly (RFC §10). Code that
  uses an alias of a type with `!` fields is now checked like the type itself.
- A differential of v1.13.0 against this release over the three repositories'
  86 `.gon` files and the 526-file standard-library corpus, and `gon check
  ./...` on this repository, is reported in the RFC's §11.

## [1.13.0] — 2026-11-17

### Fixed

- **The operands of a `return` were not checked.** `_ = Box{P: nil}` was GN001,
  but `return Box{P: nil}` was silently accepted, and so were the other checks
  that every other statement form already had. Measured: of 17 syntactic
  positions (conditions, `switch` tags and cases, `defer`, `go`, channel sends,
  `range`, op-assignments, `var` declarations, …) only `return` was skipped; the
  only thing checked there was the v1.7 element contracts. This was an oversight
  of coverage, not a design exception: it was never documented, and the code
  comment that excused it described the v1.7 change, not a rule.
  Now every operand of a `return` is checked like any other expression:
  - **GN001** — `return Put(nil)` (a nil to a `!` parameter, plain functions,
    `.gna` functions and, since v1.11, methods), `return Box{P: nil}` (a nil in a
    `!` field), `return v.(!*T)`;
  - **GN002** — `return Box{}` and `return new(Box)` when `Box` has a required
    `!` field;
  - **GW001** — `return p == nil` / `p != nil` for a non-nil `p`;
  - the body of a function literal in the operand, and a `return` inside one.
  Element contracts (v1.7) are still reported exactly once.
- **A function literal is checked as a function, not scanned.** Found in the audit
  of the change above, and not limited to `return`: since v1.0 a closure's body
  was only scanned for calls and composite literals, in the scope and with the
  result contract of the **enclosing** function. Measured on v1.12.0:
  - a closure parameter that shadows an outer `!` parameter was taken for it (a
    **false GW001**): `func f(p !*T) { g := func(p *T) bool { return p == nil } }`;
  - a `return nil` in a closure with a `!` result was never reported
    (`g := func() !*T { return nil }`);
  - a closure's own `!` parameter was not a non-nil name (no GW001 on
    `q == nil`).
  Now a function literal gets a scope of its own with its parameters (so a
  parameter shadows as in Go), its `return` statements answer to **its own** result
  contract, the enclosing function's contract is restored afterwards, and its body
  is walked as statements (so a local `var p !*T = nil` inside a closure is
  GN001 too). This applies wherever a closure appears, not only in a `return`.
- `gon version` reports `1.13.0`.

### Compatibility

- **Possibly breaking, in the same sense as v1.8 / v1.9 / v1.11 (R1).** Code that
  violates a `!` contract inside a `return` operand, or inside the body of a
  function literal, was not reported and is now. One false positive goes away
  (the shadowed closure parameter above).
  The fix is the same as anywhere else: pass a non-nil value, or remove the `!`.
- Code with no `!` produces byte-identical output: a differential run of v1.12.0
  against this release over 582 files (the 56 `.gon` files of the three
  repositories and 526 standard-library files), text and `--json`, found **0
  differences**. That means none of the repositories' own `.gon` code violates a
  contract in a `return`; it does not measure what other annotated code would
  report.

## [1.12.0] — 2026-11-16

### Added

- **Directory form for `gon transpile` and `gon build`** — M9
  (`docs/rfc-transpile-dir.md`). `gon transpile <dir>` and `gon transpile
  ./...` check the directory's `.gon` files as **one package** (the v1.10
  machinery: contract diagnostics plus the Go type check of the whole package)
  and write `<stem>.go` next to each `.gon` **only if the package passes**.
  A package with an error diagnostic or a Go type error writes **nothing**, and
  an existing same-named `.go` is left as it was. Warnings are printed and do
  not block.
  - A `.gon` replaces the `.go` of the same base name; other `.go` files are
    untouched and no stale `.go` is deleted. `*_test.gon` is not transpiled in
    directory mode (tests stay `.go`).
  - With `./...`, packages are transpiled **after the packages of the same
    module that they import**, so a fresh tree with no `.go` at all works; a
    failing package does not stop the others, and only the passing ones are
    written (exit 1 if any failed).
  - `gon build <dir> [go build flags]` = the above, then `go build [flags]
    <dir>`; nothing is forced (no `-o`).
  - A directory with no non-test `.gon` file is exit 2. Mixing a file operand
    with directories is exit 2.
- No new syntax, no new diagnostic code, no `.gna` or protocol change.

### Changed

- **Formatting is done in process with `go/format`** instead of running the
  `gofmt` found on `PATH`. Before, a missing `gofmt`, or one too old to parse the
  source, made `transpile` silently write the **unformatted** source, so the same
  `.gon` could emit different bytes on different machines. The output is the same
  as `gofmt`'s (identical on all 23 non-test sources of this repository), and
  source that does not parse is now an error instead of being passed through.
- `gon version` reports `1.12.0`.

### Compatibility

- **Additive.** A directory operand was an error for `transpile` and `build`;
  the file form is unchanged (exit code, output and emitted `.go` identical on
  56 `.gon` files, v1.11.0 against this release). The only observable change to
  the file form is the formatter case above.

## [1.11.0] — 2026-11-16

### Added

- **Call-site contracts for source-declared methods** — M7
  (`docs/rfc-method-contracts.md`). A `!` written on a **method's** parameter or
  result is now enforced at its callers, like a plain function's. Until now
  only the method's own body was checked (`return nil` into a `!*T` result,
  `p` as a non-nil local); none of the ways to call it was.
  - **GN001** for a literal `nil` (or an ordinary interface value to a `!I`)
    passed to a `!` parameter; a `!` **result** is a non-nil source at the
    immediate use site (`g := b.Get(); g == nil` → **GW001**).
  - Matched through `go/types` **object identity**, never by name: a direct
    call, a value-receiver call, a method expression (`(*T).M(x, nil)`, with the
    receiver offset), a call **promoted through embedding**, a call **through an
    interface** (the interface's own `!` applies; an implementing method need
    not repeat it), and a generic receiver or a method with its own type
    parameters (through `Origin()`).
  - Scope is the files checked together: the file in file mode, **every file of
    the package** in package mode (v1.10).
- No new syntax, no new diagnostic code, `.gna` schema stays **1**.

### Compatibility

- **Possibly breaking, in the same sense as v1.8 / v1.9.** Code that writes `!`
  on a method parameter or result and then contradicts it at a call site was
  not reported before and is now (GN001 / GW001). The fix is the same as for a
  plain function: pass a non-nil value, or remove the `!`.
- Code with **no `!` on any method** produces byte-identical output: a
  differential run of v1.10.0 against this release over 582 files (the 56
  `.gon` files of the three repositories and 526 standard-library files), text
  and `--json`, found 0 differences. None of those files annotates a method, so
  the run shows no regression, not what the new diagnostics find in real
  annotated code (`docs/rfc-method-contracts.md` §14).
- **A method value carries no contract** (`f := b.Set; f(nil)` is not
  checked), and **receiver contracts** (`!` on the receiver) are not part of
  this release (`.gna` `receivers:` stays reserved). Both are documented
  limits, not diagnostics.
- A source declaration **without any `!`** is treated as absent, so a `.gna`
  `methods:` entry for a local type keeps working exactly as before; when the
  declaration does carry a `!`, it wins.

### Changed

- `gon version` reports `1.11.0`.

## [Unreleased]

### Fixed

- **`gon check <dir>` on a directory with no `.gon` file at all** said
  `no .gon files in <dir> (*_test.gon files are not checked in package mode)`,
  blaming test files that were not there. The parenthesis now appears only when
  the directory does hold `*_test.gon` files.

## [1.10.0] — 2026-11-15

### Added

- **Package-aware checking** — M8 (`docs/rfc-package-check.md`). `gon check`
  and `gon vet` accept a **directory** (`gon check ./internal/foo`) or a
  `./...` pattern and check it as one package: the directory's `.gon` files are
  type-checked together in their real location (via `go/packages`, with each
  file's clean source overlaid at its real path), declaration contracts
  (function parameters and results, struct fields) are shared across the files,
  and each diagnostic is reported against its own file. `--json` is still one
  Diagnostic Protocol v1 document; the schema is unchanged.
  - A `.gon` file **replaces the `.go` file of the same base name** (the one
    `gon transpile` emits next to it); other `.go` files of the directory are
    loaded as they are; `*_test.go` and `*_test.gon` files are excluded (a
    directory holding only `*_test.gon` files is exit 2); build constraints apply.
  - A Go type error in one file does not stop the other files from being
    checked: contract diagnostics are printed first, then the type errors, then
    the process exits 1. With `--json` the document still holds only Protocol v1
    diagnostics, the type errors go to stderr, and the exit code is 1 when the
    package does not type-check. (Single-file `--json` behaves the same; see Fixed below.)
  - `./...` skips `testdata`, `vendor` and names starting with `_` or `.`, as
    the `go` tool does. A directory without `.gon` files is exit 2.
- No new syntax, no new diagnostic codes, `.gna` schema stays **1**.

### Compatibility

- **Additive.** A **file** operand behaves as before (one exception, below: `--json` on a Go type error now exits 1); a directory
  operand used to be rejected (`input must be a .gon file`). Verified with a
  differential run of `main` against this release over every `.gon` file in the
  three repositories (file operand, text and `--json`) and a 540-file standard
  library corpus: identical output.
- A folder of independent programs (several files each declaring `main`) is
  *not* a package unless it is named as one; name its files individually.
- Contracts of **imported** packages are still read from `.gna`, for module-local
  packages too. Reading a module-local package's contracts straight from its
  `.gon` sources is a later RFC.

### Changed

- `gon version` reports `1.10.0`; the usage text mentions directory operands.

### Fixed

- **`gon check --json <file.gon>` exited 0 on a Go type error.** A file whose
  only problem was a Go type error produced `{"schemaVersion":1,"diagnostics":[]}`
  and exit 0, so a caller that checked only the exit status saw a file that does
  not compile as a success. It now exits 1, with `gon: Go type error: …` on
  stderr and an unchanged Protocol v1 document on stdout (a Go type error is not
  a Diagnostic). This matches Diagnostic Protocol v1 §7 as clarified in the same
  release: exit status and the `diagnostics` array are related but not
  equivalent. Human output is unchanged.

## [1.9.2] — 2026-11-14

### Fixed

- **A method could overwrite the contract of a same-named function or of a
  method on another type.** The tables behind call resolution (`funcParams`,
  `funcReturns`) were keyed by the bare declaration name, for plain functions
  and methods alike, and the `return` check inside each body read the same
  table. The declaration seen last won, which produced false positives on the
  plain function:

  ```go
  func Make() *T { return nil }          // GN001 "cannot return nil …" — wrong
  func (b *Box) Make() !*T { … }
  Put(nil)                               // GN001 on a plain func — wrong
  m := Make(); if m == nil {}            // GW001 on a plain func — wrong
  ```

  and silently lost a contract in the other order. Methods no longer enter the
  name-keyed tables (a bare identifier can only name a plain function), and a
  body's `return` contract is read from its own declaration. Present since the
  tables were introduced.

### Changed

- `gon version` reports `1.9.2`.

### Compatibility

- **Removes false positives** on a plain function whose name is shared with a
  method that carries a `!`.
- **Can newly report GN001 in a method body.** A method with a `!` result whose
  `return nil` was previously masked by a later same-named declaration is now
  checked. This is the documented contract being applied as written.
- Call-site contracts for source-declared methods are still **not** tracked
  (a separate, open design question); this release only stops methods from
  corrupting the function tables.

## [1.9.1] — 2026-11-14

### Fixed

- **Official release binaries could not check a file outside a module.** On a
  file whose folder has no `go.mod` above it, `gon check` failed with
  `gon: Go type error: could not import fmt … ($GOROOT not set)`. Release
  binaries are built with `-trimpath`, which leaves `go/build`'s GOROOT empty;
  the standalone importer used for such files then had no standard library.
  Gon now fills GOROOT at start-up, from the `GOROOT` environment variable if
  set, otherwise from `go env GOROOT`. Present in every release binary
  (verified on v1.7.0, v1.8.1 and v1.9.0). Files inside a module, and binaries
  built without `-trimpath`, were not affected. A regression test builds a real
  `-trimpath` binary and runs it on such a file with `GOROOT` unset.

### Changed

- `gon version` reports `1.9.1`.

### Compatibility

- No change to diagnostics. Only a path that previously failed outright now
  works. It still needs the `go` command on `PATH` when `GOROOT` is not set,
  as module files already did.

## [1.9.0] — 2026-11-14

### Added

- **Generic `.gna` and external generic contract application** — M5b
  (`docs/rfc-generic-gna.md`). No new `.gna` syntax; schema stays **1**.
  - **Explicit instantiation resolves like inference.** `pkg.F[T](…)`,
    `pkg.Box[T]{…}`, `new(pkg.Box[T])` and a method on an instantiated
    receiver now apply the same `.gna` entry as the inferred form. Before,
    `slices.Clone(nil)` was reported and `slices.Clone[[]int](nil)` was not.
  - **Every `!` flag is validated against the symbol's origin signature**
    (`go/types`, before instantiation): valid only on a nilable kind
    (pointer, slice, map, channel, function, interface, `unsafe.Pointer`) or
    a type parameter whose constraint restricts it to nilable kinds. An
    invalid flag is **GN003** and **only that flag is dropped** — the
    position becomes ordinary and the entry's other positions keep their
    contract. Applies to `params`, `results` and `types:` fields, to generic
    and non-generic symbols alike, and to type parameters a method declares
    itself (Go 1.27). Arity mismatch still drops the whole entry.
  - **`!` inside a `.gna` type-argument list** (`"Box[!*T]"`,
    `"Box[[]!*T]"`) is a load error — the `.gna` mirror of the v1.8 source
    rule.
- No new diagnostic codes.

### Fixed

- **`import "io"` produced three false GW004 warnings** (`Reader.Read`,
  `Writer.Write`, `WriterTo.WriteTo` "which the package does not provide").
  Annotated interface methods were looked up in the method set of a pointer
  to the interface, which is empty. Present since v1.5; found by the
  bundled-annotation audit added in this release.

### Changed

- `gon version` reports `1.9.0`.
- The bundled `extlib` reference annotation claimed `"!string"` for
  `Take(s string)`; `Take` now takes `*string` and the claim is `"!*string"`.
  About ten test fixtures that used `"!string"` as a placeholder were
  rewritten the same way.

### Compatibility

- **Possibly breaking, by design (RFC X9), shipped solo per R1:**
  - A `.gna` `!` over a non-nilable kind (`"!string"` on a `string`
    parameter) or a non-nilable type parameter used to be accepted silently
    and is now GN003 with that flag dropped. This is the rule
    `gna-spec-v1.md` has stated since v1.6. **Migration:** correct the type,
    or delete the `!`.
  - A call or literal through an *explicit* instantiation of an external
    generic that violates a `.gna` contract is newly reported (the inferred
    form already was).
  - `"Box[!*T]"` and `"Box[[]!*T]"` in a `.gna` string are now load errors.
- **Unchanged:** `.gna` schema, source-level behaviour, diagnostics for code
  that imports no annotated generic symbol. Differential run of v1.8.1 vs
  v1.9.0 over 46 existing `.gon` files: identical output.

## [1.8.1] — 2026-11-14

### Changed

- **Build minimum raised to Go 1.27.0** (`go.mod`, CI, release workflow).
  Gon parses `.gon` with the `go/parser` of the toolchain that builds it;
  Go 1.27 accepts methods with their own type parameters
  (`func (b Box[X]) Map[Y any](…)`), earlier toolchains reject them at parse
  time. With one build minimum every Gon binary now behaves the same.
- `gon version` reports `1.8.1`.

### Compatibility

- **No change to diagnostics or to accepted/rejected programs**, apart from
  source that uses generic methods, which was previously a parse error.
- **Building from source needs Go 1.27.0+.** `go install …@v1.8.1` works from
  any Go that can auto-download toolchains (Go 1.21+, `GOTOOLCHAIN` not set to
  `local`); release binaries need no Go at all.
- The Go installed on a *user's* machine is not constrained: with the user's
  `go` forced to 1.21.13, 1.22.12, 1.23.12, 1.24.7, 1.25.0, 1.26.0 and 1.27.0
  the same file produced identical diagnostics. (Observed, not a support
  guarantee; CI does not yet run this matrix.)
- Generic-method contracts: body-level checks work (G2 on method type
  parameters, GN001 on returns); call-site contracts for methods remain
  untracked for all receivers — see the roadmap note.

## [1.8.0] — 2026-11-14

### Added

- **Generic Semantics** — M3 (`docs/rfc-generic-semantics.md`). Contracts are
  attached to a *declaration position*, never to a type argument:
  instantiation substitutes types and neither adds, removes, nor carries a
  `!`.
  - **`!X` on a type parameter** is valid only when `X` is a *nilable type
    parameter* — some element of its constraint is a non-empty union whose
    every term has a nilable underlying kind (`~*T`, `~[]E`, `*A | *B`,
    `interface{ ~int | ~*T; ~*T }`). `X any`, `comparable`, method-only and
    non-nilable constraints are **GN003**. `var v !P` with no initializer
    (nilable `P`) is **GN002**.
  - **Field contracts survive instantiation**: `Box[T]{}`, `Box[T]{v: nil}`,
    `var a Box[T]`, `Box[Box[T]]{}` are checked against `Box`'s declaration
    (GN002 / GN001 / GW001).
  - **Call contracts resolve through explicit instantiation**:
    `NN[T](…)` now applies `NN`'s parameter and result contracts, like the
    already-working inferred form `Must(x)`.
  - **`!` inside a type argument** (`Box[!*T]`, `F[!*T]()`) and **inside a
    constraint** (`~!*T`, `interface{ ~[]!*T }`) are **GN003** (reserved).
- No new diagnostic codes; `.gna` schema stays **1**.

### Fixed

- **Diagnostic order was nondeterministic.** Missing-required-field GN002s
  were emitted by iterating a Go map, so the order varied run to run
  (observed since v1.7.1: 7 distinct outputs in 12 runs of one file). Local
  struct fields are now reported in declaration order; `.gna` `types:` fields
  in alphabetical order. Diagnostic *content* is unchanged.
- `!` on an element type written in a composite-literal *expression*
  (`_ = []!int{}`) was accepted; it is now GN003 like the declaration form.

### Changed

- `gon version` reports `1.8.0`.
- The preprocessor strips `!` after `~`, after `[` before a type-only token
  following an identifier, and after `,` inside `[…]`, so the reserved
  positions above surface as GN003 rather than a Go parse error. `m[!ok]`
  indexing is unchanged.

### Compatibility

- **Possibly breaking, by design (RFC G11).** Rejected where v1.7 accepted:
  `!X` for a non-nilable type parameter `X`; `!` in a type argument or
  constraint. Newly reported on code that already violated a documented
  contract: zero/nil construction of an instantiated generic struct, a nil
  argument to an explicitly instantiated `!`-parameter.
- Unchanged: instantiations of **external** generics (`pkg.F[T]`) — generic
  `.gna` application remains M5b (v1.9). Source-declared **method** contracts
  remain untracked for generic and non-generic receivers alike; generic
  interfaces follow the non-generic rules (v1.4 D5). See RFC §13.1.

## [1.7.1] — 2026-11-13

### Fixed

- **Untyped local `var x = <expr>` initializers were never checked.** In the
  no-explicit-type branch of a local `var` declaration the initializer
  expressions were not walked, so composite-literal contracts (element `!`
  GN001 / GN002, and struct field contracts) were silently skipped for
  `var arr = [3]!*T{2: p}` while `arr := [3]!*T{2: p}` and package-level
  `var` were reported. Initializers are now checked like every other
  declaration form.

### Changed

- `gon version` reports `1.7.1`.

### Compatibility

- Strictly a bug fix: it can surface GN001 / GN002 on code that violated an
  already-documented contract but was previously missed.

## [1.7.0] — 2026-09-07

### Added

- **Element-Contract Construction** — M2b
  (`docs/rfc-element-contract-construction.md`). `!` in **element / value
  position** is now activated: `[]!*T`, `[N]!*T`, `map[K]!*V`, `[]!I` — "a
  slice/array/map whose elements (or map values) are non-nil". It is a
  **construction-time** obligation, not a lifetime invariant of the
  container: after the value exists, `s[i] = nil`, `append`, and map
  assignment are ordinary Go and are not tracked.
  - **Explicit `nil` written as an element / map value under an element `!`
    is GN001** — `[]!*T{nil}`, `map[string]!*T{"k": nil}`. Ordinary
    expressions (including calls that may return nil) are accepted,
    conservatively, exactly as for outer `!` (Type Coverage C4).
  - **A fixed-array literal that leaves a `!` nilable element index at its
    zero value is GN002** — one diagnostic per literal. Coverage is by
    index: `[3]!*T{x}`, `[3]!*T{2: x}`, and `[3]!*T{0: x, 2: x}` are each
    GN002; `[...]!*T{x}` (inferred length) never is.
  - **Malformed element `!` is GN003** — on a channel element (`chan !*T`),
    on a map **key** (`map[!*K]V`, deferred), or on a non-nilable element
    kind (`[]!int`).
  - The outermost-binding rule is unchanged and compositional: `![]!*T`
    constrains both the slice and its elements; a leading `!` still binds
    only the outermost constructor.
- **`.gna` type strings** accept element `!` (`"[]!*T"`,
  `"map[string]!*Service"`, `"[8]!*Slot"`). The recorded flag still
  reflects only the outermost `!`. An `!` on a channel element remains a
  load error.

### Changed

- `gon version` reports `1.7.0`.
- `gon fmt` preserves element `!` in place across the format round-trip
  (no change needed — re-insertion is offset-driven).

### Compatibility

- **Non-breaking.** Every new diagnostic fires only on syntax that v1.6
  rejected outright (element `!` was GN003 / a parse error / a `.gna` load
  error). Programs valid under v1.6 remain valid.
- `.gna` semantic schema remains **1** (I7) — only the acceptance of a
  previously-reserved string form changes.
- Older `.gna` tooling built against v1.6 may still reject the newly legal
  element forms; consumers that re-validate `.gna` should update.

## [1.6.0] — 2026-09-06

### Added

- **Type Coverage** — M1b (`docs/rfc-type-coverage.md`). `!` now has a
  single, specified meaning across every nilable Go kind: it constrains the
  **reference value** (slice header, map, channel, function value, pointer)
  to be non-nil, and says nothing about length, contents, emptiness, or
  channel state — the same boundary `!I` draws for interfaces (v1.4).
  - **`var x !S` with no initializer is now GN002** for a nilable reference
    kind (`![]T`, `!map[K]V`, `!chan T`, `!func(…)`, `!*T`, and named types
    / aliases whose underlying kind is one of those). This closes a
    pre-existing gap in which `var p !*T` was silently accepted. Interface
    types are excluded (owned by M1a). An explicit non-nil initializer
    (`make(...)`, a composite literal, a function literal/name) is accepted;
    `= nil` is GN001.
  - **`!` on a non-nilable type is now GN003.** Writing `!` on a struct,
    array, or basic type — directly or through a named type / alias
    (`type Point struct{…}` → `!Point`) — is a malformed contract.
  - **`x.(!T)` for any assertion target is now GN001.** v1.4 locked this for
    interface targets and deferred concrete ones; v1.6 adopts the amendment
    so `x.(!*T)` / `x.(![]T)` are rejected too. Assertions never establish a
    non-nil guarantee.
  - Specified and regression-tested (previously accidental) behaviour: a
    literal `nil` into any `!` reference position is GN001; a `![]T` /
    `!map` / `!chan` / `!func` **field** left at zero is GN002, `f = nil` is
    GN001, and the selector is a non-nil source; `append`, reslice,
    conversion, and assertion results are ordinary and never carry `!`; the
    zero-value containment walk still stops at every slice/map/chan/func
    boundary.

### Changed

- `gon version` reports `1.6.0`.
- **`.gna` type strings:** a leading `!` binds only the outermost type
  constructor. An `!` in element position (`"[]!*T"`, `"map[string]!V"`) is
  now rejected at load time (reserved for M2b) instead of being silently
  ignored.

### Compatibility

- `.gna` schema remains **1**. `"![]T"`, `"!map[K]V"`, `"!func(…)"` were
  always legal strings; their semantics are now defined.
- New rejections vs v1.5: bare `var x !S` (nilable kind, no initializer) →
  GN002; `!` on a non-nilable type → GN003; `x.(!T)` concrete target →
  GN001; element-position `!` in a `.gna` string → load error. Code that
  does not annotate a reference-value type with `!` is unchanged.
- Solo release per roadmap R1; M2b (element-contract construction) remains
  v1.7.

## [1.5.1] — 2026-09-06

### Fixed

- **`gon check` / `gon vet` — spurious "Go type error: could not import" for
  module-local imports.** The post-check Go re-validation used the
  GOPATH/GOROOT importer, which cannot resolve a package that only exists in
  the enclosing module. A `.gon` file that imported a sibling package in its
  own module (the normal shape for v1.5 external `.gna` contracts) always
  failed with `could not import <pkg>` and exit 1, even when the code was
  well-typed. Re-validation is now module-aware: when the file lives in a
  module it is type-checked against the real module graph (matching the
  checker's own resolution); standalone single-file inputs keep using the
  GOPATH/GOROOT importer.

### Changed

- `gon version` reports `1.5.1`.

### Compatibility

- No semantic change. Checker behaviour, diagnostic codes, severities, and
  Diagnostic Protocol v1 are unchanged from v1.5.0. `.gna` schema remains **1**.

## [1.5.0] — 2026-09-06

### Added

- **Ecosystem Contract Expansion** — M4a + M4b + M5a
  (`docs/rfc-ecosystem-contract-expansion.md`). External `.gna` `types:`
  field contracts are now applied across the package boundary at the same
  three sites as local field contracts.
  - **M4a — non-interface application.**
    - A keyed `pkg.T{…}` literal that writes an explicit `nil` for a `!`
      field is **GN001** (previously silent; GN002 for a *missing* `!` field
      was already emitted).
    - `new(pkg.T)`, `&pkg.T{}`, and `pkg.T{}` are zero-value construction
      sites: a `!` field left at zero is **GN002**, including a `!` field
      **promoted from an embedded external type** (one hop).
    - A selector `x.F` where `x` has external type `pkg.T` and `.gna` marks
      `T.F` as `!` is a **non-nil source**: `x.F == nil` → **GW001**; `x.F`
      into a `!U` target is accepted. The non-nil-source recogniser now also
      accepts `!`-field selectors for local types (previously a gap).
    - `x.F = nil` into an external `!` field is **GN001**.
    - Unkeyed `pkg.T{a, b}` stays behind the firewall — no diagnostic in
      either direction (Gon does not reconstruct external field order).
    - External `types:` resolution is **exact** (import path + type + field);
      the local name-only field heuristic is never used across the boundary.
  - **M4b — interface-typed application.** An external contract position
    (param, result, or `types:` field) whose Go type is an interface carries
    `docs/rfc-interface-semantics.md` semantics: `!I` = interface value
    non-nil; an ordinary interface-typed value into that position is
    **GN001** (D3b); a concrete/typed-nil operand is accepted (D3a).
  - **M5a — `.gna` validation against the real package.**
    - **GN003** — the `.gna` `params`/`results` count for a resolved
      function or method must equal the Go signature's (a variadic final
      parameter counts as one). On mismatch the entry is **dropped** (its
      `!` flags no longer apply) so a stale annotation cannot shift a claim
      onto the wrong argument.
    - **GW004 (new code, warning)** — a `.gna` key under `functions:`,
      `methods:`, or `types:` that names a symbol the package does not
      provide. `methods:` validation uses the full `go/types` method set, so
      methods promoted from an embedded field are accepted.
    - Validation is best-effort (only for packages `go/types` resolved) and
      reports each condition **once per annotated symbol per run**.

### Changed

- `gon version` reports `1.5.0`.

### Compatibility

- `.gna` schema remains **1** — no file-format change.
- One new diagnostic code: **GW004** (warning; contributes exit 0).
- **Possibly breaking** in three narrow ways, each surfacing a
  previously-silent defect: a `.gna` with the wrong arity now emits GN003
  (was: silently shifted contract); a `.gna` naming a missing symbol now
  emits GW004; code that writes explicit `nil` into — or relies on flow
  reasoning about — an external `!` field now emits GN001/GW001.
- Code importing only unannotated packages, or correct `.gna` files used
  correctly, is unaffected.
- Still no type coverage, no generics, no flow-sensitive nilability, no
  dynamic-value tracking. `receivers:` remains parsed and reserved (no
  call-site effect). `--strict` is deferred to a follow-on.

## [1.4.1] — 2026-09-06

### Fixed

- **`gon fmt` mis-placed `!` on re-insertion.** The formatter stripped `!`,
  ran `go/format`, then re-inserted `!` before the *Nth textual occurrence*
  of each type name. When one type appeared as both `!T` and `T` in the same
  file (common with v1.4 interface contracts — an ordinary `I` value flowing
  into `!I`), every `!` landed on the wrong occurrence: a silent semantic
  corruption of the source.
- Re-insertion now walks the clean and formatted **token streams** in
  lockstep (comments and semicolons dropped; a `go/format` trailing-comma
  change is resynced), so the Nth marked type token maps to its exact
  counterpart regardless of textual repetition. If the streams cannot be
  aligned, `gon fmt` returns an error and leaves the file untouched instead
  of writing a corrupted result.

### Changed

- `gon version` reports `1.4.1`.

### Compatibility

- No semantic change. Diagnostic codes, severities, checker behaviour, and
  Diagnostic Protocol v1 are all unchanged from v1.4.0.
- `.gna` schema remains **1**.
- A file that `go/format` leaves byte-identical now round-trips through
  `gon fmt` unchanged.

## [1.4.0] — 2026-09-05

### Added

- **M1a — Interface non-nil contracts** (`docs/rfc-interface-semantics.md`).
  `!I` means the *interface value* is non-nil; it never constrains the
  interface's dynamic value.
  - **D3a** — a concrete/typed value assignable to `I` satisfies `!I`, even
    when it is itself typed-nil.
  - **D3b (new rejection)** — an ordinary interface-typed value cannot
    satisfy `!I` (GN001), unconditionally. An explicit `!= nil` check does
    not narrow it. Enforced at `var` init, plain assignment, `return`, and
    `!I` call arguments.
  - **D3c** — `nil` literal → `!I` is GN001 (unchanged path).
  - **D3d / D4** — an existing `!I` binding, or a result declared `!I`,
    satisfies `!I` and is a non-nil source at the call site — but only for
    the **same** interface type (`types.Identical`). `!ReadCloser` is not a
    `!Reader` source and vice versa: `!` does not propagate across an
    embedded/embedding interface (§3.7). Enforced with the resolved target
    type at all four sites: `var` init, plain assignment, `return`, and
    `!I` call arguments.
  - **D6a** — `x.(!I)` (interface assertion target) is rejected with GN001
    instead of a parse error; the preprocessor now recognises the `!` in a
    type-assertion target. `x.(!*T)` (concrete) is left for a later RFC.
  - **D6c** — an explicit conversion `I(x)` has interface static type and
    cannot satisfy `!I`; the checker does not look through the conversion to
    the concrete operand.

### Changed

- `gon version` reports `1.4.0`.

### Compatibility

- No `.gna` schema bump (schema remains **1**). `!` on interface types was
  already valid syntax; this release only defines its semantics.
- Existing GN001/GN002/GW001/GW002/GW003 codes and severities preserved.
- The only new rejection is ordinary `I` → `!I` (and `nil`/`x.(!I)` in the
  same position). Code that does not put a `!` on an interface type is
  unchanged.
- Still no type coverage, no generics, no flow-sensitive nilability, no
  dynamic-value tracking.

## [1.3.0] — 2026-08-19

### Added

- **M2a construction completeness**
  - `new(T)` is a zero-value construction site when `T` is a local struct (GN002).
  - Unkeyed composite literals use declaration order from the *local* struct AST only.
  - External (`SelectorExpr`) types remain keyed-only + `.gna` (M4 firewall).
  - Unified construction entry points in `internal/checker/construction.go`.
- **ContractTrace** — first-class diagnostic data on GN001/GN002 (wire `String()` unchanged).
- **GW003** — redundant type-assertion warning on declared `!T` identifiers only.
- **`gon fmt`** — format Gon source in place.
- **`gon lsp`** — minimal stdio LSP.

### Changed

- `gon check` remains canonical; `vet` stays a compatibility alias.
- `gon version` reports `1.3.0`.

### Compatibility

- No `.gna` schema bump (schema remains **1**).
- Existing GN001/GN002/GW001 codes and severities preserved.
- Scope firewall: no interface `!I`, no type coverage, no generics, no flow-sensitive nilability.

## [1.2.1] — 2026-08-18

### Fixed

- **Preprocessor: `!` in multi-result function signatures.** Local forms such
  as `func Open() (!*string, error)`, `func Open() (*string, !error)`, and
  `func F(a !*T) (!*U, error)` are recognized as type modifiers. Parameter
  and parenthesized result lists (including interface methods) are tracked so
  unary `!` in expressions (`f(a, !b)`, `(!flag)`) is not stripped.

### Compatibility

- No checker semantics change relative to v1.2.0.
- Field contracts and return-value contracts unchanged.
- `.gna` schema remains **1**.

> Gon v1.2.1 is a preprocessor bugfix so valid local multi-return `!`
> syntax works as intended since v1.1.

### Changed

- `gon version` reports `1.2.1`.

## [1.2.0] — 2026-08-17

### Added

- **Field contracts.** A `!T` field is an invariant of its declared storage
  location. Enforced at three local sites only:
  - **Construction (GN002):** zero-value containment walk over struct,
    embedded, fixed array, and anonymous types; stops at every indirection
    boundary (`*T`, `[]T`, `map`, `chan`, `interface`, `func`).
  - **Mutation (GN001):** explicit `nil` assigned into a `!T` field
    (`s.Client = nil`). Ordinary values remain accepted (conservative).
  - **Selector (non-nil source):** selecting a `!T` field yields a non-nil
    source for immediate use (GW001 on nil comparison).
- **`.gna` `types:` section.** External packages may declare field contracts:
  ```yaml
  types:
    Config:
      fields:
        Client: "!*http.Client"
  ```
- Spec: [docs/rfc-field-contracts.md](docs/rfc-field-contracts.md).

### Compatibility

- `.gna` schema remains **1** (no format change; `types:` was already
  syntactically open under schema 1).
- Programs accepted by v1.1 remain accepted unless they construct or mutate
  a value that violates a newly annotated `!` field.
- No flow analysis, no path sensitivity, no reconstruction of object state.
- Ordinary value into `!T` field remains accepted.

> Gon v1.2 makes `!T` fields storage invariants checked at construction,
> mutation, and selector use — still local and non-flow-sensitive.

### Changed

- `gon version` reports `1.2.0`.
- Scope document updated for v1.2 semantics.

## [1.1.0] — 2026-08-17

### Added

- **Return-value contracts.** An explicitly annotated `!T` result position
  (in `.gna` or a local Gon signature) becomes a non-nil source at its
  immediate use site (`:=` or `var` without an explicit type). Observable via
  GW001 on nil comparison.
- Spec: [docs/rfc-return-value-contracts.md](docs/rfc-return-value-contracts.md).

### Compatibility

- `.gna` schema remains **1** (no format change).
- Ordinary (unannotated) values assigned to `!T` remain accepted.
- No flow analysis, no conditional contracts, no propagation through
  conversion / type assertion / assignment from names.
- Explicit type on a binding always wins over a return contract.

> Gon v1.1 adds explicit return-value contracts without introducing flow
> analysis. Annotated `!T` return positions become non-nil sources at their
> immediate use sites; existing ordinary-value acceptance remains unchanged.

### Changed

- `gon version` reports `1.1.0`.
- Scope document updated for v1.1 semantics.

## [1.0.0] — 2026-08-16

Initial public release.

- Local non-flow-sensitive nilability checks (`!T`).
- External packages via `.gna` + module-aware `go/packages`.
- Conservative reference annotations for `io` and `os`.
