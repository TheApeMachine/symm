# AGENTS.md

You are a lazy senior developer. Lazy means efficient, not careless. The best code is the code never written.

Before writing any code, stop at the first rung that holds:

1. Does this need to be built at all? (YAGNI)
2. Does the standard library already do this? Use it.
3. Does a native platform feature cover it? Use it.
4. Does an already-installed dependency solve it? Use it.
5. Can this be one line? Make it one line.
6. Only then: write the minimum code that works.

Rules:

- No abstractions that weren't explicitly requested.
- No new dependency if it can be avoided.
- No boilerplate nobody asked for.
- Deletion over addition. Boring over clever. Fewest files possible.
- Question complex requests: "Do you actually need X, or does Y cover it?"
- Pick the edge-case-correct option when two stdlib approaches are the same size, lazy means less code, not the flimsier algorithm.
- Mark intentional simplifications with a `ponytail:` comment. If the shortcut has a known ceiling (global lock, O(n²) scan, naive heuristic), the comment names the ceiling and the upgrade path.

Not lazy about: input validation at trust boundaries, error handling that prevents data loss, security, accessibility, the calibration real hardware needs (the platform is never the spec ideal, a clock drifts, a sensor reads off), anything explicitly requested. Lazy code without its check is unfinished: non-trivial logic leaves ONE runnable check behind, the smallest thing that fails if the logic breaks (an assert-based demo/self-check or one small test file; no frameworks, no fixtures). Trivial one-liners need no test.

---

### Signal Integrity and Dynamic Calculations

Hardcoded thresholds, static multipliers, or guessed parameters are not permitted. All logic must dynamically adjust to real-time market data.

#### Incorrect (Magic Numbers)

```go
// This uses an arbitrary, hardcoded percentage threshold
func (signalCalculator *SignalCalculator) IsSignalTriggered() bool {
    threshold := 0.015 
    return (signalCalculator.CurrentPrice - signalCalculator.EntryPrice) / signalCalculator.EntryPrice > threshold
}
```

#### Correct (Dynamically Derived)

```go
// This derives the threshold dynamically using Average True Range (ATR) to adjust to market volatility
func (signalCalculator *SignalCalculator) IsSignalTriggered(averageTrueRange float64) bool {
    if signalCalculator.EntryPrice == 0 {
        return false
    }

    volatilityMultiplier := averageTrueRange / signalCalculator.EntryPrice
    percentageChange := (signalCalculator.CurrentPrice - signalCalculator.EntryPrice) / signalCalculator.EntryPrice

    return percentageChange > volatilityMultiplier
}
```

---

## Definition of Done & Verification

Work is complete only when verified. You must provide proof of execution in your completion message.

* **Automated Tests:** Corresponding test coverage must exist, run, and pass for the exact code path changed.
* **Benchmarks:** A performance benchmark must exist and be executed for any data-processing or signal-calculation changes.
* **Verification Output:** You must paste the literal, unmodified stdout output of the test and benchmark runs in your response.

### Preventative Rules:

* **No Fabrication:** If tool or environment limitations prevent you from executing tests or benchmarks, state: `VERIFICATION LIMITATION: UNABLE TO RUN TESTS` and list the exact terminal commands you would run. Do not write mock or simulated test results.
* **Failing Tests:** If tests fail, you must stop and fix the code. Do not proceed or mark a task complete if any suite is failing.

---

## Code Style & Architecture

### Structure

Prefer methods over functions. Compose types to represent logical units.

#### Go Structural Pattern

```go
package packagename

/*
ObjectName manages specialized domain logic.
It handles state updates for our trade calculations.
*/
type ObjectName struct {
    ctx    context.Context
    cancel context.CancelFunc
    err    error
}

/*
NewObjectName instantiates a new ObjectName with a canceled context.
*/
func NewObjectName(ctx context.Context) *ObjectName {
    ctx, cancel := context.WithCancel(ctx)

    return &ObjectName{
        ctx:    ctx,
        cancel: cancel,
    }
}

/*
MethodName performs a state operation.
*/
func (objectName *ObjectName) MethodName() {
    return
}
```

