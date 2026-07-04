# symm — Critical Code Review

Scope: `symm`, plus the `nomagique` and `datura` sibling packages it depends on via the `replace` directives in `go.mod`. Four axes, per request: signal-math correctness against the per-signal `Signal`-type spec, code shape against the compositional pattern in `AGENTS.md`, re-implementation of things that already exist, and lock-free/performance. Findings are not ranked — everything below is treated as equally critical.

Method: the tree was swept by five focused passes (two math passes across all thirteen signals, one shape pass, one duplication pass, one performance/lock-free pass), each reading the actual source and quoting it. Two of the highest-impact claims (the Hawkes shadowed named-returns and the manifold `medianAbsolute`) were re-read and confirmed line-for-line during write-up. All paths are relative to the three repo roots.

---

## 1. Signal math vs the spec

The governing rule (`AGENTS.md`): the comment block above each package's `Signal` type is the absolute spec; no magic numbers, no fixed windows/horizons, no fallbacks, no performative math, no assuming all symbols share one temporal scale, and dump/exhaustion detection must be direction-aware.

### Cross-cutting math defects (recur across most signals)

- **No signal derives any window or horizon from observed timestamps.** fluid/manifold run on a fixed `integrationInterval`/`DeltaT`; resonance, pumpdump, exhaust, toxicity, cvd, depthflow all window by *sample/frame count* (`len(history)`, frame counters, fixed caps of 32/64/128). `nomagique/timeline` is imported by none of the signal packages. Every spec that promises "tick cadence" / "observed update cadence" is contradicted by the code.
- **`nomagique/statistic.ResolveWindows` is misused as a no-op or a retention length.** In the count-only path (`nomagique/statistic/windows.go:109-121`) it forces `longWindow == sampleCount`, so the "adaptive" flow/RVOL windows collapse to the entire buffer. In resonance (`baselines.go:26`, `feed_ring.go:122`) its second return (`longWindow`, a computation length) is used as a ring-trim count — a category error — and it is fed epoch-nanosecond timestamps as the value series, so its CV is ≈0 and the window degenerates to `~sqrt(n)` regardless of true cadence.
- **A `1/N` uniform constant is used as a computed measurement.** `probabilities [0.25,0.25,0.25,0.25]` (4-way) or `[1/3,1/3,1/3]` (3-way) with a hard category label is stamped whenever a stage yields nothing — pumpdump (ticker/book/trade), exhaust, toxicity. This fabricates a fully-formed, often *directional* conclusion in place of an abstention.
- **Category scores are unbounded products/ratios fed to a softmax.** correlation, leadlag, depthflow, manifold, and the shared `nomagique/probability/classifier.go:241` softmax compare scores built on incommensurable scales, so the winner is whichever score is numerically largest, not a calibrated posterior. Bare `(1-x)`, `1/(1+x)`, and `*` multipliers with no statistic in the denominator are the norm (the `AGENTS.md` "bare multipliers" anti-pattern).
- **Direction-blind / positive-only detection recurs** where the spec demands two-sidedness: fluid (F8), manifold (M9), resonance (R10), pumpdump (PD3), exhaust (EX2).

### hawkes

- `nomagique/algorithm/excitation_symbol.go:444-502` `organicHeadroomScores` declares named returns `(frenzy, saturation, organic, exhaustion)`, then at `:466` `saturation, err :=` and `:484` `frenzy, err :=` use `:=`, creating locals that **shadow** the named returns. Those locals feed only the discarded `headroom`. The function therefore **always returns `frenzy=0`, `saturation=0`, and `exhaustion=0`** (exhaustion is never assigned anywhere); only `organic` survives. Two of the four Hawkes regime scores are dead. *(Re-read and confirmed.)*
- `nomagique/algorithm/excitation.go:115-116` wires **branching ratio and spectral radius to the same value** (`BranchingRatio` written to both keys); `excitation_symbol.go:154` sets `branchingRatio = fit.SpectralRadius`. The spec treats α (descendants/parent) and ρ (stability eigenvalue) as distinct; the true branching ratio is never emitted.
- Magic horizons/multipliers: `excitation.go:15` `hawkesFitCooldownMult = 50` applied bare (`windowSpan * 50`, `:305-311`); `excitation.go:240` `EventCount < 4`; `trade_excitation_sample.go:13-14` caps `128`/`64`; `:271-278` `if required < 8 { return 8 }`.
- `excitation_symbol.go:178,508-540` `confirmAsymmetryWithBook` folds L2 top-of-book imbalance into the Hawkes asymmetry — the signal's own spec (signal.go:42-45) explicitly declares book confirmation **out of scope** because there is no L3 ingest.
- `nomagique/hawkes/fit.go:266-269` `ClampSubcritical` returns unchanged for every subcritical fit, and `Valid()` already rejects supercritical fits, so the clamp body is unreachable.

