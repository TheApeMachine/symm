# Global Signals Specification

## 1. Purpose

A signal is a measuring instrument.

Its responsibility is to observe a defined phenomenon, derive defensible measurements from those observations, preserve provenance and estimator quality, and publish metrics that downstream systems can combine.

A signal MUST NOT:

- assign intent;
- classify a market state;
- select a hypothesis;
- produce trading advice;
- encode labels such as `spoof`, `organic`, `synthetic`, `exhausted`, `alpha`, `healthy`, or `risky`;
- collapse distinct observables into an opaque score when the components can be preserved.

A signal MAY:

- transform raw observations into physically or statistically meaningful quantities;
- maintain causal baselines;
- estimate dispersion, covariance, rates, correlations, or fitted model parameters;
- measure divergence from historical state;
- measure the motion of that divergence;
- measure recurrence or novelty of a trajectory;
- publish dimensionless structural metrics designed for comparison.

The governing test is:

> Two downstream consumers may disagree about what a metric implies while agreeing completely about what the metric measures.

If that is not possible because the metric already contains the conclusion, the metric belongs downstream.

---

## 2. Required Measurement Envelope

Every `Measurement` MUST contain:

| Field      | Meaning                                                                                 | Contract                                                                 |
|------------|-----------------------------------------------------------------------------------------|--------------------------------------------------------------------------|
| `From`     | Start of the observation or retained estimator interval contributing to the measurement | Required. `From <= At`.                                                  |
| `At`       | Event-time instant at which the measurement is valid                                    | Required.                                                                |
| `Maturity` | Effective amount of evidence supporting the stateful estimator                          | Required, dimensionless, `[0,1]`.                                        |
| `SNR`      | Signal power relative to estimated noise power                                          | Required field; value MAY be undefined until a noise model is estimable. |
| `Metrics`  | Named measured quantities                                                               | Required collection.                                                     |

Additional provenance such as symbol, source, peer identity, sample count, effective sample size, model epoch, or observation cadence SHOULD be included when needed to interpret the measurement correctly.

### 2.1 `From`

For a point measurement with no historical estimator:

\[
From = At
\]

For a stateful estimator, `From` is the earliest retained observation with non-zero contribution to the estimator or trajectory represented by the measurement.

For bivariate or cross-sectional measurements, each side MUST preserve its own observation interval if the intervals differ.

---

## 3. Metric Classes

Every published metric SHOULD belong to one of these classes.

### 3.1 Direct observable

A quantity read directly from the source.

Examples: best bid price, trade quantity, event count, resting depth.

### 3.2 Derived measurement

A deterministic transformation of observables with a stable mathematical meaning.

\[
spread = ask-bid
\]

\[
return = \log(P_t/P_{t-1})
\]

\[
imbalance = \frac{A-B}{A+B}
\]

### 3.3 Historical comparison

A current observation expressed relative to its own causal history, such as ratio to baseline, log divergence, standardized residual, or historical percentile.

### 3.4 Temporal dynamic

A measurement of how another measured quantity is changing, such as velocity, justified acceleration, local trend residual, or persistence.

### 3.5 Fitted model quantity

A parameter or diagnostic produced by a formally specified estimator, such as Hawkes intensity, decay rate, spectral radius, likelihood difference, or correlation.

A fitted parameter is not a market conclusion.

### 3.6 Structural metric

A scale-free description of geometry or arrangement, such as concentration, entropy, normalized spacing, symmetry, or distributional distance.

Structural metrics are preferred for cross-symbol comparison when absolute scales are not comparable.

---

## 4. Causality

Every historical comparison MUST be causal.

Let the current observation be \(x_t\). A baseline and noise model used to evaluate \(x_t\) MUST be constructed from information available strictly before the evaluation of \(x_t\):

\[
\mu_{t-},\;\sigma_{t-},\;\Sigma_{t-}
\]

The current observation is evaluated first:

\[
\delta_t = x_t-\mu_{t-}
\]

Only after the measurement is formed may \(x_t\) update the estimator.

A signal MUST NOT allow the current observation to reduce its own apparent divergence by first moving the baseline or enlarging the dispersion against which it is judged.

---

## 5. Baselines

A baseline is an estimate of the state against which a current observation is compared.

The choice of estimator MUST follow the nature of the measured quantity.

### 5.1 Additive quantities

\[
\delta_t = x_t-\mu_{t-}
\]

### 5.2 Positive multiplicative quantities

For strictly positive quantities whose changes are naturally ratios, operate in log space:

\[
y_t = \log x_t
\]

\[
\delta_t = y_t-\mu_{t-}
\]

The corresponding intuitive ratio is:

\[
R_t = e^{\delta_t}
\]

so reciprocal changes are symmetric:

\[
100\rightarrow200 \Rightarrow \delta=+\log2
\]

\[
100\rightarrow50 \Rightarrow \delta=-\log2
\]

### 5.3 Estimator horizons

