# Correlation Signal Specification

## 1. Purpose

The correlation signal measures contemporaneous dependence between explicitly supplied price paths.

It measures:

1. signed asynchronous return correlation;
2. absolute dependence magnitude;
3. covariance and self-energy supporting that correlation;
4. temporal overlap and estimator support;
5. each path's own return-energy scale;
6. relative return energy between explicitly compared paths;
7. how correlation and relative energy differ from their own causal histories;
8. how those relationships evolve and recur through time.

An optional cohort aggregation may summarize pair measurements over an explicitly supplied comparison set.

The signal does not infer herd behavior, alpha, stress, noise regimes, market leadership, causality, diversification quality, or trading opportunity.

---

## 2. Pair Orientation

Every primitive correlation measurement is bivariate.

Let:

\[
X=\text{reference path}
\]

\[
Y=\text{measured path}
\]

The measurement belongs to \(Y\) and preserves \(X\) as its peer/reference identity.

Correlation is symmetric mathematically:

\[
\rho_{XY}=\rho_{YX}
\]

but provenance is not.

Each path therefore preserves its own:

- symbol;
- venue/source when relevant;
- `From`;
- `At`;
- price definition.

---

## 3. First Principles

Let positive price observations for \(X\) be:

\[
(t_i^X,P_i^X)
\]

and for \(Y\):

\[
(t_j^Y,P_j^Y)
\]

Define log returns:

\[
\boxed{
r_i^X=
\log\left(
\frac{P_i^X}{P_{i-1}^X}
\right)
}
\]

\[
\boxed{
r_j^Y=
\log\left(
\frac{P_j^Y}{P_{j-1}^Y}
\right)
}
\]

with return intervals:

\[
I_i^X=(t_{i-1}^X,t_i^X]
\]

\[
I_j^Y=(t_{j-1}^Y,t_j^Y]
\]

For asynchronous paths, covariance is measured without resampling either series onto an invented common clock.

Define the overlap indicator:

\[
\omega_{ij}
=
\mathbf{1}
\left(
I_i^X\cap I_j^Y\neq\varnothing
\right)
\]

The Hayashi-Yoshida covariance is:

\[
\boxed{
C_{XY}
=
\sum_{i,j}
r_i^Xr_j^Y\omega_{ij}
}
\]

Only return intervals that participate in at least one cross-path overlap contribute to the self-energy terms.

Let:

\[
\mathcal{I}_X=
\left\{
i:\exists j,\omega_{ij}=1
\right\}
\]

\[
\mathcal{I}_Y=
\left\{
j:\exists i,\omega_{ij}=1
\right\}
\]

Then:

\[
\boxed{
V_X=
\sum_{i\in\mathcal{I}_X}(r_i^X)^2
}
\]

\[
\boxed{
V_Y=
\sum_{j\in\mathcal{I}_Y}(r_j^Y)^2
}
\]

and:

\[
\boxed{
\rho_{XY}
=
\frac{C_{XY}}
{\sqrt{V_XV_Y}}
}
\]

when:

\[
V_X>0,\qquad V_Y>0
\]

The correlation is bounded:

\[
-1\le\rho_{XY}\le1
\]

Using only overlap-participating returns prevents unmatched high-frequency observations on one path from inflating its denominator while contributing nothing to the cross-covariance.

---

## 4. Inputs

### 4.1 Required observations

For each path:

| Input | Unit | Validity |
|---|---|---|
| price | quote/base | finite, positive |
| event timestamp | time | finite, causally ordered |
| stream identity | symbol / venue / instrument | explicit |

### 4.2 Price definition

The price definition MUST be explicit and stable within a pair.

Examples:

- midpoint;
- last trade;
- mark price;
- index price.

Different price definitions MUST NOT be silently mixed.

### 4.3 Path integrity

Each path MUST:

- preserve event-time ordering;
- reject time regression;
- distinguish duplicate observations from new price evidence;
- retain exact observation timestamps;
- preserve feed discontinuities as provenance.

---

## 5. Measurement Envelope

Every pair measurement contains:

- `From`;
- `At`;
- `PeerFrom`;
- `PeerAt`;
- `Maturity`;
- `SNR`.

### 5.1 `From` and `At`

For the measured path \(Y\):

\[
From=\min t_j^Y
\]

\[
At=\max t_j^Y
\]

over observations contributing to the retained pair estimator.

### 5.2 `PeerFrom` and `PeerAt`

For the reference path \(X\):

