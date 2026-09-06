# nomagique

> **Normative specification**
>
> `nomagique` is a closed algebra for computation.
>
> This document is the contract. Code that contradicts it is wrong.
>
> The words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are normative.

---

# 1. The law

There is one computational material in this system:

```go
core.Primitive
```

Everything that participates in computation is a `Primitive`.

That includes values, arithmetic, calculus, logic, stores, transport, generators,
iterators, test inputs, introspection results, compositions, compositions of
compositions, and arbitrarily large graphs of compositions.

There is no privileged orchestration layer above the algebra. There is no second
execution model. There is no helper object that gets to behave like a Primitive
without being one.

If it computes, stores, routes, selects, retains, emits, delays, fans, gates,
describes, accumulates, or composes, it is a `Primitive`.

The system is **closed under composition**:

> A composition of Primitives is itself usable anywhere a Primitive is accepted.

A consumer MUST NOT care whether its incoming `Primitive` is a literal carrier,
one operation, a five-stage composition, a baseline made from fifty smaller
compositions, or a graph of graphs containing thousands of internal Primitives.

It receives a `Primitive`. That is all it is entitled to know.

---

# 2. The entire interface

```go
type Primitive interface {
    Next(Primitive) Primitive
    Read() any
    Error(...error) error
}
```

These three methods are the complete contract.

Implementing them is the whole of joining the system.

The interface MUST NOT grow to make one caller, one formula, one algorithm, one
data type, or one use case easier.

Do not add alternate advance or inspection methods such as `Step`, `Run`,
`Apply`, `Compute`, `Eval`, `Update`, `Flush`, `Reset`, `Done`, `More`, `Inputs`,
`Outputs`, `Metadata`, or `Mode` to the contract.

If behaviour is missing, compose existing Primitives or add the smallest missing
Primitive. Never add a second mechanism.

---

# 3. Fundamental axioms

## 3.1 Closure

Every computation is a Primitive. Every composition is a Primitive.

A composition can be passed to another Primitive exactly as a scalar carrier
can. A Primitive MUST NOT know the scale or internal topology of its neighbour.

## 3.2 One Primitive, one thing

Each Primitive owns exactly one irreducible behaviour at the lowest level where
that behaviour exists.

Not one feature. Not one use case. Not one algorithm. One operation.

Examples:

```text
Add       -> addition
Multiply  -> multiplication
Retained  -> retention
KV        -> keyed storage
Gate      -> gating
Fan       -> fan transport
IO        -> transport topology/configuration
```

A Primitive MAY own the state required for its one operation. It MUST NOT own
unrelated state because it is convenient to have nearby.

## 3.3 Composition is the only option

High-level behaviour is expressed by arranging lower-level Primitives.

If a requested behaviour can already be expressed by composition, no new
Primitive is added.

If it cannot, identify the smallest missing operation, add that Primitive, and
try the composition again. Repeat until the requested behaviour falls out of
the algebra.

There is no third path.

## 3.4 Build bottom-up

Never design the feature first and invent supporting infrastructure underneath
it.

Start with the irreducible operations required by the problem. Build those.
Compose upward. The high-level behaviour is a consequence of the low-level
vocabulary.

Top-down design tends to invent use-case types such as:

```text
EMAState
BaselineMeta
AnomalyContext
FanOut
FanIn
```

Bottom-up design asks what those things actually do and tends to discover more
general Primitives:

```text
Retained
KV
Fan
IO
```

## 3.5 Vocabulary grows downward

A use case may reveal a missing Primitive. The use case does not get to name it.

Before adding a Primitive, ask:

> What is the most general irreducible operation exposed by this need?

Bad:

```text
Meta
Metadata
BaselineMetadata
```

If the operation is "associate a key with a value", the Primitive is
`store.KV`.

Bad:

```text
FanIn
FanOut
```

If both are the same transport operation under different endpoint topology, the
Primitive is `transport.Fan`, configured through `transport.IO`.

