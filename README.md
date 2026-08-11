![Header image of S.Y.M.M.](header.png)

# S.Y.M.M. — Shake Your Money Maker

S.Y.M.M. is an experimental Kraken Spot market-research and trading system. It conditions ticker, trade, and Level 3 order-book data into typed numerical measurements; assembles those measurements into a per-symbol thesis; derives predictive, causal, physical, cognitive, and graph evidence; and admits only executable decisions to a broker-owned position lifecycle.

Paper execution is the configured default. This project is not financial advice, and its live path should be treated as experimental software controlling real funds.

The current runtime is thesis-driven. The DMT tree is used by the cognition solver; it is not the market-data bus, and there is no YAML playbook decision engine in the current code.

## Contents

- [Architecture](#architecture)
- [Runtime data flow](#runtime-data-flow)
- [Signals](#signals)
- [Logic and strategy](#logic-and-strategy)
- [Execution and recovery](#execution-and-recovery)
- [Dashboard](#dashboard)
- [Configuration](#configuration)
- [Build and run](#build-and-run)
- [Tests and benchmarks](#tests-and-benchmarks)
- [Repository map](#repository-map)
- [Design references](#design-references)

## Architecture

```text
Kraken WebSocket v2 / REST
        │
        ├── instrument, ticker, trade, L2 and authenticated L3
        ▼
kraken/websocket.API
        │
        ▼
trader.Crypto ── append market observations ──► types.Thesis
                                                    │
                           ticker/trade/book fanout │
                                                    ▼
                         ten independent signal conditioners
                                                    │
                                    typed numerical Measurements
                                                    ▼
                                            logic.Analyzer
                     category · resonance · manifold · causal · cognition · graph
                                                    │
                              per-symbol readiness + final evidence graph
                                                    ▼
                                           strategy.Planner
                          forecast gate · causal MCTS · economics · sizing
                                                    │
                                     enter / nothing Decision
                                                    ▼
                                             broker.Desk
                          order lifecycle · stoploss · recovery · accounting
                                                    │
                   ┌────────────────────────────────┴──────────────────────┐
                   ▼                                                       ▼
           paper or Kraken orders                               audit + dashboard
```

The central coordination object is [`types.Thesis`](types/thesis.go). It owns the current market observations, measurements, per-symbol analysis products, decisions, account equity, lifecycle state, and a non-blocking semaphore fanout between runtime stages.

Each symbol has an explicit [`types.Readiness`](types/readiness.go) record. A stage stamps a symbol after evaluating it, downstream stages check their prerequisites, and the symbol is reset only after the trader has consumed its completed decision. Missing evidence remains missing; it is not silently replaced with a successful default.

## Runtime data flow

### Boot

[`cmd.Boot`](cmd/boot.go) assembles the system in dependency order:

1. Rotate and open `runtime-audit.jsonl` when configured.
2. Open the SQLite position store at `<system.data_path>/symm.sqlite`.
3. Open or accept injected public and private Kraken connections.
4. Construct the transport API, price cache, instrument universe, balance, and desk.
5. Recover exchange inventory, working orders, and stored stop state.
6. Construct the shared DMT cognition tree and the market-data coordinator.
7. Start the ten signal workers.
8. Start the logic analyzer, strategy planner, and global regulator.
9. Serve the dashboard WebSocket and manifold WebRTC endpoint.

Production constructors report readiness through [`utils.Waiter`](utils/waiter.go), so dependent components are not built before their upstream services report `READY`.

### Market ingress

[`broker.Instrument`](broker/instrument.go) discovers every online pair for the configured quote currency, loads fee metadata, and subscribes in batches to trades, L2 books, tickers, and authenticated L3 books.

[`trader.Crypto`](trader/crypto.go) is the single market-ingress coordinator:

- ticker frames update the thesis and the broker price cache;
- trade frames append to the thesis trade tape;
- applied L3 updates publish the affected symbol;
- each input wakes only the signal stages that consume it.

Signals retain their own incremental estimator state, but the observations for the current evaluation epoch live in the shared thesis.

### Measurements

A [`types.Measurement`](types/measurement.go) is a source-and-symbol observation with explicit provenance:

```go
type Measurement struct {
    ID               string
    Source           SourceType
    Symbol           string
    Peer             string
    At               time.Time
    ObservedFrom     time.Time
    Horizon          time.Duration
    PeerAt           time.Time
    PeerObservedFrom time.Time
    Maturity         float64
    Uncertainty      *MeasurementUncertainty
    Metrics          map[string]MetricSample
}
```

Metrics preserve raw value, optional normalized value, and unit. A signal reports numerical observations; it does not choose a market category or trading action.

## Signals

Signals subscribe to thesis wake-ups, consume only new observations for their source, publish measurements, and stamp every symbol they evaluated. Their estimators use observed data to derive windows and scales rather than sharing fixed market horizons.

| Signal | Primary inputs | Responsibility |
|---|---|---|
| `correlation` | ticker | Asynchronous cross-symbol co-movement and cohort divergence using Hayashi–Yoshida relationships. |
| `cvd` | ticker, trade | Signed aggressor flow against midpoint response, separating drive from absorption. |
| `depthflow` | trade, L3 | Touch and depth-weighted imbalance, thinning, pressure, and toxic depth. |
| `exhaust` | trade, L3 | Side-specific decay in the microstructure that would support a position. |
| `hawkes` | trade | Bivariate buy/sell arrival intensity, excitation, stability, and fitted process parameters. |
| `leadlag` | ticker | Asynchronous leader/follower timing, lag, synchronization, and anchor state. |
| `liquidity` | ticker | Executable touch depth and reported turnover relative to the current cohort. |
| `pumpdump` | trade, L3 snapshot | Trade lift and price movement conditioned by current midpoint and spread structure. |
| `sentiment` | ticker | Cross-sectional breadth, leadership, and peer divergence. |
| `toxicity` | ticker, trade, L3 | Touch cancellation, retreat, replenishment, and execution asymmetry. |

Detailed mathematical notes live beside each implementation under `signal/<name>/README.md`. `signal/compute` is shared Metal-initialization infrastructure, not a market signal.

## Logic and strategy

### Analyzer

[`logic.Analyzer`](logic/analyzer.go) runs six solvers concurrently. Solvers that do not yet have their prerequisites return without inventing output; the analyzer repeats while readiness advances and stops if the manifold explicitly needs a newer book or no solver can progress.

| Solver | Output |
|---|---|
| `category` | Cross-signal hypotheses derived from metric support, opposition, and missing evidence. |
| `resonance` | Per-symbol predictive-coding state and an online return forecast with a Student-t predictive distribution. |
| `manifold` | A physical readout of the visible L3 order population, conditioned by Hawkes excitation. |
| `causal` | Historical causal rows, treatment level, sample support, and estimate precision. |
| `cognition` | DMT-backed episodic sequences and cognitive paths. |
| `graph` | The final evidence graph, including support, contradiction, lead/lag, and temporal relationships. |

The graph stage waits for category, resonance, causal, cognition, and—when Hawkes evidence is present—manifold completion. It is the final structural product of the logic layer.

### Forecasts

[`types.ResonanceForecast`](types/resonance.go) carries a retention-supported forward log-return curve, expected simple return, posterior directional confidence, predictive scale, degrees of freedom, and supported horizon.

The resonance learner is prequential: it scores a prior prediction against a later resolved return before using that return to update the model. Current confidence and historical evidence that the learner beats a zero-return baseline remain separate quantities.

### Decisions

[`strategy.Planner`](strategy/planner.go) considers a symbol only after its signal and logic stages are complete. The entry path is:

1. Require a current resonance forecast and matching graph evidence.
2. Require a ready predictive distribution above the configured confidence gate.
3. Compare entry against standing aside with causal MCTS.
4. Convert the chosen candidate into an executable allocation.
5. Recheck bid, ask, pair increments, fees, spread, and expected impact.
6. Reject a forecast that does not clear current round-trip friction.
7. Construct forecast-derived stop geometry and publish the decision.

Public decision actions are `enter`, `exit`, `hold`, and `nothing`. The current planner emits entry or standing-aside decisions. The desk rejects strategy-driven exits; an open lot exits only when its bound stoploss triggers.

## Execution and recovery

[`broker.Desk`](broker/desk.go) owns positions and serializes exchange-facing state. A planned entry is placed into the desk before network submission so concurrent decisions cannot overbook the same capacity.

For every entry, the desk:

- verifies the forecast and proposed quantity;
- re-evaluates entry economics against the latest executable prices;
- reserves a normal or independently qualified position slot;
- submits a market order with the decision UUID as client order ID;
- updates the lot from authenticated execution frames;
- persists stoploss state in SQLite;
- marks and regulates the position from ticker updates;
- submits the sell only after the stoploss reaches `TRIGGERED`.

On boot, [`broker.Recovery`](broker/recovery.go) reconciles exchange balances, trade history, working orders, and stored stop state. The exchange wallet is authoritative: balance snapshots replace the local wallet rather than merging into it.

### Paper and real execution

`trading.model: paper` routes balances, fills, history, and orders through the native `kraken paper` CLI while public market data continues to come from Kraken. The paper ledger is external to this process; it is not an in-memory fake exchange.

`trading.model: real` routes account operations and orders to Kraken. Authenticated transports read:

| Environment variable | Purpose |
|---|---|
| `KRAKEN_API_KEY` | Kraken API public key. |
| `KRAKEN_API_SECRET` | Kraken API secret consumed by the SDK. |
| `SYMM_PPROF` | Enables the pprof listener even when config disables it. |

There is no automatic `SYMM_*` mapping for Viper configuration in the current loader. Use a YAML configuration file for other settings.

## Dashboard

[`ui.Hub`](ui/hub.go) exposes:

- `ws://127.0.0.1:8765/ws` for JSON state and telemetry;
- `http://127.0.0.1:8765/webrtc/manifold` for non-trickle WebRTC signaling;
- WebRTC data channels for binary manifold field and particle frames.

The frontend is a React 19 and TanStack Start terminal. Its surfaces include the dashboard, global regulator, evidence graph, fluid manifold, signal inspection, decisions, trade journal, latent-state x-ray, cognitive tree, and allocation view. The browser sends the selected focus symbol back to the backend so detailed telemetry can be gated to the active market.

The WebSocket URL defaults to `ws://127.0.0.1:8765/ws` and can be changed at frontend build time with `VITE_SYMM_WS_URL`.

## Configuration

The Cobra entrypoint loads configuration in this order:

1. `--config <path>` when explicitly supplied; failure is fatal.
2. `cmd/cfg/config.yml`.
3. `./config.yml`.
4. `$HOME/.symm/config.yml`.
5. The copy of `cmd/cfg/config.yml` embedded in the binary.

The checked-in configuration defaults to:

- data under `~/.symm/data`;
- audit rotation on boot;
- paper execution;
- USD quote-market discovery;
- authenticated L3 depth alongside ticker, trade, and L2 subscriptions;
- two normal and two reserved position slots;
- a predictive-confidence entry gate and a maximum per-entry cash fraction;
- an in-process cognition tree;
- pprof disabled unless enabled by config or `SYMM_PPROF`.

See [`cmd/cfg/config.yml`](cmd/cfg/config.yml) for the complete checked-in configuration. Some adaptive strategy values are held in [`system.Config`](system/config.go) and can be updated at runtime by the equity-driven global regulator.

Runtime files under `system.data_path` include:

| File | Purpose |
|---|---|
| `runtime-audit.jsonl` | Orchestration, evidence, decision, and execution audit records. |
| `symm.sqlite` | Persisted per-symbol stoploss state used during recovery. |
| authenticated nonce state | Monotonic Kraken nonce continuity across restarts. |

## Build and run

### Prerequisites

- Go 1.26.1.
- pnpm 10.28.1 for the frontend.
- Local sibling checkouts of `../datura` and `../nomagique`, as required by `go.mod` replacements.
- A supported Metal environment for the GPU-backed analysis path.
- The native `kraken` CLI for paper account and execution operations.
- Kraken credentials for authenticated private and Level 3 connections.

`qpool`, reached through DMT, currently uses `go:linkname` runtime hooks. The Makefile exports the required linker setting:

```text
GOFLAGS=-ldflags=-checklinkname=0
```

Use the Makefile so the setting reaches nested Go and cgo subprocesses:

```bash
make build
make run
```

`make build` writes the race-enabled binary to `bin/symm`. `make run` starts the backend and blocks in the dashboard hub.

Run the frontend separately:

```bash
cd frontend
pnpm install
pnpm dev
```

The frontend development server listens on port 3000 by default.

To use a configuration outside the normal search path:

```bash
make build
./bin/symm --config /absolute/path/to/config.yml
```

Enable profiling with `system.pprof.enabled: true` or `SYMM_PPROF=1`, then open `http://127.0.0.1:6060/debug/pprof/`. `make run-profile`, `make profile`, and `make profile-report` wrap the common workflow.

## Tests and benchmarks

The Go suite uses GoConvey and includes package tests, concurrency checks, calculation benchmarks, transport fixtures, and a deterministic multi-symbol market simulator. [`tests.Market`](tests/market.go) drives production websocket parsing with coherent ticker, trade, L2, L3, candle, execution, balance, latency, fault, and reconnect behavior.

Metal pipeline creation is process-global, so Go package test binaries run serially with `-p 1`.

```bash
make test-go          # Go tests
make test-race        # Go race suite
make test-cover       # Go coverage → runs/coverage.out
make bench            # Go benchmarks with allocations
make test             # Go tests + race suite + frontend production build
```

Frontend verification is explicit:

```bash
cd frontend
pnpm test             # Vitest
pnpm typecheck        # TypeScript
pnpm lint             # Biome
pnpm build            # production build
pnpm bench            # Vitest benchmarks
```

`make test-frontend` runs `pnpm build`; it does not run Vitest.

## Repository map

| Path | Responsibility |
|---|---|
| `main.go`, `cmd/` | Cobra entrypoint, configuration loading, and system assembly. |
| `kraken/` | Kraken wire models and normalized exchange payloads. |
| `kraken/websocket/` | Live public/private/L3 transport, subscriptions, nonce management, and paper routing. |
| `types/` | Thesis, measurements, readiness, evidence, forecasts, decisions, holdings, and stoploss types. |
| `signal/` | Numerical market conditioners and their per-source estimator state. |
| `logic/` | Category, resonance, manifold, causal, cognition, graph, and analyzer stages. |
| `strategy/` | Forecast admission, graph evidence, causal MCTS, and allocation. |
| `broker/` | Instruments, price and fee economics, wallet, desk, orders, positions, persistence, and recovery. |
| `trader/` | Raw market ingress and completed-decision handoff. |
| `regulator/` | Equity-driven global predictive regulator. |
| `audit/` | JSONL recording and boot rotation. |
| `ui/` | Dashboard WebSocket, WebRTC manifold transport, and error bridge. |
| `frontend/` | React/TanStack terminal and WebSocket-backed stores. |
| `tests/` | Deterministic venue, market scenarios, fixtures, execution model, and stack harness. |
| `system/` | Runtime-tunable analysis, risk, and planner configuration. |
| `utils/` | Small shared transport, JSON, math, path, publish, and readiness helpers. |
| `specs/` | Design contracts, research notes, reviews, and simulator specifications. |

### Adding or changing a signal

1. Emit dimensional numerical metrics with correct timestamps, units, maturity, and uncertainty.
2. Keep market categories and actions out of the signal package.
3. Subscribe the signal to the thesis and expose `Name` and `Close`.
4. Register its source, readiness stamp, and ticker/trade/book receiver wiring.
5. Update category and graph composition only where the new metric has documented semantic meaning.
6. Cover multi-step behavior and invalid input with mirrored GoConvey tests.
7. Add and run a benchmark when the change affects repeated calculation or data processing.

## Design references

- [`AGENTS.md`](AGENTS.md) — implementation, safety, testing, and architecture rules.
- [`specs/thesis.md`](specs/thesis.md) — measurement, evidence, lifecycle, and thesis ownership contract.
- [`specs/manifold.md`](specs/manifold.md) — L3 physical-model contract and validation sequence.
- [`specs/pnl.md`](specs/pnl.md) — execution precision, fees, and PnL design.
- [`specs/market-simulator.md`](specs/market-simulator.md) — deterministic venue model.
- [`specs/test.md`](specs/test.md) — testing direction.
- Per-package READMEs under `signal/` and `logic/` — focused mathematical and semantic notes.

## Terminal

![Image of S.Y.M.M. Terminal](terminal1.png)

![Image of S.Y.M.M. Terminal](terminal2.png)

![Image of S.Y.M.M. Terminal](terminal3.png)

![Image of S.Y.M.M. Terminal](terminal4.png)

![Image of S.Y.M.M. Terminal](terminal5.png)

![Image of S.Y.M.M. Terminal](terminal6.png)
