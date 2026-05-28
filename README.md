# SYMM — Shake Your Money Maker

A Kraken spot microstructure engine that turns live market data into scored trade ideas, forward-return forecasts, and closed-loop calibration. Signals read the wire; the trader reads only what signals publish. The booter starts every registered `System` as a long-lived `qpool` worker; market cadence comes from WebSocket, replay, and internal broadcast traffic rather than timer polling.

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

Each `Tick` is the system's long-running event loop. It starts one goroutine per subscribed broadcast channel, so `tick`, `book`, `trade`, `feedback`, and control channels drain independently instead of competing inside one multiplexed select. Systems do not call each other; they **publish and subscribe** through named broadcast groups on a shared `qpool.Q`.

Registration lives in `cmd/root.go`. `Booter.Boot` starts the UI hub, refreshes wallet publishers, schedules one `Tick` worker per registered system with `ScheduleFast`, and waits for those workers to exit. Throughput comes from the concurrent workers and broadcast queues, not from repeated booter rounds.

---

## How system workers run

```
┌─────────────────────────────────────────────────────────────┐
│  Booter startup                                             │
│    for each System: ScheduleFast(long-running Tick)          │
│    wait until a worker exits or the context is canceled      │
└─────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
   PublicClient          Signal packages      Prediction + Crypto
   (Kraken WS)           (per-symbol logic)   (measurements / settle)
```

1. **Public client** maintains the WebSocket, parses Kraken v2 frames, and fans out `tick`, `trade`, `book`, `symbols`, `ohlc` (and `ui` events like `candle_bar`).
2. **Signals** consume those channels through per-channel subscriber readers, maintain per-symbol state, and when their criteria fire they emit `engine.Measurement` values on the **`measurements`** group.
3. **`price.Prediction`** subscribes to **`tick`** only to mark prices for settlement; it does not trade.
4. **`trader.Crypto`** scores **`measurements`**, applies **`feedback`**, handles **`exits`**, observes **`tick`** marks, and publishes `engine_pulse` on **`ui`**.

The UI hub (`ui.Hub`) mirrors `wallet`, `confidence`, `feedback`, `ohlc`, `executions`, `orders`, `exits`, and `ui` to browsers at `ws://127.0.0.1:8765/ws` (configurable via `SYMM_UI_ADDR`). Each mirrored channel has its own reader goroutine; websocket writes are serialized per browser connection. On reconnect the hub writes the latest wallet, confidence, field, candle, and mark snapshots directly to the new socket before live streaming continues. Live marks for open positions and every Kraken-subscribed symbol are pushed from `kraken/client/public.go` as `ui` events (`event: mark`, `symbol`, `price`) on ticker, trade, and OHLC close; `trader.Crypto` also rebroadcasts `wallet.Marks` (from `price.Prediction.LastPrice`) on each wallet update.

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

#### Step 3 — Predict per perspective bucket

The code then walks **every** perspective bucket:

```text
for each symbol → for each perspectiveType
    RecordPerspective(symbol, perspective, now)
```

`Prediction` stores one open forecast per `(symbol, perspectiveSource)`, where the source is `perspective:microstructure`, `perspective:flow`, `perspective:cross_asset`, or `perspective:sentiment`. The signal sources inside `perspective.Measurements` stay attached as support metadata so settled prediction error can flow back down to every signal that contributed.

Entry selection uses the perspective forecast. The executable quote still comes from the strongest priced measurement inside that perspective.

### 3. Predictions (always, not only on entry)

For every symbol/perspective bucket, `Crypto` calls `price.Prediction.RecordPerspective`, whether or not a trade is entered. That stores an open forecast with:

- **Anchor** — `Last` on the strongest priced measurement in the perspective (or mid of bid/ask if needed)
- **Runway** — the longest horizon implied by the measurements in the perspective
- **Predicted return** — `perspective confidence × return scale` for that `(perspectiveSource, symbol)` series. The signed scale is learned from settled forward returns when available; before that it comes from the symbol’s observed tick-to-tick return movement. No unit fallback is used, and a negative learned scale blocks positive market-move fallback for that perspective-symbol pair.

Until a `(perspectiveSource, symbol)` has settled returns, forecasts use the observed symbol movement scale. Settlement still runs on every anchored open forecast so perspective-local return scale can take over when it has evidence.

### 4. Settlement and feedback

`Prediction.Tick` runs as its own long-lived worker, ingests ticks for **mark prices**, expires forecasts whose `dueAt` has passed, compares **actual** forward return to **predicted**, and broadcasts `PredictionFeedback` on **`feedback`** when the forecast was anchored.

Each entry signal subscribes to `feedback`; `PredictionFeedback.Sources` identifies which signals contributed to the settled perspective. Matching signals route that top-down error into their per-symbol **`PredictionCalibrator`** or `learned.Forecast`, which scales internal parameters and values—not post-hoc confidence cosmetics. When the trader has enough symbol return history, the feedback bucket is the derived market regime (`choppy`, `bullish`, `bearish`, etc.); otherwise it preserves the signal's source-local regime label. Forecasts are observational by default; an active position is bound separately to the perspective source and `dueAt` that authorized entry, so settlement of an unrelated shorter forecast cannot close that position.