Primitive names describe what they do, never why the first caller wanted them.

## 3.6 Complexity belongs in topology, not nodes

A complicated system may have a complicated graph. An individual Primitive
should remain painfully simple.

When opening a Primitive implementation, the reader should immediately be able
to answer:

> What one thing does this do?

If the implementation is clever, broad, heavily branched, or full of helper
machinery, assume it has not been reduced far enough.

The sophistication belongs in the arrangement of boring pieces.

---

# 4. `Next`

```go
Next(Primitive) Primitive
```

`Next` is the sole temporal operation of the algebra. It advances the callee and
yields one result in the callee's current delivery run.

`Next` is a generator protocol, not a conventional function call.

## 4.1 The caller's iteration obligation

When consuming a Primitive directly:

> **Keep calling `Next` until it returns `nil`.**

Nothing replaces this rule.

Do not infer the number of values in advance. Do not infer arity from type. Do
not assume one call means one value. Do not assume one `nil` means the Primitive
is dead.

## 4.2 Arity lives in time

A Primitive may yield:

```text
value
nil
```

or:

```text
value
value
value
nil
```

or any other finite delivery run defined by its semantics.

This lets one scalar, many scalars, a batch, or unrelated Primitives travel
through the exact same interface. The signature never changes.

## 4.3 Nil output belongs to the callee

A returned `nil` means only:

> The callee has ended the current delivery run for this caller.

It does **not** mean permanently exhausted, invalid, zero, false, closed forever,
or incapable of yielding in a later run.

A Primitive MAY intentionally use `nil` as a temporal boundary. For example, a
batching Primitive may deliver one batch, return `nil` to decouple the current
caller, retain its internal state, and resume delivery for a later caller.

The caller MUST infer nothing beyond "this run is over".

## 4.4 Nil input belongs to the callee

A `nil` input means only:

> The caller currently has no Primitive to offer.

What the callee yields in response is defined by that callee's one operation.
There is intentionally no universal answer.

Legitimate callee-defined behaviour can include an accumulator yielding its
configured/current seed, a source yielding what it holds, a composition yielding
a description of activated inputs, an introspection Primitive yielding a
`store.KV`, or a batcher yielding whatever its own delivery contract says is
currently available.

`nil` input is therefore part of the temporal protocol. It is not globally
synonymous with zero, reset, flush, previous value, final value, or description.

The meaning belongs to the Primitive receiving it.

## 4.5 Nil semantics do not justify mixed responsibilities

Although `Next(nil)` is callee-defined, a Primitive MUST NOT use nil as an excuse
to own unrelated behaviours.

This is immediately suspicious:

```go
if in == nil {
    doOneSubsystem()
} else {
    doAnotherSubsystem()
}
```

When a nil branch reveals a reusable operation such as fan, gate, retain,
select, describe, batch, route, or merge, factor that operation into a Primitive
and compose it.

Nil is expressive. It is not permission to create a switchboard.

---

# 5. Incoming Primitives are iterators

A non-nil incoming Primitive may yield one value or many.

Therefore a Primitive consuming `in` MUST NOT inspect it as though it were a
single value.

Forbidden inside ordinary Primitive computation:

```go
in.Read()
```

Forbidden:

```go
core.To[T](in)
```

Forbidden if it consumes only one assumed value:

```go
value := in.Next(...)
```

Forbidden as a local reimplementation of draining:

```go
for value := in.Next(...); value != nil; value = in.Next(...) {
    ...
}
```

There is exactly one canonical draining mechanism: `core.Yield`.

---

# 6. `Yield`

Canonical form:

```go
core.Yield(left, right, fold)
```

`Yield` is the one place that knows how to consume a Primitive correctly.

It drains `right` until `right.Next(...)` returns `nil` for the current run and
folds every yielded value into `left`.

