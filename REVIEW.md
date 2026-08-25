# Symm Code Review — Correctness, Performance, UI Wiring

This review was produced by reading the code directly (no single file was skimmed; the
findings below carry exact file/line references). It is organized around the three
dimensions requested and, per instructions, every finding includes a concrete resolution.

**Scope note on "nomagique":** the project ships **two divergent copies** of the nomagique
algebra (see §1.1). Because "re-implementing logic already handled in nomagique" was called
out explicitly, the most serious finding is that the wrong copy is being depended on, and
the two copies have already drifted apart at exactly the packages the core solvers use.

---

## 1. Correctness

### 1.1 — CRITICAL — Two divergent nomagique trees, with the replace directive pointing at the stale one

`go.mod`:

```go
require github.com/theapemachine/nomagique v0.0.9
replace github.com/theapemachine/nomagique => ../nomagique
```

But the repository **also** contains a full vendored copy at `./nomagique/` (imported as
`github.com/theapemachine/symm/nomagique/...`). The two are not the same:

| | `../nomagique` (sibling, the `replace` target) | `./nomagique` (in-repo copy) |
|---|---|---|
| files | 371 | 266 |
| extra packages | `adaptive`, `core`, `decay`, `geometry`, `hawkes`, `timeline`, `vector` | `calculus`, `data` |
| has `types` pkg | **no** (`types` → `core` rewrite) | **yes** |
| `mcts/` structure | `causal.go`, `graph.go`, `node.go`, `search.go` | `action.go`, `node.go`, `search.go`, `state.go` |

14 of the 40 files that exist in **both** trees already differ, including the exact
packages the core pipeline leans on:

- `learning/resonance.go`, `learning/pace.go`, `learning/resonance_workspace.go` **(differ)**
- `mcts/search.go`, `mcts/node.go` **(differ)**
- `causal/table.go`, `correlation/hayashi.go`, `statistic/mean.go`, `statistic/median.go` **(differ)**

The application then mixes the two trees **in the same files**:

- `logic/resonance/solver.go` imports `github.com/theapemachine/nomagique/adaptive` (sibling)
  **and** `github.com/theapemachine/symm/nomagique/learning` + `.../nomagique/runtime` + `.../types` (in-repo).
- `types/decision.go` imports `github.com/theapemachine/nomagique/mcts` (sibling)
  **and** `github.com/theapemachine/symm/nomagique/learning` (in-repo).
- `strategy/planner.go` imports `.../symm/nomagique/mcts`, while `strategy/causal_economy.go`
  imports the same package through the sibling path.

**Why this is correctness-critical:** the sibling tree has moved on (renamed `types`→`core`,
restructured `mcts`), so the `adaptive`, `geometry`, `probability` symbols the solvers pull
from it are a *different generation* than the `learning`, `runtime`, `types`, `mcts` symbols
they pull from the in-repo copy. The two `mcts.Action` enums, the two `causal` packages, and
the two `Frame` types are **distinct Go types** that share only a name. Any boundary that
passes a value from one tree into the other will not compile-merge silently — it will type-
mismatch or, worse, resolve to a different behavior — and the divergence means behavior
drifts without either side being checked.

**Worse:** the in-repo copy is not a self-contained library. `./nomagique/mcts/action.go`
imports `github.com/theapemachine/symm/logic/causal` (a *reverse dependency* back into the
application). The "library" depends on the app it is supposed to be a leaf of.

**Resolution (in order of preference):**

1. Pick **one** nomagique. The sibling `../nomagique` is the live module (it has `go.mod`,
   `AGENTS.md`, `Makefile`, and the newer `core`/`adaptive`/`geometry` layout). Migrate all
   `github.com/theapemachine/symm/nomagique/...` imports to `github.com/theapemachine/nomagique/...`
   and delete `./nomagique/` (or make it a thin re-export). This is a one-mechanical-rename
   migration, plus a re-verification of the `types`→`core` API surface the signal packages use.