\[
PeerFrom=\min t_i^X
\]

\[
PeerAt=\max t_i^X
\]

over observations contributing to the same estimator.

These intervals are preserved independently because asynchronous paths need not share identical boundaries.

---

## 6. Core Pair Metrics

### 6.1 `signed_correlation`

\[
\boxed{
\rho=\rho_{XY}
}
\]

**Range:** `[-1,1]`.

**Meaning:** signed contemporaneous dependence between overlapping asynchronous log-return intervals.

- positive: overlapping returns tend to have the same sign;
- negative: overlapping returns tend to have opposite signs;
- near zero: little measured linear dependence under this estimator.

**Non-claim:** correlation does not imply causality.

---

### 6.2 `absolute_correlation`

\[
\boxed{
|\rho|
}
\]

**Range:** `[0,1]`.

**Meaning:** magnitude of measured linear dependence independent of sign.

**Downstream use:** compare dependence strength where directional sign is handled separately.

It MUST NOT replace `signed_correlation`.

---

### 6.3 `covariance`

\[
\boxed{
C_{XY}
=
\sum_{i,j}
r_i^Xr_j^Y\omega_{ij}
}
\]

**Unit:** squared log-return.

**Meaning:** unnormalized asynchronous cross-return product sum.

**Why:** preserves the numerator from which correlation was formed.

---

### 6.4 `return_energy:reference`

\[
\boxed{
V_X=
\sum_{i\in\mathcal{I}_X}(r_i^X)^2
}
\]

**Unit:** squared log-return.

**Meaning:** overlap-supported realized return energy of the reference path.

---

### 6.5 `return_energy:measured`

\[
\boxed{
V_Y=
\sum_{j\in\mathcal{I}_Y}(r_j^Y)^2
}
\]

Same contract as reference energy.

These self-energy terms SHOULD be published with correlation so the denominator is auditable.

---

## 7. Overlap and Support Metrics

### 7.1 `overlap_pair_count`

\[
\boxed{
N_{\cap}
=
\sum_{i,j}\omega_{ij}
}
\]

**Unit:** count.

**Meaning:** number of overlapping return-interval pairs contributing to covariance.

It is support provenance, not necessarily an independent sample count.

---

### 7.2 `supported_return_count:reference`

\[
\boxed{
N_X=
|\mathcal{I}_X|
}
\]

**Unit:** count.

### 7.3 `supported_return_count:measured`

\[
\boxed{
N_Y=
|\mathcal{I}_Y|
}
\]

### 7.4 `shared_time`

Let the shared path interval be:

\[
[t_s,t_e]
=
[
\max(From,PeerFrom),
\min(At,PeerAt)
]
\]

Then:

\[
\boxed{
T_{\mathrm{shared}}
=
\max(0,t_e-t_s)
}
\]

**Unit:** seconds.

### 7.5 `overlap_density`

When \(T_{\mathrm{shared}}>0\):

\[
\boxed{
D_{\cap}
=
\frac{N_{\cap}}
{T_{\mathrm{shared}}}
}
\]

**Unit:** overlap pairs / second.

**Meaning:** observation-overlap density.

This is feed/support context, not correlation strength.

---

## 8. Time-Normalized Return Energy

Raw realized energy grows with observation span.

For cross-market activity context, define a robust time-normalized return-energy scale.

For each valid return interval:

\[
\Delta t_i=t_i-t_{i-1}>0
\]

define instantaneous return energy rate:

\[
\boxed{
e_i=
\frac{r_i^2}{\Delta t_i}
}
\]

**Unit:** inverse second.

For each path:

\[
\boxed{
E_X=
\operatorname{median}_{i\in\mathcal{I}_X}
\left(
\frac{(r_i^X)^2}{\Delta t_i^X}
\right)
}
\]

\[
\boxed{
E_Y=
\operatorname{median}_{j\in\mathcal{I}_Y}
\left(
\frac{(r_j^Y)^2}{\Delta t_j^Y}
\right)
}
\]

The median gives a robust typical return-energy rate without allowing a single large move to define the path's normal activity scale.

### 8.1 `return_energy_rate:reference`

\[
\boxed{E_X}
\]

### 8.2 `return_energy_rate:measured`

\[
\boxed{E_Y}
\]

### 8.3 `relative_return_energy`

When both are positive:

\[
\boxed{
R_E=
\frac{E_Y}{E_X}
}
\]

**Unit:** dimensionless.

Interpretation:

- `1`: equal typical time-normalized return energy;
- `2`: measured path has twice the reference energy rate;
- `0.5`: measured path has half the reference energy rate.

This is descriptive activity context.

It is not alpha, stress, or volatility quality.

---

## 9. Effective Support

Raw overlap-pair count MUST NOT automatically be treated as independent sample count.

When the estimator uses contribution weights \(w_k\), effective support is:

\[
\boxed{
N_{\mathrm{eff}}
=
\frac{(\sum_kw_k)^2}
{\sum_kw_k^2}
}
\]

For equally weighted independent supported increments this reduces to their count.

For asynchronous overlap structures, repeated contributions from the same return interval MUST NOT be counted as independent merely because they overlap several intervals on the other path.

The signal SHOULD publish:

- `effective_sample_count` when defensibly estimable;
- raw overlap counts regardless.

Inferential metrics that require effective independent support are undefined when no defensible support estimator exists.

---

## 10. Maturity

Using the estimator's effective support:

\[
\boxed{
Maturity=
\begin{cases}
0,&N_{\mathrm{eff}}\le1\\[4pt]
1-\frac{1}{N_{\mathrm{eff}}},&N_{\mathrm{eff}}>1
\end{cases}
}
\]

Maturity measures estimator support.

It does not measure correlation strength or statistical significance.

If no defensible effective support can be estimated, baseline-dependent pair measurements remain available but inferential maturity SHOULD reflect only the actual estimator weights, not raw overlap multiplicity.

---

## 11. Fisher Correlation Coordinate

Historical modeling SHOULD operate on the Fisher transform:

\[
\boxed{
z_\rho=
\operatorname{atanh}(\rho)
}
\]

for:

\[
-1<\rho<1
\]

The transform maps:

\[
(-1,1)\rightarrow(-\infty,\infty)
\]

and provides a more suitable additive coordinate for historical residual modeling.

Exactly saturated values \(\rho=\pm1\) require finite estimator uncertainty before any Fisher-space metric can be formed; they MUST NOT be silently nudged inward with an arbitrary epsilon.

---

## 12. Correlation Historical Baseline

Let the causal Fisher-space baseline be:

\[
\mu^\rho_{t-}
\]

and prior residual noise scale:

\[
\sigma^\rho_{t-}
\]

### 12.1 `correlation_baseline`

When finite:

\[
\boxed{
B_\rho=
\tanh(\mu^\rho_{t-})
}
\]

**Range:** `[-1,1]`.

### 12.2 `correlation_divergence`

\[
\boxed{
d_\rho=
z_{\rho,t}-\mu^\rho_{t-}
}
\]

**Unit:** Fisher-correlation coordinate.

### 12.3 `correlation_zscore`

\[
\boxed{
Z_\rho=
\frac{d_\rho}
{\sigma^\rho_{t-}}
}
\]

**Meaning:** current signed correlation departure relative to this pair's own historical correlation noise.

---

## 13. Pair Signal-to-Noise Ratio

For the pair measurement, the primary measured phenomenon is correlation.

Therefore:

\[
\boxed{
SNR=
\frac{
d_\rho^2
}{
(\sigma^\rho_{t-})^2
}
=
Z_\rho^2
}
\]

**Unit:** dimensionless, non-negative, unbounded.

Interpretation:

- `0`: correlation equals its historical baseline;
- `1`: current Fisher-space departure equals one historical noise scale;
- `4`: departure amplitude equals two historical noise scales.

SNR is undefined until the causal residual noise scale is estimable and non-zero.

It is not confidence, significance, or hypothesis separation.

---

## 14. Relative-Energy Historical Baseline

Because relative return energy is positive and multiplicative:

\[
\boxed{
y_E=
\log R_E
}
\]

Let causal baseline and noise scale be:

\[
\mu^E_{t-},\qquad\sigma^E_{t-}
\]

### 14.1 `relative_return_energy_baseline`

\[
\boxed{
B_E=
e^{\mu^E_{t-}}
}
\]

### 14.2 `relative_return_energy_divergence`

\[
\boxed{
d_E=
\log R_E-\mu^E_{t-}
}
\]

### 14.3 `relative_return_energy_zscore`

\[
\boxed{
Z_E=
\frac{d_E}{\sigma^E_{t-}}
}
\]

**Meaning:** how unusual the measured/reference activity ratio is for this pair.

It remains separate from correlation SNR.

---

## 15. Optional Correlation Inference

When a defensible effective sample count satisfies:

\[
N_{\mathrm{eff}}>3
\]

