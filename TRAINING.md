# TRAINING.md

The current `training.json` scans the archive, orders capture records, mines
excursions, and then traverses the captured records through the existing signal
graphs and A/B/C grader. `training_replay.json` owns replay advancement in JSON;
`training_fragment.json` separates causal observations from future event labels.
Overlapping fragments do not deliver the same record to the signals twice.
The pair-evidence stage of the remapper is composed in `training_pair.json` and
`training_pair_step.json`. Spatial settling, region tokens, and the connection
to `training_reinforce.json` remain unimplemented.

How the system learns. Read `ARCHITECTURE.md` first for how the graph is built
and run; this describes what is built on top of it.

## What is being predicted

**Precursors, not price.**

The system does not forecast a return. It learns to recognise the shape the
market makes *before* something happens, and to say what to do about it. A
prediction is an action — wait, enter, exit — judged against what the tape
actually went on to do.

This distinction decides everything downstream. A model that predicts price is
graded on error against a number. A model that predicts precursors is graded on
whether it called the turn in the right place, which is why the ground truth
below is a set of points on the tape rather than a series of values.

## The pipeline

```
raw market data ──► virtual grid ──► impulse map ──► region token ──► radix trie
   (5 streams)      (metrics)        (remapper)      (N hot regions)   (WAIT/ENTER/EXIT)
```

There is no longer an agent. The trie is the thing that decides; the layer that
used to deliberate over advisors and plans is gone.

## 1. The virtual grid

**Status: built.** `nomagique/store/grid.go`.

Raw market data from every stream — spot ticker, spot trade, spot level3,
futures ticker, futures trade — is written to one grid. The grid holds no
values of its own.

A metric registers with the grid by declaring the fields it needs
(`trade.data.price`, `ticker.data.qty`, and so on). When the grid is written
to, each metric receives exactly the fields it asked for and nothing else, so a
metric is never handed a frame it cannot read and never has to recognise one it
should ignore. When the grid is read, the metrics are asked for their current
state.

A record carries the fields of the feed it came from, so most deliveries fill
only some of the declared slots. Every declared interest keeps its own slot and
`present` says which ones this record carried. A metric waiting on a slot that
stayed empty keeps waiting — it is never handed a zero standing in for a field
that was not there.

**A metric is one scalar.** It is the first operation, the last operation, and
whatever has to go in between. Nothing wraps it. Quality figures — SNR,
maturity — belong to the measurement the metric sits in, not to the metric.

Per-symbol state is `store.Radix`: a metric keyed by symbol retains its own
estimate for each instrument it observes and continues it on the next
observation.

**Not built yet: the 2D coordinate.** Each metric must be assigned a coordinate
by the grid, and that coordinate is its identity in everything that follows. The
grid currently hands out positional slots, not coordinates.

## 2. The impulse map

**Status: not built.**

A 2D coordinate space where metrics write their observations. Cells light up as
they react to the tape. The map is then reorganised so that cells cluster
*sympathetically*.

Sympathy is a ladder of priorities. They are additive when they occur together,
and each holds on its own.

**First — co-movement.** Values that move together attract.

**Second — relative magnitude.** Among values that move together, those whose
relative magnitudes match most closely attract more strongly.

Sign is irrelevant as long as the match is *consistent*. A rising while B falls
is attraction, provided A falling while B rises also holds. What matters is
that the relationship reproduces, not its direction.

Inconsistency repels. Both of these are inconsistent:

- A+ with B−, then A− with B−
- A+ with B−, then A at zero or absent while B rises

**Third — authority.** Maturity and SNR power the attraction. Where A is
stronger than B, B moves toward A further than A moves toward B. Pull is
asymmetric and weighted by how much each cell has earned the right to pull.

The third priority is what produces structure: hot spots of strong, agreeing
cells, separated by a colder gradient.

### Pair evidence

`training_pair.json` accepts scoped metric-coordinate pairs, exact capture
cursors, and two observed scalar values. It derives reactions between successive
joint observations. Missing readings leave the anchor unchanged; the first joint
reading establishes the anchor. Cursor identities and accumulated statistics are
committed together in `store.Radix`. Duplicate readings do not add support;
conflicting duplicates and new out-of-order readings fail the evaluation.