It also owns error propagation, fold-boundary type checking, preservation of the
iterator contract, and identity behaviour when there is nothing to drain.

No Primitive reimplements this machinery.

## 6.1 The consumption rule

> **Every path in `Next` that consumes a non-nil incoming Primitive MUST consume
> it through `core.Yield`.**

This is non-negotiable.

If the Primitive ignores `in` entirely because ignoring it is part of its one
operation, there is nothing to drain. An inert carrier/source is the narrow
example.

Once a Primitive wants values from `in`, `Yield` is the only legal door.

## 6.2 "Usually one value" is irrelevant

A caller is allowed to replace a scalar input with a composition that yields
many values. The callee MUST continue to work without modification.

That is why direct `Read`, direct `To`, and hand-written single-step consumption
are forbidden.

## 6.3 No right-hand input

When there is no Primitive to drain, `Yield` returns the configured accumulator
according to its contract. This allows a composition to begin without a second
source mechanism.

## 6.4 The fold is the operation

The fold passed to `Yield` should be embarrassingly small.

For `Add`:

```go
func(held, value float64) float64 {
    return held + value
}
```

For a pure retain-latest operation:

```go
func(_, value float64) float64 {
    return value
}
```

If a fold contains unrelated transformations, orchestration, storage policy,
routing, warmup, fallback, or mode switching, the Primitive is doing more than
one thing. Factor it.

---

# 7. `Read`

`Read` surfaces the underlying Go value so a boundary can leave the algebra.

Inside computation, Primitives pass Primitives.

A Primitive MUST NOT use `Read` to inspect an incoming neighbour. A Primitive
MUST NOT unwrap another Primitive merely because it wants to know what concrete
thing is underneath.

The neighbour may be a huge composition. That opacity is intentional.

---

# 8. `Error`

`Error` both reports and records.

Called with no arguments:

```go
err := primitive.Error()
```

it reports accumulated failure.

Called with errors:

```go
primitive.Error(errA, errB)
```

it records them while preserving errors already carried.

A composition can fail in several places for unrelated reasons, so one later
error MUST NOT erase an earlier one.

Embedding `core.PrimitiveError` is canonical.

## 8.1 Errors travel with values

When a Primitive consumes another Primitive, errors carried by yielded values
continue through the composition. `Yield` owns this propagation for ordinary
folds.

Wrong types become errors. They do not silently become plausible zero values
inside computation.

## 8.2 Poison propagates

NaN and infinities are signal, not inconvenience.

A Primitive MUST NOT hide them by replacing them with zero, saturating them
without mathematical reason, skipping them, or catching them and continuing
with a fallback.

A mathematically saturating operation MAY map an extreme value to a finite
result when that is the operation's definition. That exception must be stated in
its test.

---

# 9. Entering and leaving the algebra

The ordinary value boundary is:

```go
core.From(value)
core.To[T](primitive)
```

## 9.1 `From`

`From` wraps a Go value as a Primitive. The wrapper is `core.Proto`.

A value entering the algebra does not gain domain behaviour. It simply becomes
something that can travel through the universal contract.

## 9.2 `To`

`To` reads a Primitive back into a Go value.

It belongs at a real boundary: serialization, public API output, logging,
diagnostics, UI projection, test assertion, or integration with non-nomagique
code.

Inside a composition, `To` is almost always evidence of a broken composition.
A Primitive should not need to escape the algebra to understand its neighbour.

---

# 10. `Proto`

`Proto` is an inert carrier. Its one job is to hold a Go value as a Primitive.

Canonical shape:

```go
type Proto struct {
    PrimitiveError
    state any
}

func NewProto(state any) *Proto {
    return &Proto{
        state: state,
    }
}

func (primitive *Proto) Next(in Primitive) Primitive {
    return primitive
}

func (primitive *Proto) Read() any {
    return primitive.state
}
```

`Proto` does not consume `in`, so it has no reason to call `Yield`.