and the return-dependence assumptions required by the variance estimator are satisfied, the signal MAY publish a correlation standard error.

Under the standard Fisher approximation:

\[
\boxed{
SE_z=
\frac{1}{\sqrt{N_{\mathrm{eff}}-3}}
}
\]

For the null:

\[
H_0:\rho=0
\]

the Fisher test statistic is:

\[
\boxed{
T_\rho=
\frac{\operatorname{atanh}(\rho)}
{SE_z}
}
\]

and the two-sided p-value is:

\[
\boxed{
p=
2[
1-\Phi(|T_\rho|)
]
}
\]

Recommended optional metrics:

- `correlation_standard_error_fisher`;
- `correlation_p_value`.

No significance threshold is embedded in the signal.

If the assumptions are not defensible, these metrics are undefined.

---

## 16. Temporal Dynamics

### 16.1 `correlation_velocity`

Fit a causal local regression in Fisher space:

\[
z_{\rho,i}
=
a+\beta_\rho(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_\rho=
\beta_\rho
}
\]

**Unit:** Fisher-correlation units / second.

**Meaning:** rate at which signed dependence is changing.

---

### 16.2 `relative_return_energy_velocity`

Fit:

\[
\log R_{E,i}
=
a+\beta_E(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_E=
\beta_E
}
\]

**Unit:** log-ratio / second.

**Meaning:** rate at which the measured/reference activity ratio is changing.

---

### 16.3 Slope SNR

For either fitted slope:

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

## 17. Historical Recurrence

The pair signal MAY retain the standardized trajectory:

\[
\boxed{
Z_t=
\begin{bmatrix}
Z_{\rho,t}\\
Z_{E,t}
\end{bmatrix}
}
\]

when both components are defined.

The current trajectory is compared with non-overlapping historical trajectories of equivalent causal support.

Recommended metrics:

### 17.1 `historical_path_distance`

Distance to the closest prior correlation/activity trajectory.

### 17.2 `historical_path_percentile`

Empirical percentile of that nearest-match distance within retained pair history.

### 17.3 `historical_match_from`

Start time of the nearest historical trajectory.

These measurements describe recurrence or novelty only.

---

## 18. Optional Explicit Cohort Aggregation

A cohort measurement is permitted only when the comparison set is supplied explicitly.

The signal MUST NOT invent peers.

Let focal symbol be \(X\), with explicit peer set:

\[
\mathcal{P}=
\{Y_1,\ldots,Y_m\}
\]

For each valid pair \(p\), let:

\[
\rho_p
\]

be the pair correlation and:

\[
N_{\mathrm{eff},p}
\]

its effective support.

### 18.1 Fisher information weight

When:

\[
N_{\mathrm{eff},p}>3
\]

define:

\[
\boxed{
w_p=N_{\mathrm{eff},p}-3
}
\]

This is proportional to inverse variance under the standard Fisher approximation.

If effective support is not defensible, inverse-variance cohort aggregation is undefined.

---

### 18.2 `cohort_signed_correlation`

Let:

\[
z_p=\operatorname{atanh}(\rho_p)
\]

Then:

\[
\boxed{
\bar z=
\frac{\sum_pw_pz_p}
{\sum_pw_p}
}
\]

and:

\[
\boxed{
\rho_{\mathrm{cohort}}
=
\tanh(\bar z)
}
\]

**Meaning:** support-weighted signed dependence of the focal path with the explicitly supplied cohort.

---

### 18.3 `cohort_absolute_correlation`

Define:

\[
a_p=
\operatorname{atanh}(|\rho_p|)
\]

Then:

\[
\boxed{
\bar a=
\frac{\sum_pw_pa_p}
{\sum_pw_p}
}
\]

\[
\boxed{
A_{\mathrm{cohort}}
=
\tanh(\bar a)
}
\]

**Meaning:** typical dependence magnitude with the cohort independent of sign.

This MUST remain separate from signed cohort correlation.

---

### 18.4 `cohort_correlation_dispersion`

In Fisher space:

\[
\boxed{
S_z=
\sqrt{
\frac{
\sum_pw_p(z_p-\bar z)^2
}{
\sum_pw_p
}
}
}
\]

**Unit:** Fisher-correlation units.

**Meaning:** cross-peer dispersion of signed pair relationships.

A focal path can have the same mean cohort correlation under tightly clustered or highly heterogeneous pair relationships; this metric preserves that distinction.

---

## 19. Cohort Return Energy

