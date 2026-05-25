# SYMM — Shake Your Money Maker

Kraken WebSocket v2 microstructure engine with eight entry signals across four market perspectives (`pumpdump`, `hawkes`, `fluid`, `causal`, `depthflow`, `leadlag`, `basis`, `sentiment`), an `exhaust` exit advisor, paper trading, JSONL replay, and a SciChart telemetry UI.

## Build and test

Go 1.26+ links `qpool` runtime hooks; use the Makefile targets (they pass `-checklinkname=0`):

```bash
make build
make test
make bench
```

`make test` runs Go tests and the frontend Vitest suite. Plain `go test ./...` fails at link time without that flag.

## Run

```bash
make run
```

No CLI flags — runtime defaults live in `config.NewConfig()`. Logs go to stdout and `runs/symm-<timestamp>.log`.

Replay dry-run (JSONL Kraken v2 frames via environment):

```bash
make replay REPLAY_FILE=replay/fixtures/sample.jsonl
```

Or directly:

```bash
SYMM_REPLAY_FILE=replay/fixtures/sample.jsonl ./bin/symm
```

Optional: `SYMM_REPLAY_PACE=50ms` (default `50ms`).

Live spot trading (WebSocket v2 `add_order` on `wss://ws-auth.kraken.com/v2`):

```bash
SYMM_KRAKEN_API_KEY=... SYMM_KRAKEN_API_SECRET=... make run
```

Without API keys the trader stays in paper mode. Paper and live share proceeds-based taker fees, depth-walk fills with configurable partial-fill coverage (`PaperMinFillCoverage`), optional simulated rejections (`PaperOrderRejectRate`), OTO stop order ids, cash reservation during entry, base-asset inventory, stop-loss-limit stop exits (paper), and stop ratchets via `amend_order` / paper stop amend. With keys, startup runs live reconciliation against Kraken balances and the order journal (`runs/orders.jsonl`), recovers open positions when inventory and journal align, and halts on orphan inventory or missing stop protection; entries wait for the exchange stop order id before committing; live stop fills are polled from the buffer instead of duplicate market exits.

On live startup (skipped during replay), SYMM fetches recent Kraken OHLC candles for the first 64 EUR pairs and seeds volume/return baselines plus calibrator scales before the trader loop. Disable with `SYMM_OHLC_WARM=false`.

Offline calibration report from a replay capture:

```bash
make eval REPLAY_FILE=replay/fixtures/sample.jsonl
make eval REPLAY_FILE=replay/fixtures/sample.jsonl FORMAT=csv
./bin/symm eval --file replay/fixtures/sample.jsonl --format json
```

The report includes per-signal/source calibration, hit rate, error percentiles, and confidence-decile forward returns.

## How the trading algorithm works

SYMM is a closed-loop system: eight entry signals grouped into four **perspectives** (microstructure, flow, cross-asset, sentiment), an **exhaust** exit advisor for open positions, and a trader that **selects angles** rather than summing every source equally.

```
  pumpdump / hawkes / depthflow ── microstructure ──┐
  fluid / depthflow ──────────── flow ──────────────┤
  leadlag / basis ────────────── cross-asset ───────├──► TRADER ──► portfolio ◄── exhaust
  sentiment / causal ─────────── sentiment ─────────┘         │
                                                             │ settled error
                                                             ▼
                                                   SourceTrustStore + calibrators
```

### Rescore tick

`trader.Crypto.Run()` is the single scheduler. Each **rescore pulse** runs in order on the orchestrator thread:

1. **Drain tickables** — flush bounded batches of pending book/trade/ticker updates into per-symbol track stores (`MaxPendingPerSignal`, `MaxPendingGlobal`); `engine.WithTickDrain` ensures `Measure` never runs while configured drain capacity is unused.
2. **Measure signals** — call all eight entry signals sequentially; each yields zero or more `engine.Measurement` values.
3. **Ingest readings** — for every measurement, update per-symbol `PairState`, derive a trader forecast, record an open prediction **anchored at the live quote**, and settle any predictions whose runway has elapsed. If the return model is still cold, record a non-executable calibration probe instead; the probe only seeds empirical forward returns from actual market movement and never becomes an entry candidate.
4. **Settle due predictions** — scan all pair states again and emit matured `PredictionFeedback` (also updates `SourceTrustStore`).
5. **Execute** — mark open positions (trailing stops + **exhaust** early exit), merge live score candidates, classify cross-section **regime**, build the weighted ensemble decision, and enter when gates pass.
6. **Publish telemetry** — `engine_pulse`, decision trace, scoreboard, status, and source-side `candle_bar` frames to the UI hub.

There is no parallel signal measurement inside the trader loop: `processSignals` runs each signal in registration order so track-store drains stay deterministic. Signal-owned symbol scans may still use `qpool` internally in chunks, so market-wide work is parallel without interleaving different signal engines on the orchestrator thread.

### Measurements (signal → trader)

Every signal implements `engine.Signal`:

```go
type Signal interface {
    Source() string
    Measure(ctx context.Context, now time.Time) iter.Seq[Measurement]
    Feedback(feedback PredictionFeedback)
}
```