No arithmetic, storage, conversion policy, metadata behaviour, routing, or
control belongs on `Proto`.

---

# 11. Composition

Composition is part of the algebra.

A composition MUST yield/return a `Primitive`. No caller receives a special
graph type that must be handled differently. No consumer gets privileged access
to the internals of the composition.

The resulting Primitive may be passed to `Next` exactly like any other
Primitive.

## 11.1 `Number`

A canonical numeric composition may be expressed as:

```go
func Number(primitives ...core.Primitive) (out core.Primitive) {
    for _, primitive := range primitives {
        out = primitive.Next(out)

        if out.Error() != nil {
            return
        }
    }

    return
}
```

The important contract is not the exact helper body. The important contract is
that every stage is a Primitive, every intermediate is a Primitive, the result
is a Primitive, the result may be passed into another Primitive, and no stage
knows whether its input is a scalar carrier or a massive composition.

## 11.2 No privileged high-level graph object

A named high-level construct such as a baseline MAY be a composition of many
Primitives. Its consumers still receive only a Primitive.

Do not create special `BaselineMeta`, `BaselineState`, or `BaselineRuntime`
types merely because the internal graph is complex.

If the graph needs keyed values, use `store.KV`. If it needs retention, use a
retention Primitive. If it needs routing, use transport Primitives. If it needs
introspection, the introspection result is itself a Primitive.

## 11.3 A Primitive never knows the scale of its neighbour

This is a hard substitutability rule.

The same `Next` implementation must remain valid when its input is a literal
carrier or a 200-stage composition of compositions, provided the yielded values
are valid for the consuming operation.

Concrete type checks against neighbouring Primitives are therefore presumed
wrong.

---

# 12. State, recurrence, and control by proxy

State belongs to the Primitive whose irreducible operation requires that state.
Do not duplicate recurrence state in a caller. Do not add state slots to
unrelated Primitives because one formula needs them.

## 12.1 Retention

If a formula needs a prior value, use a Primitive whose one job is retention.

Do not teach `Add` about previous values. Do not teach `Multiply` about EMA. Do
not create a recurrence engine.

Retention participates in the same algebra as everything else.

## 12.2 Control by proxy

The preferred way to alter behaviour of an existing Primitive is to alter the
Primitive stream it sees.

Do not modify the existing Primitive.

Compose another Primitive in front of it, behind it, or around the relevant
transport.

Examples:

```text
need a previous value             -> retention Primitive
need conditional passage          -> logic Primitive
need one value to reach N places  -> transport Primitive
need N sources to become one run  -> transport Primitive
need keyed state                  -> store.KV
```

The owner being controlled remains unchanged.

## 12.3 EMA

EMA is a use case, not automatically a Primitive.

Start from the recurrence and identify the low-level operations: retention,
multiplication, addition/subtraction, and transport if required.

Compose those.

If one irreducible operation is missing, add only that operation.

Do not create a monolithic `EMA` Primitive that manually owns prior state,
coefficient math, routing, warmup, arithmetic, and fallback behaviour.

The goal is for EMA to become topology, not special implementation.

---

# 13. Storage

Storage is computation and therefore Primitive.

Do not create metadata systems simply because a caller calls some keyed data
"metadata".

Ask what operation is actually required.

If the requirement is keyed association, use/build:

```text
store.KV
```

A `KV` can serve metadata, configuration, inputs, outputs, intermediate state,
introspection, or future uses not yet imagined. That breadth is evidence the
Primitive is named at the correct abstraction.

---

# 14. Introspection

Introspection stays inside the algebra.

If a complex Primitive needs to answer a question such as:

> Which inputs are currently activated?

its answer is itself a Primitive.

For example, the callee's `Next(nil)` contract may yield a `store.KV` whose
values are the relevant input Primitives.

Do not add `Metadata()`, `Inputs()`, or `Describe()` to `Primitive`.

