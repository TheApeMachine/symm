# Category Graph Design

Date: 2026-07-25

## Goal

Maximize wallet / minimize time by restoring **composed categories** as the
compact cognitive vocabulary, and a **single resident category graph** that
strategy can read — without rebuilding a graph every tick or exploding DMT
token space with raw metric bags.

## Decisions (locked)

1. **Nodes are categories** (per symbol). Not measurements-as-nodes; not a
   per-tick gonum rebuild.
2. **Categories are composed in logic** from measurements via
   `types.CategoryAffinity` (DECISION.md taxonomy). Signals do not emit
   category winners.
3. **DMT trains and predicts on category / category-transition tokens**, so
   sequence size stays bounded by active categories, not measurement count.
4. **One graph is built once** (on Analyzer / stack lifetime) and **updated**
   each cut: edge weights strengthen or decay from observed co-activation and
   typed relationships.
5. **Strategy consumes** live category mass, predicted transitions, and typed
   edge structure (e.g. trap categories contradicting opportunity categories).

## Category state (per symbol × category)

Retained on each update (thesis.md):

- Supporting / opposing / missing required evidence (metric keys)
- Maturity, uncertainty, freshness
- Redundancy adjustment (shared evidence across metrics)
- Strength / confidence from composed evidence (not from signal Category rows)
- Optional historical calibration mass from DMT cohort when available

Composition rule:

```
for each valid measurement on symbol:
  affinity = CategoryAffinity[metric]
  for cat in affinity.Supports: accumulate support mass
  for cat in affinity.Opposes: accumulate oppose mass
category.strength ∝ support / (support + oppose + missing_penalty)
```

Masses use measurement normalized magnitudes and maturity already on Thesis —
no static windows.

## Relationship vocabulary (typed edges)

Edges are **category → category** on one symbol. Types from thesis.md. Each
type has an evidence derivation — edges are not minted from trap/opportunity
labels or from DMT top-winner flips.

| Type | Derivation |
|------|------------|
| Contradicts | `CategoryAffinity`: live metrics that Supports A and Opposes B |
| RedundantWith | Jaccard overlap of composed Supporting metric keys |
| Conditions | A's Supporting fills entries in B's Missing required evidence |
| Leads / Lags | Supporting-measurement envelopes (`ObservedFrom`/`At`/`Horizon`) ordered in time; also prior-cut activation before a new peer activates |
| StaleRelativeTo | A's latest evidence older than B's by more than A's own Horizon |
| IncomparableWith | Evidence intervals disjoint and neither Horizon covers the gap |
| Supports | Complementary co-activation: disjoint evidence, no affinity contradiction, no independence statistic |
| IndependentOf | Pair-memory joint mass below product baseline, and/or live `decoupled` / `noise_score` mass |

Each edge stores: type, weight ∈ (0,∞) monotonic strengthen on evidence,
evidence refs (metric keys), timestamps.

## Resident graph lifecycle

- Owned by `logic` (Analyzer-composed `category.Graph`), **not** cleared in
  `Thesis.ResetTick`.
- Thesis holds a **pointer / view** into the resident graph for the cut
  (or publishes a compact snapshot for UI/strategy), never owns the mutating
  structure as a per-tick rebuild.
- `thesis.Graphs` sync.Map clear on ResetTick is retired for this purpose.

Update each cut after category composition / after DMT consumes transition tokens:

1. Upsert active category nodes for symbols with evidence.
2. Derive Contradicts / RedundantWith / Conditions / Supports from affinity +
   composed evidence sets.
3. Derive Leads/Lags / StaleRelativeTo / IncomparableWith from measurement
   clocks and prior-cut activation order.
4. Record prior top only for DMT transition tokens — never as a Leads source.
5. Decay idle edges by `mean/(mean+age)` using the symbol's observed inter-cut
   mean cadence.

## DMT / cognition

Replace measurement-bag `sensorySequence` with:

```
symbol-<sym> _ cat-<type>-<polarity> _ ... _ transition-<from>-<to>
```

sorted stable bag under the symbol hop. Polarity from composed strength sign /
presence. Transitions append when prior winner ≠ current top category.

Classify / PredictNext still drive Cognition; `composeCategories` fills
`thesis.Categories` from **composed** category states (primary), with DMT
confidence/surprisal stamped onto the strongest row as calibration — not as
the sole category source.

## Strategy use

- Trap veto / opportunity already uses measurement masses; extend with
  category graph: if Contradicts-weighted trap categories dominate opportunity
  categories for a symbol, refuse enter (alongside existing TrapShare).
- Rotate / hold can read Leads edges toward exhaustion / vacuum as exit
  pressure when calibrated.
- No new magic thresholds: compare relative edge-weighted masses.

## Out of scope

- Reviving the removed stance/composer packages as-was
- Building a new gonum graph every tick
- Signal-emitted category rows
- Partial position reduces

## Success criteria

| Proof | Outcome |
|-------|---------|
| Composition unit | Multi-metric support/oppose fills Supporting/Opposing/Missing |
| Resident update | Same graph pointer across cuts; edge weight increases on repeat |
| DMT sequence | Token count ≪ measurement count on fat cuts |
| Market-sim | FastPump still enterable; trap tapes still refuse; categories non-empty on thesis |
| Strategy | Graph-readable fields used in at least trap/exit path without static cutoffs |
