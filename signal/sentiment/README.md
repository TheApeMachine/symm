# Sentiment / Cross-Sectional Price State Signal Specification

## 1. Purpose

The sentiment signal measures the cross-sectional price state of an explicitly supplied market cohort.

It measures:

1. each member's return over a common event-time horizon;
2. advance, decline, and unchanged participation;
3. signed breadth and directional agreement;
4. the typical signed move and typical move magnitude;
5. cross-sectional return dispersion;
6. the identity and magnitude of the largest absolute mover;
7. how that largest move compares with the rest of the cohort;
8. how the current cross-sectional state differs from its own causal history;
9. whether the current cross-sectional trajectory resembles prior trajectories.

The signal does not determine whether the market is bullish, bearish, surging, slumping, divergent, risk-on, risk-off, strong, weak, confirming, or disconfirming.

It does not measure textual, social, news, or psychological sentiment.

---

## 2. Cohort Contract

The cohort is supplied explicitly.

Let:

\[
\mathcal{C}=
\{1,\ldots,N\}
\]

be the configured population of comparable instruments.

The signal MUST NOT infer cohort membership from:

- symbol names;
- current price movement;
- market capitalization;
- correlation;
- exchange listing proximity;
- arbitrary availability in the process.

Cohort definition is an outer analytical fact.

Examples of valid cohorts include explicitly configured:

- same-venue spot markets;
- a sector basket;
- a derivatives universe;
- a curated set of economically related instruments.

All cross-sectional measurements are conditional on the cohort definition.

---

## 3. Common Event-Time Horizon

Cross-sectional returns MUST represent a common target horizon.

Comparing each symbol's most recent tick-to-tick return directly would mix different elapsed times when symbols update at different cadences.

### 3.1 Per-symbol cadence

For each eligible symbol \(i\), estimate its causal typical positive observation spacing:

\[
\boxed{
\delta_i=
\operatorname{median}
\left(
t_{i,k}-t_{i,k-1}
\right)
}
\]

over its retained positive inter-arrival intervals.

### 3.2 Cohort horizon

Define the common cohort horizon as:

\[
\boxed{
H=
\operatorname{median}_{i\in\mathcal{C}_{ready}}
(\delta_i)
}
\]

where \(\mathcal{C}_{ready}\) contains symbols with an estimable causal cadence.

**Unit:** seconds.

**Why:** the horizon is derived from the cohort's own observation clock rather than a fixed wall-clock constant.

### 3.3 Cross-sectional cut

At cohort cut time \(T\):

\[
\boxed{
At=T
}
\]

\[
\boxed{
From=T-H
}
\]

Every member return is evaluated against these same target endpoints.

---

## 4. As-Of Price Construction

For symbol \(i\), define the causal as-of price:

\[
\boxed{
P_i(t)=
P_{i,k^\ast}
\quad
\text{where}
\quad
k^\ast=
\operatorname*{arg\,max}_{k:t_{i,k}\le t}
t_{i,k}
}
\]

This is zero-order hold of the last actually observed price.

It is not interpolation.

### 4.1 Endpoint ages

Current-endpoint age:

\[
\boxed{
A_i^{at}
=
T-t_i^{at}
}
\]

where \(t_i^{at}\) is the timestamp of the observation supplying \(P_i(T)\).

Start-endpoint age:

\[
\boxed{
A_i^{from}
=
(T-H)-t_i^{from}
}
\]

where \(t_i^{from}\) supplies \(P_i(T-H)\).

A member is valid for the current cut only when both endpoint observations satisfy the cohort's causal horizon resolution:

\[
\boxed{
A_i^{at}\le H
}
\]

\[
\boxed{
A_i^{from}\le H
}
\]

This bound is not an independent tuning constant; it is the same horizon the cross-section is attempting to resolve.

Members failing endpoint freshness are excluded from the current cut and counted explicitly.

---

## 5. Member Return

For valid member \(i\):

\[
\boxed{
r_i=
\log
\left(
\frac{P_i(T)}
{P_i(T-H)}
\right)
}
\]

**Unit:** dimensionless log return.

All cross-sectional return statistics use this common target horizon.

### 5.1 `return`

\[
\boxed{r_i}
\]