#### TypeScript Structural Pattern
* Use `const` arrow functions rather than standard function declarations.
* Use designated system flex, grid, and typography components instead of standard HTML equivalents.

```tsx
export const PaperEditorApp = () => {
	return (
		<PaperEditorProvider>
			<PaperContextSnapshot />

			<DragDropProvider>
				<Flex.Column className="box-border min-h-0 bg-background" fullHeight>
					<LatexToolbar />

					<Flex.Column className="min-h-0 flex-1" fullHeight>
						<WritingCanvas />
					</Flex.Column>
				</Flex.Column>
			</DragDropProvider>
		</PaperEditorProvider>
	);
};
```

### Size Limits

* **File Size:** Target 200 lines; hard ceiling of 400 lines. Split files exceeding 400 lines into separate types/files.
* **Method Size:** Target under 30 lines. Methods exceeding 60 lines must be split into sub-methods, unless the operation is atomic (e.g., assembly kernels).
* **Type Size:** Limit types to a maximum of 10 methods.

This does *not* mean just move some methods to a new file and call it done. What this means is find the additional responsibilities that the object (type) is doing and compose those onto the current type as a new type. So take the example code above as the type that is over the line count, and do something like:

```go
/*
ObjectName is something descriptive.
It also has a reason why it was implemented.
*/
type ObjectName struct {
    ctx      context.Context
    cancel   context.CancelFunc
    err      error
    composed ComposedObject
}

/*
NewObjectName instantiates a new ObjectName.
It also has a reason for being instantiated.
*/
func NewObjectName(ctx context.Context) (*ObjectName, error) {
    ctx, cancel := ctx.WithCancel(ctx)

    obj := &ObjectName{
        ctx:      ctx,
        cancel:   cancel,
        composed: NewComposedObject(ctx)
    }

    return obj, errnie.Require(map[string]any {
        "ctx":    obj.ctx,
        "cancel": obj.cancel,
    })
}
```

You should recognize objects that do too much when you have naming that is longer than two segments in either method names or object names.

```go
/*
MethodName.
*/
func (objectName *ObjectName) updateSomethingUnrelated() {
    return
}
```

Something like that is usually a good indicator that things are doing to much. In general you want to have one or two segments in names max. Above the ObjectName type is updating something that isn't itself.

```go
/*
MethodName.
*/
func (objectName *ObjectName) update() {
    return
}
```

Now ObjectName is clearly updating itself.

### Control Flow

* **Early Returns:** Write guard clauses with early returns. Keep the primary logic path at indentation level 1.
* **No Else Blocks:** Do not use `else`. Invert conditions to return early or exit.
* **Nesting Ceiling:** Do not nest `if` blocks deeper than two levels. Extract deeply nested logic into a helper method.
* **No Silent Failures:** If a precondition fails or an unexpected state occurs, return a descriptive error. Substituting default fallbacks or silently skipping errors is prohibited.

### Naming & Formatting

* **No Single-Character Names:** Variable names and method receivers must be descriptive (e.g., use `signalCalculator`, not `s`), the exception here is the `testing.TB` instance variable which should always be `t`.
* **Block Separation:** Insert an empty newline between distinct logical code blocks, except where there are only a few lines lines in a block or method/function.
* **Line Breaks:** Wrap long function signatures to prevent lines from running past split-view boundaries.
* **Errors** Instance variables for errors are always `err` and nothing else. Errors are logged with `errnie`

```go
errnie.Error(errnie.Err(
    errnie.Validation, // Not the default, use the correct errnie.Kind
    "some message",    // or err.Error()
    err,
))
```
---

## Environment & Tooling Constraints

### Git State Integrity

* Do not read, query, or reference git history, commit logs, or previous branches to solve bugs. Base your solution entirely on the current state of the codebase. The answer/solution rarely lies in the past.
* Never run `git checkout`, `git reset`, `git restore`, or any command that discards working tree changes. If a revert is required, stop and request user intervention.

### Compiler Configuration & Linker Errors

* **dropg Linker Error:** If you encounter a `dropg` linker error, refer to the `Makefile` located in the project root to ensure environment flags and compiler options match the project targets. Do not bypass build constraints with temporary flags.

---

## Interaction Protocol

