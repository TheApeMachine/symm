## 1. Short assessment

The current `./tests/` simulator has the right architectural boundary: it replaces the underlying websocket `Conn`, boots the real production stack, generates deterministic data for arbitrary symbol pairs, and allows each pair to transition through controlled regimes. That makes it a strong **end-to-end integration and mechanics harness**.

It is already well suited to validate:

- websocket connection, parsing, routing, and fan-out;
- subscription and symbol isolation;
- event ordering and state transitions;
- order, position, stop-loss, and balance lifecycles;
- recovery, reconciliation, restart, and reconnect behavior;
- deterministic regression tests;
- decision-pipeline behavior under known inputs;
- multi-symbol capacity and orchestration logic.

It is not, by itself, a credible proof of strategy quality. A clean, open-loop generator can demonstrate that the system responds correctly to a scripted `FastPump`, `FlashCrash`, or `SidewaysChop` sequence while saying very little about whether the strategy has an edge in real markets. If fills occur immediately at the current best quote and the market continues independently of the system’s orders, the simulation will systematically understate:

- spread and fee drag;
- latency-induced price movement;
- partial fills and no-fill outcomes;
- market impact and finite liquidity;
- adverse selection;
- queue position;
- correlated cross-asset shocks;
- malformed, duplicated, delayed, or missing exchange events.

The best evolution is therefore **not** to build a miniature Kraken. Keep the current deterministic generator as the foundation, then add selective layers for execution friction, timing uncertainty, coherent market state, latent regimes, cross-symbol relationships, and failure injection.

A useful separation is:

1. **Deterministic mechanics harness** — correctness, recovery, invariants, and fault handling.
2. **Seeded stochastic scenario harness** — robustness of signals, strategy logic, and regime response.
3. **Historical replay** — generalization and performance under actual market paths.
4. **Paper trading and limited live exposure** — venue behavior, operational reality, and live execution economics.

---

## 2. Suitability by validation objective

| Validation objective | Suitability of the current architecture | What it can legitimately establish |
|---|---:|---|
| System mechanics and infrastructure | **High** | The real stack can ingest, route, process, recover from, and reconcile controlled event streams. |
| Order, position, and stop-loss lifecycle | **Medium to high** | Strong if the simulator can generate rejected orders, delayed acknowledgements, partial fills, missing executions, reconnects, and reconciliation drift. |
| Signal implementation | **Medium** | It can test indicator calculations and pipeline wiring, but only if the generated ticker, book, trade, and candle data are coherent. |
| Logic and regime response | **Medium** | It can test whether known observable patterns produce the intended response, provided the regime is not leaked through an explicit label or overly artificial transition. |
| Strategy execution quality | **Low to medium** | It can reject obviously fragile logic and test sensitivity to costs, but not validate real fill quality without an execution model. |
| Strategy profitability | **Low** | Positive synthetic PnL may be an artifact of smooth paths, favorable fills, infinite liquidity, or generator-specific patterns. |
| Generalization to live trading | **Low by itself** | It does not establish robustness against unseen historical paths, current venue behavior, participant adaptation, or real latency. |

The central distinction is:

> The simulator can prove that the system responds correctly to a scenario. It cannot prove that the scenario is representative of the market.

That distinction should be reflected in test names, reports, and release criteria.

---

## 3. Main concerns and recommended improvements

