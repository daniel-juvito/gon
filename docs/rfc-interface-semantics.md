# RFC: Interface Semantics for Non-Nil Contracts

**Status:** Draft  
**Target:** Gon v1.4  
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`, `docs/roadmap-v1.3-plus.md`, `docs/rfc-return-value-contracts.md`, `docs/rfc-field-contracts.md`  
**Date:** 2026-08-18

## 1. Motivation

Gon adds explicit non-nil contracts to Go types through the `!` modifier. For concrete types, the meaning of such a contract is direct: `!T` states that the value represented by `T` is non-nil. Field contracts and return-value contracts build on this same model by allowing the checker to recognize explicitly declared non-nil values as sources for subsequent use.

Interfaces introduce a distinct semantic boundary.

An interface value has two relevant dimensions: the interface value itself, and the dynamic value represented through its dynamic type and dynamic value. These dimensions do not have identical nil semantics. In particular, a typed-nil concrete value can be stored in a non-nil interface value.

For example:

```go
var p *File = nil
var r io.Reader = p
```

Here, `r` is a non-nil interface value whose dynamic type is `*File` and whose dynamic value is nil. Consequently, the phrase "non-nil interface" is insufficiently precise for a static contract unless it specifies which level of value is being constrained.

Gon therefore needs an explicit definition for the meaning of `!` when applied to an interface type.

The existing design principles provide the required boundary. Gon contracts are explicit and non-flow-sensitive. The checker does not perform general flow-sensitive narrowing, and interface contracts do not introduce runtime checks or dynamic-value tracking. Within that model, an interface non-nil contract must describe a property of the interface value itself rather than attempt to establish a separate contract over the dynamic value.

This RFC consequently defines:

```
!I
```

as a guarantee that the interface value is non-nil.

It does not guarantee that the dynamic value is non-nil. A typed-nil dynamic value therefore does not violate a `!I` contract.

This distinction is deliberate. It preserves the existing Gon boundary between explicit nilability contracts and value-flow analysis while retaining ordinary Go rules for interface identity, assignability, method sets, and embedding.

The same boundary also determines how `!I` interacts with construction, assignment, return-value contracts, assertions, and conversions. A concrete value assignable to `I` can satisfy `!I`, because the resulting interface value is non-nil even when the concrete value is typed-nil. An ordinary `I` value, however, cannot be promoted to `!I`, because Gon does not use flow-sensitive reasoning to manufacture a new explicit contract. Likewise, assertions and conversions do not provide implicit propagation mechanisms for `!I`.

The purpose of this RFC is therefore not to introduce a new model of Go interfaces. It is to define precisely where the existing Gon non-nil contract model meets Go's interface semantics, while keeping the `!` modifier orthogonal to Go type identity and interface method-set semantics.

## 2. Decision Matrix

The following decisions are locked. The normative rules in §3 MUST NOT introduce semantics beyond these decisions.

### Terminology

This RFC uses two terms with distinct meanings:

- **Interface value** — the interface value itself. An interface value is either "nil" or a non-nil interface containing a dynamic type and dynamic value.
- **Dynamic value** — the concrete value stored by a non-nil interface value. The dynamic value may itself be a typed-nil value.

The term non-nil MUST be qualified when referring to interfaces. `!I` guarantees that the interface value is non-nil; it does not guarantee that the dynamic value is non-nil.

### D1 — Meaning of `!I`

| Option | Decision |
|--------|----------|
| `!I` means the interface value is non-nil. | **LOCKED** |
| `!I` also guarantees that the dynamic value is not typed-nil. | Rejected |

`!I` is therefore an interface-nilness contract, not a dynamic-value-nilness contract.

### D2 — Typed-nil Interaction

D2 follows directly from D1.

A typed-nil concrete value assigned to an interface produces a non-nil interface value. Therefore, the resulting interface value satisfies `!I`.

The checker MUST NOT interpret a typed-nil dynamic value as a violation of `!I`.

### D3 — Construction and Assignment

**D3a — Concrete/typed value → `!I`**

A concrete or typed value that is assignable to interface `I` MAY be assigned to `!I`.

The nilness of the concrete value does not affect satisfaction of the `!I` contract.

**D3b — Ordinary `I` → `!I`**

An ordinary interface value of type `I` MUST NOT be assigned to `!I`.

This rejection is unconditional. The checker MUST NOT permit the assignment merely because it can establish non-nilness through local reasoning.

In particular, flow-sensitive narrowing MUST NOT make such an assignment valid.

**D3c — `nil` literal → `!I`**

Assigning the `nil` literal to `!I` is invalid and produces GN001.

**D3d — `!I` → `!I`**

An existing `!I` value MAY be assigned to another `!I` value of the same interface type.

### D4 — Method and Return Contracts

A function or method result declared as `!I` guarantees that the returned interface value is non-nil.

A caller receiving that result MAY use it as a non-nil source under the existing return-value contract rules.

The contract does not guarantee that the result's dynamic value is non-nil.

### D5 — Assignability and Method-Set Invariance

I3/I4 summary — not a verbatim quotation: The existing interface invariants can be summarized as follows:

- **I3 — Method-set invariance:** `!I` does not alter the Go method set or interface implementation relation of `I`.
- **I4 — Assignability invariance:** `!I` does not introduce a new Go type or alter Go's assignability or type-identity rules; `!` expresses a Gon nilability contract over the existing Go type.

`!I` therefore does not create a distinct Go type.

Go type identity, assignability, method sets, interface implementation, and interface embedding remain governed by Go semantics.

The `!` modifier adds a Gon nilability contract; it does not alter the underlying Go type.

### D6 — Assertion and Conversion Boundary

**D6a — Type Assertion Target**

A type assertion MUST NOT use `!I` as its asserted type.

A construct such as:

```go
x.(!I)
```

is not a Gon mechanism for obtaining a `!I` value.

**D6b — Explicit `!I(x)` Conversion**

`!I(x)` is not an explicit conversion mechanism.

The `!` modifier does not turn a Go interface conversion into a non-nil assertion or runtime check.

**D6c — Ordinary Interface Conversion**

An ordinary conversion to `I` produces an ordinary `I` value, not a `!I` value.

For example, converting a concrete value to `I` does not by itself constitute a separate propagation mechanism for `!`.

**D6d — No Assertion/Conversion Promotion**

Neither type assertions nor conversions MAY implicitly promote their result to `!I`.

The checker MUST NOT infer a `!I` contract from an assertion or conversion merely because the resulting interface value can be shown to be non-nil.

### D7 — `.gna` Representation

The `.gna` representation of an interface non-nil contract uses the same `Type.NonNil` representation used for other Gon non-nil contracts.

This is a deliberate representational uniformity, not a claim of semantic equivalence.

For concrete or pointer types, `!T` expresses that the value represented by `T` is non-nil. For an interface type, `!I` expresses only that the interface value is non-nil. It does not extend the contract to the interface's dynamic value.

Thus, the syntax and AST representation of `!T` and `!I` are intentionally identical while their semantic guarantees differ according to the kind of Go value being contracted.

### D8 — Breaking-Change Matrix

Existing ordinary interface types retain their existing Go semantics.

The introduction of `!I`:

- does not change the meaning of existing `I` declarations;
- does not change Go assignability or method-set rules;
- does not introduce runtime checks;
- does not reinterpret existing interface values as dynamically non-nil;
- only adds an explicit non-nil interface-value contract where `!I` is written.

Consequently, existing Gon code that does not use interface non-nil contracts remains semantically unchanged.

### D9 — Non-Goals

Interface non-nil contracts do not introduce:

1. runtime typed-nil checks;
2. flow-sensitive interface narrowing;
3. tracking of dynamic-value nilness;
4. propagation of non-nilness through arbitrary interface values;
5. assertion-based creation of `!I`;
6. conversion-based creation of `!I`;
7. inference that a non-nil interface value implies a non-nil dynamic value.

These exclusions are intentional. Gon tracks the nilness contract of the value represented by the declared Go type; it does not introduce a separate data-flow system for tracking the nilness of values hidden inside interfaces.

## 3. Normative Rules

### 3.1 Interface Contract Semantics

For an interface type `I`, the declaration `!I` means:

> The interface value is guaranteed to be non-nil.

It does not mean:

> The dynamic value contained by the interface is guaranteed to be non-nil.

This distinction is normative.

For example:

```go
var p *File = nil
var r !io.Reader = p
```

is valid provided `*File` implements `io.Reader`.

The resulting interface value `r` is non-nil even though its dynamic value is the typed-nil `*File`.

### 3.2 Concrete Values Assigned to `!I`

A concrete or typed value MAY be assigned to `!I` whenever its type is assignable to `I` under Go's ordinary assignability rules.

The checker MUST NOT require an additional proof that the concrete value itself is non-nil.

For example:

```go
var p *File = nil
var r !io.Reader = p
```

is valid.

The relevant property is the resulting interface value, not the nilness of `p` as a pointer value.

### 3.3 Ordinary Interface Values Assigned to `!I`

An ordinary `I` value MUST NOT be assigned to `!I`.

For example:

```go
var r io.Reader = someReader()
var x !io.Reader = r // GN001
```

The rejection remains valid even when the source is preceded by a nil check:

```go
var r io.Reader = someReader()

