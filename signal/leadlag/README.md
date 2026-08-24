# Lead-Lag Signal Specification

## 1. Purpose

The lead-lag signal measures the temporal alignment of two price-return paths.

It measures:

1. contemporaneous dependence between the paths;
2. dependence after explicit time shifts;
3. the time shift at which absolute dependence is greatest;
4. how much the best shifted relationship improves on zero-lag alignment;
5. the amount of data and search breadth supporting that result;
6. how the current lag/dependence structure differs from its own causal history;
7. whether the current lead-lag trajectory resembles prior trajectories.

The signal measures temporal precedence and dependence.

It does not infer causality, information transmission, inefficiency, exploitability, leadership, following, decoupling, or trading opportunity.

---

## 2. Pair Orientation

Every measurement is explicitly oriented.

Let:

\[
X=\text{reference path}
\]

\[
Y=\text{measured path}
\]

The measurement belongs to \(Y\) and carries \(X\) as its peer/reference identity.

Lag sign convention:

\[
\boxed{
\tau>0
\iff
X \text{ is compared with later observations of }Y
}
\]

Therefore a positive lag means:

> the strongest measured alignment occurs when changes in \(X\) precede corresponding changes in \(Y\) by \(\tau\).

A negative lag means the reverse temporal ordering.

This convention describes temporal alignment only.

It does not mean that either path causes the other.

---

## 3. First Principles

Let positive price observations for \(X\) be:

\[
(t^X_i,P^X_i)
\]

and for \(Y\):

\[
(t^Y_j,P^Y_j)
\]

Define log-price increments:

\[
\boxed{
\Delta X_i=
\log\left(
\frac{P^X_i}{P^X_{i-1}}
\right)
}
\]

\[
\boxed{
\Delta Y_j=
\log\left(
\frac{P^Y_j}{P^Y_{j-1}}
\right)
}
\]

with observation intervals:

\[
I^X_i=(t^X_{i-1},t^X_i]
\]

\[
I^Y_j=(t^Y_{j-1},t^Y_j]
\]

For candidate lag \(\tau\), shift the \(Y\) intervals backward by \(\tau\):

\[
\boxed{
I^{Y,\tau}_j=
(t^Y_{j-1}-\tau,\ t^Y_j-\tau]
}
\]

Positive \(\tau\) therefore compares an earlier \(X\) change with a later \(Y\) change.

For asynchronous observations, dependence is measured using the Hayashi-Yoshida overlap construction:

\[
\boxed{
C_{XY}(\tau)=
\sum_{i,j}
\Delta X_i\Delta Y_j
\mathbf{1}
\left(
I^X_i\cap I^{Y,\tau}_j\neq\varnothing
\right)
}
\]

The corresponding self-quadratic variations are:

\[
V_X=
\sum_i(\Delta X_i)^2
\]

\[
V_Y=
\sum_j(\Delta Y_j)^2
\]

and the normalized lagged correlation is:

\[
\boxed{
\rho(\tau)=
\frac{C_{XY}(\tau)}
{\sqrt{V_XV_Y}}
}
\]

when both quadratic variations are positive.

The zero-lag value is:

\[
\boxed{
\rho_0=\rho(0)
}
\]

The best measured lag is:

\[
\boxed{
\tau^\ast=
\operatorname*{arg\,max}_{\tau\in\mathcal{T}}
|\rho(\tau)|
}
\]

with:

\[
\boxed{
\rho^\ast=\rho(\tau^\ast)
}
\]

The argmax is a measured property of the searched correlation surface.

It is not a causal estimator.

---

## 4. Inputs

### 4.1 Required observations

For both paths:

| Input | Unit | Validity |
|---|---|---|
| price | quote/base | finite, positive |
| event timestamp | time | finite, causally ordered |
| stream identity | symbol / venue / instrument | explicit |

### 4.2 Path integrity

Each path MUST:

- preserve event-time ordering;
- distinguish duplicate observations from new observations;
- preserve its own timestamps;
- reject time regression;
- avoid treating transport heartbeats as new price evidence.

### 4.3 Price source

The price source MUST be explicit and consistent within a pair.

Examples include:

- midpoint;
- last trade;
- mark price;
- index price.

Different price definitions MUST NOT be silently mixed in one path.

---