A `Measurement` is one reading for one or more pairs. It carries **what** the signal saw, not **how much money** to expect:

| Field        | Role                                                                                                 |
|--------------|------------------------------------------------------------------------------------------------------|
| `Type`       | Trade direction hint: `Pump`, `Dump`, `Momentum`, `Flow`, `Causal`                                   |
| `Source`     | Signal name (`pumpdump`, `hawkes`, `fluid`, `causal`) — routes feedback                              |
| `Regime`     | Hold-bucket label (`pump`, `momentum`, `flow`, `causal`, …)                                          |
| `Reason`     | Human/debug tag (`cluster_buy`, `actual_pump`, `shock`, …)                                           |
| `Pairs`      | Affected Kraken wsname symbols                                                                       |
| `Confidence` | Unitless score for ranking and UI gauges — dynamically normalized per symbol, never a magic constant |

Signals do **not** populate expected return or runway. Confidence is derived inside each engine from live cross-section statistics (rolling histories, gauge scans, intervention uplifts, etc.) and is used for relative ranking, not as a post-hoc fudge after feedback.

### Forecasts and predictions (trader-owned)

When a measurement arrives, `trader.BuildSignalForecast` derives profit expectations from settled forward returns (`ReturnModel` EWMA per source/regime) once `MinCalibrationSamples` exist:

- **Expected return** = `confidence × EWMA(actual forward return)` from settled predictions and calibration probes. No executable forecast is emitted until enough samples exist.
- **Edge gate** compares expected return to dynamic costs: round-trip fees + live spread + depth slippage for slot notional + stale-data penalty + `MinEdgeReturn`.
- **Runway** (hold horizon before the prediction is due) comes from `config.System` by regime:
  - `ScalpHoldBeforeExit` — pump / momentum / dump
  - `FlowHoldBeforeExit` — flow
  - `MinHoldBeforeRotate` — causal and default

Each symbol keeps a `PairState` with:

- the latest signal reading (confidence, regime, reason, type),
- the trader-derived forecast (expected return, runway),
- a slice of open `Prediction` records — one per `(symbol, source)` pair today; the architecture allows more concurrent predictions per symbol as the trader evolves.

Lifecycle for one prediction:

1. **Record** at measurement time with `dueAt = now + runway` and **baseline quote** from the live ticker at record time. Cold return models record calibration-only probes with zero predicted return so signal calibrators and trust weights do not consume guessed forecasts.
2. **Settle** when `now ≥ dueAt`: compute `actualReturn` from baseline → exit quote, signed by measurement direction; emit `PredictionFeedback` with `Error = predicted − actual`.
3. **Replace** — a new measurement from the same source on the same symbol replaces the still-open forecast for that source only; matured predictions settle regardless.

`AnchorPending` remains as a fallback when baseline was not set at record time.

### Error feedback (trader → signal)

`PredictionFeedback` is the top-down teaching signal:

```go
type PredictionFeedback struct {
    Source, Symbol, Regime, Reason string
    Type MeasurementType
    Confidence, PredictedReturn, ActualReturn, Error float64
    Runway time.Duration
    SettledAt time.Time
    Unanchored bool   // baseline quote was never attached
}
```

After settlement, `trader.Crypto.applyFeedback`:

1. optionally forwards to a bound sink (eval/replay tooling),
2. calls `Signal.Feedback` on every registered signal; each signal ignores feedback whose `Source` does not match.

Inside each signal, `engine.PredictionCalibrator` maintains an EWMA **scale** from calibration samples. Losses preserve magnitude via `max(0, 1 + actual/predicted)` rather than flat zero. That scale feeds back into the **next** internal fit — Hawkes excitation, pump precursor weights, fluid shock thresholds, causal uplift — not into the confidence number shown on the dashboard.

The trader also maintains `SourceTrustStore`: per-source hit-rate, win magnitude, and loss-severity EWMAs from settled feedback. Unknown sources start at 0.5 trust until samples exist; weight is `hitRate × winMagnitude / (1 + lossSeverity)`. `DecisionEngine.Build` multiplies each candidate by regime weight and source trust instead of summing raw confidence equally.

Unanchored or zero predicted-return feedback is dropped — no silent defaults.

### Decision and execution (trader → portfolio)

Forecast feedback and trade entry are separate paths:

- **Candidates** — each measurement also becomes a `SignalCandidate` (symbol, source, confidence, trader expected return, runway, direction).
- **Decision engine** — stable-universe `ClassifyMarketRegime`; perspective-diverse gating (`ActivePerspectives`, not raw source count); live gauge rows kept separate from executable candidates; MAD entry line + dynamic post-cost edge gate.
- **Portfolio** — spot-long by default (`AllowPaperShorts` / `AllowLiveShorts` off); hard stops bypass min hold; broker I/O outside portfolio lock; `OrderJournal` for live reconciliation; `PortfolioStore` (`runs/portfolio.json`) restores paper positions, cash, and base inventory across restarts; depth-weighted VWAP fills (`config.SlippageFill`); **`exhaust` early exit** when urgency exceeds `ExitUrgencyThreshold`.