**Meaning:** member's signed price change over the common cohort horizon.

### 5.2 `absolute_return`

\[
\boxed{
a_i=|r_i|
}
\]

**Meaning:** magnitude of the member's move independent of direction.

### 5.3 `asof_age_seconds`

\[
\boxed{
A_i^{at}
}
\]

### 5.4 `from_age_seconds`

\[
\boxed{
A_i^{from}
}
\]

These ages preserve temporal-alignment quality.

---

## 6. Population Counts

Let \(n\) be the number of valid members in the current cut.

Define:

\[
N_+
=
\sum_i
\mathbf{1}(r_i>0)
\]

\[
N_-
=
\sum_i
\mathbf{1}(r_i<0)
\]

\[
N_0
=
\sum_i
\mathbf{1}(r_i=0)
\]

Then:

\[
\boxed{
n=N_++N_-+N_0
}
\]

Recommended metrics:

- `cohort_member_count`;
- `valid_member_count`;
- `excluded_member_count`;
- `advance_count`;
- `decline_count`;
- `unchanged_count`.

These are direct cross-sectional facts.

---

## 7. Participation Fractions

### 7.1 `advance_fraction`

\[
\boxed{
F_+=\frac{N_+}{n}
}
\]

### 7.2 `decline_fraction`

\[
\boxed{
F_-=\frac{N_-}{n}
}
\]

### 7.3 `unchanged_fraction`

\[
\boxed{
F_0=\frac{N_0}{n}
}
\]

with:

\[
F_++F_-+F_0=1
\]

### 7.4 `directional_participation`

\[
\boxed{
P_d=
\frac{N_++N_-}{n}
=
1-F_0
}
\]

**Range:** `[0,1]`.

**Meaning:** fraction of valid members that moved non-zero over the common horizon.

---

## 8. Breadth and Directional Agreement

### 8.1 `breadth`

\[
\boxed{
B=
\frac{N_+-N_-}{n}
}
\]

**Range:** `[-1,1]`.

Interpretation:

- `+1`: every valid member advanced;
- `0`: advance and decline counts offset, subject to unchanged members;
- `-1`: every valid member declined.

Breadth is a counting statistic.

It does not measure move magnitude.

---

### 8.2 `directional_agreement`

When:

\[
N_++N_->0
\]

define:

\[
\boxed{
A_d=
\frac{
\max(N_+,N_-)
}{
N_++N_-
}
}
\]

**Range:** `[0.5,1]` when both signs are represented, and `1` when all directional members share one sign.

**Meaning:** fraction of moving members belonging to the more common direction.

It is undefined when no member moved.

---

### 8.3 `directional_consensus`

When:

\[
N_++N_->0
\]

define:

\[
\boxed{
C_d=
\frac{
|N_+-N_-|
}{
N_++N_-
}
}
\]

**Range:** `[0,1]`.

**Meaning:** imbalance between advancing and declining members among directional participants.

It is related to, but distinct from, breadth because unchanged members do not enter its denominator.

---

## 9. Typical Cross-Sectional Return

### 9.1 `median_return`

\[
\boxed{
M_r=
\operatorname{median}_i(r_i)
}
\]

**Unit:** log return.

**Meaning:** typical signed move across the cohort.

**Why median:** one extreme member cannot dominate the cohort center.

---

### 9.2 `median_absolute_return`

\[
\boxed{
M_a=
\operatorname{median}_i(|r_i|)
}
\]

**Unit:** absolute log return.

**Meaning:** typical move magnitude across the cohort.

This is distinct from the signed median.

---

### 9.3 `mean_absolute_return`

\[
\boxed{
\bar a=
\frac{1}{n}
\sum_i|r_i|
}
\]

**Unit:** absolute log return.

**Why:** preserves total cross-sectional movement per member and can be compared with the robust median.

---

### 9.4 `rms_return`

\[
\boxed{
R_{\mathrm{rms}}
=
\sqrt{
\frac{1}{n}
\sum_i r_i^2
}
}
\]

**Unit:** absolute log return.

**Meaning:** cross-sectional return-energy amplitude.

Large moves receive quadratic weight.

No semantic label is attached.

---

## 10. Cross-Sectional Dispersion

### 10.1 `return_mad`

