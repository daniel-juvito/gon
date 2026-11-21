# Conformance fixtures: explicit non-nil conversion, Part A

These fixtures define the behaviour of Part A of
`docs/rfc-explicit-nonnil-conversion.md` before it is implemented, as the plan
`docs/plan-explicit-nonnil-conversion-part-a.md` requires (section 2 and 4).
They are read by `internal/conformance`.

- `manifest.json` lists every entry: its RFC clause, its kind (`preproc`, `fmt`,
  `check`, `tokenclass`), its status, the checker site and `diagnostics`, the set of codes that the fixture expects (a test
  compares it with the codes of the `want` part; it is empty for the other kinds).
- `syntax.txtar`, `fmt.txtar`, `flows.txtar` hold the fixtures as `ID/src` and
  `ID/want`. In a `preproc` want, `‹!›` stands immediately before a token that
  carries a non-nil `!`; it is written by hand and is the oracle for the offsets. In
  a `check` want, each line is `LINE CODE`; an empty want means no diagnostic. A
  `check` source must be valid Go once its `!` are stripped.
- The token-class table is in `internal/conformance/tokenclass_test.go`.
- `pending-allowlist.txt` lists the entries whose status is `pending-PR…`. A test
  keeps it equal to the manifest, and a second test requires every pending entry
  to fail on the current code by a result that differs, so a fixture cannot rot as
  pending. A fixture that fails because its clean source is not valid Go (it uses
  syntax that the code does not know yet) is accepted only if it is valid Go once its
  `!` are stripped, without the preprocessor, so a typo in a pending fixture is caught
  now; any other runner error is a failure. A PR that
  implements a behaviour moves its entries to `enabled` in the manifest and removes
  them from the allowlist; the last PR leaves the allowlist empty.

The fixtures use the extension `.txtar`, not `.gon`, so that the production format
check of `.gon` files and the transpile gate of the pinned gon never read them.

Not covered by this suite yet, and why: the Go type error of a conversion that Go
rejects (it needs the CLI, because the checker treats a Go type error as non-fatal),
slots declared by `.gna` (they wait for the measurement of section 5 of the plan),
cross-package module-local slots, the JSON output and exit code of GW006 (the
protocol conformance suite of `cmd/gon`), and the formatter cases with comments.
