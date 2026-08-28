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
branch gates its own advance on `Fresh`: fresh, it runs its temporal-context
stage; not fresh, it is a deliberate no-op — the frame returns exactly as
given, with no error, never a condition for anything downstream to interpret
or forgive. `Fresh` already says everything there is to say about whether a
branch applies to this event, so the branches compose with plain
`nomagique.Fork`, not a permissive variant that infers absence from whether a
frame happened to change: a fresh branch's genuine error (malformed input, a
regressed clock, whatever) always propagates immediately and unconditionally.

The `Fresh` marker itself must never survive into what gets committed, or it
would read as fresh again on every later call regardless of what that call
delivered — `Number.Step` commits whatever the pipeline returns, and `Merge`
only overlays populated slots, never clearing one absent from a later call's
own input (and a not-fresh branch passing its input through unchanged does not
clear it either). `LiquidityPipeline` therefore runs an explicit `scrubFresh`
stage after `Fork` on every call, deleting every binding's marker from the
composed output regardless of which branches ran.

Together these are what let a liquidity Measurement and a later depthflow
Measurement for the same symbol both contribute to the same committed state
without either erasing or duplicating the other.

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

Each `MetricReading` is one named fact a pipeline emitted for one composed
metric: `Metric` is the interned identity of the value (a bound metric's raw
value, or one of its derived statistics), `Value` is the value itself, and
`Defined` is false until the pipeline has actually produced that fact — so an
undefined reading's zero `Value` is never mistaken for a real, observed zero.
`Maturity`, `SNR`, and `SNRDefined` carry the composed metric's own
provenance forward. A consumer determines what a reading means from its
`Metric` identity, never from its position in the `Readings` array — and
Perspective itself never assumes a pipeline produces any particular shape of
readings per composed metric (not "value plus baseline plus z-score plus
velocity"; that shape belongs to whichever pipeline happens to produce it).

`PerspectiveMetricCapacity` bounds `Readings` to a fixed-size array; Liquidity's
12 declared outputs (3 bound metrics × 4 each) fill it exactly, with no spare
capacity. `NewAdvisor` panics at construction if a pipeline declares more
outputs than that — a wiring-time structural mismatch, not a runtime condition
to degrade through, so a future wider pipeline fails loudly instead of
silently losing readings.

## 8. Live Composition

`cmd/boot.go` wires one `KindLiquidity` advisor (`NewLiquidityAdvisor`) over
three measurement metrics:

- `liquidity/relative_spread`
- `liquidity/touch_notional_imbalance`
- `depthflow/book_imbalance`

describing, per symbol, how unusual the current execution terrain (spread,
touch capacity, book imbalance) is relative to its own history, and to each
other within the same committed state.

## 9. Non-Goals

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

## 10. Deferred (blocked on upstream measurement work)

- **CoordinationAdvisor pipeline** (named-pair coupling) — the lead-lag and
  correlation signals emit per-symbol cohort aggregates, not named-pair
  measurements, so a pair identity has nothing to consume. Requires the
  named-pair admission path (spec §17/§38: Research Catalog, spot/perpetual,
  or explicit config).
- **MorphologyAdvisor pipeline** (normalized book morphology) — the depthflow
  signal emits flow/mutation metrics, not the structural measures the spec
  lists (normalized entropy, Herfindahl concentration, spacing regularity,
  size quantization). Requires those measurements to exist first.
