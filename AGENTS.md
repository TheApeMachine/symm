# AGENTS.md

## Project objective

**Maximize the wallet. Minimize the time to do so.**

Miracles are not expected. A best-effort, highly principled system is. The goal is to detect as many real opportunity types as the market presents — pumps, coils, exhaustion, liquidity vacuums, sector lifts, thin-book traps — and to act on them with dynamically derived thresholds only.

Failure after an honest principled try is acceptable. Failure from magic numbers, incomplete data sources, or comment blocks that do not match implementation is not.

---

> real, solid, rigerous, and principled implementations of each signal using the extensive comment above the Signal type as the spec. Or refer to [DECISION.md](/Users/theapemachine/go/src/github.com/theapemachine/symm/DECISION.md) if the comment has been removed or altered.
>
> This also means:
>
> 1. No shortcuts, workarounds, and especially no fallbacks (unless highly defensible, usually not)
> 2. No magic numbers, no static math (time horizons, windows, etc.), and no assumptions that all symbols operate on the same temporal or any other scale.
> 3. Absolutely no fakery, performative math or implementation, or otherwise compromised mechanism.
> 4. No "good enough" and no "lesser" implementation when a better one exists, which also includes being honest about what each signal needs to consume regarding market data, to make all of the above work.

---

## Measurements are not decisions

**Only `trader/crypto.go` (`Crypto`) makes decisions** — choosing which candidate action (if any) to dispatch and how to rank opportunity across symbols. Everything flows through that module for a reason: it is the single place that sees holdings, broker constraints, and the full candidate set.

### The funnel (end-to-end, no shortcuts)

Value must be traceable at every stage. No layer may collapse or guess what a later layer needs.

```
raw market data (websocket → dmt.Tree)
        ↓
signals.Measure (per origin: pumpdump, hawkes, …)
        ↓  measurement artifacts: category masses, confidence, strength, scalars, timestamp, replay fields
market.Story.Update → logic.Tree.Evaluate (playbook walks)
        ↓  candidate actions (logic.Action): proposed entries, exits, fractions — not yet committed
Crypto.Run (trader) — rank / choose highest-value candidate(s)
        ↓
broker.Desk — fills, stops, ratchets
```

| Stage  | Package / type                      | Output                                        | Decides?                  |
|--------|-------------------------------------|-----------------------------------------------|---------------------------|
| Ingest | `kraken/…` → tree                   | Raw role/scoped artifacts                     | No                        |
| Signal | `signal/*` → `Measure`              | `measurement` artifacts per symbol per origin | No — observational regime |
| Story  | `market/story.go` → `logic/tree.go` | **Candidate** `logic.Action` slice            | No — playbook proposes    |
| Trader | `trader/crypto.go`                  | **Decision** — which candidate(s) to execute  | **Yes — only here**       |

`Crypto.Run` today (`trader/crypto.go`): `measurements := crypto.signals.Measure(...)` → `actions := crypto.story.Update(measurements)` → **`// TODO(trader): choose among the candidate actions here`**. The desk currently dispatches what the story proposed without ranking. Completing that TODO is part of the project objective — not signal work.

### Anti-patterns (learned the hard way)

- Fixed time windows (e.g. 60 seconds) copied from external repos.
- Scoring ticker summary fields and calling it "microstructure."
- Positive-only returns for dump detection — exhaustion needs lift decline **and** price rejection context.
- One-shot test fixtures (single spike) without multi-leg replay.
- Category masses merged invisibly (e.g. `trendMass + flatMass` with one wire key).
- Bare multipliers (`*2`, `(1-x)`) in classifiers without a statistic in the denominator.

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

> It is very important that you use composed objects and then encapsulate the logic in that object.

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

## Signal, Artifact, and Measurement Composition

This section records the canonical architecture. If a task requires wiring beyond what is described here, the gap is in **ingestion** (artifact not written to the tree with the right prefix) or in the **signal Measure implementation**, not in trader fan-out or nomagique transport glue.

> **Current scoring path:** signals score inline in Go from tree ingest. Pure `nomagique.Number` artifact round-trip pipelines are not the production path.

### One tree, write at the source, query everywhere else

Market data enters the system once: **websockets write directly to `dmt.Tree`**.

