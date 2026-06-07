# SYMM — End-to-End System Review

**Date:** 2026-06-07 · **Scope:** full repository at HEAD (`90a66a29`) plus working-tree state, and the live data in `./runs`.

**Method.** Every claim below was derived from the code as it exists right now and from the actual run data, not from README/AGENTS/DECISION commentary. I read the full money path personally (cmd, market/story, trader, broker, execution, kraken/paper, optimizer/replay, optimizer/reasoning, perspectives/reasoning, the qpool v1.2.4 sources as pinned in go.mod), used search agents for breadth on the remaining ~70k lines and then re-verified every claim of theirs that appears here (several of their "critical" findings were wrong and are *not* in this report — e.g. "double confidence stabilization" and "causal look-ahead" do not survive a careful read of `surprise.go` and `causal/pending.go`). I profiled `runs/capture.jsonl` (17.65M rows), `runs/audit.jsonl(.1)` (144,775 frames), the raw signal dumps, and `runs/network_latency.json`. I also built the entire module with Go 1.26.3 against the **published** `qpool v1.2.4` / `errnie v1.2.3` (the `replace` directives point at sibling directories that don't ship with the repo) — the build passes; `broker`'s own test suite does not (§1.3).

**Epistemic note.** The one thing I could not inspect is your local `../qpool` and `../errnie` (they are outside this repo). Where behavior depends on them I say so explicitly and back the claim with run-data evidence instead.

---

## 0. Executive summary

The architecture is genuinely good in outline: one shared economic model (broker fills, preflight, sizing, instrument rules) used by live, paper, and replay; a typed reasoning language for playbooks; a self-describing measurement stream; an audit trail; raw signal dumps with a certification harness. The discipline of "replay must call the same code as the desk" (EXECUTION.md) is the right instinct and is mostly honored in structure.

The execution of that vision currently fails in five load-bearing places, and the run data proves three of them are not theoretical:

1. **The system is trading blind right now (P0).** Commit `e26ef63b` (Jun 7, 03:10) switched `broker.QuoteCache` and `broker.InstrumentRulesCache` from the pool's named-group registry to `qpool.NewBroadcastGroup`, which (in qpool as pinned) creates a **free-standing group nothing publishes to**. Since that run started: zero bid/ask/book in all 17.65M captured rows, zero audit frames since 22:57 yesterday, every order would be rejected "no quote". The engine kept running for ~an hour producing a 4.3 GB tape that the optimizer cannot trade on (replay entries require book depth). §1.

2. **Both paper and replay fills systematically erase the spread (P0).** With execution stress enabled (the default), the fill price is recomputed from `quote.Last` instead of the touch or the book-walk VWAP: buys can fill at, or even below, the bid. On the wide-spread microcaps this playbook explicitly targets (`max_spread_bps: 120`), that under-charges up to ~1.2% per round trip — far larger than the fees being modeled to two decimal places. Every PnL number the optimizer maximizes and the paper wallet reports inherits this. §2.1.

3. **The optimizer is pure in-sample selection (P0).** `tune` derives a vocabulary from the tape, beam-searches forests, scores every candidate on the *same* tape, and writes the best scorer to the live playbook. There is no holdout, no walk-forward, no embargo — a complete walk-forward helper exists in `replay/walkforward.go` and is never called. The min-trades discount caps at 12 closed trades. This maximizes selection bias almost by construction. §3.1.

4. **SNR — the primary gate your playbooks tune on — is two different quantities glued together (P1).** For the first 12 observations of a tracker it is `surprisal/0.1` (values 15–20, confirmed in the tape); after warmup it converges to ≈0.5047 for any stable regime (34.8% of all 17.65M rows carry that *exact* constant). "snr ≥ 1" therefore mostly means "tracker is cold or category just flipped", which is not what a tuned threshold appears to mean. §4.1.

5. **The audit trail cannot account for the system's round trips (P1).** Over a full trading day: 144,241 playbook walks → 534 trade decisions → 8 fills → **0 position outcomes**. Protective/exchange-side closes never write `position_outcome` (the code path skips it), the audit writer fail-stops permanently on the first I/O error, and rotation keeps only ~100 MB of history. §5.3.

Beyond these, there is a long tail of real but smaller issues (stuck in-flight symbols when a fill frame is dropped; a dead circuit breaker; ws reads with no deadline; per-measurement decision-tree broadcasting; config/Makefile drift; repo hygiene). They are catalogued with fixes below.

The structural fixes are not large. The bus regression is a one-line revert per call site plus a watchdog; the fill anchoring is one function; walk-forward is plumbing you already wrote. The most important cultural fix is §7: this system has no mechanism that notices when a critical input goes silent — every one of the data-layer failures above was *silent*.

---

## 1. P0 incident: the quote/rules caches are disconnected from the bus

### 1.1 The evidence chain (all from this repo and ./runs)

- `broker/quote.go:119` — `qpool.NewBroadcastGroup(cache.ctx, "raw", 10*time.Millisecond)`; `broker/instrument_rules.go:53` — same with TTL 0. Everything else in production uses `pool.CreateBroadcastGroup(...)`.
- In qpool **as pinned** (`v1.2.4`, module cache): `NewBroadcastGroup` constructs a group and returns it — it never touches the `GroupRegistry`. Only `QSpace.CreateBroadcastGroup` does get-or-create in the registry (`qspace.go:207`, `group_registry.go`). A subscriber on a fresh group receives only what is sent to *that object*. Nothing ever sends to the quote cache's private group.
- `git log`: commit `e26ef63b` ("Refactor bus usage to qpool broadcast groups", **Sun Jun 7 03:10**) replaced the previous `bus.Group(pool, "raw", …)` with `qpool.NewBroadcastGroup` in exactly these files.
- Run data, yesterday (binary predating the commit): `audit.jsonl.1` spans 08:33→22:57 with quote-dependent evidence everywhere — "stale quote" rejections, spread values computed from real bid/ask ("spread 1201.50 bps exceeds limit"), 8 fills.
- Run data, today (process started 05:46, after the commit): `capture.jsonl` grew to 17.65M rows in under an hour and contains **zero** occurrences of `"bid":`, `"book_bids":` — the book enricher (`broker.MeasurementBookEnricher` → `quotes.Snapshot`) found nothing for any symbol, ever. `audit.jsonl` has not received a single frame since 22:57 (the `quoteReady` gate in `market/story.go:283` blocks every action when `Quotes.Snapshot` is empty). Meanwhile `depthflow_raw.jsonl` (a signal subscribed through `pool.CreateBroadcastGroup`) was happily computing `spread_bps: 13.4` from live book frames at 06:42 — the bus itself is fine; only the `NewBroadcastGroup` consumers starve.
- Independent confirmation: `go test ./broker/` against the pinned qpool fails `TestStressCacheBroadcast` ("Expected toxic_bluff / Actual ''") — a cache subscribing one way never sees what the test publishes the other way.

### 1.2 Impact

Desk: every `AddOrder` → "preflight: no quote" (no trades, live or paper). Story: every playbook action suppressed before audit. Capture: a 4.3 GB tape with no bid/ask/spread/depth — `optimizer/replay.resolveEntryFill` requires `HasBookDepth()`, so a `make tune` on this tape can never open a position; it will quietly conclude "no profitable forest". Instrument rules: `PrepareEntryOrder` runs against an empty cache. All of it silent.

### 1.3 Fixes

(a) Revert the two call sites to the pool registry (`pool.CreateBroadcastGroup("raw", …).Subscribe(...)`), or have `runtime.New` construct these caches with an injected `*qpool.BroadcastGroup` taken from the pool — DI here also removes the hidden ordering dependency. (b) In qpool, either delete the foot-gun (`NewBroadcastGroup` exported alongside registry-backed creation with identical signature feel) or rename it `NewDetachedBroadcastGroup`. (c) `QuoteCache.run` currently does `if err != nil { return }` — a silent unsubscribed-forever exit. Any consumer whose subscription fails must log loudly and/or crash the engine: this code converted a wiring bug into an hour of bad data. (d) Add the watchdog from §7 so "0 quote frames in 60s while raw frames flow" pages you. (e) The repo as published cannot pass its own tests (`replace ../qpool` hides this). Either publish the real qpool/errnie versions and pin them, or vendor them; CI should run against exactly what go.mod resolves for a fresh clone.

Two adjacent cleanups while you're in there: `integration/harness.go`, `harness_inject.go`, `rawrelay.go`, `observe.go` all publish through `qpool.NewBroadcastGroup` — under pinned semantics the harness exercises a private bus, not the one production components subscribe to, which means the e2e suite's "production stack on synthetic tape" claim depends on your local qpool's behavior; and qpool ignores the TTL argument entirely (`_ = ttl`), so the carefully chosen 10ms/500ms TTLs sprinkled across the codebase are decorative — delete them or implement TTL.

---

## 2. Economic model integrity (paper + replay fills)

This section is about whether the numbers the system optimizes and reports are the numbers reality would produce. Several biases all point the same (favorable) direction.

### 2.1 Stress-path fills are anchored at `Last`, erasing the spread — P0

`broker.SlippageFill` walks the book correctly (VWAP across levels, coverage, slippage measured from best). But when `trading.replay.execution_stress_enabled` is true — **the shipped default**, and it governs the paper matcher too — the result is discarded and rebuilt from the reference price:

- `execution/stress.go:97` `StressedFillQuote`: `price = reference × (1 ± slippagePct)` where `reference = quote.Last` (`broker/execution_stress.go:63-66`) and `slippagePct = (slippage-from-best) × multiplier`.
- For a small order fully filled at the touch, slippage-from-best ≈ 0 and the multiplier is 1 in calm regimes ⇒ a **buy fills at `Last`**, which after a sell print is the bid. The book walk's ask-side price is thrown away. Sells symmetrically fill at `Last`.
- Wiring: paper taker fills go `kraken/paper/response/order_fill.go:67` → `broker.StressedSlippageFill` → `applyExecutionStress`; replay goes `optimizer/replay/fill.go:47` → `StressedSlippageReplayFill` → same.

A round trip therefore costs ≈ 2×fee and ~zero spread. On BTC/EUR that's a ~1bp flattery; on the 120bps-spread microcaps the config comments say this playbook hunts, it overstates PnL by up to ~1.2% per round trip — bigger than the entire stop/take grid (1.2%/2.4%). The optimizer will preferentially select strategies whose real-world cost is dominated by exactly the cost term the simulator deletes (high-churn, wide-spread names).

**Fix.** Make the stress transform *worsen* the book-walk price rather than re-derive from `Last`: `price = fill.Price × (1 ± extraStressPct)`, with `extraStressPct = (multiplier−1)×baseSlippage + shortfall term`. If you keep a reference-anchored form, the reference for a buy must be `max(ask, fill.Price)` and for a sell `min(bid, fill.Price)`. Add one regression test: calm regime, full coverage, buy ⇒ `price ≥ quote.Ask`. That single assertion would have caught this.

### 2.2 Replay's latency model fills at the signal-time price — P1

`optimizer/replay/ledger.go:224` holds actions for `execution_latency_ms`, then fills with `executionFillMeasurement(item.measurement, currentRow)` — but `currentRow` is whatever tape row happened to expire the timer. If it belongs to **another symbol** (the common case on a 376-symbol interleaved tape), the function returns the original signal-time measurement, i.e. **zero price drift during latency**. Adverse selection during the latency window — the main cost latency models exist to capture — is unmodeled except in the rare same-symbol coincidence.

**Fix.** At queue time, resolve the fill row properly: scan forward in the symbol's own precompiled index (`symbolIndices` already exists in `tape_precompile.go`) for the first row of that symbol with `At ≥ executeAt`, and store its tick index on the pending item. Fall back to `lastQuotedRows` only at tape end.

### 2.3 Maker/limit optimism — P1

Three related generosities: (a) Protective `*_limit` exits in both paper (`order.go:354-369`) and replay (`ledger_trigger.go:160-171`) fill **at the trigger level on first touch** with maker fees — no queue, no trade-through requirement; touch-fills are notoriously ~50% optimistic. (b) Replay maker entry queue depletion is hardcoded at `Last × 0.0001` per tick (`ledger_maker.go`), an arbitrary proxy with no relation to printed volume. (c) Paper partial fills price the covered quantity at the *blended* price that included an optimistic mid-priced remainder (`slippage.go:84-93` + `order_fill.go:85`), slightly under-charging every partial.

**Fix.** (a) Require trade-through (a print strictly beyond the level, which you already have on the live trade feed; in replay require the *next* same-symbol price to be beyond the level) or charge a touch-fill probability haircut. (b) Drive depletion from the actual `TradesChannel` volume you already subscribe to (paper does this via `TradeDepletesMakerQueue`; replay can use per-row `Volume` deltas), or at least make the constant a config. (c) When reducing quantity to covered, recompute the price from the covered book-walk only (`avgPrice`, not `blended`).

### 2.4 The latency profile is all zeros and the recorder can't work — P1

`runs/network_latency.json` is 64 lines of `0`. Three independent bugs guarantee that: (1) Kraken v2 pongs arrive as `{"method":"pong",…}` but `readFrame` checks `message.Type == "pong"` and `SocketMessage` has no `method` field (`kraken/public/websocket.go:235-243,321`), so a latency sample is never taken; (2) `recordLatency` only runs when `time.Now().Unix()%64 == 0` inside a 10s ticker — ~1 chance in 6.4 per ping cycle; (3) the file is opened `O_WRONLY` without `O_TRUNC`, so shrinking content would leave stale bytes. Net effect: paper trading runs with **zero one-way latency** (`scheduleTaker` executes instantly), while replay uses a separate configured 100ms that fills at stale prices (§2.2). Paper and replay thus model latency differently, and both wrong. Also note `time.Sleep(latency)` inside the paper socket loop (`kraken/paper/websocket.go:138-143`) stalls the *entire matching engine* (trigger checks included) rather than delaying one order.

**Fix.** Parse pongs by `method`; sample every ping; write atomically (temp+rename, `O_TRUNC`); record real RTT/2. In the paper engine, delete the sleep — latency is already modeled per-order via `pendingTakers.due`. Then give replay the same sampled profile instead of a fixed 100ms.

### 2.5 Smaller economic-model notes — P2

`trader.recordPositionClose` computes realized PnL as `(exit×qty − exitFee) − entry×qty`, omitting the entry fee that `paper.Balances` correctly folds into cost basis — the audit's number will disagree with the wallet's by ~0.16–0.26% per trip. `sizeEntry` sizes at `action.Price` (last) while the fill happens at the ask + slippage, so buys can exceed the reserved slot and bounce off `ApplyFill`'s insufficient-funds gate — size at the ask (you already compute `sizingReferencePrice` in replay; the trader should use the same). Shorts in replay debit `notional×(1+fee)` as collateral and settle symmetrically — fine as a model, but worth a comment that it is a collateral model, not spot.

---

## 3. Optimizer methodology

### 3.1 Pure in-sample selection — P0

The pipeline (`cmd/tune.go` → `optimizer/tune/measurements.go` → `reasoning.Search`) is: load the whole tape → derive vocabulary *from that tape* → seed + beam-search (beam 8, ≤20 rounds, ≤24 nodes, deterministic, no RNG) → score every candidate with `walletVelocityScore` on the **same tape** → write the best to `market/perspectives/cfg/perspectives.yaml`, which the live story loads. `replay/walkforward.go` (expanding-window holdouts, plus `HoldoutDecay` in `ledger_position.go:430`) exists and has **zero non-test callers**.

The vocabulary (`vocabulary.go`) gives categories × regimes × thresholds × lookbacks × offsets × fractions × action types — thousands of leaf predicates; `countNodes ≤ 24` still admits an enormous hypothesis class. The low-trade discount stops at `maxEffectiveMinRoundTrips = 12` (`min_round_trips.go:7`). Selecting the max over hundreds of evaluated forests of a noisy PnL estimate with ≥12 trades is a textbook recipe for selecting noise; the velocity multiplier (`score_velocity.go`: scale by `TotalTicks/ExposureTicks`) further rewards lucky quick flips, and the strategy-breadth bonus rewards adding branches.

**Fix (concrete, in order of value):**
1. Wire `WalkForwardHoldouts` into `TuneMeasurements`: search on each train fold, score the *single* chosen candidate per fold on its holdout tail, select by mean holdout score, and refuse to write the playbook when `holdoutDecay > θ` (you already wrote that function — use it as the gate).
2. Purge/embargo at the split: drop `max(horizon, regime window)` worth of rows around each boundary so latched chains and the 60s prediction horizon don't straddle folds.
3. Raise the statistical bar: replace the linear `<12 trades` discount with a penalty that scales with search breadth (e.g. require `trades ≥ k·log(candidatesEvaluated)`, or apply a deflated-score correction). Report per-trade mean and its standard error in `CandidateScore`, not just the sum.
4. Stress-test the survivor: rescore the final candidate with fees +50%, spread fully charged (§2.1 fixed), latency ×3; refuse to publish if the sign flips. Cheap, brutal, effective.
5. Keep `--candidate-report` on by default during tunes; it's your selection-bias forensics.

### 3.2 The replay decision window is not the live window — P1

Live: `story.rememberMeasurement` keeps a **per-symbol ring of the last 1024 measurements of that symbol** (`market/story.go:34-50`, `story_record.go:34`). Replay: `mergeSnapshotIndices` (`tape.go:73`) takes the symbol's rows **within the last 1024 global tape rows**. On a 376-symbol tape a typical symbol gets ~1024/376 ≈ 3 rows of context in replay versus up to 1024 live. Regime classification (`min_samples: 16`), `ago` lookbacks, and `within` durations therefore see structurally different histories in the two clocks — the comment on `AppendSnapshot` ("the exact live-story ring snapshot") is wrong for any multi-symbol tape. A playbook tuned under 3-row windows trades live under 1024-row windows.

**Fix.** Precompile per-symbol windows: for tick *i* of symbol *S*, the snapshot indices are the last ≤1024 indices of `symbolIndices[S]` up to *i* (plus globals interleaved by index). That's exactly what `indicesInWindow` already supports if you bound by *count* on the symbol's own index list instead of by global tick distance. One test: build a story ring and a tape from the same row sequence and assert identical snapshots per tick.

Related minor divergences worth one line each in code comments or tests: live resets `ReasonState` on entry *emission* (`story.go:301-303`), replay only on holding/regime flip inside `evaluateStateful` — they differ when an entry is emitted but blocked; prediction rows (Source 11) are in both window populations (consistent — fine), but they double every price point on the tape and halve the effective per-symbol window depth in replay's global-window scheme, which fix #1 above also resolves.

### 3.3 The tape itself — P1

From profiling `capture.jsonl` (17.65M rows, 56 minutes, 376 symbols): 50.0% are Source 11 (prediction) by construction; of the signal rows, exhaustion (20%) and depthflow (18.6%) dominate while fluid (0.014%), liquidity (0.03%), cvd (0.14%), sentiment, correlation, hawkes, pumpdump are nearly absent. The currently deployed playbook gates on `laminar` (fluid) **and** `extreme_scarcity` (liquidity) — on a tape like today's those predicates have essentially no data to be true on. Also: ~0.25% of sampled rows have non-monotonic timestamps (multi-goroutine emission through the bus), and `make run --record` **truncates** the capture on every start (`os.Create`, `story.go:145`) — yesterday's 8-fill day is gone from the tape the next tune would use.

**Fix.** Rotate captures instead of truncating (timestamped files, tune over a directory); enforce/normalize ordering at load (`LoadMeasurements` could stable-sort by `At` within a small jitter window, or the recorder could sequence rows); log per-source row counts at tune start and refuse to tune a playbook whose predicates reference sources with < N rows (this also catches the §1 degenerate tape: "0 rows with book depth — aborting" instead of silently scoring 0 trades).

### 3.4 Memory: the whole tape, precompiled, in RAM — P2

`LoadMeasurements` reads everything into a slice; `PrecompileTapeWorkers` adds a `PrecompiledTick` (with snapshot index slice) per row. Today's 4.3 GB / 17.6M-row capture will not precompile comfortably on a laptop. With per-symbol windows (§3.2 fix) snapshot indices get much smaller; additionally support `--max-measurements` defaulting to a sane cap with a warning, and consider chunked walk-forward (which you need anyway) as the natural memory bound.

---

## 4. Signal & numeric layer

### 4.1 SNR is two regimes glued together, and 35% of the tape sits on one constant — P1

`numeric/adaptive/snr.go`: before `minObs=12` observations the score is `value/minStd = surprisal/0.1` — the tape's cold-start rows show exactly `20` (2 bits, uniform-over-4 prior) and `15.85` (log₂3). After warmup, a stable category converges to surprisal `s* ≈ 0.102` bits (the +α/N smoothing floor) and the score becomes `s*/(s*+0.1) = 0.5047` — **34.8% of all 17.65M rows carry exactly `0.5046552843593092`**, across symbols and sources. So "SNR ≥ 1" doesn't mean "1σ above baseline"; it means "tracker cold, or category just changed". The optimizer happily tunes thresholds like 1.0 against this mixed scale, and cold-start spikes are concentrated at session starts and symbol discovery — i.e., the replay can learn "trade the first minutes of the tape".

**Fix.** (a) Don't emit gate-able SNR during warmup: return 0 (the measurement contract already allows snr 0) or mark rows `warmup=true` and have both `WindowReason.Signal` and replay ignore them for `UnitSNR` predicates. (b) After warmup, score `(value−mean)/std` symmetrically rather than the `value/(mean+minStd)` branch, so the stable-state value is ~0 instead of an arbitrary 0.5 constant; if you keep the asymmetric form, document that 0.5047 is the "nothing happening" fixed point and that thresholds live on (0.5, ~3]. (c) Add a unit test asserting the warmup and steady-state scales match within a factor, so the next tuning of `minStd` doesn't silently move the threshold semantics again.

### 4.2 Conviction ranking compares incomparable quantities — P2

`actionConviction = SNR × Confidence` ranks entry candidates across *different sources and symbols* (`entry_batch.go:51`), and preemption closes a held position when a new candidate's instantaneous conviction beats the *stored entry-time* conviction of the victim. Given §4.1, cross-source SNR scales differ; and conviction decays as positions age only in the sense that it was frozen at entry. There is no hysteresis: a marginally-better candidate triggers a full round trip of costs.

**Fix.** Require a margin (e.g. `new > victim × 1.5`) plus a per-symbol preemption cooldown; rank within-source by SNR percentile (you already keep per-source distributions in the calibrators — expose a percentile) before multiplying by confidence. Also fix the small state bug: `preemptPlan` is a single slot — a second preemption planned before the first victim's close arrives overwrites the first plan (`entry_batch.go:159`), and the displaced entry is silently lost while its victim still gets sold; queue plans per victim symbol, or refuse a new plan while one is pending.

### 4.3 Shared calibration: defensible, but two real risks — P2

The pooled `BandCalibrator` (one distribution across symbols per signal, periodic quantile refits) is a reasonable answer to per-symbol sparsity, but (a) refits re-tune band edges *online from the same stream the bands classify*, so category labels are non-stationary across a session (the cvd dumps show shares are *roughly* stable today — drift is an ongoing risk, not an observed crisis), and (b) cold-start seeding reads the *previous session's own dump* (`numeric/calibration.go`, `SeedCalibratorFromDump`), so a mis-tuned session partially inherits itself. The dumps also contain duplicated consecutive rows (identical depthflow records nanoseconds apart), which double-weight refit samples.

**Fix.** Freeze band edges after a warmup window and refit on a slow cadence with a max-step (you already blend edges; bound the per-refit movement). De-duplicate identical consecutive observations before they hit the calibrator window. For seeding, validate the seed distribution against the live one (KS-distance gate) and fall back to defaults on mismatch.

### 4.4 Toxicity/quote-notional double duty — P2

`StampQuoteNotional` (`market/story.go:629`) rewrites `measurement.Volume` from "whatever the signal put there" to "24h base volume × last", in place, for every measurement. The field then means different things before/after stamping and between live (stamped) and any code reading signals pre-story. Give the notional its own field (`QuoteNotional`) on `Measurement` and leave `Volume` alone; `volume rose_by` predicates then declare which one they read.

---

## 5. Live trading robustness

### 5.1 Dropped fill frames permanently wedge a symbol — P1

Delivery on the bus is at-most-once: each subscriber is an SPSC ring with `dropOldestOnFull=true` (qpool `broadcastgroup.go:386-396`, `spsc_ring.go:61`). The trader's single `raw` consumer (buffer 1024, `trader/crypto.go:163`) receives the **entire public market-data firehose** plus actions plus executions, and drains it on a loop that sleeps 2ms when idle and does UI work on 500ms marks. A burst (book snapshots after reconnect across hundreds of symbols) can evict queued frames; if an `executions` envelope is evicted, `crypto.pending[symbol]` is never cleared — the symbol is "order still resolving" until process restart, and the trader's inventory silently diverges from the paper wallet (which *did* apply the fill). Same risk applies to the story's measurement consumer (decisions skipped) and the hub (cosmetic).

**Fix.** Three cheap layers: (1) dedicated channels — publish executions/holdings on their own broadcast group (`"executions"`), so the money-path consumer queue is not shared with ticker spam; (2) a pending TTL — if an order ack/fill hasn't arrived within `order_ack_timeout` (already in config, currently unused by the trader), clear pending, audit a `lost_execution` frame, and reconcile via a holdings snapshot request; (3) make the paper engine publish holdings periodically (it currently only does so on subscribe), so `reconcilePositions` actually has a recurring signal to heal from.

### 5.2 The protective-order model has three sharp edges — P1

(a) **One trigger per symbol.** Paper keeps `triggers[symbol]` (`order.go:269`) and replay one `triggerType` per position — arming a take-profit *replaces* the stop. The trader's dedup key is `symbol:side` (`crypto.go:390`), and stop+take for a long are both sells, so a playbook that emits both alternately would re-arm (and on live Kraken, re-*place*) an order every tick, alternating which protection exists. Today's playbook only emits `trailing_stop`, so this is latent, not active. Fix: model a stop/take *pair* (Kraken supports conditional close / OCO semantics) or at minimum key storage and dedup by `(symbol, side, actionType)` and reject arming a second protective type with a clear audit reason.
(b) **Short-cover via trigger books a phantom long.** When a resting protective fill arrives with no pending entry, `observeExecution` treats `ActionNone+buy` as `openPosition` (`crypto.go:777-785`) — for a triggered short-cover this *opens a long* equal to the cover quantity. Unreachable today (margin off ⇒ no shorts) but armed and waiting for the day margin is enabled. Fix: when an untracked buy execution arrives for a symbol with `shortInventory > 0`, close the short.
(c) **Orphaned exchange orders.** `closePosition` clears local `armed` state but never cancels the resting protective order. Paper's `executeTaker` happens to delete the trigger on any sell; a real exchange would keep the stop working — a stray order that can fire later. Fix: track order IDs from the ack and cancel on close (the desk already has `CancelOrder` plumbing in `kraken/trading`).

### 5.3 The audit trail can't do its one job — P1

Evidence from a full day (`audit.jsonl.1` + `audit.jsonl`): 144,241 `playbook_walk`, 534 `trade_decision` (515 rejected / 11 submitted / 8 filled), and **0 `position_outcome`** frames. Causes, all in code: `recordPositionClose` only runs on the explicit trader-initiated exit path — fills arriving for protective exits (no `pending` entry ⇒ `ActionNone` branch) close positions without writing an outcome (`crypto.go:805-815` vs `777-785`); the audit writer latches the first I/O error forever and stops the drain loop (`audit/writer.go:197-212`), with each subsequent `Write` failing; `bufio` is flushed only on rotation/close, so a crash loses the tail; rotation keeps 32 MB × 3+1 ≈ 130 MB ≈ half a day at yesterday's rate.

**Fix.** Write `position_outcome` from the *wallet*, not the trader: `paper.Balances.ApplyFill` knows entry basis (fee-inclusive) and realized PnL exactly — emit the outcome frame there (and from the live executions stream keyed on `cl_ord_id`), so every close is accounted regardless of which path closed it. Make the writer recover: on write/rotate error, retry with backoff and reopen the file; only fail-stop after N consecutive failures, and surface that on the dashboard. Flush on a 1s ticker. Make `gate-reject` rates per-gate counters in `runstats` so the audit isn't the only place this information exists.

### 5.4 The public websocket has no liveness guarantee — P1

`kraken/public/websocket.go`: `readFrame` blocks on `ReadJSON` with **no read deadline**; the ping is sent from the same loop, so a silently dead TCP connection (NAT timeout, half-open) blocks the loop forever — no error, no reconnect, prices freeze. The quote staleness gate would stop *entries* but also blocks *exits* ("stale last price for exit"), so open positions would be unmanageable precisely when the feed dies. Additionally `isConnected`/`conns[0]` are written by the read loop and readable via `AppendConn`/`Close` without synchronization (a `go vet`-clean but race-detector-dirty pattern), and `markDisconnected` closes within the same select cycle.

**Fix.** `SetReadDeadline(now + 2×heartbeat)` before each read (Kraken heartbeats every second; a deadline turns silence into an error that the existing reconnect path already handles); set a write deadline on pings; make `isConnected` an `atomic.Bool` and guard `conns`. Also worth knowing: `reconnectBackoff` is a nice Binet's-formula Fibonacci, but a comment would help the next reader.

### 5.5 Dead and decorative safety machinery — P2

`Desk.TripHalt` (circuit breaker + cancel-all) has **zero callers** — no error storm, drawdown, or feed-death condition ever trips it. `trading.order_ack_timeout` is configured and read nowhere. `NextClOrdID` restarts at `s…0001` every process start — Kraken treats duplicate client order IDs across sessions as yours to deconflict; prefix with a session epoch (`fmt.Sprintf("s%08x%08x", startUnix, counter)`). Wire TripHalt to: N desk errors in M seconds, audit writer failure (§5.3), wallet/inventory reconciliation mismatch, and feed silence (§7).

### 5.6 The trader/story event loops do avoidable work — P2

`publishDecisionTree` rebuilds and broadcasts the entire tree map on **every measurement** (`story.go:389` via `foldDecisionTrace`); at thousands of measurements/sec this is the largest allocation source in the story and floods the hub (which relays to every dashboard client synchronously — a stalled client's `writeJSON` blocks the hub Tick; add per-client send queues with drop-on-full and a write deadline). Throttle the tree to a 250ms cadence or on-change (you already have `lastGauge` rate-limiting for gauges — reuse it). The trader and paper sockets use 2ms `time.After` poll loops — fine functionally, but `Wait(ctx)` with a marks ticker in a second goroutine (or qpool gaining a select-able channel) would remove constant timer churn. `story.loadThoughts` re-reads the playbook file on every measurement while no playbook exists (`story.go:233`); cache the miss with a 5s recheck. Lastly `signal/pool.go`'s sync.Pool pattern (`Get`, unmarshal, copy, `Put`) allocates a fresh copy every call — the pool saves nothing; either return the pooled slice with an explicit release contract or drop the pool.

---

## 6. Code health, config, hygiene — P2 unless noted

**Config truth drift.** Yesterday's run rejected at `max_spread_bps 60`; the checked-in config says 120 — tunes and runs are not pinned to the config they ran with. Stamp the *effective* config (viper snapshot hash) into the capture header and audit prologue. The Makefile defines `TUNE_WORKERS/TUNE_MAX_THRESHOLDS/TUNE_BEAM_WIDTH/TUNE_CANDIDATE_LIMIT` and passes none of them to the binary — beam width is silently the code default 8, not 256; either export them as flags/env the binary reads, or delete them. `config.yml` (the "legacy merged" file `make run` still uses via `CONFIG ?= cmd/cfg/config.yml`) and `infra.yml`+`strategy.yml` are two parallel sources of truth — pick one (the Makefile default contradicts the file's own header comment).

**Repo hygiene.** Two 16 MB architecture PDFs, a 5.5 MB and a 1.9 MB PNG, and a 2 MB `symm.txt` dump are committed (and `symm.txt` is in `.gitignore` yet tracked); `frontend/dist/**` (including four SciChart WASM blobs) and `.pnpm-store` are committed; stray test artifacts live in `kraken/public/runs/`, `kraken/paper/runs/`, `market/runs/` because `recordLatency` and audit tests write relative paths from package CWDs. Move docs to LFS or a docs repo, `git rm --cached` the dist/dump/stores, and route all runtime file outputs through one `runs-dir` config value so tests stop seeding directories around the tree.

**Error handling style.** The pattern `if err != nil { return }` inside goroutine setup (quote cache, rules cache, several `Start` methods) converts wiring failures into silent feature loss — §1 is the proof of consequence. Adopt a rule: a component that cannot subscribe/open its inputs must `errnie.Error` *and* either crash the engine or mark itself unhealthy on the hub. The engine's `Start`/`errs` machinery already supports failing fast — use it.

**Tests.** `make test-race` excludes nothing relevant on Linux but on Darwin skips `/engine$` (no such package — the filter is stale). The integration harness's bus-injection issue is described in §1.3. `analyze`'s signal certification (DEAD/FLICKERING/FLAT verdicts) is a genuinely good idea — but nothing in the run path consumes its verdicts; a signal certified DEAD still feeds the trader. Wire the diagnostics thresholds (already in `strategy.yml signals.diagnostics`) into a startup/periodic check that benches (excludes from stress/conviction) a signal whose live diagnostic is DEAD or FLICKERING. The audit ring queue (`audit/ring_queue.go:55-60`) CASes `published` before storing the slot — a preempted producer makes the consumer spin/sleep briefly on a nil slot; store-then-publish (claim with CAS on a separate `reserved` counter, or store the frame then publish) removes the stutter, though no frames are lost as written.

---

## 7. The meta-fix: silence must be loud

Every serious data-layer failure found in this review was *silent*: the quote cache starving (§1), the latency recorder writing zeros (§2.4), audit frames stopping at 22:57 (§5.3), a tape with no books (§3.3), a dead circuit breaker (§5.5). The system has rich telemetry for what *is* flowing and none for what *should be flowing but isn't*.

Add one small component (e.g. `runstats.Watchdog`) with declarative expectations, each tied to an escalation:

| Expectation | Source | On violation |
|---|---|---|
| quote cache ingested >0 ticker+book frames / 30s while raw frames flow | QuoteCache counter vs ws counter | TripHalt + hub banner |
| every audit Write ok; queue depth < 50% | audit.Writer | hub banner; halt if trading.model=live |
| capture rows carry bid/ask when record enabled | story recorder | log + banner ("tape will be untunable") |
| pending order resolves within order_ack_timeout | trader | clear + reconcile + audit `lost_execution` |
| pong observed each ping cycle | public ws | force reconnect (with §5.4's read deadline) |
| per-source measurement rate within ±5σ of trailing norm | signal pool | bench the source (ties into analyze verdicts) |

This is a day of work and converts the whole class of "ran blind for hours" incidents into pages.

---

## 8. Prioritized action plan

| # | Priority | Item | Effort | Section |
|---|---|---|---|---|
| 1 | P0 | Reconnect QuoteCache/InstrumentRulesCache to the pool registry; loud-fail subscriptions; discard today's bookless tape | hours | §1 |
| 2 | P0 | Anchor stressed fills to the touch/book-walk price (paper + replay); add the `price ≥ ask` regression test | hours | §2.1 |
| 3 | P0 | Walk-forward holdout in `tune` with `holdoutDecay` publish-gate; purge boundaries | 1–2 days | §3.1 |
| 4 | P1 | Replay latency fills from the symbol's own post-latency row | ~1 day | §2.2 |
| 5 | P1 | Per-symbol replay snapshot windows = live ring semantics + equivalence test | ~1 day | §3.2 |
| 6 | P1 | SNR warmup suppression + steady-state rescale; document/threshold semantics | ~1 day | §4.1 |
| 7 | P1 | Wallet-sourced `position_outcome`; audit writer retry/flush; pending-order TTL + holdings reconciliation cadence | 1–2 days | §5.3, §5.1 |
| 8 | P1 | WS read deadlines + atomic connection state; watchdog component | 1–2 days | §5.4, §7 |
| 9 | P1 | Fix latency recorder (pong parsing) and remove matching-engine sleep | hours | §2.4 |
| 10 | P2 | Protective-order pair model; short-cover fix; cancel-on-close; ClOrdID epoch; wire TripHalt | 2–3 days | §5.2, §5.5 |
| 11 | P2 | Maker touch-fill haircut; volume-driven queue depletion; partial-fill pricing | 1–2 days | §2.3 |
| 12 | P2 | Capture rotation + tape sanity gates + per-source coverage report at tune start | ~1 day | §3.3 |
| 13 | P2 | Preemption hysteresis + plan queue; conviction percentile normalization | ~1 day | §4.2 |
| 14 | P2 | Decision-tree throttle; hub per-client queues; loop cleanups | ~1 day | §5.6 |
| 15 | P2 | Config/Makefile truth (effective-config stamping, dead knobs); repo hygiene; publishable qpool/errnie pins | ~1 day | §6 |

**Sequencing note.** Items 1–3 change the meaning of every number the system produces; re-record a capture and re-tune only after they land — any tuning before that optimizes the artifacts this report documents. Items 4–6 should land before you trust a tune enough to consider live keys. The rest hardens operations.

---

## 9. What is good (and worth protecting)

So the criticism lands in context: the shared-economics discipline (one `SlippageFill`, one `PreflightGates`, one `EntryDeployFraction` across desk/paper/replay) is the single most important design decision in the codebase and it is *right* — §2's bugs are fixable precisely because the model lives in one place. The reasoning language with latched temporal chains is expressive yet auditable, and `EvaluateStateful`'s episode semantics are clean. Deterministic beam search was a smart choice over MCTS for reproducibility. The measurement/raw-dump/analyze loop is a better signal-QA story than most production trading systems have. The audit-frame vocabulary (playbook_walk → trade_decision → fill → outcome) is the right schema — it just needs the outcome leg actually wired. And the per-symbol surprisal framing of SNR is a genuinely interesting idea; §4.1 is about its scale pathologies, not the concept.

*Generated by a full-code review on 2026-06-07. Line numbers refer to the working tree at `90a66a29` (plus uncommitted modifications present at review time).*