### cvd

- `nomagique/algorithm/trade_flow_sample.go:163-169` sizes the "active flow window" via `ResolveWindows(notionals,0,0)` → window == whole buffer (capped at `flowSampleHistoryCap = 128`). Spec promises a window derived from observed notional history; in practice it is a fixed 128-tick cap that dispersion never shrinks.
- `nomagique/equation/flow.go:137-142,280` gate `flowPressure` and `impactEfficiency` (ratios) against the literal `1` with no distributional fence — magic thresholds.
- `trade_flow_sample.go:109-113` trusts the feed's `side` field as the aggressor tag with no tick-rule verification, while the spec markets "Tick Integrity." Unstated dependency.

### correlation

- `market/cross_section.go:142-149` builds per-symbol return series by *arrival index* and drops zero returns, then `market/peer_cache.go:60` aligns cross-symbol returns by tail position. Symbols update at different cadences (the code even tracks `updateGaps`), so index *i* is a different wall-clock instant per symbol — Pearson of index-aligned, differently-clocked series is not the "synchronized log-returns" the spec claims, and violates the shared-temporal-scale rule.
- `peer_cache.go:54-71` computes the measured symbol's correlation against a **leave-one-out** market median, but the `peerCorrelations` it is compared against (`:86-96`) use a **full-market (self-included)** median. `relativeCorrelation = alignment/median` (signal.go:223) divides quantities computed against different reference vectors.
- `signal/correlation/signal.go:265-279` `EnergyLift` is an unbounded ratio squashed only at the very end; the "Variance" axis is represented solely by this ratio into the softmax.

### leadlag

- `signal/leadlag/section_correlation.go:156-199` `alignedReturns` derives a single `interval` from the anchor's median spacing and shifts **both** anchor and follower by the same integer `shift = lag/interval`. The follower has its own spacing, so the realized wall-clock lag differs from the intended `lag` — the shared-temporal-scale anti-pattern, in the one signal whose entire purpose is temporal lead/lag.
- `section_correlation.go:201-220` index-pairs the two return vectors after independent zero-price filtering, so they can differ in length for reasons unrelated to lag, then tail-aligns by count.
- `signal/leadlag/ticker.go:157-160` category scores are un-normalized polynomial products; `sampleSupport = SampleCount/minCorrelationSamples` (`:115-121`) is unbounded and multiplies every score. `lagDampExponent = 1 + LagBars*LagCorr*(1+StallMargin)` (`:143-147`) uses the *signed* `LagCorr`, so a negative value drives `math.Pow(1-lagFraction, exponent)` toward divergence.
- `section.go:280-293` `moveThreshold` computes `mean, std := meanStdDev(...)`, discards `mean` (`_ = mean`), and returns `median + std` — mixing a median center with a std dispersion (MAD is used elsewhere), the discarded mean signaling an unfinished change.

### causal

- `nomagique/algorithm/pearl_sample.go:246` sets the "Macro Momentum" control node to the **same symbol's** ticker `change_pct`. The spec defines node 0 as the broad-market drift the backdoor adjustment must control *for*; controlling for the target's own return makes the "Local vs. Global / Systemic Beta" decomposition vacuous. No `crossSection` data enters `PearlSample` at all despite the spec requiring cross-asset contagion.
- `nomagique/causal/contagion.go:60-99` `peakFromTable` returns `max|sample|` over the latest single row of one symbol's table — single-symbol, unbounded, never normalized toward 1.0, and not a co-movement measure. The regime gate (`regime.go:70-153`) then outlier-detects this raw magnitude and calls it contagion.
- `signal/causal/signal.go:114-124` hardcodes `"minHistory": 5.0`, `"window": 1.0`, never sets `history`; `pearl_sample.go:303-307` falls back to `minHistory`, so every symbol's causal buffer is a fixed 5 rows — OLS over 4 nodes × 5 rows sits at the rank floor. A literal fixed horizon.
- `signal/causal/ticker.go:57-60` (and book/trade) set `counterfactualReady = true` whenever `root == "output"`, regardless of whether Rung-3 actually ran; the ladder only computes uplift when `intervention > 0`, so "counterfactual ready" can report an untouched zero.
- The estimator itself (Frisch–Waugh residualization, ridge normal equations, Nadaraya–Watson weighting, condition number) is correctly implemented in `nomagique/causal/linear_fit.go` and `table.go`. The defects are in the node semantics and horizons feeding it.

### depthflow