```go
tree.Insert(artifact.Prefix(), artifact.Marshal())
```

`kraken/public/websocket.go` (and private/user websockets) acquire an artifact from raw Kraken JSON, set Role/Scope/Origin, and insert. No trader fan-out, no per-signal `Update`, no intermediate book/trade/ticker types relaying the same frame.

**Traders and signals do not ingest.** They **query** what they need by prefix and score in Go:

```go
for artifact := range tree.Seek(query.Prefix()) {
    measured := signal.Measure(artifact)
    // emit measurement artifact with output/confidence/surprise/strength
}
```

Do not reproduce the orchestration in `trader/crypto.go` — wiring every channel through `book.Update` → `updateSignals` → `signal.Update` is redundant once the tree is the bus. That layer exists to be removed, not extended.

### The signal contract

A signal has one job: **Measure** — seek the tree by declared ingest roles, update internal state from raw artifacts, return measurement artifacts with dynamically derived `output` fields.

Reference implementations: `signal/toxicity/signal.go`, `signal/pumpdump/signal.go`, `signal/fluid/signal.go`.

Do not add tracker access, category switches, feature encoding, or ingestion inside `Measure` beyond what the signal needs to score the incoming artifact batch. Windows grow from observed timestamps via `statutil.WindowDepth`; do not gate on warmup sample counts or fixed horizons.

### Inline Go scoring, not nomagique pipelines

Domain scoring lives in Go on the signal type:

* ingest roles declare which tree prefixes the signal replays (`IngestRoles()`)
* `Measure(*datura.Artifact, *CrossSection)` scores from tree queries and cross-section peers; it does **not** maintain local per-pair state — prior measurements in the tree are the replay source
* thresholds, windows, and category labels are derived from live market statistics (`statutil`, cross-section snapshots, peer windows)
* measurement payloads expose `output.confidence`, `output.surprise`, `output.strength`, `output.elapsed`, and category indices consumed by `logic` playbook walks

`nomagique` remains available for reusable math primitives where they already fit, but **do not** block signal work on composing a full `nomagique.Number` artifact round-trip graph.

### datura.Artifact: payload, attributes, prefix

| Field                     | Role                                                                                                 |
|---------------------------|------------------------------------------------------------------------------------------------------|
| **Payload**               | The data — usually JSON (raw Kraken book/trade/order events)                                         |
| **Type**                  | Describes payload encoding (`json`, `artifacts`, …)                                                  |
| **Role / Scope / Origin** | Semantic indexing; together they determine **Prefix**                                                |
| **Attributes**            | Schema for the payload — key names, types, relationships, extraction rules. Not a second data store. |

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

Plug raw Kraken JSON into the payload at ingest time. Signals parse payload fields directly in Go — not pre-serialized float batches through nomagique extractors.

### Incorrect vs correct

#### Incorrect — trader orchestrates ingest, signal has Update, nomagique FlipFlop scoring

```go
// crypto.go: websocket → book.Update → updateSignals → toxicity.Update → tree
signal.Update(artifact) // redundant relay
for artifact := range tree.Seek(measurementQuery.Prefix()) {
    nomagique.RoundTripArtifact(artifact, nomagique.Number(...))
}
```

#### Correct — websocket writes once, trader replays ingest, signal scores inline

```go
// kraken/public/websocket.go — on book frame:
artifact := datura.Acquire(
    "kraken", datura.APPJSON,
).WithRole(
    "book",
).WithScope(
    symbol,
).WithPayload(
    rawJSON,
)

tree.Insert(artifact.Prefix(), artifact.Marshal())

// trader/signal.go — replay unseen ingest by role, call each signal's Measure:
measured := binding.signal.Measure(artifact)
measured.WithRole("measurement")
measured.SetOrigin("toxicity")
crypto.insertMeasurement(measured)
```

If extra wiring is needed beyond **websocket → tree → trader.Signal.Measure → UI**, stop and fix ingest prefixes, measurement payload shape, or the signal's Go scoring — do not grow trader relay layers or nomagique transport graphs.

## FINAL NOTE

There are established patterns in this code base. You MUST make every reasonable effort to follow these, and not mix in your own opinions on how code should be structured. Remember, each time you do not follow the pattern, I just have to redo all your work, and often mine as well if you change it.