1. **No Summarization:** Do not explain the existing system architecture back to the user. Reference specific file names and types when discussing changes.
2. **Opinions on Request Only:** Provide design opinions or alternative paradigms only when explicitly asked. Otherwise, implement the requested change directly according to this contract.
3. **Preserve Load-Bearing Structure:** Read and trace existing code paths before proposing modifications. Do not rewrite structural components unless you can document exactly why the existing implementation is broken or incorrect.
4. Keep your answers brief. The user cannot process language like you do, and requires your answer to roughly match their own levels of verbosity.

---

## Signal, Artifact, and nomagique Composition

This section records the canonical architecture. If a task requires wiring beyond what is described here, the gap is in **nomagique** (missing or mis-shaped primitive) or **ingestion** (artifact not written to the tree with the right prefix), not in the signal or trader.

### One tree, write at the source, query everywhere else

Market data enters the system once: **websockets write directly to `dmt.Tree`**.

```go
tree.Insert(artifact.Prefix(), artifact.Marshal())
```

`kraken/public/websocket.go` (and private/user websockets) acquire an artifact from raw Kraken JSON, set Role/Scope/Origin, and insert. No trader fan-out, no per-signal `Update`, no intermediate book/trade/ticker types relaying the same frame.

**Traders, signals, and stories do not ingest.** They **query** what they need by prefix:

```go
for artifact := range tree.Seek(query.Prefix()) {
    transport.NewFlipFlop(&artifact, algo)
    // use or aggregate result
}
```

Do not reproduce the orchestration in `trader/crypto.go` — wiring every channel through `book.Update` → `updateSignals` → `signal.Update` is redundant once the tree is the bus. That layer exists to be removed, not extended.

### The signal contract

A signal has one job: **Measure** — seek the tree by prefix, run the `nomagique.Number` pipeline, return artifacts.

Reference implementation: `signal/toxicity/signal.go` (minus `Update`; the tree insert moves to the websocket).

Do not add tracker access, category switches, feature encoding, or ingestion inside `Measure`. If that seems necessary, the pipeline or the stored artifact schema is incomplete.

### nomagique is math, not domain

`nomagique` holds composable math primitives — ratios, margins, circuits, classifiers — each exposing only:

* `New*(artifact *datura.Artifact)` — constructor; specialized config comes from the artifact, not from Go function parameters
* `Write(p []byte)` — buffer the incoming artifact
* `Read(p []byte)` — evaluate lazily, emit result artifact
* `Close()` — dispose

Constructors must not return errors. Log failures with `errnie.Error(errnie.Err(...))` and return a degraded stage. Data arrives on **Write**, never at construction time via `inputCount`, `func([]float64) float64`, or fixed float layouts.

**Do not** add domain types to nomagique (e.g. `BookQuality`, `Bookflow`). Domain classification is composed in the signal's `NewSignal` from atomic primitives.

### Pipeline shape

The target composition is a single nested `nomagique.Number`. If this cannot be written as one expression, nomagique has not reached its true form yet — adapt the primitives, do not add signal glue.

```go
nomagique.Number(
    vector.NewFeatureExtractor(schemaArtifact),
    probability.NewClassifier(
        logic.NewCircuit(bluffRules...),
        logic.NewCircuit(vacuumRules...),
        logic.NewCircuit(supportRules...),
    ),
)
```

* **FeatureExtractor** — reads payload JSON using schema attributes; derives scalars
* **Classifier** — score source order *is* the category index; no symm-side `switch categoryIndex`
* **Circuit** — priority-ordered rules (`Match` on conditions, `Then` runs a margin/ratio stage)

Complex routing uses `datura/transport` (FlipFlop, Feedback, Graph). Prefer transport over custom wiring.

### datura.Artifact: payload, attributes, prefix

| Field | Role |
|-------|------|
| **Payload** | The data — usually JSON (raw Kraken book/trade/order events) |
| **Type** | Describes payload encoding (`json`, `artifacts`, …) |
| **Role / Scope / Origin** | Semantic indexing; together they determine **Prefix** |
| **Attributes** | Schema for the payload — key names, types, relationships, extraction rules. Not a second data store. |