Let:

\[
M_r=\operatorname{median}(r_i)
\]

Then:

\[
\boxed{
D_r=
\operatorname{median}_i
|r_i-M_r|
}
\]

**Unit:** log return.

**Meaning:** robust dispersion of signed returns around the cross-sectional median.

---

### 10.2 `magnitude_mad`

Let:

\[
M_a=\operatorname{median}(|r_i|)
\]

Then:

\[
\boxed{
D_a=
\operatorname{median}_i
\left|
|r_i|-M_a
\right|
}
\]

**Unit:** absolute log return.

**Meaning:** robust dispersion of move magnitudes.

This separates "the market has varied directions" from "the market has varied move sizes."

---

### 10.3 `return_interquartile_range`

When enough members exist:

\[
\boxed{
IQR_r=
Q_{0.75}(r)-Q_{0.25}(r)
}
\]

**Unit:** log return.

**Meaning:** central cross-sectional spread of returns.

No Gaussian assumption is required.

---

## 11. Largest-Move Identity

Define:

\[
\boxed{
L=
\max_i|r_i|
}
\]

and:

\[
\boxed{
i^\ast=
\operatorname*{arg\,max}_i|r_i|
}
\]

A unique identity is emitted only when the maximum is unique.

When multiple members tie exactly at \(L\), the identity is undefined and the tie count is published.

Recommended provenance:

- `largest_move_symbol`;
- `largest_move_tie_count`.

The signal uses the term **largest mover**, not leader.

Largest absolute movement does not establish information leadership.

---

## 12. Largest-Move Metrics

### 12.1 `largest_absolute_return`

\[
\boxed{L}
\]

**Unit:** absolute log return.

### 12.2 `largest_signed_return`

For unique \(i^\ast\):

\[
\boxed{
R_L=r_{i^\ast}
}
\]

**Unit:** log return.

### 12.3 `largest_move_share`

Let:

\[
S_a=
\sum_i|r_i|
\]

For \(S_a>0\):

\[
\boxed{
F_L=
\frac{L}{S_a}
}
\]

**Range:** `(0,1]`.

**Meaning:** fraction of total absolute cohort movement contributed by the largest mover.

It is not evidence strength.

---

## 13. Largest Mover Relative to Peers

Let:

\[
\mathcal{P}
=
\mathcal{C}_{valid}
\setminus\{i^\ast\}
\]

for a unique largest mover.

### 13.1 `peer_median_absolute_return`

\[
\boxed{
M_P=
\operatorname{median}_{j\in\mathcal{P}}
|r_j|
}
\]

### 13.2 `peer_magnitude_mad`

\[
\boxed{
D_P=
\operatorname{median}_{j\in\mathcal{P}}
\left|
|r_j|-M_P
\right|
}
\]

### 13.3 `largest_move_excess`

\[
\boxed{
E_L=
L-M_P
}
\]

By definition of the maximum:

\[
E_L\ge0
\]

### 13.4 `largest_move_ratio`

When:

\[
M_P>0
\]

define:

\[
\boxed{
R_L^{peer}
=
\frac{L}{M_P}
}
\]

**Unit:** dimensionless.

Interpretation:

- `1`: largest move equals peer median magnitude;
- `2`: largest move is twice the peer median magnitude.

No bounded transform is applied.

---

### 13.5 `largest_move_mad_excess`

When:

\[
D_P>0
\]

define:

\[
\boxed{
Z_L^{MAD}
=
\frac{L-M_P}{D_P}
}
\]

**Unit:** dimensionless peer-MAD units.

This is a robust standardized excess.

It is not a probability or confidence score.

If peer MAD is zero, the metric is undefined.

No arbitrary epsilon is inserted.

---

## 14. Peer Direction Relative to Largest Mover

For unique largest mover with:

\[
R_L\neq0
\]

let:

\[
s_L=\operatorname{sign}(R_L)
\]

For each peer \(j\), classify only by measured return sign.

### 14.1 `same_direction_peer_count`

\[
\boxed{
N_{\parallel}
=
\sum_{j\in\mathcal{P}}
\mathbf{1}
(
\operatorname{sign}(r_j)=s_L
)
}
\]

excluding zero returns.

