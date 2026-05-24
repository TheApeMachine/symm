# SYMM — Shake Your Money Maker

Kraken WebSocket v2 microstructure engine with four parallel signals (`pumpdump`, `hawkes`, `fluid`, `causal`), paper trading, JSONL replay, and a SciChart telemetry UI.

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

Offline calibration report from a replay capture:

```bash
make eval REPLAY_FILE=replay/fixtures/sample.jsonl
make eval REPLAY_FILE=replay/fixtures/sample.jsonl FORMAT=csv
./bin/symm eval --file replay/fixtures/sample.jsonl --format json
```

The report includes per-signal/source calibration, hit rate, error percentiles, and confidence-decile forward returns.

## How the trading algorithm works

SYMM is a closed-loop system: four parallel signal engines read Kraken microstructure, emit **measurements** upward into the trader, the trader records **predictions** with a hold horizon, and when time catches up it computes **error** and sends that back down to tune each signal’s internal parameters. Signals never decide position size, expected return, or hold time — those belong to the trader.

```
  pumpdump ──measurement──┐
  hawkes   ──measurement──┤
  fluid    ──measurement──├──► TRADER ──► paper portfolio
  causal   ──measurement──┘       │
                                  │ prediction matures
                                  ▼
                            error feedback
                                  │
          ┌───────────────────────┼───────────────────────┐
          ▼                       ▼                       ▼
      pumpdump                 hawkes                   …
   (precursor scale)      (excitation scale)     (per-source calibrators)
```

### Rescore tick

`trader.Crypto.Run()` is the single scheduler. Each **rescore pulse** runs in order on the orchestrator thread:

1. **Drain tickables** — flush pending book/trade/ticker updates into per-symbol track stores (`engine.WithTickDrain` ensures `Measure` never runs while stores are stale).
2. **Measure signals** — call `Signal.Measure` on `pumpdump`, `hawkes`, `fluid`, and `causal` sequentially; each yields zero or more `engine.Measurement` values.
3. **Ingest readings** — for every measurement, update per-symbol `PairState`, derive a trader forecast, record an open prediction, anchor it to the live quote, and settle any predictions whose runway has elapsed.
4. **Settle due predictions** — scan all pair states again and emit matured `PredictionFeedback`.
5. **Execute** — mark open positions, merge live score candidates, build the cross-symbol decision, and enter/exit paper trades when gates pass.
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

- **Expected return** = `confidence × (spreadBPS / 10_000)` using the current bid/ask.
- **Runway** (hold horizon before the prediction is due) comes from `config.System` by regime:
  - `ScalpHoldBeforeExit` — pump / momentum / dump
  - `FlowHoldBeforeExit` — flow
  - `MinHoldBeforeRotate` — causal and default

Each symbol keeps a `PairState` with:

- the latest signal reading (confidence, regime, reason, type),
- the trader-derived forecast (expected return, runway),
- a slice of open `Prediction` records — one per `(symbol, source)` pair today; the architecture allows more concurrent predictions per symbol as the trader evolves.

Lifecycle for one prediction:

1. **Record** at measurement time with `dueAt = now + runway`.
2. **Anchor** baseline quote from the live ticker (required for signed actual return).
3. **Settle** when `now ≥ dueAt`: compute `actualReturn` from baseline → exit quote, signed by measurement direction; emit `PredictionFeedback` with `Error = predicted − actual`.
4. **Replace** — a new measurement from the same source on the same symbol replaces the still-open forecast for that source only; matured predictions settle regardless.

The prediction nodes in the architecture diagram are not a fixed pair — they stand for however many open forecasts the trader is tracking at once (per symbol, per source, and potentially more shapes later). Each stores “I expect X return over Y seconds from this reading,” then checks reality when Y elapses.

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

Inside each signal, `engine.PredictionCalibrator` maintains an EWMA **scale** from `actualReturn / predictedReturn` samples. That scale feeds back into the **next** internal fit — Hawkes excitation, pump precursor weights, fluid shock thresholds, causal uplift — not into the confidence number shown on the dashboard. Confidence stays a live market-relative score; calibration tunes the physics parameters that produce it.

Unanchored or zero predicted-return feedback is dropped — no silent defaults.

### Decision and execution (trader → portfolio)

Forecast feedback and trade entry are separate paths:

- **Candidates** — each measurement also becomes a `SignalCandidate` (symbol, source, confidence, trader expected return, runway, direction).
- **Decision engine** — `DecisionEngine.Build` combines candidates per symbol (support count, MAD-scored line, warming gate), producing `Evaluation` rows with `Allow` / `Why`.
- **Portfolio** — `Portfolio.TryEnter` opens long or short paper positions with depth-weighted VWAP fills (`config.SlippageFill`), regime-aware minimum hold, trailing stops, and fee accounting.

Warm-up: the first `MinWarmPulses` rescans collect measurements and predictions but suppress entries until gauges and calibrators have context.

### Signal engines

All four share the same contract (`engine.Signal`) and sharded per-symbol track stores (`engine.ShardedStore` + `SymbolLock`), fed by Kraken v2 book, trade, and ticker observers via qpool broadcast groups.

| Engine       | Detects                                                        | Emits                                            |
|--------------|----------------------------------------------------------------|--------------------------------------------------|
| **pumpdump** | Overlapping 5-minute volume windows vs cross-section median    | `Pump` when precursor volume spikes align        |
| **hawkes**   | Bivariate self-exciting trade clustering (MLE + grid fallback) | `Momentum` (buy cluster) / `Dump` (sell cluster) |
| **fluid**    | Burgers shock with book-depth viscosity                        | `Flow` on spread/imbalance shocks                |
| **causal**   | Gradient-boosted stumps + kernel backdoor regression           | `Causal` when intervention uplift exceeds fence  |

Each package owns its feature extraction, confidence normalization (`GaugeScan` peak across the symbol set), and `ApplyPredictionFeedback` hook that maps error into its internal parameters.

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
- Frontend opens one WebSocket to `config.System.UIAddr` and routes all event types through a single feed handler

## Frontend

```bash
cd frontend && pnpm install && pnpm dev
```

SciChart wasm is copied to `frontend/public/scichart/` on install (`pnpm sync:scichart-wasm`). Override with `VITE_SCICHART_WASM_BASE` or set `VITE_SCICHART_WASM_CDN=true` to load from jsDelivr.

Dashboard connects to `ws://127.0.0.1:8765/ws` (default `config.System.UIAddr`).
