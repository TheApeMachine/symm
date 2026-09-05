# The Young Agent’s Guide to Becoming a Trader

---

## Table of Contents

The sidebar on the left shows a table of contents built from your headings. Click any heading to jump there. Try it now with the sections below.

## Impulse Map

A `2D` coordinate space where `Signals` and `Logic Solvers` write their `Observations`. This will naturally create a grid where cells “light up” when reacting with the market tape.

The next step is to re-organize the `Impulse Map` to cluster sympathetically. Where sympathy is a ladder of priorities. These priorities are additive when they happen at the same time, though hold true stand-alone too.

- **First:** `Values` that move together attract
- **Second:** `Values` with closest relative magnitude during movement attract\
  \*Sign is irrelevant if the match is consistent, so `A+` and `B-` can attract only if `A-` and `B+` holds\*\
  \*Repel if inconsistent, so either `A+` and `B-` then `A-` and `B-`, or `A+` and `B-` then `A0-or-Nil` and `B+`
- **Third**: `Maturity` and `SNR` powers attraction, if `A` is stronger than `B`, `B` moves more to `A` than `A` to `B`. This creates natural hot spots, separated by a colder gradient around it.

### Regions

The third priority naturally generates regions, the borders being defined by where the weakest cells meet.

| 1.0 (A) | 0.5 (A) | 0.25 (A) | 0.25 (B) |
| --- | --- | --- | --- |
| 0.5 (A) | 0.5 (A) | 0.25 (A) | 0.25 (B) |
| 0.25 (A) | 0.25 (A) | 0.25 (B) | 0.5 (B) |
| 0.25 (B) | 0.25 (B) | 0.5 (B) | 1.0 (B) |

The `Impulse` the `Agent` reacts to are the regions that light up the strongest, using something like SNR-weighted Otsu’s method, the topological "water table" split, or mean plus standard deviation

## Agent

The agent reacts to the `Impulse Map` by receiving the `Region Sequence` ordered from strongest to weakest.

Combined with `Priors` stored in its `Model` it will learn: `Strategy(Action([A, B, C, …]), Action([C, B, D, …])…)`

### Model

Segmented by a `Keyed Universe` (in this case market `Symbol Pair`) each horizontal lane spans the life-cycle of what is being `Acted` upon (in this case `Positions`). A secondary lane runs in parallel, which is the previous matching `Position`. Given that prior lane has fully lived out its lifecycle, it will also have been assigned its reward. This is the mechanism that allows the `Agent` to decide to math the prior action, or divert.

<table style="min-width: 144px;">
<colgroup><col style="min-width: 48px;"><col style="min-width: 48px;"><col style="min-width: 48px;"></colgroup><tbody><tr><th colspan="1" rowspan="1"><p>BTC/USD</p></th><th colspan="1" rowspan="1"><p>Weight{Impulse: [A, B, C, …], Action: Enter{Qty: 100}, Reward: 0}<br>—-<br>Weight{Impulse: [A, B, C, …], Action: Enter{Qty: 100}, Reward: 0.1}</p></th><th colspan="1" rowspan="1"><p>Weight{Impulse: [C, B, D, …], Action: Wait{}, Reward: 0}<br>—-<br>Weight{Impulse: [C, B, D, …], Action: Wait{}, Reward: 0.5}</p></th></tr><tr><td colspan="1" rowspan="1"><p>ETH/USD</p></td><th colspan="1" rowspan="1"><p>Weight{Impulse: [A, B, C, …], Action: Enter{Qty: 10}, Reward: 0}<br>—-<br>Weight{Impulse: [A, B, C, …], Action: Enter{Qty: 10}, Reward: 0.25}</p></th><th colspan="1" rowspan="1"><p>Weight{Impulse: [A, B, C, …], Action: Scale{Qty: -5}, Reward: 0}<br>—-<br>Weight{Impulse: [A, B, C, …], Action: Enter{Qty: 100}, Reward: 0.7}</p></th></tr><tr><td colspan="1" rowspan="1"><p>DOGE/USD</p></td><th colspan="1" rowspan="1"><p>Weight{Impulse: [A, B, C, …], Action: Wait{}, Reward: 0}<br>—-<br>Weight{Impulse: [A, B, C, …], Action: Wait{}, Reward: 0.9}</p></th><th colspan="1" rowspan="1"><p>Weight{Impulse: [A, B, C, …], Action: Wait{}, Reward: 0}<br>—-<br>Weight{Impulse: [A, B, C, …], Action: Wait{}, Reward: 0.3}</p></th></tr></tbody>
</table>