## 1. Executive summary

The system’s main weakness is not the sophistication of its signal stack. It is the conversion of that sophistication into a small number of hard, weakly calibrated trading decisions.

The broad design—market data → signals → forecast → strategy selection → allocation → execution—is strategically sensible. The system also has several good foundations:

- explicit separation between normal and reserved capacity;
- a planner admission threshold of **0.80**;
- forecast downside being considered alongside expected return;
- explicit entry economics involving spread, fees, and impact;
- per-symbol adaptive forecasting;
- graph-based support and contradiction handling;
- deterministic simulation as a mechanics harness.

However, the current design appears to compress too much information into scalar values and binary gates. The most important examples are:

```text
confidence - coherenceMag2
```

and:

```text
ExpectedReturn - WorstIntermediateDrawdown()
```

followed by hard thresholds and fixed-fraction sizing.

Those calculations may be numerically valid as code, but they are not yet demonstrably valid as statistical or economic decision variables. The system is therefore at risk of:

- treating heterogeneous quantities as if they were comparable;
- double-counting correlated evidence;
- accepting gross edge that disappears after execution costs;
- assigning the same strategic meaning to confidence across symbols and regimes;
- using stale or uncertain model outputs as if they were current;
- sizing by account fraction rather than marginal portfolio risk;
- reserving capital for internally generated “cognitive lead” without proving incremental net expectancy;
- optimizing MCTS or downstream logic against an open-loop market model.

The most material distinction is this:

- **Mechanics correctness** asks whether the calculations and state transitions behave as coded.
- **Forecast validity** asks whether the signals predict future outcomes out of sample.
- **Strategy validity** asks whether the planner converts those forecasts into positive net expectancy.
- **Execution validity** asks whether that expectancy survives latency, partial fills, adverse selection, impact, and portfolio constraints.

These should not be collapsed into one confidence or one test result.

A useful target representation for every candidate trade is not a single confidence score, but a conditional distribution:

\[
\left(
E[R_{\text{net}}],
P(R_{\text{net}}<0),
ES_\alpha,
P(\text{fill}),
T_{\text{hold}},
E[\text{MAE}],
E[\text{MFE}],
\text{signal age},
\text{marginal portfolio risk}
\right)
\]

where every quantity is conditioned on:

- symbol;
- direction;
- holding horizon;
- market regime;
- information timestamp;
- intended order type and size;
- current portfolio exposure.

On the evidence available, I would classify the system as **strategically valid in intent but materially under-calibrated and under-modeled in execution and portfolio economics**. Several findings below are objectively concerning; others require source-level confirmation before being called outright bugs.

---

## 2. Objective math errors

### 2.1 `CognitiveLead = Confidence - CoherenceMag2` is not a well-defined statistical comparison

**Relevant area:** `strategy/allocation.go`, `classifyAllocation`

The allocation logic reportedly computes a lead measure equivalent to:

```text
cognitiveLead = cognition.Confidence - thesis.Manifold.CoherenceMag2
```

This is the most clearly questionable formula in the visible strategy layer.

`Confidence` appears to be probability-like or score-like. `CoherenceMag2`, by name, appears to be a squared magnitude or energy measure. Unless both values have been explicitly normalized, calibrated, and defined on the same scale, subtracting them has no clear statistical interpretation.

The fact that the result is numerically bounded or easy to threshold does not make it meaningful. A squared magnitude is not automatically a probability, and a probability is not automatically comparable to an energy-like quantity.

#### Strategic impact

This can produce:

- arbitrary reserve-slot admission;
- unstable flips near zero;
- symbol-independent bias if coherence is global;
- excessive confidence when the manifold scale changes;
- suppression of valid opportunities when a global coherence value is large for unrelated reasons.

A small change in `CoherenceMag2` can flip the allocation class without representing a meaningful change in expected trading outcome.

#### Improvement

Replace the subtraction with a calibrated, common-domain quantity. For example:

\[
\text{lead}
=
P(\text{trade succeeds}\mid \text{cognition, regime, execution})
-
P(\text{trade succeeds}\mid \text{baseline, regime, execution})
\]

Alternatively, estimate a standardized lift:

\[
\text{lead}
=
\frac{
\text{observed cognitive score}
-
E[\text{score}\mid \text{baseline}]
}{
\operatorname{SD}[\text{score}\mid \text{baseline}]
}
\]

At minimum, normalize coherence by symbol and regime:

\[
\text{normalizedCoherence}
=
\frac{
\text{CoherenceMag2}-\mu_{s,r}
}{
\sigma_{s,r}
}
\]

Then calibrate the resulting score against a precise outcome definition, such as:

> “The trade reaches +X net basis points before −Y basis points within H minutes after costs.”

Do not use the word “confidence” unless a confidence of **0.80** means approximately 80% success for comparable, timestamped, out-of-sample cases.

---

### 2.2 `OpportunityMargin = ExpectedReturn - WorstIntermediateDrawdown()` is not a complete edge calculation

**Relevant area:** `strategy/allocation.go`, `classifyAllocation`

The formula:

```text
opportunityMargin = expectedReturn - worstIntermediateDrawdown
```

can be a useful conservative heuristic, but only under strict conditions:

1. both terms refer to the same instrument and direction;
2. both use the same horizon;
3. both use the same price basis;
4. expected return is already net of all execution costs;
5. drawdown is not being double-counted elsewhere;
6. the drawdown statistic has a defined confidence level.

Otherwise, the subtraction combines unlike quantities.

For example, expected return may be a mean terminal return while worst intermediate drawdown is a path statistic. A trade may have a positive terminal expectation but a high probability of breaching a stop before eventually recovering. Conversely, subtracting a pessimistic path statistic from an expected value may discard profitable trades without properly representing their distribution.

#### Strategic impact

The current formula may:

- approve trades that are positive only before costs;
- reject trades with favorable risk-adjusted distributions;
- treat a single worst-path estimate as representative;
- fail to distinguish high-probability small gains from low-probability large gains;
- ignore execution failure and gap risk.

#### Improvement

Use a net return distribution and a separate risk constraint:

\[
E[R_{\text{net}}]
=
E[R_{\text{gross}}]
-
E[C_{\text{entry}}+C_{\text{exit}}]
\]

with:

\[
C =
C_{\text{fees}}
+
C_{\text{spread}}
+
C_{\text{slippage}}
+
C_{\text{impact}}
+
C_{\text{latency}}
\]

Then require conditions such as:

\[
E[R_{\text{net}}] > r_{\min}
\]

\[
P(R_{\text{net}}<0) < p_{\max}
\]

\[
ES_{\alpha}(R_{\text{net}}) > -L_{\max}
\]

and:

\[
P(\text{stop or risk breach before target}) < b_{\max}
\]

A simple first improvement would be:

```text
netEdge =
    expectedReturn
    - expectedFees
    - expectedSpreadCost
    - expectedSlippage
    - expectedImpact
    - tailRiskPenalty
```

with the terms explicitly defined in either fractional-return or currency units.

---

### 2.3 Entry economics must use executable prices and round-trip costs

**Relevant areas:** `broker/entry_economics.go`, `broker/price.go`, strategy allocation.

The entry calculation reportedly compares forecast, spread, fees, and impact in midpoint units. That is only correct if all terms share:

- the same units;
- the same direction;
- the same horizon;
- the same one-way or round-trip basis;
- the same execution assumptions.

For a long trade, the correct economic object is closer to:

\[
E[\text{PnL}]
=
q\left(
E[P_{\text{exit}}^{\text{exec}}]
-
P_{\text{entry}}^{\text{exec}}
\right)
-
F_{\text{entry}}
-
F_{\text{exit}}
-
I_{\text{entry}}
-
I_{\text{exit}}
\]

The entry should generally use the ask or expected aggressive execution price, not the midpoint. The exit should use the bid or the expected execution distribution. If the trade is passive, then the model must include fill probability and adverse selection.

Subtracting “the spread” once is not equivalent to modeling entry and exit execution. A marketable round trip can incur spread costs twice, while a passive order may pay less spread but face non-fill and adverse-selection risk.

#### Strategic impact

This is especially important for:

- short-horizon trades;
- wide-spread symbols;
- small forecast edges;
- high-turnover strategies;
- large orders relative to visible depth;
- periods of elevated toxicity.

#### Improvement

Represent execution economics explicitly:

```text
entrySide
exitSide
entryPriceDistribution
exitPriceDistribution
entryFee
exitFee
spreadCost
expectedSlippage
marketImpact
latencyDecay
partialFillProbability
roundTripNetReturn
```

Reject or reduce trades when:

\[
P(\text{net PnL}>0)
\]

or expected utility falls below the required threshold after all costs.