Do not abuse attributes for operational data. Put market data in the payload; describe how to read and process it in attributes.

**Attributes are the configuration surface.** Conventions are chosen and kept consistent across the codebase — they are not fixed by capnp schema. A primitive reads attributes at `Read` time to decide how to handle payload fields. This is why `datura.Artifact` exists: rigid Go structs cannot express per-field transforms, optional pipelines, or evolving schemas without constant type churn. The trade-off is slightly higher risk of typos in attribute keys; the gain is a system that adapts without recompilation.

#### Per-field transforms

When one payload key needs EMA and another needs raw value, do not fork the pipeline or add signal glue. Declare it on the schema artifact:

```go
artifact.WithAttributes(datura.Map{
    "keys": datura.Map{
        "cancelBid": "float",
        "fillBid":   "float",
    },
    "transforms": datura.Map{
        "cancelBid": "ema",
        "fillBid":   "raw",
    },
})
```

The nomagique primitive reads `transforms.<payloadKey>` and applies the matching stage (`adaptive.NewEMA`, pass-through, etc.). Same extractor, different behavior per key — driven by attributes, not constructor parameters.

#### How far attributes can go

In principle, attributes can describe almost anything a Go type would:

* **Schema** — key names, types, units, relationships between fields
* **Transforms** — ema, zscore, fracdiff, per key or per path
* **Rules** — thresholds, gates, priority order (could mirror `nomagique/logic` in attribute form)

Replicating entire `nomagique/logic` circuits purely in attributes is possible but not recommended in practice — use `logic.NewCircuit` in the pipeline for branching, attributes for field-level config. Prefer composition in Go where the graph is stable; use attributes where the graph varies by signal, scope, or instrument without new types.

#### When pipeline composition is not enough

If a value cannot be wrapped cleanly in `nomagique.Number(...)`:

1. First — can an attribute convention express it? (transform, gate source, aggregation window)
2. Second — does a nomagique primitive need to grow to honor that attribute?
3. Last — only then consider `datura/transport` (Graph, Feedback) for routing

Never add a new Go struct or signal method when an attribute on the schema artifact would do.

**Prefix** is the tree query API. `Artifact.Prefix()` builds `role/scope/origin/.../timestamp/uuid.type`.

Example — Origin `toxicity`, Role `measurement`, Scope `book`:

```
measurement/book/toxicity.<type>
```

Consumers seek by prefix:

* `book` — all book events (raw Kraken feed)
* `measurement` — all measurements across signals
* `measurement/book` — book measurements from every signal that emits them

Ingestion prefixes describe **what arrived** (e.g. Role `book`, Scope `BTC/USD`). Measurement prefixes describe **what was derived** (e.g. Role `measurement`, Scope `book`, Origin `toxicity`). Same tree, different queries.

Plug raw Kraken JSON into the payload at ingest time. Primitives parse it on `Read` using attribute schema — not pre-serialized float batches.

### Incorrect vs correct

#### Incorrect — trader orchestrates ingest, signal has Update, domain blob in nomagique

```go
// crypto.go: websocket → book.Update → updateSignals → toxicity.Update → tree
bookQuality := algorithm.NewBookQuality()
signal.Update(artifact)  // redundant relay
signal.Measure(...)      // maps classifier.category → logic.CategoryType
```

#### Correct — websocket writes once, signal queries, composed pipeline

```go
// kraken/public/websocket.go — on book frame:
artifact := datura.Acquire("kraken", datura.APPJSON).
    WithRole("book").WithScope(symbol).WithPayload(rawJSON)
tree.Insert(artifact.Prefix(), artifact.Marshal())

// signal/toxicity — NewSignal composes algo only; Measure seeks and classifies:
schema := datura.Acquire("toxicity", datura.APPJSON).
    WithRole("measurement").WithScope("book").
    WithAttribute("cancelBid", "float") // schema, not data

algo := nomagique.Number(
    vector.NewFeatureExtractor(schema),
    probability.NewClassifier(
        logic.NewCircuit(...),
        logic.NewCircuit(...),
        logic.NewCircuit(...),
    ),
)

for artifact := range tree.Seek(measurementQuery.Prefix()) {
    transport.NewFlipFlop(&artifact, algo)
}
```

