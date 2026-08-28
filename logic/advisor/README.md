# Advisor / Perspective Layer

## 1. Purpose

Advisors are SYMM's descriptive context layer. An Advisor is one operational
component that composes already-produced **Measurements** into bounded resident
state and emits a **Perspective** — its current descriptive output.

A Perspective answers:

> **What context is relevant to the current symbol, relationship, market state, or position?**

It never chooses an action, imposes a gate, or asserts an opportunity score.

The governing principle is:

> **Advisors advise. Consumers do their own research.**

## 2. Contract

Every Advisor:

1. subscribes to the `ChannelMeasurements` stream;
2. consumes each Measurement exactly once;
3. mutates bounded resident state through a `nomagique.Number` pipeline;
4. emits a Perspective;
5. retains no unbounded event backlog;
6. never reconstructs a world snapshot to process one event.

## 3. One Advisor Type, One Pipeline Per Instance

There is exactly one `Advisor` Go type. It hosts a caller-supplied
`nomagique.Number` pipeline — the bounded per-symbol resident state plus
whatever mathematics that pipeline expresses — a set of `MetricBinding` values
declaring which measurement metrics feed it, and a set of `Output` values
declaring which named facts the pipeline emits back.

**The pipeline is not universal, and neither is its output shape.** There is
no single mathematics every Advisor runs, and Advisor never assumes a fixed
number or meaning of readings per composed metric. Each Advisor instance
receives its own pipeline, bindings, and outputs at construction:

```go
liquidityAdvisor := NewAdvisor(
    "advisor:liquidity",
    types.KindLiquidity,
    LiquidityPipeline(bindings),
    bindings,
    LiquidityOutputs(bindings),
)
```

Different Advisor families are different pipelines composed through the same
`Advisor` type — never a distinct Go struct per family (no
`LiquidityAdvisor`, `MorphologyAdvisor`, `CoordinationAdvisor`, ...). The
Liquidity family's temporal-context composition
(`temporal.Window → statistic.ZScore → statistic.Baseline → statistic.Velocity`,
namespaced per bound metric and gated on freshness) lives in `liquidity.go` as
`LiquidityPipeline` — a named function returning one Advisor's mathematics,
not a second Advisor type. It declares four named outputs per bound metric
(value, baseline, z-score, velocity) via `LiquidityOutputs`; a future family
with different mathematics declares however many outputs its own pipeline
produces, from one to many, and Advisor's projection code never has to change.

## 4. One Number Per Logical Subject

An Advisor exists to **compose** multiple already-measured facts about one
subject, not to run each metric through an isolated stream. The Liquidity
Advisor is keyed by `symbol` alone: every bound metric for one symbol —
`liquidity/relative_spread`, `liquidity/touch_notional_imbalance`,
`depthflow/book_imbalance` — merges into the *same* committed
`nomagique.Number` state for that symbol, exactly as `Number.Step` already
merges an incoming Frame over the committed Frame before running the pipeline.

Each `MetricBinding` declares a measurement `Source` and `Metric` name plus a
distinct series `Prefix`. The prefix namespaces that metric's interned Frame
slots (its value, event time, and derived estimator state) so several bindings
compose into one Frame without collision — not so each metric gets its own
independent `Number`.

## 5. Fresh-Event Semantics

Because the composed inputs arrive independently (a liquidity Measurement now,
a depthflow Measurement later), `Number.Step`'s merge means every bound
metric's pipeline branch sees every metric's *retained* value and event time
on *every* call — not just the metric this call's Measurement actually
delivered. A branch cannot tell "this event supplied this fact" from "this
fact is still sitting in committed state from an earlier, unrelated event"
without an explicit marker.

`MetricBinding.Fresh` is that marker: `Advisor.Step` sets it in the incoming
Frame only for the bindings this specific Measurement carries. Each pipeline
branch gates its own advance on `Fresh`: fresh, it runs its own stage; not
fresh, it is a deliberate no-op — the frame returns exactly as given, with no
error, never a condition for anything downstream to interpret or forgive.
`Fresh` already says everything there is to say about whether a branch
applies to this event, so no branch needs a permissive combinator that infers
absence from whether a frame happened to change: a fresh branch's genuine
error (malformed input, a regressed clock, whatever) always propagates
immediately and unconditionally.