- `nomagique/equation/bookflow.go:251-315` measures "Book Thinning: Rapidly Falling" as a static ratio of medians (`flatMedian/weightedMedian`) with **no rate/derivative** — nothing computes depth change over time, so the temporal crumbling story is absent.
- `nomagique/algorithm/bookflow_sample.go:275-282` trade-pressure EMA uses `smoothing = 2/(tradeFrameCount+1)` with an ever-incrementing, never-reset counter, so its effective window expands to the whole session — a de facto unbounded fixed horizon.
- `bookflow.go:159-171` spoof/thin/neutral scores are raw imbalance differences on different scales fed to the shared softmax.
- Magic caps/floors: `bookflow_sample.go:13-15` cap `64`, `bookflow.go:11` `minBookGateHistory = 3`, `bookflow_sample.go:531-535` `flatDepth` floored to `2`.

### fluid

- `signal/fluid/grid.go:179-190`, `grid_solver.go:66-84`: explicit Rusanov/RK2 advection+diffusion driven by `dt = integrationInterval.Seconds()` (default one minute) with **no CFL bound anywhere** (`max|v|·dt/dx ≤ 1`, `D·dt/dx² ≤ 1/2`). At minute-scale dt / tick-scale dx the scheme is unconditionally unstable.
- `grid_solver.go:55-64` Neumann ghost-copy boundaries (`rho[0]=rho[1]`) inject/delete mass every step, breaking the finite-volume conservation the framing claims; RK2 combine only touches interior cells.
- `grid_velocity.go:32-64`: velocity is built from the same `observedRho − remappedRho` residual that the source terms use, so `v` in `∂ρ/∂t + ∇·(ρv) = sources` is defined from the source residual — circular.
- `grid_velocity.go:12` `midPriceVelocity` divides by fixed dt not observed Δt; in the catch-up loop later sub-steps force velocity to 0.
- `grid_velocity.go:70-91` `estimateDiffusionCoefficient` returns `mean(|Δv|)/2` (velocity units) where `D` needs length²/time — dimensionally wrong, no length scale, no stability bound.
- `grid.go:365-399,505-541` `measureReplenishment`/Reynolds `flow` store incommensurable quantities (ratio vs rate·dt vs density-change/time) under one field and one set of quantile thresholds; `Re` returns `Inf` when `ν ≤ 0`.
- `grid.go:481-499` `medianObservedRho` sums positive densities and divides by *total cell count* — a count-diluted mean mislabeled median, while `stats.go:9 sampleQuantile` exists unused.
- `symbol.go:538-580` `priceMemoryFromSamples` is a 1-step ratio over a fixed 32-sample count, not the fractional-diff the spec claims. `symbol.go:517-536` `fluidViscosity` uses a bare `(1 + replenishment)` with dimensionally different branches.
- `grid_remap.go:24-46` piles out-of-domain mass onto edge cells then rescales, manufacturing a spurious source at the boundary whenever mid drifts ≥1 tick. `dynamics.go:33-48,111-121` append stamps unconditionally but values conditionally, desyncing `len(series) != len(stamps)`.

### manifold

- `field_config.go:29-30,96-105`: one global `DeltaT` (viper constant, or `1/bookDepth` fallback) divides every symbol's variance; `lastEventAt` is written (`field_feed.go:43,102,151`) but never read. Per-symbol temporal scale is discarded.
- `field_math.go:10-52` `returnAnalyticPhase` is `atan2` over a linear index ramp — not a Hilbert/analytic phase, but it is fed as `Oscillator.Phase`. `field_math.go:105-136` `returnFrequency` returns `2π/dt` on cold start (identical fabricated ω for every new symbol) and otherwise `sqrt(variance)/dt`, conflating return volatility with oscillation frequency.
- `field_deposit.go:17-18` deposits truncate the book to the **top level only** (`truncateLevels(...,1)`) while the grid is sized `bookDepth*4`; the depth profile collapses to two spikes. `field_deposit.go:111-158` `filterManifoldDensity` is an invented `alpha=|∇²ρ|/(|∇²ρ|+localMass)` mean-reversion with hard-coded 3-point stencil and `/2` — not the CFL-bounded diffusion the config enforces.
- `field_carriers.go:76-103` `liquidityRho` divides physical mass by `carrierCapacity = max(activeCarriers, MaxModes)` where `MaxModes` is a 256-thread GPU bound — mass scaled by hardware; whale path and book path use inconsistent `rho` scales.
- `signal.go:304-313` classifier features are raw/`1+`/negative-capable products, argmaxed over incommensurable scales; `RollingZScore` unused. `universe.go:205-219` orders the Z-axis by `medianAbsolute(returns)` (volatility) rather than the market-cap/beta ordering the spec's torus demands. `universe.go:80,471-472` `defaultBookDepth = 10` used as a per-level reference divisor. `field_step.go:15-16` advances exactly one fixed `DeltaT` per call regardless of real inter-arrival gap, decoupling the CFL guarantee from the data.

