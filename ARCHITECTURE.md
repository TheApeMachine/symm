# SYMM / nomagique — Runtime Architecture

## Status

This document defines the target runtime architecture for SYMM / nomagique.

It supersedes the previous closure-based `Value` / `Number` execution model, the `StreamNode[any, any]` migration, typed `*Sink` routing for ordinary primitives, and generated per-primitive execution glue.

The objective is:

> Compile Flume JSON directly into an executable, in-memory Cap'n Proto program that can be replaced safely at runtime when the graph changes.

There is no generated Go application for a graph.

There is no generic Go payload type.

There is no second RPC/event framework layered over Cap'n Proto.

Cap'n Proto is the object model, invocation protocol, type system, and RPC substrate.

Flume JSON is the program.

---

# 1. Non-negotiable invariants

These rules define the architecture.

1. **JSON is the program.** A Flume graph is compiled in memory into an executable `Program`.
2. **Compilation never generates Go application code.** `foo.json -> foo.go` does not exist.
3. **The only primitive-specific bootstrap is a small constructor registry.** It maps a node type string to a Cap'n Proto capability constructor.
4. **The constructor registry returns `capnp.Client`.** The compiler does not retain concrete Go server implementations.
5. **No graph value is represented as `any` or `interface{}`.** No `WriteAny`, `SetDownstreamAny`, `map[string]any`, payload type assertion, or equivalent belongs in graph execution.
6. **Cap'n Proto schemas are authoritative for ports and types.** Flume input ports map to `write` parameter fields. Flume output ports map to `done` result fields.
7. **Ordinary primitives do not know their downstreams.** Graph routing belongs to the compiled `Program`, not to primitive servers.
8. **Ordinary primitives do not retain state across evaluations.** `Done()` returns the result and resets the primitive.
9. **Retention is explicit composition.** If state must survive an evaluation, compose a `store.*` primitive.
10. **A compiled `Program` is immutable.** Editing Flume compiles a candidate program and swaps it in only after successful validation.
11. **A failed recompile never damages the running program.** The current program continues unchanged.
12. **Topology is compiled away.** Runtime execution uses numeric node indices, pre-resolved methods, pre-resolved fields, readiness masks, and route tables. It does not interpret JSON.

---

# 2. Primitive algebra

An ordinary primitive is a stateful Cap'n Proto object for the duration of one evaluation.

Its protocol is:

```capnp
interface Primitive {
  write @0 (...) -> stream;
  done @1 () -> (...results...);
}
```

`write` receives the primitive's inputs.

`done`:

1. returns the primitive's result;
2. resets all ephemeral primitive state;
3. leaves the capability ready for the next evaluation.

Example:

```capnp
interface Add {
  write @0 (
    a :Float64,
    b :Float64
  ) -> stream;

  done @1 () -> (
    out :Float64
  );
}
```

Implementation:

```go
type AddServer struct {
    out float64
}

func (srv *AddServer) Write(
    ctx context.Context,
    call Add_write,
) error {
    srv.out = call.Args().A() + call.Args().B()
    return nil
}

func (srv *AddServer) Done(
    ctx context.Context,
    call Add_done,
) error {
    result, err := call.AllocResults()
    if err != nil {
        return err
    }

    result.SetOut(srv.out)
    srv.out = 0
    return nil
}

func NewAdd() *AddServer {
    return &AddServer{}
}
```

The primitive has no knowledge of Flume, JSON, node IDs, downstream nodes, graph routing, graph scheduling, `Sink` capabilities, or a generic payload type. It implements only its algebra.

---

# 3. Why `write -> stream` and `done -> result`

The protocol intentionally separates input delivery from result retrieval.

The runtime executes an ordinary node as:

```text
write(args)
    |
    v
WaitStreaming()
    |
    v
done()
    |
    v
result fields
```

This uses Cap'n Proto directly:

- `write -> stream` uses the streaming call path;
- `WaitStreaming()` is the evaluation fence;
- `done -> (...)` is an ordinary RPC returning a future;
- the future yields the result struct.

For an ordinary scalar node there will commonly be one `write` per evaluation.