2. **At minimum**, add a build-time guard so the two trees cannot drift silently again —
   e.g. a CI step that diffs `./nomagique` against `../nomagique` on every commit and fails
   on any divergence.
3. Remove the reverse dependency (`./nomagique/mcts/action.go` → `logic/causal`). Either
   move the MCTS `ActionEstimate` identification type into nomagique proper, or invert the
   dependency so the app maps nomagique's neutral type onto its `causal.IdentificationStatus`.

---

### 1.2 — CRITICAL — MCTS economic model double-counts the entry fee and marks positions at the buy side

`strategy/planner.go:627-659` (`deskMarketInputs`):

```go
inputs.feeRate = fee.Fee.Float64() / 100
inputs.mark = tick.Ask.Float64() * (1 + inputs.feeRate)   // "fee-inclusive buy mark"
```

`nomagique/mcts/state.go:98-100` (`CostModel.TotalFraction`):

```go
return costs.FeeRate + costs.SpreadFraction + costs.SlippageFraction
```

`nomagique/mcts/state.go:232-269` (`ApplyAction`, `Enter` branch):

```go
price := state.Portfolio.MarkPrice            // = ask * (1 + feeRate)
notional := state.UnitQuantity * price
cost := notional * state.Costs.TotalFraction() // includes FeeRate AGAIN
cash -= notional + cost
```

The **entry fee is therefore applied twice**: once baked into `MarkPrice`
(`ask × (1+fee)`), once inside `TotalFraction()` (`feeRate + spread + slippage`). Because
`MarkPrice` is also the single price used for every other valuation in the search —

- `Wealth()` = `Cash + Position*MarkPrice` (`state.go:116-118`) marks an open long at the
  *buy* side (`ask × (1+fee)`), not the exit-able *bid* side;
- `Exit` (`state.go:259-267`) sells at `MarkPrice` (the buy mark) and then deducts
  `TotalFraction()` again, i.e. `cost = notional × (fee + spread + slippage)` on top of a price
  that was never the bid in the first place;

— every rollout systematically overstates net wealth and understates exit cost. This biases
the search toward entering and toward delaying exits.

**Resolution:** make the economic model honor direction-aware marks instead of one scalar.

- Split `PortfolioState.MarkPrice` into `Bid`/`Ask` (or pass a `Mark(buy|sell)` accessor) and
  value `Enter` at the ask side and `Exit`/`Wealth` at the bid side.
- Remove `FeeRate` from the `CostModel.TotalFraction()` for the *entry* leg (or stop
  pre-multiplying the mark by `(1+fee)` in `deskMarketInputs`), so the fee is charged exactly
  once per side.
- Add a unit test in `nomagique/mcts/state_test.go` that runs one Enter→Exit round-trip with a
  zero market move and asserts the account returns to `Cash - 2*fee - 2*spread - 2*slippage`
  (a fully deterministic invariant), which fails today.

---

### 1.3 — HIGH — `errnie` return values are discarded, silently swallowing construction errors

`logic/manifold/solver.go:141-149`:

```go
physics, err := pmanifold.NewManifold(...)
errnie.Error(errnie.Err(errnie.NotAcceptable, "manifold: error building manifold", err)) // ignored
corpus, corpusErr := geometry.NewCorpus[types.PhaseOutcome](phaseCorpusCapacity)
errnie.Error(corpusErr)                 // ignored
angles, angleErr := geometry.PhasePath(phaseScanAngles)
errnie.Error(angleErr)                  // ignored
```

If `NewManifold` returns a nil `physics` and a non-nil `err`, the code logs it and **continues**
with `physics == nil`. Downstream `Update`/`advance` dereference `solver.physics.Step(...)`
(`solver.go:791`) which will nil-panic — but only *after* the system has already been running
in a half-initialized state. This is exactly the "silent death" the project's `AGENTS.md`
pattern forbids ("No Silent Failures … Let unexpected panics surface").