### 14.2 `opposite_direction_peer_count`

\[
\boxed{
N_{\mathrm{opp}}
=
\sum_{j\in\mathcal{P}}
\mathbf{1}
(
\operatorname{sign}(r_j)=-s_L
)
}
\]

### 14.3 `zero_return_peer_count`

\[
\boxed{
N_{\mathrm{zero}}
=
\sum_{j\in\mathcal{P}}
\mathbf{1}(r_j=0)
}
\]

### 14.4 `same_direction_peer_fraction`

\[
\boxed{
F_{\parallel}
=
\frac{N_{\parallel}}{|\mathcal{P}|}
}
\]

### 14.5 `opposite_direction_peer_fraction`

\[
\boxed{
F_{\mathrm{opp}}
=
\frac{N_{\mathrm{opp}}}{|\mathcal{P}|}
}
\]

### 14.6 `zero_return_peer_fraction`

\[
\boxed{
F_{\mathrm{zero}}
=
\frac{N_{\mathrm{zero}}}{|\mathcal{P}|}
}
\]

with:

\[
F_{\parallel}
+
F_{\mathrm{opp}}
+
F_{\mathrm{zero}}
=
1
\]

These fractions preserve the facts previously hidden inside semantic "confirmation" or "divergence" scores.

---

## 15. Endpoint-Age Quality Metrics

Cross-sectional state depends on as-of observations.

The signal SHOULD publish:

### 15.1 `median_asof_age_seconds`

\[
\boxed{
M_A=
\operatorname{median}_i
A_i^{at}
}
\]

### 15.2 `max_asof_age_seconds`

\[
\boxed{
A_{\max}
=
\max_i A_i^{at}
}
\]

### 15.3 `median_from_age_seconds`

\[
\boxed{
M_F=
\operatorname{median}_i
A_i^{from}
}
\]

These are temporal provenance metrics.

They are not blended into breadth or return magnitude.

---

## 16. Cross-Sectional Maturity

The current cross-section has equal member weights unless an outer cohort contract explicitly provides economically meaningful weights.

For equal member weights:

\[
N_{\mathrm{cross,eff}}=n
\]

and:

\[
\boxed{
M_{\mathrm{cross}}
=
\begin{cases}
0,&n\le1\\[4pt]
1-\frac{1}{n},&n>1
\end{cases}
}
\]

Historical baseline estimators have their own effective support:

\[
N_{\mathrm{hist,eff}}
=
\frac{(\sum_kw_k)^2}
{\sum_kw_k^2}
\]

with:

\[
M_{\mathrm{hist}}
=
\begin{cases}
0,&N_{\mathrm{hist,eff}}\le1\\[4pt]
1-\frac{1}{N_{\mathrm{hist,eff}}},&
N_{\mathrm{hist,eff}}>1
\end{cases}
\]

Measurement-level maturity is:

\[
\boxed{
Maturity=
\min(
M_{\mathrm{cross}},
M_{\mathrm{hist}}
)
}
\]

when historical SNR is published.

Before historical quality is estimable, direct cross-sectional metrics remain valid while SNR is undefined.

---

## 17. Causal Historical State

Define the core cross-sectional state vector:

\[
\boxed{
X_t=
\begin{bmatrix}
M_{r,t}\\
B_t\\
M_{a,t}\\
D_{r,t}
\end{bmatrix}
}
\]

where:

- \(M_r\) = median signed return;
- \(B\) = breadth;
- \(M_a\) = median absolute return;
- \(D_r\) = return MAD.

Let:

\[
\mu_{t-}
\]

be the causal pre-observation mean state and:

\[
\Sigma_{t-}
\]

its causal residual covariance.

The current cross-sectional cut is evaluated against:

\[
\mu_{t-},\Sigma_{t-}
\]

before it updates either estimator.

---

## 18. Signal-to-Noise Ratio

Define:

\[
\delta_t=
X_t-\mu_{t-}
\]

Then:

\[
\boxed{
SNR_t=
\frac{1}{4}
\delta_t^\top
\Sigma_{t-}^{-1}
\delta_t
}
\]

**Unit:** dimensionless, non-negative, unbounded.

### 18.1 Meaning

SNR answers:

> How unusual is the current combination of typical return, breadth, typical magnitude, and cross-sectional dispersion relative to this cohort's own established noise?

It does not answer:

- whether the market is bullish or bearish;
- whether the move will persist;
- whether the largest mover is informative;
- whether the cohort is in a named regime.

SNR is undefined until the causal covariance is estimable and non-degenerate.

---

## 19. Historical Baselines

The signal SHOULD maintain causal baselines for at least:

- median return;
- breadth;
- median absolute return;
- return MAD;
- largest move share;
- largest move ratio when defined;
- opposite-direction peer fraction when defined.

### 19.1 `median_return_baseline`

\[
\boxed{
\mu^r_{t-}
=
E[M_r]_{t-}
}
\]

### 19.2 `median_return_divergence`

\[
\boxed{
d_r=
M_r-\mu^r_{t-}
}
\]

### 19.3 `median_return_zscore`

\[
\boxed{
z_r=
\frac{d_r}{\sigma^r_{t-}}
}
\]

---

### 19.4 `breadth_baseline`

\[
\boxed{
\mu^B_{t-}
=
E[B]_{t-}
}
\]

### 19.5 `breadth_divergence`

\[
\boxed{
d_B=
B-\mu^B_{t-}
}
\]

### 19.6 `breadth_zscore`

\[
\boxed{
z_B=
\frac{d_B}{\sigma^B_{t-}}
}
\]

---

### 19.7 `median_absolute_return_baseline`

For positive historical magnitude observations, multiplicative modeling MAY use:

\[
y_a=\log M_a
\]

with zero magnitude preserved as an explicit zero state.

When a positive log-space baseline is available:

\[
\boxed{
B_a=
e^{\mu^a_{t-}}
}
\]

### 19.8 `median_absolute_return_ratio`

\[
\boxed{
R_a=
\frac{M_a}{B_a}
}
\]

when both are positive.

### 19.9 `median_absolute_return_zscore`

\[
\boxed{
z_a=
\frac{
\log M_a-\mu^a_{t-}
}{
\sigma^a_{t-}
}
}
\]

for positive \(M_a\).

---

### 19.10 `return_dispersion_baseline`

For positive \(D_r\):

\[
\boxed{
B_D=e^{\mu^D_{t-}}
}
\]

### 19.11 `return_dispersion_ratio`

\[
\boxed{
R_D=
\frac{D_r}{B_D}
}
\]

### 19.12 `return_dispersion_zscore`

\[
\boxed{
z_D=
\frac{
\log D_r-\mu^D_{t-}
}{
\sigma^D_{t-}
}
}
\]

when positive.

---

## 20. Largest-Move Historical Context

Largest-mover statistics remain facts, but their historical unusualness may be measured.

### 20.1 `largest_move_share_baseline`

\[
\boxed{
\mu^{F_L}_{t-}
=
E[F_L]_{t-}
}
\]

### 20.2 `largest_move_share_zscore`

\[
\boxed{
z_{F_L}
=
\frac{
F_L-\mu^{F_L}_{t-}
}{
\sigma^{F_L}_{t-}
}
}
\]

### 20.3 `largest_move_ratio_baseline`

For positive ratio:

\[
\boxed{
\mu^{L}_{t-}
=
E[\log R_L^{peer}]_{t-}
}
\]

### 20.4 `largest_move_ratio_zscore`

\[
\boxed{
z_L=
\frac{
\log R_L^{peer}-\mu^L_{t-}
}{
\sigma^L_{t-}
}
}
\]

This measures whether concentration in one large mover is ordinary or unusual for the cohort.

It does not declare the mover a leader or anomaly.

---

## 21. Temporal Dynamics

### 21.1 `median_return_velocity`

Fit a causal local regression:

\[
M_{r,i}
=
a+\beta_r(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_r=\beta_r
}
\]

**Unit:** log return / second.

---

### 21.2 `breadth_velocity`

\[
B_i
=
a+\beta_B(t_i-t)+\epsilon_i
\]

\[
\boxed{
v_B=\beta_B
}
\]

**Unit:** breadth / second.

---

### 21.3 `median_absolute_return_velocity`

For positive values, fit:

\[
\log M_{a,i}
=
a+\beta_a(t_i-t)+\epsilon_i
\]