A primitive may accept multiple writes within one evaluation only if its algebra explicitly defines such behavior. The result still appears only at `done`.

`Done()` is the evaluation boundary. It is not persistence and it is not object destruction.

---

# 4. Persistent state is a primitive

Ordinary primitives reset on `Done()`.

If an operation needs information from previous evaluations, that state must be explicit in the graph.

```text
input
  |
  v
Store
  |
  v
RelativeChange
```

The retention belongs to `Store`. It does not belong invisibly inside `RelativeChange`.

Resource state is different from algebraic state. A network connection, file handle, HTTP listener, or exchange socket may live across evaluations because it is an external resource owner. It must not be used as a hidden substitute for algebraic storage.

---

# 5. Flume ports map directly to Cap'n Proto fields

A Flume node is an invocation shape.

Input ports are the fields of `write`.
Output ports are the fields of `done`.

For:

```capnp
interface Add {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 () -> (out :Float64);
}
```

Flume:

```text
        a ─┐
           ├── Add ── out
        b ─┘
```

The edge:

```text
foo.out -> add.a
```

means exactly:

```text
Foo.done result field "out"
    ->
Add.write parameter field "a"
```

No `AddA` interface exists. No `Float64Sink` is needed for ordinary primitive composition. No handwritten adapter defines what `add.a` means. The schema already defines it.

## 5.1 Naming convention

Prefer schema field names that match Flume port names exactly.

Unary primitive:

```capnp
write @0 (in :Float64) -> stream;
done  @1 () -> (out :Float64);
```

Binary primitive:

```capnp
write @0 (a :Float64, b :Float64) -> stream;
done  @1 () -> (out :Float64);
```

Multi-output primitive:

```capnp
done @1 () -> (
  class :Text,
  confidence :Float64,
  novelty :Float64
);
```

Do not create a permanent alias layer to compensate for inconsistent names. During migration, make schemas and Flume agree.

---

# 6. The constructor registry

Go cannot instantiate an arbitrary concrete implementation from a runtime string without a static bootstrap. That is the only reason the constructor registry exists.

Its responsibility is:

```text
node type string
    ->
construct implementation
    ->
ServerToClient
    ->
capnp.Client
```

Conceptually:

```go
type Constructor func(
    context.Context,
    json.RawMessage,
) (capnp.Client, error)

type Factory struct {
    InterfaceID uint64
    New         Constructor
}

type Registry map[string]Factory
```

Example:

```go
registry["arithmetic.Add"] = Factory{
    InterfaceID: arithmetic.Add_TypeID,
    New: func(
        ctx context.Context,
        config json.RawMessage,
    ) (capnp.Client, error) {
        server := arithmetic.NewAdd()
        client := arithmetic.Add_ServerToClient(server)
        return capnp.Client(client), nil
    },
}
```

Factories that need typed process dependencies capture them in their closure at application bootstrap. Do not introduce a generic dependency bag containing `any`.

The registry must not contain input setters, output readers, input sinks, downstream binders, per-primitive `Invoke` functions, `Done` wrappers, graph routes, or primitive-specific switches.

After construction, the compiler sees only the capability. The concrete Go server disappears from compiler ownership.

---

# 7. Cap'n Proto schema metadata is the runtime type system

The compiler obtains the registered interface schema using the factory's interface/type ID.

From that schema it resolves the protocol methods:

```text
write
done
```

For each node it compiles:

```text
write:
    interface ID
    method ID
    argument object size
    parameter fields
    parameter field types

done:
    interface ID
    method ID
    result object size
    result fields
    result field types
```

The compiler does not generate per-primitive setters such as:

```go
arithmetic.Add_write_Params(s).SetA(...)
```

The runtime uses Cap'n Proto's generic send plus schema/dynamic APIs. Generated Go bindings remain useful inside primitive implementations, but the graph compiler does not duplicate them.

---

# 8. What "compile" means

`Compile` transforms a Flume graph into an immutable executable plan in memory.

Conceptually:

```go
program, err := Compile(document, registry, previous)
```

Compilation performs all expensive interpretation once.