Predicted return formula:

```text
predictedReturn = perspectiveConfidence × derivedReturnScale(perspectiveSource, symbol)
```

---

## Registered systems

| System         | Package         | Role                                                             |
|----------------|-----------------|------------------------------------------------------------------|
| Public client  | `kraken/client` | Kraken WS v2: instruments, tickers, trades, book, OHLC           |
| Pump/dump      | `pumpdump`      | Multi-scale volume spikes: 10s fast pump, 5m medium, 14d RVOL slow breakout |
| Depth flow     | `depthflow`     | Distance-weighted book imbalance + Level-1 spoof rejection |
| Hawkes         | `hawkes`        | Bivariate Hawkes trade clustering (buy/sell excitation); MLE refit throttled by `HawkesFitCooldown` |
| Lead/lag       | `leadlag`       | Anchor pair vs laggard path using asynchronous HY correlation     |
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

1. Build perspectives and call `RecordPerspective` once for each symbol/perspective bucket.
2. Track the best net opportunity at the perspective level: `engine.FuseMeasurements` produces perspective confidence, `price.Prediction` produces the perspective forecast, and edge is `perspective predicted return - entry friction`. Every positive-return perspective can support an opportunity immediately; there is no elapsed warmup gate.
3. If wallet and risk capacity remain and net edge is positive after observed fee/spread friction, **enter** on that symbol using the perspective’s lead executable measurement `Last` / `Bid` / `Ask`.
4. Publish per-source **confidence** EMA on `confidence` (gauges) and **`engine_pulse`** on `ui`. **Prediction chart** uses aggregate `engine_pulse.avg_prediction` and `engine_pulse.avg_error`; it does not plot per-symbol forecast segments.

Paper entries use maker limit fills at the bid when `UseMakerEntries` is true (lower `MakerFeePct`); resting bids chase the inner bid up to `MaxEntrySlippageBPS` before abandonment. The paper entry slot is the quote-currency budget: buy fees reduce acquired base, and the wallet does not require extra cash above the reserved slot. Taker fallback uses `SlippageFill`; if visible book depth is insufficient, the remaining notional is priced at an adverse impact level instead of dropping the trade. Live entries post `LimitBuyBid` or `MarketBuyCash` on `orders`; chase re-quotes via cancel/replace. Live maker prices require exchange price precision, and live sells floor base quantity to Kraken lot precision from the instrument snapshot or bound wallet position.

Before entry, fused perspective scoring uses `engine.FuseMeasurements` for source agreement, subtracts fee/spread friction derived from the current quote and entry mode, applies a portfolio risk dampener to executable confidence/edge, applies fractional Kelly sizing from settled feedback and fused confidence, and then runs `PortfolioRisk` gates (one slot per symbol — open inventory or resting maker bid blocks re-entry). The dampener is derived from remaining drawdown capacity and the candidate's systemic covariance eigenmode against open symbols; it leaves the underlying signal forecast recorded while suppressing entries when portfolio state makes the standalone signal unsafe. There is no fixed global slot count; cash, Kelly sizing, optional deploy cap, drawdown, spread/slippage, and correlation decide capacity. Blocked entries emit `entry_blocked`; adverse `depthflow`/`Dump` measurements cancel resting bids.

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

Signal internals and calibration lean on `numeric/` and `numeric/adaptive/` (EMAs, windows, peaks, fences, learned forecast ratios, robust median/MAD scaling)—not magic constants in the trader. Hawkes in particular fits a bivariate self-exciting model via constrained MLE (`hawkes/`), with timelines and decay helpers under `numeric/timeline` and `numeric/decay`. The causal package maps financial observations into indexed DAG nodes before regression, backdoor adjustment, kernels, and counterfactual scoring; the math layer works on node indexes rather than finance-named fields. Cross-asset covariance uses allocation-free Hayashi-Yoshida interval overlap with stale intervals capped to the current microstructure window.

---

## Mental model for operators

- **A running booter should have one long-lived worker per registered system.** If a `Tick` returns unexpectedly, inspect that system's subscriber input and fatal error path.
- **Zero `engine_pulse` measurements** usually means no signal passed `Measure()` yet, or measurements are not reaching `Crypto`'s subscriber.
- **`avg_prediction` and `forecast_symbols` stay at zero** only when no perspective has positive derived return support yet. Observed symbol movement can supply initial scale; settled feedback then replaces it with perspective-local forward-return scale.
- **Gauge confidence on the UI** is the trader’s per-source EMA of `Measurement.Confidence` on the `confidence` channel. **Prediction chart** data comes from `engine_pulse.avg_prediction` and `engine_pulse.avg_error`. The solid green line is the current aggregate prediction, the dashed orange line projects that average one observed pulse interval ahead, and the error line is the running average error.

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