---

### 2.4 Fixed-fraction sizing is not risk-normalized

**Relevant area:** `strategy/allocation.go`, `size`

The visible sizing logic appears to use something like:

```text
notional = cash * maxFraction
```

with:

```text
maxFraction = 0.20
```

A 20% notional cap is not a 20% risk cap. Two instruments with identical notional sizes can have very different:

- volatility;
- stop distance;
- liquidity;
- gap risk;
- spread;
- market impact;
- correlation with existing positions.

#### Strategic impact

Fixed-fraction sizing can:

- oversize volatile or illiquid symbols;
- undersize stable, liquid, high-edge opportunities;
- ignore signal uncertainty;
- ignore stop distance;
- ignore existing correlated exposure;
- create unstable portfolio drawdowns across regimes.

#### Improvement

Use a constrained risk-budget formula:

\[
q_{\text{risk}}
=
\frac{B_{\text{risk}}}
{\text{stopDistance}\times \text{pointValue}}
\]

Then cap it by liquidity and portfolio constraints:

\[
q
=
\min(
q_{\text{risk}},
q_{\text{notional}},
q_{\text{liquidity}},
q_{\text{correlation}},
q_{\text{drawdown}}
)
\]

A practical implementation can use:

```text
riskBudget
stopDistance
forecastUncertainty
expectedSlippage
participationLimit
marginalPortfolioES
```

Fractional Kelly can be evaluated later, but only after return distributions and drawdown behavior are reliable. Unconstrained Kelly sizing would be inappropriate here.

---

### 2.5 RLS claims require source confirmation; the important issue is uncertainty and adaptation

**Relevant area:** `logic/resonance/solver.go`, `logic/resonance/state.go`

One response asserted that RLS lacks forgetting and will suffer covariance windup. That is a serious possibility, but it should not be treated as confirmed without inspecting the actual update equations. If a forgetting factor, covariance reset, or regularization already exists, that specific criticism would be wrong.

The broader issue is definitely material: an adaptive return learner must explicitly handle:

- heteroskedastic returns;
- irregular observation intervals;
- overlapping labels;
- delayed outcomes;
- feature/label timestamp alignment;
- regime-dependent decay;
- covariance conditioning;
- forecast uncertainty.

For an \(h\)-period label:

\[
r_{t,h}=\log(P_{t+h}/P_t)
\]

successive labels overlap heavily:

\[
r_{t+1,h}=\log(P_{t+1+h}/P_{t+1})
\]

Treating these as independent observations can make the learner appear more effective than it is.

#### Strategic impact

The model may confuse:

- volatility changes with directional skill;
- repeated observations with independent evidence;
- stale historical relationships with current edge;
- increased sample count with increased information.

#### Improvement

Verify and test:

- exponential forgetting, possibly regime-dependent;
- covariance regularization;
- volatility-normalized targets;
- purged and embargoed validation;
- non-overlapping or appropriately weighted labels;
- forecast expiry;
- out-of-sample calibration by horizon and regime.

Track at least:

- directional accuracy;
- magnitude calibration;
- Brier score for event probabilities;
- forecast decay by age;
- net expectancy by confidence decile;
- turnover-adjusted PnL.

---

### 2.6 Normalization and denominator handling can manufacture signals

**Relevant areas:** `utils/math.go`, signal modules, percentile and cross-sectional calculations.

Signals such as RVOL, z-scores, liquidity scarcity, breadth, and toxicity can become misleading when:

- the denominator is zero or near zero;
- the reference window is stale;
- the sample count is too small;
- volatility is regime-dependent;
- outliers dominate the mean and standard deviation;
- missing data is converted to zero or a neutral-looking value.

For example:

\[
z=\frac{x-\mu}{\sigma}
\]

is unstable for small \(\sigma\), while:

\[
RVOL=\frac{V_t}{\bar V}
\]

is not comparable across symbols if \(\bar V\) has different session, liquidity, or regime properties.

#### Improvement

Every normalized signal should retain:

```text
rawValue
referenceWindow
sampleCount
dispersionEstimate
outlierPolicy
missingness
staleness
effectiveSampleSize
```

Use median/MAD, winsorization, robust exponentially weighted baselines, minimum sample requirements, and explicit `UNREADY` states rather than silently substituting zero.

---

### 2.7 Correlation support semantics need correction or clarification