## 5. Measurement Envelope

Every measurement contains:

- `From`;
- `At`;
- `Maturity`;
- `SNR`.

Because the measurement is bivariate, it SHOULD also preserve:

- `PeerFrom`;
- `PeerAt`;
- reference symbol;
- measured symbol.

### 5.1 `From`

`From` is the earliest observation in the measured path contributing non-zero support to the current lag search.

### 5.2 `At`

`At` is the latest measured-path observation represented by the measurement.

### 5.3 Peer interval

`PeerFrom` and `PeerAt` preserve the actual reference-path interval.

They MUST NOT be inferred from the measured path when the two paths are asynchronous.

---

## 6. Search Resolution

Lag MUST be expressed primarily in time, not bars.

Let the median positive observation spacing of each retained path be:

\[
\delta_X=
\operatorname{median}
(t^X_i-t^X_{i-1})
\]

\[
\delta_Y=
\operatorname{median}
(t^Y_j-t^Y_{j-1})
\]

Define lag-search resolution:

\[
\boxed{
\delta_\tau=
\max(\delta_X,\delta_Y)
}
\]

**Why:** a pair cannot reliably resolve lag finer than the slower path's typical observation cadence.

Candidate lags are:

\[
\boxed{
\mathcal{T}
=
\{k\delta_\tau:k\in\mathbb{Z}\}
}
\]

restricted to shifts for which the two paths retain sufficient overlapping return support to estimate correlation.

No fixed arbitrary maximum lag is required.

The observed path spans and minimum mathematical support determine the admissible search domain.

---

## 7. Search Support

For each candidate lag \(\tau\), define:

### 7.1 `overlap_pair_count`

\[
\boxed{
N_{\cap}(\tau)=
\sum_{i,j}
\mathbf{1}
\left(
I^X_i\cap I^{Y,\tau}_j\neq\varnothing
\right)
}
\]

**Unit:** count.

**Meaning:** number of overlapping increment pairs contributing to the Hayashi-Yoshida covariance.

### 7.2 `reference_return_count`

Number of reference-path returns represented by the retained interval.

### 7.3 `measured_return_count`

Number of measured-path returns represented by the retained interval.

### 7.4 `search_count`

\[
\boxed{
M=|\mathcal{T}|
}
\]

**Unit:** count.

**Meaning:** number of candidate temporal shifts actually evaluated.

Search breadth MUST always accompany a selected best lag.

---

## 8. Core Dependence Metrics

### 8.1 `contemporaneous_correlation`

\[
\boxed{
\rho_0=\rho(0)
}
\]

**Range:** `[-1,1]`.

**Meaning:** signed zero-lag dependence of the asynchronous return paths.

**Downstream use:** distinguish immediate co-movement from delayed alignment.

---

### 8.2 `best_lag_correlation`

\[
\boxed{
\rho^\ast=\rho(\tau^\ast)
}
\]

**Range:** `[-1,1]`.

**Meaning:** signed correlation at the candidate time shift with greatest absolute dependence.

The sign describes co-movement:

- positive: aligned return signs;
- negative: opposite return signs.

The sign does not describe which path comes first; lag sign does that.

---

### 8.3 `best_lag_seconds`

\[
\boxed{
L=\tau^\ast
}
\]

**Unit:** seconds.

**Meaning:** signed temporal offset of maximum measured absolute dependence.

Convention:

- `L > 0`: reference path temporally precedes measured path at the best alignment;
- `L = 0`: strongest alignment is contemporaneous;
- `L < 0`: measured path temporally precedes reference path at the best alignment.

---

### 8.4 `best_lag_index`

\[
\boxed{
K^\ast=
\frac{\tau^\ast}{\delta_\tau}
}
\]

**Unit:** candidate steps.

**Purpose:** implementation and provenance.

It MUST NOT replace `best_lag_seconds`.

---

### 8.5 `absolute_correlation_gain`

\[
\boxed{
G_\rho=
|\rho^\ast|-|\rho_0|
}
\]

By construction:

\[
G_\rho\ge0
\]

when zero lag participates in the same candidate search.

**Meaning:** amount by which the best temporal shift improves absolute measured dependence relative to contemporaneous alignment.

**Why:** a non-zero best lag is much less informative when its correlation is nearly identical to the zero-lag correlation.

