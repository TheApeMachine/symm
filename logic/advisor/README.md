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

## 3. One Advisor Type, Composed by a Pipeline

There is exactly one `Advisor` type. It wraps a `nomagique.Number` pipeline —
the bounded per-symbol, per-metric resident state plus its derived statistics —
and is configured with the set of measurement metrics it composes.

Each composed metric runs through the same temporal-context stage:

```text
temporal.Window → statistic.ZScore → statistic.Baseline → statistic.Velocity
```

so each metric's current value, adaptive baseline, z-score (departure from that
baseline in units of its own dispersion), and first difference are derived from
the metric's own event-time history. The `MetricBinding{Source, Metric}` declares
which measurement feeds which stream; the `Source` disambiguates a metric name
emitted by more than one signal.

## 4. Perspective

`types.Perspective` is the fixed-size wire value: a symbol/peer identity, an
interned `PerspectiveKind`, an `At` timestamp, a monotonic `Sequence`, and a
fixed-size array of `MetricReading` values (`Value`, `Baseline`, `ZScore`,
`Velocity`, `Ready`). `Count` says how many readings are populated.

Definedness is explicit: a reading's `Ready` is false until every derived slot
exists, so a not-ready reading's zeros are never mistaken for a real estimate.
Each Advisor family owns a distinct `PerspectiveKind` so perspectives do not
collide on identity.

## 5. Live Composition

`cmd/boot.go` wires one `KindLiquidity` advisor over three measurement metrics:

- `liquidity/relative_spread`
- `liquidity/touch_notional_imbalance`
- `depthflow/book_imbalance`

describing, per symbol, how unusual the current execution terrain (spread, touch
capacity, book imbalance) is relative to its own history.

## 6. Non-Goals

- no universal scoring system or generic confidence scalar;
- no hard-coded "bot" or manipulation labels;
- no direct trading authority — a Perspective is never a buy/sell/hold;
- no re-deriving a raw signal: the advisor composes already-emitted Measurements;
- no world snapshots, cloned histories, or unbounded backlogs;
- no permanent all-to-all pair state.

## 7. Deferred (blocked on upstream measurement work)

- **CoordinationAdvisor** (named-pair coupling) — the lead-lag and correlation
  signals emit per-symbol cohort aggregates, not named-pair measurements, so a
  pair identity has nothing to consume. Requires the named-pair admission path
  (spec §17/§38: Research Catalog, spot/perpetual, or explicit config).
- **MorphologyAdvisor** (normalized book morphology) — the depthflow signal
  emits flow/mutation metrics, not the structural measures the spec lists
  (normalized entropy, Herfindahl concentration, spacing regularity, size
  quantization). Requires those measurements to exist first.