if r != nil {
    var x !io.Reader = r // GN001
}
```

The condition does not narrow `r` from `I` to `!I`.

This is not an implementation limitation. Flow-sensitive narrowing is outside the semantics of `!I`.

### 3.4 Nil Literal

The `nil` literal does not satisfy any `!I` contract.

For example:

```go
var r !io.Reader = nil // GN001
```

### 3.5 Existing Non-Nil Interface Contracts

An existing `!I` value satisfies another compatible `!I` contract directly:

```go
func Open() !io.Reader {
    // ...
}

var r !io.Reader = Open()
var x !io.Reader = r
```

No additional proof is required.

### 3.6 Return and Method Results

A result declared as `!I` establishes a non-nil interface-value contract at the call site.

For example:

```go
func Open() !io.Reader {
    // ...
}

r := Open()
```

The result of `Open()` is a non-nil interface value according to the return-value contract.

This contract does not imply that the dynamic value stored in `r` is non-nil.

### 3.7 Go Assignability and Method Sets

The `!` modifier does not change the underlying Go type.

Consequently:

- interface implementation remains determined by Go method sets;
- interface embedding remains governed by Go;
- assignability remains governed by Go;
- `!I` and `I` have the same underlying Go type identity.

These statements are a summary of the existing I3/I4 invariants, not a verbatim quotation of those earlier specifications.

Interface embedding does not propagate `!`.

For example, if:

```go
type Reader interface {
    Read([]byte) (int, error)
}