---

### 8.6 `lag_search_span`

Let:

\[
\tau_{\max}=
\max_{\tau\in\mathcal{T}}|\tau|
\]

Then:

\[
\boxed{
S_\tau=\tau_{\max}
}
\]

**Unit:** seconds.

**Meaning:** maximum absolute lag the retained data could test.

---

### 8.7 `lag_fraction`

When \(\tau_{\max}>0\):

\[
\boxed{
F_\tau=
\frac{|\tau^\ast|}{\tau_{\max}}
}
\]

**Range:** `[0,1]`.

**Meaning:** location of the selected lag within the available search domain.

This is search provenance.

It is not evidence strength.

---

## 9. Correlation-Surface Metrics

A single argmax discards information about the shape of the lagged-correlation surface.

The signal SHOULD preserve measurements describing that surface.

### 9.1 `lag_peak_prominence`

Let the immediate valid neighboring candidates of \(\tau^\ast\) be:

\[
\tau^-=\tau^\ast-\delta_\tau
\]

\[
\tau^+=\tau^\ast+\delta_\tau
\]

When both exist:

\[
\boxed{
P_\rho=
|\rho^\ast|
-
\frac{
|\rho(\tau^-)|+|\rho(\tau^+)|
}{2}
}
\]

**Range:** approximately `[-1,1]`.

**Meaning:** local height of the selected correlation peak above adjacent lag candidates.

Large positive prominence means the selected lag is locally distinct.

A value near zero means the correlation surface is locally flat.

---

### 9.2 `lag_peak_curvature`

When both neighboring candidates exist:

\[
\boxed{
\kappa_\rho=
\frac{
2|\rho^\ast|
-
|\rho(\tau^-)|
-
|\rho(\tau^+)|
}{
\delta_\tau^2
}
}
\]

**Unit:** inverse seconds squared.

**Meaning:** local sharpness of the selected absolute-correlation maximum.

**Downstream use:** distinguish a well-localized temporal alignment from a broad plateau where many nearby lags fit similarly.

No arbitrary width threshold is required.

---

## 10. Multiple-Search Accounting

Selecting:

\[
\max_{\tau\in\mathcal{T}}|\rho(\tau)|
\]

creates a multiple-search problem.

The signal MUST expose the number of searched candidates and MUST NOT treat the largest raw correlation as though it had been specified in advance.

### 10.1 Fisher statistic

When a defensible effective support estimate satisfies:

\[
N_{\mathrm{eff}}(\tau)>3
\]

define:

\[
\boxed{
Z_F(\tau)
=
\operatorname{atanh}(\rho(\tau))
\sqrt{N_{\mathrm{eff}}(\tau)-3}
}
\]

Under the standard independent-return approximation and zero population correlation:

\[
Z_F\approx\mathcal{N}(0,1)
\]

### 10.2 `correlation_p_value`

For a pre-specified candidate lag:

\[
\boxed{
p(\tau)
=
2\left[
1-\Phi(|Z_F(\tau)|)
\right]
}
\]

### 10.3 `search_adjusted_p_value`

For \(M\) tested candidates:

\[
\boxed{
p_{\mathrm{adj}}
=
\min(1,Mp(\tau^\ast))
}
\]

This is the Bonferroni family-wise adjustment.

The factor \(M\) follows directly from the number of tests performed.

No significance cutoff is embedded in the signal.

### 10.4 Validity condition

Fisher/Bonferroni p-values are emitted only when the estimator can justify its effective-support approximation.

If asynchronous overlap or serial dependence invalidates that approximation and no defensible variance estimator is available, these p-values are undefined.

The raw correlation surface remains valid as a descriptive measurement.

---

## 11. Effective Support

When path observations carry weights \(w_i\), effective sample size is:

\[
\boxed{
N_{\mathrm{eff}}
=
\frac{(\sum_iw_i)^2}
{\sum_iw_i^2}
}
\]

For equally weighted independent returns:

\[
N_{\mathrm{eff}}=N
\]

The signal SHOULD publish:

- `effective_sample_count`;
- `reference_return_count`;
- `measured_return_count`;
- `overlap_pair_count`;
- `search_count`.

Effective sample count MUST NOT be invented from raw overlap count when repeated asynchronous overlaps create dependent contributions.

