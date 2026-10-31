# signals

Story phases: **ENTER → HOLD → EXIT**. Grid regions **R00–R14** each carry **exactly three** pinned metrics (45 total). Region brightness is the arithmetic mean of three independent channels.

### Enter R00–R02

| Region         | Channel A                          | Channel B                     | Channel C (unique)             |
|----------------|------------------------------------|-------------------------------|--------------------------------|
| R00 Spark      | `cvd/aggressor_backing`            | `pumpdump/coiled_compression` | `cvd/initiative_lock`          |
| R01 Support    | `correlation/structural_alignment` | `leadlag/led_peer_share`      | `leadlag/cohort_lift`          |
| R02 Opposition | `pumpdump/starved_tape`            | `depthflow/ask_absorption`    | `morphology/bid_notional_lean` |

### Exit R03–R05

| Region         | Channel A                      | Channel B                   | Channel C (unique)           |
|----------------|--------------------------------|-----------------------------|------------------------------|
| R03 Detector   | `pumpdump/rejection_tick`      | `cvd/cvd_divergence`        | `cvd/flow_sign_flip`         |
| R04 Support    | `depthflow/bid_ladder_release` | `toxicity/retreat_fraction` | `depthflow/touch_bid_decay`  |
| R05 Opposition | `depthflow/ask_ladder_release` | `morphology/ask_vacuum`     | `depthflow/ask_ladder_build` |

### Hold R06–R08

| Region         | Channel A                      | Channel B                      | Channel C (unique)            |
|----------------|--------------------------------|--------------------------------|-------------------------------|
| R06 Ignition   | `hawkes/supercritical_cascade` | `pumpdump/clock_acceleration`  | `hawkes/branching_asymmetry`  |
| R07 Support    | `depthflow/bid_ladder_build`   | `morphology/book_shape_stable` | `depthflow/bid_touch_refresh` |
| R08 Opposition | `pumpdump/blowoff_dip`         | `liquidity/spread_expansion`   | `liquidity/move_per_spread`   |

### Board oracles R09–R11

| Region               | Channel A                     | Channel B                              | Channel C (unique)                 |
|----------------------|-------------------------------|----------------------------------------|------------------------------------|
| R09 Enter confidence | `pumpdump/board_coiled`       | `correlation/board_dispersion_squeeze` | `pumpdump/board_starved`           |
| R10 Hold confidence  | `hawkes/board_cascade`        | `sentiment/cohort_still_aligned`       | `hawkes/board_branching_asymmetry` |
| R11 Exit confidence  | `depthflow/board_bid_release` | `toxicity/board_withdrawal_rate`       | `depthflow/board_ask_release`      |

### Risk R12–R14

| Region        | Channel A                          | Channel B                       | Channel C (unique)          |
|---------------|------------------------------------|---------------------------------|-----------------------------|
| R12 Friction  | `liquidity/market_friction`        | `liquidity/friction_drag_ratio` | `liquidity/touch_friction`  |
| R13 Contagion | `correlation/structure_uncoupling` | `leadlag/anchor_breakdown`      | `leadlag/lag_dispersion`    |
| R14 Capacity  | `liquidity/capacity_stress`        | `liquidity/liquidation_impact`  | `liquidity/spread_tail_gap` |

Pin map: `nomagique/store/grid_pins.go`. Aggregate layers use `…/aggregate/…` where noted in the frontend impulse map.

### Third-channel semantics (one per region)

- **`initiative_lock`** — directional volume-bar flow while midpoint repricing stays inside the quiet band (spark before breakout).
- **`cohort_lift`** — share of peers whose recent return sign matches the focal symbol (support, not temporal lead).
- **`bid_notional_lean`** — ask-heavy displayed notional imbalance (opposition on the book, not offer thinning alone).
- **`flow_sign_flip`** — bar-to-bar reversal of signed flow (exit detector distinct from price–CVD divergence).
- **`touch_bid_decay`** — best-bid size eroding at an unchanged touch (support failing at the inside).
- **`ask_ladder_build`** — net ask size stacking in on the offer ladder (opposition building, not release/vacuum).
- **`branching_asymmetry`** — Hawkes buy/sell excitation asymmetry at the symbol (ignition shape, not spectral radius alone).
- **`bid_touch_refresh`** — best-bid size replenishing at an unchanged touch (hold support refresh).
- **`move_per_spread`** — last midpoint return per unit relative spread (extension cost, not spread widening alone).
- **`board_starved`** — median thin-tape posture across symbols (enter oracle complement to board coiled).
- **`board_branching_asymmetry`** — median Hawkes asymmetry across symbols (hold oracle complement to cascade radius).
- **`board_ask_release`** — median ask-ladder build across symbols (exit oracle complement to bid release).
- **`touch_friction`** — symbol touch spread vs board median spread (local friction, not board median alone).
- **`lag_dispersion`** — cross-symbol dispersion of temporal-leadership scores (contagion in lead structure).
- **`spread_tail_gap`** — worst-minus-median board spread (capacity tail stress, not median friction alone).

### Book ladder (depthflow)

- **`bid_ladder_build`** — net bid size stacking in (`max(0, added − removed)`).
- **`bid_ladder_release`** — net bid size peeling off (`max(0, removed − added)`).
- **`ask_ladder_release`** — net ask size peeling off on the offer ladder.
- **`ask_absorption`** — buy volume at a stuck best ask relative to displayed size (iceberg-style absorption).

### Toxicity

- **`retreat_fraction`** — unfilled bid fraction when the touch steps down (per symbol).
- **`board_withdrawal_rate`** — median touch withdrawal rate across symbols, computed inside `toxicity` (`toxicity/aggregate/…`; not published per symbol).