`training_pair_step.json` contains the arithmetic. On active paired intervals,
let `q = sign(left reaction) * sign(right reaction)`, `N` be support, `Q = sum(q)`,
and `S = sum(q²)`. Consistency is `(Q²-S)/(N*(N-1))`: the average sign-product
agreement over distinct intervals, defined only for `N > 1`. Joint quiet does
not increase support. Observed one-sided nonresponse contributes a separate
repulsion rate. Magnitude match is `sum(abs(left*right)) / sqrt(sum(left²)*sum(right²))`
over the same paired support. It is scale invariant and exists only when both
sides have observed energy. Sympathy adds consistency-weighted magnitude match
to consistency minus nonresponse rate. These are empirical relationship
statistics, not probabilities or confidence claims. The graph publishes the
underlying sufficient statistics alongside them.

### Regions

Regions fall out of the third priority. Their borders lie where the weakest
cells meet.

| 1.0 (A) | 0.5 (A) | 0.25 (A) | 0.25 (B) |
| --- | --- | --- | --- |
| 0.5 (A) | 0.5 (A) | 0.25 (A) | 0.25 (B) |
| 0.25 (A) | 0.25 (A) | 0.25 (B) | 0.5 (B) |
| 0.25 (B) | 0.25 (B) | 0.5 (B) | 1.0 (B) |

Two regions, A and B, with the boundary running through the 0.25 cells — the
coldest ground between them.

### The impulse

The impulse is the set of regions lighting up most strongly. Which regions
qualify has to be **derived from the map's own distribution**, not compared
against a fixed number. Candidates:

- SNR-weighted Otsu's method
- the topological "water table" split
- mean plus standard deviation

Whichever is used, the cut comes from the data in front of it. A hardcoded
threshold here would decide market belief by fiat, which this codebase does not
permit (`AGENTS.md`).

### The remapper

A second virtual grid. It holds a mapping from each metric's original
coordinate to its repositioned one, clusters metric state into regions, and
iterates until the clusters settle.

**Nothing downstream happens until the remapping has settled.** An unsettled map
produces region tokens that mean something different on every pass, and a trie
trained on those learns noise. Settling is a gate, not a preference.

Once settled, it emits the N hottest regions as the **region token**.

## 3. The radix trie

**Status: prediction and truth reinforcement implemented in**
`manifest/training_reinforce.json`, using `store.Radix`, `statistic.Tally`, and
`cognition.Attractor`. The archive graph still needs settled remapper tokens
before it can use this stage.

The predictive structure.

- **Edges** encode region token sequences.
- **Nodes** encode actions: WAIT, ENTER, EXIT.
- A prediction takes the **current region token plus priors** — the sequence
  leading here, not just the latest token.
- A sequence that predicted correctly is **reinforced**, carrying more weight
  next time.

The trie is the whole decision mechanism. There is no second path.

## 4. Ground truth

**Status: capture, excursion mining, record replay, and A/B/C grading are wired
in the training graph.** Flat, resolved non-event fragments are not yet emitted
by the archive replay path.

Training needs to know what actually happened, which means the raw tape has to
be kept.

**Step one — store raw market data in Iceberg tables.** Every stream, as
received. `nomagique/store/tables/table.capnp` has `IcebergTable` and
`IcebergScan`; `store.Capture` now supplies a byte-preserving envelope and `manifest/capture.json`
connects socket receipt metadata to it. A local Iceberg round trip is tested.
The `raw_frames_v3` schema stores explicit capture sessions and numeric sequences.
`store.Tape` orders each session and deduplicates identities before mining.
Live subscriptions and authenticated feeds still require deployment configuration.

**Step two — compute excursions.** `temporal.Mine` processes every record in the
configured channel and keeps separate state per session, endpoint, and symbol.
It uses the canonical `Excursion` calculation and persists confirmed event batches
in `excursion_fragments_v1`. The manifest selects ticker records and `last`.

**Step three — retrieve the fragments** that contain those excursions.
`store.Sequence` retains the ordered records and confirmed events; the JSON
replay cursor visits each record and selects its applicable fragments. Signal
state advances once per record, in tape order. This implementation retains the
offline snapshot in memory and compares each record with every event.

**Step four — mark the three points.** Every fragment carries:

| Point | What it is |
| --- | --- |
| **A** | a random point *before* ignition |
| **B** | ignition |
| **C** | exhaustion, stagnation, or reversal |

A is deliberately random so the model cannot learn a fixed distance from the
event. The leg **A → B** is what entry prediction trains on: the run into
ignition is the precursor. The leg **B → C** is what exit prediction trains on.