Execution must not repeatedly parse JSON, resolve node type strings, resolve port names, inspect schemas, search maps by node name, infer field types, rebuild adjacency, or decide how to copy a field.

Those are compile-time operations.

---

# 9. Compilation pipeline

## Phase 1 — Parse the Flume document

Read the selected graph from the editor document.

Layout fields such as `x`, `y`, `width`, `height`, `viewport`, and comments are not execution semantics.

Execution-relevant fields include:

```text
versionKey
nodes
node.id
node.type
node.inputData
node.connections
```

The editor version becomes the candidate program version.

## Phase 2 — Resolve nested definitions

A node such as:

```text
definition:signals
```

is recursively loaded and expanded in memory.

Expanded node IDs are namespaced so stable identity is preserved, for example:

```text
signals/returns
signals/corr
signals/zscore
```

Definition boundaries may remain as debug metadata but disappear from execution topology.

## Phase 3 — Resolve factories

For each concrete node:

```text
node.type -> Factory
```

The factory supplies an interface ID and constructor.

Unknown types fail compilation.

No capability must be constructed yet.

## Phase 4 — Resolve protocol schemas

Using the interface ID, compile the node's Cap'n Proto protocol.

Ordinary nodes must satisfy:

```text
write -> stream
done  -> result
```

The compiler records parameter and result field descriptors.

## Phase 5 — Resolve Flume ports

For each Flume input port:

```text
port name -> write parameter field
```

For each Flume output port:

```text
port name -> done result field
```

A missing field is a compile error. There is no fallback port and no best-effort inference.

## Phase 6 — Compile static inputs

A `write` parameter may be supplied by an incoming graph edge or by static `inputData`.

Example:

```json
{
  "type": "data.Extract",
  "inputData": {
    "path": {
      "string": "last"
    }
  }
}
```

The compiler validates the static value against the Cap'n Proto field type and writes it into an immutable argument template for the node.

Static values are not reparsed on every evaluation.

A required field with neither an incoming edge nor a valid static value is a compile error.

## Phase 7 — Validate edges

For each connection:

```text
fromNode.fromPort -> toNode.toPort
```

resolve:

```text
fromPort -> source done result field
toPort   -> destination write parameter field
```

Then validate Cap'n Proto type compatibility.

Examples:

```text
Float64 -> Float64       valid
Text    -> Float64       invalid
FooStruct -> FooStruct   valid
FooStruct -> BarStruct   invalid unless schema assignability explicitly permits it
```

Type mismatch is a compile error.

## Phase 8 — Validate topology

Validate referenced nodes and ports, required inputs, route compatibility, source/terminal semantics, and unsupported cycles.

Ordinary execution is a DAG.

Feedback must be represented by an explicit temporal/store/delay construct that gives the cycle defined semantics. Do not silently execute arbitrary structural cycles.

## Phase 9 — Compile field copy operations

Each edge becomes a precompiled field transfer operation.

Conceptually:

```go
type Copier func(
    src capnp.Struct,
    dst capnp.Struct,
) error
```

`Copier` is created from Cap'n Proto field descriptors and is generic by Cap'n Proto type kind.

It may handle Bool, signed/unsigned integers, floats, Text, Data, Enum, Struct, List, Interface, and AnyPointer.

No graph value becomes a Go `any`.

## Phase 10 — Compile numeric topology

Replace hot-path strings with compact indices.

Conceptually:

```go
type NodeID uint32
type FieldID uint16
```

Compile:

```text
"corr"."out" -> "atanh"."in"
```

into something equivalent to:

```text
node 4, result field 0
    ->
node 7, argument field 0
```

Human-readable IDs remain only as debug metadata.

## Phase 11 — Reuse or construct capabilities

Only after structural and type validation succeeds do we create resources.

If a previous `Program` exists, reuse a node capability when all of these match:

```text
stable node ID
node type
constructor configuration fingerprint
interface ID
```

Topology does not affect capability identity.

Reusing unchanged capabilities avoids needless resource reconnection and lets explicit Store nodes preserve state across topology-only recompiles.

When reusing a capability, the new program acquires its own Cap'n Proto reference.