Let focal return energy rate be:

\[
E_X>0
\]

and peer energy rates:

\[
E_p>0
\]

Use the same pair information weights \(w_p\).

The support-weighted geometric peer energy is:

\[
\boxed{
E_{\mathrm{peer}}
=
\exp
\left(
\frac{
\sum_pw_p\log E_p
}{
\sum_pw_p
}
\right)
}
\]

### 19.1 `focal_return_energy_rate`

\[
\boxed{E_X}
\]

### 19.2 `peer_return_energy_rate`

\[
\boxed{E_{\mathrm{peer}}}
\]

### 19.3 `relative_cohort_return_energy`

\[
\boxed{
R_{\mathrm{cohort},E}
=
\frac{E_X}{E_{\mathrm{peer}}}
}
\]

**Meaning:** focal activity scale relative to the explicitly supplied cohort's typical activity scale.

No score or interpretation is attached.

---

## 20. Cohort SNR

When a cohort measurement is emitted, define the causal state:

\[
\boxed{
X_t=
\begin{bmatrix}
\operatorname{atanh}(\rho_{\mathrm{cohort},t})\\
\log R_{\mathrm{cohort},E,t}
\end{bmatrix}
}
\]

with causal baseline:

\[
\mu_{t-}
\]

and residual covariance:

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
SNR_{\mathrm{cohort}}
=
\frac{1}{2}
\delta_t^\top
\Sigma_{t-}^{-1}
\delta_t
}
\]

This measures joint departure of cohort dependence and focal-relative activity from their established historical noise.

It does not distinguish competing market hypotheses.

---

## 21. Cohort Maturity

Cohort maturity MUST reflect the actual weights of the aggregate.

For pair weights \(w_p\):

\[
\boxed{
N_{\mathrm{peers,eff}}
=
\frac{(\sum_pw_p)^2}
{\sum_pw_p^2}
}
\]

This measures effective cohort breadth.

The aggregate also depends on within-pair estimator maturity.

The measurement-level cohort maturity is:

\[
\boxed{
M_{\mathrm{cohort}}
=
\min(
M_{\mathrm{peer\ breadth}},
M_{\mathrm{pair\ information}}
)
}
\]

where each component is derived from the effective weights of the actual estimator.

If the implementation cannot expose enough information to compute both honestly, cohort maturity is undefined and a cohort measurement MUST NOT be emitted as mature evidence.

---

## 22. Relationship to Lead-Lag

Correlation measures dependence under contemporaneous alignment.

Lead-lag measures dependence across explicit temporal shifts.

Useful downstream comparisons include:

- `signed_correlation`;
- lead-lag `best_lag_correlation`;
- lead-lag `best_lag_seconds`;
- lead-lag `absolute_correlation_gain`.

A strong contemporaneous correlation and a strong shifted correlation are different measurements.

Correlation does not select a lag.

Lead-lag does not replace contemporaneous correlation.

---

## 23. Relationship to CVD / Executed Flow

CVD measures local executed-flow direction and size.

Correlation measures price-path dependence.

Useful downstream combinations include:

- correlation departure + local signed net flow;
- correlation velocity + flow velocity;
- high pair correlation with opposing local executed-flow signs;
- low pair correlation despite similar executed-flow configurations.

The correlation signal does not use flow to strengthen or weaken its own measured correlation.

---

## 24. Relationship to Hawkes

Hawkes measures event-arrival dependence in time.

Correlation measures return dependence.

Useful comparisons include:

- price correlation versus arrival-intensity similarity;
- correlation divergence versus Hawkes excitation changes;
- high price correlation with weak event-process coupling;
- weak price correlation with strong event-arrival coupling.

Neither relationship implies the other.

---

## 25. Relationship to Liquidity

Liquidity measures displayed executable capacity and spread.

Useful downstream combinations include:

- correlation changes under unusual liquidity SNR;
- correlation changes when one market's depth diverges from baseline;
- pair correlation versus relative spread;
- relative return energy versus relative liquidity state.

A correlation change observed during unusual liquidity can be contextualized without the correlation signal assigning a cause.

---

## 26. Relationship to Depthflow and Toxicity

Useful downstream comparisons include:

- correlation divergence versus book-turnover divergence;
- correlation changes during unusual touch withdrawal or retreat;
- pair return-energy differences versus book-mutation rates;
- cohort-correlation dispersion versus heterogeneous local book states.

The signals remain independent measurements.

---

