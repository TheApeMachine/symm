# SYMM — Shake Your Money Maker

A Kraken spot microstructure engine that turns live market data into scored trade ideas, forward-return forecasts, and closed-loop calibration. Signals read the wire; the trader reads only what signals publish. Nothing in the hot path sleeps on a timer—the booter spins every registered `System` in parallel as fast as the machine will run.

---

## What it is

SYMM is not a single model or a single strategy. It is a **fleet of independent observers** (hawkes clustering, pump/dump volume spikes, book flow, cross-asset lag, sentiment breadth, and others), each implemented as its own package, plus a **trader** that fuses their outputs and a **prediction layer** that learns whether those fusions were right.

The default wallet is **paper** (€200, configurable). Point it at Kraken WebSocket v2 for live data; optionally add API keys for real orders. Point it at a JSONL replay file for offline runs. A React dashboard listens on a local WebSocket for wallet, confidence, feedback, candles, and engine telemetry.

---

## The one rule: everything is a `System`

Every runnable unit implements the same contract:

```go
type System interface {
    Start() error
    State() State
    Tick() error
    Close() error
}
```

Each `Tick` is a tight `select`: handle one incoming message if present, otherwise return immediately. No blocking waits in the default branch. Systems do not call each other; they **publish and subscribe** through named broadcast groups on a shared `qpool.Q`.

Registration lives in `cmd/root.go`. Order matters only insofar as signals should run before the trader in the same booter round so measurements are on the wire when `Crypto` scores—but all systems are scheduled **concurrently** each loop via `ScheduleFast`, so throughput stays high.

---

## How a booter round works

```
┌─────────────────────────────────────────────────────────────┐
│  Booter loop (no sleep — runs as fast as CPUs allow)        │
│    for each System: ScheduleFast(Tick)  →  wait all         │
└─────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
   PublicClient          Signal packages      Prediction + Crypto
   (Kraken WS)           (per-symbol logic)   (measurements / settle)
```

1. **Public client** maintains the WebSocket, parses Kraken v2 frames, and fans out `tick`, `trade`, `book`, `symbols`, `ohlc` (and `ui` events like `candle_bar`).
2. **Signals** consume those channels, maintain per-symbol state, and when their criteria fire they emit `engine.Measurement` values on the **`measurements`** group.
3. **`price.Prediction`** subscribes to **`tick`** only to mark prices for settlement; it does not trade.
4. **`trader.Crypto`** subscribes to **`measurements`** only. When a batch arrives it builds perspectives, records predictions, may enter a position, and publishes `engine_pulse` on **`ui`**.

The UI hub (`ui.Hub`) mirrors `wallet`, `confidence`, `feedback`, `ohlc`, `executions`, `orders`, `exits`, and `ui` to browsers at `ws://127.0.0.1:8765/ws` (configurable via `SYMM_UI_ADDR`).

---

## The data pipeline (this is the product)

This is the spine. If you remember one diagram, remember this:

```
Kraken WS ──► Signals ──► measurements ──► Crypto (trader)
                              │                │
                              │                ├──► perspectives
                              │                ├──► Prediction.Record (always)
                              │                └──► optional paper/live entry
                              │
                              ▼
                         (Last, Bid, Ask on each Measurement)

Prediction ◄── tick (settlement prices only)
     │
     └──► feedback ──► Signals.Feedback (calibrator scale)
```

### 1. Measurements

A measurement is one signal’s opinion about one moment in the market:

| Field                | Role                                                                                                       |
|----------------------|------------------------------------------------------------------------------------------------------------|
| `Source`             | Which signal (`hawkes`, `pumpdump`, `fluid`, …)                                                            |
| `Type`               | Regime class (`Pump`, `Flow`, `LeadLag`, …)                                                                |
| `Confidence`         | How completely the **current** observation matches that signal’s criteria (not “probability of profit”)    |
| `Pairs`              | Symbol(s), usually one Kraken pair                                                                         |
| `Last`, `Bid`, `Ask` | Quote at emit time — the trader anchors predictions and paper fills from these, not from its own tick feed |
| `Regime`, `Reason`   | Human- and machine-readable labels                                                                         |