If a node is new or its construction identity changed, construct a new capability from the small registry.

If construction fails, abort the candidate and keep the active program unchanged.

## Phase 12 — Produce immutable `Program`

After compilation succeeds, freeze the program.

No topology mutation occurs inside a live `Program`.

---

# 10. Compiled program representation

The exact Go types are implementation detail, but ownership should resemble:

```go
type Program struct {
    Version string

    Nodes  []CompiledNode
    Routes []Route

    Roots []NodeID
}
```

A compiled node contains capability and compiled schema information only:

```go
type CompiledNode struct {
    Client capnp.Client

    Write CompiledMethod
    Done  CompiledMethod

    Inputs  []CompiledInput
    Outputs []CompiledOutput

    Required InputMask
    ArgsTemplate capnp.Struct

    Identity NodeIdentity
}
```

It does not contain a concrete Go server, `server any`, payload `any`, a primitive-specific invoke function, or a primitive-specific downstream binder.

A route contains resolved indices and a compiled copy operation:

```go
type Route struct {
    FromNode  NodeID
    FromField FieldID

    ToNode    NodeID
    ToField   FieldID

    Copy Copier
}
```

The executor never asks what primitive type a node is.

---

# 11. Evaluation frame

Execution of one graph observation uses an evaluation frame.

The frame contains per-node invocation state, not generic graph values.

Conceptually:

```go
type NodeFrame struct {
    Args     capnp.Struct
    Ready    InputMask
    Executed bool
}
```

Each node begins with a copy of its immutable static argument template.

Incoming routes fill the remaining Cap'n Proto argument fields.

Readiness is a bitset.

For `Add.write(a,b)`:

```text
required = 11
ready    = 00
```

After `a` arrives:

```text
required = 11
ready    = 01
```

Do nothing.

After `b` arrives:

```text
required = 11
ready    = 11
```

The node is runnable.

No `map[string]any` exists.

---

# 12. Node execution

When a node becomes runnable:

1. send its compiled `write` streaming call using the assembled argument struct;
2. wait for streaming delivery to complete;
3. send its compiled `done` call;
4. await the result struct;
5. route each connected result field to downstream argument fields;
6. mark downstream nodes ready;
7. release result call resources.

Conceptually:

```text
assembled write args
       |
       v
SendStreamCall(write)
       |
       v
WaitStreaming()
       |
       v
SendCall(done)
       |
       v
result struct
       |
       +--> route result field 0
       +--> route result field 1
       +--> ...
```

Because `Done()` resets the primitive, the capability is ready for the next evaluation.

---

# 13. Fan-out

Fan-out is a routing property of the compiled plan.

If one output connects to four nodes:

```text
corr.out
   |
   +--> sink_signed_correlation.value
   +--> abs.in
   +--> atanh.in
   +--> zscore.in
```

compile four routes from the same result field.

At runtime the result field is copied into each destination argument struct.

The producer does not know it has four consumers.

No `BroadcastSink` is necessary.

---

# 14. Fan-in

Fan-in is argument assembly.

For:

```text
left.out  -> add.a
right.out -> add.b
```

the frame waits until both destination fields are ready.

Then and only then does it invoke `Add.write(a,b)`.

No input sink capability is required.

No evaluation ID needs to be embedded into every scalar merely to correlate `a` and `b`, because the evaluation frame already owns both argument slots.

If values cross an asynchronous boundary where multiple evaluations can interleave, that boundary protocol must carry an explicit correlation identity. Do not make every ordinary primitive pay for that boundary requirement.

---

# 15. Execution ordering and concurrency

The primitive protocol stores ephemeral result state between `Write()` and `Done()`.

Therefore overlapping evaluations must not race through the same capability.

The default execution model is:

> One graph evaluation at a time per `Program`.

Within one evaluation, independent DAG branches may execute concurrently.

For:

```text
             +--> Atanh --+
input -------+            +--> Add
             +--> Square -+
```

`Atanh` and `Square` may execute in parallel because they are independent within the same evaluation.

A second source evaluation does not enter the same program until the first has crossed its `Done()` boundaries.

