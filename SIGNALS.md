# signals

Story phases: **ENTER → HOLD → EXIT**. Each phase has **detector**, **support**, and **opposition** (grid regions R00–R02, R06–R08, R03–R05).

| Phase | Role       | Metric                  | Source      | Region |
|-------|------------|-------------------------|-------------|--------|
| ENTER | detector   | `coiled_compression`    | pumpdump    | R00    |
| ENTER | support    | `structural_alignment`  | correlation | R01    |
| ENTER | opposition | `starved_tape`          | pumpdump    | R02    |
| HOLD  | detector   | `supercritical_cascade` | hawkes      | R06    |
| HOLD  | support    | `bid_ladder_build`      | depthflow   | R07    |
| HOLD  | opposition | `blowoff_dip`           | pumpdump    | R08    |
| EXIT  | detector   | `rejection_tick`        | pumpdump    | R03    |
| EXIT  | support    | `bid_ladder_release`    | depthflow   | R04    |
| EXIT  | opposition | `ask_ladder_release`    | depthflow   | R05    |

### Board oracles (`…/aggregate/…`)

Precursors at **market** scale: they add confidence to enter, hold, or exit (R09–R11), or veto when the board cannot carry size (R12–R14). Each metric is pinned once.

**Confidence (adds to the matching story band in the gate and labels paths when brightest)**

| Region | Action | Metric              | Source    | Precursor read                                                                 |
|--------|--------|---------------------|-----------|--------------------------------------------------------------------------------|
| R09    | Enter  | `board_coiled`      | pumpdump  | Median symbol coiling — quiet board before ignition.                           |
| R10    | Hold   | `board_cascade`     | hawkes    | Median Hawkes ρ — tape still self-exciting board-wide.                         |
| R11    | Exit   | `board_bid_release` | depthflow | Median bid-ladder release — support fading across names before the crowd runs. |

**Risk (veto spelling when risk beats positive net force; R14 can block cognitive exit into a thin book)**

| Region | Role            | Metric                 | Source      |
|--------|-----------------|------------------------|-------------|
| R12    | Friction hurdle | `market_friction`      | liquidity   |
| R13    | Contagion       | `structure_uncoupling` | correlation |
| R14    | Capacity / exit | `capacity_stress`      | liquidity   |

`signal/toxicity` and `signal/sentiment` exist in the tree but are **not** on the live booter until a metric is pinned on the grid.

### Book ladder (depthflow)

Ladder metrics read **displayed depth mutations** between L3 snapshots (added vs removed notional inside the visible bid or ask ladder).

- **`bid_ladder_build`** — net bid size **stacking in** (`max(0, added − removed)`): book **building under** the price (hold support).
- **`bid_ladder_release`** — net bid size **peeling off** (`max(0, removed − added)`): ladder **stabilizing / coming down** without waiting for touch toxicity (exit support — sell while bids are still being displayed to hit).
- **`ask_ladder_release`** — net ask size **peeling off** (`max(0, removed − added)` on the offer ladder): visible supply **thinning before the mark-up** — exit opposition (precursor that the move may still have room; don’t fire exit on a false top).

### `coiled_compression`

On each closed volume bar: `volume_ratio × quiet_price`, where `volume_ratio` is volume rate over its causal baseline and `quiet_price = 1 − min(1, positive_midpoint_return / ε)` with ε = 5×10⁻⁴.

### `structural_alignment`

One board-wide reading per tick (`correlation/aggregate/…`): median absolute Hayashi correlation over eligible symbol pairs (capped sample). High = many reprices are moving on the same current; no focal symbol.

### `starved_tape`

On each closed volume bar: participation collapsed vs this symbol’s recent norm — flat lines / fake tries. High ≈ do not enter.

### `supercritical_cascade`

Hawkes branching spectral radius ρ; ρ ≥ 1 reads as supercritical feedback.

### `blowoff_dip`

Strong down bar after a remembered strong up bar — blow-off pullback (hold opposition).

### `rejection_tick`

`max(0, −log(mid_close / mid_open))` on the bar — exit detector.

### `board_coiled`

Median of each symbol’s latest `coiled_compression` (`pumpdump/aggregate/…`): board-wide quiet coiling before moves (R09 enter confidence).

### `board_cascade`

Median of each symbol’s latest Hawkes spectral radius (`hawkes/aggregate/…`): board still in feedback (R10 hold confidence).

### `board_bid_release`

Median of each symbol’s latest `bid_ladder_release` (`depthflow/aggregate/…`): displayed bids peeling across the market (R11 exit confidence).

### `market_friction` / `capacity_stress`

Across symbols with a touch: median relative spread (`market_friction`, R12) and worst relative spread (`capacity_stress`, R14).

### `structure_uncoupling`

`1 − structural_alignment` on the same tick (`correlation/aggregate/…`): peers drifting apart (R13 risk).

### `spoof_pressure` / `floor_evaporation` (off booter)

Late touch retreat / withdrawal; not wired to the grid in this slate.
