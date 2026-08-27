# Advisor / Perspective Layer

## 1. Purpose

Advisors are SYMM's descriptive context layer. An Advisor is an operational
component that consumes already-produced observations, maintains bounded
resident state, and emits a **Perspective** — its current descriptive output.

A Perspective answers:

> **What context is relevant to the current symbol, relationship, market state, or position?**

It never chooses an action, imposes a gate, or asserts an opportunity score.

The governing principle is:

> **Advisors advise. Consumers do their own research.**

## 2. Contract

Every Advisor:

1. subscribes to one or more typed streams;
2. consumes each event exactly once;
3. mutates bounded resident state;
4. optionally emits a Perspective;
5. retains no unbounded event backlog;
6. never reconstructs a world snapshot to process one event.

## 3. Perspective

`types.Perspective` is the fixed-size wire value: a symbol/peer identity, an
interned `PerspectiveKind`, a `From`/`At` window, a monotonic `Sequence`, a
bounded `Maturity`, and a kind-specific value payload. Structural identity is
`types.PerspectiveKey` (Symbol, Peer, Kind) — a comparable value type, never a
built string.

Definedness is explicit: each payload carries an integer support count, and the
accompanying floats are meaningless when that count is zero. A missing
historical-analogue model is never rendered as `distance = 0`.

## 4. HistoricalAnalogueAdvisor

Answers, per symbol:

> **Has this symbol previously exhibited a regime trajectory similar to the one observed now, and where does the present trajectory sit relative to those archived episodes?**

It consumes the ranked `[]types.Category` batch (`ChannelCategories`), reduces
each tick to its dominant regime (interned category index), and matches the
in-progress trajectory against the symbol's own bounded archive of completed
trajectory windows.

Outputs:

- `Support` — archived episode count (zero means no comparison was possible);
- `NearestDistance` — minimum normalized Hamming distance to an archived window;
- `MedianDistance` — the archive's own typical self-distance, the honest scale
  against which "unusually close" is judged without a tuned threshold;
- `StageAlignment` — the in-progress trajectory's fill fraction.

It deliberately does **not** fabricate a `P(VerticalIgnition)` from the analogue
count, nor a distance "percentile" with no null model. Those calibrated
quantities belong to offline research (the Research Catalog), not to this live
descriptive stage.

## 5. RelativeStateAdvisor

Answers, per symbol:

> **How does this symbol's current regime compare with the rest of the market population's regimes?**

It consumes the same ranked `[]types.Category` batch, maintains a bounded
population of each symbol's current dominant regime, and reports measured
cross-sectional facts:

- `PeerCount` — population size (zero means no comparison was possible);
- `SameRegime` — symbols sharing this symbol's regime;
- `Breadth` — `SameRegime / PeerCount`, the fraction of the population in the
  same regime;
- `MajorityRegime` / `MajorityBreadth` — the most frequent regime and its share.

These are measured shares, never an outlier/leader classification.

## 6. Non-Goals

- no universal scoring system or generic confidence scalar;
- no hard-coded "bot" or manipulation labels;
- no direct trading authority — a Perspective is never a buy/sell/hold;
- no world snapshots, cloned histories, or unbounded backlogs;
- no permanent all-to-all pair state.

## 7. Deferred (blocked on upstream measurement work)

- **CoordinationAdvisor** (named-pair coupling) — the lead-lag and correlation
  signals emit per-symbol cohort aggregates, not named-pair measurements, so a
  pair identity has nothing to consume. Requires the named-pair admission path
  (spec §17/§38: Research Catalog, spot/perpetual, or explicit config).
- **MorphologyAdvisor** (normalized book morphology) — the depthflow signal
  emits flow/mutation metrics (book_imbalance, turnover, resolution_gap) but not
  the structural measures the spec lists (normalized entropy, Herfindahl
  concentration, spacing regularity, size quantization). Requires those
  measurements to exist first.