**Resolution:** return the error from `NewSolver` and abort construction:

```go
physics, err := pmanifold.NewManifold(...)
if err != nil {
    return nil, errnie.Error(errnie.Err(errnie.NotAcceptable, "manifold: error building manifold", err))
}
```

Do the same for `corpusErr` and `angleErr` (have `NewSolver` return `(*Solver, error)`), or
`panic` immediately. The same audit should be applied to any other `errnie.Error(...)` call
whose return is dropped (search for `errnie.Error(` followed by no assignment).

---

### 1.4 — HIGH — `standardize` substitutes `0` on measurement error (silent zero fallback)

`logic/resonance/solver.go:399-403`:

```go
score, err := standardizers[index].Measure(value)
if err != nil {
    standardized[index] = 0   // <-- silent fallback
    continue
}
```

A failed standardizer measurement becomes `0.0` and is fed into the predictive coder's
manifold. Per `AGENTS.md`: "If for example we return 0 as the fallback and then use that in a
multiplication, we instantly zero out the operations." A transient standardizer error will
quietly poison the latent state rather than surface.

**Resolution:** propagate the error from `standardize` (return `([]float64, error)`) and let
`Update` fail the symbol/solver with a descriptive `errnie` error, rather than manufacturing a
zero row. The coder already validates width; add a "no fabricated zero rows" invariant to the
resonance test.

---

### 1.5 — MEDIUM — Stop-loss drawdown treats a *directional prediction* as a *log return*

`types/stoploss.go:831-851` (`forecastGeometry`):

```go
for _, predictedReturn := range forwardCurve {
    cumulativeReturn += predictedReturn
    if cumulativeReturn < minimumPathReturn { minimumPathReturn = cumulativeReturn }
}
drawdown := -math.Expm1(minimumPathReturn)
...
survival := one.Sub(decimal.NewFromFloat64(drawdown))
floor := floorToTick(currentMark.Mul(survival), tick)
```

