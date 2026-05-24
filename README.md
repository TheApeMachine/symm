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

Without API keys the trader stays in paper mode. With keys, entries use market orders with OTO stop-loss-limit; exits use market orders; stop ratchets call `amend_order` when an exchange stop order id is known.

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

1. **Drain tickables** — flush pending book/trade/ticker updates into per-symbol track stores (`engine.WithTickDrain` ensures `Measure` never runs while stores are stale).
2. **Measure signals** — call all eight entry signals sequentially; each yields zero or more `engine.Measurement` values.
3. **Ingest readings** — for every measurement, update per-symbol `PairState`, derive a trader forecast, record an open prediction **anchored at the live quote**, and settle any predictions whose runway has elapsed.
4. **Settle due predictions** — scan all pair states again and emit matured `PredictionFeedback` (also updates `SourceTrustStore`).
5. **Execute** — mark open positions (trailing stops + **exhaust** early exit), merge live score candidates, classify cross-section **regime**, build the weighted ensemble decision, and enter when gates pass.
6. **Publish telemetry** — `engine_pulse`, decision trace, scoreboard, and status frames to the UI hub.

There is no parallel signal measurement inside the trader loop: `processSignals` runs each signal in registration order so track-store drains stay deterministic.

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

When a measurement arrives, `trader.BuildSignalForecast` derives profit expectations from the reading plus the live quote:

- **Expected return** = `confidence × (spreadBPS / 10_000) × ForecastSpreadMultiple` (default 4×). Spread is not double-counted in the edge gate — fills already pay bid/ask via `SlippageFill`.
- **Edge gate** compares expected return to round-trip fees + slippage + `MinEdgeReturn` only.
- **Runway** (hold horizon before the prediction is due) comes from `config.System` by regime:
  - `ScalpHoldBeforeExit` — pump / momentum / dump
  - `FlowHoldBeforeExit` — flow
  - `MinHoldBeforeRotate` — causal and default

Each symbol keeps a `PairState` with:

- the latest signal reading (confidence, regime, reason, type),
- the trader-derived forecast (expected return, runway),
- a slice of open `Prediction` records — one per `(symbol, source)` pair today; the architecture allows more concurrent predictions per symbol as the trader evolves.

Lifecycle for one prediction:

1. **Record** at measurement time with `dueAt = now + runway` and **baseline quote** from the live ticker at record time.
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

The trader also maintains `SourceTrustStore`: per-source hit-rate × magnitude EWMA from settled feedback. `DecisionEngine.Build` multiplies each candidate by regime weight and source trust instead of summing raw confidence equally.

Unanchored or zero predicted-return feedback is dropped — no silent defaults.

### Decision and execution (trader → portfolio)

Forecast feedback and trade entry are separate paths:

- **Candidates** — each measurement also becomes a `SignalCandidate` (symbol, source, confidence, trader expected return, runway, direction).
- **Decision engine** — `ClassifyMarketRegime` gates specialists; `scorePerspectives` + `combinePerspectives` blend the top 1–2 angles; `SourceTrustStore` weights sources from settled accuracy; MAD entry line + post-cost edge gate.
- **Portfolio** — `Portfolio.TryEnter` opens long or short paper positions with depth-weighted VWAP fills (`config.SlippageFill`), regime-aware minimum hold, trailing stops, and **`exhaust` early exit** when book-thinning / pressure-fade urgency exceeds `ExitUrgencyThreshold`.

Warm-up: the first `MinWarmPulses` rescans collect measurements and predictions but suppress entries until gauges and calibrators have context.

### Signal engines

All four share the same contract (`engine.Signal`) and sharded per-symbol track stores (`engine.ShardedStore` + `SymbolLock`), fed by Kraken v2 book, trade, and ticker observers via qpool broadcast groups.

| Engine       | Perspective    | Detects                                                        |
|--------------|----------------|----------------------------------------------------------------|
| **pumpdump** | microstructure | Overlapping 5-minute volume windows vs cross-section median    |
| **hawkes**   | microstructure | Bivariate self-exciting trade clustering                       |
| **depthflow**| microstructure | Multi-level book imbalance at depth                            |
| **fluid**    | flow           | Burgers shock with book-depth viscosity                        |
| **leadlag**  | cross-asset    | Volume-leader move vs laggard catch-up                         |
| **basis**    | cross-asset    | 24h relative strength vs cross-section (spot premium proxy)    |
| **sentiment**| sentiment      | Cross-section pressure + momentum breadth z-scores             |
| **causal**   | sentiment      | Gradient-boosted stumps + kernel backdoor regression           |

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

- Long and short paper positions with depth-weighted VWAP fills
- Regime-aware min hold: `ScalpHoldBeforeExit` (pump/momentum), `FlowHoldBeforeExit` (flow), `MinHoldBeforeRotate` (default)
- Per-symbol sharded track stores in all four signal packages

### Telemetry

- Hub replay order matches live publish: `engine_pulse` → `decision_trace` → `scoreboard` → `status`
- On websocket connect the hub sends `hello`, then the cached dashboard snapshot (`status`, `scoreboard`, `decision_trace`) so the header shows wallet equity immediately
- `PrimeDashboard()` runs before the hub starts accepting clients; every rescore tick refreshes the same frames live
- Frontend opens one WebSocket to `config.System.UIAddr` and routes all event types through a single feed handler

## Frontend

```bash
cd frontend && pnpm install && pnpm dev
```

SciChart wasm is copied to `frontend/public/scichart/` on install (`pnpm sync:scichart-wasm`). Override with `VITE_SCICHART_WASM_BASE` or set `VITE_SCICHART_WASM_CDN=true` to load from jsDelivr.

Dashboard connects to `ws://127.0.0.1:8765/ws` (default `config.System.UIAddr`).