| Concern | Why it matters | Recommended improvement | Validation value |
|---|---|---|---|
| **Open-loop market generation** | The feed continues independently of the system’s orders. A strategy can appear profitable regardless of its order size, timing, or market impact. | Preserve the generator, but add an optional stateful execution layer that consumes simulated liquidity when orders are submitted. Keep the default open-loop mode for mechanics tests. | Separates pure decision testing from executable strategy testing without sacrificing fast deterministic tests. |
| **Clean or atomic fills** | Real orders may be rejected, partially filled, delayed, canceled, or filled across multiple executions. Atomic fills hide position-fragmentation and lifecycle bugs. | Extend `WithAutoFill` with options such as `PartialFillProb`, `MeanFragmentCount`, `FragmentDelayMs`, rejection probability, cancellation, and no-fill behavior. | Tests order aggregation, PnL, stop-loss handling, retries, and close-state correctness. |
| **Fills at the best quote or mid-derived price** | Immediate best-quote fills ignore spread crossing, slippage, queue position, and price movement between decision and execution. | Add a lightweight execution model: marketable orders cross the spread; limit orders fill only when reached; apply configurable slippage and execution delay. | Makes entry economics and exit behavior meaningfully testable without requiring a complete matching engine. |
| **No finite depth or size-dependent impact** | A strategy can assume unlimited liquidity and use the same price for a small and very large order. | Add several depth levels per pair, finite quantities, and a simple impact curve based on order size, depth, volatility, urgency, and regime. | Tests sizing, capacity, spread-plus-fee economics, and liquidity-sensitive behavior. |
| **No queue-position model** | Limit orders may appear to fill merely because the market touched their price, even though they would have been behind existing liquidity. | Start with configurable queue assumptions or fill probabilities. Add full price-time priority only if queue position is a demonstrated failure mode. | Improves execution realism at much lower cost than a full exchange simulator. |
| **No latency model or overly deterministic timing** | The system may decide and fill against the same market state. Real market-data, decision, REST, acknowledgement, and execution delays can invalidate marginal trades. | Add separate seeded latency components for market-data arrival, analysis, order submission, acknowledgement, and execution. Support deterministic replay of each sampled latency. | Exposes stale-decision bugs, timing races, stop-loss gaps, and false edge caused by instantaneous execution. |
| **Clean event ordering** | Real venues produce duplicates, gaps, reordering, delayed private events, stale snapshots, and execution-before-acknowledgement scenarios. | Add first-class fault injection for drops, duplicates, reordering, delays, sequence gaps, malformed messages, reconnects, stale snapshots, and delayed REST reconciliation. | High value for idempotency, recovery, reconciliation, and concurrency correctness. |
| **Incoherent market views** | If ticker, book, trades, candles, and executions are generated independently, the system may consume an impossible market and pass tests for the wrong reasons. | Generate one underlying event stream and derive ticker, book, trades, candles, and execution observations from it. Enforce invariants such as `bid < ask`, precision, timestamps, and volume consistency. | Finds genuine ingestion, parser, sequencing, and signal-consistency defects. |
| **Regimes are too explicit or too smooth** | The strategy may recognize a synthetic transition marker instead of inferring a regime from observable evidence. Smooth trends also make indicators and forecasts look unusually effective. | Keep controllable regime states for test orchestration, but hide labels from the system. Express regimes through returns, volatility, spread, depth, trade intensity, imbalance, jumps, and correlation. Add gradual transitions, false breakouts, whipsaws, and noisy precursors. | Tests actual regime inference and robustness instead of generator recognition. |
| **Only one family of regimes** | A strategy that handles a clean pump/dump sequence may fail in chop, slow drift, compression, liquidity evaporation, or rapid reversals. | Build a regime matrix including baseline, trend, mean reversion, compression, sideways chop, volatility expansion, flash crash, thin liquidity, spoof-like conditions, false breakout, and mixed transitions. | Measures false positives, detection delay, adaptation, and defensive behavior across materially different conditions. |
| **Independent symbol streams** | Independent pairs create artificial diversification and miss portfolio-level correlation, contagion, and factor exposure. | Add market-level latent factors, configurable correlation matrices, leader/follower relationships, anti-correlation, shared volatility shocks, and asynchronous pair transitions. | Tests `Thesis`, capacity, allocation, cross-sectional reasoning, hedging, and portfolio stress behavior. |
| **Identical characteristics for arbitrary pairs** | Pair-specific tick size, liquidity, spread, volatility, volume, fees, and latency affect both signals and execution. | Add per-symbol profiles for price precision, tick size, spread range, depth, volatility, trade intensity, fee tier, latency, and correlation relationships. | Tests symbol registration and prevents accidental dependence on one canonical pair. |
| **No adverse selection or post-fill drift** | A signal may receive a favorable fill and then an unrealistically benign path. Real aggressive orders often precede unfavorable short-term movement. | Add conditional post-fill drift based on aggressor side, volatility, spread, imbalance, and regime. Keep it parameterized and seeded rather than attempting to model all participant behavior. | Tests whether an apparent edge survives being late, paying for urgency, and receiving adverse fills. |
| **Balance is too synchronous with local fills** | If local position state and exchange-owned balance always update together, recovery and reconciliation paths remain untested. | Model exchange-side balance updates independently. Delay, omit, duplicate, or temporarily contradict balance snapshots in controlled scenarios. | Validates wallet truth, boot recovery, insufficient inventory handling, and reconciliation conflict behavior. |
| **Incomplete order lifecycle** | An acknowledgement is not a fill, and a market trade is not necessarily the system’s execution. Conflating them hides state-machine errors. | Represent `submitted`, `acknowledged`, `open`, `partially_filled`, `filled`, `canceled`, `rejected`, and `expired` explicitly. Generate executions consistently but independently from acknowledgements. | Improves validation of broker, position, PnL, stop-loss, and retry logic. |
| **Weak stop-loss stress coverage** | Stop-loss behavior is most important when price gaps through the trigger, spreads widen, fills fragment, or the system restarts. | Test gap-through-stop, spread widening at trigger, partial stop fills, delayed activation, repeated trigger events, failed sell submission, restart while armed, and rebind after fills. | Directly tests protective behavior and prevents false confidence from clean exits. |
| **No null or negative controls** | A strategy can look good simply because every scenario contains a predictive relationship. | Add zero-edge random walks, shuffled return/signal relationships, spread-only movement, wrong-direction signals, edge below fees, false breakouts, and volatility spikes immediately after entry. | Measures false-positive behavior and verifies that the system does not manufacture opportunities. |
| **No parameter or seed robustness testing** | Passing one carefully designed path says little about stability. | Run scenario matrices over seeds, symbols, spreads, fees, latency, volatility, regime duration, transition timing, and order size. Separate development, fixed regression, and hidden evaluation scenarios. | Detects brittleness and generator-specific overfitting. |
| **No statistical calibration** | Synthetic data can look plausible while having incorrect tails, volatility clustering, inter-arrival times, imbalance persistence, or cross-pair correlations. | Compare generated distributions with historical reference data: returns, tails, volatility clustering, spread percentiles, depth, trade sizes, arrival intensity, imbalance, jump frequency, and correlations. | Establishes whether the stochastic model is suitable for robustness experiments. |
| **Incomplete economic accounting** | Ignoring fees, precision, rounding, residual balances, and mark-price conventions overstates PnL. | Reconcile order-level cash and asset movements against position PnL, fees, rounding, dust, and exchange-style balances. Report gross and net results separately. | Validates financial correctness independently of strategy quality. |
| **No meaningful quality reporting** | “Test passed” does not reveal whether a strategy was challenged or merely benefited from a favorable path. | Record regime exposure, detection delay, order outcomes, fill ratio, latency, slippage, false positives, drawdown, adverse/favorable excursion, capacity, and invariant violations. | Makes simulator results auditable, comparable, and difficult to misinterpret. |