The `Fresh` marker itself must never survive into what gets committed, or it
would read as fresh again on every later call regardless of what that call
delivered — `Number.Step` commits whatever the pipeline returns, and `Merge`
only overlays populated slots, never clearing one absent from a later call's
own input (and a not-fresh branch passing its input through unchanged does not
clear it either). Every family's pipeline therefore runs an explicit
`scrubFresh` stage after composing its branches, deleting every binding's
marker from the composed output regardless of which branches ran.

Together these are what let independently-arriving Measurements for the same
symbol all contribute to the same committed state without either erasing or
duplicating one another.

### 5.1 Composing two or more genuinely stateful branches: `ForkStrict`, not `Fork`

When one Measurement carries two or more bound metrics at once, two or more
branches are fresh in the same Step, and each branch's returned frame is a
*full copy* of the shared input — including every other binding's
already-populated prior state, untouched. Plain `nomagique.Fork` overlays each
branch's output onto the composed result unconditionally and in sequence, so a
later branch's untouched copy of an earlier branch's freshly mutated slot
silently reverts it back to its stale value. This is a real bug, not a
theoretical one: it reproduces with two Liquidity metrics arriving on the same
ticker Measurement, and it was only caught while building Historical
Analogue's multivariate composition, where two or three dimensions are
routinely fresh together. `nomagique.ForkStrict` compares each branch's output
against the shared input to isolate only what that branch actually changed
before merging, so two branches that each mutate only their own series never
step on one another regardless of merge order. Every Advisor pipeline that
composes more than one stateful branch (`LiquidityPipeline`,
`HistoricalPipeline`) uses `ForkStrict`.

## 6. Ingress Bindings and Quality Provenance

A `MetricBinding` names one measurement fact, its destination series, and
where its source quality provenance is projected:

```go
MetricBinding{
    Source: "liquidity",
    Metric: "relative_spread",
    Prefix: "advisor/liquidity/relative_spread",
    // Series, Fresh, Maturity, SNR, SNRDefined resolved from Prefix.
}
```

`Advisor.Step` scans its bindings against one Measurement, projects every
matching metric's value, event time, `Fresh` marker, and the source
Measurement's own `Maturity`/`SNR`/`SNRDefined` into a small incoming Frame,
and steps the Number for the Measurement's symbol. A Measurement carrying no
bound metric returns `nil` — no context is asserted, because none was
observed. An Advisor composes already-produced Measurements and must not
discard or re-derive the quality facts they already established.

## 7. Perspective

`types.Perspective` is the fixed-size wire value: a symbol/peer identity, an
interned `PerspectiveKind`, an `At` timestamp, a fixed-size array of
`MetricReading` values, and `Err`. `Count` says how many readings are
populated.

`Err` carries a genuine pipeline transition failure for this Step — absence is
already handled by the `Fresh` no-op, so a non-nil `Err` here is always a real
defect. `Number` only commits successful output, so on a failure the Readings
still reflect the last successfully committed state, not this event's
contribution — a consumer must check `Err` before trusting them.

Each `MetricReading` is one named fact a pipeline emitted: `Metric` is the
interned identity of the value (a bound metric's raw value, or one of its
derived statistics), `Value` is the value itself, and `Defined` is false until
the pipeline has actually produced that fact — so an undefined reading's zero
`Value` is never mistaken for a real, observed zero. `Maturity`, `SNR`, and
`SNRDefined` carry that fact's own provenance forward. A consumer determines
what a reading means from its `Metric` identity, never from its position in
the `Readings` array — and Perspective itself never assumes a pipeline
produces any particular shape of readings per composed metric (not "value
plus baseline plus z-score plus velocity"; that shape belongs to whichever
pipeline happens to produce it).

`PerspectiveMetricCapacity` bounds `Readings` to a fixed-size array; Liquidity's
12 declared outputs (3 bound metrics × 4 each) fill it exactly, with no spare
capacity. `NewAdvisor` panics at construction if a pipeline declares more
outputs than that — a wiring-time structural mismatch, not a runtime condition
to degrade through, so a future wider pipeline fails loudly instead of
silently losing readings.

### 7.1 `Output` provenance is declared per output, not inferred from one metric