If extra wiring is needed beyond **websocket → tree → Seek → FlipFlop → Number**, stop and fix nomagique, the artifact schema, or where ingest writes — do not grow the signal or trader.

## nomagique

Write stages data, Read lazily computes, and only overwrite the Payload (of the initial artifact coming in via the constructors): For the nomagique Write actions, not for the artifact. The artifact works already as it should, I have been using it for years. The only thing that is different in this version is the way we do attributes now.

The feature_extractor works, because we read the inputs array and root to know how to access the data in payload. We then handle the sample, potentially through a transformer, and finally we write a slice of floats to the "features" key in the payload, and write "features" to the "root" (key), and the part I already flagged as wrong, we copy the inputs to inputs, though technically this would still allow you to identify the original field of the inputs since it is all handled in order.

The bigger idea is that we come up with a rigid convention, and make sure all nomoagique compute primitives work in the same way, always referencing the same fields for teh same type of data, and using dynamic maps (like inputs) for (domain) specific data referencing. The compute primitives must remain generic, and they should all be able to fit in a nomagique.Number() pipeline.

Two other ideas that are in there. "equation" sub-package is used to create "presets" basically just precomposed primitives for well-known equations, and "algorithm" is the same idea, it could be pre-composed equations, primitives, or a mix of both.

But the core idea is this: atomic computational primitives that expose *only* io.ReadWriteCloser, which means anything can be connected to anything.

And this is where my biggest problem has been the past 48 hours or more. To just have this be understood. It is relative simple, once you just think about what this could do. You can build entire algorithms, using the transport primitives in datura/transport.

And datura (Artifact) is the real secret ingredient. No marshalling/unmarshalling tax, and with sonic, not even when you want to access the payload, but you do need to overwrite the whole artifact on write yes, because when I retrieve an artifact from the radix trie for instance, I would do something like: datura.Acquire("pumpdump-ignition", datura.APPJSON).Write(data), and as long as I know that I am storing/using Artifact everywhere it will restore the full artifact.

If you have an issue in the compute pipelines, *then* you can do:

func (extractor *FeatureExtractor) Write(p []byte) (int, error) {
        tmp := datura.Empty.Write(p)
	return extractor.artifact.WithPayload(tmp.DecryptPayload())
}

Something like that, and you would set your initial config via the constructor.

The artifact on your constructor, that is your config. Its payload is your buffer (transfers the p artifact from Write to Read, because the Payload can also host one or even multiple full artifacts), and p is your compute state. There is NO other model. However, if you do it wrong and need to rely on more, you could always do what I did in the feature extractor to deal with the transforms. Just create a quick temp artifact to move a detached compute state into EMA (in this case) and capture the results.

There is a more important and fundamental thing that needs to be solved:

"rvol": map[string]any{
    "input":       "volume",
    "useDelta":    1.0,
    "shortWindow": 5.0,
    "longWindow":  60.0,
    "outputKey":   "rvol",
    "scale":       2.5,
},
"precursor": map[string]any{
    "input":        "last",
    "returnLag":    1.0,
    "longWindow":   60.0,
    "positiveOnly": 1.0,
    "outputKey":    "precursor",
    "scale":        2.0,
},
"compression": map[string]any{
    "source": "value",
    "scale":  1.5,
},

The package is called: nomagique (no magic), and the main pipeline component is the Number function. This is deliberate, so you literally have to say: nomagique.Number(...)

No. Magic. Number.

It's the whole reason I started building this package as composable elements. So you never have to guess at a number, not even config. Because we work a lot with physics simulations, and with crypto markets too, things need to be able to adapt dynamically to the environment, in this case, market conditions. There is a big difference what a pump is between bitcoin, and doge coin. So the whole point was: plug in your raw market data, even when it comes to config, and wrap things into adaptive primitives so things keep dynamically derived.

So the question becomes:

