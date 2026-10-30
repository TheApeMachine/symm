# signals

Story phases: **ENTER → HOLD → EXIT**. Grid regions **R00–R14** each carry **exactly two** pinned metrics (30 total) so top-2 brightness pooling is an unbiased mean of two channels, not order-statistics skew from unequal sample sizes.

### Enter R00–R02

| Region         | Channel A                          | Channel B                     |
|----------------|------------------------------------|-------------------------------|
| R00 Spark      | `cvd/aggressor_backing`            | `pumpdump/coiled_compression` |
| R01 Support    | `correlation/structural_alignment` | `leadlag/led_peer_share`      |
| R02 Opposition | `pumpdump/starved_tape`            | `depthflow/ask_absorption`    |

### Exit R03–R05

| Region         | Channel A                      | Channel B                   |
|----------------|--------------------------------|-----------------------------|
| R03 Detector   | `pumpdump/rejection_tick`      | `cvd/cvd_divergence`        |
| R04 Support    | `depthflow/bid_ladder_release` | `toxicity/retreat_fraction` |
| R05 Opposition | `depthflow/ask_ladder_release` | `morphology/ask_vacuum`     |

### Hold R06–R08

| Region         | Channel A                      | Channel B                      |
|----------------|--------------------------------|--------------------------------|
| R06 Ignition   | `hawkes/supercritical_cascade` | `pumpdump/clock_acceleration`  |
| R07 Support    | `depthflow/bid_ladder_build`   | `morphology/book_shape_stable` |
| R08 Opposition | `pumpdump/blowoff_dip`         | `liquidity/spread_expansion`   |

### Board oracles R09–R11

| Region               | Channel A                     | Channel B                              |
|----------------------|-------------------------------|----------------------------------------|
| R09 Enter confidence | `pumpdump/board_coiled`       | `correlation/board_dispersion_squeeze` |
| R10 Hold confidence  | `hawkes/board_cascade`        | `sentiment/cohort_still_aligned`       |
| R11 Exit confidence  | `depthflow/board_bid_release` | `toxicity/board_withdrawal_rate`       |

### Risk R12–R14

| Region        | Channel A                          | Channel B                       |
|---------------|------------------------------------|---------------------------------|
| R12 Friction  | `liquidity/market_friction`        | `liquidity/friction_drag_ratio` |
| R13 Contagion | `correlation/structure_uncoupling` | `leadlag/anchor_breakdown`      |
| R14 Capacity  | `liquidity/capacity_stress`        | `liquidity/liquidation_impact`  |

Pin map: `nomagique/store/grid_pins.go`. Aggregate layers use `…/aggregate/…` where noted in the frontend impulse map.

### Book ladder (depthflow)

- **`bid_ladder_build`** — net bid size stacking in (`max(0, added − removed)`).
- **`bid_ladder_release`** — net bid size peeling off (`max(0, removed − added)`).
- **`ask_ladder_release`** — net ask size peeling off on the offer ladder.
- **`ask_absorption`** — buy volume at a stuck best ask relative to displayed size (iceberg-style absorption).

### Toxicity

- **`retreat_fraction`** — unfilled bid fraction when the touch steps down (per symbol).
- **`board_withdrawal_rate`** — median touch withdrawal rate across symbols, computed inside `toxicity` (`toxicity/aggregate/…`; not published per symbol).