If higher throughput is later required, concurrency must be explicit through capability pools, `Parallel` composition, sharded programs, or another algebraically visible mechanism.

Do not silently make ordinary node servers handle overlapping evaluations.

---

# 16. Source nodes

A live source is a boundary object, not an ordinary downstream callback.

The graph runtime owns source admission.

A source capability should expose a normal Cap'n Proto operation that yields the next observation, for example:

```capnp
interface Source {
  next @0 () -> (
    out :WireMeasurement
  );
}
```

The implementation may block internally waiting for Kraken, a file, IPC, or another transport.

The runtime loop is conceptually:

```text
Source.next()
    |
    v
new evaluation
    |
    v
compiled Program
```

If a source genuinely requires asynchronous callback capabilities, use Cap'n Proto capability passing directly at that boundary. Do not generalize that callback pattern into ordinary primitive composition.

Candidate compilation must not accidentally start duplicate uncontrolled listeners or disturb the active source.

---

# 17. Terminal and side-effect nodes

A terminal node may have no meaningful result.

It can still use:

```capnp
interface Sink {
  write @0 (value :WireMeasurement) -> stream;
  done @1 ();
}
```

or return diagnostic data if useful.

A sink is a real side-effect boundary. It is not a generic transport abstraction inserted between ordinary nodes.

---

# 18. Constructor configuration

Invocation inputs and construction configuration are different concepts.

## 18.1 Invocation inputs

Fields of `write(...)` supplied by graph edges or static `inputData`.

## 18.2 Construction configuration

Values required once to construct a capability.

These belong in a dedicated node configuration object rather than streamed arguments.

The constructor registry receives configuration as immutable raw JSON or another non-erased representation and parses it into the primitive's typed constructor configuration.

Do not use `map[string]any`.

Live process dependencies are captured by the registered factory closure.

Secrets and resource handles do not belong in Flume JSON.

---

# 19. Recursive definitions

A JSON definition is syntax, not a second runtime object model.

If the root graph contains:

```text
definition:signals
```

the compiler recursively loads and expands it into the candidate program.

Definitions may contain other definitions.

The compiler detects recursive definition cycles.

Definition expansion preserves stable namespaced node identity so nodes can be reused across recompiles.

No generated Go composite is created.

No `pipeline.Signals` pseudo-primitive is registered.

---

# 20. Hot recompilation

Hot recompilation is a first-class requirement.

Editing Flume must not mutate the active execution graph in place.

The runtime owns an active immutable program:

```go
type Runtime struct {
    active atomic.Pointer[Program]
}
```

The sequence is:

```text
Flume edit
    |
    v
new JSON/version
    |
    v
Compile candidate while old Program keeps running
    |
    +--> compile error
    |       |
    |       +--> report error to editor
    |       +--> keep old Program unchanged
    |
    +--> compile success
            |
            v
        activation barrier
            |
            v
        atomic Program swap
            |
            v
        retire old Program
```

## 20.1 Initial swap policy: quiescent swap

The first implementation should prefer determinism over cleverness.

1. Compile the candidate completely while the current program runs.
2. Stop admitting a new source evaluation.
3. Let the currently active evaluation finish.
4. Activate any new resources needed by the candidate.
5. Atomically swap the active `Program`.
6. Retire capabilities owned only by the old program.
7. Resume source admission.

The pause is only the activation boundary, not compilation time.

This prevents old and new topology from processing the same evaluation and avoids cross-generation races through ephemeral primitive state.

---

# 21. Capability reuse across recompilation

A topology edit should not reconstruct every object.

A candidate compiler receives the previous `Program`.

For each node, build a construction identity from:

```text
stable node ID
node type
interface ID
constructor config digest
```

If the identity is unchanged, reuse the previous `capnp.Client` via `AddRef()`.

If it changed, construct a new capability.

Topology does not affect capability identity.

Example:

Old:

```text
corr.out -> atanh.in
```

New:

```text
square.out -> atanh.in
```

If `atanh` itself did not change, reuse the same Atanh capability. Only the immutable route table changes.

This also lets explicit stores preserve state across topology-only recompiles without a separate state migration framework.

