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

## Architecture

- `kraken/client.PublicClient` — live feed with ping, reconnect, resubscribe, and feed-pause on unrecoverable disconnect
- Observers (`book`, `trades`, `ticker`) → `engine.Signal` scan queues → `trader.Crypto` unified scheduler
- `work.NewPool` — shared qpool for parallel signal measurement drain
- `replay/` — offline JSONL replay through the same client path

## Frontend

```bash
cd frontend && pnpm dev
```

Dashboard connects to `ws://127.0.0.1:8765/ws` (default `config.System.UIAddr`).