Do not create a special `Meta` type if an existing general Primitive such as
`store.KV` already expresses the operation.

---

# 15. Logic and control flow

Logic is Primitive. Control is Primitive.

An `if` statement is not automatically illegal, because some irreducible
operations are conditional by mathematics. But every `if` creates a burden of
proof.

Before keeping an `if`, ask:

> What distinction caused this branch, and is that distinction itself a reusable
> Primitive?

If yes, factor it.

Likely missing operations may include gate, choose/select, minimum/maximum, fan,
route, batch, or retain.

An `if` that selects between responsibilities is almost certainly a design
failure. An `if` that is the mathematical definition of the one irreducible
operation may be correct.

The required instinct is always:

> **How would I factor this out?**

Repeat until the node is square.

---

# 16. Transport

Transport is Primitive.

Do not create separate `FanIn` and `FanOut` operations when they are the same
general transport operation under different topology.

Canonical vocabulary:

```text
transport.IO
transport.Fan
```

`IO` configures/describes an endpoint side. `Fan` owns the general fan transport
operation. Direction emerges from configuration.

Conceptually:

```go
transport.NewFan(
    transport.NewIO(core.From[int](1)),
    transport.NewIO(core.From[int](3)),
)
```

expresses one-to-three fanout.

And:

```go
transport.NewFan(
    transport.NewIO(core.From[int](3)),
    transport.NewIO(core.From[int](1)),
)
```

expresses three-to-one fanin.

Do not invent `FanIn`, `FanOut`, `FanMode`, or direction enums when `Fan + IO`
already spans the cases.

The exact internal representation MAY evolve, but the abstraction law does not:

> one transport operation; topology supplied through Primitives.

---

# 17. Batching and temporal decoupling

A batcher is a Primitive.

Its ability to return `nil` is part of the temporal protocol. It may intentionally
close the current caller's delivery run while preserving state for the next run.

Do not add a separate "batch complete" mechanism merely because a conventional
function API would need one.

If additional information about the batch is required, yield another Primitive,
for example `store.KV`.

---

# 18. Primitive discovery procedure

When a task appears, do not start coding the feature.

## Step 1 — describe behaviour without naming a type

Example:

> Keep the prior observation available while accepting the current observation.

Not:

> Implement EMAState.

## Step 2 — list irreducible operations

Example:

```text
retain prior value
multiply value
add values
route one value to two consumers
```

## Step 3 — search the existing Primitive vocabulary

For every operation, find the canonical Primitive that already owns it.

If all required operations exist, compose them and stop.

## Step 4 — identify the smallest missing operation

If something cannot be expressed, ask repeatedly:

> Is this the root operation, or merely the first use case I encountered?

Reject use-case names. Reject duplicated variants that differ only by
configuration. Reject garbage abstraction names such as `Helper`, `Meta`,
`Manager`, `Context`, `Processor`, `Runtime`, `Utils`, and `Common` when a
concrete operation exists underneath.

## Step 5 — add one square

Implement the missing Primitive.

It should normally have tiny state, one obvious constructor, one `Next`, one
`Read`, embedded `PrimitiveError`, and no need to teach neighbouring Primitives
about it.

Then return to the composition.

---

# 19. Forbidden architecture

The following are contract violations unless the operation itself proves they
are irreducible.

## 19.1 Use-case types for lower-level concepts

Bad:

```text
Metadata
EMAContext
BaselineState
AnomalyInputs
FanOut
FanIn
```

when the underlying operations are already represented more generally by:

```text
KV
Retained
IO
Fan
```

## 19.2 Concrete neighbour knowledge

Bad:

```go
switch value := in.(type) {
case *Baseline:
case *Add:
}
```

A Primitive receives `Primitive`. It does not care what graph produced it.

## 19.3 Direct input reads

Bad:

```go
value := in.Read()
```

Bad:

```go
value := core.To[float64](in)
```

Use `Yield`.

