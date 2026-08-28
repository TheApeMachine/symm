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
whatever mathematics that pipeline expresses — and a set of `MetricBinding`
values that declare which measurement metrics feed it.

**The pipeline is not universal.** There is no single mathematics every Advisor
runs. Each Advisor instance receives its own pipeline at construction:

```go
liquidityAdvisor := NewAdvisor(
    "advisor:liquidity",
    types.KindLiquidity,
    LiquidityPipeline(bindings),
    bindings...,
)
```

Different Advisor families are different pipelines composed through the same
`Advisor` type — never a distinct Go struct per family (no
`LiquidityAdvisor`, `MorphologyAdvisor`, `CoordinationAdvisor`, ...). The
Liquidity family's temporal-context composition
(`temporal.Window → statistic.ZScore → statistic.Baseline → statistic.Velocity`,
namespaced per bound metric and folded with `nomagique.TryFork`) lives in
`liquidity.go` as `LiquidityPipeline` — a named function returning one
Advisor's mathematics, not a second Advisor type. A future family with
different mathematics (cross-sectional, relational, morphological) supplies
its own pipeline function the same way.

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

Because the composed inputs arrive independently (a liquidity Measurement now,
a depthflow Measurement later), the pipeline uses `nomagique.TryFork`: each
bound metric's temporal-context branch runs against the full merged state, and
a branch whose own metric has never been observed yet fails before writing
anything and is dropped rather than failing the whole step. Every other
branch's already-composed facts survive untouched. A branch that fails *after*
writing a slot is a genuine defect and still propagates — `TryFork` forgives
absence, never a real error.

## 5. Ingress Bindings

A `MetricBinding` names one measurement fact and its destination:

```go
MetricBinding{
    Source: "liquidity",
    Metric: "relative_spread",
    Prefix: "advisor/liquidity/relative_spread",
    // Series, Baseline, ZScore, Velocity resolved from Prefix at construction.
}
```

`Advisor.Step` scans its bindings against one Measurement, projects every
matching metric's value and event time into a small incoming Frame under that
binding's interned series, and steps the Number for the Measurement's symbol.
A Measurement carrying no bound metric returns `nil` — no context is asserted,
because none was observed.

## 6. Perspective

`types.Perspective` is the fixed-size wire value: a symbol/peer identity, an
interned `PerspectiveKind`, an `At` timestamp, and a fixed-size array of
`MetricReading` values. `Count` says how many readings are populated.

Each `MetricReading` carries the interned `Metric` identity (the bound
series' value slot) alongside `Value`, `Baseline`, `ZScore`, `Velocity`, and
`Ready`. A consumer determines what a reading means from its `Metric`
identity, never from its position in the `Readings` array. Definedness is
explicit: `Ready` is false until every derived slot exists, so a not-ready
reading's zeros are never mistaken for a real estimate.

## 7. Live Composition

`cmd/boot.go` wires one `KindLiquidity` advisor (`NewLiquidityAdvisor`) over
three measurement metrics:

- `liquidity/relative_spread`
- `liquidity/touch_notional_imbalance`
- `depthflow/book_imbalance`

describing, per symbol, how unusual the current execution terrain (spread,
touch capacity, book imbalance) is relative to its own history, and to each
other within the same committed state.

## 8. Non-Goals

- no universal Advisor mathematics — the pipeline is supplied per instance;
- no per-metric Number instances — one logical subject owns one Number;
- no universal scoring system or generic confidence scalar;
- no hard-coded "bot" or manipulation labels;
- no direct trading authority — a Perspective is never a buy/sell/hold;
- no re-deriving a raw signal: the advisor composes already-emitted Measurements;
- no world snapshots, cloned histories, or unbounded backlogs;
- no permanent all-to-all pair state;
- no distinct Advisor Go type per family.

## 9. Deferred (blocked on upstream measurement work)

- **CoordinationAdvisor pipeline** (named-pair coupling) — the lead-lag and
  correlation signals emit per-symbol cohort aggregates, not named-pair
  measurements, so a pair identity has nothing to consume. Requires the
  named-pair admission path (spec §17/§38: Research Catalog, spot/perpetual,
  or explicit config).
- **MorphologyAdvisor pipeline** (normalized book morphology) — the depthflow
  signal emits flow/mutation metrics, not the structural measures the spec
  lists (normalized entropy, Herfindahl concentration, spacing regularity,
  size quantization). Requires those measurements to exist first.
