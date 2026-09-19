# SYMM / nomagique — Master Spec

## Goal

Make the entire SYMM system constructible from JSON definitions composed exclusively from `nomagique` primitives.

The core idea is simple:

```go
system := nomagique.NewNumber(
    stage1,
    stage2,
    stage3,
)

system(input)
```

and recursively:

```go
system := nomagique.NewNumber(
    nomagique.NewNumber(
        stage1,
        stage2,
        stage3,
    ),
    nomagique.NewNumber(
        stage4,
        stage5,
        nomagique.NewNumber(
            stage6,
            stage7,
            stage8,
        ),
    ),
)

system(input)
```

JSON describes these compositions.

The compiler reads the JSON, constructs the referenced nomagique primitives, recursively composes them, and returns the executable composition **in memory**.

There is one generic compiler.

There is no generated Go implementation for each JSON file.

---

# 1. The fundamental runtime model

The execution atom is:

```go
type Value[T, U any] func(T) U
```

Sequential composition is:

```go
type Number[T any] types.Value[T, T]

func NewNumber[T any](
    stages ...types.Value[T, T],
) Number[T]
```

Everything should reduce to nomagique primitives composed together.

A signal is a composition.

A logic stage is a composition.

A strategy stage is a composition.

Transport is a composition.

Broker interaction is a composition.

The entire system is a composition.

There is no second application architecture layered above nomagique.

---

# 2. What "compile" means

This is the most important rule.

When this spec says:

> compile a JSON graph

it means:

> Parse the JSON, resolve each node against the nomagique constructor registry, instantiate the primitives, recursively compose them into `Value` / `Number` closures, and return that executable composition in memory.

Conceptually:

```go
pipeline, err := compiler.Compile(graph, registry, dependencies)
```

then:

```go
result := pipeline(input)
```

Compilation does **not** mean:

```text
foo.json → foo.go
bar.json → bar.go
system.json → system.go
```

Do not generate `.go` files for graphs.

Do not generate Go implementations for:

* signals
* logic
* strategies
* execution stages
* transports
* broker stages
* the system itself

The JSON is the program.

---

# 3. Remove the current generated application branch

The current repository has drifted into generating application code under:

```text
generated/
```

That is not the desired architecture.

Remove the generated Go implementations of JSON graphs.

In particular, the final system must not depend on:

```go
generated.NewTrainingSystem(...)
generated.NewSystem(...)
generated.NewSignals(...)
generated.NewLogic(...)
generated.NewExecution(...)
```

Likewise, `nomagique/compiler/codegen.go` must not generate application `.go` files from JSON.

Any useful validation or graph-reading logic inside that code may be reused inside the in-memory compiler.

The application code-generation path itself should go away.

---

# 4. The one allowed generated artifact: the primitive constructor registry

The nomagique source scanner already has the right broad purpose.

It scans `nomagique` and discovers graph-buildable primitives.

Conceptually:

```go
schemas, err := scan.Tree("nomagique")
```

From those schemas we may generate:

```text
nomagique/catalog/primitives.json
nomagique/compiler/registry.go
```

The registry exists because JSON contains names such as:

```text
statistic.ZScore
transport.Fan
cognition.Classification
```

and the compiler needs a way to turn those names into actual Go constructors such as:

```go
statistic.NewZScore()
transport.NewFan(...)
cognition.NewClassification()
```

That constructor registry may be generated from Go source.

That is very different from generating the application itself.

Allowed:

```text
Go primitive source
→ generated constructor registry
```

Not allowed:

```text
JSON graph
→ generated Go application
```

---

# 5. Build the primitive catalog from `nomagique`

Scan `nomagique` and every relevant subpackage.

The catalog should capture enough information to construct each primitive:

* operation name
* package
* constructor
* input type
* output type
* constructor arguments
* whether an argument is variadic
* configuration fields
* injected runtime dependencies where required

Do not manually maintain a second registry.

Do not hardcode special cases into the compiler when the scanner can describe them correctly.

If the scanner is missing information, improve the scanner/catalog.

---

# 6. Flume consumes the same catalog

The Flume editor should display what nomagique actually exposes.

The path is:

```text
nomagique source
→ catalog scan
→ primitive schemas
→ /workbench/primitives
→ buildFlumeConfigFromSchemas
→ Flume
```

Do not manually recreate available node types in the frontend.

If Flume needs metadata the catalog does not provide, add it to the catalog.

The Flume graph and compiler must agree on exactly the same node vocabulary.

---

# 7. Constructor configuration comes from JSON

A constructor argument is not a streamed port.

For example:

```go
NewCollect(batchSize int)
```

