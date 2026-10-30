# signals

Story phases: **ENTER → HOLD → EXIT**. Each phase has **detector**, **support**, and **opposition** (grid regions R00–R02, R06–R08, R03–R05).

| Phase | Role | Metric | Source | Region |
|-------|------|--------|--------|--------|
| ENTER | detector | `coiled_compression` | pumpdump | R00 |
| ENTER | support | `structural_alignment` | correlation | R01 |
| ENTER | opposition | `starved_tape` | pumpdump | R02 |
| HOLD | detector | `supercritical_cascade` | hawkes | R06 |
| HOLD | support | `bid_ladder_build` | depthflow | R07 |
| HOLD | opposition | `blowoff_dip` | pumpdump | R08 |
| EXIT | detector | `rejection_tick` | pumpdump | R03 |
| EXIT | support | `bid_ladder_release` | depthflow | R04 |
| EXIT | opposition | `bid_ladder_build` | depthflow | R05 |

### Oracles (board / regime, mostly `…/aggregate/…`)

| Region | Role | Metric | Source |
|--------|------|--------|--------|
| R09 | Dead / starved board | `board_starvation` | pumpdump |
| R10 | Macro recurrence | `structural_alignment` | correlation |
| R11 | Sentiment recurrence | `cohort_still_aligned` | sentiment |
| R12 | Friction hurdle | `market_friction` | liquidity |
| R13 | Macro contagion | `structure_uncoupling` | correlation |
| R14 | Capacity / thin exit | `capacity_stress` | liquidity |

Per-symbol touch toxicity (`spoof_pressure`, `floor_evaporation`) remains in `signal/toxicity` but is off the live booter until we pin or aggregate it.

### Book ladder (depthflow)

Both read **displayed bid depth mutations** between L3 snapshots (added vs removed notional inside the visible ladder).

- **`bid_ladder_build`** — net bid size **stacking in** (`max(0, added − removed)`): book **building under** the price (hold support; also exit opposition when reload fights your sell).
- **`bid_ladder_release`** — net bid size **peeling off** (`max(0, removed − added)`): ladder **stabilizing / coming down** without waiting for touch toxicity (exit support — sell while bids are still being displayed to hit).

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

### `board_starvation`

Median of each symbol’s latest `starved_tape` (`pumpdump/aggregate/…`): how dead the board feels, not one name.

### `market_friction` / `capacity_stress`

Across symbols with a touch: median relative spread (`market_friction`, R12) and worst relative spread (`capacity_stress`, R14).

### `structure_uncoupling`

`1 − structural_alignment` on the same tick: peers drifting apart (R13 contagion).

### `cohort_still_aligned`

Median across symbols of same-direction peer counts (`sentiment/aggregate/…`): how much the board still moves together (R11 hold/regime recurrence).

### `spoof_pressure` / `floor_evaporation` (dormant on booter)

Late touch retreat / withdrawal; not wired to the grid in this slate.