A fragment where nothing happens is still training data. The system has to
learn to pass over an illiquid or flat episode, so class boundaries are never
widened to fill an empty class.

## 5. Training on fragments

Fragments are replayed through the full pipeline — grid, impulse map, region
token — and the trie predicts as it goes.

Each decision is graded by **what it would actually have made or lost**. The
tape holds the Level 3 book, so a decision is executed against it:
`paper_exchange.json` rebuilds each symbol's book order by order (verified against
the exchange's checksum), rests the order until the next Level 3 frame for its
symbol, and fills it by walking the queue, taker fee included. A/B/C select
which stretches of tape are replayed; they are not the grade.

The balance is part of what is learned. Each training universe carries its own
account: every ENTER spends 20% of the cash not already committed, every EXIT
sells the whole position, losses compound, and an account that has shrunk
below the exchange's minimums finds its orders refused — a consequence it
lives with, never a case that is skipped or resized. An entry and its exit
share the PnL of the round trip they made together; a refusal the replay
cannot judge (no book, no instrument rules) is unknown, not a loss.

Because the fragment's ground truth is known in advance, this loop is fast and
repeatable, and it can be run over the whole archive.

## 6. Live paper trading

A second process, running at the same time and **not** training on fragments.
It takes the trie as developed and trades it against the live market on paper.

It is evaluated two ways:

- **Equity**, immediately — the running result of its own decisions.
- **Ground truth, once it catches up** — when the tape has moved far enough for
  the excursions around those decisions to be computed, the same A/B/C grading
  used in fragment training is applied to what the live process actually did.

## 7. Promotion

Paper trading is not a rung on a ladder to be climbed on schedule. The live
process switches to real money **only once it has demonstrated robust
profitability** on paper, judged by both signals above.

Paper versus real is a deployment setting, not a stage of learning.

## Where this stands

| Piece | State |
| --- | --- |
| Virtual grid, metric registration, per-symbol state | built |
| Metric 2D coordinates | not built |
| Impulse map and sympathy clustering | not built |
| Remapper and settling gate | not built |
| Region tokens | not built |
| Radix trie | partly built (`nomagique/cognition`) |
| Raw capture into Iceberg | explicit cursors and ordered deduplicated replay built; deployment subscriptions pending |
| Excursion mining | per-symbol mining and persisted event cursors built |
| Fragment retrieval | ordered archive traversal and A–C fragment selection implemented in JSON |
| A/B/C fragment selection | `training_grade.json`, connected to archive fragment replay |
| PnL grading against the L3 book | `paper_exchange.json` wired into `training.json`: replayed records and, until the trie decides, the fragments' own ENTER at B / EXIT at C are one ordered event stream; closed round trips go to `paper_round_trips_v1`. Proven end to end by `TestCompileTrainingPaper` (archive → mining → fills against recorded L3 → positive round trip archived). Fee is the account's measured 0.80% taker as a visible constant |
| Level 3 capture | `capture.json` verified live: 603 L3 symbols admitted in minutes (rate-limited ones retried), instrument/ticker/trade/L3 in one session in `symmtables/symm/raw_frames_v3` |
| Throughput | measured on a live archive: ~909 graph passes/s, 97% of CPU in goroutine park/wake (one Cap'n Proto server hand-off per node call), and the replay loops every mined event per record. A few minutes of full L3 capture (~620k frames) cannot be replayed in useful time yet |
| Fragment training loop | archive → signals and truth grading wired; remapper/token/reinforcement connection pending |
| Live paper process | not built |

The capture is the dependency everything else waits on: without the stored tape
there are no excursions, without excursions there are no fragments, and without
fragments there is nothing to grade a prediction against.

## Rules that apply throughout

These come from `AGENTS.md` and matter especially here, because this is where
the temptation to fudge is strongest.

- **No arbitrary statistical constants.** Thresholds, horizons, windows,
  confidence cuts and region boundaries come from measured statistics —
  support, variance, SNR, event rate, stability — not from a number someone
  picked. Naming a constant does not make it derived.
- **No fallback fakery.** Missing, immature or undefined evidence stays that
  way. Never substitute a convenient default.
- **Never check NaN or Inf.** Let invalid mathematics surface so its cause gets
  fixed.
- **Streaming.** A step gets one observation and one opportunity to process it.
  Update sufficient statistics; do not accumulate history that can be summarised.
