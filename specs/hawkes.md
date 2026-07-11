I’ve now read the paper and traced the complete current path through `symm` and `nomagique`.

My overall conclusion is:

> The paper is useful as a prompt to compare exponential and heavy-tailed kernels, but it is not rigorous enough to serve as an implementation specification. Our implementation is mathematically more careful in several places, yet its sampling, refitting, classification, and strategy interpretation currently prevent it from being a trustworthy Hawkes signal.

The right direction is not to implement the paper literally. It is to retain our exact likelihood machinery, repair the observation and estimation model, then add a properly validated multi-timescale kernel and event taxonomy.

## What the paper actually proposes

The paper models buy and sell arrivals as a bivariate Hawkes process:

\[
\lambda_i(t)
=
\mu_i
+
\sum_{j=1}^{2}
\int_0^t \phi_{ij}(t-s)\,dN_j(s)
\]

It compares:

\[
\phi_{ij}(u)=\alpha_{ij}e^{-\beta_{ij}u}
\]

against:

\[
\phi_{ij}(u)=\frac{\alpha_{ij}}{(u+\epsilon_{ij})^{\beta_{ij}}}
\]

Its claimed conclusion is that the power-law kernel visually reproduces clustering better than the exponential kernel. It then proposes using high buy/sell intensity to provide liquidity around anticipated directional movement.

That is a plausible research question, but the evidence in the paper is weak:

- Roughly 200 events.
- A two-second observation interval.
- Apparently only one small selection of instruments and time.
- Primarily visual comparison of intensity plots.
- No reported out-of-sample likelihood.
- No information criteria.
- No residual diagnostics.
- No confidence intervals.
- No parameter-stability analysis.
- No execution simulation supporting the proposed strategy.
- No comparison against simple order-flow baselines.

The strategy section is therefore a hypothesis, not a demonstrated result.

## What our implementation currently does

The runtime path is:

1. [signal/hawkes/trade.go](/Users/theapemachine/go/src/github.com/theapemachine/symm/signal/hawkes/trade.go) receives executed Kraken trades.
2. `nomagique/algorithm.TradeExcitationSample` stores buy and sell arrival timestamps.
3. `nomagique/algorithm.Excitation` fits a bivariate exponential Hawkes process.
4. `nomagique/hawkes.BivariateEstimator` maximizes the exact log-likelihood.
5. The fitted process is converted into:
   - Frenzy.
   - Saturation.
   - Organic.
   - Exhaustion.
6. A general score classifier turns those values into category probabilities.
7. The resulting measurement enters the ordinary signal pipeline through [trader/signal.go](/Users/theapemachine/go/src/github.com/theapemachine/symm/trader/signal.go).

The actual model is:

\[
\lambda_B(t)
=
\mu_B
+
\alpha_{BB}R_B(t)
+
\alpha_{BS}R_S(t)
\]

\[
\lambda_S(t)
=
\mu_S
+
\alpha_{SB}R_B(t)
+
\alpha_{SS}R_S(t)
\]

with shared exponential decay:

\[
R_j(t)=\sum_{t_k^j<t}e^{-\beta(t-t_k^j)}
\]

This is a legitimate bivariate exponential Hawkes model.

## What is already better than the paper

### Exact likelihood evaluation

The implementation evaluates the continuous-time point-process likelihood rather than fitting a curve to sampled intensities.

The likelihood has the proper form:

\[
\log L
=
\sum_k\log\lambda_{m_k}(t_k)
-
\sum_i\int_0^T\lambda_i(t)\,dt
\]

That is the correct fitting objective.

### Simultaneous-event treatment

`ExcitationState.LogLikelihoodSum` evaluates all events sharing a timestamp before adding their excitation to the state. This avoids events at the exact same timestamp spuriously exciting each other according to iteration order.

That is a good and important detail.

### Proper stability concept

Our implementation calculates the spectral radius of the branching matrix:

\[
G =
\frac{1}{\beta}
\begin{bmatrix}
\alpha_{BB} & \alpha_{BS}\\
\alpha_{SB} & \alpha_{SS}
\end{bmatrix}
\]

and requires:

\[
\rho(G)<1
\]

The paper incorrectly suggests that individual conditions such as \(\alpha_{ij}/\beta_{ij}<1\) ensure stationarity. In the multivariate case, the relevant condition is the spectral radius of the matrix of integrated kernels.

### Cross-excitation is fitted explicitly

The implementation distinguishes:

- Buy-to-buy excitation.
- Sell-to-buy excitation.
- Buy-to-sell excitation.
- Sell-to-sell excitation.

That is central to a meaningful bivariate model.

### Poisson baseline is available

The estimator can fall back structurally to a zero-excitation Poisson fit when there is insufficient evidence for a Hawkes fit. Conceptually, comparing Hawkes against Poisson is exactly what should happen.

The implementation around that comparison needs correction, however.

# Critical findings

## 1. `PoissonImprovement` is not Poisson improvement

In `enrichReading`, the code compares the full model against:

```go
restricted := hawkes.BivariateFit{
    MuX: fit.MuX,
    MuY: fit.MuY,
    AlphaXX: fit.AlphaXX,
    AlphaYY: fit.AlphaYY,
    Beta: fit.Beta,
}
```

This restricted model retains self-excitation and removes only cross-excitation.

Therefore:

```go
reading.poissonImprovement = hawkesLikelihood -
    restricted.LogLikelihood(...)
```

actually measures incremental cross-excitation likelihood, not improvement over Poisson.

### Why this matters

`excitationEligible` uses this value to determine whether a high-risk reading is eligible:

```go
return outcome.PoissonImprovement > 0
```

A strong self-exciting process with no cross-excitation can therefore be rejected because the mislabeled metric is approximately zero.

Conversely, the name and downstream interpretation imply that the entire Hawkes model has beaten the Poisson baseline when only its cross terms have been tested.

### Resolution

Expose two distinct quantities:

\[
\Delta\ell_{\mathrm{Hawkes-Poisson}}
=
\ell(\hat\theta_{\mathrm{full}})
-
\ell(\hat\theta_{\mathrm{Poisson}})
\]

and:

\[
\Delta\ell_{\mathrm{Cross}}
=
\ell(\hat\theta_{\mathrm{full}})
-
\ell(\hat\theta_{\mathrm{self-only}})
\]

The Poisson comparison must use:

```go
BivariateFit{
    MuX: fit.MuX,
    MuY: fit.MuY,
    Beta: fit.Beta,
}
```

Preferably, the restricted models should each be re-optimized under their restrictions instead of merely zeroing coefficients from the full fit.

## 2. The fixed timestamp caps distort the observation process

The sample retains at most:

- 64 buy timestamps.
- 64 sell timestamps.
- 128 total timestamps.

These are arbitrary event-count windows.

### Why this matters

Hawkes likelihood depends on both event history and the observation interval. A fixed count window creates a variable-duration sample:

- A busy regime covers a short duration.
- A quiet regime covers a long duration.
- The inferred baseline and decay scale are therefore conditioned on activity in a mechanically inconsistent way.

The trimming is also side-biased:

```go
for trim > 0 && len(window.buySeconds) > 0 {
    window.buySeconds = window.buySeconds[1:]
}
```

Buys are removed first, followed by sells. If the total cap is exceeded while both sides contain history, older buy observations can be discarded before older sell observations even when the sell observations occurred earlier.

That directly distorts asymmetry and cross-excitation.

### Resolution

Maintain one chronologically ordered marked-event stream:

```text
(timestamp, side, optional marks)
```

Derive the observation interval from a statistically justified memory horizon tied to the fitted kernel support and data sufficiency.

If memory must be bounded for resource reasons, evict the globally oldest event, regardless of side. Resource capacity and statistical observation horizon should be separate concepts.

## 3. Refitting is effectively suppressed by the cooldown

The fitting cooldown is:

\[
50\times\text{current observation span}
\]

If the current sample spans two seconds, a fit may be reused for 100 seconds while only its terminal intensity is updated.

### Why this matters

This means parameters describing the current excitation regime may remain fixed through many completely different event populations.

Because the stored window itself is bounded to only 128 events, the fitted parameters can describe events that have already been evicted from the current sample.

This produces a hybrid state:

- Old parameters.
- New event history.
- New horizon intensity.
- Potentially unrelated statistical regime.

There is no likelihood-based reason for the factor 50.

### Resolution

Refit based on information change rather than elapsed wall time.

Deterministic triggers could include:

- The effective event set changed materially.
- A fitted-kernel score or likelihood gradient exceeds its uncertainty.
- Predictive residuals cease to resemble unit-rate exponential residuals.
- A sequential likelihood-ratio test rejects the current parameter state.
- The current fit’s confidence region no longer covers the updated score.

For performance, use online recursive excitation-state updates between fits. A refit policy must have a measurable error-versus-latency justification.

## 4. The exponential model has only one shared decay rate

