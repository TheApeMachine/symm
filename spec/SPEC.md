# Project spec

## Contents

- [Vision](#vision)
- [Authority](#authority)
- [Requirements](#requirements)
- [datura.Artifact](#daturaartifact)
- [Roadmap](#roadmap) (Phases 1–5, T1.1–T5.4)
- [Progress](#progress) (Phase 1 tracker)
- [Acceptance criteria](#acceptance-criteria)
- [Orchestrator log](#orchestrator-log)
- [Constraints](#constraints)
- [Open questions](#open-questions)

---

## Vision

**S.Y.M.M. (Shake Your Money Maker)** is a Kraken spot microstructure trading engine. Live market data flows through a fleet of adaptive classifiers (signals), each emitting categorized readings with finite **confidence** and temporal **surprise**. A decision layer walks YAML-encoded playbooks, and a paper or live desk allocates capital proportional to edge across the live cross-section.

The product goal (from `README.md`) is unchanged: measure microstructure honestly, gate entries and exits through explicit playbook theses, simulate execution with paper/live parity, and expose telemetry through a React dashboard at `ws://127.0.0.1:8765/ws`.

The **architectural goal** is `AGENTS.md` §8 — the canonical contract for this migration:

- **One tree** — Kraken websockets insert raw JSON into a shared `dmt.Tree` (it is already implemented as a singleton, no matter where you call dmt.NewTree, you will always get the same Tree instance) as `datura.Artifact` rows (`tree.Insert(artifact.Prefix(), artifact.Marshal())`).
- **Write once at ingest** — no trader fan-out, no per-signal `Update`, no intermediate book/trade/ticker relay types.
- **Measure only** — signals seek by prefix → `transport.NewFlipFlop` → one `nomagique.Number` pipeline → measurement artifacts.
- **Query everywhere else** — traders, stories, and cognitive memory read the tree; they do not ingest feeds.
- **No magic numbers** — thresholds derive from live market statistics via nomagique primitives and artifact attributes.

Reference implementations:

- **`signal/pumpdump/signal.go`** — `Measure` + tree seek + FlipFlop shape (target contract; measurement publish still needs T2.10 cleanup).
- **`signal/toxicity/signal.go`** — `InsertMeasurement` publish path via `feed.InsertMeasurement` (`feed` aliases `github.com/theapemachine/symm/signal`).

The `curious` branch is mid-migration **toward subtraction**, not toward new relay layers. Legacy orchestration (`trader/crypto.go` → `updateSignals` → `signal.Update`) and transitional packages (`signal/replay/`, `signal/codec/`, `signal/buffer/`, `signal/export.go`) are **debt to remove**, not targets to wire. **`signal/tree.go` stays** — `InsertMeasurement` / `InsertTreeArtifact` are the shared publish helpers (T2.10 trims nothing else there). Playbooks live in `logic/rules/tree.yml` (embedded via `logic/tree.go`). Boot targets four goroutines in `cmd/root.go`: `kraken/public`, `kraken/paper`, `trader.Crypto`, `ui.Hub`.

### Current state (code snapshot)

Checked against on-disk sources after roadmap expand (run 6). **Phase 1 not started** — all T1.x still open.

| Area | Today | Target (§8 / Roadmap) |
|------|--------|------------------------|
| Boot | `cmd/root.go` calls `public.NewWebSocket(ctx, pool)` only — no shared tree injected | T1.1 |
| Public WS ingest | `kraken/public/websocket.go` inserts every frame as `role=status`, `scope=disconnected` | T1.2–T1.6 |
| WS tests | `websocket_test.go` seeks `status/disconnected`; documents current behavior | T1.7 after T1.2–T1.6 |
| REST | `kraken/public/rest.go` — artifact-in/out `Do(ctx, *Artifact) *Artifact` | R3 (done pattern) |
| Trader relay | `trader/crypto.go` still fans out `updateSignals` / `Update`; `trader/book.go` etc. present | T1.10–T1.12 |
| Replay / codec | `signal/replay/`, `signal/codec/`, `signal/buffer/`, `signal/export.go` still on disk | T1.8–T1.9 |
| Signals | Many packages still expose `Update(*datura.Artifact)`; `pumpdump` is Measure-only but uses local `dmt.NewTree("")` | Phase 2 |
| `signal/tree.go` | `InsertMeasurement` helper exists; call sites use `feed` import alias | Keep; T2.10 standardizes on `signal.InsertMeasurement` |

Artifact doc examples below labeled **target** describe post–T1.2 ingest unless noted **current**.

---

## Authority

When sources conflict, resolve in this order:

1. **`AGENTS.md` §8** — architecture (tree ingress, Measure-only signals, nomagique pipeline shape, artifact field reference).
2. **`spec/SPEC.md`** — migration tasks, acceptance, and [artifact contract](#daturaartifact) (roles, payload vs attributes).
3. **On-disk reference files** — especially `signal/pumpdump/signal.go` (currently the best example of a mostly correctly written signal), `kraken/public/websocket.go`, `kraken/public/rest.go`.
4. **Failing tests and git diffs** — symptoms of incomplete migration only; **never** treated as direction to add relay layers, shims, or compatibility wrappers.

If a test expects architecture that contradicts §8, fix or delete the test — do not grow production code toward the failure.

---

## Requirements

- [ ] R1: **Single tree ingress** — Kraken public (and paper/private) websockets parse live frames and `tree.Insert` artifacts with Kraken-consistent `role`/`scope`/`origin` prefixes. Raw Kraken JSON lives in the payload; no duplicate relay through trader book/trade/ticker types or `signal.Update`.
- [ ] R2: **Measure-only signals** — Every signal exposes `NewSignal` + `Measure(query)` composing one `nomagique.Number` pipeline; no `Update`, no tracker access, no category `switch` inside `Measure`. Reference: `signal/pumpdump/signal.go` (seek path); publish via `signal.InsertMeasurement` (see `signal/toxicity/signal.go`).
- [ ] R3: **Artifact-only data model** — All requests, queries, results, events, schemas, and errors are `*datura.Artifact`. No parallel Go structs, DTOs, or domain models for the same information. See [datura.Artifact](#daturaartifact) below.
- [ ] R4: **Measurement contract** — Publishable measurements carry `classifier.category` (>0), `classifier.confidence` ∈ (0, 1], and surprise metadata. `logic.MeasurementFromArtifact` is the canonical bridge to playbook evaluation.
- [ ] R5: **Playbook evaluation** — `logic.Tree` (embedded `logic/rules/tree.yml`) walks branches on `logic.Measurement` slices and holdings; deepest reachable leaf wins. Exits are evaluated before entries.
- [ ] R6: **Trader desk** — `trader.Crypto` maintains latest readings per `(symbol, source)` by **querying** tree measurements (not orchestrating per-feed `Update`), applies friction/economics/cross-section gates, sizes edge-proportionally, and routes fills through `broker` (paper default, live when configured).
- [ ] R7: **Forward feedback** — `market.Story` records per-source forward movement labels; calibrated scales sharpen or soften signal feature scoring (README “forward truth” loop).
- [ ] R8: **Cognitive layer** — `trader/cognitive.Memory` consolidates per-scope observations into regime readings on the shared DMT tree; trader respects cognitive action filtering.
- [ ] R9: **UI telemetry** — `ui.Hub` fans out schema-driven frames over WebSocket with connect-time snapshot for dashboard state.
- [ ] R10: **Verification** — Host-runnable tests (Goconvey) and benchmarks pass for every changed data-processing path; use `make test-go` / `make bench` (`GOFLAGS=-ldflags=-checklinkname=0`). No fabricated test output. Tests document **current** behavior or **§8** contract — not intermediate relay architecture.
- [ ] R11: **Code style** — Follow `AGENTS.md`: methods over functions, early returns, no `else`, file ≤400 lines, method ≤60 lines, `errnie` for errors, prefer `nomagique`/`qpool`/`datura`/`errnie` over bespoke primitives.

---

## datura.Artifact

**Rule:** SYMM speaks one wire type — `*datura.Artifact`. Every boundary (websocket ingest, REST call, tree row, signal query, measurement result, UI frame source, broker request) passes artifacts. Do not introduce `BookUpdate`, `MeasurementDTO`, `ReplayBatch`, or other parallel structs when an artifact can carry the same information.

Full pipeline detail also lives in `AGENTS.md` §8. This section is the agent-facing contract for *what an artifact is* and *how to use it*.

### One type, many roles

The **same** capnp type plays different roles depending on what you set on it. Role is not a separate Go type — it is convention on `Role`, `Scope`, `Origin`, attributes, and payload:

| Role in the system | What it is | Typical `Role` / `Origin` | Payload holds | Attributes hold |
|--------------------|------------|---------------------------|---------------|-----------------|
| **Request** | Outbound call descriptor | any; `method`, `destination`, `headers` in attributes | empty or body bytes | HTTP method, URL, headers, params |
| **Query** | Tree seek key + optional filters | set on query artifact before `Measure` | usually empty | seek prefix hints, signal config |
| **Data / event** | Something that happened | ingest: `book`, `trade`, `ticker`, … | **raw Kraken JSON** (or encrypted bytes) | rarely — ingest metadata only |
| **Schema** | How to read other artifacts | `measurement` scope in `NewSignal` | empty | keys, types, transforms, rules |
| **Result** | Output of a pipeline or API | `measurement` + signal origin | result JSON or encrypted blob | `classifier.*`, `surprise`, … |
| **Error** | Failed operation | any | **`err.Error()` text** via `WithError` | optional error metadata in attributes |

Examples already in the tree:

```go
// REST request (kraken/public/rest.go) — attributes describe the call; payload optional
req := datura.Acquire("kraken", datura.APPJSON).
    WithAttribute("method", "GET").
    WithAttribute("destination", "https://api.kraken.com/0/public/Ticker").
    WithAttribute("headers", map[string]string{...})

// REST response + tree row — payload is the response body
out := datura.Acquire("kraken", datura.APPJSON).
    WithDestination(string(endpoint)).
    WithPayload(responseBody)
tree.Insert(out.Prefix(), out.Marshal())

// Ingest event (target — T1.2–T1.6) — payload is live Kraken frame; Role/Scope index the tree
artifact := datura.Acquire("kraken:public", datura.Artifact_Type_json).
    WithRole("book").WithScope("BTC/USD").WithPayload(rawKrakenJSON)
tree.Insert(artifact.Prefix(), artifact.Marshal())

// Measure query — passed into signal.Measure; Seek uses query.Prefix()
query := datura.Acquire("pumpdump", datura.APPJSON).
    WithRole("book").WithScope("BTC/USD")

// Schema (constructed once in NewSignal) — attributes only; no market data
schema := datura.Acquire("pumpdump", datura.APPJSON).
    WithRole("measurement").WithScope("trade").
    WithAttributes(datura.Map{
        "keys": datura.Map{"volume": "float", "price": "float"},
        "transforms": datura.Map{"volume": "ema", "price": "raw"},
    })
```

### Fields (capnp)

| Field | Purpose |
|-------|---------|
| **Payload** | The **bytes of record** — usually JSON for Kraken events, HTTP bodies, or encrypted capnp wire after pipeline stages. Market facts live here. |
| **Type** | Encoding of payload (`json`, `artifacts`, …) — `datura.APPJSON` for JSON. |
| **Role** | What kind of thing this is for indexing (`book`, `trade`, `measurement`, `status`, …). |
| **Scope** | Primary partition — almost always **symbol** on ingest (`BTC/USD`), or stream name (`trade`, `book`) on measurements. |
| **Origin** | Who created it (`kraken:public`, `pumpdump`, `toxicity`, …). Set by `datura.Acquire(origin, type)`. |
| **Destination** | Where to send it when routing outbound (e.g. `kraken:public` for websocket write). |
| **Attributes** | **Instructions for reading and processing** payload — schema, transforms, classifier outputs, HTTP metadata. Not a second copy of market data. |
| **Prefix** | `role/scope/origin/timestamp/uuid.type` — tree query API; use `tree.Seek(query.Prefix())`. |

### Payload vs attributes — the distinction agents get wrong

**Payload = what happened.** Immutable facts from the outside world: Kraken websocket JSON, REST response body, serialized measurement JSON. Stored as raw **`[]byte`** via `WithPayload` — see [JSON and raw bytes](#json-and-raw-bytes--no-struct-unmarshal). Read fields with `datura.PeekPayload[T](artifact, "json.path")`; read full bytes with `PayloadQuiet()` / `DecryptPayload()`.

**Attributes = how to interpret it.** Configuration and derived scalar metadata keyed by convention:

- **Request:** `method`, `destination`, `headers`
- **Schema:** `keys`, `transforms`, `rules` (see `AGENTS.md` §8)
- **Classifier output:** `classifier.category`, `classifier.confidence`, `classifier.strength`, `surprise`, `price`, …
- **Peek:** `datura.Peek[T](artifact, "dotted.key")` — read attributes, not payload JSON

| Put it in **payload** | Put it in **attributes** |
|-----------------------|---------------------------|
| Full Kraken book/trade/ticker JSON | Field names and types to extract |
| HTTP response body | HTTP method, URL, status |
| Large or nested event structure | Per-key transform (`ema`, `raw`, `zscore`) |
| Anything a exchange sent you | Classifier index, confidence, surprise |
| | Thresholds and rules driven by schema (not magic Go constants) |

**Do not** duplicate payload fields into attributes (e.g. storing `last_price` in attributes when it is already in the JSON payload). **Do not** put operational/market bulk data only in attributes because Go structs were easier — that bypasses the tree and breaks Seek.

### JSON and raw bytes — no struct Unmarshal

Agents may not have the `datura` repo open. You still do not need Kraken frame structs, `json.Unmarshal`, or manual capnp parsing of **payload JSON**. The payload is **`[]byte`** end to end.

#### Two different "Marshal" words

| Call | What it does | When |
|------|----------------|------|
| `artifact.WithPayload(raw []byte)` | Stores **your JSON bytes** (encrypted at rest inside the artifact) | Ingest, REST response, test fixtures |
| `artifact.Marshal()` | Serializes the **artifact capnp frame** for `tree.Insert` | Tree write only — not JSON, not a Go struct |
| `artifact.Unmarshal(wire)` | Loads artifact **capnp frame** from tree bytes | Rare in symm; tree seek yields artifacts for you |

**Never** `json.Unmarshal` a Kraken frame into a symm `BookUpdate` / `TradeEvent` struct as the primary path. **Never** `json.Marshal` a struct just to put market data on the artifact when you already have the websocket bytes.

#### Write — pass bytes through unchanged

Kraken websocket and HTTP already give you JSON as `[]byte`. Copy them straight into the artifact:

```go
// Target websocket ingest (T1.2–T1.6) — current code still uses status/disconnected
_, rawFrame, err := conn.ReadMessage()

artifact := datura.Acquire("kraken:public", datura.Artifact_Type_json).
    WithRole("book").
    WithScope(symbolFromFrame). // set Role/Scope from PeekPayload on a scratch artifact, or parse channel once
    WithPayload(rawFrame)        // raw Kraken JSON — no struct in between

tree.Insert(artifact.Prefix(), artifact.Marshal())
artifact.Release()
```

REST response body is the same: `WithPayload(response.Body())` — see `kraken/public/rest.go`.

Tests: build JSON with a string literal or `[]byte(`{"channel":"trade",...}`)`; insert into tree; no code-generated types.

#### Read — path access without structs

Use **`datura.PeekPayload`** to read fields from JSON inside the payload. Dot paths; numeric segments for array indices. No struct, no full-document unmarshal:

```go
channel := datura.PeekPayload[string](artifact, "channel")
symbol  := datura.PeekPayload[string](artifact, "symbol")
price   := datura.PeekPayload[float64](artifact, "price")
qty     := datura.PeekPayload[float64](artifact, "qty")

if channel, ok := datura.PeekPayloadOK[string](artifact, "channel"); ok {
    // present
}

count, ok := datura.PayloadLen(artifact) // when root is a JSON array
// Per-element fields: PeekPayload[float64](artifact, "0.price"), "1.price", …
```

(Array roots: use numeric path segments or `PayloadEach` — no `[]Trade` struct.)

Use **`datura.Peek[T](artifact, "attribute.key")`** for attributes (classifier outputs, HTTP metadata) — that is **not** payload JSON.

When you need the **full decrypted JSON bytes** (compare in tests, forward to websocket, log):

```go
payload, ok := artifact.PayloadQuiet() // []byte, no error — preferred in tests/ hot paths
// or
payload, err := artifact.DecryptPayload() // when you require explicit error handling
```

Example from tests (`kraken/public/websocket_test.go`): after `tree.Seek`, compare `string(payload)` to the original frame bytes — no struct round-trip.

#### Ingest: derive Role/Scope from JSON paths, not structs (target — T1.2)

At websocket ingest, channel and symbol live in the raw JSON. Read them with `PeekPayload` on a temporary artifact (or on the same artifact before Insert), then set indexing fields:

```go
artifact := datura.Acquire("kraken:public", datura.Artifact_Type_json).
    WithPayload(rawFrame)

channel := datura.PeekPayload[string](artifact, "channel")
symbol  := datura.PeekPayload[string](artifact, "symbol")

artifact.WithRole(channel).WithScope(symbol)
tree.Insert(artifact.Prefix(), artifact.Marshal())
```

Nomagique **`FeatureExtractor`** reads the same payload paths declared in schema **attributes** (`keys`, `transforms`) — the signal code does not unmarshal into per-channel Go types.

#### Classifier results — attributes, not payload JSON

Pipeline outputs (`classifier.category`, `classifier.confidence`, `surprise`, …) are written as **attributes**. Playbooks consume them via `logic.MeasurementFromArtifact`, which uses `datura.Peek`, not payload struct tags:

```go
categoryIndex := datura.Peek[int](artifact, "classifier.category")
confidence    := datura.Peek[float64](artifact, "classifier.confidence")
```

#### Allowed exception

`logic.MeasurementFromArtifact` may `sonic.Unmarshal` payload JSON into `logic.Measurement` when the artifact already carries a pre-built measurement JSON blob (legacy/export path). That is **not** the pattern for Kraken ingest or signal feature extraction.

#### Quick reference (import `github.com/theapemachine/datura`)

| Goal | API |
|------|-----|
| Attach raw Kraken/HTTP JSON | `WithPayload([]byte)` |
| Read JSON field | `PeekPayload[T](artifact, "path.to.field")` |
| Read JSON array length / iterate | `PayloadLen`, `PayloadEach` |
| Read attribute | `Peek[T](artifact, "dotted.key")`, `WithAttribute` |
| Get full JSON bytes back | `PayloadQuiet()` or `DecryptPayload()` |
| Store in tree | `tree.Insert(artifact.Prefix(), artifact.Marshal())` |
| Return failure from artifact method | `return datura.Acquire(...).WithError(errnie.Err(...))` |
| Compare in test | `PayloadQuiet()` vs original `[]byte` frame |

### Tree: write, seek, release

```go
// Write (ingest or publish)
tree.Insert(artifact.Prefix(), artifact.Marshal())
artifact.Release()

// Read (Measure, trader, story)
for inbound := range tree.Seek(query.Prefix()) {
    transport.NewFlipFlop(&inbound, algo)
    inbound.Release()
}
```

Ingest prefixes describe **what arrived** (`Role: book`, `Scope: BTC/USD`). Measurement prefixes describe **what was derived** (`Role: measurement`, `Scope: trade`, `Origin: pumpdump`). Same tree, different seeks.

### Lifecycle

```go
artifact := datura.Acquire(origin, datura.APPJSON) // pool; sets origin, uuid, timestamp
artifact.WithRole(...).WithScope(...).WithPayload(...)
// use
artifact.Release() // return to pool after Insert or when done reading
```

Constructors chain (`WithRole`, `WithPayload`, `WithAttributes`, `WithDestination`). Prefer `datura.Map` for nested attribute trees.

### Errors on the artifact — not `(T, error)`

When a method is **artifact in → artifact out**, the error travels **on the returned artifact**. You do not need a second `error` return value — the artifact is the result *and* the error channel.

**Preferred signature:**

```go
func (rest *Rest) Do(ctx context.Context, request *datura.Artifact) *datura.Artifact
func (signal *Signal) Measure(query *datura.Artifact) *datura.Artifact
```

**On failure** — return a fresh artifact with `WithError` (still an artifact, always non-nil pointer):

```go
return datura.Acquire(origin, datura.APPJSON).WithError(
    errnie.Err(errnie.IO, "kraken/public: request failed", err),
)
```

Reference: `kraken/public/rest.go` — `RestClient.Do` returns `*datura.Artifact` only; HTTP failure returns an error artifact, success returns payload + tree insert.

**On pipeline failure inside Measure** — attach to the working copy, then let the caller skip invalid rows:

```go
if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
    _ = processed.WithError(flipErr)
}
// caller checks classifier attributes; invalid rows are released, not propagated as Go error
```

Reference: `signal/toxicity/signal.go`, `signal/pumpdump/signal.go`.

**Build errors with `errnie.Err`** (kind, message, wrapped cause) before passing to `WithError`. Log at boundaries with `errnie.Error(...)` when something must appear in run logs — the artifact carries the failure; logging is optional observation.

**Caller contract:**

```go
out := client.Do(ctx, req)

if out == nil {
    return // Acquire failed — rare
}

measurement, ok := logic.MeasurementFromArtifact("pumpdump", out)

if !ok {
    out.Release()
    return // error artifact or incomplete measurement — no separate err variable
}
```

Success and failure are distinguished by **artifact shape**, not a parallel return:

| Outcome | What to check |
|---------|----------------|
| Success (REST) | `PayloadQuiet()` has JSON body; tree row inserted |
| Success (Measure) | `classifier.category` > 0, `classifier.confidence` ∈ (0, 1] |
| Failure | `WithError` set — payload holds `err.Error()` text; success fields absent |

**When a Go `error` return still makes sense:** lifecycle on long-lived objects (`Close() error`, `Run() error`), qpool job callbacks that have not been migrated yet, and context cancellation on types that are not artifact pipelines. Do **not** add `( *datura.Artifact, error)` on new artifact-in/out APIs — pick artifact-only.

```go
// Wrong — redundant error return when artifact already carries the outcome
func Do(ctx context.Context, req *datura.Artifact) (*datura.Artifact, error)

// Right
func Do(ctx context.Context, req *datura.Artifact) *datura.Artifact
```

### nomagique and schema artifacts

Signal pipelines are configured by a **schema artifact** created in `NewSignal` — it has **attributes only**, no payload. Stages receive **data artifacts** on `Write` via FlipFlop:

```go
algo := nomagique.Number(
    vector.NewFeatureExtractor(schemaArtifact), // reads attributes + inbound payload
    probability.NewClassifier(logic.NewCircuit(...), ...),
)
```

Feature extraction reads payload JSON paths defined in schema attributes. Classifier writes results back onto the artifact as attributes (`classifier.category`, etc.). `logic.MeasurementFromArtifact` bridges to playbooks.

### Where artifacts are mandatory

| Location | Pattern |
|----------|---------|
| `kraken/public/websocket.go` | **Target (T1.2–T1.6):** parse frame → artifact with raw JSON payload + channel/symbol role/scope → `tree.Insert`. **Current:** all frames `status/disconnected`. |
| `kraken/public/rest.go` | `Do(ctx, requestArtifact) *Artifact` — request via attributes, response via payload |
| `signal/*/signal.go` | `Measure(query *Artifact)` — seek with `query.Prefix()` |
| `logic/measurement_artifact.go` | `MeasurementFromArtifact` — read classifier attributes |
| `trader/` | Seek measurement prefixes; no parallel reading map types |
| Tests | Build fixtures with `datura.Acquire` + `WithPayload`; insert into tree — no replay structs |

### Anti-patterns

```go
// Wrong — parallel struct for data the tree already models
type BookUpdate struct { Symbol string; Bids [][]float64 }

// Wrong — json.Unmarshal Kraken frame into a struct at ingest
var update BookUpdate
json.Unmarshal(rawFrame, &update)

// Wrong — market data in attributes
artifact.WithAttribute("bids", bidSlice)

// Wrong — redundant (artifact, error) when artifact carries failure
func Do(req *datura.Artifact) (*datura.Artifact, error)

// Wrong — hardcoded extractor params instead of schema artifact
vector.NewFeatureExtractor(16, []string{"volume"})

// Wrong — signal Update ingesting feeds (use tree insert at websocket)
func (s *Signal) Update(frame *BookUpdate) { ... }

// Right — raw JSON in payload, schema in attributes, one type throughout
artifact.WithRole("book").WithScope(symbol).WithPayload(rawJSON)
```

If tempted to add a new Go struct or method parameter for market or config data: **use an artifact attribute or payload path instead.** If nomagique cannot express it, extend a primitive or attribute convention first (`AGENTS.md` §8 decision order).

---

## Roadmap

Tasks use stable IDs. The sync phase checks these off when review passes. Each task is scoped for one develop→review cycle.

**Phase 1 ordering:** T1.2–T1.6 before T1.7 (tests assert routed prefixes). T1.8–T1.13 are subtraction — safe in parallel with ingest work but trader must not depend on deleted packages before T1.11–T1.12 land.

### Phase 1: Tree ingress and subtraction

- [ ] T1.1 — Construct shared `*dmt.Tree` in `cmd/root.go` and pass into `public.NewWebSocket` and `public.NewRest` (replace per-package `dmt.NewTree("")`) (requirement: R1)
- [ ] T1.2 — `kraken/public/websocket.go`: derive `role`/`scope` from `PeekPayload` (`channel`, `symbol`); remove `status`/`disconnected` placeholder on data frames (requirement: R1, R3)
- [ ] T1.3 — Route `book` channel JSON into tree artifacts (`role=book`, symbol scope, raw payload) (requirement: R1)
- [ ] T1.4 — Route `trade` channel JSON into tree artifacts (`role=trade`) (requirement: R1)
- [ ] T1.5 — Route `ticker` channel JSON into tree artifacts (`role=ticker`) (requirement: R1)
- [ ] T1.6 — Route `instrument` and `ohlc` channel JSON into tree artifacts (requirement: R1)
- [ ] T1.7 — `kraken/public/websocket_test.go`: after T1.2–T1.6, seek book/trade/ticker rows; assert `PayloadQuiet()` matches fixture bytes (requirement: R1, R10)
- [ ] T1.8 — Delete `signal/replay/` and remove `replay` imports from `trader/crypto.go` (requirement: R1)
- [ ] T1.9 — Delete `signal/export.go`; migrate or remove remaining `signal/codec` and `signal/buffer` call sites (requirement: R1, R3)
- [ ] T1.10 — Remove `kraken/market` feed helpers and `replay.Ingest*` calls from `trader/crypto.go` subscription handlers (requirement: R1, R3)
- [ ] T1.11 — Delete `trader/book.go`, `trader/ticker.go`, `trader/trade.go`; drop relay `Update` subscriptions from `trader/crypto.go` (requirement: R1, R6)
- [ ] T1.12 — Remove `updateSignals` and per-signal `Update` fan-out from `trader/crypto.go` (requirement: R1, R2, R6)
- [ ] T1.13 — Delete `signal/toxicity/ingest.go` tracker/replay path; toxicity measures from tree seeks only (requirement: R1, R2)

### Phase 2: Measure-only signal fleet

- [ ] T2.1 — `signal/depthflow`: delete `Update`; `Measure` seeks shared tree + FlipFlop only (requirement: R2)
- [ ] T2.2 — `signal/liquidity`: delete `Update`; `Measure` seeks shared tree only (requirement: R2)
- [ ] T2.3 — `signal/correlation`: delete `Update`; `Measure` seeks shared tree only (requirement: R2)
- [ ] T2.4 — `signal/leadlag`: delete `Update`; `Measure` seeks shared tree only (requirement: R2)
- [ ] T2.5 — `signal/manifold`: delete `Update`; remove `manifoldCategory` switch from classification path (requirement: R2, R3)
- [ ] T2.6 — `signal/causal`: delete `Update`; `Measure` seeks shared tree only (requirement: R2)
- [ ] T2.7 — `signal/fluid`: delete `Update`; remove `fluidCategory` switch from classification path (requirement: R2, R3)
- [ ] T2.8 — `signal/resonance`: delete `Update`; migrate `Measure` to return `*datura.Artifact` (drop `logic.Measurement, error`) (requirement: R2, R4)
- [ ] T2.9 — Thread shared `*dmt.Tree` through all `NewSignal` constructors; remove redundant per-signal `dmt.NewTree` fields (requirement: R1, R2)
- [ ] T2.10 — Publish measurements via `signal.InsertMeasurement` from `Measure` (today many signals import `feed "github.com/theapemachine/symm/signal"`); keep `signal/tree.go` as the shared insert helper only (requirement: R4)
- [ ] T2.11 — Goconvey + benchmark: `signal/toxicity` tree-seek `Measure` path (requirement: R10)
- [ ] T2.12 — Goconvey + benchmark: `signal/pumpdump` tree-seek `Measure` path (requirement: R10)
- [ ] T2.13 — Goconvey + benchmark: `signal/hawkes` and `signal/cvd` `Measure` paths (requirement: R10)
- [ ] T2.14 — Goconvey + benchmark: `signal/sentiment` and `signal/exhaust` `Measure` paths (requirement: R10)
- [ ] T2.15 — Goconvey + benchmark: migrated `depthflow`, `liquidity`, `correlation` `Measure` paths (requirement: R10)
- [ ] T2.16 — Goconvey + benchmark: migrated `leadlag`, `manifold`, `causal`, `fluid` `Measure` paths (requirement: R10)
- [ ] T2.17 — Goconvey + benchmark: `signal/resonance` `Measure` path after artifact return migration (requirement: R10)

### Phase 3: Trader, story, and execution

- [ ] T3.1 — `trader.Crypto` measure cycle seeks `measurement/<origin>` tree prefixes instead of inline relay-driven `signal.Measure` (requirement: R2, R6)
- [ ] T3.2 — Trader reading path uses `logic.MeasurementFromArtifact` for playbook inputs (requirement: R4, R6)
- [ ] T3.3 — `logic.Tree.Evaluate` drives desk actions; exit branches evaluated before entries (requirement: R5)
- [ ] T3.4 — Paper broker fills respect quote freshness, spread, slippage, and latency profile from config (requirement: R6)
- [ ] T3.5 — `market.Story` forward-feedback loop: record per-source labels and apply calibrated scales (requirement: R7)
- [ ] T3.6 — Wire `trader/cognitive.Memory` from `cognitive.*` in `cmd/cfg/config.yml`; consolidate per-scope observations on DMT tree (requirement: R8)
- [ ] T3.7 — Trader desk respects cognitive `Sideline` / regime action filtering (requirement: R8)
- [ ] T3.8 — Symbol discovery and instrument subscription via REST artifact requests (`kraken/public/rest.go`) (requirement: R1, R6)
- [ ] T3.9 — Paper wallet bootstrap and balance subscription parity with README execution contract (requirement: R6)

### Phase 4: UI and frontend parity

- [ ] T4.1 — Publish measurement gauges via `ui.Publish*` from story/trader readings (schema-driven; no per-signal Go unions at hub) (requirement: R9)
- [ ] T4.2 — `trader/decision_tree_publish.go` + connect snapshot matches embedded `logic/rules/tree.yml` (requirement: R9)
- [ ] T4.3 — Publish cognitive readings over WebSocket from `trader/cognitive.Memory` (requirement: R8, R9)
- [ ] T4.4 — Wire `frontend/src/collections/cognitive.ts` and `CognitivePanel` to cognitive WebSocket frames (requirement: R8, R9)
- [ ] T4.5 — Refactor `frontend/src/collections/signals.ts` to schema-driven gauges (no hard-coded signal union) (requirement: R9)
- [ ] T4.6 — `make test-frontend` TypeScript check and Vitest pass (requirement: R10)

### Phase 5: Integration and tooling

- [ ] T5.1 — Expand `integration/master_test.go` with tree-inserted fixtures covering signal categories and playbook walks (requirement: R10)
- [ ] T5.2 — Host-runnable `make test-go` green for all packages touched by migration (requirement: R10)
- [ ] T5.3 — `make build` produces `bin/symm`; `make run` boots paper mode with UI websocket at `ws://127.0.0.1:8765/ws` (requirement: R6, R9)
- [ ] T5.4 — Update `README.md` repository map and boot sequence to match post-migration layout (non-blocking documentation sync)

---

## Progress

Phase 1 only — mirrors [Roadmap Phase 1](#phase-1-tree-ingress-and-subtraction). Phases 2–5 progress lives in the Roadmap checkboxes until Phase 1 completes.

- [ ] T1.1 — Construct shared `*dmt.Tree` in `cmd/root.go` and pass into public websocket/REST
- [ ] T1.2 — Derive `role`/`scope` from `PeekPayload`; remove status/disconnected placeholder
- [ ] T1.3 — Route `book` channel JSON into tree artifacts
- [ ] T1.4 — Route `trade` channel JSON into tree artifacts
- [ ] T1.5 — Route `ticker` channel JSON into tree artifacts
- [ ] T1.6 — Route `instrument` and `ohlc` channel JSON into tree artifacts
- [ ] T1.7 — Websocket tests: after T1.2–T1.6, seek book/trade/ticker rows; assert payload bytes
- [ ] T1.8 — Delete `signal/replay/` and remove trader replay imports
- [ ] T1.9 — Delete `signal/export.go`; migrate codec/buffer call sites
- [ ] T1.10 — Remove `kraken/market` feed helpers from `trader/crypto.go`
- [ ] T1.11 — Delete trader book/ticker/trade relay types and subscriptions
- [ ] T1.12 — Remove `updateSignals` and per-signal `Update` fan-out
- [ ] T1.13 — Delete `signal/toxicity/ingest.go` tracker path

---

## Acceptance criteria

Work is **done** when all of the following hold for the tasks in scope:

1. **Architecture** — Market data enters once via websocket → `tree.Insert`; signals only `Measure` via `Seek` → FlipFlop → `Number`; trader/story query; **no** relay chain; **no** `signal/replay` or codec/buffer paths.
2. **Artifact-only** — No new domain structs for data/config that belong in artifact payload or attributes; requests, queries, and results are `*datura.Artifact` at boundaries.
3. **Signal integrity** — No hardcoded thresholds; adaptive nomagique stages derive bounds from live observations.
4. **Measurement validity** — Confidence ∈ (0, 1]; category index > 0; surprise fields populated where playbooks gate on them.
5. **Playbooks** — `logic/rules/tree.yml` loads; exit branches precede entry branches.
6. **Execution** — Paper fills respect quote freshness, spread, slippage, and latency profile; live mode fails closed when credentials/session invalid.
7. **Tests** — Host-runnable `go test` and benchmarks pass for changed packages; literal stdout pasted per `AGENTS.md` §3. Tests aligned with §8, not with deleted relay packages.
8. **Style** — `AGENTS.md` size limits, control flow, naming, and errnie usage satisfied.
9. **Working tree** — Implementation present in files; commits are human-owned.

---

## Orchestrator log

| Run | Agent | Branch | Task | Result | Notes |
|-----|-------|--------|------|--------|-------|
| 1 | spec-author | curious | bootstrap | PASS | Initial `spec/SPEC.md` from README and working-tree exploration |
| 2 | parent | curious | realign | PASS | SPEC aligned to `AGENTS.md` §8; replay/codec/buffer marked debt; authority order added |
| 3 | parent | curious | artifact-doc | PASS | Artifact section: roles, payload vs attributes, tree lifecycle |
| 4 | parent | curious | payload-bytes | PASS | JSON/raw-bytes guide: PeekPayload, no struct Unmarshal at ingest |
| 5 | parent | curious | artifact-errors | PASS | Errors via `WithError` on artifact-in/out methods; no `(T, error)` |
| 6 | roadmap-planner | curious | expand-roadmap | PASS | Phased tasks T1.1–T5.4 sized for single develop→review cycles |
| 7 | parent | curious | spec-once-over | PASS | Current-state snapshot; target vs current labels; Error row; `signal/tree.go` keep; open Q1 wording |

---

## Constraints

### Technology

- **Language**: Go 1.26+ backend; React/TypeScript frontend (`frontend/`).
- **Key libraries**: `github.com/theapemachine/datura`, `dmt`, `nomagique`, `qpool`, `errnie`; Kraken WebSocket v2; Fiber WebSocket hub.
- **Build**: Use `Makefile` targets; `GOFLAGS=-ldflags=-checklinkname=0` required for qpool linkname hooks.
- **Config**: `cmd/cfg/config.yml` (embedded default); viper env overrides (`SYMM_*`).
- **Host**: Curious runs on **arm64**. Tests behind `//go:build amd64` are optional; host-runnable tests and code inspection suffice.

### Style and process (from AGENTS.md)

- Code inventory → refactoring identification (what to **remove**) → three approaches → minimal scope.
- Prefer methods over functions; early returns; no `else`; nesting ≤2; no silent failures.
- File target 200 lines (hard max 400); method target <30 lines (hard max 60).
- Tests: Goconvey, BDD nested style, benchmarks at bottom; test variable `t`.
- Errors: `errnie` only; error variable always named `err`.
- Do not read git history to solve bugs; do not infer architecture from deleted files or failing tests.

### Agent git policy

- **Human commits only** — agents deliver changes in the working tree; humans commit, push, and manage branches.
- Agents may use **read-only** git for inspection: `git status`, `git diff`, `git log`, `git show`, `git branch`, `git rev-parse`, `git ls-files`.
- Agents **must not** run mutating git commands (`git add`, `git commit`, `git reset`, `git restore`, `git checkout` / `git switch`, `git stash`, `git worktree`, or any command that changes refs, index, or working tree).
- Stay on the branch assigned (`curious`); do not create worktrees or switch branches.
- When uncertain whether work landed, **read source files and `git diff`** — file content is the source of truth, not chat history, orchestrator summaries, or `HEAD` assumptions.

### Verification

- Develop and review on the **local host architecture only** — no GitHub Actions or CI URLs required for review PASS.
- On **arm64**, tests behind `//go:build amd64` are optional; host-runnable tests and code inspection suffice.
- PASS when host-runnable tests pass (or limitations documented) and implementation matches §8 for tasks in scope.
- FAIL for wrong/missing code, failing host tests, or tests that reintroduce relay architecture — not for missing optional amd64-only output.

### Non-goals (this migration)

- **`signal/replay/`**, **`signal/codec/`**, **`signal/buffer/`** — delete, do not wire or extend.
- **`updateSignals`**, per-signal **`Update`**, trader-side feed orchestration — remove, do not grow.
- **Compatibility shims** (`signal/export.go` re-exporting deleted packages) — delete call sites, do not restore.
- Reintroducing `market/perspectives` Go builtin registry (YAML in `logic/` is canonical).
- Adding domain types to nomagique or signal glue when an attribute or primitive extension would suffice.
- Generalized abstractions, auxiliary helper files, or out-of-scope refactors.

If extra wiring seems necessary beyond **websocket → tree → Seek → FlipFlop → Number**, stop and fix nomagique, the artifact schema, or ingest prefixes — do not grow the signal or trader.

---

## Open questions

1. **Shared tree ownership** — To be resolved by T1.1 (explicit boot injection for ingest) and T2.9 (signal constructors). Today `dmt.NewTree("")` is a process singleton, but boot does not pass one shared pointer into public/trader/signals — prefer explicit injection for tests and clarity.

> It literally makes no difference.

2. **Kraken frame typing** — Parse channel/symbol from raw JSON at ingest; no restored `kraken/market` types package unless a thin constant set is unavoidable.

> It is never unavoidable if the example of pumpdump is followed closely.

3. **Resonance** — Meta-signal that seeks measurement prefixes across the fleet, or fold into per-signal outputs? Must still obey Measure-only contract (no `Update`, no ingest).

> The signal follows the same pattern, so yes it will have an Update method for the trader to call.

4. **Tune/eval CLI** — Deferred until tree + Measure path is stable; offline eval uses tree-inserted fixtures, not a replay package.

> This is old news. Do not focus on this.

5. **SNR field naming** — README `Measurement.SNR` vs `logic.Measurement.Surprise`; pick one canonical field for playbook `value:` gates and UI.

> supercalifragilistichespiralidoso

6. **Quote currency** — `cmd/cfg/config.yml` vs README default; pick one for symbol discovery.

> USD

7. **Cognitive gating** — Block all desk actions on `Sideline`, or scale size only?

> Per definition not entirely correct. The Desk may NEVER be blocked for exists. It should always be allowed and able to respond to any event now or later to be introduced that makes the Desk "decide" to exit a position. As for entries, yes block.

8. **L3 websocket** — After core public tree path is stable.

> It's just a simple websocket connection with a token added in the header. Don't make it more than it needs to be, just wire it in.

9. **Frontend cognitive contract** — Final WebSocket event name and payload shape for cognitive readings.

> Don't do more work than needed, you send data in the shape that its in at the moment you want to send it to the websocket. And you don't waste compute on all kinds of checks, validations, or typing. Just feed the raw frames into the charts or widgets. The occassional crash that may happen in the begining I would rather have than the overhead in a system already looking for optimizations to even be able to run. Speaking of which, try to think about bulk transport of websocket messages, and only send what you actually use, to contain the websocket flood.
