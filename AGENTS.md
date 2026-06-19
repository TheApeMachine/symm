# AGENTS.md

This document defines how coding agents operate on this platform. It is a strict contract, not a style guide. Sections are ordered by execution priority.

---

## 1. Sequence of Operations (Before Writing Code)

When a task is assigned, follow these steps in order before modifying any files:

1. **Code Inventory:** Identify and name the exact files and types involved in the change.
2. **Refactoring Identification:** Explicitly state what can be removed or simplified before writing new code.
3. **Approach Evaluation:** Formulate three distinct solution approaches. Select the approach that maximizes execution performance and structural correctness.
4. **Scope Control:** Execute the literal request. Do not implement generalized abstractions, auxiliary helper files, or out-of-scope modifications.

---

## 2. Backend Implementation Contract

This platform is a cryptocurrency trading system integrated with the Kraken API.

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

## 3. Definition of Done & Verification

Work is complete only when verified. You must provide proof of execution in your completion message.

* **Automated Tests:** Corresponding test coverage must exist, run, and pass for the exact code path changed.
* **Benchmarks:** A performance benchmark must exist and be executed for any data-processing or signal-calculation changes.
* **Verification Output:** You must paste the literal, unmodified stdout output of the test and benchmark runs in your response.

### Preventative Rules:

* **No Fabrication:** If tool or environment limitations prevent you from executing tests or benchmarks, state: `VERIFICATION LIMITATION: UNABLE TO RUN TESTS` and list the exact terminal commands you would run. Do not write mock or simulated test results.
* **Failing Tests:** If tests fail, you must stop and fix the code. Do not proceed or mark a task complete if any suite is failing.

---

## 4. Code Style & Architecture

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

* **No Single-Character Names:** Variable names and method receivers must be descriptive (e.g., use `signalCalculator`, not `s`).
* **Block Separation:** Insert an empty newline between distinct logical code blocks.
* **Line Breaks:** Wrap long function signatures to prevent lines from running past split-view boundaries.

---

## 5. Environment & Tooling Constraints

### Git State Integrity

* Do not read, query, or reference git history, commit logs, or previous branches to solve bugs. Base your solution entirely on the current state of the codebase.
* Never run `git checkout`, `git reset`, `git restore`, or any command that discards working tree changes. If a revert is required, stop and request user intervention.

### Compiler Configuration & Linker Errors

* **dropg Linker Error:** If you encounter a `dropg` linker error, refer to the `Makefile` located in the project root to ensure environment flags and compiler options match the project targets. Do not bypass build constraints with temporary flags.

---

## 6. Interaction Protocol

1. **No Summarization:** Do not explain the existing system architecture back to the user. Reference specific file names and types when discussing changes.
2. **Opinions on Request Only:** Provide design opinions or alternative paradigms only when explicitly asked. Otherwise, implement the requested change directly according to this contract.
3. **Preserve Load-Bearing Structure:** Read and trace existing code paths before proposing modifications. Do not rewrite structural components unless you can document exactly why the existing implementation is broken or incorrect.

## 7. Final Checklist

1. **Always check nomagique, qpool, datura, and errnie** They give you a lot of nice primitives and abstractions to work with. Always prefer them over building things from scratch.

For example, which is an excellent, and correct way to use nomagique (always work from a `nomagique.Number`):

```go
nomagique.Number(
    statistic.NewPanel(),
    statistic.NewMedian(nil, nil),
    ladder,
    probability.NewClassifier(
        ladder.UpliftReading(),
        ladder.ContagionReading(),
        ladder.AssociationReading(),
        ladder.InterventionReading(),
    ),
    probability.NewTransitionSurprise(
        4, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
    ),
)
```

2. **Errors** Use `errnie` (example below). The variable for errors is `err` at all times and not anything else.

```go
// errnie.Error is logging, errnie.Err is our custom error type.
errnie.Error(errnie.Err(
    errnie.Validation, // This is *NOT* the default, use the correct errnie.Kind
    "some error message",
    err, // The original error, or nil if no err exist.
))
```

3. **Tests** Use Goconvey, and mirror the file names and method names, use nested BDD style, test meaningful things and add benchmarks at the bottom. The variable for testing.T is `t` and not `testingTB`
4. **Complexity** Has to be earned. No "helper" methods with just one line of code, no overly defensive programming, and no abstractions that require many hops to understand. Keep it simple first, then we will see if we want to abstract complexity away afterwards.

---

## 8. Signal, Artifact, and nomagique Composition

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