**Relevant area:** `signal/correlation/hayashi.go`, `supportedCorrelation`, `hayashiMoments`

A reported issue concerns the use of:

```text
support = min(leftCount, rightCount)
```

while covariance and variance sums are accumulated over all overlapping pairs.

This is not necessarily an algebraic error. It may simply mean that `support` is intended to represent the number of usable observations on the scarcer side rather than the number of paired returns. But if downstream confidence or weighting interprets `support` as effective paired sample size, then the statistic is internally misdescribed.

The more important question is what the correlation actually uses:

- number of paired observations;
- number of unique intervals;
- number of valid observations per side;
- effective sample size after serial dependence.

A single overlapping pair is not equivalent to two independent observations, and highly asynchronous data should not be assigned confidence based only on raw count.

#### Improvement

Return separate fields:

```text
pairedCount
leftCount
rightCount
effectiveSampleSize
overlapDuration
dispersion
correlation
uncertainty
```

Use a minimum paired-count rule and uncertainty estimate. Do not “fix” sparse correlation by returning a correlation from one overlap. A better behavior is to return a provisional estimate with very low weight or an explicit `UNREADY` status.

The suggestion to loosen the gate to one overlap would increase availability but would likely worsen false signals. Availability should be improved through partial weighting, not by pretending that insufficient data is statistically adequate.

---

## 3. Logic flaws and inconsistencies

### 3.1 The planner is binary where the market is continuous

**Relevant areas:** `strategy/planner.go`, `strategy/allocation.go`

The system appears to have:

- admission at `confidence >= 0.80`;
- allocation into normal or reserved;
- positive or non-positive opportunity margin;
- positive or non-positive cognitive lead;
- fixed normal and reserved capacity.

This creates discontinuities. A score of **0.80** is admitted while **0.79** is rejected, despite the two values being practically indistinguishable unless the threshold is calibrated and the underlying score is well-defined.

Likewise, a trade barely above a margin threshold may be treated similarly to a trade with substantially better net utility.

#### Impact

- excessive sensitivity near thresholds;
- lost ranking information;
- lower capital efficiency;
- regime-dependent over-filtering;
- unstable trade frequency.

#### Improvement

Use stages with distinct purposes:

1. **Validity gate:** Is data current and the strategy state admissible?
2. **Economic gate:** Is net edge positive after costs and risk?
3. **Ranking:** How does this candidate compare with alternatives?
4. **Sizing:** How much marginal risk should it receive?
5. **Execution policy:** How should it be implemented?

Use continuous scores for ranking and sizing. Retain hard gates only for safety constraints.

---

### 3.2 Thresholds are not clearly calibrated to realized outcomes

**Relevant areas:** `strategy/planner.go`, `strategy/allocation.go`

The explicit **0.80** admission threshold is real according to the associated tests: **0.79** is rejected and **0.80** is accepted. That confirms implementation behavior, not predictive validity.

The relevant question is:

> Among comparable out-of-sample cases admitted at confidence 0.80, what fraction actually meets the defined success criterion after costs?

If that answer is not approximately 80%, then the value is a score or heuristic, not a probability.

#### Improvement

Calibrate separately by:

- symbol;
- regime;
- horizon;
- direction;
- execution mode;
- signal age;
- liquidity state.

Use reliability diagrams, Brier score, log loss, and net PnL by score decile. Thresholds should move based on calibration and capacity, not remain universal by default.

---

### 3.3 The system may double-count correlated signals

**Relevant areas:** signal aggregation, `strategy/graph.go`, `logic/graph/solver.go`

CVD, depth flow, trade imbalance, toxicity, liquidity, Hawkes excitation, and short-term price action can all be correlated manifestations of the same underlying order-flow event.

If each produces a support edge, the graph may implicitly treat them as independent confirmations:

\[
P(H\mid E_1,\dots,E_n)
\propto
P(H)\prod_i P(E_i\mid H)
\]

even though:

\[
P(E_1,\dots,E_n\mid H)
\neq
\prod_iP(E_i\mid H)
\]

This inflates confidence precisely during synchronized events, when many features move together.

#### Improvement

Group evidence into latent families:

- order flow;
- liquidity;
- volatility;
- price action;
- cross-sectional;
- sentiment;
- execution state.

Then use:

- regularized logistic regression;
- covariance-adjusted evidence;
- Bayesian model averaging;
- a calibrated meta-model;
- or capped contribution per family.