The model has four excitation amplitudes but one `Beta`:

\[
\beta_{BB}
=
\beta_{BS}
=
\beta_{SB}
=
\beta_{SS}
\]

### Why this matters

This forces every causal channel to have the same memory:

- Buy continuation.
- Sell continuation.
- Buy response to sells.
- Sell response to buys.

There is no reason to assume those effects decay identically.

The paper allows separate \(\beta_{ij}\), although it does not validate them adequately.

### Resolution

Do not immediately jump to a raw power-law implementation. First implement a sum-of-exponentials basis:

\[
\phi_{ij}(t)
=
\sum_{k=1}^{K}
a_{ij,k}e^{-\beta_k t}
\]

This provides:

- Approximation of power-law memory.
- Constant-time recursive updates per basis component.
- Stable integral evaluation.
- A finite branching matrix.
- Practical real-time computation.
- Direct comparison with the current single-exponential model.

The decay bases should be selected from empirical event-time scales and validated out of sample. They must not be copied from the paper.

## 5. The current “branching ratio” is not the system branching ratio

The code reports a count-weighted average of branching-matrix column sums:

\[
\bar n
=
\frac{
N_B(G_{BB}+G_{SB})
+
N_S(G_{BS}+G_{SS})
}{
N_B+N_S
}
\]

This is a defensible descriptive statistic, but it is not the same as the spectral radius and is not a universal branching ratio for the coupled process.

### Why this matters

The signal comment describes it as:

> “The descendant trades likely to be triggered by a single parent trade.”

That interpretation depends on the parent type and, for total descendants across generations, involves:

\[
(I-G)^{-1}-I
\]

not simply one weighted average of first-generation column sums.

### Resolution

Expose explicit quantities:

- `ImmediateOffspringBuyParent`.
- `ImmediateOffspringSellParent`.
- `ExpectedCascadeBuyParent`.
- `ExpectedCascadeSellParent`.
- `SpectralRadius`.

If a population-weighted aggregate is useful, name it `ObservedParentMixImmediateOffspring` and document the weighting.

## 6. The model is trade-arrival-only, not an LOB Hawkes model

The paper alternates imprecisely between “orders” and executed activity. Our runtime is more explicit: it consumes only executed Kraken trades.

The `TouchImbalance` field exists in the sampling and excitation input, but `excitationSymbol.measure` discards it through an unused argument. Hawkes is not registered as a book or L3 consumer.

### Why this matters

A bivariate buy/sell trade model can measure clustering of aggressive executions. It cannot distinguish:

- Aggressive execution.
- Limit-order replenishment.
- Cancellation cascades.
- Quote withdrawal.
- Passive absorption.
- Spoof-like add/cancel activity.
- Liquidity depletion caused by fills versus cancellations.

Calling it a general limit-order-book excitation model would overstate what it observes.

### Resolution

Keep the current process explicitly named as an aggressive-trade Hawkes process.

Then construct a separate marked multivariate order-flow process once reliable L3 lifecycle data is available. Candidate event dimensions are:

- Aggressive buy.
- Aggressive sell.
- Bid add.
- Ask add.
- Bid cancel.
- Ask cancel.
- Bid fill.
- Ask fill.
- Upward mid-price move.
- Downward mid-price move.

Do not begin with all ten dimensions if data support is insufficient. Select the smallest event taxonomy that answers a defined forecast question.

## 7. Trade size, price distance, and venue state are discarded

Only timestamp and side enter the Hawkes fit.

A tiny trade and a market-sweeping trade both add exactly one unit of excitation.

### Why this matters

The point process is measuring message clustering, not economic-flow clustering. Trade-message fragmentation can make one execution algorithm appear much more exciting than another even when their total quantity is identical.

### Resolution

Evaluate marked Hawkes alternatives where an event carries causal, bounded marks such as:

- Normalized executed quantity.
- Quote notional.
- Number of price levels crossed.
- Distance from prior mid.
- Pre-trade touch depth.
- Resulting price move.

Marks must enter through an explicitly estimated impact function. Do not simply multiply excitation by raw volume.

Compare unmarked and marked models out of sample before selecting one.

## 8. Floating Unix seconds reduce timestamp resolution

Trade nanoseconds are converted to `float64` Unix seconds and later reconstructed as `time.Time`.

At current epoch magnitude, a `float64` cannot retain nanosecond resolution. Events separated by sufficiently small intervals may collapse to the same representable timestamp.

### Why this matters

Kernel likelihood is particularly sensitive to short inter-arrival times. Artificial ties affect:

- Median gap.
- Candidate decay rates.
- Simultaneous-event treatment.
- Likelihood.
- Intensity peaks.

### Resolution

Keep timestamps as integer nanoseconds or as durations relative to the start of the observation epoch.

For numerical fitting, use:

\[
\tau_i =
\frac{t_i-t_0}{1\ \mathrm{second}}
\]

This keeps values near zero and preserves far more subsecond precision.

## 9. Classification is not a validated consequence of Hawkes theory

The categories “frenzy,” “saturation,” “organic,” and “exhaustion” are hand-built interpretations over:

- Spectral radius.
- Intensity asymmetry.
- Current intensity-to-baseline ratio.
- Historical quantile gates.
- An EMA transition.

These categories do not come from the Hawkes likelihood itself.

### Why this matters

Near-critical spectral radius means the fitted stationary process has strong endogenous propagation. It does not by itself mean:

- A breakout is imminent.
- Volatility will rise.
- Buyers or sellers will win.
- Liquidity is about to disappear.

Likewise, current intensity below fitted baseline is not necessarily “exhaustion.” It can be an ordinary low realization of the process.

### Resolution

The model should initially emit typed statistical state:

- Buy and sell conditional intensity.
- Baseline intensity.
- Self-excitation matrix.
- Cross-excitation matrix.
- Spectral radius.
- Cascade expectation.
- Kernel memory.
- Likelihood improvement.
- Parameter uncertainty.
- Residual diagnostic status.

Forecast models can then test whether those quantities predict:

- Next aggressive side.
- Price move.
- Touch depletion.
- Spread change.
- Volatility.
- Replenishment.
- Executable return.

Only retain regime names if those regimes acquire empirically defined outcomes.

## 10. The category gate logic has questionable semantics

`FitGatesFromHistory` chooses:

- Saturation from an extreme upper quantile of spectral radius.
- Frenzy asymmetry from an extreme lower quantile of absolute asymmetry.

The lower quantile means the frenzy threshold can become very small. Ordinary asymmetry may then qualify as “frenzy.”

Fallbacks also replace zero saturation radius with 1 and zero asymmetry with 1.

### Why this matters

These substitutions hide degenerate history instead of reporting that the classifier is not identifiable.

The thresholds are adaptive in the narrow sense, but their statistical meaning is unclear.

### Resolution

Remove categorical thresholds from the core Hawkes estimator.

If classification remains useful, derive it from a labeled forecast target or an explicit posterior decision problem. Degenerate history must return “not ready,” not threshold substitution.

## 11. The Poisson baseline below the fit floor is reported as a market state

Below the bivariate fitting floor, the code deliberately emits a Poisson fit and then passes it into the same classifier.

### Why this matters

“No excitation can yet be identified” is not evidence of organic flow, exhaustion, or any other market regime.

Maturity partly communicates this, but the category still exists and can enter aggregation.

### Resolution

Separate:

- `ReadyForIntensity`: enough data to report empirical arrival rate.
- `ReadyForHawkesFit`: enough data to identify excitation parameters.
- `ReadyForForecast`: residual and out-of-sample validation passed.

Before Hawkes readiness, publish an arrival-rate observation, not a Hawkes category measurement.

## 12. Concurrency ownership is unsafe

The sampler uses `sync.Map`, but each stored `tradeExcitationWindow` contains ordinary mutable slices with no synchronization.

### Why this matters

`sync.Map` protects the map, not the stored window. Concurrent ingestion for the same symbol can race on slice append, trimming, and feature extraction.

Even if the current runtime happens to serialize a symbol today, the type’s contract does not enforce that.

### Resolution

Give every per-symbol stream one explicit owner:

- Serialize events through the existing symbol processing path, or
- Protect each population with a narrowly scoped lock.

The preferred solution is ownership and sequencing, not broad shared locking.

## 13. The paper’s liquidity-provision strategy should not be adopted directly

The paper suggests placing sell limits during buy clusters and buy limits during sell clusters.

### Why this is dangerous

High aggressive-buy intensity can mean:

- Profitable upward continuation.
- Imminent exhaustion.
- Toxic informed flow.
- A market order splitting program.
- Temporary noise.

Posting a sell into aggressive buying can capture spread, or it can provide cheap liquidity to informed buyers immediately before the price rises.

The paper does not estimate:

- Fill probability.
- Adverse selection.
- Queue position.
- Post-fill price drift.
- Cancellation latency.
- Inventory risk.
- Fees or rebates.
- Impact.
- Opportunity cost.