Signals publish measurements when their internal `publishPulse()` runs—typically after a relevant `tick`, `trade`, or `book` event on a subscribed symbol.

### 2. Perspectives

A **perspective** is not computed by a model. It is a **bucket**: all measurements in one scoring batch that share the same **symbol** and the same **perspective type**, stored together so each can get its own prediction while still belonging to one fused view of that symbol for that lens.

This happens in `trader/crypto.go` inside `score`, before any call to `Prediction.Record`.

#### Step 1 — Classify each measurement’s type

`perspectiveType(measurement)` maps `measurement.Type` to one of four `engine.PerspectiveType` values:

| `measurement.Type`                                                                   | Perspective bucket          |
|--------------------------------------------------------------------------------------|-----------------------------|
| `Flow`, `DepthFlow`                                                                  | `PerspectiveFlow`           |
| `Basis`, `LeadLag`                                                                   | `PerspectiveCrossAsset`     |
| `Sentiment`, `Causal`                                                                | `PerspectiveSentiment`      |
| Everything else (`Pump`, `Dump`, `Momentum`, `Liquidity`, `Hawkes` momentum/dump, …) | `PerspectiveMicrostructure` |

#### Step 2 — Nest maps: symbol → type → slice of measurements

For each measurement in the batch (skipping empty `Pairs`):

1. Take **symbol** = `measurement.Pairs[0].Wsname` (first pair on the measurement).
2. Take **perspective type** = result of `perspectiveType(measurement)`.
3. Look up `perspectives[symbol][perspectiveType]`.
4. If missing, create an empty `engine.Perspective` with that `Type`.
5. **Append** this measurement to `perspective.Measurements`.
6. Write the perspective back into `perspectives[symbol][perspectiveType]`.

Data structure:

```text
perspectives: map[symbol]map[PerspectiveType]Perspective
                      └── Perspective { Type, Measurements: []Measurement }
```

There is **no averaging**, **no voting**, and **no merge of confidence** at this stage—only grouping. `Regime` on `engine.Perspective` is not set here (it stays the zero value).

#### Example (one batch)

| Source     | Symbol     | Type       | → Bucket                                                                |
|------------|------------|------------|-------------------------------------------------------------------------|
| `pumpdump` | `PUMP/EUR` | `Pump`     | `PUMP/EUR` / Microstructure                                             |
| `hawkes`   | `PUMP/EUR` | `Momentum` | `PUMP/EUR` / Microstructure (same bucket—two entries in `Measurements`) |
| `fluid`    | `PUMP/EUR` | `Flow`     | `PUMP/EUR` / Flow (separate bucket)                                     |
| `leadlag`  | `SOL/EUR`  | `LeadLag`  | `SOL/EUR` / Cross-asset                                                 |

That yields **three** perspective objects for this batch, not three symbols × four types unless types differ.

#### Step 3 — Predict per measurement, not per bucket

The code then walks **every** perspective and **every** measurement inside it:

```text
for each symbol → for each perspectiveType → for each measurement in perspective.Measurements
    Record(perspective, measurement, anchorPrice(measurement), now)
```

So the **same** `engine.Perspective` value (with multiple measurements attached) is passed into `Record` for each member. `Prediction` stores one open forecast per `(symbol, source)`—`source` comes from `measurement.Source`, not from the bucket name. Two signals in the same perspective on the same symbol still produce **two** records if their `Source` strings differ.

Entry selection uses the single measurement that achieved the highest **predicted return** in that batch (`bestMeasurement`), not the whole perspective aggregate.

### 3. Predictions (always, not only on entry)

For every measurement in every perspective, `Crypto` calls `price.Prediction.Record`. That stores an open forecast with:

- **Anchor** — `Last` on the measurement (or mid of bid/ask if needed)
- **Runway** — hold horizon by type (scalp vs flow vs causal)
- **Predicted return** — `confidence × |EWMA(actual forward return)|` once the source has at least `MinCalibrationSamples` (default 12) settled returns

