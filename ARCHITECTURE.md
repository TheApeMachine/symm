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

---

Listed directory fanapp
Searched for "FlumeConfig"
Searched for "flume"
Listed directory frontend
Searched for "FlumeConfig"
Viewed build-config-from-schemas.ts:60-120
Viewed build-config-from-schemas.ts:1-50

Look at that JSON graph—that is the exact, canonical Flume runtime format:

```json
"connections": {
    "inputs": {
        "in": [ { "nodeId": "16fbf205...", "portName": "value" } ]
    }
},
"inputData": {
    "open": { "open": true }
}
```

Notice the crucial pattern Flume uses:
1. **When a port is wired:** Flume populates `connections.inputs[portName]`.
2. **When a port is unwired:** Flume populates `inputData[portName]` with the control's value (e.g. `{ "open": true }` or `{ "string": "GET" }`).

Here is how making `method` and `rawURL` into `types.Value` bridges Flume's JSON directly into Go:

---

### 1. Port Types in Go Mirror Flume Port Types

In Flume, you define ports like:
```ts
config
  .addPortType({ type: "string", ... })
  .addPortType({ type: "number", ... })
  .addPortType({ type: "map",    ... })
```

In [nomagique/types/value.go](file:///Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/types/value.go), the Go port types are simply closures that produce those types from the incoming frame:

```go
type Value[T, U any] func(T) U

type String  types.Value[any, string]
type Integer types.Value[any, int]
type Float   types.Value[any, float64]
type Boolean types.Value[any, bool]
type Map     types.Value[any, map[string]any]
type Any     types.Value[any, any]
```

A `types.String` is simply: *a function that, given the current context `in`, yields a `string`*.

---

### 2. Constructor Arguments ARE the Input Ports

Look at how `NewHTTPRequest` is declared in Go:

```go
func NewHTTPRequest(
    method types.String,
    rawURL types.String,
) types.Map {
    cc := client.New()

    return func(in any) map[string]any {
        // Sample the input ports against the incoming frame:
        m := "GET"
        if method != nil {
            m = method(in)
        }

        u := ""
        if rawURL != nil {
            u = rawURL(in)
        }

        // Execute request using Fiber client
        ...
        return out
    }
}
```

Look at the beauty of this constructor signature:
* `method types.String` $\rightarrow$ Flume Input Port: `"method"` (`type: "string"` with `Controls.text`)
* `rawURL types.String` $\rightarrow$ Flume Input Port: `"rawURL"` (`type: "string"` with `Controls.text`)
* Return type `types.Map` $\rightarrow$ Streaming node with Input Port `"in"` (`map`) and Output Port `"out"` (`map`)

The Go AST of the constructor **directly defines the Flume node schema**. There is zero duplication.

---

### 3. How the Compiler Resolves Both Cases

When the compiler instantiates `NewHTTPRequest`, it resolves each argument port by inspecting Flume's JSON:

```go
func resolveStringPort(node FlumeNode, portName string, registry Registry) types.String {
    // 1. Is the port wired to an upstream node?
    if wires := node.Connections.Inputs[portName]; len(wires) > 0 {
        upstreamClosure := registry.Get(wires[0].NodeID)
        return func(in any) string {
            return fmt.Sprint(upstreamClosure(in))
        }
    }

    // 2. Is the port unwired with a control value in inputData?
    if controlData, ok := node.InputData[portName]; ok {
        // e.g. inputData["method"] = { "string": "GET" }
        staticVal := extractControlValue(controlData)
        return func(any) string {
            return staticVal
        }
    }

    // 3. Fallback / Default
    return func(any) string { return "" }
}
```

Look at what this accomplishes:
* If the user **types** `"https://api.kraken.com/..."` into Flume's text control, the compiler wraps it in a constant closure `func(any) string { return "https://api.kraken.com/..." }`.
* If the user **drags a wire** from an upstream URL-builder node, the compiler passes that upstream node's closure.
* `NewHTTPRequest` executes identically in both cases: `u := rawURL(in)`. It never knows or cares whether the port was wired or configured via a control.

---

### 4. Why This Works Everywhere

Every primitive with parameters follows this exact contract:

1. **`Gate` (from your JSON above):**
   ```go
   func NewGate(open types.Boolean) types.Any
   ```
   - In Flume, `open` has a checkbox control (`"open": { "open": true }`).
   - If unwired, it reads the static checkbox.
   - If wired, a logic / regime node controls whether the gate is open dynamically.

2. **`Process` (Shell node):**
   ```go
   func NewProcess(binary types.String, args types.Value[any, []string]) types.Any
   ```
   - If unwired: runs `kraken paper` from text controls.
   - If wired: can execute dynamic CLI commands generated by the graph.

3. **`EMA`:**
   ```go
   func NewEMA(alpha types.Float) types.Float
   ```
   - If unwired: fixed period (e.g. `0.05`).
   - If wired: adaptive alpha driven by market volatility.

This gives you a 1:1, mathematically closed mapping: **every Flume port is a `types.Value`, every node constructor takes its ports as `types.Value`, and every primitive composes into `nomagique.NewNumber`.**

## What the FlumeConfig Looks Like

Remember, the type of the port is something we control, and for Flume just means what can connect to what, it is not actually typed in any way on the frontend. So we need to have one PortType for any unique input and output type of nomagique primitives.

```js
import { FlumeConfig, Colors, Controls } from 'flume'

const config = new FlumeConfig()
config
  .addPortType({
    type: "string",
    name: "string",
    label: "Text",
    color: Colors.green,
    controls: [
      Controls.text({
        name: "string",
        label: "Text"
      })
    ]
  })
  .addNodeType({
    type: "string",
    label: "Text",
    description: "Outputs a string of text",
    inputs: ports => [
      ports.string()
    ],
    outputs: ports => [
      ports.string()
    ]
  })
```

So the idea is that any things like "config" are essentially individual input nodes. In Flume an input node that is not connected will have a manual control (text field, check box, select, etc.), but you can also connect any compatible output to it and it will be used instead.

## What a Flume Compatible JSON Looks Like

```json
{
    "s:local-default": {
        "versionKey": "66704a4f-87fb-4a20-b15a-b09609a6ebc0",
        "data": {
            "id": "local-default",
            "project_id": null,
            "schema_version": 1,
            "nodes": {
                "15e80914-aa01-45a1-944a-372c717d39f8": {
                    "id": "15e80914-aa01-45a1-944a-372c717d39f8",
                    "x": -737.2435848238741,
                    "y": -668.4491574642204,
                    "type": "gate",
                    "width": 300,
                    "connections": {
                        "inputs": {
                            "in": [
                                {
                                    "nodeId": "16fbf205-b374-460e-ab25-8e1fdebfefbf",
                                    "portName": "value"
                                }
                            ]
                        },
                        "outputs": {
                            "out": [
                                {
                                    "nodeId": "f9b5ea26-23e9-4bcf-90b0-324428be021c",
                                    "portName": "value"
                                },
                                {
                                    "nodeId": "c565c6d0-acc4-44bb-9345-8bc28cc53cef",
                                    "portName": "value"
                                },
                                {
                                    "nodeId": "ac95b659-64b5-42da-b773-85ee0572036c",
                                    "portName": "value"
                                },
                                {
                                    "nodeId": "aae2f1e8-dfd4-4005-9188-68fd11fb56fc",
                                    "portName": "value"
                                }
                            ]
                        }
                    },
                    "inputData": {
                        "in": {},
                        "open": {
                            "open": true
                        }
                    },
                    "height": 140
                },
                "f9b5ea26-23e9-4bcf-90b0-324428be021c": {
                    "id": "f9b5ea26-23e9-4bcf-90b0-324428be021c",
                    "x": -285.56630595957046,
                    "y": -787.0325601618924,
                    "type": "sink",
                    "width": 280,
                    "connections": {
                        "inputs": {
                            "value": [
                                {
                                    "nodeId": "15e80914-aa01-45a1-944a-372c717d39f8",
                                    "portName": "out"
                                }
                            ]
                        },
                        "outputs": {}
                    },
                    "inputData": {
                        "value": {}
                    },
                    "height": 93
                },
                "16fbf205-b374-460e-ab25-8e1fdebfefbf": {
                    "id": "16fbf205-b374-460e-ab25-8e1fdebfefbf",
                    "x": -1170.165538473203,
                    "y": -677.1452335181216,
                    "type": "source",
                    "width": 280,
                    "connections": {
                        "inputs": {},
                        "outputs": {
                            "value": [
                                {
                                    "nodeId": "15e80914-aa01-45a1-944a-372c717d39f8",
                                    "portName": "in"
                                }
                            ]
                        }
                    },
                    "inputData": {},
                    "height": 101
                },
                "c565c6d0-acc4-44bb-9345-8bc28cc53cef": {
                    "id": "c565c6d0-acc4-44bb-9345-8bc28cc53cef",
                    "x": -285.59653679535063,
                    "y": -665.0646556421777,
                    "type": "sink",
                    "width": 280,
                    "connections": {
                        "inputs": {
                            "value": [
                                {
                                    "nodeId": "15e80914-aa01-45a1-944a-372c717d39f8",
                                    "portName": "out"
                                }
                            ]
                        },
                        "outputs": {}
                    },
                    "inputData": {
                        "value": {}
                    },
                    "height": 93
                },
                "ac95b659-64b5-42da-b773-85ee0572036c": {
                    "id": "ac95b659-64b5-42da-b773-85ee0572036c",
                    "x": -281.5696911577668,
                    "y": -525.4673811691282,
                    "type": "sink",
                    "width": 280,
                    "connections": {
                        "inputs": {
                            "value": [
                                {
                                    "nodeId": "15e80914-aa01-45a1-944a-372c717d39f8",
                                    "portName": "out"
                                }
                            ]
                        },
                        "outputs": {}
                    },
                    "inputData": {
                        "value": {}
                    },
                    "height": 93
                },
                "aae2f1e8-dfd4-4005-9188-68fd11fb56fc": {
                    "id": "aae2f1e8-dfd4-4005-9188-68fd11fb56fc",
                    "x": -284.2594982047466,
                    "y": -396.6083207664436,
                    "type": "sink",
                    "width": 280,
                    "connections": {
                        "inputs": {
                            "value": [
                                {
                                    "nodeId": "15e80914-aa01-45a1-944a-372c717d39f8",
                                    "portName": "out"
                                }
                            ]
                        },
                        "outputs": {}
                    },
                    "inputData": {
                        "value": {}
                    },
                    "height": 93
                }
            },
            "comments": {},
            "viewport": {
                "scale": 0.745,
                "translate": {
                    "x": -393.47254491253625,
                    "y": -340.9731989710005
                }
            },
            "updated_at": "2026-09-19T16:18:20.450Z"
        }
    }
}
```

## GOAL

1. Make sure everything in nomagique is properly Value primitive compatible.
2. Make sure that things in nomagique are generic building blocks, and not application specifics (that should be defined in the JSON definitions as node inputs)
   TO MAKE THIS MORE CLEAR, CONSIDER THE FOLLOWING EXAMPLES:
   - Don't have a "WSSubscribe" node to do the Kraken Instrument subscriptions, instead have a WSMessage node you can configure and can be written to a generic WSConnection.
   - Don't have a Kraken CLI Paper node, instead have a Shell Execution node, with Command and Params inputs, that can be set in the JSON definition.
   - Don't have IcebergTable nodes with SYMM specific schema data, instead have generics IcebergTable storage (and related) nodes with the schema as inputs coming from the JSON definition files.
3. Make sure your JSON definitions are actualy correct, they could have been written when the system was still different because of misunderstandings! They live in signal/definitions.
3. Make sure that in root.go you compile one single JSON graph into dynamically built (nested) nomagique.Number pipelines (it is fine to have everything in separate JSON files, but in the end they will have to be combined into a single graph).
4. Make sure the top-level (system) pipeline can be executed and the system comes to life.
5. Make sure the Pipeline Editor can show the full graph in the Flume Nodegraph Editor.
6. Make sure that all other Frontend UI components are hooked up to real data again, and for newly introduced components that still use mocked/syntheically generated data, make sure those are hooked up to real data too.

/Users/theapemachine/go/src/github.com/fanfactory/phoneapp <- I already built a system on this once, an API integration system with data transformation and various data store sinks.

https://flume.dev/docs/basic-config
https://flume.dev/docs/root-node
https://flume.dev/docs/logic-nodes
https://flume.dev/docs/saving-nodes
https://flume.dev/docs/dynamic-nodes
https://flume.dev/docs/NodeEditor
https://flume.dev/docs/flume-config
https://flume.dev/docs/controls

Please do not allow yourself to be lazy, take shortcuts, or allow "fake tests" to be green. We have to make this work now, and we need real tests that are seriously testing things thoroughly, including adverserial scenarios, edge cases, memory leaks, race conditions, etc. 

Now, I need you to understand something, I have no more time, or patience to wait on this. It has been MONTHS. You need to get this finished, and stop being lazy and just chase lint errors, or trust in your fake tests being green. Time is UP!!