\[
\boxed{
v_a=\beta_a
}
\]

**Unit:** log-magnitude / second.

---

### 21.4 `return_dispersion_velocity`

For positive dispersion:

\[
\log D_{r,i}
=
a+\beta_D(t_i-t)+\epsilon_i
\]

\[
\boxed{
v_D=\beta_D
}
\]

---

### 21.5 Slope SNR

For any fitted slope:

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

## 22. Historical Recurrence

The signal MAY retain the standardized cross-sectional trajectory:

\[
\boxed{
Z_t=
\begin{bmatrix}
z_{r,t}\\
z_{B,t}\\
z_{a,t}\\
z_{D,t}
\end{bmatrix}
}
\]

using only dimensions defined for the compared interval.

The current trajectory is compared with non-overlapping historical trajectories of equivalent causal support.

Recommended metrics:

### 22.1 `historical_path_distance`

Distance to the nearest prior cross-sectional trajectory.

### 22.2 `historical_path_percentile`

Empirical percentile of that nearest-match distance within retained cohort history.

### 22.3 `historical_match_from`

Start time of the nearest prior trajectory.

These metrics measure recurrence or novelty only.

They do not predict the future continuation of the matched historical path.

---

## 23. Relationship to Correlation

Correlation measures pairwise dependence.

Cross-sectional price state measures the distribution of simultaneous returns across an explicit population.

Useful downstream combinations include:

- breadth + cohort signed correlation;
- return dispersion + correlation dispersion;
- median return + absolute cohort correlation;
- largest move share + focal/peer correlation.

Examples:

A cohort may have broad positive breadth but modest pairwise correlation if members move upward by heterogeneous paths.

A cohort may have high absolute correlation while all members move downward.

The signals preserve these distinctions.

---

## 24. Relationship to Lead-Lag

Lead-lag measures temporal alignment between explicit pairs.

Cross-sectional state measures the current common-horizon return distribution.

Useful downstream combinations include:

- breadth changes followed by pair lag changes;
- largest-move identity + explicit lead-lag analysis against selected peers;
- return dispersion + lag dispersion across pairs.

The largest mover MUST NOT automatically become the reference path for lead-lag.

Pair selection remains an outer analytical decision.

---

## 25. Relationship to CVD / Executed Flow

CVD measures local aggressive execution.

Useful downstream combinations include:

- member return + local signed net flow;
- cohort median return + cross-sectional distribution of local flow;
- largest absolute mover + its own executed-flow state;
- breadth divergence from history + aggregate flow conditions.

A member can move with or against its local executed-flow imbalance.

The cross-sectional signal does not infer why.

---

## 26. Relationship to Liquidity

Liquidity measures displayed executable capacity and spread.

Useful downstream combinations include:

- breadth under ordinary versus unusual liquidity;
- cross-sectional dispersion versus dispersion in liquidity SNR;
- largest move ratio versus that member's depth ratio;
- median absolute return versus cohort liquidity state.

The same return distribution under deep and shallow displayed capacity represents different measured context without changing the cross-sectional return facts.

---

## 27. Relationship to Depthflow and Toxicity

Useful downstream combinations include:

- breadth changes versus cross-sectional book-turnover changes;
- largest-move identity versus local touch withdrawal;
- return dispersion versus book-imbalance dispersion;
- opposite-direction peer fraction versus heterogeneous liquidity disposition.

No microstructure explanation is assigned by this signal.

---

## 28. Relationship to Hawkes

Hawkes measures event-arrival dynamics.

Useful downstream combinations include:

- median absolute return + aggregate arrival intensity;
- breadth changes + buy/sell arrival asymmetry across members;
- largest mover + its local excitation fraction;
- cross-sectional dispersion + dispersion of Hawkes SNR.

Large cross-sectional movement may arise under either clustered or ordinary event arrival dynamics.

---

## 29. Relationship to Derivatives

For derivative cohorts, downstream analysis may combine:

- breadth with open-interest changes;
- median return with basis distribution;
- largest mover with liquidation flow;
- return dispersion with derivatives-state dispersion.

Cross-sectional price state itself does not infer leverage, squeezing, deleveraging, or liquidation causality.

---