### resonance

- `attention.go:49-76` `AttentionCategoryIndex` is not attention (no query/key/value, no softmax, no temperature): stress is hard-assigned to latent index 1 via `abs(z[1]) > abs(z[0])+abs(z[2])`, and the autoencoder is fixed-seed (`rand.NewSource(42)`), so the axis has no guaranteed meaning. `attention.go:78-132` confidence/strength are bare reciprocals (`1/(1+surprise)`) maxed against unbounded raw activation or `abs(spread)` in bps — incommensurable; `:98` rewards a *more negative* invalid spread.
- `baselines.go:26-33,41-77` misuses `ResolveWindows` (return-value and value-series both wrong, per cross-cutting) and cold-starts with synthetic "normal" readings (`return 1`) — biasing exactly the obscure movers it targets.
- `sensory_facts.go:109,120` channels 0 and 11 (`changePct` and `|changePct|`) share one baseline ring, contaminating its MAD; channel 11 double-counts channel 0. `sensory.go:186-217` `buyPressure` divides net notional by `abs(net)` → collapses to `sign(net)` (±1), destroying imbalance magnitude. `sensory.go:199-206` trade rate divides by the whole ring span, not a rolling window. `sensory_facts.go:55,97` `tickCadence` overwrites the ticker clock with the trade clock and floors at a magic `1e-3`.
- `batch_engine_cpu.go:52` + `learning/resonance.go:147` reset every symbol to identical fixed-seed weights, wiping per-symbol adaptation on churn. `batch_engine_cpu.go:86-90` advances temporal state twice per settle (`Settle(...,true)` then `Learn(nil)`), corrupting the temporal-coupling error. `signal.go:352-378` the confidence gate almost never fires, `strength = peakActivation` (raw), and baselines are the constant `1/latentWidth`.

### pumpdump

- `signal.go:19-22` spec ("derived from the pair's tick cadence") vs `ticker.go:52-53` + `nomagique/statistic/mean_median_ratio.go:237-290`: window hint 0 → window = frame count; `shortWindow==longWindow==len(history)` so RVOL short-mean and long-median are over the same slice, and `if longMedian <= 0 { longMedian = shortMean }` pins the ratio to 1.
- `ticker.go:59-70` `positiveOnly: 1.0` clamps the precursor z-score at 0 — all four states are pump-side, so despite the name the package cannot detect a dump.
- `ticker.go:135-175` the ticker path (unlike book/trade) swallows the RoundTrip error and stamps the `0.25`-uniform fallback with `category = trend`. `book.go`/`trade.go` (`:71-123`) carry the same uniform default.

### exhaust

- `signal.go:19-65` spec ("series lengths derived from observed cadence") vs `nomagique/algorithm/decay_sample.go:20,314`: `ResolveWindows` derives from `sqrt(sampleCount)` over fixed-cap-64 rings; `timeline` unimported.
- `nomagique/equation/decay.go:130-349`: `depthTrend`, `spreadWiden`, `pressureFade` are all one-sided/positive-only; only `imbalanceFlip` is sign-aware, and `lastPrice` (`:51`) is validated but never used — so the price-rejection half of the mandated "lift decline AND price rejection" is entirely missing. `signal.go:200-252` carries the `0.25`-uniform fallback.

### toxicity

- `signal.go:182-230` `completeMeasurement` synthesizes `probabilities [1/3,1/3,1/3]` with `category = HardSupport` when the BookQuality stage emits nothing, and is called unconditionally at the end of `level3.go:60`/`trade.go:55` — a no-evidence frame exits as the directional conclusion "the wall will hold," not an abstention.
- `nomagique/algorithm/book_quality_sample.go`: EMA `smoothing = 2/(frameCount+1)` (expanding window keyed to event count, not a half-life); `resolvedGatePercentile` uses hard floors/ceilings (`0.5`/`0.75`/`0.9`), a `< 3`-frame warm-up, and a `/17` ramp; touch-price detection uses exact float `==` (any representational drift silently reclassifies a touch), inconsistent with the MAD-tolerant fill matcher.
- Correct: `IngestRoles` is `["level3","trade"]` only, so L2 qty deltas are never used as a cancel/fill fallback, and fill labels come from matching L3 deletes against actual trade prices — this honors the spec.

### liquidity