## 19.4 Hand-rolled iterator consumption

Bad inside an ordinary Primitive:

```go
for value := in.Next(...); value != nil; value = in.Next(...) {
    ...
}
```

Use `Yield`.

## 19.5 Second execution APIs

Bad when they advance computation outside `Next`:

```go
primitive.Step(...)
primitive.Run(...)
primitive.Compute(...)
```

## 19.6 Behaviour flags

Bad:

```go
if primitive.Mode == "ema" { ... }
```

Bad:

```go
if primitive.FanOut { ... }
```

Compose different Primitives/configuration instead.

## 19.7 Helper mini-systems

Bad when they own real behaviour outside the algebra:

```text
baselineHelper
emaEngine
fanController
metadataManager
```

If it owns behaviour, discover the irreducible Primitive it actually is.

## 19.8 Duplicate mechanisms

There is one way to advance: `Next`.

There is one way to drain incoming Primitive runs inside ordinary computation:
`Yield`.

There is one ordinary way in: `From`.

There is one ordinary way out: `To`.

There is one error channel: `Error`.

There is one computational material: `Primitive`.

Do not add parallel mechanisms.

---

# 20. Canonical implementation shape

An ordinary operation should be boring.

```go
package arithmetic

import (
    "github.com/theapemachine/symm/nomagique/core"
)

/*
Add is addition with retained accumulator state.
*/
type Add struct {
    core.PrimitiveError
    current core.Primitive
}

func NewAdd(state core.Primitive) *Add {
    return &Add{
        current: state,
    }
}

func (add *Add) Next(in core.Primitive) core.Primitive {
    add.current = core.Yield(
        add.current,
        in,
        func(held, value float64) float64 {
            return held + value
        },
    )

    return add.current
}

func (add *Add) Read() any {
    return add.current.Read()
}
```

If a new Primitive is dramatically more complicated, stop and ask what should
be factored out.

---

# 21. Constructors

Constructors configure. `Next` advances.

A constructor MAY accept other Primitives as configuration, MAY store those
Primitives, and MAY validate static configuration.

A constructor MUST NOT drain configured Primitives or execute runtime computation.

A Primitive should be whole before it is stepped.

---

# 22. Files and packages

One Primitive per implementation file.

The file is named after the Primitive according to repository convention.

A test file mirrors its implementation file.

Do not create behaviour dumping grounds such as:

```text
helpers.go
utils.go
common.go
meta.go
operations.go
special.go
ema_support.go
```

If code in such a file owns behaviour, discover the Primitive it actually is.

---

# 23. Package vocabulary

Package boundaries describe operation families, not use cases.

## `core`

Owns only the algebra itself:

```text
Primitive
PrimitiveError
Proto
From
To
Yield
```

plus minimal core errors/type constraints required by those mechanisms.

No domain operation belongs in `core`.

## `arithmetic`

Field operations such as:

```text
Add
Subtract
Multiply
Divide
```

## `calculus`

Scalar transforms/extrema such as:

```text
Absolute
Negate
Square
Sign
Exp
Log
Sqrt
Reciprocal
Tanh
Atanh
Erfc
Minimum
Maximum
```

## `store`

Storage/retention operations such as:

```text
Retained
KV
```

## `transport`

Transport topology such as:

```text
IO
Fan
```

## `logic`

Irreducible logical operations. No high-level strategy names belong here.

## `tests`

The shared Primitive test algebra/harness. Generated test inputs may themselves
be Primitives.

---

# 24. Testing follows the same architecture

Tests do not get an escape hatch.

Tests use canonical shared Primitives and shared test infrastructure.

Do not create 126 local fake implementations of the same source, constructor,
mock transport, or expected arithmetic.

Shared behaviour gets one owner in tests exactly as in production.

Changing the constructor of one underlying dependency should not require
mechanical edits across unrelated test files.

## 24.1 Mirroring

If implementation is:

```text
arithmetic/add.go
```

then test is:

```text
arithmetic/add_test.go
```

If implementation method is:

```go
func (add *Add) Next(...)
```

test method is:

```go
func TestAddNext(t *testing.T)
```

## 24.2 Tests declare cases, not answers

Use the shared `nomagique/tests` harness.

Example:

```go
func TestAddNext(t *testing.T) {
    tests.NewTestTable(
        tests.NewTestCase(
            "float64",
            "add",
            NewAdd(core.From(0.0)),
            tests.WithGenerator[float64](0, 0, 10, true),
        ),
    ).Run(t)
}
```

The test names the operation and supplies the case domain. It does not restate
the implementation in handwritten expected-value logic.

## 24.3 Iterator behaviour is mandatory

A Primitive that consumes input MUST be tested with both a one-value incoming
run and a multi-value incoming run.

Passing scalar tests alone does not prove compliance with `Next`.

## 24.4 Nil semantics are part of the contract

If a Primitive defines meaningful behaviour for `Next(nil)`, test it.

Where relevant, also test that returned `nil` ends only the current delivery
run rather than permanently killing the Primitive.

## 24.5 Poison propagation

The harness should exercise:

```text
NaN
+Inf
-Inf
```

A Primitive propagates poison unless saturation is mathematically defined. A
saturation exemption must be documented directly above the test.

---

# 25. The `if` review rule

Every `if` in a Primitive triggers architectural review.

Ask:

1. Is this branch the actual irreducible mathematics of this Primitive?
2. Is it selecting between behaviours that should be separate Primitives?
3. Can the condition itself be represented by a logic Primitive?
4. Can transport topology remove the branch?
5. Can storage/retention remove the branch?
6. Can configuration through another Primitive remove the branch?

If the branch can be factored, factor it.

The standing review question is:

> **How would you factor this out?**

Design as though that question will be asked every time.

---

# 26. Agent protocol

An Agent modifying `nomagique` MUST follow this procedure in order.

## Before editing

1. Read this specification.
2. Describe the requested behaviour without inventing a type name.
3. Decompose the behaviour into irreducible operations.
4. Search the existing Primitive vocabulary for each operation.
5. Attempt the complete behaviour by composition.
6. Only if composition fails, identify the smallest genuinely missing operation.
7. Ask whether the proposed name describes the operation or merely the first use case.
8. Search for a more general existing abstraction before adding anything.

## While editing

9. Add only genuinely missing Primitives.
10. One Primitive owns one operation.
11. One Primitive per file.
12. Constructors configure; `Next` advances.
13. Every consumed non-nil incoming Primitive goes through `core.Yield`.
14. Never directly `Read` or `To` an incoming neighbour.
15. Never assume an incoming Primitive yields exactly one value.
16. Never assume an incoming Primitive has a concrete type.
17. Never add a second execution/draining/composition mechanism.
18. Never add use-case flags to an existing Primitive.
19. Prefer control by proxy over changing an existing Primitive.
20. Treat every `if` as a factorization challenge.
21. If behaviour can be moved into a more general Primitive, move it.
22. Do not create `Meta`, `Helper`, `Manager`, `Context`, or similar abstractions when a concrete general operation exists.

## Testing

23. Add the mirrored test.
24. Use the shared harness.
25. Test single-value input.
26. Test multi-value input.
27. Test nil semantics where applicable.
28. Test poison propagation.
29. Do not duplicate reference arithmetic or local fake infrastructure.

## Before delivery

30. Confirm the requested high-level behaviour is composition.
31. Confirm every new Primitive is lower-level than the use case that revealed it.
32. Confirm each new Primitive has plausible uses outside the first use case.
33. Confirm no existing Primitive had to learn about the new Primitive.
34. Confirm no caller needs to know whether an input is scalar or a huge composition.
35. Confirm there remains one way to advance, drain, enter, leave, and compose.
36. Search the diff for `if` and justify every surviving branch as irreducible.
37. Search the diff for concrete Primitive type assertions.
38. Search the diff for direct `Read`/`To` of incoming Primitives.
39. Run package tests.
40. Stop.