When a defensible effective-support estimator is unavailable, inferential metrics depending on it are undefined.

---

## 12. Maturity

Lead-lag maturity reflects effective retained evidence supporting the pair estimator.

Using:

\[
N_{\mathrm{eff}}
\]

define:

\[
\boxed{
Maturity=
\begin{cases}
0,&N_{\mathrm{eff}}\le1\\[4pt]
1-\frac{1}{N_{\mathrm{eff}}},&N_{\mathrm{eff}}>1
\end{cases}
}
\]

If historical SNR or recurrence requires a less mature pair-state estimator, measurement-level maturity is the minimum maturity required by those published metrics.

Maturity does not indicate whether the lag is real, useful, or stable.

---

## 13. Signal-to-Noise Ratio

The universal SNR for lead-lag measures how far the current temporal-dependence state has departed from its own causal historical state.

Define the pair-state vector:

\[
\boxed{
X_t=
\begin{bmatrix}
\operatorname{atanh}(\rho_{0,t})\\
\operatorname{atanh}(\rho^\ast_t)\\
\tau^\ast_t
\end{bmatrix}
}
\]

Let the causal baseline be:

\[
\mu_{t-}
\]

and the causal residual covariance be:

\[
\Sigma_{t-}
\]

Then:

\[
\delta_t=X_t-\mu_{t-}
\]

and:

\[
\boxed{
SNR_t=
\frac{1}{3}
\delta_t^\top
\Sigma_{t-}^{-1}
\delta_t
}
\]

### 13.1 Why Fisher-transform correlation

Raw correlation is bounded.

The Fisher transform:

\[
\operatorname{atanh}(\rho)
\]

maps:

\[
(-1,1)\rightarrow(-\infty,\infty)
\]

and gives a more suitable approximately additive coordinate for historical residual modeling.

### 13.2 Meaning

SNR answers:

> How unusual is the current combination of contemporaneous dependence, best lagged dependence, and selected lag relative to this pair's own established noise structure?

It does not answer whether either series causes the other.

SNR is undefined until the causal covariance is estimable and non-degenerate.

---

## 14. Historical Baselines

The signal SHOULD maintain causal baselines for:

- Fisher-transformed contemporaneous correlation;
- Fisher-transformed best-lag correlation;
- signed lag seconds;
- absolute correlation gain;
- lag peak prominence.

The current observation is evaluated before it updates those estimators.

### 14.1 `lag_baseline_seconds`

\[
\boxed{
B_L=E[\tau^\ast]_{t-}
}
\]

**Unit:** seconds.

---

### 14.2 `lag_divergence_seconds`

\[
\boxed{
d_L=
\tau^\ast-B_L
}
\]

**Unit:** seconds.

---

### 14.3 `lag_noise_scale_seconds`

\[
\boxed{
\sigma_L=
\sqrt{
E[d_L^2]_{t-}
}
}
\]

**Unit:** seconds.

---

### 14.4 `lag_zscore`

\[
\boxed{
z_L=
\frac{\tau^\ast-B_L}{\sigma_L}
}
\]

**Unit:** dimensionless.

**Meaning:** current temporal offset relative to the pair's own historical lag variability.

---

### 14.5 `best_lag_correlation_baseline`

In Fisher space:

\[
\boxed{
B_{\rho^\ast}
=
E[
\operatorname{atanh}(\rho^\ast)
]_{t-}
}
\]

---

### 14.6 `best_lag_correlation_zscore`

\[
\boxed{
z_{\rho^\ast}
=
\frac{
\operatorname{atanh}(\rho^\ast)-B_{\rho^\ast}
}{
\sigma_{\rho^\ast}
}
}
\]

**Meaning:** how unusual the current signed lagged dependence is for this pair.

---

### 14.7 `correlation_gain_baseline`

\[
\boxed{
B_G=E[G_\rho]_{t-}
}
\]

### 14.8 `correlation_gain_zscore`

\[
\boxed{
z_G=
\frac{G_\rho-B_G}{\sigma_G}
}
\]

**Meaning:** whether the improvement obtained by temporal shifting is itself unusual for the pair.

---

## 15. Temporal Dynamics

### 15.1 `lag_velocity`

Fit a causal local regression over historical lag estimates:

\[
\tau^\ast_i
=
a+\beta_L(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_L=\beta_L
}
\]

**Unit:** seconds of lag / second of event time.

**Meaning:** rate at which the measured temporal offset is moving.

---

### 15.2 `correlation_gain_velocity`

\[
G_{\rho,i}
=
a+\beta_G(t_i-t)+\epsilon_i
\]

\[
\boxed{
v_G=\beta_G
}
\]

**Unit:** correlation / second.

**Meaning:** whether temporal shifting is becoming more or less important relative to contemporaneous alignment.

---

### 15.3 Slope SNR

For a fitted slope:

\[
\boxed{
SNR_\beta=
\frac{\beta^2}
{\operatorname{Var}(\beta)}
}
\]

MAY be emitted when regression uncertainty is estimable.

Acceleration is not required.

---

## 16. Historical Recurrence

The signal MAY retain a standardized pair trajectory:

\[
\boxed{
Z_t=
\begin{bmatrix}
z_{\rho_0,t}\\
z_{\rho^\ast,t}\\
z_{L,t}\\
z_{G,t}
\end{bmatrix}
}
\]

where each component is standardized against its causal historical estimator.

The current trajectory is compared with non-overlapping historical trajectories of equivalent support.

Recommended metrics:

### 16.1 `historical_path_distance`

Distance to the closest prior lead-lag trajectory.

### 16.2 `historical_path_percentile`

Empirical percentile of the nearest-match distance within retained pair history.

### 16.3 `historical_match_from`

Start time of the nearest historical trajectory.

These measurements describe recurrence or novelty.

No relationship regime is assigned.

---

## 17. Pair Selection

The lead-lag signal measures an explicitly supplied pair.

It MUST NOT silently choose a reference symbol because it:

- moved the most;
- has the largest market capitalization;
- appears to be a market leader;
- has the strongest recent return;
- belongs to a presumed peer group.

Pair construction is an outer analytical decision.

Valid examples include explicit comparison of:

- the same asset across venues;
- spot and perpetual markets for the same asset;
- an explicitly configured pair;
- two assets selected by a separate cross-sectional analysis.

The signal knows the pair it was given.

It does not decide which market should be a reference.

---

## 18. Relationship to Correlation

Correlation and lead-lag are related but distinct.

Correlation measures dependence under a defined alignment.

Lead-lag measures how that dependence varies with temporal alignment.

Useful downstream comparisons include:

- contemporaneous correlation from the correlation signal;
- `best_lag_correlation`;
- `best_lag_seconds`;
- `absolute_correlation_gain`;
- `lag_peak_curvature`.

A highly correlated pair may have:

- a zero best lag;
- a stable non-zero best lag;
- a broad correlation plateau;
- a rapidly changing lag.

These are different measured structures.

---

## 19. Relationship to CVD / Executed Flow

CVD measures signed executed flow.

Lead-lag measures price-path temporal alignment.

Downstream analysis MAY test whether:

- aggressive flow in \(X\) precedes price response in \(Y\);
- signed flow changes in \(X\) align with the measured price lag;
- the price lag persists when local executed flow in \(Y\) changes.

The lead-lag signal itself does not ingest CVD to strengthen or weaken a lag.

Doing so would convert measurement into interpretation.

---

## 20. Relationship to Hawkes

Hawkes measures event-arrival excitation.

Lead-lag measures price-return alignment.

Useful downstream comparisons include:

- cross-market arrival excitation versus price lag;
- changes in Hawkes intensity before changes in `best_lag_seconds`;
- event-process coupling versus price-path coupling.

A temporal relationship in event arrivals does not establish the same relationship in prices.

A price lag does not establish event-level excitation.

---

## 21. Relationship to Liquidity

Liquidity measures displayed executable capacity and spread.

Useful downstream combinations include:

- lag seconds under ordinary versus unusual liquidity;
- lag-correlation gain versus depth divergence;
- changes in lag when spreads widen or touch capacity falls;
- lag stability versus liquidity SNR.

A measured lag may change when one market becomes slower to reprice under changing liquidity conditions, but the lead-lag signal does not assign that explanation.

---

## 22. Relationship to Depthflow and Toxicity

Useful downstream combinations include:

- lag changes versus book-turnover rate;
- lag changes versus touch/full-book imbalance disagreement;
- lag changes versus touch withdrawal or replenishment;
- temporal precedence during unusual book mutation.