The graph should preserve provenance and contradiction structure, not automatically turn every edge into an independent vote.

---

### 3.4 Contradictions should affect different decision dimensions

A liquidity contradiction does not necessarily invalidate direction. It may instead invalidate:

- size;
- order type;
- urgency;
- expected fill;
- stop reliability.

Therefore, a single scalar support/contradiction score is too blunt.

Use separate quantities:

\[
P(\text{direction})
\]

\[
P(\text{fill})
\]

\[
P(\text{favorable excursion})
\]

\[
P(\text{risk breach})
\]

A toxicity spike should typically reduce allowable size or shift the execution policy before it reverses a directional forecast.

---

### 3.5 Readiness, validity, confidence, and execution readiness are different states

**Relevant areas:** `types/readiness.go`, `types/measurement.go`, `strategy/planner.go`

A `Ready` flag is useful, but readiness does not mean:

- the data is fresh;
- the model is calibrated;
- the state is inside the model’s training domain;
- the execution connection is healthy;
- the signal has sufficient lead time;
- required peer data is available.

Use orthogonal states:

```text
dataReadiness
modelValidity
forecastConfidence
executionReadiness
riskCapacity
```

A trade should require the relevant combination, but one state should not stand in for the others.

Also preserve signal-specific validity:

```text
depthflow: invalid
sentiment: stale
resonance: valid
execution: degraded
```

This is better than linearly reducing one aggregate confidence score.

---

### 3.6 Sequential dependency may compound errors and latency

**Relevant area:** `logic/analyzer.go`

The reported sequence is broadly:

```text
category
→ manifold
→ resonance
→ causal
→ cognition
→ graph
→ planner
```

There are two separate risks.

#### Statistical risk

If category classification is wrong, later modules may condition on the wrong regime. If the manifold is unstable, resonance may learn from unstable features. Later causal and graph modules may rationalize rather than correct the error.

#### Timing risk

If each stage uses a slightly stale snapshot, the final thesis may combine:

- category from time \(t\);
- manifold from \(t+\Delta_1\);
- resonance from \(t+\Delta_2\);
- graph from \(t+\Delta_3\);

without clearly enforcing a common information timestamp.

The claim that the pipeline necessarily takes “hundreds of milliseconds” cannot be accepted without profiling, but the risk is real for short-horizon microstructure strategies.

#### Improvement

Use explicit timestamps and dynamic time-alignment (there may be some work in /Users/theapemachine/go/src/github.com/theapemachine/nomagique/adaptive/time_elastic.go that could be useful, otherwise if not, something in those lines).

Run independent signal families in parallel where possible. Do not require every downstream module to wait for every upstream module. A meta-layer should consume independently produced estimates and uncertainty, rather than treating the first classifier as ground truth.

---

### 3.7 Reserved capacity based on cognitive lead is a valid hypothesis, not a validated privilege

**Relevant area:** `strategy/allocation.go`

The “reserved” lane is conceptually interesting: it attempts to enter before conventional physical confirmation when an internal sequence or cognitive model anticipates an event.

But it creates an asymmetry: an abstract internal state receives scarce capital despite having less direct confirmation from current executable market conditions.

This can be justified only if the reserved strategy demonstrates incremental, out-of-sample, net-of-cost value:

\[
\Delta E
=
E[\text{PnL}\mid \text{cognitive lead}]
-
E[\text{PnL}\mid \text{physical baseline}]
\]

measured after:

- early-entry adverse movement;
- latency;
- spread;
- impact;
- failed fills;
- stop behavior.

#### Improvement

Treat reserved entries as a separate strategy sleeve with:

- independent risk budget;
- separate calibration;
- maximum holding time;
- separate stop and exit policy;
- performance attribution;
- kill switch;
- maximum share of total portfolio risk.

“Reserved” should mean “empirically validated early information,” not “less evidence required.”

---

### 3.8 Capacity limits do not control portfolio risk

**Relevant areas:** `strategy/allocation.go`, slot management

The apparent constraints—**2 normal positions**, **2 reserved positions**, and **20% maximum fraction**—control count and notional, but not aggregate risk.

Four positions can share:

- BTC beta;
- USD or stablecoin risk;
- a sector factor;
- a liquidity regime;
- a liquidation cascade;
- a common exchange or venue dependency.

#### Improvement

Select candidates by marginal portfolio utility:

\[
\Delta U_i
=
E[\Delta R_i]
-
\lambda \Delta ES_i
-
\gamma \Delta \text{drawdown}_i
-
\eta \Delta \text{capacity}_i
\]

Use covariance or factor exposure for ordinary conditions, and correlation-aware expected shortfall for stressed conditions.

---

### 3.9 Execution and recovery state can alter strategy behavior

**Relevant areas:** `broker/recovery.go`, `broker/position_store.go`, stop and position state

Operational state is part of the strategy if it determines available capital, exposure, or stop protection.

If local state and exchange state disagree, the planner may:

- double-enter;
- size on nonexistent free cash;
- fail to recognize an existing position;
- omit or duplicate a stop;
- incorrectly assign capacity.

The claim that SQLite persistence itself necessarily creates stop latency is not established without inspecting the actual execution path. Nevertheless, stop protection must not depend on asynchronous persistence completing.

#### Improvement

Separate:

- live in-memory risk state;
- exchange-confirmed state;
- durable persistence.

During ambiguity:

```text
exchangeConfirmed
locallyPersisted
inferred
ambiguous
```

If exposure is ambiguous, assume the conservative exposure and prohibit new risk until reconciliation completes. Persistence should mirror a live state machine, not be the source of truth for immediate protection.

---

## 4. Strategy-stage critique

### Stage 1: Market data and signal construction

The broad signal coverage is a strength. CVD, depth flow, Hawkes activity, liquidity, toxicity, lead-lag, sentiment, and correlation each can be useful.

The problem is that breadth does not imply independent information. The first priority is not adding more signals, but measuring incremental value:

\[
\Delta_i
=
\text{Performance(full model)}
-
\text{Performance(model without signal }i\text{)}
\]

Run this:

- in isolation;
- conditional on the strongest existing signal family;
- by regime;
- by symbol;
- after costs;
- with strict time ordering.

Signals should also carry freshness, sample size, and validity rather than just a scalar value.

---

### Stage 2: Category and regime classification

Regime classification is valuable if categories correspond to materially different conditional distributions.

For every category, test whether it changes:

- forward-return distribution;
- volatility;
- spread;
- fill probability;
- adverse excursion;
- signal half-life;
- stop-out probability;
- execution cost.

If two categories do not produce meaningfully different distributions, merge them. A narrative taxonomy that does not change a decision is not adding strategic value.

Regime handling should also be dynamic. The same confidence score should not have the same meaning during:

- compression;
- ignition;
- continuation;
- exhaustion;
- liquidation;
- thin-liquidity conditions.

---

### Stage 3: Manifold and fluid representation

A lower-dimensional or physics-inspired representation can be useful for compressing complex state information. But mathematical sophistication does not establish economic usefulness.

Two specific concerns require testing:

1. **Scale consistency:** Hawkes excitation, particle mass, energy, and book-derived quantities must be normalized by symbol and regime.
2. **Convergence validity:** a settled manifold is not necessarily a predictive manifold.

If particle inputs vary by orders of magnitude across symbols, the same settling tolerance and energy threshold can have different meanings. A global `CoherenceMag2` is especially problematic if used directly in symbol-level allocation.

Benchmark the manifold against simpler baselines:

- log returns;
- realized volatility;
- spread;
- depth;
- signed volume;
- order-book imbalance;
- realized impact;
- cross-sectional rank.

The manifold should remain only if it provides incremental, out-of-sample, net-of-cost value.

---

### Stage 4: Resonance forecasting

An adaptive per-symbol learner is appropriate for nonstationary crypto markets, but adaptation introduces its own risks.

The forecast should include:

- expected return;
- forecast uncertainty;
- horizon;
- age;
- decay rate;
- training sample count;
- calibration regime;
- model validity.

A forecast should expire. A positive forecast stored in a thesis should not remain actionable merely because the thesis object remains present.

Test the learner with:

- purged and embargoed validation;
- delayed labels;
- regime holdouts;
- volatility-normalized targets;
- overlapping-label correction;
- forecast calibration by horizon;
- net PnL after turnover and costs.

If a forgetting factor exists, tune it to a measured regime half-life rather than choosing **0.95–0.995** generically. If it does not exist, add one together with covariance stabilization and reset logic.

---

### Stage 5: Causal reasoning

Causal modeling can help with timing, confounding, and scenario analysis. But notation such as:

\[
do(X=x)
\]

does not make an intervention identifiable.

For a market variable to support an intervention claim, the system needs:

- a defined treatment;
- temporal ordering;
- a causal graph;
- a valid adjustment set or experiment;
- positivity and overlap;
- assumptions about unobserved confounders;
- stability across environments;
- a model of the system’s own market impact.

The system’s own order can invalidate static causal assumptions through impact and adverse selection.

Classify outputs as:

```text
observationalAssociation
adjustedAssociation
quasiExperimental
identifiedIntervention
structuralExtrapolation
```

Use unverified causal outputs for diagnostics, robustness checks, and scenario generation—not as automatic probability multipliers.

---

### Stage 6: Cognition, sequence states, and attractor basins

Sequence recognition can detect useful event progressions such as:

```text
compression → ignition → continuation → exhaustion
```

But it is highly vulnerable to hindsight leakage. If a state label includes information from a later phase, the model may appear to identify the current basin only because the label was constructed with future data.

Enforce:

- online labels using only information available at time \(t\);
- explicit delay before future-confirmed labels enter training;
- the same missingness and latency in validation as live inference;
- a measured lead time long enough to overcome execution delay.

The cognitive layer should also report when it is extrapolating outside its training domain.

---

### Stage 7: Graph evidence aggregation

The graph is potentially valuable because it can preserve support, contradiction, and conditional relationships better than a flat score.

Its output must nevertheless be calibrated. A graph with support edges is not automatically a posterior probability.

Evaluate:

- reliability diagrams;
- Brier score;
- log loss;
- precision/recall at trade thresholds;
- net expectancy by confidence decile;
- calibration drift by regime.

Support and contradiction should be typed by effect:

```text
direction
timing
fill
size
stop
```

A severe liquidity warning should reduce size or change order type, not necessarily reverse direction.

---

### Stage 8: MCTS planning

MCTS is appropriate only if its transition model represents the actual decision environment well enough to support search.

For trading, transitions must account for:

- latency;
- changing spread;
- partial fills;
- order interaction;
- adverse selection;
- market impact;
- signal decay;
- existing positions;
- stop execution;
- competing opportunities.

If MCTS uses deterministic or open-loop branches, it may optimize an imaginary market. If it uses a realistic stochastic simulator, computational cost may become incompatible with short-lived microstructure edge. This should be measured rather than assumed.

A more appropriate reward is:

\[
U =
E[\text{net PnL}]
-\lambda ES_\alpha
-\gamma \text{turnover}
-\eta \text{capacity usage}
-\rho \text{drawdown contribution}
\]

For short-horizon tactical choices, a calibrated contextual policy or bandit may be preferable to MCTS. MCTS can be retained for slower allocation questions if its state transitions are reliable.

---

### Stage 9: Allocation and execution

This is currently the highest-leverage strategic stage.

The system should not only decide whether to trade. It should decide:

- whether to trade;
- which candidate gets scarce capacity;
- how much to trade;
- whether to use market, aggressive limit, passive limit, or sliced execution;
- whether to wait;
- when to cancel;
- how to exit.

Execution policy should compare:

\[
\text{edge decay rate}
\quad \text{against} \quad
\text{spread savings}
\quad \text{and} \quad
\text{fill probability}
\]

The planner needs a stateful execution model, not merely a static entry-economics check.

---

## 5. Missed strategic edges

### 5.1 Cross-venue and perpetual-futures information

If the system currently executes on Kraken spot, external perpetual venues may provide useful price-discovery information. Funding, basis, liquidation flow, and cross-venue lead-lag can be valuable exogenous features.

The exact lead time—claims such as **50–500 ms** should be measured rather than assumed—will vary by venue, pair, market condition, and network path.

Potential additions include:

- perpetual versus spot basis;
- aggressive trade flow on major futures venues;
- liquidation events;
- funding-rate changes;
- cross-venue spread dislocations;
- venue-specific order-flow leadership.

These should be added only after latency alignment and cost-adjusted incremental testing.

---

### 5.2 Per-symbol execution profiles

The strategy should maintain profiles for:

- tick size;
- minimum order size;
- typical spread;
- depth by level;
- volatility;
- fee tier;
- latency;
- fill probability;
- impact curve;
- toxicity;
- signal half-life.

Without these, identical confidence and fraction rules produce very different risk across instruments.

---

### 5.3 Nonlinear, state-dependent impact

A static or linear impact estimate is insufficient for orders that consume multiple book levels.

Impact should depend on:

- intended notional;
- visible depth;
- queue position;
- participation rate;
- spread;
- volatility;
- cancellation rate;
- book resiliency;
- order urgency.

Use empirical book-depletion curves or a calibrated impact model. A square-root law can be a baseline, but it should not replace venue-specific estimation:

\[
I(q)
\propto
\sigma
\left(\frac{q}{V}\right)^\beta
\]

with \(\beta\) estimated from replay data.

---

### 5.4 Signal half-life and joint horizon/exit optimization

Every signal needs a decay curve:

\[
\alpha(\tau)
=
E[R_{t+\tau}\mid \text{signal at }t]
\]

A signal may be profitable at 5 minutes and harmful at 30 minutes. Entry and exit should be optimized jointly.

Add:

- time stops;
- signal-reversal exits;
- exhaustion exits;
- opportunity-cost exits;
- take-profit probability;
- continuation probability;
- trailing-stop degradation;
- spread-widening exits.

Exit logic is often as important as entry detection. A good entry with a poor exit policy can surrender most favorable excursion.

---

### 5.5 Meta-labeling

Use a second-stage model to predict whether a primary signal should actually be traded:

\[
P(\text{trade succeeds}
\mid
\text{primary signal},
\text{regime},
\text{execution state},
\text{portfolio state})
\]

This allows the system to distinguish:

- “the directional signal is valid”;
- “this is a valid trade now.”

A strong signal may still be rejected because spread, toxicity, latency, or capacity makes implementation uneconomic.

---

### 5.6 Opportunity-cost-aware capacity allocation

With only a few positions, positive expectancy is not sufficient. The system needs to choose the best use of scarce capacity.

Rank candidates by expected utility per unit of:

- marginal expected shortfall;
- capital;
- liquidity consumption;
- holding-time occupancy;
- correlation-adjusted risk;
- execution load.

A candidate that is positive in isolation may be inferior to another candidate with higher net utility and lower capacity usage.

---

### 5.7 Portfolio correlation and factor exposure

Cross-symbol correlation should be used in two distinct ways:

1. **Prediction:** does another asset help forecast this asset?
2. **Risk:** does holding both amplify the same loss?

A lead-lag relationship that improves prediction can simultaneously increase portfolio concentration. Maintain factor and covariance exposure separately from signal evidence.

---

### 5.8 Closed-loop execution feedback

Execution outcomes should feed back into strategy validation and calibration:

- fill ratio;
- realized spread;
- slippage;
- adverse selection;
- time to fill;
- post-fill price movement;
- market impact;
- stop execution quality;
- signal performance conditional on execution delay.

This creates the required loop:

```text
detection → decision → execution → realized outcome → recalibration
```

Without it, the system can improve its forecast while remaining economically wrong.

---

## 6. Highest-impact improvements

### 1. Build a calibrated distributional net-edge layer

Replace scalar confidence plus gross expected return with calibrated estimates for:

- forward return distribution;
- execution-price distribution;
- fill probability;
- adverse excursion;
- stop-out probability;
- expected shortfall;
- holding-time distribution;
- net cost.

This directly addresses the risk of trading paper edge.

---

### 2. Make execution stateful and part of the planner

Extend deterministic simulation with optional:

- latency;
- partial fills;
- spread changes;
- slippage;
- order interaction;
- adverse selection;
- seeded stochasticity;
- fault injection.

Then let allocation and MCTS query the same execution assumptions used in validation. Otherwise the planner is optimized against one market and tested against another.

---

### 3. Remove evidence double-counting

Cluster signals by information family, perform ablation and conditional incremental tests, and calibrate the final score. Do not let five correlated order-flow measurements behave like five independent votes.

---

### 4. Replace fixed-fraction and slot logic with marginal-risk allocation

Retain hard safety limits, including the **20%** maximum fraction if desired, but treat it as only one cap among several. Add:

- stop-distance sizing;
- volatility targeting;
- liquidity limits;
- participation constraints;
- factor exposure;
- correlation-adjusted expected shortfall;
- opportunity-cost ranking.

---

### 5. Separate model interpretation from trade authorization

Causal, manifold, and cognitive layers can remain useful, but each must declare:

- uncertainty;
- data freshness;
- calibration domain;
- identification status;
- incremental out-of-sample value.

The planner should not grant reserved capital merely because an internal model reports a cognitive lead. Reserve capacity should be a separately validated strategy sleeve.