`forwardCurve` here is the `PredictiveOutput.ForwardCurve`, which the producer documents as
**"cumulative directional prediction"** / signed call values, not log returns
(`nomagique/learning/predictive.go:202-236`:

```go
// Element k of the curve is the cumulative directional prediction over the next k+1 ticks
predictions[horizonIndex] = rollout.Value
```

`RolloutTaskForecast` returns task-head posterior values (a `score`-scale readout), which the
resonance solver itself thresholds into `-1/0/+1` (`logic/resonance/solver.go:466-475`).
Applying `-Expm1(...)` — an *exponential/log-return* transform — to a *directional score* yields
a drawdown fraction that is not a price return in any unit. The resulting stop floor is thus
derived from a quantity with the wrong semantics.

**Resolution:** clarify the contract in one place and make both sides agree:

- Either have `RolloutTaskForecast`/`ForwardCurve` return genuine per-step **log returns** and
  document it at the type level (rename to `ForwardLogReturns`), so `Expm1` is correct; **or**
- Change `forecastGeometry` to derive the floor from the *risk plan / tick size / entry
  geometry* (which is what the live path already does via `NewStoplossWithPlan`) and stop
  consuming `ForwardCurve` as a return series in `forecastGeometry`.

Note `NewStoploss` (the non-`WithPlan` constructor) is the path that reaches this code; the
live `allocation.go` uses `NewStoplossWithPlan`. Either way the unit mismatch is real and
should be fixed or the dead path removed.

---

### 1.6 — MEDIUM — Legacy decision fields are wired to the UI but never populated by the causal-MCTS path

`types/decision.go:42-52` still declares `GraphScore`, `ThesisScore`, `AdmissionGraphThreshold`,
`AdmissionUtilityThreshold`, `ThesisConfidence`, `ThesisSupport`, `PredictiveReady`, etc., and
`types/decision_wire.go:25-35` copies them onto the wire frame. But `strategy/planner.go`
(`decisionFromCausalState` → `recordEconomic`) only writes:

```go
alternatives["economic:expected_outcome"], ["economic:outcome_uncertainty"], ["economic:visits"]
```

The planner's own comment (`planner.go:38-40`) states the admission policy / opportunity
scores / predictive-readiness veto "play no role in this path." `types/admission.go` is now
referenced only by validation tags and `broker/trade_store.go` — it is a disconnected
admission policy. Those `Decision` fields therefore stay at their zero/default values forever,
`json`-serialized and rendered as `0`/blank in the dashboard.

**Resolution:** either (a) delete the legacy fields from `Decision`, the `DecisionT` FlatBuffers
table, and the `data-paint` bindings that read them (`graphScore`, `thesisScore`,
`decision.graphScore`, `decision.thesisScore`, `decision.admissionGraphThreshold`), or (b) if
they are intended to return, populate them from a real source. Keeping them wired-but-zero is a
silent lie to the UI.

---

## 2. Performance

### 2.1 — HIGH — `economicOrder` / `economicTrace` sort and re-map per decision pass

`strategy/allocation.go:307-331` and `planner.go:681-703` are fine in isolation, but the hot
path is per-tick and per-symbol, and several allocations occur on every pass:

- `planner.go:302` `drainPending` rebuilds a slice and ranges a `sync.Map` per pass.
- `planner.go:312-335` spawns one goroutine per symbol per pass (bounded by
  `errgroup.Group` default limit? — here no `SetLimit` is called, so it is unbounded and will
  spawn a goroutine per symbol every tick).
- `allocation.go:339-367` `admitBest` sorts `eligible` on every pass.
- `reasoner.go:147-179` holds a single `reasoner.mu` around every `Ingest`, so all symbols'
  measurement ingestion serializes on one mutex — a throughput ceiling as the universe grows.

None of these is fatal at small N, but they are all hot-path allocations/serialization with no
pooling or sharding.

**Resolution:**

- Add `g.SetLimit(...)` (or `g2.SetLimit`) to the `errgroup.Group`s in `planner.go`, or reuse a
  worker pool; do not spawn unbounded goroutines per symbol per tick.
- Shard the reasoner lock by symbol (or replace `sync.Mutex` + map with a concurrent
  structure) so different symbols' `Ingest` calls do not contend.
- Memoize/reuse the sort buffers and the `drainPending` slice across passes instead of
  allocating fresh each tick.

---

### 2.2 — MEDIUM — `applyAdverseExcursion` / risk-multiple math runs even when there is no entry

`strategy/allocation.go:64-79` (`riskMultiples`) calls `desk.PassageAdverseQuantile(confidence)`
unconditionally before the `hasEntry` check at `allocation.go:101-112`. On ticks with no
entry decision, the quantile is still computed.

**Resolution:** lazy-load the excursion/risk multiples inside the branch that actually has an
entry to price, so the no-op passes stay cheap.

---

### 2.3 — MEDIUM — `bookOscillators` relies on a mutating sort + many small allocations per symbol per settle

`logic/manifold/solver.go:519-766` builds `orders` (a slice of anonymous structs), sorts it
twice (`sort.Slice` with closures at line 591 and 658), builds `ages`, `logSizes`, `ageOrder`,
`ageRank`, `queueRank`, `sideCount`, `prior`, `next` — all fresh per `load` call, which runs
on the manifold's `step_interval` (50ms default) for the entire universe. With `oscillatorPoolCapacity = 1024*68 ≈ 69k` and ~68 orders/symbol, this is repeated allocation + closure-based
sorting in the physics hot loop.

**Resolution:** pre-allocate/reuse the osc/ages/rank slices on the `Solver` struct, and replace
the two `sort.Slice` closure sorts (which allocate the comparator closure and do runtime
interface calls) with `sort.SliceStable` on a pre-allocated index, or an insertion path that
leverages the already-sorted book traversal. The `sort.Strings(symbolNames)` per `load` is fine.
Profile first with `make run-profile` + `profile-report` to confirm the allocation wall before
optimizing the wrong thing.

---

### 2.4 — LOW — `price.Fee`/`WithFee` allocate new decimals on every call

`broker/price_fee.go:205-218` allocates a fresh `decimal.NewFromInt64(0).Add(...)` chain per
call. `feeRate` is recomputed (`.Fee.Div(100)`) on every `WithFee`/`EntryCost` invocation.

**Resolution:** cache the per-symbol fee **rate** (not the raw percent) when `GetFees` loads it,
and reuse it in `WithFee`/`EntryCost`/`Mark`. This is a micro-opt; only worth doing if the
`profile` output shows `decimal` allocations as a hot spot.

---

## 3. UI Wiring Mismatches

### 3.1 — HIGH — `registerKey="activity"` has no producer

`frontend/src/components/engine.tsx:94` uses `<Component registerKey="activity">`, and
`frontend/src/providers/ws-stores.ts:330` special-cases `key === "activity"`. But no
`Frame.*` type maps to an `activity` wire key in `ws-flatbuffers.ts` `decodeTelemetryTable`
(there is no `ActivityFrame` in `telemetry/telemetry.fbs`), and no other producer emits it. The
binding is permanently empty.

**Resolution:** remove the `registerKey="activity"` and the `applyFrame` `"activity"` special
case, or add an actual `ActivityFrame` producer on the Go side if an activity readout was
intended. Do not leave a paint target that can never receive data.

---

### 3.2 — MEDIUM — `balances` and `error` FlatBuffers frames have no consumer

`decodeTelemetryTable` emits `{ balances: ... }` (`BalancesFrame`) and `{ error: ... }`
(`ErrorFrame`), but nothing in `frontend/src` subscribes to the `balances` or `error` keys
(the `"error"` hits in the tree are unrelated string-literal status strings, and `balances`
only appears inside `ws-flatbuffers.ts` itself). These frames are decoded per-frame on the
hot path and then dropped.

**Resolution:** either wire a component to `registerKey="balances"` (and surface the error
frame), or stop emitting/decoding them. Dropping decoded payloads is wasted CPU and a silent
signal that the account/error surfaces are not actually displayed.

---

### 3.3 — MEDIUM — Two flatbuffer identifiers (`SYMM` vs `SYMB`) and two decode paths can disagree

- `decodeTelemetryFrame` checks `Envelope.bufferHasIdentifier` → identifier `SYMM`.
- `openTelemetryBatch` / `decodeTelemetryBatch` check `__has_identifier("SYMB")` → `SYMB`.

The Go side must emit exactly one of these per path; if a producer and consumer are ever
paired incorrectly the error is a runtime throw on every frame. This is fragile and relies on
everyone remembering which identifier goes with which envelope.

**Resolution:** centralize the identifier as a single exported constant consumed by both the
encoder (Go) and decoder (TS) — ideally generated into both `telemetry/generated` and
`frontend/src/providers/telemetry` from the same `.fbs` via `flatc`, rather than hand-retyped
in three places.

---

### 3.4 — MEDIUM — `fluidPhase` vs `fluid` dual transport is easy to miswire

The manifold publishes `FluidPhaseFrame` (flatbuffer, decoded to key `fluidPhase`, consumed by
`fluid-3d/wire.ts:131` `frame.fluidPhase`) **and** raw field/particle slabs over the WebRTC
`ChannelFluid` path (`logic/manifold/solver.go:825-832`, Makefile calls out
`http://127.0.0.1:8765/webrtc/manifold`). Two different transports carry "fluid" data with two
different key names (`fluidPhase` vs the WebRTC slabs), and the phase frame is only
`FluidPhaseFrame` while particles/fields bypass flatbuffers entirely.

**Resolution:** document (or better, type) the two channels separately at the boundary so a
rename of one cannot silently break the other; add an integration test in `tests/` that replays
a manifold frame and asserts both the flatbuffer `fluidPhase` decode and the slab decode
succeed against the same capture.

---

### 3.5 — MEDIUM — `diagnostics` uses a separate transport, separate from the DiagnosticsFrame

`frontend/src/components/dashboard/diagnostics-transport.ts` opens its own `diagnosticsChannel`
and reads `.diagnostics` from *that*, while `ws-flatbuffers.ts` also decodes a `DiagnosticsFrame`
→ `{diagnostics: ...}`. So there are two "diagnostics" data paths with overlapping names but
different transport. Confusing, and a likely source of "the graph shows nothing" bugs if one
side is stopped.

**Resolution:** converge on one transport for diagnostics (prefer the flatbuffer
`DiagnosticsFrame` already defined in `.fbs`), delete the bespoke channel, and keep the
`diagnosticNames` camel→snake mapping (`ws-flatbuffers.ts:256-268`) in exactly one place.

---

### 3.6 — LOW — `BalancesFrame` amount is stored as `string`, painted as a number

`telemetry/telemetry.fbs:35` declares `Balance { asset:string; amount:string; }` (and
`EquityFrame` also carries `string` cash/unrealized/equity). `Balance.tsx` paints them with
`data-paint-format=".2f"`. If the paint system does `parseFloat`/`toFixed` on the string this
works; if it treats any string as a literal, the `.2f` numeric format is a no-op or produces
`NaN`. Either confirm the paint layer coerces `string → number` before formatting, or carry
these as `double` in the schema.

**Resolution:** carry money as `double` (or a scaled `long`) in the `.fbs` schema rather than
`string`, so the frontend, Go, and any future consumer share one unambiguous numeric type and
formatting works uniformly. This also avoids locale/decimal-separator bugs.

---

## Summary

| Severity | Finding | Where |
|---|---|---|
| Critical | Two divergent nomagique trees; `replace` targets stale copy; cross-tree imports; reverse dep | `go.mod`, `./nomagique/*`, `logic/*/solver.go`, `types/decision.go` |
| Critical | MCTS economic model double-counts entry fee + wrong-side mark | `nomagique/mcts/state.go`, `strategy/planner.go:627-659` |
| High | Dropped `errnie` returns swallow construction errors | `logic/manifold/solver.go:141-149` |
| High | `standardize` silent `0` fallback | `logic/resonance/solver.go:399-403` |
| High | Unbounded goroutines per symbol per tick + single reasoner mutex | `strategy/planner.go:312`, `reasoner.go:158` |
| High | `registerKey="activity"` has no producer | `engine.tsx:94`, `ws-flatbuffers.ts` |
| Medium | Stoploss drawdown treats directional prediction as log return | `types/stoploss.go:831-851` |
| Medium | Legacy score fields wired but never populated | `types/decision.go`, `decision_wire.go` |
| Medium | `balances`/`error` frames decoded but never consumed | `ws-flatbuffers.ts` |
| Medium | `SYMM` vs `SYMB` identifiers + dual decode paths | `ws-flatbuffers.ts`, `.fbs` |
| Medium | `fluidPhase` (flatbuffer) vs `fluid` (WebRTC) dual transport | `solver.go`, `fluid-3d/*` |
| Medium | Duplicate diagnostics transports | `diagnostics-transport.ts` vs `DiagnosticsFrame` |
| Medium | `bookOscillators` allocation-heavy in physics loop | `logic/manifold/solver.go:519-766` |
| Medium | `riskMultiples` computed even when no entry | `strategy/allocation.go:64` |
| Low | Fee-rate recomputed per call | `broker/price_fee.go` |
| Low | Money as `string` in `.fbs` | `telemetry/telemetry.fbs:34-35` |

The two **Critical** findings (1.1 and 1.2) are the highest-leverage: they affect the
*identity* of the math library the system computes with, and the *accounting* the strategy
optimizes against. Both should be fixed and covered by a deterministic unit test before any
further feature work, because every downstream signal, decision, and PnL figure inherits the
wrongness.