type ReadCloser interface {
    Reader
    Close() error
}
```

then `!ReadCloser` does not cause `Reader` to become `!Reader`, and `!Reader` does not automatically propagate into `!ReadCloser`.

The nilability contract is attached to the particular interface value whose type is annotated.

### 3.8 Assertion Boundary

Type assertions do not create `!I` contracts.

An assertion such as:

```go
v, ok := x.(io.Reader)
```

produces the ordinary interface type `io.Reader` when the asserted type is an interface.

`!io.Reader` is not a type-assertion target.

There is therefore no assertion form that means "assert that this interface value is non-nil" as a mechanism for producing `!io.Reader`.

### 3.9 Conversion Boundary

Interface conversion does not create a `!I` contract.

An explicit conversion changes the static type of the expression to the conversion target. When that target is ordinary interface type `I`, the resulting expression has ordinary type `I`, not `!I`.

Therefore:

```go
var p *File = nil

var r !io.Reader = p             // valid: D3a
var s !io.Reader = io.Reader(p)  // GN001: D6c → D3b
```

The two expressions have equivalent Go-level interface conversion behavior, but they are not equivalent for Gon contract propagation.

The rule chain is:

```
p
│
├── static type *File
│   └── assignable to !io.Reader → D3a → valid
│
└── io.Reader(p)
    └── static type io.Reader
        └── ordinary I → !I → D3b → GN001
```

The checker MUST evaluate the immediate expression's static type for the assignment. It MUST NOT recover the original concrete operand type through an explicit conversion in order to bypass D3b.

Likewise, Gon does not introduce `!io.Reader(p)` as a separate non-nil conversion mechanism.

### 3.10 No Dynamic-Value Tracking

The checker MUST NOT track the nilness of an interface's dynamic value as part of the `!I` contract.

For example, given:

```go
var p *File = nil
var r !io.Reader = p
```

the checker records the contract on `r` as:

```
interface value: non-nil
dynamic value: not constrained
```

The dynamic value may be typed-nil, and that state does not invalidate `!io.Reader`.

## 4. Examples

### 4.1 Valid: Concrete Value to `!I`

A concrete value assignable to an interface may be used to initialize `!I`:

```go
var f *File
var r !io.Reader = f
```

Whether `f` itself is nil is irrelevant to the interface contract.

If `f` is nil, the resulting interface value is still non-nil because it contains the dynamic type `*File`.

### 4.2 Valid: Typed-nil Concrete Value

The typed-nil case is intentional:

```go
var f *File = nil