## 27. Relationship to Sentiment / Cross-Sectional Price State

A cross-sectional price-state signal may measure:

- breadth;
- median return;
- return dispersion;
- leader magnitude.

Correlation may complement that with:

- focal/cohort dependence;
- pairwise sign;
- correlation dispersion;
- relative return energy.

Breadth is not correlation.

High breadth can occur with heterogeneous pair correlations, and high correlation can occur with either positive or negative common movement.

---

## 28. Relationship to Derivatives

Useful explicit pairs include:

- spot versus perpetual;
- mark versus index;
- same contract across venues.

Correlation may then be compared downstream with:

- basis;
- open-interest change;
- liquidation flow;
- derivative/spot lead-lag.

Correlation does not infer price discovery, leverage transmission, or causal linkage.

---

## 29. Cross-Symbol and Cross-Pair Comparability

The following MAY be compared across pairs when estimator construction is equivalent:

- signed correlation;
- absolute correlation;
- Fisher-space correlation divergence;
- correlation z-score;
- relative return energy;
- relative-energy z-score;
- SNR;
- historical-path percentile.

The following require observation-support context and SHOULD NOT be ranked blindly:

- raw covariance;
- raw realized self-energy;
- overlap-pair count;
- shared time;
- overlap density.

Cohort comparisons require explicit cohort definitions.

Two different cohorts are not assumed interchangeable merely because both produce one aggregate number.

---

## 30. Invalid and Missing States

The signal MUST distinguish:

1. no reference path;
2. no measured path;
3. fewer than two valid prices on either path;
4. no overlapping return intervals;
5. zero reference self-energy;
6. zero measured self-energy;
7. measured zero correlation;
8. unavailable time-normalized energy;
9. unavailable effective support;
10. unavailable Fisher inference;
11. unavailable historical baseline;
12. unavailable SNR;
13. undefined cohort because peers were not supplied;
14. undefined cohort because pair support is insufficient;
15. feed discontinuity or incompatible price semantics.

Rules:

- no overlap is missing correlation, not zero correlation;
- zero covariance with positive self-energies may legitimately produce zero correlation;
- zero return energy makes relative-energy ratios undefined;
- exact \(\rho=\pm1\) remains a valid raw measurement but Fisher-dependent metrics are undefined unless finite uncertainty permits a principled representation;
- missing metrics are never filled with semantic fallback scores.

---

## 31. Explicit Non-Claims

The correlation signal does not determine:

- whether markets are moving as a herd;
- whether one asset has alpha;
- whether a market is noisy;
- whether markets are stressed;
- whether one market causes another;
- whether one symbol is a leader;
- whether high correlation is good or bad;
- whether low correlation implies diversification;
- whether correlation will persist;
- whether relative return energy is opportunity;
- whether a cohort is economically meaningful unless its definition was supplied externally.

Those are downstream reasoning tasks.

---

## 32. Minimal Required Pair Metric Set

A valid pair-correlation implementation SHOULD minimally publish:

- `reference_symbol` as provenance;
- `signed_correlation`;
- `absolute_correlation`;
- `covariance`;
- `return_energy:reference`;
- `return_energy:measured`;
- `overlap_pair_count`;
- `supported_return_count:reference`;
- `supported_return_count:measured`;
- `shared_time`;
- `overlap_density`;
- `return_energy_rate:reference`;
- `return_energy_rate:measured`;
- `relative_return_energy`;
- `effective_sample_count` when estimable;
- `correlation_baseline`;
- `correlation_divergence`;
- `correlation_zscore`;
- `relative_return_energy_baseline`;
- `relative_return_energy_divergence`;
- `relative_return_energy_zscore`;
- `correlation_velocity`;
- `relative_return_energy_velocity`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `PeerFrom`;
- `PeerAt`;
- `Maturity`;
- `SNR`.

Optional inferential metrics are emitted only when their statistical assumptions are satisfied.

---

## 33. Optional Cohort Metric Set

When an explicit cohort and defensible pair information weights are supplied, the signal MAY additionally publish:

- `cohort_peer_count`;
- `cohort_effective_peer_count`;
- `cohort_signed_correlation`;
- `cohort_absolute_correlation`;
- `cohort_correlation_dispersion`;
- `focal_return_energy_rate`;
- `peer_return_energy_rate`;
- `relative_cohort_return_energy`;
- cohort historical divergences;
- cohort historical-path distance;
- cohort `Maturity`;
- cohort `SNR`.

No cohort-level semantic classification is emitted.