## 30. Cross-Cohort Comparability

Dimensionless metrics such as:

- breadth;
- advance fraction;
- decline fraction;
- unchanged fraction;
- directional consensus;
- largest move share;

are mathematically comparable in form across cohorts.

They are not automatically economically comparable.

Results depend on:

- cohort composition;
- cohort size;
- instrument type;
- venue;
- common-horizon cadence;
- missing-member fraction.

Raw comparisons across materially different cohorts require explicit justification.

Historical normalization SHOULD generally be cohort-specific.

---

## 31. Invalid and Missing States

The signal MUST distinguish:

1. no configured cohort;
2. insufficient members with cadence estimates;
3. invalid common horizon;
4. missing as-of current endpoint;
5. missing as-of historical endpoint;
6. endpoint age exceeding the common horizon;
7. valid zero return;
8. no directional members;
9. tied largest absolute movers;
10. zero peer median magnitude;
11. zero peer MAD;
12. unavailable historical baseline;
13. unavailable covariance for SNR;
14. cohort-composition change.

Rules:

- excluded stale members are counted, not silently treated as zero-return members;
- zero return is a valid observation;
- directional agreement is undefined when every return is zero;
- largest-move identity is undefined under an exact tie;
- ratios with zero denominators are undefined;
- no arbitrary epsilon is inserted;
- cohort-composition changes MUST be represented as provenance and SHOULD begin a new historical baseline epoch when the population definition materially changes.

---

## 32. Explicit Non-Claims

The sentiment / cross-sectional price-state signal does not determine:

- bullishness;
- bearishness;
- surge;
- slump;
- divergence;
- confirmation;
- leadership;
- market strength;
- market weakness;
- alpha;
- whether the largest mover is informative;
- whether peers should follow the largest mover;
- whether broad movement will continue;
- whether a move is organic or synthetic;
- human, social, textual, or news sentiment.

Those are downstream reasoning tasks.

---

## 33. Minimal Required Metric Set

A valid sentiment / cross-sectional price-state implementation SHOULD minimally publish:

- `cohort_member_count`;
- `valid_member_count`;
- `excluded_member_count`;
- `cohort_horizon_seconds`;
- `return`;
- `absolute_return`;
- `asof_age_seconds`;
- `from_age_seconds`;
- `advance_count`;
- `decline_count`;
- `unchanged_count`;
- `advance_fraction`;
- `decline_fraction`;
- `unchanged_fraction`;
- `directional_participation`;
- `breadth`;
- `directional_agreement`;
- `directional_consensus`;
- `median_return`;
- `median_absolute_return`;
- `mean_absolute_return`;
- `rms_return`;
- `return_mad`;
- `magnitude_mad`;
- `return_interquartile_range` when estimable;
- `largest_move_symbol` as provenance when unique;
- `largest_move_tie_count`;
- `largest_absolute_return`;
- `largest_signed_return` when unique;
- `largest_move_share`;
- `peer_median_absolute_return`;
- `peer_magnitude_mad`;
- `largest_move_excess`;
- `largest_move_ratio` when defined;
- `largest_move_mad_excess` when defined;
- `same_direction_peer_count`;
- `opposite_direction_peer_count`;
- `zero_return_peer_count`;
- `same_direction_peer_fraction`;
- `opposite_direction_peer_fraction`;
- `zero_return_peer_fraction`;
- `median_asof_age_seconds`;
- `max_asof_age_seconds`;
- `median_from_age_seconds`;
- `median_return_baseline`;
- `median_return_divergence`;
- `median_return_zscore`;
- `breadth_baseline`;
- `breadth_divergence`;
- `breadth_zscore`;
- `median_absolute_return_baseline`;
- `median_absolute_return_ratio`;
- `median_absolute_return_zscore`;
- `return_dispersion_baseline`;
- `return_dispersion_ratio`;
- `return_dispersion_zscore`;
- `largest_move_share_baseline`;
- `largest_move_share_zscore`;
- `median_return_velocity`;
- `breadth_velocity`;
- `median_absolute_return_velocity`;
- `return_dispersion_velocity`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.

Metrics whose prerequisites are not satisfied are explicitly undefined rather than replaced with scores, labels, or provisional zeros.
