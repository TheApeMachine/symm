# SYMM — Shake Your Money Maker

A Kraken spot microstructure engine. Live market data flows through eleven measurement-emitting signal systems (plus a shared toxicity tracker), each classifying its observation into a semantic category and scoring it against its own noise floor (SNR). The trader holds the latest reading per source per symbol, asks the perspective playbooks whether to enter or exit, and sizes entries against the live cross-section. A paper wallet (€200 default) records fills; point at Kraken WebSocket v2 for live data, or replay a JSONL fixture for offline analysis.

Category semantics and the design rationale behind each signal row live in [`DECISION.md`](DECISION.md).

---

## Contents

- [Architecture](#architecture)
- [The data pipeline](#the-data-pipeline)
- [Everything is a `System`](#everything-is-a-system)
- [Boot sequence](#boot-sequence)
- [Core types](#core-types)
- [Perspectives and playbooks](#perspectives-and-playbooks)
- [Signal systems](#signal-systems)
- [Trader mechanics](#trader-mechanics)
- [Sizing](#sizing)
- [UI and telemetry](#ui-and-telemetry)
- [Numeric layer](#numeric-layer)
- [Build and run](#build-and-run)
- [Configuration reference](#configuration-reference)
- [Repository map](#repository-map)

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Kraken WebSocket v2 (shared feeds in kraken/market)             │
│    trade · ticker · book · instruments · ohlc                    │
└──────────────┬───────────────────────────────────────────────────┘
               │  one upstream per channel, fan-out to all signals
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  11 signals (signal/*) + toxicity (toxicity/ → measurements)    │
│  pumpdump · depthflow · hawkes · leadlag · liquidity             │
│  sentiment · correlation · fluid · causal · cvd · exhaust        │
│  toxicity → shared book-quality service (no measurements)        │
└──────────────┬───────────────────────────────────────────────────┘
               │  perspectives.Measurement on "measurements" bus
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  market — perspective playbooks (decision trees)                 │
│  trend · drive · leadlag · scarcity · pump                       │
└──────────────┬───────────────────────────────────────────────────┘
               │  entry / exit verdicts
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  trader.Crypto                                                   │
│  latest readings per (symbol, source) → Decide → paper fills     │
│  cross-section edge calibration → wallet allocation              │
└──────────────┬──────────────────────┬────────────────────────────┘
               │  ui frames           │  focus.Set (open positions)
               ▼                      ▼
┌───────────────────────────┐ ┌────────────────────────────────────┐
│  view.Gauges · view.OHLC  │ │  ui.Hub → ws://127.0.0.1:8765/ws   │
└───────────────────────────┘ │           → React dashboard        │
                              └────────────────────────────────────┘
```

SYMM is not a single model. It is a **fleet of classifiers** — each signal is a standalone system with its own adaptive machinery — plus a **market layer** that encodes trade theses as decision trees, and a **trader** that turns those theses into wallet events.

---

## The data pipeline

This is the loop to understand. Everything else supports it.

```
Kraken feeds ──► Signals ──► Measurement {Source, Category, SNR, Last}
                                    │
                                    ▼
                         trader: latest reading per source
                                    │
                                    ▼
                         market.Decisions / Decide
                           (perspective trees)
                                    │
                    ┌───────────────┴───────────────┐
                    ▼                               ▼
              flat → consider entry          held → manage exit
                    │                               │
                    └──────── broker.FillPaper ─────┘
                                    │
                                    ▼
                              wallet + ui audit
```

**Three properties of this design:**

1. **Signals never call each other.** They subscribe to shared Kraken feeds and publish to the `measurements` broadcast. Coupling is only through categorized readings on the bus.

2. **Entry and exit are one thesis, re-evaluated.** A flat symbol is offered to the playbooks for `ActionEnter`. A held symbol is offered the same playbooks with `ObservationHolding`, which unlocks stop-loss and take-profit leaves in the tree. The reason the trade opened decides when it closes.

3. **SNR is computed in the signal, not the trader.** Each signal scores its own fused strength against an adaptive noise floor (`numeric/adaptive.SNR`). Perspective branches compare `Measurement.SNR` to a unitless floor (1 = one sigma above the signal's own baseline churn). Thresholds are self-scaling, not hand-tuned prices.

---

## Everything is a `System`

Every runnable unit implements:

```go
type System interface {
    Tick() error
    Close() error
}
```

`Tick` is the long-running event loop. Signals typically `range` over a shared feed channel (`market.NewTradeSubscription`, `NewBookSubscription`, etc.); the trader and view systems `select` on broadcast subscribers and heartbeats.

Registration lives in `cmd/root.go`. Systems communicate only through named broadcast groups on a shared `qpool.Q`. The booter starts `ui.Hub` first, then launches each system's `Tick` in its own goroutine; any fatal `Tick` error cancels the rest.

---

## Boot sequence

```
cmd.Execute()
  └─ rootCmd.Run
       ├─ create qpool (1 producer, NumCPU×4 workers)
       ├─ DiscoverSymbols(QuoteCurrency) → config.System.Symbols
       ├─ focus.NewSet()  (shared open-position symbol set)
       ├─ instantiate all systems
       └─ Booter.Boot()
            ├─ start ui.Hub on config.UIAddr (:8765)
            ├─ ResendWallet() on systems that implement it
            └─ for each System: go Tick(); wait; any error → Close all
```

**System registration order:**

| #  | System      | Package              |
|----|-------------|----------------------|
| 1  | PumpDump    | `signal/pumpdump`    |
| 2  | Correlation | `signal/correlation` |
| 3  | DepthFlow   | `signal/depthflow`   |
| 4  | Hawkes      | `signal/hawkes`      |
| 5  | LeadLag     | `signal/leadlag`     |
| 6  | Liquidity   | `signal/liquidity`   |
| 7  | Sentiment   | `signal/sentiment`   |
| 8  | Fluid       | `signal/fluid`       |
| 9  | Causal      | `signal/causal`      |
| 10 | CVD         | `signal/cvd`         |
| 11 | Toxicity    | `toxicity`           |
| 12 | Exhaust     | `signal/exhaust`     |
| 13 | Crypto      | `trader`             |
| 14 | OHLC view   | `view`               |
| 15 | Gauges view | `view`               |

There is no separate public-client system. Kraken connectivity lives in `kraken/market` as shared, auto-reconnecting feeds multiplexed across every subscriber.

At startup, `market.DiscoverSymbols` replaces the symbol list with every online pair in the configured quote currency (default EUR), so signals watch the full tradable universe rather than a fixed watch list.

---

## Core types

### Measurement

One signal's classified reading on one symbol at one moment.

```go
type Measurement struct {
    Symbol     string
    Source     SourceType   // fluid, hawkes, pumpdump, cvd, toxicity, …
    Category   CategoryType // semantic row from DECISION.md
    Strength   float64      // raw fused strength (dashboard gauges)
    SNR        float64      // playbook score after the signal noise floor
    Last       float64      // last trade price at emit time (for sizing/fill)
}
```

Each signal emits exactly one category at a time. Requiring a category in a perspective tree implicitly excludes that signal's contradicting siblings — a CVD tree demanding `AggressiveDrive` will not see `StochasticBalance` on the same source simultaneously.

Freshness is trader-local: the desk keeps the newest reading per `(symbol, source)` and drops stale slots based on each source's observed inter-arrival cadence.

### Perspective and Decision

A perspective is a **playbook** — a static decision tree over categories and observations.

```go
type Decision struct {
    Name        string          // "trend", "drive", "pump", …
    Action      ActionType      // Enter, StopLoss, TakeProfit, Short
    Perspective Perspective
}
```

`market.Decisions` returns every playbook that authorizes action for the current measurement set. `market.Decide` returns the first actionable verdict in fixed priority order (for callers that need one deterministic answer).

**Actions:** `ActionEnter`, `ActionDeny`, `ActionWait`, `ActionStopLoss`, `ActionTakeProfit`, `ActionShort` (close long on flip cue).

**Entry gates:** every standard playbook walks shared deny branches (toxic bluff, saturation, turbulent chaos, liquidity shock, systemic beta/herd, …). Pump allows `SpoofTrap` entries; trend/leadlag require breadth (`RiskOnSurge`, `DivergentMove`, or `DecoupledAlpha`).

**Exit gates:** entry and exit trees are separate — decayed entry categories do not block exit leaves. `market.ExitDecisions` merges a universal exhaust overlay with the opening playbook's exit tree; the trader picks the most urgent action. `MinExhaustHold` suppresses soft take-profits; `PerspectiveTTL` forces exit when the thesis horizon expires; pump positions also ratchet on `PumpTrailPct` from peak price.

---

## Perspectives and playbooks

Playbooks live in `market/perspectives/` and are registered in priority order in `market/perspective.go`. Order is conviction-first: playbooks that demand more confirming categories before entry sit earlier, so the best-supported thesis wins when several apply.

| Priority | Playbook   | Thesis (summary)                                                                                                                                 |
|----------|------------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| 1        | `trend`    | Breadth + `EndogenousAlpha` + (`Frenzy`/`Laminar`/`Inertial`) + `AggressiveDrive`. Denies manipulation/overheating.                             |
| 2        | `drive`    | `AggressiveDrive` or `HiddenAbsorption` with lighter denies; full exit thesis.                                                                   |
| 3        | `leadlag`  | Breadth + `InefficientLag`; exit on `ActiveReversal`, `AnchorStall`, `SynchronizedDrift`.                                                        |
| 4        | `scarcity` | `ExtremeScarcity` + ignition; exit on reversal, fade, or mechanical collapse.                                                                    |
| 5        | `pump`     | `CoiledCompression` or `SpoofTrap` entry; category exits + trader peak trail (`PumpTrailPct`).                                                   |

**Tree walking:** at each branch, the tree checks whether the measurement set includes the required category with `SNR > 1`. Gates (`snrGate`) require a category but carry no action themselves. The deepest reachable leaf wins — more confirmations mean a more specific verdict. Full category names and signal mappings are in `market/perspectives/category.go` and [`DECISION.md`](DECISION.md).

---

## Signal systems

Each signal follows the same contract:

- Subscribe to the shared Kraken feeds it needs (`trade`, `ticker`, `book`, …)
- Maintain per-symbol state
- Fuse raw metrics through adaptive pipelines (`numeric.Classed`, EMA baselines, sigma clamps)
- Emit `perspectives.Measurement` values on the `measurements` broadcast

Signals classify into four-category families (see DECISION.md for semantics). Summaries:

| Signal          | Package              | Categories (examples)                                                               | Feeds               |
|-----------------|----------------------|-------------------------------------------------------------------------------------|---------------------|
| **PumpDump**    | `signal/pumpdump`    | `vertical_ignition`, `coiled_compression`, `organic_trend`, `faded_exhaustion`      | trade               |
| **DepthFlow**   | `signal/depthflow`   | `loaded_imbalance`, `spoof_trap`, `book_thinning`, `dense_neutrality`               | book                |
| **Hawkes**      | `signal/hawkes`      | `frenzy`, `saturation`, `organic`, `exhaustion`                                     | trade               |
| **LeadLag**     | `signal/leadlag`     | `inefficient_lag`, `synchronized_drift`, `decoupled_move`, `anchor_stall`           | trade, ticker       |
| **Liquidity**   | `signal/liquidity`   | `extreme_scarcity`, `median_depth`, `robust_liquidity`                              | trade               |
| **Sentiment**   | `signal/sentiment`   | `risk_on_surge`, `divergent_move`, `systemic_slump`                                 | trade               |
| **Correlation** | `signal/correlation` | `decoupled_alpha`, `stochastic_noise`, `divergent_stress`, `systemic_herd`          | trade               |
| **Fluid**       | `signal/fluid`       | `laminar`, `turbulent`, `inertial`, `viscous`                                       | book, trade, ticker |
| **Causal**      | `signal/causal`      | `endogenous_alpha`, `systemic_beta`, `liquidity_shock`, `causal_noise`              | trade, book         |
| **CVD**         | `signal/cvd`         | `hidden_absorption`, `aggressive_drive`, `stochastic_balance`, `volume_starvation`  | trade               |
| **Toxicity**    | `toxicity`           | `toxic_bluff`, `liquidity_vacuum`, `hard_support`                                   | book, trade, ticker |
| **Exhaust**     | `signal/exhaust`     | `mechanical_collapse`, `thermal_exhaustion`, `active_reversal`, `fragile_expansion` | book, trade, ticker |

**PumpDump** hunts verticality: volume lift (RVOL) and price precursor off a rolling window, self-scaled against per-symbol EMA baselines, fused and banded into ignition categories.

**DepthFlow** applies distance-decayed book imbalance with anti-spoof filtering; toxic near-touch walls are excluded via the shared `toxicity.Tracker`.

**Toxicity** joins the L2 book and trade tape to split liquidity removals into fills vs cancels, publishes book-quality measurements on the `measurements` bus, and still feeds `toxicity.IsToxic` into depthflow and fluid.

**Hawkes** fits a bivariate self-exciting process on the trade stream; MLE refit is cooldown-throttled per symbol.

**LeadLag** uses BTC/EUR as anchor; measures correlation and unfinished lag fraction for altcoin catch-up.

**Fluid** partitions book depth into a `FluidGridSize × FluidGridSize` grid, tracks field dynamics (Reynolds, divergence, vorticity, turbulence), and also publishes `field_row` frames to the UI bus.

**Causal** implements Pearl's ladder (association → intervention → counterfactual) on a microstructure DAG with Hayashi-Yoshida covariance and regime switching under contagion.

**Exhaust** classifies microstructure decay modes; exit timing is decided by perspective trees (`ActionStopLoss` / `ActionTakeProfit`), not a separate exit channel.

---

## Trader mechanics

`trader.Crypto` is deliberately thin. It does not score signals itself — the perspectives do.

### Measurement ingestion

On each `measurements` message:

1. **Record** the reading in `readings[symbol][source]`, replacing the prior category for that source
2. **Snapshot** non-stale measurements for the symbol
3. **Route** to entry (`consider`) or exit (`manage`) depending on whether the wallet holds the base asset

### Entry path

1. `market.Decisions(measurements, nil)` — collect every playbook authorizing `ActionEnter` (deny/wait omitted)
2. **Thesis score** — RMS of playbook-relevant SNRs, scaled by √confirmations when multiple playbooks agree
3. **Friction gate** — require `thesisScore ≥ EntryEdgeMultiple × round_trip_friction` (fees + slippage)
4. **Playbook economics gate** — `trader/economics` ledger: cold playbooks gather samples; warm playbooks require post-fee net forward/exit returns above `ForwardReturnSignificanceZ` (pump uses `PumpForwardReturnMinSamples`)
5. **Cross-section calibration** — compare the symbol's score to the robust median + MAD of all observed symbols; require positive edge
6. **Size** — allocate cash proportional to edge share of total market score mass
7. **Fill** — `submitEntry` / `submitExit` (paper simulates live: submit, ack, delayed fill); stressed quote when `ExecutionStressEnabled`; economics labels; bind `Playbook` + `PerspectiveTTL`

### Exit path

For held symbols: enforce pump peak trail and `PerspectiveTTL`, then `market.ExitDecisions` with `ObservationHolding` (opening playbook + universal overlay). `MostUrgentExit` chooses stop before take-profit. Soft exits respect `MinExhaustHold`.

### Paper vs live

Paper fills use the same `broker.Quote` path as live: the trader caches Kraken ticker bid/ask and L2 depth per symbol, then `market.SlippageFill` / `SlippagePrice` price the order (VWAP through the book when depth is available, otherwise half-spread on last). `broker.Buy` / `broker.Sell` apply `PreflightGates` (quote freshness, max spread, projected slippage) before reserving cash.

Per-pair taker fees come from Kraken `AssetPairs` at boot (`market.LoadPairCatalog`), tiered by `Fee30DVolume`, and are stored on `PositionBinding.TakerFeePct` for the exit leg. When REST metadata is unavailable, `TakerFeePct` / `WalletEUR` defaults apply.

Gauges display `Measurement.Strength` (raw signal energy). Playbook trees still gate on `SNR` after each signal's adaptive noise floor warms up (~12 samples per symbol).

Live trading: set `SYMM_KRAKEN_API_KEY`, `SYMM_KRAKEN_API_SECRET`, and `SYMM_LIVE=1`. The desk uses `wallet.CryptoWallet`, routes entries/exits through `kraken/order.Client` (authenticated WebSocket v2 + executions channel), and records the same economics labels on exchange fills as paper does on `FillPaper`. Pending entries block duplicate signals until the fill or reject ack arrives.

**Execution economics** (`trader/economics/`): every entry/exit/forward label records post-fee net returns per playbook. Audit frames include `quote_age_ms`, `depth_coverage`, `playbook_econ_mean`, and `forward` events when the forward window matures.

**Paper/live parity:** paper uses the same pipeline as live — `entry_submit` → (optional `PaperOrderLatency`) → fill or `order_reject` ack → `entry`/`exit` on the shared `applyBuyFill` / `applySellFill` wallet path. `PaperOrderRejectRate` simulates exchange rejects; `ExecutionStressEnabled` applies the same quote stress (stale age, shallow depth, adverse ask) to **both** modes. Any new live execution behavior must be mirrored in `trader/paper.go` / `broker.SubmitPaper`.

### Focus set

`focus.Set` tracks symbols with open positions. The trader adds on entry and removes on exit. `view.OHLC` reconciles candle streams against this set so chart data is only published for the anchor symbol (BTC/EUR) and symbols being traded.

---

## Sizing

There is no fixed slot count or Kelly fraction in the current desk. Capital allocation is **edge-proportional across the live cross-section**:

```
thesisScore(symbol) = RMS(playbook-relevant SNR) × √confirmations
edge(symbol)        = thesisScore − median(all scores) − MAD(all scores)
share(symbol)       = edge(symbol) / (thesisScore(symbol) + Σ positive scores)
notional            = free_cash × share(symbol)
```

This lets several symbols enter concurrently when each is a genuine outlier versus the rest of the market, while preventing a single broad signal from consuming the wallet. `MinCostEUR` remains the exchange-cost floor.

---

## UI and telemetry

`ui.Hub` subscribes to the `ui` broadcast and fans out to WebSocket clients at `ws://127.0.0.1:8765/ws`.

**Lossy telemetry ring:** default 512 slots (`UITelemetryBuffer`). Slow clients drop frames rather than back-pressuring producers.

**Audit replay:** the hub keeps a ring of recent audit frames and replays them to newly connected clients so the decision log is not empty after a late connect.

**Producers:**

| Component       | Frames                                          |
|-----------------|-------------------------------------------------|
| `view.Gauges`   | per-source SNR gauge (rate-limited 200 ms)      |
| `view.OHLC`     | `candle_bar` for anchor + open-position symbols |
| `signal/fluid`  | `field_row` for spatial book visualization      |
| `trader.Crypto` | `wallet`, `audit`, fill events                  |
| `ui.Hub`        | `heartbeat` (monotonic seq, queue depth)        |

### UI frame events

| Event        | Source      | Contents                                          |
|--------------|-------------|---------------------------------------------------|
| `confidence` | view.Gauges | per-source SNR gauge                              |
| `wallet`     | Crypto      | balance, inventory, marks                         |
| `audit`      | Crypto      | entry/exit detail, conviction, edge, perspectives |
| `candle_bar` | view.OHLC   | OHLC + volume                                     |
| `field_row`  | Fluid       | book-flow grid row                                |
| `heartbeat`  | Hub         | seq, queue depth, drop count                      |
| fill         | Crypto      | order fill payload                                |

---

## Numeric layer

Signal internals lean on `numeric/` and `numeric/adaptive/` rather than hand-written constants.

### Derived pipeline

`numeric/dynamic.go` chains `Dynamic` filters:

```
EMA → SigmaClamp → Peak → …
```

Each stage calls `Next(out, ...values)` and feeds into the next.

### Adaptive primitives

| Type         | Location                 | Behavior                                                    |
|--------------|--------------------------|-------------------------------------------------------------|
| `EMA`        | `adaptive/ema.go`        | Auto-bootstraps; adaptive rate from observed range          |
| `SigmaClamp` | `adaptive/`              | Kalman-like volatility detector; clamps N-sigma outliers    |
| `SNR`        | `adaptive/snr.go`        | Scores strength vs running noise floor (z-score)            |
| `Classifier` | `adaptive/classifier.go` | Discretizes continuous values into named bands              |
| `FracDiff`   | `adaptive/fracdiff.go`   | Fractional differentiation; preserves memory, reduces AR(1) |
| `Kalman`     | `adaptive/kalman.go`     | Scalar Kalman with asymmetric gain                          |

### Robust statistics

`numeric/` provides `Median`, `Mean`, `PercentileSorted`, `Quartiles`, and `MedianAbsoluteDeviation` — used by the trader's cross-section calibration and throughout signal pipelines.

---

## Build and run

SYMM links against `qpool`, which requires a linkname flag. **Always use the Makefile:**

```bash
make build          # → bin/symm
make run            # build + run (paper defaults)
make test-go        # full test suite with correct ldflags
make bench          # package benchmarks
```

Using bare `go test ./...` without `-ldflags=-checklinkname=0` will fail.

**Replay captured traffic:**

```bash
make replay REPLAY_FILE=replay/fixtures/sample.jsonl REPLAY_PACE=50ms
```

**Frontend (separate terminal):**

```bash
cd frontend && pnpm install && pnpm dev
```

### Environment variables

| Variable                 | Effect                                     |
|--------------------------|--------------------------------------------|
| `SYMM_REPLAY_FILE`       | JSONL replay instead of live WebSocket     |
| `SYMM_REPLAY_PACE`       | Delay between replay lines (e.g., `50ms`)  |
| `SYMM_KRAKEN_API_KEY`    | Kraken API key for live WebSocket v2 orders |
| `SYMM_KRAKEN_API_SECRET` | Base64-encoded API secret                  |
| `SYMM_LIVE`              | `1` or `true` enables live desk + crypto wallet |
| `SYMM_UI_ADDR`           | WebSocket listen address (default `:8765`) |
| `SYMM_WALLET_EUR`        | Starting paper wallet (default `200.0`)    |
| `SYMM_QUOTE_CURRENCY`    | Quote currency (default `EUR`)             |

Full environment wiring is in `config/config.go`.

---

## Configuration reference

<details>
<summary>📋 Wallet and desk</summary>

| Field            | Default | Description                                       |
|------------------|---------|---------------------------------------------------|
| `WalletEUR`      | `200.0` | Paper trading capital                             |
| `MinCostEUR`     | `0.45`  | Minimum trade size (avoids fee domination)        |
| `PerspectiveTTL` | `30s`   | Position binding horizon stamped at entry         |
| `TakerFeePct`    | `0.40`  | Fallback taker fee when pair schedule unavailable |

</details>

<details>
<summary>📋 Market data</summary>

| Field             | Default | Description                          |
|-------------------|---------|--------------------------------------|
| `QuoteCurrency`   | `EUR`   | Universe filter for symbol discovery |
| `BookDepthLevels` | `5`     | Order book snapshot depth            |
| `SubscribeBatch`  | `50`    | Symbol subscribe batch size          |

</details>

<details>
<summary>📋 Signal-specific parameters</summary>

| Field                    | Default | Description                                |
|--------------------------|---------|--------------------------------------------|
| `FastPumpWindow`         | `10s`   | Fast pump detection window                 |
| `MediumPumpWindow`       | `5m`    | Medium pump detection window               |
| `FastPumpVolumeRatio`    | `15`    | Fast pump RVOL threshold                   |
| `SlowRVOLThreshold`      | `5`     | Slow breakout RVOL threshold               |
| `HawkesFitCooldown`      | `5s`    | Hawkes MLE refit minimum interval          |
| `BookDepthDecayLambda`   | `1000`  | Volume weight decay half-life (ms)         |
| `SpoofWeightedThreshold` | `0.5`   | Spoof detection weighted skew threshold    |
| `SpoofLevel1Reject`      | `-0.1`  | Level-1 contradiction threshold            |
| `MinFillToCancelRatio`   | `0.15`  | Toxicity gate threshold                    |
| `BookFluxWindow`         | `10s`   | Book flux measurement window               |
| `FluidGridSize`          | `32`    | Fluid dynamics grid dimension              |
| `FluidHeightEMAAlpha`    | `0.35`  | Field height smoothing                     |
| `CorrelationBarSeconds`  | `10`    | Bar size for correlation computation       |
| `CausalConditionSwitch`  | `1000`  | Kalman-gated Q threshold for regime switch |
| `CausalContagionBreak`   | `0.9`   | Contagion break detection threshold        |
| `FractionalDiffOrder`    | `0.4`   | FracDiff order                             |
| `FractionalDiffWidth`    | `16`    | FracDiff window width                      |

</details>

<details>
<summary>📋 UI and infrastructure</summary>

| Field                 | Default | Description               |
|-----------------------|---------|---------------------------|
| `UIAddr`              | `:8765` | WebSocket listen address  |
| `UITelemetryBuffer`   | `512`   | Lossy telemetry ring size |
| `UIHeartbeatInterval` | `250ms` | Wallet republish cadence  |
| `LogDir`              | `runs`  | Directory for run logs    |
| `LogLevel`            | `info`  | Logging verbosity         |

</details>

Active desk fields include `EntryEdgeMultiple`, `MinExhaustHold`, `PerspectiveTTL`, and pump trail/stop percents. `ForwardReturnMinSamples`, `ForwardReturnSignificanceZ`, and `ExecutionForwardWindow` are wired into the desk economics gate and forward labels. Exploration/Kelly sizing knobs remain unused for slot allocation.

---

## Repository map

| Path                   | Contents                                                     |
|------------------------|--------------------------------------------------------------|
| `cmd/`                 | Cobra entry point, booter, system registration               |
| `market/`              | Perspective registry, `Decide` / `Decisions`, playbook trees |
| `market/perspectives/` | Category types, decision trees, individual playbooks         |
| `signal/`              | All microstructure signal systems                            |
| `toxicity/`            | L3 fill-to-cancel toxicity detector                          |
| `trader/`              | Crypto desk, cross-section sizing, reading freshness         |
| `kraken/`              | WebSocket clients, shared feeds, market types                |
| `broker/`              | Paper and live order execution                               |
| `wallet/`              | Balance, inventory, position bindings                        |
| `view/`                | Dashboard feeds (gauges, OHLC)                               |
| `focus/`               | Open-position symbol set                                     |
| `ui/`                  | WebSocket hub, telemetry ring                                |
| `frontend/`            | React dashboard                                              |
| `numeric/`             | Derived pipelines, adaptive filters, robust stats            |
| `numeric/adaptive/`    | EMA, SNR, classifier, fracdiff, Kalman                       |
| `config/`              | Runtime parameters and environment wiring                    |
| `DECISION.md`          | Category semantics and signal design rationale               |
| `AGENTS.md`            | Agent contract: tests, benchmarks, style                     |

Adding a signal means: implement `Tick`/`Close`, subscribe to the feeds you need, publish `perspectives.Measurement` values with `Source`, `Category`, `SNR`, and `Last` attached, and register the constructor in `cmd/root.go`. Register or extend a perspective tree in `market/perspectives/` when the new categories should authorize trades.