A useful implementation principle is to separate **scenario identity** from **randomness**. A scenario should define the market structure and intended regime schedule; a seed should determine the reproducible perturbations. Every failure should persist the complete scenario configuration, seed, transition timeline, injected faults, and generated event log.

---

## 4. What belongs in each validation layer

### A. The deterministic simulator

The deterministic mode should remain intentionally controllable and can be more adversarial than realistic. Use it for:

- connection establishment and subscription;
- symbol routing and per-pair isolation;
- ticker, book, trade, candle, execution, and balance parsing;
- sequence numbers, timestamps, snapshots, and deltas;
- duplicate, missing, malformed, stale, delayed, and out-of-order events;
- reconnect and resubscription;
- persistence and restart recovery;
- order state transitions;
- rejected, canceled, partial, and delayed fills;
- balance reconciliation;
- position capacity exhaustion;
- stop-loss arming, ratcheting, triggering, persistence, and rebind;
- audit ordering and completeness;
- fan-out cleanup after position closure;
- readiness transitions;
- deterministic decision-pipeline routing;
- invariants such as no duplicate PnL, no orphan positions, no negative balances outside explicitly modeled cases, and no leaked subscriptions.

For mechanics, realism is secondary to **control, fault coverage, and reproducibility**.

### B. Seeded stochastic simulation

Use a richer seeded mode for:

- signal sensitivity to noise;
- regime classification and transition behavior;
- spread, fee, latency, and slippage sensitivity;
- order sizing and capacity;
- partial fills and no-fill outcomes;
- cross-symbol and portfolio logic;
- null and adversarial controls;
- robustness across seeds and parameter perturbations;
- controlled behavior of signal components such as order-flow imbalance, CVD, Hawkes-like clustering, depth flow, correlation, and lead/lag;
- metamorphic properties.

Examples of useful metamorphic assertions include:

- increasing fees should not improve net PnL;
- increasing latency should not systematically improve entry quality;
- widening spreads should reduce actionable opportunities;
- removing predictive structure should move performance toward the null baseline;
- doubling order size should not reduce modeled impact;
- a worse fill should not improve realized economics;
- forecast confidence or readiness should degrade after repeated forecast error.

The target here is not “the strategy makes money.” It is “the strategy’s behavior degrades in understandable ways as conditions become less favorable.”

### C. Historical replay

Historical replay should be the main tool for testing whether the strategy generalizes beyond scenarios authored by the engineering team. Use it for:

- actual return paths and volatility clustering;
- real spread and depth behavior;
- real trade sizes and arrival rates;
- unanticipated regime transitions;
- false breakouts, gaps, and news-driven moves;
- cross-pair correlations;
- out-of-sample and walk-forward evaluation;
- parameter stability;
- signal calibration, including Hawkes or other event-intensity models;
- drawdown, tail behavior, and strategy attribution.

A candle-only backtest is insufficient for microstructure-sensitive logic. A practical hierarchy is:

1. trade or ticker replay for broad signal and lifecycle behavior;
2. top-of-book replay for spread crossing and basic execution;
3. level-2 replay for depth and impact;
4. level-3 or venue-specific replay when queue position is central to the strategy.

Replay still needs explicit fill assumptions. Historical prices tell you what happened, not whether your hypothetical order would have filled. Document assumptions for latency, queue position, partial fills, cancellation, market impact, and stop execution through gaps.

### D. Paper trading and limited live exposure

Do not trust the simulator or replay alone for:

- current exchange availability and operational behavior;
- real REST and websocket latency;
- rate limits and authentication failures;
- actual queue position;
- exchange maintenance and partial outages;
- unacknowledged orders during disconnects;
- live balance discrepancies;
- production clock synchronization;
- monitoring and alerting;
- current fill quality and adverse selection;
- whether the edge persists after deployment.

Paper trading against real market data is the bridge between replay and capital. It should validate the complete deployed loop, including real connectivity, timing, monitoring, reconciliation, and simulated order decisions. After that, initial live exposure should be small enough that an incorrect execution model cannot create unacceptable loss.

---

## 5. Competing approaches and tradeoffs

### Keep the simulator narrow and deterministic

**Advantages**

- fast CI execution;
- easy failure reproduction;
- low maintenance;
- excellent for infrastructure and state-machine tests;
- easy to inject pathological event sequences.

**Disadvantages**

- weak evidence for strategy robustness;
- high risk of overfitting to scripted patterns;
- no direct evidence about real market economics.

This should remain the foundation.

### Add a richer seeded stochastic market model

**Advantages**

- supports broad scenario sweeps;
- preserves reproducibility;
- exercises regime ambiguity, noise, correlations, and execution friction;
- useful for sensitivity and robustness testing.

**Disadvantages**

- requires calibration;
- increases implementation and debugging complexity;
- can create plausible-looking but still incorrect markets;
- may become an optimization oracle if strategy parameters are tuned against it.

This is the right extension, but only to the level justified by concrete testing needs.

### Build a stateful local order book

A lightweight stateful local book is a reasonable middle ground:

- maintain finite depth per symbol;
- cross marketable orders through the spread;
- consume depth based on order size;
- support partial fills and delayed executions;
- optionally adjust subsequent quotes after simulated consumption.

This is much more useful than a best-quote fill, but it should not automatically become a full price-time-priority exchange. Detailed queue simulation is justified only if queue position or maker execution is central to the system’s strategy.

### Build a full matching or agent-based exchange simulator

This is generally not worth doing initially. A complete matching engine, venue-specific rule set, strategic participant population, or HFT/adverse-selection model introduces many uncalibrated parameters and can create an illusion of realism. Historical replay and paper trading usually provide better evidence at lower cost.

---

## 6. Prioritized roadmap

### Near-term: preserve the current architecture and close the highest-value gaps