### Resolution

Use Hawkes state to forecast execution outcomes before using it for liquidity provision:

\[
P(\mathrm{fill}\mid \mathcal F_t)
\]

\[
E[\Delta m_{t+h}\mid \mathrm{fill},\mathcal F_t]
\]

\[
E[\mathrm{spread\ capture}
-\mathrm{adverse\ selection}
-\mathrm{fees}]
\]

Only provide liquidity when expected post-fill value is positive. Cluster intensity alone is insufficient.

# Recommended target design

I recommend evolving the implementation in this order.

## Stage 1: Repair the current exponential model

- Replace the two side-specific slices with one ordered marked stream.
- Preserve timestamp precision using relative integer nanoseconds.
- Remove fixed side-biased trimming.
- Correct Poisson versus self-only likelihood comparisons.
- Remove or rename the current aggregate branching ratio.
- Replace cooldown-based refitting with evidence-based fit invalidation.
- Expose estimator readiness separately from arrival-rate readiness.
- Remove categorical strategy claims from the core estimator.
- Add time-rescaling residual diagnostics.

This produces a trustworthy baseline.

## Stage 2: Add kernel comparison

Implement a common kernel interface supporting:

1. Single exponential.
2. Sum of exponentials approximating heavy-tailed decay.
3. Direct regularized power law only if it remains computationally and statistically justified.

Compare using:

- Held-out log-likelihood.
- AIC/BIC where appropriate.
- Time-rescaled residuals.
- Parameter stability.
- Calibration of next-event forecasts.
- Runtime and allocation benchmarks.

The paper’s visual conclusion should not decide the kernel.

## Stage 3: Add meaningful marks

Test trade quantity and book-state marks separately.

Do not add every possible mark simultaneously. For each one, establish whether it improves unseen-event likelihood or an explicit market forecast.

## Stage 4: Extend to L3 event types

Once L3 delivery is gap-free, model the specific interactions the strategy needs.

For example, for liquidity survival:

\[
N(t)=
\begin{bmatrix}
N_{\mathrm{bidAdd}}\\
N_{\mathrm{bidCancel}}\\
N_{\mathrm{bidFill}}\\
N_{\mathrm{askAdd}}\\
N_{\mathrm{askCancel}}\\
N_{\mathrm{askFill}}
\end{bmatrix}
\]

For directional aggressive flow, the current two-dimensional trade process may remain a separate, simpler model.

## Stage 5: Forecast before classification

First forecast concrete outcomes. Then determine whether named regimes add anything useful.

The initial output should be statistical state, not market storytelling.

## Stage 6: Strategy integration

For liquidity provision, combine:

- Hawkes fill intensity.
- Cancellation/replenishment intensity.
- Queue state.
- Post-fill adverse-selection forecast.
- Current spread.
- Fees.
- Inventory.
- Exit feasibility.

The Hawkes process should answer “what event is likely next, and at what rate?” The execution model should answer “is participating in that event profitable?”

## Priority assessment

| Priority | Finding                                         | Consequence                                            |
|----------|-------------------------------------------------|--------------------------------------------------------|
| Critical | Side-biased fixed-count trimming                | Corrupts fitted asymmetry and excitation               |
| Critical | Stale fit from `50 × span` cooldown             | Parameters may describe an obsolete regime             |
| Critical | `PoissonImprovement` is misnamed and misused    | Eligibility tests the wrong hypothesis                 |
| High     | Categorical interpretation is unvalidated       | Statistical state becomes unsupported strategy meaning |
| High     | Timestamp precision loss                        | Distorts the shortest and most informative gaps        |
| High     | No residual or out-of-sample diagnostics        | A converged fit can still be a bad model               |
| High     | Trade-only process presented using LOB language | Overstates the information consumed                    |
| Medium   | Shared decay rate                               | Cannot express channel-specific or heavy-tailed memory |
| Medium   | Unmarked arrivals                               | Message fragmentation can dominate economic meaning    |
| Medium   | Mutable per-symbol windows lack ownership       | Potential races under concurrent ingestion             |

The most pragmatic next move is therefore not “implement the paper’s power-law strategy.” It is a focused correctness pass on the existing exponential baseline, followed by a properly benchmarked sum-of-exponentials comparison. That gives us a reliable control model and a practical route to the paper’s genuinely interesting idea—long-memory excitation—without inheriting its evidentiary weaknesses.

No files were changed and no tests were run; this was a read-only review of the paper and current implementation.