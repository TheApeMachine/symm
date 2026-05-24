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

## Architecture

- `kraken/client.PublicClient` — live feed with ping, reconnect, resubscribe, and feed-pause on unrecoverable disconnect
- Observers (`book`, `trades`, `ticker`) → `engine.Signal` scan queues → `trader.Crypto` unified scheduler
- `book` retains multi-level depth (default 5 levels for fills) for VWAP slippage and fluid viscosity
- `trader.MarketQuotes` — ticker + book depth for paper fills via `config.SlippageFill`
- `work.NewPool` — shared qpool for parallel signal measurement drain
- `replay/` — offline JSONL replay through the same client path

### Signal engines

- **hawkes** — bound-constrained coordinate-descent MLE with grid fallback; emits `Momentum` (buy cluster) and `Dump` (sell cluster)
- **causal** — gradient-boosted stumps + kernel backdoor regression (non-linear SCM)
- **fluid** — Burgers shock with depth-slope viscosity (spread adjusted by book depth)
- **pumpdump** — overlapping rolling 5-minute volume windows (tick-aligned, not fixed bucket boundaries)

### Execution

- Long and short paper positions with depth-weighted VWAP fills
- Regime-aware min hold: `ScalpHoldBeforeExit` (pump/momentum), `FlowHoldBeforeExit` (flow), `MinHoldBeforeRotate` (default)
- Per-symbol sharded track stores (`engine.ShardedStore` + `SymbolLock`) in all four signal packages

### Telemetry

- Hub replay order matches live publish: `engine_pulse` → `decision_trace` → `scoreboard` → `status`

## Frontend

```bash
cd frontend && pnpm install && pnpm dev
```

SciChart wasm is copied to `frontend/public/scichart/` on install (`pnpm sync:scichart-wasm`). Override with `VITE_SCICHART_WASM_BASE` or set `VITE_SCICHART_WASM_CDN=true` to load from jsDelivr.

Dashboard connects to `ws://127.0.0.1:8765/ws` (default `config.System.UIAddr`).