should be represented by node configuration:

```json
{
  "type": "transport.Collect",
  "inputData": {
    "_config": {
      "batchSize": 32
    }
  }
}
```

The compiler constructs:

```go
transport.NewCollect[SomeType](32)
```

once.

It does not configure the primitive on every input.

Likewise:

```go
NewHTTPRequest(method, url)
NewHTTPHeader(key, value)
NewHMACSHA512(secret)
NewWSConnect(endpoint)
```

must be constructible generically from configuration plus runtime dependencies.

The current registry generator skipping constructors with arguments must be fixed.

---

# 8. Runtime dependencies are not JSON configuration

Some constructor dependencies are live process resources.

Examples:

```text
context.Context
HTTP client
websocket dialer
credentials
store/catalog handles
other live resource owners
```

These should be injected into the compiler/factory environment.

They should not be serialized into JSON.

Secrets must never be embedded in graph definitions.

A node definition describes what primitive should exist.

The dependency environment supplies the live resources required to construct it.

---

# 9. JSON definitions recursively become executable stages

Every JSON definition compiles into an executable stage.

That stage can then be used inside another JSON definition.

For example:

```text
cvd_trade.json
→ compiled Value

hawkes_trade.json
→ compiled Value
```

Those may then participate in a higher-level signals graph.

That signals graph compiles into another `Value`.

That can then be composed with logic.

Logic can be composed with strategy.

Strategy can be composed into the root system.

No generated Go file is needed between those levels.

---

# 10. Do not manually register compiled stages

The current special registrations such as:

```text
pipeline.Signals
pipeline.Logic
pipeline.Execution
```

are not the desired architecture.

Do not maintain a handwritten pipeline registry.

A JSON definition should itself become an available composable definition.

If a graph references another graph, the compiler recursively loads and compiles that graph.

For example:

```json
{
  "type": "definition:signals"
}
```

or whatever clean reference format fits the existing schema.

The exact naming scheme is implementation detail.

The important point is that subgraphs are resolved generically.

---

# 11. The compiler constructs composition once

The current `Builder.Compose()` has the correct high-level purpose:

```text
JSON
→ executable nomagique pipeline
```

Keep that idea.

However, it currently still walks the graph for every incoming datum using:

```go
state := make(map[string]any)
```

and an execution order.

That is still an interpreter.

The final compiler should inspect the graph once and build the composition once.

After compilation, one input should execute something equivalent to:

```go
compiled(input)
```

not:

```text
allocate state map
walk nodes
look up edges
dispatch node
repeat
```

The graph topology should disappear into the constructed closures.

---

# 12. Straight chains become `nomagique.NewNumber`

If the graph is:

```text
A → B → C
```

and those stages fit `Value[T,T]`, compile it to:

```go
nomagique.NewNumber(
    a,
    b,
    c,
)
```

Do not manually recreate sequential composition.

Use `Number`.

---

# 13. Topology is expressed by nomagique primitives

Fan-in, fan-out, branching, joining, routing, batching, etc. are not special runtime graph concepts.

They are nomagique primitives.

The repository already contains useful examples such as:

```text
Fan
Fork
Join
Route
Gate
Tee
Collect
Parallel
Broadcast
Grid
IO
Conn
```

Review and consolidate them where they overlap.

If JSON contains:

```text
        A
       / \
input       Join → D
       \ /
        B
```

the compiler should construct an appropriate composition involving `Fan`/`Fork` and `Join`.

It should not teach a runtime interpreter what fan-out means.

The rule is:

> Graph topology lowers into nomagique primitives.

---

# 14. Nested composition is the whole architecture

Conceptually the system should reduce to:

```go
signals := ...
logic := ...
strategy := ...
execution := ...

system := nomagique.NewNumber(
    signals,
    logic,
    strategy,
    execution,
)
```

where any one of those may itself be:

```go
nomagique.NewNumber(
    stage1,
    stage2,
    nomagique.NewNumber(
        stage3,
        stage4,
    ),
)
```

or contain topology primitives.

There should be no conceptual difference between a small pipeline and the whole system.

Only scale.

---

# 15. Signals remain JSON

The existing files under:

```text
signal/definitions/
```

are programs.

Keep them as JSON.

Compile them in memory.

Do not generate:

```text
generated/correlation_ticker.go
generated/cvd_trade.go
generated/hawkes_trade.go
...
```

A signal definition should be directly editable and immediately compilable.

---

# 16. Logic remains JSON

`logic.json` is a graph.

It should compile through the same compiler as a signal.

There should not be a handwritten:

```go
NewLogic()
```