- `signal.go:92-110` bypasses the spec-faithful `nomagique/equation/depth.go` (25/50/75 peer quantiles + baseline gate) entirely; `median` is the cross-sectional median of the raw 24h summary `volume` field — one instantaneous snapshot, no window. `scarcity = 1-relative`, `depth = relative-1` are bare deviations of a ratio from 1.0 with no dispersion in the denominator (a 2× and a 10× event are indistinguishable in σ). `balance = 1/(1+|relative-1|)` is strictly positive, so the signal fires for every row; `relative` collapses to 1 when peers < 2.

### sentiment

- `signal.go:91-114` is built directly from the raw 24h `change_pct` summary field — the "scoring ticker summary fields and calling it microstructure" anti-pattern — with no entropy/KL/z-score against a live baseline (`statistic/entropy.go`, `kl.go`, `rolling_zscore.go` unused). `surgeScore`/`divergentScore`/`slumpScore` are ad-hoc products with `(1-breadth)` and `1/(1+leaderEvidence)` forms; `relativeLead` is a hard 0/1 indicator so `divergentScore` is identically 0 for every non-leader, while `slumpScore` is non-zero for every non-leader (a systemic-slump default hidden by the confidence≤0 silent drop). `cross_section.go:197-238` assumes one temporal scale for all symbols.

---

## 2. Code shape vs the compositional pattern

