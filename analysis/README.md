# Performance & attribution analysis

This directory tracks how the system is actually performing in live/replay
trading — realized PnL, win rates, prediction calibration, and (going forward)
per-signal attribution.

## Files

- **`attribution.py`** — reads `runs/audit-*.jsonl`, reconstructs every
  round-trip trade, and emits a metrics report. Run it after any session:
  ```sh
  python3 analysis/attribution.py --report analysis/PERFORMANCE.md
  ```
- **`PERFORMANCE.md`** — the latest generated report (regenerated each run;
  do not hand-edit, it gets clobbered).

## The attribution gap (why "which signal earns the most edge" was unanswerable)

The honest answer to *"which of Hawkes, Causal, Fluid, DepthFlow, CVD
contributes the most realized edge"* on the runs to date is: **it cannot be
computed from the logged data.** Three reasons:

1. **Fusion discards source identity.** `engine.FuseMeasurements` collapses all
   contributing sources into one noisy-OR joint confidence. Once a trade fills,
   the per-source confidences are gone.
2. **`trade_entry_fill` recorded no source.** Despite `recordStats` reading a
   `source` field, the entry-fill emitter never set one, so `sourceFills`
   counted everything under the empty key. Exits log only a `reason`.
3. **`perspective_ready` logs `perspective_type` (4 buckets), not the signal.**
   The finest grain ever persisted was microstructure / flow / cross_asset /
   sentiment — not the individual signal.

### Fix (2026-05-29)

`trade_entry_fill` now carries:

- `contributions` — `{source: max_confidence}` for every signal that fed the
  fusion behind that entry,
- `dominant_source` — the highest-confidence signal,
- `perspective_type` — the bucket.

`attribution.py` does a confidence-weighted split of each trade's net PnL across
its `contributions`, so once new runs accumulate, the "Per-signal attribution"
table populates automatically. Pre-fix runs degrade gracefully and say so.

## What the current 8 runs (28 round-trips) actually show

These findings hold regardless of the attribution gap:

- **Net PnL is negative: −0.51 EUR (−0.33% of notional), 14% net win rate.**
  Gross (pre-fee) is +0.29 EUR at a 43% win rate — **fees (0.81 EUR) exceed the
  entire gross edge.** The system is fee-dominated, not signal-dominated.
- **Every one of the 28 exits is `runway_expired`** (~30s holds). No
  take-profit and no stop ever triggered. The strategy is effectively "hold for
  the runway, then market-out," which guarantees paying the full ~0.52%
  round-trip taker fee against an average realized move of just 0.09%.
- **Predictions are ~12x optimistic**: avg predicted return 1.08% vs avg
  realized 0.09%. The edge gate (`EntryEdgeMultiple`) is being cleared on
  forecasts that don't materialize over the hold.
- **Measurement volume is wildly skewed**: fluid 75%, then sentiment 8%,
  depthflow 6%, causal 6%, liquidity 4%, hawkes 1%, pumpdump 0.2%. **CVD emits
  nothing** in these runs — its gates (≥40 trades, ≥60% net fraction, flat-price
  band) almost never open, so it currently contributes zero to live decisions.

### Implications worth investigating

- The dominant lever right now is **fee/hold structure**, not signal selection:
  maker entries, longer runways so predicted moves can realize, or a
  take-profit that exits before fees swamp the edge.
- Recalibrate forecasts — an 11.8x optimism ratio means the edge gate is
  letting through trades that can't pay for themselves.
- Decide whether CVD's gates are too strict to ever contribute, or whether it's
  miswired upstream.

## Metrics tracked

Portfolio net/gross PnL, fees, net & gross win rate, predicted-vs-realized
return and optimism ratio; breakdowns by perspective type, symbol, and exit
reason; full exit-reason and entry-skip distributions; measurement volume by
source; and per-signal attributed net PnL (once contribution-tagged runs exist).