---

# 27. Rejection checklist

A review MUST reject a change if any of these remain unexplained:

- a new Primitive owns more than one operation;
- the requested behaviour could already be expressed by composition;
- a type is named after the use case rather than the operation;
- a more general existing Primitive covers the same behaviour;
- an incoming Primitive is directly read or converted;
- an incoming Primitive is consumed without `Yield`;
- the implementation assumes one incoming value;
- the implementation assumes the concrete type of its neighbour;
- a composition is represented by a non-Primitive runtime object;
- a second execution or iterator mechanism was added;
- a constructor advances runtime computation;
- a behaviour flag was added where composition should choose behaviour;
- a helper object owns behaviour outside the algebra;
- recurrence state has more than one owner;
- a branch exists that can be factored into another Primitive;
- `FanIn`/`FanOut` exists where `Fan + IO` expresses topology;
- a `Meta` type exists where `KV` expresses keyed storage;
- tests duplicate shared infrastructure;
- tests declare answers by reimplementing operations;
- poison is silently sanitised.

---

# 28. Canonical reasoning examples

## 28.1 "I need metadata"

Wrong question.

Ask what operation is needed.

If the answer is "associate keys with values", use `store.KV`.

## 28.2 "I need fanout"

Wrong noun.

Ask what operation routes N inputs to M outputs.

Use `transport.Fan` configured by `transport.IO`.

Do not create `FanOut`.

## 28.3 "I need EMA"

Do not start with `EMA`.

Start with the recurrence. Find retention, multiply, add/subtract, and routing
operations. Compose. If one operation is missing, add that one operation.

## 28.4 "I need anomaly detection over a complex baseline"

The anomaly Primitive receives `core.Primitive`.

The baseline may be an enormous composition underwater. The anomaly Primitive
does not know or care. It consumes what the baseline yields through `Yield`.

## 28.5 "I need to know which inputs are active"

Do not add an `Inputs()` interface method. Do not invent `Meta`.

If the callee's contract says `Next(nil)` surfaces introspection, it may yield a
`store.KV` whose values are themselves the relevant input Primitives.

The result remains inside the algebra.

---

# 29. Canonical mental model

```text
                                  +-------------------------------+
                                  |  huge composition of graphs   |
                                  |  stores / logic / transport   |
                                  |  arithmetic / calculus / ...  |
                                  +---------------+---------------+
                                                  |
                                                  | still just
                                                  v
                                            core.Primitive
                                                  |
                                                  v
                                            Next(Primitive)
                                                  |
                           consume non-nil input  | through Yield
                                                  v
                                            core.Primitive
                                                  |
                                                  v
                                                ...
```

There is no point where the graph becomes "too complex" and earns the right to
leave the algebra.

There is no high-level object that becomes special.

There are only more Primitives.

---

# 30. The square-snowflake rule

Each Primitive is a snowflake trimmed to a square.

Its implementation is plain.

Its name is general.

Its one operation is obvious.

Its input may be arbitrarily complex.

Its output remains a Primitive.

Its power comes from where it is connected, not from how many responsibilities
are hidden inside it.

When a new problem appears:

1. work from the bottom;
2. find the low-level operations;
3. compose what already exists;
4. add only the missing square;
5. compose again.

That is how the vocabulary becomes more powerful without the nodes becoming more
complicated.

---

# 31. Final rule

If a developer feels tempted to go around the algebra, stop.

Do not add an escape hatch.

Do not add a helper runtime.

Do not add a special-case interface method.

Do not unwrap the graph.

Do not teach an existing Primitive a new mode.

Do not name the first use case.

Find the missing low-level operation.

Make it a Primitive.

Compose.

**There is no third option.**