Until calibration warms up, predicted return is zero but the open record still exists; settlement still updates the return EMA when anchor and market price are valid.

### 4. Settlement and feedback

`Prediction.Tick` (on its own schedule in the booter) ingests ticks for **mark prices**, expires forecasts whose `dueAt` has passed, compares **actual** forward return to **predicted**, and broadcasts `PredictionFeedback` on **`feedback`** when `ValidPredictionFeedback` passes (non-zero predicted return, anchored).

Each entry signal subscribes to `feedback` and routes errors into its per-symbol **`PredictionCalibrator`** (`engine` + `numeric/learned`), which scales internal parameters—not post-hoc confidence cosmetics.

Predicted return formula:

```text
predictedReturn = measurement.Confidence × |EMA(actualReturn per source)|
```

---

## Registered systems

| System         | Package         | Role                                                             |
|----------------|-----------------|------------------------------------------------------------------|
| Public client  | `kraken/client` | Kraken WS v2: instruments, tickers, trades, book, OHLC           |
| Pump/dump      | `pumpdump`      | Multi-scale volume spikes: 10s fast pump, 5m medium, 14d RVOL slow breakout |
| Depth flow     | `depthflow`     | Distance-weighted book imbalance + Level-1 spoof rejection |
| Hawkes         | `hawkes`        | Bivariate Hawkes trade clustering (buy/sell excitation); MLE refit throttled by `HawkesFitCooldown` |
| Lead/lag       | `leadlag`       | Anchor pair vs laggard change                                    |
| Liquidity      | `liquidity`     | Quote volume below cross-section median                          |
| Sentiment      | `sentiment`     | Cross-section bullish breadth                                    |
| Fluid          | `fluid`         | Weighted book flow, fill-to-cancel flux gate, trade pressure (also `field_row` to UI) |
| Causal         | `causal`        | Pearl-ladder style uplift from microstructure samples            |
| Exhaust        | `exhaust`       | Exit **urgency** on `exits` channel (not an entry measurement)   |
| Prediction     | `price`         | Open forecasts, settlement, feedback                             |
| Crypto         | `trader`        | Consumes measurements and exhaust exits; paper wallet + optional live orders |
| Private client | `kraken/client` | Optional; live orders and fills when API keys set                |

Entry signals share the same shape: subscribe to market channels, request deeper subscriptions when a symbol qualifies, `Measure()` per symbol, publish to `measurements` with prices attached.

**Pump regimes (`pumpdump`):** `fast_pump` when 10s volume / fast baseline exceeds `FastPumpVolumeRatio`; `actual_pump` on the 5m medium window; `slow_breakout` when 1h volume / 14d median hourly volume exceeds `SlowRVOLThreshold`. OHLC warm-up seeds the slow baseline via REST.

**Anti-spoof book filters (`depthflow`, `fluid`):** imbalance uses exponential distance decay (`BookDepthDecayLambda`) so deep walls weigh less than the touch. Entries reject when flat or weighted skew contradicts Level-1 touch (`SpoofWeightedThreshold`, `SpoofLevel1Reject`). `fluid` additionally tracks per-level book change flux vs trade flux (`MinFillToCancelRatio` over `BookFluxWindow`); the first snapshot is ignored so resting liquidity is not mistaken for cancel spam.

---

## Trading behavior (`trader.Crypto`)

On each `measurements` message (coalescing any others already queued):

1. Build perspectives and call `Record` for each measurement.
2. Track the best **predicted return** in the batch.
3. After `MinWarmPulses` (default 50), if slots remain and `bestReturn ≥ MinEdgeReturn`, **enter** on that symbol using the winning measurement’s `Last` / `Bid` / `Ask`.
4. Publish per-source **confidence** averages and an **`engine_pulse`** UI event (`measurements`, `open`, `avg_prediction`, `avg_error`, `forecast_symbols`, `seq`).

Paper entries use maker limit fills at the bid when `UseMakerEntries` is true (lower `MakerFeePct`); taker fallback uses `SlippageFill`. Live entries post `LimitBuyBid` or `MarketBuyCash` on `orders`; the private client handles cancel/amend.

