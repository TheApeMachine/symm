# SYMM — Shake Your Money Maker

Kraken WebSocket v2 microstructure engine with four parallel signals (`pumpdump`, `hawkes`, `fluid`, `causal`), paper trading, JSONL replay, and a SciChart telemetry UI.

## Build and test

Go 1.26+ links `qpool` runtime hooks; use the Makefile targets (they pass `-checklinkname=0`):

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

Replay dry-run:

```bash
make replay REPLAY_FILE=replay/fixtures/sample.jsonl
```

## Flags

| Flag | Purpose |
|------|---------|
| `--wallet` | Paper wallet size in quote currency |
| `--quote` | Quote filter (default EUR) |
| `--ui-addr` | Telemetry WebSocket server (`:8765`) |
| `--replay-file` | JSONL Kraken v2 frames |
| `--log-level` | errnie log level |
| `--log-dir` / `--log-file` | Run log output |

## Architecture

- `kraken/client.PublicClient` — live feed with ping, reconnect, and resubscribe
- Observers (`book`, `trades`, `ticker`) → `engine.Signal` → `trader.Crypto`
- `work.NewPool` — shared qpool for parallel signal measurement drain
- `replay/` — offline JSONL replay through the same client path

## Frontend

```bash
cd frontend && pnpm dev
```

Dashboard connects to `ws://127.0.0.1:8765/ws` when `--ui-addr :8765` is set.