A Store with the same stable node ID, type, interface, and constructor config is the same object and therefore retains its state.

If it is removed, renamed, changes type, or changes construction identity, the old capability is retired.

---

# 22. Program ownership and release

Every `Program` owns references to all capabilities in its node table.

When reusing a capability, the new program acquires its own Cap'n Proto reference.

On candidate failure, release all capabilities created or referenced solely by the failed candidate.

On successful swap, retire the old program and release its references after no evaluation is using it.

Reused capabilities stay alive because the new program owns its own references.

Normal Cap'n Proto capability lifetime rules own object lifetime.

---

# 23. UniConn and remote execution

UniConn is transport only.

```text
capnp.Client
    |
    v
rpc.Conn
    |
    v
rpc.StreamTransport
    |
    v
UniConn
    |
    v
io.ReadWriteCloser
```

Do not place UniConn between ordinary local nodes.

Do not turn UniConn into a graph scheduler, node wrapper, type system, generic message bus, or event router.

Local capabilities stay local.

If a capability is remote, the same `capnp.Client` semantics continue across `rpc.Conn`.

The graph compiler should not care whether a capability resolves locally or remotely.

---

# 24. Flume editor contract

Flume is an editor for the source program.

The editor and compiler consume the same Cap'n Proto-derived primitive catalog.

For every primitive, Flume needs descriptive metadata only:

```text
operation name
display name
write input fields
done output fields
Cap'n Proto types
constructor configuration schema
documentation
```

The catalog is not executable glue.

When the editor changes a connection, the runtime recompiles the JSON.

Graph fragments such as:

```text
returns.out -> corr.in
returns.out -> square.in

corr.out -> abs.in
corr.out -> atanh.in
corr.out -> zscore.in
```

compile into in-memory routing tables.

No primitive is rewritten and no Go source is generated.

---

# 25. Compile errors are editor feedback

Compilation errors are part of the Flume experience.

Examples:

```text
unknown primitive type
unknown input port
unknown output port
type mismatch
missing required input
duplicate binding
unsupported cycle
invalid static value
invalid constructor config
definition not found
recursive definition cycle
capability construction failure
```

A compile error must identify the node ID, node type, port/field when relevant, source graph/definition, and human-readable reason.

The previous program stays active.

A broken editor state is not a broken runtime.

---

# 26. No custom application code generation

Prohibited:

```text
Flume JSON
    ->
generated Go graph
```

Also prohibited:

```text
primitive schemas
    ->
generated giant registry containing:
    - setters
    - sink builders
    - invokers
    - downstream adapters
    - primitive-specific switches
```

Cap'n Proto already generates its language bindings.

SYMM must not generate a shadow binding layer.

If automatic constructor registration is desired, the maximum acceptable custom generation is a boring constructor table containing only:

```text
operation name
interface ID
func(...) capnp.Client
```

Manual constructor registration is also acceptable.

---

# 27. No `any`

The execution ownership path must contain no Go `any` / `interface{}` payloads.

Specifically prohibited:

```go
WriteAny(...)
SetDownstreamAny(...)
map[string]any
[]any
func(any)
payload.(float64)
server any
server.(*ConcreteServer)
```

The compiler may parse arbitrary JSON using a streaming/token/raw-message representation, but it must convert values into typed Cap'n Proto fields during compilation.

The runtime operates on:

```text
capnp.Client
capnp.Struct
Cap'n Proto schema descriptors
compiled field copy operations
numeric indices
bit masks
```

Graph values do not leave the Cap'n Proto type system.

---

# 28. Generic runtime logic is allowed; primitive-specific runtime logic is not

The compiler/executor may switch on Cap'n Proto type kind:

```text
Bool
Int
UInt
Float
Text
Data
Enum
Struct
List
Interface
AnyPointer
```

because those are properties of the type system.

It must not switch on primitive identity:

```text
if op == "arithmetic.Add"
if op == "cognition.Associate"
if op == "execution.Decide"
```

If a primitive requires special runtime behavior that cannot be described by its Cap'n Proto protocol, the schema/protocol is incomplete.

Fix the protocol instead of adding a compiler special case.

