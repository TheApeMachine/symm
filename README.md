# SYMM — Shake Your Money Maker

Kraken WebSocket v2 microstructure engine: entry signals, an exhaust exit advisor, paper/live trader, and a SciChart dashboard.

## Build and test

Go 1.26+ links `qpool` runtime hooks. Always use Makefile targets (`-checklinkname=0`):

```bash
make build
make test
make bench
```

Plain `go test ./...` fails at link time without that flag.

## Run

```bash
make run
```

Defaults live in `config.NewConfig()`. Logs go to stdout and `runs/symm-<timestamp>.log`.

Paper wallet by default. Live orders when both env vars are set:

```bash
SYMM_KRAKEN_API_KEY=... SYMM_KRAKEN_API_SECRET=... make run
```

Offline replay (JSONL Kraken v2 frames, one object per line):

```bash
make replay REPLAY_FILE=replay/fixtures/sample.jsonl
```

Optional pace: `SYMM_REPLAY_PACE=50ms` (default `50ms`).

Dashboard: `cd frontend && pnpm dev` — WebSocket at `ws://127.0.0.1:8765/ws` (`config.System.UIAddr`).

## Architecture

Every component is an `engine.System` registered in `cmd/root.go` and driven by `cmd/booter.go`:

```go
type System interface {
    Start() error
    State() State
    Tick() error
    Close() error
}
```

Each system uses the same `select`/`default` tick shape. Data moves through `qpool` broadcast groups — no shared mutable bus beyond that.

### Data path

```
Kraken WS v2 (or JSONL replay)
  → client.PublicClient routes frames to tick / trade / book / symbols / ohlc groups
  → signal systems subscribe, update per-symbol state, Measure on idle tick
  → measurements group
  → trader.Crypto
  → feedback group → signal.Feedback (calibrator scale)
  → ui group → ui.Hub → dashboard WebSocket
```

### Entry signals

| Source | Package | Detects |
|--------|---------|---------|
| pumpdump | microstructure | Volume spike vs baseline, book pressure |
| hawkes | microstructure | Bivariate self-exciting trade clustering |
| depthflow | microstructure | Multi-level book imbalance |
| fluid | flow | Book imbalance × trade pressure, spread dampening |
| leadlag | cross-asset | Volume-leader vs laggard |
| sentiment | sentiment | Cross-section bullish breadth |
| causal | sentiment | Intervention/uplift from Pearl-ladder samples |
| liquidity | microstructure | Quote volume below cross-section median |

**exhaust** (exit advisor, not an entry signal) watches book decay and publishes exit urgency on the `exits` group; the trader closes inventory when urgency exceeds `config.System.ExitUrgencyThreshold`.

### Trader loop

On each idle tick, `trader.Crypto`:

1. Settles due predictions → `PredictionFeedback` on the `feedback` group
2. Groups buffered measurements into perspectives
3. Records a prediction per `(symbol, source)` — always, not only on entry
4. After `MinWarmPulses`, enters the best calibrated candidate if `MaxSlots` allows
5. Publishes `engine_pulse`, `scoreboard`, `status`, `signal_score`, `decision_trace`

Predicted return = `confidence × |EWMA(actual forward return)|` once `MinCalibrationSamples` exist per source. Feedback with zero predicted return is dropped (`engine.ValidPredictionFeedback`).

### Signal interface

```go
type Signal interface {
    Source() string
    Measure() iter.Seq[Measurement]
    Feedback(feedback PredictionFeedback)
}
```

Signals emit `engine.Measurement` (confidence, regime, reason, pairs). Expected return and hold horizon are trader-owned.

## Frontend

```bash
cd frontend && pnpm install && pnpm dev
```

SciChart wasm: `pnpm sync:scichart-wasm` on install. Override with `VITE_SCICHART_WASM_BASE` or `VITE_SCICHART_WASM_CDN=true`.