Rules (`AGENTS.md`): methods over loose functions; compose types (don't just move methods to new files); file ≤400 (target 200); method ≤60 (target 30); type ≤10 methods; guard clauses + early return; no `else`; nesting ≤2; names one–two segments; no silent fallbacks. Naming discipline is otherwise well-observed — no single-character receivers or locals were found, and the `errnie` error style is applied consistently.

**Files over the 400-line ceiling** (≈42 more sit in the 200–400 over-target band): `broker/desk.go` **1613**, `market/cognitive.go` 701, `signal/fluid/grid.go` 600, `signal/fluid/symbol.go` 580, `kraken/public/response/order.go` 541, `signal/manifold/universe.go` 486, `signal/resonance/signal.go` 463, `logic/operand.go` 434, `market/cross_section.go` 429, `trader/decide.go` 423. `broker/desk.go` at 4× the ceiling is its own subsystem — its doc comment says "Desk only owns the live stop map and forwards orders," but the file also does order construction, quantity/price resolution, balance caching, pending-order lifecycle, diagnostics publishing, and native-protective-stop submission: five-plus responsibilities that the pattern says to compose out.

**Methods over 60 lines:** `Orders.Send` (`kraken/public/response/order.go` ~82-313, ~232 lines, `switch` with a ~150-line inline `add_order` payload, 3+ nesting levels); `ConditionOperand.Resolve` (`logic/operand.go:52-395`, ~343 lines, one `switch` over `SubjectType`, the `SubjectEigenmode` arm alone `:252-387` has nested loops and an inline closure); `hydrateFieldFromTree` (`signal/manifold/observe.go:11-96`, nested 4-5 levels); `ConditionType.Evaluate` (`logic/condition.go:26-172`, ~146 lines); `readingFromEngine` (`market/cognitive.go:385-498`); `crypto.Run` (`trader/crypto.go:164-316`, ~152 lines); `feedBookLocked` (`signal/fluid/symbol.go:239-317`); `websocket.Run` (`kraken/public/websocket.go:156-235`); `manifold.Measure` (`signal/manifold/signal.go:129-198`).

**Types over 10 methods:** `CrossSection` (~28 methods, `market/cross_section.go`); `FluidGrid` (~22 methods + ~30 fields, `signal/fluid/grid.go:15-51`); `FluidSymbol` (~19); manifold `Signal` (~16 across `signal.go`+`observe.go` — split into a second *file*, which `AGENTS.md:148` explicitly calls out as *not* the intended fix); `Desk` (25 struct fields `:28-54` + dozens of associated functions — a god-type).

**Over-guarding (guard outweighing logic):** `logic/condition.go` `ConditionType.Evaluate` — 11 near-identical `case` arms whose real work is one comparison but whose error plumbing is ~6 lines each, re-wrapping an already-`errnie`-wrapped error in a second `errnie.Err(errnie.Validation, err.Error(), err)`; that double-wrap idiom (message = `err.Error()` *and* cause = `err`) appears 16× in this file and 55× across 21 files. `trader/crypto.go` reconstructs-and-revalidates the balances artifact three times (`:141-149`, `:187-191`, `:257-265`) with the same `origin=="" || scope=="" || len(payload)==0` guard. `market/story.go:58-66` re-initializes `symbols`/`dirty` with nil-checks that `NewStory:34-49` already guarantees. Pervasive `if <receiver> == nil` on types only ever built via their `New…` constructor (`Desk.storePending:458`, `releaseAckLock:482`, `pendingByClientOrExchangeID:572`, etc.).

**Loose functions that should be methods on a composed type:** `broker/desk.go` — ~16 free functions operating on order/execution/action artifacts (`actionAllowedForDispatch:238`, `actionSetupKey:256`, `terminalExecutionStatus:420`, `executionOrderIDs:734`, `requiresTriggerPrice:982`, `executionQuantity:1389`, `baseAsset:1206`, `artifactOrderID:1546`, …) — the behavior of would-be `Order`/`Execution`/`PendingOrder` types. `market/positions.go` — the *entire* file is package functions taking `tree *dmt.Tree` as the first arg (`PositionReadings`, `latestBalances`, `openPositions`, `latestMark`, …) — textbook `Positions{tree}` type. `market/cognitive.go` — a `CognitiveEvaluator` type exists (`:60-63`) but the real work lives in free functions beside it (`cognitiveObservations:185`, `readingFromEngine:385`, `cognitiveBranches:560`, `bestBeamScore:685`).

**`else` blocks and deep nesting:** `else` appears 16×, e.g. `signal/sentiment/signal.go:104`, `signal/resonance/attention.go:66`, `signal/correlation/signal.go:222`, `market/cognitive.go:450,674`, `market/positions.go:56-68` (a three-branch nested else-if chain), `broker/desk.go:369,1504,1576`. Nesting past two levels: `hydrateFieldFromTree`, `Orders.Send`, `ConditionOperand.Resolve`, `cognitiveBranches` (recursion).

**Convoluted / pass-through indirection:** `market/story.go:125-128` `Actions` is a pass-through to `ActionsWithTrace` that silently discards the error/trace. `logic/condition.go:229-249` `Condition.Evaluate` passes through to `Type.Evaluate` adding only another redundant re-wrap. `signal/fluid/symbol.go:235-237` `FeedBook` → `feedBookLocked` (the `…Locked` suffix implies a mutex the struct no longer has). `signal/fluid/grid.go` one-line getters that just rename private fields (`midAddRateAtTouch:401`, `viscosity:443`, `reynolds:501`). `kraken/public/response/order.go:527-535` a bespoke `fillPriceError` string type where `fmt.Errorf` would do.

**Names mutating something other than themselves:** `market/cognitive.go:314` `ApplyCognitiveReadings` (three-segment, and it mutates the `measurements` slice passed in, not a receiver — the exact `AGENTS.md:190-195` anti-pattern). `broker/desk.go` `submitNativeProtectiveStop:1230`, `markProtectiveWorking:631`, `publishCriticalStopDiagnostic:1407` — three-segment names, several mutating `Stoploss` state rather than Desk's own.

**Silent fallbacks / swallowed errors:** `signal/resonance/signal.go:424-430` `observedAt` returns `time.Now()` when the timestamp is zero — a default substituting for missing data. `signal/manifold/observe.go` `instrumentTick` returns `0` on every failure path; `observeBook/Trade/Ticker` and `forEachKrakenElement` silently drop elements on `json.Unmarshal` failure (`if json.Unmarshal(...) == nil`) — inconsistent with `panic`-on-feed-error in the *same* file (`:179,246,289`). `signal/manifold/universe.go` `loadSymbol:159`/`registerSymbols:173`/`loadIdentity:136` return nil/continue on error; `configureTickFromBook` replaces the wrapped cause with a fixed `fmt.Errorf("manifold: tick size is zero")`. `broker/stoploss_snapshot.go:62-71` swallows unmarshal failures (`return nil`). `market/story.go:173-195` logs `Evaluate`/`Marshal` errors but proceeds with partial candidates.

---

## 3. Re-implementation of existing primitives

Confirmed available and bypassed: `nomagique/statistic` (Mean, Median/`MedianOf`, MedianAbsolute/`MedianAbsoluteOf`, StdDev, Quantile, RollingZScore, Entropy, KL, ResolveWindows, ObservationRing, PriceRing), `nomagique/probability` (Softmax, Classifier, CUSUM), `nomagique/correlation` (Pearson→gonum, Covariance, HayashiYoshida), `datura/structure` (SPSCRing, MPMCRing, UltraRingBuffer, ListRing, ClockRing). symm imports only `ClockRing`/`ListRing`; `SPSCRing`/`MPMCRing`/`UltraRingBuffer` have **zero references** in symm.

**Statistics rolled by hand instead of `nomagique/statistic`:** `market/peer_cache.go:148,169,177,180,191` (median/median-abs via direct `gonum stat.Quantile`, skipping the nomagique NaN/Inf guards); `signal/manifold/universe.go:324` `median`, `:307` `medianAbsolute`; `signal/leadlag/section_correlation.go:245` `meanStdDev`; `signal/manifold/field_math.go:112-131` mean+variance; `signal/fluid/grid.go:574-591` mean+variance; `signal/fluid/stats.go:9` `sampleQuantile`; `market/cross_section.go:219-221,419` direct `stat.Quantile(0.5,...)`.

- **`signal/manifold/universe.go:307` `medianAbsolute` is a genuine bug, not just duplication.** It sorts the *raw* values and returns `|sorted[mid]|` — i.e. the absolute value of the median — whereas `nomagique/statistic.MedianAbsoluteOf` sorts the *absolute* values first. These differ whenever the input has negatives; the local version is not a median-of-absolutes at all. *(Re-read and confirmed.)*

**Correlation rolled by hand instead of `nomagique/correlation`:** `signal/leadlag/section_correlation.go:222` `pearson` (full hand-rolled mean/stddev/covariance/ratio) duplicates both `correlation/Pearson` and the `gonum stat.Correlation` it wraps; `market/peer_cache.go:61,91` call `gonum stat.Correlation` directly, bypassing the project's `Pearson` wrapper.

**Ring buffers rolled instead of `datura/structure`:** `signal/resonance/feed_ring.go:20` `symbolRing` (hand-rolled fixed-capacity ring with parallel typed columns — the role of `SPSCRing`/`ListRing`); `signal/resonance/baselines.go:10` `scalarRing` (overlaps `statistic.ObservationRing`/`PriceRing`).

**Near-duplicate files:** `kraken/public/websocket.go` (385) vs `replayer.go` (351) are near-verbatim twins (`onMessage`, `Run`, `Error`, `Close`, `Connect`, `disconnect`, `setConnection`, `writeMessage`, …) — and they have already diverged into a bug: `WebSocket.setConnection:331-334` does **not** lock `connMu` while `Replayer.setConnection:292-298` does. `signal/manifold/ticksize.go` vs `signal/fluid/ticksize.go` are byte-identical except package name and one helper. The per-role `trade.go`/`book.go`/`ticker.go` files across signal packages repeat the identical `Measure → parse timestamp → SetTimestamp → RoundTripArtifact → SetOrigin → completeMeasurement` skeleton with only a per-package origin constant differing.

---

## 4. Lock-free & performance

The tick loop (`trader/crypto.go:167`, 100ms) calls `signals.Measure()` → each signal's `Measure` (run **sequentially** in one goroutine, `trader/signal.go:261-273` — the "concurrent signal reads" comment at `:239-241` is aspirational, nothing fans out) → `cross_section` / `peer_cache` / `story`.

**Locks on/near the hot path that should be lock-free:**

- `trader/signal.go:53,55` `pendingMu`/`regimeMu`. `pendingMu` guards a `[]*datura.Artifact` produced in `onMessage` and swapped out in `Measure` — a textbook SPSC hand-off that `datura/structure/spsc.go` (`SPSCRing`) or `mpmc.go` exist to replace. `regimeMu` guards a single `*datura.Artifact` pointer with a cross-goroutine reader (`Regime()` from `crypto.Run`) — replaceable by `atomic.Pointer`.
- `trader/crypto.go:43` `balancesMu sync.RWMutex` guards five scalar fields, read on **every tick** (`:238-244`) plus a defensive per-tick `append([]byte(nil),...)` copy under lock. A snapshot behind `atomic.Pointer[balancesSnapshot]` makes the tick read a single atomic load.
- `market/story.go:23,24` `symbols`/`dirty` are `sync.Map` but `Update` and `ActionsWithTrace` both run single-goroutine (`crypto.go:273-274`), so `sync.Map` only adds boxing + `LoadOrStore` allocation per tick over a plain map.
- Per-symbol `sync.Map` state read/written inside every signal's single-goroutine `Measure`: `resonance/signal.go:97,98`, `feed_ring.go:34`, `baselines.go:99`; `manifold/field_types.go:22` (which even documents "single-owner signal job"), `universe.go:49`; `fluid/registry.go:11`; `leadlag/section.go:32`. Each pays `sync.Map` overhead for concurrency that is never exercised; `leadlag/section.go:110` `ensure` also allocates a throwaway `&symbolState{}` on every `LoadOrStore`.
- `signal/resonance/batch_engine.go:39` `slotRegistry.mutex` on the per-tick settle path (single-writer in practice; the one lock where a lock-free replacement is genuinely non-trivial).

**Concurrency correctness hazards:** `structure.ListRing` is **not thread-safe** (`structure/list.go:69`, plain non-atomic `Push`), yet it is stored in `story.go`'s `sync.Map` that advertises concurrency — safe only because access is currently single-goroutine. `trader/crypto.go` `crypto.state` (`:30`, plain `uint8`) is read/written across goroutines (`:170`, `:209`) **without atomics** — a data race, with a `tick *atomic.Int64` sitting right beside it showing the available pattern. The whole shared-mutable-stage design (one `market.Signal` per source reused across all symbols and ticks, holding plain unsynchronized fields like `leadlag/section.go:33-34 anchorSymbol`/`moveHistory`) is one fan-out away from immediate races — exactly the fan-out the `:239-241` comment says is intended.

**Hot-path allocations (every tick):** `trader/signal.go:135-139` builds a formatted string per elapsed second, then `:161` does `[]byte(role+"/update/"+prefix)` (string concat + fresh `[]byte`) per (prefix × role). `:141,182,183` new maps per tick. `:159-179` `datura.Acquire` per walked frame — and `datura.Acquire` (`datura/artifact.go:18`) is **not pooled**: it allocates a capnp segment, `NewRootArtifact`, a `uuid.NewString()` (`:34`) and three `Set*` calls, while `Release()` (`:160`) is a no-op. This is the single largest per-tick allocation source; `sync.Pool` precedent exists (`datura/transport/copier.go:12`). `:213-218` and `:257-259` run **two `sort.Slice` per tick** (reflection-based, closure alloc) over data that `WalkPrefix` already returns in timestamp order — a k-way merge of the already-sorted role buckets would avoid both. `datura.Peek` (`datura/attributes.go:13`) does a full `sonic` JSON AST parse **per field access** and is called pervasively (`:152,186,188,201,203,262` plus inside every signal), re-parsing the same symbol 2-3× per artifact with no caching.

**Repeated / O(n²) recomputation per tick:** `market/cross_section.go:165` `refreshAggregates` runs on *every* ticker row, looping all symbols and rebuilding `volumes`/`absChanges` from scratch — O(symbols × rows) per tick for incrementally-updatable aggregates; `:143` `refreshConfig` re-runs `ResolveWindows` and allocates a config artifact per row. `MajorityThreshold:218`, `Leader:284`, `leadershipThreshold:420` each `append`-copy and `sort.Float64s` the full buffer per call. `market/peer_cache.go` is O(symbols² × window): `build:74` correlates each peer against a `marketReturns:140-153` that `sort.Float64s` a column *per time index*, and `marketReturnsExcluding:155` re-runs the whole leave-one-out median build per symbol, bypassing the cache — whose `crossSection.version` key is invalidated on every observed row anyway (`cross_section.go:164`). `trader/signal.go:161` issues a separate `WalkPrefix` per role per second-prefix per tick (5×k scans for a k-second tick), when `dmt.Tree.WalkLowerBound` (`tree.go:193`) exists precisely to scan `[role/timestamp, next-role)` without manufacturing every intermediate second prefix.

**Tree access is genuinely lock-free** — `WalkPrefix`/`Seek`/`Get` do a single `atomic.Pointer.Load` over an immutable radix snapshot (`datura/dmt/tree.go:37-49,127,169,323`); `Insert`/`Delete` use a CAS retry loop. The only tree lock is `persistMu` (`tree.go:30`), held across WAL append + root store in the persistent-write branch (`tree_persistence.go:43,89`), which serializes ingest writers when persistence is enabled but does not touch the read-only Measure path. `broker/desk.go:48 treeMu` wraps already-CAS-safe tree ops — redundant serialization unless it is guarding a multi-call read-modify-write (worth confirming at `desk.go:1500,1572`).

**Busy-wait:** `trader/crypto.go:170-211` readiness polls with `time.Sleep(1s)`, re-`RLock`-ing, copying the payload, and rebuilding+JSON-peeking a balances artifact every second until ready — a `close(ready chan struct{})` from `onMessage` removes the spin. If a tick's work exceeds 100ms (plausible given the O(n²) peer work), ticks silently coalesce on `ticker.C`.

---

## Notes on verification

Per `AGENTS.md`, findings that touch code paths need tests/benchmarks run to close them out; this review is static and does not execute the suite. **VERIFICATION LIMITATION: tests and benchmarks were not run as part of this review.** To confirm the correctness findings before acting, the relevant commands would be `go test ./...` in each of the three repos and `go test -bench . -benchmem ./signal/...` and `./trader/...` in symm. Two representative claims (hawkes `organicHeadroomScores` shadowing at `nomagique/algorithm/excitation_symbol.go:466,484`, and manifold `medianAbsolute` at `signal/manifold/universe.go:307`) were re-read directly and confirmed; the remainder are traced from source with quoted lines but not independently executed.
