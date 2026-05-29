# SYMM — Shake Your Money Maker

A Kraken spot microstructure engine. Live market data flows through twelve independent signal systems, each emitting calibrated observations. A trader fuses those observations into perspectives, forecasts forward returns, and manages a wallet. Forecasts settle against realized prices, and the error feeds back into every signal that contributed — tightening their calibration without touching their logic.

The default wallet is paper (€200). Point it at Kraken WebSocket v2 for live data; add API keys for real orders; or replay a JSONL fixture for offline analysis.

---

## Contents

- [Architecture](#architecture)
- [The data pipeline](#the-data-pipeline)
- [Everything is a `System`](#everything-is-a-system)
- [Boot sequence](#boot-sequence)
- [Core types](#core-types)
- [Signal systems](#signal-systems)
- [Trader mechanics](#trader-mechanics)
- [Prediction and feedback](#prediction-and-feedback)
- [Risk and sizing](#risk-and-sizing)
- [Calibration](#calibration)
- [UI and telemetry](#ui-and-telemetry)
- [Numeric layer](#numeric-layer)
- [Build and run](#build-and-run)
- [Configuration reference](#configuration-reference)
- [Repository map](#repository-map)

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Kraken WebSocket v2                                             │
│    tick · trade · book · symbols · ohlc                          │
└──────────────┬───────────────────────────────────────────────────┘
               │  qpool broadcast
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  12 Signal Systems                                               │
│  pumpdump · depthflow · hawkes · leadlag · liquidity             │
│  sentiment · correlation · fluid · causal · cvd · toxicity       │
│  exhaust (exit-only)                                             │
└──────────────┬───────────────────────────────────────────────────┘
               │  measurements broadcast
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  Trader (Crypto)                                                 │
│  bucket measurements by (symbol, perspective-type)               │
│  fuse → predict → entry gates → broker → wallet                  │
└──────────────┬──────────────────────┬────────────────────────────┘
               │  predictions         │  feedback broadcast
               ▼                      ▼
┌─────────────────────┐   ┌────────────────────────────────────────┐
│  price.Prediction   │   │  Every signal updates its calibrator   │
│  settle at DueAt    │──►│  for each symbol/regime bucket         │
│  emit feedback      │   └────────────────────────────────────────┘
└─────────────────────┘
               │  ui broadcast
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  ui.Hub  →  ws://127.0.0.1:8765/ws  →  React dashboard           │
└──────────────────────────────────────────────────────────────────┘
```

SYMM is not a model. It is a **fleet of observers** — each signal is a standalone system with its own statistical machinery — plus a **trader** that fuses their outputs and a **prediction layer** that learns whether those fusions were right.

---

## The data pipeline

This is the thing to understand. Everything else is support for this loop.

```
Kraken WS ──► Signals ──► measurements ──► Crypto (trader)
                                │                │
                                │                ├──► Perspective buckets
                                │                ├──► Prediction.Record (always)
                                │                └──► optional paper/live entry
                                │
                                ▼
                     (Last, Bid, Ask on each Measurement)

Prediction ◄── tick (settlement prices only)
     │
     └──► feedback ──► Signals.Feedback (calibrator scale)
```

**Three key properties of this design:**

1. **Predictions are always recorded**, not only on entry. Cold-start buckets accumulate evidence without risking capital; they start trading once they have ≥30 settled samples and statistically positive mean realized return.

2. **Feedback flows top-down.** `price.Prediction` is the single settlement authority. Each signal gets the realized return error for every settled forecast it contributed to, regardless of whether a trade was taken. This keeps calibration honest.

3. **Signals never call each other.** They publish to named broadcast groups and subscribe from them. The only coupling is through the `measurements` and `feedback` channels.

---

## Everything is a `System`

Every runnable unit implements the same interface:

```go
type System interface {
    Start() error
    State() State
    Tick() error
    Close() error
}
```

`Tick` is the long-running event loop. It spawns one goroutine per subscribed broadcast channel so that `tick`, `book`, `trade`, `feedback`, and control channels drain independently — no shared select, no head-of-line blocking.

Registration lives in `cmd/root.go`. Systems do not call each other; they publish and subscribe through named broadcast groups on a shared `qpool.Q`.

---

## Boot sequence

```
cmd.Execute()
  └─ rootCmd.Run
       ├─ create qpool (1 producer, NumCPU×4 workers)
       ├─ instantiate all systems (ordered)
       └─ Booter.Boot()
            ├─ start ui.Hub on config.UIAddr (:8765)
            ├─ for each System: Start() → ScheduleFast(Tick)
            └─ wait; any Tick error cancels context → all Close()
```

**System registration order:**

| #  | System                      | Package         |
|----|-----------------------------|-----------------|
| 1  | Public client               | `kraken/client` |
| 2  | PumpDump                    | `pumpdump`      |
| 3  | Correlation                 | `correlation`   |
| 4  | DepthFlow                   | `depthflow`     |
| 5  | Hawkes                      | `hawkes`        |
| 6  | LeadLag                     | `leadlag`       |
| 7  | Liquidity                   | `liquidity`     |
| 8  | Sentiment                   | `sentiment`     |
| 9  | Fluid                       | `fluid`         |
| 10 | Causal                      | `causal`        |
| 11 | CVD                         | `cvd`           |
| 12 | Toxicity                    | `toxicity`      |
| 13 | Exhaust                     | `exhaust`       |
| 14 | Prediction                  | `price`         |
| 15 | Crypto (trader)             | `trader`        |
| 16 | Private client *(optional)* | `kraken/client` |
| 17 | L3 client *(optional)*      | `kraken/client` |

Private and L3 clients start only when API keys are configured. L3 powers the toxicity detector.

---

## Core types

### 📐 Measurement

One signal's reading on one moment in the market.

```go
type Measurement struct {
    Type       MeasurementType  // Pump, Dump, Momentum, Flow, Causal, …
    Source     string           // "pumpdump", "depthflow", "hawkes", …
    Regime     string           // "pump_fast", "choppy", "dead", …
    Reason     string           // "volume_spike", "imbalance_flip", …
    Pairs      []asset.Pair     // symbol + quote currency
    Confidence float64          // (0, 1): match strength against current criteria
    Last       float64          // last trade price at emit time
    Bid, Ask   float64          // best bid/ask at emit time
    Timeframe  Timeframe        // nanosecond start/end
}
```

> [!IMPORTANT]
> `Confidence` is **not** a historical win rate. It is how completely the current observation matches this signal's criteria right now. History enters only through calibration and feedback — not through raw confidence values.

`AnchorPrice()` returns `Last` if populated, otherwise the bid/ask midpoint. The trader uses this anchor — not its own tick feed — to price entries and predictions.

### 🗂️ Perspective

A fusion bucket: all measurements on the same `(symbol, perspective-type)` pair, accumulated in one batch.

```go
type Perspective struct {
    Type         PerspectiveType    // Microstructure, Flow, CrossAsset, Sentiment
    Measurements []Measurement
    Regime       MarketRegime       // Unknown, Dead, Choppy, Trending, Bullish, Bearish
}
```

**Perspective-type mapping:**

| Measurement types     | Bucket                      | Fusion family  |
|-----------------------|-----------------------------|----------------|
| `Flow`, `DepthFlow`   | `PerspectiveFlow`           | cross_section  |
| `Basis`, `LeadLag`    | `PerspectiveCrossAsset`     | independent    |
| `Sentiment`, `Causal` | `PerspectiveSentiment`      | independent    |
| Everything else       | `PerspectiveMicrostructure` | microstructure |

**Fusion logic** (`FuseMeasurements`): independent sources combine via noisy-OR. Sources in the same family (e.g., two microstructure signals) are down-weighted by 1/√k to avoid compounding correlated evidence. Final: `1 - ∏(1 - clamp(conf))^weight`.

### 🔮 Prediction

A mature perspective, ready for trading decisions.

```go
type Prediction struct {
    Type           PredictionType
    Perspective    Perspective
    Confidence     float64
    ExpectedReturn float64        // calibrated forecast (0 until bucket has ≥30 samples)
    Direction      int            // -1 short, +1 long
    Runway         time.Duration  // forecast validity window
    DueAt          time.Time      // settlement time
    PredictedAt    time.Time
    Err            float64        // populated after settlement
}
```

### 📬 PredictionFeedback

The realized outcome, emitted at `DueAt` by `price.Prediction`.

```go
type PredictionFeedback struct {
    Source, Sources  string, []string  // which signals contributed
    Symbol, Regime   string
    Confidence       float64
    PredictedReturn  float64
    ActualReturn     float64
    Error            float64           // predicted - actual
    Runway           time.Duration
    PredictedAt, DueAt, SettledAt time.Time
    Unanchored       bool              // true → skip calibration update
}
```

---

## Signal systems

Each signal follows the same contract:
- Subscribe to `symbols`, `tick`/`book`/`trade` (as needed), `feedback`
- Maintain per-symbol state under a mutex
- Emit `Measurement` values to the `measurements` broadcast
- Ingest `PredictionFeedback` to update its per-symbol `PredictionCalibrator`

---

### 💥 PumpDump

Detects rapid, directional volume surges.

Three parallel detectors operate independently:

| Detector        | Window | Trigger                                                            |
|-----------------|--------|--------------------------------------------------------------------|
| `fast_pump`     | 10 s   | 10s volume ÷ fast baseline ≥ `FastPumpVolumeRatio` (×15)           |
| `actual_pump`   | 5 m    | 5m volume ÷ medium baseline above threshold                        |
| `slow_breakout` | 1 h    | 1h volume ÷ 14-day median hourly volume ≥ `SlowRVOLThreshold` (×5) |

OHLC warm-up seeds the slow baseline via REST before the WebSocket stream is live. The peak tracker records per-symbol highs for the anti-chase guard in the trader.

**Anti-chase guard**: entry into a `pump_fast` regime requires a retrace from the tracked peak of at least `PumpPullbackMin` (3%) and at most `PumpPullbackMax` (20%). Below 3%: still chasing. Above 20%: the leg is over.

---

### 📚 DepthFlow

Distance-weighted order-book imbalance with anti-spoof filtering.

- Aggregates bid/ask volumes across `BookDepthLevels` (5) levels
- Applies exponential distance decay (`BookDepthDecayLambda = 1000 ms`) so deep walls weigh less than the touch
- Rejects signals when flat or weighted skew contradicts the Level-1 touch (`SpoofWeightedThreshold`, `SpoofLevel1Reject`)
- Emits `DepthFlow` measurements with bid depth, ask depth, and directional pressure

Adverse `DepthFlow` or `Dump` measurements from this signal cancel resting maker bids in the trader.

---

### ⚡ Hawkes

Bivariate self-exciting point process fitted on the trade stream.

Hawkes processes model how trade arrivals excite future arrivals: a burst of buys raises the intensity of subsequent buys. This captures clustering that moving averages miss.

- Per-symbol ring of recent trades (capped slice)
- Bivariate fit: separate buy/sell intensity with mutual excitation
- MLE refit throttled by `HawkesFitCooldown` (5 s) to avoid churning on thin symbols
- Per-symbol RWMutex allows cross-symbol parallelism
- Emits `Momentum` measurements with current arrival intensity

---

### 📡 LeadLag

Altcoin catch-up signal using asynchronous cross-correlation.

- BTC/EUR serves as the anchor asset
- Peak detector fires on anchor moves; lag time to altcoin response is measured
- Per-symbol 256-bar ring buffer; Pearson correlation computed at 200ms throttle (avoids O(ring × maxLag × symbols) explosion)
- Emits `LeadLag` measurements: when BTC moves and an altcoin hasn't yet, the signal forecasts the catch-up

---

### 💧 Liquidity

Cross-section quote volume relative to peer median.

- Tracks daily quote volume (sum of `trade volume × price`) per symbol
- Compares against the running cross-section median
- Emits `Liquidity` measurements; illiquid symbols get negative values
- The prediction model learns whether low liquidity is a leading indicator for the symbol

---

### 🌡️ Sentiment

Breadth of bullish returns across the full symbol set.

- Tracks per-symbol return since the last observation
- A `Sentiment` measurement fires when the breadth (fraction of symbols up) exceeds `minBreadth` (0.55) and an anchor price move threshold is crossed
- Designed as a macro overlay, not a per-symbol trigger

---

### 🔗 Correlation

Synchronized return correlation across asset pairs.

- Maintains `CorrelationBarSeconds × MinCorrelationSamples` bar windows
- Computes Pearson cross-symbol correlation matrix
- Emits `Momentum` measurements when correlation exceeds threshold
- Also feeds the portfolio risk gate: if two symbols have r > `MaxSymbolCorrelation` (0.85), a new entry for the second is blocked when `MaxCorrelatedSlots` (1) is already open

---

### 🌊 Fluid

Book-flow field dynamics using a spatial grid.

- Partitions order-book depth into a `FluidGridSize × FluidGridSize` (32×32) grid
- Tracks order residence time (time-of-flight) per price level
- Height EMA smoothed at `FluidHeightEMAAlpha` (0.35)
- Fill-to-cancel flux gate (`MinFillToCancelRatio = 0.15` over `BookFluxWindow = 10 s`) — the first snapshot is discarded so resting liquidity is not mistaken for cancel spam
- Emits `Flow` measurements; also sends `field_row` frames directly to the UI

---

### 🧪 Causal

Pearl's causal ladder applied to microstructure data.

Implements association → intervention → counterfactual reasoning on a small DAG:

```
MacroMomentum ──► PriceVelocity ◄── LocalFlow
                       ▲
                   Liquidity (backdoor control)
```

- Hayashi-Yoshida estimator for asynchronous covariance (handles irregular tick times)
- Nonlinear ridge regression under structural breaks (Kalman-gated Q threshold: `CausalConditionSwitch = 1000`)
- Contagion break detection (`CausalContagionBreak = 0.9`, `CausalContagionWindow = 128`)
- The math layer operates on indexed DAG nodes, not finance-named fields
- Emits `Causal` measurements with treatment effect estimates

---

### 📊 CVD

Cumulative Volume Delta — aggregate order-flow imbalance.

- Tracks running buy volume minus sell volume across the trade stream
- Simpler than DepthFlow (no book-level decay, purely trade-side)
- Emits `Pump` or `Dump` measurements based on delta sign and magnitude
- Complements DepthFlow: one reads the book, the other reads executed flow

---

### ☠️ Toxicity

Order-book toxicity via fill-to-cancel ratio.

- Requires L3 (order-by-order) feed; disabled when `SYMM_KRAKEN_API_KEY` is absent
- Monitors order add/cancel/fill events at individual order granularity
- Low `fill_to_cancel` ratio signals spoof-heavy conditions
- Emits `Momentum` measurements; high toxicity acts as a caution overlay

---

### 🚪 Exhaust *(exit-only)*

Detects when a position's microstructure support has degraded.

Exhaust is the only signal that does not emit `Measurement` values to the `measurements` broadcast. It emits `Exit` signals directly to the `exits` channel.

**Soft exits** (suppressed for `MinExhaustHold = 5 s` after entry):

| Reason           | Trigger                                   |
|------------------|-------------------------------------------|
| `book_thinning`  | Order-book depth drops below threshold    |
| `spread_widen`   | Bid-ask spread expands beyond normal band |
| `imbalance_flip` | Book imbalance reverses sharply           |
| `pressure_fade`  | Trade pressure decays below entry level   |

**Hard exits** (always immediate):

| Reason           | Trigger                                        |
|------------------|------------------------------------------------|
| `stop_hit`       | Price crosses stop level in `price.Prediction` |
| `profit_target`  | Price reaches take-profit level                |
| `runway_expired` | `DueAt` elapsed, no structural exit yet        |

Peak exits (`imbalance_flip`, `pressure_fade` with urgency ≥ `ExitPeakUrgency`) emit a `peak_exit` UI event for immediate escape at the pump top. Urgency is a continuous `[0, 1]` value; only exits with urgency ≥ `ExitUrgencyThreshold` (0.65) reach the trader.

---

## Trader mechanics

`trader.Crypto` is the system that turns signal output into wallet events.

### Measurement ingestion

On each `measurements` message (coalescing any others already queued):

1. Observe price mark on the risk account
2. Check and update regime shock state
3. Apply per-source calibrator trust multiplier to raw confidence
4. Emit confidence gauge event to UI
5. Settle any predictions whose `DueAt` has passed
6. Find or create the perspective bucket for `(symbol, perspective-type)`
7. Add measurement to bucket
8. Attempt perspective prediction

### Entry gates (in order)

Before any position is opened, the candidate must clear every gate:

| Gate                     | Default       | Description                                             |
|--------------------------|---------------|---------------------------------------------------------|
| Cold-start               | ≥30 samples   | Bucket must have settled enough forecasts               |
| Statistical significance | z ≥ 1.96      | Mean realized return must be positive                   |
| Edge multiple            | ≥2× friction  | `predictedReturn ≥ EntryEdgeMultiple × round-trip cost` |
| Take-profit ratio        | ≥2R           | `predictedReturn ≥ TakeProfitR × stop_distance`         |
| Spread cap               | configurable  | `spread_bps ≤ MaxSpreadBPS` (0 = disabled)              |
| Slippage cap             | 50 bps        | Estimated fill slippage must be within bounds           |
| Pump anti-chase          | 3–20% retrace | For `pump_fast` regime only                             |
| Daily loss               | €20           | Running daily loss below limit                          |
| Per-trade loss           | €2            | Worst-case loss on this position below limit            |
| Drawdown                 | derived       | Portfolio drawdown below limit                          |
| Correlation              | r ≤ 0.85      | At most one open position per correlation cluster       |
| Portfolio dampener       | continuous    | Eigenmode covariance against open positions             |

The portfolio dampener applies a continuous multiplier derived from remaining drawdown capacity and the candidate's covariance with open symbols. It suppresses the executable edge without discarding the underlying forecast.

### Execution path

**Paper (default):**
- Immediate fill at ask + slippage for entries; bid − adverse-selection for exits
- Friction model: taker fee both ways plus full bid-ask spread
- No maker entries by default (`UseMakerEntries = false`)

**Live (API keys required):**
- Market order via Kraken WebSocket v2 private client
- Optional OTO stop-loss-limit attached at entry
- Fill deduplication via LRU ring (4096 slots) prevents double-application
- Maker fallback: after `ExecutionMakerFallbackTicks` (4) retries, falls back to market; only after cancel acknowledgement confirms the resting order is no longer live

### Regime shock breaker

`regimeShockBreaker` watches shock-capable telemetry (`correlation`, `fluid`) against its own rolling median/MAD history. When a discontinuous outlier breaches 6σ:

- Non-foundational sources are muted to `RegimeShockTrustFloor` (0.02)
- Feedback learning for those sources is paused
- Recovery requires `RegimeShockRecoverySamples` (64) consecutive non-shock samples
- **Foundational sources** (`cvd`, `depthflow`) continue unaffected — the engine falls back to raw executed flow and book imbalance rather than stale parameterized fits

### Hindsight audit

Every entry *skip* decision is recorded against the same `(symbol, perspectiveSource, predictedAt, dueAt)` key used by `PredictionFeedback`. When the forecast settles, skipped entries are written to `runs/hindsight-*.jsonl` only if the realized forward return would have cleared the same economic gates used for entry. Post-due skip decisions are dropped. The row includes original skip reason, last valid skip reason, required return, return multiple, and realized ground truth — so tuning can distinguish genuinely actionable misses from noise.

---

## Prediction and feedback

`price.Prediction` runs as its own long-lived worker. It is the **only** settlement authority.

### Open prediction lifecycle

1. **Record**: `RecordPerspective(symbol, perspective, now)` stores anchor price, runway, `DueAt`, and `predictedReturn` (0 during cold-start)
2. **Observe**: every ticker arrival updates `quotes[symbol]` with `{last, bid, ask, event-time, local-time}`
3. **Settle**: when `DueAt` passes, compute realized return against stored quote; populate `Err`
4. **Emit**: `PredictionFeedback` on `feedback` broadcast

### Return model

`price.ReturnModel` learns per `(perspectiveSource, marketRegime)` bucket:
- Input: `(calibrated confidence, observed forward return)` pairs
- Requires `ForwardReturnMinSamples` (30) anchored settlements
- Requires mean return passing z ≥ `ForwardReturnSignificanceZ` (1.96)
- Until both conditions are met, `predictedReturn = 0`
- Smoothed via `ForwardReturnSlopeAlpha` (0.05 EMA)

### Stop management

`price.Prediction` arms and evaluates stops on every tick:

| Stop type       | Trigger                         | Notes                                |
|-----------------|---------------------------------|--------------------------------------|
| Hard floor      | price ≤ floor                   | Set at entry; absolute minimum       |
| Take-profit     | price ≥ target                  | `TakeProfitCapture × ExpectedReturn` |
| Trailing stop   | retrace from peak > `trailFrac` | Updates peak on each new high        |
| Pump trailing   | retrace > `PumpTrailPct` (8%)   | Tighter; for `pump_fast` regime      |
| Pump hard floor | 12% below entry                 | Absolute floor for pump positions    |

Stops fire as `stop_hit` exits with fill price capped at the trigger level; they bypass `MinExhaustHold` and are never suppressed.

---

## Risk and sizing

### Kelly sizer

Per-source fractional Kelly from settled feedback history:

```
f* = (p × b − q) / b
```

where `p` = win ratio, `q` = 1 − p, `b` = average win / average loss.

Executable size = `f* × KellyFraction (0.5) × available_cash`. The 0.5 multiplier is a deliberate safety factor — full Kelly is theoretically optimal only under distributional assumptions that are rarely satisfied in microstructure data.

### Portfolio risk gates

| Gate                  | Parameter              | Default             |
|-----------------------|------------------------|---------------------|
| Max slot %            | `MaxSlotPct`           | 5% of wallet        |
| Max loss per trade    | `MaxLossPerTradeEUR`   | €2                  |
| Max daily loss        | `MaxDailyLossEUR`      | €20                 |
| Max drawdown          | derived                | daily loss ÷ wallet |
| Max correlated slots  | `MaxCorrelatedSlots`   | 1                   |
| Correlation threshold | `MaxSymbolCorrelation` | 0.85                |

There is no fixed global slot count. Capacity is determined jointly by cash, Kelly sizing, optional deploy cap, drawdown headroom, spread/slippage estimates, and correlation structure.

---

## Calibration

### PredictionCalibrator

Each signal maintains one calibrator per `(symbol, regime)` bucket. The calibrator is an asymmetric scalar Kalman filter:

- **Downside fast**: gain spikes on 6σ loss events — immediate contraction of trust
- **Upside gradual**: recovery delayed over `RegimeShockRecoverySamples` (64) samples
- **Baseline drift**: α = 0.05 (slow in calm markets)

Adaptive half-life: between `CalibrationHalfLifeFloor` (2 s) and `CalibrationHalfLifeCeiling` (15 m), scaled to the measurement's runway.

The calibrator adjusts the **scale** of internal signal parameters — it does not post-hoc decorate raw confidence with a historical win rate.

### Confidence fence

Per `(symbol, source)` confidence history:
- Fence = Q3 + 1.5 × IQR (robust upper bound for the source on this symbol)
- Normalized confidence = `raw / (raw + fence)`, clamped `[0, 1]`
- Returns 0 until `MinConfidenceHistory` (4) samples exist

---

## UI and telemetry

`ui.Hub` subscribes to the `ui` broadcast and fans out to WebSocket clients at `ws://127.0.0.1:8765/ws`.

**Lossy telemetry ring:** default 512 slots. When browsers or chart payloads fall behind, old frames are overwritten rather than back-pressuring trading goroutines.

**Focus set**: `{BTC/EUR anchor} ∪ {symbols with open positions}`. Symbol-specific frames outside the focus set are dropped before client write. Aggregate frames (audit, prediction, wallet) always pass.

**On reconnect**: the hub writes the latest wallet, confidence, field, candle, and mark snapshots directly to the new socket before live streaming resumes.

**Priority heartbeat**: a monotonic sequence + throttling counters so the dashboard can distinguish chart throttling from engine offline.

### UI frame events

| Event                | Source                | Contents                                         |
|----------------------|-----------------------|--------------------------------------------------|
| `tick`               | PublicClient          | price, volume per symbol                         |
| `confidence`         | Crypto                | per-source EMA confidence gauge                  |
| `wallet`             | Crypto                | balance, inventory, marks, PnL                   |
| `audit`              | Crypto                | gate checks, edge calc, entry/exit detail        |
| `prediction`         | Crypto                | perspective source, predicted return, confidence |
| `prediction_settled` | Crypto                | realized vs predicted, error, sources            |
| `engine_pulse`       | Crypto                | `avg_prediction_multiple`, `avg_error_multiple`  |
| `candle_bar`         | PublicClient          | OHLC + volume for chart                          |
| `mark`               | PublicClient / Crypto | live mark price per symbol                       |
| `field_row`          | Fluid                 | book-flow grid row for spatial visualization     |
| `heartbeat`          | Hub                   | monotonic seq, queue depth, drop count           |

---

## Numeric layer

Signal internals and calibration lean on `numeric/` and `numeric/adaptive/` rather than hand-written constants.

### Derived pipeline

`numeric/dynamic.go` chains `Dynamic` filters:

```
EMA → SigmaClamp → Peak → ...
```

Each stage calls `Next(out, ...values)` and feeds into the next. Stages nest freely; `Value()` reads the last output without pushing a new observation.

### Adaptive primitives

| Type         | Location                 | Behavior                                                                                                     |
|--------------|--------------------------|--------------------------------------------------------------------------------------------------------------|
| `EMA`        | `adaptive/ema.go`        | Auto-bootstraps on first observation; adaptive rate derived from per-tick delta relative to observed range   |
| `SigmaClamp` | `adaptive/`              | Kalman-like volatility detector; clamps outliers beyond N-sigma                                              |
| `Peak`       | `adaptive/peak.go`       | Stateless; returns new peak on every observation                                                             |
| `Classifier` | `adaptive/classifier.go` | Discretizes continuous values using configurable thresholds                                                  |
| `FracDiff`   | `adaptive/fracdiff.go`   | Fractional differentiation (order 0.4, width 16); preserves long-range memory while reducing AR(1) structure |
| `Kalman`     | `adaptive/kalman.go`     | Scalar Kalman with state-dependent measurement covariance and asymmetric gain                                |

### Hayashi-Yoshida covariance

Cross-asset covariance uses allocation-free Hayashi-Yoshida interval overlap — handles asynchronous, irregularly-sampled tick data without interpolation. Stale intervals are capped to the current microstructure window.

### Robust statistics

`numeric/` provides `Median`, `Mean`, `PercentileSorted`, `Quartiles`, and `MedianAbsoluteDeviation` — robust statistics that resist the outliers that are routine in microstructure data.

---

## Build and run

SYMM links against `qpool`, which requires a linkname flag. **Always use the Makefile:**

```bash
make build          # → bin/symm
make run            # build + run (paper defaults)
make test-go        # full test suite with correct ldflags
make bench          # package benchmarks
```

Using bare `go test ./...` without `-ldflags=-checklinkname=0` will fail.

**Replay captured traffic:**

```bash
make replay REPLAY_FILE=replay/fixtures/sample.jsonl REPLAY_PACE=50ms
```

**Frontend (separate terminal):**

```bash
cd frontend && pnpm install && pnpm dev
```

### Environment variables

| Variable                 | Effect                                     |
|--------------------------|--------------------------------------------|
| `SYMM_REPLAY_FILE`       | JSONL replay instead of live WebSocket     |
| `SYMM_REPLAY_PACE`       | Delay between replay lines (e.g., `50ms`)  |
| `SYMM_KRAKEN_API_KEY`    | Enables private client + live orders       |
| `SYMM_KRAKEN_API_SECRET` | Paired with above                          |
| `SYMM_UI_ADDR`           | WebSocket listen address (default `:8765`) |
| `SYMM_WALLET_EUR`        | Starting paper wallet (default `200.0`)    |
| `SYMM_QUOTE_CURRENCY`    | Quote currency (default `EUR`)             |

Full environment wiring is in `config/config.go`.

---

## Configuration reference

<details>
<summary>📋 Risk &amp; position sizing</summary>

| Field                     | Default | Description                               |
|---------------------------|---------|-------------------------------------------|
| `WalletEUR`               | `200.0` | Paper trading capital                     |
| `MaxSlotPct`              | `0.05`  | Max per-position fraction of wallet       |
| `MaxLossPerTradeEUR`      | `2.0`   | Hard loss cap per trade                   |
| `MaxDailyLossEUR`         | `20.0`  | Daily loss ceiling                        |
| `MaxPortfolioDrawdownPct` | derived | Daily loss ÷ wallet                       |
| `MaxDeployPct`            | `1.0`   | Max fraction of wallet deployed at once   |
| `KellyFraction`           | `0.5`   | Conservative multiplier on Kelly estimate |
| `AllowPaperShorts`        | `false` | Enable short selling in paper mode        |
| `AllowLiveShorts`         | `false` | Enable short selling with live orders     |

</details>

<details>
<summary>📋 Entry economics</summary>

| Field                    | Default | Description                                |
|--------------------------|---------|--------------------------------------------|
| `EntryEdgeMultiple`      | `2.0`   | Forecast must be ≥ N× round-trip friction  |
| `TakeProfitR`            | `2.0`   | Forecast must be ≥ N× stop distance        |
| `TakeProfitCapture`      | `0.75`  | Exit at this fraction of expected return   |
| `MinCostEUR`             | `0.45`  | Minimum trade size (avoids fee domination) |
| `MinQuoteCoverage`       | `0.95`  | Require 95% bid/ask book coverage          |
| `MaxEntrySlippageBPS`    | `50`    | Hard slippage cap                          |
| `MaxSpreadBPS`           | `0`     | Max allowed spread (0 = disabled)          |
| `ForecastSpreadMultiple` | `4`     | Forecast must be ≥ N× spread               |
| `UseMakerEntries`        | `false` | Use limit orders for entries               |
| `MakerFeePct`            | `0.16`  | Maker fee rate                             |
| `AdverseSelectionBPS`    | `5.0`   | Extra cost added to maker entry estimates  |

</details>

<details>
<summary>📋 Exit &amp; hold times</summary>

| Field                  | Default | Description                                 |
|------------------------|---------|---------------------------------------------|
| `ScalpHoldBeforeExit`  | `90s`   | Minimum hold before scalp exit eligibility  |
| `FlowHoldBeforeExit`   | `30s`   | Minimum hold before flow exit eligibility   |
| `MinHoldBeforeRotate`  | `1m`    | Minimum hold before re-entry on same symbol |
| `MinExhaustHold`       | `5s`    | Suppress soft exits after entry             |
| `ExitEvery`            | `10ms`  | Exit scan frequency                         |
| `ExitUrgencyThreshold` | `0.65`  | Exhaust urgency threshold for acting        |

</details>

<details>
<summary>📋 Stop &amp; trail parameters</summary>

| Field                 | Default | Description                                        |
|-----------------------|---------|----------------------------------------------------|
| `StopVolMultiple`     | `8.0`   | Stop = N× recent per-tick volatility               |
| `PumpTrailPct`        | `0.08`  | Fast pump trailing stop: 8% retrace from peak      |
| `PumpSlowTrailPct`    | `0.20`  | Slow pump trailing stop                            |
| `PumpHardStopPct`     | `0.12`  | Hard floor 12% below entry for pump positions      |
| `PumpSizeFraction`    | `0.25`  | Pump positions sized at 25% of normal slot         |
| `PumpPullbackMin`     | `0.03`  | Minimum retrace before pump entry (anti-chase)     |
| `PumpPullbackMax`     | `0.20`  | Maximum retrace before pump leg is considered dead |
| `TrailSpreadMultiple` | `2`     | Trail width = N× mid-spread                        |
| `DefaultTrailPct`     | `0.35`  | Default trailing stop percentage                   |
| `MinTrailPct`         | `0.15`  | Minimum trail width                                |
| `MaxTrailPct`         | `3.0`   | Maximum trail width                                |
| `TrailRiskEMAAlpha`   | `0.2`   | Trail risk smoothing                               |

</details>

<details>
<summary>📋 Prediction &amp; calibration</summary>

| Field                         | Default | Description                                    |
|-------------------------------|---------|------------------------------------------------|
| `PriceHistory`                | `128`   | Return model window                            |
| `ForwardReturnMinSamples`     | `30`    | Min settlements before a bucket can trade      |
| `PumpForwardReturnMinSamples` | `8`     | Reduced requirement for pump buckets           |
| `ForwardReturnSignificanceZ`  | `1.96`  | Min z-score for positive return (95% CI)       |
| `ForwardReturnSlopeAlpha`     | `0.05`  | EMA smoothing on slope                         |
| `MaxActivePerspectives`       | `2`     | Max concurrent bets per symbol                 |
| `PerspectiveTTL`              | `30s`   | Discard measurements older than this           |
| `MaxPerspectiveMeasurements`  | `256`   | Cap on measurements per perspective bucket     |
| `MinCalibrationSamples`       | `12`    | Min feedback samples before calibration adapts |
| `CalibrationHalfLifeFloor`    | `2s`    | Minimum calibration half-life                  |
| `CalibrationHalfLifeCeiling`  | `15m`   | Maximum calibration half-life                  |

</details>

<details>
<summary>📋 Regime shock detection</summary>

| Field                        | Default | Description                          |
|------------------------------|---------|--------------------------------------|
| `RegimeShockWindow`          | `128`   | Rolling history for shock detector   |
| `RegimeShockMinSamples`      | `64`    | Min samples before shock can trigger |
| `RegimeShockZScore`          | `6`     | Sigma threshold for shock detection  |
| `RegimeShockRecoverySamples` | `64`    | Samples needed for recovery          |
| `RegimeShockTrustFloor`      | `0.02`  | Muted trust level during shock       |

</details>

<details>
<summary>📋 Signal-specific parameters</summary>

| Field                    | Default | Description                                |
|--------------------------|---------|--------------------------------------------|
| `FastPumpWindow`         | `10s`   | Fast pump detection window                 |
| `MediumPumpWindow`       | `5m`    | Medium pump detection window               |
| `FastPumpVolumeRatio`    | `15`    | Fast pump RVOL threshold                   |
| `SlowRVOLThreshold`      | `5`     | Slow breakout RVOL threshold               |
| `HawkesFitCooldown`      | `5s`    | Hawkes MLE refit minimum interval          |
| `BookDepthLevels`        | `5`     | Order book snapshot depth                  |
| `BookDepthDecayLambda`   | `1000`  | Volume weight decay half-life (ms)         |
| `SpoofWeightedThreshold` | `0.5`   | Spoof detection weighted skew threshold    |
| `SpoofLevel1Reject`      | `-0.1`  | Level-1 contradiction threshold            |
| `MinFillToCancelRatio`   | `0.15`  | Toxicity gate threshold                    |
| `BookFluxWindow`         | `10s`   | Book flux measurement window               |
| `FluidGridSize`          | `32`    | Fluid dynamics grid dimension              |
| `CorrelationBarSeconds`  | `10`    | Bar size for correlation computation       |
| `MaxSymbolCorrelation`   | `0.85`  | Correlation gate threshold                 |
| `MaxCorrelatedSlots`     | `1`     | Max open positions per correlation cluster |

</details>

<details>
<summary>📋 Symbol selection &amp; sampling</summary>

| Field                    | Default | Description                           |
|--------------------------|---------|---------------------------------------|
| `MaxScanSymbols`         | `64`    | Max symbols under active watch        |
| `SubscribeBatch`         | `50`    | Symbol subscribe batch size           |
| `SymbolActivityHalfLife` | `30s`   | Activity score decay rate             |
| `WinBoostHalfLife`       | `2h`    | Up-weight recently profitable symbols |
| `OHLCIntervalMinutes`    | `5`     | OHLC bar interval for warm-up         |
| `OHLCMaxSymbols`         | `64`    | Max symbols subscribed for OHLC       |
| `VolumeClockBarsPerDay`  | `8640`  | Volume clock resolution               |

</details>

<details>
<summary>📋 Execution &amp; infrastructure</summary>

| Field                         | Default | Description                                |
|-------------------------------|---------|--------------------------------------------|
| `ExecutionMakerFallbackTicks` | `4`     | Retries before maker falls back to market  |
| `PaperOrderLatency`           | `0`     | Simulated paper order latency              |
| `PaperMinFillCoverage`        | `1`     | Minimum fill coverage in paper mode        |
| `PaperOrderRejectRate`        | `0`     | Simulated paper order reject rate          |
| `LiveInventoryEpsilon`        | `1e-8`  | Precision tolerance for live inventory     |
| `UIAddr`                      | `:8765` | WebSocket listen address                   |
| `UITelemetryBuffer`           | `512`   | Lossy telemetry ring size                  |
| `UIHeartbeatInterval`         | `250ms` | Heartbeat cadence                          |
| `LogDir`                      | `runs`  | Directory for run logs and hindsight files |
| `LogLevel`                    | `info`  | Logging verbosity                          |

</details>

---

## Repository map

| Path                | Contents                                                                   |
|---------------------|----------------------------------------------------------------------------|
| `cmd/`              | Cobra entry point, booter, system registration                             |
| `engine/`           | `Measurement`, `Perspective`, `Prediction`, feedback and calibration types |
| `kraken/`           | WebSocket clients (public, private, L3), market types, OHLC helpers        |
| `pumpdump/`         | Multi-scale volume spike detector                                          |
| `depthflow/`        | Distance-weighted book imbalance                                           |
| `hawkes/`           | Bivariate self-exciting process                                            |
| `leadlag/`          | Anchor/laggard cross-correlation                                           |
| `liquidity/`        | Cross-section quote volume                                                 |
| `sentiment/`        | Bullish breadth signal                                                     |
| `correlation/`      | Return correlation matrix                                                  |
| `fluid/`            | Book-flow field dynamics                                                   |
| `causal/`           | Pearl-ladder DAG causal inference                                          |
| `cvd/`              | Cumulative volume delta                                                    |
| `toxicity/`         | L3 fill-to-cancel toxicity                                                 |
| `exhaust/`          | Exit urgency signal                                                        |
| `trader/`           | Wallet, Crypto scorer, sizing, risk gates                                  |
| `price/`            | Prediction lifecycle, stop management, return model                        |
| `broker/`           | Buy/sell execution (paper and live)                                        |
| `wallet/`           | Balance, inventory, fill deduplication                                     |
| `ui/`               | WebSocket hub, telemetry ring                                              |
| `frontend/`         | React dashboard                                                            |
| `numeric/`          | Derived pipelines, EMA, SigmaClamp, FracDiff, Kalman, Hayashi-Yoshida      |
| `numeric/adaptive/` | Adaptive EMA, peak, classifier, robust statistics                          |
| `config/`           | All config fields, defaults, and environment wiring                        |
| `AGENTS.md`         | Agent contract: tests, benchmarks, style                                   |

Adding a signal means: implement `System`, subscribe to the market channels you need, publish `Measurement` values with prices attached, and register the constructor in `cmd/root.go`. The existing trader, prediction machinery, and feedback loop handle the rest.