implementation that knows where `logic.json` lives and manually wraps it.

Generic definition resolution should handle it.

---

# 17. Strategy and execution become graphs too

As the primitive vocabulary becomes sufficient, strategy and execution orchestration should also move into JSON.

But do not fake external behavior to accomplish this.

For example, current experimental primitives that fabricate:

```text
BTC/USD
FILLED
```

are not acceptable representations of real broker execution.

A primitive must either:

* perform a real transformation; or
* interact with a real injected external capability.

Never fabricate successful external effects.

---

# 18. `nomagique` remains domain-neutral

`nomagique` is not a trading library.

No primitive should fundamentally mean:

```text
BTC
Kraken
long position
market order
trade filled
```

Generic concepts are appropriate:

```text
HTTP
websocket
authentication
state
gate
route
request
response
source
sink
store
statistics
learning
transport
```

SYMM-specific market behavior is built by composing generic primitives.

---

# 19. HTTP primitives must be genuinely generic

The generic transport layer should provide reusable pieces such as:

```text
HTTP request construction
method
URL
path
query parameter
header
body
execute
response extraction
```

Those should work for arbitrary HTTP APIs.

They must not secretly implement Kraken rules.

---

# 20. Authentication primitives must be genuinely generic

Expose atomic reusable operations such as:

```text
nonce
timestamp
SHA256
SHA512
HMAC
base64
bearer auth
header auth
query auth
```

Protocol-specific signing should be a composition of those primitives.

For example, avoid a generic-looking:

```go
NewSigner(...)
```

that secretly knows:

```text
API-Key
API-Sign
Nonce
Kraken payload construction
```

If Kraken requires those exact steps, express Kraken signing as a JSON composition built from the generic auth primitives.

---

# 21. Websocket primitives must be genuinely generic

Provide reusable capabilities such as:

```text
connect
read
write
close
ping/pong
message encode
message decode
```

No-op primitives are not acceptable.

The current `NewWSWrite` must either genuinely write to a connection or be redesigned.

Do not claim a capability exists when it does not.

---

# 22. External protocols are compositions

Eventually:

```text
generic websocket primitives
+
generic JSON primitives
+
generic auth primitives
+
Kraken-specific message/config data
```

should compose into a Kraken connection.

The same generic primitives should also be capable of composing a completely unrelated websocket service.

Likewise for HTTP.

---

# 23. The root system JSON is just another graph

`system.json` is not a special language.

It should be compiled by the same compiler.

Today it may contain something simple like:

```text
signals
→ logic
→ execution
```

Eventually it should describe more of the actual system orchestration as suitable primitives become available.

But there must never be a separate "system compiler".

One graph compiler is enough.

---

# 24. The current handwritten root is the migration reference

The old root construction showed the actual relationships that ultimately need to become composition:

```text
catalog initialization

public websocket
private websocket
futures websocket

API

broker desk
trader

feeds → pipeline

hub
WebRTC

evaluations → UI

instrument
price
balance

subscription
initialization
readiness
```

Do not copy this into a generated Go file.

Use it as the behavioral reference when deciding which generic primitives/compositions are still missing.

---

# 25. Minimal bootstrap

The eventual root Go code should do little more than:

```text
load process config
create process context
construct runtime dependency environment
load root JSON definition
compile root definition
run it
```

Conceptually:

```go
definitions := signal.Definitions()

pipeline, err := compiler.Compile(
    "system",
    definitions,
    compiler.Registry,
    dependencies,
)
if err != nil {
    return err
}

return runtime.Run(ctx, pipeline)
```

Exact APIs are flexible.

No generated application package should be imported.

---

# 26. `runtime.System` remains the lifecycle mechanism

Do not invent another lifecycle framework.

Existing concepts such as:

```text
READY
BUSY
error handling
context cancellation
closers
```

remain the lifecycle substrate.

Where lifecycle sequencing is declarative, represent it through primitives/composition.

Do not hardcode domain-specific startup ordering into the compiler.

---

# 27. Unknown nodes are compilation errors

The compiler must not silently skip a node it cannot construct.

Compile errors include:

```text
unknown primitive
unknown graph definition
missing constructor config
invalid constructor config
missing runtime dependency
incompatible connection
cycle
invalid graph
```

If the graph cannot be faithfully instantiated, compilation fails.

---

# 28. Do not silently return fake defaults

Avoid behavior such as:

```text
missing stage → nil
invalid primitive → skip
failed external action → fake success
unsupported path → zero value
```

unless zero/nil is explicitly the mathematical semantic of the primitive.

Construction problems are errors.

External failures are real failures/results.

---

# 29. Stateful primitives are instantiated once