var r !io.Reader = f // valid
```

Conceptually:

```
f:
    concrete value = nil
    type = *File

r:
    interface value = non-nil
    dynamic type = *File
    dynamic value = nil
```

`!io.Reader` constrains the second line, not the fourth.

### 4.3 Invalid: Nil Literal

A nil interface value cannot satisfy `!I`:

```go
var r !io.Reader = nil // GN001
```

### 4.4 Invalid: Ordinary Interface to `!I`

An ordinary interface value cannot be promoted to `!I`:

```go
var r io.Reader = someReader()

var x !io.Reader = r // GN001
```

The source type is ordinary `io.Reader`, so the checker has no `!` contract to propagate.

### 4.5 Invalid: Nil Check Does Not Narrow

A preceding nil check does not change the static contract:

```go
var r io.Reader = someReader()

if r != nil {
    var x !io.Reader = r // GN001
}
```

The condition establishes a runtime fact at that point in the program, but Gon does not model that fact as a new static `!io.Reader` type.

### 4.6 Valid: Existing `!I` Contract

A non-nil interface contract can be assigned to another compatible `!I`:

```go
func Open() !io.Reader {
    // ...
}

var r !io.Reader = Open()
var x !io.Reader = r
```

### 4.7 Return Contract Does Not Constrain Dynamic Value

A function may return a `!I` while its dynamic value is typed-nil:

```go
func Open() !io.Reader {
    var f *File = nil
    return f
}
```

The return contract guarantees that the returned interface value is non-nil.

It does not guarantee that the returned dynamic value is non-nil.

### 4.8 Assertion Does Not Produce `!I`

The ordinary assertion:

```go
r, ok := x.(io.Reader)
```

does not produce a `!io.Reader` contract.

The following is not a Gon mechanism:

```go
r, ok := x.(!io.Reader) // invalid construct
```

### 4.9 Conversion Does Not Produce `!I`

Ordinary conversion remains ordinary:

```go
r := io.Reader(f)
```

The result is an `io.Reader`, not an implicit `!io.Reader`.

There is no separate `!io.Reader(f)` conversion mechanism.

### 4.9a Explicit Conversion Prevents `!I` Satisfaction

The distinction between a concrete expression and an explicitly converted expression is intentional:

```go
var p *File = nil

var r !io.Reader = p             // valid: D3a
var s !io.Reader = io.Reader(p)  // GN001: D6c → D3b
```

The first assignment considers the static type of `p`, which is `*File`, so D3a applies.

The second assignment considers the static type of the immediate expression `io.Reader(p)`, which is ordinary `io.Reader`, so D6c applies first and D3b rejects the assignment.

The checker MUST NOT treat the second expression as if it still had the original concrete static type merely because its operand was `p`.

### 4.10 Interface Embedding Does Not Propagate Contracts

Given:

```go
type Reader interface {
    Read([]byte) (int, error)
}

type ReadCloser interface {
    Reader
    Close() error
}
```

these are independent contracts:

```go
var r !Reader
var rc !ReadCloser
```

`!ReadCloser` does not make a value of `Reader` non-nil, and `!Reader` does not automatically establish `!ReadCloser`.

The `!` modifier applies to the specific interface type in which it appears.

### 4.11 Semantic Asymmetry

For a concrete pointer:

```go
var p !*File
```

the contract concerns the pointer value itself.

For an interface:

```go
var r !io.Reader
```

the contract concerns the interface value itself.

Although both are represented by `Type.NonNil`, their guarantees are intentionally different:

```
!*File
  └── pointer value is non-nil

!io.Reader
  └── interface value is non-nil
      └── dynamic value: unconstrained