Estimator horizons SHOULD be derived from observed event timing, effective sample support, or measured stability.

Fixed constants are permitted only when they are part of the formal definition of an established method or of an external market convention.

---

## 6. Noise

Noise is the empirical residual variation remaining after the baseline or model has accounted for the estimated state.

For a scalar quantity:

\[
\epsilon_i = x_i-\mu_{i-}
\]

Estimate noise power causally from prior residuals:

\[
\sigma^2_{t-} = E[\epsilon^2]_{t-}
\]

For a vector signal state:

\[
\epsilon_i \in \mathbb{R}^k
\]

estimate the causal covariance matrix:

\[
\Sigma_{t-}=E[\epsilon\epsilon^\top]_{t-}
\]

The signal MUST NOT add an arbitrary epsilon merely to force a finite result. If the noise model is not estimable, the dependent metric is undefined.

---

## 7. Signal-to-Noise Ratio

`SNR` measures how large the current departure is relative to the noise that normally surrounds the measurement.

It is not a hypothesis score and is not a probability.

### 7.1 Scalar SNR

For current divergence:

\[
\delta_t=x_t-\mu_{t-}
\]

and causal noise variance:

\[
\sigma^2_{t-}
\]

define:

\[
\boxed{SNR_t=\frac{\delta_t^2}{\sigma^2_{t-}}}
\]

If:

\[
z_t=\frac{\delta_t}{\sigma_{t-}}
\]

then:

\[
\boxed{SNR_t=z_t^2}
\]

Interpretation:

- `SNR = 0`: no measured departure from baseline;
- `SNR = 1`: departure amplitude equals one noise standard deviation;
- `SNR = 4`: departure amplitude equals two noise standard deviations;
- `SNR = 9`: departure amplitude equals three noise standard deviations.

SNR MUST remain an unbounded non-negative ratio. It MUST NOT be compressed into `[0,1]`.

### 7.2 Multivariate SNR

For a \(k\)-dimensional signal state:

\[
\delta_t \in \mathbb{R}^k
\]

with causal covariance:

\[
\Sigma_{t-}
\]

define:

\[
\boxed{SNR_t=\frac{1}{k}\delta_t^\top\Sigma_{t-}^{-1}\delta_t}
\]

The covariance term prevents correlated dimensions from being counted repeatedly.

If \(\Sigma\) cannot be estimated or inverted reliably, `SNR` is undefined.

---

## 8. Maturity

Maturity measures how much effective evidence supports an estimator. It does not measure how strong the current signal is.

For observations with weights \(w_i\), define effective sample size:

\[
\boxed{N_{\mathrm{eff}}=\frac{(\sum_i w_i)^2}{\sum_i w_i^2}}
\]

For normalized weights:

\[
N_{\mathrm{eff}}=\frac{1}{\sum_iw_i^2}
\]

Define maturity:

\[
\boxed{
M =
\begin{cases}
0, & N_{\mathrm{eff}}\le1\\[4pt]
1-\frac{1}{N_{\mathrm{eff}}}, & N_{\mathrm{eff}}>1
\end{cases}
}
\]

Examples:

| Effective observations | Maturity |
|-----------------------:|---------:|
|                      1 |        0 |
|                      2 |      0.5 |
|                     10 |      0.9 |
|                    100 |     0.99 |

For a stateless direct measurement that requires no historical estimator, maturity is `1`.

If a measurement contains several independently supported stateful estimators, per-metric maturity SHOULD be retained and the global `Maturity` SHOULD be the minimum maturity of the estimators required for the published joint SNR.

---

## 9. Divergence and Momentum

When a signal measures departure from baseline, it SHOULD preserve both the present divergence and its temporal development when useful.

### 9.1 Divergence

Additive:

\[
d_t=x_t-\mu_{t-}
\]

Positive multiplicative:

\[
d_t=\log x_t-\mu_{t-}
\]

### 9.2 Standardized divergence

\[
z_t=\frac{d_t}{\sigma_{t-}}
\]

### 9.3 Divergence velocity

For irregular event timing, divergence velocity SHOULD be estimated from a causal local regression:

\[
d_i=\alpha+\beta(t_i-t)+\epsilon_i
\]

The slope:

\[
\boxed{v_d=\beta}
\]

measures divergence change per unit time.

Signals SHOULD also expose the residual error or slope SNR when downstream consumers need to distinguish coherent movement from jitter.

Acceleration SHOULD only be added if it demonstrably contributes information not already present in divergence and velocity.

---

## 10. Historical Recurrence and Novelty

A signal MAY compare the present trajectory with its own prior trajectories.

The signal MUST report similarity or distance, not a regime label.

Let a standardized multivariate trajectory be:

\[
Z_t=[z^{(1)}_t,\ldots,z^{(k)}_t]
\]

over a causally derived observation horizon.

The signal MAY use a motif/discord method such as a multivariate matrix profile to compare the current subsequence with non-overlapping historical subsequences.