These signals provide local microstructure context.

They do not convert temporal precedence into causality.

---

## 23. Relationship to Sentiment / Cross-Sectional Price State

A cross-sectional price-state signal may identify broad movement, breadth, or dispersion.

Lead-lag may then be applied to explicitly selected pairs.

Useful comparisons include:

- broad market return path versus one symbol;
- cross-sectional dispersion versus lag stability;
- breadth changes versus correlation gain.

The cross-sectional population definition remains external to lead-lag.

---

## 24. Relationship to Derivatives

Lead-lag is particularly suitable for explicitly related markets such as:

- spot versus perpetual;
- mark versus index;
- the same derivative across venues.

Useful downstream measurements include:

- spot/perpetual lag seconds;
- lag correlation versus basis;
- lag movement versus open-interest change;
- lag changes during liquidation flow.

The signal does not infer price discovery or informational leadership.

---

## 25. Cross-Pair Comparison

The following MAY be compared across pairs when estimator construction is equivalent:

- contemporaneous correlation;
- best-lag correlation;
- absolute correlation gain;
- lag peak prominence;
- lag peak curvature after accounting for lag resolution;
- standardized lag divergence;
- standardized correlation divergence;
- SNR;
- historical-path percentile;
- search-adjusted p-value when its statistical assumptions are satisfied.

Raw lag seconds are comparable only when the economic and observation timescales of the pairs make that comparison meaningful.

`lag_fraction` is search-domain provenance and SHOULD NOT be used as a universal relationship-strength score.

---

## 26. Invalid and Missing States

The signal MUST distinguish:

1. no reference path;
2. no measured path;
3. insufficient return observations;
4. zero quadratic variation in one path;
5. no admissible lag candidates;
6. zero contemporaneous correlation;
7. zero best-lag correlation;
8. best lag equal to zero;
9. unavailable effective sample count;
10. unavailable inferential p-value;
11. unavailable historical covariance;
12. invalid event ordering;
13. mismatched price-source semantics.

Rules:

- no valid pair means relationship metrics are undefined;
- insufficient support is not represented as zero correlation;
- a measured zero correlation is a valid zero;
- a best lag of zero is a valid measured result;
- SNR is undefined until historical covariance exists;
- p-values are undefined when their variance/support assumptions are not defensible;
- missing metrics are never filled with provisional zeros merely to preserve shape.

---

## 27. Explicit Non-Claims

The lead-lag signal does not determine:

- which market is the leader;
- which market is the follower;
- whether one market causes another;
- whether information is transmitted from one market to another;
- whether a lag is exploitable;
- whether a lag is an inefficiency;
- whether two markets are decoupled;
- whether a market has stalled;
- whether a relationship will persist;
- whether transaction costs permit a trade;
- whether the best searched lag is economically meaningful.

Those are downstream reasoning and validation tasks.

---

## 28. Minimal Required Metric Set

A valid lead-lag implementation SHOULD minimally publish:

- `reference_symbol` as provenance;
- `contemporaneous_correlation`;
- `best_lag_correlation`;
- `best_lag_seconds`;
- `best_lag_index`;
- `absolute_correlation_gain`;
- `lag_search_resolution_seconds`;
- `lag_search_span`;
- `lag_fraction`;
- `reference_return_count`;
- `measured_return_count`;
- `overlap_pair_count`;
- `search_count`;
- `effective_sample_count` when estimable;
- `lag_peak_prominence` when neighbors exist;
- `lag_peak_curvature` when neighbors exist;
- `correlation_p_value` when statistically estimable;
- `search_adjusted_p_value` when statistically estimable;
- `lag_baseline_seconds`;
- `lag_divergence_seconds`;
- `lag_noise_scale_seconds`;
- `lag_zscore`;
- `best_lag_correlation_baseline`;
- `best_lag_correlation_zscore`;
- `correlation_gain_baseline`;
- `correlation_gain_zscore`;
- `lag_velocity`;
- `correlation_gain_velocity`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `PeerFrom`;
- `PeerAt`;
- `Maturity`;
- `SNR`.

Metrics whose mathematical prerequisites are not satisfied are explicitly undefined rather than replaced with zeros, classifications, or fallback scores.