```

This asymmetry is deliberate and is not an implementation artifact.

## 5. Non-Goals

Interface non-nil contracts deliberately do not expand Gon into a flow-sensitive or runtime-aware type system.

### 5.1 No Typed-nil Detection

Gon does not attempt to determine whether an interface's dynamic value is a typed-nil value.

A `!I` contract therefore remains valid even when the dynamic value is known, or could be known, to be nil.

### 5.2 No Flow-sensitive Narrowing

A runtime check such as:

```go
if r != nil {
    // ...
}
```

does not change the static type or nilability contract of `r`.

In particular, it does not permit:

```go
var x !I = r
```

when `r` was declared as ordinary `I`.

### 5.3 No Runtime Checks

`!I` does not introduce runtime checks for:

- interface nilness;
- typed-nil dynamic values;
- dynamic type inspection;
- assertion of non-nilness.

The contract is enforced statically through the same Gon checker architecture used by other non-nil contracts.

### 5.4 No Dynamic-value Propagation

The checker does not propagate nilability information through an interface's dynamic value.

A concrete `!T` contract does not become a dynamic-value contract merely because a `T` value is stored in an interface.

Conversely, a `!I` contract does not establish `!T` for the dynamic value.

### 5.5 No Assertion or Conversion Propagation

Type assertions and explicit conversions are not additional mechanisms for manufacturing `!I` contracts.

Neither:

```go
x.(I)
```

nor:

```go
I(x)
```

implicitly produces a `!I` source.

The checker MUST NOT introduce special-case promotion based on the fact that a particular assertion or conversion happens to produce a non-nil interface value.

### 5.6 No Changes to Go Interface Semantics

This RFC does not modify:

- Go interface representation;
- Go interface nil semantics;
- Go dynamic type semantics;
- Go assignability;
- Go method sets;
- Go interface embedding;
- Go type identity.

The feature adds a Gon nilability contract on top of those existing semantics.

### 5.7 No General Flow Analysis

This RFC does not introduce general flow analysis for interface values.

Facts established by control-flow constructs, assignments, branches, or local reasoning are not converted into persistent `!I` contracts unless an existing explicit Gon contract mechanism provides that information.

### 5.8 No Stronger Interpretation of `!I`

Implementations and users MUST NOT interpret:

```
!I
```

as shorthand for:

```
interface value != nil
AND
dynamic value != nil
```

The only guaranteed property is:

```
interface value != nil
```

The dynamic value remains outside the contract.

## 6. Compatibility

- Schema version remains **1**. The syntax already permits `!` on interface types; this RFC only defines the semantics.
- Programs accepted by v1.2 remain accepted by a future v1.4 that implements this RFC, unless they attempt an ordinary `I` → `!I` assignment (or nil literal) that was previously accepted only because interface contracts were undefined.
- Adding a `!` annotation to an interface type is a deliberate strengthening by the annotation author.
- Ordinary values of concrete types remain assignable to `!I` under D3a (no tightening relative to concrete non-nil rules).
- Conversion and assertion continue to neither create nor propagate non-nil interface contracts.

## 7. Implementation Sketch

- Treat interface types that carry `Type.NonNil` under the same AST/representation path already used for concrete `!T`.
- At assignment and return sites, distinguish the static type of the immediate RHS:
  - concrete / named type assignable to `I` → accept for `!I` (D3a);
  - ordinary interface type `I` → reject (D3b / GN001);
  - existing `!I` → accept (D3d);
  - nil literal → reject (D3c).
- Do not walk through conversions or assertions to recover a concrete operand type.
- Do not consult control-flow facts or nil-check results.
- Re-use the existing non-nil-source machinery for results declared `!I`.
- Method-set and assignability checks continue to be performed by `go/types` against the underlying (non-`!`) interface type.

## 8. Test Requirements

Minimum regression suite:

- Concrete (including typed-nil) → `!I` accepted.
- Ordinary `I` → `!I` rejected (GN001), including after an explicit `!= nil` check.
- Nil literal → `!I` rejected (GN001).
- `!I` → `!I` accepted.
- Return of `!I` is a non-nil source at the call site.
- Explicit conversion `I(concrete)` produces ordinary `I` and cannot satisfy `!I`.
- Type assertion target cannot be `!I`.
- Interface embedding does not propagate `!`.
- Dynamic-value nilness is never reported as a violation of `!I`.
- Existing concrete, field, and return-value contract tests remain green.

## 9. Open Questions

None that affect the locked decisions in §2. Implementation details (diagnostic wording, exact interaction with type aliases of interfaces) can be finalised during review.

---

**Locked decisions (summary)**

| Decision | Choice |
|----------|--------|
| `!I` means interface value non-nil only | Yes |
| Typed-nil dynamic value violates `!I` | No |
| Concrete → `!I` | Accepted (even if concrete is nil) |
| Ordinary `I` → `!I` | Rejected (unconditional) |
| Flow-sensitive narrowing | No |
| Assertion / conversion creates `!I` | No |
| Schema bump | No (remain 1) |
| Runtime checks | No |
| Dynamic-value tracking | No |