If a primitive owns state, the compiler constructs it once.

For example:

```text
running mean
variance
collector
learner
state store
```

must preserve state across repeated calls to the compiled graph.

Do not reconstruct primitives per input.

---

# 30. Improve the scanner rather than guessing

The current scanner has started discovering things like:

```text
constructor parameters
statefulness
injected dependencies
```

Good direction.

But do not infer architectural semantics from fragile heuristics such as:

```go
len(function.Body.List) > 1
```

to decide whether something is stateful.

Use real type/source information or explicit metadata where required.

The catalog must describe the primitive accurately.

---

# 31. The frontend should receive the real catalog

`/workbench/primitives` should expose actual schema information.

Do not reduce it to:

```json
{
  "inputs": [],
  "outputs": []
}
```

The frontend already builds Flume nodes from schemas.

Feed it the real schemas.

The UI and compiler should therefore share the same source of truth.

---

# 32. The graph schema should stay simple

Do not add a complicated IR unless there is an actual need.

The existing graph concepts are already sufficient at a high level:

```text
id
type
connections
inputData / config
subgraphs where applicable
```

The compiler's job is to lower those into nomagique composition.

Keep the JSON understandable and editable by Flume.

---

# 33. Do not introduce another runtime graph engine

This includes avoiding a "temporary" interpreter as the final architecture.

A builder that:

```text
walks every node
maintains per-call state maps
dynamically routes values through edges
```

is useful as a prototype but is not the desired endpoint.

Compilation should produce direct closure composition.

The runtime executes the composition.

---

# 34. Future dynamic editing is a design constraint

Later, Flume should be able to change a running system.

The desired future flow is:

```text
edit graph in Flume
→ save JSON
→ compile new graph in memory
→ validate
→ swap compiled Value
```

not:

```text
edit graph
→ generate Go
→ rebuild binary
→ restart
```

Do not solve hot-swapping now.

But do not make architectural decisions that prevent it.

---

# 35. The future swappable unit is a compiled composition

A JSON definition should compile into a self-contained executable object.

That makes the future replacement unit conceptually:

```text
old compiled Value
        ↓
atomic replacement
        ↑
new compiled Value
```

State migration and safe handoff are future concerns.

The important part today is that compilation already happens entirely in memory.

---

# 36. Immediate implementation direction

Work in this order:

```text
A. Delete the per-graph Go-generation path.

B. Remove `generated/*` application implementations.

C. Restore root startup to runtime compilation of system JSON.

D. Keep and improve the nomagique source scanner.

E. Keep and improve constructor registry generation.

F. Make constructor registry support configured constructors.

G. Replace handwritten `pipeline.Signals/Logic/Execution`.

H. Add generic graph-definition resolution.

I. Rework Builder.Compose into a real compile-once lowering step.

J. Lower straight chains into NewNumber.

K. Lower topology into nomagique topology primitives.

L. Compile existing signal JSON definitions directly.

M. Compile logic JSON directly.

N. Compile execution/strategy JSON directly.

O. Compile system JSON recursively.

P. Continue extracting generic transport/auth/broker capabilities until the handwritten root collapses naturally.
```

---

# 37. Required tests

Tests should prove at least:

```text
linear JSON graph compiles into executable composition

nested JSON definition compiles recursively

configured constructor receives correct values

stateful primitive is instantiated once

Fan/Fork executes all branches

Join combines branch output

Route selects the intended branch

compiled child definition works inside a parent

unknown primitive fails compilation

missing config fails compilation

cycle fails compilation

missing runtime dependency fails compilation

system JSON compiles in memory

repeated system execution does not reparse/reconstruct graph

compiling JSON creates no .go files
```

---

# 38. Definition of done

The architecture is correct when the system path is:

```text
nomagique Go primitives
        ↓
catalog scan
        ↓
constructor registry
        ↓
JSON / Flume graph
        ↓
generic in-memory compiler
        ↓
nested nomagique Values / Numbers
        ↓
execute
```

and there is no:

```text
JSON
→ generated application Go
```

layer.

---

# 39. The rules to remember

> `Value[T,U]` is the atom.

> `Number` is sequential composition.

> Fan-in, fan-out, routing, joining, batching, etc. are nomagique primitives.

> JSON describes composition.

> Flume edits JSON composition.

> The registry maps JSON node names to real constructors.

> Compilation constructs the closures once, in memory.

> Execution calls those closures.

> A compiled JSON definition can itself be composed.

> The system graph is not special.

> Do not generate Go files for graphs.

> nomagique is general-purpose.

> SYMM is simply one graph built with it.

> The graph is the program.