Recommended outputs:

| Metric                       | Meaning                                                                     |
|------------------------------|-----------------------------------------------------------------------------|
| `historical_path_distance`   | Distance to the closest prior trajectory under the specified metric         |
| `historical_path_percentile` | Empirical percentile of that nearest-match distance within retained history |
| `historical_match_from`      | Start time of the nearest prior trajectory, when useful                     |

Low distance means a familiar path. High distance means the path is structurally unusual relative to retained history.

No regime label is emitted.

---

## 11. Cross-Symbol Comparison

Cross-symbol comparison is valid only when the compared quantity has a meaningful common scale.

### 11.1 Absolute quantities

Absolute values such as notional depth MUST NOT be compared across arbitrary symbols unless the comparison population is explicitly defined and the units are economically comparable.

The identity of a comparison group is part of the analytical question and MUST NOT be silently invented by a signal.

### 11.2 Scale-free structure

Dimensionless morphology MAY be compared across arbitrary symbols when normalization removes price and size scale.

Examples include normalized book-distance distributions, concentration, entropy, symmetry, normalized spacing regularity, and shape-change distance.

Cross-sectional ranking or clustering of those measurements remains a downstream operation.

---

## 12. Raw and Normalized Values

A normalized value MAY be published only when the normalization has an intrinsic mathematical interpretation.

Valid examples:

\[
\frac{x}{baseline}
\]

\[
\frac{ask-bid}{midpoint}
\]

\[
\frac{A-B}{A+B}
\]

\[
z=\frac{x-\mu}{\sigma}
\]

A value MUST NOT be normalized merely to place unrelated metrics into a common `[0,1]` competition space.

---

## 13. Missingness and Undefined Values

The following states MUST remain distinct:

1. measured zero;
2. not observed;
3. not yet estimable;
4. invalid input;
5. estimator failure.

A signal MUST NOT substitute zero for an undefined or unavailable measurement.

Dependent metrics are omitted or explicitly undefined until their prerequisites exist.

---

## 14. Units and Timescales

Every metric MUST state:

- physical unit;
- whether it is dimensionless;
- whether it is a level, rate, ratio, derivative, count, or fitted parameter;
- the observation interval from which it was derived.

Estimator update time does not change the unit of the measured quantity.

For example, an event-time EMA of quote-currency depth is still quote-currency depth, not quote-currency per second.

---

## 15. Cross-Signal Relationships

A signal specification MUST document how its metrics may strengthen, weaken, or contextualize metrics from other signals.

These relationships are not computed by the signal unless the other observation is part of its explicit measurement domain.

For example:

\[
\frac{\text{aggressive buy notional}}{\text{ask touch notional}}
\]

relates executed flow to displayed capacity.

A large cancellation measurement combined with falling displayed depth differs from the same cancellation measurement followed by full replenishment.

A large price move under shallow displayed liquidity differs mechanically from the same move under deep displayed liquidity.

The signal reports the components; downstream reasoning evaluates their joint meaning.

---

## 16. Signal Specification Template

Every signal specification MUST use the following structure.

### 16.1 Purpose

Define the phenomenon being measured in one paragraph.

### 16.2 First Principles

State the irreducible observables and the physical/statistical reasoning that makes the measurement meaningful.

### 16.3 Inputs

List required and optional observations, units, validity conditions, and timestamps.

### 16.4 Measurement Envelope

Define `From`, `At`, `Maturity`, `SNR`, and signal-specific provenance.

### 16.5 Metrics

For every metric specify:

1. **Name**
2. **Meaning**
3. **Formula**
4. **Unit / range**
5. **Why this calculation is principled**
6. **How it is used downstream**
7. **Relationships to other metrics/signals**
8. **Conditions under which it is undefined**

### 16.6 Baseline and Noise Model

Define the causal baseline, residual, dispersion/covariance estimator, and update ordering.

### 16.7 Temporal Dynamics

Define velocity, trend, persistence, or acceleration metrics.

### 16.8 Historical Recurrence

Define trajectory representation, comparison method, and recurrence/novelty metrics if applicable.

### 16.9 Cross-Sectional Comparability

State which quantities may be compared across symbols and why.

### 16.10 Cross-Signal Relationships

Document useful downstream combinations without assigning conclusions.

### 16.11 Invalid and Missing States

Define how missing, invalid, unready, and zero observations are distinguished.

### 16.12 Explicit Non-Claims

List conclusions the signal does not make.

---

## 17. Naming Rules

Metric names SHOULD describe the measured quantity rather than an inferred interpretation.

Prefer:

- `depth_divergence`;
- `spread_ratio`;
- `cancellation_fraction`;
- `relative_energy`;
- `shape_change_distance`.

Avoid:

- `scarcity_score`;
- `spoof_score`;
- `alpha_score`;
- `exhaustion`;
- `urgency`;
- `healthy`;
- `organic`;
- `synthetic`.

Interpretive names belong to downstream reasoning.
