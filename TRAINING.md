# TRAINING.md

The current `training.json` scans the archive, orders capture records, mines
excursions, and then replays each mined fragment through the signal graphs, the
A/B/C grader and the paper exchange. `training_replay.json` owns replay
advancement in JSON; `training_fragment.json` separates causal observations from
future event labels. Only fragments are replayed, never the whole tape.
The impulse map is composed in `impulse_map.json` and wired between the signal
grid and `training_reinforce.json`: coordinates are rearranged by sympathy, and
settled regions light up as region tokens.

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

### The composed impulse map

`impulse_map.json` takes the grid's readings: one slot per original coordinate,
slot i at (i mod width, i div width) on the square lattice, with a flag for
whether it reported now. A coordinate's identity is its slot; what wrote it is
not the map's concern. Every step is a generic node, and every piece of state
lives in a `store.Vector` written back as feedback:

1. `calculus.Change` against the previous reading (`previous`): a coordinate
   that did not report now has no movement, never zero; one that reported and
   did not move moved by exactly zero.
2. `statistic.Authority` (`authority_state`) uses `data.Quality`'s definitions
   on each coordinate's own history: a reading's SNR is divergence² over noise
   variance (its square over the coordinate's earlier mean square), maturity is
   1 − 1/support, and authority is maturity × snrFraction, the mean of
   snr/(1+snr) over its readings (bounded, so no extreme reading dominates).
   Maturity and snrFraction are published beside it. The standardized
   reading keeps zero as zero; energy is standard² × authority.
3. `graph.Complete` (with `reach`) joins every coordinate that reported to
   every other coordinate, and `statistic.Concordance` reads those pairs
   (`pair_state`, read only for the pairs touched now): alignment is the
   product of directions, so a consistent inverse is as concordant as a direct
   pair. One moving while the other reported and did not is a non-response
   (A0); one moving while the other did not report at all (A Nil) is a
   non-response too, but only for a relationship already established, and the
   silent end's value is never read — absence is not taken for zero. Strength
   is |mean alignment| less its standard error plus consistency-weighted
   magnitude agreement, and a pair that just broke its orientation reads
   negative — the repulsion of the second priority.
4. `geometry.Inversion` turns strength into a target distance (sympathy under
   one cell, repulsion over), and `geometry.Relaxation` (`positions`) takes one
   stress step per pair. Its pull is |strength| × the pair's combined
   authority, so maturity and SNR power attraction, and each end moves by the
   other end's share of that authority: the weak cell travels to the strong
   one.
5. `geometry.Peak` (`partition`) drains every coordinate to its highest
   sympathetic neighbour the arrangement has pulled within one cell, up to a
   peak. A partition has held when every coordinate drained to the same peak on
   the two evaluations before. Regions are only ever read from a partition that
   held, as it stood when the evaluation began, so an evaluation's own evidence
   never changes the regions it is read against. While the arrangement moves,
   the last partition that held stands — a map in flight is never read — and
   until one has held nothing is published. Gating each pass instead would
   silence exactly the passes that matter: the ignition and the extremum are
   what move the arrangement.
6. `statistic.GroupSum` of energy per settled region and `statistic.Otsu` over
   those sums: the regions lighting up now, named by their peak's original
   coordinate, are the region token. Peak also publishes the partition's
   vocabulary: the same name under another partition may cover other ground,
   so the vocabulary scopes the token history and is part of the trie key.

### Precursor windows

What is learned is the development, not the landmark. B and C only grade.

- The entry precursor is the token trajectory from A to B. The token history
  starts at A (`window_start`), and the ENTER truth at B is keyed by the path
  A→B.
- The exit precursor is the trajectory from B to C. `cognition.TokenSequence`
  is scoped by {vocabulary, holding}, and holding is the causal inventory: it
  turns true on the first cursor after the entry filled, so a fresh history
  begins there and the EXIT truth at C is keyed by the path B→C.
- A step that brings no token is read against the development so far; before
  any token the sequence is idle and nothing is reinforced.
- The labels travel only on the fragment's truth; `TestTrainingTokensAreCausal`
  checks on the compiled graph that nothing downstream of the truth feeds a
  signal or the impulse map.

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

**Step three — retrieve the fragments** that contain those excursions, the way
hindsight's `RunIndex` did. The tape is streamed once into `store.Index`, one
list per (capture session, instrument) in capture order. Each confirmed event is
then one contiguous range of its own instrument's list: seek the last Level 3
snapshot at or before A (the book is only defined from a snapshot; without one
the range starts at A), walk to D, one record per evaluation, then take the next
event. Records before A only rebuild the book and warm the signals; A..C is the
graded window; C..D gives the exit a book to fill against. Events without a
precursor are passed over. Cost is one pass per archived record for the tape
plus one per record inside a fragment — never records × events.

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

The loop, in five steps:

1. **Ground truth is known before the fragment is trained on.** Every fragment
   is one of four types:
   - upward movement that clears friction (order book and fees);
   - upward movement that does not clear friction;
   - stagnant or choppy movement;
   - downward movement.

   Direction is the mined excursion's sign. Friction is measured, not assumed:
   `paper_exchange.json` executes ENTER at B and EXIT at C against the recorded
   Level 3 book (order by order, checksum-verified, fills walking the queue,
   taker fee included). A positive round trip clears friction. Stagnant
   fragments are resolved stretches with no event.
2. **B and C are known; A is random.** B is ignition, C is stagnation,
   exhaustion or reversal. A is drawn uniformly before B, with its seed
   recorded, so the system cannot learn a fixed offset to the event.
3. **The prediction comes from the region sequence**, prior and current: the
   path of region tokens that led here, not only the latest one.
4. **Evaluation knows the fragment type.** On an upward fragment that clears
   friction, the ENTER prediction is too early, too late or about right
   relative to B, and the EXIT prediction likewise relative to C. "About right"
   is not a tolerance someone chose: a predicted point is right when the round
   trip it produces through the same book still clears friction, early or late
   when it does not. On any other fragment type, an ENTER is wrong.
5. **The model is adjusted accordingly.**

Profitability is how a predicted entry or exit is judged, never a substitute
for the precursor truth the model learns from.

The balance is part of the simulation. The account carries across fragments:
every ENTER spends 20% of the cash not already committed, every EXIT sells the
whole position, losses compound, and an account that has shrunk below the
exchange's minimums finds its orders refused — a consequence it lives with,
never a case that is skipped or resized. A refusal the replay cannot judge (no
book, no instrument rules) is unknown, not a loss.

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
| Metric 2D coordinates | built: slot i of the grid is original coordinate (i mod width, i div width) |
| Impulse map and sympathy clustering | built: `impulse_map.json` (`TestImpulseMapManifest`) |
| Remapper and settling gate | built: `geometry.Relaxation` arranges, `geometry.Peak` gates on a partition that held |
| Region tokens | built: `statistic.Otsu` over settled region energy, into `cognition.TokenSequence` (`TestCompileTrainingPaper`) |
| Radix trie | partly built (`nomagique/cognition`) |
| Raw capture into Iceberg | explicit cursors and ordered deduplicated replay built; deployment subscriptions pending |
| Excursion mining | per-symbol mining and persisted event cursors built |
| Fragment retrieval | ordered archive traversal and A–C fragment selection implemented in JSON |
| Fragment replay | `training_replay.json` over `store.Index`: tape once, then each event walks its instrument from the book's snapshot through D (`TestProgramExecuteReplay`, `TestCompile`) |
| A/B/C fragment selection | `training_grade.json`, fed by the fragment replay |
| PnL grading against the L3 book | `paper_exchange.json` wired into `training.json`: replayed records and, until the trie decides, the fragments' own ENTER at B / EXIT at C are one ordered event stream; closed round trips go to `paper_round_trips_v1`. Proven end to end by `TestCompileTrainingPaper` (archive → mining → fills against recorded L3 → positive round trip archived). Fee is the account's measured 0.80% taker as a visible constant |
| Level 3 capture | `capture.json` verified live: 603 L3 symbols admitted in minutes (rate-limited ones retried), instrument/ticker/trade/L3 in one session in `symmtables/symm/raw_frames_v3` |
| Throughput | measured on a live archive: ~909 graph passes/s, 97% of CPU in goroutine park/wake (one Cap'n Proto server hand-off per node call). The tape still costs one pass per archived row to scan and index |
| Fragment training loop | built: the tape is indexed once; each mined event walks its own instrument, and that walk alone feeds the signals under the fragment's scope (every stateful signal node, the grid and the impulse map's last readings start fresh per fragment). `data.Gather` hands the impulse map each record's metrics on the pass they are computed, so the token joined to a cursor's truth is the state after that cursor's record. The token sequence restarts with each fragment. Holding is a retained inventory per symbol, read before each decision and changed by the fragment's own ENTER at B and EXIT at C; both reach the trie (`TestCompileTrainingPaper`) |
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