1. **Formalize the simulator contract**

   Document:

   - supported event types;
   - synchronous versus scheduled behavior;
   - ordering guarantees;
   - timestamp and sequence semantics;
   - snapshot versus delta behavior;
   - balance and execution semantics;
   - supported fault modes;
   - what the simulator intentionally does not model.

2. **Introduce a seeded scenario configuration**

   Include:

   - seed;
   - symbol profiles;
   - initial prices;
   - tick sizes and precision;
   - fees;
   - regime schedule;
   - volatility;
   - spread;
   - depth;
   - trade intensity;
   - latency;
   - fault injection;
   - order-size and capacity settings.

   Persist this configuration with every failed test.

3. **Add adversarial timing and transport faults**

   Implement configurable, reproducible:

   - latency and jitter;
   - delayed acknowledgements;
   - duplicate messages;
   - dropped messages;
   - out-of-order events;
   - sequence gaps;
   - stale snapshots;
   - reconnects;
   - execution-before-acknowledgement;
   - delayed REST balance updates.

   This is likely the highest-leverage infrastructure improvement.

4. **Add a lightweight execution model**

   Extend `WithAutoFill` rather than replacing it. Support:

   - spread crossing;
   - configurable slippage;
   - delayed execution;
   - partial fills;
   - no-fill limit orders;
   - rejection;
   - cancellation;
   - gap-through-stop behavior.

5. **Add invariants and accounting reconciliation**

   Assert properties such as:

   - duplicate events do not duplicate fills or PnL;
   - closed positions cannot receive later fills;
   - stop-loss floors do not move in the wrong direction;
   - execution quantity does not exceed order quantity;
   - balances reconcile to fills and fees;
   - PnL reconciles to cash, asset movements, fees, precision, and rounding;
   - no orphan positions or leaked subscriptions remain after scenario completion.

6. **Strengthen regime fixtures**

   Keep explicit states and precursor contracts, but add:

   - configurable precursor length;
   - volatility ramps;
   - volume and liquidity absorption;
   - spread jitter;
   - noisy baselines;
   - gradual transitions;
   - false breakouts;
   - whipsaws;
   - abrupt shocks;
   - rapid regime reversals.

7. **Add null and negative controls**

   Include scenarios where:

   - returns are random;
   - signal and return relationships are shuffled;
   - the forecast edge is below fees;
   - spread widens without directional opportunity;
   - the signal is systematically wrong;
   - volatility spikes immediately after entry.

8. **Separate mechanics and economics in reports**

   Report infrastructure pass/fail independently from:

   - decision output;
   - order acceptance;
   - fill ratio;
   - gross PnL;
   - fees;
   - slippage;
   - net PnL;
   - drawdown;
   - false positives;
   - invariant violations.

### Medium-term: make the harness useful for strategy and regime robustness

1. **Add coherent market generation**

   Generate a single underlying event process and derive book, ticker, trades, candles, and executions from it. Add heavy-tailed returns, clustered volatility, jumps, mean reversion, volatility-of-volatility, and realistic trade-arrival behavior where useful.

2. **Add latent regimes**

   Regime labels should remain available to the test oracle but never to the system under test. Define regimes through observable distributions of:

   - returns;
   - volatility;
   - spread;
   - depth;
   - trade intensity;
   - order-flow imbalance;
   - jump probability;
   - correlation;
   - liquidity;
   - lead/lag behavior.

3. **Add multi-symbol structure**

   Support:

   - positively and negatively correlated pairs;
   - independent pairs;
   - leader/follower relationships;
   - shared volatility shocks;
   - liquidity divergence;
   - asynchronous regime transitions;
   - correlated drawdowns.

4. **Add size-dependent depth and impact**

   A full exchange replica is unnecessary. A few depth tiers, finite quantities, spread crossing, and a simple impact curve will capture much of the relevant execution risk.

5. **Add post-fill drift and adverse selection**

   Make post-fill movement conditionally worse in situations such as:

   - aggressive entry during high toxicity;
   - thin liquidity;
   - large spread;
   - high volatility;
   - late entry into a trend.

   Keep the effect parameterized and seed-controlled.