---

# 29. Constructors must be safe for candidate compilation

A constructor creates a capability object. It must not unexpectedly mutate global runtime state.

Candidate compilation must not steal a live port from the active program, replace the active source connection, start duplicate uncontrolled listeners, mutate a reused store, or shut down an active resource.

External resource owners should separate construction from activation when necessary.

The hot-recompile guarantee is:

> A candidate that never activates must not alter the behavior of the active program.

---

# 30. Example: compiling the current Flume shape

Given:

```text
source.out -> lastPrice.in
lastPrice.out -> returns.in
returns.out -> corr.in
returns.out -> square.in
corr.out -> abs.in
corr.out -> atanh.in
corr.out -> zscore.in
square.out -> energy_mean.in
square.out -> energy_zscore.in
```

compile routes conceptually equivalent to:

```text
Route 0:
Source result out
    -> Extract.write.in

Route 1:
Extract.done.out
    -> LogReturns.write.in

Route 2:
LogReturns.done.out
    -> Tanh.write.in

Route 3:
LogReturns.done.out
    -> Square.write.in

Route 4:
Tanh.done.out
    -> Absolute.write.in

Route 5:
Tanh.done.out
    -> Atanh.write.in

Route 6:
Tanh.done.out
    -> ZScore.write.in

Route 7:
Square.done.out
    -> Mean.write.in

Route 8:
Square.done.out
    -> ZScore.write.in
```

`Extract.path = "last"` is compiled into Extract's static argument template.

At runtime none of these route names need to be resolved again.

---

# 31. Example execution

One source observation enters the active program.

```text
Source.next()
    |
    v
WireMeasurement
    |
    v
Extract.write(in=measurement, path="last")
WaitStreaming()
Extract.done() -> out=price
    |
    v
LogReturns.write(in=price)
WaitStreaming()
LogReturns.done() -> out=return
    |
    +----------------------+
    |                      |
    v                      v
Tanh.write(in=return)   Square.write(in=return)
...                    ...
```

Independent branches may execute concurrently.

After all terminal work for the evaluation completes, the runtime admits the next source observation.

---

# 32. Testing requirements

Tests must prove the architecture, not merely object construction.

## 32.1 Primitive protocol

For `Add`:

```text
Write(a=1,b=2)
WaitStreaming()
Done()
```

must return `out=3`.

Then a second evaluation:

```text
Write(a=4,b=5)
Done()
```

must return `out=9` with no dependency on the previous result.

## 32.2 Schema-derived ports

Compile a graph using `Add.a`, `Add.b`, and `Add.out` and prove those ports came from Cap'n Proto schema metadata rather than a handwritten descriptor.

## 32.3 Type mismatch

Compile `Text -> Add.a(Float64)` and assert compile failure before capability activation.

## 32.4 Static input

Compile `Extract.path = "last"` as a static typed argument and verify execution does not parse JSON again.

## 32.5 Fan-out

One result connected to multiple consumers must feed every consumer.

## 32.6 Fan-in

A multi-input node must not execute until every required dynamic/static argument is ready.

## 32.7 Recursive definition

Compile the real root graph with nested `definition:*` nodes.

## 32.8 Hot recompile success

Run Program A, compile Program B while A remains active, swap at the quiescent boundary, and verify subsequent observations use B.

## 32.9 Hot recompile failure

Run Program A, attempt invalid Program B, and verify A remains active and unchanged.

## 32.10 Capability reuse

Reconnect an unchanged node and verify the candidate reuses the same capability identity via an additional reference.

## 32.11 Store preservation

Populate a Store, perform a topology-only recompile preserving node identity/configuration, and verify the Store state survives.

## 32.12 No `any`

Add an AST architecture test over compiler/runtime ownership that rejects `any` / `interface{}` in graph execution structures and APIs.

## 32.13 No generated execution glue

Add an architecture test preventing generated setters, downstream binders, per-primitive invokers, or giant primitive switches.

---

# 33. Migration plan

Do not perform another repository-wide mechanical rewrite before the execution core is proven.

## Stage 1 — Freeze the primitive protocol

Migrate only a tiny vertical slice:

```text
Source fixture
Atanh
Add
terminal fixture
```

Use:

```text
write -> stream
done  -> typed result + reset
```

No sinks. No downstream fields. No `any`.

## Stage 2 — Build the tiny constructor registry

Implement:

```text
string -> Factory{InterfaceID, New}
```

Nothing else.

Delete per-primitive runtime descriptors.

## Stage 3 — Build schema compilation

Given a capability interface ID:

- resolve `write`;
- resolve `done`;
- resolve input fields;
- resolve output fields;
- resolve Cap'n Proto types;
- compile field-copy operations.

No primitive-specific code.

## Stage 4 — Build immutable Program

Compile nodes, static argument templates, readiness masks, routes, and execution dependencies.

Prove `Atanh -> Add` executes.

## Stage 5 — Add fan-out and fan-in

Prove real branch topology.

## Stage 6 — Add candidate recompilation and quiescent swap

Prove good edits activate and bad edits do not disturb the running program.

## Stage 7 — Add capability reuse

Reuse unchanged nodes by stable node identity plus construction fingerprint.

Prove explicit Store state survives topology-only edits.

## Stage 8 — Restore recursive definitions

Compile `definition:*` graphs into the same immutable Program.

## Stage 9 — Migrate the real source boundary

Use the real market-data capability and remove test ingress from production.

## Stage 10 — Migrate remaining primitives

Only after the runtime has proven the protocol and hot-recompile behavior.

Each migrated primitive should become simpler:

```text
write inputs
compute / accumulate ephemeral result
done returns result and resets
```

Delete old downstream callback fields as each primitive migrates.

Do not preserve parallel execution paths.

---

# 34. Things to delete from the current migration

When replaced, remove:

```text
StreamNode
WriteAny
SetDownstreamAny
NewStreamNode
Float64Sink for ordinary node composition
Int64Sink for ordinary node composition
TextSink for ordinary node composition
BoolSink for ordinary node composition
DataSink for ordinary node composition
Broadcast*Sink used only for graph fan-out
InvocationAssembler designs that accept generic Go values
server fields retained by compiler
BindDownstream
CreateInputSink
CreateSetter
CreateStaticSetter execution glue
per-primitive Invoke callbacks
generated per-primitive registry execution code
primitive-specific switches in registry generation
fake production Float64 source
```

Do not delete a capability interface if it has a genuine boundary use unrelated to ordinary graph composition.

The architectural rule is not "Sink is a forbidden word".

The rule is:

> Ordinary node-to-node graph composition is request/result routing performed by the compiled Program, not callback/sink wiring embedded into primitives.

---

# 35. Definition of done

The migration is complete when all of the following are true.

1. The current Flume JSON compiles directly into an in-memory `Program`.
2. No Go application code is generated from that graph.
3. The compiler retains no concrete primitive servers.
4. The runtime graph data path contains no `any` / `interface{}`.
5. Input ports come from `write` schema fields.
6. Output ports come from `done` result fields.
7. Ordinary primitives have no downstream graph references.
8. `Done()` returns the current result and resets ordinary primitives.
9. Persistence is represented by explicit Store composition.
10. Fan-in and fan-out are compiled route behavior.
11. Execution uses pre-resolved fields and numeric topology rather than JSON interpretation.
12. Nested definitions compile recursively in memory.
13. A Flume edit can compile a candidate while the old program runs.
14. Invalid edits leave the active program untouched.
15. Valid edits replace the active program at a deterministic evaluation boundary.
16. Unchanged capabilities can be reused across recompiles.
17. The real production source feeds the compiled program.
18. UniConn exists only beneath Cap'n Proto RPC at real transport boundaries.
19. There is no generated shadow execution layer around Cap'n Proto.
20. The runtime is, in practical terms:

```text
Flume JSON
    |
    v
in-memory compiler
    |
    v
immutable Program
    |
    +--> capnp.Client
    +--> compiled method descriptors
    +--> compiled field descriptors
    +--> static argument templates
    +--> readiness masks
    +--> numeric route table
    |
    v
Cap'n Proto execution
```

That is the architecture.