`Output{Slot, Maturity, SNR, SNRDefined}` declares one named fact plus where
*that fact's own* quality provenance lives — it does not require a single
parent `MetricBinding`. `NewMetricOutput(slot, binding)` is the honest shape
for an output derived from exactly one measurement (every Liquidity output:
current value, baseline, z-score, and velocity are all properties of one
bound metric, so reusing that metric's own `Maturity`/`SNR`/`SNRDefined` is
truthful). `NewDerivedOutput(slot, maturitySlot)` is the honest shape for an
output derived from several composed metrics jointly (Historical Analogue's
distance/percentile/match-count, each a property of the whole multivariate
comparison, not of any one bound dimension) — it declares its own maturity
slot and leaves `SNR`/`SNRDefined` explicitly undeclared, since no principled
SNR definition applies to a nearest-neighbor distance.

An `Output` field left at its Go zero value would silently read back
`Symbol(0)` — some other package's real, unrelated interned slot, decided by
whichever order `init()` functions across the whole program happened to run
in. `undeclaredProvenance` is the dedicated, permanently-unpopulated sentinel
`NewDerivedOutput` uses instead, so `Get` on an undeclared provenance field
always and honestly reports "not found," never a fact that happens to belong
to something else entirely.

## 8. Live Composition

`cmd/boot.go` wires two Advisor instances, each an independent `WireKeyed`
subscriber over the same `ChannelMeasurements` → `ChannelPerspectives` pair —
no second Perspective transport, no new channel architecture, one subscriber
per family, keyed by symbol.

**Liquidity** (`NewLiquidityAdvisor`) composes three measurement metrics:

- `liquidity/relative_spread`
- `liquidity/touch_notional_imbalance`
- `depthflow/book_imbalance`

describing, per symbol, how unusual the current execution terrain (spread,
touch capacity, book imbalance) is relative to its own history, and to each
other within the same committed state.

**Historical Analogue** (`NewHistoricalAdvisor`, `historical.go`) composes
three already-standardized measurement metrics into one bounded, causal
multivariate trajectory comparison — see §11.

## 9. Historical Analogue

Historical Analogue answers strategy/ADVISORS.md §10's question: *has this
symbol previously exhibited a trajectory similar to the one observed now, and
how unusual/recurrent is the current trajectory relative to its own retained
history?* It does not predict what happens next.

### 9.1 Selected inputs and why

`HistoricalBindings()` binds three genuinely complementary,
**already-standardized** metrics (verified against the live signal source,
not invented):

| Source | Metric | Structural family |
|---|---|---|
| `cvd` | `signed_net_fraction_zscore` | executed-flow (aggressor-side imbalance) |
| `depthflow` | `book_imbalance_zscore` | liquidity/book (displayed asymmetry) |
| `hawkes` | `excitation_fraction:buy` | event/excitation (a dimensionless state of the arrival process, not an event residual) |

Each is already a dimensionless z-score published causally by its own signal
(signal/README.md §12: normalize only when the normalization has an intrinsic
mathematical interpretation), so the Advisor performs **no further
normalization** — recomputing a second normalization universe here would
silently redefine what "close" means. The three dimensions cover distinct
structural families with no redundancy, matching the smallest-sufficient-state-
vector guidance in strategy/ADVISORS.md §10.2. `Category` and `Manifold`,
mentioned as examples in that section, are **not** Measurement sources in this
codebase — both are downstream consumers of Measurements (`logic/category`,
`logic/manifold`) that never call `Project(...)` with their own `Source`
string, so they cannot be bound the way a real signal can.

### 9.2 Mathematics: bounded causal matrix-profile self-join

`nomagique/recurrence.Analogue` (a new, generic nomagique primitive — the
comparison mathematics does not belong inside `logic/advisor`) retains each
bound dimension as its own `temporal.Path` and performs a self-join matrix
profile over the joint trajectory, per signal/README.md §10:

- each dimension is retained as a timestamped `temporal.Path`, and the three
  streams are aligned in **wall-clock time**, not by sample ordinal: CVD and
  Hawkes observe on trades while Depthflow observes on L3 book events. Each
  dimension is a piecewise-constant step function of time — its last observed
  value holds until the next observation — and never a resampled grid;