Before entry, fused perspective scoring requires `MinActivePerspectives` independent sources with joint confidence via `engine.FuseMeasurements`, edge above `MinRoundTripEdge` (derived from round-trip taker fees), fractional Kelly sizing from settled feedback, and `PortfolioRisk` gates. Blocked entries emit `entry_blocked`; adverse `depthflow`/`Dump` measurements cancel resting bids.

On each `exits` message from `exhaust` (urgency ≥ `ExitUrgencyThreshold`), `Crypto` closes inventory for that symbol: paper exits use `SlippageFill` with the last tick price from `price.Prediction`; live exits send `MarketSellBase` on `orders`. Peak exits (`imbalance_flip`, `pressure_fade` with urgency ≥ `ExitPeakUrgency`) emit a `peak_exit` UI event for immediate escape at the pump top.

---

## Build and run

Go 1.26+ links against `qpool`, which needs the linkname flag—**always use the Makefile**:

```bash
make build          # → bin/symm
make run            # build + run (paper defaults)
make test-go        # full test suite with correct ldflags
make bench          # package benchmarks
```

Replay captured traffic:

```bash
make replay REPLAY_FILE=replay/fixtures/sample.jsonl REPLAY_PACE=50ms
```

Frontend (separate terminal):

```bash
cd frontend && pnpm install && pnpm dev
```

Use `make` targets, not bare `go test ./...`, unless you pass `-ldflags=-checklinkname=0` yourself.

### Environment (common)

| Variable                                         | Effect                                     |
|--------------------------------------------------|--------------------------------------------|
| `SYMM_REPLAY_FILE`                               | JSONL replay instead of live WS            |
| `SYMM_REPLAY_PACE`                               | Delay between replay lines                 |
| `SYMM_KRAKEN_API_KEY` / `SYMM_KRAKEN_API_SECRET` | Enables private client + live orders       |
| `SYMM_UI_ADDR`                                   | WebSocket listen address (default `:8765`) |
| `SYMM_WALLET_EUR`, `SYMM_QUOTE_CURRENCY`, …      | See `config/config.go` for full set        |

---

## Numeric layer

Signal internals and calibration lean on `numeric/` and `numeric/adaptive/` (EMAs, windows, peaks, fences, learned forecast ratios)—not magic constants in the trader. Hawkes in particular fits a bivariate self-exciting model via constrained MLE (`hawkes/`), with timelines and decay helpers under `numeric/timeline` and `numeric/decay`.

---

## Mental model for operators

- **High booter tick count** (millions per minute) is expected: every system gets a concurrent `Tick` every loop iteration.
- **Zero `engine_pulse` measurements** usually means no signal passed `Measure()` yet, or measurements are not reaching `Crypto`’s subscriber— not because the loop is “too fast.”
- **`avg_prediction` and `forecast_symbols` stay at zero** until each source accumulates enough settled returns for non-zero predicted return; then feedback starts moving calibrators.
- **Confidence on the UI** is the signal’s self-alignment score, not the trader’s edge estimate; edge lives in predicted return after calibration.

---

## Repository map (where to look)

| Path          | Contents                                                  |
|---------------|-----------------------------------------------------------|
| `cmd/`        | Cobra entry, booter, system registration                  |
| `engine/`     | `Measurement`, `Perspective`, feedback, calibration types |
| `kraken/`     | WS clients, market types, OHLC subscribe helpers          |
| `*/signal.go` | One package per signal system                             |
| `trader/`     | Wallet, `Crypto` scorer                                   |
| `price/`      | Prediction lifecycle                                      |
| `ui/`         | WebSocket hub                                             |
| `frontend/`   | Dashboard                                                 |
| `config/`     | `System` defaults and env wiring                          |
| `AGENTS.md`   | Agent contract (tests, benchmarks, style)                 |

SYMM is built to add another signal by implementing `System`, subscribing to the market groups you need, publishing `Measurement` values with prices, and registering the constructor in `cmd/root.go`—then letting the existing trader and prediction machinery do the rest.