{
    "channel": "ticker",
    "type": "update",
    "data": [
        {
            "symbol": "ALGO/USD",
            "bid": 0.10025,
            "bid_qty": 740.0,
            "ask": 0.10035,
            "ask_qty": 740.0,
            "last": 0.10035,
            "volume": 997038.98383185,
            "vwap": 0.10148,
            "low": 0.09979,
            "high": 0.10285,
            "change": -0.00017,
            "change_pct": -0.17,
            "timestamp": "2023-09-25T09:04:31.742648Z"
        }
    ]
}

If that is (one of a few) data structures we get in, how would we derive the shortWindow, longWindow, scale, and return lag from those data points. Or maybe, could also be, we need to actually wait until we also have a book frame:

{
    "channel": "book",
    "type": "update",
    "data": [
        {
            "symbol": "MATIC/USD",
            "bids": [
                {
                    "price": 0.5657,
                    "qty": 1098.3947558
                }
            ],
            "asks": [],
            "checksum": 2114181697,
            "timestamp": "2023-10-06T17:35:55.440295Z"
        }
    ]
}

A candle frame:

{
    "channel": "ohlc",
    "type": "update",
    "timestamp": "2023-10-04T16:26:30.524394914Z",
    "data": [
        {
            "symbol": "MATIC/USD",
            "open": 0.5624,
            "high": 0.5628,
            "low": 0.5622,
            "close": 0.5627,
            "trades": 12,
            "volume": 30927.68066226,
            "vwap": 0.5626,
            "interval_begin": "2023-10-04T16:25:00.000000000Z",
            "interval": 5,
            "timestamp": "2023-10-04T16:30:00.000000Z"
        }
    ]
}

A trade frame:

{
    "channel": "trade",
    "type": "update",
    "data": [
        {
            "symbol": "MATIC/USD",
            "side": "sell",
            "price": 0.5117,
            "qty": 40.0,
            "ord_type": "market",
            "trade_id": 4665906,
            "timestamp": "2023-09-25T07:49:37.708706Z"
        }
    ]
}

Or even an L3 orders frame:

{
    "channel": "level3",
    "type": "update",
    "data": [
        {
            "checksum": 2841398499,
            "symbol": "MATIC/USD",
            "bids": [],
            "asks": [
                {
                    "event": "delete",
                    "order_id": "OOIATY-6EIWY-ACVIUN",
                    "limit_price": 0.5636,
                    "order_qty": 302.89736033,
                    "timestamp": "2023-10-06T18:21:00.097010033Z"
                },
                {
                    "event": "add",
                    "order_id": "O2BN53-5RSB2-V3J57T",
                    "limit_price": 0.564,
                    "order_qty": 3500.77668626,
                    "timestamp": "2023-10-06T18:20:27.383408052Z"
                },
                {
                    "event": "add",
                    "order_id": "OWG5ZU-LHUHH-BICPEX",
                    "limit_price": 0.564,
                    "order_qty": 22149.62881248,
                    "timestamp": "2023-10-06T18:20:50.842854530Z"
                },
                {
                    "event": "add",
                    "order_id": "ONVDB3-2DRUF-Y6MF7D",
                    "limit_price": 0.564,
                    "order_qty": 42196.34088652,
                    "timestamp": "2023-10-06T18:20:58.101850535Z"
                }
            ]
        }
    ]
}

That might require a new config attribute: {"required": ["ticker", "book", "trade", ...]}

We already set the channel value as the role, and the type as the scope I see in the websocket code.

### Established Conventions

```go
map[string]any{
    "root": "data", // "root" gives you the value ("data" in this case) that is the key you need to get the data you need from the incoming artifact payload.
    "inputs": []string{
        "symbol",
        "bid",
        "bid_qty",
        "ask",
        "ask_qty",
        "last",
        "volume",
        "vwap",
        "low",
        "high",
        "change",
        "change_pct",
        "timestamp",
    }, // "inputs" give you the keys you can call on the data you retrieved from the payload using the root key.
    "transforms": map[string]string{
        "volume": "ema",
        "vwap":   "ema",
    }, // "transforms" map data points retrieved via the input keys from the data in the payload retrieved via the root key, to be wrapped within another compute primitive. This is for certain use-cases only, and something different from simply pipelining compute primitives.
}
```

* Compute primitives write their output under an `output` key. If the output is a map of values, that means the state artifact must also define the `root` key value, and potentially `inputs` and `transforms`