- the comparison horizon `Q` is **not** a sample-count split. It is an explicit
  control fact — the symbol's own Hawkes excitation e-folding timescale
  `tau = 1/beta` — wired into `recurrence.Analogue` by `HistoricalPipeline`,
  never hardcoded inside recurrence and never a magic constant;
- a query window is `[now−Q, now]`; every earlier non-overlapping window of
  the same duration, stepped back through the entire retained joint history,
  is searched — the nearest analogue is reachable however far back it
  occurred, and `match_count` grows with retained history rather than being
  capped at a fixed small number;
- distance is the **exact time-weighted RMS** of the squared difference of the
  two step functions, integrated over the window's actual change points and
  divided by elapsed time (no invented grid resolution). A period a dimension
  did not observe contributes zero width, never a fabricated zero value;
- the percentile is a genuine **causal percentile**: today's nearest distance
  is ranked against a bounded history of prior scans' nearest distances, as
  the fraction of priors that were closer. Near 0 the nearest match is
  unusually close (familiar/recurring); near 1 it is unusually far (novel).
  Its support grows with history up to the bounded baseline;
- `maturity` follows the spec formula (signal/README.md §8) applied to the
  scan's own effective support, the candidate count actually searched.

### 9.3 Outputs and their meaning

| Output | Meaning |
|---|---|
| `recurrence/nearest_distance` | distance to the closest non-overlapping prior trajectory (time-weighted RMS) |
| `recurrence/nearest_percentile` | causal percentile vs prior nearest distances: 0 familiar/recurring, 1 novel |
| `recurrence/match_count` | how many non-overlapping candidate windows were searched |
| `recurrence/match_from_unix_sec` / `_nsec` | when the nearest match began |
| `recurrence/query_length` | the comparison horizon `Q` (seconds) the scan used |
| `recurrence/maturity` | this comparison's own effective-support quality (`NewDerivedOutput`, not borrowed from any one bound metric) |

`Defined=false` (not a fabricated zero) until the horizon `Q` has been wired,
the query window contains a full aligned observation on every dimension, and
at least one non-overlapping candidate window exists entirely within retained
history — see §7.1 for why `SNR`/`SNRDefined` stay explicitly undeclared for
these outputs.

### 9.4 What this Advisor explicitly does not claim

Per strategy/ADVISORS.md §10.4: a low distance means a familiar path, not a
predicted outcome. Historical Analogue never reports a probability, a
regime label, a bullish/bearish characterization, or a buy/sell/hold — only
recorded structural similarity to the symbol's own retained history.

### 9.5 Live cost

State is `O(3 bounded Path series + 1 control fact + 1 bounded baseline ring)`
per symbol — each capped at `temporal.MaxPathSamples` (the percentile baseline
at its own smaller `baselineCapacity`). Per-Step cost is `O(dimensions ×
changePoints × matchCount)` — change points are the union of retained
observations inside a window, and `matchCount` is the number of candidate
windows actually searched, both bounded by the retained-history ceiling; no
per-event allocation beyond the emitted `*Perspective` plus the small
per-window change-point slices. No cross-symbol state, no O(symbols²) work, no
unbounded retained history.

## 10. Non-Goals

- no universal Advisor mathematics — the pipeline is supplied per instance;
- no universal output shape — each pipeline declares its own Outputs;
- no per-metric Number instances — one logical subject owns one Number;
- no universal scoring system or generic confidence scalar;
- no hard-coded "bot" or manipulation labels;
- no direct trading authority — a Perspective is never a buy/sell/hold;
- no re-deriving a raw signal: the advisor composes already-emitted Measurements;
- no world snapshots, cloned histories, or unbounded backlogs;
- no permanent all-to-all pair state;
- no distinct Advisor Go type per family.

## 11. Deferred (blocked on upstream measurement work)

- **CoordinationAdvisor pipeline** (named-pair coupling) — the lead-lag and
  correlation signals emit per-symbol cohort aggregates, not named-pair
  measurements, so a pair identity has nothing to consume. Requires the
  named-pair admission path (spec §17/§38: Research Catalog, spot/perpetual,
  or explicit config).
- **MorphologyAdvisor pipeline** (normalized book morphology) — the depthflow
  signal emits flow/mutation metrics, not the structural measures the spec
  lists (normalized entropy, Herfindahl concentration, spacing regularity,
  size quantization). Requires those measurements to exist first.