6. **Calibrate the generator against historical data**

   Compare generated and observed distributions for:

   - log returns and tails;
   - volatility clustering;
   - spread percentiles;
   - depth;
   - trade sizes;
   - inter-arrival times;
   - order-flow imbalance;
   - jump frequency;
   - regime duration;
   - cross-pair correlations.

   Calibration should produce a report rather than relying on visual chart inspection.

7. **Add robustness and walk-forward scenario sets**

   Maintain separate:

   - development scenarios;
   - fixed regression scenarios;
   - hidden evaluation scenarios;
   - historical replay scenarios.

   Do not tune parameters and perform final evaluation on the same synthetic family.

8. **Add regime-response scorecards**

   Measure:

   - regime detection delay;
   - false-transition rate;
   - forecast calibration;
   - decision frequency;
   - entry rejection rate;
   - stop-loss rate;
   - average adverse excursion;
   - average favorable excursion;
   - net economics after costs;
   - capacity utilization;
   - recovery correctness.

### Probably not worth it initially

- A full Kraken exchange replica.
- Complete price-time-priority matching for every order type.
- Agent-based simulation of market makers, arbitrageurs, and liquidators.
- A high-fidelity HFT or queue simulator without evidence that maker queue position is central.
- Dynamic market behavior that reacts deeply to the system’s own trading.
- Margin, liquidation, funding, and cross-venue mechanics unless those are core to the deployed strategy.
- ML-generated synthetic markets as the primary validation layer.
- GPU optimization before scenario generation is shown to be a real bottleneck.
- Optimizing strategy parameters directly for synthetic PnL.

The practical target is a **layered harness**, not maximum simulation fidelity.

---

## 7. “Good enough” criteria

### Good enough for mechanics validation

The simulator is good enough for infrastructure when:

- the real production stack boots and runs through the actual websocket boundary;
- all supported event types can be generated deterministically;
- ordering, timestamps, and sequence semantics are explicit;
- duplicates, drops, delays, reordering, reconnects, stale data, and malformed messages are testable;
- orders can be acknowledged, rejected, canceled, partially filled, delayed, and reconciled;
- balance truth can diverge from local state and recover;
- restart and recovery scenarios pass;
- position, PnL, stop-loss, audit, readiness, and subscription invariants hold;
- multi-pair and capacity-exhaustion scenarios are covered;
- the same scenario and seed reproduce the same result;
- failures preserve enough artifacts to replay exactly.

At this level, the market does not need to be statistically realistic. **Control and adversarial coverage matter more than realism.**

### Good enough for strategy validation

The simulator is good enough for strategy experimentation—but not certification—when:

- book, ticker, trades, candles, and executions are internally coherent;
- spread, fees, latency, slippage, finite depth, partial fills, and no-fill outcomes are modeled;
- regimes are latent and observable only through market data;
- null and adversarial regimes are included;
- results are evaluated across many seeds, symbols, regime durations, and parameter perturbations;
- strategy results are reported net of fees and execution costs;
- performance is compared with simple baselines and a no-edge control;
- parameters are tested out of sample;
- generated distributions have been compared with historical reference distributions;
- results are stable across scenario families rather than dependent on one scripted path;
- worsening friction generally worsens economics rather than producing paradoxical improvements.

The correct conclusion is:

> “The strategy is robust within the tested synthetic distribution,”

not:

> “The strategy has been validated for live profitability.”

Historical replay is still required for generalization, and paper trading is still required for operational and venue validation.

### Good enough for logic and regime-response validation

The simulator is good enough for this purpose when:

- each regime has explicit observable characteristics;
- transitions include gradual changes, abrupt shocks, false breaks, whipsaws, and mixed states;
- the system cannot see the internal regime label;
- the test oracle can compare behavior with the intended regime;
- detection delay and false-transition rates are measured;
- signals, readiness, confidence, allocation, and stop-loss posture change as expected;
- the system behaves safely when the forecast is wrong;
- cross-symbol relationships and asynchronous transitions are tested;
- results remain stable across seeds and regime durations;
- the system rejects no-edge and below-fee scenarios;
- metamorphic expectations hold—for example, greater latency and wider spreads reduce actionable edge.

That validates the **decision logic and state transitions**, not the existence of durable alpha.

The core recommendation is to keep the current simulator’s deterministic websocket-level architecture, add a small number of high-value execution and fault models, and make the boundary between **mechanics correctness**, **logic robustness**, and **strategy profitability** explicit in both the code and the test reports.