Warm-up: the first `MinWarmPulses` rescans collect measurements and predictions but suppress entries until gauges and calibrators have context.

### Signal engines

Each signal is an `engine.System`: **`symbols`** defines the watch list (from UI / SymbolWatch); **`tick`**, **`trade`**, and **`book`** carry market data. One message per `Tick`, score on idle, publish `engine.Measurement` on `measurements`. Per-symbol state is composed `numeric.Derived` chains — no TrackStore.

| Engine       | Perspective    | Detects                                                        |
|--------------|----------------|----------------------------------------------------------------|
| **pumpdump** | microstructure | Volume spike vs EMA baseline, cross-section peak, book pressure |
| **hawkes**   | microstructure | Bivariate self-exciting trade clustering                       |
| **depthflow**| microstructure | Multi-level book imbalance at depth                            |
| **fluid**    | flow           | Burgers shock with book-depth viscosity                        |
| **leadlag**  | cross-asset    | Volume-leader move vs laggard catch-up                         |
| **basis**    | cross-asset    | 24h relative strength vs cross-section (spot premium proxy)    |
| **sentiment**| sentiment      | Cross-section buy-pressure and momentum z-scores (market-internal, not external sentiment) |
| **causal**   | causal         | Bounded intervention/uplift heuristics with kernel backdoor regression           |

**exhaust** (exit advisor) tracks bid/ask depth thinning, spread widening, density collapse, pressure fade, and imbalance reversal on open symbols; closes early when urgency ≥ `ExitUrgencyThreshold`.

### Data path (live)

```
Kraken WS v2
  → book / trades / ticker observers
  → qpool broadcast groups (tick, trade, book, ui)
  → signal track stores (updated during tick drain)
  → Signal.Measure → Measurement
  → trader.Crypto rescore loop
  → PairState predictions → PredictionFeedback → Signal.Feedback
  → decision engine → paper portfolio
  → ui.Hub → dashboard WebSocket
```

`kraken/client.PublicClient` handles ping, reconnect, resubscribe, and feed pause on unrecoverable disconnect. `book` retains multi-level depth (default 5 levels) for VWAP slippage and fluid viscosity. `trader.MarketQuotes` merges ticker last/bid/ask with book depth for fills.
Watched chart symbols are bucketed from executed trade ticks in the Kraken websocket handler and emitted as OHLCV `candle_bar` events on the shared `ui` group; the React chart renders those bars directly.

### Offline replay and eval

Set `SYMM_REPLAY_FILE` to a captured JSONL fixture: frames replay through the same client path at `SYMM_REPLAY_PACE`. The trader loop, feedback loop, and telemetry shape are identical to live — only the feed is synthetic.

`make eval` reports per-signal calibration, hit rate, error percentiles, and confidence-decile forward returns from a replay capture.

## Architecture (components)

- `kraken/client.PublicClient` — live feed with ping, reconnect, resubscribe, and feed-pause on unrecoverable disconnect
- Observers (`book`, `trades`, `ticker`) → `engine.Signal` track stores → `trader.Crypto` unified scheduler
- `book` retains multi-level depth (default 5 levels for fills) for VWAP slippage and fluid viscosity
- `trader.MarketQuotes` — ticker + book depth for paper fills via `config.SlippageFill`
- `work.NewPool` / `qpool.Q` — broadcast groups for market fan-out and UI streaming (not parallel signal measurement)
- `replay/` — offline JSONL replay through the same client path

### Execution defaults

- Spot-long paper/live by default; synthetic shorts require explicit `AllowPaperShorts` / `AllowLiveShorts`
- Safety gates: `MaxLossPerTradeEUR`, `MaxDailyLossEUR`, `MaxSpreadBPS`, `SnapshotFreshnessTTL` (signals + entries)
- Regime-aware min hold: `ScalpHoldBeforeExit` (pump/momentum); hard stops always bypass min hold
- `make test-race` runs `go test -race` with the qpool linkname flag (on macOS, skips `engine/` because the race detector crashes on qpool parallel measure tests)

### Telemetry

- Hub replay order matches live publish: `engine_pulse` → `decision_trace` → `scoreboard` → `status`
- On websocket connect the hub sends `hello`, then the cached dashboard snapshot (`status`, `scoreboard`, `decision_trace`) so the header shows wallet equity immediately
- Chart panels consume OHLCV `candle_bar` frames from the shared `ui` qpool broadcast group. Backend candle production runs synchronously in the Kraken trade websocket handler so React never constructs live candles from ticker snapshots.
- `PrimeDashboard()` runs before the hub starts accepting clients; every rescore tick refreshes the same frames live
- Frontend opens one WebSocket to `config.System.UIAddr` and routes all event types through a single feed handler

## Frontend

```bash
cd frontend && pnpm install && pnpm dev
```

SciChart wasm is copied to `frontend/public/scichart/` on install (`pnpm sync:scichart-wasm`). Override with `VITE_SCICHART_WASM_BASE` or set `VITE_SCICHART_WASM_CDN=true` to load from jsDelivr.

Dashboard connects to `ws://127.0.0.1:8765/ws` (default `config.System.UIAddr`).
